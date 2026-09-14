import { acceptPokerSnapshot, matchesPokerIntentContext, pokerVersionFence, type PokerReadScope, type PokerVersionFence } from './poker-client-boundary';
import { pokerUUID } from './poker-view';
import { parsePokerServerText, pokerReceiptQuery, type PokerReceipt, type PokerPendingLocator, type PokerReceiptKind } from './poker-wire';
import type { ConnectionState, PokerAuthority, PokerConnectionControl, PokerControlRecovery, PokerIntentContext, PokerTicketIntent, TableView } from './poker-ui-types';

interface Pending extends PokerPendingLocator {
  phase:'SENT'|'ACKNOWLEDGED'|'UNKNOWN';receipt?:PokerReceipt;
}
export interface PokerChatPending {
  request_id:string;phase:'SENT'|'ACKNOWLEDGED'|'UNKNOWN';receipt?:PokerReceipt;querying?:boolean;can_retry?:boolean;
}
export interface PokerStream {
  scope:PokerReadScope;auth_request_id:string;connection_state:ConnectionState;
  ticket_intent:PokerTicketIntent;awaiting_control?:boolean;
  table?:TableView;event_sequence:string;snapshot_generation:number;
  connection_id?:string;control?:PokerConnectionControl;control_generation:number;
  control_fence?:Pick<PokerConnectionControl,'session_id'|'control_epoch'>;
  pending?:Pending;chat_pending?:PokerChatPending;last_receipt?:PokerReceipt;last_error?:string;last_chat_error?:string;durable_fence?:PokerVersionFence;
  last_pong_request_id?:string;
  receipt_querying?:boolean;
  can_retry_pending?:boolean;
  can_recover_control?:boolean;control_recovery?:PokerControlRecovery;
  control_history?:{count:number;querying:boolean};
}
const scopeKeys=['user_id','session_generation','request_generation','table_id','viewer_kind'] as const;
const sameScope=(a:PokerReadScope,b:PokerReadScope)=>scopeKeys.every(k=>a[k]===b[k]);
const sameSession=(a:PokerReadScope,b:PokerReadScope)=>scopeKeys.filter(k=>k!=='request_generation').every(k=>a[k]===b[k]);
const key=(v:unknown):v is string=>typeof v==='string'&&/^[\x21-\x7e]{16,128}$/.test(v);
const fail=()=>new Error('牌桌操作上下文已失效，请重新同步。');

/** Call only after a fresh socket opens, with a generation never reused in this authenticated session. */
export function openPokerStream(scope:PokerReadScope,authRequestID:string,intent:PokerTicketIntent,previous?:PokerStream):PokerStream {
  if(!['CLAIM_CONTROL','READ_ONLY'].includes(intent))throw fail();
  if(!/^[1-9][0-9]{0,18}$/.test(scope.user_id)||BigInt(scope.user_id)>9223372036854775807n||!pokerUUID(scope.table_id)||!['PLAYER_SELF','SPECTATOR','HOST','OPS'].includes(scope.viewer_kind)||!key(authRequestID)||[scope.session_generation,scope.request_generation].some(n=>!Number.isSafeInteger(n)||n<0))throw fail();
  if(previous&&sameSession(previous.scope,scope)&&scope.request_generation<=previous.scope.request_generation)throw fail();
  return {scope:{...scope},auth_request_id:authRequestID,ticket_intent:intent,connection_state:'AUTH_PENDING',event_sequence:'0',snapshot_generation:0,control_generation:0,
    ...(previous&&sameSession(previous.scope,scope)?{pending:uncertain(previous.pending),chat_pending:uncertainChat(previous.chat_pending),durable_fence:previous.durable_fence,control_fence:previous.control_fence}:{})};
}
function uncertain(pending?:Pending):Pending|undefined {return pending?{...pending,phase:pending.receipt?'ACKNOWLEDGED':'UNKNOWN'}:undefined;}
function uncertainChat(pending?:PokerChatPending):PokerChatPending|undefined {return pending?{...pending,phase:pending.receipt?'ACKNOWLEDGED':'UNKNOWN',querying:false,can_retry:false}:undefined;}
export function disconnectPokerStream(state:PokerStream):PokerStream {
  return {...state,connection_state:'DISCONNECTED',pending:uncertain(state.pending),chat_pending:uncertainChat(state.chat_pending)};
}
function degraded(state:PokerStream):PokerStream {return {...state,connection_state:'DEGRADED',last_error:'POKER_PROTOCOL_INVALID',pending:uncertain(state.pending),chat_pending:uncertainChat(state.chat_pending)};}
export function pokerStreamContext(state:PokerStream):PokerIntentContext {
  const t=state.table;
  return {user_id:state.scope.user_id,runtime_id:`${state.scope.session_generation}:${state.scope.request_generation}:${state.snapshot_generation}:${state.control_generation}`,event_sequence:state.event_sequence,...(t?{
    table_id:t.table_id,table_version:t.table_version,hand_id:t.hand?.hand_id,hand_version:t.hand?.hand_version,
    session_id:t.viewer.session_id,control_epoch:t.viewer.control_epoch,action_sequence:t.hand?.action_sequence,
  }:{})};
}
const sameControl=(a:PokerConnectionControl,b:PokerConnectionControl)=>a.connection_id===b.connection_id&&a.session_id===b.session_id&&a.mode===b.mode&&a.control_epoch===b.control_epoch;
function controlRegresses(state:PokerStream,next:PokerConnectionControl):boolean {
  return !!next.session_id&&[state.control_fence,state.durable_fence?.viewer].some(old=>!!old&&old.session_id===next.session_id&&BigInt(old.control_epoch)>BigInt(next.control_epoch));
}
function controlFence(state:PokerStream,next:PokerConnectionControl){return next.session_id?{session_id:next.session_id,control_epoch:next.control_epoch}:state.control_fence;}
/** A verified live stream and can_act are insufficient: this exact socket must own the matching full-snapshot session/epoch. */
export function pokerStreamAuthority(state:PokerStream):PokerAuthority {
  const v=state.table?.viewer,c=state.control,context=pokerStreamContext(state);
  const bound=state.connection_state==='LIVE'&&!!v&&!!c&&c.connection_id===state.connection_id&&!!c.session_id&&c.session_id===v.session_id&&c.control_epoch===v.control_epoch;
  const can_control=!!(state.ticket_intent==='CLAIM_CONTROL'&&bound&&c?.mode==='CONTROLLER'&&v?.control&&sameControl(c,v.control));
  return {user_id:state.scope.user_id,runtime_id:context.runtime_id,event_sequence:state.event_sequence,connection_state:state.connection_state,has_snapshot:!!state.table,pending:!!state.pending,viewer_kind:state.scope.viewer_kind,can_control,
    ticket_intent:state.ticket_intent,chat_recovery:state.chat_pending?{phase:state.chat_pending.phase,querying:state.chat_pending.querying===true,can_retry:state.chat_pending.can_retry===true}:undefined,can_retry_pending:state.can_retry_pending===true,can_recover_control:state.can_recover_control,control_recovery:state.control_recovery,control_history:state.control_history,can_reconnect:state.connection_state==='DISCONNECTED'||state.connection_state==='DEGRADED',can_takeover:!!(state.ticket_intent==='CLAIM_CONTROL'&&bound&&c?.mode==='READ_ONLY')};
}
/** Synchronous pending latch only, not authorization. The real caller must first verify socket control and the current intent capability. Assign this state BEFORE calling send/request. */
export function beginPokerPending(state:PokerStream,rendered:PokerIntentContext,requestID:string,actionID:string|null,kind:PokerReceiptKind):PokerStream {
  if(state.connection_state!=='LIVE'||!state.table||state.pending||!key(requestID)||(actionID!==null&&!key(actionID))||!matchesPokerIntentContext(rendered,pokerStreamContext(state)))throw fail();
  const locator:PokerPendingLocator={kind,request_id:requestID,action_id:actionID,target_session_id:state.table.viewer.session_id!,...(kind==='action'?{target_hand_id:state.table.hand?.hand_id}:{})};pokerReceiptQuery(locator);
  return {...state,pending:{...locator,phase:'SENT'},last_error:undefined,last_receipt:undefined};
}
function settledPending(pending:Pending|undefined,table?:TableView):Pending|undefined {
  return pending?.receipt&&table&&BigInt(table.table_version)>=BigInt(pending.receipt.table_version)?undefined:pending;
}
/** Receipts already decoded by the HTTP/WS boundary; no optimistic amount or controller mutation. */
export function acknowledgePokerPending(state:PokerStream,requestID:string,actionID:string|null,receipt:PokerReceipt):PokerStream {
  const pending=state.pending;if(!pending||pending.request_id!==requestID)return state;
  if(pending.action_id!==actionID||receipt.table_id!==state.scope.table_id)return degraded(state);
  if(receipt.status==='FAILED_NO_EFFECT')return {...state,pending:undefined,last_receipt:receipt,last_error:receipt.failure_code??'FAILED_NO_EFFECT'};
  return {...state,last_receipt:receipt,pending:settledPending({...pending,phase:'ACKNOWLEDGED',receipt},state.table)};
}

/** No effects, timers, retries, optimistic table mutation or event-id parsing. Late scopes are rejected BEFORE JSON/private payload decoding. */
export function receivePokerStream(state:PokerStream,captured:PokerReadScope,text:string):PokerStream {
  if(!sameScope(state.scope,captured)||!['AUTH_PENDING','SYNCING','LIVE'].includes(state.connection_state))return state;
  try{
    const event=parsePokerServerText(text,state.scope.table_id,state.scope.viewer_kind);
    if(state.connection_state==='AUTH_PENDING'){
      if(event.type!=='auth.accepted'||event.payload.request_id!==state.auth_request_id)return degraded(state);
      return {...state,connection_id:event.payload.connection_id,connection_state:'SYNCING'};
    }
    if(event.type==='auth.accepted')return degraded(state);
    if(event.type==='control.changed'){
      const c=event.payload;
      if(c.connection_id!==state.connection_id||c.session_id&&state.table&&c.session_id!==state.table.viewer.session_id||controlRegresses(state,c))return state;
      if(state.ticket_intent==='READ_ONLY'&&c.mode==='CONTROLLER')return degraded(state);
      if(state.control&&sameControl(c,state.control))return state;
      if(state.control_generation>=Number.MAX_SAFE_INTEGER)return degraded(state);
      return {...state,control:c,awaiting_control:true,control_fence:controlFence(state,c),control_generation:state.control_generation+1};
    }
    if(event.type==='table.snapshot'){
      const c=event.payload.viewer.control!;
      if(c.connection_id!==state.connection_id||controlRegresses(state,c))return state;
      if(state.ticket_intent==='READ_ONLY'&&c.mode==='CONTROLLER')return degraded(state);
      if(state.awaiting_control&&state.control&&!sameControl(c,state.control))return state;
      if(state.control?.mode==='READ_ONLY'&&c.mode==='CONTROLLER'&&state.control.session_id===c.session_id&&BigInt(c.control_epoch)<=BigInt(state.control.control_epoch))return state;
      const table=acceptPokerSnapshot(event.payload,captured,state.scope,state.durable_fence);if(!table)return state;
      if(state.snapshot_generation>=Number.MAX_SAFE_INTEGER)return degraded(state);
      const chatPending=state.chat_pending?.receipt?.chat_sequence&&table.chat&&BigInt(table.chat.last_sequence)>=BigInt(state.chat_pending.receipt.chat_sequence)?undefined:state.chat_pending;
      return {...state,table,control:c,awaiting_control:false,control_fence:controlFence(state,c),durable_fence:pokerVersionFence(table),connection_state:'LIVE',event_sequence:event.event_seq,snapshot_generation:state.snapshot_generation+1,pending:settledPending(state.pending,table),chat_pending:chatPending};
    }
    if(event.type==='pong')return {...state,last_pong_request_id:event.payload.request_id};
    const chat=state.chat_pending;
    if(chat&&event.payload.request_id===chat.request_id){
      if(event.type==='error')return{...state,chat_pending:{...chat,phase:'UNKNOWN',can_retry:false},last_chat_error:event.payload.code};
      if(event.type!=='service.notice'||event.payload.action_id!==null||event.payload.receipt.status!=='CHAT_ACCEPTED'||!event.payload.receipt.chat_sequence)return degraded(state);
      return{...state,chat_pending:{...chat,phase:'ACKNOWLEDGED',receipt:event.payload.receipt,can_retry:false},last_chat_error:undefined};
    }
    const pending=state.pending;if(!pending||event.payload.request_id!==pending.request_id)return state;
    if(event.type==='error'){
      // A timeout/internal error does not prove rollback. Until a domain receipt
      // resolves the outcome, even a later snapshot must not unlock resubmission.
      return {...state,pending:uncertain(pending),last_error:event.payload.code};
    }
    if(event.type!=='service.notice'||event.payload.action_id!==pending.action_id)return degraded(state);
    return acknowledgePokerPending(state,event.payload.request_id,event.payload.action_id,event.payload.receipt);
  }catch{return degraded(state);}
}
