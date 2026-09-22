import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { ApiClient } from '../api';
import { PokerTable } from './PokerTable';
import { PokerTableClient } from './poker-socket';
import { beginPokerPending, disconnectPokerStream, pokerStreamAuthority, pokerStreamContext } from './poker-stream';
import { POKER_SUBPROTOCOL } from './poker-wire';
import type { TableIntent } from './poker-ui-types';
import self from './fixtures/g3-player-self.json';

const ticket='ct1.eyJzeW50aGV0aWMiOnRydWV9.c2lnbmF0dXJl',ok=(data:unknown)=>new Response(JSON.stringify({success:true,data}),{headers:{'Content-Type':'application/json'}});
const clients:PokerTableClient[]=[];afterEach(()=>{clients.splice(0).forEach(c=>c.dispose());vi.restoreAllMocks();vi.useRealTimers();});
class Socket extends EventTarget {
  readyState=0;bufferedAmount=0;protocol=POKER_SUBPROTOCOL;sent:string[]=[];constructor(readonly id:string){super();}
  send(text:string){this.sent.push(text);}close(){this.readyState=3;this.dispatchEvent(new CloseEvent('close',{code:1000}));}
  open(){this.readyState=1;this.dispatchEvent(new Event('open'));}message(data:string){this.dispatchEvent(new MessageEvent('message',{data}));}
}
function frame(type:string,payload:any,version=7){return JSON.stringify({type,event_id:'retry-synthetic-envelope',event_seq:10,table_id:self.table_id,table_version:version,hand_id:payload?.hand?.hand_id??self.hand.hand_id,hand_version:version+1,server_time:self.server_now,payload});}
function snapshot(s:Socket,version=7,patch:{viewer?:any;hand?:any;readonly?:boolean}={}){const v:any=structuredClone(self);v.table_version=String(version);v.hand={...v.hand,hand_version:String(version+1),...patch.hand};v.timeline.hand_id=v.hand.hand_id;v.viewer={...v.viewer,can_resume:true,control_epoch:String(version-6),...patch.viewer};v.viewer.control={connection_id:s.id,session_id:v.viewer.session_id,mode:patch.readonly?'READ_ONLY':'CONTROLLER',control_epoch:v.viewer.control_epoch};if(patch.readonly){v.viewer.can_act=false;delete v.viewer.legal;}return frame('table.snapshot',v,version);}
function authenticated(s:Socket,version=7,patch:Parameters<typeof snapshot>[2]={}){s.open();s.message(frame('auth.accepted',{request_id:JSON.parse(s.sent[0]).request_id,connection_id:s.id}));s.message(snapshot(s,version,patch));}
async function setup(readonly=false){const fetcher=vi.fn().mockResolvedValueOnce(ok({access_token:'synthetic',access_expires_at:4102444800,user:{id:910002,username:'synthetic',role:1},session:{sid:'synthetic'}})),api=new ApiClient(fetcher);await api.login('synthetic','synthetic');const sockets:Socket[]=[],client=new PokerTableClient(api,{socketFactory:()=>{const s=new Socket(String(sockets.length+1).repeat(32));sockets.push(s);return s as unknown as WebSocket;}});clients.push(client);fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));await client.connect(self.table_id,'PLAYER_SELF',readonly?'READ_ONLY':'CLAIM_CONTROL');authenticated(sockets[0],7,{readonly});return{api,fetcher,sockets,client};}
type Runtime=Awaited<ReturnType<typeof setup>>;
const context=(r:Runtime)=>pokerStreamContext(r.client.getSnapshot()!),actions=(r:Runtime)=>r.sockets.flatMap(s=>s.sent.map(t=>JSON.parse(t))).filter(f=>['hand.action','session.sit_out_next_hand','session.resume_play'].includes(f.type));
const missing=(r:Runtime)=>{const p=r.client.getSnapshot()!.pending!;return{table_id:self.table_id,kind:p.kind,mutation_id:p.action_id??p.request_id,state:'NOT_FOUND'};};
const receipt=(r:Runtime,version='9')=>({table_id:self.table_id,session_id:r.client.getSnapshot()!.pending!.target_session_id,hand_id:self.hand.hand_id,status:'PENDING',table_version:version,duplicate:true});
async function reconnect(r:Runtime,patch:Parameters<typeof snapshot>[2]={}){r.client.disconnect();r.fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));await r.client.connect(self.table_id,'PLAYER_SELF',patch.readonly?'READ_ONLY':'CLAIM_CONTROL');r.fetcher.mockResolvedValueOnce(ok(missing(r)));authenticated(r.sockets.at(-1)!,9,patch);await r.client.recheckPending();}
async function unknown(intent:TableIntent={type:'action',action_type:'RAISE',target_to_units:'10000000'},readonly=false){const r=await setup(readonly);if(['topup','leave','takeover'].includes(intent.type)){r.fetcher.mockRejectedValueOnce(Error('synthetic lost result'));await expect(r.client.submit(intent,context(r))).rejects.toThrow();}else await r.client.submit(intent,context(r));return r;}

describe('R1 immutable original semantic intent and new-only keys',()=>{
  it('generates exactly 32 random business bytes only for new action/HTTP keys, not correlation/auth/ping IDs',async()=>{
    const random=vi.spyOn(crypto,'getRandomValues').mockImplementation((a:any)=>{expect(a).toHaveLength(32);a.fill(0x7f);return a;});const r=await unknown();const first=actions(r)[0],key=first.action_id;expect(key).toMatch(/^ik1_[A-Za-z0-9_-]{43}$/);expect(atob(key.slice(4).replace(/-/g,'+').replace(/_/g,'/'))).toHaveLength(32);expect(first.request_id).not.toMatch(/^ik1_/);expect(JSON.parse(r.sockets[0].sent[0]).request_id).not.toMatch(/^ik1_/);r.client.ping();expect(JSON.parse(r.sockets[0].sent.at(-1)!).request_id).not.toMatch(/^ik1_/);expect(random).toHaveBeenCalledTimes(1);
    const http=await unknown({type:'topup',amount_units:'500000'});expect(JSON.parse(http.fetcher.mock.calls[2][1].body).request_id).toMatch(/^ik1_[A-Za-z0-9_-]{43}$/);expect(random).toHaveBeenCalledTimes(2);
  });
  it('detaches caller values, retains original IDs and refreshes only current guard refs at explicit retry',async()=>{
    const intent:Extract<TableIntent,{type:'action'}>={type:'action',action_type:'RAISE',target_to_units:'10000000'},r=await unknown(intent),first=actions(r)[0];intent.action_type='FOLD';intent.target_to_units='0';await reconnect(r);expect(actions(r)).toHaveLength(1);expect(r.client.getSnapshot()?.can_retry_pending).toBe(true);await r.client.retryPending(context(r));const retried=actions(r)[1];expect(retried).toEqual({...first,expected_table_version:9,expected_hand_version:10,control_epoch:3});expect(JSON.stringify(r.client)).toBe('{}');expect(Object.keys(r.client.getSnapshot()!.pending!)).not.toContain('intent');expect(Object.keys(r.client.getSnapshot()!.pending!)).not.toContain('target_to_units');
  });
  it('keeps a legacy locator query-only without guessing a missing private intent or replacing its UUID keys',async()=>{
    const r=await setup(),legacy='01900000-0000-7000-8000-000000000001',correlation='01900000-0000-7000-8000-000000000002',state=r.client.getSnapshot()!;Object.assign(state,disconnectPokerStream(beginPokerPending(state,context(r),correlation,legacy,'action')));const random=vi.spyOn(crypto,'getRandomValues');r.fetcher.mockResolvedValueOnce(ok(missing(r)));await r.client.recheckPending();expect(JSON.parse(r.fetcher.mock.calls.at(-1)![1].body)).toEqual({kind:'action',mutation_id:legacy});await expect(r.client.retryPending(context(r))).rejects.toThrow();expect(r.client.getSnapshot()?.pending).toMatchObject({request_id:correlation,action_id:legacy,phase:'UNKNOWN'});expect(random).not.toHaveBeenCalled();
  });
});

describe('R1 original target and action decision-window guards',()=>{
  it('keeps old Session A query-only after seating B for HTTP and WS session operations',async()=>{
    for(const intent of [{type:'topup',amount_units:'500000'},{type:'leave',return_to_lobby:true},{type:'sitout'},{type:'resume'}] as TableIntent[]){const r=await unknown(intent),id=r.client.getSnapshot()!.pending!.request_id;await reconnect(r,{viewer:{session_id:self.table_id}});const calls=r.fetcher.mock.calls.length,sends=actions(r).length;await expect(r.client.retryPending(context(r))).rejects.toThrow();expect(r.client.getSnapshot()?.pending).toMatchObject({request_id:id,phase:'UNKNOWN',target_session_id:self.viewer.session_id});expect(r.fetcher).toHaveBeenCalledTimes(calls);expect(actions(r)).toHaveLength(sends);expect(r.client.getSnapshot()?.can_retry_pending).not.toBe(true);}
  });
  it('does not fit an old action to another hand/window, illegal amount/action, or expired deadline',async()=>{
    for(const patch of [{hand:{hand_id:self.table_id}},{hand:{action_sequence:'2'}},{viewer:{legal:{...self.viewer.legal,minimum_raise_to_units:'15000000',shortcuts:[]}}},{viewer:{legal:{...self.viewer.legal,actions:['CALL']}}}]){const r=await unknown();await reconnect(r,patch);expect(r.client.getSnapshot()?.connection_state,JSON.stringify(patch)).toBe('LIVE');await expect(r.client.retryPending(context(r))).rejects.toThrow();expect(actions(r)).toHaveLength(1);expect(r.client.getSnapshot()?.pending?.phase).toBe('UNKNOWN');expect(r.client.getSnapshot()?.can_retry_pending).not.toBe(true);}
    const expired=await unknown();await reconnect(expired);const time=vi.spyOn(performance,'now').mockReturnValue(performance.now()+31000);await expect(expired.client.retryPending(context(expired))).rejects.toThrow();expect(actions(expired)).toHaveLength(1);expect(expired.client.getSnapshot()?.pending?.phase).toBe('UNKNOWN');time.mockRestore();
  });
  it('permits original topup/leave with fresh owner capabilities on a read-only socket, never requiring controller',async()=>{
    for(const intent of [{type:'topup',amount_units:'500000'},{type:'leave',return_to_lobby:true}] as TableIntent[]){const r=await unknown(intent,true),body=JSON.parse(r.fetcher.mock.calls[2][1].body),path=r.fetcher.mock.calls[2][0];await reconnect(r,{readonly:true});r.fetcher.mockResolvedValueOnce(ok(receipt(r)));await r.client.retryPending(context(r));expect(r.fetcher.mock.calls.at(-1)![0]).toBe(path);expect(JSON.parse(r.fetcher.mock.calls.at(-1)![1].body)).toEqual(body);expect(r.client.getSnapshot()?.pending).toBeUndefined();expect(pokerStreamAuthority(r.client.getSnapshot()!).can_control).toBe(false);expect(actions(r)).toHaveLength(0);}
  });
});

describe('R1 synchronous retry latch and reentrant pre-send failures',()=>{
  it('preserves original UNKNOWN on retry pre-send disconnect and still retries that original after fresh recovery',async()=>{
    const r=await unknown(),original=r.client.getSnapshot()!.pending!;await reconnect(r);let once=true;const off=r.client.subscribe(()=>{if(once&&r.client.getSnapshot()?.pending?.phase==='SENT'){once=false;r.client.disconnect();}});await expect(r.client.retryPending(context(r))).rejects.toThrow();off();expect(r.client.getSnapshot()?.pending).toMatchObject({...original,phase:'UNKNOWN'});expect(actions(r)).toHaveLength(1);await reconnect(r);await r.client.retryPending(context(r));expect(actions(r)).toHaveLength(2);expect(actions(r)[1].action_id).toBe(original.action_id);
  });
  it('two concurrent explicit retries produce exactly one send and preserve both original IDs',async()=>{
    const r=await unknown();await reconnect(r);const original=r.client.getSnapshot()!.pending!,a=r.client.retryPending(context(r)),b=r.client.retryPending(context(r));await a;await expect(b).rejects.toThrow();expect(actions(r)).toHaveLength(2);expect(r.client.getSnapshot()?.pending).toMatchObject({request_id:original.request_id,action_id:original.action_id,phase:'SENT'});
  });
  it('a retry transport send failure retains the original UNKNOWN and waits for new query/control evidence',async()=>{
    const r=await unknown();await reconnect(r);const p=r.client.getSnapshot()!.pending!;vi.spyOn(r.sockets.at(-1)!,'send').mockImplementationOnce(()=>{throw Error('synthetic private retry send');});await expect(r.client.retryPending(context(r))).rejects.toMatchObject({uncertain:true});expect(r.client.getSnapshot()?.pending).toMatchObject({...p,phase:'UNKNOWN'});expect(r.client.getSnapshot()?.connection_state).toBe('RECONNECTING');expect(r.client.getSnapshot()?.can_retry_pending).toBe(false);expect(JSON.stringify(r.client.getSnapshot())).not.toContain('synthetic private retry send');expect(actions(r)).toHaveLength(1);
  });
});

describe('R1 receipt-query prerequisite and old attempt fencing',()=>{
  it('requires current NOT_FOUND then uses the same key across a synthetic late-commit race, never a second effect',async()=>{
    const r=await unknown({type:'topup',amount_units:'500000'}),body=JSON.parse(r.fetcher.mock.calls[2][1].body);await expect(r.client.retryPending(context(r))).rejects.toThrow();expect(r.fetcher).toHaveBeenCalledTimes(3);await reconnect(r,{readonly:true});const committed=receipt(r),durable=new Map([[body.request_id,committed]]);let effects=1;r.fetcher.mockImplementationOnce((_path,init)=>{const retry=JSON.parse(init.body);if(!durable.has(retry.request_id)){effects++;durable.set(retry.request_id,committed);}return Promise.resolve(ok(durable.get(retry.request_id)));});await r.client.retryPending(context(r));expect(effects).toBe(1);expect(JSON.parse(r.fetcher.mock.calls.at(-1)![1].body)).toEqual(body);expect(r.client.getSnapshot()?.pending).toBeUndefined();expect(r.client.getSnapshot()?.last_receipt).toEqual(committed);await expect(r.client.retryPending(context(r))).rejects.toThrow();
  });
  it('a newer failed query invalidates prior NOT_FOUND readiness, and query itself never unlocks ordinary submit',async()=>{
    const r=await unknown();await reconnect(r);expect(r.client.getSnapshot()?.can_retry_pending).toBe(true);await expect(r.client.submit({type:'action',action_type:'CALL',target_to_units:'0'},context(r))).rejects.toThrow();r.fetcher.mockRejectedValueOnce(Error('synthetic query lost'));await r.client.recheckPending();expect(r.client.getSnapshot()?.can_retry_pending).toBe(false);await expect(r.client.retryPending(context(r))).rejects.toThrow();expect(r.client.getSnapshot()?.pending?.phase).toBe('UNKNOWN');expect(actions(r)).toHaveLength(1);
  });
  it('an older HTTP send whose query already resolved cannot decode into a newer same-scope business attempt',async()=>{
    const r=await setup(true);let oldReply!:(v:Response)=>void;r.fetcher.mockReturnValueOnce(new Promise<Response>(resolve=>oldReply=resolve));const old=r.client.submit({type:'topup',amount_units:'500000'},context(r));const p=r.client.getSnapshot()!.pending!;r.fetcher.mockResolvedValueOnce(ok({...missing(r),state:'FOUND',receipt:receipt(r,'7')}));await r.client.recheckPending();expect(r.client.getSnapshot()?.pending).toBeUndefined();let newReply!:(v:Response)=>void;r.fetcher.mockReturnValueOnce(new Promise<Response>(resolve=>newReply=resolve));const next=r.client.submit({type:'topup',amount_units:'1000000'},context(r)),pending=r.client.getSnapshot()!.pending!;expect(pending.request_id).not.toBe(p.request_id);oldReply(ok({private_secret:'invalid stale old attempt'}));await expect(old).resolves.toBeUndefined();expect(r.client.getSnapshot()?.pending).toEqual(pending);newReply(ok(receipt(r,'7')));await next;expect(r.client.getSnapshot()?.pending).toBeUndefined();
  });
});

describe('R1 lifecycle and zero automatic business replay',()=>{
  it('fences stale rendered context and never turns queries, full snapshots, ping or visibility into retries',async()=>{
    const r=await unknown(),old=context(r);await reconnect(r);await expect(r.client.retryPending(old)).rejects.toThrow();r.fetcher.mockResolvedValueOnce(ok(missing(r)));await r.client.recheckPending();r.client.ping();vi.spyOn(document,'visibilityState','get').mockReturnValue('visible');document.dispatchEvent(new Event('visibilitychange'));r.sockets.at(-1)!.message(snapshot(r.sockets.at(-1)!,10));expect(actions(r)).toHaveLength(1);expect(r.client.getSnapshot()?.pending?.phase).toBe('UNKNOWN');expect(localStorage.length).toBe(0);expect(sessionStorage.length).toBe(0);expect(r.fetcher.mock.calls.map(([path])=>path).some((p:string)=>p.includes('?'))).toBe(false);
  });
  it('logout, dispose and table switch remove original retry eligibility and retain no enumerable private intent',async()=>{
    for(const mode of ['logout','dispose','table','account','session']){const r=await unknown();await reconnect(r);if(mode==='logout'){r.fetcher.mockResolvedValueOnce(ok({}));await r.api.logout();}else if(mode==='dispose')r.client.dispose();else if(mode==='account'||mode==='session'){r.fetcher.mockResolvedValueOnce(ok({access_token:'synthetic changed',access_expires_at:4102444800,user:{id:mode==='account'?910003:910002,username:'synthetic',role:1},session:{sid:'synthetic new session'}}));await r.api.login('synthetic','synthetic');}else{r.fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));await r.client.connect(self.hand.hand_id,'PLAYER_SELF','CLAIM_CONTROL');}await expect(r.client.retryPending({user_id:'910002',runtime_id:'stale',event_sequence:'0'})).rejects.toThrow();expect(r.client.getSnapshot()?.pending).toBeUndefined();expect(r.client.getSnapshot()?.can_retry_pending).not.toBe(true);expect(JSON.stringify(r.client)).toBe('{}');expect(actions(r)).toHaveLength(1);}
  });
});

describe('R1 explicit retry presentation keeps ordinary mutation lock',()=>{
  it('sends only rendered context through the folded status retry and exposes it only while runtime eligibility holds',async()=>{
    const r=await unknown();await reconnect(r);const s=r.client.getSnapshot()!,retry=vi.fn(),mutation=vi.fn(),props={table:s.table!,authority:{...pokerStreamAuthority(s),can_retry_pending:true},ui:{bet:{action_type:'RAISE' as const,amount_chips:'999'},top_up_chips:'999',confirm_leave:false,confirm_takeover:false},onUiChange:vi.fn(),onIntent:mutation,onRetryPending:retry};const view=render(<PokerTable {...props}/>);expect(screen.queryByRole('button',{name:'重试原操作'})).not.toBeInTheDocument();fireEvent.click(screen.getByRole('button',{name:'查看状态'}));fireEvent.click(screen.getByRole('button',{name:'重试原操作'}));expect(retry).toHaveBeenCalledWith(pokerStreamContext(s));expect(mutation).not.toHaveBeenCalled();view.rerender(<PokerTable {...props} receipt_querying/>);expect(screen.getByRole('button',{name:'重试原操作'})).toBeDisabled();fireEvent.click(screen.getByRole('button',{name:'重试原操作'}));expect(retry).toHaveBeenCalledTimes(1);view.rerender(<PokerTable {...props} authority={{...props.authority,can_retry_pending:false}}/>);expect(screen.queryByRole('button',{name:'重试原操作'})).not.toBeInTheDocument();for(const b of screen.getAllByRole('button'))if(b.textContent?.includes('跟注'))expect(b).toBeDisabled();
  });
});
