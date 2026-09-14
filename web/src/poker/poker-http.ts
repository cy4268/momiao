import { ApiError, type ApiClient } from '../api';
import { acceptPokerSnapshot, type PokerReadScope, type PokerVersionFence } from './poker-client-boundary';
import { pokerChipAmount, pokerConnectionID, pokerUUID } from './poker-view';
import { parsePokerReceipt, parsePokerReceiptLookup, pokerReceiptQuery, type PokerReceipt, type PokerPendingLocator, type PokerReceiptLookup } from './poker-wire';
import type { PokerAuthority, PokerTicketIntent, TableView } from './poker-ui-types';

const base='/api/v1/poker';
const scopeKeys=['user_id','session_generation','request_generation','table_id','viewer_kind'] as const;
type CurrentScope=()=>PokerReadScope|null;
const invalid=()=>new ApiError('牌桌请求上下文或输入已失效。',0,'POKER_INVALID_CLIENT_REQUEST');
const key=(v:unknown):v is string=>typeof v==='string'&&/^[\x21-\x7e]{16,128}$/.test(v);

export function capturePokerReadScope(client:ApiClient,tableID:string,viewerKind:PokerAuthority['viewer_kind'],requestGeneration:number):PokerReadScope {
  const auth=client.getSnapshot(),sessionGeneration=client.getSessionGeneration();
  if(!auth.ready||auth.loggingOut||!auth.user||!Number.isSafeInteger(auth.user.id)||auth.user.id<=0||!pokerUUID(tableID)||!['PLAYER_SELF','SPECTATOR','HOST','OPS'].includes(viewerKind)||[requestGeneration,sessionGeneration].some(n=>!Number.isSafeInteger(n)||n<0))throw invalid();
  return {user_id:String(auth.user.id),session_generation:sessionGeneration,request_generation:requestGeneration,table_id:tableID,viewer_kind:viewerKind};
}
export function currentPokerScope(client:ApiClient,captured:PokerReadScope,getCurrent:CurrentScope):boolean {
  const view=getCurrent();if(!view||scopeKeys.some(k=>view[k]!==captured[k]))return false;
  try{const auth=capturePokerReadScope(client,captured.table_id,captured.viewer_kind,captured.request_generation);return scopeKeys.every(k=>auth[k]===captured[k]);}catch{return false;}
}
const current=currentPokerScope;
/** Owner-scoped read-only POST. Lookup key is never a URL/query-string or a replacement mutation ID. */
export async function readPokerHttpReceipt(client:ApiClient,scope:PokerReadScope,getCurrent:CurrentScope,locator:PokerPendingLocator):Promise<PokerReceiptLookup|undefined> {
  const captured={...scope},original={...locator};if(!current(client,captured,getCurrent))throw invalid();const body=pokerReceiptQuery(original);
  try{
    const raw=await client.request(`${base}/tables/${captured.table_id}/receipt-query`,'POST',body,undefined,()=>current(client,captured,getCurrent));
    if(!current(client,captured,getCurrent))return undefined;return parsePokerReceiptLookup(raw,captured.table_id,original);
  }catch(error){if(!current(client,captured,getCurrent))return undefined;throw new ApiError('原操作回执暂未核对完成，操作结果仍待确认。',error instanceof ApiError?error.status:0,'POKER_RECEIPT_QUERY_UNAVAILABLE');}
}
/** Shape only: never decode signed claims into client identity/permissions or treat this as signature/TTL verification. */
export function parsePokerConnectTicket(raw:unknown):string {
  if(!raw||typeof raw!=='object'||Array.isArray(raw)||Object.keys(raw).length!==1||!Object.hasOwn(raw,'poker_connect_ticket'))throw invalid();
  const ticket=(raw as {poker_connect_ticket:unknown}).poker_connect_ticket;
  if(typeof ticket!=='string'||ticket.length>8192||!/^ct1\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/.test(ticket))throw invalid();
  const alphabet='ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_';
  for(const part of ticket.split('.').slice(1)){const rem=part.length%4;if(rem===1||rem!==0&&alphabet.indexOf(part.at(-1)!)%(rem===2?16:4)!==0)throw invalid();}
  return ticket;
}
/** Table-bound, one-shot mint. The returned ticket belongs only in the current pending socket's ephemeral memory. */
export async function mintPokerConnectTicket(client:ApiClient,scope:PokerReadScope,getCurrent:CurrentScope,intent:PokerTicketIntent):Promise<string|undefined> {
  const captured={...scope};if(!current(client,captured,getCurrent)||!['CLAIM_CONTROL','READ_ONLY'].includes(intent))throw invalid();
  try{
    const raw=await client.request(`${base}/connect-tickets`,'POST',{target_table_id:captured.table_id,control_intent:intent},undefined,()=>current(client,captured,getCurrent));
    if(!current(client,captured,getCurrent))return undefined;return parsePokerConnectTicket(raw);
  }catch(error){
    if(!current(client,captured,getCurrent))return undefined;
    const code=error instanceof ApiError&&error.status===0&&error.code===''?'POKER_TICKET_NETWORK_UNAVAILABLE':error instanceof ApiError&&/^[A-Z0-9_]{1,80}$/.test(error.code)?error.code:'POKER_TICKET_UNAVAILABLE';
    throw new ApiError('连接票据未就绪。',error instanceof ApiError?error.status:0,code);
  }
}
/** Real ApiClient request path. Late or logged-out replies are ignored before viewer decoding. */
export async function readPokerHttpTable(client:ApiClient,scope:PokerReadScope,getCurrent:CurrentScope,previous?:PokerVersionFence):Promise<TableView|undefined> {
  const captured={...scope};if(!current(client,captured,getCurrent))throw invalid();
  let raw:unknown;try{raw=await client.request(`${base}/tables/${captured.table_id}`,'GET',undefined,undefined,()=>current(client,captured,getCurrent));}catch(error){if(!current(client,captured,getCurrent))return undefined;throw error;}
  if(!current(client,captured,getCurrent))return undefined;
  const table=acceptPokerSnapshot(raw,captured,getCurrent(),previous);
  if(table&&(table.viewer.control!==undefined||table.viewer.can_act||table.viewer.legal!==undefined))throw new ApiError('HTTP 牌桌读取仅提供只读状态，请等待当前连接的权威同步。',0,'POKER_INVALID_HTTP_PROJECTION');
  return table;
}

export type PokerHttpCommand=
  | {type:'reserve';request_id:string;seat_no:number}
  | {type:'buyin';request_id:string;reservation_id:string;amount_units:string}
  | {type:'topup';request_id:string;session_id:string;amount_units:string}
  | {type:'leave';request_id:string;session_id:string}
  | {type:'takeover';request_id:string;session_id:string;connection_id:string};
function bindCommand(command:PokerHttpCommand,tableID:string):{path:string;body:Record<string,unknown>;session?:string} {
  if(!key(command.request_id))throw invalid();
  const body:Record<string,unknown>={request_id:command.request_id};
  if(command.type==='reserve'){
    if(!Number.isInteger(command.seat_no)||command.seat_no<1||command.seat_no>9)throw invalid();
    return {path:`${base}/tables/${tableID}/seat-reservations`,body:{...body,seat_no:command.seat_no}};
  }
  if(command.type==='buyin'||command.type==='topup'){
    if(!pokerChipAmount(command.amount_units)||BigInt(command.amount_units)<=0n)throw invalid();body.amount_units=command.amount_units;
  }
  if(command.type==='buyin'){
    if(!pokerUUID(command.reservation_id))throw invalid();return {path:`${base}/tables/${tableID}/buy-ins`,body:{...body,reservation_id:command.reservation_id}};
  }
  if(command.type!=='topup'&&command.type!=='leave'&&command.type!=='takeover')throw invalid();
  if(!pokerUUID(command.session_id))throw invalid();
  if(command.type==='takeover'){if(!pokerConnectionID(command.connection_id))throw invalid();body.connection_id=command.connection_id;}
  return {path:`${base}/sessions/${command.session_id}/${command.type==='topup'?'top-ups':command.type==='takeover'?'take-over':'safe-leave'}`,body,session:command.session_id};
}
/** Caller owns the current capability and synchronous pending latch. This sends once; it never retries or treats an error as a no-effect receipt. */
export async function submitPokerHttpCommand(client:ApiClient,command:PokerHttpCommand,scope:PokerReadScope,getCurrent:CurrentScope):Promise<PokerReceipt|undefined> {
  const captured={...scope};if(!current(client,captured,getCurrent))throw invalid();const binding=bindCommand(command,captured.table_id);
  try{
    const raw=await client.request(binding.path,'POST',binding.body,undefined,()=>current(client,captured,getCurrent));if(!current(client,captured,getCurrent))return undefined;
    const receipt=parsePokerReceipt(raw,captured.table_id);
    if(binding.session&&receipt.session_id&&receipt.session_id!==binding.session)throw invalid();
    return receipt;
  }catch(error){
    if(!current(client,captured,getCurrent))return undefined;
    const status=error instanceof ApiError?error.status:0,code=error instanceof ApiError&&/^[A-Z0-9_]{1,80}$/.test(error.code)?error.code:'POKER_OUTCOME_UNCONFIRMED';
    throw new ApiError('牌桌操作结果尚未确认，请核对原请求回执，勿重复提交。',status,code,true);
  }
}
