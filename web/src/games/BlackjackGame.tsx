import { useId, useState, type CSSProperties } from 'react';
import { integer } from '../wallet-api';
import { ExtraNotice, ExtraWager, formatChipUnits, useExtraCommand, type DirectExtraControls } from './SlotGame';
import { assetUrl, assetSrcSet, type ImageAsset } from '../game-hall-assets';
import pokerArt from '../poker/poker-art.json';
import art from './blackjack-art.json';
import './direct-extra-games.css';
import './blackjack-stage.css';

export type BlackjackActionType = 'HIT' | 'STAND' | 'DOUBLE' | 'SPLIT';
export interface BlackjackValueDTO { hard_total: number; best_total: number; is_soft: boolean }
export interface BlackjackHandDTO {
    hand_id: string; hand_index: number; cards: number[]; stake_units: string;
    hand_state: 'ACTIVE' | 'STOOD' | 'BUST' | 'DOUBLED_COMPLETE' | 'SPLIT_ACES_COMPLETE' | 'NATURAL_COMPLETE';
    value: BlackjackValueDTO; is_natural: boolean; result?: string; payout_units: string; net_change_units: string;
}
export interface BlackjackProjectionDTO {
    phase: 'PLAYER_TURN' | 'SETTLED'; round_version: string; active_hand_id: string; hands: BlackjackHandDTO[];
    dealer_cards: number[]; dealer_revealed: boolean; dealer_total?: BlackjackValueDTO;
    legal_actions: BlackjackActionType[] | null; last_player_action_at: string; auto_resolve_at: string;
    total_stake_units: string; total_payout_units: string; fair_return_units?: string; net_change_units: string; result_class?: 'LOSS' | 'BREAK_EVEN' | 'WIN';
}
export interface BlackjackCommand { hand_id: string; action_type: BlackjackActionType; expected_round_version: string }
export interface BlackjackGameProps extends DirectExtraControls {
    snapshot: BlackjackProjectionDTO | null; autoResolving?: boolean; salon?: boolean; onRules?: () => void;
    onDeal: (wagerUnits: string) => Promise<void>; onAction: (action: BlackjackCommand) => Promise<void>;
}

const ranks = ['A', '2', '3', '4', '5', '6', '7', '8', '9', '10', 'J', 'Q', 'K'];
const suits = ['♣', '♦', '♥', '♠']; const suitNames = ['梅花', '方块', '红桃', '黑桃'];
function BlackjackArt({ asset, className }: { asset: ImageAsset; className: string }) {
    return <img className={className} src={assetUrl(asset.src)} srcSet={assetSrcSet(asset)} sizes={className === 'blackjack-table-art' ? '80vw' : '160px'} width={asset.width} height={asset.height} alt="" decoding="async" />;
}
function PlayingCard({ code, salon }: { code: number; salon?: boolean }) {
    const suit = Math.floor(code / 13); const rank = ranks[code % 13]; const value = code % 13 + 1;
    return <span className={`extra-playing-card${suit === 1 || suit === 2 ? ' is-red' : ''}`} data-testid="playing-card" role="img" aria-label={`${suitNames[suit]} ${rank}`}>
        <span className="bj-corner">{rank}<small>{suits[suit]}</small></span>{salon && <span className="bj-corner bj-corner-bottom" aria-hidden="true">{rank}<small>{suits[suit]}</small></span>}
        <b className={`${salon ? 'bj-pips' : ''}${value > 10 ? ' is-face' : ''}`} aria-hidden="true" data-value={value}>{!salon ? suits[suit] : value > 10 ? <>{rank}<small>{suits[suit]}</small></> : Array.from({ length: value }, (_, i) => <i key={i}>{suits[suit]}</i>)}</b>
    </span>;
}
export function BlackjackRules() {
    return <div className="blackjack-rules"><p>每局重新确定六副牌牌序；美式暗牌与预检查；庄家软 17 停牌（S17）。原始未分牌的两张 Natural Blackjack 支付 3:2；分牌后的 21 为普通 21。</p><p>允许任意两张加倍、分牌后加倍（DAS）、同点值分牌及非 A 再分，最多 4 手。分 A 后每手仅补一张并自动完成。无保险、投降、边注或策略提示。结算时按初始下注返还 0.37079%，原子单位尾数由独立公平随机流取整，使已验证参考策略的长期净期望为 0；具体操作仍会影响个人结果。</p></div>;
}

const handStates = { ACTIVE: '等待行动', STOOD: '已停牌', BUST: '爆牌', DOUBLED_COMPLETE: '加倍已完成', SPLIT_ACES_COMPLETE: '分 A 后自动完成', NATURAL_COMPLETE: 'Natural Blackjack' };
const handResults: Record<string, string> = { LOSS: '落败', BUST: '爆牌', PUSH: '平局返还', NORMAL_WIN: '普通获胜', NATURAL: 'Natural Blackjack' };

export function BlackjackGame(props: BlackjackGameProps) {
    const title = useId(); const command = useExtraCommand(); const [inspection, setInspection] = useState<{ handID: string; version: string } | null>(null);
    const s = props.snapshot; const hands = [...(s?.hands || [])].sort((a, b) => a.hand_index - b.hand_index);
    const active = hands.find(hand => hand.hand_id === s?.active_hand_id); const activeOrdinal = hands.findIndex(hand => hand.hand_id === s?.active_hand_id) + 1;
    const inspected = inspection && inspection.version === s?.round_version && hands.some(hand => hand.hand_id === inspection.handID) ? inspection.handID : active?.hand_id || hands[0]?.hand_id;
    const blocked = props.busy || command.pending || !!props.recovering || !!props.autoResolving || !!props.disabledReason;
    const balance = integer(props.availableUnits) ? BigInt(props.availableUnits) : null;
    const stake = active && integer(active.stake_units) ? BigInt(active.stake_units) : null;
    const canAdd = balance !== null && stake !== null && balance >= stake;
    const legal = s?.legal_actions || [];
    function act(type: BlackjackActionType) {
        if (blocked || !s || !active || s.phase !== 'PLAYER_TURN' || !legal.includes(type) || ((type === 'DOUBLE' || type === 'SPLIT') && !canAdd)) return;
        void command.run(() => props.onAction({ hand_id: active.hand_id, action_type: type, expected_round_version: s.round_version }));
    }
    return <section className={`extra-game extra-blackjack${props.salon ? ' blackjack-salon' : ''}`} aria-labelledby={title} aria-busy={props.busy || command.pending}>
        <header className="extra-heading"><div><p className="extra-eyebrow">CASINO CAMELOT / VIP ROYAL TABLE</p><h2 id={title}>皇家牌桌 · Blackjack</h2><p>同一副牌序，继续你亲手作出的选择。</p></div><div className="extra-balance"><span>可用筹码</span><strong>{formatChipUnits(props.availableUnits)}</strong></div></header>
        <div className="extra-layout">
            <div className="extra-royal-table" data-phase={s?.phase || 'READY'} data-hands={hands.length}>
                {props.salon && <BlackjackArt asset={pokerArt.table} className="blackjack-table-art"/>}
                <div className="extra-dealer" role="group" aria-label="庄家公开手牌"><p className="extra-stage-label">{props.salon && <BlackjackArt asset={pokerArt.crest} className="blackjack-dealer-crest"/>}DEALER · 系统庄家</p><div className="extra-card-row">{(s?.dealer_revealed ? s.dealer_cards : s?.dealer_cards.slice(0, 1))?.map((card, i) => <PlayingCard code={card} salon={props.salon} key={i} />)}{s && !s.dealer_revealed && <span className="extra-card-back" role="img" aria-label="庄家暗牌，尚未公开">{props.salon ? <BlackjackArt asset={art.cardBack} className="blackjack-card-back-art"/> : <span aria-hidden="true">◇</span>}</span>}{!s && <span className="extra-table-empty">请选择下注，准备发牌</span>}</div>{s?.dealer_revealed && s.dealer_total ? <p aria-label="庄家总点数">{s.dealer_total.is_soft ? '软' : '硬'} {s.dealer_total.best_total} 点</p> : s ? <p>暗牌尚未公开</p> : <p>6 副牌 / S17 / Natural 3:2</p>}</div>
                <div className="extra-table-arc" aria-hidden="true"><span>6 DECKS · S17 · BLACKJACK PAYS 3:2</span></div>
                {hands.length > 1 && <nav className="extra-hand-nav" aria-label="查看玩家手牌">{hands.map((hand, i) => <button key={hand.hand_id} type="button" aria-label={`查看第 ${i + 1} 手`} aria-pressed={inspected === hand.hand_id} onClick={() => setInspection({ handID: hand.hand_id, version: s!.round_version })}>{i + 1} 手{hand.hand_id === active?.hand_id ? ' · 当前' : ''}</button>)}</nav>}
                {!s && props.salon && <div className="blackjack-ready"><BlackjackArt asset={art.cardBack} className="blackjack-ready-card"/><p>皇家牌桌，静候入席。</p><small>选择金额后，主动发牌开始本局。</small></div>}
                <div className="extra-hand-lanes">{hands.map((hand, i) => <article key={hand.hand_id} data-testid="blackjack-hand" data-hand-id={hand.hand_id} className={`extra-hand${hand.hand_id === active?.hand_id ? ' is-active' : ''}${hand.hand_id === inspected ? ' is-inspected' : ''}`} aria-label={`第 ${i + 1} 手${hand.hand_id === active?.hand_id ? '，当前行动' : ''}`}>
                    <header><strong>第 {i + 1} 手</strong><span>{hand.hand_id === active?.hand_id ? '● 当前行动' : handStates[hand.hand_state]}</span></header>
                    <div className="extra-card-row" style={{ '--bj-card-count': hand.cards.length } as CSSProperties}>{hand.cards.map((card, index) => <PlayingCard code={card} salon={props.salon} key={`${index}-${card}`} />)}</div>
                    <p className="extra-hand-total">{hand.value.is_soft ? '软' : '硬'} {hand.value.best_total} 点<span>{formatChipUnits(hand.stake_units)} 筹码</span></p>
                    {(hand.hand_state === 'SPLIT_ACES_COMPLETE' || hand.hand_state === 'DOUBLED_COMPLETE') && <p className="extra-stage-note">{handStates[hand.hand_state]}</p>}
                    {s?.phase === 'SETTLED' && <div className="extra-hand-result"><strong>{handResults[hand.result || ''] || handStates[hand.hand_state]}</strong><span>派彩 {formatChipUnits(hand.payout_units)} · 净 {formatChipUnits(hand.net_change_units, true)}</span></div>}
                </article>)}</div>
            </div>
            <aside className="extra-console">
                {props.salon && <div className="blackjack-balance"><BlackjackArt asset={art.chips} className="blackjack-chip-art"/><div><span>可用筹码</span><strong>{formatChipUnits(props.availableUnits)}</strong></div></div>}
                {!s || s.phase === 'SETTLED' ? <ExtraWager controls={props} blocked={blocked} label="初始下注 · 筹码" verb="Deal" onSubmit={units => void command.run(() => props.onDeal(units))} /> : <div className="extra-action-dock">
                    <p className="extra-eyebrow">PLAYER TURN</p><h3>操作当前第 {activeOrdinal || '—'} 手</h3><p className="extra-locked-stake">本局总下注 <strong>{formatChipUnits(s.total_stake_units)} 筹码</strong></p>
                    <div className="extra-main-actions"><button type="button" disabled={blocked || !active || !legal.includes('HIT')} onClick={() => act('HIT')} aria-label="Hit · 要牌"><span>要牌</span><small>HIT</small></button><button type="button" disabled={blocked || !active || !legal.includes('STAND')} onClick={() => act('STAND')} aria-label="Stand · 停牌"><span>停牌</span><small>STAND</small></button></div>
                    <div className="extra-add-actions">{(['DOUBLE', 'SPLIT'] as const).filter(type => legal.includes(type)).map(type => <button key={type} type="button" disabled={blocked || !canAdd} onClick={() => act(type)} aria-label={`${type === 'DOUBLE' ? 'Double' : 'Split'} · +${formatChipUnits(active?.stake_units || null)} 筹码`}><span>{type === 'DOUBLE' ? '加倍' : '分牌'}</span><small>+{formatChipUnits(active?.stake_units || null)} 筹码</small></button>)}</div>
                    {active && !canAdd && <p className="extra-note">{balance === null ? '正在核对追加下注所需余额。' : `当前余额不足以追加 ${formatChipUnits(active.stake_units)} 筹码；合法的要牌与停牌仍可使用。`}</p>}
                    {active && inspected !== active.hand_id && <p className="extra-note">正在查看其他手牌；行动按钮始终对应当前第 {activeOrdinal} 手。</p>}

                </div>}
                <ExtraNotice controls={props} failure={command.failure} message={props.recovering ? '正在恢复同一局、相同手牌与当前行动。' : props.autoResolving ? '服务端正在自动停牌并结算，等待确认结果。' : props.busy || command.pending ? '行动提交中，等待服务端确认。' : s?.phase === 'PLAYER_TURN' ? '你的回合。离开页面不会自动停牌或重新发牌。' : s ? '本局已结算。下一局仍需主动 Deal。' : '点击 Deal 即提交初始下注，无额外确认弹窗。'} />
                {s?.phase === 'PLAYER_TURN' && !props.salon && <p className="extra-deadline">长期未操作处理时间<br /><time dateTime={s.auto_resolve_at}>{new Date(s.auto_resolve_at).toLocaleString('zh-CN', { hour12: false })}</time><small>最后一次成功行动后 24 小时，由服务端处理；这里没有短促决策倒计时。</small></p>}
            </aside>
        </div>
        {s?.phase === 'SETTLED' && <div className={`extra-result is-${s.result_class?.toLowerCase() || 'loss'}`} role="status" aria-label="本局结算"><strong>{s.result_class === 'WIN' ? '净盈利' : s.result_class === 'BREAK_EVEN' ? '回本' : '净亏损'}</strong><dl><div><dt>总下注</dt><dd>{formatChipUnits(s.total_stake_units)}</dd></div><div><dt>公平返还</dt><dd>{formatChipUnits(s.fair_return_units || '0')}</dd></div><div><dt>总派彩（含返还）</dt><dd>{formatChipUnits(s.total_payout_units)}</dd></div><div><dt>整局净变化 · 筹码</dt><dd>{formatChipUnits(s.net_change_units, true)}</dd></div></dl></div>}
        {props.salon ? <nav className="blackjack-tool-dock" aria-label="牌桌辅助信息">
            <button type="button" onClick={props.onRules}>游戏规则<span aria-hidden="true">＋</span></button>
            <button type="button" onClick={props.onFairness}>公平验证<span aria-hidden="true">＋</span></button>
            {props.onHistory && <button type="button" onClick={props.onHistory}>本局记录 ↗</button>}
            <span className="blackjack-round">{props.roundID ? `Round · ${props.roundID} · v${s?.round_version || '—'}` : 'CASINO CAMELOT · ROYAL TABLE'}</span>
        </nav> : <div className="extra-utilities"><details><summary>六副牌与固定规则</summary><BlackjackRules/></details><div className="extra-links">{props.onFairness && <button type="button" onClick={props.onFairness}>公平详情</button>}{props.onHistory && <button type="button" onClick={props.onHistory}>本局记录</button>}</div>{props.roundID && <p className="extra-round-id">Round · {props.roundID} · v{s?.round_version || '—'}</p>}</div>}
    </section>;
}
