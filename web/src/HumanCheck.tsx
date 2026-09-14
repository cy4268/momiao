import { useEffect, useRef, useState } from 'react';

type TurnstileAPI = {
    render(element: HTMLElement, options: Record<string, unknown>): string;
    remove(id: string): void;
};
const api = () => (window as Window & { turnstile?: TurnstileAPI }).turnstile;
let loading: Promise<TurnstileAPI> | undefined;
function load(): Promise<TurnstileAPI> {
    const ready = api();
    if (ready) return Promise.resolve(ready);
    if (loading) return loading;
    loading = new Promise<TurnstileAPI>((resolve, reject) => {
        const script = document.createElement('script');
        script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit';
        script.async = true;
        const fail = () => { clearTimeout(timer); script.remove(); loading = undefined; reject(new Error('验证组件暂时无法加载，请重试。')); };
        const timer = setTimeout(fail, 15000);
        script.onerror = fail;
        script.onload = () => { const value = api(); if (!value) { fail(); return; } clearTimeout(timer); resolve(value); };
        document.head.appendChild(script);
    });
    return loading;
}

export function HumanCheck({ siteKey, reset, onToken }: { siteKey: string; reset: number; onToken: (token: string) => void }) {
    const container = useRef<HTMLDivElement>(null);
    const [retry, setRetry] = useState(0), [notice, setNotice] = useState('正在加载安全验证…');
    useEffect(() => {
        let active = true, widget: string | undefined, service: TurnstileAPI | undefined;
        onToken(''); setNotice('正在加载安全验证…');
        load().then(value => {
            if (!active || !container.current) return;
            service = value;
            const invalid = (message: string) => { if (active) { onToken(''); setNotice(message); } };
            widget = value.render(container.current, {
                sitekey: siteKey, size: 'flexible', theme: 'auto',
                callback: (token: string) => { if (active) { onToken(token); setNotice('安全验证已完成。'); } },
                'expired-callback': () => invalid('安全验证已过期，请重新验证。'),
                'error-callback': () => invalid('安全验证未完成，请重试。'),
            });
            if (!widget) throw new Error('验证组件未能启动，请重试。');
            setNotice('请完成安全验证。');
        }).catch(() => { if (active) { onToken(''); setNotice('安全验证暂时不可用，请重试。'); } });
        return () => { active = false; onToken(''); if (service && widget) service.remove(widget); };
    }, [siteKey, reset, retry, onToken]);
    return <section aria-label="安全验证"><div ref={container} /><p role="status">{notice}</p><button type="button" onClick={() => { onToken(''); setRetry(value => value + 1); }}>重新验证</button></section>;
}
