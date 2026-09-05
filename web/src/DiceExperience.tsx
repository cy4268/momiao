import { useState } from 'react';
import { resolveDice, rollDice, type DiceChoice, type DiceResult } from './dice';
import { Alert, Empty } from './ui';

type RecordItem = DiceResult & { choice: DiceChoice; at: string };
const labels = { BIG: '大', SMALL: '小', TRIPLE: '豹子' };
const storageKey = (user: string) => `momiao.dice.experience.v1.${user}`;
const storageNotice = '浏览器存储暂不可用，本次结果仅在当前页面保留，刷新后可能丢失。';

function readHistory(user: string): { items: RecordItem[]; notice: string } {
    let raw: string | null;
    try { raw = sessionStorage.getItem(storageKey(user)); }
    catch { return { items: [], notice: storageNotice }; }
    if (!raw) return { items: [], notice: '' };
    try {
        if (raw.length > 12000) throw new Error();
        const data: unknown = JSON.parse(raw);
        if (!Array.isArray(data) || data.length > 20) throw new Error();
        const items = data.map(item => {
            if (!item || !Array.isArray(item.dice) || (item.choice !== 'BIG' && item.choice !== 'SMALL') || typeof item.at !== 'string' || item.at.length > 30 || new Date(item.at).toISOString() !== item.at) throw new Error();
            // Local records are editable. Recompute the labels; never treat them as server receipts.
            return { ...resolveDice(item.dice), choice: item.choice as DiceChoice, at: item.at };
        });
        return { items, notice: '' };
    } catch { return { items: [], notice: '本机记录格式异常，已忽略。下一次体验会重新保存记录。' }; }
}

const pipPositions = [[], [[50, 50]], [[27, 27], [73, 73]], [[27, 27], [50, 50], [73, 73]], [[27, 27], [73, 27], [27, 73], [73, 73]], [[27, 27], [73, 27], [50, 50], [27, 73], [73, 73]], [[27, 27], [73, 27], [27, 50], [73, 50], [27, 73], [73, 73]]];
function Die({ value }: { value?: number }) {
    return <svg className="dice-face" viewBox="0 0 100 100" aria-hidden="true">
        <rect x="2" y="2" width="96" height="96" rx="13" fill="var(--paper)" stroke="currentColor" strokeWidth="1.5" />
        {value ? pipPositions[value].map(([x, y], i) => <circle key={i} cx={x} cy={y} r="6" fill="currentColor" />) : <path d="M39 36a13 13 0 0 1 25 4c0 12-14 11-14 24m0 8v4" fill="none" stroke="currentColor" strokeWidth="4" />}
    </svg>;
}
const verdict = (item: RecordItem) => item.side === 'TRIPLE' ? '大小均未命中' : item.side === item.choice ? '猜中了' : '未猜中';

export function DiceExperience({ userID }: { userID: string }) {
    const [state, setState] = useState(() => readHistory(userID));
    const [choice, setChoice] = useState<DiceChoice>('SMALL');
    const [error, setError] = useState('');
    const latest = state.items[0];

    function roll() {
        setError('');
        let item: RecordItem;
        try { item = { ...rollDice(), choice, at: new Date().toISOString() }; }
        catch { setError('浏览器随机源暂不可用，请稍后再试。未生成新结果。'); return; }
        const items = [item, ...state.items].slice(0, 20);
        let notice = '';
        try { sessionStorage.setItem(storageKey(userID), JSON.stringify(items)); }
        catch { notice = storageNotice; }
        setState({ items, notice });
    }
    function clear() {
        try { sessionStorage.removeItem(storageKey(userID)); setState({ items: [], notice: '' }); setError(''); }
        catch { setError('本机存储暂不可用，记录尚未清除。请在浏览器恢复存储后重试。'); }
    }

    return <div className="dice-page">
        <header className="page-heading"><div><p className="eyebrow">CHALDEA / DICE EXPERIENCE</p><h1>三枚骰子，一次猜想。</h1><p>选大或小，亲手掷一次，看看三枚骰子会如何落下。</p></div><span className="dice-mode">体验模式</span></header>
        <p className="dice-scope">不扣除筹码，不发放奖励，不改变 API 额度。这里只体验规则，不进行资产结算。</p>
        <div className="dice-layout">
            <section className="panel dice-table" aria-labelledby="dice-table-title">
                <div className="section-heading"><div><p className="eyebrow">THREE DICE / SIX SIDES</p><h2 id="dice-table-title">骰子体验</h2></div><span className="dice-local-label">本机模拟</span></div>
                <div className="dice-stage">
                    <div className="dice-faces" role="img" aria-label={latest ? `三枚骰子：${latest.dice.join('、')} 点` : '三枚骰子尚未掷出'}>{[0, 1, 2].map(i => <Die key={i} value={latest?.dice[i]} />)}</div>
                    <p className="dice-result" role="status" aria-live="polite" aria-atomic="true">{latest ? `${latest.total} 点 · ${labels[latest.side]} · ${verdict(latest)}` : '选择一侧，再掷出你的第一组点数。'}</p>
                    {latest && <p className="dice-previous-choice">这一轮选择：{labels[latest.choice]} · 修改下方选项只影响下一轮</p>}
                </div>
                <fieldset className="dice-choices"><legend>你的猜想</legend>{(['SMALL', 'BIG'] as const).map(side => <label key={side}><input type="radio" name="dice-choice" value={side} checked={choice === side} onChange={() => setChoice(side)} /><span><strong>猜{labels[side]}</strong><small>{side === 'SMALL' ? '4–10 点' : '11–17 点'}，不含豹子</small></span></label>)}</fieldset>
                <button type="button" className="primary dice-roll" onClick={roll}>模拟掷骰<span aria-hidden="true">↗</span></button>
                {error && <Alert>{error}</Alert>}
                <p className="hint dice-random-note">点数由浏览器随机源生成；结果仅用于本机体验，不是服务端公平性凭证。</p>
            </section>
            <aside className="panel dice-rules" aria-labelledby="dice-rules-title"><p className="eyebrow">KNOW THE RULES</p><h2 id="dice-rules-title">看懂每一种结果</h2>
                <dl><div><dt>小 <span>4–10</span></dt><dd>三枚点数相加为 4 到 10，且不是三个相同点数。</dd></div><div><dt>大 <span>11–17</span></dt><dd>三枚点数相加为 11 到 17，且不是三个相同点数。</dd></div><div><dt>豹子 <span>三枚相同</span></dt><dd>从 1·1·1 到 6·6·6，不论总和，猜大或猜小均未命中。</dd></div></dl>
                <div className="dice-probability"><strong>216 种等可能组合</strong><p>大 105 种 · 小 105 种 · 豹子 6 种</p><p className="hint">每次独立生成，上一轮的结果不会改变下一轮的概率。短期记录不必符合理论比例。</p></div>
            </aside>
        </div>
        <section className="panel dice-history" aria-labelledby="dice-history-title"><div className="section-heading"><div><p className="eyebrow">THIS TAB / LOCAL HISTORY</p><h2 id="dice-history-title">本机体验记录</h2></div><button type="button" onClick={clear} disabled={!state.items.length}>清空本机记录</button></div>
            <p className="hint">仅保留当前账户在本标签页的最近 20 次体验。刷新可恢复；不跨设备同步，也不是钱包账本或正式游戏回执。</p>
            {state.notice && <p className="dice-storage-note">{state.notice}</p>}
            {state.items.length ? <div className="table-wrap" role="region" aria-label="本机记录表格" tabIndex={0}><table aria-label="本机体验记录"><thead><tr><th>时间</th><th>点数 / 总和</th><th>猜想</th><th>结果</th></tr></thead><tbody>{state.items.map((item, i) => <tr key={`${item.at}-${i}`}><td><time dateTime={item.at}>{new Date(item.at).toLocaleTimeString('zh-CN', { hour12: false })}</time></td><td><strong>{item.dice.join(' · ')}</strong><small>合计 {item.total} 点</small></td><td>{labels[item.choice]}</td><td>{labels[item.side]}<small>{verdict(item)}</small></td></tr>)}</tbody></table></div> : <Empty title="还没有体验记录">点击“模拟掷骰”后，点数与猜想会记在这里。</Empty>}
        </section>
    </div>;
}
