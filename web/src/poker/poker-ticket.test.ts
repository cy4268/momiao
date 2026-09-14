import { describe, expect, it, vi } from 'vitest';
import { ApiClient } from '../api';
import { capturePokerReadScope, mintPokerConnectTicket, parsePokerConnectTicket } from './poker-http';
import self from './fixtures/g3-player-self.json';

const ticket='ct1.eyJzeW50aGV0aWMiOnRydWV9.c2lnbmF0dXJl';
const ok=(data:unknown)=>new Response(JSON.stringify({success:true,data}),{headers:{'Content-Type':'application/json'}});
async function setup(){const fetcher=vi.fn().mockResolvedValueOnce(ok({access_token:'synthetic-access',access_expires_at:4102444800,user:{id:910002,username:'synthetic',display_name:'Synthetic',role:1},session:{sid:'synthetic-session'}})),client=new ApiClient(fetcher);await client.login('synthetic','synthetic');const scope=capturePokerReadScope(client,self.table_id,'PLAYER_SELF',1);return{client,fetcher,scope};}
describe('Poker scope-local ticket boundary',()=>{
  it('accepts only the exact single-field canonical ct1 shape without interpreting signed identity claims',()=>{
    expect(parsePokerConnectTicket({poker_connect_ticket:ticket})).toBe(ticket);
    for(const raw of [null,[],{},ticket,{poker_connect_ticket:ticket,connection_id:'a'.repeat(32)},{poker_connect_ticket:ticket,controller:true},{poker_connect_ticket:ticket,expires_at:1}])expect(()=>parsePokerConnectTicket(raw)).toThrow();
  });
  it.each(['ct2.YQ.Yg','ct1.YQ==.Yg','ct1.YR.Yg','ct1.a.Yg','ct1..Yg','ct1.YQ.Yg.extra','ct1.YQ.Yg\n','ct1.'+'YQ'.repeat(4096)+'.Yg'])('rejects noncanonical, padded or oversized ticket %s',value=>{
    expect(()=>parsePokerConnectTicket({poker_connect_ticket:value})).toThrow();
  });
  it('mints once through native ApiClient and preserves exact scoped intent without user-selected identity',async()=>{
    const {client,fetcher,scope}=await setup();fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:ticket}));expect(await mintPokerConnectTicket(client,scope,()=>scope,'READ_ONLY')).toBe(ticket);
    expect(fetcher.mock.calls[1][0]).toBe('/api/v1/poker/connect-tickets');expect(JSON.parse(fetcher.mock.calls[1][1].body)).toEqual({target_table_id:self.table_id,control_intent:'READ_ONLY'});expect(fetcher.mock.calls[1][1]).toMatchObject({method:'POST',credentials:'same-origin',cache:'no-store'});expect(fetcher).toHaveBeenCalledTimes(2);
  });
  it('discards late replies before ticket decoding and never revives an old socket scope',async()=>{
    const {client,fetcher,scope}=await setup();let finish!:(r:Response)=>void;fetcher.mockReturnValueOnce(new Promise<Response>(r=>finish=r));let current=scope;
    const result=mintPokerConnectTicket(client,scope,()=>current,'CLAIM_CONTROL');current={...scope,request_generation:2};finish(ok({private_secret:'old-scope-content'}));expect(await result).toBeUndefined();
  });
  it('invalid intent and stale auth stop before I/O; mint failures are redacted and never retried',async()=>{
    const {client,fetcher,scope}=await setup();await expect(mintPokerConnectTicket(client,scope,()=>scope,'CONTROLLER' as never)).rejects.toThrow();expect(fetcher).toHaveBeenCalledTimes(1);
    fetcher.mockResolvedValueOnce(new Response(JSON.stringify({success:false,message:'private-ticket-secret',code:'POKER_TICKET_DENIED'}),{status:403}));await expect(mintPokerConnectTicket(client,scope,()=>scope,'CLAIM_CONTROL')).rejects.toMatchObject({code:'POKER_TICKET_DENIED'});
    fetcher.mockResolvedValueOnce(ok({poker_connect_ticket:'private-ticket-secret'}));await expect(mintPokerConnectTicket(client,scope,()=>scope,'CLAIM_CONTROL')).rejects.not.toThrow('private-ticket-secret');expect(fetcher).toHaveBeenCalledTimes(3);
  });
});
