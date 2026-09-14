import { describe, expect, it } from 'vitest';
import { encodePokerClient, parsePokerServer, parsePokerServerText } from './poker-wire';
import self from './fixtures/g3-player-self.json';
import publicView from './fixtures/g3-public.json';

const request='request-00000001',action='action-000000001';
const refs={request_id:request,action_id:action,table_id:self.table_id,hand_id:self.hand.hand_id,table_version:self.table_version,hand_version:self.hand.hand_version,control_epoch:self.viewer.control_epoch};
function wireView(source:any=self):any {const v=structuredClone(source);v.viewer.control={connection_id:'a'.repeat(32),mode:v.viewer.session_id?'CONTROLLER':'READ_ONLY',control_epoch:v.viewer.control_epoch,...(v.viewer.session_id?{session_id:v.viewer.session_id}:{})};return v;}
function frame(type='table.snapshot',payload:unknown=wireView()):any {return {type,event_id:'opaque-event-id',event_seq:9,table_id:self.table_id,table_version:7,hand_id:self.hand.hand_id,hand_version:8,server_time:self.server_now,payload};}
const read=(raw:unknown)=>parsePokerServer(raw,self.table_id,'PLAYER_SELF');
const receipt=()=>({table_id:self.table_id,session_id:self.viewer.session_id,status:'PENDING',amount_units:'500000',table_version:'8',duplicate:false});

describe('frozen Poker WS codec',()=>{
  it('checks exact envelope and source metadata without converting payload versions/assets',()=>{
    const raw=frame(),value=read(raw);expect(value.event_seq).toBe('9');expect(value.table_version).toBe('7');
    expect(value.type).toBe('table.snapshot');if(value.type!=='table.snapshot')throw Error();
    expect(value.payload).toEqual(wireView());expect(value.payload.seats[1].hole_cards).toEqual([5,19]);
    raw.payload.seats[1].hole_cards[0]=51;expect(value.payload.seats[1].hole_cards).toEqual([5,19]);
    const p=frame('table.snapshot',wireView(publicView));p.table_id=publicView.table_id;p.table_version=Number(publicView.table_version);p.hand_id=publicView.hand.hand_id;p.hand_version=Number(publicView.hand.hand_version);p.server_time=publicView.server_now;
    expect(parsePokerServer(p,publicView.table_id,'SPECTATOR').type).toBe('table.snapshot');
  });
  it.each(['type','event_id','event_seq','table_id','table_version','hand_id','hand_version','server_time','payload'])('requires server envelope slot %s',key=>{const raw=frame();delete raw[key];expect(()=>read(raw)).toThrow();});
  it.each(['9',null,-1,-0,1.5,9007199254740992,NaN,Infinity])('rejects non-safe wire counter %s',n=>{const raw=frame();raw.event_seq=n;expect(()=>read(raw)).toThrow();});
  it('accepts exact safe maximum and rejects metadata drift or private extensions',()=>{
    const raw=frame();raw.event_seq=Number.MAX_SAFE_INTEGER;expect(read(raw).event_seq).toBe('9007199254740991');
    for(const mutate of [(v:any)=>v.table_version++, (v:any)=>v.hand_version++, (v:any)=>v.hand_id=null, (v:any)=>v.server_time='2026-02-30T12:00:00Z', (v:any)=>v.server_time='2026-09-06T12:00:00Z', (v:any)=>v.user_id='910002', (v:any)=>v.payload.raw_deck=[0], (v:any)=>v.payload.seats[0].hole_cards=[]]){const bad=frame();mutate(bad);expect(()=>read(bad)).toThrow();}
  });
  it('decodes only current server families and keeps a receipt separate from state',()=>{
    for(const type of ['auth.accepted','pong']){const p={request_id:request,...(type==='auth.accepted'?{connection_id:'a'.repeat(32)}:{})};expect(read(frame(type,p)).payload).toEqual(p);}
    const r=receipt(),raw=frame('service.notice',{request_id:request,action_id:action,receipt:r}),value=read(raw);
    expect(value.type).toBe('service.notice');expect(value.payload).toEqual({request_id:request,action_id:action,receipt:r});
    if(value.type!=='service.notice')throw Error();r.amount_units='1000000';expect(value.payload.receipt.amount_units).toBe('500000');
    expect(read(frame('error',{request_id:request,code:'POKER_COMMAND_DENIED'})).type).toBe('error');
    for(const type of ['hand.delta','hand.action_ack','table.patch','control.transferred'])expect(()=>read(frame(type,{}))).toThrow();
    for(const mutation of [(r:any)=>r.raw_seed='secret',(r:any)=>r.amount_units=500000,(r:any)=>r.table_version=8,(r:any)=>r.table_id='00000000-0000-0000-0000-000000000000']){const r=receipt();mutation(r);expect(()=>read(frame('service.notice',{request_id:request,action_id:action,receipt:r}))).toThrow();}
  });
  it('rejects malformed/oversized/non-text frames without putting private contents in errors',()=>{
    expect(parsePokerServerText(JSON.stringify(frame()),self.table_id,'PLAYER_SELF').type).toBe('table.snapshot');
    for(const text of ['secret-ticket',JSON.stringify({...frame(),event_seq:'secret-ticket'}),' '.repeat(65537)]){try{parsePokerServerText(text,self.table_id,'PLAYER_SELF');throw Error('unexpected success');}catch(e){expect(String(e)).not.toContain('secret-ticket');expect(String(e)).not.toContain('unexpected success');}}
    expect(()=>parsePokerServerText(new ArrayBuffer(8) as unknown as string,self.table_id,'PLAYER_SELF')).toThrow();
  });
  it('rejects rounded fractional/exponent number lexemes before accepting otherwise safe parsed integers',()=>{
    const raw=JSON.stringify(frame());
    for(const number of ['1.0000000000000001','9007199254740991.1','1e0','-0'])expect(()=>parsePokerServerText(raw.replace('"event_seq":9',`"event_seq":${number}`),self.table_id,'PLAYER_SELF')).toThrow();
    const quoted=frame();quoted.payload.name='Quoted \\" 1.0000000000000001, 1e100';expect(parsePokerServerText(JSON.stringify(quoted),self.table_id,'PLAYER_SELF').type).toBe('table.snapshot');
  });
  it.each(['top-level','nested-escaped'])('rejects duplicate %s JSON slots in the raw frame before JSON.parse loses them',kind=>{
    const raw=JSON.stringify(frame());const duplicate=kind==='top-level'?raw.slice(0,-1)+',"payload":'+JSON.stringify(wireView())+'}':raw.replace('"can_act":true','"can_act":true,"can_\\u0061ct":true');
    expect(()=>parsePokerServerText(duplicate,self.table_id,'PLAYER_SELF')).toThrow();
  });
  it('sends the exact first auth frame, with no credentials in path/subprotocol or identity slots',()=>{
    const value=JSON.parse(encodePokerClient({type:'auth.connect',poker_connect_ticket:'ct1.fixture.signature'},refs));
    expect(value).toEqual({type:'auth.connect',request_id:request,table_id:self.table_id,hand_id:null,expected_table_version:0,expected_hand_version:0,control_epoch:0,action_id:null,payload:{poker_connect_ticket:'ct1.fixture.signature'}});
    for(const type of ['sync.request','ping'] as const){const v=JSON.parse(encodePokerClient({type},refs));expect(v.payload).toEqual({});expect(v.action_id).toBeNull();expect(v.hand_id).toBeNull();expect(Object.keys(v)).toHaveLength(9);}
  });
  it.each(['FOLD','CHECK','CALL','ALL_IN'] as const)('maps UI %s target zero to wire null, never the amount string zero',action_type=>{
    const value=JSON.parse(encodePokerClient({type:'hand.action',action_type,target_to_units:'0'},refs));
    expect(value.payload).toEqual({action_type,requested_to_units:null});expect(value.action_id).toBe(action);expect(value.expected_hand_version).toBe(8);expect(value.control_epoch).toBe(1);
    expect(()=>encodePokerClient({type:'hand.action',action_type,target_to_units:'500000'},refs)).toThrow();
  });
  it('preserves large target-to strings exactly and never falls back to blind version zero',()=>{
    const command={type:'hand.action' as const,action_type:'RAISE' as const,target_to_units:'9007199255000000'};
    expect(JSON.parse(encodePokerClient(command,refs)).payload.requested_to_units).toBe(command.target_to_units);
    for(const target_to_units of ['0','0500000','1.5','500001','9223372036855000000'])expect(()=>encodePokerClient({...command,target_to_units},refs)).toThrow();
    for(const key of ['table_version','hand_version','control_epoch','hand_id','action_id']){const bad:any={...refs};delete bad[key];expect(()=>encodePokerClient(command,bad)).toThrow();}
    expect(()=>encodePokerClient(command,{...refs,hand_version:'9007199254740992'})).toThrow();
  });
  it('uses known session/seed/chat messages only, bounded in UTF-8 with stable IDs',()=>{
    for(const type of ['session.sit_out_next_hand','session.resume_play'] as const){const v=JSON.parse(encodePokerClient({type},refs));expect(v.payload).toEqual({});expect(v.action_id).toBe(action);}
    expect(JSON.parse(encodePokerClient({type:'client_seed.set_next',client_seed:'御主'},refs)).payload).toEqual({client_seed:'御主'});
    expect(JSON.parse(encodePokerClient({type:'chat.send',message:'你好'},refs)).action_id).toBeNull();
    expect(()=>encodePokerClient({type:'client_seed.set_next',client_seed:'御'.repeat(86)},refs)).toThrow();
    expect(()=>encodePokerClient({type:'chat.send',message:'御'.repeat(683)},refs)).toThrow();
    expect(()=>encodePokerClient({type:'chat.send',message:'  '},refs)).toThrow();
    expect(()=>encodePokerClient({type:'START_HAND'} as never,refs)).toThrow();
    expect(()=>encodePokerClient({type:'ping'},{...refs,request_id:'short'})).toThrow();
  });
});
