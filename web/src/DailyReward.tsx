import type { ReactNode } from 'react';
import { RewardArt } from './WalletChamber';
import type { Daily } from './economy-api';
import { Alert, Loading } from './ui';

// Presentation only. WalletActions owns the one pending request/reconciliation flow.
export function DailyReward({ daily, blocked, onClaim, links }: { daily: { data?: Daily; loading: boolean; error: string; reload: () => void }; blocked: boolean; onClaim: () => void; links?: ReactNode }) {
    return <section className="panel reward-card" aria-labelledby="daily-title"><div><p className="eyebrow">DAILY / RESERVE SUPPLY</p><h2 id="daily-title">每日签到</h2><RewardArt name="voucher" /><p className="wallet-amount">500 <small>Reserve</small></p><p className="reward-description">每日一次 · 上海自然日</p></div><div className="daily-action">
        {daily.loading ? <Loading /> : daily.error ? <><Alert>签到状态读取未完成。</Alert><button onClick={daily.reload}>刷新签到状态</button></> : daily.data && <><p className="daily-state" role="status">{daily.data.claimed ? '今日已领取' : '今日待领取'}</p><p className="hint">奖励日期：{daily.data.business_date}<br />下次刷新：{new Date(daily.data.next_reset_at).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false })}（上海时间）</p><button className="primary" disabled={blocked || daily.data.claimed} onClick={onClaim}>{daily.data.claimed ? '今日已领取' : '领取今日 500 额度'}</button><button className="quiet" disabled={blocked} onClick={daily.reload}>刷新签到状态</button></>}

    </div>{links}</section>;
}
