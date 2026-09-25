import { it, expect, vi } from 'vitest';
import { ApiClient } from './api';
const user = { id: 1, username: 'test', display_name: 'Test', role: 100, group: 'team' };
const bundle = { access_token: 'memory', access_expires_at: 9999999999, session: { sid: 'sid' }, user };
const ok = (data?: unknown) => new Response(JSON.stringify({ success: true, data }));
async function setup(response: Response) { const f = vi.fn().mockResolvedValueOnce(ok(bundle)).mockResolvedValueOnce(response); const c = new ApiClient(f); await c.login('a', 'b'); return { c, f }; }
it('validates actual groups and preserves user group', async () => {
 const { c, f } = await setup(ok({ team: { ratio: 1, desc: 'Team' } }));
 expect(c.getSnapshot().user?.group).toBe('team');
 expect(await c.groups()).toEqual({ team: { ratio: 1, desc: 'Team' } });
 expect(f.mock.lastCall?.[0]).toBe('/api/user/self/groups');
 f.mockResolvedValueOnce(ok({ team: { ratio: 1, desc: 7 } }));
 await expect(c.groups()).rejects.toThrow();
});
