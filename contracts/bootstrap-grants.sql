-- Operator-reviewed template; NOT a migration and NOT run by the CLI.
-- Existing dedicated roles are supplied as psql identifiers. This file creates
-- no roles, changes no password, and contains no production identity or secret.
-- Required psql variables: database, function_owner, bootstrap_role, runtime_role.
-- Review role membership, schema ownership and production target before use.
\set ON_ERROR_STOP on
BEGIN;
SELECT set_config('bootstrap_review.owner', :'function_owner', true),
       set_config('bootstrap_review.executor', :'bootstrap_role', true),
       set_config('bootstrap_review.runtime', :'runtime_role', true),
       set_config('bootstrap_review.database', :'database', true);
DO $$
DECLARE r record;
BEGIN
 IF current_database()<>current_setting('bootstrap_review.database') THEN
  RAISE EXCEPTION 'bootstrap grants database mismatch';
 END IF;
 IF current_setting('bootstrap_review.owner')=current_setting('bootstrap_review.executor')
  OR current_setting('bootstrap_review.owner')=current_setting('bootstrap_review.runtime')
  OR current_setting('bootstrap_review.executor')=current_setting('bootstrap_review.runtime') THEN
  RAISE EXCEPTION 'bootstrap roles must be distinct';
 END IF;
 FOR r IN SELECT * FROM pg_catalog.pg_roles WHERE rolname IN
  (current_setting('bootstrap_review.owner'),current_setting('bootstrap_review.executor'),current_setting('bootstrap_review.runtime')) LOOP
  IF r.rolsuper OR r.rolcreatedb OR r.rolcreaterole OR r.rolbypassrls THEN
   RAISE EXCEPTION 'overprivileged bootstrap role';
  END IF;
  IF r.rolname=current_setting('bootstrap_review.owner') AND r.rolcanlogin THEN
   RAISE EXCEPTION 'function owner must be NOLOGIN';
  END IF;
  IF r.rolname=current_setting('bootstrap_review.owner') AND
   EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members WHERE member=r.oid) THEN
   RAISE EXCEPTION 'function owner must have no memberships';
  END IF;
  IF r.rolname=current_setting('bootstrap_review.executor') AND
   (r.rolinherit OR EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members WHERE member=r.oid)) THEN
   RAISE EXCEPTION 'deployment identity must have no inherited or switchable memberships';
  END IF;
 END LOOP;
 IF (SELECT count(*) FROM pg_catalog.pg_roles WHERE rolname IN
  (current_setting('bootstrap_review.owner'),current_setting('bootstrap_review.executor'),current_setting('bootstrap_review.runtime')))<>3 THEN
  RAISE EXCEPTION 'bootstrap roles must already exist';
 END IF;
END $$;
GRANT USAGE ON SCHEMA identity,ops TO :"function_owner";
GRANT SELECT,INSERT ON identity.account_refs,ops.admin_principals,ops.admin_role_history,
 ops.bootstrap_closure,ops.admin_operations TO :"function_owner";
GRANT SELECT,UPDATE ON ops.access_control_guards TO :"function_owner";
ALTER FUNCTION ops.bootstrap_super_admin(TEXT,BIGINT,TEXT,TEXT,BOOLEAN,UUID,UUID,UUID) OWNER TO :"function_owner";
ALTER FUNCTION ops.lock_admin_principal_set() OWNER TO :"function_owner";
ALTER FUNCTION ops.close_bootstrap_on_principal() OWNER TO :"function_owner";
REVOKE ALL ON FUNCTION ops.bootstrap_super_admin(TEXT,BIGINT,TEXT,TEXT,BOOLEAN,UUID,UUID,UUID) FROM PUBLIC,:"runtime_role";
REVOKE ALL ON FUNCTION ops.lock_admin_principal_set(),ops.close_bootstrap_on_principal() FROM PUBLIC,:"bootstrap_role",:"runtime_role";
REVOKE ALL ON ALL TABLES IN SCHEMA identity,ops FROM :"bootstrap_role";
REVOKE CREATE ON SCHEMA identity,ops,public FROM :"bootstrap_role";
REVOKE INSERT,UPDATE,DELETE,TRUNCATE ON ops.admin_principals,ops.admin_principal_scopes,
 ops.admin_role_history,ops.bootstrap_closure,ops.access_control_guards FROM :"runtime_role";
GRANT CONNECT ON DATABASE :"database" TO :"bootstrap_role";
GRANT USAGE ON SCHEMA ops TO :"bootstrap_role";
GRANT EXECUTE ON FUNCTION ops.bootstrap_super_admin(TEXT,BIGINT,TEXT,TEXT,BOOLEAN,UUID,UUID,UUID) TO :"bootstrap_role";
-- REVOKE from one role does not subtract PUBLIC or membership privileges.
-- Abort rather than silently changing unrelated shared grants to compensate.
DO $$
DECLARE executor TEXT:=current_setting('bootstrap_review.executor');
BEGIN
 IF has_function_privilege(current_setting('bootstrap_review.runtime'),
  'ops.bootstrap_super_admin(text,bigint,text,text,boolean,uuid,uuid,uuid)','EXECUTE')
  OR has_table_privilege(executor,'ops.admin_principals','INSERT,UPDATE,DELETE,TRUNCATE')
  OR has_table_privilege(executor,'ops.admin_operations','INSERT,UPDATE,DELETE,TRUNCATE')
  OR has_table_privilege(executor,'ops.admin_role_history','INSERT,UPDATE,DELETE,TRUNCATE')
  OR has_table_privilege(executor,'identity.account_refs','INSERT,UPDATE,DELETE,TRUNCATE')
  OR has_schema_privilege(executor,'ops','CREATE')
  OR has_schema_privilege(executor,'public','CREATE')
  OR has_database_privilege(executor,current_database(),'CREATE') THEN
  RAISE EXCEPTION 'bootstrap effective privileges exceed the narrow entry point';
 END IF;
END $$;
COMMIT;

-- Read-only verification (must report true, false, false respectively):
SELECT has_function_privilege(:'bootstrap_role', 'ops.bootstrap_super_admin(text,bigint,text,text,boolean,uuid,uuid,uuid)', 'EXECUTE') AS bootstrap_execute,
 has_function_privilege(:'runtime_role', 'ops.bootstrap_super_admin(text,bigint,text,text,boolean,uuid,uuid,uuid)', 'EXECUTE') AS runtime_execute,
 has_table_privilege(:'bootstrap_role','ops.admin_principals','INSERT') AS bootstrap_direct_insert;
-- After one-shot completion or uncertainty, revoke temporary access separately:
-- REVOKE EXECUTE ON FUNCTION ops.bootstrap_super_admin(TEXT,BIGINT,TEXT,TEXT,BOOLEAN,UUID,UUID,UUID) FROM :"bootstrap_role";
