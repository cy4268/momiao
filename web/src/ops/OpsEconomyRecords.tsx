import { useState, type FormEvent } from 'react';
import { Alert, Loading, useResource } from '../ui';
import { opsError, opsID } from './ops-api';
import { OpsMutationPanel, useOpsMutation } from './OpsMutation';
import { opsAmount, opsAmountMatches, opsAsset, opsInt64, opsPositiveInt64, opsRecord, opsText, opsTimestamp, opsUserID } from './OpsEconomy';
import { opsTime, opsState } from './OpsOperationDetail';
import { useOps } from './OpsShell';

export type OpsWallet={user_id:string;asset:'RESERVE_API_CREDIT'|'AVAILABLE_CHIPS';balance_units:string;ledger_seq:string;version:string};
type Ledger={id:string;transaction_id:string;user_id:string;asset:'RESERVE_API_CREDIT'|'AVAILABLE_CHIPS';ledger_seq:string;wallet_version:string;entry_type:string;biz_type:string;biz_id:string;delta_units:string;balance_before_units:string;balance_after_units:string;created_at:string};
type Transfer={id:string;user_id:string;amount_units:string;amount:string;status:'PENDING'|'CONFIRMED'|'REFUNDED'|'NEEDS_REVIEW';reason:string;native_before:string|null;native_after:string|null;created_at:string;updated_at:string;version:string};
type Page<T>={items:T[];next_before?:string;next_before_id?:string};
const names:Record<string,string>={RESERVE_API_CREDIT:'Reserve API Credit',AVAILABLE_CHIPS:'可用筹码'};
const transferReasons:Record<Transfer['status'],string[]>={PENDING:['','ADMIN_RESUME'],CONFIRMED:[''],REFUNDED:['ACCOUNT_RESTRICTED','SOURCE_INCOMPATIBLE','BALANCE_OVERFLOW'],NEEDS_REVIEW:['REFUND_BALANCE_OVERFLOW','ADMIN_MARKED_FOR_REVIEW']};
export function opsSignedAmount(value:string){if(!opsInt64(value,true)||value==='0')return '—';return value.startsWith('-')?`−${opsAmount(value.slice(1))}`:`+${opsAmount(value)}`;}
export function checkOpsPage<T extends {id:string;created_at:string}>(value:unknown,parse:(item:unknown)=>T):Page<T>{
 if(!opsRecord(value)||!Array.isArray(value.items)||value.items.length>50)throw new Error('记录分页响应无法核对。');
 const items=value.items.map(parse);
 if(new Set(items.map(item=>item.id)).size!==items.length)throw new Error('记录分页响应无法核对。');
 const hasCursor=value.next_before!==undefined||value.next_before_id!==undefined;
 if(hasCursor&&(!opsTimestamp(value.next_before)||!opsID(value.next_before_id))||hasCursor!==(items.length===50))throw new Error('记录分页响应无法核对。');
 if(hasCursor){const last=items[items.length-1];if(!last||value.next_before!==last.created_at||value.next_before_id!==last.id)throw new Error('记录分页响应无法核对。');return {items,next_before:value.next_before as string,next_before_id:value.next_before_id as string};}
 return {items};
}
function readLedger(value:unknown,user:string):Ledger{
 if(!opsRecord(value))throw new Error('账变记录无法核对。');
 const item=value as unknown as Ledger;
 if(!opsID(item.id)||!opsID(item.transaction_id)||!opsUserID(item.user_id)||item.user_id!==user||!opsAsset(item.asset)||!opsPositiveInt64(item.ledger_seq)||!opsPositiveInt64(item.wallet_version)||!opsText(item.entry_type,128)||!opsText(item.biz_type,128)||!opsText(item.biz_id,128)||!opsInt64(item.delta_units,true)||item.delta_units==='0'||!opsInt64(item.balance_before_units)||!opsInt64(item.balance_after_units)||!opsTimestamp(item.created_at))throw new Error('账变记录无法核对。');
 if(BigInt(item.balance_before_units)+BigInt(item.delta_units)!==BigInt(item.balance_after_units))throw new Error('账变记录无法核对。');
 return item;
}
function readTransfer(value:unknown,status:string):Transfer{
 if(!opsRecord(value))throw new Error('额度转账记录无法核对。');
 const item=value as unknown as Transfer;
 if(!opsID(item.id)||!opsUserID(item.user_id)||!opsPositiveInt64(item.amount_units)||BigInt(item.amount_units)>9007199254740991n||!opsAmountMatches(item.amount,item.amount_units)||!Object.hasOwn(transferReasons,item.status)||!opsText(item.reason,128,true)||!transferReasons[item.status]?.includes(item.reason)||!(item.native_before===null||opsInt64(item.native_before))||!(item.native_after===null||opsInt64(item.native_after))||(item.native_before===null)!==(item.native_after===null)||!opsTimestamp(item.created_at)||!opsTimestamp(item.updated_at)||Date.parse(item.updated_at)<Date.parse(item.created_at)||!opsPositiveInt64(item.version)||status&&item.status!==status)throw new Error('额度转账记录无法核对。');
 if(item.status==='PENDING'&&(item.native_before!==null||item.native_after!==null)||item.status==='CONFIRMED'&&(item.native_before===null||item.native_after===null||BigInt(item.native_after)-BigInt(item.native_before)!==BigInt(item.amount_units)))throw new Error('额度转账记录无法核对。');
 return item;
}
export function OpsLedger({user}:{user:string}){
 const {client}=useOps();const [cursor,setCursor]=useState('');
 const resource=useResource(async()=>{try{return checkOpsPage(await client.request<unknown>(`/api/v1/ops/economy/ledger?newapi_user_id=${user}&limit=50${cursor}`),item=>readLedger(item,user))}catch(error){throw new Error(opsError(error))}},[client,user,cursor]);
 return <section className="panel"><div className="section-heading"><h2>账户账变</h2><button onClick={()=>{setCursor('');resource.reload()}} disabled={resource.loading}>最新记录</button></div>{resource.loading?<Loading/>:resource.error?<Alert>{resource.error}</Alert>:resource.data&&<><div className="table-wrap"><table><thead><tr><th>时间 / 交易</th><th>资产</th><th>变化</th><th>变更后</th><th>来源</th></tr></thead><tbody>{resource.data.items.map(item=><tr key={item.id}><td>{opsTime(item.created_at)}<small>{item.transaction_id}</small></td><td>{names[item.asset]||item.asset}</td><td>{opsSignedAmount(item.delta_units)}</td><td>{opsAmount(item.balance_after_units)}</td><td>{item.biz_type}<small>{item.biz_id}</small></td></tr>)}</tbody></table></div>{!resource.data.items.length&&<p>该账户暂无账变记录。</p>}{resource.data.next_before&&<button onClick={()=>setCursor(`&before=${encodeURIComponent(resource.data!.next_before!)}&before_id=${resource.data!.next_before_id}`)}>更早记录</button>}</>}</section>;
}
export function OpsAdjustment({user,wallets}:{user:string;wallets:OpsWallet[]}){
 const {bootstrap}=useOps(),mutation=useOpsMutation();const [asset,setAsset]=useState('RESERVE_API_CREDIT'),[delta,setDelta]=useState(''),[reference,setReference]=useState(''),[reason,setReason]=useState(''),[error,setError]=useState('');
 if(!bootstrap.operations.some(item=>item.operation_type==='ECONOMY_ADJUSTMENT'&&item.available&&bootstrap.principal.permissions.includes(item.required_permission)))return null;
 function submit(event:FormEvent){event.preventDefault();const wallet=wallets.find(item=>item.asset===asset&&item.user_id===user);if(!wallet||!opsPositiveInt64(wallet.version)||!opsInt64(delta,true)||delta==='0'){setError('请核对钱包版本和非零整数变动量。');return}setError('');mutation.prepare({operation_type:'ECONOMY_ADJUSTMENT',target:{type:'WALLET',id:user,expected_version:wallet.version},input:{asset,delta_units:delta,reference:reference.trim()},reason:reason.trim()})}
 return <section className="panel"><h2>人工账务调整</h2><p className="hint">每 500,000 单位等于 1 资产额度。负数扣减，正数增加；请提供可追踪的依据。</p><form onSubmit={submit}><fieldset disabled={!!mutation.pending}><label>资产<select value={asset} onChange={event=>setAsset(event.target.value)}>{Object.entries(names).map(([value,label])=><option key={value} value={value}>{label}</option>)}</select></label><label>变动单位<input value={delta} onChange={event=>setDelta(event.target.value)} inputMode="text" maxLength={20} required/></label><label>关联依据<input value={reference} onChange={event=>setReference(event.target.value)} maxLength={128} required/></label><label>操作原因<textarea value={reason} onChange={event=>setReason(event.target.value)} maxLength={1000} required/></label><button type="submit">预览账务影响</button></fieldset></form>{error&&<Alert>{error}</Alert>}<OpsMutationPanel mutation={mutation}/></section>;
}
export function OpsTransfers(){
 const {client,bootstrap}=useOps(),mutation=useOpsMutation();const [status,setStatus]=useState('NEEDS_REVIEW'),[cursor,setCursor]=useState(''),[reason,setReason]=useState('');
 const resource=useResource(async()=>{try{return checkOpsPage(await client.request<unknown>(`/api/v1/ops/economy/transfers?limit=50${status?'&status='+status:''}${cursor}`),item=>readTransfer(item,status))}catch(error){throw new Error(opsError(error))}},[client,status,cursor]);
 const canWrite=bootstrap.operations.some(item=>item.operation_type==='ECONOMY_TRANSFER_RECONCILE'&&item.available&&bootstrap.principal.permissions.includes(item.required_permission));
 function prepare(item:Transfer){mutation.prepare({operation_type:'ECONOMY_TRANSFER_RECONCILE',target:{type:'QUOTA_TRANSFER',id:item.id,expected_version:item.version},input:{action:item.status==='NEEDS_REVIEW'?'RESUME':'MARK_FOR_REVIEW'},reason:reason.trim()})}
 return <section className="panel"><div className="section-heading"><h2>额度转账处理</h2><button onClick={resource.reload} disabled={resource.loading}>刷新转账</button></div><label>状态<select value={status} onChange={event=>{setStatus(event.target.value);setCursor('')}}><option value="">全部状态</option><option value="NEEDS_REVIEW">需要核对</option><option value="PENDING">处理中</option><option value="CONFIRMED">已确认</option><option value="REFUNDED">已退回</option></select></label>{canWrite&&<label>处理原因<textarea value={reason} onChange={event=>setReason(event.target.value)} maxLength={1000} disabled={!!mutation.pending}/></label>}{resource.loading?<Loading/>:resource.error?<Alert>{resource.error}</Alert>:resource.data&&<><div className="table-wrap"><table><thead><tr><th>账户 / 转账</th><th>金额</th><th>状态</th><th>更新时间</th><th>操作</th></tr></thead><tbody>{resource.data.items.map(item=><tr key={item.id}><td>{item.user_id}<small>{item.id}</small></td><td>{opsAmount(item.amount_units)}</td><td>{opsState[item.status]||item.status}{item.reason&&<small>{item.reason}</small>}</td><td>{opsTime(item.updated_at)}</td><td>{canWrite&&['NEEDS_REVIEW','PENDING'].includes(item.status)&&<button disabled={!!mutation.pending||!reason.trim()} onClick={()=>prepare(item)}>{item.status==='NEEDS_REVIEW'?'预览恢复原转账':'预览标记待核对'}</button>}</td></tr>)}</tbody></table></div>{!resource.data.items.length&&<p>暂无符合条件的转账。</p>}{resource.data.next_before&&<button onClick={()=>setCursor(`&before=${encodeURIComponent(resource.data!.next_before!)}&before_id=${resource.data!.next_before_id}`)}>更早记录</button>}</>}<OpsMutationPanel mutation={mutation}/></section>;
}


