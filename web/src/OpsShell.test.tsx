import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, it, vi } from 'vitest';
import { ApiClient } from './api';
import { App } from './App';
import { ok, failed } from './m1-test-fixtures';

it('saves maintenance directly with a local admin and shows all operations without legacy bootstrap', async () => {
 let state={enabled:false,message:'',scopes:[] as string[]};
 let failRead=true;
 const fetcher=vi.fn(async(path:string,init?:RequestInit)=>{
  if(path==='/api/v1/session/bootstrap')return ok({authentication_state:'AUTHENTICATED',csrf_token:'local-csrf'});
  if(path==='/api/user/self')return ok({id:1,username:'operator',role:10});
  if(path.startsWith('/platform/v1/access-gate?'))return ok({stage:'READY',user_id:'1',route:'/ops/maintenance'});
  if(path==='/api/v1/maintenance/notices')return ok({items:state.enabled?[{id:'maintenance',message:state.message,scopes:state.scopes}]:[]});
  if(path==='/api/v1/ops/maintenance'){
   if(init?.method==='PUT'){state=JSON.parse(String(init.body));return ok(state)}
   return failRead?failed():ok(state);
  }
  throw new Error('Unexpected endpoint: '+path);
 });
 const client=new ApiClient(fetcher);await client.bootstrap();
 render(<MemoryRouter initialEntries={['/ops/maintenance']}><App client={client}/></MemoryRouter>);
 expect(await screen.findByRole('heading',{name:'维护设置'})).toBeVisible();
 expect(await screen.findByRole('alert')).toBeVisible();
 failRead=false;fireEvent.click(screen.getByRole('button',{name:'重新读取'}));
 const form=await screen.findByRole('group',{name:'维护设置'});
 expect(within(form).getByRole('checkbox',{name:'奖励领取'})).toBeChecked();
 fireEvent.click(within(form).getByRole('checkbox',{name:'开启维护'}));
 fireEvent.change(screen.getByRole('textbox',{name:'提示语（可选）'}),{target:{value:'预计十分钟后恢复'}});
 fireEvent.click(screen.getByRole('button',{name:'保存'}));
 expect(await screen.findByRole('status')).toHaveTextContent('维护设置已保存');
 expect(state.enabled).toBe(true);expect(state.scopes).toHaveLength(7);
 expect(state.message).toBe('预计十分钟后恢复');
 const write=fetcher.mock.calls.find(([path,init])=>path==='/api/v1/ops/maintenance'&&init?.method==='PUT')!;
 expect(new Headers(write[1]?.headers).get('X-CSRF-Token')).toBe('local-csrf');
 expect(Object.keys(JSON.parse(String(write[1]?.body))).sort()).toEqual(['enabled','message','scopes']);
 fireEvent(document,new Event('visibilitychange'));
 expect(await screen.findByText('维护进行中')).toBeVisible();
 expect(within(screen.getByRole('status',{name:'维护与服务影响'})).getByText('预计十分钟后恢复')).toBeVisible();
 fireEvent.click(within(form).getByRole('checkbox',{name:'开启维护'}));
 fireEvent.click(screen.getByRole('button',{name:'保存'}));
 await waitFor(()=>expect(state.enabled).toBe(false));
 const nav=screen.getByRole('navigation',{name:'运营导航'});
 expect(within(nav).getByRole('link',{name:'用户'})).toBeVisible();
 expect(within(nav).getByRole('link',{name:'经济与账务'})).toBeVisible();
 expect(fetcher.mock.calls.some(([path])=>path.includes('/ops/bootstrap')||path.includes('/ops/operations')||path.includes('/ops/fresh')||path.includes('/native-runtime'))).toBe(false);
});
