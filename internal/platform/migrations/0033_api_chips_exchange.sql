-- Companion receipts own the saga. Existing confirmed asset transactions and
-- immutable wallet history are never repurposed as pending transfer records.
CREATE TABLE economy.api_chips_exchanges (
 exchange_id uuid PRIMARY KEY,
 newapi_user_id bigint NOT NULL REFERENCES identity.account_refs ON DELETE RESTRICT,
 request_key_hash bytea NOT NULL CHECK(octet_length(request_key_hash)=32),
 direction text NOT NULL DEFAULT 'API_TO_CHIPS' CHECK(direction='API_TO_CHIPS'),
 requested_units bigint NOT NULL CHECK(requested_units>0),
 reserve_debit_units bigint NOT NULL CHECK(reserve_debit_units>=0),
 active_debit_units bigint NOT NULL CHECK(active_debit_units>0 AND active_debit_units<=2147483647),
 chips_credit_units bigint NOT NULL CHECK(chips_credit_units=requested_units),
 debit_operation_id uuid NOT NULL UNIQUE,
 plan_hash bytea NOT NULL CHECK(octet_length(plan_hash)=32),
 status text NOT NULL DEFAULT 'PENDING' CHECK(status IN('PENDING','SOURCE_DEBITED','COMPENSATING','CONFIRMED','COMPENSATED','FAILED_NO_EFFECT','NEEDS_REVIEW')),
 reason text NOT NULL DEFAULT '',
 reserve_ledger_id uuid,
 chips_ledger_id uuid,
 reserve_refund_ledger_id uuid,
 refund_operation_id uuid UNIQUE,
 native_before bigint, native_after bigint,
 refund_before bigint, refund_after bigint,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(), confirmed_at timestamptz,
 UNIQUE(newapi_user_id,request_key_hash),
 FOREIGN KEY(reserve_ledger_id,newapi_user_id) REFERENCES economy.wallet_ledger(ledger_entry_id,newapi_user_id) ON DELETE RESTRICT,
 FOREIGN KEY(chips_ledger_id,newapi_user_id) REFERENCES economy.wallet_ledger(ledger_entry_id,newapi_user_id) ON DELETE RESTRICT,
 FOREIGN KEY(reserve_refund_ledger_id,newapi_user_id) REFERENCES economy.wallet_ledger(ledger_entry_id,newapi_user_id) ON DELETE RESTRICT,
 CHECK(reserve_debit_units::numeric+active_debit_units::numeric=requested_units::numeric),
 CHECK((reserve_debit_units=0)=(reserve_ledger_id IS NULL)),
 CHECK((native_before IS NULL)=(native_after IS NULL)),
 CHECK(native_before IS NULL OR (native_before>=0 AND native_after>=0 AND native_before-native_after=active_debit_units)),
 CHECK((refund_before IS NULL)=(refund_after IS NULL)),
 CHECK(refund_before IS NULL OR (refund_operation_id IS NOT NULL AND native_before IS NOT NULL AND refund_before>=0 AND refund_after>=0 AND refund_after-refund_before=active_debit_units)),
 CHECK(status<>'SOURCE_DEBITED' OR native_before IS NOT NULL),
 CHECK(status<>'COMPENSATING' OR (native_before IS NOT NULL AND refund_operation_id IS NOT NULL)),
 CHECK((status='CONFIRMED')=(chips_ledger_id IS NOT NULL)),
 CHECK((status='CONFIRMED')=(confirmed_at IS NOT NULL)),
 CHECK(status<>'CONFIRMED' OR (native_before IS NOT NULL AND refund_operation_id IS NULL AND reserve_refund_ledger_id IS NULL)),
 CHECK(status<>'COMPENSATED' OR ((reserve_debit_units=0 OR reserve_refund_ledger_id IS NOT NULL) AND (native_before IS NULL OR refund_before IS NOT NULL))),
 CHECK(status<>'FAILED_NO_EFFECT' OR (reserve_debit_units=0 AND native_before IS NULL))
);
CREATE UNIQUE INDEX api_chips_unresolved ON economy.api_chips_exchanges(newapi_user_id)
 WHERE status NOT IN('CONFIRMED','COMPENSATED','FAILED_NO_EFFECT');
CREATE INDEX api_chips_job ON economy.api_chips_exchanges(updated_at,exchange_id)
 WHERE status IN('PENDING','SOURCE_DEBITED','COMPENSATING');
CREATE INDEX api_chips_history ON economy.api_chips_exchanges(newapi_user_id,exchange_id DESC);
CREATE FUNCTION economy.guard_api_chips_exchange() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.exchange_id,NEW.newapi_user_id,NEW.request_key_hash,NEW.direction,NEW.requested_units,
 NEW.reserve_debit_units,NEW.active_debit_units,NEW.chips_credit_units,NEW.debit_operation_id,NEW.plan_hash,NEW.reserve_ledger_id,NEW.created_at)
 IS DISTINCT FROM (OLD.exchange_id,OLD.newapi_user_id,OLD.request_key_hash,OLD.direction,OLD.requested_units,
 OLD.reserve_debit_units,OLD.active_debit_units,OLD.chips_credit_units,OLD.debit_operation_id,OLD.plan_hash,OLD.reserve_ledger_id,OLD.created_at)
 OR OLD.status IN('CONFIRMED','COMPENSATED','FAILED_NO_EFFECT','NEEDS_REVIEW')
 OR (OLD.refund_operation_id IS NOT NULL AND NEW.refund_operation_id IS DISTINCT FROM OLD.refund_operation_id)
 OR (OLD.native_before IS NOT NULL AND (NEW.native_before,NEW.native_after) IS DISTINCT FROM (OLD.native_before,OLD.native_after))
 OR (OLD.refund_before IS NOT NULL AND (NEW.refund_before,NEW.refund_after) IS DISTINCT FROM (OLD.refund_before,OLD.refund_after))
 OR (OLD.reserve_refund_ledger_id IS NOT NULL AND NEW.reserve_refund_ledger_id IS DISTINCT FROM OLD.reserve_refund_ledger_id)
 OR NOT ((OLD.status='PENDING' AND NEW.status IN('PENDING','SOURCE_DEBITED','COMPENSATED','FAILED_NO_EFFECT','NEEDS_REVIEW'))
 OR (OLD.status='SOURCE_DEBITED' AND NEW.status IN('SOURCE_DEBITED','CONFIRMED','COMPENSATING','NEEDS_REVIEW'))
 OR (OLD.status='COMPENSATING' AND NEW.status IN('COMPENSATING','COMPENSATED','NEEDS_REVIEW'))) THEN
 RAISE EXCEPTION 'invalid immutable exchange plan or transition' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER api_chips_transition BEFORE UPDATE ON economy.api_chips_exchanges
 FOR EACH ROW EXECUTE FUNCTION economy.guard_api_chips_exchange();
CREATE TRIGGER api_chips_history BEFORE DELETE OR TRUNCATE ON economy.api_chips_exchanges
 FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
REVOKE ALL ON economy.api_chips_exchanges FROM PUBLIC;
REVOKE ALL ON FUNCTION economy.guard_api_chips_exchange() FROM PUBLIC;

-- Source-authorized whole-exchange history. A local leg detail resolves the
-- companion receipt instead of suggesting that one confirmed debit is success.
CREATE FUNCTION economy.history_api_chips_exchange_read(p_user bigint,p_id uuid) RETURNS jsonb
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog SET TimeZone='UTC' AS $$
DECLARE p economy.api_chips_exchanges; effects jsonb; native_effects jsonb:='[]'; invalid boolean;
BEGIN
 IF p_user IS NULL OR p_user<=0 OR p_id IS NULL THEN RAISE EXCEPTION 'HISTORY_INVALID' USING ERRCODE='22023'; END IF;
 SELECT e.* INTO p FROM economy.api_chips_exchanges e WHERE e.newapi_user_id=p_user AND
 (e.exchange_id=p_id OR EXISTS(SELECT 1 FROM economy.wallet_ledger l WHERE l.transaction_id=p_id
 AND l.ledger_entry_id IN(e.reserve_ledger_id,e.chips_ledger_id,e.reserve_refund_ledger_id)));
 IF NOT FOUND THEN RETURN NULL; END IF;
 WITH expected AS (
 SELECT * FROM (VALUES (p.reserve_ledger_id,'RESERVE_API_CREDIT',-p.reserve_debit_units,'reserve-debit','API_CHIPS_RESERVE_DEBIT',1),
 (p.chips_ledger_id,'AVAILABLE_CHIPS',p.chips_credit_units,'chips-credit','API_CHIPS_CHIPS_CREDIT',2),
 (p.reserve_refund_ledger_id,'RESERVE_API_CREDIT',p.reserve_debit_units,'reserve-refund','API_CHIPS_RESERVE_REFUND',3)) v(id,asset,delta,leg,entry,n)
 WHERE id IS NOT NULL
 ), actual AS (
 SELECT e.*,l.ledger_entry_id,l.newapi_user_id,l.asset_type,l.delta_units,l.balance_before_units,l.balance_after_units,
 l.biz_type,l.biz_id,l.entry_type,l.leg_no,l.transaction_id,t.status AS transaction_status,t.newapi_user_id AS transaction_user,
 t.biz_type AS transaction_biz_type,t.biz_id AS transaction_biz_id,t.operation_type,
 row_number() OVER(ORDER BY e.n) AS output_leg
 FROM expected e LEFT JOIN economy.wallet_ledger l ON l.ledger_entry_id=e.id
 LEFT JOIN economy.asset_transactions t ON t.transaction_id=l.transaction_id
 )
 SELECT coalesce(bool_or(ledger_entry_id IS NULL OR newapi_user_id<>p_user OR asset_type<>asset OR delta_units<>delta
 OR delta_units=0 OR balance_before_units<0 OR balance_after_units<0
 OR balance_after_units::numeric<>balance_before_units::numeric+delta_units::numeric
 OR biz_type<>'API_CHIPS_EXCHANGE_LEG' OR biz_id<>p.exchange_id::text||':'||leg OR entry_type<>entry OR leg_no<>1
 OR transaction_status<>'CONFIRMED' OR transaction_user<>p_user OR transaction_biz_type<>biz_type OR transaction_biz_id<>biz_id OR operation_type<>entry_type),false),
 coalesce(jsonb_agg(jsonb_build_object('ledger_id',ledger_entry_id,'leg_no',output_leg,'asset',asset_type,
 'delta_units',delta_units::text,'balance_before_units',balance_before_units::text,'balance_after_units',balance_after_units::text) ORDER BY n),'[]')
 INTO invalid,effects FROM actual;
 IF invalid THEN RAISE EXCEPTION 'HISTORY_SOURCE_UNAVAILABLE'; END IF;
 IF p.native_before IS NOT NULL THEN native_effects:=native_effects||jsonb_build_array(jsonb_build_object('delta_units',(-p.active_debit_units)::text,'before_units',p.native_before::text,'after_units',p.native_after::text)); END IF;
 IF p.refund_before IS NOT NULL THEN native_effects:=native_effects||jsonb_build_array(jsonb_build_object('delta_units',p.active_debit_units::text,'before_units',p.refund_before::text,'after_units',p.refund_after::text)); END IF;
 RETURN jsonb_strip_nulls(jsonb_build_object('id',p_id,'exchange_id',p.exchange_id,'kind','API_CHIPS_EXCHANGE','status',p.status,
 'created_at',p.created_at,'confirmed_at',p.confirmed_at,'reserve_debit_units',p.reserve_debit_units::text,
 'active_debit_units',p.active_debit_units::text,'chips_credit_units',p.chips_credit_units::text,
 'effects',effects,'native_effects',native_effects,'links','[]'::jsonb));
END $$;
REVOKE ALL ON FUNCTION economy.history_api_chips_exchange_read(bigint,uuid) FROM PUBLIC;
