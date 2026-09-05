package platform

import (
	"context"
	"errors"
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestOpenRejectsEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, dsn := range []string{"", " ", "\r\n\t"} {
		s, err := Open(ctx, dsn)
		if s != nil {
			s.Close()
			t.Fatal("empty DSN returned a store")
		}
		if !errors.Is(err, ErrInvalidDatabaseURL) {
			t.Fatalf("empty DSN error: %v", err)
		}
	}
}

func TestMutationValidation(t *testing.T) {
	base := Mutation{UserID: 1, Asset: ReserveAPICredit, DeltaUnits: 1, BizType: "TEST", BizID: "one", EntryType: "CREDIT", IdempotencyKey: "0123456789abcdef"}
	for _, key := range []string{"", "123456789012345", strings.Repeat("x", 129), "0123456789abcde ", "0123456789abcde\n", "0123456789abcdeé"} {
		m := base
		m.IdempotencyKey = key
		if _, _, err := m.hashes(); !errors.Is(err, ErrInvalidMutation) {
			t.Fatalf("invalid key %q: %v", key, err)
		}
	}
	for _, key := range []string{base.IdempotencyKey, strings.Repeat("~", 128)} {
		m := base
		m.IdempotencyKey = key
		if _, _, err := m.hashes(); err != nil {
			t.Fatal(err)
		}
	}
	for _, change := range []func(*Mutation){func(m *Mutation) { m.UserID = 0 }, func(m *Mutation) { m.Asset = "ACTIVE" }, func(m *Mutation) { m.DeltaUnits = 0 }, func(m *Mutation) { m.BizID = "" }, func(m *Mutation) { m.EntryType = "" }, func(m *Mutation) { m.BizType = " " }} {
		m := base
		change(&m)
		if _, _, err := m.hashes(); !errors.Is(err, ErrInvalidMutation) {
			t.Fatalf("invalid mutation accepted: %+v", m)
		}
	}
	_, h1, _ := base.hashes()
	m := base
	m.IdempotencyKey = "different-key-123"
	_, h2, _ := m.hashes()
	if h1 != h2 {
		t.Fatal("key is not semantic")
	}
	m.DeltaUnits++
	_, h2, _ = m.hashes()
	if h1 == h2 {
		t.Fatal("amount missing from semantics")
	}
}

func TestUUIDv7(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id, err := uuidV7()
		if err != nil {
			t.Fatal(err)
		}
		if len(id) != 36 || id[14] != '7' || !strings.ContainsRune("89ab", rune(id[19])) || seen[id] {
			t.Fatalf("bad UUID %s", id)
		}
		seen[id] = true
	}
}

func integrationStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("MOMIAO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MOMIAO_TEST_DATABASE_URL absent: PostgreSQL integration not run")
	}
	if err := validateTestDatabase(dsn); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	var db string
	if err = s.pool.QueryRow(ctx, "select current_database()").Scan(&db); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(db, "momiao_test_") {
		t.Fatal("refusing writes outside momiao_test_ database")
	}
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return s
}
func mustEnsure(t *testing.T, s *Store, u int64) {
	t.Helper()
	if err := s.EnsureAccount(context.Background(), u); err != nil {
		t.Fatal(err)
	}
}
func mutation(u int64, id string, delta int64) Mutation {
	return Mutation{UserID: u, Asset: ReserveAPICredit, DeltaUnits: delta, BizType: "TEST", BizID: id, EntryType: "ADJUSTMENT", IdempotencyKey: "key-for-operation-" + id}
}
func mustApply(t *testing.T, s *Store, m Mutation) LedgerEntry {
	t.Helper()
	r, e := s.Apply(context.Background(), m)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestTestDatabaseGuard(t *testing.T) {
	for _, tc := range []struct {
		dsn   string
		valid bool
	}{
		{"postgres://localhost/postgres", false}, {"dbname=production", false},
		{"postgres://localhost/momiao_test_abc", true}, {"dbname=momiao_test_abc", true},
		{"postgres://localhost/momiao_test_abc?dbname=production", false},
		{"dbname=momiao_test_abc dbname=production", false},
		{"postgres://localhost/not_momiao_test_abc", false},
	} {
		err := validateTestDatabase(tc.dsn)
		if (err == nil) != tc.valid {
			t.Fatalf("guard %s: %v", tc.dsn, err)
		}
	}
}

func TestStoreIntegration(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	base := time.Now().UnixMicro()
	t.Run("migration_lock_concurrent_unknown", func(t *testing.T) {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(714622314783630001)"); err != nil {
			t.Fatal(err)
		}
		c, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		if err = s.Migrate(c); err == nil {
			t.Fatal("migration ignored advisory lock")
		}
		if err = tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for i := 0; i < 12; i++ {
			wg.Go(func() {
				if e := s.Migrate(ctx); e != nil {
					t.Error(e)
				}
			})
		}
		wg.Wait()
		if _, err = s.pool.Exec(ctx, "INSERT INTO platform_meta.schema_migrations(version,checksum) VALUES(999,'unknown')"); err != nil {
			t.Fatal(err)
		}
		defer s.pool.Exec(ctx, "DELETE FROM platform_meta.schema_migrations WHERE version=999")
		if err = s.Migrate(ctx); !errors.Is(err, ErrMigrationMismatch) {
			t.Fatalf("unknown migration: %v", err)
		}
	})
	t.Run("migration_repeat_checksum", func(t *testing.T) {
		if err := s.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
		var checksum string
		if err := s.pool.QueryRow(ctx, "select checksum from platform_meta.schema_migrations where version=1").Scan(&checksum); err != nil {
			t.Fatal(err)
		}
		if _, err := s.pool.Exec(ctx, "update platform_meta.schema_migrations set checksum='tampered' where version=1"); err != nil {
			t.Fatal(err)
		}
		defer s.pool.Exec(ctx, "update platform_meta.schema_migrations set checksum=$1 where version=1", checksum)
		if err := s.Migrate(ctx); !errors.Is(err, ErrMigrationMismatch) {
			t.Fatalf("checksum: %v", err)
		}
	})
	t.Run("initialization_repeat_missing", func(t *testing.T) {
		u := base
		mustEnsure(t, s, u)
		mustEnsure(t, s, u)
		var wg sync.WaitGroup
		for i := 0; i < 12; i++ {
			wg.Go(func() {
				if err := s.EnsureAccount(ctx, u); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
		for _, a := range []Asset{ReserveAPICredit, AvailableChips} {
			w, err := s.ReadWallet(ctx, u, a)
			if err != nil || w.BalanceUnits != 0 || w.LedgerSeq != 0 || w.Version != 1 {
				t.Fatalf("wallet %+v %v", w, err)
			}
		}
		var n int
		if err := s.pool.QueryRow(ctx, "select count(*) from economy.wallet_balances where newapi_user_id=$1", u).Scan(&n); err != nil || n != 2 {
			t.Fatalf("rows %d %v", n, err)
		}
		if _, err := s.ReadWallet(ctx, u+999999, ReserveAPICredit); !errors.Is(err, ErrWalletNotFound) {
			t.Fatal(err)
		}
		if _, err := s.Apply(ctx, mutation(u+999999, "missing-wallet", 1)); !errors.Is(err, ErrWalletNotFound) {
			t.Fatalf("missing mutation: %v", err)
		}
		for _, limit := range []int{0, 101} {
			if _, err := s.Ledger(ctx, u, ReserveAPICredit, 0, limit); !errors.Is(err, ErrInvalidPage) {
				t.Fatalf("invalid page: %v", err)
			}
		}
	})
	t.Run("replay_conflict_business_identity_isolation", func(t *testing.T) {
		u := base + 1
		mustEnsure(t, s, u)
		id := strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "")
		m := mutation(u, id, 10)
		first := mustApply(t, s, m)
		mustApply(t, s, mutation(u, id+"later", 5))
		replay := mustApply(t, s, m)
		if replay != first {
			t.Fatalf("original replay changed: %+v %+v", first, replay)
		}
		m.DeltaUnits++
		if _, err := s.Apply(ctx, m); !errors.Is(err, ErrIdempotencyConflict) {
			t.Fatal(err)
		}
		m.DeltaUnits--
		m.IdempotencyKey += "new"
		if r := mustApply(t, s, m); r != first {
			t.Fatal("business identity duplicated")
		}
		m.BizID += "changed"
		if _, err := s.Apply(ctx, m); !errors.Is(err, ErrIdempotencyConflict) {
			t.Fatal("alias key not bound", err)
		}
		other := u + 100
		mustEnsure(t, s, other)
		m = mutation(other, id, 10)
		if _, err := s.Apply(ctx, m); !errors.Is(err, ErrIdempotencyConflict) {
			t.Fatal("cross-user business replay", err)
		}
		m.BizID += "other"
		mustApply(t, s, m)
		rows, err := s.Ledger(ctx, u, ReserveAPICredit, 0, 1)
		if err != nil || len(rows) != 1 || rows[0] != first {
			t.Fatalf("first page %+v %v", rows, err)
		}
		rows, err = s.Ledger(ctx, u, ReserveAPICredit, rows[0].LedgerSeq, 100)
		if err != nil || len(rows) != 1 || rows[0].UserID != u {
			t.Fatalf("second page %+v %v", rows, err)
		}
		rows, err = s.Ledger(ctx, other, AvailableChips, 0, 100)
		if err != nil || len(rows) != 0 {
			t.Fatal("personal isolation", err)
		}
	})
	t.Run("concurrent_duplicate_and_debit", func(t *testing.T) {
		u := base + 2
		mustEnsure(t, s, u)
		id := time.Now().Format("150405.000000000")
		m := mutation(u, id, 10)
		var wg sync.WaitGroup
		errs := make(chan error, 20)
		ids := make(chan string, 20)
		for i := 0; i < 20; i++ {
			wg.Go(func() { r, e := s.Apply(ctx, m); errs <- e; ids <- r.ID })
		}
		wg.Wait()
		close(errs)
		close(ids)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		first := ""
		for id := range ids {
			if first != "" && id != first {
				t.Fatal("duplicate result")
			}
			first = id
		}
		var ok, insufficient atomic.Int32
		for i := 0; i < 20; i++ {
			i := i
			wg.Go(func() {
				mm := mutation(u, id+strings.Repeat("x", i+1), -1)
				_, e := s.Apply(ctx, mm)
				if e == nil {
					ok.Add(1)
				} else if errors.Is(e, ErrInsufficientBalance) {
					insufficient.Add(1)
				} else {
					t.Error(e)
				}
			})
		}
		wg.Wait()
		if ok.Load() != 10 || insufficient.Load() != 10 {
			t.Fatalf("debits %d insufficient %d", ok.Load(), insufficient.Load())
		}
		w, e := s.ReadWallet(ctx, u, ReserveAPICredit)
		if e != nil || w.BalanceUnits != 0 || w.LedgerSeq != 11 || w.Version != 12 {
			t.Fatalf("wallet %+v %v", w, e)
		}
	})
	t.Run("overflow_and_rollback", func(t *testing.T) {
		u := base + 3
		mustEnsure(t, s, u)
		id := time.Now().Format("150405.000000000")
		mustApply(t, s, mutation(u, id, math.MaxInt64))
		m := mutation(u, id+"overflow", 1)
		if _, err := s.Apply(ctx, m); !errors.Is(err, ErrBalanceOverflow) {
			t.Fatal(err)
		}
		m.DeltaUnits = -1
		mustApply(t, s, m)
		m = mutation(u, id+"negative", math.MinInt64)
		if _, err := s.Apply(ctx, m); !errors.Is(err, ErrInsufficientBalance) {
			t.Fatal(err)
		}
		rows, e := s.Ledger(ctx, u, ReserveAPICredit, 0, 100)
		if e != nil || len(rows) != 2 {
			t.Fatalf("rollback rows %d %v", len(rows), e)
		}
	})
	t.Run("statement_failure_rollback", func(t *testing.T) {
		u := base + 4
		mustEnsure(t, s, u)
		_, err := s.pool.Exec(ctx, `CREATE FUNCTION economy.test_fail_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected failure'; END $$; CREATE TRIGGER test_fail_insert BEFORE INSERT ON economy.wallet_ledger FOR EACH STATEMENT EXECUTE FUNCTION economy.test_fail_insert()`)
		if err != nil {
			t.Fatal(err)
		}
		defer s.pool.Exec(ctx, "DROP TRIGGER IF EXISTS test_fail_insert ON economy.wallet_ledger; DROP FUNCTION IF EXISTS economy.test_fail_insert()")
		m := mutation(u, time.Now().Format("150405.000000000"), 10)
		if _, err = s.Apply(ctx, m); err == nil {
			t.Fatal("injection did not fail")
		}
		w, err := s.ReadWallet(ctx, u, ReserveAPICredit)
		if err != nil || w.BalanceUnits != 0 || w.LedgerSeq != 0 || w.Version != 1 {
			t.Fatalf("partial wallet %+v %v", w, err)
		}
		if _, err = s.pool.Exec(ctx, "DROP TRIGGER test_fail_insert ON economy.wallet_ledger; DROP FUNCTION economy.test_fail_insert()"); err != nil {
			t.Fatal(err)
		}
		mustApply(t, s, m)
	})
	t.Run("immutable_history", func(t *testing.T) {
		tables := []struct{ table, trigger, column string }{
			{"economy.wallet_ledger", "wallet_ledger_immutable", "delta_units"},
			{"economy.asset_transactions", "asset_transactions_immutable", "operation_type"},
			{"platform_meta.mutation_idempotency_records", "mutation_idempotency_immutable", "scope"},
		}
		for _, target := range tables {
			for _, op := range []string{"update", "delete", "truncate"} {
				for _, protected := range []bool{true, false} {
					name := target.table + "/" + op + "/trigger_enabled"
					if !protected {
						name = target.table + "/" + op + "/trigger_removed_negative_control"
					}
					t.Run(name, func(t *testing.T) {
						tx, err := s.pool.Begin(ctx)
						if err != nil {
							t.Fatal(err)
						}
						// All DDL and destructive negative controls are rolled back.
						defer rollback(tx)
						for _, other := range tables {
							if other.table != target.table || !protected {
								if _, err = tx.Exec(ctx, "DROP TRIGGER "+other.trigger+" ON "+other.table); err != nil {
									t.Fatal(err)
								}
							}
						}
						if _, err = tx.Exec(ctx, "DROP TRIGGER daily_checkins_immutable ON rewards.daily_checkins"); err != nil {
							t.Fatal(err)
						}
						// Remove FK dependents for a real DELETE, so a constraint
						// cannot mask a missing trigger. Other triggers are absent.
						if op == "delete" && target.table != "platform_meta.mutation_idempotency_records" {
							if _, err = tx.Exec(ctx, "DELETE FROM platform_meta.mutation_idempotency_records"); err != nil {
								t.Fatal(err)
							}
							if target.table == "economy.asset_transactions" {
								if _, err = tx.Exec(ctx, "DELETE FROM rewards.daily_checkins; DELETE FROM economy.wallet_ledger"); err != nil {
									t.Fatal(err)
								}
							}
						}
						var count int
						if err = tx.QueryRow(ctx, "SELECT count(*) FROM "+target.table).Scan(&count); err != nil || count == 0 {
							t.Fatalf("negative control needs real history rows: %d %v", count, err)
						}
						sql := "UPDATE " + target.table + " SET " + target.column + "=" + target.column
						if op == "delete" {
							sql = "DELETE FROM " + target.table
						}
						if op == "truncate" {
							sql = "TRUNCATE " + target.table + " CASCADE"
						}
						_, err = tx.Exec(ctx, sql)
						if protected {
							var pgErr *pgconn.PgError
							if !errors.As(err, &pgErr) || pgErr.Code != "55000" {
								t.Fatalf("expected target immutable trigger SQLSTATE 55000: %s: %v", sql, err)
							}
						} else if err != nil {
							t.Fatalf("operation should succeed with target trigger removed: %s: %v", sql, err)
						}
					})
				}
			}
		}
	})
	t.Run("cancelled_transaction", func(t *testing.T) {
		u := base + 5
		mustEnsure(t, s, u)
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, "select 1 from economy.wallet_balances where newapi_user_id=$1 for update", u); err != nil {
			t.Fatal(err)
		}
		c, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		m := mutation(u, time.Now().Format("150405.000000000"), 1)
		if _, err = s.Apply(c, m); err == nil {
			t.Fatal("cancel accepted")
		}
		if err = tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		w, err := s.ReadWallet(ctx, u, ReserveAPICredit)
		if err != nil || w.BalanceUnits != 0 || w.LedgerSeq != 0 {
			t.Fatalf("cancel changed wallet %+v %v", w, err)
		}
		mustApply(t, s, m)
	})
}
