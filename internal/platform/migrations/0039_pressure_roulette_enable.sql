-- Enable only the reviewed compiled Pressure v1 after native acceptance.
DO $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM games.game_registry WHERE game_slug='pressure-roulette'
  AND publication_state='COMING_SOON' AND configured_runtime_state='AVAILABLE'
  AND implementation_key='roulette.pressure.v1'
  AND active_config_version_id='019a6000-0000-7000-8000-000000000032') THEN
  RAISE EXCEPTION 'pressure roulette catalog precondition differs';
 END IF;
 UPDATE games.game_registry SET publication_state='PUBLISHED',version=version+1 WHERE game_slug='pressure-roulette';
END $$;
