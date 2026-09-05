import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, it } from 'vitest';
import { App } from './App';
import { failed, fixtureClient } from './m1-test-fixtures';

it('renders the public home immediately while session bootstrap is unresolved', () => {
    const { client } = fixtureClient(() => new Promise<Response>(() => {}));
    render(<MemoryRouter initialEntries={['/']}><App client={client} /></MemoryRouter>);
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('在月光下');
    expect(screen.getByRole('link', { name: '登录账户' })).toHaveAttribute('href', '/login');
});
it.each(['guest', 'failure'])('keeps / public after %s session verification', async state => {
    const { client, fetcher } = fixtureClient(() => state === 'guest' ? new Response(JSON.stringify({ success: false }), { status: 401 }) : failed());
    render(<MemoryRouter initialEntries={['/']}><App client={client} /></MemoryRouter>);
    await waitFor(() => expect(client.getSnapshot().ready).toBe(true));
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('在月光下');
    expect(fetcher.mock.calls.map(c => c[0])).toEqual(['/api/user/auth/refresh']);
    expect(screen.queryByLabelText('密码')).not.toBeInTheDocument();
});
it('enhances the same home for a signed-in user without loading personal business data', async () => {
    const { client, fetcher } = fixtureClient();
    render(<MemoryRouter initialEntries={['/']}><App client={client} /></MemoryRouter>);
    expect(await screen.findByRole('link', { name: '进入个人中心' })).toHaveAttribute('href', '/me');
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('在月光下');
    expect(fetcher.mock.calls.every(c => c[0].startsWith('/api/user/'))).toBe(true);
    expect(screen.queryByText(/今日已领取|今日待领取|在线人数|热门榜|服务正常/)).not.toBeInTheDocument();
    expect(screen.getByText(/无资产骰子体验/)).toBeVisible();
});
it('keeps the hero content usable if the decorative image fails', async () => {
    const { client } = fixtureClient(() => failed());
    const view = render(<MemoryRouter initialEntries={['/']}><App client={client} /></MemoryRouter>);
    const art = view.container.querySelector('img');
    expect(art).not.toBeNull();
    fireEvent.error(art!);
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('在月光下');
    expect(screen.getByRole('link', { name: '登录账户' })).toBeVisible();
});
it('does not open the native model directory to guests', async () => {
    const { client, fetcher } = fixtureClient(() => new Response(JSON.stringify({ success: false }), { status: 401 }));
    render(<MemoryRouter initialEntries={['/models']}><App client={client} /></MemoryRouter>);
    await screen.findByLabelText('用户名');
    expect(fetcher.mock.calls.some(c => c[0].includes('/models'))).toBe(false);
});
