// Package historyfixture owns disposable localhost databases; imported only by tests.
package historyfixture

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
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
	"github.com/cy4268/momiao/internal/testutil/pokerhistoryfixture"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Fixture struct {
	Owner, Runtime                                                       *platform.Store
	Pool                                                                 *pgxpool.Pool
	Database, OwnerRole, PlatformRole, PokerRole, ReaderRole, WorkerRole string
	manifest                                                             manifest
	service                                                              string
}

type manifest = pokerhistoryfixture.Manifest

func Check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func Open(t *testing.T) *Fixture {
	t.Helper()
	m := pokerhistoryfixture.LoadManifest(t)
	var c struct {
		Host, User, Password string
		Port                 int
	}
	raw, err := os.ReadFile(m.ConnectionFile)
	Check(t, err)
	Check(t, json.Unmarshal(raw, &c))
	if c.Host != "127.0.0.1" || c.Port != 55432 || strings.ContainsAny(c.Password, "\r\n") {
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
	suffix := strings.ToLower(rand.Text())
	f := &Fixture{Database: "root24_detail_" + suffix, OwnerRole: "root24_b1owner_" + suffix, PlatformRole: "root24_b1platform_" + suffix, PokerRole: "root24_b1poker_" + suffix, ReaderRole: "root24_b1reader_" + suffix, WorkerRole: "root24_b1worker_" + suffix, manifest: m}
	var created bool
	roles := []string{}
	t.Cleanup(func() {
		if f.Runtime != nil {
			f.Runtime.Close()
		}
		if f.Owner != nil {
			f.Owner.Close()
		}
		if f.Pool != nil {
			f.Pool.Close()
		}
		clean, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		defer admin.Close(clean)
		if created {
			var owned bool
			e := admin.QueryRow(clean, "SELECT pg_get_userbyid(datdba)=$2 FROM pg_database WHERE datname=$1", f.Database, f.OwnerRole).Scan(&owned)
			if e != nil || !owned || !strings.HasPrefix(f.Database, "root24_detail_") {
				t.Error("fixture ownership mismatch")
				return
			}
			if _, e = admin.Exec(clean, "DROP DATABASE "+pgx.Identifier{f.Database}.Sanitize()+" WITH (FORCE)"); e != nil {
				t.Error("fixture database cleanup failed")
				return
			}
		}
		for _, role := range roles {
			if _, e := admin.Exec(clean, "DROP ROLE "+pgx.Identifier{role}.Sanitize()); e != nil {
				t.Error("fixture role cleanup failed")
			}
		}
		var left int
		Check(t, admin.QueryRow(clean, "SELECT (SELECT count(*) FROM pg_database WHERE datname=$1)+(SELECT count(*) FROM pg_roles WHERE rolname=ANY($2))", f.Database, roles).Scan(&left))
		if left != 0 {
			t.Error("fixture artifacts remain")
		}
		t.Logf("CLEANUP %s: database and five roles absent", f.Database)
	})
	secret := make([]byte, 24)
	_, err = rand.Read(secret)
	Check(t, err)
	password := hex.EncodeToString(secret)
	for i, role := range []string{f.OwnerRole, f.PlatformRole, f.PokerRole, f.ReaderRole, f.WorkerRole} {
		attrs := " NOLOGIN"
		if i > 0 {
			attrs = " LOGIN PASSWORD '" + password + "'"
		}
		_, err = admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{role}.Sanitize()+attrs+" NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS")
		if err != nil {
			t.Fatal("isolated role creation failed")
		}
		roles = append(roles, role)
		t.Logf("ROOT24_ROLE_CREATED %s", role)
	}
	_, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{f.Database}.Sanitize()+" OWNER "+pgx.Identifier{f.OwnerRole}.Sanitize())
	Check(t, err)
	created = true
	t.Logf("ROOT24_DB_CREATED %s owner=%s", f.Database, f.OwnerRole)
	ownerDSN := dsn(f.Database, c.User, c.Password, f.OwnerRole)
	f.Owner, err = platform.Open(ctx, ownerDSN)
	if err != nil {
		t.Fatal("isolated owner unavailable")
	}
	f.Pool, err = pgxpool.New(ctx, ownerDSN)
	Check(t, err)
	var fresh bool
	Check(t, f.Pool.QueryRow(ctx, "SELECT to_regclass('platform_meta.schema_migrations') IS NULL").Scan(&fresh))
	if !fresh {
		t.Fatal("fresh root B1 fixture already has a migration registry")
	}
	Check(t, f.Owner.Migrate(ctx))
	registry := "SELECT md5(jsonb_agg(to_jsonb(m) ORDER BY version)::text) FROM platform_meta.schema_migrations m"
	var first, second string
	Check(t, f.Pool.QueryRow(ctx, registry).Scan(&first))
	Check(t, f.Owner.Migrate(ctx))
	Check(t, f.Pool.QueryRow(ctx, registry).Scan(&second))
	if first != second {
		t.Fatal("second B1 Migrate changed registry checksums or applied_at")
	}
	var count, maximum int
	var prerequisite string
	Check(t, f.Pool.QueryRow(ctx, "SELECT count(*),max(version),max(checksum) FILTER(WHERE version=22) FROM platform_meta.schema_migrations").Scan(&count, &maximum, &prerequisite))
	if count != 24 || maximum != 24 || prerequisite != m.Migrations[21].SHA256 {
		t.Fatal("real contiguous 0001-0024 required")
	}
	paths, err := filepath.Glob(filepath.Join(m.Repo, "internal/platform/migrations/*.sql"))
	Check(t, err)
	for _, path := range paths {
		Check(t, f.Pool.QueryRow(ctx, "SELECT checksum FROM platform_meta.schema_migrations WHERE version=$1::bigint", strings.Split(filepath.Base(path), "_")[0]).Scan(&prerequisite))
		verifyFile(t, path, prerequisite)
		t.Logf("MIGRATION %s registry=%s source=match", filepath.Base(path), prerequisite)
	}
	f.service = filepath.Join(t.TempDir(), "pgservice.conf")
	service := fmt.Sprintf("[h1b_owner]\nhost=127.0.0.1\nport=55432\ndbname=%s\nuser=%s\npassword=%s\noptions=-c role=%s\n", f.Database, c.User, c.Password, f.OwnerRole)
	Check(t, os.WriteFile(f.service, []byte(service), 0600))
	for _, g := range m.Grants {
		f.Grant(t, g.Path, g.Runtime, true)
	}
	f.Runtime, err = platform.Open(ctx, dsn(f.Database, f.PlatformRole, password, ""))
	if err != nil {
		t.Fatal("isolated runtime unavailable")
	}
	for _, user := range []int64{101, 102, 103} {
		Check(t, f.Owner.EnsureAccount(ctx, user))
	}
	f.SQL(t, `INSERT INTO identity.master_profiles(newapi_user_id,display_name,normalized_name) VALUES(101,'Player A','player a'),(102,'Player B','player b')`)
	t.Logf("NEW_FIXTURE %s: 24 checksum-verified migrations, exact approved grant order", f.Database)
	return f
}

func verifyFile(t *testing.T, path, want string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	Check(t, err)
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != strings.ToLower(want) {
		t.Fatalf("prerequisite checksum mismatch: %s", filepath.Base(path))
	}
}

func (f *Fixture) SQL(t *testing.T, query string, args ...any) {
	t.Helper()
	_, err := f.Pool.Exec(context.Background(), query, append([]any{pgx.QueryExecModeSimpleProtocol}, args...)...)
	Check(t, err)
}

func (f *Fixture) Grant(t *testing.T, path, runtime string, wantOK bool) {
	t.Helper()
	role := f.PlatformRole
	if runtime == "poker" {
		role = f.PokerRole
	}
	cmd := exec.Command(f.manifest.PSQL, "-X", "-d", "service=h1b_owner", "-v", "ON_ERROR_STOP=1", "-v", "apply_grants=true",
		"-v", "schema_owner="+f.OwnerRole, "-v", "database_name="+f.Database, "-v", "runtime_role="+role,
		"-v", "platform_runtime_role="+f.PlatformRole, "-v", "poker_runtime_role="+f.PokerRole,
		"-v", "history_reader_role="+f.ReaderRole, "-v", "history_worker_role="+f.WorkerRole, "-f", filepath.Join(f.manifest.Repo, path))
	cmd.Env = append(os.Environ(), "PGSERVICEFILE="+f.service)
	if out, err := cmd.CombinedOutput(); (err == nil) != wantOK {
		t.Fatalf("grant %s success=%v wanted=%v: %s", filepath.Base(path), err == nil, wantOK, out)
	}
}
