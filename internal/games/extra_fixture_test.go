package games

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

func runtimeRole(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(os.Getenv("MOMIAO_GAMES_TEST_CONNECTION_FILE"))
	var c struct{ RuntimeRole string }
	if err != nil || json.Unmarshal(raw, &c) != nil {
		t.Fatal("missing local connection")
	}
	return c.RuntimeRole
}

// Isolated, explicitly opted-in branch fixture only. G3 owns missing 0013/0014.
// Verify the already migrated 0001..0012 hashes and keep pending receipts in a
// separate local table; never weaken Store.Migrate or forge its global registry.
// The assembled release must still pass normal contiguous fresh migration.
func isolatedPendingMigrations(ctx context.Context, owner *platform.Store) error {
	return owner.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(73190615)`); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM platform_meta.schema_migrations`).Scan(&count); err != nil {
			return err
		}
		if count != 12 {
			return fmt.Errorf("isolated pending fixture requires exactly 12 baseline migrations, got %d", count)
		}
		for i := 1; i <= 12; i++ {
			paths, err := filepath.Glob(fmt.Sprintf("../platform/migrations/%04d_*.sql", i))
			if err != nil || len(paths) != 1 {
				return fmt.Errorf("missing baseline %d", i)
			}
			raw, err := os.ReadFile(paths[0])
			if err != nil {
				return err
			}
			hash := sha256.Sum256(raw)
			var saved string
			if err = tx.QueryRow(ctx, `SELECT checksum FROM platform_meta.schema_migrations WHERE version=$1`, i).Scan(&saved); err != nil {
				return err
			}
			if saved != hex.EncodeToString(hash[:]) {
				return fmt.Errorf("baseline checksum mismatch %d", i)
			}
		}
		if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS platform_meta.g1_pending_migrations(version integer PRIMARY KEY,checksum text NOT NULL,scope text NOT NULL CHECK(scope='LOCAL_BRANCH_ONLY'))`); err != nil {
			return err
		}
		for _, version := range []int{15, 16} {
			paths, err := filepath.Glob(fmt.Sprintf("../platform/migrations/%04d_*.sql", version))
			if err != nil {
				return err
			}
			if len(paths) == 0 {
				continue
			}
			if len(paths) != 1 {
				return fmt.Errorf("duplicate pending migration")
			}
			raw, err := os.ReadFile(paths[0])
			if err != nil {
				return err
			}
			hash := sha256.Sum256(raw)
			actual := hex.EncodeToString(hash[:])
			var saved string
			err = tx.QueryRow(ctx, `SELECT checksum FROM platform_meta.g1_pending_migrations WHERE version=$1`, version).Scan(&saved)
			if err == nil {
				if saved != actual {
					return fmt.Errorf("pending migration %d changed; use a fresh owned fixture", version)
				}
				continue
			}
			if err != pgx.ErrNoRows {
				return err
			}
			if _, err = tx.Exec(ctx, string(raw), pgx.QueryExecModeSimpleProtocol); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO platform_meta.g1_pending_migrations VALUES($1,$2,'LOCAL_BRANCH_ONLY')`, version, actual); err != nil {
				return err
			}
		}
		return nil
	})
}
