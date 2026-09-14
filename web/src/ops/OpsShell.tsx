import { useEffect, useRef } from 'react';
import { Link, NavLink, Outlet, useLocation, useOutletContext } from 'react-router-dom';
import { ApiClient } from '../api';
import { Alert, Brand, Loading, useResource } from '../ui';
import { opsError, readOpsBootstrap, type OpsBootstrap } from './ops-api';
import './ops.css';

export const opsPages=[
 ['/ops','运营总览','operations.read'],['/ops/models','模型目录','models.read'],['/ops/announcements','公告','announcements.read'],
 ['/ops/games','游戏','games.read'],['/ops/poker','Poker','poker.read'],['/ops/economy','经济与账务','economy.read'],['/ops/rewards','奖励','rewards.read'],
 ['/ops/rankings','排行榜','rankings.read'],['/ops/users','用户','users.read'],['/ops/records','记录查询','records.read'],['/ops/support-cases','支持工单','support-cases.read'],['/ops/incidents','事件处理','incidents.read'],
 ['/ops/maintenance','维护','maintenance.read'],['/ops/service-health','服务健康','service-health.read'],['/ops/jobs','后台任务','jobs.read'],['/ops/attention','待处理事项','attention.read'],
 ['/ops/operations','操作记录','operations.read'],['/ops/audit','审计','audit.read'],['/ops/access-control','访问控制','access_control.read'],
] as const;
export type OpsContext={client:ApiClient;bootstrap:OpsBootstrap;reload:()=>void};
export const useOps=()=>useOutletContext<OpsContext>();
export function OpsShell({client}:{client:ApiClient}){
 const location=useLocation(),main=useRef<HTMLElement>(null);
 const resource=useResource(()=>readOpsBootstrap(client).catch(error=>{throw new Error(opsError(error));}),[client,location.pathname]);
 const title=opsPages.find(([path])=>path===location.pathname)?.[1]||'操作详情';
 useEffect(()=>{document.title=`${title} · Chaldea Ops`;main.current?.focus();},[title,location.pathname]);
 const allowed=opsPages.filter(([, ,permission])=>resource.data?.principal.permissions.includes(permission));
 return <div className="ops-shell"><a href="#ops-main" className="skip-link">跳至运营内容</a><header className="ops-header"><Link to="/" className="brand"><Brand/></Link><span className="ops-badge">OPERATIONS</span><Link to="/me">返回个人中心</Link></header><div className="ops-layout">
 <aside className="ops-sidebar"><p className="eyebrow">WORKSPACE</p><nav aria-label="运营导航">{allowed.map(([path,label])=><NavLink end={path==='/ops'} key={path} to={path}>{label}</NavLink>)}</nav>{resource.data&&<p className="hint">{({SUPER_ADMIN:'最高管理员',OPERATOR:'运营人员',AUDITOR:'审计员'})[resource.data.principal.base_role]}</p>}</aside>
 <main id="ops-main" ref={main} tabIndex={-1} className="ops-main">{resource.loading?<Loading/>:resource.error?<><Alert>{resource.error}</Alert><button onClick={resource.reload}>重新核对权限</button></>:resource.data?<Outlet context={{client,bootstrap:resource.data,reload:resource.reload} satisfies OpsContext}/>:null}</main></div></div>;
}
export function OpsHome(){const {bootstrap}=useOps();return <><header className="page-heading"><div><p className="eyebrow">CHALDEA / OPERATIONS</p><h1>运营工作台</h1><p>查看业务状态，核对操作影响，并追踪处理结果。</p></div></header><div className="ops-entry-grid">{opsPages.filter(([path,,permission])=>path!=='/ops'&&bootstrap.principal.permissions.includes(permission)).map(([path,label])=><Link className="panel ops-entry" to={path} key={path}><strong>{label}</strong><span aria-hidden="true">↗</span></Link>)}</div></>;}
