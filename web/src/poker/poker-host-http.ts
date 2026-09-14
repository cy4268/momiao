import { ApiError, type ApiClient } from '../api';
import { currentPokerScope } from './poker-http';
import type { PokerReadScope } from './poker-client-boundary';
import type { PokerHostCommand } from './poker-ui-types';
import { parsePokerReceipt, type PokerReceipt } from './poker-wire';
import { pokerUUID } from './poker-view';

export type PokerHostHttpCommand=PokerHostCommand&{request_id:string};
export type PokerHostReceiptLookup={table_id:string;kind:'host';mutation_id:string}&({state:'FOUND';receipt:PokerReceipt}|{state:'NOT_FOUND'});
type CurrentScope=()=>PokerReadScope|null;
type Value=Record<string,unknown>;
const base='/api/v1/poker';
const invalid=()=>new ApiError('房主管理请求上下文或输入已失效。',0,'POKER_INVALID_HOST_COMMAND');
const key=(v:unknown):v is string=>typeof v==='string'&&/^[\x21-\x7e]{16,128}$/.test(v);
const user=(v:unknown):v is string=>typeof v==='string'&&/^[1-9][0-9]{0,18}$/.test(v)&&BigInt(v)<=9223372036854775807n;
function need(ok:unknown):asserts ok {if(!ok)throw invalid();}
function object(raw:unknown,required:string,optional=''):Value {
  need(raw!==null&&typeof raw==='object'&&!Array.isArray(raw)&&(Object.getPrototypeOf(raw)===Object.prototype||Object.getPrototypeOf(raw)===null));
  const value=raw as Value,keys=required.split(' '),allowed=new Set([...keys,...optional.split(' ')]);
  need(keys.every(k=>Object.hasOwn(value,k))&&Object.keys(value).every(k=>allowed.has(k)));return value;
}
function bind(command:PokerHostHttpCommand,table:string):{path:string;body:Value} {
  need(pokerUUID(table));const value=object(command,'request_id command','target_session_id target_user_id');need(key(value.request_id));
  if(['PAUSE_ACCEPTING_PLAYERS','RESUME_ACCEPTING_PLAYERS','CLOSE_TABLE'].includes(value.command as string))object(value,'request_id command');
  else if(value.command==='REMOVE_PLAYER_AFTER_HAND')need(object(value,'request_id command target_session_id target_user_id')&&pokerUUID(value.target_session_id)&&user(value.target_user_id));
  else if(value.command==='REMOVE_SPECTATOR')need(object(value,'request_id command target_user_id')&&user(value.target_user_id));
  else if(value.command==='MUTE_CHAT_USER'){
    object(value,'request_id command target_user_id','target_session_id');need(user(value.target_user_id));
    if(Object.hasOwn(value,'target_session_id'))need(pokerUUID(value.target_session_id));
  }else throw invalid();
  return{path:`${base}/tables/${table}/commands`,body:structuredClone(value)};
}

/** One send with one durable request id. Callers retain the original command for explicit recovery. */
export async function submitPokerHostCommand(client:ApiClient,scope:PokerReadScope,getCurrent:CurrentScope,command:PokerHostHttpCommand,canDispatch?:()=>boolean):Promise<PokerReceipt|undefined>{
  const captured={...scope},original=structuredClone(command),current=()=>currentPokerScope(client,captured,getCurrent);need(current());const binding=bind(original,captured.table_id);
  try{
    const raw=await client.request(binding.path,'POST',binding.body,undefined,()=>current()&&(canDispatch?.()??true));if(!current())return undefined;
    return parsePokerReceipt(raw,captured.table_id);
  }catch(error){
    if(!current())return undefined;
    const status=error instanceof ApiError?error.status:0,code=error instanceof ApiError&&/^[A-Z0-9_]{1,80}$/.test(error.code)?error.code:'POKER_HOST_OUTCOME_UNCONFIRMED';
    throw new ApiError('房主管理结果尚未确认，请查询原回执。',status,code,true);
  }
}

/** Read only. NOT_FOUND never proves the command had no effect. */
export async function readPokerHostReceipt(client:ApiClient,scope:PokerReadScope,getCurrent:CurrentScope,command:PokerHostHttpCommand):Promise<PokerHostReceiptLookup|undefined>{
  const captured={...scope},original=structuredClone(command),current=()=>currentPokerScope(client,captured,getCurrent);need(current());bind(original,captured.table_id);
  try{
    const raw=await client.request(`${base}/tables/${captured.table_id}/receipt-query`,'POST',{kind:'host',mutation_id:original.request_id},undefined,current);if(!current())return undefined;
    const value=object(raw,'table_id kind mutation_id state','receipt');need(value.table_id===captured.table_id&&value.kind==='host'&&value.mutation_id===original.request_id&&(value.state==='FOUND'||value.state==='NOT_FOUND'));
    if(value.state==='NOT_FOUND'){need(!Object.hasOwn(value,'receipt'));return{table_id:captured.table_id,kind:'host',mutation_id:original.request_id,state:'NOT_FOUND'};}
    need(Object.hasOwn(value,'receipt'));return{table_id:captured.table_id,kind:'host',mutation_id:original.request_id,state:'FOUND',receipt:parsePokerReceipt(value.receipt,captured.table_id)};
  }catch(error){
    if(!current())return undefined;
    throw new ApiError('原房主管理回执暂未核实。',error instanceof ApiError?error.status:0,'POKER_HOST_RECEIPT_UNAVAILABLE');
  }
}
