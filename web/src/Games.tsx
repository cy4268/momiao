import { Fragment, useEffect, useRef, useState, useSyncExternalStore, type FormEvent } from 'react';
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom';
import { ApiClient, ApiError } from './api';
import { Alert, Brand, Empty, Loading, useResource } from './ui';
import { DiceStage, ScratchStage, SummonStage } from './games/DirectStages';
import {SlotGame} from './games/SlotGame';
import {BlackjackGame,type BlackjackCommand} from './games/BlackjackGame';
import {blackjackActionStorage,findBlackjackAction,readBlackjackAction,type PendingBlackjackAction} from './games-api';
import { chips, createGame, findPendingGame, gameError, gameNames, gameSlugs, gameUUID, outcomeNames, parseRound, pendingStorage, readGameBootstrap, readPending, units, wagerCost, type Commitment, type GameBootstrap, type GameConfig, type GameEntry, type GameInput, type GameRound, type GameSlug, type GameVerification, type PendingGame } from './games-api';
import './games.css';
import './game-hall.css';
import { assetSrcSet, assetUrl, gameArt, hallArt, type ImageAsset } from './game-hall-assets';
import { catalogAvailability, catalogQuerySchema, parseCatalogQuery, serializeCatalogQuery, parsePublicCatalog, type CatalogQuery } from './game-catalog-query';

const gameCopy:Record<GameSlug,{eyebrow:string;intro:string}>={dice:{eyebrow:'LUCKY DICE SALON',intro:'三颗骰子，一次选择。让星光为这一刻落定。'},scratch:{eyebrow:'TREASURE VOUCHER SALON',intro:'刮开星纹，寻找属于你的三枚相同印记。'},summon:{eyebrow:'GRAND MANIFESTATION THEATRE',intro:'点亮召唤阵，让每一次独立的星光回应你。'},slot:{eyebrow:'ROYAL TREASURY GALLERY',intro:'五轴星纹落定，十条线共同回应这一局。'},blackjack:{eyebrow:'VIP ROYAL TABLE',intro:'坐进皇家牌桌，让同一副牌序回应你的每次选择。'}};
const runtimeNames:Record<string,string>={PLAY:'可进入',RESUME:'恢复本局',MAINTENANCE:'维护中',TEMPORARILY_UNAVAILABLE:'暂不可用',COMING_SOON:'即将开放',RETIRED:'已退役'};

function HallImage({asset,alt='',className,sizes,lazy=false}:{asset:ImageAsset;alt?:string;className?:string;sizes:string;lazy?:boolean}) {
    return <img key={asset.src} className={className} src={assetUrl(asset.src)} srcSet={assetSrcSet(asset)} sizes={sizes}
        width={asset.width} height={asset.height} alt={alt} loading={lazy?'lazy':'eager'} decoding="async"
        onError={event=>{event.currentTarget.style.visibility='hidden';}}/>;
}

function HallOrnament({name}:{name:keyof typeof hallArt.ornaments.cells}) {
    const atlas=hallArt.ornaments,cell=atlas.cells[name],scale=1/Math.max(cell.width,cell.height);
    return <span className="hall-ornament" aria-hidden="true"><span style={{
        width:cell.width*scale+'em',height:cell.height*scale+'em',
        backgroundImage:`url("${assetUrl(atlas.src)}")`,
        backgroundSize:`${atlas.width*scale}em ${atlas.height*scale}em`,
        backgroundPosition:`${-cell.x*scale}em ${-cell.y*scale}em`,
    }}/></span>;
}

function catalogDestination(game:GameEntry):string|undefined {
    const rouletteRoutes:Record<string,string>={'devil-roulette':'roulette.devil.v1','pressure-roulette':'roulette.pressure.v1'};
    if(rouletteRoutes[game.slug]===game.implementation_key&&(game.effective_runtime==='PLAY'||game.effective_runtime==='MAINTENANCE')) return '/roulette/'+game.slug;
    if(game.slug==='texas-holdem' && game.implementation_key==='poker.texas-holdem.v1'
        && (game.effective_runtime==='PLAY'||game.effective_runtime==='MAINTENANCE')) return '/poker';
    if(gameSlugs.includes(game.slug as GameSlug) && game.implementation_key===`direct.${game.slug}.v1`
        && (game.effective_runtime==='PLAY' || game.slug==='blackjack' && game.effective_runtime==='MAINTENANCE')) return '/games/'+game.slug;
}
export function canEnterCatalogGame(game:GameEntry):boolean { return catalogDestination(game)!==undefined; }
function CatalogFilters({query,onApply,onClear}:{query:CatalogQuery;onApply:(query:CatalogQuery)=>void;onClear:()=>void}) {
    const [draft,setDraft]=useState(query),[error,setError]=useState('');
    function apply(event:FormEvent) {
        event.preventDefault();const result=catalogQuerySchema.safeParse(draft);
        if(!result.success){setError('搜索词最多 128 个字符，不能包含无效字符或控制字符。');return;}
        setError('');setDraft(result.data);onApply(result.data);
    }
    return <form className="game-catalog-filters" onSubmit={apply} aria-label="筛选游戏目录">
        <label>搜索游戏<input type="search" value={draft.q} onChange={e=>setDraft({...draft,q:e.target.value})} placeholder="游戏名称或标识"/></label>
        <label>运行状态<select value={draft.availability} onChange={e=>setDraft({...draft,availability:e.target.value as CatalogQuery['availability']})}>{catalogAvailability.map(state=><option key={state} value={state}>{state==='ALL'?'全部状态':runtimeNames[state]}</option>)}</select></label>
        <label>排序方式<select value={draft.sort} onChange={e=>setDraft({...draft,sort:e.target.value as CatalogQuery['sort']})}><option value="RECOMMENDED">推荐顺序</option><option value="NAME">名称顺序</option></select></label>
        <div><button type="submit">应用筛选</button><button type="button" onClick={()=>{setDraft(catalogQuerySchema.parse({}));setError('');onClear();}}>清除筛选</button></div>
        {error&&<Alert>{error}</Alert>}
    </form>;
}

export function GamesCatalog({client}:{client:ApiClient}) {
    const session=useSyncExternalStore(client.subscribe,client.getSnapshot);
    const location=useLocation(),navigate=useNavigate(),discovery=location.pathname==='/games';
    const search=discovery?location.search:'';
    let query:CatalogQuery|null=null;
    try{query=parseCatalogQuery(search);}catch{/* Invalid URLs stay visible and never trigger a catalog request. */}
    const catalog=useResource(async()=>query?parsePublicCatalog(await client.gameCatalog<unknown>(discovery?query:undefined)):null,[client,session.user?.id,search,discovery]);
    const clear=()=>navigate({pathname:location.pathname,search:''});
    return <div className="games-catalog game-hall"><a className="skip-link" href="#games-catalog-content">跳至主要内容</a>
        <header className="portal-header">
            <Link to="/" className="brand hall-brand" aria-label="Chaldea Platform 首页"><span>CHALDEA</span><small>MOMIAO</small></Link>
            <nav className="portal-global" aria-label="主导航"><Link to="/">首页</Link><Link to="/models">模型目录</Link><Link to="/entertainment" aria-current="page">娱乐</Link><Link to="/announcements">公告</Link></nav>
            <Link className="button hall-account" to={session.user?'/me':'/login'}>{session.user?'个人中心':'登录账户'}</Link>
        </header>
        <main id="games-catalog-content">
            <section className="entertainment-hero">
                <HallImage asset={hallArt.hero.background} className="hall-backdrop" sizes="(max-width: 1440px) 100vw, 1440px"/>
                <HallImage asset={hallArt.hero.character} className="hall-character" alt="爱尔奎特·布伦史塔德，微笑着倚坐在星夜会馆" sizes="(max-width: 720px) 230px, 390px"/>
                <div className="entertainment-hero-copy"><p className="eyebrow">CHALDEA / GAME LOBBY</p><h1>游戏大厅</h1><p className="hall-subtitle"><span>留一点时间，</span><span>给意外的惊喜。</span></p><a className="button primary" href="#game-directory">探索游戏 <span aria-hidden="true">→</span></a></div>
                <p className="hall-character-name" aria-hidden="true">Arcueid<br/>Brunestud</p>
            </section>
        <section className="game-directory" id="game-directory"><header className="section-heading"><h2>今晚，想玩些什么？</h2><HallOrnament name="star"/><span className="hall-rule" aria-hidden="true"/><p className="hall-motto">GOOD GAMES, A BRIGHTER TOMORROW.</p>{session.user&&<Link to="/history" className="text-link">我的游戏记录 →</Link>}</header>
        {discovery&&<CatalogFilters key={search} query={query??catalogQuerySchema.parse({})} onApply={value=>navigate({pathname:location.pathname,search:serializeCatalogQuery(value)})} onClear={clear}/>}
        {!query?<Alert>目录筛选参数无效，请清除筛选后重试。</Alert>:catalog.loading?<p className="loading" role="status">正在读取游戏目录…</p>:catalog.error?<><Alert>游戏目录暂时无法读取，请重试。</Alert><button onClick={catalog.reload}>重新读取游戏目录</button></>:catalog.data&&<>
        {discovery&&<p className="game-catalog-count" role="status">找到 {catalog.data.items.length} 个游戏</p>}
        {catalog.data.items.length===0?<Empty title="没有匹配的游戏">试试其他名称或状态，也可以清除筛选查看全部游戏。</Empty>:<div className="game-catalog-grid">{catalog.data.items.map((game,i)=>{
            const art=gameArt(game.slug);
            return <article key={game.slug} className={'game-catalog-item game-'+game.slug}>
                {art&&<div className="game-item-visual" aria-hidden="true"><HallImage asset={art} sizes="(max-width: 720px) calc(100vw - 40px), (max-width: 1100px) calc((100vw - 72px) / 2), 360px" lazy={i>=3}/></div>}
                <div className="game-item-body"><h3>{game.title}</h3><p className="hall-description">{game.slug==='texas-holdem'?'浏览公开牌桌，或回到原会话继续你的牌局。':gameSlugs.includes(game.slug as GameSlug)?gameCopy[game.slug as GameSlug].intro:'新的沙龙正在准备，敬请期待。'}</p><div>
                    <span className={'hall-status state-'+game.effective_runtime}>{runtimeNames[game.effective_runtime]||'状态待核对'}</span>
                    {canEnterCatalogGame(game)?<Link to={catalogDestination(game)!} className="text-link">{game.slug==='texas-holdem'?(game.effective_runtime==='PLAY'?'进入 Poker 大厅':'查看大厅与恢复牌局'):(game.effective_runtime==='PLAY'?'进入游戏':'查看与恢复牌局')} →</Link>:<span className="game-entry-disabled">{game.effective_runtime==='PLAY'?'暂不可用':runtimeNames[game.effective_runtime]||'暂不可用'}</span>}
                </div></div>
            </article>;
        })}</div>}</>}
        <nav className="game-catalog-links" aria-label="游戏服务"><Link to="/history"><HallOrnament name="history"/>游戏记录</Link><Link to="/rewards"><HallOrnament name="rewards"/>每日奖励</Link><Link to="/wallet"><HallOrnament name="wallet"/>筹码钱包</Link><Link to="/rankings"><HallOrnament name="rankings"/>排行榜</Link></nav>
        <p className="entertainment-note">筹码来自免费奖励与已有额度兑换。不提供购买、转赠或交易。</p>
        </section></main><footer className="workspace-foot"><span>CHALDEA / ENTERTAINMENT</span><span>愿每一局，都有值得记住的瞬间。</span></footer></div>;
}

export function GamePage({client,userID,slug}:{client:ApiClient;userID:string;slug:GameSlug}) {
    const navigate=useNavigate();
    const [bootstrap,setBootstrap]=useState<GameBootstrap>();
    const [round,setRound]=useState<GameRound|null>(null);
    const [diceAnimating,setDiceAnimating]=useState(false);
    const [busy,setBusy]=useState(false),[loading,setLoading]=useState(true),[notice,setNotice]=useState('');
    const [pending,setPending]=useState<PendingGame|null>(null),[retryReady,setRetryReady]=useState(false),[storageBlocked,setStorageBlocked]=useState(false);
    const [pendingAction,setPendingAction]=useState<PendingBlackjackAction|null>(null),[actionRetryReady,setActionRetryReady]=useState(false);
    const [wager,setWager]=useState('10'),[choice,setChoice]=useState<'BIG'|'SMALL'|null>(null),[mode,setMode]=useState<'SINGLE'|'TENFOLD'>('SINGLE');
    const live=useRef(true),lock=useRef(false),loadVersion=useRef(0),hydratedInput=useRef(false),roundAction=useRef<{round:string;id:string}|null>(null);
    const generation=client.getSessionGeneration();
    const current=()=>live.current&&client.getSessionGeneration()===generation&&String(client.getSnapshot().user?.id)===userID&&!client.getSnapshot().loggingOut;
    async function load(reconcile:PendingGame|null=null,reconcileAction:PendingBlackjackAction|null=null) {
        const version=++loadVersion.current;setLoading(true);setRetryReady(false);setActionRetryReady(false);
        try {
            const recovered=reconcile?await findPendingGame(client,slug,reconcile.key):null;
            const recoveredAction=reconcileAction?await findBlackjackAction(client,reconcileAction):null;
            const b=await readGameBootstrap(client,slug);if(!current()||version!==loadVersion.current)return;
            setBootstrap(b);setRound(b.active_round||b.latest_round);
            if(!hydratedInput.current){
                const input=reconcile?.input||b.latest_round?.input;
                if(input?.type===slug.toUpperCase()){
                    const value=input.type==='SUMMON'?input.base_wager:input.type==='SLOT'?input.total_wager:input.type==='BLACKJACK'?input.initial_wager:input.wager;
                    const savedMode=input.type==='SUMMON'?input.mode:'SINGLE';
                    if(wagerCost(value,savedMode,slug)!==null){setWager(value);if(input.type==='DICE')setChoice(input.choice);if(input.type==='SUMMON')setMode(input.mode);}
                }
                hydratedInput.current=true;
            }
            if(reconcile){
                if(recovered){if(slug!=='blackjack')setRound(recovered);setPending(null);sessionStorage.removeItem(pendingStorage(userID,slug));setNotice('已从历史恢复原局，没有再次下注。');}
                else if(b.next_commitment?.id===reconcile.commitment){setPending(null);sessionStorage.removeItem(pendingStorage(userID,slug));setNotice('服务器确认原下注未受理，已解除锁定。');}
                else{setPending(reconcile);setRetryReady(true);setNotice('尚未找到已受理的局。可以使用原请求重试，核对前不会生成新请求。');}
            }
            if(reconcileAction){
                if(recoveredAction){sessionStorage.removeItem(blackjackActionStorage(userID));setPendingAction(null);setNotice('原行动已受理，已恢复最新牌局。');}
                else{setPendingAction(reconcileAction);setActionRetryReady(true);setNotice('尚未找到原行动。可以用同一行动编号重试，期间保持锁定。');}
            }
        }catch(error){if(current()){setBootstrap(undefined);setNotice(gameError(error));}}
        finally{if(current()&&version===loadVersion.current)setLoading(false);}
    }
    useEffect(()=>{
        live.current=true;let stored:PendingGame|null=null,action:PendingBlackjackAction|null=null;
        try{stored=readPending(userID,slug);setPending(stored);if(slug==='blackjack'){action=readBlackjackAction(userID);setPendingAction(action);}}catch(error){setStorageBlocked(true);setNotice(gameError(error));}
        void load(stored,action);return()=>{live.current=false;loadVersion.current++;};
    },[client,userID,slug]);
    const revealIncomplete=slug==='scratch'&&round?.scratch&&!round.presentation_completed_at;
    const cost=wagerCost(wager,slug==='summon'?mode:'SINGLE',slug);
    const available=bootstrap?units(revealIncomplete&&round?round.balance_before_units:bootstrap.available_units):null;
    const invalid=slug==='dice'&&!choice?'请先选择大或小。':cost===null?'最低下注 10 筹码，只接受整数筹码。':available!==null&&cost>available?'可用筹码不足。':'';
    const actionBlocked=busy||loading||!!pending||!!pendingAction||storageBlocked||!bootstrap||round?.recovery_state==='NEEDS_REVIEW';
    const activeBlackjack=slug==='blackjack'&&round?.state==='PLAYER_TURN';
    const blocked=(slug==='dice'&&diceAnimating)||actionBlocked||bootstrap?.game.effective_runtime!=='PLAY'||!bootstrap?.next_commitment||!!revealIncomplete||activeBlackjack;
    async function play(original?:PendingGame) {
        if(lock.current||!bootstrap||(original?(!retryReady||loading):blocked||!!invalid))return;
        lock.current=true;setBusy(true);setNotice('');setRetryReady(false);
        try{
            let request=original;
            if(!request){if(slug==='dice'&&choice===null)return;const input:GameInput=slug==='dice'?{type:'DICE',wager,choice:choice!}:slug==='scratch'?{type:'SCRATCH',wager}:slug==='slot'?{type:'SLOT',total_wager:wager}:slug==='blackjack'?{type:'BLACKJACK',initial_wager:wager}:{type:'SUMMON',base_wager:wager,mode};request={key:gameUUID(),commitment:bootstrap.next_commitment!.id,input};}
            // Only recovery identities and typed input persist, never auth or seed keys.
            try{sessionStorage.setItem(pendingStorage(userID,slug),JSON.stringify(request));}
            catch{setStorageBlocked(true);setNotice('浏览器无法保存本局恢复标识，尚未提交下注。请允许本网站使用会话存储后刷新。');return;}
            setPending(request);
            const result=await createGame(client,slug,request,bootstrap.csrf_token);if(!current())return;
            setRound(result);sessionStorage.removeItem(pendingStorage(userID,slug));setPending(null);
            await load();
        }catch(error){if(current()){
            if(error instanceof ApiError&&!error.uncertain&&error.status>=400&&error.status<500){sessionStorage.removeItem(pendingStorage(userID,slug));setPending(null);await load();setNotice(gameError(error));}
            else{setNotice('本次请求结果尚未确认。请核对本局，确认前暂停新下注。'+gameError(error));}
        }}finally{if(current()){lock.current=false;setBusy(false);}}
    }
    async function completeScratch() {
        if(lock.current||!round||!bootstrap||round.presentation_completed_at)return;
        lock.current=true;setBusy(true);setNotice('');
        try{
            if(roundAction.current?.round!==round.id)roundAction.current={round:round.id,id:gameUUID()};
            const next=parseRound(await client.request(`/api/v1/game-rounds/${round.id}/actions`,'POST',{action_id:roundAction.current.id,action_type:'SCRATCH_REVEAL_COMPLETE'},{'X-CSRF-Token':bootstrap.csrf_token}));
            if(!current())return;setRound(next);await load();
        }catch(error){if(current())setNotice('揭晓状态尚未确认，刷新即可恢复本张；不会再次购买。'+gameError(error));}
        finally{if(current()){lock.current=false;setBusy(false);}}
    }
    async function performAction(command:BlackjackCommand,original?:PendingBlackjackAction){
        if(lock.current||!bootstrap||!round||(original?(!actionRetryReady||loading):actionBlocked||!activeBlackjack))return;
        lock.current=true;setBusy(true);setNotice('');setActionRetryReady(false);
        const request=original||{round:round.id,input:{...command,action_id:gameUUID()}};
        try{
            try{sessionStorage.setItem(blackjackActionStorage(userID),JSON.stringify(request));}catch{setStorageBlocked(true);setNotice('无法保存行动恢复标识，尚未提交行动。请允许会话存储后刷新。');return;}
            setPendingAction(request);
            const result=parseRound(await client.request(`/api/v1/game-rounds/${request.round}/actions`,'POST',request.input,{'X-CSRF-Token':bootstrap.csrf_token}));
            if(result.id!==request.round||result.game!=='blackjack')throw new ApiError('行动响应尚未核对。',0,'',true);
            if(!current())return;sessionStorage.removeItem(blackjackActionStorage(userID));setPendingAction(null);await load();
        }catch(error){if(current()){
            if(error instanceof ApiError&&!error.uncertain&&error.status>=400&&error.status<500){sessionStorage.removeItem(blackjackActionStorage(userID));setPendingAction(null);await load();setNotice(gameError(error));}
            else setNotice('行动结果尚未确认，请先核对本次行动。'+gameError(error));
        }}finally{if(current()){lock.current=false;setBusy(false);}}
    }
    const copy=gameCopy[slug];
    const salonLayout=slug==='dice'||slug==='scratch';
    const StageLayout=salonLayout?'div':Fragment;
    return <div className={'direct-game game-'+slug}>
        <header className="game-page-heading"><div><p className="eyebrow">CHALDEA / {copy.eyebrow}</p><h1>{gameNames[slug]}</h1><p>{copy.intro}</p></div><div className="game-heading-links"><Link to="/games">所有游戏 ↗</Link><Link to="/history">游戏记录</Link><button onClick={()=>void load(pending,pendingAction)} disabled={busy||loading}>刷新恢复</button></div></header>
        {notice&&<Alert>{notice}</Alert>}
        {pending&&<div className="game-recovery" role="status"><p>有一笔下注等待核对，新下注已暂停。</p><button onClick={()=>void load(pending)} disabled={busy||loading}>核对本局</button>{retryReady&&<button onClick={()=>void play(pending)} disabled={busy}>使用原请求重试</button>}</div>}
        {pendingAction&&<div className="game-recovery" role="status"><p>有一次行动等待核对，其他行动已暂停。</p><button disabled={busy||loading} onClick={()=>void load(pending,pendingAction)}>核对本次行动</button>{actionRetryReady&&<button disabled={busy||loading} onClick={()=>void performAction(pendingAction.input,pendingAction)}>重试原行动</button>}</div>}
        {round?.recovery_state==='NEEDS_REVIEW'&&<Alert>本局状态需要核对，已暂停行动。请保留本局编号并联系管理员。</Alert>}
        {slug==='blackjack'?<BlackjackGame snapshot={round?.blackjack||null} availableUnits={bootstrap?.available_units||null} wagerChips={wager} onWagerChange={setWager} busy={busy||loading} recovering={!!pending||!!pendingAction} disabledReason={activeBlackjack?(actionBlocked?'请先恢复当前状态。':null):(blocked?'请先恢复当前状态。':null)} roundID={round?.id} onDeal={async()=>{await play()}} onAction={command=>performAction(command)} onRecover={()=>void load(pending,pendingAction)} onWallet={()=>navigate('/wallet')} onRewards={()=>navigate('/rewards')} onHistory={round?()=>navigate('/history/rounds/'+round.id):undefined} onFairness={round?()=>navigate('/history/rounds/'+round.id):undefined}/>:null}
        {slug==='blackjack'?null:slug==='slot'?<SlotGame result={round?.slot||null} rulesetVersion={round?.ruleset_version||bootstrap?.game.config?.ruleset_version} availableUnits={bootstrap?.available_units||null} wagerChips={wager} onWagerChange={setWager} busy={busy||loading} recovering={!!pending} disabledReason={blocked?'请先恢复当前状态。':null} roundID={round?.id} onSpin={async()=>{await play()}} onRecover={()=>void load(pending)} onWallet={()=>navigate('/wallet')} onRewards={()=>navigate('/rewards')} onHistory={round?()=>navigate('/history/rounds/'+round.id):undefined} onFairness={round?()=>navigate('/history/rounds/'+round.id):undefined}/>:<StageLayout {...(salonLayout?{className:slug+'-play-layout'}:{})}><section className={'game-stage '+(busy?'game-busy':'')} aria-label="游戏舞台">
            <div className="game-stage-corners" aria-hidden="true"/>
            {slug==='dice'?<DiceStage key={userID} result={round?.dice} busy={busy} loading={loading} recovering={!!pending} onAnimatingChange={setDiceAnimating}/>:slug==='scratch'?<ScratchStage round={round} onComplete={()=>void completeScratch()} busy={busy}/>:<SummonStage round={round} busy={busy}/>}
        </section>
        <section className="game-console" aria-label="下注控制台"><div className="game-balance"><p>可用筹码</p><strong>{available===null?'—':chips(available.toString())}</strong><div><Link to="/wallet">钱包兑换 →</Link><Link to="/rewards">免费签到 →</Link></div></div>
            <form className="wager-form" onSubmit={e=>{e.preventDefault();void play()}}>
                <label>基础下注（筹码）<input inputMode="numeric" autoComplete="off" maxLength={19} value={wager} onChange={e=>setWager(e.target.value)} disabled={blocked} aria-describedby="wager-hint"/></label>
                <div className="wager-quick" aria-label="快捷下注">{['10','100','500','1000'].map(n=>{const quickCost=wagerCost(n,slug==='summon'?mode:'SINGLE',slug);return <button type="button" key={n} disabled={blocked||available===null||quickCost===null||quickCost>available} onClick={()=>setWager(n)}>{n}</button>})}</div>
                {slug==='dice'&&<fieldset className="game-choice" disabled={blocked}><legend>选择大小</legend>{(['SMALL','BIG'] as const).map(side=><label className={choice===side?'selected':''} key={side}><input type="radio" name="dice-choice" checked={choice===side} onChange={()=>setChoice(side)}/><strong>{side==='SMALL'?'小':'大'}</strong><span>{side==='SMALL'?'4–10 点':'11–17 点'}</span></label>)}</fieldset>}
                {slug==='summon'&&<fieldset className="summon-mode" disabled={blocked}><legend>召唤方式</legend>{(['SINGLE','TENFOLD'] as const).map(value=><label key={value}><input type="radio" name="summon-mode" checked={mode===value} onChange={()=>setMode(value)}/>{value==='SINGLE'?'单次召唤 · 1 抽':'十连召唤 · 10 抽'}</label>)}</fieldset>}
                <p className={'wager-hint '+(invalid?'invalid':'')} id="wager-hint">{invalid||'最低 10 筹码 · 整数步长 1 筹码'}</p>
                <div className="wager-cost"><span>本次总消耗 <strong>{cost===null?'—':chips(cost.toString())}</strong></span><span>最低预估余额 <strong>{cost!==null&&available!==null&&cost<=available?chips((available-cost).toString()):'—'}</strong></span></div>
                <button className="primary game-play" type="submit" disabled={blocked||!!invalid}>{busy?'正在核对…':slug==='dice'?(diceAnimating?'落定中…':'掷骰'):slug==='scratch'?'购买刮刮卡':`${mode==='TENFOLD'?'十连召唤':'单次召唤'} · ${cost===null?'—':chips(cost.toString())} 筹码`}</button>
                {revealIncomplete&&<p className="hint">请先完成上方刮卡揭晓，再购买下一张。</p>}
                {bootstrap&&bootstrap.game.effective_runtime!=='PLAY'&&<p className="hint">{runtimeNames[bootstrap.game.effective_runtime]||'暂不可用'}，历史结果仍可查阅。</p>}
            </form>
        </section>
        </StageLayout>}{loading&&!bootstrap&&<Loading/>}
        {round&&!revealIncomplete&&!(slug==='dice'&&(busy||diceAnimating||pending))&&<RoundReceipt round={round}/> }
        <div className="game-information">{slug==='slot'||slug==='blackjack'?<ExtraRules config={bootstrap?.game.config} blackjack={slug==='blackjack'}/>:<GameRules slug={slug} config={bootstrap?.game.config}/>} {bootstrap&&<FairnessControl client={client} slug={slug} bootstrap={bootstrap} disabled={blocked} onChanged={()=>void load()}/>}</div>
    </div>;
}

function ExtraRules({config,blackjack=false}:{config?:GameConfig;blackjack?:boolean}){
    if(blackjack)return <section className="game-rules"><p className="eyebrow">FROZEN RULES</p><h2>固定规则</h2><p>固定六副牌、S17、Natural 3:2；加倍与分牌会增加整局下注。结算追加规则规定的公平返还，总派彩减全部下注才是本局净变化。</p><p>具体操作会改变个人结果；牌桌不提供策略推荐。完整规则、结果与可复算输入保存在本局详情。</p><small>{config?.ruleset_version&&`规则版本 ${config.ruleset_version}`}</small></section>;
    return <section className="game-rules"><p className="eyebrow">FROZEN RULES</p><h2>固定规则</h2><p>总下注平均分配给固定的 10 条线。中奖线的返还相加，再减总下注，得到整局净变化；部分返还仍计作净输。</p><p>卷轴、中奖线和本局使用的奖励倍率可在奖表及公平详情中核对。</p><small>{config?.ruleset_version&&`规则版本 ${config.ruleset_version}`}</small></section>
}
function GameRules({slug,config}:{slug:GameSlug;config?:GameConfig}) {
    return <section className="game-rules"><p className="eyebrow">KNOW YOUR GAME</p><h2>规则与奖励</h2>
        {slug==='dice'?<p>三颗六面骰。小为 4–10 点，大为 11–17 点。任意豹子（3 颗相同）返还本次下注；猜中总派彩为下注的 2 倍，普通未中派彩为 0。</p>:slug==='scratch'?<p>九宫格出现 3 枚相同功能星纹，获得对应倍数的总派彩；未匹配时派彩为 0。结果在购买时确定，刮擦和立即揭晓只改变展示。</p>:<p>单抽与十连使用相同奖池。十连总消耗为基础下注的 10 倍，每抽独立，没有保底或共享修正；整轮结果按总派彩减总消耗计算。</p>}
        <p>总派彩包含本金，净变化 = 总派彩 − 总消耗。回本不计作净赢。</p>
        {config&&<>{config.prizes&&<table aria-label="完整奖励表"><thead><tr><th>等级</th><th>总派彩倍数</th></tr></thead><tbody>{config.prizes.map(p=><tr key={p.tier}><td>{p.tier}</td><td>×{p.multiplier}</td></tr>)}</tbody></table>}<small>规则版本 {config.ruleset_version} · 奖励表与当前配置一致。</small></>}
    </section>;
}
function FairnessControl({client,slug,bootstrap,disabled,onChanged}:{client:ApiClient;slug:GameSlug;bootstrap:GameBootstrap;disabled:boolean;onChanged:()=>void}) {
    const [seed,setSeed]=useState(bootstrap.client_seed_preference.client_seed),[busy,setBusy]=useState(false),[error,setError]=useState('');
    const active=useRef(true);useEffect(()=>{active.current=true;return()=>{active.current=false}},[]);
    useEffect(()=>setSeed(bootstrap.client_seed_preference.client_seed),[bootstrap.client_seed_preference.version]);
    async function save(e:FormEvent) {e.preventDefault();if(busy||disabled)return;setBusy(true);setError('');try{await client.request<Commitment>(`/api/v1/games/${slug}/client-seed`,'PUT',{client_seed:seed},{'X-CSRF-Token':bootstrap.csrf_token});if(active.current)onChanged()}catch(e){if(active.current)setError(gameError(e))}finally{if(active.current)setBusy(false)}}
    return <section className="game-fairness"><p className="eyebrow">PROVABLY FAIR</p><h2>开局前的承诺</h2><p>服务器已承诺下一局的种子哈希。结算后公开原种子，你可以在局详情中复算结果。</p>
        {bootstrap.next_commitment?<><p className="fairness-hash-label">下一局 Server Seed Hash</p><code className="fairness-hash">{bootstrap.next_commitment.server_seed_hash}</code><p className="hint">Nonce {bootstrap.next_commitment.nonce} · {bootstrap.next_commitment.algorithm_version}</p></>:<p className="hint">完成当前局或恢复游戏状态后，将读取下一份预承诺。</p>}
        <details><summary>自定义 Client Seed</summary><form onSubmit={e=>void save(e)}><label>Client Seed<input value={seed} onChange={e=>setSeed(e.target.value)} disabled={disabled||busy} maxLength={128}/></label><p className="hint">1–128 UTF-8 字节，不能包含控制字符；只影响下一局。</p><button disabled={disabled||busy||!seed}>{busy?'正在更新…':'更新下一局种子'}</button></form>{error&&<Alert>{error}</Alert>}</details>
        {bootstrap.game.config&&<small>配置版本 {bootstrap.game.config.version_id}</small>}
    </section>;
}
export function RoundReceipt({round}:{round:GameRound}) {
    if(round.state!=='SETTLED')return <section className="round-receipt" aria-label="本局进行中"><div><p className="eyebrow">PLAYER TURN / 尚未结算</p><h2>{round.recovery_state==='NEEDS_REVIEW'?'本局等待核对':'继续当前牌局'}</h2><p>已投入 {chips(round.total_stake_units)} 筹码，最终派彩与净变化将在结算后显示。</p><Link className="button" to="/games/blackjack">返回牌桌恢复本局 →</Link></div></section>;
    const d=round.dice;
    return <section className={'round-receipt result-'+round.common_result} aria-label="本局结果"><div><p className="eyebrow">SETTLED / 本局已结算</p><h2>{outcomeNames[round.common_result||'LOSS']} <strong>{chips(round.net_change_units,true)}</strong></h2>
        {d&&<p>{d.dice.join('、')} · 合计 {d.total} 点 · 实际{d.triple?'豹子':d.side==='BIG'?'大':'小'} · 选择{d.choice==='BIG'?'大':'小'}{d.triple?'（返还下注）':''}</p>}
        {round.scratch&&<p>{round.scratch.reward.payout_multiplier==='0'?'未组成三枚相同星纹':`匹配三枚星纹 · 总派彩 ×${round.scratch.reward.payout_multiplier}`}</p>}
        {round.summon&&<p>{round.summon.draws.length} 抽已结算 · 最高等级 {round.summon.highest_tier} · 以整轮净变化判定结果</p>}
        <Link className="text-link" to={'/history/rounds/'+round.id}>查看本局详情与公平验证 →</Link></div>
        <dl><div><dt>总消耗</dt><dd>{chips(round.total_stake_units)}</dd></div><div><dt>总派彩</dt><dd>{chips(round.total_payout_units)}</dd></div><div><dt>结算后余额</dt><dd>{chips(round.balance_after_units)}</dd></div></dl>
    </section>;
}

export function GameHistory({client}:{client:ApiClient}) {
    const [game,setGame]=useState(''),[cursors,setCursors]=useState(['']);const cursor=cursors[cursors.length-1];
    const history=useResource(async()=>{const q=new URLSearchParams();if(game)q.set('game',game);if(cursor)q.set('before',cursor);const p=await client.request<{items:unknown[];next_cursor?:string}>('/api/v1/game-rounds?'+q);return{items:p.items.map(parseRound),next_cursor:p.next_cursor}},[client,game,cursor]);
    return <div className="game-history"><header className="page-heading"><div><p className="eyebrow">YOUR MOMENTS / GAME HISTORY</p><h1>游戏记录</h1><p>每一局的消耗、派彩与结果，都留在这里。</p></div><Link to="/games" className="button">返回游戏目录 →</Link></header><section className="panel"><div className="section-heading"><label>筛选游戏<select value={game} onChange={e=>{setGame(e.target.value);setCursors([''])}}><option value="">全部游戏</option>{gameSlugs.map(slug=><option key={slug} value={slug}>{gameNames[slug]}</option>)}</select></label><button disabled={history.loading} onClick={history.reload}>刷新记录</button></div>
    {history.loading?<Loading/>:history.error?<Alert>{history.error}</Alert>:history.data?.items.length?<div className="table-wrap"><table aria-label="我的游戏记录"><thead><tr><th>游戏 / 时间</th><th>总消耗</th><th>总派彩</th><th>净变化</th><th>状态 / 详情</th></tr></thead><tbody>{history.data.items.map(r=><tr key={r.id}><td>{gameNames[r.game]}<small>{new Date(r.created_at).toLocaleString('zh-CN')}</small></td><td>{chips(r.total_stake_units)}</td><td>{r.state==='SETTLED'?chips(r.total_payout_units):'待结算'}</td><td>{r.state==='SETTLED'?chips(r.net_change_units,true):'—'}<small>{r.common_result?outcomeNames[r.common_result]:'进行中'}</small></td><td><Link to={'/history/rounds/'+r.id}>{r.state==='SETTLED'?'已结算':'进行中'} · 查看本局 →</Link></td></tr>)}</tbody></table></div>:<Empty title="还没有游戏记录">手动开始一局后，已结算结果会保存在这里。</Empty>}
    <nav className="pager" aria-label="游戏历史分页"><button disabled={history.loading||cursors.length===1} onClick={()=>setCursors(c=>c.slice(0,-1))}>上一页</button><button disabled={history.loading||!history.data?.next_cursor} onClick={()=>setCursors(c=>[...c,history.data!.next_cursor!])}>下一页</button></nav></section></div>;
}
export function GameRoundDetail({client}:{client:ApiClient}) {
    const {id=''}=useParams();const r=useResource(()=>client.request(`/api/v1/game-rounds/${id}`).then(parseRound),[client,id]);
    const [verification,setVerification]=useState<GameVerification>(),[verifying,setVerifying]=useState(false),[error,setError]=useState('');
    async function verify(){setVerifying(true);setError('');try{const result=await client.request<GameVerification>(`/api/v1/game-rounds/${id}/fairness`);if(result.round_id!==id||typeof result.verified!=='boolean')throw new Error('验证响应尚未核对。');setVerification(result)}catch(e){setError(gameError(e))}finally{setVerifying(false)}}
    return <div className="game-round-detail"><header className="page-heading"><div><p className="eyebrow">ROUND / DETAILS</p><h1>本局详情</h1><p className="round-id">{id}</p></div><Link to="/history" className="button">← 返回游戏记录</Link></header>{r.loading?<Loading/>:r.error?<Alert>{r.error}</Alert>:r.data&&<><RoundReceipt round={r.data}/><section className="panel round-authority"><h2>结果明细</h2>{r.data.scratch&&<ol className="history-cells">{r.data.scratch.cells.map((c,i)=><li key={i}>第 {i+1} 格 · ×{c.symbol.slice(1)}{c.matching?' · 匹配星纹':''}</li>)}</ol>}{r.data.summon&&<ol className="history-cells">{r.data.summon.draws.map(d=><li key={d.index}>第 {d.index} 抽 · {d.tier} · ×{d.multiplier}</li>)}</ol>}
        {r.data.slot&&<SlotGame result={r.data.slot} rulesetVersion={r.data.ruleset_version} availableUnits={r.data.balance_after_units} wagerChips={r.data.input.type==='SLOT'?r.data.input.total_wager:'10'} onWagerChange={()=>{}} onSpin={async()=>{}} busy={false} disabledReason="历史局仅供回看。返回游戏页后才能创建新局。" roundID={r.data.id}/>}
        {r.data.blackjack&&<BlackjackGame snapshot={r.data.blackjack} availableUnits={r.data.balance_after_units} wagerChips={r.data.input.type==='BLACKJACK'?r.data.input.initial_wager:'10'} onWagerChange={()=>{}} onDeal={async()=>{}} onAction={async()=>{}} busy={false} disabledReason="历史快照仅供回看；返回牌桌恢复当前局。" roundID={r.data.id}/>}
        {r.data.game==='scratch'&&!r.data.presentation_completed_at&&<Link className="button" to="/games/scratch">恢复本张，完成揭晓 →</Link>}
        <dl><div><dt>游戏</dt><dd>{gameNames[r.data.game]}</dd></div><div><dt>结算时间</dt><dd>{r.data.settled_at?new Date(r.data.settled_at).toLocaleString('zh-CN'):'尚未结算'}</dd></div><div><dt>规则版本</dt><dd>{r.data.ruleset_version}</dd></div><div><dt>结果算法</dt><dd>{r.data.algorithm_version}</dd></div><div><dt>扣注流水事务</dt><dd>{r.data.wager_transaction_id}</dd></div><div><dt>结算流水事务</dt><dd>{r.data.settlement_transaction_id||'结算后产生'}</dd></div></dl></section><section className="panel round-verification"><p className="eyebrow">VERIFY THIS ROUND</p><h2>公平验证</h2><p>根据本局锁定的种子、Nonce、配置和算法复算，不生成新一局。</p><button className="primary" disabled={verifying} onClick={()=>void verify()}>{verifying?'正在复算…':'验证本局'}</button>{error&&<Alert>{error}</Alert>}{verification&&<div className="verification-result" role="status"><h3>{verification.reveal_state!=='REVEALED'?'本局尚未结算，种子将在结算后公开。':verification.verified?'验证通过：结果与原始输入一致':'验证未通过：请保留本局编号'}</h3><dl><div><dt>Server Seed Hash</dt><dd><code>{verification.server_seed_hash}</code></dd></div><div><dt>公开 Server Seed</dt><dd><code>{verification.server_seed}</code></dd></div><div><dt>Client Seed</dt><dd><code>{verification.commitment.client_seed}</code></dd></div><div><dt>Nonce</dt><dd>{verification.commitment.nonce}</dd></div><div><dt>随机流版本</dt><dd>{verification.commitment.fairness_stream_version}</dd></div><div><dt>配置哈希</dt><dd><code>{verification.commitment.config_hash}</code></dd></div></dl><details><summary>查看完整可复算输入</summary><pre>{JSON.stringify(verification.commitment,null,2)}</pre><pre>{verification.canonical_config}</pre>{verification.blackjack_audit!==undefined&&<pre>{JSON.stringify(verification.blackjack_audit,null,2)}</pre>}</details></div>}</section></>}</div>;
}
