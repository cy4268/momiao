import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { createElement, StrictMode } from 'react';
import { renderToString } from 'react-dom/server';
import { ApiClient } from '../api';
import { LivePokerTable, type LivePokerTableProps } from './LivePokerTable';
import { parsePokerLobbySnapshot } from './poker-lobby-read';
import publicView from './fixtures/g3-public.json';
import lobby from './fixtures/l1-lobby.synthetic.json';
import { PokerTable } from './PokerTable';
import { parsePokerTableView } from './poker-view';
import type { PokerTableProps } from './poker-ui-types';
import self from './fixtures/g3-player-self.json';
const observed=vi.hoisted(()=>({current:null as PokerTableProps|null}));
// Pass-through observes saved React callbacks, never replaces the real Table/client or injects stream state.
vi.mock('./PokerTable',async original=>{const actual=await original<typeof import('./PokerTable')>();return{PokerTable:(p:PokerTableProps)=>{observed.current=p;return createElement(actual.PokerTable,p);}};});
afterEach(()=>{cleanup();vi.useRealTimers();vi.unstubAllGlobals();vi.restoreAllMocks();Socket.all=[];observed.current=null;});

const ok=(data:unknown)=>new Response(JSON.stringify({success:true,data}),{headers:{'Content-Type':'application/json'}});
const table=self.table_id,session=self.viewer.session_id,other='019a0000-0000-7000-8000-000000000011';
const active=()=>({session_id:session,table_id:table,table_name:'原会话牌桌',state:'ACTIVE',seat_no:2,stack_units:'1000000',committed_units:'500000',poker_in_play_units:'1500000',control_epoch:'1',initial_buy_in_units:'2000000',total_top_up_units:'0',started_at:'2026-09-06T00:00:00Z'});
const settled=()=>({...active(),state:'SETTLED',stack_units:'0',committed_units:'0',poker_in_play_units:'0',final_cash_out_units:'1500000',realized_pl_units:'-500000',ended_at:'2026-09-06T01:00:00Z',end_reason:'SAFE_LEAVE'});
function deferred<T>(){let resolve!:(value:T)=>void,reject!:(reason?:unknown)=>void;return{promise:new Promise<T>((r,j)=>{resolve=r;reject=j;}),resolve,reject};}
class Socket extends EventTarget {
  static all:Socket[]=[];readyState=0;bufferedAmount=0;protocol='chaldea-poker.v1';sent:string[]=[];id=String(Socket.all.length+1).repeat(32);
  constructor(){super();Socket.all.push(this);}send(text:string){this.sent.push(text);}close(){this.readyState=3;this.dispatchEvent(new CloseEvent('close',{code:1000}));}
  message(data:string){this.dispatchEvent(new MessageEvent('message',{data}));}
}
function frame(type:string,payload:unknown,version=7){return JSON.stringify({type,event_id:'live-synthetic',event_seq:10,table_id:table,table_version:version,hand_id:self.hand.hand_id,hand_version:version+1,server_time:self.server_now,payload});}
function full(socket:Socket,kind:'PLAYER_SELF'|'SPECTATOR'='PLAYER_SELF',version=7,controller=true,sid=session){
  const v:any=structuredClone(kind==='PLAYER_SELF'?self:publicView);v.server_now=self.server_now;v.table_version=String(version);v.hand.hand_version=String(version+1);
  if(kind==='PLAYER_SELF')v.viewer.session_id=sid;
  v.viewer.control={connection_id:socket.id,...(kind==='PLAYER_SELF'?{session_id:sid}:{}),mode:kind==='PLAYER_SELF'&&controller?'CONTROLLER':'READ_ONLY',control_epoch:v.viewer.control_epoch};
  if(!controller){v.viewer.can_act=false;delete v.viewer.legal;}socket.message(frame('table.snapshot',v,version));
}
function auth(socket:Socket,kind:'PLAYER_SELF'|'SPECTATOR'='PLAYER_SELF',controller=true,version=7){socket.readyState=1;socket.dispatchEvent(new Event('open'));socket.message(frame('auth.accepted',{request_id:JSON.parse(socket.sent[0]).request_id,connection_id:socket.id}));full(socket,kind,version,controller);}
async function setup(spectator=false){
  const fetcher=vi.fn<(path:string,init?:RequestInit)=>Promise<Response>>();
  fetcher.mockResolvedValueOnce(ok({access_token:'synthetic',access_expires_at:4102444800,user:{id:910002,username:'synthetic',role:1},session:{sid:'synthetic'}}));
  const api=new ApiClient(fetcher);await api.login('synthetic','synthetic');vi.stubGlobal('WebSocket',Socket);
  const raw:any=structuredClone(lobby);raw.viewer.user_id='910002';
  if(!spectator){raw.active_session={...active(),small_blind_units:self.small_blind_units,big_blind_units:self.big_blind_units,ante_units:'0',can_reconnect:true};for(const k of ['control_epoch','initial_buy_in_units','total_top_up_units','started_at'])delete raw.active_session[k];raw.viewer.poker_in_play_units='1500000';}
  const p:LivePokerTableProps={client:api,table_id:table,admission:{snapshot:parsePokerLobbySnapshot(raw,'910002'),scope:{user_id:'910002',session_generation:api.getSessionGeneration(),request_generation:1,query_key:'/api/v1/poker'}},mutation_blocked:false,onReturnLobby:vi.fn()};
  let sessionRead=()=>Promise.resolve(ok(active()));
  fetcher.mockImplementation((path)=>path.endsWith('/connect-tickets')?Promise.resolve(ok({poker_connect_ticket:'ct1.eyJzeW50aGV0aWMiOnRydWV9.c2lnbmF0dXJl'})):path.endsWith('/logout')?Promise.resolve(ok({})):path.endsWith('/safe-leave')?Promise.reject(Error('synthetic lost ACK')):path.endsWith('/sessions/'+session)?sessionRead():Promise.resolve(new Response('',{status:403})));
  return{api,fetcher,p,setRead:(read:()=>Promise<Response>)=>{sessionRead=read;}};
}
type Runtime=Awaited<ReturnType<typeof setup>>;
const calls=(r:Runtime,suffix:string)=>r.fetcher.mock.calls.filter(([path])=>path.endsWith(suffix));
const actions=()=>Socket.all.flatMap(s=>s.sent.map(t=>JSON.parse(t))).filter(f=>f.type==='hand.action');
async function live(r:Runtime,strict=false){const view=render(strict?<StrictMode><LivePokerTable {...r.p}/></StrictMode>:<LivePokerTable {...r.p}/>);await waitFor(()=>expect(Socket.all.filter(s=>s.readyState===0)).toHaveLength(1));const s=Socket.all.at(-1)!;act(()=>auth(s,r.p.admission.snapshot.active_session?'PLAYER_SELF':'SPECTATOR'));expect(observed.current?.authority.connection_state).toBe('LIVE');return{...view,s};}
const context=()=>{const p=observed.current!;return{...p.authority,table_id:p.table.table_id,table_version:p.table.table_version,hand_id:p.table.hand?.hand_id,hand_version:p.table.hand?.hand_version,session_id:p.table.viewer.session_id,control_epoch:p.table.viewer.control_epoch,action_sequence:p.table.hand?.action_sequence};};
async function leave(){fireEvent.click(screen.getByRole('button',{name:'安全离座'}));fireEvent.click(screen.getByRole('button',{name:'确认安全离座'}));await act(async()=>{});}

describe('Live Table policy presentation boundary',()=>{
  it.each(['action','takeover','retry','control-retry'])('blocks %s at the real Table UI without invoking recovery reads',kind=>{
    const callback=vi.fn(),query=vi.fn();
    const p:PokerTableProps & {mutation_blocked:boolean}={table:parsePokerTableView(self,self.table_id,'PLAYER_SELF'),mutation_blocked:true,
      authority:{user_id:'910002',runtime_id:'synthetic-component',event_sequence:'10',connection_state:'LIVE',has_snapshot:true,pending:false,can_reconnect:false,can_takeover:false,viewer_kind:'PLAYER_SELF',can_control:true},
      ui:{bet:{action_type:'RAISE',amount_chips:''},top_up_chips:'1',confirm_leave:false,confirm_takeover:false},onUiChange:callback,onIntent:callback,onRetryPending:callback,onRetryTakeover:callback,onQueryReceipt:query};
    if(kind==='takeover'){p.authority.can_control=false;p.authority.can_takeover=true;p.ui.confirm_takeover=true;}
    if(kind==='retry'){p.authority.pending=true;p.authority.can_retry_pending=true;}
    if(kind==='control-retry')p.authority.control_recovery={phase:'UNKNOWN',querying:false,can_retry:true,target_current:true};
    render(<PokerTable {...p}/>);
    if(kind==='retry'||kind==='control-retry')fireEvent.click(screen.getByRole('button',{name:'查看状态'}));
    const name=kind==='action'?/跟注 .* Chips/:kind==='takeover'?'确认接管牌桌':kind==='retry'?'重试原操作':'重试本次接管';
    const button=screen.getByRole('button',{name});expect(button).toBeDisabled();fireEvent.click(button);expect(callback).not.toHaveBeenCalled();
    expect(query).not.toHaveBeenCalled();
  });
});

describe('Live Table native lifecycle and real transport binding',()=>{
  it('keeps explicit reconnect reachable before the first full snapshot, with no fabricated Table',async()=>{
    const r=await setup();r.fetcher.mockResolvedValueOnce(new Response(JSON.stringify({success:false,code:'POKER_ACCESS_DENIED'}),{status:403}));render(<LivePokerTable {...r.p}/>);await act(async()=>{});
    expect(observed.current).toBeNull();fireEvent.click(screen.getByRole('button',{name:'重新连接'}));await waitFor(()=>expect(Socket.all).toHaveLength(1));act(()=>auth(Socket.all[0]));expect(observed.current?.authority.connection_state).toBe('LIVE');
  });
  it('does no render-time I/O/listener work and StrictMode keeps one usable connection',async()=>{
    const r=await setup(),listen=vi.spyOn(document,'addEventListener');renderToString(<LivePokerTable {...r.p}/>);
    expect(listen.mock.calls.filter(([name])=>name==='visibilitychange')).toHaveLength(0);expect(calls(r,'/connect-tickets')).toHaveLength(0);
    const view=await live(r,true);expect(Socket.all.filter(s=>s.readyState===1)).toHaveLength(1);view.unmount();expect(Socket.all.every(s=>s.readyState===3)).toBe(true);
  });
  it('derives spectator only from accepted whole Lobby state and permits read-only return',async()=>{
    const r=await setup(true);await live(r);expect(JSON.parse(calls(r,'/connect-tickets')[0][1]!.body as string).control_intent).toBe('READ_ONLY');
    expect(observed.current?.authority.viewer_kind).toBe('SPECTATOR');fireEvent.click(screen.getByRole('button',{name:'返回大厅'}));expect(r.p.onReturnLobby).toHaveBeenCalledTimes(1);
  });
  it.each(['wrong-owner','other-table','stale-native'])('opens no connection for %s admission',async kind=>{
    const r=await setup();if(kind==='wrong-owner')r.p.admission.scope.user_id='910003';if(kind==='other-table')r.p.admission.snapshot.active_session!.table_id=other;if(kind==='stale-native')r.p.admission.scope.session_generation++;
    render(<LivePokerTable {...r.p}/>);await act(async()=>{});expect(calls(r,'/connect-tickets')).toHaveLength(0);expect(observed.current).toBeNull();
  });
  it('soft policies preserve UNKNOWN and saved callbacks use latest gate; automatic reads and retry use original IDs',async()=>{
    const r=await setup(),v=await live(r),saved=observed.current!.onIntent,oldContext=context();
    r.fetcher.mockImplementation(async(path,init)=>path.endsWith('/connect-tickets')?ok({poker_connect_ticket:'ct1.eyJzeW50aGV0aWMiOnRydWV9.c2lnbmF0dXJl'}):path.endsWith('/receipt-query')?(()=>{const body=JSON.parse(init!.body as string);return ok({table_id:table,kind:body.kind,mutation_id:body.mutation_id,state:'NOT_FOUND'});})():ok({}));
    fireEvent.click(screen.getByRole('button',{name:/跟注 .* Chips/}));expect(actions()).toHaveLength(1);act(()=>v.s.close());
    const original=actions()[0];await waitFor(()=>expect(calls(r,'/receipt-query')).toHaveLength(1));expect(calls(r,'/receipt-query')[0][1]!.body).toContain(original.action_id);
    for(const blocked of [true,true,true,false]){v.rerender(<LivePokerTable {...r.p} mutation_blocked={blocked} admission={{...r.p.admission,scope:{...r.p.admission.scope,request_generation:9}}}/>);await act(async()=>{});expect(calls(r,'/connect-tickets')).toHaveLength(1);expect(observed.current?.authority.pending).toBe(true);}
    v.rerender(<LivePokerTable {...r.p} mutation_blocked/>);act(()=>saved({type:'action',action_type:'CALL',target_to_units:'0'},oldContext));expect(actions()).toHaveLength(1);
    fireEvent.click(screen.getByRole('button',{name:'重新连接'}));await waitFor(()=>expect(Socket.all).toHaveLength(2));act(()=>auth(Socket.all[1]));await act(async()=>{});
    fireEvent.click(screen.getByRole('button',{name:'查看状态'}));
    expect(screen.getByRole('button',{name:'重试原操作'})).toBeDisabled();const savedRetry=observed.current!.onRetryPending!;act(()=>savedRetry(context()));expect(actions()).toHaveLength(1);
    v.rerender(<LivePokerTable {...r.p}/>);fireEvent.click(screen.getByRole('button',{name:'重试原操作'}));await act(async()=>{});expect(actions()).toHaveLength(2);expect(actions()[1]).toMatchObject({action_id:original.action_id,request_id:original.request_id});
  });
  it('does not turn MAINTENANCE metadata or a same-native-session token refresh into a hard boundary',async()=>{
    const r=await setup(),v=await live(r),generation=r.api.getSessionGeneration();r.fetcher.mockResolvedValueOnce(ok({access_token:'synthetic-refresh',access_expires_at:4102444800,user:{id:910002,username:'synthetic',role:1},session:{sid:'synthetic'}}));await act(async()=>{await r.api.login('synthetic','synthetic');});
    v.rerender(<LivePokerTable {...r.p} admission={{...r.p.admission,snapshot:{...r.p.admission.snapshot,service:{state:'MAINTENANCE',production_ready:false,blockers:['MAINTENANCE_ACTIVE'],maintenance_scopes:['POKER_NEW_TABLES_NEW_HANDS']}}}}/>);
    expect(r.api.getSessionGeneration()).toBe(generation);expect(calls(r,'/connect-tickets')).toHaveLength(1);expect(observed.current?.authority.can_control).toBe(true);
    const raw:any=structuredClone(self);raw.table_version='8';raw.hand.hand_version='9';raw.viewer.control={connection_id:v.s.id,session_id:session,mode:'CONTROLLER',control_epoch:'1'};
    raw.chat={enabled:true,can_send:true,muted:false,last_sequence:'0',truncated:false,messages:[]};raw.host={is_host:true,capabilities:['PAUSE_ACCEPTING_PLAYERS'],players:[],spectators:[],spectators_truncated:false,chat_targets:[],chat_targets_truncated:false};
    act(()=>v.s.message(frame('table.snapshot',raw,8)));const before=r.fetcher.getMockImplementation()!,post=deferred<Response>();vi.useFakeTimers();
    r.fetcher.mockImplementation(async(path,init)=>{if(path.endsWith('/commands'))return post.promise;if(path.endsWith('/receipt-query')){const body=JSON.parse(init!.body as string);return ok({table_id:table,kind:body.kind,mutation_id:body.mutation_id,state:'NOT_FOUND'});}return before(path,init);});
    act(()=>observed.current!.onSendChat?.('自动核对消息',context()));const chat=JSON.parse([...v.s.sent].reverse().find(value=>JSON.parse(value).type==='chat.send')!);
    act(()=>v.s.message(frame('service.notice',{request_id:chat.request_id,action_id:null,receipt:{table_id:table,status:'CHAT_ACCEPTED',table_version:'9',duplicate:false,chat_sequence:'1'}},9)));
    await act(async()=>{await vi.advanceTimersByTimeAsync(0);});const syncs=()=>v.s.sent.map(value=>JSON.parse(value)).filter(value=>value.type==='sync.request').length;expect(syncs()).toBe(1);
    await act(async()=>{await vi.advanceTimersByTimeAsync(1999);});expect(syncs()).toBe(1);await act(async()=>{await vi.advanceTimersByTimeAsync(1);});expect(syncs()).toBe(2);
    const projected:any=structuredClone(raw);projected.table_version='9';projected.hand.hand_version='10';projected.chat.last_sequence='1';act(()=>v.s.message(frame('table.snapshot',projected,9)));
    await act(async()=>{await vi.advanceTimersByTimeAsync(8000);});expect(syncs()).toBe(2);expect(observed.current?.authority.chat_recovery).toBeUndefined();expect(vi.getTimerCount()).toBe(0);
    const hostContext=context();act(()=>observed.current!.onHostCommand?.({command:'PAUSE_ACCEPTING_PLAYERS'},hostContext));act(()=>{observed.current!.onQueryHostReceipt?.();observed.current!.onRetryHost?.(hostContext);window.dispatchEvent(new Event('focus'));});
    await act(async()=>{await vi.advanceTimersByTimeAsync(0);});expect(calls(r,'/commands')).toHaveLength(1);expect(calls(r,'/receipt-query').filter(([,init])=>JSON.parse(init!.body as string).kind==='host')).toHaveLength(0);
    await act(async()=>{post.reject(Error('synthetic lost host ACK'));await Promise.resolve();});
    await act(async()=>{await vi.advanceTimersByTimeAsync(0);});
    expect(calls(r,'/receipt-query').filter(([,init])=>JSON.parse(init!.body as string).kind==='host')).toHaveLength(1);expect(observed.current?.host_operation).toMatchObject({phase:'UNKNOWN',querying:false,can_retry:true});
    expect((observed.current as PokerTableProps&{recovery_status?:string})?.recovery_status).toContain('继续自动核对');
    fireEvent.click(screen.getByRole('button',{name:/跟注 .* Chips/}));expect(actions()).toHaveLength(1);
  });
  it('uses the existing server clock for display without commands and resets controlled drafts at hand/action fences',async()=>{
    const r=await setup(),v=await live(r);vi.useFakeTimers();const count=v.s.sent.length,http=r.fetcher.mock.calls.length;
    act(()=>observed.current!.onUiChange({...observed.current!.ui,bet:{action_type:'RAISE',amount_chips:'8'},confirm_leave:true}));expect(observed.current?.ui.bet.amount_chips).toBe('8');
    await act(async()=>{vi.advanceTimersByTime(3000);});expect(v.s.sent).toHaveLength(count);expect(r.fetcher.mock.calls).toHaveLength(http);expect(observed.current?.display_now).toBeTruthy();
    act(()=>full(v.s,'PLAYER_SELF',8));expect(observed.current?.ui.bet.amount_chips).toBe('8');const raw:any=structuredClone(self);raw.hand.action_sequence='2';raw.table_version='9';raw.hand.hand_version='10';raw.viewer.control={connection_id:v.s.id,session_id:session,mode:'CONTROLLER',control_epoch:'1'};
    act(()=>v.s.message(frame('table.snapshot',raw,9)));expect(observed.current?.ui.bet.amount_chips).toBe('');expect(observed.current?.ui.confirm_leave).toBe(false);
  });
  it('clears a stale draft when a same-epoch control.changed fence revokes the socket',async()=>{
    const r=await setup(),v=await live(r);act(()=>observed.current!.onUiChange({...observed.current!.ui,bet:{action_type:'RAISE',amount_chips:'9'},confirm_takeover:true}));
    act(()=>v.s.message(frame('control.changed',{connection_id:v.s.id,session_id:session,mode:'READ_ONLY',control_epoch:'1'})));expect(observed.current?.ui.bet.amount_chips).toBe('');expect(observed.current?.ui.confirm_takeover).toBe(false);
  });
  it('forwards current/historical control queries and original-key Retry while separating READ_ONLY connection and TakeOver',async()=>{
    const r=await setup(),v=await live(r);act(()=>full(v.s,'PLAYER_SELF',8,false));let original='';
    r.fetcher.mockImplementation(async(path,init)=>{if(path.endsWith('/connect-tickets'))return ok({poker_connect_ticket:'ct1.eyJzeW50aGV0aWMiOnRydWV9.c2lnbmF0dXJl'});if(path.endsWith('/take-over')){original ||=JSON.parse(init!.body as string).request_id;throw Error('synthetic lost takeover ACK');}if(path.endsWith('/receipt-query'))return ok({table_id:table,kind:'takeover',mutation_id:JSON.parse(init!.body as string).mutation_id,state:'NOT_FOUND'});return ok({});});
    fireEvent.click(screen.getByRole('button',{name:'申请接管'}));fireEvent.click(screen.getByRole('button',{name:'确认接管牌桌'}));await act(async()=>{});const retry=observed.current!.onRetryTakeover!;
    await waitFor(()=>expect(calls(r,'/receipt-query')).toHaveLength(1));expect(calls(r,'/receipt-query')[0][1]!.body).toContain(original);fireEvent.click(screen.getByRole('button',{name:'取消接管'}));
    v.rerender(<LivePokerTable {...r.p} mutation_blocked/>);act(()=>retry(context()));expect(calls(r,'/take-over')).toHaveLength(1);fireEvent.click(screen.getByRole('button',{name:'查看状态'}));expect(screen.getByRole('button',{name:'重试本次接管'})).toBeDisabled();
    v.rerender(<LivePokerTable {...r.p}/>);fireEvent.click(screen.getByRole('button',{name:'重试本次接管'}));await act(async()=>{});expect(calls(r,'/take-over')).toHaveLength(2);expect(JSON.parse(calls(r,'/take-over')[1][1]!.body as string).request_id).toBe(original);
    act(()=>v.s.close());act(()=>observed.current!.onIntent({type:'reconnect',control_intent:'READ_ONLY'},context()));await waitFor(()=>expect(Socket.all).toHaveLength(2));act(()=>auth(Socket.all[1],'PLAYER_SELF',false,9));await act(async()=>{});
    expect(observed.current?.authority.ticket_intent).toBe('READ_ONLY');expect(observed.current?.authority.control_history?.count).toBe(1);await waitFor(()=>expect(calls(r,'/receipt-query').length).toBeGreaterThan(1));expect(calls(r,'/receipt-query').at(-1)![1]!.body).toContain(original);
    fireEvent.click(screen.getByRole('button',{name:'申请控制连接'}));await waitFor(()=>expect(Socket.all).toHaveLength(3));act(()=>auth(Socket.all[2],'PLAYER_SELF',false,10));expect(calls(r,'/take-over')).toHaveLength(2);expect(actions()).toHaveLength(0);
    fireEvent.click(screen.getByRole('button',{name:'申请接管'}));fireEvent.click(screen.getByRole('button',{name:'确认接管牌桌'}));await act(async()=>{});expect(calls(r,'/take-over')).toHaveLength(3);expect(JSON.parse(calls(r,'/take-over')[2][1]!.body as string).request_id).not.toBe(original);
  });
  it.each(['table','viewer','logout','unmount'])('hard %s boundary hides old private view and stale callbacks never send',async kind=>{
    const r=await setup(),v=await live(r),saved=observed.current!.onIntent,c=context(),before=v.s.sent.length;
    if(kind==='table')v.rerender(<LivePokerTable {...r.p} table_id={other}/>);else if(kind==='viewer'){const snapshot={...r.p.admission.snapshot,active_session:null,viewer:{...r.p.admission.snapshot.viewer,poker_in_play_units:'0'}};v.rerender(<LivePokerTable {...r.p} admission={{...r.p.admission,snapshot}}/>);}else if(kind==='logout')await act(async()=>{await r.api.logout();});else v.unmount();
    expect(screen.queryByText('当前会话')).not.toBeInTheDocument();act(()=>{saved({type:'action',action_type:'CALL',target_to_units:'0'},c);full(v.s);});expect(v.s.sent).toHaveLength(before);expect(r.p.onReturnLobby).not.toHaveBeenCalled();
  });
});

describe('Original Session safe-exit binding, not a receipt or empty seat inference',()=>{
  it('keeps the original Session query visible during reconnect without a Table snapshot',async()=>{
    const r=await setup(),v=await live(r);r.setRead(async()=>new Response('',{status:503}));await leave();act(()=>v.s.close());fireEvent.click(screen.getByRole('button',{name:'重新连接'}));await waitFor(()=>expect(Socket.all).toHaveLength(2));
    expect(screen.queryByRole('button',{name:'核对原会话出金'})).not.toBeInTheDocument();r.setRead(async()=>ok(settled()));await act(async()=>{window.dispatchEvent(new Event('focus'));});expect(r.p.onReturnLobby).toHaveBeenCalledTimes(1);
  });
  it('does not revive A after a committed B admission even if an older A admission later reappears',async()=>{
    const r=await setup(),d=deferred<Response>();r.setRead(()=>d.promise);const v=await live(r);await leave();
    v.rerender(<LivePokerTable {...r.p} admission={{...r.p.admission,snapshot:{...r.p.admission.snapshot,active_session:{...r.p.admission.snapshot.active_session!,session_id:other}}}}/>);v.rerender(<LivePokerTable {...r.p}/>);
    await act(async()=>{d.resolve(ok(settled()));});expect(r.p.onReturnLobby).not.toHaveBeenCalled();
  });
  it('starts before lost leave ACK, polls only original A and navigates once for complete SETTLED while Table GET is 403',async()=>{
    const r=await setup(),v=await live(r),ack=deferred<Response>(),fetch=r.fetcher.getMockImplementation()!;r.fetcher.mockImplementation((path,init)=>path.endsWith('/safe-leave')?ack.promise:fetch(path,init));vi.useFakeTimers();await leave();expect(calls(r,'/safe-leave')).toHaveLength(1);expect(calls(r,'/sessions/'+session)).toHaveLength(1);expect(r.p.onReturnLobby).not.toHaveBeenCalled();
    await expect(r.api.request('/api/v1/poker/tables/'+table)).rejects.toMatchObject({status:403});act(()=>full(v.s,'SPECTATOR',8));expect(observed.current?.table.viewer.session_id).toBeUndefined();expect(r.p.onReturnLobby).not.toHaveBeenCalled();
    r.setRead(async()=>ok(settled()));await act(async()=>{vi.advanceTimersByTime(2000);});expect(r.p.onReturnLobby).toHaveBeenCalledTimes(1);await act(async()=>{vi.advanceTimersByTime(8000);});expect(calls(r,'/sessions/'+session)).toHaveLength(2);expect(r.p.onReturnLobby).toHaveBeenCalledTimes(1);
  });
  it.each(['ACTIVE','NEEDS_REVIEW','cashout','pnl','endtime','reason','exposure','wrong-session','receipt'])('retains exit target and never navigates for %s',async kind=>{
    const r=await setup();let value:any=settled();if(kind==='ACTIVE'||kind==='NEEDS_REVIEW')value={...active(),state:kind};if(kind==='cashout')delete value.final_cash_out_units;if(kind==='pnl')value.realized_pl_units='0';if(kind==='endtime')delete value.ended_at;if(kind==='reason')delete value.end_reason;if(kind==='exposure'){value.stack_units='500000';value.poker_in_play_units='500000';}if(kind==='wrong-session')value.session_id=other;if(kind==='receipt')value={table_id:table,session_id:session,status:'CONFIRMED',table_version:'8',duplicate:false};
    r.setRead(async()=>ok(value));await live(r);await leave();expect(calls(r,'/sessions/'+session)).toHaveLength(1);expect(r.p.onReturnLobby).not.toHaveBeenCalled();
  });
  it('singleflights exit reads, backs off after read error, focus wakes A, and soft gate retains exit checking',async()=>{
    const r=await setup(),d=deferred<Response>(),v=await live(r),before=r.fetcher.getMockImplementation()!;r.setRead(()=>d.promise);vi.useFakeTimers();
    const raw:any=structuredClone(self);raw.table_version='8';raw.hand.hand_version='9';raw.viewer.control={connection_id:v.s.id,session_id:session,mode:'CONTROLLER',control_epoch:'1'};raw.chat={enabled:true,can_send:true,muted:false,last_sequence:'0',truncated:false,messages:[]};act(()=>v.s.message(frame('table.snapshot',raw,8)));
    r.fetcher.mockImplementation((path,init)=>path.endsWith('/receipt-query')?Promise.resolve((()=>{const body=JSON.parse(init!.body as string);return ok({table_id:table,kind:body.kind,mutation_id:body.mutation_id,state:'NOT_FOUND'});})()):before(path,init));
    act(()=>observed.current!.onSendChat?.('离座查询不阻塞聊天',context()));await leave();await act(async()=>{await Promise.resolve();});
    expect(calls(r,'/receipt-query').some(([,init])=>JSON.parse(init!.body as string).kind==='chat')).toBe(true);
    act(()=>{window.dispatchEvent(new Event('focus'));window.dispatchEvent(new Event('focus'));});expect(calls(r,'/sessions/'+session)).toHaveLength(1);
    await act(async()=>{d.resolve(new Response('',{status:503}));});await act(async()=>{vi.advanceTimersByTime(1999);});expect(calls(r,'/sessions/'+session)).toHaveLength(1);
    await act(async()=>{vi.advanceTimersByTime(1);});expect(calls(r,'/sessions/'+session)).toHaveLength(2);
    for(const delay of [4000,8000,16000,30000,30000]){const count=calls(r,'/sessions/'+session).length;await act(async()=>{vi.advanceTimersByTime(delay-1);});expect(calls(r,'/sessions/'+session)).toHaveLength(count);await act(async()=>{vi.advanceTimersByTime(1);});expect(calls(r,'/sessions/'+session)).toHaveLength(count+1);}
    v.rerender(<LivePokerTable {...r.p} mutation_blocked/>);r.setRead(async()=>ok(settled()));await act(async()=>{vi.advanceTimersByTime(1000);window.dispatchEvent(new Event('focus'));vi.advanceTimersByTime(0);});expect(r.p.onReturnLobby).toHaveBeenCalledTimes(1);expect(calls(r,'/safe-leave')).toHaveLength(1);
    expect((observed.current as PokerTableProps&{recovery_status?:string})?.recovery_status).toContain('已确认');
  });
  it.each(['B','WS-B','native-SID','account','viewer','logout','table','unmount'])('drops a delayed original A response before decode after %s',async kind=>{
    const r=await setup(),d=deferred<Response>();r.setRead(()=>d.promise);const v=await live(r);await leave();let decoded=0;const raw=settled();Object.defineProperty(raw,'state',{enumerable:true,get(){decoded++;return 'SETTLED';}});
    const response={ok:true,status:200,json:async()=>({success:true,data:raw})} as Response;
    if(kind==='B')v.rerender(<LivePokerTable {...r.p} admission={{...r.p.admission,snapshot:{...r.p.admission.snapshot,active_session:{...r.p.admission.snapshot.active_session!,session_id:other}}}}/>);
    else if(kind==='WS-B')act(()=>full(v.s,'PLAYER_SELF',8,true,other));else if(kind==='viewer')v.rerender(<LivePokerTable {...r.p} admission={{...r.p.admission,snapshot:{...r.p.admission.snapshot,active_session:null}}}/>);
    else if(kind==='native-SID'||kind==='account'){r.fetcher.mockResolvedValueOnce(ok({access_token:'synthetic-new',access_expires_at:4102444800,user:{id:kind==='account'?910003:910002,username:'synthetic',role:1},session:{sid:'synthetic-new'}}));await act(async()=>{await r.api.login('synthetic','synthetic');});}
    else if(kind==='logout')await act(async()=>{await r.api.logout();});else if(kind==='table')v.rerender(<LivePokerTable {...r.p} table_id={other}/>);else v.unmount();
    await act(async()=>{d.resolve(response);});expect(decoded).toBe(0);expect(r.p.onReturnLobby).not.toHaveBeenCalled();
  });
  it('old A finally neither decodes nor clears the new explicitly captured B singleflight',async()=>{
    const r=await setup(),a=deferred<Response>(),b=deferred<Response>();r.setRead(()=>a.promise);const original=r.fetcher.getMockImplementation()!;
    r.fetcher.mockImplementation((path,init)=>path.endsWith('/safe-leave')?Promise.resolve(ok({table_id:table,session_id:path.includes(other)?other:session,status:'LEAVE_REQUESTED',table_version:path.includes(other)?'10':'8',duplicate:false})):path.endsWith('/sessions/'+other)?b.promise:original(path,init));
    const v=await live(r);await leave();act(()=>full(v.s,'PLAYER_SELF',8));expect(observed.current?.authority.pending).toBe(false);
    v.rerender(<LivePokerTable {...r.p} admission={{...r.p.admission,snapshot:{...r.p.admission.snapshot,active_session:{...r.p.admission.snapshot.active_session!,session_id:other}}}}/>);act(()=>full(v.s,'PLAYER_SELF',9,true,other));await leave();expect(calls(r,'/sessions/'+other)).toHaveLength(1);
    let decoded=0;const stale=settled();Object.defineProperty(stale,'state',{enumerable:true,get(){decoded++;return 'SETTLED';}});await act(async()=>{a.resolve({ok:true,status:200,json:async()=>({success:true,data:stale})} as Response);});expect(decoded).toBe(0);expect(screen.queryByRole('button',{name:'核对原会话出金'})).not.toBeInTheDocument();expect(r.p.onReturnLobby).not.toHaveBeenCalled();
    await act(async()=>{b.resolve(ok({...settled(),session_id:other}));});expect(r.p.onReturnLobby).toHaveBeenCalledTimes(1);expect(calls(r,'/sessions/'+other)).toHaveLength(1);
  });
  it('rejected preflight and an unrelated original pending never latch exit or infer return',async()=>{
    const r=await setup();await live(r);const c=context();act(()=>observed.current!.onIntent({type:'leave',return_to_lobby:true},{...c,table_version:'0'}));await act(async()=>{});expect(calls(r,'/sessions/'+session)).toHaveLength(0);
    fireEvent.click(screen.getByRole('button',{name:/跟注 .* Chips/}));act(()=>{observed.current!.onIntent({type:'leave',return_to_lobby:true},context());observed.current!.onIntent({type:'return_lobby'},context());});await act(async()=>{});expect(calls(r,'/safe-leave')).toHaveLength(0);expect(calls(r,'/sessions/'+session)).toHaveLength(0);expect(r.p.onReturnLobby).not.toHaveBeenCalled();
  });
});
