package poker

import (
	"context"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type passwordProfileFixtureInput struct {
	OwnerURL, RuntimeURL, PlatformURL, OpsURL               string
	Database, OwnerRole, RuntimeRole, PlatformRole, OpsRole string
	DatabaseOID, OwnerOID, RuntimeOID, PlatformOID, OpsOID  uint32
}

type passwordProfileOpener func(*testing.T) (*pgxpool.Pool, *pgxpool.Pool, *pgxpool.Pool, *pgxpool.Pool)

func passwordProfileFixture(t *testing.T, raw21 bool) (owner, runtime, platform, ops *pgxpool.Pool) {
	t.Helper()
	return passwordProfileFixtureFactory(t, raw21)(t)
}

func passwordProfileFixtureFactory(t *testing.T, raw21 bool) passwordProfileOpener {
	t.Helper()
	if path := os.Getenv("POKER_PROFILE_TEST_CONNECTION_FILE"); path != "" {
		return func(t *testing.T) (*pgxpool.Pool, *pgxpool.Pool, *pgxpool.Pool, *pgxpool.Pool) {
			return openPasswordProfileFixture(t, path)
		}
	}
	if os.Getenv("POKER_TEST_CONNECTION_FILE") == "" {
		return func(t *testing.T) (*pgxpool.Pool, *pgxpool.Pool, *pgxpool.Pool, *pgxpool.Pool) {
			t.Skip("set POKER_PROFILE_TEST_CONNECTION_FILE or POKER_TEST_CONNECTION_FILE for isolated real-PG profile test")
			return nil, nil, nil, nil
		}
	}
	owner, runtime := localPokerDB(t, passwordProfileMigrationSkips(t, raw21)...)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	adminConfig := owner.Config().ConnConfig.Copy()
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal("profile fixture admin connection failed")
	}
	extraRoles := []string{runtime.Config().ConnConfig.User + "_platform", runtime.Config().ConnConfig.User + "_ops"}
	passwords := []string{uuid(), uuid()}
	var createdRoles []string
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		defer admin.Close(cleanup)
		for _, role := range createdRoles {
			if _, err := admin.Exec(cleanup, "DROP ROLE "+pgx.Identifier{role}.Sanitize()); err != nil {
				t.Errorf("profile fixture outsider role cleanup failed: %T", err)
			}
		}
	})
	for i, role := range extraRoles {
		_, err := admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{role}.Sanitize()+" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD '"+passwords[i]+"'")
		if err != nil {
			t.Fatal("profile fixture outsider role creation failed")
		}
		createdRoles = append(createdRoles, role)
	}
	if !raw21 {
		applyLocalPasswordProfileGrants(t, ctx, owner, runtime)
	}
	version := "current migrations"
	if raw21 {
		version = "raw21"
	}
	t.Logf("isolated profile fixture at %s ready; no DSN emitted", version)
	path := localPasswordProfileManifest(t, owner, runtime, adminConfig, extraRoles, passwords)
	return func(t *testing.T) (*pgxpool.Pool, *pgxpool.Pool, *pgxpool.Pool, *pgxpool.Pool) {
		resetLocalPasswordProfile(t, owner, runtime.Config().ConnConfig.User, raw21)
		return openPasswordProfileFixture(t, path)
	}
}

func resetLocalPasswordProfile(t *testing.T, owner *pgxpool.Pool, runtimeRole string, raw21 bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal("profile fixture reset transaction failed")
	}
	defer rollback(tx)
	if !raw21 {
		if _, err = tx.Exec(ctx, "GRANT SELECT,INSERT ON poker.table_access_credentials TO "+pgx.Identifier{runtimeRole}.Sanitize()); err != nil {
			t.Fatal("profile fixture credential grants reset failed")
		}
		for _, sql := range []string{
			"ALTER TABLE poker.table_access_credentials DISABLE TRIGGER table_access_credentials_immutable",
			"DELETE FROM poker.table_access_credentials",
			"ALTER TABLE poker.table_access_credentials ENABLE TRIGGER table_access_credentials_immutable",
		} {
			if _, err = tx.Exec(ctx, sql); err != nil {
				t.Fatal("profile fixture immutable rows reset failed")
			}
		}
	}
	if _, err = tx.Exec(ctx, "DELETE FROM poker.tables"); err != nil {
		t.Fatal("profile fixture tables reset failed")
	}
	var clean bool
	if raw21 {
		err = tx.QueryRow(ctx, "SELECT to_regclass('poker.table_access_credentials') IS NULL AND NOT EXISTS(SELECT 1 FROM poker.tables)").Scan(&clean)
	} else {
		err = tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM poker.tables)
 AND NOT EXISTS(SELECT 1 FROM poker.table_access_credentials)
 AND has_table_privilege($1,'poker.table_access_credentials','SELECT')
 AND has_table_privilege($1,'poker.table_access_credentials','INSERT')`, runtimeRole).Scan(&clean)
	}
	if err != nil || !clean {
		t.Fatal("profile fixture fresh-state verification failed")
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal("profile fixture reset commit failed")
	}
	t.Log("fresh isolated profile state and grants verified")
}

func openPasswordProfileFixture(t *testing.T, path string) (owner, runtime, platform, ops *pgxpool.Pool) {
	t.Helper()
	var input passwordProfileFixtureInput
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

func passwordProfileMigrationSkips(t *testing.T, raw21 bool) []string {
	t.Helper()
	if !raw21 {
		return nil
	}
	var skips []string
	files, err := filepath.Glob(filepath.Join("..", "platform", "migrations", "*.sql"))
	if err != nil {
		t.Fatal("profile migration discovery failed")
	}
	for _, file := range files {
		name := filepath.Base(file)
		if len(name) < 5 {
			t.Fatal("profile migration filename is invalid")
		}
		version, err := strconv.Atoi(name[:4])
		if err != nil {
			t.Fatal("profile migration filename is invalid")
		}
		if version > 21 {
			skips = append(skips, name)
		}
	}
	return skips
}

func localPasswordProfileManifest(t *testing.T, owner, runtime *pgxpool.Pool, adminBase *pgx.ConnConfig, extraRoles, passwords []string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	adminConfig := adminBase.Copy()
	adminConfig.Database = owner.Config().ConnConfig.Database
	input := passwordProfileFixtureInput{
		OwnerURL:     passwordProfileURL(adminConfig),
		RuntimeURL:   passwordProfileURL(runtime.Config().ConnConfig),
		PlatformURL:  passwordProfileRoleURL(runtime.Config().ConnConfig, extraRoles[0], passwords[0]),
		OpsURL:       passwordProfileRoleURL(runtime.Config().ConnConfig, extraRoles[1], passwords[1]),
		Database:     adminConfig.Database,
		RuntimeRole:  runtime.Config().ConnConfig.User,
		PlatformRole: extraRoles[0],
		OpsRole:      extraRoles[1],
	}
	var databaseOwnerOID uint32
	if err := owner.QueryRow(ctx, `SELECT d.oid,current_user,r.oid,d.datdba
 FROM pg_catalog.pg_database d,pg_catalog.pg_roles r
 WHERE d.datname=current_database() AND r.rolname=current_user`).Scan(
		&input.DatabaseOID, &input.OwnerRole, &input.OwnerOID, &databaseOwnerOID); err != nil || databaseOwnerOID != input.OwnerOID {
		t.Fatal("profile fixture owner metadata query failed")
	}
	if err := owner.QueryRow(ctx, `SELECT
 (SELECT oid FROM pg_catalog.pg_roles WHERE rolname=$1),
 (SELECT oid FROM pg_catalog.pg_roles WHERE rolname=$2),
 (SELECT oid FROM pg_catalog.pg_roles WHERE rolname=$3)`,
		input.RuntimeRole, input.PlatformRole, input.OpsRole).Scan(&input.RuntimeOID, &input.PlatformOID, &input.OpsOID); err != nil {
		t.Fatal("profile fixture runtime metadata query failed")
	}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal("profile fixture manifest encoding failed")
	}
	path := filepath.Join(t.TempDir(), "profile-connections.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal("profile fixture manifest write failed")
	}
	return path
}

func passwordProfileRoleURL(base *pgx.ConnConfig, role, password string) string {
	config := base.Copy()
	config.User, config.Password = role, password
	return passwordProfileURL(config)
}

func passwordProfileURL(config *pgx.ConnConfig) string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(config.User, config.Password),
		Host:   net.JoinHostPort(config.Host, strconv.Itoa(int(config.Port))),
		Path:   "/" + config.Database,
	}
	query := u.Query()
	query.Set("sslmode", "disable")
	u.RawQuery = query.Encode()
	return u.String()
}

func applyLocalPasswordProfileGrants(t *testing.T, ctx context.Context, owner, runtime *pgxpool.Pool) {
	t.Helper()
	var ownerRole string
	if err := owner.QueryRow(ctx, "SELECT current_user").Scan(&ownerRole); err != nil {
		t.Fatal("profile fixture owner role query failed")
	}
	config := owner.Config().ConnConfig
	cmd := exec.CommandContext(ctx, pokerTestPSQL(), "-X", "-q", "-h", config.Host, "-p", strconv.Itoa(int(config.Port)), "-U", config.User, "-d", config.Database,
		"-v", "schema_owner="+ownerRole, "-v", "runtime_role="+runtime.Config().ConnConfig.User, "-v", "apply_grants=true",
		"-f", filepath.Join("..", "..", "deploy", "sql", "runtime-grants-0022-poker-access.psql"))
	cmd.Env = append(os.Environ(), "PGPASSWORD="+config.Password)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("profile fixture credential grants failed: %s", output)
	}
}
