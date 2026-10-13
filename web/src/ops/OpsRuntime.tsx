import { useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { ApiClient } from '../api';
import { Alert, Empty, Loading, Modal, Pager, useResource } from '../ui';
import { opsTime } from './OpsOperationDetail';
import './ops-system.css';

type Summary={kind:string;pending_count:string;attention_count:string;due_count:string;oldest_due_at:string|null};
type Job={kind:string;job_id:string;target_id:string;state:string;pending:boolean;attention:boolean;run_at:string|null;updated_at:string;legacy:boolean};
type JobsPage={summary:Summary[];items:Job[];total:number;page:number;page_size:number};
type Health={snapshot:{ready:boolean;checked_at:string;components:{name:string;state:string}[]};observed_at:string;stale_after:string;stale:boolean};
const jobNames:Record<string,string>={ANNOUNCEMENTS:'公告定时发布与到期',GAME_ROUNDS:'游戏回合恢复',REGISTRATION_GRANTS:'注册奖励',KEY_PURPOSE_SYNC:'密钥用途同步（旧）',QUOTA_TRANSFERS:'额度转账（旧）',MAINTENANCE:'维护窗口（旧）',OPS_REMOTE:'Poker 运营操作（旧）',RANKING_REBUILDS:'排行榜影子重建'};
const destinations:Record<string,string>={ANNOUNCEMENTS:'/ops/announcements',GAME_ROUNDS:'/ops/games',REGISTRATION_GRANTS:'/ops/rewards',KEY_PURPOSE_SYNC:'/ops/economy',QUOTA_TRANSFERS:'/ops/economy',MAINTENANCE:'/ops/maintenance',OPS_REMOTE:'/ops/poker',RANKING_REBUILDS:'/ops/rankings'};
const states:Record<string,string>={PENDING:'待执行',RUNNING:'执行中',DISPATCHING:'派发中',RETRY:'等待重试',RECOVERING:'恢复中',NEEDS_REVIEW:'待核对',REJECTED:'已拒绝',FAILED:'失败'};
const componentNames:Record<string,string>={web:'页面资源',native:'Native 服务',native_auth_web:'Native 登录服务',session_database:'会话数据库',session_cache:'会话缓存',platform_database:'平台数据库',history_reader:'历史记录查询',history_worker_database:'历史归档数据库',history_poker_reader:'Poker 历史来源',request_attribution:'请求归因同步',maintenance_worker:'维护执行器',ops_remote_worker:'远程运营执行器',poker_process:'Poker 进程',poker_database:'Poker 数据库',poker_auth_database:'Poker 身份只读库',poker_cache:'Poker 票据缓存'};

export function OpsJobs({client}:{client:ApiClient}){return <OpsJobList client={client} attentionOnly={false}/>}
export function OpsAttention({client}:{client:ApiClient}){return <OpsJobList client={client} attentionOnly/>}
function OpsJobList({client,attentionOnly}:{client:ApiClient;attentionOnly:boolean}){
 const [page,setPage]=useState(1),[query,setQuery]=useState(''),[selected,setSelected]=useState<Job|null>(null);
 const root=attentionOnly?'/api/v1/ops/attention':'/api/v1/ops/jobs';
 const resource=useResource(()=>client.request<JobsPage>(`${root}?page=${page}${query}`),[client,root,page,query]);
 function filter(event:FormEvent<HTMLFormElement>){
  event.preventDefault();const params=new URLSearchParams();
  for(const [key,value] of new FormData(event.currentTarget)){const v=String(value).trim();if(v)params.set(key,key==='from'||key==='to'?new Date(v).toISOString():v)}
  setPage(1);setQuery('&'+params.toString());
 }
 return <><header className="page-heading ops-system-heading"><div><p className="eyebrow">OPERATIONS / {attentionOnly?'ATTENTION':'JOBS'}</p><h1>{attentionOnly?'待处理事项':'后台任务'}</h1><p>{attentionOnly?'列出队列中标记为待核对、失败或拒绝的事项。':'查看当前未完成任务；已完成任务不在本列表中。'}</p></div><button onClick={resource.reload} disabled={resource.loading}>刷新任务</button></header>
 <section className="panel ops-system-list"><form className="filters" onSubmit={filter}>
 <label>任务类型<select name="kind"><option value="">全部类型</option>{Object.entries(jobNames).map(([v,l])=><option value={v} key={v}>{l}</option>)}</select></label>
 <label>任务状态<select name="status"><option value="">全部状态</option>{Object.entries(states).map(([v,l])=><option value={v} key={v}>{l}</option>)}</select></label>
 <label>记录时间从<input type="datetime-local" name="from"/></label><label>记录时间至（不含）<input type="datetime-local" name="to"/></label><button type="submit">筛选任务</button></form>
 <p className="hint">时间范围按任务更新时间筛选；公告队列采用创建时间。这里只读取元数据，不会执行或重试任务。</p>
 {resource.loading?<Loading/>:resource.error?<Alert>{resource.error}</Alert>:resource.data&&<>
 <div className="table-wrap" role="region" aria-label="任务明细" tabIndex={0}><table><thead><tr><th>任务类型</th><th>状态</th><th>计划时间</th><th>记录时间</th><th>详情</th></tr></thead><tbody>{resource.data.items.map(item=><tr key={item.kind+item.job_id}><th scope="row">{jobNames[item.kind]}</th><td>{states[item.state]||item.state}{item.attention&&' · 待核对'}</td><td>{item.run_at?opsTime(item.run_at):'—'}</td><td>{opsTime(item.updated_at)}</td><td><button onClick={()=>setSelected(item)}>查看任务</button></td></tr>)}</tbody></table></div>
 {!resource.data.items.length&&<Empty title="暂无匹配任务">可调整筛选或刷新队列。</Empty>}<Pager page={page} total={resource.data.total} size={20} onChange={setPage}/>
 <details><summary>全局队列概况（不受上方筛选影响）</summary><div className="table-wrap" role="region" aria-label="全局队列概况" tabIndex={0}><table><thead><tr><th>任务</th><th>待处理</th><th>已到执行时间</th><th>待核对</th><th>最早计划时间</th></tr></thead><tbody>{resource.data.summary.map(item=><tr key={item.kind}><th scope="row">{jobNames[item.kind]}</th><td>{item.pending_count}</td><td>{item.due_count}</td><td>{item.attention_count}</td><td>{item.oldest_due_at?opsTime(item.oldest_due_at):'—'}</td></tr>)}</tbody></table></div></details>
 <p className="hint">空队列不代表执行器正在运行；旧队列仅保留历史流程的未完成记录，运行情况以服务健康与部署状态为准。</p></>}
 </section>
 {selected&&<Modal title="任务详情" onClose={()=>setSelected(null)}><dl><dt>任务类型</dt><dd>{jobNames[selected.kind]}</dd><dt>任务编号</dt><dd>{selected.job_id}</dd><dt>业务对象</dt><dd>{selected.target_id}</dd><dt>状态</dt><dd>{states[selected.state]||selected.state} / {selected.state}</dd><dt>待处理 / 待核对</dt><dd>{selected.pending?'是':'否'} / {selected.attention?'是':'否'}</dd><dt>计划时间</dt><dd>{selected.run_at?opsTime(selected.run_at):'—'}</dd><dt>记录时间</dt><dd>{opsTime(selected.updated_at)}</dd></dl>{selected.legacy&&<p>旧流程记录；此页不执行旧命令。</p>}<Link to={destinations[selected.kind]}>进入业务页面 →</Link></Modal>}</>;
}

export function OpsServiceHealth({client}:{client:ApiClient}){
 const resource=useResource(()=>client.request<Health>('/api/v1/ops/service-health'),[client]);
 return <><header className="page-heading ops-system-heading"><div><p className="eyebrow">OPERATIONS / SERVICE HEALTH</p><h1>服务健康</h1><p>当前环境的一份最新检查快照，不是历史监控列表。</p></div><button onClick={resource.reload} disabled={resource.loading}>刷新快照</button></header>
 {resource.loading?<Loading/>:resource.error?<Alert>{resource.error}</Alert>:resource.data&&<section className="panel ops-system-health">
 <h2 className={'ops-system-health-summary '+(resource.data.stale?'is-unknown':resource.data.snapshot.ready?'is-ready':'is-unavailable')}>{resource.data.stale?'服务状态快照已过期':resource.data.snapshot.ready?'已配置依赖检查通过':'存在不可用或尚未配置的依赖'}</h2>
 <p className="ops-system-observed">检查时间：{opsTime(resource.data.observed_at)} · 有效至：{opsTime(resource.data.stale_after)}</p>
 <div className="table-wrap" role="region" aria-label="依赖检查结果" tabIndex={0}><table><thead><tr><th>依赖</th><th>检查结果</th></tr></thead><tbody>{resource.data.snapshot.components.map(item=><tr key={item.name}><th scope="row">{componentNames[item.name]||item.name}</th><td><span className={'ops-system-health-result '+(resource.data!.stale?'is-unknown':item.state==='READY'?'is-ready':'is-unavailable')}>{resource.data!.stale?'未知（快照过期）':item.state==='READY'?'可用':'不可用'}</span></td></tr>)}</tbody></table></div>
 <p className="hint">后台每 15 秒检查一次，45 秒未更新的快照显示为未知。快照保留实际部署的依赖名称，不代表迁移验收完成。</p>
 <nav className="ops-system-links" aria-label="相关运营页面"><Link to="/ops/jobs">查看后台任务 →</Link><Link to="/ops/audit">查看操作日志 →</Link></nav>
 </section>}</>;
}
