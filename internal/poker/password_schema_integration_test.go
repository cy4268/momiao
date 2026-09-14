package poker

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/testutil/pokerhistoryfixture"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPasswordSchemaForwardPrivileges(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "platform", "migrations", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	hashes := make(map[string][32]byte)
	for i, file := range files {
		if !strings.HasPrefix(filepath.Base(file), fmt.Sprintf("%04d_", i+1)) {
			t.Fatal("noncontiguous actual SQL inputs")
		}
		body, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		hashes[file] = sha256.Sum256(body)
		t.Logf("module_sql_input %s sha256=%x", filepath.Base(file), hashes[file])
	}
	f := pokerhistoryfixture.Open(t) // Fresh registry and all seven grants precede runtime pools.
	owner, runtime := f.Pool, f.Poker
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	t.Logf("password_schema_fixture database=%s runtime=%s", runtime.Config().ConnConfig.Database, runtime.Config().ConnConfig.User)
	var exists bool
	if err = owner.QueryRow(ctx, "SELECT to_regclass('poker.table_access_credentials') IS NOT NULL").Scan(&exists); err != nil || !exists {
		t.Fatal("real PG has no password credential storage")
	}
	if len(files) != 24 {
		t.Fatal("expected actual contiguous 0001 through 0024 SQL inputs")
	}
	for _, file := range files {
		body, e := os.ReadFile(file)
		if e != nil || sha256.Sum256(body) != hashes[file] {
			t.Fatal("executed SQL source drift")
		}
	}
	run := func(pool *pgxpool.Pool, sql, state string, args ...any) {
		t.Helper()
		_, e := pool.Exec(ctx, sql, args...)
		passwordSQLState(t, e, state)
	}
	var initial bool
	passwordSQLState(t, owner.QueryRow(ctx, `SELECT has_table_privilege($1,'poker.table_access_credentials','SELECT')
      AND has_table_privilege($1,'poker.table_access_credentials','INSERT')
      AND NOT has_table_privilege($1,'poker.table_access_credentials','UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')
      AND NOT has_any_column_privilege($1,'poker.table_access_credentials','UPDATE,REFERENCES')
      AND NOT EXISTS(SELECT 1 FROM unnest($2::text[]) r WHERE has_any_column_privilege(r,'poker.table_access_credentials','SELECT,INSERT,UPDATE,REFERENCES')
        OR has_table_privilege(r,'poker.table_access_credentials','DELETE,TRUNCATE,TRIGGER'))`, f.PokerRole, []string{f.PlatformRole, f.ReaderRole, f.WorkerRole}).Scan(&initial), "")
	if !initial {
		t.Fatal("current24 initial PHC profile differs from completed seven-grant fixture")
	}
	run(owner, "REVOKE SELECT,INSERT ON poker.table_access_credentials FROM "+pgx.Identifier{f.PokerRole}.Sanitize(), "")
	run(runtime, "SELECT password_phc FROM poker.table_access_credentials", "42501")
	t.Log("verified current24 PHC profile, then explicitly revoked fixture-local SELECT/INSERT for original convergence cases")
	adminConfig := owner.Config().ConnConfig.Copy()
	if adminConfig.RuntimeParams["options"] != "-c role="+f.OwnerRole {
		t.Fatal("fixture owner startup role missing")
	}
	passwordSQLState(t, owner.QueryRow(ctx, "SELECT session_user=$1 AND current_user=$2 AND current_database()=$3", adminConfig.User, f.OwnerRole, f.Database).Scan(&initial), "")
	if !initial {
		t.Fatal("fixture owner session identity mismatch")
	}
	delete(adminConfig.RuntimeParams, "options")
	delete(adminConfig.RuntimeParams, "role")
	admin, err := pgx.ConnectConfig(ctx, adminConfig) // Test-owned admin, used only for outsider role lifecycle.
	if err != nil {
		t.Fatal("isolated fixture admin connection")
	}
	var others []*pgxpool.Pool
	var roles []string
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		defer admin.Close(cleanup)
		for _, p := range others {
			p.Close()
		}
		for _, role := range roles {
			if _, e := admin.Exec(cleanup, "DROP OWNED BY "+pgx.Identifier{role}.Sanitize()+"; DROP ROLE "+pgx.Identifier{role}.Sanitize()); e != nil {
				t.Errorf("isolated password role cleanup failed: %T", e)
			}
		}
		var remaining int
		passwordSQLState(t, admin.QueryRow(cleanup, "SELECT count(*) FROM pg_roles WHERE rolname=ANY($1)", roles).Scan(&remaining), "")
		if remaining != 0 {
			t.Error("registered password outsider roles remain")
		}
	})
	passwordSQLState(t, admin.QueryRow(ctx, "SELECT session_user=current_user AND current_user=$1 AND current_database()=$2", adminConfig.User, f.Database).Scan(&initial), "")
	if !initial {
		t.Fatal("fixture admin retained owner role or changed session identity")
	}
	for _, kind := range []string{"ops", "platform"} {
		cfg := runtime.Config().Copy()
		cfg.ConnConfig.User += "_" + kind
		cfg.ConnConfig.Password = uuid()
		role := cfg.ConnConfig.User
		_, err = admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{role}.Sanitize()+" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD '"+cfg.ConnConfig.Password+"'")
		passwordSQLState(t, err, "")
		roles = append(roles, role)
		t.Logf("PASSWORD_OUTSIDER_CREATED %s", role)
		p, e := pgxpool.NewWithConfig(ctx, cfg)
		if e != nil {
			t.Fatal("isolated other runtime pool")
		}
		others = append(others, p)
		passwordSQLState(t, p.Ping(ctx), "")
	}
	_, err = admin.Exec(ctx, "GRANT "+pgx.Identifier{runtime.Config().ConnConfig.User}.Sanitize()+" TO "+pgx.Identifier{roles[0]}.Sanitize())
	passwordSQLState(t, err, "")
	passwordApplyGrants(t, ctx, owner, runtime, true)
	_, err = admin.Exec(ctx, "REVOKE "+pgx.Identifier{runtime.Config().ConnConfig.User}.Sanitize()+" FROM "+pgx.Identifier{roles[0]}.Sanitize())
	passwordSQLState(t, err, "")
	t.Log("grant installer rejects another runtime inheriting the necessary Poker role")
	// Deliberate local ACL drift, not a claim about the old enumerated grant script.
	var publicUsage bool
	passwordSQLState(t, owner.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_namespace n CROSS JOIN LATERAL aclexplode(n.nspacl) a
      WHERE n.nspname='poker' AND a.grantee=0 AND a.privilege_type='USAGE')`).Scan(&publicUsage), "")
	run(owner, "GRANT USAGE ON SCHEMA poker TO PUBLIC", "")
	grantees := "PUBLIC," + pgx.Identifier{runtime.Config().ConnConfig.User}.Sanitize()
	for _, role := range roles {
		grantees += "," + pgx.Identifier{role}.Sanitize()
	}
	run(owner, "GRANT ALL ON poker.table_access_credentials TO "+grantees, "")
	run(owner, "GRANT SELECT(password_phc),UPDATE(password_phc) ON poker.table_access_credentials TO "+pgx.Identifier{roles[0]}.Sanitize(), "")
	for _, p := range others {
		run(p, "SELECT password_phc FROM poker.table_access_credentials", "")
	}
	passwordApplyGrants(t, ctx, owner, runtime, false)
	for _, p := range append(others, runtime) {
		for _, sql := range []string{"UPDATE poker.table_access_credentials SET password_phc=password_phc", "DELETE FROM poker.table_access_credentials", "TRUNCATE poker.table_access_credentials"} {
			run(p, sql, "42501")
		}
	}
	for _, p := range others {
		run(p, "SELECT password_phc FROM poker.table_access_credentials", "42501")
		run(p, "INSERT INTO poker.table_access_credentials DEFAULT VALUES", "42501")
	}
	// Structurally encoded fixture only; this schema test makes no KDF-strength claim.
	phc := "$argon2id$v=19$m=64,t=1,p=1$AAECAwQFBgcICQoLDA0ODw$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	table := uuid()
	run(owner, "INSERT INTO poker.tables(table_id,owner_newapi_user_id,name,max_seats,blind_preset_version,ruleset_version) VALUES($1,101,'password-schema-fixture',2,'5-10','poker-cash-v1-20260906')", "", table)
	insert := "INSERT INTO poker.table_access_credentials(table_id,password_phc) VALUES($1,$2)"
	run(runtime, insert, "", table, phc)
	var stored string
	if err = runtime.QueryRow(ctx, "SELECT password_phc FROM poker.table_access_credentials WHERE table_id=$1", table).Scan(&stored); err != nil || stored != phc {
		t.Fatal("necessary runtime must read exactly its inserted PHC")
	}
	run(runtime, insert, "23505", table, phc)
	run(runtime, insert, "23503", uuid(), phc)
	run(runtime, insert, "23502", table, nil)
	for _, invalid := range []string{"", strings.Repeat("x", 513)} {
		run(runtime, insert, "23514", table, invalid)
	}
	for _, sql := range []string{"UPDATE poker.table_access_credentials SET password_phc=password_phc", "DELETE FROM poker.table_access_credentials", "TRUNCATE poker.table_access_credentials"} {
		run(owner, sql, "55000")
	}
	if !publicUsage {
		run(owner, "REVOKE USAGE ON SCHEMA poker FROM PUBLIC", "")
	}
	t.Log("real PG credentials, immutable history, required SELECT/INSERT and revocation of table/column/PUBLIC drift verified")
}

func passwordSQLState(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil && want == "" {
		return
	}
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.Code != want {
		t.Fatalf("expected SQLSTATE %q, error type %T; no SQL parameters emitted", want, err)
	}
}

func passwordApplyGrants(t *testing.T, ctx context.Context, owner, runtime *pgxpool.Pool, denied bool) {
	t.Helper()
	var role string
	passwordSQLState(t, owner.QueryRow(ctx, "SELECT current_user").Scan(&role), "")
	c := owner.Config().ConnConfig
	cmd := exec.CommandContext(ctx, "psql", "-X", "-q", "-h", c.Host, "-p", strconv.Itoa(int(c.Port)), "-U", c.User, "-d", c.Database, "-v", "schema_owner="+role, "-v", "runtime_role="+runtime.Config().ConnConfig.User, "-v", "apply_grants=true", "-f", filepath.Join("..", "..", "deploy", "sql", "runtime-grants-0022-poker-access.psql"))
	cmd.Env = append(os.Environ(), "PGPASSWORD="+c.Password)
	output, err := cmd.CombinedOutput()
	if (err != nil) != denied || denied && !strings.Contains(string(output), "Poker access role separation failed") {
		t.Fatalf("password forward grants: %s", output)
	}
}
