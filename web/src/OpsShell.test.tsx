import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, it, vi } from 'vitest';
import { ApiClient } from './api';
import { App } from './App';
import { ok, failed } from './m1-test-fixtures';

it('saves maintenance directly with a local admin and shows all operations without legacy bootstrap', async () => {
 let healthStale=true,healthReady=true;
 const supportID='10000000-0000-7000-8000-000000000081',incidentID='10000000-0000-7000-8000-000000000082';
 let support={case_id:supportID,subject_newapi_user_id:'2',category:'IDENTITY',state:'OPEN',safe_summary:'开发工单',created_by:'1',created_at:'2026-10-13T08:00:00Z',updated_at:'2026-10-13T08:00:00Z'};
 let incident={incident_id:incidentID,title:'开发事件',severity:'WARNING',state:'OPEN',safe_summary:'开发环境检查',created_by:'1',created_at:'2026-10-13T08:00:00Z',updated_at:'2026-10-13T08:00:00Z'};
 const supportWrites:{path:string;body:Record<string,unknown>}[]=[],incidentWrites:{path:string;body:Record<string,unknown>}[]=[];
 const supportFacts:unknown[]=[],incidentEvents:Record<string,unknown>[]=[];
 let supportReadsFail=false;
 let pokerAccepting=true,pokerHidden=false,pokerReadsFail=false;
 const pokerID='10000000-0000-7000-8000-000000000074',messageID='10000000-0000-7000-8000-000000000075';
 const pokerWrites:Record<string,unknown>[]=[];
 const pokerTable=()=>({table_id:pokerID,name:'开发牌桌',lifecycle_state:'WAITING',occupied_seats:'0',max_seats:2,spectator_count:'0',accepting_players:pokerAccepting,allow_new_hands:true,needs_review:false});
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
  if(path.startsWith('/api/v1/ops/jobs?')||path.startsWith('/api/v1/ops/attention?')){
   const q=new URLSearchParams(path.split('?')[1]),attention=path.includes('/attention?'),page=Number(q.get('page'));
   return ok({summary:[{kind:'GAME_ROUNDS',pending_count:'21',attention_count:'1',due_count:'3',oldest_due_at:null}],items:[{kind:attention?'OPS_REMOTE':'GAME_ROUNDS',job_id:attention?'legacy-job':`job-${page}`,target_id:'dev-target',state:attention?'NEEDS_REVIEW':'PENDING',pending:!attention,attention,legacy:attention,run_at:null,updated_at:'2026-10-13T09:00:00Z'}],total:attention?1:21,page,page_size:20});
  }
  if(path==='/api/v1/ops/service-health')return ok({snapshot:{ready:healthReady,checked_at:'2026-10-13T09:00:00Z',components:[{name:'platform_database',state:healthReady?'READY':'UNAVAILABLE'}]},observed_at:'2026-10-13T09:00:00Z',stale_after:'2026-10-13T09:00:45Z',stale:healthStale});
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
  if(path.startsWith('/api/v1/ops/support-cases')){
   if(init?.method!=='GET'){
    const body=JSON.parse(String(init?.body));supportWrites.push({path,body});
    if(path==='/api/v1/ops/support-cases')support={...support,...body};
    else if(path.endsWith('/facts'))supportFacts.push(body);
    else{support={...support,state:body.next_state};supportReadsFail=true}
    return ok(support);
   }
   if(supportReadsFail)return failed();
   if(path.includes('?'))return ok({items:[support],total:21});
   return ok({case:support,verification_facts:supportFacts});
  }
  if(path.startsWith('/api/v1/ops/incidents')){
   if(init?.method!=='GET'){
    const body=JSON.parse(String(init?.body));incidentWrites.push({path,body});
    if(path==='/api/v1/ops/incidents')incident={...incident,...body};
    else if(path.endsWith('/events'))incidentEvents.push({event_id:'dev-event',actor_newapi_user_id:'1',occurred_at:'2026-10-13T08:00:00Z',...body});
    else incident={...incident,state:body.next_state};
    return ok(incident);
   }
   if(path.includes('?'))return ok({items:[incident],total:21});
   return ok({incident,events:incidentEvents});
  }
  if(path==='/api/v1/ops/poker')return ok({blockers:[],service_state:'AVAILABLE'});
  if(path.startsWith('/api/v1/ops/poker/tables?'))return ok({items:[pokerTable()],next_cursor:pokerID});
  if(path.startsWith('/api/v1/ops/poker/sessions?'))return ok({items:[]});
  if(path===`/api/v1/ops/poker/tables/${pokerID}`){
   if(init?.method==='POST'){const input=JSON.parse(String(init.body));pokerWrites.push(input);if(input.command_type==='POKER_CHAT_VISIBILITY_SET')pokerHidden=input.hidden;else{pokerAccepting=input.accepting;pokerReadsFail=true}return ok({table_id:pokerID,status:'SAVED'})}
   if(pokerReadsFail)return failed();
   return ok({table:pokerTable(),players:[],messages:[{message_id:messageID,kind:'USER_TEXT',body:'开发消息',created_at:'2026-10-13T07:00:00Z',moderation_state:pokerHidden?'HIDDEN':'VISIBLE',author:{display_name:'测试玩家',user_id:'2'}}],recent_events:[]});
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
 failRead=false;await act(async()=>{fireEvent.click(screen.getByRole('button',{name:'重新读取'}));});
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
 expect(within(nav).queryByRole('link',{name:'访问控制'})).not.toBeInTheDocument();
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
 fireEvent.click(within(screen.getByRole('navigation',{name:'运营导航'})).getByRole('link',{name:'Poker'}));
 expect(await screen.findByRole('heading',{name:'Poker 管理'})).toBeVisible();
 const tableRegion=await screen.findByRole('region',{name:'Poker 牌桌列表'});
 fireEvent.click(within(tableRegion).getByRole('button',{name:'开发牌桌'}));
 const pokerDialog=await screen.findByRole('dialog',{name:'牌桌详情'});
 fireEvent.click(await within(pokerDialog).findByRole('button',{name:'保存牌桌操作'}));
 expect(await within(pokerDialog).findByRole('status')).toHaveTextContent('已保存');
 expect(await within(pokerDialog).findByRole('alert')).toBeVisible();
 expect(pokerWrites).toEqual([{command_type:'POKER_ACCEPTING_PLAYERS_SET',accepting:false}]);
 pokerReadsFail=false;fireEvent.click(within(pokerDialog).getByRole('button',{name:'刷新本桌'}));
 fireEvent.click(await within(pokerDialog).findByRole('button',{name:'隐藏消息'}));
 expect(await within(pokerDialog).findByRole('button',{name:'恢复消息'})).toBeVisible();
 expect(pokerWrites).toHaveLength(2);expect(pokerWrites[1]).toEqual({command_type:'POKER_CHAT_VISIBILITY_SET',message_id:messageID,hidden:true});
 expect(within(pokerDialog).queryByText('操作原因')).not.toBeInTheDocument();
 fireEvent.click(within(pokerDialog).getByRole('button',{name:'关闭对话框'}));
 const tableSection=screen.getByRole('heading',{name:'牌桌'}).closest('section')!;
 fireEvent.click(within(tableSection).getByRole('button',{name:'下一页'}));
 await waitFor(()=>expect(fetcher.mock.calls.some(([path])=>path.includes('/poker/tables?')&&path.includes('cursor='+pokerID))).toBe(true));
 fireEvent.change(screen.getByRole('textbox',{name:'牌桌名称'}),{target:{value:'开发'}});
 fireEvent.click(screen.getByRole('button',{name:'筛选牌桌'}));
 await waitFor(()=>expect(fetcher.mock.calls.some(([path])=>path.includes('q=%E5%BC%80%E5%8F%91')&&path.endsWith('cursor='))).toBe(true));
 expect(pokerWrites).toHaveLength(2);
 fireEvent.click(within(screen.getByRole('navigation',{name:'运营导航'})).getByRole('link',{name:'支持工单'}));
 fireEvent.click(await screen.findByRole('button',{name:'新建工单'}));
 const createCase=await screen.findByRole('dialog',{name:'新建工单'});
 fireEvent.change(within(createCase).getByRole('spinbutton',{name:'工单账户'}),{target:{value:'2'}});
 fireEvent.change(within(createCase).getByRole('textbox',{name:'摘要'}),{target:{value:'直接新建工单'}});
 fireEvent.click(within(createCase).getByRole('button',{name:'保存新工单'}));
 const caseDialog=await screen.findByRole('dialog',{name:'工单详情'});
 fireEvent.click(await within(caseDialog).findByRole('button',{name:'保存核验事实'}));
 await waitFor(()=>expect(supportWrites).toHaveLength(2));
 expect(supportWrites[0].body).toEqual({subject_newapi_user_id:'2',category:'ACCOUNT_ACCESS',safe_summary:'直接新建工单'});
 expect(supportWrites[1].body).toEqual({fact_type:'IDENTITY_MATCH',result:'INCONCLUSIVE',safe_reference:''});
 fireEvent.click(await within(caseDialog).findByRole('button',{name:'保存工单状态'}));
 expect(await within(caseDialog).findByRole('status')).toHaveTextContent('工单已保存');
 expect(await within(caseDialog).findByRole('alert')).toBeVisible();
 expect(supportWrites).toHaveLength(3);expect(supportWrites[2].body).toEqual({next_state:'VERIFYING'});
 supportReadsFail=false;fireEvent.click(screen.getByRole('button',{name:'刷新工单'}));
 expect(await within(caseDialog).findByRole('button',{name:'保存工单状态'})).toBeVisible();
 fireEvent.click(within(caseDialog).getByRole('button',{name:'关闭对话框'}));
 fireEvent.click(screen.getByRole('button',{name:'下一页'}));
 await waitFor(()=>expect(fetcher.mock.calls.some(([path])=>path.includes('/support-cases?page=2'))).toBe(true));
 fireEvent.change(screen.getByRole('spinbutton',{name:'账户编号'}),{target:{value:'2'}});
 fireEvent.change(screen.getByRole('combobox',{name:'工单状态'}),{target:{value:'VERIFYING'}});
 fireEvent.click(screen.getByRole('button',{name:'筛选工单'}));
 await waitFor(()=>expect(fetcher.mock.calls.some(([path])=>path.includes('/support-cases?page=1')&&path.includes('subject_newapi_user_id=2')&&path.includes('state=VERIFYING'))).toBe(true));
 expect(supportWrites).toHaveLength(3);
 fireEvent.click(within(screen.getByRole('navigation',{name:'运营导航'})).getByRole('link',{name:'事件处理'}));
 fireEvent.click(await screen.findByRole('button',{name:'创建事件'}));
 const createIncident=await screen.findByRole('dialog',{name:'创建事件'});
 fireEvent.change(within(createIncident).getByRole('textbox',{name:'标题'}),{target:{value:'直接新建事件'}});
 fireEvent.change(within(createIncident).getByRole('textbox',{name:'业务影响摘要'}),{target:{value:'仅开发验收'}});
 fireEvent.click(within(createIncident).getByRole('button',{name:'保存新事件'}));
 const incidentDialog=await screen.findByRole('dialog',{name:'事件详情'});
 fireEvent.change(await within(incidentDialog).findByRole('textbox',{name:'处理备注'}),{target:{value:'已经检查'}});
 fireEvent.click(within(incidentDialog).getByRole('button',{name:'保存处理记录'}));
 expect(await within(incidentDialog).findByText('已经检查')).toBeVisible();
 fireEvent.change(within(incidentDialog).getByRole('combobox',{name:'下一状态'}),{target:{value:'CLOSED'}});
 fireEvent.click(within(incidentDialog).getByRole('button',{name:'保存事件状态'}));
 await waitFor(()=>expect(within(incidentDialog).queryByRole('button',{name:'保存处理记录'})).not.toBeInTheDocument());
 expect(window.confirm).toHaveBeenCalledWith('确认关闭这项事件？');
 expect(incidentWrites.map(v=>v.body)).toEqual([{title:'直接新建事件',severity:'WARNING',safe_summary:'仅开发验收'},{event_type:'COMMENT',safe_summary:'已经检查'},{next_state:'CLOSED'}]);
 fireEvent.click(within(incidentDialog).getByRole('button',{name:'关闭对话框'}));
 fireEvent.click(screen.getByRole('button',{name:'下一页'}));
 await waitFor(()=>expect(fetcher.mock.calls.some(([path])=>path.includes('/incidents?page=2'))).toBe(true));
 fireEvent.change(screen.getByRole('textbox',{name:'搜索事件'}),{target:{value:'直接'}});
 fireEvent.click(screen.getByRole('button',{name:'筛选事件'}));
 await waitFor(()=>expect(fetcher.mock.calls.some(([path])=>path.includes('/incidents?page=1')&&path.includes('q=%E7%9B%B4%E6%8E%A5'))).toBe(true));
 fireEvent.click(within(screen.getByRole('navigation',{name:'运营导航'})).getByRole('link',{name:'后台任务'}));
 fireEvent.click(await screen.findByRole('button',{name:'查看任务'}));
 const jobDialog=await screen.findByRole('dialog',{name:'任务详情'});
 expect(within(jobDialog).getByText('job-1')).toBeVisible();
 expect(within(jobDialog).getByRole('link',{name:'进入业务页面 →'})).toHaveAttribute('href','/ops/games');
 fireEvent.click(within(jobDialog).getByRole('button',{name:'关闭对话框'}));
 fireEvent.click(screen.getByRole('button',{name:'下一页'}));
 await waitFor(()=>expect(fetcher.mock.calls.some(([path])=>path==='/api/v1/ops/jobs?page=2')).toBe(true));
 fireEvent.change(screen.getByRole('combobox',{name:'任务类型'}),{target:{value:'GAME_ROUNDS'}});
 fireEvent.change(screen.getByRole('combobox',{name:'任务状态'}),{target:{value:'PENDING'}});
 fireEvent.change(screen.getByLabelText('记录时间从'),{target:{value:'2026-10-13T00:00'}});
 fireEvent.click(screen.getByRole('button',{name:'筛选任务'}));
 await waitFor(()=>expect(fetcher.mock.calls.some(([path])=>path.includes('/ops/jobs?page=1')&&path.includes('kind=GAME_ROUNDS')&&path.includes('status=PENDING')&&path.includes('from='))).toBe(true));
 expect(await screen.findByText('全局队列概况（不受上方筛选影响）')).toBeVisible();
 fireEvent.click(within(screen.getByRole('navigation',{name:'运营导航'})).getByRole('link',{name:'待处理事项'}));
 fireEvent.click(await screen.findByRole('button',{name:'查看任务'}));
 const attentionDialog=await screen.findByRole('dialog',{name:'任务详情'});
 expect(within(attentionDialog).getByText('legacy-job')).toBeVisible();
 expect(within(attentionDialog).getByText('旧流程记录；此页不执行旧命令。')).toBeVisible();
 expect(within(attentionDialog).getByRole('link',{name:'进入业务页面 →'})).toHaveAttribute('href','/ops/poker');
 fireEvent.click(within(attentionDialog).getByRole('button',{name:'关闭对话框'}));
 fireEvent.click(within(screen.getByRole('navigation',{name:'运营导航'})).getByRole('link',{name:'服务健康'}));
 expect(await screen.findByRole('heading',{name:'服务状态快照已过期'})).toBeVisible();
 expect(screen.getByText('未知（快照过期）')).toBeVisible();
 healthStale=false;healthReady=false;fireEvent.click(screen.getByRole('button',{name:'刷新快照'}));
 expect(await screen.findByRole('heading',{name:'存在不可用或尚未配置的依赖'})).toBeVisible();
 expect(screen.getByText('不可用')).toBeVisible();
 healthReady=true;fireEvent.click(screen.getByRole('button',{name:'刷新快照'}));
 expect(await screen.findByRole('heading',{name:'已配置依赖检查通过'})).toBeVisible();
 expect(fetcher.mock.calls.filter(([path])=>path.includes('/ops/jobs?')||path.includes('/ops/attention?')||path.endsWith('/ops/service-health')).every(([,init])=>init?.method==='GET')).toBe(true);
 expect(screen.queryByText('本次操作原因')).not.toBeInTheDocument();
 expect(fetcher.mock.calls.some(([path])=>path.includes('/ops/bootstrap')||path.includes('/ops/operations')||path.includes('/ops/fresh')||path.includes('/native-runtime')||path.includes('authz_epoch'))).toBe(false);
});


it('manages local users and filtered relay logs without legacy operations', async()=>{
 let u={id:2,username:'member',display_name:'测试用户',master_name:'原昵称',discord_id:'123456789',group:'default',role:1,status:1,two_fa_enabled:true,reserve:'2',wallet_initialized:true,created_at:'2026-10-12T10:00:00Z',last_login_at:null};
 let keys=[{id:7,name:'user key',key:'abcd**********wxyz',status:1,purpose:'GENERAL'}];
 const writes:{path:string;body:Record<string,unknown>}[]=[],ledger:unknown[]=[];
 let adjustment='';let readsFail=false;let rewardReadsFail=false,requeues=0;let assetReadsFail=false,chips='1000000';
 let gamblerPhase='SNAPSHOT_READY',gamblerReadsFail=false;const gamblerWrites:unknown[]=[];
 const gamblerRoot='/api/v1/ops/economy/gambler-campaigns/01995000-2026-7000-8000-000000000927';
 let master={newapi_user_id:'2',display_name:'原昵称',profile_version:'1',rename_required:false,nickname_changed_at:null};let masterReadsFail=false;
 const rewardID='10000000-0000-4000-8000-000000000061';
 vi.spyOn(window,'confirm').mockReturnValue(true);
 const fetcher=vi.fn(async(path:string,init?:RequestInit)=>{
  if(path==='/api/v1/session/bootstrap')return ok({authentication_state:'AUTHENTICATED',csrf_token:'local-csrf'});
  if(path==='/api/user/self')return ok({id:1,username:'operator',role:100});
  if(path.startsWith('/platform/v1/access-gate?'))return ok({stage:'READY',user_id:'1',route:new URLSearchParams(path.split('?')[1]).get('route')});
  if(path==='/api/v1/maintenance/notices')return ok({items:[]});
  if(path===gamblerRoot&&init?.method==='POST'){
   gamblerWrites.push(JSON.parse(String(init.body)));gamblerPhase='GRANTED';gamblerReadsFail=true;return ok({phase:gamblerPhase});
  }
  if(path.startsWith(gamblerRoot+'?')){
   const q=new URLSearchParams(path.split('?')[1]);const user=q.get('user')||(q.get('page')==='2'?'25':'2');
   return gamblerReadsFail?failed():ok({id:'01995000-2026-7000-8000-000000000927',phase:gamblerPhase,cutoff_at:u.created_at,reset_cutoff_at:null,target_count:'25',completed_count:'0',eligible_count:'4',blocking_facts:[],policy_activated_at:null,rankings_published_at:null,total:q.has('user')?1:25,accounts:[{user_id:user,eligible:true,qualification_units:'500000000000001',reset_state:'PENDING',reset_before:{reserve_units:'500000'},reset_after:null,reset_receipt:{record:'historical-checkpoint'}}]});
  }
  if(path==='/api/v1/ops/economy/overview')return ok({observed_at:'2026-10-13T06:00:00Z',wallet_accounts:'44',reserve_units:'500000',available_chips_units:'0',transactions:'100'});
  if(path.startsWith('/api/v1/ops/economy/user?'))return assetReadsFail?failed():ok({observed_at:'2026-10-13T06:00:00Z',assets:{reserve_units:'500000',available_chips_units:chips,poker_stack_units:'1500000',poker_pot_units:'2000000',single_player_in_flight_units:'2500000',roulette_escrow_units:'3000000',total_units:String(9500000+Number(chips)),observed_at:'2026-10-13T06:00:00Z'},migration:{registration_status:'CONFIRMED'}});
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
  if(path==='/api/v1/ops/users/2/master'&&init?.method==='GET')return masterReadsFail?failed():ok(master);
  if(path.startsWith('/api/v1/ops/users/2')&&init?.method!=='GET'){
   const body=JSON.parse(String(init?.body));writes.push({path,body});
   if(path==='/api/v1/ops/users/2'){u={...u,...body};readsFail=true}
   if(path.endsWith('/reset-2fa'))u.two_fa_enabled=false;
   if(path.endsWith('/chips')){chips='999999';assetReadsFail=true}
   if(path.endsWith('/master-rename-required'))master={...master,rename_required:body.required};
   if(path.endsWith('/master-force-rename')){master={...master,display_name:body.display_name,rename_required:false};u.master_name=body.display_name;masterReadsFail=true}
   if(path.endsWith('/reserve')){
    if(!adjustment){adjustment=body.request_id;u.reserve='1.999998';ledger.push({id:'entry',created_at:u.created_at,biz_type:'OPS_ADJUST',delta_amount:'-0.000002',balance_after_amount:u.reserve,note:body.note});return failed()}
    expect(body.request_id).toBe(adjustment);
   }
   if(path.endsWith('/disable-key'))keys[0].status=2;
   if(path.endsWith('/delete-key'))keys=[];
   return ok(null);
  }
  if(path.startsWith('/api/v1/ops/channels?'))return ok({items:[{id:20,name:'dev channel',models:['gpt-test']}],total:1});
  const log={id:1,user_id:2,username:'member',display_name:u.master_name,key_id:7,key_name:'user key',model:'gpt-test',channel_id:20,channel_name:'dev channel',group:'工作',created_at:u.created_at,tokens:'15',prompt_tokens:'10',completion_tokens:'5',cache_read_tokens:'0',cache_write_tokens:'0',cache_write_tokens_1h:'0',cost_units:'1',is_stream:true,cost:'0.000002',duration_ms:120,status_code:200,error:'',kind:2,request_id:'req',attempt:2};
  if(path==='/api/v1/ops/logs/1')return ok({items:[{...log,id:2,attempt:1,kind:5,status_code:502,error:'upstream failed'},log]});
  if(path.startsWith('/api/v1/ops/logs?'))return ok({items:[log],total:1,page:1,page_size:20,summary:{requests:1,attempts:2,errors:0,tokens:'15',cost:'0.000002',error_rate:0},top:[{name:'gpt-test',requests:1,attempts:2,errors:0,tokens:'15',cost:'0.000002',error_rate:0}]});
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
 fireEvent.change(await screen.findByLabelText('用户名、显示名、Discord ID 或 ID'),{target:{value:'原昵称'}});
 await waitFor(()=>expect(fetcher.mock.calls.some(([p])=>p.includes('/ops/users?')&&new URLSearchParams(p.split('?')[1]).get('q')==='原昵称')).toBe(true));
 const userReads=()=>fetcher.mock.calls.filter(([p])=>p.startsWith('/api/v1/ops/users?')).length;
 const beforeOpen=userReads();
 fireEvent.click(await screen.findByRole('button',{name:'原昵称 · #2'}));
 expect(await screen.findByRole('dialog',{name:'原昵称 · #2'})).toBeVisible();
 fireEvent.click(screen.getByRole('button',{name:'关闭对话框'}));
 expect(screen.getByLabelText('用户名、显示名、Discord ID 或 ID')).toHaveValue('原昵称');
 expect(userReads()).toBe(beforeOpen);
 fireEvent.click(screen.getByRole('button',{name:'原昵称 · #2'}));
 fireEvent.change(await screen.findByLabelText('用户分组'),{target:{value:'工作'}});
 fireEvent.change(screen.getByLabelText('用户角色'),{target:{value:'10'}});
 fireEvent.change(screen.getByLabelText('账号状态'),{target:{value:'2'}});
 vi.mocked(window.confirm).mockReturnValueOnce(false);
 fireEvent.click(screen.getByRole('button',{name:'保存资料'}));
 expect(window.confirm).toHaveBeenLastCalledWith('确认封禁此用户并撤销全部会话？');
 expect(writes).toHaveLength(0);
 fireEvent.click(screen.getByRole('button',{name:'保存资料'}));
 expect(await screen.findByText('操作已保存，详情读取失败，请刷新核对。')).toBeVisible();
 readsFail=false;fireEvent.click(screen.getByRole('button',{name:'刷新详情'}));
 expect(await screen.findByLabelText('用户分组')).toHaveValue('工作');
 expect(writes[0]).toEqual({path:'/api/v1/ops/users/2',body:{group:'工作',status:2,role:10}});
 fireEvent.click(screen.getByRole('button',{name:'重置 2FA'}));
 expect(await screen.findByText('2FA：未启用')).toBeVisible();
 fireEvent.click(screen.getByRole('button',{name:'强制下线'}));
 await waitFor(()=>expect(writes.some(w=>w.path.endsWith('/logout'))).toBe(true));
 await screen.findByLabelText('用户分组');
 fireEvent.click(screen.getByRole('button',{name:'Master 资料'}));
 fireEvent.click(await screen.findByLabelText('要求用户改名'));
 fireEvent.click(screen.getByRole('button',{name:'保存改名要求'}));
 await waitFor(()=>expect(writes.find(w=>w.path.endsWith('/master-rename-required'))?.body).toEqual({required:true}));
 expect(await screen.findByLabelText('要求用户改名')).toBeChecked();
 fireEvent.change(screen.getByLabelText('Master 昵称'),{target:{value:'新昵称'}});
 fireEvent.click(screen.getByRole('button',{name:'强制改名'}));
 expect(await screen.findByRole('alert')).toBeVisible();
 expect(screen.getByText('操作已保存')).toBeVisible();
 expect(writes.find(w=>w.path.endsWith('/master-force-rename'))?.body).toEqual({display_name:'新昵称'});
 masterReadsFail=false;fireEvent.click(screen.getByRole('button',{name:'刷新 Master 资料'}));
 expect(await screen.findByLabelText('Master 昵称')).toHaveValue('新昵称');
 expect(screen.getByLabelText('要求用户改名')).not.toBeChecked();
 expect(writes.filter(w=>w.path.includes('/master-'))).toHaveLength(2);
 master={...master,profile_version:'0'};fireEvent.click(screen.getByRole('button',{name:'刷新 Master 资料'}));
 expect(await screen.findByText('此账号尚未完成 Master 资料，不创建资料或钱包。')).toBeVisible();
 expect(screen.queryByRole('button',{name:'强制改名'})).not.toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'余额与流水'}));
 fireEvent.change(await screen.findByLabelText('调整金额（负数扣减）'),{target:{value:'-0.000002'}});
 fireEvent.change(screen.getByLabelText('调账备注'),{target:{value:'核对最小单位'}});
 vi.mocked(window.confirm).mockReturnValueOnce(false);
 fireEvent.click(screen.getByRole('button',{name:'保存调账'}));
 expect(window.confirm).toHaveBeenLastCalledWith('确认扣减此用户的 Reserve？');
 expect(writes.filter(w=>w.path.endsWith('/reserve'))).toHaveLength(0);
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
 // The prefilled user label arrives after the lookup, not with the input itself.
 await waitFor(()=>expect(screen.getByLabelText('搜索用户')).toHaveValue('新昵称 · #2'));
 await screen.findByText(/请求 1 · 尝试 2/);
 fireEvent.change(screen.getByLabelText('搜索用户'),{target:{value:'新昵称'}});
 fireEvent.click(await screen.findByRole('button',{name:'新昵称 · #2'}));
 vi.useFakeTimers({toFake:['Date']});
 try{
  vi.setSystemTime(new Date('2026-10-14T10:00:00Z'));
  fireEvent.change(screen.getByLabelText('时间范围'),{target:{value:'7d'}});
  await waitFor(()=>expect(fetcher.mock.calls.some(([path])=>{const q=new URLSearchParams(path.split('?')[1]);return path.includes('/ops/logs?')&&q.get('user')==='2'&&Number(q.get('end'))-Number(q.get('start'))===7*86400})).toBe(true));
  vi.setSystemTime(new Date('2026-10-14T10:05:00Z'));
  fireEvent.click(screen.getByRole('button',{name:'刷新日志'}));
  await waitFor(()=>expect(fetcher.mock.calls.some(([path])=>{const q=new URLSearchParams(path.split('?')[1]);return path.includes('/ops/logs?')&&q.get('user')==='2'&&Number(q.get('end'))===Date.now()/1000&&Number(q.get('end'))-Number(q.get('start'))===7*86400})).toBe(true));
 }finally{vi.useRealTimers()}
 expect(screen.getByRole('combobox',{name:'渠道'})).toHaveTextContent('dev channel');
 expect(screen.getByRole('combobox',{name:'模型'})).toHaveTextContent('gpt-test');
 const logsReads=fetcher.mock.calls.filter(([p])=>p.startsWith('/api/v1/ops/logs?')).length;
 fireEvent.click(screen.getByRole('button',{name:'日志 #1'}));
 const logDrawer=await screen.findByRole('dialog',{name:'调用详情 #1'});
 expect(await within(logDrawer).findByText('upstream failed')).toBeVisible();
 expect(within(logDrawer).getByText('第 2 次尝试')).toBeVisible();
 expect(within(logDrawer).getAllByText('req')).toHaveLength(2);
 fireEvent.click(within(logDrawer).getByRole('button',{name:'关闭对话框'}));
 expect(fetcher.mock.calls.filter(([p])=>p.startsWith('/api/v1/ops/logs?')).length).toBe(logsReads);
 fireEvent.click(screen.getByText('调用 Top 20'));
 fireEvent.change(screen.getByLabelText('汇总维度'),{target:{value:'user'}});
 fireEvent.click(screen.getByLabelText('仅错误'));
 await waitFor(()=>expect(fetcher.mock.calls.some(([path])=>path.includes('/ops/logs?')&&path.includes('user=2')&&path.includes('dimension=user')&&path.includes('errors_only=true'))).toBe(true));
 fireEvent.click(screen.getByRole('link',{name:'新昵称'}));
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
 expect(screen.getByText(/总资产：21 ·/)).toBeVisible();
 for(const label of ['扑克桌面','扑克底池','单人游戏在途','轮盘托管'])expect(screen.getByText(label)).toBeVisible();
 fireEvent.change(screen.getByLabelText('调整可用筹码（负数扣减）'),{target:{value:'-0.000002'}});
 fireEvent.change(screen.getByLabelText('筹码调账备注'),{target:{value:'本地筹码调整'}});
 fireEvent.click(screen.getByRole('button',{name:'保存筹码调账'}));
 expect(await screen.findByText('调账已保存，资产读取失败，请重新查询账户。')).toBeVisible();
 expect(screen.getByRole('status')).toHaveTextContent('筹码调账已保存');
 expect(window.confirm).toHaveBeenCalledWith('确认扣减此账户的可用筹码？');
 assetReadsFail=false;fireEvent.click(screen.getByRole('button',{name:'查询账户与账变'}));
 expect(await screen.findByText(/总资产：20.999998 ·/)).toBeVisible();
 const chipWrites=writes.filter(w=>w.path.endsWith('/chips'));
 expect(chipWrites).toHaveLength(1);
 expect(chipWrites[0].body).toEqual({amount:'-0.000002',note:'本地筹码调整',request_id:expect.any(String)});
 await screen.findByText('latest-adjustment');
 fireEvent.change(screen.getByLabelText('业务类型'),{target:{value:'OPS_ADJUST'}});
 fireEvent.click(screen.getByRole('button',{name:'筛选账变'}));
 await waitFor(()=>expect(fetcher.mock.calls.some(([p])=>p.includes('/economy/ledger?')&&p.includes('newapi_user_id=2')&&p.includes('biz_type=OPS_ADJUST'))).toBe(true));
 fireEvent.click(screen.getByRole('link',{name:'赌怪一次性活动'}));
 expect(await screen.findByRole('heading',{name:'资格已封存'})).toBeVisible();
 expect(screen.queryByLabelText('维护窗口编号')).not.toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'查看活动账户'}));
 const campaignDialog=await screen.findByRole('dialog',{name:'活动账户详情'});
 expect(within(campaignDialog).getByText('historical-checkpoint')).toBeVisible();
 fireEvent.click(within(campaignDialog).getByRole('button',{name:'关闭对话框'}));
 fireEvent.click(screen.getByRole('button',{name:'下一页'}));
 expect(await screen.findByText('25',{selector:'td'})).toBeVisible();
 fireEvent.change(screen.getByLabelText('活动用户 ID'),{target:{value:'2'}});
 fireEvent.change(screen.getByLabelText('勋章资格'),{target:{value:'true'}});
 fireEvent.change(screen.getByLabelText('重置状态'),{target:{value:'PENDING'}});
 fireEvent.change(screen.getByLabelText('快照时间从'),{target:{value:'2026-09-01T00:00'}});
 fireEvent.change(screen.getByLabelText('快照时间至（不含）'),{target:{value:'2026-10-01T00:00'}});
 fireEvent.click(screen.getByRole('button',{name:'筛选活动账户'}));
 await waitFor(()=>expect(fetcher.mock.calls.some(([p])=>p.startsWith(gamblerRoot+'?')&&p.includes('page=1')&&p.includes('user=2')&&p.includes('eligible=true')&&p.includes('state=PENDING')&&p.includes('from=')&&p.includes('to='))).toBe(true));
 fireEvent.click(await screen.findByRole('button',{name:'授予永久勋章'}));
 expect(window.confirm).toHaveBeenCalledWith('确认授予永久勋章？');
 expect(await screen.findByText('活动操作已保存')).toBeVisible();await screen.findByRole('alert');
 gamblerReadsFail=false;fireEvent.click(screen.getByRole('button',{name:'刷新活动'}));
 expect(await screen.findByRole('heading',{name:'勋章已授予'})).toBeVisible();
 expect(gamblerWrites).toEqual([{action:'grant',requests_drained:false}]);
 const campaignCall=fetcher.mock.calls.find(([p,i])=>p===gamblerRoot&&i?.method==='POST')!;
 expect(new Headers(campaignCall[1]?.headers).get('X-CSRF-Token')).toBe('local-csrf');
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
