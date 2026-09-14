-- Durable cross-process Operations dispatch. Platform PostgreSQL remains the
-- authorization and lifecycle authority; remote commands run only after the
-- operation and this outbox row commit together.
CREATE TABLE ops.remote_dispatches (
 operation_id UUID PRIMARY KEY REFERENCES ops.admin_operations ON DELETE RESTRICT,
 operation_type TEXT NOT NULL CHECK(operation_type ~ '^[A-Z][A-Z0-9_]{0,127}$'),
 command_payload JSONB NOT NULL CHECK(jsonb_typeof(command_payload)='object' AND octet_length(command_payload::text)<=32768),
 command_hash BYTEA NOT NULL CHECK(octet_length(command_hash)=32),
 state TEXT NOT NULL DEFAULT 'PENDING' CHECK(state IN (
  'PENDING','DISPATCHING','RETRY','SUCCEEDED','FAILED_NO_EFFECT','NEEDS_REVIEW')),
 attempt_count BIGINT NOT NULL DEFAULT 0 CHECK(attempt_count>=0),
 next_attempt_at TIMESTAMPTZ,
 claim_token UUID,
 lease_expires_at TIMESTAMPTZ,
 failure_code TEXT CHECK(failure_code IS NULL OR (
  octet_length(failure_code) BETWEEN 1 AND 128 AND failure_code ~ '^[A-Z][A-Z0-9_]*$')),
 receipt_payload JSONB CHECK(receipt_payload IS NULL OR (
  jsonb_typeof(receipt_payload)='object' AND octet_length(receipt_payload::text)<=32768)),
 receipt_hash BYTEA CHECK(receipt_hash IS NULL OR octet_length(receipt_hash)=32),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 settled_at TIMESTAMPTZ,
 CHECK(
  (state='PENDING' AND attempt_count=0 AND next_attempt_at IS NOT NULL AND
   claim_token IS NULL AND lease_expires_at IS NULL AND failure_code IS NULL AND
   receipt_payload IS NULL AND receipt_hash IS NULL AND settled_at IS NULL) OR
  (state='DISPATCHING' AND attempt_count>0 AND next_attempt_at IS NULL AND
   claim_token IS NOT NULL AND lease_expires_at IS NOT NULL AND
   receipt_payload IS NULL AND receipt_hash IS NULL AND settled_at IS NULL) OR
  (state='RETRY' AND attempt_count>0 AND next_attempt_at IS NOT NULL AND
   claim_token IS NULL AND lease_expires_at IS NULL AND failure_code IS NOT NULL AND
   receipt_payload IS NULL AND receipt_hash IS NULL AND settled_at IS NULL) OR
  (state='SUCCEEDED' AND attempt_count>0 AND next_attempt_at IS NULL AND
   claim_token IS NULL AND lease_expires_at IS NULL AND failure_code IS NULL AND
   receipt_payload IS NOT NULL AND receipt_hash IS NOT NULL AND settled_at IS NOT NULL) OR
  (state='FAILED_NO_EFFECT' AND attempt_count>0 AND next_attempt_at IS NULL AND
   claim_token IS NULL AND lease_expires_at IS NULL AND failure_code IS NOT NULL AND
   receipt_payload IS NULL AND receipt_hash IS NULL AND settled_at IS NOT NULL) OR
  (state='NEEDS_REVIEW' AND attempt_count>0 AND next_attempt_at IS NULL AND
   claim_token IS NULL AND lease_expires_at IS NULL AND failure_code IS NOT NULL AND
   (receipt_payload IS NULL)=(receipt_hash IS NULL) AND settled_at IS NOT NULL)
 )
);
CREATE INDEX remote_dispatches_due ON ops.remote_dispatches(
 state,next_attempt_at,lease_expires_at,operation_id)
 WHERE state IN ('PENDING','DISPATCHING','RETRY');

CREATE FUNCTION ops.guard_remote_dispatch_update() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF (NEW.operation_id,NEW.operation_type,NEW.command_payload,NEW.command_hash,NEW.created_at) IS DISTINCT FROM
    (OLD.operation_id,OLD.operation_type,OLD.command_payload,OLD.command_hash,OLD.created_at) OR
    NEW.updated_at<OLD.updated_at THEN
  RAISE EXCEPTION 'immutable remote dispatch core' USING ERRCODE='55000';
 END IF;
 IF NEW.state='DISPATCHING' AND (
    OLD.state IN ('PENDING','RETRY') OR
    OLD.state='DISPATCHING' AND OLD.lease_expires_at<=statement_timestamp()) THEN
  IF NEW.attempt_count<>OLD.attempt_count+1 OR NEW.claim_token IS NULL OR
     NEW.claim_token IS NOT DISTINCT FROM OLD.claim_token OR
     NEW.lease_expires_at<=statement_timestamp() THEN
   RAISE EXCEPTION 'invalid remote dispatch claim' USING ERRCODE='55000';
  END IF;
 ELSIF OLD.state='DISPATCHING' AND NEW.state IN (
    'RETRY','SUCCEEDED','FAILED_NO_EFFECT','NEEDS_REVIEW') THEN
  IF NEW.attempt_count<>OLD.attempt_count THEN
   RAISE EXCEPTION 'invalid remote dispatch completion' USING ERRCODE='55000';
  END IF;
 ELSE
  RAISE EXCEPTION 'invalid remote dispatch transition' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER remote_dispatch_state_guard BEFORE UPDATE ON ops.remote_dispatches
 FOR EACH ROW EXECUTE FUNCTION ops.guard_remote_dispatch_update();
CREATE TRIGGER remote_dispatch_remove_guard BEFORE DELETE OR TRUNCATE ON ops.remote_dispatches
 FOR EACH STATEMENT EXECUTE FUNCTION content.reject_history_change();

-- Aggregate only scheduling and attention facts. Commands, receipts, failure
-- details and claim tokens never cross this read boundary.
CREATE OR REPLACE FUNCTION ops.runtime_jobs_read()
RETURNS TABLE(kind TEXT,pending_count BIGINT,attention_count BIGINT,due_count BIGINT,oldest_due_at TIMESTAMPTZ)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog AS $$
 SELECT 'ANNOUNCEMENTS'::text,count(*),0::bigint,
  count(*) FILTER(WHERE due_at<=statement_timestamp()),min(due_at)
 FROM content.announcement_jobs WHERE status='PENDING'
 UNION ALL
 SELECT 'GAME_ROUNDS',count(*) FILTER(WHERE status='PENDING'),count(*) FILTER(WHERE status='NEEDS_REVIEW'),
  count(*) FILTER(WHERE status='PENDING' AND run_at<=statement_timestamp()),min(run_at) FILTER(WHERE status='PENDING')
 FROM games.round_jobs WHERE status IN('PENDING','NEEDS_REVIEW')
 UNION ALL
 SELECT 'REGISTRATION_GRANTS',count(*),count(*) FILTER(WHERE status='RECOVERING'),
  count(*) FILTER(WHERE next_attempt_at<=statement_timestamp()),min(next_attempt_at)
 FROM platform_meta.registration_grant_jobs WHERE status<>'DONE'
 UNION ALL
 SELECT 'KEY_PURPOSE_SYNC',count(*) FILTER(WHERE status='PENDING'),count(*) FILTER(WHERE status='REJECTED'),
  count(*) FILTER(WHERE status='PENDING' AND next_attempt_at<=statement_timestamp()),min(next_attempt_at) FILTER(WHERE status='PENDING')
 FROM platform_meta.api_key_purpose_outbox WHERE status IN('PENDING','REJECTED')
 UNION ALL
 SELECT 'QUOTA_TRANSFERS',count(*) FILTER(WHERE status='PENDING'),count(*) FILTER(WHERE status='NEEDS_REVIEW'),
  count(*) FILTER(WHERE status='PENDING'),min(created_at) FILTER(WHERE status='PENDING')
 FROM economy.quota_transfers WHERE status IN('PENDING','NEEDS_REVIEW')
 UNION ALL
 SELECT 'MAINTENANCE',count(*) FILTER(WHERE status='PENDING'),count(*) FILTER(WHERE status='FAILED'),
  count(*) FILTER(WHERE status='PENDING' AND due_at<=statement_timestamp()),min(due_at) FILTER(WHERE status='PENDING')
 FROM ops.maintenance_jobs WHERE status IN('PENDING','FAILED')
 UNION ALL
 SELECT 'RANKING_REBUILDS',count(*) FILTER(WHERE status IN('PENDING','RUNNING')),
  count(*) FILTER(WHERE status='FAILED'),
  count(*) FILTER(WHERE status='PENDING' AND (
   failure_code IS NULL OR updated_at<=statement_timestamp()-interval '5 seconds') OR
   status='RUNNING' AND updated_at<=statement_timestamp()-interval '2 minutes'),
  min(CASE
   WHEN status='PENDING' AND failure_code IS NULL THEN created_at
   WHEN status='PENDING' THEN updated_at+interval '5 seconds'
   WHEN status='RUNNING' THEN updated_at+interval '2 minutes'
  END)
 FROM rankings.rebuild_jobs WHERE status IN('PENDING','RUNNING','FAILED')
 UNION ALL
 SELECT 'OPS_REMOTE',count(*) FILTER(WHERE state IN('PENDING','DISPATCHING','RETRY')),
  count(*) FILTER(WHERE state='NEEDS_REVIEW'),
  count(*) FILTER(WHERE state IN('PENDING','RETRY') AND next_attempt_at<=statement_timestamp() OR
   state='DISPATCHING' AND lease_expires_at<=statement_timestamp()),
  min(CASE WHEN state='DISPATCHING' THEN lease_expires_at ELSE next_attempt_at END)
 FROM ops.remote_dispatches WHERE state IN('PENDING','DISPATCHING','RETRY','NEEDS_REVIEW')
$$;

REVOKE ALL ON ops.remote_dispatches FROM PUBLIC;
REVOKE ALL ON FUNCTION ops.guard_remote_dispatch_update(),ops.runtime_jobs_read() FROM PUBLIC;
