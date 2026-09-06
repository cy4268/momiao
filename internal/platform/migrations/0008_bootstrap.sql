-- Deployment-only one-shot initial administrator, IS 177/423/550 and TD 939/940.
-- This does not implement Level 3 Access Control or grant runtime role management.
CREATE TABLE ops.access_control_guards (
 guard_key TEXT PRIMARY KEY CHECK(guard_key='ADMIN_PRINCIPAL_SET')
);
INSERT INTO ops.access_control_guards VALUES('ADMIN_PRINCIPAL_SET');

-- A historical identity is deliberately not an FK to the mutable principal row.
-- Deleting/retiring a live principal must not erase evidence or reopen bootstrap.
CREATE TABLE ops.bootstrap_closure (
 singleton BOOLEAN PRIMARY KEY CHECK(singleton),
 first_admin_principal_id UUID,
 first_newapi_user_id BIGINT CHECK(first_newapi_user_id>0),
 reason TEXT NOT NULL CHECK(reason IN ('PRINCIPAL_OBSERVED','LEGACY_BOOTSTRAP_AUDIT')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE ops.admin_role_history (
 role_history_id UUID PRIMARY KEY,
 admin_principal_id UUID NOT NULL,
 newapi_user_id BIGINT NOT NULL CHECK(newapi_user_id>0),
 action TEXT NOT NULL CHECK(action IN ('SYSTEM_BOOTSTRAP','CREATE','CHANGE_ROLE','CHANGE_SCOPES','DISABLE','ENABLE')),
 previous_role TEXT CHECK(previous_role IN ('SUPER_ADMIN','OPERATOR','AUDITOR')),
 next_role TEXT CHECK(next_role IN ('SUPER_ADMIN','OPERATOR','AUDITOR')),
 previous_status TEXT CHECK(previous_status IN ('ACTIVE','DISABLED')),
 next_status TEXT CHECK(next_status IN ('ACTIVE','DISABLED')),
 authz_epoch BIGINT NOT NULL CHECK(authz_epoch>0),
 actor_kind TEXT NOT NULL CHECK(actor_kind IN ('SYSTEM','ADMIN','OFFLINE')),
 actor_newapi_user_id BIGINT CHECK(actor_newapi_user_id>0),
 operation_id UUID NOT NULL REFERENCES ops.admin_operations(operation_id)
  ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX admin_role_history_principal ON ops.admin_role_history(admin_principal_id,created_at,role_history_id);

CREATE FUNCTION ops.reject_bootstrap_history_change() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN RAISE EXCEPTION 'immutable administrator history' USING ERRCODE='55000'; END $$;
CREATE TRIGGER bootstrap_closure_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON ops.bootstrap_closure
 FOR EACH STATEMENT EXECUTE FUNCTION ops.reject_bootstrap_history_change();
CREATE TRIGGER admin_role_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON ops.admin_role_history
 FOR EACH STATEMENT EXECUTE FUNCTION ops.reject_bootstrap_history_change();
CREATE TRIGGER access_control_guards_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON ops.access_control_guards
 FOR EACH STATEMENT EXECUTE FUNCTION ops.reject_bootstrap_history_change();

-- All current/future principal-set mutations serialize before taking row locks.
-- Formal Level 3 writers still need actor/proof/version/last-super-admin checks;
-- this trigger is the shared lock, not that authorization implementation.
CREATE FUNCTION ops.lock_admin_principal_set() RETURNS trigger
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
BEGIN
 PERFORM pg_catalog.pg_advisory_xact_lock(714622314783630008);
 PERFORM 1 FROM ops.access_control_guards WHERE guard_key='ADMIN_PRINCIPAL_SET' FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'BOOTSTRAP_GUARD_UNAVAILABLE' USING ERRCODE='55000'; END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER admin_set_guard BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON ops.admin_principals
 FOR EACH STATEMENT EXECUTE FUNCTION ops.lock_admin_principal_set();
CREATE TRIGGER admin_scope_set_guard BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON ops.admin_principal_scopes
 FOR EACH STATEMENT EXECUTE FUNCTION ops.lock_admin_principal_set();

CREATE FUNCTION ops.close_bootstrap_on_principal() RETURNS trigger
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
BEGIN
 INSERT INTO ops.bootstrap_closure(singleton,first_admin_principal_id,first_newapi_user_id,reason)
 VALUES(true,NEW.admin_principal_id,NEW.newapi_user_id,'PRINCIPAL_OBSERVED')
 ON CONFLICT(singleton) DO NOTHING;
 RETURN NEW;
END $$;
CREATE TRIGGER principal_closes_bootstrap AFTER INSERT ON ops.admin_principals
 FOR EACH ROW EXECUTE FUNCTION ops.close_bootstrap_on_principal();

-- Capture pre-migration authority, including disabled or non-super principals.
INSERT INTO ops.bootstrap_closure(singleton,first_admin_principal_id,first_newapi_user_id,reason,created_at)
 SELECT true,admin_principal_id,newapi_user_id,'PRINCIPAL_OBSERVED',created_at
 FROM ops.admin_principals ORDER BY created_at,admin_principal_id LIMIT 1
 ON CONFLICT(singleton) DO NOTHING;
INSERT INTO ops.bootstrap_closure(singleton,first_newapi_user_id,reason,created_at)
 SELECT true,newapi_user_id,'LEGACY_BOOTSTRAP_AUDIT',created_at
 FROM ops.admin_operations WHERE action='SYSTEM_BOOTSTRAP' ORDER BY created_at,operation_id LIMIT 1
 ON CONFLICT(singleton) DO NOTHING;

-- Only a separate, temporary deployment identity may receive EXECUTE. The
-- deployment tool verifies the source before BEGIN; DB privilege is not proof
-- of native identity. No ordinary runtime role receives this function or DML.
CREATE FUNCTION ops.bootstrap_super_admin(
 p_environment TEXT,p_user_id BIGINT,p_username TEXT,p_release_build TEXT,p_expected_empty BOOLEAN,
 p_principal_id UUID,p_history_id UUID,p_operation_id UUID
) RETURNS TABLE(admin_principal_id UUID,operation_id UUID,created_at TIMESTAMPTZ)
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE
 v_now TIMESTAMPTZ;
 v_details JSONB;
 v_hash TEXT;
BEGIN
 IF p_environment IS NULL OR p_environment NOT IN ('DEVELOPMENT','STAGING','PRODUCTION')
  OR p_user_id IS NULL OR p_user_id<=0 OR p_username IS NULL
  OR char_length(p_username) NOT BETWEEN 1 AND 128
  OR p_release_build IS NULL OR p_release_build !~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$'
  OR p_expected_empty IS DISTINCT FROM true
  OR p_principal_id IS NULL OR p_history_id IS NULL OR p_operation_id IS NULL THEN
  RAISE EXCEPTION 'BOOTSTRAP_INVALID_INPUT' USING ERRCODE='22023';
 END IF;
 IF current_setting('transaction_isolation')<>'read committed' THEN
  RAISE EXCEPTION 'BOOTSTRAP_ISOLATION_REQUIRED' USING ERRCODE='25000';
 END IF;
 PERFORM pg_catalog.pg_advisory_xact_lock(714622314783630008);
 PERFORM 1 FROM ops.access_control_guards WHERE guard_key='ADMIN_PRINCIPAL_SET' FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'BOOTSTRAP_GUARD_UNAVAILABLE' USING ERRCODE='55000'; END IF;
 IF EXISTS(SELECT 1 FROM ops.admin_principals)
  OR EXISTS(SELECT 1 FROM ops.bootstrap_closure)
  OR EXISTS(SELECT 1 FROM ops.admin_role_history)
  OR EXISTS(SELECT 1 FROM ops.admin_operations WHERE action='SYSTEM_BOOTSTRAP') THEN
  RAISE EXCEPTION 'BOOTSTRAP_ALREADY_CLOSED' USING ERRCODE='P0001';
 END IF;
 INSERT INTO identity.account_refs(newapi_user_id) VALUES(p_user_id) ON CONFLICT DO NOTHING;
 v_now:=clock_timestamp();
 INSERT INTO ops.admin_principals(admin_principal_id,newapi_user_id,base_role,status,created_at,updated_at)
 VALUES(p_principal_id,p_user_id,'SUPER_ADMIN','ACTIVE',v_now,v_now);
 INSERT INTO ops.admin_role_history(role_history_id,admin_principal_id,newapi_user_id,action,next_role,next_status,authz_epoch,actor_kind,operation_id,created_at)
 VALUES(p_history_id,p_principal_id,p_user_id,'SYSTEM_BOOTSTRAP','SUPER_ADMIN','ACTIVE',1,'SYSTEM',p_operation_id,v_now);
 v_details:=jsonb_build_object('environment',p_environment,'target_newapi_user_id',p_user_id::text,
  'expected_username',p_username,'created_principal',p_principal_id::text,'release_build',p_release_build,
  'actor','SYSTEM_BOOTSTRAP','timestamp',v_now);
 v_hash:=encode(sha256(convert_to(v_details::text,'UTF8')),'hex');
 INSERT INTO ops.admin_operations(operation_id,actor_kind,newapi_user_id,action,request_hash,details,result,created_at)
 VALUES(p_operation_id,'SYSTEM',NULL,'SYSTEM_BOOTSTRAP',v_hash,v_details,
  jsonb_build_object('admin_principal_id',p_principal_id::text,'role_history_id',p_history_id::text),v_now);
 RETURN QUERY SELECT p_principal_id,p_operation_id,v_now;
END $$;

-- Existing runtime announcement writers can INSERT generic admin_operations.
-- A dedicated invoker trigger prevents that grant from forging bootstrap audit:
-- inside the SECURITY DEFINER entry point current_user is its controlled owner;
-- direct runtime INSERT stays runtime. No caller-set GUC is trusted as proof.
CREATE FUNCTION ops.restrict_bootstrap_audit() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.action='SYSTEM_BOOTSTRAP' OR NEW.details->>'actor'='SYSTEM_BOOTSTRAP' THEN
  IF current_user IS DISTINCT FROM (
   SELECT pg_catalog.pg_get_userbyid(proowner) FROM pg_catalog.pg_proc
   WHERE oid='ops.bootstrap_super_admin(text,bigint,text,text,boolean,uuid,uuid,uuid)'::regprocedure
  ) THEN
   RAISE EXCEPTION 'bootstrap audit requires deployment authority' USING ERRCODE='42501';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER bootstrap_audit_authority BEFORE INSERT ON ops.admin_operations
 FOR EACH ROW EXECUTE FUNCTION ops.restrict_bootstrap_audit();

REVOKE ALL ON ops.access_control_guards,ops.bootstrap_closure,ops.admin_role_history FROM PUBLIC;
REVOKE ALL ON FUNCTION ops.reject_bootstrap_history_change(),ops.lock_admin_principal_set(),ops.close_bootstrap_on_principal(),ops.restrict_bootstrap_audit() FROM PUBLIC;
REVOKE ALL ON FUNCTION ops.bootstrap_super_admin(TEXT,BIGINT,TEXT,TEXT,BOOLEAN,UUID,UUID,UUID) FROM PUBLIC;
