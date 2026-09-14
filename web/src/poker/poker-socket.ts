import { ApiError, type ApiClient } from '../api';
import { capturePokerReadScope, currentPokerScope, mintPokerConnectTicket, readPokerHttpReceipt, submitPokerHttpCommand, type PokerHttpCommand } from './poker-http';
import { acknowledgePokerPending, beginPokerPending, disconnectPokerStream, openPokerStream, pokerStreamAuthority, pokerStreamContext, receivePokerStream, type PokerStream } from './poker-stream';
import { matchesPokerIntentContext, type PokerReadScope } from './poker-client-boundary';
import type { PokerAuthority, PokerIntentContext, PokerTicketIntent, TableIntent } from './poker-ui-types';
import { pokerChipAmount } from './poker-view';
import { encodePokerClient, parsePokerReceipt, samePokerPendingLocator, POKER_MAX_FRAME_BYTES, POKER_SUBPROTOCOL, POKER_WS_PATH, type PokerClientCommand, type PokerPendingLocator, type PokerReceipt } from './poker-wire';

interface Options {
  socketFactory?:(url:string,protocol:string)=>WebSocket;
  /** Approved local runtime defaults, not frozen product rules; ping never renews a controller lease. */
  heartbeatIntervalMs?:number;pongTimeoutMs?:number;
}
const invalid=()=>new ApiError('牌桌连接上下文已失效，请显式重新连接。',0,'POKER_CONNECTION_UNAVAILABLE');
const requestID=()=>crypto.randomUUID();
const businessID=()=>`ik1_${btoa(String.fromCharCode(...crypto.getRandomValues(new Uint8Array(32)))).replace(/\+/g,'-').replace(/\//g,'_').replace(/=+$/,'')}`;
/** Ephemeral primitive-only semantic record; never a saved wire frame or public projection. */
interface OriginalIntent {scope:Readonly<PokerReadScope>;locator:Readonly<PokerPendingLocator>;intent:Readonly<TableIntent>;action_sequence?:string}
interface AuxiliaryControl {
  readonly scope:Readonly<PokerReadScope>;readonly locator:Readonly<PokerPendingLocator>;readonly connection_id:string;
  phase:'SENT'|'UNKNOWN'|'ACKNOWLEDGED';receipt?:PokerReceipt;send?:object;not_found?:PokerReadScope;
  query?:{scope:PokerReadScope;promise:Promise<void>};
}
interface ChatOriginal {scope:PokerReadScope;request_id:string;message:string}
// IS §554 / FRZ-668: bounded delay, not bounded attempts. A reconnect is a fresh auth, never a command retry.
const RECONNECT_INITIAL_MS=500,RECONNECT_MAX_MS=5000,RECONNECT_JITTER=.20;
/** One table stream. Uses native ApiClient and a real same-origin WebSocket. No App routing, storage or command replay. */
export class PokerTableClient {
  #state:PokerStream|null=null;#socket?:WebSocket;#ticket?:string;#generation=0;#disposed=false;
  #detach?:()=>void;#unsubscribe:()=>void;#listeners=new Set<()=>void>();
  #heartbeat?:ReturnType<typeof setTimeout>;#pongTimer?:ReturnType<typeof setTimeout>;#pingRequest?:string;#pongDeadline=0;
  #reconnectTimer?:ReturnType<typeof setTimeout>;#reconnectBase=RECONNECT_INITIAL_MS;#autoReconnect=false;
  #factory:(url:string,protocol:string)=>WebSocket;#interval:number;#pongTimeout:number;
  #snapshotReceivedAt=0;#api:ApiClient;
  #queryFlight?:{scope:PokerReadScope;locator:PokerPendingLocator;promise:Promise<void>};
  #original?:Readonly<OriginalIntent>;#attempt?:object;#retryQuery?:{scope:PokerReadScope;locator:PokerPendingLocator};
  #aux?:AuxiliaryControl;
  #previousControls=new Set<AuxiliaryControl>();
  #historyQuery?:{scope:PokerReadScope;promise:Promise<void>};
  #chatOriginal?:ChatOriginal;
  constructor(api:ApiClient,options:Options={}){
    this.#api=api;
    this.#factory=options.socketFactory??((url,protocol)=>new WebSocket(url,protocol));
    this.#interval=options.heartbeatIntervalMs??30000;this.#pongTimeout=options.pongTimeoutMs??10000;
    if([this.#interval,this.#pongTimeout].some(n=>!Number.isSafeInteger(n)||n<=0||n>300000))throw invalid();
    this.#unsubscribe=api.subscribe(()=>{const s=this.#state;if(s&&!currentPokerScope(api,s.scope,()=>s.scope)){this.#stopReconnect();this.#release();this.#publish(null);}});
    document.addEventListener('visibilitychange',this.#visible);
  }
  getSnapshot=():PokerStream|null=>this.#state;
  recheckTakeover(previous=false):Promise<void>{
    if(!previous)return this.#queryControl(this.#aux);
    const s=this.#state;if(!s||!this.#current(s.scope)||!this.#previousControls.size)return Promise.resolve();
    if(this.#historyQuery&&this.#current(this.#historyQuery.scope))return this.#historyQuery.promise;
    const scope={...s.scope},records=[...this.#previousControls],flight={scope,promise:Promise.resolve()};
    this.#historyQuery=flight;
    flight.promise=Promise.resolve().then(async()=>{
      try{
        for(const a of records){
          if(!this.#current(scope))break;
          await this.#queryControl(a);
        }
      }finally{
        if(this.#historyQuery===flight){
          this.#historyQuery=undefined;
          this.#publish(this.#state);
        }
      }
    });
    this.#publish(s);return flight.promise;
  }
  #queryControl(a?:AuxiliaryControl):Promise<void>{
    const s=this.#state;
    if(!a||a.receipt||!s||!this.#current(s.scope))return Promise.resolve();
    if(a.query&&this.#current(a.query.scope))return a.query.promise;
    const scope={...s.scope},retained=()=>this.#aux===a||this.#previousControls.has(a);
    const current=()=>this.#current(scope)&&retained()&&!a.receipt?this.#state!.scope:null;
    const query={scope,promise:Promise.resolve()};a.query=query;a.not_found=undefined;
    query.promise=Promise.resolve().then(async()=>{
      try{
        if(!current())return;
        const result=await readPokerHttpReceipt(this.#api,scope,current,a.locator);
        if(!result||!current())return;
        if(result.state==='FOUND')this.#acceptControlReceipt(a,result.receipt);else if(this.#aux===a)a.not_found=scope;
      }catch{if(current())this.#publish({...this.#state!,last_error:'POKER_CONTROL_QUERY_UNAVAILABLE'});}
      finally{if(retained()&&a.query===query){a.query=undefined;if(this.#current(scope))this.#publish(this.#state);}}
    });
    this.#publish(s);return query.promise;
  }
  async retryTakeover(rendered:PokerIntentContext):Promise<void>{
    const a=this.#aux;
    if(!a||a.phase!=='UNKNOWN'||a.send||a.query||!a.not_found||!this.#current(a.not_found))throw invalid();
    return this.#sendControl(rendered,a);
  }
  #recoveryBoundary(s:PokerStream):boolean {
    if(s.connection_state!=='LIVE'||!s.table?.viewer.session_id||s.scope.viewer_kind!=='PLAYER_SELF'||!this.#current(s.scope,this.#socket))return false;
    if(!s.table.seats.some(seat=>seat.is_self&&seat.seat_no===s.table!.viewer.seat_no))return false;
    if(!s.pending)return true;
    const p=s.pending,q=this.#retryQuery;
    return !!(this.#original&&this.#ownsOriginal(s,this.#original)&&p.phase==='UNKNOWN'&&!p.receipt&&!this.#queryFlight&&q&&this.#current(q.scope)&&samePokerPendingLocator(p,q.locator)&&p.target_session_id===s.table.viewer.session_id);
  }
  #controlState(rendered:PokerIntentContext,own?:AuxiliaryControl,passive=false):PokerStream {
    const s=this.#state;
    if(!s||this.#aux!==own||own?.query||!this.#recoveryBoundary(s))throw invalid();
    if(own&&(own.connection_id!==s.connection_id||own.locator.target_session_id!==s.table!.viewer.session_id))throw invalid();
    return this.#authorize({type:'takeover'},rendered,s.pending?.request_id,passive);
  }
  #acceptControlReceipt(a:AuxiliaryControl,receipt:PokerReceipt){
    if(receipt.session_id!==a.locator.target_session_id)throw invalid();
    if(this.#previousControls.delete(a)){
      a.query=undefined;this.#publish(this.#state);return;
    }
    a.receipt=receipt;a.phase='ACKNOWLEDGED';this.#publish(this.#state);
    if(this.#aux===a&&this.#state&&['LIVE','SYNCING'].includes(this.#state.connection_state)){
      try{this.sync();}catch{/* Receipt is known; transport recovery owns the failed readonly sync. */}
    }
  }
  async #sendControl(rendered:PokerIntentContext,original?:AuxiliaryControl):Promise<void>{
    const s=this.#controlState(rendered,original),scope={...s.scope};
    const a:AuxiliaryControl=original??{
      scope:Object.freeze({...scope}),connection_id:s.connection_id!,phase:'SENT',
      locator:Object.freeze({kind:'takeover',request_id:businessID(),action_id:null,target_session_id:s.table!.viewer.session_id!}),
    };
    const attempt={};this.#aux=a;a.phase='SENT';a.send=attempt;this.#publish(s);
    const current=()=>this.#current(scope)&&this.#aux===a&&a.send===attempt&&!a.receipt?this.#state!.scope:null;
    try{this.#controlState(rendered,a);if(!current())throw invalid();}
    catch{
      if(this.#aux===a){a.send=undefined;if(original)a.phase='UNKNOWN';else this.#aux=undefined;this.#publish(this.#state);}
      throw invalid();
    }
    a.not_found=undefined;
    try{
      const result=await submitPokerHttpCommand(this.#api,{type:'takeover',request_id:a.locator.request_id,session_id:a.locator.target_session_id,connection_id:a.connection_id},scope,current);
      if(result&&current())this.#acceptControlReceipt(a,result);
    }catch{
      if(current()){a.phase='UNKNOWN';this.#publish({...this.#state!,last_error:'POKER_CONTROL_OUTCOME_UNCONFIRMED'});}
      throw new ApiError('本次接管结果待确认，请核对接管回执。',0,'POKER_CONTROL_OUTCOME_UNCONFIRMED',true);
    }finally{if(this.#aux===a&&a.send===attempt){a.send=undefined;this.#publish(this.#state);}}
  }
  /** User-only entry: never takes a replacement intent, amount or identity. */
  async retryPending(rendered:PokerIntentContext):Promise<void>{
    const state=this.#retryState(rendered),original=this.#original!;
    return this.#transmit(original.intent,rendered,state,original.locator.request_id,original.locator.action_id,true);
  }
  /** Explicit read only; may run without a WS controller or even an open socket. */
  recheckPending():Promise<void>{
    const state=this.#state,pending=state?.pending;if(!state||!pending||pending.receipt||!this.#current(state.scope))return Promise.resolve();
    if(this.#queryFlight&&this.#current(this.#queryFlight.scope)&&samePokerPendingLocator(pending,this.#queryFlight.locator))return this.#queryFlight.promise;
    const scope={...state.scope},locator:PokerPendingLocator={kind:pending.kind,request_id:pending.request_id,action_id:pending.action_id,target_session_id:pending.target_session_id,...(pending.target_hand_id?{target_hand_id:pending.target_hand_id}:{})};
    const current=()=>this.#current(scope)&&samePokerPendingLocator(this.#state?.pending,locator)&&!this.#state?.pending?.receipt?this.#state!.scope:null;
    const flight={scope,locator,promise:Promise.resolve()};this.#queryFlight=flight;this.#retryQuery=undefined;
    flight.promise=Promise.resolve().then(async()=>{
      try{
        if(!current())return;const result=await readPokerHttpReceipt(this.#api,scope,current,locator);if(!result||!current())return;
        if(result.state==='FOUND'){
          this.#publish(acknowledgePokerPending(this.#state!,locator.request_id,locator.action_id,result.receipt));
          if(this.#current(scope)&&this.#state?.pending?.receipt&&['LIVE','SYNCING'].includes(this.#state.connection_state))this.sync();
        }else this.#retryQuery={scope,locator};
      }catch{if(current())this.#publish({...this.#state!,last_error:'POKER_RECEIPT_QUERY_UNAVAILABLE'});}
      finally{if(this.#queryFlight===flight){this.#queryFlight=undefined;if(this.#current(scope))this.#publish({...this.#state!,receipt_querying:false});}}
    });
    this.#publish({...state,receipt_querying:true});return flight.promise;
  }
  #ownsOriginal(state:PokerStream,original:OriginalIntent){return samePokerPendingLocator(state.pending,original.locator)&&(['user_id','session_generation','table_id','viewer_kind'] as const).every(k=>state.scope[k]===original.scope[k]);}
  #retryState(rendered:PokerIntentContext,sent=false,passive=false):PokerStream {
    const s=this.#state,o=this.#original,q=this.#retryQuery,p=s?.pending;
    if(!s||!o||!p||this.#aux||!this.#ownsOriginal(s,o)||p.receipt||p.phase!==(sent?'SENT':'UNKNOWN')||this.#queryFlight||!q||!this.#current(q.scope)||!samePokerPendingLocator(p,q.locator))throw invalid();
    if(s.table?.viewer.session_id!==o.locator.target_session_id||o.intent.type==='action'&&(s.table.hand?.hand_id!==o.locator.target_hand_id||s.table.hand?.action_sequence!==o.action_sequence))throw invalid();
    return this.#authorize(o.intent,rendered,p.request_id,passive);
  }
  subscribe=(fn:()=>void)=>{this.#listeners.add(fn);return()=>{this.#listeners.delete(fn);};};
  #publish(state:PokerStream|null){
    if(!state||this.#queryFlight&&!currentPokerScope(this.#api,this.#queryFlight.scope,()=>state.scope))this.#queryFlight=undefined;
    if(this.#original&&(!state||!this.#ownsOriginal(state,this.#original)))this.#original=undefined;
    if(!state?.pending){this.#attempt=undefined;this.#retryQuery=undefined;}
    if(this.#retryQuery&&(!state||!currentPokerScope(this.#api,this.#retryQuery.scope,()=>state.scope)||!samePokerPendingLocator(state.pending,this.#retryQuery.locator)))this.#retryQuery=undefined;
    const a=this.#aux;
    for(const old of this.#previousControls)if(!state||(['user_id','session_generation','table_id','viewer_kind'] as const).some(k=>old.scope[k]!==state.scope[k]))this.#previousControls.delete(old);
    if(this.#historyQuery&&(!state||!currentPokerScope(this.#api,this.#historyQuery.scope,()=>state.scope)))this.#historyQuery=undefined;
    if(a&&(!state||(['user_id','session_generation','table_id','viewer_kind'] as const).some(k=>a.scope[k]!==state.scope[k])))this.#aux=undefined;
    if(a&&this.#aux===a&&state){
      if(!a.receipt&&['DISCONNECTED','DEGRADED','RECONNECTING','CONNECTING','AUTH_PENDING'].includes(state.connection_state))a.phase='UNKNOWN';
      if(a.query&&!currentPokerScope(this.#api,a.query.scope,()=>state.scope))a.query=undefined;
      if(state.connection_id&&state.connection_id!==a.connection_id){
        if(!a.receipt)this.#previousControls.add(a);
        a.send=undefined;a.not_found=undefined;this.#aux=undefined;
      }
      if(a.receipt&&(a.receipt.status==='FAILED_NO_EFFECT'||state.connection_state==='LIVE'&&state.table&&BigInt(state.table.table_version)>=BigInt(a.receipt.table_version)))this.#aux=undefined;
    }
    if(this.#chatOriginal&&(!state||!state.chat_pending||state.chat_pending.request_id!==this.#chatOriginal.request_id||(['user_id','session_generation','table_id','viewer_kind'] as const).some(k=>state.scope[k]!==this.#chatOriginal!.scope[k])))this.#chatOriginal=undefined;
    this.#state=state;
    if(state){let ready=false;if(this.#original&&this.#retryQuery)try{this.#retryState(pokerStreamContext(state),false,true);ready=true;}catch{/* Pure capability projection; no transport effects. */}this.#state={...state,can_retry_pending:ready};}
    if(state){
      this.#state={...this.#state!,control_history:this.#previousControls.size?{count:this.#previousControls.size,querying:!!this.#historyQuery}:undefined};
      const control=this.#aux;let canRetry=false;
      if(control?.phase==='UNKNOWN'&&!control.send&&!control.query&&control.not_found&&this.#current(control.not_found))try{this.#controlState(pokerStreamContext(state),control,true);canRetry=true;}catch{/* No effect from a UI capability read. */}
      this.#state={...this.#state!,can_recover_control:!control&&this.#recoveryBoundary(state),control_recovery:control?{
        phase:control.phase,querying:!!control.query,can_retry:canRetry,
        target_current:control.connection_id===state.connection_id&&control.locator.target_session_id===state.table?.viewer.session_id,
      }:undefined};
    }
    this.#listeners.forEach(fn=>fn());
  }
  #current(scope:PokerReadScope,socket?:WebSocket){return !this.#disposed&&!!this.#state&&(!socket||socket===this.#socket)&&currentPokerScope(this.#api,scope,()=>this.#state?.scope??null);}
  #heartbeatExpired(){return !!this.#pingRequest&&performance.now()>=this.#pongDeadline;}
  #visible=()=>{
    const s=this.#state;if(document.visibilityState!=='visible'||!s||s.connection_state!=='LIVE'||!this.#current(s.scope,this.#socket))return;
    if(this.#heartbeatExpired()){this.#fail('POKER_PONG_TIMEOUT',true);return;}
    try{this.sync();}catch{/* sync owns any transport failure; no foreground mutation or accelerated retry. */}
  };
  #cancelReconnect(){clearTimeout(this.#reconnectTimer);this.#reconnectTimer=undefined;}
  #stopReconnect(){this.#autoReconnect=false;this.#cancelReconnect();this.#reconnectBase=RECONNECT_INITIAL_MS;}
  #scheduleReconnect(state:PokerStream){
    if(!this.#autoReconnect||!this.#current(state.scope)||this.#reconnectTimer!==undefined)return;
    const delay=Math.min(RECONNECT_MAX_MS,Math.round(this.#reconnectBase*(1-RECONNECT_JITTER+2*RECONNECT_JITTER*Math.random())));
    this.#reconnectBase=Math.min(RECONNECT_MAX_MS,this.#reconnectBase*2);
    this.#reconnectTimer=setTimeout(()=>{
      if(!this.#autoReconnect||!this.#current(state.scope)||this.#state?.connection_state!=='RECONNECTING')return;
      this.#reconnectTimer=undefined;
      void this.#connectAttempt(state.scope.table_id,state.scope.viewer_kind,state.ticket_intent,true).catch(()=>{if(this.#current(state.scope))this.#fail();});
    },delay);
    this.#publish({...this.#state!,connection_state:'RECONNECTING'});
  }
  #clearTimers(){clearTimeout(this.#heartbeat);clearTimeout(this.#pongTimer);this.#heartbeat=undefined;this.#pongTimer=undefined;this.#pingRequest=undefined;this.#pongDeadline=0;}
  #release(){this.#ticket=undefined;this.#clearTimers();this.#detach?.();this.#detach=undefined;const socket=this.#socket;this.#socket=undefined;if(socket&&socket.readyState<2)try{socket.close(1000,'Client stopped');}catch{/* No raw browser/provider error is exposed. */}}
  #fail(code='POKER_CONNECTION_UNAVAILABLE',retry=false){
    const state=this.#state;this.#cancelReconnect();if(!retry)this.#stopReconnect();this.#release();
    if(state){this.#publish({...disconnectPokerStream(state),connection_state:'DEGRADED',last_error:code});if(retry&&this.#current(state.scope))this.#scheduleReconnect(state);}
  }
  async connect(tableID:string,viewer:PokerAuthority['viewer_kind'],intent:PokerTicketIntent):Promise<void>{
    this.#stopReconnect();this.#autoReconnect=true;
    return this.#connectAttempt(tableID,viewer,intent);
  }
  async #connectAttempt(tableID:string,viewer:PokerAuthority['viewer_kind'],intent:PokerTicketIntent,retainView=false):Promise<void>{
    if(this.#disposed||this.#generation>=Number.MAX_SAFE_INTEGER)throw invalid();
    const previous=this.#state,scope=capturePokerReadScope(this.#api,tableID,viewer,++this.#generation),state=openPokerStream(scope,requestID(),intent,previous??undefined);
    const sameView=retainView&&previous&&(['user_id','session_generation','table_id','viewer_kind'] as const).every(k=>previous.scope[k]===scope[k]);
    if(sameView&&this.#chatOriginal)this.#chatOriginal.scope={...scope};
    this.#cancelReconnect();this.#release();this.#publish({...state,...(sameView?{table:previous.table}:{}),connection_state:'CONNECTING'});
    try{
      const ticket=await mintPokerConnectTicket(this.#api,scope,()=>this.#state?.connection_state==='CONNECTING'?this.#state.scope:null,intent);
      if(!ticket||!this.#current(scope)||this.#state?.connection_state!=='CONNECTING')return;
      const url=new URL(POKER_WS_PATH,location.origin);if(url.protocol!=='https:'&&url.protocol!=='http:')throw invalid();url.protocol=url.protocol==='https:'?'wss:':'ws:';
      this.#ticket=ticket;const socket=this.#factory(url.href,POKER_SUBPROTOCOL);this.#socket=socket;
      const open=()=>{
        if(!this.#current(scope,socket))return;
        if(socket.protocol!==POKER_SUBPROTOCOL||!this.#ticket||this.#state?.connection_state!=='CONNECTING'){this.#fail('POKER_PROTOCOL_INVALID');return;}
        try{
          const frame=encodePokerClient({type:'auth.connect',poker_connect_ticket:this.#ticket},{request_id:state.auth_request_id,table_id:scope.table_id});
          this.#ticket=undefined;this.#publish({...this.#state,connection_state:'AUTH_PENDING'});
          if(this.#current(scope,socket))socket.send(frame);
        }catch{if(this.#current(scope,socket))this.#fail('POKER_CONNECTION_UNKNOWN',true);}finally{this.#ticket=undefined;}
      };
      const message=(event:MessageEvent)=>{
        if(!this.#current(scope,socket))return;
        if(this.#heartbeatExpired()){this.#fail('POKER_PONG_TIMEOUT',true);return;}
        if(typeof event.data!=='string'){this.#fail('POKER_PROTOCOL_INVALID');return;}
        const previous=this.#state!,next=receivePokerStream(previous,scope,event.data);
        if(next.snapshot_generation!==previous.snapshot_generation){this.#snapshotReceivedAt=performance.now();this.#reconnectBase=RECONNECT_INITIAL_MS;}this.#publish(next);
        if(!this.#current(scope,socket))return;
        if(next.connection_state==='DEGRADED'){this.#stopReconnect();this.#release();return;}
        if(this.#pingRequest&&next.last_pong_request_id===this.#pingRequest){clearTimeout(this.#pongTimer);this.#pongTimer=undefined;this.#pingRequest=undefined;this.#pongDeadline=0;}
        if(next.connection_state==='LIVE'){
          this.#scheduleHeartbeat(scope,socket);
          if(previous.snapshot_generation===0&&next.snapshot_generation===1&&next.pending&&!next.pending.receipt)void this.recheckPending();
          if(previous.snapshot_generation===0&&next.snapshot_generation===1&&next.chat_pending&&!next.chat_pending.receipt)void this.recheckChat();
        }
      };
      const close=(event:CloseEvent)=>{
        if(!this.#current(scope,socket))return;
        // 1006 also covers opaque initial upgrade failures: retain UNKNOWN cause, not a guessed auth/network diagnosis.
        const retry=[1006,1012,1013].includes(event.code)||event.code===1011&&event.reason==='POKER_AUTH_UNAVAILABLE';
        if(retry)this.#fail(event.code===1011?'POKER_AUTH_UNAVAILABLE':'POKER_CONNECTION_UNKNOWN',true);
        else{const next=disconnectPokerStream(this.#state!);this.#stopReconnect();this.#release();this.#publish(next);}
      };
      // Browser error has no usable close code. Revoke authority now, but keep close listener for classification.
      const error=()=>{if(this.#current(scope,socket)){this.#ticket=undefined;this.#clearTimers();this.#publish({...disconnectPokerStream(this.#state!),connection_state:'DEGRADED',last_error:'POKER_CONNECTION_UNKNOWN'});}};
      socket.addEventListener('open',open);socket.addEventListener('message',message);socket.addEventListener('close',close);socket.addEventListener('error',error);
      this.#detach=()=>{socket.removeEventListener('open',open);socket.removeEventListener('message',message);socket.removeEventListener('close',close);socket.removeEventListener('error',error);};
    }catch(error){if(this.#current(scope)){
      const retry=error instanceof ApiError&&(error.status===0&&error.code==='POKER_TICKET_NETWORK_UNAVAILABLE'||error.status===503&&error.code==='POKER_AUTH_UNAVAILABLE');
      this.#fail(error instanceof ApiError&&/^[A-Z0-9_]{1,80}$/.test(error.code)?error.code:undefined,retry);
    }}
  }
  #scheduleHeartbeat(scope:PokerReadScope,socket:WebSocket){
    if(this.#heartbeat!==undefined||this.#pingRequest)return;
    this.#heartbeat=setTimeout(()=>{if(!this.#current(scope,socket))return;this.#heartbeat=undefined;try{this.ping();}catch{if(this.#current(scope,socket))this.#fail('POKER_CONNECTION_UNKNOWN',true);}},this.#interval);
  }
  #sendRead(type:'sync.request'|'ping',id:string){
    const s=this.#state,socket=this.#socket;if(!s||!socket||socket.readyState!==1||!this.#current(s.scope,socket)||!['LIVE','SYNCING'].includes(s.connection_state))throw invalid();
    try{socket.send(encodePokerClient({type},{request_id:id,table_id:s.scope.table_id}));}catch{if(this.#current(s.scope,socket))this.#fail('POKER_CONNECTION_UNKNOWN',true);throw invalid();}
  }
  sync(){const s=this.#state;if(!s||!['LIVE','SYNCING'].includes(s.connection_state))throw invalid();this.#publish({...s,connection_state:'SYNCING'});this.#sendRead('sync.request',requestID());}
  ping(){
    const state=this.#state,socket=this.#socket;if(this.#pingRequest||!state||!socket||socket.readyState!==1||!this.#current(state.scope,socket)||!['LIVE','SYNCING'].includes(state.connection_state))throw invalid();
    const id=requestID(),scope=state.scope;clearTimeout(this.#heartbeat);this.#heartbeat=undefined;this.#pingRequest=id;this.#pongDeadline=performance.now()+this.#pongTimeout;
    this.#pongTimer=setTimeout(()=>{if(!this.#current(scope,socket)||this.#pingRequest!==id)return;this.#pongTimer=undefined;this.#fail('POKER_PONG_TIMEOUT',true);},this.#pongTimeout);
    this.#sendRead('ping',id);
  }
  #authorizeChat(rendered:PokerIntentContext,ownedRequest?:string):PokerStream {
    if(this.#heartbeatExpired()){this.#fail('POKER_PONG_TIMEOUT',true);throw invalid();}
    const state=this.#state,socket=this.#socket,chat=state?.table?.chat;
    if(!state||!socket||socket.readyState!==1||socket.bufferedAmount>POKER_MAX_FRAME_BYTES||!this.#current(state.scope,socket)||state.connection_state!=='LIVE'||
      state.chat_pending&&state.chat_pending.request_id!==ownedRequest||!state.table||!matchesPokerIntentContext(rendered,pokerStreamContext(state))||!chat?.enabled||!chat.can_send||chat.muted)throw invalid();
    return state;
  }
  sendChat(message:string,rendered:PokerIntentContext):void {
    const body=message.trim();
    if(!body||new TextEncoder().encode(body).length>2048||/[\p{Cc}\p{Cs}]/u.test(body.replace(/[\n\t]/g,'')))throw invalid();
    const state=this.#authorizeChat(rendered),request=requestID(),scope={...state.scope},socket=this.#socket!;
    this.#chatOriginal={scope,request_id:request,message:body};
    this.#publish({...state,chat_pending:{request_id:request,phase:'SENT'},last_chat_error:undefined});
    try{
      if(!this.#current(scope,socket)||!this.#state?.chat_pending||this.#state.chat_pending.request_id!==request)throw invalid();
      socket.send(encodePokerClient({type:'chat.send',message:body},{request_id:request,table_id:scope.table_id}));
    }catch{
      if(this.#state?.chat_pending?.request_id===request)this.#publish({...this.#state,chat_pending:{request_id:request,phase:'UNKNOWN'},last_chat_error:'POKER_CHAT_OUTCOME_UNCONFIRMED'});
      if(this.#current(scope,socket))this.#fail('POKER_CHAT_OUTCOME_UNCONFIRMED',true);
      throw invalid();
    }
  }
  async recheckChat():Promise<void>{
    const state=this.#state,pending=state?.chat_pending;
    if(!state||!pending||pending.querying||!this.#current(state.scope))return;
    const scope={...state.scope},request=pending.request_id,current=()=>this.#current(scope)&&this.#state?.chat_pending?.request_id===request?this.#state!.scope:null;
    this.#publish({...state,chat_pending:{...pending,querying:true,can_retry:false}});
    try{
      const raw=await this.#api.request(`/api/v1/poker/tables/${scope.table_id}/receipt-query`,'POST',{kind:'chat',mutation_id:request},undefined,()=>current()!==null);
      if(!current())return;
      if(!raw||typeof raw!=='object'||Array.isArray(raw)||(Object.getPrototypeOf(raw)!==Object.prototype&&Object.getPrototypeOf(raw)!==null))throw invalid();
      const value=raw as Record<string,unknown>,keys=Object.keys(value);
      if(value.table_id!==scope.table_id||value.kind!=='chat'||value.mutation_id!==request||(value.state!=='FOUND'&&value.state!=='NOT_FOUND')||keys.some(k=>!['table_id','kind','mutation_id','state','receipt'].includes(k)))throw invalid();
      if(value.state==='FOUND'){
        if(!Object.hasOwn(value,'receipt'))throw invalid();const receipt=parsePokerReceipt(value.receipt,scope.table_id);
        if(receipt.status!=='CHAT_ACCEPTED'||!receipt.chat_sequence)throw invalid();
        this.#publish({...this.#state!,chat_pending:{request_id:request,phase:'ACKNOWLEDGED',receipt},last_chat_error:undefined});
        if(this.#current(scope)&&['LIVE','SYNCING'].includes(this.#state!.connection_state))this.sync();
      }else{
        if(Object.hasOwn(value,'receipt'))throw invalid();this.#publish({...this.#state!,chat_pending:{request_id:request,phase:'UNKNOWN',can_retry:true},last_chat_error:'POKER_CHAT_OUTCOME_UNCONFIRMED'});
      }
    }catch{
      if(current())this.#publish({...this.#state!,chat_pending:{...this.#state!.chat_pending!,querying:false,can_retry:false},last_chat_error:'POKER_CHAT_RECEIPT_UNAVAILABLE'});
    }finally{
      if(current()&&this.#state!.chat_pending!.querying)this.#publish({...this.#state!,chat_pending:{...this.#state!.chat_pending!,querying:false}});
    }
  }
  retryChat(rendered:PokerIntentContext):void {
    const state=this.#state,pending=state?.chat_pending,original=this.#chatOriginal;
    if(!state||!pending||pending.phase!=='UNKNOWN'||!pending.can_retry||pending.querying||!original||original.request_id!==pending.request_id||!this.#current(state.scope)||
      (['user_id','session_generation','table_id','viewer_kind'] as const).some(k=>state.scope[k]!==original.scope[k]))throw invalid();
    const authorized=this.#authorizeChat(rendered,pending.request_id),scope={...authorized.scope},socket=this.#socket!;
    original.scope=scope;this.#publish({...authorized,chat_pending:{request_id:pending.request_id,phase:'SENT'},last_chat_error:undefined});
    try{socket.send(encodePokerClient({type:'chat.send',message:original.message},{request_id:pending.request_id,table_id:scope.table_id}));}
    catch{if(this.#current(scope,socket)){this.#publish({...this.#state!,chat_pending:{request_id:pending.request_id,phase:'UNKNOWN'},last_chat_error:'POKER_CHAT_OUTCOME_UNCONFIRMED'});this.#fail('POKER_CHAT_OUTCOME_UNCONFIRMED',true);}throw invalid();}
  }
  /** Display estimate and an additional stale-deadline guard only; never extends a lease or submits a timer-driven action. */
  serverNow():number{return this.#state?.table?Date.parse(this.#state.table.server_now)+Math.max(0,performance.now()-this.#snapshotReceivedAt):NaN;}
  #authorize(intent:TableIntent,rendered:PokerIntentContext,ownedPending?:string,passive=false):PokerStream {
    if(this.#aux&&intent.type!=='takeover')throw invalid();
    if(this.#heartbeatExpired()){if(!passive)this.#fail('POKER_PONG_TIMEOUT',true);throw invalid();}
    const s=this.#state,socket=this.#socket;
    if(!s||!s.table||!socket||socket.readyState!==1||socket.bufferedAmount>POKER_MAX_FRAME_BYTES||!this.#current(s.scope,socket)||s.connection_state!=='LIVE'||s.pending&&s.pending.request_id!==ownedPending||!matchesPokerIntentContext(rendered,pokerStreamContext(s)))throw invalid();
    const t=s.table,v=t.viewer,h=t.hand,a=pokerStreamAuthority(s),self=t.seats.find(seat=>seat.is_self&&seat.seat_no===v.seat_no);
    if(intent.type==='takeover'){if(!a.can_takeover||!v.session_id)throw invalid();return s;}
    if(!v.session_id||!self||s.scope.viewer_kind!=='PLAYER_SELF'||h?.recovering)throw invalid();
    if((intent.type==='action'||intent.type==='sitout'||intent.type==='resume')&&!a.can_control)throw invalid();
    if(intent.type==='action'){
      const legal=v.legal;if(!v.can_act||!h||h.actor_seat!==v.seat_no||!h.action_deadline_at||!Number.isFinite(this.serverNow())||this.serverNow()>=Date.parse(h.action_deadline_at)||!legal?.actions.includes(intent.action_type))throw invalid();
      if(intent.action_type==='BET'||intent.action_type==='RAISE'){
        const min=intent.action_type==='BET'?legal.minimum_bet_units:legal.minimum_raise_to_units;
        if(!legal.raise_rights||!pokerChipAmount(intent.target_to_units)||BigInt(intent.target_to_units)<=0n||BigInt(intent.target_to_units)<BigInt(min)||BigInt(intent.target_to_units)>BigInt(legal.maximum_raise_to_units)||BigInt(intent.target_to_units)<BigInt(self.street_committed_units))throw invalid();
      }else if(intent.target_to_units!=='0')throw invalid();
    }else if(intent.type==='topup'){
      if(!v.can_top_up||!pokerChipAmount(intent.amount_units)||BigInt(intent.amount_units)<=0n||!v.top_up_min_units||!v.top_up_max_units||BigInt(intent.amount_units)<BigInt(v.top_up_min_units)||BigInt(intent.amount_units)>BigInt(v.top_up_max_units))throw invalid();
    }else if(intent.type==='leave'){if(!v.can_leave||intent.return_to_lobby!==true)throw invalid();}
    else if(intent.type==='sitout'){if(!v.can_sit_out)throw invalid();}
    else if(intent.type==='resume'){if(!v.can_resume)throw invalid();}
    else throw invalid();
    return s;
  }
  /** Explicit Table mutation only. Navigation/reconnect remain separate calls; no generated retries or local settlement. */
  async submit(intent:TableIntent,rendered:PokerIntentContext):Promise<void>{
    if(intent.type==='takeover')return this.#sendControl(rendered);
    if(intent.type==='return_lobby'||intent.type==='reconnect')throw invalid();
    const semantic:Readonly<TableIntent>=Object.freeze(intent.type==='action'?{type:'action',action_type:intent.action_type,target_to_units:intent.target_to_units}:intent.type==='topup'?{type:'topup',amount_units:intent.amount_units}:intent.type==='leave'?{type:'leave',return_to_lobby:intent.return_to_lobby}:{type:intent.type});
    const before=this.#authorize(semantic,rendered),wire=semantic.type==='action'||semantic.type==='sitout'||semantic.type==='resume';
    return this.#transmit(semantic,rendered,before,wire?requestID():businessID(),wire?businessID():null);
  }
  async #transmit(intent:Readonly<TableIntent>,rendered:PokerIntentContext,before:PokerStream,request:string,action:string|null,retry=false):Promise<void>{
    const scope=before.scope,socket=this.#socket!,wire=intent.type==='action'||intent.type==='sitout'||intent.type==='resume';
    let encoded:string|undefined,http:PokerHttpCommand|undefined;
    if(wire){
      const command:PokerClientCommand=intent.type==='action'?{type:'hand.action',action_type:intent.action_type,target_to_units:intent.target_to_units}:{type:intent.type==='sitout'?'session.sit_out_next_hand':'session.resume_play'};
      encoded=encodePokerClient(command,{request_id:request,action_id:action!,table_id:scope.table_id,table_version:before.table!.table_version,hand_id:before.table!.hand?.hand_id,hand_version:before.table!.hand?.hand_version,control_epoch:before.table!.viewer.control_epoch});
    }else if(intent.type==='topup')http={type:'topup',request_id:request,session_id:before.table!.viewer.session_id!,amount_units:intent.amount_units};
    else if(intent.type==='leave')http={type:'leave',request_id:request,session_id:before.table!.viewer.session_id!};
    else throw invalid();
    const next=retry?{...before,pending:{...before.pending!,phase:'SENT' as const}}:beginPokerPending(before,rendered,request,action,intent.type),p=next.pending!;
    const locator=Object.freeze({kind:p.kind,request_id:p.request_id,action_id:p.action_id,target_session_id:p.target_session_id,...(p.target_hand_id?{target_hand_id:p.target_hand_id}:{})});
    if(!retry)this.#original=Object.freeze({scope:Object.freeze({...scope}),locator,intent,action_sequence:intent.type==='action'?before.table!.hand!.action_sequence:undefined});
    const attempt={};this.#attempt=attempt;this.#publish(next);
    const current=()=>this.#current(scope)&&this.#attempt===attempt&&samePokerPendingLocator(this.#state?.pending,locator)&&!this.#state?.pending?.receipt?this.#state!.scope:null;
    try{if(retry)this.#retryState(rendered,true);else this.#authorize(intent,rendered,request);if(!this.#current(scope,socket)||!current())throw invalid();}
    catch{if(current())this.#publish({...this.#state!,pending:retry?{...this.#state!.pending!,phase:'UNKNOWN'}:undefined});throw invalid();}
    this.#retryQuery=undefined;
    try{
      if(encoded!==undefined)socket.send(encoded);
      else{
        const receipt=await submitPokerHttpCommand(this.#api,http!,scope,current);
        if(receipt&&current())this.#publish(acknowledgePokerPending(this.#state!,request,action,receipt));
      }
    }catch(error){
      if(current()){
        const code=error instanceof ApiError&&/^[A-Z0-9_]{1,80}$/.test(error.code)?error.code:'POKER_OUTCOME_UNCONFIRMED';
        this.#publish({...this.#state!,pending:{...this.#state!.pending!,phase:'UNKNOWN'},last_error:code});
        if(wire&&this.#current(scope,socket))this.#fail(code,true);
      }
      throw new ApiError('牌桌操作结果尚未确认，请核对原请求回执，勿重复提交。',0,'POKER_OUTCOME_UNCONFIRMED',true);
    }
  }
  disconnect(){const state=this.#state;this.#stopReconnect();this.#release();if(state)this.#publish(disconnectPokerStream(state));}
  dispose(){if(this.#disposed)return;this.#disposed=true;this.#stopReconnect();this.#release();this.#unsubscribe();document.removeEventListener('visibilitychange',this.#visible);this.#publish(null);this.#listeners.clear();}
}
