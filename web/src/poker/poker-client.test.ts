import { describe, expect, it } from 'vitest';
import { parsePokerTableView } from './poker-view';
import { acceptPokerSnapshot, matchesPokerIntentContext, pokerEnvelopeCounter, type PokerReadScope } from './poker-client-boundary';
import { stateLabel } from './poker-ui-types';
import g3Public from './fixtures/g3-public.json';
import g3Self from './fixtures/g3-player-self.json';

const tableID='019a0000-0000-7000-8000-000000000001',sessionID='019a0000-0000-7000-8000-000000000002',handID='019a0000-0000-7000-8000-000000000003';
const chip=(n:string)=>(BigInt(n)*500000n).toString();
const host=()=>({is_host:true,capabilities:[],players:[],spectators:[],spectators_truncated:false,chat_targets:[],chat_targets_truncated:false});
function snapshot(): any {
  return {table_id:tableID,name:'月光长廊',lifecycle_state:'IN_HAND',table_version:'8',max_seats:2,
    small_blind_units:chip('5'),big_blind_units:chip('10'),settings_locked:true,server_now:'2026-09-06T12:00:10.123456789Z',
    seats:[{seat_no:1,display_name:'御主',state:'ACTIVE',is_self:true,connected:true,stack_units:chip('780'),street_committed_units:chip('20'),total_committed_units:chip('40'),is_folded:false,is_all_in:false,hole_card_count:2,hole_cards:[0,13],hole_cards_released:false,sit_out_next_hand:false,leave_after_hand:false,pending_top_up_units:'0'},
      {seat_no:2,display_name:'Moonlit',state:'ACTIVE',is_self:false,connected:true,stack_units:chip('600'),street_committed_units:chip('50'),total_committed_units:chip('80'),is_folded:false,is_all_in:false,hole_card_count:2,hole_cards_released:false,sit_out_next_hand:false,leave_after_hand:false,pending_top_up_units:'0'}],
    hand:{hand_id:handID,hand_version:'6',street:'FLOP',button_seat:2,actor_seat:1,board_cards:[28,42,8],pot_units:chip('101'),pots:[{index:0,amount_units:chip('101'),eligible_seats:[1,2],awards:[]}],action_sequence:'4',action_deadline_at:'2026-09-06T12:00:30Z',recovering:false,server_seed_hash:'a'.repeat(64),deck_hash:'b'.repeat(64)},
    viewer:{session_id:sessionID,seat_no:1,control_epoch:'3',can_act:true,can_top_up:true,can_leave:true,can_resume:false,can_sit_out:true,can_start:false,top_up_min_units:chip('1'),top_up_max_units:chip('200'),legal:{actions:['FOLD','CALL','RAISE','ALL_IN'],to_call_units:chip('30'),call_applied_units:chip('30'),minimum_bet_units:chip('10'),minimum_raise_to_units:chip('80'),maximum_raise_to_units:chip('800'),raise_rights:true,shortcuts:[{name:'1/2 Pot',action_type:'RAISE',target_to_units:chip('115')}] }},
    timeline:{hand_id:handID,last_sequence:'0',truncated:false,events:[]}};
}
const parse=(v:unknown,kind:'PLAYER_SELF'|'SPECTATOR'|'HOST'|'OPS'='PLAYER_SELF')=>parsePokerTableView(v,tableID,kind);
const scope:PokerReadScope={user_id:'42',session_generation:3,request_generation:7,table_id:tableID,viewer_kind:'PLAYER_SELF'};
const context={user_id:'42',runtime_id:'runtime-1',event_sequence:'4',table_id:tableID,table_version:'8',hand_id:handID,hand_version:'6',action_sequence:'4',session_id:sessionID,control_epoch:'3'};

describe('exact G3 Poker viewer decoder',()=>{
  it('accepts a valid 40-grapheme table name above 128 UTF-8 bytes in the actual G3 DTO shape',()=>{
    const raw=structuredClone(g3Self);raw.name='👨‍👩‍👧‍👦'.repeat(40);
    expect([...new Intl.Segmenter('zh',{granularity:'grapheme'}).segment(raw.name)]).toHaveLength(40);
    expect(new TextEncoder().encode(raw.name).length).toBeGreaterThan(128);
    expect(parsePokerTableView(raw,raw.table_id,'PLAYER_SELF').name).toBe(raw.name);
  });
  it('accepts both actual isolated PG → Service.View → json.Marshal projections without a compatibility branch',()=>{
    const publicView=parsePokerTableView(g3Public,g3Public.table_id,'SPECTATOR');
    const selfView=parsePokerTableView(g3Self,g3Self.table_id,'PLAYER_SELF');
    expect(publicView.hand?.board_cards).toEqual([]);expect(publicView.seats.every(s=>!s.is_self&&!Object.hasOwn(s,'hole_cards'))).toBe(true);
    expect(selfView.viewer.can_act).toBe(true);expect(selfView.seats.find(s=>s.is_self)?.hole_cards).toHaveLength(2);
    expect(selfView.viewer.legal?.shortcuts.map(s=>s.name)).toEqual(['Min','1/2 Pot','2/3 Pot','Pot','All-in']);
    expect(()=>parsePokerTableView(g3Public,g3Public.table_id,'UNKNOWN' as 'SPECTATOR')).toThrow();
  });
  it('accepts numeric cards and exact large Chip strings, returning a detached full projection',()=>{
    const raw=snapshot();raw.seats[0].stack_units='9007199255000000';raw.table_version='18446744073709551615';
    const value=parse(raw);expect(value.seats[0].stack_units).toBe('9007199255000000');expect(value.table_version).toBe('18446744073709551615');
    raw.seats[0].hole_cards[0]=51;expect(value.seats[0].hole_cards).toEqual([0,13]);
  });
  it('accepts the real empty-table and COMMITTED snapshots without inventing a hand or legal actions',()=>{
    const raw=snapshot();raw.lifecycle_state='WAITING';raw.settings_locked=false;raw.seats=[];delete raw.hand;delete raw.timeline.hand_id;
    raw.viewer={control_epoch:'0',can_act:false,can_top_up:false,can_leave:false,can_resume:false,can_sit_out:false,can_start:true,top_up_min_units:'0',top_up_max_units:'0'};raw.host=host();
    expect(parse(raw,'HOST').seats).toEqual([]);expect(parse(raw,'HOST').hand).toBeUndefined();
    raw.hand={...snapshot().hand,street:'COMMITTED',board_cards:[],pots:[],pot_units:'0',hand_version:'0',action_sequence:'0',actor_seat:0};raw.timeline.hand_id=raw.hand.hand_id;delete raw.hand.action_deadline_at;
    expect(parse(raw,'HOST').hand?.board_cards).toEqual([]);
  });
  it.each(['AA0=','',null])('rejects byte-slice/Base64 board representation %s',value=>{
    const raw=snapshot();raw.hand.board_cards=value;expect(()=>parse(raw)).toThrow();
  });
  it.each(['hole_cards','public_hole_cards'])('rejects Base64 %s instead of silently decoding it',key=>{
    const raw=snapshot();raw.seats[0][key]='AA0=';if(key==='public_hole_cards')raw.seats[0].hole_cards_released=true;expect(()=>parse(raw)).toThrow();
  });
  it('accepts only explicitly released, nonfolded public cards and never another seat private field',()=>{
    const raw=snapshot();raw.seats[1].public_hole_cards=[12,25];raw.seats[1].hole_cards_released=true;
    expect(parse(raw).seats[1].public_hole_cards).toEqual([12,25]);
    raw.seats[1].hole_cards_released=false;expect(()=>parse(raw)).toThrow();raw.seats[1].hole_cards_released=true;
    raw.seats[1].is_folded=true;expect(()=>parse(raw)).toThrow();raw.seats[1].is_folded=false;
    raw.seats[1].hole_cards=[];expect(()=>parse(raw)).toThrow();
  });
  it.each(['SPECTATOR','HOST','OPS'] as const)('rejects self/private projection in a %s read',kind=>{
    expect(()=>parse(snapshot(),kind)).toThrow();const raw=snapshot();raw.seats[0].is_self=false;delete raw.seats[0].hole_cards;
    raw.viewer={control_epoch:'0',can_act:false,can_top_up:false,can_leave:false,can_resume:false,can_sit_out:false,can_start:false,top_up_min_units:'0',top_up_max_units:'0'};if(kind==='HOST')raw.host=host();
    expect(parse(raw,kind).seats.every(s=>!s.is_self&&s.hole_cards===undefined)).toBe(true);
  });
  it.each([
    (v:any)=>{v.deck=[0,1,2];},(v:any)=>{v.hand.server_seed='unreleased';},(v:any)=>{v.hand.events=[];},
    (v:any)=>{v.seats[0].stack_units=500000;},(v:any)=>{v.seats[0].stack_units='500001';},(v:any)=>{v.seats[0].stack_units='9223372036855000000';},
    (v:any)=>{v.table_version='08';},(v:any)=>{v.table_version='18446744073709551616';},(v:any)=>{delete v.viewer.can_sit_out;},
    (v:any)=>{v.seats[1].seat_no=1;},(v:any)=>{v.viewer.seat_no=2;},(v:any)=>{v.hand.board_cards=[28,28,8];},
    (v:any)=>{v.hand.board_cards=[0,42,8];},(v:any)=>{v.seats[1].public_hole_cards=[0,25];v.seats[1].hole_cards_released=true;},
    (v:any)=>{v.viewer.legal.actions.push('POST_BB_NOW');},(v:any)=>{v.viewer.can_act=true;v.hand.actor_seat=2;},
    (v:any)=>{v.viewer.can_act=true;v.hand.action_deadline_at=v.server_now;},(v:any)=>{v.server_now='2026-02-30T12:00:00Z';},
  ])('fails closed for malformed or inconsistent wire shape %#',mutate=>{const raw=snapshot();mutate(raw);expect(()=>parse(raw)).toThrow();});
  it('does not echo a private payload in a protocol failure',()=>{
    const raw=snapshot();raw.hand.server_seed='PRIVATE_MARKER';try{parse(raw);throw new Error('decoder accepted');}catch(e){expect(String(e)).not.toContain('PRIVATE_MARKER');}
  });
});

describe('client-only Poker acceptance fences',()=>{
  it('rejects invalid local scopes instead of accepting an unbound authenticated identity',()=>{
    for(const next of [{...scope,user_id:'0'},{...scope,user_id:'9223372036854775808'},{...scope,session_generation:-1}])expect(()=>acceptPokerSnapshot(snapshot(),next,next)).toThrow();
  });
  it('rejects old identity/session/request/table/viewer replies before decoding their payload',()=>{
    for(const next of [{...scope,user_id:'43'},{...scope,session_generation:4},{...scope,request_generation:8},{...scope,table_id:handID},{...scope,viewer_kind:'SPECTATOR' as const},null]){
      expect(acceptPokerSnapshot({hand:{server_seed:'old private result'}},scope,next)).toBeUndefined();
    }
  });
  it('replaces complete snapshots and rejects stale durable versions without fabricating sequence gaps',()=>{
    const previous=acceptPokerSnapshot(snapshot(),scope,scope)!;
    const next=snapshot();next.table_version='9';next.hand.hand_version='7';delete next.hand.action_deadline_at;next.viewer.can_act=false;delete next.viewer.legal;
    const accepted=acceptPokerSnapshot(next,scope,scope,previous)!;expect(accepted.viewer.legal).toBeUndefined();expect(accepted.hand?.action_deadline_at).toBeUndefined();
    next.table_version='7';expect(acceptPokerSnapshot(next,scope,scope,previous)).toBeUndefined();
    next.table_version='9';next.hand.hand_version='5';expect(acceptPokerSnapshot(next,scope,scope,previous)).toBeUndefined();
    next.hand.hand_version='7';next.viewer.control_epoch='2';expect(acceptPokerSnapshot(next,scope,scope,previous)).toBeUndefined();
  });
  it('compares every current rendered fence without treating a match as authorization',()=>{
    expect(matchesPokerIntentContext(context,{...context})).toBe(true);
    for(const key of Object.keys(context))expect(matchesPokerIntentContext(context,{...context,[key]:`${context[key as keyof typeof context]}x`})).toBe(false);
    expect(matchesPokerIntentContext({...context,control_epoch:undefined},{...context,control_epoch:undefined})).toBe(false);
    expect(matchesPokerIntentContext({...context,event_sequence:'04'},{...context,event_sequence:'04'})).toBe(false);
  });
  it('keeps frozen WS envelope counters numeric and converts only safe integers losslessly',()=>{
    expect(pokerEnvelopeCounter(9007199254740991)).toBe('9007199254740991');expect(pokerEnvelopeCounter(0)).toBe('0');
    for(const value of ['3',9007199254740992,NaN,Infinity,-1,-0,1.5,null])expect(()=>pokerEnvelopeCounter(value)).toThrow();
  });
  it('labels the actual service seat/lifecycle/hand states rather than exposing internal state names',()=>{
    expect(stateLabel('ACTIVE')).toBe('参与本手');expect(stateLabel('WAITING_BIG_BLIND')).toBe('等待大盲');
    expect(stateLabel('REBUY_WINDOW')).toBe('等待补充筹码');expect(stateLabel('COMMITTED')).toBe('本手已承诺 · 等待发牌');expect(stateLabel('RECOVERING')).toBe('服务恢复中');
  });
});
