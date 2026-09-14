import { useEffect, useState, useSyncExternalStore } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { ApiClient } from '../api';
import { Alert, Brand, Crest, Empty, useResource } from '../ui';
import { saveRouteIntent } from '../post-auth-intent';
import './rankings.css';

const labels:Record<string,string>={TOTAL_ASSETS:'总资产',GAME_PROFIT:'游戏净盈利',BIGGEST_WIN:'单局最高净收益',TOTAL_WAGERED:'累计下注',POKER_PROFIT:'Poker 已实现盈利',RP_CALLS:'成功调用',RP_ERRORS:'失败调用',RP_CREDITS:'实际消耗'};
type Model={model_id:string;display_name:string;calls:string;errors:string;credits_units:string};
type Row={rank:string;display_name:string;avatar_id:string;value:string;calls:string;errors:string;credits_units:string;models:Model[]};
type Page={state:'READY'|'STALE'|'UNAVAILABLE';metric:string;period:string;period_start:string;period_end:string|null;last_updated:string|null;items:Row[];total:string;page:number;page_size:number;historical_periods:string[];my_rank?:Row};
const decimal=(value:unknown):value is string=>typeof value==='string'&&/^-?(0|[1-9][0-9]{0,37})$/.test(value);
const unsigned=(value:unknown):value is string=>decimal(value)&&!value.startsWith('-');
function checkRow(row:Row){
    if(!row||!unsigned(row.rank)||BigInt(row.rank)<1n||typeof row.display_name!=='string'||row.avatar_id!=='system-default'||!decimal(row.value)||!unsigned(row.calls)||!unsigned(row.errors)||!unsigned(row.credits_units)||!Array.isArray(row.models))throw new Error('排行响应格式异常。');
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
export function Rankings({client}:{client:ApiClient}){
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
    useEffect(()=>{setModelDraft(model)},[model]);
    useEffect(()=>{const timer=setInterval(()=>setRefresh(value=>value+1),60000);return()=>clearInterval(timer)},[]);
    const query=new URLSearchParams({metric,period,page:String(page)});if(model)query.set('model',model);if(date)query.set('date',date);
    const serialized=query.toString(),generation=client.getSessionGeneration();
    const result=useResource(()=>read(client,new URLSearchParams(serialized),false),[client,serialized,generation,refresh]);
    const mine=useResource(()=>session.user?read(client,new URLSearchParams(serialized),true):Promise.resolve(null),[client,serialized,generation,session.user?.id,refresh]);
    function change(values:Record<string,string>){const next=new URLSearchParams(serialized);next.delete('page');for(const [key,value]of Object.entries(values)){if(value)next.set(key,value);else next.delete(key)}setParams(next);}
    const data=result.data;
    return <div className="workspace ranking-workspace"><header className="workspace-bar"><Link to="/" className="brand"><Brand/></Link><nav aria-label="排行导航"><Link to="/entertainment">娱乐中心</Link><Link to={session.user?'/me':'/login'} onClick={()=>{if(!session.user)saveRouteIntent('/rankings')}}>{session.user?'个人中心':'登录'}</Link></nav></header><main className="ranking-main"><header className="page-heading"><div><p className="eyebrow">CHALDEA / RANKINGS</p><h1>记录每一份积累</h1><p>公开聚合排行 · Asia/Shanghai · 每周一开始新一周</p></div><button onClick={()=>{result.reload();mine.reload()}} disabled={result.loading}>刷新排行</button></header>
    <nav className="ranking-domains" aria-label="排行类别"><button aria-pressed={!rp} onClick={()=>change({metric:'TOTAL_ASSETS',period:'CURRENT',date:'',model:''})}>Assets & Games</button><button aria-pressed={rp} onClick={()=>change({metric:'RP_CALLS',period:'DAY',date:'',model:''})}>RP Usage</button></nav>
    <section className="panel"><div className="ranking-filters"><label>指标<select value={metric} onChange={event=>change({metric:event.target.value,period:event.target.value==='TOTAL_ASSETS'?'CURRENT':period==='CURRENT'?'DAY':period,date:''})}>{Object.entries(labels).filter(([key])=>key.startsWith('RP_')===rp).map(([key,label])=><option key={key} value={key}>{label}</option>)}</select></label>{metric!=='TOTAL_ASSETS'&&<><label>周期<select value={period} onChange={event=>change({period:event.target.value,date:''})}><option value="DAY">Today · 日榜</option><option value="WEEK">This Week · 周榜</option><option value="ALL_TIME">All Time · 全部</option></select></label>{period!=='ALL_TIME'&&<label>历史周期<select value={date} onChange={event=>change({date:event.target.value})}><option value="">当前周期</option>{data?.historical_periods.map(value=><option key={value} value={value}>{value}</option>)}</select></label>}</>}{rp&&<form onSubmit={event=>{event.preventDefault();change({model:modelDraft.trim()})}}><label>筛选模型 ID<input value={modelDraft} maxLength={256} onChange={event=>setModelDraft(event.target.value)} placeholder="全部模型"/></label><button type="submit">应用</button>{model&&<button type="button" onClick={()=>{setModelDraft('');change({model:''})}}>清除</button>}</form>}</div>
    <p className="hint">{rp?'按请求发生时的 RP 用途归类，使用最终结算值。同一逻辑请求的渠道重试不重复计数。':metric==='TOTAL_ASSETS'?'当前资产的 Credit 等值快照，包含 API 额度、娱乐筹码及尚属本人的 Poker 与处理中资产。':'仅统计正式结算的游戏结果；Poker 在 Session 完成 Cash Out 后计入。'}</p>
    {data?.last_updated&&<p className="ranking-updated">Last Updated · {new Date(data.last_updated).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai'})}</p>}
    {result.loading?<p role="status">正在读取排行…</p>:result.error?<Alert>{result.error}</Alert>:data?.state==='UNAVAILABLE'?<Empty title="排行暂未开放或数据尚未就绪">排行数据准备完成后会在这里显示，请稍后刷新。</Empty>:data&&<>{data.state==='STALE'&&<Alert>数据更新暂有延迟，以下保留上次快照，请留意更新时间。</Alert>}{data.items.length===0?<Empty title="当前筛选暂无上榜记录">试试其他指标、周期或模型。指标为零的用户不进入对应榜单。</Empty>:<div className="table-wrap" tabIndex={0} role="region" aria-label={labels[metric]+'排行'}><table><thead><tr><th>名次</th><th>Master</th><th>{labels[metric]}{metric==='RP_CREDITS'?' · Credit':metric==='TOTAL_ASSETS'?' · Credit 等值':!rp?' · 筹码':''}</th>{rp&&<><th>错误率 / 总请求</th><th>实际消耗 · Credit</th><th>模型分布</th></>}</tr></thead><tbody>{data.items.map((row,index)=><tr key={index}><td className="ranking-position">{row.rank}</td><td><span className="ranking-master"><span className="ranking-avatar" role="img" aria-label="系统默认头像"><Crest/></span>{row.display_name}</span></td><td>{valueText(metric,row.value)}</td>{rp&&<><td>{errorRate(row)}<small>{count((BigInt(row.calls)+BigInt(row.errors)).toString())} 次请求</small></td><td>{credits(row.credits_units)}</td><td><Models row={row} metric={metric}/></td></>}</tr>)}</tbody></table></div>}<nav className="pager" aria-label="排行分页"><span>共 {count(data.total)} 位 · 第 {page} 页</span><div><button disabled={page<=1} onClick={()=>change({page:String(page-1)})}>上一页</button><button disabled={BigInt(page*50)>=BigInt(data.total)||page>=10000} onClick={()=>change({page:String(page+1)})}>下一页</button></div></nav></>}
    </section>{session.user&&<aside className="ranking-mine panel" aria-label="我的名次"><h2>My Rank</h2>{mine.loading?<p role="status">正在读取我的名次…</p>:mine.error?<Alert>{mine.error}</Alert>:mine.data?.state==='UNAVAILABLE'?<p>个人名次尚未就绪。</p>:mine.data?.my_rank?<p><strong>第 {mine.data.my_rank.rank} 名</strong> · {valueText(metric,mine.data.my_rank.value)}{mine.data.state==='STALE'?' · 数据更新延迟':''}</p>:<p>你在当前筛选下尚未上榜。</p>}{rp&&<Link className="text-link" to="/logs?purpose=ROLEPLAY">查看我的 RP 调用记录 →</Link>}</aside>}</main></div>;
}
