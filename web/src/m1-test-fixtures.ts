import { vi } from 'vitest';
import { ApiClient } from './api';

// Synthetic identity and balances: this helper is imported only by tests.
export const user = { id: 1, username: 'fixture-account', display_name: 'Native Fixture', role: 1, quota: 0, used_quota: 0, request_count: 0 };
export const profile = { user_id: '1', short_account_id: 'CA-012345ABCDEF', status: 'COMPLETE', display_name: '月海观测员', avatar_id: 'system-default', profile_version: '1', nickname_changed_at: null, next_rename_at: null, suggested_name: 'Master-CA-012345ABCDEF', avatars: [{ id: 'system-default', label: '系统默认头像', source: 'SYSTEM' }] };
export const receipt = { id: '01990000-1111-7777-aaaa-000000000001', user_id: '1', biz_id: 'daily:1:2026-09-06', kind: 'DAILY_REWARD', status: 'CONFIRMED', from_asset: '', to_asset: 'RESERVE_API_CREDIT', amount_units: '250000000', amount: '500', created_at: '2026-09-06T00:00:00Z', confirmed_at: '2026-09-06T00:00:00Z' };
export const daily = { user_id: '1', business_date: '2026-09-06', timezone: 'Asia/Shanghai', next_reset_at: '2026-09-06T16:00:00Z', amount: '500', amount_units: '250000000', asset: 'RESERVE_API_CREDIT', policy_version: '1', claimed: false, transaction_id: null };
export const wallet = { initialized: true, user_id: '1', scope: 'LOCAL_WALLETS_ONLY', total_assets: null, wallets: [{ asset: 'RESERVE_API_CREDIT', amount: '500', balance_units: '250000000', ledger_seq: '1', version: '2' }, { asset: 'AVAILABLE_CHIPS', amount: '0', balance_units: '0', ledger_seq: '0', version: '1' }] };
export const ok = (data?: unknown) => new Response(JSON.stringify({ success: true, data }));
export const failed = () => new Response(JSON.stringify({ success: false, message: 'fixture unavailable' }), { status: 503 });
export const bundle = { access_token: 'fixture-only', access_expires_at: 9999999999, user, session: { sid: 'fixture-session' } };
export function fixtureClient(respond?: (path: string, init?: RequestInit) => Response | Promise<Response> | undefined) {
    const fetcher = vi.fn(async (path: string, init?: RequestInit) => {
        const response = respond?.(path, init);
        if (response) return response;
        if (path.includes('/auth/refresh') || path === '/api/user/login') return ok(bundle);
        if (path === '/api/user/self') return ok(user);
        if (path === '/platform/v1/master-profile') return ok(profile);
        if (path === '/platform/v1/wallet') return ok(wallet);
        if (path === '/platform/v1/rewards/daily') return ok(daily);
        if (path.startsWith('/platform/v1/transactions')) return ok({ items: [], has_more: false, next_after_id: null });
        if (path.startsWith('/platform/v1/wallet/ledger')) return ok({ items: [], has_more: false, next_after_seq: null });
        return ok({ items: [], total: 0, page: 1, page_size: 10 });
    });
    return { client: new ApiClient(fetcher), fetcher };
}
