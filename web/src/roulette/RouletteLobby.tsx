import {RouletteArt} from './RouletteArt';
import {useEffect,useRef,useState,type FormEvent} from 'react';
import {Link,useNavigate} from 'react-router-dom';
import {ApiClient,ApiError} from '../api';
import {chips} from '../games-api';
import {amountUnits} from '../economy-api';
import {findReceipt,newKey,pendingSlot,persist,readLobby,readPending,rouletteError,rouletteNames,sendPending,stateNames,type Lobby,type Pending,type RouletteSlug} from './roulette-api';
import './roulette.css';

export function RouletteLobby({client,userID,slug}:{client:ApiClient;userID:string;slug:RouletteSlug}){
 const navigate=useNavigate(),generation=client.getSessionGeneration(),slot=pendingSlot(userID,generation,'create');
 const [lobby,setLobby]=useState<Lobby>(),[stake,setStake]=useState('10'),[players,setPlayers]=useState(slug==='devil-roulette'?2:4);
 const [notice,setNotice]=useState(''),[busy,setBusy]=useState(false),[pending,setPending]=useState<Pending|null>(null),[storageBlocked,setStorageBlocked]=useState(false),[cursor,setCursor]=useState('');
 const alive=useRef(true),lock=useRef(false);
 const current=()=>alive.current&&client.getSessionGeneration()===generation&&String(client.getSnapshot().user?.id)===userID&&!client.getSnapshot().loggingOut;
 useEffect(()=>{
  alive.current=true;let stopped=false,timer:ReturnType<typeof setTimeout>|undefined,running=false;
  try{setPending(readPending(slot,userID,generation));}catch(e){setStorageBlocked(true);setNotice(rouletteError(e));}
  const load=async()=>{if(running)return;running=true;try{const value=await readLobby(client,slug,cursor);if(!stopped&&current())setLobby(value);}catch(e){if(!stopped&&current())setNotice(rouletteError(e));}finally{running=false;if(!stopped)timer=setTimeout(load,document.hidden?10000:5000);}};
  const wake=()=>{if(!document.hidden){clearTimeout(timer);void load();}};void load();document.addEventListener('visibilitychange',wake);
  return()=>{alive.current=false;stopped=true;clearTimeout(timer);document.removeEventListener('visibilitychange',wake);};
 },[client,userID,generation,slug,cursor,slot]);
 async function create(e?:FormEvent,retry?:Pending){
  e?.preventDefault();if(lock.current||!lobby||storageBlocked||(!retry&&(pending||lobby.state!=='PLAY')))return;
  lock.current=true;setBusy(true);setNotice('');
  try{
   if(!retry){const units=amountUnits(stake),step=BigInt(lobby.step_units);if(units===null||step<=0n||units<BigInt(lobby.minimum_units)||units%step!==0n||units>9223372036854775807n/BigInt(players))throw new Error('请按当前最低投入与步长填写金额，最多六位小数。');}
   const p:Pending=retry||{kind:'create',key:newKey(),user_id:userID,generation,body:{game:slug,stake,players}};
   try{persist(slot,p);}catch{setStorageBlocked(true);setNotice('浏览器未保存恢复标识，本次创建尚未发送。请允许会话存储后刷新。');return;}setPending(p);
   const receipt=(retry&&await findReceipt(client,p.key))||await sendPending(client,p,lobby.csrf_token,current);if(!current())return;
   sessionStorage.removeItem(slot);setPending(null);navigate('/roulette/rooms/'+receipt.round_id);
  }catch(e){if(current()){
   if(e instanceof ApiError&&!e.uncertain&&e.status>=400&&e.status<500){sessionStorage.removeItem(slot);setPending(null);}setNotice(rouletteError(e));
  }}finally{if(current()){lock.current=false;setBusy(false);}}
 }
 const devil=slug==='devil-roulette';
 return <div className="roulette-page">
  <nav className="roulette-breadcrumb" aria-label="轮盘导航"><Link to="/games">← 游戏目录</Link><span>ROYAL OBSERVATORY</span><Link to="/history">游戏记录 ↗</Link></nav>
  <header className="roulette-heading"><div><p className="roulette-kicker">{devil?'DUEL OF FATE · 双人策略':'A TABLE OF CHANCES · 多人博弈'}</p><h1>{rouletteNames[slug]}</h1><p>{devil?'知道下一发是什么，还是让对手先做选择？':'一把左轮，一圈同伴。压力会增加，投入不会。'}</p></div><div className="roulette-switch"><Link aria-current={devil?'page':undefined} to="/roulette/devil-roulette">双人对决</Link><Link aria-current={!devil?'page':undefined} to="/roulette/pressure-roulette">圆桌危局</Link></div></header>
  {notice&&<p className="roulette-notice" role="alert">{notice}</p>}
  {pending&&<div className="roulette-notice" role="status">原创建结果尚待核对。<button disabled={busy||!lobby} onClick={()=>void create(undefined,pending)}>核对原请求 / 同标识重试</button></div>}
  {lobby?.own_round_id&&<div className="roulette-resume"><span>你的座位仍在等你。</span><Link to={'/roulette/rooms/'+lobby.own_round_id}>返回未结束房间 →</Link></div>}
  <section className="roulette-lobby-hero">
   <div className="roulette-preview"><RouletteArt name="compass" className="roulette-star"/><span className="roulette-kicker">MOMIAO / CHALDEA ROYAL SALON</span><img src={'/roulette/'+(devil?'devil-shotgun.png':'pressure-revolver.png')} className={devil?'roulette-gun':'roulette-revolver-preview'} alt=""/><div className="roulette-preview-footer"><span>{devil?'4 点生命 · 9 种道具':'3—6 人 · 六格弹巢'}</span><span>固定投入 · 零抽水</span></div></div>
   <form className="roulette-create" onSubmit={e=>void create(e)}><p className="roulette-kicker">TAKE YOUR SEAT</p><h2>开一间新房</h2><label>每人投入 / 筹码<input inputMode="decimal" value={stake} onChange={e=>setStake(e.target.value)} required pattern="[0-9]+([.][0-9]{1,6})?" maxLength={30}/></label><p className="roulette-hint">{lobby?`最低 ${chips(lobby.minimum_units)} 筹码，步长 ${chips(lobby.step_units)} 筹码。`:'正在读取下注策略…'}创建和入座不扣款。</p><label>开局人数<select value={players} disabled={devil} onChange={e=>setPlayers(Number(e.target.value))}>{(devil?[2]:[3,4,5,6]).map(n=><option key={n} value={n}>{n} 人 · 全员准备后开局</option>)}</select></label><div className="roulette-balance">可用筹码 <strong>{lobby?chips(lobby.available_units):'—'}</strong></div><button className="roulette-primary" disabled={!lobby||lobby.state!=='PLAY'||busy||!!pending||storageBlocked||!!lobby.own_round_id}>{busy?'正在核对…':lobby&&lobby.state!=='PLAY'?'暂不接受新房':'创建房间 →'}</button><p className="roulette-hint">每位玩家确认承诺并准备后，才托管相同金额。开局后投入固定；胜者获得全部奖池。</p></form>
  </section>
  <section className="roulette-rooms"><div className="roulette-section-title"><h2>可加入的房间</h2><span>{lobby?'每页最多 50 间':'正在读取…'}</span></div>{lobby&&!lobby.rooms.length?<p className="roulette-empty">桌面已备好。创建房间，邀请朋友先坐下来。</p>:<div className="roulette-room-list">{lobby?.rooms.map(r=><article key={r.id}><RouletteArt name="compass" className="roulette-room-mark"/><div><h3>{r.players[0]?.name||'旅人'}的房间</h3><p>{r.players.length} / {r.target_players} 人 · {stateNames[r.state]}</p></div><div><strong>{chips(r.stake_units)}</strong><small>每人筹码</small></div><Link to={'/roulette/rooms/'+r.id}>{r.state==='WAITING'?'查看房间':'旁观对局'} →</Link></article>)}</div>}<div className="roulette-pagination">{cursor&&<button onClick={()=>setCursor('')}>返回首页</button>}{lobby?.next_cursor&&<button onClick={()=>setCursor(lobby.next_cursor!)}>下一页</button>}</div></section>
  <details className="roulette-rules"><summary>开局前，了解规则与公平验证</summary><p>{devil?'每人 4 点生命，每轮装填 5—8 发实弹与空弹。向自己打出空弹可保留行动；新弹仓先手交替。道具最多携带 4 件，观察所得情报仅本人可见。62 秒未行动将自动向对手射击。':'轮流开枪后可选择传枪、再开一枪或加压。加压只增加局内装弹与强制开枪压力，不追加扣款。每人一次退弹机会；哑弹与空膛都不淘汰。'}</p><p>等待最多 5 分钟，对局最多 30 分钟。开局前取消准备或离开退回实际托管；开局后认输会失去本局投入。双方／存活玩家在时间上限平分剩余奖池。服务器种子承诺在准备前公开，结局后参与者可复算整条行动与结算记录。</p></details>
 </div>;
}
