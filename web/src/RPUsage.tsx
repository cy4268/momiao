import { useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { ApiClient, ApiError } from './api';
import { Alert, Empty, Loading, useResource } from './ui';

type Item = { logical_request_id:string;token_id:string;model_id:string;model_name:string;request_kind:string;provider_attempt_count:number;final_status:string;error_category:string;charged_raw_quota:string;charged_amount:string;requested_at:string;completed_at:string };
type Page = {items:Item[];page:number;page_size:number;total:string;has_more:boolean;observed_at:string};
const statuses:Record<string,string>={SUCCESS:'成功',ERROR:'失败',CANCELLED_POST_UPSTREAM:'请求发出后取消'};
const timestamp=(value:string)=>new Date(value).toLocaleString('zh-CN');
async function read(client:ApiClient,page:number,query:string):Promise<Page>{
 const value=await client.request<Page>(`/api/v1/usage/rp?page=${page}${query?'&'+query:''}`);
 if(!value||!Array.isArray(value.items)||value.page!==page||value.page_size!==50||typeof value.total!=='string'||!/^\d+$/.test(value.total)||typeof value.has_more!=='boolean'||!Number.isFinite(Date.parse(value.observed_at)))throw new ApiError('RP 用量响应格式异常。');
 for(const item of value.items){if(!item||typeof item.logical_request_id!=='string'||typeof item.token_id!=='string'||!/^\d+$/.test(item.token_id)||typeof item.model_name!=='string'||typeof item.model_id!=='string'||typeof item.request_kind!=='string'||typeof item.error_category!=='string'||!Object.hasOwn(statuses,item.final_status)||!Number.isInteger(item.provider_attempt_count)||item.provider_attempt_count<0||typeof item.charged_amount!=='string'||!/^\d+(\.\d+)?$/.test(item.charged_amount)||!Number.isFinite(Date.parse(item.requested_at))||!Number.isFinite(Date.parse(item.completed_at)))throw new ApiError('RP 用量记录格式异常。');}
 return value;
}
export function RPUsage({client}:{client:ApiClient}){
 const [page,setPage]=useState(1),[query,setQuery]=useState('');
 const [model,setModel]=useState(''),[status,setStatus]=useState(''),[from,setFrom]=useState(''),[to,setTo]=useState(''),[error,setError]=useState('');
 const resource=useResource(()=>read(client,page,query),[client,page,query]);
 function filter(event:FormEvent){event.preventDefault();if(from&&to&&from>to){setError('结束日期应不早于开始日期。');return;}setError('');const params=new URLSearchParams();if(model.trim())params.set('model',model.trim());if(status)params.set('status',status);if(from)params.set('from',new Date(`${from}T00:00:00+08:00`).toISOString());if(to){const end=new Date(`${to}T00:00:00+08:00`);end.setUTCDate(end.getUTCDate()+1);params.set('to',end.toISOString());}setQuery(params.toString());setPage(1);}
 function reset(){setModel('');setStatus('');setFrom('');setTo('');setQuery('');setPage(1);setError('');}
 return <><header className="page-heading"><div><p className="eyebrow">ROLEPLAY / USAGE</p><h1>RP 调用记录</h1><p>按请求发生时的密钥用途记录，查看已完成的 RP 调用。</p></div><button onClick={resource.reload} disabled={resource.loading}>刷新记录</button></header>
 <nav className="filter-actions" aria-label="调用记录类别"><Link to="/logs">全部调用记录</Link><Link to="/rankings?metric=RP_CALLS">RP 排行榜</Link></nav>
 <section className="panel"><form className="filters" onSubmit={filter}><label>模型 ID<input value={model} onChange={e=>setModel(e.target.value)} maxLength={128} placeholder="完整模型 ID"/></label><label>结果<select value={status} onChange={e=>setStatus(e.target.value)}><option value="">全部结果</option>{Object.entries(statuses).map(([key,label])=><option key={key} value={key}>{label}</option>)}</select></label><label>开始日期<input type="date" value={from} onChange={e=>setFrom(e.target.value)}/></label><label>结束日期<input type="date" value={to} onChange={e=>setTo(e.target.value)}/></label><div className="filter-actions"><button className="primary" type="submit">应用筛选</button><button type="button" onClick={reset}>重置</button></div></form>
 {error&&<Alert>{error}</Alert>}<p className="hint">筛选日期按北京时间计算，记录时间按设备时区显示。仅含启用归因后进入模型流程并已结束的 RP 请求；密钥用途变更不改写历史。</p>
 {resource.loading?<Loading/>:resource.error?<><Alert>{resource.error}</Alert><p className="hint">同步未就绪时暂不展示用量，请稍后重新加载。</p><button onClick={resource.reload}>重新加载</button></>:resource.data&&<><p className="hint">同步时间：{timestamp(resource.data.observed_at)} · 共 {resource.data.total} 条</p>{resource.data.items.length===0?<Empty title="暂无符合条件的 RP 记录">可以调整筛选，或在 RP 密钥完成调用后刷新。</Empty>:<div className="table-wrap" role="region" aria-label="个人 RP 调用记录" tabIndex={0}><table><thead><tr><th>时间 / 请求</th><th>模型 / 密钥</th><th>结果</th><th>上游尝试</th><th>API Credit 消耗</th></tr></thead><tbody>{resource.data.items.map(item=><tr key={item.logical_request_id}><td><time dateTime={item.requested_at}>{timestamp(item.requested_at)}</time><small>{item.request_kind} · {item.logical_request_id}</small></td><td><strong>{item.model_name||item.model_id||'未指定模型'}</strong><small>密钥 {item.token_id}</small></td><td>{statuses[item.final_status]}{item.error_category&&<small>{item.error_category}</small>}</td><td>{item.provider_attempt_count}</td><td className="numeric">{item.charged_amount}</td></tr>)}</tbody></table></div>}<nav className="filter-actions" aria-label="RP 记录分页"><button disabled={page<=1} onClick={()=>setPage(page-1)}>上一页</button><span>第 {page} 页</span><button disabled={!resource.data.has_more} onClick={()=>setPage(page+1)}>下一页</button></nav></>}
 </section></>;
}
