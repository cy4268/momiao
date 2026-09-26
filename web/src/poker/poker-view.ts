import { capSettlementSchema } from '../economy-cap';
import { integer } from '../wallet-api';
import { UNITS_PER_CHIP, type PokerAuthority, type PokerConnectionControl, type TableView } from './poker-ui-types';

type ObjectValue = Record<string, unknown>;
const own = (v:ObjectValue,k:string) => Object.hasOwn(v,k);
const fail = () => new Error('牌桌权威数据格式异常，请重新同步。');
function need(ok:unknown): asserts ok { if(!ok)throw fail(); }
function object(v:unknown,required:string,optional=''):ObjectValue {
  need(v!==null&&typeof v==='object'&&!Array.isArray(v));
  need(Object.getPrototypeOf(v)===Object.prototype||Object.getPrototypeOf(v)===null);
  const result=v as ObjectValue,keys=required.split(' '),allowed=new Set([...keys,...optional.split(' ')]);
  need(keys.every(k=>own(result,k))&&Object.keys(result).every(k=>allowed.has(k)));return result;
}
const bool=(v:unknown)=>typeof v==='boolean';
const text=(v:unknown,max=1024):v is string=>typeof v==='string'&&v.length>0&&new TextEncoder().encode(v).length<=max;
const id=(v:unknown):v is string=>typeof v==='string'&&/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(v);
export const pokerVersion=(v:unknown):v is string=>typeof v==='string'&&/^(0|[1-9][0-9]{0,19})$/.test(v)&&BigInt(v)<=18446744073709551615n;
export const pokerConnectionID=(v:unknown):v is string=>typeof v==='string'&&/^[0-9a-f]{32}$/.test(v);
export function parsePokerControl(raw:unknown):PokerConnectionControl {
  const c=object(raw,'connection_id mode control_epoch','session_id');
  need(pokerConnectionID(c.connection_id)&&(c.mode==='CONTROLLER'||c.mode==='READ_ONLY')&&pokerVersion(c.control_epoch));
  if(own(c,'session_id'))need(id(c.session_id));
  if(c.mode==='CONTROLLER')need(id(c.session_id)&&BigInt(c.control_epoch)>0n);
  return structuredClone(c) as unknown as PokerConnectionControl;
}
const amount=(v:unknown):v is string=>integer(v)&&BigInt(v)%UNITS_PER_CHIP===0n;
export { id as pokerUUID, time as pokerTimestamp, amount as pokerChipAmount };
const range=(v:unknown,min:number,max:number):v is number=>typeof v==='number'&&Number.isInteger(v)&&v>=min&&v<=max;
function time(v:unknown):v is string {
  if(typeof v!=='string'||!/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?(?:Z|[+-]\d\d:\d\d)$/.test(v)||!Number.isFinite(Date.parse(v)))return false;
  const [year,month,day]=v.slice(0,10).split('-').map(Number),d=new Date(0);d.setUTCFullYear(year,month-1,day);
  return d.getUTCFullYear()===year&&d.getUTCMonth()===month-1&&d.getUTCDate()===day&&Number(v.slice(11,13))<24&&Number(v.slice(14,16))<60&&Number(v.slice(17,19))<60;
}
function optional(v:ObjectValue,k:string,check:(value:unknown)=>boolean) { if(own(v,k))need(check(v[k])); }
function numbers(v:unknown,maxLength:number,min:number,max:number):number[] {
  need(Array.isArray(v)&&v.length<=maxLength&&v.every(n=>range(n,min,max))&&new Set(v).size===v.length);return v;
}
function cards(v:unknown,lengths:number[]):number[] { const list=numbers(v,5,0,51);need(lengths.includes(list.length));return list; }
const actions=['FOLD','CHECK','CALL','BET','RAISE','ALL_IN'];
const timelineTypes=['HAND_COMMITTED','POST_SB','POST_BB','FOLD','CHECK','CALL','BET','RAISE','ALL_IN','AUTO_FOLD','AUTO_CHECK','DISCONNECTED','RECONNECTED','RECOVERING','RESUMED','ACTOR_CHANGED','ALL_IN_RUNOUT','DEAL_FLOP','DEAL_TURN','DEAL_RIVER','RETURN_UNCALLED','SHOWDOWN','POT_AWARD','SYSTEM_SETTLEMENT'];
const timelineStreets=['COMMITTED','PREFLOP','FLOP','TURN','RIVER','SETTLED'];
const hostCommands=['PAUSE_ACCEPTING_PLAYERS','RESUME_ACCEPTING_PLAYERS','REMOVE_PLAYER_AFTER_HAND','REMOVE_SPECTATOR','MUTE_CHAT_USER','CLOSE_TABLE'];
const positiveUser=(v:unknown):v is string=>typeof v==='string'&&/^[1-9][0-9]{0,18}$/.test(v)&&BigInt(v)<=9223372036854775807n;
function legal(v:unknown):ObjectValue {
  const l=object(v,'actions to_call_units call_applied_units minimum_bet_units minimum_raise_to_units maximum_raise_to_units raise_rights shortcuts');
  need(Array.isArray(l.actions)&&l.actions.length>0&&l.actions.length<=6&&l.actions.every(a=>typeof a==='string'&&actions.includes(a))&&new Set(l.actions).size===l.actions.length);
  for(const k of ['to_call_units','call_applied_units','minimum_bet_units','minimum_raise_to_units','maximum_raise_to_units'])need(amount(l[k]));
  need(bool(l.raise_rights)&&Array.isArray(l.shortcuts)&&l.shortcuts.length<=5);
  for(const item of l.shortcuts){const s=object(item,'name action_type target_to_units');need(text(s.name,32)&&['BET','RAISE','ALL_IN'].includes(s.action_type as string)&&amount(s.target_to_units));}
  return l;
}

/** G3 TableView only, not a WS envelope or an engine snapshot. No coercion/fallback values. */
export function parsePokerTableView(value:unknown,expectedTableID:string,viewerKind:PokerAuthority['viewer_kind']):TableView {
  need(['PLAYER_SELF','SPECTATOR','HOST','OPS'].includes(viewerKind));
  const t=object(value,'table_id name lifecycle_state table_version max_seats small_blind_units big_blind_units settings_locked server_now seats viewer timeline','intermission_until hand chat host');
  // IS §554 bounds names in graphemes at the domain, not bytes in the viewer.
  need(id(expectedTableID)&&t.table_id===expectedTableID&&typeof t.name==='string'&&t.name.length>0);
  need(['WAITING','IN_HAND','INTERMISSION','PAUSED','RECOVERING','CLOSING','CLOSED'].includes(t.lifecycle_state as string)&&pokerVersion(t.table_version));
  need(range(t.max_seats,2,9)&&amount(t.small_blind_units)&&amount(t.big_blind_units)&&BigInt(t.small_blind_units)>0n&&BigInt(t.big_blind_units)>=BigInt(t.small_blind_units));
  need(bool(t.settings_locked)&&time(t.server_now));optional(t,'intermission_until',time);
  need(Array.isArray(t.seats)&&t.seats.length<=t.max_seats);
  const maxSeats=t.max_seats,seatNumbers=new Set<number>(),visibleCards=new Set<number>();
  let self:ObjectValue|undefined;
  const seats=t.seats.map(item=>{
    const s=object(item,'seat_no display_name state is_self connected stack_units street_committed_units total_committed_units is_folded is_all_in hole_card_count hole_cards_released sit_out_next_hand leave_after_hand pending_top_up_units','hole_cards public_hole_cards rebuy_deadline_at');
    need(range(s.seat_no,1,maxSeats)&&!seatNumbers.has(s.seat_no));seatNumbers.add(s.seat_no);
    need(text(s.display_name)&&['WAITING_ENTRY','ACTIVE','WAITING_BIG_BLIND','SIT_OUT','LEAVE_AFTER_HAND','REBUY_WINDOW','LEFT'].includes(s.state as string));
    for(const k of ['is_self','connected','is_folded','is_all_in','hole_cards_released','sit_out_next_hand','leave_after_hand'])need(bool(s[k]));
    for(const k of ['stack_units','street_committed_units','total_committed_units','pending_top_up_units'])need(amount(s[k]));
    need(s.hole_card_count===0||s.hole_card_count===2);optional(s,'rebuy_deadline_at',time);
    if(s.is_self){need(viewerKind==='PLAYER_SELF'&&!self);self=s;}
    let faces:number[]|undefined;
    if(own(s,'hole_cards')){need(s.is_self&&viewerKind==='PLAYER_SELF'&&s.hole_card_count===2);faces=cards(s.hole_cards,[2]);}
    if(s.is_self&&s.hole_card_count===2)need(faces);
    if(s.hole_cards_released){need(!s.is_folded&&s.hole_card_count===2);const pub=cards(s.public_hole_cards,[2]);if(faces)need(pub.every((n,i)=>n===faces![i]));else faces=pub;}
    else need(!own(s,'public_hole_cards'));
    if(faces)for(const card of faces){need(!visibleCards.has(card));visibleCards.add(card);}
    return s;
  });
  const v=object(t.viewer,'control_epoch can_act can_top_up can_leave can_resume can_sit_out can_start top_up_min_units top_up_max_units','session_id seat_no legal control');
  need(pokerVersion(v.control_epoch));for(const k of ['can_act','can_top_up','can_leave','can_resume','can_sit_out','can_start'])need(bool(v[k]));
  need(amount(v.top_up_min_units)&&amount(v.top_up_max_units));
  if(own(v,'session_id')){need(id(v.session_id)&&range(v.seat_no,1,maxSeats)&&self?.seat_no===v.seat_no&&BigInt(v.control_epoch)>0n);}
  else need(!own(v,'seat_no')&&!self&&v.control_epoch==='0'&&!v.can_act&&!v.can_top_up&&!v.can_leave&&!v.can_resume&&!v.can_sit_out);
  if(own(v,'control')){const c=parsePokerControl(v.control);need(c.control_epoch===v.control_epoch&&c.session_id===v.session_id);}
  if(v.can_top_up)need(BigInt(v.top_up_min_units)>0n&&BigInt(v.top_up_max_units)>=BigInt(v.top_up_min_units));
  let h:ObjectValue|undefined;
  if(own(t,'hand')){
    h=object(t.hand,'hand_id hand_version street button_seat actor_seat board_cards pot_units pots action_sequence recovering server_seed_hash deck_hash','action_deadline_at recovery_until economy_settlement');
    if(own(h,'economy_settlement'))need(capSettlementSchema.safeParse(h.economy_settlement).success);
    need(id(h.hand_id)&&pokerVersion(h.hand_version)&&pokerVersion(h.action_sequence));
    need(['COMMITTED','PREFLOP','FLOP','TURN','RIVER','SETTLED'].includes(h.street as string)&&range(h.button_seat,1,maxSeats)&&range(h.actor_seat,0,maxSeats));
    const board=cards(h.board_cards,h.street==='COMMITTED'||h.street==='PREFLOP'?[0]:h.street==='FLOP'?[3]:h.street==='TURN'?[4]:h.street==='RIVER'?[5]:[0,3,4,5]);
    for(const card of board){need(!visibleCards.has(card));visibleCards.add(card);}
    need(amount(h.pot_units)&&bool(h.recovering));optional(h,'action_deadline_at',time);optional(h,'recovery_until',time);
    for(const k of ['server_seed_hash','deck_hash'])need(typeof h[k]==='string'&&/^[0-9a-f]{64}$/.test(h[k]));
    need(Array.isArray(h.pots)&&h.pots.length<=9);const indices=new Set<number>();
    for(const item of h.pots){const p=object(item,'index amount_units eligible_seats awards');need(range(p.index,0,8)&&!indices.has(p.index)&&amount(p.amount_units));indices.add(p.index);numbers(p.eligible_seats,9,1,maxSeats);need(Array.isArray(p.awards)&&p.awards.length<=9);const winners=new Set<number>();for(const item of p.awards){const a=object(item,'seat_no amount_units');need(range(a.seat_no,1,maxSeats)&&!winners.has(a.seat_no)&&amount(a.amount_units));winners.add(a.seat_no);}}
  }else need(seats.every(s=>s.hole_card_count===0&&!own(s,'hole_cards')&&!own(s,'public_hole_cards')));
  if(v.can_act){
    need(self&&h&&!h.recovering&&self.connected&&!self.is_folded&&!self.is_all_in&&h.actor_seat===v.seat_no&&h.street!=='COMMITTED'&&h.street!=='SETTLED'&&time(h.action_deadline_at)&&Date.parse(t.server_now)<Date.parse(h.action_deadline_at));
    legal(v.legal);
  }else need(!own(v,'legal'));
  const timeline=object(t.timeline,'last_sequence truncated events','hand_id');
  need(pokerVersion(timeline.last_sequence)&&bool(timeline.truncated)&&Array.isArray(timeline.events)&&timeline.events.length<=128);
  need(own(timeline,'hand_id')?!!h&&timeline.hand_id===h.hand_id:!h);
  let timelineSequence=0n;
  for(const raw of timeline.events){
    const event=object(raw,'sequence hand_version type street occurred_at','seat_no delta_units to_units');
    need(pokerVersion(event.sequence)&&BigInt(event.sequence)>timelineSequence&&pokerVersion(event.hand_version));timelineSequence=BigInt(event.sequence as string);
    need(timelineTypes.includes(event.type as string)&&timelineStreets.includes(event.street as string)&&time(event.occurred_at));
    optional(event,'seat_no',n=>range(n,1,maxSeats));
    for(const k of ['delta_units','to_units'])if(own(event,k))need(amount(event[k])&&BigInt(event[k] as string)>0n);
  }
  need(timeline.last_sequence===(timeline.events.length?String(timelineSequence):'0'));
  if(own(t,'chat')){
    const chat=object(t.chat,'enabled can_send muted last_sequence truncated messages');
    need(chat.enabled===true&&bool(chat.can_send)&&bool(chat.muted)&&pokerVersion(chat.last_sequence)&&bool(chat.truncated)&&Array.isArray(chat.messages)&&chat.messages.length<=100);
    need(!chat.muted||chat.can_send===false);
    let chatSequence=0n;
    for(const raw of chat.messages){
      const message=object(raw,'sequence message_id kind body created_at','author');
      need(pokerVersion(message.sequence)&&BigInt(message.sequence)>chatSequence&&id(message.message_id)&&['USER_TEXT','SYSTEM'].includes(message.kind as string)&&text(message.body,2048)&&time(message.created_at));
      chatSequence=BigInt(message.sequence as string);
      if(message.kind==='USER_TEXT'){
        const author=object(message.author,'member_id display_name avatar_id');
        need(id(author.member_id)&&text(author.display_name,4096)&&text(author.avatar_id,512));
      }else need(!own(message,'author'));
    }
    need(BigInt(chat.last_sequence as string)>=chatSequence);
  }
  if(own(t,'host')){
    need(viewerKind==='HOST'||viewerKind==='PLAYER_SELF');
    const host=object(t.host,'is_host capabilities players spectators spectators_truncated chat_targets chat_targets_truncated');
    need(host.is_host===true&&Array.isArray(host.capabilities)&&host.capabilities.length<=hostCommands.length&&host.capabilities.every(c=>hostCommands.includes(c as string))&&new Set(host.capabilities).size===host.capabilities.length);
    need(Array.isArray(host.players)&&host.players.length<=maxSeats&&Array.isArray(host.spectators)&&host.spectators.length<=256&&bool(host.spectators_truncated)&&Array.isArray(host.chat_targets)&&host.chat_targets.length<=100&&bool(host.chat_targets_truncated));
    const seatsSeen=new Set<number>(),sessionsSeen=new Set<string>(),usersSeen=new Set<string>();
    for(const raw of host.players){
      const player=object(raw,'target_session_id target_user_id seat_no display_name removal_pending muted');
      need(id(player.target_session_id)&&!sessionsSeen.has(player.target_session_id)&&positiveUser(player.target_user_id)&&range(player.seat_no,1,maxSeats)&&!seatsSeen.has(player.seat_no)&&text(player.display_name,4096)&&bool(player.removal_pending)&&bool(player.muted));
      sessionsSeen.add(player.target_session_id as string);seatsSeen.add(player.seat_no as number);
    }
    for(const raw of host.spectators){
      const spectator=object(raw,'target_user_id display_name avatar_id muted');
      need(positiveUser(spectator.target_user_id)&&!usersSeen.has(spectator.target_user_id)&&text(spectator.display_name,4096)&&text(spectator.avatar_id,512)&&bool(spectator.muted));usersSeen.add(spectator.target_user_id as string);
    }
    const membersSeen=new Set<string>();
    for(const raw of host.chat_targets){
      const target=object(raw,'target_user_id member_id display_name avatar_id muted');
      need(positiveUser(target.target_user_id)&&id(target.member_id)&&!membersSeen.has(target.member_id)&&text(target.display_name,4096)&&text(target.avatar_id,512)&&bool(target.muted));membersSeen.add(target.member_id as string);
    }
  }else if(viewerKind==='HOST')need(false);
  // The caller replaces its whole projection. Sharing mutable parsed input could reintroduce private fields after validation.
  return structuredClone(t) as unknown as TableView;
}
