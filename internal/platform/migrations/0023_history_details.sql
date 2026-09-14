-- Source-authorized read capabilities; the immutable 0021 snapshot/index stay unchanged.
CREATE FUNCTION games.history_record_snapshot(p_user bigint,p_type text,p_id uuid) RETURNS jsonb
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog SET TimeZone='UTC' AS $$
DECLARE owned boolean; result jsonb;
BEGIN
 IF p_user<=0 OR p_user IS NULL OR p_id IS NULL THEN RAISE EXCEPTION 'HISTORY_INVALID' USING ERRCODE='22023'; END IF;
 CASE p_type
 WHEN 'DIRECT_PLAY_ROUND' THEN SELECT EXISTS(SELECT 1 FROM games.game_rounds WHERE round_id=p_id AND newapi_user_id=p_user) INTO owned;
 WHEN 'POKER_SESSION' THEN SELECT EXISTS(SELECT 1 FROM poker.sessions WHERE session_id=p_id AND newapi_user_id=p_user) INTO owned;
 WHEN 'POKER_HAND' THEN SELECT EXISTS(SELECT 1 FROM poker.hand_participants WHERE hand_id=p_id AND newapi_user_id=p_user) INTO owned;
 ELSE RAISE EXCEPTION 'HISTORY_INVALID' USING ERRCODE='22023';
 END CASE;
 IF NOT owned THEN RETURN NULL; END IF;
 SELECT jsonb_build_object('snapshot_id',s.snapshot_id,'game_slug',s.game_slug,'game_title',s.game_title,
  'table_id',s.table_id,'table_name',s.table_name,'actor_display_name',s.actor_display_name,
  'metadata_origin',s.metadata_origin,'captured_at',s.captured_at) INTO result
 FROM games.history_display_snapshots s WHERE s.newapi_user_id=p_user AND s.record_type=p_type AND s.source_id=p_id;
 IF NOT FOUND THEN RAISE EXCEPTION 'HISTORY_SOURCE_UNAVAILABLE'; END IF;
 RETURN result;
END $$;

CREATE FUNCTION economy.history_transaction_read(p_user bigint,p_id uuid) RETURNS jsonb
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
   CASE WHEN r.wager_transaction_id=p_id THEN -(r.total_stake_units::numeric-coalesce((SELECT sum(a.additional_stake_units::numeric) FROM games.round_actions a WHERE a.round_id=r.round_id),0)) ELSE r.total_payout_units::numeric END delta,
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
 )
 SELECT count(*),min(kind),min(id::text)::uuid,min(delta),min(ledger::text)::uuid,
  coalesce(bool_or(uid<>p_user OR biz<>t.biz_type OR operation<>t.operation_type OR business_id<>t.biz_id OR NOT coalesce(valid,false)),false)
 INTO refs,source_type,source_id,expected,expected_ledger,invalid FROM sources;
 IF invalid OR refs>1 OR (t.operation_type IN('GAME_WAGER','GAME_ADDITIONAL_WAGER','GAME_PAYOUT','POKER_BUY_IN','POKER_TOP_UP','POKER_REBUY','POKER_CASH_OUT') AND refs<>1)
  OR (refs=1 AND ((t.operation_type IN('GAME_WAGER','GAME_ADDITIONAL_WAGER','POKER_BUY_IN','POKER_TOP_UP','POKER_REBUY') AND expected>=0) OR (t.operation_type IN('GAME_PAYOUT','POKER_CASH_OUT') AND expected<0)))
  OR (n=0 AND (refs<>1 OR expected<>0 OR t.operation_type NOT IN('GAME_PAYOUT','POKER_CASH_OUT')))
  OR (refs=1 AND ((expected=0 AND n<>0) OR (expected<>0 AND (n<>1 OR debit<>expected OR effects->0->>'asset'<>'AVAILABLE_CHIPS'))))
  OR (expected_ledger IS NOT NULL AND (n<>1 OR effects->0->>'ledger_id'<>expected_ledger::text))
 THEN RAISE EXCEPTION 'HISTORY_SOURCE_UNAVAILABLE'; END IF;
 IF refs=1 THEN links:=jsonb_build_array(jsonb_build_object('record_type',source_type,'source_id',source_id)); END IF;
 RETURN jsonb_build_object('id',t.transaction_id,'kind',t.operation_type,'status',t.status,
  'created_at',t.created_at,'confirmed_at',t.confirmed_at,'effects',effects,'links',links);
END $$;
REVOKE ALL ON FUNCTION games.history_record_snapshot(bigint,text,uuid),economy.history_transaction_read(bigint,uuid) FROM PUBLIC;
