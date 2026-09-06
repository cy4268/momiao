import { useEffect, useRef, useState, useSyncExternalStore, type FormEvent } from 'react';
import { Link, NavLink, Navigate, Outlet, Route, Routes, useLocation, useNavigate, useOutletContext } from 'react-router-dom';
import { api, ApiClient, type User, type UsageLog, errorText } from './api';
import { Keys } from './Keys';
import { Playground } from './Models';
import { Catalog, CatalogDetailPage, APIAccess } from './Catalog';
import { OpsCatalog } from './OpsCatalog';
import type { CatalogLoginRequired } from './catalog-api';
import { Channels } from './Channels';
import { Wallet } from './Wallet';
import { QuotaActivation } from './QuotaActivation';
import { DiceExperience } from './DiceExperience';
import { MasterProfile } from './MasterProfile';
import { Home } from './Home';
import { Announcements, AnnouncementDetail, AnnouncementEntry } from './Announcements';
import { OpsAnnouncements } from './OpsAnnouncements';
import { PersonalHub, MasterSummary, type MasterResource } from './PersonalHub';
import { Rewards } from './Rewards';
import { readProfile, profileError } from './profile-api';
import { Alert, Brand, Crest, Empty, Loading, Pager, date, number, role, useResource } from './ui';
import { Authentication, DiscordCallback, type CapturedCallback } from './Authentication';
import { Account } from './Account';
import { AccessGate, LoginRequired, PostAuthGate } from './PostAuthGate';
import type { SensitiveProof } from './admission-api';
const missingCallback:CapturedCallback={error:'授权回调已失效，请重新开始。'};
export function App({ client = api, capturedCallback = missingCallback, onCatalogLoginRequired }: { client?: ApiClient; capturedCallback?:CapturedCallback; onCatalogLoginRequired?: CatalogLoginRequired }) {
    const session = useSyncExternalStore(client.subscribe, client.getSnapshot);
    const navigate=useNavigate();const location=useLocation();
    const [proof,setProof]=useState<{value:SensitiveProof;generation:number}>();
    useEffect(()=>{if(proof&&(proof.generation!==client.getSessionGeneration()||!['/account','/oauth/discord'].includes(location.pathname)))setProof(undefined);},[session.user,client,location.pathname,proof]);
    useEffect(() => { void client.bootstrap(); }, [client]);
    const loading = <div className="session-loading"><Crest /><Loading /></div>;
    const announcementSession = (session.user?.id || 'guest') + ':' + client.getSessionGeneration() + ':' + session.ready;
    const routedSession=announcementSession+':'+location.pathname+location.search;
    return <>{session.ready && !session.user && <AnnouncementEntry client={client} />}<Routes>
        <Route path="/" element={<Home key={announcementSession} signedIn={!!session.user} client={client} />} />
        <Route path="/announcements" element={<Announcements key={announcementSession} client={client} />} />
        <Route path="/announcements/:id" element={<AnnouncementDetail key={announcementSession} client={client} />} />
        <Route path="/models" element={<Catalog key={routedSession} client={client} />} />
        <Route path="/models/*" element={<CatalogDetailPage key={routedSession} client={client} onLoginRequired={onCatalogLoginRequired} />} />
        <Route path="/api/access" element={<APIAccess key={routedSession} client={client} onLoginRequired={onCatalogLoginRequired} />} />
        <Route path="/ops/models" element={!session.ready ? loading : session.user ? <AccessGate key={routedSession} client={client} user={session.user} route="/ops/models"><OpsCatalog client={client} /></AccessGate> : <LoginRequired />} />
        <Route path="/ops/announcements" element={!session.ready ? loading : session.user ? <AccessGate key={announcementSession+location.pathname} client={client} user={session.user} route="/ops/announcements"><OpsAnnouncements client={client} /></AccessGate> : <LoginRequired />} />
        <Route path="/login" element={!session.ready ? loading : session.user ? <Navigate to="/welcome" replace /> : <Authentication client={client} mode="login" />} />
        <Route path="/register" element={!session.ready ? loading : session.user ? <Navigate to="/welcome" replace /> : <Authentication client={client} mode="registration" />} />
        <Route path="/oauth/discord" element={!session.ready?loading:<DiscordCallback client={client} captured={capturedCallback} onComplete={value=>{if(value){setProof({value,generation:client.getSessionGeneration()});navigate('/account',{replace:true});}else navigate('/welcome',{replace:true});}}/>}/>
        <Route path="/welcome" element={!session.ready?loading:session.user?<PostAuthGate key={session.user.id+':'+client.getSessionGeneration()} client={client} user={session.user}/>:<Navigate to="/login" replace/>}/>
        <Route path="/sign-in" element={<Navigate to="/login" replace />} />
        <Route element={!session.ready ? loading : session.user ? <AccessGate key={routedSession} client={client} user={session.user} route={location.pathname+(location.pathname==='/keys'?location.search:'')}><Shell client={client} user={session.user} /></AccessGate> : <LoginRequired />}>
            <Route path="/account" element={<Account key={session.user?.id+':'+client.getSessionGeneration()} client={client} proof={proof?.generation===client.getSessionGeneration()?proof.value:undefined} clearProof={()=>setProof(undefined)}/>}/>
            <Route path="/dashboard" element={<Dashboard client={client} user={session.user!} />} />
            <Route path="/me" element={<PersonalHub client={client} user={session.user!} />} />
            <Route path="/rewards" element={<Rewards client={client} user={session.user!} />} />
            <Route path="/games/dice" element={session.user && <DiceExperience key={session.user.id + ':' + client.getSessionGeneration()} userID={String(session.user.id)} />} />
            <Route path="/wallet/activate" element={session.user && <QuotaActivation key={session.user.id + ':' + client.getSessionGeneration()} client={client} userID={String(session.user.id)} />} />
            <Route path="/wallet" element={<Wallet client={client} user={session.user!} />} />
            <Route path="/master-profile" element={<ProfileRoute client={client} user={session.user!} />} />
            <Route path="/playground" element={<Playground client={client} user={session.user!} />} />
            <Route path="/admin/channels" element={<Channels client={client} user={session.user!} />} />
            <Route path="/keys" element={<Keys client={client} />} />
            <Route path="/logs" element={<Logs client={client} />} />
        </Route>
        <Route path="*" element={!session.ready ? loading : <Navigate to={session.user ? '/dashboard' : '/login'} replace />} />
    </Routes></>;
}
function ProfileRoute({ client, user }: { client: ApiClient; user: User }) {
    const master = useOutletContext<MasterResource>();
    return <MasterProfile client={client} user={user} onSaved={master.reload} />;
}
const pageTitles: Record<string, string> = {
    '/dashboard': '指挥台', '/me': '个人中心', '/rewards': '奖励中心', '/games/dice': '骰子体验',
    '/master-profile': 'Master 资料', '/account': '账户与安全', '/wallet/activate': '转入原生额度', '/wallet': '我的钱包',
    '/models': '模型目录', '/playground': '文本测试', '/admin/channels': '渠道管理', '/keys': '密钥管理', '/logs': '调用记录',
};
function Shell({ client, user }: { client: ApiClient; user: User }) {
    const location = useLocation();
    const domain = ['/models', '/keys', '/logs', '/playground'].includes(location.pathname) ? 'models'
        : ['/wallet', '/wallet/activate', '/rewards'].includes(location.pathname) ? 'assets'
        : ['/me', '/account', '/master-profile', '/admin/channels'].includes(location.pathname) ? 'my'
        : location.pathname === '/games/dice' ? 'experience' : 'home';
    const contextLinks = domain === 'models' ? [['/models', '模型目录'], ['/keys', '密钥管理'], ['/logs', '调用记录'], ['/playground', '文本测试']]
        : domain === 'assets' ? [['/wallet', '我的钱包'], ['/rewards', '奖励中心']]
        : domain === 'experience' ? [['/games/dice', '骰子体验']] : [];
    const [menu, setMenu] = useState(false);
    const main = useRef<HTMLElement>(null);
    const account = useRef<HTMLDivElement>(null);
    const trigger = useRef<HTMLButtonElement>(null);
    const master = useResource(() => readProfile(client, String(user.id)).catch(e => { throw new Error(profileError(e)); }), [client, user.id, location.pathname]);
    useEffect(() => { setMenu(false); document.title = (pageTitles[location.pathname] || '指挥台') + ' · momiao'; main.current?.focus(); }, [location.pathname]);
    useEffect(() => {
        if (!menu) return;
        const outside = (e: PointerEvent) => { if (!account.current?.contains(e.target as Node)) setMenu(false); };
        document.addEventListener('pointerdown', outside);
        return () => document.removeEventListener('pointerdown', outside);
    }, [menu]);
    const masterName = master.loading ? '正在读取 Master' : master.error ? 'Master 资料待核对' : master.data?.status === 'COMPLETE' ? master.data.display_name : 'Master 资料未完成';
    return <div className="portal-shell"><a className="skip-link" href="#main-content">跳至主要内容</a>
        <header className="portal-header"><Link to="/" className="brand" aria-label="Chaldea Platform 首页"><Brand /></Link>
            <nav className="portal-global" aria-label="主导航">
                <Link to="/dashboard" aria-current={domain === 'home' ? 'page' : undefined}>指挥台</Link>
                <Link to="/models" aria-current={domain === 'models' ? 'true' : undefined}>模型目录</Link>
                <button disabled aria-label="娱乐（未开放）">娱乐<small>未开放</small></button>
                <Link to="/announcements">公告</Link>
            </nav>
            <div className="portal-account"><Link className="asset-shortcut" to="/wallet" aria-label="资产快捷入口" aria-current={domain === 'assets' ? 'true' : undefined}><span aria-hidden="true">◇</span> 资产</Link><div className="account-wrap" ref={account} onKeyDown={e => { if (e.key === 'Escape' && menu) { setMenu(false); trigger.current?.focus(); } }} onBlur={e => { if (!e.currentTarget.contains(e.relatedTarget)) setMenu(false); }}>
            <button ref={trigger} className="account-button" aria-label="账户菜单" aria-expanded={menu} aria-controls="account-menu" onClick={() => setMenu(!menu)}><span className="avatar"><Crest /></span><span>{masterName}<small>Master 身份</small></span><span aria-hidden="true">⌄</span></button>
            {menu && <div className="account-menu" id="account-menu"><strong>{user.display_name || user.username}</strong><span>原生登录身份 · {user.username}</span><span>{role(user.role)}</span><Link to="/me">个人中心</Link><Link to="/master-profile">Master 资料</Link><Link to="/rewards">奖励中心</Link>{user.role >= 10 && <Link to="/admin/channels">渠道管理</Link>}<button onClick={() => void client.logout().catch(() => {})}>退出登录</button></div>}
        </div></div></header>
        {contextLinks.length > 0 && <nav className="portal-context" aria-label="页面导航">{contextLinks.map(([path, title]) => <NavLink to={path} key={path}>{title}</NavLink>)}</nav>}
        <main ref={main} id="main-content" tabIndex={-1} className="main-content"><Outlet context={master} /></main>
        <footer className="workspace-foot"><span>momiao / Chaldea Platform <span> / </span><a href="https://github.com/cy4268/momiao" target="_blank" rel="noopener noreferrer">源代码</a></span><span>你的连接，由你掌握。</span></footer>
        <nav className="mobile-nav" aria-label="底部导航">
            <Link to="/dashboard" aria-current={domain === 'home' ? 'page' : undefined}>首页</Link>
            <Link to="/models" aria-current={domain === 'models' ? 'true' : undefined}>模型</Link>
            <button disabled aria-label="娱乐（未开放）" aria-current={domain === 'experience' ? 'true' : undefined}>娱乐<small>未开放</small></button>
            <Link to="/wallet" aria-current={domain === 'assets' ? 'true' : undefined}>资产</Link>
            <Link to="/me" aria-current={domain === 'my' ? 'true' : undefined}>我的</Link>
        </nav>
    </div>;
}
function Dashboard({ client, user }: { client: ApiClient; user: User }) {
    const master = useOutletContext<MasterResource>();
    const r = useResource(async () => { const [self, keys, logs] = await Promise.all([client.loadSelf(), client.keys(1, 5), client.logs('p=1&page_size=5')]); return { self, keys, logs }; }, [client]);
    const current = r.data?.self || user;
    return <><header className="page-heading"><div><p className="eyebrow">CHALDEA / COMMAND CENTER</p><h1>指挥台</h1><p>欢迎回来。管理连接，开始下一次创造。</p></div><Link className="button primary" to="/models">选择模型并测试 <span aria-hidden="true">↗</span></Link></header>
        <MasterSummary master={master} />
        <div className="dashboard-paths"><Link to="/me">个人中心 →</Link><Link to="/rewards">领取每日奖励 →</Link><Link to="/wallet/activate">激活 API 额度 →</Link><span>原生登录身份：{current.display_name} · {current.username}</span></div>
        <section className="account-ledger" aria-label="账户使用概览"><div><p>可用原生额度</p><strong>{number(current.quota)}</strong><span>原生单位</span></div><div><p>已用原生额度</p><strong>{number(current.used_quota)}</strong><span>原生单位</span></div><div><p>累计请求</p><strong>{number(current.request_count)}</strong><span>次调用</span></div><div><p>API 密钥</p><strong>{r.loading ? '…' : r.error ? '—' : number(r.data?.keys.total)}</strong><Link to="/keys">管理密钥 ↗</Link></div></section><p className="ledger-note">原生额度为当前服务的计量单位，与平台本地钱包独立。<Link className="text-link" to="/wallet">查看 Reserve API Credit 与可用筹码 →</Link></p>
        <section className="panel recent"><div className="section-heading"><div><p className="eyebrow">RECENT ACTIVITY</p><h2>最近调用与活动</h2></div><Link className="text-link" to="/logs">查看全部 →</Link></div>{r.loading ? <Loading /> : r.error ? <><Alert>{r.error}</Alert><button onClick={r.reload}>重新加载</button></> : r.data && <>{r.data.keys.total === 0 && <div className="first-key"><div><strong>建立你的第一个连接</strong><p>创建一枚独立密钥，再将它添加到你信任的应用。</p></div><Link className="button" to="/keys">创建第一枚密钥 →</Link></div>}<LogTable items={r.data.logs.items} /></>}</section>
    </>;
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
