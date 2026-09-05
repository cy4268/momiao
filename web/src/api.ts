import { readChat, aborted, modelFailure, type ChatInput, type ChatResult } from './chat';
// Native contract: new-api f116414, controller/{user,auth_session,token,log}.go.
// Tokens and session identity deliberately live only in this instance's memory.
export interface User {
    id: number;
    username: string;
    display_name: string;
    role: number;
    group?: string;
    quota?: number;
    used_quota?: number;
    request_count?: number;
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
interface Bundle {
    access_token: string;
    access_expires_at: number;
    user: User;
    session: {
        sid: string;
    };
}
export interface TwoFactor {
    require_2fa: true;
    flow_token: string;
}
interface Snapshot {
    user: User | null;
    ready: boolean;
    loggingOut: boolean;
    notice: string;
}
export class ApiError extends Error {
    constructor(message: string, public status = 0, public code = '', public uncertain = false) { super(message); this.name = 'ApiError'; }
}
const isAuthError = (e: unknown) => e instanceof ApiError && (e.status === 401 || ['AUTH_UNAUTHORIZED', 'AUTH_TOKEN_EXPIRED', 'AUTH_SESSION_REVOKED', 'AUTH_SESSION_MISMATCH', 'AUTH_USER_DISABLED', 'AUTH_USER_INVALID'].includes(e.code));
export const errorText = (e: unknown) => e instanceof Error ? e.message : '请求未完成，请稍后重试。';
function safeUser(u: User): User { if (!u || !Number.isInteger(u.id) || u.id <= 0 || typeof u.username !== 'string')
    throw new ApiError('账户响应格式异常，请重新登录。'); return { id: u.id, username: u.username, display_name: u.display_name || u.username, role: u.role, group: typeof u.group === 'string' ? u.group : undefined, quota: u.quota, used_quota: u.used_quota, request_count: u.request_count }; }
export class ApiClient {
    private streams = new Set<AbortController>();
    private token = '';
    private sid = '';
    private expires = 0;
    private epoch = 0;
    private snapshot: Snapshot = { user: null, ready: false, loggingOut: false, notice: '' };
    private listeners = new Set<() => void>();
    private refreshFlight: Promise<void> | null = null;
    private loginFlight: Promise<Bundle | TwoFactor> | null = null;
    private bootstrapFlight: Promise<void> | null = null;
    private logoutFlight: Promise<void> | null = null;
    constructor(private fetcher: (path: string, init?: RequestInit) => Promise<Response> = (...args) => fetch(...args)) { }
    getSnapshot = () => this.snapshot;
    subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn); }; };
    private publish(patch: Partial<Snapshot>) { this.snapshot = { ...this.snapshot, ...patch }; this.listeners.forEach(fn => fn()); }
    private clear(notice = '') { this.streams.forEach(c => c.abort()); this.streams.clear(); this.epoch++; this.token = ''; this.sid = ''; this.expires = 0; this.publish({ user: null, ready: true, notice }); }
    private accept(bundle: Bundle, epoch: number) { if (epoch !== this.epoch || this.snapshot.loggingOut)
        throw new ApiError('登录状态已改变，请重新登录。', 401); if (!bundle?.access_token || !bundle.session?.sid || !Number.isFinite(bundle.access_expires_at))
        throw new ApiError('登录响应格式异常，请重新登录。'); const user = safeUser(bundle.user); this.token = bundle.access_token; this.sid = bundle.session.sid; this.expires = bundle.access_expires_at; this.publish({ user, ready: true, notice: '' }); }
    private headers() { const headers = new Headers(); if (this.token)
        headers.set('Authorization', `Bearer ${this.token}`); if (this.snapshot.user)
        headers.set('New-Api-User', String(this.snapshot.user.id)); if (this.sid)
        headers.set('X-Auth-Session', this.sid); return headers; }
    private async raw<T>(path: string, method = 'GET', body?: unknown, headers = new Headers()): Promise<T> {
        headers.set('Accept', 'application/json');
        if (body !== undefined)
            headers.set('Content-Type', 'application/json');
        let response: Response;
        try {
            response = await this.fetcher(path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body), credentials: 'same-origin', cache: 'no-store', signal: AbortSignal.timeout(25000) });
        }
        catch {
            throw new ApiError(method === 'GET' ? '网络连接中断，请检查连接后重试。' : '网络连接中断，操作结果尚未确认。请先刷新列表核对，勿重复提交。', 0, '', method !== 'GET');
        }
        let envelope: {
            success?: boolean;
            message?: string;
            code?: string;
            data: T;
        };
        try {
            envelope = await response.json();
        }
        catch {
            throw new ApiError(`服务响应格式异常（HTTP ${response.status}）。写入结果尚未确认时，请先刷新列表核对。`, response.status, '', method !== 'GET');
        }
        if (!response.ok || envelope?.success !== true)
            throw new ApiError(typeof envelope?.message === 'string' && envelope.message ? envelope.message : `请求未完成（HTTP ${response.status}）。`, response.status, envelope?.code, method !== 'GET' && response.status >= 500);
        return envelope.data;
    }
    refresh(): Promise<void> {
        if (this.snapshot.loggingOut)
            return Promise.reject(new ApiError('正在退出登录。', 401));
        if (this.refreshFlight)
            return this.refreshFlight;
        const epoch = this.epoch;
        this.refreshFlight = this.raw<Bundle>('/api/user/auth/refresh', 'POST', undefined, this.headers()).then(b => this.accept(b, epoch)).catch(e => { if (epoch === this.epoch && isAuthError(e))
            this.clear(e instanceof ApiError && e.code === 'AUTH_SESSION_MISMATCH' ? '浏览器会话已变更，本页登录已失效，请重新登录。' : '登录已过期，请重新登录。'); throw e; }).finally(() => { this.refreshFlight = null; });
        return this.refreshFlight;
    }
    bootstrap(): Promise<void> {
        if (this.bootstrapFlight)
            return this.bootstrapFlight;
        if (this.snapshot.ready)
            return Promise.resolve();
        this.bootstrapFlight = (async () => { try {
            await this.refresh();
            await this.loadSelf();
        }
        catch (e) {
            if (!this.snapshot.loggingOut)
                this.publish({ ready: true, notice: isAuthError(e) ? '' : errorText(e) });
        }
        finally {
            this.publish({ ready: true });
        } })();
        return this.bootstrapFlight;
    }
    private async authenticate(path: string, body: unknown): Promise<TwoFactor | void> {
        if (this.snapshot.loggingOut || this.loginFlight)
            throw new ApiError('登录请求处理中，请稍候。');
        const epoch = this.epoch;
        this.loginFlight = this.raw<Bundle | TwoFactor>(path, 'POST', body);
        try {
            const data = await this.loginFlight;
            if (epoch !== this.epoch)
                throw new ApiError('登录状态已改变。', 401);
            if ('require_2fa' in data && data.require_2fa) {
                if (!data.flow_token)
                    throw new ApiError('验证会话缺失，请重新登录。');
                return data;
            }
            this.accept(data as Bundle, epoch);
        }
        finally {
            this.loginFlight = null;
        }
    }
    login = (username: string, password: string) => this.authenticate('/api/user/login', { username, password });
    verify2fa = (flow_token: string, code: string) => this.authenticate('/api/user/login/2fa', { flow_token, code });
    logout(): Promise<void> {
        if (this.logoutFlight)
            return this.logoutFlight;
        const headers = this.headers();
        const pending = [this.refreshFlight, this.loginFlight];
        this.clear();
        this.publish({ loggingOut: true });
        // Wait for any Set-Cookie rotation before revoking the cookie. Never restore stale state.
        this.logoutFlight = (async () => { await Promise.allSettled(pending); try {
            await this.raw('/api/user/auth/logout', 'POST', undefined, headers);
        }
        catch (e) {
            this.publish({ notice: `本页已退出；服务端退出未确认。请重试退出后再离开。${errorText(e)}` });
            throw e;
        }
        finally {
            this.publish({ loggingOut: false });
            this.logoutFlight = null;
        } })();
        return this.logoutFlight;
    }
    async request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
        if (!this.snapshot.user || this.snapshot.loggingOut)
            throw new ApiError('请先登录。', 401);
        const epoch = this.epoch;
        if (this.expires <= Date.now() / 1000 + 15)
            await this.refresh();
        const check = () => { if (epoch !== this.epoch || !this.snapshot.user || this.snapshot.loggingOut)
            throw new ApiError('登录状态已改变，请重新登录。', 401); };
        check();
        const sentToken = this.token;
        try {
            const data = await this.raw<T>(path, method, body, this.headers());
            check();
            return data;
        }
        catch (e) {
            check();
            if (e instanceof ApiError && e.status === 401 && method === 'GET') {
                try {
                    if (this.token === sentToken)
                        await this.refresh();
                    check();
                    const data = await this.raw<T>(path, method, body, this.headers());
                    check();
                    return data;
                }
                catch (next) {
                    if (isAuthError(next) && epoch === this.epoch)
                        this.clear('登录已过期，请重新登录。');
                    throw next;
                }
            }
            if (isAuthError(e) && epoch === this.epoch)
                this.clear('登录已过期或访问被拒绝，请重新登录。');
            throw e;
        }
    }
    async loadSelf() { const epoch = this.epoch; const user = safeUser(await this.request<User>('/api/user/self')); if (epoch !== this.epoch || this.snapshot.loggingOut)
        throw new ApiError('登录状态已改变。', 401); this.publish({ user }); return user; }
    async page<T>(path: string): Promise<Page<T>> { const p = await this.request<Page<T>>(path); if (!p || !Array.isArray(p.items) || !Number.isFinite(p.total) || !Number.isFinite(p.page_size))
        throw new ApiError('列表响应格式异常，请重新加载。'); return p; }
    async groups(): Promise<Record<string, { ratio: number | string; desc: string }>> {
        const data = await this.request<Record<string, { ratio: number | string; desc: string }>>('/api/user/self/groups');
        if (!data || typeof data !== 'object' || Array.isArray(data) || Object.values(data).some(g => !g || typeof g.desc !== 'string' || !['number', 'string'].includes(typeof g.ratio))) throw new ApiError('分组响应格式异常，请重新加载。');
        return data;
    }
    async models(group: string): Promise<string[]> {
        const data = await this.request<string[] | null>(`/api/user/models?group=${encodeURIComponent(group)}`);
        if (data === null) return [];
        if (!Array.isArray(data) || data.some(m => typeof m !== 'string' || !m)) throw new ApiError('模型列表响应格式异常，请重新加载。');
        return [...new Set(data)].sort();
    }
    async playground(input: ChatInput, signal: AbortSignal, onUpdate: (r: ChatResult) => void): Promise<ChatResult> {
        if (!input.model || !input.group || !input.prompt.trim() || input.prompt.length > 16000 || !Number.isInteger(input.maxTokens) || input.maxTokens < 1 || input.maxTokens > 4096) throw new ApiError('请选择模型与分组，输入 1–16000 字提示词及 1–4096 的整数输出预算。');
        if (!this.snapshot.user || this.snapshot.loggingOut) throw new ApiError('请先登录。', 401);
        const epoch = this.epoch;
        const controller = new AbortController();
        const stop = () => controller.abort();
        signal.addEventListener('abort', stop, { once: true });
        this.streams.add(controller);
        let timedOut = false;
        const timer = setTimeout(() => { timedOut = true; controller.abort(); }, 300000);
        const check = () => { if (signal.aborted || controller.signal.aborted || epoch !== this.epoch) throw aborted(); };
        try {
            check();
            if (this.expires <= Date.now() / 1000 + 15) await this.refresh();
            check();
            const headers = this.headers(); headers.set('Content-Type', 'application/json'); headers.set('Accept', 'text/event-stream, application/json');
            const response = await this.fetcher('/pg/chat/completions', { method: 'POST', headers, credentials: 'same-origin', cache: 'no-store', signal: controller.signal, body: JSON.stringify({ model: input.model, group: input.group, messages: [{ role: 'user', content: input.prompt }], stream: true, max_tokens: input.maxTokens }) });
            check();
            if (response.status === 401) { void response.body?.cancel().catch(() => {}); this.clear('登录已过期，请重新登录。'); throw modelFailure(401); }
            return await readChat(response, controller.signal, r => { check(); onUpdate(r); });
        } catch (e) {
            if (e instanceof ApiError && e.status === 401) throw e;
            if (timedOut) throw new ApiError('请求已达 5 分钟上限，响应可能不完整。', 0, 'STREAM_TIMEOUT');
            if (controller.signal.aborted || signal.aborted || epoch !== this.epoch) throw aborted();
            if (e instanceof ApiError) throw e;
            throw modelFailure();
        } finally { clearTimeout(timer); signal.removeEventListener('abort', stop); this.streams.delete(controller); }
    }
    keys = (page = 1, size = 10) => this.page<Key>(`/api/token/?p=${page}&page_size=${size}`);
    logs = (query = 'p=1&page_size=10') => this.page<UsageLog>(`/api/log/self?${query}`);
}
export const api = new ApiClient();
