import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { expect, it, vi } from 'vitest';
import { ApiClient } from '../api';
import { HistoryList, HistoryRound, HistorySession } from './History';
import { RouletteHistory } from '../roulette/RouletteHistory';

it('keeps the archive filter, inner scroll and focus through a read-only round and lazy proof', async () => {
  const id = '01993200-0000-7000-8000-000000000001', time = '2026-09-06T10:00:00Z';
  const metadata = { snapshot_id: id, game_title: '命运骰盅', table_id: null, table_name: null, actor_display_name: '御主', metadata_origin: 'creation_snapshot', captured_at: time };
  const client = new ApiClient();
  const request = vi.spyOn(client, 'request').mockImplementation(async (path, method) => {
    expect(method).toBe('GET');
    if (path.includes('/sessions/')) return { session_id: id, table_id: id, seat_no: 1, state: 'CLOSED', started_at: time, ended_at: time, end_reason: 'LEAVE', initial_buyin_units: '5000000', confirmed_topup_units: '10000000', confirmed_rebuy_units: '0', final_cashout_units: '15000000', realized_pl_units: '0', metadata, configuration: {}, funding_count: '2', hand_count: '0', hands: [], funding: [{ funding_operation_id: id, kind: path.includes('funding_cursor=') ? 'TOP_UP' : 'BUY_IN', state: 'CONFIRMED', amount_units: path.includes('funding_cursor=') ? '10000000' : '5000000', created_at: time, confirmed_at: time, failure_code: null }], next_funding_cursor: path.includes('funding_cursor=') ? undefined : 'next-funding', read_at: time } as never;
    if (path.endsWith('/verify')) return { released: false, server_seed_hash: 'a'.repeat(64) } as never;
    if (path.includes('/rounds/')) return { economy_settlement:{policy_version:'economy-cap-v1',policy_hash:'a'.repeat(64),gross_payout_units:'10000000',credited_payout_units:'7500000',withheld_units:'2500000',actual_net_units:'2500000'}, id, game: 'dice', state: 'SETTLED', recovery_state: 'NORMAL', metadata, created_at: time, settled_at: time, total_stake_units: '5000000', total_payout_units: '10000000', net_change_units: '5000000', balance_before_units: '500000000', balance_after_units: '505000000', common_result: 'WIN', input: { type: 'DICE', wager: '10', choice: 'SMALL' }, fairness: { algorithm_version: 'dice-map-v1' }, transactions: [], dice: { dice: [2, 3, 4], total: 9, side: 'SMALL', triple: false, choice: 'SMALL' } } as never;
    return { items: [{ record_type: 'DIRECT_PLAY_ROUND', source_id: id, parent_source_id: null, game_slug: 'dice', mode: 'DIRECT_PLAY', occurred_at: time, ended_at: time, result: 'WIN', status: 'SETTLED', source_version: '1', stake_units: '5000000', payout_units: '10000000', net_change_units: '5000000', initial_buyin_units: null, total_topup_units: null, final_cashout_units: null, snapshot: metadata }], next_cursor: null, has_more: false, game_options: [{ game_slug: 'dice', game_title: '命运骰盅', retired: false }] } as never;
  });
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {});
  const view = render(<MemoryRouter initialEntries={['/history?game_slug=dice&status=SETTLED']}><Routes><Route path="/history" element={<HistoryList client={client} />} /><Route path="/history/rounds/:id" element={<HistoryRound client={client} />} /></Routes></MemoryRouter>);
  expect(await screen.findByRole('link', { name: '查看详情 →' })).toBeVisible();
  expect(view.container.querySelector('.history-archive')).not.toBeNull();
  const filters = screen.getByRole('button', { name: /更多筛选/ });
  filters.focus(); fireEvent.click(filters);
  const dialog = screen.getByRole('dialog', { name: '筛选游戏记录' });
  expect(within(dialog).getByLabelText('游戏')).toHaveValue('dice');
  expect(within(dialog).getByLabelText('状态')).toHaveValue('SETTLED');
  fireEvent.change(within(dialog).getByLabelText('游玩模式'), { target: { value: 'DIRECT_PLAY' } });
  fireEvent.click(within(dialog).getByRole('button', { name: '应用筛选' }));
  expect(await screen.findByRole('link', { name: '查看详情 →' })).toBeVisible();
  await waitFor(() => expect(request.mock.calls.filter(([path]) => path.startsWith('/api/v1/history?'))).toHaveLength(2));
  await new Promise(resolve => requestAnimationFrame(resolve));
  expect.soft(screen.getByRole('button', { name: /更多筛选/ })).toHaveFocus();
  const rows = screen.getByRole('region', { name: '历史记录列表' }); rows.scrollTop = 240;
  fireEvent.click(screen.getByRole('link', { name: '查看详情 →' }));
  expect(await screen.findByLabelText('骰子点数 2、3、4，合计 9 点')).toBeVisible();
  expect(screen.getByLabelText('经济结算回执')).toHaveTextContent('实际到账15');expect(screen.getByLabelText('经济结算回执')).toHaveTextContent('封顶未入账5');
  for (const name of ['本局输入与版本记录', '钱包交易', '公平性记录']) expect(screen.getByText(name, { selector: 'summary' }).closest('details')).not.toHaveAttribute('open');
  expect(request.mock.calls.some(([path]) => path.endsWith('/verify'))).toBe(false);
  fireEvent.click(screen.getByText('公平性记录'));
  expect(await screen.findByText('a'.repeat(64))).toBeVisible();
  expect(screen.queryByText('验证通过')).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('link', { name: '← 返回筛选列表' }));
  const restored = await screen.findByRole('link', { name: '查看详情 →' });
  await waitFor(() => expect(restored).toHaveFocus());
  expect(screen.getByRole('region', { name: '历史记录列表' }).scrollTop).toBe(240);
  const last = new URL(request.mock.calls.at(-1)![0], 'https://example.test');
  expect(last.searchParams.get('game_slug')).toBe('dice'); expect(last.searchParams.get('status')).toBe('SETTLED');
  view.unmount();

  const session = render(<MemoryRouter initialEntries={['/history/sessions/' + id]}><Routes><Route path="/history/sessions/:id" element={<HistorySession client={client} />} /></Routes></MemoryRouter>);
  fireEvent.click(await screen.findByText('资金操作与钱包交易 · 2 笔'));
  fireEvent.click(within(screen.getByRole('navigation', { name: '资金操作分页' })).getByRole('button', { name: '下一页' }));
  expect.soft(await screen.findByText('补充筹码 · 20 筹码')).toBeVisible();
  session.unmount();

  const binding = { config_version_id: id, config_hash: 'a'.repeat(64), wager_policy_version_id: id, wager_policy_hash: 'b'.repeat(64), ruleset_version: 'momiao-devil-rules-v1', algorithm_version: 'momiao-devil-rng-v1', fairness_stream_version: 'chaldea-pf-hmac-sha256-v1' };
  const room = { id, game: 'devil-roulette', title: '恶魔轮盘', version: '1', sequence: '0', state: 'WAITING', target_players: 2, stake_units: '5000000', pool_units: '0', binding, server_seed_hash: 'c'.repeat(64), turn_seat: null, server_now: time, deadline: null, game_deadline: null, players: [], self: null, actions: [], log: [], devil: null, pressure: null };
  request.mockImplementation(async (path, method) => {
    expect(method).toBe('GET');
    return (path.endsWith('/verify') ? { round_id: id, status: 'UNREVEALED', valid: false, commitment_valid: false, config_valid: false, actions_valid: false, settlement_valid: false, binding, action_count: '0', history_path: '/history/roulette/' + id } : { id, game: 'devil-roulette', state: 'WAITING', binding, stake_units: '0', payout_units: '0', net_units: '0', reason: '', created_at: time, ended_at: null, snapshot: metadata, view: room, actions: [], commands: [], frames: [], next_cursor: null, transaction_ids: [], transactions_truncated: false }) as never;
  });
  render(<MemoryRouter initialEntries={['/history/roulette/' + id]}><Routes><Route path="/history/roulette/:id" element={<RouletteHistory client={client} />} /></Routes></MemoryRouter>);
  fireEvent.click(await screen.findByText('验证本局随机与结算'));
  expect(await screen.findByText('尚未揭示 · 结束后可复算')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: '刷新记录' }));
  expect.soft(await screen.findByText('尚未揭示 · 结束后可复算')).toBeVisible();
});
