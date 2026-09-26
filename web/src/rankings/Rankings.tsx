import { useEffect, useRef, useState, useSyncExternalStore } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { ApiClient } from '../api';
import { Alert, Brand, Empty, Modal, useResource } from '../ui';
import { saveRouteIntent } from '../post-auth-intent';
import { assetSrcSet, assetUrl } from '../game-hall-assets';
import art from './rankings-art.json';
import './rankings.css';

const labels:Record<string,string>={TOTAL_ASSETS:'总资产',GAME_PROFIT:'游戏净盈利',BIGGEST_WIN:'单局最高净收益',TOTAL_WAGERED:'累计下注',POKER_PROFIT:'德州已实现盈利',RP_CALLS:'成功调用',RP_ERRORS:'失败调用',RP_CREDITS:'实际消耗'};
type Model={model_id:string;display_name:string;calls:string;errors:string;credits_units:string};
type Badge={code:string;name:string;icon_path:string;description:string};
type Row={badges?:Badge[];rank:string;display_name:string;avatar_id:string;value:string;calls:string;errors:string;credits_units:string;models:Model[]};
type Page={state:'READY'|'STALE'|'UNAVAILABLE';metric:string;period:string;period_start:string;period_end:string|null;last_updated:string|null;items:Row[];total:string;page:number;page_size:number;historical_periods:string[];my_rank?:Row};
const decimal=(value:unknown):value is string=>typeof value==='string'&&/^-?(0|[1-9][0-9]{0,37})$/.test(value);
const unsigned=(value:unknown):value is string=>decimal(value)&&!value.startsWith('-');
const gamblerBadge:Badge={code:'gambler-ruler-202609',name:'赌怪',icon_path:'ui/badges/gambler-ruler.e0c69ec268a86ee0.png',description:'纪念资产调整前总资产超过十亿的御主'};
function checkRow(row:Row){
    if(!row||!unsigned(row.rank)||BigInt(row.rank)<1n||typeof row.display_name!=='string'||row.avatar_id!=='system-default'||!decimal(row.value)||!unsigned(row.calls)||!unsigned(row.errors)||!unsigned(row.credits_units)||!Array.isArray(row.models))throw new Error('排行响应格式异常。');
    if(row.badges!==undefined&&(!Array.isArray(row.badges)||row.badges.length>1||row.badges.some(badge=>!badge||badge.code!==gamblerBadge.code||badge.icon_path!==gamblerBadge.icon_path||badge.name!==gamblerBadge.name||badge.description!==gamblerBadge.description)))throw new Error('勋章响应格式异常。');
    for(const model of row.models)if(!model||typeof model.model_id!=='string'||typeof model.display_name!=='string'||!unsigned(model.calls)||!unsigned(model.errors)||!unsigned(model.credits_units))throw new Error('模型排行响应格式异常。');
}
async function read(client:ApiClient,query:URLSearchParams,mine:boolean){
    const data=await client.rankingsRequest<Page>(query,mine);
    if(!data||!['READY','STALE','UNAVAILABLE'].includes(data.state)||!Array.isArray(data.items)||!unsigned(data.total)||!Number.isSafeInteger(data.page)||data.page<1||data.page_size!==50||!Array.isArray(data.historical_periods)||data.historical_periods.some(value=>typeof value!=='string'||!/^\d{4}-\d{2}-\d{2}$/.test(value))||data.last_updated!==null&&!Number.isFinite(Date.parse(data.last_updated)))throw new Error('排行响应格式异常。');
    if(data.metric!==query.get('metric')||data.period!==query.get('period'))throw new Error('排行筛选与响应不匹配。');
    data.items.forEach(checkRow);if(data.my_rank)checkRow(data.my_rank);return data;
}
function credits(value:string){const n=BigInt(value),absolute=n<0n?-n:n;const fraction=((absolute%500000n)*2n).toString().padStart(6,'0').replace(/0+$/,'');return (n<0n?'-':'')+(absolute/500000n).toLocaleString('zh-CN')+(fraction?'.'+fraction:'');}
function count(value:string){return BigInt(value).toLocaleString('zh-CN');}
function errorRate(row:{calls:string;errors:string}){const total=BigInt(row.calls)+BigInt(row.errors);if(total===0n)return '—';const hundredths=BigInt(row.errors)*10000n/total;return `${hundredths/100n}.${(hundredths%100n).toString().padStart(2,'0')}%`;}
function valueText(metric:string,value:string){return metric==='RP_CALLS'||metric==='RP_ERRORS'?count(value):credits(value);}
function Models({row,metric}:{row:Row;metric:string}){
    const field=metric==='RP_ERRORS'?'errors':metric==='RP_CREDITS'?'credits_units':'calls';
    const sorted=[...row.models].sort((a,b)=>BigInt(a[field])===BigInt(b[field])?a.model_id.localeCompare(b.model_id):BigInt(a[field])>BigInt(b[field])?-1:1);
    const positive=sorted.filter(model=>BigInt(model[field])>0n);
    const top=positive.slice(0,3),other=positive.slice(3).reduce((sum,model)=>sum+BigInt(model[field]),0n);
    return <><span>{top.map(model=>model.display_name||model.model_id).join(' · ')||'—'}{other>0n&&` · Other ${field==='credits_units'?credits(other.toString()):count(other.toString())}`}</span>{sorted.length>0&&<details><summary>完整模型分布</summary><ul>{sorted.map(model=><li key={model.model_id}><strong>{model.display_name||model.model_id}</strong><code>{model.model_id}</code><small>成功 {count(model.calls)} · 失败 {count(model.errors)} · {credits(model.credits_units)} Credit</small></li>)}</ul></details>}</>;
}
function PersonalMedal({badge}:{badge:Badge}){
    const [open,setOpen]=useState(false),[failed,setFailed]=useState(false);
    return <><button className="ranking-personal-medal" aria-label="赌怪勋章说明" aria-haspopup="dialog" onClick={()=>setOpen(true)}>{!failed&&<img src={assetUrl(badge.icon_path)} width={32} height={32} alt="阿尔托莉雅·Ruler 赌场兔女郎勋章" onError={()=>setFailed(true)}/>}<span>{badge.name}</span></button>{open&&<Modal title={badge.name} onClose={()=>setOpen(false)}><p>{badge.description}</p><p>永久纪念勋章，与当前名次或余额无关；各榜单展示当前持有状态。</p></Modal>}</>;
}
export function Rankings({client}:{client:ApiClient}) {
    const session=useSyncExternalStore(client.subscribe,client.getSnapshot);
    const [params,setParams]=useSearchParams();
    const requested=params.get('metric')||'TOTAL_ASSETS',metric=labels[requested]?requested:'TOTAL_ASSETS';
    const rp=metric.startsWith('RP_');
    const requestedPeriod=params.get('period')||'DAY',period=metric==='TOTAL_ASSETS'?'CURRENT':['DAY','WEEK','ALL_TIME'].includes(requestedPeriod)?requestedPeriod:'DAY';
    const pageValue=Number(params.get('page')||'1'),page=Number.isSafeInteger(pageValue)&&pageValue>0&&pageValue<=10000?pageValue:1;
    const model=rp?(params.get('model')||''):'';
    const date=period==='DAY'||period==='WEEK'?(params.get('date')||''):'';
    const [modelDraft,setModelDraft]=useState(model);
    const [refresh,setRefresh]=useState(0);
    const [info,setInfo]=useState(false);
    const main=useRef<HTMLElement>(null);
    useEffect(()=>{document.title='排行榜 · Chaldea';main.current?.focus()},[]);
    useEffect(()=>{setModelDraft(model)},[model]);
    // Poll the published snapshot; this GET never triggers aggregation or publication.
    useEffect(()=>{const timer=setInterval(()=>setRefresh(value=>value+1),60000);return()=>clearInterval(timer)},[]);
    const query=new URLSearchParams({metric,period,page:String(page)});if(model)query.set('model',model);if(date)query.set('date',date);
    const serialized=query.toString(),generation=client.getSessionGeneration();
    const result=useResource(()=>read(client,new URLSearchParams(serialized),false),[client,serialized,generation,refresh]);
    const mine=useResource(()=>session.user?read(client,new URLSearchParams(serialized),true):Promise.resolve(null),[client,serialized,generation,session.user?.id,refresh]);
    function change(values:Record<string,string>){const next=new URLSearchParams(serialized);next.delete('page');for(const [key,value]of Object.entries(values)){if(value)next.set(key,value);else next.delete(key)}setParams(next);}
    function chooseMetric(next:string){change({metric:next,period:next==='TOTAL_ASSETS'?'CURRENT':period==='CURRENT'?'DAY':period,date:''})}
    const data=result.data;
    const unit=metric==='RP_CREDITS'?'Credit':metric==='TOTAL_ASSETS'?'Credit 等值':rp?'次':'筹码';
    return <div className="ranking-workspace">
      <a className="skip-link" href="#ranking-content">跳至排行榜</a>
      <header className="portal-header ranking-header">
        <Link to="/" className="brand" aria-label="Chaldea Platform 首页"><Brand/></Link>
        <nav className="portal-global" aria-label="主导航"><Link to="/dashboard">指挥台</Link><Link to="/models">模型目录</Link><Link to="/entertainment">娱乐</Link><Link to="/rankings" aria-current="page">排行榜</Link><Link to="/announcements">公告</Link></nav>
        <div className="ranking-account"><Link to="/wallet">资产</Link><Link to={session.user?'/me':'/login'} onClick={()=>{if(!session.user)saveRouteIntent('/rankings?'+serialized)}}>{session.user?'个人中心':'登录账户'}</Link></div>
      </header>
      <main ref={main} tabIndex={-1} id="ranking-content" className={'ranking-main'+(rp?' ranking-rp':'')}>
        <img className="ranking-background" src={assetUrl(art.background.src)} srcSet={assetSrcSet(art.background)} sizes="100vw" width={art.background.width} height={art.background.height} alt="梅塔特隆·贞德，第二再临形态，戴着耳机倚坐在龙骨游戏座椅上" fetchPriority="high"/>
        <div className="ranking-body">
          <header className="ranking-heading"><p className="eyebrow">CHALDEA / RANKINGS</p><h1>迦勒底排行榜</h1><p>每小时更新 · 公开榜单</p></header>
          <nav className="ranking-domains" aria-label="排行类别"><button aria-pressed={!rp} onClick={()=>change({metric:'TOTAL_ASSETS',period:'CURRENT',date:'',model:''})}>资产与游戏</button><button aria-pressed={rp} onClick={()=>change({metric:'RP_CALLS',period:'DAY',date:'',model:''})}>调用统计</button></nav>
          <nav className="ranking-metrics" aria-label="排行指标">{Object.entries(labels).filter(([key])=>key.startsWith('RP_')===rp).map(([key,label])=><button key={key} aria-pressed={metric===key} onClick={()=>chooseMetric(key)}>{label}</button>)}</nav>
          <section className="ranking-board" aria-label="榜单内容">
            <div className="ranking-filters">
              {metric==='TOTAL_ASSETS'?<span className="ranking-current">当前资产</span>:<><label>周期<select value={period} onChange={event=>change({period:event.target.value,date:''})}><option value="DAY">日榜</option><option value="WEEK">周榜</option><option value="ALL_TIME">全部时间</option></select></label>{period!=='ALL_TIME'&&<label>历史周期<select value={date} onChange={event=>change({date:event.target.value})}><option value="">当前周期</option>{data?.historical_periods.map(value=><option key={value} value={value}>{value}</option>)}</select></label>}</>}
              <div className="ranking-update"><span>{data?.last_updated?<><span>更新时间 </span><time dateTime={data.last_updated}>{new Date(data.last_updated).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai',hour12:false})}</time><span> · UTC+8</span></>:'等待已发布快照'}</span><button className="ranking-refresh" aria-label="刷新排行" title="读取最新已发布榜单" onClick={()=>{result.reload();mine.reload()}} disabled={result.loading}>刷新</button></div>
              {rp&&<form onSubmit={event=>{event.preventDefault();change({model:modelDraft.trim()})}}><label>筛选模型 ID<input value={modelDraft} maxLength={256} onChange={event=>setModelDraft(event.target.value)} placeholder="全部模型"/></label><button type="submit">应用</button>{model&&<button type="button" onClick={()=>{setModelDraft('');change({model:''})}}>清除</button>}</form>}
            </div>
            <div className="ranking-scroll" tabIndex={0} role="region" aria-label={labels[metric]+'排行'} aria-busy={result.loading}>
              {result.loading?<p role="status">正在读取排行…</p>:result.error?<Alert>{result.error}</Alert>:data?.state==='UNAVAILABLE'?<Empty title="排行暂未开放或数据尚未就绪">排行数据准备完成后会在这里显示，请稍后刷新。</Empty>:data&&<>
                {data.state==='STALE'&&<Alert>数据更新暂有延迟，以下保留上次快照，请留意更新时间。</Alert>}
                {data.items.length===0?<Empty title="当前筛选暂无上榜记录">试试其他指标、周期或模型。指标为零的用户不进入对应榜单。</Empty>:<table><thead><tr><th scope="col">排名</th><th scope="col">御主</th><th scope="col">{labels[metric]}<small>{unit}</small></th>{rp&&<><th scope="col">错误率 / 总请求</th><th scope="col">实际消耗<small>Credit</small></th><th scope="col">模型分布</th></>}</tr></thead><tbody>{data.items.map((row,index)=><tr key={index}>
                  <td className="ranking-position"><span className={'ranking-rank'+(['1','2','3'].includes(row.rank)?' ranking-medal rank-'+row.rank:'')}>{['1','2','3'].includes(row.rank)&&<img src={assetUrl(art.medallion.src)} width={44} height={44} alt=""/>}<span>{row.rank}</span></span></td>
                  <th scope="row" className="ranking-master"><span className="ranking-owner">{row.display_name}{row.badges?.map(badge=><PersonalMedal key={badge.code} badge={badge}/>)}</span></th><td className="ranking-value" data-label={labels[metric]}>{valueText(metric,row.value)}</td>{rp&&<><td data-label="错误率 / 总请求">{errorRate(row)}<small>{count((BigInt(row.calls)+BigInt(row.errors)).toString())} 次请求</small></td><td data-label="实际消耗 · Credit">{credits(row.credits_units)}</td><td className="ranking-models" data-label="模型分布"><Models row={row} metric={metric}/></td></>}
                </tr>)}</tbody></table>}
              </>}
            </div>
            {session.user?<aside className="ranking-mine" aria-label="我的名次"><h2>我的排名</h2>{mine.loading?<p role="status">正在读取我的名次…</p>:mine.error?<Alert>{mine.error}</Alert>:mine.data?.state==='UNAVAILABLE'?<p>个人名次尚未就绪。</p>:mine.data?.my_rank?<p><strong>第 {mine.data.my_rank.rank} 名</strong><span> · {valueText(metric,mine.data.my_rank.value)} {unit}{mine.data.state==='STALE'?' · 数据更新延迟':''}</span></p>:<p>你在当前筛选下尚未上榜。</p>}{rp&&<Link to="/logs?purpose=ROLEPLAY">我的调用记录 ↗</Link>}</aside>:<div className="ranking-guest"><Link to="/login" onClick={()=>saveRouteIntent('/rankings?'+serialized)}>登录查看我的排名</Link><span>榜单向所有访客公开</span></div>}
            <footer className="ranking-board-footer"><nav className="pager" aria-label="排行分页"><span>{data&&data.state!=='UNAVAILABLE'?`共 ${count(data.total)} 位 · 第 ${page} 页`:'公开排行'}</span><div><button disabled={result.loading||page<=1} onClick={()=>change({page:String(page-1)})}>上一页</button><button disabled={result.loading||!data||data.state==='UNAVAILABLE'||BigInt(page*50)>=BigInt(data.total)||page>=10000} onClick={()=>change({page:String(page+1)})}>下一页</button></div></nav><button className="ranking-info" aria-label="统计口径" aria-haspopup="dialog" onClick={()=>setInfo(true)}>统计口径 <span aria-hidden="true">＋</span></button></footer>
          </section>
        </div>
        <p className="ranking-character-label" aria-hidden="true">METATRON JEANNE<small>CHALDEA / NIGHT LOUNGE</small></p>
        {info&&<Modal title="统计口径" onClose={()=>setInfo(false)}><p>榜单由服务端每小时生成并发布。页面自动读取已发布快照，手动刷新也只读取，不会触发重新计算。</p><p>日期按 Asia/Shanghai（UTC+8）划分；周榜从周一开始。更新时间来自数据源核验时间，更新延迟时保留上次快照并明确标识。</p><p>{rp?'按请求发生时的 RP 用途归类，使用最终结算值。同一逻辑请求的渠道重试不重复计数。':metric==='TOTAL_ASSETS'?'当前资产的 Credit 等值快照，包含 API 额度、娱乐筹码及尚属本人的德州与处理中资产。':'仅统计正式结算的游戏结果；德州在牌桌会话完成离桌结算后计入。'}</p><p>指标为零的用户不进入对应榜单，相同指标值并列排名。公开榜单只展示御主显示名及聚合统计；个人名次仅供本人登录后查看。</p></Modal>}
      </main>
      <footer className="ranking-site-footer"><span>CHALDEA / PUBLIC RANKINGS</span><Link to="/entertainment">返回娱乐大厅 ↗</Link></footer>
    </div>;
}
