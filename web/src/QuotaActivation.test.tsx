import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, it, vi } from 'vitest';
import { ApiClient, ApiError } from './api';
import { QuotaActivation } from './QuotaActivation';
const receipt={id:'01990000-1111-7777-aaaa-000000000001',user_id:'1',amount:'1',amount_units:'500000',status:'PENDING',reason:'',native_before:null,native_after:null,created_at:'2026-09-05T12:00:00Z',updated_at:'2026-09-05T12:00:00Z'};
const wallet={initialized:true,user_id:'1',scope:'LOCAL_WALLETS_ONLY',total_assets:null,wallets:[{asset:'RESERVE_API_CREDIT',amount:'500',balance_units:'250000000',ledger_seq:'1',version:'2'},{asset:'AVAILABLE_CHIPS',amount:'0',balance_units:'0',ledger_seq:'0',version:'1'}]};
afterEach(()=>sessionStorage.clear());
function setup(action:(path:string,method?:string,body?:unknown)=>unknown){
 const request=vi.fn(async(path:string,method?:string,body?:unknown)=>method==='POST'||path.includes('by-key')?action(path,method,body):path.endsWith('/wallet')?wallet:path.endsWith('/native-quota')?{user_id:'1',raw_quota:'0',amount:'0',enabled:true}:[]);
 const client={request,getSessionGeneration:()=>1,getSnapshot:()=>({user:{id:1},loggingOut:false})} as unknown as ApiClient;
 return {request,...render(<MemoryRouter><QuotaActivation client={client} userID="1"/></MemoryRouter>)};
}
it('preserves an uncertain original request across remount and never auto-posts',async()=>{
 let lost=true;const action=(path:string)=>{if(path.includes('by-key'))return null;if(lost){lost=false;throw new ApiError('lost',0,'',true)}return receipt};
 const a=setup(action);await screen.findByText('500');fireEvent.change(screen.getByLabelText('转入数量'),{target:{value:'1'}});fireEvent.click(screen.getByRole('button',{name:'确认转入原生额度'}));
 await screen.findByRole('button',{name:'核对划转结果'});const p=JSON.parse(sessionStorage.getItem('momiao.quota.pending.1')!);a.unmount();
 const b=setup(action);await screen.findByRole('button',{name:'核对划转结果'});expect(b.request.mock.calls.filter(c=>c[1]==='POST')).toHaveLength(0);expect(screen.getByRole('button',{name:'确认转入原生额度'})).toBeDisabled();
 fireEvent.click(screen.getByRole('button',{name:'核对划转结果'}));fireEvent.click(await screen.findByRole('button',{name:'按原请求重试'}));
 await waitFor(()=>expect(sessionStorage.getItem('momiao.quota.pending.1')).toBeNull());expect(b.request.mock.calls.find(c=>c[1]==='POST')![2]).toEqual({idempotency_key:p.key,amount:'1'});
 expect(screen.getByRole('button',{name:'确认转入原生额度'})).toBeDisabled();
});
