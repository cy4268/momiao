import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import { readFileSync } from 'node:fs';
export default defineConfig({ plugins: [react(), {
    name: 'upstream-notices',
    generateBundle() {
        for (const name of ['LICENSE', 'NOTICE', 'THIRD-PARTY-LICENSES.md']) {
            this.emitFile({ type: 'asset', fileName: `notices/new-api/${name}.txt`, source: readFileSync(new URL(`../docs/upstream/new-api/${name}`, import.meta.url)) });
        }
    },
}], test: { environment: 'jsdom', setupFiles: ['./src/test-setup.ts'], restoreMocks: true } });
