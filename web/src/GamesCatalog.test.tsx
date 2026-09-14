import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom';
import { expect, it } from 'vitest';
import { ApiClient } from './api';
import { GamesCatalog } from './Games';

const items = [
    {slug:'dice',title:'骰子 Dice',effective_runtime:'PLAY',implementation_key:'direct.dice.v1'},
    {slug:'scratch',title:'未知实现卡',effective_runtime:'PLAY',implementation_key:'uploaded.script'},
    {slug:'blackjack',title:'二十一点',effective_runtime:'MAINTENANCE',implementation_key:'direct.blackjack.v1'},
    {slug:'future-card',title:'新沙龙',effective_runtime:'COMING_SOON',implementation_key:'future.card.v1'},
];
function fixture(status=200) {
    const requests:string[]=[];
    const client=new ApiClient(async(path,init)=>{
        requests.push(path);
        if(!path.startsWith('/api/v1/games')||init?.method!=='GET')throw new Error('Unexpected private or mutation request');
        const result=path.includes('q=none')?[]:path.includes('q=Dice')?[items[0]]:items;
        return new Response(JSON.stringify(status===200?{success:true,data:{items:result}}:{success:false,message:'目录服务暂不可用',code:'GAME_TEMPORARILY_UNAVAILABLE'}),{status});
    });
    return {client,requests};
}
function NavigationProbe(){const location=useLocation(),navigate=useNavigate();return <><output data-testid="route">{location.pathname+location.search}</output><button onClick={()=>navigate(-1)}>浏览器后退</button><button onClick={()=>navigate(1)}>浏览器前进</button></>}
function show(client:ApiClient,entry='/games') {return render(<MemoryRouter initialEntries={[entry]}><NavigationProbe/><GamesCatalog client={client}/></MemoryRouter>)}

it('submits public server filters and restores them through Back and Forward',async()=>{
    const f=fixture();show(f.client);await screen.findByText('新沙龙');
    fireEvent.change(screen.getByRole('searchbox',{name:'搜索游戏'}),{target:{value:' Dice '}});
    fireEvent.change(screen.getByLabelText('运行状态'),{target:{value:'PLAY'}});
    fireEvent.change(screen.getByLabelText('排序方式'),{target:{value:'NAME'}});
    fireEvent.click(screen.getByRole('button',{name:'应用筛选'}));
    await waitFor(()=>expect(screen.getByTestId('route')).toHaveTextContent('/games?q=Dice&availability=PLAY&sort=NAME'));
    await waitFor(()=>expect(screen.queryByText('新沙龙')).not.toBeInTheDocument());
    expect(f.requests.at(-1)).toBe('/api/v1/games?q=Dice&availability=PLAY&sort=NAME');
    fireEvent.click(screen.getByRole('button',{name:'浏览器后退'}));
    await screen.findByText('新沙龙');expect(screen.getByRole('searchbox',{name:'搜索游戏'})).toHaveValue('');
    fireEvent.click(screen.getByRole('button',{name:'浏览器前进'}));
    await waitFor(()=>expect(screen.getByRole('searchbox',{name:'搜索游戏'})).toHaveValue('Dice'));
    expect(screen.getByLabelText('运行状态')).toHaveValue('PLAY');
    expect(f.requests.every(p=>p.startsWith('/api/v1/games'))).toBe(true);
});

it('hydrates a refreshed URL and clears a genuinely empty matching result',async()=>{
    const f=fixture();show(f.client,'/games?q=none&sort=NAME');
    await screen.findByRole('heading',{name:'没有匹配的游戏'});
    expect(screen.getByRole('searchbox',{name:'搜索游戏'})).toHaveValue('none');
    fireEvent.click(screen.getByRole('button',{name:'清除筛选'}));
    await screen.findByText('新沙龙');expect(screen.getByTestId('route')).toHaveTextContent(/^\/games$/);
});

it.each(['/games?q=%FF','/games??q=Dice'])('rejects malformed URL %s before any catalog request',async entry=>{
    const f=fixture();show(f.client,entry);
    await screen.findByRole('alert');expect(f.requests).toEqual([]);
    fireEvent.click(screen.getByRole('button',{name:'清除筛选'}));await screen.findByText('新沙龙');
});

it('fails closed on unknown implementations while preserving Blackjack maintenance recovery',async()=>{
    const f=fixture();show(f.client);const unknown=await screen.findByRole('heading',{name:'未知实现卡'});
    expect(within(unknown.closest('article')!).queryByRole('link')).not.toBeInTheDocument();
    const blackjack=screen.getByRole('heading',{name:'二十一点'}).closest('article')!;
    expect(within(blackjack).getByRole('link',{name:/查看与恢复牌局/})).toHaveAttribute('href','/games/blackjack');
});

it('keeps service failure distinct from empty matches and leaves Hub queries untouched',async()=>{
    const f=fixture(503);show(f.client,'/entertainment?q=Dice');
    await screen.findByRole('alert');expect(screen.queryByRole('heading',{name:'没有匹配的游戏'})).not.toBeInTheDocument();
    expect(screen.queryByRole('searchbox')).not.toBeInTheDocument();expect(f.requests).toEqual(['/api/v1/games']);
});

it('keeps game identity symbols through filtering and sorting',async()=>{
    const unknown={...items[3],slug:'constructor'};
    const client=new ApiClient(async path=>new Response(JSON.stringify({success:true,data:{items:path.includes('q=blackjack')?[items[2]]:path.includes('sort=NAME')?[items[2],items[0],unknown]:[items[0],items[2],unknown]}}),{status:200}));
    show(client);
    const symbol=(name:string)=>screen.getByRole('heading',{name}).closest('article')!.querySelector('.game-item-visual > span');
    await screen.findByText('二十一点');
    fireEvent.change(screen.getByRole('searchbox',{name:'搜索游戏'}),{target:{value:'blackjack'}});
    fireEvent.click(screen.getByRole('button',{name:'应用筛选'}));
    await waitFor(()=>expect(screen.queryByText('骰子 Dice')).not.toBeInTheDocument());
    expect(symbol('二十一点')).toHaveTextContent('♠');
    fireEvent.click(screen.getByRole('button',{name:'清除筛选'}));await screen.findByText('骰子 Dice');
    fireEvent.change(screen.getByLabelText('排序方式'),{target:{value:'NAME'}});
    fireEvent.click(screen.getByRole('button',{name:'应用筛选'}));
    await waitFor(()=>expect(screen.getAllByRole('article')[0]).toContainElement(screen.getByRole('heading',{name:'二十一点'})));
    expect(symbol('二十一点')).toHaveTextContent('♠');expect(symbol('骰子 Dice')).toHaveTextContent('⚄');
    expect(symbol('新沙龙')).toHaveTextContent('◇');
});

it.each([['/games','PLAY'],['/games','MAINTENANCE'],['/entertainment','PLAY'],['/entertainment','MAINTENANCE']])('routes exact Poker %s/%s to the Lobby',async(path,state)=>{
        const requests:string[]=[];
        const client=new ApiClient(async p=>{requests.push(p);return new Response(JSON.stringify({success:true,data:{items:[{slug:'texas-holdem',title:'德州扑克',effective_runtime:state,implementation_key:'poker.texas-holdem.v1'}]}}));});
        const view=show(client,path);const card=(await screen.findByRole('heading',{name:'德州扑克'})).closest('article')!;
        expect(within(card).getByRole('link',{name:state==='PLAY'?'进入 Poker 大厅 →':'查看大厅与恢复牌局 →'})).toHaveAttribute('href','/poker');
        expect(within(card).queryByText('新的沙龙正在准备，敬请期待。')).not.toBeInTheDocument();
        expect(requests).toEqual(['/api/v1/games']);view.unmount();
});

it.each([
    ['texas-holdem','poker.texas-holdem.v1','COMING_SOON'],
    ['texas-holdem','poker.texas-holdem.v1','RETIRED'],
    ['texas-holdem','poker.texas-holdem.v1','TEMPORARILY_UNAVAILABLE'],
    ['texas-holdem','direct.texas-holdem.v1','PLAY'],
    ['texas-holdem','poker.texas-holdem.v2','MAINTENANCE'],
    ['future-card','poker.texas-holdem.v1','PLAY'],
])('keeps catalog tuple %s/%s/%s closed',async(slug,implementation_key,effective_runtime)=>{
    const client=new ApiClient(async()=>new Response(JSON.stringify({success:true,data:{items:[{slug,title:'待核对目录项',implementation_key,effective_runtime}]}})));
    show(client);const card=(await screen.findByRole('heading',{name:'待核对目录项'})).closest('article')!;
    expect(within(card).queryByRole('link')).not.toBeInTheDocument();
});

it('preserves each of the five direct PLAY destinations beside Poker',async()=>{
    const slugs=['dice','scratch','summon','slot','blackjack'];
    const client=new ApiClient(async()=>new Response(JSON.stringify({success:true,data:{items:slugs.map(slug=>({slug,title:slug,effective_runtime:'PLAY',implementation_key:`direct.${slug}.v1`}))}})));
    show(client);await screen.findByRole('heading',{name:'blackjack'});
    for(const slug of slugs)expect(within(screen.getByRole('heading',{name:slug}).closest('article')!).getByRole('link')).toHaveAttribute('href','/games/'+slug);
});
