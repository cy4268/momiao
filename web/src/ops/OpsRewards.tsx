import { useState, type FormEvent } from 'react';
import { Alert, Loading, useResource } from '../ui';
import { opsError, opsID } from './ops-api';
import { OpsMutationPanel, useOpsMutation } from './OpsMutation';
import { opsAmount, opsAmountMatches, opsInt64, opsPositiveInt64, opsRecord, opsText, opsTimestamp, opsUserID } from './OpsEconomy';
import { checkOpsPage } from './OpsEconomyRecords';
import { opsTime, opsState } from './OpsOperationDetail';
import { useOps } from './OpsShell';

type Program='REGISTRATION'|'DAILY'|'HOURLY'|'RELIEF';
type Policy={program:Program;policy_version:string;amount:string;amount_units:string;asset:'RESERVE_API_CREDIT';editable:false;schedule:string;cooldown_seconds?:string;threshold?:string;threshold_units?:string};
type Claim={id:string;program:Program;newapi_user_id:string;policy_version:string;status:'PENDING'|'RECOVERING'|'SUCCEEDED';amount:string;amount_units:string;asset:'RESERVE_API_CREDIT';transaction_id:string|null;business_id:string;created_at:string;claimed_at?:string;attempts:string;last_error_code?:'GRANT_RETRY_REQUIRED';next_action_at?:string;retryable:boolean;total_assets_units?:string;assets_observed_at?:string;version:string};
const programs:Record<string,string>={REGISTRATION:'注册奖励',DAILY:'每日签到',HOURLY:'整点奖励',RELIEF:'救济金'};
const schedules:Record<string,string>={ONE_TIME_ON_ACCEPTED_REGISTRATION:'符合注册资格时领取一次',ASIA_SHANGHAI_NATURAL_DAY:'上海时区，每自然日一次',ASIA_SHANGHAI_NATURAL_HOUR_MAX_24_PER_DAY:'上海时区，每自然小时一次，每日最多 24 次',ROLLING_AFTER_SUCCESS:'从上次成功领取起计算冷却'};
const policySchedules:Record<Program,string>={REGISTRATION:'ONE_TIME_ON_ACCEPTED_REGISTRATION',DAILY:'ASIA_SHANGHAI_NATURAL_DAY',HOURLY:'ASIA_SHANGHAI_NATURAL_HOUR_MAX_24_PER_DAY',RELIEF:'ROLLING_AFTER_SUCCESS'};
function readPolicy(value:unknown):Policy{
 if(!opsRecord(value))throw new Error('奖励政策响应无法核对。');
 const item=value as unknown as Policy;
 if(!Object.hasOwn(programs,item.program)||!opsText(item.policy_version,64)||!opsPositiveInt64(item.amount_units)||!opsAmountMatches(item.amount,item.amount_units)||item.asset!=='RESERVE_API_CREDIT'||item.editable!==false||item.schedule!==policySchedules[item.program])throw new Error('奖励政策响应无法核对。');
 if(item.program==='RELIEF'){
  if(!opsPositiveInt64(item.cooldown_seconds)||!opsPositiveInt64(item.threshold_units)||!opsAmountMatches(item.threshold,item.threshold_units))throw new Error('奖励政策响应无法核对。');
 }else if(item.cooldown_seconds!==undefined||item.threshold!==undefined||item.threshold_units!==undefined)throw new Error('奖励政策响应无法核对。');
 return item;
}
function readPolicies(value:unknown):Policy[]{
 if(!opsRecord(value)||!Array.isArray(value.items)||value.items.length!==4)throw new Error('奖励政策响应无法核对。');
 const items=value.items.map(readPolicy);
 if(new Set(items.map(item=>item.program)).size!==4||Object.keys(programs).some(program=>!items.some(item=>item.program===program)))throw new Error('奖励政策响应无法核对。');
 return items;
}
function readClaim(value:unknown,user:string,program:string,status:string):Claim{
 if(!opsRecord(value))throw new Error('奖励领取记录无法核对。');
 const item=value as unknown as Claim;
 if(!opsID(item.id)||!Object.hasOwn(programs,item.program)||!opsUserID(item.newapi_user_id)||!opsText(item.policy_version,64)||!['PENDING','RECOVERING','SUCCEEDED'].includes(item.status)||!opsPositiveInt64(item.amount_units)||!opsAmountMatches(item.amount,item.amount_units)||item.asset!=='RESERVE_API_CREDIT'||!(item.transaction_id===null||opsID(item.transaction_id))||!opsText(item.business_id,128)||!opsTimestamp(item.created_at)||!(item.claimed_at===undefined||opsTimestamp(item.claimed_at))||!opsInt64(item.attempts)||!(item.last_error_code===undefined||item.last_error_code==='GRANT_RETRY_REQUIRED')||!(item.next_action_at===undefined||opsTimestamp(item.next_action_at))||typeof item.retryable!=='boolean'||!(item.total_assets_units===undefined||opsInt64(item.total_assets_units))||!(item.assets_observed_at===undefined||opsTimestamp(item.assets_observed_at))||!opsPositiveInt64(item.version)||(user&&item.newapi_user_id!==user)||(program&&item.program!==program)||(status&&item.status!==status))throw new Error('奖励领取记录无法核对。');
 const succeeded=item.status==='SUCCEEDED';
 if(succeeded!==(item.transaction_id!==null)||succeeded!==(item.claimed_at!==undefined)||item.claimed_at!==undefined&&Date.parse(item.claimed_at)<Date.parse(item.created_at))throw new Error('奖励领取记录无法核对。');
 if(item.program==='REGISTRATION'){
  if(item.business_id!==`initial_grant:registration:${item.newapi_user_id}`||item.next_action_at===undefined||item.total_assets_units!==undefined||item.assets_observed_at!==undefined||item.retryable!==(item.status!=='SUCCEEDED')||item.status==='PENDING'&&(item.attempts!=='0'||item.last_error_code!==undefined)||item.status==='RECOVERING'&&(!opsPositiveInt64(item.attempts)||item.last_error_code!=='GRANT_RETRY_REQUIRED')||item.status==='SUCCEEDED'&&(!opsPositiveInt64(item.attempts)||item.last_error_code!==undefined))throw new Error('奖励领取记录无法核对。');
 }else{
  if(item.status!=='SUCCEEDED'||item.retryable||item.attempts!=='0'||item.last_error_code!==undefined)throw new Error('奖励领取记录无法核对。');
  if(item.program==='RELIEF'){
   if(item.total_assets_units===undefined||item.assets_observed_at===undefined||item.next_action_at===undefined||Date.parse(item.assets_observed_at)>Date.parse(item.claimed_at!)||Date.parse(item.next_action_at)<Date.parse(item.claimed_at!))throw new Error('奖励领取记录无法核对。');
  }else if(item.id!==item.transaction_id||item.total_assets_units!==undefined||item.assets_observed_at!==undefined||item.next_action_at!==undefined)throw new Error('奖励领取记录无法核对。');
 }
 return item;
}
function cooldown(value:string){if(!opsPositiveInt64(value))return '—';const seconds=BigInt(value);return seconds%3600n===0n?`${seconds/3600n} 小时`:`${seconds} 秒`;}
export function OpsRewards(){
 const {client,bootstrap}=useOps(),mutation=useOpsMutation();const [input,setInput]=useState(''),[user,setUser]=useState(''),[program,setProgram]=useState(''),[status,setStatus]=useState(''),[cursor,setCursor]=useState(''),[reason,setReason]=useState(''),[error,setError]=useState('');
 const policies=useResource(async()=>{try{return readPolicies(await client.request<unknown>('/api/v1/ops/rewards/policies'))}catch(error){throw new Error(opsError(error))}},[client]);
 const claims=useResource(async()=>{try{return checkOpsPage(await client.request<unknown>(`/api/v1/ops/rewards/claims?limit=50${user?'&newapi_user_id='+user:''}${program?'&program='+program:''}${status?'&status='+status:''}${cursor}`),item=>readClaim(item,user,program,status))}catch(error){throw new Error(opsError(error))}},[client,user,program,status,cursor]);
 const canRetry=bootstrap.operations.some(item=>item.operation_type==='REWARD_CLAIM_RETRY'&&item.available&&bootstrap.principal.permissions.includes(item.required_permission));
 function filter(event:FormEvent){event.preventDefault();if(input.trim()&&!opsUserID(input.trim())){setError('请输入有效的账户编号，或留空查询全部账户。');return}setError('');setUser(input.trim());setCursor('');claims.reload()}
 function retry(item:Claim){mutation.prepare({operation_type:'REWARD_CLAIM_RETRY',target:{type:'REWARD_CLAIM',id:item.id,expected_version:item.version},input:{action:'RETRY'},reason:reason.trim()})}
 return <><header className="page-heading"><div><p className="eyebrow">OPERATIONS / REWARDS</p><h1>奖励运营</h1><p>核对生效政策、领取记录和需要恢复的注册奖励。</p></div><button onClick={()=>{policies.reload();claims.reload()}} disabled={policies.loading||claims.loading}>刷新奖励</button></header>{policies.loading?<Loading/>:policies.error?<Alert>{policies.error}</Alert>:policies.data&&<div className="ops-entry-grid">{policies.data.map(item=><section className="panel" key={item.program}><h2>{programs[item.program]}</h2><strong>{opsAmount(item.amount_units)} Reserve API Credit</strong><p>{schedules[item.schedule]}</p>{item.cooldown_seconds&&<p>成功后冷却 {cooldown(item.cooldown_seconds)}</p>}{item.threshold_units&&<p>统一资产须低于 {opsAmount(item.threshold_units)}</p>}<p className="hint">政策版本 {item.policy_version} · 当前不可编辑</p></section>)}</div>}<section className="panel"><h2>领取记录</h2><form className="filters" onSubmit={filter}><label>账户编号<input value={input} onChange={event=>setInput(event.target.value)} maxLength={19} inputMode="numeric" placeholder="全部账户"/></label><label>奖励类型<select value={program} onChange={event=>{setProgram(event.target.value);setCursor('')}}><option value="">全部类型</option>{Object.entries(programs).map(([value,label])=><option value={value} key={value}>{label}</option>)}</select></label><label>状态<select value={status} onChange={event=>{setStatus(event.target.value);setCursor('')}}><option value="">全部状态</option><option value="PENDING">处理中</option><option value="RECOVERING">恢复中</option><option value="SUCCEEDED">已完成</option></select></label><button type="submit">查询记录</button></form>{error&&<Alert>{error}</Alert>}{canRetry&&<label>重试原因<textarea value={reason} onChange={event=>setReason(event.target.value)} maxLength={1000} disabled={!!mutation.pending}/></label>}{claims.loading?<Loading/>:claims.error?<Alert>{claims.error}</Alert>:claims.data&&<><div className="table-wrap"><table><thead><tr><th>账户 / 奖励</th><th>金额</th><th>状态</th><th>时间</th><th>处理</th></tr></thead><tbody>{claims.data.items.map(item=><tr key={item.id}><td>{item.newapi_user_id} · {programs[item.program]}<small>{item.id}</small></td><td>{opsAmount(item.amount_units)}</td><td>{opsState[item.status]||item.status}{item.last_error_code&&<small>{item.last_error_code}</small>}</td><td>{opsTime(item.claimed_at||item.created_at)}{item.next_action_at&&<small>下次处理：{opsTime(item.next_action_at)}</small>}</td><td>{item.transaction_id&&<small>交易：{item.transaction_id}</small>}{canRetry&&item.retryable&&item.program==='REGISTRATION'&&<button onClick={()=>retry(item)} disabled={!!mutation.pending||!reason.trim()}>预览恢复原奖励</button>}<small>已尝试 {item.attempts} 次</small></td></tr>)}</tbody></table></div>{!claims.data.items.length&&<p>暂无符合条件的领取记录。</p>}{claims.data.next_before&&<button onClick={()=>setCursor(`&before=${encodeURIComponent(claims.data!.next_before!)}&before_id=${claims.data!.next_before_id}`)}>更早记录</button>}</>}<OpsMutationPanel mutation={mutation}/></section></>;
}

