import { useEffect, useRef, useState, useSyncExternalStore, type FormEvent } from 'react';
import { Link, NavLink, Navigate, Outlet, Route, Routes, useLocation } from 'react-router-dom';
import { api, ApiClient, type User, type UsageLog, type TwoFactor, errorText } from './api';
import { Keys } from './Keys';
import { Models, Playground } from './Models';
import { Channels } from './Channels';
import { Wallet } from './Wallet';
import { MasterProfile } from './MasterProfile';
import { Alert, Crest, Empty, Loading, Pager, date, number, role, useResource } from './ui';
export function App({ client = api }: {
    client?: ApiClient;
}) {
    const session = useSyncExternalStore(client.subscribe, client.getSnapshot);
    useEffect(() => { void client.bootstrap(); }, [client]);
    if (!session.ready)
        return <div className="session-loading"><Crest /><Loading /></div>;
    return <Routes><Route path="/login" element={session.user ? <Navigate to="/dashboard" replace/> : <Login client={client}/>}/><Route path="/sign-in" element={<Navigate to="/login" replace/>}/><Route element={session.user ? <Shell client={client} user={session.user}/> : <Navigate to="/login" replace/>}><Route path="/dashboard" element={<Dashboard client={client} user={session.user!}/>}/><Route path="/wallet" element={<Wallet client={client} user={session.user!}/>}/><Route path="/master-profile" element={<MasterProfile client={client} user={session.user!}/>}/><Route path="/models" element={<Models client={client} user={session.user!}/>}/><Route path="/playground" element={<Playground client={client} user={session.user!}/>}/><Route path="/admin/channels" element={<Channels client={client} user={session.user!}/>}/><Route path="/keys" element={<Keys client={client}/>}/><Route path="/logs" element={<Logs client={client}/>}/></Route><Route path="*" element={<Navigate to={session.user ? '/dashboard' : '/login'} replace/>}/></Routes>;
}
function Login({ client }: {
    client: ApiClient;
}) {
    useEffect(() => { document.title = '登录 · momiao'; }, []);
    const session = useSyncExternalStore(client.subscribe, client.getSnapshot);
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [code, setCode] = useState('');
    const [twoFactor, setTwoFactor] = useState<TwoFactor | null>(null);
    const [error, setError] = useState('');
    const [busy, setBusy] = useState(false);
    const lock = useRef(false);
    async function submit(e: FormEvent) { e.preventDefault(); if (lock.current || session.loggingOut)
        return; lock.current = true; setBusy(true); setError(''); try {
        const next = twoFactor ? await client.verify2fa(twoFactor.flow_token, code.trim()) : await client.login(username.trim(), password);
        setPassword('');
        if (next)
            setTwoFactor(next);
    }
    catch (e) {
        setError(errorText(e));
    }
    finally {
        lock.current = false;
        setBusy(false);
    } }
    return <main className="login-layout"><section className="login-story"><a href="/" className="brand"><Crest /><span>momiao<small>CHALDEA PLATFORM</small></span></a><div className="login-title"><p className="eyebrow">YOUR PERSONAL COMMAND DECK</p><h1>Make room<br />for possibility<span>.</span></h1><p>每一次连接，<br />从你的指挥台开始。</p></div><div className="login-orbit"><Crest large/></div><div className="login-foot"><span>MOONLIT / CONNECTED</span><span>账户 · 密钥 · 调用记录</span></div></section><section className="login-entry"><div className="login-form"><p className="eyebrow">WELCOME ABOARD</p><h2>{twoFactor ? '验证你的身份' : '欢迎回来'}</h2><p className="subtitle">{twoFactor ? '输入验证器中的验证码或备用码，完成登录。' : '登录 momiao，管理你的 API 连接。'}</p>{session.notice && <Alert>{session.notice}{session.notice.includes('服务端退出未确认') && <button disabled={session.loggingOut} onClick={() => void client.logout().catch(() => { })}>重试退出</button>}</Alert>}<form onSubmit={submit}>{twoFactor ? <label>验证码或备用码<input autoFocus autoComplete="one-time-code" value={code} onChange={e => setCode(e.target.value)} required maxLength={64}/></label> : <><label>用户名<input autoComplete="username" value={username} onChange={e => setUsername(e.target.value)} required maxLength={128} placeholder="输入你的用户名"/></label><label>密码<input type="password" autoComplete="current-password" value={password} onChange={e => setPassword(e.target.value)} required maxLength={256} placeholder="输入账户密码"/></label></>}{error && <Alert>{error}</Alert>}<button className="primary login-submit" disabled={busy || session.loggingOut} type="submit">{session.loggingOut ? '正在退出…' : busy ? '正在验证…' : twoFactor ? '验证并登录' : '登录控制台'}<span aria-hidden="true">↗</span></button>{twoFactor && <button type="button" disabled={busy} onClick={() => { setTwoFactor(null); setCode(''); setError(''); }}>返回账户登录</button>}</form><div className="login-note"><span aria-hidden="true">◇</span><p>会话由安全 Cookie 续期。请勿在共享设备上保留登录状态。</p></div></div><footer>momiao <span> / </span> Chaldea Platform <span> / </span><a href="https://github.com/cy4268/momiao" target="_blank" rel="noopener noreferrer">源代码</a></footer></section></main>;
}
function Shell({ client, user }: {
    client: ApiClient;
    user: User;
}) {
    const location = useLocation();
    const [menu, setMenu] = useState(false);
    const main = useRef<HTMLElement>(null);
    useEffect(() => { setMenu(false); document.title = `${location.pathname === '/master-profile' ? 'Master 资料' : location.pathname === '/wallet' ? '我的钱包' : location.pathname === '/models' ? '模型目录' : location.pathname === '/playground' ? '文本测试' : location.pathname === '/admin/channels' ? '渠道管理' : location.pathname === '/keys' ? '密钥管理' : location.pathname === '/logs' ? '调用记录' : '指挥台'} · momiao`; main.current?.focus(); }, [location.pathname]);
    return <div className="app-shell"><a className="skip-link" href="#main-content">跳至主要内容</a><aside className="sidebar"><Link to="/dashboard" className="brand"><Crest /><span>momiao<small>CHALDEA PLATFORM</small></span></Link><p className="nav-label">WORKSPACE</p><nav aria-label="主导航"><NavLink to="/dashboard"><span aria-hidden="true">⌘</span>指挥台<small>Overview</small></NavLink><NavLink to="/wallet"><span aria-hidden="true">▤</span>我的钱包<small>Wallet</small></NavLink><NavLink to="/models"><span aria-hidden="true">⊞</span>模型目录<small>Models</small></NavLink><NavLink to="/playground"><span aria-hidden="true">▷</span>文本测试<small>Playground</small></NavLink>{user.role >= 10 && <NavLink to="/admin/channels"><span aria-hidden="true">⇄</span>渠道管理<small>Channels</small></NavLink>}<NavLink to="/keys"><span aria-hidden="true">◇</span>密钥管理<small>API keys</small></NavLink><NavLink to="/logs"><span aria-hidden="true">≡</span>调用记录<small>Usage logs</small></NavLink></nav><div className="sidebar-foot"><Crest /><p>A quieter space.<br />A clearer connection.</p><span>MOONLIT EDITION</span></div></aside><div className="workspace"><header className="topbar"><span className="topbar-label">PERSONAL WORKSPACE <span>/</span> 个人控制台</span><div className="account-wrap"><button className="account-button" aria-describedby="native-identity-label" aria-expanded={menu} aria-controls="account-menu" onClick={() => setMenu(!menu)}><span className="avatar">{(user.display_name || user.username).slice(0, 1)}</span><span>{user.display_name || user.username}<small id="native-identity-label">原生登录身份</small></span><span aria-hidden="true">⌄</span></button>{menu && <div className="account-menu" id="account-menu"><strong>{user.username}</strong><span>{role(user.role)} · 原生账户</span><Link to="/master-profile">Master 资料</Link><button onClick={() => void client.logout().catch(() => { })}>退出登录</button></div>}</div></header><main ref={main} id="main-content" tabIndex={-1} className="main-content"><Outlet /></main><footer className="workspace-foot"><span>momiao / Chaldea Platform <span> / </span><a href="https://github.com/cy4268/momiao" target="_blank" rel="noopener noreferrer">源代码</a></span><span>你的连接，由你掌握。</span></footer></div></div>;
}
function Dashboard({ client, user }: {
    client: ApiClient;
    user: User;
}) {
    const r = useResource(async () => { const [self, keys, logs] = await Promise.all([client.loadSelf(), client.keys(1, 5), client.logs('p=1&page_size=5')]); return { self, keys, logs }; }, [client]);
    const current = r.data?.self || user;
    return <><section className="command-deck"><div className="command-copy"><p className="eyebrow">CHALDEA / PERSONAL COMMAND DECK</p><h1>Your next<br />connection<span>.</span></h1><div className="command-greeting"><span className="short-rule"/><p>欢迎回来，<strong>{current.display_name || current.username}</strong>。<br />管理连接，开始下一次创造。</p></div><div className="hero-actions"><Link className="button primary" to="/models">选择模型并测试 <span aria-hidden="true">↗</span></Link><Link className="text-link" to="/logs">查看调用记录 →</Link></div></div><div className="deck-emblem"><span className="orbit-label top">MOMIAO · CHALDEA</span><Crest large/><span className="orbit-label bottom">CONNECTED TO POSSIBILITY</span></div><div className="deck-status"><span><i /> 账户已连接</span><span>{role(current.role)}</span></div></section>
 <section className="account-ledger" aria-label="账户使用概览"><div><p>可用原生额度</p><strong>{number(current.quota)}</strong><span>原生单位</span></div><div><p>已用原生额度</p><strong>{number(current.used_quota)}</strong><span>原生单位</span></div><div><p>累计请求</p><strong>{number(current.request_count)}</strong><span>次调用</span></div><div><p>API 密钥</p><strong>{r.loading ? '…' : r.error ? '—' : number(r.data?.keys.total)}</strong><Link to="/keys">管理密钥 ↗</Link></div></section><p className="ledger-note">原生额度为当前服务的计量单位，与平台本地钱包独立。<Link className="text-link" to="/wallet">查看 Reserve API Credit 与可用筹码 →</Link></p>
 <section className="panel recent"><div className="section-heading"><div><p className="eyebrow">RECENT ACTIVITY</p><h2>最近调用与活动</h2></div><Link className="text-link" to="/logs">查看全部 →</Link></div>{r.loading ? <Loading /> : r.error ? <><Alert>{r.error}</Alert><button onClick={r.reload}>重新加载</button></> : r.data && <>{r.data.keys.total === 0 && <div className="first-key"><div><strong>建立你的第一个连接</strong><p>创建一枚独立密钥，再将它添加到你信任的应用。</p></div><Link className="button" to="/keys">创建第一枚密钥 →</Link></div>}<LogTable items={r.data.logs.items}/></>}</section></>;
}
const logTypes: Record<number, string> = { 1: '充值', 2: '消费', 3: '管理', 4: '系统', 5: '错误', 6: '退款', 7: '登录' };
function LogTable({ items }: {
    items: UsageLog[];
}) { return items.length === 0 ? <Empty title="暂无记录">开始使用 API 后，在这里查看真实调用和额度消耗；筛选后无结果时，可调整筛选条件。</Empty> : <div className="table-wrap" role="region" aria-label="个人调用记录" tabIndex={0}><table><thead><tr><th>时间 / 类型</th><th>模型</th><th>密钥名称</th><th>输入 / 输出 Tokens</th><th>额度（原生单位）</th></tr></thead><tbody>{items.map(log => <tr key={log.id}><td><time>{date(log.created_at)}</time><small>{logTypes[log.type] || '其他'}</small></td><td><strong>{log.model_name || '—'}</strong></td><td>{log.token_name || '—'}</td><td className="numeric">{number(log.prompt_tokens)} <span className="muted">/</span> {number(log.completion_tokens)}</td><td className="numeric">{number(log.quota)}</td></tr>)}</tbody></table></div>; }
function Logs({ client }: {
    client: ApiClient;
}) {
    const [page, setPage] = useState(1);
    const [query, setQuery] = useState('');
    const [type, setType] = useState('0');
    const [model, setModel] = useState('');
    const [start, setStart] = useState('');
    const [end, setEnd] = useState('');
    const [error, setError] = useState('');
    const r = useResource(() => client.logs(`p=${page}&page_size=10${query ? `&${query}` : ''}`), [client, page, query]);
    function filter(e: FormEvent) { e.preventDefault(); if (start && end && start > end) {
        setError('结束日期应不早于开始日期。');
        return;
    } setError(''); const p = new URLSearchParams({ type, model_name: model.trim() }); if (start)
        p.set('start_timestamp', String(Math.floor(new Date(`${start}T00:00:00`).getTime() / 1000))); if (end)
        p.set('end_timestamp', String(Math.floor(new Date(`${end}T23:59:59`).getTime() / 1000))); setPage(1); setQuery(p.toString()); }
    function reset() { setType('0'); setModel(''); setStart(''); setEnd(''); setError(''); setPage(1); setQuery(''); }
    return <><header className="page-heading"><div><p className="eyebrow">OBSERVE / USAGE LOGS</p><h1>调用记录</h1><p>查看个人活动、模型调用与原生额度消耗。</p></div><button onClick={r.reload} disabled={r.loading}>刷新记录</button></header><section className="panel"><form className="filters" onSubmit={filter}><label>记录类型<select value={type} onChange={e => setType(e.target.value)}><option value="0">全部类型</option>{Object.entries(logTypes).map(([id, name]) => <option value={id} key={id}>{name}</option>)}</select></label><label className="model-filter">模型名称<input value={model} onChange={e => setModel(e.target.value)} maxLength={200} placeholder="完整模型名"/></label><label>开始日期<input type="date" value={start} onChange={e => setStart(e.target.value)}/></label><label>结束日期<input type="date" value={end} onChange={e => setEnd(e.target.value)}/></label><div className="filter-actions"><button type="submit" className="primary">应用筛选</button><button type="button" onClick={reset}>重置</button></div></form>{error && <Alert>{error}</Alert>}<p className="hint">时间按设备所在时区显示。仅展示调用元数据，不展示提示词或响应内容。</p>{r.loading ? <Loading /> : r.error ? <><Alert>{r.error}</Alert><button onClick={r.reload}>重新加载</button></> : r.data && <><LogTable items={r.data.items}/><Pager page={page} total={r.data.total} size={r.data.page_size} onChange={setPage}/></>}</section></>;
}
