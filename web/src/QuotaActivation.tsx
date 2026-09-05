import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { ApiClient, ApiError } from './api';
import { amountUnits } from './economy-api';
import { readWallet } from './wallet-api';
import { maxNativeUnits, parseNativeQuota, parseTransfer, parseTransferPending, parseTransfers, transferError, transferStatus, type Transfer, type TransferPending } from './quota-api';
import { Alert, Empty, Loading, useResource } from './ui';

export function QuotaActivation({client,userID}:{client:ApiClient;userID:string}) {
 const storageKey=`momiao.quota.pending.${userID}`;
 const [pending,setPending]=useState<TransferPending|null>(null);const [storageError,setStorageError]=useState(false);
 const [receipt,setReceipt]=useState<Transfer|null>(null);const [amount,setAmount]=useState('');const [busy,setBusy]=useState(false);const [retry,setRetry]=useState(false);const [notice,setNotice]=useState('');
 const active=useRef(false);const lock=useRef(false);const generation=useRef(client.getSessionGeneration());
 useEffect(()=>{active.current=true;try{setPending(parseTransferPending(sessionStorage.getItem(storageKey)))}catch{setStorageError(true)}return()=>{active.current=false}},[storageKey]);
 const current=()=>active.current && client.getSessionGeneration()===generation.current && String(client.getSnapshot().user?.id)===userID && !client.getSnapshot().loggingOut;
 const r=useResource(async()=>{const [wallet,native,history]=await Promise.all([readWallet(client,userID),client.request('/platform/v1/native-quota'),client.request('/platform/v1/quota-transfers')]);return {wallet,native:parseNativeQuota(native,userID),history:parseTransfers(history,userID)}},[client,userID]);
 const unresolved=r.data?.history.find(t=>t.status==='PENDING'||t.status==='NEEDS_REVIEW');
 const tracking=receipt?.status==='PENDING'?receipt:unresolved?.status==='PENDING'?unresolved:null;
 // Only GETs are automatic. The durable server worker continues when this page closes.
 useEffect(()=>{
  if(!tracking)return;let live=true;let timer:ReturnType<typeof setTimeout>;
  async function poll(){try{
   const list=parseTransfers(await client.request('/platform/v1/quota-transfers'),userID);
   if(!live)return;const found=list.find(t=>t.id===tracking!.id);
   if(found && found.status!=='PENDING'){setReceipt(found);r.reload();return}
  }catch{/* Keep the original receipt visible and allow explicit refresh. */}
   if(live)timer=setTimeout(()=>void poll(),2000);
  }
  timer=setTimeout(()=>void poll(),2000);return()=>{live=false;clearTimeout(timer)};
 },[client,userID,tracking?.id]);
 const balance=r.data?.wallet.wallets.find(w=>w.asset==='RESERVE_API_CREDIT');
 const units=amountUnits(amount);
 const valid=units!==null && units<=maxNativeUnits && !!balance && units<=BigInt(balance.balance_units);
 const blocked=busy || pending!==null || storageError || r.loading || !!r.error || !r.data?.native.enabled || !!unresolved || receipt?.status==='PENDING' || receipt?.status==='NEEDS_REVIEW';
 function finish(value:unknown,p:TransferPending){
  const t=parseTransfer(value,userID);if(BigInt(t.amount_units)!==amountUnits(p.amount))throw new Error('Receipt amount mismatch');
  sessionStorage.removeItem(storageKey);setPending(null);setRetry(false);setReceipt(t);r.reload();
 }
 async function send(p:TransferPending,isRetry=false){
  if(lock.current || storageError || (!isRetry && blocked))return;lock.current=true;setBusy(true);setNotice('');
  try{sessionStorage.setItem(storageKey,JSON.stringify(p));setPending(p);setRetry(false);
   const value=await client.request('/platform/v1/quota-transfers','POST',{idempotency_key:p.key,amount:p.amount});if(current())finish(value,p);
  }catch(e){if(current()){
   // Only explicit pre-acceptance rejections release this key; transport/5xx ambiguity stays locked.
   if(e instanceof ApiError && e.status>=400 && e.status<500 && ['INSUFFICIENT_BALANCE','WALLET_NOT_INITIALIZED','TRANSFER_PENDING','INVALID_AMOUNT','INVALID_REQUEST','ORIGIN_REJECTED','INVALID_CONTENT_TYPE'].includes(e.code)){
    try{sessionStorage.removeItem(storageKey);setPending(null);r.reload()}catch{setStorageError(true)}
   }
   setNotice(transferError(e));
  }}finally{lock.current=false;if(current())setBusy(false)}
 }
 async function reconcile(){
  if(!pending || lock.current)return;const p=pending;lock.current=true;setBusy(true);setNotice('');setRetry(false);
  try{const v=await client.request(`/platform/v1/quota-transfers/by-key?key=${p.key}`);if(current()){
   if(v===null){setRetry(true);setNotice('尚未查到该请求。可以原编号重试，不会更换请求。')}else finish(v,p);
  }}catch(e){if(current())setNotice(transferError(e))}finally{lock.current=false;if(current())setBusy(false)}
 }
 function submit(e:FormEvent){e.preventDefault();if(!blocked && valid)void send({key:crypto.randomUUID(),amount})}
 return <div className="wallet-page"><header className="page-heading"><div><p className="eyebrow">RESERVE → ACTIVE / API CREDIT</p><h1>转入原生额度</h1><p>将储备接入 API 调用，而不是仅停留在钱包中。</p></div><Link className="text-link" to="/wallet">返回我的钱包 →</Link></header>
  <div className="wallet-scope"><p>Reserve 与原生额度 1:1 转入，无手续费；1 API Credit = 500,000 原生单位。</p><p>本页暂不支持转回 Reserve。受理后由后台继续处理，关闭页面不取消划转。</p></div>
  {notice && <Alert>{notice}</Alert>}{storageError && <Alert>浏览器待核对请求读取异常，新的划转已暂停。请保留原请求记录。</Alert>}
  {pending && <section className="panel pending-operation" aria-label="待核对划转"><h2>先核对上一笔划转</h2><p>转入 {pending.amount} API Credit</p><code>{pending.key}</code><div className="actions"><button disabled={busy} onClick={()=>void reconcile()}>核对划转结果</button>{retry && <button disabled={busy} onClick={()=>void send(pending,true)}>按原请求重试</button>}</div></section>}
  {receipt && <p className="notice" role="status">{transferStatus[receipt.status]} · {receipt.amount} API Credit<br/><span className="transaction-id">{receipt.id}</span></p>}
  <div className="section-heading"><h2>当前余额</h2><button disabled={r.loading || busy} onClick={r.reload}>刷新余额与记录</button></div>
  {r.loading?<Loading/>:r.error?<Alert>{r.error}</Alert>:r.data && <>
   {!r.data.wallet.initialized?<Empty title="先建立钱包">请返回钱包页初始化，领取的每日签到额度将进入 Reserve。</Empty>:<section className="wallet-balances" aria-label="划转两端余额"><article className="panel wallet-balance"><p className="eyebrow">FROM / RESERVE</p><h2>储备 API Credit</h2><p className="wallet-amount">{balance?.amount}</p><p className="hint">可转入的来源余额</p></article><article className="panel wallet-balance"><p className="eyebrow">TO / ACTIVE</p><h2>原生可用 API Credit</h2><p className="wallet-amount">{r.data.native.amount}</p><p className="hint">{r.data.native.raw_quota} 原生单位 · 随 API 使用变化</p></article></section>}
   {!r.data.native.enabled && <Alert>额度划转暂未启用；已有回执仍可查看。</Alert>}
  </>}
  <section className="panel"><form className="exchange-form" onSubmit={submit}><label>转入数量<input inputMode="decimal" maxLength={30} value={amount} disabled={blocked} onChange={e=>setAmount(e.target.value)} placeholder="例如 100"/></label><p className="hint">最小步长 0.000002 API Credit；只使用 Reserve，不自动兑换筹码。</p>{amount && <p aria-live="polite">{!valid?'请填写可精确表示、且不超过来源余额的数量。':`扣除 ${amount} Reserve API Credit，转入 ${amount} 原生可用 API Credit。`}</p>}<button className="primary" disabled={blocked || !valid}>确认转入原生额度</button>{(unresolved||tracking) && <p className="hint">上一笔划转尚在处理或核对中，新划转暂缓。后台会继续处理原请求。</p>}</form></section>
  <section className="panel"><p className="eyebrow">RECENT / TRANSFERS</p><h2>最近 20 笔划转</h2><p className="hint">已退回表示目标未入账、Reserve 已恢复；待人工核对时请保留交易编号。</p>{r.data && !r.loading && !r.error && (r.data.history.length===0?<Empty title="暂无划转">主动确认转入后，这里会显示进度与回执。</Empty>:<div className="table-wrap" role="region" aria-label="原生额度划转记录" tabIndex={0}><table><thead><tr><th>时间 / 编号</th><th>API Credit</th><th>状态</th></tr></thead><tbody>{r.data.history.map(t=><tr key={t.id}><td><time dateTime={t.created_at}>{new Date(t.created_at).toLocaleString('zh-CN',{hour12:false})}</time><small className="transaction-id">{t.id}</small></td><td className="numeric">{t.amount}</td><td>{transferStatus[t.status]}{t.reason && <small>{t.reason}</small>}</td></tr>)}</tbody></table></div>)}</section>
 </div>;
}
