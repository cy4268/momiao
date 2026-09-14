import { parsePokerTableView, pokerVersion } from './poker-view';
import type { PokerAuthority, PokerIntentContext, TableView } from './poker-ui-types';

/** Local request fencing only. None of these generation fields is serialized to the service. */
export interface PokerReadScope {
  user_id:string; session_generation:number; request_generation:number;
  table_id:string; viewer_kind:PokerAuthority['viewer_kind'];
}
const readKeys=['user_id','session_generation','request_generation','table_id','viewer_kind'] as const;
/** Retain only durable fences across a same-session reconnect, never old cards or balances. */
export interface PokerVersionFence {
  table_id:string;table_version:string;viewer:{session_id?:string;control_epoch:string};
  hand?:{hand_id:string;hand_version:string;action_sequence:string};
}
export function pokerVersionFence(table:TableView):PokerVersionFence {
  return {table_id:table.table_id,table_version:table.table_version,viewer:{session_id:table.viewer.session_id,control_epoch:table.viewer.control_epoch},...(table.hand?{hand:{hand_id:table.hand.hand_id,hand_version:table.hand.hand_version,action_sequence:table.hand.action_sequence}}:{})};
}
export function acceptPokerSnapshot(raw:unknown,captured:PokerReadScope,current:PokerReadScope|null,previous?:PokerVersionFence):TableView|undefined {
  if(!current||readKeys.some(k=>captured[k]!==current[k]))return undefined;
  if(!/^[1-9][0-9]{0,18}$/.test(current.user_id)||BigInt(current.user_id)>9223372036854775807n||!Number.isSafeInteger(current.session_generation)||current.session_generation<0||!Number.isSafeInteger(current.request_generation)||current.request_generation<0)throw new Error('牌桌读取上下文已失效。');
  const next=parsePokerTableView(raw,current.table_id,current.viewer_kind);
  if(previous?.table_id===next.table_id){
    if(BigInt(next.table_version)<BigInt(previous.table_version))return undefined;
    if(previous.viewer.session_id&&previous.viewer.session_id===next.viewer.session_id&&BigInt(next.viewer.control_epoch)<BigInt(previous.viewer.control_epoch))return undefined;
    if(previous.hand&&next.hand?.hand_id===previous.hand.hand_id&&(BigInt(next.hand.hand_version)<BigInt(previous.hand.hand_version)||BigInt(next.hand.action_sequence)<BigInt(previous.hand.action_sequence)))return undefined;
  }
  return next;
}
const contextKeys=['user_id','runtime_id','event_sequence','table_id','table_version','hand_id','hand_version','session_id','control_epoch','action_sequence'] as const;
function validContext(v:PokerIntentContext):boolean {
  if(!v.user_id||!v.runtime_id||!pokerVersion(v.event_sequence))return false;
  if(v.table_id){if(!pokerVersion(v.table_version)||!pokerVersion(v.control_epoch))return false;}
  else if(v.table_version!==undefined||v.control_epoch!==undefined||v.session_id!==undefined||v.hand_id!==undefined)return false;
  if(v.hand_id){if(!pokerVersion(v.hand_version)||!pokerVersion(v.action_sequence))return false;}
  else if(v.hand_version!==undefined||v.action_sequence!==undefined)return false;
  return true;
}
/** A context match is only a freshness test. Connection/capability/pending guards and stable action IDs remain required. */
export function matchesPokerIntentContext(rendered:PokerIntentContext,current:PokerIntentContext):boolean {
  return validContext(rendered)&&validContext(current)&&contextKeys.every(k=>rendered[k]===current[k]);
}
/** IS §344 envelope numbers stay numbers on the wire; payload strings never pass through this helper. */
export function pokerEnvelopeCounter(value:unknown):string {
  if(typeof value!=='number'||!Number.isSafeInteger(value)||value<0||Object.is(value,-0))throw new Error('牌桌事件计数格式异常，请重新同步。');
  return String(value);
}
