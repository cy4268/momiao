-- Durable v0 reserves no nickname. Existing complete rows and history survive.
ALTER TABLE identity.master_profiles DROP CONSTRAINT master_profiles_display_name_check;
ALTER TABLE identity.master_profiles DROP CONSTRAINT master_profiles_normalized_name_check;
ALTER TABLE identity.master_profiles DROP CONSTRAINT master_profiles_profile_version_check;
ALTER TABLE identity.master_profiles ALTER COLUMN normalized_name DROP NOT NULL;
ALTER TABLE identity.master_profiles ADD CONSTRAINT master_profiles_state_check CHECK (
 (profile_version=0 AND display_name='' AND normalized_name IS NULL AND nickname_changed_at IS NULL)
 OR (profile_version>0 AND octet_length(display_name) BETWEEN 1 AND 4096
     AND normalized_name IS NOT NULL AND octet_length(normalized_name) BETWEEN 1 AND 8192)
);
CREATE TABLE platform_meta.registration_cursor (
 singleton BOOLEAN PRIMARY KEY CHECK(singleton), ordinal BIGINT NOT NULL DEFAULT 0 CHECK(ordinal>=0),
 source_available BOOLEAN NOT NULL DEFAULT false, last_attempt_at TIMESTAMPTZ, last_success_at TIMESTAMPTZ
);
INSERT INTO platform_meta.registration_cursor(singleton) VALUES(true);

-- Immutable inbox also supplies the one-to-one trusted account-source mapping.
CREATE TABLE identity.native_registration_inbox (
 ordinal BIGINT PRIMARY KEY CHECK(ordinal>0), operation_id UUID NOT NULL UNIQUE,
 native_user_id BIGINT NOT NULL UNIQUE REFERENCES identity.account_refs(newapi_user_id) ON DELETE RESTRICT,
 discord_subject TEXT NOT NULL UNIQUE CHECK(discord_subject ~ '^[1-9][0-9]{16,19}$'),
 source TEXT NOT NULL CHECK(source='NEW_DISCORD_REGISTRATION'),
 policy_version TEXT NOT NULL CHECK(octet_length(policy_version) BETWEEN 1 AND 64),
 native_created_at TIMESTAMPTZ NOT NULL, received_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(ordinal,native_user_id)
);
CREATE TRIGGER native_registration_inbox_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
 ON identity.native_registration_inbox FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TABLE rewards.registration_grants (
 claim_id UUID PRIMARY KEY, newapi_user_id BIGINT NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
 claim_kind TEXT NOT NULL CHECK(claim_kind='INITIAL_GRANT_REGISTRATION'), source_ordinal BIGINT NOT NULL UNIQUE,
 biz_id TEXT NOT NULL UNIQUE CHECK(biz_id='initial_grant:registration:'||newapi_user_id::text),
 status TEXT NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','RECOVERING','CONFIRMED')),
 transaction_id UUID UNIQUE REFERENCES economy.asset_transactions ON DELETE RESTRICT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), confirmed_at TIMESTAMPTZ,
 CHECK((status='CONFIRMED')=(transaction_id IS NOT NULL AND confirmed_at IS NOT NULL)),
 CHECK(status='CONFIRMED' OR (transaction_id IS NULL AND confirmed_at IS NULL)),
 UNIQUE(newapi_user_id,claim_kind), UNIQUE(claim_id,newapi_user_id,biz_id),
 FOREIGN KEY(source_ordinal,newapi_user_id) REFERENCES identity.native_registration_inbox(ordinal,native_user_id) ON DELETE RESTRICT
);
CREATE TABLE platform_meta.registration_grant_jobs (
 claim_id UUID PRIMARY KEY REFERENCES rewards.registration_grants ON DELETE RESTRICT,
 status TEXT NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','RECOVERING','DONE')),
 attempts BIGINT NOT NULL DEFAULT 0 CHECK(attempts>=0), next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 last_error_code TEXT CHECK(last_error_code='GRANT_RETRY_REQUIRED'), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX registration_jobs_pending ON platform_meta.registration_grant_jobs(next_attempt_at,claim_id) WHERE status<>'DONE';

-- Narrow supply issuance fact linked to the original transaction/wallet leg.
CREATE TABLE rewards.registration_issuances (
 claim_id UUID PRIMARY KEY, newapi_user_id BIGINT NOT NULL UNIQUE, biz_id TEXT NOT NULL UNIQUE,
 direction TEXT NOT NULL CHECK(direction='ISSUE'), amount_units BIGINT NOT NULL CHECK(amount_units=500000000),
 asset_type TEXT NOT NULL CHECK(asset_type='RESERVE_API_CREDIT'), policy_version TEXT NOT NULL,
 transaction_id UUID NOT NULL UNIQUE REFERENCES economy.asset_transactions ON DELETE RESTRICT,
 ledger_entry_id UUID NOT NULL UNIQUE REFERENCES economy.wallet_ledger ON DELETE RESTRICT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(claim_id,newapi_user_id,biz_id) REFERENCES rewards.registration_grants(claim_id,newapi_user_id,biz_id) ON DELETE RESTRICT
);
CREATE TRIGGER registration_issuances_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
 ON rewards.registration_issuances FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TRIGGER registration_grants_no_remove BEFORE DELETE OR TRUNCATE
 ON rewards.registration_grants FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE FUNCTION rewards.guard_registration_claim() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.status='CONFIRMED' OR (NEW.claim_id,NEW.newapi_user_id,NEW.claim_kind,NEW.source_ordinal,NEW.biz_id,NEW.created_at)
 IS DISTINCT FROM (OLD.claim_id,OLD.newapi_user_id,OLD.claim_kind,OLD.source_ordinal,OLD.biz_id,OLD.created_at)
 OR (OLD.status='RECOVERING' AND NEW.status='PENDING') THEN
  RAISE EXCEPTION 'immutable registration claim authority' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER registration_claim_guard BEFORE UPDATE ON rewards.registration_grants
 FOR EACH ROW EXECUTE FUNCTION rewards.guard_registration_claim();

-- Ledger -> issuance -> CONFIRMED may be ordered within a transaction, but a
-- partial or mismatched issuance/confirmation cannot commit.
CREATE FUNCTION rewards.check_registration_issuance() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE g rewards.registration_grants%ROWTYPE; i rewards.registration_issuances%ROWTYPE;
BEGIN
 SELECT * INTO g FROM rewards.registration_grants WHERE claim_id=NEW.claim_id;
 SELECT * INTO i FROM rewards.registration_issuances WHERE claim_id=NEW.claim_id;
 IF g.status='CONFIRMED' THEN
  IF i.claim_id IS NULL OR i.transaction_id<>g.transaction_id OR NOT EXISTS (
   SELECT 1 FROM economy.wallet_ledger l JOIN economy.asset_transactions t USING(transaction_id)
    JOIN identity.native_registration_inbox n ON n.ordinal=g.source_ordinal
   WHERE l.ledger_entry_id=i.ledger_entry_id AND l.transaction_id=i.transaction_id
    AND l.newapi_user_id=g.newapi_user_id AND t.newapi_user_id=g.newapi_user_id
    AND l.biz_type='INITIAL_GRANT_REGISTRATION' AND t.biz_type=l.biz_type
    AND l.biz_id=g.biz_id AND t.biz_id=g.biz_id AND l.leg_no=1
    AND l.entry_type='INITIAL_GRANT_REGISTRATION' AND t.operation_type=l.entry_type
    AND l.asset_type=i.asset_type AND l.delta_units=i.amount_units
    AND t.status='CONFIRMED' AND n.policy_version=i.policy_version
  ) THEN RAISE EXCEPTION 'incomplete registration issuance' USING ERRCODE='23514'; END IF;
 ELSIF i.claim_id IS NOT NULL THEN
  RAISE EXCEPTION 'unconfirmed registration issuance' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER registration_confirmation_complete AFTER INSERT OR UPDATE ON rewards.registration_grants
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION rewards.check_registration_issuance();
CREATE CONSTRAINT TRIGGER registration_issuance_confirmed AFTER INSERT ON rewards.registration_issuances
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION rewards.check_registration_issuance();
