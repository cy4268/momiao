import { useEffect, useRef, useState, type ReactNode } from 'react';
import { errorText } from './api';
export const number = (n?: number) => Number.isFinite(n) ? new Intl.NumberFormat('zh-CN').format(n!) : '—';
export const date = (n?: number) => n === -1 ? '永不过期' : n && n > 0 ? new Date(n * 1000).toLocaleString('zh-CN', { hour12: false }) : '—';
export const role = (n: number) => n === 100 ? '超级管理员' : n === 10 ? '管理员' : '成员';
export function Crest({ large = false }: {
    large?: boolean;
}) { return <svg className={large ? 'crest large' : 'crest'} viewBox="0 0 160 180" fill="none" aria-hidden="true"><path d="M80 8 140 40v69c0 27-36 51-60 64-24-13-60-37-60-64V40Z" stroke="currentColor" strokeWidth="2"/><path d="M80 23 126 48v58c0 21-26 42-46 54-20-12-46-33-46-54V48Z" stroke="currentColor"/><circle cx="80" cy="87" r="34" stroke="currentColor"/><path d="M87 54a32 32 0 1 0 20 56 34 34 0 0 1-20-56Z" fill="currentColor" fillOpacity=".16"/><path d="m80 42 7 34 27 11-27 9-7 35-7-35-27-9 27-11Z" fill="currentColor"/><path d="M80 4v17M9 87h24m94 0h24M80 157v19" stroke="currentColor" strokeWidth="2"/></svg>; }
export function Alert({ children }: {
    children: ReactNode;
}) { return <div className="alert" role="alert"><strong>请留意</strong><div>{children}</div></div>; }
export function Empty({ title, children }: {
    title: string;
    children: ReactNode;
}) { return <div className="empty"><span aria-hidden="true">◇</span><h3>{title}</h3><p>{children}</p></div>; }
export function Loading() { return <p className="loading" role="status">正在读取账户数据<span aria-hidden="true">…</span></p>; }
export function useResource<T>(load: () => Promise<T>, dependencies: unknown[]) {
    const [state, setState] = useState<{
        data?: T;
        error: string;
        loading: boolean;
    }>({ error: '', loading: true });
    const [version, setVersion] = useState(0);
    useEffect(() => { let current = true; setState({ error: '', loading: true }); load().then(data => { if (current)
        setState({ data, error: '', loading: false }); }).catch(e => { if (current)
        setState({ error: errorText(e), loading: false }); }); return () => { current = false; }; }, [...dependencies, version]);
    return { ...state, reload: () => setVersion(v => v + 1) };
}
export function Pager({ page, total, size, onChange, disabled = false }: {
    page: number;
    total: number;
    size: number;
    onChange: (p: number) => void;
    disabled?: boolean;
}) { const pages = Math.max(1, Math.ceil(total / size)); return <nav className="pager" aria-label="分页"><span>共 {number(total)} 条 · 第 {page} / {pages} 页</span><div><button disabled={disabled || page <= 1} onClick={() => onChange(page - 1)} aria-label="上一页">← 上一页</button><button disabled={disabled || page >= pages} onClick={() => onChange(page + 1)} aria-label="下一页">下一页 →</button></div></nav>; }
export function Modal({ title, children, onClose, busy = false }: {
    title: string;
    children: ReactNode;
    onClose: () => void;
    busy?: boolean;
}) {
    const ref = useRef<HTMLDialogElement>(null);
    useEffect(() => { const previous = document.activeElement as HTMLElement | null; const dialog = ref.current; dialog?.showModal(); return () => { dialog?.close(); previous?.focus(); }; }, []);
    return <dialog ref={ref} className="modal" aria-labelledby="dialog-title" onCancel={e => { e.preventDefault(); if (!busy)
        onClose(); }}><div className="modal-heading"><h2 id="dialog-title">{title}</h2><button className="icon-button" aria-label="关闭对话框" disabled={busy} onClick={onClose}>×</button></div>{children}</dialog>;
}
