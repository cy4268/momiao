import { useEffect, useId, useState, type FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { ApiClient, errorText, type Page } from './api';
import { Alert, Loading, Modal, Pager, useResource } from './ui';
import { FilterBar, useDebouncedValue } from './ops/components/FilterBar';
import { DataTable, usePageSize, type Column } from './ops/components/DataTable';
import { Drawer } from './ops/components/Drawer';
import { KeyValueEditor } from './ops/components/KeyValueEditor';
import { TagInput } from './ops/components/TagInput';

const root='/api/v1/ops';
type Group={name:string;description:string;ratio:string};
type Channel={id:number;name:string;type:number;base_url:string;models:string[];groups:string[];model_mapping:Record<string,string>;header_override:Record<string,string>;priority:number;weight:number;enabled:boolean;key_configured:boolean;requests_24h:number;errors_24h:number;error_rate:number};
type Availability={model:string;channels:Pick<Channel,'id'|'name'|'groups'|'enabled'>[]};
type TestResult={ok:boolean;model:string;status_code:number;duration_ms:number;message:string};
const types=[[1,'OpenAI 兼容'],[14,'Anthropic'],[24,'Gemini'],[41,'Vertex AI'],[43,'DeepSeek']] as const;
const empty:Channel={id:0,name:'',type:1,base_url:'',models:[],groups:[],model_mapping:{},header_override:{},priority:0,weight:0,enabled:true,key_configured:false,requests_24h:0,errors_24h:0,error_rate:0};

export function Channels({client}:{client:ApiClient}){
 const [filter,setFilter]=useState({q:'',page:1,group:'',enabled:''}),[search,setSearch]=useState('');
 const {q,page,group,enabled}=filter;
 const settledSearch=useDebouncedValue(search);
 const setPage=(page:number)=>setFilter(current=>({...current,page}));
 useEffect(()=>{setFilter(current=>({...current,q:settledSearch,page:1}))},[settledSearch]);
 const [pageSize,setPageSize]=usePageSize('momiao.ops.channels.pageSize');
 const [editing,setEditing]=useState<Channel>(),[busy,setBusy]=useState(false),[error,setError]=useState(''),[notice,setNotice]=useState('');
 const pending=busy||search!==q;
 const {channelID}=useParams(),navigate=useNavigate();
 const resource=useResource(()=>client.request<Page<Channel>>(root+'/channels?'+new URLSearchParams({page:String(page),page_size:String(pageSize),q,group,enabled})),[client,page,pageSize,q,group,enabled]);
 const groups=useResource(()=>client.request<Group[]>(root+'/groups'),[client]);
 const detail=useResource(()=>channelID?client.request<Channel>(root+'/channels/'+channelID):Promise.resolve(undefined),[client,channelID]);
 const editor=editing||(channelID?detail.data:undefined);
 useEffect(()=>{document.title=(editor?editor.id?'编辑渠道':'新建渠道':'渠道')+' · Chaldea Ops'},[editor]);
 function updateRow(next:Channel){
  const list=resource.data!;
  resource.setData({...list,items:list.items.map(c=>c.id===next.id?{...next,requests_24h:c.requests_24h,errors_24h:c.errors_24h,error_rate:c.error_rate}:c)});
 }
 async function action(channel:Channel,kind:'status'|'delete'|'test'){
  if(kind==='delete'&&!window.confirm('删除渠道“'+channel.name+'”？历史调用日志会保留。'))return;
  setBusy(true);setError('');setNotice('');
  try{
   if(kind==='test'){const v=await client.request<TestResult>(root+'/channels/'+channel.id+'/test','POST',{});setNotice(`${v.model} · ${v.message} · ${v.duration_ms} ms · HTTP ${v.status_code||'—'}`)}
   else{
    await client.request(root+'/channels/'+channel.id+(kind==='status'?'/status':''),kind==='status'?'PUT':'DELETE',kind==='status'?{enabled:!channel.enabled}:undefined);
    setNotice(kind==='delete'?'渠道已删除':'状态已保存');
    if(kind==='status')updateRow({...channel,enabled:!channel.enabled});else resource.reload();
   }
  }catch(e){setError(errorText(e))}finally{setBusy(false)}
 }
 function close(){setEditing(undefined);if(channelID)navigate('/ops/channels')}
 const columns:Column<Channel>[]=[
  {key:'id',label:'ID',render:c=>c.id},
  {key:'name',label:'名称 / 类型',render:c=><><strong>{c.name}</strong><small>{types.find(([id])=>id===c.type)?.[1]||c.type}</small></>},
  {key:'status',label:'状态',render:c=><span className={'badge'+(c.enabled?' active':'')}>{c.enabled?'启用':'停用'}</span>},
  {key:'groups',label:'分组',render:c=>c.groups.join('、')||'无分组'},
  {key:'models',label:'模型数',render:c=><span title={c.models.join('\n')}>{c.models.length}</span>},
  {key:'priority',label:'优先级 / 权重',render:c=>c.priority+' / '+c.weight},
  {key:'stats',label:'24h 调用 / 错误率',render:c=>c.requests_24h+' / '+(c.error_rate*100).toFixed(1)+'%'},
  {key:'actions',label:'操作',render:c=><div className="row-actions"><button disabled={pending} onClick={()=>setEditing(c)}>编辑</button><button disabled={pending} onClick={()=>void action(c,'status')}>{c.enabled?'停用':'启用'}</button><button disabled={pending} onClick={()=>void action(c,'test')}>测试</button><button disabled={pending} onClick={()=>void action(c,'delete')}>删除</button></div>},
 ];
 return <><header className="page-heading"><div><p className="eyebrow">OPERATIONS / CHANNELS</p><h1>渠道管理</h1><p>本地配置直接生效；调用统计为最近 24 小时的渠道尝试。</p></div><button onClick={()=>setEditing(empty)} disabled={pending||resource.loading||groups.loading}>新建渠道</button></header>
 <section className="panel"><FilterBar>
  <label>搜索渠道或模型<input disabled={busy} value={search} onChange={e=>setSearch(e.target.value)}/></label>
  <label>分组<select disabled={pending} value={group} onChange={e=>setFilter({...filter,group:e.target.value,page:1})}><option value="">全部分组</option>{groups.data?.map(g=><option key={g.name}>{g.name}</option>)}</select></label>
  <label>状态<select disabled={pending} value={enabled} onChange={e=>setFilter({...filter,enabled:e.target.value,page:1})}><option value="">全部状态</option><option value="true">启用</option><option value="false">停用</option></select></label>
  <button onClick={()=>{resource.reload();groups.reload()}} disabled={pending}>刷新列表</button><Link to="/ops/groups">分组与可用性</Link>
 </FilterBar>
 {notice&&<p role="status">{notice}</p>}{error&&<Alert>{error}</Alert>}{resource.error&&<Alert>{resource.error}</Alert>}{groups.error&&<Alert>{groups.error}</Alert>}
 {resource.loading?<Loading label="加载中"/>:resource.data&&<DataTable columns={columns} rows={resource.data.items} rowKey={c=>String(c.id)} page={page} total={resource.data.total} pageSize={pageSize} onPageChange={setPage} onPageSizeChange={size=>{setPageSize(size);setPage(1)}} disabled={pending}/>}</section>
 {channelID&&detail.error&&<Alert>{detail.error}</Alert>}{channelID&&detail.loading&&<Loading label="加载中"/>}
 {editor&&groups.data&&<ChannelEditor key={editor.id} client={client} channel={editor} groups={groups.data} onClose={close} onSaved={saved=>{setNotice('渠道已保存');if(editor.id&&resource.data)updateRow(saved);else resource.reload();close()}}/>}</>;
}
function ChannelEditor({client,channel,groups,onClose,onSaved}:{client:ApiClient;channel:Channel;groups:Group[];onClose:()=>void;onSaved:(channel:Channel)=>void}){
 const formId=useId();
 const [v,setV]=useState(channel),[mapping,setMapping]=useState(JSON.stringify(channel.model_mapping,null,2)),[headers,setHeaders]=useState(JSON.stringify(channel.header_override,null,2)),[key,setKey]=useState('');
 const [busy,setBusy]=useState(false),[error,setError]=useState(''),[notice,setNotice]=useState('');
 async function save(e:FormEvent){
  e.preventDefault();setBusy(true);setError('');
  try{
   const saved=await client.request<Channel>(root+'/channels','PUT',{id:v.id,name:v.name,type:v.type,base_url:v.base_url,models:v.models,groups:v.groups,model_mapping:JSON.parse(mapping),header_override:JSON.parse(headers),priority:v.priority,weight:v.weight,enabled:v.enabled,...(key.trim()?{key:key.trim()}:{})});
   setKey('');onSaved(saved);
  }catch(e){setError(errorText(e))}finally{setBusy(false)}
 }
 async function pull(){setBusy(true);setError('');try{const items=await client.request<string[]>(root+'/channels/'+channel.id+'/models','POST',{});setV({...v,models:items});setNotice('已拉取 '+items.length+' 个模型；保存后生效。')}catch(e){setError(errorText(e))}finally{setBusy(false)}}
 return <Drawer title={channel.id?'编辑渠道':'新建渠道'} busy={busy} onClose={onClose} footer={<><button type="button" disabled={busy} onClick={onClose}>取消</button><button type="submit" form={formId} className="primary" disabled={busy}>保存渠道</button></>}>
  <form id={formId} onSubmit={save}><fieldset disabled={busy} className="plain-fieldset">
   <section className="ops-form-section"><h3>基本信息</h3><div className="ops-form-grid">
    <label>渠道名称<input required value={v.name} onChange={e=>setV({...v,name:e.target.value})}/></label>
    <label>类型<select value={v.type} onChange={e=>setV({...v,type:Number(e.target.value)})}>{types.map(([id,name])=><option key={id} value={id}>{name}</option>)}</select></label>
    <label><input type="checkbox" checked={v.enabled} onChange={e=>setV({...v,enabled:e.target.checked})}/>启用渠道</label>
   </div></section>
   <section className="ops-form-section"><h3>连接</h3><div className="ops-form-grid">
    <label>基础 URL<input type="url" placeholder="留空使用官方默认地址" value={v.base_url} onChange={e=>setV({...v,base_url:e.target.value})}/></label>
    <label>上游密钥（留空保留）<textarea rows={2} autoComplete="off" placeholder={channel.key_configured?'已配置，留空不修改':'填写上游密钥'} required={!channel.id} value={key} onChange={e=>setKey(e.target.value)}/></label>
   </div><p className="hint">已导入的参数、多 Key、Vertex 设置保持不变。</p></section>
   <section className="ops-form-section"><h3>模型与分组</h3>
    <TagInput label="模型列表" values={v.models} onChange={models=>setV({...v,models})}/>
    <div className="row-actions"><button type="button" disabled={!channel.id} onClick={()=>void pull()}>从上游拉取模型</button><button type="button" onClick={()=>{void navigator.clipboard.writeText(v.models.join('\n')).then(()=>setNotice('模型列表已复制')).catch(e=>setError(errorText(e)))}}>复制全部</button><button type="button" onClick={()=>{if(window.confirm('清空当前表单的模型列表？保存后生效。'))setV({...v,models:[]})}}>清空</button></div>
    <p className="hint">拉取使用已保存的地址和密钥；新渠道请先保存。测试消耗上游额度，不扣用户 Reserve。</p>
    <TagInput label="渠道分组" values={v.groups} options={groups.map(g=>g.name)} onChange={groups=>setV({...v,groups})}/>
   </section>
   <section className="ops-form-section"><h3>路由与覆盖</h3><div className="ops-form-grid">
    <label>优先级<input type="number" step="1" value={v.priority} onChange={e=>setV({...v,priority:Number(e.target.value)})}/></label><label>权重<input type="number" min="0" max="1000000" step="1" value={v.weight} onChange={e=>setV({...v,weight:Number(e.target.value)})}/></label>
   </div><KeyValueEditor label="模型映射" value={mapping} onChange={setMapping} leftLabel="请求模型" rightLabel="上游模型" options={v.models}/><KeyValueEditor label="额外请求头" value={headers} onChange={setHeaders}/></section>
  </fieldset>{error&&<Alert>{error}</Alert>}{notice&&<p role="status">{notice}</p>}</form>
 </Drawer>;
}

export function Groups({client}:{client:ApiClient}){
 const [page,setPage]=useState(1),[q,setQ]=useState(''),[editing,setEditing]=useState<(Group&{original_name:string})>(),[busy,setBusy]=useState(false),[error,setError]=useState(''),[notice,setNotice]=useState('');
 const resource=useResource(()=>Promise.all([client.request<Group[]>(root+'/groups'),client.request<Page<Availability>>(root+'/groups/availability?'+new URLSearchParams({page:String(page),page_size:'20',q}))]),[client,page,q]);
 async function save(e:FormEvent){e.preventDefault();setBusy(true);setError('');try{await client.request(root+'/groups','PUT',editing);setEditing(undefined);setNotice('分组已保存');resource.reload()}catch(e){setError(errorText(e))}finally{setBusy(false)}}
 async function remove(g:Group){if(!window.confirm('删除分组“'+g.name+'”？'))return;setBusy(true);setError('');try{await client.request(root+'/groups','DELETE',{name:g.name});setNotice('分组已删除');resource.reload()}catch(e){setError(errorText(e))}finally{setBusy(false)}}
 return <><header className="page-heading"><div><p className="eyebrow">OPERATIONS / GROUPS</p><h1>分组与可用性</h1><p>倍率用于目录报价与 Reserve 计费。重命名同步更新用户与渠道引用。</p></div><button disabled={busy} onClick={()=>setEditing({name:'',description:'',ratio:'1',original_name:''})}>新建分组</button></header>{notice&&<p role="status">{notice}</p>}{error&&<Alert>{error}</Alert>}{resource.error&&<Alert>{resource.error}</Alert>}<label>搜索模型<input value={q} onChange={e=>{setQ(e.target.value);setPage(1)}}/></label><button disabled={busy} onClick={resource.reload}>刷新</button>{resource.loading?<Loading/>:resource.data&&<><section className="panel"><table><thead><tr><th>名称</th><th>描述</th><th>倍率</th><th>操作</th></tr></thead><tbody>{resource.data[0].map(g=><tr key={g.name}><td>{g.name}</td><td>{g.description}</td><td>{g.ratio}</td><td><button disabled={busy} onClick={()=>setEditing({...g,original_name:g.name})}>编辑 {g.name}</button><button disabled={busy||g.name==='default'} onClick={()=>void remove(g)}>删除 {g.name}</button></td></tr>)}</tbody></table></section><section className="panel"><h2>模型可用性矩阵</h2><p className="hint">按启用渠道的模型与分组判断配置可达，不代表上游健康、价格已配置或 Key 额度。灰格可打开相关渠道补充分组。</p><div className="table-wrap" role="region" aria-label="模型分组可用性" tabIndex={0}><table><thead><tr><th>模型</th>{resource.data[0].map(g=><th key={g.name}>{g.name}</th>)}</tr></thead><tbody>{resource.data[1].items.map(m=><tr key={m.model}><th>{m.model}</th>{resource.data![0].map(g=>{const active=m.channels.filter(c=>c.enabled&&c.groups.includes(g.name));return <td key={g.name}><span className={'badge'+(active.length?' active':'')}>{active.length?'可用':'不可用'}</span>{(active.length?active:m.channels).map(c=><small key={c.id}><Link to={'/ops/channels/'+c.id}>{c.name}{active.length?'':c.enabled?' · 未分配':' · 已停用'}</Link></small>)}</td>})}</tr>)}</tbody></table></div><Pager page={page} total={resource.data[1].total} size={20} onChange={setPage} disabled={busy}/></section></>}
 {editing&&<Modal title={editing.original_name?'编辑分组':'新建分组'} busy={busy} onClose={()=>setEditing(undefined)}><form onSubmit={save}><fieldset disabled={busy}><label>分组名称<input required readOnly={editing.original_name==='default'} value={editing.name} onChange={e=>setEditing({...editing,name:e.target.value})}/></label><label>描述<input value={editing.description} onChange={e=>setEditing({...editing,description:e.target.value})}/></label><label>倍率<input type="number" min="0" step="any" required value={editing.ratio} onChange={e=>setEditing({...editing,ratio:e.target.value})}/></label><button type="submit">保存分组</button></fieldset>{error&&<Alert>{error}</Alert>}</form></Modal>}</>;
}
