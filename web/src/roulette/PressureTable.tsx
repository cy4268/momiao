import type {CSSProperties} from 'react';
import type {Action,RoomView} from './roulette-api';
import {RouletteArt,seatCrests} from './RouletteArt';

const chamberLabels={UNKNOWN:'未揭示',EMPTY:'已验空膛',LIVE_SPENT:'实弹已击发',DUD_SPENT:'哑弹已击发'};
export function PressureTable({view,onAction,busy,timer}:{view:RoomView;onAction:(action:Action)=>void;busy:boolean;timer:string}){
 const p=view.pressure,order=p?.order||Array.from({length:view.target_players},(_,i)=>i);
 const all=[...order,...view.players.filter(v=>!order.includes(v.seat)).map(v=>v.seat)];
 const labels:Record<string,[string,string]>={PRESSURE_FIRE:['扣动扳机',p?.riposte_target!==null?'反手强制枪':p?.forced?`还需 ${p.forced} 枪`:'查看下一格'],PRESSURE_PASS:['传枪','清空蓄力 · 转向下一位'],PRESSURE_AGAIN:['再开一枪',`蓄力 +1 · 当前 ${p?.charge??0} 层`],PRESSURE_CHARGE:['加压传枪',`实际装入 ${p?.actual_load??0} 发 · 下家 ${p?.forced_shots??1} 枪`],PRESSURE_UNLOAD:['退弹',p?.unload_skips_shot?'抵消最后一枪并传枪':'弃 1 发 · 重转开枪 · 存活后传枪'],PRESSURE_RIPOSTE:['反手还击','加压者强制 1 枪 · 不补枪']};
 const actions=view.actions.filter(a=>a.kind.startsWith('PRESSURE_'));
 return <section className="roulette-table roulette-pressure" aria-label="加压轮盘桌面">
  <div className="roulette-pressure-top"><p className="roulette-table-legend">A TABLE OF CHANCES</p><time aria-label="剩余行动时间"><RouletteArt name="hourglass"/> {timer}</time></div>
  <div className="roulette-roundtable"><div className="roulette-pressure-seats">{all.map((seat,i)=>{
   const player=view.players.find(v=>v.seat===seat),angle=90+(i-Math.max(0,all.indexOf(view.self?.seat??all[0])))*360/all.length;
   return <div key={seat} style={{'--seat-x':`${50+42*Math.cos(angle*Math.PI/180)}%`,'--seat-y':`${50+41*Math.sin(angle*Math.PI/180)}%`} as CSSProperties} className={'roulette-player '+(view.turn_seat===seat?'is-turn':'')+(!player?.alive&&player?' is-out':'')}>
    <RouletteArt name={seatCrests[seat]} className="roulette-medallion"/><strong>{player?.name||`座位 ${seat+1}`}</strong><small>{!player?'等待入座':view.state==='WAITING'?(player.ready?'已准备':'未准备'):player.forfeited?'已认输':!player.alive?'已淘汰':view.turn_seat===seat?'当前持枪':'等待轮转'}{view.self?.seat===seat?' · 你':''}</small>
   </div>;
  })}</div>
  <div className="roulette-pressure-center"><img src="/roulette/pressure-revolver.png" alt=""/>
   <div className="roulette-cylinder-row"><span className="roulette-cylinder-count">枪内<strong>{p?.loaded??'—'} / 6</strong><small>含哑弹</small></span>
    <div className="roulette-cylinder"><RouletteArt name="cylinder" className="roulette-cylinder-art"/>
     <ol className="roulette-chambers" aria-label="公开弹巢状态">{(p?.chambers||Array(6).fill('UNKNOWN') as Array<keyof typeof chamberLabels>).map((state,index)=>{
      const angle=(-90+index*60)*Math.PI/180;
      return <li key={index} data-state={state} style={{'--chamber-x':`${50+24.7*Math.cos(angle)}%`,'--chamber-y':`${50+24.7*Math.sin(angle)}%`} as CSSProperties} className={p?.pointer===index?'is-next':''} aria-label={`第 ${index+1} 格：${chamberLabels[state as keyof typeof chamberLabels]}${p?.pointer===index?'，下一格':''}`}><span aria-hidden="true">{state==='UNKNOWN'?'?':state==='EMPTY'?'空':state==='LIVE_SPENT'?'实':'哑'}</span><small aria-hidden="true">{index+1}</small></li>;
     })}</ol>
    </div>
    <span className="roulette-cylinder-count">待发池<strong>{p?.pool_remaining??'—'} <small>/ 9</small></strong><small>本波哑弹 {p?.duds??'—'} 发</small></span>
   </div>
   <p>{view.state==='WAITING'?'全员准备，危局开启':p?.phase==='VOTE'?'收场，还是再来一轮？':p?.phase==='CHOICE'?'扳机之后，选择在你':p?.phase==='ENDED'?'圆桌的命运已定':'下一枪，谁来承担？'}</p>
  </div></div>
  <div className="roulette-pressure-summary"><span>蓄力 <strong>{p?.charge??0}</strong></span><span>{p?.forced?`欠 ${p.forced} 枪`:'固定筹码投入'}</span><span>未知格不代表空膛</span></div>
  {p?.phase==='VOTE'&&<p className="roulette-vote-status">已投票 {Object.keys(p.votes).length} / {p.order.length} · 任何反对继续新波；到期未投视为同意。</p>}
  <div className="roulette-pressure-actions">{actions.map(a=>{
   if(a.kind==='PRESSURE_VOTE')return <button key={String(a.agree)} disabled={busy} onClick={()=>onAction(a)}><strong>{a.agree?'同意和局':'继续新波'}</strong><small>{a.agree?'存活者平分奖池':'任何反对立即重新装填'}</small></button>;
   const text=labels[a.kind];
   return text&&<button key={a.kind} className={a.kind==='PRESSURE_CHARGE'?'roulette-pressure-charge':a.kind==='PRESSURE_FIRE'?'roulette-primary':''} disabled={busy} onClick={()=>onAction(a)}><RouletteArt name={a.kind==='PRESSURE_CHARGE'?'ammoBox':a.kind==='PRESSURE_RIPOSTE'?'opponent':a.kind==='PRESSURE_PASS'?'compass':a.kind==='PRESSURE_UNLOAD'?'blank':'live'}/><strong>{text[0]}</strong><small>{text[1]}</small></button>;
  })}{!actions.length&&<p>{view.state==='PLAYING'?'等待当前玩家行动；公开事件会自动更新。':'每次准备只托管约定金额，加压不再扣款。'}</p>}</div>
  <p className="roulette-pressure-footnote">金圈为下一格 · ? 为未揭示 · 空／实／哑为已公开状态<br/>每人一次退弹 · 挂机累计后行动时间缩至 25 / 10 秒</p>
 </section>;
}
