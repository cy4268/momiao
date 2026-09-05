import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { it, expect, vi } from 'vitest';
import { ApiClient } from './api';
import { App } from './App';
const user = { id: 1, username: 'test-user', display_name: 'Test User', role: 1, quota: 100, used_quota: 0, request_count: 0 };
const ok = (data?: unknown) => new Response(JSON.stringify({ success: true, data }));
const bundle = { access_token: 'synthetic-token', access_expires_at: 9999999999, user, session: { sid: 'test-sid' } };
const list = { items: [], total: 0, page: 1, page_size: 10 };
it('opens the authenticated dice experience without wallet or game requests', async () => {
    const fetcher = vi.fn(async (path: string) => path.includes('/refresh') ? ok(bundle) : path === '/api/user/self' ? ok(user) : ok(list));
    render(<MemoryRouter initialEntries={['/games/dice']}><App client={new ApiClient(fetcher)} /></MemoryRouter>);
    expect(await screen.findByRole('button', { name: '模拟掷骰' })).toBeVisible();
    expect(document.title).toBe('骰子体验 · momiao');
    expect(screen.getByRole('link', { name: /骰子体验/ })).toHaveAttribute('aria-current', 'page');
    expect(fetcher.mock.calls.every(([path]) => path.includes('/refresh') || path === '/api/user/self' || path === '/platform/v1/master-profile')).toBe(true);
});
it('renders login failure and keeps login form', async () => { const f = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({ success: false }), { status: 401 })).mockResolvedValueOnce(new Response(JSON.stringify({ success: false, message: '凭据不匹配' }))); render(<MemoryRouter initialEntries={['/login']}><App client={new ApiClient(f)}/></MemoryRouter>); await screen.findByLabelText('用户名'); fireEvent.change(screen.getByLabelText('用户名'), { target: { value: 'test' } }); fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'pass' } }); fireEvent.click(screen.getByRole('button', { name: '登录控制台' })); expect(await screen.findByRole('alert')).toHaveTextContent('凭据不匹配'); });
it('prevents duplicate key creation and refreshes the list without revealing keys', async () => { let resolve!: (r: Response) => void; const f = vi.fn(async (path: string, init?: RequestInit) => { if (path.includes('/auth/refresh'))
    return ok(bundle); if (path === '/api/user/self')
    return ok(user); if (init?.method === 'POST' && path === '/api/token/')
    return new Promise<Response>(r => resolve = r); return ok(list); }); render(<MemoryRouter initialEntries={['/keys']}><App client={new ApiClient(f)}/></MemoryRouter>); fireEvent.click(await screen.findByRole('button', { name: '创建 API 密钥' })); fireEvent.change(screen.getByLabelText('密钥名称'), { target: { value: 'synthetic-key' } }); fireEvent.change(screen.getByLabelText('额度上限（原生单位）'), { target: { value: '100' } }); const button = screen.getByRole('button', { name: '确认创建' }); fireEvent.click(button); fireEvent.click(button); await waitFor(() => expect(f.mock.calls.filter(c => c[0] === '/api/token/' && c[1]?.method === 'POST')).toHaveLength(1)); expect(button).toBeDisabled(); resolve(ok()); await screen.findByText('密钥已创建。在列表中选择“查看密钥”后再复制。'); expect(f.mock.calls.some(c => c[0].endsWith('/key'))).toBe(false); });
it('shows list error and a retry action', async () => { const f = vi.fn(async (path: string) => path.includes('/refresh') ? ok(bundle) : path === '/api/user/self' ? ok(user) : new Response(JSON.stringify({ success: false, message: '列表暂不可用' }))); render(<MemoryRouter initialEntries={['/keys']}><App client={new ApiClient(f)}/></MemoryRouter>); expect(await screen.findByRole('alert')).toHaveTextContent('列表暂不可用'); expect(screen.getByRole('button', { name: '重新加载' })).toBeVisible(); });
it('reveals only by explicit action, prefixes once and clears on navigation', async () => { const key = { id: 9, name: 'synthetic-key', key: 'abc***xyz', status: 1, created_time: 1, expired_time: -1, remain_quota: 100, used_quota: 0, unlimited_quota: false }; const f = vi.fn(async (path: string) => path.includes('/refresh') ? ok(bundle) : path === '/api/user/self' ? ok(user) : path === '/api/token/9/key' ? ok({ key: 'synthetic-plaintext' }) : path.startsWith('/api/log/self') ? ok(list) : ok({ ...list, items: [key], total: 1 })); render(<MemoryRouter initialEntries={['/keys']}><App client={new ApiClient(f)}/></MemoryRouter>); await screen.findByText('synthetic-key'); expect(f.mock.calls.some(c => c[0].endsWith('/key'))).toBe(false); fireEvent.click(screen.getByRole('button', { name: '查看密钥' })); expect(await screen.findByLabelText('完整 API 密钥')).toHaveValue('sk-synthetic-plaintext'); fireEvent.click(screen.getByRole('button', { name: '关闭' })); expect(screen.queryByLabelText('完整 API 密钥')).not.toBeInTheDocument(); fireEvent.click(screen.getByRole('button', { name: '查看密钥' })); await screen.findByLabelText('完整 API 密钥'); fireEvent.click(screen.getByRole('link', { name: /调用记录/ })); expect(screen.queryByLabelText('完整 API 密钥')).not.toBeInTheDocument(); });
it('requires explicit delete confirmation and sends exact per-id route once', async () => { const key = { id: 9, name: 'synthetic-key', key: '***', status: 1, created_time: 1, expired_time: -1, remain_quota: 100, used_quota: 0, unlimited_quota: false }; const f = vi.fn(async (path: string, init?: RequestInit) => path.includes('/refresh') ? ok(bundle) : path === '/api/user/self' ? ok(user) : init?.method === 'DELETE' ? ok() : ok({ ...list, items: [key], total: 1 })); render(<MemoryRouter initialEntries={['/keys']}><App client={new ApiClient(f)}/></MemoryRouter>); fireEvent.click(await screen.findByRole('button', { name: '删除' })); expect(f.mock.calls.some(c => c[1]?.method === 'DELETE')).toBe(false); fireEvent.click(screen.getByRole('button', { name: '保留密钥' })); expect(screen.queryByRole('dialog')).not.toBeInTheDocument(); fireEvent.click(screen.getByRole('button', { name: '删除' })); fireEvent.click(screen.getByRole('button', { name: '确认删除' })); await screen.findByText('密钥已删除。'); expect(f.mock.calls.filter(c => c[1]?.method === 'DELETE')).toHaveLength(1); expect(f.mock.calls.find(c => c[1]?.method === 'DELETE')?.[0]).toBe('/api/token/9'); });
it('ambiguous create blocks resubmission until the user checks the refreshed list', async () => { const f = vi.fn(async (path: string, init?: RequestInit) => { if (path.includes('/refresh'))
    return ok(bundle); if (path === '/api/user/self')
    return ok(user); if (init?.method === 'POST')
    throw new TypeError('offline'); return ok(list); }); render(<MemoryRouter initialEntries={['/keys']}><App client={new ApiClient(f)}/></MemoryRouter>); fireEvent.click(await screen.findByRole('button', { name: '创建 API 密钥' })); fireEvent.change(screen.getByLabelText('密钥名称'), { target: { value: 'test' } }); fireEvent.change(screen.getByLabelText('额度上限（原生单位）'), { target: { value: '1' } }); fireEvent.click(screen.getByRole('button', { name: '确认创建' })); await screen.findByText('请关闭此窗口并核对列表后，再决定是否创建。'); expect(screen.getByRole('button', { name: '确认创建' })).toBeDisabled(); expect(f.mock.calls.filter(c => c[0] === '/api/token/' && c[1]?.method === 'POST')).toHaveLength(1); });
it('toggles exact status_only payload and paginates the native list', async () => { const key = { id: 9, name: 'synthetic-key', key: '***', status: 1, created_time: 1, expired_time: -1, remain_quota: 100, used_quota: 0, unlimited_quota: false }; const f = vi.fn(async (path: string) => path.includes('/refresh') ? ok(bundle) : path === '/api/user/self' ? ok(user) : ok({ ...list, items: [key], total: 11 })); render(<MemoryRouter initialEntries={['/keys']}><App client={new ApiClient(f)}/></MemoryRouter>); fireEvent.click(await screen.findByRole('button', { name: '停用' })); await screen.findByText('密钥已停用。'); const call = f.mock.calls.find(c => c[0].includes('status_only')) as unknown as [
    string,
    RequestInit
]; expect(call[0]).toBe('/api/token/?status_only=true'); expect(call[1].method).toBe('PUT'); expect(JSON.parse(String(call[1].body))).toEqual({ id: 9, status: 2 }); fireEvent.click(await screen.findByRole('button', { name: '下一页' })); await waitFor(() => expect(f.mock.calls.some(c => c[0] === '/api/token/?p=2&page_size=10')).toBe(true)); });
it('resets the document title to login after logout', async () => {
    const fetcher = vi.fn(async (path: string) => path.includes('/refresh') ? ok(bundle) : path === '/api/user/self' ? ok(user) : path.includes('/logout') ? ok() : ok(list));
    render(<MemoryRouter initialEntries={['/keys']}><App client={new ApiClient(fetcher)} /></MemoryRouter>);
    await screen.findByRole('button', { name: '创建 API 密钥' });
    expect(document.title).toBe('密钥管理 · momiao');
    fireEvent.click(screen.getByRole('button', { name: '账户菜单' }));
    fireEvent.click(screen.getByRole('button', { name: '退出登录' }));
    await screen.findByLabelText('用户名');
    expect(document.title).toBe('登录 · momiao');
});
it('redirects a mismatched old session to login without logging out the shared cookie', async () => {
    const fetcher = vi.fn().mockResolvedValueOnce(ok({ ...bundle, access_expires_at: 1 }))
        .mockResolvedValueOnce(new Response(JSON.stringify({ success: false, code: 'AUTH_SESSION_MISMATCH', message: 'Conflict' }), { status: 409 }));
    const client = new ApiClient(fetcher);
    await client.login('test-user', 'synthetic-password');
    render(<MemoryRouter initialEntries={['/keys']}><App client={client} /></MemoryRouter>);
    await screen.findByLabelText('用户名');
    expect(screen.getByRole('alert')).toHaveTextContent('浏览器会话已变更，本页登录已失效，请重新登录。');
    expect(document.title).toBe('登录 · momiao');
    expect(screen.queryByRole('button', { name: '账户菜单' })).not.toBeInTheDocument();
    expect(fetcher).toHaveBeenCalledTimes(2);
    expect(fetcher.mock.calls.some(call => call[0].includes('/auth/logout'))).toBe(false);
});
