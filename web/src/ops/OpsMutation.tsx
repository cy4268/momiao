import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { Alert } from '../ui';
import { canAcknowledgeOpsReview, checkOperation, checkOperationView, opsError, type OpsCommand, type OpsPrepared, type OpsOperationView } from './ops-api';
import { OpsFacts, OpsReceipt, opsState } from './OpsOperationDetail';
import { useOps } from './OpsShell';
import { opsPopupMessage, opsPopupPrefix, validateOpsDiscordURL } from './OpsDiscordPopup';
import { clearOpsPending, opsPendingKey, readOpsPending, saveOpsPending } from './ops-pending';

type Pending={id:string;command:OpsCommand;schema:string;epoch:string;restored?:boolean};
const secret=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9_-]{16,512}$/.test(value);
export function useOpsMutation(){
 const {client,bootstrap}=useOps();const [pending,setPending]=useState<Pending>(),[prepared,setPrepared]=useState<OpsPrepared>(),[receipt,setReceipt]=useState<OpsOperationView>(),[busy,setBusy]=useState(false),[error,setError]=useState(''),[flow,setFlow]=useState(''),[verified,setVerified]=useState(false),[uncertain,setUncertain]=useState(false);
 const running=useRef(false),live=useRef(true);
 const storageKey=opsPendingKey(bootstrap.principal.principal_id);
 const [discordWaiting,setDiscordWaiting]=useState(false),[factorKind,setFactorKind]=useState<'PASSWORD'|'DISCORD'>('PASSWORD');
 const popup=useRef<{window:Window;pending:Pending;challenge:NonNullable<OpsPrepared['fresh_challenge']>}|undefined>(undefined);
 useEffect(()=>{live.current=true;return()=>{live.current=false}},[]);
 useEffect(()=>{
  const restore=()=>{try{const marker=readOpsPending(storageKey);if(marker){setPending(value=>value?.id===marker.id?value:{id:marker.id,command:{operation_type:marker.operation_type,target:{type:'',id:'',expected_version:'0'},input:{},reason:''},schema:'',epoch:bootstrap.principal.authz_epoch,restored:true});setUncertain(true);setPrepared(undefined);setVerified(false);setFlow('')}}catch(error){setError(opsError(error))}};
  restore();const receive=(event:StorageEvent)=>{if(event.key===storageKey)restore()};window.addEventListener('storage',receive);return()=>window.removeEventListener('storage',receive);
 },[storageKey,bootstrap.principal.authz_epoch]);
 useEffect(()=>{
  const receive=(event:MessageEvent)=>{
   const attempt=popup.current;if(!attempt||event.source!==attempt.window||event.origin!==window.location.origin||event.data?.type!==opsPopupMessage)return;
   popup.current=undefined;attempt.window.close();setDiscordWaiting(false);
   void run(async()=>{const input=event.data.input;if(!input||typeof input.state!=='string'||input.state.length>2048||!((typeof input.code==='string'&&input.code.length<=2048&&!input.error)||(input.error==='access_denied'&&!input.code)))throw new Error('Discord 验证回调无效，请重新开始。');
    const result=await client.request<{proof?:string;require_2fa?:boolean;flow_token?:string}>('/api/v1/ops/fresh/discord/callback','POST',{operation_id:attempt.pending.id,authz_epoch:attempt.pending.epoch,challenge_id:attempt.challenge.challenge_id,state:input.state,...(input.code?{code:input.code}:{error:input.error,...(typeof input.error_description==='string'?{error_description:input.error_description}:{})})});
    await acceptFactor(result,attempt.pending,attempt.challenge,'DISCORD');
   });
  };
  window.addEventListener('message',receive);const timer=window.setInterval(()=>{if(popup.current?.window.closed){popup.current=undefined;setDiscordWaiting(false)}},1000);
  return()=>{window.removeEventListener('message',receive);window.clearInterval(timer);popup.current?.window.close();popup.current=undefined};
 },[client]);
 async function run(action:()=>Promise<void>){if(running.current)return;running.current=true;setBusy(true);setError('');try{await action()}catch(error){if(live.current)setError(opsError(error))}finally{running.current=false;if(live.current)setBusy(false)}}
 async function loadPreview(value:Pending,renew=false){
  const result=await client.request<OpsPrepared>(renew?`/api/v1/ops/operations/${value.id}/fresh/renew`:'/api/v1/ops/operations','POST',renew?{authz_epoch:value.epoch}:{operation_id:value.id,operation_type:value.command.operation_type,authz_epoch:value.epoch,input_schema_version:value.schema,target:value.command.target,input:value.command.input,reason:value.command.reason},undefined,()=>live.current);
  checkOperation(result?.operation);
  if(result.operation.operation_id!==value.id||!result.descriptor||result.descriptor.operation_type!==value.command.operation_type||result.descriptor.requires_fresh_auth&&(!result.fresh_challenge||!/^[A-Za-z0-9_-]{43}$/.test(result.fresh_challenge.challenge_id)||!secret(result.fresh_challenge.challenge_token)||!Number.isFinite(Date.parse(result.fresh_challenge.expires_at))))throw new Error('预览响应无法核对，请查询原操作。');
  if(live.current){setPrepared(result);setFlow('');setVerified(false);setFactorKind('PASSWORD')}
 }
 function prepare(command:OpsCommand){if(pending||running.current)return;const descriptor=bootstrap.operations.find(item=>item.operation_type===command.operation_type&&item.available&&bootstrap.principal.permissions.includes(item.required_permission));if(!descriptor){setError('当前身份或环境未提供此操作。');return}const value={id:crypto.randomUUID(),command,schema:descriptor.input_schema_version,epoch:bootstrap.principal.authz_epoch};setPending(value);setReceipt(undefined);setUncertain(false);void run(()=>loadPreview(value));}
 function previewAgain(){if(pending&&!pending.restored&&!uncertain)void run(async()=>{setVerified(false);setFlow('');popup.current?.window.close();popup.current=undefined;setDiscordWaiting(false);await loadPreview(pending,!!prepared?.descriptor.requires_fresh_auth)})}
 function verify(password:string,code:string){if(!pending||!prepared?.fresh_challenge)return;const current=pending,challenge=prepared.fresh_challenge;void run(async()=>{
  if(Date.parse(challenge.expires_at)<=Date.now())throw new Error('验证已过期，请刷新本次身份验证。');
  const result=flow?await client.request<{proof?:string;require_2fa?:boolean;flow_token?:string}>(factorKind==='DISCORD'?'/api/v1/ops/fresh/discord/2fa':'/api/v1/ops/fresh/2fa','POST',{flow_token:flow,code,...(factorKind==='DISCORD'?{operation_id:current.id,authz_epoch:current.epoch,challenge_id:challenge.challenge_id}:{})},undefined,()=>live.current):await client.request<{proof?:string;require_2fa?:boolean;flow_token?:string}>('/api/v1/ops/fresh/password','POST',{challenge_token:challenge.challenge_token,password},undefined,()=>live.current);
  await acceptFactor(result,current,challenge,flow?factorKind:'PASSWORD');
 })}
 async function acceptFactor(result:{proof?:string;require_2fa?:boolean;flow_token?:string},current:Pending,challenge:NonNullable<OpsPrepared['fresh_challenge']>,kind:'PASSWORD'|'DISCORD'){
  if(!live.current)return;
  if(result.require_2fa===true&&secret(result.flow_token)&&!result.proof){setFlow(result.flow_token);setFactorKind(kind);return}
  if(!secret(result.proof)||result.require_2fa||result.flow_token)throw new Error('身份验证响应异常，请刷新本次身份验证。');
  await client.exchangeOpsFresh({operation_id:current.id,authz_epoch:current.epoch,challenge_id:challenge.challenge_id,proof:result.proof});
  if(live.current){setVerified(true);setFlow('')}
 }
 function discord(){if(!pending||!prepared?.fresh_challenge||running.current||popup.current)return;const opened=window.open('',opsPopupPrefix+crypto.randomUUID(),'popup,width=520,height=760');if(!opened){setError('验证窗口被浏览器拦截，请允许弹出窗口后重试。');return}const attempt={window:opened,pending,challenge:prepared.fresh_challenge};popup.current=attempt;setDiscordWaiting(true);void run(async()=>{try{const result=await client.request<{authorization_url:string}>('/api/v1/ops/fresh/discord/start','POST',{operation_id:pending.id,authz_epoch:pending.epoch,challenge_id:attempt.challenge.challenge_id,challenge_token:attempt.challenge.challenge_token});if(popup.current===attempt)opened.location.href=validateOpsDiscordURL(result?.authorization_url)}catch(error){opened.close();popup.current=undefined;setDiscordWaiting(false);throw error}})}
 function execute(confirmed:boolean,typed:string){if(!pending||!prepared||uncertain)return;const current=pending,preview=prepared;void run(async()=>{
  if(!navigator.locks)throw new Error('当前浏览器无法协调管理写入，请使用支持安全锁的浏览器。');
  await navigator.locks.request(storageKey,async()=>{
  if(!live.current) return;
  saveOpsPending(storageKey,{id:current.id,operation_type:current.command.operation_type,path:window.location.pathname});
  // Once sent, only a read of this operation may resolve an unknown outcome.
  setUncertain(true);
  const result=await client.request<{operation:OpsOperationView}>(`/api/v1/ops/operations/${current.id}/execute`,'POST',{authz_epoch:current.epoch,impact_hash:preview.operation.impact_hash,confirmed,typed_confirmation:typed,...(preview.fresh_challenge?{fresh:{challenge_id:preview.fresh_challenge.challenge_id}}:{})},undefined,()=>live.current);
  const view=checkOperationView(result?.operation);if(view.operation.operation_id!==current.id||view.operation.actor_newapi_user_id!==bootstrap.principal.newapi_user_id)throw new Error('回执编号或操作者不一致，请查询原操作。');if(live.current){setReceipt(view);setUncertain(!['PREPARED','SUCCEEDED','FAILED_NO_EFFECT','CANCELLED'].includes(view.operation.state)&&!canAcknowledgeOpsReview(view))}
  });
 })}
 function readOriginal(){if(!pending)return;const current=pending;void run(async()=>{const view=checkOperationView(await client.request<OpsOperationView>(`/api/v1/ops/operations/${current.id}?authz_epoch=${bootstrap.principal.authz_epoch}`));if(view.operation.operation_id!==current.id||view.operation.actor_newapi_user_id!==bootstrap.principal.newapi_user_id)throw new Error('原操作回执编号或操作者不一致，仍需核对。');if(live.current){setReceipt(view);setUncertain(!['PREPARED','SUCCEEDED','FAILED_NO_EFFECT','CANCELLED'].includes(view.operation.state)&&!canAcknowledgeOpsReview(view))}})}
 function reset(){if(running.current||uncertain||!pending)return;const current=pending;void run(async()=>{
  if(!navigator.locks)throw new Error('当前浏览器无法协调管理写入，请使用支持安全锁的浏览器。');
  await navigator.locks.request(storageKey,async()=>{
   if(!live.current)return;
   const acknowledgeReview=canAcknowledgeOpsReview(receipt);
   const view=acknowledgeReview
    ?checkOperationView(await client.request<OpsOperationView>(`/api/v1/ops/operations/${current.id}?authz_epoch=${bootstrap.principal.authz_epoch}`))
    :checkOperationView((await client.request<{operation:OpsOperationView}>(`/api/v1/ops/operations/${current.id}/cancel`,'POST',{authz_epoch:bootstrap.principal.authz_epoch},undefined,()=>live.current))?.operation);
   if(view.operation.operation_id!==current.id||view.operation.actor_newapi_user_id!==bootstrap.principal.newapi_user_id||!(acknowledgeReview?canAcknowledgeOpsReview(view):['CANCELLED','SUCCEEDED','FAILED_NO_EFFECT'].includes(view.operation.state)))throw new Error('原操作尚未安全结束，请查询原操作后重试。');
   if(view.operation.state!=='CANCELLED'&&receipt?.operation.state!==view.operation.state){if(live.current){setReceipt(view);setUncertain(false)}return;}
   clearOpsPending(storageKey,current.id);
   if(live.current){popup.current?.window.close();popup.current=undefined;setDiscordWaiting(false);setPending(undefined);setPrepared(undefined);setReceipt(undefined);setFlow('');setVerified(false);setError('')}
  });
 })}
 return {pending,prepared,receipt,busy:busy||discordWaiting,error,flow,verified,uncertain,discordWaiting,prepare,previewAgain,verify,discord,execute,readOriginal,reset};
}
export function OpsMutationPanel({mutation}:{mutation:ReturnType<typeof useOpsMutation>}){
 const [password,setPassword]=useState(''),[code,setCode]=useState(''),[confirmed,setConfirmed]=useState(false),[typed,setTyped]=useState('');
 const {pending,prepared,receipt,busy,error}=mutation;
 useEffect(()=>{setPassword('');setCode('');setConfirmed(false);setTyped('')},[pending?.id,prepared]);
 if(!pending)return error?<Alert>{error}</Alert>:null;
 const descriptor=prepared?.descriptor,impact=prepared?.operation.impact_preview;
 function verify(event:FormEvent){event.preventDefault();mutation.verify(password,code);setPassword('');setCode('')}
 const terminal=receipt&&receipt.operation.state!=='PREPARED';
 return <section className="panel" aria-label="管理操作确认"><h2>核对本次操作</h2><p>操作编号：<Link to={`/ops/operations/${pending.id}`}>{pending.id}</Link></p>{error&&<Alert>{error}</Alert>}{prepared&&impact&&<><p><strong>{opsState[prepared.operation.environment]}</strong> · 目标 {prepared.operation.target.id}</p><p>操作原因：{pending.command.reason||'—'}</p><div className="ops-impact"><section><h3>当前状态</h3><OpsFacts value={impact.current_state}/></section><section><h3>拟进行的变更</h3><OpsFacts value={impact.proposed_change}/></section>{impact.delta&&<section><h3>变化与依据</h3><OpsFacts value={impact.delta}/></section>}</div>{impact.blocking_facts?.length>0&&<Alert><ul>{impact.blocking_facts.map(item=><li key={item}>{item}</li>)}</ul></Alert>}{impact.continuing_accepted_work?.length>0&&<details><summary>仍会继续的已受理工作</summary><ul>{impact.continuing_accepted_work.map(item=><li key={item}>{item}</li>)}</ul></details>}{impact.unavailable_measurements?.length>0&&<details><summary>无法测量的影响项</summary><p className="hint">以下项目目前不可读取，请结合其他业务页面核对，不能按零处理。</p><OpsFacts value={impact.unavailable_measurements}/></details>}</>}{prepared&&!terminal&&!mutation.uncertain&&<>
 {descriptor?.requires_fresh_auth&&!mutation.verified&&<form onSubmit={verify}><fieldset disabled={busy}><legend>验证当前身份</legend>{mutation.flow?<label>二次验证码<input value={code} onChange={event=>setCode(event.target.value)} autoComplete="one-time-code" maxLength={64} required/></label>:<label>当前密码<input type="password" value={password} onChange={event=>setPassword(event.target.value)} autoComplete="current-password" maxLength={256} required/></label>}<button type="submit">验证身份</button>{!mutation.flow&&<button type="button" onClick={mutation.discord}>使用 Discord 验证</button>}</fieldset></form>}
 {mutation.discordWaiting&&<p role="status">请在 Discord 窗口完成验证；关闭该窗口可返回选择。</p>}{mutation.verified&&<p role="status">本次操作的身份验证已完成。</p>}
 {descriptor?.confirmation_mode==='EXPLICIT'&&<label><input type="checkbox" checked={confirmed} onChange={event=>setConfirmed(event.target.checked)} disabled={busy}/> 我已核对本次影响并确认执行</label>}
 {descriptor?.confirmation_mode==='TYPED'&&<label>请输入确认语句：<code>{prepared.confirmation_phrase}</code><input value={typed} onChange={event=>setTyped(event.target.value)} autoComplete="off" disabled={busy} maxLength={256}/></label>}
 <div className="ops-actions"><button className="primary" disabled={busy||!!impact?.blocking_facts?.length||!!descriptor?.requires_fresh_auth&&!mutation.verified||descriptor?.confirmation_mode==='EXPLICIT'&&!confirmed||descriptor?.confirmation_mode==='TYPED'&&typed!==prepared.confirmation_phrase} onClick={()=>mutation.execute(confirmed,typed)}>确认执行</button></div></>}
 {mutation.uncertain&&<p role="status">执行结果尚待核对。请查询原操作后再继续。</p>}
 {canAcknowledgeOpsReview(receipt)&&<p role="status">已确认远端回执和审计，业务仍需人工核对。关闭本次回执会保留待核对记录，可随后进入对应业务的恢复流程。</p>}
 <div className="ops-actions"><button disabled={busy} onClick={mutation.readOriginal}>查询原操作</button>{!terminal&&!pending.restored&&!mutation.uncertain&&<button disabled={busy} onClick={mutation.previewAgain}>{prepared?.descriptor.requires_fresh_auth?'刷新本次身份验证':'读取原预览'}</button>}<button disabled={busy||mutation.uncertain} onClick={mutation.reset}>{canAcknowledgeOpsReview(receipt)?'保留待核对记录并关闭回执':terminal?'关闭回执':'返回编辑'}</button></div>{receipt&&<OpsReceipt view={receipt}/>}</section>;
}



