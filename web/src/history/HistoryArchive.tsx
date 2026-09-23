import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { assetSrcSet, assetUrl, gameArt } from '../game-hall-assets';
import art from './history-art.json';
import './history.css';

export function HistoryArchive({ title, detail, navigation, list = false, children }: {
  title: string; detail: string; navigation?: ReactNode; list?: boolean; children: ReactNode;
}) {
  return <div className={'history-page history-archive' + (list ? ' history-archive-list' : '')}>
    <img className="archive-background" src={assetUrl(art.background.src)} srcSet={assetSrcSet(art.background)} sizes="100vw" width={art.background.width} height={art.background.height} alt="" />
    <aside className="archive-guide" aria-label="档案室引导角色">
      <img src={assetUrl(art.holmes.src)} srcSet={assetSrcSet(art.holmes)} sizes="(max-width: 999px) 180px, 34vw" width={art.holmes.width} height={art.holmes.height} alt="FGO 夏洛克·福尔摩斯，手持放大镜的档案室引导者" />
      <div><strong>夏洛克·福尔摩斯</strong><span>RULER / CHALDEA ARCHIVE</span></div>
    </aside>
    <div className="archive-main">
      {navigation}
      <header className="page-heading archive-heading"><div><p className="eyebrow">CHALDEA / ARCHIVE</p><h1>{title}</h1><p>{detail}</p></div><Link className="archive-exit" to="/games">返回游戏目录 ↗</Link></header>
      <div className="archive-content" tabIndex={list ? undefined : 0} role={list ? undefined : 'region'} aria-label={list ? undefined : '记录详情内容'}>{children}</div>
    </div>
  </div>;
}

export function HistoryCover({ game }: { game: string }) {
  const cover = gameArt(game);
  return cover ? <img className="archive-game-cover" src={assetUrl(cover.src)} srcSet={assetSrcSet(cover)} sizes="80px" width={cover.width} height={cover.height} alt="" loading="lazy" /> : null;
}
