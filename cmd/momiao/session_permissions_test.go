package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestSessionPermissionContract(t *testing.T) {
	dsn := os.Getenv("S1_PERMISSION_TEST_DSN")
	if dsn == "" {
		t.Skip("explicit local transaction fixture required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("fixture connection failed")
	}
	defer c.Close(context.Background())
	tx, err := c.Begin(ctx)
	if err != nil {
		t.Fatal("fixture transaction failed")
	}
	defer tx.Rollback(context.Background())
	exec := func(sql string) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql); err != nil {
			t.Fatalf("fixture SQL failed: %v", err)
		}
	}
	var empty bool
	if err := tx.QueryRow(ctx, "SELECT NOT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname IN('identity','economy','ops'))").Scan(&empty); err != nil || !empty {
		t.Fatal("fixture requires database without identity schema")
	}
	suffix := fmt.Sprint(time.Now().UnixNano())
	owner, role := "s1_permission_owner_"+suffix, "s1_permission_runtime_"+suffix
	exec("CREATE ROLE " + owner + " NOLOGIN NOINHERIT; CREATE ROLE " + role + " LOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS")
	exec("CREATE SCHEMA identity AUTHORIZATION " + owner)
	exec("CREATE SCHEMA economy AUTHORIZATION " + owner)
	exec("CREATE SCHEMA ops AUTHORIZATION " + owner)
	exec(`CREATE TABLE identity.account_refs(newapi_user_id bigint,security_epoch bigint,security_epoch_changed_at timestamptz,private_fixture text);
CREATE TABLE identity.master_profiles(private_fixture text);
CREATE TABLE identity.native_session_bindings(session_id_hash text,newapi_user_id bigint,session_version bigint,native_auth_version bigint,security_epoch_snapshot bigint,native_created_at timestamptz,bound_at timestamptz)`)
	exec("CREATE TABLE ops.admin_principals(newapi_user_id bigint,status text,authz_epoch bigint,base_role text)")
	exec("ALTER TABLE ops.admin_principals OWNER TO " + owner)
	exec("ALTER TABLE identity.account_refs OWNER TO " + owner + "; ALTER TABLE identity.native_session_bindings OWNER TO " + owner)
	exec("GRANT USAGE ON SCHEMA identity TO " + role)
	exec("GRANT SELECT(newapi_user_id,security_epoch,security_epoch_changed_at) ON identity.account_refs TO " + role)
	exec("GRANT SELECT(session_id_hash,newapi_user_id,session_version,native_auth_version,security_epoch_snapshot,native_created_at), INSERT(session_id_hash,newapi_user_id,session_version,native_auth_version,security_epoch_snapshot,native_created_at), UPDATE(session_version,native_auth_version) ON identity.native_session_bindings TO " + role)
	exec("GRANT USAGE ON SCHEMA ops TO " + role)
	exec("GRANT SELECT(newapi_user_id,status,authz_epoch) ON ops.admin_principals TO " + role)
	for _, tc := range []struct {
		name, change string
		want         bool
	}{
		{"exact grants", "", true},
		{"privileged login with role switch", "", false},
		{"configured login differs", "", false},
		{"extra account read", "GRANT SELECT(private_fixture) ON identity.account_refs TO " + role, false},
		{"extra binding read", "GRANT SELECT(bound_at) ON identity.native_session_bindings TO " + role, false},
		{"extra profile read", "GRANT SELECT ON identity.master_profiles TO " + role, false},
		{"missing ops schema usage", "REVOKE USAGE ON SCHEMA ops FROM " + role, false},
		{"missing ops user read", "REVOKE SELECT(newapi_user_id) ON ops.admin_principals FROM " + role, false},
		{"missing ops status read", "REVOKE SELECT(status) ON ops.admin_principals FROM " + role, false},
		{"missing ops epoch read", "REVOKE SELECT(authz_epoch) ON ops.admin_principals FROM " + role, false},
		{"extra ops role read", "GRANT SELECT(base_role) ON ops.admin_principals TO " + role, false},
		{"extra ops update", "GRANT UPDATE(authz_epoch) ON ops.admin_principals TO " + role, false},
		{"identity schema create", "GRANT CREATE ON SCHEMA identity TO " + role, false},
		{"economy schema create", "GRANT CREATE ON SCHEMA economy TO " + role, false},
		{"missing insert", "REVOKE INSERT(session_id_hash) ON identity.native_session_bindings FROM " + role, false},
		{"missing update", "REVOKE UPDATE(session_version) ON identity.native_session_bindings FROM " + role, false},
		{"extra epoch update", "GRANT UPDATE(security_epoch_snapshot) ON identity.native_session_bindings TO " + role, false},
		{"extra bound insert", "GRANT INSERT(bound_at) ON identity.native_session_bindings TO " + role, false},
		{"extra references", "GRANT REFERENCES(newapi_user_id) ON identity.native_session_bindings TO " + role, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec("SAVEPOINT permission_case")
			defer exec("ROLLBACK TO SAVEPOINT permission_case")
			if tc.change != "" {
				exec(tc.change)
			}
			login := role
			if tc.name == "privileged login with role switch" {
				exec("SET LOCAL ROLE " + role)
			} else {
				// Predicate-only simulation; not evidence of a real runtime login.
				exec("SET LOCAL SESSION AUTHORIZATION " + role)
			}
			if tc.name == "configured login differs" {
				login = owner
			}
			var allowed bool
			if err := tx.QueryRow(ctx, "SELECT ($1::name IS NOT NULL) AND ("+sessionPoolPermissionsSQL+")", login).Scan(&allowed); err != nil {
				t.Fatalf("permission query failed: %v", err)
			}
			if allowed != tc.want {
				t.Errorf("startup allowed=%v, want %v", allowed, tc.want)
			}
		})
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal("fixture rollback failed")
	}
	var absent bool
	if err := c.QueryRow(ctx, "SELECT NOT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname IN('identity','economy','ops')) AND NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=ANY($1))", []string{owner, role}).Scan(&absent); err != nil || !absent {
		t.Fatal("fixture cleanup not proven")
	}
}
