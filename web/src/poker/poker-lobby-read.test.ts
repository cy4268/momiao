import { describe, expect, it, vi } from 'vitest';
import { ApiClient } from '../api';
import { capturePokerLobbyScope, currentPokerLobbyScope, parsePokerLobbySnapshot, pokerLobbyReadPath, readPokerHttpLobby } from './poker-lobby-read';
import type { LobbyFilters, PokerLobbyReadScope } from './poker-ui-types';
import synthetic from './fixtures/l1-lobby.synthetic.json';

const filters:LobbyFilters={query:'',visibility:'ALL',open_seats_only:false,max_seats:'ALL',blind_preset_id:'ALL',lifecycle_state:'ALL',spectators_only:false,sort:'LOW_BLIND',limit:50,cursor:null};
const raw=():any=>structuredClone(synthetic),owner='910001';
const ok=(data:unknown)=>new Response(JSON.stringify({success:true,data}),{headers:{'Content-Type':'application/json'}});
const bundle=(id=910001)=>({access_token:'synthetic-memory-token',access_expires_at:4102444800,user:{id,username:'synthetic',display_name:'Synthetic',role:1},session:{sid:'synthetic-session'}});
async function setup(query?:LobbyFilters){const fetcher=vi.fn().mockResolvedValueOnce(ok(bundle())),client=new ApiClient(fetcher);await client.login('synthetic','synthetic');return {fetcher,client,scope:capturePokerLobbyScope(client,query,1)};}
const session=()=>({session_id:'019a0000-0000-7000-8000-000000000001',table_id:synthetic.tables[0].table_id,table_name:'月光长廊',state:'ACTIVE',seat_no:1,stack_units:'400000000',committed_units:'10000000',poker_in_play_units:'410000000',small_blind_units:'2500000',big_blind_units:'5000000',ante_units:'0',can_reconnect:true});

describe('L2a closed whole lobby read',()=>{
  it('retains every section and exact atomic wallet remainder without inventing socket authority',()=>{
    const v=raw();v.viewer.available_chips_units='9007199254740993';const result=parsePokerLobbySnapshot(v,owner);
    expect(result).toEqual(v);expect(result.viewer.available_chips_units).toBe('9007199254740993');expect(result).not.toHaveProperty('control');
    v.tables[0].name='mutated';expect(result.tables[0].name).toBe('月光长廊');
  });
  it('keeps nullable branches and real ACTIVE/NEEDS_REVIEW session exposure',()=>{
    const v=raw();v.ruleset=null;v.service={state:'CONFIG_INCOMPLETE',production_ready:false,blockers:['POKER_RULESET_INCOMPLETE'],maintenance_scopes:[]};expect(parsePokerLobbySnapshot(v,owner).ruleset).toBeNull();
    for(const state of ['ACTIVE','NEEDS_REVIEW']){const active=raw();active.active_session={...session(),state};active.viewer.poker_in_play_units='410000000';active.viewer.owned_open_table_id=synthetic.tables[0].table_id;expect(parsePokerLobbySnapshot(active,owner).active_session?.state).toBe(state);}
  });
  it('rejects wrong owner, malformed nested facts, money, versions, cardinality and capabilities',()=>{
    const changes:((v:any)=>void)[]=[
      v=>v.viewer.user_id='910002',v=>delete v.page,v=>v.viewer.available_chips_units='9223372036854775808',
      v=>v.viewer.wallet_version=12,v=>v.viewer.poker_in_play_units='1',v=>v.viewer.owned_open_table_id='short',
      v=>v.ruleset.entry_mode='IMMEDIATE_BB',v=>v.service.state='LIVE',v=>v.service.production_ready='true',
      v=>v.service.blockers=['UNKNOWN'],v=>v.service.maintenance_scopes=['ALL'],v=>v.create_options.access_modes=['PASSWORD','PUBLIC'],
      v=>v.create_options.chat_configurable='true',v=>v.tables=null,v=>v.blind_presets=Array(101).fill(v.blind_presets[0]),
      v=>v.tables.push(v.tables[0]),v=>v.tables[0].open_seat_numbers=[3,3],v=>v.tables[0].open_seat_numbers=[7],
      v=>v.tables[0].small_blind_units='1',v=>v.tables[0].table_version='-1',v=>v.tables[0].occupied_seats=7,
      v=>v.tables[0].can_join=1,v=>v.tables[0].can_request_access=true,v=>v.tables[1].can_request_access='false',v=>v.tables[0].lifecycle_state='ACTIVE',v=>v.tables[0].maximum_buyin_units='0',
      v=>v.active_session={...session(),state:'DISCONNECTED'},v=>v.active_session={...session(),poker_in_play_units:'0'},
      v=>v.page.limit=101,v=>v.page.next_cursor='bad=',v=>v.tables=Array(101).fill(v.tables[0]),
    ];
    changes.forEach(change=>{const v=raw();change(v);expect(()=>parsePokerLobbySnapshot(v,owner)).toThrow();});
    expect(()=>parsePokerLobbySnapshot(raw(),'0')).toThrow();expect(()=>parsePokerLobbySnapshot([],owner)).toThrow();
  });
  it('closes every object layer against hidden hand/control fields and missing required fields',()=>{
    for(const path of ['', 'service','ruleset','viewer','create_options','blind_presets.0','tables.0','page','active_session']){
      for(const extra of [true,false]){const v=raw();v.active_session=session();v.viewer.poker_in_play_units='410000000';const target=path?path.split('.').reduce((a,k)=>a[k],v):v;
        if(extra)target.control={secret:'never render'};else delete target[Object.keys(target)[0]];expect(()=>parsePokerLobbySnapshot(v,owner)).toThrow();}
    }
  });
});
describe('L2a canonical public query and native HTTP scope',()=>{
  it('encodes only the ten public fields once and passes opaque NEAR_FULL cursors unchanged',()=>{
    expect(pokerLobbyReadPath()).toBe('/api/v1/poker');
    expect(pokerLobbyReadPath({...filters,query:'  % _ &  ',visibility:'PASSWORD',open_seats_only:true,max_seats:6,blind_preset_id:'5-10',lifecycle_state:'IN_HAND',spectators_only:true,sort:'NEAR_FULL',limit:2,cursor:'eyJ2IjoxfQ'})).toBe('/api/v1/poker/tables?q=%25+_+%26&access_mode=PASSWORD&open_seats_only=true&max_seats=6&blind_preset=5-10&lifecycle_state=IN_HAND&spectators_only=true&sort=NEAR_FULL&limit=2&cursor=eyJ2IjoxfQ');
    expect(pokerLobbyReadPath(filters)).toBe('/api/v1/poker/tables?access_mode=ALL&open_seats_only=false&blind_preset=ALL&lifecycle_state=ALL&spectators_only=false&sort=LOW_BLIND&limit=50');
  });
  it('rejects unknown query keys, invalid types, old sort, overlong graphemes and bad cursor shapes',()=>{
    for(const patch of [{secret:'x'},{sort:'NEAR_START'},{limit:0},{limit:101},{max_seats:10},{open_seats_only:'false'},{query:'x\u0000'},{query:'👨‍👩‍👧‍👦'.repeat(41)},{cursor:'x='},{cursor:'x'.repeat(513)},{lifecycle_state:'ACTIVE'},{visibility:'private'}])expect(()=>pokerLobbyReadPath({...filters,...patch} as LobbyFilters)).toThrow();
    expect(pokerLobbyReadPath({...filters,query:'👨‍👩‍👧‍👦'.repeat(40)})).toContain('q=');
  });
  it('uses one native-auth GET for the whole snapshot with no socket scope or grant',async()=>{
    const {fetcher,client,scope}=await setup();fetcher.mockResolvedValueOnce(ok(raw()));expect(await readPokerHttpLobby(client,scope,()=>scope)).toEqual(raw());
    expect(scope).toEqual({user_id:owner,session_generation:client.getSessionGeneration(),request_generation:1,query_key:'/api/v1/poker'});expect(currentPokerLobbyScope(client,scope,()=>scope)).toBe(true);
    expect(fetcher).toHaveBeenCalledTimes(2);const [path,init]=fetcher.mock.calls[1];expect(path).toBe('/api/v1/poker');expect(init).toMatchObject({method:'GET',credentials:'same-origin',cache:'no-store'});expect(init.body).toBeUndefined();expect(new Headers(init.headers).get('New-Api-User')).toBe(owner);
  });
  it('blocks stale scope and query-path mismatch before I/O; errors never become zero wallet/empty tables',async()=>{
    const {fetcher,client,scope}=await setup();await expect(readPokerHttpLobby(client,scope,()=>null)).rejects.toThrow();await expect(readPokerHttpLobby(client,scope,()=>scope,filters)).rejects.toThrow();expect(fetcher).toHaveBeenCalledTimes(1);
    fetcher.mockResolvedValueOnce(new Response(JSON.stringify({success:false,message:'secret'}),{status:503}));await expect(readPokerHttpLobby(client,scope,()=>scope)).rejects.toMatchObject({code:'POKER_LOBBY_READ_UNAVAILABLE',status:503});
    fetcher.mockRejectedValueOnce(Error('private'));await expect(readPokerHttpLobby(client,scope,()=>scope)).rejects.not.toThrow('private');expect(fetcher).toHaveBeenCalledTimes(3);
  });
  it.each(['owner','session','request','filter','page','dispose','native-session','native-account','logout'])('drops late %s data before Poker decoding',async(kind)=>{
    const {fetcher,client,scope}=await setup();let resolve!:(r:Response)=>void,current:PokerLobbyReadScope|null=scope,decoded=0;
    fetcher.mockReturnValueOnce(new Promise<Response>(r=>resolve=r));const pending=readPokerHttpLobby(client,scope,()=>current);
    if(kind==='dispose')current=null;
    else if(kind==='owner')current={...scope,user_id:'910002'};
    else if(kind==='session')current={...scope,session_generation:scope.session_generation+1};
    else if(kind==='request')current={...scope,request_generation:2};
    else if(kind==='filter'||kind==='page')current={...scope,query_key:kind==='filter'?'/api/v1/poker/tables?q=new':'/api/v1/poker/tables?cursor=eyJ2IjoxfQ'};
    else {const next=bundle(kind==='native-account'?910002:910001);next.session.sid='synthetic-new-session';fetcher.mockResolvedValueOnce(ok(kind==='logout'?{}:next));if(kind==='logout')await client.logout();else await client.login('synthetic','synthetic');}
    const v=raw();Object.defineProperty(v,'server_now',{enumerable:true,get(){decoded++;throw Error('stale decoded');}});
    resolve({ok:true,status:200,json:async()=>({success:true,data:v})} as Response);expect(await pending).toBeUndefined();expect(decoded).toBe(0);
  });
  it('drops an old HTTP failure after a newer request without overwriting fresh data',async()=>{
    const {fetcher,client,scope}=await setup();let reject!:(e:Error)=>void,current=scope;fetcher.mockReturnValueOnce(new Promise<Response>((_,r)=>reject=r));const old=readPokerHttpLobby(client,scope,()=>current);
    current={...scope,request_generation:2};fetcher.mockResolvedValueOnce(ok(raw()));expect((await readPokerHttpLobby(client,current,()=>current))?.viewer.wallet_version).toBe('12');reject(Error('old private'));expect(await old).toBeUndefined();
  });
});
