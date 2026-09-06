import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, it, vi } from 'vitest';
import { ApiClient } from './api';
import { App } from './App';
import { saveRouteIntent } from './post-auth-intent';
import { profile, wallet, daily, ok } from './m1-test-fixtures';

it('ensures provisional Master only after the authoritative gate then rechecks before returning without replaying a wallet write',async()=>{
 const user={id:12,username:'native-readonly',display_name:'Native',role:1};let initialized=false;
 const ownProfile=()=>({...profile,user_id:'12',status:initialized?'COMPLETE':'INCOMPLETE',display_name:initialized?'星之海':'',profile_version:initialized?'1':'0'});
 const f=vi.fn(async(path:string,init?:RequestInit)=>{
  if(path==='/api/user/login')return ok({access_token:'synthetic',access_expires_at:4102444800,session:{sid:'synthetic'},user});
  if(path==='/platform/v1/admission/config')return ok({enabled:true,registration_enabled:true,eligibility:'资格验证'});
  if(path.startsWith('/platform/v1/access-gate?'))return ok({user_id:'12',route:new URLSearchParams(path.split('?')[1]).get('route'),stage:initialized?'READY':'MASTER_REQUIRED'});
  if(path==='/api/momiao/account')return ok({id:12,username:user.username,has_password:false,discord_connected:true,two_fa_enabled:false});
  if(path==='/platform/v1/admission/ensure'||path==='/platform/v1/master-profile')return ok(ownProfile());
  if(path==='/platform/v1/master-profile/initialize'){initialized=true;return ok(ownProfile());}
  if(path==='/platform/v1/admission')return ok({user_id:'12',source:'NEW_DISCORD_REGISTRATION',grant_status:'PENDING',amount_units:'500000000',transaction_id:null,source_available:false});
  if(path==='/platform/v1/wallet')return ok({...wallet,user_id:'12'});
  if(path==='/platform/v1/rewards/daily')return ok({...daily,user_id:'12'});
  if(path.startsWith('/platform/v1/transactions')||path.startsWith('/platform/v1/wallet/ledger'))return ok({items:[],has_more:false,next_after_id:null,next_after_seq:null});
  throw new Error(`unexpected test request ${path} ${init?.method}`);
 });
 const c=new ApiClient(f);await c.login('a','b');saveRouteIntent('/wallet');
 const pending={kind:'EXCHANGE',key:'01990000-1111-7777-aaaa-000000000001',from_asset:'AVAILABLE_CHIPS',amount:'1'};sessionStorage.setItem('momiao.wallet.pending.12',JSON.stringify(pending));
 render(<MemoryRouter initialEntries={['/welcome']}><App client={c}/></MemoryRouter>);
 fireEvent.change(await screen.findByLabelText('Master 昵称'),{target:{value:'星之海'}});fireEvent.click(screen.getByRole('button',{name:'保存并初始化'}));
 await screen.findByRole('heading',{name:'我的钱包'});
 expect(f.mock.calls.findIndex(([p])=>p.startsWith('/platform/v1/access-gate?'))).toBeLessThan(f.mock.calls.findIndex(([p])=>p==='/platform/v1/admission/ensure'));
 expect(f.mock.calls.filter(([,i])=>i?.method==='POST').map(([p])=>p)).toEqual(['/api/user/login','/platform/v1/admission/ensure','/platform/v1/master-profile/initialize']);
 expect(sessionStorage.getItem('momiao.wallet.pending.12')).toBe(JSON.stringify(pending));
});
it('a failed initialization response offers reconciliation rather than replaying registration',async()=>{
 const user={id:12,username:'u',display_name:'U',role:1};const f=vi.fn(async(path:string)=>path==='/api/user/login'?ok({access_token:'synthetic',access_expires_at:4102444800,session:{sid:'synthetic'},user}):path.includes('/admission/config')?ok({enabled:true,registration_enabled:true,eligibility:'资格验证'}):path==='/api/momiao/account'?ok({id:12,username:'u',has_password:false,discord_connected:true,two_fa_enabled:false}):new Response(JSON.stringify({success:false,code:'ADMISSION_UNAVAILABLE'}),{status:503}));const c=new ApiClient(f);await c.login('a','b');
 render(<MemoryRouter initialEntries={['/welcome']}><App client={c}/></MemoryRouter>);
 await screen.findByRole('button',{name:'重新核对访问状态'});expect(f.mock.calls.some(([p])=>p.includes('/registration/start'))).toBe(false);expect(f.mock.calls.some(([p])=>p.includes('/ensure'))).toBe(false);
});

it('keeps migration acknowledgement separate and holds the intent and feature reads until its rechecked gate is ready',async()=>{
 const user={id:12,username:'u',display_name:'U',role:1};let acknowledged=false;
 const notice=()=>({user_id:'12',state:acknowledged?'ACKNOWLEDGED':'REQUIRED',required_migration_version:'2',acknowledged_migration_version:acknowledged?'2':'0',acknowledged_at:acknowledged?'2026-09-06T00:00:00Z':null,title:'迁移事实确认',body:'此处只确认已完成的事实。',completed_at:'2026-09-05T00:00:00Z'});
 const f=vi.fn(async(path:string,init?:RequestInit)=>{
  if(path==='/api/user/login')return ok({access_token:'synthetic',access_expires_at:4102444800,session:{sid:'synthetic'},user});
  if(path.startsWith('/platform/v1/access-gate?'))return ok({user_id:'12',route:'/keys',stage:acknowledged?'READY':'MIGRATION_REQUIRED',...(acknowledged?{}:{migration_notice:notice()})});
  if(path==='/platform/v1/migration-notice/acknowledge'){acknowledged=true;return new Response(JSON.stringify({success:false}),{status:503});}
  if(path==='/platform/v1/master-profile')return ok({...profile,user_id:'12'});
  if(path.includes('/announcements/'))return ok({item:null});
  return ok({items:[],total:0,page:1,page_size:20});
 });
 const client=new ApiClient(f);await client.login('a','b');saveRouteIntent('/keys');
 render(<MemoryRouter initialEntries={['/welcome']}><App client={client}/></MemoryRouter>);
 await screen.findByRole('heading',{name:'迁移事实确认'});
 expect(f.mock.calls.some(([p])=>p.startsWith('/api/token'))).toBe(false);
 expect(sessionStorage.getItem('chaldea.post-auth.route.v2')).toContain('/keys');
 fireEvent.click(screen.getByRole('button',{name:'我已了解，继续'}));
 await screen.findByText(/确认结果尚未核实/);expect(screen.getByRole('button',{name:'我已了解，继续'})).toBeDisabled();
 fireEvent.click(screen.getByRole('button',{name:'重新核对访问状态'}));
 await screen.findByRole('button',{name:'创建 API 密钥'});
 const writes=f.mock.calls.filter(([p])=>p==='/platform/v1/migration-notice/acknowledge');expect(writes).toHaveLength(1);expect(JSON.parse(writes[0][1]?.body as string)).toEqual({version:'2'});
 expect(sessionStorage.getItem('chaldea.post-auth.route.v2')).toBeNull();
 expect(f.mock.calls.some(([p])=>p.includes('/admission/ensure')||p.includes('/rewards/')||p.includes('/quota-transfers'))).toBe(false);
});
