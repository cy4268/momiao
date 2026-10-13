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
  if(path.startsWith('/platform/v1/access-gate?'))return ok({stage:'READY',user_id:'1',route:new URLSearchParams(path.split('?')[1]).get('route')});
  if(path==='/api/v1/maintenance/notices')return ok({items:state.enabled?[{id:'maintenance',message:state.message,scopes:state.scopes}]:[]});
  if(path.startsWith('/api/v1/ops/audit?')){
   const query=new URLSearchParams(path.split('?')[1]);const page=Number(query.get('page')||1);
   return ok({items:[{audit_id:'10000000-0000-4000-8000-000000000001',actor_newapi_user_id:'1',actor_name:'operator',action:page===1?'MAINTENANCE_SAVE':'USER_UPDATE',target_type:'MAINTENANCE',target_id:'DEVELOPMENT',before_snapshot:{enabled:false},after_snapshot:{enabled:true},occurred_at:'2026-10-12T23:00:00Z'}],total:21,page,page_size:20});
  }
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
 fireEvent.click(within(nav).getByRole('link',{name:'操作日志'}));
 expect(await screen.findByRole('heading',{name:'操作日志'})).toBeVisible();
 expect(await screen.findByText('MAINTENANCE_SAVE')).toBeVisible();
 fireEvent.click(screen.getByRole('button',{name:'查看详情'}));
 const detail=await screen.findByRole('dialog',{name:'变更详情'});
 expect(within(detail).getByText(/"enabled": true/)).toBeVisible();
 fireEvent.click(within(detail).getByRole('button',{name:'关闭对话框'}));
 fireEvent.click(screen.getByRole('button',{name:'下一页'}));
 expect(await screen.findByText('USER_UPDATE')).toBeVisible();
 fireEvent.change(screen.getByRole('textbox',{name:'操作人 ID'}),{target:{value:'1'}});
 fireEvent.change(screen.getByRole('textbox',{name:'对象类型'}),{target:{value:'MAINTENANCE'}});
 fireEvent.click(screen.getByRole('button',{name:'筛选'}));
 await waitFor(()=>expect(fetcher.mock.calls.some(([path])=>path.includes('actor_user_id=1')&&path.includes('target_type=MAINTENANCE')&&path.includes('page=1'))).toBe(true));
 expect(fetcher.mock.calls.some(([path])=>path.includes('/ops/bootstrap')||path.includes('/ops/operations')||path.includes('/ops/fresh')||path.includes('/native-runtime')||path.includes('authz_epoch'))).toBe(false);
});


it('manages local users and filtered relay logs without legacy operations', async()=>{
 let u={id:2,username:'member',display_name:'测试用户',discord_id:'123456789',group:'default',role:1,status:1,two_fa_enabled:true,reserve:'2',wallet_initialized:true,created_at:'2026-10-12T10:00:00Z',last_login_at:null};
 let keys=[{id:7,name:'user key',key:'abcd**********wxyz',status:1,purpose:'GENERAL'}];
 const writes:{path:string;body:Record<string,unknown>}[]=[],ledger:unknown[]=[];
 let adjustment='';let readsFail=false;
 vi.spyOn(window,'confirm').mockReturnValue(true);
 const fetcher=vi.fn(async(path:string,init?:RequestInit)=>{
  if(path==='/api/v1/session/bootstrap')return ok({authentication_state:'AUTHENTICATED',csrf_token:'local-csrf'});
  if(path==='/api/user/self')return ok({id:1,username:'operator',role:100});
  if(path.startsWith('/platform/v1/access-gate?'))return ok({stage:'READY',user_id:'1',route:new URLSearchParams(path.split('?')[1]).get('route')});
  if(path==='/api/v1/maintenance/notices')return ok({items:[]});
  if(path==='/api/v1/ops/groups')return ok([{name:'default'},{name:'工作'}]);
  if(path.startsWith('/api/v1/ops/users?'))return ok({items:[u],page:1,page_size:20,total:1});
  if(path.startsWith('/api/v1/ops/users/2/ledger?'))return ok({items:ledger,total:ledger.length,page:1,page_size:20});
  if(path.startsWith('/api/v1/ops/users/2/keys?'))return ok({items:keys,total:keys.length,page:1,page_size:20});
  if(path==='/api/v1/ops/users/2'&&init?.method==='GET')return readsFail?failed():ok(u);
  if(path.startsWith('/api/v1/ops/users/2')&&init?.method!=='GET'){
   const body=JSON.parse(String(init?.body));writes.push({path,body});
   if(path==='/api/v1/ops/users/2'){u={...u,...body};readsFail=true}
   if(path.endsWith('/reset-2fa'))u.two_fa_enabled=false;
   if(path.endsWith('/reserve')){
    if(!adjustment){adjustment=body.request_id;u.reserve='2.000002';ledger.push({id:'entry',created_at:u.created_at,biz_type:'OPS_ADJUST',delta_amount:'0.000002',balance_after_amount:u.reserve,note:body.note});return failed()}
    expect(body.request_id).toBe(adjustment);
   }
   if(path.endsWith('/disable-key'))keys[0].status=2;
   if(path.endsWith('/delete-key'))keys=[];
   return ok(null);
  }
  if(path.startsWith('/api/v1/ops/logs?'))return ok({items:[{id:1,user_id:2,username:'member',key_id:7,key_name:'user key',model:'gpt-test',channel_id:20,channel_name:'dev channel',group:'工作',created_at:u.created_at,tokens:'15',cost:'0.000002',duration_ms:120,status_code:200,error:'',kind:2,request_id:'req',attempt:2}],total:1,page:1,page_size:20,summary:{requests:1,attempts:2,errors:0,tokens:'15',cost:'0.000002',error_rate:0},top:[{name:'gpt-test',requests:1,attempts:2,errors:0,tokens:'15',cost:'0.000002',error_rate:0}]});
  throw new Error('Unexpected endpoint: '+path);
 });
 const client=new ApiClient(fetcher);await client.bootstrap();
 render(<MemoryRouter initialEntries={['/ops/users']}><App client={client}/></MemoryRouter>);
 fireEvent.change(await screen.findByLabelText('用户名、显示名、Discord ID 或 ID'),{target:{value:'member'}});
 fireEvent.click(await screen.findByRole('link',{name:'member · #2'}));
 fireEvent.change(await screen.findByLabelText('用户分组'),{target:{value:'工作'}});
 fireEvent.change(screen.getByLabelText('用户角色'),{target:{value:'10'}});
 fireEvent.click(screen.getByRole('button',{name:'保存资料'}));
 expect(await screen.findByText('操作已保存，详情读取失败，请刷新核对。')).toBeVisible();
 readsFail=false;fireEvent.click(screen.getByRole('button',{name:'刷新详情'}));
 expect(await screen.findByLabelText('用户分组')).toHaveValue('工作');
 expect(writes[0]).toEqual({path:'/api/v1/ops/users/2',body:{group:'工作',status:1,role:10}});
 fireEvent.click(screen.getByRole('button',{name:'重置 2FA'}));
 expect(await screen.findByText('2FA：未启用')).toBeVisible();
 fireEvent.click(screen.getByRole('button',{name:'强制下线'}));
 await waitFor(()=>expect(writes.some(w=>w.path.endsWith('/logout'))).toBe(true));
 await screen.findByLabelText('用户分组');
 fireEvent.click(screen.getByRole('button',{name:'余额与流水'}));
 fireEvent.change(await screen.findByLabelText('调整金额（负数扣减）'),{target:{value:'0.000002'}});
 fireEvent.change(screen.getByLabelText('调账备注'),{target:{value:'核对最小单位'}});
 fireEvent.click(screen.getByRole('button',{name:'保存调账'}));
 await screen.findByRole('alert');
 fireEvent.click(screen.getByRole('button',{name:'保存调账'}));
 expect(await screen.findByText('OPS_ADJUST')).toBeVisible();
 expect(screen.getByText('核对最小单位')).toBeVisible();
 expect(writes.filter(w=>w.path.endsWith('/reserve'))).toHaveLength(2);
 fireEvent.click(screen.getByRole('button',{name:'API Key'}));
 fireEvent.click(await screen.findByRole('button',{name:'停用 Key'}));
 await screen.findByText('停用 / GENERAL');
 fireEvent.click(screen.getByRole('button',{name:'删除 Key'}));
 await waitFor(()=>expect(screen.queryByText('user key')).not.toBeInTheDocument());
 fireEvent.click(screen.getAllByRole('link',{name:'调用日志'})[1]);
 expect(await screen.findByRole('heading',{name:'全局调用日志'})).toBeVisible();
 expect(screen.getByLabelText('用户 ID')).toHaveValue('2');
 await screen.findByText(/请求 1 · 尝试 2/);
 fireEvent.change(screen.getByLabelText('汇总维度'),{target:{value:'user'}});
 fireEvent.click(screen.getByLabelText('仅错误'));
 await waitFor(()=>expect(fetcher.mock.calls.some(([path])=>path.includes('/ops/logs?')&&path.includes('user=2')&&path.includes('dimension=user')&&path.includes('errors_only=true'))).toBe(true));
 expect(fetcher.mock.calls.some(([path])=>path.includes('bootstrap')&&path.includes('/ops/'))).toBe(false);
 expect(writes.some(w=>JSON.stringify(w.body).includes('authz_epoch'))).toBe(false);
});
