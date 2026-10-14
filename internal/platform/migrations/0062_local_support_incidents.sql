-- Local role-based support and incidents; existing immutable history is retained.
ALTER TABLE ops.support_cases DROP CONSTRAINT support_cases_subject_newapi_user_id_fkey, ADD FOREIGN KEY (subject_newapi_user_id) REFERENCES identity.users(id) ON DELETE RESTRICT;
ALTER TABLE ops.support_cases DROP CONSTRAINT support_cases_created_by_fkey, ADD FOREIGN KEY (created_by) REFERENCES identity.users(id) ON DELETE RESTRICT;
ALTER TABLE ops.support_verification_facts DROP CONSTRAINT support_verification_facts_actor_newapi_user_id_fkey, ADD FOREIGN KEY (actor_newapi_user_id) REFERENCES identity.users(id) ON DELETE RESTRICT;
ALTER TABLE ops.incidents DROP CONSTRAINT incidents_created_by_fkey, ADD FOREIGN KEY (created_by) REFERENCES identity.users(id) ON DELETE RESTRICT;
ALTER TABLE ops.incident_events DROP CONSTRAINT incident_events_actor_newapi_user_id_fkey, ADD FOREIGN KEY (actor_newapi_user_id) REFERENCES identity.users(id) ON DELETE RESTRICT;
ALTER TABLE ops.support_cases ALTER COLUMN last_operation_id DROP NOT NULL;
ALTER TABLE ops.support_verification_facts ALTER COLUMN operation_id DROP NOT NULL;
ALTER TABLE ops.incidents ALTER COLUMN last_operation_id DROP NOT NULL;
ALTER TABLE ops.incident_events ALTER COLUMN operation_id DROP NOT NULL;

CREATE OR REPLACE FUNCTION ops.guard_support_case_update() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF (NEW.case_id,NEW.subject_newapi_user_id,NEW.category,NEW.safe_summary,NEW.created_by,NEW.created_at) IS DISTINCT FROM
    (OLD.case_id,OLD.subject_newapi_user_id,OLD.category,OLD.safe_summary,OLD.created_by,OLD.created_at) OR
    NEW.version<>OLD.version+1 OR NEW.updated_at<OLD.updated_at THEN
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

CREATE OR REPLACE FUNCTION ops.guard_incident_update() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF (NEW.incident_id,NEW.title,NEW.severity,NEW.safe_summary,NEW.created_by,NEW.created_at) IS DISTINCT FROM
    (OLD.incident_id,OLD.title,OLD.severity,OLD.safe_summary,OLD.created_by,OLD.created_at) OR
    NEW.version<>OLD.version+1 OR NEW.updated_at<OLD.updated_at THEN
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
