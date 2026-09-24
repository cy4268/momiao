import { useEffect, useRef, useState, useSyncExternalStore, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { ApiClient, ApiError } from './api';
import { admissionError, readAdmission, readNativeAccount, type AdmissionStatus, type NativeAccount, type SensitiveProof } from './admission-api';
import { readProfile } from './profile-api';
import { PasswordInput } from './Authentication';
import { Alert, Loading } from './ui';
import { IdentityChamber, IdentityDisclosure, IdentityEmblem } from './IdentityChamber';

export function GrantStatus({client}: {client:ApiClient}) {
    const [data,setData]=useState<AdmissionStatus>();const [error,setError]=useState('');const [loading,setLoading]=useState(false);const active=useRef(true);const serial=useRef(0);
    async function reload(){const n=++serial.current;setLoading(true);try{const next=await readAdmission(client);if(active.current&&serial.current===n){setData(next);setError('');}}catch{if(active.current&&serial.current===n)setError('注册赠额状态暂时无法核对，账户与已保存资料仍然有效。');}finally{if(active.current&&serial.current===n)setLoading(false);}}
    useEffect(()=>{active.current=true;void reload();return()=>{active.current=false;serial.current++;};},[client]);
    useEffect(()=>{if(data?.grant_status!=='PENDING'&&data?.grant_status!=='RECOVERING')return;const timer=setInterval(()=>void reload(),5000);return()=>clearInterval(timer);},[data?.grant_status,client]);
    return <section className="panel account-grant" aria-label="注册赠额状态"><div className="section-heading"><div><p className="eyebrow">REGISTRATION / RESERVE</p><h2>注册赠额</h2></div><button disabled={loading} onClick={()=>void reload()}>核对赠额状态</button></div>{error&&<Alert>{error}</Alert>}{loading&&!data&&<Loading/>}{data&&<>
        <p role="status">{data.grant_status==='CONFIRMED'?'1,000 Reserve 已到账。':data.grant_status==='PENDING'?'注册资格已确认，1,000 Reserve 正在入账。':data.grant_status==='RECOVERING'?'正在恢复原注册赠额，请稍候核对。':'尚未确认新注册来源，当前未发放注册赠额。'}</p>
        <p className="hint">{data.grant_status==='PENDING_SOURCE'?'已有账户不会因为登录或补全资料重复获得新注册赠额。':'同一注册资格只发放一次。网络中断后会继续核对原领取记录，无需重新注册。'}{!data.source_available&&' 注册来源服务暂时不可用，可稍后核对。'}</p>
        {data.grant_status==='CONFIRMED'&&<Link to="/wallet" className="text-link">前往钱包查看账本 →</Link>}
    </>}</section>;
}

export function Account({client,proof,clearProof=()=>{}}: {client:ApiClient;proof?:SensitiveProof;clearProof?:()=>void}) {
    const session=useSyncExternalStore(client.subscribe,client.getSnapshot);const generation=client.getSessionGeneration();
    const opaqueBlocked=client.sessionFeatureUnavailable('GENERAL_BFF_S2');
    const [account,setAccount]=useState<NativeAccount>();const [shortID,setShortID]=useState('');const [error,setError]=useState('');const [notice,setNotice]=useState('');const [loading,setLoading]=useState(true);const [busy,setBusy]=useState(false);const [uncertain,setUncertain]=useState(false);
    const [password,setPassword]=useState('');const [confirm,setConfirm]=useState('');const [old,setOld]=useState('');const [show,setShow]=useState(false);const [showConfirm,setShowConfirm]=useState(false);const [showOld,setShowOld]=useState(false);const active=useRef(true);const lock=useRef(false);
    const validProof=proof&&proof.expires_at>Date.now()/1000?proof:undefined;
    const mode=validProof?.purpose?(validProof.purpose==='PASSWORD_SET'?'set':'reset'):validProof?(account?.has_password?'reset':'set'):'change';
    async function reload(){setLoading(true);setError('');if(opaqueBlocked){try{await client.refresh();}catch(e){if(active.current)setError(admissionError(e));}finally{if(active.current)setLoading(false);}return;}try{const d=await readNativeAccount(client);if(active.current)setAccount(d);const p=await readProfile(client,String(d.id));if(active.current)setShortID(p.short_account_id);}catch(e){if(active.current)setError(admissionError(e));}finally{if(active.current)setLoading(false);}}
    useEffect(()=>{active.current=true;if(opaqueBlocked){setLoading(false);setError('');}else void reload();return()=>{active.current=false;};},[client,generation,opaqueBlocked]);
    async function authorize(reset:boolean){if(lock.current)return;lock.current=true;setBusy(true);setError('');clearProof();try{const url=await client.discordStart(reset?'password-reset':'fresh');if(active.current)window.location.assign(url);}catch(e){if(active.current)setError(admissionError(e));}finally{if(active.current){lock.current=false;setBusy(false);}}}
    async function submit(e:FormEvent){e.preventDefault();if(lock.current||uncertain||!account)return;if(password!==confirm){setError('两次输入的新密码不一致。');return;}if([...password].length<8||[...password].length>20||new TextEncoder().encode(password).length>72){setError('新密码需为 8–20 个字符，且不超过 72 字节。');return;}
        lock.current=true;setBusy(true);setError('');setNotice('');try{await client.updatePassword(mode,mode==='change'?{password,old_password:old}:{password,proof:validProof?.proof});clearProof();if(active.current){setNotice('密码已更新，当前登录已重新核对。');await reload();}}catch(e){clearProof();if(active.current){setError(admissionError(e));if(!(e instanceof ApiError)||e.uncertain)setUncertain(true);}}finally{setPassword('');setConfirm('');setOld('');setShow(false);setShowOld(false);setShowConfirm(false);if(active.current){lock.current=false;setBusy(false);}}}
    if(opaqueBlocked)return <IdentityChamber page="security">
        <header className="page-heading"><div><p className="eyebrow">MY / ACCOUNT &amp; SECURITY</p><h1>账户与安全</h1><p>当前仅显示已核对的不透明会话身份。</p></div><button disabled={loading||session.loggingOut} onClick={()=>void reload()}>重新核对会话</button></header>
        {error&&<Alert>{error}</Alert>}
        <Alert>密码变更尚未接入不透明会话模式；账户详情与 Discord 连接同属 GENERAL_BFF_S2，当前不会调用旧原生端点。</Alert>
        <section className="panel account-identity account-verified"><p className="eyebrow">VERIFIED SESSION IDENTITY</p><h2>已核对身份</h2><IdentityEmblem/>
            <dl className="account-details"><div><dt>登录标识 · 只读</dt><dd>{session.user?.username||'会话待核对'}</dd></div><div><dt>平台用户 ID · 只读</dt><dd>{session.user?.id||'—'}</dd></div></dl>
            <p className="hint">密码、Discord 连接状态和账户扩展资料将在对应 BFF 契约接入后开放。</p>
        </section>
        <div className="account-tools"><AccountExplanation/><button className="account-signout" disabled={session.loggingOut} onClick={()=>{clearProof();void client.logout().catch(()=>{});}}>{session.loggingOut?'正在退出…':'退出当前登录'}</button></div>
    </IdentityChamber>;
    return <IdentityChamber page="security">
        <header className="page-heading"><div><p className="eyebrow">MY / ACCOUNT &amp; SECURITY</p><h1>账户与安全</h1><p>核对登录身份、连接状态与密码。</p></div><button disabled={loading||busy} onClick={()=>void reload()}>刷新账户状态</button></header>
        {error&&<Alert>{error}</Alert>}{notice&&<p className="profile-notice" role="status">{notice}</p>}{loading&&!account&&<Loading/>}
        {account&&<div className="account-grid">
            <section className="panel account-identity"><p className="eyebrow">YOUR LOGIN IDENTITY</p><h2>登录身份</h2><IdentityEmblem/>
                <dl className="account-details"><div><dt>登录标识 · 只读</dt><dd>{account.username}</dd></div><div><dt>账户短 ID · 仅本人可见</dt><dd>{shortID||'正在核对'}</dd></div><div><dt>Discord 连接</dt><dd>{account.discord_connected?'已连接':'尚未连接'}</dd></div><div><dt>账户密码</dt><dd>{account.has_password?'已设置':'尚未设置'}</dd></div><div><dt>二次验证</dt><dd>{account.two_fa_enabled===undefined?'状态暂未提供':account.two_fa_enabled?'已启用':'未启用'}</dd></div></dl>
                <p className="hint">公开昵称在 Master 资料中维护，登录标识保持固定。</p><Link to="/master-profile">查看 Master 资料 →</Link>
            </section>
            <section className="panel account-password"><p className="eyebrow">PASSWORD &amp; SESSION</p><h2>{validProof?mode==='set'?'设置首个密码':'重置密码':account.has_password?'更改密码':'设置账户密码'}</h2>
                {validProof&&<p className="account-proof" role="status">Discord 身份验证已通过。请在本页完成密码操作，离开或刷新后需重新验证。</p>}
                {!account.has_password&&!validProof?<><p className="hint">先重新验证已绑定的 Discord，即可设置首个密码。</p><button className="primary" disabled={busy||!account.discord_connected} onClick={()=>void authorize(false)}>验证 Discord 并设置密码</button></>:<form onSubmit={e=>void submit(e)}>
                    {mode==='change'&&<PasswordInput id="account-old" label="当前密码" value={old} setValue={setOld} show={showOld} setShow={setShowOld} autoComplete="current-password" disabled={busy||uncertain}/>}
                    <PasswordInput id="account-new" label="新密码" value={password} setValue={setPassword} show={show} setShow={setShow} autoComplete="new-password" disabled={busy||uncertain}/><PasswordInput id="account-confirm" label="确认新密码" value={confirm} setValue={setConfirm} show={showConfirm} setShow={setShowConfirm} autoComplete="new-password" disabled={busy||uncertain}/>
                    <p className="hint">8–20 个字符，且不超过 72 字节。成功后其他旧会话将失效。</p><button className="primary" type="submit" disabled={busy||uncertain}>{busy?'正在提交…':mode==='set'?'设置密码':mode==='reset'?'确认重置密码':'更新密码'}</button>
                </form>}
                {account.has_password&&!validProof&&<div className="auth-switch"><p className="hint">忘记当前密码？使用同一已绑定的 Discord 重新验证。</p><button disabled={busy||!account.discord_connected} onClick={()=>void authorize(true)}>通过 Discord 重置密码</button></div>}
                {uncertain&&<p className="hint">密码操作结果尚未确认，本页已暂停提交。请退出后重新登录，核对实际凭据。</p>}
            </section>
        </div>}
        <GrantStatus key={generation} client={client}/>
        <div className="account-tools"><AccountExplanation/>{account&&<button className="account-signout" disabled={session.loggingOut||busy} onClick={()=>{clearProof();void client.logout().catch(()=>{});}}>退出当前登录</button>}</div>
    </IdentityChamber>;
}
function AccountExplanation() {
    return <IdentityDisclosure title="账户与会话说明">
        <h3>登录身份与展示资料</h3><p>登录标识保持固定。Master 昵称与默认头像单独维护，账户短 ID 仅本人可见。二次验证在本页仅展示当前状态。</p>
        <h3>密码与 Discord 验证</h3><p>密码需为 8–20 个字符，且不超过 72 字节。成功后其他旧会话将失效。设置首个密码或重置密码需重新验证同一已绑定的 Discord；验证凭据仅临时保存在当前页面，离开或刷新后需重新验证。</p>
        <p>密码操作结果未确认时暂停再次提交，请退出后重新登录核对实际凭据。当前会话未接入的功能不会调用旧端点。</p>
        <h3>注册赠额</h3><p>同一注册资格只发放一次，已有账户不会因为登录或补全资料重复获得新注册赠额。到账状态与账本以服务端记录为准。</p>
    </IdentityDisclosure>;
}
