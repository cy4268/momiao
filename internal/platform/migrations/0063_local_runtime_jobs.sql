-- Reuse the existing queues without exposing command payloads or source errors.
CREATE FUNCTION ops.runtime_job_items()
RETURNS TABLE(kind TEXT,job_id TEXT,target_id TEXT,state TEXT,pending BOOLEAN,attention BOOLEAN,run_at TIMESTAMPTZ,updated_at TIMESTAMPTZ,legacy BOOLEAN)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog AS $$
 SELECT 'ANNOUNCEMENTS'::text,job_key,announcement_id::text,status,true,false,due_at,created_at,false
 FROM content.announcement_jobs WHERE status='PENDING'
 UNION ALL
 SELECT 'GAME_ROUNDS',job_id::text,round_id::text,status,status='PENDING',status='NEEDS_REVIEW',run_at,updated_at,false
 FROM games.round_jobs WHERE status IN('PENDING','NEEDS_REVIEW')
 UNION ALL
 SELECT 'REGISTRATION_GRANTS',claim_id::text,claim_id::text,status,true,status='RECOVERING',next_attempt_at,updated_at,false
 FROM platform_meta.registration_grant_jobs WHERE status<>'DONE'
 UNION ALL
 SELECT 'KEY_PURPOSE_SYNC',operation_id::text,token_id::text,status,status='PENDING',status='REJECTED',next_attempt_at,updated_at,true
 FROM platform_meta.api_key_purpose_outbox WHERE status IN('PENDING','REJECTED')
 UNION ALL
 SELECT 'QUOTA_TRANSFERS',transfer_id::text,transfer_id::text,status,status='PENDING',status='NEEDS_REVIEW',created_at,updated_at,true
 FROM economy.quota_transfers WHERE status IN('PENDING','NEEDS_REVIEW')
 UNION ALL
 SELECT 'MAINTENANCE',job_key,maintenance_id::text,status,status='PENDING',status='FAILED',due_at,updated_at,true
 FROM ops.maintenance_jobs WHERE status IN('PENDING','FAILED')
 UNION ALL
 SELECT 'RANKING_REBUILDS',operation_id::text,operation_id::text,status,status IN('PENDING','RUNNING'),status='FAILED',
  CASE WHEN status='PENDING' AND failure_code IS NULL THEN created_at
       WHEN status='PENDING' THEN updated_at+interval '5 seconds'
       WHEN status='RUNNING' THEN updated_at+interval '2 minutes' END,updated_at,false
 FROM rankings.rebuild_jobs WHERE status IN('PENDING','RUNNING','FAILED')
 UNION ALL
 SELECT 'OPS_REMOTE',operation_id::text,operation_id::text,state,state IN('PENDING','DISPATCHING','RETRY'),state='NEEDS_REVIEW',
  CASE WHEN state='DISPATCHING' THEN lease_expires_at ELSE next_attempt_at END,updated_at,true
 FROM ops.remote_dispatches WHERE state IN('PENDING','DISPATCHING','RETRY','NEEDS_REVIEW')
$$;
REVOKE ALL ON FUNCTION ops.runtime_job_items() FROM PUBLIC;

-- Preserve the original aggregate port and its empty-queue zero rows.
CREATE OR REPLACE FUNCTION ops.runtime_jobs_read()
RETURNS TABLE(kind TEXT,pending_count BIGINT,attention_count BIGINT,due_count BIGINT,oldest_due_at TIMESTAMPTZ)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog AS $$
 SELECT kinds.kind,count(j.job_id) FILTER(WHERE j.pending),count(j.job_id) FILTER(WHERE j.attention),
 count(j.job_id) FILTER(WHERE j.pending AND j.run_at<=statement_timestamp()),min(j.run_at) FILTER(WHERE j.pending)
 FROM unnest(ARRAY['ANNOUNCEMENTS','GAME_ROUNDS','REGISTRATION_GRANTS','KEY_PURPOSE_SYNC','QUOTA_TRANSFERS','MAINTENANCE','RANKING_REBUILDS','OPS_REMOTE']) kinds(kind)
 LEFT JOIN ops.runtime_job_items() j ON j.kind=kinds.kind GROUP BY kinds.kind
$$;
