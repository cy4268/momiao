import { describe, expect, it, vi } from 'vitest';
import { ApiClient } from '../api';
import { capturePokerEntryScope, currentPokerEntryScope, parsePokerEntryLookup, parsePokerEntryReceipt, parsePokerReservation, readPokerEntryReceipt, readPokerReservation, submitPokerEntry, type PokerEntryCommand, type PokerEntryScope } from './poker-entry-http';

const table='019a0000-0000-7000-8000-000000000001',res='019a0000-0000-7000-8000-000000000002',session='019a0000-0000-7000-8000-000000000003',funding='019a0000-0000-7000-8000-000000000004';
const create:PokerEntryCommand={kind:'create',request_id:'create-original-key-01',name:'原牌桌',blind_preset:'5-10',max_seats:6,allow_spectators:false,access_mode:'PUBLIC',chat_enabled:false};
const reserve:PokerEntryCommand={kind:'reserve',request_id:'reserve-original-key-01',table_id:table,seat_no:2};
const buyin:PokerEntryCommand={kind:'buyin',request_id:'buyin-original-key-01',table_id:table,reservation_id:res,amount_units:'200000000'};
const receipt=(kind:string)=>({table_id:table,table_version:'1',duplicate:false,...(kind==='create'?{status:'WAITING'}:kind==='reserve'?{status:'LEASE_ACTIVE',reservation_id:res}:{status:'CONFIRMED',session_id:session,funding_operation_id:funding,amount_units:'200000000'})});
const lookup=(c:PokerEntryCommand)=>({user_id:'910002',kind:c.kind,mutation_id:c.request_id,state:'FOUND',receipt:receipt(c.kind)});
const reservation=()=>({user_id:'910002',table_id:table,reservation_id:res,state:'FOUND',reservation:{seat_no:2,durable_state:'LEASE_ACTIVE',expires_at:'2026-09-07T00:00:30Z',checked_at:'2026-09-07T00:00:29Z',valid:true}});
const ok=(data:unknown)=>new Response(JSON.stringify({success:true,data}),{headers:{'Content-Type':'application/json'}});
async function setup(){const fetcher=vi.fn().mockResolvedValueOnce(ok({access_token:'synthetic-entry-token',access_expires_at:4102444800,user:{id:910002,username:'synthetic',role:1},session:{sid:'synthetic-entry-sid'}}));const client=new ApiClient(fetcher);await client.login('synthetic','synthetic');
  const scope:PokerEntryScope={user_id:'910002',session_generation:client.getSessionGeneration(),entry_generation:1};return{fetcher,client,scope};}

describe('Entry commands and original owner recovery',()=>{
  it('accepts each complete kind-specific receipt and exact large chip strings, detached from the input',()=>{
    for(const c of [create,reserve,buyin]){const raw=receipt(c.kind),out=parsePokerEntryReceipt(raw,c);expect(out).toEqual(raw);raw.table_version='2';expect(out.table_version).toBe('1');expect(parsePokerEntryLookup(lookup(c),'910002',c)).toEqual(lookup(c));}
    const large='9007199255000000';expect(parsePokerEntryReceipt({...receipt('buyin'),amount_units:large},{...buyin,amount_units:large}).amount_units).toBe(large); // Parser precision stress only, not a valid current-preset buy-in.
    for(const extra of [{},{funding_operation_id:funding}])expect(parsePokerEntryReceipt({table_id:table,table_version:'2',duplicate:false,status:'FAILED_NO_EFFECT',failure_code:'RESERVATION_LEASE_LOST',...extra},buyin).status).toBe('FAILED_NO_EFFECT');
  });
  it('binds receipt kind/table and amount and rejects invented authority, incomplete identities and numeric coercion',()=>{
    const cases:[PokerEntryCommand,object][]=[[create,{...receipt('create'),session_id:session}],[create,{...receipt('create'),status:'CONFIRMED'}],[reserve,{...receipt('reserve'),table_id:session}],
      [reserve,{...receipt('reserve'),reservation_id:null}],[reserve,{...receipt('reserve'),amount_units:'500000'}],[buyin,{...receipt('buyin'),amount_units:'500000'}],
      [buyin,{...receipt('buyin'),session_id:undefined}],[buyin,{...receipt('buyin'),funding_operation_id:undefined}],[buyin,{...receipt('buyin'),control_epoch:'1'}],
      [buyin,{...receipt('buyin'),amount_units:9007199255000000}],[buyin,{...receipt('buyin'),status:'WAITING'}],[buyin,{...receipt('buyin'),table_version:'01'}]];
    for(const [c,v] of cases)expect(()=>parsePokerEntryReceipt(v,c)).toThrow();
    for(const v of [{...receipt('create'),table_id:'../x'},{...receipt('create'),table_version:'0'},{...receipt('create'),server_seed:'secret'}])expect(()=>parsePokerEntryReceipt(v,create)).toThrow();
  });
  it('keeps NOT_FOUND distinct and closed, with original owner/key/kind and no leaked receipt',()=>{
    for(const c of [create,reserve,buyin]){const missing={user_id:'910002',kind:c.kind,mutation_id:c.request_id,state:'NOT_FOUND'};expect(parsePokerEntryLookup(missing,'910002',c)).toEqual(missing);
      for(const patch of [{user_id:'910003'},{user_id:910002},{mutation_id:'replacement-key-01'},{kind:'leave'},{state:'SETTLED'},{receipt:null},{table_id:table}])expect(()=>parsePokerEntryLookup({...missing,...patch},'910002',c)).toThrow();}
    const raw=lookup(buyin);for(const patch of [{receipt:undefined},{receipt:{...raw.receipt,table_id:res}},{secret:'hidden'}])expect(()=>parsePokerEntryLookup({...raw,...patch},'910002',buyin)).toThrow();
  });
  it('validates the original lease observation without fabricating a lifetime or treating a durable receipt as a live lease',()=>{
    const raw=reservation(),out=parsePokerReservation(raw,'910002',table,res);expect(out).toEqual(raw);raw.reservation.valid=false;expect(out.state==='FOUND'&&out.reservation.valid).toBe(true);
    for(const state of ['LEASE_ACTIVE','EXPIRED','CONSUMED','FAILED_NO_EFFECT'])expect(parsePokerReservation({...raw,reservation:{...raw.reservation,durable_state:state,valid:false}},'910002',table,res).state).toBe('FOUND');
    expect(parsePokerReservation({...raw,reservation:{...raw.reservation,valid:true,expires_at:'2026-09-07T00:00:00.000000002Z',checked_at:'2026-09-07T00:00:00.000000001Z'}},'910002',table,res).state).toBe('FOUND');
    for(const patch of [{valid:true,durable_state:'CONSUMED'},{valid:true,expires_at:raw.reservation.checked_at},{seat_no:10},{durable_state:'WAITING'},{expires_at:'2026-02-30T00:00:00Z'},{checked_at:1},{lease_token:'secret'}])expect(()=>parsePokerReservation({...raw,reservation:{...raw.reservation,...patch}},'910002',table,res)).toThrow();
    for(const patch of [{user_id:'910003'},{table_id:res},{reservation_id:session},{state:'NOT_FOUND'},{reservation:null}])expect(()=>parsePokerReservation({...raw,...patch},'910002',table,res)).toThrow();
    expect(parsePokerReservation({user_id:'910002',table_id:table,reservation_id:res,state:'NOT_FOUND'},'910002',table,res).state).toBe('NOT_FOUND');
  });
  it('sends exact one-shot commands and read-only POST lookups through real native headers, never keys in URLs',async()=>{
    const {client,fetcher,scope}=await setup();const expected=[['/api/v1/poker/tables',{request_id:create.request_id,name:'原牌桌',blind_preset:'5-10',max_seats:6,allow_spectators:false,access_mode:'PUBLIC',chat_enabled:false}],
      [`/api/v1/poker/tables/${table}/seat-reservations`,{request_id:reserve.request_id,seat_no:2}],[`/api/v1/poker/tables/${table}/buy-ins`,{request_id:buyin.request_id,reservation_id:res,amount_units:'200000000'}]];
    for(const [i,c] of [create,reserve,buyin].entries()){fetcher.mockResolvedValueOnce(ok(receipt(c.kind)));expect(await submitPokerEntry(client,scope,()=>scope,c)).toEqual(receipt(c.kind));const [path,init]=fetcher.mock.calls.at(-1)!;expect(path).toBe(expected[i][0]);expect(JSON.parse(init.body)).toEqual(expected[i][1]);
      expect(init).toMatchObject({method:'POST',credentials:'same-origin',cache:'no-store'});const h=new Headers(init.headers);expect(h.get('Authorization')).toBe('Bearer synthetic-entry-token');expect(h.get('X-Auth-Session')).toBe('synthetic-entry-sid');expect(h.get('New-Api-User')).toBe('910002');
      fetcher.mockResolvedValueOnce(ok(lookup(c)));expect(await readPokerEntryReceipt(client,scope,()=>scope,c)).toEqual(lookup(c));const [query,request]=fetcher.mock.calls.at(-1)!;expect(query).toBe('/api/v1/poker/entry-receipt-query');expect(JSON.parse(request.body)).toEqual({kind:c.kind,mutation_id:c.request_id,...(c.kind==='create'?{}:{table_id:table})});}
    fetcher.mockResolvedValueOnce(ok(reservation()));expect(await readPokerReservation(client,scope,()=>scope,table,res)).toEqual(reservation());const [path,init]=fetcher.mock.calls.at(-1)!;expect(path).toBe(`/api/v1/poker/tables/${table}/reservation-query`);expect(JSON.parse(init.body)).toEqual({reservation_id:res});expect(fetcher).toHaveBeenCalledTimes(8);
  });
  it('rejects unsupported command fields and invalid identifiers before sending or dropping anything silently',async()=>{
    const {client,fetcher,scope}=await setup();const bad:unknown[]=[{...create,table_id:null},{...create,password:'secret'},{...create,access_mode:'PASSWORD'},{...create,chat_enabled:'true'},
      {...create,name:' leading'},{...create,name:'x\n'},{...create,name:'x'.repeat(41)},{...create,max_seats:10},{...create,allow_spectators:1},{...create,blind_preset:''},
      {...reserve,table_id:'../another'},{...reserve,seat_no:0},{...reserve,request_id:'short'},{...buyin,reservation_id:'bad'},{...buyin,amount_units:'1'},{...buyin,amount_units:'0'},{...buyin,amount_units:'9223372036855000000'},Object.assign(Object.create({extra:'unsupported'}),create)];
    for(const c of bad)await expect(submitPokerEntry(client,scope,()=>scope,c as PokerEntryCommand)).rejects.toThrow();
    await expect(readPokerEntryReceipt(client,scope,()=>scope,{...create,table_id:null} as unknown as PokerEntryCommand)).rejects.toThrow();await expect(readPokerReservation(client,scope,()=>scope,table,'bad')).rejects.toThrow();expect(fetcher).toHaveBeenCalledTimes(1);
  });
  it('captures native identity and independent entry generation, rejecting stale scopes before I/O',async()=>{
    const {client,fetcher,scope}=await setup();expect(capturePokerEntryScope(client,1)).toEqual(scope);expect(currentPokerEntryScope(client,scope,()=>({...scope}))).toBe(true);
    for(const bad of [null,{...scope,user_id:'910003'},{...scope,entry_generation:2},{...scope,session_generation:scope.session_generation+1}]){expect(currentPokerEntryScope(client,scope,()=>bad)).toBe(false);await expect(submitPokerEntry(client,scope,()=>bad,create)).rejects.toThrow();}
    expect(()=>capturePokerEntryScope(client,-1)).toThrow();expect(fetcher).toHaveBeenCalledTimes(1);
  });
  it('drops old generation, unmount and native logout replies before decoding even private getters',async()=>{
    for(const mode of ['entry','unmount','logout'])for(const route of ['submit','receipt','reservation']){const {client,fetcher,scope}=await setup();let current:PokerEntryScope|null=scope,resolve!:(r:Response)=>void;const decoded=vi.fn(()=>{throw Error('old private data');});
      fetcher.mockReturnValueOnce(new Promise<Response>(r=>resolve=r));const pending=route==='submit'?submitPokerEntry(client,scope,()=>current,create):route==='receipt'?readPokerEntryReceipt(client,scope,()=>current,create):readPokerReservation(client,scope,()=>current,table,res);
      if(mode==='logout'){fetcher.mockResolvedValueOnce(ok({}));await client.logout();}else current=mode==='unmount'?null:{...scope,entry_generation:2};
      const raw=route==='submit'?receipt('create'):route==='receipt'?lookup(create):reservation();Object.defineProperty(raw,route==='submit'?'table_id':'user_id',{enumerable:true,get:decoded});
      resolve({ok:true,status:200,json:async()=>({success:true,data:raw})} as Response);expect(await pending).toBeUndefined();expect(decoded).not.toHaveBeenCalled();}
  });
  it('ignores late network failures from all entry paths without disturbing the replacement entry',async()=>{
    for(const route of ['submit','receipt','reservation']){const {client,fetcher,scope}=await setup();let current=scope,reject!:(e:Error)=>void;fetcher.mockReturnValueOnce(new Promise<Response>((_,r)=>reject=r));
      const pending=route==='submit'?submitPokerEntry(client,scope,()=>current,create):route==='receipt'?readPokerEntryReceipt(client,scope,()=>current,create):readPokerReservation(client,scope,()=>current,table,res);
      current={...scope,entry_generation:2};reject(Error('old private network failure'));expect(await pending).toBeUndefined();expect(client.getSnapshot().user?.id).toBe(910002);expect(fetcher).toHaveBeenCalledTimes(2);}
  });
  it('copies original command before awaiting and redacts failures without automatic mutation retry',async()=>{
    const {client,fetcher,scope}=await setup();let resolve!:(r:Response)=>void;fetcher.mockReturnValueOnce(new Promise<Response>(r=>resolve=r));const command={...buyin},pending=submitPokerEntry(client,scope,()=>scope,command);command.amount_units='500000';resolve(ok(receipt('buyin')));expect((await pending)?.amount_units).toBe('200000000');
    for(const response of [new Response(JSON.stringify({success:false,code:'PRIVATE_SECRET',message:'private-upstream-secret'}),{status:503}),ok({secret:'private-upstream-secret'})]){fetcher.mockResolvedValueOnce(response);await expect(submitPokerEntry(client,scope,()=>scope,create)).rejects.toMatchObject({code:'POKER_ENTRY_OUTCOME_UNCONFIRMED',uncertain:true});}
    fetcher.mockRejectedValueOnce(Error('private-upstream-secret'));await expect(readPokerEntryReceipt(client,scope,()=>scope,create)).rejects.toMatchObject({code:'POKER_ENTRY_READ_UNAVAILABLE',uncertain:false});expect(fetcher).toHaveBeenCalledTimes(5);
  });
  it('does not replay a 401 mutation and lets the real native client invalidate the old entry scope',async()=>{
    const {client,fetcher,scope}=await setup();fetcher.mockResolvedValueOnce(new Response(JSON.stringify({success:false,code:'AUTH_REQUIRED',message:'synthetic'}),{status:401}));
    expect(await submitPokerEntry(client,scope,()=>scope,create)).toBeUndefined();expect(client.getSnapshot().user).toBeNull();expect(currentPokerEntryScope(client,scope,()=>scope)).toBe(false);expect(fetcher).toHaveBeenCalledTimes(2);
  });
  it('checks entry lifetime after proactive same-session refresh and before any protected POST begins',async()=>{
    for(const mode of ['entry','unmount','same']){const bundle={access_token:'synthetic-entry-token',access_expires_at:1,user:{id:910002,username:'synthetic',role:1},session:{sid:'synthetic-entry-sid'}};
      const fetcher=vi.fn().mockResolvedValueOnce(ok(bundle)),client=new ApiClient(fetcher);await client.login('synthetic','synthetic');const scope=capturePokerEntryScope(client,1);let current:PokerEntryScope|null=scope,resolve!:(r:Response)=>void;
      fetcher.mockReturnValueOnce(new Promise<Response>(r=>resolve=r)).mockResolvedValueOnce(ok(receipt('buyin')));const pending=submitPokerEntry(client,scope,()=>current,buyin);
      expect(fetcher.mock.calls.at(-1)![0]).toBe('/api/user/auth/refresh');if(mode!=='same')current=mode==='unmount'?null:{...scope,entry_generation:2};resolve(ok({...bundle,access_token:'synthetic-refreshed-token',access_expires_at:4102444800}));
      if(mode==='same'){expect(await pending).toEqual(receipt('buyin'));expect(fetcher.mock.calls.map(c=>c[0])).toEqual(['/api/user/login','/api/user/auth/refresh',`/api/v1/poker/tables/${table}/buy-ins`]);expect(JSON.parse(fetcher.mock.calls[2][1].body)).toEqual({request_id:buyin.request_id,reservation_id:res,amount_units:'200000000'});}
      else{expect(await pending).toBeUndefined();expect(fetcher.mock.calls.map(c=>c[0])).toEqual(['/api/user/login','/api/user/auth/refresh']);}expect(client.getSessionGeneration()).toBe(scope.session_generation);expect(client.getSnapshot().user?.id).toBe(910002);
    }
  });
  it('checks the optional native send guard again before a GET retry without clearing the current login',async()=>{
    const {client,fetcher}=await setup();let alive=true,resolve!:(r:Response)=>void;fetcher.mockResolvedValueOnce(new Response(JSON.stringify({success:false,code:'AUTH_TOKEN_EXPIRED'}),{status:401})).mockReturnValueOnce(new Promise<Response>(r=>resolve=r)).mockResolvedValueOnce(ok({}));
    const pending=client.request('/api/user/self','GET',undefined,undefined,()=>alive);await vi.waitFor(()=>expect(fetcher).toHaveBeenCalledTimes(3));alive=false;
    resolve(ok({access_token:'synthetic-refreshed-token',access_expires_at:4102444800,user:{id:910002,username:'synthetic',role:1},session:{sid:'synthetic-entry-sid'}}));
    await expect(pending).rejects.toMatchObject({code:'REQUEST_SCOPE_EXPIRED',uncertain:false});expect(fetcher.mock.calls.map(c=>c[0])).toEqual(['/api/user/login','/api/user/self','/api/user/auth/refresh']);expect(client.getSnapshot().user?.id).toBe(910002);
  });
});
