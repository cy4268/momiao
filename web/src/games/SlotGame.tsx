import { useEffect, useId, useRef, useState, type FormEvent } from 'react';
import { amountUnits } from '../economy-api';
import { integer } from '../wallet-api';
import './direct-extra-games.css';

export type SlotSymbol = 'L1' | 'L2' | 'L3' | 'M1' | 'M2' | 'H1' | 'H2' | 'W';
export interface SlotLineDTO { line_number: number; interpreted_symbol: SlotSymbol | ''; match_length: number; multiplier: number; line_stake_units: string; line_payout_units: string }
export interface SlotResultDTO {
    stops: number[]; full_grid: SlotSymbol[][]; lines: SlotLineDTO[];
    total_wager_units: string; line_stake_units: string; total_payout_units: string; net_change_units: string;
    result_class: 'LOSS' | 'BREAK_EVEN' | 'WIN'; result_detail: 'NO_WIN' | 'PARTIAL_RETURN' | 'BREAK_EVEN' | 'WIN';
}
export interface DirectExtraControls {
    availableUnits: string | null; wagerChips: string; onWagerChange: (text: string) => void;
    busy: boolean; error?: string | null; recovering?: boolean; disabledReason?: string | null; roundID?: string;
    onRecover?: () => void; onWallet?: () => void; onRewards?: () => void; onFairness?: () => void; onHistory?: () => void;
}
export interface SlotGameProps extends DirectExtraControls { result: SlotResultDTO | null; onSpin: (wagerUnits: string) => Promise<void> }

// The wallet already validates fixed-point amounts but has no units formatter.
// Keep all six decimal places exact; no monetary value passes through Number.
export function formatChipUnits(units: string | null, signed = false): string {
    if (!integer(units, true)) return '—';
    const n = BigInt(units); const micros = (n < 0n ? -n : n) * 2n;
    const fraction = String(micros % 1000000n).padStart(6, '0').replace(/0+$/, '');
    return `${n < 0n ? '-' : signed && n > 0n ? '+' : ''}${micros / 1000000n}${fraction ? `.${fraction}` : ''}`;
}
export function validWager(chips: string): bigint | null {
    if (!/^(0|[1-9]\d*)$/.test(chips)) return null;
    const units = amountUnits(chips); return units !== null && units >= 5000000n ? units : null;
}

// A request latch, not a client game state machine. Parents own reconciliation,
// account-scoped wager memory, idempotency keys, HTTP and authoritative DTOs.
export function useExtraCommand() {
    const latch = useRef(false); const alive = useRef(true);
    const [pending, setPending] = useState(false); const [failure, setFailure] = useState('');
    useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
    async function run(command: () => Promise<void>) {
        if (latch.current) return;
        latch.current = true; setPending(true); setFailure('');
        try { await command(); }
        catch { if (alive.current) setFailure('提交尚未确认，请先核对本局状态；不会自动重试。'); }
        finally { latch.current = false; if (alive.current) setPending(false); }
    }
    return { pending, failure, run };
}

export function ExtraWager({ controls, blocked, label, verb, onSubmit, lineStake = false }: { controls: DirectExtraControls; blocked: boolean; label: string; verb: string; onSubmit: (units: string) => void; lineStake?: boolean }) {
    const id = useId(); const units = validWager(controls.wagerChips);
    const balance = integer(controls.availableUnits) ? BigInt(controls.availableUnits) : null;
    const reason = balance === null ? '正在核对可用筹码。' : units === null ? '请输入不少于 10 的整数筹码。' : units > balance ? '当前可用筹码不足。' : '';
    function submit(event: FormEvent) { event.preventDefault(); if (!blocked && !reason && units !== null) onSubmit(String(units)); }
    return <form className="extra-wager" onSubmit={submit} noValidate>
        <label htmlFor={id}>{label}<input id={id} aria-describedby={`${id}-hint`} inputMode="numeric" autoComplete="off" maxLength={20} value={controls.wagerChips} disabled={blocked} onChange={e => controls.onWagerChange(e.target.value)} /></label>
        <div className="extra-quick" role="group" aria-label="只填入下注金额">{['10', '100', '500', '1000'].map(chips => <button key={chips} type="button" aria-label={`${chips} 筹码`} disabled={blocked || balance === null || BigInt(chips) * 500000n > balance} onClick={() => controls.onWagerChange(chips)}>{chips}</button>)}</div>
        <p id={`${id}-hint`} className="extra-note">{lineStake ? <>10 条线始终启用 · <span>每线 {units === null ? '—' : formatChipUnits(String(units / 10n))} 筹码</span></> : '最低 10 筹码 · 快捷金额只填入，不会自动开局。'}</p>
        <button className="extra-primary" type="submit" disabled={blocked || !!reason}>{verb} · {units === null ? '—' : formatChipUnits(String(units))} 筹码</button>
        {reason && <p className="extra-note" aria-live="polite">{reason}</p>}
        {balance !== null && balance < 5000000n && <div className="extra-links">{controls.onWallet && <button type="button" onClick={controls.onWallet}>前往钱包</button>}{controls.onRewards && <button type="button" onClick={controls.onRewards}>奖励中心</button>}</div>}
    </form>;
}

export function ExtraNotice({ controls, failure, message }: { controls: DirectExtraControls; failure: string; message: string }) {
    return <div className="extra-notices">
        <p className="extra-current" role="status">{message}</p>
        {(controls.error || failure) && <p className="extra-error" role="alert">{controls.error || failure}</p>}
        {controls.disabledReason && <p className="extra-note">{controls.disabledReason}</p>}
        {controls.onRecover && (controls.recovering || controls.error || failure) && <button type="button" disabled={controls.busy} onClick={controls.onRecover}>核对本局状态</button>}
    </div>;
}

const paylineRows = [[1,1,1,1,1],[0,0,0,0,0],[2,2,2,2,2],[0,1,2,1,0],[2,1,0,1,2],[0,0,1,2,2],[2,2,1,0,0],[1,0,0,0,1],[1,2,2,2,1],[2,1,1,1,0]];
const publicPaytable: [SlotSymbol, number, number, number][] = [['L1',4,15,50],['L2',8,25,80],['L3',10,40,150],['M1',15,60,250],['M2',25,100,500],['H1',50,250,1000],['H2',100,500,2500],['W',125,1000,5000]];
const paths: Record<SlotSymbol, string> = {
    L1: 'M18 4 29 18 18 32 7 18Z M18 10V26 M12 18H24',
    L2: 'M7 9H29V27H7Z M18 4V32 M4 18H32',
    L3: 'M18 4 32 29H4Z M18 13V23 M13 26H23',
    M1: 'M6 27V9L18 18 30 9V27Z M12 27V22 M24 27V22',
    M2: 'M18 3 23 12 33 18 23 24 18 33 13 24 3 18 13 12Z M13 18H23',
    H1: 'M6 12 12 18 18 7 24 18 30 12 27 28H9Z M10 32H26',
    H2: 'M7 30 29 8 M7 8 29 30 M4 4 10 10 M26 26 32 32 M4 32 10 26 M26 10 32 4',
    W: 'M18 2V7 M18 29V34 M2 18H7 M29 18H34 M6 6 10 10 M26 26 30 30 M6 30 10 26 M26 10 30 6 M18 10 26 18 18 26 10 18Z',
};
function Sigil({ symbol }: { symbol: SlotSymbol }) { return <svg viewBox="0 0 36 36" aria-hidden="true"><path d={paths[symbol]} /></svg>; }
const resultNames = { NO_WIN: '未中奖 · 净亏损', PARTIAL_RETURN: '部分返还 · 净亏损', BREAK_EVEN: '回本', WIN: '净盈利' };

export function SlotGame(props: SlotGameProps) {
    const command = useExtraCommand(); const title = useId(); const [selected, setSelected] = useState<number | null>(null); const [replay, setReplay] = useState(false);
    const result = props.result; const blocked = props.busy || command.pending || !!props.recovering || !!props.disabledReason;
    useEffect(() => { setSelected(null); setReplay(false); }, [result, props.roundID]);
    useEffect(() => { if (!replay) return; const timer = window.setTimeout(() => setReplay(false), 1500); return () => window.clearTimeout(timer); }, [replay]);
    const line = selected === null ? undefined : result?.lines.find(item => item.line_number === selected);
    return <section className="extra-game extra-slot" aria-labelledby={title} aria-busy={props.busy || command.pending}>
        <header className="extra-heading"><div><p className="extra-eyebrow">KING’S TREASURY / SLOT GALLERY</p><h2 id={title}>王之宝库 · Slot</h2><p>五轴三行，一眼看清整局结果。</p></div><div className="extra-balance"><span>可用筹码</span><strong>{formatChipUnits(props.availableUnits)}</strong></div></header>
        <div className="extra-layout">
            <div className={`extra-slot-stage${replay ? ' is-replaying' : ''}`}>
                <div className="extra-stage-label"><span>ROYAL TREASURY SIGILS</span><span>5 × 3 / 10 LINES</span></div>
                <div className="extra-reels" aria-label="服务端确认的五轴三行盘面">
                    {[0, 1, 2].flatMap(row => [0, 1, 2, 3, 4].map(reel => {
                        const symbol = result?.full_grid[reel]?.[row]; const active = selected !== null && paylineRows[selected - 1][reel] === row;
                        return <div data-testid="slot-cell" key={`${reel}-${row}`} className={`extra-sigil${active ? ' is-line' : ''}`} aria-label={`第 ${reel + 1} 轴，${['上', '中', '下'][row]}行：${symbol || '尚无结果'}`} style={{ animationDelay: `${reel * 120}ms` }}>{symbol ? <><Sigil symbol={symbol} /><span>{symbol === 'W' ? 'W · WILD' : symbol}</span></> : <span className="extra-unresolved">—</span>}</div>;
                    }))}
                    {selected !== null && <svg className="extra-line-overlay" viewBox="0 0 500 300" preserveAspectRatio="none" aria-hidden="true"><polyline points={paylineRows[selected - 1].map((row, reel) => `${reel * 100 + 50},${row * 100 + 50}`).join(' ')} /></svg>}
                </div>
                <div className="extra-line-markers" role="group" aria-label="查看固定中奖线">{paylineRows.map((_, i) => <button type="button" key={i} aria-label={`查看第 ${i + 1} 线`} aria-pressed={selected === i + 1} onClick={() => setSelected(selected === i + 1 ? null : i + 1)}>{i + 1}{result?.lines.some(item => item.line_number === i + 1 && item.multiplier > 0) && <span aria-label="有派彩">·</span>}</button>)}</div>
                <p className="extra-line-detail" role="status" aria-label="当前中奖线">{line ? `第 ${line.line_number} 线 · ${line.multiplier > 0 ? `${line.interpreted_symbol} · ${line.match_length} 连 · 每线 ${formatChipUnits(line.line_stake_units)} × ${line.multiplier} → ${formatChipUnits(line.line_payout_units)} 筹码` : '无派彩'}` : selected ? `第 ${selected} 线 · ${paylineRows[selected - 1].map(row => ['上', '中', '下'][row]).join('—')}` : '选择线号查看路径；十条线始终参与，非开关。'}</p>
                <div className="extra-replay"><span>{result ? 'Stop · ' + result.stops.join(' / ') : '等待第一局结果'}</span>{result && <button type="button" onClick={() => setReplay(value => !value)}>{replay ? 'Fast Stop · 直接显示结果' : '回看本局结果'}</button>}</div>
                {result && <p className="extra-stage-note">仅回看相同盘面与中奖线，不会创建新局或重复结算。</p>}
            </div>
            <aside className="extra-console">
                <ExtraWager controls={props} blocked={blocked} label="总下注 · 筹码" verb="Spin" lineStake onSubmit={units => void command.run(() => props.onSpin(units))} />
                <ExtraNotice controls={props} failure={command.failure} message={props.recovering ? '正在恢复同一局，保留已确认盘面。' : props.busy || command.pending ? '正在提交或核对结果，请稍候。' : result ? '本局结果已确认。下一局仍需主动 Spin。' : '选择总下注，点击 Spin 即提交本局。'} />
            </aside>
        </div>
        {result && <div className={`extra-result is-${result.result_class.toLowerCase()}`} role="status" aria-label="本局结算"><strong>{resultNames[result.result_detail]}</strong><dl><div><dt>本局总下注</dt><dd>{formatChipUnits(result.total_wager_units)}</dd></div><div><dt>总派彩（含返还）</dt><dd>{formatChipUnits(result.total_payout_units)}</dd></div><div><dt>净变化 · 筹码</dt><dd>{formatChipUnits(result.net_change_units, true)}</dd></div></dl></div>}
        <div className="extra-utilities"><details><summary>奖表与固定规则</summary><p>从左向右至少 3 连；Wild 替代普通符号或按自身奖表，只支付同线最高解释。倍数基于每线下注，不叠加同线 3 / 4 / 5 连。</p><table><caption>slot-paytable-v1 · 总派彩倍数</caption><thead><tr><th>符号</th><th>3 连</th><th>4 连</th><th>5 连</th></tr></thead><tbody>{publicPaytable.map(([symbol, ...values]) => <tr key={symbol}><th>{symbol}</th>{values.map((value, i) => <td key={i}>{value}×</td>)}</tr>)}</tbody></table><p>slot-strips-v1 / slot-paylines-v1。完整卷轴、配置及数学验证记录请查看公平详情。</p></details><div className="extra-links">{props.onFairness && <button type="button" onClick={props.onFairness}>公平详情</button>}{props.onHistory && <button type="button" onClick={props.onHistory}>本局记录</button>}</div>{props.roundID && <p className="extra-round-id">Round · {props.roundID}</p>}</div>
    </section>;
}
