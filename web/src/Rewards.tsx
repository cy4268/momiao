import { useState } from 'react';
import { Link } from 'react-router-dom';
import type { ApiClient, User } from './api';
import { WalletChamber, WalletRules, BalanceEmblem } from './WalletChamber';
import { WalletActions } from './WalletActions';
import { readWallet } from './wallet-api';
import { Alert, Empty, Loading, useResource } from './ui';

export function Rewards({ client, user }: { client: ApiClient; user: User }) {
    return <RewardsView key={user.id + ':' + client.getSessionGeneration()} client={client} userID={String(user.id)} />;
}
function RewardsView({ client, userID }: { client: ApiClient; userID: string }) {
    const wallet = useResource(() => readWallet(client, userID), [client, userID]);
    const [receipt, setReceipt] = useState('');
    return <WalletChamber page="rewards"><header className="page-heading"><div><p className="eyebrow">REWARDS / RESERVE SUPPLY</p><h1>奖励中心</h1><p>每日、小时与低资产补给都由你主动领取。</p></div><Link className="button" to="/wallet">查看我的钱包</Link></header>
        {wallet.data?.initialized && <section className="reward-rail"><BalanceEmblem /><span>当前 Reserve</span><strong>{wallet.data.wallets.find(w => w.asset === 'RESERVE_API_CREDIT')!.amount}</strong><span>三种补给均进入 Reserve</span></section>}
        {receipt && <p className="notice" role="status">交易已确认。交易编号：{receipt}</p>}
        {wallet.loading ? <Loading /> : wallet.error ? <section className="panel"><Alert>钱包读取未完成。{wallet.error}</Alert><button onClick={wallet.reload}>重新读取钱包</button></section> : wallet.data && (!wallet.data.initialized ? <section className="panel"><Empty title="先建立你的钱包">每日奖励存入 Reserve。请先前往钱包，主动初始化零余额钱包，再回来领取。</Empty><Link className="button primary" to="/wallet">前往初始化钱包</Link></section> : <WalletActions client={client} userID={userID} wallet={wallet.data} dailyOnly beforeHistory={<section className="reward-next"><div><p className="eyebrow">FROM RESERVE TO API</p><h2>领取之后，按需激活</h2><p>将 Reserve 转入原生额度后用于 API 调用。</p></div><Link className="button" to="/wallet/activate">激活 API 额度 <span aria-hidden="true">↗</span></Link></section>} onChange={t => { setReceipt(t.id); wallet.reload(); }} />)}

        <WalletRules rewards wallet={wallet.data} />
    </WalletChamber>;
}
