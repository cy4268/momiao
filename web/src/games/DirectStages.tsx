import { useEffect, useState } from 'react';
import type { GameRound } from '../games-api';

export { DiceStage } from './DiceStage';

export { ScratchStage } from './ScratchStage';

export function SummonStage({round,busy}:{round:GameRound|null;busy:boolean}) {
    const [revealed,setRevealed]=useState(true);
    useEffect(()=>{if(!round?.summon)return;setRevealed(false);const t=setTimeout(()=>setRevealed(true),round.summon.mode==='TENFOLD'?2200:1400);return()=>clearTimeout(t)},[round?.id]);
    const result=round?.summon;
    return <div className={'manifestation-theatre '+(busy?'preparing':'')+(revealed?' revealed':'')}>
        <div className="manifestation-array" aria-hidden="true"><i/><i/><span>✦</span></div>
        {result?<><div className={'summon-tiles '+(result.mode==='SINGLE'?'single':'')} aria-label="本轮召唤结果">{result.draws.map((draw,i)=><div key={draw.index} className={'summon-tile tier-'+draw.tier} style={{animationDelay:revealed?'0ms':`${i*110}ms`}}><small>第 {draw.index} 抽</small><span aria-hidden="true">{draw.tier==='T5'?'✺':draw.tier==='T4'?'✵':'✧'}</span><strong>{draw.tier}</strong><p>×{draw.multiplier}</p></div>)}</div>{!revealed&&<button className="summon-skip" onClick={()=>setRevealed(true)}>全部揭晓 · 跳过演出</button>}</>:<div className="manifestation-core"><span aria-hidden="true">◇</span><p>{busy?'正在连接召唤阵…':'让星光，为这一刻回应。'}</p></div>}
        <span className="stage-serial" aria-hidden="true">GRAND MANIFESTATION THEATRE</span>
    </div>;
}
