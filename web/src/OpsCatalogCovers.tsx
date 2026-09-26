import {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {ApiClient, ApiError} from './api';
import {Alert, Modal} from './ui';
import {assetUrl} from './game-hall-assets';
import {catalogError, catalogTime, executeFamilyCover, prepareFamilyCover, readFamilyCovers, uploadFamilyCover, validCatalogCoverSrc, type CatalogCoverImage, type CatalogCoverUploadCommand, type CatalogCoverUploadResult, type CatalogFamilyCover, type CatalogFamilyCoverCommand, type CatalogFamilyCoverPage, type CatalogFamilyCoverPreview, type CatalogFamilyCoverResult} from './catalog-api';

type Pending={command:CatalogFamilyCoverCommand;preview:CatalogFamilyCoverPreview;uncertain:boolean};
const names:Record<string,string>={gpt:'GPT',claude:'Claude',gemini:'Gemini',deepseek:'DeepSeek',glm:'GLM',kimi:'Kimi',ernie:'ERNIE / 文心',qwen:'Qwen / 千问',grok:'Grok',other:'其他家族'};

export function OpsCatalogCovers({client,initialFamily,onClose,onChanged}:{client:ApiClient;initialFamily?:string;onClose:()=>void;onChanged:(cover:CatalogFamilyCover)=>void}){
    useSyncExternalStore(client.subscribe,client.getSnapshot);
    const epoch=client.getSessionGeneration();
    const [page,setPage]=useState<CatalogFamilyCoverPage>();
    const [family,setFamily]=useState(initialFamily||'gpt');
    const [file,setFile]=useState<File>();
    const [alt,setAlt]=useState('');const [rights,setRights]=useState('ORIGINAL_GENERATED');const [note,setNote]=useState('');const [reason,setReason]=useState('');
    const [attempt,setAttempt]=useState<{command:CatalogCoverUploadCommand;file:File}>();
    const [candidate,setCandidate]=useState<CatalogCoverUploadResult>();
    const [pending,setPending]=useState<Pending>();
    const [receipt,setReceipt]=useState<{action:string;result:CatalogFamilyCoverResult}>();
    const [needsRead,setNeedsRead]=useState(false);const [error,setError]=useState('');const [busy,setBusy]=useState('read');
    const [loaded,setLoaded]=useState('');const [imageError,setImageError]=useState(false);const [imageRetry,setImageRetry]=useState(0);
    const generation=useRef(0);const lock=useRef(false);
    useEffect(()=>{
        const n=++generation.current;setPage(undefined);setPending(undefined);setCandidate(undefined);setAttempt(undefined);setReceipt(undefined);setNeedsRead(false);setLoaded('');setError('');setBusy('read');lock.current=true;
        readFamilyCovers(client).then(next=>{if(n===generation.current)setPage(next)}).catch(e=>{if(n===generation.current)setError(catalogError(e))}).finally(()=>{if(n===generation.current){lock.current=false;setBusy('')}});
        return()=>{generation.current++};
    },[client,epoch]);
    const cover=page?.items.find(c=>c.family===family);
    const canWrite=!!page?.principal.permissions.includes('models.write');const canPublish=!!page?.principal.permissions.includes('models.publish');
    const previewImage=pending?.preview.after.image||(!pending?candidate?.image:null);
    const src=previewImage&&validCatalogCoverSrc(previewImage.src)?previewImage.src:'';
    const previewReady=!previewImage||!!src&&loaded===src&&!imageError;
    async function run(mode:string,task:(current:()=>boolean)=>Promise<void>){
        if(lock.current)return;lock.current=true;const n=++generation.current;const session=client.getSessionGeneration();
        const current=()=>generation.current===n&&session===client.getSessionGeneration();setBusy(mode);setError('');
        try{await task(current)}catch(e){if(current())setError(catalogError(e))}finally{if(current()){lock.current=false;setBusy('')}}
    }
    function clearCandidate(){setAttempt(undefined);setCandidate(undefined);setPending(undefined);setLoaded('');setImageError(false)}
    function changeFamily(value:string){generation.current++;lock.current=false;setBusy('');setFamily(value);setFile(undefined);setAlt('');setNote('');setReason('');clearCandidate();setReceipt(undefined);setError('');setNeedsRead(false)}
    async function refresh(current:()=>boolean){const next=await readFamilyCovers(client);if(current()){setPage(next);setNeedsRead(false)}}
    async function upload(current:()=>boolean){
        if(!page||!file)throw new Error('请选择一张图片。');
        if(!file.size||file.size>8*1024*1024)throw new Error('图片大小须在 1 字节至 8 MiB 之间。');
        if(!alt.trim()||!note.trim()||!reason.trim())throw new Error('请填写图片替代文字、素材来源说明和封面变更原因。');
        const op=attempt||{file,command:{operation_id:crypto.randomUUID(),authz_epoch:page.principal.authz_epoch,family,alt:alt.trim(),rights_status:rights,rights_note:note.trim(),reason:reason.trim()}};
        setAttempt(op);const result=await uploadFamilyCover(client,op.command,op.file,current);
        if(current()){setCandidate(result);setAttempt(undefined);setLoaded('');setImageError(false)}
    }
    async function prepare(action:'SET'|'RESET',current:()=>boolean){
        if(!page||!cover||!reason.trim())throw new Error('请填写封面变更原因。');
        const command:CatalogFamilyCoverCommand={operation_id:crypto.randomUUID(),authz_epoch:page.principal.authz_epoch,action,family,asset_id:action==='SET'?candidate?.asset_id||'':'',expected_version:cover.version,reason:reason.trim()};
        const preview=await prepareFamilyCover(client,command);
        if(current()){setPending({command,preview,uncertain:false});setLoaded('');setImageError(false)}
    }
    async function execute(current:()=>boolean){
        if(!pending)return;const op=pending;let result:CatalogFamilyCoverResult;
        try{result=await executeFamilyCover(client,op.command,op.preview.preview_id)}catch(e){if(current()&&e instanceof ApiError&&e.uncertain)setPending({...op,uncertain:true});throw e}
        if(!current())return;
        setReceipt({action:op.command.action,result});setPending(undefined);setCandidate(undefined);setAttempt(undefined);setLoaded('');setNeedsRead(true);
        setPage(old=>old?{...old,items:old.items.map(c=>c.family===result.cover.family?result.cover:c)}:old);onChanged(result.cover);
        try{await refresh(current)}catch{if(current())setError('封面已确认，最新状态读取失败。已保留确认回执，请重新读取封面。')}
    }
    function image(image:CatalogCoverImage|null|undefined,isPreview=false){
        return image&&validCatalogCoverSrc(image.src)?<img key={image.src+':'+imageRetry} src={assetUrl(image.src)} alt={image.alt} width={image.width||undefined} height={image.height||undefined} style={{objectPosition:image.focal_point.map(n=>n*100+'%').join(' ')}} onLoad={()=>{if(isPreview){setLoaded(image.src);setImageError(false)}}} onError={()=>{if(isPreview){setLoaded('');setImageError(true)}}}/>:<p className="hint">此家族尚无默认人物图</p>;
    }
    const formDisabled=!canWrite||!page?.upload_enabled||!!busy||!!pending||needsRead||!!attempt;
    return <Modal title="家族封面管理" onClose={onClose} busy={busy==='execute'||!!pending?.uncertain}>
        <div className="catalog-cover-manager">
            <div className="catalog-cover-toolbar"><label>封面家族<select value={family} disabled={!page||busy==='execute'||!!pending?.uncertain} onChange={e=>changeFamily(e.target.value)}>{(page?.items||[]).map(c=><option value={c.family} key={c.family}>{names[c.family]||c.family}</option>)}</select></label><button disabled={!!busy||!!pending} onClick={()=>void run('read',refresh)}>重新读取封面</button></div>
            {receipt&&<section className="catalog-operation-receipt" role="status"><strong>已确认：{receipt.action==='SET'?'家族封面已更新':'已恢复默认封面'}</strong><p>{names[receipt.result.cover.family]} · 版本 {receipt.result.cover.version}</p><code>{receipt.result.operation_id}</code></section>}
            {error&&<Alert>{error}</Alert>}
            {!page&&busy==='read'&&<p role="status">正在读取家族封面…</p>}
            {page&&<>
                <p className="hint">同一家族的目录卡片及所有模型详情共用这张封面。用户刷新或重新进入页面后读取最新版本。</p>
                <div className="catalog-cover-columns"><section className="catalog-cover-current"><h3>当前封面 <small>版本 {cover?.version}</small></h3><div className="catalog-cover-art">{image(cover?.image)}</div></section>
                <section className="catalog-cover-edit">
                    {pending?<>
                        <h3>{pending.command.action==='SET'?'确认新封面':'确认恢复默认'}</h3><p>影响 {names[family]} 家族的全部模型，不修改价格与接入配置。</p>
                        <div className="catalog-cover-art">{image(pending.preview.after.image,true)}</div>
                        {previewImage&&!previewReady&&<p role="status">{imageError?'CDN 图片未载入，当前绑定保持不变。':'正在从 CDN 验证图片…'}</p>}
                        {imageError&&<button onClick={()=>{setImageError(false);setImageRetry(n=>n+1)}}>重新加载预览图片</button>}
                        <p>原因：{pending.command.reason}</p><p className="hint">预览有效至 {catalogTime(pending.preview.expires_at)}</p>
                        {pending.uncertain&&<p role="status">确认结果尚未返回；仅使用原操作编号重试。</p>}
                        <div className="catalog-actions"><button className="primary" disabled={!!busy||!canPublish||!previewReady} onClick={()=>void run('execute',execute)}>{pending.uncertain?'使用原操作编号重试':'确认家族封面'}</button><button disabled={!!busy||pending.uncertain} onClick={()=>setPending(undefined)}>返回封面编辑</button></div>
                    </>:<>
                        <h3>上传与预览</h3><p className="catalog-cover-disclosure">素材桶公开可访问：上传后文件即拥有公共地址；“确认”决定是否在目录展示，并非保密开关。</p>
                        {!page.upload_enabled&&<p role="status">上传尚未配置；当前封面与恢复默认仍可用。</p>}
                        <fieldset disabled={formDisabled}><legend>新图片</legend>
                            <label>上传图片<input key={family} type="file" accept="image/png,image/jpeg,image/webp" onChange={e=>{setFile(e.target.files?.[0]);clearCandidate()}}/></label><small>静态 PNG / JPEG / WebP；≤ 8 MiB，每边 ≤ 8192 px，总像素 ≤ 16,777,216。保留原图，不转码。</small>
                            {file&&<p>{file.name} · {(file.size/1024/1024).toFixed(2)} MiB</p>}
                            <label>图片替代文字<input value={alt} maxLength={120} onChange={e=>{setAlt(e.target.value);clearCandidate()}}/></label>
                            <label>素材来源<select value={rights} onChange={e=>{setRights(e.target.value);clearCandidate()}}><option value="ORIGINAL_GENERATED">生成原创</option><option value="ORIGINAL_PLATFORM">平台原创</option><option value="LICENSED_OR_APPROVED">已获许可</option></select></label>
                            <label>素材来源说明<textarea value={note} maxLength={500} rows={2} onChange={e=>{setNote(e.target.value);clearCandidate()}}/></label>
                        </fieldset>
                        <label>封面变更原因<textarea value={reason} maxLength={500} rows={2} disabled={!!busy||!!attempt||needsRead||!canPublish&&!canWrite} onChange={e=>setReason(e.target.value)}/></label>
                        <button disabled={!canWrite||!page.upload_enabled||!!busy||needsRead} onClick={()=>void run('upload',upload)}>{busy==='upload'?'上传中…':attempt?'使用原上传编号重试':'上传候选图'}</button>
                        {attempt&&<><code className="catalog-cover-operation">{attempt.command.operation_id}</code><button disabled={!!busy} onClick={clearCandidate}>放弃候选操作</button></>}
                        {candidate&&<section><h4>{candidate.reused?'图片已登记，沿用原有说明':'候选图已登记，尚未公开绑定'}</h4><div className="catalog-cover-art">{image(candidate.image,true)}</div><p>{candidate.rights_note}</p>{imageError&&<p role="alert">CDN 图片未载入，请重新加载预览后确认。</p>}<button disabled={!!busy||!canPublish||needsRead} onClick={()=>void run('prepare',c=>prepare('SET',c))}>预览家族封面</button></section>}
                        <button disabled={!!busy||!canPublish||needsRead} onClick={()=>void run('prepare',c=>prepare('RESET',c))}>恢复默认封面</button>
                    </>}
                </section></div>
            </>}
        </div>
    </Modal>;
}
