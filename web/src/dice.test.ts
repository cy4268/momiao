import { afterEach, expect, it, vi } from 'vitest';
import { resolveDice, rollDice, type DiceValues } from './dice';

afterEach(() => vi.restoreAllMocks());

it('classifies all 216 three-dice outcomes, including all six triples', () => {
    const counts = { BIG: 0, SMALL: 0, TRIPLE: 0 };
    for (let a = 1; a <= 6; a++) for (let b = 1; b <= 6; b++) for (let c = 1; c <= 6; c++) {
        const result = resolveDice([a, b, c]);
        expect(result.total).toBe(a + b + c);
        const expected = a === b && b === c ? 'TRIPLE' : a + b + c <= 10 ? 'SMALL' : 'BIG';
        expect(result.side).toBe(expected);
        counts[result.side]++;
    }
    expect(counts).toEqual({ BIG: 105, SMALL: 105, TRIPLE: 6 });
});

it('rejects malformed dice rather than displaying a made-up result', () => {
    for (const values of [[0, 2, 3], [7, 2, 3], [1.5, 2, 3], [NaN, 2, 3], [1, 2], [1, 2, 3, 4]]) {
        expect(() => resolveDice(values as DiceValues)).toThrow();
    }
});

it('rejects bytes 252..255 before mapping to six equal outcomes', () => {
    const bytes = [252, 253, 254, 255, 0, 1, 5];
    vi.spyOn(crypto, 'getRandomValues').mockImplementation(array => {
        (array as Uint8Array)[0] = bytes.shift()!;
        return array;
    });
    expect(rollDice()).toEqual({ dice: [1, 2, 6], total: 9, side: 'SMALL' });
    expect(bytes).toHaveLength(0);
});

it('reports unavailable randomness, with no Math.random fallback', () => {
    vi.spyOn(crypto, 'getRandomValues').mockImplementation(() => { throw new Error('unavailable'); });
    const fallback = vi.spyOn(Math, 'random');
    expect(() => rollDice()).toThrow();
    expect(fallback).not.toHaveBeenCalled();
});
