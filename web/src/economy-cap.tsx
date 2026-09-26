import { z } from 'zod';
import './economy-cap.css';

const amount=z.string().regex(/^(0|[1-9]\d*)$/).max(19).refine(v=>BigInt(v)<=9223372036854775807n);
const signed=z.string().regex(/^(0|-?[1-9]\d*)$/).max(20).refine(v=>BigInt(v)>=-9223372036854775808n&&BigInt(v)<=9223372036854775807n);
const hash=z.string().regex(/^[a-f0-9]{64}$/),version=z.string().min(1).max(64);
export const economicPolicySchema=z.object({version,hash,single_player_max_units:amount,asset_cap_units:amount,cap_mode:z.literal('CLIP_PROFIT')}).strict();
export const capSettlementSchema=z.object({policy_version:version,policy_hash:hash,gross_payout_units:amount,credited_payout_units:amount,withheld_units:amount,actual_net_units:signed,neutral_return_units:amount.optional()}).strict().refine(v=>BigInt(v.gross_payout_units)===BigInt(v.credited_payout_units)+BigInt(v.withheld_units)&&BigInt(v.actual_net_units)<=BigInt(v.credited_payout_units)&&(v.withheld_units==='0'||BigInt(v.actual_net_units)>=0n));
export type EconomicPolicy=z.infer<typeof economicPolicySchema>;
export type CapSettlement=z.infer<typeof capSettlementSchema>;
function display(raw:string,sign=false){const n=BigInt(raw),a=n<0n?-n:n,f=((a%500000n)*2n).toString().padStart(6,'0').replace(/0+$/,'');return `${n<0n?'-':sign&&n>0n?'+':''}${(a/500000n).toLocaleString('en-US')}${f?'.'+f:''}`}
export function capResult(c:CapSettlement|undefined,fallback:string):'WIN'|'BREAK_EVEN'|'LOSS'{if(!c)return fallback==='WIN'?'WIN':fallback==='BREAK_EVEN'?'BREAK_EVEN':'LOSS';const n=BigInt(c.actual_net_units);return n>0n?'WIN':n===0n?'BREAK_EVEN':'LOSS'}
export function CapReceipt({receipt}:{receipt?:CapSettlement}){
 if(!receipt)return null;
 return <div className="economy-cap-receipt" role="group" aria-label="经济结算回执"><dl><div><dt>原规则派彩</dt><dd>{display(receipt.gross_payout_units)}</dd></div><div><dt>实际到账</dt><dd>{display(receipt.credited_payout_units)}</dd></div><div><dt>封顶未入账</dt><dd>{display(receipt.withheld_units)}</dd></div><div><dt>实际净变化</dt><dd>{display(receipt.actual_net_units,true)}</dd></div>{receipt.neutral_return_units&&<div><dt>未跟注本金退回</dt><dd>{display(receipt.neutral_return_units)}</dd></div>}</dl>{receipt.withheld_units!=='0'&&<p>资产封顶仅扣减超额利润，本金返还不受影响；原始游戏结果与公平验证保持不变。</p>}</div>
}
export function CapRules({active,single=false}:{active:boolean;single?:boolean}){return active?<p className="economy-cap-rules">{single?'单人游戏每局累计下注上限 1,000,000 筹码；十连、分牌与加倍均计入整局。':'多人游戏不受单人每局百万下注上限约束。'}统一资产上限 100,000,000,000 API Credits；仅在结算时裁剪超额利润，不削减本金或退款，以本局锁定政策为准。</p>:null}
