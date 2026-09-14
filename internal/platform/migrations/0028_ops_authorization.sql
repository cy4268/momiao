-- General Chaldea Operations authority. Existing Models/Announcements audit rows
-- remain valid legacy rows; new typed operations use the extended columns.
ALTER TABLE ops.admin_principal_scopes
 DROP CONSTRAINT admin_principal_scopes_scope_check;
ALTER TABLE ops.admin_principal_scopes
 ADD CONSTRAINT admin_principal_scopes_scope_check CHECK(scope IN (
  'MODELS','USERS_IDENTITY','GAMES','POKER','REWARDS','RANKINGS','RECORDS','ANNOUNCEMENTS'));

ALTER TABLE ops.admin_principals
 ADD COLUMN created_by BIGINT REFERENCES identity.account_refs ON DELETE RESTRICT,
 ADD COLUMN updated_by BIGINT REFERENCES identity.account_refs ON DELETE RESTRICT,
 ADD COLUMN disabled_at TIMESTAMPTZ,
 ADD COLUMN disabled_by BIGINT REFERENCES identity.account_refs ON DELETE RESTRICT;

DROP TRIGGER operations_immutable ON ops.admin_operations;

ALTER TABLE ops.admin_operations
 ADD COLUMN operation_type TEXT,
 ADD COLUMN actor_role_snapshot TEXT,
 ADD COLUMN actor_scopes_snapshot TEXT[],
 ADD COLUMN actor_authz_epoch_snapshot BIGINT,
 ADD COLUMN required_permission TEXT,
 ADD COLUMN risk_level TEXT,
 ADD COLUMN target_type TEXT,
 ADD COLUMN target_id TEXT,
 ADD COLUMN target_version_snapshot TEXT,
 ADD COLUMN input_schema_version TEXT,
 ADD COLUMN input_payload JSONB,
 ADD COLUMN input_hash BYTEA,
 ADD COLUMN environment TEXT,
 ADD COLUMN impact_schema_version TEXT,
 ADD COLUMN impact_preview JSONB,
 ADD COLUMN impact_hash BYTEA,
 ADD COLUMN confirmation_mode TEXT,
 ADD COLUMN requires_fresh_auth BOOLEAN,
 ADD COLUMN confirmation_challenge_hash BYTEA,
 ADD COLUMN fresh_renewal_hash BYTEA,
 ADD COLUMN fresh_renewal_expires_at TIMESTAMPTZ,
 ADD COLUMN fresh_auth_verified_at TIMESTAMPTZ,
 ADD COLUMN state TEXT NOT NULL DEFAULT 'SUCCEEDED',
 ADD COLUMN failure_code TEXT,
 ADD COLUMN scheduled_for TIMESTAMPTZ,
 ADD COLUMN state_version BIGINT NOT NULL DEFAULT 1,
 ADD COLUMN authorized_at TIMESTAMPTZ,
 ADD COLUMN executing_at TIMESTAMPTZ,
 ADD COLUMN completed_at TIMESTAMPTZ,
 ADD COLUMN related_business_id TEXT,
 ADD COLUMN request_id TEXT,
 ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 ADD CONSTRAINT admin_operation_typed_shape CHECK(operation_type IS NULL OR (
  actor_kind='ADMIN' AND newapi_user_id IS NOT NULL AND newapi_user_id>0 AND
  operation_type ~ '^[A-Z][A-Z0-9_]{0,127}$' AND action=operation_type AND
  actor_role_snapshot IN ('SUPER_ADMIN','OPERATOR','AUDITOR') AND
  actor_scopes_snapshot IS NOT NULL AND actor_authz_epoch_snapshot IS NOT NULL AND
  actor_authz_epoch_snapshot>0 AND required_permission IS NOT NULL AND
  required_permission ~ '^[a-z][a-z0-9_.]{0,127}$' AND
  risk_level IN ('LEVEL_1_ROUTINE','LEVEL_2_IMPACTFUL','LEVEL_3_CRITICAL') AND
  target_type IS NOT NULL AND octet_length(target_type) BETWEEN 1 AND 128 AND
  target_id IS NOT NULL AND octet_length(target_id) BETWEEN 1 AND 512 AND
  target_version_snapshot IS NOT NULL AND octet_length(target_version_snapshot) BETWEEN 1 AND 128 AND
  input_schema_version IS NOT NULL AND input_payload IS NOT NULL AND jsonb_typeof(input_payload)='object' AND
  input_hash IS NOT NULL AND octet_length(input_hash)=32 AND
  environment IN ('DEVELOPMENT','STAGING','PRODUCTION') AND
  impact_schema_version IS NOT NULL AND impact_preview IS NOT NULL AND jsonb_typeof(impact_preview)='object' AND
  impact_hash IS NOT NULL AND octet_length(impact_hash)=32 AND
  confirmation_mode IN ('NONE','EXPLICIT','TYPED') AND requires_fresh_auth IS NOT NULL AND
  (risk_level<>'LEVEL_1_ROUTINE' OR requires_fresh_auth=false) AND
  (risk_level<>'LEVEL_3_CRITICAL' OR requires_fresh_auth=true))),
 ADD CONSTRAINT admin_operation_state_check CHECK(state IN (
  'PREPARED','AUTHORIZED','EXECUTING','SUCCEEDED','FAILED_NO_EFFECT','RECOVERING','NEEDS_REVIEW','CANCELLED')),
 ADD CONSTRAINT admin_operation_state_version_check CHECK(state_version>0),
 ADD CONSTRAINT admin_operation_failure_code_check CHECK(
  failure_code IS NULL OR (octet_length(failure_code) BETWEEN 1 AND 128 AND failure_code ~ '^[A-Z][A-Z0-9_]*$')),
 ADD CONSTRAINT admin_operation_challenge_hash_check CHECK(
  confirmation_challenge_hash IS NULL OR octet_length(confirmation_challenge_hash)=32),
 ADD CONSTRAINT admin_operation_fresh_renewal_check CHECK(
  (fresh_renewal_hash IS NULL AND fresh_renewal_expires_at IS NULL) OR
  (state='PREPARED' AND requires_fresh_auth=true AND confirmation_challenge_hash IS NULL AND
   octet_length(fresh_renewal_hash)=32 AND fresh_renewal_expires_at IS NOT NULL));

CREATE FUNCTION ops.guard_admin_operation_update() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE v_transition BOOLEAN;
BEGIN
 IF OLD.operation_type IS NULL THEN
  RAISE EXCEPTION 'immutable legacy admin operation' USING ERRCODE='55000';
 END IF;
 IF (NEW.operation_id,NEW.actor_kind,NEW.newapi_user_id,NEW.action,NEW.announcement_id,NEW.model_id,
     NEW.request_hash,NEW.details,NEW.created_at,NEW.operation_type,NEW.actor_role_snapshot,
     NEW.actor_scopes_snapshot,NEW.actor_authz_epoch_snapshot,NEW.required_permission,NEW.risk_level,NEW.target_type,NEW.target_id,
     NEW.target_version_snapshot,NEW.input_schema_version,NEW.input_payload,NEW.input_hash,
     NEW.environment,NEW.impact_schema_version,NEW.impact_preview,NEW.impact_hash,
     NEW.confirmation_mode,NEW.requires_fresh_auth,NEW.scheduled_for) IS DISTINCT FROM
    (OLD.operation_id,OLD.actor_kind,OLD.newapi_user_id,OLD.action,OLD.announcement_id,OLD.model_id,
     OLD.request_hash,OLD.details,OLD.created_at,OLD.operation_type,OLD.actor_role_snapshot,
     OLD.actor_scopes_snapshot,OLD.actor_authz_epoch_snapshot,OLD.required_permission,OLD.risk_level,OLD.target_type,OLD.target_id,
     OLD.target_version_snapshot,OLD.input_schema_version,OLD.input_payload,OLD.input_hash,
     OLD.environment,OLD.impact_schema_version,OLD.impact_preview,OLD.impact_hash,
     OLD.confirmation_mode,OLD.requires_fresh_auth,OLD.scheduled_for) THEN
  RAISE EXCEPTION 'immutable admin operation core' USING ERRCODE='55000';
 END IF;
 IF NEW.state_version<>OLD.state_version+1 OR NEW.updated_at<OLD.updated_at THEN
  RAISE EXCEPTION 'invalid admin operation state version' USING ERRCODE='55000';
 END IF;
 v_transition :=
  (OLD.state='PREPARED' AND NEW.state IN ('PREPARED','AUTHORIZED','FAILED_NO_EFFECT','CANCELLED')) OR
  (OLD.state='AUTHORIZED' AND NEW.state IN ('EXECUTING','FAILED_NO_EFFECT')) OR
  (OLD.state='EXECUTING' AND NEW.state IN ('SUCCEEDED','FAILED_NO_EFFECT','RECOVERING','NEEDS_REVIEW')) OR
  (OLD.state='RECOVERING' AND NEW.state IN ('SUCCEEDED','FAILED_NO_EFFECT','RECOVERING','NEEDS_REVIEW')) OR
  (OLD.state='NEEDS_REVIEW' AND NEW.state IN ('RECOVERING','NEEDS_REVIEW'));
 IF NOT v_transition THEN
  RAISE EXCEPTION 'invalid admin operation state transition' USING ERRCODE='55000';
 END IF;
 IF NEW.confirmation_challenge_hash IS DISTINCT FROM OLD.confirmation_challenge_hash AND NOT (
    OLD.state='PREPARED' AND NEW.state='PREPARED' AND OLD.requires_fresh_auth=true OR
    OLD.state='PREPARED' AND NEW.state='CANCELLED' AND NEW.confirmation_challenge_hash IS NULL) THEN
  RAISE EXCEPTION 'immutable confirmation challenge' USING ERRCODE='55000';
 END IF;
 IF OLD.fresh_auth_verified_at IS NOT NULL AND
    NEW.fresh_auth_verified_at IS DISTINCT FROM OLD.fresh_auth_verified_at THEN
  RAISE EXCEPTION 'immutable fresh authentication evidence' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER admin_operation_state_guard BEFORE UPDATE ON ops.admin_operations
 FOR EACH ROW EXECUTE FUNCTION ops.guard_admin_operation_update();
CREATE TRIGGER admin_operation_remove_guard BEFORE DELETE OR TRUNCATE ON ops.admin_operations
 FOR EACH STATEMENT EXECUTE FUNCTION content.reject_history_change();

CREATE TABLE ops.admin_operation_events (
 event_id UUID PRIMARY KEY,
 operation_id UUID NOT NULL REFERENCES ops.admin_operations ON DELETE RESTRICT,
 state TEXT NOT NULL CHECK(state IN (
  'PREPARED','AUTHORIZED','EXECUTING','SUCCEEDED','FAILED_NO_EFFECT','RECOVERING','NEEDS_REVIEW','CANCELLED')),
 substate TEXT CHECK(substate IS NULL OR (octet_length(substate) BETWEEN 1 AND 128 AND substate ~ '^[A-Z][A-Z0-9_]*$')),
 state_version BIGINT NOT NULL CHECK(state_version>0),
 failure_code TEXT CHECK(failure_code IS NULL OR (octet_length(failure_code) BETWEEN 1 AND 128 AND failure_code ~ '^[A-Z][A-Z0-9_]*$')),
 safe_details JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(safe_details)='object' AND octet_length(safe_details::text)<=8192),
 occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(operation_id,state_version)
);
CREATE INDEX admin_operation_events_timeline ON ops.admin_operation_events(operation_id,state_version);
CREATE TRIGGER admin_operation_events_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON ops.admin_operation_events
 FOR EACH STATEMENT EXECUTE FUNCTION content.reject_history_change();

CREATE SCHEMA audit;
CREATE TABLE audit.audit_events (
 audit_id UUID PRIMARY KEY,
 actor_newapi_user_id BIGINT NOT NULL CHECK(actor_newapi_user_id>0),
 actor_role TEXT NOT NULL CHECK(actor_role IN ('SUPER_ADMIN','OPERATOR','AUDITOR')),
 actor_scopes_snapshot TEXT[] NOT NULL,
 action TEXT NOT NULL CHECK(octet_length(action) BETWEEN 1 AND 128),
 target_type TEXT NOT NULL CHECK(octet_length(target_type) BETWEEN 1 AND 128),
 target_id TEXT NOT NULL CHECK(octet_length(target_id) BETWEEN 1 AND 512),
 before_snapshot JSONB NOT NULL CHECK(jsonb_typeof(before_snapshot)='object' AND octet_length(before_snapshot::text)<=32768),
 after_snapshot JSONB NOT NULL CHECK(jsonb_typeof(after_snapshot)='object' AND octet_length(after_snapshot::text)<=32768),
 reason TEXT NOT NULL CHECK(octet_length(reason) BETWEEN 1 AND 2048),
 operation_id UUID NOT NULL REFERENCES ops.admin_operations ON DELETE RESTRICT,
 result JSONB NOT NULL CHECK(jsonb_typeof(result)='object' AND octet_length(result::text)<=16384),
 related_business_id TEXT CHECK(related_business_id IS NULL OR octet_length(related_business_id) BETWEEN 1 AND 512),
 request_id TEXT CHECK(request_id IS NULL OR octet_length(request_id) BETWEEN 1 AND 128),
 environment TEXT NOT NULL CHECK(environment IN ('DEVELOPMENT','STAGING','PRODUCTION')),
 occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(operation_id)
);
CREATE INDEX audit_events_occurred ON audit.audit_events(occurred_at DESC,audit_id DESC);
CREATE INDEX audit_events_target ON audit.audit_events(target_type,target_id,occurred_at DESC);
CREATE FUNCTION audit.reject_audit_change() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN RAISE EXCEPTION 'append-only audit authority' USING ERRCODE='55000'; END $$;
CREATE TRIGGER audit_events_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON audit.audit_events
 FOR EACH STATEMENT EXECUTE FUNCTION audit.reject_audit_change();

REVOKE ALL ON SCHEMA audit FROM PUBLIC;
REVOKE ALL ON ops.admin_operation_events,audit.audit_events FROM PUBLIC;
REVOKE ALL ON FUNCTION ops.guard_admin_operation_update(),audit.reject_audit_change() FROM PUBLIC;
