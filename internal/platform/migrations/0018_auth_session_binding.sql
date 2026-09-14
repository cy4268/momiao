-- Native remains the login/password authority. This stores only the platform
-- epoch snapshot of an online-verified native authentication chain.
ALTER TABLE identity.account_refs ADD COLUMN security_epoch_changed_at TIMESTAMPTZ;
UPDATE identity.account_refs SET security_epoch_changed_at=clock_timestamp() WHERE security_epoch>0;

CREATE FUNCTION identity.guard_security_epoch() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.security_epoch < OLD.security_epoch
       OR NEW.security_epoch_changed_at IS DISTINCT FROM OLD.security_epoch_changed_at THEN
        RAISE EXCEPTION 'invalid security epoch change' USING ERRCODE='23514';
    END IF;
    IF NEW.security_epoch > OLD.security_epoch THEN
        NEW.security_epoch_changed_at=clock_timestamp();
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER account_security_epoch_guard BEFORE UPDATE ON identity.account_refs
    FOR EACH ROW EXECUTE FUNCTION identity.guard_security_epoch();

CREATE TABLE identity.native_session_bindings (
    session_id_hash TEXT PRIMARY KEY CHECK(session_id_hash ~ '^[0-9a-f]{64}$'),
    newapi_user_id BIGINT NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
    session_version BIGINT NOT NULL CHECK(session_version BETWEEN 1 AND 9007199254740991),
    native_auth_version BIGINT NOT NULL CHECK(native_auth_version BETWEEN 1 AND 9007199254740991),
    security_epoch_snapshot BIGINT NOT NULL CHECK(security_epoch_snapshot BETWEEN 0 AND 9007199254740991),
    native_created_at TIMESTAMPTZ NOT NULL,
    bound_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX native_session_bindings_user ON identity.native_session_bindings(newapi_user_id);
CREATE FUNCTION identity.guard_native_session_binding() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE current_epoch BIGINT; changed_at TIMESTAMPTZ;
BEGIN
    IF TG_OP='UPDATE' AND (
        NEW.session_id_hash IS DISTINCT FROM OLD.session_id_hash OR
        NEW.newapi_user_id IS DISTINCT FROM OLD.newapi_user_id OR
        NEW.security_epoch_snapshot IS DISTINCT FROM OLD.security_epoch_snapshot OR
        NEW.native_created_at IS DISTINCT FROM OLD.native_created_at OR
        NEW.bound_at IS DISTINCT FROM OLD.bound_at OR
        NEW.session_version < OLD.session_version OR
        NEW.native_auth_version < OLD.native_auth_version) THEN
        RAISE EXCEPTION 'immutable authentication chain' USING ERRCODE='23514';
    END IF;
    SELECT security_epoch,security_epoch_changed_at INTO current_epoch,changed_at
        FROM identity.account_refs WHERE newapi_user_id=NEW.newapi_user_id;
    IF NOT FOUND OR current_epoch<>NEW.security_epoch_snapshot OR
       (TG_OP='INSERT' AND current_epoch>0 AND
        (changed_at IS NULL OR NEW.native_created_at<=changed_at)) THEN
        RAISE EXCEPTION 'revoked authentication chain' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER native_session_binding_guard BEFORE INSERT OR UPDATE ON identity.native_session_bindings
    FOR EACH ROW EXECUTE FUNCTION identity.guard_native_session_binding();
CREATE TRIGGER native_session_binding_no_delete BEFORE DELETE OR TRUNCATE ON identity.native_session_bindings
    FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
