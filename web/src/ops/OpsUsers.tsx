import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { ApiClient, errorText, type User, type Page, type Key } from '../api';
import { Alert, Loading, Pager, role, useResource } from '../ui';
import { FilterBar, useDebouncedValue } from './components/FilterBar';
import { DataTable, usePageSize, type Column } from './components/DataTable';
import { Drawer } from './components/Drawer';

type OpsUser={id:number;username:string;display_name:string;master_name:string;discord_id:string;group:string;role:number;status:number;two_fa_enabled:boolean;reserve:string;wallet_initialized:boolean;created_at:string;last_login_at:string|null};
type Group={name:string};
type Ledger={id:string;created_at:string;biz_type:string;delta_amount:string;balance_after_amount:string;note:string};
type Master={newapi_user_id:string;display_name:string;profile_version:string;rename_required:boolean;nickname_changed_at:string|null};
const root='/api/v1/ops/users';
const time=(v:string|null)=>v?new Date(v).toLocaleString('zh-CN',{hour12:false}):'—';
export function OpsUsers({client,user}:{client:ApiClient;user:User}){
 const {userID}=useParams(),navigate=useNavigate(),[editing,setEditing]=useState('');
 const selected=editing||userID;
 const [filters,setFilters]=useState({page:1,q:'',group:'',role:'',status:''}),[search,setSearch]=useState('');
 const settled=useDebouncedValue(search),[pageSize,setPageSize]=usePageSize('momiao.ops.users.pageSize');
 useEffect(()=>{setFilters(current=>({...current,q:settled,page:1}))},[settled]);
 useEffect(()=>{if(!selected)document.title='用户 · Chaldea Ops'},[selected]);
 const groups=useResource(()=>client.request<Group[]>('/api/v1/ops/groups'),[client]);
 const resource=useResource(()=>client.request<Page<OpsUser>>(root+'?'+new URLSearchParams({...filters,page:String(filters.page),page_size:String(pageSize)})),[client,pageSize,filters]);
 function filter(key:string,value:string){setFilters(current=>({...current,[key]:value,page:1}))}
 const columns:Column<OpsUser>[]=[
  {key:'name',label:'Master / 账号',render:u=><><button className="ops-text-button" onClick={()=>setEditing(String(u.id))}>{u.master_name||u.username} · #{u.id}</button><small>{u.username}{u.display_name?' · '+u.display_name:''}</small></>},
  {key:'discord',label:'Discord ID',render:u=>u.discord_id||'未绑定'},
  {key:'status',label:'分组 / 角色 / 状态',render:u=><>{u.group} / {role(u.role)} / {u.status===1?'正常':'封禁'}</>},
  {key:'reserve',label:'Reserve',render:u=>u.wallet_initialized?u.reserve:'未初始化'},
  {key:'time',label:'注册 / 最近登录',render:u=><>{time(u.created_at)}<small>{time(u.last_login_at)}</small></>},
 ];
 function close(changed:boolean){setEditing('');if(userID)navigate('/ops/users');if(changed)resource.reload()}
 return <><header className="page-heading"><div><p className="eyebrow">OPERATIONS / USERS</p><h1>用户管理</h1><p>本地账号、会话与 Reserve。</p></div><button onClick={resource.reload}>刷新用户</button></header><section className="panel">
 <FilterBar><label>用户名、显示名、Discord ID 或 ID<input maxLength={100} value={search} onChange={e=>setSearch(e.target.value)}/></label><label>分组<select value={filters.group} onChange={e=>filter('group',e.target.value)}><option value="">全部分组</option>{groups.data?.map(g=><option key={g.name}>{g.name}</option>)}</select></label><label>角色<select value={filters.role} onChange={e=>filter('role',e.target.value)}><option value="">全部角色</option>{[1,10,100].map(r=><option value={r} key={r}>{role(r)}</option>)}</select></label><label>状态<select value={filters.status} onChange={e=>filter('status',e.target.value)}><option value="">全部状态</option><option value="1">正常</option><option value="2">封禁</option></select></label></FilterBar>
 {groups.error&&<Alert>{groups.error}</Alert>}{resource.error&&<Alert>{resource.error}</Alert>}{resource.loading?<Loading/>:resource.data&&<DataTable columns={columns} rows={resource.data.items} rowKey={u=>String(u.id)} page={filters.page} total={resource.data.total} pageSize={pageSize} onPageChange={page=>setFilters(current=>({...current,page}))} onPageSizeChange={size=>{setPageSize(size);setFilters(current=>({...current,page:1}))}} disabled={search!==settled}/>}</section>
 {selected&&<UserDetail key={selected} client={client} current={user} id={selected} onClose={close}/>}</>;
}
function UserDetail({client,current,id,onClose}:{client:ApiClient;current:User;id:string;onClose:(changed:boolean)=>void}){
 const resource=useResource(()=>client.request<OpsUser>(root+'/'+id),[client,id]);
 const groups=useResource(()=>client.request<Group[]>('/api/v1/ops/groups'),[client]);
 const [tab,setTab]=useState('profile'),[busy,setBusy]=useState(false),[error,setError]=useState(''),[notice,setNotice]=useState(''),[version,setVersion]=useState(0);
 const changed=useRef(false);
 async function action(kind:string,body:unknown){
  setBusy(true);setError('');setNotice('');
  try{await client.request(root+'/'+id+(kind?'/'+kind:''),kind?'POST':'PUT',body);changed.current=true;setNotice('操作已保存');resource.reload();setVersion(v=>v+1);return true}catch(e){setError(errorText(e));return false}finally{setBusy(false)}
 }
 const u=resource.data,canManage=!!u&&(current.role===100||current.role>u.role);
 const title=`${u?.master_name||u?.username||'用户详情'} · #${id}`;
 useEffect(()=>{document.title=title+' · Chaldea Ops'},[title]);
 return <Drawer title={title} onClose={()=>onClose(changed.current)} busy={busy}>
 {notice&&<p role="status">{notice}</p>}{error&&<Alert>{error}</Alert>}{resource.error&&<Alert>{notice?'操作已保存，详情读取失败，请刷新核对。':resource.error}</Alert>}<button disabled={busy} onClick={resource.reload}>刷新详情</button>
 <div className="row-actions">{[['profile','资料与会话'],['master','Master 资料'],['balance','余额与流水'],['keys','API Key']].map(([key,label])=><button key={key} aria-pressed={tab===key} onClick={()=>setTab(key)}>{label}</button>)}<Link to={'/ops/logs?user='+id}>调用日志</Link><Link to={'/ops/records?user='+id}>游戏记录</Link></div>
 {resource.loading?<Loading/>:u&&<><p>登录名 {u.username} · 账号显示名 {u.display_name||'—'} · Discord {u.discord_id||'未绑定'} · 注册 {time(u.created_at)} · 最近登录 {time(u.last_login_at)}</p>
 {tab==='profile'&&<section className="panel"><Profile key={version} u={u} groups={groups.data||[]} root={current.role===100} disabled={busy||!canManage} save={body=>action('',body)}/>{groups.error&&<Alert>{groups.error}</Alert>}<p>2FA：{u.two_fa_enabled?'已启用':'未启用'}</p><div className="row-actions"><button disabled={busy||!canManage} onClick={()=>{if(window.confirm('重置此用户的 2FA 并撤销全部会话？'))void action('reset-2fa',{})}}>重置 2FA</button><button disabled={busy||!canManage} onClick={()=>{if(window.confirm('撤销此用户的全部登录会话？'))void action('logout',{})}}>强制下线</button></div></section>}
 {tab==='balance'&&<Balance key={version} client={client} u={u} disabled={busy||!canManage} adjust={body=>action('reserve',body)}/>}
 {tab==='master'&&<MasterProfile key={version} client={client} id={id} disabled={busy||!canManage} action={action}/>}
 {tab==='keys'&&<UserKeys key={version} client={client} id={id} disabled={busy||!canManage} action={action}/>}</>}
 </Drawer>;
}
function Profile({u,groups,root:asRoot,disabled,save}:{u:OpsUser;groups:Group[];root:boolean;disabled:boolean;save:(body:unknown)=>Promise<boolean>}){
 const [group,setGroup]=useState(u.group),[status,setStatus]=useState(u.status),[newRole,setRole]=useState(u.role);
 return <form onSubmit={e=>{e.preventDefault();if(status===2&&u.status!==2&&!window.confirm('确认封禁此用户并撤销全部会话？'))return;void save({group,status,...(asRoot?{role:newRole}:{})})}}><fieldset disabled={disabled}><label>用户分组<select value={group} onChange={e=>setGroup(e.target.value)}>{[...new Set([u.group,...groups.map(g=>g.name)])].map(g=><option key={g}>{g}</option>)}</select></label><label>账号状态<select value={status} onChange={e=>setStatus(Number(e.target.value))}><option value={1}>正常</option><option value={2}>封禁</option></select></label>{asRoot?<label>用户角色<select value={newRole} onChange={e=>setRole(Number(e.target.value))}>{[1,10,100].map(r=><option value={r} key={r}>{role(r)}</option>)}</select></label>:<p>角色：{role(u.role)}</p>}<button type="submit">保存资料</button></fieldset></form>;
}
function MasterProfile({client,id,disabled,action}:{client:ApiClient;id:string;disabled:boolean;action:(kind:string,body:unknown)=>Promise<boolean>}){
 const resource=useResource(()=>client.request<Master>(root+'/'+id+'/master'),[client,id]);
 return <section className="panel"><h2>Master 昵称管理</h2><p>管理游戏和排行榜使用的昵称，不修改登录账号的显示名。</p><button onClick={resource.reload} disabled={disabled}>刷新 Master 资料</button>
 {resource.error&&<Alert>{resource.error}</Alert>}{resource.loading?<Loading/>:resource.data&&(resource.data.profile_version==='0'?<p>此账号尚未完成 Master 资料，不创建资料或钱包。</p>:<MasterForm master={resource.data} disabled={disabled} action={action}/>)}</section>;
}
function MasterForm({master,disabled,action}:{master:Master;disabled:boolean;action:(kind:string,body:unknown)=>Promise<boolean>}){
 const [name,setName]=useState(master.display_name),[required,setRequired]=useState(master.rename_required);
 return <><p>用户最近主动改名：{time(master.nickname_changed_at)}。管理员强制改名不占用用户的七天改名冷却。</p>
 <form onSubmit={e=>{e.preventDefault();void action('master-rename-required',{required})}}><fieldset disabled={disabled}><label><input type="checkbox" checked={required} onChange={e=>setRequired(e.target.checked)}/>要求用户改名</label><button type="submit">保存改名要求</button></fieldset></form>
 <form onSubmit={e=>{e.preventDefault();if(window.confirm('将此用户的 Master 昵称改为“'+name+'”并清除改名要求？'))void action('master-force-rename',{display_name:name})}}><fieldset disabled={disabled}><label>Master 昵称<input required value={name} onChange={e=>setName(e.target.value)}/></label><button type="submit">强制改名</button></fieldset></form></>;
}
function Balance({client,u,disabled,adjust}:{client:ApiClient;u:OpsUser;disabled:boolean;adjust:(body:unknown)=>Promise<boolean>}){
 const [page,setPage]=useState(1),[amount,setAmount]=useState(''),[note,setNote]=useState(''),[requestID]=useState(()=>crypto.randomUUID());
 const ledger=useResource(()=>client.request<Page<Ledger>>(root+'/'+u.id+'/ledger?page='+page),[client,u.id,page]);
 async function submit(e:FormEvent){e.preventDefault();if(amount.startsWith('-')&&!window.confirm('确认扣减此用户的 Reserve？'))return;await adjust({amount,note,request_id:requestID})}
 return <section className="panel"><h2>Reserve：{u.wallet_initialized?u.reserve:'未初始化'}</h2><form onSubmit={submit}><fieldset disabled={disabled||!u.wallet_initialized}><label>调整金额（负数扣减）<input required inputMode="decimal" value={amount} onChange={e=>setAmount(e.target.value)} placeholder="0.000002"/></label><label>调账备注<textarea required maxLength={1000} value={note} onChange={e=>setNote(e.target.value)}/></label><button type="submit">保存调账</button></fieldset><p>以 Credit 为单位；最小变动 0.000002。请求失败时保留同一调账编号重试，不重复入账。</p></form>
 {ledger.error&&<Alert>{ledger.error}</Alert>}{ledger.loading?<Loading/>:ledger.data&&<><div className="table-wrap"><table><thead><tr><th>时间</th><th>业务</th><th>变动</th><th>余额</th><th>备注</th></tr></thead><tbody>{ledger.data.items.map(e=><tr key={e.id}><td>{time(e.created_at)}</td><td>{e.biz_type}</td><td>{e.delta_amount}</td><td>{e.balance_after_amount}</td><td>{e.note}</td></tr>)}</tbody></table></div><Pager page={page} size={20} total={ledger.data.total} onChange={setPage} disabled={disabled}/></>}</section>;
}
function UserKeys({client,id,disabled,action}:{client:ApiClient;id:string;disabled:boolean;action:(kind:string,body:unknown)=>Promise<boolean>}){
 const [page,setPage]=useState(1);const resource=useResource(()=>client.request<Page<Key>>(root+'/'+id+'/keys?page='+page),[client,id,page]);
 return <section className="panel"><h2>API Key</h2>{resource.error&&<Alert>{resource.error}</Alert>}{resource.loading?<Loading/>:resource.data&&<><div className="table-wrap"><table><thead><tr><th>名称</th><th>掩码</th><th>状态 / 用途</th><th>操作</th></tr></thead><tbody>{resource.data.items.map(k=><tr key={k.id}><td>{k.name}</td><td>{k.key}</td><td>{k.status===1?'启用':'停用'} / {k.purpose}</td><td><button disabled={disabled||k.status!==1} onClick={()=>void action('disable-key',{key_id:k.id})}>停用 Key</button><button disabled={disabled} onClick={()=>{if(window.confirm('删除此 Key？历史调用日志保留。'))void action('delete-key',{key_id:k.id})}}>删除 Key</button></td></tr>)}</tbody></table></div><Pager page={page} size={20} total={resource.data.total} onChange={setPage} disabled={disabled}/></>}</section>;
}
