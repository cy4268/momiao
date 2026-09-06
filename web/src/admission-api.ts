import { ApiError, type ApiClient, type TwoFactor } from './api';

export type DiscordPurpose = 'login' | 'registration' | 'fresh' | 'password-reset';
export interface DiscordCallbackInput { code: string; state: string }
export interface SensitiveProof { proof: string; expires_at: number }
export type AdmissionResult = TwoFactor | SensitiveProof | void;
export interface NativeAccount { id: number; username: string; has_password: boolean; discord_connected: boolean; two_fa_enabled: boolean }
export interface AdmissionConfig { enabled: boolean; registration_enabled: boolean; eligibility: string }
export interface AdmissionStatus { user_id:string; source:'UNVERIFIED'|'NEW_DISCORD_REGISTRATION'; grant_status:'PENDING_SOURCE'|'PENDING'|'RECOVERING'|'CONFIRMED'; amount_units:string; transaction_id:string|null; source_available:boolean }
export async function readAdmission(client: ApiClient):Promise<AdmissionStatus> {
    const d=await client.request<AdmissionStatus>('/platform/v1/admission');
    if(!d || d.user_id!==String(client.getSnapshot().user?.id) || !['UNVERIFIED','NEW_DISCORD_REGISTRATION'].includes(d.source) || !['PENDING_SOURCE','PENDING','RECOVERING','CONFIRMED'].includes(d.grant_status) || typeof d.source_available!=='boolean' || !['0','500000000'].includes(d.amount_units) || (d.transaction_id!==null && !/^[0-9a-f-]{36}$/.test(d.transaction_id))) throw new ApiError('注册状态暂时无法核对。');
    if(d.source==='UNVERIFIED' ? d.grant_status!=='PENDING_SOURCE'||d.amount_units!=='0'||d.transaction_id!==null : d.amount_units!=='500000000'||(d.grant_status==='CONFIRMED')!==(d.transaction_id!==null)) throw new ApiError('注册状态暂时无法核对。');
    return d;
}

export function captureDiscordCallback(location: Pick<Location,'pathname'|'search'|'hash'>, history: Pick<History,'replaceState'>): DiscordCallbackInput {
    const query = location.search;
    history.replaceState(null, '', '/oauth/discord');
    const p = new URLSearchParams(query);
    if (p.has('error')) throw new ApiError('Discord 授权未完成。可以返回登录或注册页面重新开始。', 403, 'DISCORD_DENIED');
    if (query.length > 8192 || [...p.keys()].some(k => !['code','state'].includes(k)) || p.getAll('code').length!==1 || p.getAll('state').length!==1 || !p.get('code') || !p.get('state')) throw new ApiError('授权回调无效或已失效，请重新开始。', 400, 'CALLBACK_INVALID');
    return {code:p.get('code')!, state:p.get('state')!};
}

export function validateDiscordAuthorization(value: unknown, origin: string): string {
    const fail = () => new ApiError('授权地址未通过验证，请稍后重新开始。');
    if (typeof value !== 'string' || value.length > 8192) throw fail();
    let u: URL;
    try { u = new URL(value); } catch { throw fail(); }
    const p = u.searchParams;
    if (u.origin !== 'https://discord.com' || u.pathname !== '/oauth2/authorize' || u.username || u.password || u.hash || p.get('redirect_uri') !== origin + '/oauth/discord' || p.get('response_type') !== 'code' || !/^[1-9][0-9]{16,19}$/.test(p.get('client_id') || '') || !p.get('state')) throw fail();
    if ([...p.keys()].some(k => !['client_id','redirect_uri','response_type','scope','state'].includes(k) || p.getAll(k).length!==1)) throw fail();
    const scopes = (p.get('scope') || '').split(' ');
    if (!scopes.includes('identify') || scopes.some(s => !['identify','guilds.members.read'].includes(s))) throw fail();
    return u.href;
}

export async function readNativeAccount(client: ApiClient): Promise<NativeAccount> {
    const data = await client.request<NativeAccount>('/api/momiao/account');
    if (!data || data.id !== client.getSnapshot().user?.id || typeof data.username !== 'string' || ['has_password','discord_connected','two_fa_enabled'].some(k => typeof data[k as keyof NativeAccount] !== 'boolean')) throw new ApiError('账户状态暂时无法核对，请重新加载。');
    return data;
}

const messages: Record<string,string> = {
    DISCORD_DENIED:'授权已取消，可以重新开始。', DISCORD_NOT_MEMBER:'尚未加入指定 Discord 服务器，请完成加入后再注册。',
    DISCORD_ROLE_REQUIRED:'尚未取得所需身份组，请完成资格要求后再注册。', DISCORD_RATE_LIMITED:'Discord 请求过于频繁，请稍后再试。',
    DISCORD_CONFIG_ERROR:'注册服务配置暂未就绪，请稍后再试。', DISCORD_UNAVAILABLE:'Discord 暂时无法连接，请稍后重试。',
    MOMIAO_DISCORD_UNBOUND:'此 Discord 尚未绑定本站账户，请使用新用户注册入口。', MOMIAO_AUTH_RESTART_REQUIRED:'授权已过期或与当前账户不符，请重新验证。',
    MOMIAO_FLOW_CONSUMED:'此授权已使用，请重新读取账户状态后再开始验证。', MOMIAO_INVALID_REQUEST:'密码或验证信息未通过校验，请核对后重试。',
    MOMIAO_NOT_ELIGIBLE:'此账户暂不符合准入要求，请核对账户状态。', DISCORD_CODE_INVALID:'授权码已失效，请重新开始授权。',
};
export function admissionError(error: unknown): string {
    if (error instanceof ApiError) return messages[error.code] || (error.uncertain ? '操作结果尚未确认。请重新登录或读取账户状态核对，避免重复提交。' : '账户服务暂时未能完成请求，请重新核对状态后再试。');
    return '账户服务暂时无法连接，请稍后再试。';
}
