import type { FormEvent } from 'react';
import type { PokerAuthority, PokerChatView, PokerIntentContext } from './poker-ui-types';

interface PokerChatProps {
  chat:PokerChatView;authority:PokerAuthority;context:PokerIntentContext;draft:string;mutation_blocked?:boolean;
  onDraftChange?:(next:string)=>void;onSend?:(message:string,context:PokerIntentContext)=>void;
  onQueryReceipt?:()=>void;onRetry?:(context:PokerIntentContext)=>void;
}

function validMessage(value:string):boolean {
  const body=value.trim();
  return body.length>0&&new TextEncoder().encode(body).length<=2048&&!/[\p{Cc}\p{Cs}]/u.test(body.replace(/[\n\t]/g,''));
}

export function PokerChat(p:PokerChatProps) {
  const recovery=p.authority.chat_recovery,connected=p.authority.connection_state==='LIVE';
  const canSend=connected&&!p.mutation_blocked&&!recovery&&p.chat.can_send&&!p.chat.muted&&!!p.onSend&&validMessage(p.draft);
  const submit=(event:FormEvent)=>{event.preventDefault();if(canSend)p.onSend?.(p.draft.trim(),p.context);};
  return <section className="pk-live-panel pk-chat" aria-labelledby="pk-chat-title">
    <header><div><span className="pk-eyebrow">TABLE CHAT</span><h2 id="pk-chat-title">牌桌聊天</h2></div><small>{p.chat.messages.length} / 100</small></header>
    <div className="pk-chat-log" role="log" aria-live="polite" aria-relevant="additions text">
      {p.chat.truncated&&<p className="pk-panel-empty">较早消息已从当前实时快照省略。</p>}
      {p.chat.messages.length===0?<p className="pk-panel-empty">还没有聊天消息。</p>:p.chat.messages.map(message=><article key={message.message_id} className={message.kind==='SYSTEM'?'pk-chat-system':''}>
        <div className="pk-chat-avatar" aria-hidden="true">{message.author?.display_name.trim().slice(0,1)||'·'}</div>
        <div><div className="pk-chat-meta"><strong>{message.author?.display_name??'牌桌系统'}</strong><time dateTime={message.created_at}>{new Date(message.created_at).toLocaleTimeString('zh-CN',{hour:'2-digit',minute:'2-digit',hour12:false})}</time></div><p>{message.body}</p></div>
      </article>)}
    </div>
    {recovery&&<div className="pk-chat-recovery" role="status"><p>{recovery.phase==='ACKNOWLEDGED'?'消息已受理，等待权威快照。':'消息结果尚未确认；先核对原回执。'}</p><div>
      {recovery.phase!=='ACKNOWLEDGED'&&<button disabled={recovery.querying} aria-busy={recovery.querying} onClick={()=>{if(!recovery.querying)p.onQueryReceipt?.();}}>核对消息回执</button>}
      {recovery.phase==='UNKNOWN'&&<button disabled={p.mutation_blocked||recovery.querying||!recovery.can_retry} onClick={()=>{if(!p.mutation_blocked&&!recovery.querying&&recovery.can_retry)p.onRetry?.(p.context);}}>重试原消息</button>}
    </div></div>}
    <form onSubmit={submit}><label htmlFor="pk-chat-draft" className="pk-sr-only">聊天消息</label><textarea id="pk-chat-draft" value={p.draft} maxLength={2048} rows={2} placeholder={p.chat.muted?'你已被本桌静音':connected?'输入纯文本消息':'等待实时连接'} disabled={!connected||p.chat.muted||!!recovery||!p.onDraftChange} onChange={event=>p.onDraftChange?.(event.target.value)}/><button className="pk-primary" type="submit" disabled={!canSend}>发送</button></form>
    <small>纯文本 · 每 10 秒最多 5 条，每分钟最多 30 条</small>
  </section>;
}
