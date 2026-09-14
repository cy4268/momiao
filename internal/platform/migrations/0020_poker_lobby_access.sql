-- The previous implementation had no password table capability: preserve its
-- actual PUBLIC semantics explicitly, without changing any existing chat flag.
ALTER TABLE poker.tables ADD COLUMN access_mode text NOT NULL DEFAULT 'PUBLIC'
 CHECK(access_mode IN ('PUBLIC','PASSWORD'));

-- Domain validation enforces 40 graphemes; HTTP bounds the complete payload.
-- The old 128-byte ceiling incorrectly rejected forty four-byte graphemes.
ALTER TABLE poker.tables DROP CONSTRAINT tables_name_check;
ALTER TABLE poker.tables ADD CONSTRAINT poker_table_name_not_empty
 CHECK(octet_length(name)>0);

-- Read in the caller's snapshot. The caller supplies only its verified user ID.
-- Missing rows remain missing; this never creates a wallet or exposes history.
CREATE FUNCTION economy.poker_lobby_balance(p_user_id bigint)
 RETURNS TABLE(balance_units bigint,version bigint)
 LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog AS $$
BEGIN
 IF p_user_id IS NULL OR p_user_id<=0 THEN
  RAISE EXCEPTION 'POKER_LOBBY_USER_INVALID' USING ERRCODE='22023';
 END IF;
 RETURN QUERY SELECT w.balance_units,w.version FROM economy.wallet_balances w
  WHERE w.newapi_user_id=p_user_id AND w.asset_type='AVAILABLE_CHIPS';
END $$;
REVOKE ALL ON FUNCTION economy.poker_lobby_balance(bigint) FROM PUBLIC;
