import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, it, vi } from 'vitest';
import { Catalog, CatalogDetailPage, APIAccess, CatalogPersona, CatalogPriceTable } from './Catalog';
import { ApiClient } from './api';
import { catalogModelPath, catalogTime, type CatalogModel, type CatalogPage } from './catalog-api';
import { bundle, ok } from './m1-test-fixtures';
import { App } from './App';
import { peekRouteIntent } from './post-auth-intent';

export const model:CatalogModel={model_id:'组织/model',metadata:{display_name:'月海合成模型',family:'gemini',summary:'仅用于验收的模型说明。',context_length:null,subtitle:'可核对的连接',tags:['writing'],use_cases:['writing'],special_pricing_note:'',asset_id:''},publication_state:'PUBLISHED',recommended:true,sort_order:0,version:'2',metadata_version:'2',published_at:null,retired_at:null,updated_at:'2026-09-06T00:00:00Z',availability_state:'CONFIGURED',source_observed_at:'2026-09-06T00:00:00Z',last_seen_at:'2026-09-06T00:00:00Z',endpoint_status:'configured_subset_not_health',endpoints:[{kind:'openai',path:'/v1/chat/completions',method:'POST'}],price:{mode:'ratio',configured:true,status:'reference',dimensions:[{kind:'input',unit:'API_Credit_per_1M_tokens',amount:'0.00000000001',source:'native',condition:'uncached_plain_text_tokens',support:'configured'}],unquoted_dimensions:['image']},can_use:true,freshness:{state:'CURRENT',last_observed_at:'2026-09-06T00:00:00Z',last_verified_at:'2026-09-06T00:00:00Z',stale_after_seconds:600,disable_after_seconds:1800}};
export const page:CatalogPage={items:[model],total:1,offset:0,limit:24,freshness:model.freshness,vocabulary:{families:[{value:'gemini',label:'Gemini'}],tags:[{value:'writing',label:'写作'}],use_cases:[{value:'writing',label:'创作与写作'}],assets:[]},price_dimension:'',price_unit:''};
function browser(path:string,element:React.ReactNode){return render(<MemoryRouter initialEntries={[path]}>{element}</MemoryRouter>)}
it('keeps public model detail anonymous and saves its choice through the single App login intent',async()=>{
 sessionStorage.clear();const calls:string[]=[];
 const c=new ApiClient(async path=>{calls.push(path);if(path.includes('/auth/refresh'))return new Response('{}',{status:401});if(path==='/platform/v1/admission/config')return ok({enabled:false,registration_enabled:false,eligibility:'注册暂未开放'});if(path.startsWith('/platform/v1/announcements'))return ok({item:null});return ok({item:model,vocabulary:page.vocabulary,api_base_url:'https://api.example/v1'})});
 browser(catalogModelPath(model.model_id),<App client={c}/>);await waitFor(()=>expect(c.getSnapshot().ready).toBe(true));fireEvent.click(await screen.findByRole('button',{name:'使用此模型'}));await screen.findByLabelText('用户名');expect(peekRouteIntent()).toBe('/api/access?model_id=%E7%BB%84%E7%BB%87%2Fmodel&intent=use');expect(calls.some(path=>path.startsWith('/platform/v1/access-gate')||path.startsWith('/api/token/'))).toBe(false);
});
it('does not mount model operations before the central Gate resolves its scope',async()=>{
 const calls:string[]=[];const c=new ApiClient(async path=>{calls.push(path);if(path.includes('/auth/refresh'))return ok(bundle);if(path.startsWith('/platform/v1/access-gate?'))return ok({user_id:'1',route:'/ops/models',stage:'ROLE_DENIED'});return new Response('{}',{status:503})});
 browser('/ops/models',<App client={c}/>);expect(await screen.findByRole('alert')).toHaveTextContent('当前账户未具备此页面所需权限');expect(calls.some(path=>path.startsWith('/platform/v1/ops/models'))).toBe(false);
});
it('binds the key-page Gate to the exact selected model query',async()=>{
 const routes:string[]=[];const c=new ApiClient(async path=>{if(path.includes('/auth/refresh'))return ok(bundle);if(path.startsWith('/platform/v1/access-gate?')){const route=new URLSearchParams(path.split('?')[1]).get('route')!;routes.push(route);return ok({user_id:'1',route,stage:'MIGRATION_UNVERIFIED'})}return new Response('{}',{status:503})});
 browser('/keys?model_id=team%2Fmodel',<App client={c}/>);await screen.findByRole('alert');expect(routes).toEqual(['/keys?model_id=team%2Fmodel']);
});
it('reads the public catalog and applies explicit same-dimension price and context filters',async()=>{
 const paths:string[]=[];const c=new ApiClient(async path=>{paths.push(path);return ok(page)});
 browser('/models',<Catalog client={c}/>);expect(await screen.findByRole('heading',{name:'月海合成模型'})).toBeInTheDocument();expect(screen.getByText('0.00000000001')).toBeInTheDocument();
 fireEvent.change(screen.getByLabelText('名称或模型 ID'),{target:{value:'组织'}});fireEvent.change(screen.getByLabelText('比较价格维度'),{target:{value:'input'}});fireEvent.change(screen.getByLabelText('最低价格'),{target:{value:'0.00000001'}});fireEvent.click(screen.getByLabelText('仅显示未知 context'));fireEvent.click(screen.getByRole('button',{name:'应用筛选'}));
 await waitFor(()=>expect(paths.at(-1)).toContain('price_dimension=input'));expect(paths.at(-1)).toContain('min_price=0.00000001');expect(paths.at(-1)).toContain('unknown_context=true');
});
it('shows unavailable source and unknown context without inventing a price or use action',async()=>{
 const expired={...model,can_use:false,availability_state:'NOT_OBSERVED',price:{...model.price,dimensions:[]},freshness:{...model.freshness,state:'EXPIRED'}};
 const c=new ApiClient(async()=>ok({item:expired,vocabulary:page.vocabulary,api_base_url:''}));
 browser(catalogModelPath(model.model_id),<CatalogDetailPage client={c}/>);expect(await screen.findByText('本次来源未观察到')).toBeInTheDocument();expect(screen.getByText('Context 未知')).toBeInTheDocument();expect(screen.getByText('未提供数值报价')).toBeInTheDocument();expect(screen.getByRole('button',{name:'暂不可接入'})).toBeDisabled();
});
it('hands the bounded model choice to the existing login gate without writing browser auth storage',async()=>{
 const login=vi.fn();const c=new ApiClient(async path=>path.includes('/auth/refresh')?new Response('{}',{status:401}):ok({item:model,vocabulary:page.vocabulary,api_base_url:'https://api.example/v1'}));await c.bootstrap();
 browser(catalogModelPath(model.model_id),<CatalogDetailPage client={c} onLoginRequired={login}/>);fireEvent.click(await screen.findByRole('button',{name:'使用此模型'}));await waitFor(()=>expect(login).toHaveBeenCalledWith('/api/access?model_id=%E7%BB%84%E7%BB%87%2Fmodel&intent=use'));
});
it('copies a placeholder cURL without reading keys or calling a model',async()=>{
 const copy=vi.fn().mockResolvedValue(undefined);Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:copy}});const paths:string[]=[];
 const c=new ApiClient(async path=>{paths.push(path);return ok(path.includes('/detail?')?{item:model,vocabulary:page.vocabulary,api_base_url:'https://api.example/v1'}:path.endsWith('/access-config')?{api_base_url:'https://api.example/v1'}:page)});
 browser('/api/access?model_id='+encodeURIComponent(model.model_id),<APIAccess client={c}/>);fireEvent.click(await screen.findByRole('button',{name:'复制 cURL'}));await waitFor(()=>expect(copy).toHaveBeenCalledTimes(1));expect(copy.mock.calls[0][0]).toContain('<YOUR_API_KEY>');expect(paths.every(p=>p.startsWith('/platform/v1/models'))).toBe(true);
});
it('clears personal quotes after logout and never restores a late private response',async()=>{
 let resolve!:(r:Response)=>void;const privateResponse=new Promise<Response>(r=>resolve=r);
 const c=new ApiClient(async path=>path==='/api/user/login'?ok(bundle):path.includes('/auth/logout')?ok():path.includes('/personal-price?')?privateResponse:ok({item:model,vocabulary:page.vocabulary,api_base_url:''}));await c.login('one','x');
 browser(catalogModelPath(model.model_id),<CatalogDetailPage client={c}/>);await screen.findByRole('heading',{name:'本人会话报价'});await c.logout();resolve(ok({model_id:model.model_id,basis:'current_user_group_reference_not_token_selection',observed_at:'2026-09-06T00:00:00Z',quotes:[{candidate:1,price:{...model.price,dimensions:[{...model.price.dimensions[0],amount:'987.654'}]}}]}));
 await waitFor(()=>expect(screen.queryByRole('heading',{name:'本人会话报价'})).not.toBeInTheDocument());expect(screen.queryByText('987.654')).not.toBeInTheDocument();
});
it('labels conditional request pricing and keeps its own unit',()=>{
 render(<CatalogPriceTable price={{mode:'per_request',configured:true,status:'conditional',dimensions:[{kind:'text_request_base',unit:'API_Credit_per_request',amount:'0',condition:'plain_text_request_without_extra_multipliers_or_tool_fees',source:'native_effective',support:'not_asserted'}],unquoted_dimensions:[]}}/>);
 expect(screen.getByText('· 条件计价')).toBeInTheDocument();expect(screen.getByText('API Credit / 次请求')).toBeInTheDocument();expect(screen.getByText('0')).toBeInTheDocument();
});
it('keeps the persona slot usable after both approved local images fail',()=>{
 const view=render(<CatalogPersona model={{...model,metadata:{...model.metadata,asset_id:'synthetic-approved'}}} assets={[{asset_id:'synthetic-approved',src:'/assets/models/test-master.webp',fallback:'/assets/models/test-fallback.webp',focal_point:[0.5,0.5],safe_area:0.08,status:'PRODUCTION_READY',rights_status:'LICENSED_OR_APPROVED'}]}/>);
 const image=view.container.querySelector('img')!;expect(image).toHaveAttribute('src','/assets/models/test-master.webp');fireEvent.error(image);expect(image).toHaveAttribute('src','/assets/models/test-fallback.webp');fireEvent.error(image);expect(view.container.querySelector('img')).toBeNull();expect(screen.getByLabelText('模型家族几何标识')).toBeInTheDocument();
});
it('labels retained price and endpoint observation separately from the later missing-source check',async()=>{
 const missing={...model,can_use:false,availability_state:'NOT_OBSERVED',last_seen_at:'2026-09-01T00:00:00Z'};
 const c=new ApiClient(async()=>ok({item:missing,vocabulary:page.vocabulary,api_base_url:''}));
 browser(catalogModelPath(model.model_id),<CatalogDetailPage client={c}/>);
 expect(await screen.findByText('本次来源未观察到')).toBeInTheDocument();
 expect(screen.getByText('价格与端点最后观察：'+catalogTime(missing.last_seen_at))).toBeInTheDocument();
});
