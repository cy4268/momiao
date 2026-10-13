import { useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { ApiClient } from '../api';
import { Alert, Loading, useResource } from '../ui';
import { OpsFacts, opsTime } from './OpsOperationDetail';
import { OpsLedger, OpsTransfers, type OpsWallet, assetNames } from './OpsEconomyRecords';
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
type UserResponse={observed_at:string;wallets:OpsWallet[];migration:unknown};
export function OpsEconomy({client}:{client:ApiClient}){
 const [input,setInput]=useState(''),[user,setUser]=useState('');
 const overview=useResource(()=>client.request<Overview>('/api/v1/ops/economy/overview'),[client]);
 const account=useResource(()=>user?client.request<UserResponse>(`/api/v1/ops/economy/user?newapi_user_id=${user}`):Promise.resolve(null),[client,user]);
 function filter(event:FormEvent){event.preventDefault();setUser(input.trim());account.reload()}
 return <><header className="page-heading"><div><p className="eyebrow">OPERATIONS / ECONOMY</p><h1>经济与账务</h1><p>全局钱包账变与奖励资金记录；留空账户编号查看全部账户。</p></div><button onClick={overview.reload} disabled={overview.loading}>刷新总览</button></header>
 {overview.loading?<Loading/>:overview.error?<Alert>{overview.error}</Alert>:overview.data&&<section className="panel ops-business-economy-overview"><h2>账务总览</h2><p className="hint">观察时间：{opsTime(overview.data.observed_at)}</p><dl className="ops-impact"><div><dt>钱包账户数</dt><dd>{overview.data.wallet_accounts}</dd></div><div><dt>Reserve</dt><dd>{opsAmount(overview.data.reserve_units)}</dd></div><div><dt>可用筹码</dt><dd>{opsAmount(overview.data.available_chips_units)}</dd></div><div><dt>交易记录数</dt><dd>{overview.data.transactions}</dd></div></dl><p className="hint">这里只统计钱包余额，不包含游戏内筹码与在途资产。</p></section>}
 <section className="panel ops-business-account"><h2>账户筛选</h2><form className="filters" onSubmit={filter}><label>账户编号（可选）<input value={input} onChange={e=>setInput(e.target.value)} inputMode="numeric" pattern="[1-9][0-9]*" maxLength={10} placeholder="全部账户"/></label><button type="submit">查询账户与账变</button></form>
 {user&&(account.loading?<Loading/>:account.error?<Alert>{account.error}</Alert>:account.data&&<><h3>账户 {user}</h3><dl className="ops-impact">{account.data.wallets.map(v=><div key={v.asset}><dt>{assetNames[v.asset]}</dt><dd>{opsAmount(v.balance_units)}</dd></div>)}</dl>{!account.data.wallets.length&&<p>账户尚未初始化钱包。</p>}<Link to={'/ops/users/'+user}>管理账户与调整 Reserve</Link><details><summary>迁移与注册奖励</summary><OpsFacts value={account.data.migration}/></details></>)}
 </section><OpsLedger key={user} client={client} user={user}/><details className="panel"><summary>历史额度划转（只读）</summary><OpsTransfers key={user} client={client} user={user}/></details><p><Link to="/ops/gambler-campaign">赌怪一次性活动（原管理入口）</Link></p></>;
}
