-- Aggregate-only readiness: the campaign runtime receives no raw Poker grant.
CREATE FUNCTION economy.gambler_drain_read() RETURNS jsonb LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog AS $$
 SELECT jsonb_build_object(
  'quota_transfers',(SELECT count(*) FROM economy.quota_transfers WHERE status IN('PENDING','NEEDS_REVIEW')),
  'api_chips_exchanges',(SELECT count(*) FROM economy.api_chips_exchanges WHERE status NOT IN('CONFIRMED','COMPENSATED','FAILED_NO_EFFECT')),
  'solo_rounds',(SELECT count(*) FROM games.game_rounds WHERE state<>'SETTLED' OR recovery_state='NEEDS_REVIEW'),
  'roulette_rounds',(SELECT count(*) FROM roulette.rounds WHERE state NOT IN('FINISHED','CANCELLED')),
  'poker_hands',(SELECT count(*) FROM poker.hands WHERE state<>'SETTLED'),
  'poker_funds',(SELECT count(*) FROM poker.sessions WHERE state<>'SETTLED' AND current_stack_units>0),
  'poker_funding',(SELECT count(*) FROM poker.funding_operations WHERE state='PENDING'),
  'registration_grants',(SELECT count(*) FROM rewards.registration_grants WHERE status<>'CONFIRMED')
 )
$$;
REVOKE ALL ON FUNCTION economy.gambler_drain_read() FROM PUBLIC;
