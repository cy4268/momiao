import { it, expect, vi } from 'vitest';
import { ApiClient } from './api';
const user = { id: 1, username: 'test', display_name: 'Test', role: 100, group: 'team' };
const ok = (data?: unknown) => new Response(JSON.stringify({ success: true, data }));
async function setup(response: Response) {
 const f = vi.fn().mockResolvedValueOnce(ok({authentication_state:'ANONYMOUS',csrf_token:'anon'}))
  .mockResolvedValueOnce(ok({authentication_state:'AUTHENTICATED',csrf_token:'local'}))
  .mockResolvedValueOnce(ok(user)).mockResolvedValueOnce(response);
 const c = new ApiClient(f); await c.login('a', 'b'); return { c, f };
}
it('reads the local user group after password login', async () => {
 const { c, f } = await setup(ok({ team: { ratio: 1, desc: 'Team' } }));
 expect(c.getSnapshot().user?.group).toBe('team');
 expect(await c.groups()).toEqual({ team: { ratio: 1, desc: 'Team' } });
 expect(f.mock.lastCall?.[0]).toBe('/api/user/self/groups');
 expect(f.mock.calls.map(call => call[0])).toEqual(['/api/v1/session/bootstrap','/api/v1/auth/password/login','/api/user/self','/api/user/self/groups']);
});
