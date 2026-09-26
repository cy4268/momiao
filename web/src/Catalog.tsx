import { useEffect, useRef, useState, useSyncExternalStore, type CSSProperties, type FormEvent, type ReactNode } from 'react';
import { Link, NavLink, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { ApiClient } from './api';
import { normalizeRouteIntent, saveRouteIntent } from './post-auth-intent';
import { Alert, Brand, Empty, Modal, Pager, useResource } from './ui';
import { availabilityLabels, catalogCurl, catalogError, catalogModelPath, catalogSelectionPath, catalogTime, decodeCatalogModelPath, decimalDisplay, dimensionLabels, endpointLabels, readCatalog, readCatalogDetail, readPersonalPrice, selectedModel, useCatalogModel, validCatalogCoverSrc, type CatalogAsset, type CatalogChoice, type CatalogFreshness, type CatalogLoginRequired, type CatalogModel, type CatalogPrice, type CatalogVocabulary } from './catalog-api';
import { assetUrl } from './game-hall-assets';
import workshopArt from './catalog-workshop-art.json';
import './catalog.css';
import './catalog-workshop.css';

type CatalogProps={client:ApiClient;onLoginRequired?:CatalogLoginRequired};
function useCatalogLogin(onLoginRequired?:CatalogLoginRequired){
    const navigate=useNavigate();
    return onLoginRequired||((returnPath:string)=>{saveRouteIntent(returnPath);navigate('/login');});
}
export function CatalogShell({client,title,children}:{client:ApiClient;title:string;children:ReactNode}){
    const session=useSyncExternalStore(client.subscribe,client.getSnapshot);
    const location=useLocation();const main=useRef<HTMLElement>(null);
    useEffect(()=>{document.title=title+' · Chaldea';main.current?.focus()},[location.pathname,title]);
    return <div className="catalog-shell" style={{'--workshop-art':`url("${assetUrl(workshopArt.background)}")`} as CSSProperties}><a className="skip-link" href="#catalog-content">跳至主要内容</a>
        <header className="portal-header"><Link className="brand" to="/" aria-label="Chaldea Platform 首页"><Brand/></Link>
            <nav className="portal-global" aria-label="主导航"><Link to="/">首页</Link><NavLink to="/models">模型目录</NavLink><Link to="/entertainment">娱乐</Link><Link to="/rankings">排行榜</Link><Link to="/announcements">公告</Link></nav>
            <Link className="button" to={session.user?'/me':'/login'}>{session.user?'个人中心':'登录账户'}</Link>
        </header>
        <nav className="portal-context" aria-label="模型服务导航"><NavLink to="/models">发现模型</NavLink><NavLink to="/api/access">API 接入</NavLink><Link to="/keys">密钥管理</Link><Link to="/logs">调用记录</Link></nav>
        <main ref={main} tabIndex={-1} id="catalog-content" className="catalog-content">{children}</main>
        <footer className="workspace-foot"><span>CHALDEA / DA VINCI WORKSHOP</span><span>模型工房 · 发现你的下一位搭档</span></footer>
    </div>;
}
export function CatalogLoading(){return <p className="catalog-loading" role="status">正在读取模型目录…</p>}
export function CatalogRetry({error,retry}:{error:string;retry:()=>void}){return <div className="catalog-read-error"><Alert>{error}</Alert><button onClick={retry}>重新读取</button></div>}
export function CatalogFreshnessNote({freshness}:{freshness:CatalogFreshness}){
    const text=freshness.state==='EXPIRED'?'目录来源已过期，暂时停用接入操作。保留上次核验的信息供查阅。':freshness.state==='STALE'?'目录来源较久未更新，请在接入前留意最新信息。':freshness.state==='NEVER_SYNCED'?'目录尚未完成首次同步。':'目录基于最近一次完整核验。';
    return <div className={'catalog-freshness '+(freshness.state==='CURRENT'?'current':'warning')}><p>{text}</p><small>来源观察：{catalogTime(freshness.last_observed_at)} · 平台核验：{catalogTime(freshness.last_verified_at)}</small></div>;
}
const familyPersonaID=(family:string)=>'persona_'+(family==='ernie'?'wenxin':family)+'_master';
export function CatalogPersona({model,assets=[],compact=false}:{model:CatalogModel;assets?:CatalogAsset[];compact?:boolean}){
    const approved=assets.filter(a=>a.status==='PRODUCTION_READY'&&['ORIGINAL_PLATFORM','ORIGINAL_GENERATED','LICENSED_OR_APPROVED'].includes(a.rights_status));
    const asset=approved.find(a=>a.asset_id===familyPersonaID(model.metadata.family))||approved.find(a=>a.asset_id===model.metadata.asset_id);
    const cover=model.family_cover;
    const [failure,setFailure]=useState(0);
    useEffect(()=>setFailure(0),[model.metadata.family,cover?.version,cover?.image?.src,cover?.default_image?.src,asset?.src,asset?.fallback]);
    const legacy=asset?[{src:asset.src,alt:'',width:1024,height:1536,focal_point:asset.focal_point},...(asset.fallback!==asset.src?[{src:asset.fallback,alt:'',width:1024,height:1536,focal_point:asset.focal_point}]:[])]:[];
    const choices=cover===undefined?legacy:cover.family===model.metadata.family?[cover.image,cover.default_image].filter((a,i,all)=>a&&all.findIndex(b=>b?.src===a.src)===i):[];
    const image=choices.filter(a=>a&&validCatalogCoverSrc(a.src))[failure];
    return <figure className={'catalog-persona family-'+model.metadata.family+(compact?' compact':'')+(image?' has-image':'')} aria-label={image?'模型家族形象':'模型家族形象未载入'}>
        {image?<img src={assetUrl(image.src)} alt={image.alt} width={image.width||undefined} height={image.height||undefined} loading="lazy" onError={()=>setFailure(n=>n+1)} style={{objectPosition:image.focal_point.map(n=>n*100+'%').join(' ')}}/>:<p className="catalog-art-unavailable">家族形象暂未载入</p>}
    </figure>;
}
function ChoiceLabels({values,choices}:{values:string[];choices:CatalogChoice[]}){return <>{values.map(value=><span className="catalog-tag" key={value}>{choices.find(c=>c.value===value)?.label||value}</span>)}</>}
const conditions:Record<string,string>={uncached_plain_text_tokens:'未命中缓存的纯文本输入',plain_text_output_tokens:'纯文本输出',only_if_native_reports_billable_cache_read_tokens:'原生返回可计费缓存读取用量时',only_if_native_reports_billable_generic_cache_write_tokens:'原生返回可计费缓存写入用量时',only_if_native_reports_anthropic_5m_cache_write_tokens:'原生返回 5 分钟缓存写入用量时',only_if_native_reports_anthropic_1h_cache_write_tokens:'原生返回 1 小时缓存写入用量时',plain_text_request_without_extra_multipliers_or_tool_fees:'无额外倍率或工具费用的纯文本请求'};
export function CatalogPriceTable({price,personal=false}:{price:CatalogPrice;personal?:boolean}){
    return <div className="catalog-prices">
        <p className="catalog-price-basis">{personal?'本人会话参考价':'原生 default 公开参考价'} <span>{price.status==='conditional'?'· 条件计价':price.status==='unquotable'?'· 暂无可核对数值':''}</span></p>
        {price.dimensions.length?<dl className="catalog-price-list">{price.dimensions.map(d=><div key={d.kind}><dt>{dimensionLabels[d.kind]||d.kind}<small>{d.unit==='API_Credit_per_request'?'API Credit / 次请求':'API Credit / 百万 tokens'}</small></dt><dd><strong>{decimalDisplay(d.amount)}</strong><small>{conditions[d.condition]||'以原生结算条件为准'}</small></dd></div>)}</dl>:<p className="catalog-unknown">未提供数值报价</p>}
        <p className="hint">API Credit 是原生美元计价的记账单位，不表示汇率换算。额外图像、音频、工具等费用及整数额度取整未包含；最终金额由原生结算确定。</p>
    </div>;
}
function familyPath(family:string){
    return '/models?'+new URLSearchParams({view:'family',family}).toString();
}
export function CatalogCard({model,vocabulary}:{model:CatalogModel;vocabulary:CatalogVocabulary}){
    const label=vocabulary.families.find(f=>f.value===model.metadata.family)?.label||model.metadata.family;
    return <article className="catalog-family-card"><Link to={familyPath(model.metadata.family)} aria-label={label+' · 查看家族型号'}><CatalogPersona model={model} assets={vocabulary.assets} compact/><h2>{label}</h2></Link></article>;
}
export function CatalogHomeRecommendations({client}:{client:ApiClient}){
    const r=useResource(()=>readCatalog(client,'?recommended=true&group_by=family&limit=3'),[client]);
    return <section className="catalog-home-recommendations" aria-labelledby="recommended-models"><div className="section-heading"><div><p className="eyebrow">CURATED CONNECTIONS</p><h2 id="recommended-models">值得探索的模型</h2></div><Link className="text-link" to="/models">探索全部 →</Link></div>{r.loading?<CatalogLoading/>:r.error?<CatalogRetry error={r.error} retry={r.reload}/>:r.data?.items.length?<div className="catalog-grid">{r.data.items.map(model=><CatalogCard key={model.model_id} model={model} vocabulary={r.data!.vocabulary}/>)}</div>:<Empty title="精选模型正在整理">经审核发布并推荐的模型会出现在这里。</Empty>}</section>;
}
function CatalogFilters({query,vocabulary,onApply}:{query:URLSearchParams;vocabulary?:CatalogVocabulary;onApply:(q:URLSearchParams)=>void}){
    const [priceDimension,setPriceDimension]=useState(query.get('price_dimension')||'');
    const [unknownContext,setUnknownContext]=useState(query.get('unknown_context')==='true');
    function submit(e:FormEvent<HTMLFormElement>){e.preventDefault();const next=new URLSearchParams();for(const [key,value] of new FormData(e.currentTarget)){if(typeof value==='string'&&value.trim())next.set(key,value.trim())}if(!priceDimension){next.delete('min_price');next.delete('max_price');if(next.get('sort')==='price')next.set('sort','recommended')}if(unknownContext)next.delete('min_context');onApply(next)}
    const choices=(name:string,label:string,items:CatalogChoice[]=[]) => <label>{label}<select name={name} defaultValue={query.get(name)||''}><option value="">全部</option>{items.map(c=><option key={c.value} value={c.value}>{c.label}</option>)}</select></label>;
    return <form className="catalog-filter-form" onSubmit={submit}><label className="catalog-filter-search">名称或模型 ID<input name="q" defaultValue={query.get('q')||''} maxLength={200} placeholder="搜索展示名或完整模型 ID"/></label><div className="catalog-filter-fields">{choices('family','模型家族',vocabulary?.families)}{choices('availability','接入配置状态',Object.entries(availabilityLabels).map(([value,label])=>({value,label})))}{choices('tag','能力标签',vocabulary?.tags)}{choices('use_case','推荐用途',vocabulary?.use_cases)}<label>最小 context<input name="min_context" type="number" min="1" max="9007199254740991" step="1" defaultValue={query.get('min_context')||''} disabled={unknownContext}/></label><label>比较价格维度<select name="price_dimension" value={priceDimension} onChange={e=>setPriceDimension(e.target.value)}><option value="">不比较价格</option>{Object.entries(dimensionLabels).map(([value,label])=><option key={value} value={value}>{label}{value==='text_request_base'?' / 次请求':' / 百万 tokens'}</option>)}</select></label><label>最低价格<input name="min_price" inputMode="decimal" pattern="(0|[1-9][0-9]*)(\.[0-9]+)?" defaultValue={query.get('min_price')||''} disabled={!priceDimension}/></label><label>最高价格<input name="max_price" inputMode="decimal" pattern="(0|[1-9][0-9]*)(\.[0-9]+)?" defaultValue={query.get('max_price')||''} disabled={!priceDimension}/></label><label>排序<select name="sort" defaultValue={query.get('sort')||'recommended'}><option value="recommended">推荐优先</option><option value="name">名称</option><option value="context">Context 从高到低</option><option value="price" disabled={!priceDimension}>所选维度价格从低到高</option></select></label></div><div className="catalog-filter-checks"><label className="checkbox"><input type="checkbox" name="recommended" value="true" defaultChecked={query.get('recommended')==='true'}/>仅显示精选</label><label className="checkbox"><input type="checkbox" name="unknown_context" value="true" checked={unknownContext} onChange={e=>setUnknownContext(e.target.checked)}/>仅显示未知 context</label></div><p className="hint">价格只在所选维度与同一单位内比较；缺失值不计为零。</p><div className="catalog-actions"><button type="submit" className="primary">应用筛选</button><button type="button" onClick={()=>onApply(new URLSearchParams())}>重置筛选</button></div></form>;
}
export function Catalog(props:CatalogProps){
    const [query]=useSearchParams();
    return query.get('view')==='family'?<CatalogFamily key={query.get('family')} {...props}/>:<CatalogDirectory {...props}/>;
}
function CatalogDirectory({client}:CatalogProps){
    const [query,setQuery]=useSearchParams();const [filters,setFilters]=useState(false);
    const request=new URLSearchParams(query);request.set('group_by','family');
    const queryString=request.toString();const r=useResource(()=>readCatalog(client,'?'+queryString),[client,queryString]);
    const apply=(next:URLSearchParams)=>{setQuery(next);setFilters(false)};
    return <CatalogShell client={client} title="模型目录"><section className="workshop-main workshop-directory">
        <header className="workshop-heading"><p className="eyebrow">DA VINCI'S WORKSHOP / MODEL FAMILIES</p><h1>模型工房</h1><p>选择一位搭档，开启下一次创造。</p></header>
        <div className="catalog-toolbar"><p>{r.data?`${r.data.total} 个模型家族`:'探索模型家族'}</p><button onClick={()=>setFilters(true)}>搜索与筛选</button><Link className="text-link" to="/api/access">API 接入指南 ↗</Link></div>
        {filters&&<Modal title="筛选模型" onClose={()=>setFilters(false)}><CatalogFilters key={queryString} query={query} vocabulary={r.data?.vocabulary} onApply={apply}/></Modal>}
        {r.loading?<CatalogLoading/>:r.error?<CatalogRetry error={r.error} retry={r.reload}/>:r.data&&<>
            {r.data.freshness.state!=='CURRENT'&&<CatalogFreshnessNote freshness={r.data.freshness}/>}
            {r.data.items.length?<div className="catalog-grid" aria-label="模型家族" tabIndex={0}>{r.data.items.map(model=><CatalogCard key={model.metadata.family} model={model} vocabulary={r.data!.vocabulary}/>)}</div>:<Empty title={query.toString()?'没有符合条件的模型家族':'模型目录正在整理'}>调整筛选条件，或稍后再来看看。</Empty>}
            <div className="workshop-list-foot"><small>具体型号、渠道与参考价格，请进入家族查看。</small><Pager page={Math.floor(r.data.offset/r.data.limit)+1} total={r.data.total} size={r.data.limit} onChange={page=>{const next=new URLSearchParams(query);next.set('offset',String((page-1)*r.data!.limit));setQuery(next)}}/></div>
            {r.data.freshness.state==='CURRENT'&&<CatalogFreshnessNote freshness={r.data.freshness}/>}
        </>}
    </section><aside className="workshop-host-note" aria-label="达·芬奇的模型工房"><p>LEONARDO DA VINCI</p><span>每一次灵感，<br/>都值得一个好搭档。</span></aside></CatalogShell>;
}
function CatalogFamily({client,onLoginRequired}:CatalogProps){
    const [query,setQuery]=useSearchParams();const family=query.get('family')||'';
    const request=new URLSearchParams({family});for(const key of ['offset','limit']){const value=query.get(key);if(value)request.set(key,value)}
    const requestKey=request.toString();const r=useResource(()=>family?readCatalog(client,'?'+requestKey):Promise.reject(new Error('请选择模型家族。')),[client,requestKey,family]);
    const id=selectedModel(query.toString())||r.data?.items[0]?.model_id;
    const label=r.data?.vocabulary.families.find(f=>f.value===family)?.label||family;
    const choose=(id:string)=>{const next=new URLSearchParams(query);next.set('model_id',id);setQuery(next)};
    const portrait=r.data?.items[0];
    return <CatalogShell client={client} title={label+' · 模型家族'}><section className="workshop-main workshop-family">
        <Link className="catalog-back" to="/models">← 返回模型目录</Link>
        <header className="workshop-heading"><p className="eyebrow">FAMILY PROFILE</p><h1>{label}</h1><p>同一个家族，多种连接方式。选择具体型号后查看详情。</p></header>
        {r.loading?<CatalogLoading/>:r.error?<CatalogRetry error={r.error} retry={r.reload}/>:r.data&&<div className="workshop-family-grid">
            {portrait&&<aside className="workshop-family-portrait"><CatalogPersona model={portrait} assets={r.data.vocabulary.assets}/><h2>{label}</h2><p>MODEL FAMILY</p></aside>}
            <div className="workshop-family-content"><section className="workshop-model-picker" aria-label="选择型号"><div className="section-heading"><h2>选择型号</h2><small>{r.data.total} 个公开型号</small></div>
                <div className="workshop-model-options">{r.data.items.map(m=><button key={m.model_id} className={'workshop-model-option'+(m.model_id===id?' selected':'')} aria-pressed={m.model_id===id} onClick={()=>choose(m.model_id)}><span>{m.metadata.display_name}</span><code>{m.model_id}</code><small>{availabilityLabels[m.availability_state]||'状态待核对'}</small></button>)}</div>
                <Pager page={Math.floor(r.data.offset/r.data.limit)+1} total={r.data.total} size={r.data.limit} onChange={page=>{const next=new URLSearchParams(query);next.delete('model_id');next.set('offset',String((page-1)*r.data!.limit));setQuery(next)}}/>
                {!r.data.items.length&&<Empty title="没有匹配的型号"><Link to={familyPath(family)}>清除筛选条件</Link></Empty>}
            </section>{id&&<CatalogSelectedDetail key={id} client={client} id={id} family={family} onLoginRequired={onLoginRequired}/>}</div>
        </div>}
    </section></CatalogShell>;
}
function CatalogUseButton({client,model,onLoginRequired}:{client:ApiClient;model:CatalogModel;onLoginRequired?:CatalogLoginRequired}){
    const login=useCatalogLogin(onLoginRequired);const navigate=useNavigate();const [busy,setBusy]=useState(false);const [error,setError]=useState('');const lock=useRef(false);const active=useRef(true);
    useEffect(()=>{active.current=true;return()=>{active.current=false}},[]);
    async function use(){if(lock.current)return;lock.current=true;setBusy(true);setError('');try{await useCatalogModel(client,model.model_id,navigate,login,()=>active.current)}catch(e){if(active.current)setError(catalogError(e))}finally{lock.current=false;if(active.current)setBusy(false)}}
    return <div className="catalog-use"><button className="primary" disabled={!model.can_use||busy} onClick={()=>void use()}>{busy?'正在确认接入方式…':model.can_use?'使用此模型':'暂不可接入'}</button>{error&&<Alert>{error}</Alert>}{!model.can_use&&<p className="hint">模型信息保留供查阅，接入配置或目录时效仍需核对。</p>}</div>;
}
function CatalogPersonalPrice({client,id}:{client:ApiClient;id:string}){
    const r=useResource(()=>readPersonalPrice(client,id),[client,id]);
    return <section className="catalog-detail-section"><h2>本人会话报价</h2>{r.loading?<p role="status">正在核对本人报价…</p>:r.error?<CatalogRetry error={r.error} retry={r.reload}/>:r.data&&<><p className="hint">{r.data.basis==='eligible_auto_candidates_not_selected'?'以下是可选 auto 候选，尚未为调用选定最终组。':'基于当前账户组，仅作本人会话参考；不代表某枚密钥已经选择该组。'}</p>{r.data.quotes.map(q=><div key={q.candidate} className="catalog-personal-candidate">{r.data!.quotes.length>1&&<h3>候选 {q.candidate}</h3>}{q.price?<CatalogPriceTable price={q.price} personal/>:<p>该候选未配置此模型，暂无数值报价。</p>}</div>)}<small>报价观察：{catalogTime(r.data.observed_at)}</small></>}</section>;
}
function CatalogModelInformation({client,model,vocabulary,onLoginRequired}:{client:ApiClient;model:CatalogModel;vocabulary:CatalogVocabulary;onLoginRequired?:CatalogLoginRequired}){
    const session=useSyncExternalStore(client.subscribe,client.getSnapshot);const epoch=client.getSessionGeneration();
    return <div className="catalog-detail-info">
        <header><p className="eyebrow">MODEL SPECIFICATION</p><h2>{model.metadata.display_name}</h2><code className="catalog-model-id">{model.model_id}</code><p className="catalog-detail-summary">{model.metadata.summary}</p><span className="catalog-availability">{availabilityLabels[model.availability_state]}</span><div className="catalog-tags"><ChoiceLabels values={model.metadata.tags} choices={vocabulary.tags}/></div></header>
        <div className="workshop-use-actions"><CatalogUseButton client={client} model={model} onLoginRequired={onLoginRequired}/><Link className="text-link" to={catalogSelectionPath(model.model_id,false)}>查看接入示例 →</Link></div>
        <section className="catalog-detail-section"><h2>能力与用途</h2><p>{model.metadata.context_length?decimalDisplay(model.metadata.context_length)+' context tokens':'Context 未知'}</p><div className="catalog-tags"><ChoiceLabels values={model.metadata.use_cases} choices={vocabulary.use_cases}/></div>{!model.metadata.use_cases.length&&<p className="hint">推荐用途尚未补充。</p>}</section>
        <section className="catalog-detail-section"><h2>价格与计量</h2><CatalogPriceTable price={model.price}/><p className="hint">价格与端点最后观察：{catalogTime(model.last_seen_at)}</p>{model.metadata.special_pricing_note&&<p className="catalog-special-price">{model.metadata.special_pricing_note}</p>}</section>
        {session.user&&<CatalogPersonalPrice key={session.user.id+':'+epoch+':'+model.model_id} client={client} id={model.model_id}/>}
        <section className="catalog-detail-section"><h2>接入方式</h2>{model.endpoints.length?<ul className="catalog-endpoints">{model.endpoints.map(ep=><li key={ep.kind}><strong>{endpointLabels[ep.kind]}</strong><code>{ep.method} {ep.path}</code></li>)}</ul>:<p>来源尚未提供可核验的接入端点。</p>}<p className="hint">配置支持不代表实时调用健康；模型缺席时保留最后观察到的配置。</p></section>
    </div>;
}
function CatalogSelectedDetail({client,id,family,onLoginRequired}:{client:ApiClient;id:string;family?:string;onLoginRequired?:CatalogLoginRequired}){
    useSyncExternalStore(client.subscribe,client.getSnapshot);const epoch=client.getSessionGeneration();
    const r=useResource(async()=>{const data=await readCatalogDetail(client,id);if(family&&data.item.metadata.family!==family)throw new Error('此型号不属于当前家族，请重新选择。');return data},[client,id,family,epoch]);
    return <section className="workshop-selected-detail" aria-label="型号详情">{r.loading?<CatalogLoading/>:r.error?<CatalogRetry error={r.error} retry={r.reload}/>:r.data&&<><CatalogFreshnessNote freshness={r.data.item.freshness}/><CatalogModelInformation client={client} model={r.data.item} vocabulary={r.data.vocabulary} onLoginRequired={onLoginRequired}/></>}</section>;
}
export function CatalogDetailPage({client,onLoginRequired}:CatalogProps){
    const location=useLocation();const id=decodeCatalogModelPath(location.pathname);
    return <CatalogShell client={client} title="模型详情"><section className="workshop-main workshop-legacy-detail"><Link className="catalog-back" to="/models">← 返回模型目录</Link><header className="workshop-heading"><p className="eyebrow">MODEL PROFILE</p><h1>模型详情</h1></header>{id?<CatalogSelectedDetail key={id} client={client} id={id} onLoginRequired={onLoginRequired}/>:<Alert>此模型暂不可访问。</Alert>}</section></CatalogShell>;
}
function CatalogAccessBody({client,id,onLoginRequired,intent}:{client:ApiClient;id:string;onLoginRequired?:CatalogLoginRequired;intent:boolean}){
    const session=useSyncExternalStore(client.subscribe,client.getSnapshot);const epoch=client.getSessionGeneration();const navigate=useNavigate();const login=useCatalogLogin(onLoginRequired);
    const r=useResource(()=>readCatalogDetail(client,id),[client,id,epoch]);const [endpoint,setEndpoint]=useState('');const [notice,setNotice]=useState('');const [continuation,setContinuation]=useState('');const once=useRef('');const active=useRef(true);
    useEffect(()=>{active.current=true;return()=>{active.current=false}},[]);
    useEffect(()=>{if(!intent||!session.ready||!session.user||!r.data?.item.can_use)return;const key=id+':'+epoch;if(once.current===key)return;once.current=key;void useCatalogModel(client,id,navigate,login,()=>active.current).catch(e=>{if(active.current)setContinuation(catalogError(e))})},[intent,session.ready,session.user,id,epoch,r.data,navigate,login]);
    const ep=r.data?.item.endpoints.find(e=>e.kind===endpoint)||r.data?.item.endpoints[0];
    const curl=ep&&r.data?catalogCurl(r.data.api_base_url,id,ep):'';
    async function copy(value:string,label:string){try{await navigator.clipboard.writeText(value);setNotice(label+'已复制。')}catch{setNotice('剪贴板不可用，请选择下方文本手动复制。')}}
    return r.loading?<CatalogLoading/>:r.error?<CatalogRetry error={r.error} retry={r.reload}/>:r.data?<><CatalogFreshnessNote freshness={r.data.item.freshness}/><section className="catalog-access-selected"><div><p className="eyebrow">SELECTED MODEL</p><h2>{r.data.item.metadata.display_name}</h2><code>{id}</code></div><Link className="text-link" to={catalogModelPath(id)}>模型详情 ↗</Link></section><div className="catalog-access-grid"><section className="catalog-access-config"><h2>连接设置</h2><label>Base URL<input readOnly value={r.data.api_base_url} placeholder="部署尚未配置 API Base URL" onFocus={e=>e.target.select()}/></label>{r.data.api_base_url&&<button onClick={()=>void copy(r.data!.api_base_url,'Base URL')}>复制 Base URL</button>}<label>Endpoint<select value={ep?.kind||''} onChange={e=>{setEndpoint(e.target.value);setNotice('')}} disabled={!r.data.item.endpoints.length}>{!r.data.item.endpoints.length&&<option value="">尚无可核验端点</option>}{r.data.item.endpoints.map(e=><option key={e.kind} value={e.kind}>{endpointLabels[e.kind]} · {e.path}</option>)}</select></label><p className="hint">端点来自已核验的配置，不表示实时调用健康。</p><h3>认证</h3><p>在可信客户端中，将 <code>&lt;YOUR_API_KEY&gt;</code> 替换为你主动创建的 API 密钥。</p><CatalogUseButton client={client} model={r.data.item} onLoginRequired={onLoginRequired}/><div className="catalog-access-links"><Link to={'/keys?'+new URLSearchParams({model_id:id})}>管理 API 密钥 →</Link><Link to="/logs">查看调用记录 →</Link></div>{continuation&&<Alert>{continuation}</Alert>}</section><section className="catalog-access-example"><div className="section-heading"><div><p className="eyebrow">REQUEST EXAMPLE</p><h2>cURL 示例</h2></div>{curl&&<button onClick={()=>void copy(curl,'cURL')}>复制 cURL</button>}</div>{curl?<><textarea aria-label="cURL 示例代码" readOnly spellCheck={false} rows={18} value={curl} onFocus={e=>e.target.select()}/><p className="hint">适用于 POSIX shell（如 Bash）。示例使用占位密钥，复制不会发起模型调用。</p></>:<Empty title="接入示例暂不可用">需要已配置的 API Base URL 与可核验的 Endpoint。</Empty>}{notice&&<p role="status" className="notice">{notice}</p>}</section></div></>:null;
}
export function APIAccess({client,onLoginRequired}:CatalogProps){
    const [query,setQuery]=useSearchParams();const location=useLocation();const id=normalizeRouteIntent('/api/access'+location.search)?selectedModel(query.toString()):null;const [search,setSearch]=useState('');const [term,setTerm]=useState('');
    const r=useResource(()=>readCatalog(client,'?limit=100'+(term?'&q='+encodeURIComponent(term):'')),[client,term]);
    const config=useResource(()=>client.catalogRequest<{api_base_url:string}>('/platform/v1/models/access-config'),[client]);
    return <CatalogShell client={client} title="API 接入"><section className="workshop-main workshop-access"><header className="workshop-heading"><p className="eyebrow">DA VINCI'S WORKSHOP / API ACCESS</p><h1>连接你的创造</h1><p>选择型号，核对设置，在你信任的客户端中连接。</p></header><section className="catalog-access-picker"><form onSubmit={e=>{e.preventDefault();setTerm(search.trim())}}><label>搜索要接入的模型<input value={search} onChange={e=>setSearch(e.target.value)} maxLength={200} placeholder="名称或模型 ID"/></label><button type="submit">查找模型</button></form>{r.loading?<CatalogLoading/>:r.error?<CatalogRetry error={r.error} retry={r.reload}/>:r.data&&<><label>选择模型<select value={id||''} onChange={e=>{const next=new URLSearchParams();if(e.target.value)next.set('model_id',e.target.value);setQuery(next)}}><option value="">请选择模型</option>{id&&!r.data.items.some(m=>m.model_id===id)&&<option value={id}>当前查询中的模型</option>}{r.data.items.map(model=><option key={model.model_id} value={model.model_id}>{model.metadata.display_name} · {model.model_id}</option>)}</select></label>{r.data.total>r.data.items.length&&<p className="hint">当前显示前 {r.data.items.length} 项，请搜索以缩小范围。</p>}{!r.data.items.length&&<p className="hint">没有匹配的公开模型，可调整搜索条件。</p>}</>}</section>{id?<CatalogAccessBody key={id} client={client} id={id} intent={query.get('intent')==='use'} onLoginRequired={onLoginRequired}/>:<section className="catalog-access-intro"><h2>连接前，准备好三项设置</h2><ol><li><strong>API Base URL</strong><code>{config.data?.api_base_url||'部署尚未配置'}</code></li><li><strong>模型 ID</strong><span>从上方目录选择，保留完整 ID。</span></li><li><strong>API 密钥</strong><span>在密钥管理中主动创建，妥善保管。</span></li></ol>{config.error&&<CatalogRetry error={config.error} retry={config.reload}/>}<Link to="/models" className="text-link">先探索模型能力 →</Link></section>}</section></CatalogShell>;
}
