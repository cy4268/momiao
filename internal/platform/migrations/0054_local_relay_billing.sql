CREATE TABLE relay.prices (
 model text PRIMARY KEY,
 input numeric NOT NULL DEFAULT 0 CHECK(input>=0),
 output numeric NOT NULL DEFAULT 0 CHECK(output>=0),
 cache_read numeric NOT NULL DEFAULT 0 CHECK(cache_read>=0),
 cache_write numeric NOT NULL DEFAULT 0 CHECK(cache_write>=0),
 cache_write_1h numeric NOT NULL DEFAULT 0 CHECK(cache_write_1h>=0),
 per_request numeric CHECK(per_request>=0),
 -- The existing video model charges by duration, not tokens or a fixed request.
 per_second numeric CHECK(per_second>=0),
 default_seconds integer NOT NULL DEFAULT 0 CHECK(default_seconds>=0),
 CHECK(per_request IS NULL OR per_second IS NULL)
);
REVOKE ALL ON relay.prices FROM PUBLIC;

-- Local identity, key IDs and Reserve use bigint, not native int32 ceilings.
ALTER TABLE economy.request_attributions
 DROP CONSTRAINT request_attributions_newapi_user_id_check,
 DROP CONSTRAINT request_attributions_token_id_check,
 DROP CONSTRAINT request_attributions_charged_raw_quota_check,
 ADD CHECK(newapi_user_id>0),
 ADD CHECK(token_id>0),
 ADD CHECK(charged_raw_quota>=0);

CREATE OR REPLACE FUNCTION rankings.build_rp_entries(p_snapshot uuid,p_source uuid) RETURNS bigint
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
 WHERE (source_instance_id=p_source OR source_instance_id='8be7cf11-88df-46e9-a6ce-e81bf166e1d4'::uuid) AND key_purpose_snapshot='ROLEPLAY' AND entered_model_flow
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
