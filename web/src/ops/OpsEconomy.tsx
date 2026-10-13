import { useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { ApiClient, errorText } from '../api';
import { Alert, Loading, useResource } from '../ui';
import { OpsFacts, opsTime } from './OpsOperationDetail';
import { OpsLedger, OpsTransfers } from './OpsEconomyRecords';
import './ops-business.css';

const maxInt64=9223372036854775807n;
const aggregateUnsigned=(value:unknown):value is string=>typeof value==='string'&&/^(0|[1-9][0-9]{0,39})$/.test(value);
export const opsRecord=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
export const opsInt64=(value:unknown,signed=false):value is string=>typeof value==='string'&&(signed?/^(0|-?[1-9][0-9]{0,18})$/:/^(0|[1-9][0-9]{0,18})$/).test(value)&&BigInt(value)<=maxInt64&&BigInt(value)>=(signed?-9223372036854775808n:0n);
export const opsPositiveInt64=(value:unknown):value is string=>opsInt64(value)&&BigInt(value)>0n;
export const opsUserID=(value:unknown):value is string=>opsPositiveInt64(value)&&BigInt(value)<=2147483647n;
export const opsTimestamp=(value:unknown):value is string=>typeof value==='string'&&/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(value)&&Number.isFinite(Date.parse(value));
export const opsText=(value:unknown,maximum:number,allowEmpty=false):value is string=>typeof value==='string'&&value.length<=maximum&&!value.includes('\0')&&value.trim()===value&&(allowEmpty||value.length>0);
export const opsAsset=(value:unknown):value is 'RESERVE_API_CREDIT'|'AVAILABLE_CHIPS'=>value==='RESERVE_API_CREDIT'||value==='AVAILABLE_CHIPS';
function plainAmount(units:string){const value=BigInt(units),whole=value/500000n,fraction=((value%500000n)*2n).toString().padStart(6,'0').replace(/0+$/,'');return `${whole}${fraction?'.'+fraction:''}`;}
export function opsAmount(units:string){if(!aggregateUnsigned(units))return '—';const value=BigInt(units),whole=value/500000n,fraction=((value%500000n)*2n).toString().padStart(6,'0').replace(/0+$/,'');return `${whole.toLocaleString('zh-CN')}${fraction?'.'+fraction:''}`;}
export const opsAmountMatches=(value:unknown,units:unknown):value is string=>typeof value==='string'&&opsInt64(units)&&value===plainAmount(units);
type Overview={observed_at:string;wallet_accounts:string;reserve_units:string;available_chips_units:string;transactions:string};
type Assets={reserve_units:string;available_chips_units:string;poker_stack_units:string;poker_pot_units:string;single_player_in_flight_units:string;roulette_escrow_units:string;total_units:string;observed_at:string};
type UserResponse={observed_at:string;assets:Assets|null;migration:unknown};
const assetFields=[['reserve_units','Reserve'],['available_chips_units','可用筹码'],['poker_stack_units','扑克桌面'],['poker_pot_units','扑克底池'],['single_player_in_flight_units','单人游戏在途'],['roulette_escrow_units','轮盘托管']] as const;
export function OpsEconomy({client}:{client:ApiClient}){
 const [input,setInput]=useState(''),[user,setUser]=useState('');
 const [busy,setBusy]=useState(false),[error,setError]=useState(''),[notice,setNotice]=useState(''),[version,setVersion]=useState(0);
 const overview=useResource(()=>client.request<Overview>('/api/v1/ops/economy/overview'),[client]);
 const account=useResource(()=>user?client.request<UserResponse>(`/api/v1/ops/economy/user?newapi_user_id=${user}`):Promise.resolve(null),[client,user]);
 const assets=account.data?.assets;
 function filter(event:FormEvent){event.preventDefault();setNotice('');setError('');setUser(input.trim());account.reload()}
 async function adjust(body:unknown){setBusy(true);setError('');setNotice('');try{await client.request(`/api/v1/ops/users/${user}/chips`,'POST',body);setNotice('筹码调账已保存');account.reload();overview.reload();setVersion(v=>v+1)}catch(e){setError(errorText(e))}finally{setBusy(false)}}
 return <><header className="page-heading"><div><p className="eyebrow">OPERATIONS / ECONOMY</p><h1>经济与账务</h1><p>全局钱包账变与奖励资金记录；留空账户编号查看全部账户。</p></div><button onClick={overview.reload} disabled={overview.loading}>刷新总览</button></header>
 {overview.loading?<Loading/>:overview.error?<Alert>{overview.error}</Alert>:overview.data&&<section className="panel ops-business-economy-overview"><h2>账务总览</h2><p className="hint">观察时间：{opsTime(overview.data.observed_at)}</p><dl className="ops-impact"><div><dt>钱包账户数</dt><dd>{overview.data.wallet_accounts}</dd></div><div><dt>Reserve</dt><dd>{opsAmount(overview.data.reserve_units)}</dd></div><div><dt>可用筹码</dt><dd>{opsAmount(overview.data.available_chips_units)}</dd></div><div><dt>交易记录数</dt><dd>{overview.data.transactions}</dd></div></dl><p className="hint">这里只统计钱包余额，不包含游戏内筹码与在途资产。</p></section>}
 <section className="panel ops-business-account"><h2>账户筛选</h2><form className="filters" onSubmit={filter}><label>账户编号（可选）<input value={input} onChange={e=>setInput(e.target.value)} inputMode="numeric" pattern="[1-9][0-9]*" maxLength={10} placeholder="全部账户"/></label><button type="submit">查询账户与账变</button></form>
 {notice&&<p role="status">{notice}</p>}{error&&<Alert>{error}</Alert>}
 {user&&(account.loading?<Loading/>:account.error?<Alert>{notice?'调账已保存，资产读取失败，请重新查询账户。':account.error}</Alert>:account.data&&<><h3>账户 {user}</h3>{assets?<><p>总资产：{opsAmount(assets.total_units)} · 观察时间：{opsTime(assets.observed_at)}</p><dl className="ops-impact">{assetFields.map(([key,label])=><div key={key}><dt>{label}</dt><dd>{opsAmount(assets[key])}</dd></div>)}</dl><ChipAdjustment key={version} disabled={busy} save={adjust}/></>:<p>账户尚未初始化钱包。</p>}<Link to={'/ops/users/'+user}>管理账户与调整 Reserve</Link><details><summary>迁移与注册奖励</summary><OpsFacts value={account.data.migration}/></details></>)}
 </section><OpsLedger key={user+':'+version} client={client} user={user}/><details className="panel"><summary>历史额度划转（只读）</summary><OpsTransfers key={user} client={client} user={user}/></details><p><Link to="/ops/gambler-campaign">赌怪一次性活动</Link></p></>;
}
function ChipAdjustment({disabled,save}:{disabled:boolean;save:(body:unknown)=>Promise<void>}){
 const [amount,setAmount]=useState(''),[note,setNote]=useState(''),[requestID]=useState(()=>crypto.randomUUID());
 return <form onSubmit={e=>{e.preventDefault();if(amount.startsWith('-')&&!window.confirm('确认扣减此账户的可用筹码？'))return;void save({amount,note,request_id:requestID})}}><fieldset disabled={disabled}><label>调整可用筹码（负数扣减）<input required inputMode="decimal" value={amount} onChange={e=>setAmount(e.target.value)}/></label><label>筹码调账备注<textarea required maxLength={1000} value={note} onChange={e=>setNote(e.target.value)}/></label><button type="submit">保存筹码调账</button></fieldset><p>以 Credit 为单位；最小变动 0.000002。只改可用筹码，不改游戏内托管资金。</p></form>;
}
