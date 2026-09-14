import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Brand } from './ui';
import type { ApiClient } from './api';
import './home.css';

export function Home({ signedIn }: { signedIn: boolean; client?: ApiClient }) {
    const [artFailed, setArtFailed] = useState(false);
    useEffect(() => { document.title = 'Chaldea · 在月光下，与你相逢'; }, []);
    return <div className="moon-home">
        <a className="skip-link" href="#home-content">跳至主要内容</a>
        {!artFailed && <img className="moon-home-background" src="/assets/home/chaldea-moonlit-palace.png" alt="" width="1672" height="941" fetchPriority="high" onError={() => setArtFailed(true)} />}
        <div className="moon-home-shade" aria-hidden="true" />
        <header className="moon-home-brand" aria-label="Chaldea Platform"><Brand /></header>
        <main className="moon-home-content" id="home-content" tabIndex={-1}>
            <p className="moon-home-eyebrow">CHALDEA</p>
            <h1>在月光下，<br />与你相逢。</h1>
            <p className="moon-home-description">一处与朋友分享灵感的地方。</p>
            {signedIn ? <Link className="moon-home-enter" to="/dashboard">返回指挥台 <span aria-hidden="true">↗</span></Link> :
                <a className="moon-home-enter" href="/sign-in"><svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path fill="currentColor" d="M20.3 4.5a19.4 19.4 0 0 0-4.8-1.5l-.6 1.2a18 18 0 0 0-5.8 0L8.5 3a19.4 19.4 0 0 0-4.8 1.5C.7 9 .1 13.5.4 17.9a19.5 19.5 0 0 0 5.9 3l1.2-2a12 12 0 0 1-1.9-.9l.5-.4a14 14 0 0 0 11.8 0l.5.4a12 12 0 0 1-1.9.9l1.2 2a19.5 19.5 0 0 0 5.9-3c.4-5.1-.7-9.6-3.3-13.4ZM8 15.4c-1.1 0-2-1-2-2.3s.9-2.3 2-2.3 2 1 2 2.3-.9 2.3-2 2.3Zm8 0c-1.1 0-2-1-2-2.3s.9-2.3 2-2.3 2 1 2 2.3-.9 2.3-2 2.3Z" /></svg>Discord 登录</a>}
        </main>
    </div>;
}
