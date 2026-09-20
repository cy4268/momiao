import { useEffect, useRef, useState, type PointerEvent } from 'react';
import type { GameRound } from '../games-api';

export { DiceStage } from './DiceStage';

export function ScratchStage({round,onComplete,busy}:{round:GameRound|null;onComplete:()=>void;busy:boolean}) {
    const canvas=useRef<HTMLCanvasElement>(null);const drawing=useRef(false);const previous=useRef<{x:number;y:number}|null>(null);
    const completed=!!round?.presentation_completed_at;
    const [revealed,setRevealed]=useState<number[]>([]);const revealedRef=useRef<number[]>([]);
    useEffect(()=>{
        setRevealed([]);revealedRef.current=[];drawing.current=false;previous.current=null;const ctx=canvas.current?.getContext('2d');if(!ctx||!round?.scratch||completed)return;
        ctx.globalCompositeOperation='source-over';const fill=ctx.createLinearGradient(0,0,600,330);fill.addColorStop(0,'#dac9a0');fill.addColorStop(.5,'#fff5d9');fill.addColorStop(1,'#beaa7d');ctx.fillStyle=fill;ctx.fillRect(0,0,600,330);
        ctx.strokeStyle='#a18d68';ctx.lineWidth=1;for(let x=-300;x<900;x+=26){ctx.beginPath();ctx.moveTo(x,0);ctx.lineTo(x+330,330);ctx.stroke()}
        ctx.fillStyle='#51493c';ctx.textAlign='center';ctx.font='28px serif';ctx.fillText('CHALDEA · TREASURE VOUCHER',300,155);ctx.font='20px sans-serif';ctx.fillText('按住拖动 · 刮开星纹',300,194);
    },[round?.id,completed]);
    function scratch(e:PointerEvent<HTMLCanvasElement>) {
        if(!drawing.current||busy||completed)return;const node=canvas.current,ctx=node?.getContext('2d');if(!node||!ctx)return;
        const bounds=node.getBoundingClientRect(),point={x:(e.clientX-bounds.left)*600/bounds.width,y:(e.clientY-bounds.top)*330/bounds.height};
        ctx.globalCompositeOperation='destination-out';ctx.lineWidth=54;ctx.lineCap='round';ctx.beginPath();ctx.moveTo(previous.current?.x??point.x,previous.current?.y??point.y);ctx.lineTo(point.x,point.y);ctx.stroke();previous.current=point;
    }
    function finish() {
        drawing.current=false;previous.current=null;const ctx=canvas.current?.getContext('2d');if(!ctx||completed||busy)return;
        const next=[...revealedRef.current];
        for(let cell=0;cell<9;cell++){
            if(next.includes(cell))continue;
            const x=(cell%3)*200,y=Math.floor(cell/3)*110;
            const pixels=ctx.getImageData(x,y,200,110).data;let erased=0,total=0;
            for(let i=3;i<pixels.length;i+=64){total++;if(pixels[i]<32)erased++}
            if(erased/total>=0.65){next.push(cell);ctx.clearRect(x,y,200,110)}
        }
        const allNew=next.length===9&&revealedRef.current.length!==9;
        revealedRef.current=next;setRevealed(next);if(allNew)onComplete();
    }
    return <div className="scratch-salon">
        <div className={'treasure-voucher '+(round?.scratch?'purchased':'')}>
            <div className="voucher-heading"><span>CHALDEA TREASURY</span><span>✦ 星纹凭证 ✦</span></div>
            <div className="scratch-surface">
                {round?.scratch?<div className="scratch-cells" aria-label="本张九宫格">{round.scratch.cells.map((cell,i)=><div key={i} aria-hidden={!completed&&!revealed.includes(i)} className={(completed||revealed.includes(i))&&cell.matching?'matched':''}><span aria-hidden="true">✧</span><strong>×{cell.symbol.slice(1)}</strong></div>)}</div>:<div className="scratch-idle"><span aria-hidden="true">✦</span><p>刮开星纹，揭晓你的惊喜</p></div>}
                {round?.scratch&&!completed&&<canvas ref={canvas} width={600} height={330} aria-label="刮卡涂层，按住并拖动；也可以使用立即揭晓按钮" onPointerDown={e=>{if(busy||(e.pointerType==='mouse'&&e.button!==0))return;drawing.current=true;e.currentTarget.setPointerCapture(e.pointerId);scratch(e)}} onPointerMove={scratch} onPointerUp={finish} onPointerCancel={finish}/>}
            </div>
            <div className="voucher-foot"><span>3 枚相同星纹 · 获得对应倍数总派彩</span><span>No. {round?.nonce??'—'}</span></div>
        </div>
        {round?.scratch&&!completed?<div className="reveal-controls"><p>{revealed.length>0?`已揭晓 ${revealed.length} / 9 格`:'本张已结算，按住拖动逐格揭晓。'}</p><button onClick={onComplete} disabled={busy}>立即揭晓</button></div>:<p className="stage-caption">{round?.scratch?'本张已全部揭晓，可继续下一张。':'每张九宫格，结果在购买时一次确定。'}</p>}
    </div>;
}

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
