import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, it } from 'vitest';
import { App } from './App';
import { bundle, failed, fixtureClient, ok, profile, user } from './m1-test-fixtures';

it('shows verified Master identity separately from the native account and links the existing journey', async () => {
    const { client, fetcher } = fixtureClient();
    render(<MemoryRouter initialEntries={['/me']}><App client={client} /></MemoryRouter>);
    const hub = await screen.findByRole('region', { name: 'Master 身份' });
    expect(await within(hub).findByText(profile.display_name)).toBeVisible();
    expect(screen.getByText('Native Fixture · fixture-account')).toBeVisible();
    const main = screen.getByRole('main');
    for (const path of ['/master-profile', '/rewards', '/wallet', '/wallet/activate', '/keys', '/logs', '/playground']) {
        expect(main.querySelector('a[href="' + path + '"]')).not.toBeNull();
    }
    expect(main.querySelector('a[href="/admin/channels"]')).toBeNull();
    expect(fetcher.mock.calls.some(c => c[0].includes('initialize'))).toBe(false);
});
it('does not use a suggested or native nickname as an initialized Master identity', async () => {
    const { client, fetcher } = fixtureClient(p => p === '/platform/v1/master-profile' ? ok({ ...profile, status: 'INCOMPLETE', display_name: '', profile_version: '0' }) : undefined);
    render(<MemoryRouter initialEntries={['/me']}><App client={client} /></MemoryRouter>);
    expect(await screen.findByRole('link', { name: '建立 Master 资料' })).toHaveAttribute('href', '/master-profile');
    expect(screen.queryByText(profile.suggested_name)).not.toBeInTheDocument();
    expect(fetcher.mock.calls.every(c => c[1]?.method !== 'PATCH' && !c[0].includes('initialize'))).toBe(true);
});
it('a failed Master read offers a real retry and keeps other personal paths available', async () => {
    let fail = true;
    const { client } = fixtureClient(p => p === '/platform/v1/master-profile' && fail ? failed() : undefined);
    render(<MemoryRouter initialEntries={['/me']}><App client={client} /></MemoryRouter>);
    expect(await screen.findByRole('alert')).toHaveTextContent('Master 资料读取未完成');
    expect(screen.queryByRole('link', { name: '建立 Master 资料' })).not.toBeInTheDocument();
    fail = false;
    fireEvent.click(screen.getByRole('button', { name: '重新读取 Master 资料' }));
    expect(await within(screen.getByRole('region', { name: 'Master 身份' })).findByText(profile.display_name)).toBeVisible();
});
it('refreshes Master identity after an explicitly saved profile and when returning to the dashboard', async () => {
    let saved = profile;
    const { client, fetcher } = fixtureClient((p, init) => {
        if (p !== '/platform/v1/master-profile') return;
        if (init?.method === 'PATCH') saved = { ...profile, display_name: '晨星观测员', profile_version: '2' };
        return ok(saved);
    });
    render(<MemoryRouter initialEntries={['/master-profile']}><App client={client} /></MemoryRouter>);
    await waitFor(() => expect(screen.getByLabelText('Master 昵称')).toHaveValue(profile.display_name));
    fireEvent.change(screen.getByLabelText('Master 昵称'), { target: { value: '晨星观测员' } });
    fireEvent.click(screen.getByRole('button', { name: '保存修改' }));
    await screen.findByText('资料已保存，并已核对最新状态。');
    await waitFor(() => expect(screen.getByRole('button', { name: /账户菜单/ })).toHaveTextContent('晨星观测员'));
    fireEvent.click(within(screen.getByRole('navigation', { name: '主导航' })).getByRole('link', { name: '指挥台' }));
    await waitFor(() => expect(screen.getByRole('main')).toHaveTextContent('晨星观测员'));
    expect(fetcher.mock.calls.filter(c => c[1]?.method === 'PATCH')).toHaveLength(1);
});
it('discards a late identity from the previous account after logout and a new login', async () => {
    let resolve!: (value: Response) => void;
    let account = 1;
    const { client } = fixtureClient(p => {
        if (p === '/platform/v1/master-profile') return account === 1 ? new Promise<Response>(r => { resolve = r; }) : ok({ ...profile, user_id: '2', display_name: '另一个观测员' });
        if (p === '/api/user/login' && account === 2) return ok({ ...bundle, user: { ...user, id: 2 }, session: { sid: 'second-session' } });
        if (p === '/api/user/self' && account === 2) return ok({ ...user, id: 2 });
    });
    render(<MemoryRouter initialEntries={['/me']}><App client={client} /></MemoryRouter>);
    await waitFor(() => expect(resolve).toBeTypeOf('function'));
    await act(async () => { await client.logout(); account = 2; await client.login('fixture-second', 'fixture'); resolve(ok(profile)); });
    fireEvent.click(within(screen.getByRole('navigation', { name: '主导航' })).getByRole('link', { name: '个人中心' }));
    expect(await within(screen.getByRole('region', { name: 'Master 身份' })).findByText('另一个观测员')).toBeVisible();
    expect(screen.queryByText(profile.display_name)).not.toBeInTheDocument();
});
