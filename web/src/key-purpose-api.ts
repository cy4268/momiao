import { ApiClient, ApiError } from './api';

export type KeyPurpose = 'GENERAL' | 'ROLEPLAY' | 'UNCLASSIFIED';
export type KeyPurposeItem = { token_id: string; purpose: KeyPurpose; version: string; effective_at: string | null; sync_state: 'EFFECTIVE' | 'PENDING' };
const positiveID = (value: unknown): value is string => typeof value === 'string' && /^[1-9][0-9]{0,18}$/.test(value);
function item(value: unknown): KeyPurposeItem {
    if (!value || typeof value !== 'object') throw new ApiError('密钥用途响应格式异常。');
    const row = value as KeyPurposeItem;
    if (!positiveID(row.token_id) || !['GENERAL','ROLEPLAY','UNCLASSIFIED'].includes(row.purpose) || typeof row.version !== 'string' || !/^(0|[1-9][0-9]{0,18})$/.test(row.version) || !['EFFECTIVE','PENDING'].includes(row.sync_state) || !(row.effective_at === null || typeof row.effective_at === 'string' && Number.isFinite(Date.parse(row.effective_at)))) throw new ApiError('密钥用途响应格式异常。');
    return row;
}
export async function keyPurposes(client: ApiClient): Promise<KeyPurposeItem[]> {
    const response = await client.request<{items: unknown[]}>('/platform/v1/key-purposes');
    if (!response || !Array.isArray(response.items)) throw new ApiError('密钥用途响应格式异常。');
    const items = response.items.map(item);
    if (new Set(items.map(row => row.token_id)).size !== items.length) throw new ApiError('密钥用途记录重复。');
    return items;
}
export async function saveKeyPurpose(client: ApiClient, tokenID: string, purpose: Exclude<KeyPurpose,'UNCLASSIFIED'>, operationID: string, version = '0'): Promise<KeyPurposeItem> {
    if (!positiveID(tokenID)) throw new ApiError('密钥编号无效。');
    const result = version === '0'
        ? await client.request('/platform/v1/key-purposes', 'POST', {operation_id: operationID, token_id: tokenID, purpose})
        : await client.request(`/platform/v1/key-purposes/${tokenID}`, 'PUT', {operation_id: operationID, purpose, expected_version: version});
    try { const parsed = item(result); if (parsed.token_id !== tokenID || parsed.purpose !== purpose) throw new Error(); return parsed; }
    catch { throw new ApiError('用途提交结果尚未确认，请刷新用途记录。', 0, 'KEY_PURPOSE_UNKNOWN', true); }
}
export const keyPurposeLabel = (purpose: KeyPurpose) => ({GENERAL:'General · 通用',ROLEPLAY:'RP · 角色扮演',UNCLASSIFIED:'未分类'}[purpose]);
