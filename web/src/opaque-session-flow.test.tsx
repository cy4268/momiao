import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Account } from './Account';
import { api, ApiClient, ApiError } from './api';
import { App } from './App';
import { Authentication, DiscordCallback } from './Authentication';

const csrfA = 'a'.repeat(43), csrfB = 'b'.repeat(43), csrfC = 'd'.repeat(43), challenge = 'c'.repeat(43);
const chainIDA = 'f'.repeat(43), chainIDB = 'g'.repeat(43);
type PasswordProvider = { state: string; turnstile_required: boolean; turnstile_site_key?: string };
const password: PasswordProvider = { state: 'AVAILABLE', turnstile_required: false };
const unavailable = ['DISCORD_S2', 'GENERAL_BFF_S2', 'POKER_LOGOUT_S2'];
const user = { id: 12, username: 'opaque-user', display_name: 'Opaque User', role: 1, group: 'default', quota: 80, used_quota: 20, request_count: 2 };
const session = {
    auth_chain_started_at: '2026-09-08T00:00:00Z', absolute_expires_at: '2099-09-08T01:00:00Z', idle_expires_at: '2099-09-08T00:30:00Z', fresh_auth_at: null, fresh_auth_method: '',
};
const preauth = (authentication_state = 'ANONYMOUS', provider: PasswordProvider = password, csrf_token = csrfA) => ({ authentication_state, csrf_token, anonymous_expires_at: '2099-09-08T00:30:00Z', password: provider, unavailable });
const authenticated = (id = user.id, csrf_token = csrfB, auth_chain_started_at = session.auth_chain_started_at, auth_chain_id = chainIDA) => ({ authentication_state: 'AUTHENTICATED', csrf_token, identity: { newapi_user_id: String(id) }, session: { ...session, auth_chain_started_at, auth_chain_id }, password, unavailable });
const loginChallenge = { authentication_state: 'TWO_FA', csrf_token: csrfC, challenge: { challenge_id: challenge, expires_at: '2099-09-08T00:05:00Z' } };
const loginAccepted = (csrf_token = csrfB) => ({ authentication_state: 'AUTHENTICATED', csrf_token });
const logoutPending = { platform_session: 'REVOKED', native_session: 'PENDING_COMPENSATION', poker_control: 'UNAVAILABLE_S2' };
const logoutConfirmed = { platform_session: 'REVOKED', native_session: 'REVOKED', poker_control: 'UNAVAILABLE_S2' };
const profile = { user_id: '12', short_account_id: 'CA-012345ABCDEF', status: 'COMPLETE', display_name: '月海观测员', avatar_id: 'system-default', profile_version: '1', nickname_changed_at: null, next_rename_at: null, suggested_name: 'Master-CA-012345ABCDEF', avatars: [{ id: 'system-default', label: '系统默认头像', source: 'SYSTEM' }] };
const ok = (data: unknown, status = 200) => new Response(JSON.stringify({ success: true, data }), { status, headers: { 'Content-Type': 'application/json' } });
const fault = (status: number, code: string) => new Response(JSON.stringify({ success: false, error: { code } }), { status, headers: { 'Content-Type': 'application/json' } });
const headers = (call: unknown[]) => new Headers((call[1] as RequestInit).headers);
const body = (call: unknown[]) => JSON.parse(String((call[1] as RequestInit).body));
const deferred = () => { let resolve!: (response: Response) => void; const promise = new Promise<Response>(done => { resolve = done; }); return { promise, resolve }; };

afterEach(() => { vi.unstubAllEnvs(); });

describe('opt-in opaque browser session flow', () => {
    it('uses the owned Discord POST routes and adopts only the verified platform identity', async () => {
        const authorizationURL = 'https://discord.com/oauth2/authorize?client_id=123456789012345678&redirect_uri=https%3A%2F%2Fportal.example%2Foauth%2Fdiscord&response_type=code&scope=identify+guilds.members.read&state=fixture-state&prompt=consent';
        const fetcher = vi.fn()
            .mockResolvedValueOnce(ok({ ...preauth(), unavailable: [] }))
            .mockResolvedValueOnce(ok({ authentication_state: 'DISCORD_CALLBACK', csrf_token: csrfB, authorization_url: authorizationURL }))
            .mockResolvedValueOnce(ok(loginAccepted(csrfC)))
            .mockResolvedValueOnce(ok({ ...authenticated(user.id, csrfC), unavailable: [] }))
            .mockResolvedValueOnce(ok(user));
        const client = new ApiClient(fetcher, { sessionMode: 'opaque' });
        await client.bootstrap();
        expect(await client.discordStart('registration', 'https://portal.example')).toBe(authorizationURL);
        await client.discordCallback({ code: 'fixture-code', state: 'fixture-state' });
        expect(client.getSnapshot().user?.id).toBe(user.id);
        expect(fetcher.mock.calls[1][0]).toBe('/api/v1/auth/discord/registration/start');
        expect(fetcher.mock.calls[2][0]).toBe('/api/v1/auth/discord/callback');
        expect(fetcher.mock.calls[2][1]?.method).toBe('POST');
        expect(new Headers(fetcher.mock.calls[2][1]?.headers).get('X-CSRF-Token')).toBe(csrfB);
        expect(fetcher.mock.calls.some(([path]) => String(path).startsWith('/api/momiao/'))).toBe(false);
    });

    it('does not replay an uncertain Discord callback or send another mutation while its state is unknown', async () => {
        const fetcher = vi.fn()
            .mockResolvedValueOnce(ok({ ...preauth('DISCORD_CALLBACK'), unavailable: [] }))
            .mockResolvedValueOnce(new Response(JSON.stringify({ success: false, error: { code: 'AUTH_RESULT_UNKNOWN' } }), { status: 503 }));
        const client = new ApiClient(fetcher, { sessionMode: 'opaque' });
        await client.bootstrap();
        await expect(client.discordCallback({ code: 'fixture-code', state: 'fixture-state' })).rejects.toMatchObject({ code: 'AUTH_RESULT_UNKNOWN' });
        await expect(client.discordCallback({ code: 'fixture-code', state: 'fixture-state' })).rejects.toMatchObject({ code: 'AUTH_RESULT_UNKNOWN' });
        expect(fetcher).toHaveBeenCalledTimes(2);
        expect(client.getSnapshot().user).toBeNull();
    });
    it('reads public admission configuration for Master initialization without enabling Discord', async () => {
        const config = { enabled: true, registration_enabled: true, eligibility: 'Discord membership is required for new accounts.' };
        const fetcher = vi.fn().mockResolvedValueOnce(ok(preauth())).mockResolvedValueOnce(ok(config));
        const client = new ApiClient(fetcher, { sessionMode: 'opaque' });
        await client.bootstrap();
        expect(client.sessionFeatureUnavailable('DISCORD_S2')).toBe(true);
        expect(await client.admissionConfig()).toEqual(config);
        await expect(client.discordStart('registration', 'https://portal.example')).rejects.toMatchObject({ code: 'DISCORD_S2' });
        expect(fetcher.mock.calls.map(([path]) => path)).toEqual(['/api/v1/session/bootstrap', '/platform/v1/admission/config']);
    });

    it('keeps the shipped client on Native mode unless the build explicitly opts into opaque mode', async () => {
        expect(api.getSessionMode()).toBe('native');
        vi.stubEnv('VITE_MOMIAO_SESSION_MODE', 'opaque');
        vi.resetModules();
        expect((await import('./api')).api.getSessionMode()).toBe('opaque');
    });

    it('uses Cookie bootstrap, direct password login, refresh, and projected self without browser Native credentials', async () => {
        const fetcher = vi.fn()
            .mockResolvedValueOnce(ok(preauth()))
            .mockResolvedValueOnce(ok(loginAccepted()))
            .mockResolvedValueOnce(ok(authenticated()))
            .mockResolvedValueOnce(ok(user))
            .mockResolvedValueOnce(ok(authenticated(user.id, csrfC)))
            .mockResolvedValueOnce(ok(user));
        const client = new ApiClient(fetcher, { sessionMode: 'opaque' });
        await client.bootstrap();
        await client.login('opaque-user', 'secret');
        expect(client.getSnapshot().user).toEqual(user);
        const generation = client.getSessionGeneration();
        await client.refresh();
        expect(client.getSnapshot().user?.id).toBe(user.id);
        expect(client.getSessionGeneration()).toBe(generation);
        expect(fetcher.mock.calls.map(([path]) => path)).toEqual([
            '/api/v1/session/bootstrap', '/api/v1/auth/password/login', '/api/v1/session/bootstrap', '/api/user/self', '/api/v1/session/bootstrap', '/api/user/self',
        ]);
        expect(body(fetcher.mock.calls[1])).toEqual({ identifier: 'opaque-user', password: 'secret' });
        expect(headers(fetcher.mock.calls[1]).get('X-CSRF-Token')).toBe(csrfA);
        expect(headers(fetcher.mock.calls[3]).get('X-CSRF-Token')).toBe(csrfB);
        expect(headers(fetcher.mock.calls[5]).get('X-CSRF-Token')).toBe(csrfC);
        for (const index of [0, 2, 4]) expect(headers(fetcher.mock.calls[index]).has('X-CSRF-Token')).toBe(false);
        for (const call of fetcher.mock.calls) {
            const sent = headers(call);
            expect(sent.has('Authorization')).toBe(false); expect(sent.has('New-Api-User')).toBe(false); expect(sent.has('X-Auth-Session')).toBe(false);
            expect((call[1] as RequestInit).credentials).toBe('same-origin');
        }
    });

    it('submits the Native 2FA challenge once, rotates CSRF, then verifies projected self before publishing identity', async () => {
        const fetcher = vi.fn()
            .mockResolvedValueOnce(ok(preauth()))
            .mockResolvedValueOnce(ok(loginChallenge))
            .mockResolvedValueOnce(ok(loginAccepted()))
            .mockResolvedValueOnce(ok(authenticated()))
            .mockResolvedValueOnce(ok(user));
        const client = new ApiClient(fetcher, { sessionMode: 'opaque' });
        await client.bootstrap();
        expect(await client.login('opaque-user', 'secret', 'turnstile-token')).toEqual({ require_2fa: true, flow_token: challenge, expires_at: '2099-09-08T00:05:00Z' });
        expect(client.getSnapshot().user).toBeNull();
        expect(body(fetcher.mock.calls[1])).toEqual({ identifier: 'opaque-user', password: 'secret', turnstile_token: 'turnstile-token' });
        await client.verify2fa(challenge, '123456');
        expect(body(fetcher.mock.calls[2])).toEqual({ challenge_id: challenge, code: '123456' });
        expect(headers(fetcher.mock.calls[2]).get('X-CSRF-Token')).toBe(csrfC);
        expect(fetcher.mock.calls.slice(2).map(([path]) => path)).toEqual(['/api/v1/auth/password/2fa', '/api/v1/session/bootstrap', '/api/user/self']);
        expect(client.getSnapshot().user?.id).toBe(user.id);
        const calls = fetcher.mock.calls.length;
        await expect(client.verify2fa(challenge, '123456')).rejects.toBeInstanceOf(ApiError);
        expect(fetcher).toHaveBeenCalledTimes(calls);
    });

    it('recovers the server preauth state after an uncertain password result and never automatically resubmits credentials', async () => {
        const fetcher = vi.fn()
            .mockResolvedValueOnce(ok(preauth()))
            .mockResolvedValueOnce(fault(503, 'AUTH_RESULT_UNKNOWN'))
            .mockResolvedValueOnce(ok(preauth('UNKNOWN')));
        const client = new ApiClient(fetcher, { sessionMode: 'opaque' });
        await client.bootstrap();
        await expect(client.login('opaque-user', 'secret')).rejects.toMatchObject({ code: 'AUTH_RESULT_UNKNOWN', uncertain: true });
        expect(fetcher.mock.calls.map(([path]) => path)).toEqual(['/api/v1/session/bootstrap', '/api/v1/auth/password/login', '/api/v1/session/bootstrap']);
        await expect(client.login('opaque-user', 'secret')).rejects.toMatchObject({ code: 'AUTH_RESULT_UNKNOWN', uncertain: true });
        expect(fetcher).toHaveBeenCalledTimes(3);
    });

    it('reconciles malformed successful password and 2FA responses before permitting any repeat submit', async () => {
        const malformed = { authentication_state: 'AUTHENTICATED' };
        const passwordFetch = vi.fn()
            .mockResolvedValueOnce(ok(preauth()))
            .mockResolvedValueOnce(ok(malformed))
            .mockResolvedValueOnce(ok(authenticated()))
            .mockResolvedValueOnce(ok(user));
        const passwordClient = new ApiClient(passwordFetch, { sessionMode: 'opaque' });
        await passwordClient.bootstrap();
        await passwordClient.login('opaque-user', 'secret');
        expect(passwordClient.getSnapshot().user?.id).toBe(user.id);
        expect(passwordFetch.mock.calls.map(([path]) => path)).toEqual([
            '/api/v1/session/bootstrap', '/api/v1/auth/password/login', '/api/v1/session/bootstrap', '/api/user/self',
        ]);

        const twoFactorFetch = vi.fn()
            .mockResolvedValueOnce(ok(preauth()))
            .mockResolvedValueOnce(ok(loginChallenge))
            .mockResolvedValueOnce(ok(malformed))
            .mockResolvedValueOnce(ok(authenticated()))
            .mockResolvedValueOnce(ok(user));
        const twoFactorClient = new ApiClient(twoFactorFetch, { sessionMode: 'opaque' });
        await twoFactorClient.bootstrap();
        await twoFactorClient.login('opaque-user', 'secret');
        await twoFactorClient.verify2fa(challenge, '123456');
        expect(twoFactorClient.getSnapshot().user?.id).toBe(user.id);
        expect(twoFactorFetch.mock.calls.filter(([path]) => path === '/api/v1/auth/password/2fa')).toHaveLength(1);
    });

    it('restores an unconsumed 2FA preauth after timeout and waits for a new explicit submit', async () => {
        const fetcher = vi.fn()
            .mockResolvedValueOnce(ok(preauth()))
            .mockResolvedValueOnce(ok(loginChallenge))
            .mockRejectedValueOnce(new TypeError('offline'))
            .mockResolvedValueOnce(ok(preauth('TWO_FA')))
            .mockResolvedValueOnce(ok(loginAccepted()))
            .mockResolvedValueOnce(ok(authenticated()))
            .mockResolvedValueOnce(ok(user));
        const client = new ApiClient(fetcher, { sessionMode: 'opaque' });
        await client.bootstrap();
        await client.login('opaque-user', 'secret');
        await expect(client.verify2fa(challenge, '111111')).rejects.toMatchObject({ uncertain: true });
        expect(fetcher.mock.calls.filter(([path]) => path === '/api/v1/auth/password/2fa')).toHaveLength(1);
        expect(client.getSessionCapabilities().authenticationState).toBe('TWO_FA');
        await client.verify2fa(challenge, '222222');
        expect(fetcher.mock.calls.filter(([path]) => path === '/api/v1/auth/password/2fa')).toHaveLength(2);
        expect(client.getSnapshot().user?.id).toBe(user.id);
    });

    it('retains nested fault codes and makes legacy-cookie migration failure explicit without fallback', async () => {
        const invalidFetch = vi.fn().mockResolvedValueOnce(ok(preauth())).mockResolvedValueOnce(fault(401, 'AUTH_INVALID_CREDENTIALS'));
        const invalid = new ApiClient(invalidFetch, { sessionMode: 'opaque' }); await invalid.bootstrap();
        await expect(invalid.login('opaque-user', 'wrong')).rejects.toMatchObject({ code: 'AUTH_INVALID_CREDENTIALS', uncertain: false });
        expect(invalid.getSessionCapabilities().authenticationState).toBe('ANONYMOUS');

        const legacyFetch = vi.fn().mockResolvedValue(fault(400, 'AUTH_INPUT_INVALID'));
        const legacy = new ApiClient(legacyFetch, { sessionMode: 'opaque' }); await legacy.bootstrap();
        expect(legacy.getSnapshot().notice).toMatch(/原生刷新 Cookie|迁移/);
        expect(legacyFetch.mock.calls.map(([path]) => path)).toEqual(['/api/v1/session/bootstrap']);
    });

    it('treats identity mismatch as an uncertain accepted login and never publishes the wrong projected user', async () => {
        const fetcher = vi.fn().mockResolvedValueOnce(ok(preauth())).mockResolvedValueOnce(ok(loginAccepted())).mockResolvedValueOnce(ok(authenticated(12))).mockResolvedValueOnce(ok({ ...user, id: 13 }));
        const client = new ApiClient(fetcher, { sessionMode: 'opaque' }); await client.bootstrap();
        await expect(client.login('opaque-user', 'secret')).rejects.toMatchObject({ code: 'AUTH_IDENTITY_MISMATCH', uncertain: true });
        expect(client.getSnapshot().user).toBeNull();
        expect(fetcher.mock.calls.some(([path]) => path === '/api/user/login' || path === '/api/user/auth/refresh')).toBe(false);
    });

    it('clears immediately, waits for a late login cookie rotation, and uses csrfB for the one final logout', async () => {
        const pending = deferred();
        const fetcher = vi.fn().mockResolvedValueOnce(ok(preauth())).mockReturnValueOnce(pending.promise).mockResolvedValueOnce(ok(authenticated())).mockResolvedValueOnce(ok(logoutPending, 202));
        const client = new ApiClient(fetcher, { sessionMode: 'opaque' }); await client.bootstrap();
        const login = client.login('opaque-user', 'secret'); const logout = client.logout();
        expect(client.getSnapshot().user).toBeNull(); pending.resolve(ok(loginAccepted()));
        await expect(login).rejects.toMatchObject({ status: 401 }); await logout;
        expect(fetcher.mock.calls.map(([path]) => path)).toEqual(['/api/v1/session/bootstrap', '/api/v1/auth/password/login', '/api/v1/session/bootstrap', '/api/v1/auth/logout']);
        expect(body(fetcher.mock.calls[3])).toEqual({}); expect(headers(fetcher.mock.calls[3]).get('X-CSRF-Token')).toBe(csrfB);
        expect(client.getSnapshot()).toMatchObject({ user: null, ready: true, loggingOut: false });
        expect(client.getSnapshot().notice).toContain('原生 PENDING_COMPENSATION');
    });

    it('aborts an in-flight private read on logout and never republishes its late payload', async () => {
        let readSignal: AbortSignal | undefined;
        const fetcher = vi.fn(async (path: string, init?: RequestInit) => {
            if (path === '/api/v1/session/bootstrap') return ok(authenticated());
            if (path === '/api/user/self') return ok(user);
            if (path === '/api/v1/private') return new Promise<Response>(resolve => {
                readSignal = init?.signal as AbortSignal;
                readSignal?.addEventListener('abort', () => resolve(ok({ private: 'late' })), { once: true });
                setTimeout(() => resolve(ok({ private: 'late' })), 25);
            });
            if (path === '/api/v1/auth/logout') return ok(logoutConfirmed);
            return fault(404, 'UNEXPECTED_ROUTE');
        });
        const client = new ApiClient(fetcher, { sessionMode: 'opaque' }); await client.bootstrap();
        const read = client.request('/api/v1/private');
        await Promise.resolve();
        await client.logout();
        await expect(read).rejects.toMatchObject({ status: 401 });
        expect(readSignal?.aborted).toBe(true);
        expect(client.getSnapshot().user).toBeNull();
        expect(client.getSnapshot().notice).toMatch(/平台与原生会话已确认退出.*Poker.*UNAVAILABLE_S2/);
    });

    it('expires an opaque identity on the first rejected private read without a Native refresh retry', async () => {
        const fetcher = vi.fn().mockResolvedValueOnce(ok(authenticated())).mockResolvedValueOnce(ok(user)).mockResolvedValueOnce(fault(401, 'SESSION_UNAUTHORIZED'));
        const client = new ApiClient(fetcher, { sessionMode: 'opaque' }); await client.bootstrap();
        await expect(client.request('/api/v1/private')).rejects.toMatchObject({ code: 'SESSION_UNAUTHORIZED' });
        expect(client.getSnapshot().user).toBeNull();
        expect(client.getSnapshot().notice).toMatch(/登录已过期|访问被拒绝/);
        expect(fetcher.mock.calls.map(([path]) => path)).toEqual(['/api/v1/session/bootstrap', '/api/user/self', '/api/v1/private']);
    });

    it('fences a stale tab when a shared Cookie jar switches to another account without deleting the new Cookie', async () => {
        const user13 = { ...user, id: 13, username: 'other-user', display_name: 'Other User' };
        const chainA = '2026-09-08T00:00:00Z', chainB = '2026-09-08T00:10:00Z';
        let cookieJar: { state: 'ANONYMOUS' | 'AUTHENTICATED'; csrf: string; account: typeof user; chain: string } = {
            state: 'AUTHENTICATED', csrf: csrfB, account: user, chain: chainA,
        };
        const fetcher = vi.fn(async (path: string, init?: RequestInit) => {
            if (path === '/api/v1/session/bootstrap') return ok(cookieJar.state === 'AUTHENTICATED'
                ? authenticated(cookieJar.account.id, cookieJar.csrf, cookieJar.chain, cookieJar.account.id === user.id ? chainIDA : chainIDB)
                : preauth('ANONYMOUS', password, cookieJar.csrf));
            if (path === '/api/v1/auth/password/login') {
                cookieJar = { state: 'AUTHENTICATED', csrf: csrfC, account: user13, chain: chainB };
                return ok(loginAccepted(csrfC));
            }
            if (path === '/api/user/self') return ok(cookieJar.account);
            if (path === '/api/v1/private') {
                const expected = headers([path, init]).get('X-CSRF-Token');
                return expected && expected !== cookieJar.csrf ? fault(403, 'SESSION_CSRF_FAILED') : ok({ owner: cookieJar.account.id });
            }
            return fault(404, 'UNEXPECTED_ROUTE');
        });
        const staleTab = new ApiClient(fetcher, { sessionMode: 'opaque' });
        await staleTab.bootstrap();
        expect(staleTab.getSnapshot().user?.id).toBe(user.id);

        cookieJar = { state: 'ANONYMOUS', csrf: csrfA, account: user, chain: '' };
        const switchingTab = new ApiClient(fetcher, { sessionMode: 'opaque' });
        await switchingTab.bootstrap();
        await switchingTab.login('other-user', 'secret');
        expect(switchingTab.getSnapshot().user?.id).toBe(user13.id);

        await expect(staleTab.request('/api/v1/private')).rejects.toMatchObject({ code: 'SESSION_CSRF_FAILED' });
        const privateCall = fetcher.mock.calls.find(([path]) => path === '/api/v1/private');
        expect(headers(privateCall!).get('X-CSRF-Token')).toBe(csrfB);
        expect(staleTab.getSnapshot().user).toBeNull();
        expect(staleTab.getSessionCapabilities().authenticationState).toBe('UNINITIALIZED');
        expect(staleTab.getSessionCapabilities().unavailable).toEqual(unavailable);
        expect(switchingTab.getSnapshot().user?.id).toBe(user13.id);
        expect(fetcher.mock.calls.some(([path]) => path === '/api/v1/auth/logout')).toBe(false);
        expect(fetcher.mock.calls.filter(([path]) => path === '/api/v1/session/bootstrap')).toHaveLength(3);
    });

    it('uses the just-returned bootstrap CSRF to fence a Cookie swap before projected self', async () => {
        let currentCSRF = csrfB;
        const fetcher = vi.fn(async (path: string, init?: RequestInit) => {
            if (path === '/api/v1/session/bootstrap') {
                const response = ok(authenticated(user.id, currentCSRF));
                currentCSRF = csrfC;
                return response;
            }
            if (path === '/api/user/self') return headers([path, init]).get('X-CSRF-Token') === currentCSRF
                ? ok({ ...user, id: 13 })
                : fault(403, 'SESSION_CSRF_FAILED');
            return fault(404, 'UNEXPECTED_ROUTE');
        });
        const client = new ApiClient(fetcher, { sessionMode: 'opaque' });
        await client.bootstrap();
        expect(headers(fetcher.mock.calls[1]).get('X-CSRF-Token')).toBe(csrfB);
        expect(client.getSnapshot().user).toBeNull();
        expect(client.getSessionCapabilities()).toMatchObject({ authenticationState: 'UNINITIALIZED', password: { state: 'AVAILABLE' }, unavailable });
        expect(fetcher.mock.calls.some(([path]) => path === '/api/v1/auth/logout')).toBe(false);
    });

    it('advances generation before self when the same user and timestamp have a different auth-chain ID', async () => {
        const oldRead = deferred(), newSelf = deferred();
        const chainTime = '2026-09-08T00:00:00Z';
        let current = { csrf: csrfB, chainID: chainIDA };
        let readSignal: AbortSignal | undefined, selfCalls = 0;
        const fetcher = vi.fn(async (path: string, init?: RequestInit) => {
            if (path === '/api/v1/session/bootstrap') return ok(authenticated(user.id, current.csrf, chainTime, current.chainID));
            if (path === '/api/user/self') return ++selfCalls === 1 ? ok(user) : newSelf.promise;
            if (path === '/api/v1/private') {
                readSignal = init?.signal as AbortSignal;
                return oldRead.promise;
            }
            return fault(404, 'UNEXPECTED_ROUTE');
        });
        const client = new ApiClient(fetcher, { sessionMode: 'opaque' });
        await client.bootstrap();
        const generation = client.getSessionGeneration();
        const read = client.request<{ owner: number }>('/api/v1/private');
        await vi.waitFor(() => expect(readSignal).toBeDefined());

        current = { csrf: csrfC, chainID: chainIDB };
        const refresh = client.refresh();
        await vi.waitFor(() => expect(selfCalls).toBe(2));
        const wasAbortedBeforeSelf = readSignal?.aborted;
        newSelf.resolve(ok(user));
        await refresh;
        oldRead.resolve(ok({ owner: user.id }));

        await expect(read).rejects.toMatchObject({ status: 401 });
        expect(client.getSessionGeneration()).toBe(generation + 1);
        expect(wasAbortedBeforeSelf).toBe(true);
        expect(client.getSnapshot().user?.id).toBe(user.id);
    });

    it('discards a superseded deferred bootstrap before it can overwrite the new local boundary', async () => {
        const staleBootstrap = deferred();
        let bootstrapCalls = 0;
        const stale = {
            ...authenticated(user.id, csrfC, session.auth_chain_started_at, chainIDB),
            password: { state: 'DISABLED', turnstile_required: false },
            unavailable: ['STALE_CAPABILITY'],
            session: { ...session, auth_chain_id: chainIDB, idle_expires_at: '2099-09-08T00:59:00Z' },
        };
        const fetcher = vi.fn(async (path: string) => {
            if (path === '/api/v1/session/bootstrap') return ++bootstrapCalls === 1 ? ok(authenticated()) : staleBootstrap.promise;
            if (path === '/api/user/self') return ok(user);
            if (path === '/api/v1/private') return fault(403, 'SESSION_CSRF_FAILED');
            return fault(404, 'UNEXPECTED_ROUTE');
        });
        const client = new ApiClient(fetcher, { sessionMode: 'opaque' });
        await client.bootstrap();
        const refresh = client.refresh();
        await vi.waitFor(() => expect(bootstrapCalls).toBe(2));
        await expect(client.request('/api/v1/private')).rejects.toMatchObject({ code: 'SESSION_CSRF_FAILED' });
        expect(client.getSessionCapabilities()).toMatchObject({ authenticationState: 'UNINITIALIZED', password: { state: 'AVAILABLE' }, unavailable });

        staleBootstrap.resolve(ok(stale));
        await expect(refresh).rejects.toMatchObject({ status: 401 });
        expect(client.getSnapshot().user).toBeNull();
        expect(client.getSessionCapabilities()).toMatchObject({ authenticationState: 'UNINITIALIZED', password: { state: 'AVAILABLE' }, unavailable });
        expect(Reflect.get(client, 'opaqueCSRF')).toBe('');
        expect(Reflect.get(client, 'expires')).toBe(0);
    });

    it('sends distinct platform and game CSRF headers on an opaque game write', async () => {
        const gameCSRF = 'e'.repeat(64);
        const fetcher = vi.fn()
            .mockResolvedValueOnce(ok(authenticated()))
            .mockResolvedValueOnce(ok(user))
            .mockResolvedValueOnce(ok({ receipt: 'accepted' }));
        const client = new ApiClient(fetcher, { sessionMode: 'opaque' });
        await client.bootstrap();
        await client.request('/api/v1/games/dice/rounds', 'POST', { wager: 1 }, { 'X-CSRF-Token': gameCSRF, 'Idempotency-Key': 'round-1' });
        const sent = headers(fetcher.mock.calls[2]);
        expect(sent.get('X-CSRF-Token')).toBe(csrfB);
        expect(sent.get('X-Game-CSRF-Token')).toBe(gameCSRF);
        expect(sent.get('Idempotency-Key')).toBe('round-1');
        fetcher.mockResolvedValueOnce(ok({asset_id:'accepted'}));const form=new FormData();form.append('metadata','{}');
        await client.request('/platform/v1/ops/models/family-covers/upload','POST',form);
        const upload=headers(fetcher.mock.calls[3]);expect(upload.get('X-CSRF-Token')).toBe(csrfB);
        for(const name of ['Content-Type','X-Game-CSRF-Token','Authorization','New-Api-User','X-Auth-Session'])expect(upload.has(name)).toBe(false);
        expect(fetcher.mock.calls[3][1]?.body).toBe(form);

    });

    it('keeps Account, Discord, and Poker routes visible while truthfully holding unavailable S2 capabilities', async () => {
        const anonymousFetch = vi.fn().mockResolvedValueOnce(ok(preauth()));
        const anonymousClient = new ApiClient(anonymousFetch, { sessionMode: 'opaque' }); await anonymousClient.bootstrap();
        const login = render(<MemoryRouter><Authentication client={anonymousClient} mode="login" /></MemoryRouter>);
        expect(screen.getByText(/Discord.*尚未接入/)).toBeVisible(); expect(screen.getByRole('button', { name: '登录控制台' })).toBeEnabled();
        login.unmount();
        const callback = render(<MemoryRouter><DiscordCallback client={anonymousClient} captured={{ input: { code: 'private-code', state: 'private-state' } }} onComplete={vi.fn()} /></MemoryRouter>);
        expect(await screen.findByRole('alert')).toHaveTextContent(/Discord.*尚未接入/); callback.unmount();
        expect(anonymousFetch).toHaveBeenCalledTimes(1);

        const accountFetch = vi.fn().mockResolvedValueOnce(ok(authenticated())).mockResolvedValueOnce(ok(user)).mockResolvedValueOnce(ok({items:[]}));
        const accountClient = new ApiClient(accountFetch, { sessionMode: 'opaque' }); await accountClient.bootstrap();
        const account = render(<MemoryRouter><Account client={accountClient} /></MemoryRouter>);
        expect(screen.getByRole('alert')).toHaveTextContent(/密码变更.*尚未接入/);
        expect(document.querySelector('.identity-chamber.chamber-security')).toBeInTheDocument();
        expect(screen.queryByLabelText('新密码')).not.toBeInTheDocument(); account.unmount();
        const poker = render(<MemoryRouter initialEntries={['/poker']}><App client={accountClient} /></MemoryRouter>);
        expect(await screen.findByRole('alert')).toHaveTextContent(/Poker.*尚未接入/); poker.unmount();
        await waitFor(() => expect(accountFetch.mock.calls.map(([path]) => path)).toEqual(['/api/v1/session/bootstrap', '/api/user/self', '/api/v1/maintenance/notices']));
    });

    it('mounts the canonical account security route without replacing the legacy route', async () => {
        const fetcher = vi.fn(async (path: string) => {
            if (path === '/api/v1/session/bootstrap') return ok(authenticated());
            if (path === '/api/user/self') return ok(user);
            if (path.startsWith('/platform/v1/access-gate?')) return ok({ user_id: '12', route: '/account/security', stage: 'READY' });
            if (path === '/platform/v1/master-profile') return ok(profile);
            return fault(404, 'UNEXPECTED_ROUTE');
        });
        const client = new ApiClient(fetcher, { sessionMode: 'opaque' }); await client.bootstrap();
        render(<MemoryRouter initialEntries={['/account/security']}><App client={client} /></MemoryRouter>);
        expect(await screen.findByRole('heading', { name: '账户与安全' })).toBeVisible();
        expect(fetcher.mock.calls.some(([path]) => String(path).startsWith('/platform/v1/access-gate?'))).toBe(true);
    });

    it('keeps password submission disabled until the required human check produces a token', async () => {
        const fetcher = vi.fn().mockResolvedValueOnce(ok(preauth('ANONYMOUS', { state: 'AVAILABLE', turnstile_required: true, turnstile_site_key: 'site-key' })));
        const client = new ApiClient(fetcher, { sessionMode: 'opaque' }); await client.bootstrap();
        render(<MemoryRouter><Authentication client={client} mode="login" /></MemoryRouter>);
        expect(screen.getByRole('region', { name: '安全验证' })).toBeVisible();
        expect(screen.getByRole('button', { name: '登录控制台' })).toBeDisabled();
    });
});
