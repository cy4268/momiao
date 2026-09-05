import { useEffect, useRef, useState, type FormEvent } from 'react';
import { ApiClient, ApiError } from './api';
import { Alert, Empty, Loading, useResource } from './ui';
import { assetNames, assets, type Asset, type WalletData } from './wallet-api';
import { amountUnits, matchesOperation, operationError, parsePending, parseTransaction, readDaily, readTransactions, type PendingOperation, type Transaction } from './economy-api';

export function WalletActions({client,userID,wallet,onChange}:{client:ApiClient;userID:string;wallet:WalletData;onChange:(t:Transaction)=>void}) {
 const storageKey=`momiao.wallet.pending.${userID}`;
 const [pending,setPending]=useState<PendingOperation|null>(null);
 const [storageError,setStorageError]=useState(false);
 const [busy,setBusy]=useState(false);const [retry,setRetry]=useState(false);const [notice,setNotice]=useState('');
 const [from,setFrom]=useState<Asset>('RESERVE_API_CREDIT');const [amount,setAmount]=useState('');
 const lock=useRef(false);const active=useRef(false);const generation=useRef(client.getSessionGeneration());
 useEffect(()=>{active.current=true;try{setPending(parsePending(sessionStorage.getItem(storageKey)))}catch{setStorageError(true)}return()=>{active.current=false}},[storageKey]);
 const current=()=>active.current && client.getSessionGeneration()===generation.current && String(client.getSnapshot().user?.id)===userID && !client.getSnapshot().loggingOut;
 const daily=useResource(()=>readDaily(client,userID),[client,userID]);
 const blocked=busy || pending!==null || storageError;
 const to:Asset=from==='RESERVE_API_CREDIT'?'AVAILABLE_CHIPS':'RESERVE_API_CREDIT';
 const units=amountUnits(amount);const balance=wallet.wallets.find(w=>w.asset===from)!;
 function finish(t:Transaction,p:PendingOperation){if(!matchesOperation(t,p))throw new Error('Receipt mismatch');sessionStorage.removeItem(storageKey);setPending(null);setRetry(false);onChange(t)}
 async function send(p:PendingOperation,isRetry=false){
  if(lock.current || storageError || (!isRetry && pending))return;
  lock.current=true;setBusy(true);setNotice('');
  try {
   // Persist the non-secret original request before sending; a reload never invents a retry key.
   sessionStorage.setItem(storageKey,JSON.stringify(p));setPending(p);setRetry(false);
   const result=await client.request(p.kind==='DAILY'?'/platform/v1/rewards/daily/claim':'/platform/v1/wallet/exchange','POST',{idempotency_key:p.key,...(p.kind==='EXCHANGE'?{from_asset:p.from_asset,amount:p.amount}:{})});
   if(current())finish(parseTransaction(result,userID),p);
  }catch(e){if(current()){
   if(e instanceof ApiError && e.status>=400 && e.status<500 && e.code && e.code!=='IDEMPOTENCY_CONFLICT') {sessionStorage.removeItem(storageKey);setPending(null)}
   setNotice(operationError(e));
  }}finally{lock.current=false;if(current())setBusy(false)}
 }
 async function reconcile(){
  if(!pending || lock.current)return;const p=pending;lock.current=true;setBusy(true);setNotice('');setRetry(false);
  try{const value=await client.request(`/platform/v1/transactions/by-key?kind=${p.kind}&key=${p.key}`);if(current()){if(value===null){setRetry(true);setNotice('尚未查到已确认交易。可按原请求重试，不会更换请求编号。')}else{finish(parseTransaction(value,userID),p)}}}
  catch(e){if(current())setNotice(operationError(e))}finally{lock.current=false;if(current())setBusy(false)}
 }
 function exchange(e:FormEvent){e.preventDefault();if(blocked || units===null || units>BigInt(balance.balance_units))return;void send({kind:'EXCHANGE',key:crypto.randomUUID(),from_asset:from,amount})}
 return <>
  {notice && <Alert>{notice}</Alert>}
  {storageError && <Alert>浏览器未能读取待核对请求。请检查会话存储；为避免重复交易，资产操作已暂停。</Alert>}
  {pending && <section className="panel pending-operation" aria-label="待核对交易"><h2>先核对上一笔操作</h2><p>{pending.kind==='DAILY'?'每日签到':`兑换 ${pending.amount} ${assetNames[pending.from_asset!]}`} · 本页保留原请求，核对前暂停新的资产操作。</p><code>{pending.key}</code><div className="actions"><button disabled={busy} onClick={()=>void reconcile()}>核对交易结果</button>{retry && <button disabled={busy} onClick={()=>void send(pending,true)}>按原请求重试</button>}</div></section>}
  <div className="wallet-actions-grid">
   <section className="panel reward-card" aria-labelledby="daily-title"><p className="eyebrow">DAILY / RESERVE SUPPLY</p><h2 id="daily-title">每日签到</h2><p className="wallet-amount">500 <small>API Credit</small></p><p>固定进入 Reserve，按上海自然日每天一次。</p>{daily.loading?<Loading/>:daily.error?<><Alert>签到状态读取未完成。</Alert><button onClick={daily.reload}>刷新签到状态</button></>:daily.data && <><p className="hint">{daily.data.business_date} · {daily.data.claimed?'今日已领取':'今日待领取'}<br/>下次刷新：{new Date(daily.data.next_reset_at).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai',hour12:false})}（上海时间）</p><button className="primary" disabled={blocked || daily.data.claimed} onClick={()=>void send({kind:'DAILY',key:crypto.randomUUID()})}>{daily.data.claimed?'今日已领取':'领取今日 500 额度'}</button></>}<p className="hint">初始赠金、小时奖励与救济尚未开放。</p></section>
   <section className="panel" aria-labelledby="exchange-title"><p className="eyebrow">EXCHANGE / LOCAL WALLETS</p><h2 id="exchange-title">本地资产兑换</h2><p>1:1 双向兑换，无手续费；不动原生 Active 额度。</p><form className="exchange-form" onSubmit={exchange}><label>兑换方向<select disabled={blocked} value={from} onChange={e=>setFrom(e.target.value as Asset)}>{assets.map(a=><option value={a} key={a}>{assetNames[a]} → {assetNames[a==='RESERVE_API_CREDIT'?'AVAILABLE_CHIPS':'RESERVE_API_CREDIT']}</option>)}</select></label><label>兑换数量<input inputMode="decimal" maxLength={30} value={amount} disabled={blocked} onChange={e=>setAmount(e.target.value)} placeholder="例如 100"/></label><p className="hint">可用 {balance.amount} {assetNames[from]}；最小步长 0.000002。</p>{amount && <p aria-live="polite">{units===null?'请输入可精确表示的正数。':units>BigInt(balance.balance_units)?'来源余额不足。':`将扣除 ${amount} ${assetNames[from]}，获得 ${amount} ${assetNames[to]}。`}</p>}<button className="primary" disabled={blocked || units===null || units>BigInt(balance.balance_units)}>确认兑换</button></form></section>
  </div>
  <TransactionHistory client={client} userID={userID}/>
 </>;
}
function TransactionHistory({client,userID}:{client:ApiClient;userID:string}) {
 const [cursors,setCursors]=useState(['']);const after=cursors[cursors.length-1];
 const r=useResource(()=>readTransactions(client,userID,after),[client,userID,after]);
 return <section className="panel"><div className="section-heading"><div><p className="eyebrow">CONFIRMED / TRANSACTIONS</p><h2>资产交易记录</h2></div><button disabled={r.loading} onClick={r.reload}>刷新交易</button></div>{r.loading?<Loading/>:r.error?<Alert>{r.error}</Alert>:r.data && (r.data.items.length===0?<Empty title="暂无已确认交易">签到和兑换完成后，这里显示真实交易回执。</Empty>:<div className="table-wrap" role="region" aria-label="资产交易记录" tabIndex={0}><table><thead><tr><th>时间 / 交易编号</th><th>业务</th><th>数量 / 去向</th><th>状态</th></tr></thead><tbody>{r.data.items.map(t=><tr key={t.id}><td><time dateTime={t.created_at}>{new Date(t.created_at).toLocaleString('zh-CN',{hour12:false})}</time><small className="transaction-id">{t.id}</small></td><td>{t.kind==='DAILY_REWARD'?'每日签到':'本地兑换'}</td><td>{t.amount}<small>{t.from_asset?`${assetNames[t.from_asset]} → `:''}{assetNames[t.to_asset]}</small></td><td>已确认</td></tr>)}</tbody></table></div>)}<nav className="pager" aria-label="资产交易分页"><span>每页 20 条，最新交易在前</span><div><button disabled={r.loading || cursors.length===1} onClick={()=>setCursors(c=>c.slice(0,-1))}>上一页交易</button><button disabled={r.loading || !r.data?.has_more || !!r.error} onClick={()=>{if(r.data?.next_after_id)setCursors(c=>[...c,r.data!.next_after_id!])}}>下一页交易</button></div></nav></section>;
}
