CREATE TABLE economy.quota_transfers (
 transfer_id UUID PRIMARY KEY,
 newapi_user_id BIGINT NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
 request_key_hash BYTEA NOT NULL CHECK (octet_length(request_key_hash)=32),
 amount_units BIGINT NOT NULL CHECK (amount_units>0 AND amount_units<=9007199254740991),
 status TEXT NOT NULL CHECK (status IN ('PENDING','CONFIRMED','REFUNDED','NEEDS_REVIEW')),
 reason TEXT NOT NULL DEFAULT '', native_before BIGINT, native_after BIGINT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(newapi_user_id,request_key_hash),
 CHECK (status<>'CONFIRMED' OR (native_before IS NOT NULL AND native_after IS NOT NULL AND native_after::numeric=native_before::numeric+amount_units))
);
CREATE UNIQUE INDEX quota_transfer_unresolved ON economy.quota_transfers(newapi_user_id) WHERE status IN ('PENDING','NEEDS_REVIEW');
CREATE INDEX quota_transfer_pending ON economy.quota_transfers(created_at,transfer_id) WHERE status='PENDING';
CREATE INDEX quota_transfer_user_history ON economy.quota_transfers(newapi_user_id,transfer_id DESC);
CREATE FUNCTION economy.guard_quota_transfer() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.status<>'PENDING' OR NEW.status='PENDING' OR
 (NEW.transfer_id,NEW.newapi_user_id,NEW.request_key_hash,NEW.amount_units,NEW.created_at) IS DISTINCT FROM
 (OLD.transfer_id,OLD.newapi_user_id,OLD.request_key_hash,OLD.amount_units,OLD.created_at) THEN
 RAISE EXCEPTION 'invalid transfer transition' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER quota_transfer_transition BEFORE UPDATE ON economy.quota_transfers FOR EACH ROW EXECUTE FUNCTION economy.guard_quota_transfer();
CREATE TRIGGER quota_transfer_history BEFORE DELETE OR TRUNCATE ON economy.quota_transfers FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
