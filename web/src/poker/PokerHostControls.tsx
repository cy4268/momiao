import { useState } from 'react';
import type { PokerHostCommand, PokerHostOperationView, PokerHostView, PokerIntentContext } from './poker-ui-types';

interface PokerHostControlsProps {
  host:PokerHostView;context:PokerIntentContext;mutation_blocked?:boolean;operation?:PokerHostOperationView;
  onCommand?:(command:PokerHostCommand,context:PokerIntentContext)=>void;onQueryReceipt?:()=>void;onRetry?:(context:PokerIntentContext)=>void;
}
type Confirmation={command:PokerHostCommand;label:string;detail:string};
const has=(host:PokerHostView,command:PokerHostCommand['command'])=>host.capabilities.includes(command);

export function PokerHostControls(p:PokerHostControlsProps) {
  const [confirmation,setConfirmation]=useState<Confirmation>();
  const busy=!!p.operation,enabled=!p.mutation_blocked&&!busy&&!!p.onCommand;
  const prepare=(next:Confirmation)=>{if(enabled&&has(p.host,next.command.command))setConfirmation(next);};
  const submit=()=>{if(!confirmation||!enabled||!has(p.host,confirmation.command.command))return;p.onCommand?.(confirmation.command,p.context);setConfirmation(undefined);};
  const knownUsers=new Set([...p.host.players.map(x=>x.target_user_id),...p.host.spectators.map(x=>x.target_user_id)]);
  return <section className="pk-live-panel pk-host" aria-labelledby="pk-host-title">
    <header><div><span className="pk-eyebrow">HOST WORKSPACE</span><h2 id="pk-host-title">房主管理</h2></div><small>命令以服务端回执为准</small></header>
    {p.operation&&<div className="pk-host-operation" role="status"><p><strong>{p.operation.phase==='ACKNOWLEDGED'?'命令已受理':'命令结果尚未确认'}</strong>{p.operation.status&&<span> · {p.operation.status}</span>}</p><details><summary>命令状态详情</summary>
      {p.operation.phase==='UNKNOWN'&&<button disabled={p.mutation_blocked||p.operation.querying||!p.operation.can_retry} onClick={()=>{if(!p.mutation_blocked&&!p.operation?.querying&&p.operation?.can_retry)p.onRetry?.(p.context);}}>重试原命令</button>}
    </details></div>}
    <div className="pk-host-global">
      <button disabled={!enabled||!has(p.host,'PAUSE_ACCEPTING_PLAYERS')} onClick={()=>prepare({command:{command:'PAUSE_ACCEPTING_PLAYERS'},label:'暂停新玩家入桌',detail:'现有牌局继续按服务端规则进行。'})}>暂停入桌</button>
      <button disabled={!enabled||!has(p.host,'RESUME_ACCEPTING_PLAYERS')} onClick={()=>prepare({command:{command:'RESUME_ACCEPTING_PLAYERS'},label:'恢复新玩家入桌',detail:'只恢复入桌资格，不在本地启动新一手。'})}>恢复入桌</button>
      <button className="pk-commit" disabled={!enabled||!has(p.host,'CLOSE_TABLE')} onClick={()=>prepare({command:{command:'CLOSE_TABLE'},label:'安全关闭牌桌',detail:'本手先完成结算，所有座位随后安全出金。'})}>安全关闭牌桌</button>
    </div>
    <details><summary>玩家 <span>{p.host.players.length}</span></summary><ul className="pk-host-list">{p.host.players.length===0?<li className="pk-panel-empty">当前没有在桌玩家。</li>:p.host.players.map(player=><li key={player.target_session_id}><div><strong>{player.seat_no} 号 · {player.display_name}</strong><small>{player.removal_pending?'已安排本手后离桌':player.muted?'已静音':'在桌玩家'}</small></div><div><button disabled={!enabled||player.removal_pending||!has(p.host,'REMOVE_PLAYER_AFTER_HAND')} onClick={()=>prepare({command:{command:'REMOVE_PLAYER_AFTER_HAND',target_session_id:player.target_session_id,target_user_id:player.target_user_id},label:`移除 ${player.display_name}`,detail:'若正在参与本手，将在结算完成后安全离桌。'})}>本手后移除</button><button disabled={!enabled||player.muted||!has(p.host,'MUTE_CHAT_USER')} onClick={()=>prepare({command:{command:'MUTE_CHAT_USER',target_session_id:player.target_session_id,target_user_id:player.target_user_id},label:`静音 ${player.display_name}`,detail:'静音仅作用于当前牌桌，且不可由此面板撤销。'})}>静音</button></div></li>)}</ul></details>
    <details><summary>观战者 <span>{p.host.spectators.length}</span></summary>{p.host.spectators_truncated&&<p className="pk-panel-empty">观战者列表已按实时帧预算裁剪，请刷新后再处理未显示目标。</p>}<ul className="pk-host-list">{p.host.spectators.length===0?<li className="pk-panel-empty">当前没有已授权观战者。</li>:p.host.spectators.map(spectator=><li key={spectator.target_user_id}><div><strong>{spectator.display_name}</strong><small>{spectator.muted?'已静音':'实时观战'}</small></div><div><button disabled={!enabled||!has(p.host,'REMOVE_SPECTATOR')} onClick={()=>prepare({command:{command:'REMOVE_SPECTATOR',target_user_id:spectator.target_user_id},label:`移除观战者 ${spectator.display_name}`,detail:'服务端将撤销此牌桌的当前实时连接。'})}>移出观战</button><button disabled={!enabled||spectator.muted||!has(p.host,'MUTE_CHAT_USER')} onClick={()=>prepare({command:{command:'MUTE_CHAT_USER',target_user_id:spectator.target_user_id},label:`静音 ${spectator.display_name}`,detail:'静音仅作用于当前牌桌，且不可由此面板撤销。'})}>静音</button></div></li>)}</ul></details>
    {(p.host.chat_targets_truncated||p.host.chat_targets.some(target=>!knownUsers.has(target.target_user_id)))&&<details><summary>最近发言者</summary>{p.host.chat_targets_truncated&&<p className="pk-panel-empty">较早发言者已从当前实时快照省略。</p>}<ul className="pk-host-list">{p.host.chat_targets.filter(target=>!knownUsers.has(target.target_user_id)).map(target=><li key={target.member_id}><div><strong>{target.display_name}</strong><small>{target.muted?'已静音':'最近在本桌发言'}</small></div><button disabled={!enabled||target.muted||!has(p.host,'MUTE_CHAT_USER')} onClick={()=>prepare({command:{command:'MUTE_CHAT_USER',target_user_id:target.target_user_id},label:`静音 ${target.display_name}`,detail:'静音仅作用于当前牌桌，且不可由此面板撤销。'})}>静音</button></li>)}</ul></details>}
    {confirmation&&<section className="pk-host-confirm" aria-labelledby="pk-host-confirm-title"><div><strong id="pk-host-confirm-title">{confirmation.label}？</strong><p>{confirmation.detail}</p></div><button className="pk-primary" disabled={!enabled} onClick={submit}>确认提交</button><button autoFocus onClick={()=>setConfirmation(undefined)}>取消</button></section>}
  </section>;
}
