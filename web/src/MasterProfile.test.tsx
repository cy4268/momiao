import { withReadyAccessGate } from './m1-test-fixtures';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, it, vi } from 'vitest';
import { App } from './App';
import { ApiClient } from './api';
const user = { id: 1, username: 'native-user', display_name: 'Native User', role: 1 };
const incomplete = { user_id: '1', short_account_id: 'CA-123456789ABC', status: 'INCOMPLETE', display_name: '', avatar_id: 'system-default', profile_version: '0', nickname_changed_at: null, next_rename_at: null, suggested_name: 'Master-CA-123456789ABC', avatars: [{ id: 'system-default', label: '系统默认头像', source: 'SYSTEM' }] };
const complete = { ...incomplete, status: 'COMPLETE', display_name: 'Moonlit', profile_version: '1' };
const ok = (data: unknown) => new Response(JSON.stringify({ success: true, data }));
const bundle = (sid = 'sid', id = 1) => ({ access_token: 'memory', access_expires_at: 9999999999, session: { sid }, user: { ...user, id } });
const fail = (code: string, status = 409) => new Response(JSON.stringify({ success: false, code, message: 'upstream English detail' }), { status });
function setup(respond?: (path: string, init?: RequestInit) => Response | Promise<Response> | undefined) {
    const f = vi.fn(async (p: string, init?: RequestInit) => {
        const custom = respond?.(p, init); if (custom) return custom;
        if (p.includes('/refresh')) return ok(bundle());
        if (p === '/api/user/self') return ok(user);
        if (p === '/platform/v1/master-profile') return ok(incomplete);
        return ok({ items: [], total: 0, page: 1, page_size: 10 });
    });
    const client = new ApiClient(withReadyAccessGate(f));
    return { f, client, ...render(<MemoryRouter initialEntries={['/master-profile']}><App client={client} /></MemoryRouter>) };
}
const writes = (f: ReturnType<typeof setup>['f']) => f.mock.calls.filter(([p, init]) => p.startsWith('/platform/v1/master-profile') && init?.method !== 'GET');
const edit = (name: string) => fireEvent.change(screen.getByLabelText('Master 昵称'), { target: { value: name } });

it('opens an optional profile route without initializing or copying native identity', async () => {
    const { f } = setup();
    await screen.findByRole('heading', { name: 'Master 资料', level: 1 });
    expect(await screen.findByLabelText('Master 昵称')).toHaveValue('');
    expect(screen.getByRole('button', { name: '保存并初始化' })).toBeDisabled();
    expect(screen.getByText('尚未初始化')).toBeVisible(); expect(writes(f)).toHaveLength(0);
    expect(screen.getByRole('button', { name: '账户菜单' })).toHaveTextContent('Master 资料未完成');
    fireEvent.click(screen.getByRole('button', { name: '账户菜单' }));
    expect(within(document.getElementById('account-menu')!).getByText('Native User')).toBeVisible();
    expect(within(document.getElementById('account-menu')!).getByText('原生登录身份 · native-user')).toBeVisible();
    expect(within(document.getElementById('account-menu')!).getByRole('link', { name: 'Master 资料' })).toHaveAttribute('href', '/master-profile');
});
it('previews without writing, explicitly initializes once and round-trips the identity', async () => {
    let saved = false;
    const { f, client } = setup(p => {
        if (p.endsWith('/initialize')) { saved = true; return ok(complete); }
        if (p === '/platform/v1/master-profile') return ok(saved ? complete : incomplete);
    });
    await screen.findByLabelText('Master 昵称'); edit('Moonlit'); fireEvent.click(screen.getByRole('button', { name: '预览资料' }));
    expect(within(screen.getByRole('region', { name: '公开身份预览' })).getByText('Moonlit')).toBeVisible();
    expect(screen.getByText('未保存的预览')).toBeVisible(); expect(writes(f)).toHaveLength(0);
    const save = screen.getByRole('button', { name: '保存并初始化' }); fireEvent.click(save); fireEvent.click(save);
    await screen.findByText('资料已保存，并已核对最新状态。'); expect(writes(f)).toHaveLength(1);
    expect(writes(f)[0][0]).toBe('/platform/v1/master-profile/initialize'); expect(writes(f)[0][1]?.method).toBe('POST');
    expect(JSON.parse(String(writes(f)[0][1]?.body))).toEqual({ expected_version: '0', display_name: 'Moonlit', avatar_id: 'system-default' });
    const headers = new Headers(writes(f)[0][1]?.headers);
    expect(headers.get('New-Api-User')).toBe('1'); expect(headers.get('Authorization')).toBe('Bearer memory');
    // Editor and shell each confirm their view before and after the explicit save.
    expect(f.mock.calls.filter(([p, init]) => p === '/platform/v1/master-profile' && init?.method === 'GET')).toHaveLength(4);
    expect(client.getSnapshot().user).toMatchObject({ username: 'native-user', display_name: 'Native User' });
});
it('sends PATCH with exact string version and renders server rename timestamps', async () => {
    let saved = false;
    const renamed = { ...complete, display_name: 'Moon II', profile_version: '9007199254740994', nickname_changed_at: '2026-09-05T01:00:00Z', next_rename_at: '2026-09-12T01:00:00Z' };
    const { f } = setup((p, init) => {
        if (p !== '/platform/v1/master-profile') return;
        if (init?.method === 'PATCH') { saved = true; return ok(renamed); }
        return ok(saved ? renamed : { ...complete, profile_version: '9007199254740993' });
    });
    await screen.findByDisplayValue('Moonlit'); edit('Moon II'); fireEvent.click(screen.getByRole('button', { name: '保存修改' }));
    await screen.findByText('资料已保存，并已核对最新状态。');
    expect(JSON.parse(String(writes(f)[0][1]?.body))).toEqual({ expected_version: '9007199254740993', display_name: 'Moon II' });
    expect(screen.getByText('9007199254740994')).toBeVisible();
    expect(document.querySelector('time[datetime="2026-09-05T01:00:00Z"]')).toBeInTheDocument();
    expect(document.querySelector('time[datetime="2026-09-12T01:00:00Z"]')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '保存修改' })).toBeDisabled();
});
it.each([['NICKNAME_TAKEN', 409, /昵称已被使用/], ['NICKNAME_RESERVED', 403, /昵称属于保留名称/], ['INVALID_NICKNAME', 400, /昵称格式/], ['RENAME_COOLDOWN', 409, /改名冷却/], ['INVALID_AVATAR', 400, /头像选项/]] as const)('translates %s while retaining the draft and login', async (code, status, message) => {
    const { client, f } = setup((p, init) => p === '/platform/v1/master-profile' ? init?.method === 'PATCH' ? fail(code, status) : ok(complete) : undefined);
    await screen.findByDisplayValue('Moonlit'); edit('Another'); fireEvent.click(screen.getByRole('button', { name: '保存修改' }));
    await screen.findByText(message); expect(screen.getByLabelText('Master 昵称')).toHaveValue('Another');
    expect(client.getSnapshot().user?.id).toBe(1); expect(writes(f)).toHaveLength(1); expect(screen.queryByText(/upstream English/)).not.toBeInTheDocument();
});
it('refreshes stale versions without automatically replaying the draft', async () => {
    let stale = false;
    const { f } = setup((p, init) => {
        if (p !== '/platform/v1/master-profile') return;
        if (init?.method === 'PATCH') { stale = true; return fail('STALE_RESOURCE_VERSION'); }
        return ok(stale ? { ...complete, display_name: 'Other tab', profile_version: '2' } : complete);
    });
    await screen.findByDisplayValue('Moonlit'); edit('New draft'); fireEvent.click(screen.getByRole('button', { name: '保存修改' }));
    await screen.findByText(/资料版本已更新/); await screen.findByDisplayValue('Other tab');
    expect(screen.getByRole('button', { name: '保存修改' })).toBeDisabled(); expect(writes(f)).toHaveLength(1);
});
it('locks ambiguous writes across failed reads until successful explicit GET and never retries a write', async () => {
    let readsFail = false;
    const { f } = setup(p => {
        if (p.endsWith('/initialize')) return Promise.reject(new Error('lost'));
        if (p === '/platform/v1/master-profile' && readsFail) return fail('PROFILE_UNAVAILABLE', 503);
    });
    await screen.findByLabelText('Master 昵称'); edit('Draft'); fireEvent.click(screen.getByRole('button', { name: '保存并初始化' }));
    await screen.findByText(/保存结果尚未确认/); expect(screen.getByRole('button', { name: '保存并初始化' })).toBeDisabled();
    expect(f.mock.calls.filter(([p]) => p === '/platform/v1/master-profile')).toHaveLength(2);
    readsFail = true; fireEvent.click(screen.getByRole('button', { name: '刷新资料' })); await screen.findByText(/资料服务暂不可用/);
    expect(screen.getByRole('button', { name: '保存并初始化' })).toBeDisabled();
    readsFail = false; fireEvent.click(screen.getByRole('button', { name: '刷新资料' }));
    await waitFor(() => expect(screen.getByRole('button', { name: '刷新资料' })).toBeEnabled()); edit('Reviewed');
    expect(screen.getByRole('button', { name: '保存并初始化' })).toBeEnabled(); expect(writes(f)).toHaveLength(1);
});
it('locks malformed successful writes rather than displaying another user identity', async () => {
    setup(p => p.endsWith('/initialize') ? ok({ ...complete, user_id: '2' }) : undefined);
    await screen.findByLabelText('Master 昵称'); edit('Draft'); fireEvent.click(screen.getByRole('button', { name: '保存并初始化' }));
    await screen.findByText(/保存结果尚未确认/); expect(screen.getByRole('button', { name: '保存并初始化' })).toBeDisabled();
    expect(within(screen.getByRole('region', { name: '公开身份预览' })).queryByText('Moonlit')).not.toBeInTheDocument();
});
it('rejects foreign or malformed GET, recovers by read, and uses only the static Crest', async () => {
    let broken = true;
    setup(p => p === '/platform/v1/master-profile' ? ok(broken ? { ...complete, user_id: '2', avatar_id: 'https://external.invalid/a.png' } : complete) : undefined);
    await screen.findByText(/资料响应格式异常/); expect(screen.queryByLabelText('Master 昵称')).not.toBeInTheDocument();
    broken = false; fireEvent.click(screen.getByRole('button', { name: '刷新资料' })); await screen.findByDisplayValue('Moonlit');
    expect(screen.getAllByRole('img', { name: '系统默认头像' }).length).toBeGreaterThan(0); expect(document.querySelector('img[src]')).not.toBeInTheDocument();
});
it('changing accounts drops the old GET and clears drafts', async () => {
    let resolve!: (r: Response) => void; let account = '1';
    const { f, client } = setup(p => {
        if (p === '/api/user/login') { account = '2'; return ok(bundle('sid2', 2)); }
        if (p === '/platform/v1/master-profile') return account === '1' ? new Promise<Response>(r => resolve = r) : ok({ ...incomplete, user_id: '2' });
    });
    await waitFor(() => expect(f.mock.calls.some(([p]) => p === '/platform/v1/master-profile')).toBe(true));
    await act(() => client.login('second', 'password')); await screen.findByLabelText('Master 昵称'); await act(async () => resolve(ok(complete)));
    expect(screen.getByLabelText('Master 昵称')).toHaveValue(''); expect(screen.queryByText('Moonlit')).not.toBeInTheDocument();
});
it('a new session for the same user drops prior writes and drafts', async () => {
    let resolve!: (r: Response) => void;
    const { client, f } = setup(p => p.endsWith('/initialize') ? new Promise<Response>(r => resolve = r) : p === '/api/user/login' ? ok(bundle('new-same-user-session')) : undefined);
    await screen.findByLabelText('Master 昵称'); edit('Old draft'); fireEvent.click(screen.getByRole('button', { name: '保存并初始化' }));
    await act(() => client.login('native-user', 'password')); await waitFor(() => expect(screen.getByLabelText('Master 昵称')).toHaveValue(''));
    await act(async () => resolve(ok(complete)));
    expect(screen.queryByText('资料已保存，并已核对最新状态。')).not.toBeInTheDocument(); expect(writes(f)).toHaveLength(1);
});
it('normal token refresh preserves an unsaved draft in the same session', async () => {
    const { client, f } = setup(); await screen.findByLabelText('Master 昵称'); edit('Unsaved');
    await act(() => client.refresh()); expect(screen.getByLabelText('Master 昵称')).toHaveValue('Unsaved');
    expect(f.mock.calls.filter(([p]) => p === '/platform/v1/master-profile')).toHaveLength(2);
});
it('leaving the page drops late writes without restoring the profile', async () => {
    let resolve!: (r: Response) => void;
    setup(p => p.endsWith('/initialize') ? new Promise<Response>(r => resolve = r) : undefined);
    await screen.findByLabelText('Master 昵称'); edit('Draft'); fireEvent.click(screen.getByRole('button', { name: '保存并初始化' }));
    fireEvent.click(within(screen.getByRole('navigation', { name: '主导航' })).getByRole('link', { name: /指挥台/ })); await screen.findByRole('heading', { name: '指挥台', level: 1 });
    await act(async () => resolve(ok(complete))); expect(screen.queryByText('Moonlit')).not.toBeInTheDocument();
});
it('a write followed by an older GET stays locked instead of falsely claiming persistence was checked', async () => {
    setup(p => p.endsWith('/initialize') ? ok(complete) : undefined);
    await screen.findByLabelText('Master 昵称'); edit('Draft'); fireEvent.click(screen.getByRole('button', { name: '保存并初始化' }));
    await screen.findByText(/读取的资料版本落后于保存结果/);
    expect(screen.queryByText('资料已保存，并已核对最新状态。')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: '保存并初始化' })).toBeDisabled();
});
it('separates future public name/avatar usage from private account identifiers and metadata', async () => {
    setup(); await screen.findByLabelText('Master 昵称');
    const preview = screen.getByRole('region', { name: '公开身份预览' });
    expect(within(preview).queryByText('CA-123456789ABC')).not.toBeInTheDocument();
    expect(within(screen.getByRole('region', { name: '仅本人可见的账户信息' })).getByText('CA-123456789ABC')).toBeVisible();
    expect(screen.getByText(/账户短 ID、版本与改名时间仅本人可见/)).toBeVisible();
    expect(screen.getByText(/昵称与头像会用于后续开放的游戏、排行等公开区域/)).toHaveTextContent('本轮仅保存资料和本人预览，不创建他人可访问的公开主页。');
});
