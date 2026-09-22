import { useId, useState, type CSSProperties } from 'react';
import { chipInput, chipText, chips, connectionLabel, hasAuthority, intentContext, pokerUnits, stateLabel, UNITS_PER_CHIP, type ActionType, type PokerTableProps, type TableIntent } from './poker-ui-types';
import { PokerActionTimeline } from './PokerActionTimeline';
import { PokerChat } from './PokerChat';
import { PokerHostControls } from './PokerHostControls';
import { Modal } from '../ui';
import { PokerArt } from './PokerArt';
import './poker-ui.css';
import './poker-salon.css';

const suits=['♣','♦','♥','♠'], suitNames=['梅花','方块','红桃','黑桃'], ranks=['A','2','3','4','5','6','7','8','9','10','J','Q','K'];
function Card({card,back=false}:{card?:number;back?:boolean}) {
  if(back)return <span className="pk-card pk-card-back" role="img" aria-label="未公开手牌"><PokerArt name="cardBack" sizes="70px"/></span>;
  if(card===undefined||!Number.isInteger(card)||card<0||card>51)return <span className="pk-card pk-card-empty" aria-label="待发公共牌">·</span>;
  const suit=Math.floor(card/13),rank=card%13;
  return <span className={`pk-card${suit===1||suit===2?' pk-card-blue':''}`} role="img" aria-label={`${ranks[rank]} ${suitNames[suit]}`}><b>{ranks[rank]}</b><span aria-hidden="true">{suits[suit]}</span></span>;
}
const positions:Record<number,number[][]>={
  2:[[50,88],[50,10]],3:[[50,88],[17,20],[83,20]],4:[[50,88],[12,45],[50,10],[88,45]],
  5:[[50,88],[12,61],[27,12],[73,12],[88,61]],6:[[50,88],[12,68],[12,27],[50,9],[88,27],[88,68]],
  7:[[50,90],[15,77],[12,39],[32,10],[68,10],[88,39],[85,77]],
  8:[[50,90],[19,80],[10,51],[22,16],[50,8],[78,16],[90,51],[81,80]],
  9:[[50,91],[22,83],[10,60],[11,28],[32,9],[68,9],[89,28],[90,60],[78,83]],
};

export function PokerTable(p: PokerTableProps) {
  const [panel,setPanel]=useState<'rules'|'fairness'|'history'|'session'|'status'|'host'|'activity'|null>(null);
  const [activity,setActivity]=useState<'timeline'|'chat'>('timeline');
  const id=useId(),t=p.table,v=t.viewer,h=t.hand,legal=v.legal,context=intentContext(p.authority,t);
  const self=t.seats.find(s=>s.is_self && s.seat_no===v.seat_no), seated=!!v.session_id;
  const ready=!p.mutation_blocked && hasAuthority(p.authority) && !h?.recovering && !!t.table_id && /^[0-9]+$/.test(t.table_version) && /^[0-9]+$/.test(v.control_epoch);
  const controlReady=ready&&p.authority.can_control===true;
  const readOnly=p.authority.can_control!==true,live=p.authority.connection_state==='LIVE';
  const recovery=p.authority.control_recovery,recoveryAllowed=!recovery&&(!p.authority.pending||p.authority.can_recover_control===true);
  const history=p.authority.control_history;
  const canTakeover=!p.mutation_blocked&&p.authority.can_takeover&&recoveryAllowed;
  const canRequestConnection=live&&seated&&p.authority.viewer_kind==='PLAYER_SELF'&&p.authority.ticket_intent==='READ_ONLY';
  const serverTime=Date.parse(t.server_now),deadline=h?.action_deadline_at?Date.parse(h.action_deadline_at):NaN;
  const expired=!Number.isFinite(serverTime)||!Number.isFinite(deadline)||serverTime>=deadline;
  const act=controlReady && seated && !!self && p.authority.viewer_kind==='PLAYER_SELF' && v.can_act && !h?.recovering &&
    !!h && !!h.hand_id && /^[0-9]+$/.test(h.hand_version) && /^[0-9]+$/.test(h.action_sequence) && h.actor_seat===v.seat_no && !expired;
  const can=(action:ActionType)=>act && !!legal?.actions.includes(action);
  const selected=p.ui.bet.action_type, amount=chipInput(p.ui.bet.amount_chips),committed=pokerUnits(self?.street_committed_units);
  const min=pokerUnits(selected==='BET'?legal?.minimum_bet_units:legal?.minimum_raise_to_units), max=pokerUnits(legal?.maximum_raise_to_units);
  const amountValid=selected==='ALL_IN'?can('ALL_IN'):can(selected) && !!legal?.raise_rights && amount!==undefined && committed!==undefined &&
    min!==undefined && max!==undefined && amount>=min && amount<=max && amount>=committed;
  const topup=chipInput(p.ui.top_up_chips),topMin=pokerUnits(v.top_up_min_units),topMax=pokerUnits(v.top_up_max_units);
  const canTopUp=ready&&seated&&v.can_top_up&&topMin!==undefined&&topMax!==undefined&&topMin>0n&&topMax>=topMin;
  const topValid=canTopUp&&topup!==undefined&&topup>=topMin!&&topup<=topMax!;
  const canLeave=ready&&seated&&v.can_leave;
  const n=Number.isInteger(t.max_seats)&&t.max_seats>=2&&t.max_seats<=9?t.max_seats:2;
  const seatPositions=positions[n], rotation=seated&&Number.isInteger(v.seat_no)&&v.seat_no!>=1&&v.seat_no!<=n?(v.seat_no!-1):0;
  const displayTime=Date.parse(p.display_now??t.server_now),seconds=Number.isFinite(displayTime)&&Number.isFinite(deadline)?Math.max(0,Math.ceil((deadline-displayTime)/1000)):undefined;
  const timerText=h?.recovering?'服务恢复中 · 行动已暂停':h?.street==='SETTLED'?'本手已结束 · 等待下一手':!h?.actor_seat?'等待服务端行动窗口':seconds===undefined?'等待服务端行动时限':`服务端行动剩余 ${seconds} 秒`;
  const send=(intent:TableIntent)=>{if(!p.mutation_blocked||intent.type==='return_lobby'||intent.type==='reconnect')p.onIntent(intent,context);};
  const showAmount=!readOnly&&(legal?.actions.some(a=>a==='BET'||a==='RAISE')??false);
  const ownCards=seated&&p.authority.viewer_kind==='PLAYER_SELF'&&self?.is_self?self.hole_cards?.slice(0,2):undefined;
  const targetDisplay=selected==='ALL_IN'?chips(self?.stack_units):amount===undefined?'—':chips(amount.toString());
  const sliderEnabled=act&&!!legal?.raise_rights&&min!==undefined&&max!==undefined&&max>min;
  const sliderPosition=sliderEnabled&&amount!==undefined?Number(((amount<min!?min!:amount>max!?max!:amount)-min!)*1000n/(max!-min!)):0;

  const activityPanel=()=>(<><div className="pk-activity-tabs" aria-label="牌桌侧栏"><button aria-pressed={activity==='timeline'} onClick={()=>setActivity('timeline')}>行动记录</button>{t.chat&&<button aria-pressed={activity==='chat'} onClick={()=>setActivity('chat')}>聊天</button>}</div>
      <div className="pk-activity-content" hidden={activity!=='timeline'}><PokerActionTimeline timeline={t.timeline}/></div>
      {t.chat&&<div className="pk-activity-content" hidden={activity!=='chat'}><PokerChat chat={t.chat} authority={p.authority} context={context} draft={p.chat_draft??''} mutation_blocked={p.mutation_blocked} onDraftChange={p.onChatDraftChange} onSend={p.onSendChat} onRetry={p.onRetryChat}/></div>}</>);
  const statusText=p.recovery_status??p.notice??(h?.recovering?'牌局正在恢复，请稍候':p.authority.pending||recovery||history?'正在自动确认操作结果':p.mutation_blocked?'当前暂停新操作':p.authority.connection_state!=='LIVE'?connectionLabel[p.authority.connection_state]:'');
  const modalTitle={rules:'游戏规则',fairness:'公平验证',history:'牌局详情',session:'会话与补充筹码',status:'连接与操作状态',host:'房主管理',activity:'行动记录与聊天'};
  return <section className="pk-ui pk-table pk-salon" aria-label="沉浸式 Poker 牌桌">
    <PokerArt name="room" className="pk-salon-backdrop"/>
    <header className="pk-table-header"><div className="pk-header-left"><button className="pk-back" aria-label="返回大厅" disabled={seated?!canLeave:p.authority.pending} onClick={()=>{if(seated){if(canLeave)p.onUiChange({...p.ui,confirm_leave:true});}else if(!p.authority.pending)send({type:'return_lobby'});}}><span aria-hidden="true">←</span><span>返回大厅</span></button><div><p className="pk-eyebrow">CHALDEA · POKER</p><h1>{t.name}</h1><small>{stateLabel(t.lifecycle_state)}</small></div></div><div className="pk-header-right"><strong>{chips(t.small_blind_units)} / {chips(t.big_blind_units)} <span>Chips</span></strong><small>无 Ante · {n} 人桌</small><span className="pk-connection">{connectionLabel[p.authority.connection_state]}{live&&readOnly?" · 只读":""}</span></div></header>
    {statusText&&<div className="pk-salon-status" role="status"><span>{statusText}</span><button onClick={()=>setPanel('status')}>查看状态</button>{p.authority.can_reconnect&&<button onClick={()=>{if(p.authority.can_reconnect)send({type:'reconnect'});}}>重新连接</button>}</div>}
    {p.ui.confirm_takeover&&<Modal title="接管确认" busy={recovery?.phase==='SENT'} onClose={()=>p.onUiChange({...p.ui,confirm_takeover:false})}><section className="pk-confirm" aria-label="接管确认"><div><h2>在这台设备接管牌桌？</h2><p>原设备将转为只读。此处只发起接管，控制权以服务端确认结果为准。</p></div><button className="pk-primary" disabled={!canTakeover} onClick={()=>{if(canTakeover)send({type:'takeover'});}}>确认接管牌桌</button><button disabled={recovery?.phase==='SENT'} onClick={()=>{if(recovery?.phase!=='SENT')p.onUiChange({...p.ui,confirm_takeover:false});}}>取消接管</button></section></Modal>}
    {p.ui.confirm_leave&&<Modal title="安全离座确认" busy={p.authority.pending} onClose={()=>p.onUiChange({...p.ui,confirm_leave:false})}><section className="pk-confirm" aria-label="安全离座确认"><div><h2>安全离座，而不是中断牌局</h2><p>正在参与或已弃牌的本手仍须正常结算；服务端完成 Cash Out 后再返回大厅。这里不清空牌局，也不预测到账。</p></div><button className="pk-primary" disabled={!canLeave} onClick={()=>{if(canLeave)send({type:'leave',return_to_lobby:true});}}>确认安全离座</button><button disabled={p.authority.pending} onClick={()=>{if(!p.authority.pending)p.onUiChange({...p.ui,confirm_leave:false});}}>留在牌桌</button></section></Modal>}
    <div className="pk-table-workspace"><div className="pk-table-main">
    <div className="pk-table-stage" data-seats={n}>
      <PokerArt name="table" className="pk-oval-art"/>
      <ol className="pk-seats" aria-label="牌桌座位">{Array.from({length:n},(_,i)=>{
        const seatNo=i+1,s=t.seats.find(x=>x.seat_no===seatNo),index=(i-rotation+n)%n,xy=seatPositions[index];
        const publicCards=s&&!s.is_folded&&s.hole_cards_released===true&&Array.isArray(s.public_hole_cards)&&s.public_hole_cards.length===2&&s.public_hole_cards.every(c=>Number.isInteger(c)&&c>=0&&c<=51)&&s.public_hole_cards[0]!==s.public_hole_cards[1]?s.public_hole_cards:undefined;
        return <li key={seatNo} aria-label={`${seatNo} 号座位`} className={`pk-seat${s?.is_self?' pk-seat-self':''}${h?.actor_seat===seatNo?' pk-seat-actor':''}${s?.is_folded?' pk-seat-folded':''}${s?'':' pk-seat-empty'}`} style={{'--seat-x':`${xy[0]}%`,'--seat-y':`${xy[1]}%`} as CSSProperties}>
          {s?<><PokerArt name="crest" className="pk-seat-crest" sizes="90px"/><div className="pk-seat-name"><span className="pk-seat-number">{seatNo.toString().padStart(2,'0')}</span><strong title={s.display_name}>{s.display_name}</strong>{h?.button_seat===seatNo&&<span className="pk-dealer" aria-label="庄位">D</span>}</div><strong className="pk-stack">{chips(s.stack_units)} <span>Chips</span></strong><div className="pk-seat-status">{s.leave_after_hand?'本手后离座':s.sit_out_next_hand?'下手暂离':!s.connected?'已断线':s.is_all_in?'ALL-IN':s.is_folded?'已弃牌':stateLabel(s.state)}</div><small>本轮 {chips(s.street_committed_units)}</small>
            {!s.is_self&&<div className={`pk-seat-cards${publicCards?' pk-public-cards':''}`}>{publicCards?publicCards.map((c,j)=><Card key={j} card={c}/>):Array.from({length:Math.min(2,Math.max(0,Number.isInteger(s.hole_card_count)?s.hole_card_count:0))},(_,j)=><Card key={j} back/>)}</div>}
            {h?.actor_seat===seatNo&&<span className="pk-turn-tag">当前行动</span>}
          </>:<><span className="pk-seat-number">{seatNo.toString().padStart(2,'0')}</span><span>空座</span></>}
        </li>;
      })}</ol>
      <div className="pk-board-center"><div className="pk-street"><span>{h?stateLabel(h.street):'等待下一手'}</span></div><div className="pk-board-cards" aria-label="公共牌">{Array.from({length:5},(_,i)=><Card key={i} card={h?.board_cards[i]}/>)}</div><div className="pk-pot-total"><span>当前总底池</span><strong>{chips(h?.pot_units)} <small>Chips</small></strong></div><div className="pk-pots" title="完整底池分配可在牌局详情查看">{h?.pots.map(pot=><section key={pot.index} aria-label={pot.index===0?'主池':`边池 ${pot.index}`} className="pk-pot"><span>{pot.index===0?'主池':`边池 ${pot.index}`}</span><strong>{chips(pot.amount_units)}</strong></section>)}</div>{!h&&<p className="pk-wait-copy">入桌等待大盲 · 不在本地开局</p>}</div>
    </div>
    <footer className="pk-action-tray" aria-label="当前行动区"><div className="pk-own-hand"><div><span className="pk-eyebrow">{seated?'YOUR HAND':p.authority.viewer_kind==='HOST'?'HOST':'SPECTATOR'}</span><strong>{seated?'你的手牌':p.authority.viewer_kind==='HOST'?'房主管理':'只读观战'}</strong><small>{seated?`桌上 ${chips(self?.stack_units)} Chips`:'不持有行动控制权'}</small></div><div className="pk-own-cards">{ownCards?.map((c,i)=><Card key={i} card={c}/>)}{!ownCards?.length&&<span className="pk-no-cards">{self?.state==='WAIT_FOR_BB'?'等待大盲':seated?'尚无可见私牌':'公开牌桌视图'}</span>}</div></div>
      <div className="pk-action-content"><div className="pk-action-heading"><strong>{readOnly?'当前连接只读':h?.actor_seat===v.seat_no&&seated?'轮到你行动':h?.actor_seat?`等待 ${h.actor_seat} 号座位`:'等待服务端状态'}</strong><span className="pk-timer" aria-label="服务端行动计时">{timerText}</span>{h?.action_deadline_at&&<time className="pk-sr-only" dateTime={h.action_deadline_at}>行动截止 {h.action_deadline_at}</time>}</div>
        {showAmount&&<div className="pk-bet-editor"><label htmlFor={`${id}-bet`}>本轮下注总额<input id={`${id}-bet`} value={p.ui.bet.amount_chips} inputMode="numeric" pattern="[0-9]*" disabled={!act||!legal?.raise_rights} onChange={e=>{if(act&&legal?.raise_rights)p.onUiChange({...p.ui,bet:{action_type:legal.actions.includes('RAISE')?'RAISE':'BET',amount_chips:e.target.value}});}}/></label><div className="pk-bet-side"><div className="pk-quick-row" onFocus={e=>{if(e.target instanceof HTMLButtonElement&&e.target.parentElement===e.currentTarget)e.target.scrollIntoView({block:'nearest',inline:'nearest'});}}>{!legal?.shortcuts.some(shortcut=>shortcut.name==='Min')&&<button disabled={!act||!legal?.raise_rights||min===undefined} onClick={()=>{if(act&&legal?.raise_rights&&min!==undefined)p.onUiChange({...p.ui,bet:{action_type:legal.actions.includes('RAISE')?'RAISE':'BET',amount_chips:chipText(min.toString())}});}}>Min</button>}{legal?.shortcuts.map((shortcut,index)=><button key={`${shortcut.name}-${index}`} disabled={!act||!legal.actions.includes(shortcut.action_type)||!legal.raise_rights||pokerUnits(shortcut.target_to_units)===undefined} onClick={()=>{if(act&&legal.actions.includes(shortcut.action_type)&&legal.raise_rights&&pokerUnits(shortcut.target_to_units)!==undefined)p.onUiChange({...p.ui,bet:{action_type:shortcut.action_type,amount_chips:chipText(shortcut.target_to_units)}});}}>{shortcut.name}</button>)}</div><small>{selected==='ALL_IN'?`将投入剩余 ${chips(self?.stack_units)} Chips`:amount!==undefined&&committed!==undefined&&amount>=committed?`本轮总额 ${chips(amount.toString())} · 本次追加 ${chips((amount-committed).toString())} Chips`:'请输入合法的整筹码总额'} · 最低 {chips(min?.toString())} / 最高 {chips(max?.toString())}</small></div></div>}
        {showAmount&&<input className="pk-bet-slider" type="range" aria-label="调整本轮下注总额" aria-valuetext={`${targetDisplay} Chips`} min="0" max="1000" step="1" value={sliderPosition} disabled={!sliderEnabled} onChange={e=>{if(sliderEnabled&&min!==undefined&&max!==undefined&&legal){const position=BigInt(e.target.value);if(position>=0n&&position<=1000n){const target=min+((max-min)*position/1000n/UNITS_PER_CHIP)*UNITS_PER_CHIP;p.onUiChange({...p.ui,bet:{action_type:legal.actions.includes('RAISE')?'RAISE':'BET',amount_chips:chipText(target.toString())}});}}}}/>}
        {legal?.actions.includes('CALL')&&<small className="pk-call-detail">尚需跟注 {chips(legal.to_call_units)} · 本次跟注追加 {chips(legal.call_applied_units)} Chips</small>}
        <div className="pk-action-buttons">{live&&readOnly&&p.authority.can_takeover&&<button className="pk-primary" disabled={!canTakeover} onClick={()=>{if(canTakeover)p.onUiChange({...p.ui,confirm_takeover:true});}}>申请接管</button>}{canRequestConnection&&<button className="pk-primary" disabled={!recoveryAllowed} onClick={()=>{if(canRequestConnection&&recoveryAllowed)send({type:'reconnect',control_intent:'CLAIM_CONTROL'});}}>申请控制连接</button>}{legal?.actions.includes('FOLD')&&<button className="pk-fold" disabled={!can('FOLD')} onClick={()=>{if(can('FOLD'))send({type:'action',action_type:'FOLD',target_to_units:'0'});}}>弃牌</button>}{legal?.actions.includes('CHECK')&&<button disabled={!can('CHECK')} onClick={()=>{if(can('CHECK'))send({type:'action',action_type:'CHECK',target_to_units:'0'});}}>过牌</button>}{legal?.actions.includes('CALL')&&<button disabled={!can('CALL')||pokerUnits(legal.call_applied_units)===undefined} onClick={()=>{if(can('CALL')&&pokerUnits(legal.call_applied_units)!==undefined)send({type:'action',action_type:'CALL',target_to_units:'0'});}}>跟注 {chips(legal.call_applied_units)} Chips</button>}
          {showAmount&&<button className="pk-primary" disabled={!amountValid} onClick={()=>{if(amountValid)send({type:'action',action_type:selected,target_to_units:selected==='ALL_IN'?'0':amount!.toString()});}}>{selected==='ALL_IN'?`确认全下 ${targetDisplay} Chips`:`确认${selected==='BET'?'下注':'加注'}至 ${targetDisplay} Chips`}</button>}
          {legal?.actions.includes('ALL_IN')&&(!showAmount||selected!=='ALL_IN')&&<button className="pk-commit" disabled={!can('ALL_IN')} onClick={()=>{if(can('ALL_IN'))send({type:'action',action_type:'ALL_IN',target_to_units:'0'});}}>全下 {chips(self?.stack_units)} Chips</button>}
          {!legal?.actions.length&&<p className="pk-no-action">当前没有可提交的行动；等待权威状态。</p>}
        </div>
      </div>
    </footer>
    </div>{panel!=='activity'&&<aside className="pk-salon-sidebar" aria-label="牌桌实时动态">{activityPanel()}</aside>}</div>
    <footer className="pk-utility-bar" aria-label="牌桌工具">
      <div><button onClick={()=>setPanel('rules')}>游戏规则</button><button onClick={()=>setPanel('fairness')}>公平验证</button><button onClick={()=>setPanel('history')}>牌局详情</button><button className="pk-mobile-activity" onClick={()=>setPanel('activity')}>记录与聊天</button>{t.host&&<button onClick={()=>setPanel('host')}>房主管理</button>}</div>
      <div><button onClick={()=>setPanel('session')}>补充筹码</button><div className="pk-session-actions">
      <button disabled={!controlReady||!seated||!v.can_sit_out} onClick={()=>{if(controlReady&&seated&&v.can_sit_out)send({type:'sitout'});}}>下手暂离</button><button disabled={!controlReady||!seated||!v.can_resume} onClick={()=>{if(controlReady&&seated&&v.can_resume)send({type:'resume'});}}>恢复参与 · 等待大盲</button><button disabled={!canLeave} onClick={()=>{if(canLeave)p.onUiChange({...p.ui,confirm_leave:true});}}>安全离座</button>
    </div></div>
    </footer>
    {panel&&!p.ui.confirm_leave&&!p.ui.confirm_takeover&&<Modal title={modalTitle[panel]} onClose={()=>setPanel(null)}>
      {panel==='rules'&&<div className="pk-modal-copy"><p>无限注德州扑克 · 无 Ante · 新入桌等待大盲。</p><p>首手庄位公平随机，后续依规则轮转。标准高牌型比较，A2345 为五高，花色不破同分。</p><p>下注与加注输入本轮总额，不是额外追加数；快捷金额由服务端按跟注后底池、整 Chip 向下取整提供。只显示当前合法行动。</p><p>暂离与安全离座由服务端执行；正在参与或已弃牌的本手先完成结算，随后才完成出金。</p></div>}
      {panel==='fairness'&&<div className="pk-modal-copy"><p>仅展示本手公开承诺；未公开种子、牌序与他人私牌始终隐藏。</p><dl><dt>Server seed hash</dt><dd>{h?.server_seed_hash??'等待本手承诺'}</dd><dt>Deck hash</dt><dd>{h?.deck_hash??'等待本手承诺'}</dd></dl><p>结算后的公开记录与检验入口请在牌局历史中查看。本页不在客户端预测胜负。</p><a href="/history" target="_blank" rel="noopener noreferrer">打开游戏记录</a></div>}
      {panel==='history'&&<div className="pk-modal-copy"><dl><dt>牌桌</dt><dd>{t.table_id} · 版本 {t.table_version}</dd><dt>本手</dt><dd>{h?.hand_id??'尚未开始'} · 版本 {h?.hand_version??'—'}</dd></dl>{h?.pots.map(pot=><section key={pot.index}><h3>{pot.index===0?'主池':`边池 ${pot.index}`} · {chips(pot.amount_units)} Chips</h3><p>资格席位 {pot.eligible_seats.join(' / ')}</p>{pot.awards.length>0&&<p>服务端结算：{pot.awards.map(a=>`${a.seat_no} 号 +${chips(a.amount_units)}`).join('；')}</p>}</section>)}<a href="/history" target="_blank" rel="noopener noreferrer">打开牌局历史（新页面）</a></div>}
      {panel==='session'&&<section className="pk-table-tools" aria-label="会话管理"><div><span className="pk-caption">当前会话</span><strong>{seated?`座位 ${v.seat_no} · ${v.session_id}`:p.authority.viewer_kind==='HOST'?'房主只读连接':'只读观战'}</strong>{self?.leave_after_hand&&<p className="pk-notice">已请求本手结束后离座，等待服务端结算。</p>}{self?.pending_top_up_units&&self.pending_top_up_units!=='0'&&<p>待补充 {chips(self.pending_top_up_units)} Chips · 手间重新核验</p>}{self?.rebuy_deadline_at&&<small>补充筹码保座截止 <time dateTime={self.rebuy_deadline_at}>{self.rebuy_deadline_at}</time></small>}</div><section className="pk-topup"><h3>补充筹码 · 仅手间生效</h3><form onSubmit={e=>{e.preventDefault();if(topValid&&topup!==undefined)send({type:'topup',amount_units:topup.toString()});}}><label htmlFor={`${id}-topup`}>补充筹码数量<input id={`${id}-topup`} value={p.ui.top_up_chips} inputMode="numeric" pattern="[0-9]*" disabled={!canTopUp} onChange={e=>{if(canTopUp)p.onUiChange({...p.ui,top_up_chips:e.target.value});}}/></label><p className="pk-footnote">权威允许范围 {chips(v.top_up_min_units)}–{chips(v.top_up_max_units)} Chips。手间重新校验钱包，不自动 Rebuy。</p><button disabled={!topValid} type="submit">提交补充筹码</button></form></section></section>}
      {panel==='activity'&&<div className="pk-modal-activity">{activityPanel()}</div>}
      {panel==='host'&&t.host&&<PokerHostControls host={t.host} context={context} mutation_blocked={p.mutation_blocked} operation={p.host_operation} onCommand={p.onHostCommand} onRetry={p.onRetryHost}/>}
      {panel==='status'&&<div className="pk-modal-copy"><p>{statusText}</p><p>系统会自动读取原请求的结果与最新牌局。结果未确认前保持操作锁定，不自动重复提交。</p>{p.notice&&p.notice!==statusText&&<p>{p.notice}</p>}{history&&<p>还有 {history.count} 次旧连接接管结果自动确认中。</p>}
        {p.authority.pending&&p.onRetryPending&&p.authority.can_retry_pending&&<button disabled={p.mutation_blocked||p.receipt_querying} onClick={()=>{if(!p.mutation_blocked&&p.authority.can_retry_pending&&!p.receipt_querying)p.onRetryPending?.(context);}}>重试原操作</button>}
        {recovery?.phase==='UNKNOWN'&&recovery.can_retry&&p.onRetryTakeover&&<button disabled={p.mutation_blocked||recovery.querying} onClick={()=>{if(!p.mutation_blocked&&recovery.can_retry&&!recovery.querying)p.onRetryTakeover?.(context);}}>重试本次接管</button>}
        {!live&&p.authority.can_takeover&&<button disabled={!canTakeover} onClick={()=>{if(canTakeover)p.onUiChange({...p.ui,confirm_takeover:true});}}>申请接管</button>}
      </div>}
    </Modal>}
  </section>;
}
