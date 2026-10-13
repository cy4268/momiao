import { ApiClient, ApiError } from './api';
import { readAccessGate } from './access-gate-api';
import { saveRouteIntent, validModelID } from './post-auth-intent';
export { validModelID } from './post-auth-intent';

export const catalogRoot='/platform/v1/models';
export const catalogOps='/platform/v1/ops/models';
export interface CatalogCoverImage {asset_id:string;src:string;alt:string;width:number;height:number;focal_point:[number,number]}
export interface CatalogFamilyCover {family:string;version:string;image:CatalogCoverImage|null;default_image:CatalogCoverImage|null}
export interface CatalogFamilyCoverPage {items:CatalogFamilyCover[];upload_enabled:boolean}
const coverRoot=catalogOps+'/family-covers';
export function validCatalogCoverSrc(src:string){return src.startsWith('/assets/models/uploads/')?/^\/assets\/models\/uploads\/(?:deepseek|gpt|claude|gemini|glm|kimi|ernie|qwen|grok|other)\/[a-f0-9]{64}\.(?:png|jpg|webp)$/.test(src):/^\/assets\/models\/[a-zA-Z0-9][a-zA-Z0-9_-]*\.(?:webp|png)$/.test(src)}
export interface CatalogCoverUploadCommand {family:string;alt:string;rights_status:string;rights_note:string}
export interface CatalogCoverUploadResult {asset_id:string;image:CatalogCoverImage;rights_status:string;rights_note:string;reused:boolean}
export function readFamilyCovers(client:ApiClient){return client.request<CatalogFamilyCoverPage>(coverRoot)}
export function uploadFamilyCover(client:ApiClient,command:CatalogCoverUploadCommand,file:File){
 const body=new FormData();body.append('metadata',JSON.stringify(command));body.append('file',file);
 return client.request<CatalogCoverUploadResult>(coverRoot+'/upload','POST',body);
}
export function saveFamilyCover(client:ApiClient,family:string,asset_id:string){return client.request<CatalogFamilyCover>(coverRoot,'PUT',{family,asset_id})}
export type CatalogChoice={value:string;label:string};
export interface CatalogAsset {asset_id:string;src:string;fallback:string;focal_point:[number,number];safe_area:number;status:string;rights_status:string}
export interface CatalogVocabulary {families:CatalogChoice[];tags:CatalogChoice[];use_cases:CatalogChoice[];assets:CatalogAsset[]}
export interface CatalogMetadata {display_name:string;family:string;summary:string;context_length:string|null;subtitle:string;tags:string[];use_cases:string[];special_pricing_note:string;asset_id:string}
export interface CatalogDimension {kind:string;unit:string;amount:string|null;condition:string;source:string;support:string}
export interface CatalogPrice {default_seconds?:number;mode:string;configured:boolean;status:string;reason?:string;dimensions:CatalogDimension[];unquoted_dimensions:string[]}
export interface CatalogEndpoint {kind:string;path:string;method:string}
export interface CatalogFreshness {state:string;last_observed_at:string|null;last_verified_at:string|null;stale_after_seconds:number;disable_after_seconds:number}
export interface CatalogModel {family_cover?:CatalogFamilyCover;model_id:string;metadata:CatalogMetadata;publication_state:string;recommended:boolean;sort_order:number;version:string;metadata_version:string;published_at:string|null;retired_at:string|null;updated_at:string;availability_state:string;source_observed_at:string;last_seen_at:string;endpoint_status:string;endpoints:CatalogEndpoint[];price:CatalogPrice;can_use:boolean;freshness:CatalogFreshness}
export interface CatalogPage {items:CatalogModel[];total:number;offset:number;limit:number;freshness:CatalogFreshness;vocabulary:CatalogVocabulary;price_dimension:string;price_unit:string}
export type CatalogOpsPage = Omit<CatalogPage,'price_dimension'|'price_unit'>;
export interface CatalogPriceSettings {input:string;output:string;cache_read:string;cache_write:string;cache_write_1h:string;per_request:string|null;per_second:string|null;default_seconds:number}
export interface CatalogOpsDetail {item:CatalogModel;price_settings:CatalogPriceSettings|null}
export interface CatalogSettings {model_id:string;metadata:CatalogMetadata;public:boolean;recommended:boolean;sort_order:number;price_settings:CatalogPriceSettings|null}
export interface CatalogDetail {item:CatalogModel;vocabulary:CatalogVocabulary;api_base_url:string}
export interface CatalogPersonal {model_id:string;observed_at:string;basis:string;quotes:{candidate:number;enabled_configuration:boolean;price:CatalogPrice|null;reason?:string}[]}
export type CatalogLoginRequired=(returnPath:string)=>void;
export const dimensionLabels:Record<string,string>={input:'输入',output:'输出',cache_read:'缓存读取',cache_write:'缓存写入',cache_write_5m:'缓存写入 · 5 分钟',cache_write_1h:'缓存写入 · 1 小时',text_request_base:'按次价格',second:'按秒价格'};
export const availabilityLabels:Record<string,string>={CONFIGURED:'已配置接入',DISABLED:'渠道已停用',NO_CHANNEL:'无渠道'};
export const publicationLabels:Record<string,string>={PENDING_METADATA:'待完善',PUBLISHED:'已公开',HIDDEN:'已隐藏',RETIRED:'已退役'};
export const endpointLabels:Record<string,string>={openai:'Chat Completions','openai-response':'Responses',anthropic:'Messages','image-generation':'Images'};
export const catalogTime=(value:string|null)=>value?new Date(value).toLocaleString('zh-CN',{hour12:false}):'尚无记录';
export function catalogModelPath(id:string){if(!validModelID(id))throw new Error('无效的模型 ID。');return '/models/'+(id==='.'?'~Lg':id==='..'?'~Li4':encodeURIComponent(id).replace(/[~!'()*]/g,c=>'%'+c.charCodeAt(0).toString(16).toUpperCase()));}
export function decodeCatalogModelPath(path:string):string|null {if(!path.startsWith('/models/'))return null;const raw=path.slice(8);try{const id=raw==='~Lg'?'.':raw==='~Li4'?'..':decodeURIComponent(raw);return validModelID(id)?id:null;}catch{return null}}
export function catalogSelectionPath(id:string,intent=true){if(!validModelID(id))throw new Error('无效的模型 ID。');return '/api/access?'+new URLSearchParams({model_id:id,...(intent?{intent:'use'}:{})}).toString();}
export function selectedModel(query:string){const q=new URLSearchParams(query);const id=q.get('model_id');return q.getAll('model_id').length===1&&validModelID(id)?id:null;}
export function decimalDisplay(value:string|null){if(value===null)return '未提供';if(!/^(0|[1-9][0-9]*)(\.[0-9]+)?$/.test(value)||value.length>1400)return '未提供';return value.includes('.')?value.replace(/0+$/,'').replace(/\.$/,''):value;}
const endpointPaths:Record<string,string>={openai:'/v1/chat/completions','openai-response':'/v1/responses',anthropic:'/v1/messages','image-generation':'/v1/images/generations'};
export function catalogCurl(base:string,id:string,endpoint:CatalogEndpoint){
 try{const u=new URL(base);if(u.protocol!=='https:'||u.pathname!=='/v1'||u.username||u.password||u.search||u.hash||u.href!==base||!validModelID(id)||endpoint.method!=='POST'||endpointPaths[endpoint.kind]!==endpoint.path)return '';}catch{return ''}
 const shell=(s:string)=>"'"+s.replace(/'/g,"'\\''")+"'";
 const body=endpoint.kind==='openai-response'?{model:id,input:'Hello'}:endpoint.kind==='image-generation'?{model:id,prompt:'A quiet moonlit garden',n:1}:{model:id,...(endpoint.kind==='anthropic'?{max_tokens:128}:{}),messages:[{role:'user',content:'Hello'}]};
 const auth=endpoint.kind==='anthropic'?['x-api-key: <YOUR_API_KEY>','anthropic-version: 2023-06-01']:['Authorization: Bearer <YOUR_API_KEY>'];
 return ['curl '+shell(base+endpoint.path.slice(3)),...['Content-Type: application/json',...auth].map(h=>'  -H '+shell(h)),'  --data-raw '+shell(JSON.stringify(body,null,2))].join(' \\\n');
}
export function catalogError(e:unknown){const messages:Record<string,string>={CATALOG_ASSET_UPLOAD_DISABLED:'上传尚未配置；当前封面与恢复默认仍可用。',CATALOG_ASSET_UPLOAD_FAILED:'图片上传失败，请重试。',CATALOG_UNAVAILABLE:'模型目录暂时无法读取，请重试。',PERSONAL_PRICE_UNAVAILABLE:'本人报价暂时无法读取；公开参考价仍可查看。',MODEL_NOT_FOUND:'此模型暂不可访问。',MODELS_FORBIDDEN:'当前账户没有模型运营权限。',MODEL_METADATA_INCOMPLETE:'公开前请补全展示名称、家族和简介。',CATALOG_INVALID_REQUEST:'请检查筛选条件或元数据字段。',};return e instanceof ApiError?messages[e.code]||e.message:e instanceof Error?e.message:'读取失败，请重试。';}
export function readCatalog(client:ApiClient,query=''){return client.catalogRequest<CatalogPage>(catalogRoot+query)}
export function readCatalogDetail(client:ApiClient,id:string){return client.catalogRequest<CatalogDetail>(catalogRoot+'/detail?'+new URLSearchParams({model_id:id}))}
export function readPersonalPrice(client:ApiClient,id:string){return client.request<CatalogPersonal>(catalogRoot+'/personal-price?'+new URLSearchParams({model_id:id}))}
export async function useCatalogModel(client:ApiClient,id:string,navigate:(path:string)=>void,login:CatalogLoginRequired,isCurrent:()=>boolean=()=>true){
 if(!validModelID(id))throw new Error('无效的模型 ID。');const snap=client.getSnapshot();if(!snap.ready)throw new Error('正在确认登录状态，请稍后重试。');if(!snap.user){login(catalogSelectionPath(id));return}
 const epoch=client.getSessionGeneration(),route=catalogSelectionPath(id);const gate=await readAccessGate(client,route);
 if(!isCurrent())return;
 if(epoch!==client.getSessionGeneration()||!client.getSnapshot().user)throw new Error('登录状态已改变，请重新选择模型。');
 if(gate.stage!=='READY'){saveRouteIntent(route);navigate('/welcome');return;}
 const keys=await client.keys(1,1);if(!isCurrent())return;if(epoch!==client.getSessionGeneration()||!client.getSnapshot().user)throw new Error('登录状态已改变，请重新选择模型。');navigate(keys.total>0?catalogSelectionPath(id,false):'/keys?'+new URLSearchParams({model_id:id}).toString());
}
