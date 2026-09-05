-- Add two-leg local exchanges without altering existing immutable entries.
ALTER TABLE economy.wallet_ledger DROP CONSTRAINT wallet_ledger_leg_no_check;
ALTER TABLE economy.wallet_ledger ADD CHECK (leg_no IN (1,2));
ALTER TABLE platform_meta.mutation_idempotency_records DROP CONSTRAINT mutation_idempotency_records_scope_check;
ALTER TABLE platform_meta.mutation_idempotency_records ADD CHECK (scope IN ('wallet.apply.v1','wallet.exchange.v1'));

CREATE SCHEMA rewards;
-- One row is the daily claim, versioned policy snapshot and issuance receipt.
-- The referenced transaction/ledger is committed in the same local transaction.
CREATE TABLE rewards.daily_checkins (
    newapi_user_id BIGINT NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
    checkin_date DATE NOT NULL,
    policy_version INTEGER NOT NULL CHECK (policy_version = 1),
    amount_units BIGINT NOT NULL CHECK (amount_units = 250000000),
    asset_type TEXT NOT NULL CHECK (asset_type = 'RESERVE_API_CREDIT'),
    transaction_id UUID NOT NULL UNIQUE REFERENCES economy.asset_transactions ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (newapi_user_id, checkin_date)
);
CREATE TRIGGER daily_checkins_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
    ON rewards.daily_checkins FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE INDEX asset_transactions_user_id_desc ON economy.asset_transactions(newapi_user_id,transaction_id DESC);
