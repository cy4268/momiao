import { ApiClient, ApiError, errorText } from './api';
import type {SlotResultDTO} from './games/SlotGame';
import type {BlackjackCommand,BlackjackProjectionDTO} from './games/BlackjackGame';

export type GameSlug = 'dice'|'scratch'|'summon'|'slot'|'blackjack';
export type Outcome = 'WIN'|'BREAK_EVEN'|'LOSS';
export type GameInput = {type:'DICE';wager:string;choice:'BIG'|'SMALL'} | {type:'SCRATCH';wager:string} | {type:'SUMMON';base_wager:string;mode:'SINGLE'|'TENFOLD'} | {type:'SLOT';total_wager:string} | {type:'BLACKJACK';initial_wager:string};
export type Reward={cost_multiplier:string;payout_multiplier:string;outcome:Outcome};
export type DiceResult={dice:[number,number,number];total:number;triple:boolean;side:'BIG'|'SMALL'|'TRIPLE';choice:'BIG'|'SMALL';reward:Reward};
export type ScratchResult={tier:string;cells:{symbol:string;matching:boolean}[];reward:Reward};
export type SummonResult={mode:'SINGLE'|'TENFOLD';highest_tier:string;draws:{index:number;tier:string;multiplier:string}[];reward:Reward};
export interface GameRound {
    id:string;game:GameSlug;state:'PLAYER_TURN'|'SETTLED';recovery_state:'NORMAL'|'NEEDS_REVIEW';input:GameInput;
    total_stake_units:string;total_payout_units:string;net_change_units:string;common_result:Outcome|'';
    balance_before_units:string;balance_after_units:string;wager_transaction_id:string;settlement_transaction_id:string;
    config_version_id:string;config_hash:string;wager_policy_version_id:string;wager_policy_hash:string;
    algorithm_version:string;ruleset_version:string;fairness_stream_version:string;nonce:string;commitment_id:string;
    created_at:string;settled_at:string|null;presentation_completed_at?:string;
    dice?:DiceResult;scratch?:ScratchResult;summon?:SummonResult;slot?:SlotResultDTO;blackjack?:BlackjackProjectionDTO;
}
export interface Commitment {
    id:string;reserved_round_id:string;server_seed_hash:string;nonce:string;client_seed:string;client_seed_version:string;
    config_version_id:string;config_hash:string;ruleset_version:string;algorithm_version:string;fairness_stream_version:string;
    wager_policy_version_id:string;wager_policy_hash:string;resource_versions:Record<string,string>;
}
export interface GameConfig {
    version_id:string;hash:string;schema:string;ruleset_version:string;algorithm_version:string;
    prizes?:{multiplier:number;tier:string;weight:number}[];
    statistics:{rtp:string;loss:string;break_even:string;win:string;top:string};
    validation?:{result_json:string;source_artifact_sha256:string;validator_version:string};
}
export interface GameEntry {slug:string;title:string;effective_runtime:string;implementation_key:string;config?:GameConfig}
export interface GameBootstrap {
    game:GameEntry;wager_policy:{version_id:string;version:string;minimum_wager_units:string;maximum_mode:string;input_step_units:string;quick_amount_units:string[];hash:string};
    available_units:string;latest_round:GameRound|null;active_round:GameRound|null;scratch_presentation_blocker:GameRound|null;
    effective_entry_action:string;next_commitment:Commitment|null;client_seed_preference:{client_seed:string;version:string};csrf_token:string;
}
export interface GameVerification {round_id:string;commitment:Commitment;server_seed_hash:string;server_seed:string;reveal_state:string;verified:boolean;canonical_config:string;result:GameRound;blackjack_audit?:unknown}
export interface PendingGame {key:string;commitment:string;input:GameInput}
export interface PendingBlackjackAction {round:string;input:BlackjackCommand&{action_id:string}}
const uuid=/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const max=9223372036854775807n;
export const gameSlugs:GameSlug[]=['dice','scratch','summon','slot','blackjack'];
export const gameNames:Record<string,string>={dice:'命运骰盅',scratch:'星纹刮刮卡',summon:'圣晶召唤',slot:'月光回响',blackjack:'二十一点','texas-holdem':'德州扑克'};
export const outcomeNames:Record<Outcome,string>={WIN:'净赢',BREAK_EVEN:'回本',LOSS:'净输'};
const bad=()=>new ApiError('游戏响应尚未核对，请刷新恢复本局。',0,'GAME_RESPONSE_INVALID',true);
export function units(value:unknown,signed=false):bigint {
    if(typeof value!=='string'||!(signed?/^-?(0|[1-9]\d*)$/:/^(0|[1-9]\d*)$/).test(value)||value.length>20)throw bad();
    const n=BigInt(value);if(n>max||n< -max)throw bad();return n;
}
export function chips(value:string,sign=false):string {
    const n=units(value,true),absolute=n<0n?-n:n;
    const remainder=absolute%500000n;
    const decimal=remainder?'.'+(remainder*2n).toString().padStart(6,'0').replace(/0+$/,''):'';
    return (n<0n?'-':sign&&n>0n?'+':'')+(absolute/500000n).toLocaleString('en-US')+decimal;
}
export function wagerCost(wager:string,mode='SINGLE',game:GameSlug='summon'):bigint|null {
    if(!/^\d{1,19}$/.test(wager))return null;
    const base=BigInt(wager);if(base<10n)return null;
    const multiplier=mode==='TENFOLD'?10n:1n;
    const maximumPayoutMultiplier=game==='dice'?2n:game==='slot'?5164n:game==='blackjack'?17n:100n;
    const divisor=game==='slot'?10n:1n;
    if(base*500000n>max||base*500000n/divisor>max/maximumPayoutMultiplier/multiplier)return null;
    return base*500000n*multiplier;
}
export function parseRound(raw:unknown):GameRound {
    if(!raw||typeof raw!=='object')throw bad();const r=raw as GameRound;
    const blackjack=r.game==='blackjack',settled=r.state==='SETTLED';
    if(!uuid.test(r.id)||!gameSlugs.includes(r.game)||(!settled&&!(blackjack&&r.state==='PLAYER_TURN'))||(r.recovery_state!=='NORMAL'&&!(blackjack&&r.recovery_state==='NEEDS_REVIEW'))||!r.input)throw bad();
    const stake=units(r.total_stake_units),payout=units(r.total_payout_units),net=units(r.net_change_units,true),before=units(r.balance_before_units),after=units(r.balance_after_units);
    if(stake<=0n||net!==payout-stake||(!blackjack&&after!==before+net)||(settled?r.common_result!==(net>0n?'WIN':net===0n?'BREAK_EVEN':'LOSS'):r.common_result!==''||payout!==0n))throw bad();
    if(!uuid.test(r.commitment_id)||!uuid.test(r.wager_transaction_id)||(settled?!uuid.test(r.settlement_transaction_id)||!r.settled_at:r.settlement_transaction_id!==''||r.settled_at!==null)||!/^[0-9a-f]{64}$/.test(r.config_hash)||!/^[0-9a-f]{64}$/.test(r.wager_policy_hash))throw bad();
    if(r.game==='dice'&&(!r.dice||r.dice.dice.length!==3||r.dice.dice.some(n=>!Number.isInteger(n)||n<1||n>6)||r.dice.total!==r.dice.dice.reduce((a,b)=>a+b,0)))throw bad();
    if(r.game==='scratch'&&(!r.scratch||r.scratch.cells.length!==9||r.scratch.cells.some(c=>!/^P(1|2|3|5|10|25|100)$/.test(c.symbol)||typeof c.matching!=='boolean')))throw bad();
    if(r.game==='summon'&&(!r.summon||r.summon.draws.length!==(r.summon.mode==='TENFOLD'?10:1)||r.summon.draws.some((d,i)=>d.index!==i+1||!/^T[0-5]$/.test(d.tier)||units(d.multiplier)>100n)))throw bad();
    if(r.game==='slot'){
        const s=r.slot;const symbols=['L1','L2','L3','M1','M2','H1','H2','W'];
        if(!s||s.stops.length!==5||s.stops.some(n=>!Number.isInteger(n)||n<0||n>31)||s.full_grid.length!==5||s.full_grid.some(reel=>reel.length!==3||reel.some(v=>!symbols.includes(v)))||s.lines.length!==10)throw bad();
        let sum=0n;for(const [i,l] of s.lines.entries()){
            const pay=units(l.line_payout_units);if(l.line_number!==i+1||units(l.line_stake_units)!==stake/10n||!Number.isInteger(l.multiplier)||l.multiplier<0||l.multiplier>5000||pay!==BigInt(l.multiplier)*units(l.line_stake_units))throw bad();sum+=pay;
        }
        if(sum!==payout||units(s.total_wager_units)!==stake||units(s.total_payout_units)!==payout||units(s.net_change_units,true)!==net||s.result_class!==r.common_result)throw bad();
    }
    if(blackjack&&r.recovery_state==='NORMAL'){
        const b=r.blackjack,card=(n:number)=>Number.isInteger(n)&&n>=0&&n<52;
        if(!b||b.phase!==r.state||units(b.round_version)<1n||!Array.isArray(b.hands)||b.hands.length<1||b.hands.length>4||!Array.isArray(b.dealer_cards)||b.dealer_cards.some(n=>!card(n))||b.dealer_revealed!==settled||(!settled&&b.dealer_cards.length!==1)||(settled&&b.dealer_cards.length<2))throw bad();
        let sum=0n,handPayout=0n;const ids=new Set<string>(),indices=new Set<number>();
        for(const h of b.hands){
            if(!uuid.test(h.hand_id)||ids.has(h.hand_id)||!Number.isInteger(h.hand_index)||h.hand_index<0||h.hand_index>7||indices.has(h.hand_index)||!Array.isArray(h.cards)||h.cards.length<2||h.cards.some(n=>!card(n))||!['ACTIVE','STOOD','BUST','DOUBLED_COMPLETE','SPLIT_ACES_COMPLETE','NATURAL_COMPLETE'].includes(h.hand_state)||!h.value||!Number.isInteger(h.value.best_total)||!Number.isInteger(h.value.hard_total)||typeof h.value.is_soft!=='boolean')throw bad();
            ids.add(h.hand_id);indices.add(h.hand_index);const hs=units(h.stake_units),hp=units(h.payout_units),hn=units(h.net_change_units,true);if(hs<=0n||(settled&&hn!==hp-hs))throw bad();sum+=hs;handPayout+=hp;
        }
        const legal=b.legal_actions||[],fairReturn=units(b.fair_return_units||'0');
        if(legal.some(a=>!['HIT','STAND','DOUBLE','SPLIT'].includes(a))||(!settled&&!b.hands.some(h=>h.hand_id===b.active_hand_id&&h.hand_state==='ACTIVE'))||(settled&&(b.active_hand_id!==''||legal.length!==0))||sum!==stake||units(b.total_stake_units)!==stake||units(b.total_payout_units)!==payout||(!settled&&fairReturn!==0n)||(settled&&(handPayout+fairReturn!==payout||units(b.net_change_units,true)!==net||b.result_class!==r.common_result)))throw bad();
    }
    return r;
}
export async function readGameBootstrap(client:ApiClient,slug:GameSlug):Promise<GameBootstrap> {
    const b=await client.request<GameBootstrap>(`/api/v1/games/${slug}/bootstrap`);
    if(!b||b.game?.slug!==slug||typeof b.csrf_token!=='string'||b.csrf_token.length!==64||!b.client_seed_preference)throw bad();
    units(b.available_units);if(b.latest_round)parseRound(b.latest_round);if(b.active_round)parseRound(b.active_round);if(b.scratch_presentation_blocker)parseRound(b.scratch_presentation_blocker);
    if(b.next_commitment&&(!uuid.test(b.next_commitment.id)||!/^[0-9a-f]{64}$/.test(b.next_commitment.server_seed_hash)))throw bad();
    return b;
}
export async function createGame(client:ApiClient,slug:GameSlug,pending:PendingGame,csrf:string):Promise<GameRound> {
    const r=parseRound(await client.request(`/api/v1/games/${slug}/rounds`,'POST',pending.input,{'Idempotency-Key':pending.key,'X-CSRF-Token':csrf,'X-Fairness-Commitment':pending.commitment}));
    if(r.game!==slug||r.commitment_id!==pending.commitment)throw bad();return r;
}
export async function findPendingGame(client:ApiClient,slug:GameSlug,key:string):Promise<GameRound|null> {
    const response=await client.request<{round:unknown}>(`/api/v1/games/${slug}/rounds/by-key?key=${encodeURIComponent(key)}`);
    if(!response||!('round'in response))throw bad();return response.round===null?null:parseRound(response.round);
}
export function gameUUID():string {
    const b=crypto.getRandomValues(new Uint8Array(16));let ms=BigInt(Date.now());for(let i=5;i>=0;i--){b[i]=Number(ms&255n);ms>>=8n;}b[6]=(b[6]&15)|0x70;b[8]=(b[8]&63)|0x80;
    const s=Array.from(b,x=>x.toString(16).padStart(2,'0')).join('');return `${s.slice(0,8)}-${s.slice(8,12)}-${s.slice(12,16)}-${s.slice(16,20)}-${s.slice(20)}`;
}
export function pendingStorage(user:string,slug:GameSlug) {return `chaldea.game.pending.v1.${user}.${slug}`;}
export function blackjackActionStorage(user:string){return `chaldea.blackjack.action.v1.${user}`;}
export function readBlackjackAction(user:string):PendingBlackjackAction|null {
    const raw=sessionStorage.getItem(blackjackActionStorage(user));if(!raw)return null;const p=JSON.parse(raw) as PendingBlackjackAction;
    if(!p||!uuid.test(p.round)||!p.input||!uuid.test(p.input.action_id)||!uuid.test(p.input.hand_id)||units(p.input.expected_round_version)<1n||!['HIT','STAND','DOUBLE','SPLIT'].includes(p.input.action_type))throw bad();return p;
}
export async function findBlackjackAction(client:ApiClient,p:PendingBlackjackAction):Promise<GameRound|null>{
    const data=await client.request<{round:unknown}>(`/api/v1/game-rounds/${p.round}/actions/${p.input.action_id}`);
    if(!data||!('round'in data))throw bad();return data.round===null?null:parseRound(data.round);
}
export function readPending(user:string,slug:GameSlug):PendingGame|null {
    const raw=sessionStorage.getItem(pendingStorage(user,slug));if(!raw)return null;
    const p=JSON.parse(raw) as PendingGame;
    if(!p||Object.keys(p).sort().join(',')!=='commitment,input,key'||!uuid.test(p.key)||!uuid.test(p.commitment)||p.input?.type!==slug.toUpperCase())throw new Error('待核对请求记录异常，请先查看历史并刷新恢复。');
    return p;
}
export function gameError(error:unknown):string {
    const messages:Record<string,string>={BLACKJACK_ACTIVE_ROUND_EXISTS:'已有未结束牌局，请恢复当前局。',BLACKJACK_NEEDS_REVIEW:'本局需要核对，行动已暂停。请保留本局编号。',BLACKJACK_STALE_ROUND_VERSION:'本局已在另一页面更新，已重新读取当前手牌。',BLACKJACK_HAND_NOT_ACTIVE:'当前行动手牌已改变，请根据最新牌局继续。',BLACKJACK_ACTION_NOT_ALLOWED:'当前手牌不允许此行动，请刷新恢复。',BLACKJACK_MAX_HANDS_REACHED:'本局已达到最多四手。',BLACKJACK_INSUFFICIENT_CHIPS_FOR_DOUBLE:'可用筹码不足以加倍。',BLACKJACK_INSUFFICIENT_CHIPS_FOR_SPLIT:'可用筹码不足以分牌。',INSUFFICIENT_CHIPS:'可用筹码不足。前往钱包兑换筹码，或领取每日签到奖励。',GAME_MAINTENANCE:'游戏维护中，已完成的局仍可查看。',FAIRNESS_COMMITMENT_INVALID:'本次预承诺已更新或在另一设备使用，请刷新后重新选择。',SCRATCH_PREVIOUS_REVEAL_INCOMPLETE:'请先揭晓上一张刮刮卡，再购买新卡。',IDEMPOTENCY_CONFLICT:'请求编号已对应其他下注，请恢复并核对原局。',GAME_AMOUNT_OVERFLOW:'金额超出可安全结算的范围，请减少下注。',GAME_INVALID_REQUEST:'请检查下注金额与选项。',GAME_TEMPORARILY_UNAVAILABLE:'游戏暂时不可用，请稍后刷新。',CSRF_REJECTED:'页面验证已更新，请刷新后继续。'};
    return error instanceof ApiError&&messages[error.code]?messages[error.code]:errorText(error);
}
export function fractionPercent(value:string):string {const [a,b='1']=value.split('/');return (Number(a)/Number(b)*100).toLocaleString('zh-CN',{maximumFractionDigits:4})+'%';}
