import { fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { DiceExperience } from './DiceExperience';
import * as dice from './dice';

beforeEach(() => sessionStorage.clear());
afterEach(() => vi.restoreAllMocks());

it('only rolls on explicit action, remembers this tab history and isolates accounts', () => {
    const roll = vi.spyOn(dice, 'rollDice').mockReturnValue({ dice: [2, 3, 4], total: 9, side: 'SMALL' });
    const fetcher = vi.spyOn(globalThis, 'fetch');
    const first = render(<DiceExperience userID="1" />);
    expect(roll).not.toHaveBeenCalled();
    expect(screen.getByText(/不扣除筹码，不发放奖励/)).toBeVisible();
    fireEvent.click(screen.getByRole('radio', { name: /猜小/ }));
    expect(roll).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: '模拟掷骰' }));
    expect(roll).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('status')).toHaveTextContent('9 点 · 小 · 猜中了');
    expect(within(screen.getByRole('table', { name: '本机体验记录' })).getAllByRole('row')).toHaveLength(2);
    expect(fetcher).not.toHaveBeenCalled();
    first.unmount();
    const second = render(<DiceExperience userID="1" />);
    expect(screen.getByRole('status')).toHaveTextContent('9 点');
    expect(roll).toHaveBeenCalledTimes(1);
    second.unmount();
    const other = render(<DiceExperience userID="2" />);
    expect(screen.getByText('还没有体验记录')).toBeVisible();
    other.unmount();
    render(<DiceExperience userID="1" />);
    fireEvent.click(screen.getByRole('button', { name: '清空本机记录' }));
    expect(screen.getByText('还没有体验记录')).toBeVisible();
    expect(sessionStorage.getItem('momiao.dice.experience.v1.1')).toBeNull();
});

it('explains triples and keeps only the latest 20 results', () => {
    vi.spyOn(dice, 'rollDice').mockReturnValue({ dice: [4, 4, 4], total: 12, side: 'TRIPLE' });
    render(<DiceExperience userID="1" />);
    for (let i = 0; i < 22; i++) fireEvent.click(screen.getByRole('button', { name: '模拟掷骰' }));
    expect(screen.getByRole('status')).toHaveTextContent('12 点 · 豹子 · 大小均未命中');
    expect(within(screen.getByRole('table', { name: '本机体验记录' })).getAllByRole('row')).toHaveLength(21);
});

it('degrades gracefully on blocked storage and never invents a result on random failure', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked'); });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('blocked'); });
    const roll = vi.spyOn(dice, 'rollDice').mockImplementation(() => { throw new Error('unavailable'); });
    render(<DiceExperience userID="1" />);
    fireEvent.click(screen.getByRole('button', { name: '模拟掷骰' }));
    expect(screen.getByRole('alert')).toHaveTextContent('随机源暂不可用');
    expect(screen.getByText('还没有体验记录')).toBeVisible();
    roll.mockReturnValue({ dice: [2, 3, 6], total: 11, side: 'BIG' });
    fireEvent.click(screen.getByRole('button', { name: '模拟掷骰' }));
    expect(screen.getByRole('status')).toHaveTextContent('11 点');
    expect(screen.getByText(/刷新后可能丢失/)).toBeVisible();
});

it('ignores malformed local records instead of trusting stored outcomes', () => {
    sessionStorage.setItem('momiao.dice.experience.v1.1', JSON.stringify([{ dice: [1, 1, 9], choice: 'BIG' }]));
    render(<DiceExperience userID="1" />);
    expect(screen.getByText('还没有体验记录')).toBeVisible();
    expect(screen.getByText(/记录格式异常/)).toBeVisible();
});
