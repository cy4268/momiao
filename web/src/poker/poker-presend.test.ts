import { describe, expect, it } from 'vitest';
import { ApiClient } from '../api';
import { capturePokerReadScope, mintPokerConnectTicket, readPokerHttpReceipt, readPokerHttpTable, submitPokerHttpCommand, type PokerHttpCommand } from './poker-http';
import { capturePokerLobbyScope, readPokerHttpLobby } from './poker-lobby-read';
import { readPokerHttpSession } from './poker-session';
import type { PokerReadScope } from './poker-client-boundary';
import type { PokerLobbyReadScope } from './poker-ui-types';
import self from './fixtures/g3-player-self.json';
import lobby from './fixtures/l1-lobby.synthetic.json';

const table=self.table_id, other='019a0000-0000-7000-8000-000000000020', session=self.viewer.session_id;
const request='synthetic-request-00001', user={id:910002,username:'synthetic',display_name:'Synthetic',role:1};
const bundle=(overrides:Record<string,unknown>={})=>({access_token:'synthetic',access_expires_at:Date.now()/1000+3600,user,session:{sid:'synthetic-native-sid'},...overrides});
const ok=(data:unknown)=>new Response(JSON.stringify({success:true,data}),{headers:{'Content-Type':'application/json'}});
const operations=['topup','leave','takeover','ticket','receipt','table','session'] as const;
type Operation=typeof operations[number]|'lobby';
function deferred<T>(){let resolve!:(value:T)=>void;return {promise:new Promise<T>(r=>{resolve=r;}),resolve};}

// Real ApiClient and parsers; only fetch timing/DTOs are synthetic. No private client/stream writes.
async function harness(retry=false){
  const refresh=deferred<Response>(),entered=deferred<void>(),calls:{path:string;method:string;body:unknown}[]=[];
  let firstRead=true;
  const fetcher=async(path:string,init?:RequestInit)=>{
    calls.push({path,method:init?.method??'GET',body:init?.body?JSON.parse(String(init.body)):undefined});
    if(calls.length===1)return ok(bundle({access_expires_at:retry?Date.now()/1000+3600:1}));
    if(path==='/api/user/auth/refresh'){entered.resolve();return refresh.promise;}
    if(path==='/api/user/auth/logout')return ok({});
    if(!path.startsWith('/api/v1/poker'))throw Error('Unexpected synthetic path');
    if(retry&&firstRead){firstRead=false;return new Response('',{status:401});}
    if(path.endsWith('/connect-tickets'))return ok({poker_connect_ticket:'ct1.eyJzeW50aGV0aWMiOnRydWV9.c2lnbmF0dXJl'});
    if(path.endsWith('/receipt-query'))return ok({table_id:table,kind:'leave',mutation_id:request,state:'NOT_FOUND'});
    if(path==='/api/v1/poker'){const raw=structuredClone(lobby);raw.viewer.user_id=String(user.id);return ok(raw);}
    if(path==='/api/v1/poker/tables/'+table){const raw=structuredClone(self);raw.viewer.can_act=false;delete (raw.viewer as {legal?:unknown}).legal;return ok(raw);}
    if(path==='/api/v1/poker/sessions/'+session)return ok({table_id:table,session_id:session,table_name:'Synthetic',state:'ACTIVE',seat_no:2,stack_units:'1000000',committed_units:'500000',poker_in_play_units:'1500000',control_epoch:'1',initial_buy_in_units:'2000000',total_top_up_units:'0',started_at:'2026-09-06T00:00:00Z'});
    return ok({table_id:table,session_id:session,status:'CONFIRMED',table_version:'8',duplicate:false});
  };
  const api=new ApiClient(fetcher);await api.login('synthetic','synthetic');
  const scope=capturePokerReadScope(api,table,'PLAYER_SELF',1),lobbyScope=capturePokerLobbyScope(api,undefined,1);
  let current:PokerReadScope|null=scope,currentLobby:PokerLobbyReadScope|null=lobbyScope;
  const pokerCalls=()=>calls.filter(c=>c.path.startsWith('/api/v1/poker'));
  const start=(operation:Operation)=>{
    if(operation==='ticket')return mintPokerConnectTicket(api,scope,()=>current,'READ_ONLY');
    if(operation==='receipt')return readPokerHttpReceipt(api,scope,()=>current,{kind:'leave',request_id:request,action_id:null,target_session_id:session});
    if(operation==='table')return readPokerHttpTable(api,scope,()=>current);
    if(operation==='session')return readPokerHttpSession(api,scope,()=>current,session);
    if(operation==='lobby')return readPokerHttpLobby(api,lobbyScope,()=>currentLobby);
    const command:PokerHttpCommand=operation==='topup'?{type:'topup',request_id:request,session_id:session,amount_units:'500000'}:operation==='takeover'?{type:'takeover',request_id:request,session_id:session,connection_id:'a'.repeat(32)}:{type:'leave',request_id:request,session_id:session};
    return submitPokerHttpCommand(api,command,scope,()=>current);
  };
  return{api,scope,lobbyScope,start,refresh,entered,pokerCalls,setCurrent:(v:PokerReadScope|null)=>{current=v;},setLobby:(v:PokerLobbyReadScope|null)=>{currentLobby=v;}};
}

describe('Poker hard scopes are checked immediately before protected fetch',()=>{
  for(const operation of operations)for(const change of ['disposed','table','viewer','generation'] as const){
    it(`${operation}: ${change} during Native refresh sends zero old-scope requests`,async()=>{
      const h=await harness(),pending=h.start(operation);await h.entered.promise;expect(h.pokerCalls()).toHaveLength(0);
      h.setCurrent(change==='disposed'?null:change==='table'?{...h.scope,table_id:other}:change==='viewer'?{...h.scope,viewer_kind:'SPECTATOR'}:{...h.scope,request_generation:2});
      h.refresh.resolve(ok(bundle()));expect(await pending).toBeUndefined();expect(h.pokerCalls()).toHaveLength(0);
      expect(h.api.getSnapshot().user?.id).toBe(user.id); // Caller expiry is not logout.
    });
  }
  it.each(['disposed','generation','query'])('Lobby %s change during Native refresh sends zero requests',async change=>{
    const h=await harness(),pending=h.start('lobby');await h.entered.promise;
    h.setLobby(change==='disposed'?null:change==='generation'?{...h.lobbyScope,request_generation:2}:{...h.lobbyScope,query_key:'/api/v1/poker/tables?q=changed'});
    h.refresh.resolve(ok(bundle()));expect(await pending).toBeUndefined();expect(h.pokerCalls()).toHaveLength(0);
  });
  it.each(['table','session','lobby'] as const)('%s GET401 retry rechecks the caller before its second raw fetch',async operation=>{
    const h=await harness(true),pending=h.start(operation);await h.entered.promise;expect(h.pokerCalls()).toHaveLength(1);
    h.setCurrent(null);h.setLobby(null);h.refresh.resolve(ok(bundle()));expect(await pending).toBeUndefined();expect(h.pokerCalls()).toHaveLength(1);
  });
});

describe('Existing Native identity and unchanged scope behavior stays intact',()=>{
  for(const operation of [...operations,'lobby'] as const)for(const change of ['native_sid','account','logout'] as const){
    it(`${operation}: ${change} still sends zero protected requests`,async()=>{
      const h=await harness(),pending=h.start(operation);await h.entered.promise;
      const logout=change==='logout'?h.api.logout():undefined;
      h.refresh.resolve(ok(bundle(change==='native_sid'?{session:{sid:'synthetic-other'}}:change==='account'?{user:{...user,id:910003}}:{})));
      expect(await pending).toBeUndefined();await logout;expect(h.pokerCalls()).toHaveLength(0);
    });
  }
  it.each([...operations,'lobby'] as const)('%s unchanged scope sends once and accepts a fully parsed result',async operation=>{
    const h=await harness(),pending=h.start(operation);await h.entered.promise;expect(h.pokerCalls()).toHaveLength(0);
    h.refresh.resolve(ok(bundle()));expect(await pending).toBeDefined();expect(h.pokerCalls()).toHaveLength(1);expect(h.api.getSessionGeneration()).toBe(h.scope.session_generation);
  });
});
