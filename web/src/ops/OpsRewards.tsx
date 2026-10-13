import { useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { ApiClient, errorText } from '../api';
import { Alert, Loading, useResource } from '../ui';
import { opsAmount } from './OpsEconomy';
import { type OpsPage } from './OpsEconomyRecords';
import { opsTime, opsState } from './OpsOperationDetail';
import './ops-business.css';

type Program='REGISTRATION'|'DAILY'|'HOURLY'|'RELIEF';
type Policy={program:Program;policy_version:string;amount:string;amount_units:string;asset:'RESERVE_API_CREDIT';editable:false;schedule:string;cooldown_seconds?:string;threshold?:string;threshold_units?:string};
type Claim={id:string;program:Program;newapi_user_id:string;policy_version:string;status:'PENDING'|'RECOVERING'|'SUCCEEDED';amount:string;amount_units:string;asset:'RESERVE_API_CREDIT';transaction_id:string|null;business_id:string;created_at:string;claimed_at?:string;attempts:string;last_error_code?:'GRANT_RETRY_REQUIRED';next_action_at?:string;retryable:boolean;total_assets_units?:string;assets_observed_at?:string;};
const programs:Record<string,string>={REGISTRATION:'注册奖励',DAILY:'每日签到',HOURLY:'整点奖励',RELIEF:'救济金'};
const schedules:Record<string,string>={ONE_TIME_ON_ACCEPTED_REGISTRATION:'符合注册资格时领取一次',ASIA_SHANGHAI_NATURAL_DAY:'上海时区，每自然日一次',ASIA_SHANGHAI_NATURAL_HOUR_MAX_24_PER_DAY:'上海时区，每自然小时一次，每日最多 24 次',ROLLING_AFTER_SUCCESS:'从上次成功领取起计算冷却'};

export function OpsRewards({client}:{client:ApiClient}){
 const [input,setInput]=useState(''),[user,setUser]=useState(''),[program,setProgram]=useState(''),[status,setStatus]=useState(''),[cursors,setCursors]=useState(['']);
 const [busy,setBusy]=useState(''),[error,setError]=useState(''),[saved,setSaved]=useState('');
 const policies=useResource(()=>client.request<{items:Policy[]}>('/api/v1/ops/rewards/policies'),[client]);
 const query=new URLSearchParams({limit:'50',newapi_user_id:user,program,status});
 const claims=useResource(()=>client.request<OpsPage<Claim>>(`/api/v1/ops/rewards/claims?${query}${cursors.at(-1)}`),[client,user,program,status,cursors]);
 function filter(event:FormEvent){event.preventDefault();setUser(input.trim());setCursors([''])}
 async function retry(item:Claim){
  if(!window.confirm('立即重新排队这笔注册奖励？原奖励不会重复发放。'))return;
  setBusy(item.id);setError('');setSaved('');
  try{await client.request(`/api/v1/ops/rewards/claims/${item.id}/retry`,'POST',{});setSaved('已重新排队；发放结果由原奖励任务更新。');claims.reload()}
  catch(e){setError(errorText(e))}finally{setBusy('')}
 }
 return <><header className="page-heading"><div><p className="eyebrow">OPERATIONS / REWARDS</p><h1>奖励运营</h1><p>核对政策和领取记录；重试只重新排队原注册奖励，不直接发放余额。</p></div><button onClick={()=>{policies.reload();claims.reload()}} disabled={policies.loading||claims.loading}>刷新奖励</button></header>
 <section className="ops-business-policy-region"><h2>奖励政策（只读）</h2>{policies.loading?<Loading/>:policies.error?<Alert>{policies.error}</Alert>:policies.data&&<div className="ops-entry-grid ops-business-policy-grid">{policies.data.items.map(v=><section className="panel ops-business-policy" key={v.program}><h2>{programs[v.program]}</h2><strong>{opsAmount(v.amount_units)} Reserve</strong><p>{schedules[v.schedule]}</p>{v.cooldown_seconds&&<p>成功后冷却 {Number(v.cooldown_seconds)/3600} 小时</p>}{v.threshold_units&&<p>统一资产须低于 {opsAmount(v.threshold_units)}</p>}<p className="hint">政策版本 {v.policy_version}；实际到账以领取记录为准。</p></section>)}</div>}</section>
 <section className="panel ops-business-claims"><h2>领取记录</h2><form className="filters" onSubmit={filter}><label>奖励账户（可选）<input value={input} onChange={e=>setInput(e.target.value)} inputMode="numeric" pattern="[1-9][0-9]*" maxLength={10} placeholder="全部账户"/></label><label>奖励类型<select value={program} onChange={e=>{setProgram(e.target.value);setCursors([''])}}><option value="">全部类型</option>{Object.entries(programs).map(([value,name])=><option key={value} value={value}>{name}</option>)}</select></label><label>奖励状态<select value={status} onChange={e=>{setStatus(e.target.value);setCursors([''])}}><option value="">全部状态</option>{['PENDING','RECOVERING','SUCCEEDED'].map(value=><option key={value} value={value}>{opsState[value]||value}</option>)}</select></label><button type="submit">查询奖励</button></form>
 {error&&<Alert>{error}</Alert>}{saved&&<p role="status">{saved}</p>}{claims.loading?<Loading/>:claims.error?<Alert>{claims.error}</Alert>:claims.data&&<><div className="table-wrap"><table><thead><tr><th>账户 / 奖励</th><th>金额</th><th>状态</th><th>时间</th><th>处理</th></tr></thead><tbody>{claims.data.items.map(v=><tr key={v.id}><td><Link to={'/ops/users/'+v.newapi_user_id}>{v.newapi_user_id}</Link> · {programs[v.program]}<small>{v.id}</small></td><td>{opsAmount(v.amount_units)}</td><td>{opsState[v.status]||v.status}<small>{v.last_error_code}</small></td><td>{opsTime(v.claimed_at||v.created_at)}{v.next_action_at&&<small>下次处理：{opsTime(v.next_action_at)}</small>}</td><td>{v.transaction_id&&<small>交易：{v.transaction_id}</small>}{v.retryable&&v.program==='REGISTRATION'&&<button disabled={!!busy} onClick={()=>retry(v)}>{busy===v.id?'排队中…':'重试注册奖励'}</button>}<small>已尝试 {v.attempts} 次</small></td></tr>)}</tbody></table></div>{!claims.data.items.length&&<p>暂无符合条件的领取记录。</p>}<div className="pagination"><button disabled={cursors.length===1} onClick={()=>setCursors(cursors.slice(0,-1))}>上一页</button><span>第 {cursors.length} 页</span><button disabled={!claims.data.next_before} onClick={()=>setCursors([...cursors,`&before=${encodeURIComponent(claims.data!.next_before!)}&before_id=${claims.data!.next_before_id}`])}>下一页</button></div></>}
 </section></>;
}
