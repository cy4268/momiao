-- Local mapping of IS content/announcement + minimum announcement-domain Ops authority.
CREATE SCHEMA content;
CREATE SCHEMA ops;

CREATE TABLE ops.admin_principals (
 admin_principal_id UUID PRIMARY KEY,
 newapi_user_id BIGINT NOT NULL UNIQUE CHECK(newapi_user_id>0),
 base_role TEXT NOT NULL CHECK(base_role IN ('SUPER_ADMIN','OPERATOR','AUDITOR')),
 status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK(status IN ('ACTIVE','DISABLED')),
 authz_epoch BIGINT NOT NULL DEFAULT 1 CHECK(authz_epoch>0),
 version BIGINT NOT NULL DEFAULT 1 CHECK(version>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE ops.admin_principal_scopes (
 admin_principal_id UUID NOT NULL REFERENCES ops.admin_principals ON DELETE RESTRICT,
 scope TEXT NOT NULL CHECK(scope='ANNOUNCEMENTS'),
 PRIMARY KEY(admin_principal_id,scope)
);
CREATE FUNCTION ops.bump_principal_authority() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.newapi_user_id,NEW.admin_principal_id,NEW.created_at) IS DISTINCT FROM
    (OLD.newapi_user_id,OLD.admin_principal_id,OLD.created_at) THEN
  RAISE EXCEPTION 'immutable principal identity' USING ERRCODE='55000';
 END IF;
 NEW.authz_epoch:=OLD.authz_epoch+1; NEW.version:=OLD.version+1; NEW.updated_at:=now();
 RETURN NEW;
END $$;
CREATE TRIGGER principal_authority BEFORE UPDATE ON ops.admin_principals
 FOR EACH ROW EXECUTE FUNCTION ops.bump_principal_authority();
CREATE FUNCTION ops.bump_scope_authority() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' THEN RAISE EXCEPTION 'replace scopes explicitly' USING ERRCODE='55000'; END IF;
 UPDATE ops.admin_principals SET updated_at=now()
 WHERE admin_principal_id=CASE WHEN TG_OP='DELETE' THEN OLD.admin_principal_id ELSE NEW.admin_principal_id END;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER scope_authority BEFORE INSERT OR UPDATE OR DELETE ON ops.admin_principal_scopes
 FOR EACH ROW EXECUTE FUNCTION ops.bump_scope_authority();

CREATE TABLE content.announcements (
 announcement_id UUID PRIMARY KEY,
 canonical_key TEXT UNIQUE CHECK(canonical_key='ACKNOWLEDGEMENTS'),
 current_content_version BIGINT NOT NULL DEFAULT 1 CHECK(current_content_version>0),
 notification_revision BIGINT NOT NULL DEFAULT 1 CHECK(notification_revision>0),
 version BIGINT NOT NULL DEFAULT 1 CHECK(version>0),
 state TEXT NOT NULL DEFAULT 'DRAFT' CHECK(state IN ('DRAFT','SCHEDULED','PUBLISHED','EXPIRED','ARCHIVED')),
 publish_at TIMESTAMPTZ, visible_from TIMESTAMPTZ, visible_until TIMESTAMPTZ,
 first_published_at TIMESTAMPTZ, withdrawn_at TIMESTAMPTZ,
 expired_reason TEXT NOT NULL DEFAULT '' CHECK(expired_reason IN ('','VISIBLE_WINDOW_ENDED','MISSED_PUBLISH_WINDOW')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(visible_until IS NULL OR visible_until>visible_from),
 CHECK(state='DRAFT' OR (publish_at IS NOT NULL AND visible_from IS NOT NULL)),
 CHECK(visible_from IS NULL OR publish_at<=visible_from)
);
CREATE TABLE content.announcement_revisions (
 announcement_id UUID NOT NULL REFERENCES content.announcements ON DELETE RESTRICT,
 content_version BIGINT NOT NULL CHECK(content_version>0),
 title TEXT NOT NULL CHECK(char_length(title) BETWEEN 1 AND 160),
 type TEXT NOT NULL CHECK(type IN ('SYSTEM','NEW_MODELS','GAME_EVENTS','MAINTENANCE','IMPORTANT','ACKNOWLEDGEMENTS')),
 visibility TEXT NOT NULL CHECK(visibility IN ('PUBLIC','AUTHENTICATED')),
 body_markdown TEXT NOT NULL CHECK(octet_length(body_markdown) BETWEEN 1 AND 32768),
 sanitized_html TEXT NOT NULL,
 body_markdown_hash TEXT NOT NULL CHECK(length(body_markdown_hash)=64),
 sanitized_html_hash TEXT NOT NULL CHECK(length(sanitized_html_hash)=64),
 sanitizer_policy_version TEXT NOT NULL CHECK(sanitizer_policy_version='announcement-sanitize-v1'),
 created_by BIGINT NOT NULL CHECK(created_by>0), created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(announcement_id,content_version)
);
CREATE TABLE content.notification_revisions (
 announcement_id UUID NOT NULL REFERENCES content.announcements ON DELETE RESTRICT,
 notification_revision BIGINT NOT NULL CHECK(notification_revision>0),
 created_by BIGINT NOT NULL CHECK(created_by>0), reason TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(announcement_id,notification_revision)
);
ALTER TABLE content.announcements ADD CONSTRAINT current_content_fk
 FOREIGN KEY(announcement_id,current_content_version) REFERENCES content.announcement_revisions
 DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE content.announcements ADD CONSTRAINT current_notification_fk
 FOREIGN KEY(announcement_id,notification_revision) REFERENCES content.notification_revisions
 DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE content.announcement_placements (
 announcement_id UUID NOT NULL REFERENCES content.announcements ON DELETE RESTRICT,
 placement TEXT NOT NULL CHECK(placement IN ('PINNED_LIST','ENTRY_POPUP','POST_LOGIN_POPUP','PUBLIC_HOME_BANNER','DASHBOARD_SUMMARY')),
 manual_order INTEGER NOT NULL DEFAULT 0 CHECK(manual_order BETWEEN 0 AND 1000000),
 PRIMARY KEY(announcement_id,placement)
);
CREATE TABLE content.placement_guards (
 guard_key TEXT PRIMARY KEY CHECK(guard_key IN ('ENTRY_POPUP','PRIMARY_HOME_BANNER'))
);
INSERT INTO content.placement_guards VALUES('ENTRY_POPUP'),('PRIMARY_HOME_BANNER');
CREATE TABLE content.announcement_reads (
 newapi_user_id BIGINT NOT NULL CHECK(newapi_user_id>0), announcement_id UUID NOT NULL,
 notification_revision BIGINT NOT NULL, read_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(newapi_user_id,announcement_id,notification_revision),
 FOREIGN KEY(announcement_id,notification_revision) REFERENCES content.notification_revisions ON DELETE RESTRICT
);
CREATE TABLE content.announcement_media_assets (
 media_id UUID PRIMARY KEY, managed_path TEXT NOT NULL UNIQUE,
 sha256 TEXT NOT NULL CHECK(length(sha256)=64), mime_type TEXT NOT NULL,
 verified_at TIMESTAMPTZ NOT NULL, consent_attested_by BIGINT NOT NULL CHECK(consent_attested_by>0)
);
CREATE TABLE content.acknowledgement_entries (
 announcement_id UUID NOT NULL, content_version BIGINT NOT NULL,
 manual_order INTEGER NOT NULL CHECK(manual_order BETWEEN 0 AND 1000000),
 display_name TEXT NOT NULL CHECK(char_length(display_name) BETWEEN 1 AND 120),
 avatar_or_logo_media_id UUID REFERENCES content.announcement_media_assets ON DELETE RESTRICT,
 external_link TEXT NOT NULL DEFAULT '', acknowledgement_note TEXT NOT NULL DEFAULT '',
 group_name TEXT NOT NULL DEFAULT '', anonymous BOOLEAN NOT NULL DEFAULT false,
 consent_attested BOOLEAN NOT NULL CHECK(consent_attested),
 PRIMARY KEY(announcement_id,content_version,manual_order),
 FOREIGN KEY(announcement_id,content_version) REFERENCES content.announcement_revisions ON DELETE RESTRICT
);
CREATE TABLE content.announcement_jobs (
 job_key TEXT PRIMARY KEY, announcement_id UUID NOT NULL REFERENCES content.announcements ON DELETE RESTRICT,
 kind TEXT NOT NULL CHECK(kind IN ('PUBLISH','EXPIRE')),
 content_version BIGINT NOT NULL, notification_revision BIGINT NOT NULL,
 due_at TIMESTAMPTZ NOT NULL, status TEXT NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','DONE','OBSOLETE')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), finished_at TIMESTAMPTZ
);
CREATE INDEX announcement_jobs_due ON content.announcement_jobs(due_at,job_key) WHERE status='PENDING';
CREATE INDEX announcement_visibility ON content.announcements(state,visible_from,visible_until) WHERE withdrawn_at IS NULL;
CREATE TABLE ops.announcement_previews (
 preview_id UUID PRIMARY KEY, newapi_user_id BIGINT NOT NULL CHECK(newapi_user_id>0),
 authz_epoch BIGINT NOT NULL, command_hash TEXT NOT NULL CHECK(length(command_hash)=64),
 impact JSONB NOT NULL, expires_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE ops.admin_operations (
 operation_id UUID PRIMARY KEY, actor_kind TEXT NOT NULL CHECK(actor_kind IN ('ADMIN','SYSTEM','OFFLINE')),
 newapi_user_id BIGINT, action TEXT NOT NULL, announcement_id UUID REFERENCES content.announcements ON DELETE RESTRICT,
 request_hash TEXT NOT NULL CHECK(length(request_hash)=64),
 details JSONB NOT NULL, result JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE FUNCTION content.reject_history_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'append-only announcement history' USING ERRCODE='55000'; END $$;
CREATE TRIGGER revisions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON content.announcement_revisions
 FOR EACH STATEMENT EXECUTE FUNCTION content.reject_history_change();
CREATE TRIGGER notifications_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON content.notification_revisions
 FOR EACH STATEMENT EXECUTE FUNCTION content.reject_history_change();
CREATE TRIGGER acknowledgements_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON content.acknowledgement_entries
 FOR EACH STATEMENT EXECUTE FUNCTION content.reject_history_change();
CREATE TRIGGER reads_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON content.announcement_reads
 FOR EACH STATEMENT EXECUTE FUNCTION content.reject_history_change();
CREATE TRIGGER operations_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON ops.admin_operations
 FOR EACH STATEMENT EXECUTE FUNCTION content.reject_history_change();
CREATE TRIGGER previews_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON ops.announcement_previews
 FOR EACH STATEMENT EXECUTE FUNCTION content.reject_history_change();
CREATE TRIGGER guards_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON content.placement_guards
 FOR EACH STATEMENT EXECUTE FUNCTION content.reject_history_change();
REVOKE ALL ON SCHEMA content,ops FROM PUBLIC;
REVOKE ALL ON ALL TABLES IN SCHEMA content,ops FROM PUBLIC;
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA content,ops FROM PUBLIC;
