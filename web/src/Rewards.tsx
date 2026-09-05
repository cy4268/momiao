import { useState } from 'react';
import { Link } from 'react-router-dom';
import type { ApiClient, User } from './api';
import { WalletActions } from './WalletActions';
import { readWallet } from './wallet-api';
import { Alert, Empty, Loading, useResource } from './ui';

export function Rewards({ client, user }: { client: ApiClient; user: User }) {
    return <RewardsView key={user.id + ':' + client.getSessionGeneration()} client={client} userID={String(user.id)} />;
}
function RewardsView({ client, userID }: { client: ApiClient; userID: string }) {
    const wallet = useResource(() => readWallet(client, userID), [client, userID]);
    const [receipt, setReceipt] = useState('');
    return <div className="rewards-page"><header className="page-heading"><div><p className="eyebrow">REWARDS / DAILY SUPPLY</p><h1>奖励中心</h1><p>每天一份补给，由你主动领取。</p></div><Link className="button" to="/wallet">查看我的钱包</Link></header>
        <div className="reward-rail"><span>每日签到</span><strong>500 API Credit → Reserve</strong><span>上海自然日 · 每天一次</span></div>
        {receipt && <p className="notice" role="status">交易已确认。交易编号：{receipt}</p>}
        {wallet.loading ? <Loading /> : wallet.error ? <section className="panel"><Alert>钱包读取未完成。{wallet.error}</Alert><button onClick={wallet.reload}>重新读取钱包</button></section> : wallet.data && (!wallet.data.initialized ? <section className="panel"><Empty title="先建立你的钱包">每日奖励存入 Reserve。请先前往钱包，主动初始化零余额钱包，再回来领取。</Empty><Link className="button primary" to="/wallet">前往初始化钱包</Link></section> : <WalletActions client={client} userID={userID} wallet={wallet.data} dailyOnly onChange={t => { setReceipt(t.id); wallet.reload(); }} />)}
        <section className="reward-next"><div><p className="eyebrow">FROM RESERVE TO API</p><h2>领取之后，按需激活</h2><p>Reserve 是你的储备 API Credit。使用 API 前，在钱包中选择数量，主动转入原生可用额度。</p></div><Link className="button" to="/wallet/activate">激活 API 额度 <span aria-hidden="true">↗</span></Link></section>
    </div>;
}
