import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { ApiClient } from '../api';
import { PokerTable } from './PokerTable';
import { PokerTableClient } from './poker-socket';
import { capturePokerReadScope, readPokerHttpReceipt } from './poker-http';
import { acknowledgePokerPending, beginPokerPending, disconnectPokerStream, openPokerStream, pokerStreamAuthority, pokerStreamContext, receivePokerStream } from './poker-stream';
import { parsePokerReceipt, parsePokerReceiptLookup, pokerReceiptQuery, POKER_SUBPROTOCOL, type PokerPendingLocator, type PokerReceiptKind } from './poker-wire';
import self from './fixtures/g3-player-self.json';
import takeover from './fixtures/g3-receipt-query-takeover.json';

const request='original-request-001',action='original-action-001',connection='a'.repeat(32),ticket='ct1.eyJzeW50aGV0aWMiOnRydWV9.c2lnbmF0dXJl';
const locator:PokerPendingLocator={kind:'action',request_id:request,action_id:action,target_session_id:self.viewer.session_id,target_hand_id:self.hand.hand_id};
const receipt={table_id:self.table_id,hand_id:self.hand.hand_id,status:'PREFLOP',table_version:'8',duplicate:false};
const missing=(p:PokerPendingLocator=locator)=>({table_id:self.table_id,kind:p.kind,mutation_id:p.action_id??p.request_id,state:'NOT_FOUND'});
const found=(p:PokerPendingLocator=locator,r:unknown=receipt)=>({...missing(p),state:'FOUND',receipt:r});
const ok=(data:unknown)=>new Response(JSON.stringify({success:true,data}),{headers:{'Content-Type':'application/json'}});
const active:PokerTableClient[]=[];afterEach(()=>{active.splice(0).forEach(c=>c.dispose());vi.restoreAllMocks();vi.useRealTimers();});
async function auth(){const fetcher=vi.fn().mockResolvedValueOnce(ok({access_token:'synthetic',access_expires_at:4102444800,user:{id:910002,username:'synthetic',role:1},session:{sid:'synthetic'}})),api=new ApiClient(fetcher);await api.login('synthetic','synthetic');return{api,fetcher};}
class Socket extends EventTarget {
  readyState=0;bufferedAmount=0;protocol=POKER_SUBPROTOCOL;sent:string[]=[];
  send(text:string){this.sent.push(text);}close(){this.readyState=3;this.dispatchEvent(new CloseEvent('close',{code:1000}));}
  open(){this.readyState=1;this.dispatchEvent(new Event('open'));}message(data:string){this.dispatchEvent(new MessageEvent('message',{data}));}
}
function frame(type:string,payload:any,version=7){return JSON.stringify({type,event_id:'query-synthetic-envelope',event_seq:10,table_id:self.table_id,table_version:version,hand_id:payload?.hand?.hand_id??self.hand.hand_id,hand_version:8,server_time:self.server_now,payload});}
function snapshot(conn=connection,version=7,readonly=false,newSession?:string,newHand?:string){const v:any=structuredClone(self);v.table_version=String(version);v.viewer.session_id=newSession??v.viewer.session_id;v.hand.hand_id=newHand??v.hand.hand_id;v.timeline.hand_id=v.hand.hand_id;v.viewer.control={connection_id:conn,session_id:v.viewer.session_id,mode:readonly?'READ_ONLY':'CONTROLLER',control_epoch:'1'};if(readonly){v.viewer.can_act=false;delete v.viewer.legal;}return frame('table.snapshot',v,version);}
function authenticated(s:Socket,conn=connection,version=7,readonly=false,newSession?:string,newHand?:string){s.open();s.message(frame('auth.accepted',{request_id:JSON.parse(s.sent[0]).request_id,connection_id:conn}));s.message(snapshot(conn,version,readonly,newSession,newHand));}
async function setup(){const r=await auth(),sockets:Socket[]=[],client=new PokerTableClient(r.api,{socketFactory:()=>{const s=new Socket();sockets.push(s);return s as unknown as WebSocket;}});active.push(client);r.fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));await client.connect(self.table_id,'PLAYER_SELF','CLAIM_CONTROL');authenticated(sockets[0]);return{...r,sockets,client};}
async function pending(){const r=await setup();await r.client.submit({type:'action',action_type:'CALL',target_to_units:'0'},pokerStreamContext(r.client.getSnapshot()!));r.client.disconnect();return r;}

describe('receipt query parser and authenticated read-only HTTP',()=>{
  it('accepts the unchanged actual G3 PG/Redis TakeOver receipt as metadata, never as a socket grant',()=>{
    expect(parsePokerReceipt(takeover,takeover.table_id)).toEqual(takeover);for(const control_epoch of [3,'03','-1','18446744073709551616',null])expect(()=>parsePokerReceipt({...takeover,control_epoch},takeover.table_id)).toThrow();
  });
  it('strictly binds discriminant, kind, original mutation identity and original target without inventing no-effect',()=>{
    expect(parsePokerReceiptLookup(found(),self.table_id,locator)).toEqual(found());expect(parsePokerReceiptLookup(missing(),self.table_id,locator)).toEqual(missing());
    for(const value of [{...missing(),receipt},{...missing(),receipt:null},{...missing(),receipt:undefined},{...found(),receipt:undefined},{...found(),state:'FAILED_NO_EFFECT'},{...found(),kind:'topup'},{...found(),mutation_id:'another-action-0001'},{...found(),extra:true},{...found(),receipt:{...receipt,hand_id:self.table_id}},{...found(),table_id:self.hand.hand_id}])expect(()=>parsePokerReceiptLookup(value,self.table_id,locator)).toThrow();
    const topup:PokerPendingLocator={kind:'topup',request_id:request,action_id:null,target_session_id:self.viewer.session_id};expect(parsePokerReceiptLookup(found(topup,{...receipt,session_id:self.viewer.session_id}),self.table_id,topup).state).toBe('FOUND');expect(()=>parsePokerReceiptLookup(found(topup,receipt),self.table_id,topup)).toThrow();
  });
  it('maps all seven closed kinds to the original ID class and rejects target/ID class confusion',()=>{
    for(const kind of ['action','sitout','resume','nextseed','topup','leave','takeover'] as PokerReceiptKind[]){
      const wire=['action','sitout','resume','nextseed'].includes(kind),p:PokerPendingLocator={kind,request_id:request,action_id:wire?action:null,target_session_id:locator.target_session_id,...(kind==='action'?{target_hand_id:locator.target_hand_id}:{})};expect(pokerReceiptQuery(p)).toEqual({kind,mutation_id:wire?action:request});expect(()=>pokerReceiptQuery({...p,action_id:wire?null:action})).toThrow();
    }
    for(const p of [{...locator,target_hand_id:undefined},{...locator,target_session_id:null},{...locator,request_id:'short'},{...locator,kind:'topup',action_id:null}])expect(()=>pokerReceiptQuery(p as PokerPendingLocator)).toThrow();
  });
  it('posts only kind and original action/request ID; invalid locators and stale reads stop before private decode',async()=>{
    const r=await auth(),scope=capturePokerReadScope(r.api,self.table_id,'PLAYER_SELF',1);let current=scope;r.fetcher.mockResolvedValueOnce(ok(found()));expect(await readPokerHttpReceipt(r.api,scope,()=>current,locator)).toEqual(found());expect(r.fetcher.mock.calls[1][0]).toBe(`/api/v1/poker/tables/${self.table_id}/receipt-query`);expect(JSON.parse(r.fetcher.mock.calls[1][1].body)).toEqual({kind:'action',mutation_id:action});
    await expect(readPokerHttpReceipt(r.api,scope,()=>current,{...locator,kind:'create'} as never)).rejects.toThrow();expect(r.fetcher).toHaveBeenCalledTimes(2);
    let reply!:(v:Response)=>void;r.fetcher.mockReturnValueOnce(new Promise<Response>(resolve=>reply=resolve));const read=readPokerHttpReceipt(r.api,scope,()=>current,locator);current={...scope,request_generation:2};reply(ok({private_secret:'invalid stale body'}));expect(await read).toBeUndefined();
  });
  it('drops each changed identity boundary before issuing a receipt HTTP read',async()=>{
    const r=await auth(),scope=capturePokerReadScope(r.api,self.table_id,'PLAYER_SELF',1);for(const patch of [{user_id:'910003'},{session_generation:scope.session_generation+1},{request_generation:2},{table_id:self.hand.hand_id},{viewer_kind:'SPECTATOR' as const}])await expect(readPokerHttpReceipt(r.api,scope,()=>({...scope,...patch}),locator)).rejects.toThrow();expect(r.fetcher).toHaveBeenCalledTimes(1);
  });
});

describe('original pending locator and Receipt/full-snapshot convergence',()=>{
  it('captures immutable original hand/session/kind and keeps UNKNOWN until a correlated receipt and enough snapshot version',async()=>{
    const r=await setup(),before=r.client.getSnapshot()!,context=pokerStreamContext(before);let s=beginPokerPending(before,context,request,action,'action');expect(s.pending).toMatchObject(locator);s=disconnectPokerStream(s);
    const nextScope={...s.scope,request_generation:2};s=openPokerStream(nextScope,'new-auth-request-001','CLAIM_CONTROL',s);s=receivePokerStream(s,nextScope,frame('auth.accepted',{request_id:s.auth_request_id,connection_id:connection}));s=receivePokerStream(s,nextScope,snapshot(connection,9,false,undefined,self.table_id));expect(s.pending?.phase).toBe('UNKNOWN');const table=s.table;s=acknowledgePokerPending(s,request,action,receipt);expect(s.pending).toBeUndefined();expect(s.table).toBe(table);
    let early=beginPokerPending(before,context,request,action,'action');early=acknowledgePokerPending(early,request,action,receipt);expect(early.pending?.phase).toBe('ACKNOWLEDGED');expect(early.table?.table_version).toBe('7');early=receivePokerStream(early,early.scope,snapshot(connection,8));expect(early.pending).toBeUndefined();
  });
});

describe('query runtime is a scoped read, never business replay',()=>{
  it('queries once at reconnect first LIVE, coalesces manual clicks and allows later explicit readonly recheck',async()=>{
    const r=await pending();r.fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));await r.client.connect(self.table_id,'PLAYER_SELF','READ_ONLY');const p=r.client.getSnapshot()!.pending!;r.fetcher.mockResolvedValueOnce(ok(missing(p)));authenticated(r.sockets[1],'b'.repeat(32),9,true);await r.client.recheckPending();expect(r.fetcher).toHaveBeenCalledTimes(4);expect(r.client.getSnapshot()?.pending?.phase).toBe('UNKNOWN');r.sockets[1].message(snapshot('b'.repeat(32),9,true));await Promise.resolve();expect(r.fetcher).toHaveBeenCalledTimes(4);
    let reply!:(v:Response)=>void;r.fetcher.mockReturnValueOnce(new Promise<Response>(resolve=>reply=resolve));const a=r.client.recheckPending(),b=r.client.recheckPending();expect(a).toBe(b);await Promise.resolve();expect(r.client.getSnapshot()?.receipt_querying).toBe(true);expect(r.fetcher).toHaveBeenCalledTimes(5);reply(ok(found(p)));await a;expect(r.client.getSnapshot()?.pending).toBeUndefined();expect(r.client.getSnapshot()?.receipt_querying).toBe(false);expect(pokerStreamAuthority(r.client.getSnapshot()!).can_control).toBe(false);expect(r.sockets[1].sent.map(x=>JSON.parse(x).type)).toEqual(['auth.connect']);
  });
  it('manual disconnected HTTP query needs no controller; newer receipt waits for readonly sync/full snapshot and errors retain UNKNOWN',async()=>{
    const r=await pending(),p=r.client.getSnapshot()!.pending!;r.fetcher.mockRejectedValueOnce(Error('synthetic-private-error'));await r.client.recheckPending();expect(r.client.getSnapshot()?.pending?.phase).toBe('UNKNOWN');expect(JSON.stringify(r.client.getSnapshot())).not.toContain('synthetic-private-error');r.fetcher.mockResolvedValueOnce(ok(found(p)));await r.client.recheckPending();expect(r.client.getSnapshot()?.pending?.phase).toBe('ACKNOWLEDGED');expect(r.client.getSnapshot()?.table?.table_version).toBe('7');expect(r.sockets[0].sent.map(x=>JSON.parse(x).type)).toEqual(['auth.connect','hand.action']);
    r.fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));await r.client.connect(self.table_id,'PLAYER_SELF','READ_ONLY');authenticated(r.sockets[1],'b'.repeat(32),8,true);expect(r.client.getSnapshot()?.pending).toBeUndefined();expect(r.fetcher).toHaveBeenCalledTimes(5);
  });
  it('a LIVE found receipt ahead of the view requests only sync and waits for the authoritative full version',async()=>{
    const r=await pending(),p=r.client.getSnapshot()!.pending!;r.fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));await r.client.connect(self.table_id,'PLAYER_SELF','READ_ONLY');r.fetcher.mockResolvedValueOnce(ok(found(p)));authenticated(r.sockets[1],'b'.repeat(32),7,true);await r.client.recheckPending();expect(r.client.getSnapshot()?.connection_state).toBe('SYNCING');expect(r.client.getSnapshot()?.pending?.phase).toBe('ACKNOWLEDGED');expect(r.client.getSnapshot()?.table?.table_version).toBe('7');expect(r.sockets[1].sent.map(x=>JSON.parse(x).type)).toEqual(['auth.connect','sync.request']);await r.client.recheckPending();expect(r.fetcher).toHaveBeenCalledTimes(4);r.sockets[1].message(snapshot('b'.repeat(32),8,true));expect(r.client.getSnapshot()?.pending).toBeUndefined();expect(pokerStreamAuthority(r.client.getSnapshot()!).can_control).toBe(false);
  });
  it('a query button publication followed by reentrant logout clears private state before the deferred HTTP send',async()=>{
    const r=await pending();r.fetcher.mockResolvedValueOnce(ok({}));let logout:Promise<void>|undefined;r.client.subscribe(()=>{if(r.client.getSnapshot()?.receipt_querying)logout=r.api.logout();});await r.client.recheckPending();await logout;expect(r.client.getSnapshot()).toBeNull();expect(r.fetcher.mock.calls.map(([path])=>path).filter((path:string)=>path.endsWith('/receipt-query'))).toEqual([]);
  });
  it('an in-flight lookup cannot attach its late private response to a later explicit action locator',async()=>{
    const r=await setup();await r.client.submit({type:'action',action_type:'CALL',target_to_units:'0'},pokerStreamContext(r.client.getSnapshot()!));const old=r.client.getSnapshot()!.pending!;let reply!:(v:Response)=>void;r.fetcher.mockReturnValueOnce(new Promise<Response>(resolve=>reply=resolve));const query=r.client.recheckPending();await Promise.resolve();r.sockets[0].message(frame('service.notice',{request_id:old.request_id,action_id:old.action_id,receipt:{...receipt,table_version:'7'}}));expect(r.client.getSnapshot()?.pending).toBeUndefined();await r.client.submit({type:'action',action_type:'CALL',target_to_units:'0'},pokerStreamContext(r.client.getSnapshot()!));const next=r.client.getSnapshot()!.pending!;reply(ok({private_secret:'late wrong locator'}));await query;expect(r.client.getSnapshot()?.pending).toEqual(next);expect(next.action_id).not.toBe(old.action_id);expect(r.client.getSnapshot()?.last_error).toBeUndefined();expect(r.sockets[0].sent.map(x=>JSON.parse(x).type)).toEqual(['auth.connect','hand.action','hand.action']);
  });
  it('old locator/scope replies and disposal never mutate new private state or trigger any other operation',async()=>{
    const r=await pending();let reply!:(v:Response)=>void;r.fetcher.mockReturnValueOnce(new Promise<Response>(resolve=>reply=resolve));const read=r.client.recheckPending();await Promise.resolve();r.fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));await r.client.connect(self.table_id,'PLAYER_SELF','READ_ONLY');reply(ok({private_secret:'stale invalid query result'}));await read;expect(r.client.getSnapshot()?.pending?.phase).toBe('UNKNOWN');expect(r.client.getSnapshot()?.receipt_querying).not.toBe(true);
    let last!:(v:Response)=>void;r.fetcher.mockReturnValueOnce(new Promise<Response>(resolve=>last=resolve));const disposed=r.client.recheckPending();await Promise.resolve();r.client.dispose();last(ok({private_secret:'stale after dispose'}));await disposed;expect(r.client.getSnapshot()).toBeNull();expect(r.sockets.flatMap(s=>s.sent).filter(x=>JSON.parse(x).type==='hand.action')).toHaveLength(1);
  });
  it('original HTTP Session A stays the receipt target after the same user seats in Session B',async()=>{
    const r=await setup();const v=JSON.parse(snapshot()).payload;v.viewer.can_top_up=true;v.viewer.top_up_min_units='500000';v.viewer.top_up_max_units='5000000';r.sockets[0].message(frame('table.snapshot',v));r.fetcher.mockRejectedValueOnce(Error('lost'));await expect(r.client.submit({type:'topup',amount_units:'500000'},pokerStreamContext(r.client.getSnapshot()!))).rejects.toThrow();const p=r.client.getSnapshot()!.pending!;expect(p.target_session_id).toBe(self.viewer.session_id);r.client.disconnect();r.fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));await r.client.connect(self.table_id,'PLAYER_SELF','READ_ONLY');r.fetcher.mockResolvedValueOnce(ok(found(p,{...receipt,session_id:self.viewer.session_id,status:'PENDING'})));authenticated(r.sockets[1],'b'.repeat(32),9,true,self.table_id);await r.client.recheckPending();expect(r.client.getSnapshot()?.pending).toBeUndefined();expect(r.client.getSnapshot()?.table?.viewer.session_id).toBe(self.table_id);expect(r.sockets[1].sent.map(x=>JSON.parse(x).type)).toEqual(['auth.connect']);
  });
});

describe('pending receipt query presentation',()=>{
  it('offers only the explicit readonly callback while all new mutations remain disabled; querying coalesces clicks',async()=>{
    const r=await setup(),s=r.client.getSnapshot()!,query=vi.fn(),mutation=vi.fn();const props={table:s.table!,authority:{...pokerStreamAuthority(s),pending:true},ui:{bet:{action_type:'RAISE' as const,amount_chips:'1'},top_up_chips:'1',confirm_leave:false,confirm_takeover:false},onUiChange:vi.fn(),onIntent:mutation,onQueryReceipt:query,receipt_querying:false};const view=render(<PokerTable {...props}/>);expect(screen.queryByRole('button',{name:'核对原回执'})).toBeInTheDocument();fireEvent.click(screen.getByRole('button',{name:'核对原回执'}));expect(query).toHaveBeenCalledTimes(1);for(const b of screen.getAllByRole('button'))if(b.textContent?.includes('跟注')){expect(b).toBeDisabled();fireEvent.click(b);}expect(mutation).not.toHaveBeenCalled();view.rerender(<PokerTable {...props} receipt_querying/>);expect(screen.getByRole('button',{name:'核对原回执'})).toBeDisabled();fireEvent.click(screen.getByRole('button',{name:'核对原回执'}));expect(query).toHaveBeenCalledTimes(1);view.rerender(<PokerTable {...props} onQueryReceipt={undefined}/>);expect(screen.queryByRole('button',{name:'核对原回执'})).not.toBeInTheDocument();
  });
});
