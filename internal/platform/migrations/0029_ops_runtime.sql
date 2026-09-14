-- The existing 0019 rows remain the sole maintenance authority.
ALTER TABLE ops.maintenance_windows ADD COLUMN announcement_id uuid REFERENCES content.announcements ON DELETE RESTRICT;
CREATE TABLE ops.critical_notice_state (
 critical_notice_id uuid PRIMARY KEY,
 maintenance_id uuid NOT NULL UNIQUE REFERENCES ops.maintenance_windows ON DELETE RESTRICT,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TRIGGER critical_notice_state_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON ops.critical_notice_state
 FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
REVOKE ALL ON ops.critical_notice_state FROM PUBLIC;
CREATE TABLE ops.health_snapshots (
 environment text PRIMARY KEY CHECK(environment IN('DEVELOPMENT','STAGING','PRODUCTION')),
 snapshot jsonb NOT NULL CHECK(jsonb_typeof(snapshot)='object'),
 observed_at timestamptz NOT NULL,
 stale_after timestamptz NOT NULL CHECK(stale_after>observed_at)
);
REVOKE ALL ON ops.health_snapshots FROM PUBLIC;
CREATE TRIGGER maintenance_window_scopes_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON ops.maintenance_window_scopes
 FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE FUNCTION ops.lock_write_scopes(p_scopes text[]) RETURNS void
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE found_count integer:=0; item text;
BEGIN
 IF current_setting('transaction_read_only')<>'off' OR current_setting('transaction_isolation')<>'read committed' THEN
  RAISE EXCEPTION 'MAINTENANCE_WRITE_TRANSACTION_REQUIRED' USING ERRCODE='25000';
 END IF;
 IF p_scopes IS NULL OR cardinality(p_scopes)=0 OR cardinality(p_scopes)>7
  OR EXISTS(SELECT 1 FROM unnest(p_scopes) s WHERE s IS NULL)
  OR cardinality(p_scopes)<>(SELECT count(DISTINCT s) FROM unnest(p_scopes) s) THEN
  RAISE EXCEPTION 'MAINTENANCE_SCOPE_INVALID' USING ERRCODE='22023';
 END IF;
 FOR item IN SELECT scope FROM ops.maintenance_scope_guards WHERE scope=ANY(p_scopes) ORDER BY scope FOR SHARE LOOP
  found_count:=found_count+1;
 END LOOP;
 IF found_count<>cardinality(p_scopes) THEN RAISE EXCEPTION 'MAINTENANCE_SCOPE_INVALID' USING ERRCODE='22023'; END IF;
END $$;
REVOKE ALL ON FUNCTION ops.lock_write_scopes(text[]) FROM PUBLIC;

CREATE TABLE ops.maintenance_events (
 event_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 maintenance_id uuid NOT NULL REFERENCES ops.maintenance_windows ON DELETE RESTRICT,
 operation_id uuid NOT NULL REFERENCES ops.admin_operations ON DELETE RESTRICT,
 actor_user_id bigint NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
 state text NOT NULL CHECK(state IN('SCHEDULED','ACTIVE','COMPLETED','CANCELLED','ACTIVATION_FAILED','ENDING_FAILED')),
 source text NOT NULL CHECK(source IN('OPERATOR','SCHEDULE')),
 occurred_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TRIGGER maintenance_events_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON ops.maintenance_events
 FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
REVOKE ALL ON ops.maintenance_events FROM PUBLIC;

-- Execution schedules are created atomically with their window. Window state
-- and scope guards remain the authority for admission and lifecycle changes.
CREATE TABLE ops.maintenance_jobs (
 job_key text PRIMARY KEY,
 maintenance_id uuid NOT NULL REFERENCES ops.maintenance_windows ON DELETE RESTRICT,
 kind text NOT NULL CHECK(kind IN('MAINTENANCE_ACTIVATE','MAINTENANCE_END')),
 due_at timestamptz NOT NULL,
 status text NOT NULL DEFAULT 'PENDING' CHECK(status IN('PENDING','COMPLETED','CANCELLED','FAILED')),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(maintenance_id,kind),
 CHECK(job_key=CASE kind WHEN 'MAINTENANCE_ACTIVATE' THEN 'maintenance:activate:' ELSE 'maintenance:end:' END || maintenance_id::text)
);
CREATE INDEX maintenance_jobs_due ON ops.maintenance_jobs(due_at,job_key) WHERE status='PENDING';
INSERT INTO ops.maintenance_jobs(job_key,maintenance_id,kind,due_at)
 SELECT 'maintenance:activate:'||maintenance_id::text,maintenance_id,'MAINTENANCE_ACTIVATE',scheduled_start_at
 FROM ops.maintenance_windows WHERE state='SCHEDULED' AND scheduled_start_at IS NOT NULL
 UNION ALL
 SELECT 'maintenance:end:'||maintenance_id::text,maintenance_id,'MAINTENANCE_END',scheduled_end_at
 FROM ops.maintenance_windows WHERE state IN('SCHEDULED','ACTIVE') AND scheduled_end_at IS NOT NULL;
REVOKE ALL ON ops.maintenance_jobs FROM PUBLIC;

-- Aggregate worker facts only. The portal receives no payloads, credentials,
-- raw error messages, player state or private source records through this port.
CREATE FUNCTION ops.runtime_jobs_read()
RETURNS TABLE(kind text,pending_count bigint,attention_count bigint,due_count bigint,oldest_due_at timestamptz)
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
$$;
REVOKE ALL ON FUNCTION ops.runtime_jobs_read() FROM PUBLIC;
