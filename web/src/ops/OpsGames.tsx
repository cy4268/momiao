import { useEffect, useState, type FormEvent } from 'react';
import { z } from 'zod';
import { Alert, Loading, useResource } from '../ui';
import { OpsFacts, opsTime } from './OpsOperationDetail';
import { OpsMutationPanel, useOpsMutation } from './OpsMutation';
import { opsError } from './ops-api';
import { useOps } from './OpsShell';

const decimal=z.string().regex(/^(0|[1-9][0-9]{0,18})$/),time=z.string().refine(value=>Number.isFinite(Date.parse(value))),uuid=z.string().uuid();
const hash=z.string().regex(/^[0-9a-f]{64}$/),fraction=z.string().regex(/^(?:0|[1-9][0-9]*)(?:\/[1-9][0-9]*)?$/);
const editableSchema=z.object({type:z.enum(['SCRATCH_V1','SUMMON_V1']),prize_table_version:z.string(),prizes:z.array(z.object({tier:z.string(),multiplier:decimal,weight:decimal})).min(1).max(32)});
const configSchema=z.object({config_version_id:uuid,version_number:decimal,status:z.enum(['DRAFT','VALIDATED','PREVIEWED','ACTIVE','SUPERSEDED']),config_schema_version:z.string(),ruleset_version:z.string(),algorithm_version:z.string(),config_hash:hash,created_at:time,validated_at:time.nullish(),previewed_at:time.nullish(),activated_at:time.nullish(),editable_config:editableSchema.optional()});
const artifactSchema=z.object({validation_artifact_id:uuid,config_version_id:uuid,artifact_type:z.string(),implementation_key:z.string(),ruleset_version:z.string(),algorithm_version:z.string(),config_hash:hash,validator_version:z.string(),validation_build:z.string(),result_summary:z.json(),artifact_sha256:hash,status:z.string(),generated_at:time,verified_at:time.nullish()});
const gameSchema=z.object({game_slug:z.enum(['dice','scratch','summon','slot','blackjack','poker']),title:z.string(),sort_order:decimal,version:decimal,publication_state:z.enum(['DRAFT','PUBLISHED','COMING_SOON','RETIRED']),configured_runtime_state:z.enum(['AVAILABLE','MAINTENANCE','UNAVAILABLE']),implementation_key:z.string(),active_config:configSchema.nullish(),rounds_24h:decimal,needs_review_rounds:decimal});
const overviewSchema=z.object({generated_at:time,games:z.array(gameSchema).max(20)});
const detailSchema=z.object({generated_at:time,game:gameSchema,config_versions:z.array(configSchema).max(100),validation_artifacts:z.array(artifactSchema).max(100),baseline_locked:z.literal(true)});
const labels:Record<string,string>={DRAFT:'草稿',PUBLISHED:'已发布',COMING_SOON:'即将开放',RETIRED:'已退役',AVAILABLE:'可用',MAINTENANCE:'维护中',UNAVAILABLE:'不可用',VALIDATED:'已验证',PREVIEWED:'已预览',ACTIVE:'当前生效',SUPERSEDED:'历史版本'};
type OpsConfig=z.infer<typeof configSchema>;
type OpsArtifact=z.infer<typeof artifactSchema>;
type EditableConfig=z.infer<typeof editableSchema>;

const record=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
function gcd(a:bigint,b:bigint):bigint{while(b){const next=a%b;a=b;b=next}return a<0n?-a:a}
function exactFraction(numerator:bigint,denominator:bigint):string{
 if(denominator<=0n)return '';
 const divisor=gcd(numerator,denominator);return `${numerator/divisor}/${denominator/divisor}`;
}
function fractionPercent(value:string,digits=6):string{
 const parsed=fraction.safeParse(value);if(!parsed.success)return '—';
 const [rawNumerator,rawDenominator='1']=value.split('/'),numerator=BigInt(rawNumerator),denominator=BigInt(rawDenominator),scale=10n**BigInt(digits);
 let scaled=(numerator*100n*scale)/denominator;const remainder=(numerator*100n*scale)%denominator;if(remainder*2n>=denominator)scaled++;
 const whole=scaled/scale,tail=String(scaled%scale).padStart(digits,'0').replace(/0+$/,'');return `${whole}${tail?'.'+tail:''}%`;
}
function rateFraction(value:unknown):string|undefined{
 if(typeof value==='string'&&fraction.safeParse(value).success)return value;
 if(record(value)&&typeof value.fraction==='string'&&fraction.safeParse(value.fraction).success)return value.fraction;
}
function integerValue(value:unknown):bigint|undefined{
 if(typeof value==='number'&&Number.isSafeInteger(value)&&value>=0)return BigInt(value);
 if(typeof value==='string'&&decimal.safeParse(value).success)return BigInt(value);
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
 const parsed=z.object({rtp:fraction,win:fraction,loss:fraction,break_even:fraction,top:fraction}).safeParse(summary);
 if(!parsed.success)return <EvidenceUnavailable reason="匹配工件的数学摘要格式不完整，未展示推测值。"/>;
 const value=parsed.data;return <dl className="ops-probability-grid"><ProbabilityMetric label="理论返还率（RTP）" value={value.rtp}/><ProbabilityMetric label="单次净赢" value={value.win}/><ProbabilityMetric label="回本" value={value.break_even}/><ProbabilityMetric label="净输" value={value.loss}/><ProbabilityMetric label="最高档" value={value.top}/></dl>;
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
 else if(artifact.artifact_type==='SLOT_EXHAUSTIVE')content=<SlotProbabilityDetails config={config} artifact={artifact}/>;
 else if(artifact.artifact_type==='BLACKJACK_RTP')content=<BlackjackProbabilityDetails config={config} artifact={artifact}/>;
 else content=<ExactMathDetails summary={artifact.result_summary}/>;
 return <section className="ops-probability" aria-labelledby="ops-probability-title"><div className="section-heading"><div><p className="eyebrow">READ ONLY / CONFIG-BOUND</p><h3 id="ops-probability-title">只读概率详情</h3></div><span>{labels[config.status]}</span></div><p className="hint">当前展示所选配置；这里没有胜率调节开关，也不会把其他版本的工件混入结果。</p><dl className="ops-probability-binding"><div><dt>配置版本</dt><dd><code>{config.config_version_id}</code></dd></div><div><dt>配置哈希</dt><dd><code>{config.config_hash}</code></dd></div><div><dt>规则集</dt><dd>{config.ruleset_version}</dd></div><div><dt>算法版本</dt><dd>{config.algorithm_version}</dd></div></dl>{content}{config.editable_config&&<PrizePoolDetails config={config.editable_config}/>} {artifact&&<details><summary>可信工件标识</summary><OpsFacts value={{validation_artifact_id:artifact.validation_artifact_id,artifact_type:artifact.artifact_type,validator_version:artifact.validator_version,validation_build:artifact.validation_build,artifact_sha256:artifact.artifact_sha256,verified_at:artifact.verified_at,game_slug:gameSlug}}/></details>}</section>;
}
export function OpsGames(){
 const {client,bootstrap}=useOps(),mutation=useOpsMutation();const [slug,setSlug]=useState(''),[selectedConfig,setSelectedConfig]=useState(''),[error,setError]=useState('');
 const epoch=bootstrap.principal.authz_epoch,blocked=!!mutation.pending||mutation.busy;
 const overview=useResource(async()=>{try{return overviewSchema.parse(await client.request(`/api/v1/ops/games?authz_epoch=${epoch}`))}catch(error){throw new Error(opsError(error))}},[client,epoch]);
 const detail=useResource(async()=>{if(!slug)return null;try{const value=detailSchema.parse(await client.request(`/api/v1/ops/games/${slug}?authz_epoch=${epoch}`));if(value.game.game_slug!==slug)throw new Error('游戏编号不一致。');return value}catch(error){throw new Error(opsError(error))}},[client,epoch,slug]);
 const game=detail.data?.game,config=detail.data?.config_versions.find(item=>item.config_version_id===selectedConfig);
 const can=(type:string)=>bootstrap.operations.some(item=>item.operation_type===type&&item.available&&bootstrap.principal.permissions.includes(item.required_permission));
 useEffect(()=>{if(detail.data)setSelectedConfig(current=>detail.data!.config_versions.some(item=>item.config_version_id===current)?current:detail.data!.game.active_config?.config_version_id||'')},[detail.data]);
 useEffect(()=>{if(mutation.receipt?.operation.state==='SUCCEEDED'){overview.reload();detail.reload()}},[mutation.receipt?.operation.operation_id,mutation.receipt?.operation.state]);
 function change(event:FormEvent<HTMLFormElement>){event.preventDefault();if(!game)return;const form=new FormData(event.currentTarget),type=String(form.get('operation_type')),reason=String(form.get('reason')||'').trim();let input:Record<string,unknown>;
  if(type==='GAME_METADATA_UPDATE'){const order=String(form.get('sort_order'));if(!/^(0|[1-9][0-9]{0,8})$/.test(order)){setError('展示顺序应为非负整数。');return}input={title:String(form.get('title')).trim(),sort_order:order}}
  else if(type==='GAME_PUBLICATION_SET')input={publication_state:String(form.get('publication_state'))};else input={configured_runtime_state:String(form.get('configured_runtime_state'))};
  setError('');mutation.prepare({operation_type:type,target:{type:'game',id:game.game_slug,expected_version:game.version},input,reason});
 }
 function configAction(event:FormEvent<HTMLFormElement>){event.preventDefault();if(!game||!config)return;const form=new FormData(event.currentTarget),type=String(form.get('operation_type'));mutation.prepare({operation_type:type,target:{type:'game_config',id:config.config_version_id,expected_version:config.version_number},input:{game_slug:game.game_slug},reason:String(form.get('reason')||'').trim()})}
 function saveDraft(event:FormEvent<HTMLFormElement>){event.preventDefault();if(!game||!config?.editable_config||config.status!=='DRAFT')return;const form=new FormData(event.currentTarget);try{const edited=editableSchema.parse({...config.editable_config,prize_table_version:String(form.get('prize_table_version')||'').trim(),prizes:config.editable_config.prizes.map((prize,index)=>({...prize,weight:String(form.get(`weight_${index}`)||'')}))});setError('');mutation.prepare({operation_type:'GAME_CONFIG_DRAFT_SAVE',target:{type:'game_config',id:config.config_version_id,expected_version:config.version_number},input:{game_slug:game.game_slug,config:edited},reason:String(form.get('reason')||'').trim()})}catch{setError('请检查奖池版本与各档整数权重。')}}
 return <><header className="page-heading"><div><p className="eyebrow">OPERATIONS / GAMES</p><h1>游戏管理</h1><p>核对发布状态、运行开关与配置版本；已受理的游戏仍按原配置恢复和结算。</p></div><button onClick={overview.reload} disabled={overview.loading||blocked}>刷新列表</button></header>
 <OpsMutationPanel mutation={mutation}/>{error&&<Alert>{error}</Alert>}
 <section className="panel">{overview.loading?<Loading/>:overview.error?<Alert>{overview.error}</Alert>:overview.data&&<><p className="hint">观察时间：{opsTime(overview.data.generated_at)}</p><div className="table-wrap" role="region" aria-label="游戏运行状态" tabIndex={0}><table><thead><tr><th>游戏</th><th>发布 / 运行</th><th>最近 24 小时</th><th>需要核对</th><th>配置</th></tr></thead><tbody>{overview.data.games.map(item=><tr key={item.game_slug}><td><button disabled={blocked} onClick={()=>{setSlug(item.game_slug);setSelectedConfig('')}}>{item.title}</button><small>{item.game_slug}</small></td><td>{labels[item.publication_state]} / {labels[item.configured_runtime_state]}</td><td>{item.rounds_24h} 局</td><td>{item.needs_review_rounds} 局</td><td>{item.active_config?labels[item.active_config.status]:'未配置'}</td></tr>)}</tbody></table></div>{!overview.data.games.length&&<p>暂无可管理的游戏。</p>}</>}</section>
 {slug&&<section className="panel"><div className="section-heading"><h2>游戏详情</h2><button disabled={detail.loading||blocked} onClick={detail.reload}>刷新详情</button></div>{detail.loading?<Loading/>:detail.error?<Alert>{detail.error}</Alert>:game&&<>
 <h3>{game.title}</h3><OpsFacts value={{game_slug:game.game_slug,version:game.version,implementation_key:game.implementation_key,publication_state:game.publication_state,configured_runtime_state:game.configured_runtime_state}}/>
 <form onSubmit={change} key={game.game_slug+game.version}><fieldset disabled={blocked}><legend>发布与运行设置</legend><label>管理动作<select name="operation_type"><option value="GAME_METADATA_UPDATE" disabled={!can('GAME_METADATA_UPDATE')}>修改展示信息</option><option value="GAME_PUBLICATION_SET" disabled={!can('GAME_PUBLICATION_SET')}>修改发布状态</option><option value="GAME_RUNTIME_SET" disabled={!can('GAME_RUNTIME_SET')}>修改运行开关</option></select></label><label>游戏标题<input name="title" defaultValue={game.title} maxLength={100} required/></label><label>展示顺序<input name="sort_order" defaultValue={game.sort_order} inputMode="numeric" maxLength={9} required/></label><label>发布状态<select name="publication_state" defaultValue={game.publication_state}>{['DRAFT','PUBLISHED','COMING_SOON','RETIRED'].map(value=><option key={value} value={value}>{labels[value]}</option>)}</select></label><label>运行状态<select name="configured_runtime_state" defaultValue={game.configured_runtime_state}>{['AVAILABLE','MAINTENANCE','UNAVAILABLE'].map(value=><option key={value} value={value}>{labels[value]}</option>)}</select></label><label>操作原因<textarea name="reason" maxLength={2048} required/></label><button type="submit">读取操作影响</button></fieldset></form>
 <h3>配置版本</h3><p className="hint">默认显示当前生效版本；选择历史版本后，下方只读概率详情会跟随切换。规则基线受锁定保护；新的版本经过验证、预览和独立确认后才可生效。</p><label>选择版本<select value={selectedConfig} onChange={event=>setSelectedConfig(event.target.value)} disabled={blocked}><option value="">请选择</option>{detail.data!.config_versions.map(item=><option key={item.config_version_id} value={item.config_version_id}>{labels[item.status]} · {item.config_version_id}</option>)}</select></label>
 {config&&<><OpsFacts value={config}/><OpsProbabilityDetails gameSlug={game.game_slug} config={config} artifacts={detail.data!.validation_artifacts}/><form onSubmit={configAction}><fieldset disabled={blocked}><legend>配置生命周期</legend><label>操作<select name="operation_type"><option value="GAME_CONFIG_CLONE_DRAFT" disabled={!can('GAME_CONFIG_CLONE_DRAFT')}>从此版本建立草稿</option><option value="GAME_CONFIG_VALIDATE" disabled={config.status!=='DRAFT'||!can('GAME_CONFIG_VALIDATE')}>验证草稿</option><option value="GAME_CONFIG_PREVIEW" disabled={config.status!=='VALIDATED'||!can('GAME_CONFIG_PREVIEW')}>生成配置预览</option><option value="GAME_CONFIG_ACTIVATE" disabled={config.status!=='PREVIEWED'||!can('GAME_CONFIG_ACTIVATE')}>激活已预览配置</option></select></label><label>操作原因<textarea name="reason" maxLength={2048} required/></label><button type="submit">读取配置操作影响</button></fieldset></form></>}
 {config?.editable_config&&config.status==='DRAFT'&&<form onSubmit={saveDraft} key={config.config_version_id+config.version_number}><fieldset disabled={blocked||!can('GAME_CONFIG_DRAFT_SAVE')}><legend>编辑奖池草稿</legend><label>奖池版本<input name="prize_table_version" defaultValue={config.editable_config.prize_table_version} maxLength={128} required/></label>{config.editable_config.prizes.map((prize,index)=><label key={prize.tier}>{prize.tier} · 固定倍率 {prize.multiplier}<input name={`weight_${index}`} defaultValue={prize.weight} inputMode="numeric" maxLength={19} required/></label>)}<label>操作原因<textarea name="reason" maxLength={2048} required/></label><button type="submit">预览草稿变更</button></fieldset></form>}
 <details><summary>验证资料</summary><OpsFacts value={detail.data!.validation_artifacts}/></details>
 </>}</section>}</>;
}
