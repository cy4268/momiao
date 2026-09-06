package platform

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBootstrapInputValidation(t *testing.T) {
	for _, input := range []BootstrapInput{{}, {Environment: "TEST", UserID: 42, Username: "synthetic-user", ReleaseBuild: "synthetic-build", ExpectedEmpty: true}, {Environment: "STAGING", UserID: 42, Username: "synthetic-user", ReleaseBuild: "synthetic-build"}} {
		if _, err := (&Store{}).Bootstrap(context.Background(), input); !errors.Is(err, ErrBootstrapInvalid) {
			t.Fatalf("invalid input reached DB: %v", err)
		}
	}
}

func TestBootstrapPostgresCommittedButReceiptLost(t *testing.T) {
	s := bootstrapStore(t)
	ctx := context.Background()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cfg := s.pool.Config()
	upstream := net.JoinHostPort(cfg.ConnConfig.Host, fmt.Sprint(cfg.ConnConfig.Port))
	lost := make(chan struct{})
	proxyDone := make(chan struct{})
	go func() {
		defer close(proxyDone)
		client, err := listener.Accept()
		if err != nil {
			return
		}
		defer client.Close()
		server, err := net.Dial("tcp", upstream)
		if err != nil {
			return
		}
		defer server.Close()
		go io.Copy(server, client)
		for {
			var header [5]byte
			if _, err = io.ReadFull(server, header[:]); err != nil {
				return
			}
			length := binary.BigEndian.Uint32(header[1:])
			if length < 4 || length > 1<<24 {
				return
			}
			body := make([]byte, int(length)-4)
			if _, err = io.ReadFull(server, body); err != nil {
				return
			}
			// CommandComplete is emitted after PostgreSQL commits. Deliberately drop
			// the connection before any COMMIT acknowledgement reaches pgx.
			if header[0] == 'C' && string(body) == "COMMIT\x00" {
				close(lost)
				return
			}
			if _, err = client.Write(append(header[:], body...)); err != nil {
				return
			}
		}
	}()
	addr := listener.Addr().(*net.TCPAddr)
	cfg.ConnConfig.Host = "127.0.0.1"
	cfg.ConnConfig.Port = uint16(addr.Port)
	cfg.ConnConfig.TLSConfig = nil
	cfg.ConnConfig.Fallbacks = nil
	proxyPool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer proxyPool.Close()
	_, err = (&Store{pool: proxyPool}).Bootstrap(ctx, bootstrapInput())
	if !errors.Is(err, ErrBootstrapOutcomeUnknown) {
		t.Fatalf("lost commit receipt not classified: %v", err)
	}
	select {
	case <-lost:
	case <-time.After(time.Second):
		t.Fatal("proxy did not observe real commit")
	}
	<-proxyDone
	bootstrapCounts(t, s, 1)
	if _, err = s.Bootstrap(ctx, bootstrapInput()); !errors.Is(err, ErrBootstrapClosed) {
		t.Fatalf("retry reopened committed bootstrap: %v", err)
	}
}

func TestBootstrapPostgresLeastPrivilege(t *testing.T) {
	s := bootstrapStore(t)
	ctx := context.Background()
	suffix := fmt.Sprint(time.Now().UnixNano())
	owner := "m4_bootstrap_owner_" + suffix
	executor := "m4_bootstrap_exec_" + suffix
	runtimeRole := "m4_bootstrap_runtime_" + suffix
	password, err := uuidV7()
	if err != nil {
		t.Fatal(err)
	}
	roles := []string{owner, executor, runtimeRole}
	for i, role := range roles {
		query := "CREATE ROLE " + pgx.Identifier{role}.Sanitize() + " NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT "
		if i == 0 {
			query += "NOLOGIN"
		} else {
			query += "LOGIN PASSWORD '" + password + "'"
		}
		if _, err = s.pool.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, role := range roles {
			if _, err := s.pool.Exec(ctx, "DROP OWNED BY "+pgx.Identifier{role}.Sanitize()+" CASCADE; DROP ROLE "+pgx.Identifier{role}.Sanitize()); err != nil {
				t.Errorf("test role cleanup: %v", err)
			}
		}
	})
	oid := pgx.Identifier{owner}.Sanitize()
	eid := pgx.Identifier{executor}.Sanitize()
	rid := pgx.Identifier{runtimeRole}.Sanitize()
	function := `ops.bootstrap_super_admin(TEXT,BIGINT,TEXT,TEXT,BOOLEAN,UUID,UUID,UUID)`
	grants := `GRANT USAGE ON SCHEMA ops,identity TO ` + oid + `;
 GRANT SELECT,INSERT ON identity.account_refs,ops.admin_principals,ops.admin_role_history,ops.bootstrap_closure,ops.admin_operations TO ` + oid + `;
 GRANT SELECT,UPDATE ON ops.access_control_guards TO ` + oid + `;
 ALTER FUNCTION ` + function + ` OWNER TO ` + oid + `;
 ALTER FUNCTION ops.lock_admin_principal_set() OWNER TO ` + oid + `;
 ALTER FUNCTION ops.close_bootstrap_on_principal() OWNER TO ` + oid + `;
 GRANT USAGE ON SCHEMA ops TO ` + eid + `,` + rid + `;
 GRANT EXECUTE ON FUNCTION ` + function + ` TO ` + eid + `;
	 GRANT SELECT ON ops.admin_principals TO ` + rid + `;
	 GRANT INSERT ON ops.admin_operations TO ` + rid + `;`
	if _, err = s.pool.Exec(ctx, grants); err != nil {
		t.Fatal(err)
	}
	connect := func(role string) *Store {
		cfg := s.pool.Config()
		cfg.ConnConfig.User = role
		cfg.ConnConfig.Password = password
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			t.Fatal("restricted connection setup failed")
		}
		t.Cleanup(pool.Close)
		var current, session string
		var super bool
		if err = pool.QueryRow(ctx, `SELECT current_user,session_user,rolsuper FROM pg_roles WHERE rolname=current_user`).Scan(&current, &session, &super); err != nil || super || current != role || session != role {
			t.Fatal("not a real restricted login")
		}
		return &Store{pool: pool}
	}
	runtimeStore := connect(runtimeRole)
	if _, err = runtimeStore.pool.Exec(ctx, `INSERT INTO ops.admin_operations(operation_id,actor_kind,action,request_hash,details,result) VALUES('00000000-0000-4000-8000-000000000099','SYSTEM','SYSTEM_BOOTSTRAP',repeat('a',64),'{}','{}')`); err == nil {
		t.Fatal("runtime forged SYSTEM_BOOTSTRAP audit")
	}
	if _, err = runtimeStore.Bootstrap(ctx, bootstrapInput()); !errors.Is(err, ErrBootstrapFailed) {
		t.Fatalf("runtime got bootstrap authority: %v", err)
	}
	deploymentStore := connect(executor)
	for _, query := range []string{`INSERT INTO identity.account_refs(newapi_user_id) VALUES(99)`, `INSERT INTO ops.admin_principals(admin_principal_id,newapi_user_id,base_role) VALUES('00000000-0000-4000-8000-000000000099',99,'SUPER_ADMIN')`, `DELETE FROM ops.bootstrap_closure`, `INSERT INTO ops.admin_operations(operation_id,actor_kind,action,request_hash,details,result) VALUES('00000000-0000-4000-8000-000000000099','SYSTEM','SYSTEM_BOOTSTRAP',repeat('a',64),'{}','{}')`, `CREATE TABLE ops.synthetic_table(id INT)`} {
		if _, err = deploymentStore.pool.Exec(ctx, query); err == nil {
			t.Fatalf("deployment role got table/DDL authority: %s", query)
		}
	}
	if _, err = deploymentStore.Bootstrap(ctx, bootstrapInput()); err != nil {
		t.Fatal(err)
	}
	bootstrapCounts(t, s, 1)
	if _, err = deploymentStore.Bootstrap(ctx, bootstrapInput()); !errors.Is(err, ErrBootstrapClosed) {
		t.Fatalf("restricted retry: %v", err)
	}
}

// Each one-shot case gets an entirely new database. Never reset shared schemas,
// principals or append-only history to make another test pass.
func bootstrapStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("MOMIAO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MOMIAO_TEST_DATABASE_URL absent: bootstrap PostgreSQL integration not run")
	}
	if err := validateTestDatabase(dsn); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("test DSN invalid")
	}
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	name := "momiao_test_m4_" + strings.ReplaceAll(t.Name(), "/", "_") + "_" + time.Now().Format("150405000000")
	if len(name) > 63 {
		name = name[:45] + "_" + time.Now().Format("150405000000")
	}
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	sPool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{pool: sPool}
	t.Cleanup(func() {
		s.Close()
		_, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize())
		admin.Close()
		if err != nil {
			t.Errorf("isolated database cleanup: %v", err)
		}
	})
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return s
}
func bootstrapInput() BootstrapInput {
	return BootstrapInput{Environment: "STAGING", UserID: 42, Username: "synthetic-user", ReleaseBuild: "synthetic-build", ExpectedEmpty: true}
}
func bootstrapCounts(t *testing.T, s *Store, want int) {
	t.Helper()
	for _, table := range []string{"identity.account_refs", "ops.admin_principals", "ops.admin_role_history", "ops.bootstrap_closure", "ops.admin_operations"} {
		var count int
		if err := s.pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != want {
			t.Fatalf("%s count=%d want=%d error=%v", table, count, want, err)
		}
	}
}
func TestBootstrapPostgresAtomicAndPermanent(t *testing.T) {
	s := bootstrapStore(t)
	ctx := context.Background()
	result, err := s.Bootstrap(ctx, bootstrapInput())
	if err != nil {
		t.Fatal(err)
	}
	if result.PrincipalID == "" || result.OperationID == "" || result.CreatedAt.IsZero() {
		t.Fatal("missing receipt")
	}
	bootstrapCounts(t, s, 1)
	var role, status, actor, action, environment, target, release string
	if err = s.pool.QueryRow(ctx, `SELECT p.base_role,p.status,a.actor_kind,a.action,a.details->>'environment',a.details->>'target_newapi_user_id',a.details->>'release_build' FROM ops.admin_principals p CROSS JOIN ops.admin_operations a`).Scan(&role, &status, &actor, &action, &environment, &target, &release); err != nil {
		t.Fatal(err)
	}
	if role != "SUPER_ADMIN" || status != "ACTIVE" || actor != "SYSTEM" || action != "SYSTEM_BOOTSTRAP" || environment != "STAGING" || target != "42" || release != "synthetic-build" {
		t.Fatal("wrong authority/audit")
	}
	if _, err = s.Bootstrap(ctx, bootstrapInput()); !errors.Is(err, ErrBootstrapClosed) {
		t.Fatalf("replayed bootstrap: %v", err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE ops.admin_principals SET status='DISABLED'`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Bootstrap(ctx, bootstrapInput()); !errors.Is(err, ErrBootstrapClosed) {
		t.Fatalf("disabled reopened: %v", err)
	}
	if _, err = s.pool.Exec(ctx, `DELETE FROM ops.admin_principals`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Bootstrap(ctx, bootstrapInput()); !errors.Is(err, ErrBootstrapClosed) {
		t.Fatalf("deleted reopened: %v", err)
	}
	for _, table := range []string{"ops.admin_role_history", "ops.bootstrap_closure", "ops.admin_operations"} {
		for _, query := range []string{"UPDATE " + table + " SET created_at=created_at", "DELETE FROM " + table, "TRUNCATE " + table} {
			if _, err = s.pool.Exec(ctx, query); err == nil {
				t.Fatalf("history mutable: %s", query)
			}
		}
	}
}
func TestBootstrapPostgresConcurrent(t *testing.T) {
	s := bootstrapStore(t)
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.Bootstrap(context.Background(), bootstrapInput()); results <- err }()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrBootstrapClosed) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("successes=%d", successes)
	}
	bootstrapCounts(t, s, 1)
}
func TestBootstrapPostgresExistingRolesCloseForever(t *testing.T) {
	for _, role := range []string{"SUPER_ADMIN", "OPERATOR", "AUDITOR"} {
		for _, status := range []string{"ACTIVE", "DISABLED"} {
			t.Run(role+"_"+status, func(t *testing.T) {
				s := bootstrapStore(t)
				ctx := context.Background()
				_, err := s.pool.Exec(ctx, `INSERT INTO ops.admin_principals(admin_principal_id,newapi_user_id,base_role,status) VALUES('00000000-0000-4000-8000-000000000091',91,$1,$2)`, role, status)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = s.Bootstrap(ctx, bootstrapInput()); !errors.Is(err, ErrBootstrapClosed) {
					t.Fatalf("existing admin accepted: %v", err)
				}
				if _, err = s.pool.Exec(ctx, `DELETE FROM ops.admin_principals`); err != nil {
					t.Fatal(err)
				}
				if _, err = s.Bootstrap(ctx, bootstrapInput()); !errors.Is(err, ErrBootstrapClosed) {
					t.Fatalf("deleted admin accepted: %v", err)
				}
				var count int
				if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM identity.account_refs`).Scan(&count); err != nil || count != 0 {
					t.Fatal("failed bootstrap wrote account_ref")
				}
			})
		}
	}
}
func TestBootstrapPostgresTransactionFailure(t *testing.T) {
	for _, table := range []string{"admin_principals", "admin_role_history", "admin_operations"} {
		t.Run(table, func(t *testing.T) {
			s := bootstrapStore(t)
			ctx := context.Background()
			_, err := s.pool.Exec(ctx, `CREATE FUNCTION ops.synthetic_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic failure'; END $$; CREATE TRIGGER synthetic_failure BEFORE INSERT ON ops.`+table+` FOR EACH STATEMENT EXECUTE FUNCTION ops.synthetic_failure()`)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Bootstrap(ctx, bootstrapInput()); !errors.Is(err, ErrBootstrapFailed) {
				t.Fatalf("failure not recognized: %v", err)
			}
			bootstrapCounts(t, s, 0)
			if _, err = s.pool.Exec(ctx, "DROP TRIGGER synthetic_failure ON ops."+table); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Bootstrap(ctx, bootstrapInput()); err != nil {
				t.Fatal(err)
			}
			bootstrapCounts(t, s, 1)
		})
	}
}
