-- Typed Games, Poker and Rankings operations. Older migrations remain intact.

-- Games registry rows gain an explicit optimistic-concurrency version. Config
-- versions gain the frozen Draft -> Validated -> Previewed -> Active lifecycle.
ALTER TABLE games.game_registry ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK(version>0);

ALTER TABLE games.game_config_versions
 ADD COLUMN version_number bigint,
 ADD COLUMN status text,
 ADD COLUMN created_by bigint REFERENCES identity.account_refs(newapi_user_id),
 ADD COLUMN validated_at timestamptz,
 ADD COLUMN previewed_at timestamptz,
 ADD COLUMN activated_at timestamptz,
 ADD COLUMN superseded_at timestamptz;

-- 0010 made the whole table statement-immutable. Remove that trigger before
-- the lifecycle backfill; the stricter row lifecycle plus delete/truncate
-- guard are installed below in this same migration transaction.
DROP TRIGGER immutable_history ON games.game_config_versions;

WITH numbered AS (
 SELECT config_version_id,row_number() OVER(PARTITION BY game_slug ORDER BY created_at,config_version_id) AS value
 FROM games.game_config_versions
)
UPDATE games.game_config_versions c SET version_number=n.value FROM numbered n
 WHERE n.config_version_id=c.config_version_id;
UPDATE games.game_config_versions c
 SET status=CASE WHEN r.active_config_version_id=c.config_version_id THEN 'ACTIVE' ELSE 'SUPERSEDED' END,
     validated_at=c.created_at,previewed_at=c.created_at,activated_at=c.created_at,
     superseded_at=CASE WHEN r.active_config_version_id=c.config_version_id THEN NULL ELSE c.created_at END
 FROM games.game_registry r WHERE r.game_slug=c.game_slug;

ALTER TABLE games.game_config_versions
 ALTER COLUMN version_number SET NOT NULL,
 ALTER COLUMN status SET NOT NULL,
 ADD CONSTRAINT game_config_version_number_unique UNIQUE(game_slug,version_number),
 ADD CONSTRAINT game_config_status_check CHECK(status IN('DRAFT','VALIDATED','PREVIEWED','ACTIVE','SUPERSEDED')),
 ADD CONSTRAINT game_config_lifecycle_shape CHECK(
  (status='DRAFT' AND validated_at IS NULL AND previewed_at IS NULL AND activated_at IS NULL AND superseded_at IS NULL)
  OR (status='VALIDATED' AND validated_at IS NOT NULL AND previewed_at IS NULL AND activated_at IS NULL AND superseded_at IS NULL)
  OR (status='PREVIEWED' AND validated_at IS NOT NULL AND previewed_at IS NOT NULL AND activated_at IS NULL AND superseded_at IS NULL)
  OR (status='ACTIVE' AND validated_at IS NOT NULL AND previewed_at IS NOT NULL AND activated_at IS NOT NULL AND superseded_at IS NULL)
  OR (status='SUPERSEDED' AND validated_at IS NOT NULL AND previewed_at IS NOT NULL AND activated_at IS NOT NULL AND superseded_at IS NOT NULL)
 );
CREATE UNIQUE INDEX games_one_active_config ON games.game_config_versions(game_slug) WHERE status='ACTIVE';

CREATE FUNCTION games.guard_config_version_lifecycle() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.config_version_id<>OLD.config_version_id OR NEW.game_slug<>OLD.game_slug
  OR NEW.parent_version_id IS DISTINCT FROM OLD.parent_version_id OR NEW.version_number<>OLD.version_number
  OR NEW.created_by IS DISTINCT FROM OLD.created_by OR NEW.created_at<>OLD.created_at THEN
  RAISE EXCEPTION 'immutable config identity' USING ERRCODE='55000';
 END IF;
 IF OLD.status='DRAFT' AND NEW.status='DRAFT' THEN
  IF NEW.validated_at IS NOT NULL OR NEW.previewed_at IS NOT NULL OR NEW.activated_at IS NOT NULL OR NEW.superseded_at IS NOT NULL THEN
   RAISE EXCEPTION 'invalid config draft' USING ERRCODE='55000';
  END IF;
  RETURN NEW;
 END IF;
 IF (to_jsonb(NEW)-ARRAY['status','validated_at','previewed_at','activated_at','superseded_at'])
    IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['status','validated_at','previewed_at','activated_at','superseded_at']) THEN
  RAISE EXCEPTION 'immutable validated config' USING ERRCODE='55000';
 END IF;
 IF OLD.status='DRAFT' AND NEW.status='VALIDATED' AND NEW.validated_at IS NOT NULL
  AND NEW.previewed_at IS NULL AND NEW.activated_at IS NULL AND NEW.superseded_at IS NULL THEN RETURN NEW; END IF;
 IF OLD.status='VALIDATED' AND NEW.status='PREVIEWED' AND NEW.validated_at=OLD.validated_at
  AND NEW.previewed_at IS NOT NULL AND NEW.activated_at IS NULL AND NEW.superseded_at IS NULL THEN RETURN NEW; END IF;
 IF OLD.status='PREVIEWED' AND NEW.status='ACTIVE' AND NEW.validated_at=OLD.validated_at
  AND NEW.previewed_at=OLD.previewed_at AND NEW.activated_at IS NOT NULL AND NEW.superseded_at IS NULL THEN RETURN NEW; END IF;
 IF OLD.status='ACTIVE' AND NEW.status='SUPERSEDED' AND NEW.validated_at=OLD.validated_at
  AND NEW.previewed_at=OLD.previewed_at AND NEW.activated_at=OLD.activated_at AND NEW.superseded_at IS NOT NULL THEN RETURN NEW; END IF;
 RAISE EXCEPTION 'invalid config lifecycle transition' USING ERRCODE='55000';
END $$;
CREATE TRIGGER game_config_lifecycle BEFORE UPDATE ON games.game_config_versions
 FOR EACH ROW EXECUTE FUNCTION games.guard_config_version_lifecycle();
CREATE TRIGGER immutable_history BEFORE DELETE OR TRUNCATE ON games.game_config_versions
 FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();

-- Platform Operations needs only aggregate activity for the catalog row. Keep
-- Poker's raw tables behind its runtime role and expose six bounded counters.
CREATE FUNCTION games.ops_activity_read()
RETURNS TABLE(game_slug text,rounds_24h bigint,needs_review_rounds bigint)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog SET TimeZone='UTC' AS $$
 WITH direct_activity AS (
  SELECT r.game_slug,
   count(*) FILTER(WHERE r.created_at>=statement_timestamp()-interval '24 hours') AS rounds_24h,
   count(*) FILTER(WHERE r.recovery_state='NEEDS_REVIEW') AS needs_review_rounds
  FROM games.game_rounds r GROUP BY r.game_slug
 ), poker_activity AS (
  SELECT 'texas-holdem'::text AS game_slug,
   count(*) FILTER(WHERE h.created_at>=statement_timestamp()-interval '24 hours') AS rounds_24h,
   count(*) FILTER(WHERE h.state<>'SETTLED' AND
    (t.lifecycle_state='RECOVERING' OR rs.state='NEEDS_REVIEW')) AS needs_review_rounds
  FROM poker.hands h JOIN poker.tables t USING(table_id)
  LEFT JOIN poker.recovery_state rs USING(table_id)
 ), activity AS (
  SELECT * FROM direct_activity UNION ALL SELECT * FROM poker_activity
 )
 SELECT r.game_slug,coalesce(a.rounds_24h,0),coalesce(a.needs_review_rounds,0)
 FROM games.game_registry r LEFT JOIN activity a USING(game_slug)
 ORDER BY r.sort_order,r.game_slug
$$;
REVOKE ALL ON FUNCTION games.ops_activity_read() FROM PUBLIC;

-- Poker moderation is append-only. The latest event is the current projection;
-- the original message and every prior decision remain durable.
ALTER TABLE poker.tables ADD COLUMN updated_at timestamptz NOT NULL DEFAULT clock_timestamp();
CREATE FUNCTION poker.touch_table_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN NEW.updated_at=clock_timestamp(); RETURN NEW; END $$;
CREATE TRIGGER poker_table_updated_at BEFORE UPDATE ON poker.tables
 FOR EACH ROW EXECUTE FUNCTION poker.touch_table_updated_at();

CREATE TABLE poker.chat_moderation_events (
 event_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 operation_id uuid,
 table_id uuid NOT NULL REFERENCES poker.tables(table_id),
 target_kind text NOT NULL CHECK(target_kind IN('USER','MESSAGE')),
 target_newapi_user_id bigint REFERENCES identity.account_refs(newapi_user_id),
 message_id uuid,
 action text NOT NULL CHECK(action IN('MUTE','UNMUTE','HIDE','RESTORE')),
 actor_user_id bigint NOT NULL REFERENCES identity.account_refs(newapi_user_id),
 actor_kind text NOT NULL CHECK(actor_kind IN('HOST','ADMIN')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(operation_id),
 FOREIGN KEY(table_id,message_id) REFERENCES poker.chat_messages(table_id,message_id),
 CHECK((target_kind='USER' AND target_newapi_user_id IS NOT NULL AND message_id IS NULL AND action IN('MUTE','UNMUTE'))
    OR (target_kind='MESSAGE' AND target_newapi_user_id IS NULL AND message_id IS NOT NULL AND action IN('HIDE','RESTORE')))
);
CREATE INDEX poker_chat_moderation_user_latest
 ON poker.chat_moderation_events(table_id,target_newapi_user_id,created_at DESC,event_id DESC) WHERE target_kind='USER';
CREATE INDEX poker_chat_moderation_message_latest
 ON poker.chat_moderation_events(table_id,message_id,created_at DESC,event_id DESC) WHERE target_kind='MESSAGE';

INSERT INTO poker.chat_moderation_events(table_id,target_kind,target_newapi_user_id,action,actor_user_id,actor_kind,created_at)
 SELECT table_id,'USER',target_newapi_user_id,'MUTE',muted_by_newapi_user_id,'HOST',created_at FROM poker.chat_mutes;

CREATE TABLE poker.admin_operations (
 operation_id uuid PRIMARY KEY,
 actor_user_id bigint NOT NULL REFERENCES identity.account_refs(newapi_user_id),
 command_type text NOT NULL CHECK(command_type IN(
  'POKER_ACCEPTING_PLAYERS_SET','POKER_NEW_HANDS_SET','POKER_CLOSE_AFTER_HAND',
  'POKER_REMOVE_PLAYER_AFTER_HAND','POKER_REMOVE_SPECTATOR','POKER_CHAT_MUTE_SET',
  'POKER_CHAT_VISIBILITY_SET','POKER_TABLE_PAUSE','POKER_TABLE_RESUME',
  'POKER_RECOVERY_REQUEST','POKER_EMERGENCY_PAUSE')),
 table_id uuid NOT NULL REFERENCES poker.tables(table_id),
 request_hash bytea NOT NULL CHECK(octet_length(request_hash)=32),
 command_payload jsonb NOT NULL CHECK(jsonb_typeof(command_payload)='object'),
 result jsonb NOT NULL CHECK(jsonb_typeof(result)='object'),
 completed_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TRIGGER poker_admin_operations_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON poker.admin_operations
 FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TRIGGER poker_chat_moderation_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON poker.chat_moderation_events
 FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();

-- Rankings snapshots remain immutable. Visibility is a mutable, versioned
-- pointer with an append-only publication trail; repair builds never publish.
ALTER TABLE rankings.snapshots
 ADD COLUMN build_kind text NOT NULL DEFAULT 'ROUTINE' CHECK(build_kind IN('ROUTINE','REPAIR')),
 ADD COLUMN operation_id uuid,
 ADD COLUMN aggregate_hash bytea CHECK(aggregate_hash IS NULL OR octet_length(aggregate_hash)=32),
 ADD CONSTRAINT rankings_snapshot_operation_unique UNIQUE(operation_id);

-- A snapshot row must exist before its aggregate entries can be built. Permit
-- one finalization write for the resulting digest while keeping every other
-- snapshot field and every already-finalized digest immutable. Legacy rows
-- remain nullable because 0027 did not retain enough build material to derive
-- the new canonical digest during migration.
DROP TRIGGER ranking_snapshots_immutable ON rankings.snapshots;
CREATE FUNCTION rankings.guard_snapshot_finalization() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.aggregate_hash IS NULL AND NEW.aggregate_hash IS NOT NULL
  AND octet_length(NEW.aggregate_hash)=32
  AND (to_jsonb(NEW)-'aggregate_hash') IS NOT DISTINCT FROM (to_jsonb(OLD)-'aggregate_hash') THEN
  RETURN NEW;
 END IF;
 RAISE EXCEPTION 'immutable ranking snapshot' USING ERRCODE='55000';
END $$;
CREATE TRIGGER ranking_snapshot_finalization BEFORE UPDATE ON rankings.snapshots
 FOR EACH ROW EXECUTE FUNCTION rankings.guard_snapshot_finalization();
CREATE TRIGGER ranking_snapshots_immutable BEFORE DELETE OR TRUNCATE ON rankings.snapshots
 FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();

CREATE TABLE rankings.published_pointers (
 domain text NOT NULL CHECK(domain IN('GAMES','RP','ASSETS')),
 metric text NOT NULL CHECK(metric IN('TOTAL_ASSETS','GAME_PROFIT','BIGGEST_WIN','TOTAL_WAGERED','POKER_PROFIT','RP_CALLS','RP_ERRORS','RP_CREDITS')),
 period text NOT NULL CHECK(period IN('DAY','WEEK','ALL_TIME','CURRENT')),
 period_start timestamptz NOT NULL,
 activation_at timestamptz NOT NULL,
 snapshot_id uuid NOT NULL REFERENCES rankings.snapshots(snapshot_id) ON DELETE RESTRICT,
 version bigint NOT NULL CHECK(version>0),
 published_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(domain,metric,period,period_start,activation_at)
);
CREATE UNIQUE INDEX rankings_published_snapshot_metric ON rankings.published_pointers(snapshot_id,metric);

INSERT INTO rankings.published_pointers(domain,metric,period,period_start,activation_at,snapshot_id,version,published_at)
 SELECT domain,metric,period,period_start,activation_at,snapshot_id,1,built_at
 FROM (
  SELECT s.domain,e.metric,s.period,s.period_start,s.activation_at,s.snapshot_id,s.built_at,
   row_number() OVER(PARTITION BY s.domain,e.metric,s.period,s.period_start,s.activation_at ORDER BY s.built_at DESC,s.snapshot_id DESC) AS newest
  FROM rankings.snapshots s JOIN (SELECT DISTINCT snapshot_id,metric FROM rankings.entries) e USING(snapshot_id)
  WHERE s.status='READY'
 ) candidates WHERE newest=1;

CREATE TABLE rankings.publication_events (
 publication_event_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 operation_id uuid,
 domain text NOT NULL,
 metric text NOT NULL,
 period text NOT NULL,
 period_start timestamptz NOT NULL,
 activation_at timestamptz NOT NULL,
 previous_snapshot_id uuid REFERENCES rankings.snapshots(snapshot_id) ON DELETE RESTRICT,
 snapshot_id uuid NOT NULL REFERENCES rankings.snapshots(snapshot_id) ON DELETE RESTRICT,
 pointer_version bigint NOT NULL CHECK(pointer_version>0),
 actor_user_id bigint REFERENCES identity.account_refs(newapi_user_id),
 source text NOT NULL CHECK(source IN('ROUTINE','REPAIR')),
 published_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(operation_id),
 FOREIGN KEY(domain,metric,period,period_start,activation_at)
  REFERENCES rankings.published_pointers(domain,metric,period,period_start,activation_at)
);

CREATE TABLE rankings.rebuild_jobs (
 operation_id uuid PRIMARY KEY,
 actor_user_id bigint NOT NULL REFERENCES identity.account_refs(newapi_user_id),
 domain text NOT NULL CHECK(domain IN('GAMES','RP','ASSETS')),
 metric text NOT NULL CHECK(metric IN('TOTAL_ASSETS','GAME_PROFIT','BIGGEST_WIN','TOTAL_WAGERED','POKER_PROFIT','RP_CALLS','RP_ERRORS','RP_CREDITS')),
 period text NOT NULL CHECK(period IN('DAY','WEEK','ALL_TIME','CURRENT')),
 requested_date text NOT NULL DEFAULT '',
 model_scope text NOT NULL CHECK(model_scope IN('ALL','MODEL')),
 model_id text NOT NULL DEFAULT '',
 expected_pointer_version bigint NOT NULL CHECK(expected_pointer_version>=0),
 status text NOT NULL DEFAULT 'PENDING' CHECK(status IN('PENDING','RUNNING','SHADOW','FAILED')),
 shadow_snapshot_id uuid REFERENCES rankings.snapshots(snapshot_id) ON DELETE RESTRICT,
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts>=0),
 failure_code text,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK((model_scope='ALL' AND model_id='') OR (model_scope='MODEL' AND model_id<>'')),
 CHECK((status='SHADOW')=(shadow_snapshot_id IS NOT NULL))
);
CREATE INDEX rankings_rebuild_jobs_due ON rankings.rebuild_jobs(created_at,operation_id) WHERE status='PENDING';
CREATE TRIGGER ranking_publication_events_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON rankings.publication_events
 FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();

REVOKE ALL ON games.game_config_versions,poker.chat_moderation_events,poker.admin_operations,
 rankings.published_pointers,rankings.publication_events,rankings.rebuild_jobs FROM PUBLIC;
