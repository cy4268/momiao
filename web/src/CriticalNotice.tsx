import { useEffect, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { ApiClient } from './api';

type Notice={id:string;scopes:string[];scheduled_end_at:string|null;estimated_end_at:string|null;announcement_id:string|null};
const scopeText:Record<string,string>={CHALDEA_USER_WRITES:'平台新操作',WALLET_EXCHANGE:'钱包兑换',REWARDS:'奖励领取',DIRECT_PLAY_NEW_ROUNDS:'新游戏回合',POKER_NEW_TABLES_NEW_HANDS:'Poker 入桌与新手牌',RANKINGS_PUBLISHING:'排行榜更新',ANNOUNCEMENTS_SCHEDULING:'公告定时发布'};
const uuid=(value:unknown):value is string=>typeof value==='string'&&/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(value);
export function CriticalNotice({client}:{client:ApiClient}){
 const location=useLocation(),[items,setItems]=useState<Notice[]>([]),[unavailable,setUnavailable]=useState(false);
 useEffect(()=>{let live=true,running=false;
  async function read(){if(running||document.hidden)return;running=true;try{const data=await client.request<{items:Notice[]}>('/api/v1/maintenance/notices');if(!data||!Array.isArray(data.items)||data.items.length>8||data.items.some(item=>!item||!uuid(item.id)||!Array.isArray(item.scopes)||item.scopes.some(scope=>!scopeText[scope])||item.announcement_id!==null&&!uuid(item.announcement_id)||[item.scheduled_end_at,item.estimated_end_at].some(time=>time!==null&&!Number.isFinite(Date.parse(time)))))throw new Error('INVALID_NOTICE');if(live){setItems(data.items);setUnavailable(false)}}catch{if(live)setUnavailable(true)}finally{running=false}}
  void read();const interval=window.setInterval(()=>{void read()},60000);const visible=()=>{void read()};document.addEventListener('visibilitychange',visible);return()=>{live=false;window.clearInterval(interval);document.removeEventListener('visibilitychange',visible)};
 },[client,location.pathname]);
 if(!items.length)return null;
 return <aside className="critical-notice" aria-label="维护与服务影响" role="status">{unavailable?<p>维护状态暂时无法刷新。以下为上次读取的信息，请以操作接口返回为准。</p>:null}{items.map(item=><div key={item.id}><strong>维护进行中</strong><p>{item.scopes.map(scope=>scopeText[scope]).join('、')}暂时停止接收新工作。已受理工作的处理与安全退出继续。</p>{item.scheduled_end_at?<span>计划结束：{new Date(item.scheduled_end_at).toLocaleString('zh-CN')}</span>:item.estimated_end_at?<span>预计结束：{new Date(item.estimated_end_at).toLocaleString('zh-CN')}，实际结束以状态更新为准。</span>:<span>结束时间待确认。</span>}{item.announcement_id&&<Link to={`/announcements/${item.announcement_id}`}>查看维护公告</Link>}</div>)}</aside>;
}
