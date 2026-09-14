import { ApiError, type ApiClient } from '../api';
import { integer } from '../wallet-api';
import { pokerChipAmount, pokerTimestamp, pokerUUID } from './poker-view';
import type { LobbyFilters, PokerLobbyReadScope, PokerLobbySnapshot } from './poker-ui-types';

type Value=Record<string,unknown>;
const base='/api/v1/poker',scopeKeys=['user_id','session_generation','request_generation','query_key'] as const;
const invalid=()=>new ApiError('大厅数据或读取上下文尚未核实，请刷新。',0,'POKER_INVALID_LOBBY_READ');
function need(ok:unknown):asserts ok {if(!ok)throw invalid();}
function object(raw:unknown,fields:string):Value {
  need(raw!==null&&typeof raw==='object'&&!Array.isArray(raw));
  need(Object.getPrototypeOf(raw)===Object.prototype||Object.getPrototypeOf(raw)===null);
  const v=raw as Value,keys=fields.split(' ');
  need(keys.every(k=>Object.hasOwn(v,k))&&Object.keys(v).every(k=>keys.includes(k)));return v;
}
const bool=(v:unknown)=>typeof v==='boolean';
const text=(v:unknown,max=64):v is string=>typeof v==='string'&&v.length>0&&new TextEncoder().encode(v).length<=max&&!/\p{Cs}/u.test(v);
const range=(v:unknown,min:number,max:number):v is number=>typeof v==='number'&&Number.isInteger(v)&&v>=min&&v<=max;
const positive=(v:unknown):v is string=>integer(v)&&BigInt(v)>0n;
const cursor=(v:unknown)=>typeof v==='string'&&/^[A-Za-z0-9_-]{1,512}$/.test(v);
const lifecycle=['WAITING','IN_HAND','INTERMISSION','PAUSED','RECOVERING','CLOSING','CLOSED'];
function list(raw:unknown,max=100):unknown[] {need(Array.isArray(raw)&&raw.length<=max);return Array.from(raw);}
function enums(raw:unknown,allowed:string[]):string[] {
  const values=list(raw,allowed.length);need(values.every(v=>allowed.includes(v as string))&&new Set(values).size===values.length);return values as string[];
}
const blindFields='small_blind_units big_blind_units ante_units minimum_buyin_units maximum_buyin_units';
function blinds(v:Value,bounds=true) {
  for(const k of (bounds?blindFields:'small_blind_units big_blind_units ante_units').split(' '))need(pokerChipAmount(v[k]));
  need(BigInt(v.small_blind_units as string)>0n&&BigInt(v.big_blind_units as string)===2n*BigInt(v.small_blind_units as string)&&v.ante_units==='0');
  if(bounds)need(BigInt(v.minimum_buyin_units as string)>0n&&BigInt(v.maximum_buyin_units as string)>=BigInt(v.minimum_buyin_units as string));
}
/** Whole L1 DTO only: no socket envelope, controller, hand or entry receipt. */
export function parsePokerLobbySnapshot(raw:unknown,expectedUserID:string):PokerLobbySnapshot {
  need(positive(expectedUserID));
  const v=object(raw,'server_now service ruleset viewer active_session create_options blind_presets tables page');
  need(pokerTimestamp(v.server_now));
  const service=object(v.service,'state production_ready blockers maintenance_scopes');
  need(['READY','MAINTENANCE','CONFIG_INCOMPLETE'].includes(service.state as string)&&bool(service.production_ready));
  enums(service.blockers,['MAINTENANCE_ACTIVE','POKER_RULESET_INCOMPLETE']);
  enums(service.maintenance_scopes,['CHALDEA_USER_WRITES','POKER_NEW_TABLES_NEW_HANDS']);
  if(v.ruleset!==null){
    const expected={version:'poker-cash-v1-20260906',ante_posting_mode:'NO_ANTE',entry_mode:'WAIT_FOR_BB',initial_button_version:'poker-initial-button-v1',evaluator_version:'poker-holdem-high-v1',shortcut_version:'poker-pot-after-call-whole-chip-v1',algorithm_version:'poker-deck-v1',deal_version:'poker-deal-v1'};
    const rules=object(v.ruleset,Object.keys(expected).join(' '));need(Object.entries(expected).every(([k,value])=>rules[k]===value));
  }
  const viewer=object(v.viewer,'user_id available_chips_units wallet_version poker_in_play_units profile_complete owned_open_table_id can_create can_join');
  need(viewer.user_id===expectedUserID&&integer(viewer.available_chips_units)&&integer(viewer.wallet_version)&&pokerChipAmount(viewer.poker_in_play_units));
  need(viewer.owned_open_table_id===null||pokerUUID(viewer.owned_open_table_id));
  for(const k of ['profile_complete','can_create','can_join'])need(bool(viewer[k]));
  if(v.active_session!==null){
    const active=object(v.active_session,'session_id table_id table_name state seat_no stack_units committed_units poker_in_play_units small_blind_units big_blind_units ante_units can_reconnect');
    need(pokerUUID(active.session_id)&&pokerUUID(active.table_id)&&text(active.table_name,65536)&&range(active.seat_no,1,9));
    need(['ACTIVE','NEEDS_REVIEW'].includes(active.state as string)&&bool(active.can_reconnect));blinds(active,false);
    for(const k of ['stack_units','committed_units','poker_in_play_units'])need(pokerChipAmount(active[k]));
    need(BigInt(active.stack_units as string)+BigInt(active.committed_units as string)===BigInt(active.poker_in_play_units as string));
    need(viewer.poker_in_play_units===active.poker_in_play_units);
  }else need(viewer.poker_in_play_units==='0');
  const options=object(v.create_options,'access_modes chat_configurable');
  const accessModes=enums(options.access_modes,['PUBLIC','PASSWORD']);
  need(accessModes.length>=1&&accessModes[0]==='PUBLIC'&&bool(options.chat_configurable));
  const presets=list(v.blind_presets),presetIDs=new Set<string>();
  for(const rawPreset of presets){
    const preset=object(rawPreset,`id ${blindFields}`);need(text(preset.id)&&!presetIDs.has(preset.id));presetIDs.add(preset.id);blinds(preset);
  }
  const tables=list(v.tables),tableIDs=new Set<string>();
  for(const rawTable of tables){
    const table=object(rawTable,`table_id table_version name visibility blind_preset_id lifecycle_state max_seats occupied_seats open_seat_numbers accepting_players allow_new_hands allow_spectators chat_enabled can_join can_spectate can_request_access ${blindFields}`);
    need(pokerUUID(table.table_id)&&!tableIDs.has(table.table_id)&&positive(table.table_version));tableIDs.add(table.table_id);
    need(text(table.name,65536)&&text(table.blind_preset_id)&&['PUBLIC','PASSWORD'].includes(table.visibility as string)&&lifecycle.includes(table.lifecycle_state as string));
    need(range(table.max_seats,2,9)&&range(table.occupied_seats,0,table.max_seats));
    const seats=list(table.open_seat_numbers,table.max_seats);need(seats.every(n=>range(n,1,table.max_seats as number))&&new Set(seats).size===seats.length);
    need(seats.length<=table.max_seats-table.occupied_seats);
    for(const k of ['accepting_players','allow_new_hands','allow_spectators','chat_enabled','can_join','can_spectate','can_request_access'])need(bool(table[k]));
    need(table.visibility==='PASSWORD'||table.can_request_access===false);blinds(table);
  }
  const page=object(v.page,'limit next_cursor');need(range(page.limit,1,100)&&tables.length<=page.limit&&(page.next_cursor===null||cursor(page.next_cursor)));
  return structuredClone(v) as unknown as PokerLobbySnapshot;
}

/** Canonical read path; a server cursor is passed through, never decoded or fabricated. */
export function pokerLobbyReadPath(query?:LobbyFilters):string {
  if(query===undefined)return base;
  const q=object(query,'query visibility open_seats_only max_seats blind_preset_id lifecycle_state spectators_only sort limit cursor');
  need(typeof q.query==='string');const search=q.query.trim();
  need(!/[\p{Cc}\p{Cs}]/u.test(search)&&Array.from(new Intl.Segmenter(undefined,{granularity:'grapheme'}).segment(search)).length<=40);
  need(['ALL','PUBLIC','PASSWORD'].includes(q.visibility as string)&&bool(q.open_seats_only)&&bool(q.spectators_only));
  need(q.max_seats==='ALL'||range(q.max_seats,2,9));need(text(q.blind_preset_id)&&['ALL',...lifecycle].includes(q.lifecycle_state as string));
  need(['LOW_BLIND','HIGH_BLIND','NEAR_FULL'].includes(q.sort as string)&&range(q.limit,1,100)&&(q.cursor===null||cursor(q.cursor)));
  const params=new URLSearchParams();if(search)params.set('q',search);
  params.set('access_mode',query.visibility);params.set('open_seats_only',String(query.open_seats_only));
  if(query.max_seats!=='ALL')params.set('max_seats',String(query.max_seats));
  params.set('blind_preset',query.blind_preset_id);params.set('lifecycle_state',query.lifecycle_state);
  params.set('spectators_only',String(query.spectators_only));params.set('sort',query.sort);params.set('limit',String(query.limit));
  if(query.cursor!==null)params.set('cursor',query.cursor);return `${base}/tables?${params}`;
}
export function capturePokerLobbyScope(client:ApiClient,query:LobbyFilters|undefined,generation:number):PokerLobbyReadScope {
  const auth=client.getSnapshot(),session=client.getSessionGeneration();
  need(auth.ready&&!auth.loggingOut&&auth.user&&Number.isSafeInteger(auth.user.id)&&auth.user.id>0);
  need([generation,session].every(n=>Number.isSafeInteger(n)&&n>=0));
  return {user_id:String(auth.user.id),session_generation:session,request_generation:generation,query_key:pokerLobbyReadPath(query)};
}
export function currentPokerLobbyScope(client:ApiClient,scope:PokerLobbyReadScope,getCurrent:()=>PokerLobbyReadScope|null):boolean {
  const current=getCurrent();if(!current||scopeKeys.some(k=>current[k]!==scope[k]))return false;
  const auth=client.getSnapshot();
  return !!auth.ready&&!auth.loggingOut&&!!auth.user&&Number.isSafeInteger(auth.user.id)&&auth.user.id>0&&String(auth.user.id)===scope.user_id&&client.getSessionGeneration()===scope.session_generation&&Number.isSafeInteger(scope.request_generation)&&scope.request_generation>=0;
}
/** One whole read. The caller owns read generation/freshness; this function owns no timers or entry state. */
export async function readPokerHttpLobby(client:ApiClient,scope:PokerLobbyReadScope,getCurrent:()=>PokerLobbyReadScope|null,query?:LobbyFilters):Promise<PokerLobbySnapshot|undefined> {
  const captured={...scope},path=pokerLobbyReadPath(query),current=()=>currentPokerLobbyScope(client,captured,getCurrent);
  need(path===captured.query_key&&current());
  try{
    const raw=await client.request(path,'GET',undefined,undefined,current);if(!current())return undefined;
    return parsePokerLobbySnapshot(raw,captured.user_id);
  }catch(error){
    if(!current())return undefined;
    throw new ApiError('大厅读取尚未完成，请重新刷新。',error instanceof ApiError?error.status:0,'POKER_LOBBY_READ_UNAVAILABLE');
  }
}
