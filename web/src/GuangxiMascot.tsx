import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import type { PointerEvent as ReactPointerEvent } from 'react';
import type { GuangxiRuntime } from './guangxi-runtime';
import './guangxi-mascot.css';

const STORAGE_KEY = 'guangxi-mascot:v1';
type Position = { left: number; bottom: number };
type Preferences = { version: 1; collapsed: boolean; position: Position | null };
type Viewport = { width: number; height: number; x: number; y: number; left: number; right: number; top: number; bottom: number };

function readPreferences(): Preferences {
    const fallback: Preferences = { version: 1, collapsed: window.innerWidth <= 640, position: null };
    try {
        const value = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? 'null');
        if (value?.version !== 1 || typeof value.collapsed !== 'boolean') return fallback;
        if (value.position !== null && !(typeof value.position?.left === 'number' && Number.isFinite(value.position.left)
            && typeof value.position?.bottom === 'number' && Number.isFinite(value.position.bottom))) return fallback;
        return { version: 1, collapsed: value.collapsed, position: value.position };
    } catch { return fallback; }
}

function savePreferences(value: Preferences) {
    try { localStorage.setItem(STORAGE_KEY, JSON.stringify(value)); } catch { /* Session controls still work without storage. */ }
}

function measureViewport(element: HTMLElement | null): Viewport {
    const viewport = window.visualViewport;
    const style = element ? getComputedStyle(element) : null;
    const inset = (side: string) => Number.parseFloat(style?.getPropertyValue(`--gx-safe-${side}`) ?? '') || 0;
    return { width: viewport?.width ?? window.innerWidth, height: viewport?.height ?? window.innerHeight,
        x: viewport?.offsetLeft ?? 0, y: viewport?.offsetTop ?? 0,
        left: 16 + inset('left'), right: 16 + inset('right'), top: 16 + inset('top'), bottom: 16 + inset('bottom') };
}

function dimensions(viewport: Viewport, collapsed: boolean) {
    const availableWidth = Math.max(44, viewport.width - viewport.left - viewport.right);
    const artHeight = Math.max(0, Math.min(300, viewport.height - viewport.top - viewport.bottom - 48, availableWidth * 1.5));
    return { artHeight, width: collapsed ? 44 : Math.min(availableWidth, Math.max(132, artHeight * 2 / 3)), height: collapsed ? 44 : artHeight + 48 };
}

function clamp(position: Position | null, viewport: Viewport, collapsed: boolean): Position {
    const size = dimensions(viewport, collapsed);
    return {
        left: Math.max(viewport.left, Math.min(position?.left ?? viewport.left, viewport.width - viewport.right - size.width)),
        bottom: Math.max(viewport.bottom, Math.min(position?.bottom ?? viewport.bottom, viewport.height - viewport.top - size.height)),
    };
}

export function GuangxiMascot(): React.JSX.Element {
    const root = useRef<HTMLElement>(null);
    const canvas = useRef<HTMLCanvasElement>(null);
    const runtime = useRef<GuangxiRuntime | null>(null);
    const [preferences, setPreferences] = useState(readPreferences);
    const currentPreferences = useRef(preferences);
    const [viewport, setViewport] = useState(() => measureViewport(null));
    const [visible, setVisible] = useState(document.visibilityState !== 'hidden');
    const [reduced, setReduced] = useState(() => window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false);
    const [mode, setMode] = useState<'waiting' | 'ready' | 'failed'>('waiting');
    const [previewFailed, setPreviewFailed] = useState(false);
    const dragging = useRef<{ id: number; x: number; y: number; position: Position } | null>(null);
    const eligible = !preferences.collapsed && visible && !reduced;
    const position = clamp(preferences.position, viewport, preferences.collapsed);
    const size = dimensions(viewport, preferences.collapsed);

    const update = (next: Preferences, persist = true) => {
        currentPreferences.current = next;
        setPreferences(next);
        if (persist) savePreferences(next);
    };

    useLayoutEffect(() => {
        const resize = () => {
            const nextViewport = measureViewport(root.current);
            setViewport(nextViewport);
            const value = currentPreferences.current;
            if (value.position) {
                const next = clamp(value.position, nextViewport, value.collapsed);
                if (next.left !== value.position.left || next.bottom !== value.position.bottom) update({ ...value, position: next });
            }
            dragging.current = null;
            runtime.current?.resize();
        };
        resize();
        window.addEventListener('resize', resize);
        window.visualViewport?.addEventListener('resize', resize);
        window.visualViewport?.addEventListener('scroll', resize);
        return () => {
            window.removeEventListener('resize', resize);
            window.visualViewport?.removeEventListener('resize', resize);
            window.visualViewport?.removeEventListener('scroll', resize);
        };
    }, []);

    useEffect(() => {
        const visibility = () => setVisible(document.visibilityState !== 'hidden');
        const media = window.matchMedia?.('(prefers-reduced-motion: reduce)');
        const motion = () => setReduced(media?.matches ?? false);
        document.addEventListener('visibilitychange', visibility);
        media?.addEventListener('change', motion);
        const element = canvas.current!;
        const fail = () => { runtime.current?.dispose(); runtime.current = null; setMode('failed'); };
        element.addEventListener('webglcontextlost', fail);
        element.addEventListener('error', fail);
        const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(() => runtime.current?.resize());
        observer?.observe(element);
        return () => {
            document.removeEventListener('visibilitychange', visibility);
            media?.removeEventListener('change', motion);
            element.removeEventListener('webglcontextlost', fail);
            element.removeEventListener('error', fail);
            observer?.disconnect();
            runtime.current?.dispose();
            runtime.current = null;
        };
    }, []);

    useEffect(() => {
        if (runtime.current) { runtime.current.setActive(eligible); return; }
        if (!eligible || mode === 'failed') return;
        const controller = new AbortController();
        let idleId: number | undefined;
        let timer: number | undefined;
        let deadline: number | undefined;
        const start = async () => {
            deadline = window.setTimeout(() => { controller.abort(); setMode('failed'); }, 15_000);
            try {
                const { mountGuangxi } = await import('./guangxi-runtime');
                if (controller.signal.aborted) return;
                const instance = await mountGuangxi(canvas.current!, controller.signal);
                if (controller.signal.aborted) { instance.dispose(); return; }
                runtime.current = instance;
                instance.setActive(true);
                setMode('ready');
            } catch (error) {
                if (import.meta.env.DEV && !controller.signal.aborted) console.warn('Guangxi model unavailable:', error);
                if (!controller.signal.aborted) setMode('failed');
            } finally { window.clearTimeout(deadline); }
        };
        const schedule = () => {
            if (window.requestIdleCallback) idleId = window.requestIdleCallback(() => { void start(); }, { timeout: 1500 });
            else timer = window.setTimeout(() => { void start(); }, 250);
        };
        if (document.readyState === 'complete') schedule();
        else window.addEventListener('load', schedule, { once: true });
        return () => {
            window.removeEventListener('load', schedule);
            if (idleId !== undefined) window.cancelIdleCallback(idleId);
            window.clearTimeout(timer);
            window.clearTimeout(deadline);
            controller.abort();
        };
    }, [eligible, mode]);

    const stopDrag = (event: ReactPointerEvent<HTMLButtonElement>) => {
        if (dragging.current?.id !== event.pointerId) return;
        dragging.current = null;
        savePreferences(currentPreferences.current);
        if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
    };
    const toggle = (collapsed: boolean) => {
        const value = currentPreferences.current;
        update({ ...value, collapsed, position: value.position ? clamp(value.position, viewport, collapsed) : null });
    };

    return <aside ref={root} className="guangxi-mascot" aria-label="广喜娘" data-mode={reduced ? 'static' : mode}
        style={{ left: viewport.x + position.left, bottom: window.innerHeight - viewport.y - viewport.height + position.bottom, width: size.width }}>
        <div className="guangxi-art" hidden={preferences.collapsed} style={{ height: size.artHeight }}>
            {!previewFailed && <img src="/assets/guangxi/v1/preview.png" alt="" draggable={false}
                hidden={mode === 'ready' && !reduced} onError={() => setPreviewFailed(true)} />}
            <canvas ref={canvas} aria-hidden="true" style={{ visibility: mode === 'ready' && !reduced ? 'visible' : 'hidden' }} />
        </div>
        <div className="guangxi-controls">
            {preferences.collapsed ? <button type="button" aria-label="展开广喜娘" title="展开广喜娘" onClick={() => toggle(false)}>喜</button> : <>
                <button type="button" className="guangxi-handle" aria-label="拖动广喜娘" title="拖动；方向键移动，Shift 加速"
                    onPointerDown={event => {
                        if (!event.isPrimary || event.button !== 0) return;
                        event.currentTarget.setPointerCapture(event.pointerId);
                        dragging.current = { id: event.pointerId, x: event.clientX, y: event.clientY, position };
                    }}
                    onPointerMove={event => {
                        const drag = dragging.current;
                        if (drag?.id !== event.pointerId) return;
                        update({ ...currentPreferences.current, position: clamp({ left: drag.position.left + event.clientX - drag.x,
                            bottom: drag.position.bottom - event.clientY + drag.y }, viewport, false) }, false);
                    }}
                    onPointerUp={stopDrag} onPointerCancel={stopDrag} onLostPointerCapture={stopDrag}
                    onKeyDown={event => {
                        const delta = event.shiftKey ? 40 : 10;
                        const offsets: Record<string, [number, number]> = { ArrowLeft: [-delta, 0], ArrowRight: [delta, 0], ArrowUp: [0, delta], ArrowDown: [0, -delta] };
                        const offset = offsets[event.key];
                        if (!offset) return;
                        event.preventDefault();
                        update({ ...currentPreferences.current, position: clamp({ left: position.left + offset[0], bottom: position.bottom + offset[1] }, viewport, false) });
                    }}><span aria-hidden="true">✥</span></button>
                <button type="button" aria-label="恢复左下角" title="恢复左下角" onClick={() => update({ ...currentPreferences.current, position: null })}><span aria-hidden="true">↙</span></button>
                <button type="button" aria-label="收起广喜娘" title="收起广喜娘" onClick={() => toggle(true)}><span aria-hidden="true">−</span></button>
            </>}
        </div>
    </aside>;
}
