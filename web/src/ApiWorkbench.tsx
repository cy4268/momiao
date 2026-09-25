import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { assetUrl } from './game-hall-assets';
import art from './catalog-workshop-art.json';
import './api-workbench.css';

export function ApiWorkbench({ page, children }: { page: 'keys' | 'logs'; children: ReactNode }) {
    return <div className={'api-workbench workbench-' + page}>
        <img className="api-workbench-background" src={assetUrl(art.background)} width={1672} height={941} alt="" fetchPriority="high" />
        <div className="api-workbench-panel">{children}</div>
    </div>;
}

export function UsageNavigation({ roleplay = false }: { roleplay?: boolean }) {
    return <nav className="usage-navigation" aria-label="调用记录类别">
        <Link to="/logs" aria-current={!roleplay ? 'page' : undefined}>原生调用</Link>
        <Link to="/logs?purpose=ROLEPLAY" aria-current={roleplay ? 'page' : undefined}>RP 调用</Link>
        {roleplay && <Link className="usage-ranking-link" to="/rankings?metric=RP_CALLS">RP 排行榜 ↗</Link>}
    </nav>;
}
