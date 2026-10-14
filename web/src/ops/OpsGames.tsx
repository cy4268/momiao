import { useEffect, useState, type FormEvent } from 'react';
import { ApiClient, errorText } from '../api';
import { Alert, Loading, Modal, Pager, useResource } from '../ui';
import { OpsFacts, opsTime } from './OpsFacts';
import './ops-business.css';

type EditableConfig={type:string;prize_table_version:string;prizes:{tier:string;multiplier:string;weight:string}[]};
type OpsConfig={config_version_id:string;version_number:string;status:string;config_schema_version:string;ruleset_version:string;algorithm_version:string;config_hash:string;created_at:string;validated_at?:string|null;previewed_at?:string|null;activated_at?:string|null;editable_config?:EditableConfig};
type OpsArtifact={validation_artifact_id:string;config_version_id:string;artifact_type:string;implementation_key:string;ruleset_version:string;algorithm_version:string;config_hash:string;validator_version:string;validation_build:string;result_summary:unknown;artifact_sha256:string;status:string;generated_at:string;verified_at?:string|null};
type Game={game_slug:string;title:string;sort_order:string;version:string;publication_state:string;configured_runtime_state:string;implementation_key:string;active_config?:OpsConfig|null;rounds_24h:string;needs_review_rounds:string};
type RouletteStats={escrow_units:string;funded_units:string;refunded_units:string;paid_units:string;withheld_units:string;conservation_delta:string;completed:string;surrendered:string;draws:string;timeouts:string;needs_review:string;review_rooms:{id:string;state:string;version:string;escrow_units:string;anomaly_code:string;created_at:string}[]};
type Detail={generated_at:string;game:Game;config_versions:OpsConfig[];validation_artifacts:OpsArtifact[];baseline_locked:boolean;roulette?:RouletteStats;page:number;page_size:number;config_total:number};
const fraction=/^(?:0|[1-9][0-9]*)(?:\/[1-9][0-9]*)?$/;
const labels:Record<string,string>={DRAFT:'草稿',PUBLISHED:'已发布',COMING_SOON:'即将开放',RETIRED:'已退役',AVAILABLE:'可用',MAINTENANCE:'维护中',UNAVAILABLE:'不可用',VALIDATED:'已验证',PREVIEWED:'已预览',ACTIVE:'当前生效',SUPERSEDED:'历史版本'};

const record=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
function gcd(a:bigint,b:bigint):bigint{while(b){const next=a%b;a=b;b=next}return a<0n?-a:a}
function exactFraction(numerator:bigint,denominator:bigint):string{
 if(denominator<=0n)return '';
 const divisor=gcd(numerator,denominator);return `${numerator/divisor}/${denominator/divisor}`;
}
function fractionPercent(value:string,digits=6):string{
 if(!fraction.test(value))return '—';
 const [rawNumerator,rawDenominator='1']=value.split('/'),numerator=BigInt(rawNumerator),denominator=BigInt(rawDenominator),scale=10n**BigInt(digits);
 let scaled=(numerator*100n*scale)/denominator;const remainder=(numerator*100n*scale)%denominator;if(remainder*2n>=denominator)scaled++;
 const whole=scaled/scale,tail=String(scaled%scale).padStart(digits,'0').replace(/0+$/,'');return `${whole}${tail?'.'+tail:''}%`;
}
function rateFraction(value:unknown):string|undefined{
 if(typeof value==='string'&&fraction.test(value))return value;
 if(record(value)&&typeof value.fraction==='string'&&fraction.test(value.fraction))return value.fraction;
}
function integerValue(value:unknown):bigint|undefined{
 if(typeof value==='number'&&Number.isSafeInteger(value)&&value>=0)return BigInt(value);
 if(typeof value==='string'&&/^(0|[1-9]\d*)$/.test(value))return BigInt(value);
}
function formatCount(value:bigint|undefined):string{return value===undefined?'—':value.toLocaleString('zh-CN')}

export function findTrustedProbabilityArtifact(config:OpsConfig,artifacts:OpsArtifact[]):OpsArtifact|undefined{
 return artifacts.find(artifact=>artifact.status==='VERIFIED'&&!!artifact.verified_at&&artifact.config_version_id===config.config_version_id&&artifact.config_hash===config.config_hash&&artifact.ruleset_version===config.ruleset_version&&artifact.algorithm_version===config.algorithm_version);
}
function readEnvelope(config:OpsConfig,artifact:OpsArtifact):Record<string,unknown>|undefined{
 if(!record(artifact.result_summary)||artifact.result_summary.config_version!==config.config_version_id||artifact.result_summary.config_hash!==config.config_hash||artifact.result_summary.ruleset_version!==config.ruleset_version||artifact.result_summary.algorithm_version!==config.algorithm_version||artifact.result_summary.artifact_type!==artifact.artifact_type||artifact.result_summary.implementation_key!==artifact.implementation_key||typeof artifact.result_summary.result_json!=='string')return;
 try{const parsed:unknown=JSON.parse(artifact.result_summary.result_json);return record(parsed)?parsed:undefined}catch{return}
}
function ProbabilityMetric({label,value,note}:{label:string;value:string;note?:string}){
 return <div><dt>{label}</dt><dd><strong>{fractionPercent(value)}</strong><small>{note||value}</small></dd></div>;
}
function EvidenceUnavailable({reason}:{reason:string}){
 return <div className="ops-probability-empty"><strong>尚无可信数据</strong><p>{reason}</p></div>;
}
function ExactMathDetails({summary}:{summary:unknown}){
 const value=summary as {rtp:string;win:string;loss:string;break_even:string;top:string};return <dl className="ops-probability-grid"><ProbabilityMetric label="理论返还率（RTP）" value={value.rtp}/><ProbabilityMetric label="单次净赢" value={value.win}/><ProbabilityMetric label="回本" value={value.break_even}/><ProbabilityMetric label="净输" value={value.loss}/><ProbabilityMetric label="最高档" value={value.top}/></dl>;
}
function SlotProbabilityDetails({config,artifact}:{config:OpsConfig;artifact:OpsArtifact}){
 const result=readEnvelope(config,artifact),rates=record(result?.rates)?result.rates:undefined;
 const rtp=rateFraction(result?.rtp),win=rateFraction(rates?.WIN),draw=rateFraction(rates?.BREAK_EVEN),loss=rateFraction(rates?.LOSS);
 const combinations=integerValue(result?.total_combinations)??integerValue(result?.cases),cases=integerValue(result?.cases);
 if(!result||result.complete!==true||(result.method!=='EXACT_ENUMERATION'&&result.method!=='EXACT_ENUMERATION_PRODUCTION_LINE_EVALUATOR')||!rtp||!win||!draw||!loss||combinations===undefined||(cases!==undefined&&cases!==combinations))return <EvidenceUnavailable reason="匹配工件未包含可核对的完整枚举结果，未展示推测值。"/>;
 const top=record(result.top)?result.top:undefined,topCount=integerValue(top?.count),topRate=topCount===undefined?'':exactFraction(topCount,combinations),topMultiplier=record(top?.total_wager_multiplier)?rateFraction(top.total_wager_multiplier):undefined;
 const wild=record(result.wild_five)?result.wild_five:undefined,wildProbability=record(wild?.probability)?rateFraction(wild.probability):undefined,wildCount=integerValue(wild?.count),wildMultiplier=integerValue(wild?.line_multiplier);
 const optional=[['有派彩',rateFraction(rates?.NONZERO_PAYOUT)],['未中奖',rateFraction(rates?.NO_WIN)],['部分返还',rateFraction(rates?.PARTIAL_RETURN)]] as const;
 return <><p className="ops-evidence-label"><strong>完整枚举</strong> · {formatCount(combinations)} 种停点组合</p><dl className="ops-probability-grid"><ProbabilityMetric label="理论返还率（RTP）" value={rtp}/><ProbabilityMetric label="单转净赢" value={win}/><ProbabilityMetric label="回本" value={draw}/><ProbabilityMetric label="净输" value={loss}/>{optional.map(([label,value])=>value?<ProbabilityMetric key={label} label={label} value={value}/>:null)}{topRate&&<ProbabilityMetric label="整局最高奖" value={topRate} note={`${topRate}${topMultiplier?` · 总下注 ×${topMultiplier}`:''}`}/>} {wildProbability&&<ProbabilityMetric label="Wild 五连盘面" value={wildProbability} note={`${wildProbability}${wildCount!==undefined?` · ${formatCount(wildCount)} 种`:''}${wildMultiplier!==undefined?` · 每线 ×${wildMultiplier}`:''}`}/>}</dl></>;
}
function BlackjackProbabilityDetails({config,artifact}:{config:OpsConfig;artifact:OpsArtifact}){
 const result=readEnvelope(config,artifact),denominator=record(result?.initial_wager_denominator)?result.initial_wager_denominator:undefined,rtp=record(denominator?.rtp)?denominator.rtp:undefined;
 const point=rateFraction(rtp?.point),ci=Array.isArray(rtp?.normal_approximation_ci95)&&rtp.normal_approximation_ci95.length===2&&rtp.normal_approximation_ci95.every(value=>typeof value==='number'&&Number.isFinite(value))?rtp.normal_approximation_ci95 as number[]:undefined;
 const strategy=record(result?.reference_strategy)?result.reference_strategy:undefined,rounds=integerValue(result?.rounds),base=record(result?.base_sample)?result.base_sample:undefined,baseRTP=rateFraction(base?.rtp),fairReturn=rateFraction(result?.fair_return);
 if(!result||result.complete!==true||typeof result.method!=='string'||!result.method.startsWith('REFERENCE_STRATEGY')||!point||!strategy||typeof strategy.version!=='string'||rounds===undefined)return <EvidenceUnavailable reason="匹配工件未包含完整的参考策略样本说明，未展示推测值。"/>;
 return <><p className="ops-evidence-label"><strong>参考策略样本</strong> · {formatCount(rounds)} 局；不是任意策略或短期结果保证</p><dl className="ops-probability-grid"><ProbabilityMetric label="参考策略调整后 RTP" value={point}/>{baseRTP&&<ProbabilityMetric label="基础样本 RTP" value={baseRTP}/>} {fairReturn&&<ProbabilityMetric label="规则返还（初始下注口径）" value={fairReturn}/>}<div><dt>95% 置信区间</dt><dd><strong>{ci?ci.map(value=>(value*100).toFixed(4)+'%').join(' – '):'工件未包含'}</strong><small>蒙特卡洛样本误差范围</small></dd></div><div><dt>净赢率</dt><dd><strong>未包含</strong><small>工件没有可信净赢率，不由 RTP 推导</small></dd></div></dl><p className="hint">参考策略：{strategy.version}{typeof strategy.scope==='string'?` · ${strategy.scope}`:''}</p></>;
}
type TenfoldRates={win:string;breakEven:string;loss:string;top:string};
function tenfoldRates(config:EditableConfig,total:bigint):TenfoldRates|undefined{
 const prizes=config.prizes.map(prize=>({multiplier:BigInt(prize.multiplier),weight:BigInt(prize.weight)}));
 if(prizes.some(prize=>prize.multiplier>1000000n))return;
 let distribution=new Map<bigint,bigint>([[0n,1n]]);
 for(let draw=0;draw<10;draw++){
  const next=new Map<bigint,bigint>();for(const [sum,count] of distribution)for(const prize of prizes){const key=sum+prize.multiplier;next.set(key,(next.get(key)||0n)+count*prize.weight);if(next.size>20000)return}
  distribution=next;
 }
 let win=0n,draw=0n,loss=0n;for(const [sum,count] of distribution){if(sum>10n)win+=count;else if(sum===10n)draw+=count;else loss+=count}
 const denominator=total**10n,max=prizes.reduce((value,prize)=>prize.multiplier>value?prize.multiplier:value,0n),topWeight=prizes.filter(prize=>prize.multiplier===max).reduce((sum,prize)=>sum+prize.weight,0n);
 return {win:exactFraction(win,denominator),breakEven:exactFraction(draw,denominator),loss:exactFraction(loss,denominator),top:exactFraction(denominator-(total-topWeight)**10n,denominator)};
}
function PrizePoolDetails({config}:{config:EditableConfig}){
 const prizes=config.prizes.map(prize=>({...prize,weightValue:BigInt(prize.weight)})),total=prizes.reduce((sum,prize)=>sum+prize.weightValue,0n);
 if(total===0n)return <EvidenceUnavailable reason="所选配置的奖池总权重为 0，不能计算概率。"/>;
 const tenfold=config.type==='SUMMON_V1'?tenfoldRates(config,total):undefined;
 return <section className="ops-prize-probability"><h4>配置奖池明细</h4><p className="hint">按所选版本的整数权重精确计算；草稿权重不等同于 VERIFIED 工件。</p><div className="table-wrap" role="region" aria-label="完整奖池概率" tabIndex={0}><table><thead><tr><th>等级</th><th>总派彩倍数</th><th>权重</th><th>单抽 / 单张概率</th></tr></thead><tbody>{prizes.map((prize,index)=>{const value=exactFraction(prize.weightValue,total);return <tr key={`${prize.tier}-${index}`}><td>{prize.tier}</td><td>×{prize.multiplier}</td><td>{prize.weight}</td><td>{fractionPercent(value)}<small>{value}</small></td></tr>})}</tbody></table></div>{config.type==='SUMMON_V1'&&(tenfold?<><h4>十连整轮分布</h4><p className="hint">由该奖池进行 10 次独立抽取的整数权重卷积；总派彩倍数之和与总消耗 10 倍比较。</p><dl className="ops-probability-grid"><ProbabilityMetric label="十连净赢" value={tenfold.win}/><ProbabilityMetric label="十连回本" value={tenfold.breakEven}/><ProbabilityMetric label="十连净输" value={tenfold.loss}/><ProbabilityMetric label="十连至少一次最高档" value={tenfold.top}/></dl></>:<p className="hint">该草稿的倍率状态过多，浏览器未展开十连卷积；单抽明细仍按原始整数权重显示。</p>)}</section>;
}
export function OpsProbabilityDetails({gameSlug,config,artifacts}:{gameSlug:string;config:OpsConfig;artifacts:OpsArtifact[]}){
 const artifact=findTrustedProbabilityArtifact(config,artifacts);
 let content;
 if(!artifact)content=<EvidenceUnavailable reason="没有同时匹配所选配置版本 ID、配置哈希、规则集与算法版本的 VERIFIED 工件。"/>;
 else if(artifact.artifact_type==='MULTIPLAYER_RULESET')content=<p>全体玩家零抽水，个人胜率取决于玩法。固定规则版本不使用单人概率表。</p>;
 else if(artifact.artifact_type==='SLOT_EXHAUSTIVE')content=<SlotProbabilityDetails config={config} artifact={artifact}/>;
 else if(artifact.artifact_type==='BLACKJACK_RTP')content=<BlackjackProbabilityDetails config={config} artifact={artifact}/>;
 else content=<ExactMathDetails summary={artifact.result_summary}/>;
 return <section className="ops-probability" aria-labelledby="ops-probability-title"><div className="section-heading"><div><p className="eyebrow">READ ONLY / CONFIG-BOUND</p><h3 id="ops-probability-title">只读概率详情</h3></div><span>{labels[config.status]}</span></div><p className="hint">当前展示所选配置；这里没有胜率调节开关，也不会把其他版本的工件混入结果。</p><dl className="ops-probability-binding"><div><dt>配置版本</dt><dd><code>{config.config_version_id}</code></dd></div><div><dt>配置哈希</dt><dd><code>{config.config_hash}</code></dd></div><div><dt>规则集</dt><dd>{config.ruleset_version}</dd></div><div><dt>算法版本</dt><dd>{config.algorithm_version}</dd></div></dl>{content}{config.editable_config&&<PrizePoolDetails config={config.editable_config}/>} {artifact&&<details><summary>可信工件标识</summary><OpsFacts value={{validation_artifact_id:artifact.validation_artifact_id,artifact_type:artifact.artifact_type,validator_version:artifact.validator_version,validation_build:artifact.validation_build,artifact_sha256:artifact.artifact_sha256,verified_at:artifact.verified_at,game_slug:gameSlug}}/></details>}</section>;
}
export function OpsGames({client}:{client:ApiClient}){
 const [slug,setSlug]=useState(''),[selectedConfig,setSelectedConfig]=useState(''),[page,setPage]=useState(1),[filters,setFilters]=useState('');
 const [busy,setBusy]=useState(false),[error,setError]=useState(''),[saved,setSaved]=useState('');
 const [search,setSearch]=useState(''),[publication,setPublication]=useState('');
 const overview=useResource(()=>client.request<{generated_at:string;games:Game[]}>('/api/v1/ops/games'),[client]);
 const detail=useResource(()=>slug?client.request<Detail>(`/api/v1/ops/games/${slug}?page=${page}${filters}`):Promise.resolve(null),[client,slug,page,filters]);
 const game=detail.data?.game,config=detail.data?.config_versions.find(item=>item.config_version_id===selectedConfig);
 useEffect(()=>{const data=detail.data;if(data)setSelectedConfig(current=>data.config_versions.some(v=>v.config_version_id===current)?current:data.config_versions.find(v=>v.status==='ACTIVE')?.config_version_id||data.config_versions[0]?.config_version_id||'')},[detail.data]);
 function open(gameSlug:string){setSlug(gameSlug);setPage(1);setFilters('');setSelectedConfig('');setError('');setSaved('')}
 async function saveSettings(event:FormEvent<HTMLFormElement>){
  event.preventDefault();if(!game)return;const form=new FormData(event.currentTarget);
  const input={title:String(form.get('title')),sort_order:String(form.get('sort_order')),publication_state:String(form.get('publication_state')),configured_runtime_state:String(form.get('configured_runtime_state'))};
  if((input.publication_state!==game.publication_state||input.configured_runtime_state!==game.configured_runtime_state)&&!window.confirm('保存发布与运行状态？已接受的游戏仍按原配置结算。'))return;
  setBusy(true);setError('');setSaved('');
  try{await client.request(`/api/v1/ops/games/${slug}`,'PUT',input);setSaved('游戏设置已保存。');overview.reload();detail.reload()}
  catch(e){setError(errorText(e))}finally{setBusy(false)}
 }
 async function configAction(action:string,input?:EditableConfig){
  if(!config)return;
  if(action==='activate'&&!window.confirm('立即启用此配置？已接受的游戏保留原配置。'))return;
  setBusy(true);setError('');setSaved('');
  try{
   const value=await client.request<OpsConfig>(`/api/v1/ops/games/${slug}/configs/${config.config_version_id}${action==='save'?'':'/'+action}`,action==='save'?'PUT':'POST',input||{});
   setSaved(action==='activate'?'配置已生效。':action==='validate'?'配置已验证，可查看概率工件后启用。':action==='clone'?'已复制为草稿。':'草稿已保存。');
   setSelectedConfig(value.config_version_id);if(action==='clone'){setPage(1);setFilters('')}overview.reload();detail.reload();
  }catch(e){setError(errorText(e))}finally{setBusy(false)}
 }
 function saveDraft(event:FormEvent<HTMLFormElement>){event.preventDefault();if(!config?.editable_config)return;const form=new FormData(event.currentTarget),editable=config.editable_config;void configAction('save',{...editable,prize_table_version:String(form.get('prize_table_version')),prizes:editable.prizes.map((p,index)=>({...p,weight:String(form.get('weight-'+index))}))})}
 function filter(event:FormEvent<HTMLFormElement>){event.preventDefault();const form=new FormData(event.currentTarget),q=new URLSearchParams();q.set('status',String(form.get('status')||''));for(const k of ['from','to']){const v=String(form.get(k)||'');if(v)q.set(k,new Date(v).toISOString())}setPage(1);setFilters('&'+q)}
 async function systemVoid(id:string){
  if(!window.confirm('终止此轮盘并退还未结算的托管筹码？'))return;
  setBusy(true);setError('');setSaved('');
  try{await client.request(`/api/v1/ops/games/${slug}/rounds/${id}/void`,'POST',{});setSaved('轮盘已取消，未结算托管筹码已退还。');detail.reload();overview.reload()}
  catch(e){setError(errorText(e))}finally{setBusy(false)}
 }
 return <><header className="page-heading"><div><p className="eyebrow">OPERATIONS / GAMES</p><h1>游戏运营</h1><p>直接保存展示、发布与运行状态；配置继续使用原有数学校验和不可变历史。</p></div><button disabled={busy||overview.loading} onClick={()=>{overview.reload();detail.reload()}}>刷新状态</button></header>
 <div className="filters panel"><label>查找游戏<input value={search} onChange={e=>setSearch(e.target.value)}/></label><label>发布状态筛选<select value={publication} onChange={e=>setPublication(e.target.value)}><option value="">全部</option>{['DRAFT','PUBLISHED','COMING_SOON','RETIRED'].map(v=><option key={v} value={v}>{labels[v]}</option>)}</select></label></div>
 {!slug&&error&&<Alert>{error}</Alert>}
 {overview.loading?<Loading/>:overview.error?<Alert>{overview.error}</Alert>:overview.data&&<section className="panel"><div className="table-wrap" role="region" aria-label="游戏运营总览" tabIndex={0}><table><thead><tr><th>游戏</th><th>排序</th><th>发布 / 运行</th><th>生效配置</th><th>近 24 小时</th><th>需核查</th></tr></thead><tbody>{overview.data.games.filter(v=>(!publication||v.publication_state===publication)&&(`${v.title} ${v.game_slug}`).includes(search)).map(item=><tr key={item.game_slug}><td><button disabled={busy} onClick={()=>open(item.game_slug)}>{item.title}</button><small>{item.game_slug}</small></td><td>{item.sort_order}</td><td>{labels[item.publication_state]} / {labels[item.configured_runtime_state]}</td><td>{item.active_config?.version_number||'未配置'}</td><td>{item.rounds_24h}</td><td>{item.needs_review_rounds}</td></tr>)}</tbody></table></div></section>}
 {slug&&<Modal title="游戏详情" busy={busy} onClose={()=>setSlug('')}>
 {error&&<Alert>{error}</Alert>}{saved&&<p role="status">{saved}</p>}
 <button disabled={busy||detail.loading} onClick={detail.reload}>刷新详情</button>
 {detail.loading?<Loading/>:detail.error?<Alert>{detail.error}</Alert>:game&&detail.data&&<>
 <h3>{game.title}</h3><p className="hint">已接受的局与结算始终保留原配置绑定。</p>
 <form key={game.game_slug+game.version} className="filters" onSubmit={saveSettings}>
 <label>标题<input name="title" defaultValue={game.title} required maxLength={100}/></label><label>排序<input name="sort_order" type="number" min="0" max="999999999" step="1" defaultValue={game.sort_order} required/></label>
 <label>发布状态<select name="publication_state" defaultValue={game.publication_state}>{['DRAFT','PUBLISHED','COMING_SOON','RETIRED'].map(v=><option key={v} value={v}>{labels[v]}</option>)}</select></label>
 <label>运行状态<select name="configured_runtime_state" defaultValue={game.configured_runtime_state}>{['AVAILABLE','MAINTENANCE','UNAVAILABLE'].map(v=><option key={v} value={v}>{labels[v]}</option>)}</select></label><button disabled={busy}>保存游戏设置</button>
 </form>
 <section><h3>配置历史</h3><form className="filters" onSubmit={filter}><label>配置状态<select name="status"><option value="">全部</option>{['DRAFT','VALIDATED','PREVIEWED','ACTIVE','SUPERSEDED'].map(v=><option key={v} value={v}>{labels[v]}</option>)}</select></label><label>配置创建起始时间<input type="datetime-local" name="from"/></label><label>配置创建截止时间<input type="datetime-local" name="to"/></label><button disabled={busy}>筛选配置</button></form>
 <label>配置版本<select value={selectedConfig} onChange={e=>setSelectedConfig(e.target.value)} disabled={busy}>{detail.data.config_versions.map(item=><option key={item.config_version_id} value={item.config_version_id}>#{item.version_number} · {labels[item.status]}</option>)}</select></label>
 <Pager page={detail.data.page} size={detail.data.page_size} total={detail.data.config_total} onChange={setPage} disabled={busy}/>
 {config?<><OpsProbabilityDetails gameSlug={game.game_slug} config={config} artifacts={detail.data.validation_artifacts}/>
 <div className="inline-actions">{!['poker','devil-roulette','pressure-roulette'].includes(game.game_slug)&&<><button disabled={busy} onClick={()=>void configAction('clone')}>复制为草稿</button><button disabled={busy||config.status!=='DRAFT'} onClick={()=>void configAction('validate')}>验证配置</button></>}{game.game_slug!=='poker'&&<button disabled={busy||!['VALIDATED','PREVIEWED'].includes(config.status)} onClick={()=>void configAction('activate')}>启用配置</button>}</div>
 {config.editable_config&&config.status==='DRAFT'&&<form key={config.config_version_id+config.config_hash} className="filters" onSubmit={saveDraft}><label>奖池版本<input name="prize_table_version" required defaultValue={config.editable_config.prize_table_version}/></label>{config.editable_config.prizes.map((prize,index)=><label key={prize.tier}>{prize.tier} 权重（×{prize.multiplier}）<input name={'weight-'+index} type="number" min="0" step="1" required defaultValue={prize.weight}/></label>)}<button disabled={busy}>保存草稿</button></form>}
 </>:<p>当前筛选下没有配置。</p>}</section>
 {detail.data.roulette&&<section><h3>轮盘托管核算</h3><OpsFacts value={{funded_units:detail.data.roulette.funded_units,refunded_units:detail.data.roulette.refunded_units,paid_units:detail.data.roulette.paid_units,withheld_units:detail.data.roulette.withheld_units,escrow_units:detail.data.roulette.escrow_units,conservation_delta:detail.data.roulette.conservation_delta,completed:detail.data.roulette.completed,surrendered:detail.data.roulette.surrendered,draws:detail.data.roulette.draws,timeouts:detail.data.roulette.timeouts,needs_review:detail.data.roulette.needs_review}}/><h4>最早的 100 个需核查房间</h4>{detail.data.roulette.review_rooms.map(room=><div className="panel" key={room.id}><OpsFacts value={room}/><button disabled={busy} onClick={()=>void systemVoid(room.id)}>系统作废并退款</button></div>)}</section>}
 <section><h3>验证工件</h3>{detail.data.validation_artifacts.length===0?<p>没有匹配当前页的工件。</p>:detail.data.validation_artifacts.map(artifact=><details key={artifact.validation_artifact_id}><summary>{artifact.artifact_type} · {artifact.status} · {opsTime(artifact.generated_at)}</summary><OpsFacts value={artifact}/></details>)}</section>
 </>}
 </Modal>}
 </>;
}
