import { useEffect, useRef, useState } from 'react';
import { Link, NavLink, Outlet, useLocation, useOutletContext } from 'react-router-dom';
import { ApiClient } from '../api';
import { Alert, Brand, Loading, useResource } from '../ui';
import { opsError, readOpsBootstrap, type OpsBootstrap } from './ops-api';
import { assetUrl } from '../game-hall-assets';
import art from './ops-art.json';
import './ops.css';
import './ops-chamber.css';
import './ops-workbench.css';

export const opsPages=[
 ['/ops','运营总览','operations.read'],['/ops/models','模型目录','models.read'],['/ops/announcements','公告','announcements.read'],
 ['/ops/games','游戏','games.read'],['/ops/poker','Poker','poker.read'],['/ops/economy','经济与账务','economy.read'],['/ops/rewards','奖励','rewards.read'],
 ['/ops/rankings','排行榜','rankings.read'],['/ops/users','用户','users.read'],['/ops/records','记录查询','records.read'],['/ops/support-cases','支持工单','support-cases.read'],['/ops/incidents','事件处理','incidents.read'],
 ['/ops/maintenance','维护','maintenance.read'],['/ops/service-health','服务健康','service-health.read'],['/ops/jobs','后台任务','jobs.read'],['/ops/attention','待处理事项','attention.read'],
 ['/ops/operations','操作记录','operations.read'],['/ops/audit','审计','audit.read'],['/ops/access-control','访问控制','access_control.read'],
] as const;
const opsGroups=[
 {title:'内容与用户',paths:['/ops/models','/ops/announcements','/ops/users','/ops/support-cases']},
 {title:'业务管理',paths:['/ops/games','/ops/poker','/ops/economy','/ops/rewards','/ops/rankings','/ops/records']},
 {title:'系统与审计',paths:['/ops/incidents','/ops/maintenance','/ops/service-health','/ops/jobs','/ops/attention','/ops/operations','/ops/audit','/ops/access-control']},
];
function visibleGroups(permissions:string[]){
 return opsGroups.map(group=>({...group,pages:group.paths.flatMap(path=>opsPages.filter(page=>page[0]===path&&permissions.includes(page[2])))})).filter(group=>group.pages.length);
}
export type OpsContext={client:ApiClient;bootstrap:OpsBootstrap;reload:()=>void};
export const useOps=()=>useOutletContext<OpsContext>();
export function OpsShell({client}:{client:ApiClient}){
 const location=useLocation(),main=useRef<HTMLElement>(null);
 const [menuOpen,setMenuOpen]=useState(false);
 const [artFailed,setArtFailed]=useState(false);
 const resource=useResource(()=>readOpsBootstrap(client).catch(error=>{throw new Error(opsError(error));}),[client,location.pathname]);
 const title=opsPages.find(([path])=>path===location.pathname)?.[1]||'操作详情';
 useEffect(()=>{document.title=`${title} · Chaldea Ops`;setMenuOpen(false);main.current?.focus();},[title,location.pathname]);
 const permissions=resource.data?.principal.permissions||[],groups=visibleGroups(permissions);
 const overview=location.pathname.replace(/\/+$/,'')==='/ops';
 return <div className={'ops-shell'+(overview?' is-overview':'')}>
 <a href="#ops-main" className="skip-link">跳至运营内容</a>
 <header className="ops-header"><Link to="/" className="brand" aria-label="Chaldea Platform 首页"><Brand/></Link><span className="ops-badge">OPERATIONS</span><div className="ops-account"><Link to="/me">返回个人中心</Link>{resource.data&&<span>{({SUPER_ADMIN:'最高管理员',OPERATOR:'运营人员',AUDITOR:'审计员'})[resource.data.principal.base_role]}</span>}</div></header>
 <div className="ops-layout"><aside className="ops-sidebar">
 <button className="ops-menu-toggle" aria-expanded={menuOpen} aria-controls="ops-navigation" onClick={()=>setMenuOpen(open=>!open)}>{menuOpen?'收起运营导航':'展开运营导航'}<span aria-hidden="true">{menuOpen?'−':'+'}</span></button>
 <nav id="ops-navigation" className={menuOpen?'is-open':''} aria-label="运营导航">
 {permissions.includes(opsPages[0][2])&&<NavLink end to="/ops">运营总览</NavLink>}
 {groups.map(group=><div className="ops-nav-group" key={group.title}><p>{group.title}</p>{group.pages.map(([path,label])=><NavLink key={path} to={path}>{label}</NavLink>)}</div>)}
 </nav></aside>
 <main id="ops-main" ref={main} tabIndex={-1} className="ops-main">{!artFailed&&<img className="ops-backdrop" src={assetUrl(art.background)} alt="" decoding="async" onError={()=>setArtFailed(true)}/>}<div className={'ops-workspace'+(overview?' ops-overview-workspace':' ops-workbench')}>{resource.loading?<Loading/>:resource.error?<><Alert>{resource.error}</Alert><button onClick={resource.reload}>重新核对权限</button></>:resource.data?<Outlet context={{client,bootstrap:resource.data,reload:resource.reload} satisfies OpsContext}/>:null}</div></main></div></div>;
}
export function OpsHome(){
 const {bootstrap}=useOps(),groups=visibleGroups(bootstrap.principal.permissions);
 return <div className="ops-overview"><header className="page-heading"><div><p className="eyebrow">CHALDEA / OPERATIONS</p><h1>运营工作台</h1><p>查看业务状态，核对操作影响，并追踪处理结果。</p></div></header>
 {groups.map((group,index)=><section className="ops-entry-group" key={group.title} aria-labelledby={'ops-group-'+index}><h2 id={'ops-group-'+index}>{group.title}</h2><div className="ops-entry-grid">{group.pages.map(([path,label])=><Link className="ops-entry" to={path} key={path}><strong>{label}</strong><span aria-hidden="true">↗</span></Link>)}</div></section>)}
 <p className="ops-directory-note" role={groups.length?undefined:'status'}>{groups.length?'按当前权限显示可访问模块':'当前没有可访问的运营模块。'}</p></div>;
}
