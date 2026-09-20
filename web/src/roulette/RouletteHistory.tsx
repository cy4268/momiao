import {useState} from 'react';
import {Link,useParams} from 'react-router-dom';
import {z} from 'zod';
import type {ApiClient} from '../api';
import {Alert,Loading} from '../ui';
import {chips} from '../games-api';
import {Back,useHistory} from '../history/History';
import {PressureTable} from './PressureTable';
import {DevilTable} from './DevilTable';
import {actionSchema,bindingSchema,eventSchema,int64,roomSchema,rouletteNames,stateNames,uuid} from './roulette-api';
import './roulette.css';

const aggregate=z.string().regex(/^-?(0|[1-9]\d{0,37})$/),date=z.string().datetime({offset:true});
export const rouletteHistorySchema=z.object({id:uuid,game:z.enum(['devil-roulette','pressure-roulette']),state:z.string(),binding:bindingSchema,stake_units:aggregate,payout_units:aggregate,net_units:aggregate,reason:z.string(),created_at:date,ended_at:date.nullable(),snapshot:z.object({game_title:z.string(),actor_display_name:z.string().nullable(),metadata_origin:z.string()}),view:roomSchema,actions:z.array(eventSchema).max(100),commands:z.array(z.object({sequence:int64,seat:z.number().int(),input:z.union([actionSchema,z.object({kind:z.enum(['TIMEOUT','GAME_LIMIT'])}).strict()]),at:date,domain:z.string(),state_hash:z.string().regex(/^[0-9a-f]{64}$/),event:eventSchema})).max(100),frames:z.array(z.object({sequence:int64,view:roomSchema})).max(100),next_cursor:z.string().nullable(),transaction_ids:z.array(uuid).max(100),transactions_truncated:z.boolean()});
export const rouletteProofSchema=z.object({round_id:uuid,status:z.enum(['UNREVEALED','REVEALED']),valid:z.boolean(),commitment_valid:z.boolean(),config_valid:z.boolean(),actions_valid:z.boolean(),settlement_valid:z.boolean(),server_seed:z.string().regex(/^[0-9a-f]{64}$/).optional(),contributions:z.array(z.object({seat:z.number().int(),version:int64,seed:z.string()})).max(6).optional(),binding:bindingSchema,action_count:int64,history_path:z.string()});
function Proof({client,id}:{client:ApiClient;id:string}){
 const p=useHistory(client,`/api/v1/history/roulette/${id}/verify`,rouletteProofSchema);
 return p.loading?<Loading/>:p.error?<Alert>{p.error}</Alert>:p.data&&<div aria-live="polite"><h3>{p.data.status==='UNREVEALED'?'尚未揭示 · 结束后可复算':p.data.valid?'复算通过 · 随机与资金一致':'复算未通过 · 请保留记录核对'}</h3>{p.data.status==='REVEALED'&&<><dl className="history-facts">{(['commitment_valid','config_valid','actions_valid','settlement_valid'] as const).map((k,i)=><div key={k}><dt>{['种子承诺','配置版本','逐步动作','资金回链'][i]}</dt><dd>{p.data![k]?'通过':'未通过'}</dd></div>)}</dl><details><summary>原始证明与复算材料</summary><pre className="roulette-proof">{JSON.stringify(p.data,null,2)}</pre></details></>}</div>;
}
export function RouletteHistory({client}:{client:ApiClient}){
 const {id=''}=useParams();if(!uuid.safeParse(id).success)return <Alert>记录编号无效。</Alert>;
 return <Content key={id} client={client} id={id}/>;
}
function Content({client,id}:{client:ApiClient;id:string}){
 const [cursor,setCursor]=useState(''),[frame,setFrame]=useState(0),[proof,setProof]=useState(false);
 const read=useHistory(client,`/api/v1/history/roulette/${id}?limit=50${cursor?'&cursor='+encodeURIComponent(cursor):''}`,rouletteHistorySchema),d=read.data;
 return <div className="history-page roulette-history"><Back/><header className="page-heading"><div><p className="eyebrow">FAIR PLAY / ROUND REPLAY</p><h1>{d?rouletteNames[d.game]:'轮盘'} · 对局记录</h1><p>{id}</p></div><button onClick={read.reload}>刷新记录</button></header>{read.loading?<Loading/>:read.error?<Alert>{read.error}<button onClick={()=>setCursor('')}>返回第一页</button></Alert>:d&&<><section className="panel"><h2>{d.snapshot.game_title}</h2><p>当时昵称：{d.snapshot.actor_display_name||'旅人'} · {stateNames[d.state]} · {d.reason||'等待结束'}</p><dl className="history-facts"><div><dt>净投入（扣除退款）</dt><dd>{chips(d.stake_units)}</dd></div><div><dt>派彩</dt><dd>{chips(d.payout_units)}</dd></div><div><dt>钱包净变化</dt><dd>{chips(d.net_units,true)}</dd></div></dl><p>全体玩家零抽水；个人结果取决于玩法，不设固定净赢率。</p><details><summary>本局固定配置与公平随机绑定</summary><pre className="roulette-proof">{JSON.stringify(d.binding,null,2)}</pre></details><button onClick={()=>setProof(v=>!v)}>{proof?'收起证明':'验证本局随机与结算'}</button>{proof&&<Proof client={client} id={id}/>}</section><section className="panel"><h2>逐步回放</h2>{d.frames.length>0?<><label>本页动作<select value={frame} onChange={e=>setFrame(Number(e.target.value))}>{d.frames.map((f,i)=><option value={i} key={f.sequence}>#{f.sequence} · {d.actions[i]?.text}</option>)}</select></label>{d.game==='pressure-roulette'?<PressureTable view={d.frames[Math.min(frame,d.frames.length-1)].view} onAction={()=>{}} busy timer="历史回放"/>:<DevilTable view={d.frames[Math.min(frame,d.frames.length-1)].view} onAction={()=>{}} busy timer="历史回放"/>}</>:<p>结束前只展示公共事件与自己的视图。</p>}<ol>{d.actions.map(a=><li key={a.sequence}>#{a.sequence} · {a.text}</li>)}</ol><div className="ops-actions"><button disabled={!cursor} onClick={()=>{setCursor('');setFrame(0)}}>第一页</button><button disabled={!d.next_cursor} onClick={()=>{setCursor(d.next_cursor!);setFrame(0)}}>下一页动作</button></div></section><section className="panel"><h2>钱包交易回链</h2><ul>{d.transaction_ids.map(v=><li key={v}><Link to={'/wallet/transactions/'+v}>{v}</Link></li>)}</ul>{d.transactions_truncated&&<p>此处展示前 100 笔；完整资金核对由证明逐笔验证。</p>}</section></>}</div>;
}
