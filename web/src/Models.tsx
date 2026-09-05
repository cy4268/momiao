import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { ApiClient, ApiError, errorText, type User } from './api';
import { type ChatResult } from './chat';
import { Alert, Empty, Loading, number, useResource } from './ui';

function useModels(client: ApiClient, user: User, requestedGroup = '', requestedModel = '') {
    const groups = useResource(() => client.groups(), [client]);
    const names = Object.keys(groups.data || {}).sort();
    const [chosenGroup, chooseGroup] = useState(requestedGroup);
    const group = names.includes(chosenGroup) ? chosenGroup : names.includes(user.group || '') ? user.group! : names[0] || '';
    const models = useResource(async () => ({ group, ids: group ? await client.models(group) : [] }), [client, group]);
    // Never let the old group's options survive a render before the loading effect runs.
    const ids = models.data?.group === group ? models.data.ids : [];
    const [chosenModel, chooseModel] = useState(requestedModel);
    const model = ids.includes(chosenModel) ? chosenModel : ids[0] || '';
    const setGroup = (value: string) => { chooseGroup(value); chooseModel(''); };
    return { groups, names, group, setGroup, models, ids, model, setModel: chooseModel, loading: groups.loading || models.loading || Boolean(group && models.data?.group !== group && !models.error), error: groups.error || models.error, reload: () => { groups.reload(); models.reload(); } };
}
function ModelEmpty({ admin }: { admin: boolean }) {
    return <Empty title="当前分组暂无可用模型">模型来自实际启用的渠道配置。{admin ? <>请检查分组与渠道：<Link to="/admin/channels">配置渠道</Link>。</> : '请联系管理员检查模型与分组配置。'}</Empty>;
}
function GroupSelect({ workspace: w, disabled = false }: { workspace: ReturnType<typeof useModels>; disabled?: boolean }) {
    return <label>分组<select value={w.group} disabled={disabled || !w.names.length} onChange={e => w.setGroup(e.target.value)}>{!w.names.length && <option value="">暂无可用分组</option>}{w.names.map(name => <option value={name} key={name}>{name}{w.groups.data?.[name].desc ? ` · ${w.groups.data[name].desc}` : ''}</option>)}</select></label>;
}
export function Models({ client, user }: { client: ApiClient; user: User }) {
    const w = useModels(client, user);
    const [search, setSearch] = useState('');
    const [notice, setNotice] = useState('');
    const visible = w.ids.filter(id => id.toLowerCase().includes(search.toLowerCase()));
    async function copy(id: string) { try { await navigator.clipboard.writeText(id); setNotice('模型 ID 已复制。'); } catch { setNotice('复制未完成，请选择模型 ID 手动复制。'); } }
    return <><header className="page-heading"><div><p className="eyebrow">CONNECT / MODEL DIRECTORY</p><h1>模型目录</h1><p>按实际可用分组选择模型，开始一次文本测试。</p></div><button disabled={w.loading} onClick={w.reload}>刷新模型</button></header>
        <section className="panel"><div className="filters"><GroupSelect workspace={w}/><label>搜索模型 ID<input value={search} onChange={e => setSearch(e.target.value)} maxLength={200} placeholder="输入模型 ID 关键词"/></label></div>
        <p className="hint">API Base URL：<code className="inline-code">{window.location.origin}/v1</code>。模型可见不代表调用必定成功；实际权限、额度与渠道状态仍由服务端校验。</p>
        {notice && <p role="status" className="notice">{notice}</p>}{w.loading ? <Loading/> : w.error ? <><Alert>{w.error}</Alert><button onClick={w.reload}>重新加载</button></> : !w.ids.length ? <ModelEmpty admin={user.role >= 10}/> : !visible.length ? <Empty title="没有匹配的模型">尝试其它关键词，或清空搜索。</Empty> : <ul className="model-list">{visible.map(id => <li key={id}><code>{id}</code><div className="row-actions"><button onClick={() => void copy(id)} aria-label={`复制 ${id} 模型 ID`}>复制 ID</button><Link className="button" to={`/playground?${new URLSearchParams({ model: id, group: w.group })}`}>测试此模型</Link></div></li>)}</ul>}
        </section></>;
}
const emptyResult: ChatResult = { text: '', reasoning: '' };
export function Playground({ client, user }: { client: ApiClient; user: User }) {
    const [query] = useSearchParams();
    const w = useModels(client, user, query.get('group') || '', query.get('model') || '');
    const [prompt, setPrompt] = useState('');
    const [maxTokens, setMaxTokens] = useState('256');
    const [result, setResult] = useState<ChatResult>(emptyResult);
    const [status, setStatus] = useState('等待发送');
    const [error, setError] = useState('');
    const [latency, setLatency] = useState<number>();
    const [busy, setBusy] = useState(false);
    const controller = useRef<AbortController | null>(null);
    useEffect(() => () => { controller.current?.abort(); controller.current = null; }, [client]);
    async function send(e: FormEvent) {
        e.preventDefault(); if (controller.current) return;
        const budget = Number(maxTokens);
        if (w.loading || w.error || !w.model || !w.group || !prompt.trim() || prompt.length > 16000 || !Number.isInteger(budget) || budget < 1 || budget > 4096) { setError('请选择实际可用模型，输入 1–16000 字提示词及 1–4096 的整数输出预算。'); return; }
        const current = new AbortController(); controller.current = current;
        const start = performance.now(); setBusy(true); setError(''); setStatus('正在连接'); setResult(emptyResult); setLatency(undefined);
        try {
            const output = await client.playground({ model: w.model, group: w.group, prompt, maxTokens: budget }, current.signal, r => { if (controller.current === current && !current.signal.aborted) { setResult(r); setStatus('正在接收'); } });
            if (controller.current !== current || current.signal.aborted) return;
            setResult(output); setStatus('已完成');
            void client.loadSelf().catch(() => {});
        } catch (e) {
            if (controller.current !== current) return;
            setStatus(e instanceof ApiError && e.code === 'ABORTED' ? '已停止' : '未完成'); setError(errorText(e));
        } finally {
            if (controller.current === current) { controller.current = null; setBusy(false); setLatency(Math.round(performance.now() - start)); }
        }
    }
    return <><header className="page-heading"><div><p className="eyebrow">EXPLORE / TEXT PLAYGROUND</p><h1>文本测试</h1><p>使用当前登录账户发起单轮请求，不创建 API 密钥。</p></div><Link className="text-link" to="/models">返回模型目录 →</Link></header>
    <p className="hint">每次只发送当前提示词，不带历史。提示词和输出仅保留在当前页面内存，离开或退出即清除。调用可能消耗账户额度，停止不保证上游停止计费。</p>
    <div className="playground-grid"><section className="panel"><h2>本次请求</h2><form onSubmit={send}><fieldset disabled={busy} className="plain-fieldset"><GroupSelect workspace={w}/><label>模型<select value={w.model} disabled={w.loading || !w.ids.length} onChange={e => w.setModel(e.target.value)}>{!w.ids.length && <option value="">暂无可用模型</option>}{w.ids.map(id => <option key={id} value={id}>{id}</option>)}</select></label>
    {w.loading ? <Loading/> : w.error ? <><Alert>{w.error}</Alert><button type="button" onClick={w.reload}>重新加载</button></> : !w.ids.length && <ModelEmpty admin={user.role >= 10}/>}
    <label>最大输出 Tokens<input type="number" min="1" max="4096" step="1" required value={maxTokens} onChange={e => setMaxTokens(e.target.value)}/></label><p className="hint">默认 256 是本页面测试预算；可设为 1–4096，模型实际限制由服务端决定。</p>
    <label>提示词<textarea rows={7} maxLength={16000} required value={prompt} onChange={e => setPrompt(e.target.value)} placeholder="输入这一次要测试的问题"/></label><small>{prompt.length} / 16000 字符</small></fieldset>
    <div className="playground-actions"><button className="primary" type="submit" disabled={busy || w.loading || Boolean(w.error) || !w.model}>发送</button><button type="button" disabled={!busy} onClick={() => { setStatus('正在停止'); controller.current?.abort(); }}>停止</button><button type="button" disabled={busy} onClick={() => { setPrompt(''); setResult(emptyResult); setStatus('等待发送'); setError(''); setLatency(undefined); }}>清空</button></div></form></section>
    <section className="panel response-panel" aria-label="模型响应"><div className="section-heading"><h2>模型响应</h2><span className="badge" role="status">{status}</span></div><p className="hint">接收上限 1 MiB · 请求时限 5 分钟{latency !== undefined && <> · 本次耗时 {number(latency)} ms</>}</p>{error && <Alert>{error}</Alert>}
    {result.reasoning && <section className="reasoning-output"><h3>推理内容</h3><pre>{result.reasoning}</pre></section>}{result.text ? <pre className="model-output">{result.text}</pre> : <p className="hint">{busy ? '等待文本输出…' : '文本输出将在这里显示。'}</p>}
    {result.usage && <p className="usage-summary">服务端用量 · 输入 {number(result.usage.prompt_tokens)} / 输出 {number(result.usage.completion_tokens)} / 总计 {number(result.usage.total_tokens)} Tokens</p>}</section></div></>;
}
