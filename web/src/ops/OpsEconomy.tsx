import { useState, type FormEvent } from 'react';
import { Alert, Loading, useResource } from '../ui';
import { opsError, opsID } from './ops-api';
import { OpsFacts, opsTime } from './OpsOperationDetail';
import { OpsAdjustment, OpsLedger, OpsTransfers, type OpsWallet } from './OpsEconomyRecords';
import { useOps } from './OpsShell';

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
type Overview={observed_at:string;wallet_accounts:string;reserve_units:string;available_chips_units:string;transactions:string;pending_transfers:string;pending_transfer_units:string;needs_review_transfers:string;needs_review_transfer_units:string};
type Assets={user_id:string;active_quota_units:string;reserve_units:string;available_chips_units:string;poker_stack_units:string;poker_pot_units:string;quota_transfer_in_flight_units:string;total_units:string;total_amount:string;observed_at:string;native_observed_at:string};
type Migration={migration_batch_id:string|null;account_created_at:string;first_seen_at:string;registration_claim_id:string|null;registration_status:string;registration_transaction_id:string|null};
type UserResponse={observed_at:string;assets:Assets;wallets:OpsWallet[];migration:Migration};
const assetFields=[['active_quota_units','Active API Quota'],['reserve_units','Reserve API Credit'],['available_chips_units','可用筹码'],['poker_stack_units','Poker 桌上筹码'],['poker_pot_units','Poker 底池投入'],['quota_transfer_in_flight_units','额度转账在途']] as const;
function checkOverview(value:unknown):Overview{
 if(!opsRecord(value)||!opsTimestamp(value.observed_at)||(['wallet_accounts','transactions','pending_transfers','needs_review_transfers'] as const).some(key=>!opsInt64(value[key]))||(['reserve_units','available_chips_units','pending_transfer_units','needs_review_transfer_units'] as const).some(key=>!aggregateUnsigned(value[key])))throw new Error('经济总览响应格式异常。');
 const result=value as unknown as Overview;
 if((result.pending_transfers==='0')!==(result.pending_transfer_units==='0')||(result.needs_review_transfers==='0')!==(result.needs_review_transfer_units==='0'))throw new Error('经济总览响应格式异常。');
 return result;
}
function checkAssets(value:unknown,user:string):Assets{
 if(!opsRecord(value))throw new Error('统一资产数据无法核对。');
 const result=value as unknown as Assets;
 if(!opsUserID(result.user_id)||result.user_id!==user||!opsInt64(result.total_units)||assetFields.some(([key])=>!opsInt64(result[key]))||!opsTimestamp(result.observed_at)||!opsTimestamp(result.native_observed_at))throw new Error('统一资产数据无法核对。');
 if(assetFields.reduce((sum,[key])=>sum+BigInt(result[key]),0n)!==BigInt(result.total_units)||!opsAmountMatches(result.total_amount,result.total_units))throw new Error('统一资产数据无法核对。');
 return result;
}
function checkMigration(value:unknown):Migration{
 if(!opsRecord(value)||!(value.migration_batch_id===null||opsID(value.migration_batch_id))||!opsTimestamp(value.account_created_at)||!opsTimestamp(value.first_seen_at)||!(value.registration_claim_id===null||opsID(value.registration_claim_id))||!(value.registration_transaction_id===null||opsID(value.registration_transaction_id))||typeof value.registration_status!=='string')throw new Error('账户迁移数据无法核对。');
 const result=value as unknown as Migration;
 if(result.registration_claim_id===null?(result.registration_status!==''||result.registration_transaction_id!==null):(!['PENDING','RECOVERING','CONFIRMED'].includes(result.registration_status)||(result.registration_status==='CONFIRMED')!==(result.registration_transaction_id!==null)))throw new Error('账户迁移数据无法核对。');
 return result;
}
function checkWallets(value:unknown,user:string):OpsWallet[]{
 if(!Array.isArray(value)||(value.length!==0&&value.length!==2))throw new Error('钱包响应无法核对。');
 if(value.some(wallet=>!opsRecord(wallet)||wallet.user_id!==user||!opsUserID(wallet.user_id)||!opsAsset(wallet.asset)||!opsInt64(wallet.balance_units)||!opsInt64(wallet.ledger_seq)||!opsPositiveInt64(wallet.version)))throw new Error('钱包响应无法核对。');
 if(value.length===2&&['RESERVE_API_CREDIT','AVAILABLE_CHIPS'].some(asset=>value.filter(wallet=>wallet.asset===asset).length!==1))throw new Error('钱包响应无法核对。');
 return value as OpsWallet[];
}
function checkUserResponse(value:unknown,user:string):UserResponse{
 if(!opsRecord(value)||!opsTimestamp(value.observed_at))throw new Error('账户经济响应无法核对。');
 const assets=checkAssets(value.assets,user),wallets=checkWallets(value.wallets,user),migration=checkMigration(value.migration);
 if(value.observed_at!==assets.observed_at)throw new Error('账户经济响应无法核对。');
 return {observed_at:value.observed_at,assets,wallets,migration};
}
export function OpsEconomy(){
 const {client}=useOps();const [input,setInput]=useState(''),[user,setUser]=useState(''),[error,setError]=useState('');
 const overview=useResource(async()=>{try{return checkOverview(await client.request<unknown>('/api/v1/ops/economy/overview'))}catch(error){throw new Error(opsError(error));}},[client]);
 const account=useResource(async()=>{if(!user)return null;try{return checkUserResponse(await client.request<unknown>(`/api/v1/ops/economy/user?newapi_user_id=${user}`),user)}catch(error){throw new Error(opsError(error));}},[client,user]);
 function filter(event:FormEvent){event.preventDefault();if(!opsUserID(input.trim())){setError('请输入有效的账户编号。');return}setError('');if(user===input.trim())account.reload();else setUser(input.trim())}
 return <><header className="page-heading"><div><p className="eyebrow">OPERATIONS / ECONOMY</p><h1>经济与账务</h1><p>核对本地钱包规模、额度转账和单个账户的统一资产。</p></div><button onClick={overview.reload} disabled={overview.loading}>刷新总览</button></header>{overview.loading?<Loading/>:overview.error?<Alert>{overview.error}</Alert>:overview.data&&<section className="panel"><p>观察时间：{opsTime(overview.data.observed_at)}</p><dl className="ops-impact"><div><dt>钱包账户数</dt><dd>{overview.data.wallet_accounts}</dd></div><div><dt>Reserve API Credit</dt><dd>{opsAmount(overview.data.reserve_units)}</dd></div><div><dt>可用筹码</dt><dd>{opsAmount(overview.data.available_chips_units)}</dd></div><div><dt>交易记录数</dt><dd>{overview.data.transactions}</dd></div><div><dt>待完成转账</dt><dd>{overview.data.pending_transfers} 笔 · {opsAmount(overview.data.pending_transfer_units)}</dd></div><div><dt>需核对转账</dt><dd>{overview.data.needs_review_transfers} 笔 · {opsAmount(overview.data.needs_review_transfer_units)}</dd></div></dl></section>}<section className="panel"><h2>账户资产查询</h2><form className="filters" onSubmit={filter}><label>账户编号<input value={input} onChange={event=>setInput(event.target.value)} maxLength={19} inputMode="numeric" required/></label><button type="submit" disabled={account.loading&&!!user}>查询账户</button></form>{error&&<Alert>{error}</Alert>}{user&&(account.loading?<Loading/>:account.error?<Alert>{account.error}</Alert>:account.data&&<><h3>账户 {account.data.assets.user_id}</h3><dl className="ops-impact">{assetFields.map(([key,label])=><div key={key}><dt>{label}</dt><dd>{opsAmount(account.data!.assets[key])}</dd></div>)}<div><dt>统一资产合计</dt><dd>{opsAmount(account.data.assets.total_units)}</dd></div></dl><p className="hint">本地观察：{opsTime(account.data.assets.observed_at)}；Native 观察：{opsTime(account.data.assets.native_observed_at)}。</p><details><summary>账户迁移与注册奖励</summary><OpsFacts value={account.data.migration}/></details><OpsAdjustment key={user} user={user} wallets={account.data.wallets}/></>)}</section>{user&&<OpsLedger key={user} user={user}/>}<OpsTransfers/></>;
}

