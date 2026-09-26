CREATE TABLE catalog.family_cover_assets (
 asset_id UUID PRIMARY KEY,
 family TEXT NOT NULL CHECK(family IN ('deepseek','gpt','claude','gemini','glm','kimi','ernie','qwen','grok','other')),
 sha256 TEXT NOT NULL CHECK(sha256 ~ '^[0-9a-f]{64}$'),
 object_key TEXT NOT NULL UNIQUE,
 content_type TEXT NOT NULL CHECK(content_type IN ('image/png','image/jpeg','image/webp')),
 size_bytes BIGINT NOT NULL CHECK(size_bytes BETWEEN 1 AND 8388608),
 width INTEGER NOT NULL CHECK(width BETWEEN 1 AND 8192),
 height INTEGER NOT NULL CHECK(height BETWEEN 1 AND 8192),
 alt TEXT NOT NULL CHECK(length(alt) BETWEEN 1 AND 120),
 rights_status TEXT NOT NULL CHECK(rights_status IN ('ORIGINAL_PLATFORM','ORIGINAL_GENERATED','LICENSED_OR_APPROVED')),
 rights_note TEXT NOT NULL CHECK(length(rights_note) BETWEEN 1 AND 500),
 created_by BIGINT NOT NULL CHECK(created_by>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(family,sha256), UNIQUE(family,asset_id),
 CHECK(width::bigint*height<=16777216),
 CHECK(object_key='assets/models/uploads/'||family||'/'||sha256||CASE content_type WHEN 'image/png' THEN '.png' WHEN 'image/jpeg' THEN '.jpg' ELSE '.webp' END)
);
CREATE TABLE catalog.family_covers (
 family TEXT PRIMARY KEY CHECK(family IN ('deepseek','gpt','claude','gemini','glm','kimi','ernie','qwen','grok','other')),
 asset_id UUID,
 version BIGINT NOT NULL DEFAULT 1 CHECK(version>0),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(family,asset_id) REFERENCES catalog.family_cover_assets(family,asset_id) ON DELETE RESTRICT
);
INSERT INTO catalog.family_covers(family) VALUES ('deepseek'),('gpt'),('claude'),('gemini'),('glm'),('kimi'),('ernie'),('qwen'),('grok'),('other');
CREATE TRIGGER family_cover_assets_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON catalog.family_cover_assets
 FOR EACH STATEMENT EXECUTE FUNCTION catalog.reject_history_change();
CREATE TRIGGER family_covers_no_remove BEFORE DELETE OR TRUNCATE ON catalog.family_covers
 FOR EACH STATEMENT EXECUTE FUNCTION catalog.reject_history_change();
REVOKE ALL ON catalog.family_cover_assets,catalog.family_covers FROM PUBLIC;
