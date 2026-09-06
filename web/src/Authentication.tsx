import { useEffect, useRef, useState, useSyncExternalStore, type FormEvent, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { ApiClient, errorText, type TwoFactor } from './api';
import { admissionError, type AdmissionConfig, type AdmissionResult, type DiscordCallbackInput, type SensitiveProof } from './admission-api';
import { Alert, Brand, Loading } from './ui';

export type CapturedCallback = {input?: DiscordCallbackInput; error?: string};
export function AuthFrame({children,registration=false}: {children:ReactNode;registration?:boolean}) {
    const [artFailed,setArtFailed]=useState(false);
    const [failedCharacter,setFailedCharacter]=useState('');
    const character='/assets/characters/'+(registration?'mash-registration-idle-v001.png':'artoria-saber-login-idle-v001.png');
    return <main className="auth-layout"><section className="auth-story"><Link to="/" className="brand" aria-label="Chaldea Platform 首页"><Brand/></Link>
        <div className="auth-story-copy"><p className="eyebrow">ROYAL OBSERVATORY / CHALDEA</p><h1>{registration ? <>新的旅程，<br/>从此刻建立连接。</> : <>欢迎回来，<br/>你的指挥台已就绪。</>}</h1><p>{registration ? '为你留一个位置。完成账户验证，建立属于自己的 Master 身份。' : '让想法再次启航。你的模型、连接与每一份记录，都在这里。'}</p></div>
        <div className="auth-panorama" aria-hidden="true">{artFailed ? <Brand/> : <picture><source type="image/avif" srcSet="/assets/home/bg-royal-observatory-v001.avif"/><img src="/assets/home/bg-royal-observatory-v001.webp" alt="" width="1672" height="941" onError={()=>setArtFailed(true)}/></picture>}</div>
        {failedCharacter!==character&&<img className="auth-character" src={character} alt="" aria-hidden="true" width="1024" height="1536" decoding="async" onError={()=>setFailedCharacter(character)}/>}
        <div className="auth-caption"><span>BEACON / PERSONAL CONNECTION</span><span>01 — {registration?'ARRIVAL':'RETURN'}</span></div>
    </section><section className="auth-entry"><div className="auth-card">{children}</div><footer>Chaldea Platform <span> / </span><Link to="/">返回首页</Link></footer></section></main>;
}

export function Authentication({client,mode}: {client:ApiClient;mode:'login'|'registration'}) {
    const session=useSyncExternalStore(client.subscribe,client.getSnapshot);
    const registration=mode==='registration';
    const [config,setConfig]=useState<AdmissionConfig>(); const [configError,setConfigError]=useState('');
    const [username,setUsername]=useState(''); const [password,setPassword]=useState(''); const [show,setShow]=useState(false);
    const [challenge,setChallenge]=useState<TwoFactor>(); const [error,setError]=useState(''); const [busy,setBusy]=useState(false);
    const lock=useRef(false); const active=useRef(true);
    useEffect(()=>{active.current=true;document.title=(registration?'注册':'登录')+' · momiao';void client.admissionConfig().then(d=>{if(active.current)setConfig(d);}).catch(()=>{if(active.current)setConfigError('Discord 入口暂时无法读取，请稍后重试。');});return()=>{active.current=false;};},[client,registration]);
    async function discord(){if(lock.current||session.loggingOut)return;lock.current=true;setBusy(true);setError('');try{const url=await client.discordStart(registration?'registration':'login');if(active.current)window.location.assign(url);}catch(e){if(active.current)setError(admissionError(e));}finally{if(active.current){lock.current=false;setBusy(false);}}}
    async function passwordLogin(e:FormEvent){e.preventDefault();if(lock.current||session.loggingOut)return;lock.current=true;setBusy(true);setError('');try{const next=await client.login(username.trim(),password);if(active.current&&next)setChallenge(next);}catch(e){if(active.current)setError(errorText(e));}finally{setPassword('');setShow(false);if(active.current){lock.current=false;setBusy(false);}}}
    return <AuthFrame registration={registration}><p className="eyebrow">{registration?'A NEW CONNECTION':'WELCOME ABOARD'}</p><h2>{challenge?'验证你的身份':registration?'建立你的账户':'欢迎回来'}</h2><p className="subtitle">{challenge?'输入验证器验证码或备用码，完成本次登录。':registration?'使用 Discord 验证注册资格，随后建立 Master 资料。':'使用已绑定的 Discord 或原生账户密码登录。'}</p>
        {session.notice&&<Alert>{session.notice}{session.notice.includes('服务端退出未确认')&&<button disabled={session.loggingOut} onClick={()=>void client.logout().catch(()=>{})}>重试退出</button>}</Alert>}
        {challenge?<TwoFactorEntry busy={busy} onSubmit={async code=>{const next=await client.verify2fa(challenge.flow_token,code);if(next)setChallenge(next);}} onCancel={()=>{setChallenge(undefined);setError('');}}/>:<>
            {config?.enabled&&<div className="auth-discord"><button className="primary" disabled={busy||session.loggingOut||(registration&&!config.registration_enabled)} onClick={()=>void discord()}>{busy?'正在连接…':registration?'使用 Discord 注册':'使用 Discord 登录'}<span aria-hidden="true">↗</span></button>{registration&&<p className="hint">{config.eligibility}</p>}</div>}
            {!config&&!configError&&<Loading/>}{configError&&<p className="hint" role="status">{configError}</p>}{registration&&config&&!config.registration_enabled&&<p role="status">新用户注册暂未开放，请稍后再来。</p>}
            {!registration&&<><div className="auth-divider"><span>或使用密码</span></div><form onSubmit={e=>void passwordLogin(e)}><label htmlFor="login-username">用户名</label><input id="login-username" autoComplete="username" value={username} onChange={e=>setUsername(e.target.value)} required maxLength={128} placeholder="原生登录标识" disabled={busy}/><PasswordInput id="login-password" label="密码" value={password} setValue={setPassword} show={show} setShow={setShow} autoComplete="current-password" disabled={busy}/><button className="primary auth-submit" disabled={busy||session.loggingOut} type="submit">{session.loggingOut?'正在退出…':busy?'正在验证…':'登录控制台'}<span aria-hidden="true">↗</span></button></form></>}
        </>}{error&&<Alert>{error}</Alert>}
        <div className="auth-switch">{registration?<Link to="/login">已有账户，返回登录</Link>:<Link to="/register">新用户？查看注册资格</Link>}</div><p className="auth-note">在共享设备使用后，请退出登录。未设置密码的账户可通过已绑定的 Discord 登录。</p>
    </AuthFrame>;
}

export function PasswordInput({id,label,value,setValue,show,setShow,autoComplete,disabled=false}: {id:string;label:string;value:string;setValue:(v:string)=>void;show:boolean;setShow:(v:boolean)=>void;autoComplete:string;disabled?:boolean}) {
    return <div className="password-field"><label htmlFor={id}>{label}</label><div><input id={id} type={show?'text':'password'} value={value} onChange={e=>setValue(e.target.value)} autoComplete={autoComplete} required maxLength={256} disabled={disabled}/><button type="button" className="password-visibility" aria-label={(show?'隐藏':'显示')+label} aria-pressed={show} disabled={disabled} onClick={()=>setShow(!show)}>{show?'隐藏':'显示'}</button></div></div>;
}

export function TwoFactorEntry({onSubmit,onCancel,busy=false}: {onSubmit:(code:string)=>Promise<unknown>;onCancel:()=>void;busy?:boolean}) {
    const [code,setCode]=useState('');const [error,setError]=useState('');const [sending,setSending]=useState(false);const lock=useRef(false);
    async function submit(e:FormEvent){e.preventDefault();if(lock.current||busy)return;lock.current=true;setSending(true);setError('');try{await onSubmit(code.trim());}catch(e){setError(admissionError(e));}finally{setCode('');lock.current=false;setSending(false);}}
    return <form onSubmit={e=>void submit(e)}><label htmlFor="auth-2fa-code">验证码或备用码</label><input id="auth-2fa-code" autoFocus autoComplete="one-time-code" value={code} onChange={e=>setCode(e.target.value)} maxLength={64} required disabled={sending||busy}/>{error&&<Alert>{error}</Alert>}<div className="auth-actions"><button type="submit" className="primary" disabled={sending||busy}>{sending?'正在验证…':'完成身份验证'}</button><button type="button" disabled={sending||busy} onClick={onCancel}>重新开始</button></div></form>;
}

export function DiscordCallback({client,captured,onComplete}: {client:ApiClient;captured:CapturedCallback;onComplete:(proof?:SensitiveProof)=>void}) {
    const [challenge,setChallenge]=useState<TwoFactor>();const [error,setError]=useState(captured.error||'');const started=useRef(false);const active=useRef(true);
    function complete(result:AdmissionResult){if(!active.current)return;if(result&&'require_2fa'in result)setChallenge(result);else onComplete(result&&'proof'in result?result:undefined);}
    useEffect(()=>{active.current=true;document.title='身份验证 · momiao';if(!started.current){started.current=true;if(captured.input){const input=captured.input;captured.input=undefined;void client.discordCallback(input).then(complete).catch(e=>{if(active.current)setError(admissionError(e));});}else if(!captured.error)setError('授权回调已失效，请重新开始。');}return()=>{active.current=false;};},[client,captured]);
    return <AuthFrame><p className="eyebrow">VERIFY YOUR CONNECTION</p><h2>{challenge?'完成身份验证':'正在核对你的账户'}</h2>{error?<><Alert>{error}</Alert><div className="auth-actions"><Link className="button" to="/login">返回登录</Link><Link className="button" to="/register">查看注册资格</Link></div></>:challenge?<><p className="subtitle">启用了二次验证，请完成验证后继续。</p><TwoFactorEntry onSubmit={async code=>complete(await client.admission2fa(challenge.flow_token,code))} onCancel={()=>{setChallenge(undefined);setError('本次验证已暂停，请返回登录重新开始。');}}/></>:<><Loading/><p className="hint">请稍候，正在确认已提交的授权结果。</p></>}</AuthFrame>;
}
