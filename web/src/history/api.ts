import { z } from 'zod';
import { ApiError, type ApiClient } from '../api';
import { transactionStatus } from '../economy-api';

export const recordTypes = ['DIRECT_PLAY_ROUND', 'POKER_SESSION', 'POKER_HAND'] as const;
export const results = ['WIN', 'LOSS', 'BREAK_EVEN', 'CANCELLED', 'REFUNDED'] as const;
export const statuses = ['PROCESSING', 'SETTLED', 'CANCELLED', 'REFUNDED', 'RECOVERING'] as const;
export const idSchema = z.string().regex(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/);
const integer = z.string().regex(/^-?(0|[1-9]\d*)$/).max(20).refine(v => BigInt(v) >= -9223372036854775807n && BigInt(v) <= 9223372036854775807n);
const time = z.iso.datetime({ offset: true });
const filterTime = time.refine(v => /(?:Z|\+00:00)$/.test(v) && !/\.\d{7}/.test(v), '使用 UTC 时间且最多六位小数');
const optionalFilter = <T extends z.ZodType>(s: T) => s.optional();
export const searchSchema = z.strictObject({
  record_type: optionalFilter(z.enum(recordTypes)), mode: optionalFilter(z.enum(['DIRECT_PLAY', 'POKER'])),
  game_slug: optionalFilter(z.string().regex(/^[a-z0-9]+(?:-[a-z0-9]+)*$/).max(128)),
  time_from: optionalFilter(filterTime), time_to: optionalFilter(filterTime),
  result: optionalFilter(z.enum(results)), status: optionalFilter(z.enum(statuses)), id: optionalFilter(idSchema),
  cursor: optionalFilter(z.string().min(1).max(1024)),
}).refine(q => !q.time_from || !q.time_to || Date.parse(q.time_from) < Date.parse(q.time_to), '开始时间必须早于结束时间');
export function readSearch(raw: string) {
  const params = new URLSearchParams(raw), values: Record<string, string> = {};
  for (const [key, value] of params) {
    if (key in values) throw new Error('筛选参数重复，请清除筛选后重试。');
    values[key] = value;
  }
  return searchSchema.parse(values);
}
export function listQuery(raw: string) {
  const q = readSearch(raw), params = new URLSearchParams();
  for (const [key, value] of Object.entries(q)) if (value !== undefined) params.set(key, value);
  params.set('limit', '50');
  return params.toString();
}
export const summarySchema = z.object({
  record_type: z.enum(recordTypes), source_id: idSchema, parent_source_id: idSchema.nullable(),
  game_slug: z.string(), mode: z.string(), occurred_at: time, ended_at: time.nullable(),
  result: z.enum(results).nullable(), status: z.enum(statuses), source_version: z.string(),
  stake_units: integer.nullable(), payout_units: integer.nullable(), net_change_units: integer.nullable(),
  initial_buyin_units: integer.nullable(), total_topup_units: integer.nullable(), final_cashout_units: integer.nullable(),
  snapshot: z.object({ snapshot_id: idSchema, game_title: z.string(), table_id: idSchema.nullable(), table_name: z.string().nullable(), actor_display_name: z.string().nullable(), metadata_origin: z.string() }),
});
export const pageSchema = z.object({ items: z.array(summarySchema), next_cursor: z.string().nullable(), has_more: z.boolean(), game_options: z.array(z.object({ game_slug: z.string(), game_title: z.string(), retired: z.boolean() })) })
  .refine(p => !p.has_more || (p.items.length > 0 && !!p.next_cursor));
export const transactionSchema = z.object({
  id: idSchema, kind: z.string(), status: z.string(), created_at: time, confirmed_at: time.optional(),
  exchange_id:idSchema.optional(),reserve_debit_units:integer.optional(),active_debit_units:integer.optional(),chips_credit_units:integer.optional(),
  native_effects:z.array(z.object({delta_units:integer,before_units:integer,after_units:integer}).refine(e=>BigInt(e.before_units)>=0n && BigInt(e.after_units)>=0n && BigInt(e.delta_units)!==0n && BigInt(e.after_units)-BigInt(e.before_units)===BigInt(e.delta_units))).optional(),
  effects: z.array(z.object({ ledger_id: idSchema, leg_no: z.number().int(), asset: z.string(), delta_units: integer, balance_before_units: integer, balance_after_units: integer })),
  links: z.array(z.object({ record_type: z.enum(recordTypes), source_id: idSchema })),
}).refine(t=>t.kind==='API_CHIPS_EXCHANGE'
 ? !!t.exchange_id && Object.hasOwn(transactionStatus,t.status) && !!t.reserve_debit_units && !!t.active_debit_units && !!t.chips_credit_units
 && BigInt(t.reserve_debit_units)>=0n && BigInt(t.active_debit_units)>0n && BigInt(t.active_debit_units)<=2147483647n
 && BigInt(t.reserve_debit_units)+BigInt(t.active_debit_units)===BigInt(t.chips_credit_units) && (t.status!=='CONFIRMED' || !!t.confirmed_at)
 : !!t.confirmed_at);
const metadata = z.object({ snapshot_id: idSchema, game_title: z.string(), table_name: z.string().nullable().optional(), actor_display_name: z.string().nullable().optional(), metadata_origin: z.string(), captured_at: time });
const values = z.record(z.string(), z.json());
const cards = z.array(z.number().int().min(0).max(311));
export const roundSchema = z.object({
  id: idSchema, game: z.string(), state: z.string(), recovery_state: z.string(), metadata,
  total_stake_units: integer, total_payout_units: integer.nullable(), net_change_units: integer.nullable(),
  balance_before_units: integer, balance_after_units: integer, created_at: time, settled_at: time.nullable(),
  common_result: z.string().optional(), input: values, fairness: values, transactions: z.array(transactionSchema),
  dice: values.optional(), scratch: values.optional(), summon: values.optional(), slot: values.optional(), blackjack: values.optional(),
});
const handSummary = z.object({ hand_id: idSchema, hand_no: integer, state: z.string(), created_at: time, settled_at: time.nullable() });
export const sessionSchema = z.object({
  session_id: idSchema, table_id: idSchema, seat_no: z.number().int(), state: z.string(), started_at: time, ended_at: time.nullable(), end_reason: z.string().nullable(),
  initial_buyin_units: integer, confirmed_topup_units: integer, confirmed_rebuy_units: integer, final_cashout_units: integer.nullable(), realized_pl_units: integer.nullable(),
  metadata, configuration: values, funding_count: integer, hand_count: integer,
  funding: z.array(z.object({ funding_operation_id: idSchema, kind: z.string(), state: z.string(), amount_units: integer, created_at: time, confirmed_at: time.nullable(), failure_code: z.string().nullable(), transaction: transactionSchema.optional() })),
  hands: z.array(handSummary), next_funding_cursor: z.string().optional(), next_hand_cursor: z.string().optional(), read_at: time,
});
export const handSchema = handSummary.extend({
  table_id: idSchema, session_id: idSchema, seat_no: z.number().int(), button_seat: z.number().int(), hand_version: integer,
  metadata, configuration: values, board_cards: cards,
  participants: z.array(z.object({ seat_no: z.number().int(), display_name: z.string(), name_origin: z.string(), initial_stack_units: integer, ending_stack_units: integer.nullable(), net_change_units: integer.nullable(), folded: z.boolean(), hole_cards: cards.optional(), public_hole_cards: cards.optional() })),
  actions: z.array(z.object({ sequence: integer, hand_version: integer, type: z.string(), street: z.string(), seat_no: z.number().int().optional(), delta_units: integer, to_units: integer, card: z.number().int().optional(), at: time })),
  next_cursor: z.string().optional(), pots: z.array(z.object({ index: z.number().int(), amount_units: integer, contribution_floor: integer, contribution_ceiling: integer, eligible_seats: z.array(z.number().int()), awards: z.array(z.object({ seat_no: z.number().int(), base_share_units: integer, odd_chip_units: integer, award_units: integer })) })),
  uncalled_returns: z.array(z.object({ seat_no: z.number().int(), amount_units: integer })), settlement: values.nullable(), fairness: values, read_at: time,
});

export type Summary = z.infer<typeof summarySchema>;
export type Transaction = z.infer<typeof transactionSchema>;
export type RecordType = typeof recordTypes[number];
export function detailPath(type: RecordType, id: string) {
  idSchema.parse(id);
  return `/history/${type === 'DIRECT_PLAY_ROUND' ? 'rounds' : type === 'POKER_SESSION' ? 'sessions' : 'hands'}/${id}`;
}
export class HistoryError extends Error {
  constructor(public status: number, public code: string) {
    super(status === 401 ? '登录已过期，请重新登录。' : status === 404 ? '记录不存在，或当前账户没有查看权限。' : status === 409 ? '本手记录已更新，请重新打开第一页。' : status === 400 ? '筛选条件无效，请检查时间范围与记录编号。' : '记录暂时读取失败，请稍后重试。');
  }
}
export async function historyGet<T>(client: ApiClient, path: string, schema: z.ZodType<T>, signal?: AbortSignal, beforeSend?: () => boolean): Promise<T> {
  if (!/^\/api\/v1\/history(?:[/?]|$)/.test(path)) throw new Error('History path invalid');
  const current = () => !signal?.aborted && (!beforeSend || beforeSend());
  let body: unknown;
  try {
    body = await client.request<unknown>(path, 'GET', undefined, undefined, current);
  } catch (error) {
    if (error instanceof ApiError) throw new HistoryError(error.status, error.code || 'HISTORY_UNAVAILABLE');
    throw error;
  }
  if (!current()) throw new HistoryError(0, 'REQUEST_SCOPE_EXPIRED');
  const parsed = schema.safeParse(body);
  if (!parsed.success) throw new HistoryError(503, 'HISTORY_RESPONSE_INVALID');
  return parsed.data;
}
