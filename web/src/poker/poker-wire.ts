import { pokerEnvelopeCounter } from './poker-client-boundary';
import { parsePokerControl, parsePokerTableView, pokerChipAmount, pokerConnectionID, pokerTimestamp, pokerUUID, pokerVersion } from './poker-view';
import type { ActionType, PokerAuthority, PokerConnectionControl, TableView } from './poker-ui-types';

export const POKER_SUBPROTOCOL='chaldea-poker.v1';
export const POKER_WS_PATH='/ws/poker';
export const POKER_MAX_FRAME_BYTES=65536;
type ObjectValue=Record<string,unknown>;
const fail=()=>new Error('牌桌消息格式异常，请重新同步。');
function need(ok:unknown):asserts ok {if(!ok)throw fail();}
const own=(v:ObjectValue,k:string)=>Object.hasOwn(v,k);
function object(v:unknown,required:string,optional=''):ObjectValue {
  need(v!==null&&typeof v==='object'&&!Array.isArray(v)&&(Object.getPrototypeOf(v)===Object.prototype||Object.getPrototypeOf(v)===null));
  const o=v as ObjectValue,keys=required.split(' '),allowed=new Set([...keys,...optional.split(' ')]);
  need(keys.every(k=>own(o,k))&&Object.keys(o).every(k=>allowed.has(k)));return o;
}
const text=(v:unknown,max:number):v is string=>typeof v==='string'&&v.length>0&&new TextEncoder().encode(v).length<=max;
const key=(v:unknown):v is string=>typeof v==='string'&&/^[\x21-\x7e]{16,128}$/.test(v);
const code=(v:unknown):v is string=>typeof v==='string'&&/^[A-Z0-9_]{1,80}$/.test(v);
function wireNumber(v:unknown):number {need(pokerVersion(v)&&BigInt(v)<=BigInt(Number.MAX_SAFE_INTEGER));return Number(v);}

/** G3 domain acknowledgement only; never interpreted as a replacement TableView. */
export interface PokerReceipt {
  table_id:string;status:string;table_version:string;duplicate:boolean;
  session_id?:string;hand_id?:string;reservation_id?:string;funding_operation_id?:string;
  amount_units?:string;failure_code?:string;control_epoch?:string;chat_sequence?:string;
}
export type PokerReceiptKind='action'|'sitout'|'resume'|'nextseed'|'topup'|'leave'|'takeover';
export interface PokerPendingLocator {
  kind:PokerReceiptKind;request_id:string;action_id:string|null;target_session_id:string;target_hand_id?:string;
}
export type PokerReceiptLookup={table_id:string;kind:PokerReceiptKind;mutation_id:string}&({state:'FOUND';receipt:PokerReceipt}|{state:'NOT_FOUND'});
/** Only the original lookup identity crosses HTTP; no command/seed/controller payload. */
export function pokerReceiptQuery(p:PokerPendingLocator):{kind:PokerReceiptKind;mutation_id:string} {
  need(['action','sitout','resume','nextseed','topup','leave','takeover'].includes(p.kind)&&key(p.request_id)&&pokerUUID(p.target_session_id));
  const wire=['action','sitout','resume','nextseed'].includes(p.kind);need(wire?key(p.action_id):p.action_id===null);
  need(p.kind==='action'?pokerUUID(p.target_hand_id):p.target_hand_id===undefined);
  return {kind:p.kind,mutation_id:p.action_id??p.request_id};
}
export function samePokerPendingLocator(a:PokerPendingLocator|undefined,b:PokerPendingLocator):boolean {
  return !!a&&(['kind','request_id','action_id','target_session_id','target_hand_id'] as const).every(k=>a[k]===b[k]);
}
export function parsePokerReceiptLookup(raw:unknown,tableID:string,locator:PokerPendingLocator):PokerReceiptLookup {
  const expected=pokerReceiptQuery(locator),r=object(raw,'table_id kind mutation_id state','receipt');
  need(pokerUUID(tableID)&&r.table_id===tableID&&r.kind===expected.kind&&r.mutation_id===expected.mutation_id&&(r.state==='FOUND'||r.state==='NOT_FOUND'));
  if(r.state==='NOT_FOUND'){need(!own(r,'receipt'));return{table_id:tableID,...expected,state:'NOT_FOUND'};}
  need(own(r,'receipt'));const receipt=parsePokerReceipt(r.receipt,tableID);
  need(locator.kind==='action'?receipt.hand_id===locator.target_hand_id:receipt.session_id===locator.target_session_id);
  return {table_id:tableID,...expected,state:'FOUND',receipt};
}
export function parsePokerReceipt(raw:unknown,tableID:string):PokerReceipt {
  const r=object(raw,'table_id status table_version duplicate','session_id hand_id reservation_id funding_operation_id amount_units failure_code control_epoch chat_sequence');
  need(pokerUUID(tableID)&&r.table_id===tableID&&code(r.status)&&pokerVersion(r.table_version)&&typeof r.duplicate==='boolean');
  for(const k of ['session_id','hand_id','reservation_id','funding_operation_id'])if(own(r,k))need(pokerUUID(r[k]));
  if(own(r,'amount_units'))need(pokerChipAmount(r.amount_units));
  if(own(r,'failure_code'))need(code(r.failure_code));
  if(own(r,'control_epoch'))need(pokerVersion(r.control_epoch));
  if(own(r,'chat_sequence'))need(pokerVersion(r.chat_sequence)&&BigInt(r.chat_sequence)>0n);
  return structuredClone(r) as unknown as PokerReceipt;
}
interface EventMeta {
  event_id:string;event_seq:string;table_id:string;table_version:string;
  hand_id:string|null;hand_version:string|null;server_time:string;
}
export type PokerServerEvent=EventMeta&(
  | {type:'table.snapshot';payload:TableView}
  | {type:'auth.accepted';payload:{request_id:string;connection_id:string}}
  | {type:'pong';payload:{request_id:string}}
  | {type:'control.changed';payload:PokerConnectionControl}
  | {type:'error';payload:{request_id:string;code:string}}
  | {type:'service.notice';payload:{request_id:string;action_id:string|null;receipt:PokerReceipt}}
);
/** Envelope counters are canonicalized only after verifying the frozen safe-number wire. */
export function parsePokerServer(raw:unknown,tableID:string,viewer:PokerAuthority['viewer_kind']):PokerServerEvent {
  const e=object(raw,'type event_id event_seq table_id table_version hand_id hand_version server_time payload');
  need(pokerUUID(tableID)&&e.table_id===tableID&&text(e.event_id,256)&&pokerTimestamp(e.server_time));
  const event_seq=pokerEnvelopeCounter(e.event_seq),table_version=pokerEnvelopeCounter(e.table_version);
  need(e.hand_id===null?e.hand_version===null:pokerUUID(e.hand_id)&&e.hand_version!==null);
  const hand_version=e.hand_version===null?null:pokerEnvelopeCounter(e.hand_version);
  const meta:EventMeta={event_id:e.event_id,event_seq,table_id:tableID,table_version,hand_id:e.hand_id as string|null,hand_version,server_time:e.server_time};
  if(e.type==='table.snapshot'){
    const payload=parsePokerTableView(e.payload,tableID,viewer);
    need(payload.viewer.control&&payload.table_version===table_version&&(payload.hand?.hand_id??null)===meta.hand_id&&(payload.hand?.hand_version??null)===hand_version&&payload.server_now===meta.server_time);
    if(payload.viewer.control.mode==='READ_ONLY')need(!payload.viewer.can_act&&payload.viewer.legal===undefined);
    return {...meta,type:e.type,payload};
  }
  if(e.type==='control.changed')return {...meta,type:e.type,payload:parsePokerControl(e.payload)};
  need(e.type==='auth.accepted'||e.type==='pong'||e.type==='error'||e.type==='service.notice');
  const p=object(e.payload,e.type==='error'?'request_id code':e.type==='service.notice'?'request_id action_id receipt':e.type==='auth.accepted'?'request_id connection_id':'request_id');need(key(p.request_id));
  if(e.type==='auth.accepted'){need(pokerConnectionID(p.connection_id));return {...meta,type:e.type,payload:{request_id:p.request_id,connection_id:p.connection_id}};}
  if(e.type==='service.notice'){
    need(p.action_id===null||key(p.action_id));
    return {...meta,type:e.type,payload:{request_id:p.request_id,action_id:p.action_id,receipt:parsePokerReceipt(p.receipt,tableID)}};
  }
  if(e.type==='error'){need(code(p.code));return {...meta,type:e.type,payload:{request_id:p.request_id,code:p.code}};}
  return {...meta,type:e.type,payload:{request_id:p.request_id}};
}
export function parsePokerServerText(raw:string,tableID:string,viewer:PokerAuthority['viewer_kind']):PokerServerEvent {
  need(typeof raw==='string'&&new TextEncoder().encode(raw).length<=POKER_MAX_FRAME_BYTES);
  let value:unknown;try{value=JSON.parse(raw);}catch{throw fail();}
  // JSON.parse checks grammar, but erases duplicate keys and can round number
  // lexemes. Walk the original tokens before accepting its result. Each object
  // owns a key set; decoded names also catch escaped aliases of the same key.
  const stack:{keys?:Set<string>;expectKey:boolean}[]=[];
  for(const token of raw.matchAll(/"(?:[^"\\]|\\.)*"|[{}\[\],]|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g)){
    const t=token[0],parent=stack.at(-1);
    if(t==='{'||t==='['){stack.push({keys:t==='{'?new Set():undefined,expectKey:t==='{'});need(stack.length<=32);}
    else if(t==='}'||t===']')stack.pop();
    else if(t===','){if(parent?.keys)parent.expectKey=true;}
    else if(t[0]==='"'&&parent?.keys&&parent.expectKey){const name=JSON.parse(t) as string;need(!parent.keys.has(name));parent.keys.add(name);parent.expectKey=false;}
    else if(token[1]!==undefined)need(/^(0|[1-9][0-9]*)$/.test(token[1])&&token[1].length<=16&&BigInt(token[1])<=BigInt(Number.MAX_SAFE_INTEGER));
  }
  return parsePokerServer(value,tableID,viewer);
}

export interface PokerCommandRefs {
  request_id:string;table_id:string;action_id?:string;hand_id?:string;
  table_version?:string;hand_version?:string;control_epoch?:string;
}
export type PokerClientCommand=
  | {type:'auth.connect';poker_connect_ticket:string}
  | {type:'sync.request'|'ping'|'session.sit_out_next_hand'|'session.resume_play'}
  | {type:'hand.action';action_type:ActionType;target_to_units:string}
  | {type:'client_seed.set_next';client_seed:string}
  | {type:'chat.send';message:string};

/** Only renders explicit protocol slots. Identity, viewer kind and local generations never leave this boundary. */
export function encodePokerClient(command:PokerClientCommand,refs:PokerCommandRefs):string {
  need(key(refs.request_id)&&pokerUUID(refs.table_id));
  const e={type:command.type,request_id:refs.request_id,table_id:refs.table_id,hand_id:null as string|null,expected_table_version:0,expected_hand_version:0,control_epoch:0,action_id:null as string|null,payload:{} as ObjectValue};
  if(command.type==='auth.connect'){
    need(text(command.poker_connect_ticket,8192));e.payload={poker_connect_ticket:command.poker_connect_ticket};
  }else if(command.type==='sync.request'||command.type==='ping'){
    // These are fresh reads/heartbeat, not optimistic mutations.
  }else if(command.type==='chat.send'){
    need(text(command.message,2048)&&command.message.trim().length>0);e.payload={message:command.message};
  }else{
    need(key(refs.action_id));e.action_id=refs.action_id;
    e.expected_table_version=wireNumber(refs.table_version);e.control_epoch=wireNumber(refs.control_epoch);
    if(refs.hand_id!==undefined){need(pokerUUID(refs.hand_id));e.hand_id=refs.hand_id;e.expected_hand_version=wireNumber(refs.hand_version);}
    else need(refs.hand_version===undefined);
    if(command.type==='hand.action'){
      need(e.hand_id!==null&&['FOLD','CHECK','CALL','BET','RAISE','ALL_IN'].includes(command.action_type));
      const targeted=command.action_type==='BET'||command.action_type==='RAISE';
      need(targeted?pokerChipAmount(command.target_to_units)&&BigInt(command.target_to_units)>0n:command.target_to_units==='0');
      e.payload={action_type:command.action_type,requested_to_units:targeted?command.target_to_units:null};
    }else if(command.type==='client_seed.set_next'){
      need(text(command.client_seed,256));e.payload={client_seed:command.client_seed};
    }else need(command.type==='session.sit_out_next_hand'||command.type==='session.resume_play');
  }
  const serialized=JSON.stringify(e);need(new TextEncoder().encode(serialized).length<=POKER_MAX_FRAME_BYTES);return serialized;
}
