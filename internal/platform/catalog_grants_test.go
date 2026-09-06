package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// This role connects independently; successful owner queries are not evidence
// that the deployment runtime has the required grants.
func catalogRuntimeStore(t *testing.T, owner *Store) *Store {
	t.Helper()
	ctx := context.Background()
	id := strings.ReplaceAll(announcementID(t), "-", "")
	role := "m3_catalog_runtime_" + id
	quoted := pgx.Identifier{role}.Sanitize()
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	password := hex.EncodeToString(secret)
	if _, err := owner.pool.Exec(ctx, fmt.Sprintf("CREATE ROLE %s LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT PASSWORD '%s'", quoted, password)); err != nil {
		t.Fatal("synthetic runtime role creation failed")
	}
	t.Cleanup(func() {
		// Only revoke privileges explicitly granted in this isolated test database.
		// DROP ROLE has no CASCADE and fails if any unexpected owned object exists.
		_, err := owner.pool.Exec(context.Background(), fmt.Sprintf(`REVOKE ALL ON ALL TABLES IN SCHEMA catalog,ops FROM %s; REVOKE ALL ON ALL FUNCTIONS IN SCHEMA catalog,ops FROM %s; REVOKE ALL ON SCHEMA catalog,ops FROM %s; DROP ROLE %s`, quoted, quoted, quoted, quoted))
		if err != nil {
			t.Error("synthetic runtime role cleanup failed")
		}
	})
	u, err := url.Parse(osCatalogDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(role, password)
	runtimeStore, err := Open(ctx, u.String())
	if err != nil {
		t.Fatal("independent synthetic runtime connection failed")
	}
	t.Cleanup(runtimeStore.Close)
	var actual string
	if err = runtimeStore.pool.QueryRow(ctx, "SELECT current_user").Scan(&actual); err != nil || actual != role {
		t.Fatal("test still used database owner authority")
	}
	snapshot := catalogStoreSnapshot(t, "runtime/"+id)
	if _, err = runtimeStore.SyncCatalog(ctx, func(context.Context) (NativeCatalog, error) { return snapshot, nil }); err == nil {
		t.Fatal("PUBLIC unexpectedly granted catalog write access")
	}
	grants := fmt.Sprintf(`
 GRANT USAGE ON SCHEMA catalog,ops TO %[1]s;
 GRANT SELECT ON ops.admin_principals,ops.admin_principal_scopes TO %[1]s;
 GRANT UPDATE(updated_at) ON ops.admin_principals TO %[1]s;
 GRANT SELECT,INSERT ON ops.admin_operations,ops.model_previews TO %[1]s;
 GRANT SELECT,INSERT,UPDATE ON catalog.model_catalog_metadata,catalog.model_catalog_publication,catalog.model_availability_mappings TO %[1]s;
 GRANT SELECT,UPDATE ON catalog.model_sync_state TO %[1]s;
 GRANT SELECT,INSERT ON catalog.model_sync_snapshots,catalog.model_sync_attempts,catalog.model_metadata_revisions,catalog.historical_model_identity TO %[1]s;
 GRANT UPDATE(effective_until) ON catalog.historical_model_identity TO %[1]s;`, quoted)
	if _, err = owner.pool.Exec(ctx, grants); err != nil {
		t.Fatal("minimal runtime grants failed", err)
	}
	if result, err := runtimeStore.SyncCatalog(ctx, func(context.Context) (NativeCatalog, error) { return snapshot, nil }); err != nil || result.Status != "VERIFIED" {
		t.Fatal("minimal runtime sync failed", err)
	}
	return runtimeStore
}
func TestCatalogRuntimeUsesOnlyNarrowGrants(t *testing.T) {
	owner := catalogTestStore(t)
	runtimeStore := catalogRuntimeStore(t, owner)
	ctx := context.Background()
	if _, err := runtimeStore.CatalogSyncStatus(ctx); err != nil {
		t.Fatal(err)
	}
	for name, sql := range map[string]string{
		"schema_migration":      `CREATE TABLE catalog.forbidden_runtime_ddl(id int)`,
		"principal_role_change": `UPDATE ops.admin_principals SET base_role='SUPER_ADMIN'`,
		"scope_self_grant":      `INSERT INTO ops.admin_principal_scopes SELECT admin_principal_id,'MODELS' FROM ops.admin_principals ON CONFLICT DO NOTHING`,
		"history_delete":        `DELETE FROM catalog.historical_model_identity`,
		"snapshot_rewrite":      `UPDATE catalog.model_sync_snapshots SET source_models='[]'`,
		"audit_rewrite":         `UPDATE ops.admin_operations SET details='{}'`,
		"private_profile_read":  `SELECT * FROM identity.master_profiles`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := runtimeStore.pool.Exec(ctx, sql); err == nil {
				t.Fatal("runtime crossed grant boundary")
			}
		})
	}
	principal := announcementID(t)
	user := time.Now().UnixMicro()
	if _, err := owner.pool.Exec(ctx, `INSERT INTO ops.admin_principals(admin_principal_id,newapi_user_id,base_role) VALUES($1,$2,'OPERATOR')`, principal, user); err != nil {
		t.Fatal(err)
	}
	tx, err := runtimeStore.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, `SELECT admin_principal_id FROM ops.admin_principals WHERE admin_principal_id=$1 FOR UPDATE`, principal); err != nil {
		t.Fatal("runtime could not lock authority without role-changing rights", err)
	}
}
