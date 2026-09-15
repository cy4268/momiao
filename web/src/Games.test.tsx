import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { ApiClient, ApiError } from './api';
import { GamePage } from './Games';
import * as gameAPI from './games-api';
import type { GameBootstrap, GameRound } from './games-api';

const id='01993200-0000-7000-8000-000000000001';
const commitment='01993200-0000-7000-8000-000000000002';
const hash='a'.repeat(64);
const result:GameRound={id,game:'dice',state:'SETTLED',recovery_state:'NORMAL',input:{type:'DICE',wager:'10',choice:'SMALL'},total_stake_units:'5000000',total_payout_units:'10000000',net_change_units:'5000000',common_result:'WIN',balance_before_units:'500000000',balance_after_units:'505000000',wager_transaction_id:id,settlement_transaction_id:commitment,config_version_id:id,config_hash:hash,wager_policy_version_id:id,wager_policy_hash:hash,algorithm_version:'dice-map-v1',ruleset_version:'dice-rules-v1',fairness_stream_version:'chaldea-pf-hmac-sha256-v1',nonce:'0',commitment_id:commitment,created_at:'2026-09-06T10:00:00Z',settled_at:'2026-09-06T10:00:00Z',dice:{dice:[2,3,4],total:9,triple:false,side:'SMALL',choice:'SMALL',reward:{cost_multiplier:'1',payout_multiplier:'2',outcome:'WIN'}}};
const bootstrap:GameBootstrap={game:{slug:'dice',title:'命运骰盅',effective_runtime:'PLAY',implementation_key:'direct.dice.v1',config:{version_id:id,hash,schema:'dice-config-v1',ruleset_version:'dice-rules-v1',algorithm_version:'dice-map-v1',statistics:{rtp:'35/36',loss:'37/72',win:'35/72',top:'0',break_even:'0'}}},wager_policy:{version_id:id,version:'1',minimum_wager_units:'5000000',maximum_mode:'NONE',input_step_units:'500000',quick_amount_units:['5000000','50000000','250000000','500000000'],hash},available_units:'500000000',latest_round:null,active_round:null,scratch_presentation_blocker:null,effective_entry_action:'PLAY',next_commitment:{id:commitment,reserved_round_id:id,server_seed_hash:hash,nonce:'0',client_seed:'cs1-fixture',client_seed_version:'1',config_version_id:id,config_hash:hash,ruleset_version:'dice-rules-v1',algorithm_version:'dice-map-v1',fairness_stream_version:'chaldea-pf-hmac-sha256-v1',wager_policy_version_id:id,wager_policy_hash:hash,resource_versions:{}},client_seed_preference:{client_seed:'cs1-fixture',version:'1'},csrf_token:hash};
let client:ApiClient;
beforeEach(()=>{
    sessionStorage.clear();client=new ApiClient();
    vi.spyOn(client,'getSnapshot').mockReturnValue({user:{id:1,username:'fixture',display_name:'Fixture',role:1},ready:true,loggingOut:false,notice:''});
    vi.spyOn(gameAPI,'readGameBootstrap').mockResolvedValue(structuredClone(bootstrap));
});
afterEach(()=>vi.restoreAllMocks());
function show(){return render(<MemoryRouter><GamePage client={client} userID="1" slug="dice"/></MemoryRouter>)}
it('Slot adapter submits total chips only and keeps a lost response locked for reconciliation',async()=>{
    vi.mocked(gameAPI.readGameBootstrap).mockResolvedValue({...bootstrap,game:{...bootstrap.game,slug:'slot',config:undefined}});
    const create=vi.spyOn(gameAPI,'createGame').mockRejectedValue(new ApiError('lost response',0,'',true));
    render(<MemoryRouter><GamePage client={client} userID="1" slug="slot"/></MemoryRouter>);
    await waitFor(()=>expect(screen.getByRole('button',{name:'Spin · 10 筹码'})).toBeEnabled());
    fireEvent.change(screen.getByLabelText('总下注 · 筹码'),{target:{value:'11'}});
    fireEvent.click(screen.getByRole('button',{name:'Spin · 11 筹码'}));
    await screen.findByRole('button',{name:'核对本局'});
    expect(create).toHaveBeenCalledTimes(1);
    expect(create.mock.calls[0][2].input).toEqual({type:'SLOT',total_wager:'11'});
    expect(screen.getByRole('button',{name:'Spin · 11 筹码'})).toBeDisabled();
});
it('Blackjack resumes an active hand during maintenance and locks an uncertain action across refresh',async()=>{
    const active={...result,game:'blackjack',state:'PLAYER_TURN',input:{type:'BLACKJACK',initial_wager:'10'},total_payout_units:'0',net_change_units:'-5000000',common_result:'',balance_after_units:'495000000',settlement_transaction_id:'',settled_at:null,dice:undefined,
        blackjack:{phase:'PLAYER_TURN',round_version:'1',active_hand_id:id,hands:[{hand_id:id,hand_index:0,cards:[7,20],stake_units:'5000000',hand_state:'ACTIVE',value:{hard_total:16,best_total:16,is_soft:false},is_natural:false,payout_units:'0',net_change_units:'0'}],dealer_cards:[4],dealer_revealed:false,legal_actions:['HIT','STAND','DOUBLE','SPLIT'],last_player_action_at:'2026-09-06T10:00:00Z',auto_resolve_at:'2026-09-07T10:00:00Z',total_stake_units:'5000000',total_payout_units:'0',net_change_units:'0'}} as GameRound;
    const split={...active,total_stake_units:'10000000',net_change_units:'-10000000',balance_after_units:'490000000',blackjack:{...active.blackjack!,round_version:'2',total_stake_units:'10000000',hands:[active.blackjack!.hands[0],{...active.blackjack!.hands[0],hand_id:commitment,hand_index:4}]}};
    expect(gameAPI.parseRound(split).blackjack?.hands.map(h=>h.hand_index)).toEqual([0,4]);
    vi.mocked(gameAPI.readGameBootstrap).mockResolvedValue({...bootstrap,game:{...bootstrap.game,slug:'blackjack',effective_runtime:'MAINTENANCE',config:undefined},active_round:active,latest_round:active,next_commitment:null,effective_entry_action:'RESUME'});
    const request=vi.spyOn(client,'request').mockRejectedValue(new ApiError('lost action response',0,'',true));
    const first=render(<MemoryRouter><GamePage client={client} userID="1" slug="blackjack"/></MemoryRouter>);
    await waitFor(()=>expect(screen.getByRole('button',{name:'Stand · 停牌'})).toBeEnabled());
    expect(screen.queryByRole('button',{name:/Deal ·/})).not.toBeInTheDocument();
    expect(screen.getByRole('img',{name:'庄家暗牌，尚未公开'})).toBeVisible();
    fireEvent.click(screen.getByRole('button',{name:'Stand · 停牌'}));
    await screen.findByRole('button',{name:'核对本次行动'});
    expect(request.mock.calls[0][2]).toMatchObject({action_type:'STAND',hand_id:id,expected_round_version:'1'});
    expect(screen.getByRole('button',{name:'Hit · 要牌'})).toBeDisabled();
    first.unmount();request.mockResolvedValue({round:null});
    render(<MemoryRouter><GamePage client={client} userID="1" slug="blackjack"/></MemoryRouter>);
    await screen.findByRole('button',{name:'重试原行动'});
    expect(request.mock.calls.filter(c=>c[1]==='POST')).toHaveLength(1);
    const original=request.mock.calls.find(c=>c[1]==='POST')![2];
    request.mockRejectedValue(new ApiError('still lost',0,'',true));
    fireEvent.click(screen.getByRole('button',{name:'重试原行动'}));
    await waitFor(()=>expect(request.mock.calls.filter(c=>c[1]==='POST')).toHaveLength(2));
    expect(request.mock.calls.filter(c=>c[1]==='POST')[1][2]).toEqual(original);
});
it('publishes commitment before manual wager, locks double click, then displays durable result',async()=>{
    let resolve!:(value:GameRound)=>void;
    const create=vi.spyOn(gameAPI,'createGame').mockReturnValue(new Promise(r=>resolve=r));
    show();await screen.findByText('可用筹码');expect(create).not.toHaveBeenCalled();
    expect(screen.getByText(hash)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('radio',{name:/小/}));
    fireEvent.click(screen.getByRole('button',{name:'掷骰'}));
    expect(create).toHaveBeenCalledTimes(1);
    expect(screen.getByLabelText('基础下注（筹码）')).toBeDisabled();
    expect(create.mock.calls[0][2].input).toEqual({type:'DICE',wager:'10',choice:'SMALL'});
    vi.mocked(gameAPI.readGameBootstrap).mockResolvedValue({...bootstrap,latest_round:result,available_units:result.balance_after_units});
    resolve(result);
    await waitFor(()=>expect(screen.getByLabelText('本局结果')).toHaveTextContent('净赢'));
    expect(within(screen.getByLabelText('本局结果')).getByText('+10')).toBeVisible();
    expect(create).toHaveBeenCalledTimes(1);expect(sessionStorage.getItem(gameAPI.pendingStorage('1','dice'))).toBeNull();
});
it('unknown HTTP outcome survives refresh and reconciles without another POST',async()=>{
    const create=vi.spyOn(gameAPI,'createGame').mockRejectedValue(new ApiError('lost response',0,'',true));
    const find=vi.spyOn(gameAPI,'findPendingGame').mockResolvedValue(result);
    const first=show();await screen.findByRole('button',{name:'掷骰'});
    fireEvent.click(screen.getByRole('radio',{name:/^大/}));
    fireEvent.click(screen.getByRole('button',{name:'掷骰'}));
    await waitFor(()=>expect(screen.getByRole('button',{name:'核对本局'})).toBeVisible());
    expect(sessionStorage.getItem(gameAPI.pendingStorage('1','dice'))).toContain('commitment');first.unmount();
    show();await waitFor(()=>expect(find).toHaveBeenCalledTimes(1));
    await waitFor(()=>expect(screen.getByLabelText('本局结果')).toHaveTextContent('净赢'));
    expect(create).toHaveBeenCalledTimes(1);
});
it('unlocks an unaccepted pending blackjack deal when bootstrap still owns its commitment',async()=>{
    const pending={key:'01993200-0000-7000-8000-000000000003',commitment,input:{type:'BLACKJACK' as const,initial_wager:'100'}};
    sessionStorage.setItem(gameAPI.pendingStorage('1','blackjack'),JSON.stringify(pending));
    vi.spyOn(gameAPI,'findPendingGame').mockResolvedValue(null);
    vi.mocked(gameAPI.readGameBootstrap).mockResolvedValue({...bootstrap,game:{...bootstrap.game,slug:'blackjack',config:undefined},available_units:'200000000'});
    render(<MemoryRouter><GamePage client={client} userID="1" slug="blackjack"/></MemoryRouter>);
    await waitFor(()=>expect(sessionStorage.getItem(gameAPI.pendingStorage('1','blackjack'))).toBeNull());
    expect(screen.getByRole('button',{name:'Deal · 100 筹码'})).toBeEnabled();
    expect(screen.getByText('服务器确认原下注未受理，已解除锁定。')).toBeVisible();
});
it('refuses invalid or unaffordable wagers without calling create while keeping a settled blackjack wager editable',async()=>{
    const create=vi.spyOn(gameAPI,'createGame');const dice=show();await screen.findByRole('button',{name:'掷骰'});
    for(const value of ['9','10.5','-10','1e2','1001']) {
        fireEvent.change(screen.getByLabelText('基础下注（筹码）'),{target:{value}});
        expect(screen.getByRole('button',{name:'掷骰'})).toBeDisabled();
    }
    dice.unmount();
    const settled={...result,game:'blackjack',input:{type:'BLACKJACK',initial_wager:'500'},total_stake_units:'250000000',total_payout_units:'0',net_change_units:'-250000000',common_result:'LOSS',balance_before_units:'300000000',balance_after_units:'50000000',dice:undefined,blackjack:{phase:'SETTLED',round_version:'2',active_hand_id:'',hands:[{hand_id:id,hand_index:0,cards:[7,20],stake_units:'250000000',hand_state:'BUST',value:{hard_total:26,best_total:26,is_soft:false},is_natural:false,result:'BUST',payout_units:'0',net_change_units:'-250000000'}],dealer_cards:[4,5],dealer_revealed:true,dealer_total:{hard_total:11,best_total:21,is_soft:true},legal_actions:[],last_player_action_at:'2026-09-06T10:00:00Z',auto_resolve_at:'2026-09-07T10:00:00Z',total_stake_units:'250000000',total_payout_units:'0',net_change_units:'-250000000',result_class:'LOSS'}} as GameRound;
    vi.mocked(gameAPI.readGameBootstrap).mockResolvedValue({...bootstrap,game:{...bootstrap.game,slug:'blackjack',config:undefined},available_units:'50000000',latest_round:settled});
    render(<MemoryRouter><GamePage client={client} userID="1" slug="blackjack"/></MemoryRouter>);
    const input=await screen.findByLabelText('初始下注 · 筹码');
    await waitFor(()=>expect(input).toHaveValue('500'));
    expect(input).toBeEnabled();expect(screen.getByRole('button',{name:'Deal · 500 筹码'})).toBeDisabled();
    fireEvent.change(input,{target:{value:'9'}});
    expect(input).toBeEnabled();expect(screen.getByRole('button',{name:'Deal · — 筹码'})).toBeDisabled();
    fireEvent.change(input,{target:{value:'100'}});
    expect(input).toBeEnabled();expect(screen.getByRole('button',{name:'Deal · 100 筹码'})).toBeEnabled();
    expect(create).not.toHaveBeenCalled();
});
it('keeps the pre-purchase scratch balance visible until reveal completes',async()=>{
    vi.spyOn(HTMLCanvasElement.prototype,'getContext').mockReturnValue(null);
    const scratch={...result,game:'scratch',input:{type:'SCRATCH',wager:'100'},total_stake_units:'50000000',total_payout_units:'0',net_change_units:'-50000000',common_result:'LOSS',balance_before_units:'500000000',balance_after_units:'450000000',dice:undefined,scratch:{tier:'LOSS',cells:Array.from({length:9},()=>({symbol:'P1',matching:false})),reward:{cost_multiplier:'1',payout_multiplier:'0',outcome:'LOSS'}}} as GameRound;
    const completed={...scratch,presentation_completed_at:'2026-09-06T10:01:00Z'};
    vi.mocked(gameAPI.readGameBootstrap).mockResolvedValueOnce({...bootstrap,game:{...bootstrap.game,slug:'scratch',config:undefined},available_units:'450000000',latest_round:scratch,scratch_presentation_blocker:scratch,effective_entry_action:'RESUME',next_commitment:null}).mockResolvedValue({...bootstrap,game:{...bootstrap.game,slug:'scratch',config:undefined},available_units:'450000000',latest_round:completed});
    vi.spyOn(client,'request').mockResolvedValue(completed);
    render(<MemoryRouter><GamePage client={client} userID="1" slug="scratch"/></MemoryRouter>);
    const balance=()=>screen.getByText('可用筹码').parentElement!;
    await waitFor(()=>expect(within(balance()).getByText('1,000')).toBeVisible());
    expect(within(balance()).queryByText('900')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button',{name:'立即揭晓'}));
    await waitFor(()=>expect(within(balance()).getByText('900')).toBeVisible());
});
it('disables each quick amount by its total cost, including tenfold',async()=>{
    const summonBoot={...bootstrap,game:{...bootstrap.game,slug:'summon'},available_units:'1500000000'};
    vi.mocked(gameAPI.readGameBootstrap).mockResolvedValue(summonBoot);
    render(<MemoryRouter><GamePage client={client} userID="1" slug="summon"/></MemoryRouter>);
    await screen.findByLabelText('基础下注（筹码）');
    await waitFor(()=>expect(screen.getByRole('button',{name:'1000'})).toBeEnabled());
    fireEvent.click(screen.getByRole('radio',{name:'十连召唤 · 10 抽'}));
    expect(screen.getByRole('button',{name:'100'})).toBeEnabled();
    expect(screen.getByRole('button',{name:'500'})).toBeDisabled();
    expect(screen.getByRole('button',{name:'1000'})).toBeDisabled();
});
it('restores last durable wager on first entry without overwriting a live edit',async()=>{
    vi.mocked(gameAPI.readGameBootstrap).mockResolvedValue({...bootstrap,latest_round:{...result,input:{type:'DICE',wager:'100',choice:'SMALL'}}});
    show();await waitFor(()=>expect(screen.getByLabelText('基础下注（筹码）')).toHaveValue('100'));
    expect(screen.getByRole('radio',{name:/小/})).toBeChecked();
    fireEvent.change(screen.getByLabelText('基础下注（筹码）'),{target:{value:'50'}});
    fireEvent.click(screen.getByRole('button',{name:'刷新恢复'}));
    await waitFor(()=>expect(gameAPI.readGameBootstrap).toHaveBeenCalledTimes(2));
    expect(screen.getByLabelText('基础下注（筹码）')).toHaveValue('50');
});
