import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, it } from 'vitest';
import { OpsCatalog } from './OpsCatalog';
import { catalogOps, type CatalogCommand, type CatalogModel, type CatalogOpsPage } from './catalog-api';
import { fixtureClient, ok, failed } from './m1-test-fixtures';

it('edits, previews and publishes a model then keeps its confirmed receipt when refresh fails',async()=>{
 const principal={user_id:'1',base_role:'SUPER_ADMIN',authz_epoch:2,permissions:['models.read','models.write','models.publish']};
 let item:CatalogModel={model_id:'synthetic/model',metadata:{display_name:'',family:'',summary:'',context_length:null,subtitle:'',tags:[],use_cases:[],special_pricing_note:'',asset_id:''},publication_state:'PENDING_METADATA',recommended:false,sort_order:0,version:'1',metadata_version:'1',published_at:null,retired_at:null,updated_at:'2026-09-06T00:00:00Z',availability_state:'CONFIGURED',source_observed_at:'2026-09-06T00:00:00Z',last_seen_at:'2026-09-06T00:00:00Z',endpoint_status:'configured_subset_not_health',endpoints:[{kind:'openai',path:'/v1/chat/completions',method:'POST'}],price:{mode:'ratio',configured:true,status:'reference',dimensions:[],unquoted_dimensions:[]},can_use:false,freshness:{state:'CURRENT',last_observed_at:null,last_verified_at:null,stale_after_seconds:600,disable_after_seconds:1800}};
 const executions:{command:CatalogCommand;confirmed:boolean;preview_id:string}[]=[];let failRefresh=false;
 const {client}=fixtureClient((path,init)=>{
  if(!path.startsWith(catalogOps))return;
  if(path.endsWith('/prepare')){const {command}=JSON.parse(String(init?.body));return ok({preview_id:'preview-fixture',expires_at:'2099-01-01T00:00:00Z',impact:{action:command.action,before:item,after:{...item,metadata:command.metadata||item.metadata,publication_state:command.action==='PUBLISH'?'PUBLISHED':item.publication_state},catalog_version:'1',effect:'确认这次模型变更。'}})}
  if(path.endsWith('/execute')){const body=JSON.parse(String(init?.body));executions.push(body);item={...item,metadata:body.command.metadata||item.metadata,version:String(Number(item.version)+1),publication_state:body.command.action==='PUBLISH'?'PUBLISHED':item.publication_state};if(body.command.action==='PUBLISH')failRefresh=true;return ok({operation_id:body.command.operation_id,model_id:item.model_id,version:item.version,metadata_version:'2',publication_state:item.publication_state})}
  if(failRefresh)return failed();
  if(path.includes('/detail?'))return ok({principal,item});
  const page:CatalogOpsPage={principal,items:[item],total:1,offset:0,limit:50,sync:{version:'1',observed_count:1,last_attempt_at:null,last_attempt_status:'VERIFIED',last_observed_at:null,last_verified_at:null},freshness:item.freshness,vocabulary:{families:[{value:'gemini',label:'Gemini'}],tags:[{value:'writing',label:'写作'}],use_cases:[{value:'writing',label:'创作与写作'}],assets:[]}};return ok(page);
 });await client.login('ops','fixture');render(<MemoryRouter><OpsCatalog client={client}/></MemoryRouter>);
 fireEvent.click(await screen.findByRole('button',{name:'编辑模型'}));await screen.findByLabelText('展示名称');fireEvent.change(screen.getByLabelText('展示名称'),{target:{value:'已审核模型'}});fireEvent.change(screen.getByLabelText('模型家族'),{target:{value:'gemini'}});fireEvent.change(screen.getByLabelText('模型简介'),{target:{value:'可核对的合成模型介绍。'}});fireEvent.change(screen.getByLabelText('操作原因'),{target:{value:'补全模型信息'}});fireEvent.click(screen.getByRole('button',{name:'预览保存'}));
 fireEvent.click(await screen.findByRole('button',{name:'确认执行'}));await waitFor(()=>expect(executions.length).toBe(1));await screen.findByText('已确认：保存元数据');
 await waitFor(()=>expect(screen.getByRole('button',{name:'预览发布'})).toBeEnabled());fireEvent.click(screen.getByRole('button',{name:'预览发布'}));fireEvent.click(await screen.findByRole('button',{name:'确认执行'}));
 await screen.findByText('已确认：发布模型');await screen.findByText('操作已经确认，最新状态读取失败。请刷新核对，勿重复提交。');expect(screen.getByRole('button',{name:'预览保存'})).toBeDisabled();expect(executions.every(e=>e.confirmed&&e.preview_id==='preview-fixture')).toBe(true);expect(executions[0].command.operation_id).not.toBe(executions[1].command.operation_id);
 failRefresh=false;fireEvent.click(screen.getByRole('button',{name:'重新核对状态'}));await waitFor(()=>expect(screen.queryByText('操作已经确认，最新状态读取失败。请刷新核对，勿重复提交。')).not.toBeInTheDocument());expect(executions.length).toBe(2);
});
