import { ApiError, type ApiClient } from './api';
import { assets, amount as validAmount, integer, type Asset } from './wallet-api';
export interface Transaction { id:string; user_id:string; biz_id:string; kind:'DAILY_REWARD'|'HOURLY_REWARD'|'RELIEF_REWARD'|'LOCAL_EXCHANGE'|'INITIAL_GRANT_REGISTRATION'|'API_CHIPS_EXCHANGE'; status:'CONFIRMED'|'PENDING'|'SOURCE_DEBITED'|'COMPENSATING'|'COMPENSATED'|'FAILED_NO_EFFECT'|'NEEDS_REVIEW'; from_asset:Asset|''; to_asset:Asset; amount_units:string; amount:string; created_at:string; confirmed_at?:string; reserve_debit_units?:string; active_debit_units?:string; chips_credit_units?:string; reason?:string }
export const transactionStatus:Record<Transaction['status'],string>={CONFIRMED:'已确认',PENDING:'后台处理中',SOURCE_DEBITED:'来源已扣除，等待筹码入账',COMPENSATING:'正在退回原来源',COMPENSATED:'兑换失败，已退回原来源',FAILED_NO_EFFECT:'兑换失败，未产生扣款',NEEDS_REVIEW:'待人工核对'};
export const transactionTerminal=(t:Transaction)=>['CONFIRMED','COMPENSATED','FAILED_NO_EFFECT'].includes(t.status);
export function unitsAmount(units:bigint):string {const micros=units*2n;return `${micros/1000000n}${micros%1000000n?`.${String(micros%1000000n).padStart(6,'0').replace(/0+$/,'')}`:''}`}
export interface Daily { user_id:string; business_date:string; timezone:'Asia/Shanghai'; next_reset_at:string; amount:'500'; amount_units:'250000000'; asset:'RESERVE_API_CREDIT'; policy_version:'1'; claimed:boolean; transaction_id:string|null; maintenance_active:boolean }
export interface Hourly { user_id:string; reward_hour:string; business_date:string; timezone:'Asia/Shanghai'; next_reset_at:string; amount:'100'; amount_units:'50000000'; asset:'RESERVE_API_CREDIT'; policy_version:'1'; claimed:boolean; transaction_id:string|null; claims_today:number; daily_limit:24; accumulation:false }
export interface Relief { user_id:string; amount:'300'; amount_units:'150000000'; asset:'RESERVE_API_CREDIT'; policy_version:'1'; threshold:'10'; threshold_units:'5000000'; current_total_assets:string; current_total_assets_units:string; assets_observed_at:string; eligible:boolean; cooldown_seconds:'14400'; next_eligible_at:string|null; last_transaction_id:string|null; accumulation:false }
export interface TransactionPage { items:Transaction[]; has_more:boolean; next_after_id:string|null }
export interface PendingOperation { kind:'DAILY'|'HOURLY'|'RELIEF'|'EXCHANGE'; key:string; from_asset?:Asset; amount?:string }
const record=(v:unknown):v is Record<string,unknown>=>!!v && typeof v==='object' && !Array.isArray(v);
const uuid=(v:unknown):v is string=>typeof v==='string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(v);
const timestamp=(v:unknown):v is string=>typeof v==='string' && /^\d{4}-\d\d-\d\dT.+Z$/.test(v) && Number.isFinite(Date.parse(v));
const malformed=()=>new ApiError('交易响应异常，请核对交易结果后再操作。');
export function parseTransaction(v:unknown,userID:string):Transaction {
 if(!record(v) || !uuid(v.id) || v.user_id!==userID || typeof v.biz_id!=='string' || !v.biz_id || !integer(v.amount_units) || BigInt(v.amount_units)<=0n || !validAmount(v.amount,v.amount_units) || !timestamp(v.created_at) || !assets.includes(v.to_asset as Asset))throw malformed();
 if(v.kind==='API_CHIPS_EXCHANGE'){
  if(typeof v.status!=='string' || !Object.hasOwn(transactionStatus,v.status) || v.from_asset!=='RESERVE_API_CREDIT' || v.to_asset!=='AVAILABLE_CHIPS' || !integer(v.reserve_debit_units) || !integer(v.active_debit_units) || BigInt(v.active_debit_units)<=0n || BigInt(v.active_debit_units)>2147483647n || v.chips_credit_units!==v.amount_units || BigInt(v.reserve_debit_units)+BigInt(v.active_debit_units)!==BigInt(v.amount_units) || (v.status==='CONFIRMED' && !timestamp(v.confirmed_at)))throw malformed();
  return v as unknown as Transaction;
 }
 if(v.status!=='CONFIRMED' || !timestamp(v.confirmed_at))throw malformed();
 if(v.kind==='INITIAL_GRANT_REGISTRATION'){
  if(v.from_asset!=='' || v.to_asset!=='RESERVE_API_CREDIT' || v.amount_units!=='500000000' || v.biz_id!==`initial_grant:registration:${userID}`)throw malformed();
 }else if(v.kind==='DAILY_REWARD' || v.kind==='HOURLY_REWARD' || v.kind==='RELIEF_REWARD'){
  const units={DAILY_REWARD:'250000000',HOURLY_REWARD:'50000000',RELIEF_REWARD:'150000000'}[v.kind];
  if(v.from_asset!=='' || v.to_asset!=='RESERVE_API_CREDIT' || v.amount_units!==units)throw malformed();
 }else if(v.kind!=='LOCAL_EXCHANGE' || !assets.includes(v.from_asset as Asset) || v.from_asset===v.to_asset)throw malformed();
 return v as unknown as Transaction;
}
export function parseDaily(v:unknown,userID:string):Daily {
 if(!record(v) || v.user_id!==userID || typeof v.business_date!=='string' || !/^\d{4}-\d\d-\d\d$/.test(v.business_date) || v.timezone!=='Asia/Shanghai' || !timestamp(v.next_reset_at) || v.amount!=='500' || v.amount_units!=='250000000' || v.asset!=='RESERVE_API_CREDIT' || v.policy_version!=='1' || typeof v.claimed!=='boolean' || (v.claimed?!uuid(v.transaction_id):v.transaction_id!==null) || typeof v.maintenance_active!=='boolean')throw malformed();
 return v as unknown as Daily;
}
export function parseHourly(v:unknown,userID:string):Hourly {
 if(!record(v) || v.user_id!==userID || !timestamp(v.reward_hour) || typeof v.business_date!=='string' || !/^\d{4}-\d\d-\d\d$/.test(v.business_date) || v.timezone!=='Asia/Shanghai' || !timestamp(v.next_reset_at) || v.amount!=='100' || v.amount_units!=='50000000' || v.asset!=='RESERVE_API_CREDIT' || v.policy_version!=='1' || typeof v.claimed!=='boolean' || (v.claimed?!uuid(v.transaction_id):v.transaction_id!==null) || typeof v.claims_today!=='number' || !Number.isInteger(v.claims_today) || v.claims_today<0 || v.claims_today>24 || v.daily_limit!==24 || v.accumulation!==false)throw malformed();
 return v as unknown as Hourly;
}
export function parseRelief(v:unknown,userID:string):Relief {
 if(!record(v) || v.user_id!==userID || v.amount!=='300' || v.amount_units!=='150000000' || v.asset!=='RESERVE_API_CREDIT' || v.policy_version!=='1' || v.threshold!=='10' || v.threshold_units!=='5000000' || !integer(v.current_total_assets_units) || !validAmount(v.current_total_assets,v.current_total_assets_units) || !timestamp(v.assets_observed_at) || typeof v.eligible!=='boolean' || v.cooldown_seconds!=='14400' || !(v.next_eligible_at===null || timestamp(v.next_eligible_at)) || !(v.last_transaction_id===null || uuid(v.last_transaction_id)) || (v.last_transaction_id===null)!==(v.next_eligible_at===null) || v.accumulation!==false)throw malformed();
 return v as unknown as Relief;
}
export function parseTransactions(v:unknown,userID:string):TransactionPage {
 if(!record(v) || !Array.isArray(v.items) || v.items.length>20 || typeof v.has_more!=='boolean')throw malformed();
 const items=v.items.map(t=>parseTransaction(t,userID));
 if(items.some((t,i)=>i>0 && t.id>=items[i-1].id) || (v.has_more ? !items.length || v.next_after_id!==items[items.length-1].id : v.next_after_id!==null))throw malformed();
 return {items,has_more:v.has_more,next_after_id:v.next_after_id as string|null};
}
export function amountUnits(text:string):bigint|null {
 if(text.length>30 || !/^(0|[1-9]\d*)(\.\d{1,6})?$/.test(text))return null;
 const [whole,fraction='']=text.split('.');const micros=BigInt(whole)*1000000n+BigInt(fraction.padEnd(6,'0'));
 if(micros%2n || micros<=0n || micros/2n>9223372036854775807n)return null;return micros/2n;
}
export function parsePending(raw:string|null):PendingOperation|null {
 if(raw===null)return null;
 const p:unknown=JSON.parse(raw);
 if(!record(p) || !uuid(p.key) || !['DAILY','HOURLY','RELIEF','EXCHANGE'].includes(p.kind as string) || (p.kind==='EXCHANGE' && (!assets.includes(p.from_asset as Asset) || typeof p.amount!=='string' || amountUnits(p.amount)===null)))throw malformed();
 return p as unknown as PendingOperation;
}
export function matchesOperation(t:Transaction,p:PendingOperation):boolean {
 if(p.kind==='EXCHANGE')return (t.kind==='LOCAL_EXCHANGE' || t.kind==='API_CHIPS_EXCHANGE') && t.from_asset===p.from_asset && BigInt(t.amount_units)===amountUnits(p.amount!);
 return t.kind===`${p.kind}_REWARD`;
}
export const readDaily=async(c:ApiClient,u:string)=>parseDaily(await c.request('/platform/v1/rewards/daily'),u);
export const readHourly=async(c:ApiClient,u:string)=>parseHourly(await c.request('/platform/v1/rewards/hourly'),u);
export const readRelief=async(c:ApiClient,u:string)=>parseRelief(await c.request('/platform/v1/rewards/relief'),u);
export const readTransactions=async(c:ApiClient,u:string,after='')=>parseTransactions(await c.request(`/platform/v1/transactions${after?`?after_id=${after}`:''}`),u);
export const operationError=(e:unknown)=>{
 const messages:Record<string,string>={TRANSFER_PENDING:'已有未完成的兑换或额度划转，请先核对原交易。',INSUFFICIENT_BALANCE:'来源钱包余额不足，请调整兑换数量。',IDEMPOTENCY_CONFLICT:'原请求已绑定其他交易，请先核对交易记录。',WALLET_NOT_INITIALIZED:'请先初始化零余额钱包。',BALANCE_OVERFLOW:'目标余额超出存储范围，本次未兑换。',HOURLY_DAILY_LIMIT:'今日小时奖励已达到 24 次上限。',RELIEF_COOLDOWN:'上一份救济仍在四小时冷却中。',RELIEF_NOT_ELIGIBLE:'当前总资产不少于 10 API Credit，暂不符合救济条件。',MAINTENANCE_ACTIVE:'当前维护暂停新的资产与奖励操作，请稍后再试。',AMOUNT_INVALID:'请输入正数，最多六位小数。',AMOUNT_NOT_REPRESENTABLE:'数量须是 0.000002 的整数倍，不会自动四舍五入。',AMOUNT_OUT_OF_RANGE:'数量超出存储范围。',INVALID_REQUEST:'请求字段异常，请刷新页面核对。',ORIGIN_REJECTED:'请求来源未通过检查。'};
 return e instanceof ApiError && Object.hasOwn(messages,e.code)?messages[e.code]:'请求未完成，请核对交易结果。';
};
