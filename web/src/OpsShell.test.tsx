import { fireEvent, render, screen, within } from '@testing-library/react';
import { Link, MemoryRouter, Route, Routes } from 'react-router-dom';
import { expect, it } from 'vitest';
import { OpsHome, OpsShell } from './ops/OpsShell';
import { fixtureClient, ok, failed } from './m1-test-fixtures';

it('groups only permitted operations after a bootstrap retry and clears the directory when permissions are revoked', async () => {
    const principal = { principal_id: '01990000-1111-7777-aaaa-000000000010', newapi_user_id: '1', base_role: 'OPERATOR', status: 'ACTIVE', authz_epoch: '2', version: '1', scopes: [], permissions: ['models.read', 'users.read'] };
    let unavailable = true;
    const { client } = fixtureClient(path => path === '/api/v1/ops/bootstrap'
        ? unavailable ? failed() : ok({ principal, registry_version: '1', operations: [] }) : undefined);
    await client.login('ops', 'fixture');
    const page = (value: typeof client) => <MemoryRouter initialEntries={['/ops']}><Routes><Route path='/ops' element={<OpsShell client={value} />}><Route index element={<OpsHome />} /><Route path='models' element={<><h1>模型管理内容</h1><Link to='/ops'>返回运营目录</Link></>} /></Route></Routes></MemoryRouter>;
    const view = render(page(client));
    expect(await screen.findByRole('alert')).toBeVisible();
    unavailable = false;
    fireEvent.click(screen.getByRole('button', { name: '重新核对权限' }));
    await screen.findByRole('heading', { name: '运营工作台' });
    const group = screen.getByRole('region', { name: '内容与用户' });
    expect(within(group).getAllByRole('link')).toHaveLength(2);
    expect(within(group).getByRole('link', { name: /模型目录/ })).toHaveAttribute('href', '/ops/models');
    expect(within(group).getByRole('link', { name: /用户/ })).toHaveAttribute('href', '/ops/users');
    expect(screen.queryByRole('region', { name: '业务管理' })).not.toBeInTheDocument();
    expect(screen.queryByRole('region', { name: '系统与审计' })).not.toBeInTheDocument();
    const nav = screen.getByRole('navigation', { name: '运营导航' });
    expect(within(nav).getAllByRole('link')).toHaveLength(2);
    expect(within(nav).queryByRole('link', { name: '访问控制' })).not.toBeInTheDocument();
    expect(screen.getByRole('main')).toHaveFocus();
    expect(view.container.querySelector('.ops-backdrop')).toHaveAttribute('alt', '');
    fireEvent.error(view.container.querySelector('.ops-backdrop')!);
    expect(view.container.querySelector('.ops-backdrop')).not.toBeInTheDocument();

    fireEvent.click(within(nav).getByRole('link', { name: '模型目录' }));
    await screen.findByRole('heading', { name: '模型管理内容' });
    expect(view.container.querySelector('.ops-workspace')).toHaveClass('ops-workbench');
    expect(view.container.querySelector('.ops-shell')).not.toHaveClass('is-overview');
    expect(screen.getByRole('main')).toHaveFocus();

    fireEvent.click(screen.getByRole('link', { name: '返回运营目录' }));
    await screen.findByRole('heading', { name: '运营工作台' });
    expect(view.container.querySelector('.ops-workspace')).not.toHaveClass('ops-workbench');

    const revoked = fixtureClient(path => path === '/api/v1/ops/bootstrap'
        ? ok({ principal: { ...principal, permissions: [] }, registry_version: '1', operations: [] }) : undefined).client;
    await revoked.login('ops', 'fixture');
    view.rerender(page(revoked));
    expect(await screen.findByText('当前没有可访问的运营模块。')).toBeVisible();
    expect(screen.queryByRole('region', { name: '内容与用户' })).not.toBeInTheDocument();
    expect(within(screen.getByRole('navigation', { name: '运营导航' })).queryByRole('link')).not.toBeInTheDocument();
});
