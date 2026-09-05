import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Brand } from './ui';

export function Home({ signedIn }: { signedIn: boolean }) {
    const [artFailed, setArtFailed] = useState(false);
    useEffect(() => { document.title = 'Chaldea Platform · momiao'; }, []);
    return <div className="public-home">
        <a className="skip-link" href="#home-content">跳至主要内容</a>
        <header className="public-header"><Link className="brand" to="/" aria-label="Chaldea Platform 首页"><Brand /></Link><nav aria-label="首页导航"><a href="#pathways">探索平台</a><Link className="button" to={signedIn ? '/dashboard' : '/login'}>{signedIn ? '打开指挥台' : '登录账户'}</Link></nav></header>
        <main id="home-content" tabIndex={-1}>
            <section className="home-hero" aria-labelledby="home-title">
                <div className="home-copy"><p className="eyebrow">CHALDEA / ROYAL OBSERVATORY</p><h1 id="home-title">在月光下，<br />连接新的可能<span>。</span></h1><p className="home-description">这里是 Chaldea Platform。<br />属于你的模型连接、个人空间，<br className="desktop-break" />以及片刻轻松的起点。</p><div className="hero-actions"><Link className="button primary" to={signedIn ? '/me' : '/login'}>{signedIn ? '进入个人中心' : '登录并开始'} <span aria-hidden="true">↗</span></Link><a className="text-link" href="#pathways">了解平台 <span aria-hidden="true">↓</span></a></div><p className="home-access-note">现有账户可登录使用，社区注册将另行开放。</p></div>
                <figure className={'home-art' + (artFailed ? ' art-unavailable' : '')}>
                    {!artFailed && <picture><source type="image/avif" srcSet="/assets/home/bg-royal-observatory-v001.avif" /><img src="/assets/home/bg-royal-observatory-v001.webp" alt="" width="1672" height="941" fetchPriority="high" onError={() => setArtFailed(true)} /></picture>}
                    {artFailed && <div className="observatory-fallback" aria-hidden="true"><Brand /></div>}
                    <figcaption><span>ROYAL OBSERVATORY</span><span>月光中的观测宫</span></figcaption>
                </figure>
            </section>
            <section id="pathways" className="home-pathways" aria-labelledby="pathways-title"><div className="home-section-title"><p className="eyebrow">FIND YOUR WAY</p><h2 id="pathways-title">从这里，走向你的下一站</h2><p>现在可以使用的功能，和明确的下一步。</p></div>
                <article className="home-path api-path"><p className="eyebrow">MODELS & API</p><h3>让灵感，接入模型</h3><p>查看账户可用模型，管理独立 API 密钥，用单轮文本测试确认连接，在调用记录中核对使用情况。</p><Link className="text-link" to="/models">探索模型目录 <span aria-hidden="true">↗</span></Link><small>模型目录与 API 工具需要登录。</small></article>
                <article className="home-path experience-path"><p className="eyebrow">A MOMENT OF PLAY</p><h3>稍作休息，试试手气</h3><p>无资产骰子体验：在浏览器内选择大或小，观察掷骰结果与概率。体验不下注、不派奖，也不消耗钱包资产。</p><Link className="text-link" to="/games/dice">进入骰子体验 <span aria-hidden="true">↗</span></Link><small>需要登录。正式游戏与 Poker 尚未开放。</small></article>
            </section>
            <section className="home-journey" aria-labelledby="journey-title"><div><p className="eyebrow">YOUR PERSONAL SPACE</p><h2 id="journey-title">把下一步，交还给你</h2><p>建立 Master 资料，从奖励开始，按需使用额度。</p></div><ol><li><strong>建立个人身份</strong><span>在个人中心管理你的 Master 昵称与默认头像。</span></li><li><strong>领取每日奖励</strong><span>上海自然日每日一次，固定 500 API Credit 进入 Reserve。</span></li><li><strong>激活并使用 API</strong><span>在钱包中主动转入原生额度，再选择模型、管理密钥。</span></li></ol></section>
        </main>
        <footer className="public-footer"><span>momiao / Chaldea Platform</span><nav aria-label="页脚导航"><Link to="/models">模型目录（需登录）</Link>{signedIn && <Link to="/me">个人中心</Link>}<a href="https://github.com/cy4268/momiao" target="_blank" rel="noopener noreferrer">源代码 ↗</a></nav></footer>
    </div>;
}
