import {useEffect,useRef,useState,useSyncExternalStore} from 'react';
import {ApiClient} from './api';
import {Alert,Modal} from './ui';
import {assetUrl} from './game-hall-assets';
import {catalogError,readFamilyCovers,saveFamilyCover,uploadFamilyCover,validCatalogCoverSrc,type CatalogCoverImage,type CatalogCoverUploadResult,type CatalogFamilyCover,type CatalogFamilyCoverPage} from './catalog-api';

const names:Record<string,string>={gpt:'GPT',claude:'Claude',gemini:'Gemini',deepseek:'DeepSeek',glm:'GLM',kimi:'Kimi',ernie:'ERNIE / 文心',qwen:'Qwen / 千问',grok:'Grok',other:'其他家族'};
export function OpsCatalogCovers({client,initialFamily,onClose,onChanged}:{client:ApiClient;initialFamily?:string;onClose:()=>void;onChanged:(cover:CatalogFamilyCover)=>void}){
 useSyncExternalStore(client.subscribe,client.getSnapshot);const epoch=client.getSessionGeneration();
 const [page,setPage]=useState<CatalogFamilyCoverPage>();const [family,setFamily]=useState(initialFamily||'gpt');const [file,setFile]=useState<File>();
 const [alt,setAlt]=useState('');const [rights,setRights]=useState('ORIGINAL_GENERATED');const [note,setNote]=useState('');
 const [candidate,setCandidate]=useState<CatalogCoverUploadResult>();const [busy,setBusy]=useState(false);const [error,setError]=useState('');const [message,setMessage]=useState('');const lock=useRef(false);
 useEffect(()=>{let active=true;setPage(undefined);setCandidate(undefined);readFamilyCovers(client).then(p=>{if(active)setPage(p)}).catch(e=>{if(active)setError(catalogError(e))});return()=>{active=false}},[client,epoch]);
 async function run(task:()=>Promise<void>){if(lock.current)return;lock.current=true;setBusy(true);setError('');setMessage('');try{await task()}catch(e){setError(catalogError(e))}finally{lock.current=false;setBusy(false)}}
 async function upload(){if(!file||!file.size||file.size>8*1024*1024)throw new Error('请选择不超过 8 MiB 的图片。');if(!alt.trim()||!note.trim())throw new Error('请填写图片替代文字和素材来源说明。');setCandidate(await uploadFamilyCover(client,{family,alt:alt.trim(),rights_status:rights,rights_note:note.trim()},file))}
 async function save(asset:string){const cover=await saveFamilyCover(client,family,asset);setPage(old=>old?{...old,items:old.items.map(c=>c.family===family?cover:c)}:old);setCandidate(undefined);onChanged(cover);setMessage('封面已保存')}
 function image(value:CatalogCoverImage|null|undefined){return value&&validCatalogCoverSrc(value.src)?<img src={assetUrl(value.src)} alt={value.alt} width={value.width||undefined} height={value.height||undefined} style={{objectPosition:value.focal_point.map(n=>n*100+'%').join(' ')}}/>:<p>此家族尚无默认人物图</p>}
 return <Modal title="家族封面管理" onClose={onClose} busy={busy}><div className="catalog-cover-manager">
  {error&&<Alert>{error}</Alert>}{message&&<p role="status">{message}</p>}
  <label>封面家族<select disabled={!page||busy} value={family} onChange={e=>{setFamily(e.target.value);setFile(undefined);setCandidate(undefined);setAlt('');setNote('');setMessage('');setError('')}}>{page?.items.map(c=><option key={c.family} value={c.family}>{names[c.family]}</option>)}</select></label>
  {page&&<><p>同一家族的目录与详情共用这张封面，保存后立即生效。</p><div className="catalog-cover-columns"><section><h3>当前封面</h3><div className="catalog-cover-art">{image(page.items.find(c=>c.family===family)?.image)}</div><button disabled={busy} onClick={()=>void run(()=>save(''))}>恢复默认封面</button></section>
  <section><h3>上传图片</h3><p>上传后图片即具有公开地址；保存封面后才绑定到目录。</p>{!page.upload_enabled&&<p>上传尚未配置；仍可恢复默认封面。</p>}
  <fieldset disabled={busy||!page.upload_enabled}><legend>新图片</legend><label>上传图片<input key={family} type="file" accept="image/png,image/jpeg,image/webp" onChange={e=>{setFile(e.target.files?.[0]);setCandidate(undefined)}}/></label><p className="hint">静态 PNG / JPEG / WebP，最多 8 MiB、每边 8192 px、总像素 16,777,216；保留原图。</p>
  <label>图片替代文字<input value={alt} maxLength={120} onChange={e=>{setAlt(e.target.value);setCandidate(undefined)}}/></label><label>素材来源<select value={rights} onChange={e=>{setRights(e.target.value);setCandidate(undefined)}}><option value="ORIGINAL_GENERATED">生成原创</option><option value="ORIGINAL_PLATFORM">平台原创</option><option value="LICENSED_OR_APPROVED">已获许可</option></select></label><label>素材来源说明<textarea value={note} maxLength={500} onChange={e=>{setNote(e.target.value);setCandidate(undefined)}}/></label><button onClick={()=>void run(upload)}>{busy?'处理中…':'上传候选图'}</button></fieldset>
  {candidate&&<section><h4>{candidate.reused?'沿用已登记图片':'候选图片'}</h4><div className="catalog-cover-art">{image(candidate.image)}</div><p>{candidate.rights_note}</p><button className="primary" disabled={busy} onClick={()=>void run(()=>save(candidate.asset_id))}>保存封面</button></section>}
  </section></div></>}
 </div></Modal>;
}
