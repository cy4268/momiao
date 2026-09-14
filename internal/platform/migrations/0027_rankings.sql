-- Public projections contain aggregate values only; source/request IDs never
-- enter a public DTO. Activation is an explicit post-cutover deployment setting.
CREATE SCHEMA rankings;
CREATE TABLE rankings.snapshots (
 snapshot_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 domain text NOT NULL CHECK(domain IN('GAMES','RP','ASSETS')),
 period text NOT NULL CHECK(period IN('DAY','WEEK','ALL_TIME','CURRENT')),
 period_start timestamptz NOT NULL,
 period_end timestamptz,
 activation_at timestamptz NOT NULL,
 built_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 source_checked_at timestamptz NOT NULL,
 status text NOT NULL CHECK(status IN('READY','UNAVAILABLE')),
 CHECK(period_end IS NULL OR period_end>period_start)
);
CREATE INDEX rankings_latest ON rankings.snapshots(domain,period,period_start,built_at DESC);
CREATE TABLE rankings.entries (
 snapshot_id uuid NOT NULL REFERENCES rankings.snapshots ON DELETE RESTRICT,
 metric text NOT NULL CHECK(metric IN('TOTAL_ASSETS','GAME_PROFIT','BIGGEST_WIN','TOTAL_WAGERED','POKER_PROFIT','RP_CALLS','RP_ERRORS','RP_CREDITS')),
 model_id text NOT NULL DEFAULT '',
 model_scope text NOT NULL DEFAULT 'ALL' CHECK(model_scope IN('ALL','MODEL')),
 newapi_user_id bigint NOT NULL REFERENCES identity.master_profiles ON DELETE RESTRICT,
 display_name text NOT NULL,
 avatar_id text NOT NULL,
 value numeric(38,0) NOT NULL,
 calls numeric(38,0) NOT NULL DEFAULT 0 CHECK(calls>=0),
 errors numeric(38,0) NOT NULL DEFAULT 0 CHECK(errors>=0),
 credits_units numeric(38,0) NOT NULL DEFAULT 0 CHECK(credits_units>=0),
 models jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(models)='array'),
 PRIMARY KEY(snapshot_id,metric,model_scope,model_id,newapi_user_id)
);
CREATE INDEX rankings_order ON rankings.entries(snapshot_id,metric,model_scope,model_id,value DESC,newapi_user_id);
CREATE TRIGGER ranking_snapshots_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON rankings.snapshots
 FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TRIGGER ranking_entries_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON rankings.entries
 FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
REVOKE ALL ON SCHEMA rankings FROM PUBLIC;
REVOKE ALL ON ALL TABLES IN SCHEMA rankings FROM PUBLIC;

-- Aggregate-only capabilities avoid granting the portal role raw Poker tables
-- or private cross-user History reads. Fixed SQL, no caller-controlled SQL text.
CREATE FUNCTION rankings.build_game_entries(p_snapshot uuid) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog SET TimeZone='UTC' AS $$
DECLARE snap rankings.snapshots; changed bigint;
BEGIN
 SELECT * INTO STRICT snap FROM rankings.snapshots WHERE snapshot_id=p_snapshot AND domain='GAMES' AND status='READY';

WITH facts AS (
 SELECT r.newapi_user_id,r.record_type,r.stake_units,r.net_change_units,
  CASE WHEN r.record_type='POKER_HAND' THEN parent.ended_at ELSE r.ended_at END occurred_at
 FROM games.history_source_rows r
 LEFT JOIN games.history_source_rows parent ON r.record_type='POKER_HAND'
  AND parent.record_type='POKER_SESSION' AND parent.source_id=r.parent_source_id
  AND parent.newapi_user_id=r.newapi_user_id AND parent.display_status='SETTLED'
 WHERE r.display_status='SETTLED'
  AND (r.record_type<>'POKER_HAND' OR parent.source_id IS NOT NULL)
), totals AS (
 SELECT newapi_user_id,
 coalesce(sum(net_change_units) FILTER(WHERE record_type IN('DIRECT_PLAY_ROUND','POKER_SESSION')),0) profit,
 coalesce(max(greatest(net_change_units,0)) FILTER(WHERE record_type IN('DIRECT_PLAY_ROUND','POKER_HAND')),0) biggest,
 coalesce(sum(stake_units) FILTER(WHERE record_type IN('DIRECT_PLAY_ROUND','POKER_HAND')),0) wagered,
 coalesce(sum(net_change_units) FILTER(WHERE record_type='POKER_SESSION'),0) poker_profit
 FROM facts WHERE occurred_at >= greatest(snap.period_start,snap.activation_at)
  AND (snap.period_end IS NULL OR occurred_at<snap.period_end) GROUP BY newapi_user_id
)
INSERT INTO rankings.entries(snapshot_id,metric,model_id,newapi_user_id,display_name,avatar_id,value)
SELECT p_snapshot,m.metric,'',t.newapi_user_id,p.display_name,p.avatar_id,m.value
FROM totals t JOIN identity.master_profiles p USING(newapi_user_id)
CROSS JOIN LATERAL (VALUES ('GAME_PROFIT',t.profit),('BIGGEST_WIN',t.biggest),
 ('TOTAL_WAGERED',t.wagered),('POKER_PROFIT',t.poker_profit)) m(metric,value)
WHERE m.value<>0
;
 GET DIAGNOSTICS changed=ROW_COUNT;
 RETURN changed;
END $$;
CREATE FUNCTION rankings.build_rp_entries(p_snapshot uuid,p_source uuid) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog SET TimeZone='UTC' AS $$
DECLARE snap rankings.snapshots; changed bigint;
BEGIN
 IF p_source IS NULL THEN RAISE EXCEPTION 'RANKINGS_SOURCE_UNAVAILABLE'; END IF;
 SELECT * INTO STRICT snap FROM rankings.snapshots WHERE snapshot_id=p_snapshot AND domain='RP' AND status='READY';

WITH by_model AS (
 SELECT newapi_user_id,request_model_id_snapshot model_id,
 coalesce(nullif((array_agg(request_model_name_snapshot ORDER BY completed_at DESC,source_event_id DESC))[1],''),'未指定模型') model_name,
 count(*) FILTER(WHERE final_status='SUCCESS')::numeric calls,
 count(*) FILTER(WHERE final_status IN('ERROR','CANCELLED_POST_UPSTREAM'))::numeric errors,
 sum(charged_raw_quota)::numeric credits
 FROM economy.request_attributions
 WHERE source_instance_id=p_source AND key_purpose_snapshot='ROLEPLAY' AND entered_model_flow
  AND final_status IN('SUCCESS','ERROR','CANCELLED_POST_UPSTREAM')
  AND requested_at>=greatest(snap.period_start,snap.activation_at)
  AND (snap.period_end IS NULL OR requested_at<snap.period_end)
 GROUP BY newapi_user_id,request_model_id_snapshot
), grouped AS (
 SELECT newapi_user_id,'MODEL'::text model_scope,model_id,calls,errors,credits,
 jsonb_build_array(jsonb_build_object('model_id',model_id,'display_name',model_name,
 'calls',calls::text,'errors',errors::text,'credits_units',credits::text)) models FROM by_model
 UNION ALL
 SELECT newapi_user_id,'ALL','',sum(calls),sum(errors),sum(credits),
 jsonb_agg(jsonb_build_object('model_id',model_id,'display_name',model_name,
 'calls',calls::text,'errors',errors::text,'credits_units',credits::text) ORDER BY model_id COLLATE "C")
 FROM by_model GROUP BY newapi_user_id
)
INSERT INTO rankings.entries(snapshot_id,metric,model_scope,model_id,newapi_user_id,display_name,avatar_id,value,calls,errors,credits_units,models)
SELECT p_snapshot,m.metric,t.model_scope,t.model_id,t.newapi_user_id,p.display_name,p.avatar_id,m.value,t.calls,t.errors,t.credits,t.models
FROM grouped t JOIN identity.master_profiles p USING(newapi_user_id)
CROSS JOIN LATERAL (VALUES ('RP_CALLS',t.calls),('RP_ERRORS',t.errors),('RP_CREDITS',t.credits)) m(metric,value)
WHERE m.value<>0
;
 GET DIAGNOSTICS changed=ROW_COUNT;
 RETURN changed;
END $$;
REVOKE ALL ON FUNCTION rankings.build_game_entries(uuid),rankings.build_rp_entries(uuid,uuid) FROM PUBLIC;
