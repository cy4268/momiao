import { useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { ApiClient } from '../api';
import { Alert, Loading, useResource } from '../ui';
import { opsAmount } from './OpsEconomy';
import { opsTime, opsState } from './OpsFacts';

export type OpsWallet={user_id:string;asset:'RESERVE_API_CREDIT'|'AVAILABLE_CHIPS';balance_units:string};
type Ledger={id:string;transaction_id:string;user_id:string;asset:OpsWallet['asset'];entry_type:string;biz_type:string;biz_id:string;delta_units:string;balance_before_units:string;balance_after_units:string;created_at:string};
type Transfer={id:string;user_id:string;amount_units:string;status:string;reason:string;created_at:string;updated_at:string};
export type OpsPage<T>={items:T[];next_before?:string;next_before_id?:string};
export const assetNames={RESERVE_API_CREDIT:'Reserve',AVAILABLE_CHIPS:'可用筹码'};
export function opsSignedAmount(value:string){return value.startsWith('-')?`−${opsAmount(value.slice(1))}`:`+${opsAmount(value)}`;}
export function OpsLedger({client,user}:{client:ApiClient;user:string}){
 const [asset,setAsset]=useState(''),[bizInput,setBizInput]=useState(''),[biz,setBiz]=useState(''),[cursors,setCursors]=useState(['']);
 const query=new URLSearchParams({limit:'50',newapi_user_id:user,asset,biz_type:biz});
 const resource=useResource(()=>client.request<OpsPage<Ledger>>(`/api/v1/ops/economy/ledger?${query}${cursors.at(-1)}`),[client,user,asset,biz,cursors]);
 function filter(e:FormEvent){e.preventDefault();setBiz(bizInput.trim());setCursors([''])}
 return <section className="panel ops-business-ledger"><div className="section-heading"><h2>{user?'账户账变':'全局账变'}</h2><button onClick={()=>{setCursors(['']);resource.reload()}} disabled={resource.loading}>最新记录</button></div>
 <form className="filters" onSubmit={filter}><label>资产<select value={asset} onChange={e=>{setAsset(e.target.value);setCursors([''])}}><option value="">全部资产</option>{Object.entries(assetNames).map(([value,name])=><option key={value} value={value}>{name}</option>)}</select></label><label>业务类型<input value={bizInput} onChange={e=>setBizInput(e.target.value)} maxLength={128} placeholder="例如 OPS_ADJUST"/></label><button type="submit">筛选账变</button></form>
 {resource.loading?<Loading/>:resource.error?<Alert>{resource.error}</Alert>:resource.data&&<><div className="table-wrap"><table><thead><tr><th>账户 / 时间</th><th>资产</th><th>变化</th><th>变更前 / 后</th><th>来源 / 交易</th></tr></thead><tbody>{resource.data.items.map(v=><tr key={v.id}><td><Link to={'/ops/users/'+v.user_id}>{v.user_id}</Link><small>{opsTime(v.created_at)}</small></td><td>{assetNames[v.asset]}</td><td>{opsSignedAmount(v.delta_units)}</td><td>{opsAmount(v.balance_before_units)} → {opsAmount(v.balance_after_units)}</td><td>{v.biz_type} · {v.entry_type}<small>{v.biz_id}</small><small>{v.transaction_id}</small></td></tr>)}</tbody></table></div>{!resource.data.items.length&&<p>暂无符合条件的账变。</p>}<div className="pagination"><button disabled={cursors.length===1} onClick={()=>setCursors(cursors.slice(0,-1))}>上一页</button><span>第 {cursors.length} 页</span><button disabled={!resource.data.next_before} onClick={()=>setCursors([...cursors,`&before=${encodeURIComponent(resource.data!.next_before!)}&before_id=${resource.data!.next_before_id}`])}>下一页</button></div></>}
 </section>;
}
export function OpsTransfers({client,user}:{client:ApiClient;user:string}){
 const [status,setStatus]=useState(''),[cursor,setCursor]=useState('');
 const resource=useResource(()=>client.request<OpsPage<Transfer>>(`/api/v1/ops/economy/transfers?limit=50&newapi_user_id=${user}&status=${status}${cursor}`),[client,user,status,cursor]);
 return <section className="ops-business-transfers"><p>保留迁移前的额度划转凭据；不再向旧额度账户发起恢复或转账。</p><label>划转状态<select value={status} onChange={e=>{setStatus(e.target.value);setCursor('')}}><option value="">全部状态</option>{['PENDING','CONFIRMED','REFUNDED','NEEDS_REVIEW'].map(value=><option key={value} value={value}>{opsState[value]||value}</option>)}</select></label>{resource.loading?<Loading/>:resource.error?<Alert>{resource.error}</Alert>:resource.data&&<><div className="table-wrap"><table><thead><tr><th>账户 / 划转</th><th>金额</th><th>状态</th><th>更新时间</th></tr></thead><tbody>{resource.data.items.map(v=><tr key={v.id}><td>{v.user_id}<small>{v.id}</small></td><td>{opsAmount(v.amount_units)}</td><td>{opsState[v.status]||v.status}<small>{v.reason}</small></td><td>{opsTime(v.updated_at)}</td></tr>)}</tbody></table></div>{!resource.data.items.length&&<p>暂无符合条件的划转。</p>}{cursor&&<button onClick={()=>setCursor('')}>最新划转</button>}{resource.data.next_before&&<button onClick={()=>setCursor(`&before=${encodeURIComponent(resource.data!.next_before!)}&before_id=${resource.data!.next_before_id}`)}>更早划转</button>}</>}</section>;
}
