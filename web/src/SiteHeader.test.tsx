import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { ApiClient, type User } from './api';
import * as profileAPI from './profile-api';
import { navDomain, SiteHeader } from './SiteHeader';

const user: User = { id: 7, username: 'dc_dfb941334b71d301', display_name: 'Fixture', role: 1 } as User;
let client: ApiClient;
function session(signedIn: boolean, ready = true) {
    vi.spyOn(client, 'getSnapshot').mockReturnValue({ user: signedIn ? user : null, ready, loggingOut: false, notice: '' });
}
function show(path: string) {
    return render(<MemoryRouter initialEntries={[path]}><SiteHeader client={client} /></MemoryRouter>);
}
const mainLinks = () => within(screen.getByRole('navigation', { name: '主导航' })).getAllByRole('link').map(a => a.textContent);
const profile = (display_name: string) => ({ user_id: '7', short_account_id: 'CA-1', status: 'COMPLETE', display_name, avatar_id: 'system-default', profile_version: '1', nickname_changed_at: null, next_rename_at: null, suggested_name: '', avatars: [{ id: 'system-default', label: '系统默认头像', source: 'SYSTEM' }] }) as profileAPI.MasterProfileData;

beforeEach(() => {
    client = new ApiClient();
    vi.spyOn(profileAPI, 'readProfile').mockResolvedValue(profile('墨恒'));
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

it('navDomain maps every route family', () => {
    const cases: Record<string, ReturnType<typeof navDomain>> = {
        '/dashboard': 'home', '/models': 'models', '/models/x': 'models', '/api/access': 'models', '/keys': 'models', '/logs': 'models',
        '/games': 'experience', '/entertainment': 'experience', '/games/dice': 'experience', '/poker': 'experience', '/poker/t/1': 'experience',
        '/roulette/devil': 'experience', '/history': 'experience', '/history/rounds/x': 'experience', '/rankings': 'rankings',
        '/announcements': 'announcements', '/announcements/1': 'announcements', '/wallet': 'assets',
        '/wallet/transactions/x': 'assets', '/rewards': 'assets', '/me': 'my', '/account': 'my', '/account/security': 'my', '/master-profile': 'my', '/': null,
    };
    for (const [path, domain] of Object.entries(cases)) expect([path, navDomain(path)]).toEqual([path, domain]);
});
it('signed-in header has the same links on every page', async () => {
    session(true);
    for (const path of ['/dashboard', '/games', '/models', '/rankings', '/announcements']) {
        const view = show(path);
        expect(mainLinks()).toEqual(['指挥台', '模型目录', '娱乐', '排行榜', '公告']);
        expect(screen.getByRole('link', { name: '资产' })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: '账户菜单' })).toBeInTheDocument();
        await screen.findByText('墨恒');
        view.unmount();
    }
});
it('signed-out header', () => {
    session(false);
    show('/games');
    expect(mainLinks()).toEqual(['首页', '模型目录', '娱乐', '排行榜', '公告']);
    expect(screen.getByRole('link', { name: '登录' })).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: '资产' })).not.toBeInTheDocument();
});
it('marks the current domain', () => {
    session(true);
    show('/games/dice');
    const nav = screen.getByRole('navigation', { name: '主导航' });
    expect(within(nav).getByRole('link', { name: '娱乐' })).toHaveAttribute('aria-current', 'page');
    expect(within(nav).getAllByRole('link').filter(a => a.hasAttribute('aria-current'))).toHaveLength(1);
});
it('never shows sign-in text before session is ready', () => {
    session(false, false);
    show('/games');
    expect(screen.queryByText(/登录|个人中心/)).not.toBeInTheDocument();
    expect(screen.getByTestId('account-placeholder')).toBeInTheDocument();
});
it('renders cached master name on remount', async () => {
    session(true);
    const first = show('/games');
    await screen.findByText('墨恒');
    first.unmount();
    vi.mocked(profileAPI.readProfile).mockReturnValue(new Promise(() => {}));
    show('/dashboard');
    const button = screen.getByRole('button', { name: '账户菜单' });
    expect(button).toHaveTextContent('墨恒');
    expect(button).not.toHaveTextContent('正在读取');
});
it('account menu hides internal identity', async () => {
    session(true);
    const request = vi.spyOn(client, 'request').mockRejectedValue(new Error('unexpected ops bootstrap'));
    const view = show('/dashboard');
    fireEvent.click(screen.getByRole('button', { name: '账户菜单' }));
    const menu = await waitFor(() => document.getElementById('account-menu')!);
    expect(menu).not.toHaveTextContent('原生登录身份');
    expect(menu).not.toHaveTextContent(user.username);
    expect(within(menu).queryByRole('link', { name: '运营工作台' })).not.toBeInTheDocument();
    view.unmount();
    vi.mocked(client.getSnapshot).mockReturnValue({ user: { ...user, role: 10 }, ready: true, loggingOut: false, notice: '' });
    show('/dashboard');
    fireEvent.click(screen.getByRole('button', { name: '账户菜单' }));
    expect(await screen.findByRole('link', { name: '运营工作台' })).toHaveAttribute('href', '/ops');
    expect(request).not.toHaveBeenCalled();
});
it('shows the domain sub-navigation', () => {
    session(true);
    const subLinks = () => within(screen.getByRole('navigation', { name: '页面导航' })).getAllByRole('link').map(a => a.textContent);
    let view = show('/games');
    expect(subLinks()).toEqual(['游戏目录', '命运骰盅', '星纹刮刮卡', '圣晶召唤', '月光回响', '二十一点', 'Poker 大厅', '游戏记录']);
    view.unmount();
    view = show('/models');
    expect(subLinks()).toEqual(['发现模型', 'API 接入', '密钥管理', '调用记录']);
    view.unmount();
    show('/rankings');
    expect(screen.queryByRole('navigation', { name: '页面导航' })).not.toBeInTheDocument();
});
it('uses the shared master resource without a second profile request', () => {
    session(true);
    render(<MemoryRouter initialEntries={['/dashboard']}><SiteHeader client={client} master={{ data: profile('改名后的墨恒'), loading: false, error: '', reload: () => {} }} /></MemoryRouter>);
    expect(screen.getByRole('button', { name: '账户菜单' })).toHaveTextContent('改名后的墨恒');
    expect(profileAPI.readProfile).not.toHaveBeenCalled();
});
it('site header styles are scoped tightly enough to outrank page stylesheets', async () => {
    const { readFileSync } = await import('node:fs');
    const css = readFileSync('src/site-header.css', 'utf8').replace(/\/\*[\s\S]*?\*\//g, '');
    const selectors = [...css.matchAll(/([^{}@]+)\{[^{}]*\}/g)].flatMap(m => m[1].split(',').map(s => s.trim())).filter(Boolean);
    const loose = selectors.filter(s => !/^\.site-(header|subnav|chrome)\.site-(header|subnav|chrome)\b/.test(s));
    expect(loose).toEqual([]);
});
it('the brand link does not inherit legacy .brand page styles', () => {
    session(true);
    show('/dashboard');
    expect(screen.getByRole('link', { name: 'Chaldea Platform 首页' })).not.toHaveClass('brand');
});
