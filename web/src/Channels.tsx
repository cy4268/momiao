// Channel controls adapted from QuantumNous/new-api@f1164142 features/channels.
// Copyright QuantumNous and contributors. AGPL-3.0-or-later.
import { useEffect, useId, useRef, useState, type FormEvent } from 'react';
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
type Channel={copy_from?:number;id:number;name:string;type:number;base_url:string;models:string[];groups:string[];model_mapping:Record<string,string>;header_override:Record<string,string>;priority:number;weight:number;enabled:boolean;key_configured:boolean;requests_24h:number;errors_24h:number;error_rate:number;remark:string;test_model:string;test_at:string|null;response_ms:number};
type Availability={model:string;channels:Pick<Channel,'id'|'name'|'groups'|'enabled'>[]};
type TestResult={ok:boolean;model:string;status_code:number;duration_ms:number;message:string;test_at:string;upstream_model:string;max_tokens:number};
const types=[[1,'OpenAI 兼容'],[14,'Anthropic'],[24,'Gemini'],[41,'Vertex AI'],[43,'DeepSeek']] as const;
const empty:Channel={id:0,name:'',type:1,base_url:'',models:[],groups:[],model_mapping:{},header_override:{},priority:0,weight:0,enabled:true,key_configured:false,requests_24h:0,errors_24h:0,error_rate:0,remark:'',test_model:'',test_at:null,response_ms:0};

export function Channels({client}:{client:ApiClient}){
 const [filter,setFilter]=useState({q:'',page:1,group:'',enabled:'',type:''}),[search,setSearch]=useState('');
 const {q,page,group,enabled,type}=filter;
 const settledSearch=useDebouncedValue(search);
 const setPage=(page:number)=>setFilter(current=>({...current,page}));
 useEffect(()=>{setFilter(current=>({...current,q:settledSearch,page:1}))},[settledSearch]);
 const [pageSize,setPageSize]=usePageSize('momiao.ops.channels.pageSize');
 const [editing,setEditing]=useState<Channel>(),[busy,setBusy]=useState(false),[error,setError]=useState(''),[notice,setNotice]=useState('');
 const [testing,setTesting]=useState<Channel>(),[selected,setSelected]=useState<string[]>([]),[pullOnOpen,setPullOnOpen]=useState(false);
 const pending=busy||search!==q;
 const {channelID}=useParams(),navigate=useNavigate();
 const resource=useResource(()=>client.request<Page<Channel>>(root+'/channels?'+new URLSearchParams({page:String(page),page_size:String(pageSize),q,group,enabled,type})),[client,page,pageSize,q,group,enabled,type]);
 useEffect(()=>setSelected([]),[page,pageSize,q,group,enabled,type]);
 const groups=useResource(()=>client.request<Group[]>(root+'/groups'),[client]);
 const detail=useResource(()=>channelID?client.request<Channel>(root+'/channels/'+channelID):Promise.resolve(undefined),[client,channelID]);
 const editor=editing||(channelID?detail.data:undefined);
 useEffect(()=>{document.title=(editor?editor.id?'编辑渠道':'新建渠道':'渠道')+' · Chaldea Ops'},[editor]);
 function updateRow(next:Channel){
  const list=resource.data!;
  resource.setData({...list,items:list.items.map(c=>c.id===next.id?{...next,requests_24h:c.requests_24h,errors_24h:c.errors_24h,error_rate:c.error_rate}:c)});
 }
 async function action(channel:Channel,kind:'status'|'delete'){
  if(kind==='delete'&&!window.confirm('删除渠道“'+channel.name+'”？历史调用日志会保留。'))return;
  setBusy(true);setError('');setNotice('');
  try{
    await client.request(root+'/channels/'+channel.id+(kind==='status'?'/status':''),kind==='status'?'PUT':'DELETE',kind==='status'?{enabled:!channel.enabled}:undefined);
    setNotice(kind==='delete'?'渠道已删除':'状态已保存');
    if(kind==='status')updateRow({...channel,enabled:!channel.enabled});else resource.reload();
  }catch(e){setError(errorText(e))}finally{setBusy(false)}
 }
 async function patch(channel:Channel,field:'priority'|'weight',value:number){
  setBusy(true);setError('');
  try{updateRow(await client.request<Channel>(root+'/channels/'+channel.id,'PATCH',{[field]:value}));setNotice('渠道路由已保存')}
  catch(e){setError(errorText(e));throw e}finally{setBusy(false)}
 }
 async function batch(kind:'enable'|'disable'|'delete'){
  if(kind==='delete'&&!window.confirm(`删除已选的 ${selected.length} 个渠道？历史调用日志会保留。`))return;
  setBusy(true);setError('');let done=0;
  try{for(const id of selected){await client.request(root+'/channels/'+id+(kind==='delete'?'':'/status'),kind==='delete'?'DELETE':'PUT',kind==='delete'?undefined:{enabled:kind==='enable'});done++}}
  catch(e){setError(errorText(e))}finally{setNotice(`已处理 ${done} / ${selected.length} 个渠道`);setSelected([]);setBusy(false);resource.reload()}
 }
 function close(){setEditing(undefined);setPullOnOpen(false);if(channelID)navigate('/ops/channels')}
 const columns:Column<Channel>[]=[
  {key:'id',label:'ID',render:c=>c.id},
  {key:'name',label:'名称 / 类型',render:c=><><strong>{c.name}</strong><small>{types.find(([id])=>id===c.type)?.[1]||c.type}</small></>},
  {key:'status',label:'状态',render:c=><span className={'badge'+(c.enabled?' active':'')}>{c.enabled?'启用':'停用'}</span>},
  {key:'groups',label:'分组',render:c=>c.groups.join('、')||'无分组'},
  {key:'models',label:'模型数',render:c=><span title={c.models.join('\n')}>{c.models.length}</span>},
  {key:'priority',label:'优先级 / 权重',render:c=><div className="ops-inline-numbers"><ChannelNumber label={`优先级 ${c.id}`} value={c.priority} disabled={pending} save={v=>patch(c,'priority',v)}/><ChannelNumber label={`权重 ${c.id}`} value={c.weight} min={0} max={1000000} disabled={pending} save={v=>patch(c,'weight',v)}/></div>},
  {key:'test',label:'最近测试',render:c=>c.test_at?<span title={new Date(c.test_at).toLocaleString()}>{c.response_ms} ms<small>{Math.max(0,Math.floor((Date.now()-Date.parse(c.test_at))/60000))} 分钟前</small></span>:'未测试'},
  {key:'stats',label:'24h 调用 / 错误率',render:c=>c.requests_24h+' / '+(c.error_rate*100).toFixed(1)+'%'},
  {key:'actions',label:'操作',render:c=><div className="row-actions"><button disabled={pending} onClick={()=>setEditing(c)}>编辑</button><button disabled={pending} onClick={()=>setTesting(c)}>测试</button><details className="ops-row-menu"><summary>更多</summary><div><button disabled={pending} onClick={()=>void action(c,'status')}>{c.enabled?'停用':'启用'}</button><button disabled={pending} onClick={()=>setEditing({...c,id:0,copy_from:c.id,name:c.name+' 副本',test_at:null,response_ms:0})}>复制渠道</button><button disabled={pending} onClick={()=>{setPullOnOpen(true);setEditing(c)}}>拉取模型</button><button disabled={pending} onClick={()=>void action(c,'delete')}>删除</button></div></details></div>},
 ];
 return <><header className="page-heading"><div><p className="eyebrow">OPERATIONS / CHANNELS</p><h1>渠道管理</h1><p>本地配置直接生效；调用统计为最近 24 小时的渠道尝试。</p></div><button onClick={()=>setEditing(empty)} disabled={pending||resource.loading||groups.loading}>新建渠道</button></header>
 <section className="panel"><FilterBar>
  <label>搜索渠道或模型<input disabled={busy} value={search} onChange={e=>setSearch(e.target.value)}/></label>
  <label>分组<select disabled={pending} value={group} onChange={e=>setFilter({...filter,group:e.target.value,page:1})}><option value="">全部分组</option>{groups.data?.map(g=><option key={g.name}>{g.name}</option>)}</select></label>
  <label>状态<select disabled={pending} value={enabled} onChange={e=>setFilter({...filter,enabled:e.target.value,page:1})}><option value="">全部状态</option><option value="true">启用</option><option value="false">停用</option></select></label>
  <label>渠道类型<select disabled={pending} value={type} onChange={e=>setFilter({...filter,type:e.target.value,page:1})}><option value="">全部类型</option>{types.map(([id,name])=><option key={id} value={id}>{name}</option>)}</select></label>
  <button onClick={()=>{resource.reload();groups.reload()}} disabled={pending}>刷新列表</button><Link to="/ops/groups">分组与可用性</Link>
 </FilterBar>
 {notice&&<p role="status">{notice}</p>}{error&&<Alert>{error}</Alert>}{resource.error&&<Alert>{resource.error}</Alert>}{groups.error&&<Alert>{groups.error}</Alert>}
 {selected.length>0&&<div className="row-actions"><span>已选 {selected.length} 项</span><button disabled={pending} onClick={()=>void batch('enable')}>批量启用</button><button disabled={pending} onClick={()=>void batch('disable')}>批量停用</button><button disabled={pending} onClick={()=>void batch('delete')}>批量删除</button></div>}
 {resource.loading?<Loading label="加载中"/>:resource.data&&<DataTable columns={columns} rows={resource.data.items} rowKey={c=>String(c.id)} page={page} total={resource.data.total} pageSize={pageSize} onPageChange={setPage} onPageSizeChange={size=>{setPageSize(size);setPage(1)}} selected={selected} onSelectionChange={setSelected} disabled={pending}/>}</section>
 {channelID&&detail.error&&<Alert>{detail.error}</Alert>}{channelID&&detail.loading&&<Loading label="加载中"/>}
 {editor&&groups.data&&<ChannelEditor key={editor.id} client={client} channel={editor} groups={groups.data} pullOnOpen={pullOnOpen} onClose={close} onSaved={saved=>{setNotice('渠道已保存');if(editor.id&&resource.data)updateRow(saved);else resource.reload();close()}}/>}
 {testing&&<ChannelTestDialog client={client} channel={testing} onClose={()=>setTesting(undefined)} onChange={next=>{updateRow(next);setTesting(next)}}/>}</>;
}
function ChannelEditor({client,channel,groups,pullOnOpen,onClose,onSaved}:{client:ApiClient;channel:Channel;groups:Group[];pullOnOpen:boolean;onClose:()=>void;onSaved:(channel:Channel)=>void}){
 const formId=useId();
 const [v,setV]=useState(channel),[mapping,setMapping]=useState(JSON.stringify(channel.model_mapping,null,2)),[headers,setHeaders]=useState(JSON.stringify(channel.header_override,null,2)),[key,setKey]=useState('');
 const [busy,setBusy]=useState(false),[error,setError]=useState(''),[notice,setNotice]=useState('');
 const [fetched,setFetched]=useState<{models:string[];mapping:Record<string,string>}>();
 useEffect(()=>{if(pullOnOpen)void pull()},[]);
 async function save(e:FormEvent){
  e.preventDefault();setBusy(true);setError('');
  try{
   const saved=await client.request<Channel>(root+'/channels','PUT',{id:v.id,name:v.name,type:v.type,base_url:v.base_url,models:v.models,groups:v.groups,model_mapping:JSON.parse(mapping),header_override:JSON.parse(headers),priority:v.priority,weight:v.weight,enabled:v.enabled,remark:v.remark,test_model:v.test_model,...(v.copy_from?{copy_from:v.copy_from}:{}),...(key.trim()?{key:key.trim()}:{})});
   setKey('');onSaved(saved);
  }catch(e){setError(errorText(e))}finally{setBusy(false)}
 }
 async function pull(){setBusy(true);setError('');try{const modelMapping=JSON.parse(mapping);const models=await client.request<string[]>(root+'/channels/models','POST',{id:v.id||v.copy_from||0,type:v.type,base_url:v.base_url,key,header_override:JSON.parse(headers)});setFetched({models,mapping:modelMapping})}catch(e){setError(errorText(e))}finally{setBusy(false)}}
 return <Drawer title={channel.id?'编辑渠道':'新建渠道'} busy={busy} onClose={onClose} footer={<><button type="button" disabled={busy} onClick={onClose}>取消</button><button type="submit" form={formId} className="primary" disabled={busy}>保存渠道</button></>}>
  <form id={formId} onSubmit={save}><fieldset disabled={busy} className="plain-fieldset">
   <section className="ops-form-section"><h3>基本信息</h3><div className="ops-form-grid">
    <label>渠道名称<input required value={v.name} onChange={e=>setV({...v,name:e.target.value})}/></label>
    <label>类型<select value={v.type} onChange={e=>setV({...v,type:Number(e.target.value)})}>{types.map(([id,name])=><option key={id} value={id}>{name}</option>)}</select></label>
    <label><input type="checkbox" checked={v.enabled} onChange={e=>setV({...v,enabled:e.target.checked})}/>启用渠道</label>
    <label>备注<input value={v.remark} onChange={e=>setV({...v,remark:e.target.value})}/></label>
   </div></section>
   <section className="ops-form-section"><h3>连接</h3><div className="ops-form-grid">
    <label>基础 URL<input type="url" placeholder="留空使用官方默认地址" value={v.base_url} onChange={e=>setV({...v,base_url:e.target.value})}/></label>
    <label>上游密钥（留空保留）<textarea rows={2} autoComplete="off" placeholder={channel.key_configured?'已配置，留空不修改':'填写上游密钥'} required={!channel.id&&!channel.copy_from} value={key} onChange={e=>setKey(e.target.value)}/></label>
   </div><p className="hint">已导入的参数、多 Key、Vertex 设置保持不变。</p></section>
   <section className="ops-form-section"><h3>模型与分组</h3>
    <TagInput label="模型列表" values={v.models} onChange={models=>setV({...v,models,test_model:models.includes(v.test_model)?v.test_model:''})}/>
    <div className="row-actions"><button type="button" onClick={()=>void pull()}>从上游拉取模型</button><button type="button" onClick={()=>{void navigator.clipboard.writeText(v.models.join('\n')).then(()=>setNotice('模型列表已复制')).catch(e=>setError(errorText(e)))}}>复制全部</button><button type="button" onClick={()=>{if(window.confirm('清空当前表单的模型列表？保存后生效。'))setV({...v,models:[],test_model:''})}}>清空</button></div>
    <p className="hint">拉取使用当前表单的连接设置；密钥留空沿用已保存的密钥。选定后只填表，保存才生效。</p>
    <TagInput label="渠道分组" values={v.groups} options={groups.map(g=>g.name)} onChange={groups=>setV({...v,groups})}/>
    <label>默认测试模型<select value={v.test_model} onChange={e=>setV({...v,test_model:e.target.value})}><option value="">首个模型</option>{v.models.map(m=><option key={m}>{m}</option>)}</select></label>
   </section>
   <section className="ops-form-section"><h3>路由与覆盖</h3><div className="ops-form-grid">
    <label>优先级<input type="number" step="1" value={v.priority} onChange={e=>setV({...v,priority:Number(e.target.value)})}/></label><label>权重<input type="number" min="0" max="1000000" step="1" value={v.weight} onChange={e=>setV({...v,weight:Number(e.target.value)})}/></label>
   </div><KeyValueEditor label="模型映射" value={mapping} onChange={setMapping} leftLabel="请求模型" rightLabel="上游模型" options={v.models}/><KeyValueEditor label="额外请求头" value={headers} onChange={setHeaders}/></section>
  </fieldset>{error&&<Alert>{error}</Alert>}{notice&&<p role="status">{notice}</p>}</form>
 {fetched&&<FetchModelsDialog models={fetched.models} current={v.models} mapping={fetched.mapping} onClose={()=>setFetched(undefined)} onSelect={models=>{setV({...v,models,test_model:models.includes(v.test_model)?v.test_model:''});setFetched(undefined);setNotice('模型列表已填入表单；保存后生效。')}}/>}
 </Drawer>;
}

function ChannelNumber({label,value,min,max,disabled,save}:{label:string;value:number;min?:number;max?:number;disabled:boolean;save:(value:number)=>Promise<void>}){
 const [draft,setDraft]=useState(String(value));
 useEffect(()=>setDraft(String(value)),[value]);
 return <input className="ops-number" type="number" step="1" min={min} max={max} aria-label={label} disabled={disabled} value={draft} onChange={e=>setDraft(e.target.value)} onKeyDown={e=>{if(e.key==='Enter')e.currentTarget.blur();if(e.key==='Escape'){e.currentTarget.value=String(value);setDraft(String(value));e.currentTarget.blur()}}} onBlur={e=>{const next=e.currentTarget.valueAsNumber;if(!e.currentTarget.validity.valid||!Number.isSafeInteger(next)){setDraft(String(value));return}if(next!==value)void save(next).catch(()=>setDraft(String(value)))}}/>;
}

function ChannelTestDialog({client,channel,onClose,onChange}:{client:ApiClient;channel:Channel;onClose:()=>void;onChange:(channel:Channel)=>void}){
 const [q,setQ]=useState(''),[page,setPage]=useState(1),[size,setSize]=usePageSize('momiao.ops.channelTest.pageSize');
 const [selected,setSelected]=useState<string[]>(channel.test_model?[channel.test_model]:[]);
 const [results,setResults]=useState<Record<string,TestResult>>({}),[active,setActive]=useState('');
 const [stream,setStream]=useState(false),[endpoint,setEndpoint]=useState('auto'),[busy,setBusy]=useState(false),[stopping,setStopping]=useState(false),[error,setError]=useState('');
 const stopped=useRef(false);
 const models=channel.models.filter(m=>m.toLowerCase().includes(q.toLowerCase()));
 const failures=channel.models.filter(m=>results[m]&&!results[m].ok);
 async function run(models:string[]){
  stopped.current=false;setStopping(false);setBusy(true);setError('');
  try{for(const model of models){
   if(stopped.current)break;
   setActive(model);
   try{
    const result=await client.request<TestResult>(root+'/channels/'+channel.id+'/test','POST',{model,stream,endpoint});
    setResults(old=>({...old,[model]:result}));onChange({...channel,test_at:result.test_at,response_ms:result.duration_ms});
   }catch(e){setResults(old=>({...old,[model]:{ok:false,model,status_code:0,duration_ms:0,message:errorText(e),test_at:'',upstream_model:'',max_tokens:0}}))}
  }}finally{setActive('');setBusy(false)}
 }
 async function removeFailed(){
  if(!window.confirm(`从渠道移除 ${failures.length} 个测试失败模型？只修改模型列表。`))return;
  setBusy(true);setError('');
  try{const next=await client.request<Channel>(root+'/channels/'+channel.id,'PATCH',{models:channel.models.filter(m=>!failures.includes(m))});onChange(next);setSelected(old=>old.filter(m=>next.models.includes(m)));setPage(1)}
  catch(e){setError(errorText(e))}finally{setBusy(false)}
 }
 const columns:Column<string>[]=[
  {key:'model',label:'模型',render:m=><><strong>{m}</strong>{channel.model_mapping[m]&&<small>→ {channel.model_mapping[m]}</small>}</>},
  {key:'state',label:'状态',render:m=>active===m?'测试中':results[m]?<span className={'badge'+(results[m].ok?' active':'')}>{results[m].ok?'成功':'失败'}</span>:'未测试'},
  {key:'duration',label:'耗时',render:m=>results[m]?results[m].duration_ms+' ms':'—'},
  {key:'message',label:'结果',render:m=>results[m]?<span title={results[m].upstream_model}>{results[m].message}<small>HTTP {results[m].status_code||'—'}{results[m].max_tokens>0&&` · 上限 ${results[m].max_tokens} tokens`}</small></span>:'—'},
  {key:'test',label:'操作',render:m=><button disabled={busy} aria-label={'测试 '+m} onClick={()=>void run([m])}>测试</button>},
 ];
 return <Modal title={'模型测试 · '+channel.name} className="ops-model-dialog" busy={busy} onClose={onClose}>
  <p className="hint">按渠道模型映射测试；消耗上游额度，不扣用户 Reserve、不写调用日志。每次请求使用该协议的最小测试预算。</p>
  <FilterBar><label>搜索模型<input value={q} onChange={e=>{setQ(e.target.value);setPage(1)}}/></label><label>端点类型<select disabled={busy} value={endpoint} onChange={e=>setEndpoint(e.target.value)}><option value="auto">自动</option><option value="chat/completions">chat/completions</option><option value="responses">responses</option><option value="messages">messages</option></select></label><label className="ops-checkbox"><input type="checkbox" disabled={busy} checked={stream} onChange={e=>setStream(e.target.checked)}/>流式测试</label></FilterBar>
  <div className="row-actions"><span>已选 {selected.length} 个模型</span><button disabled={busy||!selected.length} onClick={()=>void run(selected)}>批量测试</button>{busy&&<button disabled={stopping} onClick={()=>{stopped.current=true;setStopping(true)}}>{stopping?'当前请求结束后停止':'停止'}</button>}<button disabled={busy||!failures.length} onClick={()=>void removeFailed()}>移除失败模型 ({failures.length})</button></div>
  {error&&<Alert>{error}</Alert>}
  <DataTable columns={columns} rows={models.slice((page-1)*size,page*size)} rowKey={m=>m} selected={selected} onSelectionChange={setSelected} page={page} pageSize={size} total={models.length} onPageChange={setPage} onPageSizeChange={n=>{setSize(n);setPage(1)}} disabled={busy}/>
 </Modal>;
}

function FetchModelsDialog({models,current,mapping,onClose,onSelect}:{models:string[];current:string[];mapping:Record<string,string>;onClose:()=>void;onSelect:(models:string[])=>void}){
 const [selected,setSelected]=useState(current),[q,setQ]=useState(''),[page,setPage]=useState(1),[size,setSize]=usePageSize('momiao.ops.fetchModels.pageSize');
 const rows=[...new Set([...models,...current])].filter(m=>m.toLowerCase().includes(q.toLowerCase()));
 const columns:Column<string>[]=[{key:'model',label:'模型',render:m=>m},{key:'state',label:'变化',render:m=>!current.includes(m)?'新增':models.includes(m)?'已存在':mapping[m]?'已存在 · 映射别名':'上游已没有'}];
 return <Modal title="选择上游模型" className="ops-model-dialog" onClose={onClose}>
  <FilterBar><label>搜索上游模型<input value={q} onChange={e=>{setQ(e.target.value);setPage(1)}}/></label><button onClick={()=>setSelected([...new Set([...selected,...models])])}>全选上游模型</button><button onClick={()=>setSelected([])}>取消全选</button></FilterBar>
  <DataTable columns={columns} rows={rows.slice((page-1)*size,page*size)} rowKey={m=>m} selected={selected} onSelectionChange={setSelected} page={page} total={rows.length} pageSize={size} onPageChange={setPage} onPageSizeChange={n=>{setSize(n);setPage(1)}}/>
  <div className="row-actions"><span>已选 {selected.length} 个；不会自动删除已有模型或直接保存。</span><button onClick={onClose}>取消</button><button className="primary" onClick={()=>onSelect(selected)}>填入表单</button></div>
 </Modal>;
}

export function Groups({client}:{client:ApiClient}){
 const [page,setPage]=useState(1),[q,setQ]=useState(''),[editing,setEditing]=useState<(Group&{original_name:string})>(),[busy,setBusy]=useState(false),[error,setError]=useState(''),[notice,setNotice]=useState('');
 const resource=useResource(()=>Promise.all([client.request<Group[]>(root+'/groups'),client.request<Page<Availability>>(root+'/groups/availability?'+new URLSearchParams({page:String(page),page_size:'20',q}))]),[client,page,q]);
 async function save(e:FormEvent){e.preventDefault();setBusy(true);setError('');try{await client.request(root+'/groups','PUT',editing);setEditing(undefined);setNotice('分组已保存');resource.reload()}catch(e){setError(errorText(e))}finally{setBusy(false)}}
 async function remove(g:Group){if(!window.confirm('删除分组“'+g.name+'”？'))return;setBusy(true);setError('');try{await client.request(root+'/groups','DELETE',{name:g.name});setNotice('分组已删除');resource.reload()}catch(e){setError(errorText(e))}finally{setBusy(false)}}
 return <><header className="page-heading"><div><p className="eyebrow">OPERATIONS / GROUPS</p><h1>分组与可用性</h1><p>倍率用于目录报价与 Reserve 计费。重命名同步更新用户与渠道引用。</p></div><button disabled={busy} onClick={()=>setEditing({name:'',description:'',ratio:'1',original_name:''})}>新建分组</button></header>{notice&&<p role="status">{notice}</p>}{error&&<Alert>{error}</Alert>}{resource.error&&<Alert>{resource.error}</Alert>}<label>搜索模型<input value={q} onChange={e=>{setQ(e.target.value);setPage(1)}}/></label><button disabled={busy} onClick={resource.reload}>刷新</button>{resource.loading?<Loading/>:resource.data&&<><section className="panel"><table><thead><tr><th>名称</th><th>描述</th><th>倍率</th><th>操作</th></tr></thead><tbody>{resource.data[0].map(g=><tr key={g.name}><td>{g.name}</td><td>{g.description}</td><td>{g.ratio}</td><td><button disabled={busy} onClick={()=>setEditing({...g,original_name:g.name})}>编辑 {g.name}</button><button disabled={busy||g.name==='default'} onClick={()=>void remove(g)}>删除 {g.name}</button></td></tr>)}</tbody></table></section><section className="panel"><h2>模型可用性矩阵</h2><p className="hint">按启用渠道的模型与分组判断配置可达，不代表上游健康、价格已配置或 Key 额度。灰格可打开相关渠道补充分组。</p><div className="table-wrap" role="region" aria-label="模型分组可用性" tabIndex={0}><table><thead><tr><th>模型</th>{resource.data[0].map(g=><th key={g.name}>{g.name}</th>)}</tr></thead><tbody>{resource.data[1].items.map(m=><tr key={m.model}><th>{m.model}</th>{resource.data![0].map(g=>{const active=m.channels.filter(c=>c.enabled&&c.groups.includes(g.name));return <td key={g.name}><span className={'badge'+(active.length?' active':'')}>{active.length?'可用':'不可用'}</span>{(active.length?active:m.channels).map(c=><small key={c.id}><Link to={'/ops/channels/'+c.id}>{c.name}{active.length?'':c.enabled?' · 未分配':' · 已停用'}</Link></small>)}</td>})}</tr>)}</tbody></table></div><Pager page={page} total={resource.data[1].total} size={20} onChange={setPage} disabled={busy}/></section></>}
 {editing&&<Modal title={editing.original_name?'编辑分组':'新建分组'} busy={busy} onClose={()=>setEditing(undefined)}><form onSubmit={save}><fieldset disabled={busy}><label>分组名称<input required readOnly={editing.original_name==='default'} value={editing.name} onChange={e=>setEditing({...editing,name:e.target.value})}/></label><label>描述<input value={editing.description} onChange={e=>setEditing({...editing,description:e.target.value})}/></label><label>倍率<input type="number" min="0" step="any" required value={editing.ratio} onChange={e=>setEditing({...editing,ratio:e.target.value})}/></label><button type="submit">保存分组</button></fieldset>{error&&<Alert>{error}</Alert>}</form></Modal>}</>;
}
