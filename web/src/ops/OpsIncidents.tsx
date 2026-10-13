import { useState, type FormEvent } from 'react';
import { ApiClient } from '../api';
import { Alert, Loading, Modal, Pager, useResource } from '../ui';
import { opsError } from './ops-api';
import { OpsFacts, opsTime } from './OpsOperationDetail';

type Incident={incident_id:string;title:string;severity:string;state:string;safe_summary:string;created_by:string;created_at:string;updated_at:string;resolved_at?:string;closed_at?:string};
type Event={event_id:string;event_type:string;from_state?:string;to_state?:string;safe_summary?:string;related_type?:string;related_id?:string;actor_newapi_user_id:string;occurred_at:string};
type Detail={incident:Incident;events:Event[]};
const states:Record<string,string>={OPEN:'待研判',TRIAGED:'已研判',IN_PROGRESS:'处理中',MONITORING:'观察中',RESOLVED:'已解决',CLOSED:'已关闭'};
const severityNames:Record<string,string>={CRITICAL:'严重',WARNING:'警告',INFO:'信息'};
const nextStates:Record<string,string[]>={OPEN:['TRIAGED','CLOSED'],TRIAGED:['IN_PROGRESS','CLOSED'],IN_PROGRESS:['MONITORING','CLOSED'],MONITORING:['RESOLVED','CLOSED'],RESOLVED:['CLOSED'],CLOSED:[]};
const eventNames:Record<string,string>={COMMENT:'处理备注',OPERATION_LINKED:'关联历史操作',JOB_LINKED:'关联任务',AUDIT_LINKED:'关联审计',BUSINESS_FACT_LINKED:'关联业务事实'};
const relatedTypes:Record<string,string>={OPERATION_LINKED:'OPERATION',JOB_LINKED:'JOB',AUDIT_LINKED:'AUDIT',BUSINESS_FACT_LINKED:'BUSINESS_FACT'};
const root='/api/v1/ops/incidents';
export function OpsIncidents({client}:{client:ApiClient}){
 const [page,setPage]=useState(1),[query,setQuery]=useState(''),[selected,setSelected]=useState(''),[busy,setBusy]=useState(false),[error,setError]=useState(''),[message,setMessage]=useState(''),[eventType,setEventType]=useState('COMMENT');
 const resource=useResource(()=>client.request<{items:Incident[];total:number}>(`${root}?page=${page}${query}`),[client,page,query]);
 const detail=useResource(()=>selected&&selected!=='new'?client.request<Detail>(`${root}/${selected}`):Promise.resolve(null),[client,selected]);
 function filter(event:FormEvent<HTMLFormElement>){
  event.preventDefault();const params=new URLSearchParams();
  for(const [key,value] of new FormData(event.currentTarget)){const v=String(value).trim();if(v)params.set(key,key==='from'||key==='to'?new Date(v).toISOString():v)}
  setPage(1);setQuery('&'+params.toString());
 }
 function open(id:string){setSelected(id);setError('');setMessage('');setEventType('COMMENT')}
 async function save(event:FormEvent<HTMLFormElement>,action:'create'|'state'|'events'){
  event.preventDefault();const body:Record<string,unknown>=Object.fromEntries(new FormData(event.currentTarget));
  if(action==='events'&&eventType!=='COMMENT')body.related_type=relatedTypes[eventType];
  if(action==='state'&&body.next_state==='CLOSED'&&!window.confirm('确认关闭这项事件？'))return;
  setBusy(true);setError('');setMessage('');
  try{
   const value=await client.request<Incident>(action==='create'?root:`${root}/${selected}${action==='events'?'/events':''}`,action==='state'?'PUT':'POST',body);
   setMessage('事件已保存');setSelected(value.incident_id);resource.reload();if(action!=='create')detail.reload();
  }catch(e){setError(opsError(e))}finally{setBusy(false)}
 }
 return <><header className="page-heading ops-system-heading"><div><p className="eyebrow">OPERATIONS / INCIDENTS</p><h1>事件处理</h1><p>汇集业务影响、处理进展与关联记录。</p></div><button onClick={()=>open('new')}>创建事件</button><button onClick={()=>{resource.reload();detail.reload()}} disabled={busy}>刷新事件</button></header>
 <section className="panel ops-system-list" aria-label="事件列表与筛选"><form className="filters" onSubmit={filter}>
 <label>状态<select name="state"><option value="">全部状态</option>{Object.entries(states).map(([v,l])=><option value={v} key={v}>{l}</option>)}</select></label>
 <label>严重程度<select name="severity"><option value="">全部程度</option>{Object.entries(severityNames).map(([v,l])=><option value={v} key={v}>{l}</option>)}</select></label><label>搜索事件<input name="q"/></label>
 <label>创建时间从<input type="datetime-local" name="from"/></label><label>创建时间至（不含）<input type="datetime-local" name="to"/></label><button type="submit">筛选事件</button></form>
 {resource.loading?<Loading/>:resource.error?<Alert>{resource.error}</Alert>:resource.data&&<><div className="table-wrap" role="region" aria-label="事件列表" tabIndex={0}><table><thead><tr><th>事件</th><th>程度 / 状态</th><th>摘要</th><th>更新时间</th><th>操作</th></tr></thead><tbody>{resource.data.items.map(item=><tr key={item.incident_id}><td>{item.title}<small>{item.incident_id}</small></td><td>{severityNames[item.severity]} · {states[item.state]}</td><td>{item.safe_summary}</td><td>{opsTime(item.updated_at)}</td><td><button onClick={()=>open(item.incident_id)}>查看事件</button></td></tr>)}</tbody></table></div>{!resource.data.items.length&&<p>暂无匹配事件。</p>}<Pager page={page} size={20} total={resource.data.total} onChange={setPage}/></>}
 </section>
 {selected&&<Modal title={selected==='new'?'创建事件':'事件详情'} onClose={()=>setSelected('')} busy={busy}>
 {message&&<p role="status">{message}</p>}{error&&<Alert>{error}</Alert>}
 {selected==='new'?<form onSubmit={e=>void save(e,'create')}><fieldset disabled={busy}><label>标题<input name="title" maxLength={160} required/></label><label>事件严重程度<select name="severity" defaultValue="WARNING">{Object.entries(severityNames).map(([v,l])=><option value={v} key={v}>{l}</option>)}</select></label><label>业务影响摘要<textarea name="safe_summary" maxLength={1000} required/></label><button type="submit">保存新事件</button></fieldset></form>
 :detail.loading?<Loading/>:detail.error?<Alert>{detail.error}</Alert>:detail.data&&<><OpsFacts value={detail.data.incident}/><h3>处理时间线</h3><ol className="ops-timeline">{detail.data.events.map(item=><li key={item.event_id}><strong>{eventNames[item.event_type]||item.event_type}</strong><time dateTime={item.occurred_at}>{opsTime(item.occurred_at)}</time>{item.to_state&&<p>{item.from_state?states[item.from_state]+' → ':''}{states[item.to_state]}</p>}{item.safe_summary&&<p>{item.safe_summary}</p>}{item.related_id&&<p>{item.related_type} · {item.related_id}</p>}<small>记录账户：{item.actor_newapi_user_id}</small></li>)}</ol>
 {nextStates[detail.data.incident.state].length>0&&<form key={detail.data.incident.state} onSubmit={e=>void save(e,'state')}><fieldset disabled={busy}><label>下一状态<select name="next_state">{nextStates[detail.data.incident.state].map(v=><option value={v} key={v}>{states[v]}</option>)}</select></label><button type="submit">保存事件状态</button></fieldset></form>}
 {detail.data.incident.state!=='CLOSED'&&<form onSubmit={e=>void save(e,'events')}><fieldset disabled={busy}><h3>追加处理记录</h3><label>记录类型<select name="event_type" value={eventType} onChange={e=>setEventType(e.target.value)}>{Object.entries(eventNames).map(([v,l])=><option value={v} key={v}>{l}</option>)}</select></label>{eventType!=='COMMENT'&&<label>关联编号<input name="related_id" required maxLength={200}/></label>}<label>处理备注<textarea name="safe_summary" required={eventType==='COMMENT'} maxLength={1000}/></label><button type="submit">保存处理记录</button></fieldset></form>}</>}
 </Modal>}</>;
}
