import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { ApiClient, errorText, type User } from './api';
import { WalletActions } from './WalletActions';
import { Alert, Empty, Loading, useResource } from './ui';
import { assets, assetNames, parseWallet, readWallet, readLedger, type Asset } from './wallet-api';

export function Wallet({ client, user }: { client: ApiClient; user: User }) {
    return <WalletView key={`${user.id}:${client.getSessionGeneration()}`} client={client} userID={String(user.id)} />;
}
function WalletView({ client, userID }: { client: ApiClient; userID: string }) {
    const [busy, setBusy] = useState(false);
    const [uncertain, setUncertain] = useState(false);
    const [notice, setNotice] = useState('');
    const [receipt, setReceipt] = useState('');
    const [asset, setAsset] = useState<Asset>('RESERVE_API_CREDIT');
    const active = useRef(false);
    const lock = useRef(false);
    useEffect(() => { active.current = true; return () => { active.current = false; }; }, []);
    const r = useResource(() => readWallet(client, userID), [client, userID]);
    useEffect(() => { if (r.data) { setUncertain(false); setNotice(''); } }, [r.data]);
    async function initialize() {
        if (lock.current || uncertain || r.loading || r.error || !r.data || r.data.initialized) return;
        lock.current = true; setBusy(true); setNotice('');
        const current = () => active.current && String(client.getSnapshot().user?.id) === userID && !client.getSnapshot().loggingOut;
        try {
            const result = parseWallet(await client.request('/platform/v1/wallet/initialize', 'POST', {}), userID);
            if (!result.initialized) throw new Error('初始化响应未确认完成。');
            if (current()) r.reload();
        } catch (e) {
            // Any failed write requires a successful, explicit GET before another attempt.
            if (current()) { setUncertain(true); setNotice(`初始化结果尚未确认。请点击“刷新钱包”核对状态，勿重复提交。${errorText(e)}`); }
        } finally {
            if (current()) { lock.current = false; setBusy(false); }
        }
    }
    return <div className="wallet-page"><header className="page-heading"><div><p className="eyebrow">ACCOUNT / LOCAL WALLETS</p><h1>我的钱包</h1><p>分别查看平台本地资产与真实流水，不合并计算总资产。</p></div><button onClick={r.reload} disabled={r.loading || busy}>刷新钱包</button></header>
        <div className="wallet-scope"><p>此处仅展示 Reserve API Credit 与可用筹码。初始化只建立两个零余额钱包，不发放赠金。</p><p>每日签到与本地兑换已开放；原生额度划转与游戏尚未开放。<Link className="text-link" to="/dashboard">原生额度请到指挥台查看 →</Link></p></div>
        {notice && <Alert>{notice}</Alert>}{receipt && <p className="notice" role="status">交易已确认。交易编号：{receipt}</p>}
        {r.loading ? <Loading /> : r.error ? <Alert>{r.error} 请使用“刷新钱包”重试读取。</Alert> : r.data && (!r.data.initialized ? <section className="panel wallet-initialize"><Empty title="尚未初始化钱包">读取钱包不会创建账户资产。你可以主动初始化，建立两个独立的零余额钱包。</Empty><button className="primary" onClick={() => void initialize()} disabled={busy || uncertain}>{busy ? '正在初始化…' : '初始化零余额钱包'}</button>{uncertain && <p className="hint">再次初始化已暂停，等待刷新核对。</p>}</section> : <>
            <section className="wallet-balances" aria-label="平台本地钱包余额">{assets.map(asset => { const w = r.data!.wallets.find(w => w.asset === asset)!; return <article className="panel wallet-balance" key={asset}><p className="eyebrow">{asset === 'RESERVE_API_CREDIT' ? 'RESERVE / API CREDIT' : 'AVAILABLE / CHIPS'}</p><h2>{assetNames[asset]}</h2><p className="wallet-amount">{w.amount}</p><p className="hint">{asset === 'RESERVE_API_CREDIT' ? '储备 API Credit；与原生额度独立' : '可用筹码；与储备 API Credit 独立'}</p><dl><div><dt>原子单位</dt><dd>{w.balance_units}</dd></div><div><dt>账本序号</dt><dd>{w.ledger_seq}</dd></div><div><dt>钱包版本</dt><dd>{w.version}</dd></div></dl></article>; })}</section>
            <WalletActions client={client} userID={userID} wallet={r.data} onChange={t=>{setReceipt(t.id);r.reload()}} />
            <section className="panel"><div className="section-heading"><div><p className="eyebrow">WALLET LEDGER</p><h2>资产流水</h2></div><label>流水资产<select value={asset} onChange={e => setAsset(e.target.value as Asset)}>{assets.map(a => <option key={a} value={a}>{assetNames[a]}</option>)}</select></label></div><Ledger key={asset} client={client} userID={userID} asset={asset} /></section>
        </>)}
    </div>;
}
function Ledger({ client, userID, asset }: { client: ApiClient; userID: string; asset: Asset }) {
    const [cursors, setCursors] = useState(['0']);
    const after = cursors[cursors.length - 1];
    // Keying each page removes the old page immediately; useResource drops late completions.
    return <LedgerPage key={after} client={client} userID={userID} asset={asset} after={after} previous={cursors.length > 1 ? () => setCursors(c => c.slice(0, -1)) : undefined} next={cursor => setCursors(c => [...c, cursor])} />;
}
function LedgerPage({ client, userID, asset, after, previous, next }: { client: ApiClient; userID: string; asset: Asset; after: string; previous?: () => void; next: (cursor: string) => void }) {
    const r = useResource(() => readLedger(client, userID, asset, after), [client, userID, asset, after]);
    return <><p className="hint">按账本序号从早到晚排列，每页最多 20 条。时间按设备所在时区显示；正号为增加，负号为减少。</p>{r.loading ? <Loading /> : r.error ? <><Alert>{r.error}</Alert><button onClick={r.reload}>重新加载流水</button></> : r.data && (r.data.items.length === 0 ? <Empty title="暂无资产流水">当前资产尚无此页记录。零余额初始化不代表奖励或资产入账。</Empty> : <div className="table-wrap" role="region" aria-label={`${assetNames[asset]}流水`} tabIndex={0}><table><thead><tr><th>时间 / 序号</th><th>业务 / 类型</th><th>变动金额</th><th>变动后余额</th></tr></thead><tbody>{r.data.items.map(e => <tr key={e.id}><td><time dateTime={e.created_at}>{new Date(e.created_at).toLocaleString('zh-CN', { hour12: false })}</time><small>序号 {e.ledger_seq}</small></td><td>{e.biz_type}<small>{e.entry_type}</small></td><td className="numeric">{e.delta_amount.startsWith('-') ? e.delta_amount : `+${e.delta_amount}`}</td><td className="numeric">{e.balance_after_amount}</td></tr>)}</tbody></table></div>)}<nav className="pager" aria-label="钱包流水分页"><span>当前起始游标 {after}</span><div><button onClick={previous} disabled={!previous || r.loading} aria-label="上一页流水">← 上一页</button><button disabled={r.loading || !r.data?.has_more || !!r.error} onClick={() => { if (r.data?.next_after_seq) next(r.data.next_after_seq); }} aria-label="下一页流水">下一页 →</button></div></nav></>;
}
