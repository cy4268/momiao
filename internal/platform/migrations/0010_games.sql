-- IS-06 first three Direct Play games. Published migrations 0001-0009 stay intact.
CREATE SCHEMA games;
CREATE TABLE games.wager_policy_versions (
 wager_policy_version_id UUID PRIMARY KEY,
 version_number BIGINT NOT NULL UNIQUE,
 minimum_wager_units BIGINT NOT NULL CHECK (minimum_wager_units=5000000),
 maximum_mode TEXT NOT NULL CHECK (maximum_mode='NONE'),
 input_step_units BIGINT NOT NULL CHECK (input_step_units=500000),
 quick_amount_units BIGINT[] NOT NULL CHECK (quick_amount_units=ARRAY[5000000,50000000,250000000,500000000]::bigint[]),
 policy_hash BYTEA NOT NULL CHECK (octet_length(policy_hash)=32)
);
CREATE TABLE games.runtime_gate (
 singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
 maintenance BOOLEAN NOT NULL DEFAULT FALSE,
 active_wager_policy_version_id UUID NOT NULL REFERENCES games.wager_policy_versions,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE games.game_registry (
 game_slug TEXT PRIMARY KEY,
 title TEXT NOT NULL,
 sort_order INTEGER NOT NULL,
 publication_state TEXT NOT NULL CHECK (publication_state IN ('DRAFT','PUBLISHED','COMING_SOON','RETIRED')),
 configured_runtime_state TEXT NOT NULL CHECK (configured_runtime_state IN ('AVAILABLE','MAINTENANCE','UNAVAILABLE')),
 implementation_key TEXT NOT NULL,
 active_config_version_id UUID,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE games.game_config_versions (
 config_version_id UUID PRIMARY KEY,
 game_slug TEXT NOT NULL REFERENCES games.game_registry,
 parent_version_id UUID REFERENCES games.game_config_versions,
 config_schema_version TEXT NOT NULL,
 ruleset_version TEXT NOT NULL,
 algorithm_version TEXT NOT NULL,
 config_payload JSONB NOT NULL,
 canonical_payload BYTEA NOT NULL,
 config_hash BYTEA NOT NULL CHECK (octet_length(config_hash)=32),
 resource_versions JSONB NOT NULL DEFAULT '{}',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(config_version_id,game_slug),
 CHECK (convert_from(canonical_payload,'UTF8')::jsonb = config_payload)
);
ALTER TABLE games.game_registry ADD FOREIGN KEY(active_config_version_id,game_slug) REFERENCES games.game_config_versions(config_version_id,game_slug);
CREATE TABLE games.game_validation_artifacts (
 validation_artifact_id UUID PRIMARY KEY,
 game_slug TEXT NOT NULL,
 artifact_type TEXT NOT NULL,
 implementation_key TEXT NOT NULL,
 ruleset_version TEXT NOT NULL,
 algorithm_version TEXT NOT NULL,
 config_version_id UUID NOT NULL UNIQUE,
 config_hash BYTEA NOT NULL CHECK (octet_length(config_hash)=32),
 validator_version TEXT NOT NULL,
 validation_build TEXT NOT NULL,
 result_summary JSONB NOT NULL,
 artifact_sha256 BYTEA NOT NULL CHECK (octet_length(artifact_sha256)=32),
 status TEXT NOT NULL CHECK (status IN ('GENERATED','VERIFIED','REJECTED','SUPERSEDED')),
 generated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 verified_at TIMESTAMPTZ,
 FOREIGN KEY(config_version_id,game_slug) REFERENCES games.game_config_versions(config_version_id,game_slug),
 CHECK (status <> 'VERIFIED' OR verified_at IS NOT NULL)
);
CREATE TABLE games.client_seed_preferences (
 newapi_user_id BIGINT NOT NULL REFERENCES identity.account_refs,
 game_slug TEXT NOT NULL REFERENCES games.game_registry,
 client_seed TEXT NOT NULL CHECK (octet_length(client_seed) BETWEEN 1 AND 128),
 version BIGINT NOT NULL DEFAULT 1 CHECK (version>0),
 PRIMARY KEY(newapi_user_id,game_slug)
);
CREATE TABLE games.fairness_nonce_cursors (
 newapi_user_id BIGINT NOT NULL REFERENCES identity.account_refs,
 game_slug TEXT NOT NULL REFERENCES games.game_registry,
 next_nonce BIGINT NOT NULL DEFAULT 0 CHECK (next_nonce>=0),
 exhausted BOOLEAN NOT NULL DEFAULT FALSE,
 PRIMARY KEY(newapi_user_id,game_slug)
);
CREATE TABLE games.fairness_commitments (
 commitment_id UUID PRIMARY KEY,
 reserved_round_id UUID NOT NULL UNIQUE,
 newapi_user_id BIGINT NOT NULL REFERENCES identity.account_refs,
 game_slug TEXT NOT NULL REFERENCES games.game_registry,
 nonce BIGINT NOT NULL CHECK (nonce>=0),
 client_seed TEXT NOT NULL,
 client_seed_version BIGINT NOT NULL CHECK (client_seed_version>0),
 state TEXT NOT NULL CHECK (state IN ('AVAILABLE','CONSUMED','REVEALED','INVALIDATED')),
 server_seed_hash BYTEA NOT NULL CHECK (octet_length(server_seed_hash)=32),
 key_version TEXT NOT NULL,
 gcm_nonce BYTEA NOT NULL CHECK (octet_length(gcm_nonce)=12),
 ciphertext BYTEA NOT NULL CHECK (octet_length(ciphertext)=48),
 revealed_server_seed BYTEA CHECK (octet_length(revealed_server_seed)=32),
 ruleset_version TEXT NOT NULL,
 algorithm_version TEXT NOT NULL,
 fairness_stream_version TEXT NOT NULL,
 game_config_version_id UUID NOT NULL,
 game_config_hash BYTEA NOT NULL CHECK (octet_length(game_config_hash)=32),
 wager_policy_version_id UUID NOT NULL REFERENCES games.wager_policy_versions,
 wager_policy_hash BYTEA NOT NULL CHECK (octet_length(wager_policy_hash)=32),
 resource_versions JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(game_config_version_id,game_slug) REFERENCES games.game_config_versions(config_version_id,game_slug),
 UNIQUE(newapi_user_id,game_slug,nonce),
 UNIQUE(commitment_id,reserved_round_id,newapi_user_id,game_slug),
 CHECK ((state='REVEALED')=(revealed_server_seed IS NOT NULL))
);
CREATE UNIQUE INDEX one_available_commitment ON games.fairness_commitments(newapi_user_id,game_slug) WHERE state='AVAILABLE';
CREATE TABLE games.game_rounds (
 round_id UUID PRIMARY KEY,
 newapi_user_id BIGINT NOT NULL REFERENCES identity.account_refs,
 game_slug TEXT NOT NULL REFERENCES games.game_registry,
 commitment_id UUID NOT NULL UNIQUE,
 idempotency_key_hash BYTEA NOT NULL CHECK (octet_length(idempotency_key_hash)=32),
 request_hash BYTEA NOT NULL CHECK (octet_length(request_hash)=32),
 typed_input JSONB NOT NULL,
 implementation_key TEXT NOT NULL,
 game_config_version_id UUID NOT NULL,
 game_config_hash BYTEA NOT NULL CHECK (octet_length(game_config_hash)=32),
 wager_policy_version_id UUID NOT NULL REFERENCES games.wager_policy_versions,
 wager_policy_hash BYTEA NOT NULL CHECK (octet_length(wager_policy_hash)=32),
 fairness_stream_version TEXT NOT NULL,
 ruleset_version TEXT NOT NULL,
 algorithm_version TEXT NOT NULL,
 nonce BIGINT NOT NULL CHECK (nonce>=0),
 state TEXT NOT NULL CHECK (state='SETTLED'),
 recovery_state TEXT NOT NULL DEFAULT 'NORMAL' CHECK (recovery_state='NORMAL'),
 total_stake_units BIGINT NOT NULL CHECK (total_stake_units>0),
 total_payout_units BIGINT NOT NULL CHECK (total_payout_units>=0),
 net_change_units BIGINT NOT NULL,
 common_result TEXT NOT NULL CHECK (common_result IN ('WIN','BREAK_EVEN','LOSS')),
 balance_before_units BIGINT NOT NULL CHECK (balance_before_units>=0),
 balance_after_units BIGINT NOT NULL CHECK (balance_after_units>=0),
 wager_transaction_id UUID NOT NULL UNIQUE REFERENCES economy.asset_transactions,
 settlement_transaction_id UUID NOT NULL UNIQUE REFERENCES economy.asset_transactions,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 settled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(game_config_version_id,game_slug) REFERENCES games.game_config_versions(config_version_id,game_slug),
 FOREIGN KEY(commitment_id,round_id,newapi_user_id,game_slug) REFERENCES games.fairness_commitments(commitment_id,reserved_round_id,newapi_user_id,game_slug),
 UNIQUE(newapi_user_id,game_slug,idempotency_key_hash),
 UNIQUE(newapi_user_id,game_slug,nonce),
 UNIQUE(round_id,game_slug),
 CHECK (net_change_units::numeric=total_payout_units::numeric-total_stake_units::numeric),
 CHECK (balance_after_units::numeric=balance_before_units::numeric+net_change_units::numeric),
 CHECK ((net_change_units>0 AND common_result='WIN') OR (net_change_units=0 AND common_result='BREAK_EVEN') OR (net_change_units<0 AND common_result='LOSS'))
);
CREATE INDEX round_history ON games.game_rounds(newapi_user_id,created_at DESC,round_id DESC);
CREATE TABLE games.dice_results (
 round_id UUID PRIMARY KEY,
 game_slug TEXT GENERATED ALWAYS AS ('dice'::text) STORED,
 die_1 SMALLINT NOT NULL CHECK (die_1 BETWEEN 1 AND 6),
 die_2 SMALLINT NOT NULL CHECK (die_2 BETWEEN 1 AND 6),
 die_3 SMALLINT NOT NULL CHECK (die_3 BETWEEN 1 AND 6),
 choice TEXT NOT NULL CHECK (choice IN ('BIG','SMALL')),
 FOREIGN KEY(round_id,game_slug) REFERENCES games.game_rounds(round_id,game_slug)
);
CREATE TABLE games.scratch_results (
 round_id UUID PRIMARY KEY,
 game_slug TEXT GENERATED ALWAYS AS ('scratch'::text) STORED,
 prize_tier TEXT NOT NULL,
 payout_multiplier BIGINT NOT NULL CHECK (payout_multiplier IN (0,1,2,3,5,10,25,100)),
 presentation_completed_at TIMESTAMPTZ,
 FOREIGN KEY(round_id,game_slug) REFERENCES games.game_rounds(round_id,game_slug)
);
CREATE TABLE games.scratch_cells (
 round_id UUID NOT NULL REFERENCES games.scratch_results,
 cell_index SMALLINT NOT NULL CHECK (cell_index BETWEEN 0 AND 8),
 symbol TEXT NOT NULL CHECK (symbol IN ('P1','P2','P3','P5','P10','P25','P100')),
 is_matching_symbol BOOLEAN NOT NULL,
 PRIMARY KEY(round_id,cell_index)
);
CREATE TABLE games.summon_results (
 round_id UUID PRIMARY KEY,
 game_slug TEXT GENERATED ALWAYS AS ('summon'::text) STORED,
 mode TEXT NOT NULL CHECK (mode IN ('SINGLE','TENFOLD')),
 highest_tier TEXT NOT NULL CHECK (highest_tier IN ('T0','T1','T2','T3','T4','T5')),
 FOREIGN KEY(round_id,game_slug) REFERENCES games.game_rounds(round_id,game_slug)
);
CREATE TABLE games.summon_draws (
 round_id UUID NOT NULL REFERENCES games.summon_results,
 draw_index SMALLINT NOT NULL CHECK (draw_index BETWEEN 1 AND 10),
 prize_tier TEXT NOT NULL CHECK (prize_tier IN ('T0','T1','T2','T3','T4','T5')),
 payout_multiplier BIGINT NOT NULL CHECK (payout_multiplier IN (0,1,2,5,20,100)),
 PRIMARY KEY(round_id,draw_index)
);
CREATE TABLE games.supply_events (
 round_id UUID PRIMARY KEY REFERENCES games.game_rounds,
 event_type TEXT NOT NULL CHECK (event_type IN ('GAME_ISSUANCE','GAME_BURN')),
 direction TEXT NOT NULL CHECK (direction IN ('ISSUE','BURN')),
 amount_units BIGINT NOT NULL CHECK (amount_units>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK ((event_type='GAME_ISSUANCE' AND direction='ISSUE') OR (event_type='GAME_BURN' AND direction='BURN'))
);
CREATE FUNCTION games.guard_commitment_transition() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (to_jsonb(NEW)-'state'-'revealed_server_seed') IS DISTINCT FROM (to_jsonb(OLD)-'state'-'revealed_server_seed')
 OR NOT ((OLD.state='AVAILABLE' AND NEW.state IN ('CONSUMED','INVALIDATED')) OR (OLD.state='CONSUMED' AND NEW.state='REVEALED')) THEN
  RAISE EXCEPTION 'illegal commitment transition' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER commitment_transition BEFORE UPDATE ON games.fairness_commitments FOR EACH ROW EXECUTE FUNCTION games.guard_commitment_transition();
CREATE FUNCTION games.guard_scratch_presentation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 -- Stored generated columns are recomputed after BEFORE triggers. game_slug is
 -- not caller-writable and must not be compared before that recomputation.
 IF (to_jsonb(NEW)-'presentation_completed_at'-'game_slug') IS DISTINCT FROM (to_jsonb(OLD)-'presentation_completed_at'-'game_slug')
 OR OLD.presentation_completed_at IS NOT NULL OR NEW.presentation_completed_at IS NULL THEN
  RAISE EXCEPTION 'immutable scratch result' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER scratch_presentation BEFORE UPDATE ON games.scratch_results FOR EACH ROW EXECUTE FUNCTION games.guard_scratch_presentation();
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['wager_policy_versions','game_config_versions','game_rounds','dice_results','scratch_cells','summon_results','summon_draws','supply_events'] LOOP
  EXECUTE format('CREATE TRIGGER immutable_history BEFORE UPDATE OR DELETE OR TRUNCATE ON games.%I FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change()',t);
 END LOOP;
END $$;
-- All three implementations are atomic fast games: no paid nonterminal row can
-- commit. Deferred checks bind one debit, one settlement, typed result and net
-- supply to the durable round. They also make injected commit failures roll back.
CREATE FUNCTION games.check_fast_settlement() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c games.fairness_commitments; n bigint; paid bigint; BEGIN
 SELECT * INTO STRICT c FROM games.fairness_commitments WHERE commitment_id=NEW.commitment_id;
 IF c.state<>'REVEALED' OR c.nonce<>NEW.nonce OR c.game_config_hash<>NEW.game_config_hash OR c.game_config_version_id<>NEW.game_config_version_id
 OR c.wager_policy_hash<>NEW.wager_policy_hash OR c.wager_policy_version_id<>NEW.wager_policy_version_id OR c.ruleset_version<>NEW.ruleset_version
 OR c.algorithm_version<>NEW.algorithm_version OR c.fairness_stream_version<>NEW.fairness_stream_version THEN RAISE EXCEPTION 'round commitment mismatch'; END IF;
 SELECT count(*) INTO n FROM economy.wallet_ledger WHERE transaction_id=NEW.wager_transaction_id AND newapi_user_id=NEW.newapi_user_id
  AND asset_type='AVAILABLE_CHIPS' AND entry_type='GAME_WAGER' AND delta_units=-NEW.total_stake_units AND balance_before_units=NEW.balance_before_units;
 IF n<>1 THEN RAISE EXCEPTION 'missing wager ledger'; END IF;
 SELECT count(*),coalesce(sum(delta_units),0) INTO n,paid FROM economy.wallet_ledger WHERE transaction_id=NEW.settlement_transaction_id AND newapi_user_id=NEW.newapi_user_id AND asset_type='AVAILABLE_CHIPS' AND entry_type='GAME_PAYOUT';
 IF paid<>NEW.total_payout_units OR (NEW.total_payout_units>0 AND n<>1) OR (NEW.total_payout_units=0 AND n<>0) THEN RAISE EXCEPTION 'settlement ledger mismatch'; END IF;
 SELECT count(*) INTO n FROM games.supply_events WHERE round_id=NEW.round_id AND amount_units=abs(NEW.net_change_units)
  AND direction=CASE WHEN NEW.net_change_units>0 THEN 'ISSUE' ELSE 'BURN' END;
 IF (NEW.net_change_units<>0 AND n<>1) OR (NEW.net_change_units=0 AND EXISTS(SELECT 1 FROM games.supply_events WHERE round_id=NEW.round_id)) THEN RAISE EXCEPTION 'net supply mismatch'; END IF;
 IF NEW.game_slug='dice' AND NOT EXISTS(SELECT 1 FROM games.dice_results WHERE round_id=NEW.round_id) THEN RAISE EXCEPTION 'missing dice result'; END IF;
 IF NEW.game_slug='scratch' AND (SELECT count(*) FROM games.scratch_cells WHERE round_id=NEW.round_id)<>9 THEN RAISE EXCEPTION 'missing scratch cells'; END IF;
 IF NEW.game_slug='summon' AND (SELECT count(*) FROM games.summon_draws WHERE round_id=NEW.round_id)<>(CASE WHEN NEW.typed_input->>'mode'='TENFOLD' THEN 10 ELSE 1 END) THEN RAISE EXCEPTION 'missing summon draws'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER fast_settlement_complete AFTER INSERT ON games.game_rounds DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION games.check_fast_settlement();
