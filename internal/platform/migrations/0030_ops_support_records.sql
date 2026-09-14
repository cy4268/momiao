-- Durable O04 support, incident and Records-access authorities.
ALTER TABLE identity.master_profiles
 ADD COLUMN rename_required BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE ops.support_cases (
 case_id UUID PRIMARY KEY,
 subject_newapi_user_id BIGINT NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
 category TEXT NOT NULL CHECK(category IN ('ACCOUNT_ACCESS','IDENTITY','ECONOMY','GAMEPLAY','OTHER')),
 state TEXT NOT NULL DEFAULT 'OPEN' CHECK(state IN ('OPEN','VERIFYING','APPROVED','EXECUTED','REJECTED','CLOSED')),
 safe_summary TEXT NOT NULL CHECK(octet_length(safe_summary) BETWEEN 1 AND 2048),
 version BIGINT NOT NULL DEFAULT 1 CHECK(version>0),
 created_by BIGINT NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
 last_operation_id UUID NOT NULL REFERENCES ops.admin_operations ON DELETE RESTRICT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 closed_at TIMESTAMPTZ,
 CHECK((state='CLOSED')=(closed_at IS NOT NULL))
);
CREATE INDEX support_cases_subject ON ops.support_cases(subject_newapi_user_id,created_at DESC,case_id DESC);
CREATE INDEX support_cases_state ON ops.support_cases(state,created_at DESC,case_id DESC);

CREATE FUNCTION ops.guard_support_case_update() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF (NEW.case_id,NEW.subject_newapi_user_id,NEW.category,NEW.safe_summary,NEW.created_by,NEW.created_at) IS DISTINCT FROM
    (OLD.case_id,OLD.subject_newapi_user_id,OLD.category,OLD.safe_summary,OLD.created_by,OLD.created_at) OR
    NEW.version<>OLD.version+1 OR NEW.updated_at<OLD.updated_at OR NEW.last_operation_id=OLD.last_operation_id THEN
  RAISE EXCEPTION 'invalid support case update' USING ERRCODE='55000';
 END IF;
 IF NOT (NEW.state=OLD.state OR
   OLD.state='OPEN' AND NEW.state='VERIFYING' OR
   OLD.state='VERIFYING' AND NEW.state IN ('APPROVED','REJECTED') OR
   OLD.state='APPROVED' AND NEW.state='EXECUTED' OR
   OLD.state IN ('EXECUTED','REJECTED') AND NEW.state='CLOSED') THEN
  RAISE EXCEPTION 'invalid support case transition' USING ERRCODE='55000';
 END IF;
 IF OLD.closed_at IS NOT NULL OR
    (NEW.state='CLOSED' AND NEW.closed_at IS NULL) OR
    (NEW.state<>'CLOSED' AND NEW.closed_at IS NOT NULL) THEN
  RAISE EXCEPTION 'invalid support case closure' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER support_case_update_guard BEFORE UPDATE ON ops.support_cases
 FOR EACH ROW EXECUTE FUNCTION ops.guard_support_case_update();
CREATE TRIGGER support_case_remove_guard BEFORE DELETE OR TRUNCATE ON ops.support_cases
 FOR EACH STATEMENT EXECUTE FUNCTION content.reject_history_change();

CREATE TABLE ops.support_verification_facts (
 verification_fact_id UUID PRIMARY KEY,
 case_id UUID NOT NULL REFERENCES ops.support_cases ON DELETE RESTRICT,
 fact_type TEXT NOT NULL CHECK(fact_type IN (
  'IDENTITY_MATCH','OWNERSHIP_VERIFIED','DISCORD_BINDING_VERIFIED','CONTACT_VERIFIED','OTHER_SAFE_METADATA')),
 result TEXT NOT NULL CHECK(result IN ('VERIFIED','NOT_VERIFIED','INCONCLUSIVE')),
 safe_reference TEXT CHECK(safe_reference IS NULL OR (
  octet_length(safe_reference) BETWEEN 1 AND 256 AND safe_reference ~ '^[A-Za-z0-9][A-Za-z0-9:._/-]*$')),
 actor_newapi_user_id BIGINT NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
 operation_id UUID NOT NULL UNIQUE REFERENCES ops.admin_operations ON DELETE RESTRICT,
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX support_verification_facts_case ON ops.support_verification_facts(case_id,recorded_at,verification_fact_id);
CREATE TRIGGER support_verification_facts_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON ops.support_verification_facts
 FOR EACH STATEMENT EXECUTE FUNCTION content.reject_history_change();

CREATE TABLE ops.incidents (
 incident_id UUID PRIMARY KEY,
 title TEXT NOT NULL CHECK(octet_length(title) BETWEEN 1 AND 256),
 severity TEXT NOT NULL CHECK(severity IN ('CRITICAL','WARNING','INFO')),
 state TEXT NOT NULL DEFAULT 'OPEN' CHECK(state IN ('OPEN','TRIAGED','IN_PROGRESS','MONITORING','RESOLVED','CLOSED')),
 safe_summary TEXT NOT NULL CHECK(octet_length(safe_summary) BETWEEN 1 AND 4096),
 version BIGINT NOT NULL DEFAULT 1 CHECK(version>0),
 created_by BIGINT NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
 last_operation_id UUID NOT NULL REFERENCES ops.admin_operations ON DELETE RESTRICT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 resolved_at TIMESTAMPTZ,
 closed_at TIMESTAMPTZ,
 CHECK((state IN ('RESOLVED','CLOSED') OR resolved_at IS NULL) AND
       (state='CLOSED')=(closed_at IS NOT NULL))
);
CREATE INDEX incidents_state ON ops.incidents(state,severity,created_at DESC,incident_id DESC);

CREATE FUNCTION ops.guard_incident_update() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF (NEW.incident_id,NEW.title,NEW.severity,NEW.safe_summary,NEW.created_by,NEW.created_at) IS DISTINCT FROM
    (OLD.incident_id,OLD.title,OLD.severity,OLD.safe_summary,OLD.created_by,OLD.created_at) OR
    NEW.version<>OLD.version+1 OR NEW.updated_at<OLD.updated_at OR NEW.last_operation_id=OLD.last_operation_id THEN
  RAISE EXCEPTION 'invalid incident update' USING ERRCODE='55000';
 END IF;
 IF NOT (NEW.state=OLD.state OR
   OLD.state='OPEN' AND NEW.state='TRIAGED' OR
   OLD.state='TRIAGED' AND NEW.state='IN_PROGRESS' OR
   OLD.state='IN_PROGRESS' AND NEW.state='MONITORING' OR
   OLD.state='MONITORING' AND NEW.state='RESOLVED' OR
   OLD.state='RESOLVED' AND NEW.state='CLOSED' OR
   OLD.state<>'CLOSED' AND NEW.state='CLOSED') THEN
  RAISE EXCEPTION 'invalid incident transition' USING ERRCODE='55000';
 END IF;
 IF OLD.closed_at IS NOT NULL OR
    (NEW.state='CLOSED' AND NEW.closed_at IS NULL) OR
    (NEW.state<>'CLOSED' AND NEW.closed_at IS NOT NULL) OR
    (OLD.resolved_at IS NOT NULL AND NEW.resolved_at IS DISTINCT FROM OLD.resolved_at) OR
    (NEW.state='RESOLVED' AND NEW.resolved_at IS NULL) THEN
  RAISE EXCEPTION 'invalid incident timestamps' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER incident_update_guard BEFORE UPDATE ON ops.incidents
 FOR EACH ROW EXECUTE FUNCTION ops.guard_incident_update();
CREATE TRIGGER incident_remove_guard BEFORE DELETE OR TRUNCATE ON ops.incidents
 FOR EACH STATEMENT EXECUTE FUNCTION content.reject_history_change();

CREATE TABLE ops.incident_events (
 event_id UUID PRIMARY KEY,
 incident_id UUID NOT NULL REFERENCES ops.incidents ON DELETE RESTRICT,
 event_type TEXT NOT NULL CHECK(event_type IN (
  'STATE_CHANGED','ASSIGNED','COMMENT','OPERATION_LINKED','JOB_LINKED','AUDIT_LINKED','BUSINESS_FACT_LINKED')),
 from_state TEXT CHECK(from_state IS NULL OR from_state IN ('OPEN','TRIAGED','IN_PROGRESS','MONITORING','RESOLVED','CLOSED')),
 to_state TEXT CHECK(to_state IS NULL OR to_state IN ('OPEN','TRIAGED','IN_PROGRESS','MONITORING','RESOLVED','CLOSED')),
 safe_summary TEXT CHECK(safe_summary IS NULL OR octet_length(safe_summary) BETWEEN 1 AND 2048),
 related_type TEXT CHECK(related_type IS NULL OR related_type IN ('OPERATION','JOB','AUDIT','BUSINESS_FACT')),
 related_id TEXT CHECK(related_id IS NULL OR octet_length(related_id) BETWEEN 1 AND 512),
 assigned_principal_id UUID REFERENCES ops.admin_principals ON DELETE RESTRICT,
 actor_newapi_user_id BIGINT NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
 operation_id UUID NOT NULL REFERENCES ops.admin_operations ON DELETE RESTRICT,
 occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 CHECK(
  (event_type='STATE_CHANGED' AND from_state IS DISTINCT FROM to_state AND to_state IS NOT NULL AND
   safe_summary IS NULL AND related_type IS NULL AND related_id IS NULL AND assigned_principal_id IS NULL) OR
  (event_type='COMMENT' AND from_state IS NULL AND to_state IS NULL AND safe_summary IS NOT NULL AND
   related_type IS NULL AND related_id IS NULL AND assigned_principal_id IS NULL) OR
  (event_type='ASSIGNED' AND assigned_principal_id IS NOT NULL AND from_state IS NULL AND to_state IS NULL AND
   related_type IS NULL AND related_id IS NULL) OR
  (event_type='OPERATION_LINKED' AND related_type='OPERATION' AND related_id IS NOT NULL AND
   from_state IS NULL AND to_state IS NULL AND assigned_principal_id IS NULL) OR
  (event_type='JOB_LINKED' AND related_type='JOB' AND related_id IS NOT NULL AND
   from_state IS NULL AND to_state IS NULL AND assigned_principal_id IS NULL) OR
  (event_type='AUDIT_LINKED' AND related_type='AUDIT' AND related_id IS NOT NULL AND
   from_state IS NULL AND to_state IS NULL AND assigned_principal_id IS NULL) OR
  (event_type='BUSINESS_FACT_LINKED' AND related_type='BUSINESS_FACT' AND related_id IS NOT NULL AND
   from_state IS NULL AND to_state IS NULL AND assigned_principal_id IS NULL)),
 UNIQUE(operation_id,event_type)
);
CREATE INDEX incident_events_timeline ON ops.incident_events(incident_id,occurred_at,event_id);
CREATE TRIGGER incident_events_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON ops.incident_events
 FOR EACH STATEMENT EXECUTE FUNCTION content.reject_history_change();

CREATE TABLE audit.record_access_events (
 access_id UUID PRIMARY KEY,
 actor_newapi_user_id BIGINT NOT NULL CHECK(actor_newapi_user_id>0),
 actor_role TEXT NOT NULL CHECK(actor_role IN ('SUPER_ADMIN','OPERATOR','AUDITOR')),
 actor_scopes_snapshot TEXT[] NOT NULL,
 actor_authz_epoch BIGINT NOT NULL CHECK(actor_authz_epoch>0),
 subject_newapi_user_id BIGINT NOT NULL CHECK(subject_newapi_user_id>0),
 access_kind TEXT NOT NULL CHECK(access_kind IN ('LIST','DETAIL','VERIFY')),
 record_type TEXT NOT NULL CHECK(record_type IN (
  'HISTORY_LIST','DIRECT_PLAY_ROUND','POKER_SESSION','POKER_HAND','TRANSACTION')),
 record_id TEXT NOT NULL CHECK(octet_length(record_id) BETWEEN 1 AND 512),
 occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX record_access_events_actor ON audit.record_access_events(actor_newapi_user_id,occurred_at DESC,access_id DESC);
CREATE INDEX record_access_events_subject ON audit.record_access_events(subject_newapi_user_id,occurred_at DESC,access_id DESC);
CREATE TRIGGER record_access_events_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON audit.record_access_events
 FOR EACH STATEMENT EXECUTE FUNCTION audit.reject_audit_change();

REVOKE ALL ON ops.support_cases,ops.support_verification_facts,ops.incidents,ops.incident_events,
 audit.record_access_events FROM PUBLIC;
REVOKE ALL ON FUNCTION ops.guard_support_case_update(),ops.guard_incident_update() FROM PUBLIC;
