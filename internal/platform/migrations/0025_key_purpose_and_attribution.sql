-- Platform owns purpose intent and immutable history. Native owns the mirror
-- used at request time; the outbox makes the cross-process boundary explicit.
CREATE TABLE economy.api_key_purposes (
 token_id bigint PRIMARY KEY CHECK(token_id>0 AND token_id<=2147483647),
 newapi_user_id bigint NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
 purpose text NOT NULL CHECK(purpose IN('GENERAL','ROLEPLAY')),
 version bigint NOT NULL CHECK(version>0),
 effective_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(newapi_user_id,token_id),
 UNIQUE(token_id,version)
);
CREATE TABLE economy.api_key_purpose_events (
 operation_id uuid PRIMARY KEY,
 newapi_user_id bigint NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
 token_id bigint NOT NULL CHECK(token_id>0 AND token_id<=2147483647),
 expected_version bigint NOT NULL CHECK(expected_version>=0),
 version bigint NOT NULL CHECK(version=expected_version+1),
 purpose text NOT NULL CHECK(purpose IN('GENERAL','ROLEPLAY')),
 effective_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(token_id,version),
 UNIQUE(operation_id,newapi_user_id,token_id)
);
CREATE TABLE platform_meta.api_key_purpose_outbox (
 operation_id uuid PRIMARY KEY REFERENCES economy.api_key_purpose_events ON DELETE RESTRICT,
 newapi_user_id bigint NOT NULL,
 token_id bigint NOT NULL,
 status text NOT NULL DEFAULT 'PENDING' CHECK(status IN('PENDING','SYNCED','REJECTED')),
 attempt_count integer NOT NULL DEFAULT 0 CHECK(attempt_count>=0),
 next_attempt_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 last_error_code text NOT NULL DEFAULT '',
 synced_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(operation_id,newapi_user_id,token_id)
  REFERENCES economy.api_key_purpose_events(operation_id,newapi_user_id,token_id) ON DELETE RESTRICT,
 CHECK((status='SYNCED')=(synced_at IS NOT NULL))
);
CREATE INDEX api_key_purpose_outbox_pending
 ON platform_meta.api_key_purpose_outbox(next_attempt_at,operation_id) WHERE status='PENDING';
CREATE TRIGGER api_key_purpose_events_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
 ON economy.api_key_purpose_events FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();

-- One row is one finalized logical request. Provider retries are captured by
-- provider_attempt_count and never produce additional logical request rows.
CREATE TABLE economy.request_attributions (
 source_instance_id uuid NOT NULL,
 source_event_id bigint NOT NULL CHECK(source_event_id>0),
 logical_request_id text NOT NULL CHECK(octet_length(logical_request_id) BETWEEN 1 AND 128),
 newapi_user_id bigint NOT NULL CHECK(newapi_user_id>0 AND newapi_user_id<=2147483647),
 token_id bigint NOT NULL CHECK(token_id>0 AND token_id<=2147483647),
 key_purpose_snapshot text NOT NULL CHECK(key_purpose_snapshot IN('GENERAL','ROLEPLAY','UNCLASSIFIED')),
 purpose_version_snapshot bigint NOT NULL CHECK(purpose_version_snapshot>=0),
 request_model_id_snapshot text NOT NULL CHECK(octet_length(request_model_id_snapshot) BETWEEN 1 AND 128),
 request_model_name_snapshot text NOT NULL CHECK(octet_length(request_model_name_snapshot) BETWEEN 1 AND 128),
 request_kind text NOT NULL CHECK(octet_length(request_kind) BETWEEN 1 AND 64),
 entered_model_flow boolean NOT NULL,
 provider_attempt_count integer NOT NULL CHECK(provider_attempt_count>=0),
 final_status text NOT NULL CHECK(final_status IN('SUCCESS','ERROR','CANCELLED_PRE_UPSTREAM','CANCELLED_POST_UPSTREAM')),
 error_category text NOT NULL CHECK(octet_length(error_category)<=64),
 charged_raw_quota bigint NOT NULL CHECK(charged_raw_quota>=0 AND charged_raw_quota<=2147483647),
 requested_at timestamptz NOT NULL,
 completed_at timestamptz NOT NULL,
 imported_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(source_instance_id,source_event_id),
 UNIQUE(source_instance_id,logical_request_id),
 CHECK((key_purpose_snapshot='UNCLASSIFIED')=(purpose_version_snapshot=0)),
 CHECK(entered_model_flow=(provider_attempt_count>0)),
 CHECK(requested_at<=completed_at),
 CHECK((final_status='SUCCESS' AND error_category='') OR (final_status<>'SUCCESS' AND error_category<>''))
);
CREATE INDEX request_attributions_user_usage
 ON economy.request_attributions(source_instance_id,newapi_user_id,requested_at DESC,source_event_id DESC);
CREATE INDEX request_attributions_rp_aggregate
 ON economy.request_attributions(source_instance_id,requested_at,newapi_user_id,request_model_id_snapshot)
 WHERE key_purpose_snapshot='ROLEPLAY' AND entered_model_flow;
CREATE TRIGGER request_attributions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
 ON economy.request_attributions FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();

CREATE TABLE platform_meta.attribution_checkpoints (
 source_instance_id uuid PRIMARY KEY,
 last_cursor bigint NOT NULL DEFAULT 0 CHECK(last_cursor>=0),
 last_success_at timestamptz,
 source_observed_at timestamptz,
 fully_caught_up boolean NOT NULL DEFAULT false,
 last_error_code text NOT NULL DEFAULT '',
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK((last_success_at IS NULL)=(source_observed_at IS NULL)),
 CHECK(NOT fully_caught_up OR last_success_at IS NOT NULL)
);

REVOKE ALL ON economy.api_key_purposes,economy.api_key_purpose_events,
 platform_meta.api_key_purpose_outbox,economy.request_attributions,
 platform_meta.attribution_checkpoints FROM PUBLIC;

-- Frozen V1 policy: Hourly is 100 Reserve per Shanghai natural hour, at most
-- 24 successful claims per Shanghai day. Relief is 300 Reserve below 10 total
-- credits with a rolling four-hour cooldown after a successful claim.
CREATE TABLE rewards.hourly_claims (
 newapi_user_id bigint NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
 reward_hour timestamptz NOT NULL,
 business_date date NOT NULL,
 policy_version integer NOT NULL CHECK(policy_version=1),
 amount_units bigint NOT NULL CHECK(amount_units=50000000),
 asset_type text NOT NULL CHECK(asset_type='RESERVE_API_CREDIT'),
 transaction_id uuid NOT NULL UNIQUE REFERENCES economy.asset_transactions ON DELETE RESTRICT,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(newapi_user_id,reward_hour)
);
CREATE INDEX hourly_claims_user_day ON rewards.hourly_claims(newapi_user_id,business_date);
CREATE TRIGGER hourly_claims_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
 ON rewards.hourly_claims FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();

CREATE TABLE rewards.relief_claims (
 relief_claim_id uuid PRIMARY KEY,
 newapi_user_id bigint NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
 policy_version integer NOT NULL CHECK(policy_version=1),
 total_assets_units bigint NOT NULL CHECK(total_assets_units>=0 AND total_assets_units<5000000),
 active_quota_units bigint NOT NULL CHECK(active_quota_units>=0),
 reserve_units bigint NOT NULL CHECK(reserve_units>=0),
 available_chips_units bigint NOT NULL CHECK(available_chips_units>=0),
 poker_stack_units bigint NOT NULL CHECK(poker_stack_units>=0),
 poker_pot_units bigint NOT NULL CHECK(poker_pot_units>=0),
 quota_transfer_in_flight_units bigint NOT NULL CHECK(quota_transfer_in_flight_units>=0),
 assets_observed_at timestamptz NOT NULL,
 native_observed_at timestamptz NOT NULL,
 amount_units bigint NOT NULL CHECK(amount_units=150000000),
 asset_type text NOT NULL CHECK(asset_type='RESERVE_API_CREDIT'),
 transaction_id uuid NOT NULL UNIQUE REFERENCES economy.asset_transactions ON DELETE RESTRICT,
 claimed_at timestamptz NOT NULL,
 cooldown_until timestamptz NOT NULL,
 CHECK(total_assets_units=active_quota_units+reserve_units+available_chips_units+
  poker_stack_units+poker_pot_units+quota_transfer_in_flight_units),
 CHECK(assets_observed_at<=claimed_at),
 CHECK(cooldown_until=claimed_at+interval '4 hours')
);
CREATE INDEX relief_claims_user_time ON rewards.relief_claims(newapi_user_id,claimed_at DESC,relief_claim_id DESC);
CREATE TRIGGER relief_claims_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
 ON rewards.relief_claims FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();

REVOKE ALL ON rewards.hourly_claims,rewards.relief_claims FROM PUBLIC;

-- Request-time refill planning is serialized before a target effect is made.
-- The immutable plan binds request semantics and the exact watermark snapshot
-- to the durable transfer without changing the shared transfer state machine.
ALTER TABLE economy.quota_transfers ADD UNIQUE(transfer_id,newapi_user_id);
CREATE TABLE economy.active_quota_refill_plans (
 transfer_id uuid PRIMARY KEY,
 newapi_user_id bigint NOT NULL,
 request_id_hash bytea NOT NULL CHECK(octet_length(request_id_hash)=32),
 required_raw_quota bigint NOT NULL CHECK(required_raw_quota>0 AND required_raw_quota<=2147483647),
 observed_active_raw_quota bigint NOT NULL CHECK(observed_active_raw_quota>=0 AND observed_active_raw_quota<=2147483647),
 low_watermark bigint NOT NULL CHECK(low_watermark>=0),
 target_watermark bigint NOT NULL,
 max_active_buffer bigint NOT NULL CHECK(max_active_buffer<=2147483647),
 desired_active_raw_quota bigint NOT NULL,
 refill_raw_quota bigint NOT NULL CHECK(refill_raw_quota>0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(transfer_id,newapi_user_id)
  REFERENCES economy.quota_transfers(transfer_id,newapi_user_id) ON DELETE RESTRICT,
 UNIQUE(newapi_user_id,request_id_hash),
 CHECK(low_watermark<target_watermark AND target_watermark<=max_active_buffer),
 CHECK(required_raw_quota<=desired_active_raw_quota AND desired_active_raw_quota<=max_active_buffer),
 CHECK(observed_active_raw_quota<desired_active_raw_quota),
 CHECK(refill_raw_quota<=desired_active_raw_quota-observed_active_raw_quota)
);
CREATE TRIGGER active_quota_refill_plans_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
 ON economy.active_quota_refill_plans FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
REVOKE ALL ON economy.active_quota_refill_plans FROM PUBLIC;

-- Portal roles never receive SELECT on Poker tables. This aggregate-only
-- definer exposes no table/session/hand identifiers and reports review states
-- or broken participant ownership as unhealthy instead of certifying a total.
CREATE FUNCTION economy.unified_poker_assets_read(bigint)
RETURNS TABLE(stack_units bigint,pot_units bigint,healthy boolean)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog AS $$
 WITH stack AS (
 SELECT coalesce(sum(s.current_stack_units::numeric),0) AS amount,
   count(*) FILTER (WHERE s.state='NEEDS_REVIEW' OR r.state='NEEDS_REVIEW') AS review_count
  FROM poker.sessions s
  LEFT JOIN poker.recovery_state r ON r.table_id=s.table_id
  WHERE s.newapi_user_id=$1 AND s.state<>'SETTLED'
 ), committed AS (
  SELECT coalesce(sum(p.total_committed_units::numeric),0) AS amount,
   count(*) FILTER (WHERE s.session_id IS NULL OR s.newapi_user_id<>p.newapi_user_id OR s.state='SETTLED') AS invalid_count
  FROM poker.hand_participants p
  JOIN poker.hands h ON h.hand_id=p.hand_id AND h.state<>'SETTLED'
  LEFT JOIN poker.sessions s ON s.session_id=p.session_id
  WHERE p.newapi_user_id=$1
 ), checked AS (
  SELECT stack.amount AS stack_amount,committed.amount AS pot_amount,
   $1>0 AND $1<=2147483647 AND stack.review_count=0 AND committed.invalid_count=0
    AND stack.amount>=0 AND stack.amount<=9223372036854775807::numeric
    AND committed.amount>=0 AND committed.amount<=9223372036854775807::numeric AS ok
  FROM stack CROSS JOIN committed
 )
 SELECT CASE WHEN ok THEN stack_amount::bigint ELSE 0 END,
  CASE WHEN ok THEN pot_amount::bigint ELSE 0 END,ok
 FROM checked
$$;
REVOKE ALL ON FUNCTION economy.unified_poker_assets_read(bigint) FROM PUBLIC;

-- Ops may return a reviewed transfer to the existing worker without changing
-- its identity or inventing a second Native operation. All other historical
-- fields remain immutable and the original PENDING terminal transitions stay
-- unchanged.
CREATE OR REPLACE FUNCTION economy.guard_quota_transfer() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF (NEW.transfer_id,NEW.newapi_user_id,NEW.request_key_hash,NEW.amount_units,NEW.created_at)
  IS DISTINCT FROM
  (OLD.transfer_id,OLD.newapi_user_id,OLD.request_key_hash,OLD.amount_units,OLD.created_at)
  OR NOT (
   OLD.status='PENDING' AND NEW.status IN ('CONFIRMED','REFUNDED','NEEDS_REVIEW')
   OR OLD.status='NEEDS_REVIEW' AND NEW.status='PENDING'
    AND NEW.reason='ADMIN_RESUME' AND NEW.native_before IS NULL AND NEW.native_after IS NULL
  ) THEN
  RAISE EXCEPTION 'invalid transfer transition' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END;
$$;
