import { useEffect, useRef, useState, type FormEvent } from 'react';
import { ApiClient, ApiError, type Page, type User } from './api';
import { Alert, Empty, Loading, Modal, Pager, date, number } from './ui';
interface Channel { id: number; name: string; type: number; status: number; base_url?: string; models: string; group: string; model_mapping?: string; response_time?: number; test_time?: number }
const channelError = (e: unknown) => e instanceof ApiError && e.uncertain ? '操作结果尚未确认。请关闭窗口并刷新列表核对，勿重复提交。' : e instanceof ApiError && e.status === 403 ? '当前账户没有此操作权限。' : '渠道请求未完成，请检查输入或刷新后重试。';
export function Channels({ client, user }: { client: ApiClient; user: User }) {
    const [page, setPage] = useState(1);
    const [data, setData] = useState<Page<Channel>>();
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [notice, setNotice] = useState('');
    const [busy, setBusy] = useState(false);
    const [uncertain, setUncertain] = useState(false);
    const [dialog, setDialog] = useState<{ kind: 'edit' | 'status'; channel: Channel } | { kind: 'create' } | null>(null);
    const [version, setVersion] = useState(0);
    const lock = useRef(false);
    const mounted = useRef(true);
    const refresh = () => { setDialog(null); setVersion(v => v + 1); };
    useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
    useEffect(() => {
        if (user.role < 10) return;
        let current = true; setLoading(true); setError('');
        client.page<Channel>(`/api/channel/?p=${page}&page_size=10&id_sort=true`).then(result => {
            if (!current) return;
            if (result.items.some(c => !c || !Number.isInteger(c.id) || typeof c.name !== 'string' || !Number.isInteger(c.type) || !Number.isInteger(c.status) || typeof c.models !== 'string' || typeof c.group !== 'string')) throw new ApiError('Bad channel shape');
            setData(result); setUncertain(false);
        }).catch(e => { if (current) { setData(undefined); setError(channelError(e)); } }).finally(() => { if (current) setLoading(false); });
        return () => { current = false; };
    }, [client, page, version, user.role]);
    async function mutate(body: unknown, method: string, path = '/api/channel/') {
        if (lock.current || uncertain) return false;
        lock.current = true; setBusy(true); setError(''); setNotice('');
        try { await client.request(path, method, body); if (mounted.current) { setDialog(null); setNotice('操作已提交，请以刷新后的渠道列表为准。'); setVersion(v => v + 1); } return true; }
        catch (e) { if (mounted.current) { setError(channelError(e)); if (e instanceof ApiError && e.uncertain) setUncertain(true); } return false; }
        finally { lock.current = false; if (mounted.current) setBusy(false); }
    }
    if (user.role < 10) return <Empty title="需要管理员权限">当前账户可继续使用模型目录与文本测试。</Empty>;
    const blocked = loading || busy || uncertain || !data;
    return <><header className="page-heading"><div><p className="eyebrow">OPERATE / CHANNELS</p><h1>渠道管理</h1><p>查看原生渠道与启停状态；基础编辑仅支持类型 1。</p></div>{user.role === 100 && <button className="primary" disabled={blocked} onClick={() => setDialog({ kind: 'create' })}>新建渠道</button>}</header>
    {notice && <p className="notice" role="status">{notice}</p>}{error && !dialog && <Alert>{error}</Alert>}<section className="panel"><div className="section-heading"><h2>已配置渠道</h2><button disabled={loading || busy} onClick={refresh}>刷新列表</button></div><p className="hint">密钥不会读取或展示。历史测试耗时不是实时可用性或 SLA；高级设置保留在原生配置中。</p>
    {loading ? <Loading/> : !data ? <><Alert>{error || '列表暂未返回。'}</Alert><button onClick={refresh}>重新加载</button></> : <>{!data.items.length ? <Empty title="尚未配置渠道">超级管理员可新建基础兼容渠道；配置完成后再到模型目录查看。</Empty> : <div className="table-wrap" tabIndex={0} role="region" aria-label="渠道列表"><table><thead><tr><th>名称 / 类型</th><th>模型 ID</th><th>分组</th><th>状态</th><th>历史测试</th><th>操作</th></tr></thead><tbody>{data.items.map(c => <tr key={c.id}><td><strong>{c.name}</strong><small>{c.type === 1 ? 'OpenAI-compatible · 类型 1' : `类型 ${c.type} · 只读配置`}</small></td><td>{c.models || '—'}</td><td>{c.group || '—'}</td><td><span className={`badge ${c.status === 1 ? 'active' : ''}`}>{c.status === 1 ? '已启用' : c.status === 2 ? '手动停用' : c.status === 3 ? '自动停用' : `状态 ${c.status}`}</span></td><td>{c.test_time && c.test_time > 0 ? <>{Number.isFinite(c.response_time) ? `${number(c.response_time)} ms` : '—'}<small>{date(c.test_time)}</small></> : '尚无记录'}</td><td><div className="row-actions">{user.role === 100 && c.type === 1 && <button disabled={blocked} onClick={() => { setError(''); setDialog({ kind: 'edit', channel: c }); }}>编辑</button>}<button disabled={blocked} onClick={() => { setError(''); setDialog({ kind: 'status', channel: c }); }}>{c.status === 1 ? '停用' : '启用'}</button></div></td></tr>)}</tbody></table></div>}<Pager page={page} total={data.total} size={data.page_size} disabled={busy || uncertain} onChange={setPage}/></>}</section>
    {dialog?.kind === 'status' && <Modal title={dialog.channel.status === 1 ? '停用渠道' : '启用渠道'} busy={busy} onClose={() => setDialog(null)}><p>确认{dialog.channel.status === 1 ? '停用' : '启用'}“{dialog.channel.name}”？这会影响该渠道的后续模型分发。</p>{error && <Alert>{error}</Alert>}<div className="dialog-actions"><button disabled={busy} onClick={() => setDialog(null)}>取消</button><button className="primary" disabled={busy || uncertain} onClick={() => void mutate({ status: dialog.channel.status === 1 ? 2 : 1 }, 'POST', `/api/channel/${dialog.channel.id}/status`)}>确认{dialog.channel.status === 1 ? '停用' : '启用'}</button></div></Modal>}
    {dialog && dialog.kind !== 'status' && <ChannelEditor client={client} user={user} channel={dialog.kind === 'edit' ? dialog.channel : undefined} busy={busy} uncertain={uncertain} error={error} onClose={() => setDialog(null)} save={mutate}/>}</>;
}
function ChannelEditor({ client, user, channel, busy, uncertain, error, onClose, save }: { client: ApiClient; user: User; channel?: Channel; busy: boolean; uncertain: boolean; error: string; onClose: () => void; save: (body: unknown, method: string) => Promise<boolean> }) {
    const [name, setName] = useState(channel?.name || '');
    const [url, setURL] = useState(channel?.base_url || '');
    const [models, setModels] = useState(channel?.models || '');
    const [group, setGroup] = useState(channel?.group || '');
    const [mapping, setMapping] = useState(channel?.model_mapping || '');
    const [key, setKey] = useState('');
    const [validation, setValidation] = useState('');
    const [groupNote, setGroupNote] = useState('');
    useEffect(() => { if (channel) return; let current = true; client.groups().then(groups => { if (!current) return; const names = Object.keys(groups).sort(); setGroup(value => value || (names.includes(user.group || '') ? user.group! : names[0] || '')); if (!names.length) setGroupNote('当前没有可用分组，请明确填写已有的原生分组。'); }).catch(() => { if (current) setGroupNote('分组读取未完成，请明确填写已有的原生分组。'); }); return () => { current = false; }; }, [client, channel, user.group]);
    async function submit(e: FormEvent) {
        e.preventDefault(); if (busy || uncertain) return;
        setValidation('');
        let parsed: URL;
        try { parsed = new URL(url.trim()); } catch { setValidation('请输入有效的 http 或 https 基础 URL。'); return; }
        if (!['http:', 'https:'].includes(parsed.protocol) || parsed.username || parsed.password || parsed.search || parsed.hash || /\/chat\/completions\/?$/i.test(parsed.pathname)) { setValidation('基础 URL 仅接受 http/https，不含凭据、查询、片段或完整 chat/completions 路径。'); return; }
        if (!name.trim() || !models.trim() || !group.trim() || (!channel && !key.trim())) { setValidation('请填写渠道名称、模型 ID、明确分组和新建所需的上游密钥。'); return; }
        let model_mapping = '';
        if (mapping.trim()) {
            try { const value = JSON.parse(mapping); if (!value || Array.isArray(value) || typeof value !== 'object' || Object.values(value).some(v => typeof v !== 'string')) throw new Error(); model_mapping = JSON.stringify(value); }
            catch { setValidation('模型映射应为值全部是字符串的 JSON 对象。'); return; }
        } else if (channel?.model_mapping) model_mapping = '{}';
        // Explicit allowlist only: no spread of native channel rows or advanced settings.
        const basic = { name: name.trim(), base_url: url.trim().replace(/\/+$/, ''), models: models.trim(), group: group.trim(), model_mapping };
        const body = channel ? { id: channel.id, ...basic, ...(key.trim() ? { key: key.trim() } : {}) } : { mode: 'single', multi_key_mode: '', batch_add_set_key_prefix_2_name: false, channel: { type: 1, ...basic, key: key.trim(), status: 1 } };
        try { await save(body, channel ? 'PUT' : 'POST'); } finally { setKey(''); }
    }
    return <Modal title={channel ? '编辑渠道' : '新建渠道'} busy={busy} onClose={() => { setKey(''); onClose(); }}><form onSubmit={submit}><fieldset className="plain-fieldset" disabled={busy || uncertain}><label>渠道名称<input required maxLength={128} value={name} onChange={e => setName(e.target.value)}/></label><label>基础 URL<input type="url" required maxLength={2048} value={url} onChange={e => setURL(e.target.value)} placeholder="https://host 或 http://127.0.0.1:端口"/></label><p className="hint">类型 1 会追加 /v1/chat/completions。允许部署所需的 loopback HTTP；HTTP 本身不加密，请确保传输路径可信。</p><label>模型 ID（逗号分隔）<input required maxLength={16000} value={models} onChange={e => setModels(e.target.value)}/></label><label>渠道分组（逗号分隔）<input required maxLength={1000} value={group} onChange={e => setGroup(e.target.value)}/></label>{groupNote && <p className="hint">{groupNote}</p>}<label>模型映射 JSON（可选）<textarea rows={3} maxLength={16000} value={mapping} onChange={e => setMapping(e.target.value)} placeholder={'{"public-model":"upstream-model"}'}/></label><label>{channel ? '新上游密钥（留空保留）' : '上游密钥'}<input type="password" autoComplete="off" required={!channel} maxLength={16000} value={key} onChange={e => setKey(e.target.value)}/></label></fieldset>{(validation || error) && <Alert>{validation || error}</Alert>}<div className="dialog-actions"><button type="button" disabled={busy} onClick={() => { setKey(''); onClose(); }}>取消</button><button className="primary" type="submit" disabled={busy || uncertain}>{channel ? '保存渠道' : '确认创建渠道'}</button></div></form></Modal>;
}
