import { describe, expect, it, vi } from 'vitest';
import { ApiClient } from '../api';
import { capturePokerReadScope, readPokerHttpTable, submitPokerHttpCommand } from './poker-http';
import self from './fixtures/g3-player-self.json';

const user={id:910002,username:'synthetic',display_name:'Synthetic 910002',role:1};
const bundle={access_token:'synthetic-memory-token',access_expires_at:4102444800,user,session:{sid:'synthetic-auth-session'}};
const ok=(data:unknown)=>new Response(JSON.stringify({success:true,data}),{headers:{'Content-Type':'application/json'}});
const request='request-http-0001',reservation='019a0000-0000-7000-8000-000000000011';
const receipt=(status='PENDING')=>({table_id:self.table_id,session_id:self.viewer.session_id,status,table_version:'8',duplicate:false,amount_units:'500000'});
const httpView=()=>{const v:any=structuredClone(self);v.viewer.can_act=false;delete v.viewer.legal;return v;};
async function setup(){const fetcher=vi.fn().mockResolvedValueOnce(ok(bundle)),client=new ApiClient(fetcher);await client.login('synthetic','synthetic');const scope=capturePokerReadScope(client,self.table_id,'PLAYER_SELF',1);return {fetcher,client,scope};}

describe('Poker stable HTTP adapter through existing ApiClient',()=>{
  it('uses authenticated native headers and same-origin/no-store table GET, not a copied bearer or viewer query',async()=>{
    const {fetcher,client,scope}=await setup();fetcher.mockResolvedValueOnce(ok(httpView()));const view=await readPokerHttpTable(client,scope,()=>scope);
    expect(view).toEqual(httpView());expect(fetcher.mock.calls[1][0]).toBe(`/api/v1/poker/tables/${self.table_id}`);
    const init=fetcher.mock.calls[1][1],headers=new Headers(init.headers);expect(init).toMatchObject({method:'GET',credentials:'same-origin',cache:'no-store'});
    expect(headers.get('Authorization')).toBe('Bearer synthetic-memory-token');expect(headers.get('X-Auth-Session')).toBe('synthetic-auth-session');expect(headers.get('New-Api-User')).toBe('910002');expect(init.body).toBeUndefined();
  });
  it('ordinary HTTP has no socket grant and rejects an actionable or invented-control projection',async()=>{
    const {fetcher,client,scope}=await setup();fetcher.mockResolvedValueOnce(ok(self));await expect(readPokerHttpTable(client,scope,()=>scope)).rejects.toThrow();
    const v=httpView();v.viewer.control={connection_id:'a'.repeat(32),session_id:self.viewer.session_id,mode:'READ_ONLY',control_epoch:'1'};fetcher.mockResolvedValueOnce(ok(v));await expect(readPokerHttpTable(client,scope,()=>scope)).rejects.toThrow();
  });
  it('explicit takeover names the current server socket and receives only a durable receipt, never a live grant',async()=>{
    const {fetcher,client,scope}=await setup(),connection_id='a'.repeat(32);fetcher.mockResolvedValueOnce(ok(receipt('EPOCH_ADVANCED')));
    const result=await submitPokerHttpCommand(client,{type:'takeover',request_id:request,session_id:self.viewer.session_id,connection_id},scope,()=>scope);
    expect(fetcher.mock.calls[1][0]).toBe(`/api/v1/poker/sessions/${self.viewer.session_id}/take-over`);expect(JSON.parse(fetcher.mock.calls[1][1].body)).toEqual({request_id:request,connection_id});expect(result?.status).toBe('EPOCH_ADVANCED');expect(result).not.toHaveProperty('control');expect(fetcher).toHaveBeenCalledTimes(2);
    await expect(submitPokerHttpCommand(client,{type:'takeover',request_id:request,session_id:self.viewer.session_id,connection_id:'B'.repeat(32)},scope,()=>scope)).rejects.toMatchObject({uncertain:false});expect(fetcher).toHaveBeenCalledTimes(2);
  });
  it('stops unauthenticated/invalid identity and path scopes before a protected request',async()=>{
    const f=vi.fn(),client=new ApiClient(f);expect(()=>capturePokerReadScope(client,self.table_id,'PLAYER_SELF',1)).toThrow();expect(f).not.toHaveBeenCalled();
    const {fetcher,client:c}=await setup();for(const [id,generation] of [['/other',1],[self.table_id,-1],[self.table_id,1.5]] as const)expect(()=>capturePokerReadScope(c,id,'PLAYER_SELF',generation)).toThrow();expect(fetcher).toHaveBeenCalledTimes(1);
  });
  it('drops a late HTTP viewer response before decoding after the table request changes',async()=>{
    const {fetcher,client,scope}=await setup();let resolve!:(r:Response)=>void;fetcher.mockReturnValueOnce(new Promise<Response>(r=>resolve=r));let current=scope;
    const result=readPokerHttpTable(client,scope,()=>current);current={...scope,request_generation:2};resolve(ok({private_secret:'not a projection'}));expect(await result).toBeUndefined();
    expect(fetcher).toHaveBeenCalledTimes(2);
  });
  it('drops late reads after logout and never restores another account private cards',async()=>{
    const {fetcher,client,scope}=await setup();let resolve!:(r:Response)=>void;fetcher.mockReturnValueOnce(new Promise<Response>(r=>resolve=r)).mockResolvedValueOnce(ok({}));
    const result=readPokerHttpTable(client,scope,()=>scope);await client.logout();resolve(ok(self));expect(await result).toBeUndefined();expect(client.getSnapshot().user).toBeNull();
  });
  it.each([
    [{type:'reserve',request_id:request,seat_no:3},`/tables/${self.table_id}/seat-reservations`,{request_id:request,seat_no:3}],
    [{type:'buyin',request_id:request,reservation_id:reservation,amount_units:'9007199255000000'},`/tables/${self.table_id}/buy-ins`,{request_id:request,reservation_id:reservation,amount_units:'9007199255000000'}],
    [{type:'topup',request_id:request,session_id:self.viewer.session_id,amount_units:'500000'},`/sessions/${self.viewer.session_id}/top-ups`,{request_id:request,amount_units:'500000'}],
    [{type:'leave',request_id:request,session_id:self.viewer.session_id},`/sessions/${self.viewer.session_id}/safe-leave`,{request_id:request}],
  ])('binds exact stable command %s and preserves receipt rather than manufacturing a view',async(command,path,body)=>{
    const {fetcher,client,scope}=await setup();fetcher.mockResolvedValueOnce(ok(receipt()));
    const result=await submitPokerHttpCommand(client,command as Parameters<typeof submitPokerHttpCommand>[1],scope,()=>scope);expect(result?.status).toBe('PENDING');
    const call=fetcher.mock.calls[1];expect(call[0]).toBe(`/api/v1/poker${path}`);expect(call[1].method).toBe('POST');expect(JSON.parse(call[1].body)).toEqual(body);expect(fetcher).toHaveBeenCalledTimes(2);
  });
  it('rejects malformed inputs and stale current scope before any POST, without guessing unsupported commands',async()=>{
    const {fetcher,client,scope}=await setup();
    for(const command of [{type:'reserve',request_id:request,seat_no:0},{type:'reserve',request_id:'short',seat_no:3},{type:'buyin',request_id:request,reservation_id:'bad',amount_units:'500000'},{type:'topup',request_id:request,session_id:self.viewer.session_id,amount_units:'500001'},{type:'topup',request_id:request,session_id:self.viewer.session_id,amount_units:500000},{type:'leave',request_id:request,session_id:'bad'},{type:'takeover',request_id:request,session_id:self.viewer.session_id}])await expect(submitPokerHttpCommand(client,command as never,scope,()=>scope)).rejects.toMatchObject({uncertain:false});
    await expect(submitPokerHttpCommand(client,{type:'reserve',request_id:request,seat_no:3},scope,()=>({...scope,request_generation:2}))).rejects.toMatchObject({uncertain:false});expect(fetcher).toHaveBeenCalledTimes(1);
  });
  it('never replays POST; all standalone HTTP failures remain unknown rather than no-effect claims',async()=>{
    for(const status of [400,403,409,500,503]){
      const {fetcher,client,scope}=await setup();fetcher.mockResolvedValueOnce(new Response(JSON.stringify({success:false,code:'POKER_COMMAND_DENIED',message:'Synthetic failure'}),{status}));
      await expect(submitPokerHttpCommand(client,{type:'topup',request_id:request,session_id:self.viewer.session_id,amount_units:'500000'},scope,()=>scope)).rejects.toMatchObject({uncertain:true,status,code:'POKER_COMMAND_DENIED'});expect(fetcher).toHaveBeenCalledTimes(2);
    }
  });
  it.each(['network','not-json'])('keeps %s POST outcomes unknown and redacted with exactly one send',async(mode)=>{
    const {fetcher,client,scope}=await setup();
    if(mode==='network')fetcher.mockRejectedValueOnce(new Error('private-upstream-secret'));else fetcher.mockResolvedValueOnce(new Response('<html>private-upstream-secret</html>',{status:502}));
    try{await submitPokerHttpCommand(client,{type:'leave',request_id:request,session_id:self.viewer.session_id},scope,()=>scope);throw Error('unexpected success');}catch(error){expect(error).toMatchObject({uncertain:true});expect(String(error)).not.toContain('private-upstream-secret');}
    expect(fetcher).toHaveBeenCalledTimes(2);
  });
  it('ignores a late mutation receipt after a view switch without calling it no-effect',async()=>{
    const {fetcher,client,scope}=await setup();let resolve!:(r:Response)=>void;fetcher.mockReturnValueOnce(new Promise<Response>(r=>resolve=r));let current=scope;
    const result=submitPokerHttpCommand(client,{type:'leave',request_id:request,session_id:self.viewer.session_id},scope,()=>current);current={...scope,request_generation:2};resolve(ok({private_secret:'invalid receipt from an old view'}));expect(await result).toBeUndefined();expect(fetcher).toHaveBeenCalledTimes(2);
  });
  it('treats malformed/mismatched success receipts as uncertain and preserves the original no-effect receipt when valid',async()=>{
    const {fetcher,client,scope}=await setup(),command={type:'leave' as const,request_id:request,session_id:self.viewer.session_id};
    for(const bad of [{...receipt(),raw_deck:[0]},{...receipt(),session_id:reservation},{...receipt(),table_id:reservation},{...receipt(),table_version:8}]){fetcher.mockResolvedValueOnce(ok(bad));await expect(submitPokerHttpCommand(client,command,scope,()=>scope)).rejects.toMatchObject({uncertain:true});}
    fetcher.mockResolvedValueOnce(ok(receipt('FAILED_NO_EFFECT')));expect((await submitPokerHttpCommand(client,command,scope,()=>scope))?.status).toBe('FAILED_NO_EFFECT');
  });
});
