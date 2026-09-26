-- Bind economic authority before the hand's random stream is committed.
CREATE FUNCTION poker.guard_economic_binding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.economic_policy_version IS DISTINCT FROM OLD.economic_policy_version THEN
  RAISE EXCEPTION 'immutable hand economic authority' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER poker_economic_binding BEFORE UPDATE ON poker.hands FOR EACH ROW EXECUTE FUNCTION poker.guard_economic_binding();

CREATE FUNCTION poker.check_economic_settlement() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE h poker.hands;
BEGIN
 SELECT * INTO STRICT h FROM poker.hands WHERE hand_id=NEW.hand_id;
 IF h.state<>'SETTLED' OR h.economic_policy_version IS NULL THEN
  IF EXISTS(SELECT 1 FROM economy.cap_settlements WHERE source_kind='POKER_HAND' AND source_id=h.hand_id::text) THEN
   RAISE EXCEPTION 'unexpected poker cap receipt' USING ERRCODE='23514';
  END IF;
  RETURN NULL;
 END IF;
 IF EXISTS(SELECT 1 FROM poker.hand_participants p LEFT JOIN economy.cap_settlements c
    ON c.source_kind='POKER_HAND' AND c.source_id=p.hand_id::text AND c.newapi_user_id=p.newapi_user_id
    LEFT JOIN LATERAL (SELECT coalesce(sum(applied_delta_units::numeric) FILTER(WHERE event_type IN('POST_SB','POST_BB','CALL','BET','RAISE','ALL_IN')),0) paid,
      coalesce(sum(applied_delta_units::numeric) FILTER(WHERE event_type='RETURN_UNCALLED'),0) returned
      FROM poker.actions a WHERE a.hand_id=p.hand_id AND a.actor_seat=p.seat_no) a ON true
    WHERE p.hand_id=h.hand_id AND (c.source_id IS NULL OR c.policy_version<>h.economic_policy_version
      OR c.gross_payout_units<>a.returned+coalesce((SELECT sum(award_units::numeric) FROM poker.pot_awards w WHERE w.hand_id=p.hand_id AND w.seat_no=p.seat_no),0)
      OR c.actual_net_units<>c.credited_payout_units-a.paid
      OR c.withheld_units%500000<>0)) THEN
  RAISE EXCEPTION 'poker cap receipt mismatch' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER poker_cap_conservation AFTER INSERT OR UPDATE ON poker.hands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION poker.check_economic_settlement();
