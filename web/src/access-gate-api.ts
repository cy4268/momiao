import { ApiClient, ApiError } from './api';
import { normalizeRouteIntent } from './post-auth-intent';

export interface MigrationNotice {
 user_id:string;state:'UNVERIFIED'|'NOT_REQUIRED'|'REQUIRED'|'ACKNOWLEDGED';
 required_migration_version:string;acknowledged_migration_version:string;
 acknowledged_at:string|null;completed_at:string|null;title:string;body:string;
}
export type GateStage='READY'|'ACCOUNT_RESTRICTED'|'MASTER_REQUIRED'|'MIGRATION_REQUIRED'|'MIGRATION_UNVERIFIED'|'ROLE_DENIED'|'ROLE_UNVERIFIED'|'RESOURCE_UNAVAILABLE'|'RESOURCE_UNVERIFIED'|'MAINTENANCE';
export interface AccessGateView {user_id:string;route:string;stage:GateStage;migration_notice?:MigrationNotice}
const stages:GateStage[]=['READY','ACCOUNT_RESTRICTED','MASTER_REQUIRED','MIGRATION_REQUIRED','MIGRATION_UNVERIFIED','ROLE_DENIED','ROLE_UNVERIFIED','RESOURCE_UNAVAILABLE','RESOURCE_UNVERIFIED','MAINTENANCE'];
const malformed=()=>new ApiError('访问状态响应尚未通过核对，请重新读取。',503,'ACCESS_GATE_UNVERIFIED');
const date=(v:unknown):v is string=>typeof v==='string'&&Number.isFinite(Date.parse(v));
function checkedNotice(n:MigrationNotice,user:string):MigrationNotice{
 if(!n||n.user_id!==user||!['UNVERIFIED','NOT_REQUIRED','REQUIRED','ACKNOWLEDGED'].includes(n.state)||!/^(0|[1-9][0-9]{0,18})$/.test(n.required_migration_version)||!/^(0|[1-9][0-9]{0,18})$/.test(n.acknowledged_migration_version)||typeof n.title!=='string'||typeof n.body!=='string'||n.title.length>160||n.body.length>16384)throw malformed();
 if(n.state==='UNVERIFIED'||n.state==='NOT_REQUIRED'){
  if(n.required_migration_version!=='0'||n.acknowledged_migration_version!=='0'||n.acknowledged_at!==null||n.completed_at!==null||n.title!==''||n.body!=='')throw malformed();
 }else{
  if(n.required_migration_version==='0'||!n.title||!n.body||!date(n.completed_at))throw malformed();
  if(n.state==='ACKNOWLEDGED'?(n.acknowledged_migration_version!==n.required_migration_version||!date(n.acknowledged_at)):(n.acknowledged_migration_version!=='0'||n.acknowledged_at!==null))throw malformed();
 }
 return n;
}
export async function readAccessGate(client:ApiClient,route:string):Promise<AccessGateView>{
 if(!normalizeRouteIntent(route))throw malformed();
 const value=await client.request<AccessGateView>('/platform/v1/access-gate?route='+encodeURIComponent(route));
 if(!value||value.user_id!==String(client.getSnapshot().user?.id)||value.route!==route||!stages.includes(value.stage))throw malformed();
 if(value.stage==='MIGRATION_REQUIRED'){
  const notice=checkedNotice(value.migration_notice!,value.user_id);if(notice.state!=='REQUIRED')throw malformed();
 }else if(value.migration_notice!==undefined)throw malformed();
 return value;
}
export async function acknowledgeMigrationNotice(client:ApiClient,version:string):Promise<MigrationNotice>{
 if(!/^[1-9][0-9]{0,18}$/.test(version))throw malformed();
 const notice=checkedNotice(await client.request<MigrationNotice>('/platform/v1/migration-notice/acknowledge','POST',{version}),String(client.getSnapshot().user?.id));
 if(notice.state!=='ACKNOWLEDGED'||notice.acknowledged_migration_version!==version)throw malformed();
 return notice;
}
