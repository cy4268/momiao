import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiClient } from '../api';
import { PokerTableClient } from './poker-socket';
import { pokerStreamAuthority, pokerStreamContext } from './poker-stream';
import { POKER_SUBPROTOCOL } from './poker-wire';
import self from './fixtures/g3-player-self.json';

const ticket='ct1.eyJzeW50aGV0aWMiOnRydWV9.c2lnbmF0dXJl',connection='a'.repeat(32);
const ok=(data:unknown)=>new Response(JSON.stringify({success:true,data}),{headers:{'Content-Type':'application/json'}});
class Socket extends EventTarget{
  readyState=0;bufferedAmount=0;protocol=POKER_SUBPROTOCOL;sent:string[]=[];
  send=vi.fn((text:string)=>{if(this.readyState!==1)throw Error('not open');this.sent.push(text);});
  close=vi.fn(()=>{this.readyState=3;this.dispatchEvent(new Event('close'));});
  open(){this.readyState=1;this.dispatchEvent(new Event('open'));}
  message(data:unknown){this.dispatchEvent(new MessageEvent('message',{data}));}
}
function frame(type:string,payload:unknown,seq=10){return JSON.stringify({type,event_id:`opaque-${seq}`,event_seq:seq,table_id:self.table_id,table_version:7,hand_id:self.hand.hand_id,hand_version:8,server_time:self.server_now,payload});}
function snapshot(){const v:any=structuredClone(self);v.viewer.control={connection_id:connection,session_id:self.viewer.session_id,mode:'CONTROLLER',control_epoch:'1'};return frame('table.snapshot',v);}
async function setup(){const fetcher=vi.fn().mockResolvedValueOnce(ok({access_token:'synthetic-access',access_expires_at:4102444800,user:{id:910002,username:'synthetic',display_name:'Synthetic',role:1},session:{sid:'synthetic-session'}})),api=new ApiClient(fetcher);await api.login('synthetic','synthetic');const sockets:Socket[]=[],factory=vi.fn(()=>{const s=new Socket();sockets.push(s);return s as unknown as WebSocket;});const client=new PokerTableClient(api,{socketFactory:factory});return{client,api,fetcher,sockets,factory};}
const active:PokerTableClient[]=[];afterEach(()=>{active.splice(0).forEach(c=>c.dispose());vi.useRealTimers();});
async function opened(){const r=await setup();active.push(r.client);r.fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));await r.client.connect(self.table_id,'PLAYER_SELF','CLAIM_CONTROL');r.sockets[0].open();return r;}
function authenticate(client:PokerTableClient,s:Socket){s.message(frame('auth.accepted',{request_id:JSON.parse(s.sent[0]).request_id,connection_id:connection},1));expect(client.getSnapshot()?.connection_state).toBe('SYNCING');s.message(snapshot());}
describe('real browser Poker socket client with a synthetic socket boundary',()=>{
  it('checks current capability and immutable context before synchronously latching and sending one exact action',async()=>{
    const r=await opened();authenticate(r.client,r.sockets[0]);const context=pokerStreamContext(r.client.getSnapshot()!),order:string[]=[];r.client.subscribe(()=>{if(r.client.getSnapshot()?.pending)order.push('pending');});r.sockets[0].send.mockImplementationOnce(text=>{order.push('send');r.sockets[0].sent.push(text);});
    await r.client.submit({type:'action',action_type:'CALL',target_to_units:'0'},context);expect(order).toEqual(['pending','send']);const action=JSON.parse(r.sockets[0].sent[1]);expect(action).toMatchObject({type:'hand.action',expected_table_version:7,expected_hand_version:8,control_epoch:1,hand_id:self.hand.hand_id,payload:{action_type:'CALL',requested_to_units:null}});expect(action.action_id).toMatch(/^ik1_[A-Za-z0-9_-]{43}$/);expect(action.request_id).not.toBe(action.action_id);
    await expect(r.client.submit({type:'action',action_type:'CALL',target_to_units:'0'},context)).rejects.toThrow();expect(r.sockets[0].sent).toHaveLength(2);expect(r.client.getSnapshot()?.pending?.request_id).toBe(action.request_id);
  });
  it('stale render, illegal action/amount, read-only proof, expired deadline and backpressure all stop before command I/O',async()=>{
    const r=await opened();authenticate(r.client,r.sockets[0]);const context=pokerStreamContext(r.client.getSnapshot()!),command={type:'action' as const,action_type:'CALL' as const,target_to_units:'0'};
    for(const [intent,c] of [[command,{...context,runtime_id:'stale'}],[{...command,action_type:'CHECK'},context],[{...command,action_type:'RAISE',target_to_units:'1'},context]] as const)await expect(r.client.submit(intent as never,c)).rejects.toThrow();
    r.sockets[0].bufferedAmount=65537;await expect(r.client.submit(command,context)).rejects.toThrow();r.sockets[0].bufferedAmount=0;
    const clock=vi.spyOn(performance,'now').mockReturnValue(performance.now()+120000);await expect(r.client.submit(command,context)).rejects.toThrow();clock.mockRestore();
    r.sockets[0].message(frame('control.changed',{connection_id:connection,session_id:self.viewer.session_id,mode:'READ_ONLY',control_epoch:'1'}));await expect(r.client.submit(command,pokerStreamContext(r.client.getSnapshot()!))).rejects.toThrow();expect(r.sockets[0].sent).toHaveLength(1);expect(r.client.getSnapshot()?.pending).toBeUndefined();
  });
  it('a reentrant revocation at the pending boundary prevents unsent I/O and clears only that unsent latch',async()=>{
    const r=await opened();authenticate(r.client,r.sockets[0]);let revoked=false;r.client.subscribe(()=>{if(!revoked&&r.client.getSnapshot()?.pending){revoked=true;r.sockets[0].message(frame('control.changed',{connection_id:connection,session_id:self.viewer.session_id,mode:'READ_ONLY',control_epoch:'1'}));}});
    await expect(r.client.submit({type:'action',action_type:'CALL',target_to_units:'0'},pokerStreamContext(r.client.getSnapshot()!))).rejects.toThrow();expect(r.sockets[0].sent).toHaveLength(1);expect(r.client.getSnapshot()?.pending).toBeUndefined();expect(pokerStreamAuthority(r.client.getSnapshot()!).can_control).toBe(false);
  });
  it('HTTP top-up receipts never replace amounts, correlate pending IDs and remain latched until their durable snapshot',async()=>{
    const r=await opened();authenticate(r.client,r.sockets[0]);const s=r.client.getSnapshot()!,before=s.table;const v:any=structuredClone(before);v.viewer.can_top_up=true;v.viewer.top_up_min_units='500000';v.viewer.top_up_max_units='5000000';v.viewer.control.mode='READ_ONLY';v.viewer.can_act=false;delete v.viewer.legal;r.sockets[0].message(frame('control.changed',v.viewer.control));r.sockets[0].message(frame('table.snapshot',v));const table=r.client.getSnapshot()!.table;expect(pokerStreamAuthority(r.client.getSnapshot()!).can_control).toBe(false);
    r.fetcher.mockResolvedValueOnce(ok({table_id:self.table_id,session_id:self.viewer.session_id,status:'PENDING',table_version:'8',duplicate:false,amount_units:'500000'}));await r.client.submit({type:'topup',amount_units:'500000'},pokerStreamContext(r.client.getSnapshot()!));
    expect(r.fetcher.mock.calls[2][0]).toBe(`/api/v1/poker/sessions/${self.viewer.session_id}/top-ups`);expect(JSON.parse(r.fetcher.mock.calls[2][1].body)).toEqual({request_id:r.client.getSnapshot()?.pending?.request_id,amount_units:'500000'});expect(r.client.getSnapshot()?.table).toBe(table);expect(r.client.getSnapshot()?.pending?.phase).toBe('ACKNOWLEDGED');
    const next=JSON.parse(frame('table.snapshot',v));next.table_version=8;next.payload.table_version='8';r.sockets[0].message(JSON.stringify(next));expect(r.client.getSnapshot()?.pending).toBeUndefined();expect(r.client.getSnapshot()?.last_receipt?.status).toBe('PENDING');
  });
  it('only explicit CLAIM_CONTROL read-only takeover targets this socket; its HTTP Receipt does not grant control',async()=>{
    const r=await opened();authenticate(r.client,r.sockets[0]);const v:any=JSON.parse(snapshot()).payload;v.viewer.control.mode='READ_ONLY';v.viewer.can_act=false;delete v.viewer.legal;r.sockets[0].message(frame('control.changed',v.viewer.control));r.sockets[0].message(frame('table.snapshot',v));
    r.fetcher.mockResolvedValueOnce(ok({table_id:self.table_id,session_id:self.viewer.session_id,status:'EPOCH_ADVANCED',table_version:'7',duplicate:false}));await r.client.submit({type:'takeover'},pokerStreamContext(r.client.getSnapshot()!));expect(JSON.parse(r.fetcher.mock.calls[2][1].body)).toEqual({request_id:expect.any(String),connection_id:connection});expect(pokerStreamAuthority(r.client.getSnapshot()!).can_control).toBe(false);expect(r.client.getSnapshot()?.pending).toBeUndefined();
    r.fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));await r.client.connect(self.table_id,'PLAYER_SELF','READ_ONLY');r.sockets[1].open();r.sockets[1].message(frame('auth.accepted',{request_id:JSON.parse(r.sockets[1].sent[0]).request_id,connection_id:connection}));r.sockets[1].message(frame('table.snapshot',v));await expect(r.client.submit({type:'takeover'},pokerStreamContext(r.client.getSnapshot()!))).rejects.toThrow();expect(r.fetcher).toHaveBeenCalledTimes(4);
  });
  it('a send exception retains the same unknown action IDs and closes readonly without exposing the raw error',async()=>{
    const r=await opened();authenticate(r.client,r.sockets[0]);r.sockets[0].send.mockImplementationOnce(()=>{throw Error('synthetic-private-send-error');});await expect(r.client.submit({type:'action',action_type:'CALL',target_to_units:'0'},pokerStreamContext(r.client.getSnapshot()!))).rejects.toMatchObject({uncertain:true});expect(r.client.getSnapshot()).toMatchObject({connection_state:'RECONNECTING',pending:{phase:'UNKNOWN',request_id:expect.any(String),action_id:expect.any(String)}});expect(JSON.stringify(r.client.getSnapshot())).not.toContain('synthetic-private');expect(r.sockets).toHaveLength(1);
  });
  it('a dropped mutation or heartbeat timeout preserves the original UNKNOWN outcome across explicit reconnect without replay',async()=>{
    vi.useFakeTimers();const r=await opened();authenticate(r.client,r.sockets[0]);await r.client.submit({type:'action',action_type:'CALL',target_to_units:'0'},pokerStreamContext(r.client.getSnapshot()!));const pending=r.client.getSnapshot()!.pending!;
    await vi.advanceTimersByTimeAsync(40000);expect(r.client.getSnapshot()).toMatchObject({connection_state:'RECONNECTING',pending:{request_id:pending.request_id,action_id:pending.action_id,phase:'UNKNOWN'}});r.fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));await r.client.connect(self.table_id,'PLAYER_SELF','CLAIM_CONTROL');r.sockets[1].open();authenticate(r.client,r.sockets[1]);expect(r.sockets[1].sent).toHaveLength(1);expect(r.client.getSnapshot()?.pending?.phase).toBe('UNKNOWN');
  });
  it('one scoped heartbeat waits for its matching pong; a wrong ID cannot prevent read-only timeout',async()=>{
    vi.useFakeTimers();const r=await opened();authenticate(r.client,r.sockets[0]);await vi.advanceTimersByTimeAsync(30000);const ping=JSON.parse(r.sockets[0].sent.at(-1)!);expect(ping.type).toBe('ping');expect(()=>r.client.ping()).toThrow();
    r.sockets[0].message(frame('pong',{request_id:'foreign-pong-0001'}));const delayed=vi.spyOn(performance,'now').mockReturnValue(performance.now()+10001);r.sockets[0].message(frame('pong',{request_id:ping.request_id}));delayed.mockRestore();expect(r.client.getSnapshot()).toMatchObject({connection_state:'RECONNECTING',last_error:'POKER_PONG_TIMEOUT'});expect(pokerStreamAuthority(r.client.getSnapshot()!).can_control).toBe(false);expect(r.sockets[0].close).toHaveBeenCalledTimes(1);r.client.disconnect();await vi.advanceTimersByTimeAsync(60000);expect(r.sockets).toHaveLength(1);expect(r.fetcher).toHaveBeenCalledTimes(2);
  });
  it('matching pong schedules the next local heartbeat and logout clears both timers without renewing any grant',async()=>{
    vi.useFakeTimers();const r=await opened();authenticate(r.client,r.sockets[0]);await vi.advanceTimersByTimeAsync(30000);const id=JSON.parse(r.sockets[0].sent.at(-1)!).request_id;r.sockets[0].message(frame('pong',{request_id:id}));await vi.advanceTimersByTimeAsync(10000);expect(r.client.getSnapshot()?.connection_state).toBe('LIVE');
    r.fetcher.mockResolvedValueOnce(ok({}));await r.api.logout();await vi.advanceTimersByTimeAsync(60000);expect(r.sockets[0].sent).toHaveLength(2);expect(r.client.getSnapshot()).toBeNull();expect(r.sockets).toHaveLength(1);
  });
  it('a reentrant identity teardown during auth publication prevents even the first ticket send',async()=>{
    const r=await setup();active.push(r.client);r.fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));await r.client.connect(self.table_id,'PLAYER_SELF','CLAIM_CONTROL');r.client.subscribe(()=>{if(r.client.getSnapshot()?.connection_state==='AUTH_PENDING')r.client.dispose();});r.sockets[0].open();expect(r.sockets[0].send).not.toHaveBeenCalled();expect(r.client.getSnapshot()).toBeNull();
  });
  it('uses fixed same-origin URL and sole subprotocol, sends exactly one first auth frame, then waits for pushed full snapshot',async()=>{
    const {client,sockets,factory}=await opened(),s=sockets[0];expect(factory.mock.calls[0]).toEqual([`${location.protocol==='https:'?'wss:':'ws:'}//${location.host}/ws/poker`,POKER_SUBPROTOCOL]);
    const auth=JSON.parse(s.sent[0]);expect(auth).toMatchObject({type:'auth.connect',table_id:self.table_id,payload:{poker_connect_ticket:ticket}});expect(Object.keys(auth)).toHaveLength(9);expect(s.sent).toHaveLength(1);expect(JSON.stringify(client.getSnapshot())).not.toContain(ticket);expect(JSON.stringify(client)).not.toContain(ticket);expect(JSON.stringify(client)).not.toContain('synthetic-access');
    authenticate(client,s);expect(client.getSnapshot()?.connection_state).toBe('LIVE');expect(pokerStreamAuthority(client.getSnapshot()!).can_control).toBe(true);expect(s.sent).toHaveLength(1);
  });
  it('does not send the ticket a second time if open is raised twice',async()=>{
    const {client,sockets}=await opened(),s=sockets[0];s.open();expect(s.sent).toHaveLength(1);expect(client.getSnapshot()?.connection_state).toBe('DEGRADED');expect(s.close).toHaveBeenCalledTimes(1);
  });
  it.each(['wrong-protocol','binary','malformed'])('fails closed on %s with no raw error/private data leakage',async(mode)=>{
    const r=await setup();active.push(r.client);r.fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));await r.client.connect(self.table_id,'PLAYER_SELF','CLAIM_CONTROL');const s=r.sockets[0];if(mode==='wrong-protocol')s.protocol='other';s.open();
    if(mode==='binary')s.message(new ArrayBuffer(2));if(mode==='malformed')s.message('private-secret-not-json');expect(r.client.getSnapshot()?.connection_state).toBe('DEGRADED');expect(s.close).toHaveBeenCalledTimes(1);expect(JSON.stringify(r.client.getSnapshot())).not.toContain('private-secret');
  });
  it('old socket messages and pending mint replies never populate the replacement table scope',async()=>{
    const r=await opened();authenticate(r.client,r.sockets[0]);let finish!:(v:Response)=>void;r.fetcher.mockReturnValueOnce(new Promise<Response>(x=>finish=x));const next=r.client.connect(self.table_id,'PLAYER_SELF','READ_ONLY');const generation=r.client.getSnapshot()!.scope.request_generation;
    r.sockets[0].message(snapshot());expect(r.client.getSnapshot()?.connection_state).toBe('CONNECTING');expect(r.client.getSnapshot()?.table).toBeUndefined();r.client.disconnect();finish(ok({private_secret:'stale mint'}));await next;expect(r.sockets).toHaveLength(1);expect(r.client.getSnapshot()?.connection_state).toBe('DISCONNECTED');expect(r.client.getSnapshot()?.scope.request_generation).toBe(generation);
  });
  it('logout synchronously clears cards, closes the socket and discards any later private frame',async()=>{
    const r=await opened();authenticate(r.client,r.sockets[0]);r.fetcher.mockResolvedValueOnce(ok({}));const logout=r.api.logout();expect(r.client.getSnapshot()).toBeNull();expect(r.sockets[0].close).toHaveBeenCalledTimes(1);r.sockets[0].message(snapshot());await logout;expect(r.client.getSnapshot()).toBeNull();
  });
  it('close remains read-only and never remints or reopens until an explicit reconnect',async()=>{
    const r=await opened();authenticate(r.client,r.sockets[0]);r.sockets[0].close();expect(r.client.getSnapshot()?.connection_state).toBe('DISCONNECTED');expect(pokerStreamAuthority(r.client.getSnapshot()!).can_control).toBe(false);expect(r.fetcher).toHaveBeenCalledTimes(2);expect(r.sockets).toHaveLength(1);
    r.fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));await r.client.connect(self.table_id,'PLAYER_SELF','CLAIM_CONTROL');expect(r.sockets).toHaveLength(2);expect(r.client.getSnapshot()?.table).toBeUndefined();
  });
  it('explicit sync and ping are readonly, use fresh IDs, and are never mutation retries',async()=>{
    const r=await opened();authenticate(r.client,r.sockets[0]);r.client.sync();r.client.ping();const frames=r.sockets[0].sent.map(x=>JSON.parse(x));expect(frames.map(x=>x.type)).toEqual(['auth.connect','sync.request','ping']);for(const f of frames.slice(1))expect(f).toMatchObject({action_id:null,hand_id:null,expected_table_version:0,expected_hand_version:0,control_epoch:0,payload:{}});expect(new Set(frames.map(f=>f.request_id)).size).toBe(3);
  });
  it('dispose detaches observers, clears all private views and prevents future connections',async()=>{
    const r=await opened(),notify=vi.fn(),unsubscribe=r.client.subscribe(notify);authenticate(r.client,r.sockets[0]);expect(notify).toHaveBeenCalled();unsubscribe();r.client.dispose();expect(r.client.getSnapshot()).toBeNull();await expect(r.client.connect(self.table_id,'PLAYER_SELF','CLAIM_CONTROL')).rejects.toThrow();expect(r.sockets).toHaveLength(1);
  });
});
