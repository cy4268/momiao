-- Forward-only Blackjack integration. Earlier fast-game constraints remain in
-- force for their slugs; only Blackjack may persist a paid PLAYER_TURN.
ALTER TABLE games.game_rounds
 DROP CONSTRAINT game_rounds_state_check,
 DROP CONSTRAINT game_rounds_recovery_state_check,
 DROP CONSTRAINT game_rounds_check1,
 DROP CONSTRAINT game_rounds_check2,
 ALTER COLUMN common_result DROP NOT NULL,
 ALTER COLUMN settlement_transaction_id DROP NOT NULL,
 ALTER COLUMN settled_at DROP NOT NULL,
 ALTER COLUMN settled_at DROP DEFAULT,
 ADD CONSTRAINT game_round_phase CHECK (state='SETTLED' OR (game_slug='blackjack' AND state='PLAYER_TURN')),
 ADD CONSTRAINT game_round_recovery CHECK (recovery_state='NORMAL' OR (game_slug='blackjack' AND recovery_state='NEEDS_REVIEW')),
 ADD CONSTRAINT game_round_fast_balance CHECK (game_slug='blackjack' OR balance_after_units::numeric=balance_before_units::numeric+net_change_units::numeric),
 ADD CONSTRAINT game_round_terminal_fields CHECK (
  (state='SETTLED' AND settled_at IS NOT NULL AND settlement_transaction_id IS NOT NULL AND common_result IS NOT NULL
   AND ((net_change_units>0 AND common_result='WIN') OR (net_change_units=0 AND common_result='BREAK_EVEN') OR (net_change_units<0 AND common_result='LOSS')))
  OR (state='PLAYER_TURN' AND settled_at IS NULL AND settlement_transaction_id IS NULL AND common_result IS NULL AND total_payout_units=0)
 );
-- Fast creates rely on their original default. Blackjack explicitly inserts NULL.
ALTER TABLE games.game_rounds ALTER COLUMN settled_at SET DEFAULT now();
CREATE UNIQUE INDEX one_active_blackjack ON games.game_rounds(newapi_user_id,game_slug) WHERE game_slug='blackjack' AND state='PLAYER_TURN';
DROP TRIGGER immutable_history ON games.game_rounds;
CREATE FUNCTION games.guard_blackjack_round() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.game_slug<>'blackjack' OR OLD.state='SETTLED' OR OLD.recovery_state='NEEDS_REVIEW'
 OR (to_jsonb(NEW)-ARRAY['state','recovery_state','total_stake_units','total_payout_units','net_change_units','common_result','balance_after_units','settlement_transaction_id','settled_at'])
 IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','recovery_state','total_stake_units','total_payout_units','net_change_units','common_result','balance_after_units','settlement_transaction_id','settled_at'])
 OR NEW.total_stake_units<OLD.total_stake_units THEN RAISE EXCEPTION 'immutable round authority' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER blackjack_round_guard BEFORE UPDATE ON games.game_rounds FOR EACH ROW EXECUTE FUNCTION games.guard_blackjack_round();
CREATE TRIGGER immutable_history BEFORE DELETE OR TRUNCATE ON games.game_rounds FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
DROP TRIGGER fast_settlement_complete ON games.game_rounds;
CREATE CONSTRAINT TRIGGER fast_settlement_complete AFTER INSERT ON games.game_rounds DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (NEW.game_slug<>'blackjack') EXECUTE FUNCTION games.check_fast_settlement();

ALTER TABLE games.round_actions
 DROP CONSTRAINT round_actions_action_type_check,
 DROP CONSTRAINT round_actions_additional_stake_units_check,
 DROP CONSTRAINT round_actions_system_action_check,
 ADD COLUMN expected_round_version BIGINT,
 ADD COLUMN hand_id UUID,
 ADD COLUMN new_hand_id UUID,
 ADD COLUMN request_hash BYTEA CHECK (octet_length(request_hash)=32),
 ADD COLUMN original_response JSONB,
 ADD COLUMN stake_transaction_id UUID UNIQUE REFERENCES economy.asset_transactions,
 ADD CONSTRAINT round_action_shape CHECK (
  (action_type='SCRATCH_REVEAL_COMPLETE' AND additional_stake_units=0 AND NOT system_action AND expected_round_version IS NULL AND hand_id IS NULL AND new_hand_id IS NULL AND request_hash IS NULL AND original_response IS NULL AND stake_transaction_id IS NULL)
  OR (action_type IN ('HIT','STAND','DOUBLE','SPLIT','SYSTEM_AUTO_STAND') AND additional_stake_units>=0 AND expected_round_version>0 AND hand_id IS NOT NULL AND request_hash IS NOT NULL AND original_response IS NOT NULL
   AND system_action=(action_type='SYSTEM_AUTO_STAND') AND (action_type='SPLIT')=(new_hand_id IS NOT NULL)
   AND (additional_stake_units>0)=(stake_transaction_id IS NOT NULL)
   AND (action_type IN ('DOUBLE','SPLIT'))=(additional_stake_units>0))
 );
CREATE TABLE games.blackjack_round_state (
 round_id UUID PRIMARY KEY,
 game_slug TEXT GENERATED ALWAYS AS ('blackjack'::text) STORED,
 initial_hand_id UUID NOT NULL,
 initial_wager_units BIGINT NOT NULL CHECK (initial_wager_units>=5000000 AND initial_wager_units%500000=0),
 shoe_hash BYTEA NOT NULL CHECK (octet_length(shoe_hash)=32),
 shoe_index INTEGER NOT NULL CHECK (shoe_index BETWEEN 4 AND 312),
 round_version BIGINT NOT NULL CHECK (round_version BETWEEN 1 AND 321),
 active_hand_id UUID,
 dealer_revealed BOOLEAN NOT NULL,
 last_player_action_at TIMESTAMPTZ,
 auto_resolve_at TIMESTAMPTZ,
 FOREIGN KEY(round_id,game_slug) REFERENCES games.game_rounds(round_id,game_slug),
 CHECK ((last_player_action_at IS NULL)=(auto_resolve_at IS NULL)),
 CHECK (auto_resolve_at=last_player_action_at+INTERVAL '24 hours')
);
CREATE TABLE games.blackjack_hands (
 hand_id UUID PRIMARY KEY,
 round_id UUID NOT NULL REFERENCES games.blackjack_round_state,
 hand_index SMALLINT NOT NULL CHECK (hand_index BETWEEN 0 AND 7),
 parent_hand_id UUID REFERENCES games.blackjack_hands,
 stake_units BIGINT NOT NULL CHECK (stake_units>0),
 from_split BOOLEAN NOT NULL,
 split_aces BOOLEAN NOT NULL,
 is_natural BOOLEAN NOT NULL,
 hand_state TEXT NOT NULL CHECK (hand_state IN ('ACTIVE','STOOD','BUST','DOUBLED_COMPLETE','SPLIT_ACES_COMPLETE','NATURAL_COMPLETE')),
 hard_total INTEGER NOT NULL CHECK (hard_total BETWEEN 2 AND 40),
 best_total INTEGER NOT NULL CHECK (best_total>=hard_total AND best_total<=40),
 is_soft BOOLEAN NOT NULL,
 result TEXT NOT NULL CHECK (result IN ('','LOSS','BUST','PUSH','NORMAL_WIN','NATURAL')),
 payout_units BIGINT NOT NULL CHECK (payout_units>=0),
 net_change_units BIGINT NOT NULL,
 UNIQUE(round_id,hand_id),
 UNIQUE(round_id,hand_index),
 CHECK (NOT is_natural OR NOT from_split)
);
ALTER TABLE games.blackjack_round_state ADD FOREIGN KEY(round_id,initial_hand_id) REFERENCES games.blackjack_hands(round_id,hand_id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE games.blackjack_round_state ADD FOREIGN KEY(round_id,active_hand_id) REFERENCES games.blackjack_hands(round_id,hand_id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE games.blackjack_cards (
 round_id UUID NOT NULL REFERENCES games.blackjack_round_state,
 shoe_index INTEGER NOT NULL CHECK (shoe_index BETWEEN 0 AND 311),
 card_instance_id SMALLINT NOT NULL CHECK (card_instance_id BETWEEN 0 AND 311),
 recipient TEXT NOT NULL CHECK (recipient IN ('PLAYER','DEALER')),
 hand_id UUID,
 PRIMARY KEY(round_id,shoe_index),
 UNIQUE(round_id,card_instance_id),
 FOREIGN KEY(round_id,hand_id) REFERENCES games.blackjack_hands(round_id,hand_id),
 CHECK ((recipient='PLAYER')=(hand_id IS NOT NULL))
);
ALTER TABLE games.round_actions ADD FOREIGN KEY(round_id,hand_id) REFERENCES games.blackjack_hands(round_id,hand_id);
ALTER TABLE games.round_actions ADD FOREIGN KEY(round_id,new_hand_id) REFERENCES games.blackjack_hands(round_id,hand_id);
CREATE TABLE games.round_jobs (
 job_id UUID PRIMARY KEY,
 round_id UUID NOT NULL REFERENCES games.game_rounds,
 job_type TEXT NOT NULL CHECK (job_type IN ('BLACKJACK_AUTO_RESOLVE','GAME_ROUND_RECOVERY')),
 run_at TIMESTAMPTZ NOT NULL,
 status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','DONE','NEEDS_REVIEW')),
 attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts>=0),
 last_error TEXT,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(round_id,job_type)
);
CREATE INDEX round_jobs_due ON games.round_jobs(run_at,job_id) WHERE status='PENDING';
CREATE FUNCTION games.guard_blackjack_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM games.game_rounds WHERE round_id=OLD.round_id AND (state='SETTLED' OR recovery_state<>'NORMAL'))
 OR (to_jsonb(NEW)-ARRAY['game_slug','shoe_index','round_version','active_hand_id','dealer_revealed','last_player_action_at','auto_resolve_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['game_slug','shoe_index','round_version','active_hand_id','dealer_revealed','last_player_action_at','auto_resolve_at'])
 OR NEW.shoe_index<OLD.shoe_index OR NEW.round_version<=OLD.round_version THEN RAISE EXCEPTION 'invalid blackjack state change' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER blackjack_state_guard BEFORE UPDATE ON games.blackjack_round_state FOR EACH ROW EXECUTE FUNCTION games.guard_blackjack_state();
CREATE FUNCTION games.guard_blackjack_hand() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM games.game_rounds WHERE round_id=OLD.round_id AND (state='SETTLED' OR recovery_state<>'NORMAL'))
 OR NEW.hand_id<>OLD.hand_id OR NEW.round_id<>OLD.round_id OR NEW.hand_index<>OLD.hand_index OR NEW.parent_hand_id IS DISTINCT FROM OLD.parent_hand_id
 OR NEW.stake_units<OLD.stake_units OR (OLD.from_split AND NOT NEW.from_split) THEN RAISE EXCEPTION 'invalid blackjack hand change' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER blackjack_hand_guard BEFORE UPDATE ON games.blackjack_hands FOR EACH ROW EXECUTE FUNCTION games.guard_blackjack_hand();
CREATE FUNCTION games.guard_blackjack_card() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (to_jsonb(NEW)-'hand_id') IS DISTINCT FROM (to_jsonb(OLD)-'hand_id') OR OLD.recipient<>'PLAYER'
 OR NOT EXISTS(SELECT 1 FROM games.blackjack_hands WHERE hand_id=NEW.hand_id AND round_id=OLD.round_id AND parent_hand_id=OLD.hand_id)
 OR EXISTS(SELECT 1 FROM games.game_rounds WHERE round_id=OLD.round_id AND (state='SETTLED' OR recovery_state<>'NORMAL')) THEN RAISE EXCEPTION 'immutable dealt card' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER blackjack_card_guard BEFORE UPDATE ON games.blackjack_cards FOR EACH ROW EXECUTE FUNCTION games.guard_blackjack_card();
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['blackjack_round_state','blackjack_hands','blackjack_cards'] LOOP
  EXECUTE format('CREATE TRIGGER immutable_history BEFORE DELETE OR TRUNCATE ON games.%I FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change()',t);
 END LOOP;
END $$;

CREATE FUNCTION games.check_blackjack_persistence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r games.game_rounds; s games.blackjack_round_state; c games.fairness_commitments; n bigint; amount numeric; BEGIN
 SELECT * INTO STRICT r FROM games.game_rounds WHERE round_id=NEW.round_id;
 IF r.game_slug<>'blackjack' THEN RETURN NULL; END IF;
 SELECT * INTO STRICT s FROM games.blackjack_round_state WHERE round_id=r.round_id;
 SELECT * INTO STRICT c FROM games.fairness_commitments WHERE commitment_id=r.commitment_id;
 IF c.nonce<>r.nonce OR c.game_config_version_id<>r.game_config_version_id OR c.game_config_hash<>r.game_config_hash OR c.wager_policy_version_id<>r.wager_policy_version_id OR c.wager_policy_hash<>r.wager_policy_hash OR c.algorithm_version<>r.algorithm_version OR c.ruleset_version<>r.ruleset_version OR c.fairness_stream_version<>r.fairness_stream_version THEN RAISE EXCEPTION 'blackjack commitment mismatch'; END IF;
 SELECT count(*) INTO n FROM economy.wallet_ledger WHERE transaction_id=r.wager_transaction_id AND newapi_user_id=r.newapi_user_id AND asset_type='AVAILABLE_CHIPS' AND entry_type='GAME_WAGER' AND delta_units=-s.initial_wager_units AND balance_before_units=r.balance_before_units;
 IF n<>1 THEN RAISE EXCEPTION 'blackjack initial funding mismatch'; END IF;
 SELECT count(*),coalesce(sum(additional_stake_units),0) INTO n,amount FROM games.round_actions WHERE round_id=r.round_id;
 IF s.initial_wager_units::numeric+amount<>r.total_stake_units OR s.round_version<>n+1 THEN RAISE EXCEPTION 'blackjack action stake/version mismatch'; END IF;
 IF EXISTS(SELECT 1 FROM games.round_actions a WHERE a.round_id=r.round_id AND (a.newapi_user_id<>r.newapi_user_id OR (a.additional_stake_units>0 AND (SELECT count(*) FROM economy.wallet_ledger l WHERE l.transaction_id=a.stake_transaction_id AND l.newapi_user_id=r.newapi_user_id AND l.asset_type='AVAILABLE_CHIPS' AND l.entry_type='GAME_ADDITIONAL_WAGER' AND l.delta_units=-a.additional_stake_units)<>1))) THEN RAISE EXCEPTION 'blackjack additional funding mismatch'; END IF;
 IF r.state='SETTLED' THEN
  IF c.state<>'REVEALED' OR NOT s.dealer_revealed OR s.active_hand_id IS NOT NULL THEN RAISE EXCEPTION 'blackjack settlement not revealed'; END IF;
  SELECT count(*),coalesce(sum(delta_units),0) INTO n,amount FROM economy.wallet_ledger WHERE transaction_id=r.settlement_transaction_id AND newapi_user_id=r.newapi_user_id AND asset_type='AVAILABLE_CHIPS' AND entry_type='GAME_PAYOUT';
  IF amount<>r.total_payout_units OR (amount>0 AND n<>1) OR (amount=0 AND n<>0) THEN RAISE EXCEPTION 'blackjack payout mismatch'; END IF;
  SELECT count(*) INTO n FROM games.supply_events WHERE round_id=r.round_id AND amount_units=abs(r.net_change_units) AND direction=CASE WHEN r.net_change_units>0 THEN 'ISSUE' ELSE 'BURN' END;
  IF (r.net_change_units<>0 AND n<>1) OR (r.net_change_units=0 AND EXISTS(SELECT 1 FROM games.supply_events WHERE round_id=r.round_id)) THEN RAISE EXCEPTION 'blackjack supply mismatch'; END IF;
 ELSE
  IF c.state<>'CONSUMED' OR EXISTS(SELECT 1 FROM games.supply_events WHERE round_id=r.round_id) THEN RAISE EXCEPTION 'active blackjack prematurely finalized'; END IF;
 END IF;
 IF r.recovery_state='NEEDS_REVIEW' THEN RETURN NULL; END IF;
 SELECT count(*),sum(stake_units) INTO n,amount FROM games.blackjack_hands WHERE round_id=r.round_id;
 IF n<1 OR n>4 OR amount<>r.total_stake_units OR (SELECT count(*) FROM games.blackjack_cards WHERE round_id=r.round_id)<>s.shoe_index THEN RAISE EXCEPTION 'blackjack typed state incomplete'; END IF;
 IF r.state='SETTLED' AND (SELECT sum(payout_units) FROM games.blackjack_hands WHERE round_id=r.round_id)<>r.total_payout_units THEN RAISE EXCEPTION 'blackjack hand payout mismatch'; END IF;
 RETURN NULL;
END $$;
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['game_rounds','blackjack_round_state','blackjack_hands','blackjack_cards'] LOOP
  EXECUTE format('CREATE CONSTRAINT TRIGGER blackjack_persistence_complete AFTER INSERT OR UPDATE ON games.%I DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION games.check_blackjack_persistence()',t);
 END LOOP;
END $$;
CREATE CONSTRAINT TRIGGER blackjack_action_complete AFTER INSERT ON games.round_actions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION games.check_blackjack_persistence();

-- BEGIN FROZEN CONFIG SEED (scripts/games_extra_seed.py)
INSERT INTO games.game_config_versions(config_version_id,game_slug,config_schema_version,ruleset_version,algorithm_version,config_payload,canonical_payload,config_hash,resource_versions) VALUES('01993200-0000-7000-8000-000000000005','blackjack','blackjack-config-v1','blackjack-rules-v1','blackjack-map-v1','{"card_count":312,"card_encoding":"DECK_TIMES_52_PLUS_SUIT_TIMES_13_PLUS_RANK","config_version":"01993200-0000-7000-8000-000000000005","dealer_rule":"S17","deck_count":6,"double_after_split":true,"double_rule":"ANY_INITIAL_TWO_CARDS","even_money":false,"inactivity_action":"SYSTEM_AUTO_STAND","inactivity_hours":24,"initial_deal_order":["PLAYER","DEALER_UP","PLAYER","DEALER_HOLE"],"insurance":false,"maximum_hands":4,"natural_profit_denominator":2,"natural_profit_numerator":3,"peek_rule":"AMERICAN_ACE_OR_TEN","resplit":true,"resplit_aces":false,"ruleset_version":"blackjack-rules-v1","shoe_model":"FRESH_PER_ROUND","shuffle_algorithm_version":"blackjack-fy-v1","side_bets":false,"split_21_is_natural":false,"split_aces_draw_count":1,"split_rule":"EQUAL_POINT_VALUE","surrender":false}'::jsonb,convert_to('{"card_count":312,"card_encoding":"DECK_TIMES_52_PLUS_SUIT_TIMES_13_PLUS_RANK","config_version":"01993200-0000-7000-8000-000000000005","dealer_rule":"S17","deck_count":6,"double_after_split":true,"double_rule":"ANY_INITIAL_TWO_CARDS","even_money":false,"inactivity_action":"SYSTEM_AUTO_STAND","inactivity_hours":24,"initial_deal_order":["PLAYER","DEALER_UP","PLAYER","DEALER_HOLE"],"insurance":false,"maximum_hands":4,"natural_profit_denominator":2,"natural_profit_numerator":3,"peek_rule":"AMERICAN_ACE_OR_TEN","resplit":true,"resplit_aces":false,"ruleset_version":"blackjack-rules-v1","shoe_model":"FRESH_PER_ROUND","shuffle_algorithm_version":"blackjack-fy-v1","side_bets":false,"split_21_is_natural":false,"split_aces_draw_count":1,"split_rule":"EQUAL_POINT_VALUE","surrender":false}','UTF8'),decode('b2a7cab09c44bf381fce2809ec148c6aa115dc6132c2af4908da27961faf9fe4','hex'),'{"shuffle_algorithm_version":"blackjack-fy-v1"}'::jsonb);
INSERT INTO games.game_validation_artifacts(validation_artifact_id,game_slug,artifact_type,implementation_key,ruleset_version,algorithm_version,config_version_id,config_hash,validator_version,validation_build,result_summary,artifact_sha256,status,verified_at) VALUES('01993200-0000-7000-8000-000000000205','blackjack','BLACKJACK_RTP','direct.blackjack.v1','blackjack-rules-v1','blackjack-map-v1','01993200-0000-7000-8000-000000000005',decode('b2a7cab09c44bf381fce2809ec148c6aa115dc6132c2af4908da27961faf9fe4','hex'),'game-math-validator-v1','bbc866af6886d679f0874f09f962ef3831626f72','{"algorithm_version":"blackjack-map-v1","artifact_type":"BLACKJACK_RTP","canonical_payload_sha256":"32de666fd3f3c30aa57e96ca648eb33d573d7999bc411e70a2eea8c4bea148e0","config_hash":"b2a7cab09c44bf381fce2809ec148c6aa115dc6132c2af4908da27961faf9fe4","config_version":"01993200-0000-7000-8000-000000000005","engine_resource_sha256":{"go.mod":"be28ba63568a818c941afef52079717d0a7e496e103533f644815a81579e35a9","go.sum":"d94c510e0096cce235911916a4221f9ac744b71dbd453fffa7dc72d6ffaf44d6","internal/games/blackjack/blackjack.go":"40c33dd614866c828ab4e055a86019f12b6e6670a8615dc7236a68a148dbedbc","internal/games/blackjack/blackjack_test.go":"4dfe11ab0aa55def30ec07043e5fc00c25c4627ac165a25a6ad1f66dd06f5651","internal/games/blackjack/cards.go":"bababc2a7551115454a0808147ef2c141311c1ca63a8bdc741252396b6388c1a","internal/games/slot/slot.go":"76c6a5ff404fd844aa98e14639c9b6f3eb79ba4feeb108b12debd0fa3749fa5c","internal/games/slot/slot_test.go":"763f8393e2b269abacb6535462ab7ade0fdc4085e5cf610073a04a1d29ea6ada"},"implementation_key":"direct.blackjack.v1","result_json":"{\"method\":\"REFERENCE_STRATEGY_MONTE_CARLO\",\"complete\":true,\"rounds\":10000000,\"hands\":10277657,\"initial_wager_units\":\"5000000\",\"total_initial_wager_units\":\"50000000000000\",\"total_accepted_stake_units\":\"56569375000000\",\"total_payout_units\":\"56383980000000\",\"total_net_units\":\"-185395000000\",\"initial_wager_denominator\":{\"definition\":\"Effective RTP=1+sum(net)/sum(initial wagers); house edge=-sum(net)/sum(initial wagers). Raw payout ratio=sum(payout)/sum(initial wagers).\",\"denominator_units\":\"50000000000000\",\"rtp\":{\"point\":{\"numerator\":\"9962921\",\"denominator\":\"10000000\",\"fraction\":\"9962921/10000000\",\"decimal\":\"0.996292100000000000\"},\"variance_of_estimate\":{\"numerator\":\"14788588349751\",\"denominator\":\"111111100000000000000\",\"fraction\":\"14788588349751/111111100000000000000\",\"decimal\":\"0.000000133097308457\"},\"standard_error\":0.0003648250381449851,\"normal_approximation_ci95\":[0.9955770560645774,0.9970071439354227],\"method\":\"independent round sample mean; unbiased sample variance; normal z=1.959963984540054\"},\"house_edge\":{\"point\":{\"numerator\":\"37079\",\"denominator\":\"10000000\",\"fraction\":\"37079/10000000\",\"decimal\":\"0.003707900000000000\"},\"variance_of_estimate\":{\"numerator\":\"14788588349751\",\"denominator\":\"111111100000000000000\",\"fraction\":\"14788588349751/111111100000000000000\",\"decimal\":\"0.000000133097308457\"},\"standard_error\":0.0003648250381449851,\"normal_approximation_ci95\":[0.0029928560645773776,0.0044229439354226225],\"method\":\"independent round sample mean; unbiased sample variance; normal z=1.959963984540054\"},\"raw_payout_ratio\":{\"numerator\":\"2819199\",\"denominator\":\"2500000\",\"fraction\":\"2819199/2500000\",\"decimal\":\"1.127679600000000000\"}},\"accepted_total_stake_denominator\":{\"definition\":\"RTP=sum(payout)/sum(all accepted initial+split+double stakes); house edge=1-RTP.\",\"denominator_units\":\"56569375000000\",\"rtp\":{\"point\":{\"numerator\":\"11276796\",\"denominator\":\"11313875\",\"fraction\":\"11276796/11313875\",\"decimal\":\"0.996722696688800256\"},\"variance_of_estimate\":{\"numerator\":\"24235128290765915318016\",\"denominator\":\"233030582893478743682869221875\",\"fraction\":\"24235128290765915318016/233030582893478743682869221875\",\"decimal\":\"0.000000103999775437\"},\"standard_error\":0.00032248996176087696,\"normal_approximation_ci95\":[0.9960906279783732,0.9973547653992272],\"method\":\"ratio-of-means delta method with round-cluster residuals; normal z=1.959963984540054\"},\"house_edge\":{\"point\":{\"numerator\":\"37079\",\"denominator\":\"11313875\",\"fraction\":\"37079/11313875\",\"decimal\":\"0.003277303311199744\"},\"variance_of_estimate\":{\"numerator\":\"24235128290765915318016\",\"denominator\":\"233030582893478743682869221875\",\"fraction\":\"24235128290765915318016/233030582893478743682869221875\",\"decimal\":\"0.000000103999775437\"},\"standard_error\":0.00032248996176087696,\"normal_approximation_ci95\":[0.0026452346007727256,0.0039093720216267615],\"method\":\"ratio-of-means delta method with round-cluster residuals; normal z=1.959963984540054\"},\"raw_payout_ratio\":{\"numerator\":\"11276796\",\"denominator\":\"11313875\",\"fraction\":\"11276796/11313875\",\"decimal\":\"0.996722696688800256\"}},\"round_counts\":{\"no_win\":4760642,\"partial_return\":25012,\"break_even\":876994,\"win\":4337352,\"nonzero_payout\":5239358,\"loss\":4785654},\"hand_result_counts\":{\"BUST\":1608298,\"LOSS\":3311993,\"NATURAL\":454280,\"NORMAL_WIN\":4022620,\"PUSH\":880466},\"action_counts\":{\"DOUBLE\":1036218,\"HIT\":5601306,\"SPLIT\":277657,\"STAND\":6225863},\"split_rounds\":247058,\"double_rounds\":1023590,\"player_naturals\":475797,\"dealer_naturals\":473979,\"consumed_cards\":54792230,\"sampling\":{\"public_offline_seed_hex\":\"f4a1c928673b05deae79d0346c1208ab819a6729505e3fc4d84b709b2ca13567\",\"sampler_version\":\"offline-hmac-sha256-u32be-rejection-v1\",\"stream_and_uniform_definition\":\"For round r=0..N-1: concatenate HMAC-SHA256(key=hex-decoded public seed,message=UTF8(game-math-validation:blackjack:v1)||0x00||U64BE(r)||U32BE(block)), block starts 0. Read each block as eight U32BE words; reject x \\u003e= floor(2^32/n)*n, else return x mod n; no recycling of rejected words. Unconsumed bytes discarded at round end.\",\"shuffle_version\":\"blackjack-fy-v1\",\"shuffle_definition\":\"Actual blackjack.Shuffle, canonical card instances 0..311; descending Fisher-Yates i=311..1, UniformInt(i+1); card rank = instance%13, suit/deck as frozen engine; ShoeHash SHA256 of 312 U16BE instances\",\"round_start_index\":0,\"independent_round_definition\":\"One freshly shuffled complete 6-deck shoe each round; no shoe carry-over, no penetration model, unlimited offline extra-stake funding\",\"uniform_words_consumed\":3110000049,\"uniform_words_rejected\":49,\"hmac_blocks\":390000000,\"first_shoe_sha256\":\"712ac98bd38043b9f7c7ba5b7ab9e5f438680644ddb922af7885f260002cfe35\",\"last_shoe_sha256\":\"d38c1172a196353b632d285b278ba8ec39c41e6aa04e0a22a1ed69c8ea80cff7\"},\"reference_strategy\":{\"version\":\"blackjack-basic-s17-das-total-v1\",\"source_url\":\"https://wizardofodds.com/games/blackjack/strategy/4-decks/\",\"source_author\":\"Michael Shackleford\",\"source_accessed\":\"2026-09-06\",\"scope\":\"4-8 decks, S17, DAS total-dependent basic strategy; applied to actual frozen 6-deck engine\",\"adaptation\":\"No surrender/insurance/even-money actions; ignore surrender priority and use source total-based fallback. Split only when engine allows, max 4 hands; split aces one card each, no resplit. A fresh full shoe per independent round, no counting or composition-dependent deviations.\",\"input_boundary\":\"PLAYER_TURN PublicView only; exactly one dealer upcard; active hand cards/value and legal actions only; no hidden card, protected State, future shoe, history or RNG input\",\"priority\":\"If legal and pair table says P: SPLIT. Else total table; DH = DOUBLE if legal else HIT, DS = DOUBLE if legal else STAND. P unavailable falls through to total table.\",\"legend\":\"H HIT; S STAND; DH DOUBLE/HIT fallback; DS DOUBLE/STAND fallback; P SPLIT; - total-table fallback; ace point = 1\",\"dealer_columns\":[2,3,4,5,6,7,8,9,10,1],\"hard_totals\":{\"10\":[\"DH\",\"DH\",\"DH\",\"DH\",\"DH\",\"DH\",\"DH\",\"DH\",\"H\",\"H\"],\"11\":[\"DH\",\"DH\",\"DH\",\"DH\",\"DH\",\"DH\",\"DH\",\"DH\",\"DH\",\"H\"],\"12\":[\"H\",\"H\",\"S\",\"S\",\"S\",\"H\",\"H\",\"H\",\"H\",\"H\"],\"13\":[\"S\",\"S\",\"S\",\"S\",\"S\",\"H\",\"H\",\"H\",\"H\",\"H\"],\"14\":[\"S\",\"S\",\"S\",\"S\",\"S\",\"H\",\"H\",\"H\",\"H\",\"H\"],\"15\":[\"S\",\"S\",\"S\",\"S\",\"S\",\"H\",\"H\",\"H\",\"H\",\"H\"],\"16\":[\"S\",\"S\",\"S\",\"S\",\"S\",\"H\",\"H\",\"H\",\"H\",\"H\"],\"17\":[\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\"],\"18\":[\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\"],\"19\":[\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\"],\"20\":[\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\"],\"21\":[\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\"],\"4\":[\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\"],\"5\":[\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\"],\"6\":[\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\"],\"7\":[\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\"],\"8\":[\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\"],\"9\":[\"H\",\"DH\",\"DH\",\"DH\",\"DH\",\"H\",\"H\",\"H\",\"H\",\"H\"]},\"soft_totals\":{\"12\":[\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\",\"H\"],\"13\":[\"H\",\"H\",\"H\",\"DH\",\"DH\",\"H\",\"H\",\"H\",\"H\",\"H\"],\"14\":[\"H\",\"H\",\"H\",\"DH\",\"DH\",\"H\",\"H\",\"H\",\"H\",\"H\"],\"15\":[\"H\",\"H\",\"DH\",\"DH\",\"DH\",\"H\",\"H\",\"H\",\"H\",\"H\"],\"16\":[\"H\",\"H\",\"DH\",\"DH\",\"DH\",\"H\",\"H\",\"H\",\"H\",\"H\"],\"17\":[\"H\",\"DH\",\"DH\",\"DH\",\"DH\",\"H\",\"H\",\"H\",\"H\",\"H\"],\"18\":[\"S\",\"DS\",\"DS\",\"DS\",\"DS\",\"S\",\"S\",\"H\",\"H\",\"H\"],\"19\":[\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\"],\"20\":[\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\"],\"21\":[\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\",\"S\"]},\"pair_point_values\":{\"1\":[\"P\",\"P\",\"P\",\"P\",\"P\",\"P\",\"P\",\"P\",\"P\",\"P\"],\"10\":[\"-\",\"-\",\"-\",\"-\",\"-\",\"-\",\"-\",\"-\",\"-\",\"-\"],\"2\":[\"P\",\"P\",\"P\",\"P\",\"P\",\"P\",\"-\",\"-\",\"-\",\"-\"],\"3\":[\"P\",\"P\",\"P\",\"P\",\"P\",\"P\",\"-\",\"-\",\"-\",\"-\"],\"4\":[\"-\",\"-\",\"-\",\"P\",\"P\",\"-\",\"-\",\"-\",\"-\",\"-\"],\"5\":[\"-\",\"-\",\"-\",\"-\",\"-\",\"-\",\"-\",\"-\",\"-\",\"-\"],\"6\":[\"P\",\"P\",\"P\",\"P\",\"P\",\"-\",\"-\",\"-\",\"-\",\"-\"],\"7\":[\"P\",\"P\",\"P\",\"P\",\"P\",\"P\",\"-\",\"-\",\"-\",\"-\"],\"8\":[\"P\",\"P\",\"P\",\"P\",\"P\",\"P\",\"P\",\"P\",\"P\",\"P\"],\"9\":[\"P\",\"P\",\"P\",\"P\",\"P\",\"-\",\"P\",\"P\",\"-\",\"-\"]}},\"reference_strategy_sha256\":\"44b7f0999ae6a3484ebfa15b773a82b339c38dd1229c29a38f1649b40b90a752\",\"exact_moments\":{\"quantum_units\":\"2500000\",\"sum_payout_quanta\":\"22553592\",\"sum_stake_quanta\":\"22627750\",\"sum_net_quanta\":\"-74158\",\"sum_payout_squared\":\"113094360\",\"sum_stake_squared\":\"56996812\",\"sum_payout_stake\":\"58425852\",\"sum_net_squared\":\"53239468\"},\"ordered_outcome_sha256\":\"4df99d06f24e8e43e2c25bfe54473a1ec34deb5a2cae80ad74bcacda6f11b0ee\",\"per_round_checks\":\"Actual blackjack.New/Apply/PublicView and final VerifyRecovery; \\u003c=4 hands, accepted additional stakes reconcile, hand totals reconcile, net=payout-stake, quantum divisibility, legal public-only action, finite action bound\",\"statistical_limitations\":\"Fixed public deterministic pseudorandom sample; confidence intervals describe Monte Carlo sampling error under independent uniform-shoe model, not an exact RTP proof, RNG certification, or production VERIFIED. Initial effective RTP=1+net/initial; raw payout/initial is separate and can exceed 1 when extra stakes are accepted. Total-stake ratio uses round-cluster delta-method variance; split hands are not independent observations.\"}","ruleset_version":"blackjack-rules-v1","source_artifact_sha256":"0dfe5045ac000979be8f5c0c3ffe28b78784a9bdeab3aa0b60cc141c707f81ca","source_binary_sha256":"72e317c9e810ddba8335ad4c09134ac1fefd4a34feec379a7b14fd70a7e862d2","source_result_sha256":"c513768fc465eb12f6647f2b393cd510433e8f43a7a3102a1db6939bbd8b418f","validation_build":"bbc866af6886d679f0874f09f962ef3831626f72","validator_version":"game-math-validator-v1"}'::jsonb,decode('14695d669c49d76dbc50a97c67e795cb1dbfb4190e02f69ad26d70477bba20ab','hex'),'VERIFIED',now());
UPDATE games.game_registry SET publication_state='PUBLISHED',configured_runtime_state='AVAILABLE',active_config_version_id='01993200-0000-7000-8000-000000000005' WHERE game_slug='blackjack';
