import {useState,type CSSProperties} from 'react';
import {Modal} from '../ui';
import {itemNames,type Action,type RoomView} from './roulette-api';
import {RouletteArt,rouletteImage} from './RouletteArt';
import './roulette-devil.css';

const descriptions:Record<string,string>={magnifier:'查看当前弹',phone:'查看未来弹',handcuffs:'跳过对手行动',adrenaline:'偷取并立即使用',saw:'下一枪双倍伤害',inverter:'翻转当前弹',beer:'弹出当前弹',cigarette:'回复 1 点生命',medicine:'40% 回复 2 点，否则失去 1 点'};
function Player({view,seat}:{view:RoomView;seat:number}){
 const p=view.players.find(p=>p.seat===seat),own=view.self?.seat===seat;
 return <div className={'roulette-player '+(view.turn_seat===seat?'is-turn':'')+(p?.forfeited?' is-out':'')}>
  <RouletteArt name={own?'lyre':'compass'} className="roulette-medallion"/>
  <div><strong>{p?.name||'等待入座'}</strong><span className="roulette-player-label">{own?'你':p?'对手':'空位'}{p?.ready&&view.state==='WAITING'?' · 已准备':''}</span>
   <div className="roulette-hp" aria-label={`${p?.hp||0} / 4 点生命`}>{[0,1,2,3].map(i=><RouletteArt key={i} name={i<(p?.hp||0)?'life':'spentLife'}/>)}<small>{p?.hp||0} / 4</small></div>
  </div>
 </div>;
}
export function DevilTable({view,onAction,busy,timer}:{view:RoomView;onAction:(action:Action)=>void;busy:boolean;timer:string}){
 const [chooseStolen,setChooseStolen]=useState(false);
 const self=view.self?.seat??0,other=1-self,d=view.devil;
 const shoot=view.actions.filter(a=>a.kind==='DEVIL_SHOOT'),items=view.actions.filter(a=>a.kind==='DEVIL_ITEM');
 const ownItems=view.self?.items||[],unique=Array.from(new Set(ownItems));
 return <section className="roulette-table roulette-devil" data-state={view.state} aria-label="恶魔轮盘桌面" style={{'--roulette-table-art':`url("${rouletteImage('table')}")`} as CSSProperties}>
  <span className="roulette-table-legend">SAME STARS,<br/>DIFFERENT DESTINIES.</span>
  <Player view={view} seat={other}/>
  <div className="roulette-turn"><h2>{view.state==='PLAYING'?(view.turn_seat===view.self?.seat?'轮到你了':`等待 ${view.players.find(p=>p.seat===view.turn_seat)?.name||'玩家'} 行动`):view.state==='WAITING'?'命运尚未装填':'这一局，尘埃落定'}</h2><time aria-label="剩余行动时间"><RouletteArt name="hourglass"/> {timer}</time></div>
  <div className="roulette-gun-stage"><div className="roulette-orbit" aria-hidden="true"/><img src={rouletteImage('shotgun')} className="roulette-gun" alt=""/></div>
  <div className="roulette-ammo"><span>剩余 <strong>{d?.remaining??'—'}</strong> 发</span><span><RouletteArt name="live"/>实弹 <strong>{d?.live??'?'}</strong></span><span><RouletteArt name="blank"/>空弹 <strong>{d?.blank??'?'}</strong></span></div>
  <div className="roulette-effects" aria-live="polite">{d?.saw&&<span>手锯已启用 · 下一枪命中伤害 ×2</span>}{d?.cuffed.some(Boolean)&&<span>手铐已生效</span>}{d?.live===null&&<span>逆转后弹数暂时隐藏</span>}</div>
  <Player view={view} seat={self}/>
  <div className="roulette-action-bar"><div className="roulette-items">{unique.length?unique.map(item=>{
   const actions=items.filter(a=>a.kind==='DEVIL_ITEM'&&a.item===item);
   return <div className="roulette-item" key={item}><RouletteArt name={item} className="roulette-item-icon"/><strong>{itemNames[item]} <small>×{ownItems.filter(v=>v===item).length}</small></strong><small>{descriptions[item]}</small>
    {item==='adrenaline'?<button disabled={busy||!actions.length} onClick={()=>setChooseStolen(true)}>选择目标</button>:<button aria-label={'使用'+itemNames[item]} disabled={busy||!actions.length} onClick={()=>actions[0]&&onAction(actions[0])}>使用</button>}
   </div>;
  }):<p className="roulette-hint">{view.state==='WAITING'?'全员准备后发放道具':'当前没有道具'}</p>}</div>
  <div className="roulette-shots">{(['SELF','OPPONENT'] as const).map(target=>{
   const action=shoot.find(a=>a.kind==='DEVIL_SHOOT'&&a.target===target);
   return <button key={target} className={target==='SELF'?'roulette-shoot-self':'roulette-shoot-opponent'} disabled={busy||!action} onClick={()=>action&&onAction(action)}><RouletteArt name={target==='SELF'?'self':'opponent'}/><strong>{target==='SELF'?'对自己开枪':'对对手开枪'}</strong><small>{target==='SELF'?'空弹保留行动':'让对方承担'}</small></button>;
  })}</div></div>
  {chooseStolen&&<Modal title="选择偷取的道具" onClose={()=>setChooseStolen(false)}><p>偷取后立即使用。只显示服务器允许的目标。</p><div className="roulette-steal-options">{items.filter(a=>a.kind==='DEVIL_ITEM'&&a.item==='adrenaline').map(a=>a.kind==='DEVIL_ITEM'&&a.stolen_item&&<button key={a.stolen_item} disabled={busy} onClick={()=>{setChooseStolen(false);onAction(a);}}><RouletteArt name={a.stolen_item}/><span>偷取 {itemNames[a.stolen_item]}</span></button>)}</div></Modal>}
 </section>;
}
