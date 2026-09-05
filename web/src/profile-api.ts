import { ApiError, type ApiClient } from './api';

export interface MasterProfileData {
    user_id: string;
    short_account_id: string;
    status: 'INCOMPLETE' | 'COMPLETE';
    display_name: string;
    avatar_id: 'system-default';
    profile_version: string;
    nickname_changed_at: string | null;
    next_rename_at: string | null;
    suggested_name: string;
    avatars: [{ id: 'system-default'; label: '系统默认头像'; source: 'SYSTEM' }];
}
const record = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
const integer = (v: unknown): v is string => typeof v === 'string' && /^(0|[1-9]\d*)$/.test(v) && v.length <= 19 && BigInt(v) <= 9223372036854775807n;
function timestamp(v: unknown): v is string {
    if (typeof v !== 'string' || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,9})?Z$/.test(v)) return false;
    const time = Date.parse(v);
    return Number.isFinite(time) && new Date(time).toISOString().slice(0, 19) === v.slice(0, 19);
}
const malformed = () => new ApiError('资料响应格式异常，请刷新核对；当前响应未被采用。', 0, 'PROFILE_INVALID_RESPONSE');
export function parseProfile(v: unknown, userID: string): MasterProfileData {
    if (!record(v) || Object.keys(v).length !== 10 || !integer(v.user_id) || v.user_id === '0' || v.user_id !== userID || !integer(v.profile_version) || (v.status !== 'INCOMPLETE' && v.status !== 'COMPLETE') || typeof v.display_name !== 'string' || v.avatar_id !== 'system-default' || typeof v.short_account_id !== 'string' || !/^CA-[0-9A-F]{12}$/.test(v.short_account_id) || v.suggested_name !== `Master-${v.short_account_id}`) throw malformed();
    if (!Array.isArray(v.avatars) || v.avatars.length !== 1 || !record(v.avatars[0]) || Object.keys(v.avatars[0]).length !== 3 || v.avatars[0].id !== 'system-default' || v.avatars[0].label !== '系统默认头像' || v.avatars[0].source !== 'SYSTEM') throw malformed();
    if (v.nickname_changed_at === null ? v.next_rename_at !== null : !timestamp(v.nickname_changed_at) || !timestamp(v.next_rename_at) || Date.parse(v.next_rename_at) <= Date.parse(v.nickname_changed_at)) throw malformed();
    if (v.status === 'INCOMPLETE' ? v.display_name !== '' || v.profile_version !== '0' || v.nickname_changed_at !== null : !v.display_name.trim() || v.profile_version === '0') throw malformed();
    return v as unknown as MasterProfileData;
}
const errors: Record<string, string> = {
    NICKNAME_TAKEN: '昵称已被使用，请换一个昵称后再保存。',
    NICKNAME_RESERVED: '昵称属于保留名称，请选择其他昵称。',
    INVALID_NICKNAME: '昵称格式未通过校验。请使用 1–24 个字符的文字、数字及允许的分隔符，不含表情或隐藏字符。',
    RENAME_COOLDOWN: '改名冷却尚未结束，请刷新资料查看下次可改名时间，以服务端校验为准。',
    STALE_RESOURCE_VERSION: '资料版本已更新，正在读取最新资料。请核对后重新编辑，不会自动重放本次修改。',
    INVALID_AVATAR: '头像选项已失效，请刷新资料后使用系统默认头像。',
    PROFILE_UNAVAILABLE: '资料服务暂不可用，请稍后刷新资料；其他门户功能仍可使用。',
    PROFILE_INVALID_RESPONSE: '资料响应格式异常，请刷新核对；当前响应未被采用。',
    PROFILE_STALE_READ: '读取的资料版本落后于保存结果，请稍后刷新核对；保存继续暂停。',
    PROFILE_FORBIDDEN: '当前账户暂未开放资料访问，请稍后再试。',
    INVALID_REQUEST: '资料请求格式有误，请刷新资料后重试。',
};
export function profileError(e: unknown): string {
    if (e instanceof ApiError) {
        if (Object.hasOwn(errors, e.code)) return errors[e.code];
        if (e.status === 401) return '登录状态已改变，请重新登录后读取资料。';
        if (e.status === 403) return '资料访问被拒绝，请刷新资料核对当前账户。';
        if (e.status >= 500) return '资料服务暂不可用，请稍后刷新资料。';
    }
    return '资料请求未完成，请检查连接后刷新核对。';
}
export const readProfile = async (client: ApiClient, userID: string) => parseProfile(await client.request('/platform/v1/master-profile'), userID);
