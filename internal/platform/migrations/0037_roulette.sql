-- Native fixed-stake roulette. Prior migrations stay byte-for-byte unchanged.
CREATE SCHEMA roulette;
CREATE TABLE roulette.rounds (
  round_id uuid PRIMARY KEY,
  game_slug text NOT NULL REFERENCES games.game_registry,
  title_snapshot text NOT NULL,
  host_user_id bigint NOT NULL REFERENCES identity.account_refs(newapi_user_id),
  target_players smallint NOT NULL,
  stake_units bigint NOT NULL CHECK(stake_units>0),
  config_version_id uuid NOT NULL,
  config_hash bytea NOT NULL CHECK(octet_length(config_hash)=32),
  policy_version_id uuid NOT NULL REFERENCES games.wager_policy_versions,
  policy_hash bytea NOT NULL CHECK(octet_length(policy_hash)=32),
  ruleset_version text NOT NULL, algorithm_version text NOT NULL, stream_version text NOT NULL,
  server_seed_hash bytea NOT NULL CHECK(octet_length(server_seed_hash)=32),
  seed_key_version text NOT NULL, seed_nonce bytea NOT NULL CHECK(octet_length(seed_nonce)=12),
  seed_ciphertext bytea NOT NULL CHECK(octet_length(seed_ciphertext)=48),
  revealed_seed bytea CHECK(octet_length(revealed_seed)=32),
  snapshot_key_version text NOT NULL, snapshot_nonce bytea NOT NULL CHECK(octet_length(snapshot_nonce)=12),
  snapshot_ciphertext bytea NOT NULL CHECK(octet_length(snapshot_ciphertext)>=16),
  snapshot_version bigint NOT NULL CHECK(snapshot_version>0),
  version bigint NOT NULL DEFAULT 1 CHECK(version>0),
  action_sequence bigint NOT NULL DEFAULT 0 CHECK(action_sequence>=0),
  state text NOT NULL CHECK(state IN('WAITING','PLAYING','SETTLING','FINISHED','CANCELLED','NEEDS_REVIEW')),
  escrow_units bigint NOT NULL DEFAULT 0 CHECK(escrow_units>=0),
  outcome jsonb CHECK(outcome IS NULL OR jsonb_typeof(outcome)='object'),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  started_at timestamptz, ended_at timestamptz, deadline timestamptz, game_deadline timestamptz,
  FOREIGN KEY(config_version_id,game_slug) REFERENCES games.game_config_versions(config_version_id,game_slug),
  CHECK((game_slug='devil-roulette' AND target_players=2) OR
        (game_slug='pressure-roulette' AND target_players BETWEEN 3 AND 6)),
  CHECK(stake_units::numeric*target_players<=9223372036854775807),
  CHECK(escrow_units::numeric<=stake_units::numeric*target_players),
  CHECK(revealed_seed IS NULL OR state IN('FINISHED','CANCELLED'))
);
CREATE INDEX roulette_due ON roulette.rounds(deadline,round_id)
  WHERE state IN('WAITING','PLAYING','SETTLING');
CREATE TABLE roulette.participants (
  round_id uuid NOT NULL REFERENCES roulette.rounds,
  newapi_user_id bigint NOT NULL REFERENCES identity.account_refs(newapi_user_id),
  seat_no smallint CHECK(seat_no BETWEEN 0 AND 5),
  ready_cycle bigint NOT NULL DEFAULT 0 CHECK(ready_cycle>=0),
  contribution_version bigint NOT NULL DEFAULT 0 CHECK(contribution_version>=0),
  ready boolean NOT NULL DEFAULT false, active boolean NOT NULL DEFAULT true,
  display_name_snapshot text NOT NULL,
  PRIMARY KEY(round_id,newapi_user_id),
  CHECK(NOT ready OR (seat_no IS NOT NULL AND ready_cycle>0 AND contribution_version>0))
);
CREATE UNIQUE INDEX roulette_one_active_seat ON roulette.participants(newapi_user_id) WHERE active;
CREATE UNIQUE INDEX roulette_room_seat ON roulette.participants(round_id,seat_no) WHERE seat_no IS NOT NULL;
CREATE TABLE roulette.commands (
  actor_user_id bigint NOT NULL REFERENCES identity.account_refs(newapi_user_id),
  key_hash bytea NOT NULL CHECK(octet_length(key_hash)=32),
  semantic_hash bytea NOT NULL CHECK(octet_length(semantic_hash)=32),
  round_id uuid NOT NULL REFERENCES roulette.rounds,
  receipt jsonb NOT NULL CHECK(jsonb_typeof(receipt)='object'),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  PRIMARY KEY(actor_user_id,key_hash)
);
CREATE TABLE roulette.actions (
  round_id uuid NOT NULL REFERENCES roulette.rounds,
  sequence bigint NOT NULL CHECK(sequence>0),
  actor_user_id bigint, actor_seat smallint CHECK(actor_seat BETWEEN 0 AND 5),
  input jsonb NOT NULL CHECK(jsonb_typeof(input)='object'),
  occurred_at timestamptz NOT NULL,
  domain text NOT NULL CHECK(domain='roulette/action/v1/'||sequence::text),
  state_hash bytea NOT NULL CHECK(octet_length(state_hash)=32),
  public_event jsonb NOT NULL CHECK(jsonb_typeof(public_event)='object'),
  PRIMARY KEY(round_id,sequence),
  FOREIGN KEY(round_id,actor_user_id) REFERENCES roulette.participants(round_id,newapi_user_id)
);
CREATE TABLE roulette.funding (
  funding_id uuid PRIMARY KEY,
  round_id uuid NOT NULL, newapi_user_id bigint NOT NULL,
  ready_cycle bigint NOT NULL CHECK(ready_cycle>0),
  kind text NOT NULL CHECK(kind IN('ESCROW','REFUND','PAYOUT','VOID_REFUND')),
  amount_units bigint NOT NULL CHECK(amount_units>0),
  biz_id text NOT NULL UNIQUE,
  transaction_id uuid NOT NULL UNIQUE REFERENCES economy.asset_transactions,
  ledger_id uuid NOT NULL UNIQUE REFERENCES economy.wallet_ledger(ledger_entry_id),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  FOREIGN KEY(round_id,newapi_user_id) REFERENCES roulette.participants(round_id,newapi_user_id),
  UNIQUE(round_id,newapi_user_id,ready_cycle,kind)
);
INSERT INTO games.game_registry(game_slug,title,sort_order,publication_state,configured_runtime_state,implementation_key)
VALUES('devil-roulette','恶魔轮盘',71,'PUBLISHED','AVAILABLE','roulette.devil.v1');
INSERT INTO games.game_config_versions(config_version_id,game_slug,config_schema_version,ruleset_version,algorithm_version,config_payload,canonical_payload,config_hash,version_number,status,validated_at,previewed_at,activated_at)
VALUES('019a6000-0000-7000-8000-000000000031','devil-roulette','momiao-roulette-config-v1','momiao-devil-rules-v1','momiao-devil-rng-v1','{"algorithm_version":"momiao-devil-rng-v1","game":"devil-roulette","game_seconds":1800,"rake_bps":0,"ruleset_version":"momiao-devil-rules-v1","turn_seconds":62,"upstream_commit":"6bfbda58b19e045062f4f5939bb9223d810eddb3","wait_seconds":300}'::jsonb,convert_to('{"algorithm_version":"momiao-devil-rng-v1","game":"devil-roulette","game_seconds":1800,"rake_bps":0,"ruleset_version":"momiao-devil-rules-v1","turn_seconds":62,"upstream_commit":"6bfbda58b19e045062f4f5939bb9223d810eddb3","wait_seconds":300}','UTF8'),decode('c6bd8f5b74f13d21a4fa6f44bd75824f8ab6aa13b3d786ccd84be7067eca6cd4','hex'),1,'ACTIVE',statement_timestamp(),statement_timestamp(),statement_timestamp());
INSERT INTO games.game_validation_artifacts(validation_artifact_id,game_slug,artifact_type,implementation_key,ruleset_version,algorithm_version,config_version_id,config_hash,validator_version,validation_build,result_summary,artifact_sha256,status,verified_at)
VALUES('019a6000-0000-7000-8000-000000000231','devil-roulette','MULTIPLAYER_RULESET','roulette.devil.v1','momiao-devil-rules-v1','momiao-devil-rng-v1','019a6000-0000-7000-8000-000000000031',decode('c6bd8f5b74f13d21a4fa6f44bd75824f8ab6aa13b3d786ccd84be7067eca6cd4','hex'),'momiao-roulette-validate-v1','momiao-roulette-v1','{"conservation":"funded=refunds+payouts+escrow","rake_bps":0,"validation":"COMPILED_RULESET","version":1}'::jsonb,decode('ad8cca2d2fe06aa38194fc056d2bc099ce67f0f9dbdaa5c01c8d7cb0f3ba4a67','hex'),'VERIFIED',statement_timestamp());
UPDATE games.game_registry SET active_config_version_id='019a6000-0000-7000-8000-000000000031' WHERE game_slug='devil-roulette';
INSERT INTO games.game_registry(game_slug,title,sort_order,publication_state,configured_runtime_state,implementation_key)
VALUES('pressure-roulette','加压轮盘',72,'COMING_SOON','AVAILABLE','roulette.pressure.v1');
INSERT INTO games.game_config_versions(config_version_id,game_slug,config_schema_version,ruleset_version,algorithm_version,config_payload,canonical_payload,config_hash,version_number,status,validated_at,previewed_at,activated_at)
VALUES('019a6000-0000-7000-8000-000000000032','pressure-roulette','momiao-roulette-config-v1','momiao-pressure-rules-v1','momiao-pressure-rng-v1','{"algorithm_version":"momiao-pressure-rng-v1","game":"pressure-roulette","game_seconds":1800,"rake_bps":0,"ruleset_version":"momiao-pressure-rules-v1","turn_seconds":[45,25,10],"upstream_commit":"6bfbda58b19e045062f4f5939bb9223d810eddb3","vote_seconds":35,"wait_seconds":300}'::jsonb,convert_to('{"algorithm_version":"momiao-pressure-rng-v1","game":"pressure-roulette","game_seconds":1800,"rake_bps":0,"ruleset_version":"momiao-pressure-rules-v1","turn_seconds":[45,25,10],"upstream_commit":"6bfbda58b19e045062f4f5939bb9223d810eddb3","vote_seconds":35,"wait_seconds":300}','UTF8'),decode('5085a169761f6124e409fe3c646b29465291cecc96e30becee0087b42aacda01','hex'),1,'ACTIVE',statement_timestamp(),statement_timestamp(),statement_timestamp());
INSERT INTO games.game_validation_artifacts(validation_artifact_id,game_slug,artifact_type,implementation_key,ruleset_version,algorithm_version,config_version_id,config_hash,validator_version,validation_build,result_summary,artifact_sha256,status,verified_at)
VALUES('019a6000-0000-7000-8000-000000000232','pressure-roulette','MULTIPLAYER_RULESET','roulette.pressure.v1','momiao-pressure-rules-v1','momiao-pressure-rng-v1','019a6000-0000-7000-8000-000000000032',decode('5085a169761f6124e409fe3c646b29465291cecc96e30becee0087b42aacda01','hex'),'momiao-roulette-validate-v1','momiao-roulette-v1','{"conservation":"funded=refunds+payouts+escrow","rake_bps":0,"validation":"COMPILED_RULESET","version":1}'::jsonb,decode('ad8cca2d2fe06aa38194fc056d2bc099ce67f0f9dbdaa5c01c8d7cb0f3ba4a67','hex'),'VERIFIED',statement_timestamp());
UPDATE games.game_registry SET active_config_version_id='019a6000-0000-7000-8000-000000000032' WHERE game_slug='pressure-roulette';

CREATE FUNCTION roulette.guard_round() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.state IN('FINISHED','CANCELLED') OR NEW.version<>OLD.version+1 OR NEW.action_sequence NOT BETWEEN OLD.action_sequence AND OLD.action_sequence+1 THEN
  RAISE EXCEPTION 'invalid roulette lifecycle' USING ERRCODE='55000';
 END IF;
 IF (to_jsonb(NEW)-ARRAY['snapshot_key_version','snapshot_nonce','snapshot_ciphertext','snapshot_version','version','action_sequence','state','escrow_units','outcome','started_at','ended_at','deadline','game_deadline','revealed_seed']) IS DISTINCT FROM
    (to_jsonb(OLD)-ARRAY['snapshot_key_version','snapshot_nonce','snapshot_ciphertext','snapshot_version','version','action_sequence','state','escrow_units','outcome','started_at','ended_at','deadline','game_deadline','revealed_seed']) THEN
  RAISE EXCEPTION 'immutable roulette binding' USING ERRCODE='55000';
 END IF;
 IF OLD.outcome IS NOT NULL AND NEW.outcome IS DISTINCT FROM OLD.outcome THEN RAISE EXCEPTION 'immutable roulette outcome' USING ERRCODE='55000'; END IF;
 IF NEW.snapshot_ciphertext IS DISTINCT FROM OLD.snapshot_ciphertext AND NEW.snapshot_version<>OLD.snapshot_version+1 THEN RAISE EXCEPTION 'invalid snapshot version' USING ERRCODE='55000'; END IF;
 IF OLD.state='WAITING' AND NEW.state NOT IN('WAITING','PLAYING','SETTLING','CANCELLED','NEEDS_REVIEW') OR
    OLD.state='PLAYING' AND NEW.state NOT IN('PLAYING','SETTLING','FINISHED','NEEDS_REVIEW') OR
    OLD.state='SETTLING' AND NEW.state NOT IN('SETTLING','FINISHED','CANCELLED','NEEDS_REVIEW') OR
    OLD.state='NEEDS_REVIEW' AND NEW.state NOT IN('NEEDS_REVIEW','CANCELLED') THEN RAISE EXCEPTION 'invalid roulette transition' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER roulette_round_guard BEFORE UPDATE ON roulette.rounds FOR EACH ROW EXECUTE FUNCTION roulette.guard_round();
CREATE TRIGGER immutable_history BEFORE DELETE OR TRUNCATE ON roulette.rounds FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TRIGGER immutable_history BEFORE DELETE OR TRUNCATE ON roulette.participants FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TRIGGER immutable_history BEFORE UPDATE OR DELETE OR TRUNCATE ON roulette.commands FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TRIGGER immutable_history BEFORE UPDATE OR DELETE OR TRUNCATE ON roulette.actions FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TRIGGER immutable_history BEFORE UPDATE OR DELETE OR TRUNCATE ON roulette.funding FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE FUNCTION roulette.check_conservation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE actual numeric; held bigint;
BEGIN
 SELECT coalesce(sum(CASE WHEN kind='ESCROW' THEN amount_units ELSE -amount_units END),0) INTO actual FROM roulette.funding WHERE round_id=NEW.round_id;
 SELECT escrow_units INTO held FROM roulette.rounds WHERE round_id=NEW.round_id;
 IF actual<>held THEN RAISE EXCEPTION 'roulette escrow mismatch' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER roulette_balance_round AFTER INSERT OR UPDATE ON roulette.rounds DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION roulette.check_conservation();
CREATE CONSTRAINT TRIGGER roulette_balance_funding AFTER INSERT ON roulette.funding DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION roulette.check_conservation();
REVOKE ALL ON ALL TABLES IN SCHEMA roulette FROM PUBLIC;
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA roulette FROM PUBLIC;
