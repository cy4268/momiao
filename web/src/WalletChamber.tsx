import { useState, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { CommandChamber } from './PersonalHub';
import { Modal } from './ui';
import { assetUrl } from './game-hall-assets';
import { assetNames, type WalletData } from './wallet-api';
import art from './wallet-rewards-art.json';
import chamberArt from './command-personal-art.json';
import './wallet-chamber.css';

export function WalletChamber({ page, children }: { page: 'wallet' | 'rewards' | 'transaction'; children: ReactNode }) {
    return <CommandChamber page={page} className="finance-chamber">{children}</CommandChamber>;
}
export function RewardArt({ name }: { name: keyof typeof art }) {
    const image = art[name];
    return <img className="reward-art" src={assetUrl(image.src)} width={image.width} height={image.height} alt="" />;
}
export function BalanceEmblem() {
    return <img className="balance-emblem" src={assetUrl(chamberArt.emblem.src)} width={chamberArt.emblem.width} height={chamberArt.emblem.height} alt="" />;
}
export function WalletRules({ rewards = false, wallet }: { rewards?: boolean; wallet?: WalletData }) {
    const [open, setOpen] = useState(false);
    const title = rewards ? '领取规则与记录说明' : '钱包明细与规则';
    return <><button className="finance-disclosure" aria-haspopup="dialog" aria-expanded={open} onClick={() => setOpen(true)}>{title}<span aria-hidden="true">＋</span></button>
        {open && <Modal title={title} onClose={() => setOpen(false)}><div className="finance-rules">
            <h3>Reserve 与可用筹码</h3><p>本地资产分别记账。钱包初始化不产生赠额；经确认的注册赠额会单独记入账本。<Link to="/account">核对注册赠额状态</Link></p>
            <h3>奖励按需主动领取</h3><ul><li>每日签到 500 Reserve，按上海自然日每天一次。</li><li>小时奖励 100 Reserve，每个上海自然小时一次，每天最多 24 次，不累计补领。</li><li>低资产救济 300 Reserve，总资产严格低于 10 API Credit；成功领取后冷却四小时，不要求离开牌桌。</li></ul><p>三种奖励均进入 Reserve。领取资格、冷却及维护状态以服务端核对结果为准。</p>
            <h3>模型调用与兑换</h3><p>模型调用直接消耗 Reserve，无需激活或划转。Reserve 与筹码 1:1 双向兑换，无手续费；兑换金额以所选本地资产余额为限。</p>
            <h3>真实记录</h3><p>资产交易最新在前，流水按账本序号从早到晚，每页最多 20 条。流水正号为增加、负号为减少。时间按设备时区展示，奖励日期按上海时区计算。</p>
            {wallet?.wallets.map(w => <section key={w.asset}><h3>{assetNames[w.asset]}</h3><dl><div><dt>原子单位</dt><dd>{w.balance_units}</dd></div><div><dt>账本序号</dt><dd>{w.ledger_seq}</dd></div><div><dt>钱包版本</dt><dd>{w.version}</dd></div></dl></section>)}
        </div></Modal>}
    </>;
}
