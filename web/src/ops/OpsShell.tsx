import { useEffect, useRef, useState } from 'react';
import { Link, NavLink, Outlet, useLocation } from 'react-router-dom';
import { ApiClient, type User } from '../api';
import { Alert, Brand } from '../ui';
import { assetUrl } from '../game-hall-assets';
import art from './ops-art.json';
import './ops.css';
import './ops-chamber.css';
import './ops-workbench.css';
import './components/components.css';

const opsPages=[
 ['/ops','运营总览'],['/ops/models','模型目录'],['/ops/channels','渠道'],['/ops/groups','分组与可用性'],['/ops/announcements','公告'],
 ['/ops/games','游戏'],['/ops/poker','Poker'],['/ops/economy','经济与账务'],['/ops/rewards','奖励'],
 ['/ops/logs','调用日志'],['/ops/rankings','排行榜'],['/ops/users','用户'],['/ops/records','记录查询'],['/ops/support-cases','支持工单'],['/ops/incidents','事件处理'],
 ['/ops/maintenance','维护'],['/ops/service-health','服务健康'],['/ops/jobs','后台任务'],['/ops/attention','待处理事项'],
 ['/ops/audit','操作日志'],
] as const;
const opsGroups=[
 {title:'内容与用户',paths:['/ops/models','/ops/channels','/ops/groups','/ops/announcements','/ops/users','/ops/support-cases']},
 {title:'业务管理',paths:['/ops/games','/ops/poker','/ops/economy','/ops/rewards','/ops/rankings','/ops/records','/ops/logs']},
 {title:'系统与审计',paths:['/ops/incidents','/ops/maintenance','/ops/service-health','/ops/jobs','/ops/attention','/ops/audit']},
];
function visibleGroups(){
 return opsGroups.map(group=>({...group,pages:group.paths.flatMap(path=>opsPages.filter(page=>page[0]===path))})).filter(group=>group.pages.length);
}
export function OpsShell({client,user}:{client:ApiClient;user:User}){
 const location=useLocation(),main=useRef<HTMLElement>(null);
 const [menuOpen,setMenuOpen]=useState(false);
 const [artFailed,setArtFailed]=useState(false);
 const title=opsPages.find(([path])=>path===location.pathname||(path!=='/ops'&&location.pathname.startsWith(path+'/')))?.[1]||(location.pathname==='/ops/gambler-campaign'?'赌怪一次性活动':'操作详情');
 useEffect(()=>{document.title=`${title} · Chaldea Ops`;setMenuOpen(false);main.current?.focus();},[title,location.pathname]);
 const groups=visibleGroups();
 const overview=location.pathname.replace(/\/+$/,'')==='/ops';
 if(user.role<10)return <Alert>仅管理员可以进入运营台。</Alert>;
 return <div className={'ops-shell'+(overview?' is-overview':'')}>
 <a href="#ops-main" className="skip-link">跳至运营内容</a>
 <header className="ops-header"><Link to="/" className="brand" aria-label="Chaldea Platform 首页"><Brand/></Link><span className="ops-badge">OPERATIONS</span><div className="ops-account"><Link to="/me">返回个人中心</Link><span>{user.role>=100?'最高管理员':'管理员'}</span></div></header>
 <div className="ops-layout"><aside className="ops-sidebar">
 <button className="ops-menu-toggle" aria-expanded={menuOpen} aria-controls="ops-navigation" onClick={()=>setMenuOpen(open=>!open)}>{menuOpen?'收起运营导航':'展开运营导航'}<span aria-hidden="true">{menuOpen?'−':'+'}</span></button>
 <nav id="ops-navigation" className={menuOpen?'is-open':''} aria-label="运营导航">
 <NavLink end to="/ops">运营总览</NavLink>
 {groups.map(group=><div className="ops-nav-group" key={group.title}><p>{group.title}</p>{group.pages.map(([path,label])=><NavLink key={path} to={path}>{label}</NavLink>)}</div>)}
 </nav></aside>
 <main id="ops-main" ref={main} tabIndex={-1} className="ops-main">{!artFailed&&<img className="ops-backdrop" src={assetUrl(art.background)} alt="" decoding="async" onError={()=>setArtFailed(true)}/>}<div className={'ops-workspace'+(overview?' ops-overview-workspace':' ops-workbench')}><Outlet context={{client}}/></div></main></div></div>;
}
export function OpsHome(){
 const groups=visibleGroups();
 return <div className="ops-overview"><header className="page-heading"><div><p className="eyebrow">CHALDEA / OPERATIONS</p><h1>运营工作台</h1><p>查看业务状态，管理内容与账户。</p></div></header>
 {groups.map((group,index)=><section className="ops-entry-group" key={group.title} aria-labelledby={'ops-group-'+index}><h2 id={'ops-group-'+index}>{group.title}</h2><div className="ops-entry-grid">{group.pages.map(([path,label])=><Link className="ops-entry" to={path} key={path}><strong>{label}</strong><span aria-hidden="true">↗</span></Link>)}</div></section>)}
 <p className="ops-directory-note" role={groups.length?undefined:'status'}>管理员可以使用全部运营模块。</p></div>;
}
