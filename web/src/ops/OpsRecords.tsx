import { useState, type FormEvent } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { type ApiClient } from '../api';
import { Alert, Loading, Modal, useResource } from '../ui';
import { chips } from '../games-api';
import { recordTypes, results, statuses, type Summary, type RecordType } from '../history/api';
import { OpsFacts, opsTime } from './OpsFacts';
import '../history/history.css';
import './ops-business.css';

type Selection={kind:'roulette'|'rounds'|'sessions'|'hands'|'transactions';id:string;user:string};
type RecordPage={items:(Summary&{newapi_user_id:string})[];has_more:boolean;next_cursor:string|null;game_options:{game_slug:string;game_title:string;retired:boolean}[]};
type Detail={transactions?:{id:string}[];transaction_ids?:string[];hands?:{hand_id:string;hand_no:string}[];links?:{record_type:RecordType;source_id:string}[];next_funding_cursor?:string;next_hand_cursor?:string;next_cursor?:string};
const kinds:Record<RecordType,Selection['kind']>={ROULETTE_ROUND:'roulette',DIRECT_PLAY_ROUND:'rounds',POKER_SESSION:'sessions',POKER_HAND:'hands'};
const names:Record<string,string>={ROULETTE_ROUND:'轮盘对局',DIRECT_PLAY_ROUND:'单局游戏',POKER_SESSION:'Poker 会话',POKER_HAND:'Poker 手牌',WIN:'净赢',LOSS:'净输',BREAK_EVEN:'回本',CANCELLED:'已取消',REFUNDED:'已退回',PROCESSING:'处理中',SETTLED:'已结算',RECOVERING:'恢复中'};
const amount=(value:string|null|undefined)=>value==null?'—':chips(value,true);
export function OpsRecords({client}:{client:ApiClient}){
 const [params]=useSearchParams(),[query,setQuery]=useState(params.get('user')?'user='+encodeURIComponent(params.get('user')!):'');
 const [cursors,setCursors]=useState(['']),[selected,setSelected]=useState<Selection>(),[error,setError]=useState('');
 const cursor=cursors.at(-1)!;
 const resource=useResource(()=>client.request<RecordPage>(`/api/v1/ops/records?limit=50&${query}${cursor?'&cursor='+encodeURIComponent(cursor):''}`),[client,query,cursor]);
 function filter(event:FormEvent<HTMLFormElement>){
  event.preventDefault();const values=new FormData(event.currentTarget),next=new URLSearchParams();
  for(const [key,value] of values){const text=String(value).trim();if(text)next.set(key,key==='time_from'||key==='time_to'?new Date(text).toISOString():text)}
  if(next.has('time_from')&&next.has('time_to')&&next.get('time_from')!>=next.get('time_to')!){setError('开始时间须早于结束时间。');return}
  setError('');setSelected(undefined);setCursors(['']);setQuery(next.toString());resource.reload();
 }
 return <><header className="page-heading"><div><p className="eyebrow">OPERATIONS / RECORDS</p><h1>全局游戏记录</h1><p>用户为可选筛选；每行对应一名参与者。读取详情和公平性材料沿用原记录的可见范围，并记入操作日志。</p></div><button onClick={resource.reload} disabled={resource.loading}>刷新记录</button></header>
 <section className="panel ops-business-record-search"><h2>查询条件</h2><form className="filters ops-business-record-filters" onSubmit={filter}>
 <label>用户 ID（可选）<input name="user" defaultValue={params.get('user')||''} maxLength={19} inputMode="numeric" pattern="[1-9][0-9]*"/></label>
 <label>记录类型<select name="record_type"><option value="">全部单局与会话</option>{recordTypes.map(value=><option key={value} value={value}>{names[value]}</option>)}</select></label>
 <label>模式<select name="mode"><option value="">全部模式</option><option value="DIRECT_PLAY">直接游玩</option><option value="POKER">Poker</option><option value="ROULETTE">多人轮盘</option></select></label>
 <label>游戏<select name="game_slug"><option value="">全部游戏</option>{resource.data?.game_options.map(item=><option value={item.game_slug} key={item.game_slug}>{item.game_title}{item.retired?'（已退役）':''}</option>)}</select></label>
 <label>开始时间<input type="datetime-local" name="time_from"/></label><label>结束时间（不含）<input type="datetime-local" name="time_to"/></label>
 <label>结果<select name="result"><option value="">全部结果</option>{results.map(value=><option key={value} value={value}>{names[value]||value}</option>)}</select></label>
 <label>状态<select name="status"><option value="">全部状态</option>{statuses.map(value=><option key={value} value={value}>{names[value]||value}</option>)}</select></label>
 <label>精确记录编号<input name="id" maxLength={36}/></label><button type="submit">查询业务记录</button></form>
 {error&&<Alert>{error}</Alert>}{resource.loading?<Loading/>:resource.error?<Alert>{resource.error}</Alert>:resource.data&&<>
 <div className="section-heading ops-business-result-heading"><h2>查询结果</h2></div><p className="hint">{new URLSearchParams(query).get('user')?'当前账户：'+new URLSearchParams(query).get('user'):'全部账户'}。时间筛选使用当前设备时区。</p>
 <div className="history-records">{resource.data.items.map(item=><article className="history-record" key={item.record_type+item.source_id+item.newapi_user_id}>
 <div><small>{names[item.record_type]} · {opsTime(item.occurred_at)}</small><h3>{item.snapshot.game_title}</h3><p>{item.snapshot.table_name||''} · {names[item.status]||item.status}</p><Link to={'/ops/users/'+item.newapi_user_id}>用户 #{item.newapi_user_id}</Link><small>当时昵称：{item.snapshot.actor_display_name||'未记录'}</small></div>
 <dl className="ops-impact"><div><dt>投入</dt><dd>{amount(item.record_type==='POKER_SESSION'?item.initial_buyin_units:item.stake_units)}</dd></div><div><dt>返回</dt><dd>{amount(item.record_type==='POKER_SESSION'?item.final_cashout_units:item.payout_units)}</dd></div><div><dt>净变化</dt><dd>{amount(item.net_change_units)}</dd></div></dl>
 <button onClick={()=>setSelected({kind:kinds[item.record_type],id:item.source_id,user:item.newapi_user_id})}>读取详情</button><small>{item.source_id}</small></article>)}</div>
 {!resource.data.items.length&&<p>暂无符合条件的记录。</p>}
 <nav className="ops-actions" aria-label="游戏记录分页"><button disabled={cursors.length===1} onClick={()=>setCursors([''])}>返回第一页</button><button disabled={cursors.length===1} onClick={()=>setCursors(cursors.slice(0,-1))}>上一页</button><span>第 {cursors.length} 页</span><button disabled={!resource.data.has_more||!resource.data.next_cursor} onClick={()=>setCursors([...cursors,resource.data!.next_cursor!])}>下一页</button></nav></>}
 </section>{selected&&<Modal title="记录详情" onClose={()=>setSelected(undefined)}><OpsRecordDetail key={selected.user+selected.kind+selected.id} client={client} selection={selected} select={setSelected}/></Modal>}</>;
}
function OpsRecordDetail({client,selection,select}:{client:ApiClient;selection:Selection;select:(item:Selection)=>void}){
 const [cursor,setCursor]=useState(''),[funding,setFunding]=useState(''),[hands,setHands]=useState(''),[proof,setProof]=useState(false);
 const base=`/api/v1/ops/records/${selection.kind}/${selection.id}`,identity=`user=${selection.user}`;
 const resource=useResource(async()=>{
  const suffix=selection.kind==='sessions'?`&funding_limit=50&hand_limit=50${funding?'&funding_cursor='+encodeURIComponent(funding):''}${hands?'&hand_cursor='+encodeURIComponent(hands):''}`:(selection.kind==='hands'||selection.kind==='roulette')?`&limit=100${cursor?'&cursor='+encodeURIComponent(cursor):''}`:'';
  const value=await client.request<{record:Detail}>(`${base}?${identity}${suffix}`);return value.record;
 },[client,base,identity,selection.kind,cursor,funding,hands]);
 const verify=useResource(async()=>proof?(await client.request<{record:unknown}>(`${base}/verify?${identity}`)).record:null,[client,base,identity,proof]);
 const open=(kind:Selection['kind'],id:string)=>select({kind,id,user:selection.user}),data=resource.data;
 return <section className="ops-business-record-detail"><div className="section-heading"><p>用户 #{selection.user} · {selection.id}</p><button onClick={resource.reload} disabled={resource.loading}>刷新原记录</button></div>
 {resource.loading?<Loading/>:resource.error?<Alert>{resource.error}</Alert>:data&&<><OpsFacts value={data}/><div className="ops-actions">
 {data.transactions?.map(item=><button key={item.id} onClick={()=>open('transactions',item.id)}>交易 {item.id}</button>)}
 {data.transaction_ids?.map(id=><button key={id} onClick={()=>open('transactions',id)}>交易 {id}</button>)}
 {data.hands?.map(item=><button key={item.hand_id} onClick={()=>open('hands',item.hand_id)}>第 {item.hand_no} 手牌</button>)}
 {data.links?.map(item=><button key={item.record_type+item.source_id} onClick={()=>open(kinds[item.record_type],item.source_id)}>关联{names[item.record_type]}</button>)}
 {data.next_funding_cursor&&<button onClick={()=>setFunding(data.next_funding_cursor!)}>下一页资金操作</button>}{data.next_hand_cursor&&<button onClick={()=>setHands(data.next_hand_cursor!)}>下一页手牌</button>}{data.next_cursor&&<button onClick={()=>setCursor(data.next_cursor!)}>下一页行动</button>}</div>
 {['rounds','hands','roulette'].includes(selection.kind)&&<><button onClick={()=>setProof(value=>!value)}>{proof?'收起公平性材料':'读取获授权的公平性材料'}</button>{proof&&(verify.loading?<Loading/>:verify.error?<Alert>{verify.error}</Alert>:verify.data&&<OpsFacts value={verify.data}/>)}</>}</>}
 </section>;
}
