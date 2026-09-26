-- Additive follow-up: 0041 is already committed and retains its checksum.
CREATE OR REPLACE FUNCTION roulette.check_conservation() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE actual numeric; withheld numeric; r roulette.rounds;
BEGIN
 SELECT * INTO STRICT r FROM roulette.rounds WHERE round_id=NEW.round_id;
 SELECT coalesce(sum(CASE WHEN kind='ESCROW' THEN amount_units::numeric ELSE -amount_units::numeric END),0) INTO actual FROM roulette.funding WHERE round_id=r.round_id;
 SELECT coalesce(sum(withheld_units::numeric),0) INTO withheld FROM economy.cap_settlements WHERE source_kind='ROULETTE_ROUND' AND source_id=r.round_id::text;
 IF r.state='FINISHED' AND r.economic_policy_version IS NOT NULL THEN
  IF EXISTS(SELECT 1 FROM roulette.participants p LEFT JOIN economy.cap_settlements c
    ON c.source_kind='ROULETTE_ROUND' AND c.source_id=r.round_id::text AND c.newapi_user_id=p.newapi_user_id
    WHERE p.round_id=r.round_id AND p.ready AND p.seat_no IS NOT NULL AND
    (c.source_id IS NULL OR c.policy_version<>r.economic_policy_version
     OR c.gross_payout_units<>coalesce((SELECT (a->>'amount_units')::bigint FROM jsonb_array_elements(r.outcome->'awards') a WHERE (a->>'user_id')::bigint=p.newapi_user_id),0)
     OR c.credited_payout_units<>coalesce((SELECT sum(amount_units::numeric) FROM roulette.funding f WHERE f.round_id=r.round_id AND f.newapi_user_id=p.newapi_user_id AND f.kind='PAYOUT'),0)
     OR c.actual_net_units<>c.credited_payout_units-r.stake_units)) THEN
   RAISE EXCEPTION 'roulette cap receipt mismatch' USING ERRCODE='23514';
  END IF;
 ELSIF EXISTS(SELECT 1 FROM economy.cap_settlements WHERE source_kind='ROULETTE_ROUND' AND source_id=r.round_id::text) THEN
  RAISE EXCEPTION 'unexpected roulette cap receipt' USING ERRCODE='23514';
 END IF;
 IF actual-withheld<>r.escrow_units THEN RAISE EXCEPTION 'roulette escrow mismatch' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END $$;