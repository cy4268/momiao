import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { expect, it } from 'vitest';

const root = 'src';
function files(dir: string, ext: string): string[] {
    return readdirSync(dir).flatMap(name => {
        const path = join(dir, name);
        return statSync(path).isDirectory() ? files(path, ext) : path.endsWith(ext) ? [path] : [];
    });
}
const rel = (path: string) => relative(root, path).replaceAll('\\', '/');
const isOps = (path: string) => rel(path).startsWith('ops/');

// Selectors that style buttons by role. Only styles.css may give them an appearance.
const buttonish = /(^|[\s>+~(,])(button|\.button|\.primary|\.game-play|\.extra-primary|\.pk-primary|\.roulette-primary)(?![\w-])/;
const appearance = /(^|;)\s*(background(-color|-image)?|border(-radius|-color)?|color|box-shadow|outline(-offset)?|text-shadow|height|min-height|font(-size|-weight)?)\s*:/;
// Controls that are deliberately not buttons visually: chips, choice cards, tabs, mobile nav, the shared header.
// Illustrated game controls (art tiles, action cards, line markers, mascot controls) keep their own designs.
const allowed = ['.mobile-nav', '.site-', '.game-choice', '.spirit-', '.crystal-', '.wager-quick', '[aria-pressed', '[aria-selected', '[role="tab"]', '.icon-button', '.close', 'pk-seat', '.scratch-cell',
    '.roulette-shots', '.roulette-item', '.roulette-pressure-actions', '.roulette-steal-options', '.extra-line-markers', '.extra-main-actions', '.extra-add-actions', '.pk-action-buttons', '.pk-bet-side', '.guangxi-controls'];

// The single source of button appearance, defined once in styles.css.
const baseSelectors = new Set(['button', '.button', 'input', 'select', 'textarea', 'button:hover:not(:disabled)', '.button:hover', 'button:focus-visible', '.button:focus-visible', 'button:disabled',
    '.primary', '.game-play', '.extra-primary', '.pk-primary', '.roulette-primary', ':is(.primary,.game-play,.extra-primary,.pk-primary,.roulette-primary):hover:not(:disabled)', 'button[aria-pressed="true"]', '.quiet']);
function splitSelectors(list: string) {
    const out: string[] = []; let depth = 0, current = '';
    for (const ch of list) {
        if (ch === '(' || ch === '[') depth++;
        if (ch === ')' || ch === ']') depth--;
        if (ch === ',' && depth === 0) { out.push(current.trim()); current = ''; } else current += ch;
    }
    return [...out, current.trim()].filter(Boolean);
}
function rules(css: string) {
    return [...css.replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/([^{}@]+)\{([^{}]*)\}/g)].map(m => ({ selector: m[1].trim(), body: m[2] }));
}

it('no page stylesheet restyles buttons', () => {
    const violations = files(root, '.css')
        .filter(path => !isOps(path) && rel(path) !== 'site-header.css')
        .flatMap(path => rules(readFileSync(path, 'utf8'))
            .flatMap(rule => splitSelectors(rule.selector).map(selector => ({ selector, body: rule.body })))
            .filter(({ selector, body }) => buttonish.test(selector) && appearance.test(body) && !allowed.some(a => selector.includes(a)))
            .filter(({ selector }) => !(rel(path) === 'styles.css' && baseSelectors.has(selector)))
            .map(({ selector }) => rel(path) + ': ' + selector));
    expect(violations).toEqual([]);
});

it('no decorative arrows in player-facing labels', () => {
    const offenders = files(root, '.tsx')
        .filter(path => !isOps(path) && !path.endsWith('.test.tsx') && !rel(path).startsWith('Channels'))
        .flatMap(path => readFileSync(path, 'utf8').split('\n').flatMap((line, index) =>
            /\s→\s*(<\/|'\}|'\)|<span aria-hidden)|<span aria-hidden="true">→<\/span>/.test(line) ? [rel(path) + ':' + (index + 1)] : []));
    expect(offenders).toEqual([]);
});
