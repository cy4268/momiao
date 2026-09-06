import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useNavigate } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import { ApiClient } from './api';
import { AnnouncementDetail, AnnouncementEntry, Announcements } from './Announcements';
import { announcementRoot, entryDismissKey, markPopupSeen, popupSeen, popupSeenKey, type Announcement } from './announcement-api';
import { fixtureClient, ok } from './m1-test-fixtures';

const notice: Announcement = { announcement_id: '01990000-1111-7777-aaaa-000000000030', content_version: 1, notification_revision: 1, title: '观测站更新', type: 'SYSTEM', sanitized_html: '<p>服务端净化后的正文。</p>', state: 'PUBLISHED', publish_at: '2026-09-06T00:00:00Z', visible_from: '2026-09-06T00:00:00Z', visible_until: null, updated_at: '2026-09-06T00:00:00Z', pinned: true, read: false, acknowledgements: [] };
describe('announcements', () => {
    it('renders a real filtered public list and empty results', async () => {
        const fetcher = vi.fn(async (path: string) => ok({ items: path.includes('search=missing') ? [] : [notice], has_more: false }));
        render(<MemoryRouter><Announcements client={new ApiClient(fetcher)} /></MemoryRouter>);
        expect(await screen.findByRole('link', { name: '观测站更新' })).toHaveAttribute('href', '/announcements/' + notice.announcement_id);
        fireEvent.change(screen.getByLabelText('搜索公告'), { target: { value: 'missing' } }); fireEvent.click(screen.getByRole('button', { name: '应用筛选' }));
        expect(await screen.findByText('没有符合条件的公告')).toBeVisible();
        expect(fetcher.mock.calls.some(([path]) => path.includes('search=missing'))).toBe(true);
    });
    it('anonymous detail refresh reads canonical HTML without POST and authenticated render posts exact revision', async () => {
        const anonymous = vi.fn(async () => ok(notice));
        const view = render(<MemoryRouter initialEntries={['/announcements/' + notice.announcement_id]}><Routes><Route path='/announcements/:id' element={<AnnouncementDetail client={new ApiClient(anonymous)} />} /></Routes></MemoryRouter>);
        expect(await screen.findByText('服务端净化后的正文。')).toBeVisible(); expect(anonymous).toHaveBeenCalledTimes(1); view.unmount();
        const { client, fetcher } = fixtureClient((path, init) => path === announcementRoot + '/' + notice.announcement_id ? ok({ ...notice, notification_revision: 3 }) : path.endsWith('/reads') ? ok({ notification_revision: 3, read_at: '2026-09-06T00:00:00Z' }) : undefined);
        await client.bootstrap();
        render(<MemoryRouter initialEntries={['/announcements/' + notice.announcement_id]}><Routes><Route path='/announcements/:id' element={<AnnouncementDetail client={client} />} /></Routes></MemoryRouter>);
        await waitFor(() => expect(fetcher.mock.calls.some(([path, init]) => path.endsWith('/reads') && init?.method === 'POST' && init.body === '{"notification_revision":3}')).toBe(true));
    });
    it('dismisses entry without marking read and tolerates unavailable storage', async () => {
        localStorage.clear(); sessionStorage.clear();
        const noStorage = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('disabled'); });
        const fetcher = vi.fn(async (_path: string, _init?: RequestInit) => ok({ item: { ...notice, notification_revision: 19 } })); const client = new ApiClient(fetcher);
        const view = render(<MemoryRouter><AnnouncementEntry client={client} /></MemoryRouter>);
        expect(await screen.findByRole('dialog')).toBeVisible(); fireEvent.click(screen.getByRole('button', { name: '关闭对话框' }));
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();view.unmount();
        render(<MemoryRouter initialEntries={['/login']}><AnnouncementEntry client={client} /></MemoryRouter>);
        await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2));expect(screen.queryByRole('dialog')).not.toBeInTheDocument(); noStorage.mockRestore();
        expect(fetcher.mock.calls.every(([, init]) => !init || init.method !== 'POST')).toBe(true);
    });
    it('uses id plus notification revision for browser presentation state', () => {
        localStorage.setItem(entryDismissKey, 'broken');sessionStorage.setItem(popupSeenKey, '{broken');
        const fresh = { ...notice, notification_revision: 98 }; expect(popupSeen(fresh)).toBe(false);markPopupSeen(fresh, true);expect(popupSeen(fresh)).toBe(true);
        expect(popupSeen({ ...fresh, content_version: 2 })).toBe(true);expect(popupSeen({ ...fresh, notification_revision: 99 })).toBe(false);
    });
    it('destroys an open presentation when leaving the entry and does not resurrect it on return', async () => {
        let navigate: ReturnType<typeof useNavigate>;
        const fetcher = vi.fn(async (_path: string) => ok({ item: { ...notice, notification_revision: 113 } })); const client = new ApiClient(fetcher);
        function Navigation() { navigate = useNavigate(); return <AnnouncementEntry client={client} />; }
        render(<MemoryRouter><Navigation /></MemoryRouter>); expect(await screen.findByRole('dialog')).toBeVisible();
        await act(() => navigate!('/announcements')); expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
        await act(() => navigate!('/')); await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2));
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
});
