import { useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore } from 'react';
import type { ApiClient } from '../api';
import { PokerTable } from './PokerTable';
import { PokerTableClient } from './poker-socket';
import { pokerStreamAuthority, pokerStreamContext } from './poker-stream';
import { capturePokerReadScope, currentPokerScope } from './poker-http';
import { matchesPokerIntentContext, type PokerReadScope } from './poker-client-boundary';
import { readPokerHttpSession } from './poker-session';
import { pokerUUID } from './poker-view';
import { readPokerHostReceipt, submitPokerHostCommand, type PokerHostHttpCommand } from './poker-host-http';
import type { PokerAuthority, PokerHostCommand, PokerHostOperationView, PokerIntentContext, PokerLobbyReadScope, PokerLobbySnapshot, TableIntent, TableUi, TableView } from './poker-ui-types';
export interface LivePokerTableProps {
  client:ApiClient;table_id:string;admission:{snapshot:PokerLobbySnapshot;scope:PokerLobbyReadScope};mutation_blocked:boolean;onReturnLobby:()=>void;
}
interface Owner {api:ApiClient;key:string;client:PokerTableClient;scope:PokerReadScope;alive:boolean}
interface Exit {owner:Owner;session:string;finished:boolean;invalidated?:boolean}
interface Flight {exit:Exit;scope:PokerReadScope;promise:Promise<void>}
interface HostOperation extends PokerHostOperationView {scope:PokerReadScope;original:PokerHostHttpCommand;receipt_version?:string}
const emptyUi=():TableUi=>({bet:{action_type:'RAISE',amount_chips:''},top_up_chips:'',confirm_leave:false,confirm_takeover:false});
const subscribeNone=()=>()=>{},snapshotNone=()=>null;
const unavailable='当前操作尚未确认，请核对原回执或重新连接。';
function hostCommandCurrent(table:TableView|undefined,command:PokerHostCommand):boolean {
  const host=table?.host;if(!host?.capabilities.includes(command.command))return false;
  if(command.command==='PAUSE_ACCEPTING_PLAYERS'||command.command==='RESUME_ACCEPTING_PLAYERS'||command.command==='CLOSE_TABLE')return command.target_session_id===undefined&&command.target_user_id===undefined;
  if(command.command==='REMOVE_PLAYER_AFTER_HAND')return host.players.some(player=>player.target_session_id===command.target_session_id&&player.target_user_id===command.target_user_id&&!player.removal_pending);
  if(command.command==='REMOVE_SPECTATOR')return host.spectators.some(spectator=>spectator.target_user_id===command.target_user_id);
  if(command.command==='MUTE_CHAT_USER')return [...host.players,...host.spectators,...host.chat_targets].some(target=>target.target_user_id===command.target_user_id&&!target.muted&&(!command.target_session_id||'target_session_id' in target&&target.target_session_id===command.target_session_id));
  return false;
}
/** Accepted entry admission is retained by the parent; background loading is only a soft policy gate. */
export function LivePokerTable(p:LivePokerTableProps){
  const auth=useSyncExternalStore(p.client.subscribe,p.client.getSnapshot,p.client.getSnapshot),generation=p.client.getSessionGeneration();
  const {snapshot,scope}=p.admission,active=snapshot.active_session;
  const admitted=auth.ready&&!auth.loggingOut&&!!auth.user&&Number.isSafeInteger(auth.user.id)&&auth.user.id>0&&pokerUUID(p.table_id)&&
    scope.user_id===String(auth.user.id)&&snapshot.viewer.user_id===scope.user_id&&scope.session_generation===generation;
  const viewer:PokerAuthority['viewer_kind']|null=admitted?(active?(active.table_id===p.table_id?'PLAYER_SELF':null):snapshot.viewer.owned_open_table_id===p.table_id?'HOST':'SPECTATOR'):null;
  const hardKey=viewer?JSON.stringify([scope.user_id,generation,p.table_id,viewer]):null;
  const [owned,setOwned]=useState<Owner|null>(null),visible=owned?.api===p.client&&owned.key===hardKey&&owned.alive?owned:null;
  const stream=useSyncExternalStore(visible?.client.subscribe??subscribeNone,visible?.client.getSnapshot??snapshotNone,snapshotNone);
  const [ui,setUi]=useState(emptyUi),[chatDraft,setChatDraft]=useState(''),[notice,setNotice]=useState(''),[exitStatus,setExitStatus]=useState(''),[querying,setQuerying]=useState(false),[hostOperation,setHostOperation]=useState<HostOperation>();
  const [,tick]=useState(0),exit=useRef<Exit|null>(null),flight=useRef<Flight|null>(null),hostRef=useRef<HostOperation|undefined>(undefined),readGeneration=useRef(0),timer=useRef<ReturnType<typeof setTimeout>|undefined>(undefined);
  const latest=useRef({owner:visible,blocked:p.mutation_blocked,session:active?.session_id,onReturn:p.onReturnLobby});
  useLayoutEffect(()=>{
    latest.current={owner:visible,blocked:p.mutation_blocked,session:active?.session_id,onReturn:p.onReturnLobby};
    const target=exit.current;if(target&&[active?.session_id,stream?.table?.viewer.session_id].some(id=>id&&id!==target.session)){target.invalidated=true;clearTimer();}
  });
  const current=(owner:Owner|null):owner is Owner=>!!owner&&owner.alive&&latest.current.owner===owner&&currentPokerScope(owner.api,owner.scope,()=>owner.scope);
  const exitCurrent=(target:Exit)=>exit.current===target&&!target.finished&&!target.invalidated&&current(target.owner)&&
    (!latest.current.session||latest.current.session===target.session)&&
    (!target.owner.client.getSnapshot()?.table?.viewer.session_id||target.owner.client.getSnapshot()?.table?.viewer.session_id===target.session);
  function clearTimer(){clearTimeout(timer.current);timer.current=undefined;}
  useEffect(()=>{
    if(!hardKey||!viewer)return;
    let captured:PokerReadScope;try{captured=capturePokerReadScope(p.client,p.table_id,viewer,0);}catch{return;}
    if(captured.user_id!==scope.user_id||captured.session_generation!==generation)return;
    const owner:Owner={api:p.client,key:hardKey,client:new PokerTableClient(p.client),scope:captured,alive:true};
    setOwned(owner);setUi(emptyUi());setChatDraft('');setNotice('');setExitStatus('');setQuerying(false);hostRef.current=undefined;setHostOperation(undefined);
    void owner.client.connect(p.table_id,viewer,viewer==='PLAYER_SELF'?'CLAIM_CONTROL':'READ_ONLY').catch(()=>{if(current(owner))setNotice(unavailable);});
    return()=>{owner.alive=false;if(latest.current.owner===owner)latest.current.owner=null;clearTimer();exit.current=null;flight.current=null;hostRef.current=undefined;owner.client.dispose();};
  },[p.client,hardKey]);
  useEffect(()=>{if(!visible)return;const timer=setInterval(()=>tick(value=>value+1),1000);return()=>clearInterval(timer);},[visible]);
  useEffect(()=>{setUi(emptyUi());},[hardKey,stream?.table?.hand?.hand_id,stream?.table?.hand?.action_sequence,stream?.table?.viewer.session_id,stream?.table?.viewer.control_epoch,stream?.control_generation,stream?.connection_id]);
  useEffect(()=>{const operation=hostRef.current,version=stream?.table?.table_version;if(operation?.phase==='ACKNOWLEDGED'&&operation.receipt_version&&version&&BigInt(version)>=BigInt(operation.receipt_version)){hostRef.current=undefined;setHostOperation(undefined);}},[stream?.table?.table_version]);
  function queryExit(target:Exit):Promise<void>{
    if(!exitCurrent(target))return Promise.resolve();
    if(flight.current?.exit===target)return flight.current.promise;
    clearTimer();const readScope=capturePokerReadScope(target.owner.api,target.owner.scope.table_id,'PLAYER_SELF',++readGeneration.current);
    const next:Flight={exit:target,scope:readScope,promise:Promise.resolve()};flight.current=next;setQuerying(true);setExitStatus('正在核对原会话出金。');
    const currentRead=()=>flight.current===next&&exitCurrent(target)?readScope:null;
    next.promise=Promise.resolve().then(async()=>{
      let again=false;
      try{
        if(!currentRead())return;
        const result=await readPokerHttpSession(target.owner.api,readScope,currentRead,target.session);
        if(!result||!currentRead())return;
        if(result.state==='SETTLED'){target.finished=true;setExitStatus('原会话出金已确认。');latest.current.onReturn();}
        else{setExitStatus(result.state==='NEEDS_REVIEW'?'原会话待复核；保留出金核对。':'原会话尚未完成出金。');again=true;}
      }catch{if(currentRead())setExitStatus('原会话出金暂未核实，请手动重新核对。');}
      finally{
        if(flight.current===next){flight.current=null;if(current(target.owner))setQuerying(false);
          // Runtime proposal: singleflight, 2 s after nonterminal success; errors stop automatic polling.
          if(again&&exitCurrent(target))timer.current=setTimeout(()=>{void queryExit(target);},2000);
        }
      }
    });return next.promise;
  }
  function run(owner:Owner|null,operation:()=>Promise<void>,mutation=false){
    if(!current(owner)||mutation&&latest.current.blocked)return;
    void operation().catch(()=>{if(current(owner))setNotice(unavailable);});
  }
  function publishHost(operation:HostOperation|undefined){hostRef.current=operation;setHostOperation(operation?{...operation}:undefined);}
  function syncHost(owner:Owner){const state=owner.client.getSnapshot();if(current(owner)&&state&&['LIVE','SYNCING'].includes(state.connection_state))try{owner.client.sync();}catch{setNotice(unavailable);}}
  function sendHost(owner:Owner,operation:HostOperation){
    const currentHost=()=>current(owner)&&hostRef.current===operation&&currentPokerScope(owner.api,operation.scope,()=>owner.scope)?owner.scope:null;
    void submitPokerHostCommand(owner.api,operation.scope,currentHost,operation.original).then(receipt=>{
      if(!receipt||!currentHost())return;operation.phase='ACKNOWLEDGED';operation.querying=false;operation.can_retry=false;operation.status=receipt.status;operation.receipt_version=receipt.table_version;publishHost(operation);syncHost(owner);
    }).catch(()=>{if(current(owner)&&hostRef.current===operation){operation.phase='UNKNOWN';operation.querying=false;operation.can_retry=false;publishHost(operation);setNotice('房主管理结果尚未确认，请核对原回执。');}});
  }
  function hostCommand(command:PokerHostCommand,rendered:PokerIntentContext){
    if(!current(visible)||latest.current.blocked||hostRef.current)return;const state=visible.client.getSnapshot();
    if(!state?.table||state.connection_state!=='LIVE'||!matchesPokerIntentContext(rendered,pokerStreamContext(state))||!hostCommandCurrent(state.table,command))return;
    const operation:HostOperation={scope:{...visible.scope},original:{...structuredClone(command),request_id:crypto.randomUUID()},command:structuredClone(command),phase:'SENT',querying:false,can_retry:false};publishHost(operation);setNotice('');sendHost(visible,operation);
  }
  function queryHost(){
    const owner=visible,operation=hostRef.current;if(!current(owner)||!operation||operation.phase==='ACKNOWLEDGED'||operation.querying||!currentPokerScope(owner.api,operation.scope,()=>owner.scope))return;
    operation.querying=true;operation.can_retry=false;publishHost(operation);
    const currentHost=()=>current(owner)&&hostRef.current===operation?owner.scope:null;
    void readPokerHostReceipt(owner.api,operation.scope,currentHost,operation.original).then(result=>{
      if(!result||!currentHost())return;
      if(result.state==='FOUND'){operation.phase='ACKNOWLEDGED';operation.status=result.receipt.status;operation.receipt_version=result.receipt.table_version;operation.can_retry=false;syncHost(owner);}
      else{operation.phase='UNKNOWN';operation.can_retry=hostCommandCurrent(owner.client.getSnapshot()?.table,operation.original);}
    }).catch(()=>{if(currentHost()){operation.can_retry=false;setNotice('原房主管理回执暂未核实。');}}).finally(()=>{if(currentHost()){operation.querying=false;publishHost(operation);}});
  }
  function retryHost(rendered:PokerIntentContext){
    const owner=visible,operation=hostRef.current,state=owner?.client.getSnapshot();if(!current(owner)||latest.current.blocked||!operation||operation.phase!=='UNKNOWN'||operation.querying||!operation.can_retry||!state?.table||state.connection_state!=='LIVE'||!matchesPokerIntentContext(rendered,pokerStreamContext(state))||!currentPokerScope(owner.api,operation.scope,()=>owner.scope)||!hostCommandCurrent(state.table,operation.original))return;
    operation.phase='SENT';operation.can_retry=false;publishHost(operation);setNotice('');sendHost(owner,operation);
  }
  function sendChat(message:string,rendered:PokerIntentContext){
    if(!current(visible)||latest.current.blocked)return;try{visible.client.sendChat(message,rendered);setChatDraft('');setNotice('');}catch{setNotice('聊天消息未发送；请等待权威状态后重试。');}
  }
  function intent(value:TableIntent,rendered:PokerIntentContext){
    if(!current(visible))return;const state=visible.client.getSnapshot();if(!state)return;
    if(value.type==='reconnect'){
      if(!matchesPokerIntentContext(rendered,pokerStreamContext(state)))return;
      run(visible,()=>visible.client.connect(visible.scope.table_id,visible.scope.viewer_kind,value.control_intent??state.ticket_intent));return;
    }
    if(value.type==='return_lobby'){
      if((visible.scope.viewer_kind==='SPECTATOR'||visible.scope.viewer_kind==='HOST')&&!state.table?.viewer.session_id&&!state.pending&&!exit.current&&matchesPokerIntentContext(rendered,pokerStreamContext(state)))latest.current.onReturn();return;
    }
    if(latest.current.blocked)return;
    let target:Exit|undefined;
    if(value.type==='leave'){
      const session=state.table?.viewer.session_id;
      if(!session||session!==latest.current.session||state.pending||exit.current&&!exit.current.invalidated||!matchesPokerIntentContext(rendered,pokerStreamContext(state)))return;
      target={owner:visible,session,finished:false};exit.current=target;
    }
    const sending=visible.client.submit(value,rendered);
    if(target){
      const pending=visible.client.getSnapshot()?.pending;
      if(pending?.kind==='leave'&&pending.target_session_id===target.session){setUi(next=>({...next,confirm_leave:false}));void queryExit(target);}
      else if(exit.current===target)exit.current=null;
    }
    void sending.catch(()=>{if(current(visible))setNotice(unavailable);});
  }
  if(!visible||!stream)return <section className="pk-page" role="status">等待完整牌桌状态。{notice}</section>;
  const now=visible.client.serverNow(),target=exit.current;
  return <>
    {target&&<section className="pk-runtime-banner" role="status"><p>{exitStatus||'原会话出金状态待核对。'}</p><button disabled={querying||!exitCurrent(target)} aria-busy={querying} onClick={()=>{void queryExit(target);}}>核对原会话出金</button></section>}
    {stream.table?<PokerTable table={stream.table} authority={pokerStreamAuthority(stream)} ui={ui} mutation_blocked={p.mutation_blocked}
      display_now={Number.isFinite(now)?new Date(now).toISOString():undefined} notice={notice||undefined}
      onUiChange={next=>{if(current(visible))setUi(next);}} onIntent={intent}
      onQueryReceipt={()=>run(visible,()=>visible.client.recheckPending())} receipt_querying={stream.receipt_querying}
      onRetryPending={context=>run(visible,()=>visible.client.retryPending(context),true)}
      onQueryTakeover={()=>run(visible,()=>visible.client.recheckTakeover())} onQueryPreviousTakeovers={()=>run(visible,()=>visible.client.recheckTakeover(true))}
      onRetryTakeover={context=>run(visible,()=>visible.client.retryTakeover(context),true)}
      chat_draft={chatDraft} onChatDraftChange={next=>{if(current(visible))setChatDraft(next);}} onSendChat={sendChat}
      onQueryChatReceipt={()=>run(visible,()=>visible.client.recheckChat())} onRetryChat={context=>run(visible,()=>Promise.resolve(visible.client.retryChat(context)),true)}
      host_operation={hostOperation} onHostCommand={hostCommand} onQueryHostReceipt={queryHost} onRetryHost={retryHost}/>:
      <section className="pk-page" role="status">等待完整牌桌状态。{notice||(stream.last_error?unavailable:'')}
        {pokerStreamAuthority(stream).can_reconnect&&<button onClick={()=>intent({type:'reconnect'},pokerStreamContext(stream))}>重新连接</button>}
      </section>}
  </>;
}
