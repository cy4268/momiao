import { ApiError } from './api';
import { amountUnits } from './economy-api';
import { amount, integer } from './wallet-api';
export interface NativeQuota { user_id:string; raw_quota:string; amount:string; enabled:boolean }
export interface Transfer { id:string; user_id:string; amount_units:string; amount:string; status:'PENDING'|'CONFIRMED'|'REFUNDED'|'NEEDS_REVIEW'; reason:string; native_before:string|null; native_after:string|null; created_at:string; updated_at:string }
export interface TransferPending { key:string; amount:string }
export const maxNativeUnits=9007199254740991n;
const record=(v:unknown):v is Record<string,unknown>=>!!v && typeof v==='object' && !Array.isArray(v);
const uuid=(v:unknown):v is string=>typeof v==='string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(v);
const timestamp=(v:unknown)=>typeof v==='string' && /^\d{4}-\d\d-\d\dT.+Z$/.test(v) && Number.isFinite(Date.parse(v));
const malformed=()=>new ApiError('划转响应待核对，请保留原请求编号。');
export function parseNativeQuota(v:unknown,userID:string):NativeQuota {
 if(!record(v) || v.user_id!==userID || !amount(v.amount,v.raw_quota) || typeof v.enabled!=='boolean')throw malformed();
 return v as unknown as NativeQuota;
}
export function parseTransfer(v:unknown,userID:string):Transfer {
 if(!record(v) || !uuid(v.id) || v.user_id!==userID || !integer(v.amount_units) || BigInt(v.amount_units)<=0n || BigInt(v.amount_units)>maxNativeUnits || !amount(v.amount,v.amount_units) || !['PENDING','CONFIRMED','REFUNDED','NEEDS_REVIEW'].includes(String(v.status)) || typeof v.reason!=='string' || !timestamp(v.created_at) || !timestamp(v.updated_at) || (v.native_before!==null && !integer(v.native_before,true)) || (v.native_after!==null && !integer(v.native_after,true)))throw malformed();
 if(v.status==='CONFIRMED' && (!integer(v.native_before) || !integer(v.native_after) || BigInt(v.native_after)-BigInt(v.native_before)!==BigInt(v.amount_units)))throw malformed();
 return v as unknown as Transfer;
}
export function parseTransfers(v:unknown,userID:string):Transfer[] {
 if(!Array.isArray(v) || v.length>20)throw malformed();
 const items=v.map(t=>parseTransfer(t,userID));if(new Set(items.map(t=>t.id)).size!==items.length)throw malformed();return items;
}
export function parseTransferPending(raw:string|null):TransferPending|null {
 if(raw===null)return null;const p:unknown=JSON.parse(raw);
 if(!record(p) || !uuid(p.key) || typeof p.amount!=='string')throw malformed();const u=amountUnits(p.amount);
 if(u===null || u>maxNativeUnits)throw malformed();return {key:p.key,amount:p.amount};
}
export const transferStatus:Record<Transfer['status'],string>={PENDING:'后台处理中',CONFIRMED:'已转入',REFUNDED:'已退回 Reserve',NEEDS_REVIEW:'待人工核对'};
export function transferError(e:unknown):string {
 const messages:Record<string,string>={INSUFFICIENT_BALANCE:'Reserve 余额不足，请刷新钱包后调整数量。',WALLET_NOT_INITIALIZED:'请先在钱包页初始化零余额钱包。',TRANSFER_PENDING:'已有待处理划转，请刷新记录查看原请求。',INVALID_AMOUNT:'请输入正数，最小步长 0.000002，且不超出原生精度范围。',QUOTA_TRANSFER_UNAVAILABLE:'划转服务暂未就绪。已提交的请求请按原编号核对。',IDEMPOTENCY_CONFLICT:'原请求绑定的数量不一致，请保留编号核对。'};
 return e instanceof ApiError && Object.hasOwn(messages,e.code)?messages[e.code]:'结果尚未确认，请先核对原请求，勿重新发起。';
}
