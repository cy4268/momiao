import { useEffect, useState, type FormEvent, type ReactNode } from 'react';
import { Link, useLocation, useParams, useSearchParams } from 'react-router-dom';
import { ApiClient } from './api';
import { Alert, Brand, Empty, Modal, useResource } from './ui';
import { announcementError, announcementRoot, announcementStates, announcementTime, announcementTypes, checkedAnnouncement, markPopupSeen, popupSeen, publicAnnouncements, type Announcement } from './announcement-api';
import './announcements.css';

function AnnouncementShell({ client, children }: { client: ApiClient; children: ReactNode }) {
    const signedIn = !!client.getSnapshot().user;
    return <div className='announcement-site'><a className='skip-link' href='#announcement-content'>跳至公告内容</a><header className='public-header'><Link to='/' className='brand' aria-label='Chaldea Platform 首页'><Brand /></Link><nav aria-label='公告导航'><Link to='/announcements' aria-current='page'>公告</Link><Link to={signedIn ? '/dashboard' : '/login'}>{signedIn ? '指挥台' : '登录账户'}</Link></nav></header><main id='announcement-content' className='announcement-main' tabIndex={-1}>{children}</main><footer className='public-footer'><span>Chaldea Platform / 公告与更新</span><nav><Link to='/'>返回首页</Link>{signedIn && <Link to='/ops/announcements'>公告运营</Link>}</nav></footer></div>;
}
export function AnnouncementBody({ item }: { item: Announcement }) {
    return <><div className='announcement-body' dangerouslySetInnerHTML={{ __html: item.sanitized_html }} />{item.acknowledgements.length > 0 && <section className='announcement-acknowledgements' aria-label='致谢名单'>{item.acknowledgements.map(entry => <article key={entry.manual_order}>{entry.group_name && <small>{entry.group_name}</small>}<h3>{entry.external_link ? <a href={entry.external_link} target='_blank' rel='noopener noreferrer nofollow'>{entry.display_name}</a> : entry.display_name}</h3>{entry.acknowledgement_note && <p>{entry.acknowledgement_note}</p>}</article>)}</section>}</>;
}
export function Announcements({ client }: { client: ApiClient }) {
    const [params, setParams] = useSearchParams(); const [search, setSearch] = useState(params.get('search') || ''); const [type, setType] = useState(params.get('type') || ''); const [from, setFrom] = useState(params.get('date_from') || ''); const [to, setTo] = useState(params.get('date_to') || '');
    const archive = params.get('archive') === 'true'; const offset = Number(params.get('offset') || 0); const query = params.toString();
    const resource = useResource(() => publicAnnouncements(client, '?' + query).catch(e => { throw new Error(announcementError(e)); }), [client, query]);
    useEffect(() => { document.title = '公告 · Chaldea Platform'; }, []);
    function apply(event: FormEvent) { event.preventDefault(); const p = new URLSearchParams(); if (search.trim()) p.set('search', search.trim()); if (type) p.set('type', type); if (from) p.set('date_from', from); if (to) p.set('date_to', to); if (archive) p.set('archive', 'true'); setParams(p); }
    function page(next: number) { const p = new URLSearchParams(params); p.set('offset', String(next)); setParams(p); }
    return <AnnouncementShell client={client}><header className='announcement-title'><p className='eyebrow'>CHALDEA / BULLETIN</p><h1>公告与更新</h1><p>平台消息、维护安排，以及每一份值得记住的支持。</p></header><div className='announcement-tabs' aria-label='公告范围'><button aria-pressed={!archive} onClick={() => { const p = new URLSearchParams(params); p.delete('archive'); p.delete('offset'); setParams(p); }}>当前公告</button><button aria-pressed={archive} onClick={() => { const p = new URLSearchParams(params); p.set('archive', 'true'); p.delete('offset'); setParams(p); }}>历史归档</button></div><form className='announcement-filters' onSubmit={apply}><label>公告类别<select value={type} onChange={e => setType(e.target.value)}><option value=''>全部类别</option>{Object.entries(announcementTypes).map(([value, label]) => <option value={value} key={value}>{label}</option>)}</select></label><label className='announcement-search'>搜索公告<input value={search} onChange={e => setSearch(e.target.value)} maxLength={200} placeholder='标题或正文关键词' /></label><label>开始日期<input type='date' value={from} onChange={e => setFrom(e.target.value)} /></label><label>结束日期<input type='date' value={to} onChange={e => setTo(e.target.value)} /></label><button type='submit'>应用筛选</button></form><p className='hint'>日期与时间按上海时区显示。{archive ? '这里保留曾经发布的历史公告。' : '置顶公告按人工顺序排列，其余按发布时间排列。'}</p>
        {resource.loading ? <p role='status'>正在读取公告…</p> : resource.error ? <><Alert>{resource.error}</Alert><button onClick={resource.reload}>重新读取</button></> : resource.data?.items.length ? <><div className='announcement-list'>{resource.data.items.map(item => <article className='announcement-row' key={item.announcement_id}><div className='announcement-row-meta'><span>{announcementTypes[item.type]}</span>{item.pinned && !archive && <b>置顶</b>}{client.getSnapshot().user && <small>{item.read ? '已读' : '未读'}</small>}</div><h2><Link to={'/announcements/' + item.announcement_id + (archive ? '?archive=true' : '')}>{item.title}</Link></h2><time dateTime={item.publish_at || undefined}>{announcementTime(item.publish_at)}</time><span className='announcement-row-arrow' aria-hidden='true'>↗</span></article>)}</div><nav className='announcement-pager' aria-label='公告分页'><button disabled={offset <= 0} onClick={() => page(Math.max(0, offset - 20))}>上一页</button><span>第 {Math.floor(offset / 20) + 1} 页</span><button disabled={!resource.data.has_more || offset >= 10000} onClick={() => page(offset + 20)}>下一页</button></nav></> : <Empty title='没有符合条件的公告'>可以调整筛选条件，或稍后再来查看。</Empty>}
    </AnnouncementShell>;
}
export function AnnouncementDetail({ client }: { client: ApiClient }) {
    const { id = '' } = useParams(); const [params] = useSearchParams(); const archive = params.get('archive') === 'true';
    const resource = useResource(() => client.announcementRequest<Announcement>(announcementRoot + '/' + id + (archive ? '?archive=true' : '')).then(checkedAnnouncement).catch(e => { throw new Error(announcementError(e)); }), [client, id, archive]);
    const [readError, setReadError] = useState(''); const [readAttempt, setReadAttempt] = useState(0);
    useEffect(() => { document.title = (resource.data?.title || '公告详情') + ' · Chaldea Platform'; }, [resource.data?.title]);
    useEffect(() => {
        const item = resource.data; if (!item || !client.getSnapshot().user || item.read) return;
        let current = true; setReadError('');
        void client.request(announcementRoot + '/' + item.announcement_id + '/reads', 'POST', { notification_revision: item.notification_revision }).catch(e => { if (current) setReadError(announcementError(e)); });
        return () => { current = false; };
    }, [resource.data, client, readAttempt]);
    return <AnnouncementShell client={client}><Link className='announcement-back' to={'/announcements' + (archive ? '?archive=true' : '')}>← {archive ? '历史归档' : '全部公告'}</Link>{resource.loading ? <p role='status'>正在读取公告…</p> : resource.error ? <><Alert>{resource.error}</Alert><div className='announcement-actions'><button onClick={resource.reload}>重新读取</button>{!client.getSnapshot().user && <Link to='/login'>登录账户</Link>}<Link to='/announcements?archive=true'>查看历史归档</Link></div></> : resource.data && <article className='announcement-detail'><header><p className='eyebrow'>{announcementTypes[resource.data.type]} / {announcementStates[resource.data.state]}</p><h1>{resource.data.title}</h1><dl className='announcement-dates'><div><dt>发布时间</dt><dd>{announcementTime(resource.data.publish_at)}</dd></div><div><dt>最近更新</dt><dd>{announcementTime(resource.data.updated_at)}</dd></div>{resource.data.visible_until && <div><dt>展示截止</dt><dd>{announcementTime(resource.data.visible_until)}</dd></div>}</dl></header><AnnouncementBody item={resource.data} />{readError && <Alert>正文已读取，阅读状态暂未保存。{readError}<button onClick={() => setReadAttempt(v => v + 1)}>重试保存阅读状态</button></Alert>}</article>}</AnnouncementShell>;
}
export function AnnouncementEntry({ client }: { client: ApiClient }) {
    const { pathname } = useLocation(); const [item, setItem] = useState<Announcement | null>(null);
    useEffect(() => {
        let current = true; setItem(null); if (pathname !== '/' && pathname !== '/login') return;
        void client.announcementRequest<{ item: Announcement | null }>(announcementRoot + '/current-entry-popup').then(result => { if (!current || !result?.item) return; const next = checkedAnnouncement(result.item); if (!popupSeen(next)) { markPopupSeen(next, false); setItem(next); } }).catch(() => {});
        return () => { current = false; };
    }, [client, pathname]);
    function close() { if (item) markPopupSeen(item, true); setItem(null); }
    if (!item || pathname !== '/' && pathname !== '/login') return null;
    return <div className='announcement-overlay'><Modal title={item.title} onClose={close}><p className='eyebrow'>{announcementTypes[item.type]}</p><div className='announcement-popup-body'><AnnouncementBody item={item} /></div><div className='announcement-actions'><Link className='button primary' to={'/announcements/' + item.announcement_id} onClick={close}>查看完整公告</Link><button onClick={close}>稍后再看</button></div></Modal></div>;
}
export function AnnouncementHomeBanner({ client }: { client: ApiClient }) {
    const resource = useResource(() => client.announcementRequest<{ item: Announcement | null }>(announcementRoot + '/current-home-banner').then(r => r?.item ? checkedAnnouncement(r.item) : null), [client]);
    if (!resource.data) return null;
    return <aside className='announcement-home-banner' aria-label='首页公告'><div><p className='eyebrow'>{announcementTypes[resource.data.type]}</p><h2>{resource.data.title}</h2></div><Link className='text-link' to={'/announcements/' + resource.data.announcement_id}>查看公告 ↗</Link></aside>;
}
