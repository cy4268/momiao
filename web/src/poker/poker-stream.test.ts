import { describe, expect, it } from 'vitest';
import { openPokerStream, receivePokerStream, disconnectPokerStream, beginPokerPending, pokerStreamContext } from './poker-stream';
import type { PokerReadScope } from './poker-client-boundary';
import self from './fixtures/g3-player-self.json';

const auth='auth-request-00001',request='action-request-01',action='stable-action-001';
const scope:PokerReadScope={user_id:'910002',session_generation:4,request_generation:6,table_id:self.table_id,viewer_kind:'PLAYER_SELF'};
function message(type:string,payload:any,seq=10):string{return JSON.stringify({type,event_id:`opaque-${seq}`,event_seq:seq,table_id:self.table_id,table_version:7,hand_id:self.hand.hand_id,hand_version:8,server_time:self.server_now,payload:type==='auth.accepted'?{...payload,connection_id:'a'.repeat(32)}:payload});}
const snapshot=(seq=10)=>{const v:any=structuredClone(self);v.viewer.control={connection_id:'a'.repeat(32),session_id:self.viewer.session_id,mode:'CONTROLLER',control_epoch:'1'};return message('table.snapshot',v,seq);};
function live(){let s=openPokerStream(scope,auth,'CLAIM_CONTROL');s=receivePokerStream(s,scope,message('auth.accepted',{request_id:auth},1));return receivePokerStream(s,scope,snapshot());}
function ack(version='8',status='PENDING',requestID=request,actionID:string|null=action){return message('service.notice',{request_id:requestID,action_id:actionID,receipt:{table_id:self.table_id,session_id:self.viewer.session_id,status,table_version:version,duplicate:false}},11);}
function nextSnapshot(version='8',seq=12){const raw=JSON.parse(snapshot(seq));raw.table_version=Number(version);raw.payload.table_version=version;return JSON.stringify(raw);}

describe('Poker full-snapshot stream scope and receipts',()=>{
  it('requires correlated authentication before the first full snapshot',()=>{
    const initial=openPokerStream(scope,auth,'CLAIM_CONTROL');expect(initial.connection_state).toBe('AUTH_PENDING');expect(initial.table).toBeUndefined();
    expect(receivePokerStream(initial,scope,snapshot()).connection_state).toBe('DEGRADED');
    expect(receivePokerStream(initial,scope,message('auth.accepted',{request_id:'foreign-request-1'})).connection_state).toBe('DEGRADED');
    const accepted=receivePokerStream(initial,scope,message('auth.accepted',{request_id:auth}));expect(accepted.connection_state).toBe('SYNCING');expect(accepted.table).toBeUndefined();
    expect(receivePokerStream(accepted,scope,snapshot()).connection_state).toBe('LIVE');
  });
  it('drops stale account/session/table/socket/viewer callbacks before decoding private contents',()=>{
    const s=live();for(const old of [{...scope,user_id:'1'},{...scope,session_generation:3},{...scope,request_generation:5},{...scope,table_id:'other'},{...scope,viewer_kind:'SPECTATOR' as const}])expect(receivePokerStream(s,old,'not-json-secret')).toBe(s);
    const replacement=openPokerStream({...scope,user_id:'1',request_generation:7},auth,'CLAIM_CONTROL',s);expect(replacement.table).toBeUndefined();expect(replacement.pending).toBeUndefined();
  });
  it('takes large sequence gaps and restarted sequence baselines without a replay loop',()=>{
    const before=live(),context=pokerStreamContext(before);const gap=receivePokerStream(before,scope,snapshot(400));
    expect(gap.connection_state).toBe('LIVE');expect(gap.event_sequence).toBe('400');expect(pokerStreamContext(gap).runtime_id).not.toBe(context.runtime_id);
    const restarted=receivePokerStream(gap,scope,snapshot(1));expect(restarted.connection_state).toBe('LIVE');expect(restarted.event_sequence).toBe('1');expect(restarted.snapshot_generation).toBe(3);
    expect(pokerStreamContext(restarted).runtime_id).not.toBe(pokerStreamContext(gap).runtime_id);
    expect(receivePokerStream(restarted,scope,nextSnapshot('6',2))).toBe(restarted);
  });
  it('makes a pending latch before I/O and blocks stale/double submission',()=>{
    const s=live(),c=pokerStreamContext(s),queued=beginPokerPending(s,c,request,action,'action');
    expect(queued.pending?.phase).toBe('SENT');expect(queued.table).toBe(s.table);
    expect(()=>beginPokerPending(queued,c,request,action,'action')).toThrow();
    expect(()=>beginPokerPending(receivePokerStream(s,scope,snapshot(1)),c,request,action,'action')).toThrow();
    expect(()=>beginPokerPending(disconnectPokerStream(s),c,request,action,'action')).toThrow();
  });
  it('does not treat an ACK as state or clear pending before its committed version arrives',()=>{
    const s=live(),queued=beginPokerPending(s,pokerStreamContext(s),request,action,'action'),received=receivePokerStream(queued,scope,ack());
    expect(received.table).toBe(queued.table);expect(received.table?.table_version).toBe('7');expect(received.pending?.phase).toBe('ACKNOWLEDGED');
    expect(received.event_sequence).toBe('10');expect(received.snapshot_generation).toBe(1);
    const synced=receivePokerStream(received,scope,nextSnapshot());expect(synced.table?.table_version).toBe('8');expect(synced.pending).toBeUndefined();
  });
  it('handles snapshot-before-ACK and leaves domain PENDING top-up semantics untouched',()=>{
    const s=live(),queued=beginPokerPending(s,pokerStreamContext(s),request,action,'action'),synced=receivePokerStream(queued,scope,nextSnapshot());
    expect(synced.pending?.phase).toBe('SENT');const acked=receivePokerStream(synced,scope,ack());expect(acked.pending).toBeUndefined();expect(acked.last_receipt?.status).toBe('PENDING');expect(acked.table).toBe(synced.table);
  });
  it('ignores unrelated IDs, rejects action correlation mismatch, and distinguishes an error from a no-effect receipt',()=>{
    const s=live(),queued=beginPokerPending(s,pokerStreamContext(s),request,action,'action');
    expect(receivePokerStream(queued,scope,ack('8','PENDING','foreign-request-1'))).toBe(queued);
    expect(receivePokerStream(queued,scope,ack('8','PENDING',request,'different-action-01')).connection_state).toBe('DEGRADED');
    const failed=receivePokerStream(queued,scope,message('error',{request_id:request,code:'POKER_COMMAND_DENIED'}));expect(failed.pending?.phase).toBe('UNKNOWN');expect(failed.last_error).toBe('POKER_COMMAND_DENIED');expect(failed.table).toBe(queued.table);
    const noEffect=receivePokerStream(queued,scope,ack('8','FAILED_NO_EFFECT'));expect(noEffect.pending).toBeUndefined();expect(noEffect.table).toBe(queued.table);
  });
  it('retains unknown outcomes across same-session reconnect; a fresh snapshot alone does not authorize retry',()=>{
    const s=live(),queued=beginPokerPending(s,pokerStreamContext(s),request,action,'action'),lost=disconnectPokerStream(queued);
    expect(lost.connection_state).toBe('DISCONNECTED');expect(lost.pending?.phase).toBe('UNKNOWN');
    const nextScope={...scope,request_generation:7};let reopened=openPokerStream(nextScope,auth,'CLAIM_CONTROL',lost);expect(reopened.table).toBeUndefined();expect(reopened.pending?.action_id).toBe(action);
    reopened=receivePokerStream(reopened,nextScope,message('auth.accepted',{request_id:auth}));reopened=receivePokerStream(reopened,nextScope,nextSnapshot('9'));
    expect(reopened.pending?.phase).toBe('UNKNOWN');expect(()=>beginPokerPending(reopened,pokerStreamContext(reopened),request,action,'action')).toThrow();
    const changed=openPokerStream({...nextScope,session_generation:5},auth,'CLAIM_CONTROL',lost);expect(changed.pending).toBeUndefined();expect(changed.last_receipt).toBeUndefined();
  });
  it('retains only durable version fences across reconnect, not stale private projections',()=>{
    const s=live(),nextScope={...scope,request_generation:7};let reopened=openPokerStream(nextScope,auth,'CLAIM_CONTROL',disconnectPokerStream(s));
    expect(JSON.stringify(reopened)).not.toContain('hole_cards');expect(reopened.table).toBeUndefined();
    reopened=receivePokerStream(reopened,nextScope,message('auth.accepted',{request_id:auth}));
    expect(receivePokerStream(reopened,nextScope,nextSnapshot('6'))).toBe(reopened);
    const staleHand=JSON.parse(snapshot());staleHand.hand_version=7;staleHand.payload.hand.hand_version='7';expect(receivePokerStream(reopened,nextScope,JSON.stringify(staleHand))).toBe(reopened);
    const restored=receivePokerStream(reopened,nextScope,snapshot(1));expect(restored.connection_state).toBe('LIVE');expect(restored.table?.table_version).toBe('7');
  });
  it('goes read-only on malformed current data, duplicate auth or post-close frames without logging raw data',()=>{
    const s=live(),queued=beginPokerPending(s,pokerStreamContext(s),request,action,'action'),bad=receivePokerStream(queued,scope,'private-secret');
    expect(bad.connection_state).toBe('DEGRADED');expect(bad.pending?.phase).toBe('UNKNOWN');expect(bad.last_error).toBe('POKER_PROTOCOL_INVALID');
    expect(receivePokerStream(s,scope,message('auth.accepted',{request_id:auth})).connection_state).toBe('DEGRADED');
    const closed=disconnectPokerStream(s);expect(receivePokerStream(closed,scope,snapshot())).toBe(closed);
  });
});
