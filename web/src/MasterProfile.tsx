import { useEffect, useRef, useState, useSyncExternalStore, type FormEvent } from 'react';
import { ApiClient, ApiError, type User } from './api';
import { parseProfile, profileError, readProfile, type MasterProfileData } from './profile-api';
import { Alert, Crest, Loading } from './ui';

export function MasterProfile({ client, user }: { client: ApiClient; user: User }) {
    useSyncExternalStore(client.subscribe, client.getSnapshot);
    const generation = client.getSessionGeneration();
    if (!Number.isSafeInteger(user.id) || user.id <= 0) return <Alert>账户标识格式异常，请重新登录后读取资料。</Alert>;
    return <ProfileView key={`${user.id}:${generation}`} client={client} userID={String(user.id)} generation={generation} />;
}
function ProfileView({ client, userID, generation }: { client: ApiClient; userID: string; generation: number }) {
    const [profile, setProfile] = useState<MasterProfileData>();
    const [name, setName] = useState('');
    const [preview, setPreview] = useState<string | null>(null);
    const [loading, setLoading] = useState(true);
    const [busy, setBusy] = useState(false);
    const [uncertain, setUncertain] = useState(false);
    const [readError, setReadError] = useState('');
    const [notice, setNotice] = useState('');
    const [saved, setSaved] = useState(false);
    const active = useRef(false);
    const lock = useRef(true);
    const readSequence = useRef(0);
    const minimumVersion = useRef('0');
    const current = () => active.current && client.getSessionGeneration() === generation && String(client.getSnapshot().user?.id) === userID && !client.getSnapshot().loggingOut;

    async function reload(reason: 'manual' | 'saved' | 'stale' = 'manual') {
        const sequence = ++readSequence.current;
        lock.current = true; setLoading(true); setReadError(''); setSaved(false);
        try {
            const data = await readProfile(client, userID);
            if (!current() || sequence !== readSequence.current) return;
            if (BigInt(data.profile_version) < BigInt(minimumVersion.current)) throw new ApiError('读取的资料版本落后于保存结果。', 0, 'PROFILE_STALE_READ');
            minimumVersion.current = data.profile_version;
            setProfile(data); setName(data.display_name); setPreview(null); setUncertain(false);
            setNotice(reason === 'stale' ? '资料版本已更新，已读取最新资料。请核对后重新编辑，不会自动重放本次修改。' : '');
            setSaved(reason === 'saved'); lock.current = false;
        } catch (e) {
            if (current() && sequence === readSequence.current) setReadError(profileError(e));
        } finally {
            if (current() && sequence === readSequence.current) { setLoading(false); setBusy(false); }
        }
    }
    useEffect(() => {
        active.current = true; void reload();
        return () => { active.current = false; readSequence.current++; };
    }, [client, userID, generation]);

    async function save(e: FormEvent) {
        e.preventDefault();
        if (lock.current || !current() || !profile || uncertain || readError || !name.trim() || name === profile.display_name) return;
        lock.current = true; setBusy(true); setNotice(''); setSaved(false);
        try {
            const initial = profile.status === 'INCOMPLETE';
            const body = initial ? { expected_version: '0', display_name: name, avatar_id: 'system-default' } : { expected_version: profile.profile_version, display_name: name };
            const result = parseProfile(await client.request(`/platform/v1/master-profile${initial ? '/initialize' : ''}`, initial ? 'POST' : 'PATCH', body), userID);
            if (result.status !== 'COMPLETE' || BigInt(result.profile_version) < BigInt(profile.profile_version)) throw new ApiError('资料写入响应未确认。', 0, 'PROFILE_INVALID_RESPONSE');
            if (!current()) return;
            minimumVersion.current = result.profile_version;
            setUncertain(true);
            await reload('saved');
        } catch (e) {
            if (!current()) return;
            if (e instanceof ApiError && e.code === 'STALE_RESOURCE_VERSION') {
                setNotice(profileError(e)); setUncertain(true); await reload('stale');
            } else if (e instanceof ApiError && !e.uncertain && e.status >= 400 && e.status < 500) {
                setNotice(profileError(e)); lock.current = false;
            } else {
                // Never retry writes automatically, including malformed success responses.
                setUncertain(true); setNotice(`保存结果尚未确认。请点击“刷新资料”核对，核对成功前暂停保存。${profileError(e)}`);
            }
        } finally { if (current()) setBusy(false); }
    }
    const disabled = loading || busy || uncertain || !!readError;
    const displayedName = preview ?? profile?.display_name ?? '';
    return <div className="profile-page">
        <header className="page-heading"><div><p className="eyebrow">IDENTITY / MASTER PROFILE</p><h1>Master 资料</h1><p>为你的公开身份设定昵称，登录账户保持不变。</p></div><button onClick={() => void reload()} disabled={loading || busy}>刷新资料</button></header>
        <p className="profile-scope">这是独立的 Master 展示资料，不修改原生用户名、密码或钱包。填写前，你仍可使用现有门户功能。</p>
        {notice && <Alert>{notice}</Alert>}{readError && <Alert>{readError} 请使用“刷新资料”重试读取。</Alert>}
        {saved && <p className="profile-notice" role="status">资料已保存，并已核对最新状态。</p>}
        {loading && <Loading />}
        {profile && <div className="profile-grid">
            <section className="panel profile-editor" aria-labelledby="profile-edit-title">
                <div className="section-heading"><div><p className="eyebrow">YOUR MASTER IDENTITY</p><h2 id="profile-edit-title">{profile.status === 'INCOMPLETE' ? '建立你的 Master 资料' : '编辑展示资料'}</h2></div><span className="profile-state">{profile.status === 'INCOMPLETE' ? '尚未初始化' : '已初始化'}</span></div>
                <form onSubmit={e => void save(e)}>
                    <label htmlFor="master-name">Master 昵称</label><input id="master-name" value={name} onChange={e => { setName(e.target.value); setPreview(null); setSaved(false); }} disabled={disabled} autoComplete="off" required aria-describedby="master-name-help" placeholder="输入你想使用的昵称" />
                    <p className="hint" id="master-name-help">1–24 个字符，可使用文字、数字、空格及 _ - ·。不含表情或隐藏字符；规范化、重名与保留名由服务端校验。</p>
                    {profile.status === 'INCOMPLETE' && <p className="profile-suggestion">可选参考：<code>{profile.suggested_name}</code><br /><span>仅供参考，不会自动填入或保存。</span></p>}
                    <div className="profile-avatar-option"><div className="profile-avatar" role="img" aria-label="系统默认头像"><Crest /></div><div><strong>系统默认头像</strong><small>SYSTEM / Crest</small><p>随资料保存，无需上传。</p></div><span className="profile-state">当前头像</span></div>
                    <p className="hint">初始化后首次改名可立即进行；每次实际改名后需间隔 7 天。可改名时间以服务端为准。</p>
                    <p className="hint">昵称与头像会用于后续开放的游戏、排行等公开区域。本轮仅保存资料和本人预览，不创建他人可访问的公开主页。</p>
                    <div className="profile-actions"><button type="button" onClick={() => setPreview(name)} disabled={disabled || !name.trim()}>预览资料</button><button className="primary" type="submit" disabled={disabled || !name.trim() || name === profile.display_name}>{busy ? '正在保存…' : profile.status === 'INCOMPLETE' ? '保存并初始化' : '保存修改'}</button></div>
                    {uncertain && <p className="hint">保存已暂停，等待成功读取最新资料。</p>}
                </form>
            </section>
            <div className="profile-identity-column"><section className="panel profile-preview" aria-label="公开身份预览">
                <p className="eyebrow">MASTER / IDENTITY PREVIEW</p><h2>公开身份预览</h2>
                <div className="profile-preview-emblem" role="img" aria-label="系统默认头像"><Crest large /></div>
                <h3 className="profile-display-name">{displayedName || '等待你的昵称'}</h3>
                <p className="profile-preview-label">{preview !== null ? '未保存的预览' : profile.status === 'COMPLETE' ? '当前已保存资料' : '预览占位 · 尚未建立公开身份'}</p>
                <p className="hint">公开身份只展示昵称与头像。此处为本人预览；只有明确保存才会写入资料。</p>
            </section><section className="panel profile-private" aria-label="仅本人可见的账户信息">
                <h2>仅本人可见的账户信息</h2>
                <p className="hint">账户短 ID、版本与改名时间仅本人可见，不随昵称与头像公开展示。</p>
                <dl className="profile-details"><div><dt>账户短 ID</dt><dd>{profile.short_account_id}</dd></div><div><dt>资料版本</dt><dd>{profile.profile_version}</dd></div><div><dt>上次实际改名</dt><dd>{profile.nickname_changed_at ? <ProfileTime value={profile.nickname_changed_at} /> : '尚无改名记录'}</dd></div><div><dt>下次可改名</dt><dd>{profile.next_rename_at ? <ProfileTime value={profile.next_rename_at} /> : profile.status === 'COMPLETE' ? '可立即改名' : '初始化后可改名'}</dd></div></dl>
                <p className="hint">时间按设备所在时区显示。</p>
            </section></div>
        </div>}
    </div>;
}
function ProfileTime({ value }: { value: string }) { return <time dateTime={value}>{new Date(value).toLocaleString('zh-CN', { hour12: false })}</time>; }
