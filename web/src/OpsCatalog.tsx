import { useEffect, useRef, useState, useSyncExternalStore } from 'react';
import { Link } from 'react-router-dom';
import { ApiClient } from './api';
import { Alert, Brand } from './ui';
import { CatalogFreshnessNote, CatalogLoading, CatalogPriceTable, CatalogPersona } from './Catalog';
import './ops/ops-content.css';
import { OpsCatalogCovers } from './OpsCatalogCovers';
import { FilterBar, useDebouncedValue } from './ops/components/FilterBar';
import { DataTable, usePageSize, type Column } from './ops/components/DataTable';
import { Drawer } from './ops/components/Drawer';
import { availabilityLabels, catalogError, catalogModelPath, catalogOps, type CatalogMetadata, type CatalogModel, type CatalogOpsPage, type CatalogOpsDetail, type CatalogPriceSettings, type CatalogSettings } from './catalog-api';

const emptyPrice:CatalogPriceSettings={input:'0',output:'0',cache_read:'0',cache_write:'0',cache_write_1h:'0',per_request:null,per_second:null,default_seconds:0};
const tokenFields=[['input','输入单价'],['output','输出单价'],['cache_read','缓存读取单价'],['cache_write','缓存写入 5 分钟单价'],['cache_write_1h','缓存写入 1 小时单价']] as const;
const filters=[['','全部'],['PUBLISHED','已公开'],['UNPUBLISHED','未公开'],['UNPRICED','价格未配置'],['NO_CHANNEL','无可用渠道'],['PUBLISHED_NO_CHANNEL','已公开但无渠道']];
const modelDetail=(id:string)=>catalogOps+'/detail?'+new URLSearchParams({model_id:id});
export function OpsCatalog({client,embedded=false}:{client:ApiClient;embedded?:boolean}){
 const ContentRoot=embedded?'section':'main';
 useSyncExternalStore(client.subscribe,client.getSnapshot);const epoch=client.getSessionGeneration();
 const [page,setPage]=useState<CatalogOpsPage>();const [model,setModel]=useState<CatalogModel>();const [form,setForm]=useState<CatalogSettings>();
 const [coversOpen,setCoversOpen]=useState(false);const [coverFamily,setCoverFamily]=useState<string>();
 const [search,setSearch]=useState(''),[filter,setFilter]=useState({q:'',state:'',page:1});
 const settledSearch=useDebouncedValue(search),[pageSize,setPageSize]=usePageSize('momiao.ops.models.pageSize');
 const [selected,setSelected]=useState<string[]>([]),[batchOpen,setBatchOpen]=useState(false),[batchPrice,setBatchPrice]=useState<CatalogPriceSettings|null>(null);
 const [busy,setBusy]=useState(false),[loading,setLoading]=useState(true),[error,setError]=useState(''),[message,setMessage]=useState('');const lock=useRef(false);
 const query='?'+new URLSearchParams({limit:String(pageSize),offset:String((filter.page-1)*pageSize),...(filter.q?{q:filter.q}:{}),...(filter.state?{state:filter.state}:{})});
 const pending=busy||loading||search!==filter.q;
 useEffect(()=>setFilter(old=>({...old,q:settledSearch,page:1})),[settledSearch]);
 useEffect(()=>{document.title=(form?'编辑模型':batchOpen?'批量设置价格':'模型目录')+' · Chaldea Ops'},[form,batchOpen]);
 function close(){setModel(undefined);setForm(undefined);setBatchOpen(false);setError('')}
 function select(detail:CatalogOpsDetail){const item=detail.item;setModel(item);setForm({model_id:item.model_id,metadata:structuredClone(item.metadata),public:item.publication_state==='PUBLISHED',recommended:item.recommended,sort_order:item.sort_order,price_settings:detail.price_settings})}
 async function refresh(){setPage(await client.request<CatalogOpsPage>(catalogOps+query))}
 useEffect(()=>{close();setPage(undefined);setCoversOpen(false)},[client,epoch]);
 useEffect(()=>{let active=true;setLoading(true);setSelected([]);setError('');setMessage('');
  client.request<CatalogOpsPage>(catalogOps+query).then(next=>{if(active)setPage(next)}).catch(e=>{if(active)setError(catalogError(e))}).finally(()=>{if(active)setLoading(false)});return()=>{active=false};
 },[client,epoch,query]);
 async function perform(fn:()=>Promise<void>){if(lock.current)return;lock.current=true;setBusy(true);setError('');setMessage('');try{await fn()}catch(e){setError(catalogError(e))}finally{lock.current=false;setBusy(false)}}
 async function saved(){setMessage('已保存');try{await refresh()}catch{setError('已保存，列表读取失败，请刷新查看。')}}
 async function save(value:CatalogSettings){await client.request<CatalogOpsDetail>(catalogOps,'PUT',value);close();await saved()}
 async function publication(ids:string[],value:boolean){await client.request(catalogOps+'/publication','PUT',{model_ids:ids,public:value});setSelected([]);await saved()}
 async function savePrices(){await client.request(catalogOps+'/prices','PUT',{model_ids:selected,price_settings:batchPrice});close();setSelected([]);await saved()}
 function rowValue(item:CatalogModel,patch:Partial<CatalogSettings>):CatalogSettings{return {model_id:item.model_id,metadata:item.metadata,public:item.publication_state==='PUBLISHED',recommended:item.recommended,sort_order:item.sort_order,price_settings:null,...patch}}
 const metadata=form?.metadata;
 const patch=(value:Partial<CatalogSettings>)=>setForm(old=>old?{...old,...value}:old);
 const meta=(value:Partial<CatalogMetadata>)=>setForm(old=>old?{...old,metadata:{...old.metadata,...value}}:old);
 const columns:Column<CatalogModel>[]=[
  {key:'model',label:'模型',render:item=><><strong>{item.metadata.display_name||item.model_id}</strong>{item.metadata.display_name&&<code>{item.model_id}</code>}{item.publication_state==='PUBLISHED'&&(!item.endpoints.length||!item.price.configured)&&<small className="catalog-row-warning">{!item.endpoints.length?'已公开但无可用渠道':''}{!item.endpoints.length&&!item.price.configured?' · ':''}{!item.price.configured?'价格未配置':''}</small>}</>},
  {key:'public',label:'公开',render:item=><input aria-label={'公开 '+item.model_id} type="checkbox" checked={item.publication_state==='PUBLISHED'} disabled={pending} onChange={e=>void perform(()=>publication([item.model_id],e.target.checked))}/>},
  {key:'recommended',label:'推荐',render:item=><input aria-label={'推荐 '+item.model_id} type="checkbox" checked={item.recommended} disabled={pending} onChange={e=>void perform(()=>save(rowValue(item,{recommended:e.target.checked})))}/>},
  {key:'channel',label:'渠道',render:item=>item.availability_state==='CONFIGURED'&&!item.endpoints.length?'无支持的渠道':availabilityLabels[item.availability_state]},
  {key:'price',label:'价格',render:item=>item.price.configured?item.price.mode==='ratio'?'按 token':item.price.mode==='per_second'?'按秒':'按次':'未配置'},
  {key:'edit',label:'操作',render:item=><button disabled={pending} onClick={()=>void perform(async()=>select(await client.request<CatalogOpsDetail>(modelDetail(item.model_id))))}>编辑模型</button>},
 ];
 return <div className={'catalog-ops'+(embedded?' ops-embedded':'')}><header className="catalog-ops-header"><Link to="/" className="brand"><Brand/></Link><span>平台运营 / 模型</span><nav aria-label="模型运营导航"><Link to="/models">公开目录 ↗</Link></nav></header><ContentRoot className="catalog-ops-main">
  <header className="section-heading"><div><p className="eyebrow">MODELS / CATALOG</p><h1>模型目录运营</h1><p>渠道中的模型即时出现；展示信息、价格和公开状态直接保存。</p></div><div className="catalog-actions"><button disabled={!page||pending} onClick={()=>{setCoverFamily(undefined);setCoversOpen(true)}}>家族封面</button><button disabled={pending} onClick={()=>void perform(refresh)}>刷新</button></div></header>
  {message&&<p role="status">{message}</p>}{error&&!form&&!batchOpen&&<Alert>{error}</Alert>}{!page&&loading?<CatalogLoading/>:page&&<>
   <CatalogFreshnessNote freshness={page.freshness}/>
   <section className="catalog-ops-list" aria-labelledby="catalog-list-title"><div className="ops-content-section-heading"><h2 id="catalog-list-title">模型运营列表</h2><span className="hint">共 {page.total} 项</span></div>
    <FilterBar><label>查找模型<input value={search} onChange={e=>setSearch(e.target.value)} maxLength={200} disabled={busy}/></label><label>快捷筛选<select value={filter.state} disabled={busy} onChange={e=>setFilter(old=>({...old,state:e.target.value,page:1}))}>{filters.map(([value,label])=><option key={value} value={value}>{label}</option>)}</select></label></FilterBar>
    <div className="catalog-actions"><button disabled={pending||!selected.length} onClick={()=>void perform(()=>publication(selected,true))}>批量公开</button><button disabled={pending||!selected.length} onClick={()=>void perform(()=>publication(selected,false))}>批量隐藏</button><button disabled={pending||!selected.length} onClick={()=>{setBatchPrice(null);setBatchOpen(true);setError('')}}>批量设置价格</button><span>已选 {selected.length} 项</span></div>
    <DataTable columns={columns} rows={page.items} rowKey={item=>item.model_id} page={filter.page} total={page.total} pageSize={pageSize} onPageChange={n=>setFilter(old=>({...old,page:n}))} onPageSizeChange={size=>{setPageSize(size);setFilter(old=>({...old,page:1}))}} selected={selected} onSelectionChange={setSelected} disabled={pending} emptyText="没有匹配的目录项；渠道配置中的模型会自动出现在这里。"/>
   </section>
  </>}
  {page&&model&&form&&metadata&&<Drawer title="编辑模型" busy={busy} onClose={close} footer={<><button disabled={busy} onClick={close}>取消</button><button type="submit" form="catalog-model-form" className="primary" disabled={busy}>保存模型</button></>}>
   {error&&<Alert>{error}</Alert>}<code>{model.model_id}</code><form id="catalog-model-form" onSubmit={e=>{e.preventDefault();void perform(()=>save(form))}}><fieldset className="plain-fieldset" disabled={busy}>
   <section className="ops-form-section"><h3>展示信息</h3><div className="ops-form-grid catalog-ops-fields">
   <label>展示名称<input value={metadata.display_name} maxLength={120} onChange={e=>meta({display_name:e.target.value})}/></label><label>模型家族<select value={metadata.family} onChange={e=>meta({family:e.target.value})}><option value="">尚未选择</option>{page.vocabulary.families.map(c=><option value={c.value} key={c.value}>{c.label}</option>)}</select></label>
   <label className="wide">模型简介<textarea value={metadata.summary} maxLength={2000} rows={4} onChange={e=>meta({summary:e.target.value})}/></label><label>副标题<input value={metadata.subtitle} maxLength={160} onChange={e=>meta({subtitle:e.target.value})}/></label><label>Context tokens（未知请留空）<input type="number" min="1" max="9007199254740991" value={metadata.context_length||''} onChange={e=>meta({context_length:e.target.value||null})}/></label>
   <fieldset><legend>能力标签</legend>{page.vocabulary.tags.map(c=><label className="checkbox" key={c.value}><input type="checkbox" checked={metadata.tags.includes(c.value)} onChange={e=>meta({tags:e.target.checked?[...metadata.tags,c.value]:metadata.tags.filter(v=>v!==c.value)})}/>{c.label}</label>)}</fieldset>
   <fieldset><legend>推荐用途</legend>{page.vocabulary.use_cases.map(c=><label className="checkbox" key={c.value}><input type="checkbox" checked={metadata.use_cases.includes(c.value)} onChange={e=>meta({use_cases:e.target.checked?[...metadata.use_cases,c.value]:metadata.use_cases.filter(v=>v!==c.value)})}/>{c.label}</label>)}</fieldset>
   <label className="wide">特殊计价说明<textarea value={metadata.special_pricing_note} maxLength={1000} rows={2} onChange={e=>meta({special_pricing_note:e.target.value})}/></label>
   <label>排序顺序<input type="number" min="0" max="1000000" required value={form.sort_order} onChange={e=>patch({sort_order:Number(e.target.value)})}/></label><label className="checkbox"><input type="checkbox" checked={form.recommended} onChange={e=>patch({recommended:e.target.checked})}/>精选推荐</label><label className="checkbox"><input type="checkbox" checked={form.public} onChange={e=>patch({public:e.target.checked})}/>公开此模型</label>

   </div></section><section className="ops-form-section"><h3>价格</h3><CatalogPriceEditor client={client} value={form.price_settings} onChange={price_settings=>patch({price_settings})} perform={perform}/><h4>当前公开参考价（default 分组）</h4><CatalogPriceTable price={model.price}/></section>
   <section className="ops-form-section"><h3>封面</h3><button type="button" disabled={!metadata.family} onClick={()=>{setCoverFamily(metadata.family);setCoversOpen(true)}}>管理此家族封面</button><CatalogPersona model={{...model,metadata}} assets={page.vocabulary.assets} compact/></section>
   </fieldset></form>{model.publication_state==='PUBLISHED'&&<Link to={catalogModelPath(model.model_id)}>查看公开详情 ↗</Link>}
  </Drawer>}
  {batchOpen&&<Drawer title="批量设置价格" busy={busy} onClose={close} footer={<><button disabled={busy} onClick={close}>取消</button><button type="submit" form="catalog-prices-form" className="primary" disabled={busy||!batchPrice}>保存价格</button></>}>
   {error&&<Alert>{error}</Alert>}<p>将统一设置以下 {selected.length} 个模型的基础价格，不改变展示信息、公开状态和推荐排序。</p><ul>{selected.map(id=><li key={id}>{id}</li>)}</ul>
   <form id="catalog-prices-form" onSubmit={e=>{e.preventDefault();void perform(savePrices)}}><fieldset className="plain-fieldset" disabled={busy}><CatalogPriceEditor client={client} value={batchPrice} onChange={setBatchPrice} perform={perform}/></fieldset></form>
  </Drawer>}
  {coversOpen&&<OpsCatalogCovers client={client} initialFamily={coverFamily} onClose={()=>setCoversOpen(false)} onChanged={cover=>{setPage(old=>old?{...old,items:old.items.map(m=>m.metadata.family===cover.family?{...m,family_cover:cover}:m)}:old);setModel(old=>old&&old.metadata.family===cover.family?{...old,family_cover:cover}:old)}}/>}
 </ContentRoot></div>;
}

function CatalogPriceEditor({client,value,onChange,perform}:{client:ApiClient;value:CatalogPriceSettings|null;onChange:(value:CatalogPriceSettings)=>void;perform:(fn:()=>Promise<void>)=>Promise<void>}){
 const [search,setSearch]=useState(''),[choices,setChoices]=useState<CatalogOpsPage>(),[error,setError]=useState('');const q=useDebouncedValue(search);
 useEffect(()=>{let active=true;setError('');setChoices(undefined);client.request<CatalogOpsPage>(catalogOps+'?'+new URLSearchParams({limit:'100',...(q.trim()?{q:q.trim()}:{})})).then(next=>{if(active)setChoices(next)}).catch(e=>{if(active)setError(catalogError(e))});return()=>{active=false}},[client,q]);
 const patch=(part:Partial<CatalogPriceSettings>)=>onChange({...value!,...part});
 return <div className="catalog-price-editor">
  <p className="hint">单位为 API Credit：token 单价按百万计，按次 / 按秒分别按一次 / 一秒计。这里不含分组倍率，结算时按账号所属分组应用一次；零明确表示免费。</p>
  {error&&<Alert>{error}</Alert>}<div className="ops-form-grid"><label>查找价格来源<input value={search} onChange={e=>setSearch(e.target.value)} maxLength={200}/></label><label>从其他模型复制价格<select value="" disabled={!choices||search!==q} onChange={e=>{const id=e.target.value;if(id)void perform(async()=>{const detail=await client.request<CatalogOpsDetail>(modelDetail(id));if(!detail.price_settings)throw new Error('来源模型尚未配置价格。');onChange(detail.price_settings)})}}><option value="">选择已有价格的模型</option>{choices?.items.filter(m=>m.price.configured).map(m=><option key={m.model_id} value={m.model_id}>{m.metadata.display_name?m.metadata.display_name+' · ':''}{m.model_id}</option>)}</select></label></div>
  {choices&&choices.total>100&&<p className="hint">显示前 100 项中的已配置价格模型，可输入名称缩小范围。</p>}
  {!value?<><p>未配置价格；保存展示信息不会将其当作免费。</p><button type="button" onClick={()=>onChange({...emptyPrice,input:'',output:'',cache_read:'',cache_write:'',cache_write_1h:''})}>配置价格</button></>:<><label>计价方式<select value={value.per_request!==null?'request':value.per_second!==null?'second':'tokens'} onChange={e=>onChange({...emptyPrice,...(e.target.value==='request'?{per_request:''}:e.target.value==='second'?{per_second:''}:{input:'',output:'',cache_read:'',cache_write:'',cache_write_1h:''})})}><option value="tokens">按 token</option><option value="request">按次</option><option value="second">按秒</option></select></label><div className="ops-form-grid">
   {value.per_request===null&&value.per_second===null?tokenFields.map(([key,label])=><label key={key}>{label}<input inputMode="decimal" pattern="[0-9]+(\.[0-9]+)?" required value={value[key]} onChange={e=>patch({[key]:e.target.value})}/></label>):value.per_request!==null?<label>按次单价<input inputMode="decimal" pattern="[0-9]+(\.[0-9]+)?" required value={value.per_request} onChange={e=>patch({per_request:e.target.value})}/></label>:<><label>按秒单价<input inputMode="decimal" pattern="[0-9]+(\.[0-9]+)?" required value={value.per_second!} onChange={e=>patch({per_second:e.target.value})}/></label><label>缺省时长（秒）<input type="number" min="0" max="2147483647" required value={value.default_seconds} onChange={e=>patch({default_seconds:Number(e.target.value)})}/></label></>}
  </div></>}
 </div>;
}
