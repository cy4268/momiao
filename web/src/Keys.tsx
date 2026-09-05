import { useState, useRef, useEffect, type FormEvent } from 'react';
import { ApiClient, ApiError, type Key, errorText } from './api';
import { Alert, Empty, Loading, Modal, Pager, date, number, useResource } from './ui';
const status = (key: Key) => ({ 1: '已启用', 2: '已停用', 3: '已过期', 4: '额度用尽' }[key.status] || '未知状态');
export function Keys({ client }: {
    client: ApiClient;
}) {
    const [page, setPage] = useState(1);
    const resource = useResource(() => client.keys(page), [client, page]);
    const [dialog, setDialog] = useState<{
        kind: 'create';
    } | {
        kind: 'reveal' | 'delete';
        key: Key;
    } | null>(null);
    const [notice, setNotice] = useState('');
    const [error, setError] = useState('');
    const [busy, setBusy] = useState(false);
    const locked = useRef(false);
    const reload = () => resource.reload();
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
 {notice && <p className="notice" role="status">{notice}</p>}{error && <Alert>{error}</Alert>}
 <section className="panel"><div className="section-heading"><h2>我的密钥</h2><button disabled={resource.loading || busy} onClick={reload}>刷新列表</button></div><p className="hint">额度按原生单位显示，不代表平台 Reserve 或 API Credit。密钥默认隐藏。</p>
 {resource.loading ? <Loading /> : resource.error ? <><Alert>{resource.error}</Alert><button onClick={reload}>重新加载</button></> : resource.data && <>{resource.data.items.length === 0 ? <Empty title="还没有 API 密钥">点击“创建 API 密钥”，为你的第一个应用开启连接。</Empty> : <div className="table-wrap" tabIndex={0} role="region" aria-label="API 密钥列表"><table><thead><tr><th>名称 / 密钥</th><th>状态</th><th>额度（原生单位）</th><th>创建 / 到期</th><th>操作</th></tr></thead><tbody>{resource.data.items.map(key => <tr key={key.id}><td><strong>{key.name || '未命名密钥'}</strong><code className="masked">{key.key || '••••••••••••'}</code></td><td><span className={`badge ${key.status === 1 ? 'active' : ''}`}>{status(key)}</span></td><td><span>剩余 {key.unlimited_quota ? '不限额' : number(key.remain_quota)}</span><small>已用 {number(key.used_quota)}</small></td><td><time>{date(key.created_time)}</time><small>{date(key.expired_time)}</small></td><td><div className="row-actions"><button disabled={busy} onClick={() => setDialog({ kind: 'reveal', key })}>查看密钥</button><button disabled={busy} onClick={() => void toggle(key)}>{key.status === 1 ? '停用' : '启用'}</button><button className="quiet" disabled={busy} onClick={() => setDialog({ kind: 'delete', key })}>删除</button></div></td></tr>)}</tbody></table></div>}<Pager page={page} total={resource.data.total} size={resource.data.page_size} onChange={setPage} disabled={busy}/></>}
 </section>{dialog?.kind === 'create' && <CreateKey client={client} onClose={() => setDialog(null)} onDone={() => { setDialog(null); setPage(1); reload(); setNotice('密钥已创建。在列表中选择“查看密钥”后再复制。'); }} onAmbiguous={reload}/>}
 {dialog?.kind === 'reveal' && <RevealKey client={client} token={dialog.key} onClose={() => setDialog(null)}/>}
 {dialog?.kind === 'delete' && <DeleteKey client={client} token={dialog.key} onClose={() => setDialog(null)} onDone={() => { setDialog(null); reload(); setNotice('密钥已删除。'); }} onAmbiguous={reload}/>}
 </>;
}
function CreateKey({ client, onClose, onDone, onAmbiguous }: {
    client: ApiClient;
    onClose: () => void;
    onDone: () => void;
    onAmbiguous: () => void;
}) {
    const [name, setName] = useState('');
    const [quota, setQuota] = useState('');
    const [unlimited, setUnlimited] = useState(false);
    const [days, setDays] = useState('30');
    const [error, setError] = useState('');
    const [busy, setBusy] = useState(false);
    const [uncertain, setUncertain] = useState(false);
    const lock = useRef(false);
    async function submit(e: FormEvent) { e.preventDefault(); if (lock.current || uncertain)
        return; const value = Number(quota); if (!name.trim() || new TextEncoder().encode(name.trim()).length > 50) {
        setError('请输入 1–50 字节的名称（中文通常占 3 字节）。');
        return;
    } if (!unlimited && (!quota || !Number.isSafeInteger(value) || value < 1 || value > 1000000000000)) {
        setError('请输入 1 至 1,000,000,000,000 的整数原生额度。');
        return;
    } lock.current = true; setBusy(true); setError(''); try {
        await client.request('/api/token/', 'POST', { name: name.trim(), remain_quota: unlimited ? 0 : value, unlimited_quota: unlimited, expired_time: days === 'never' ? -1 : Math.floor(Date.now() / 1000) + Number(days) * 86400, model_limits_enabled: false, model_limits: '', allow_ips: '', group: '', cross_group_retry: false });
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
    return <Modal title="创建 API 密钥" onClose={onClose} busy={busy}><form onSubmit={submit}><p className="hint">密钥可访问账户允许的模型。使用有限额度和有效期，方便独立管理每个应用。</p><label>密钥名称<input autoFocus value={name} onChange={e => setName(e.target.value)} required maxLength={50} placeholder="例如：我的应用"/></label><small>最长 50 字节；中文名称建议不超过 16 个字。</small><label>额度上限（原生单位）<input type="number" value={quota} disabled={unlimited} onChange={e => setQuota(e.target.value)} min="1" max="1000000000000" step="1" required={!unlimited} placeholder="输入整数额度"/></label><label className="checkbox"><input type="checkbox" checked={unlimited} onChange={e => setUnlimited(e.target.checked)}/>我明确选择不限额（仍受账户可用额度约束）</label><label>有效期<select value={days} onChange={e => setDays(e.target.value)}><option value="7">7 天</option><option value="30">30 天</option><option value="90">90 天</option><option value="never">永不过期</option></select></label>{error && <Alert>{error}</Alert>}{uncertain && <p>请关闭此窗口并核对列表后，再决定是否创建。</p>}<div className="dialog-actions"><button type="button" disabled={busy} onClick={onClose}>取消</button><button className="primary" type="submit" disabled={busy || uncertain}>{busy ? '正在创建…' : '确认创建'}</button></div></form></Modal>;
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
