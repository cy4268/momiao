-- Explicit installation in native PostgreSQL, never an automatic platform migration.
-- Enable only after verifying the exact source and Redis/BATCH_UPDATE disabled.
CREATE SCHEMA momiao_quota;
CREATE TABLE momiao_quota.settings (
 singleton BOOLEAN PRIMARY KEY CHECK(singleton), enabled BOOLEAN NOT NULL DEFAULT false,
 source_revision TEXT NOT NULL CHECK(source_revision='f116414284162ad15d8925f7bca494c109b83e93'),
 raw_units_per_credit BIGINT NOT NULL CHECK(raw_units_per_credit=500000),
 accounting_mode TEXT NOT NULL CHECK(accounting_mode='POSTGRES_DIRECT_NO_REDIS_NO_BATCH')
);
INSERT INTO momiao_quota.settings VALUES(true,false,'f116414284162ad15d8925f7bca494c109b83e93',500000,'POSTGRES_DIRECT_NO_REDIS_NO_BATCH');
CREATE TABLE momiao_quota.operations (
 operation_id UUID PRIMARY KEY, newapi_user_id BIGINT NOT NULL CHECK(newapi_user_id>0),
 amount_units BIGINT NOT NULL CHECK(amount_units>0 AND amount_units<=9007199254740991),
 before_quota BIGINT, after_quota BIGINT,
 result TEXT NOT NULL CHECK(result IN ('APPLIED','ACCOUNT_RESTRICTED','SOURCE_INCOMPATIBLE','BALANCE_OVERFLOW')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(result<>'APPLIED' OR (before_quota IS NOT NULL AND after_quota IS NOT NULL AND before_quota>=0 AND after_quota::numeric=before_quota::numeric+amount_units))
);
CREATE FUNCTION momiao_quota.reject_history() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN RAISE EXCEPTION 'immutable quota operation' USING ERRCODE='55000'; END;
$$;
CREATE TRIGGER quota_operation_history BEFORE UPDATE OR DELETE OR TRUNCATE ON momiao_quota.operations FOR EACH STATEMENT EXECUTE FUNCTION momiao_quota.reject_history();
CREATE FUNCTION momiao_quota.read_quota(p_user BIGINT) RETURNS TABLE(user_id BIGINT,quota BIGINT,enabled BOOLEAN)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog AS $$
 SELECT u.id,u.quota,s.enabled FROM public.users u CROSS JOIN momiao_quota.settings s
 WHERE u.id=p_user AND u.status=1 AND u.deleted_at IS NULL AND u.quota>=0;
$$;
CREATE FUNCTION momiao_quota.query_operation(p_id UUID,p_user BIGINT) RETURNS SETOF momiao_quota.operations
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog AS $$
 SELECT * FROM momiao_quota.operations WHERE operation_id=p_id AND newapi_user_id=p_user;
$$;
CREATE FUNCTION momiao_quota.credit(p_id UUID,p_user BIGINT,p_amount BIGINT) RETURNS SETOF momiao_quota.operations
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE old_op momiao_quota.operations%ROWTYPE; before_value BIGINT; user_status BIGINT; deleted TIMESTAMPTZ; outcome TEXT;
BEGIN
 IF p_id IS NULL OR p_user IS NULL OR p_user<=0 OR p_amount IS NULL OR p_amount<=0 OR p_amount>9007199254740991 THEN
 RAISE EXCEPTION 'invalid quota operation' USING ERRCODE='22023'; END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended('momiao-quota:'||p_id::text,0));
 SELECT * INTO old_op FROM momiao_quota.operations WHERE operation_id=p_id;
 IF FOUND THEN
   IF old_op.newapi_user_id<>p_user OR old_op.amount_units<>p_amount THEN RAISE EXCEPTION 'quota operation conflict' USING ERRCODE='22023'; END IF;
   RETURN NEXT old_op; RETURN;
 END IF;
 IF NOT EXISTS(SELECT 1 FROM momiao_quota.settings WHERE enabled) THEN RAISE EXCEPTION 'quota bridge not enabled' USING ERRCODE='55000'; END IF;
 SELECT u.quota,u.status,u.deleted_at INTO before_value,user_status,deleted FROM public.users u WHERE u.id=p_user FOR UPDATE;
 IF NOT FOUND OR user_status IS DISTINCT FROM 1 OR deleted IS NOT NULL THEN outcome:='ACCOUNT_RESTRICTED';
 ELSIF before_value IS NULL OR before_value<0 THEN outcome:='SOURCE_INCOMPATIBLE';
 -- Technical range: the native auth/browser DTO currently uses JS-safe integers.
 ELSIF before_value>9007199254740991-p_amount THEN outcome:='BALANCE_OVERFLOW';
 ELSE UPDATE public.users SET quota=quota+p_amount WHERE id=p_user; outcome:='APPLIED'; END IF;
 INSERT INTO momiao_quota.operations(operation_id,newapi_user_id,amount_units,before_quota,after_quota,result)
 VALUES(p_id,p_user,p_amount,before_value,CASE WHEN outcome='APPLIED' THEN before_value+p_amount ELSE before_value END,outcome);
 RETURN QUERY SELECT * FROM momiao_quota.operations WHERE operation_id=p_id;
END;
$$;
REVOKE ALL ON SCHEMA momiao_quota FROM PUBLIC;
REVOKE ALL ON ALL TABLES IN SCHEMA momiao_quota FROM PUBLIC;
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA momiao_quota FROM PUBLIC;
