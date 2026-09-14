import { useState } from 'react';
import type { ApiClient } from './api';
import { readHourly, readRelief, type PendingOperation, type Transaction } from './economy-api';
import { Alert, Loading, useResource } from './ui';

type RewardKind = Extract<PendingOperation['kind'], 'HOURLY' | 'RELIEF'>;

export function RewardPrograms({ client, userID, blocked, version, onClaim }: {
    client: ApiClient;
    userID: string;
    blocked: boolean;
    version: number;
    onClaim: (kind: RewardKind) => Promise<Transaction | undefined>;
}) {
    const hourly = useResource(() => readHourly(client, userID), [client, userID, version]);
    const relief = useResource(() => readRelief(client, userID), [client, userID, version]);
    const [claiming, setClaiming] = useState<RewardKind | null>(null);
    async function claim(kind: RewardKind) {
        if (blocked || claiming) return;
        setClaiming(kind);
        try { await onClaim(kind); } finally { setClaiming(null); }
    }
    return <>
        <section className="panel reward-card" aria-labelledby="hourly-title"><div><p className="eyebrow">HOURLY / RESERVE SUPPLY</p><h2 id="hourly-title">小时奖励</h2><p className="wallet-amount">100 <small>API Credit</small></p><p>每个上海自然小时可领一次，不累计补领；每天最多 24 次。</p></div><div className="daily-action">
            {hourly.loading ? <Loading /> : hourly.error ? <><Alert>小时奖励状态读取未完成。</Alert><button onClick={hourly.reload}>刷新小时奖励</button></> : hourly.data && <><p className="daily-state" role="status">{hourly.data.claimed ? '本小时已领取' : hourly.data.claims_today >= hourly.data.daily_limit ? '今日已达领取上限' : '本小时待领取'}</p><p className="hint">今日已领 {hourly.data.claims_today} / {hourly.data.daily_limit} 次<br />下个自然小时：{new Date(hourly.data.next_reset_at).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false })}</p><button className="primary" disabled={blocked || claiming !== null || hourly.data.claimed || hourly.data.claims_today >= hourly.data.daily_limit} onClick={() => void claim('HOURLY')}>{claiming === 'HOURLY' ? '领取中…' : hourly.data.claimed ? '本小时已领取' : '领取本小时 100 额度'}</button><button className="quiet" disabled={blocked || claiming !== null} onClick={hourly.reload}>刷新小时奖励</button></>}
        </div></section>
        <section className="panel reward-card" aria-labelledby="relief-title"><div><p className="eyebrow">RELIEF / LOW-ASSET SUPPORT</p><h2 id="relief-title">低资产救济</h2><p className="wallet-amount">300 <small>API Credit</small></p><p>总资产严格低于 10 API Credit 时可领；成功后滚动冷却四小时，不要求离开牌桌。</p></div><div className="daily-action">
            {relief.loading ? <Loading /> : relief.error ? <><Alert>救济资格暂时无法核对；未发起领取。</Alert><button onClick={relief.reload}>重新核对资格</button></> : relief.data && <><p className="daily-state" role="status">{relief.data.eligible ? '当前符合领取条件' : relief.data.next_eligible_at && Date.parse(relief.data.next_eligible_at) > Date.now() ? '四小时冷却中' : '当前资产不符合条件'}</p><p className="hint">已核对总资产：{relief.data.current_total_assets} API Credit<br />{relief.data.next_eligible_at ? `冷却时间点：${new Date(relief.data.next_eligible_at).toLocaleString('zh-CN', { hour12: false })}` : '尚无成功领取记录'}</p><button className="primary" disabled={blocked || claiming !== null || !relief.data.eligible} onClick={() => void claim('RELIEF')}>{claiming === 'RELIEF' ? '领取中…' : '领取 300 救济额度'}</button><button className="quiet" disabled={blocked || claiming !== null} onClick={relief.reload}>重新核对资格</button></>}
        </div></section>
    </>;
}
