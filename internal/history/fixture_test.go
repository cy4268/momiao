package history

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type historyFixture struct {
	owner, reader, worker                                 *pgxpool.Pool
	store                                                 *platform.Store
	database, ownerRole, readerRole, workerRole, ownerDSN string
}

func checked(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newHistoryFixture(t *testing.T) *historyFixture {
	t.Helper()
	file := os.Getenv("MOMIAO_HISTORY_TEST_CONNECTION_FILE")
	if file == "" {
		t.Skip("explicit new isolated history fixture required")
	}
	var c struct {
		Host, User, Password string
		Port                 int
	}
	raw, err := os.ReadFile(file)
	checked(t, err)
	checked(t, json.Unmarshal(raw, &c))
	if c.Host != "127.0.0.1" || c.Port != 55432 {
		t.Fatal("refusing nonlocal fixture")
	}
	ctx := context.Background()
	dsn := func(db, user, password, role string) string {
		u := url.URL{Scheme: "postgres", Host: fmt.Sprintf("%s:%d", c.Host, c.Port), Path: "/" + db, User: url.UserPassword(user, password)}
		q := url.Values{"sslmode": {"disable"}}
		if role != "" {
			q.Set("options", "-c role="+role)
		}
		u.RawQuery = q.Encode()
		return u.String()
	}
	admin, err := pgx.Connect(ctx, dsn("postgres", c.User, c.Password, ""))
	if err != nil {
		t.Fatal("isolated PG admin unavailable")
	}
	secret := make([]byte, 24)
	_, err = rand.Read(secret)
	checked(t, err)
	pass := hex.EncodeToString(secret)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	f := &historyFixture{database: "h1a_history_" + suffix, ownerRole: "h1a_owner_" + suffix, readerRole: "h1a_reader_" + suffix, workerRole: "h1a_worker_" + suffix}
	roles := []string{}
	created := false
	t.Cleanup(func() {
		for _, p := range []*pgxpool.Pool{f.owner, f.reader, f.worker} {
			if p != nil {
				p.Close()
			}
		}
		if f.store != nil {
			f.store.Close()
		}
		clean, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		defer admin.Close(clean)
		if created {
			var owned bool
			e := admin.QueryRow(clean, "SELECT pg_get_userbyid(datdba)=$2 FROM pg_database WHERE datname=$1", f.database, f.ownerRole).Scan(&owned)
			if e != nil || !owned || !strings.HasPrefix(f.database, "h1a_history_") {
				t.Error("fixture ownership mismatch")
				return
			}
			if _, e = admin.Exec(clean, "DROP DATABASE "+pgx.Identifier{f.database}.Sanitize()+" WITH (FORCE)"); e != nil {
				t.Error("fixture cleanup failed")
				return
			}
		}
		for _, role := range roles {
			if _, e := admin.Exec(clean, "DROP ROLE "+pgx.Identifier{role}.Sanitize()); e != nil {
				t.Error("fixture role cleanup failed")
			}
		}
	})
	for i, role := range []string{f.ownerRole, f.readerRole, f.workerRole} {
		attrs := " NOLOGIN"
		if i > 0 {
			attrs = " LOGIN PASSWORD '" + pass + "'"
		}
		_, err = admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{role}.Sanitize()+attrs+" NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS")
		if err != nil {
			t.Fatal("new fixture role creation failed")
		}
		roles = append(roles, role)
	}
	_, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{f.database}.Sanitize()+" OWNER "+pgx.Identifier{f.ownerRole}.Sanitize())
	checked(t, err)
	created = true
	f.ownerDSN = dsn(f.database, c.User, c.Password, f.ownerRole)
	private := filepath.Join(filepath.Dir(file), suffix+".dsn")
	checked(t, os.WriteFile(private, []byte(f.ownerDSN), 0600))
	t.Cleanup(func() { os.Remove(private) })
	base := exec.Command(os.Getenv("MOMIAO_HISTORY_BASELINE_MIGRATOR"))
	base.Env = append(os.Environ(), "MOMIAO_MIGRATION_DSN_FILE="+private)
	if out, e := base.CombinedOutput(); e != nil {
		t.Fatalf("real baseline migration failed: %s", out)
	}
	f.owner, err = pgxpool.New(ctx, f.ownerDSN)
	checked(t, err)
	f.store, err = platform.Open(ctx, f.ownerDSN)
	checked(t, err)
	for _, user := range []int64{101, 102, 103} {
		checked(t, f.store.EnsureAccount(ctx, user))
	}
	f.sql(t, `INSERT INTO identity.master_profiles(newapi_user_id,display_name,normalized_name) VALUES(101,'Player A','player a'),(102,'Player B','player b');
INSERT INTO poker.tables(table_id,owner_newapi_user_id,name,max_seats,blind_preset_version,ruleset_version)
VALUES('00000000-0000-4000-8000-000000000001',101,'Original table',3,'5-10','poker-cash-v1-20260906');
INSERT INTO poker.seats(table_id,seat_no) SELECT table_id,n FROM poker.tables CROSS JOIN generate_series(1,3) n;
INSERT INTO poker.sessions(session_id,newapi_user_id,table_id,seat_no,display_name_snapshot,initial_buyin_units,current_stack_units,started_at)
VALUES('00000000-0000-4000-8000-000000000011',101,'00000000-0000-4000-8000-000000000001',1,'Player A',200000000,200000000,'2026-09-01T00:00:00Z');`)
	checked(t, f.store.Migrate(ctx))
	checked(t, f.store.Migrate(ctx)) // Re-read all 21 recorded checksums; no migration replay.
	f.reader, err = pgxpool.New(ctx, dsn(f.database, f.readerRole, pass, ""))
	checked(t, err)
	f.worker, err = pgxpool.New(ctx, dsn(f.database, f.workerRole, pass, ""))
	checked(t, err)
	t.Logf("fresh fixture %s, real migrations, isolated owner/reader/worker", f.database)
	return f
}

func (f *historyFixture) sql(t *testing.T, sql string, args ...any) {
	t.Helper()
	_, err := f.owner.Exec(context.Background(), sql, append([]any{pgx.QueryExecModeSimpleProtocol}, args...)...)
	checked(t, err)
}

func (f *historyFixture) transaction(t *testing.T) pgx.Tx {
	t.Helper()
	tx, err := f.owner.Begin(context.Background())
	checked(t, err)
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

func (f *historyFixture) grants(t *testing.T, wantOK bool) {
	t.Helper()
	u, err := url.Parse(f.ownerDSN)
	checked(t, err)
	password, _ := u.User.Password()
	if strings.ContainsAny(password, "\r\n") {
		t.Fatal("invalid fixture service password")
	}
	service := filepath.Join(filepath.Dir(os.Getenv("MOMIAO_HISTORY_TEST_CONNECTION_FILE")), f.database+".pgservice")
	config := fmt.Sprintf("[h1a_owner]\nhost=%s\nport=%s\ndbname=%s\nuser=%s\npassword=%s\noptions=-c role=%s\n",
		u.Hostname(), u.Port(), f.database, u.User.Username(), password, f.ownerRole)
	checked(t, os.WriteFile(service, []byte(config), 0600))
	defer os.Remove(service)
	cmd := exec.Command(os.Getenv("MOMIAO_HISTORY_PSQL"), "-X", "-d", "service=h1a_owner", "-v", "ON_ERROR_STOP=1", "-v", "apply_grants=true",
		"-v", "schema_owner="+f.ownerRole, "-v", "database_name="+f.database,
		"-v", "history_worker_role="+f.workerRole, "-v", "history_reader_role="+f.readerRole,
		"-f", "../../deploy/sql/runtime-grants-0021-history.psql")
	cmd.Env = append(os.Environ(), "PGSERVICEFILE="+service)
	if out, err := cmd.CombinedOutput(); (err == nil) != wantOK {
		t.Fatalf("history grants success=%v, want=%v: %s", err == nil, wantOK, out)
	}
}
