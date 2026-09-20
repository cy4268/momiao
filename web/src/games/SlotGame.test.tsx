import { useState } from 'react';
import { act, fireEvent, render, screen, within } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import { SlotGame, formatChipUnits, type SlotResultDTO } from './SlotGame';

const result: SlotResultDTO = {
    stops: [0, 0, 0, 0, 0], full_grid: [['L1', 'L3', 'L2'], ['M2', 'L2', 'M2'], ['W', 'L2', 'L3'], ['L2', 'M1', 'L3'], ['L3', 'L1', 'H2']],
    lines: Array.from({ length: 10 }, (_, i) => ({ line_number: i + 1, interpreted_symbol: i === 4 || i === 9 ? 'L2' : '', match_length: i === 4 || i === 9 ? 3 : 0, multiplier: i === 4 || i === 9 ? 8 : 0, line_stake_units: '550000', line_payout_units: i === 4 || i === 9 ? '4400000' : '0' })),
    total_wager_units: '5500000', line_stake_units: '550000', total_payout_units: '8800000', net_change_units: '3300000', result_class: 'WIN', result_detail: 'WIN',
};

it('edits only the controlled wager, validates whole chips and submits exact units once', async () => {
    let finish!: () => void; const spin = vi.fn(() => new Promise<void>(resolve => { finish = resolve; }));
    function Fixture() { const [wager, setWager] = useState('10'); return <SlotGame availableUnits="50000000" wagerChips={wager} onWagerChange={setWager} busy={false} result={null} onSpin={spin} />; }
    render(<Fixture />);
    expect(spin).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: '100 筹码' })); expect(spin).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: '500 筹码' })).toBeDisabled();
    const input = screen.getByLabelText('总下注 · 筹码');
    for (const value of ['9', '10.5', '1e3', '101', '-10', '999999999999999999999999']) { fireEvent.change(input, { target: { value } }); expect(screen.getByRole('button', { name: /Spin/ })).toBeDisabled(); }
    fireEvent.change(input, { target: { value: '11' } });
    expect(screen.getByText('每线 1.1 筹码')).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Spin · 11 筹码' }));
    fireEvent.submit(input.closest('form')!); expect(spin).toHaveBeenCalledTimes(1); expect(spin).toHaveBeenCalledWith('5500000');
    await act(async () => finish());
    expect(screen.getByText('等待第一局结果')).toBeVisible();
});

it('renders only the supplied grid, exposes ten selectable lines and never spins for replay', () => {
    const spin = vi.fn(async () => {});
    const view = render(<SlotGame availableUnits="50000000" wagerChips="11" onWagerChange={() => {}} busy={false} result={result} roundID="round-slot-1" rulesetVersion="slot-rules-v3" onSpin={spin} />);
    expect(screen.getAllByTestId('slot-cell')).toHaveLength(15);
    expect(screen.getByLabelText('第 1 轴，中行：L3')).toBeVisible();
    expect(within(screen.getByRole('group', { name: '查看固定中奖线' })).getAllByRole('button')).toHaveLength(10);
    fireEvent.click(screen.getByRole('button', { name: '查看第 5 线' }));
    expect(screen.getByRole('status', { name: '当前中奖线' })).toHaveTextContent('L2 · 3 连');
    expect(screen.getByRole('status', { name: '当前中奖线' })).toHaveTextContent('8.8 筹码');
    fireEvent.click(screen.getByRole('button', { name: '回看本局结果' })); expect(spin).not.toHaveBeenCalled();
    expect(screen.getByText(/仅回看相同盘面/)).toBeVisible();
    expect(screen.getByRole('status', { name: '本局结算' })).toHaveTextContent('净盈利');
    const currentPaytable = screen.getByRole('table', { name: 'slot-paytable-v3 奖励倍率表' });
    expect(within(currentPaytable).getByRole('row', { name: 'L1 11× 12× 15×' })).toBeInTheDocument();
    expect(screen.getByText(/slot-strips-v2 \/ slot-paylines-v1/)).toBeInTheDocument();
    view.rerender(<SlotGame availableUnits="50000000" wagerChips="11" onWagerChange={() => {}} busy={false} result={result} roundID="round-slot-1" rulesetVersion="slot-rules-v1" onSpin={spin} />);
    expect(within(screen.getByRole('table', { name: 'slot-paytable-v1 奖励倍率表' })).getByRole('row', { name: 'L2 8× 25× 80×' })).toBeInTheDocument();
});

it('keeps partial payout a net loss and blocks while recovering or unavailable', () => {
    const recovery = vi.fn(); const spin = vi.fn(async () => {});
    const view = render(<SlotGame availableUnits="50000000" wagerChips="11" onWagerChange={() => {}} busy={false} result={{ ...result, total_payout_units: '2000000', net_change_units: '-3500000', result_class: 'LOSS', result_detail: 'PARTIAL_RETURN' }} recovering onRecover={recovery} onSpin={spin} error="连接中断，请核对同一局。" />);
    expect(screen.getByRole('status', { name: '本局结算' })).toHaveTextContent('部分返还 · 净亏损');
    expect(screen.getByRole('button', { name: /Spin/ })).toBeDisabled();
    expect(screen.getByRole('alert')).toHaveTextContent('连接中断');
    fireEvent.click(screen.getByRole('button', { name: '核对本局状态' })); expect(recovery).toHaveBeenCalledTimes(1); expect(spin).not.toHaveBeenCalled();
    view.rerender(<SlotGame availableUnits={null} wagerChips="11" onWagerChange={() => {}} busy={false} result={null} onSpin={spin} />);
    expect(screen.getByRole('button', { name: /Spin/ })).toBeDisabled();
});

it('formats all atomic-unit digits exactly without Number conversion', () => {
    expect(formatChipUnits('13750000')).toBe('27.5');
    expect(formatChipUnits('1')).toBe('0.000002');
    expect(formatChipUnits('9223372036854775807')).toBe('18446744073709.551614');
    expect(formatChipUnits('-3500000', true)).toBe('-7');
    expect(formatChipUnits('3500000', true)).toBe('+7');
});
