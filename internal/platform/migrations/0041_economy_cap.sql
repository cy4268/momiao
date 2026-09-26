-- Inactive until an explicitly authorized Ops activation. Prior migrations stay unchanged.
CREATE TABLE economy.policy_versions (
 version text PRIMARY KEY, policy_hash text NOT NULL UNIQUE CHECK(policy_hash ~ '^[0-9a-f]{64}$'),
 canonical_json bytea NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TRIGGER policy_versions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON economy.policy_versions FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TABLE economy.policy_runtime(singleton boolean PRIMARY KEY CHECK(singleton),active_version text REFERENCES economy.policy_versions);
INSERT INTO economy.policy_runtime VALUES(true,NULL);
CREATE TABLE economy.cap_settlements (
 source_kind text NOT NULL, source_id text NOT NULL, newapi_user_id bigint NOT NULL REFERENCES identity.account_refs,
 policy_version text NOT NULL REFERENCES economy.policy_versions,
 gross_payout_units bigint NOT NULL CHECK(gross_payout_units>=0),
 credited_payout_units bigint NOT NULL CHECK(credited_payout_units>=0),
 withheld_units bigint NOT NULL CHECK(withheld_units>=0), actual_net_units bigint NOT NULL,
 receipt jsonb NOT NULL CHECK(jsonb_typeof(receipt)='object'), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(source_kind,source_id,newapi_user_id),
 CHECK(gross_payout_units::numeric=credited_payout_units::numeric+withheld_units::numeric)
);
CREATE TRIGGER cap_settlements_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON economy.cap_settlements FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TABLE economy.reward_request_keys (
 newapi_user_id bigint NOT NULL REFERENCES identity.account_refs,key_hash bytea NOT NULL CHECK(octet_length(key_hash)=32),
 request_hash bytea NOT NULL CHECK(octet_length(request_hash)=32),transaction_id uuid NOT NULL REFERENCES economy.asset_transactions,
 PRIMARY KEY(newapi_user_id,key_hash)
);
CREATE TRIGGER reward_request_keys_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON economy.reward_request_keys FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
ALTER TABLE games.fairness_commitments ADD COLUMN economic_policy_version text REFERENCES economy.policy_versions;
ALTER TABLE games.game_rounds ADD COLUMN economic_policy_version text REFERENCES economy.policy_versions;
ALTER TABLE roulette.rounds ADD COLUMN economic_policy_version text REFERENCES economy.policy_versions;
ALTER TABLE poker.hands ADD COLUMN economic_policy_version text REFERENCES economy.policy_versions;
ALTER TABLE rewards.daily_checkins DROP CONSTRAINT daily_checkins_amount_units_check, ADD CHECK(amount_units BETWEEN 0 AND 250000000);
ALTER TABLE rewards.hourly_claims DROP CONSTRAINT hourly_claims_amount_units_check, ADD CHECK(amount_units BETWEEN 0 AND 50000000);
ALTER TABLE rewards.relief_claims DROP CONSTRAINT relief_claims_amount_units_check, ADD CHECK(amount_units BETWEEN 0 AND 150000000);
ALTER TABLE rewards.registration_issuances DROP CONSTRAINT registration_issuances_amount_units_check, ADD CHECK(amount_units BETWEEN 0 AND 500000000), ALTER COLUMN ledger_entry_id DROP NOT NULL;
ALTER TABLE rewards.relief_claims ADD COLUMN single_player_in_flight_units bigint NOT NULL DEFAULT 0 CHECK(single_player_in_flight_units>=0), ADD COLUMN roulette_escrow_units bigint NOT NULL DEFAULT 0 CHECK(roulette_escrow_units>=0);
ALTER TABLE rewards.relief_claims DROP CONSTRAINT relief_claims_check, ADD CHECK(total_assets_units::numeric=active_quota_units::numeric+reserve_units+available_chips_units+poker_stack_units+poker_pot_units+quota_transfer_in_flight_units+single_player_in_flight_units+roulette_escrow_units);

-- Narrow read functions let the Poker runtime certify assets without private
-- table grants. A fixed search path and no PUBLIC execution keep authority explicit.
CREATE FUNCTION economy.cap_wallets_read(bigint) RETURNS SETOF economy.wallet_balances
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path=pg_catalog AS $$
 SELECT * FROM economy.wallet_balances WHERE newapi_user_id=$1 ORDER BY asset_type FOR UPDATE
$$;
CREATE FUNCTION economy.cap_pending_transfers_read(bigint) RETURNS TABLE(transfer_id uuid,amount_units bigint)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog AS $$
 SELECT transfer_id,amount_units FROM economy.quota_transfers WHERE newapi_user_id=$1 AND status IN('PENDING','NEEDS_REVIEW') ORDER BY transfer_id
$$;
CREATE FUNCTION economy.cap_pending_exchanges_read(bigint) RETURNS SETOF economy.api_chips_exchanges
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog AS $$
 SELECT * FROM economy.api_chips_exchanges WHERE newapi_user_id=$1 AND status NOT IN('CONFIRMED','COMPENSATED','FAILED_NO_EFFECT')
$$;
CREATE FUNCTION economy.cap_local_assets_read(bigint) RETURNS TABLE(single_player_units bigint,roulette_units bigint,healthy boolean)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog AS $$
 WITH solo AS (
  SELECT coalesce(sum(total_stake_units::numeric),0) amount,coalesce(bool_and(recovery_state='NORMAL'),true) ok
  FROM games.game_rounds WHERE newapi_user_id=$1 AND state<>'SETTLED'
 ), wheel AS (
  SELECT coalesce(sum(f.amount),0) amount,coalesce(bool_and(r.state<>'NEEDS_REVIEW' AND f.amount>=0 AND f.amount<=r.escrow_units),true) ok
  FROM roulette.rounds r
  JOIN LATERAL (SELECT sum(CASE WHEN kind='ESCROW' THEN amount_units::numeric ELSE -amount_units::numeric END) amount
    FROM roulette.funding WHERE round_id=r.round_id AND newapi_user_id=$1) f ON f.amount IS NOT NULL
  WHERE r.state NOT IN('FINISHED','CANCELLED')
 ), checked AS (
 SELECT solo.amount a,wheel.amount b,solo.ok AND wheel.ok AND solo.amount BETWEEN 0 AND 9223372036854775807::numeric AND wheel.amount BETWEEN 0 AND 9223372036854775807::numeric ok FROM solo CROSS JOIN wheel
 ) SELECT CASE WHEN ok THEN a::bigint ELSE 0 END,CASE WHEN ok THEN b::bigint ELSE 0 END,ok FROM checked
$$;
REVOKE ALL ON economy.policy_versions,economy.policy_runtime,economy.cap_settlements,economy.reward_request_keys FROM PUBLIC;
REVOKE ALL ON FUNCTION economy.cap_wallets_read(bigint),economy.cap_pending_transfers_read(bigint),economy.cap_pending_exchanges_read(bigint),economy.cap_local_assets_read(bigint) FROM PUBLIC;
INSERT INTO economy.policy_versions(version,policy_hash,canonical_json) VALUES('economy-cap-v1','820a03d141d2d0d0fc7ae9471515c4054f15717b31ba10ec658d23f044bfa227',convert_to('{"asset_cap_units":"50000000000000000","cap_mode":"CLIP_PROFIT","single_player_max_units":"500000000000","version":"economy-cap-v1"}','UTF8'));
CREATE OR REPLACE FUNCTION rewards.check_registration_issuance() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE g rewards.registration_grants%ROWTYPE; i rewards.registration_issuances%ROWTYPE;
BEGIN
 SELECT * INTO g FROM rewards.registration_grants WHERE claim_id=NEW.claim_id;
 SELECT * INTO i FROM rewards.registration_issuances WHERE claim_id=NEW.claim_id;
 IF g.status='CONFIRMED' THEN
  IF i.claim_id IS NULL OR i.transaction_id<>g.transaction_id OR NOT (EXISTS (
   SELECT 1 FROM economy.wallet_ledger l JOIN economy.asset_transactions t USING(transaction_id)
    JOIN identity.native_registration_inbox n ON n.ordinal=g.source_ordinal
   WHERE l.ledger_entry_id=i.ledger_entry_id AND l.transaction_id=i.transaction_id
    AND l.newapi_user_id=g.newapi_user_id AND t.newapi_user_id=g.newapi_user_id
    AND l.biz_type='INITIAL_GRANT_REGISTRATION' AND t.biz_type=l.biz_type
    AND l.biz_id=g.biz_id AND t.biz_id=g.biz_id AND l.leg_no=1
    AND l.entry_type='INITIAL_GRANT_REGISTRATION' AND t.operation_type=l.entry_type
    AND l.asset_type=i.asset_type AND l.delta_units=i.amount_units
    AND t.status='CONFIRMED' AND n.policy_version=i.policy_version
   ) OR (i.amount_units=0 AND i.ledger_entry_id IS NULL AND EXISTS(
    SELECT 1 FROM economy.asset_transactions t JOIN economy.cap_settlements c ON c.source_kind=t.biz_type AND c.source_id=t.biz_id AND c.newapi_user_id=t.newapi_user_id
    JOIN identity.native_registration_inbox n ON n.ordinal=g.source_ordinal
    WHERE t.transaction_id=i.transaction_id AND t.newapi_user_id=g.newapi_user_id AND t.biz_id=g.biz_id AND t.biz_type='INITIAL_GRANT_REGISTRATION'
      AND t.operation_type='INITIAL_GRANT_REGISTRATION' AND t.status='CONFIRMED' AND n.policy_version=i.policy_version
      AND c.gross_payout_units=500000000 AND c.credited_payout_units=0
      AND NOT EXISTS(SELECT 1 FROM economy.wallet_ledger l WHERE l.transaction_id=t.transaction_id)
  ))) THEN RAISE EXCEPTION 'incomplete registration issuance' USING ERRCODE='23514'; END IF;
 ELSIF i.claim_id IS NOT NULL THEN
  RAISE EXCEPTION 'unconfirmed registration issuance' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END;
$$;

-- Raw game results remain unchanged. Only the separate economic receipt and
-- actual wallet/net supply projections use the withheld amount.
ALTER TABLE games.game_rounds ADD COLUMN cap_withheld_units bigint NOT NULL DEFAULT 0 CHECK(cap_withheld_units>=0 AND cap_withheld_units<=total_payout_units);
ALTER TABLE games.game_rounds DROP CONSTRAINT game_round_fast_balance,
 ADD CONSTRAINT game_round_fast_balance CHECK(game_slug='blackjack' OR balance_after_units::numeric=balance_before_units::numeric+net_change_units::numeric-cap_withheld_units::numeric);
CREATE OR REPLACE FUNCTION games.guard_blackjack_round() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.game_slug<>'blackjack' OR OLD.state='SETTLED' OR OLD.recovery_state='NEEDS_REVIEW'
 OR (to_jsonb(NEW)-ARRAY['state','recovery_state','total_stake_units','total_payout_units','net_change_units','common_result','balance_after_units','settlement_transaction_id','settled_at','cap_withheld_units'])
 IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','recovery_state','total_stake_units','total_payout_units','net_change_units','common_result','balance_after_units','settlement_transaction_id','settled_at','cap_withheld_units'])
 OR NEW.total_stake_units<OLD.total_stake_units THEN RAISE EXCEPTION 'immutable round authority' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION games.check_fast_settlement() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c games.fairness_commitments; n bigint; paid bigint; BEGIN
 SELECT * INTO STRICT c FROM games.fairness_commitments WHERE commitment_id=NEW.commitment_id;
 IF c.economic_policy_version IS DISTINCT FROM NEW.economic_policy_version OR c.state<>'REVEALED' OR c.nonce<>NEW.nonce OR c.game_config_hash<>NEW.game_config_hash OR c.game_config_version_id<>NEW.game_config_version_id
 OR c.wager_policy_hash<>NEW.wager_policy_hash OR c.wager_policy_version_id<>NEW.wager_policy_version_id OR c.ruleset_version<>NEW.ruleset_version
 OR c.algorithm_version<>NEW.algorithm_version OR c.fairness_stream_version<>NEW.fairness_stream_version THEN RAISE EXCEPTION 'round commitment mismatch'; END IF;
 SELECT count(*) INTO n FROM economy.wallet_ledger WHERE transaction_id=NEW.wager_transaction_id AND newapi_user_id=NEW.newapi_user_id
  AND asset_type='AVAILABLE_CHIPS' AND entry_type='GAME_WAGER' AND delta_units=-NEW.total_stake_units AND balance_before_units=NEW.balance_before_units;
 IF n<>1 THEN RAISE EXCEPTION 'missing wager ledger'; END IF;
 SELECT count(*),coalesce(sum(delta_units),0) INTO n,paid FROM economy.wallet_ledger WHERE transaction_id=NEW.settlement_transaction_id AND newapi_user_id=NEW.newapi_user_id AND asset_type='AVAILABLE_CHIPS' AND entry_type='GAME_PAYOUT';
 IF paid<>(NEW.total_payout_units-NEW.cap_withheld_units) OR ((NEW.total_payout_units-NEW.cap_withheld_units)>0 AND n<>1) OR ((NEW.total_payout_units-NEW.cap_withheld_units)=0 AND n<>0) THEN RAISE EXCEPTION 'settlement ledger mismatch'; END IF;
 SELECT count(*) INTO n FROM games.supply_events WHERE round_id=NEW.round_id AND amount_units=abs((NEW.net_change_units-NEW.cap_withheld_units))
  AND direction=CASE WHEN (NEW.net_change_units-NEW.cap_withheld_units)>0 THEN 'ISSUE' ELSE 'BURN' END;
 IF ((NEW.net_change_units-NEW.cap_withheld_units)<>0 AND n<>1) OR ((NEW.net_change_units-NEW.cap_withheld_units)=0 AND EXISTS(SELECT 1 FROM games.supply_events WHERE round_id=NEW.round_id)) THEN RAISE EXCEPTION 'net supply mismatch'; END IF;
 IF NEW.game_slug='dice' AND NOT EXISTS(SELECT 1 FROM games.dice_results WHERE round_id=NEW.round_id) THEN RAISE EXCEPTION 'missing dice result'; END IF;
 IF NEW.game_slug='scratch' AND (SELECT count(*) FROM games.scratch_cells WHERE round_id=NEW.round_id)<>9 THEN RAISE EXCEPTION 'missing scratch cells'; END IF;
 IF NEW.game_slug='summon' AND (SELECT count(*) FROM games.summon_draws WHERE round_id=NEW.round_id)<>(CASE WHEN NEW.typed_input->>'mode'='TENFOLD' THEN 10 ELSE 1 END) THEN RAISE EXCEPTION 'missing summon draws'; END IF;
 RETURN NULL;
END $$;
CREATE OR REPLACE FUNCTION games.check_blackjack_persistence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r games.game_rounds; s games.blackjack_round_state; c games.fairness_commitments; n bigint; amount numeric; BEGIN
 SELECT * INTO STRICT r FROM games.game_rounds WHERE round_id=NEW.round_id;
 IF r.game_slug<>'blackjack' THEN RETURN NULL; END IF;
 SELECT * INTO STRICT s FROM games.blackjack_round_state WHERE round_id=r.round_id;
 SELECT * INTO STRICT c FROM games.fairness_commitments WHERE commitment_id=r.commitment_id;
 IF c.economic_policy_version IS DISTINCT FROM r.economic_policy_version OR c.nonce<>r.nonce OR c.game_config_version_id<>r.game_config_version_id OR c.game_config_hash<>r.game_config_hash OR c.wager_policy_version_id<>r.wager_policy_version_id OR c.wager_policy_hash<>r.wager_policy_hash OR c.algorithm_version<>r.algorithm_version OR c.ruleset_version<>r.ruleset_version OR c.fairness_stream_version<>r.fairness_stream_version THEN RAISE EXCEPTION 'blackjack commitment mismatch'; END IF;
 SELECT count(*) INTO n FROM economy.wallet_ledger WHERE transaction_id=r.wager_transaction_id AND newapi_user_id=r.newapi_user_id AND asset_type='AVAILABLE_CHIPS' AND entry_type='GAME_WAGER' AND delta_units=-s.initial_wager_units AND balance_before_units=r.balance_before_units;
 IF n<>1 THEN RAISE EXCEPTION 'blackjack initial funding mismatch'; END IF;
 SELECT count(*),coalesce(sum(additional_stake_units),0) INTO n,amount FROM games.round_actions WHERE round_id=r.round_id;
 IF s.initial_wager_units::numeric+amount<>r.total_stake_units OR s.round_version<>n+1 THEN RAISE EXCEPTION 'blackjack action stake/version mismatch'; END IF;
 IF EXISTS(SELECT 1 FROM games.round_actions a WHERE a.round_id=r.round_id AND (a.newapi_user_id<>r.newapi_user_id OR (a.additional_stake_units>0 AND (SELECT count(*) FROM economy.wallet_ledger l WHERE l.transaction_id=a.stake_transaction_id AND l.newapi_user_id=r.newapi_user_id AND l.asset_type='AVAILABLE_CHIPS' AND l.entry_type='GAME_ADDITIONAL_WAGER' AND l.delta_units=-a.additional_stake_units)<>1))) THEN RAISE EXCEPTION 'blackjack additional funding mismatch'; END IF;
 IF r.state='SETTLED' THEN
  IF c.state<>'REVEALED' OR NOT s.dealer_revealed OR s.active_hand_id IS NOT NULL THEN RAISE EXCEPTION 'blackjack settlement not revealed'; END IF;
  SELECT count(*),coalesce(sum(delta_units),0) INTO n,amount FROM economy.wallet_ledger WHERE transaction_id=r.settlement_transaction_id AND newapi_user_id=r.newapi_user_id AND asset_type='AVAILABLE_CHIPS' AND entry_type='GAME_PAYOUT';
  IF amount<>r.total_payout_units-r.cap_withheld_units OR (amount>0 AND n<>1) OR (amount=0 AND n<>0) THEN RAISE EXCEPTION 'blackjack payout mismatch'; END IF;
  SELECT count(*) INTO n FROM games.supply_events WHERE round_id=r.round_id AND amount_units=abs((r.net_change_units-r.cap_withheld_units)) AND direction=CASE WHEN (r.net_change_units-r.cap_withheld_units)>0 THEN 'ISSUE' ELSE 'BURN' END;
  IF ((r.net_change_units-r.cap_withheld_units)<>0 AND n<>1) OR ((r.net_change_units-r.cap_withheld_units)=0 AND EXISTS(SELECT 1 FROM games.supply_events WHERE round_id=r.round_id)) THEN RAISE EXCEPTION 'blackjack supply mismatch'; END IF;
 ELSE
  IF c.state<>'CONSUMED' OR s.fair_return_units<>0 OR EXISTS(SELECT 1 FROM games.supply_events WHERE round_id=r.round_id) THEN RAISE EXCEPTION 'active blackjack prematurely finalized'; END IF;
 END IF;
 IF r.recovery_state='NEEDS_REVIEW' THEN RETURN NULL; END IF;
 SELECT count(*),sum(stake_units) INTO n,amount FROM games.blackjack_hands WHERE round_id=r.round_id;
 IF n<1 OR n>4 OR amount<>r.total_stake_units OR (SELECT count(*) FROM games.blackjack_cards WHERE round_id=r.round_id)<>s.shoe_index THEN RAISE EXCEPTION 'blackjack typed state incomplete'; END IF;
 IF r.state='SETTLED' AND (SELECT sum(payout_units) FROM games.blackjack_hands WHERE round_id=r.round_id)+s.fair_return_units<>r.total_payout_units THEN RAISE EXCEPTION 'blackjack hand payout mismatch'; END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION games.check_cap_receipt() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE r games.game_rounds; c economy.cap_settlements;
BEGIN
 SELECT * INTO STRICT r FROM games.game_rounds WHERE round_id=NEW.round_id;
 SELECT * INTO c FROM economy.cap_settlements WHERE source_kind='DIRECT_PLAY_ROUND' AND source_id=r.round_id::text AND newapi_user_id=r.newapi_user_id;
 IF r.economic_policy_version IS NULL THEN
  IF c.source_id IS NOT NULL OR r.cap_withheld_units<>0 THEN RAISE EXCEPTION 'legacy cap receipt mismatch'; END IF;
 ELSIF r.state='SETTLED' AND (c.source_id IS NULL OR c.policy_version<>r.economic_policy_version OR c.gross_payout_units<>r.total_payout_units OR c.withheld_units<>r.cap_withheld_units OR c.actual_net_units<>r.net_change_units-r.cap_withheld_units) THEN
  RAISE EXCEPTION 'incomplete cap receipt';
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER game_cap_complete AFTER INSERT OR UPDATE ON games.game_rounds DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION games.check_cap_receipt();
CREATE OR REPLACE VIEW games.history_source_rows AS
WITH source_rows AS (
 SELECT 'DIRECT_PLAY_ROUND'::text AS record_type,r.round_id AS source_id,r.newapi_user_id,
  NULL::uuid AS parent_source_id,r.game_slug,'DIRECT_PLAY'::text AS mode,
  r.created_at AS occurred_at,r.settled_at AS ended_at,CASE WHEN r.state='SETTLED' THEN CASE WHEN r.net_change_units-r.cap_withheld_units>0 THEN 'WIN' WHEN r.net_change_units-r.cap_withheld_units<0 THEN 'LOSS' ELSE 'BREAK_EVEN' END END AS result_class,
  CASE WHEN r.state='SETTLED' THEN 'SETTLED' WHEN r.recovery_state='NEEDS_REVIEW' THEN 'RECOVERING'
   ELSE 'PROCESSING' END AS display_status,
  r.total_stake_units::numeric AS stake_units,
  CASE WHEN r.state='SETTLED' THEN (r.total_payout_units::numeric-r.cap_withheld_units) END AS payout_units,
  CASE WHEN r.state='SETTLED' THEN (r.net_change_units::numeric-r.cap_withheld_units) END AS net_change_units,
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
  CASE WHEN h.state='SETTLED' THEN CASE WHEN a.awarded+x.returned-x.paid-coalesce(cap.withheld_units,0)>0 THEN 'WIN'
   WHEN a.awarded+x.returned-x.paid-coalesce(cap.withheld_units,0)<0 THEN 'LOSS' ELSE 'BREAK_EVEN' END END,
  CASE WHEN h.state='SETTLED' THEN 'SETTLED' WHEN t.lifecycle_state='RECOVERING' AND t.current_hand_id=h.hand_id
   THEN 'RECOVERING' ELSE 'PROCESSING' END,
  x.paid,CASE WHEN h.state='SETTLED' THEN a.awarded+x.returned-coalesce(cap.withheld_units,0) END,
  CASE WHEN h.state='SETTLED' THEN a.awarded+x.returned-x.paid-coalesce(cap.withheld_units,0) END,NULL,NULL,NULL,
  g.title,t.table_id,t.name,s.display_name_snapshot
 FROM poker.hands h JOIN poker.hand_participants p USING(hand_id)
 LEFT JOIN economy.cap_settlements cap ON cap.source_kind='POKER_HAND' AND cap.source_id=h.hand_id::text AND cap.newapi_user_id=p.newapi_user_id
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
   CASE WHEN r.wager_transaction_id=p_id THEN -(r.total_stake_units::numeric-coalesce((SELECT sum(a.additional_stake_units::numeric) FROM games.round_actions a WHERE a.round_id=r.round_id),0)) ELSE r.total_payout_units::numeric-r.cap_withheld_units END delta,
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
  CASE WHEN f.kind='PAYOUT' THEN r.state='FINISHED' AND EXISTS(SELECT 1 FROM jsonb_array_elements(r.outcome->'awards') a WHERE a->>'user_id'=f.newapi_user_id::text AND (a->>'amount_units')::numeric=f.amount_units+coalesce((SELECT c.withheld_units FROM economy.cap_settlements c WHERE c.source_kind='ROULETTE_ROUND' AND c.source_id=r.round_id::text AND c.newapi_user_id=f.newapi_user_id),0))
   WHEN f.kind='VOID_REFUND' THEN r.state='CANCELLED' AND r.void_outcome IS NOT NULL AND f.amount_units=r.stake_units
   ELSE f.amount_units=r.stake_units END
 FROM roulette.funding f JOIN roulette.rounds r USING(round_id) LEFT JOIN roulette.participants p USING(round_id,newapi_user_id) WHERE f.transaction_id=p_id
 )
 SELECT count(*),min(kind),min(id::text)::uuid,min(delta),min(ledger::text)::uuid,
  coalesce(bool_or(uid<>p_user OR biz<>t.biz_type OR operation<>t.operation_type OR business_id<>t.biz_id OR NOT coalesce(valid,false)),false)
 INTO refs,source_type,source_id,expected,expected_ledger,invalid FROM sources;
 IF n=0 AND NOT invalid AND refs=0 AND t.operation_type IN('DAILY_REWARD','HOURLY_REWARD','RELIEF_REWARD','INITIAL_GRANT_REGISTRATION') AND EXISTS(
 SELECT 1 FROM economy.cap_settlements c WHERE c.source_kind=t.biz_type AND c.source_id=t.biz_id AND c.newapi_user_id=p_user AND c.credited_payout_units=0 AND c.gross_payout_units>0
 ) THEN
 RETURN jsonb_build_object('id',t.transaction_id,'kind',t.operation_type,'status',t.status,'created_at',t.created_at,'confirmed_at',t.confirmed_at,'effects',effects,'links',links);
 END IF;
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
CREATE OR REPLACE FUNCTION economy._poker_funding_apply(p_table uuid,p_epoch bigint,p_operation uuid,p_gateway text)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE t poker.tables%ROWTYPE; seat poker.seats%ROWTYPE; sess poker.sessions%ROWTYPE;
 op poker.funding_operations%ROWTYPE; reservation poker.seat_reservations%ROWTYPE;
 wallet economy.wallet_balances%ROWTYPE; bb bigint; amount bigint; delta bigint; balance_after bigint;
 business text; result jsonb; profile_name text; at_time timestamptz:=clock_timestamp();
BEGIN
 SELECT * INTO STRICT t FROM poker.tables WHERE table_id=p_table FOR UPDATE;
 IF t.runtime_epoch<>p_epoch THEN RAISE EXCEPTION 'STALE_RUNTIME_EPOCH' USING ERRCODE='40001'; END IF;
 SELECT * INTO STRICT op FROM poker.funding_operations WHERE funding_operation_id=p_operation AND table_id=p_table;
 IF (p_gateway='BUY_IN' AND op.kind<>'BUY_IN') OR (p_gateway='TOP_UP' AND op.kind NOT IN('TOP_UP','REBUY')) OR (p_gateway='CASH_OUT' AND op.kind<>'CASH_OUT') THEN RAISE EXCEPTION 'FUNDING_KIND_MISMATCH'; END IF;
 SELECT * INTO STRICT seat FROM poker.seats WHERE table_id=p_table AND seat_no=op.seat_no FOR UPDATE;
 IF op.kind<>'BUY_IN' THEN SELECT * INTO STRICT sess FROM poker.sessions WHERE session_id=op.session_id AND table_id=p_table AND seat_no=op.seat_no AND newapi_user_id=op.newapi_user_id FOR UPDATE; END IF;
 SELECT * INTO STRICT op FROM poker.funding_operations WHERE funding_operation_id=p_operation FOR UPDATE;
 IF op.state='CONFIRMED' THEN RETURN op.receipt; END IF;
 IF op.state<>'PENDING' THEN RAISE EXCEPTION 'FUNDING_NOT_PENDING'; END IF;
 SELECT big_blind_units INTO STRICT bb FROM poker.blind_preset_versions WHERE version=t.blind_preset_version;
 amount:=op.amount_units;
 IF op.kind='BUY_IN' THEN
   IF NOT t.accepting_players OR t.lifecycle_state IN('CLOSING','CLOSED') OR seat.session_id IS NOT NULL OR amount<40*bb OR amount>100*bb THEN RAISE EXCEPTION 'INVALID_BUY_IN'; END IF;
   SELECT * INTO STRICT reservation FROM poker.seat_reservations WHERE reservation_id=op.reservation_id AND table_id=p_table AND seat_no=op.seat_no AND newapi_user_id=op.newapi_user_id FOR UPDATE;
   IF reservation.state<>'LEASE_ACTIVE' OR reservation.expires_at<=at_time THEN RAISE EXCEPTION 'RESERVATION_EXPIRED'; END IF;
   SELECT display_name INTO STRICT profile_name FROM identity.master_profiles WHERE newapi_user_id=op.newapi_user_id AND profile_version>0;
   business:='poker_buyin:'||op.session_id::text;delta:=-amount;
 ELSE
   IF sess.state<>'ACTIVE' OR seat.session_id IS DISTINCT FROM op.session_id THEN RAISE EXCEPTION 'SESSION_NOT_ACTIVE'; END IF;
   IF EXISTS(SELECT 1 FROM poker.hand_participants p JOIN poker.hands h USING(hand_id) WHERE p.session_id=op.session_id AND h.state<>'SETTLED') THEN RAISE EXCEPTION 'HAND_UNSETTLED'; END IF;
   IF op.kind='CASH_OUT' THEN amount:=sess.current_stack_units;delta:=amount;business:='poker_cashout:'||op.session_id::text;
   ELSE
     IF t.lifecycle_state IN('CLOSING','CLOSED') OR amount<=0 OR sess.current_stack_units::numeric+amount::numeric>100*bb THEN RAISE EXCEPTION 'INVALID_TOP_UP'; END IF;
     IF op.kind='REBUY' AND (seat.state<>'REBUY_WINDOW' OR seat.rebuy_deadline_at<=at_time OR amount<40*bb) THEN RAISE EXCEPTION 'INVALID_REBUY'; END IF;
     delta:=-amount;business:='poker_'||CASE WHEN op.kind='REBUY' THEN 'rebuy:' ELSE 'topup:' END||op.funding_operation_id::text;
   END IF;
 END IF;
 -- Table -> Seat -> Session -> Wallet is the only funding lock order.
 PERFORM pg_advisory_xact_lock(('x'||substr(encode(sha256(convert_to('["quota-transfer-user",'||op.newapi_user_id::text||']','UTF8')),'hex'),1,16))::bit(64)::bigint);
 SELECT * INTO STRICT wallet FROM economy.wallet_balances WHERE newapi_user_id=op.newapi_user_id AND asset_type='AVAILABLE_CHIPS' FOR UPDATE;
 -- Wallet acquisition can wait. All validity time below is fresh after that wait.
 at_time:=clock_timestamp();
 IF op.kind='BUY_IN' AND reservation.expires_at<=at_time THEN RAISE EXCEPTION 'RESERVATION_EXPIRED'; END IF;
 IF op.kind='REBUY' AND seat.rebuy_deadline_at<=at_time THEN RAISE EXCEPTION 'INVALID_REBUY'; END IF;
 IF wallet.balance_units::numeric+delta::numeric<0 THEN RAISE EXCEPTION 'INSUFFICIENT_CHIPS' USING ERRCODE='P0001'; END IF;
 balance_after:=wallet.balance_units+delta;
 INSERT INTO economy.asset_transactions(transaction_id,biz_type,biz_id,newapi_user_id,operation_type,status,request_hash)
 VALUES(op.planned_transaction_id,'POKER_FUNDING',business,op.newapi_user_id,'POKER_'||op.kind,'CONFIRMED',op.request_hash);
 IF delta<>0 THEN
   UPDATE economy.wallet_balances SET balance_units=balance_after,ledger_seq=ledger_seq+1,version=version+1,updated_at=at_time WHERE newapi_user_id=op.newapi_user_id AND asset_type='AVAILABLE_CHIPS';
   INSERT INTO economy.wallet_ledger(ledger_entry_id,transaction_id,leg_no,newapi_user_id,asset_type,ledger_seq,wallet_version,entry_type,biz_type,biz_id,delta_units,balance_before_units,balance_after_units,metadata)
   VALUES(op.planned_ledger_id,op.planned_transaction_id,1,op.newapi_user_id,'AVAILABLE_CHIPS',wallet.ledger_seq+1,wallet.version+1,'POKER_'||op.kind,'POKER_FUNDING',business,delta,wallet.balance_units,balance_after,jsonb_build_object('table_id',p_table,'session_id',op.session_id));
 END IF;
 IF op.kind='BUY_IN' THEN
   INSERT INTO poker.sessions(session_id,newapi_user_id,table_id,seat_no,display_name_snapshot,initial_buyin_units,current_stack_units) VALUES(op.session_id,op.newapi_user_id,p_table,op.seat_no,profile_name,amount,amount);
   UPDATE poker.seats SET session_id=op.session_id,state=CASE WHEN t.hand_no=0 THEN 'ACTIVE' ELSE 'WAITING_BIG_BLIND' END,connected=true,disconnected_since=NULL,sit_out_since=NULL,timeout_count=0,timeout_sit_out=false,sit_out_next_hand=false,leave_after_hand=false,rebuy_deadline_at=NULL WHERE table_id=p_table AND seat_no=op.seat_no;
   UPDATE poker.seat_reservations SET state='CONSUMED' WHERE reservation_id=op.reservation_id;
   UPDATE poker.tables SET settings_locked_at=coalesce(settings_locked_at,at_time),empty_since=NULL WHERE table_id=p_table;
 ELSIF op.kind='CASH_OUT' THEN
   UPDATE poker.sessions SET current_stack_units=0,final_cashout_units=amount,realized_pl_units=amount-initial_buyin_units-total_topup_units,state='SETTLED',ended_at=at_time,end_reason=coalesce(end_reason,'SAFE_LEAVE') WHERE session_id=op.session_id;
   UPDATE poker.seats SET session_id=NULL,state='LEFT',leave_after_hand=false,sit_out_next_hand=false,rebuy_deadline_at=NULL WHERE table_id=p_table AND seat_no=op.seat_no;
 ELSE
   UPDATE poker.sessions SET current_stack_units=current_stack_units+amount,total_topup_units=total_topup_units+amount WHERE session_id=op.session_id;
   IF op.kind='REBUY' THEN UPDATE poker.seats SET state='WAITING_BIG_BLIND',rebuy_deadline_at=NULL WHERE table_id=p_table AND seat_no=op.seat_no; END IF;
 END IF;
 result:=jsonb_build_object('table_id',p_table,'session_id',op.session_id,'funding_operation_id',op.funding_operation_id,'status','CONFIRMED','amount_units',amount::text,'transaction_id',op.planned_transaction_id,'biz_id',business);
 UPDATE poker.funding_operations SET amount_units=amount,state='CONFIRMED',confirmed_transaction_id=op.planned_transaction_id,confirmed_ledger_id=CASE WHEN delta=0 THEN NULL ELSE op.planned_ledger_id END,receipt=result,confirmed_at=at_time WHERE funding_operation_id=p_operation;
 RETURN result;
END $$;
