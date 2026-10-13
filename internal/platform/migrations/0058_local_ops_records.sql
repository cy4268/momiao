-- Global Ops reads reuse the personal projection. Only the platform role gets
-- this capability; the existing History reader keeps its positive-user wrapper.
CREATE INDEX history_global_order ON games.history_index
 (occurred_at DESC,record_type COLLATE "C",source_id DESC,newapi_user_id);

CREATE FUNCTION games.ops_history_list(p_user bigint,q jsonb) RETURNS jsonb
 LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog SET TimeZone='UTC' AS $$
DECLARE n integer:=coalesce((q->>'limit')::integer,50); output jsonb;
BEGIN
 IF p_user IS NULL OR p_user<0 OR q IS NULL OR jsonb_typeof(q)<>'object' OR n<1 OR n>100
  OR EXISTS(SELECT 1 FROM jsonb_object_keys(q) k WHERE k NOT IN('record_type','mode','game_slug','time_from','time_to',
   'result','status','id','parent_source_id','limit','before_time','before_type','before_id','before_user'))
  OR (q ? 'record_type' AND q->>'record_type' NOT IN('DIRECT_PLAY_ROUND','POKER_SESSION','POKER_HAND','ROULETTE_ROUND'))
  OR (q ? 'mode' AND q->>'mode' NOT IN('DIRECT_PLAY','POKER','ROULETTE'))
  OR (q ? 'result' AND q->>'result' NOT IN('WIN','LOSS','BREAK_EVEN','CANCELLED','REFUNDED'))
  OR (q ? 'status' AND q->>'status' NOT IN('PROCESSING','SETTLED','RECOVERING','CANCELLED','REFUNDED')) THEN
  RAISE EXCEPTION 'HISTORY_QUERY_INVALID' USING ERRCODE='22023';
 END IF;
 WITH bounded AS (
  SELECT i.* FROM games.history_index i WHERE (p_user=0 OR i.newapi_user_id=p_user)
   AND CASE WHEN q ? 'record_type' THEN i.record_type=q->>'record_type'
    WHEN q ? 'parent_source_id' THEN i.record_type='POKER_HAND' ELSE i.record_type IN('DIRECT_PLAY_ROUND','POKER_SESSION','ROULETTE_ROUND') END
   AND (NOT q ? 'mode' OR i.mode=q->>'mode') AND (NOT q ? 'game_slug' OR i.game_slug=q->>'game_slug')
   AND (NOT q ? 'result' OR i.result_class=q->>'result') AND (NOT q ? 'status' OR i.display_status=q->>'status')
   AND (NOT q ? 'id' OR i.source_id=(q->>'id')::uuid)
   AND (NOT q ? 'parent_source_id' OR i.parent_source_id=(q->>'parent_source_id')::uuid)
   AND (NOT q ? 'time_from' OR i.occurred_at>=(q->>'time_from')::timestamptz)
   AND (NOT q ? 'time_to' OR i.occurred_at<(q->>'time_to')::timestamptz)
   AND (NOT q ? 'before_time' OR i.occurred_at<(q->>'before_time')::timestamptz
    OR (i.occurred_at=(q->>'before_time')::timestamptz AND (
     i.record_type COLLATE "C">q->>'before_type'
     OR (i.record_type=q->>'before_type' AND (
      i.source_id<(q->>'before_id')::uuid
      OR (i.source_id=(q->>'before_id')::uuid AND i.newapi_user_id>coalesce((q->>'before_user')::bigint,p_user))
     ))
    )))
  ORDER BY i.occurred_at DESC,i.record_type COLLATE "C",i.source_id DESC,i.newapi_user_id LIMIT n+1
 ), page AS (
  SELECT * FROM bounded ORDER BY occurred_at DESC,record_type COLLATE "C",source_id DESC,newapi_user_id LIMIT n
 ), options AS (
  SELECT DISTINCT ON(game_slug) game_slug,game_title,retired FROM (
   SELECT game_slug,title AS game_title,publication_state='RETIRED' AS retired,0 AS priority,NULL::timestamptz AS at
    FROM games.game_registry WHERE publication_state IN('PUBLISHED','RETIRED')
   UNION ALL SELECT s.game_slug,s.game_title,coalesce(g.publication_state='RETIRED',false),1,s.captured_at
    FROM games.history_display_snapshots s LEFT JOIN games.game_registry g USING(game_slug) WHERE (p_user=0 OR s.newapi_user_id=p_user)
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
  ORDER BY i.occurred_at DESC,i.record_type COLLATE "C",i.source_id DESC,i.newapi_user_id)
  FROM page i JOIN games.history_display_snapshots s USING(record_type,source_id,newapi_user_id)), '[]'::jsonb),
  'has_more',(SELECT count(*)>n FROM bounded),'game_options',coalesce((SELECT jsonb_agg(to_jsonb(o) ORDER BY game_slug) FROM options o),'[]'::jsonb)) INTO output;
 RETURN output;
END $$;
REVOKE ALL ON FUNCTION games.ops_history_list(bigint,jsonb) FROM PUBLIC;

CREATE OR REPLACE FUNCTION games.history_list(p_user bigint,q jsonb) RETURNS jsonb
 LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog SET TimeZone='UTC' AS $$
BEGIN
 IF p_user IS NULL OR p_user<=0 THEN
  RAISE EXCEPTION 'HISTORY_QUERY_INVALID' USING ERRCODE='22023';
 END IF;
 RETURN games.ops_history_list(p_user,q);
END $$;
