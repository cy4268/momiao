import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom';
import { App } from './App';
import { ApiClient } from './api';
import { profile } from './m1-test-fixtures';
import lobby from './poker/fixtures/l1-lobby.synthetic.json';
import self from './poker/fixtures/g3-player-self.json';
import publicView from './poker/fixtures/g3-public.json';
import { peekRouteIntent } from './post-auth-intent';

const table=self.table_id,session=self.viewer.session_id,other='019a0000-0000-7000-8000-000000000011';
const ok=(data:unknown)=>new Response(JSON.stringify({success:true,data}));
const user={id:910002,username:'synthetic-route',display_name:'Synthetic',role:1};
const bundle=(sid='route-native')=>({access_token:'synthetic',access_expires_at:4102444800,user,session:{sid}});
class Socket extends EventTarget {
  static all:Socket[]=[];readyState=0;bufferedAmount=0;protocol='chaldea-poker.v1';sent:string[]=[];id=String(Socket.all.length+1).repeat(32);
  constructor(){super();Socket.all.push(this);}send(text:string){this.sent.push(text);}close(){this.readyState=3;this.dispatchEvent(new CloseEvent('close',{code:1000}));}
  message(data:string){this.dispatchEvent(new MessageEvent('message',{data}));}
}
afterEach(()=>{cleanup();vi.unstubAllGlobals();vi.restoreAllMocks();Socket.all=[];});
function whole(active=false):any {
  const raw:any=structuredClone(lobby);raw.viewer.user_id='910002';raw.tables[0].table_id=table;
  if(active){raw.viewer.poker_in_play_units='1500000';raw.active_session={session_id:session,table_id:table,table_name:'原牌桌',state:'ACTIVE',seat_no:2,stack_units:'1000000',committed_units:'500000',poker_in_play_units:'1500000',small_blind_units:'2500000',big_blind_units:'5000000',ante_units:'0',can_reconnect:true};}
  return raw;
}
async function setup(raw=whole()) {
  const f={sid:'route-native',raw,gateStage:'READY',read:()=>Promise.resolve(ok(raw)),respond:undefined as ((path:string,init?:RequestInit)=>Promise<Response>|undefined)|undefined,gateRead:undefined as ((route:string)=>Promise<Response>)|undefined};
  const fetcher=vi.fn(async(path:string,init?:RequestInit)=>{
    if(path==='/api/user/login'||path.includes('/auth/refresh'))return ok(bundle(f.sid));
    if(path==='/api/user/self')return ok(user);
    if(path.startsWith('/platform/v1/access-gate?')){const route=new URLSearchParams(path.split('?')[1]).get('route')!;return f.gateRead?.(route)??ok({user_id:String(user.id),route,stage:f.gateStage});}
    if(path==='/platform/v1/master-profile')return ok({...profile,user_id:String(user.id)});
    if(path==='/api/v1/poker'||path.startsWith('/api/v1/poker/tables?'))return f.read();
    if(path==='/api/v1/poker/connect-tickets')return ok({poker_connect_ticket:'ct1.eyJzeW50aGV0aWMiOnRydWV9.c2lnbmF0dXJl'});
    if(path.endsWith('/logout'))return ok({});
    const response=f.respond?.(path,init);if(response)return response;
    return new Response(JSON.stringify({success:false}),{status:403});
  });
  const client=new ApiClient(fetcher);await client.login('synthetic','synthetic');vi.stubGlobal('WebSocket',Socket);
  return{client,fetcher,f};
}
const pokerCalls=(r:Awaited<ReturnType<typeof setup>>)=>r.fetcher.mock.calls.filter(([p])=>p.startsWith('/api/v1/poker'));
const tickets=(r:Awaited<ReturnType<typeof setup>>)=>r.fetcher.mock.calls.filter(([p])=>p.endsWith('/connect-tickets'));

it('mounts the whole-Lobby owner under the ordinary Shell without creating a ticket',async()=>{
  const r=await setup();render(<MemoryRouter initialEntries={['/poker']}><App client={r.client}/></MemoryRouter>);
  await waitFor(()=>expect(pokerCalls(r).some(([p])=>p==='/api/v1/poker')).toBe(true));
  expect(await screen.findByRole('region',{name:'Poker 大厅'})).toBeVisible();
  expect(screen.getByRole('navigation',{name:'底部导航'})).toBeVisible();expect(tickets(r)).toHaveLength(0);
});

function Navigation(){const navigate=useNavigate(),location=useLocation();return <><output data-testid="path">{location.pathname}</output><button onClick={()=>navigate('/wallet')}>test wallet navigation</button><button onClick={()=>navigate(-1)}>test browser back</button><button onClick={()=>navigate(1)}>test browser forward</button><button onClick={()=>navigate('/poker')}>test lobby navigation</button></>;}
function mount(r:Awaited<ReturnType<typeof setup>>,path='/poker/table/'+table){return render(<MemoryRouter initialEntries={['/wallet',path]} initialIndex={1}><Navigation/><App client={r.client}/></MemoryRouter>);}
function activate(socket:Socket,player=true){
  const frame=(type:string,payload:unknown)=>JSON.stringify({type,event_id:'app-route-synthetic',event_seq:10,table_id:table,table_version:7,hand_id:self.hand.hand_id,hand_version:8,server_time:self.server_now,payload});
  socket.readyState=1;socket.dispatchEvent(new Event('open'));socket.message(frame('auth.accepted',{request_id:JSON.parse(socket.sent[0]).request_id,connection_id:socket.id}));
  const raw:any=structuredClone(player?self:publicView);raw.table_id=table;raw.server_now=self.server_now;raw.table_version='7';raw.hand.hand_version='8';raw.viewer.control={connection_id:socket.id,...(player?{session_id:session}:{}),mode:player?'CONTROLLER':'READ_ONLY',control_epoch:raw.viewer.control_epoch};
  socket.message(frame('table.snapshot',raw));
}
async function live(r:Awaited<ReturnType<typeof setup>>,player=true){const view=mount(r);await waitFor(()=>expect(Socket.all).toHaveLength(1));act(()=>activate(Socket.all[0],player));await screen.findByText(player?'安全离座':'返回大厅');return view;}
const active=()=>({session_id:session,table_id:table,table_name:'原牌桌',state:'ACTIVE',seat_no:2,stack_units:'1000000',committed_units:'500000',poker_in_play_units:'1500000',control_epoch:'1',initial_buy_in_units:'2000000',total_top_up_units:'0',started_at:'2026-09-06T00:00:00Z'});
const settled=()=>({...active(),state:'SETTLED',stack_units:'0',committed_units:'0',poker_in_play_units:'0',final_cash_out_units:'1500000',realized_pl_units:'-500000',ended_at:'2026-09-06T01:00:00Z',end_reason:'SAFE_LEAVE'});

const publicPoker=(state='PLAY')=>({items:[{slug:'texas-holdem',title:'目录中的 Poker',implementation_key:'poker.texas-holdem.v1',effective_runtime:state}]});
it.each(['READY','MAINTENANCE','CONFIG_INCOMPLETE','RESOURCE_UNAVAILABLE'])('passes a catalog click through the actual Gate and fresh Lobby: %s',async state=>{
  const raw=whole();
  if(state==='MAINTENANCE'){raw.service={state,production_ready:true,blockers:['MAINTENANCE_ACTIVE'],maintenance_scopes:['POKER_NEW_TABLES_NEW_HANDS']};raw.viewer.can_create=false;raw.viewer.can_join=false;raw.tables.forEach((row:any)=>row.can_join=false);}
  if(state==='CONFIG_INCOMPLETE'){raw.service={state,production_ready:false,blockers:['POKER_RULESET_INCOMPLETE'],maintenance_scopes:[]};raw.ruleset=null;raw.viewer.can_create=false;raw.viewer.can_join=false;raw.tables.forEach((row:any)=>{row.can_join=false;row.can_spectate=false;});}
  const r=await setup(raw);r.f.gateStage=state==='CONFIG_INCOMPLETE'?'READY':state;
  r.f.respond=path=>path==='/api/v1/games'?Promise.resolve(ok(publicPoker(state==='MAINTENANCE'?'MAINTENANCE':'PLAY'))):undefined;
  render(<MemoryRouter initialEntries={['/games']}><Navigation/><App client={r.client}/></MemoryRouter>);
  await screen.findByRole('heading',{name:'目录中的 Poker'});expect(pokerCalls(r)).toHaveLength(0);
  fireEvent.click(screen.getByRole('link',{name:state==='MAINTENANCE'?'查看大厅与恢复牌局 →':'进入 Poker 大厅 →'}));
  await waitFor(()=>expect(r.fetcher.mock.calls.some(([p])=>p==='/platform/v1/access-gate?route=%2Fpoker')).toBe(true));
  if(state==='RESOURCE_UNAVAILABLE'){await screen.findByText(/目标功能当前未开放/);expect(pokerCalls(r)).toHaveLength(0);}
  else {await screen.findByRole('region',{name:'Poker 大厅'});expect(pokerCalls(r).map(([p])=>p)).toEqual(['/api/v1/poker']);expect(screen.getByRole('button',{name:'创建牌桌'}).hasAttribute('disabled')).toBe(state!=='READY');}
  expect(tickets(r)).toHaveLength(0);expect(Socket.all).toHaveLength(0);
  expect(pokerCalls(r).filter(([,init])=>init?.method!=='GET')).toHaveLength(0);
});

it('saves only the guest catalog destination and returns through the original post-auth Gate',async()=>{
  sessionStorage.clear();const paths:string[]=[];let signedIn=false;
  const client=new ApiClient(async(path)=>{
    paths.push(path);
    if(path.includes('/auth/refresh'))return new Response(JSON.stringify({success:false}),{status:401});
    if(path==='/api/user/login'){signedIn=true;return ok(bundle());}
    if(path==='/api/user/self')return ok(user);
    if(path==='/api/v1/games')return ok(publicPoker());
    if(path.startsWith('/platform/v1/access-gate?'))return ok({user_id:String(user.id),route:new URLSearchParams(path.split('?')[1]).get('route'),stage:'READY'});
    if(path==='/platform/v1/master-profile')return ok({...profile,user_id:String(user.id)});
    if(path==='/api/v1/poker')return ok(whole());
    return new Response(JSON.stringify({success:false}),{status:403});
  });
  vi.stubGlobal('WebSocket',Socket);
  render(<MemoryRouter initialEntries={['/entertainment']}><Navigation/><App client={client}/></MemoryRouter>);
  await screen.findByRole('heading',{name:'目录中的 Poker'});fireEvent.click(screen.getByRole('link',{name:'进入 Poker 大厅 →'}));
  await waitFor(()=>expect(screen.getByTestId('path').textContent).toBe('/login'));
  expect(peekRouteIntent()).toBe('/poker');expect(signedIn).toBe(false);expect(paths.some(p=>p.startsWith('/api/v1/poker'))).toBe(false);
  await act(async()=>{await client.login('synthetic','synthetic');});
  await screen.findByRole('region',{name:'Poker 大厅'});expect(screen.getByTestId('path').textContent).toBe('/poker');
  expect(paths.filter(p=>p.startsWith('/api/v1/poker'))).toEqual(['/api/v1/poker']);expect(Socket.all).toHaveLength(0);
  expect(peekRouteIntent()).toBe('/dashboard');sessionStorage.clear();
});
it.each(['test wallet navigation','test browser back','test lobby navigation'])('keeps the same player owner through %s until a real safe exit',async name=>{
  const r=await setup(whole(true));await live(r);const socket=Socket.all[0];fireEvent.click(screen.getByRole('button',{name}));
  await waitFor(()=>expect(screen.getByTestId('path')).toHaveTextContent('/poker/table/'+table));
  expect(screen.getByRole('button',{name:'安全离座'})).toBeEnabled();expect(Socket.all).toHaveLength(1);expect(socket.readyState).toBe(1);expect(tickets(r)).toHaveLength(1);
  const event=new Event('beforeunload',{cancelable:true});window.dispatchEvent(event);expect(event.defaultPrevented).toBe(true);
});
it('keeps live ownership and queries while Gate loading/error/maintenance only tighten mutations',async()=>{
  const r=await setup(whole(true));await live(r);let resolve!:(value:Response)=>void;r.f.gateRead=()=>new Promise(done=>{resolve=done;});
  fireEvent.click(screen.getByRole('button',{name:'重新核对访问状态'}));expect(screen.getByRole('button',{name:'安全离座'})).toBeDisabled();
  await act(async()=>resolve(new Response(JSON.stringify({success:false}),{status:503})));expect(screen.getByRole('button',{name:'安全离座'})).toBeDisabled();expect(Socket.all[0].readyState).toBe(1);
  r.f.gateRead=undefined;r.f.gateStage='MAINTENANCE';fireEvent.click(screen.getByRole('button',{name:'重新核对访问状态'}));
  await waitFor(()=>expect(screen.getByRole('button',{name:'安全离座'})).toBeEnabled());expect(Socket.all).toHaveLength(1);expect(tickets(r)).toHaveLength(1);
});
it('releases the player hold only after the original Session is SETTLED, not leave ACK or an active query',async()=>{
  const r=await setup(whole(true));let done=false;r.f.respond=(path)=>path.endsWith('/safe-leave')?Promise.reject(Error('synthetic lost ACK')):path.endsWith('/sessions/'+session)?Promise.resolve(ok(done?settled():active())):undefined;
  await live(r);fireEvent.click(screen.getByRole('button',{name:'安全离座'}));fireEvent.click(screen.getByRole('button',{name:'确认安全离座'}));
  await screen.findByText('原会话尚未完成出金。');fireEvent.click(screen.getByRole('button',{name:'test wallet navigation'}));
  await waitFor(()=>expect(screen.getByTestId('path')).toHaveTextContent('/poker/table/'+table));
  done=true;r.f.read=()=>Promise.resolve(ok(whole()));fireEvent.click(screen.getByRole('button',{name:'核对原会话出金'}));
  await waitFor(()=>expect(screen.getByTestId('path').textContent).toBe('/poker'));expect(await screen.findByRole('region',{name:'Poker 大厅'})).toBeVisible();expect(Socket.all[0].readyState).toBe(3);
  fireEvent.click(screen.getByRole('button',{name:'test wallet navigation'}));await waitFor(()=>expect(screen.getByTestId('path').textContent).toBe('/wallet'));
});
it('lets a spectator return with no cashout or player navigation hold',async()=>{
  const r=await setup();await live(r,false);fireEvent.click(screen.getByRole('button',{name:'返回大厅'}));
  await waitFor(()=>expect(screen.getByTestId('path').textContent).toBe('/poker'));expect(await screen.findByRole('region',{name:'Poker 大厅'})).toBeVisible();
  expect(pokerCalls(r).some(([p])=>p.endsWith('/safe-leave')||p.includes('/sessions/'))).toBe(false);
});
it.each(['denied','other-session'])('retires spectator admission across real back/forward when latest whole is %s',async kind=>{
  const r=await setup();render(<MemoryRouter initialEntries={['/poker','/poker/table/'+table]} initialIndex={1}><Navigation/><App client={r.client}/></MemoryRouter>);
  await waitFor(()=>expect(Socket.all).toHaveLength(1));act(()=>activate(Socket.all[0],false));await screen.findByRole('button',{name:'返回大厅'});
  fireEvent.click(screen.getByRole('button',{name:'test browser back'}));expect(await screen.findByRole('region',{name:'Poker 大厅'})).toBeVisible();
  const raw=whole(kind==='other-session');if(kind==='denied')raw.tables[0].can_spectate=false;else raw.active_session.table_id=other;
  let resolve!:(value:Response)=>void;r.f.read=()=>new Promise(done=>{resolve=done;});const count=pokerCalls(r).length;
  fireEvent.click(screen.getByRole('button',{name:'test browser forward'}));
  await waitFor(()=>expect(pokerCalls(r)).toHaveLength(count+1));expect(pokerCalls(r).at(-1)?.[0]).toContain('/tables?q='+table);
  expect(tickets(r)).toHaveLength(1);expect(Socket.all).toHaveLength(1);
  await act(async()=>resolve(ok(raw)));expect(await screen.findByText(/当前牌桌入口尚未获准/)).toBeVisible();expect(tickets(r)).toHaveLength(1);expect(Socket.all).toHaveLength(1);
});
it('drops the pin and old socket after a hard Gate restriction, then permits ordinary navigation',async()=>{
  const r=await setup(whole(true));await live(r);r.f.gateStage='ACCOUNT_RESTRICTED';fireEvent.click(screen.getByRole('button',{name:'重新核对访问状态'}));
  await screen.findByText(/账户当前受到访问限制/);expect(Socket.all[0].readyState).toBe(3);fireEvent.click(screen.getByRole('button',{name:'test wallet navigation'}));
  await waitFor(()=>expect(screen.getByTestId('path').textContent).toBe('/wallet'));
});
it('preserves the original unknown entry key through navigation and receipt NOT_FOUND without resubmitting',async()=>{
  const r=await setup();let command:any;r.f.respond=(path,init)=>{
    if(path==='/api/v1/poker/tables'&&init?.method==='POST'){command=JSON.parse(String(init.body));return Promise.reject(Error('synthetic lost ACK'));}
    if(path.endsWith('/entry-receipt-query'))return Promise.resolve(ok({user_id:'910002',kind:'create',mutation_id:command.request_id,state:'NOT_FOUND'}));
  };
  mount(r,'/poker');fireEvent.click(await screen.findByRole('button',{name:'创建牌桌'}));fireEvent.change(screen.getByLabelText('牌桌名称'),{target:{value:'Route recovery'}});fireEvent.click(screen.getByRole('button',{name:'确认创建牌桌'}));
  await waitFor(()=>expect(screen.getByRole('button',{name:'查询原操作回执'})).toBeEnabled());const key=command.request_id;
  fireEvent.click(screen.getByRole('button',{name:'test wallet navigation'}));await waitFor(()=>expect(screen.getByTestId('path').textContent).toBe('/poker'));
  fireEvent.click(screen.getByRole('button',{name:'查询原操作回执'}));await waitFor(()=>expect(screen.getByRole('button',{name:'重试原请求'})).toBeEnabled());
  fireEvent.click(screen.getByRole('button',{name:'test browser back'}));await waitFor(()=>expect(screen.getByTestId('path').textContent).toBe('/poker'));
  expect(screen.getByRole('region',{name:'原入桌操作'})).toHaveTextContent(key);expect(pokerCalls(r).filter(([p])=>p==='/api/v1/poker/tables')).toHaveLength(1);
});
it('hands the successful create/reserve/buyin owner into the same player route hold',async()=>{
  const raw=whole();raw.tables[0].open_seat_numbers=[2,4,5,6];const r=await setup(raw),reservation='019a0000-0000-7000-8000-000000000031';let bought=false;
  r.f.read=()=>Promise.resolve(ok(bought?whole(true):raw));r.f.respond=(path)=>{
    const receipt={table_id:table,table_version:'9',duplicate:false};
    if(path.endsWith('/tables'))return Promise.resolve(ok({...receipt,status:'WAITING'}));
    if(path.endsWith('/seat-reservations'))return Promise.resolve(ok({...receipt,status:'LEASE_ACTIVE',reservation_id:reservation}));
    if(path.endsWith('/reservation-query'))return Promise.resolve(ok({user_id:'910002',table_id:table,reservation_id:reservation,state:'FOUND',reservation:{seat_no:2,durable_state:'LEASE_ACTIVE',expires_at:'2026-09-06T12:00:30Z',checked_at:'2026-09-06T12:00:00Z',valid:true}}));
    if(path.endsWith('/buy-ins')){bought=true;return Promise.resolve(ok({...receipt,status:'CONFIRMED',session_id:session,funding_operation_id:other,amount_units:'200000000'}));}
  };
  mount(r,'/poker');fireEvent.click(await screen.findByRole('button',{name:'创建牌桌'}));fireEvent.change(screen.getByLabelText('牌桌名称'),{target:{value:'Successful route handoff'}});fireEvent.click(screen.getByRole('button',{name:'确认创建牌桌'}));
  await waitFor(()=>expect(screen.getByRole('button',{name:'预留 2 号座位'})).toBeEnabled());fireEvent.click(screen.getByRole('button',{name:'预留 2 号座位'}));
  await waitFor(()=>expect(screen.getByRole('button',{name:'确认买入并等待大盲'})).toBeEnabled());fireEvent.click(screen.getByRole('button',{name:'确认买入并等待大盲'}));
  await waitFor(()=>expect(Socket.all).toHaveLength(1));expect(screen.getByTestId('path').textContent).toBe('/poker/table/'+table);expect(JSON.parse(String(tickets(r)[0][1]?.body)).control_intent).toBe('CLAIM_CONTROL');
  act(()=>activate(Socket.all[0]));fireEvent.click(screen.getByRole('button',{name:'test wallet navigation'}));await waitFor(()=>expect(screen.getByTestId('path').textContent).toBe('/poker/table/'+table));
  expect(screen.getByRole('button',{name:'安全离座'})).toBeEnabled();expect(tickets(r)).toHaveLength(1);expect(pokerCalls(r).filter(([p])=>/\/(tables|seat-reservations|buy-ins)$/.test(p))).toHaveLength(3);
});
it.each(['native','client'])('retires an already held %s identity without letting old cleanup release its replacement',async boundary=>{
  const r=await setup(whole(true)),view=await live(r),old=Socket.all[0];let current=r;
  if(boundary==='native'){r.f.sid='held-native-next';await act(async()=>{await r.client.login('synthetic','synthetic');});}
  else{current=await setup(whole(true));view.rerender(<MemoryRouter initialEntries={['/wallet','/poker/table/'+table]} initialIndex={1}><Navigation/><App client={current.client}/></MemoryRouter>);}
  await waitFor(()=>expect(Socket.all).toHaveLength(2));expect(old.readyState).toBe(3);act(()=>activate(Socket.all[1]));
  act(()=>old.message('retired socket response'));fireEvent.click(screen.getByRole('button',{name:'test wallet navigation'}));
  await waitFor(()=>expect(screen.getByTestId('path').textContent).toBe('/poker/table/'+table));expect(Socket.all).toHaveLength(2);expect(Socket.all[1].readyState).toBe(1);expect(screen.getByRole('button',{name:'安全离座'})).toBeEnabled();
  expect(tickets(current)).toHaveLength(boundary==='native'?2:1);
});
it.each([true,false])('derives deep-link control only from fresh whole admission, active=%s',async active=>{
  const r=await setup(whole(active));render(<MemoryRouter initialEntries={['/poker/table/'+table]}><App client={r.client}/></MemoryRouter>);
  await waitFor(()=>expect(tickets(r)).toHaveLength(1));
  const reads=pokerCalls(r);expect(reads[0][0]).toContain('/tables?q='+table);
  expect(JSON.parse(String(tickets(r)[0][1]?.body))).toEqual({target_table_id:table,control_intent:active?'CLAIM_CONTROL':'READ_ONLY'});
  expect(screen.queryByRole('navigation',{name:'底部导航'})).toBeNull();expect(screen.queryByRole('navigation',{name:'主导航'})).toBeNull();
  expect(Socket.all).toHaveLength(1);
});
it.each(['absent','denied','other-session'])('keeps an unadmitted %s deep link out of the socket',async kind=>{
  const raw=whole(kind==='other-session');if(kind==='absent')raw.tables=[];if(kind==='denied')raw.tables[0].can_spectate=false;if(kind==='other-session')raw.active_session.table_id=other;
  const r=await setup(raw);render(<MemoryRouter initialEntries={['/poker/table/'+table]}><App client={r.client}/></MemoryRouter>);
  expect(await screen.findByRole('button',{name:'返回大厅'})).toBeEnabled();expect(tickets(r)).toHaveLength(0);expect(Socket.all).toHaveLength(0);
});
it.each(['/poker?viewer=PLAYER_SELF','/poker/table/'+table+'?password=secret','/poker/table/'+table+'#control','/poker/table/not-a-uuid','/poker/extra'])('rejects URL authority and malformed destinations: %s',async path=>{
  const r=await setup();render(<MemoryRouter initialEntries={[path]}><App client={r.client}/></MemoryRouter>);
  expect(await screen.findByText('牌桌入口格式无效，请返回大厅。')).toBeVisible();expect(pokerCalls(r)).toHaveLength(0);
});
it('discards old deep-link admission on native SID replacement while a whole read waits',async()=>{
  const r=await setup(whole(true));let resolve!:(value:Response)=>void;r.f.read=()=>new Promise(done=>{resolve=done;});
  render(<MemoryRouter initialEntries={['/poker/table/'+table]}><App client={r.client}/></MemoryRouter>);
  await waitFor(()=>expect(pokerCalls(r)).toHaveLength(1));const old=resolve;
  r.f.sid='route-native-next';await act(async()=>{await r.client.login('synthetic','synthetic');});
  await waitFor(()=>expect(pokerCalls(r)).toHaveLength(2));await act(async()=>old(ok(whole(true))));expect(tickets(r)).toHaveLength(0);
  await act(async()=>resolve(ok(whole(true))));await waitFor(()=>expect(tickets(r)).toHaveLength(1));
});
