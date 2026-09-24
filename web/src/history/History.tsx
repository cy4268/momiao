import { WalletChamber } from '../WalletChamber';
import { useEffect, useRef, useState, type CSSProperties, type FormEvent, type ReactNode } from 'react';
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom';
import { z } from 'zod';
import type { ApiClient } from '../api';
import { Alert, Empty, Loading, Modal } from '../ui';
import { transactionStatus } from '../economy-api';
import { chips } from '../games-api';
import { assetUrl } from '../game-hall-assets';
import diceArt from '../games/dice-art.json';
import { HistoryArchive, HistoryCover } from './HistoryArchive';
import { detailPath, handSchema, historyGet, idSchema, listQuery, pageSchema, readSearch, recordTypes, results, roundSchema, searchSchema, sessionSchema, statuses, transactionSchema, type Transaction } from './api';
import './history.css';

const names: Record<string, string> = { ...transactionStatus, ROULETTE_ROUND:"轮盘对局", ROULETTE:"多人轮盘", ROULETTE_ESCROW:"轮盘托管",ROULETTE_REFUND:"轮盘退款",ROULETTE_PAYOUT:"轮盘派彩",ROULETTE_VOID_REFUND:"轮盘作废退款", API_CHIPS_EXCHANGE: 'API → Chips', DIRECT_PLAY_ROUND: '单局游戏', POKER_SESSION: '牌桌会话', POKER_HAND: '单手牌局', DIRECT_PLAY: '直接游玩', POKER: '扑克', WIN: '净赢', LOSS: '净输', BREAK_EVEN: '回本', PROCESSING: '处理中', SETTLED: '已结算', CANCELLED: '已取消', REFUNDED: '已退款', RECOVERING: '恢复中', CONFIRMED: '已确认', BUY_IN: '买入', TOP_UP: '补充筹码', REBUY: '重新买入', CASH_OUT: '离桌结算', GAME_WAGER: '游戏下注', GAME_PAYOUT: '游戏派彩', POKER_CASH_OUT: '扑克离桌结算' };
const label = (value: string | null | undefined) => value ? names[value] || value : '未记录';
const money = (value: string | null | undefined, signed = false) => value == null ? '未记录' : chips(value, signed);
const when = (value: string | null | undefined) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '未记录';
type HistoryProps = { client: ApiClient };
export function useHistory<T>(client: ApiClient, path: string, schema: z.ZodType<T>) {
  const [version, setVersion] = useState(0);
  const [state, setState] = useState<{ data?: T; error?: string; loading: boolean }>({ loading: true });
  useEffect(() => {
    const controller = new AbortController();
    const generation = client.getSessionGeneration();
    const current = () => !controller.signal.aborted && client.getSessionGeneration() === generation;
    setState({ loading: true });
    historyGet(client, path, schema, controller.signal, current).then(data => {
      if (current()) setState({ data, loading: false });
    }).catch(error => {
      if (current()) setState({ error: error instanceof Error ? error.message : '记录读取失败。', loading: false });
    });
    return () => controller.abort();
  }, [client, path, schema, version]);
  return { ...state, reload: () => setVersion(v => v + 1) };
}
function Fields({ values }: { values: [string, ReactNode][] }) {
  return <dl className="history-facts">{values.map(([name, value]) => <div key={name}><dt>{name}</dt><dd>{value}</dd></div>)}</dl>;
}
function Status({ loading, error, reload }: { loading: boolean; error?: string; reload: () => void }) {
  return loading ? <Loading /> : error ? <Alert>{error} <button onClick={reload}>重试读取</button></Alert> : null;
}
function safeReturn(state: unknown): { search: string; scroll: number; focus: string } {
  const parsed = z.object({ historyReturn: z.object({ search: z.string(), scroll: z.number().finite().min(0).max(10000000), focus: z.string() }) }).safeParse(state);
  if (parsed.success) {
    try { readSearch(parsed.data.historyReturn.search); return parsed.data.historyReturn; } catch { /* Invalid UI state is discarded. */ }
  }
  return { search: '', scroll: 0, focus: '' };
}
export function Back({ parent }: { parent?: { path: string; title: string } }) {
  const location = useLocation(), saved = safeReturn(location.state);
  const parentSearch = z.object({ parentSessionSearch: z.string() }).safeParse(location.state);
  let suffix = '';
  if (parentSearch.success) { try { detailSearch(parentSearch.data.parentSessionSearch, 'sessions'); suffix = parentSearch.data.parentSessionSearch; } catch { /* Discard invalid navigation state. */ } }
  return <nav className="history-back" aria-label="历史记录层级">{parent && <Link to={parent.path + suffix} state={location.state}>← {parent.title}</Link>}<Link to={'/history' + saved.search} state={{ historyReturn: saved }}>← 返回筛选列表</Link></nav>;
}
function Title({ title, detail }: { title: string; detail: string }) {
  return <header className="page-heading"><div><p className="eyebrow">YOUR RECORDS / CHALDEA</p><h1>{title}</h1><p>{detail}</p></div><Link className="button" to="/games">浏览游戏 →</Link></header>;
}

export function HistoryList({ client }: HistoryProps) {
  const location = useLocation();
  try { readSearch(location.search); } catch { return <HistoryArchive title="游戏记录" detail="查看你自己的游戏记录。"><Alert>筛选链接无效，请检查记录编号与时间范围。<Link to="/history">清除筛选</Link></Alert></HistoryArchive>; }
  return <HistoryListContent key={location.search} client={client} />;
}
function HistoryListContent({ client }: HistoryProps) {
  const location = useLocation(), navigate = useNavigate(), q = readSearch(location.search);
  const data = useHistory(client, '/api/v1/history?' + listQuery(location.search), pageSchema);
  const [filtersOpen, setFiltersOpen] = useState(false), [error, setError] = useState('');
  const restored = useRef(false);
  const records = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!data.data || restored.current) return;
    restored.current = true;
    const saved = safeReturn(location.state);
    const frame = requestAnimationFrame(() => {
      const filterFocus = z.object({ historyFilterFocus: z.enum(['more', 'record_type', 'game_slug']) }).safeParse(location.state);
      if (filterFocus.success) document.getElementById('archive-filter-' + filterFocus.data.historyFilterFocus)?.focus({ preventScroll: true });
      if (saved.focus) document.getElementById('history-' + saved.focus)?.focus({ preventScroll: true });
      if (records.current) records.current.scrollTop = saved.scroll;
      window.scrollTo(0, saved.scroll);
    });
    return () => cancelAnimationFrame(frame);
  }, [data.data, location.state]);
  function filter(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const values = Object.fromEntries(new FormData(event.currentTarget));
    const draft: Record<string, string> = {};
    for (const [key, value] of Object.entries(values)) {
      if (typeof value !== 'string' || value === '') continue;
      if (key === 'time_from' || key === 'time_to') {
        const at = new Date(value);
        if (!Number.isFinite(at.getTime())) { setError('时间格式无效。'); return; }
        draft[key] = at.toISOString();
      } else draft[key] = value.trim();
    }
    const parsed = searchSchema.safeParse(draft);
    if (!parsed.success) { setError('请检查记录编号，并确保开始时间早于结束时间。'); return; }
    setError(''); setFiltersOpen(false); navigate('/history?' + new URLSearchParams(draft), { state: { historyFilterFocus: 'more' } });
  }
  function next(cursor: string) {
    const params = new URLSearchParams(location.search); params.set('cursor', cursor);
    navigate('/history?' + params, { state: { previousHistory: location.search } });
  }
  function quickFilter(key: 'record_type' | 'game_slug', value: string) {
    const params = new URLSearchParams(location.search);
    params.delete('cursor');
    if (value) params.set(key, value); else params.delete(key);
    navigate('/history' + (params.size ? '?' + params : ''), { state: { historyFilterFocus: key } });
  }
  const previous = z.object({ previousHistory: z.string() }).safeParse(location.state);
  const previousSearch = previous.success ? previous.data.previousHistory : '';
  const filterForm = <form onSubmit={filter} className="history-filter-form">{error && <Alert>{error}</Alert>}
    <label>记录类型<select name="record_type" defaultValue={q.record_type || ''}><option value="">全部记录（单局与会话）</option>{recordTypes.map(v => <option key={v} value={v}>{label(v)}</option>)}</select></label>
    <label>游玩模式<select name="mode" defaultValue={q.mode || ''}><option value="">全部模式</option><option value="DIRECT_PLAY">直接游玩</option><option value="POKER">扑克</option><option value="ROULETTE">多人轮盘</option></select></label>
    <label>游戏<select name="game_slug" defaultValue={q.game_slug || ''}><option value="">全部游戏</option>{q.game_slug && !data.data?.game_options.some(g => g.game_slug === q.game_slug) && <option value={q.game_slug}>{q.game_slug}</option>}{data.data?.game_options.map(g => <option key={g.game_slug} value={g.game_slug}>{g.game_title}{g.retired ? '（已退役）' : ''}</option>)}</select></label>
    <label>开始时间（本地）<input name="time_from" type="datetime-local" defaultValue={localTime(q.time_from)} /></label><label>结束时间（不含，本地）<input name="time_to" type="datetime-local" defaultValue={localTime(q.time_to)} /></label>
    <label>结果<select name="result" defaultValue={q.result || ''}><option value="">全部结果</option>{results.map(v => <option key={v} value={v}>{label(v)}</option>)}</select></label>
    <label>状态<select name="status" defaultValue={q.status || ''}><option value="">全部状态</option>{statuses.map(v => <option key={v} value={v}>{label(v)}</option>)}</select></label>
    <label className="history-filter-id">精确记录编号<input name="id" defaultValue={q.id || ''} maxLength={36} placeholder="完整的 Round / Session / Hand ID" /></label>
    <div className="history-filter-actions"><button className="primary" type="submit">应用筛选</button><Link to="/history" state={{ historyFilterFocus: 'more' }} onClick={() => setFiltersOpen(false)}>清除筛选</Link></div></form>;
  return <HistoryArchive list title="游戏记录" detail="每一局经历，皆有迹可循。">
    <section className="panel history-list-panel"><div className="archive-toolbar">
      <label><span className="sr-only">快速筛选记录类型</span><select id="archive-filter-record_type" value={q.record_type || ''} onChange={e => quickFilter('record_type', e.target.value)}><option value="">全部记录</option>{recordTypes.map(v => <option key={v} value={v}>{label(v)}</option>)}</select></label>
      <label><span className="sr-only">快速筛选游戏</span><select id="archive-filter-game_slug" value={q.game_slug || ''} onChange={e => quickFilter('game_slug', e.target.value)}><option value="">全部游戏</option>{q.game_slug && !data.data?.game_options.some(g => g.game_slug === q.game_slug) && <option value={q.game_slug}>{q.game_slug}</option>}{data.data?.game_options.map(g => <option key={g.game_slug} value={g.game_slug}>{g.game_title}{g.retired ? '（已退役）' : ''}</option>)}</select></label>
      <button id="archive-filter-more" onClick={() => setFiltersOpen(true)}>更多筛选{Object.keys(q).some(key => !['record_type', 'game_slug', 'cursor'].includes(key)) ? ' · 已应用' : ''}</button><button disabled={data.loading} onClick={data.reload}>刷新记录</button>
    </div>{filtersOpen && <Modal title="筛选游戏记录" onClose={() => setFiltersOpen(false)}>{filterForm}</Modal>}
      <Status {...data} /><div className="history-records" role="region" aria-label="历史记录列表" tabIndex={0} ref={records}>{data.data && (data.data.items.length ? data.data.items.map(item => <article className="history-record" key={item.record_type + item.source_id}>
        <div className="archive-record-title"><HistoryCover game={item.game_slug} /><div><small>{label(item.record_type)} · {label(item.status)}</small><h3>{item.snapshot.game_title}</h3><time>{when(item.occurred_at)}</time><p>{item.snapshot.table_name || label(item.mode)} · {item.snapshot.actor_display_name ?? '未记录昵称'}</p><code title={item.source_id}>{item.source_id}</code></div></div>
        <Fields values={item.record_type === 'POKER_SESSION' ? [['初始买入', money(item.initial_buyin_units)], ['累计补充', money(item.total_topup_units)], ['离桌返还', money(item.final_cashout_units)], ['净变化', money(item.net_change_units, true)]] : [['下注', money(item.stake_units)], ['派彩', money(item.payout_units)], ['净变化', money(item.net_change_units, true)], ['结果', label(item.result)]]} />
        <Link id={'history-' + item.source_id} to={detailPath(item.record_type, item.source_id)} state={{ historyReturn: { search: location.search, scroll: records.current?.scrollTop || window.scrollY, focus: item.source_id } }} onClick={event => { const saved = { historyReturn: { search: location.search, scroll: records.current?.scrollTop || window.scrollY, focus: item.source_id } }; if (!event.ctrlKey && !event.metaKey && !event.shiftKey && !event.altKey && event.button === 0) { event.preventDefault(); navigate(detailPath(item.record_type, item.source_id), { state: saved }); } }}>查看详情 →</Link>
      </article>) : <Empty title="没有符合条件的记录">调整筛选，或到游戏目录开始游玩。读取失败不会显示成空记录。</Empty>)}</div>
      <nav className="pager" aria-label="历史记录分页"><button disabled={data.loading || !q.cursor} onClick={() => { try { readSearch(previousSearch); navigate('/history' + previousSearch); } catch { navigate('/history'); } }}>{previous.success ? '上一页' : '返回第一页'}</button><button disabled={data.loading || !data.data?.has_more || !data.data.next_cursor} onClick={() => next(data.data!.next_cursor!)}>下一页</button></nav>
      <p className="muted">金额单位为娱乐筹码；会话损益与其中手牌不重复累加。退役游戏与关闭牌桌的记录仍会保留。</p>
    </section></HistoryArchive>;
}
function localTime(value?: string) {
  if (!value) return '';
  const date = new Date(value); return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
}

function Transactions({ items }: { items: Transaction[] }) {
  const location = useLocation();
  return <section className="history-section"><h2>钱包交易</h2>{items.length === 0 ? <p>没有已确认的交易记录。</p> : items.map(tx => <details className="history-transaction" id={'transaction-' + tx.id} key={tx.id}><summary>{label(tx.kind)} · {label(tx.status)} · {when(tx.confirmed_at)}</summary><code>{tx.id}</code>{tx.kind==='API_CHIPS_EXCHANGE' && <p>固定来源：Reserve {tx.reserve_debit_units} + Active {tx.active_debit_units} → Chips {tx.chips_credit_units}（原始单位）</p>}{tx.native_effects?.map((effect,index)=><Fields key={index} values={[["Active 变化",effect.delta_units+' 原始单位'],['变化前',effect.before_units],['变化后',effect.after_units]]}/>)}
    {tx.effects.length === 0 ? <p>{tx.kind==='API_CHIPS_EXCHANGE'?'当前尚无已入账的本地流水；请以兑换状态核对结果。':'此交易已确认，资产变化为零，没有钱包流水腿；不推算当前余额。'}</p> : tx.effects.map(effect => <Fields key={effect.ledger_id} values={[[`资产 · ${effect.asset}`, effect.delta_units + ' 原始单位'], ['变化前', effect.balance_before_units], ['变化后', effect.balance_after_units], ['流水编号', effect.ledger_id]]} />)}
    <Link to={'/wallet/transactions/' + tx.id} state={location.state}>打开钱包交易详情 →</Link><nav aria-label="交易关联记录">{tx.links.map(link => <Link key={link.record_type + link.source_id} to={detailPath(link.record_type, link.source_id)} state={location.state}>{label(link.record_type)} · {link.source_id}</Link>)}</nav>
  </details>)}</section>;
}
export function HistoryTransaction({ client }: HistoryProps) {
  const { id = '' } = useParams();
  if (!idSchema.safeParse(id).success) return <Alert>交易编号无效。<Link to="/wallet">返回钱包</Link></Alert>;
  return <TransactionContent key={id} id={id} client={client} />;
}
function TransactionContent({ id, client }: { id: string; client: ApiClient }) {
  const read = useHistory(client, '/api/v1/history/transactions/' + id, transactionSchema);
  return <WalletChamber page="transaction"><header className="page-heading"><div><p className="eyebrow">WALLET / TRANSACTION</p><h1>钱包交易详情</h1><p className="transaction-id">{id}</p></div><Link className="button" to="/wallet">← 返回钱包</Link></header><Back /><Status {...read} />{read.data && <section className="panel"><Fields values={[["交易类型", label(read.data.kind)], ['状态', label(read.data.status)], ['创建时间', when(read.data.created_at)], ['确认时间', when(read.data.confirmed_at)]]} /><Transactions items={[read.data]} /></section>}</WalletChamber>;
}
function Snapshot({ data }: { data: z.infer<typeof roundSchema>['metadata'] }) {
  return <p className="history-snapshot">当时昵称：{data.actor_display_name ?? '未记录'} · 快照来源：{data.metadata_origin} · {when(data.captured_at)}</p>;
}
const proofSchema = z.record(z.string(), z.json());
function Proof({ client, path }: { client: ApiClient; path: string }) {
  const [open, setOpen] = useState(false);
  return <details className="history-disclosure" onToggle={event => setOpen(event.currentTarget.open)}><summary>公平性记录</summary><p>仅展示服务端按公开时点与参与资格允许读取的内容。</p>{open && <ProofContent client={client} path={path} />}</details>;
}
function ProofContent({ client, path }: { client: ApiClient; path: string }) {
  const proof = useHistory(client, path + '/verify', proofSchema);
  return <><Status {...proof} />{proof.data && <><Fields values={Object.entries(proof.data).filter(([key]) => ['released', 'reveal_state', 'reveal_at', 'verified', 'server_seed_hash', 'deck_hash', 'algorithm_version', 'server_seed', 'effective_client_seed'].includes(key)).map(([key, value]) => [key, String(value ?? '未记录')])} /><details><summary>查看已获准的复算输入</summary><pre className="history-proof">{JSON.stringify(proof.data, null, 2)}</pre></details></>}</>;
}
function RecordedValues({ value, title }: { value: unknown; title: string }) {
  if (value == null) return <p>{title}：未记录</p>;
  if (Array.isArray(value)) return <div className="history-recorded-array" aria-label={title}>{value.map((entry, index) => <RecordedValues key={index} value={entry} title={`${title} ${index + 1}`} />)}</div>;
  if (typeof value === 'object') return <dl className="history-recorded-values">{Object.entries(value).map(([key, entry]) => <div key={key}><dt>{key}</dt><dd><RecordedValues value={entry} title={key} /></dd></div>)}</dl>;
  return <span>{typeof value === 'boolean' ? value ? '是' : '否' : String(value)}</span>;
}
const cursorValue = z.string().min(1).max(2048).optional();
const sessionSearch = z.strictObject({ funding_cursor: cursorValue, hand_cursor: cursorValue });
const handSearch = z.strictObject({ cursor: cursorValue });
function detailSearch(raw: string, kind: 'sessions' | 'hands') {
  const input: Record<string, string> = {};
  for (const [key, value] of new URLSearchParams(raw)) { if (key in input) throw new Error('duplicate'); input[key] = value; }
  return kind === 'sessions' ? sessionSearch.parse(input) : handSearch.parse(input);
}
function DetailQueryGuard({ kind, children }: { kind: 'sessions' | 'hands'; children: ReactNode }) {
  const location = useLocation();
  try { detailSearch(location.search, kind); } catch { return <Alert>详情分页链接无效。<Link to={location.pathname} state={location.state}>重新打开第一页</Link></Alert>; }
  return children;
}
function useDetailPaging(kind: 'sessions' | 'hands') {
  const location = useLocation(), navigate = useNavigate();
  const query = detailSearch(location.search, kind);
  const params = new URLSearchParams(query as Record<string, string>);
  function advance(key: string, value?: string) {
    const next = new URLSearchParams(query as Record<string, string>);
    if (value) next.set(key, value); else next.delete(key);
    navigate(location.pathname + (next.size ? '?' + next : ''), { state: location.state });
  }
  return { params, advance };
}
function URLCursorButtons({ cursor, next, change, title, disabled }: { cursor: string | null; next?: string; change: (cursor?: string) => void; title: string; disabled: boolean }) {
  return <nav className="pager" aria-label={title + '分页'}><button disabled={disabled || !cursor} onClick={() => change()}>返回第一页</button><button disabled={disabled || !next} onClick={() => change(next)}>下一页</button></nav>;
}
function CardNames({ cards }: { cards: number[] }) {
  const ranks = ['A', '2', '3', '4', '5', '6', '7', '8', '9', '10', 'J', 'Q', 'K'];
  const suits = ['梅花', '方块', '红桃', '黑桃'];
  return <span>{cards.map(card => card >= 0 && card < 52 ? suits[Math.floor(card / 13)] + ranks[card % 13] : '牌编号无效').join(' · ')}</span>;
}
const recordedDice = z.object({ dice: z.array(z.number().int().min(1).max(6)).length(3), total: z.number().int() }).refine(d => d.dice.reduce((a, b) => a + b, 0) === d.total);
const dicePips: Record<number, number[]> = { 1: [5], 2: [1, 9], 3: [1, 5, 9], 4: [1, 3, 7, 9], 5: [1, 3, 5, 7, 9], 6: [1, 3, 4, 6, 7, 9] };
function RoundResult({ round }: { round: z.infer<typeof roundSchema> }) {
  const dice = recordedDice.safeParse(round.game === 'dice' ? round.dice : undefined);
  const recorded = <RecordedValues title="游戏结果" value={round.dice || round.scratch || round.summon || round.slot || round.blackjack} />;
  return <section className="history-section archive-result"><h2>本局结果</h2>{dice.success ? <>
    <div className="archive-dice" aria-label={`骰子点数 ${dice.data.dice.join('、')}，合计 ${dice.data.total} 点`} style={{ '--archive-die-face': `url("${assetUrl(diceArt.face.src)}")` } as CSSProperties}>
      {dice.data.dice.map((face, index) => <div className="archive-die" key={index} aria-hidden="true">{Array.from({ length: 9 }, (_, pip) => <i className={dicePips[face].includes(pip + 1) ? 'is-pip' : ''} key={pip} />)}</div>)}
    </div><p className="archive-dice-total">{dice.data.dice.join(' + ')} = {dice.data.total} 点</p>
    <details className="history-disclosure"><summary>完整结果记录</summary>{recorded}</details>
  </> : recorded}</section>;
}
export function HistoryRound({ client }: HistoryProps) {
  const { id = '' } = useParams();
  if (!idSchema.safeParse(id).success) return <Alert>记录编号无效。<Link to="/history">返回列表</Link></Alert>;
  return <RoundContent key={id} id={id} client={client} />;
}
function RoundContent({ id, client }: { id: string; client: ApiClient }) {
  const path = '/api/v1/history/rounds/' + id, read = useHistory(client, path, roundSchema), d = read.data;
  return <HistoryArchive title={d?.metadata.game_title || '本局详情'} detail={'Round · ' + id} navigation={<Back />}><Status {...read} />{d && <section className="panel archive-detail-panel">
    <div className="archive-round-heading"><HistoryCover game={d.game} /><div><h2>本局详情 · {label(d.common_result)}</h2><p>{label(d.state)} · {when(d.created_at)}</p></div></div>
    <div className="archive-money"><Fields values={[["总下注", money(d.total_stake_units)], ['总派彩', money(d.total_payout_units)], ['净变化', money(d.net_change_units, true)]]} /></div>
    <RoundResult round={d} />
    <details className="history-disclosure"><summary>本局输入与版本记录</summary><Snapshot data={d.metadata} /><Fields values={[["状态", label(d.state)], ['恢复状态', d.recovery_state], ['开始时间', when(d.created_at)], ['结算时间', when(d.settled_at)]]} /><RecordedValues title="输入" value={d.input} /><RecordedValues title="公平性承诺" value={d.fairness} /></details>
    <details className="history-disclosure"><summary>钱包交易</summary><Transactions items={d.transactions} /></details><Proof client={client} path={path} /><Link className="archive-entry" to={'/games/' + encodeURIComponent(d.game)}>查看游戏入口 →</Link>
  </section>}</HistoryArchive>;
}
export function HistorySession({ client }: HistoryProps) {
  const { id = '' } = useParams();
  if (!idSchema.safeParse(id).success) return <Alert>会话编号无效。<Link to="/history">返回列表</Link></Alert>;
  return <DetailQueryGuard kind="sessions"><SessionContent key={id} id={id} client={client} /></DetailQueryGuard>;
}
function SessionContent({ id, client }: { id: string; client: ApiClient }) {
  const location = useLocation(), { params, advance } = useDetailPaging('sessions');
  const [fundingOpen, setFundingOpen] = useState(params.has('funding_cursor'));
  params.set('funding_limit', '25'); params.set('hand_limit', '25');
  const read = useHistory(client, '/api/v1/history/sessions/' + id + '?' + params, sessionSchema), d = read.data;
  return <HistoryArchive title={d?.metadata.table_name || '牌桌会话'} detail={'Session · ' + id} navigation={<Back />}><Status {...read} />{d && <section className="panel archive-detail-panel"><Snapshot data={d.metadata} />
    <Fields values={[["状态", label(d.state)], ['座位', d.seat_no], ['开始时间', when(d.started_at)], ['结束时间', when(d.ended_at)], ['结束原因', label(d.end_reason)], ['初始买入', money(d.initial_buyin_units)], ['补充筹码', money(d.confirmed_topup_units)], ['重新买入', money(d.confirmed_rebuy_units)], ['离桌返还', money(d.final_cashout_units)], ['已实现损益', money(d.realized_pl_units, true)]]} />
    <details className="history-disclosure" open={fundingOpen} onToggle={e => setFundingOpen(e.currentTarget.open)}><summary>资金操作与钱包交易 · {d.funding_count} 笔</summary>{d.funding.map(f => <article className="history-funding" key={f.funding_operation_id}><strong>{label(f.kind)} · {money(f.amount_units)} 筹码</strong><p>{label(f.state)} · {when(f.created_at)}</p>{f.failure_code && <p>未确认原因：{f.failure_code}</p>}{f.transaction ? <Transactions items={[f.transaction]} /> : <p>尚无已确认钱包交易，不展示计划交易回链。</p>}</article>)}<URLCursorButtons cursor={params.get('funding_cursor')} change={cursor => { setFundingOpen(true); advance('funding_cursor', cursor); }} next={d.next_funding_cursor} disabled={read.loading} title="资金操作" /></details>
    <section className="history-section"><h2>手牌 · {d.hand_count} 手</h2>{d.hands.length ? <ol className="history-hand-list">{d.hands.map(hand => <li key={hand.hand_id}><Link to={detailPath('POKER_HAND', hand.hand_id)} state={{ ...location.state, parentSessionSearch: location.search }}>第 {hand.hand_no} 手 · {label(hand.state)} →</Link><time>{when(hand.created_at)}</time></li>)}</ol> : <p>本页没有手牌记录。</p>}<URLCursorButtons cursor={params.get('hand_cursor')} change={cursor => advance('hand_cursor', cursor)} next={d.next_hand_cursor} disabled={read.loading} title="手牌" /></section>
    <details><summary>当时的牌桌配置</summary><RecordedValues title="配置" value={d.configuration} /></details><p>会话损益以资金结算为准，不再加总其中手牌损益。</p><Link to={'/poker/table/' + d.table_id}>查看牌桌入口 →</Link>
  </section>}</HistoryArchive>;
}
export function HistoryHand({ client }: HistoryProps) {
  const { id = '' } = useParams();
  if (!idSchema.safeParse(id).success) return <Alert>手牌编号无效。<Link to="/history">返回列表</Link></Alert>;
  return <DetailQueryGuard kind="hands"><HandContent key={id} id={id} client={client} /></DetailQueryGuard>;
}
function HandContent({ id, client }: { id: string; client: ApiClient }) {
  const { params, advance } = useDetailPaging('hands');
  const path = '/api/v1/history/hands/' + id;
  params.set('limit', '50');
  const read = useHistory(client, path + '?' + params, handSchema), d = read.data;
  return <HistoryArchive title={d ? `第 ${d.hand_no} 手` : '手牌详情'} detail={'Hand · ' + id} navigation={<Back parent={d ? { path: detailPath('POKER_SESSION', d.session_id), title: '所属会话' } : undefined} />}><Status {...read} />{read.error && params.has('cursor') && <button onClick={() => advance('cursor')}>重新读取第一页</button>}{d && <section className="panel archive-detail-panel"><Snapshot data={d.metadata} />
    <Fields values={[["状态", label(d.state)], ['开始时间', when(d.created_at)], ['结算时间', when(d.settled_at)], ['我的座位', d.seat_no], ['按钮位', d.button_seat], ['公开公共牌', d.board_cards.length ? <CardNames cards={d.board_cards} /> : '尚未发出']]} />
    <section className="history-section"><h2>参与者</h2>{d.participants.map(p => <article className="history-participant" key={p.seat_no}><h3>座位 {p.seat_no} · {p.display_name}</h3><p>昵称来源：{p.name_origin} · {p.folded ? '已弃牌' : '未弃牌'}</p><Fields values={[["起始筹码", money(p.initial_stack_units)], ['结束筹码', money(p.ending_stack_units)], ['净变化', money(p.net_change_units, true)], ['可见底牌', p.hole_cards?.length ? <CardNames cards={p.hole_cards} /> : p.public_hole_cards?.length ? <CardNames cards={p.public_hole_cards} /> : '未公开']]} /></article>)}</section>
    <section className="history-section"><h2>行动时间线</h2><ol className="history-timeline">{d.actions.map(a => <li key={a.sequence}><strong>{a.street} · {a.type}</strong><span>{a.seat_no === undefined ? '公共事件' : `座位 ${a.seat_no}`} · {money(a.delta_units)} 筹码 · 累计至 {money(a.to_units)}</span>{a.card !== undefined && <CardNames cards={[a.card]} />}<time>{when(a.at)}</time></li>)}</ol><URLCursorButtons cursor={params.get('cursor')} change={cursor => advance('cursor', cursor)} next={d.next_cursor} disabled={read.loading} title="行动" /></section>
    <section className="history-section"><h2>底池与结算</h2>{d.pots.map(p => <article key={p.index}><h3>{p.index === 0 ? '主池' : `边池 ${p.index}`} · {money(p.amount_units)} 筹码</h3><p>有资格座位：{p.eligible_seats.join('、')}</p><RecordedValues title="分配" value={p.awards} /></article>)}<RecordedValues title="未跟注退回" value={d.uncalled_returns} /><RecordedValues title="结算" value={d.settlement} /></section>
    <details><summary>当时的牌桌配置</summary><RecordedValues title="配置" value={d.configuration} /></details><Proof client={client} path={path} /><Link to={'/poker/table/' + d.table_id}>查看牌桌入口 →</Link>
  </section>}</HistoryArchive>;
}
