import { describe, expect, it } from 'vitest';
import { parsePokerControl } from './poker-view';
import { parsePokerServerText } from './poker-wire';
import { openPokerStream, receivePokerStream, pokerStreamAuthority, pokerStreamContext } from './poker-stream';
import type { PokerReadScope } from './poker-client-boundary';
import self from './fixtures/g3-player-self.json';
import acting from './fixtures/g3-socket-acting.json';
import controller from './fixtures/g3-socket-controller.json';
import readonly from './fixtures/g3-socket-readonly.json';

const connection='a'.repeat(32),oldConnection='b'.repeat(32),request='auth-request-00001',session=self.viewer.session_id;
const scope:PokerReadScope={user_id:'910002',session_generation:4,request_generation:6,table_id:self.table_id,viewer_kind:'PLAYER_SELF'};
const control=(mode:'CONTROLLER'|'READ_ONLY'='CONTROLLER',epoch='1',id=connection)=>({connection_id:id,session_id:session,mode,control_epoch:epoch});
function frame(type:string,payload:unknown,seq=10){return JSON.stringify({type,event_id:`opaque-${seq}`,event_seq:seq,table_id:self.table_id,table_version:7,hand_id:self.hand.hand_id,hand_version:8,server_time:self.server_now,payload});}
function snapshot(mode:'CONTROLLER'|'READ_ONLY'='CONTROLLER',epoch='1',id=connection){const v:any=structuredClone(self);v.viewer.control_epoch=epoch;v.viewer.control=control(mode,epoch,id);if(mode==='READ_ONLY'){v.viewer.can_act=false;delete v.viewer.legal;}return frame('table.snapshot',v);}
function authenticated(intent:'CLAIM_CONTROL'|'READ_ONLY'='CLAIM_CONTROL'){return receivePokerStream(openPokerStream(scope,request,intent),scope,frame('auth.accepted',{request_id:request,connection_id:connection},1));}
function live(mode:'CONTROLLER'|'READ_ONLY'='CONTROLLER'){return receivePokerStream(authenticated(),scope,snapshot(mode));}

describe('approved connection-specific Poker control',()=>{
  it('decodes unmodified real G3 0017 PG/TLS-Redis socket projections without reconstructing control or private fields',()=>{
    for(const v of [acting,controller,readonly]){
      const h='hand' in v?v.hand:undefined,raw={type:'table.snapshot',event_id:'synthetic-envelope-real-g3-payload',event_seq:1,table_id:v.table_id,table_version:Number(v.table_version),hand_id:h?.hand_id??null,hand_version:h?Number(h.hand_version):null,server_time:v.server_now,payload:v};
      const decoded=parsePokerServerText(JSON.stringify(raw),v.table_id,'PLAYER_SELF');expect(decoded.type).toBe('table.snapshot');expect(decoded.payload).toEqual(v);
    }
    expect(acting.viewer.control.mode).toBe('CONTROLLER');expect(acting.viewer.can_act).toBe(true);expect(readonly.viewer.control.mode).toBe('READ_ONLY');expect(readonly.viewer.can_act).toBe(false);expect(readonly.viewer.can_top_up).toBe(true);expect(readonly.viewer.can_leave).toBe(true);expect(readonly.viewer).not.toHaveProperty('legal');
  });
  it('rejects a contradictory READ_ONLY snapshot carrying actionable capability or legal actions',()=>{
    const v:any=structuredClone(self);v.viewer.control=control('READ_ONLY');expect(()=>parsePokerServerText(frame('table.snapshot',v),self.table_id,'PLAYER_SELF')).toThrow();
  });
  it('never upgrades a READ_ONLY ticket and requires a fresh explicit CLAIM_CONTROL connection before separate takeover',()=>{
    const auth=authenticated('READ_ONLY'),s=receivePokerStream(auth,scope,snapshot('READ_ONLY'));
    expect(pokerStreamAuthority(s)).toMatchObject({can_control:false,can_takeover:false,ticket_intent:'READ_ONLY'});
    expect(receivePokerStream(s,scope,snapshot('CONTROLLER')).connection_state).toBe('DEGRADED');
    expect(receivePokerStream(s,scope,frame('control.changed',control())).connection_state).toBe('DEGRADED');
    const next={...scope,request_generation:7};let fresh=openPokerStream(next,request,'CLAIM_CONTROL',s);
    fresh=receivePokerStream(fresh,next,frame('auth.accepted',{request_id:request,connection_id:connection}));
    expect(pokerStreamAuthority(fresh).can_takeover).toBe(false);
    fresh=receivePokerStream(fresh,next,snapshot('READ_ONLY'));expect(pokerStreamAuthority(fresh)).toMatchObject({can_control:false,can_takeover:true});
  });
  it('accepts a durable no-session full snapshot after safe leave and never retains the former controller',()=>{
    const s=live(),v:any=structuredClone(self);delete v.viewer.session_id;delete v.viewer.seat_no;delete v.viewer.legal;for(const flag of ['can_act','can_top_up','can_leave','can_resume','can_sit_out'])v.viewer[flag]=false;v.viewer.control_epoch='0';v.viewer.control={connection_id:connection,mode:'READ_ONLY',control_epoch:'0'};v.seats=v.seats.map((seat:any)=>{const next={...seat,is_self:false};delete next.hole_cards;return next;});
    const raw=JSON.parse(frame('table.snapshot',v));raw.table_version=8;raw.payload.table_version='8';
    const revoked=receivePokerStream(s,scope,frame('control.changed',v.viewer.control));expect(pokerStreamAuthority(revoked).can_control).toBe(false);expect(receivePokerStream(revoked,scope,snapshot())).toBe(revoked);
    const next=receivePokerStream(revoked,scope,JSON.stringify(raw));expect(next.connection_state).toBe('LIVE');expect(next.table?.viewer.session_id).toBeUndefined();expect(next.control?.session_id).toBeUndefined();expect(pokerStreamAuthority(next)).toMatchObject({can_control:false,can_takeover:false});
    expect(receivePokerStream(next,scope,snapshot())).toBe(next);
  });
  it('strictly decodes the exact shared DTO and never accepts a request ID as a controller identity',()=>{
    expect(parsePokerControl(control())).toEqual(control());expect(parsePokerControl({connection_id:connection,mode:'READ_ONLY',control_epoch:'0'}).mode).toBe('READ_ONLY');
    for(const patch of [{connection_id:request},{connection_id:'A'.repeat(32)},{connection_id:''},{control_epoch:1},{control_epoch:'01'},{control_epoch:'-1'},{mode:'CLAIM_CONTROL'},{mode:'CONTROLLER',session_id:undefined},{seed:'secret'}])expect(()=>parsePokerControl({...control(),...patch})).toThrow();
  });
  it('auth only records the current server socket ID; neither ACK nor can_act grants control',()=>{
    expect(()=>parsePokerServerText(frame('auth.accepted',{request_id:request}),self.table_id,'PLAYER_SELF')).toThrow();
    const s=authenticated();expect(s.connection_id).toBe(connection);expect(s.connection_state).toBe('SYNCING');expect(pokerStreamAuthority(s).can_control).toBe(false);
    const readonly=live('READ_ONLY');expect(readonly.table?.viewer.can_act).toBe(false);expect(pokerStreamAuthority(readonly).can_control).toBe(false);expect(pokerStreamAuthority(readonly).can_takeover).toBe(true);
    expect(pokerStreamAuthority(live()).can_control).toBe(true);
  });
  it('requires WS snapshot control and exact base viewer epoch/session correspondence',()=>{
    expect(()=>parsePokerServerText(frame('table.snapshot',self),self.table_id,'PLAYER_SELF')).toThrow();
    for(const patch of [{control_epoch:'2'},{session_id:'00000000-0000-0000-0000-000000000000'}]){const v:any=structuredClone(self);v.viewer.control={...control(),...patch};expect(()=>parsePokerServerText(frame('table.snapshot',v),self.table_id,'PLAYER_SELF')).toThrow();}
    const v:any=structuredClone(self);v.viewer.control={...control(),session_id:undefined};expect(()=>parsePokerServerText(frame('table.snapshot',v),self.table_id,'PLAYER_SELF')).toThrow();
  });
  it('ignores snapshots/control events for an old connection and never lets their payload replace current cards or grant',()=>{
    const s=live('READ_ONLY');expect(receivePokerStream(s,scope,snapshot('CONTROLLER','1',oldConnection))).toBe(s);
    expect(receivePokerStream(s,scope,frame('control.changed',control('CONTROLLER','1',oldConnection)))).toBe(s);
    expect(pokerStreamAuthority(s).can_control).toBe(false);
  });
  it('revokes immediately on READ_ONLY and invalidates old callbacks even without a new table version',()=>{
    const s=live(),old=pokerStreamContext(s),revoked=receivePokerStream(s,scope,frame('control.changed',control('READ_ONLY')));
    expect(revoked.table).toBe(s.table);expect(pokerStreamAuthority(revoked).can_control).toBe(false);expect(pokerStreamContext(revoked).runtime_id).not.toBe(old.runtime_id);expect(pokerStreamAuthority(revoked).can_takeover).toBe(true);
    expect(receivePokerStream(revoked,scope,snapshot())).toBe(revoked);
    const matched=receivePokerStream(revoked,scope,snapshot('READ_ONLY'));expect(pokerStreamAuthority(matched).can_control).toBe(false);expect(receivePokerStream(matched,scope,snapshot())).toBe(matched);
  });
  it('waits for a matching full snapshot after a newer control epoch and rejects regressing epoch messages',()=>{
    const s=live(),newControl=receivePokerStream(s,scope,frame('control.changed',control('CONTROLLER','2')));
    expect(pokerStreamAuthority(newControl).can_control).toBe(false);expect(newControl.table?.viewer.control_epoch).toBe('1');
    expect(receivePokerStream(newControl,scope,snapshot('CONTROLLER','1'))).toBe(newControl);
    expect(receivePokerStream(newControl,scope,frame('control.changed',control('CONTROLLER','1')))).toBe(newControl);
    const confirmed=receivePokerStream(newControl,scope,snapshot('CONTROLLER','2'));expect(pokerStreamAuthority(confirmed).can_control).toBe(true);
    expect(receivePokerStream(confirmed,{...scope,session_generation:3},frame('control.changed',control('READ_ONLY','3')))).toBe(confirmed);
  });
  it('discards a control event for another Poker session instead of silently transferring its grant',()=>{
    const s=live('READ_ONLY'),other={...control(),session_id:'00000000-0000-0000-0000-000000000000'};
    expect(receivePokerStream(s,scope,frame('control.changed',other))).toBe(s);expect(pokerStreamAuthority(s).can_control).toBe(false);
  });
});
