import { useEffect, useRef, useState, useSyncExternalStore, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { ApiClient } from './api';
import { Alert, Brand, Empty, Pager } from './ui';
import { CatalogFreshnessNote, CatalogLoading, CatalogPriceTable, CatalogPersona } from './Catalog';
import './ops/ops-content.css';
import { OpsCatalogCovers } from './OpsCatalogCovers';
import { availabilityLabels, catalogError, catalogModelPath, catalogOps, publicationLabels, type CatalogMetadata, type CatalogModel, type CatalogOpsPage, type CatalogOpsDetail, type CatalogPriceSettings, type CatalogSettings } from './catalog-api';

const emptyPrice:CatalogPriceSettings={input:'0',output:'0',cache_read:'0',cache_write:'0',cache_write_1h:'0',per_request:null,per_second:null,default_seconds:0};
const tokenFields=[['input','输入单价'],['output','输出单价'],['cache_read','缓存读取单价'],['cache_write','缓存写入 5 分钟单价'],['cache_write_1h','缓存写入 1 小时单价']] as const;
export function OpsCatalog({client,embedded=false}:{client:ApiClient;embedded?:boolean}){
 const ContentRoot=embedded?'section':'main';
 useSyncExternalStore(client.subscribe,client.getSnapshot);const epoch=client.getSessionGeneration();
 const [page,setPage]=useState<CatalogOpsPage>();const [model,setModel]=useState<CatalogModel>();const [form,setForm]=useState<CatalogSettings>();
 const [coversOpen,setCoversOpen]=useState(false);const [coverFamily,setCoverFamily]=useState<string>();
 const [search,setSearch]=useState('');const [state,setState]=useState('');const [query,setQuery]=useState('');const [selected,setSelected]=useState<string[]>([]);
 const [busy,setBusy]=useState(false);const [loading,setLoading]=useState(true);const [error,setError]=useState('');const [message,setMessage]=useState('');const lock=useRef(false);
 function select(detail:CatalogOpsDetail){const item=detail.item;setModel(item);setForm({model_id:item.model_id,metadata:structuredClone(item.metadata),public:item.publication_state==='PUBLISHED',recommended:item.recommended,sort_order:item.sort_order,price_settings:detail.price_settings})}
 async function refresh(id?:string){setPage(await client.request<CatalogOpsPage>(catalogOps+query));if(id)select(await client.request<CatalogOpsDetail>(catalogOps+'/detail?'+new URLSearchParams({model_id:id})))}
 useEffect(()=>{let active=true;setLoading(true);setPage(undefined);setModel(undefined);setForm(undefined);setSelected([]);setCoversOpen(false);setError('');setMessage('');
  client.request<CatalogOpsPage>(catalogOps+query).then(next=>{if(active)setPage(next)}).catch(e=>{if(active)setError(catalogError(e))}).finally(()=>{if(active)setLoading(false)});return()=>{active=false};
 },[client,epoch,query]);
 async function perform(fn:()=>Promise<void>){if(lock.current)return;lock.current=true;setBusy(true);setError('');setMessage('');try{await fn()}catch(e){setError(catalogError(e))}finally{lock.current=false;setBusy(false)}}
 async function saved(id?:string){setMessage('已保存');try{await refresh(id)}catch{setError('已保存，列表读取失败，请刷新查看。')}}
 async function save(value:CatalogSettings){await client.request<CatalogOpsDetail>(catalogOps,'PUT',value);await saved(model?.model_id)}
 async function publication(ids:string[],value:boolean){await client.request(catalogOps+'/publication','PUT',{model_ids:ids,public:value});setSelected([]);await saved(model?.model_id)}
 function rowValue(item:CatalogModel,patch:Partial<CatalogSettings>):CatalogSettings{return {model_id:item.model_id,metadata:item.metadata,public:item.publication_state==='PUBLISHED',recommended:item.recommended,sort_order:item.sort_order,price_settings:null,...patch}}
 const metadata=form?.metadata;
 const patch=(value:Partial<CatalogSettings>)=>setForm(old=>old?{...old,...value}:old);
 const meta=(value:Partial<CatalogMetadata>)=>setForm(old=>old?{...old,metadata:{...old.metadata,...value}}:old);
 const price=(value:Partial<CatalogPriceSettings>)=>setForm(old=>old?{...old,price_settings:{...(old.price_settings||emptyPrice),...value}}:old);
 function filter(e:FormEvent){e.preventDefault();const q=new URLSearchParams();if(search.trim())q.set('q',search.trim());if(state)q.set('state',state);setQuery(q.size?'?'+q:'')}
 return <div className={'catalog-ops'+(embedded?' ops-embedded':'')}><header className="catalog-ops-header"><Link to="/" className="brand"><Brand/></Link><span>平台运营 / 模型</span><nav aria-label="模型运营导航"><Link to="/models">公开目录 ↗</Link></nav></header><ContentRoot className="catalog-ops-main">
  <header className="section-heading"><div><p className="eyebrow">MODELS / CATALOG</p><h1>模型目录运营</h1><p>渠道中的模型即时出现；展示信息、价格和公开状态直接保存。</p></div><div className="catalog-actions"><button disabled={!page||busy} onClick={()=>{setCoverFamily(metadata?.family||undefined);setCoversOpen(true)}}>家族封面</button><button disabled={busy} onClick={()=>void perform(()=>refresh(model?.model_id))}>刷新</button></div></header>
  {message&&<p role="status">{message}</p>}{error&&<Alert>{error}</Alert>}{loading?<CatalogLoading/>:page&&<>
   <CatalogFreshnessNote freshness={page.freshness}/>
   <section className="catalog-ops-list" aria-labelledby="catalog-list-title"><div className="ops-content-section-heading"><h2 id="catalog-list-title">模型运营列表</h2><span className="hint">共 {page.total} 项</span></div>
    <form className="catalog-ops-filter" onSubmit={filter}><label>查找模型<input value={search} onChange={e=>setSearch(e.target.value)} maxLength={200}/></label><label>公开状态<select value={state} onChange={e=>setState(e.target.value)}><option value="">全部状态</option>{Object.entries(publicationLabels).map(([v,label])=><option value={v} key={v}>{label}</option>)}</select></label><button type="submit" disabled={busy}>查找</button></form>
    <div className="catalog-actions"><button disabled={busy||!selected.length} onClick={()=>void perform(()=>publication(selected,true))}>批量公开</button><button disabled={busy||!selected.length} onClick={()=>void perform(()=>publication(selected,false))}>批量隐藏</button><span>已选 {selected.length} 项</span></div>
    {page.items.length?<div className="table-wrap" role="region" aria-label="模型运营列表" tabIndex={0}><table><thead><tr><th><input aria-label="选择本页模型" type="checkbox" disabled={busy} checked={selected.length===page.items.length} onChange={e=>setSelected(e.target.checked?page.items.map(i=>i.model_id):[])}/></th><th>模型</th><th>公开</th><th>推荐</th><th>排序</th><th>渠道</th><th>价格</th><th>操作</th></tr></thead><tbody>{page.items.map(item=><tr key={item.model_id}>
     <td><input aria-label={'选择 '+item.model_id} type="checkbox" disabled={busy} checked={selected.includes(item.model_id)} onChange={e=>setSelected(old=>e.target.checked?[...old,item.model_id]:old.filter(id=>id!==item.model_id))}/></td>
     <td><strong>{item.metadata.display_name||'尚未命名'}</strong><code>{item.model_id}</code></td>
     <td><input aria-label={'公开 '+item.model_id} type="checkbox" checked={item.publication_state==='PUBLISHED'} disabled={busy} onChange={e=>void perform(()=>publication([item.model_id],e.target.checked))}/></td>
     <td><input aria-label={'推荐 '+item.model_id} type="checkbox" checked={item.recommended} disabled={busy} onChange={e=>void perform(()=>save(rowValue(item,{recommended:e.target.checked})))}/></td>
     <td><input aria-label={'排序 '+item.model_id} type="number" min="0" max="1000000" step="1" key={item.model_id+':'+item.sort_order} defaultValue={item.sort_order} disabled={busy} onBlur={e=>{if(e.target.validity.valid&&e.target.value!==''&&Number(e.target.value)!==item.sort_order)void perform(()=>save(rowValue(item,{sort_order:Number(e.target.value)})))}}/></td>
     <td>{availabilityLabels[item.availability_state]}</td><td>{item.price.configured?item.price.mode==='ratio'?'按 token':item.price.mode==='per_second'?'按秒':'按次':'未配置'}</td>
     <td><button disabled={busy} onClick={()=>void perform(async()=>select(await client.request<CatalogOpsDetail>(catalogOps+'/detail?'+new URLSearchParams({model_id:item.model_id}))))}>编辑模型</button></td>
    </tr>)}</tbody></table></div>:<Empty title="没有匹配的目录项">渠道配置中的模型会自动出现在这里，无需同步。</Empty>}
    <Pager total={page.total} size={page.limit} page={Math.floor(page.offset/page.limit)+1} disabled={busy} onChange={n=>{const q=new URLSearchParams(query);q.set('offset',String((n-1)*page.limit));setQuery('?'+q)}}/>
   </section>
  </>}
  {page&&model&&form&&metadata&&<section className="catalog-ops-editor" aria-label="模型编辑器"><header><h2>{model.metadata.display_name||'完善模型信息'}</h2><code>{model.model_id}</code></header><form onSubmit={e=>{e.preventDefault();void perform(()=>save(form))}}><fieldset disabled={busy}><legend>展示信息与价格</legend><div className="catalog-ops-fields">
   <label>展示名称<input value={metadata.display_name} maxLength={120} onChange={e=>meta({display_name:e.target.value})}/></label><label>模型家族<select value={metadata.family} onChange={e=>meta({family:e.target.value})}><option value="">尚未选择</option>{page.vocabulary.families.map(c=><option value={c.value} key={c.value}>{c.label}</option>)}</select></label>
   <label className="wide">模型简介<textarea value={metadata.summary} maxLength={2000} rows={4} onChange={e=>meta({summary:e.target.value})}/></label><label>副标题<input value={metadata.subtitle} maxLength={160} onChange={e=>meta({subtitle:e.target.value})}/></label><label>Context tokens（未知请留空）<input type="number" min="1" max="9007199254740991" value={metadata.context_length||''} onChange={e=>meta({context_length:e.target.value||null})}/></label>
   <fieldset><legend>能力标签</legend>{page.vocabulary.tags.map(c=><label className="checkbox" key={c.value}><input type="checkbox" checked={metadata.tags.includes(c.value)} onChange={e=>meta({tags:e.target.checked?[...metadata.tags,c.value]:metadata.tags.filter(v=>v!==c.value)})}/>{c.label}</label>)}</fieldset>
   <fieldset><legend>推荐用途</legend>{page.vocabulary.use_cases.map(c=><label className="checkbox" key={c.value}><input type="checkbox" checked={metadata.use_cases.includes(c.value)} onChange={e=>meta({use_cases:e.target.checked?[...metadata.use_cases,c.value]:metadata.use_cases.filter(v=>v!==c.value)})}/>{c.label}</label>)}</fieldset>
   <label className="wide">特殊计价说明<textarea value={metadata.special_pricing_note} maxLength={1000} rows={2} onChange={e=>meta({special_pricing_note:e.target.value})}/></label>
   <label>排序顺序<input type="number" min="0" max="1000000" required value={form.sort_order} onChange={e=>patch({sort_order:Number(e.target.value)})}/></label><label className="checkbox"><input type="checkbox" checked={form.recommended} onChange={e=>patch({recommended:e.target.checked})}/>精选推荐</label><label className="checkbox"><input type="checkbox" checked={form.public} onChange={e=>patch({public:e.target.checked})}/>公开此模型</label>
  </div><fieldset><legend>本地基础价格</legend><p className="hint">单位为 API Credit：token 单价按百万计，按次 / 按秒分别按一次 / 一秒计。这里不含分组倍率，结算时按账号所属分组应用一次；零明确表示免费。</p>
   {!form.price_settings?<><p>未配置价格，不可接入。保存展示信息不会将其当作免费。</p><button type="button" onClick={()=>price(emptyPrice)}>配置价格</button></>:<><label>计价方式<select value={form.price_settings.per_request!==null?'request':form.price_settings.per_second!==null?'second':'tokens'} onChange={e=>price({per_request:e.target.value==='request'?'0':null,per_second:e.target.value==='second'?'0':null})}><option value="tokens">按 token</option><option value="request">按次</option><option value="second">按秒</option></select></label><div className="catalog-ops-fields">
    {form.price_settings.per_request===null&&form.price_settings.per_second===null?tokenFields.map(([key,label])=><label key={key}>{label}<input inputMode="decimal" pattern="[0-9]+(\.[0-9]+)?" required value={form.price_settings![key]} onChange={e=>price({[key]:e.target.value})}/></label>):form.price_settings.per_request!==null?<label>按次单价<input inputMode="decimal" pattern="[0-9]+(\.[0-9]+)?" required value={form.price_settings.per_request} onChange={e=>price({per_request:e.target.value})}/></label>:<><label>按秒单价<input inputMode="decimal" pattern="[0-9]+(\.[0-9]+)?" required value={form.price_settings.per_second!} onChange={e=>price({per_second:e.target.value})}/></label><label>缺省时长（秒）<input type="number" min="0" max="2147483647" required value={form.price_settings.default_seconds} onChange={e=>price({default_seconds:Number(e.target.value)})}/></label></>}
   </div></>}
  </fieldset><button type="submit" className="primary">保存模型</button></fieldset></form>
  <section className="catalog-model-cover"><div><h3>所属家族封面</h3><button disabled={busy||!metadata.family} onClick={()=>{setCoverFamily(metadata.family);setCoversOpen(true)}}>管理此家族封面</button></div><CatalogPersona model={model} assets={page.vocabulary.assets} compact/></section>
  <section><h3>当前公开参考价（default 分组）</h3><CatalogPriceTable price={model.price}/></section>{model.publication_state==='PUBLISHED'&&<Link to={catalogModelPath(model.model_id)}>查看公开详情 ↗</Link>}
  </section>}
  {coversOpen&&<OpsCatalogCovers client={client} initialFamily={coverFamily} onClose={()=>setCoversOpen(false)} onChanged={cover=>{setPage(old=>old?{...old,items:old.items.map(m=>m.metadata.family===cover.family?{...m,family_cover:cover}:m)}:old);setModel(old=>old&&old.metadata.family===cover.family?{...old,family_cover:cover}:old)}}/>}
 </ContentRoot></div>;
}
