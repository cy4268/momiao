import { useEffect, useRef, useState, type CSSProperties } from 'react';
import type { GameRound } from '../games-api';
import { assetSrcSet, assetUrl } from '../game-hall-assets';
import art from './summon-art.json';
import './summon-stage.css';

// Fixed visual identities only. A server draw still supplies its tier, index and multiplier.
const servants:Record<string,typeof art.servants.T0>=art.servants;

export function SummonStage({round,animateRoundID,busy,loading,recovering}:{round:GameRound|null;animateRoundID:string|null;busy:boolean;loading:boolean;recovering:boolean}) {
    const [reveal,setReveal]=useState<{id:string;phase:'waiting'|'turning'|'done'}|null>(null);
    const seen=useRef<string|null>(null),status=useRef<HTMLParagraphElement>(null);
    const result=!busy&&!loading&&!recovering?round?.summon:undefined;
    const phase=reveal?.id===round?.id?reveal?.phase:animateRoundID&&animateRoundID===round?.id?'waiting':'done';
    const revealed=!!result&&phase==='done';
    useEffect(()=>{
        if(!result||!round)return;
        const id=round.id,animate=seen.current!==id&&animateRoundID===id;
        seen.current=id;
        // Only a freshly submitted round animates; recovered history is already a summary.
        if(!animate||window.matchMedia?.('(prefers-reduced-motion: reduce)').matches){setReveal({id,phase:'done'});return;}
        setReveal({id,phase:'waiting'});
        const delay=result.mode==='TENFOLD'?2200:1400;
        // Presentation only. Completion includes the final card's 550ms turn and stagger.
        const turn=setTimeout(()=>setReveal(value=>value?.id===id&&value.phase==='waiting'?{id,phase:'turning'}:value),delay);
        const done=setTimeout(()=>setReveal(value=>value?.id===id&&value.phase!=='done'?{id,phase:'done'}:value),delay+550+(result.draws.length-1)*65);
        return()=>{clearTimeout(turn);clearTimeout(done);};
    },[round?.id,animateRoundID,busy,loading,recovering,result?.mode]);

    return <div className={'summon-scene'+(busy||result&&phase==='waiting'?' is-summoning':'')}>
        <img className="summon-room" src={assetUrl(art.stage.src)} srcSet={assetSrcSet(art.stage)} sizes="100vw" width={art.stage.width} height={art.stage.height} alt=""/>
        {result?<ol className={'spirit-cards '+(result.mode==='TENFOLD'?'tenfold':'single')+(phase!=='waiting'?' is-revealed':'')+(revealed?' is-complete':'')} aria-label="本轮召唤结果">
            {result.draws.map((draw,i)=>{
                const servant=servants[draw.tier];
                return <li key={draw.index} className={'spirit-card tier-'+draw.tier} style={{'--card-delay':`${i*65}ms`} as CSSProperties}>
                    <div className="spirit-back" aria-hidden="true"><img src={assetUrl(art.back.src)} alt=""/></div>
                    <div className="spirit-front" aria-hidden={!revealed}>
                        {servant&&<img className="spirit-portrait" src={assetUrl(servant.src)} srcSet={assetSrcSet(servant)} sizes={result.mode==='TENFOLD'?'(max-width:600px) 46vw, 18vw':'(max-width:600px) 80vw, 420px'} width={servant.width} height={servant.height} alt={servant.name} onError={e=>{e.currentTarget.style.visibility='hidden'}}/>}
                        {servant&&<span className="spirit-rarity" role="img" aria-label={`${servant.stars} 星`}>{servant.stars===0?<span>无星</span>:Array.from({length:servant.stars},(_,n)=><img key={n} src={assetUrl(art.star.src)} alt=""/>)}</span>}
                        <div className="spirit-caption"><small>第 {draw.index} 抽</small><h2>{servant?.name??'未收录灵基'}</h2>{servant&&<span>{servant.className}</span>}<p><b>{draw.tier}</b><strong>×{draw.multiplier}</strong></p></div>
                        <img className="spirit-frame" src={assetUrl(art.frame.src)} alt=""/>
                    </div>
                </li>;
            })}
        </ol>:<div className="summon-idle"><img src={assetUrl(art.back.src)} srcSet={assetSrcSet(art.back)} sizes="(max-width:600px) 80vw, 360px" width={art.back.width} height={art.back.height} alt="待召唤的灵基卡背"/></div>}
        <div className="summon-status"><p ref={status} role="status" tabIndex={-1}>{recovering?'本局等待核对，暂不展示新卡面。':loading?'正在恢复召唤状态…':busy?'正在连接召唤阵…':result?revealed?`本轮 ${result.draws.length} 抽已揭晓 · 倍率以本局结果为准。`:'灵基显现中…':'等待召唤 · 每一次星光，独立回应。'}</p>
            {result&&!revealed&&<button type="button" onClick={()=>{setReveal({id:round!.id,phase:'done'});status.current?.focus();}}>全部揭晓 · 跳过演出</button>}
        </div>
        <details className="summon-roster"><summary>固定卡面 · 0—5 星</summary><dl>{Object.entries(servants).map(([tier,s])=><div key={tier}><dt>{tier} · {s.stars} 星</dt><dd>{s.name}</dd></div>)}</dl><p>星级仅作卡面展示，不改变奖励倍率，也没有额外保底。</p></details>
    </div>;
}
