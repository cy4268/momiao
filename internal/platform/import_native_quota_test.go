package platform

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Uses the existing dev snapshot; every wallet/account/ledger write is rolled back.
func TestNativeQuotaMerge(t *testing.T) {
	sourcePath, targetPath := os.Getenv("MOMIAO_IMPORT_NATIVE_DSN_FILE"), os.Getenv("MOMIAO_MIGRATION_DSN_FILE")
	if sourcePath == "" || targetPath == "" {
		t.Skip("requires existing isolated dev native/platform databases")
	}
	sourceDSN, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal("read source DSN")
	}
	targetDSN, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal("read target DSN")
	}
	ctx := t.Context()
	source, err := pgx.Connect(ctx, strings.TrimSpace(string(sourceDSN)))
	if err != nil {
		t.Fatal("open source database")
	}
	defer source.Close(context.Background())
	s, err := Open(ctx, strings.TrimSpace(string(targetDSN)))
	if err != nil {
		t.Fatal("open target database")
	}
	defer s.Close()
	from, err := source.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer from.Rollback(context.Background())
	to, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer to.Rollback(context.Background())
	var users, units, zero int64
	if err = from.QueryRow(ctx, `SELECT count(*) FILTER(WHERE quota>0),coalesce(sum(quota) FILTER(WHERE quota>0),0),count(*) FILTER(WHERE coalesce(quota,0)=0) FROM public.users`).Scan(&users, &units, &zero); err != nil || users < 2 {
		t.Fatal("existing native quotas", err)
	}
	var before, priorUnits int64
	if err = to.QueryRow(ctx, `SELECT (SELECT sum(balance_units) FROM economy.wallet_balances WHERE asset_type='RESERVE_API_CREDIT'),coalesce(sum(delta_units),0) FROM economy.wallet_ledger WHERE biz_type='NATIVE_QUOTA_MERGE'`).Scan(&before, &priorUnits); err != nil {
		t.Fatal(err)
	}
	// Two real accounts: one may not yet have local wallets (the imported root).
	rows, err := from.Query(ctx, `SELECT id,quota FROM public.users WHERE quota>0 ORDER BY id LIMIT 2`)
	if err != nil {
		t.Fatal(err)
	}
	samples, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) ([3]int64, error) {
		var sample [3]int64
		err := row.Scan(&sample[0], &sample[1])
		return sample, err
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := range samples {
		if err = to.QueryRow(ctx, `SELECT coalesce((SELECT balance_units FROM economy.wallet_balances WHERE newapi_user_id=$1 AND asset_type='RESERVE_API_CREDIT'),0)-coalesce((SELECT sum(delta_units) FROM economy.wallet_ledger WHERE biz_type='NATIVE_QUOTA_MERGE' AND newapi_user_id=$1),0)`, samples[i][0]).Scan(&samples[i][2]); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		got, err := mergeNativeQuota(ctx, from, to)
		if err != nil || got.Users != users || got.QuotaUnits != units || got.ZeroQuotaUsers != zero {
			t.Fatal("native quota merge", got, err)
		}
		var balance, merged, ledgerUnits int64
		if err = to.QueryRow(ctx, `SELECT (SELECT sum(balance_units) FROM economy.wallet_balances WHERE asset_type='RESERVE_API_CREDIT'),count(*),sum(delta_units) FROM economy.wallet_ledger WHERE biz_type='NATIVE_QUOTA_MERGE'`).Scan(&balance, &merged, &ledgerUnits); err != nil || balance != before+units-priorUnits || merged != users || ledgerUnits != units {
			t.Fatal("merge/replay totals", balance, merged, ledgerUnits, err)
		}
	}
	for _, sample := range samples {
		var beforeUnits, afterUnits, wallet int64
		if err = to.QueryRow(ctx, `SELECT l.balance_before_units,l.balance_after_units,w.balance_units FROM economy.wallet_ledger l JOIN economy.wallet_balances w USING(newapi_user_id,asset_type) WHERE l.biz_type='NATIVE_QUOTA_MERGE' AND l.newapi_user_id=$1`, sample[0]).Scan(&beforeUnits, &afterUnits, &wallet); err != nil || afterUnits != beforeUnits+sample[1] || wallet != sample[2]+sample[1] {
			t.Fatal("sample Reserve = previous Reserve + native quota", sample[0], err)
		}
		t.Logf("user=%d Reserve=%d+%d=%d", sample[0], beforeUnits, sample[1], afterUnits)
	}
	if _, err = to.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	t.Logf("NATIVE_QUOTA_MERGE users=%d zero=%d units=%d replay=unchanged; all writes rolled back", users, zero, units)
}
