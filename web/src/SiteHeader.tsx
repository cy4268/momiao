import { useEffect, useRef, useState, useSyncExternalStore } from 'react';
import { Link, NavLink, useLocation } from 'react-router-dom';
import type { ApiClient } from './api';
import type { MasterResource } from './PersonalHub';
import { readOpsBootstrap } from './ops/ops-api';
import { saveRouteIntent } from './post-auth-intent';
import { readProfile } from './profile-api';
import { Brand, Crest, role, useResource } from './ui';
import './site-header.css';

export type NavDomain = 'home' | 'models' | 'experience' | 'rankings' | 'announcements' | 'assets' | 'my' | null;

const under = (path: string, root: string) => path === root || path.startsWith(root + '/');
export function navDomain(path: string): NavDomain {
    if (path === '/dashboard') return 'home';
    if (['/models', '/api/access', '/keys', '/logs'].some(root => under(path, root))) return 'models';
    if (['/games', '/entertainment', '/poker', '/roulette', '/history'].some(root => under(path, root))) return 'experience';
    if (under(path, '/rankings')) return 'rankings';
    if (under(path, '/announcements')) return 'announcements';
    if (['/wallet', '/rewards'].some(root => under(path, root))) return 'assets';
    if (['/me', '/account', '/master-profile', '/admin/channels'].some(root => under(path, root))) return 'my';
    return null;
}
const subnav: Partial<Record<Exclude<NavDomain, null>, [string, string][]>> = {
    models: [['/models', '发现模型'], ['/api/access', 'API 接入'], ['/keys', '密钥管理'], ['/logs', '调用记录']],
    assets: [['/wallet', '我的钱包'], ['/rewards', '奖励中心']],
    experience: [['/games', '游戏目录'], ['/games/dice', '命运骰盅'], ['/games/scratch', '星纹刮刮卡'], ['/games/summon', '圣晶召唤'], ['/games/slot', '月光回响'], ['/games/blackjack', '二十一点'], ['/poker', 'Poker 大厅'], ['/history', '游戏记录']],
};
// The header remounts whenever the page layout changes, so the last confirmed
// Master name paints the first frame instead of a loading label.
const masterNames = new WeakMap<ApiClient, Map<number, string>>();

export function SiteHeader({ client, master }: { client: ApiClient; master?: MasterResource }) {
    const session = useSyncExternalStore(client.subscribe, client.getSnapshot);
    const location = useLocation();
    const domain = navDomain(location.pathname);
    const user = session.ready ? session.user : null;
    const current = (d: NavDomain) => domain === d ? 'page' as const : undefined;
    const links: [string, string, NavDomain][] = user
        ? [['/dashboard', '指挥台', 'home'], ['/models', '模型目录', 'models'], ['/entertainment', '娱乐', 'experience'], ['/rankings', '排行榜', 'rankings'], ['/announcements', '公告', 'announcements']]
        : [['/', '首页', null], ['/models', '模型目录', 'models'], ['/entertainment', '娱乐', 'experience'], ['/rankings', '排行榜', 'rankings'], ['/announcements', '公告', 'announcements']];
    const context = domain ? subnav[domain] ?? [] : [];
    // One wrapper element: full-height pages lay out as a grid of chrome / main / footer.
    return <div className="site-chrome">
        <header className="site-header">
            <div className="site-header-inner">
                <Link to="/" className="site-brand" aria-label="Chaldea Platform 首页"><Brand /></Link>
                <nav className="site-nav" aria-label="主导航">{links.map(([to, label, d]) => <Link key={to} to={to} aria-current={d ? current(d) : undefined}>{label}</Link>)}</nav>
                <div className="site-account">
                    {!session.ready ? <span data-testid="account-placeholder" className="site-account-placeholder" aria-hidden="true" />
                        : user ? <><Link className="site-assets" to="/wallet" aria-current={current('assets')}>资产</Link><AccountMenu client={client} master={master} /></>
                        : <Link className="site-sign-in" to="/login" onClick={() => saveRouteIntent(location.pathname + location.search)}>登录</Link>}
                </div>
            </div>
        </header>
        {context.length > 0 && <nav className="site-subnav" aria-label="页面导航"><div className="site-header-inner">{context.map(([to, label]) => <NavLink key={to} to={to} end>{label}</NavLink>)}</div></nav>}
    </div>;
}

function AccountMenu({ client, master: shared }: { client: ApiClient; master?: MasterResource }) {
    const user = client.getSnapshot().user!;
    const location = useLocation();
    const [menu, setMenu] = useState(false);
    const account = useRef<HTMLDivElement>(null), trigger = useRef<HTMLButtonElement>(null);
    // Shell pages share their profile resource so a saved rename shows here at once; public pages read their own.
    const own = useResource(() => shared ? Promise.resolve(undefined) : readProfile(client, String(user.id)), [client, user.id, !!shared]);
    const master = shared ?? own;
    const opsEntry = useResource(async () => { if (!menu || client.getSessionMode() !== 'opaque') return false; try { return (await readOpsBootstrap(client)).principal.permissions.length > 0; } catch { return false; } }, [client, user.id, menu]);
    const names = masterNames.get(client) ?? new Map<number, string>();
    masterNames.set(client, names);
    if (master.data?.status === 'COMPLETE') names.set(user.id, master.data.display_name);
    const name = names.get(user.id) ?? (user.display_name || user.username);
    useEffect(() => setMenu(false), [location.pathname]);
    useEffect(() => {
        if (!menu) return;
        const outside = (e: PointerEvent) => { if (!account.current?.contains(e.target as Node)) setMenu(false); };
        document.addEventListener('pointerdown', outside);
        return () => document.removeEventListener('pointerdown', outside);
    }, [menu]);
    return <div className="account-wrap" ref={account} onKeyDown={e => { if (e.key === 'Escape' && menu) { setMenu(false); trigger.current?.focus(); } }} onBlur={e => { if (!e.currentTarget.contains(e.relatedTarget)) setMenu(false); }}>
        <button ref={trigger} className="account-button" aria-label="账户菜单" aria-expanded={menu} aria-controls="account-menu" onClick={() => setMenu(!menu)}><span className="avatar"><Crest /></span><span>{name}<small>{role(user.role)}</small></span><span aria-hidden="true">⌄</span></button>
        {menu && <div className="account-menu" id="account-menu"><strong>{name}</strong><Link to="/me">个人中心</Link><Link to="/master-profile">Master 资料</Link><Link to="/rewards">奖励中心</Link>{opsEntry.data && <Link to="/ops">运营工作台</Link>}{user.role >= 10 && <Link to="/admin/channels">渠道管理</Link>}<button onClick={() => void client.logout().catch(() => {})}>退出登录</button></div>}
    </div>;
}
