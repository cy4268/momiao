-- Keep the existing function signature and EXECUTE grants; p_source is no longer used.
-- Historical and new attribution rows live in the same local table.
CREATE OR REPLACE FUNCTION rankings.build_rp_entries(p_snapshot uuid,p_source uuid) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog SET TimeZone='UTC' AS $$
DECLARE snap rankings.snapshots; changed bigint;
BEGIN
 SELECT * INTO STRICT snap FROM rankings.snapshots WHERE snapshot_id=p_snapshot AND domain='RP' AND status='READY';

WITH requests AS (
 SELECT DISTINCT ON (newapi_user_id,logical_request_id) * FROM economy.request_attributions
 ORDER BY newapi_user_id,logical_request_id,imported_at DESC,source_instance_id DESC
), by_model AS (
 SELECT newapi_user_id,request_model_id_snapshot model_id,
 coalesce(nullif((array_agg(request_model_name_snapshot ORDER BY completed_at DESC,source_event_id DESC))[1],''),'未指定模型') model_name,
 count(*) FILTER(WHERE final_status='SUCCESS')::numeric calls,
 count(*) FILTER(WHERE final_status IN('ERROR','CANCELLED_POST_UPSTREAM'))::numeric errors,
 sum(charged_raw_quota)::numeric credits
 FROM requests
 WHERE key_purpose_snapshot='ROLEPLAY' AND entered_model_flow
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
