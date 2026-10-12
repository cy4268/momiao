-- Keep the existing receipt fields and immutable registration history locally.
-- account_refs is the same preserved user-ID anchor used by the grant ledger.
CREATE SCHEMA momiao_admission;
CREATE TABLE momiao_admission.receipt_counter (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    ordinal BIGINT NOT NULL CHECK (ordinal >= 0)
);
CREATE TABLE momiao_admission.registration_receipts (
    ordinal BIGINT PRIMARY KEY CHECK (ordinal > 0),
    operation_id UUID NOT NULL UNIQUE,
    native_user_id BIGINT NOT NULL UNIQUE REFERENCES identity.account_refs(newapi_user_id),
    discord_subject TEXT NOT NULL UNIQUE CHECK (discord_subject <> ''),
    source TEXT NOT NULL CHECK (source = 'NEW_DISCORD_REGISTRATION'),
    policy_version TEXT NOT NULL CHECK (policy_version <> ''),
    created_at TIMESTAMPTZ NOT NULL
);

-- The inbox already holds the verified immutable source facts. Preserve their
-- ordinals so existing claims/cursors continue without issuing another grant.
INSERT INTO momiao_admission.registration_receipts
    SELECT ordinal,operation_id,native_user_id,discord_subject,source,policy_version,native_created_at
    FROM identity.native_registration_inbox;
INSERT INTO momiao_admission.receipt_counter
    SELECT 1,coalesce(max(ordinal),0) FROM momiao_admission.registration_receipts;

CREATE TRIGGER registration_receipts_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
    ON momiao_admission.registration_receipts FOR EACH STATEMENT
    EXECUTE FUNCTION economy.reject_history_change();
REVOKE ALL ON SCHEMA momiao_admission FROM PUBLIC;
REVOKE ALL ON ALL TABLES IN SCHEMA momiao_admission FROM PUBLIC;
