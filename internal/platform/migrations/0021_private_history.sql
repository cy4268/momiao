-- Install in an owner migration window after domain writers have drained.
-- Snapshots are durable metadata; only the index and cursors are rebuildable.
CREATE TABLE games.history_display_snapshots (
 record_type text NOT NULL CHECK(record_type IN('DIRECT_PLAY_ROUND','POKER_SESSION','POKER_HAND')),
 source_id uuid NOT NULL, newapi_user_id bigint NOT NULL CHECK(newapi_user_id>0),
 snapshot_id uuid NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 game_slug text NOT NULL, game_title text NOT NULL,
 table_id uuid, table_name text, actor_display_name text,
 metadata_origin text NOT NULL CHECK(metadata_origin IN('creation_snapshot','migration_metadata_backfill')),
 captured_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(record_type,source_id,newapi_user_id)
);
CREATE TRIGGER history_display_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
 ON games.history_display_snapshots FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TABLE games.history_index (
 record_type text NOT NULL CHECK(record_type IN('DIRECT_PLAY_ROUND','POKER_SESSION','POKER_HAND')),
 source_id uuid NOT NULL, newapi_user_id bigint NOT NULL CHECK(newapi_user_id>0),
 parent_source_id uuid, snapshot_id uuid NOT NULL, game_slug text NOT NULL,
 mode text NOT NULL CHECK(mode IN('DIRECT_PLAY','POKER')),
 occurred_at timestamptz NOT NULL, ended_at timestamptz,
 result_class text CHECK(result_class IN('WIN','LOSS','BREAK_EVEN','CANCELLED','REFUNDED')),
 display_status text NOT NULL CHECK(display_status IN('PROCESSING','SETTLED','RECOVERING','CANCELLED','REFUNDED')),
 stake_units numeric(38,0), payout_units numeric(38,0), net_change_units numeric(38,0),
 initial_buyin_units numeric(38,0), total_topup_units numeric(38,0), final_cashout_units numeric(38,0),
 source_version text NOT NULL, updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 checked_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(record_type,source_id,newapi_user_id)
);
CREATE INDEX private_history_owner_order ON games.history_index
 (newapi_user_id,occurred_at DESC,record_type COLLATE "C",source_id DESC);
CREATE INDEX private_history_refresh ON games.history_index(record_type,checked_at,source_id)
 WHERE display_status IN('PROCESSING','RECOVERING');
CREATE TABLE games.history_ingestion_cursors (
 source text PRIMARY KEY CHECK(source IN('DIRECT_PLAY_ROUND','POKER_SESSION','POKER_HAND')),
 source_timestamp timestamptz, source_id uuid,
 CHECK((source_timestamp IS NULL)=(source_id IS NULL))
);

CREATE FUNCTION games.history_capture_display() RETURNS trigger
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog SET TimeZone='UTC' AS $$
DECLARE rt text; sid uuid; uid bigint; slug text; tid uuid; actor text; title text; table_title text;
BEGIN
 CASE TG_TABLE_NAME
 WHEN 'game_rounds' THEN
  rt:='DIRECT_PLAY_ROUND'; sid:=NEW.round_id; uid:=NEW.newapi_user_id; slug:=NEW.game_slug;
  SELECT display_name INTO actor FROM identity.master_profiles WHERE newapi_user_id=uid;
 WHEN 'sessions' THEN
  rt:='POKER_SESSION'; sid:=NEW.session_id; uid:=NEW.newapi_user_id;
  slug:='texas-holdem'; tid:=NEW.table_id; actor:=NEW.display_name_snapshot;
 WHEN 'hand_participants' THEN
  rt:='POKER_HAND'; sid:=NEW.hand_id; uid:=NEW.newapi_user_id; slug:='texas-holdem';
  SELECT s.table_id,s.display_name_snapshot INTO STRICT tid,actor
   FROM poker.sessions s WHERE s.session_id=NEW.session_id AND s.newapi_user_id=uid;
 ELSE RAISE EXCEPTION 'HISTORY_SOURCE_INVALID' USING ERRCODE='22023';
 END CASE;
 SELECT g.title INTO STRICT title FROM games.game_registry g WHERE g.game_slug=slug;
 IF tid IS NOT NULL THEN SELECT t.name INTO STRICT table_title FROM poker.tables t WHERE t.table_id=tid; END IF;
 INSERT INTO games.history_display_snapshots
  (record_type,source_id,newapi_user_id,game_slug,game_title,table_id,table_name,actor_display_name,metadata_origin)
 VALUES(rt,sid,uid,slug,title,tid,table_title,actor,'creation_snapshot');
 RETURN NEW;
END $$;
CREATE TRIGGER private_history_round_created AFTER INSERT ON games.game_rounds
 FOR EACH ROW EXECUTE FUNCTION games.history_capture_display();
CREATE TRIGGER private_history_session_created AFTER INSERT ON poker.sessions
 FOR EACH ROW EXECUTE FUNCTION games.history_capture_display();
CREATE TRIGGER private_history_participant_created AFTER INSERT ON poker.hand_participants
 FOR EACH ROW EXECUTE FUNCTION games.history_capture_display();

-- No source row locks, ciphertext, dealt cards, seed material or ledger reads.
CREATE VIEW games.history_source_rows AS
WITH source_rows AS (
 SELECT 'DIRECT_PLAY_ROUND'::text AS record_type,r.round_id AS source_id,r.newapi_user_id,
  NULL::uuid AS parent_source_id,r.game_slug,'DIRECT_PLAY'::text AS mode,
  r.created_at AS occurred_at,r.settled_at AS ended_at,r.common_result AS result_class,
  CASE WHEN r.state='SETTLED' THEN 'SETTLED' WHEN r.recovery_state='NEEDS_REVIEW' THEN 'RECOVERING'
   ELSE 'PROCESSING' END AS display_status,
  r.total_stake_units::numeric AS stake_units,
  CASE WHEN r.state='SETTLED' THEN r.total_payout_units::numeric END AS payout_units,
  CASE WHEN r.state='SETTLED' THEN r.net_change_units::numeric END AS net_change_units,
  NULL::numeric AS initial_buyin_units,NULL::numeric AS total_topup_units,NULL::numeric AS final_cashout_units,
  g.title AS current_game_title,NULL::uuid AS table_id,NULL::text AS current_table_name,p.display_name AS current_actor_name
 FROM games.game_rounds r JOIN games.game_registry g USING(game_slug)
 LEFT JOIN identity.master_profiles p USING(newapi_user_id)
 UNION ALL
 SELECT 'POKER_SESSION',s.session_id,s.newapi_user_id,NULL,'texas-holdem','POKER',s.started_at,s.ended_at,
  CASE WHEN s.state='SETTLED' THEN CASE WHEN s.realized_pl_units>0 THEN 'WIN'
   WHEN s.realized_pl_units<0 THEN 'LOSS' ELSE 'BREAK_EVEN' END END,
  CASE WHEN s.state='SETTLED' THEN 'SETTLED' WHEN s.state='NEEDS_REVIEW' OR t.lifecycle_state='RECOVERING'
   THEN 'RECOVERING' ELSE 'PROCESSING' END,
  NULL,NULL,CASE WHEN s.state='SETTLED' THEN s.realized_pl_units END,
  s.initial_buyin_units,s.total_topup_units,CASE WHEN s.state='SETTLED' THEN s.final_cashout_units END,
  g.title,t.table_id,t.name,s.display_name_snapshot
 FROM poker.sessions s JOIN poker.tables t USING(table_id)
 JOIN games.game_registry g ON g.game_slug='texas-holdem'
 UNION ALL
 SELECT 'POKER_HAND',h.hand_id,p.newapi_user_id,p.session_id,'texas-holdem','POKER',h.created_at,h.settled_at,
  CASE WHEN h.state='SETTLED' THEN CASE WHEN a.awarded+x.returned-x.paid>0 THEN 'WIN'
   WHEN a.awarded+x.returned-x.paid<0 THEN 'LOSS' ELSE 'BREAK_EVEN' END END,
  CASE WHEN h.state='SETTLED' THEN 'SETTLED' WHEN t.lifecycle_state='RECOVERING' AND t.current_hand_id=h.hand_id
   THEN 'RECOVERING' ELSE 'PROCESSING' END,
  x.paid,CASE WHEN h.state='SETTLED' THEN a.awarded+x.returned END,
  CASE WHEN h.state='SETTLED' THEN a.awarded+x.returned-x.paid END,NULL,NULL,NULL,
  g.title,t.table_id,t.name,s.display_name_snapshot
 FROM poker.hands h JOIN poker.hand_participants p USING(hand_id)
 JOIN poker.sessions s ON s.session_id=p.session_id AND s.newapi_user_id=p.newapi_user_id
 JOIN poker.tables t ON t.table_id=h.table_id
 JOIN games.game_registry g ON g.game_slug='texas-holdem'
 CROSS JOIN LATERAL (
  SELECT coalesce(sum(applied_delta_units) FILTER(WHERE event_type IN('POST_SB','POST_BB','CALL','BET','RAISE','ALL_IN')),0) AS paid,
   coalesce(sum(applied_delta_units) FILTER(WHERE event_type='RETURN_UNCALLED'),0) AS returned
  FROM poker.actions WHERE hand_id=h.hand_id AND actor_seat=p.seat_no
 ) x
 CROSS JOIN LATERAL (
  SELECT coalesce(sum(award_units),0) AS awarded FROM poker.pot_awards WHERE hand_id=h.hand_id AND seat_no=p.seat_no
 ) a
)
SELECT r.*,s.snapshot_id,
 md5((to_jsonb(r)-ARRAY['current_game_title','current_table_name','current_actor_name'])::text) AS source_version
FROM source_rows r LEFT JOIN games.history_display_snapshots s
 USING(record_type,source_id,newapi_user_id);

INSERT INTO games.history_display_snapshots
 (record_type,source_id,newapi_user_id,game_slug,game_title,table_id,table_name,actor_display_name,metadata_origin)
SELECT record_type,source_id,newapi_user_id,game_slug,current_game_title,table_id,current_table_name,
 current_actor_name,'migration_metadata_backfill' FROM games.history_source_rows;

CREATE FUNCTION games.history_ingest_batch(p_source text,p_limit integer) RETURNS jsonb
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog SET TimeZone='UTC' AS $$
DECLARE source_no integer; c games.history_ingestion_cursors; r record;
 fresh uuid[]; missing uuid[]; pending uuid[]; selected uuid[]; written integer:=0;
BEGIN
 source_no:=array_position(ARRAY['DIRECT_PLAY_ROUND','POKER_SESSION','POKER_HAND'],p_source);
 IF source_no IS NULL OR p_limit IS NULL OR p_limit<1 OR p_limit>200 THEN
  RAISE EXCEPTION 'HISTORY_QUERY_INVALID' USING ERRCODE='22023';
 END IF;
 IF NOT pg_try_advisory_xact_lock(1212765012,source_no) THEN RETURN jsonb_build_object('busy',true); END IF;
 INSERT INTO games.history_ingestion_cursors(source) VALUES(p_source) ON CONFLICT DO NOTHING;
 SELECT * INTO STRICT c FROM games.history_ingestion_cursors WHERE source=p_source;
 SELECT coalesce(array_agg(source_id),ARRAY[]::uuid[]) INTO fresh FROM (
  SELECT source_id,occurred_at FROM games.history_source_rows WHERE record_type=p_source
   AND (c.source_id IS NULL OR (occurred_at,source_id)>(c.source_timestamp,c.source_id))
  GROUP BY source_id,occurred_at ORDER BY occurred_at,source_id LIMIT p_limit
 ) n;
 -- ponytail: this anti-join may scan old sources; measure before replacing it.
 -- A bounded output is not a bounded scan, and no time lookback proves commit order.
 SELECT coalesce(array_agg(source_id),ARRAY[]::uuid[]) INTO missing FROM (
  SELECT v.source_id,v.occurred_at FROM games.history_source_rows v WHERE v.record_type=p_source
   AND NOT EXISTS(SELECT 1 FROM games.history_index i WHERE i.record_type=v.record_type
    AND i.source_id=v.source_id AND i.newapi_user_id=v.newapi_user_id)
  GROUP BY v.source_id,v.occurred_at ORDER BY v.occurred_at,v.source_id LIMIT p_limit
 ) n;
 SELECT coalesce(array_agg(source_id),ARRAY[]::uuid[]) INTO pending FROM (
  SELECT source_id FROM games.history_index WHERE record_type=p_source AND display_status IN('PROCESSING','RECOVERING')
  GROUP BY source_id ORDER BY min(checked_at),source_id LIMIT p_limit
 ) n;
 SELECT array_agg(DISTINCT id) INTO selected FROM unnest(fresh||missing||pending) id;
 FOR r IN SELECT * FROM games.history_source_rows WHERE record_type=p_source AND source_id=ANY(selected)
  ORDER BY source_id,newapi_user_id LOOP
  IF r.snapshot_id IS NULL THEN RAISE EXCEPTION 'HISTORY_SNAPSHOT_MISSING' USING ERRCODE='55000'; END IF;
  IF r.display_status='SETTLED' AND (r.ended_at IS NULL OR r.net_change_units IS NULL) THEN
   RAISE EXCEPTION 'HISTORY_SOURCE_INVALID' USING ERRCODE='55000';
  END IF;
  INSERT INTO games.history_index AS i
   (record_type,source_id,newapi_user_id,parent_source_id,snapshot_id,game_slug,mode,occurred_at,ended_at,
    result_class,display_status,stake_units,payout_units,net_change_units,initial_buyin_units,total_topup_units,final_cashout_units,source_version)
  VALUES(r.record_type,r.source_id,r.newapi_user_id,r.parent_source_id,r.snapshot_id,r.game_slug,r.mode,r.occurred_at,r.ended_at,
   r.result_class,r.display_status,r.stake_units,r.payout_units,r.net_change_units,r.initial_buyin_units,r.total_topup_units,r.final_cashout_units,r.source_version)
  ON CONFLICT(record_type,source_id,newapi_user_id) DO UPDATE SET
   ended_at=EXCLUDED.ended_at,result_class=EXCLUDED.result_class,display_status=EXCLUDED.display_status,
   stake_units=EXCLUDED.stake_units,payout_units=EXCLUDED.payout_units,net_change_units=EXCLUDED.net_change_units,
   initial_buyin_units=EXCLUDED.initial_buyin_units,total_topup_units=EXCLUDED.total_topup_units,
   final_cashout_units=EXCLUDED.final_cashout_units,source_version=EXCLUDED.source_version,
   updated_at=CASE WHEN i.source_version=EXCLUDED.source_version THEN i.updated_at ELSE clock_timestamp() END,
   checked_at=clock_timestamp();
  written:=written+1;
 END LOOP;
 IF cardinality(fresh)>0 THEN
  SELECT occurred_at,source_id INTO c.source_timestamp,c.source_id FROM games.history_source_rows
   WHERE record_type=p_source AND source_id=ANY(fresh) ORDER BY occurred_at DESC,source_id DESC LIMIT 1;
  UPDATE games.history_ingestion_cursors SET source_timestamp=c.source_timestamp,source_id=c.source_id WHERE source=p_source;
 END IF;
 RETURN jsonb_build_object('busy',false,'new_sources',cardinality(fresh),'missing_sources',cardinality(missing),
  'refreshed_sources',cardinality(pending),'written_rows',written,'source_timestamp',c.source_timestamp,'source_id',c.source_id);
END $$;

CREATE FUNCTION games.history_list(p_user bigint,q jsonb) RETURNS jsonb
 LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog SET TimeZone='UTC' AS $$
DECLARE n integer:=coalesce((q->>'limit')::integer,50); output jsonb;
BEGIN
 IF p_user IS NULL OR p_user<=0 OR q IS NULL OR jsonb_typeof(q)<>'object' OR n<1 OR n>100
  OR EXISTS(SELECT 1 FROM jsonb_object_keys(q) k WHERE k NOT IN('record_type','mode','game_slug','time_from','time_to',
   'result','status','id','parent_source_id','limit','before_time','before_type','before_id'))
  OR (q ? 'record_type' AND q->>'record_type' NOT IN('DIRECT_PLAY_ROUND','POKER_SESSION','POKER_HAND'))
  OR (q ? 'mode' AND q->>'mode' NOT IN('DIRECT_PLAY','POKER'))
  OR (q ? 'result' AND q->>'result' NOT IN('WIN','LOSS','BREAK_EVEN','CANCELLED','REFUNDED'))
  OR (q ? 'status' AND q->>'status' NOT IN('PROCESSING','SETTLED','RECOVERING','CANCELLED','REFUNDED')) THEN
  RAISE EXCEPTION 'HISTORY_QUERY_INVALID' USING ERRCODE='22023';
 END IF;
 WITH bounded AS (
  SELECT i.* FROM games.history_index i WHERE i.newapi_user_id=p_user
   AND CASE WHEN q ? 'record_type' THEN i.record_type=q->>'record_type'
    WHEN q ? 'parent_source_id' THEN i.record_type='POKER_HAND' ELSE i.record_type IN('DIRECT_PLAY_ROUND','POKER_SESSION') END
   AND (NOT q ? 'mode' OR i.mode=q->>'mode') AND (NOT q ? 'game_slug' OR i.game_slug=q->>'game_slug')
   AND (NOT q ? 'result' OR i.result_class=q->>'result') AND (NOT q ? 'status' OR i.display_status=q->>'status')
   AND (NOT q ? 'id' OR i.source_id=(q->>'id')::uuid)
   AND (NOT q ? 'parent_source_id' OR i.parent_source_id=(q->>'parent_source_id')::uuid)
   AND (NOT q ? 'time_from' OR i.occurred_at>=(q->>'time_from')::timestamptz)
   AND (NOT q ? 'time_to' OR i.occurred_at<(q->>'time_to')::timestamptz)
   AND (NOT q ? 'before_time' OR i.occurred_at<(q->>'before_time')::timestamptz
    OR (i.occurred_at=(q->>'before_time')::timestamptz AND (i.record_type COLLATE "C">q->>'before_type'
     OR (i.record_type=q->>'before_type' AND i.source_id<(q->>'before_id')::uuid))))
  ORDER BY i.occurred_at DESC,i.record_type COLLATE "C",i.source_id DESC LIMIT n+1
 ), page AS (
  SELECT * FROM bounded ORDER BY occurred_at DESC,record_type COLLATE "C",source_id DESC LIMIT n
 ), options AS (
  SELECT DISTINCT ON(game_slug) game_slug,game_title,retired FROM (
   SELECT game_slug,title AS game_title,publication_state='RETIRED' AS retired,0 AS priority,NULL::timestamptz AS at
    FROM games.game_registry WHERE publication_state IN('PUBLISHED','RETIRED')
   UNION ALL SELECT s.game_slug,s.game_title,coalesce(g.publication_state='RETIRED',false),1,s.captured_at
    FROM games.history_display_snapshots s LEFT JOIN games.game_registry g USING(game_slug) WHERE s.newapi_user_id=p_user
  ) all_options ORDER BY game_slug,priority,at DESC,game_title
 )
 SELECT jsonb_build_object('items',coalesce((SELECT jsonb_agg(
  jsonb_build_object('record_type',i.record_type,'source_id',i.source_id,'newapi_user_id',i.newapi_user_id::text,
   'parent_source_id',i.parent_source_id,'game_slug',i.game_slug,'mode',i.mode,'occurred_at',i.occurred_at,'ended_at',i.ended_at,
   'result',i.result_class,'status',i.display_status,'source_version',i.source_version,
   'stake_units',i.stake_units::text,'payout_units',i.payout_units::text,'net_change_units',i.net_change_units::text,
   'initial_buyin_units',i.initial_buyin_units::text,'total_topup_units',i.total_topup_units::text,'final_cashout_units',i.final_cashout_units::text,
   'snapshot',jsonb_build_object('snapshot_id',s.snapshot_id,'game_title',s.game_title,'table_id',s.table_id,
    'table_name',s.table_name,'actor_display_name',s.actor_display_name,'metadata_origin',s.metadata_origin))
  ORDER BY i.occurred_at DESC,i.record_type COLLATE "C",i.source_id DESC)
  FROM page i JOIN games.history_display_snapshots s USING(record_type,source_id,newapi_user_id)), '[]'::jsonb),
  'has_more',(SELECT count(*)>n FROM bounded),'game_options',coalesce((SELECT jsonb_agg(to_jsonb(o) ORDER BY game_slug) FROM options o),'[]'::jsonb)) INTO output;
 RETURN output;
END $$;
REVOKE ALL ON games.history_display_snapshots,games.history_index,games.history_ingestion_cursors,games.history_source_rows FROM PUBLIC;
REVOKE ALL ON FUNCTION games.history_capture_display(),games.history_ingest_batch(text,integer),games.history_list(bigint,jsonb) FROM PUBLIC;
