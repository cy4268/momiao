// Copyright QuantumNous and contributors. AGPL-3.0-or-later.
// Password -> optional 2FA -> user, adapted from official new-api@f1164142
// web/src/features/auth/api.ts and sign-in/components/user-auth-form.tsx.
// momiao uses the existing HttpOnly Redis cookie, not a browser bearer token.
import { serializeCatalogQuery, type CatalogQuery } from './game-catalog-query';
import { validateDiscordAuthorization, type AdmissionConfig, type AdmissionResult, type DiscordCallbackInput, type DiscordPurpose } from './admission-api';
export interface User {
    id: number;
    username: string;
    display_name: string;
    role: number;
    group?: string;
    quota?: number;
    reserve_amount?: string;
    used_quota?: number | null;
    request_count?: number | null;
}
export interface Key {
    id: number;
    name: string;
    key: string;
    status: number;
    created_time: number;
    expired_time: number;
    remain_quota: number;
    used_quota: number;
    unlimited_quota: boolean;
}
export interface UsageLog {
    id: number;
    created_at: number;
    type: number;
    token_name: string;
    model_name: string;
    quota: number;
    prompt_tokens: number;
    completion_tokens: number;
}
export interface Page<T> {
    items: T[];
    total: number;
    page: number;
    page_size: number;
}

export interface TwoFactor { require_2fa: true; flow_token: string; expires_at?: string }
interface SessionResponse {
    authentication_state: 'ANONYMOUS' | 'AUTHENTICATED' | 'TWO_FA';
    csrf_token: string;
    challenge?: { challenge_id: string; expires_at: string };
}
interface Snapshot { user: User | null; ready: boolean; loggingOut: boolean; notice: string }
export class ApiError extends Error {
    constructor(message: string, public status = 0, public code = '', public uncertain = false) { super(message); this.name = 'ApiError'; }
}
export const errorText = (e: unknown) => e instanceof Error ? e.message : '请求未完成，请稍后重试。';
export class ApiClient {
    async rankingsRequest<T>(query: URLSearchParams, mine = false): Promise<T> {
        const path = '/api/v1/rankings'+(mine?'/me':'')+'?'+query.toString();
        if (mine) return this.request<T>(path);
        const epoch = this.epoch;
        const result = await this.raw<T>(path);
        if (epoch !== this.epoch) throw new ApiError('登录状态已改变，请重新读取排行。',401);
        return result;
    }
    async gameCatalog<T>(query?:CatalogQuery): Promise<T> {
        const epoch = this.epoch;
        const result = await this.raw<T>('/api/v1/games'+(query ? serializeCatalogQuery(query) : ''));
        if (epoch !== this.epoch) throw new ApiError('登录状态已改变，请重新读取游戏。', 401);
        return result;
    }
    async catalogRequest<T>(path: string): Promise<T> {
        if (!/^\/platform\/v1\/models(?:\?.*)?$/.test(path) && !/^\/platform\/v1\/models\/(?:detail|access-config)(?:\?.*)?$/.test(path)) throw new ApiError('无效的公开模型路径。');
        const epoch = this.epoch;
        const result = await this.raw<T>(path);
        if (epoch !== this.epoch) throw new ApiError('登录状态已改变，请重新读取模型。', 401);
        return result;
    }
    async announcementRequest<T>(path: string): Promise<T> {
        if (!/^\/platform\/v1\/announcements(?:$|[/?])/.test(path)) throw new ApiError('无效的公告路径。');
        if (this.snapshot.user) return this.request<T>(path);
        const epoch = this.epoch;
        const result = await this.raw<T>(path);
        if (epoch !== this.epoch || this.snapshot.user) throw new ApiError('登录状态已改变，请重新读取公告。', 401);
        return result;
    }

    private csrf = '';
    private epoch = 0;
    private reads = new Set<AbortController>();
    private snapshot: Snapshot = { user: null, ready: false, loggingOut: false, notice: '' };
    private listeners = new Set<() => void>();
    private refreshFlight: Promise<void> | null = null;
    private loginFlight: Promise<TwoFactor | void> | null = null;
    private logoutFlight: Promise<void> | null = null;
    constructor(private fetcher: (path: string, init?: RequestInit) => Promise<Response> = (...args) => fetch(...args)) {}
    getSnapshot = () => this.snapshot;
    getSessionGeneration = () => this.epoch;
    subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn); }; };
    private publish(patch: Partial<Snapshot>) { this.snapshot = { ...this.snapshot, ...patch }; this.listeners.forEach(fn => fn()); }
    private advanceSessionBoundary() { this.epoch++; this.reads.forEach(c => c.abort()); this.reads.clear(); }
    private clear(notice = '') { this.advanceSessionBoundary(); this.csrf = ''; this.publish({ user: null, ready: true, notice }); }
    private check(epoch: number) {
        if (epoch !== this.epoch || this.snapshot.loggingOut) throw new ApiError('登录状态已改变，请重新登录。', 401);
    }
    private writeHeaders() { return new Headers({ 'X-CSRF-Token': this.csrf }); }
    private async raw<T>(path: string, method = 'GET', body?: unknown, headers = new Headers()): Promise<T> {
        const multipart = body instanceof FormData;
        if (multipart && (path !== '/platform/v1/ops/models/family-covers/upload' || method !== 'POST'))
            throw new ApiError('此接口不接受文件上传。', 400, 'INVALID_REQUEST');
        headers.set('Accept', 'application/json');
        if (multipart) headers.delete('Content-Type');
        else if (body !== undefined)
            headers.set('Content-Type', 'application/json');
        const read = method === 'GET' || method === 'HEAD';
        const controller = read ? new AbortController() : undefined;
        if (controller) this.reads.add(controller);
        try {
            let response: Response;
            try {
                const timeout = AbortSignal.timeout(multipart ? 95000 : 25000);
                const signal = controller ? AbortSignal.any([timeout, controller.signal]) : timeout;
                response = await this.fetcher(path, { method, headers, body: multipart ? body : body === undefined ? undefined : JSON.stringify(body), credentials: 'same-origin', cache: 'no-store', signal });
            }
            catch {
                throw new ApiError(read ? '网络连接中断，请检查连接后重试。' : '网络连接中断，操作结果尚未确认。请先刷新列表核对，勿重复提交。', 0, '', !read);
            }
            let envelope: {
                success?: boolean;
                message?: string;
                code?: string;
                error?: { code?: string; message?: string };
                data: T;
            };
            try {
                envelope = await response.json();
            }
            catch {
                throw new ApiError(`服务响应格式异常（HTTP ${response.status}）。写入结果尚未确认时，请先刷新列表核对。`, response.status, '', !read);
            }
            if (!response.ok || envelope?.success !== true) {
                const code = typeof envelope?.error?.code === 'string' ? envelope.error.code : typeof envelope?.code === 'string' ? envelope.code : '';
                const message = typeof envelope?.error?.message === 'string' && envelope.error.message ? envelope.error.message : typeof envelope?.message === 'string' && envelope.message ? envelope.message : `请求未完成（HTTP ${response.status}）。`;
                throw new ApiError(message, response.status, code, !read && (response.status >= 500 || code === 'AUTH_RESULT_UNKNOWN'));
            }
            return envelope.data;
        } finally {
            if (controller) this.reads.delete(controller);
        }
    }

    private async acceptSession(session: SessionResponse, epoch: number) {
        this.check(epoch);
        const user = session.authentication_state === 'AUTHENTICATED' ? await this.raw<User>('/api/user/self') : null;
        this.check(epoch);
        if (this.csrf !== session.csrf_token || this.snapshot.user?.id !== user?.id) this.advanceSessionBoundary();
        this.csrf = session.csrf_token;
        this.publish({ user, ready: true, notice: '' });
    }
    refresh(): Promise<void> {
        if (this.snapshot.loggingOut) return Promise.reject(new ApiError('正在退出登录。',401));
        if (this.refreshFlight) return this.refreshFlight;
        const epoch = this.epoch;
        this.refreshFlight = this.raw<SessionResponse>('/api/v1/session/bootstrap').then(s => this.acceptSession(s, epoch))
            .catch(e => { if (epoch === this.epoch) this.clear(errorText(e)); throw e; })
            .finally(() => { this.refreshFlight = null; });
        return this.refreshFlight;
    }
    async bootstrap(): Promise<void> {
        if (this.snapshot.ready) return;
        try { await this.refresh(); } catch { /* refresh publishes the readable error. */ }
    }
    private authenticate(path: string, body: unknown): Promise<TwoFactor | void> {
        if (this.loginFlight || this.snapshot.loggingOut) return Promise.reject(new ApiError('登录请求处理中，请稍候。'));
        const work = async () => {
            if (!this.csrf) await this.refresh();
            if (this.snapshot.loggingOut) throw new ApiError('正在退出登录。',401);
            const epoch = this.epoch;
            const result = await this.raw<SessionResponse>(path, 'POST', body, this.writeHeaders());
            this.check(epoch);
            if (result.authentication_state === 'TWO_FA' && result.challenge) {
                return { require_2fa: true as const, flow_token: result.challenge.challenge_id, expires_at: result.challenge.expires_at };
            }
            await this.acceptSession(result, epoch);
        };
        this.loginFlight = work().finally(() => { this.loginFlight = null; });
        return this.loginFlight;
    }
    login = (username: string, password: string) => this.authenticate('/api/v1/auth/password/login', { identifier: username, password });
    verify2fa = (flow_token: string, code: string) => this.authenticate('/api/v1/auth/password/2fa', { challenge_id: flow_token, code });
    admissionConfig = () => this.raw<AdmissionConfig>('/platform/v1/admission/config');
    async discordStart(purpose: DiscordPurpose, origin = window.location.origin): Promise<string> {
        if (!this.csrf) await this.refresh();
        const data = await this.raw<{authorization_url: string}>(`/api/v1/auth/discord/${purpose}/start`, 'POST', {}, this.writeHeaders());
        return validateDiscordAuthorization(data.authorization_url, origin);
    }
    discordCallback = (input: DiscordCallbackInput): Promise<AdmissionResult> => this.authenticate('/api/v1/auth/discord/callback', input);
    admission2fa = (flow_token: string, code: string): Promise<AdmissionResult> => this.verify2fa(flow_token, code);
    async updatePassword(mode: 'set'|'change'|'reset', input: {password: string; old_password?: string; proof?: string}): Promise<void> {
        await this.request(`/api/v1/account/password/${mode}`, 'POST', input);
        await this.refresh();
    }
    logout(): Promise<void> {
        if (this.logoutFlight) return this.logoutFlight;
        const pending = [this.refreshFlight, this.loginFlight];
        this.clear(); this.publish({loggingOut: true});
        this.logoutFlight = (async () => {
            await Promise.allSettled(pending);
            // A pending login may have rotated the cookie. Read its current CSRF,
            // without restoring that user's identity in the logged-out view.
            const current = await this.raw<SessionResponse>('/api/v1/session/bootstrap');
            const result = await this.raw<SessionResponse>('/api/v1/session/logout','POST',{},new Headers({'X-CSRF-Token':current.csrf_token}));
            this.csrf = result.csrf_token;
            this.publish({notice:''});
        })().catch(e => { this.publish({notice:'服务端退出未确认，请重试。'+errorText(e)}); throw e; })
            .finally(() => { this.publish({loggingOut:false}); this.logoutFlight=null; });
        return this.logoutFlight;
    }
    async request<T>(path: string, method = 'GET', body?: unknown, gameHeaders?: Record<string,string>, beforeSend?:()=>boolean): Promise<T> {
        if (!this.snapshot.user || this.snapshot.loggingOut) throw new ApiError('请先登录。',401);
        const epoch = this.epoch, headers = this.writeHeaders();
        for (const [name,value] of Object.entries(gameHeaders || {})) headers.set(name === 'X-CSRF-Token' ? 'X-Game-CSRF-Token' : name,value);
        if (beforeSend && !beforeSend()) throw new ApiError('请求上下文已失效。',0,'REQUEST_SCOPE_EXPIRED');
        try {
            const data = await this.raw<T>(path,method,body,headers);
            this.check(epoch);
            return data;
        } catch(e) {
            if (epoch === this.epoch && e instanceof ApiError && e.status === 401) this.clear('登录已过期，请重新登录。');
            throw e;
        }
    }
    async loadSelf() { const user=await this.request<User>('/api/user/self'); if (user.id !== this.snapshot.user?.id) { this.clear('会话已改变，请重新读取。'); await this.refresh(); return this.snapshot.user!; } this.publish({user}); return user; }
    async page<T>(path: string): Promise<Page<T>> { const p = await this.request<Page<T>>(path); if (!p || !Array.isArray(p.items) || !Number.isFinite(p.total) || !Number.isFinite(p.page_size))
        throw new ApiError('列表响应格式异常，请重新加载。'); return p; }
    async groups(): Promise<Record<string, { ratio: number | string; desc: string }>> {
        const data = await this.request<Record<string, { ratio: number | string; desc: string }>>('/api/user/self/groups');
        if (!data || typeof data !== 'object' || Array.isArray(data) || Object.values(data).some(g => !g || typeof g.desc !== 'string' || !['number', 'string'].includes(typeof g.ratio))) throw new ApiError('分组响应格式异常，请重新加载。');
        return data;
    }
    keys = (page = 1, size = 10) => this.page<Key>(`/api/token/?p=${page}&page_size=${size}`);
    logs = (query = 'p=1&page_size=10') => this.page<UsageLog>(`/api/log/self?${query}`);
}
export const api = new ApiClient();
