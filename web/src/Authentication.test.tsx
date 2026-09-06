import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, it, vi } from 'vitest';
import { ApiClient } from './api';
import { Authentication, DiscordCallback } from './Authentication';
const ok=(data: unknown)=>new Response(JSON.stringify({success:true,data}));
const config={enabled:true,registration_enabled:true,eligibility:'加入指定服务器并取得所需身份组。'};
it('keeps existing-user login and new registration separate',async()=>{
 const f=vi.fn(async()=>ok(config));const c=new ApiClient(f);
 const view=render(<MemoryRouter><Authentication client={c} mode="registration"/></MemoryRouter>);
 expect(await screen.findByRole('button',{name:'使用 Discord 注册'})).toBeEnabled();
 const art=view.container.querySelector('img[src="/assets/characters/mash-registration-idle-v001.png"]');
 expect(art).toBeVisible();fireEvent.error(art!);
 expect(art).not.toBeInTheDocument();expect(screen.getByRole('button',{name:'使用 Discord 注册'})).toBeEnabled();
 expect(screen.queryByLabelText('用户名')).not.toBeInTheDocument();expect(screen.getByRole('link',{name:'已有账户，返回登录'})).toHaveAttribute('href','/login');
 expect(screen.getByText(config.eligibility)).toBeVisible();expect(f).toHaveBeenCalledTimes(1);
});
it('clears password after failure and offers explicit visibility',async()=>{
 const f=vi.fn(async(path:string)=>path.includes('/admission/config')?ok(config):new Response(JSON.stringify({success:false,message:'凭据不匹配'}),{status:400}));const c=new ApiClient(f);
 const view=render(<MemoryRouter><Authentication client={c} mode="login"/></MemoryRouter>);
 await screen.findByRole('button',{name:'使用 Discord 登录'});
 const art=view.container.querySelector('img[src="/assets/characters/artoria-saber-login-idle-v001.png"]');
 expect(art).toBeVisible();fireEvent.error(art!);expect(art).not.toBeInTheDocument();
 fireEvent.change(screen.getByLabelText('用户名'),{target:{value:'native-user'}});fireEvent.change(screen.getByLabelText('密码'),{target:{value:'synthetic-password'}});
 fireEvent.click(screen.getByRole('button',{name:'显示密码'}));expect(screen.getByLabelText('密码')).toHaveAttribute('type','text');
 fireEvent.click(screen.getByRole('button',{name:'登录控制台'}));await screen.findByText('凭据不匹配');expect(screen.getByLabelText('密码')).toHaveValue('');
});
it('reports membership failures without inferring account creation',async()=>{
 const f=vi.fn(async(path:string)=>path.includes('/admission/config')?ok(config):new Response(JSON.stringify({success:false,code:'DISCORD_NOT_MEMBER'}),{status:403}));const c=new ApiClient(f);
 render(<MemoryRouter><Authentication client={c} mode="registration"/></MemoryRouter>);
 fireEvent.click(await screen.findByRole('button',{name:'使用 Discord 注册'}));expect(await screen.findByRole('alert')).toHaveTextContent('尚未加入指定');expect(f.mock.calls.some(([p])=>p.includes('ensure'))).toBe(false);
});
it('keeps native admission 2FA before completion and never resubmits callback',async()=>{
 const f=vi.fn().mockResolvedValueOnce(ok({require_2fa:true,flow_token:'synthetic-flow'})).mockResolvedValueOnce(ok({access_token:'synthetic',access_expires_at:4102444800,session:{sid:'synthetic'},user:{id:12,username:'u',display_name:'U',role:1}})); const c=new ApiClient(f); const done=vi.fn();
 render(<MemoryRouter><DiscordCallback client={c} captured={{input:{code:'synthetic-code',state:'synthetic-state'}}} onComplete={done}/></MemoryRouter>);
 await screen.findByLabelText('验证码或备用码');expect(done).not.toHaveBeenCalled();
 fireEvent.change(screen.getByLabelText('验证码或备用码'),{target:{value:'123456'}});fireEvent.click(screen.getByRole('button',{name:'完成身份验证'}));
 await waitFor(()=>expect(done).toHaveBeenCalledTimes(1));expect(f.mock.calls.filter(([p])=>p.includes('/callback?'))).toHaveLength(1);expect(f.mock.calls[1][0]).toBe('/api/momiao/auth/2fa');
});
