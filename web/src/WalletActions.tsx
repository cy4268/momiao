import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { parseNativeQuota } from './quota-api';
import { DailyReward } from './DailyReward';
import { RewardPrograms } from './RewardPrograms';
import { ApiClient, ApiError } from './api';
import { Alert, Empty, Loading, useResource } from './ui';
import { assetNames, assets, type Asset, type WalletData } from './wallet-api';
import { amountUnits, matchesOperation, transactionStatus, transactionTerminal, unitsAmount, operationError, parsePending, parseTransaction, readDaily, readTransactions, type PendingOperation, type Transaction } from './economy-api';

export function WalletActions({client,userID,wallet,onChange,dailyOnly=false,ledger,beforeHistory,dailyLinks}:{client:ApiClient;userID:string;wallet:WalletData;onChange:(t:Transaction)=>void;dailyOnly?:boolean;ledger?:ReactNode;beforeHistory?:ReactNode;dailyLinks?:ReactNode}) {
 const [historyTab,setHistoryTab]=useState<'transactions'|'ledger'>('transactions');
 const storageKey=`momiao.wallet.pending.${userID}`;
 const [historyVersion,setHistoryVersion]=useState(0);
 const [pending,setPending]=useState<PendingOperation|null>(null);
 const [pendingReceipt,setPendingReceipt]=useState<Transaction|null>(null);
 const [storageError,setStorageError]=useState(false);
 const [busy,setBusy]=useState(false);const [retry,setRetry]=useState(false);const [notice,setNotice]=useState('');
 const [from,setFrom]=useState<Asset>('RESERVE_API_CREDIT');const [amount,setAmount]=useState('');
 const lock=useRef(false);const active=useRef(false);const generation=useRef(client.getSessionGeneration());
 useEffect(()=>{active.current=true;try{setPending(parsePending(sessionStorage.getItem(storageKey)))}catch{setStorageError(true)}return()=>{active.current=false}},[storageKey]);
 const current=()=>active.current && client.getSessionGeneration()===generation.current && String(client.getSnapshot().user?.id)===userID && !client.getSnapshot().loggingOut;
 const native=useResource(async()=>dailyOnly?null:parseNativeQuota(await client.request('/platform/v1/native-quota'),userID),[client,userID,dailyOnly,historyVersion]);
 const daily=useResource(()=>readDaily(client,userID),[client,userID]);
 // A read at the server-provided reset or on return to this tab refreshes the date; it never claims.
 useEffect(()=>{
  const reset=daily.data?.next_reset_at;if(!reset)return;
  const delay=Date.parse(reset)-Date.now();
  const timer=delay>0?window.setTimeout(daily.reload,Math.min(delay+100,2147483647)):undefined;
  const refresh=()=>daily.reload();window.addEventListener('focus',refresh);
  return()=>{window.clearTimeout(timer);window.removeEventListener('focus',refresh)};
 },[daily.data?.next_reset_at]);
 const blocked=busy || pending!==null || storageError;
 const maintenanceActive=daily.data?.maintenance_active===true;
 const rewardBlocked=blocked || daily.data===undefined || maintenanceActive;
 const to:Asset=from==='RESERVE_API_CREDIT'?'AVAILABLE_CHIPS':'RESERVE_API_CREDIT';
 const units=amountUnits(amount);const balance=wallet.wallets.find(w=>w.asset===from)!;
 const available=BigInt(balance.balance_units)+(from==='RESERVE_API_CREDIT' && native.data?.enabled?BigInt(native.data.raw_quota):0n);
 function finish(t:Transaction,p:PendingOperation){if(!matchesOperation(t,p))throw new Error('Receipt mismatch');setRetry(false);setPendingReceipt(t);setNotice(transactionStatus[t.status]);setHistoryVersion(v=>v+1);if(!transactionTerminal(t))return;sessionStorage.removeItem(storageKey);setPending(null);setPendingReceipt(null);daily.reload();onChange(t)}
 async function send(p:PendingOperation,isRetry=false):Promise<Transaction|undefined>{
  if(lock.current || storageError || (!isRetry && pending))return;
  lock.current=true;setBusy(true);setNotice('');
  try {
   // Persist the non-secret original request before sending; a reload never invents a retry key.
   sessionStorage.setItem(storageKey,JSON.stringify(p));setPending(p);setRetry(false);
   const endpoint=p.kind==='EXCHANGE'?'/platform/v1/wallet/exchange':`/platform/v1/rewards/${p.kind.toLowerCase()}/claim`;
   const result=await client.request(endpoint,'POST',{idempotency_key:p.key,...(p.kind==='EXCHANGE'?{from_asset:p.from_asset,amount:p.amount}:{})});
   if(current()){const transaction=parseTransaction(result,userID);finish(transaction,p);return transaction;}
  }catch(e){if(current()){
   if(e instanceof ApiError && e.status>=400 && e.status<500 && e.code && e.code!=='IDEMPOTENCY_CONFLICT') {sessionStorage.removeItem(storageKey);setPending(null)}
   setNotice(operationError(e));
  }}finally{lock.current=false;if(current())setBusy(false)}
 }
 async function reconcile(){
  if(!pending || lock.current)return;const p=pending;lock.current=true;setBusy(true);setNotice('');setRetry(false);
  try{const value=await client.request(`/platform/v1/transactions/by-key?kind=${p.kind}&key=${p.key}`);if(current()){if(value===null){setRetry(true);setNotice('尚未查到交易受理记录。可按原请求重试，不会更换请求编号。')}else{finish(parseTransaction(value,userID),p)}}}
  catch(e){if(current())setNotice(operationError(e))}finally{lock.current=false;if(current())setBusy(false)}
 }
 function exchange(e:FormEvent){e.preventDefault();if(blocked || units===null || units>available)return;void send({kind:'EXCHANGE',key:crypto.randomUUID(),from_asset:from,amount})}
 return <>
  {notice && <Alert>{notice}</Alert>}
  {storageError && <Alert>浏览器未能读取待核对请求。请检查会话存储；为避免重复交易，资产操作已暂停。</Alert>}
  {maintenanceActive && <Alert>奖励维护中：每日、小时与低资产救济暂停新的领取；已受理请求仍可核对。<button type="button" disabled={blocked} onClick={daily.reload}>重新读取维护状态</button></Alert>}
  {pending && <section className="panel pending-operation" aria-label="待核对交易"><h2>先核对上一笔操作</h2><p>{pending.kind==='DAILY'?'每日签到':pending.kind==='HOURLY'?'小时奖励':pending.kind==='RELIEF'?'低资产救济':`兑换 ${pending.amount} ${assetNames[pending.from_asset!]}`} · 已保留原请求，核对前暂停新的资产操作。</p><code>{pending.key}</code>{pendingReceipt?.kind==="API_CHIPS_EXCHANGE" && <p>固定来源：Reserve {unitsAmount(BigInt(pendingReceipt.reserve_debit_units!))} + Active {unitsAmount(BigInt(pendingReceipt.active_debit_units!))}；{transactionStatus[pendingReceipt.status]}</p>}<div className="actions"><button disabled={busy} onClick={()=>void reconcile()}>核对交易结果</button>{retry && <button disabled={busy} onClick={()=>void send(pending,true)}>按原请求重试</button>}</div></section>}
  <div className={dailyOnly?"reward-detail":"wallet-actions-grid"}>
   <DailyReward daily={daily} blocked={rewardBlocked} onClaim={()=>void send({kind:'DAILY',key:crypto.randomUUID()})} links={dailyLinks}/>
   {dailyOnly && <RewardPrograms client={client} userID={userID} blocked={rewardBlocked} version={historyVersion} onClaim={kind=>send({kind,key:crypto.randomUUID()})}/>}
   {!dailyOnly && <section className="panel finance-exchange" aria-labelledby="exchange-title"><p className="eyebrow">EXCHANGE / API CREDIT & CHIPS</p><h2 id="exchange-title">资产兑换</h2><p>1:1 · 无手续费 · 优先 Reserve，不足使用 Active。</p><form className="exchange-form" onSubmit={exchange}><label>兑换方向<select disabled={blocked} value={from} onChange={e=>setFrom(e.target.value as Asset)}>{assets.map(a=><option value={a} key={a}>{a==='RESERVE_API_CREDIT'?'API Credit（Reserve + Active）':assetNames[a]} → {assetNames[a==='RESERVE_API_CREDIT'?'AVAILABLE_CHIPS':'RESERVE_API_CREDIT']}</option>)}</select></label><label>兑换数量<input inputMode="decimal" maxLength={30} value={amount} disabled={blocked} onChange={e=>setAmount(e.target.value)} placeholder="例如 100"/></label><p className="hint">可用 {unitsAmount(available)} {from==='RESERVE_API_CREDIT' && native.data?.enabled?'API Credit（Reserve + Active）':assetNames[from]}；最小步长 0.000002。</p>{from==='RESERVE_API_CREDIT' && (native.loading || native.error || !native.data?.enabled) && <p className="hint">Active 额度尚未确认可用，当前仅列出 Reserve；可刷新后核对。<button type="button" disabled={native.loading || busy} onClick={native.reload}>刷新 Active 额度</button></p>}{amount && <p aria-live="polite">{units===null?'请输入可精确表示的正数。':units>available?'来源余额不足。':`将兑换 ${amount} ${from==='RESERVE_API_CREDIT'?'API Credit（优先 Reserve，其次 Active）':assetNames[from]}，获得 ${amount} ${assetNames[to]}。`}</p>}<button className="primary" disabled={blocked || units===null || units>available}>确认兑换</button></form></section>}
  </div>
  {beforeHistory}
  <section className="finance-history">
   {ledger && <nav className="finance-history-switch" aria-label="资产记录切换"><button aria-pressed={historyTab==='transactions'} onClick={()=>setHistoryTab('transactions')}>资产交易记录</button><button aria-pressed={historyTab==='ledger'} onClick={()=>setHistoryTab('ledger')}>资产流水</button></nav>}
   <div className="finance-history-page" hidden={historyTab!=='transactions'}><TransactionHistory key={historyVersion} client={client} userID={userID}/></div>
   {ledger && <div className="finance-history-page" hidden={historyTab!=='ledger'}>{ledger}</div>}
  </section>
 </>;
}
function TransactionHistory({client,userID}:{client:ApiClient;userID:string}) {
 const [cursors,setCursors]=useState(['']);const after=cursors[cursors.length-1];
 const r=useResource(()=>readTransactions(client,userID,after),[client,userID,after]);
 const label=(t:Transaction)=>t.kind==='DAILY_REWARD'?'每日签到':t.kind==='HOURLY_REWARD'?'小时奖励':t.kind==='RELIEF_REWARD'?'低资产救济':t.kind==='INITIAL_GRANT_REGISTRATION'?'新用户注册赠额':t.kind==='API_CHIPS_EXCHANGE'?'API → Chips':'本地兑换';
 return <section className="panel finance-transactions"><div className="section-heading"><div><p className="eyebrow">ACTIVITY / TRANSACTIONS</p><h2>资产交易记录</h2></div><button disabled={r.loading} onClick={r.reload}>刷新交易</button></div>{r.loading?<Loading/>:r.error?<Alert>{r.error}</Alert>:r.data && (r.data.items.length===0?<Empty title="暂无交易">注册赠额、奖励和兑换受理后，这里显示真实状态。</Empty>:<div className="table-wrap" role="region" aria-label="资产交易记录" tabIndex={0}><table><thead><tr><th>时间 / 交易编号</th><th>业务</th><th>数量 / 去向</th><th>状态</th></tr></thead><tbody>{r.data.items.map(t=><tr key={t.id}><td><time dateTime={t.created_at}>{new Date(t.created_at).toLocaleString('zh-CN',{hour12:false})}</time><small className="transaction-id">{t.id}</small></td><td>{label(t)}</td><td>{t.amount}<small>{t.from_asset?`${assetNames[t.from_asset]} → `:''}{assetNames[t.to_asset]}</small></td><td>{transactionStatus[t.status]}</td></tr>)}</tbody></table></div>)}<nav className="pager" aria-label="资产交易分页"><span>每页 20 条，最新交易在前</span><div><button disabled={r.loading || cursors.length===1} onClick={()=>setCursors(c=>c.slice(0,-1))}>上一页交易</button><button disabled={r.loading || !r.data?.has_more || !!r.error} onClick={()=>{if(r.data?.next_after_id)setCursors(c=>[...c,r.data!.next_after_id!])}}>下一页交易</button></div></nav></section>;
}
