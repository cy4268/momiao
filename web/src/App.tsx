import { useEffect, useRef, useState, useSyncExternalStore, type FormEvent, type ReactNode } from 'react';
import { Link, Navigate, Outlet, Route, Routes, useLocation, useNavigate, useOutletContext, useParams } from 'react-router-dom';
import { api, ApiClient, type User, type UsageLog, errorText } from './api';
import { Keys } from './Keys';
import { RPUsage } from './RPUsage';
import { ApiWorkbench, UsageNavigation } from './ApiWorkbench';
import { Catalog, CatalogDetailPage, APIAccess } from './Catalog';
import { OpsCatalog } from './OpsCatalog';
import { OpsShell, OpsHome, OpsLegacyBoundary } from './ops/OpsShell';
import { OpsAttention, OpsJobs, OpsServiceHealth } from './ops/OpsRuntime';
import { OpsOperationDetail } from './ops/OpsOperationDetail';
import { OpsAudit } from './ops/OpsAudit';
import { OpsMaintenance } from './ops/OpsMaintenance';
import { OpsAccessControl } from './ops/OpsAccessControl';
import { OpsEconomy } from './ops/OpsEconomy';
import { OpsRewards } from './ops/OpsRewards';
import { OpsUsers } from './ops/OpsUsers';
import { OpsLogs } from './ops/OpsLogs';
import { OpsSupportCases } from './ops/OpsCases';
import { OpsIncidents } from './ops/OpsIncidents';
import { OpsRecords } from './ops/OpsRecords';
import { OpsGames } from './ops/OpsGames';
import { OpsPoker } from './ops/OpsPoker';
import { OpsRankings } from './ops/OpsRankings';
import { CriticalNotice } from './CriticalNotice';
import { isOpsDiscordPopup, OpsDiscordPopup } from './ops/OpsDiscordPopup';
import type { CatalogLoginRequired } from './catalog-api';
import { Channels, Groups } from './Channels';
import { Wallet } from './Wallet';
import { QuotaActivation } from './QuotaActivation';
import { GamesCatalog, GamePage } from './Games';
import {RouletteLobby} from './roulette/RouletteLobby';
import {RouletteHistory} from './roulette/RouletteHistory';
import {RouletteRoom} from './roulette/RouletteRoom';
import { HistoryList, HistoryRound, HistorySession, HistoryHand, HistoryTransaction } from './history/History';
import { MasterProfile } from './MasterProfile';
import { Home } from './Home';
import { Announcements, AnnouncementDetail, AnnouncementEntry } from './Announcements';
import { OpsAnnouncements } from './OpsAnnouncements';
import { PersonalHub, MasterSummary, CommandChamber, type MasterResource } from './PersonalHub';
import { Rewards } from './Rewards';
import { readProfile, profileError } from './profile-api';
import { Alert, Crest, Empty, Loading, Pager, date, number, useResource } from './ui';
import { Authentication, DiscordCallback, type CapturedCallback } from './Authentication';
import { Account } from './Account';
import { AccessGate, LoginRequired, PostAuthGate } from './PostAuthGate';
import type { SensitiveProof } from './admission-api';
import { PokerRoute } from './poker/PokerRoute';
import { Rankings } from './rankings/Rankings';
import { SiteHeader } from './SiteHeader';
const missingCallback:CapturedCallback={error:'授权回调已失效，请重新开始。'};
export function App({ client = api, capturedCallback = missingCallback, onCatalogLoginRequired }: { client?: ApiClient; capturedCallback?:CapturedCallback; onCatalogLoginRequired?: CatalogLoginRequired }) {
    const session = useSyncExternalStore(client.subscribe, client.getSnapshot);
    const navigate=useNavigate();const location=useLocation();
    const [pokerHold,setPokerHold]=useState<{client:ApiClient;userID:number;generation:number;owner:object;path:string}>();
    const [pokerNotice,setPokerNotice]=useState(false);
    const held=pokerHold?.client===client&&pokerHold.userID===session.user?.id&&pokerHold.generation===client.getSessionGeneration()&&!session.loggingOut?pokerHold:undefined;
    const effectiveLocation=held?{...location,pathname:held.path,search:'',hash:''}:location;
    const effectivePath=effectiveLocation.pathname+effectiveLocation.search+effectiveLocation.hash;
    useEffect(()=>{
        if(!held){setPokerNotice(false);return;}
        if(location.pathname+location.search+location.hash!==held.path){setPokerNotice(true);navigate(held.path,{replace:true});}
        const leave=(event:BeforeUnloadEvent)=>{event.preventDefault();event.returnValue='';};
        window.addEventListener('beforeunload',leave);return()=>window.removeEventListener('beforeunload',leave);
    },[held,location.pathname,location.search,location.hash,navigate]);
    const holdPoker=(owner:object,path:string|null)=>{
        if(path===null){setPokerHold(previous=>previous?.owner===owner?undefined:previous);return;}
        const auth=client.getSnapshot();if(!auth.ready||!auth.user||auth.loggingOut)return;
        setPokerHold({client,userID:auth.user.id,generation:client.getSessionGeneration(),owner,path});
    };
    const [proof,setProof]=useState<{value:SensitiveProof;generation:number}>();
    useEffect(()=>{if(proof&&(proof.generation!==client.getSessionGeneration()||!['/account','/account/security','/oauth/discord'].includes(location.pathname)))setProof(undefined);},[session.user,client,location.pathname,proof]);
    useEffect(() => { if(!isOpsDiscordPopup())void client.bootstrap(); }, [client]);
    const loading = <div className="session-loading"><Crest /><Loading /></div>;
    const announcementSession = (session.user?.id || 'guest') + ':' + client.getSessionGeneration() + ':' + session.ready;
    const routedSession=announcementSession+':'+location.pathname+location.search;
    // Credential rotation must not discard the account form's completed result.
    const privateRouteSession=['/account','/account/security'].includes(location.pathname)?session.user?.id+':'+location.pathname:routedSession;
    return <>{location.pathname !== '/' && <CriticalNotice client={client}/>}{location.pathname !== '/' && session.ready && !session.user && <AnnouncementEntry client={client} />}{pokerNotice&&held&&<p role="status">{held.path==='/poker'?'入桌结果正在自动同步，已保留当前页面。':'当前牌桌会话仍在进行，安全离座后即可切换页面。'}</p>}<Routes location={effectiveLocation}>
        <Route path="/" element={<Home key={announcementSession} signedIn={!!session.user} client={client} />} />
        <Route path="/entertainment" element={<GamesCatalog key={announcementSession} client={client}/>} />
        <Route path="/games" element={<GamesCatalog key={announcementSession} client={client}/>} />
        <Route path="/rankings" element={<Rankings key={announcementSession} client={client}/>} />
        <Route path="/announcements" element={<Announcements key={announcementSession} client={client} />} />
        <Route path="/announcements/:id" element={<AnnouncementDetail key={announcementSession} client={client} />} />
        <Route path="/models" element={<Catalog key={routedSession} client={client} />} />
        <Route path="/models/*" element={<CatalogDetailPage key={routedSession} client={client} onLoginRequired={onCatalogLoginRequired} />} />
        <Route path="/api/access" element={<APIAccess key={routedSession} client={client} onLoginRequired={onCatalogLoginRequired} />} />
        <Route path="/ops" element={!session.ready?loading:session.user?<AccessGate key={routedSession} client={client} user={session.user} route={location.pathname}><OpsShell client={client} user={session.user}/></AccessGate>:<LoginRequired/>}>
            <Route path="models" element={<OpsCatalog client={client} embedded/>}/>
            <Route path="channels" element={<Channels client={client}/>}/>
            <Route path="channels/:channelID" element={<Channels client={client}/>}/>
            <Route path="groups" element={<Groups client={client}/>}/>
            <Route path="users" element={<OpsUsers client={client} user={session.user!}/>}/>
            <Route path="users/:userID" element={<OpsUsers client={client} user={session.user!}/>}/>
            <Route path="logs" element={<OpsLogs key={location.search} client={client}/>}/>
            <Route element={<OpsLegacyBoundary/>}>
            <Route path="announcements" element={<OpsAnnouncements client={client} embedded/>}/>
            <Route path="jobs" element={<OpsJobs/>}/>
            <Route path="attention" element={<OpsAttention/>}/>
            <Route path="operations/:operationID" element={<OpsOperationDetail/>}/>
            <Route path="access-control" element={<OpsAccessControl/>}/>
            <Route path="economy" element={<OpsEconomy/>}/>
            <Route path="rewards" element={<OpsRewards/>}/>
            <Route path="support-cases" element={<OpsSupportCases/>}/>
            <Route path="incidents" element={<OpsIncidents/>}/>
            <Route path="records" element={<OpsRecords/>}/>
            <Route path="games" element={<OpsGames/>}/>
            <Route path="poker" element={<OpsPoker/>}/>
            <Route path="rankings" element={<OpsRankings/>}/>
            <Route path="service-health" element={<OpsServiceHealth/>}/>
            </Route>
            <Route path="maintenance" element={<OpsMaintenance/>}/>
            <Route path="operations" element={<Navigate to="/ops/audit" replace/>}/>
            <Route path="audit" element={<OpsAudit/>}/>
            <Route path="audit/:auditID" element={<OpsAudit/>}/>
            <Route index element={<OpsHome/>}/>
        </Route>
        <Route path="/login" element={!session.ready?loading:session.user?<Navigate to="/welcome" replace/>:<Authentication client={client} mode="login"/>} />
        <Route path="/register" element={!session.ready?loading:session.user?<Navigate to="/welcome" replace/>:<Authentication client={client} mode="registration"/>} />
        <Route path="/oauth/discord" element={isOpsDiscordPopup()?<OpsDiscordPopup captured={capturedCallback}/>:!session.ready?loading:<DiscordCallback client={client} captured={capturedCallback} onComplete={value=>{if(value){setProof({value,generation:client.getSessionGeneration()});navigate('/account/security',{replace:true});}else navigate('/welcome',{replace:true});}}/>}/>
        <Route path="/welcome" element={!session.ready?loading:session.user?<PostAuthGate key={session.user.id+':'+client.getSessionGeneration()} client={client} user={session.user}/>:<Navigate to="/login" replace/>}/>
        <Route path="/poker/*" element={!session.ready?loading:session.user?<AccessGate key={announcementSession} client={client} user={session.user} route="/poker" pokerRecovery><PokerRoute key={announcementSession} client={client} path={effectivePath} onHold={holdPoker} frame={children=><Shell client={client} user={session.user!}>{children}</Shell>}/></AccessGate>:<LoginRequired/>}/>
        <Route element={!session.ready ? loading : session.user ? <AccessGate key={privateRouteSession} client={client} user={session.user} route={location.pathname+(['/keys','/logs'].includes(location.pathname)?location.search:'')}><Shell client={client} user={session.user} /></AccessGate> : <LoginRequired />}>
            <Route path="/account" element={<Account key={session.user?.id} client={client} proof={proof?.generation===client.getSessionGeneration()?proof.value:undefined} clearProof={()=>setProof(undefined)}/>}/>
            <Route path="/account/security" element={<Account key={session.user?.id} client={client} proof={proof?.generation===client.getSessionGeneration()?proof.value:undefined} clearProof={()=>setProof(undefined)}/>}/>
            <Route path="/dashboard" element={<Dashboard client={client} user={session.user!} />} />
            <Route path="/me" element={<PersonalHub client={client} user={session.user!} />} />
            <Route path="/rewards" element={<Rewards client={client} user={session.user!} />} />
            {(['dice','scratch','summon','slot','blackjack'] as const).map(slug=><Route key={slug} path={'/games/'+slug} element={session.user&&<GamePage key={session.user.id+':'+client.getSessionGeneration()+':'+slug} client={client} userID={String(session.user.id)} slug={slug}/>}/>)}
            <Route path="/history" element={<HistoryList key={routedSession} client={client}/>} />
            {(['devil-roulette','pressure-roulette'] as const).map(slug=><Route key={slug} path={'/roulette/'+slug} element={session.user&&<RouletteLobby key={routedSession+slug} client={client} userID={String(session.user.id)} slug={slug}/>}/>)}
            <Route path="/roulette/rooms/:id" element={session.user&&<RouletteRoom key={routedSession} client={client} userID={String(session.user.id)}/>}/>
            <Route path="/history/:id" element={<LegacyRoundRedirect/>} />
            <Route path="/history/roulette/:id" element={<RouletteHistory key={routedSession} client={client}/>}/>
            <Route path="/history/rounds/:id" element={<HistoryRound key={routedSession} client={client}/>} />
            <Route path="/history/sessions/:id" element={<HistorySession key={routedSession} client={client}/>} />
            <Route path="/history/hands/:id" element={<HistoryHand key={routedSession} client={client}/>} />
            <Route path="/wallet/transactions/:id" element={<HistoryTransaction key={routedSession} client={client}/>} />
            <Route path="/wallet/activate" element={session.user && <QuotaActivation key={session.user.id + ':' + client.getSessionGeneration()} client={client} userID={String(session.user.id)} />} />
            <Route path="/wallet" element={<Wallet client={client} user={session.user!} />} />
            <Route path="/master-profile" element={<ProfileRoute client={client} user={session.user!} />} />
            <Route path="/admin/channels" element={<Navigate to="/ops/channels" replace/>} />
            <Route path="/keys" element={<Keys client={client} />} />
            <Route path="/logs" element={<Logs client={client} />} />
        </Route>
        <Route path="*" element={!session.ready ? loading : <Navigate to={session.user ? '/dashboard' : '/login'} replace />} />
    </Routes></>;
}
function LegacyRoundRedirect() {
    const {id}=useParams();
    return <Navigate to={id&&/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(id)?`/history/rounds/${id}`:'/history'} replace/>;
}
function ProfileRoute({ client, user }: { client: ApiClient; user: User }) {
    const master = useOutletContext<MasterResource>();
    return <MasterProfile client={client} user={user} onSaved={master.reload} />;
}
const pageTitles: Record<string, string> = {
    '/dashboard': '指挥台', '/me': '个人中心', '/rewards': '奖励中心', '/games/dice': '命运骰盅', '/games/scratch':'星纹刮刮卡', '/games/summon':'圣晶召唤', '/games/slot':'月光回响', '/games/blackjack':'二十一点', '/history':'游戏记录',
    '/master-profile': 'Master 资料', '/account': '账户与安全', '/account/security': '账户与安全', '/wallet/activate': '转入 API 额度', '/wallet': '我的钱包', '/poker':'Poker 大厅',
    '/models': '模型目录', '/admin/channels': '渠道管理', '/keys': '密钥管理', '/logs': '调用记录',
};
function Shell({ client, user, children }: { client: ApiClient; user: User; children?:ReactNode }) {
    const location = useLocation();
    const domain = ['/models', '/keys', '/logs'].includes(location.pathname) ? 'models'
        : ['/wallet', '/wallet/activate', '/rewards'].includes(location.pathname) || location.pathname.startsWith('/wallet/transactions/') ? 'assets'
        : ['/me', '/account', '/account/security', '/master-profile', '/admin/channels'].includes(location.pathname) ? 'my'
        : location.pathname==='/poker' || location.pathname.startsWith('/games/') || location.pathname.startsWith('/roulette/') || location.pathname.startsWith('/history') ? 'experience' : 'home';
    const main = useRef<HTMLElement>(null);
    const master = useResource(() => readProfile(client, String(user.id)).catch(e => { throw new Error(profileError(e)); }), [client, user.id, location.pathname]);
    useEffect(() => { document.title = (pageTitles[location.pathname] || (location.pathname.startsWith('/wallet/transactions/') ? '钱包交易详情' : location.pathname.startsWith('/history/sessions/') ? '牌桌会话详情' : location.pathname.startsWith('/history/hands/') ? '手牌详情' : location.pathname.startsWith('/history/') ? '本局详情' : '指挥台')) + ' · momiao'; main.current?.focus(); }, [location.pathname]);
    return <div className="portal-shell"><a className="skip-link" href="#main-content">跳至主要内容</a>
        <SiteHeader client={client} master={master} />
        <main ref={main} id="main-content" tabIndex={-1} className="main-content">{children??<Outlet context={master} />}</main>
        <footer className="workspace-foot"><span>momiao / Chaldea Platform <span> / </span><a href="https://github.com/cy4268/momiao" target="_blank" rel="noopener noreferrer">源代码</a></span><span>你的连接，由你掌握。</span></footer>
        <nav className="mobile-nav" aria-label="底部导航">
            <Link to="/dashboard" aria-current={domain === 'home' ? 'page' : undefined}>首页</Link>
            <Link to="/models" aria-current={domain === 'models' ? 'true' : undefined}>模型</Link>
            <Link to="/entertainment" aria-current={domain === 'experience' ? 'true' : undefined}>娱乐</Link>
            <Link to="/wallet" aria-current={domain === 'assets' ? 'true' : undefined}>资产</Link>
            <Link to="/me" aria-current={domain === 'my' ? 'true' : undefined}>我的</Link>
        </nav>
    </div>;
}
function Dashboard({ client, user }: { client: ApiClient; user: User }) {
    const master = useOutletContext<MasterResource>();
    const r = useResource(async () => { const [self, keys, logs] = await Promise.all([client.loadSelf(), client.keys(1, 5), client.logs('p=1&page_size=5')]); return { self, keys, logs }; }, [client]);
    const current = r.data?.self || user;
    return <CommandChamber page="dashboard"><header className="page-heading"><div><p className="eyebrow">COMMAND CENTER</p><h1>指挥台</h1><p>欢迎回来，御主。</p></div><Link className="button primary" to="/models">选择模型并接入</Link></header>
        <MasterSummary master={master} />
        <div className="chamber-ledger"><section className="account-ledger" aria-label="账户使用概览"><div><p>Reserve 余额</p><strong>{current.reserve_amount ?? '—'}</strong></div><div><p>已用 API 额度</p><strong>{number(current.used_quota ?? undefined)}</strong></div><div><p>累计请求</p><strong>{number(current.request_count ?? undefined)}</strong><span>次调用</span></div><div><p>API 密钥</p><strong>{r.loading ? '…' : r.error ? '—' : number(r.data?.keys.total)}</strong><Link to="/keys">管理密钥</Link></div></section><p className="ledger-note"><Link className="text-link" to="/wallet">查看 Reserve 与筹码</Link></p></div>
        <div className="dashboard-paths"><Link to="/me">个人中心</Link><Link to="/rewards">领取每日奖励</Link><Link to="/wallet/activate">激活 API 额度</Link></div>
        <section className="panel recent"><div className="section-heading"><h2>最近调用与活动</h2><Link className="text-link" to="/logs">查看全部</Link></div><div className="chamber-activity" role="region" aria-label="最近调用内容" tabIndex={0}>{r.loading ? <Loading /> : r.error ? <><Alert>{r.error}</Alert><button onClick={r.reload}>重新加载</button></> : r.data && <>{r.data.keys.total === 0 && <div className="first-key"><div><strong>建立你的第一个连接</strong><p>创建一枚独立密钥，再将它添加到你信任的应用。</p></div><Link className="button" to="/keys">创建第一枚密钥</Link></div>}<LogTable items={r.data.logs.items} /></>}</div></section>
    </CommandChamber>;
}
const logTypes: Record<number, string> = { 1: '充值', 2: '消费', 3: '管理', 4: '系统', 5: '错误', 6: '退款', 7: '登录' };
function LogTable({ items }: {
    items: UsageLog[];
}) { return items.length === 0 ? <Empty title="暂无记录">开始使用 API 后，在这里查看真实调用和额度消耗；筛选后无结果时，可调整筛选条件。</Empty> : <div className="table-wrap" role="region" aria-label="个人调用记录" tabIndex={0}><table><thead><tr><th>时间 / 类型</th><th>模型</th><th>密钥名称</th><th>输入 / 输出 Tokens</th><th>额度</th></tr></thead><tbody>{items.map(log => <tr key={log.id}><td data-label="时间 / 类型"><time>{date(log.created_at)}</time><small>{logTypes[log.type] || '其他'}</small></td><td data-label="模型"><strong>{log.model_name || '—'}</strong></td><td data-label="密钥名称">{log.token_name || '—'}</td><td data-label="输入 / 输出 Tokens" className="numeric">{number(log.prompt_tokens)} <span className="muted">/</span> {number(log.completion_tokens)}</td><td data-label="额度" className="numeric">{number(log.quota)}</td></tr>)}</tbody></table></div>; }
function Logs({ client }: {
    client: ApiClient;
}) {
    const location=useLocation();
    return <ApiWorkbench page="logs">{new URLSearchParams(location.search).get('purpose')==='ROLEPLAY'?<RPUsage client={client}/>:<NativeLogs client={client}/>}</ApiWorkbench>;
}
function NativeLogs({ client }: {
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
    return <><header className="page-heading"><div><p className="eyebrow">USAGE LOGS</p><h1>调用记录</h1><p>查看个人活动、模型调用与 API 额度消耗。</p></div><button onClick={r.reload} disabled={r.loading}>刷新记录</button></header><UsageNavigation/><section className="api-ledger"><form className="filters" onSubmit={filter}><label>记录类型<select value={type} onChange={e => setType(e.target.value)}><option value="0">全部类型</option>{Object.entries(logTypes).map(([id, name]) => <option value={id} key={id}>{name}</option>)}</select></label><label className="model-filter">模型名称<input value={model} onChange={e => setModel(e.target.value)} maxLength={200} placeholder="完整模型名"/></label><label>开始日期<input type="date" value={start} onChange={e => setStart(e.target.value)}/></label><label>结束日期<input type="date" value={end} onChange={e => setEnd(e.target.value)}/></label><div className="filter-actions"><button type="submit">应用筛选</button><button type="button" onClick={reset}>重置</button></div></form>{error && <Alert>{error}</Alert>}<p className="hint">时间按设备所在时区显示。仅展示调用元数据，不展示提示词或响应内容。</p>{r.loading ? <Loading /> : r.error ? <><Alert>{r.error}</Alert><button onClick={r.reload}>重新加载</button></> : r.data && <><LogTable items={r.data.items}/><Pager page={page} total={r.data.total} size={r.data.page_size} onChange={setPage}/></>}</section></>;
}
