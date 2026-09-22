import {RouletteArt,rouletteImage} from './RouletteArt';
import {useEffect,useRef,useState,type CSSProperties,type FormEvent} from 'react';
import {Link,useNavigate} from 'react-router-dom';
import {ApiClient,ApiError} from '../api';
import {chips} from '../games-api';
import {amountUnits} from '../economy-api';
import {Modal} from '../ui';
import {RouletteRules} from './RouletteRules';
import {findReceipt,newKey,pendingSlot,persist,readLobby,readPending,readRoom,rouletteError,rouletteNames,sendPending,stateNames,type Lobby,type Pending,type RouletteSlug} from './roulette-api';
import './roulette.css';
import './roulette-devil.css';
import './roulette-pressure.css';

type Owner={client:ApiClient;userID:string;generation:number;slug:RouletteSlug};
export function RouletteLobby({client,userID,slug}:{client:ApiClient;userID:string;slug:RouletteSlug}){
 const navigate=useNavigate(),generation=client.getSessionGeneration(),slot=pendingSlot(userID,generation,'create'),devil=slug==='devil-roulette';
 const [lobby,setLobby]=useState<Lobby>(),[stake,setStake]=useState('10'),[players,setPlayers]=useState(devil?2:4);
 const [notice,setNotice]=useState(''),[busy,setBusy]=useState(false),[pending,setPending]=useState<Pending|null>(null),[storageBlocked,setStorageBlocked]=useState(false),[cursor,setCursor]=useState('');
 const [canRetry,setCanRetry]=useState(false);
 const [panel,setPanel]=useState<'rules'|'fairness'|'recovery'|null>(null);
 const retryKeyRef=useRef<string|null>(null);
 const ownerRef=useRef<Owner|null>(null),pendingRef=useRef<Pending|null>(null),lock=useRef<object|null>(null),poll=useRef<()=>Promise<void>>(async()=>{});
 const current=(owner:Owner|null)=>!!owner&&ownerRef.current===owner&&owner.client===client&&owner.userID===userID&&owner.slug===slug&&owner.generation===generation&&client.getSessionGeneration()===generation&&String(client.getSnapshot().user?.id)===userID&&!client.getSnapshot().loggingOut;
 const finish=async(owner:Owner,key:string,round:string,version:string)=>{
  if(!current(owner)||pendingRef.current?.key!==key)return;
  try{const view=await readRoom(client,round);if(!current(owner)||pendingRef.current?.key!==key||view.id!==round||BigInt(view.version)<BigInt(version))return;
   const p=pendingRef.current;if(p.kind!=='create'||view.game!==p.body.game)return;
  }catch{return;}
  try{sessionStorage.removeItem(slot);}catch{setStorageBlocked(true);setNotice('浏览器未能更新恢复标识，创建仍保持锁定。');return;}
  pendingRef.current=null;retryKeyRef.current=null;setCanRetry(false);setPending(null);navigate('/roulette/rooms/'+round);
 };
 useEffect(()=>{
  const owner:Owner={client,userID,generation,slug};ownerRef.current=owner;pendingRef.current=null;retryKeyRef.current=null;setCanRetry(false);setLobby(undefined);setNotice('');setPending(null);setBusy(false);setStorageBlocked(false);setPanel(null);setPlayers(slug==='devil-roulette'?2:4);
  let stopped=false,timer:ReturnType<typeof setTimeout>|undefined,flight:Promise<void>|null=null,delay=1000,nextReceipt=0;
  try{const saved=readPending(slot,userID,generation);pendingRef.current=saved;setPending(saved);}catch(e){setStorageBlocked(true);setNotice(rouletteError(e));}
  const paused=()=>document.hidden||navigator.onLine===false;
  const run=async()=>{
   try{const value=await readLobby(client,slug,cursor);if(!current(owner))return;setLobby(value);}catch(e){if(current(owner))setNotice(rouletteError(e));}
   const p=pendingRef.current;if(!current(owner)||!p||lock.current||Date.now()<nextReceipt)return;
   retryKeyRef.current=null;setCanRetry(false);
   try{const receipt=await findReceipt(client,p.key);if(!current(owner)||pendingRef.current?.key!==p.key)return;if(receipt){await finish(owner,p.key,receipt.round_id,receipt.version);}else{retryKeyRef.current=p.key;setCanRetry(true);}}catch{/* Unknown receipts retain the original key and the create lock. */}
   nextReceipt=Date.now()+delay;delay=Math.min(delay*2,30000);
  };
  const load=()=>{
   if(stopped||!current(owner)||paused())return Promise.resolve();if(flight)return flight;
   clearTimeout(timer);flight=run().finally(()=>{flight=null;if(!stopped&&current(owner)&&!paused())timer=setTimeout(()=>void load(),pendingRef.current?Math.min(5000,Math.max(1000,nextReceipt-Date.now())):5000);});return flight;
  };
  poll.current=load;void load();
  const wake=()=>{clearTimeout(timer);if(!paused()){nextReceipt=0;void load();}},pause=()=>clearTimeout(timer);
  document.addEventListener('visibilitychange',wake);window.addEventListener('online',wake);window.addEventListener('offline',pause);window.addEventListener('focus',wake);
  return()=>{stopped=true;clearTimeout(timer);document.removeEventListener('visibilitychange',wake);window.removeEventListener('online',wake);window.removeEventListener('offline',pause);window.removeEventListener('focus',wake);if(ownerRef.current===owner)ownerRef.current=null;if(poll.current===load)poll.current=async()=>{};};
 },[client,userID,generation,slug,cursor,slot]);
 async function create(e?:FormEvent,retry?:Pending){
  e?.preventDefault();const owner=ownerRef.current;
  if(!current(owner)||lock.current||!lobby||lobby.game!==slug||storageBlocked||(!retry&&(pendingRef.current||lobby.state!=='PLAY'||lobby.own_round_id))||(retry&&(pendingRef.current?.key!==retry.key||retry.kind!=='create'||retry.body.game!==slug||retryKeyRef.current!==retry.key)))return;
  const token={};lock.current=token;retryKeyRef.current=null;setCanRetry(false);setBusy(true);setNotice('');
  let p:Pending|null=null;
  try{
   const count=devil?2:players;
   if(!retry){const units=amountUnits(stake),step=BigInt(lobby.step_units);if(units===null||step<=0n||units<BigInt(lobby.minimum_units)||units%step!==0n||units>9223372036854775807n/BigInt(count))throw new Error('请按当前最低投入与步长填写金额，最多六位小数。');}
   p=retry||{kind:'create',key:newKey(),user_id:userID,generation,body:{game:slug,stake,players:count}};
   try{persist(slot,p);}catch{setStorageBlocked(true);setNotice('浏览器未保存恢复标识，本次创建尚未发送。请允许会话存储后刷新。');return;}
   pendingRef.current=p;setPending(p);
   let receipt=retry?await findReceipt(client,p.key):null;if(!current(owner))return;
   receipt??=await sendPending(client,p,lobby.csrf_token,()=>current(owner));if(!current(owner))return;
   await finish(owner!,p.key,receipt.round_id,receipt.version);
  }catch(e){if(current(owner)){
   if(!retry&&p&&pendingRef.current?.key===p.key&&e instanceof ApiError&&!e.uncertain&&e.status>=400&&e.status<500&&e.status!==404){
    try{sessionStorage.removeItem(slot);pendingRef.current=null;setPending(null);}catch{setStorageBlocked(true);}
    setNotice(rouletteError(e));
   }else if(!p)setNotice(rouletteError(e));
  }}finally{if(lock.current===token){lock.current=null;if(current(owner)){setBusy(false);void poll.current();}}}
 }
 const active=lobby?.game===slug?lobby:undefined;
 const createForm=<form className="roulette-create" onSubmit={e=>void create(e)}>
  <div className="roulette-create-art" aria-hidden="true"><img src={rouletteImage(devil?'shotgun':'revolver')} alt=""/>{devil&&<><RouletteArt name="magnifier"/><RouletteArt name="live"/></>}</div>
  <p className="roulette-kicker">A SEAT AWAITS YOU</p><h2>{devil?'开启一场命运对决':'创建圆桌'}</h2>
  <label>每人投入 / 筹码<input inputMode="decimal" value={stake} onChange={e=>setStake(e.target.value)} required pattern="[0-9]+([.][0-9]{1,6})?" maxLength={30}/></label>
  <p className="roulette-hint">{active?`最低 ${chips(active.minimum_units)} · 步长 ${chips(active.step_units)} 筹码`:'正在读取投入策略…'}</p>
  <label>开局人数{devil?<input value="2 人 · 双人对决" disabled/>:<select value={players} onChange={e=>setPlayers(Number(e.target.value))}>{[3,4,5,6].map(n=><option key={n} value={n}>{n} 人 · 全员准备后开局</option>)}</select>}</label>
  <div className="roulette-balance">可用筹码 <strong>{active?chips(active.available_units):'—'}</strong></div>
  <button className="roulette-primary" disabled={!active||active.state!=='PLAY'||busy||!!pending||storageBlocked||!!active.own_round_id}>{pending?'正在同步创建结果…':active?.own_round_id?'请先返回当前房间':active&&active.state!=='PLAY'?'暂不接受新房':'创建房间 →'}</button>
  <p className="roulette-hint">创建和入座不扣款。准备时确认并托管；全员准备后开局，局内不追加筹码。</p>
 </form>;
 const rooms=<section className="roulette-rooms"><div className="roulette-section-title"><div><p className="roulette-kicker">{devil?'FIND YOUR DUEL':'FIND YOUR TABLE'}</p><h2>{devil?'等待命运的房间':'寻找圆桌'}</h2></div><span>{active?`${active.rooms.length} 间 · 当前页`:'正在读取…'}</span></div>
  {active&&!active.rooms.length?<div className="roulette-empty"><RouletteArt name="compass"/><h3>{devil?'今夜的第一场对决':'圆桌正待落座'}</h3><p>还没有开放房间。创建一间，邀请朋友落座吧。</p></div>:<div className="roulette-room-list" tabIndex={0} aria-label="房间列表">{active?.rooms.map((r,index)=><article key={r.id}><RouletteArt name={(['compass','lyre','lily','eagle'] as const)[index%4]} className="roulette-room-mark"/><div className="roulette-room-name"><h3>{r.players[0]?.name||'旅人'}的房间</h3><p>{r.players.length} / {r.target_players} 人 <span className={'room-state '+(r.state==='WAITING'?'is-waiting':'')}>{stateNames[r.state]}</span></p></div><div className="roulette-room-stake"><strong>{chips(r.stake_units)}</strong><small>每人筹码</small></div><Link to={'/roulette/rooms/'+r.id}>{r.state==='WAITING'?'查看房间':'旁观对局'} →</Link></article>)}</div>}
  <div className="roulette-pagination">{cursor&&<button disabled={busy||!!pending} onClick={()=>setCursor('')}>返回首页</button>}{active?.next_cursor&&<button disabled={busy||!!pending} onClick={()=>setCursor(active.next_cursor!)}>下一页</button>}</div>
  <p className="roulette-lobby-note">{devil?'同一片星空，不同的选择。':'一把左轮，一圈同伴。'}<span>{devil?'4 点生命 · 9 种道具 · 固定投入':'3—6 人圆桌 · 六格弹巢 · 固定投入'}</span></p>
 </section>;
 return <div className={'roulette-page roulette-salon-lobby'+(devil?' roulette-devil-lobby':' roulette-pressure-lobby')} style={{'--roulette-room-art':`url("${rouletteImage('room')}")`,'--roulette-table-art':`url("${rouletteImage('table')}")`} as CSSProperties}>
  <nav className="roulette-breadcrumb" aria-label="轮盘导航"><Link to="/games">← 游戏目录</Link><span>CHALDEA ROYAL SALON</span><Link to="/history">游戏记录 ↗</Link></nav>
  <header className="roulette-heading"><div><p className="roulette-kicker">{devil?'DUEL OF FATE · ROOM LOBBY':'A TABLE OF CHANCES · 多人博弈'}</p><h1>{devil?'命运对决 · ':'圆桌危局 · '}{rouletteNames[slug]}</h1><p>{devil?'在星光落定之前，选好你的对手。':'一把左轮，一圈同伴。压力会增加，投入不会。'}</p></div><div className="roulette-switch"><Link aria-current={devil?'page':undefined} to="/roulette/devil-roulette">双人对决</Link><Link aria-current={!devil?'page':undefined} to="/roulette/pressure-roulette">圆桌危局</Link></div></header>
  {notice&&<p className="roulette-notice" role="alert">{notice}</p>}
  {active?.own_round_id&&<div className="roulette-resume"><span>你的座位仍在等你。</span><Link to={'/roulette/rooms/'+active.own_round_id}>返回未结束房间 →</Link></div>}
  <div className="roulette-salon-lobby-grid">{rooms}{createForm}</div>
  <footer className="roulette-room-footer"><div><button onClick={()=>setPanel('rules')}>游戏规则</button><button onClick={()=>setPanel('fairness')}>公平验证</button>{pending&&<button onClick={()=>setPanel('recovery')}>创建状态</button>}</div><span>固定投入 · 准备前不扣款</span></footer>
  {panel==='rules'&&<Modal title="游戏规则" onClose={()=>setPanel(null)}><RouletteRules slug={slug}/></Modal>}
  {panel==='fairness'&&<Modal title="公平验证" onClose={()=>setPanel(null)}><div className="roulette-proof"><p>每间房在准备前公开服务器种子承诺，准备时绑定固定投入与以下版本。对局结束后，参与者从游戏记录核对种子与行动重放。</p>{active&&<dl><dt>规则版本</dt><dd>{active.binding.ruleset_version}</dd><dt>算法版本</dt><dd>{active.binding.algorithm_version}</dd><dt>配置 SHA-256</dt><dd>{active.binding.config_hash}</dd><dt>下注策略 SHA-256</dt><dd>{active.binding.wager_policy_hash}</dd></dl>}</div></Modal>}
  {panel==='recovery'&&pending&&<Modal title="创建状态" onClose={()=>setPanel(null)}><p role="status">正在用原标识自动只读核对。结果未确认前，创建保持锁定。</p><details><summary>回执异常详情</summary><p>原标识：{pending.key}</p><p>仅在需要重试原操作时使用；不会创建另一份请求。</p>{pending.kind==='create'&&pending.body.game!==slug?<Link to={'/roulette/'+pending.body.game}>返回原游戏查看创建状态</Link>:<button disabled={!canRetry||busy||storageBlocked||!active} onClick={()=>void create(undefined,pending)}>同标识重试原创建</button>}</details></Modal>}
 </div>;
}
