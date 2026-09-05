CREATE SCHEMA identity;
CREATE SCHEMA economy;

-- Verified external identity type: NewAPI users.id is BIGINT. This is an
-- external identity anchor, not a copy of NewAPI authentication/profile data.
CREATE TABLE identity.account_refs (
    newapi_user_id BIGINT PRIMARY KEY CHECK (newapi_user_id > 0),
    security_epoch BIGINT NOT NULL DEFAULT 0 CHECK (security_epoch >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    migration_batch_id UUID
);
CREATE TABLE economy.wallet_balances (
    newapi_user_id BIGINT NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
    asset_type TEXT NOT NULL CHECK (asset_type IN ('RESERVE_API_CREDIT','AVAILABLE_CHIPS')),
    balance_units BIGINT NOT NULL DEFAULT 0 CHECK (balance_units >= 0),
    ledger_seq BIGINT NOT NULL DEFAULT 0 CHECK (ledger_seq >= 0),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (newapi_user_id, asset_type)
);
CREATE TABLE economy.asset_transactions (
    transaction_id UUID PRIMARY KEY,
    biz_type TEXT NOT NULL,
    biz_id TEXT NOT NULL,
    newapi_user_id BIGINT NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
    operation_type TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status = 'CONFIRMED'),
    request_hash BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    confirmed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (biz_type, biz_id),
    UNIQUE (transaction_id, newapi_user_id, biz_type, biz_id)
);
CREATE TABLE economy.wallet_ledger (
    ledger_entry_id UUID PRIMARY KEY,
    transaction_id UUID NOT NULL,
    leg_no INTEGER NOT NULL CHECK (leg_no = 1),
    newapi_user_id BIGINT NOT NULL,
    asset_type TEXT NOT NULL,
    ledger_seq BIGINT NOT NULL CHECK (ledger_seq > 0),
    wallet_version BIGINT NOT NULL CHECK (wallet_version > 1),
    entry_type TEXT NOT NULL,
    biz_type TEXT NOT NULL,
    biz_id TEXT NOT NULL,
    delta_units BIGINT NOT NULL CHECK (delta_units <> 0),
    balance_before_units BIGINT NOT NULL CHECK (balance_before_units >= 0),
    balance_after_units BIGINT NOT NULL CHECK (balance_after_units >= 0),
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (balance_after_units::numeric = balance_before_units::numeric + delta_units::numeric),
    FOREIGN KEY (newapi_user_id, asset_type) REFERENCES economy.wallet_balances ON DELETE RESTRICT,
    FOREIGN KEY (transaction_id, newapi_user_id, biz_type, biz_id)
        REFERENCES economy.asset_transactions (transaction_id, newapi_user_id, biz_type, biz_id) ON DELETE RESTRICT,
    UNIQUE (newapi_user_id, asset_type, ledger_seq),
    UNIQUE (transaction_id, leg_no),
    UNIQUE (ledger_entry_id, newapi_user_id)
);
CREATE TABLE platform_meta.mutation_idempotency_records (
    idempotency_record_id UUID PRIMARY KEY,
    newapi_user_id BIGINT NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
    scope TEXT NOT NULL CHECK (scope = 'wallet.apply.v1'),
    key_hash BYTEA NOT NULL CHECK (octet_length(key_hash) = 32),
    request_hash BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    resource_type TEXT NOT NULL CHECK (resource_type = 'wallet_ledger'),
    resource_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (resource_id, newapi_user_id) REFERENCES economy.wallet_ledger (ledger_entry_id, newapi_user_id) ON DELETE RESTRICT,
    UNIQUE (newapi_user_id, scope, key_hash)
);
CREATE FUNCTION economy.reject_history_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'immutable history: append new entries instead' USING ERRCODE = '55000';
END;
$$;
CREATE TRIGGER wallet_ledger_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
    ON economy.wallet_ledger FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TRIGGER asset_transactions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
    ON economy.asset_transactions FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TRIGGER mutation_idempotency_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
    ON platform_meta.mutation_idempotency_records FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
