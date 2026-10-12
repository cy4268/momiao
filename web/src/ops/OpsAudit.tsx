import { useEffect, useState, type FormEvent } from 'react';
import { useNavigate, useOutletContext, useParams } from 'react-router-dom';
import { ApiClient } from '../api';
import { Alert, Empty, Loading, Modal, Pager, useResource } from '../ui';

type Audit={audit_id:string;actor_newapi_user_id:string;actor_name:string;action:string;target_type:string;target_id:string;before_snapshot:unknown;after_snapshot:unknown;occurred_at:string};
type AuditPage={items:Audit[];total:number;page:number;page_size:number};
export function OpsAudit(){
 const {client}=useOutletContext<{client:ApiClient}>(),{auditID}=useParams(),navigate=useNavigate();
 const [page,setPage]=useState(1),[query,setQuery]=useState(''),[selected,setSelected]=useState<Audit>();
 const resource=useResource(()=>client.request<AuditPage>(`/api/v1/ops/audit?page=${page}&page_size=20${query}${auditID?'&audit_id='+encodeURIComponent(auditID):''}`),[client,page,query,auditID]);
 useEffect(()=>{if(auditID&&resource.data)setSelected(resource.data.items[0])},[auditID,resource.data]);
 function filter(event:FormEvent<HTMLFormElement>){
  event.preventDefault();const data=new FormData(event.currentTarget),params=new URLSearchParams();
  for(const [name,value] of data){const text=String(value).trim();if(text)params.set(name,name==='from'||name==='to'?new Date(text).toISOString():text)}
  setPage(1);setQuery(params.size?'&'+params.toString():'');if(auditID)navigate('/ops/audit');
 }
 function close(){setSelected(undefined);if(auditID)navigate('/ops/audit')}
 return <><header className="page-heading ops-system-heading"><div><p className="eyebrow">OPERATIONS / AUDIT</p><h1>操作日志</h1><p>查看全部管理变更，包括迁移前保留的审计记录。</p></div><button onClick={resource.reload} disabled={resource.loading}>刷新记录</button></header>
 <section className="panel ops-system-list"><form className="filters" onSubmit={filter}>
 <label>操作人 ID<input name="actor_user_id" inputMode="numeric"/></label><label>对象类型<input name="target_type" placeholder="例如 MAINTENANCE"/></label><label>对象 ID<input name="target_id"/></label><label>操作<input name="action"/></label>
 <label>开始时间<input name="from" type="datetime-local"/></label><label>结束时间（不含）<input name="to" type="datetime-local"/></label><button type="submit">筛选</button></form>
 {resource.loading?<Loading/>:resource.error?<Alert>{resource.error}</Alert>:resource.data&&<>{resource.data.items.length?<div className="table-wrap" role="region" aria-label="操作日志" tabIndex={0}><table><thead><tr><th>时间</th><th>操作人</th><th>操作</th><th>对象</th><th>详情</th></tr></thead><tbody>{resource.data.items.map(item=><tr key={item.audit_id}><td>{new Date(item.occurred_at).toLocaleString('zh-CN')}</td><td>{item.actor_name} · {item.actor_newapi_user_id}</td><td>{item.action}</td><td>{item.target_type} · {item.target_id}</td><td><button onClick={()=>setSelected(item)}>查看详情</button></td></tr>)}</tbody></table></div>:<Empty title="没有匹配的操作日志">调整筛选条件后重试。</Empty>}<Pager page={page} size={20} total={resource.data.total} onChange={setPage}/></>}
 </section>{selected&&<Modal title="变更详情" onClose={close}><p>{selected.action} · {selected.target_type} · {selected.target_id}</p><p>操作人：{selected.actor_name} · {selected.actor_newapi_user_id}</p><div className="ops-system-snapshot-grid"><section><h3>变更前</h3><pre>{JSON.stringify(selected.before_snapshot,null,2)}</pre></section><section><h3>变更后</h3><pre>{JSON.stringify(selected.after_snapshot,null,2)}</pre></section></div></Modal>}</>;
}
