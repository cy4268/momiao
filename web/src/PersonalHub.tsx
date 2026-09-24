import type { ReactNode } from 'react';
import { Link, useOutletContext } from 'react-router-dom';
import type { ApiClient, User } from './api';
import type { MasterProfileData } from './profile-api';
import { Alert, Loading, role } from './ui';
import { assetSrcSet, assetUrl } from './game-hall-assets';
import art from './command-personal-art.json';
import './command-personal.css';

export function CommandChamber({ page, children, className = '' }: { page: 'dashboard' | 'personal' | 'wallet' | 'rewards' | 'activation' | 'transaction' | 'profile' | 'security'; children: ReactNode; className?: string }) {
    return <div className={'command-chamber chamber-' + page + ' ' + className}>
        <img className="chamber-background" src={assetUrl(art.background.src)} srcSet={assetSrcSet(art.background)} sizes="100vw" width={art.background.width} height={art.background.height} alt="" fetchPriority="high" />
        <div className="chamber-content">{children}</div>
    </div>;
}

export interface MasterResource { data?: MasterProfileData; loading: boolean; error: string; reload: () => void }
export function MasterSummary({ master }: { master: MasterResource }) {
    return <section className="master-summary" aria-label="Master 身份">
        <div className="master-emblem" role="img" aria-label="系统默认头像"><img src={assetUrl(art.emblem.src)} width={art.emblem.width} height={art.emblem.height} alt="" /></div>
        <div className="master-copy"><p className="eyebrow">MASTER IDENTITY</p>
            {master.loading ? <Loading /> : master.error ? <><Alert>Master 资料读取未完成。{master.error}</Alert><button onClick={master.reload}>重新读取 Master 资料</button></> : master.data?.status === 'COMPLETE' ? <><h2>{master.data.display_name}</h2><p className="hint">账户短 ID：{master.data.short_account_id} · 仅本人可见</p><Link className="text-link" to="/master-profile">编辑 Master 资料 →</Link></> : <><h2>等待你的 Master 身份</h2><p>设定自己的昵称，并明确保存后开始使用。</p><Link className="button" to="/master-profile">建立 Master 资料</Link></>}
        </div>
    </section>;
}
export function PersonalHub({ client, user }: { client: ApiClient; user: User }) {
    const master = useOutletContext<MasterResource>();
    return <CommandChamber page="personal"><header className="page-heading"><div><p className="eyebrow">MY / PERSONAL HUB</p><h1>个人中心</h1><p>你的 Master 身份与常用入口。</p></div></header>
        <MasterSummary master={master} />
        <div className="hub-sections"><section className="panel hub-group"><p className="eyebrow">RESERVE & REWARDS</p><h2>奖励与资产</h2><HubLink to="/rewards" title="每日奖励" detail="查看领取状态，领取每日固定 500 Reserve。" /><HubLink to="/wallet" title="钱包与账本" detail="核对本地资产、兑换与交易回执。" /><HubLink to="/wallet/activate" title="激活 API 额度" detail="主动将 Reserve 转入原生可用额度。" /></section>
            <section className="panel hub-group"><p className="eyebrow">MODELS & CONNECTIONS</p><h2>API 工作空间</h2><HubLink to="/keys" title="管理 API 密钥" detail="为不同应用建立与管理独立连接。" /><HubLink to="/logs" title="个人调用记录" detail="查看模型调用和真实额度消耗。" /><HubLink to="/playground" title="单轮文本测试" detail="选择账户可用模型后，按需发起测试。" /><HubLink to="/models" title="浏览可用模型" detail="查看当前账户与分组可用的模型。" /></section>
        </div>
        <section className="panel hub-admin"><div><h2>娱乐时光</h2><p>选一间沙龙，开始一局；结果与流水随时可查。</p></div><Link className="button" to="/entertainment">探索游戏</Link></section>
        {user.role >= 10 && <section className="panel hub-admin"><div><p className="eyebrow">ADMINISTRATION</p><h2>管理入口</h2><p>管理已有模型渠道与连接配置。</p></div><Link className="button" to="/admin/channels">渠道管理</Link></section>}
        <section className="native-account" aria-label="原生登录账户"><div><h2>原生登录身份</h2><p>{user.display_name} · {user.username}</p><small>{role(user.role)} · Master 昵称不作为登录用户名。</small></div><div className="native-account-actions"><Link className="button" to="/account">账户与安全</Link><button onClick={() => void client.logout().catch(() => {})}>退出登录</button></div></section>
    </CommandChamber>;
}
function HubLink({ to, title, detail }: { to: string; title: string; detail: string }) {
    return <Link className="hub-link" to={to}><div><strong>{title}</strong><span>{detail}</span></div><span aria-hidden="true">↗</span></Link>;
}
