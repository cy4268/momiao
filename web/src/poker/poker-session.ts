import { ApiError, type ApiClient } from '../api';
import { integer } from '../wallet-api';
import type { PokerReadScope } from './poker-client-boundary';
import { currentPokerScope } from './poker-http';
import { pokerChipAmount, pokerTimestamp, pokerUUID } from './poker-view';

export interface PokerSessionSummary {
  session_id:string;table_id:string;table_name:string;state:'ACTIVE'|'SETTLED'|'NEEDS_REVIEW';seat_no:number;
  stack_units:string;committed_units:string;poker_in_play_units:string;control_epoch:string;
  initial_buy_in_units:string;total_top_up_units:string;started_at:string;
  final_cash_out_units?:string;realized_pl_units?:string;ended_at?:string;end_reason?:string;
}
const invalid=()=>new ApiError('会话与出金状态尚未核实，请重新读取。',0,'POKER_INVALID_SESSION_READ');
function need(ok:unknown):asserts ok {if(!ok)throw invalid();}
const text=(v:unknown,max:number)=>typeof v==='string'&&v.length>0&&new TextEncoder().encode(v).length<=max;
const required='session_id table_id table_name state seat_no stack_units committed_units poker_in_play_units control_epoch initial_buy_in_units total_top_up_units started_at'.split(' ');
const optional='final_cash_out_units realized_pl_units ended_at end_reason'.split(' ');
/** Existing owner Session read model only: no receipt, wallet estimate or socket authority. */
export function parsePokerSession(raw:unknown,tableID:string,sessionID:string):PokerSessionSummary {
  need(pokerUUID(tableID)&&pokerUUID(sessionID)&&raw!==null&&typeof raw==='object'&&!Array.isArray(raw));
  need(Object.getPrototypeOf(raw)===Object.prototype||Object.getPrototypeOf(raw)===null);
  const v=raw as Record<string,unknown>,has=(key:string)=>Object.hasOwn(v,key);
  need(required.every(has)&&Object.keys(v).every(k=>required.includes(k)||optional.includes(k)));
  need(v.table_id===tableID&&v.session_id===sessionID&&text(v.table_name,65536));
  need(['ACTIVE','SETTLED','NEEDS_REVIEW'].includes(v.state as string)&&Number.isInteger(v.seat_no)&&(v.seat_no as number)>=1&&(v.seat_no as number)<=9);
  for(const k of ['stack_units','committed_units','poker_in_play_units','initial_buy_in_units','total_top_up_units'])need(pokerChipAmount(v[k]));
  need(integer(v.control_epoch)&&BigInt(v.control_epoch)>0n&&pokerTimestamp(v.started_at));
  need(BigInt(v.stack_units as string)+BigInt(v.committed_units as string)===BigInt(v.poker_in_play_units as string));
  if(has('final_cash_out_units'))need(pokerChipAmount(v.final_cash_out_units));
  if(has('realized_pl_units'))need(integer(v.realized_pl_units,true)&&v.realized_pl_units!=='-0'&&BigInt(v.realized_pl_units)%500000n===0n);
  if(has('ended_at'))need(pokerTimestamp(v.ended_at)&&Date.parse(v.ended_at)>=Date.parse(v.started_at as string));
  if(has('end_reason'))need(text(v.end_reason,128));
  if(v.state==='SETTLED'){
    need(optional.every(has)&&v.stack_units==='0'&&v.committed_units==='0'&&v.poker_in_play_units==='0');
    need(BigInt(v.final_cash_out_units as string)-BigInt(v.initial_buy_in_units as string)-BigInt(v.total_top_up_units as string)===BigInt(v.realized_pl_units as string));
  }
  return structuredClone(v) as unknown as PokerSessionSummary;
}
/** One authenticated read, never a key mint, command retry, controller grant or navigation effect. */
export async function readPokerHttpSession(client:ApiClient,scope:PokerReadScope,getCurrent:()=>PokerReadScope|null,sessionID:string):Promise<PokerSessionSummary|undefined> {
  const captured={...scope},current=()=>currentPokerScope(client,captured,getCurrent);
  need(pokerUUID(sessionID)&&current());
  try{
    const raw=await client.request(`/api/v1/poker/sessions/${sessionID}`,'GET',undefined,undefined,current);
    if(!current())return undefined;
    return parsePokerSession(raw,captured.table_id,sessionID);
  }catch(error){
    if(!current())return undefined;
    throw new ApiError('会话与出金状态暂未核对完成，请重新读取。',error instanceof ApiError?error.status:0,'POKER_SESSION_READ_UNAVAILABLE');
  }
}
