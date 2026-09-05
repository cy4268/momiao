import { ApiError, type ApiClient } from './api';
export const assets = ['RESERVE_API_CREDIT', 'AVAILABLE_CHIPS'] as const;
export type Asset = typeof assets[number];
export const assetNames: Record<Asset, string> = { RESERVE_API_CREDIT: 'Reserve API Credit', AVAILABLE_CHIPS: '可用筹码' };
export interface WalletBalance { asset: Asset; balance_units: string; amount: string; ledger_seq: string; version: string }
export interface WalletData { initialized: boolean; user_id: string; wallets: WalletBalance[]; scope: 'LOCAL_WALLETS_ONLY'; total_assets: null }
export interface LedgerEntry { id: string; transaction_id: string; user_id: string; asset: Asset; ledger_seq: string; wallet_version: string; entry_type: string; biz_type: string; biz_id: string; delta_units: string; balance_before_units: string; balance_after_units: string; created_at: string; delta_amount: string; balance_after_amount: string }
export interface LedgerPage { items: LedgerEntry[]; has_more: boolean; next_after_seq: string | null }
const record = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
export const integer = (v: unknown, signed = false): v is string => typeof v === 'string' && (signed ? /^-?(0|[1-9]\d*)$/ : /^(0|[1-9]\d*)$/).test(v) && v.length <= 20 && BigInt(v) <= 9223372036854775807n && BigInt(v) >= (signed ? -9223372036854775808n : 0n);
// Exact fixed-point validation: display the server string; never pass money through floating point.
export function amount(v: unknown, units: unknown, signed = false): v is string {
    if (typeof v !== 'string' || !integer(units, signed) || !(signed ? /^-?(0|[1-9]\d*)(\.\d{1,6})?$/ : /^(0|[1-9]\d*)(\.\d{1,6})?$/).test(v) || v.length > 30) return false;
    const [whole, fraction = ''] = v.replace(/^-/, '').split('.');
    const micros = (BigInt(whole) * 1000000n + BigInt(fraction.padEnd(6, '0'))) * (v.startsWith('-') ? -1n : 1n);
    return micros === BigInt(units) * 2n;
}
const malformed = () => new ApiError('钱包响应格式异常，请刷新核对；当前未展示余额。');
export function parseWallet(v: unknown, userID: string): WalletData {
    if (!record(v) || typeof v.initialized !== 'boolean' || !integer(v.user_id) || v.user_id !== userID || v.scope !== 'LOCAL_WALLETS_ONLY' || v.total_assets !== null || !Array.isArray(v.wallets)) throw malformed();
    const wallets = v.wallets;
    if (!v.initialized ? wallets.length !== 0 : wallets.length !== 2 || assets.some(asset => wallets.filter(w => record(w) && w.asset === asset).length !== 1)) throw malformed();
    for (const w of v.wallets) if (!record(w) || !assets.includes(w.asset as Asset) || !amount(w.amount, w.balance_units) || !integer(w.ledger_seq) || !integer(w.version)) throw malformed();
    return v as unknown as WalletData;
}
export function parseLedger(v: unknown, asset: Asset, after: string, userID: string): LedgerPage {
    if (!record(v) || !Array.isArray(v.items) || v.items.length > 20 || typeof v.has_more !== 'boolean' || !(v.next_after_seq === null || integer(v.next_after_seq))) throw malformed();
    let previous = BigInt(after);
    for (const e of v.items) {
        if (!record(e) || ['id','transaction_id','entry_type','biz_type','biz_id'].some(k => typeof e[k] !== 'string' || !e[k]) || e.user_id !== userID || e.asset !== asset || !integer(e.ledger_seq) || BigInt(e.ledger_seq) <= previous || !integer(e.wallet_version) || !integer(e.balance_before_units) || !amount(e.delta_amount,e.delta_units,true) || !amount(e.balance_after_amount,e.balance_after_units) || BigInt(e.balance_before_units) + BigInt(e.delta_units as string) !== BigInt(e.balance_after_units as string) || typeof e.created_at !== 'string' || !/^\d{4}-\d\d-\d\dT/.test(e.created_at) || !isFinite(Date.parse(e.created_at))) throw malformed();
        previous = BigInt(e.ledger_seq);
    }
    if (v.has_more ? !v.items.length || v.next_after_seq !== String(previous) : v.next_after_seq !== null) throw malformed();
    return v as unknown as LedgerPage;
}
export const readWallet = async (client: ApiClient, userID: string) => parseWallet(await client.request('/platform/v1/wallet'), userID);
export const readLedger = async (client: ApiClient, userID: string, asset: Asset, after: string) => parseLedger(await client.request(`/platform/v1/wallet/ledger?asset=${asset}&after_seq=${after}&limit=20`), asset, after, userID);
