import type { Daily } from './economy-api';
import { Alert, Loading } from './ui';

// Presentation only. WalletActions owns the one pending request/reconciliation flow.
export function DailyReward({ daily, blocked, onClaim }: { daily: { data?: Daily; loading: boolean; error: string; reload: () => void }; blocked: boolean; onClaim: () => void }) {
    return <section className="panel reward-card" aria-labelledby="daily-title"><div><p className="eyebrow">DAILY / RESERVE SUPPLY</p><h2 id="daily-title">每日签到</h2><p className="wallet-amount">500 <small>API Credit</small></p><p>固定进入 Reserve，按上海自然日每天一次。</p></div><div className="daily-action">
        {daily.loading ? <Loading /> : daily.error ? <><Alert>签到状态读取未完成。</Alert><button onClick={daily.reload}>刷新签到状态</button></> : daily.data && <><p className="daily-state" role="status">{daily.data.claimed ? '今日已领取' : '今日待领取'}</p><p className="hint">奖励日期：{daily.data.business_date}<br />下次刷新：{new Date(daily.data.next_reset_at).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false })}（上海时间）</p><button className="primary" disabled={blocked || daily.data.claimed} onClick={onClaim}>{daily.data.claimed ? '今日已领取' : '领取今日 500 额度'}</button><button className="quiet" disabled={blocked} onClick={daily.reload}>刷新签到状态</button></>}
        <p className="hint">小时奖励与救济尚未开放。注册赠额状态可在账户与安全中核对。</p>
    </div></section>;
}
