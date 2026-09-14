-- IS105/442-446 durable authority only. Native Ops management is a later gate.
-- Existing migrations, Ops scopes, previews and credential schemas stay intact.
CREATE TABLE ops.maintenance_scope_guards (
 scope text PRIMARY KEY CHECK(scope IN (
  'CHALDEA_USER_WRITES','WALLET_EXCHANGE','REWARDS','DIRECT_PLAY_NEW_ROUNDS',
  'POKER_NEW_TABLES_NEW_HANDS','RANKINGS_PUBLISHING','ANNOUNCEMENTS_SCHEDULING'))
);
INSERT INTO ops.maintenance_scope_guards(scope) VALUES
 ('CHALDEA_USER_WRITES'),('WALLET_EXCHANGE'),('REWARDS'),('DIRECT_PLAY_NEW_ROUNDS'),
 ('POKER_NEW_TABLES_NEW_HANDS'),('RANKINGS_PUBLISHING'),('ANNOUNCEMENTS_SCHEDULING');
CREATE TRIGGER maintenance_scope_guards_immutable
 BEFORE UPDATE OR DELETE OR TRUNCATE ON ops.maintenance_scope_guards
 FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();

CREATE TABLE ops.maintenance_windows (
 maintenance_id uuid PRIMARY KEY,
 state text NOT NULL CHECK(state IN (
  'DRAFT','SCHEDULED','ACTIVE','ENDING','COMPLETED','CANCELLED',
  'ACTIVATION_FAILED','ENDING_FAILED')),
 reason text NOT NULL CHECK(octet_length(reason)>0),
 impact_snapshot jsonb NOT NULL CHECK(jsonb_typeof(impact_snapshot)='object'),
 impact_hash bytea NOT NULL CHECK(octet_length(impact_hash)=32),
 environment text NOT NULL CHECK(environment IN ('DEVELOPMENT','STAGING','PRODUCTION')),
 state_version bigint NOT NULL DEFAULT 1 CHECK(state_version>0),
 scheduled_start_at timestamptz, scheduled_end_at timestamptz,
 estimated_end_at timestamptz, activated_at timestamptz, ended_at timestamptz,
 created_by bigint NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
 activated_by bigint REFERENCES identity.account_refs ON DELETE RESTRICT,
 ended_by bigint REFERENCES identity.account_refs ON DELETE RESTRICT,
 operation_id uuid NOT NULL UNIQUE REFERENCES ops.admin_operations
  ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK(scheduled_end_at IS NULL OR
  (scheduled_start_at IS NOT NULL AND scheduled_end_at>scheduled_start_at)),
 CHECK(ended_at IS NULL OR activated_at IS NULL OR ended_at>=activated_at)
);
CREATE TABLE ops.maintenance_window_scopes (
 maintenance_id uuid NOT NULL REFERENCES ops.maintenance_windows ON DELETE RESTRICT,
 scope text NOT NULL REFERENCES ops.maintenance_scope_guards ON DELETE RESTRICT,
 PRIMARY KEY(maintenance_id,scope)
);
CREATE INDEX maintenance_scopes_lookup ON ops.maintenance_window_scopes(scope,maintenance_id);

-- Read-only projection: no locks, no Redis and no writes. A missing/unknown
-- scope is a configuration/input error, never an invented inactive state.
CREATE FUNCTION ops.is_maintenance_scope_active(p_scope text)
 RETURNS boolean LANGUAGE plpgsql STABLE SECURITY DEFINER
 SET search_path=pg_catalog AS $$
BEGIN
 IF p_scope IS NULL OR NOT EXISTS (
  SELECT 1 FROM ops.maintenance_scope_guards WHERE scope=p_scope) THEN
  RAISE EXCEPTION 'MAINTENANCE_SCOPE_INVALID' USING ERRCODE='22023';
 END IF;
 RETURN EXISTS (
  SELECT 1 FROM ops.maintenance_window_scopes s
  JOIN ops.maintenance_windows w USING(maintenance_id)
  WHERE s.scope=p_scope AND w.state='ACTIVE');
END $$;

-- Mutation-only linearization. Call in the accepting READ COMMITTED tx, then
-- read is_maintenance_scope_active in a NEW statement after any lock wait.
-- Ops Activate/End must take the same lexical guards FOR UPDATE, without
-- taking Poker table locks. These SHARE locks remain held until tx end.
CREATE FUNCTION ops.lock_poker_admission_scopes()
 RETURNS void LANGUAGE plpgsql VOLATILE SECURITY DEFINER
 SET search_path=pg_catalog AS $$
DECLARE v_scope text; v_count integer:=0;
BEGIN
 IF current_setting('transaction_read_only')<>'off' OR
  current_setting('transaction_isolation')<>'read committed' THEN
  RAISE EXCEPTION 'POKER_ADMISSION_TRANSACTION_REQUIRED' USING ERRCODE='25000';
 END IF;
 FOR v_scope IN
  SELECT scope FROM ops.maintenance_scope_guards
  WHERE scope IN ('CHALDEA_USER_WRITES','POKER_NEW_TABLES_NEW_HANDS')
  ORDER BY scope FOR SHARE
 LOOP
  v_count:=v_count+1;
 END LOOP;
 IF v_count<>2 THEN
  RAISE EXCEPTION 'MAINTENANCE_GUARDS_INCOMPLETE' USING ERRCODE='55000';
 END IF;
END $$;

REVOKE ALL ON ops.maintenance_windows,ops.maintenance_window_scopes,
 ops.maintenance_scope_guards FROM PUBLIC;
REVOKE ALL ON FUNCTION ops.is_maintenance_scope_active(text),
 ops.lock_poker_admission_scopes() FROM PUBLIC;
