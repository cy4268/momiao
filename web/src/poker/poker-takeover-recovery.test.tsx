import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { ApiClient } from '../api';
import { PokerTableClient } from './poker-socket';
import { PokerTable } from './PokerTable';
import { pokerStreamAuthority, pokerStreamContext } from './poker-stream';
import { hasAuthority, type PokerTicketIntent } from './poker-ui-types';
import { POKER_SUBPROTOCOL } from './poker-wire';
import self from './fixtures/g3-player-self.json';

const ticket='ct1.eyJzeW50aGV0aWMiOnRydWV9.c2lnbmF0dXJl';
const ok=(data:unknown)=>new Response(JSON.stringify({success:true,data}),{headers:{'Content-Type':'application/json'}});
const clients:PokerTableClient[]=[];
afterEach(()=>{clients.splice(0).forEach(c=>c.dispose());vi.restoreAllMocks();});
class Socket extends EventTarget {
  readyState=0;bufferedAmount=0;protocol=POKER_SUBPROTOCOL;sent:string[]=[];
  constructor(readonly id:string){super();}
  send(text:string){this.sent.push(text);}close(){this.readyState=3;this.dispatchEvent(new CloseEvent('close',{code:1000}));}
  open(){this.readyState=1;this.dispatchEvent(new Event('open'));}message(data:string){this.dispatchEvent(new MessageEvent('message',{data}));}
}
function frame(type:string,payload:any,version=7){return JSON.stringify({type,event_id:'aux-synthetic',event_seq:10,table_id:self.table_id,table_version:version,hand_id:self.hand.hand_id,hand_version:version+1,server_time:self.server_now,payload});}
function full(s:Socket,version=7,controller=false,epoch='1'){
  const v:any=structuredClone(self);v.table_version=String(version);v.hand.hand_version=String(version+1);v.viewer.control_epoch=epoch;
  v.viewer.control={connection_id:s.id,session_id:v.viewer.session_id,mode:controller?'CONTROLLER':'READ_ONLY',control_epoch:epoch};
  if(!controller){v.viewer.can_act=false;delete v.viewer.legal;}s.message(frame('table.snapshot',v,version));
}
function authenticated(s:Socket,version=7,controller=false){s.open();s.message(frame('auth.accepted',{request_id:JSON.parse(s.sent[0]).request_id,connection_id:s.id}));full(s,version,controller);}
async function setup(controller=true){
  const fetcher=vi.fn().mockResolvedValueOnce(ok({access_token:'synthetic',access_expires_at:4102444800,user:{id:910002,username:'synthetic',role:1},session:{sid:'synthetic'}}));
  const api=new ApiClient(fetcher);await api.login('synthetic','synthetic');const sockets:Socket[]=[];
  const client=new PokerTableClient(api,{socketFactory:()=>{const s=new Socket(String(sockets.length+1).repeat(32));sockets.push(s);return s as unknown as WebSocket;}});clients.push(client);
  fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));await client.connect(self.table_id,'PLAYER_SELF','CLAIM_CONTROL');authenticated(sockets[0],7,controller);return{client,api,fetcher,sockets};
}
type Runtime=Awaited<ReturnType<typeof setup>>;
const context=(r:Runtime)=>pokerStreamContext(r.client.getSnapshot()!);
const posts=(r:Runtime,suffix:string)=>r.fetcher.mock.calls.filter(([p])=>p.endsWith(suffix));
const auxiliaryKey=(r:Runtime)=>JSON.parse(posts(r,'/take-over').at(-1)![1].body).request_id;
const receipt=(version='10')=>({table_id:self.table_id,session_id:self.viewer.session_id,status:'CONTROL_EPOCH_ADVANCED',table_version:version,control_epoch:'2',duplicate:false});
function lookup(r:Runtime,kind='takeover',found=false){const p=r.client.getSnapshot()!.pending;return{table_id:self.table_id,kind,mutation_id:kind==='takeover'?auxiliaryKey(r):p!.action_id,state:found?'FOUND':'NOT_FOUND',...(found?{receipt:receipt()}:{})};}
async function reconnect(r:Runtime,intent:PokerTicketIntent='CLAIM_CONTROL'){
  r.client.disconnect();r.fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));await r.client.connect(self.table_id,'PLAYER_SELF',intent);
  r.fetcher.mockResolvedValueOnce(ok(lookup(r,'action')));authenticated(r.sockets.at(-1)!,9);await r.client.recheckPending();
}
async function unknown(intent:PokerTicketIntent='CLAIM_CONTROL'){
  const r=await setup();await r.client.submit({type:'action',action_type:'CALL',target_to_units:'0'},context(r));await reconnect(r,intent);return r;
}
async function auxiliaryUnknown(){const r=await unknown();r.fetcher.mockRejectedValueOnce(Error('synthetic lost takeover'));await expect(r.client.submit({type:'takeover'},context(r))).rejects.toThrow();expect(posts(r,'/take-over')).toHaveLength(1);return r;}
const props=(r:Runtime)=>({table:r.client.getSnapshot()!.table!,authority:pokerStreamAuthority(r.client.getSnapshot()!),ui:{bet:{action_type:'RAISE' as const,amount_chips:'1'},top_up_chips:'1',confirm_leave:false,confirm_takeover:false},onUiChange:vi.fn(),onIntent:vi.fn()});

describe('R2 independent control intent and primary UNKNOWN',()=>{
  it('sends a separate takeover key, blocks duplicates/business retry, and leaves original query available',async()=>{
    const r=await unknown(),original={...r.client.getSnapshot()!.pending};let reply!:(v:Response)=>void;
    r.fetcher.mockReturnValueOnce(new Promise<Response>(resolve=>reply=resolve));const send=r.client.submit({type:'takeover'},context(r));void send.catch(()=>{});
    expect(posts(r,'/take-over')).toHaveLength(1);expect(auxiliaryKey(r)).toMatch(/^ik1_[A-Za-z0-9_-]{43}$/);expect(auxiliaryKey(r)).not.toBe(original.action_id);
    expect(r.client.getSnapshot()?.pending).toEqual(original);expect(r.client.getSnapshot()?.control_recovery?.phase).toBe('SENT');
    expect(JSON.stringify(r.client.getSnapshot()?.control_recovery)).not.toContain(auxiliaryKey(r));expect(localStorage.length+sessionStorage.length).toBe(0);
    await expect(r.client.submit({type:'takeover'},context(r))).rejects.toThrow();await expect(r.client.retryPending(context(r))).rejects.toThrow();
    r.fetcher.mockResolvedValueOnce(ok(lookup(r,'action')));await r.client.recheckPending();expect(posts(r,'/receipt-query').at(-1)![1].body).toContain(original.action_id!);
    reply(ok(receipt()));await send;expect(r.client.getSnapshot()?.pending).toEqual(original);expect(r.client.getSnapshot()?.control_recovery?.phase).toBe('ACKNOWLEDGED');
    expect(pokerStreamAuthority(r.client.getSnapshot()!).can_control).toBe(false);full(r.sockets.at(-1)!,10,true,'2');
    expect(r.client.getSnapshot()?.control_recovery).toBeUndefined();expect(r.client.getSnapshot()?.pending).toEqual(original);expect(r.client.getSnapshot()?.can_retry_pending).toBe(true);
    expect(r.sockets.flatMap(s=>s.sent).filter(t=>JSON.parse(t).type==='hand.action')).toHaveLength(1);
    await r.client.retryPending(context(r));const actions=r.sockets.flatMap(s=>s.sent.map(t=>JSON.parse(t))).filter(f=>f.type==='hand.action');
    expect(actions).toHaveLength(2);expect(actions[1]).toMatchObject({action_id:original.action_id,request_id:original.request_id,expected_table_version:10,expected_hand_version:11,control_epoch:2});
  });
  it('ordinary takeover without a primary pending uses the same separate lane and waits for a full version, not epoch metadata',async()=>{
    const r=await setup(false);r.fetcher.mockResolvedValueOnce(ok(receipt('8')));await r.client.submit({type:'takeover'},context(r));
    expect(r.client.getSnapshot()?.pending).toBeUndefined();expect(r.client.getSnapshot()?.control_recovery?.phase).toBe('ACKNOWLEDGED');
    expect(hasAuthority(pokerStreamAuthority(r.client.getSnapshot()!))).toBe(false);expect(pokerStreamAuthority(r.client.getSnapshot()!).can_control).toBe(false);
    await expect(r.client.submit({type:'topup',amount_units:'500000'},context(r))).rejects.toThrow();expect(posts(r,'/top-ups')).toHaveLength(0);
    full(r.sockets[0],8,true,'2');expect(r.client.getSnapshot()?.control_recovery).toBeUndefined();expect(pokerStreamAuthority(r.client.getSnapshot()!).can_control).toBe(true);
  });
});
describe('R2 explicit READ_ONLY to CLAIM_CONTROL connection',()=>{
  it('enables only the explicit connection intent first and never takes over on the fresh readonly snapshot',async()=>{
    const r=await unknown('READ_ONLY'),p=props(r);render(<PokerTable {...p}/>);fireEvent.click(screen.getByRole('button',{name:'申请控制连接'}));
    expect(p.onIntent).toHaveBeenCalledWith({type:'reconnect',control_intent:'CLAIM_CONTROL'},context(r));
    await expect(r.client.submit({type:'takeover'},context(r))).rejects.toThrow();expect(posts(r,'/take-over')).toHaveLength(0);
    await reconnect(r);expect(posts(r,'/take-over')).toHaveLength(0);expect(pokerStreamAuthority(r.client.getSnapshot()!).can_control).toBe(false);
    r.fetcher.mockResolvedValueOnce(ok(receipt()));await r.client.submit({type:'takeover'},context(r));expect(posts(r,'/take-over')).toHaveLength(1);
  });
});
describe('R2 auxiliary read and explicit same-target retry',()=>{
  it('coalesces only its own locator query, retries its original key once and never acknowledges the primary from its receipt',async()=>{
    const r=await auxiliaryUnknown(),original={...r.client.getSnapshot()!.pending},key=auxiliaryKey(r);let reply!:(v:Response)=>void;
    r.fetcher.mockReturnValueOnce(new Promise<Response>(resolve=>reply=resolve));const a=r.client.recheckTakeover(),b=r.client.recheckTakeover();expect(a).toBe(b);
    await Promise.resolve();expect(JSON.parse(posts(r,'/receipt-query').at(-1)![1].body)).toEqual({kind:'takeover',mutation_id:key});reply(ok(lookup(r)));await a;
    r.fetcher.mockRejectedValueOnce(Error('synthetic query failed'));await r.client.recheckTakeover();expect(r.client.getSnapshot()?.control_recovery?.can_retry).toBe(false);
    await expect(r.client.retryTakeover(context(r))).rejects.toThrow();r.fetcher.mockResolvedValueOnce(ok(lookup(r)));await r.client.recheckTakeover();
    expect(r.client.getSnapshot()?.control_recovery?.can_retry).toBe(true);let finish!:(v:Response)=>void;r.fetcher.mockReturnValueOnce(new Promise<Response>(resolve=>finish=resolve));
    const retry=r.client.retryTakeover(context(r));await expect(r.client.retryTakeover(context(r))).rejects.toThrow();expect(posts(r,'/take-over')).toHaveLength(2);expect(auxiliaryKey(r)).toBe(key);
    finish(ok(receipt()));await retry;expect(r.client.getSnapshot()?.pending).toEqual(original);expect(pokerStreamAuthority(r.client.getSnapshot()!).can_control).toBe(false);
  });
  it('rejects a mismatched auxiliary target and treats a genuine found epoch only as its own historical receipt',async()=>{
    const r=await auxiliaryUnknown(),original={...r.client.getSnapshot()!.pending};r.fetcher.mockResolvedValueOnce(ok({...lookup(r,'takeover',true),receipt:{...receipt(),session_id:self.table_id}}));
    await r.client.recheckTakeover();expect(r.client.getSnapshot()?.control_recovery?.phase).toBe('UNKNOWN');expect(r.client.getSnapshot()?.pending).toEqual(original);
    r.fetcher.mockResolvedValueOnce(ok(lookup(r,'takeover',true)));await r.client.recheckTakeover();expect(r.client.getSnapshot()?.control_recovery?.phase).toBe('ACKNOWLEDGED');
    expect(r.client.getSnapshot()?.pending).toEqual(original);expect(pokerStreamAuthority(r.client.getSnapshot()!).can_control).toBe(false);
  });
});
describe('R2 reentrant teardown and the documented R3 boundary',()=>{
  it('a reentrant initial pre-send disconnect clears only the definitely-unsent auxiliary lane, not the primary UNKNOWN',async()=>{
    const r=await unknown(),original={...r.client.getSnapshot()!.pending};let once=true;
    r.client.subscribe(()=>{if(once&&r.client.getSnapshot()?.control_recovery?.phase==='SENT'){once=false;r.client.disconnect();}});
    await expect(r.client.submit({type:'takeover'},context(r))).rejects.toThrow();expect(posts(r,'/take-over')).toHaveLength(0);
    expect(r.client.getSnapshot()?.control_recovery).toBeUndefined();expect(r.client.getSnapshot()?.pending).toEqual(original);
  });
  it('keeps both unknowns on retry pre-send disconnect; a fresh connection archives the old target query-only',async()=>{
    const r=await auxiliaryUnknown(),original={...r.client.getSnapshot()!.pending},key=auxiliaryKey(r);r.fetcher.mockResolvedValueOnce(ok(lookup(r)));await r.client.recheckTakeover();
    let once=true;const off=r.client.subscribe(()=>{if(once&&r.client.getSnapshot()?.control_recovery?.phase==='SENT'){once=false;r.client.disconnect();}});
    await expect(r.client.retryTakeover(context(r))).rejects.toThrow();off();expect(posts(r,'/take-over')).toHaveLength(1);expect(r.client.getSnapshot()?.pending).toEqual(original);
    await reconnect(r);r.fetcher.mockResolvedValueOnce(ok(lookup(r)));await r.client.recheckTakeover(true);expect(r.client.getSnapshot()?.control_history).toEqual({count:1,querying:false});
    expect(r.client.getSnapshot()?.control_recovery).toBeUndefined();await expect(r.client.retryTakeover(context(r))).rejects.toThrow();expect(auxiliaryKey(r)).toBe(key);expect(posts(r,'/take-over')).toHaveLength(1);
  });
  it('teardown and owner/table changes clear auxiliary private references and drop late invalid queries',async()=>{
    for(const mode of ['logout','dispose','account','session','table']){const r=await auxiliaryUnknown();let reply!:(v:Response)=>void;r.fetcher.mockReturnValueOnce(new Promise<Response>(resolve=>reply=resolve));
      const read=r.client.recheckTakeover();await Promise.resolve();
      if(mode==='logout'){r.fetcher.mockResolvedValueOnce(ok({}));await r.api.logout();}else if(mode==='dispose')r.client.dispose();
      else if(mode==='table'){r.fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));await r.client.connect(self.hand.hand_id,'PLAYER_SELF','CLAIM_CONTROL');}
      else{r.fetcher.mockResolvedValueOnce(ok({access_token:'synthetic changed',access_expires_at:4102444800,user:{id:mode==='account'?910003:910002,username:'synthetic',role:1},session:{sid:'synthetic changed'}}));await r.api.login('synthetic','synthetic');}
      reply(ok({private_secret:'stale auxiliary result'}));await read;expect(r.client.getSnapshot()?.control_recovery).toBeUndefined();expect(r.client.getSnapshot()?.pending).toBeUndefined();expect(JSON.stringify(r.client)).toBe('{}');}
  });
  it('a query-resolved old HTTP attempt is ignored before decode even when another auxiliary starts in the same scope',async()=>{
    const r=await setup(false);let oldReply!:(v:Response)=>void;r.fetcher.mockReturnValueOnce(new Promise<Response>(resolve=>oldReply=resolve));
    const old=r.client.submit({type:'takeover'},context(r)),key=auxiliaryKey(r);r.fetcher.mockResolvedValueOnce(ok({...lookup(r,'takeover',true),receipt:receipt('7')}));await r.client.recheckTakeover();
    expect(r.client.getSnapshot()?.control_recovery).toBeUndefined();let newReply!:(v:Response)=>void;r.fetcher.mockReturnValueOnce(new Promise<Response>(resolve=>newReply=resolve));
    const next=r.client.submit({type:'takeover'},context(r));expect(auxiliaryKey(r)).not.toBe(key);oldReply(ok({private_secret:'old HTTP attempt'}));await expect(old).resolves.toBeUndefined();
    expect(r.client.getSnapshot()?.control_recovery?.phase).toBe('SENT');newReply(ok(receipt('7')));await next;expect(r.client.getSnapshot()?.control_recovery).toBeUndefined();
  });
});
describe('R2 recovery controls preserve all ordinary mutation locks',()=>{
  it('keeps readonly queries automatic and exposes only eligible same-key retry inside folded status details',async()=>{
    const r=await auxiliaryUnknown();r.fetcher.mockResolvedValueOnce(ok(lookup(r)));await r.client.recheckTakeover();const p=props(r),query=vi.fn(),retry=vi.fn(),primary=vi.fn();
    const view=render(<PokerTable {...p} onQueryReceipt={primary} onQueryTakeover={query} onRetryTakeover={retry} onRetryPending={vi.fn()}/>);
    expect(screen.getByRole('status')).toHaveTextContent('正在自动确认操作结果');expect(screen.queryByRole('button',{name:'核对接管回执'})).not.toBeInTheDocument();expect(screen.queryByRole('button',{name:'核对原回执'})).not.toBeInTheDocument();expect(screen.queryByRole('button',{name:'重试本次接管'})).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button',{name:'查看状态'}));fireEvent.click(screen.getByRole('button',{name:'重试本次接管'}));expect(query).not.toHaveBeenCalled();expect(primary).not.toHaveBeenCalled();expect(retry).toHaveBeenCalledWith(context(r));expect(screen.queryByRole('button',{name:'重试原操作'})).not.toBeInTheDocument();expect(p.onIntent).not.toHaveBeenCalled();
    view.rerender(<PokerTable {...p} authority={{...p.authority,control_recovery:{...p.authority.control_recovery!,querying:true,can_retry:false}}} onQueryReceipt={primary} onQueryTakeover={query} onRetryTakeover={retry}/>);
    expect(screen.queryByRole('button',{name:'核对接管回执'})).not.toBeInTheDocument();expect(screen.queryByRole('button',{name:'重试本次接管'})).not.toBeInTheDocument();expect(screen.queryByRole('button',{name:'核对原回执'})).not.toBeInTheDocument();expect(query).not.toHaveBeenCalled();expect(primary).not.toHaveBeenCalled();
  });
});

describe('R3 previous-connection query-only locators',()=>{
  it('archives the old target, creates a new key only on explicit confirmation, and isolates a late historical FOUND',async()=>{
    const r=await auxiliaryUnknown(),original={...r.client.getSnapshot()!.pending},old=auxiliaryKey(r);
    await reconnect(r);expect(r.client.getSnapshot()?.control_recovery).toBeUndefined();
    expect(r.client.getSnapshot()?.control_history).toEqual({count:1,querying:false});expect(posts(r,'/take-over')).toHaveLength(1);
    await expect(r.client.retryTakeover(context(r))).rejects.toThrow();let reply!:(v:Response)=>void;
    r.fetcher.mockReturnValueOnce(new Promise<Response>(resolve=>reply=resolve));const historical=r.client.recheckTakeover(true);await Promise.resolve();await Promise.resolve();
    expect(JSON.parse(posts(r,'/receipt-query').at(-1)![1].body)).toEqual({kind:'takeover',mutation_id:old});
    r.fetcher.mockRejectedValueOnce(Error('new target ACK lost'));await expect(r.client.submit({type:'takeover'},context(r))).rejects.toThrow();
    const current={...r.client.getSnapshot()!.control_recovery},latest=JSON.parse(posts(r,'/take-over').at(-1)![1].body);
    expect(latest).toMatchObject({connection_id:r.sockets.at(-1)!.id});expect(latest.request_id).not.toBe(old);
    reply(ok({table_id:self.table_id,kind:'takeover',mutation_id:old,state:'FOUND',receipt:receipt()}));await historical;
    expect(r.client.getSnapshot()?.control_history).toBeUndefined();expect(r.client.getSnapshot()?.control_recovery).toEqual(current);
    expect(r.client.getSnapshot()?.pending).toEqual(original);expect(pokerStreamAuthority(r.client.getSnapshot()!).can_control).toBe(false);
    expect(posts(r,'/take-over').map(([,o])=>JSON.parse(o.body).request_id)).toEqual([old,latest.request_id]);
    expect(r.sockets.flatMap(s=>s.sent).filter(t=>JSON.parse(t).type==='hand.action')).toHaveLength(1);
  });
  it('retains multiple NOT_FOUND locators and coalesces explicit historical reads without replay or current-lane unlock',async()=>{
    const r=await auxiliaryUnknown(),first=auxiliaryKey(r);await reconnect(r);
    expect(r.client.getSnapshot()?.control_history?.count).toBe(1);
    r.fetcher.mockRejectedValueOnce(Error('second target ACK lost'));await expect(r.client.submit({type:'takeover'},context(r))).rejects.toThrow();
    const second=auxiliaryKey(r);await reconnect(r);expect(r.client.getSnapshot()?.control_history?.count).toBe(2);
    const replies:Array<(v:Response)=>void>=[];for(let i=0;i<2;i++)r.fetcher.mockReturnValueOnce(new Promise<Response>(resolve=>replies.push(resolve)));
    const before=posts(r,'/receipt-query').length,a=r.client.recheckTakeover(true),b=r.client.recheckTakeover(true);expect(a).toBe(b);await Promise.resolve();await Promise.resolve();
    expect(posts(r,'/receipt-query').slice(before).map(([,o])=>JSON.parse(o.body).mutation_id)).toEqual([first]);
    replies[0](ok({table_id:self.table_id,kind:'takeover',mutation_id:first,state:'NOT_FOUND'}));await vi.waitFor(()=>expect(posts(r,'/receipt-query')).toHaveLength(before+2));
    replies[1](ok({table_id:self.table_id,kind:'takeover',mutation_id:second,state:'NOT_FOUND'}));await a;
    expect(r.client.getSnapshot()?.control_history).toEqual({count:2,querying:false});expect(r.client.getSnapshot()?.pending?.phase).toBe('UNKNOWN');
    expect(posts(r,'/take-over')).toHaveLength(2);expect(r.client.getSnapshot()?.can_recover_control).toBe(true);
    r.fetcher.mockRejectedValueOnce(Error('history query failed')).mockRejectedValueOnce(Error('history query failed'));await r.client.recheckTakeover(true);expect(r.client.getSnapshot()?.control_history).toEqual({count:2,querying:false});
    const original=r.client.getSnapshot()!.pending;full(r.sockets.at(-1)!,10,true,'2');expect(r.client.getSnapshot()?.can_retry_pending).toBe(true);await r.client.retryPending(context(r));
    expect(JSON.parse(r.sockets.at(-1)!.sent.at(-1)!)).toMatchObject({type:'hand.action',request_id:original!.request_id,action_id:original!.action_id,expected_table_version:10,control_epoch:2});
  });
  it('a newer connection query supersedes a late old-generation result before its private payload is decoded',async()=>{
    const r=await auxiliaryUnknown(),key=auxiliaryKey(r);await reconnect(r);expect(r.client.getSnapshot()?.control_history?.count).toBe(1);let reply!:(v:Response)=>void;
    r.fetcher.mockReturnValueOnce(new Promise<Response>(resolve=>reply=resolve));const stale=r.client.recheckTakeover(true);await Promise.resolve();await Promise.resolve();
    await reconnect(r);r.fetcher.mockResolvedValueOnce(ok({table_id:self.table_id,kind:'takeover',mutation_id:key,state:'NOT_FOUND'}));await r.client.recheckTakeover(true);
    const current=r.client.getSnapshot();reply(ok({private_secret:'superseded historical read'}));await stale;expect(r.client.getSnapshot()).toEqual(current);
    expect(r.client.getSnapshot()?.control_history).toEqual({count:1,querying:false});expect(posts(r,'/take-over')).toHaveLength(1);
  });
  it('a late old-target HTTP send is fenced before decode after a new explicit auxiliary starts',async()=>{
    const r=await unknown();let reply!:(v:Response)=>void;r.fetcher.mockReturnValueOnce(new Promise<Response>(resolve=>reply=resolve));
    const old=r.client.submit({type:'takeover'},context(r)),key=auxiliaryKey(r);await reconnect(r);expect(r.client.getSnapshot()?.control_history?.count).toBe(1);
    r.fetcher.mockRejectedValueOnce(Error('new target ACK lost'));await expect(r.client.submit({type:'takeover'},context(r))).rejects.toThrow();
    const current=r.client.getSnapshot();reply(ok({private_secret:'old target send'}));await expect(old).resolves.toBeUndefined();expect(r.client.getSnapshot()).toBe(current);
    expect(auxiliaryKey(r)).not.toBe(key);expect(posts(r,'/take-over')).toHaveLength(2);expect(r.client.getSnapshot()?.pending?.phase).toBe('UNKNOWN');
  });
  it('clears historical locators and drops late invalid reads on every owner boundary',async()=>{
    for(const mode of ['logout','dispose','account','session','table','viewer']){
      const r=await auxiliaryUnknown();await reconnect(r);expect(r.client.getSnapshot()?.control_history?.count).toBe(1);let reply!:(v:Response)=>void;
      r.fetcher.mockReturnValueOnce(new Promise<Response>(resolve=>reply=resolve));const read=r.client.recheckTakeover(true);await Promise.resolve();await Promise.resolve();
      if(mode==='logout'){r.fetcher.mockResolvedValueOnce(ok({}));await r.api.logout();}else if(mode==='dispose')r.client.dispose();
      else if(mode==='table'||mode==='viewer'){r.fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));await r.client.connect(mode==='table'?self.hand.hand_id:self.table_id,mode==='viewer'?'SPECTATOR':'PLAYER_SELF','READ_ONLY');}
      else{r.fetcher.mockResolvedValueOnce(ok({access_token:'changed',access_expires_at:4102444800,user:{id:mode==='account'?910003:910002,username:'changed',role:1},session:{sid:'changed'}}));await r.api.login('changed','changed');}
      reply(ok({private_secret:'late historical result'}));await read;const calls=r.fetcher.mock.calls.length;await r.client.recheckTakeover(true);
      expect(r.client.getSnapshot()?.control_history).toBeUndefined();expect(r.client.getSnapshot()?.pending).toBeUndefined();expect(r.fetcher.mock.calls).toHaveLength(calls);
      expect(JSON.stringify(r.client)).toBe('{}');expect(localStorage.length+sessionStorage.length).toBe(0);
    }
  });
});
describe('R3 stopped transport remains explicitly recoverable',()=>{
  it('history-only controller banner stays neutral and preserves the legal action',async()=>{
    const r=await setup(),p=props(r);expect(p.authority).toMatchObject({pending:false,can_control:true});expect(p.authority.control_recovery).toBeUndefined();
    render(<PokerTable {...p} authority={{...p.authority,control_history:{count:1,querying:false}}}/>);
    expect(screen.getByRole('status')).toHaveTextContent('正在自动确认操作结果');expect(screen.getByRole('status')).not.toHaveTextContent('当前只读');expect(screen.queryByText('还有 1 次旧连接接管结果自动确认中。')).not.toBeInTheDocument();fireEvent.click(screen.getByRole('button',{name:'查看状态'}));expect(screen.getByRole('dialog',{name:'连接与操作状态'})).toHaveTextContent('还有 1 次旧连接接管结果自动确认中。');
    fireEvent.click(screen.getByRole('button',{name:'关闭对话框'}));
    const call=screen.getByRole('button',{name:/^跟注 /});expect(call).not.toBeDisabled();fireEvent.click(call);
    expect(p.onIntent).toHaveBeenCalledWith({type:'action',action_type:'CALL',target_to_units:'0'},context(r));
  });
  it('the DEGRADED auto-stopped UI reconnects with both UNKNOWN locks and sends no takeover or business retry',async()=>{
    vi.useFakeTimers();try{
      const r=await auxiliaryUnknown(),original={...r.client.getSnapshot()!.pending};r.sockets.at(-1)!.message('invalid');
      expect(r.client.getSnapshot()?.connection_state).toBe('DEGRADED');await vi.advanceTimersByTimeAsync(60000);expect(r.sockets).toHaveLength(2);
      const p=props(r);let connecting:Promise<void>|undefined;
      p.onIntent.mockImplementation(intent=>{if(intent.type==='reconnect')connecting=r.client.connect(self.table_id,'PLAYER_SELF',intent.control_intent??'CLAIM_CONTROL');});
      render(<PokerTable {...p} onQueryReceipt={vi.fn()} onQueryTakeover={vi.fn()}/>);expect(screen.getByRole('button',{name:'重新连接'})).not.toBeDisabled();
      r.fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));fireEvent.click(screen.getByRole('button',{name:'重新连接'}));await connecting;
      expect(r.sockets).toHaveLength(3);expect(r.client.getSnapshot()?.pending).toEqual(original);
      r.fetcher.mockResolvedValueOnce(ok(lookup(r,'action')));authenticated(r.sockets.at(-1)!,9);await r.client.recheckPending();
      expect(posts(r,'/take-over')).toHaveLength(1);expect(pokerStreamAuthority(r.client.getSnapshot()!).can_control).toBe(false);
      expect(r.sockets.flatMap(s=>s.sent).filter(t=>JSON.parse(t).type==='hand.action')).toHaveLength(1);
    }finally{vi.useRealTimers();}
  });
  it('keeps old reads automatic while READ_ONLY requests a fresh CLAIM_CONTROL before a separate takeover confirmation',async()=>{
    const r=await auxiliaryUnknown();await reconnect(r,'READ_ONLY');const p=props(r),query=vi.fn();
    const view=render(<PokerTable {...p} onQueryPreviousTakeovers={query}/>);expect(screen.getByRole('status')).toHaveTextContent('正在自动确认操作结果');expect(screen.queryByRole('button',{name:'核对上次接管回执'})).not.toBeInTheDocument();fireEvent.click(screen.getByRole('button',{name:'查看状态'}));expect(screen.getByRole('dialog',{name:'连接与操作状态'})).toHaveTextContent('还有 1 次旧连接接管结果自动确认中。');expect(query).not.toHaveBeenCalled();fireEvent.click(screen.getByRole('button',{name:'关闭对话框'}));
    fireEvent.click(screen.getByRole('button',{name:'申请控制连接'}));expect(p.onIntent).toHaveBeenCalledWith({type:'reconnect',control_intent:'CLAIM_CONTROL'},context(r));
    await reconnect(r);expect(posts(r,'/take-over')).toHaveLength(1);p.onIntent.mockClear();
    view.rerender(<PokerTable {...props(r)} onIntent={p.onIntent} onUiChange={p.onUiChange} onQueryPreviousTakeovers={query}/>);
    fireEvent.click(screen.getByRole('button',{name:'申请接管'}));expect(p.onIntent).not.toHaveBeenCalled();expect(p.onUiChange).toHaveBeenCalledWith(expect.objectContaining({confirm_takeover:true}));
    const updated=props(r);view.rerender(<PokerTable {...updated} ui={{...updated.ui,confirm_takeover:true}} onIntent={p.onIntent} onQueryPreviousTakeovers={query}/>);
    fireEvent.click(screen.getByRole('button',{name:'确认接管牌桌'}));expect(p.onIntent).toHaveBeenCalledWith({type:'takeover'},context(r));
  });
});
