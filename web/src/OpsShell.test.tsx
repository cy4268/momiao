import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, it, vi } from 'vitest';
import { ApiClient } from './api';
import { App } from './App';
import { ok, failed } from './m1-test-fixtures';

it('saves maintenance directly with a local admin and shows all operations without legacy bootstrap', async () => {
 let state={enabled:false,message:'',scopes:[] as string[]};
 let failRead=true;
 let rebuilt=false,published=false,rankingReadsFail=false;
 const rankingWrites:{path:string;body:unknown}[]=[];
 const snapshotID='10000000-0000-4000-8000-000000000072';
 const sliceID='GAMES:GAME_PROFIT:DAY:1791820800';
 const snapshot=()=>({snapshot_id:snapshotID,slice_target_id:sliceID,domain:'GAMES',period:'DAY',period_start:'2026-10-12T16:00:00Z',period_end:null,build_kind:'REPAIR',source_checked_at:'2026-10-13T06:00:00Z',built_at:'2026-10-13T06:00:00Z',entry_count:'2',aggregate_hash:'abc',published,pointer_version:published?'2':'0',current_pointer_version:published?'2':'1'});
 vi.spyOn(window,'confirm').mockReturnValue(true);
 const fetcher=vi.fn(async(path:string,init?:RequestInit)=>{
  if(path==='/api/v1/session/bootstrap')return ok({authentication_state:'AUTHENTICATED',csrf_token:'local-csrf'});
  if(path==='/api/user/self')return ok({id:1,username:'operator',role:10});
  if(path.startsWith('/platform/v1/access-gate?'))return ok({stage:'READY',user_id:'1',route:new URLSearchParams(path.split('?')[1]).get('route')});
  if(path==='/api/v1/maintenance/notices')return ok({items:state.enabled?[{id:'maintenance',message:state.message,scopes:state.scopes}]:[]});
  if(path.startsWith('/api/v1/ops/audit?')){
   const query=new URLSearchParams(path.split('?')[1]);const page=Number(query.get('page')||1);
   return ok({items:[{audit_id:'10000000-0000-4000-8000-000000000001',actor_newapi_user_id:'1',actor_name:'operator',action:page===1?'MAINTENANCE_SAVE':'USER_UPDATE',target_type:'MAINTENANCE',target_id:'DEVELOPMENT',before_snapshot:{enabled:false},after_snapshot:{enabled:true},occurred_at:'2026-10-12T23:00:00Z'}],total:21,page,page_size:20});
  }
  if(path==='/api/v1/ops/rankings/rebuilds'){
   rankingWrites.push({path,body:JSON.parse(String(init?.body))});rebuilt=true;return ok({job_id:'10000000-0000-4000-8000-000000000073'});
  }
  if(path===`/api/v1/ops/rankings/snapshots/${snapshotID}/publish`){
   rankingWrites.push({path,body:JSON.parse(String(init?.body))});published=true;rankingReadsFail=true;return ok({snapshot_id:snapshotID,state:'PUBLISHED'});
  }
  if(path===`/api/v1/ops/rankings/snapshots/${snapshotID}`)return ok({snapshot:snapshot(),current:null,diff:{old_count:'1',new_count:'2',added:'1',removed:'0',changed:'0',old_hash:null,new_hash:'abc'}});
  if(path.startsWith('/api/v1/ops/rankings?')){
   if(rankingReadsFail)return failed();
   const page=Number(new URLSearchParams(path.split('?')[1]).get('page'));
   return ok({page,page_size:20,job_total:21,shadow_total:rebuilt&&!published?1:0,generated_at:'2026-10-13T06:00:00Z',aggregation_status:[{slice_target_id:sliceID,domain:'GAMES',metric:'GAME_PROFIT',period:'DAY',date:'2026-10-13',state:'READY',last_source_checked_at:null,lag_seconds:null}],published:published?[snapshot()]:[],shadow_rebuilds:rebuilt&&!published?[snapshot()]:[],rebuild_jobs:rebuilt?[{job_id:'10000000-0000-4000-8000-000000000073',metric:'GAME_PROFIT',period:'DAY',status:published?'PUBLISHED':'SHADOW',attempts:'1',shadow_snapshot_id:snapshotID,updated_at:'2026-10-13T06:00:00Z'}]:[]});
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
 fireEvent.click(within(screen.getByRole('navigation',{name:'运营导航'})).getByRole('link',{name:'排行榜'}));
 expect(await screen.findByRole('heading',{name:'排行榜管理'})).toBeVisible();
 fireEvent.click(await screen.findByRole('button',{name:'重建'}));
 fireEvent.click(screen.getByRole('button',{name:'创建重建任务'}));
 expect(await screen.findByRole('status')).toHaveTextContent('已创建重建任务');
 expect(rankingWrites[0].body).toEqual({domain:'GAMES',metric:'GAME_PROFIT',period:'DAY',date:'2026-10-13',model_scope:'ALL'});
 fireEvent.click(await screen.findByRole('button',{name:'查看生成的快照'}));
 const candidate=await screen.findByRole('dialog',{name:'快照与差异'});
 fireEvent.click(await within(candidate).findByRole('button',{name:'发布候选'}));
 expect(await screen.findByRole('status')).toHaveTextContent('候选已发布');
 expect(await screen.findByRole('alert')).toBeVisible();
 expect(rankingWrites).toHaveLength(2);expect(rankingWrites[1].body).toEqual({});
 rankingReadsFail=false;fireEvent.click(screen.getByRole('button',{name:'刷新状态'}));
 expect(await screen.findByRole('region',{name:'已发布快照'})).toBeVisible();
 fireEvent.click(screen.getByRole('button',{name:'下一页'}));
 await waitFor(()=>expect(fetcher.mock.calls.some(([path])=>path.includes('/ops/rankings?')&&path.includes('page=2'))).toBe(true));
 fireEvent.change(screen.getByRole('combobox',{name:'排行榜范围'}),{target:{value:'GAMES'}});
 fireEvent.click(screen.getByRole('button',{name:'筛选排行榜'}));
 await waitFor(()=>expect(fetcher.mock.calls.some(([path])=>path.includes('/ops/rankings?')&&path.includes('domain=GAMES')&&path.includes('page=1'))).toBe(true));
 expect(rankingWrites).toHaveLength(2);
 expect(fetcher.mock.calls.some(([path])=>path.includes('/ops/bootstrap')||path.includes('/ops/operations')||path.includes('/ops/fresh')||path.includes('/native-runtime')||path.includes('authz_epoch'))).toBe(false);
});


it('manages local users and filtered relay logs without legacy operations', async()=>{
 let u={id:2,username:'member',display_name:'测试用户',discord_id:'123456789',group:'default',role:1,status:1,two_fa_enabled:true,reserve:'2',wallet_initialized:true,created_at:'2026-10-12T10:00:00Z',last_login_at:null};
 let keys=[{id:7,name:'user key',key:'abcd**********wxyz',status:1,purpose:'GENERAL'}];
 const writes:{path:string;body:Record<string,unknown>}[]=[],ledger:unknown[]=[];
 let adjustment='';let readsFail=false;let rewardReadsFail=false,requeues=0;
 const rewardID='10000000-0000-4000-8000-000000000061';
 vi.spyOn(window,'confirm').mockReturnValue(true);
 const fetcher=vi.fn(async(path:string,init?:RequestInit)=>{
  if(path==='/api/v1/session/bootstrap')return ok({authentication_state:'AUTHENTICATED',csrf_token:'local-csrf'});
  if(path==='/api/user/self')return ok({id:1,username:'operator',role:100});
  if(path.startsWith('/platform/v1/access-gate?'))return ok({stage:'READY',user_id:'1',route:new URLSearchParams(path.split('?')[1]).get('route')});
  if(path==='/api/v1/maintenance/notices')return ok({items:[]});
  if(path==='/api/v1/ops/economy/overview')return ok({observed_at:'2026-10-13T06:00:00Z',wallet_accounts:'44',reserve_units:'500000',available_chips_units:'0',transactions:'100'});
  if(path.startsWith('/api/v1/ops/economy/user?'))return ok({observed_at:'2026-10-13T06:00:00Z',wallets:[{user_id:'2',asset:'RESERVE_API_CREDIT',balance_units:'500000'}],migration:{registration_status:'CONFIRMED'}});
  if(path.startsWith('/api/v1/ops/economy/ledger?')){
   const q=new URLSearchParams(path.split('?')[1]);const second=q.has('before');
   return ok({items:[{id:second?'second-ledger':'first-ledger',user_id:q.get('newapi_user_id')||'3',asset:'RESERVE_API_CREDIT',delta_units:'-1',balance_before_units:'500000',balance_after_units:'499999',biz_type:'OPS_ADJUST',biz_id:second?'older-adjustment':'latest-adjustment',transaction_id:'00000000-0000-4000-8000-000000000002',entry_type:'OPS_ADJUST',created_at:'2026-10-13T06:00:00Z'}],...(!second?{next_before:'2026-10-13T06:00:00Z',next_before_id:'00000000-0000-4000-8000-000000000002'}:{})});
  }
  if(path.startsWith('/api/v1/ops/economy/transfers?'))return ok({items:[]});
  if(path==='/api/v1/ops/rewards/policies')return ok({items:[{program:'REGISTRATION',policy_version:'1',amount_units:'500000000',schedule:'ONE_TIME_ON_ACCEPTED_REGISTRATION'}]});
  if(path===`/api/v1/ops/rewards/claims/${rewardID}/retry`){requeues++;return ok({claim_id:rewardID})}
  if(path.startsWith('/api/v1/ops/rewards/claims?'))return rewardReadsFail?failed():ok({items:[{id:rewardID,newapi_user_id:'2',program:'REGISTRATION',amount_units:'0',status:'RECOVERING',created_at:'2026-10-13T06:00:00Z',retryable:true,attempts:'2'}]});
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
  if(path.startsWith('/api/v1/ops/records?')){
   const q=new URLSearchParams(path.split('?')[1]),users=q.get('user')?[q.get('user')!]:q.has('cursor')?['4']:['2','3'];
   return ok({items:users.map(user=>({record_type:'DIRECT_PLAY_ROUND',source_id:'01993200-0000-7000-8000-000000000001',newapi_user_id:user,occurred_at:u.created_at,status:'SETTLED',stake_units:'5000000',payout_units:'10000000',net_change_units:'5000000',snapshot:{game_title:'命运骰盅',actor_display_name:'玩家'+user}})),has_more:!q.has('cursor'),next_cursor:q.has('cursor')?null:'next-records',game_options:[{game_slug:'dice',game_title:'命运骰盅',retired:false}]});
  }
  if(path.startsWith('/api/v1/ops/records/')){
   expect(path).not.toContain('authz_epoch');
   return ok({subject_user_id:new URLSearchParams(path.split('?')[1]).get('user'),record_type:'DIRECT_PLAY_ROUND',record:path.includes('/verify?')?{released:false}:path.includes('/transactions/')?{id:'01993200-0000-7000-8000-000000000002',kind:'GAME_PAYOUT',links:[]}:{id:'01993200-0000-7000-8000-000000000001',state:'SETTLED',transactions:[{id:'01993200-0000-7000-8000-000000000002'}]}});
  }
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
 fireEvent.click(screen.getByRole('link',{name:'member'}));
 fireEvent.click(await screen.findByRole('link',{name:'游戏记录'}));
 expect(await screen.findByRole('heading',{name:'全局游戏记录'})).toBeVisible();
 expect(screen.getByLabelText('用户 ID（可选）')).toHaveValue('2');
 await screen.findByRole('button',{name:'读取详情'});
 fireEvent.change(screen.getByLabelText('用户 ID（可选）'),{target:{value:''}});
 fireEvent.click(screen.getByRole('button',{name:'查询业务记录'}));
 await waitFor(()=>expect(screen.getAllByRole('button',{name:'读取详情'})).toHaveLength(2));
 fireEvent.click(screen.getAllByRole('button',{name:'读取详情'})[1]);
 const recordDialog=await screen.findByRole('dialog',{name:'记录详情'});
 await waitFor(()=>expect(fetcher.mock.calls.some(([path])=>path.includes('/ops/records/rounds/')&&path.includes('user=3'))).toBe(true));
 fireEvent.click(await within(recordDialog).findByRole('button',{name:'读取获授权的公平性材料'}));
 expect(await within(recordDialog).findByText('released')).toBeVisible();
 fireEvent.click(within(recordDialog).getByRole('button',{name:/交易 0199/}));
 expect(await within(recordDialog).findByText('GAME_PAYOUT')).toBeVisible();
 expect(fetcher.mock.calls.some(([path])=>path.includes('/ops/records/transactions/')&&path.includes('user=3'))).toBe(true);
 fireEvent.click(within(recordDialog).getByRole('button',{name:'关闭对话框'}));
 fireEvent.click(screen.getByRole('button',{name:'下一页'}));
 expect(await screen.findByText('当时昵称：玩家4')).toBeVisible();
 fireEvent.click(screen.getByRole('button',{name:'上一页'}));
 expect(await screen.findByText('当时昵称：玩家3')).toBeVisible();
 expect(fetcher.mock.calls.some(([path])=>path.includes('bootstrap')&&path.includes('/ops/'))).toBe(false);
 fireEvent.click(within(screen.getByRole('navigation',{name:'运营导航'})).getByRole('link',{name:'经济与账务'}));
 expect(await screen.findByRole('heading',{name:'全局账变'})).toBeVisible();
 expect(await screen.findByText('latest-adjustment')).toBeVisible();
 expect(screen.queryByText('Active API Quota')).not.toBeInTheDocument();
 expect(screen.getByText('−0.000002')).toBeVisible();
 fireEvent.click(screen.getByRole('button',{name:'下一页'}));
 expect(await screen.findByText('older-adjustment')).toBeVisible();
 fireEvent.click(screen.getByRole('button',{name:'上一页'}));
 expect(await screen.findByText('latest-adjustment')).toBeVisible();
 fireEvent.change(screen.getByLabelText('账户编号（可选）'),{target:{value:'2'}});
 fireEvent.click(screen.getByRole('button',{name:'查询账户与账变'}));
 expect(await screen.findByRole('link',{name:'管理账户与调整 Reserve'})).toHaveAttribute('href','/ops/users/2');
 await screen.findByText('latest-adjustment');
 fireEvent.change(screen.getByLabelText('业务类型'),{target:{value:'OPS_ADJUST'}});
 fireEvent.click(screen.getByRole('button',{name:'筛选账变'}));
 await waitFor(()=>expect(fetcher.mock.calls.some(([p])=>p.includes('/economy/ledger?')&&p.includes('newapi_user_id=2')&&p.includes('biz_type=OPS_ADJUST'))).toBe(true));
 fireEvent.click(within(screen.getByRole('navigation',{name:'运营导航'})).getByRole('link',{name:'奖励'}));
 const retry=await screen.findByRole('button',{name:'重试注册奖励'});
 rewardReadsFail=true;fireEvent.click(retry);
 expect(await screen.findByRole('status')).toHaveTextContent('已重新排队');
 expect(await screen.findByRole('alert')).toBeVisible();
 expect(requeues).toBe(1);
 rewardReadsFail=false;fireEvent.click(screen.getByRole('button',{name:'刷新奖励'}));
 await screen.findByRole('button',{name:'重试注册奖励'});expect(requeues).toBe(1);
 const retryCall=fetcher.mock.calls.find(([p])=>p.endsWith('/retry'))!;
 expect(new Headers(retryCall[1]?.headers).get('X-CSRF-Token')).toBe('local-csrf');
 expect(JSON.parse(String(retryCall[1]?.body))).toEqual({});
 expect(fetcher.mock.calls.some(([p])=>p.includes('/ops/bootstrap')||p.includes('/ops/operations')||p.includes('/native-runtime'))).toBe(false);
 expect(writes.some(w=>JSON.stringify(w.body).includes('authz_epoch'))).toBe(false);
});
