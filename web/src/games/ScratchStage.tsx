import { useLayoutEffect, useRef, useState, type CSSProperties, type PointerEvent } from 'react';
import type { GameRound } from '../games-api';
import { assetSrcSet, assetUrl } from '../game-hall-assets';
import art from './scratch-art.json';
import './scratch-stage.css';

// Keep anonymous reads separate from a display-only response in a browser cache.
const foilUrl=assetUrl(art.foil.src)+'?cors=1';

// Artwork names are presentation only; the server's symbols and multipliers stay authoritative.
const symbols:Record<string,{name:string;index:number}>={
    P1:{name:'赤铜果实',index:0},P2:{name:'圣晶石',index:1},P3:{name:'黄金果实',index:2},
    P5:{name:'呼符',index:3},P10:{name:'QP',index:4},P25:{name:'圣杯',index:5},P100:{name:'传承结晶',index:6},
};

export function ScratchStage({round,onComplete,busy}:{round:GameRound|null;onComplete:()=>void;busy:boolean}) {
    const canvas=useRef<HTMLCanvasElement>(null),drawing=useRef(false),touched=useRef(false);
    const previous=useRef<{x:number;y:number}|null>(null),revealedRef=useRef<number[]>([]);
    const [revealed,setRevealed]=useState<number[]>([]),[canScratch,setCanScratch]=useState(false);
    const purchased=!!round?.scratch,completed=!!round?.presentation_completed_at;

    useLayoutEffect(()=>{
        setRevealed([]);revealedRef.current=[];drawing.current=false;touched.current=false;previous.current=null;
        const ctx=canvas.current?.getContext('2d');
        setCanScratch(!!ctx);
        if(!ctx||!purchased||completed)return;
        // Cover before the first paint, even while the anonymous CDN texture is loading.
        ctx.globalCompositeOperation='source-over';
        ctx.fillStyle='#b99a5e';ctx.fillRect(0,0,600,330);
        const foil=new Image();let cancelled=false;
        foil.crossOrigin='anonymous';
        foil.onload=()=>{
            if(cancelled||touched.current)return;
            for(let i=0;i<9;i++)ctx.drawImage(foil,(i%3)*200,Math.floor(i/3)*110,200,110);
        };
        foil.src=foilUrl;
        return()=>{cancelled=true;foil.onload=null;};
    },[round?.id,purchased,completed]);

    function scratch(e:PointerEvent<HTMLCanvasElement>) {
        if(!drawing.current||busy||completed)return;
        const node=canvas.current,ctx=node?.getContext('2d');if(!node||!ctx)return;
        touched.current=true;
        const bounds=node.getBoundingClientRect(),point={x:(e.clientX-bounds.left)*600/bounds.width,y:(e.clientY-bounds.top)*330/bounds.height};
        ctx.globalCompositeOperation='destination-out';ctx.lineWidth=54;ctx.lineCap='round';ctx.beginPath();
        ctx.moveTo(previous.current?.x??point.x,previous.current?.y??point.y);ctx.lineTo(point.x,point.y);ctx.stroke();previous.current=point;
    }
    function finish() {
        drawing.current=false;previous.current=null;
        const ctx=canvas.current?.getContext('2d');if(!ctx||completed||busy)return;
        const next=[...revealedRef.current];
        for(let cell=0;cell<9;cell++){
            if(next.includes(cell))continue;
            const x=(cell%3)*200,y=Math.floor(cell/3)*110;
            const pixels=ctx.getImageData(x,y,200,110).data;let erased=0,total=0;
            for(let i=3;i<pixels.length;i+=64){total++;if(pixels[i]<32)erased++;}
            if(erased/total>=0.65){next.push(cell);ctx.clearRect(x,y,200,110);}
        }
        const allNew=next.length===9&&revealedRef.current.length!==9;
        revealedRef.current=next;setRevealed(next);if(allNew)onComplete();
    }

    return <div className="scratch-scene" style={{

        '--scratch-symbols':`image-set(url("${assetUrl(art.symbols.variants[0].src)}") 1x, url("${assetUrl(art.symbols.src)}") 2x)`,
    } as CSSProperties}>
        <img className="scratch-room" src={assetUrl(art.stage.src)} srcSet={assetSrcSet(art.stage)} sizes="(max-width:900px) 100vw, 75vw" width={art.stage.width} height={art.stage.height} alt=""/>
        <div className="voucher-art-card">
            <img className="voucher-frame" src={assetUrl(art.frame.src)} srcSet={assetSrcSet(art.frame)} sizes="(max-width:900px) 100vw, 65vw" width={art.frame.width} height={art.frame.height} alt=""/>
            <div className="voucher-title"><h2>迦勒底补给凭证</h2><span>CHALDEA / SUPPLY VOUCHER</span></div>
            <div className="voucher-surface">
                {round?.scratch&&<div className="voucher-prizes" aria-label="本张九宫格">{round.scratch.cells.map((cell,i)=>{
                    const visible=completed||revealed.includes(i),symbol=symbols[cell.symbol];
                    return <div key={i} aria-hidden={!visible} className={visible&&cell.matching?'matched':''}>
                        {symbol&&<span className="voucher-symbol" role="img" aria-label={symbol.name} style={{backgroundPosition:`${(symbol.index%4)*100/3}% ${Math.floor(symbol.index/4)*100}%`}}/>}
                        <strong>×{cell.symbol.slice(1)}</strong>
                    </div>;
                })}</div>}
                {!completed&&(!purchased||!canScratch)&&<div className="voucher-foil" role="img" aria-label={purchased?'未揭晓涂层':'尚未购买的刮刮卡'}>{Array.from({length:9},(_,i)=><img key={i} src={foilUrl} crossOrigin="anonymous" alt=""/>)}</div>}
                {purchased&&!completed&&<canvas ref={canvas} width={600} height={330} aria-hidden={!canScratch} aria-label="刮卡涂层，按住并拖动；也可以使用立即揭晓按钮" onPointerDown={e=>{if(!canScratch||busy||(e.pointerType==='mouse'&&e.button!==0))return;drawing.current=true;e.currentTarget.setPointerCapture(e.pointerId);scratch(e);}} onPointerMove={scratch} onPointerUp={finish} onPointerCancel={finish}/>}
            </div>
            <p className="voucher-motto">三枚相同星纹 · 对应倍数总派彩</p>
        </div>
        <div className="voucher-controls">
            <p aria-live="polite">{purchased?(completed?'本张已全部揭晓，可继续下一张。':revealed.length>0?`已揭晓 ${revealed.length} / 9 格`:'按住拖动，逐格揭晓本张。'):'等待购买 · 每张九宫格'}</p>
            {purchased&&!completed&&<button onClick={onComplete} disabled={busy}>立即揭晓</button>}
        </div>
    </div>;
}
