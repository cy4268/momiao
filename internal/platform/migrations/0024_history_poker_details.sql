CREATE FUNCTION poker.history_funding_transactions(p_subject bigint,p_session uuid,p_transaction_ids uuid[]) RETURNS jsonb
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog SET TimeZone='UTC' AS $$
DECLARE sess record; id uuid; valid boolean; item jsonb; result jsonb:='[]';
BEGIN
 IF p_subject IS NULL OR p_subject<=0 OR p_session IS NULL OR p_transaction_ids IS NULL
  OR cardinality(p_transaction_ids)>100
  OR (cardinality(p_transaction_ids)>0 AND array_ndims(p_transaction_ids)<>1)
  OR cardinality(p_transaction_ids)<>(SELECT count(DISTINCT x) FROM unnest(p_transaction_ids) x)
 THEN RAISE EXCEPTION 'HISTORY_INVALID' USING ERRCODE='22023'; END IF;
 SELECT table_id,seat_no INTO sess FROM poker.sessions WHERE session_id=p_session AND newapi_user_id=p_subject;
 IF NOT FOUND THEN RETURN NULL; END IF;
 FOREACH id IN ARRAY p_transaction_ids LOOP
  SELECT count(*)=1 AND coalesce(bool_and(f.newapi_user_id=p_subject AND f.table_id=sess.table_id
   AND f.seat_no=sess.seat_no AND f.state='CONFIRMED' AND f.confirmed_transaction_id=f.planned_transaction_id),false)
  INTO valid FROM poker.funding_operations f WHERE f.session_id=p_session AND f.confirmed_transaction_id=id;
  IF NOT valid THEN RAISE EXCEPTION 'HISTORY_SOURCE_UNAVAILABLE'; END IF;
  item:=economy.history_transaction_read(p_subject,id);
  IF item IS NULL OR NOT coalesce(item->>'id'=id::text AND jsonb_array_length(item->'links')=1
   AND item#>>'{links,0,record_type}'='POKER_SESSION' AND item#>>'{links,0,source_id}'=p_session::text,false)
  THEN RAISE EXCEPTION 'HISTORY_SOURCE_UNAVAILABLE'; END IF;
  result:=result||jsonb_build_array(item);
 END LOOP;
 RETURN result;
END $$;
REVOKE ALL ON FUNCTION poker.history_funding_transactions(bigint,uuid,uuid[]) FROM PUBLIC;
