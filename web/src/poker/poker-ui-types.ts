import type { ReactNode } from 'react';

/** Viewer projections only. Never pass an engine State/Snapshot or raw events. */
export type Units = string;
export type ActionType = 'FOLD' | 'CHECK' | 'CALL' | 'BET' | 'RAISE' | 'ALL_IN';
export type ConnectionState = 'IDLE' | 'CONNECTING' | 'AUTH_PENDING' | 'SYNCING' | 'LIVE' | 'DEGRADED' | 'RECONNECTING' | 'DISCONNECTED' | 'TAKEN_OVER' | 'CLOSED';
export type PokerTicketIntent = 'CLAIM_CONTROL' | 'READ_ONLY';
export interface PokerControlRecovery {
  phase:'SENT'|'UNKNOWN'|'ACKNOWLEDGED';querying:boolean;can_retry:boolean;target_current:boolean;
}
export interface PokerAuthority {
  user_id: string; runtime_id: string; event_sequence: string;
  connection_state: ConnectionState; has_snapshot: boolean; pending: boolean;
  can_reconnect: boolean; can_takeover: boolean;
  /** Runtime projection of original-intent/query/current-guard eligibility, never ordinary mutation authority. */
  can_retry_pending?:boolean;
  can_recover_control?:boolean;control_recovery?:PokerControlRecovery;
  control_history?:{count:number;querying:boolean};
  /** Confirmed for this specific socket/session/epoch; absent is read-only at the table. */
  can_control?: boolean;
  chat_recovery?: { phase:'SENT'|'UNKNOWN'|'ACKNOWLEDGED';querying:boolean;can_retry:boolean };
  ticket_intent?: PokerTicketIntent;
  viewer_kind: 'PLAYER_SELF' | 'SPECTATOR' | 'HOST' | 'OPS';
}
export interface PokerChatAuthor {
  member_id:string;display_name:string;avatar_id:string;
}
export interface PokerChatMessage {
  sequence:string;message_id:string;kind:'USER_TEXT'|'SYSTEM';body:string;created_at:string;author?:PokerChatAuthor;
}
export interface PokerChatView {
  enabled:true;can_send:boolean;muted:boolean;last_sequence:string;truncated:boolean;messages:PokerChatMessage[];
}
export type PokerTimelineType =
  | 'HAND_COMMITTED'|'POST_SB'|'POST_BB'|'FOLD'|'CHECK'|'CALL'|'BET'|'RAISE'|'ALL_IN'
  | 'AUTO_FOLD'|'AUTO_CHECK'|'DISCONNECTED'|'RECONNECTED'|'RECOVERING'|'RESUMED'|'ACTOR_CHANGED'
  | 'ALL_IN_RUNOUT'|'DEAL_FLOP'|'DEAL_TURN'|'DEAL_RIVER'|'RETURN_UNCALLED'|'SHOWDOWN'|'POT_AWARD'|'SYSTEM_SETTLEMENT';
export interface PokerTimelineEvent {
  sequence:string;hand_version:string;type:PokerTimelineType;street:string;seat_no?:number;
  delta_units?:Units;to_units?:Units;occurred_at:string;
}
export interface PokerTimelineView {
  hand_id?:string;last_sequence:string;truncated:boolean;events:PokerTimelineEvent[];
}
export type PokerHostCommandType = 'PAUSE_ACCEPTING_PLAYERS'|'RESUME_ACCEPTING_PLAYERS'|'REMOVE_PLAYER_AFTER_HAND'|'REMOVE_SPECTATOR'|'MUTE_CHAT_USER'|'CLOSE_TABLE';
export interface PokerHostPlayer {
  target_session_id:string;target_user_id:string;seat_no:number;display_name:string;removal_pending:boolean;muted:boolean;
}
export interface PokerHostSpectator {
  target_user_id:string;display_name:string;avatar_id:string;muted:boolean;
}
export interface PokerHostChatTarget extends PokerHostSpectator { member_id:string }
export interface PokerHostView {
  is_host:true;capabilities:PokerHostCommandType[];players:PokerHostPlayer[];
  spectators:PokerHostSpectator[];spectators_truncated:boolean;
  chat_targets:PokerHostChatTarget[];chat_targets_truncated:boolean;
}
export interface PokerHostCommand {
  command:PokerHostCommandType;target_session_id?:string;target_user_id?:string;
}
export interface PokerHostOperationView {
  command:PokerHostCommand;phase:'SENT'|'UNKNOWN'|'ACKNOWLEDGED';querying:boolean;can_retry:boolean;status?:string;
}
export interface LegalActions {
  actions: ActionType[]; to_call_units: Units; call_applied_units: Units;
  minimum_bet_units: Units; minimum_raise_to_units: Units; maximum_raise_to_units: Units;
  raise_rights: boolean;
  shortcuts: { name: string; action_type: 'BET' | 'RAISE' | 'ALL_IN'; target_to_units: Units }[];
}
interface SeatBase {
  seat_no: number; display_name: string; state: string; connected: boolean;
  stack_units: Units; street_committed_units: Units; total_committed_units: Units;
  is_folded: boolean; is_all_in: boolean; hole_card_count: number;
  /** Explicitly released by the server; never copied from private hole_cards. */
  public_hole_cards?: number[];
  hole_cards_released?: boolean;
  sit_out_next_hand: boolean; leave_after_hand: boolean; pending_top_up_units: Units;
  rebuy_deadline_at?: string;
}
export type SeatView = SeatBase & ({ is_self: true; hole_cards?: number[] } | { is_self: false; hole_cards?: never });
export interface HandView {
  economy_settlement?:import('../economy-cap').CapSettlement;
  hand_id: string; hand_version: string; street: string; button_seat: number; actor_seat: number;
  board_cards: number[]; pot_units: Units;
  pots: { index: number; amount_units: Units; eligible_seats: number[]; awards: { seat_no: number; amount_units: Units }[] }[];
  action_sequence: string; action_deadline_at?: string; recovering: boolean; recovery_until?: string;
  server_seed_hash: string; deck_hash: string;
}
export interface ViewerView {
  session_id?: string; seat_no?: number; control_epoch: string;
  can_act: boolean; can_top_up: boolean; can_leave: boolean; can_resume: boolean;
  can_sit_out?: boolean; can_start?: boolean; top_up_min_units?: Units; top_up_max_units?: Units;
  legal?: LegalActions;
  /** WS-only transport projection; ordinary HTTP has no socket grant. */
  control?: PokerConnectionControl;
}
export interface PokerConnectionControl {
  connection_id:string;session_id?:string;mode:'CONTROLLER'|'READ_ONLY';control_epoch:string;
}
export interface TableView {
  table_id: string; name: string; lifecycle_state: string; table_version: string; max_seats: number;
  small_blind_units: Units; big_blind_units: Units; settings_locked: boolean; server_now: string;
  intermission_until?: string; seats: SeatView[]; hand?: HandView; viewer: ViewerView;
  chat?:PokerChatView;timeline:PokerTimelineView;host?:PokerHostView;
}
export interface PokerIntentContext {
  user_id: string; runtime_id: string; event_sequence: string;
  table_id?: string; table_version?: string; hand_id?: string; hand_version?: string;
  session_id?: string; control_epoch?: string; action_sequence?: string;
}
export type TableIntent =
  | { type: 'action'; action_type: ActionType; target_to_units: Units }
  | { type: 'topup'; amount_units: Units }
  | { type: 'leave'; return_to_lobby: true }
  | { type: 'reconnect'; control_intent?: PokerTicketIntent }
  | { type: 'sitout' | 'resume' | 'takeover' | 'return_lobby' };
export interface TableUi {
  bet: { action_type: 'BET' | 'RAISE' | 'ALL_IN'; amount_chips: string };
  top_up_chips: string; confirm_leave: boolean; confirm_takeover: boolean;
}
export interface PokerTableProps {
  table: TableView; authority: PokerAuthority; ui: TableUi;
  /** Parent policy gate only; never fabricates transport authority or clears pending. */
  mutation_blocked?: boolean;
  /** Supplied by the parent's server-aligned clock; display only, never a timer command. */
  display_now?: string;
  notice?: string;recovery_status?:string;
  onUiChange: (next: TableUi) => void;
  onIntent: (intent: TableIntent, context: PokerIntentContext) => void;
  /** Read-only recovery callback, deliberately separate from mutation intents. */
  onQueryReceipt?:()=>void;receipt_querying?:boolean;
  onRetryPending?:(context:PokerIntentContext)=>void;
  onQueryTakeover?:()=>void;onRetryTakeover?:(context:PokerIntentContext)=>void;
  onQueryPreviousTakeovers?:()=>void;
  chat_draft?:string;onChatDraftChange?:(next:string)=>void;
  onSendChat?:(message:string,context:PokerIntentContext)=>void;
  onQueryChatReceipt?:()=>void;onRetryChat?:(context:PokerIntentContext)=>void;
  host_operation?:PokerHostOperationView;
  onHostCommand?:(command:PokerHostCommand,context:PokerIntentContext)=>void;
  onQueryHostReceipt?:()=>void;onRetryHost?:(context:PokerIntentContext)=>void;
}

export interface LobbyBlinds {
  small_blind_units:Units;big_blind_units:Units;ante_units:Units;
  minimum_buyin_units:Units;maximum_buyin_units:Units;
}
export interface BlindPreset extends LobbyBlinds { id:string }
export interface LobbyTable extends LobbyBlinds {
  table_id: string; name: string; visibility: 'PUBLIC' | 'PASSWORD'; lifecycle_state: string;
  table_version:Units;blind_preset_id:string;max_seats:number;occupied_seats:number;open_seat_numbers:number[];
  accepting_players:boolean;allow_new_hands:boolean;allow_spectators:boolean;chat_enabled:boolean;can_join:boolean;can_spectate:boolean;can_request_access:boolean;
}
export interface PokerLobbyReadScope { user_id:string;session_generation:number;request_generation:number;query_key:string }
export interface PokerLobbyAuthority { scope:PokerLobbyReadScope;read_state:'FRESH'|'LOADING'|'STALE'|'ERROR' }
export type PokerLobbyIntentContext = PokerLobbyReadScope;
export interface PokerLobbySnapshot {
  server_now:string;
  service:{state:'READY'|'MAINTENANCE'|'CONFIG_INCOMPLETE';production_ready:boolean;blockers:string[];maintenance_scopes:string[]};
  ruleset:{version:string;ante_posting_mode:string;entry_mode:string;initial_button_version:string;evaluator_version:string;shortcut_version:string;algorithm_version:string;deal_version:string}|null;
  viewer:{user_id:string;available_chips_units:Units;wallet_version:Units;poker_in_play_units:Units;profile_complete:boolean;owned_open_table_id:string|null;can_create:boolean;can_join:boolean};
  active_session:{session_id:string;table_id:string;table_name:string;state:'ACTIVE'|'NEEDS_REVIEW';seat_no:number;stack_units:Units;committed_units:Units;poker_in_play_units:Units;small_blind_units:Units;big_blind_units:Units;ante_units:Units;can_reconnect:boolean}|null;
  create_options:{access_modes:('PUBLIC'|'PASSWORD')[];chat_configurable:boolean};
  blind_presets:BlindPreset[];tables:LobbyTable[];page:{limit:number;next_cursor:string|null};
}
export interface LobbyFilters {
  query: string; visibility: 'ALL' | 'PUBLIC' | 'PASSWORD'; open_seats_only: boolean;
  max_seats: 'ALL' | number; blind_preset_id: string; lifecycle_state: string;
  spectators_only: boolean; sort: 'LOW_BLIND' | 'HIGH_BLIND' | 'NEAR_FULL';limit:number;cursor:string|null;
}
export interface CreateDraft {
  name: string; visibility: 'PUBLIC' | 'PASSWORD'; password: string; max_seats: number;
  blind_preset_id: string; allow_spectators: boolean; chat_enabled: boolean;
}
export interface JoinDraft { seat_no?: number; password: string; buy_in_chips: string }
export type LobbyFlow = { kind: 'NONE' } | { kind: 'CREATE'; draft: CreateDraft } | {
  kind: 'JOIN'; table_id: string; access_granted: boolean; can_reserve: boolean; can_buy_in: boolean;
  seats: { seat_no: number; available: boolean }[]; draft: JoinDraft;
  reservation?: { reservation_id: string; seat_no: number; expires_at: string; valid: boolean };
};
export type LobbyIntent =
  | { type: 'create'; name: string; visibility: 'PUBLIC' | 'PASSWORD'; password?: string; max_seats: number; blind_preset_id: string; allow_spectators: boolean; chat_enabled: boolean }
  | { type: 'open_table' | 'spectate'; table_id: string }
  | { type: 'access'; table_id: string; password: string }
  | { type: 'reserve'; table_id: string; seat_no: number }
  | { type: 'buyin'; table_id: string; reservation_id: string; seat_no: number; amount_units: Units; entry_mode: 'WAIT_FOR_BB' }
  | { type: 'reconnect'; table_id: string; session_id: string }
  | { type: 'refresh' | 'wallet' | 'history' | 'rewards' };
export interface PokerLobbyProps {
  snapshot:PokerLobbySnapshot;authority:PokerLobbyAuthority;entry_pending:boolean;
  /** Parent policy only; does not revoke original receipts or read-only recovery. */
  entry_blocked?:boolean;
  filters:LobbyFilters;flow:LobbyFlow;
  notice?: string;
  details?: ReactNode;
  onFiltersChange: (next: LobbyFilters) => void;
  onFlowChange: (kind: 'NONE' | 'CREATE') => void;
  onCreateDraftChange: (next: CreateDraft) => void;
  onJoinDraftChange: (next: JoinDraft) => void;
  onIntent: (intent: LobbyIntent, context: PokerLobbyIntentContext) => void;
}

export const UNITS_PER_CHIP = 500000n;
export function units(value?: Units): bigint | undefined {
  return value !== undefined && /^(0|[1-9][0-9]{0,39})$/.test(value) ? BigInt(value) : undefined;
}
export function pokerUnits(value?: Units): bigint | undefined {
  const n=units(value); return n !== undefined && n % UNITS_PER_CHIP === 0n ? n : undefined;
}
export function chipInput(value: string): bigint | undefined {
  return /^[0-9]{1,34}$/.test(value) ? BigInt(value)*UNITS_PER_CHIP : undefined;
}
export function chips(value?: Units): string {
  const n=units(value); if(n===undefined)return '—';
  const whole=(n/UNITS_PER_CHIP).toLocaleString('en-US');
  const fraction=((n%UNITS_PER_CHIP)*2n).toString().padStart(6,'0').replace(/0+$/,'');
  return whole+(fraction?'.'+fraction:'');
}
export function chipText(value: Units): string { const n=pokerUnits(value);return n===undefined?'':(n/UNITS_PER_CHIP).toString(); }
export function hasAuthority(a: PokerAuthority): boolean {
  return !!a.user_id && !!a.runtime_id && /^[0-9]+$/.test(a.event_sequence) && a.has_snapshot && a.connection_state==='LIVE' && !a.pending && !a.control_recovery;
}
export function intentContext(a: PokerAuthority, table?: TableView): PokerIntentContext {
  return {user_id:a.user_id,runtime_id:a.runtime_id,event_sequence:a.event_sequence,...(table?{
    table_id:table.table_id,table_version:table.table_version,hand_id:table.hand?.hand_id,hand_version:table.hand?.hand_version,
    session_id:table.viewer.session_id,control_epoch:table.viewer.control_epoch,action_sequence:table.hand?.action_sequence,
  }: {})};
}
export const connectionLabel: Record<ConnectionState,string> = {
  IDLE:'尚未连接',CONNECTING:'连接中',AUTH_PENDING:'等待认证',SYNCING:'同步权威状态',LIVE:'实时连接',
  DEGRADED:'连接不稳定',RECONNECTING:'重新连接中',DISCONNECTED:'连接已断开',TAKEN_OVER:'其他设备已接管',CLOSED:'连接已关闭',
};
export function stateLabel(state: string): string {
  return ({WAITING_ENTRY:'等待入座',ACTIVE:'参与本手',WAITING_BIG_BLIND:'等待大盲',REBUY_WINDOW:'等待补充筹码',LEAVE_AFTER_HAND:'本手后离座',LEFT:'已离座',COMMITTED:'本手已承诺 · 等待发牌',RECOVERING:'服务恢复中',WAITING:'等待开局',IN_HAND:'牌局进行中',INTERMISSION:'手间休息',PAUSED:'已暂停',CLOSING:'安全关闭中',CLOSED:'已关闭',PLAYING:'参与本手',WAIT_FOR_BB:'等待大盲',SIT_OUT:'暂离',RESERVED:'座位已预留',READY:'准备就绪',REBUY:'等待补充筹码',PREFLOP:'翻牌前',FLOP:'翻牌',TURN:'转牌',RIVER:'河牌',SETTLED:'本手已结算'} as Record<string,string>)[state] ?? state;
}
