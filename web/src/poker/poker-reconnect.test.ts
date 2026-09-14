import { afterEach, expect, it, vi } from 'vitest';
import { ApiClient } from '../api';
import { PokerTableClient } from './poker-socket';
import { pokerStreamAuthority, pokerStreamContext } from './poker-stream';
import { POKER_SUBPROTOCOL } from './poker-wire';
import self from './fixtures/g3-player-self.json';

const ticket='ct1.eyJzeW50aGV0aWMiOnRydWV9.c2lnbmF0dXJl';
const ok=(data:unknown)=>new Response(JSON.stringify({success:true,data}),{headers:{'Content-Type':'application/json'}});
class Socket extends EventTarget {
  readyState=0;bufferedAmount=0;protocol=POKER_SUBPROTOCOL;sent:string[]=[];
  send(text:string){if(this.readyState!==1)throw Error('closed');this.sent.push(text);}
  close(){this.readyState=3;this.dispatchEvent(new CloseEvent('close',{code:1000}));}
  remoteClose(code:number,reason=''){this.readyState=3;this.dispatchEvent(new CloseEvent('close',{code,reason}));}
  open(){this.readyState=1;this.dispatchEvent(new Event('open'));}
  message(data:string){this.dispatchEvent(new MessageEvent('message',{data}));}
}
const active:PokerTableClient[]=[];
afterEach(()=>{active.splice(0).forEach(c=>c.dispose());vi.restoreAllMocks();vi.useRealTimers();});
async function setup(intent:'READ_ONLY'|'CLAIM_CONTROL'='READ_ONLY') {
  vi.useFakeTimers();vi.spyOn(Math,'random').mockReturnValue(.5);
  const fetcher=vi.fn().mockResolvedValueOnce(ok({access_token:'synthetic',access_expires_at:4102444800,user:{id:910002,username:'synthetic',role:1},session:{sid:'synthetic'}})).mockImplementation(()=>Promise.resolve(ok({poker_connect_ticket:ticket})));
  const api=new ApiClient(fetcher);await api.login('synthetic','synthetic');const sockets:Socket[]=[];
  const client=new PokerTableClient(api,{socketFactory:()=>{const s=new Socket();sockets.push(s);return s as unknown as WebSocket;}});active.push(client);
  await client.connect(self.table_id,'PLAYER_SELF',intent);return{client,sockets,fetcher,api};
}
function frame(type:string,payload:unknown){return JSON.stringify({type,event_id:'synthetic-reconnect',event_seq:10,table_id:self.table_id,table_version:7,hand_id:self.hand.hand_id,hand_version:8,server_time:self.server_now,payload});}
function ack(s:Socket,connection:string){s.open();s.message(frame('auth.accepted',{request_id:JSON.parse(s.sent[0]).request_id,connection_id:connection}));}
function snapshot(s:Socket,connection:string,mode:'READ_ONLY'|'CONTROLLER'='READ_ONLY'){
  const v:any=structuredClone(self);v.viewer.control={connection_id:connection,session_id:v.viewer.session_id,mode,control_epoch:'1'};
  if(mode==='READ_ONLY'){v.viewer.can_act=false;delete v.viewer.legal;}s.message(frame('table.snapshot',v));
}
const action={type:'action' as const,action_type:'CALL' as const,target_to_units:'0'};

it('initial opaque 1006 retries with fresh mint, original READ_ONLY intent and a new scope; ACK alone grants nothing',async()=>{
  const r=await setup(),first=r.client.getSnapshot()!.scope;r.sockets[0].dispatchEvent(new Event('error'));r.sockets[0].remoteClose(1006);
  expect(r.client.getSnapshot()).toMatchObject({connection_state:'RECONNECTING',last_error:'POKER_CONNECTION_UNKNOWN'});
  await vi.advanceTimersByTimeAsync(499);expect(r.sockets).toHaveLength(1);await vi.advanceTimersByTimeAsync(1);expect(r.sockets).toHaveLength(2);
  expect(r.client.getSnapshot()!.scope.request_generation).toBeGreaterThan(first.request_generation);
  expect(r.fetcher.mock.calls.slice(1).map(c=>JSON.parse(c[1]!.body as string))).toEqual(Array(2).fill({target_table_id:self.table_id,control_intent:'READ_ONLY'}));
  ack(r.sockets[1],'b'.repeat(32));expect(r.client.getSnapshot()?.connection_state).toBe('SYNCING');await expect(r.client.submit(action,pokerStreamContext(r.client.getSnapshot()!))).rejects.toThrow();
  snapshot(r.sockets[1],'b'.repeat(32));expect(r.client.getSnapshot()?.connection_state).toBe('LIVE');expect(pokerStreamAuthority(r.client.getSnapshot()!).can_control).toBe(false);
  expect(r.sockets.flatMap(s=>s.sent.map(x=>JSON.parse(x).type))).toEqual(['auth.connect']);
});
it('exponential delays have no attempt cap and reset only after a matching full snapshot, not auth ACK',async()=>{
  const r=await setup();
  for(const delay of [500,1000,2000,4000,5000,5000,5000]){
    const n=r.sockets.length,s=r.sockets.at(-1)!;ack(s,'a'.repeat(32));s.remoteClose(1006);await vi.advanceTimersByTimeAsync(delay-1);expect(r.sockets).toHaveLength(n);await vi.advanceTimersByTimeAsync(1);expect(r.sockets).toHaveLength(n+1);
  }
  ack(r.sockets.at(-1)!,'b'.repeat(32));snapshot(r.sockets.at(-1)!,'b'.repeat(32));r.sockets.at(-1)!.remoteClose(1012);await vi.advanceTimersByTimeAsync(500);expect(r.sockets).toHaveLength(9);
});
it.each([[0,400],[1,600]])('jitter random %s gives the frozen initial delay %s ms',async(random,delay)=>{
  const r=await setup();vi.mocked(Math.random).mockReturnValue(random);r.sockets[0].remoteClose(1006);await vi.advanceTimersByTimeAsync(delay-1);expect(r.sockets).toHaveLength(1);await vi.advanceTimersByTimeAsync(1);expect(r.sockets).toHaveLength(2);
});
it('positive jitter never exceeds the frozen 5-second final delay cap',async()=>{
  const r=await setup();vi.mocked(Math.random).mockReturnValue(1);
  for(const delay of [600,1200,2400,4800,5000,5000]){const n=r.sockets.length;r.sockets.at(-1)!.remoteClose(1006);await vi.advanceTimersByTimeAsync(delay-1);expect(r.sockets).toHaveLength(n);await vi.advanceTimersByTimeAsync(1);expect(r.sockets).toHaveLength(n+1);}
});
it('same-auth last snapshot stays read-only through retry; old socket frames and authority never carry forward',async()=>{
  const r=await setup('CLAIM_CONTROL');ack(r.sockets[0],'a'.repeat(32));snapshot(r.sockets[0],'a'.repeat(32),'CONTROLLER');const previous=r.client.getSnapshot()!,rendered=pokerStreamContext(previous);
  r.sockets[0].remoteClose(1006);expect(r.client.getSnapshot()?.table).toBe(previous.table);expect(pokerStreamAuthority(r.client.getSnapshot()!).can_control).toBe(false);
  await vi.advanceTimersByTimeAsync(500);expect(r.client.getSnapshot()?.table).toBe(previous.table);expect(r.client.getSnapshot()?.control).toBeUndefined();expect(r.client.getSnapshot()?.connection_id).toBeUndefined();
  snapshot(r.sockets[0],'a'.repeat(32),'CONTROLLER');ack(r.sockets[1],'b'.repeat(32));await expect(r.client.submit(action,rendered)).rejects.toThrow();snapshot(r.sockets[1],'b'.repeat(32),'CONTROLLER');
  expect(pokerStreamAuthority(r.client.getSnapshot()!).can_control).toBe(true);await expect(r.client.submit(action,rendered)).rejects.toThrow();expect(r.sockets[1].sent).toHaveLength(1);
});
it('explicit disconnect stops retries; explicit connect replaces old retry plan and logout/dispose clear private state and timers',async()=>{
  const r=await setup();r.sockets[0].remoteClose(1006);expect(r.client.getSnapshot()?.connection_state).toBe('RECONNECTING');r.client.disconnect();await vi.advanceTimersByTimeAsync(5000);expect(r.sockets).toHaveLength(1);
  await r.client.connect(self.table_id,'PLAYER_SELF','READ_ONLY');r.sockets[1].remoteClose(1006);await r.client.connect(self.table_id,'PLAYER_SELF','CLAIM_CONTROL');await vi.advanceTimersByTimeAsync(5000);expect(r.sockets).toHaveLength(3);expect(r.client.getSnapshot()?.ticket_intent).toBe('CLAIM_CONTROL');
  ack(r.sockets[2],'c'.repeat(32));snapshot(r.sockets[2],'c'.repeat(32),'CONTROLLER');r.sockets[2].remoteClose(1006);r.fetcher.mockResolvedValueOnce(ok({}));await r.api.logout();expect(r.client.getSnapshot()).toBeNull();await vi.advanceTimersByTimeAsync(10000);expect(r.sockets).toHaveLength(3);expect(vi.getTimerCount()).toBe(0);
  const other=await setup();other.sockets[0].remoteClose(1006);other.client.dispose();await vi.advanceTimersByTimeAsync(10000);expect(other.sockets).toHaveLength(1);expect(other.client.getSnapshot()).toBeNull();
});
it('account/session replacement clears retained private data and rejects an in-flight old retry mint before opening a socket',async()=>{
  const r=await setup();ack(r.sockets[0],'a'.repeat(32));snapshot(r.sockets[0],'a'.repeat(32));let reply!:(r:Response)=>void;r.fetcher.mockImplementationOnce(()=>new Promise<Response>(resolve=>reply=resolve));r.sockets[0].remoteClose(1006);await vi.advanceTimersByTimeAsync(500);expect(r.client.getSnapshot()?.table).toBeDefined();
  r.fetcher.mockResolvedValueOnce(ok({access_token:'other',access_expires_at:4102444800,user:{id:910003,username:'other',role:1},session:{sid:'other-session'}}));await r.api.login('other','synthetic');expect(r.client.getSnapshot()).toBeNull();reply(ok({poker_connect_ticket:ticket}));await vi.advanceTimersByTimeAsync(10000);expect(r.sockets).toHaveLength(1);expect(vi.getTimerCount()).toBe(0);
});
it('reentrant disconnect at retry publication cancels its timer without stale callbacks reviving the connection',async()=>{
  const r=await setup();r.client.subscribe(()=>{if(r.client.getSnapshot()?.connection_state==='RECONNECTING')r.client.disconnect();});r.sockets[0].remoteClose(1006);await vi.advanceTimersByTimeAsync(10000);expect(r.client.getSnapshot()?.connection_state).toBe('DISCONNECTED');expect(r.sockets).toHaveLength(1);expect(vi.getTimerCount()).toBe(0);
});
it('an already-queued old timer callback cannot erase the replacement plan timer handle',async()=>{
  const r=await setup(),timers=vi.spyOn(globalThis,'setTimeout');r.sockets[0].remoteClose(1006);const stale=timers.mock.calls.at(-1)![0] as ()=>void;
  await r.client.connect(self.table_id,'PLAYER_SELF','READ_ONLY');r.sockets[1].remoteClose(1006);expect(vi.getTimerCount()).toBe(1);stale();r.client.disconnect();expect(vi.getTimerCount()).toBe(0);await vi.advanceTimersByTimeAsync(10000);expect(r.sockets).toHaveLength(2);
});
it.each(['heartbeat','pong'])('a queued old %s callback cannot erase the reauthenticated socket timer handle',async(kind)=>{
  const r=await setup(),timers=vi.spyOn(globalThis,'setTimeout');ack(r.sockets[0],'a'.repeat(32));snapshot(r.sockets[0],'a'.repeat(32));if(kind==='pong')r.client.ping();const stale=timers.mock.calls.at(-1)![0] as ()=>void;
  r.sockets[0].remoteClose(1006);await vi.advanceTimersByTimeAsync(500);ack(r.sockets[1],'b'.repeat(32));snapshot(r.sockets[1],'b'.repeat(32));if(kind==='pong')r.client.ping();stale();r.client.disconnect();expect(vi.getTimerCount()).toBe(0);
});
it('only known transient close signals retry; auth/protocol and ambiguous 1011 stop rather than guess',async()=>{
  for(const [code,reason,retry] of [[1013,'',true],[1011,'POKER_AUTH_UNAVAILABLE',true],[1008,'POKER_AUTH_UNAVAILABLE',false],[1011,'POKER_INTERNAL_ERROR',false],[1000,'',false],[1002,'',false]] as const){
    const r=await setup();r.sockets[0].remoteClose(code,reason);await vi.advanceTimersByTimeAsync(500);expect(r.sockets).toHaveLength(retry?2:1);r.client.dispose();
  }
  const malformed=await setup();malformed.sockets[0].open();malformed.sockets[0].message('private invalid data');await vi.advanceTimersByTimeAsync(10000);expect(malformed.sockets).toHaveLength(1);
});
it('mint retries transport loss or explicit 503 auth unavailable, but stops explicit 403 and unavailable/projection responses',async()=>{
  for(const [status,code,retry] of [[503,'POKER_AUTH_UNAVAILABLE',true],[403,'SESSION_INVALID',false],[503,'POKER_UNAVAILABLE',false],[503,'POKER_CAPABILITY_UNAVAILABLE',false],[500,'POKER_INTERNAL_ERROR',false]] as const){
    const r=await setup();r.fetcher.mockResolvedValueOnce(new Response(JSON.stringify({success:false,code}),{status}));await r.client.connect(self.table_id,'PLAYER_SELF','READ_ONLY');await vi.advanceTimersByTimeAsync(500);expect(r.sockets).toHaveLength(retry?2:1);r.client.dispose();
  }
  const r=await setup();r.fetcher.mockRejectedValueOnce(new TypeError('synthetic network loss'));await r.client.connect(self.table_id,'PLAYER_SELF','READ_ONLY');await vi.advanceTimersByTimeAsync(500);expect(r.sockets).toHaveLength(2);
});
it('unknown pending IDs survive automatic reauthentication and full snapshot, with no mutation replay or automatic takeover',async()=>{
  const r=await setup('CLAIM_CONTROL');ack(r.sockets[0],'a'.repeat(32));snapshot(r.sockets[0],'a'.repeat(32),'CONTROLLER');await r.client.submit(action,pokerStreamContext(r.client.getSnapshot()!));const pending=r.client.getSnapshot()!.pending!;
  r.sockets[0].remoteClose(1006);await vi.advanceTimersByTimeAsync(500);ack(r.sockets[1],'b'.repeat(32));snapshot(r.sockets[1],'b'.repeat(32),'CONTROLLER');expect(r.client.getSnapshot()?.pending).toEqual({...pending,phase:'UNKNOWN'});await expect(r.client.submit(action,pokerStreamContext(r.client.getSnapshot()!))).rejects.toThrow();expect(r.sockets[1].sent.map(x=>JSON.parse(x).type)).toEqual(['auth.connect']);
});
it('foreground restoration requests server sync and waits for snapshot without action, takeover or bypassing retry delay',async()=>{
  const r=await setup('CLAIM_CONTROL');ack(r.sockets[0],'a'.repeat(32));snapshot(r.sockets[0],'a'.repeat(32),'CONTROLLER');vi.spyOn(document,'visibilityState','get').mockReturnValue('visible');document.dispatchEvent(new Event('visibilitychange'));
  expect(r.client.getSnapshot()?.connection_state).toBe('SYNCING');expect(r.sockets[0].sent.map(x=>JSON.parse(x).type)).toEqual(['auth.connect','sync.request']);await expect(r.client.submit(action,pokerStreamContext(r.client.getSnapshot()!))).rejects.toThrow();snapshot(r.sockets[0],'a'.repeat(32),'CONTROLLER');expect(r.client.getSnapshot()?.connection_state).toBe('LIVE');
  r.sockets[0].remoteClose(1006);document.dispatchEvent(new Event('visibilitychange'));await vi.advanceTimersByTimeAsync(499);expect(r.sockets).toHaveLength(1);r.client.dispose();document.dispatchEvent(new Event('visibilitychange'));expect(r.client.getSnapshot()).toBeNull();
});
