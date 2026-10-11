import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { expect, it } from 'vitest';

const root = 'src';
function files(dir: string): string[] {
    return readdirSync(dir).flatMap(name => {
        const path = join(dir, name);
        return statSync(path).isDirectory() ? files(path) : path.endsWith('.tsx') ? [path] : [];
    });
}
const rel = (path: string) => relative(root, path).replaceAll('\\', '/');
// Operator and admin-only screens keep their system vocabulary.
const playerFiles = files(root).filter(path => !/^(ops\/|Ops|Channels)/.test(rel(path)) && !path.endsWith('.test.tsx'));

it('player pages use plain names instead of internal system vocabulary', () => {
    const banned = ['原生', '短 ID', 'Reserve + Active', '最小步长'];
    const hits = playerFiles.flatMap(path => readFileSync(path, 'utf8').split('\n').flatMap((line, index) =>
        banned.filter(word => line.includes(word)).map(word => `${rel(path)}:${index + 1} ${word}`)));
    expect(hits).toEqual([]);
});
