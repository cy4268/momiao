import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, it } from 'vitest';
import { OpsAnnouncements } from './OpsAnnouncements';
import { announcementOps, type AnnouncementCommand, type OpsAnnouncement } from './announcement-api';
import { fixtureClient, ok } from './m1-test-fixtures';

it('creates a draft, previews server HTML, confirms publication and withdraws using stable operations', async () => {
    const principal = { user_id: '1', base_role: 'SUPER_ADMIN', authz_epoch: 2, permissions: ['announcements.read', 'announcements.write', 'announcements.publish'] };
    let stored: OpsAnnouncement | null = null; const executions: { command: AnnouncementCommand; confirmed: boolean; preview_id: string }[] = [];
    const { client } = fixtureClient((path, init) => {
        if (!path.startsWith(announcementOps)) return;
        if (path === announcementOps) return ok({ principal, items: stored ? [stored] : [] });
        if (path.endsWith('/render-preview')) return ok({ sanitized_html: '<p>可见的服务端预览</p>' });
        if (path.endsWith('/prepare')) { const { command } = JSON.parse(String(init?.body)); return ok({ preview_id: '01990000-1111-7777-aaaa-000000000041', expires_at: '2099-01-01T00:00:00Z', impact: { ...command, title: stored?.title, visibility: 'PUBLIC', notification_revision: 1, read_accounts: 0, placements: command.placements || [], effect: '这次操作会改变公告可见性。' } }); }
        if (path.endsWith('/execute')) {
            const body = JSON.parse(String(init?.body)); executions.push(body); const c: AnnouncementCommand = body.command;
            stored = { announcement_id: '01990000-1111-7777-aaaa-000000000040', title: c.content?.title || stored?.title || '', type: 'SYSTEM', sanitized_html: '<p>服务端正文</p>', state: c.action === 'PUBLISH' ? 'PUBLISHED' : stored?.state || 'DRAFT', version: (stored?.version || 0) + 1, content_version: 1, notification_revision: 1, content: c.content || stored!.content, placements: c.placements || [], publish_at: c.publish_at || null, visible_from: c.visible_from || null, visible_until: null, updated_at: '2026-09-06T00:00:00Z', withdrawn_at: c.action === 'WITHDRAW' ? '2026-09-06T00:00:00Z' : null, first_published_at: null, expired_reason: '', canonical_key: '', acknowledgements: [], pinned: false, read: false };
            return ok({ operation_id: c.operation_id, announcement_id: stored.announcement_id, version: stored.version, state: stored.state, content_version: 1, notification_revision: 1 });
        }
        return ok({ principal, item: stored });
    });
    await client.bootstrap(); render(<MemoryRouter><OpsAnnouncements client={client} /></MemoryRouter>);
    fireEvent.click(await screen.findByRole('button', { name: '新建公告' }));
    fireEvent.change(screen.getByLabelText('公告标题'), { target: { value: '候选发布测试' } }); fireEvent.change(screen.getByLabelText('公告正文'), { target: { value: '**测试正文**' } });
    fireEvent.click(screen.getByRole('button', { name: '净化预览' })); expect(await screen.findByText('可见的服务端预览')).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: '保存草稿' })); await waitFor(() => expect(executions.length).toBe(1));
    fireEvent.click(await screen.findByRole('button', { name: '发布公告' })); expect(await screen.findByText('这次操作会改变公告可见性。')).toBeVisible(); expect(executions.length).toBe(1);
    fireEvent.click(screen.getByRole('button', { name: '确认执行' })); await waitFor(() => expect(executions.length).toBe(2)); expect(executions[1].confirmed).toBe(true); expect(executions[1].preview_id).toBeTruthy();
    fireEvent.click(await screen.findByLabelText('首页横幅')); fireEvent.click(screen.getByRole('button', { name: '预览渠道调整影响' })); fireEvent.click(await screen.findByRole('button', { name: '确认执行' })); await waitFor(() => expect(executions.length).toBe(3)); expect(executions[2].command.action).toBe('UPDATE_PLACEMENTS'); expect(executions[2].command.placements).toEqual([{ placement: 'PUBLIC_HOME_BANNER', manual_order: 0 }]);
    fireEvent.change(await screen.findByLabelText('操作原因'), { target: { value: '本地验收结束' } }); fireEvent.click(screen.getByRole('button', { name: '撤回公告' }));
    fireEvent.click(await screen.findByRole('button', { name: '确认执行' })); await waitFor(() => expect(executions.length).toBe(4)); expect(executions[3].command.action).toBe('WITHDRAW'); expect(new Set(executions.map(x => x.command.operation_id)).size).toBe(4);
});

it('keeps a confirmed creation when its detail GET fails and recovers by GET without another mutation', async () => {
    const principal = { user_id: '1', base_role: 'SUPER_ADMIN', authz_epoch: 1, permissions: ['announcements.read', 'announcements.write', 'announcements.publish'] };
    let posts = 0; let detailFails = true;
    const id = '01990000-1111-7777-aaaa-000000000099'; const op = '01990000-1111-7777-aaaa-000000000100';
    const stored = { announcement_id: id, title: '已确认草稿', type: 'SYSTEM', content_version: 1, notification_revision: 1, version: 1, state: 'DRAFT', content: { title: '已确认草稿', type: 'SYSTEM', visibility: 'PUBLIC', body_markdown: '正文', acknowledgements: [] }, placements: [], sanitized_html: '<p>正文</p>', publish_at: null, visible_from: null, visible_until: null, withdrawn_at: null, acknowledgements: [] };
    const { client } = fixtureClient((path) => {
        if (path === announcementOps) return ok({ principal, items: posts ? [stored] : [] });
        if (path.endsWith('/execute')) { posts++; return ok({ operation_id: op, announcement_id: id, version: 1, state: 'DRAFT', content_version: 1, notification_revision: 1 }); }
        if (path === announcementOps + '/' + id) { if (detailFails) return Promise.reject(new Error('synthetic read outage')); return ok({ principal, item: stored }); }
    });
    await client.bootstrap(); render(<MemoryRouter><OpsAnnouncements client={client} /></MemoryRouter>);
    fireEvent.click(await screen.findByRole('button', { name: '新建公告' })); fireEvent.change(screen.getByLabelText('公告标题'), { target: { value: '已确认草稿' } }); fireEvent.change(screen.getByLabelText('公告正文'), { target: { value: '正文' } });
    fireEvent.click(screen.getByRole('button', { name: '保存草稿' }));
    expect(await screen.findByText(/写入已确认/)).toBeVisible(); expect(screen.getByRole('button', { name: '保存草稿' })).toBeDisabled(); expect(screen.getByText(op, { exact: false })).toBeVisible();
    detailFails = false; fireEvent.click(screen.getByRole('button', { name: '重新读取已保存公告' }));
    await waitFor(() => expect(screen.getByRole('button', { name: '保存草稿' })).toBeEnabled()); expect(posts).toBe(1); expect(screen.getByLabelText('公告标题')).toHaveValue('已确认草稿');
});
