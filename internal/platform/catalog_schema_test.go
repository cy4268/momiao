package platform

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func catalogTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("MOMIAO_CATALOG_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("dedicated m3_catalog_platform PostgreSQL database required")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil || cfg.Host != "127.0.0.1" || cfg.Port != 55432 || !strings.HasPrefix(cfg.Database, "m3_catalog_platform_") {
		t.Fatal("refusing a nonlocal or non-catalog test database")
	}
	s, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatal("local catalog test database unavailable")
	}
	t.Cleanup(s.Close)
	if err = s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func osCatalogDSN(t *testing.T) string {
	t.Helper()
	return os.Getenv("MOMIAO_CATALOG_TEST_DATABASE_URL")
}
func TestCatalogSchemaSeparatesProjectionAndPreservesHistory(t *testing.T) {
	s := catalogTestStore(t)
	ctx := context.Background()
	for _, table := range []string{"model_catalog_metadata", "model_catalog_publication", "model_sync_snapshots", "model_sync_attempts", "model_sync_state", "model_availability_mappings", "historical_model_identity", "model_metadata_revisions"} {
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, "catalog."+table).Scan(&exists); err != nil || !exists {
			t.Fatalf("catalog table missing: %s %v", table, err)
		}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(tx)
	model := "schema/" + announcementID(t)
	identity := announcementID(t)
	snapshot := announcementID(t)
	if _, err = tx.Exec(ctx, `INSERT INTO catalog.model_catalog_metadata(model_id,display_name,family) VALUES($1,'Synthetic','other')`, model); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO catalog.model_catalog_publication(model_id) VALUES($1)`, model); err != nil {
		t.Fatal(err)
	}
	var state string
	if err = tx.QueryRow(ctx, `SELECT publication_state FROM catalog.model_catalog_publication WHERE model_id=$1`, model).Scan(&state); err != nil || state != "PENDING_METADATA" {
		t.Fatal("new model did not default to pending", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO catalog.historical_model_identity(historical_identity_id,model_id,display_name_snapshot,family_snapshot,effective_from) VALUES($1,$2,'Synthetic','other',now())`, identity, model); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO catalog.model_sync_snapshots(sync_snapshot_id,source_identity,source_hash,observed_model_count,status,observed_at,source_models) VALUES($1,'momiao.native-catalog.v1',decode(repeat('ab',32),'hex'),0,'VERIFIED',now(),'[]')`, snapshot); err != nil {
		t.Fatal(err)
	}
	assertReject := func(name, sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, "SAVEPOINT catalog_guard"); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, sql, args...); err == nil {
			t.Fatalf("schema accepted %s", name)
		}
		if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT catalog_guard"); err != nil {
			t.Fatal(err)
		}
	}
	assertReject("opaque identity rewrite", `UPDATE catalog.model_catalog_metadata SET model_id='replacement' WHERE model_id=$1`, model)
	assertReject("negative context", `UPDATE catalog.model_catalog_metadata SET context_length=-1 WHERE model_id=$1`, model)
	assertReject("unknown publication state", `UPDATE catalog.model_catalog_publication SET publication_state='AVAILABLE' WHERE model_id=$1`, model)
	assertReject("snapshot rewrite", `UPDATE catalog.model_sync_snapshots SET source_models='[{}]' WHERE sync_snapshot_id=$1`, snapshot)
	assertReject("history delete", `DELETE FROM catalog.historical_model_identity WHERE historical_identity_id=$1`, identity)
	assertReject("metadata delete", `DELETE FROM catalog.model_catalog_metadata WHERE model_id=$1`, model)
	assertReject("history identity rewrite", `UPDATE catalog.historical_model_identity SET display_name_snapshot='Rewritten' WHERE historical_identity_id=$1`, identity)
	assertReject("history backward end", `UPDATE catalog.historical_model_identity SET effective_until=effective_from-interval '1s' WHERE historical_identity_id=$1`, identity)
	if _, err = tx.Exec(ctx, `UPDATE catalog.historical_model_identity SET effective_until=now()+interval '1s' WHERE historical_identity_id=$1`, identity); err != nil {
		t.Fatal(err)
	}
	assertReject("history end rewrite", `UPDATE catalog.historical_model_identity SET effective_until=now()+interval '2s' WHERE historical_identity_id=$1`, identity)
	principal := announcementID(t)
	if _, err = tx.Exec(ctx, `INSERT INTO ops.admin_principals(admin_principal_id,newapi_user_id,base_role) VALUES($1,987654321123,'OPERATOR');`, principal); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ops.admin_principal_scopes(admin_principal_id,scope) VALUES($1,'MODELS')`, principal); err != nil {
		t.Fatal("Models scope migration missing", err)
	}
	assertReject("unknown scope", `INSERT INTO ops.admin_principal_scopes(admin_principal_id,scope) VALUES($1,'ROOT')`, principal)
	if _, err = tx.Exec(ctx, `INSERT INTO ops.admin_principal_scopes(admin_principal_id,scope) VALUES($1,'ANNOUNCEMENTS')`, principal); err != nil {
		t.Fatal("frozen announcements scope broken", err)
	}
}
