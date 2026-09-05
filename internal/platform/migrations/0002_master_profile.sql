CREATE TABLE identity.master_profiles (
    newapi_user_id BIGINT PRIMARY KEY REFERENCES identity.account_refs ON DELETE RESTRICT,
    display_name TEXT NOT NULL CHECK (octet_length(display_name) BETWEEN 1 AND 4096),
    normalized_name TEXT NOT NULL CHECK (octet_length(normalized_name) BETWEEN 1 AND 8192),
    avatar_id TEXT NOT NULL DEFAULT 'system-default' CHECK (avatar_id = 'system-default'),
    profile_version BIGINT NOT NULL DEFAULT 1 CHECK (profile_version > 0),
    nickname_changed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT master_profiles_normalized_name_key UNIQUE (normalized_name)
);
CREATE TABLE identity.master_profile_name_history (
    newapi_user_id BIGINT NOT NULL REFERENCES identity.master_profiles ON DELETE RESTRICT,
    profile_version BIGINT NOT NULL CHECK (profile_version > 0),
    display_name TEXT NOT NULL,
    normalized_name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (newapi_user_id, profile_version)
);
CREATE TRIGGER master_profile_name_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
    ON identity.master_profile_name_history FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
