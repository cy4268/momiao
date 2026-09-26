import { serializeCatalogQuery, type CatalogQuery } from './game-catalog-query';
import { validateDiscordAuthorization, type AdmissionConfig, type AdmissionResult, type DiscordCallbackInput, type DiscordPurpose, type SensitiveProof } from './admission-api';
import { opaqueLogoutConfirmed, parseOpaqueBootstrap, parseOpaqueLogin, parseOpaqueLogout, parseOpaqueDiscordStart, parseOpaqueAdmission, type OpaqueBootstrap, type OpaquePasswordProvider } from './opaque-session';
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
export type SessionMode = 'native' | 'opaque';
export interface ApiClientOptions { sessionMode?: SessionMode }
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
    expires_at?: string;
}
export interface SessionCapabilities {
    authenticationState: string;
    password?: OpaquePasswordProvider;
    unavailable: readonly string[];
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
const isAuthError = (e: unknown) => e instanceof ApiError && (e.status === 401 || ['AUTH_UNAUTHORIZED', 'SESSION_UNAUTHORIZED', 'AUTH_TOKEN_EXPIRED', 'AUTH_SESSION_REVOKED', 'AUTH_SESSION_MISMATCH', 'AUTH_USER_DISABLED', 'AUTH_USER_INVALID'].includes(e.code));
const isSessionCSRFFenceError = (e: unknown) => e instanceof ApiError && e.code === 'SESSION_CSRF_FAILED';
export const errorText = (e: unknown) => e instanceof Error ? e.message : '请求未完成，请稍后重试。';
function safeUser(u: User): User { if (!u || !Number.isSafeInteger(u.id) || u.id <= 0 || typeof u.username !== 'string')
    throw new ApiError('账户响应格式异常，请重新登录。'); return { id: u.id, username: u.username, display_name: u.display_name || u.username, role: u.role, group: typeof u.group === 'string' ? u.group : undefined, quota: u.quota, used_quota: u.used_quota, request_count: u.request_count }; }
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
    private reads = new Set<AbortController>();
    private token = '';
    private sid = '';
    private expires = 0;
    private epoch = 0;
    private snapshot: Snapshot = { user: null, ready: false, loggingOut: false, notice: '' };
    private listeners = new Set<() => void>();
    private refreshFlight: Promise<void> | null = null;
    private loginFlight: Promise<TwoFactor | void> | null = null;
    private admissionFlight: Promise<unknown> | null = null;
    private bootstrapFlight: Promise<void> | null = null;
    private logoutFlight: Promise<void> | null = null;
    private readonly sessionMode: SessionMode;
    private opaqueCSRF = '';
    private opaqueIdentity = '';
    private opaqueChainID = '';
    private opaqueState = 'UNINITIALIZED';
    private opaquePassword?: OpaquePasswordProvider;
    private opaqueUnavailable = new Set<string>();
    private opaqueChallenge?: { id: string; expiresAt: number };
    private opaqueMutationBlocked = false;
    constructor(private fetcher: (path: string, init?: RequestInit) => Promise<Response> = (...args) => fetch(...args), options: ApiClientOptions = {}) { this.sessionMode = options.sessionMode ?? 'native'; }
    getSessionMode = () => this.sessionMode;
    getSessionCapabilities = (): SessionCapabilities => ({ authenticationState: this.sessionMode === 'opaque' ? this.opaqueState : 'NATIVE', password: this.opaquePassword, unavailable: [...this.opaqueUnavailable] });
    sessionFeatureUnavailable = (feature: string) => this.sessionMode === 'opaque' && this.opaqueUnavailable.has(feature);
    getSnapshot = () => this.snapshot;
    // Non-sensitive identity boundary; ordinary token refresh keeps drafts intact.
    getSessionGeneration = () => this.epoch;
    subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn); }; };
    private publish(patch: Partial<Snapshot>) { this.snapshot = { ...this.snapshot, ...patch }; this.listeners.forEach(fn => fn()); }
    private advanceSessionBoundary() { this.epoch++; this.reads.forEach(c => c.abort()); this.reads.clear(); }
    private clear(notice = '') { this.advanceSessionBoundary(); this.token = ''; this.sid = ''; this.expires = 0; this.publish({ user: null, ready: true, notice }); }
    private clearOpaqueFence(notice: string) { this.opaqueCSRF = ''; this.opaqueIdentity = ''; this.opaqueChainID = ''; this.opaqueState = 'UNINITIALIZED'; this.opaqueChallenge = undefined; this.opaqueMutationBlocked = false; this.clear(notice); }
    private accept(bundle: Bundle, epoch: number) { if (epoch !== this.epoch || this.snapshot.loggingOut)
        throw new ApiError('登录状态已改变，请重新登录。', 401); if (!bundle?.access_token || !bundle.session?.sid || !Number.isFinite(bundle.access_expires_at))
        throw new ApiError('登录响应格式异常，请重新登录。'); const user = safeUser(bundle.user); if (this.sid !== bundle.session.sid || this.snapshot.user?.id !== user.id) this.epoch++; this.token = bundle.access_token; this.sid = bundle.session.sid; this.expires = bundle.access_expires_at; this.publish({ user, ready: true, notice: '' }); }
    private headers() { const headers = new Headers(); if (this.sessionMode === 'native' && this.token)
        headers.set('Authorization', `Bearer ${this.token}`); if (this.snapshot.user)
        if (this.sessionMode === 'native') headers.set('New-Api-User', String(this.snapshot.user.id)); if (this.sessionMode === 'native' && this.sid)
        headers.set('X-Auth-Session', this.sid); return headers; }
    private opaqueWriteHeaders() { const headers = this.headers(); if (!this.opaqueCSRF) throw new ApiError('不透明会话缺少 CSRF 状态，请刷新页面重新核对。', 403, 'AUTH_CSRF_MISSING'); headers.set('X-CSRF-Token', this.opaqueCSRF); return headers; }
    private protectedHeaders(path: string, method: string, gameHeaders?: Record<string,string>) {
        const headers = this.headers(), game = /^\/api\/v1\/(games|game-rounds|roulette)(?:\/|$)/.test(path);
        for (const [name, value] of Object.entries(gameHeaders || {})) {
            if (!game || !['Idempotency-Key', 'X-CSRF-Token', 'X-Fairness-Commitment'].includes(name)) throw new ApiError('无效的游戏请求头。');
            headers.set(this.sessionMode === 'opaque' && name === 'X-CSRF-Token' ? 'X-Game-CSRF-Token' : name, value);
        }
        if (this.sessionMode === 'opaque') {
            if (!this.opaqueCSRF) throw new ApiError('不透明会话缺少 CSRF 状态，请刷新页面重新核对。', 403, 'AUTH_CSRF_MISSING');
            headers.set('X-CSRF-Token', this.opaqueCSRF);
        }
        return headers;
    }
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
    private opaqueProtocol<T>(read: () => T, message: string): T { try { return read(); } catch { throw new ApiError(message, 0, 'AUTH_PROTOCOL_INVALID'); } }
    private rememberOpaque(session: OpaqueBootstrap) {
        this.opaqueCSRF = session.csrfToken;
        this.opaqueState = session.authenticationState;
        this.opaquePassword = session.password;
        this.opaqueUnavailable = new Set(session.unavailable);
        this.expires = session.expiresAt;
    }
    private opaqueStateNotice(state: string, recoverableChallenge = false) {
        if (state === 'DISCORD_CALLBACK') return 'Discord 授权尚待回调；取消后可重新开始。';
        if (state === 'DISCORD_TWO_FA') return recoverableChallenge ? '请完成 Discord 登录的二次验证。' : '此页面没有原二次验证凭据；请先取消本次授权，再重新开始。';
        if (state === 'AUTHENTICATION_PENDING') return '登录结果仍在处理，请先重新核对会话，勿重复提交。';
        if (state === 'TWO_FA') return recoverableChallenge ? '二次验证结果尚未确认；已恢复待验证状态，页面不会自动重放。' : '二次验证仍待完成，但本页没有可重放的验证凭据；请退出清理后重新登录。';
        if (state === 'UNKNOWN') return '登录结果尚未确认，已暂停再次提交；请先退出清理并核对账户。';
        if (state === 'CANCELLED') return '本次登录已取消，请退出清理后刷新页面重新开始。';
        return '';
    }
    private async readOpaqueSession(epoch: number, mutationAccepted = false): Promise<void> {
        const payload = await this.raw<unknown>('/api/v1/session/bootstrap');
        const session = this.opaqueProtocol(() => parseOpaqueBootstrap(payload), '不透明会话引导响应格式异常。');
        if (epoch !== this.epoch || this.snapshot.loggingOut) throw new ApiError('登录状态已改变。', 401);
        this.rememberOpaque(session);
        const hadBoundary = !!(this.snapshot.user || this.opaqueIdentity || this.opaqueChainID);
        if (session.authenticationState !== 'AUTHENTICATED') {
            if (hadBoundary) this.advanceSessionBoundary();
            const challenge = ['TWO_FA','DISCORD_TWO_FA'].includes(session.authenticationState) && this.opaqueChallenge && this.opaqueChallenge.expiresAt > Date.now() / 1000 ? this.opaqueChallenge : undefined;
            this.token = ''; this.sid = ''; this.opaqueIdentity = ''; this.opaqueChainID = ''; this.opaqueChallenge = challenge;
            this.opaqueMutationBlocked = !['ANONYMOUS'].includes(session.authenticationState);
            this.publish({ user: null, ready: true, notice: this.opaqueStateNotice(session.authenticationState, !!challenge) });
            return;
        }
        const nextChainID = session.authChainID || '';
        const boundaryChanged = this.opaqueIdentity !== session.identityID || this.opaqueChainID !== nextChainID || String(this.snapshot.user?.id || '') !== session.identityID;
        const advancedBeforeSelf = hadBoundary && boundaryChanged;
        if (advancedBeforeSelf) { this.advanceSessionBoundary(); this.publish({ user: null, notice: '' }); }
        const verificationEpoch = this.epoch;
        let projected: User;
        try {
            const selfHeaders = new Headers(); selfHeaders.set('X-CSRF-Token', session.csrfToken);
            projected = safeUser(await this.raw<User>('/api/user/self', 'GET', undefined, selfHeaders));
            if (verificationEpoch !== this.epoch || this.snapshot.loggingOut) throw new ApiError('登录状态已改变。', 401);
            if (session.identityID !== String(projected.id)) throw new ApiError('不透明会话身份与账户投影不一致；本页未采用该身份。', 0, 'AUTH_IDENTITY_MISMATCH', mutationAccepted);
        } catch (error) {
            if (advancedBeforeSelf && verificationEpoch === this.epoch && !this.snapshot.loggingOut) this.clearOpaqueFence('浏览器会话已变更，但新身份核对失败；本页旧登录已失效。');
            throw error;
        }
        if (boundaryChanged && !advancedBeforeSelf) this.advanceSessionBoundary();
        this.opaqueIdentity = session.identityID; this.opaqueChainID = nextChainID; this.opaqueChallenge = undefined; this.opaqueMutationBlocked = false;
        this.publish({ user: projected, ready: true, notice: '' });
    }
    private opaqueBootstrapNotice(error: unknown) {
        if (error instanceof ApiError && error.code === 'AUTH_INPUT_INVALID') return '检测到原生刷新 Cookie 与不透明会话模式冲突；请在隔离的浏览器配置中完成迁移后重试。本页未自动清除现有原生会话。';
        if (error instanceof ApiError && error.code === 'AUTH_RESULT_UNKNOWN') return '不透明会话结果尚未确认；请勿重复提交，先刷新页面核对或退出清理。';
        if (isSessionCSRFFenceError(error)) return '浏览器会话已在其他页面变更；本页旧登录已失效，请重新核对。';
        return errorText(error);
    }
    refresh(): Promise<void> {
        if (this.snapshot.loggingOut)
            return Promise.reject(new ApiError('正在退出登录。', 401));
        if (this.refreshFlight)
            return this.refreshFlight;
        const epoch = this.epoch;
        this.refreshFlight = (this.sessionMode === 'opaque'
            ? this.readOpaqueSession(epoch)
            : this.raw<Bundle>('/api/user/auth/refresh', 'POST', undefined, this.headers()).then(b => this.accept(b, epoch)))
            .catch(e => { if (epoch === this.epoch && (isAuthError(e) || isSessionCSRFFenceError(e))) {
                const notice = isSessionCSRFFenceError(e) || e instanceof ApiError && e.code === 'AUTH_SESSION_MISMATCH' ? '浏览器会话已变更，本页登录已失效，请重新登录。' : '登录已过期，请重新登录。';
                if (this.sessionMode === 'opaque') this.clearOpaqueFence(notice); else this.clear(notice);
            } throw e; })
            .finally(() => { this.refreshFlight = null; });
        return this.refreshFlight;
    }
    bootstrap(): Promise<void> {
        if (this.bootstrapFlight)
            return this.bootstrapFlight;
        if (this.snapshot.ready)
            return Promise.resolve();
        const flight = (async () => { try {
            await this.refresh();
            if (this.sessionMode === 'native') await this.loadSelf();
        }
        catch (e) {
            if (!this.snapshot.loggingOut)
                this.publish({ ready: true, notice: this.sessionMode === 'opaque' ? this.opaqueBootstrapNotice(e) : isAuthError(e) ? '' : errorText(e) });
        }
        finally {
            this.publish({ ready: true });
        } })();
        const tracked = flight.finally(() => { if (this.bootstrapFlight === tracked) this.bootstrapFlight = null; });
        this.bootstrapFlight = tracked;
        return tracked;
    }
    private nativeAuthenticate(path: string, body: unknown): Promise<TwoFactor | void> {
        if (this.snapshot.loggingOut || this.loginFlight || this.admissionFlight)
            return Promise.reject(new ApiError('登录请求处理中，请稍候。'));
        const epoch = this.epoch;
        const flight = (async () => {
            const data = await this.raw<Bundle | TwoFactor>(path, 'POST', body);
            if (epoch !== this.epoch)
                throw new ApiError('登录状态已改变。', 401);
            if ('require_2fa' in data && data.require_2fa) {
                if (!data.flow_token)
                    throw new ApiError('验证会话缺失，请重新登录。');
                return data;
            }
            this.accept(data as Bundle, epoch);
        })();
        this.loginFlight = flight;
        return flight.finally(() => { if (this.loginFlight === flight) this.loginFlight = null; });
    }
    private opaqueAcceptedFailure(error: unknown): ApiError {
        const known = error instanceof ApiError ? error : new ApiError('不透明会话响应格式异常。', 0, 'AUTH_PROTOCOL_INVALID');
        return known.uncertain ? known : new ApiError(known.message, known.status, known.code, true);
    }
    private async recoverOpaqueUncertain(epoch: number, original: ApiError, notice: string): Promise<void> {
        if (epoch !== this.epoch || this.snapshot.loggingOut) throw new ApiError('登录状态已改变。', 401);
        try {
            await this.readOpaqueSession(epoch, true);
        } catch (recovery) {
            if ((epoch !== this.epoch && !(this.opaqueState === 'AUTHENTICATED' && this.snapshot.user)) || this.snapshot.loggingOut) throw new ApiError('登录状态已改变。', 401);
            this.token = ''; this.sid = ''; this.opaqueIdentity = ''; this.opaqueChallenge = undefined; this.opaqueMutationBlocked = true; this.opaqueState = 'UNKNOWN';
            this.publish({ user: null, ready: true, notice });
            const failed = recovery instanceof ApiError && recovery.code === 'AUTH_IDENTITY_MISMATCH' ? recovery : original;
            throw this.opaqueAcceptedFailure(failed);
        }
        if (this.snapshot.loggingOut) throw new ApiError('登录状态已改变。', 401);
        if (this.opaqueState === 'AUTHENTICATED' && this.snapshot.user) return;
        throw this.opaqueAcceptedFailure(original);
    }
    private opaqueAuthentication(work: (epoch: number) => Promise<TwoFactor | void>): Promise<TwoFactor | void> {
        if (this.snapshot.loggingOut || this.loginFlight || this.admissionFlight) throw new ApiError('登录请求处理中，请稍候。');
        const epoch = this.epoch, flight = work(epoch);
        this.loginFlight = flight;
        return flight.finally(() => { if (this.loginFlight === flight) this.loginFlight = null; });
    }
    private opaqueLogin(identifier: string, password: string, turnstileToken: string): Promise<TwoFactor | void> {
        if (this.opaqueMutationBlocked) return Promise.reject(new ApiError('上一笔登录结果尚未确认，已暂停再次提交。', 0, 'AUTH_RESULT_UNKNOWN', true));
        if (this.opaqueState !== 'ANONYMOUS' || !this.opaqueCSRF) return Promise.reject(new ApiError('当前不透明会话尚未准备好密码登录。', 409, 'AUTH_CONFLICT'));
        if (this.opaquePassword?.state !== 'AVAILABLE') return Promise.reject(new ApiError('当前密码登录能力不可用。', 503, 'AUTH_UNAVAILABLE'));
        if (this.opaquePassword.turnstileRequired && !turnstileToken) return Promise.reject(new ApiError('本次登录需要 Turnstile 凭据。', 400, 'AUTH_TURNSTILE_REQUIRED'));
        return this.opaqueAuthentication(async epoch => {
            let accepted = false, responseAccepted = false;
            try {
                const body = { identifier, password, ...(turnstileToken ? { turnstile_token: turnstileToken } : {}) };
                const payload = await this.raw<unknown>('/api/v1/auth/password/login', 'POST', body, this.opaqueWriteHeaders());
                responseAccepted = true;
                const result = this.opaqueProtocol(() => parseOpaqueLogin(payload), '不透明密码登录响应格式异常。');
                this.opaqueCSRF = result.csrfToken;
                if (epoch !== this.epoch || this.snapshot.loggingOut) throw new ApiError('登录状态已改变。', 401);
                if (result.authenticationState === 'TWO_FA') {
                    this.opaqueState = 'TWO_FA'; this.opaqueChallenge = { id: result.challengeID, expiresAt: result.expiresAtSeconds };
                    this.publish({ notice: '' });
                    return { require_2fa: true, flow_token: result.challengeID, expires_at: result.expiresAt };
                }
                accepted = true;
                await this.readOpaqueSession(epoch, true);
                if (this.opaqueState !== 'AUTHENTICATED' || !this.snapshot.user) throw new ApiError('密码登录后的会话状态尚未确认。', 0, 'AUTH_RESULT_UNKNOWN', true);
            } catch (error) {
                const failed = error instanceof ApiError ? error : new ApiError('不透明密码登录响应格式异常。', 0, 'AUTH_PROTOCOL_INVALID');
                if (!accepted && (responseAccepted || failed.uncertain)) return this.recoverOpaqueUncertain(epoch, this.opaqueAcceptedFailure(failed), '登录结果尚未确认，已暂停再次提交；请先退出清理并核对账户。');
                if (accepted) {
                    this.opaqueMutationBlocked = true; this.opaqueChallenge = undefined;
                    if (!this.snapshot.loggingOut) { this.opaqueState = 'UNKNOWN'; this.publish({ user: null, notice: '登录结果尚未确认，已暂停再次提交；请先退出清理并核对账户。' }); }
                    throw this.opaqueAcceptedFailure(failed);
                }
                throw failed;
            }
        });
    }
    private opaqueVerify2FA(challengeID: string, code: string): Promise<TwoFactor | void> {
        const challenge = this.opaqueChallenge;
        if (!challenge || this.opaqueState !== 'TWO_FA' || challenge.id !== challengeID || challenge.expiresAt <= Date.now() / 1000) return Promise.reject(new ApiError('二次验证凭据已失效或已使用，请重新登录。', 409, 'AUTH_CHALLENGE_INVALID'));
        return this.opaqueAuthentication(async epoch => {
            let accepted = false, responseAccepted = false;
            try {
                const payload = await this.raw<unknown>('/api/v1/auth/password/2fa', 'POST', { challenge_id: challengeID, code }, this.opaqueWriteHeaders());
                responseAccepted = true;
                const result = this.opaqueProtocol(() => parseOpaqueLogin(payload), '不透明二次验证响应格式异常。');
                this.opaqueCSRF = result.csrfToken;
                if (epoch !== this.epoch || this.snapshot.loggingOut) throw new ApiError('登录状态已改变。', 401);
                if (result.authenticationState !== 'AUTHENTICATED') throw new ApiError('二次验证未返回已认证状态。', 0, 'AUTH_PROTOCOL_INVALID');
                accepted = true; this.opaqueChallenge = undefined;
                await this.readOpaqueSession(epoch, true);
                if (this.opaqueState !== 'AUTHENTICATED' || !this.snapshot.user) throw new ApiError('二次验证后的会话状态尚未确认。', 0, 'AUTH_RESULT_UNKNOWN', true);
            } catch (error) {
                const failed = error instanceof ApiError ? error : new ApiError('不透明二次验证响应格式异常。', 0, 'AUTH_PROTOCOL_INVALID');
                if (!accepted && (responseAccepted || failed.uncertain)) return this.recoverOpaqueUncertain(epoch, this.opaqueAcceptedFailure(failed), '二次验证结果尚未确认，已暂停再次提交；请先退出清理并核对账户。');
                if (accepted) {
                    this.opaqueMutationBlocked = true; this.opaqueChallenge = undefined;
                    if (!this.snapshot.loggingOut) { this.opaqueState = 'UNKNOWN'; this.publish({ user: null, notice: '二次验证结果尚未确认，已暂停再次提交；请先退出清理并核对账户。' }); }
                    throw this.opaqueAcceptedFailure(failed);
                }
                throw failed;
            }
        });
    }
    login = (username: string, password: string, turnstileToken = '') => this.sessionMode === 'opaque' ? this.opaqueLogin(username, password, turnstileToken) : this.nativeAuthenticate('/api/user/login', { username, password });
    verify2fa = (flow_token: string, code: string) => this.sessionMode === 'opaque' ? this.opaqueVerify2FA(flow_token, code) : this.nativeAuthenticate('/api/user/login/2fa', { flow_token, code });
    async admissionConfig(): Promise<AdmissionConfig> {
        const data = await this.raw<AdmissionConfig>('/platform/v1/admission/config');
        if (!data || typeof data.enabled !== 'boolean' || typeof data.registration_enabled !== 'boolean' || typeof data.eligibility !== 'string') throw new ApiError('账户服务配置暂时无法读取。');
        return data;
    }
    private admissionOperation<T>(work: (epoch: number) => Promise<T>): Promise<T> {
        if (this.snapshot.loggingOut || this.loginFlight || this.admissionFlight) return Promise.reject(new ApiError('账户请求处理中，请稍候。'));
        const epoch = this.epoch;
        const flight = work(epoch).finally(() => { if (this.admissionFlight === flight) this.admissionFlight = null; });
        this.admissionFlight = flight;
        return flight;
    }
    private checkAdmissionEpoch(epoch: number) {
        if (epoch !== this.epoch || this.snapshot.loggingOut) throw new ApiError('登录状态已改变，请重新开始。', 401);
    }
    private acceptAdmission(data: Bundle | TwoFactor | SensitiveProof, epoch: number): AdmissionResult {
        this.checkAdmissionEpoch(epoch);
        if (data && 'require_2fa' in data && data.require_2fa === true && typeof data.flow_token === 'string' && data.flow_token.length > 0) return { require_2fa:true, flow_token:data.flow_token };
        if (data && 'proof' in data && typeof data.proof === 'string' && data.proof && Number.isFinite(data.expires_at) && data.expires_at > Date.now()/1000) {
            if (!this.snapshot.user) throw new ApiError('请重新登录后验证账户。',401);
            return {proof:data.proof,expires_at:data.expires_at};
        }
        this.accept(data as Bundle, epoch);
    }
    private failOpaqueAdmission(error: unknown, epoch: number, accepted: boolean): never {
        const failed = error instanceof ApiError ? error : new ApiError('账户响应格式异常。', 0, 'AUTH_PROTOCOL_INVALID');
        if (epoch === this.epoch && !this.snapshot.loggingOut && (accepted || failed.uncertain || failed.code === 'AUTH_RESULT_UNKNOWN' || isAuthError(failed) || isSessionCSRFFenceError(failed))) {
            this.clearOpaqueFence('本次账户操作结果尚未确认，请重新核对会话；不要重复提交原操作。');
            this.opaqueState = 'UNKNOWN'; this.opaqueMutationBlocked = true;
        }
        throw accepted ? this.opaqueAcceptedFailure(failed) : failed;
    }
    private opaqueAdmissionCall(path: string, body: unknown): Promise<AdmissionResult> {
        const continuing = path === '/api/v1/auth/discord/callback' && this.opaqueState === 'DISCORD_CALLBACK' || path === '/api/v1/auth/discord/2fa' && this.opaqueState === 'DISCORD_TWO_FA';
        if (this.opaqueMutationBlocked && !continuing) return Promise.reject(new ApiError('请先核对或清理上一笔账户操作。', 409, 'AUTH_RESULT_UNKNOWN', true));
        return this.admissionOperation(async epoch => {
            let accepted = false;
            try {
                const payload = await this.raw<unknown>(path, 'POST', body, this.opaqueWriteHeaders());
                accepted = true;
                this.checkAdmissionEpoch(epoch);
                if (path.startsWith('/api/v1/account/password/') && (!payload || typeof payload !== 'object' || !('has_password' in payload) || payload.has_password !== true)) throw new ApiError('密码操作响应格式异常。',0,'AUTH_PROTOCOL_INVALID',true);
                const result = this.opaqueProtocol(() => parseOpaqueAdmission(payload), '账户操作响应格式异常。');
                if (path.startsWith('/api/v1/account/password/') && result.authenticationState !== 'AUTHENTICATED' || path === '/api/v1/auth/discord/2fa' && result.authenticationState === 'DISCORD_TWO_FA') throw new ApiError('账户操作返回了不匹配的状态。',0,'AUTH_PROTOCOL_INVALID',true);
                this.opaqueCSRF = result.csrfToken;
                if (result.authenticationState === 'DISCORD_TWO_FA') {
                    if (result.expiresAtSeconds <= Date.now()/1000) throw new ApiError('验证凭据已过期。', 409, 'AUTH_CHALLENGE_INVALID');
                    this.opaqueState = 'DISCORD_TWO_FA'; this.opaqueChallenge = { id: result.challengeID, expiresAt: result.expiresAtSeconds }; this.opaqueMutationBlocked = true;
                    this.publish({ notice: '' });
                    return { require_2fa: true, flow_token: result.challengeID, expires_at: result.expiresAt };
                }
                if (result.authenticationState === 'ACCOUNT_PROOF_READY') {
                    if (!this.snapshot.user || result.expiresAtSeconds <= Date.now()/1000) throw new ApiError('账户验证已失效，请重新登录。', 401);
                    this.opaqueState = 'AUTHENTICATED'; this.opaqueChallenge = undefined; this.opaqueMutationBlocked = false;
                    return { purpose: result.purpose, expires_at: result.expiresAtSeconds };
                }
                await this.readOpaqueSession(epoch, true);
                if (this.opaqueState !== 'AUTHENTICATED' || !this.snapshot.user) throw new ApiError('账户操作后的身份尚未确认。', 0, 'AUTH_RESULT_UNKNOWN', true);
            } catch (error) { return this.failOpaqueAdmission(error, epoch, accepted); }
        });
    }
    discordStart(purpose: DiscordPurpose, origin = window.location.origin): Promise<string> {
        if (this.sessionFeatureUnavailable('DISCORD_S2')) return Promise.reject(new ApiError('Discord 授权尚未接入不透明会话模式。', 503, 'DISCORD_S2'));
        if (this.sessionMode === 'opaque') {
            if (!['login','registration','fresh','password-reset'].includes(purpose) || this.opaqueMutationBlocked || !this.opaqueCSRF) return Promise.reject(new ApiError('请先核对当前会话，再开始授权。', 409, 'AUTH_CONFLICT'));
            const expectedState = purpose === 'login' || purpose === 'registration' ? 'ANONYMOUS' : 'AUTHENTICATED';
            if (this.opaqueState !== expectedState) return Promise.reject(new ApiError('当前会话不能开始这项授权，请先核对会话。',409,'AUTH_CONFLICT'));
            return this.admissionOperation(async epoch => {
                let accepted = false;
                try {
                    const payload = await this.raw<unknown>(`/api/v1/auth/discord/${purpose}/start`, 'POST', {}, this.opaqueWriteHeaders());
                    accepted = true; this.checkAdmissionEpoch(epoch);
                    const result = this.opaqueProtocol(() => parseOpaqueDiscordStart(payload), 'Discord 授权响应格式异常。');
                    const url = validateDiscordAuthorization(result.authorizationURL, origin);
                    this.opaqueCSRF = result.csrfToken; this.opaqueState = 'DISCORD_CALLBACK'; this.opaqueMutationBlocked = true;
                    this.publish({ notice: '' });
                    return url;
                } catch (error) { return this.failOpaqueAdmission(error, epoch, accepted); }
            });
        }
        return this.admissionOperation(async epoch => {
            if (!['login','registration','fresh','password-reset'].includes(purpose)) throw new ApiError('授权操作无效。');
            const path = `/api/momiao/auth/discord/${purpose}/start`;
            const data = purpose === 'fresh' || purpose === 'password-reset'
                ? await this.request<{authorization_url:string}>(path,'POST',{})
                : await this.raw<{authorization_url:string}>(path,'POST',{});
            this.checkAdmissionEpoch(epoch);
            return validateDiscordAuthorization(data?.authorization_url,origin);
        });
    }
    discordCallback(input: DiscordCallbackInput): Promise<AdmissionResult> {
        if (this.sessionFeatureUnavailable('DISCORD_S2')) return Promise.reject(new ApiError('Discord 授权回调尚未接入不透明会话模式。', 503, 'DISCORD_S2'));
        if (this.sessionMode === 'opaque') return this.opaqueAdmissionCall('/api/v1/auth/discord/callback', input);
        if ('error' in input) return Promise.reject(new ApiError('Discord 授权已取消。',403,'DISCORD_DENIED'));
        return this.admissionOperation(async epoch => {
            if (!input.code || !input.state || input.code.length + input.state.length > 8192) throw new ApiError('授权回调无效。');
            const query = new URLSearchParams({code:input.code,state:input.state});
            const data = await this.raw<Bundle | TwoFactor | SensitiveProof>(`/api/momiao/auth/discord/callback?${query}`, 'GET', undefined, this.headers());
            return this.acceptAdmission(data,epoch);
        });
    }
    admission2fa(flow_token: string, code: string): Promise<AdmissionResult> {
        if (this.sessionFeatureUnavailable('GENERAL_BFF_S2')) return Promise.reject(new ApiError('账户验证尚未接入不透明会话模式。', 503, 'GENERAL_BFF_S2'));
        if (this.sessionMode === 'opaque') {
            if (this.opaqueState !== 'DISCORD_TWO_FA' || this.opaqueChallenge?.id !== flow_token || this.opaqueChallenge.expiresAt <= Date.now()/1000) return Promise.reject(new ApiError('验证凭据已失效，请重新开始。', 409, 'AUTH_CHALLENGE_INVALID'));
            return this.opaqueAdmissionCall('/api/v1/auth/discord/2fa', { challenge_id: flow_token, code });
        }
        return this.admissionOperation(async epoch => this.acceptAdmission(await this.raw<Bundle | TwoFactor | SensitiveProof>('/api/momiao/auth/2fa','POST',{flow_token,code},this.headers()),epoch));
    }
    updatePassword(mode: 'set'|'change'|'reset', input: {password: string; old_password?: string; proof?: string}): Promise<void> {
        if (this.sessionFeatureUnavailable('GENERAL_BFF_S2')) return Promise.reject(new ApiError('密码变更尚未接入不透明会话模式。', 503, 'GENERAL_BFF_S2'));
        if (this.sessionMode === 'opaque') {
            if (!this.snapshot.user || !['set','change','reset'].includes(mode)) return Promise.reject(new ApiError('请先登录。',401));
            const body = mode === 'change' ? { password: input.password, old_password: input.old_password } : { password: input.password };
            return this.opaqueAdmissionCall(`/api/v1/account/password/${mode}`, body).then(result => { if (result !== undefined) throw new ApiError('密码操作结果需要重新核对。',0,'AUTH_PROTOCOL_INVALID',true); });
        }
        return this.admissionOperation(async epoch => {
            const user = this.snapshot.user;
            if (!user || !['set','change','reset'].includes(mode)) throw new ApiError('请先登录。',401);
            const body = mode === 'change' ? {password:input.password,old_password:input.old_password} : {password:input.password,proof:input.proof};
            const data = await this.request<Omit<Bundle,'user'> & {has_password:boolean}>(`/api/momiao/account/password/${mode}`,'POST',body);
            this.checkAdmissionEpoch(epoch);
            if (data.has_password !== true) throw new ApiError('密码操作结果需要重新核对。',0,'',true);
            this.accept({...data,user},epoch);
        });
    }
    logout(): Promise<void> {
        if (this.logoutFlight)
            return this.logoutFlight;
        const headers = this.headers();
        const pending = [this.refreshFlight, this.loginFlight, this.admissionFlight];
        this.clear();
        this.publish({ loggingOut: true });
        // Wait for any Set-Cookie rotation before revoking the cookie. Never restore stale state.
        this.logoutFlight = (async () => { await Promise.allSettled(pending); try {
            if (this.sessionMode === 'opaque') {
                // Pending mutations may have rotated Cookie/CSRF after the local
                // epoch was cleared. Read only the current cookie's fence without
                // accepting its identity back into this logged-out view.
                const fence = parseOpaqueBootstrap(await this.raw<unknown>('/api/v1/session/bootstrap'));
                this.opaqueCSRF = fence.csrfToken;
                const payload = await this.raw<unknown>('/api/v1/auth/logout', 'POST', {}, this.opaqueWriteHeaders());
                let result;
                try { result = parseOpaqueLogout(payload); } catch { throw new ApiError('退出响应格式异常，服务端结果尚未确认。', 0, 'AUTH_PROTOCOL_INVALID', true); }
                const notice = opaqueLogoutConfirmed(result)
                    ? result.platformSession === 'NOT_ISSUED'
                        ? '本次登录流程已清理；刷新页面后可重新开始。'
                        : result.pokerControl === 'REVOKED'
                        ? '平台、原生与 Poker 会话已确认退出；刷新页面后可建立新的登录会话。'
                        : '平台与原生会话已确认退出；Poker 退出控制仍为 UNAVAILABLE_S2，尚未联合验收。'
                    : `本页已退出；后端注销仍在处理（平台 ${result.platformSession} / 原生 ${result.nativeSession} / Poker ${result.pokerControl}）。请稍后重新核对。`;
                this.opaqueCSRF = ''; this.opaqueIdentity = ''; this.opaqueChainID = ''; this.opaqueState = 'UNINITIALIZED'; this.opaqueChallenge = undefined; this.opaqueMutationBlocked = false; this.expires = 0;
                this.publish({ notice });
            } else await this.raw('/api/user/auth/logout', 'POST', undefined, headers);
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
    async request<T>(path: string, method = 'GET', body?: unknown, gameHeaders?: Record<string,string>, beforeSend?:()=>boolean): Promise<T> {
        if (!this.snapshot.user || this.snapshot.loggingOut)
            throw new ApiError('请先登录。', 401);
        const epoch = this.epoch;
        if (this.expires <= Date.now() / 1000 + 15)
            await this.refresh();
        const check = () => { if (epoch !== this.epoch || !this.snapshot.user || this.snapshot.loggingOut)
            throw new ApiError('登录状态已改变，请重新登录。', 401); };
        check();
        const sentToken = this.token;
        const headers = () => this.protectedHeaders(path, method, gameHeaders);
        // Caller lifecycle may change while native refresh is pending; identity checks alone do not cover it.
        const send = () => { if (beforeSend && !beforeSend()) throw new ApiError('请求上下文已失效。', 0, 'REQUEST_SCOPE_EXPIRED'); return this.raw<T>(path, method, body, headers()); };
        try {
            const data = await send();
            check();
            return data;
        }
        catch (e) {
            check();
            if (this.sessionMode === 'opaque') {
                if ((isAuthError(e) || isSessionCSRFFenceError(e)) && epoch === this.epoch) this.clearOpaqueFence(isSessionCSRFFenceError(e) ? '浏览器会话已变更，本页旧登录已失效。' : '登录已过期或访问被拒绝，请重新登录。');
                throw e;
            }
            if (e instanceof ApiError && e.status === 401 && method === 'GET') {
                try {
                    if (this.token === sentToken)
                        await this.refresh();
                    check();
                    const data = await send();
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
    async exchangeOpsFresh(input:{operation_id:string;authz_epoch:string;challenge_id:string;proof:string}):Promise<void> {
        if(this.sessionMode!=='opaque')throw new ApiError('运营身份验证需要平台会话。',403,'OPS_FRESH_UNAVAILABLE');
        const epoch=this.epoch;
        try {
            const result=await this.request<{session:{newapi_user_id:string;csrf_token:string;absolute_expires_at:string;idle_expires_at:string;confirmation?:{operation_id:string}}}>('/api/v1/ops/fresh/exchange','POST',input);
            if(epoch!==this.epoch||this.snapshot.loggingOut)throw new ApiError('登录状态已改变。',401);
            const value=result?.session,absolute=Date.parse(value?.absolute_expires_at)/1000,idle=Date.parse(value?.idle_expires_at)/1000;
            if(!value||value.newapi_user_id!==this.opaqueIdentity||value.confirmation?.operation_id!==input.operation_id||!/^[A-Za-z0-9_-]{43}$/.test(value.csrf_token)||!Number.isFinite(absolute)||!Number.isFinite(idle)||Math.min(absolute,idle)<=Date.now()/1000)throw new ApiError('身份验证响应无法核对，请重新读取会话。',0,'AUTH_PROTOCOL_INVALID',true);
            this.opaqueCSRF=value.csrf_token;this.expires=Math.min(absolute,idle);
        } catch(error) {
            // Exchange may have rotated the cookie even if its response was
            // lost. Refresh only reads current state and never replays proof.
            if(epoch===this.epoch&&!this.snapshot.loggingOut&&error instanceof ApiError&&error.uncertain){try{await this.refresh();}catch{}}
            throw error;
        }
    }
    async loadSelf() { const epoch = this.epoch; const user = safeUser(await this.request<User>('/api/user/self')); if (epoch !== this.epoch || this.snapshot.loggingOut)
        throw new ApiError('登录状态已改变。', 401); if (this.sessionMode === 'opaque' && this.opaqueIdentity !== String(user.id)) { this.clearOpaqueFence('会话身份核对失败，请重新登录。'); throw new ApiError('不透明会话身份与账户投影不一致。', 401, 'AUTH_IDENTITY_MISMATCH'); } this.publish({ user }); return user; }
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
function configuredSessionMode(): SessionMode {
    const value = import.meta.env.VITE_MOMIAO_SESSION_MODE;
    if (value === undefined || value === '') return import.meta.env.PROD ? 'opaque' : 'native';
    if (value === 'native') return 'native';
    if (value === 'opaque') return 'opaque';
    throw new Error('VITE_MOMIAO_SESSION_MODE must be native or opaque');
}
export const api = new ApiClient(undefined, { sessionMode: configuredSessionMode() });
