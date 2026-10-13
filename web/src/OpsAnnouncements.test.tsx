import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, it, vi } from 'vitest';
import { App } from './App';
import { ApiClient } from './api';
import { announcementOps, type AnnouncementCommand, type OpsAnnouncement } from './announcement-api';
import { ok, failed } from './m1-test-fixtures';

it('creates, renders, publishes and withdraws directly using local admin roles', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true);
    let stored: OpsAnnouncement | null = null; const executions: AnnouncementCommand[] = [];
    const fetcher = vi.fn(async (path: string, init?: RequestInit) => {
        if(path==='/api/v1/session/bootstrap') return ok({authentication_state:'AUTHENTICATED',csrf_token:'local-csrf'});
        if(path==='/api/user/self') return ok({id:1,username:'operator',role:10});
        if (path.startsWith('/platform/v1/access-gate?')) return ok({ stage: 'READY', user_id: '1', route: '/ops/announcements' });
        if (path === '/api/v1/maintenance/notices') return ok({ items: [] });
        if (!path.startsWith(announcementOps)) throw new Error('Unexpected endpoint: ' + path);
        if (path === announcementOps && init?.method !== 'POST') return ok({ items: stored ? [stored] : [] });
        if (path.endsWith('/render-preview')) return ok({ sanitized_html: '<p>可见的服务端预览</p>' });
        if (path === announcementOps && init?.method === 'POST') {
            const body = JSON.parse(String(init?.body)); executions.push(body); const c: AnnouncementCommand = body;
            stored = { announcement_id: '01990000-1111-7777-aaaa-000000000040', title: c.content?.title || stored?.title || '', type: 'SYSTEM', sanitized_html: '<p>服务端正文</p>', state: c.action === 'PUBLISH' ? 'PUBLISHED' : stored?.state || 'DRAFT', content_version: 1, notification_revision: 1, content: c.content || stored!.content, placements: c.placements || [], publish_at: c.publish_at || null, visible_from: c.visible_from || null, visible_until: null, updated_at: '2026-09-06T00:00:00Z', withdrawn_at: c.action === 'WITHDRAW' ? '2026-09-06T00:00:00Z' : null, first_published_at: null, expired_reason: '', canonical_key: '', acknowledgements: [], pinned: false, read: false };
            return ok(stored);
        }
        return ok(stored);
    });
    const client = new ApiClient(fetcher); await client.bootstrap(); render(<MemoryRouter initialEntries={['/ops/announcements']}><App client={client}/></MemoryRouter>);
    fireEvent.click(await screen.findByRole('button', { name: '新建公告' }));
    fireEvent.change(screen.getByLabelText('公告标题'), { target: { value: '候选发布测试' } }); fireEvent.change(screen.getByLabelText('公告正文'), { target: { value: '**测试正文**' } });
    fireEvent.click(screen.getByRole('button', { name: '净化预览' })); expect(await screen.findByText('可见的服务端预览')).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: '保存草稿' })); await waitFor(() => expect(executions.length).toBe(1));
    await waitFor(() => expect(screen.getByRole('button', { name: '发布公告' })).toBeEnabled());
    expect(executions[0]).not.toHaveProperty('operation_id'); expect(executions[0]).not.toHaveProperty('authz_epoch'); expect(executions[0]).not.toHaveProperty('expected_version');
    fireEvent.click(screen.getByRole('button', { name: '发布公告' }));
    await waitFor(() => expect(screen.getByRole('button', { name: '保存展示渠道' })).toBeEnabled()); expect(executions.length).toBe(2);
    fireEvent.click(screen.getByLabelText('首页横幅')); fireEvent.click(screen.getByRole('button', { name: '保存展示渠道' }));
    await waitFor(() => expect(screen.getByRole('button', { name: '保存展示渠道' })).toBeEnabled());
    expect(executions[2].action).toBe('UPDATE_PLACEMENTS'); expect(executions[2].placements).toEqual([{ placement: 'PUBLIC_HOME_BANNER', manual_order: 0 }]);
    fireEvent.change(screen.getByLabelText('公告正文'), { target: { value: '更新正文' } }); fireEvent.click(screen.getByRole('button', { name: '保存正文' }));
    await waitFor(() => expect(screen.getByRole('button', { name: '保存正文' })).toBeEnabled()); expect(executions[3].action).toBe('UPDATE_CONTENT_ONLY');
    fireEvent.click(screen.getByRole('button', { name: '重新通知' }));
    await waitFor(() => expect(screen.getByRole('button', { name: '重新通知' })).toBeEnabled()); expect(executions[4].action).toBe('RE_NOTIFY');
    confirm.mockReturnValueOnce(false); fireEvent.click(screen.getByRole('button', { name: '撤回公告' })); expect(executions).toHaveLength(5);
    fireEvent.click(screen.getByRole('button', { name: '撤回公告' }));
    await waitFor(() => expect(screen.queryByRole('button', { name: '撤回公告' })).not.toBeInTheDocument()); expect(executions[5].action).toBe('WITHDRAW');
    expect(screen.queryByLabelText('操作原因')).not.toBeInTheDocument();
    expect(fetcher.mock.calls.some(([path]) => /\/(prepare|execute)$/.test(path) || path === '/api/v1/ops/bootstrap')).toBe(false);
    expect(confirm).toHaveBeenCalledTimes(3); confirm.mockRestore();
});

it('keeps the saved item when the following list refresh fails without repeating the write', async () => {
    const id = '01990000-1111-7777-aaaa-000000000099'; let posts = 0; let failRead = false;
    const stored = { announcement_id: id, title: '已保存草稿', type: 'SYSTEM', content_version: 1, notification_revision: 1, state: 'DRAFT', content: { title: '已保存草稿', type: 'SYSTEM', visibility: 'PUBLIC', body_markdown: '正文', acknowledgements: [] }, placements: [], sanitized_html: '<p>正文</p>', publish_at: null, visible_from: null, visible_until: null, withdrawn_at: null, acknowledgements: [] };
    const fetcher = vi.fn(async (path: string, init?: RequestInit) => {
        if (path === '/api/v1/session/bootstrap') return ok({ authentication_state: 'AUTHENTICATED', csrf_token: 'local-csrf' });
        if (path === '/api/user/self') return ok({ id: 1, username: 'operator', role: 10 });
        if (path.startsWith('/platform/v1/access-gate?')) return ok({ stage: 'READY', user_id: '1', route: '/ops/announcements' });
        if (path === '/api/v1/maintenance/notices') return ok({ items: [] });
        if (path === announcementOps && init?.method === 'POST') { posts++; failRead = true; return ok(stored); }
        if (path === announcementOps) return failRead ? failed() : ok({ items: posts ? [stored] : [] });
        throw new Error('Unexpected endpoint: ' + path);
    });
    const client = new ApiClient(fetcher); await client.bootstrap();
    render(<MemoryRouter initialEntries={['/ops/announcements']}><App client={client}/></MemoryRouter>);
    fireEvent.click(await screen.findByRole('button', { name: '新建公告' })); fireEvent.change(screen.getByLabelText('公告标题'), { target: { value: '已保存草稿' } }); fireEvent.change(screen.getByLabelText('公告正文'), { target: { value: '正文' } });
    fireEvent.click(screen.getByRole('button', { name: '保存草稿' }));
    expect(await screen.findByText('公告已保存。')).toBeVisible();
    await waitFor(() => expect(screen.getByRole('button', { name: '发布公告' })).toBeEnabled());
    expect(await screen.findByRole('alert')).toBeVisible(); expect(posts).toBe(1); expect(screen.getByLabelText('公告标题')).toHaveValue('已保存草稿');
    failRead = false; fireEvent.click(screen.getByRole('button', { name: '刷新列表' }));
    await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument()); expect(posts).toBe(1);
});
