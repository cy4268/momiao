import { chips, stateLabel, type PokerTimelineEvent, type PokerTimelineView } from './poker-ui-types';

const labels:Record<PokerTimelineEvent['type'],string>={
  HAND_COMMITTED:'本手已承诺',POST_SB:'投入小盲',POST_BB:'投入大盲',FOLD:'弃牌',CHECK:'过牌',CALL:'跟注',BET:'下注',RAISE:'加注',ALL_IN:'全下',
  AUTO_FOLD:'超时自动弃牌',AUTO_CHECK:'超时自动过牌',DISCONNECTED:'连接中断',RECONNECTED:'重新连接',RECOVERING:'服务恢复',RESUMED:'恢复牌局',
  ACTOR_CHANGED:'行动位变更',ALL_IN_RUNOUT:'全下发完公共牌',DEAL_FLOP:'发出翻牌',DEAL_TURN:'发出转牌',DEAL_RIVER:'发出河牌',
  RETURN_UNCALLED:'退回无人跟注',SHOWDOWN:'摊牌',POT_AWARD:'底池结算',SYSTEM_SETTLEMENT:'系统结算',
};

function eventDetail(event:PokerTimelineEvent):string {
  const parts:string[]=[];
  if(event.seat_no!==undefined)parts.push(`${event.seat_no} 号座位`);
  if(event.delta_units!==undefined)parts.push(`${chips(event.delta_units)} Chips`);
  if(event.to_units!==undefined)parts.push(`本轮至 ${chips(event.to_units)} Chips`);
  return parts.join(' · ');
}

/** Public, server-ordered hand events only. Cards, seeds and future deck data have no rendering slot. */
export function PokerActionTimeline({timeline}:{timeline:PokerTimelineView}) {
  return <section className="pk-live-panel pk-timeline" aria-labelledby="pk-timeline-title">
    <header><div><span className="pk-eyebrow">LIVE TIMELINE</span><h2 id="pk-timeline-title">实时行动记录</h2></div><small>序列 {timeline.last_sequence}</small></header>
    {timeline.truncated&&<p className="pk-panel-notice">仅显示本手最近 128 条公开事件。</p>}
    {timeline.events.length===0?<p className="pk-panel-empty">等待本手公开行动。</p>:
      <ol>{timeline.events.map(event=><li key={event.sequence}>
        <span className="pk-timeline-dot" aria-hidden="true"/>
        <div><strong>{labels[event.type]}</strong><small>{eventDetail(event)||stateLabel(event.street)}</small></div>
        <time dateTime={event.occurred_at}>{new Date(event.occurred_at).toLocaleTimeString('zh-CN',{hour:'2-digit',minute:'2-digit',second:'2-digit',hour12:false})}</time>
      </li>)}</ol>}
  </section>;
}
