import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, it } from 'vitest';
import { App } from './App';
import { bundle, daily, failed, fixtureClient, ok, profile, receipt, user, wallet } from './m1-test-fixtures';

afterEach(() => sessionStorage.clear());
const nav = (name: string) => within(screen.getByRole('navigation', { name: '主导航' })).getByRole('link', { name });
it('claims fixed Shanghai-day rewards only on click, refreshes confirmed status, and links activation', async () => {
    let claimed = false;
    const { client, fetcher } = fixtureClient((p, init) => {
        if (p.endsWith('/daily/claim') && init?.method === 'POST') { claimed = true; return ok(receipt); }
        if (p.endsWith('/rewards/daily')) return ok({ ...daily, claimed, transaction_id: claimed ? receipt.id : null });
    });
    render(<MemoryRouter initialEntries={['/rewards']}><App client={client} /></MemoryRouter>);
    const button = await screen.findByRole('button', { name: '领取今日 500 额度' });
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('奖励中心');
    expect(screen.queryByLabelText('兑换数量')).not.toBeInTheDocument();
    expect(fetcher.mock.calls.some(c => c[0].includes('/claim'))).toBe(false);
    fireEvent.click(button); fireEvent.click(button);
    expect(await screen.findByRole('button', { name: '今日已领取' })).toBeDisabled();
    expect(fetcher.mock.calls.filter(c => c[0].endsWith('/claim'))).toHaveLength(1);
    expect(Object.keys(JSON.parse(String(fetcher.mock.calls.find(c => c[0].endsWith('/claim'))![1]?.body)))).toEqual(['idempotency_key']);
    expect(screen.getByRole('main').querySelector('a[href="/wallet/activate"]')).not.toBeNull();
    fireEvent.click(nav('我的钱包'));
    expect(await screen.findByRole('button', { name: '今日已领取' })).toBeDisabled();
});
it('keeps a lost daily request locked from rewards to wallet, reconciles its key without another POST', async () => {
    const { client, fetcher } = fixtureClient(p => {
        if (p.endsWith('/daily/claim')) throw new TypeError('lost');
        if (p.includes('/transactions/by-key')) return ok(receipt);
    });
    render(<MemoryRouter initialEntries={['/rewards']}><App client={client} /></MemoryRouter>);
    fireEvent.click(await screen.findByRole('button', { name: '领取今日 500 额度' }));
    await screen.findByRole('alert');
    const pending = JSON.parse(sessionStorage.getItem('momiao.wallet.pending.1')!);
    fireEvent.click(nav('我的钱包'));
    const reconcile = await screen.findByRole('button', { name: '核对交易结果' });
    expect(await screen.findByRole('button', { name: '领取今日 500 额度' })).toBeDisabled();
    fireEvent.click(reconcile);
    await waitFor(() => expect(sessionStorage.getItem('momiao.wallet.pending.1')).toBeNull());
    expect(fetcher.mock.calls.some(c => c[0] === '/platform/v1/transactions/by-key?kind=DAILY&key=' + pending.key)).toBe(true);
    expect(fetcher.mock.calls.filter(c => c[0].endsWith('/claim'))).toHaveLength(1);
});
it('recovers an existing unknown exchange on rewards instead of creating a competing daily operation', async () => {
    const key = '01990000-1111-7777-aaaa-000000000002';
    sessionStorage.setItem('momiao.wallet.pending.1', JSON.stringify({ kind: 'EXCHANGE', key, amount: '1', from_asset: 'RESERVE_API_CREDIT' }));
    const { client, fetcher } = fixtureClient((p, init) => {
        if (p.includes('/transactions/by-key')) return ok(null);
        if (p.endsWith('/wallet/exchange') && init?.method === 'POST') return ok({ ...receipt, kind: 'LOCAL_EXCHANGE', from_asset: 'RESERVE_API_CREDIT', to_asset: 'AVAILABLE_CHIPS', amount: '1', amount_units: '500000' });
    });
    render(<MemoryRouter initialEntries={['/rewards']}><App client={client} /></MemoryRouter>);
    expect(await screen.findByRole('button', { name: '领取今日 500 额度' })).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: '核对交易结果' }));
    fireEvent.click(await screen.findByRole('button', { name: '按原请求重试' }));
    await waitFor(() => expect(sessionStorage.length).toBe(0));
    expect(fetcher.mock.calls.some(c => c[0].endsWith('/daily/claim'))).toBe(false);
    expect(JSON.parse(String(fetcher.mock.calls.find(c => c[0].endsWith('/wallet/exchange'))![1]?.body))).toEqual({ idempotency_key: key, from_asset: 'RESERVE_API_CREDIT', amount: '1' });
});
it.each(['wallet', 'daily', 'uninitialized'])('blocks claiming when %s cannot confirm readiness', async state => {
    const { client, fetcher } = fixtureClient(p => {
        if (p === '/platform/v1/wallet') return state === 'wallet' ? failed() : state === 'uninitialized' ? ok({ ...wallet, initialized: false, wallets: [] }) : undefined;
        if (p === '/platform/v1/rewards/daily' && state === 'daily') return failed();
    });
    render(<MemoryRouter initialEntries={['/rewards']}><App client={client} /></MemoryRouter>);
    await screen.findByRole('heading', { name: '奖励中心' });
    if (state === 'uninitialized') await screen.findByRole('link', { name: '前往初始化钱包' });
    else await screen.findByRole('alert');
    expect(screen.queryByRole('button', { name: '领取今日 500 额度' })).not.toBeInTheDocument();
    expect(fetcher.mock.calls.some(c => c[0].includes('/claim') || c[0].includes('/initialize'))).toBe(false);
});
it('never consumes a previous account receipt after switching sessions', async () => {
    let resolve!: (value: Response) => void;
    let account = 1;
    const { client, fetcher } = fixtureClient(p => {
        if (p.endsWith('/daily/claim')) return new Promise<Response>(r => { resolve = r; });
        if (account === 2) {
            if (p === '/api/user/login') return ok({ ...bundle, user: { ...user, id: 2 }, session: { sid: 'second' } });
            if (p === '/api/user/self') return ok({ ...user, id: 2 });
            if (p === '/platform/v1/master-profile') return ok({ ...profile, user_id: '2' });
            if (p === '/platform/v1/wallet') return ok({ ...wallet, user_id: '2' });
            if (p === '/platform/v1/rewards/daily') return ok({ ...daily, user_id: '2' });
        }
    });
    render(<MemoryRouter initialEntries={['/rewards']}><App client={client} /></MemoryRouter>);
    fireEvent.click(await screen.findByRole('button', { name: '领取今日 500 额度' }));
    await waitFor(() => expect(resolve).toBeTypeOf('function'));
    await act(async () => { await client.logout(); account = 2; await client.login('second', 'fixture'); resolve(ok(receipt)); });
    fireEvent.click(nav('奖励中心'));
    expect(await screen.findByRole('button', { name: '领取今日 500 额度' })).toBeEnabled();
    expect(screen.queryByRole('button', { name: '核对交易结果' })).not.toBeInTheDocument();
    expect(sessionStorage.getItem('momiao.wallet.pending.1')).not.toBeNull();
    expect(sessionStorage.getItem('momiao.wallet.pending.2')).toBeNull();
    expect(fetcher.mock.calls.filter(c => c[0].endsWith('/claim'))).toHaveLength(1);
});
