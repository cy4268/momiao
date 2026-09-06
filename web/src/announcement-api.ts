import { ApiClient, ApiError } from './api';

export const announcementTypes = { SYSTEM: '系统公告', NEW_MODELS: '新模型', GAME_EVENTS: '游戏活动', MAINTENANCE: '维护通知', IMPORTANT: '重要提醒', ACKNOWLEDGEMENTS: '致谢' } as const;
export const announcementStates: Record<string, string> = { DRAFT: '草稿', SCHEDULED: '已预约', PUBLISHED: '已发布', EXPIRED: '已过期', ARCHIVED: '已归档' };
export const placementLabels: Record<string, string> = { PINNED_LIST: '列表置顶', ENTRY_POPUP: '匿名入口弹窗', POST_LOGIN_POPUP: '登录后弹窗候选', PUBLIC_HOME_BANNER: '首页横幅', DASHBOARD_SUMMARY: '指挥台摘要' };
export type AnnouncementType = keyof typeof announcementTypes;
export interface Acknowledgement { display_name: string; external_link: string; acknowledgement_note: string; group_name: string; manual_order: number; anonymous: boolean; consent_attested?: boolean }
export interface Announcement { announcement_id: string; content_version: number; notification_revision: number; title: string; type: AnnouncementType; sanitized_html: string; state: string; publish_at: string | null; visible_from: string | null; visible_until: string | null; updated_at: string; pinned: boolean; read: boolean; acknowledgements: Acknowledgement[] }
export interface AnnouncementContent { title: string; type: AnnouncementType; visibility: 'PUBLIC' | 'AUTHENTICATED'; body_markdown: string; acknowledgements: Acknowledgement[] }
export interface Placement { placement: string; manual_order: number }
export interface OpsAnnouncement extends Announcement { version: number; content: AnnouncementContent; placements: Placement[]; withdrawn_at: string | null; first_published_at: string | null; expired_reason: string; canonical_key: string }
export interface AnnouncementPrincipal { user_id: string; base_role: string; authz_epoch: number; permissions: string[] }
export interface AnnouncementCommand { operation_id: string; authz_epoch: number; announcement_id: string; expected_version: number; action: string; content?: AnnouncementContent; publish_at?: string; visible_from?: string; visible_until?: string; placements?: Placement[]; reason: string }
export interface AnnouncementImpact { action: string; announcement_id: string; target_version: number; title: string; visibility: string; notification_revision: number; read_accounts: number; publish_at: string | null; visible_from: string | null; visible_until: string | null; placements: Placement[]; effect: string }
export interface AnnouncementPreview { preview_id: string; expires_at: string; impact: AnnouncementImpact }
export interface AnnouncementResult { operation_id: string; announcement_id: string; version: number; content_version: number; notification_revision: number; state: string }
export const announcementRoot = '/platform/v1/announcements';
export const announcementOps = '/platform/v1/ops/announcements';
export const announcementTime = (value: string | null) => value ? new Date(value).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false }) : '长期有效';
export function announcementError(error: unknown) {
    const messages: Record<string, string> = { ANNOUNCEMENTS_FORBIDDEN: '当前账户没有公告运营权限。', AUTHORIZATION_STALE: '运营权限已变更，请刷新权限后重新操作。', ANNOUNCEMENT_VERSION_CONFLICT: '公告已被其他操作更新，请重新读取后再编辑。', ANNOUNCEMENT_PLACEMENT_CONFLICT: '入口弹窗或首页横幅与已有预约时间重叠，请调整时间或展示渠道。', ANNOUNCEMENT_CONFIRMATION_REQUIRED: '发布预览已过期或内容已变更，请重新生成影响预览。', ANNOUNCEMENT_OPERATION_CONFLICT: '操作编号已用于其他内容，请先核对原操作结果。', ANNOUNCEMENT_INVALID: '请检查标题、受控 Markdown、展示渠道及发布时间。', ANNOUNCEMENT_NOT_FOUND: '公告暂不可访问，可能已撤回、过期或需要登录。', ANNOUNCEMENTS_UNAVAILABLE: '公告暂时无法读取，请稍后重试。' };
    return error instanceof ApiError ? messages[error.code] || error.message : error instanceof Error ? error.message : '请求未完成，请重试。';
}
export function checkedAnnouncement(value: Announcement): Announcement {
    if (!value || !/^[0-9a-f-]{36}$/i.test(value.announcement_id) || !Number.isSafeInteger(value.notification_revision) || value.notification_revision < 1 || typeof value.title !== 'string' || !(value.type in announcementTypes) || typeof value.sanitized_html !== 'string' || !Array.isArray(value.acknowledgements)) throw new Error('公告响应格式异常，请重新读取。');
    return value;
}
export async function publicAnnouncements(client: ApiClient, query = '') { const page = await client.announcementRequest<{ items: Announcement[]; has_more: boolean }>(announcementRoot + query); if (!page || !Array.isArray(page.items) || typeof page.has_more !== 'boolean') throw new Error('公告列表响应格式异常。'); return { ...page, items: page.items.map(checkedAnnouncement) }; }

export const entryDismissKey = 'chaldea.announcement.entry-dismissed.v1';
export const popupSeenKey = 'chaldea.announcement.popup-seen.v1';
type Presentation = { announcement_id: string; notification_revision: number; dismissed_at?: string };
const seenInMemory = new Set<string>();
const identity = (a: Presentation) => a.announcement_id + ':' + a.notification_revision;
function storedPresentation(key: string, local: boolean): Presentation[] {
    try {
        const value: unknown = JSON.parse((local ? window.localStorage : window.sessionStorage).getItem(key) || '[]');
        return Array.isArray(value) ? value.slice(-256).filter((x): x is Presentation => !!x && typeof x.announcement_id === 'string' && Number.isSafeInteger(x.notification_revision)) : [];
    } catch { return []; }
}
export function popupSeen(a: Announcement) { return seenInMemory.has(identity(a)) || [...storedPresentation(entryDismissKey, true), ...storedPresentation(popupSeenKey, false)].some(x => identity(x) === identity(a)); }
export function markPopupSeen(a: Announcement, dismissed: boolean) {
    seenInMemory.add(identity(a));
    if (seenInMemory.size > 512) seenInMemory.delete(seenInMemory.values().next().value!);
    const value: Presentation = { announcement_id: a.announcement_id, notification_revision: a.notification_revision };
    for (const local of dismissed ? [false, true] : [false]) {
        const key = local ? entryDismissKey : popupSeenKey;
        try { (local ? window.localStorage : window.sessionStorage).setItem(key, JSON.stringify([...storedPresentation(key, local).filter(x => identity(x) !== identity(value)), local ? { ...value, dismissed_at: new Date().toISOString() } : value].slice(-256))); } catch { /* Browser presentation state is optional; memory still prevents route loops. */ }
    }
}
