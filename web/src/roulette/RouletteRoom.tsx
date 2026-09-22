import {RouletteArt} from './RouletteArt';
import {useEffect,useRef,useState} from 'react';
import {Link,useParams} from 'react-router-dom';
import {ApiClient,ApiError} from '../api';
import {chips} from '../games-api';
import {Modal} from '../ui';
import {PressureTable} from './PressureTable';
import {DevilTable} from './DevilTable';
import {RouletteRules} from './RouletteRules';
import {findReceipt,newKey,newSeed,pendingSlot,persist,readLobby,readPending,readRoom,readySchema,rouletteError,sendPending,stateNames,type Action,type Pending,type RoomView} from './roulette-api';
import './roulette.css';
import './roulette-devil.css';
import './roulette-pressure.css';

type Owner={client:ApiClient;userID:string;generation:number;id:string};
type Panel='ready'|'rules'|'fairness'|'mobile'|null;

export function RouletteRoom({client,userID}:{client:ApiClient;userID:string}){
 const {id=''}=useParams(),generation=client.getSessionGeneration(),slot=pendingSlot(userID,generation,id);
 const [view,setView]=useState<RoomView>(),[csrf,setCSRF]=useState(''),[notice,setNotice]=useState(''),[seed,setSeed]=useState(newSeed);
 const [pending,setPending]=useState<Pending|null>(null),[busy,setBusy]=useState(false),[storageBlocked,setStorageBlocked]=useState(false),[time,setTime]=useState(Date.now());
 const [recovery,setRecovery]=useState(''),[canRetry,setCanRetry]=useState(false),[panel,setPanel]=useState<Panel>(null);
 const ownerRef=useRef<Owner|null>(null),pendingRef=useRef<Pending|null>(null),ackRef=useRef<{key:string;round_id:string;version:string}|null>(null);
 const retryKeyRef=useRef<string|null>(null),receiptAfter=useRef(0),receiptDelay=useRef(1000);
 const lock=useRef<{owner:Owner;token:object}|null>(null),clockOffset=useRef(0),poll=useRef<()=>Promise<void>>(async()=>{});
 const current=(owner:Owner|null=ownerRef.current)=>!!owner&&ownerRef.current===owner&&owner.client===client&&owner.userID===userID&&owner.generation===generation&&owner.id===id&&client.getSessionGeneration()===generation&&String(client.getSnapshot().user?.id)===userID&&!client.getSnapshot().loggingOut;
 const clearPending=(owner:Owner,key:string)=>{if(!current(owner)||pendingRef.current?.key!==key)return false;try{sessionStorage.removeItem(slot);}catch{setStorageBlocked(true);setNotice('浏览器未能清除恢复标识；行动继续保持锁定，请刷新后核对。');return false;}pendingRef.current=null;ackRef.current=null;retryKeyRef.current=null;receiptAfter.current=0;receiptDelay.current=1000;setPending(null);setCanRetry(false);setRecovery('');return true;};
 useEffect(()=>{
  const owner:Owner={client,userID,generation,id};ownerRef.current=owner;pendingRef.current=null;ackRef.current=null;retryKeyRef.current=null;receiptAfter.current=0;receiptDelay.current=1000;setView(undefined);setCSRF('');setNotice('');setSeed(newSeed());setPending(null);setBusy(false);setStorageBlocked(false);setRecovery('');setCanRetry(false);setPanel(null);
  let stopped=false,timer:ReturnType<typeof setTimeout>|undefined,flight:Promise<void>|null=null,hasToken=false,latest:RoomView|undefined;
  try{const restored=readPending(slot,userID,generation,id);pendingRef.current=restored;setPending(restored);if(restored)setRecovery('正在只读核对原行动回执；新行动暂缓。');}catch(e){setStorageBlocked(true);setNotice(rouletteError(e));}
  const paused=()=>document.hidden||navigator.onLine===false;
  const schedule=()=>{clearTimeout(timer);if(!stopped&&current(owner)&&!paused())timer=setTimeout(()=>void load(),1000);};
  const covers=(next:RoomView|undefined,version:string)=>!!next&&next.id===id&&BigInt(next.version)>=BigInt(version);
  const reconcile=async(next?:RoomView)=>{
   const original=pendingRef.current;if(!original)return;
   const acknowledged=ackRef.current;
   if(acknowledged?.key===original.key){retryKeyRef.current=null;setCanRetry(false);if(acknowledged.round_id!==id){setRecovery('原行动回执范围不一致；行动保持锁定。');return;}if(covers(next,acknowledged.version))clearPending(owner,original.key);return;}
   if(Date.now()<receiptAfter.current)return;
   retryKeyRef.current=null;setCanRetry(false);
   setRecovery('正在只读核对原行动回执；新行动暂缓。');
   try{
    const receipt=await findReceipt(client,original.key);if(!current(owner)||pendingRef.current?.key!==original.key)return;
    const confirmed=ackRef.current;if(confirmed?.key===original.key){if(covers(next,confirmed.version))clearPending(owner,original.key);return;}
    if(!receipt){retryKeyRef.current=original.key;setCanRetry(true);receiptAfter.current=Date.now()+receiptDelay.current;receiptDelay.current=Math.min(receiptDelay.current*2,30000);setRecovery('原行动回执尚未出现；将按退避间隔继续只读核对。');return;}
    if(receipt.round_id!==id){setRecovery('原行动回执范围不一致；行动保持锁定。');return;}
    receiptAfter.current=0;receiptDelay.current=1000;ackRef.current={key:original.key,round_id:receipt.round_id,version:receipt.version};setRecovery('原行动回执已确认；正在读取权威桌面。');
    if(covers(next,receipt.version))clearPending(owner,original.key);
   }catch{if(current(owner)&&pendingRef.current?.key===original.key){const confirmed=ackRef.current;if(confirmed?.key===original.key){if(covers(next,confirmed.version))clearPending(owner,original.key);else setRecovery('行动回执已确认；正在读取权威桌面。');}else{receiptAfter.current=Date.now()+receiptDelay.current;receiptDelay.current=Math.min(receiptDelay.current*2,30000);setRecovery('原行动回执暂时不可读取；将按退避间隔继续只读核对。');}}}
  };
  const run=async()=>{
   try{const next=await readRoom(client,id);if(!current(owner))return;if(next.id!==id)throw new Error('房间响应与请求不一致，请重新读取。');const accepted=latest&&BigInt(latest.version)>BigInt(next.version)?latest:next;latest=accepted;clockOffset.current=Date.parse(accepted.server_now)-Date.now();setTime(Date.now());setView(accepted);}catch(e){if(current(owner))setNotice(rouletteError(e));}
   if(!current(owner))return;await reconcile(latest);if(!current(owner))return;
   if(!hasToken&&latest)try{const lobby=await readLobby(client,latest.game);if(!current(owner))return;setCSRF(lobby.csrf_token);hasToken=true;}catch(e){if(current(owner))setNotice(rouletteError(e));}
  };
  const load=()=>{if(stopped||!current(owner)||paused())return Promise.resolve();if(flight)return flight;flight=run().finally(()=>{flight=null;schedule();});return flight;};
  poll.current=load;void load();
  const wake=()=>{clearTimeout(timer);if(!paused()){if(pendingRef.current)receiptAfter.current=0;void load();}},pause=()=>clearTimeout(timer);
  document.addEventListener('visibilitychange',wake);window.addEventListener('online',wake);window.addEventListener('offline',pause);window.addEventListener('focus',wake);
  const display=setInterval(()=>setTime(Date.now()),1000);
  return()=>{stopped=true;clearTimeout(timer);clearInterval(display);document.removeEventListener('visibilitychange',wake);window.removeEventListener('online',wake);window.removeEventListener('offline',pause);window.removeEventListener('focus',wake);if(ownerRef.current===owner)ownerRef.current=null;if(poll.current===load)poll.current=async()=>{};};
 },[client,userID,generation,id,slot]);
 async function act(action?:Action,retry?:Pending){
  const owner=ownerRef.current,restored=pendingRef.current,isRetry=!!retry;
  if(!owner||!current(owner)||lock.current?.owner===owner||!view||!csrf||storageBlocked||(!retry&&(restored||!action))||(retry&&(retry.kind!=='command'||retry.round_id!==id||restored?.key!==retry.key||retryKeyRef.current!==retry.key)))return;
  if(!isRetry&&action?.kind==='SURRENDER'&&!window.confirm('确认认输？本局已托管的 '+chips(view.stake_units)+' 筹码将保留在奖池中。'))return;
  const token={};lock.current={owner,token};setBusy(true);setNotice('');
  let original:Pending|null=null;
  try{
   if(retry)original=retry;else{const ready=action!.kind==='READY'?readySchema.parse({client_seed:seed,config_hash:view.binding.config_hash,policy_hash:view.binding.wager_policy_hash,server_seed_hash:view.server_seed_hash,stake_units:view.stake_units}):undefined;original={kind:'command',key:newKey(),user_id:userID,generation,round_id:id,body:{expected_version:view.version,action:action!,...(ready?{ready}:{})}};try{persist(slot,original);}catch{setStorageBlocked(true);setNotice('浏览器未保存恢复标识，本次行动尚未发送。请允许会话存储后刷新。');return;}}
   retryKeyRef.current=null;receiptAfter.current=0;receiptDelay.current=1000;setCanRetry(false);
   pendingRef.current=original;ackRef.current=null;setPending(original);setRecovery('行动已发送；正在只读核对原行动回执。');
   const result=await sendPending(client,original,csrf,()=>current(owner));if(!current(owner)||pendingRef.current?.key!==original.key)return;
   if(result.round_id!==id){setRecovery('原行动回执范围不一致；行动保持锁定。');return;}
   ackRef.current={key:original.key,round_id:result.round_id,version:result.version};setRecovery('行动回执已确认；正在读取权威桌面。');await poll.current();
  }catch(e){if(current(owner)){
   if(!original){setNotice(action?.kind==='READY'?'随机贡献需为 1—128 字节，且不含控制字符。':rouletteError(e));if(action?.kind==='READY')setPanel('ready');}
   else if(pendingRef.current?.key===original.key){
    if(!isRetry&&e instanceof ApiError&&!e.uncertain&&e.status>=400&&e.status<500&&e.status!==404){clearPending(owner,original.key);await poll.current();setNotice(rouletteError(e));}
    else{receiptAfter.current=0;receiptDelay.current=1000;setRecovery(isRetry?'同标识重试结果尚未确认；页面将恢复自动只读核对。':'行动结果尚未确认；页面只会用原标识自动只读核对，不会重发。');void poll.current();}
   }
  }}finally{if(lock.current?.token===token){lock.current=null;if(current(owner))setBusy(false);}}
 }
 const active=ownerRef.current?.client===client&&ownerRef.current?.userID===userID&&ownerRef.current?.generation===generation&&ownerRef.current?.id===id&&view?.id===id?view:undefined;
 if(!active)return <div className="roulette-page roulette-room-page"><Link to="/games">← 游戏目录</Link><h1>正在打开房间</h1><p role={notice?'alert':'status'}>{notice||'读取已保存的桌面与托管状态…'}</p><button onClick={()=>void poll.current()}>重新读取</button></div>;
 const seconds=active.deadline?Math.max(0,Math.ceil((Date.parse(active.deadline)-time-clockOffset.current)/1000)):0,timer=active.deadline?`${Math.floor(seconds/60).toString().padStart(2,'0')}:${(seconds%60).toString().padStart(2,'0')}`:'—';
 const blocked=busy||!!pending||storageBlocked||!csrf,waiting=active.state==='WAITING',myPlayer=active.players.find(p=>p.seat===active.self?.seat),readyAction=active.actions.find(a=>a.kind==='READY');
 const labels:Record<string,string>={JOIN:'免费入座',LEAVE:'离开房间',UNREADY:'取消准备并退款',CANCEL:'取消房间并退款'};
 const intel=()=> <section><h2><RouletteArt name={active.game==='devil-roulette'?'lock':'compass'}/>{active.game==='devil-roulette'?'仅你可见':'你的席位'}</h2>{active.self?<><p className="roulette-hint">这些情报不会向对手或旁观者显示。</p><div className="roulette-intel">{active.self.intel.length?active.self.intel.map(i=><p key={i.index}>{i.index===0?'当前一发':`往后第 ${i.index} 发`}<strong>{i.live?'实弹':'空弹'}</strong></p>):<p>{active.game==='devil-roulette'?'暂无弹序情报':'加压轮盘不公开未验弹巢'}<br/><small>{active.game==='devil-roulette'?'使用放大镜或手机获取线索。':'加压只改变枪况，不改变托管筹码。'}</small></p>}</div><p className="roulette-balance">可用筹码 <strong>{chips(active.self.available_units)}</strong></p></>:<p>旁观视角 · 隐藏情报不公开。</p>}</section>;
 const log=()=> <section className="roulette-log"><h2>行动记录</h2><ol>{active.log.length?active.log.slice().reverse().map(e=><li key={e.sequence}><span>#{e.sequence}</span><p>{e.text}</p><time>{new Date(e.at).toLocaleTimeString('zh-CN',{hour:'2-digit',minute:'2-digit'})}</time></li>):<li className="roulette-log-empty">玩家确认准备后，这里将记录每一步公开行动。</li>}</ol></section>;
 return <div className={'roulette-page roulette-room-page roulette-salon-room'+(active.game==='devil-roulette'?' roulette-devil-room':' roulette-pressure-room')}><nav className="roulette-breadcrumb" aria-label="轮盘导航"><Link to={'/roulette/'+active.game}>← 返回大厅</Link><span>CHALDEA ROYAL SALON</span><Link to="/history">游戏记录 ↗</Link></nav><header className="roulette-heading roulette-room-heading"><div><p className="roulette-kicker">{active.game==='devil-roulette'?'DUEL OF FATE':'A TABLE OF CHANCES'}</p><h1>{active.title}</h1></div><div className="roulette-room-facts"><span>{active.target_players} 人对局</span><span>每人 <strong>{chips(active.stake_units)}</strong></span><span>托管池 <strong>{chips(active.pool_units)}</strong></span><span className="roulette-state">{stateNames[active.state]}</span></div></header>
  {notice&&panel!=='ready'&&<p role="alert" className="roulette-notice">{notice}</p>}{pending&&<div className="roulette-recovery"><p className="roulette-recovery-status" role="status">{recovery||'正在只读核对原行动回执；新行动暂缓。'}</p>{canRetry&&<details className="roulette-recovery-details"><summary>回执异常详情</summary><p>只读查询暂未找到原回执。若需要主动确认，只会按已保存的原请求与同一标识重试一次；页面不会自动重发。</p><button disabled={busy||!csrf||storageBlocked} onClick={()=>void act(undefined,pending)}>同标识重试原行动</button></details>}</div>}
  {waiting&&<section className="roulette-ready-panel"><div><h2>{active.players.length} / {active.target_players} 位玩家已入座</h2><p>只有全员准备才开局。{myPlayer?.ready?'你的投入已托管；开局前可取消准备退款。':'准备前会展示并确认固定投入、规则版本与服务器种子承诺。'}</p><p className="roulette-hint">等待剩余 {timer} · 房主离开会取消房间并退款</p></div><div className="roulette-wait-actions">{active.actions.map(a=>a.kind==='READY'?<button className="roulette-primary" key={a.kind} disabled={blocked||(active.self!==null&&BigInt(active.self.available_units)<BigInt(active.stake_units))} onClick={()=>setPanel('ready')}>托管准备</button>:<button key={a.kind} disabled={blocked} onClick={()=>void act(a)}>{labels[a.kind]||a.kind}</button>)}</div></section>}
  <div className="roulette-room">{active.game==='devil-roulette'?<DevilTable view={active} timer={timer} busy={blocked} onAction={a=>void act(a)}/>:<PressureTable view={active} timer={timer} busy={blocked} onAction={a=>void act(a)}/>}<aside className="roulette-sidebar">{intel()}{log()}</aside></div>
  <button className="roulette-mobile-tools" onClick={()=>setPanel('mobile')}>{active.game==='devil-roulette'?'查看私密情报与行动记录':'查看席位与行动记录'}</button>
  <footer className="roulette-room-footer"><div>{active.actions.some(a=>a.kind==='SURRENDER')&&<button disabled={blocked} onClick={()=>void act({kind:'SURRENDER'})}>认输</button>}<button onClick={()=>setPanel('rules')}>游戏规则</button><button onClick={()=>setPanel('fairness')}>公平验证</button><button disabled={busy} onClick={()=>{void navigator.clipboard?.writeText(window.location.href).then(()=>setNotice('房间链接已复制。')).catch(()=>setNotice('请从地址栏复制房间链接。'));}}>复制房间链接</button>{active.self&&['FINISHED','CANCELLED'].includes(active.state)&&<Link to={'/history/roulette/'+id}>查看结算与公平证明 →</Link>}</div><span>固定投入 · 局内不追加筹码</span></footer>
  {panel==='ready'&&readyAction&&<Modal title="准备并托管" busy={busy} onClose={()=>{if(!busy)setPanel(null);}}><div className="roulette-ready-confirm"><p>确认后将托管固定投入；只有全员准备才会开局。</p>{notice&&<p role="alert" className="roulette-notice">{notice}</p>}<dl><dt>本局固定投入</dt><dd>{chips(active.stake_units)}</dd><dt>服务器种子 SHA-256</dt><dd>{active.server_seed_hash}</dd><dt>配置 SHA-256</dt><dd>{active.binding.config_hash}</dd><dt>下注策略 SHA-256</dt><dd>{active.binding.wager_policy_hash}</dd></dl><label>你的随机贡献<input value={seed} maxLength={128} onChange={e=>setSeed(e.target.value)} disabled={blocked} autoComplete="off" spellCheck={false}/></label><div className="dialog-actions"><button disabled={busy} onClick={()=>setPanel(null)}>返回</button><button className="roulette-primary" disabled={blocked||(active.self!==null&&BigInt(active.self.available_units)<BigInt(active.stake_units))} onClick={()=>{setPanel(null);void act(readyAction);}}>确认并托管 {chips(active.stake_units)} 筹码</button></div></div></Modal>}
  {panel==='rules'&&<Modal title="游戏规则" onClose={()=>setPanel(null)}><RouletteRules slug={active.game}/></Modal>}
  {panel==='fairness'&&<Modal title="公平验证" onClose={()=>setPanel(null)}><div className="roulette-proof"><dl><dt>服务器种子 SHA-256</dt><dd>{active.server_seed_hash}</dd><dt>配置版本</dt><dd>{active.binding.config_version_id}</dd><dt>配置 SHA-256</dt><dd>{active.binding.config_hash}</dd><dt>下注策略 SHA-256</dt><dd>{active.binding.wager_policy_hash}</dd><dt>规则 / 算法</dt><dd>{active.binding.ruleset_version} / {active.binding.algorithm_version}</dd><dt>对局编号</dt><dd>{id}</dd></dl><p>准备即确认以上承诺与固定投入。终局后参与者可核对种子、行动重放和资金结算。</p></div></Modal>}
  {panel==='mobile'&&<Modal title={active.game==='devil-roulette'?'私密情报与行动记录':'席位与行动记录'} onClose={()=>setPanel(null)}><div className="roulette-mobile-panel">{intel()}{log()}</div></Modal>}
 </div>;
}
