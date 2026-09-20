import {z} from 'zod';
import {ApiClient, ApiError} from '../api';
import {gameUUID} from '../games-api';

export const rouletteSlugs=['devil-roulette','pressure-roulette'] as const;
export type RouletteSlug=typeof rouletteSlugs[number];
export const rouletteNames:Record<RouletteSlug,string>={'devil-roulette':'恶魔轮盘','pressure-roulette':'加压轮盘'};
export const int64=z.string().regex(/^(0|[1-9]\d*)$/).max(19).refine(v=>BigInt(v)<=9223372036854775807n);
const version=int64.refine(v=>BigInt(v)>0n), hash=z.string().regex(/^[0-9a-f]{64}$/);
export const uuid=z.string().regex(/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
const seat=z.number().int().min(0).max(5), date=z.string().datetime({offset:true});
const item=z.enum(['handcuffs','adrenaline','saw','magnifier','inverter','phone','beer','cigarette','medicine']);
const simpleKinds=['JOIN','LEAVE','READY','UNREADY','CANCEL','SURRENDER','PRESSURE_FIRE','PRESSURE_PASS','PRESSURE_AGAIN','PRESSURE_CHARGE','PRESSURE_UNLOAD','PRESSURE_RIPOSTE'] as const;
export const actionSchema=z.union([
 z.object({kind:z.enum(simpleKinds)}).strict(),
 z.object({kind:z.literal('DEVIL_SHOOT'),target:z.enum(['SELF','OPPONENT'])}).strict(),
 z.object({kind:z.literal('DEVIL_ITEM'),item,stolen_item:item.optional()}).strict().refine(a=>a.item==='adrenaline'?!!a.stolen_item&&a.stolen_item!=='adrenaline':!a.stolen_item),
 z.object({kind:z.literal('PRESSURE_VOTE'),agree:z.boolean()}).strict(),
]);
export type Action=z.infer<typeof actionSchema>;
export const bindingSchema=z.object({config_version_id:uuid,config_hash:hash,wager_policy_version_id:uuid,wager_policy_hash:hash,ruleset_version:z.enum(['momiao-devil-rules-v1','momiao-pressure-rules-v1']),algorithm_version:z.enum(['momiao-devil-rng-v1','momiao-pressure-rng-v1']),fairness_stream_version:z.literal('chaldea-pf-hmac-sha256-v1')}).strict();
export const playerSchema=z.object({seat,name:z.string().max(128),ready:z.boolean(),alive:z.boolean(),forfeited:z.boolean(),hp:z.number().int().min(0).max(4),item_count:z.number().int().min(0).max(4)}).strict();
export const eventSchema=z.object({sequence:version,at:date,seat:seat.nullable(),kind:z.string(),text:z.string().max(1000)}).strict();
export const devilSchema=z.object({remaining:z.number().int().min(0).max(8),live:z.number().int().min(0).max(8).nullable(),blank:z.number().int().min(0).max(8).nullable(),saw:z.boolean(),cuffed:z.tuple([z.boolean(),z.boolean()])}).strict();
export const pressureSchema=z.object({actual_load:z.number().int().min(0).max(6),forced_shots:z.number().int().min(1).max(2),order:z.array(seat).min(0).max(6),pointer:z.number().int().min(0).max(5),unload_skips_shot:z.boolean(),timeout_tier:z.number().int().min(0).max(2),phase:z.enum(['FIRE','CHOICE','VOTE','ENDED']),chambers:z.array(z.enum(['UNKNOWN','EMPTY','LIVE_SPENT','DUD_SPENT'])).length(6),pool_remaining:z.number().int().min(0).max(9),duds:z.number().int().min(1).max(3),charge:z.number().int().min(0),loaded:z.number().int().min(0).max(6),forced:z.number().int().min(0).max(2),aggressor:seat.nullable(),riposte_target:seat.nullable(),votes:z.record(z.string(),z.boolean())}).strict();
const state=z.enum(['WAITING','PLAYING','SETTLING','FINISHED','CANCELLED','NEEDS_REVIEW']);
const roomBase=z.object({id:uuid,game:z.enum(rouletteSlugs),title:z.string(),version,sequence:int64,state,target_players:z.number().int().min(2).max(6),stake_units:int64,pool_units:int64,binding:bindingSchema,server_seed_hash:hash,turn_seat:seat.nullable(),server_now:date,deadline:date.nullable(),game_deadline:date.nullable(),players:z.array(playerSchema).max(6),self:z.object({seat,available_units:int64,items:z.array(item).max(4),intel:z.array(z.object({index:z.number().int().min(0).max(7),live:z.boolean()}).strict()).max(8)}).strict().nullable(),actions:z.array(actionSchema).max(32),log:z.array(eventSchema).max(100),devil:devilSchema.nullable(),pressure:pressureSchema.nullable()}).strict();
export const roomSchema=roomBase.superRefine((r,ctx)=>{
 const fail=()=>ctx.addIssue({code:'custom',message:'轮盘状态与动作不一致'});
 if(new Set(r.players.map(p=>p.seat)).size!==r.players.length||r.players.some(p=>p.seat>=r.target_players))fail();
 if(r.game==='devil-roulette'&&(r.target_players!==2||r.pressure!==null||r.binding.ruleset_version!=='momiao-devil-rules-v1'||r.binding.algorithm_version!=='momiao-devil-rng-v1'))fail();
 if(r.game==='pressure-roulette'&&(r.target_players<3||r.devil!==null||r.binding.ruleset_version!=='momiao-pressure-rules-v1'||r.binding.algorithm_version!=='momiao-pressure-rng-v1'))fail();
 if(r.state==='PLAYING'&&(!r.deadline||!r.game_deadline||r.turn_seat===null||(!r.devil&&!r.pressure)))fail();
 if(r.state!=='PLAYING'&&r.turn_seat!==null)fail();
 for(const a of r.actions){
  if(r.state==='WAITING'){if(!['JOIN','READY','UNREADY','LEAVE','CANCEL'].includes(a.kind))fail();}
  else if(r.state!=='PLAYING'||!r.self)fail();
  else if(a.kind!=='SURRENDER'){
   if(a.kind.startsWith('DEVIL_')){if(r.game!=='devil-roulette'||r.turn_seat!==r.self.seat)fail();}
   else if(a.kind.startsWith('PRESSURE_')){if(r.game!=='pressure-roulette'||!r.pressure)fail();else if(a.kind==='PRESSURE_VOTE'?r.pressure.phase!=='VOTE':r.pressure.phase==='VOTE'||r.turn_seat!==r.self.seat)fail();}
   else fail();
  }
 }
});
export type RoomView=z.infer<typeof roomSchema>;
// Lobby summaries deliberately carry no private rule state.
const lobbyRoom=roomBase.extend({binding:z.object({config_version_id:z.string(),config_hash:z.string(),wager_policy_version_id:z.string(),wager_policy_hash:z.string(),ruleset_version:z.string(),algorithm_version:z.string(),fairness_stream_version:z.string()}).strict(),server_seed_hash:z.string()});
const lobbySchema=z.object({game:z.enum(rouletteSlugs),state:z.enum(['PLAY','MAINTENANCE','COMING_SOON','RETIRED','TEMPORARILY_UNAVAILABLE']),binding:bindingSchema,minimum_units:int64,step_units:int64,available_units:int64,own_round_id:uuid.nullable(),rooms:z.array(lobbyRoom).max(50),next_cursor:z.string().max(256).nullable(),csrf_token:z.string().regex(/^[0-9a-f]{64}$/)}).strict();
export type Lobby=z.infer<typeof lobbySchema>;
export const receiptSchema=z.object({round_id:uuid,version,sequence:int64,state}).strict();
const seed=z.string().refine(v=>new TextEncoder().encode(v).length>=1&&new TextEncoder().encode(v).length<=128&&!/[\p{Cc}]/u.test(v));
export const readySchema=z.object({client_seed:seed,config_hash:hash,policy_hash:hash,server_seed_hash:hash,stake_units:int64}).strict();
const createSchema=z.object({game:z.enum(rouletteSlugs),stake:z.string().regex(/^\d+(\.\d{1,6})?$/).max(32),players:z.number().int().min(2).max(6)}).strict().refine(v=>v.game==='devil-roulette'?v.players===2:v.players>=3);
const commandSchema=z.object({expected_version:version,action:actionSchema,ready:readySchema.optional()}).strict().refine(v=>(v.action.kind==='READY')===!!v.ready);
const pendingSchema=z.discriminatedUnion('kind',[
 z.object({kind:z.literal('create'),key:uuid,user_id:version,generation:z.number().int().min(0),body:createSchema}).strict(),
 z.object({kind:z.literal('command'),key:uuid,user_id:version,generation:z.number().int().min(0),round_id:uuid,body:commandSchema}).strict(),
]);
export type Pending=z.infer<typeof pendingSchema>;
export const pendingSlot=(user:string,generation:number,round:string)=>`momiao.roulette.pending.v1.${user}.${generation}.${round}`;
export function readPending(slot:string,user:string,generation:number,round?:string):Pending|null{
 const raw=sessionStorage.getItem(slot);if(!raw)return null;const p=pendingSchema.parse(JSON.parse(raw));
 if(p.user_id!==user||p.generation!==generation||(round?p.kind!=='command'||p.round_id!==round:p.kind!=='create'))throw new Error('恢复标识与当前账户不一致，请重新核对。');return p;
}
export function persist(slot:string,p:Pending){sessionStorage.setItem(slot,JSON.stringify(pendingSchema.parse(p)));}
export function newSeed(){return Array.from(crypto.getRandomValues(new Uint8Array(32)),x=>x.toString(16).padStart(2,'0')).join('');}
export const newKey=gameUUID;
export async function readLobby(client:ApiClient,game:RouletteSlug,cursor=''){return lobbySchema.parse(await client.request(`/api/v1/roulette?game=${game}${cursor?'&cursor='+encodeURIComponent(cursor):''}`));}
export async function readRoom(client:ApiClient,id:string){return roomSchema.parse(await client.request(`/api/v1/roulette/rooms/${uuid.parse(id)}`));}
export async function findReceipt(client:ApiClient,key:string){try{return receiptSchema.parse(await client.request(`/api/v1/roulette/receipts/${uuid.parse(key)}`));}catch(e){if(e instanceof ApiError&&e.status===404)return null;throw e;}}
export async function sendPending(client:ApiClient,p:Pending,csrf:string,current:()=>boolean){
 p=pendingSchema.parse(p);const path=p.kind==='create'?'/api/v1/roulette/rooms':`/api/v1/roulette/rooms/${p.round_id}/commands`;
 return receiptSchema.parse(await client.request(path,'POST',p.body,{'Idempotency-Key':p.key,'X-CSRF-Token':csrf},current));
}
export function rouletteError(error:unknown){
 const codes:Record<string,string>={ROULETTE_VERSION_CONFLICT:'桌面已经变化，请查看最新状态后重新选择。',ROULETTE_ACTION_INVALID:'现在不能执行这个动作，请查看当前回合。',ROULETTE_ALREADY_SEATED:'你已有未结束的轮盘房间，请先返回原房间。',ROULETTE_UNAVAILABLE:'当前暂不可用；已有托管保留，请稍后重新核对。',ROULETTE_IDEMPOTENCY_CONFLICT:'恢复标识与原操作不一致，请核对原记录。',INSUFFICIENT_CHIPS:'可用筹码不足，尚未准备。',GAME_AMOUNT_OVERFLOW:'筹码金额超过可处理范围。'};
 return error instanceof ApiError?codes[error.code]||error.message:error instanceof z.ZodError?'服务器数据未通过校验，请刷新核对。':error instanceof Error?error.message:'读取失败，请重试。';
}
export const itemNames:Record<string,string>={handcuffs:'手铐',adrenaline:'肾上腺素',saw:'手锯',magnifier:'放大镜',inverter:'逆转器',phone:'手机',beer:'啤酒',cigarette:'香烟',medicine:'过期药'};
export const stateNames:Record<string,string>={WAITING:'等待准备',PLAYING:'对局中',SETTLING:'结算中',FINISHED:'已结算',CANCELLED:'已取消 · 托管已退回',NEEDS_REVIEW:'待人工核对 · 托管保留'};
