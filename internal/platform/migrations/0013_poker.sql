-- Poker shares this platform database and its existing immutable wallet ledger.
-- Deployment owner applies this forward migration, never the Poker runtime.
CREATE SCHEMA poker;
CREATE DOMAIN poker.amount_units AS bigint CHECK(VALUE>=0 AND VALUE%500000=0);
CREATE TABLE poker.ruleset_versions (
 version text PRIMARY KEY, ante_posting_mode text NOT NULL CHECK(ante_posting_mode='NO_ANTE'),
 entry_mode text NOT NULL CHECK(entry_mode='WAIT_FOR_BB'), initial_button_version text NOT NULL,
 evaluator_version text NOT NULL, shortcut_version text NOT NULL, algorithm_version text NOT NULL,
 deal_version text NOT NULL, active boolean NOT NULL DEFAULT false
);
CREATE TABLE poker.blind_preset_versions (
 version text PRIMARY KEY, small_blind_units poker.amount_units NOT NULL CHECK(small_blind_units>0),
 big_blind_units poker.amount_units NOT NULL CHECK(big_blind_units=small_blind_units*2),
 minimum_buyin_bb integer NOT NULL CHECK(minimum_buyin_bb=40), maximum_buyin_bb integer NOT NULL CHECK(maximum_buyin_bb=100)
);
CREATE TABLE poker.tables (
 table_id uuid PRIMARY KEY, owner_newapi_user_id bigint NOT NULL REFERENCES identity.account_refs,
 name text NOT NULL CHECK(octet_length(name) BETWEEN 1 AND 128), max_seats integer NOT NULL CHECK(max_seats BETWEEN 2 AND 9),
 blind_preset_version text NOT NULL REFERENCES poker.blind_preset_versions,
 ruleset_version text NOT NULL REFERENCES poker.ruleset_versions,
 allow_spectators boolean NOT NULL DEFAULT true, chat_enabled boolean NOT NULL DEFAULT false,
 lifecycle_state text NOT NULL DEFAULT 'WAITING' CHECK(lifecycle_state IN('WAITING','IN_HAND','INTERMISSION','PAUSED','RECOVERING','CLOSING','CLOSED')),
 accepting_players boolean NOT NULL DEFAULT true, allow_new_hands boolean NOT NULL DEFAULT true,
 settings_locked_at timestamptz, table_version bigint NOT NULL DEFAULT 1 CHECK(table_version>0),
 runtime_epoch bigint NOT NULL DEFAULT 0 CHECK(runtime_epoch>=0), hand_no bigint NOT NULL DEFAULT 0 CHECK(hand_no>=0),
 button_seat integer, current_hand_id uuid, intermission_until timestamptz, empty_since timestamptz DEFAULT clock_timestamp(),
 spectator_count integer NOT NULL DEFAULT 0 CHECK(spectator_count>=0), created_at timestamptz NOT NULL DEFAULT clock_timestamp(), closed_at timestamptz
);
CREATE UNIQUE INDEX poker_one_owned_open_table ON poker.tables(owner_newapi_user_id) WHERE lifecycle_state<>'CLOSED';
CREATE TABLE poker.seats (
 table_id uuid NOT NULL REFERENCES poker.tables, seat_no integer NOT NULL CHECK(seat_no BETWEEN 1 AND 9),
 session_id uuid, state text NOT NULL DEFAULT 'WAITING_ENTRY' CHECK(state IN('WAITING_ENTRY','ACTIVE','WAITING_BIG_BLIND','SIT_OUT','LEAVE_AFTER_HAND','REBUY_WINDOW','LEFT')),
 connected boolean NOT NULL DEFAULT true, disconnected_since timestamptz, sit_out_since timestamptz,
 timeout_count integer NOT NULL DEFAULT 0 CHECK(timeout_count>=0), timeout_sit_out boolean NOT NULL DEFAULT false,
 sit_out_next_hand boolean NOT NULL DEFAULT false, leave_after_hand boolean NOT NULL DEFAULT false,
 rebuy_deadline_at timestamptz, next_client_seed_contribution text NOT NULL DEFAULT '', contribution_version bigint NOT NULL DEFAULT 1 CHECK(contribution_version>0),
 PRIMARY KEY(table_id,seat_no)
);
CREATE TABLE poker.sessions (
 session_id uuid PRIMARY KEY, newapi_user_id bigint NOT NULL REFERENCES identity.account_refs,
 table_id uuid NOT NULL, seat_no integer NOT NULL, display_name_snapshot text NOT NULL,
 state text NOT NULL DEFAULT 'ACTIVE' CHECK(state IN('ACTIVE','SETTLED','NEEDS_REVIEW')),
 initial_buyin_units poker.amount_units NOT NULL, total_topup_units poker.amount_units NOT NULL DEFAULT 0,
 current_stack_units poker.amount_units NOT NULL, final_cashout_units poker.amount_units,
 realized_pl_units bigint CHECK(realized_pl_units%500000=0), control_epoch bigint NOT NULL DEFAULT 1 CHECK(control_epoch>0),
 started_at timestamptz NOT NULL DEFAULT clock_timestamp(), ended_at timestamptz, end_reason text,
 FOREIGN KEY(table_id,seat_no) REFERENCES poker.seats
);
ALTER TABLE poker.seats ADD FOREIGN KEY(session_id) REFERENCES poker.sessions;
CREATE UNIQUE INDEX poker_one_active_session_per_user ON poker.sessions(newapi_user_id) WHERE state<>'SETTLED';
CREATE UNIQUE INDEX poker_one_active_session_per_seat ON poker.sessions(table_id,seat_no) WHERE state<>'SETTLED';
CREATE TABLE poker.seat_reservations (
 reservation_id uuid PRIMARY KEY, table_id uuid NOT NULL, seat_no integer NOT NULL,
 newapi_user_id bigint NOT NULL REFERENCES identity.account_refs, lease_token text NOT NULL,
 state text NOT NULL CHECK(state IN('LEASE_ACTIVE','CONSUMED','EXPIRED','FAILED_NO_EFFECT')),
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(table_id,seat_no) REFERENCES poker.seats
);
CREATE UNIQUE INDEX poker_one_active_reservation ON poker.seat_reservations(table_id,seat_no) WHERE state='LEASE_ACTIVE';
CREATE TABLE poker.funding_operations (
 funding_operation_id uuid PRIMARY KEY, table_id uuid NOT NULL, seat_no integer NOT NULL,
 session_id uuid NOT NULL, newapi_user_id bigint NOT NULL REFERENCES identity.account_refs,
 kind text NOT NULL CHECK(kind IN('BUY_IN','TOP_UP','REBUY','CASH_OUT')),
 amount_units poker.amount_units NOT NULL, reservation_id uuid REFERENCES poker.seat_reservations,
 request_hash bytea NOT NULL CHECK(octet_length(request_hash)=32),
 planned_transaction_id uuid NOT NULL UNIQUE, planned_ledger_id uuid NOT NULL UNIQUE,
 state text NOT NULL DEFAULT 'PENDING' CHECK(state IN('PENDING','CONFIRMED','FAILED_NO_EFFECT')),
 confirmed_transaction_id uuid REFERENCES economy.asset_transactions, confirmed_ledger_id uuid REFERENCES economy.wallet_ledger,
 receipt jsonb, failure_code text, created_at timestamptz NOT NULL DEFAULT clock_timestamp(), confirmed_at timestamptz,
 CHECK((state='CONFIRMED')=(confirmed_transaction_id IS NOT NULL)),
 FOREIGN KEY(table_id,seat_no) REFERENCES poker.seats
);
CREATE UNIQUE INDEX poker_one_buyin_per_session ON poker.funding_operations(session_id) WHERE kind='BUY_IN' AND state<>'FAILED_NO_EFFECT';
CREATE UNIQUE INDEX poker_one_cashout_per_session ON poker.funding_operations(session_id) WHERE kind='CASH_OUT' AND state<>'FAILED_NO_EFFECT';
CREATE TABLE poker.hands (
 hand_id uuid PRIMARY KEY, table_id uuid NOT NULL REFERENCES poker.tables, hand_no bigint NOT NULL,
 state text NOT NULL CHECK(state IN('COMMITTED','PREFLOP','FLOP','TURN','RIVER','SETTLED')),
 button_seat integer NOT NULL CHECK(button_seat BETWEEN 1 AND 9), runtime_epoch bigint NOT NULL,
 setup_cipher bytea NOT NULL, snapshot_cipher bytea, snapshot_hash bytea,
 hand_version bigint NOT NULL DEFAULT 0, event_sequence bigint NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), settled_at timestamptz,
 UNIQUE(table_id,hand_no)
);
ALTER TABLE poker.tables ADD FOREIGN KEY(current_hand_id) REFERENCES poker.hands;
CREATE UNIQUE INDEX poker_one_unsettled_hand ON poker.hands(table_id) WHERE state<>'SETTLED';
CREATE TABLE poker.hand_fairness (
 hand_id uuid PRIMARY KEY REFERENCES poker.hands,
 server_seed_hash bytea NOT NULL CHECK(octet_length(server_seed_hash)=32),
 encrypted_server_seed bytea NOT NULL, server_seed_key_version text NOT NULL,
 effective_client_seed_hash bytea NOT NULL CHECK(octet_length(effective_client_seed_hash)=32),
 algorithm_version text NOT NULL,deck_version text NOT NULL,deal_sequence_version text NOT NULL,
 deck_hash bytea NOT NULL CHECK(octet_length(deck_hash)=32),next_deck_index integer NOT NULL DEFAULT 0 CHECK(next_deck_index BETWEEN 0 AND 52),
 full_fairness_reveal_at timestamptz
);
CREATE TABLE poker.hand_participants (
 hand_id uuid NOT NULL REFERENCES poker.hands, seat_no integer NOT NULL, session_id uuid NOT NULL REFERENCES poker.sessions,
 newapi_user_id bigint NOT NULL REFERENCES identity.account_refs,
 hand_start_stack_units poker.amount_units NOT NULL, street_committed_units poker.amount_units NOT NULL DEFAULT 0,
 total_committed_units poker.amount_units NOT NULL DEFAULT 0, folded boolean NOT NULL DEFAULT false,
 contribution_version bigint NOT NULL, contribution_cipher bytea NOT NULL,
 PRIMARY KEY(hand_id,seat_no), UNIQUE(hand_id,session_id)
);
CREATE TABLE poker.actions (
 hand_id uuid NOT NULL REFERENCES poker.hands, event_sequence bigint NOT NULL CHECK(event_sequence>0),
 hand_version bigint NOT NULL, event_type text NOT NULL, actor_seat integer,
 applied_delta_units poker.amount_units NOT NULL DEFAULT 0, to_units poker.amount_units NOT NULL DEFAULT 0,
 payload_cipher bytea NOT NULL, created_at timestamptz NOT NULL, PRIMARY KEY(hand_id,event_sequence)
);
CREATE TABLE poker.dealt_cards (
 hand_id uuid NOT NULL REFERENCES poker.hands, deck_index integer NOT NULL CHECK(deck_index BETWEEN 0 AND 51),
 recipient_seat integer, visibility text NOT NULL CHECK(visibility IN('PRIVATE','PUBLIC')),
 card_cipher bytea NOT NULL, PRIMARY KEY(hand_id,deck_index)
);
CREATE TABLE poker.pots (
 hand_id uuid NOT NULL REFERENCES poker.hands, pot_index integer NOT NULL, amount_units poker.amount_units NOT NULL,
 contribution_floor poker.amount_units NOT NULL, contribution_ceiling poker.amount_units NOT NULL,
 PRIMARY KEY(hand_id,pot_index)
);
CREATE TABLE poker.pot_eligible_players (
 hand_id uuid NOT NULL,pot_index integer NOT NULL,seat_no integer NOT NULL,
 PRIMARY KEY(hand_id,pot_index,seat_no),FOREIGN KEY(hand_id,pot_index) REFERENCES poker.pots
);
CREATE TABLE poker.pot_awards (
 hand_id uuid NOT NULL,pot_index integer NOT NULL,seat_no integer NOT NULL,
 base_share_units poker.amount_units NOT NULL,odd_chip_units poker.amount_units NOT NULL CHECK(odd_chip_units IN(0,500000)),
 award_units poker.amount_units NOT NULL CHECK(award_units=base_share_units+odd_chip_units),
 PRIMARY KEY(hand_id,pot_index,seat_no),FOREIGN KEY(hand_id,pot_index,seat_no) REFERENCES poker.pot_eligible_players
);
CREATE TABLE poker.settlements (
 hand_id uuid PRIMARY KEY REFERENCES poker.hands,biz_id text NOT NULL UNIQUE,
 total_commitment_units poker.amount_units NOT NULL,total_award_units poker.amount_units NOT NULL,uncalled_return_units poker.amount_units NOT NULL,
 settled_at timestamptz NOT NULL DEFAULT clock_timestamp(),CHECK(total_commitment_units=total_award_units+uncalled_return_units)
);
CREATE TABLE poker.recovery_state (
 table_id uuid PRIMARY KEY REFERENCES poker.tables,hand_id uuid REFERENCES poker.hands,runtime_epoch bigint NOT NULL,
 state text NOT NULL CHECK(state IN('GRACE','RESUMED')),started_at timestamptz NOT NULL,grace_until timestamptz NOT NULL,resumed_at timestamptz
);
CREATE TABLE poker.request_receipts (
 newapi_user_id bigint NOT NULL REFERENCES identity.account_refs,scope text NOT NULL,key_hash bytea NOT NULL CHECK(octet_length(key_hash)=32),
 request_hash bytea NOT NULL CHECK(octet_length(request_hash)=32),table_id uuid NOT NULL REFERENCES poker.tables,
 response jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(newapi_user_id,scope,key_hash)
);
CREATE INDEX poker_sessions_table ON poker.sessions(table_id,state);
CREATE INDEX poker_funding_pending ON poker.funding_operations(table_id,created_at) WHERE state='PENDING';
CREATE TRIGGER poker_actions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON poker.actions FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TRIGGER poker_deals_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON poker.dealt_cards FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TRIGGER poker_receipts_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON poker.request_receipts FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TRIGGER poker_settlement_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON poker.settlements FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();

-- Private implementation shared only by the three constant-kind wrappers.
-- Every lookup is schema-qualified; caller-supplied SQL/object/asset names never enter.
CREATE FUNCTION economy._poker_funding_apply(p_table uuid,p_epoch bigint,p_operation uuid,p_gateway text)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE t poker.tables%ROWTYPE; seat poker.seats%ROWTYPE; sess poker.sessions%ROWTYPE;
 op poker.funding_operations%ROWTYPE; reservation poker.seat_reservations%ROWTYPE;
 wallet economy.wallet_balances%ROWTYPE; bb bigint; amount bigint; delta bigint; balance_after bigint;
 business text; result jsonb; profile_name text; at_time timestamptz:=clock_timestamp();
BEGIN
 SELECT * INTO STRICT t FROM poker.tables WHERE table_id=p_table FOR UPDATE;
 IF t.runtime_epoch<>p_epoch THEN RAISE EXCEPTION 'STALE_RUNTIME_EPOCH' USING ERRCODE='40001'; END IF;
 SELECT * INTO STRICT op FROM poker.funding_operations WHERE funding_operation_id=p_operation AND table_id=p_table;
 IF (p_gateway='BUY_IN' AND op.kind<>'BUY_IN') OR (p_gateway='TOP_UP' AND op.kind NOT IN('TOP_UP','REBUY')) OR (p_gateway='CASH_OUT' AND op.kind<>'CASH_OUT') THEN RAISE EXCEPTION 'FUNDING_KIND_MISMATCH'; END IF;
 SELECT * INTO STRICT seat FROM poker.seats WHERE table_id=p_table AND seat_no=op.seat_no FOR UPDATE;
 IF op.kind<>'BUY_IN' THEN SELECT * INTO STRICT sess FROM poker.sessions WHERE session_id=op.session_id AND table_id=p_table AND seat_no=op.seat_no AND newapi_user_id=op.newapi_user_id FOR UPDATE; END IF;
 SELECT * INTO STRICT op FROM poker.funding_operations WHERE funding_operation_id=p_operation FOR UPDATE;
 IF op.state='CONFIRMED' THEN RETURN op.receipt; END IF;
 IF op.state<>'PENDING' THEN RAISE EXCEPTION 'FUNDING_NOT_PENDING'; END IF;
 SELECT big_blind_units INTO STRICT bb FROM poker.blind_preset_versions WHERE version=t.blind_preset_version;
 amount:=op.amount_units;
 IF op.kind='BUY_IN' THEN
   IF NOT t.accepting_players OR t.lifecycle_state IN('CLOSING','CLOSED') OR seat.session_id IS NOT NULL OR amount<40*bb OR amount>100*bb THEN RAISE EXCEPTION 'INVALID_BUY_IN'; END IF;
   SELECT * INTO STRICT reservation FROM poker.seat_reservations WHERE reservation_id=op.reservation_id AND table_id=p_table AND seat_no=op.seat_no AND newapi_user_id=op.newapi_user_id FOR UPDATE;
   IF reservation.state<>'LEASE_ACTIVE' OR reservation.expires_at<=at_time THEN RAISE EXCEPTION 'RESERVATION_EXPIRED'; END IF;
   SELECT display_name INTO STRICT profile_name FROM identity.master_profiles WHERE newapi_user_id=op.newapi_user_id AND profile_version>0;
   business:='poker_buyin:'||op.session_id::text;delta:=-amount;
 ELSE
   IF sess.state<>'ACTIVE' OR seat.session_id IS DISTINCT FROM op.session_id THEN RAISE EXCEPTION 'SESSION_NOT_ACTIVE'; END IF;
   IF EXISTS(SELECT 1 FROM poker.hand_participants p JOIN poker.hands h USING(hand_id) WHERE p.session_id=op.session_id AND h.state<>'SETTLED') THEN RAISE EXCEPTION 'HAND_UNSETTLED'; END IF;
   IF op.kind='CASH_OUT' THEN amount:=sess.current_stack_units;delta:=amount;business:='poker_cashout:'||op.session_id::text;
   ELSE
     IF t.lifecycle_state IN('CLOSING','CLOSED') OR amount<=0 OR sess.current_stack_units::numeric+amount::numeric>100*bb THEN RAISE EXCEPTION 'INVALID_TOP_UP'; END IF;
     IF op.kind='REBUY' AND (seat.state<>'REBUY_WINDOW' OR seat.rebuy_deadline_at<=at_time OR amount<40*bb) THEN RAISE EXCEPTION 'INVALID_REBUY'; END IF;
     delta:=-amount;business:='poker_'||CASE WHEN op.kind='REBUY' THEN 'rebuy:' ELSE 'topup:' END||op.funding_operation_id::text;
   END IF;
 END IF;
 -- Table -> Seat -> Session -> Wallet is the only funding lock order.
 SELECT * INTO STRICT wallet FROM economy.wallet_balances WHERE newapi_user_id=op.newapi_user_id AND asset_type='AVAILABLE_CHIPS' FOR UPDATE;
 -- Wallet acquisition can wait. All validity time below is fresh after that wait.
 at_time:=clock_timestamp();
 IF op.kind='BUY_IN' AND reservation.expires_at<=at_time THEN RAISE EXCEPTION 'RESERVATION_EXPIRED'; END IF;
 IF op.kind='REBUY' AND seat.rebuy_deadline_at<=at_time THEN RAISE EXCEPTION 'INVALID_REBUY'; END IF;
 IF wallet.balance_units::numeric+delta::numeric<0 THEN RAISE EXCEPTION 'INSUFFICIENT_CHIPS' USING ERRCODE='P0001'; END IF;
 balance_after:=wallet.balance_units+delta;
 INSERT INTO economy.asset_transactions(transaction_id,biz_type,biz_id,newapi_user_id,operation_type,status,request_hash)
 VALUES(op.planned_transaction_id,'POKER_FUNDING',business,op.newapi_user_id,'POKER_'||op.kind,'CONFIRMED',op.request_hash);
 IF delta<>0 THEN
   UPDATE economy.wallet_balances SET balance_units=balance_after,ledger_seq=ledger_seq+1,version=version+1,updated_at=at_time WHERE newapi_user_id=op.newapi_user_id AND asset_type='AVAILABLE_CHIPS';
   INSERT INTO economy.wallet_ledger(ledger_entry_id,transaction_id,leg_no,newapi_user_id,asset_type,ledger_seq,wallet_version,entry_type,biz_type,biz_id,delta_units,balance_before_units,balance_after_units,metadata)
   VALUES(op.planned_ledger_id,op.planned_transaction_id,1,op.newapi_user_id,'AVAILABLE_CHIPS',wallet.ledger_seq+1,wallet.version+1,'POKER_'||op.kind,'POKER_FUNDING',business,delta,wallet.balance_units,balance_after,jsonb_build_object('table_id',p_table,'session_id',op.session_id));
 END IF;
 IF op.kind='BUY_IN' THEN
   INSERT INTO poker.sessions(session_id,newapi_user_id,table_id,seat_no,display_name_snapshot,initial_buyin_units,current_stack_units) VALUES(op.session_id,op.newapi_user_id,p_table,op.seat_no,profile_name,amount,amount);
   UPDATE poker.seats SET session_id=op.session_id,state=CASE WHEN t.hand_no=0 THEN 'ACTIVE' ELSE 'WAITING_BIG_BLIND' END,connected=true,disconnected_since=NULL,sit_out_since=NULL,timeout_count=0,timeout_sit_out=false,sit_out_next_hand=false,leave_after_hand=false,rebuy_deadline_at=NULL WHERE table_id=p_table AND seat_no=op.seat_no;
   UPDATE poker.seat_reservations SET state='CONSUMED' WHERE reservation_id=op.reservation_id;
   UPDATE poker.tables SET settings_locked_at=coalesce(settings_locked_at,at_time),empty_since=NULL WHERE table_id=p_table;
 ELSIF op.kind='CASH_OUT' THEN
   UPDATE poker.sessions SET current_stack_units=0,final_cashout_units=amount,realized_pl_units=amount-initial_buyin_units-total_topup_units,state='SETTLED',ended_at=at_time,end_reason=coalesce(end_reason,'SAFE_LEAVE') WHERE session_id=op.session_id;
   UPDATE poker.seats SET session_id=NULL,state='LEFT',leave_after_hand=false,sit_out_next_hand=false,rebuy_deadline_at=NULL WHERE table_id=p_table AND seat_no=op.seat_no;
 ELSE
   UPDATE poker.sessions SET current_stack_units=current_stack_units+amount,total_topup_units=total_topup_units+amount WHERE session_id=op.session_id;
   IF op.kind='REBUY' THEN UPDATE poker.seats SET state='WAITING_BIG_BLIND',rebuy_deadline_at=NULL WHERE table_id=p_table AND seat_no=op.seat_no; END IF;
 END IF;
 result:=jsonb_build_object('table_id',p_table,'session_id',op.session_id,'funding_operation_id',op.funding_operation_id,'status','CONFIRMED','amount_units',amount::text,'transaction_id',op.planned_transaction_id,'biz_id',business);
 UPDATE poker.funding_operations SET amount_units=amount,state='CONFIRMED',confirmed_transaction_id=op.planned_transaction_id,confirmed_ledger_id=CASE WHEN delta=0 THEN NULL ELSE op.planned_ledger_id END,receipt=result,confirmed_at=at_time WHERE funding_operation_id=p_operation;
 RETURN result;
END $$;
CREATE FUNCTION economy.poker_buy_in_apply(uuid,bigint,uuid) RETURNS jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog AS $$ SELECT economy._poker_funding_apply($1,$2,$3,'BUY_IN') $$;
CREATE FUNCTION economy.poker_top_up_apply(uuid,bigint,uuid) RETURNS jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog AS $$ SELECT economy._poker_funding_apply($1,$2,$3,'TOP_UP') $$;
CREATE FUNCTION economy.poker_cash_out_apply(uuid,bigint,uuid) RETURNS jsonb LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog AS $$ SELECT economy._poker_funding_apply($1,$2,$3,'CASH_OUT') $$;
REVOKE ALL ON FUNCTION economy._poker_funding_apply(uuid,bigint,uuid,text),economy.poker_buy_in_apply(uuid,bigint,uuid),economy.poker_top_up_apply(uuid,bigint,uuid),economy.poker_cash_out_apply(uuid,bigint,uuid) FROM PUBLIC;
REVOKE ALL ON SCHEMA poker FROM PUBLIC;
REVOKE ALL ON ALL TABLES IN SCHEMA poker FROM PUBLIC;
