import { useState, type FormEvent } from 'react';
import { ApiClient } from '../api';
import { Alert, Loading, Modal, Pager, useResource } from '../ui';
import { OpsFacts, opsTime } from './OpsOperationDetail';
import { opsAmount } from './OpsEconomy';
import { opsError } from './ops-api';

const root='/api/v1/ops/economy/gambler-campaigns/01995000-2026-7000-8000-000000000927';
const phases:Record<string,string>={DRAFT:'尚未开始',SNAPSHOT_PREPARING:'资格快照进行中',SNAPSHOT_READY:'资格已封存',GRANTED:'勋章已授予',RESET_PREPARING:'重置预检进行中',RESET_READY:'等待单独确认重置',RESET_RUNNING:'重置进行中',COMPLETED:'重置完成'};
const actions:Record<string,[string,string][]>= {DRAFT:[['snapshot','开始资格快照']],SNAPSHOT_PREPARING:[['resume','继续原活动']],SNAPSHOT_READY:[['grant','授予永久勋章'],['resume','继续原活动']],GRANTED:[['prepare-reset','开始重置预检']],RESET_PREPARING:[['resume','继续原活动']],RESET_READY:[['reset','执行独立余额重置'],['resume','继续原活动']],RESET_RUNNING:[['resume','继续原活动']],COMPLETED:[['activate','启用经济政策'],['resume','继续原活动']]};
const resetStates:Record<string,string>={PENDING:'待检查',CHECKED:'已检查',COMPLETED:'已完成'};
type Account={user_id:string;eligible:boolean;qualification_units:string;reset_state:string;reset_before:unknown;reset_after:unknown;reset_receipt:unknown};
type View={id:string;phase:string;cutoff_at:string|null;reset_cutoff_at:string|null;target_count:string;completed_count:string;eligible_count:string;blocking_facts:string[];policy_activated_at:string|null;rankings_published_at:string|null;accounts:Account[];total:number};

export function OpsGamblerCampaign({client}:{client:ApiClient}){
 const [page,setPage]=useState(1),[query,setQuery]=useState(''),[selected,setSelected]=useState<Account|null>(null),[busy,setBusy]=useState(false),[error,setError]=useState(''),[message,setMessage]=useState('');
 const resource=useResource(()=>client.request<View>(`${root}?page=${page}${query}`),[client,page,query]);
 const view=resource.data;
 function filter(event:FormEvent<HTMLFormElement>){
  event.preventDefault();const params=new URLSearchParams();
  for(const [key,value] of new FormData(event.currentTarget)){const v=String(value).trim();if(v)params.set(key,key==='from'||key==='to'?new Date(v).toISOString():v)}
  setPage(1);setQuery('&'+params.toString());setSelected(null);
 }
 async function save(action:string,label:string){
  const drained=['snapshot','prepare-reset','reset','resume'].includes(action);
  const target=action==='reset'?'所有目标账户的 Reserve 将设为 10,000，筹码设为 0。':'';
  if(!window.confirm(`${target}${drained?'请确认维护已开启、之前受理的模型请求已排空。':''}确认${label}？`))return;
  setBusy(true);setError('');setMessage('');
  try{await client.request(root,'POST',{action,requests_drained:drained});setMessage('活动操作已保存');resource.reload()}
  catch(e){setError(opsError(e))}finally{setBusy(false)}
 }
 return <><header className="page-heading ops-system-heading"><div><p className="eyebrow">OPERATIONS / GAMBLER</p><h1>赌怪一次性活动</h1><p>资格快照与永久勋章保留；余额重置是独立操作，不重新执行已完成活动。</p></div><button disabled={busy} onClick={()=>resource.reload()}>刷新活动</button></header>
 {message&&<p role="status">{message}</p>}{error&&<Alert>{error}</Alert>}
 {resource.loading?<Loading/>:resource.error?<Alert>{resource.error}</Alert>:view&&<section className="panel"><h2>{phases[view.phase]}</h2><p>目标 {view.target_count} · 已重置 {view.completed_count} · 勋章资格 {view.eligible_count}</p><p>资格截止：{view.cutoff_at?opsTime(view.cutoff_at):'—'} · 重置截止：{view.reset_cutoff_at?opsTime(view.reset_cutoff_at):'—'}</p><p>政策启用：{view.policy_activated_at?opsTime(view.policy_activated_at):'—'} · 榜单发布：{view.rankings_published_at?opsTime(view.rankings_published_at):'—'}</p>
 <p>快照和重置使用六项本地资产；历史资格总额保持原值。维护会阻止新请求，已受理的模型请求需先排空；游戏与资金在途由服务端检查。</p>
 {view.blocking_facts.length>0&&<Alert>{view.blocking_facts.join(' · ')}</Alert>}
 <div className="actions">{(view.phase==='COMPLETED'&&view.policy_activated_at?[]:actions[view.phase]).map(([action,label])=><button key={action} disabled={busy} onClick={()=>void save(action,label)}>{label}</button>)}</div></section>}
 <section className="panel ops-system-list"><h2>全局活动账户</h2><form className="filters" onSubmit={filter}><label>活动用户 ID<input name="user" inputMode="numeric" pattern="[1-9][0-9]*"/></label><label>勋章资格<select name="eligible"><option value="">全部</option><option value="true">有资格</option><option value="false">无资格</option></select></label><label>重置状态<select name="state"><option value="">全部</option>{Object.entries(resetStates).map(([v,l])=><option key={v} value={v}>{l}</option>)}</select></label><label>快照时间从<input name="from" type="datetime-local"/></label><label>快照时间至（不含）<input name="to" type="datetime-local"/></label><button type="submit" disabled={busy}>筛选活动账户</button></form>
 {!resource.loading&&!resource.error&&view&&<><div className="table-wrap" role="region" aria-label="活动账户列表" tabIndex={0}><table><thead><tr><th>账户</th><th>资格快照总资产</th><th>勋章资格</th><th>重置状态</th><th>详情</th></tr></thead><tbody>{view.accounts.map(a=><tr key={a.user_id}><td>{a.user_id}</td><td>{a.qualification_units?opsAmount(a.qualification_units):'—'}</td><td>{a.eligible?'有资格':'无资格'}</td><td>{resetStates[a.reset_state]}</td><td><button onClick={()=>setSelected(a)}>查看活动账户</button></td></tr>)}</tbody></table></div>{!view.accounts.length&&<p>暂无匹配账户。</p>}<Pager page={page} size={20} total={view.total} onChange={setPage}/></>}
 </section>
 {selected&&<Modal title="活动账户详情" onClose={()=>setSelected(null)}><p>账户 {selected.user_id} · 资格快照总资产：{selected.qualification_units?opsAmount(selected.qualification_units):'—'}</p><h3>原始重置记录</h3><OpsFacts value={{before:selected.reset_before,after:selected.reset_after,receipt:selected.reset_receipt}}/></Modal>}
 </>;
}
