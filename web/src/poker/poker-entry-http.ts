import { ApiError, type ApiClient } from '../api';
import { integer } from '../wallet-api';
import { pokerChipAmount, pokerTimestamp, pokerUUID } from './poker-view';
import { parsePokerReceipt, type PokerReceipt } from './poker-wire';

export interface PokerEntryScope { user_id:string;session_generation:number;entry_generation:number }
export type PokerEntryCommand=
  | {kind:'create';request_id:string;name:string;blind_preset:string;max_seats:number;allow_spectators:boolean;access_mode:'PUBLIC'|'PASSWORD';password?:string;chat_enabled:boolean}
  | {kind:'reserve';request_id:string;table_id:string;seat_no:number}
  | {kind:'buyin';request_id:string;table_id:string;reservation_id:string;amount_units:string};
export type PokerEntryLookup={user_id:string;kind:PokerEntryCommand['kind'];mutation_id:string}&({state:'FOUND';receipt:PokerReceipt}|{state:'NOT_FOUND'});
export type PokerReservationLookup={user_id:string;table_id:string;reservation_id:string}&({state:'NOT_FOUND'}|{state:'FOUND';reservation:{seat_no:number;durable_state:'LEASE_ACTIVE'|'CONSUMED'|'EXPIRED'|'FAILED_NO_EFFECT';expires_at:string;checked_at:string;valid:boolean}});
type CurrentScope=()=>PokerEntryScope|null;
type Value=Record<string,unknown>;
const base='/api/v1/poker',scopeKeys=['user_id','session_generation','entry_generation'] as const;
const invalid=()=>new ApiError('入桌请求或原操作记录尚未核实。',0,'POKER_INVALID_ENTRY');
function need(ok:unknown):asserts ok {if(!ok)throw invalid();}
function object(raw:unknown,required:string,optional=''):Value {
  need(raw!==null&&typeof raw==='object'&&!Array.isArray(raw));
  need(Object.getPrototypeOf(raw)===Object.prototype||Object.getPrototypeOf(raw)===null);
  const v=raw as Value,keys=required.split(' '),allowed=[...keys,...optional.split(' ')];
  need(keys.every(k=>Object.hasOwn(v,k))&&Object.keys(v).every(k=>allowed.includes(k)));return v;
}
const range=(v:unknown,min:number,max:number)=>typeof v==='number'&&Number.isInteger(v)&&v>=min&&v<=max;
const positive=(v:unknown)=>integer(v)&&BigInt(v)>0n;
/** Entry lifecycle is independent of read pagination/filter generations. */
export function capturePokerEntryScope(client:ApiClient,generation:number):PokerEntryScope {
  const auth=client.getSnapshot(),session=client.getSessionGeneration();
  need(auth.ready&&!auth.loggingOut&&auth.user&&Number.isSafeInteger(auth.user.id)&&auth.user.id>0);
  need([generation,session].every(n=>Number.isSafeInteger(n)&&n>=0));
  return{user_id:String(auth.user.id),session_generation:session,entry_generation:generation};
}
export function currentPokerEntryScope(client:ApiClient,scope:PokerEntryScope,getCurrent:CurrentScope):boolean {
  const current=getCurrent();if(!current||scopeKeys.some(k=>current[k]!==scope[k]))return false;
  try{const actual=capturePokerEntryScope(client,scope.entry_generation);return scopeKeys.every(k=>actual[k]===scope[k]);}catch{return false;}
}
/** Closed supported command; extra access/password/host fields are not silently dropped. */
function bind(command:PokerEntryCommand):{path:string;body:Value} {
  const v=object(command,'kind request_id','name blind_preset max_seats allow_spectators access_mode password chat_enabled table_id seat_no reservation_id amount_units');
  need(typeof v.request_id==='string'&&/^[\x21-\x7e]{16,128}$/.test(v.request_id));
  if(v.kind==='create'){
    object(v,'kind request_id name blind_preset max_seats allow_spectators access_mode chat_enabled','password');
    need(typeof v.name==='string'&&v.name.length>0&&v.name.trim()===v.name&&!/[\p{Cc}\p{Cs}]/u.test(v.name)&&Array.from(new Intl.Segmenter(undefined,{granularity:'grapheme'}).segment(v.name)).length<=40);
    need(typeof v.blind_preset==='string'&&v.blind_preset.length>0&&new TextEncoder().encode(v.blind_preset).length<=64&&!/[\p{Cc}\p{Cs}]/u.test(v.blind_preset));
    need(range(v.max_seats,2,9)&&typeof v.allow_spectators==='boolean'&&typeof v.chat_enabled==='boolean'&&(v.access_mode==='PUBLIC'||v.access_mode==='PASSWORD'));
    if(v.access_mode==='PASSWORD')need(typeof v.password==='string'&&v.password.length>0&&new TextEncoder().encode(v.password).length<=128);
    else need(!Object.hasOwn(v,'password'));
    return{path:`${base}/tables`,body:{request_id:v.request_id,name:v.name,blind_preset:v.blind_preset,max_seats:v.max_seats,allow_spectators:v.allow_spectators,access_mode:v.access_mode,chat_enabled:v.chat_enabled,...(v.access_mode==='PASSWORD'?{password:v.password}:{})}};
  }
  need(pokerUUID(v.table_id));
  if(v.kind==='reserve'){
    object(v,'kind request_id table_id seat_no');need(range(v.seat_no,1,9));
    return{path:`${base}/tables/${v.table_id}/seat-reservations`,body:{request_id:v.request_id,seat_no:v.seat_no}};
  }
  need(v.kind==='buyin');object(v,'kind request_id table_id reservation_id amount_units');
  need(pokerUUID(v.reservation_id)&&pokerChipAmount(v.amount_units)&&BigInt(v.amount_units)>0n);
  return{path:`${base}/tables/${v.table_id}/buy-ins`,body:{request_id:v.request_id,reservation_id:v.reservation_id,amount_units:v.amount_units}};
}

/** Password is scoped to this one POST and is never returned, logged or placed in a URL. */
export async function verifyPokerTableAccess(client:ApiClient,scope:PokerEntryScope,getCurrent:CurrentScope,table:string,password:string,canDispatch?:()=>boolean):Promise<boolean> {
  const captured={...scope},current=()=>currentPokerEntryScope(client,captured,getCurrent);
  need(current()&&pokerUUID(table)&&typeof password==='string'&&password.length>0&&new TextEncoder().encode(password).length<=128);
  try{
    const raw=await client.request(`${base}/tables/${table}/access`,'POST',{password},undefined,()=>current()&&(canDispatch?.()??true));
    if(!current())return false;
    const result=object(raw,'table_id');need(result.table_id===table);return true;
  }catch(error){
    if(!current())return false;
    const status=error instanceof ApiError?error.status:0,code=error instanceof ApiError&&/^[A-Z0-9_]{1,80}$/.test(error.code)?error.code:'TABLE_ACCESS_UNAVAILABLE';
    throw new ApiError(code==='TABLE_PASSWORD_INVALID'?'牌桌密码不正确。':code==='RATE_LIMITED'?'密码尝试过于频繁，请稍后再试。':'牌桌访问权限尚未确认。',status,code);
  }
}
/** A receipt is the original command outcome, never current admission or control authority. */
export function parsePokerEntryReceipt(raw:unknown,command:PokerEntryCommand):PokerReceipt {
  bind(command);const value=object(raw,'table_id status table_version duplicate','reservation_id session_id funding_operation_id amount_units failure_code');
  need(pokerUUID(value.table_id));const receipt=parsePokerReceipt(value,command.kind==='create'?value.table_id:command.table_id);need(BigInt(receipt.table_version)>0n);
  const common='table_id status table_version duplicate';
  if(command.kind==='create'){object(value,common);need(receipt.status==='WAITING');}
  else if(command.kind==='reserve'){object(value,`${common} reservation_id`);need(receipt.status==='LEASE_ACTIVE');}
  else if(receipt.status==='CONFIRMED'){
    object(value,`${common} session_id funding_operation_id amount_units`);need(receipt.amount_units===command.amount_units);
  }else{object(value,`${common} failure_code`,'funding_operation_id');need(receipt.status==='FAILED_NO_EFFECT');}
  return receipt;
}
export function parsePokerEntryLookup(raw:unknown,user:string,command:PokerEntryCommand):PokerEntryLookup {
  bind(command);need(positive(user));const v=object(raw,'user_id kind mutation_id state','receipt');
  need(v.user_id===user&&v.kind===command.kind&&v.mutation_id===command.request_id);
  if(v.state==='NOT_FOUND'){object(v,'user_id kind mutation_id state');return{user_id:user,kind:command.kind,mutation_id:command.request_id,state:'NOT_FOUND'};}
  need(v.state==='FOUND'&&Object.hasOwn(v,'receipt'));
  return{user_id:user,kind:command.kind,mutation_id:command.request_id,state:'FOUND',receipt:parsePokerEntryReceipt(v.receipt,command)};
}
// PG timestamps can differ within one JS millisecond; retain the RFC3339 fractional remainder.
const timestampNS=(v:string)=>BigInt(Date.parse(v))*1000000n+BigInt((v.match(/\.(\d+)/)?.[1]??'').padEnd(9,'0').slice(3));
export function parsePokerReservation(raw:unknown,user:string,table:string,reservation:string):PokerReservationLookup {
  need(positive(user)&&pokerUUID(table)&&pokerUUID(reservation));const v=object(raw,'user_id table_id reservation_id state','reservation');
  need(v.user_id===user&&v.table_id===table&&v.reservation_id===reservation);
  if(v.state==='NOT_FOUND')object(v,'user_id table_id reservation_id state');
  else{
    need(v.state==='FOUND');const r=object(v.reservation,'seat_no durable_state expires_at checked_at valid');
    need(range(r.seat_no,1,9)&&['LEASE_ACTIVE','CONSUMED','EXPIRED','FAILED_NO_EFFECT'].includes(r.durable_state as string)&&typeof r.valid==='boolean');
    need(pokerTimestamp(r.expires_at)&&pokerTimestamp(r.checked_at));
    need(!r.valid||(r.durable_state==='LEASE_ACTIVE'&&timestampNS(r.expires_at)>timestampNS(r.checked_at)));
  }
  return structuredClone(v) as unknown as PokerReservationLookup;
}
/** One shot only. Caller owns the synchronous pending latch, original key and explicit recovery actions. */
export async function submitPokerEntry(client:ApiClient,scope:PokerEntryScope,getCurrent:CurrentScope,command:PokerEntryCommand,canDispatch?:()=>boolean):Promise<PokerReceipt|undefined> {
  const captured={...scope},current=()=>currentPokerEntryScope(client,captured,getCurrent);need(current());
  const binding=bind(command),original=structuredClone(command);
  try{const raw=await client.request(binding.path,'POST',binding.body,undefined,()=>current()&&(canDispatch?.()??true));if(!current())return undefined;return parsePokerEntryReceipt(raw,original);}
  catch(error){if(!current())return undefined;throw new ApiError('入桌操作结果尚未确认，请核对原请求，勿重复提交。',error instanceof ApiError?error.status:0,'POKER_ENTRY_OUTCOME_UNCONFIRMED',true);}
}
export async function readPokerEntryReceipt(client:ApiClient,scope:PokerEntryScope,getCurrent:CurrentScope,command:PokerEntryCommand):Promise<PokerEntryLookup|undefined> {
  const captured={...scope},current=()=>currentPokerEntryScope(client,captured,getCurrent);need(current());
  bind(command);const original=structuredClone(command);
  const body={kind:original.kind,mutation_id:original.request_id,...(original.kind==='create'?{}:{table_id:original.table_id})};
  try{const raw=await client.request(`${base}/entry-receipt-query`,'POST',body,undefined,current);if(!current())return undefined;return parsePokerEntryLookup(raw,captured.user_id,original);}
  catch(error){if(!current())return undefined;throw new ApiError('原入桌回执暂未核实，请重新查询。',error instanceof ApiError?error.status:0,'POKER_ENTRY_READ_UNAVAILABLE');}
}
export async function readPokerReservation(client:ApiClient,scope:PokerEntryScope,getCurrent:CurrentScope,table:string,reservation:string):Promise<PokerReservationLookup|undefined> {
  const captured={...scope},current=()=>currentPokerEntryScope(client,captured,getCurrent);need(current()&&pokerUUID(table)&&pokerUUID(reservation));
  try{const raw=await client.request(`${base}/tables/${table}/reservation-query`,'POST',{reservation_id:reservation},undefined,current);if(!current())return undefined;return parsePokerReservation(raw,captured.user_id,table,reservation);}
  catch(error){if(!current())return undefined;throw new ApiError('原座位预留暂未核实，请重新查询。',error instanceof ApiError?error.status:0,'POKER_ENTRY_READ_UNAVAILABLE');}
}
