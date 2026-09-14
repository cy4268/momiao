package poker

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func passwordProfileFixture(t *testing.T) (owner, runtime, platform, ops *pgxpool.Pool) {
	t.Helper()
	var input struct {
		OwnerURL, RuntimeURL, PlatformURL, OpsURL               string
		Database, OwnerRole, RuntimeRole, PlatformRole, OpsRole string
		DatabaseOID, OwnerOID, RuntimeOID, PlatformOID, OpsOID  uint32
	}
	path := os.Getenv("POKER_PROFILE_TEST_CONNECTION_FILE")
	if path == "" {
		t.Fatal("external profile fixture connection file is required")
	}
	body, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(body, &input) != nil || input.Database == "" || input.DatabaseOID == 0 {
		t.Fatal("external profile fixture connection metadata is invalid")
	}
	urls := []string{input.OwnerURL, input.RuntimeURL, input.PlatformURL, input.OpsURL}
	roles := []string{input.OwnerRole, input.RuntimeRole, input.PlatformRole, input.OpsRole}
	oids := []uint32{input.OwnerOID, input.RuntimeOID, input.PlatformOID, input.OpsOID}
	pools := make([]*pgxpool.Pool, 4)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for i, uri := range urls {
		if roles[i] == "" || oids[i] == 0 || slices.Contains(roles[:i], roles[i]) || slices.Contains(oids[:i], oids[i]) {
			t.Fatal("fixture requires four distinct expected role names and OIDs")
		}
		config, err := pgxpool.ParseConfig(uri)
		if err != nil || uri == "" {
			t.Fatal("fixture connection URL is invalid")
		}
		if config.ConnConfig.Host != "127.0.0.1" || config.ConnConfig.Port != 55432 || config.ConnConfig.Database != input.Database {
			t.Fatal("fixture connection must use the exact loopback database")
		}
		if len(config.ConnConfig.Fallbacks) != 0 || config.ConnConfig.TLSConfig != nil || len(config.ConnConfig.RuntimeParams) != 0 {
			t.Fatal("fixture fallback or session override is forbidden")
		}
		if i == 0 {
			config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
				_, err := conn.Exec(ctx, "SET ROLE "+pgx.Identifier{input.OwnerRole}.Sanitize())
				return err
			}
		} else if config.ConnConfig.User != roles[i] {
			t.Fatal("fixture LOGIN identity differs from the named role")
		}
		config.MinConns, config.MaxConns = 0, 1
		config.ConnConfig.ConnectTimeout = 5 * time.Second
		pool, err := pgxpool.NewWithConfig(ctx, config)
		if err != nil {
			t.Fatal("fixture pool construction failed")
		}
		t.Cleanup(pool.Close)
		pools[i] = pool
		var database, user string
		var databaseOID, roleOID, ownerOID uint32
		var login, super, createDB, createRole, replication, bypass, member bool
		err = pool.QueryRow(ctx, `SELECT current_database(),d.oid,current_user,r.oid,d.datdba,
 r.rolcanlogin,r.rolsuper,r.rolcreatedb,r.rolcreaterole,r.rolreplication,r.rolbypassrls,
 EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members m WHERE m.roleid=r.oid OR m.member=r.oid)
 FROM pg_catalog.pg_database d,pg_catalog.pg_roles r
 WHERE d.datname=current_database() AND r.rolname=current_user`).Scan(
			&database, &databaseOID, &user, &roleOID, &ownerOID, &login,
			&super, &createDB, &createRole, &replication, &bypass, &member)
		if err != nil || database != input.Database || databaseOID != input.DatabaseOID || user != roles[i] || roleOID != oids[i] {
			t.Fatal("fixture actual connection identity differs from the expected manifest")
		}
		if ownerOID != input.OwnerOID || login != (i != 0) || super || createDB || createRole || replication || bypass || member {
			t.Fatal("fixture role ownership or privilege separation failed")
		}
	}
	return pools[0], pools[1], pools[2], pools[3]
}
