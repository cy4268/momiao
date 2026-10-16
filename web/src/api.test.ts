import { it, expect, vi } from 'vitest';
import { ApiClient } from './api';
const user = { id: 12, username: 'test-user', display_name: 'Test', role: 1, quota: 80 };
const anonymous = { authentication_state: 'ANONYMOUS', csrf_token: 'anonymous-csrf' };
const authenticated = { authentication_state: 'AUTHENTICATED', csrf_token: 'session-csrf' };
const ok = (data?: unknown) => new Response(JSON.stringify({ success: true, data }));
const failure = (status = 401) => new Response(JSON.stringify({ success: false, message: 'test failure' }), { status });
const deferred = () => { let resolve!: (value: Response) => void; const promise = new Promise<Response>(r => resolve = r); return { promise, resolve }; };

it('uses local cookie login, self, game CSRF and logout without native credentials', async () => {
 const f = vi.fn().mockResolvedValueOnce(ok(anonymous)).mockResolvedValueOnce(ok(authenticated)).mockResolvedValueOnce(ok(user))
  .mockResolvedValueOnce(ok({done:true})).mockResolvedValueOnce(ok(authenticated)).mockResolvedValueOnce(ok(anonymous));
 const c = new ApiClient(f); await c.bootstrap(); await c.login('a', 'b');
 expect(c.getSnapshot().user?.id).toBe(12);
 await c.request('/api/v1/games/dice', 'POST', {}, {'X-CSRF-Token':'game-csrf'});
 const login = new Headers(f.mock.calls[1][1].headers), game = new Headers(f.mock.calls[3][1].headers);
 expect(login.get('X-CSRF-Token')).toBe('anonymous-csrf'); expect(game.get('X-CSRF-Token')).toBe('session-csrf');
 expect(game.get('X-Game-CSRF-Token')).toBe('game-csrf');
 await c.logout(); expect(c.getSnapshot().user).toBeNull();
 expect(f.mock.calls.map(([p])=>p)).toEqual(['/api/v1/session/bootstrap','/api/v1/auth/password/login','/api/user/self','/api/v1/games/dice','/api/v1/session/bootstrap','/api/v1/session/logout']);
 for (const [,init] of f.mock.calls) { const h=new Headers(init.headers); expect(init.credentials).toBe('same-origin'); for(const name of ['Authorization','New-Api-User','X-Auth-Session']) expect(h.has(name)).toBe(false); }
});
it('does not replay a rejected mutation or keep an expired login', async () => {
 const f=vi.fn().mockResolvedValueOnce(ok(authenticated)).mockResolvedValueOnce(ok(user)).mockResolvedValueOnce(failure());
 const c=new ApiClient(f);await c.bootstrap();await expect(c.request('/api/token/','POST',{})).rejects.toThrow('test failure');
 expect(f).toHaveBeenCalledTimes(3);expect(c.getSnapshot().user).toBeNull();
});
it('keeps two-factor login separate from an authenticated session', async () => {
 const f=vi.fn().mockResolvedValueOnce(ok(anonymous)).mockResolvedValueOnce(ok({authentication_state:'TWO_FA',csrf_token:'anonymous-csrf',challenge:{challenge_id:'flow',expires_at:'2099-01-01T00:00:00Z'}})).mockResolvedValueOnce(ok(authenticated)).mockResolvedValueOnce(ok(user));
 const c=new ApiClient(f);await c.bootstrap();expect(await c.login('a','b')).toMatchObject({require_2fa:true,flow_token:'flow'});expect(c.getSnapshot().user).toBeNull();
 await c.verify2fa('flow','123456');expect(c.getSnapshot().user?.id).toBe(12);expect(JSON.parse(f.mock.calls[2][1].body)).toEqual({challenge_id:'flow',code:'123456'});
});
it('logout waits for cookie rotation and a late login never restores the user', async () => {
 const pending=deferred();const f=vi.fn().mockResolvedValueOnce(ok(anonymous)).mockReturnValueOnce(pending.promise).mockResolvedValueOnce(ok(authenticated)).mockResolvedValueOnce(ok(anonymous));
 const c=new ApiClient(f);await c.bootstrap();const login=c.login('a','b');const logout=c.logout();pending.resolve(ok(authenticated));
 await Promise.allSettled([login,logout]);expect(c.getSnapshot().user).toBeNull();expect(f.mock.calls.at(-1)?.[0]).toBe('/api/v1/session/logout');
});
it('late protected response after logout is discarded', async () => {
 const pending=deferred();const f=vi.fn().mockResolvedValueOnce(ok(authenticated)).mockResolvedValueOnce(ok(user)).mockReturnValueOnce(pending.promise).mockResolvedValueOnce(ok(authenticated)).mockResolvedValueOnce(ok(anonymous));
 const c=new ApiClient(f);await c.bootstrap();const request=c.request('/api/user/self');await c.logout();pending.resolve(ok(user));await expect(request).rejects.toThrow();expect(c.getSnapshot().user).toBeNull();
});
it('reports HTTP success:false and failed writes without automatically retrying',async()=>{
 const f=vi.fn().mockResolvedValueOnce(ok(anonymous)).mockResolvedValueOnce(failure(400));const c=new ApiClient(f);await c.bootstrap();await expect(c.login('a','b')).rejects.toThrow('test failure');expect(f).toHaveBeenCalledTimes(2);
 const probes=vi.fn().mockResolvedValueOnce(ok(authenticated)).mockResolvedValueOnce(ok(user)).mockImplementation(async()=>ok({ok:false,status_code:429,message:'quota exceeded'}));const probeClient=new ApiClient(probes);await probeClient.bootstrap();const timeout=vi.spyOn(AbortSignal,'timeout');
 expect(await probeClient.request('/api/v1/ops/channels/25/test','POST',{})).toMatchObject({status_code:429,message:'quota exceeded'});expect(timeout).toHaveBeenLastCalledWith(360000);
 await probeClient.request('/api/v1/ops/channels/25/models','POST',{});expect(timeout).toHaveBeenLastCalledWith(25000);
 await probeClient.request('/api/v1/ops/channels/25/test','GET');expect(timeout).toHaveBeenLastCalledWith(25000);
});
