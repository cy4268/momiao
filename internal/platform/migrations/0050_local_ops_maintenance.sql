-- Direct saves reuse the existing maintenance authority and admission locks.
ALTER TABLE ops.maintenance_windows
 ADD COLUMN message text NOT NULL DEFAULT '',
 ALTER COLUMN reason DROP NOT NULL,
 ALTER COLUMN impact_snapshot DROP NOT NULL,
 ALTER COLUMN impact_hash DROP NOT NULL,
 ALTER COLUMN operation_id DROP NOT NULL;

-- Scheduled maintenance is removed; existing active windows stay active until
-- an administrator saves the new form. Historical windows/events remain.
UPDATE ops.maintenance_windows SET state='CANCELLED',ended_at=clock_timestamp()
 WHERE state IN ('DRAFT','SCHEDULED');
UPDATE ops.maintenance_jobs SET status='CANCELLED',updated_at=clock_timestamp() WHERE status='PENDING';

CREATE TABLE ops.audit_log (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 actor_user_id bigint NOT NULL REFERENCES identity.account_refs(newapi_user_id),
 occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 action text NOT NULL,
 target text NOT NULL,
 before_state jsonb NOT NULL,
 after_state jsonb NOT NULL
);
CREATE INDEX audit_log_time ON ops.audit_log(occurred_at DESC,id DESC);
REVOKE ALL ON ops.audit_log FROM PUBLIC;
