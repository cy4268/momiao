import { withReadyAccessGate } from './m1-test-fixtures';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, useNavigate } from 'react-router-dom';
import { expect, it, vi } from 'vitest';
import { ApiClient } from './api';
import { Account } from './Account';
import { App } from './App';
import { saveRouteIntent, consumeRouteIntent } from './post-auth-intent';
const user={id:12,username:'native-readonly',display_name:'Display',role:1};
const session={authentication_state:'AUTHENTICATED',csrf_token:'local-csrf'};
const profile={user_id:'12',short_account_id:'CA-00000000000C',status:'INCOMPLETE',display_name:'',avatar_id:'system-default',profile_version:'0',nickname_changed_at:null,next_rename_at:null,suggested_name:'Master-CA-00000000000C',avatars:[{id:'system-default',label:'系统默认头像',source:'SYSTEM'}]};
const status={user_id:'12',source:'UNVERIFIED',grant_status:'PENDING_SOURCE',amount_units:'0',transaction_id:null,source_available:false};
const ok=(data:unknown)=>new Response(JSON.stringify({success:true,data}));
it('keeps the account form through local credential rotations and resets it on user change',async()=>{
 let enabled=false;let generation=1;let currentUser=user;let restricted=false;
 const f=vi.fn(async(path:string)=>{
  if(path==='/api/v1/account/2fa/enable'){enabled=true;generation++;return ok({});}
  if(path==='/api/v1/account/2fa/disable'){enabled=false;generation++;return ok({});}
  if(path==='/api/v1/session/bootstrap')return ok({authentication_state:'AUTHENTICATED',csrf_token:`local-${generation}`});
  if(path==='/api/user/self')return ok(currentUser);
  if(path==='/api/v1/account/password/change'){generation++;return ok({});}
  if(path.startsWith('/platform/v1/access-gate?'))return ok({user_id:String(currentUser.id),route:new URLSearchParams(path.split('?')[1]).get('route'),stage:restricted?'ACCOUNT_RESTRICTED':'READY'});
  if(path==='/api/v1/account')return ok({id:currentUser.id,username:currentUser.username,has_password:true,discord_linked:true,two_fa_enabled:enabled});
  if(path==='/api/v1/account/2fa/setup')return ok({secret:'TEST-SECRET',qr_code_data:'otpauth://totp/momiao:test',backup_codes:['AAAA-BBBB','CCCC-DDDD','EEEE-FFFF','GGGG-HHHH']});
  return ok(path.includes('master-profile')?{...profile,user_id:String(currentUser.id),short_account_id:currentUser.id===12?profile.short_account_id:'CA-00000000000D'}:status);
 });
 const c=new ApiClient(f);await c.bootstrap();render(<MemoryRouter initialEntries={['/account/security']}><App client={c}/></MemoryRouter>);
 const heading=await screen.findByRole('heading',{name:'账户与安全'});const firstGeneration=c.getSessionGeneration();
 fireEvent.click(await screen.findByRole('button',{name:'设置二次验证'}));
 expect(await screen.findByText('TEST-SECRET')).toBeVisible();expect(screen.getByText('AAAA-BBBB')).toBeVisible();
 fireEvent.change(screen.getByLabelText('验证器验证码'),{target:{value:'123456'}});
 fireEvent.click(screen.getByRole('button',{name:'启用二次验证'}));
 const disable=await screen.findByRole('button',{name:'关闭二次验证'});
 expect(c.getSessionGeneration()).toBeGreaterThan(firstGeneration);
 expect(screen.getByRole('heading',{name:'账户与安全'})).toBe(heading);
 expect(screen.queryByText('TEST-SECRET')).not.toBeInTheDocument();expect(screen.queryByText('AAAA-BBBB')).not.toBeInTheDocument();
 fireEvent.change(screen.getByLabelText('验证码或备用码'),{target:{value:'AAAA-BBBB'}});fireEvent.click(disable);
 expect(await screen.findByRole('button',{name:'设置二次验证'})).toBeEnabled();
 expect(f.mock.calls.filter(([p])=>p.startsWith('/api/v1/account/2fa/')).map(([p])=>p)).toEqual(['/api/v1/account/2fa/setup','/api/v1/account/2fa/enable','/api/v1/account/2fa/disable']);
 fireEvent.change(screen.getByLabelText('当前密码'),{target:{value:'previous-pass'}});
 fireEvent.change(screen.getByLabelText('新密码'),{target:{value:'changed-pass'}});
 fireEvent.change(screen.getByLabelText('确认新密码'),{target:{value:'changed-pass'}});
 fireEvent.click(screen.getByRole('button',{name:'更新密码'}));
 expect(await screen.findByText('密码已更新，当前登录已重新核对。')).toBeVisible();
 expect(screen.getByRole('heading',{name:'账户与安全'})).toBe(heading);
 expect(screen.getByLabelText('当前密码')).toHaveValue('');expect(screen.getByLabelText('新密码')).toHaveValue('');expect(screen.getByLabelText('确认新密码')).toHaveValue('');
 expect(f.mock.calls.filter(([p])=>p==='/api/v1/account/password/change')).toHaveLength(1);
 await waitFor(()=>expect(f.mock.calls.filter(([p])=>p.startsWith('/platform/v1/access-gate?'))).toHaveLength(4));
 currentUser={...user,id:13,username:'second-local'};generation++;
 await act(()=>c.refresh());
 expect(await screen.findByText('second-local')).toBeVisible();
 expect(screen.getByRole('heading',{name:'账户与安全'})).not.toBe(heading);
 expect(screen.queryByText('密码已更新，当前登录已重新核对。')).not.toBeInTheDocument();
 restricted=true;generation++;
 await act(()=>c.refresh());
 expect(await screen.findByText('账户当前受到访问限制。请退出登录，或通过既有支持入口核对账户状态。')).toBeVisible();
 expect(screen.queryByLabelText('新密码')).not.toBeInTheDocument();

});
it('shows actual read-only login identifier and first set requires a fresh proof',async()=>{
 const f=vi.fn(async(path:string)=>ok(path==='/api/v1/session/bootstrap'?{authentication_state:'AUTHENTICATED',csrf_token:'local-csrf'}:path==='/api/user/self'?user:path==='/api/v1/account'?{id:12,username:'native-readonly',has_password:false,discord_linked:true,two_fa_enabled:true}:path.includes('master-profile')?profile:status));const c=new ApiClient(withReadyAccessGate(f));await c.bootstrap();
 render(<MemoryRouter><Account client={c}/></MemoryRouter>);
 expect(await screen.findByText('native-readonly')).toBeVisible();expect(screen.queryByLabelText('用户名')).not.toBeInTheDocument();
 // The account page is the one place that keeps the login name and account number, labelled plainly.
 expect(screen.getByText('登录名').nextElementSibling).toHaveTextContent('native-readonly');
 await waitFor(()=>expect(screen.getByText('账户编号').nextElementSibling).toHaveTextContent('CA-00000000000C'));
 expect(document.querySelector('.identity-chamber.chamber-security')).toBeInTheDocument();
 expect(screen.getByRole('button',{name:'账户与会话说明'})).toHaveAttribute('aria-expanded','false');
 expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
 expect(screen.getByRole('button',{name:'验证 Discord 并设置密码'})).toBeEnabled();expect(screen.queryByLabelText('新密码')).not.toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'账户与会话说明'}));
 expect(screen.getByRole('dialog',{name:'账户与会话说明'})).toHaveTextContent('成功后其他旧会话将失效');
 expect(f.mock.calls.some(([path])=>path.includes('/password/'))).toBe(false);
});
it('uses a supplied in-memory proof for first set then clears it without replay',async()=>{
 const f=vi.fn(async(path:string)=>ok(path==='/api/v1/session/bootstrap'?session:path==='/api/user/self'?user:path==='/api/v1/account'?{id:12,username:'native-readonly',has_password:false,discord_linked:true,two_fa_enabled:false}:path.includes('master-profile')?profile:path.includes('/password/set')?session:status));const c=new ApiClient(withReadyAccessGate(f));await c.bootstrap();const clear=vi.fn();
 render(<MemoryRouter><Account client={c} proof={{proof:'synthetic-proof',purpose:'PASSWORD_SET',expires_at:4102444800}} clearProof={clear}/></MemoryRouter>);
 fireEvent.change(await screen.findByLabelText('新密码'),{target:{value:'synthetic-pass'}});fireEvent.change(screen.getByLabelText('确认新密码'),{target:{value:'synthetic-pass'}});fireEvent.click(screen.getByRole('button',{name:'设置密码'}));
 await waitFor(()=>expect(clear).toHaveBeenCalled());expect(f.mock.calls.filter(([p])=>p.includes('/password/set'))).toHaveLength(1);
});
it('stores only an expiring fixed route key and consumes it once',()=>{
 sessionStorage.clear();saveRouteIntent('/wallet',1000);expect(consumeRouteIntent(1001)).toBe('/wallet');expect(consumeRouteIntent(1001)).toBe('/dashboard');
 saveRouteIntent('https://evil.example',1000);expect(consumeRouteIntent(1001)).toBe('/dashboard');
 saveRouteIntent('/keys',1000);expect(consumeRouteIntent(1000+30*60*1000+1)).toBe('/dashboard');
 saveRouteIntent('/wallet?amount=123',1000);expect(consumeRouteIntent(1001)).toBe('/dashboard');
});

it('hands the sensitive callback proof to the account form and clears it on leaving that page',async()=>{
 const f=vi.fn(async(path:string)=>ok(path==='/api/v1/session/bootstrap'?session:path==='/api/user/self'?user:path==='/api/v1/auth/discord/callback'?{proof:'synthetic-callback-proof',purpose:'PASSWORD_SET',expires_at:4102444800}:path==='/api/v1/account'?{id:12,username:user.username,has_password:false,discord_linked:true,two_fa_enabled:false}:path.startsWith('/platform/v1/access-gate?')?{user_id:'12',route:new URLSearchParams(path.split('?')[1]).get('route'),stage:'READY'}:path.includes('master-profile')?profile:status));
 const c=new ApiClient(f);await c.bootstrap();const generation=c.getSessionGeneration();
 function ReturnToAccount(){const navigate=useNavigate();return <button onClick={()=>navigate('/account')}>Test return to account</button>;}
 render(<MemoryRouter initialEntries={['/oauth/discord']}><App client={c} capturedCallback={{input:{code:'synthetic-code',state:'synthetic-state'}}}/><ReturnToAccount/></MemoryRouter>);
 expect(await screen.findByRole('heading',{name:'设置首个密码'})).toBeVisible();
 expect(c.getSnapshot().user?.id).toBe(12);expect(c.getSessionGeneration()).toBe(generation);
 expect(screen.getByLabelText('新密码')).toBeVisible();
 fireEvent.click(screen.getByRole('link',{name:'查看 Master 资料'}));
 await screen.findByRole('heading',{name:'Master 资料'});
 fireEvent.click(screen.getByRole('button',{name:'Test return to account'}));
 expect(await screen.findByRole('button',{name:'验证 Discord 并设置密码'})).toBeEnabled();
 expect(screen.queryByLabelText('新密码')).not.toBeInTheDocument();
 expect(f.mock.calls.filter(([p])=>p==='/api/v1/auth/discord/callback')).toHaveLength(1);
});
