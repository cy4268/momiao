import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { createElement, StrictMode } from 'react';
import { renderToString } from 'react-dom/server';
import { ApiClient } from '../api';
import { LivePokerLobby, type LivePokerLobbyProps } from './LivePokerLobby';
import type { PokerLobbyProps } from './poker-ui-types';
import { parsePokerLobbySnapshot } from './poker-lobby-read';
import lobby from './fixtures/l1-lobby.synthetic.json';

const observed=vi.hoisted(()=>({current:null as PokerLobbyProps|null}));
// Observe captured callbacks while exercising the actual pure UI and real HTTP boundary.
vi.mock('./PokerLobby',async original=>{const actual=await original<typeof import('./PokerLobby')>();return{PokerLobby:(p:PokerLobbyProps)=>{observed.current=p;return createElement(actual.PokerLobby,p);}};});
afterEach(()=>{cleanup();vi.useRealTimers();vi.restoreAllMocks();vi.unstubAllGlobals();observed.current=null;});
const table=lobby.tables[0].table_id,reservation='019a0000-0000-7000-8000-000000000031',session='019a0000-0000-7000-8000-000000000041';
const ok=(data:unknown)=>new Response(JSON.stringify({success:true,data}),{headers:{'Content-Type':'application/json'}});
const bundle=(sid='native-a',user=910001)=>({access_token:'synthetic-native',access_expires_at:4102444800,user:{id:user,username:'synthetic',role:1},session:{sid}});
function deferred<T>(){let resolve!:(value:T)=>void;return{promise:new Promise<T>(r=>resolve=r),resolve};}
const whole=()=>parsePokerLobbySnapshot(lobby,'910001');
async function setup(){
  const fetcher=vi.fn<(path:string,init?:RequestInit)=>Promise<Response>>().mockResolvedValueOnce(ok(bundle()));
  const api=new ApiClient(fetcher);await api.login('synthetic','synthetic');fetcher.mockImplementation(()=>Promise.resolve(ok(whole())));
  const p:LivePokerLobbyProps={client:api,policy:{stage:'READY',recovery_only:false,mutation_blocked:false},onEnter:vi.fn(),onNavigate:vi.fn(),onPendingChange:vi.fn()};
  return{api,fetcher,p};
}
type Runtime=Awaited<ReturnType<typeof setup>>;
const reads=(r:Runtime)=>r.fetcher.mock.calls.filter(([path,init])=>path.startsWith('/api/v1/poker')&&init?.method==='GET');
const writes=(r:Runtime)=>r.fetcher.mock.calls.filter(([path,init])=>init?.method==='POST'&&(/\/tables$|\/seat-reservations$|\/buy-ins$/.test(path)));
const receipt=(kind:'create'|'reserve'|'buyin')=>({table_id:table,table_version:'9',duplicate:false,...(kind==='create'?{status:'WAITING'}:kind==='reserve'?{status:'LEASE_ACTIVE',reservation_id:reservation}:{status:'CONFIRMED',session_id:session,funding_operation_id:'019a0000-0000-7000-8000-000000000051',amount_units:'200000000'})});
const lease=()=>({user_id:'910001',table_id:table,reservation_id:reservation,state:'FOUND',reservation:{seat_no:3,durable_state:'LEASE_ACTIVE',expires_at:'2026-09-06T12:00:30Z',checked_at:'2026-09-06T12:00:00Z',valid:true}});
function seated(){const s=whole();s.active_session={session_id:session,table_id:table,table_name:'月光长廊',state:'ACTIVE',seat_no:3,stack_units:'200000000',committed_units:'0',poker_in_play_units:'200000000',small_blind_units:'2500000',big_blind_units:'5000000',ante_units:'0',can_reconnect:true};s.viewer.poker_in_play_units='200000000';return s;}
async function openCreate(){fireEvent.click(screen.getByRole('button',{name:'创建牌桌'}));fireEvent.change(screen.getByLabelText('牌桌名称'),{target:{value:'月下新桌'}});}
const confirmCreate=()=>fireEvent.click(screen.getByRole('button',{name:'确认创建牌桌'}));
async function prepare(r:Runtime,kind:'create'|'reserve'|'buyin'){
  const view=await live(r);
  if(kind==='create')await openCreate();
  else{
    fireEvent.click(screen.getAllByRole('button',{name:/查看座位/})[0]);
    if(kind==='buyin'){fireEvent.click(screen.getByRole('button',{name:'预留 3 号座位'}));await waitFor(()=>expect(screen.getByRole('button',{name:'确认买入并等待大盲'})).toBeEnabled());}
  }
  return{...view,submit:()=>fireEvent.click(screen.getByRole('button',{name:kind==='create'?'确认创建牌桌':kind==='reserve'?'预留 3 号座位':'确认买入并等待大盲'}))};
}
const kindFor=(path:string)=>path.endsWith('/tables')?'create':path.endsWith('/seat-reservations')?'reserve':'buyin';
async function live(r:Runtime){const view=render(<LivePokerLobby {...r.p}/>);await screen.findByText('HTTP 已更新');return view;}

describe('Live Lobby whole-read owner',()=>{
  it('loads the real whole DTO instead of leaving the authenticated lobby inert',async()=>{
    const r=await setup(),response=deferred<Response>();r.fetcher.mockImplementation(()=>response.promise.then(value=>value.clone()));
    renderToString(<LivePokerLobby {...r.p}/>);expect(reads(r)).toHaveLength(0);
    render(<StrictMode><LivePokerLobby {...r.p}/></StrictMode>);await act(async()=>{});
    expect(reads(r).length).toBeGreaterThan(0);expect(observed.current).toBeNull();
    await act(async()=>response.resolve(ok(whole())));expect(await screen.findByText('月光长廊')).toBeInTheDocument();
    expect(reads(r).at(-1)?.[0]).toBe('/api/v1/poker');expect(r.p.onPendingChange).not.toHaveBeenCalled();
    expect(r.fetcher.mock.calls.filter(([path,init])=>path.startsWith('/api/v1/poker')&&init?.method==='POST')).toHaveLength(0);
  });
});

describe('Live Lobby original entry pipeline',()=>{
  it('binds create → actual empty seats → reserve → real lease → buyin → original-session admission',async()=>{
    const r=await setup(),create=deferred<Response>(),target=deferred<Response>(),reserve=deferred<Response>(),reservationRead=deferred<Response>(),buyin=deferred<Response>(),admission=deferred<Response>();
    let afterBuyin=false;
    r.fetcher.mockImplementation((path,init)=>{
      if(init?.method==='GET')return path.includes('?')?target.promise:afterBuyin?admission.promise:Promise.resolve(ok(whole()));
      expect(r.p.onPendingChange).toHaveBeenLastCalledWith(true);
      if(path.endsWith('/tables'))return create.promise;
      if(path.endsWith('/seat-reservations'))return reserve.promise;
      if(path.endsWith('/reservation-query'))return reservationRead.promise;
      if(path.endsWith('/buy-ins')){afterBuyin=true;return buyin.promise;}
      throw Error('unexpected request '+path);
    });
    const uuid=vi.spyOn(crypto,'randomUUID').mockReturnValueOnce('00000000-0000-4000-8000-000000000001').mockReturnValueOnce('00000000-0000-4000-8000-000000000002').mockReturnValueOnce('00000000-0000-4000-8000-000000000003');
    await live(r);await openCreate();
    const form=screen.getByRole('button',{name:'确认创建牌桌'}).closest('form')!;
    act(()=>{fireEvent.submit(form);fireEvent.submit(form);});
    expect(writes(r)).toHaveLength(1);expect(uuid).toHaveBeenCalledTimes(1);expect(r.p.onPendingChange).toHaveBeenCalledExactlyOnceWith(true);
    expect(JSON.parse(writes(r)[0][1]!.body as string)).toEqual({request_id:'00000000-0000-4000-8000-000000000001',name:'月下新桌',access_mode:'PUBLIC',blind_preset:'5-10',max_seats:6,allow_spectators:true,chat_enabled:false});
    expect(reads(r)).toHaveLength(1);expect(screen.getByRole('button',{name:'收起'})).toBeDisabled();
    await act(async()=>create.resolve(ok(receipt('create'))));expect(writes(r)).toHaveLength(1);
    expect(screen.getByLabelText('搜索牌桌')).toHaveValue(table);expect(reads(r).at(-1)?.[0]).toContain('q='+table);
    const targetWhole=whole();targetWhole.tables=[{...targetWhole.tables[0],open_seat_numbers:[3,5]}];
    await act(async()=>target.resolve(ok(targetWhole)));
    expect(screen.getByRole('button',{name:'预留 3 号座位'})).toBeEnabled();expect(screen.getByRole('button',{name:'预留 4 号座位'})).toBeDisabled();
    fireEvent.click(screen.getByRole('button',{name:'预留 3 号座位'}));expect(writes(r)).toHaveLength(2);
    await act(async()=>reserve.resolve(ok(receipt('reserve'))));expect(screen.getByRole('button',{name:'确认买入并等待大盲'})).toBeDisabled();
    expect(r.fetcher.mock.calls.find(([path])=>path.endsWith('/reservation-query'))?.[1]?.body).toBe(JSON.stringify({reservation_id:reservation}));
    await act(async()=>reservationRead.resolve(ok(lease())));expect(screen.getByRole('button',{name:'确认买入并等待大盲'})).toBeEnabled();
    expect(screen.getByText('2026-09-06T12:00:30Z')).toBeInTheDocument();fireEvent.click(screen.getByRole('button',{name:'确认买入并等待大盲'}));
    expect(writes(r)).toHaveLength(3);expect(JSON.parse(writes(r)[2][1]!.body as string)).toEqual({request_id:'00000000-0000-4000-8000-000000000003',reservation_id:reservation,amount_units:'200000000'});
    for(const [,init] of writes(r)){expect(new Headers(init?.headers).get('Authorization')).toBe('Bearer synthetic-native');expect(new Headers(init?.headers).get('X-Auth-Session')).toBe('native-a');expect(new Headers(init?.headers).get('New-Api-User')).toBe('910001');}
    await act(async()=>buyin.resolve(ok(receipt('buyin'))));expect(r.p.onEnter).not.toHaveBeenCalled();expect(r.p.onPendingChange).toHaveBeenLastCalledWith(true);
    await act(async()=>admission.resolve(ok(seated())));expect(r.p.onEnter).toHaveBeenCalledExactlyOnceWith(table,{snapshot:seated(),scope:{user_id:'910001',session_generation:r.api.getSessionGeneration(),request_generation:3,query_key:'/api/v1/poker'}});
    expect(r.p.onPendingChange).toHaveBeenLastCalledWith(false);expect(uuid).toHaveBeenCalledTimes(3);expect(writes(r)).toHaveLength(3);
    expect(r.fetcher.mock.calls.some(([path])=>path===`/api/v1/poker/tables/${table}`)).toBe(false);
  });
  it.each(['create','reserve','buyin'] as const)('retains %s timeout through filters / NOT_FOUND and retries only the original command',async kind=>{
    const r=await setup();let attempts=0,confirmed=false;
    r.fetcher.mockImplementation((path,init)=>{
      if(init?.method==='GET')return Promise.resolve(ok(confirmed?seated():whole()));
      if(path.endsWith('/reservation-query'))return Promise.resolve(ok(lease()));
      if(path.endsWith('/entry-receipt-query')){const body=JSON.parse(init!.body as string);return Promise.resolve(ok({user_id:'910001',kind:body.kind,mutation_id:body.mutation_id,state:'NOT_FOUND'}));}
      const actual=kindFor(path);if(actual===kind&&++attempts===1)return Promise.reject(Error('lost ACK'));
      if(actual==='buyin')confirmed=true;return Promise.resolve(ok(receipt(actual)));
    });
    const uuid=vi.spyOn(crypto,'randomUUID'),view=await prepare(r,kind);view.submit();await screen.findByRole('button',{name:'查询原操作回执'});
    await waitFor(()=>expect(screen.getByRole('button',{name:'查询原操作回执'})).toBeEnabled());
    const original=writes(r).at(-1)!,keyCount=uuid.mock.calls.length,pendingCalls=vi.mocked(r.p.onPendingChange).mock.calls.length;
    expect(screen.getByRole('button',{name:'收起'})).toBeDisabled();expect(screen.getByRole('button',{name:'查看钱包 ↗'})).toBeDisabled();
    fireEvent.change(screen.getByLabelText('搜索牌桌'),{target:{value:'另一筛选'}});await act(async()=>{});
    expect(r.p.onPendingChange).toHaveBeenCalledTimes(pendingCalls);expect(uuid).toHaveBeenCalledTimes(keyCount);
    fireEvent.click(screen.getByRole('button',{name:'查询原操作回执'}));await screen.findByText(/原回执尚未读到/);
    expect(r.p.onPendingChange).toHaveBeenLastCalledWith(true);expect(attempts).toBe(1);
    fireEvent.click(screen.getByRole('button',{name:'重试原请求'}));await waitFor(()=>expect(attempts).toBe(2));await act(async()=>{});
    expect(writes(r).at(-1)?.[0]).toBe(original[0]);expect(writes(r).at(-1)?.[1]?.body).toBe(original[1]?.body);expect(uuid).toHaveBeenCalledTimes(keyCount);
    expect(r.fetcher.mock.calls.filter(([path])=>path.endsWith('/entry-receipt-query'))).toHaveLength(2);
    expect(r.p.onPendingChange).toHaveBeenLastCalledWith(false);
  });
  it('blocks a write still waiting on native refresh when the latest soft policy is loading',async()=>{
    const r=await setup(),refresh=deferred<Response>();r.fetcher.mockImplementation((path)=>path.endsWith('/refresh')?refresh.promise:Promise.resolve(ok(whole())));
    const view=await prepare(r,'create');vi.spyOn(Date,'now').mockReturnValue(4102444790000);view.submit();await act(async()=>{});
    expect(writes(r)).toHaveLength(0);expect(r.p.onPendingChange).toHaveBeenLastCalledWith(true);
    view.rerender(<LivePokerLobby {...r.p} policy={{stage:undefined,recovery_only:true,mutation_blocked:true}}/>);
    await act(async()=>refresh.resolve(ok(bundle())));expect(writes(r)).toHaveLength(0);
    expect(r.p.onPendingChange).toHaveBeenLastCalledWith(true);view.rerender(<LivePokerLobby {...r.p}/>);await act(async()=>{});expect(writes(r)).toHaveLength(0);
  });
});

describe('Live Lobby read / handoff boundaries',()=>{
  it('ignores out-of-order whole replies and disables stale callbacks after a failed refresh',async()=>{
    const r=await setup();await live(r);const old=observed.current!,first=deferred<Response>(),second=deferred<Response>();
    r.fetcher.mockImplementationOnce(()=>first.promise).mockImplementationOnce(()=>second.promise);
    fireEvent.change(screen.getByLabelText('搜索牌桌'),{target:{value:'old-query'}});
    fireEvent.change(screen.getByLabelText('搜索牌桌'),{target:{value:'new-query'}});
    const next=whole();next.tables=[{...next.tables[0],name:'最新整包'}];next.viewer.available_chips_units='123456789';
    await act(async()=>second.resolve(ok(next)));await act(async()=>first.resolve(ok(whole())));
    expect(screen.getByText('最新整包')).toBeInTheDocument();expect(screen.queryByText('月光长廊')).not.toBeInTheDocument();expect(observed.current?.snapshot.viewer.available_chips_units).toBe('123456789');
    act(()=>old.onIntent({type:'open_table',table_id:table},old.authority.scope));expect(screen.queryByRole('complementary',{name:'座位与买入'})).not.toBeInTheDocument();
    r.fetcher.mockRejectedValueOnce(Error('offline'));fireEvent.click(screen.getByRole('button',{name:'刷新列表'}));await screen.findByText('HTTP 读取失败');
    expect(screen.getByText('最新整包')).toBeInTheDocument();expect(screen.getByRole('button',{name:'创建牌桌'})).toBeDisabled();expect(writes(r)).toHaveLength(0);
  });
  it.each(['reconnect','spectate'] as const)('requires fresh exact %s admission, not old UI facts, while policy blocks new entry',async kind=>{
    const r=await setup(),original=kind==='reconnect'?seated():whole();r.fetcher.mockResolvedValueOnce(ok(original));
    r.p.policy={stage:'MAINTENANCE',recovery_only:true,mutation_blocked:true};const view=await live(r),response=deferred<Response>();r.fetcher.mockReturnValueOnce(response.promise);
    fireEvent.click(screen.getByRole('button',{name:kind==='reconnect'?'回到当前牌桌':'观战'}));expect(r.p.onEnter).not.toHaveBeenCalled();
    const latestEnter=vi.fn();view.rerender(<LivePokerLobby {...r.p} onEnter={latestEnter}/>);
    await act(async()=>response.resolve(ok(original)));
    expect(latestEnter).toHaveBeenCalledExactlyOnceWith(table,{snapshot:original,scope:expect.objectContaining({user_id:'910001',session_generation:r.api.getSessionGeneration(),request_generation:2})});
    expect(r.p.onEnter).not.toHaveBeenCalled();expect(writes(r)).toHaveLength(0);
  });
  it.each(['reconnect','spectate'] as const)('does not substitute a changed session into original %s intent',async kind=>{
    const r=await setup();r.fetcher.mockResolvedValueOnce(ok(kind==='reconnect'?seated():whole()));await live(r);
    const changed=seated();changed.active_session!.session_id='019a0000-0000-7000-8000-000000000042';r.fetcher.mockResolvedValueOnce(ok(changed));
    fireEvent.click(screen.getByRole('button',{name:kind==='reconnect'?'回到当前牌桌':'观战'}));await act(async()=>{});expect(r.p.onEnter).not.toHaveBeenCalled();expect(writes(r)).toHaveLength(0);
  });
  it('uses only the fixed navigation callbacks and current captured owner',async()=>{
    const r=await setup(),view=await live(r),old=observed.current!,next=vi.fn();view.rerender(<LivePokerLobby {...r.p} onNavigate={next}/>);
    act(()=>{old.onIntent({type:'wallet'},old.authority.scope);old.onIntent({type:'history'},old.authority.scope);old.onIntent({type:'rewards'},old.authority.scope);});
    expect(next.mock.calls).toEqual([['wallet'],['history'],['rewards']]);expect(r.p.onNavigate).not.toHaveBeenCalled();view.unmount();
    act(()=>old.onIntent({type:'wallet'},old.authority.scope));expect(next).toHaveBeenCalledTimes(3);
  });
});

describe('Live Lobby preserved ACK and latest retry conditions',()=>{
  it('continues a create ACK when manual pagination finally returns its real row, without resetting back to page one',async()=>{
    const r=await setup();let created=false;
    r.fetcher.mockImplementation((path,init)=>{
      if(init?.method==='POST'){created=true;return Promise.resolve(ok(receipt('create')));}
      const s=whole();if(created&&!path.includes('cursor=')){s.tables=[{...s.tables[1],name:table}];s.page.next_cursor='eyJ2IjoyfQ';}return Promise.resolve(ok(s));
    });
    const view=await prepare(r,'create');view.submit();await waitFor(()=>expect(screen.getByRole('button',{name:'继续核对原结果'})).toBeEnabled());
    expect(r.p.onPendingChange).toHaveBeenLastCalledWith(true);fireEvent.click(screen.getByRole('button',{name:'下一页'}));
    await waitFor(()=>expect(screen.getByRole('button',{name:'预留 3 号座位'})).toBeEnabled());expect(r.p.onPendingChange).toHaveBeenLastCalledWith(false);
    expect(reads(r).at(-1)?.[0]).toContain('cursor=eyJ2IjoyfQ');expect(writes(r)).toHaveLength(1);
  });
  it('keeps an already sent create ACK during maintenance and never replays it after READY returns',async()=>{
    const r=await setup(),response=deferred<Response>();r.fetcher.mockImplementation((path,init)=>init?.method==='GET'?Promise.resolve(ok(whole())):response.promise);
    const view=await prepare(r,'create');view.submit();expect(writes(r)).toHaveLength(1);
    view.rerender(<LivePokerLobby {...r.p} policy={{stage:'MAINTENANCE',recovery_only:true,mutation_blocked:true}}/>);
    await act(async()=>response.resolve(ok(receipt('create'))));expect(screen.getByRole('region',{name:'原入桌操作'})).toHaveTextContent('WAITING');
    expect(r.p.onPendingChange).toHaveBeenLastCalledWith(false);expect(screen.getByRole('button',{name:'预留 3 号座位'})).toBeDisabled();
    view.rerender(<LivePokerLobby {...r.p}/>);await act(async()=>{});expect(writes(r)).toHaveLength(1);
  });
  it.each(['null','other-session','read-error'] as const)('holds CONFIRMED buyin for %s and later admits only its original session',async mode=>{
    const r=await setup();let after=false,correct=false;
    r.fetcher.mockImplementation((path,init)=>{
      if(init?.method==='GET'){
        if(!after)return Promise.resolve(ok(whole()));if(correct)return Promise.resolve(ok(seated()));if(mode==='read-error')return Promise.reject(Error('read failed'));
        const s=mode==='null'?whole():seated();if(s.active_session)s.active_session.session_id='019a0000-0000-7000-8000-000000000042';return Promise.resolve(ok(s));
      }
      if(path.endsWith('/reservation-query'))return Promise.resolve(ok(lease()));
      const kind=kindFor(path);if(kind==='buyin')after=true;return Promise.resolve(ok(receipt(kind)));
    });
    const view=await prepare(r,'buyin');view.submit();await waitFor(()=>expect(screen.getByRole('button',{name:'继续核对原结果'})).toBeEnabled());
    expect(r.p.onEnter).not.toHaveBeenCalled();expect(r.p.onPendingChange).toHaveBeenLastCalledWith(true);expect(screen.queryByRole('button',{name:'重试原请求'})).not.toBeInTheDocument();
    correct=true;fireEvent.click(screen.getByRole('button',{name:'继续核对原结果'}));await waitFor(()=>expect(r.p.onEnter).toHaveBeenCalledTimes(1));expect(writes(r)).toHaveLength(2);
  });
  it('uses FOUND original receipts without another mutation and treats FAILED_NO_EFFECT as terminal',async()=>{
    const r=await setup();r.fetcher.mockImplementation((path,init)=>{
      if(init?.method==='GET')return Promise.resolve(ok(whole()));if(path.endsWith('/reservation-query'))return Promise.resolve(ok(lease()));
      if(path.endsWith('/entry-receipt-query')){const body=JSON.parse(init!.body as string);return Promise.resolve(ok({user_id:'910001',kind:'buyin',mutation_id:body.mutation_id,state:'FOUND',receipt:{table_id:table,table_version:'9',duplicate:true,status:'FAILED_NO_EFFECT',failure_code:'WALLET_INSUFFICIENT'}}));}
      return path.endsWith('/buy-ins')?Promise.reject(Error('lost')):Promise.resolve(ok(receipt('reserve')));
    });
    const view=await prepare(r,'buyin');view.submit();await waitFor(()=>expect(screen.getByRole('button',{name:'查询原操作回执'})).toBeEnabled());
    fireEvent.click(screen.getByRole('button',{name:'查询原操作回执'}));await waitFor(()=>expect(r.p.onPendingChange).toHaveBeenLastCalledWith(false));
    expect(screen.getByRole('region',{name:'原入桌操作'})).toHaveTextContent('FAILED_NO_EFFECT');expect(writes(r)).toHaveLength(2);expect(r.p.onEnter).not.toHaveBeenCalled();
  });
  it.each([
    ['create','owned'],['create','preset'],['create','active'],['create','maintenance'],
    ['reserve','seat'],['reserve','row'],['reserve','join'],['buyin','wallet'],['buyin','min'],['buyin','max'],['buyin','lease'],
  ] as const)('rechecks %s retry against latest %s instead of the original render',async(kind,condition)=>{
    const r=await setup();let checking=false;
    r.fetcher.mockImplementation((path,init)=>{
      if(init?.method==='GET'){
        const s=condition==='active'&&checking?seated():whole();
        if(checking){if(condition==='owned')s.viewer.owned_open_table_id=table;if(condition==='preset')s.blind_presets=[];if(condition==='maintenance')s.service.state='MAINTENANCE';if(condition==='seat')s.tables[0].open_seat_numbers=[4,5];if(condition==='row')s.tables=[];if(condition==='join')s.tables[0].can_join=false;if(condition==='wallet')s.viewer.available_chips_units='199999999';if(condition==='min')s.tables[0].minimum_buyin_units='250000000';if(condition==='max')s.tables[0].maximum_buyin_units='499500000';}
        return Promise.resolve(ok(s));
      }
      if(path.endsWith('/reservation-query')){const value=lease();if(checking&&condition==='lease'){value.reservation.valid=false;value.reservation.durable_state='EXPIRED';}return Promise.resolve(ok(value));}
      if(path.endsWith('/entry-receipt-query')){const body=JSON.parse(init!.body as string);return Promise.resolve(ok({user_id:'910001',kind:body.kind,mutation_id:body.mutation_id,state:'NOT_FOUND'}));}
      return kindFor(path)===kind?Promise.reject(Error('lost')):Promise.resolve(ok(receipt('reserve')));
    });
    const view=await prepare(r,kind);if(condition==='max')fireEvent.click(screen.getByRole('button',{name:'最高买入'}));view.submit();await waitFor(()=>expect(screen.getByRole('button',{name:'查询原操作回执'})).toBeEnabled());const count=writes(r).length;
    fireEvent.click(screen.getByRole('button',{name:'查询原操作回执'}));await waitFor(()=>expect(screen.getByRole('button',{name:'重试原请求'})).toBeEnabled());
    checking=true;fireEvent.click(screen.getByRole('button',{name:'重试原请求'}));await waitFor(()=>expect(screen.getByRole('button',{name:'查询原操作回执'})).toBeEnabled());
    expect(writes(r)).toHaveLength(count);expect(r.p.onPendingChange).toHaveBeenLastCalledWith(true);
  });
});

describe('Live Lobby lifecycle and lease observation',()=>{
  it('retires entry callbacks when an explicit flow is replaced even if the same whole read stays current',async()=>{
    const r=await setup();await live(r);fireEvent.click(screen.getAllByRole('button',{name:/查看座位/})[0]);const old=observed.current!;
    fireEvent.click(screen.getByRole('button',{name:'收起'}));fireEvent.click(screen.getAllByRole('button',{name:/查看座位/})[0]);
    act(()=>old.onIntent({type:'reserve',table_id:table,seat_no:3},old.authority.scope));expect(writes(r)).toHaveLength(0);expect(r.p.onPendingChange).not.toHaveBeenCalled();
  });
  it('invalidates an outstanding valid query when a subsequent filter value is rejected',async()=>{
    const r=await setup();await live(r);const old=deferred<Response>();r.fetcher.mockReturnValueOnce(old.promise);
    fireEvent.change(screen.getByLabelText('搜索牌桌'),{target:{value:'valid-old'}});fireEvent.change(screen.getByLabelText('搜索牌桌'),{target:{value:'x'.repeat(41)}});
    await act(async()=>old.resolve(ok(whole())));expect(screen.getByText('HTTP 读取失败')).toBeInTheDocument();expect(screen.getByRole('button',{name:'创建牌桌'})).toBeDisabled();
  });
  it('retains the original lease query target after a manual read failure, without treating it as live',async()=>{
    const r=await setup();r.fetcher.mockImplementation((path,init)=>Promise.resolve(ok(init?.method==='GET'?whole():path.endsWith('/reservation-query')?lease():receipt('reserve'))));
    await prepare(r,'buyin');r.fetcher.mockRejectedValueOnce(Error('offline'));fireEvent.click(screen.getByRole('button',{name:'查询当前预留'}));
    await waitFor(()=>expect(screen.getByRole('button',{name:'确认买入并等待大盲'})).toBeDisabled());expect(screen.getByRole('button',{name:'查询当前预留'})).toBeEnabled();
    fireEvent.click(screen.getByRole('button',{name:'查询当前预留'}));await waitFor(()=>expect(screen.getByRole('button',{name:'确认买入并等待大盲'})).toBeEnabled());
    expect(r.fetcher.mock.calls.filter(([path])=>path.endsWith('/reservation-query')).map(([,init])=>init?.body)).toEqual(Array(3).fill(JSON.stringify({reservation_id:reservation})));expect(writes(r)).toHaveLength(1);
  });
  it.each(['success','error'] as const)('drops old-client late %s without decoding the old DTO or clearing new pending',async outcome=>{
    const first=await setup(),late=deferred<Response>();first.fetcher.mockImplementation((path,init)=>init?.method==='GET'?Promise.resolve(ok(whole())):late.promise);
    const view=await prepare(first,'create'),old=observed.current!;view.submit();const next=await setup(),newLate=deferred<Response>();next.fetcher.mockImplementation((path,init)=>init?.method==='GET'?Promise.resolve(ok(whole())):newLate.promise);
    view.rerender(<LivePokerLobby {...next.p}/>);await screen.findByText('HTTP 已更新');await openCreate();confirmCreate();expect(next.p.onPendingChange).toHaveBeenLastCalledWith(true);
    const payload=receipt('create'),getter=vi.fn(()=>payload.table_id),guarded={...payload};Object.defineProperty(guarded,'table_id',{enumerable:true,get:getter});
    const response=ok({});vi.spyOn(response,'json').mockResolvedValue(outcome==='success'?{success:true,data:guarded}:{success:false,code:'OLD_ERROR'});
    await act(async()=>late.resolve(response));act(()=>old.onIntent({type:'wallet'},old.authority.scope));
    expect(getter).not.toHaveBeenCalled();expect(first.p.onPendingChange).toHaveBeenCalledExactlyOnceWith(true);expect(next.p.onPendingChange).toHaveBeenCalledExactlyOnceWith(true);
    expect(first.p.onEnter).not.toHaveBeenCalled();expect(next.p.onEnter).not.toHaveBeenCalled();expect(first.p.onNavigate).not.toHaveBeenCalled();expect(writes(next)).toHaveLength(1);view.unmount();
  });
  it.each(['account','sid','dispose'] as const)('blocks old entry dispatch after %s changes during native refresh',async boundary=>{
    const r=await setup(),refresh=deferred<Response>();r.fetcher.mockImplementation((path)=>path.endsWith('/refresh')?refresh.promise:Promise.resolve(ok(whole())));
    const view=await prepare(r,'create');vi.spyOn(Date,'now').mockReturnValue(4102444790000);view.submit();await act(async()=>{});
    if(boundary==='dispose')view.unmount();
    await act(async()=>refresh.resolve(ok(bundle(boundary==='sid'?'native-b':'native-a',boundary==='account'?910002:910001))));
    expect(writes(r)).toHaveLength(0);expect(r.p.onPendingChange).toHaveBeenCalledExactlyOnceWith(true);expect(r.p.onEnter).not.toHaveBeenCalled();
  });
  it('suppresses stale whole GET after native refresh while permitting the latest query only',async()=>{
    const r=await setup();await live(r);const refresh=deferred<Response>();r.fetcher.mockImplementation((path)=>path.endsWith('/refresh')?refresh.promise:Promise.resolve(ok(whole())));vi.spyOn(Date,'now').mockReturnValue(4102444790000);
    fireEvent.change(screen.getByLabelText('搜索牌桌'),{target:{value:'retired'}});fireEvent.change(screen.getByLabelText('搜索牌桌'),{target:{value:'current'}});
    await act(async()=>refresh.resolve(ok(bundle())));expect(reads(r)).toHaveLength(2);expect(reads(r).at(-1)?.[0]).toContain('q=current');expect(reads(r).some(([path])=>path.includes('q=retired'))).toBe(false);
  });
  it.each(['seat','owner','table','id','CONSUMED','EXPIRED','invalid','NOT_FOUND'] as const)('keeps reserve ACK but rejects %s as current buyin authority',async condition=>{
    const r=await setup();r.fetcher.mockImplementation((path,init)=>{
      if(init?.method==='GET')return Promise.resolve(ok(whole()));if(path.endsWith('/seat-reservations'))return Promise.resolve(ok(receipt('reserve')));
      const value=lease();if(condition==='seat')value.reservation.seat_no=4;if(condition==='owner')value.user_id='910002';if(condition==='table')value.table_id='019a0000-0000-7000-8000-000000000009';if(condition==='id')value.reservation_id='019a0000-0000-7000-8000-000000000032';
      if(condition==='CONSUMED'||condition==='EXPIRED'||condition==='invalid'){value.reservation.valid=false;if(condition!=='invalid')value.reservation.durable_state=condition;}
      return Promise.resolve(ok(condition==='NOT_FOUND'?{user_id:'910001',table_id:table,reservation_id:reservation,state:'NOT_FOUND'}:value));
    });
    const view=await prepare(r,'reserve');view.submit();await act(async()=>{});expect(screen.getByRole('button',{name:'确认买入并等待大盲'})).toBeDisabled();
    const p=observed.current!;act(()=>p.onIntent({type:'buyin',table_id:table,reservation_id:reservation,seat_no:3,amount_units:'200000000',entry_mode:'WAIT_FOR_BB'},p.authority.scope));
    expect(writes(r)).toHaveLength(1);expect(screen.getByRole('region',{name:'原入桌操作'})).toHaveTextContent('LEASE_ACTIVE');expect(r.p.onEnter).not.toHaveBeenCalled();
  });
  it('uses the original server deadline minus RTT and monotonic elapsed time, without timer I/O or keys',async()=>{
    vi.useFakeTimers({toFake:['setInterval','clearInterval']});let now=100;vi.spyOn(performance,'now').mockImplementation(()=>now);const uuid=vi.spyOn(crypto,'randomUUID');
    const r=await setup(),leaseRead=deferred<Response>();r.fetcher.mockImplementation((path,init)=>init?.method==='GET'?Promise.resolve(ok(whole())):path.endsWith('/reservation-query')?leaseRead.promise:Promise.resolve(ok(receipt('reserve'))));
    const view=await prepare(r,'reserve');view.submit();await act(async()=>{});now=2100;await act(async()=>leaseRead.resolve(ok(lease())));
    expect(screen.getByRole('region',{name:'服务端预留观察'})).toHaveTextContent('28 秒');expect(screen.getByRole('button',{name:'确认买入并等待大盲'})).toBeEnabled();
    const count=r.fetcher.mock.calls.length;vi.spyOn(Date,'now').mockReturnValue(0);now=31101;act(()=>vi.advanceTimersByTime(1000));
    expect(screen.getByRole('region',{name:'服务端预留观察'})).toHaveTextContent('0 秒');expect(screen.getByRole('button',{name:'确认买入并等待大盲'})).toBeDisabled();
    expect(r.fetcher).toHaveBeenCalledTimes(count);expect(uuid).toHaveBeenCalledTimes(1);expect(screen.getByText('2026-09-06T12:00:30Z')).toBeInTheDocument();
  });
  it.each(['create','reserve','buyin'] as const)('retains %s original key for both HTTP 503 and malformed positive acknowledgements',async kind=>{
    for(const failure of ['503','malformed']){
      const r=await setup();r.fetcher.mockImplementation((path,init)=>{
        if(init?.method==='GET')return Promise.resolve(ok(whole()));if(path.endsWith('/reservation-query'))return Promise.resolve(ok(lease()));
        if(kindFor(path)!==kind)return Promise.resolve(ok(receipt('reserve')));
        return Promise.resolve(failure==='503'?new Response(JSON.stringify({success:false,code:'TEMPORARY'}),{status:503}):ok({...receipt(kind),table_version:'0'}));
      });
      const view=await prepare(r,kind);view.submit();await waitFor(()=>expect(screen.getByRole('button',{name:'查询原操作回执'})).toBeEnabled());
      expect(screen.getByRole('region',{name:'原入桌操作'})).toHaveTextContent(JSON.parse(writes(r).at(-1)![1]!.body as string).request_id);expect(r.p.onPendingChange).toHaveBeenLastCalledWith(true);expect(r.p.onEnter).not.toHaveBeenCalled();view.unmount();
    }
  });
  it('rejects invalid monetary / reservation inputs at the owner despite invoking a saved callback directly',async()=>{
    const r=await setup();r.fetcher.mockImplementation((path,init)=>Promise.resolve(ok(init?.method==='GET'?whole():path.endsWith('/reservation-query')?lease():receipt('reserve'))));
    await prepare(r,'buyin');const p=observed.current!,uuid=vi.spyOn(crypto,'randomUUID');
    for(const amount of ['0','199500000','500500000','200000001','9007199254740993','2e8','-200000000'])act(()=>p.onIntent({type:'buyin',table_id:table,reservation_id:reservation,seat_no:3,amount_units:amount,entry_mode:'WAIT_FOR_BB'},p.authority.scope));
    act(()=>p.onIntent({type:'buyin',table_id:table,reservation_id:reservation,seat_no:4,amount_units:'200000000',entry_mode:'WAIT_FOR_BB'},p.authority.scope));
    expect(writes(r)).toHaveLength(1);expect(uuid).not.toHaveBeenCalled();expect(r.p.onEnter).not.toHaveBeenCalled();
  });
  it('does not mint an unknown operation for an invalid or superseded create draft',async()=>{
    const r=await setup();await prepare(r,'create');const old=observed.current!,uuid=vi.spyOn(crypto,'randomUUID');
    fireEvent.change(screen.getByLabelText('牌桌名称'),{target:{value:'最新草稿'}});
    for(const name of ['月下新桌','', 'x'.repeat(41), 'bad\u0001name'])act(()=>old.onIntent({type:'create',name,visibility:'PUBLIC',max_seats:6,blind_preset_id:'5-10',allow_spectators:true,chat_enabled:false},old.authority.scope));
    expect(writes(r)).toHaveLength(0);expect(uuid).not.toHaveBeenCalled();expect(r.p.onPendingChange).not.toHaveBeenCalled();
  });
  it('rechecks the current whole-chip draft when an older valid buyin callback is retained',async()=>{
    const r=await setup();r.fetcher.mockImplementation((path,init)=>Promise.resolve(ok(init?.method==='GET'?whole():path.endsWith('/reservation-query')?lease():receipt('reserve'))));
    await prepare(r,'buyin');const old=observed.current!,uuid=vi.spyOn(crypto,'randomUUID');fireEvent.click(screen.getByRole('button',{name:'最高买入'}));
    act(()=>old.onIntent({type:'buyin',table_id:table,reservation_id:reservation,seat_no:3,amount_units:'200000000',entry_mode:'WAIT_FOR_BB'},old.authority.scope));
    expect(writes(r)).toHaveLength(1);expect(uuid).not.toHaveBeenCalled();
  });
});
