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
interface HostOperation extends PokerHostOperationView {scope:PokerReadScope;original:PokerHostHttpCommand;receipt_version?:string;send?:object}
interface RecoveryLoop {owner:Owner|null;epoch:number;signature:string;attempt:number;running:boolean;wake:boolean;timer?:ReturnType<typeof setTimeout>}
const emptyUi=():TableUi=>({bet:{action_type:'RAISE',amount_chips:''},top_up_chips:'',confirm_leave:false,confirm_takeover:false});
const subscribeNone=()=>()=>{},snapshotNone=()=>null;
const unavailable='当前操作尚未确认；系统将自动核对原回执，连接恢复前请勿重复提交。';
const recoveryDelays=[2000,4000,8000,16000,30000] as const;
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
  const [,tick]=useState(0),exit=useRef<Exit|null>(null),flight=useRef<Flight|null>(null),hostRef=useRef<HostOperation|undefined>(undefined),readGeneration=useRef(0),recovery=useRef<RecoveryLoop>({owner:null,epoch:0,signature:'',attempt:0,running:false,wake:false});
  const latest=useRef({owner:visible,blocked:p.mutation_blocked,session:active?.session_id,onReturn:p.onReturnLobby});
  useLayoutEffect(()=>{
    latest.current={owner:visible,blocked:p.mutation_blocked,session:active?.session_id,onReturn:p.onReturnLobby};
    const target=exit.current;if(target&&[active?.session_id,stream?.table?.viewer.session_id].some(id=>id&&id!==target.session))target.invalidated=true;
  });
  const current=(owner:Owner|null):owner is Owner=>!!owner&&owner.alive&&latest.current.owner===owner&&currentPokerScope(owner.api,owner.scope,()=>owner.scope);
  const exitCurrent=(target:Exit)=>exit.current===target&&!target.finished&&!target.invalidated&&current(target.owner)&&
    (!latest.current.session||latest.current.session===target.session)&&
    (!target.owner.client.getSnapshot()?.table?.viewer.session_id||target.owner.client.getSnapshot()?.table?.viewer.session_id===target.session);
  function clearRecoveryTimer(){const loop=recovery.current;clearTimeout(loop.timer);loop.timer=undefined;}
  function cancelRecovery(owner?:Owner){const loop=recovery.current;if(owner&&loop.owner!==owner)return;clearRecoveryTimer();loop.owner=null;loop.signature='';loop.attempt=0;loop.running=false;loop.wake=false;loop.epoch++;}
  useEffect(()=>{
    if(!hardKey||!viewer)return;
    let captured:PokerReadScope;try{captured=capturePokerReadScope(p.client,p.table_id,viewer,0);}catch{return;}
    if(captured.user_id!==scope.user_id||captured.session_generation!==generation)return;
    const owner:Owner={api:p.client,key:hardKey,client:new PokerTableClient(p.client),scope:captured,alive:true};
    setOwned(owner);setUi(emptyUi());setChatDraft('');setNotice('');setExitStatus('');setQuerying(false);hostRef.current=undefined;setHostOperation(undefined);
    void owner.client.connect(p.table_id,viewer,viewer==='PLAYER_SELF'?'CLAIM_CONTROL':'READ_ONLY').catch(()=>{if(current(owner))setNotice(unavailable);});
    return()=>{owner.alive=false;if(latest.current.owner===owner)latest.current.owner=null;cancelRecovery(owner);exit.current=null;flight.current=null;hostRef.current=undefined;owner.client.dispose();};
  },[p.client,hardKey]);
  useEffect(()=>{if(!visible)return;const timer=setInterval(()=>tick(value=>value+1),1000);return()=>clearInterval(timer);},[visible]);
  useEffect(()=>{setUi(emptyUi());},[hardKey,stream?.table?.hand?.hand_id,stream?.table?.hand?.action_sequence,stream?.table?.viewer.session_id,stream?.table?.viewer.control_epoch,stream?.control_generation,stream?.connection_id]);
  useEffect(()=>{const operation=hostRef.current,version=stream?.table?.table_version;if(operation?.phase==='ACKNOWLEDGED'&&operation.receipt_version&&version&&BigInt(version)>=BigInt(operation.receipt_version)){hostRef.current=undefined;setHostOperation(undefined);}},[stream?.table?.table_version,hostOperation]);
  useEffect(()=>{if(visible)observeRecovery(visible);},[visible,stream,hostOperation,exitStatus]);
  useEffect(()=>{if(!visible)return;const wake=()=>wakeRecovery(visible);window.addEventListener('online',wake);window.addEventListener('focus',wake);document.addEventListener('visibilitychange',wake);return()=>{window.removeEventListener('online',wake);window.removeEventListener('focus',wake);document.removeEventListener('visibilitychange',wake);};},[visible]);
  function queryExit(target:Exit):Promise<void>{
    if(!exitCurrent(target))return Promise.resolve();
    if(flight.current?.exit===target)return flight.current.promise;
    const readScope=capturePokerReadScope(target.owner.api,target.owner.scope.table_id,'PLAYER_SELF',++readGeneration.current);
    const next:Flight={exit:target,scope:readScope,promise:Promise.resolve()};flight.current=next;setQuerying(true);setExitStatus('正在核对原会话出金。');
    const currentRead=()=>flight.current===next&&exitCurrent(target)?readScope:null;
    next.promise=Promise.resolve().then(async()=>{
      try{
        if(!currentRead())return;
        const result=await readPokerHttpSession(target.owner.api,readScope,currentRead,target.session);
        if(!result||!currentRead())return;
        if(result.state==='SETTLED'){target.finished=true;setExitStatus('原会话出金已确认。');latest.current.onReturn();}
        else setExitStatus(result.state==='NEEDS_REVIEW'?'原会话待复核；系统将继续自动核对出金。':'原会话尚未完成出金；系统将继续自动核对。');
      }catch{if(currentRead())setExitStatus('原会话出金暂未核实；系统将在连接可用后继续自动核对。');}
      finally{
        if(flight.current===next){flight.current=null;if(current(target.owner))setQuerying(false);}
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
    const attempt={};operation.send=attempt;publishHost(operation);
    const currentHost=()=>current(owner)&&hostRef.current===operation&&operation.send===attempt&&currentPokerScope(owner.api,operation.scope,()=>owner.scope)?owner.scope:null;
    void submitPokerHostCommand(owner.api,operation.scope,currentHost,operation.original).then(receipt=>{
      if(!receipt||!currentHost())return;operation.phase='ACKNOWLEDGED';operation.querying=false;operation.can_retry=false;operation.status=receipt.status;operation.receipt_version=receipt.table_version;publishHost(operation);syncHost(owner);
    }).catch(()=>{if(currentHost()){operation.phase='UNKNOWN';operation.querying=false;operation.can_retry=false;publishHost(operation);setNotice('房主管理结果尚未确认；系统将自动核对原回执。');}})
      .finally(()=>{if(current(owner)&&hostRef.current===operation&&operation.send===attempt){operation.send=undefined;publishHost(operation);}});
  }
  function hostCommand(command:PokerHostCommand,rendered:PokerIntentContext){
    if(!current(visible)||latest.current.blocked||hostRef.current)return;const state=visible.client.getSnapshot();
    if(!state?.table||state.connection_state!=='LIVE'||!matchesPokerIntentContext(rendered,pokerStreamContext(state))||!hostCommandCurrent(state.table,command))return;
    const operation:HostOperation={scope:{...visible.scope},original:{...structuredClone(command),request_id:crypto.randomUUID()},command:structuredClone(command),phase:'SENT',querying:false,can_retry:false};publishHost(operation);setNotice('');sendHost(visible,operation);
  }
  function queryHost(owner:Owner|null=visible,operation:HostOperation|undefined=hostRef.current):Promise<void>{
    if(!current(owner)||!operation||operation.phase!=='UNKNOWN'||operation.send||operation.querying||hostRef.current!==operation||!currentPokerScope(owner.api,operation.scope,()=>owner.scope))return Promise.resolve();
    operation.querying=true;operation.can_retry=false;publishHost(operation);
    const currentHost=()=>current(owner)&&hostRef.current===operation?owner.scope:null;
    return readPokerHostReceipt(owner.api,operation.scope,currentHost,operation.original).then(result=>{
      if(!result||!currentHost())return;
      if(result.state==='FOUND'){operation.phase='ACKNOWLEDGED';operation.status=result.receipt.status;operation.receipt_version=result.receipt.table_version;operation.can_retry=false;syncHost(owner);}
      else{operation.phase='UNKNOWN';operation.can_retry=hostCommandCurrent(owner.client.getSnapshot()?.table,operation.original);}
    }).catch(()=>{if(currentHost()){operation.can_retry=false;setNotice('原房主管理回执暂未核实；系统将在连接可用后继续自动核对。');}}).finally(()=>{if(currentHost()){operation.querying=false;publishHost(operation);}});
  }
  function recoverySignature(owner:Owner):string {
    if(!current(owner))return '';
    const state=owner.client.getSnapshot(),parts:string[]=[];
    if(state?.pending)parts.push(state.pending.receipt?`projection:pending:${state.pending.receipt.table_version}`:`pending:${state.pending.kind}:${state.pending.request_id}:${state.pending.action_id??''}:${state.pending.target_session_id}`);
    if(state?.control_recovery)parts.push(state.control_recovery.phase==='ACKNOWLEDGED'?`projection:takeover:${state.connection_id??''}:${state.control_recovery.target_current}`:`takeover:${state.connection_id??''}:${state.control_recovery.target_current}`);
    if(state?.control_history?.count)parts.push(`takeover-history:${state.control_history.count}`);
    if(state?.chat_pending)parts.push(state.chat_pending.receipt?`projection:chat:${state.chat_pending.receipt.table_version}:${state.chat_pending.receipt.chat_sequence??''}`:`chat:${state.chat_pending.request_id}`);
    const operation=hostRef.current;if(operation&&currentPokerScope(owner.api,operation.scope,()=>owner.scope)){
      if(operation.phase==='UNKNOWN'&&!operation.send)parts.push(`host:${operation.original.request_id}`);
      else if(operation.phase==='ACKNOWLEDGED'&&operation.receipt_version&&(!state?.table||BigInt(state.table.table_version)<BigInt(operation.receipt_version)))parts.push(`projection:host:${operation.original.request_id}:${operation.receipt_version}`);
    }
    const target=exit.current;if(target&&exitCurrent(target))parts.push(`exit:${target.session}`);
    return parts.length?JSON.stringify(parts):'';
  }
  const recoveryAvailable=()=>typeof document==='undefined'||(document.visibilityState==='visible'&&(typeof navigator==='undefined'||navigator.onLine!==false));
  function bindRecovery(owner:Owner):{loop:RecoveryLoop;changed:boolean}{
    const loop=recovery.current;
    if(loop.owner!==owner){clearRecoveryTimer();loop.owner=owner;loop.epoch++;loop.signature='';loop.attempt=0;loop.running=false;loop.wake=false;}
    const signature=recoverySignature(owner),changed=signature!==loop.signature;
    if(changed){clearRecoveryTimer();loop.signature=signature;loop.attempt=0;if(loop.running)loop.wake=true;}
    return{loop,changed};
  }
  function scheduleRecovery(owner:Owner,delay:number){
    const {loop}=bindRecovery(owner);if(!loop.signature||loop.running||loop.timer!==undefined||!recoveryAvailable())return;
    const epoch=loop.epoch;loop.timer=setTimeout(()=>{if(loop.owner!==owner||loop.epoch!==epoch)return;loop.timer=undefined;void runRecovery(owner,epoch);},delay);
  }
  function observeRecovery(owner:Owner){
    const {loop,changed}=bindRecovery(owner);if(!loop.signature){clearRecoveryTimer();loop.attempt=0;loop.wake=false;return;}
    const alreadySyncing=owner.client.getSnapshot()?.connection_state==='SYNCING';
    if(!loop.running&&!loop.timer)scheduleRecovery(owner,changed&&!alreadySyncing?0:recoveryDelays[Math.min(loop.attempt,recoveryDelays.length-1)]);
  }
  function wakeRecovery(owner:Owner){
    const {loop}=bindRecovery(owner);if(!loop.signature||loop.running||!recoveryAvailable())return;
    clearRecoveryTimer();loop.attempt=0;void runRecovery(owner,loop.epoch);
  }
  async function runRecovery(owner:Owner,epoch:number){
    const loop=recovery.current;if(loop.owner!==owner||loop.epoch!==epoch||loop.running||!loop.signature||!current(owner)||!recoveryAvailable())return;
    loop.running=true;loop.wake=false;
    try{
      const jobs:Promise<void>[]=[],target=exit.current,state=owner.client.getSnapshot(),operation=hostRef.current;
      if(target&&exitCurrent(target))jobs.push(queryExit(target));
      if(current(owner)&&state?.pending&&!state.pending.receipt)jobs.push(owner.client.recheckPending());
      if(current(owner)&&state?.control_recovery&&state.control_recovery.phase!=='ACKNOWLEDGED')jobs.push(owner.client.recheckTakeover());
      if(current(owner)&&state?.control_history?.count)jobs.push(owner.client.recheckTakeover(true));
      if(current(owner)&&state?.chat_pending&&!state.chat_pending.receipt)jobs.push(owner.client.recheckChat());
      if(current(owner)&&operation?.phase==='UNKNOWN'&&!operation.send)jobs.push(queryHost(owner,operation));
      const projectionPending=!!(state?.pending?.receipt||state?.control_recovery?.phase==='ACKNOWLEDGED'||state?.chat_pending?.receipt||operation?.phase==='ACKNOWLEDGED'&&operation.receipt_version&&(!state?.table||BigInt(state.table.table_version)<BigInt(operation.receipt_version)));
      if(projectionPending&&current(owner)&&state&&['LIVE','SYNCING'].includes(state.connection_state))try{owner.client.sync();}catch{setNotice(unavailable);}
      void Promise.allSettled(jobs);
    }finally{
      if(loop.owner!==owner||loop.epoch!==epoch)return;
      loop.running=false;const wake=loop.wake;loop.wake=false;const signature=recoverySignature(owner);
      if(!signature){clearRecoveryTimer();loop.signature='';loop.attempt=0;return;}
      if(signature!==loop.signature){loop.signature=signature;loop.attempt=0;}
      if(wake){scheduleRecovery(owner,0);return;}
      const delay=recoveryDelays[Math.min(loop.attempt,recoveryDelays.length-1)];loop.attempt=Math.min(loop.attempt+1,recoveryDelays.length-1);scheduleRecovery(owner,delay);
    }
  }
  function retryHost(rendered:PokerIntentContext){
    const owner=visible,operation=hostRef.current,state=owner?.client.getSnapshot();if(!current(owner)||latest.current.blocked||!operation||operation.phase!=='UNKNOWN'||operation.send||operation.querying||!operation.can_retry||!state?.table||state.connection_state!=='LIVE'||!matchesPokerIntentContext(rendered,pokerStreamContext(state))||!currentPokerScope(owner.api,operation.scope,()=>owner.scope)||!hostCommandCurrent(state.table,operation.original))return;
    operation.phase='SENT';operation.can_retry=false;publishHost(operation);setNotice('');sendHost(owner,operation);
  }
  function sendChat(message:string,rendered:PokerIntentContext){
    if(!current(visible)||latest.current.blocked)return;try{visible.client.sendChat(message,rendered);setChatDraft('');setNotice('');}catch{setNotice('聊天消息结果尚未确认；系统将自动核对原回执。');}
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
      if(pending?.kind==='leave'&&pending.target_session_id===target.session){setUi(next=>({...next,confirm_leave:false}));if(recovery.current.running)void queryExit(target);else wakeRecovery(visible);}
      else if(exit.current===target)exit.current=null;
    }
    void sending.catch(()=>{if(current(visible))setNotice(unavailable);});
  }
  if(!visible||!stream)return <section className="pk-page" role="status">等待完整牌桌状态。{notice}</section>;
  const now=visible.client.serverNow(),target=exit.current;
  const authority=pokerStreamAuthority(stream),autoQuerying=querying||stream.receipt_querying===true||stream.control_recovery?.querying===true||stream.control_history?.querying===true||stream.chat_pending?.querying===true||hostOperation?.querying===true;
  const explicitRetry=stream.can_retry_pending||stream.control_recovery?.can_retry||stream.chat_pending?.can_retry||hostOperation?.can_retry;
  const unresolved=!!target||!!stream.pending||!!stream.control_recovery||!!stream.control_history||!!stream.chat_pending||!!hostOperation;
  const recoveryStatus=target?(exitStatus||'正在自动核对原会话出金。'):
    autoQuerying?'正在自动核对原操作结果。':
    explicitRetry?'原回执暂未足以确认结果；操作保持锁定，系统将继续自动核对。':
    stream.control_history?`还有 ${stream.control_history.count} 次历史接管结果待确认；系统将继续自动核对，不重发旧目标。`:
    unresolved?'原操作结果仍待确认；系统将继续自动核对。':undefined;
  return <>
    {stream.table?<PokerTable table={stream.table} authority={authority} ui={ui} mutation_blocked={p.mutation_blocked}
      display_now={Number.isFinite(now)?new Date(now).toISOString():undefined} notice={notice||undefined} recovery_status={recoveryStatus}
      onUiChange={next=>{if(current(visible))setUi(next);}} onIntent={intent}
      onQueryReceipt={()=>run(visible,()=>visible.client.recheckPending())} receipt_querying={stream.receipt_querying}
      onRetryPending={context=>run(visible,()=>visible.client.retryPending(context),true)}
      onQueryTakeover={()=>run(visible,()=>visible.client.recheckTakeover())} onQueryPreviousTakeovers={()=>run(visible,()=>visible.client.recheckTakeover(true))}
      onRetryTakeover={context=>run(visible,()=>visible.client.retryTakeover(context),true)}
      chat_draft={chatDraft} onChatDraftChange={next=>{if(current(visible))setChatDraft(next);}} onSendChat={sendChat}
      onQueryChatReceipt={()=>run(visible,()=>visible.client.recheckChat())} onRetryChat={context=>run(visible,()=>Promise.resolve(visible.client.retryChat(context)),true)}
      host_operation={hostOperation} onHostCommand={hostCommand} onQueryHostReceipt={()=>{void queryHost();}} onRetryHost={retryHost}/>:
      <section className="pk-page" role="status">等待完整牌桌状态。{notice||(stream.last_error?unavailable:'')}
        {pokerStreamAuthority(stream).can_reconnect&&<button onClick={()=>intent({type:'reconnect'},pokerStreamContext(stream))}>重新连接</button>}
      </section>}
  </>;
}
