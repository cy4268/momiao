import { withReadyAccessGate } from './m1-test-fixtures';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { it, expect, vi } from 'vitest';
import { ApiClient } from './api';
import { App } from './App';
import type { GameBootstrap } from './games-api';
import { fixtureClient } from './m1-test-fixtures';
const user = { id: 1, username: 'test-user', display_name: 'Test User', role: 1, quota: 100, used_quota: 0, request_count: 0 };
const ok = (data?: unknown) => new Response(JSON.stringify({ success: true, data }));
const bundle = { access_token: 'synthetic-token', access_expires_at: 9999999999, user, session: { sid: 'test-sid' } };
const list = { items: [], total: 0, page: 1, page_size: 10 };
it.each([
    ['/dashboard', '/dashboard', []], ['/me', '/me', []],
    ['/rewards', '/wallet', ['/wallet', '/rewards']], ['/wallet/activate', '/wallet', ['/wallet', '/rewards']],
    ['/keys', '/models', ['/models', '/api/access', '/keys', '/logs']],
])('R1 keeps five bottom destinations and only the current context on %s', async (path, selected, contextPaths) => {
    const { client } = fixtureClient();
    render(<MemoryRouter initialEntries={[path as string]}><App client={client} /></MemoryRouter>);
    const bottom = await screen.findByRole('navigation', { name: '底部导航' });
    for (const [name, href] of [['首页', '/dashboard'], ['模型', '/models'], ['资产', '/wallet'], ['我的', '/me']]) {
        expect(within(bottom).getByRole('link', { name })).toHaveAttribute('href', href);
    }
    expect(bottom.children).toHaveLength(5);
    expect(within(bottom).getByRole('link', { name: '娱乐' })).toHaveAttribute('href', '/entertainment');
    expect(bottom.querySelector('[href="' + selected + '"]')).toHaveAttribute('aria-current');
    expect(bottom.querySelector('a[href="/games/dice"]')).toBeNull();
    const context = screen.queryByRole('navigation', { name: '页面导航' });
    if (!contextPaths.length) expect(context).not.toBeInTheDocument();
    else expect(within(context!).getAllByRole('link').map(link => link.getAttribute('href'))).toEqual(contextPaths);
});
it('R1 separates desktop global routes, asset shortcut and account destinations', async () => {
    const { client } = fixtureClient();
    render(<MemoryRouter initialEntries={['/keys']}><App client={client} /></MemoryRouter>);
    const global = await screen.findByRole('navigation', { name: '主导航' });
    expect(within(global).getAllByRole('link').map(link => link.getAttribute('href'))).toEqual(['/dashboard', '/models', '/entertainment', '/rankings', '/announcements']);
    expect(within(global).getByRole('link', { name: '模型目录' })).toHaveAttribute('aria-current');
    expect(within(global).getByRole('link', { name: '娱乐' })).toHaveAttribute('href', '/entertainment');
    expect(within(global).getByRole('link', { name: '公告' })).toHaveAttribute('href', '/announcements');
    expect(screen.getByRole('link', { name: '资产快捷入口' })).toHaveAttribute('href', '/wallet');
    fireEvent.click(screen.getByRole('button', { name: '账户菜单' }));
    const menu = within(document.getElementById('account-menu')!);
    expect(menu.getByRole('link', { name: '个人中心' })).toHaveAttribute('href', '/me');
    expect(menu.queryByRole('link', { name: '渠道管理' })).not.toBeInTheDocument();
});
it('R1 preserves the admin account entry and entertainment discovery', async () => {
    const { client } = fixtureClient(p => p.includes('/auth/refresh') ? ok({ ...bundle, user: { ...user, role: 10 } }) : p === '/api/user/self' ? ok({ ...user, role: 10 }) : undefined);
    render(<MemoryRouter initialEntries={['/me']}><App client={client} /></MemoryRouter>);
    fireEvent.click(await screen.findByRole('button', { name: '账户菜单' }));
    expect(within(document.getElementById('account-menu')!).getByRole('link', { name: '渠道管理' })).toHaveAttribute('href', '/admin/channels');
    expect(within(screen.getByRole('main')).getByRole('link', { name: '探索游戏' })).toHaveAttribute('href', '/entertainment');
});
it('reads the authenticated dice bootstrap without automatically wagering or moving funds', async () => {
    const id='01993200-0000-7000-8000-000000000001',hash='a'.repeat(64);
    const bootstrap:GameBootstrap={
        game:{slug:'dice',title:'命运骰盅',effective_runtime:'PLAY',implementation_key:'direct.dice.v1',config:{version_id:id,hash,schema:'dice-config-v1',ruleset_version:'dice-rules-v1',algorithm_version:'dice-map-v1',statistics:{rtp:'35/36',loss:'37/72',win:'35/72',top:'0',break_even:'0'}}},
        wager_policy:{version_id:id,version:'1',minimum_wager_units:'5000000',maximum_mode:'NONE',input_step_units:'500000',quick_amount_units:['5000000'],hash},
        available_units:'500000000',latest_round:null,active_round:null,scratch_presentation_blocker:null,effective_entry_action:'PLAY',
        next_commitment:{id,reserved_round_id:id,server_seed_hash:hash,nonce:'0',client_seed:'cs1-fixture',client_seed_version:'1',config_version_id:id,config_hash:hash,ruleset_version:'dice-rules-v1',algorithm_version:'dice-map-v1',fairness_stream_version:'chaldea-pf-hmac-sha256-v1',wager_policy_version_id:id,wager_policy_hash:hash,resource_versions:{}},
        client_seed_preference:{client_seed:'cs1-fixture',version:'1'},csrf_token:hash,
    };
    const {client,fetcher}=fixtureClient(path=>path==='/api/v1/games/dice/bootstrap'?ok(bootstrap):undefined);
    render(<MemoryRouter initialEntries={['/games/dice']}><App client={client} /></MemoryRouter>);
    await screen.findByRole('radio',{name:/^大/});
    expect(screen.getByRole('button',{name:'掷骰'})).toBeDisabled();
    fireEvent.click(screen.getByRole('radio',{name:/^大/}));
    expect(screen.getByRole('button',{name:'掷骰'})).toBeEnabled();
    expect(document.title).toBe('命运骰盅 · momiao');
    expect(screen.getByRole('link', { name: '命运骰盅' })).toHaveAttribute('aria-current', 'page');
    expect(fetcher.mock.calls.filter(([path])=>path.startsWith('/api/v1/games/')).map(([path,init])=>[path,init?.method])).toEqual([['/api/v1/games/dice/bootstrap','GET']]);
    expect(fetcher.mock.calls.some(([path])=>path.startsWith('/platform/v1/wallet')||path.startsWith('/platform/v1/transactions'))).toBe(false);
});
it('hands the old login route to the full-page Native login without a custom form', async () => {
    const { client, fetcher } = fixtureClient(() => new Response(JSON.stringify({ success: false }), { status: 401 }));
    render(<MemoryRouter initialEntries={['/login']}><App client={client} /></MemoryRouter>);
    expect(await screen.findByRole('link', { name: '前往 New API 登录' })).toHaveAttribute('href', '/sign-in');
    expect(screen.queryByLabelText('用户名')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('密码')).not.toBeInTheDocument();
    expect(fetcher.mock.calls.some(call => call[1]?.method === 'POST' && !call[0].includes('/auth/refresh'))).toBe(false);
});
it('prevents duplicate key creation and refreshes the list without revealing keys', async () => {
    let resolve!: (r: Response) => void, listReads = 0;
    const f = vi.fn(async (path: string, init?: RequestInit) => {
        if (path.includes('/auth/refresh')) return ok(bundle);
        if (path === '/api/user/self') return ok(user);
        if (path === '/api/token/' && init?.method === 'POST') return new Promise<Response>(r => resolve = r);
        if (path === '/platform/v1/key-purposes') return init?.method === 'POST'
            ? ok({ token_id: '9', purpose: 'GENERAL', version: '1', effective_at: '2026-09-10T00:00:00Z', sync_state: 'EFFECTIVE' })
            : ok({ items: [] });
        if (path.startsWith('/api/token/?')) listReads++;
        return ok(list);
    });
    render(<MemoryRouter initialEntries={['/keys']}><App client={new ApiClient(withReadyAccessGate(f))}/></MemoryRouter>);
    fireEvent.click(await screen.findByRole('button', { name: '创建 API 密钥' }));
    fireEvent.change(screen.getByLabelText('用途'), { target: { value: 'GENERAL' } });
    fireEvent.change(screen.getByLabelText('密钥名称'), { target: { value: 'synthetic-key' } });
    fireEvent.change(screen.getByLabelText('额度上限（原生单位）'), { target: { value: '100' } });
    const button = screen.getByRole('button', { name: '确认创建' });
    fireEvent.click(button); fireEvent.click(button);
    await waitFor(() => expect(f.mock.calls.filter(c => c[0] === '/api/token/' && c[1]?.method === 'POST')).toHaveLength(1));
    expect(button).toBeDisabled();
    resolve(ok({ id: '9' }));
    await screen.findByText('密钥已创建。在列表中选择“查看密钥”后再复制。');
    await waitFor(() => expect(listReads).toBeGreaterThanOrEqual(2));
    const purposeCalls = f.mock.calls.filter(c => c[0] === '/platform/v1/key-purposes' && c[1]?.method === 'POST');
    expect(purposeCalls).toHaveLength(1);
    expect(JSON.parse(String(purposeCalls[0][1]?.body))).toEqual(expect.objectContaining({ token_id: '9', purpose: 'GENERAL' }));
    expect(f.mock.calls.some(c => c[0].endsWith('/key'))).toBe(false);
});
it('shows list error and a retry action', async () => { const f = vi.fn(async (path: string) => path.includes('/refresh') ? ok(bundle) : path === '/api/user/self' ? ok(user) : new Response(JSON.stringify({ success: false, message: '列表暂不可用' }))); render(<MemoryRouter initialEntries={['/keys']}><App client={new ApiClient(withReadyAccessGate(f))}/></MemoryRouter>); expect(await screen.findByRole('alert')).toHaveTextContent('列表暂不可用'); expect(screen.getByRole('button', { name: '重新加载' })).toBeVisible(); });
it('reveals only by explicit action, prefixes once and clears on navigation', async () => { const key = { id: 9, name: 'synthetic-key', key: 'abc***xyz', status: 1, created_time: 1, expired_time: -1, remain_quota: 100, used_quota: 0, unlimited_quota: false }; const f = vi.fn(async (path: string) => path.includes('/refresh') ? ok(bundle) : path === '/api/user/self' ? ok(user) : path === '/api/token/9/key' ? ok({ key: 'synthetic-plaintext' }) : path.startsWith('/api/log/self') ? ok(list) : ok({ ...list, items: [key], total: 1 })); render(<MemoryRouter initialEntries={['/keys']}><App client={new ApiClient(withReadyAccessGate(f))}/></MemoryRouter>); await screen.findByText('synthetic-key'); expect(document.querySelector('.api-workbench-background')).toHaveAttribute('alt',''); expect(f.mock.calls.some(c => c[0].endsWith('/key'))).toBe(false); fireEvent.click(screen.getByRole('button', { name: '查看密钥' })); expect(await screen.findByLabelText('完整 API 密钥')).toHaveValue('sk-synthetic-plaintext'); fireEvent.click(screen.getByRole('button', { name: '关闭' })); expect(screen.queryByLabelText('完整 API 密钥')).not.toBeInTheDocument(); fireEvent.click(screen.getByRole('button', { name: '查看密钥' })); await screen.findByLabelText('完整 API 密钥'); fireEvent.click(screen.getByRole('link', { name: /调用记录/ })); expect(screen.queryByLabelText('完整 API 密钥')).not.toBeInTheDocument(); });
it('requires explicit delete confirmation and sends exact per-id route once', async () => { const key = { id: 9, name: 'synthetic-key', key: '***', status: 1, created_time: 1, expired_time: -1, remain_quota: 100, used_quota: 0, unlimited_quota: false }; const f = vi.fn(async (path: string, init?: RequestInit) => path.includes('/refresh') ? ok(bundle) : path === '/api/user/self' ? ok(user) : init?.method === 'DELETE' ? ok() : ok({ ...list, items: [key], total: 1 })); render(<MemoryRouter initialEntries={['/keys']}><App client={new ApiClient(withReadyAccessGate(f))}/></MemoryRouter>); fireEvent.click(await screen.findByText('更多', { selector: 'summary' })); fireEvent.click(screen.getByRole('button', { name: '删除' })); expect(f.mock.calls.some(c => c[1]?.method === 'DELETE')).toBe(false); fireEvent.click(screen.getByRole('button', { name: '保留密钥' })); expect(screen.queryByRole('dialog')).not.toBeInTheDocument(); fireEvent.click(screen.getByRole('button', { name: '删除' })); fireEvent.click(screen.getByRole('button', { name: '确认删除' })); await screen.findByText('密钥已删除。'); expect(f.mock.calls.filter(c => c[1]?.method === 'DELETE')).toHaveLength(1); expect(f.mock.calls.find(c => c[1]?.method === 'DELETE')?.[0]).toBe('/api/token/9'); });
it('ambiguous create blocks resubmission until the user checks the refreshed list', async () => {
    let listReads = 0;
    const f = vi.fn(async (path: string, init?: RequestInit) => {
        if (path.includes('/refresh')) return ok(bundle);
        if (path === '/api/user/self') return ok(user);
        if (path === '/api/token/' && init?.method === 'POST') throw new TypeError('offline');
        if (path === '/platform/v1/key-purposes') return ok({ items: [] });
        if (path.startsWith('/api/token/?')) listReads++;
        return ok(list);
    });
    render(<MemoryRouter initialEntries={['/keys']}><App client={new ApiClient(withReadyAccessGate(f))}/></MemoryRouter>);
    fireEvent.click(await screen.findByRole('button', { name: '创建 API 密钥' }));
    fireEvent.change(screen.getByLabelText('用途'), { target: { value: 'ROLEPLAY' } });
    fireEvent.change(screen.getByLabelText('密钥名称'), { target: { value: 'test' } });
    fireEvent.change(screen.getByLabelText('额度上限（原生单位）'), { target: { value: '1' } });
    fireEvent.click(screen.getByRole('button', { name: '确认创建' }));
    await screen.findByText('请关闭此窗口并核对列表后，再决定是否创建。');
    expect(screen.getByRole('button', { name: '确认创建' })).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: '确认创建' }));
    expect(f.mock.calls.filter(c => c[0] === '/api/token/' && c[1]?.method === 'POST')).toHaveLength(1);
    expect(f.mock.calls.some(c => c[0] === '/platform/v1/key-purposes' && c[1]?.method === 'POST')).toBe(false);
    await waitFor(() => expect(listReads).toBeGreaterThanOrEqual(2));
});
it('toggles exact status_only payload and paginates the native list', async () => { const key = { id: 9, name: 'synthetic-key', key: '***', status: 1, created_time: 1, expired_time: -1, remain_quota: 100, used_quota: 0, unlimited_quota: false }; const f = vi.fn(async (path: string) => path.includes('/refresh') ? ok(bundle) : path === '/api/user/self' ? ok(user) : ok({ ...list, items: [key], total: 11 })); render(<MemoryRouter initialEntries={['/keys']}><App client={new ApiClient(withReadyAccessGate(f))}/></MemoryRouter>); fireEvent.click(await screen.findByText('更多', { selector: 'summary' })); fireEvent.click(screen.getByRole('button', { name: '停用' })); await screen.findByText('密钥已停用。'); const call = f.mock.calls.find(c => c[0].includes('status_only')) as unknown as [
    string,
    RequestInit
]; expect(call[0]).toBe('/api/token/?status_only=true'); expect(call[1].method).toBe('PUT'); expect(JSON.parse(String(call[1].body))).toEqual({ id: 9, status: 2 }); fireEvent.click(await screen.findByRole('button', { name: '下一页' })); await waitFor(() => expect(f.mock.calls.some(c => c[0] === '/api/token/?p=2&page_size=10')).toBe(true)); });
it('edits current basic key fields once and refreshes after the verified receipt', async () => {
    const key = { id: 9, name: 'synthetic-key', key: 'ab****yz', status: 1, created_time: 1, expired_time: -1, remain_quota: 100, used_quota: 7, unlimited_quota: false };
    let finish!: (response: Response) => void, listReads = 0;
    const f = vi.fn(async (path: string, init?: RequestInit) => {
        if (path.includes('/auth/refresh')) return ok(bundle);
        if (path === '/api/user/self') return ok(user);
        if (path === '/platform/v1/key-purposes') return ok({ items: [] });
        if (path.startsWith('/api/token/?p=')) { listReads++; return ok({ ...list, items: [key], total: 1 }); }
        if (path === '/api/token/' && init?.method === 'PUT') return new Promise<Response>(resolve => finish = resolve);
        return ok(list);
    });
    render(<MemoryRouter initialEntries={['/keys']}><App client={new ApiClient(withReadyAccessGate(f))}/></MemoryRouter>);
    fireEvent.click(await screen.findByRole('button', { name: '编辑基础设置' }));
    expect(screen.getByLabelText('密钥名称')).toHaveValue('synthetic-key');
    expect(screen.getByLabelText('额度上限（原生单位）')).toHaveValue(100);
    expect(screen.getByLabelText('有效期')).toHaveValue('never');
    fireEvent.change(screen.getByLabelText('密钥名称'), { target: { value: ' edited key ' } });
    fireEvent.change(screen.getByLabelText('额度上限（原生单位）'), { target: { value: '200' } });
    const save = screen.getByRole('button', { name: '保存基础设置' });
    fireEvent.click(save);
    fireEvent.click(save);
    await waitFor(() => expect(f.mock.calls.filter(call => call[0] === '/api/token/' && call[1]?.method === 'PUT')).toHaveLength(1));
    expect(screen.getByRole('button', { name: '正在保存…' })).toBeDisabled();
    const editCall = f.mock.calls.find(call => call[0] === '/api/token/' && call[1]?.method === 'PUT')!;
    expect(JSON.parse(String(editCall[1]?.body))).toEqual({ id: 9, name: 'edited key', remain_quota: 200, unlimited_quota: false, expired_time: -1 });
    finish(ok({ ...key, name: 'edited key', remain_quota: 200 }));
    await screen.findByText('密钥基础设置已更新。');
    await waitFor(() => expect(listReads).toBeGreaterThanOrEqual(2));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
});
it('rejects an edit above the Native quota bound before sending it', async () => {
    const key = { id: 9, name: 'synthetic-key', key: '***', status: 1, created_time: 1, expired_time: -1, remain_quota: 100, used_quota: 0, unlimited_quota: false };
    const f = vi.fn(async (path: string, _init?: RequestInit) => path.includes('/auth/refresh') ? ok(bundle) : path === '/api/user/self' ? ok(user) : path === '/platform/v1/key-purposes' ? ok({ items: [] }) : ok({ ...list, items: [key], total: 1 }));
    render(<MemoryRouter initialEntries={['/keys']}><App client={new ApiClient(withReadyAccessGate(f))}/></MemoryRouter>);
    fireEvent.click(await screen.findByRole('button', { name: '编辑基础设置' }));
    fireEvent.change(screen.getByLabelText('额度上限（原生单位）'), { target: { value: '2147483648' } });
    fireEvent.click(screen.getByRole('button', { name: '保存基础设置' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('1 至 2,147,483,647');
    expect(f.mock.calls.some(call => call[0] === '/api/token/' && call[1]?.method === 'PUT')).toBe(false);
});
it('blocks repeat editing when the basic edit receipt cannot be verified', async () => {
    const key = { id: 9, name: 'synthetic-key', key: '***', status: 1, created_time: 1, expired_time: -1, remain_quota: 100, used_quota: 0, unlimited_quota: false };
    const f = vi.fn(async (path: string, init?: RequestInit) => path.includes('/auth/refresh') ? ok(bundle) : path === '/api/user/self' ? ok(user) : path === '/platform/v1/key-purposes' ? ok({ items: [] }) : path === '/api/token/' && init?.method === 'PUT' ? ok({ ...key, id: 10 }) : ok({ ...list, items: [key], total: 1 }));
    render(<MemoryRouter initialEntries={['/keys']}><App client={new ApiClient(withReadyAccessGate(f))}/></MemoryRouter>);
    fireEvent.click(await screen.findByRole('button', { name: '编辑基础设置' }));
    fireEvent.click(screen.getByRole('button', { name: '保存基础设置' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('密钥编辑结果尚未确认');
    expect(screen.getByRole('button', { name: '保存基础设置' })).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: '保存基础设置' }));
    expect(f.mock.calls.filter(call => call[0] === '/api/token/' && call[1]?.method === 'PUT')).toHaveLength(1);
});
it('resets the document title to login after logout', async () => {
    const fetcher = vi.fn(async (path: string) => path.includes('/refresh') ? ok(bundle) : path === '/api/user/self' ? ok(user) : path.includes('/logout') ? ok() : ok(list));
    render(<MemoryRouter initialEntries={['/keys']}><App client={new ApiClient(withReadyAccessGate(fetcher))} /></MemoryRouter>);
    await screen.findByRole('button', { name: '创建 API 密钥' });
    expect(document.title).toBe('密钥管理 · momiao');
    fireEvent.click(screen.getByRole('button', { name: '账户菜单' }));
    fireEvent.click(screen.getByRole('button', { name: '退出登录' }));
    await screen.findByRole('link', { name: '前往 New API 登录' });
    expect(document.title).toBe('登录 · momiao');
});
it('redirects a mismatched old session to login without logging out the shared cookie', async () => {
    const fetcher = vi.fn().mockResolvedValueOnce(ok({ ...bundle, access_expires_at: 1 }))
        .mockResolvedValueOnce(new Response(JSON.stringify({ success: false, code: 'AUTH_SESSION_MISMATCH', message: 'Conflict' }), { status: 409 }));
    const client = new ApiClient(withReadyAccessGate(fetcher));
    await client.login('test-user', 'synthetic-password');
    render(<MemoryRouter initialEntries={['/keys']}><App client={client} /></MemoryRouter>);
    await screen.findByRole('link', { name: '前往 New API 登录' });
    expect(screen.queryByLabelText('密码')).not.toBeInTheDocument();
    expect(document.title).toBe('登录 · momiao');
    expect(screen.queryByRole('button', { name: '账户菜单' })).not.toBeInTheDocument();
    expect(fetcher.mock.calls.filter(call => !String(call[0]).startsWith('/platform/v1/announcements/') && call[0] !== '/platform/v1/admission/config')).toHaveLength(2);
    expect(fetcher.mock.calls.some(call => call[0].includes('/auth/logout'))).toBe(false);
});

it('keeps native and RP usage separate with existing filter semantics and no model calls', async () => {
    const { client, fetcher } = fixtureClient(path => {
        if (path.startsWith('/api/log/self')) return ok({ ...list, total: 1, items: [{ id: 1, type: 2, created_at: 1727000000, model_name: 'account-model', token_name: 'account-key', prompt_tokens: 321, completion_tokens: 123, quota: 444 }] });
        if (path.startsWith('/api/v1/usage/rp')) return ok({ items: [{ logical_request_id: 'fixture-request', token_id: '9', model_id: 'rp-model', model_name: 'RP model', request_kind: 'CHAT', provider_attempt_count: 2, final_status: 'SUCCESS', error_category: '', charged_raw_quota: '50000', charged_amount: '0.1', requested_at: '2026-09-06T00:00:00Z', completed_at: '2026-09-06T00:00:01Z' }], total: '1', page: 1, page_size: 50, has_more: false, observed_at: '2026-09-06T00:01:00Z' });
    });
    render(<MemoryRouter initialEntries={['/logs']}><App client={client}/></MemoryRouter>);
    await screen.findByText('account-model');
    expect(screen.getByRole('link', { name: '原生调用' })).toHaveAttribute('aria-current','page');
    expect(screen.getByRole('columnheader', { name: '额度（原生单位）' })).toBeVisible();
    fireEvent.change(screen.getByLabelText('模型名称'), { target: { value: 'account-model' } });
    fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-09-06' } });
    fireEvent.change(screen.getByLabelText('结束日期'), { target: { value: '2026-09-06' } });
    fireEvent.click(screen.getByRole('button', { name: '应用筛选' }));
    await waitFor(() => expect(fetcher.mock.calls.some(([path]) => path.startsWith('/api/log/self?') && new URLSearchParams(path.split('?')[1]).get('start_timestamp') === String(new Date('2026-09-06T00:00:00').getTime()/1000))).toBe(true));
    fireEvent.click(screen.getByRole('link', { name: 'RP 调用' }));
    await screen.findByText('RP model');
    expect(screen.getByRole('link', { name: 'RP 调用' })).toHaveAttribute('aria-current','page');
    expect(screen.getByRole('columnheader', { name: 'API Credit 消耗' })).toBeVisible();
    expect(screen.queryByRole('columnheader', { name: '额度（原生单位）' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: '下一页' })).toBeDisabled();
    fireEvent.change(screen.getByLabelText('模型 ID'), { target: { value: 'rp-model' } });
    fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-09-06' } });
    fireEvent.change(screen.getByLabelText('结束日期'), { target: { value: '2026-09-06' } });
    fireEvent.click(screen.getByRole('button', { name: '应用筛选' }));
    await waitFor(() => expect(fetcher.mock.calls.some(([path]) => { const q=new URLSearchParams(path.split('?')[1]); return path.startsWith('/api/v1/usage/rp?') && q.get('model')==='rp-model' && q.get('from')==='2026-09-05T16:00:00.000Z' && q.get('to')==='2026-09-06T16:00:00.000Z'; })).toBe(true));
    expect(fetcher.mock.calls.some(([path]) => path.startsWith('/v1/') || path.startsWith('/pg/'))).toBe(false);
});
