import { useState } from 'react';
import { act, fireEvent, render, screen, within } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import { BlackjackGame, type BlackjackProjectionDTO } from './BlackjackGame';

const snapshot: BlackjackProjectionDTO = {
    phase: 'PLAYER_TURN', round_version: '12', active_hand_id: 'left', dealer_cards: [0], dealer_revealed: false,
    hands: [
        { hand_id: 'right', hand_index: 4, cards: [5, 9], stake_units: '5500000', hand_state: 'STOOD', value: { hard_total: 16, best_total: 16, is_soft: false }, is_natural: false, payout_units: '0', net_change_units: '0' },
        { hand_id: 'left', hand_index: 0, cards: [7, 20], stake_units: '5500000', hand_state: 'ACTIVE', value: { hard_total: 16, best_total: 16, is_soft: false }, is_natural: false, payout_units: '0', net_change_units: '0' },
    ],
    legal_actions: ['HIT', 'STAND', 'DOUBLE', 'SPLIT'], last_player_action_at: '2026-09-06T08:00:00Z', auto_resolve_at: '2026-09-07T08:00:00Z', total_stake_units: '11000000', total_payout_units: '0', net_change_units: '0',
};
const base = { availableUnits: '50000000', wagerChips: '11', onWagerChange: () => {}, busy: false, onDeal: vi.fn(async () => {}), onAction: vi.fn(async () => {}) };

it('starts only on explicit Deal with a valid controlled wager and no extra confirmation', async () => {
    const deal = vi.fn(async () => {});
    function Fixture() { const [wager, setWager] = useState('10'); return <BlackjackGame {...base} snapshot={null} wagerChips={wager} onWagerChange={setWager} onDeal={deal} />; }
    render(<Fixture />); fireEvent.click(screen.getByRole('button', { name: '100 筹码' })); expect(deal).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText('初始下注 · 筹码'), { target: { value: '11' } });
    await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Deal · 11 筹码' })));
    expect(deal).toHaveBeenCalledExactlyOnceWith('5500000'); expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
});

it('uses stable hand order, neutral hidden hole card and only server-approved commands', async () => {
    let finish!: () => void; const command = vi.fn(() => new Promise<void>(resolve => { finish = resolve; }));
    render(<BlackjackGame {...base} snapshot={{ ...snapshot, dealer_cards: [0, 51] }} onAction={command} roundID="bj-round" />);
    const hands = screen.getAllByTestId('blackjack-hand'); expect(hands.map(h => h.getAttribute('data-hand-id'))).toEqual(['left', 'right']);
    expect(screen.getByLabelText('庄家暗牌，尚未公开')).toBeVisible(); expect(screen.queryByLabelText('庄家总点数')).not.toBeInTheDocument();
    expect(within(screen.getByRole('group', { name: '庄家公开手牌' })).getAllByTestId('playing-card')).toHaveLength(1);
    fireEvent.click(screen.getByRole('button', { name: '查看第 2 手' }));
    fireEvent.click(screen.getByRole('button', { name: 'Hit · 要牌' }));
    fireEvent.click(screen.getByRole('button', { name: 'Stand · 停牌' }));
    expect(command).toHaveBeenCalledExactlyOnceWith({ hand_id: 'left', action_type: 'HIT', expected_round_version: '12' });
    expect(screen.getByRole('button', { name: 'Stand · 停牌' })).toBeDisabled();
    await act(async () => finish());
    expect(within(hands[0]).getAllByTestId('playing-card')).toHaveLength(2); // no predicted hit card
});

it('blocks only additional-stake actions on low balance, then obeys recovery and server timeout state', () => {
    const view = render(<BlackjackGame {...base} availableUnits="5000000" snapshot={snapshot} />);
    expect(screen.getByRole('button', { name: 'Hit · 要牌' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Double · +11 筹码' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Split · +11 筹码' })).toBeDisabled();
    expect(screen.getByText(/当前余额不足以追加/)).toBeVisible();
    view.rerender(<BlackjackGame {...base} snapshot={snapshot} recovering />);
    expect(screen.getByRole('button', { name: 'Hit · 要牌' })).toBeDisabled(); expect(screen.getByText(/正在恢复同一局/)).toBeVisible();
    view.rerender(<BlackjackGame {...base} snapshot={snapshot} autoResolving />);
    expect(screen.getByRole('button', { name: 'Hit · 要牌' })).toBeDisabled(); expect(screen.getByText(/服务端正在自动停牌/)).toBeVisible();
    view.rerender(<BlackjackGame {...base} snapshot={{ ...snapshot, legal_actions: ['STAND'] }} />);
    expect(screen.getByRole('button', { name: 'Hit · 要牌' })).toBeDisabled(); expect(screen.queryByRole('button', { name: /Double/ })).not.toBeInTheDocument();
});

it('shows supplied per-hand and net round results without promoting a partial hand win', () => {
    const terminal: BlackjackProjectionDTO = { ...snapshot, phase: 'SETTLED', active_hand_id: '', legal_actions: [], dealer_revealed: true, dealer_cards: [0, 5], dealer_total: { hard_total: 7, best_total: 17, is_soft: true }, total_payout_units: '10000000', net_change_units: '-1000000', result_class: 'LOSS', hands: snapshot.hands.map((h, i) => ({ ...h, result: i === 0 ? 'NORMAL_WIN' : 'LOSS', payout_units: i === 0 ? '10000000' : '0', net_change_units: i === 0 ? '4500000' : '-5500000' })) };
    render(<BlackjackGame {...base} snapshot={terminal} />);
    expect(screen.queryByLabelText('庄家暗牌，尚未公开')).not.toBeInTheDocument(); expect(screen.getByLabelText('庄家总点数')).toHaveTextContent('17');
    expect(screen.getByRole('status', { name: '本局结算' })).toHaveTextContent('净亏损');
    expect(screen.getByRole('status', { name: '本局结算' })).toHaveTextContent('-2');
    expect(screen.getByText('普通获胜')).toBeVisible(); expect(screen.getByRole('button', { name: 'Deal · 11 筹码' })).toBeEnabled();
});
