import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Link, Navigate, useLocation } from 'react-router-dom';
import { ApiClient, type User } from './api';
import { MasterProfile } from './MasterProfile';
import { parseProfile } from './profile-api';
import { consumeRouteIntent, peekRouteIntent, saveRouteIntent } from './post-auth-intent';
import { acknowledgeMigrationNotice, readAccessGate, type AccessGateView } from './access-gate-api';
import { Alert, Brand, Loading } from './ui';

export function LoginRequired() {
 const location=useLocation();
 useEffect(()=>saveRouteIntent(location.pathname+location.search),[location.pathname,location.search]);
 return <Navigate to="/login" replace/>;
}
export function PostAuthGate({client,user}:{client:ApiClient;user:User}){
 return <GateSurface client={client} user={user} route={peekRouteIntent()} postAuth/>;
}
export function AccessGate({client,user,route,children}:{client:ApiClient;user:User;route:string;children:ReactNode}){
 return <GateSurface client={client} user={user} route={route}>{children}</GateSurface>;
}
const messages:Record<string,string>={
 ACCOUNT_RESTRICTED:'账户当前受到访问限制。请退出登录，或通过既有支持入口核对账户状态。',
 MIGRATION_UNVERIFIED:'迁移适用状态尚未核实。本站不会把缺少迁移记录解释为迁移已经完成。',
 ROLE_DENIED:'当前账户未具备此页面所需权限。原生管理员与平台运营权限保持独立。',
 ROLE_UNVERIFIED:'当前权限状态尚未核实，请重新读取。',
 RESOURCE_UNAVAILABLE:'目标功能当前未开放，请选择已开放入口。',
 RESOURCE_UNVERIFIED:'目标功能的开放状态尚未核实，请重新读取。',
 MAINTENANCE:'目标功能正在维护，恢复开放后再继续。',
};
function GateSurface({client,user,route,postAuth=false,children}:{client:ApiClient;user:User;route:string;postAuth?:boolean;children?:ReactNode}){
 const [view,setView]=useState<AccessGateView>();const [loading,setLoading]=useState(true);const [error,setError]=useState('');const [destination,setDestination]=useState('');
 const [saving,setSaving]=useState(false);const [reconcile,setReconcile]=useState(false);const active=useRef(true);const loadLock=useRef(false);const writeLock=useRef(false);
 async function load(){if(loadLock.current)return;loadLock.current=true;setLoading(true);setError('');setView(undefined);const generation=client.getSessionGeneration();
  try{
   const next=await readAccessGate(client,route);if(!active.current||generation!==client.getSessionGeneration())return;
   // Preserve M2's idempotent durable provisional profile, only after native
   // active status is verified. This is never part of migration notice ACK.
   if(next.stage==='MASTER_REQUIRED'){
    const config=await client.admissionConfig();if(config.enabled)parseProfile(await client.request('/platform/v1/admission/ensure','POST',{}),String(user.id));
    if(!active.current||generation!==client.getSessionGeneration())return;
   }
   setReconcile(false);
   if(next.stage==='READY'&&postAuth){
    if(consumeRouteIntent()!==route){setError('返回入口已发生变化，请重新核对访问状态。');return;}
    setDestination(route);return;
   }
   setView(next);
  }catch{if(active.current&&generation===client.getSessionGeneration())setError('访问状态尚未核实。请重新读取；当前不会自动放行或重放先前操作。');}
  finally{loadLock.current=false;if(active.current)setLoading(false);}
 }
 async function acknowledge(){if(writeLock.current||!view?.migration_notice||reconcile)return;writeLock.current=true;setSaving(true);setError('');
  try{await acknowledgeMigrationNotice(client,view.migration_notice.required_migration_version);if(active.current)await load();}
  catch{if(active.current){setReconcile(true);setError('确认结果尚未核实。请先重新核对访问状态；既有确认记录将保留原时间。');}}
  finally{writeLock.current=false;if(active.current)setSaving(false);}
 }
 useEffect(()=>{active.current=true;document.title='访问状态核对 · momiao';void load();return()=>{active.current=false;};},[client,user.id,route]);
 if(destination)return <Navigate to={destination} replace/>;
 if(!loading&&view?.stage==='READY')return <>{children}</>;
 const notice=view?.migration_notice;
 return <main className="welcome-page"><Link to="/" className="brand"><Brand/></Link>
  <div className="welcome-steps"><strong>01 核对账户</strong><span>02 Master 身份</span><span>03 迁移确认</span><span>04 权限与开放状态</span></div>
  {loading&&<Loading/>}
  {error&&<Alert>{error}</Alert>}
  {!loading&&view?.stage==='MASTER_REQUIRED'&&<MasterProfile client={client} user={user} onSaved={()=>void load()}/>}
  {!loading&&view?.stage==='MIGRATION_REQUIRED'&&notice&&<section aria-label="迁移确认"><p className="eyebrow">独立迁移通知 · 版本 {notice.required_migration_version}</p><h1>{notice.title}</h1><p style={{whiteSpace:'pre-wrap'}}>{notice.body}</p><p className="hint">此确认只记录你已了解已完成的迁移事实；不会重置额度、发放赠额、创建资料或迁移密钥。</p><button disabled={saving||reconcile} onClick={()=>void acknowledge()}>{saving?'正在确认…':'我已了解，继续'}</button></section>}
  {!loading&&view&&messages[view.stage]&&<Alert>{messages[view.stage]}</Alert>}
  {!loading&&<div className="auth-actions"><button disabled={saving} onClick={()=>void load()}>重新核对访问状态</button>{view?.stage!=='ACCOUNT_RESTRICTED'&&route!=='/dashboard'&&<Link to="/dashboard">返回指挥台入口</Link>}<button disabled={saving} onClick={()=>void client.logout().catch(()=>{})}>退出登录</button></div>}
 </main>;
}