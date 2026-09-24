import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import { Link, Navigate, useLocation } from 'react-router-dom';
import { ApiClient, type User } from './api';
import { MasterProfile } from './MasterProfile';
import { parseProfile } from './profile-api';
import { consumeRouteIntent, peekRouteIntent, pokerRouteIntent, rouletteRouteIntent, saveRouteIntent, normalizeRouteIntent } from './post-auth-intent';
import { acknowledgeMigrationNotice, readAccessGate, type AccessGateView } from './access-gate-api';
import { Alert, Brand, Loading } from './ui';

export interface PokerGatePolicy {stage:AccessGateView['stage']|undefined;recovery_only:boolean;mutation_blocked:boolean}
const PokerGateContext=createContext<PokerGatePolicy>({stage:undefined,recovery_only:true,mutation_blocked:true});
export const usePokerGatePolicy=()=>useContext(PokerGateContext);
type GateScope={client:ApiClient;userID:number;generation:number;route:string};

export function LoginRequired() {
 const location=useLocation();
 useEffect(()=>saveRouteIntent(normalizeRouteIntent(location.pathname+location.search)||location.pathname),[location.pathname,location.search]);
 return <Navigate to="/login" replace/>;
}
export function PostAuthGate({client,user}:{client:ApiClient;user:User}){
 return <GateSurface client={client} user={user} route={peekRouteIntent()} postAuth pokerRecovery/>;
}
export function AccessGate({client,user,route,children,pokerRecovery=false}:{client:ApiClient;user:User;route:string;children:ReactNode;pokerRecovery?:boolean}){
 return <GateSurface client={client} user={user} route={route} pokerRecovery={pokerRecovery}>{children}</GateSurface>;
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
function GateSurface({client,user,route,postAuth=false,pokerRecovery=false,children}:{client:ApiClient;user:User;route:string;postAuth?:boolean;pokerRecovery?:boolean;children?:ReactNode}){
 const [loaded,setLoaded]=useState<{value:AccessGateView;scope:GateScope}>();const [loading,setLoading]=useState(true);const [error,setError]=useState('');const [destination,setDestination]=useState<GateScope>();
 const [admitted,setAdmitted]=useState<GateScope>();const revision=useRef(0);const generation=client.getSessionGeneration();const scope=useRef<GateScope>({client,userID:user.id,generation,route});
 scope.current={client,userID:user.id,generation,route};const poker=pokerRecovery&&pokerRouteIntent(route),roulette=rouletteRouteIntent(route);
 const matches=(s:GateScope|undefined)=>!!s&&s.client===scope.current.client&&s.userID===scope.current.userID&&s.generation===scope.current.generation&&s.route===scope.current.route&&s.generation===s.client.getSessionGeneration();
 const view=loaded&&matches(loaded.scope)?loaded.value:undefined;
 const [saving,setSaving]=useState(false);const [reconcile,setReconcile]=useState(false);const active=useRef(true);const loadLock=useRef(false);const writeLock=useRef(false);
 async function load(){if(loadLock.current)return;loadLock.current=true;setLoading(true);setError('');setLoaded(undefined);const start=scope.current;const request=++revision.current;
  const valid=()=>active.current&&request===revision.current&&matches(start);
  try{
   const next=await readAccessGate(client,route);if(!valid())return;
   setAdmitted((poker||roulette)&&(next.stage==='READY'||next.stage==='MAINTENANCE')?start:undefined);
   // Preserve M2's idempotent durable provisional profile, only after native
   // active status is verified. This is never part of migration notice ACK.
   if(next.stage==='MASTER_REQUIRED'){
    const config=await client.admissionConfig();if(!valid())return;if(config.enabled)parseProfile(await client.request('/platform/v1/admission/ensure','POST',{}),String(user.id));
    if(!valid())return;
   }
   setReconcile(false);
   if((next.stage==='READY'||((poker||roulette)&&next.stage==='MAINTENANCE'))&&postAuth){
    if(consumeRouteIntent()!==route){setError('返回入口已发生变化，请重新核对访问状态。');return;}
    setDestination(start);return;
   }
   setLoaded({value:next,scope:start});
  }catch{if(valid())setError('访问状态尚未核实。请重新读取；当前不会自动放行或重放先前操作。');}
  finally{if(valid()){loadLock.current=false;setLoading(false);}}
 }
 async function acknowledge(){if(writeLock.current||!view?.migration_notice||reconcile)return;writeLock.current=true;setSaving(true);setError('');const start=scope.current;
  try{await acknowledgeMigrationNotice(client,view.migration_notice.required_migration_version);if(active.current&&matches(start))await load();}
  catch{if(active.current&&matches(start)){setReconcile(true);setError('确认结果尚未核实。请先重新核对访问状态；既有确认记录将保留原时间。');}}
  finally{if(active.current&&matches(start)){writeLock.current=false;setSaving(false);}}
 }
 useEffect(()=>{active.current=true;setSaving(false);setReconcile(false);document.title='访问状态核对 · momiao';void load();return()=>{active.current=false;revision.current++;loadLock.current=false;writeLock.current=false;};},[client,user.id,route,generation]);
 if(destination&&matches(destination))return <Navigate to={destination.route} replace/>;
 // Page policy only tightens the UI; domain/controller guards still decide every operation.
 if(roulette&&matches(admitted))return <>{view?.stage==='MAINTENANCE'&&<aside aria-label="轮盘访问状态"><Alert>维护中：已开始对局继续，保留退款与回执核对；新开局暂时关闭。</Alert></aside>}{children}</>;
 if(poker&&matches(admitted))return <PokerGateContext.Provider value={{stage:view?.stage,recovery_only:view?.stage!=='READY',mutation_blocked:loading||!!error||!view}}>
  <aside aria-label="Poker 访问状态">{loading&&<Loading/>}{error&&<Alert>{error}</Alert>}{view?.stage==='MAINTENANCE'&&<Alert>仅保留既有对局、回执核对与安全退出入口；新开局等入口暂时关闭。</Alert>}<button disabled={loading||saving} onClick={()=>void load()}>重新核对访问状态</button></aside>
  {children}
 </PokerGateContext.Provider>;
 if(!loading&&view?.stage==='READY')return <>{children}</>;
 const notice=view?.migration_notice;
 return <main className="welcome-page"><Link to="/" className="brand"><Brand/></Link>
  <div className="welcome-steps"><strong>01 核对账户</strong><span>02 Master 身份</span><span>03 迁移确认</span><span>04 权限与开放状态</span></div>
  {loading&&<Loading/>}
  {error&&<Alert>{error}</Alert>}
  {!loading&&view?.stage==='MASTER_REQUIRED'&&<MasterProfile client={client} user={user} onSaved={()=>void load()} embedded/>}
  {!loading&&view?.stage==='MIGRATION_REQUIRED'&&notice&&<section aria-label="迁移确认"><p className="eyebrow">独立迁移通知 · 版本 {notice.required_migration_version}</p><h1>{notice.title}</h1><p style={{whiteSpace:'pre-wrap'}}>{notice.body}</p><p className="hint">此确认只记录你已了解已完成的迁移事实；不会重置额度、发放赠额、创建资料或迁移密钥。</p><button disabled={saving||reconcile} onClick={()=>void acknowledge()}>{saving?'正在确认…':'我已了解，继续'}</button></section>}
  {!loading&&view&&messages[view.stage]&&<Alert>{messages[view.stage]}</Alert>}
  {!loading&&<div className="auth-actions"><button disabled={saving} onClick={()=>void load()}>重新核对访问状态</button>{view?.stage!=='ACCOUNT_RESTRICTED'&&route!=='/dashboard'&&<Link to="/dashboard">返回指挥台入口</Link>}<button disabled={saving} onClick={()=>void client.logout().catch(()=>{})}>退出登录</button></div>}
 </main>;
}
