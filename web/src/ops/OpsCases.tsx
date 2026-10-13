import { useState, type FormEvent } from 'react';
import { useLocation } from 'react-router-dom';
import { ApiClient } from '../api';
import { Alert, Loading, Modal, Pager, useResource } from '../ui';
import { opsError } from './ops-api';
import { OpsFacts, opsTime } from './OpsOperationDetail';
import './ops-content.css';

type Case={case_id:string;subject_newapi_user_id:string;category:string;state:string;safe_summary:string;created_by:string;created_at:string;updated_at:string;closed_at?:string};
type Detail={case:Case;verification_facts:unknown[]};
type Page={items:Case[];total:number};
const states:Record<string,string>={OPEN:'待处理',VERIFYING:'核验中',APPROVED:'已批准',EXECUTED:'已执行',REJECTED:'已拒绝',CLOSED:'已关闭'};
const categories:Record<string,string>={ACCOUNT_ACCESS:'账户访问',IDENTITY:'身份资料',ECONOMY:'经济账务',GAMEPLAY:'游戏',OTHER:'其他'};
const facts:Record<string,string>={IDENTITY_MATCH:'身份匹配',OWNERSHIP_VERIFIED:'归属核验',DISCORD_BINDING_VERIFIED:'Discord 绑定核验',CONTACT_VERIFIED:'联系渠道核验',OTHER_SAFE_METADATA:'其他非敏感事实'};
const nextStates:Record<string,string[]>={OPEN:['VERIFYING'],VERIFYING:['APPROVED','REJECTED'],APPROVED:['EXECUTED'],EXECUTED:['CLOSED'],REJECTED:['CLOSED'],CLOSED:[]};
const root='/api/v1/ops/support-cases';
export function OpsSupportCases({client}:{client:ApiClient}){
 const supplied=new URLSearchParams(useLocation().search).get('subject_newapi_user_id')||'';
 const [page,setPage]=useState(1),[query,setQuery]=useState(supplied?'&subject_newapi_user_id='+encodeURIComponent(supplied):''),[selected,setSelected]=useState(''),[busy,setBusy]=useState(false),[error,setError]=useState(''),[message,setMessage]=useState('');
 const resource=useResource(()=>client.request<Page>(`${root}?page=${page}${query}`),[client,page,query]);
 const detail=useResource(()=>selected&&selected!=='new'?client.request<Detail>(`${root}/${selected}`):Promise.resolve(null),[client,selected]);
 function filter(event:FormEvent<HTMLFormElement>){
  event.preventDefault();const params=new URLSearchParams();
  for(const [key,value] of new FormData(event.currentTarget)){const v=String(value).trim();if(v)params.set(key,key==='from'||key==='to'?new Date(v).toISOString():v)}
  setPage(1);setQuery('&'+params.toString());
 }
 function open(id:string){setSelected(id);setError('');setMessage('')}
 async function save(event:FormEvent<HTMLFormElement>,action:'create'|'state'|'facts'){
  event.preventDefault();const body=Object.fromEntries(new FormData(event.currentTarget));
  if(action==='state'&&(body.next_state==='CLOSED'||body.next_state==='REJECTED')&&!window.confirm('确认保存这项工单状态？'))return;
  setBusy(true);setError('');setMessage('');
  try{
   const value=await client.request<Case>(action==='create'?root:`${root}/${selected}${action==='facts'?'/facts':''}`,action==='state'?'PUT':'POST',body);
   setMessage('工单已保存');setSelected(value.case_id);resource.reload();if(action!=='create')detail.reload();
  }catch(e){setError(opsError(e))}finally{setBusy(false)}
 }
 return <><header className="page-heading"><div><p className="eyebrow">OPERATIONS / SUPPORT</p><h1>支持工单</h1><p>记录核验依据和处理进度；保存同时记录审计，不会改动账户凭据。</p></div><button onClick={()=>open('new')}>新建工单</button><button onClick={()=>{resource.reload();detail.reload()}} disabled={busy}>刷新工单</button></header>
 <section className="panel ops-cases-list" aria-label="工单列表"><form className="filters" onSubmit={filter}>
 <label>账户编号<input name="subject_newapi_user_id" defaultValue={supplied} type="number" min="1" placeholder="全部账户"/></label>
 <label>工单状态<select name="state"><option value="">全部状态</option>{Object.entries(states).map(([v,l])=><option value={v} key={v}>{l}</option>)}</select></label>
 <label>工单类别<select name="category"><option value="">全部类别</option>{Object.entries(categories).map(([v,l])=><option value={v} key={v}>{l}</option>)}</select></label>
 <label>搜索摘要<input name="q"/></label><label>创建时间从<input type="datetime-local" name="from"/></label><label>创建时间至（不含）<input type="datetime-local" name="to"/></label><button type="submit">筛选工单</button></form>
 {resource.loading?<Loading/>:resource.error?<Alert>{resource.error}</Alert>:resource.data&&<><div className="table-wrap"><table><thead><tr><th>账户 / 工单</th><th>类别 / 状态</th><th>摘要</th><th>更新时间</th><th>操作</th></tr></thead><tbody>{resource.data.items.map(item=><tr key={item.case_id}><td>{item.subject_newapi_user_id}<small>{item.case_id}</small></td><td>{categories[item.category]} · {states[item.state]}</td><td>{item.safe_summary}</td><td>{opsTime(item.updated_at)}</td><td><button onClick={()=>open(item.case_id)}>查看工单</button></td></tr>)}</tbody></table></div>{!resource.data.items.length&&<p>暂无匹配工单。</p>}<Pager page={page} size={20} total={resource.data.total} onChange={setPage}/></>}
 </section>
 {selected&&<Modal title={selected==='new'?'新建工单':'工单详情'} onClose={()=>setSelected('')} busy={busy}>
 {message&&<p role="status">{message}</p>}{error&&<Alert>{error}</Alert>}
 {selected==='new'?<form onSubmit={e=>void save(e,'create')}><fieldset disabled={busy}>
 <label>工单账户<input name="subject_newapi_user_id" defaultValue={supplied} type="number" min="1" required/></label>
 <label>类别<select name="category">{Object.entries(categories).map(([v,l])=><option value={v} key={v}>{l}</option>)}</select></label>
 <label>摘要<textarea name="safe_summary" required maxLength={1000}/></label><button type="submit">保存新工单</button></fieldset></form>
 :detail.loading?<Loading/>:detail.error?<Alert>{detail.error}</Alert>:detail.data&&<><OpsFacts value={detail.data.case}/><h3>核验事实</h3><OpsFacts value={detail.data.verification_facts}/>
 {nextStates[detail.data.case.state].length>0&&<form key={detail.data.case.state} onSubmit={e=>void save(e,'state')}><fieldset disabled={busy}><label>下一状态<select name="next_state">{nextStates[detail.data.case.state].map(v=><option key={v} value={v}>{states[v]}</option>)}</select></label><button type="submit">保存工单状态</button></fieldset></form>}
 {detail.data.case.state!=='CLOSED'&&<form onSubmit={e=>void save(e,'facts')}><fieldset disabled={busy}><h3>追加核验事实</h3><label>核验项目<select name="fact_type">{Object.entries(facts).map(([v,l])=><option key={v} value={v}>{l}</option>)}</select></label><label>结果<select name="result"><option value="INCONCLUSIVE">尚无结论</option><option value="VERIFIED">已核验</option><option value="NOT_VERIFIED">未通过核验</option></select></label><label>非敏感依据编号（可选）<input name="safe_reference" maxLength={200}/></label><button type="submit">保存核验事实</button></fieldset></form>}</>}
 </Modal>}</>;
}
