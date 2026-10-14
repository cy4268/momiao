import { useEffect, useState, type FormEvent } from 'react';
import { useOutletContext } from 'react-router-dom';
import { ApiClient, errorText } from '../api';
import { Alert, Loading, useResource } from '../ui';

type Maintenance={enabled:boolean;message:string;scopes:string[]};
const scopes=[['CHALDEA_USER_WRITES','平台用户新写入'],['WALLET_EXCHANGE','钱包兑换'],['REWARDS','奖励领取'],['DIRECT_PLAY_NEW_ROUNDS','新游戏回合'],['POKER_NEW_TABLES_NEW_HANDS','Poker 新桌与新手牌'],['RANKINGS_PUBLISHING','排行榜发布'],['ANNOUNCEMENTS_SCHEDULING','公告定时发布']] as const;
export function OpsMaintenance(){
 const {client}=useOutletContext<{client:ApiClient}>();
 const resource=useResource(()=>client.request<Maintenance>('/api/v1/ops/maintenance'),[client]);
 const [value,setValue]=useState<Maintenance>({enabled:false,message:'',scopes:scopes.map(([key])=>key)});
 const [busy,setBusy]=useState(false),[error,setError]=useState(''),[saved,setSaved]=useState(false);
 useEffect(()=>{if(resource.data)setValue({...resource.data,scopes:resource.data.scopes.length?resource.data.scopes:scopes.map(([key])=>key)})},[resource.data]);
 async function save(event:FormEvent){
  event.preventDefault();setBusy(true);setError('');setSaved(false);
  try{await client.request('/api/v1/ops/maintenance','PUT',{enabled:value.enabled,message:value.message,scopes:value.scopes});setSaved(true)}
  catch(error){setError(errorText(error))}finally{setBusy(false)}
 }
 return <><header className="page-heading ops-system-heading"><div><p className="eyebrow">OPERATIONS / MAINTENANCE</p><h1>维护设置</h1><p>暂停选定范围的新工作；已受理工作的结算和退出继续。</p></div></header>
 {resource.loading?<Loading/>:resource.error?<><Alert>{resource.error}</Alert><button onClick={resource.reload}>重新读取</button></>:<section className="panel ops-system-editor"><form onSubmit={save}><fieldset disabled={busy}><legend>维护设置</legend>
 <label><input type="checkbox" checked={value.enabled} onChange={event=>{setValue({...value,enabled:event.target.checked});setSaved(false)}}/>开启维护</label>
 <label>提示语（可选）<textarea value={value.message} onChange={event=>{setValue({...value,message:event.target.value});setSaved(false)}} rows={3}/></label>
 <fieldset className="ops-system-choice-grid"><legend>影响范围（默认全部）</legend>{scopes.map(([key,label])=><label key={key}><input type="checkbox" checked={value.scopes.includes(key)} onChange={event=>{setValue({...value,scopes:event.target.checked?[...value.scopes,key]:value.scopes.filter(scope=>scope!==key)});setSaved(false)}}/>{label}</label>)}</fieldset>
 <button type="submit" disabled={value.enabled&&!value.scopes.length}>{busy?'保存中…':'保存'}</button></fieldset></form>{error&&<Alert>{error}</Alert>}{saved&&<p role="status">维护设置已保存。</p>}</section>}</>;
}
