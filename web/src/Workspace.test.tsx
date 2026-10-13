import { withReadyAccessGate } from './m1-test-fixtures';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { it, expect, vi } from 'vitest';
import { App } from './App';
import { ApiClient } from './api';
const user = { id: 1, username: 'test', display_name: 'Test', role: 100, group: 'team' };
const ok = (data?: unknown) => new Response(JSON.stringify({ success: true, data }));
function setup(path: string, role = 100, respond?: (path: string, init?: RequestInit) => Response | Promise<Response> | undefined) {
 const f = vi.fn(async (p: string, init?: RequestInit) => { const custom = respond?.(p, init); if (custom) return custom; if (p.includes('/refresh')) return ok({ access_token: 'memory', access_expires_at: 9999999999, session: { sid: 'sid' }, user: { ...user, role } }); if (p === '/api/user/self') return ok({ ...user, role }); if (p.endsWith('/groups')) return ok({ team: { ratio: 1, desc: 'Team' }, another: { ratio: 2, desc: 'Another' } }); if (p.includes('/models?group=team')) return ok(['alpha', 'beta']); if (p.includes('/models?group=another')) return ok(['gamma']); return ok({ items: [], total: 0, page: 1, page_size: 10 }); });
 const c = new ApiClient(withReadyAccessGate(f)); const view = render(<MemoryRouter initialEntries={[path]}><App client={c}/></MemoryRouter>); return { f, c, ...view };
}
it('keeps public discovery independent from the private native group reader', async () => { const {f,c}=setup('/models'); await waitFor(()=>expect(c.getSnapshot().ready).toBe(true)); expect(screen.getByRole('heading',{level:1})).toHaveTextContent('模型工房'); expect(screen.queryByLabelText('分组')).not.toBeInTheDocument(); expect(screen.queryByRole('link',{name:'文本测试'})).not.toBeInTheDocument(); expect(f.mock.calls.some(c=>c[0].endsWith('/groups')||c[0].includes('/models?group='))).toBe(false); });
it('retires the text playground without native model calls', async () => {
 const { f } = setup('/playground?model=missing&group=team');
 expect(await screen.findByRole('heading', { level: 1, name: '指挥台' })).toBeVisible();
 expect(screen.queryByLabelText('提示词')).not.toBeInTheDocument();
 expect(screen.queryByRole('link', { name: /文本测试|选择模型并测试/ })).not.toBeInTheDocument();
 expect(f.mock.calls.some(([path]) => path.endsWith('/groups') || path.includes('/models?group=') || path.startsWith('/pg/'))).toBe(false);
});
it('saves local channels and groups directly with availability links',async()=>{
 const root='/api/v1/ops';
 let value={id:7,name:'My channel',type:14,base_url:'https://api.anthropic.com',models:['alpha'],groups:['team'],model_mapping:{alpha:'actual'},header_override:{},priority:2,weight:3,enabled:true,key_configured:true,requests_24h:10,errors_24h:1,error_rate:0.1};
 let groups=[{name:'team',description:'Team',ratio:'1'},{name:'another',description:'Another',ratio:'2'}];
 const f=vi.fn(async(p:string,i?:RequestInit)=>{
  if(p==='/api/v1/session/bootstrap')return ok({authentication_state:'AUTHENTICATED',csrf_token:'local-csrf'});
  if(p==='/api/user/self')return ok({...user,role:10});
  if(p.startsWith('/platform/v1/access-gate?'))return ok({stage:'READY',user_id:'1',route:new URLSearchParams(p.split('?')[1]).get('route')});
  if(p==='/api/v1/maintenance/notices')return ok({items:[]});
  if(p===root+'/groups'){
   if(i?.method==='PUT'){const b=JSON.parse(String(i.body));groups=groups.map(g=>g.name===b.original_name?{name:b.name,description:b.description,ratio:b.ratio}:g);return ok(b)}
   if(i?.method==='DELETE')return new Response(JSON.stringify({success:false,message:'分组仍有用户或渠道引用'}),{status:409});
   return ok(groups);
  }
  if(p.startsWith(root+'/groups/availability'))return ok({items:[{model:'alpha',channels:[value]}],page:1,page_size:20,total:1});
  if(p===root+'/channels/7/models')return ok(['alpha','beta']);
  if(p===root+'/channels/7/test')return ok({ok:true,model:'alpha',duration_ms:10,status_code:200,message:'测试成功'});
  if(p===root+'/channels/7/status'){value={...value,enabled:JSON.parse(String(i?.body)).enabled};return ok(null)}
  if(p===root+'/channels'&&i?.method==='PUT'){value={...value,...JSON.parse(String(i.body))};return ok(value)}
  if(p===root+'/channels/7')return ok(value);
  if(p.startsWith(root+'/channels?'))return ok({items:[value],total:1,page:1,page_size:20});
  throw new Error('Unexpected endpoint: '+p);
 });
 const c=new ApiClient(f);await c.bootstrap();
 const view=render(<MemoryRouter initialEntries={['/admin/channels']}><App client={c}/></MemoryRouter>);
 expect(await screen.findByRole('button',{name:'新建渠道'})).toBeVisible();
 fireEvent.click(await screen.findByRole('link',{name:'编辑'}));
 expect(await screen.findByRole('dialog',{name:'编辑渠道'})).toBeVisible();
 expect(screen.getByLabelText('上游密钥（留空保留）')).toHaveValue('');
 fireEvent.click(screen.getByRole('button',{name:'从上游拉取模型'}));
 await screen.findByText('已拉取 2 个模型；保存后生效。');
 fireEvent.change(screen.getByLabelText('渠道名称'),{target:{value:'Renamed'}});
 fireEvent.click(screen.getByRole('checkbox',{name:'another'}));
 fireEvent.click(screen.getByRole('button',{name:'保存渠道'}));
 await screen.findByText('Renamed');
 const saved=f.mock.calls.find(([p,i])=>p===root+'/channels'&&i?.method==='PUT')!;
 expect(new Headers(saved[1]?.headers).get('X-CSRF-Token')).toBe('local-csrf');
 expect(JSON.parse(String(saved[1]?.body))).toEqual({id:7,name:'Renamed',type:14,base_url:value.base_url,models:['alpha','beta'],groups:['team','another'],model_mapping:{alpha:'actual'},header_override:{},priority:2,weight:3,enabled:true});
 fireEvent.click(screen.getByRole('button',{name:'停用'}));await waitFor(()=>expect(value.enabled).toBe(false));
 fireEvent.click(await screen.findByRole('button',{name:'测试'}));await screen.findByText(/测试成功.*10 ms/);
 fireEvent.click(screen.getAllByRole('link',{name:'分组与可用性'})[0]);
 await screen.findByRole('heading',{name:'模型可用性矩阵'});
 fireEvent.click(screen.getByRole('button',{name:'编辑 team'}));
 fireEvent.change(screen.getByLabelText('倍率'),{target:{value:'1.25'}});
 fireEvent.click(screen.getByRole('button',{name:'保存分组'}));await screen.findByText('分组已保存');
 await waitFor(()=>expect(groups[0].ratio).toBe('1.25'));
 vi.spyOn(window,'confirm').mockReturnValue(true);
 fireEvent.click(await screen.findByRole('button',{name:'删除 team'}));await screen.findByText('分组仍有用户或渠道引用');
 const links=screen.getAllByRole('link',{name:'Renamed · 已停用'});expect(links[0]).toHaveAttribute('href','/ops/channels/7');
 fireEvent.click(links[0]);await screen.findByRole('dialog',{name:'编辑渠道'});
 expect(f.mock.calls.some(([p])=>p.startsWith('/api/channel/')||p.includes('/prepare')||p.includes('/execute')||p==='/api/v1/ops/bootstrap')).toBe(false);
 view.unmount();
});
