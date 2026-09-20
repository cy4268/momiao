-- Append roulette to existing private History capabilities; old sources retain their order.
ALTER TABLE games.history_display_snapshots DROP CONSTRAINT history_display_snapshots_record_type_check;
ALTER TABLE games.history_display_snapshots ADD CHECK(record_type IN('DIRECT_PLAY_ROUND','POKER_SESSION','POKER_HAND','ROULETTE_ROUND'));
ALTER TABLE games.history_index DROP CONSTRAINT history_index_record_type_check;
ALTER TABLE games.history_index ADD CHECK(record_type IN('DIRECT_PLAY_ROUND','POKER_SESSION','POKER_HAND','ROULETTE_ROUND'));
ALTER TABLE games.history_index DROP CONSTRAINT history_index_mode_check;
ALTER TABLE games.history_index ADD CHECK(mode IN('DIRECT_PLAY','POKER','ROULETTE'));
ALTER TABLE games.history_ingestion_cursors DROP CONSTRAINT history_ingestion_cursors_source_check;
ALTER TABLE games.history_ingestion_cursors ADD CHECK(source IN('DIRECT_PLAY_ROUND','POKER_SESSION','POKER_HAND','ROULETTE_ROUND'));
-- An administrative void never overwrites the original adjudicated outcome or hidden snapshot.
ALTER TABLE roulette.rounds ADD COLUMN void_outcome jsonb CHECK(void_outcome IS NULL OR (state='CANCELLED' AND escrow_units=0 AND void_outcome->>'reason'='SYSTEM_VOID' AND void_outcome->>'refunds'='true'));
CREATE OR REPLACE FUNCTION roulette.guard_round() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.state IN('FINISHED','CANCELLED') OR NEW.version<>OLD.version+1 OR NEW.action_sequence NOT BETWEEN OLD.action_sequence AND OLD.action_sequence+1 THEN
  RAISE EXCEPTION 'invalid roulette lifecycle' USING ERRCODE='55000';
 END IF;
 IF (to_jsonb(NEW)-ARRAY['snapshot_key_version','snapshot_nonce','snapshot_ciphertext','snapshot_version','version','action_sequence','state','escrow_units','outcome','started_at','ended_at','deadline','game_deadline','revealed_seed','void_outcome']) IS DISTINCT FROM
    (to_jsonb(OLD)-ARRAY['snapshot_key_version','snapshot_nonce','snapshot_ciphertext','snapshot_version','version','action_sequence','state','escrow_units','outcome','started_at','ended_at','deadline','game_deadline','revealed_seed','void_outcome']) THEN
  RAISE EXCEPTION 'immutable roulette binding' USING ERRCODE='55000';
 END IF;
 IF OLD.outcome IS NOT NULL AND NEW.outcome IS DISTINCT FROM OLD.outcome THEN RAISE EXCEPTION 'immutable roulette outcome' USING ERRCODE='55000'; END IF;
 IF NEW.snapshot_ciphertext IS DISTINCT FROM OLD.snapshot_ciphertext AND NEW.snapshot_version<>OLD.snapshot_version+1 THEN RAISE EXCEPTION 'invalid snapshot version' USING ERRCODE='55000'; END IF;
 IF OLD.state='WAITING' AND NEW.state NOT IN('WAITING','PLAYING','SETTLING','CANCELLED','NEEDS_REVIEW') OR
    OLD.state='PLAYING' AND NEW.state NOT IN('PLAYING','SETTLING','FINISHED','NEEDS_REVIEW') OR
    OLD.state='SETTLING' AND NEW.state NOT IN('SETTLING','FINISHED','CANCELLED','NEEDS_REVIEW') OR
    OLD.state='NEEDS_REVIEW' AND NEW.state NOT IN('NEEDS_REVIEW','CANCELLED') THEN RAISE EXCEPTION 'invalid roulette transition' USING ERRCODE='55000'; END IF;
 IF NEW.void_outcome IS DISTINCT FROM OLD.void_outcome AND (OLD.void_outcome IS NOT NULL OR OLD.state<>'NEEDS_REVIEW' OR NEW.void_outcome IS NULL OR EXISTS(SELECT 1 FROM roulette.funding WHERE round_id=OLD.round_id AND kind='PAYOUT')) THEN RAISE EXCEPTION 'invalid roulette void' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION roulette.capture_history() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.kind='ESCROW' THEN
  INSERT INTO games.history_display_snapshots(record_type,source_id,newapi_user_id,game_slug,game_title,actor_display_name,metadata_origin)
  SELECT 'ROULETTE_ROUND',r.round_id,NEW.newapi_user_id,r.game_slug,r.title_snapshot,p.display_name_snapshot,'creation_snapshot'
  FROM roulette.rounds r JOIN roulette.participants p USING(round_id) WHERE r.round_id=NEW.round_id AND p.newapi_user_id=NEW.newapi_user_id
  ON CONFLICT(record_type,source_id,newapi_user_id) DO NOTHING;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER roulette_history_funded AFTER INSERT ON roulette.funding FOR EACH ROW EXECUTE FUNCTION roulette.capture_history();
REVOKE ALL ON FUNCTION roulette.capture_history() FROM PUBLIC;
INSERT INTO games.history_display_snapshots(record_type,source_id,newapi_user_id,game_slug,game_title,actor_display_name,metadata_origin)
SELECT DISTINCT 'ROULETTE_ROUND',r.round_id,p.newapi_user_id,r.game_slug,r.title_snapshot,p.display_name_snapshot,'migration_metadata_backfill'
FROM roulette.funding f JOIN roulette.rounds r USING(round_id) JOIN roulette.participants p USING(round_id,newapi_user_id)
WHERE f.kind='ESCROW' ON CONFLICT(record_type,source_id,newapi_user_id) DO NOTHING;
CREATE OR REPLACE VIEW games.history_source_rows AS
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
 UNION ALL
 SELECT 'ROULETTE_ROUND',r.round_id,p.newapi_user_id,NULL,r.game_slug,'ROULETTE',f.first_at,r.ended_at,
  CASE WHEN r.state='CANCELLED' THEN 'REFUNDED' WHEN r.state='FINISHED' THEN CASE WHEN f.paid-f.net>0 THEN 'WIN' WHEN f.paid-f.net<0 THEN 'LOSS' ELSE 'BREAK_EVEN' END END,
  CASE r.state WHEN 'FINISHED' THEN 'SETTLED' WHEN 'CANCELLED' THEN 'REFUNDED' WHEN 'NEEDS_REVIEW' THEN 'RECOVERING' ELSE 'PROCESSING' END,
  f.net,CASE WHEN r.state IN('FINISHED','CANCELLED') THEN f.paid END,
  CASE WHEN r.state IN('FINISHED','CANCELLED') THEN f.paid-f.net END,NULL,NULL,NULL,
  r.title_snapshot,NULL,NULL,p.display_name_snapshot
 FROM roulette.rounds r JOIN roulette.participants p USING(round_id)
 CROSS JOIN LATERAL (SELECT min(created_at) FILTER(WHERE kind='ESCROW') first_at,
  coalesce(sum(amount_units::numeric) FILTER(WHERE kind='ESCROW'),0)-coalesce(sum(amount_units::numeric) FILTER(WHERE kind IN('REFUND','VOID_REFUND')),0) net,
  coalesce(sum(amount_units::numeric) FILTER(WHERE kind='PAYOUT'),0) paid
  FROM roulette.funding WHERE round_id=r.round_id AND newapi_user_id=p.newapi_user_id) f
 WHERE f.first_at IS NOT NULL
)
SELECT r.*,s.snapshot_id,
 md5((to_jsonb(r)-ARRAY['current_game_title','current_table_name','current_actor_name'])::text) AS source_version
FROM source_rows r LEFT JOIN games.history_display_snapshots s
 USING(record_type,source_id,newapi_user_id);

CREATE OR REPLACE FUNCTION games.history_ingest_batch(p_source text,p_limit integer) RETURNS jsonb
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog SET TimeZone='UTC' AS $$
DECLARE source_no integer; c games.history_ingestion_cursors; r record;
 fresh uuid[]; missing uuid[]; pending uuid[]; selected uuid[]; written integer:=0;
BEGIN
 source_no:=array_position(ARRAY['DIRECT_PLAY_ROUND','POKER_SESSION','POKER_HAND','ROULETTE_ROUND'],p_source);
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

CREATE OR REPLACE FUNCTION games.history_list(p_user bigint,q jsonb) RETURNS jsonb
 LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog SET TimeZone='UTC' AS $$
DECLARE n integer:=coalesce((q->>'limit')::integer,50); output jsonb;
BEGIN
 IF p_user IS NULL OR p_user<=0 OR q IS NULL OR jsonb_typeof(q)<>'object' OR n<1 OR n>100
  OR EXISTS(SELECT 1 FROM jsonb_object_keys(q) k WHERE k NOT IN('record_type','mode','game_slug','time_from','time_to',
   'result','status','id','parent_source_id','limit','before_time','before_type','before_id'))
  OR (q ? 'record_type' AND q->>'record_type' NOT IN('DIRECT_PLAY_ROUND','POKER_SESSION','POKER_HAND','ROULETTE_ROUND'))
  OR (q ? 'mode' AND q->>'mode' NOT IN('DIRECT_PLAY','POKER','ROULETTE'))
  OR (q ? 'result' AND q->>'result' NOT IN('WIN','LOSS','BREAK_EVEN','CANCELLED','REFUNDED'))
  OR (q ? 'status' AND q->>'status' NOT IN('PROCESSING','SETTLED','RECOVERING','CANCELLED','REFUNDED')) THEN
  RAISE EXCEPTION 'HISTORY_QUERY_INVALID' USING ERRCODE='22023';
 END IF;
 WITH bounded AS (
  SELECT i.* FROM games.history_index i WHERE i.newapi_user_id=p_user
   AND CASE WHEN q ? 'record_type' THEN i.record_type=q->>'record_type'
    WHEN q ? 'parent_source_id' THEN i.record_type='POKER_HAND' ELSE i.record_type IN('DIRECT_PLAY_ROUND','POKER_SESSION','ROULETTE_ROUND') END
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
-- Source-authorized read capabilities; the immutable 0021 snapshot/index stay unchanged.
CREATE OR REPLACE FUNCTION games.history_record_snapshot(p_user bigint,p_type text,p_id uuid) RETURNS jsonb
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog SET TimeZone='UTC' AS $$
DECLARE owned boolean; result jsonb;
BEGIN
 IF p_user<=0 OR p_user IS NULL OR p_id IS NULL THEN RAISE EXCEPTION 'HISTORY_INVALID' USING ERRCODE='22023'; END IF;
 CASE p_type
 WHEN 'DIRECT_PLAY_ROUND' THEN SELECT EXISTS(SELECT 1 FROM games.game_rounds WHERE round_id=p_id AND newapi_user_id=p_user) INTO owned;
 WHEN 'POKER_SESSION' THEN SELECT EXISTS(SELECT 1 FROM poker.sessions WHERE session_id=p_id AND newapi_user_id=p_user) INTO owned;
 WHEN 'POKER_HAND' THEN SELECT EXISTS(SELECT 1 FROM poker.hand_participants WHERE hand_id=p_id AND newapi_user_id=p_user) INTO owned;
 WHEN 'ROULETTE_ROUND' THEN SELECT EXISTS(SELECT 1 FROM roulette.funding WHERE round_id=p_id AND newapi_user_id=p_user AND kind='ESCROW') INTO owned;
 ELSE RAISE EXCEPTION 'HISTORY_INVALID' USING ERRCODE='22023';
 END CASE;
 IF NOT owned THEN RETURN NULL; END IF;
 SELECT jsonb_build_object('snapshot_id',s.snapshot_id,'game_slug',s.game_slug,'game_title',s.game_title,
  'table_id',s.table_id,'table_name',s.table_name,'actor_display_name',s.actor_display_name,
  'metadata_origin',s.metadata_origin,'captured_at',s.captured_at) INTO result
 FROM games.history_display_snapshots s WHERE s.newapi_user_id=p_user AND s.record_type=p_type AND s.source_id=p_id;
 IF NOT FOUND THEN RAISE EXCEPTION 'HISTORY_SOURCE_UNAVAILABLE'; END IF;
 RETURN result;
END $$;

CREATE OR REPLACE FUNCTION economy.history_transaction_read(p_user bigint,p_id uuid) RETURNS jsonb
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog SET TimeZone='UTC' AS $$
DECLARE t record; effects jsonb; links jsonb:='[]'; n integer; refs integer; invalid boolean;
 first_leg integer; last_leg integer; assets integer; debit numeric; credit numeric;
 source_type text; source_id uuid; expected numeric; expected_ledger uuid;
BEGIN
 IF p_user<=0 OR p_user IS NULL OR p_id IS NULL THEN RAISE EXCEPTION 'HISTORY_INVALID' USING ERRCODE='22023'; END IF;
 SELECT transaction_id,newapi_user_id,biz_type,biz_id,operation_type,status,created_at,confirmed_at INTO t
 FROM economy.asset_transactions WHERE transaction_id=p_id AND newapi_user_id=p_user;
 IF NOT FOUND THEN RETURN NULL; END IF;
 -- Inspect all legs first: an owner filter must not hide a corrupt foreign leg.
 SELECT count(*),min(l.leg_no),max(l.leg_no),count(DISTINCT l.asset_type),
  min(l.delta_units) FILTER(WHERE l.leg_no=1),min(l.delta_units) FILTER(WHERE l.leg_no=2),
  coalesce(bool_or(l.newapi_user_id<>p_user OR l.biz_type<>t.biz_type OR l.biz_id<>t.biz_id
   OR l.entry_type<>t.operation_type OR l.delta_units=0 OR l.balance_before_units<0 OR l.balance_after_units<0
   OR l.balance_after_units::numeric<>l.balance_before_units::numeric+l.delta_units::numeric
   OR l.asset_type NOT IN('AVAILABLE_CHIPS','RESERVE_API_CREDIT')),false),
  coalesce(jsonb_agg(jsonb_build_object('ledger_id',l.ledger_entry_id,'leg_no',l.leg_no,'asset',l.asset_type,
   'delta_units',l.delta_units::text,'balance_before_units',l.balance_before_units::text,
   'balance_after_units',l.balance_after_units::text) ORDER BY l.leg_no),'[]')
 INTO n,first_leg,last_leg,assets,debit,credit,invalid,effects
 FROM economy.wallet_ledger l WHERE l.transaction_id=p_id;
 IF invalid OR n>2 OR (n>0 AND (first_leg<>1 OR last_leg<>n OR assets<>n))
  OR (t.operation_type='LOCAL_EXCHANGE' AND (n<>2 OR debit>=0 OR credit<>-debit))
  OR (t.operation_type<>'LOCAL_EXCHANGE' AND n>1) THEN RAISE EXCEPTION 'HISTORY_SOURCE_UNAVAILABLE'; END IF;
 -- Each confirmed domain transaction has exactly one formal source association.
 WITH sources AS (
  SELECT 'DIRECT_PLAY_ROUND'::text kind,r.round_id id,r.newapi_user_id uid,
   CASE WHEN r.wager_transaction_id=p_id THEN -(r.total_stake_units::numeric-coalesce((SELECT sum(a.additional_stake_units::numeric) FROM games.round_actions a WHERE a.round_id=r.round_id),0)) ELSE r.total_payout_units::numeric END delta,
   CASE WHEN r.wager_transaction_id=p_id THEN 'GAME_WAGER' ELSE 'GAME_SETTLEMENT' END biz,
   CASE WHEN r.wager_transaction_id=p_id THEN 'GAME_WAGER' ELSE 'GAME_PAYOUT' END operation,r.round_id::text business_id,
   NULL::uuid ledger,
   r.wager_transaction_id IS DISTINCT FROM r.settlement_transaction_id
    AND (r.wager_transaction_id=p_id OR r.state='SETTLED')
    AND NOT EXISTS(SELECT 1 FROM games.round_actions a WHERE a.round_id=r.round_id AND (a.newapi_user_id<>r.newapi_user_id OR (a.additional_stake_units>0)<>(a.stake_transaction_id IS NOT NULL))) valid
  FROM games.game_rounds r WHERE p_id IN(r.wager_transaction_id,r.settlement_transaction_id)
  UNION ALL
  SELECT 'DIRECT_PLAY_ROUND',r.round_id,a.newapi_user_id,-a.additional_stake_units::numeric,
   'GAME_ADDITIONAL_WAGER','GAME_ADDITIONAL_WAGER',r.round_id::text||':'||a.action_id::text,NULL::uuid,
   r.newapi_user_id=a.newapi_user_id AND a.additional_stake_units>0
  FROM games.round_actions a JOIN games.game_rounds r USING(round_id) WHERE a.stake_transaction_id=p_id
  UNION ALL
  SELECT 'POKER_SESSION',f.session_id,f.newapi_user_id,
   CASE WHEN f.kind='CASH_OUT' THEN f.amount_units::numeric ELSE -f.amount_units::numeric END,
   'POKER_FUNDING','POKER_'||f.kind,
   'poker_'||CASE f.kind WHEN 'BUY_IN' THEN 'buyin:'||f.session_id::text WHEN 'CASH_OUT' THEN 'cashout:'||f.session_id::text WHEN 'REBUY' THEN 'rebuy:'||f.funding_operation_id::text ELSE 'topup:'||f.funding_operation_id::text END,
   f.confirmed_ledger_id,
   f.state='CONFIRMED' AND s.newapi_user_id=f.newapi_user_id AND s.table_id=f.table_id AND s.seat_no=f.seat_no
    AND (f.kind<>'CASH_OUT' OR (s.state='SETTLED' AND s.final_cashout_units=f.amount_units))
    AND ((f.amount_units=0 AND f.kind='CASH_OUT' AND f.confirmed_ledger_id IS NULL) OR (f.amount_units>0 AND f.confirmed_ledger_id IS NOT NULL))
  FROM poker.funding_operations f LEFT JOIN poker.sessions s USING(session_id) WHERE f.confirmed_transaction_id=p_id
 UNION ALL
 SELECT 'ROULETTE_ROUND',f.round_id,f.newapi_user_id,
  CASE WHEN f.kind='ESCROW' THEN -f.amount_units::numeric ELSE f.amount_units::numeric END,
  'ROULETTE_FUNDING','ROULETTE_'||f.kind,
  'roulette/'||f.round_id::text||'/'||f.newapi_user_id::text||'/'||f.ready_cycle::text||'/'||f.kind,f.ledger_id,
  p.newapi_user_id IS NOT NULL AND f.ready_cycle<=p.ready_cycle AND f.biz_id='roulette/'||f.round_id::text||'/'||f.newapi_user_id::text||'/'||f.ready_cycle::text||'/'||f.kind AND
  CASE WHEN f.kind='PAYOUT' THEN r.state='FINISHED' AND EXISTS(SELECT 1 FROM jsonb_array_elements(r.outcome->'awards') a WHERE a->>'user_id'=f.newapi_user_id::text AND (a->>'amount_units')::numeric=f.amount_units)
   WHEN f.kind='VOID_REFUND' THEN r.state='CANCELLED' AND r.void_outcome IS NOT NULL AND f.amount_units=r.stake_units
   ELSE f.amount_units=r.stake_units END
 FROM roulette.funding f JOIN roulette.rounds r USING(round_id) LEFT JOIN roulette.participants p USING(round_id,newapi_user_id) WHERE f.transaction_id=p_id
 )
 SELECT count(*),min(kind),min(id::text)::uuid,min(delta),min(ledger::text)::uuid,
  coalesce(bool_or(uid<>p_user OR biz<>t.biz_type OR operation<>t.operation_type OR business_id<>t.biz_id OR NOT coalesce(valid,false)),false)
 INTO refs,source_type,source_id,expected,expected_ledger,invalid FROM sources;
 IF invalid OR refs>1 OR (t.operation_type IN('GAME_WAGER','GAME_ADDITIONAL_WAGER','GAME_PAYOUT','POKER_BUY_IN','POKER_TOP_UP','POKER_REBUY','POKER_CASH_OUT','ROULETTE_ESCROW','ROULETTE_REFUND','ROULETTE_PAYOUT','ROULETTE_VOID_REFUND') AND refs<>1)
  OR (refs=1 AND ((t.operation_type IN('GAME_WAGER','GAME_ADDITIONAL_WAGER','POKER_BUY_IN','POKER_TOP_UP','POKER_REBUY','ROULETTE_ESCROW') AND expected>=0) OR (t.operation_type IN('GAME_PAYOUT','POKER_CASH_OUT','ROULETTE_REFUND','ROULETTE_PAYOUT','ROULETTE_VOID_REFUND') AND expected<0)))
  OR (n=0 AND (refs<>1 OR expected<>0 OR t.operation_type NOT IN('GAME_PAYOUT','POKER_CASH_OUT')))
  OR (refs=1 AND ((expected=0 AND n<>0) OR (expected<>0 AND (n<>1 OR debit<>expected OR effects->0->>'asset'<>'AVAILABLE_CHIPS'))))
  OR (expected_ledger IS NOT NULL AND (n<>1 OR effects->0->>'ledger_id'<>expected_ledger::text))
 THEN RAISE EXCEPTION 'HISTORY_SOURCE_UNAVAILABLE'; END IF;
 IF refs=1 THEN links:=jsonb_build_array(jsonb_build_object('record_type',source_type,'source_id',source_id)); END IF;
 RETURN jsonb_build_object('id',t.transaction_id,'kind',t.operation_type,'status',t.status,
  'created_at',t.created_at,'confirmed_at',t.confirmed_at,'effects',effects,'links',links);
END $$;
REVOKE ALL ON FUNCTION games.history_record_snapshot(bigint,text,uuid),economy.history_transaction_read(bigint,uuid) FROM PUBLIC;
CREATE OR REPLACE FUNCTION games.ops_activity_read()
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
  SELECT * FROM direct_activity UNION ALL SELECT * FROM poker_activity UNION ALL SELECT game_slug,count(*) FILTER(WHERE created_at>=statement_timestamp()-interval '24 hours'),count(*) FILTER(WHERE state='NEEDS_REVIEW') FROM roulette.rounds GROUP BY game_slug
 )
 SELECT r.game_slug,coalesce(a.rounds_24h,0),coalesce(a.needs_review_rounds,0)
 FROM games.game_registry r LEFT JOIN activity a USING(game_slug)
 ORDER BY r.sort_order,r.game_slug
$$;
ALTER TABLE audit.record_access_events DROP CONSTRAINT record_access_events_record_type_check;
ALTER TABLE audit.record_access_events ADD CHECK(record_type IN('HISTORY_LIST','DIRECT_PLAY_ROUND','POKER_SESSION','POKER_HAND','TRANSACTION','ROULETTE_ROUND'));
