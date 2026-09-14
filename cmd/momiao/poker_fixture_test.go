package main

// Opt-in application acceptance: fresh owned PG, all real migrations through
// Store.Migrate, distinct least-privilege roles and actual verified-TLS Redis.
// The regression test labels its synthetic authentication explicitly. Native
// joint tests use the separate real native server, never this callback.
import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/poker"
	"github.com/cy4268/momiao/internal/poker/connectticket"
	"github.com/cy4268/momiao/internal/poker/redislease"
	pt "github.com/cy4268/momiao/internal/poker/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type pokerAppFixture struct {
	t                                             *testing.T
	ctx                                           context.Context
	dir, database, ownerRole, authRole, pokerRole string
	authDSN, pokerDSN, redisFile                  string
	owner, auth, domain                           *pgxpool.Pool
	redis                                         *redis.Client
	keys                                          map[string]bool
}

func appRandom(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func appPrivateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dir, 0700); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	const script = `$ErrorActionPreference='Stop'
$sid=[Security.Principal.WindowsIdentity]::GetCurrent().User
$acl=New-Object Security.AccessControl.DirectorySecurity
$acl.SetAccessRuleProtection($true,$false)
$rule=New-Object Security.AccessControl.FileSystemAccessRule($sid,'FullControl','ContainerInherit,ObjectInherit','None','Allow')
$acl.AddAccessRule($rule)
[IO.Directory]::SetAccessControl($env:POKER_APP_ACL_PATH,$acl)
$verified=[IO.Directory]::GetAccessControl($env:POKER_APP_ACL_PATH)
$rules=$verified.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier])
if (-not $verified.AreAccessRulesProtected -or $rules.Count -ne 1 -or $rules[0].IdentityReference.Value -ne $sid.Value -or $rules[0].AccessControlType -ne 'Allow' -or $rules[0].FileSystemRights -ne 'FullControl' -or $rules[0].InheritanceFlags -ne 'ContainerInherit,ObjectInherit' -or $rules[0].IsInherited) { throw 'private ACL verification failed' }`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Env = append(os.Environ(), "POKER_APP_ACL_PATH="+dir)
	if _, err := cmd.CombinedOutput(); err != nil {
		t.Fatal("private fixture ACL setup failed before writing credentials")
	}
	return dir
}

func (f *pokerAppFixture) write(name string, b []byte) string {
	f.t.Helper()
	path := filepath.Join(f.dir, name)
	if err := os.WriteFile(path, b, 0600); err != nil {
		f.t.Fatal("private fixture write failed")
	}
	return path
}

func newPokerAppFixture(t *testing.T, users ...int64) *pokerAppFixture {
	t.Helper()
	file := os.Getenv("POKER_APP_TEST_CONNECTION_FILE")
	if file == "" {
		t.Skip("set POKER_APP_TEST_CONNECTION_FILE for owned fresh application acceptance")
	}
	if os.Getenv("POKER_APP_TEST_CONFIRM") != "owned-local-fixture" {
		t.Fatal("explicit owned-local-fixture confirmation required")
	}
	var c struct {
		Host, User, Password, Database string
		Port                           int
	}
	b, err := os.ReadFile(file)
	if err != nil || json.Unmarshal(b, &c) != nil || c.Host != "127.0.0.1" || c.Port != 55432 {
		t.Fatal("guarded local PG fixture configuration required")
	}
	ctx := context.Background()
	f := &pokerAppFixture{t: t, ctx: ctx, dir: appPrivateDir(t), keys: map[string]bool{}}
	suffix := strconv.Itoa(os.Getpid()) + "_" + appRandom(t, 5)
	f.database, f.ownerRole, f.authRole, f.pokerRole = "g2_papp_test_"+suffix, "g2_papp_owner_"+suffix, "g2_papp_auth_"+suffix, "g2_papp_poker_"+suffix
	dsn := func(db, user, pass, role string) string {
		u := &url.URL{Scheme: "postgres", Host: "127.0.0.1:55432", Path: "/" + db, User: url.UserPassword(user, pass)}
		q := u.Query()
		q.Set("sslmode", "disable")
		if role != "" {
			q.Set("options", "-c role="+role)
		}
		u.RawQuery = q.Encode()
		return u.String()
	}
	admin, err := pgxpool.New(ctx, dsn(c.Database, c.User, c.Password, ""))
	if err != nil || admin.Ping(ctx) != nil {
		t.Fatal("owned PG fixture unavailable")
	}
	createdDB := false
	var createdRoles []string
	t.Cleanup(func() {
		if f.domain != nil {
			f.domain.Close()
		}
		if f.auth != nil {
			f.auth.Close()
		}
		if f.owner != nil {
			f.owner.Close()
		}
		clean, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if createdDB {
			var owned bool
			if err := admin.QueryRow(clean, "SELECT pg_get_userbyid(datdba)=$2 FROM pg_database WHERE datname=$1", f.database, f.ownerRole).Scan(&owned); err != nil || !owned || !strings.HasPrefix(f.database, "g2_papp_test_") {
				t.Error("owned database cleanup scope verification failed")
				admin.Close()
				return
			}
			if _, err := admin.Exec(clean, "DROP DATABASE "+pgx.Identifier{f.database}.Sanitize()+" WITH (FORCE)"); err != nil {
				t.Error("owned database cleanup failed")
			}
		}
		for i := len(createdRoles) - 1; i >= 0; i-- {
			if _, err := admin.Exec(clean, "DROP ROLE "+pgx.Identifier{createdRoles[i]}.Sanitize()); err != nil {
				t.Error("owned role cleanup failed")
			}
		}
		var remaining int
		if err := admin.QueryRow(clean, "SELECT (SELECT count(*) FROM pg_database WHERE datname=$1)+(SELECT count(*) FROM pg_roles WHERE rolname=ANY($2))", f.database, createdRoles).Scan(&remaining); err != nil || remaining != 0 {
			t.Error("owned PG object absence unverified")
		} else {
			t.Log("APP_FIXTURE_PG_CLEANED database and three roles absent")
		}
		admin.Close()
	})
	authPass, pokerPass := appRandom(t, 24), appRandom(t, 24)
	for _, r := range []struct{ name, pass string }{{f.ownerRole, ""}, {f.authRole, authPass}, {f.pokerRole, pokerPass}} {
		attrs := " NOLOGIN"
		if r.pass != "" {
			attrs = " LOGIN PASSWORD '" + r.pass + "'"
		}
		if _, err = admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{r.name}.Sanitize()+attrs+" NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS"); err != nil {
			t.Fatal("owned role creation failed")
		}
		createdRoles = append(createdRoles, r.name)
	}
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{f.database}.Sanitize()+" OWNER "+pgx.Identifier{f.ownerRole}.Sanitize()); err != nil {
		t.Fatal("owned database creation failed")
	}
	createdDB = true
	ownerDSN := dsn(f.database, c.User, c.Password, f.ownerRole)
	f.owner, err = pgxpool.New(ctx, ownerDSN)
	if err != nil {
		t.Fatal("owner fixture connection failed")
	}
	var absent bool
	if err = f.owner.QueryRow(ctx, "SELECT to_regclass('platform_meta.schema_migrations') IS NULL").Scan(&absent); err != nil || !absent {
		t.Fatal("fresh database registry already exists")
	}
	store, err := platform.Open(ctx, ownerDSN)
	if err != nil {
		t.Fatal("actual migrator owner connection failed")
	}
	err = store.Migrate(ctx)
	store.Close()
	if err != nil {
		t.Fatal("actual contiguous Migrate failed", err)
	}
	files, err := filepath.Glob(filepath.Join("..", "..", "internal", "platform", "migrations", "*.sql"))
	if err != nil || len(files) != 21 {
		t.Fatal("expected actual twenty-one-file migration manifest")
	}
	var count int
	if err = f.owner.QueryRow(ctx, "SELECT count(*) FROM platform_meta.schema_migrations").Scan(&count); err != nil || count != 21 {
		t.Fatal("migration registry count mismatch")
	}
	for i, path := range files {
		if !strings.HasPrefix(filepath.Base(path), fmt.Sprintf("%04d_", i+1)) {
			t.Fatal("migration file sequence is not exactly 0001-0021")
		}
		body, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		digest := fmt.Sprintf("%x", sha256.Sum256(body))
		var actual string
		if err = f.owner.QueryRow(ctx, "SELECT checksum FROM platform_meta.schema_migrations WHERE version=$1", i+1).Scan(&actual); err != nil || actual != digest {
			t.Fatal("actual migration registry hash mismatch", i+1)
		}
		t.Logf("APP_MIGRATION version=%04d file=%s sha256=%s", i+1, filepath.Base(path), actual)
	}
	cmd := exec.Command("psql", "-X", "-q", "-h", c.Host, "-p", strconv.Itoa(c.Port), "-U", c.User, "-d", f.database, "-v", "schema_owner="+f.ownerRole, "-v", "platform_runtime_role="+f.authRole, "-v", "poker_runtime_role="+f.pokerRole, "-v", "apply_grants=true", "-f", filepath.Join("..", "..", "deploy", "sql", "runtime-grants-poker-application.psql"))
	cmd.Env = append(os.Environ(), "PGPASSWORD="+c.Password)
	if output, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("actual aggregate grants failed: %s", output)
	}
	forward := filepath.Join("..", "..", "deploy", "sql", "runtime-grants-0019-0020-poker-lobby.psql")
	body, err := os.ReadFile(forward)
	if err != nil {
		t.Fatal("forward grants source missing")
	}
	cmd = exec.Command("psql", "-X", "-q", "-h", c.Host, "-p", strconv.Itoa(c.Port), "-U", c.User, "-d", f.database, "-v", "schema_owner="+f.ownerRole, "-v", "runtime_role="+f.pokerRole, "-v", "apply_grants=true", "-f", forward)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+c.Password)
	if output, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("actual forward lobby grants failed: %s", output)
	}
	t.Logf("APP_FORWARD_GRANTS original_aggregate_then=%s sha256=%x actual_exit=0", filepath.Base(forward), sha256.Sum256(body))
	historySelects := func(role string) int {
		var n int
		if e := f.owner.QueryRow(ctx, `SELECT count(*) FROM pg_catalog.pg_class c WHERE c.oid IN('games.history_display_snapshots'::regclass,'games.history_index'::regclass,'games.history_ingestion_cursors'::regclass,'games.history_source_rows'::regclass) AND pg_catalog.has_table_privilege($1,c.oid,'SELECT')`, role).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	prePlatform, prePoker := historySelects(f.authRole), historySelects(f.pokerRole)
	t.Logf("APP_HISTORY_PRECONVERGENCE platform_select_relations=%d poker_select_relations=%d runtime_pools_open=false", prePlatform, prePoker)
	if prePlatform != 4 || prePoker != 0 {
		t.Fatal("old aggregate History leakage baseline changed")
	}
	historyForward := filepath.Join("..", "..", "deploy", "sql", "runtime-grants-0021-application-history.psql")
	historyBody, err := os.ReadFile(historyForward)
	if err != nil {
		t.Fatal("History convergence source missing")
	}
	for _, test := range []struct {
		name, apply, role, database string
		ok                          bool
		selects                     int
	}{{"dry-run", "false", f.authRole, f.database, true, 4}, {"wrong-database", "true", f.authRole, f.database + "_wrong", false, 4}, {"owner-as-runtime", "true", f.ownerRole, f.database, false, 4}, {"apply", "true", f.authRole, f.database, true, 0}, {"idempotent", "true", f.authRole, f.database, true, 0}} {
		cmd = exec.Command("psql", "-X", "-q", "-h", c.Host, "-p", strconv.Itoa(c.Port), "-U", c.User, "-d", f.database, "-v", "schema_owner="+f.ownerRole, "-v", "platform_runtime_role="+test.role, "-v", "poker_runtime_role="+f.pokerRole, "-v", "database_name="+test.database, "-v", "apply_grants="+test.apply, "-f", historyForward)
		cmd.Env = append(os.Environ(), "PGPASSWORD="+c.Password)
		output, e := cmd.CombinedOutput()
		exitCode := 0
		if e != nil {
			var exited *exec.ExitError
			if !errors.As(e, &exited) {
				t.Fatal("History convergence psql launch failed")
			}
			exitCode = exited.ExitCode()
		}
		afterPlatform, afterPoker := historySelects(f.authRole), historySelects(f.pokerRole)
		t.Logf("APP_HISTORY_FORWARD case=%s sha256=%x actual_exit=%d platform_select_relations=%d poker_select_relations=%d", test.name, sha256.Sum256(historyBody), exitCode, afterPlatform, afterPoker)
		if (e == nil) != test.ok || afterPlatform != test.selects || afterPoker != 0 {
			t.Fatalf("History convergence gate failed: %s output=%s", test.name, output)
		}
	}
	for _, role := range []string{f.authRole, f.pokerRole} {
		if n := historySelects(role); n != 0 {
			t.Fatalf("application History SELECT must be absent: role=%s effective_relations=%d", role, n)
		}
	}
	f.authDSN = f.write("auth.dsn", []byte(dsn(f.database, f.authRole, authPass, "")))
	f.pokerDSN = f.write("poker.dsn", []byte(dsn(f.database, f.pokerRole, pokerPass, "")))
	f.auth, err = openPokerPool(ctx, f.authDSN)
	if err != nil {
		t.Fatal("real platform runtime connection failed")
	}
	f.domain, err = openPokerPool(ctx, f.pokerDSN)
	if err != nil {
		t.Fatal("real poker runtime connection failed")
	}
	if err = validatePokerPools(ctx, f.auth, f.domain); err != nil {
		t.Fatal("real runtime authority validation failed", err)
	}
	for role, pool := range map[string]*pgxpool.Pool{f.authRole: f.auth, f.pokerRole: f.domain} {
		var denied bool
		if err = pool.QueryRow(ctx, `SELECT current_user=$1 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='games' AND c.relname IN('history_display_snapshots','history_index','history_ingestion_cursors','history_source_rows') AND (pg_catalog.has_any_column_privilege(current_user,c.oid,'SELECT,INSERT,UPDATE,REFERENCES') OR pg_catalog.has_table_privilege(current_user,c.oid,'SELECT,INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER,MAINTAIN'))) AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='games' AND p.proname IN('history_capture_display','history_ingest_batch','history_list') AND pg_catalog.has_function_privilege(current_user,p.oid,'EXECUTE'))`, role).Scan(&denied); err != nil || !denied {
			t.Fatal("actual application role retains effective History authority", role, err)
		}
		for i, sql := range []string{"SELECT * FROM games.history_display_snapshots WHERE false", "SELECT * FROM games.history_index WHERE false", "SELECT * FROM games.history_ingestion_cursors WHERE false", "SELECT * FROM games.history_source_rows WHERE false", "SELECT games.history_list(910001,'{}'::jsonb)", "SELECT games.history_ingest_batch('POKER_SESSION',1)"} {
			_, e := pool.Exec(ctx, sql)
			var pgErr *pgconn.PgError
			if !errors.As(e, &pgErr) || pgErr.Code != "42501" {
				t.Fatal("real application History denial missing", role, i, e)
			}
			t.Logf("APP_HISTORY_DENIED role=%s statement=%d SQLSTATE=%s effective_table_column_function_rights=false", role, i, pgErr.Code)
		}
	}
	var gamesPreserved bool
	if err = f.auth.QueryRow(ctx, `SELECT has_table_privilege(current_user,'games.game_rounds','SELECT') AND has_table_privilege(current_user,'games.game_rounds','INSERT') AND has_column_privilege(current_user,'games.fairness_nonce_cursors','next_nonce','UPDATE')`).Scan(&gamesPreserved); err != nil || !gamesPreserved {
		t.Fatal("existing platform Games positive grants changed", err)
	}
	if _, err = f.auth.Exec(ctx, "SELECT game_slug FROM games.game_registry WHERE false"); err != nil {
		t.Fatal("actual platform Games read failed", err)
	}
	for _, table := range []string{"history_display_snapshots", "history_index", "history_ingestion_cursors", "history_source_rows"} {
		if _, err = f.owner.Exec(ctx, "SELECT * FROM games."+table+" WHERE false"); err != nil {
			t.Fatal("owner History read failed", err)
		}
	}
	t.Log("APP_HISTORY_POSITIVE owner_four_relations_read=true owner_four_relations_three_functions_ownership_checked=true platform_games_select_insert_nonce_update=true no_history_roles_created=true")
	var narrow bool
	if err = f.domain.QueryRow(ctx, `SELECT has_function_privilege(current_user,'ops.is_maintenance_scope_active(text)','EXECUTE') AND has_function_privilege(current_user,'ops.lock_poker_admission_scopes()','EXECUTE') AND has_function_privilege(current_user,'economy.poker_lobby_balance(bigint)','EXECUTE') AND NOT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname IN('economy','ops') AND c.relkind IN('r','p','v','m') AND has_table_privilege(current_user,c.oid,'SELECT,INSERT,UPDATE,DELETE,TRUNCATE'))`).Scan(&narrow); err != nil || !narrow {
		t.Fatal("three lobby function grants or direct Economy/Maintenance denial failed", err)
	}
	t.Log("APP_LOBBY_GRANTS three_function_execute=true economy_and_ops_direct_table_read_write=false")
	if _, err = f.domain.Exec(ctx, "UPDATE economy.wallet_balances SET balance_units=balance_units WHERE false"); err == nil {
		t.Fatal("poker runtime has direct wallet authority")
	}
	t.Logf("APP_RUNTIME database=%s owner=%s platform=%s poker=%s distinct_nonowner_login=true direct_economy_denied=true three_gateways=true", f.database, f.ownerRole, f.authRole, f.pokerRole)
	for _, user := range users {
		for _, stmt := range []string{"INSERT INTO identity.account_refs(newapi_user_id) VALUES($1)", "INSERT INTO identity.master_profiles(newapi_user_id,display_name,normalized_name) VALUES($1,'Fixture '||$1::bigint::text,'fixture '||$1::bigint::text)", "INSERT INTO economy.wallet_balances(newapi_user_id,asset_type,balance_units) VALUES($1,'AVAILABLE_CHIPS',1500000000)"} {
			if _, err = f.owner.Exec(ctx, stmt, user); err != nil {
				t.Fatal("owned synthetic platform identity/funding seed failed", err)
			}
		}
	}
	f.setupRedis()
	return f
}

func (f *pokerAppFixture) setupRedis() {
	t := f.t
	dir := os.Getenv("POKER_APP_TEST_REDIS_FIXTURE_DIR")
	if !filepath.IsAbs(dir) {
		t.Fatal("absolute owned Redis fixture directory required")
	}
	secret, err := os.ReadFile(filepath.Join(dir, "admin-secret.txt"))
	if err != nil {
		t.Fatal("owned Redis credential unavailable")
	}
	f.redis = redis.NewClient(&redis.Options{Addr: "127.0.0.1:16379", Username: "g2-admin", Password: string(secret), Protocol: 2, DisableIdentity: true, MaxRetries: -1, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second})
	if err = f.redis.Ping(f.ctx).Err(); err != nil {
		t.Fatal("owned Redis unavailable")
	}
	var users []string
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		clean := true
		var keys []string
		for key := range f.keys {
			keys = append(keys, key)
		}
		if len(keys) > 0 {
			// Shared fixture admin intentionally has no broad control/ticket key
			// access. Provision one ephemeral cleanup identity for exact owned keys.
			user, password := "g2-app-clean-"+appRandom(t, 8), appRandom(t, 24)
			hash := sha256.Sum256([]byte(password))
			args := []any{"ACL", "SETUSER", user, "reset", "on", "#" + hex.EncodeToString(hash[:]), "+hello", "+del", "+exists"}
			for _, key := range keys {
				args = append(args, "~"+key)
			}
			if err := f.redis.Do(ctx, args...).Err(); err != nil {
				t.Error("exact cleanup ACL creation failed")
				clean = false
			} else {
				users = append(users, user)
				cleanup := redis.NewClient(&redis.Options{Addr: "127.0.0.1:16379", Username: user, Password: password, Protocol: 2, DisableIdentity: true, MaxRetries: -1, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second})
				if err := cleanup.Del(ctx, keys...).Err(); err != nil {
					t.Error("exact owned Redis keys cleanup failed")
					clean = false
				}
				if n, err := cleanup.Exists(ctx, keys...).Result(); err != nil || n != 0 {
					t.Error("exact owned Redis keys absence unverified")
					clean = false
				}
				cleanup.Close()
			}
		}
		for _, user := range users {
			if err := f.redis.Do(ctx, "ACL", "DELUSER", user).Err(); err != nil {
				t.Error("owned Redis ACL cleanup failed")
				clean = false
			}
			if value, err := f.redis.Do(ctx, "ACL", "GETUSER", user).Result(); err != redis.Nil && (err != nil || value != nil) {
				t.Error("owned Redis ACL absence unverified")
				clean = false
			}
		}
		f.redis.Close()
		if clean {
			t.Logf("APP_FIXTURE_REDIS_CLEANED exact tracked keys=%d and own ACL users=%d verified absent; shared server retained", len(keys), len(users))
		}
	})
	leaseUser, ticketUser := "g2-app-lease-"+appRandom(t, 8), "g2-app-ticket-"+appRandom(t, 8)
	leasePass, ticketPass := appRandom(t, 24), appRandom(t, 24)
	for _, v := range []struct {
		user, pass string
		rules      []string
	}{{leaseUser, leasePass, []string{"+ping", "+hello", "+eval", "+time", "+get", "+pttl", "+set", "+del", "~" + redislease.KeyPrefix + "*", "~" + redislease.ControlKeyPrefix + "*"}}, {ticketUser, ticketPass, []string{"+ping", "+hello", "+set", "~" + connectticket.RedisKeyPrefix + "*"}}} {
		hash := sha256.Sum256([]byte(v.pass))
		args := []any{"ACL", "SETUSER", v.user, "reset", "on", "#" + hex.EncodeToString(hash[:])}
		for _, r := range v.rules {
			args = append(args, r)
		}
		if err = f.redis.Do(f.ctx, args...).Err(); err != nil {
			t.Fatal("isolated Redis runtime ACL creation failed")
		}
		users = append(users, v.user)
	}
	b, err := json.Marshal(map[string]string{"addr": "127.0.0.1:16380", "username": leaseUser, "password": leasePass, "ticket_username": ticketUser, "ticket_password": ticketPass, "ca_file": filepath.Join(dir, "server.crt"), "server_name": "localhost"})
	if err != nil {
		t.Fatal(err)
	}
	f.redisFile = f.write("redis.json", b)
	t.Log("APP_REDIS verified TLS endpoint=127.0.0.1:16380 server_name=localhost separate lease/control and ticket ACL users; family-scoped keys, exact owned cleanup")
}

func (f *pokerAppFixture) track(r poker.Receipt) {
	if r.TableID != "" {
		for seat := 1; seat <= 9; seat++ {
			f.keys[redislease.KeyPrefix+r.TableID+":"+strconv.Itoa(seat)] = true
		}
	}
	if r.SessionID != "" {
		f.keys[redislease.ControlKeyPrefix+r.SessionID] = true
	}
}

func appHTTP(t *testing.T, h http.Handler, method, path string, body any, headers http.Header) (int, json.RawMessage, string) {
	t.Helper()
	var b []byte
	var err error
	if body != nil {
		b, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	r.Header.Set("Origin", "http://127.0.0.1:32000")
	r.Header.Set("Content-Type", "application/json")
	for k, vs := range headers {
		r.Header[k] = vs
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Code    string          `json:"code"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal("application response envelope malformed")
	}
	return w.Code, envelope.Data, envelope.Code
}

func appReceipt(t *testing.T, f *pokerAppFixture, h http.Handler, path string, body any, headers http.Header) poker.Receipt {
	t.Helper()
	status, data, code := appHTTP(t, h, http.MethodPost, path, body, headers)
	if status != 200 {
		t.Fatalf("HTTP %s status=%d code=%s", path, status, code)
	}
	var receipt poker.Receipt
	if json.Unmarshal(data, &receipt) != nil {
		t.Fatal("receipt malformed")
	}
	f.track(receipt)
	return receipt
}

func TestPokerApplicationOldSessionURL(t *testing.T) {
	f := newPokerAppFixture(t, 910001)
	lease, _, err := readPokerRedisConfig(f.redisFile)
	if err != nil {
		t.Fatal(err)
	}
	principal := pt.Principal{UserID: 910001, SessionIDHash: strings.Repeat("a", 64), SessionVersion: 1, SecurityEpoch: 1}
	validate := func(context.Context, pt.Principal) error { return nil }
	s, err := poker.NewWithRedis(f.ctx, poker.Options{Pool: f.domain, Keyring: poker.Keyring{Current: "test", Keys: map[string][]byte{"test": make([]byte, 32)}}, ValidateSession: func(context.Context, poker.AuthSession) error { return nil }}, lease)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := &pokerAdapter{service: s}
	h, err := pt.New(pt.Options{Origin: "http://127.0.0.1:32000", AuthHTTP: func(*http.Request) (pt.Principal, error) { return principal, nil }, AuthenticateTicket: func(context.Context, pt.ConnectRequest) (pt.Principal, error) { return principal, nil }, ValidateSession: validate, Snapshot: a.snapshot, Connected: a.connected, Disconnected: a.disconnected, AuthorizeControl: a.authorize, Ports: a.ports()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	t.Log("AUTHENTICATION=SYNTHETIC_CALLBACK; real application adapter + fresh 0001-0020 PG + verified-TLS Redis; not native joint proof")
	created := appReceipt(t, f, h, "/api/v1/poker/tables", map[string]any{"request_id": "app-create-old-session-url", "name": "Old session URL", "blind_preset": "5-10", "max_seats": 2, "allow_spectators": true}, nil)
	for _, operation := range []string{"top-ups", "safe-leave"} {
		t.Run(operation, func(t *testing.T) {
			buy := func(key string) poker.Receipt {
				r := appReceipt(t, f, h, "/api/v1/poker/tables/"+created.TableID+"/seat-reservations", map[string]any{"request_id": "reserve-" + key, "seat_no": 1}, nil)
				return appReceipt(t, f, h, "/api/v1/poker/tables/"+created.TableID+"/buy-ins", map[string]any{"request_id": "app-buy-" + key, "reservation_id": r.ReservationID, "amount_units": "200000000"}, nil)
			}
			old := buy("old-" + operation)
			appReceipt(t, f, h, "/api/v1/poker/sessions/"+old.SessionID+"/safe-leave", map[string]any{"request_id": "leave-old-" + operation}, nil)
			current := buy("current-" + operation)
			if current.SessionID == old.SessionID {
				t.Fatal("replacement session not created")
			}
			var before string
			if err := f.owner.QueryRow(f.ctx, "SELECT row_to_json(s)::text FROM poker.sessions s WHERE session_id=$1", current.SessionID).Scan(&before); err != nil {
				t.Fatal(err)
			}
			body := map[string]any{"request_id": "stale-url-" + operation}
			if operation == "top-ups" {
				body["amount_units"] = "5000000"
			}
			status, _, code := appHTTP(t, h, http.MethodPost, "/api/v1/poker/sessions/"+old.SessionID+"/"+operation, body, nil)
			var after string
			if err := f.owner.QueryRow(f.ctx, "SELECT row_to_json(s)::text FROM poker.sessions s WHERE session_id=$1", current.SessionID).Scan(&after); err != nil {
				t.Fatal(err)
			}
			t.Logf("OLD_SESSION_URL operation=%s status=%d code=%s new_session_unchanged=%t", operation, status, code, before == after)
			if status != 403 || code != "POKER_COMMAND_DENIED" || before != after {
				t.Errorf("historical session URL mutated or admitted replacement session: status=%d unchanged=%t", status, before == after)
			}
			// In the RED case stale safe-leave already settled the new session.
			if operation != "safe-leave" || status != 200 {
				appReceipt(t, f, h, "/api/v1/poker/sessions/"+current.SessionID+"/safe-leave", map[string]any{"request_id": "cleanup-current-" + operation}, nil)
			}
		})
	}
}
