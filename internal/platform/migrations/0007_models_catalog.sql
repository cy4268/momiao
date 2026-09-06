-- Stable textual native identities; editorial publication never follows sync.
CREATE SCHEMA catalog;
CREATE TABLE catalog.model_catalog_metadata (
 model_id TEXT PRIMARY KEY CHECK(octet_length(model_id) BETWEEN 1 AND 255 AND btrim(model_id)=model_id),
 display_name TEXT NOT NULL DEFAULT '' CHECK(char_length(display_name)<=120),
 family TEXT NOT NULL DEFAULT '' CHECK(char_length(family)<=64),
 summary TEXT CHECK(char_length(summary)<=2000),
 context_length BIGINT CHECK(context_length>0 AND context_length<=9007199254740991),
 metadata JSONB NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(metadata)='object' AND octet_length(metadata::text)<=16384),
 metadata_version BIGINT NOT NULL DEFAULT 1 CHECK(metadata_version>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE catalog.model_catalog_publication (
 model_id TEXT PRIMARY KEY REFERENCES catalog.model_catalog_metadata ON DELETE RESTRICT,
 publication_state TEXT NOT NULL DEFAULT 'PENDING_METADATA'
  CHECK(publication_state IN ('PENDING_METADATA','PUBLISHED','HIDDEN','RETIRED')),
 recommended BOOLEAN NOT NULL DEFAULT false,
 sort_order INTEGER NOT NULL DEFAULT 0 CHECK(sort_order BETWEEN 0 AND 1000000),
 published_at TIMESTAMPTZ, retired_at TIMESTAMPTZ,
 version BIGINT NOT NULL DEFAULT 1 CHECK(version>0), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(publication_state<>'PUBLISHED' OR published_at IS NOT NULL),
 CHECK((publication_state='RETIRED')=(retired_at IS NOT NULL))
);
CREATE TABLE catalog.model_sync_snapshots (
 sync_snapshot_id UUID PRIMARY KEY,
 source_identity TEXT NOT NULL CHECK(source_identity='momiao.native-catalog.v1'),
 source_hash BYTEA NOT NULL UNIQUE CHECK(octet_length(source_hash)=32),
 observed_model_count INTEGER NOT NULL CHECK(observed_model_count BETWEEN 0 AND 1000),
 status TEXT NOT NULL CHECK(status='VERIFIED'),
 safe_summary JSONB NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(safe_summary)='object' AND octet_length(safe_summary::text)<=4096),
 observed_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 source_models JSONB NOT NULL CHECK(jsonb_typeof(source_models)='array'
  AND jsonb_array_length(source_models)=observed_model_count AND octet_length(source_models::text)<=4194304)
);
-- Every attempt is immutable, including repeated hashes. Failed reads contain no
-- source observation or fabricated snapshot. The singleton tracks last-good only.
CREATE TABLE catalog.model_sync_attempts (
 attempt_id UUID PRIMARY KEY,
 trigger_kind TEXT NOT NULL CHECK(trigger_kind IN ('SCHEDULED','OPS')),
 status TEXT NOT NULL CHECK(status IN ('VERIFIED','FAILED')),
 source_snapshot_id UUID REFERENCES catalog.model_sync_snapshots ON DELETE RESTRICT,
 failure_code TEXT CHECK(failure_code IN ('CATALOG_READ_FAILED','CATALOG_SOURCE_INVALID')),
 observed_at TIMESTAMPTZ, verified_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK((status='VERIFIED')=(source_snapshot_id IS NOT NULL AND observed_at IS NOT NULL AND verified_at IS NOT NULL)),
 CHECK((status='FAILED')=(failure_code IS NOT NULL)),
 CHECK(status<>'FAILED' OR (source_snapshot_id IS NULL AND observed_at IS NULL AND verified_at IS NULL))
);
CREATE TABLE catalog.model_sync_state (
 singleton BOOLEAN PRIMARY KEY CHECK(singleton),
 source_snapshot_id UUID REFERENCES catalog.model_sync_snapshots ON DELETE RESTRICT,
 last_attempt_id UUID REFERENCES catalog.model_sync_attempts ON DELETE RESTRICT,
 last_verified_at TIMESTAMPTZ, last_observed_at TIMESTAMPTZ,
 version BIGINT NOT NULL DEFAULT 0 CHECK(version>=0),
 CHECK((source_snapshot_id IS NULL)=(last_verified_at IS NULL)),
 CHECK((last_verified_at IS NULL)=(last_observed_at IS NULL))
);
INSERT INTO catalog.model_sync_state(singleton) VALUES(true);
CREATE TABLE catalog.model_availability_mappings (
 model_id TEXT PRIMARY KEY REFERENCES catalog.model_catalog_metadata ON DELETE RESTRICT,
 availability_state TEXT NOT NULL CHECK(availability_state IN ('CONFIGURED','NATIVE_HIDDEN','NOT_OBSERVED')),
 source_snapshot_id UUID NOT NULL REFERENCES catalog.model_sync_snapshots ON DELETE RESTRICT,
 observed_at TIMESTAMPTZ NOT NULL, last_seen_at TIMESTAMPTZ NOT NULL,
 source_facts JSONB NOT NULL CHECK(jsonb_typeof(source_facts)='object' AND octet_length(source_facts::text)<=65536)
);
CREATE TABLE catalog.historical_model_identity (
 historical_identity_id UUID PRIMARY KEY,
 model_id TEXT NOT NULL REFERENCES catalog.model_catalog_metadata ON DELETE RESTRICT,
 display_name_snapshot TEXT NOT NULL CHECK(char_length(display_name_snapshot)<=255),
 family_snapshot TEXT NOT NULL CHECK(char_length(family_snapshot)<=64),
 effective_from TIMESTAMPTZ NOT NULL, effective_until TIMESTAMPTZ,
 CHECK(effective_until IS NULL OR effective_until>=effective_from)
);
CREATE UNIQUE INDEX model_current_identity ON catalog.historical_model_identity(model_id) WHERE effective_until IS NULL;
CREATE TABLE catalog.model_metadata_revisions (
 model_id TEXT NOT NULL REFERENCES catalog.model_catalog_metadata ON DELETE RESTRICT,
 metadata_version BIGINT NOT NULL CHECK(metadata_version>0),
 content JSONB NOT NULL CHECK(jsonb_typeof(content)='object' AND octet_length(content::text)<=32768),
 created_by BIGINT CHECK(created_by>0), created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(model_id,metadata_version)
);
CREATE FUNCTION catalog.reject_history_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'immutable catalog history' USING ERRCODE='55000'; END $$;
CREATE FUNCTION catalog.guard_identity_interval() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.effective_until IS NOT NULL OR NEW.effective_until IS NULL OR
 (NEW.historical_identity_id,NEW.model_id,NEW.display_name_snapshot,NEW.family_snapshot,NEW.effective_from)
 IS DISTINCT FROM (OLD.historical_identity_id,OLD.model_id,OLD.display_name_snapshot,OLD.family_snapshot,OLD.effective_from) THEN
  RAISE EXCEPTION 'immutable model identity interval' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION catalog.guard_model_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.model_id,NEW.created_at) IS DISTINCT FROM (OLD.model_id,OLD.created_at) THEN
  RAISE EXCEPTION 'immutable textual model identity' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER catalog_identity_guard BEFORE UPDATE ON catalog.model_catalog_metadata
 FOR EACH ROW EXECUTE FUNCTION catalog.guard_model_identity();
CREATE TRIGGER catalog_identity_interval_guard BEFORE UPDATE ON catalog.historical_model_identity
 FOR EACH ROW EXECUTE FUNCTION catalog.guard_identity_interval();
CREATE TRIGGER catalog_identity_no_remove BEFORE DELETE OR TRUNCATE ON catalog.historical_model_identity
 FOR EACH STATEMENT EXECUTE FUNCTION catalog.reject_history_change();
CREATE TRIGGER catalog_metadata_no_remove BEFORE DELETE OR TRUNCATE ON catalog.model_catalog_metadata
 FOR EACH STATEMENT EXECUTE FUNCTION catalog.reject_history_change();
CREATE TRIGGER catalog_publication_no_remove BEFORE DELETE OR TRUNCATE ON catalog.model_catalog_publication
 FOR EACH STATEMENT EXECUTE FUNCTION catalog.reject_history_change();
CREATE TRIGGER catalog_availability_no_remove BEFORE DELETE OR TRUNCATE ON catalog.model_availability_mappings
 FOR EACH STATEMENT EXECUTE FUNCTION catalog.reject_history_change();
CREATE TRIGGER catalog_state_no_remove BEFORE DELETE OR TRUNCATE ON catalog.model_sync_state
 FOR EACH STATEMENT EXECUTE FUNCTION catalog.reject_history_change();
CREATE TRIGGER catalog_snapshots_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON catalog.model_sync_snapshots
 FOR EACH STATEMENT EXECUTE FUNCTION catalog.reject_history_change();
CREATE TRIGGER catalog_attempts_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON catalog.model_sync_attempts
 FOR EACH STATEMENT EXECUTE FUNCTION catalog.reject_history_change();
CREATE TRIGGER catalog_revisions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON catalog.model_metadata_revisions
 FOR EACH STATEMENT EXECUTE FUNCTION catalog.reject_history_change();

-- Extend the domain vocabulary here; frozen announcements SQL remains unchanged.
ALTER TABLE ops.admin_principal_scopes DROP CONSTRAINT admin_principal_scopes_scope_check;
ALTER TABLE ops.admin_principal_scopes ADD CONSTRAINT admin_principal_scopes_scope_check CHECK(scope IN ('ANNOUNCEMENTS','MODELS'));
CREATE TABLE ops.model_previews (
 preview_id UUID PRIMARY KEY, newapi_user_id BIGINT NOT NULL CHECK(newapi_user_id>0),
 authz_epoch BIGINT NOT NULL CHECK(authz_epoch>0), command_hash TEXT NOT NULL CHECK(length(command_hash)=64),
 source_hash TEXT CHECK(source_hash ~ '^sha256:[0-9a-f]{64}$'),
 impact JSONB NOT NULL CHECK(jsonb_typeof(impact)='object' AND octet_length(impact::text)<=65536),
 expires_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER model_previews_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON ops.model_previews
 FOR EACH STATEMENT EXECUTE FUNCTION catalog.reject_history_change();
ALTER TABLE ops.admin_operations ADD COLUMN model_id TEXT REFERENCES catalog.model_catalog_metadata ON DELETE RESTRICT;
ALTER TABLE ops.admin_operations ADD CONSTRAINT admin_operation_single_domain CHECK(announcement_id IS NULL OR model_id IS NULL);
REVOKE ALL ON SCHEMA catalog FROM PUBLIC;
REVOKE ALL ON ALL TABLES IN SCHEMA catalog FROM PUBLIC;
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA catalog FROM PUBLIC;
REVOKE ALL ON ops.model_previews FROM PUBLIC;
