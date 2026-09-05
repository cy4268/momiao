export type DiceSide = 'BIG' | 'SMALL' | 'TRIPLE';
export type DiceChoice = Exclude<DiceSide, 'TRIPLE'>;
export type DiceValues = [number, number, number];
export type DiceResult = { dice: DiceValues; total: number; side: DiceSide };

export function resolveDice(dice: DiceValues): DiceResult {
    if (dice.length !== 3 || dice.some(n => !Number.isInteger(n) || n < 1 || n > 6)) throw new Error('Invalid dice');
    const total = dice.reduce((sum, n) => sum + n, 0);
    const side = dice.every(n => n === dice[0]) ? 'TRIPLE' : total <= 10 ? 'SMALL' : 'BIG';
    return { dice: [...dice], total, side };
}

export function rollDice(): DiceResult {
    const dice: number[] = [];
    const byte = new Uint8Array(1);
    // 252 is divisible by six; discard the four remaining byte values.
    for (let attempts = 0; dice.length < 3 && attempts < 128; attempts++) {
        crypto.getRandomValues(byte);
        if (byte[0] < 252) dice.push(byte[0] % 6 + 1);
    }
    if (dice.length !== 3) throw new Error('Random source failed');
    return resolveDice(dice as DiceValues);
}
