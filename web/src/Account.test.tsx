import { withReadyAccessGate } from './m1-test-fixtures';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, useNavigate } from 'react-router-dom';
import { expect, it, vi } from 'vitest';
import { ApiClient } from './api';
import { Account } from './Account';
import { App } from './App';
import { saveRouteIntent, consumeRouteIntent } from './post-auth-intent';
const user={id:12,username:'native-readonly',display_name:'Display',role:1};
const bundle={access_token:'synthetic',access_expires_at:4102444800,session:{sid:'synthetic'},user};
const profile={user_id:'12',short_account_id:'CA-00000000000C',status:'INCOMPLETE',display_name:'',avatar_id:'system-default',profile_version:'0',nickname_changed_at:null,next_rename_at:null,suggested_name:'Master-CA-00000000000C',avatars:[{id:'system-default',label:'系统默认头像',source:'SYSTEM'}]};
const status={user_id:'12',source:'UNVERIFIED',grant_status:'PENDING_SOURCE',amount_units:'0',transaction_id:null,source_available:false};
const ok=(data:unknown)=>new Response(JSON.stringify({success:true,data}));
it('shows actual read-only login identifier and first set requires a fresh proof',async()=>{
 const f=vi.fn(async(path:string)=>ok(path==='/api/user/login'?bundle:path==='/api/momiao/account'?{id:12,username:'native-readonly',has_password:false,discord_connected:true,two_fa_enabled:true}:path.includes('master-profile')?profile:status));const c=new ApiClient(withReadyAccessGate(f));await c.login('a','b');
 render(<MemoryRouter><Account client={c}/></MemoryRouter>);
 expect(await screen.findByText('native-readonly')).toBeVisible();expect(screen.queryByLabelText('用户名')).not.toBeInTheDocument();
 expect(screen.getByRole('button',{name:'验证 Discord 并设置密码'})).toBeEnabled();expect(screen.queryByLabelText('新密码')).not.toBeInTheDocument();
});
it('uses a supplied in-memory proof for first set then clears it without replay',async()=>{
 const f=vi.fn(async(path:string)=>ok(path==='/api/user/login'?bundle:path==='/api/momiao/account'?{id:12,username:'native-readonly',has_password:false,discord_connected:true,two_fa_enabled:false}:path.includes('master-profile')?profile:path.includes('/password/set')?{...bundle,user:undefined,has_password:true}:status));const c=new ApiClient(withReadyAccessGate(f));await c.login('a','b');const clear=vi.fn();
 render(<MemoryRouter><Account client={c} proof={{proof:'synthetic-proof',expires_at:4102444800}} clearProof={clear}/></MemoryRouter>);
 fireEvent.change(await screen.findByLabelText('新密码'),{target:{value:'synthetic-pass'}});fireEvent.change(screen.getByLabelText('确认新密码'),{target:{value:'synthetic-pass'}});fireEvent.click(screen.getByRole('button',{name:'设置密码'}));
 await waitFor(()=>expect(clear).toHaveBeenCalled());expect(f.mock.calls.filter(([p])=>p.includes('/password/set'))).toHaveLength(1);
});
it('stores only an expiring fixed route key and consumes it once',()=>{
 sessionStorage.clear();saveRouteIntent('/wallet/activate',1000);expect(consumeRouteIntent(1001)).toBe('/wallet/activate');expect(consumeRouteIntent(1001)).toBe('/dashboard');
 saveRouteIntent('https://evil.example',1000);expect(consumeRouteIntent(1001)).toBe('/dashboard');
 saveRouteIntent('/keys',1000);expect(consumeRouteIntent(1000+30*60*1000+1)).toBe('/dashboard');
 saveRouteIntent('/wallet?amount=123',1000);expect(consumeRouteIntent(1001)).toBe('/dashboard');
});

it('hands the sensitive callback proof to the account form and clears it on leaving that page',async()=>{
 const f=vi.fn(async(path:string)=>ok(path==='/api/user/login'?bundle:path.startsWith('/api/momiao/auth/discord/callback?')?{proof:'synthetic-callback-proof',expires_at:4102444800}:path==='/api/momiao/account'?{id:12,username:user.username,has_password:false,discord_connected:true,two_fa_enabled:false}:path.includes('master-profile')?profile:status));
 const c=new ApiClient(withReadyAccessGate(f));await c.login('a','b');
 function ReturnToAccount(){const navigate=useNavigate();return <button onClick={()=>navigate('/account')}>Test return to account</button>;}
 render(<MemoryRouter initialEntries={['/oauth/discord']}><App client={c} capturedCallback={{input:{code:'synthetic-code',state:'synthetic-state'}}}/><ReturnToAccount/></MemoryRouter>);
 expect(await screen.findByRole('heading',{name:'设置首个密码'})).toBeVisible();
 expect(screen.getByLabelText('新密码')).toBeVisible();
 fireEvent.click(screen.getByRole('link',{name:'查看 Master 资料 →'}));
 await screen.findByRole('heading',{name:'Master 资料'});
 fireEvent.click(screen.getByRole('button',{name:'Test return to account'}));
 expect(await screen.findByRole('button',{name:'验证 Discord 并设置密码'})).toBeEnabled();
 expect(screen.queryByLabelText('新密码')).not.toBeInTheDocument();
 expect(f.mock.calls.filter(([p])=>p.startsWith('/api/momiao/auth/discord/callback?'))).toHaveLength(1);
});
