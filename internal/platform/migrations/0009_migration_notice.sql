-- Migration notices acknowledge completed facts only, never perform a cutover.
-- Publication/target assignment belongs to controlled deployment authority;
-- ordinary runtime receives SELECT and acknowledgement INSERT only.
CREATE TABLE identity.migration_notice_versions (
 version BIGINT PRIMARY KEY CHECK(version>0),
 title TEXT NOT NULL CHECK(char_length(title) BETWEEN 1 AND 160),
 body TEXT NOT NULL CHECK(octet_length(body) BETWEEN 1 AND 16384),
 completed_at TIMESTAMPTZ NOT NULL,
 evidence_ref TEXT NOT NULL CHECK(octet_length(evidence_ref) BETWEEN 1 AND 256),
 published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(completed_at<=published_at)
);
CREATE TABLE identity.migration_notice_requirements (
 newapi_user_id BIGINT NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
 version BIGINT NOT NULL REFERENCES identity.migration_notice_versions ON DELETE RESTRICT,
 assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(newapi_user_id,version)
);
CREATE TABLE identity.migration_notice_acknowledgements (
 newapi_user_id BIGINT NOT NULL,
 version BIGINT NOT NULL,
 acknowledged_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(newapi_user_id,version),
 FOREIGN KEY(newapi_user_id,version) REFERENCES identity.migration_notice_requirements ON DELETE RESTRICT
);
CREATE TRIGGER migration_notice_versions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON identity.migration_notice_versions
 FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TRIGGER migration_notice_requirements_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON identity.migration_notice_requirements
 FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TRIGGER migration_notice_acknowledgements_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON identity.migration_notice_acknowledgements
 FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
REVOKE ALL ON identity.migration_notice_versions,identity.migration_notice_requirements,identity.migration_notice_acknowledgements FROM PUBLIC;
