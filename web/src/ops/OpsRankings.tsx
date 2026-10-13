import { useState, type FormEvent } from 'react';
import { ApiClient, errorText } from '../api';
import { Alert, Loading, Modal, Pager, useResource } from '../ui';
import { OpsFacts, opsTime } from './OpsOperationDetail';
import './ops-business.css';

type Aggregation={slice_target_id:string;domain:string;metric:string;period:string;date:string|null;last_source_checked_at:string|null;lag_seconds:string|null;state:string};
type Snapshot={snapshot_id:string;slice_target_id:string;domain:string;period:string;build_kind:string;built_at:string;entry_count:string;published:boolean;pointer_version:string};
type Job={job_id:string;metric:string;period:string;date?:string;status:string;attempts:string;failure_code?:string;updated_at:string;shadow_snapshot_id?:string};
type Overview={generated_at:string;aggregation_status:Aggregation[];published:Snapshot[];shadow_rebuilds:Snapshot[];rebuild_jobs:Job[];page:number;page_size:number;job_total:number;shadow_total:number};
type Detail={snapshot:Snapshot;current:Snapshot|null;diff:{old_count:string;new_count:string;added:string;removed:string;changed:string;old_hash:string|null;new_hash:string}};
const names:Record<string,string>={GAMES:'游戏',RP:'RP 调用',ASSETS:'资产',DAY:'日榜',WEEK:'周榜',ALL_TIME:'总榜',CURRENT:'当前快照',TOTAL_ASSETS:'总资产',GAME_PROFIT:'游戏净收益',BIGGEST_WIN:'最高单次净赢',TOTAL_WAGERED:'累计投入',POKER_PROFIT:'Poker 净收益',RP_CALLS:'RP 请求数',RP_ERRORS:'RP 错误数',RP_CREDITS:'RP 消耗额度'};
const jobStates:Record<string,string>={PENDING:'等待处理',RUNNING:'正在重建',SHADOW:'影子快照已生成',PUBLISHED:'已发布',FAILED:'重建失败'};

export function OpsRankings({client}:{client:ApiClient}){
 const [slice,setSlice]=useState(''),[snapshot,setSnapshot]=useState('');
 const [filters,setFilters]=useState(''),[page,setPage]=useState(1);
 const [busy,setBusy]=useState(false),[error,setError]=useState(''),[saved,setSaved]=useState('');
 const resource=useResource(()=>client.request<Overview>(`/api/v1/ops/rankings?page=${page}${filters}`),[client,page,filters]);
 const detail=useResource(()=>snapshot?client.request<Detail>(`/api/v1/ops/rankings/snapshots/${snapshot}`):Promise.resolve(null),[client,snapshot]);
 const selected=resource.data?.aggregation_status.find(item=>item.slice_target_id===slice);
 function filter(event:FormEvent<HTMLFormElement>){
  event.preventDefault();const form=new FormData(event.currentTarget),query=new URLSearchParams();
  query.set('domain',String(form.get('domain')||''));
  for(const key of ['from','to']){const value=String(form.get(key)||'');if(value)query.set(key,new Date(value).toISOString())}
  setPage(1);setFilters('&'+query);setSlice('');
 }
 async function rebuild(event:FormEvent){
  event.preventDefault();if(!selected)return;
  setBusy(true);setError('');setSaved('');
  try{
   const result=await client.request<{job_id:string}>('/api/v1/ops/rankings/rebuilds','POST',{domain:selected.domain,metric:selected.metric,period:selected.period,...(selected.date?{date:selected.date}:{}),model_scope:'ALL'});
   setSaved('已创建重建任务：'+result.job_id+'。后台生成候选后可查看差异。');setSlice('');resource.reload();
  }catch(e){setError(errorText(e))}finally{setBusy(false)}
 }
 async function publish(){
  if(!window.confirm('发布这个候选，替换当前榜单？'))return;
  setBusy(true);setError('');setSaved('');
  try{await client.request(`/api/v1/ops/rankings/snapshots/${snapshot}/publish`,'POST',{});setSaved('候选已发布。');setSnapshot('');resource.reload()}
  catch(e){setError(errorText(e))}finally{setBusy(false)}
 }
 function snapshots(items:Snapshot[],label:string){return <div className="table-wrap" role="region" aria-label={label} tabIndex={0}><table><thead><tr><th>范围</th><th>生成时间</th><th>条目</th><th>状态</th><th>详情</th></tr></thead><tbody>{items.map(item=><tr key={item.snapshot_id}><td>{names[item.domain]} · {names[item.period]}<small>{item.slice_target_id}</small></td><td>{opsTime(item.built_at)}</td><td>{item.entry_count}</td><td>{item.published?'正在发布':item.build_kind}</td><td><button disabled={busy} onClick={()=>setSnapshot(item.snapshot_id)}>查看快照</button></td></tr>)}</tbody></table></div>}
 return <><header className="page-heading"><div><p className="eyebrow">OPERATIONS / RANKINGS</p><h1>排行榜管理</h1><p>生成影子快照、比较差异，再发布；重建不会直接替换当前榜单。</p></div><button onClick={resource.reload} disabled={busy||resource.loading}>刷新状态</button></header>
 <form className="filters panel" onSubmit={filter}><label>排行榜范围<select name="domain"><option value="">全部</option>{['GAMES','RP','ASSETS'].map(v=><option key={v} value={v}>{names[v]}</option>)}</select></label><label>任务创建起始时间<input name="from" type="datetime-local"/></label><label>任务创建截止时间<input name="to" type="datetime-local"/></label><button>筛选排行榜</button><small>时间范围筛选任务与候选；当前榜单状态始终保留。</small></form>
 {error&&<Alert>{error}</Alert>}{saved&&<p role="status">{saved}</p>}
 {resource.loading?<Loading/>:resource.error?<Alert>{resource.error}</Alert>:resource.data&&<>
 <section className="panel ops-business-aggregation"><h2>聚合状态</h2><p className="hint">观察时间：{opsTime(resource.data.generated_at)}</p><div className="table-wrap" role="region" aria-label="排行聚合状态" tabIndex={0}><table><thead><tr><th>榜单</th><th>状态</th><th>来源检查</th><th>延迟</th><th>操作</th></tr></thead><tbody>{resource.data.aggregation_status.map(item=><tr key={item.slice_target_id}><td>{names[item.domain]} · {names[item.metric]} · {names[item.period]}<small>{item.date||'当前范围'}</small></td><td>{item.state}</td><td>{item.last_source_checked_at?opsTime(item.last_source_checked_at):'尚未检查'}</td><td>{item.lag_seconds==null?'未知':item.lag_seconds+' 秒'}</td><td><button disabled={busy} onClick={()=>setSlice(item.slice_target_id)}>重建</button></td></tr>)}</tbody></table></div>
 {selected&&<form className="ops-business-rebuild" onSubmit={rebuild}><fieldset disabled={busy}><legend>建立影子重建任务</legend><p>{names[selected.domain]} · {names[selected.metric]} · {names[selected.period]} · {selected.date||'当前范围'}</p><button>创建重建任务</button></fieldset></form>}</section>
 <section className="panel ops-business-jobs"><h2>重建任务</h2><p className="hint">任务生成影子快照后，可以查看差异并发布。</p>{resource.data.rebuild_jobs.length?<div className="table-wrap" role="region" aria-label="排行榜重建任务" tabIndex={0}><table><thead><tr><th>榜单 / 任务</th><th>任务状态</th><th>最后更新</th><th>处理原因</th><th>详情</th></tr></thead><tbody>{resource.data.rebuild_jobs.map(job=><tr key={job.job_id}><td>{names[job.metric]} · {names[job.period]}<small>{job.date||'当前范围'}</small><small>{job.job_id}</small></td><td>{jobStates[job.status]}<small>尝试次数 {job.attempts}</small></td><td>{opsTime(job.updated_at)}</td><td>{job.failure_code||'—'}</td><td>{job.shadow_snapshot_id&&<button disabled={busy} onClick={()=>setSnapshot(job.shadow_snapshot_id!)}>查看生成的快照</button>}</td></tr>)}</tbody></table></div>:<p>暂无重建任务。</p>}</section>
 <div className="ops-business-snapshot-grid"><section className="panel ops-business-published"><h2>正在发布的快照</h2><p className="hint">同一批常规快照合并展示；各指标发布状态见上方聚合状态。</p>{resource.data.published.length?snapshots(resource.data.published,'已发布快照'):<p>暂无已发布快照。</p>}</section><section className="panel ops-business-shadow"><h2>影子重建候选</h2>{resource.data.shadow_rebuilds.length?snapshots(resource.data.shadow_rebuilds,'影子快照'):<p>暂无影子快照。</p>}</section></div>
 <Pager page={resource.data.page} size={resource.data.page_size} total={Math.max(resource.data.job_total,resource.data.shadow_total)} onChange={setPage} disabled={busy}/>
 </>}
 {snapshot&&<Modal title="快照与差异" onClose={()=>setSnapshot('')} busy={busy}><button onClick={detail.reload} disabled={busy||detail.loading}>刷新候选</button>{detail.loading?<Loading/>:detail.error?<Alert>{detail.error}</Alert>:detail.data&&<><OpsFacts value={detail.data.snapshot}/><h3>与当前快照比较</h3><OpsFacts value={detail.data.diff}/>{!detail.data.snapshot.published&&detail.data.snapshot.pointer_version==='0'&&detail.data.snapshot.build_kind==='REPAIR'&&<button disabled={busy} onClick={publish}>发布候选</button>}</>}{error&&<Alert>{error}</Alert>}</Modal>}
 </>;
}
