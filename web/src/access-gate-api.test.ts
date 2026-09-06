import { expect, it } from 'vitest';
import { ApiClient } from './api';
import { acknowledgeMigrationNotice, readAccessGate } from './access-gate-api';
import { bundle, ok } from './m1-test-fixtures';

it('rejects cross-session, malformed gate and nonmatching acknowledgement responses', async()=>{
 let response:unknown;
 const client=new ApiClient(async(path:string)=>path==='/api/user/login'?ok(bundle):ok(response));
 await client.login('synthetic','synthetic');
 const gate={user_id:String(bundle.user.id),route:'/keys',stage:'READY'};
 for(const invalid of [{...gate,user_id:'43'},{...gate,route:'/wallet'},{...gate,stage:'UNKNOWN'},{...gate,stage:'MIGRATION_REQUIRED'}]){
  response=invalid;await expect(readAccessGate(client,'/keys')).rejects.toMatchObject({code:'ACCESS_GATE_UNVERIFIED'});
 }
 response=gate;await expect(readAccessGate(client,'/keys')).resolves.toEqual(gate);
 const notice={user_id:gate.user_id,state:'ACKNOWLEDGED',required_migration_version:'2',acknowledged_migration_version:'2',acknowledged_at:'2026-09-06T00:00:00Z',completed_at:'2026-09-05T00:00:00Z',title:'Synthetic fact',body:'Synthetic fact only'};
 response=notice;await expect(acknowledgeMigrationNotice(client,'1')).rejects.toMatchObject({code:'ACCESS_GATE_UNVERIFIED'});
 await expect(acknowledgeMigrationNotice(client,'2')).resolves.toEqual(notice);
});
