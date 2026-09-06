import { describe, expect, it, vi } from 'vitest';
import { ApiClient } from './api';
import { captureDiscordCallback, validateDiscordAuthorization } from './admission-api';

const user = { id: 12, username: 'native-readonly', display_name: 'Native', role: 1 };
const bundle = { access_token: 'synthetic-memory-token', access_expires_at: Date.now()/1000+3600, user, session: {sid:'synthetic-sid'} };
const ok = (data: unknown = {}) => new Response(JSON.stringify({success:true,data}));
const deferred = () => { let resolve!: (r: Response)=>void; const promise = new Promise<Response>(r=>{resolve=r;}); return {promise,resolve}; };
describe('native admission session boundary',()=>{
 it('waits for callback before logout and discards its late session',async()=>{
  const late=deferred(); const f=vi.fn().mockReturnValueOnce(late.promise).mockResolvedValueOnce(ok()); const c=new ApiClient(f);
  const callback=c.discordCallback({code:'synthetic-code',state:'synthetic-state'}); const logout=c.logout();
  expect(f).toHaveBeenCalledTimes(1); late.resolve(ok(bundle));
  const results=await Promise.allSettled([callback,logout]); expect(results[0].status).toBe('rejected'); expect(c.getSnapshot().user).toBeNull();
  expect(f.mock.calls[1][0]).toBe('/api/user/auth/logout');
 });
 it('keeps 2FA and proof in memory and never accepts proof as login',async()=>{
  const storage=vi.spyOn(Storage.prototype,'setItem'); const f=vi.fn().mockResolvedValueOnce(ok(bundle)).mockResolvedValueOnce(ok({require_2fa:true,flow_token:'synthetic-flow'})).mockResolvedValueOnce(ok({proof:'synthetic-proof',expires_at:4102444800})); const c=new ApiClient(f);
  await c.login('a','b'); const epoch=c.getSessionGeneration();
  expect(await c.discordCallback({code:'synthetic-code',state:'synthetic-state'})).toEqual({require_2fa:true,flow_token:'synthetic-flow'});
  expect(await c.admission2fa('synthetic-flow','123456')).toEqual({proof:'synthetic-proof',expires_at:4102444800});
  expect(c.getSessionGeneration()).toBe(epoch); expect(c.getSnapshot().user?.id).toBe(12); expect(storage).not.toHaveBeenCalled();
 });
 it('blocks overlapping login and admission completions',async()=>{
  const late=deferred(); const f=vi.fn().mockReturnValue(late.promise); const c=new ApiClient(f);
  const first=c.admission2fa('synthetic-flow','123456'); await expect(c.login('a','b')).rejects.toThrow(); await expect(c.admission2fa('synthetic-flow','123456')).rejects.toThrow();
  late.resolve(ok(bundle)); await first; expect(f).toHaveBeenCalledTimes(1);
 });
 it('updates the native password session while retaining verified user identity',async()=>{
  const f=vi.fn().mockResolvedValueOnce(ok(bundle)).mockResolvedValueOnce(ok({...bundle,user:undefined,session:{sid:'rotated'},has_password:true})); const c=new ApiClient(f); await c.login('a','b');
  await c.updatePassword('change',{password:'new-synthetic-password',old_password:'old-synthetic-password'});
  expect(c.getSnapshot().user?.username).toBe('native-readonly'); expect(f.mock.calls[1][0]).toBe('/api/momiao/account/password/change');
  expect(new Headers(f.mock.calls[1][1].headers).get('X-Auth-Session')).toBe('synthetic-sid');
 });
 it('does not replay a password write when the outcome is unknown',async()=>{
  const f=vi.fn().mockResolvedValueOnce(ok(bundle)).mockRejectedValueOnce(new TypeError('offline')); const c=new ApiClient(f); await c.login('a','b');
  await expect(c.updatePassword('set',{password:'new-synthetic-password',proof:'synthetic-proof'})).rejects.toMatchObject({uncertain:true}); expect(f).toHaveBeenCalledTimes(2);
 });
});
describe('callback capture and authorized destination',()=>{
 it('scrubs code and state before the first request and consumes once',()=>{
  const location={pathname:'/oauth/discord',search:'?code=synthetic-code&state=synthetic-state',hash:'#fragment'};
  const history={replaceState:vi.fn()};
  const data=captureDiscordCallback(location,history); expect(history.replaceState).toHaveBeenCalledWith(null,'','/oauth/discord');
  expect(data).toEqual({code:'synthetic-code',state:'synthetic-state'});
 });
 it.each(['?code=a&code=b&state=c','?code=a&state=b&purpose=registration','?state=b','?code=a','?error=denied&state=b'])('clears even invalid callback %s',search=>{
  const history={replaceState:vi.fn()}; expect(()=>captureDiscordCallback({pathname:'/oauth/discord',search,hash:''},history)).toThrow(); expect(history.replaceState).toHaveBeenCalledTimes(1);
 });
 it('allows only the actual Discord authorize endpoint and same-origin callback',()=>{
  const url='https://discord.com/oauth2/authorize?client_id=123456789012345678&redirect_uri=https%3A%2F%2Fportal.example%2Foauth%2Fdiscord&response_type=code&scope=identify&state=synthetic-state';
  expect(validateDiscordAuthorization(url,'https://portal.example')).toBe(url);
  for(const bad of [url.replace('discord.com','discord.com.evil.example'),url.replace('/oauth2/authorize','/invite'),url.replace('portal.example','other.example'),'javascript:alert(1)']) expect(()=>validateDiscordAuthorization(bad,'https://portal.example')).toThrow();
 });
});
