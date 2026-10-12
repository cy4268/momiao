import { describe, expect, it, vi } from 'vitest';
import { captureDiscordCallback, validateDiscordAuthorization } from './admission-api';

describe('callback capture and authorized destination',()=>{
 it.each([
  ['code=synthetic-code', {code:'synthetic-code',state:'synthetic-state'}],
  ['error=access_denied', {error:'access_denied',state:'synthetic-state'}],
  ['error=access_denied&error_description=Cancelled', {error:'access_denied',error_description:'Cancelled',state:'synthetic-state'}],
 ])('accepts the exact Discord issuer without forwarding it: %s',(response,expected)=>{
  const history={replaceState:vi.fn()};
  expect(captureDiscordCallback({pathname:'/oauth/discord',search:`?${response}&state=synthetic-state&iss=https%3A%2F%2Fdiscord.com`,hash:''},history)).toEqual(expected);
  expect(history.replaceState).toHaveBeenCalledExactlyOnceWith(null,'','/oauth/discord');
 });
 it.each(['', 'https://evil.example', 'https://discord.com/', 'https://discord.com.evil.example', 'https://DISCORD.com', 'https://discord.com&iss=https://discord.com'])('rejects invalid or duplicate issuers: %s',issuer=>{
  for (const response of ['code=synthetic-code','error=access_denied']) {
   const history={replaceState:vi.fn()};
   expect(()=>captureDiscordCallback({pathname:'/oauth/discord',search:`?${response}&state=synthetic-state&iss=${issuer}`,hash:''},history)).toThrow();
   expect(history.replaceState).toHaveBeenCalledExactlyOnceWith(null,'','/oauth/discord');
  }
 });
 it.each(['code=a&state=b&state=c','code=a&code=b&state=c','code=a&state=b&extra=c','error=access_denied&code=a&state=b','error=access_denied&state=b&error=access_denied','code=a','state=b'])('keeps callback validation with a valid issuer: %s',response=>{
  expect(()=>captureDiscordCallback({pathname:'/oauth/discord',search:`?${response}&iss=https://discord.com`,hash:''},{replaceState:vi.fn()})).toThrow();
 });
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
  const url='https://discord.com/oauth2/authorize?client_id=123456789012345678&redirect_uri=https%3A%2F%2Fportal.example%2Foauth%2Fdiscord&response_type=code&scope=identify&state=synthetic-state&prompt=consent';
  expect(validateDiscordAuthorization(url,'https://portal.example')).toBe(url);
  for(const bad of [url.replace('discord.com','discord.com.evil.example'),url.replace('/oauth2/authorize','/invite'),url.replace('portal.example','other.example'),'javascript:alert(1)',url.replace('prompt=consent','prompt=none'),url+'&prompt=consent']) expect(()=>validateDiscordAuthorization(bad,'https://portal.example')).toThrow();
 });
});
