package authbridge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker/connectticket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Real local PG, synthetic native authority only. No production database or
// native credential is read. Each test owns its exact fresh DB and two roles.
func bindingDB(t *testing.T) (*pgxpool.Pool, *pgxpool.Pool) {
	t.Helper()
	path := os.Getenv("POKER_AUTH_TEST_CONNECTION_FILE")
	if path == "" {
		t.Skip("set POKER_AUTH_TEST_CONNECTION_FILE for real local PG")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("test connection file unavailable")
	}
	var c struct {
		Host, User, Password, Database string
		Port                           int
	}
	if json.Unmarshal(data, &c) != nil || c.Host != "127.0.0.1" || c.Port != 55432 {
		t.Fatal("wrong local test authority")
	}
	ctx := context.Background()
	cfg, _ := pgxpool.ParseConfig("")
	cfg.ConnConfig.Host, cfg.ConnConfig.Port, cfg.ConnConfig.User, cfg.ConnConfig.Password, cfg.ConnConfig.Database = c.Host, uint16(c.Port), c.User, c.Password, c.Database
	cfg.ConnConfig.TLSConfig = nil
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("test admin config")
	}
	if err = admin.Ping(ctx); err != nil {
		t.Fatal("test admin unavailable")
	}
	var random [12]byte
	if _, err = rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	suffix := strconv.Itoa(os.Getpid()) + "_" + hex.EncodeToString(random[:4])
	db, owner, runtime := "root_poker_auth_"+suffix, "root_pauth_owner_"+suffix, "root_pauth_runtime_"+suffix
	password := hex.EncodeToString(random[:])
	for _, sql := range []string{
		"CREATE ROLE " + pgx.Identifier{owner}.Sanitize() + " NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS",
		"CREATE ROLE " + pgx.Identifier{runtime}.Sanitize() + " LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD '" + password + "'",
		"CREATE DATABASE " + pgx.Identifier{db}.Sanitize() + " OWNER " + pgx.Identifier{owner}.Sanitize(),
	} {
		if _, err = admin.Exec(ctx, sql); err != nil {
			t.Fatal("owned test database setup")
		}
	}
	var ownerPool, runtimePool *pgxpool.Pool
	t.Cleanup(func() {
		if runtimePool != nil {
			runtimePool.Close()
		}
		if ownerPool != nil {
			ownerPool.Close()
		}
		if !strings.HasPrefix(db, "root_poker_auth_") {
			panic("test scope")
		}
		for _, sql := range []string{"DROP DATABASE " + pgx.Identifier{db}.Sanitize() + " WITH (FORCE)", "DROP ROLE " + pgx.Identifier{runtime}.Sanitize(), "DROP ROLE " + pgx.Identifier{owner}.Sanitize()} {
			if _, e := admin.Exec(ctx, sql); e != nil {
				t.Error("owned test cleanup failed")
			}
		}
		admin.Close()
	})
	ownerCfg := cfg.Copy()
	ownerCfg.ConnConfig.Database = db
	ownerCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, e := conn.Exec(ctx, "SET ROLE "+pgx.Identifier{owner}.Sanitize())
		return e
	}
	ownerPool, err = pgxpool.NewWithConfig(ctx, ownerCfg)
	if err != nil {
		t.Fatal(err)
	}
	// Only committed base migrations 0001–0009 are present in this independent
	// checkout. Apply 0018 explicitly: this is NOT a merged 0001–0018 manifest test.
	for n := 1; n <= 9; n++ {
		matches, _ := filepath.Glob(filepath.Join("..", "..", "platform", "migrations", fmtVersion(n)+"_*.sql"))
		if len(matches) != 1 {
			t.Fatal("base migration path")
		}
		raw, e := os.ReadFile(matches[0])
		if e != nil {
			t.Fatal(e)
		}
		if n == 1 {
			if _, e = ownerPool.Exec(ctx, "CREATE SCHEMA platform_meta"); e != nil {
				t.Fatal(e)
			}
		}
		if _, e = ownerPool.Exec(ctx, string(raw), pgx.QueryExecModeSimpleProtocol); e != nil {
			t.Fatal("base migration", n, e)
		}
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "platform", "migrations", "0018_auth_session_binding.sql"))
	if err != nil {
		t.Fatal("new binding migration missing")
	}
	if _, err = ownerPool.Exec(ctx, string(raw), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal("binding migration", err)
	}
	if _, err = ownerPool.Exec(ctx, "GRANT CONNECT ON DATABASE "+pgx.Identifier{db}.Sanitize()+" TO "+pgx.Identifier{runtime}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	grant, err := os.ReadFile(filepath.Join("..", "..", "platform", "testdata", "runtime-auth-binding-0018.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ownerPool.Exec(ctx, strings.ReplaceAll(string(grant), `:"runtime_role"`, pgx.Identifier{runtime}.Sanitize()), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	runtimeCfg := cfg.Copy()
	runtimeCfg.ConnConfig.Database, runtimeCfg.ConnConfig.User, runtimeCfg.ConnConfig.Password = db, runtime, password
	runtimePool, err = pgxpool.NewWithConfig(ctx, runtimeCfg)
	if err != nil {
		t.Fatal(err)
	}
	var actual string
	var super bool
	if err = runtimePool.QueryRow(ctx, "SELECT current_user,rolsuper FROM pg_roles WHERE rolname=current_user").Scan(&actual, &super); err != nil || super || actual != runtime {
		t.Fatal("test must use least-privilege runtime")
	}
	return ownerPool, runtimePool
}
func fmtVersion(n int) string { return "000" + strconv.Itoa(n) }

func TestBindingRuntimeUsesDatabaseOwnedAuditTime(t *testing.T) {
	owner, pool := bindingDB(t)
	ctx := context.Background()
	if _, err := owner.Exec(ctx, "INSERT INTO identity.account_refs(newapi_user_id) VALUES(42)"); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(ctx, `INSERT INTO identity.native_session_bindings(session_id_hash,newapi_user_id,session_version,native_auth_version,security_epoch_snapshot,native_created_at,bound_at) VALUES(repeat('a',64),42,1,1,0,now(),TIMESTAMPTZ '2000-01-01 00:00:00Z')`)
	var denied *pgconn.PgError
	if !errors.As(err, &denied) || denied.Code != "42501" {
		t.Fatal("runtime must not supply the database-owned binding audit time", err)
	}
	var before time.Time
	if err = owner.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&before); err != nil {
		t.Fatal(err)
	}
	created := time.Now().Add(-time.Hour).Truncate(time.Second)
	a, err := New(pool, func(_ context.Context, ref NativeRef) (NativeSession, error) {
		return NativeSession{Ref: ref, UserAuthVersion: 1, CreatedAt: created, ExpiresAt: created.Add(2 * time.Hour)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := NativeRef{UserID: "42", SessionIDHash: strings.Repeat("b", 64), SessionVersion: 1}
	if _, err = a.Bind(ctx, ref); err != nil {
		t.Fatal("normal six-column binding must still work", err)
	}
	ref.SessionVersion = 2
	session, err := a.Bind(ctx, ref)
	if err != nil || a.Check(ctx, session) != nil {
		t.Fatal("same-epoch version advancement must still work", err)
	}
	var bound time.Time
	if err = pool.QueryRow(ctx, "SELECT bound_at FROM identity.native_session_bindings WHERE session_id_hash=$1", ref.SessionIDHash).Scan(&bound); err != nil || bound.Before(before) {
		t.Fatal("binding audit time must come from database default", err)
	}
}

func TestBindingRealPGPreservesEpochAndNativeAuthority(t *testing.T) {
	owner, pool := bindingDB(t)
	ctx := context.Background()
	if _, err := owner.Exec(ctx, "INSERT INTO identity.account_refs(newapi_user_id) VALUES(42)"); err != nil {
		t.Fatal(err)
	}
	ref := NativeRef{UserID: "42", SessionIDHash: strings.Repeat("a", 64), SessionVersion: 1}
	live := NativeSession{Ref: ref, UserAuthVersion: 1, CreatedAt: time.Now().Add(-time.Hour).Truncate(time.Second), ExpiresAt: time.Now().Add(time.Hour).Truncate(time.Second)}
	var mu sync.RWMutex
	var nativeErr error
	a, err := New(pool, func(_ context.Context, r NativeRef) (NativeSession, error) {
		mu.RLock()
		defer mu.RUnlock()
		if nativeErr != nil {
			return NativeSession{}, nativeErr
		}
		if r != live.Ref {
			return NativeSession{}, connectticket.ErrRevoked
		}
		return live, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, e := a.Bind(ctx, ref)
			if e == nil && (s.SecurityEpoch != 0 || s.UserID != "42" || s.SessionVersion != 1) {
				e = errors.New("wrong immutable snapshot")
			}
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	for e := range results {
		if e != nil {
			t.Fatal(e)
		}
	}
	s := connectticket.Session{UserID: "42", SessionIDHash: ref.SessionIDHash, SessionVersion: 1, SecurityEpoch: 0}
	if err = a.Check(ctx, s); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = owner.QueryRow(ctx, "SELECT count(*) FROM identity.native_session_bindings").Scan(&count); err != nil || count != 1 {
		t.Fatal("binding not idempotent", count, err)
	}
	mu.Lock()
	nativeErr = connectticket.ErrRevoked
	mu.Unlock()
	if !errors.Is(a.Check(ctx, s), connectticket.ErrRevoked) {
		t.Fatal("native revocation did not deny old ticket")
	}
	mu.Lock()
	nativeErr = errors.New("secret provider failure")
	mu.Unlock()
	if e := a.Check(ctx, s); e != connectticket.ErrUnavailable {
		t.Fatal("provider error leaked or did not fail closed", e)
	}
	mu.Lock()
	nativeErr = nil
	live.Ref.SessionVersion = 2
	live.UserAuthVersion = 2
	mu.Unlock()
	if !errors.Is(a.Check(ctx, s), connectticket.ErrRevoked) {
		t.Fatal("old native session version accepted")
	}
	ref.SessionVersion = 2
	s, err = a.Bind(ctx, ref)
	if err != nil || s.SessionVersion != 2 {
		t.Fatal("native version advance in same epoch", err)
	}
	if _, err = owner.Exec(ctx, "UPDATE identity.account_refs SET security_epoch=1 WHERE newapi_user_id=42"); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(a.Check(ctx, s), connectticket.ErrRevoked) {
		t.Fatal("epoch mismatch did not revoke")
	}
	if _, err = a.Bind(ctx, ref); !errors.Is(err, connectticket.ErrRevoked) {
		t.Fatal("old SID rebound to new epoch", err)
	}
	mu.Lock()
	live.Ref.SessionIDHash = strings.Repeat("b", 64)
	mu.Unlock()
	ref.SessionIDHash = strings.Repeat("b", 64)
	if _, err = a.Bind(ctx, ref); !errors.Is(err, connectticket.ErrRevoked) {
		t.Fatal("previously unbound old native chain adopted new epoch", err)
	}
	var fence time.Time
	if err = owner.QueryRow(ctx, "SELECT security_epoch_changed_at FROM identity.account_refs WHERE newapi_user_id=42").Scan(&fence); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	live.CreatedAt = fence.Truncate(time.Second)
	mu.Unlock()
	if _, err = a.Bind(ctx, ref); !errors.Is(err, connectticket.ErrRevoked) {
		t.Fatal("same-second ambiguous chain accepted", err)
	}
	time.Sleep(time.Until(fence.Truncate(time.Second).Add(2 * time.Second)))
	mu.Lock()
	live.CreatedAt = time.Now().Truncate(time.Second)
	mu.Unlock()
	s, err = a.Bind(ctx, ref)
	if err != nil || s.SecurityEpoch != 1 {
		t.Fatal("new post-event auth chain rejected", err)
	}
	if err = a.Check(ctx, s); err != nil {
		t.Fatal(err)
	}
	t.Log("real PG: eight concurrent bindings / one row; native revoke, version and outage fail closed; immutable epoch; old and same-second chains denied; fresh chain accepted")
}

func TestBindingRealPGGuardsAndInput(t *testing.T) {
	owner, pool := bindingDB(t)
	ctx := context.Background()
	if _, err := owner.Exec(ctx, "INSERT INTO identity.account_refs(newapi_user_id) VALUES(42)"); err != nil {
		t.Fatal(err)
	}
	a, err := New(pool, func(context.Context, NativeRef) (NativeSession, error) {
		t.Error("malformed reference reached native authority")
		return NativeSession{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []NativeRef{{"042", strings.Repeat("a", 64), 1}, {"0", strings.Repeat("a", 64), 1}, {"42", strings.Repeat("A", 64), 1}, {"42", strings.Repeat("a", 64), 0}, {"42", strings.Repeat("a", 64), 9007199254740992}} {
		if _, e := a.Bind(ctx, r); e != connectticket.ErrInvalid {
			t.Fatal("malformed reference", e)
		}
	}
	for _, sql := range []string{"UPDATE identity.account_refs SET security_epoch=1", "INSERT INTO identity.native_session_bindings(session_id_hash,newapi_user_id,session_version,native_auth_version,security_epoch_snapshot,native_created_at) VALUES(repeat('a',64),42,1,1,0,now())"} {
		if strings.HasPrefix(sql, "UPDATE") {
			if _, e := pool.Exec(ctx, sql); e == nil {
				t.Fatal("runtime mutated account epoch")
			}
		} else {
			if _, e := pool.Exec(ctx, sql); e != nil {
				t.Fatal("runtime insert grant missing", e)
			}
		}
	}
	for _, sql := range []string{"UPDATE identity.native_session_bindings SET security_epoch_snapshot=1", "DELETE FROM identity.native_session_bindings", "TRUNCATE identity.native_session_bindings", "UPDATE identity.account_refs SET security_epoch_changed_at=now()+interval '1 hour'", "UPDATE identity.account_refs SET security_epoch=-1"} {
		if _, e := owner.Exec(ctx, sql); e == nil {
			t.Fatal("durable auth guard missing", sql)
		}
	}
}
