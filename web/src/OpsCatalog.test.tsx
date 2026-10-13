import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, it, vi } from 'vitest';
import { App } from './App';
import { ApiClient } from './api';
import { catalogOps, type CatalogModel } from './catalog-api';
import { ok, failed } from './m1-test-fixtures';
import { OpsProbabilityDetails } from './ops/OpsGames';

it('saves local model prices, publication and family covers directly without legacy bootstrap',async()=>{
 let item:CatalogModel={model_id:'synthetic/model',metadata:{display_name:'',family:'',summary:'',context_length:null,subtitle:'',tags:[],use_cases:[],special_pricing_note:'',asset_id:''},publication_state:'PENDING_METADATA',recommended:false,sort_order:0,version:'1',metadata_version:'1',published_at:null,retired_at:null,updated_at:'2026-09-06T00:00:00Z',availability_state:'CONFIGURED',source_observed_at:'2026-09-06T00:00:00Z',last_seen_at:'2026-09-06T00:00:00Z',endpoint_status:'configured_subset_not_health',endpoints:[{kind:'openai',path:'/v1/chat/completions',method:'POST'}],price:{mode:'ratio',configured:true,status:'reference',dimensions:[],unquoted_dimensions:[]},can_use:false,freshness:{state:'CURRENT',last_observed_at:null,last_verified_at:null,stale_after_seconds:600,disable_after_seconds:1800}};

 let price={input:'0.5',output:'1',cache_read:'0',cache_write:'0',cache_write_1h:'0',per_request:null,per_second:null,default_seconds:0};
 let failRead=false;
 const image={asset_id:'uploaded-cover',src:'/assets/models/uploads/gemini/'+'a'.repeat(64)+'.png',alt:'新家族封面',width:640,height:960,focal_point:[0.5,0.5]};
 let cover={family:'gemini',version:'1',image:null as typeof image|null,default_image:null};
 const fetcher=vi.fn(async(path:string,init?:RequestInit)=>{
  if(path==='/api/v1/session/bootstrap')return ok({authentication_state:'AUTHENTICATED',csrf_token:'local-csrf'});
  if(path==='/api/user/self')return ok({id:1,username:'operator',role:10});
  if(path.startsWith('/platform/v1/access-gate?'))return ok({stage:'READY',user_id:'1',route:new URLSearchParams(path.split('?')[1]).get('route')});
  if(path==='/api/v1/maintenance/notices')return ok({items:[]});
  if(path===catalogOps+'/family-covers/upload')return ok({asset_id:image.asset_id,image,rights_status:'ORIGINAL_GENERATED',rights_note:'现有素材',reused:false});
  if(path===catalogOps+'/family-covers'){
   if(init?.method==='PUT'){const body=JSON.parse(String(init.body));cover={...cover,image:body.asset_id?image:null};return ok(cover)}
   return ok({items:[cover],upload_enabled:true});
  }
  if(path===catalogOps&&init?.method==='PUT'){
   const v=JSON.parse(String(init.body));item={...item,metadata:v.metadata,recommended:v.recommended,sort_order:v.sort_order,publication_state:v.public?'PUBLISHED':'HIDDEN'};price=v.price_settings||price;return ok({item,price_settings:price});
  }
  if(path===catalogOps+'/publication'&&init?.method==='PUT'){item={...item,publication_state:JSON.parse(String(init.body)).public?'PUBLISHED':'HIDDEN'};return ok({saved:true})}
  if(path.startsWith(catalogOps+'/detail?'))return ok({item,price_settings:price});
  if(path.startsWith(catalogOps))return failRead?failed():ok({items:[item],total:1,offset:0,limit:50,freshness:item.freshness,vocabulary:{families:[{value:'gemini',label:'Gemini'}],tags:[],use_cases:[],assets:[]}});
  throw new Error('Unexpected endpoint: '+path);
 });
 const client=new ApiClient(fetcher);await client.bootstrap();
 const view=render(<MemoryRouter initialEntries={['/ops/models']}><App client={client}/></MemoryRouter>);
 fireEvent.click(await screen.findByRole('button',{name:'编辑模型'}));
 fireEvent.change(await screen.findByLabelText('展示名称'),{target:{value:'本地模型'}});
 fireEvent.change(screen.getByLabelText('模型家族'),{target:{value:'gemini'}});
 fireEvent.change(screen.getByLabelText('模型简介'),{target:{value:'本地可编辑目录。'}});
 fireEvent.change(screen.getByLabelText('输入单价'),{target:{value:'0.125'}});
 fireEvent.click(screen.getByRole('checkbox',{name:'公开此模型'}));
 failRead=true;
 fireEvent.click(screen.getByRole('button',{name:'保存模型'}));
 await screen.findByText('已保存');expect(price.input).toBe('0.125');expect(item.publication_state).toBe('PUBLISHED');
 await screen.findByText('已保存，列表读取失败，请刷新查看。');
 failRead=false;fireEvent.click(screen.getByRole('button',{name:'刷新'}));
 await waitFor(()=>expect(screen.queryByText('已保存，列表读取失败，请刷新查看。')).not.toBeInTheDocument());
 const save=fetcher.mock.calls.find(([p,i])=>p===catalogOps&&i?.method==='PUT')!;
 expect(new Headers(save[1]?.headers).get('X-CSRF-Token')).toBe('local-csrf');
 expect(Object.keys(JSON.parse(String(save[1]?.body))).sort()).toEqual(['metadata','model_id','price_settings','public','recommended','sort_order']);
 fireEvent.click(screen.getByRole('checkbox',{name:'选择 synthetic/model'}));
 fireEvent.click(screen.getByRole('button',{name:'批量隐藏'}));await waitFor(()=>expect(item.publication_state).toBe('HIDDEN'));
 await waitFor(()=>expect(screen.getByRole('button',{name:'家族封面'})).toBeEnabled());
 fireEvent.click(screen.getByRole('button',{name:'家族封面'}));
 const dialog=await screen.findByRole('dialog',{name:'家族封面管理'});
 fireEvent.change(within(dialog).getByLabelText('上传图片'),{target:{files:[new File(['raster'],'cover.png',{type:'image/png'})]}});
 fireEvent.change(within(dialog).getByLabelText('图片替代文字'),{target:{value:image.alt}});
 fireEvent.change(within(dialog).getByLabelText('素材来源说明'),{target:{value:'现有素材'}});
 fireEvent.click(within(dialog).getByRole('button',{name:'上传候选图'}));
 fireEvent.click(await within(dialog).findByRole('button',{name:'保存封面'}));
 await within(dialog).findByText('封面已保存');expect(cover.image?.asset_id).toBe(image.asset_id);
 const upload=fetcher.mock.calls.find(([p])=>p===catalogOps+'/family-covers/upload')!;
 expect(upload[1]?.body).toBeInstanceOf(FormData);expect(new Headers(upload[1]?.headers).has('Content-Type')).toBe(false);
 fireEvent.click(within(dialog).getByRole('button',{name:'恢复默认封面'}));await waitFor(()=>expect(cover.image).toBeNull());
 expect(fetcher.mock.calls.some(([p])=>/prepare|execute|ops\/bootstrap/.test(p))).toBe(false);
 view.unmount();
});

it('shows existing game probability details and directly saves local game configuration',async()=>{
  const config={config_version_id:'01993200-0000-7000-8000-000000000003',version_number:'2',status:'ACTIVE' as const,config_schema_version:'summon-config-v1',ruleset_version:'summon-rules-v1',algorithm_version:'summon-map-v1',config_hash:'a'.repeat(64),created_at:'2026-09-20T00:00:00Z',validated_at:'2026-09-20T00:00:00Z',previewed_at:'2026-09-20T00:00:00Z',activated_at:'2026-09-20T00:00:00Z',editable_config:{type:'SUMMON_V1' as const,prize_table_version:'summon-prize-v3',prizes:[{tier:'T0',multiplier:'0',weight:'1'},{tier:'T5',multiplier:'100',weight:'1'}]}};
  const artifact={validation_artifact_id:'01993200-0000-7000-8000-000000000203',config_version_id:config.config_version_id,artifact_type:'EXACT_MATH',implementation_key:'direct.summon.v1',ruleset_version:config.ruleset_version,algorithm_version:config.algorithm_version,config_hash:config.config_hash,validator_version:'direct-validate-v1',validation_build:'direct-games-v1',result_summary:{rtp:'50',win:'1/2',loss:'1/2',break_even:'0',top:'1/2'},artifact_sha256:'b'.repeat(64),status:'VERIFIED',generated_at:'2026-09-20T00:00:00Z',verified_at:'2026-09-20T00:00:00Z'};
  const probability=render(<OpsProbabilityDetails gameSlug="summon" config={config} artifacts={[{...artifact,config_hash:'c'.repeat(64)}]}/>);
  expect(screen.getByText('尚无可信数据')).toBeVisible();expect(screen.queryByText('理论返还率（RTP）')).not.toBeInTheDocument();
  probability.rerender(<OpsProbabilityDetails gameSlug="summon" config={config} artifacts={[artifact]}/>);
  expect(screen.queryByText('尚无可信数据')).not.toBeInTheDocument();expect(screen.getByText('summon-map-v1')).toBeVisible();
  const metrics=screen.getByRole('heading',{name:'只读概率详情'}).closest('section')!;expect(within(metrics).getByText('理论返还率（RTP）')).toBeVisible();expect(within(metrics).getByText('十连净赢')).toBeVisible();
  const slotConfig={...config,ruleset_version:'slot-rules-v3',algorithm_version:'slot-map-v1',editable_config:undefined};
  const slotResult={complete:true,method:'EXACT_ENUMERATION_PRODUCTION_LINE_EVALUATOR',cases:33554432,total_combinations:33554432,rates:{WIN:{fraction:'4565/8192'},BREAK_EVEN:{fraction:'0/1'},LOSS:{fraction:'3627/8192'},NONZERO_PAYOUT:{fraction:'4565/8192'},NO_WIN:{fraction:'3627/8192'},PARTIAL_RETURN:{fraction:'0/1'}},rtp:{fraction:'19181593/16777216'},top:{count:1,total_wager_multiplier:{fraction:'5201/10'}},wild_five:{count:10,line_multiplier:5000,probability:{fraction:'5/16777216'}}};
  const slotArtifact={...artifact,artifact_type:'SLOT_EXHAUSTIVE',implementation_key:'direct.slot.v1',ruleset_version:slotConfig.ruleset_version,algorithm_version:slotConfig.algorithm_version,result_summary:{config_version:slotConfig.config_version_id,config_hash:slotConfig.config_hash,ruleset_version:slotConfig.ruleset_version,algorithm_version:slotConfig.algorithm_version,artifact_type:'SLOT_EXHAUSTIVE',implementation_key:'direct.slot.v1',result_json:JSON.stringify(slotResult)}};
  probability.rerender(<OpsProbabilityDetails gameSlug="slot" config={slotConfig} artifacts={[slotArtifact]}/>);expect(screen.getByText('完整枚举').parentElement).toHaveTextContent('33,554,432');expect(screen.getByText('单转净赢')).toBeVisible();expect(screen.getByText('Wild 五连盘面').parentElement).toHaveTextContent('每线 ×5000');
  const blackjackConfig={...config,ruleset_version:'blackjack-rules-v2',algorithm_version:'blackjack-map-v1',editable_config:undefined};
  const blackjackResult={complete:true,method:'REFERENCE_STRATEGY_BASE_PLUS_EXACT_EXPECTED_FAIR_RETURN',rounds:10000000,initial_wager_denominator:{rtp:{point:{fraction:'1/1'},normal_approximation_ci95:[0.99,1.01]}},base_sample:{rtp:{fraction:'9962921/10000000'}},fair_return:{fraction:'37079/10000000'},reference_strategy:{version:'blackjack-basic-s17-das-total-v1',scope:'six-deck reference'}};
  const blackjackArtifact={...artifact,artifact_type:'BLACKJACK_RTP',implementation_key:'direct.blackjack.v1',ruleset_version:blackjackConfig.ruleset_version,algorithm_version:blackjackConfig.algorithm_version,result_summary:{config_version:blackjackConfig.config_version_id,config_hash:blackjackConfig.config_hash,ruleset_version:blackjackConfig.ruleset_version,algorithm_version:blackjackConfig.algorithm_version,artifact_type:'BLACKJACK_RTP',implementation_key:'direct.blackjack.v1',result_json:JSON.stringify(blackjackResult)}};
  probability.rerender(<OpsProbabilityDetails gameSlug="blackjack" config={blackjackConfig} artifacts={[blackjackArtifact]}/>);expect(screen.getByText(/不是任意策略或短期结果保证/)).toBeVisible();expect(screen.getByText('净赢率').parentElement).toHaveTextContent('未包含');
  probability.unmount();
  let failDetail=false;
  let game={game_slug:'summon',title:'召唤',sort_order:'3',version:'1',publication_state:'PUBLISHED',configured_runtime_state:'AVAILABLE',implementation_key:'direct.summon.v1',active_config:config,rounds_24h:'10',needs_review_rounds:'0'};
  let configs=[{...config,status:'ACTIVE' as string}],artifacts=[artifact];
  const fetcher=vi.fn(async(path:string,init?:RequestInit)=>{
   if(path==='/api/v1/session/bootstrap')return ok({authentication_state:'AUTHENTICATED',csrf_token:'game-csrf'});
   if(path==='/api/user/self')return ok({id:1,username:'operator',role:10});
   if(path.startsWith('/platform/v1/access-gate?'))return ok({stage:'READY',user_id:'1',route:'/ops/games'});
   if(path==='/api/v1/maintenance/notices')return ok({items:[]});
   if(path==='/api/v1/ops/games')return ok({generated_at:config.created_at,games:[game]});
   if(path==='/api/v1/ops/games/summon'&&init?.method==='PUT'){game={...game,...JSON.parse(String(init.body)),version:'2'};return ok(game)}
   if(path.startsWith('/api/v1/ops/games/summon/configs/')){
    const action=path.split('/')[8]||'save';let next={...configs[0]};
    if(action==='clone')next={...config,config_version_id:'01993200-0000-7000-8000-000000000900',version_number:'3',status:'DRAFT'};
    if(action==='save')next={...next,editable_config:JSON.parse(String(init?.body)),config_hash:'d'.repeat(64)};
    if(action==='validate')next={...next,status:'VALIDATED'};
    if(action==='activate')next={...next,status:'ACTIVE'};
    configs=[next];artifacts=[{...artifact,config_version_id:next.config_version_id,config_hash:next.config_hash}];return ok(next);
   }
   if(path.startsWith('/api/v1/ops/games/summon?')){
    if(failDetail)return failed();const q=new URLSearchParams(path.split('?')[1]);
    return ok({generated_at:config.created_at,game,config_versions:configs,validation_artifacts:artifacts,baseline_locked:true,page:Number(q.get('page')),page_size:20,config_total:21});
   }
   throw new Error('Unexpected games endpoint: '+path);
  });
  const client=new ApiClient(fetcher);await client.bootstrap();
  const app=render(<MemoryRouter initialEntries={['/ops/games']}><App client={client}/></MemoryRouter>);
  fireEvent.click(await screen.findByRole('button',{name:'召唤'}));
  const dialog=await screen.findByRole('dialog',{name:'游戏详情'});
  await within(dialog).findByRole('button',{name:'保存游戏设置'});
  fireEvent.click(within(dialog).getByRole('button',{name:'下一页'}));
  await waitFor(()=>expect(fetcher.mock.calls.some(([p])=>p.includes('/summon?page=2'))).toBe(true));
  await within(dialog).findByText('共 21 条 · 第 2 / 2 页');
  fireEvent.click(within(dialog).getByRole('button',{name:'上一页'}));
  await within(dialog).findByText('共 21 条 · 第 1 / 2 页');
  fireEvent.change(within(dialog).getByLabelText('标题'),{target:{value:'新召唤标题'}});
  failDetail=true;fireEvent.click(within(dialog).getByRole('button',{name:'保存游戏设置'}));
  await within(dialog).findByText('游戏设置已保存。');await within(dialog).findByRole('alert');
  failDetail=false;fireEvent.click(within(dialog).getByRole('button',{name:'刷新详情'}));
  await within(dialog).findByDisplayValue('新召唤标题');
  expect(fetcher.mock.calls.filter(([p,i])=>p==='/api/v1/ops/games/summon'&&i?.method==='PUT')).toHaveLength(1);
  fireEvent.click(within(dialog).getByRole('button',{name:'复制为草稿'}));
  await within(dialog).findByRole('button',{name:'保存草稿'});
  fireEvent.change(within(dialog).getByLabelText('奖池版本'),{target:{value:'summon-ops-local'}});
  fireEvent.click(within(dialog).getByRole('button',{name:'保存草稿'}));
  await within(dialog).findByText('草稿已保存。');
  await waitFor(()=>expect(within(dialog).getByRole('button',{name:'验证配置'})).toBeEnabled());
  fireEvent.click(within(dialog).getByRole('button',{name:'验证配置'}));
  await within(dialog).findByText('配置已验证，可查看概率工件后启用。');
  const confirm=vi.spyOn(window,'confirm').mockReturnValue(true);
  await waitFor(()=>expect(within(dialog).getByRole('button',{name:'启用配置'})).toBeEnabled());
  fireEvent.click(within(dialog).getByRole('button',{name:'启用配置'}));
  await within(dialog).findByText('配置已生效。');expect(confirm).toHaveBeenCalledOnce();confirm.mockRestore();
  const writes=fetcher.mock.calls.filter(([,i])=>i?.method==='PUT'||i?.method==='POST');expect(writes).toHaveLength(5);
  for(const [,init] of writes){expect(new Headers(init?.headers).get('X-CSRF-Token')).toBe('game-csrf');expect(String(init?.body)).not.toMatch(/expected_version|authz_epoch|operation_id|reason/)}
  expect(fetcher.mock.calls.some(([p])=>/prepare|execute|ops\/bootstrap|authz_epoch/.test(p))).toBe(false);
  app.unmount();

 });
