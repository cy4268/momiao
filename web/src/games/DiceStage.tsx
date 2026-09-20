import { useEffect, useRef, useState, type CSSProperties } from 'react';
import type { DiceResult } from '../games-api';
import { assetSrcSet, assetUrl } from '../game-hall-assets';
import art from './dice-art.json';
import './dice-stage.css';

const pips:Record<number,number[]>={1:[5],2:[1,9],3:[1,5,9],4:[1,3,7,9],5:[1,3,5,7,9],6:[1,3,4,6,7,9]};
const orientations:Record<number,string>={1:'rotateX(0deg)',2:'rotateX(-90deg)',3:'rotateY(-90deg)',4:'rotateY(90deg)',5:'rotateX(90deg)',6:'rotateY(180deg)'};

export function DiceStage({result,busy,loading,recovering,onAnimatingChange}:{result?:DiceResult;busy:boolean;loading:boolean;recovering:boolean;onAnimatingChange:(value:boolean)=>void}) {
    const [started,setStarted]=useState(false),[animating,setAnimating]=useState(false);
    const timer=useRef<ReturnType<typeof setTimeout>|undefined>(undefined);
    useEffect(()=>{if(busy||result||recovering)setStarted(true)},[busy,result,recovering]);
    useEffect(()=>{
        if(!busy||window.matchMedia?.('(prefers-reduced-motion: reduce)').matches)return;
        clearTimeout(timer.current);setAnimating(true);onAnimatingChange(true);
        // Presentation only: the request and server result never depend on this timer.
        timer.current=setTimeout(()=>{setAnimating(false);onAnimatingChange(false)},1100);
    },[busy,onAnimatingChange]);
    useEffect(()=>()=>{clearTimeout(timer.current);onAnimatingChange(false)},[onAnimatingChange]);
    const rolling=busy||animating,settled=!rolling&&!recovering&&!loading?result:undefined;
    const showCup=!started&&!result&&!busy&&!recovering&&!loading;
    // Idle faces are decorative; only a confirmed server result supplies landed points.
    const faces=settled?.dice??[2,3,5];
    return <div className={'dice-scene'+(showCup?' is-first':'')} style={{'--dice-face':`url("${assetUrl(art.face.src)}")`} as CSSProperties}>
        <img className="dice-room" src={assetUrl(art.stage.src)} srcSet={assetSrcSet(art.stage)} sizes="(max-width: 900px) 100vw, 75vw" width={art.stage.width} height={art.stage.height} alt="" onError={e=>{e.currentTarget.style.visibility='hidden'}}/>
        <div className="dice-scene-heading" aria-hidden="true"><span>LUCKY DICE SALON</span><p>一掷之间，星光落定。</p></div>
        {showCup&&<img className="dice-cup" src={assetUrl(art.cup.src)} srcSet={assetSrcSet(art.cup)} sizes="(max-width: 900px) 50vw, 34vw" width={art.cup.width} height={art.cup.height} alt="星月骰盅，仅在首次开局前展示" onError={e=>{e.currentTarget.style.visibility='hidden'}}/>}
        <div className={'crystal-dice'+(rolling?' is-rolling':'')} aria-label={rolling?'三颗骰子正在翻滚，等待落定':settled?`骰子点数 ${settled.dice.join('、')}，合计 ${settled.total} 点`:'三颗骰子，等待开局'}>
            {faces.map((value,index)=><div className="die-position" key={index} aria-hidden="true"><div className="die-tumble"><div className="crystal-die" style={{'--landed-face':orientations[value]} as CSSProperties}>
                {[1,6,3,4,2,5].map(face=><div className={'crystal-face face-'+face} key={face}>{Array.from({length:9},(_,i)=><i key={i} className={pips[face].includes(i+1)?'crystal-pip':'pip-space'}/>)}</div>)}
            </div></div></div>)}
        </div>
        <p className="dice-status" role="status">{rolling?'骰子翻滚中，等待落定…':recovering?'本局等待核对，暂不展示新点数。':loading?'正在恢复游戏状态…':settled?`${settled.dice.join(' + ')} = ${settled.total} 点 · ${settled.triple?'豹子':settled.side==='BIG'?'大':'小'}`:started?'选择大小，再掷一局。':'等待开局 · 三颗骰子，一次选择。'}</p>
    </div>;
}
