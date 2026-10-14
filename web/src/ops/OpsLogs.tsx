import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { ApiClient, type Page } from '../api';
import { Alert, Loading, useResource } from '../ui';
import { FilterBar, useDebouncedValue } from './components/FilterBar';
import { DataTable, usePageSize, type Column } from './components/DataTable';
import { Drawer } from './components/Drawer';
import { OpsFacts, opsTime } from './OpsFacts';

type Summary={requests:number;attempts:number;errors:number;tokens:string;cost:string;error_rate:number};
type Log={id:number;user_id:number;username:string;display_name:string;key_id:number;key_name:string;model:string;channel_id:number;channel_name:string;group:string;created_at:string;tokens:string;cost:string;duration_ms:number;status_code:number;error:string;kind:number;request_id:string;attempt:number;prompt_tokens:string;completion_tokens:string;cache_read_tokens:string;cache_write_tokens:string;cache_write_tokens_1h:string;cost_units:string;is_stream:boolean};
type Logs=Page<Log>&{summary:Summary;top:(Summary&{name:string;display_name:string})[]};
type UserOption={id:number;username:string;master_name:string};
type ChannelOption={id:number;name:string;models:string[]};
const userLabel=(u:UserOption)=>`${u.master_name||u.username} · #${u.id}`;
const localTime=(d:Date)=>new Date(d.getTime()-d.getTimezoneOffset()*60000).toISOString().slice(0,19);

export function OpsLogs({client}:{client:ApiClient}){
 const [params]=useSearchParams(),[dimension,setDimension]=useState('model'),[selected,setSelected]=useState<Log>();
 const [pageSize,setPageSize]=usePageSize('momiao.ops.logs.pageSize');
 const [range,setRange]=useState('all');
 const [filters,setFilters]=useState({page:1,user:params.get('user')||'',key:'',model:'',channel:'',group:'',start:'',end:'',errors_only:false});
 const query=new URLSearchParams({...filters,page:String(filters.page),page_size:String(pageSize),dimension,start:filters.start?String(Math.floor(new Date(filters.start).getTime()/1000)):'',end:filters.end?String(Math.floor(new Date(filters.end).getTime()/1000)):'',errors_only:String(filters.errors_only)});
 const resource=useResource(()=>client.request<Logs>('/api/v1/ops/logs?'+query),[client,pageSize,dimension,filters]);
 const options=useResource(async()=>{
  const channels:ChannelOption[]=[];
  for(let page=1;;page++){
   const data=await client.request<Page<ChannelOption>>(`/api/v1/ops/channels?page=${page}&page_size=100`);
   channels.push(...data.items);if(channels.length>=data.total)break;
  }
  const groups=await client.request<{name:string}[]>('/api/v1/ops/groups');
  return {channels,groups,models:[...new Set(channels.flatMap(c=>c.models))].sort()};
 },[client]);
 useEffect(()=>{document.title=(selected?`调用详情 #${selected.id}`:'调用日志')+' · Chaldea Ops'},[selected]);
 function filter(key:string,value:string|boolean){setFilters(current=>({...current,[key]:value,page:1}))}
 function changeRange(value:string){
  setRange(value);
  if(value==='custom')return;
  const end=new Date(),start=new Date(end);
  if(value==='today')start.setHours(0,0,0,0);else if(value!=='all')start.setTime(end.getTime()-Number(value.replace('d',''))*86400000);
  setFilters(current=>({...current,page:1,start:value==='all'?'':localTime(start),end:value==='all'?'':localTime(end)}));
 }
 const columns:Column<Log>[]=[
  {key:'time',label:'时间 / 尝试',render:l=><><button className="ops-text-button" onClick={()=>setSelected(l)}>日志 #{l.id}</button><small>{opsTime(l.created_at)} · 第 {l.attempt} 次</small></>},
  {key:'user',label:'用户 / Key',render:l=><><Link to={'/ops/users/'+l.user_id}>{l.display_name||l.username||l.user_id}</Link><small>{l.key_name} · #{l.key_id}</small></>},
  {key:'model',label:'模型 / 分组',render:l=><>{l.model}<small>{l.group}</small></>},
  {key:'channel',label:'渠道',render:l=>l.channel_id?<Link to={'/ops/channels/'+l.channel_id}>{l.channel_name||'#'+l.channel_id}</Link>:'—'},
  {key:'cost',label:'Token / 费用',render:l=><>{l.tokens}<small>{l.cost} Credit</small></>},
  {key:'status',label:'耗时 / 状态',render:l=><>{l.duration_ms} ms / {l.status_code||'未记录'}<small>{l.kind===5?'错误':'消费'}{l.error?' · '+l.error:''}</small></>},
 ];
 return <><header className="page-heading"><div><p className="eyebrow">OPERATIONS / RELAY LOGS</p><h1>全局调用日志</h1><p>每行是一条渠道尝试；点开可查看同一请求的各次尝试。请求数合并重试，历史记录各算一次。</p></div><button onClick={resource.reload}>刷新日志</button></header>
 <section className="panel"><FilterBar>
  <UserFilter client={client} value={filters.user} onChange={value=>filter('user',value)}/>
  <label>Key ID<input type="number" min="1" value={filters.key} onChange={e=>filter('key',e.target.value)}/></label>
  <label>模型<select value={filters.model} onChange={e=>filter('model',e.target.value)}><option value="">全部模型</option>{options.data?.models.map(m=><option key={m}>{m}</option>)}</select></label>
  <label>渠道<select value={filters.channel} onChange={e=>filter('channel',e.target.value)}><option value="">全部渠道</option>{options.data?.channels.map(c=><option key={c.id} value={c.id}>{c.name} · #{c.id}</option>)}</select></label>
  <label>日志分组<select value={filters.group} onChange={e=>filter('group',e.target.value)}><option value="">全部分组</option>{options.data?.groups.map(g=><option key={g.name}>{g.name}</option>)}</select></label>
  <label>时间范围<select value={range} onChange={e=>changeRange(e.target.value)}><option value="all">全部时间</option><option value="today">今天</option><option value="1d">24 小时</option><option value="7d">7 天</option><option value="30d">30 天</option><option value="custom">自定义</option></select></label>
  {range==='custom'&&<><label>开始时间<input type="datetime-local" step="1" value={filters.start} max={filters.end||undefined} onChange={e=>filter('start',e.target.value)}/></label><label>结束时间<input type="datetime-local" step="1" min={filters.start||undefined} value={filters.end} onChange={e=>filter('end',e.target.value)}/></label></>}
  <label className="ops-checkbox"><input type="checkbox" checked={filters.errors_only} onChange={e=>filter('errors_only',e.target.checked)}/>仅错误</label>
 </FilterBar>{options.error&&<Alert>{options.error}</Alert>}{resource.error&&<Alert>{resource.error}</Alert>}</section>
 <details className="panel ops-log-top"><summary>调用 Top 20</summary><label>汇总维度<select value={dimension} onChange={e=>setDimension(e.target.value)}><option value="model">模型</option><option value="channel">渠道</option><option value="user">用户</option></select></label>{resource.data&&<div className="table-wrap"><table><thead><tr><th>{dimension==='model'?'模型':dimension==='channel'?'渠道':'用户'}</th><th>请求 / 尝试</th><th>Token</th><th>费用 Credit</th><th>错误率</th></tr></thead><tbody>{resource.data.top.map(v=><tr key={v.name}><td>{dimension==='user'?<Link to={'/ops/users/'+v.name}>{v.display_name||v.name}</Link>:dimension==='channel'&&v.name!=='0'?<Link to={'/ops/channels/'+v.name}>{v.display_name||v.name}</Link>:v.name}</td><td>{v.requests} / {v.attempts}</td><td>{v.tokens}</td><td>{v.cost}</td><td>{(v.error_rate*100).toFixed(2)}%</td></tr>)}</tbody></table></div>}</details>
 <section className="panel">{resource.loading?<Loading/>:resource.data&&<><p role="status">请求 {resource.data.summary.requests} · 尝试 {resource.data.summary.attempts} · Token {resource.data.summary.tokens} · 费用 {resource.data.summary.cost} Credit · 错误率 {(resource.data.summary.error_rate*100).toFixed(2)}%</p><DataTable columns={columns} rows={resource.data.items} rowKey={l=>String(l.id)} page={filters.page} total={resource.data.total} pageSize={pageSize} onPageChange={page=>setFilters(current=>({...current,page}))} onPageSizeChange={size=>{setPageSize(size);setFilters(current=>({...current,page:1}))}} onRowClick={setSelected}/></>}</section>
 {selected&&<LogDetail client={client} log={selected} onClose={()=>setSelected(undefined)}/>}</>;
}

function UserFilter({client,value,onChange}:{client:ApiClient;value:string;onChange:(id:string)=>void}){
 const [search,setSearch]=useState(''),[open,setOpen]=useState(false),settled=useDebouncedValue(search);
 const selected=useResource(()=>value?client.request<UserOption>('/api/v1/ops/users/'+value):Promise.resolve(undefined),[client,value]);
 const matches=useResource(()=>open&&settled.trim()?client.request<Page<UserOption>>('/api/v1/ops/users?'+new URLSearchParams({q:settled,page_size:'20'})):Promise.resolve(undefined),[client,open,settled]);
 useEffect(()=>{if(selected.data){setSearch(userLabel(selected.data));setOpen(false)}},[selected.data]);
 return <div className="ops-user-search"><label>搜索用户<input maxLength={100} autoComplete="off" value={search} placeholder="Master 显示名或登录名" onFocus={()=>{if(!value)setOpen(true)}} onChange={e=>{setSearch(e.target.value);setOpen(true);if(value)onChange('')}} onKeyDown={e=>{if(e.key==='Escape')setOpen(false)}}/></label>
 {search&&<button type="button" className="ops-user-clear" aria-label="清除用户筛选" onClick={()=>{setSearch('');setOpen(false);onChange('')}}>×</button>}
 {open&&settled.trim()&&<div className="ops-user-matches">{matches.loading||search!==settled?<Loading/>:matches.error?<Alert>{matches.error}</Alert>:matches.data?.items.length?<ul aria-label="匹配用户">{matches.data.items.map(u=><li key={u.id}><button type="button" onClick={()=>{setSearch(userLabel(u));setOpen(false);onChange(String(u.id))}}>{userLabel(u)}</button></li>)}</ul>:<p>没有匹配的用户</p>}</div>}
 {selected.error&&<Alert>{selected.error}</Alert>}</div>;
}

function LogDetail({client,log,onClose}:{client:ApiClient;log:Log;onClose:()=>void}){
 const detail=useResource(()=>client.request<{items:Log[]}>('/api/v1/ops/logs/'+log.id),[client,log.id]);
 return <Drawer title={'调用详情 #'+log.id} onClose={onClose}>{detail.loading?<Loading/>:detail.error?<Alert>{detail.error}</Alert>:detail.data?.items.map(l=><section className="ops-form-section" key={l.id}><h3>第 {l.attempt} 次尝试</h3><OpsFacts value={{
  '日志 ID':l.id,'请求 ID':l.request_id||'历史独立记录','用户':l.display_name||l.username,'登录名':l.username,'用户 ID':l.user_id,'Key ID':l.key_id,'Key 名称':l.key_name,'模型':l.model,'渠道 ID':l.channel_id,'渠道':l.channel_name,'分组':l.group,'时间':l.created_at,
  '日志类型':l.kind===5?'错误 (5)':'消费 (2)','输入 Token':l.prompt_tokens,'输出 Token':l.completion_tokens,'总 Token':l.tokens,'缓存读取 Token':l.cache_read_tokens,'缓存写入 Token':l.cache_write_tokens,'1h 缓存写入 Token':l.cache_write_tokens_1h,
  '费用 Credit':l.cost,'费用原始单位':l.cost_units,'流式':l.is_stream,'耗时 ms':l.duration_ms,'状态码':l.status_code||'未记录','错误信息':l.error||'无',
 }}/></section>)}</Drawer>;
}
