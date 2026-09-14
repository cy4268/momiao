import { useState, useRef, useEffect, type FormEvent } from 'react';
import { ApiClient, ApiError, type Key, errorText } from './api';
import { Alert, Empty, Loading, Modal, Pager, date, number, useResource } from './ui';
import { Link, useLocation } from 'react-router-dom';
import { catalogSelectionPath, selectedModel } from './catalog-api';
import { keyPurposes, keyPurposeLabel, saveKeyPurpose, type KeyPurposeItem } from './key-purpose-api';
const status = (key: Key) => ({ 1: '已启用', 2: '已停用', 3: '已过期', 4: '额度用尽' }[key.status] || '未知状态');
type KeyBasicEdit = { id: number; name: string; remain_quota: number; unlimited_quota: boolean; expired_time: number };
async function saveKeyBasics(client: ApiClient, input: KeyBasicEdit): Promise<Key> {
    const value = await client.request<unknown>('/api/token/', 'PUT', input);
    const key = value as Key;
    if (!key || key.id !== input.id || key.name !== input.name || key.remain_quota !== input.remain_quota || key.unlimited_quota !== input.unlimited_quota || key.expired_time !== input.expired_time ||
        typeof key.key !== 'string' || !Number.isSafeInteger(key.status) || key.status < 1 || key.status > 4 || !Number.isSafeInteger(key.created_time) || key.created_time < 0 || !Number.isSafeInteger(key.used_quota) || key.used_quota < 0) {
        throw new ApiError('密钥编辑结果尚未确认，请刷新列表核对。', 0, 'KEY_EDIT_UNKNOWN', true);
    }
    return key;
}
export function Keys({ client }: {
    client: ApiClient;
}) {
    const modelID = selectedModel(useLocation().search);
    const [page, setPage] = useState(1);
    const resource = useResource(() => client.keys(page), [client, page]);
    const purposes = useResource(() => keyPurposes(client), [client]);
    const [purposeKey,setPurposeKey] = useState<Key | null>(null);
    const [dialog, setDialog] = useState<{
        kind: 'create';
    } | {
        kind: 'edit' | 'reveal' | 'delete';
        key: Key;
    } | null>(null);
    const [notice, setNotice] = useState('');
    const [error, setError] = useState('');
    const [busy, setBusy] = useState(false);
    const locked = useRef(false);
    const reload = () => { resource.reload(); purposes.reload(); };
    async function toggle(key: Key) { if (locked.current)
        return; locked.current = true; setBusy(true); setError(''); setNotice(''); try {
        await client.request('/api/token/?status_only=true', 'PUT', { id: key.id, status: key.status === 1 ? 2 : 1 });
        setNotice(key.status === 1 ? '密钥已停用。' : '密钥已启用。');
    }
    catch (e) {
        setError(errorText(e));
    }
    finally {
        locked.current = false;
        setBusy(false);
        reload();
    } }
    return <><header className="page-heading"><div><p className="eyebrow">ACCESS / API KEYS</p><h1>密钥管理</h1><p>为每个应用创建独立密钥，按需设置额度和有效期。</p></div><button className="primary" onClick={() => { setNotice(''); setDialog({ kind: 'create' }); }}>创建 API 密钥</button></header>
 {modelID && <section className="panel"><p>已选择模型：<code>{modelID}</code></p><p className="hint">创建密钥需要你主动确认；模型选择不会替你创建或修改密钥。</p><Link className="text-link" to={catalogSelectionPath(modelID, false)}>继续查看 API 接入设置 →</Link></section>}
 {notice && <p className="notice" role="status">{notice}</p>}{error && <Alert>{error}</Alert>}
 <section className="panel"><div className="section-heading"><h2>我的密钥</h2><button disabled={resource.loading || busy} onClick={reload}>刷新列表</button></div><p className="hint">额度按原生单位显示，不代表平台 Reserve 或 API Credit。密钥默认隐藏。</p>
 {resource.loading ? <Loading /> : resource.error ? <><Alert>{resource.error}</Alert><button onClick={reload}>重新加载</button></> : resource.data && <>{resource.data.items.length === 0 ? <Empty title="还没有 API 密钥">点击“创建 API 密钥”，为你的第一个应用开启连接。</Empty> : <div className="table-wrap" tabIndex={0} role="region" aria-label="API 密钥列表"><table><thead><tr><th>名称 / 密钥</th><th>用途</th><th>状态</th><th>额度（原生单位）</th><th>创建 / 到期</th><th>操作</th></tr></thead><tbody>{resource.data.items.map(key => <tr key={key.id}><td><strong>{key.name || '未命名密钥'}</strong><code className="masked">{key.key || '••••••••••••'}</code></td><td>{purposes.loading?'读取中…':purposes.error?'用途暂不可用':<>{keyPurposeLabel(purposes.data?.find(row=>row.token_id===String(key.id))?.purpose||'UNCLASSIFIED')}{purposes.data?.find(row=>row.token_id===String(key.id))?.sync_state==='PENDING'&&<small>用途同步中</small>}</>}</td><td><span className={`badge ${key.status === 1 ? 'active' : ''}`}>{status(key)}</span></td><td><span>剩余 {key.unlimited_quota ? '不限额' : number(key.remain_quota)}</span><small>已用 {number(key.used_quota)}</small></td><td><time>{date(key.created_time)}</time><small>{date(key.expired_time)}</small></td><td><div className="row-actions"><button disabled={busy} onClick={() => setDialog({ kind: 'edit', key })}>编辑基础设置</button><button disabled={busy||purposes.loading||!!purposes.error} onClick={()=>setPurposeKey(key)}>修改用途</button><button disabled={busy} onClick={() => setDialog({ kind: 'reveal', key })}>查看密钥</button><button disabled={busy} onClick={() => void toggle(key)}>{key.status === 1 ? '停用' : '启用'}</button><button className="quiet" disabled={busy} onClick={() => setDialog({ kind: 'delete', key })}>删除</button></div></td></tr>)}</tbody></table></div>}<Pager page={page} total={resource.data.total} size={resource.data.page_size} onChange={setPage} disabled={busy}/></>}
 </section>{dialog?.kind === 'create' && <CreateKey client={client} onClose={() => setDialog(null)} onDone={() => { setDialog(null); setPage(1); reload(); setNotice('密钥已创建。在列表中选择“查看密钥”后再复制。'); }} onAmbiguous={reload}/>}
 {dialog?.kind === 'edit' && <EditKey client={client} token={dialog.key} onClose={() => setDialog(null)} onDone={() => { setDialog(null); reload(); setNotice('密钥基础设置已更新。'); }} onAmbiguous={reload}/>}
 {dialog?.kind === 'reveal' && <RevealKey client={client} token={dialog.key} onClose={() => setDialog(null)}/>}
 {dialog?.kind === 'delete' && <DeleteKey client={client} token={dialog.key} onClose={() => setDialog(null)} onDone={() => { setDialog(null); reload(); setNotice('密钥已删除。'); }} onAmbiguous={reload}/>}
 {purposeKey&&<PurposeDialog client={client} token={purposeKey} current={purposes.data?.find(row=>row.token_id===String(purposeKey.id))} onClose={()=>setPurposeKey(null)} onDone={()=>{setPurposeKey(null);reload();setNotice('用途已提交，实际生效状态请查看列表。');}}/>}
 </>;
}
function CreateKey({ client, onClose, onDone, onAmbiguous }: {
    client: ApiClient;
    onClose: () => void;
    onDone: () => void;
    onAmbiguous: () => void;
}) {
    const [name, setName] = useState('');
    const [purpose,setPurpose] = useState<''|'GENERAL'|'ROLEPLAY'>('');
    const [createdID,setCreatedID] = useState('');
    const purposeOperation = useRef('');
    const [quota, setQuota] = useState('');
    const [unlimited, setUnlimited] = useState(false);
    const [days, setDays] = useState('30');
    const [error, setError] = useState('');
    const [busy, setBusy] = useState(false);
    const [uncertain, setUncertain] = useState(false);
    const lock = useRef(false);
    async function submit(e: FormEvent) { e.preventDefault(); if (lock.current || uncertain)
        return; const value = Number(quota); if (!purpose) { setError('请选择 General 或 RP 用途。'); return; } if (!name.trim() || new TextEncoder().encode(name.trim()).length > 50) {
        setError('请输入 1–50 字节的名称（中文通常占 3 字节）。');
        return;
    } if (!unlimited && (!quota || !Number.isSafeInteger(value) || value < 1 || value > 1000000000000)) {
        setError('请输入 1 至 1,000,000,000,000 的整数原生额度。');
        return;
    } lock.current = true; setBusy(true); setError(''); try {
        if (createdID) return;
        const created = await client.request<{id:string}>('/api/token/', 'POST', { name: name.trim(), remain_quota: unlimited ? 0 : value, unlimited_quota: unlimited, expired_time: days === 'never' ? -1 : Math.floor(Date.now() / 1000) + Number(days) * 86400, model_limits_enabled: false, model_limits: '', allow_ips: '', group: '', cross_group_retry: false });
        if (!created || typeof created.id !== 'string' || !/^[1-9][0-9]{0,18}$/.test(created.id)) throw new ApiError('密钥创建结果尚未确认，请关闭窗口核对列表。',0,'KEY_CREATE_UNKNOWN',true);
        setCreatedID(created.id); setUncertain(true); onAmbiguous();
        purposeOperation.current=crypto.randomUUID();
        await saveKeyPurpose(client,created.id,purpose,purposeOperation.current);
        onDone();
    }
    catch (e) {
        setError(errorText(e));
        if (e instanceof ApiError && e.uncertain) {
            setUncertain(true);
            onAmbiguous();
        }
    }
    finally {
        lock.current = false;
        setBusy(false);
    } }
    return <Modal title="创建 API 密钥" onClose={onClose} busy={busy}><form onSubmit={submit}><p className="hint">密钥可访问账户允许的模型。使用有限额度和有效期，方便独立管理每个应用。</p><label>用途<select value={purpose} onChange={e=>setPurpose(e.target.value as typeof purpose)} required disabled={!!createdID}><option value="">请选择用途</option><option value="GENERAL">General · 通用</option><option value="ROLEPLAY">RP · 角色扮演</option></select></label><p className="hint">用途变更仅影响后续请求；旧请求不会重新计入 RP 排行。</p><label>密钥名称<input autoFocus value={name} onChange={e => setName(e.target.value)} required maxLength={50} placeholder="例如：我的应用"/></label><small>最长 50 字节；中文名称建议不超过 16 个字。</small><label>额度上限（原生单位）<input type="number" value={quota} disabled={unlimited} onChange={e => setQuota(e.target.value)} min="1" max="1000000000000" step="1" required={!unlimited} placeholder="输入整数额度"/></label><label className="checkbox"><input type="checkbox" checked={unlimited} onChange={e => setUnlimited(e.target.checked)}/>我明确选择不限额（仍受账户可用额度约束）</label><label>有效期<select value={days} onChange={e => setDays(e.target.value)}><option value="7">7 天</option><option value="30">30 天</option><option value="90">90 天</option><option value="never">永不过期</option></select></label>{error && <Alert>{error}</Alert>}{createdID?<p role="status">密钥 {createdID} 已创建。用途尚需核对，请关闭窗口刷新列表，再通过“修改用途”继续；无需重新创建密钥。</p>:uncertain&&<p>请关闭此窗口并核对列表后，再决定是否创建。</p>}<div className="dialog-actions"><button type="button" disabled={busy} onClick={onClose}>取消</button><button className="primary" type="submit" disabled={busy || uncertain}>{busy ? '正在创建…' : '确认创建'}</button></div></form></Modal>;
}
function EditKey({ client, token, onClose, onDone, onAmbiguous }: {
    client: ApiClient;
    token: Key;
    onClose: () => void;
    onDone: () => void;
    onAmbiguous: () => void;
}) {
    const [name, setName] = useState(token.name);
    const [quota, setQuota] = useState(String(token.remain_quota));
    const [unlimited, setUnlimited] = useState(token.unlimited_quota);
    const [expiry, setExpiry] = useState<'current' | '7' | '30' | '90' | 'never'>(token.expired_time === -1 ? 'never' : 'current');
    const [error, setError] = useState('');
    const [busy, setBusy] = useState(false);
    const [uncertain, setUncertain] = useState(false);
    const lock = useRef(false);
    async function submit(event: FormEvent) {
        event.preventDefault();
        if (lock.current || uncertain) return;
        const cleanName = name.trim(), quotaValue = Number(quota);
        const expiredTime = expiry === 'never' ? -1 : expiry === 'current' ? token.expired_time : Math.floor(Date.now() / 1000) + Number(expiry) * 86400;
        if (!cleanName || new TextEncoder().encode(cleanName).length > 50) {
            setError('请输入 1–50 字节的名称（中文通常占 3 字节）。');
            return;
        }
        if (!unlimited && (!quota || !Number.isSafeInteger(quotaValue) || quotaValue < 1 || quotaValue > 2147483647)) {
            setError('请输入 1 至 2,147,483,647 的整数原生额度。');
            return;
        }
        if (!(expiredTime === -1 || Number.isSafeInteger(expiredTime) && expiredTime >= 0)) {
            setError('有效期超出安全范围，请重新选择。');
            return;
        }
        lock.current = true;
        setBusy(true);
        setError('');
        try {
            await saveKeyBasics(client, { id: token.id, name: cleanName, remain_quota: unlimited ? 0 : quotaValue, unlimited_quota: unlimited, expired_time: expiredTime });
            onDone();
        } catch (e) {
            setError(errorText(e));
            if (e instanceof ApiError && e.uncertain) {
                setUncertain(true);
                onAmbiguous();
            }
        } finally {
            lock.current = false;
            setBusy(false);
        }
    }
    return <Modal title={`编辑基础设置 · ${token.name}`} onClose={onClose} busy={busy}><form noValidate onSubmit={event => void submit(event)}><p className="hint">只修改名称、额度与有效期；状态、用途和高级访问范围保持不变。</p><label>密钥名称<input autoFocus value={name} onChange={event => setName(event.target.value)} disabled={busy || uncertain} maxLength={50}/></label><small>最长 50 字节；中文名称建议不超过 16 个字。</small><label>额度上限（原生单位）<input type="number" value={quota} disabled={busy || uncertain || unlimited} onChange={event => setQuota(event.target.value)} min="1" max="2147483647" step="1"/></label><label className="checkbox"><input type="checkbox" checked={unlimited} disabled={busy || uncertain} onChange={event => setUnlimited(event.target.checked)}/>我明确选择不限额（仍受账户可用额度约束）</label><label>有效期<select value={expiry} disabled={busy || uncertain} onChange={event => setExpiry(event.target.value as typeof expiry)}>{token.expired_time !== -1 && <option value="current">保持当前设置（{date(token.expired_time)}）</option>}<option value="7">从现在起 7 天</option><option value="30">从现在起 30 天</option><option value="90">从现在起 90 天</option><option value="never">永不过期</option></select></label>{error && <Alert>{error}</Alert>}{uncertain && <p>结果尚未确认。请关闭窗口并核对刷新后的列表，勿重复提交。</p>}<div className="dialog-actions"><button type="button" disabled={busy} onClick={onClose}>关闭</button><button className="primary" type="submit" disabled={busy || uncertain}>{busy ? '正在保存…' : '保存基础设置'}</button></div></form></Modal>;
}
function PurposeDialog({client,token,current,onClose,onDone}:{client:ApiClient;token:Key;current?:KeyPurposeItem;onClose:()=>void;onDone:()=>void}) {
    const [purpose,setPurpose]=useState<'GENERAL'|'ROLEPLAY'>(current?.purpose==='ROLEPLAY'?'ROLEPLAY':'GENERAL');
    const [busy,setBusy]=useState(false),[error,setError]=useState(''),[uncertain,setUncertain]=useState(false);
    const lock=useRef(false),operation=useRef('');
    async function submit(event:FormEvent){
        event.preventDefault();if(lock.current||uncertain||current?.sync_state==='PENDING')return;
        lock.current=true;setBusy(true);setError('');
        try{operation.current ||= crypto.randomUUID();await saveKeyPurpose(client,String(token.id),purpose,operation.current,current?.version||'0');onDone();}
        catch(e){setError(errorText(e));setUncertain(true);}
        finally{lock.current=false;setBusy(false);}
    }
    return <Modal title={`密钥用途 · ${token.name}`} onClose={onClose} busy={busy}><form onSubmit={event=>void submit(event)}><p>当前用途：{keyPurposeLabel(current?.purpose||'UNCLASSIFIED')}。变更只影响生效后的请求，已记录的用量不会改写。</p>{current?.sync_state==='PENDING'&&<Alert>上一项用途正在同步，请关闭窗口并刷新列表，生效前不提交新变更。</Alert>}<label>新用途<select value={purpose} disabled={busy||uncertain||current?.sync_state==='PENDING'} onChange={event=>setPurpose(event.target.value as typeof purpose)}><option value="GENERAL">General · 通用</option><option value="ROLEPLAY">RP · 角色扮演</option></select></label>{error&&<Alert>{error}</Alert>}{uncertain&&<p>请关闭窗口并刷新列表，核对原操作结果后再修改。</p>}<div className="dialog-actions"><button type="button" disabled={busy} onClick={onClose}>关闭</button><button className="primary" type="submit" disabled={busy||uncertain||current?.sync_state==='PENDING'}>{busy?'正在提交…':'保存用途'}</button></div></form></Modal>;
}
function RevealKey({ client, token, onClose }: {
    client: ApiClient;
    token: Key;
    onClose: () => void;
}) {
    const [secret, setSecret] = useState('');
    const [error, setError] = useState('');
    const [copied, setCopied] = useState(false);
    useEffect(() => { let active = true; client.request<{
        key: string;
    }>(`/api/token/${token.id}/key`, 'POST').then(data => { if (!data || typeof data.key !== 'string' || !data.key)
        throw new Error('密钥响应格式异常。'); if (active)
        setSecret(data.key.startsWith('sk-') ? data.key : `sk-${data.key}`); }).catch(e => { if (active)
        setError(errorText(e)); }); return () => { active = false; }; }, [client, token.id]);
    async function copy() { try {
        await navigator.clipboard.writeText(secret);
        setCopied(true);
    }
    catch {
        setError('剪贴板访问失败，请手动选择并复制密钥。');
    } }
    return <Modal title={`查看密钥 · ${token.name}`} onClose={onClose}><p>仅在可信应用中使用。关闭窗口或离开本页后，本页将清除明文；系统剪贴板需自行清理。</p>{error && <Alert>{error}</Alert>}{secret ? <><label>完整 API 密钥<textarea className="secret" readOnly value={secret} rows={3} onFocus={e => e.target.select()}/></label><div className="dialog-actions"><button onClick={onClose}>关闭</button><button className="primary" onClick={() => void copy()}>{copied ? '已复制' : '复制密钥'}</button></div></> : !error && <Loading />}</Modal>;
}
function DeleteKey({ client, token, onClose, onDone, onAmbiguous }: {
    client: ApiClient;
    token: Key;
    onClose: () => void;
    onDone: () => void;
    onAmbiguous: () => void;
}) {
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [uncertain, setUncertain] = useState(false);
    const lock = useRef(false);
    async function remove() { if (lock.current || uncertain)
        return; lock.current = true; setBusy(true); try {
        await client.request(`/api/token/${token.id}`, 'DELETE');
        onDone();
    }
    catch (e) {
        setError(errorText(e));
        if (e instanceof ApiError && e.uncertain) {
            setUncertain(true);
            onAmbiguous();
        }
    }
    finally {
        lock.current = false;
        setBusy(false);
    } }
    return <Modal title="删除这枚密钥？" onClose={onClose} busy={busy}><p>将永久删除 <strong>{token.name}</strong>。使用它的应用将失去访问权限，这项操作不可撤销。</p>{error && <Alert>{error}</Alert>}{uncertain && <p>请关闭窗口并刷新列表，核对是否已删除。</p>}<div className="dialog-actions"><button disabled={busy} onClick={onClose}>保留密钥</button><button className="primary" disabled={busy || uncertain} onClick={() => void remove()}>{busy ? '正在删除…' : '确认删除'}</button></div></Modal>;
}
