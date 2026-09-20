import {RouletteArt} from './RouletteArt';
import {useEffect,useRef,useState} from 'react';
import {Link,useParams} from 'react-router-dom';
import {ApiClient,ApiError} from '../api';
import {chips} from '../games-api';
import {PressureTable} from './PressureTable';
import {DevilTable} from './DevilTable';
import {findReceipt,newKey,newSeed,pendingSlot,persist,readLobby,readPending,readRoom,readySchema,rouletteError,sendPending,stateNames,type Action,type Pending,type RoomView} from './roulette-api';
import './roulette.css';

export function RouletteRoom({client,userID}:{client:ApiClient;userID:string}){
 const {id=''}=useParams(),generation=client.getSessionGeneration(),slot=pendingSlot(userID,generation,id);
 const [view,setView]=useState<RoomView>(),[csrf,setCSRF]=useState(''),[notice,setNotice]=useState(''),[seed,setSeed]=useState(newSeed);
 const [pending,setPending]=useState<Pending|null>(null),[busy,setBusy]=useState(false),[storageBlocked,setStorageBlocked]=useState(false),[time,setTime]=useState(Date.now());
 const alive=useRef(true),lock=useRef(false),clockOffset=useRef(0),poll=useRef<()=>Promise<void>>(async()=>{});
 const current=()=>alive.current&&client.getSessionGeneration()===generation&&String(client.getSnapshot().user?.id)===userID&&!client.getSnapshot().loggingOut;
 useEffect(()=>{
  alive.current=true;let stopped=false,running=false,timer:ReturnType<typeof setTimeout>|undefined,hasToken=false;
  try{setPending(readPending(slot,userID,generation,id));}catch(e){setStorageBlocked(true);setNotice(rouletteError(e));}
  const load=async()=>{if(running)return;running=true;try{
   const next=await readRoom(client,id);if(stopped||!current())return;
   clockOffset.current=Date.parse(next.server_now)-Date.now();setTime(Date.now());setView(old=>old&&BigInt(old.version)>BigInt(next.version)?old:next);
   if(!hasToken){const lobby=await readLobby(client,next.game);if(stopped||!current())return;setCSRF(lobby.csrf_token);hasToken=true;}
  }catch(e){if(!stopped&&current())setNotice(rouletteError(e));}finally{running=false;if(!stopped){clearTimeout(timer);timer=setTimeout(load,document.hidden?10000:1000);}}};
  poll.current=load;void load();const wake=()=>{if(!document.hidden){clearTimeout(timer);void load();}};document.addEventListener('visibilitychange',wake);
  const display=setInterval(()=>setTime(Date.now()),1000);
  return()=>{stopped=true;alive.current=false;clearTimeout(timer);clearInterval(display);document.removeEventListener('visibilitychange',wake);};
 },[client,userID,generation,id,slot]);
 async function act(action?:Action,retry?:Pending){
  if(lock.current||!view||!csrf||storageBlocked||(!retry&&pending))return;
  if(action?.kind==='SURRENDER'&&!window.confirm('确认认输？本局已托管的 '+chips(view.stake_units)+' 筹码将保留在奖池中。'))return;
  lock.current=true;setBusy(true);setNotice('');
  try{
   let p=retry;if(!p){if(!action)return;const ready=action.kind==='READY'?readySchema.parse({client_seed:seed,config_hash:view.binding.config_hash,policy_hash:view.binding.wager_policy_hash,server_seed_hash:view.server_seed_hash,stake_units:view.stake_units}):undefined;p={kind:'command',key:newKey(),user_id:userID,generation,round_id:id,body:{expected_version:view.version,action,...(ready?{ready}:{})}};}
   try{persist(slot,p);}catch{setStorageBlocked(true);setNotice('浏览器未保存恢复标识，本次行动尚未发送。请允许会话存储后刷新。');return;}setPending(p);
   const result=(retry&&await findReceipt(client,p.key))||await sendPending(client,p,csrf,current);if(!current())return;
   if(result.round_id!==id)throw new Error('回执房间不一致，请核对原记录。');sessionStorage.removeItem(slot);setPending(null);await poll.current();
  }catch(e){if(current()){
   if(e instanceof ApiError&&!e.uncertain&&e.status>=400&&e.status<500){sessionStorage.removeItem(slot);setPending(null);await poll.current();}
   setNotice(rouletteError(e));
  }}finally{if(current()){lock.current=false;setBusy(false);}}
 }
 if(!view)return <div className="roulette-page"><Link to="/games">← 游戏目录</Link><h1>正在打开房间</h1><p role={notice?'alert':'status'}>{notice||'读取已保存的桌面与托管状态…'}</p><button onClick={()=>void poll.current()}>重新读取</button></div>;
 const seconds=view.deadline?Math.max(0,Math.ceil((Date.parse(view.deadline)-time-clockOffset.current)/1000)):0,timer=view.deadline?`${Math.floor(seconds/60).toString().padStart(2,'0')}:${(seconds%60).toString().padStart(2,'0')}`:'—';
 const blocked=busy||!!pending||storageBlocked||!csrf,waiting=view.state==='WAITING',myPlayer=view.players.find(p=>p.seat===view.self?.seat);
 const labels:Record<string,string>={JOIN:'免费入座',LEAVE:'离开房间',UNREADY:'取消准备并退款',CANCEL:'取消房间并退款'};
 return <div className="roulette-page roulette-room-page"><nav className="roulette-breadcrumb" aria-label="轮盘导航"><Link to={'/roulette/'+view.game}>← 返回大厅</Link><span>CHALDEA ROYAL SALON</span><Link to="/history">游戏记录 ↗</Link></nav><header className="roulette-heading roulette-room-heading"><div><p className="roulette-kicker">{view.game==='devil-roulette'?'DUEL OF FATE':'A TABLE OF CHANCES'}</p><h1>{view.title}</h1></div><div className="roulette-room-facts"><span>{view.target_players} 人对局</span><span>每人 <strong>{chips(view.stake_units)}</strong></span><span>托管池 <strong>{chips(view.pool_units)}</strong></span><span className="roulette-state">{stateNames[view.state]}</span></div></header>
  {notice&&<p role="alert" className="roulette-notice">{notice}</p>}{pending&&<div className="roulette-notice" role="status">原行动结果待核对；新行动暂停。<button disabled={busy||!csrf} onClick={()=>void act(undefined,pending)}>核对回执 / 原标识重试</button></div>}
  {waiting&&<section className="roulette-ready-panel"><div><h2>{view.players.length} / {view.target_players} 位玩家已入座</h2><p>只有全员准备才开局。{myPlayer?.ready?'你的投入已托管；开局前可取消准备退款。':'准备时确认固定投入、规则版本与下方种子承诺。'}</p><p className="roulette-hint">等待剩余 {timer} · 房主离开会取消房间并退款</p></div>{view.self&&!myPlayer?.ready&&<label>你的随机贡献<input value={seed} maxLength={128} onChange={e=>setSeed(e.target.value)} disabled={blocked} autoComplete="off" spellCheck={false}/></label>}<div className="roulette-wait-actions">{view.actions.map(a=><button className={a.kind==='READY'?'roulette-primary':''} key={a.kind} disabled={blocked||(a.kind==='READY'&&view.self!==null&&BigInt(view.self.available_units)<BigInt(view.stake_units))} onClick={()=>void act(a)}>{a.kind==='READY'?`确认并托管 ${chips(view.stake_units)} 筹码`:labels[a.kind]}</button>)}</div></section>}
  <div className="roulette-room">{view.game==='devil-roulette'?<DevilTable view={view} timer={timer} busy={blocked} onAction={a=>void act(a)}/>:<PressureTable view={view} timer={timer} busy={blocked} onAction={a=>void act(a)}/>}<aside className="roulette-sidebar"><section><h2><RouletteArt name={view.game==='devil-roulette'?'lock':'compass'}/>{view.game==='devil-roulette'?'仅你可见':'你的席位'}</h2>{view.self?<><p className="roulette-hint">这些情报不会向对手或旁观者显示。</p><div className="roulette-intel">{view.self.intel.length?view.self.intel.map(i=><p key={i.index}>{i.index===0?'当前一发':`往后第 ${i.index} 发`}<strong>{i.live?'实弹':'空弹'}</strong></p>):<p>{view.game==='devil-roulette'?'暂无弹序情报':'加压轮盘不公开未验弹巢'}<br/><small>{view.game==='devil-roulette'?'使用放大镜或手机获取线索。':'加压只改变枪况，不改变托管筹码。'}</small></p>}</div><p className="roulette-balance">可用筹码 <strong>{chips(view.self.available_units)}</strong></p></>:<p>旁观视角 · 隐藏情报不公开。</p>}</section><section className="roulette-log"><h2>行动记录</h2><ol>{view.log.length?view.log.slice().reverse().map(e=><li key={e.sequence}><span>#{e.sequence}</span><p>{e.text}</p><time>{new Date(e.at).toLocaleTimeString('zh-CN',{hour:'2-digit',minute:'2-digit'})}</time></li>):<li className="roulette-log-empty">玩家确认准备后，这里将记录每一步公开行动。</li>}</ol></section></aside></div>
  <footer className="roulette-room-footer"><div>{view.actions.some(a=>a.kind==='SURRENDER')&&<button disabled={blocked} onClick={()=>void act({kind:'SURRENDER'})}>⚑ 认输</button>}<button disabled={busy} onClick={()=>{void navigator.clipboard?.writeText(window.location.href).then(()=>setNotice('房间链接已复制。')).catch(()=>setNotice('请从地址栏复制房间链接。'));}}>复制房间链接</button>{view.self&&['FINISHED','CANCELLED'].includes(view.state)&&<Link to={'/history/roulette/'+id}>查看结算与公平证明 →</Link>}</div><span>固定投入 · 局内不追加筹码</span></footer>
  <details className="roulette-proof"><summary>公平承诺与规则版本</summary><dl><dt>服务器种子 SHA-256</dt><dd>{view.server_seed_hash}</dd><dt>配置版本</dt><dd>{view.binding.config_version_id}</dd><dt>配置 SHA-256</dt><dd>{view.binding.config_hash}</dd><dt>下注策略 SHA-256</dt><dd>{view.binding.wager_policy_hash}</dd><dt>规则 / 算法</dt><dd>{view.binding.ruleset_version} / {view.binding.algorithm_version}</dd><dt>对局编号</dt><dd>{id}</dd></dl><p>准备即确认以上承诺与固定投入。终局后参与者可核对种子、行动重放和资金结算。</p></details>
 </div>;
}
