import { act,fireEvent,render,screen,waitFor } from '@testing-library/react';
import { afterEach,expect,it,vi } from 'vitest';
import { ApiClient,ApiError } from './api';
import { WalletActions } from './WalletActions';
import type { WalletData } from './wallet-api';
const daily={user_id:'1',business_date:'2026-09-05',timezone:'Asia/Shanghai',next_reset_at:'2026-09-05T16:00:00Z',amount:'500',amount_units:'250000000',asset:'RESERVE_API_CREDIT',policy_version:'1',claimed:false,transaction_id:null};
const receipt={id:'01990000-1111-7777-aaaa-000000000001',user_id:'1',biz_id:'daily:1:2026-09-05',kind:'DAILY_REWARD',status:'CONFIRMED',from_asset:'',to_asset:'RESERVE_API_CREDIT',amount_units:'250000000',amount:'500',created_at:'2026-09-05T12:00:00Z',confirmed_at:'2026-09-05T12:00:00Z'};
const wallet:WalletData={initialized:true,user_id:'1',scope:'LOCAL_WALLETS_ONLY',total_assets:null,wallets:[{asset:'RESERVE_API_CREDIT',amount:'500',balance_units:'250000000',ledger_seq:'1',version:'2'},{asset:'AVAILABLE_CHIPS',amount:'0',balance_units:'0',ledger_seq:'0',version:'1'}]};
afterEach(()=>{sessionStorage.clear();vi.useRealTimers()});
function setup(action:(path:string,method?:string,body?:unknown)=>unknown,loadDaily:()=>unknown=()=>daily){const request=vi.fn(async(path:string,method?:string,body?:unknown)=>{if(method==='POST'||path.includes('by-key'))return action(path,method,body);return path.endsWith('/daily')?loadDaily():{items:[],has_more:false,next_after_id:null}});const c={request,getSessionGeneration:()=>1,getSnapshot:()=>({user:{id:1},loggingOut:false})} as unknown as ApiClient;const onChange=vi.fn();const mounted=render(<WalletActions client={c} userID="1" wallet={wallet} onChange={onChange}/>);return {request,onChange,...mounted}}
it('never auto-claims and explicit daily claim sends only an idempotency key',async()=>{const {request,onChange}=setup(()=>receipt);const button=await screen.findByRole('button',{name:'领取今日 500 额度'});expect(request.mock.calls.filter(c=>c[1]==='POST')).toHaveLength(0);fireEvent.click(button);await waitFor(()=>expect(onChange).toHaveBeenCalledOnce());expect(Object.keys(request.mock.calls.find(c=>c[1]==='POST')![2] as object)).toEqual(['idempotency_key']);expect(sessionStorage.length).toBe(0)});
it('keeps a lost exchange locked across remount and retries only the same key after lookup',async()=>{let first=true;const exchange={...receipt,kind:'LOCAL_EXCHANGE',from_asset:'RESERVE_API_CREDIT',to_asset:'AVAILABLE_CHIPS',amount_units:'500000',amount:'1'};const action=(path:string)=>{if(path.includes('by-key'))return null;if(first){first=false;throw new ApiError('lost',0,'',true)}return exchange};const a=setup(action);await screen.findByRole('button',{name:'领取今日 500 额度'});fireEvent.change(screen.getByLabelText('兑换数量'),{target:{value:'1'}});fireEvent.click(screen.getByRole('button',{name:'确认兑换'}));await screen.findByRole('button',{name:'核对交易结果'});const original=JSON.parse(sessionStorage.getItem('momiao.wallet.pending.1')!);a.unmount();const b=setup(action);await screen.findByRole('button',{name:'核对交易结果'});expect(screen.getByRole('button',{name:'确认兑换'})).toBeDisabled();expect(b.request.mock.calls.filter(c=>c[1]==='POST')).toHaveLength(0);fireEvent.click(screen.getByRole('button',{name:'核对交易结果'}));fireEvent.click(await screen.findByRole('button',{name:'按原请求重试'}));await waitFor(()=>expect(b.onChange).toHaveBeenCalledOnce());expect(b.request.mock.calls.find(c=>c[1]==='POST')![2]).toEqual({idempotency_key:original.key,from_asset:'RESERVE_API_CREDIT',amount:'1'})});
it('reconciles a confirmed response without sending another mutation',async()=>{sessionStorage.setItem('momiao.wallet.pending.1',JSON.stringify({kind:'DAILY',key:'01990000-1111-7777-aaaa-000000000001'}));const {request,onChange}=setup(()=>receipt);fireEvent.click(await screen.findByRole('button',{name:'核对交易结果'}));await waitFor(()=>expect(onChange).toHaveBeenCalledOnce());expect(request.mock.calls.filter(c=>c[1]==='POST')).toHaveLength(0)});
it('refreshes the displayed Shanghai day at the server reset without claiming',async()=>{
 vi.useFakeTimers();vi.setSystemTime(new Date('2026-09-05T15:59:59Z'));
 let tomorrow=false;let view!:ReturnType<typeof setup>;
 await act(async()=>{view=setup(()=>receipt,()=>tomorrow?{...daily,business_date:'2026-09-06',next_reset_at:'2026-09-06T16:00:00Z'}:{...daily,claimed:true,transaction_id:receipt.id})});
 expect(screen.getByRole('button',{name:'今日已领取'})).toBeDisabled();
 tomorrow=true;
 await act(async()=>{await vi.advanceTimersByTimeAsync(1100)});
 expect(screen.getByRole('button',{name:'领取今日 500 额度'})).toBeEnabled();
 expect(screen.getByText(/奖励日期：2026-09-06/)).toBeVisible();
 expect(view.request.mock.calls.filter(c=>c[1]==='POST')).toHaveLength(0);
});
it('checks a claim made elsewhere when the tab regains focus without POST',async()=>{
 let claimed=false;
 const {request}=setup(()=>receipt,()=>({...daily,claimed,transaction_id:claimed?receipt.id:null}));
 await screen.findByRole('button',{name:'领取今日 500 额度'});
 claimed=true;fireEvent.focus(window);
 expect(await screen.findByRole('button',{name:'今日已领取'})).toBeDisabled();
 expect(request.mock.calls.filter(c=>c[1]==='POST')).toHaveLength(0);
});
