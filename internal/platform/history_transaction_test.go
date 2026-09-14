package platform_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/historyaccess"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/testutil/historyfixture"
	"github.com/jackc/pgx/v5"
)

func TestHistoryDetailAccessFresh(t *testing.T) {
	ctx := context.Background()
	if viewer, subject, err := historyaccess.Own(101).Check(ctx); err != nil || viewer != 101 || subject != 101 {
		t.Fatal("authenticated own access rejected")
	}
	granted := true
	scope := historyaccess.Records(102, 101, func(_ context.Context, viewer, subject int64) error {
		if granted && viewer == 102 && subject == 101 {
			return nil
		}
		return errors.New("revoked")
	})
	if _, subject, err := scope.Check(ctx); err != nil || subject != 101 {
		t.Fatal("verified scope rejected")
	}
	granted = false
	if _, _, err := scope.Check(ctx); !errors.Is(err, historyaccess.ErrNotFound) {
		t.Fatal("revoked scope retained")
	}
	for _, a := range []historyaccess.Access{{}, historyaccess.Own(0), historyaccess.Records(102, 101, nil)} {
		if _, _, err := a.Check(ctx); err == nil {
			t.Fatal("unverified access accepted")
		}
	}
	var injected historyaccess.Access
	if err := json.Unmarshal([]byte(`{"viewer":101,"subject":101,"authorized":true}`), &injected); err != nil {
		t.Fatal(err)
	}
	if _, _, err := injected.Check(ctx); err == nil {
		t.Fatal("JSON became authorization")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := historyaccess.Own(101).Check(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled request accepted")
	}
}

func TestHistoryDetailTransactionPG(t *testing.T) {
	f := historyfixture.Open(t)
	ctx := context.Background()
	apply := func(kind string, amount int64) platform.LedgerEntry {
		e, err := f.Owner.Apply(ctx, platform.Mutation{UserID: 101, Asset: platform.AvailableChips, DeltaUnits: amount, BizType: kind, BizID: kind, EntryType: kind, IdempotencyKey: "history-test-" + kind})
		historyfixture.Check(t, err)
		return e
	}
	large := apply("HISTORY_TEST_GRANT", 9007199255000000)
	exchange, err := f.Owner.Exchange(ctx, 101, "00000000-0000-4000-8000-000000000020", platform.AvailableChips, 1000000)
	historyfixture.Check(t, err)
	f.SQL(t, `INSERT INTO poker.tables(table_id,owner_newapi_user_id,name,max_seats,blind_preset_version,ruleset_version) VALUES('00000000-0000-4000-8000-000000000001',101,'Closed source',2,'5-10','poker-cash-v1-20260906');
INSERT INTO poker.seats(table_id,seat_no) VALUES('00000000-0000-4000-8000-000000000001',1);
INSERT INTO poker.sessions(session_id,newapi_user_id,table_id,seat_no,display_name_snapshot,initial_buyin_units,current_stack_units) VALUES('00000000-0000-4000-8000-000000000002',101,'00000000-0000-4000-8000-000000000001',1,'Player A',200000000,0);
UPDATE poker.seats SET session_id='00000000-0000-4000-8000-000000000002' WHERE table_id='00000000-0000-4000-8000-000000000001';
INSERT INTO poker.funding_operations(funding_operation_id,table_id,seat_no,session_id,newapi_user_id,kind,amount_units,request_hash,planned_transaction_id,planned_ledger_id) VALUES('00000000-0000-4000-8000-000000000003','00000000-0000-4000-8000-000000000001',1,'00000000-0000-4000-8000-000000000002',101,'CASH_OUT',0,decode(repeat('00',32),'hex'),'00000000-0000-4000-8000-000000000004','00000000-0000-4000-8000-000000000005');
SELECT economy.poker_cash_out_apply('00000000-0000-4000-8000-000000000001',0,'00000000-0000-4000-8000-000000000003');
INSERT INTO poker.sessions(session_id,newapi_user_id,table_id,seat_no,display_name_snapshot,initial_buyin_units,current_stack_units) VALUES('00000000-0000-4000-8000-000000000012',101,'00000000-0000-4000-8000-000000000001',1,'Player A',200000000,5000000);
UPDATE poker.seats SET session_id='00000000-0000-4000-8000-000000000012',state='ACTIVE' WHERE table_id='00000000-0000-4000-8000-000000000001';
INSERT INTO poker.funding_operations(funding_operation_id,table_id,seat_no,session_id,newapi_user_id,kind,amount_units,request_hash,planned_transaction_id,planned_ledger_id) VALUES('00000000-0000-4000-8000-000000000013','00000000-0000-4000-8000-000000000001',1,'00000000-0000-4000-8000-000000000012',101,'CASH_OUT',5000000,decode(repeat('00',32),'hex'),'00000000-0000-4000-8000-000000000014','00000000-0000-4000-8000-000000000015');
SELECT economy.poker_cash_out_apply('00000000-0000-4000-8000-000000000001',0,'00000000-0000-4000-8000-000000000013');
INSERT INTO economy.asset_transactions(transaction_id,biz_type,biz_id,newapi_user_id,operation_type,status,request_hash) VALUES('00000000-0000-4000-8000-000000000006','GAME_SETTLEMENT','missing',101,'GAME_PAYOUT','CONFIRMED',decode(repeat('00',32),'hex'))`)
	for _, tc := range []struct {
		id    string
		legs  int
		delta string
	}{
		{large.TransactionID, 1, "9007199255000000"}, {exchange.ID, 2, "-1000000"}, {"00000000-0000-4000-8000-000000000004", 0, ""},
		{"00000000-0000-4000-8000-000000000014", 1, "5000000"},
	} {
		v, e := f.Runtime.HistoryTransaction(ctx, historyaccess.Own(101), tc.id)
		if e != nil || v.ID != tc.id || len(v.Effects) != tc.legs {
			t.Fatal("owned transaction or zero-effect receipt lost", e)
		}
		if tc.legs > 0 && v.Effects[0].DeltaUnits != tc.delta {
			t.Fatal("signed exact integer changed")
		}
		if tc.legs == 2 && (v.Effects[1].DeltaUnits != "1000000" || v.Effects[0].Asset == v.Effects[1].Asset) {
			t.Fatal("exchange legs collapsed")
		}
		if tc.legs == 0 && (len(v.Links) != 1 || v.Links[0].SourceID != "00000000-0000-4000-8000-000000000002") {
			t.Fatal("zero cash-out backlink missing")
		}
		if _, e = f.Runtime.HistoryTransaction(ctx, historyaccess.Own(102), tc.id); !errors.Is(e, historyaccess.ErrNotFound) {
			t.Fatal("foreign transaction disclosed")
		}
	}
	// Owner-only fault injection and the capability read share a rolled-back transaction.
	for _, tc := range []struct{ name, id, sql string }{
		{"exchange_amount", exchange.ID, `UPDATE economy.wallet_ledger SET delta_units=delta_units+1,balance_after_units=balance_after_units+1 WHERE transaction_id=$1 AND leg_no=2`},
		{"exchange_sign", exchange.ID, `UPDATE economy.wallet_ledger SET delta_units=-delta_units,balance_after_units=balance_before_units-delta_units WHERE transaction_id=$1 AND leg_no=1`},
		{"leg_sequence", large.TransactionID, `UPDATE economy.wallet_ledger SET leg_no=2 WHERE transaction_id=$1`},
		{"confirmed_ledger", "00000000-0000-4000-8000-000000000014", `UPDATE poker.funding_operations SET confirmed_ledger_id='` + large.ID + `' WHERE confirmed_transaction_id=$1`},
		{"duplicate_source", "00000000-0000-4000-8000-000000000014", `DROP INDEX poker.poker_one_cashout_per_session; INSERT INTO poker.funding_operations(funding_operation_id,table_id,seat_no,session_id,newapi_user_id,kind,amount_units,request_hash,planned_transaction_id,planned_ledger_id,state,confirmed_transaction_id,confirmed_ledger_id) SELECT '00000000-0000-4000-8000-000000000023',table_id,seat_no,session_id,newapi_user_id,kind,amount_units,request_hash,'00000000-0000-4000-8000-000000000024','00000000-0000-4000-8000-000000000025',state,confirmed_transaction_id,confirmed_ledger_id FROM poker.funding_operations WHERE confirmed_transaction_id=$1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx, e := f.Pool.Begin(ctx)
			historyfixture.Check(t, e)
			defer tx.Rollback(ctx)
			_, e = tx.Exec(ctx, `ALTER TABLE economy.wallet_ledger DISABLE TRIGGER wallet_ledger_immutable; `+tc.sql, pgx.QueryExecModeSimpleProtocol, tc.id)
			historyfixture.Check(t, e)
			if _, e = platform.HistoryTransactionInTx(ctx, tx, 101, tc.id); !errors.Is(e, historyaccess.ErrUnavailable) {
				t.Fatal("corrupt formal money accepted", e)
			}
			historyfixture.Check(t, tx.Rollback(ctx))
		})
	}
	broken := apply("LOCAL_EXCHANGE", 10)
	scoped := historyaccess.Records(102, 101, func(context.Context, int64, int64) error { return nil })
	if _, err = f.Runtime.HistoryTransaction(ctx, scoped, large.TransactionID); err != nil {
		t.Fatal("verified server scope not applied", err)
	}
	if _, err = f.Runtime.HistoryTransaction(ctx, historyaccess.Records(102, 101, nil), large.TransactionID); !errors.Is(err, historyaccess.ErrNotFound) {
		t.Fatal("missing verifier did not deny foreign read")
	}
	for _, id := range []string{broken.TransactionID, "00000000-0000-4000-8000-000000000006"} {
		if _, err = f.Runtime.HistoryTransaction(ctx, historyaccess.Own(101), id); !errors.Is(err, historyaccess.ErrUnavailable) {
			t.Fatal("incomplete source accepted", err)
		}
	}
	for id, want := range map[string]error{"bad": historyaccess.ErrInvalid, "00000000-0000-4000-8000-000000000099": historyaccess.ErrNotFound} {
		if _, err = f.Runtime.HistoryTransaction(ctx, historyaccess.Own(101), id); !errors.Is(err, want) {
			t.Fatal("missing/invalid transaction classification")
		}
	}
	var safe bool
	historyfixture.Check(t, f.Pool.QueryRow(ctx, `SELECT NOT has_function_privilege($1,'economy.history_transaction_read(bigint,uuid)','EXECUTE') AND NOT has_function_privilege($2,'economy.history_transaction_read(bigint,uuid)','EXECUTE') AND NOT has_function_privilege($3,'economy.history_transaction_read(bigint,uuid)','EXECUTE') AND NOT has_any_column_privilege($4,'games.history_display_snapshots','SELECT')`, f.PokerRole, f.ReaderRole, f.WorkerRole, f.PlatformRole).Scan(&safe))
	if !safe {
		t.Fatal("cross-domain/history role capability leaked")
	}
	for _, acl := range []string{
		"SELECT ON games.history_index|" + f.PlatformRole,
		"EXECUTE ON FUNCTION economy.history_transaction_read(bigint,uuid)|" + f.PokerRole,
		"EXECUTE ON FUNCTION economy.history_transaction_read(bigint,uuid)|" + f.ReaderRole,
		"EXECUTE ON FUNCTION economy.history_transaction_read(bigint,uuid)|" + f.WorkerRole,
		"EXECUTE ON FUNCTION games.history_record_snapshot(bigint,text,uuid)|" + f.ReaderRole,
		"EXECUTE ON FUNCTION games.history_record_snapshot(bigint,text,uuid)|" + f.WorkerRole,
	} {
		parts := strings.Split(acl, "|")
		f.SQL(t, "GRANT "+parts[0]+" TO "+parts[1])
		f.Grant(t, "deploy/sql/runtime-grants-0023-history-details.psql", "platform", false)
		f.SQL(t, "REVOKE "+parts[0]+" FROM "+parts[1])
		f.Grant(t, "deploy/sql/runtime-grants-0023-history-details.psql", "platform", true)
	}
	lock, err := f.Pool.Begin(ctx)
	historyfixture.Check(t, err)
	defer lock.Rollback(ctx)
	_, err = lock.Exec(ctx, "LOCK TABLE economy.asset_transactions IN ACCESS EXCLUSIVE MODE")
	historyfixture.Check(t, err)
	start := time.Now()
	_, err = f.Runtime.HistoryTransaction(ctx, historyaccess.Own(101), large.TransactionID)
	if !errors.Is(err, historyaccess.ErrUnavailable) || time.Since(start) > 4*time.Second {
		t.Fatal("history SQL timeout not bounded or safely classified", err)
	}
	historyfixture.Check(t, lock.Rollback(ctx))
	t.Log("TRANSACTIONS: exact large integers, signed two legs, genuine zero cashout, owner, missing facts, narrow grants")
}
