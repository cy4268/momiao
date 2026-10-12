import { useEffect, useState } from 'react';
import { useLocation } from 'react-router-dom';
import { ApiClient } from './api';

type Notice={id:string;scopes:string[];message:string};
const scopeText:Record<string,string>={CHALDEA_USER_WRITES:'平台新操作',WALLET_EXCHANGE:'钱包兑换',REWARDS:'奖励领取',DIRECT_PLAY_NEW_ROUNDS:'新游戏回合',POKER_NEW_TABLES_NEW_HANDS:'Poker 入桌与新手牌',RANKINGS_PUBLISHING:'排行榜更新',ANNOUNCEMENTS_SCHEDULING:'公告定时发布'};
export function CriticalNotice({client}:{client:ApiClient}){
 const location=useLocation(),[items,setItems]=useState<Notice[]>([]),[unavailable,setUnavailable]=useState(false);
 useEffect(()=>{let live=true,running=false;
  async function read(){if(running||document.hidden)return;running=true;try{const data=await client.request<{items:Notice[]}>('/api/v1/maintenance/notices');if(live){setItems(data.items);setUnavailable(false)}}catch{if(live)setUnavailable(true)}finally{running=false}}
  void read();const interval=window.setInterval(()=>{void read()},60000);const visible=()=>{void read()};document.addEventListener('visibilitychange',visible);return()=>{live=false;window.clearInterval(interval);document.removeEventListener('visibilitychange',visible)};
 },[client,location.pathname]);
 if(!items.length)return null;
 return <aside className="critical-notice" aria-label="维护与服务影响" role="status">{unavailable?<p>维护状态暂时无法刷新。以下为上次读取的信息，请以操作接口返回为准。</p>:null}{items.map(item=><div key={item.id}><strong>维护进行中</strong><p>{item.scopes.map(scope=>scopeText[scope]).join('、')}暂时停止接收新工作。已受理工作的处理与安全退出继续。</p>{item.message&&<p>{item.message}</p>}</div>)}</aside>;
}
