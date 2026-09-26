-- Fixed one-time campaign. No account selection, grants or activation at install.
CREATE TABLE economy.gambler_campaigns (
 campaign_id uuid PRIMARY KEY,
 badge_code text NOT NULL DEFAULT 'gambler-ruler-202609' UNIQUE CHECK(badge_code='gambler-ruler-202609'),
 policy_version text NOT NULL DEFAULT 'economy-cap-v1' REFERENCES economy.policy_versions,
 phase text NOT NULL DEFAULT 'DRAFT' CHECK(phase IN('DRAFT','SNAPSHOT_PREPARING','SNAPSHOT_READY','GRANTED','RESET_PREPARING','RESET_READY','RESET_RUNNING','COMPLETED')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 cutoff_at timestamptz, snapshot_sealed_at timestamptz, granted_at timestamptz,
 reset_cutoff_at timestamptz, reset_prepared_at timestamptz, reset_started_at timestamptz, completed_at timestamptz,
 snapshot_target_hash text CHECK(snapshot_target_hash ~ '^[0-9a-f]{64}$'),
 reset_target_hash text CHECK(reset_target_hash ~ '^[0-9a-f]{64}$'),
 maintenance_id uuid REFERENCES ops.maintenance_windows,
 accepted_by bigint REFERENCES identity.account_refs,
 accepted_epoch bigint, accepted_operation_id uuid REFERENCES ops.admin_operations,
 checkpoint jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(checkpoint)='object'),
 blocking_facts jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(blocking_facts)='array'),
 native_unchanged boolean NOT NULL DEFAULT true,
 policy_activated_at timestamptz, rankings_published_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE economy.gambler_campaign_accounts (
 campaign_id uuid NOT NULL REFERENCES economy.gambler_campaigns,
 newapi_user_id bigint NOT NULL REFERENCES identity.account_refs,
 snapshot_member boolean NOT NULL DEFAULT false,
 snapshot_assets jsonb CHECK(snapshot_assets IS NULL OR jsonb_typeof(snapshot_assets)='object'),
 snapshot_digest text CHECK(snapshot_digest ~ '^[0-9a-f]{64}$'),
 snapshot_at timestamptz, eligible boolean NOT NULL DEFAULT false,
 reset_member boolean NOT NULL DEFAULT false,
 reset_before jsonb CHECK(reset_before IS NULL OR jsonb_typeof(reset_before)='object'),
 reset_after jsonb CHECK(reset_after IS NULL OR jsonb_typeof(reset_after)='object'),
 reset_receipt jsonb CHECK(reset_receipt IS NULL OR jsonb_typeof(reset_receipt)='object'),
 reset_state text NOT NULL DEFAULT 'PENDING' CHECK(reset_state IN('PENDING','CHECKED','COMPLETED')),
 reset_at timestamptz,
 PRIMARY KEY(campaign_id,newapi_user_id),
 CHECK(NOT eligible OR (snapshot_member AND snapshot_assets IS NOT NULL AND (snapshot_assets->>'total_units')::numeric>500000000000000)),
 CHECK(reset_state<>'COMPLETED' OR (reset_member AND reset_before IS NOT NULL AND reset_after IS NOT NULL AND reset_receipt IS NOT NULL AND reset_at IS NOT NULL))
);
CREATE TABLE identity.account_badges (
 newapi_user_id bigint NOT NULL REFERENCES identity.account_refs,
 badge_code text NOT NULL CHECK(badge_code='gambler-ruler-202609'),
 campaign_id uuid NOT NULL,
 awarded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(newapi_user_id,badge_code),
 FOREIGN KEY(campaign_id,newapi_user_id) REFERENCES economy.gambler_campaign_accounts
);
CREATE TRIGGER account_badges_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON identity.account_badges FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE FUNCTION economy.guard_gambler_campaign() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.campaign_id<>OLD.campaign_id OR NEW.badge_code<>OLD.badge_code OR NEW.policy_version<>OLD.policy_version
  OR NEW.version<>OLD.version+1 OR (OLD.snapshot_sealed_at IS NOT NULL AND
   (NEW.cutoff_at,NEW.snapshot_sealed_at,NEW.snapshot_target_hash) IS DISTINCT FROM (OLD.cutoff_at,OLD.snapshot_sealed_at,OLD.snapshot_target_hash))
  OR (OLD.reset_prepared_at IS NOT NULL AND
   (NEW.reset_cutoff_at,NEW.reset_prepared_at,NEW.reset_target_hash) IS DISTINCT FROM (OLD.reset_cutoff_at,OLD.reset_prepared_at,OLD.reset_target_hash))
  OR (NEW.phase<>OLD.phase AND (OLD.phase,NEW.phase) NOT IN
   (('DRAFT','SNAPSHOT_PREPARING'),('SNAPSHOT_PREPARING','SNAPSHOT_READY'),('SNAPSHOT_READY','GRANTED'),('GRANTED','RESET_PREPARING'),('RESET_PREPARING','RESET_READY'),('RESET_READY','RESET_RUNNING'),('RESET_RUNNING','COMPLETED'))) THEN
  RAISE EXCEPTION 'immutable campaign authority or invalid transition' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER gambler_campaign_guard BEFORE UPDATE ON economy.gambler_campaigns FOR EACH ROW EXECUTE FUNCTION economy.guard_gambler_campaign();
CREATE TRIGGER gambler_campaign_no_delete BEFORE DELETE OR TRUNCATE ON economy.gambler_campaigns FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE FUNCTION economy.guard_gambler_account() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c economy.gambler_campaigns;
BEGIN
 SELECT * INTO STRICT c FROM economy.gambler_campaigns WHERE campaign_id=NEW.campaign_id;
 IF TG_OP='INSERT' THEN
  IF (c.snapshot_sealed_at IS NOT NULL AND NEW.snapshot_member) OR (c.reset_prepared_at IS NOT NULL AND NEW.reset_member) THEN
   RAISE EXCEPTION 'sealed campaign membership' USING ERRCODE='55000';
  END IF;
 ELSE
  IF (NEW.campaign_id,NEW.newapi_user_id) IS DISTINCT FROM (OLD.campaign_id,OLD.newapi_user_id)
    OR (c.snapshot_sealed_at IS NOT NULL AND
      (NEW.snapshot_member,NEW.snapshot_assets,NEW.snapshot_digest,NEW.snapshot_at,NEW.eligible) IS DISTINCT FROM (OLD.snapshot_member,OLD.snapshot_assets,OLD.snapshot_digest,OLD.snapshot_at,OLD.eligible))
    OR (c.reset_prepared_at IS NOT NULL AND (NEW.reset_member,NEW.reset_before) IS DISTINCT FROM (OLD.reset_member,OLD.reset_before))
    OR (OLD.reset_state='COMPLETED' AND NEW IS DISTINCT FROM OLD) THEN
   RAISE EXCEPTION 'immutable campaign account snapshot' USING ERRCODE='55000';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER gambler_account_guard BEFORE INSERT OR UPDATE ON economy.gambler_campaign_accounts FOR EACH ROW EXECUTE FUNCTION economy.guard_gambler_account();
CREATE TRIGGER gambler_accounts_no_delete BEFORE DELETE OR TRUNCATE ON economy.gambler_campaign_accounts FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE FUNCTION identity.check_gambler_award() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM economy.gambler_campaign_accounts a JOIN economy.gambler_campaigns c USING(campaign_id)
   WHERE a.campaign_id=NEW.campaign_id AND a.newapi_user_id=NEW.newapi_user_id AND a.eligible AND a.snapshot_member
    AND (a.snapshot_assets->>'total_units')::numeric>500000000000000 AND c.snapshot_sealed_at IS NOT NULL AND c.badge_code=NEW.badge_code) THEN
  RAISE EXCEPTION 'gambler qualification not sealed' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER gambler_award_check BEFORE INSERT ON identity.account_badges FOR EACH ROW EXECUTE FUNCTION identity.check_gambler_award();
