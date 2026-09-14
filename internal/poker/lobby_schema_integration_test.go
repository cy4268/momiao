package poker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const lobbyMaintenanceMigration = "0019_ops_maintenance.sql"
const lobbyAccessMigration = "0020_poker_lobby_access.sql"

func applyLobbyGrants(t *testing.T, owner, runtime *pgxpool.Pool) {
	t.Helper()
	file := filepath.Join("..", "..", "deploy", "sql", "runtime-grants-0019-0020-poker-lobby.psql")
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return // Allows the pre-implementation behavior RED on the old schema.
	}
	ctx := context.Background()
	var role string
	if err := owner.QueryRow(ctx, "SELECT current_user").Scan(&role); err != nil {
		t.Fatal(err)
	}
	c := owner.Config().ConnConfig
	cmd := exec.CommandContext(ctx, pokerTestPSQL(), "-X", "-q", "-h", c.Host, "-p", strconv.Itoa(int(c.Port)), "-U", c.User, "-d", c.Database, "-v", "schema_owner="+role, "-v", "runtime_role="+runtime.Config().ConnConfig.User, "-v", "apply_grants=true", "-f", file)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+c.Password)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("lobby forward grants: %s", output)
	}
}

func TestLobbyForwardSchemaContracts(t *testing.T) {
	owner, runtime := localPokerDB(t, lobbyMaintenanceMigration, lobbyAccessMigration)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	table := uuid()
	if _, err := owner.Exec(ctx, `INSERT INTO poker.tables(table_id,owner_newapi_user_id,name,max_seats,blind_preset_version,ruleset_version,chat_enabled) VALUES($1,910001,'legacy-public',3,'5-10','poker-cash-v1-20260906',true)`, table); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{lobbyMaintenanceMigration, lobbyAccessMigration} {
		body, err := os.ReadFile(filepath.Join("..", "platform", "migrations", name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err = owner.Exec(ctx, string(body), pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatalf("forward %s: %v", name, err)
		}
	}
	applyLobbyGrants(t, owner, runtime)
	var installed bool
	if err := owner.QueryRow(ctx, `SELECT to_regprocedure('ops.is_maintenance_scope_active(text)') IS NOT NULL AND to_regprocedure('ops.lock_poker_admission_scopes()') IS NOT NULL AND to_regprocedure('economy.poker_lobby_balance(bigint)') IS NOT NULL`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if !installed {
		t.Fatal("lobby admission/read contract missing: old schema exposes neither durable maintenance nor narrow wallet read")
	}
	var count int
	if err := owner.QueryRow(ctx, `SELECT count(*) FROM ops.maintenance_scope_guards`).Scan(&count); err != nil || count != 7 {
		t.Fatalf("seven immutable guards: count=%d err=%v", count, err)
	}
	var access, name string
	var chat bool
	if err := runtime.QueryRow(ctx, `SELECT access_mode,name,chat_enabled FROM poker.tables WHERE table_id=$1`, table).Scan(&access, &name, &chat); err != nil || access != "PUBLIC" || name != "legacy-public" || !chat {
		t.Fatalf("explicit legacy PUBLIC preserves actual table fields: access=%s name=%s chat=%v err=%v", access, name, chat, err)
	}
	if _, err := owner.Exec(ctx, `UPDATE poker.tables SET name=$2 WHERE table_id=$1`, table, strings.Repeat("😀", 40)); err != nil {
		t.Fatal("40 graphemes must not hit obsolete 128-byte constraint", err)
	}
	for _, sql := range []string{`UPDATE poker.tables SET name=''`, `UPDATE poker.tables SET access_mode='UNLISTED'`, `DELETE FROM ops.maintenance_scope_guards`, `UPDATE ops.maintenance_scope_guards SET scope=scope`} {
		if _, err := owner.Exec(ctx, sql); err == nil {
			t.Fatalf("schema invariant admitted %s", sql)
		}
	}
	for _, sql := range []string{`SELECT * FROM economy.wallet_balances`, `SELECT * FROM ops.maintenance_windows`, `UPDATE ops.maintenance_scope_guards SET scope=scope`, `SELECT economy._poker_funding_apply(NULL,0,NULL,'BUY_IN')`} {
		if _, err := runtime.Exec(ctx, sql); err == nil {
			t.Fatalf("runtime escaped narrow read/lock privileges: %s", sql)
		}
	}
	var balance, version int64
	if err := runtime.QueryRow(ctx, `SELECT balance_units,version FROM economy.poker_lobby_balance(910001)`).Scan(&balance, &version); err != nil || balance != 1500000000 || version != 1 {
		t.Fatalf("actual owner balance/version: %d/%d %v", balance, version, err)
	}
	if err := runtime.QueryRow(ctx, `SELECT balance_units FROM economy.poker_lobby_balance(910099)`).Scan(&balance); err != pgx.ErrNoRows {
		t.Fatalf("missing wallet must not become zero: %v", err)
	}
	for _, sql := range []string{`SELECT * FROM economy.poker_lobby_balance(0)`, `SELECT ops.is_maintenance_scope_active('poker')`, `SELECT ops.is_maintenance_scope_active(NULL)`} {
		if _, err := runtime.Exec(ctx, sql); err == nil {
			t.Fatalf("invalid narrow input accepted: %s", sql)
		}
	}
	if err := owner.QueryRow(ctx, `SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE (n.nspname,p.proname) IN (('ops','is_maintenance_scope_active'),('ops','lock_poker_admission_scopes'),('economy','poker_lobby_balance')) AND p.prosecdef AND p.proconfig @> ARRAY['search_path=pg_catalog'] AND NOT EXISTS(SELECT 1 FROM aclexplode(coalesce(p.proacl,acldefault('f',p.proowner))) a WHERE a.grantee=0 AND a.privilege_type='EXECUTE')`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("three fixed-search-path definer functions without PUBLIC execute: %d %v", count, err)
	}
	verifyMaintenanceLockBoundary(t, ctx, owner, runtime)
	t.Log("real PG legacy forward migration, narrow grants, read-only snapshot and both shared/exclusive lock orderings verified; no native Ops mutation claim")
}

func verifyMaintenanceLockBoundary(t *testing.T, ctx context.Context, owner, runtime *pgxpool.Pool) {
	t.Helper()
	window, operation := uuid(), uuid()
	if _, err := owner.Exec(ctx, `INSERT INTO ops.admin_operations(operation_id,actor_kind,newapi_user_id,action,request_hash,details,result) VALUES($1,'OFFLINE',910001,'SYNTHETIC_MAINTENANCE_FIXTURE',repeat('a',64),'{}','{}');`, operation); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `INSERT INTO ops.maintenance_windows(maintenance_id,state,reason,impact_snapshot,impact_hash,environment,scheduled_start_at,created_by,operation_id) VALUES($1,'SCHEDULED','synthetic local maintenance','{}',decode(repeat('a',64),'hex'),'STAGING',clock_timestamp(),910001,$2);`, window, operation); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `INSERT INTO ops.maintenance_window_scopes(maintenance_id,scope) VALUES($1,'POKER_NEW_TABLES_NEW_HANDS')`, window); err != nil {
		t.Fatal(err)
	}
	activate, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(activate)
	if _, err = activate.Exec(ctx, `SELECT scope FROM ops.maintenance_scope_guards WHERE scope IN ('CHALDEA_USER_WRITES','POKER_NEW_TABLES_NEW_HANDS') ORDER BY scope FOR UPDATE; UPDATE ops.maintenance_windows SET state='ACTIVE',activated_at=clock_timestamp(),activated_by=910001,state_version=state_version+1`); err != nil {
		t.Fatal(err)
	}
	read, err := runtime.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(read)
	var readonly string
	var active bool
	if err = read.QueryRow(ctx, "SHOW transaction_read_only").Scan(&readonly); err != nil || readonly != "on" {
		t.Fatalf("not a true read-only transaction: %s %v", readonly, err)
	}
	quick, stop := context.WithTimeout(ctx, time.Second)
	err = read.QueryRow(quick, `SELECT ops.is_maintenance_scope_active('POKER_NEW_TABLES_NEW_HANDS')`).Scan(&active)
	stop()
	if err != nil || active {
		t.Fatalf("read-only projection blocked on guard or saw uncommitted activation: %v/%v", active, err)
	}
	accept, err := runtime.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(accept)
	var pid int
	if err = accept.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := accept.Exec(ctx, `SELECT ops.lock_poker_admission_scopes()`); done <- err }()
	waitLobbyBlocked(t, ctx, owner, pid)
	if err = activate.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if err = accept.QueryRow(ctx, `SELECT ops.is_maintenance_scope_active('POKER_NEW_TABLES_NEW_HANDS')`).Scan(&active); err != nil || !active {
		t.Fatalf("post-wait NEW statement missed committed maintenance: %v/%v", active, err)
	}
	if err = read.QueryRow(ctx, `SELECT ops.is_maintenance_scope_active('POKER_NEW_TABLES_NEW_HANDS')`).Scan(&active); err != nil || active {
		t.Fatalf("read snapshot changed across activation: %v/%v", active, err)
	}
	if err = accept.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// Reverse ordering: existing business shared locks fence End until Commit.
	accept, err = runtime.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(accept)
	if _, err = accept.Exec(ctx, `SELECT ops.lock_poker_admission_scopes()`); err != nil {
		t.Fatal(err)
	}
	end, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(end)
	if err = end.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatal(err)
	}
	go func() {
		_, err := end.Exec(ctx, `SELECT scope FROM ops.maintenance_scope_guards WHERE scope IN ('CHALDEA_USER_WRITES','POKER_NEW_TABLES_NEW_HANDS') ORDER BY scope FOR UPDATE`)
		done <- err
	}()
	waitLobbyBlocked(t, ctx, owner, pid)
	if err = accept.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if _, err = end.Exec(ctx, `UPDATE ops.maintenance_windows SET state='COMPLETED',ended_at=clock_timestamp(),ended_by=910001,state_version=state_version+1`); err != nil {
		t.Fatal(err)
	}
	if err = end.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func waitLobbyBlocked(t *testing.T, ctx context.Context, owner *pgxpool.Pool, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var blocked bool
		if err := owner.QueryRow(ctx, `SELECT cardinality(pg_blocking_pids($1))>0`, pid).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected real PG shared/exclusive guard lock wait")
}
