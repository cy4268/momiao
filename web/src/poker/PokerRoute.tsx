import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import type { ApiClient } from '../api';
import { usePokerGatePolicy } from '../PostAuthGate';
import { pokerRouteIntent } from '../post-auth-intent';
import { LivePokerLobby } from './LivePokerLobby';
import { LivePokerTable, type LivePokerTableProps } from './LivePokerTable';
import { capturePokerLobbyScope, currentPokerLobbyScope, readPokerHttpLobby } from './poker-lobby-read';
import type { LobbyFilters, PokerLobbyReadScope } from './poker-ui-types';

type Admission=LivePokerTableProps['admission'];
export function PokerRoute(p:{client:ApiClient;path:string;frame:(children:ReactNode)=>ReactNode;onHold:(owner:object,path:string|null)=>void}){
  const navigate=useNavigate(),policy=usePokerGatePolicy(),token=useRef({});
  const [admission,setAdmission]=useState<{table:string;value:Admission}>(),[error,setError]=useState(''),[reading,setReading]=useState(false);
  const alive=useRef(true),read=useRef<PokerLobbyReadScope|null>(null),generation=useRef(0),latest=useRef(p);
  latest.current=p;
  const valid=pokerRouteIntent(p.path),table=valid&&p.path!=='/poker'?p.path.slice('/poker/table/'.length):null;
  const native=p.client.getSessionGeneration(),user=p.client.getSnapshot().user?.id;
  const current=()=>alive.current&&latest.current.client===p.client&&p.client.getSessionGeneration()===native&&p.client.getSnapshot().user?.id===user&&!p.client.getSnapshot().loggingOut;
  function permitted(id:string,a:Admission){
    if(!current()||!currentPokerLobbyScope(p.client,a.scope,()=>a.scope)||a.snapshot.viewer.user_id!==a.scope.user_id||!pokerRouteIntent('/poker/table/'+id))return false;
    const active=a.snapshot.active_session;
    return active?active.table_id===id&&active.can_reconnect:a.snapshot.tables.some(row=>row.table_id===id&&row.can_spectate);
  }
  function enter(id:string,value:Admission){
    if(!permitted(id,value))return;
    read.current=null;generation.current++;
    setAdmission({table:id,value});setError('');
    p.onHold(token.current,value.snapshot.active_session?'/poker/table/'+id:null);
    navigate('/poker/table/'+id);
  }
  function returnLobby(){if(!current())return;read.current=null;generation.current++;setAdmission(undefined);setError('');p.onHold(token.current,null);navigate('/poker');}
  useEffect(()=>{alive.current=true;return()=>{alive.current=false;read.current=null;generation.current++;p.onHold(token.current,null);};},[p.client,native,user]);
  useEffect(()=>{
    document.title=(table?'Poker 牌桌':'Poker 大厅')+' · momiao';
    if(!valid||!table){setAdmission(undefined);return;}
    if(admission?.table===table)return;
    const requested=++generation.current;
    const query:LobbyFilters={query:table,visibility:'ALL',open_seats_only:false,max_seats:'ALL',blind_preset_id:'ALL',lifecycle_state:'ALL',spectators_only:false,sort:'LOW_BLIND',limit:50,cursor:null};
    setReading(true);setError('');setAdmission(undefined);
    const same=()=>current()&&requested===generation.current&&latest.current.path==='/poker/table/'+table;
    void(async()=>{
      try{
        const scope=capturePokerLobbyScope(p.client,query,requested);read.current=scope;
        const snapshot=await readPokerHttpLobby(p.client,scope,()=>same()?read.current:null,query);
        if(!snapshot||!same())return;
        const value={scope,snapshot};
        if(permitted(table,value)){setAdmission({table,value});p.onHold(token.current,snapshot.active_session?'/poker/table/'+table:null);}
        else setError('当前牌桌入口尚未获准，请回大厅核对当前会话或访问权限。');
      }catch{if(same())setError('牌桌入口尚未核实，请返回大厅重新读取。');}
      finally{if(same())setReading(false);}
    })();
    return()=>{if(requested===generation.current){generation.current++;read.current=null;}};
  },[p.client,p.path,native,user]);
  if(!valid)return <section className="pk-page"><p role="alert">牌桌入口格式无效，请返回大厅。</p><button onClick={returnLobby}>返回大厅</button></section>;
  if(table){
    if(admission?.table===table&&permitted(table,admission.value))return <LivePokerTable client={p.client} table_id={table} admission={admission.value} mutation_blocked={policy.mutation_blocked} onReturnLobby={returnLobby}/>;
    return <section className="pk-page" role="status"><p>{reading?'正在核对牌桌入口。':error||'等待完整牌桌入口状态。'}</p><button onClick={returnLobby}>返回大厅</button></section>;
  }
  return p.frame(<LivePokerLobby client={p.client} policy={policy} onEnter={enter}
    onNavigate={destination=>{if(current())navigate('/'+destination);}}
    onPendingChange={pending=>{if(current())p.onHold(token.current,pending?'/poker':null);}}/>);
}
