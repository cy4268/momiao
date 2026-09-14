package platform

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHistoryDatabaseIdentity(t *testing.T) {
	dsn := os.Getenv("HISTORY_DATABASE_TEST_DSN")
	if dsn == "" {
		t.Skip("explicit read-only local database fixture required")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || cfg.ConnConfig.Host != "127.0.0.1" || cfg.ConnConfig.Port != 55432 || cfg.ConnConfig.Database != "momiao_m2_test" {
		t.Fatal("unexpected fixture target")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	store, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal("fixture wallet connection failed")
	}
	defer store.Close()
	authority, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("fixture authority configuration failed")
	}
	defer authority.Close()
	t.Run("same live database", func(t *testing.T) {
		if store.CheckSameDatabase(ctx, authority) != nil {
			t.Fatal("same live database rejected")
		}
	})
	t.Run("missing authority", func(t *testing.T) {
		if store.CheckSameDatabase(ctx, nil) == nil {
			t.Fatal("missing authority accepted")
		}
	})
	t.Run("different configured database", func(t *testing.T) {
		different := cfg.Copy()
		different.ConnConfig.Database = "unopened_identity_mismatch"
		pool, e := pgxpool.NewWithConfig(ctx, different)
		if e != nil {
			t.Fatal(e)
		}
		defer pool.Close()
		if store.CheckSameDatabase(ctx, pool) == nil {
			t.Fatal("different database accepted")
		}
	})
	t.Run("fallback rejected", func(t *testing.T) {
		fallback := cfg.Copy()
		fallback.ConnConfig.Fallbacks = []*pgconn.FallbackConfig{{Host: "127.0.0.1", Port: 55433}}
		pool, e := pgxpool.NewWithConfig(ctx, fallback)
		if e != nil {
			t.Fatal(e)
		}
		defer pool.Close()
		if store.CheckSameDatabase(ctx, pool) == nil {
			t.Fatal("fallback endpoint accepted")
		}
	})
}
