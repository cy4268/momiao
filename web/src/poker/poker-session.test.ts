import { describe, expect, it, vi } from 'vitest';
import { ApiClient } from '../api';
import { capturePokerReadScope } from './poker-http';
import { parsePokerSession, readPokerHttpSession } from './poker-session';
import self from './fixtures/g3-player-self.json';

const table=self.table_id,session=self.viewer.session_id,other='019a0000-0000-7000-8000-000000000011';
const active=()=>({session_id:session,table_id:table,table_name:'原会话牌桌',state:'ACTIVE',seat_no:2,
  stack_units:'9007199255000000',committed_units:'500000',poker_in_play_units:'9007199255500000',control_epoch:'3',
  initial_buy_in_units:'9007199256000000',total_top_up_units:'500000',started_at:'2026-09-06T00:00:00Z'});
const settled=()=>({...active(),state:'SETTLED',stack_units:'0',committed_units:'0',poker_in_play_units:'0',
  final_cash_out_units:'9007199255500000',realized_pl_units:'-1000000',ended_at:'2026-09-06T00:10:00Z',end_reason:'SAFE_LEAVE'});
const ok=(data:unknown)=>new Response(JSON.stringify({success:true,data}),{headers:{'Content-Type':'application/json'}});
async function setup(){const fetcher=vi.fn().mockResolvedValueOnce(ok({access_token:'synthetic-session-token',access_expires_at:4102444800,user:{id:910002,username:'synthetic',role:1},session:{sid:'synthetic-session-sid'}}));
  const client=new ApiClient(fetcher);await client.login('synthetic','synthetic');const scope=capturePokerReadScope(client,table,'PLAYER_SELF',1);return {fetcher,client,scope};}

describe('Original owner Session cash-out boundary',()=>{
  it('reads exact active exposure and detached settled cash-out without Number rounding or socket authority',()=>{
    const input=settled(),result=parsePokerSession(input,table,session);expect(result).toEqual(input);input.final_cash_out_units='0';expect(result.final_cash_out_units).toBe('9007199255500000');
    expect(parsePokerSession(active(),table,session).state).toBe('ACTIVE');expect(parsePokerSession({...active(),state:'NEEDS_REVIEW'},table,session).state).toBe('NEEDS_REVIEW');
    expect(result).not.toHaveProperty('control');expect(result).not.toHaveProperty('can_act');
  });
  it('requires the original Session and table, never a newer session or a safe-leave acknowledgement',()=>{
    for(const bad of [{...settled(),session_id:other},{...settled(),table_id:other},{table_id:table,session_id:session,status:'CONFIRMED',table_version:'8',duplicate:false}])expect(()=>parsePokerSession(bad,table,session)).toThrow();
    expect(()=>parsePokerSession(settled(),table.toUpperCase(),session)).toThrow();
  });
  it('rejects incomplete or contradictory terminal settlement instead of enabling exit',()=>{
    for(const field of ['final_cash_out_units','realized_pl_units','ended_at','end_reason']){const bad:any=settled();delete bad[field];expect(()=>parsePokerSession(bad,table,session),field).toThrow();}
    for(const patch of [{stack_units:'500000',poker_in_play_units:'500000'},{committed_units:'500000',poker_in_play_units:'500000'},
      {realized_pl_units:'-500000'},{final_cash_out_units:'9007199255500001'},{ended_at:'2026-09-05T23:59:59Z'},
      {ended_at:'2026-02-30T00:00:00Z'},{end_reason:''}])expect(()=>parsePokerSession({...settled(),...patch},table,session),JSON.stringify(patch)).toThrow();
  });
  it('rejects unknown fields, invalid states, unsafe money, inconsistent exposure and noncanonical counters',()=>{
    for(const patch of [{control:{mode:'CONTROLLER'}},{user_id:'other-owner'},{state:'LEFT'},{seat_no:10},{control_epoch:'0'},
      {control_epoch:'03'},{stack_units:9007199255000000},{stack_units:'-500000'},{stack_units:'9223372036855000000'},
      {poker_in_play_units:'9007199255000000'},{realized_pl_units:'-0'},{table_name:''}])expect(()=>parsePokerSession({...active(),...patch},table,session),JSON.stringify(patch)).toThrow();
  });
  it('uses the exact authenticated owner GET with no original mutation key, ticket or automatic replay',async()=>{
    const {fetcher,client,scope}=await setup();fetcher.mockResolvedValueOnce(ok(settled()));expect(await readPokerHttpSession(client,scope,()=>scope,session)).toEqual(settled());
    expect(fetcher).toHaveBeenCalledTimes(2);expect(fetcher.mock.calls[1][0]).toBe(`/api/v1/poker/sessions/${session}`);
    const init=fetcher.mock.calls[1][1],headers=new Headers(init.headers);expect(init).toMatchObject({method:'GET',credentials:'same-origin',cache:'no-store'});expect(init.body).toBeUndefined();
    expect(headers.get('Authorization')).toBe('Bearer synthetic-session-token');expect(headers.get('X-Auth-Session')).toBe('synthetic-session-sid');expect(headers.get('New-Api-User')).toBe('910002');
  });
  it('drops late malformed data before decoding after native logout or read-generation replacement',async()=>{
    for(const logout of [false,true]){const {fetcher,client,scope}=await setup();let resolve!:(r:Response)=>void;let current=scope;
      fetcher.mockReturnValueOnce(new Promise<Response>(r=>resolve=r));const pending=readPokerHttpSession(client,scope,()=>current,session);
      if(logout){fetcher.mockResolvedValueOnce(ok({}));await client.logout();}else current={...scope,request_generation:2};
      resolve(ok({secret:'old-private-response'}));expect(await pending).toBeUndefined();}
  });
  it('keeps dependency and malformed success failures redacted, not settled or NOT_FOUND',async()=>{
    const {fetcher,client,scope}=await setup();for(const mode of ['network','shape']){if(mode==='network')fetcher.mockRejectedValueOnce(Error('private-upstream-secret'));else fetcher.mockResolvedValueOnce(ok({private:'private-upstream-secret'}));
      await expect(readPokerHttpSession(client,scope,()=>scope,session)).rejects.toMatchObject({code:'POKER_SESSION_READ_UNAVAILABLE',uncertain:false});}
    expect(fetcher).toHaveBeenCalledTimes(3);
  });
  it('rejects invalid original identity or expired scope before any protected network operation',async()=>{
    const {fetcher,client,scope}=await setup();await expect(readPokerHttpSession(client,scope,()=>null,session)).rejects.toThrow();await expect(readPokerHttpSession(client,scope,()=>scope,'../another')).rejects.toThrow();
    expect(fetcher).toHaveBeenCalledTimes(1);
  });
});
