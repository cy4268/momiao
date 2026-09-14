package poker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker/engine"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Local test double for the external Redis-style lease boundary, deliberately
// not a production implementation and not reconstructed from reservation rows.
type fixtureLeases struct {
	mu      sync.Mutex
	entries map[string]leaseEntry
}
type leaseEntry struct {
	token   string
	expires time.Time
}

func (l *fixtureLeases) Acquire(_ context.Context, table string, seat int, token string, expires time.Time) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	k := fmt.Sprintf("%s:%d", table, seat)
	old, ok := l.entries[k]
	if ok && old.expires.After(expires.Add(-30*time.Second)) {
		return old.token == token, nil
	}
	l.entries[k] = leaseEntry{token, expires}
	return true, nil
}
func (l *fixtureLeases) Valid(_ context.Context, table string, seat int, token string, now time.Time) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entries[fmt.Sprintf("%s:%d", table, seat)]
	return e.token == token && e.expires.After(now), nil
}
func (l *fixtureLeases) Release(_ context.Context, table string, seat int, token string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	k := fmt.Sprintf("%s:%d", table, seat)
	if l.entries[k].token == token {
		delete(l.entries, k)
	}
	return nil
}

func localPokerDB(t *testing.T, skipMigrations ...string) (*pgxpool.Pool, *pgxpool.Pool) {
	t.Helper()
	path := os.Getenv("POKER_TEST_CONNECTION_FILE")
	if path == "" {
		t.Skip("set POKER_TEST_CONNECTION_FILE for isolated real-PG service test")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("test connection file unavailable")
	}
	var c struct {
		Host, User, Password, Database, SSLMode string
		Port                                    int
	}
	if json.Unmarshal(data, &c) != nil || c.Host != "127.0.0.1" || c.Port != 55432 {
		t.Fatal("test connection must target the explicitly named local PG")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Host = c.Host
	cfg.ConnConfig.Port = uint16(c.Port)
	cfg.ConnConfig.User = c.User
	cfg.ConnConfig.Password = c.Password
	cfg.ConnConfig.Database = c.Database
	cfg.ConnConfig.TLSConfig = nil
	cfg.MaxConns = 8
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("local admin pool unavailable")
	}
	if err = admin.Ping(ctx); err != nil {
		t.Fatal("local PG not reachable")
	}
	suffix := strconv.Itoa(os.Getpid())
	db := "g3_poker_test_" + suffix
	owner := "g3_poker_owner_" + suffix
	runtime := "g3_poker_runtime_" + suffix
	var password [24]byte
	if _, err = rand.Read(password[:]); err != nil {
		t.Fatal(err)
	}
	pass := hex.EncodeToString(password[:])
	for _, sql := range []string{"CREATE ROLE " + pgx.Identifier{owner}.Sanitize() + " NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS", "CREATE ROLE " + pgx.Identifier{runtime}.Sanitize() + " LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD '" + pass + "'", "CREATE DATABASE " + pgx.Identifier{db}.Sanitize() + " OWNER " + pgx.Identifier{owner}.Sanitize()} {
		if _, err = admin.Exec(ctx, sql); err != nil {
			t.Fatal("isolated test DB/role creation failed", err)
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
		if !strings.HasPrefix(db, "g3_poker_test_") {
			panic("test DB scope")
		}
		_, _ = admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{db}.Sanitize()+" WITH (FORCE)")
		_, _ = admin.Exec(ctx, "DROP ROLE "+pgx.Identifier{runtime}.Sanitize())
		_, _ = admin.Exec(ctx, "DROP ROLE "+pgx.Identifier{owner}.Sanitize())
		admin.Close()
	})
	ownerCfg := cfg.Copy()
	ownerCfg.ConnConfig.Database = db
	ownerCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET ROLE "+pgx.Identifier{owner}.Sanitize())
		return err
	}
	ownerPool, err = pgxpool.NewWithConfig(ctx, ownerCfg)
	if err != nil {
		t.Fatal("owner pool")
	}
	if _, err = ownerPool.Exec(ctx, "CREATE SCHEMA platform_meta"); err != nil {
		t.Fatal(err)
	}
	// This isolated module checkout has 0001–0009 and reserved 0013/0014;
	// the combined platform manifest also includes G1's independent 0010–0012.
	files, err := filepath.Glob(filepath.Join("..", "platform", "migrations", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if slices.Contains(skipMigrations, filepath.Base(file)) {
			continue
		}
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = ownerPool.Exec(ctx, string(body), pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatalf("migration %s: %v", filepath.Base(file), err)
		}
	}
	psql := pokerTestPSQL()
	cmd := exec.CommandContext(ctx, psql, "-X", "-q", "-h", c.Host, "-p", strconv.Itoa(c.Port), "-U", c.User, "-d", db, "-v", "schema_owner="+owner, "-v", "runtime_role="+runtime, "-v", "apply_grants=true", "-f", filepath.Join("..", "..", "deploy", "sql", "runtime-grants-0013-poker.psql"))
	cmd.Env = append(os.Environ(), "PGPASSWORD="+c.Password)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("reviewed grants application: %s", output)
	}
	cmd = exec.CommandContext(ctx, psql, "-X", "-q", "-h", c.Host, "-p", strconv.Itoa(c.Port), "-U", c.User, "-d", db, "-v", "schema_owner="+owner, "-v", "runtime_role="+runtime, "-v", "apply_grants=true", "-f", filepath.Join("..", "..", "deploy", "sql", "runtime-grants-0017-poker.psql"))
	cmd.Env = append(os.Environ(), "PGPASSWORD="+c.Password)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("0017 grants application: %s", output)
	}
	runtimeCfg := cfg.Copy()
	runtimeCfg.ConnConfig.Database = db
	runtimeCfg.ConnConfig.User = runtime
	runtimeCfg.ConnConfig.Password = pass
	runtimePool, err = pgxpool.NewWithConfig(ctx, runtimeCfg)
	if err != nil {
		t.Fatal("runtime pool")
	}
	if err = runtimePool.Ping(ctx); err != nil {
		t.Fatal("runtime connection")
	}
	if len(skipMigrations) == 0 {
		applyLobbyGrants(t, ownerPool, runtimePool)
	}
	for _, user := range []int64{910001, 910002, 910003} {
		_, err = ownerPool.Exec(ctx, `INSERT INTO identity.account_refs(newapi_user_id) VALUES($1);`, user)
		if err != nil {
			t.Fatal(err)
		}
		_, err = ownerPool.Exec(ctx, `INSERT INTO identity.master_profiles(newapi_user_id,display_name,normalized_name) VALUES($1,$2,$2)`, user, fmt.Sprintf("Synthetic %d", user))
		if err != nil {
			t.Fatal(err)
		}
		_, err = ownerPool.Exec(ctx, `INSERT INTO economy.wallet_balances(newapi_user_id,asset_type,balance_units) VALUES($1,'AVAILABLE_CHIPS',$2)`, user, 3000*engine.UnitsPerChip)
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Log("isolated real PG database/roles and reviewed runtime grants ready; no DSN emitted")
	return ownerPool, runtimePool
}

func pokerTestPSQL() string {
	if path := os.Getenv("POKER_TEST_PSQL"); path != "" {
		return path
	}
	return "psql"
}

func TestRealPGCashTableFundingAndRecovery(t *testing.T) {
	owner, pool := localPokerDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	var commits atomic.Int64
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	opts := Options{Pool: pool, Keyring: Keyring{Current: "poker-test-v1", Keys: map[string][]byte{"poker-test-v1": key}}, Leases: &fixtureLeases{entries: map[string]leaseEntry{}}, MailboxCapacity: 8, AfterCommit: func(tableID string, version uint64) {
		var committed uint64
		if err := owner.QueryRow(context.Background(), "SELECT table_version FROM poker.tables WHERE table_id=$1", tableID).Scan(&committed); err != nil || committed < version {
			t.Error("publication before durable COMMIT")
		}
		commits.Add(1)
	}}
	s, err := legacyTestNew(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, sql := range []string{"UPDATE economy.wallet_balances SET balance_units=balance_units", "SELECT economy._poker_funding_apply(NULL,0,NULL,'BUY_IN')"} {
		if _, err := pool.Exec(ctx, sql); err == nil {
			t.Fatal("runtime escaped narrow funding privileges")
		}
	}
	created, err := s.CreateTable(ctx, CreateTableCommand{UserID: 910001, Key: "g3-create-table-001", Name: "Synthetic friends", BlindPreset: "5-10", MaxSeats: 3, AllowSpectators: true})
	if err != nil {
		t.Fatal(err)
	}
	table := created.TableID
	reserved, err := s.ReserveSeat(ctx, ReserveSeatCommand{UserID: 910001, Key: "g3-reserve-seat-001", TableID: table, Seat: 1})
	if err != nil {
		t.Fatal(err)
	}
	buy := BuyInCommand{UserID: 910001, Key: "g3-buyin-seat-0001", TableID: table, ReservationID: reserved.ReservationID, AmountUnits: 400 * engine.UnitsPerChip}
	var wg sync.WaitGroup
	out := make(chan Receipt, 3)
	errs := make(chan error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r, e := s.BuyIn(ctx, buy); out <- r; errs <- e }()
	}
	wg.Wait()
	close(out)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var session string
	for r := range out {
		if r.Status != "CONFIRMED" {
			t.Fatal(r)
		}
		if session != "" && session != r.SessionID {
			t.Fatal("duplicate buy-in created sessions")
		}
		session = r.SessionID
	}
	reserved2, err := s.ReserveSeat(ctx, ReserveSeatCommand{UserID: 910002, Key: "g3-reserve-seat-002", TableID: table, Seat: 2})
	if err != nil {
		t.Fatal(err)
	}
	var session2 string
	if r, err := s.BuyIn(ctx, BuyInCommand{UserID: 910002, Key: "g3-buyin-seat-0002", TableID: table, ReservationID: reserved2.ReservationID, AmountUnits: 400 * engine.UnitsPerChip}); err != nil || r.Status != "CONFIRMED" {
		t.Fatal(r, err)
	} else {
		session2 = r.SessionID
	}
	legacyRef(t, s, table, 910001)
	legacyRef(t, s, table, 910002)
	started, err := s.StartHand(ctx, StartHandCommand{UserID: 910001, Key: "g3-start-hand-0001", TableID: table})
	if err != nil {
		t.Fatal(err)
	}
	if started.HandID == "" {
		t.Fatal("missing durable hand")
	}
	view, err := s.View(ctx, table, 910001)
	if err != nil {
		t.Fatal(err)
	}
	if view.Hand == nil || view.Hand.Street == "COMMITTED" {
		t.Fatal("phase B missing")
	}
	for _, seat := range view.Seats {
		if seat.IsSelf && len(seat.HoleCards) != 2 {
			t.Fatal("own cards missing")
		}
		if !seat.IsSelf && len(seat.HoleCards) != 0 {
			t.Fatal("opponent cards leaked")
		}
	}
	public, err := s.View(ctx, table, 910003)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(public)
	if strings.Contains(string(body), "\"hole_cards\"") || strings.Contains(string(body), "frozen_deck") || strings.Contains(string(body), "server_seed\"") {
		t.Fatal("spectator private material leaked")
	}
	// A second process claims an epoch, reconstructs the encrypted snapshot and
	// starts a real DB-timed 30-second grace. The old actor must be fenced.
	recovered, err := legacyTestNew(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if err = recovered.Recover(ctx, table); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RequestTopUp(ctx, TopUpCommand{UserID: 910002, Key: "g3-stale-actor-topup", TableID: table, TargetSessionID: session2, AmountUnits: 50 * engine.UnitsPerChip}); err == nil {
		t.Fatal("old actor ignored runtime epoch")
	}
	view, err = recovered.View(ctx, table, 910001)
	if err != nil || !view.Hand.Recovering {
		t.Fatal("recovery not durable", err)
	}
	wait := time.Until(*view.Hand.RecoveryUntil) + 30*time.Millisecond
	if wait > 31*time.Second || wait < 0 {
		t.Fatal("wrong reconnect grace")
	}
	time.Sleep(wait)
	if err = recovered.Tick(ctx, table); err != nil {
		t.Fatal(err)
	}
	view, err = recovered.View(ctx, table, 910001)
	if err != nil || view.Hand.Recovering || view.Hand.ActionDeadlineAt.Sub(view.ServerNow) < 29*time.Second {
		t.Fatal("fresh recovery decision window missing", err)
	}
	if r, err := recovered.RequestTopUp(ctx, TopUpCommand{UserID: 910002, Key: "g3-pending-topup-001", TableID: table, TargetSessionID: session2, AmountUnits: 50 * engine.UnitsPerChip}); err != nil || r.Status != "PENDING" {
		t.Fatal(r, err)
	}
	if r, err := recovered.RequestSafeLeave(ctx, SessionCommand{UserID: 910001, Key: "g3-safe-leave-0001", TableID: table, TargetSessionID: session}); err != nil || r.Status != "LEAVE_AFTER_HAND" {
		t.Fatal(r, err)
	}
	actorUser := int64(910000 + view.Hand.ActorSeat)
	actor, err := recovered.View(ctx, table, actorUser)
	if err != nil {
		t.Fatal(err)
	}
	hv, _ := strconv.ParseUint(actor.Hand.HandVersion, 10, 64)
	ce, _ := strconv.ParseUint(actor.Viewer.ControlEpoch, 10, 64)
	if _, err := legacyAct(t, recovered, ActCommand{UserID: actorUser, Key: "g3-fold-action-0001", TableID: table, HandID: actor.Hand.HandID, ControlEpoch: ce, HandVersion: hv, Kind: engine.Fold}); err != nil {
		t.Fatal(err)
	}
	if _, err := recovered.RequestSafeLeave(ctx, SessionCommand{UserID: 910002, Key: "g3-safe-leave-0002", TableID: table, TargetSessionID: session2}); err != nil {
		t.Fatal(err)
	}
	var walletSum, stackSum, ledgerCount, settlements int64
	if err = owner.QueryRow(ctx, "SELECT sum(balance_units) FROM economy.wallet_balances WHERE newapi_user_id IN(910001,910002)").Scan(&walletSum); err != nil {
		t.Fatal(err)
	}
	if err = owner.QueryRow(ctx, "SELECT coalesce(sum(current_stack_units),0) FROM poker.sessions WHERE table_id=$1", table).Scan(&stackSum); err != nil {
		t.Fatal(err)
	}
	_ = owner.QueryRow(ctx, "SELECT count(*) FROM economy.wallet_ledger WHERE biz_type='POKER_FUNDING'").Scan(&ledgerCount)
	_ = owner.QueryRow(ctx, "SELECT count(*) FROM poker.settlements WHERE hand_id=$1", started.HandID).Scan(&settlements)
	if walletSum != 6000*engine.UnitsPerChip || stackSum != 0 || ledgerCount != 5 || settlements != 1 {
		t.Fatalf("funding closure: wallet=%d stack=%d ledger=%d settlements=%d", walletSum, stackSum, ledgerCount, settlements)
	}
	if commits.Load() == 0 {
		t.Fatal("no post-commit publication")
	}
	proof, err := recovered.Fairness(ctx, started.HandID, 910001)
	if err != nil || proof.Released || proof.ServerSeed != "" || len(proof.Deck) != 0 {
		t.Fatal("premature fairness proof", err)
	}
	if _, err = recovered.Fairness(ctx, started.HandID, 910003); err != ErrDenied {
		t.Fatal("spectator got full-proof endpoint", err)
	}
	// Owner-only historical clock fixture checks release eligibility without
	// waiting a day; the runtime always reads PostgreSQL time, never this test clock.
	if _, err = owner.Exec(ctx, "UPDATE poker.hand_fairness SET full_fairness_reveal_at=clock_timestamp()-interval '1 second' WHERE hand_id=$1", started.HandID); err != nil {
		t.Fatal(err)
	}
	proof, err = recovered.Fairness(ctx, started.HandID, 910001)
	if err != nil || !proof.Released || len(proof.Deck) != 52 || len(proof.ServerSeed) != 64 || len(proof.Contributions) != 2 {
		t.Fatal("participant proof release", err)
	}
	t.Log("create/reserve/duplicate buy-in/two-phase hand/private view/epoch fence/real recovery grace/pending top-up/action/settlement/safe cashout passed; wallet+in-play conserved")
}

func TestRealPGLeaseReplacementAndFundingRollback(t *testing.T) {
	owner, pool := localPokerDB(t)
	ctx := context.Background()
	leases := &fixtureLeases{entries: map[string]leaseEntry{}}
	s, err := legacyTestNew(Options{Pool: pool, Keyring: Keyring{Current: "test", Keys: map[string][]byte{"test": make([]byte, 32)}}, Leases: leases, MailboxCapacity: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	created, err := s.CreateTable(ctx, CreateTableCommand{UserID: 910001, Key: "lease-replace-create", Name: "Lease regression", BlindPreset: "5-10", MaxSeats: 3, AllowSpectators: true})
	if err != nil {
		t.Fatal(err)
	}
	table := created.TableID
	key := "same-client-key-001"
	a, err := s.ReserveSeat(ctx, ReserveSeatCommand{UserID: 910001, Key: key, TableID: table, Seat: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReserveSeat(ctx, ReserveSeatCommand{UserID: 910002, Key: key, TableID: table, Seat: 1}); err != ErrBusy {
		t.Fatal("same client key from another identity acquired owner", err)
	}
	leases.mu.Lock()
	delete(leases.entries, fmt.Sprintf("%s:1", table))
	leases.mu.Unlock()
	b, err := s.ReserveSeat(ctx, ReserveSeatCommand{UserID: 910002, Key: "replacement-owner-001", TableID: table, Seat: 1})
	if err != nil {
		t.Fatal("live lease replacement blocked by stale audit", err)
	}
	r, err := s.BuyIn(ctx, BuyInCommand{UserID: 910001, Key: "old-reservation-buyin", TableID: table, ReservationID: a.ReservationID, AmountUnits: 400 * engine.UnitsPerChip})
	if err != nil || r.Status != "FAILED_NO_EFFECT" || r.FailureCode != "RESERVATION_LEASE_LOST" {
		t.Fatal(r, err)
	}
	retry, err := s.ReserveSeat(ctx, ReserveSeatCommand{UserID: 910001, Key: key, TableID: table, Seat: 1})
	if err != nil || !retry.Duplicate || retry.ReservationID != a.ReservationID {
		t.Fatal("immutable request receipt changed", retry, err)
	}
	// Inject a late ledger failure after the narrow gateway has attempted the
	// wallet update. Its savepoint must roll back all wallet/session effects.
	_, err = owner.Exec(ctx, `CREATE FUNCTION poker.test_ledger_failure() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'SYNTHETIC_LEDGER_FAILURE';END$$; CREATE TRIGGER poker_test_ledger_failure BEFORE INSERT ON economy.wallet_ledger FOR EACH ROW EXECUTE FUNCTION poker.test_ledger_failure();`)
	if err != nil {
		t.Fatal(err)
	}
	buy := BuyInCommand{UserID: 910002, Key: "rollback-late-buyin", TableID: table, ReservationID: b.ReservationID, AmountUnits: 400 * engine.UnitsPerChip}
	failed, err := s.BuyIn(ctx, buy)
	if err != nil || failed.Status != "FAILED_NO_EFFECT" {
		t.Fatal(failed, err)
	}
	var wallet, sessions, ledger int64
	_ = owner.QueryRow(ctx, "SELECT sum(balance_units) FROM economy.wallet_balances").Scan(&wallet)
	_ = owner.QueryRow(ctx, "SELECT count(*) FROM poker.sessions").Scan(&sessions)
	_ = owner.QueryRow(ctx, "SELECT count(*) FROM economy.wallet_ledger").Scan(&ledger)
	if wallet != 9000*engine.UnitsPerChip || sessions != 0 || ledger != 0 {
		t.Fatal("late failure moved value", wallet, sessions, ledger)
	}
	duplicate, err := s.BuyIn(ctx, buy)
	if err != nil || !duplicate.Duplicate || duplicate.FundingID != failed.FundingID {
		t.Fatal("failed receipt not stable", duplicate, err)
	}
	if _, err = owner.Exec(ctx, "DROP TRIGGER poker_test_ledger_failure ON economy.wallet_ledger; DROP FUNCTION poker.test_ledger_failure()"); err != nil {
		t.Fatal(err)
	}
	buy.Key = "rollback-new-buyin"
	r, err = s.BuyIn(ctx, buy)
	if err != nil || r.Status != "CONFIRMED" {
		t.Fatal(r, err)
	}
	leases.mu.Lock()
	_, exists := leases.entries[fmt.Sprintf("%s:1", table)]
	leases.mu.Unlock()
	if exists {
		t.Fatal("committed buy-in retained external lease")
	}
	t.Log("lost lease replacement before PG expiry, old reservation no debit, identity-scoped token, stable receipts and late ledger rollback passed")
}

func TestRealPGLifecycleAndRebuySameSession(t *testing.T) {
	owner, pool := localPokerDB(t)
	ctx := context.Background()
	s, err := legacyTestNew(Options{Pool: pool, Keyring: Keyring{Current: "test", Keys: map[string][]byte{"test": make([]byte, 32)}}, Leases: &fixtureLeases{entries: map[string]leaseEntry{}}, MailboxCapacity: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	created, err := s.CreateTable(ctx, CreateTableCommand{UserID: 910001, Key: "lifecycle-create-001", Name: "Lifecycle regression", BlindPreset: "5-10", MaxSeats: 3, AllowSpectators: true})
	if err != nil {
		t.Fatal(err)
	}
	table := created.TableID
	reserve, err := s.ReserveSeat(ctx, ReserveSeatCommand{UserID: 910001, Key: "lifecycle-reserve-001", TableID: table, Seat: 1})
	if err != nil {
		t.Fatal(err)
	}
	bought, err := s.BuyIn(ctx, BuyInCommand{UserID: 910001, Key: "lifecycle-buyin-001", TableID: table, ReservationID: reserve.ReservationID, AmountUnits: 400 * engine.UnitsPerChip})
	if err != nil || bought.Status != "CONFIRMED" {
		t.Fatal(bought, err)
	}
	c := SessionCommand{UserID: 910001, Key: "lifecycle-sitout-001", TableID: table}
	if _, err = s.RequestSitOut(ctx, legacySession(t, s, c)); err != nil {
		t.Fatal(err)
	}
	v, err := s.ViewForConnection(ctx, legacyRef(t, s, table, 910001))
	if err != nil || v.Seats[0].State != "SIT_OUT" || !v.Viewer.CanResume {
		t.Fatal("sit-out projection", v, err)
	}
	c.Key = "lifecycle-resume-001"
	if _, err = s.ResumeSeat(ctx, legacySession(t, s, c)); err != nil {
		t.Fatal(err)
	}
	v, err = s.View(ctx, table, 910001)
	if err != nil || v.Seats[0].State != "WAITING_BIG_BLIND" {
		t.Fatal("resume skips BB", v, err)
	}
	c.Key = "lifecycle-takeover-001"
	replacement := connected(t, s, fixtureRef(table, 910001), true)
	took, err := s.TakeOver(ctx, TakeOverCommand{RequestID: c.Key, SessionID: bought.SessionID, TargetConnectionID: replacement.Ref.ConnectionID, Auth: replacement.Ref.auth()})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.View(ctx, table, 910001)
	if err != nil || v.Viewer.ControlEpoch != took.Receipt.ControlEpoch {
		t.Fatal("control epoch", v, err)
	}
	if _, err = s.HostControl(ctx, HostCommand{UserID: 910002, Key: "lifecycle-notowner-001", TableID: table, Kind: "PAUSE"}); err != ErrDenied {
		t.Fatal("nonowner controls table", err)
	}
	if _, err = s.HostControl(ctx, HostCommand{UserID: 910001, Key: "lifecycle-pause-001", TableID: table, Kind: "PAUSE"}); err != nil {
		t.Fatal(err)
	}
	// Owner-only setup: persisted post-settlement zero stack. This is a boundary
	// fixture, not a synthetic claim that a losing hand ran in this test.
	if _, err = owner.Exec(ctx, "UPDATE poker.sessions SET current_stack_units=0 WHERE session_id=$1", bought.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.mutate(ctx, table, func(ctx context.Context, tx pgx.Tx, tbl *tableRow) (Receipt, error) {
		return Receipt{Status: "BOUNDARY"}, s.boundary(ctx, tx, tbl)
	}); err != nil {
		t.Fatal(err)
	}
	v, err = s.View(ctx, table, 910001)
	if err != nil || v.Seats[0].State != "REBUY_WINDOW" || v.Seats[0].RebuyDeadlineAt == nil || v.Seats[0].RebuyDeadlineAt.Sub(v.ServerNow) < 59*time.Second {
		t.Fatal("rebuy window", v, err)
	}
	r, err := s.RequestTopUp(ctx, TopUpCommand{UserID: 910001, Key: "lifecycle-rebuy-001", TableID: table, TargetSessionID: bought.SessionID, AmountUnits: 400 * engine.UnitsPerChip})
	if err != nil || r.Status != "CONFIRMED" || r.SessionID != bought.SessionID {
		t.Fatal("rebuy replaced session", r, err)
	}
	v, err = s.View(ctx, table, 910001)
	if err != nil || v.Seats[0].State != "WAITING_BIG_BLIND" || v.Seats[0].RebuyDeadlineAt != nil {
		t.Fatal("rebuy entry", v, err)
	}
	if _, err = s.HostControl(ctx, HostCommand{UserID: 910001, Key: "lifecycle-close-001", TableID: table, Kind: "CLOSE"}); err != nil {
		t.Fatal(err)
	}
	v, err = s.View(ctx, table, 910001)
	if err != nil || v.LifecycleState != "CLOSED" || len(v.Seats) != 0 {
		t.Fatal("safe close", v, err)
	}
	t.Log("explicit sit-out/resume WAIT_FOR_BB, control takeover, host authority, 60s rebuy same-session and safe host close passed")
}

func TestRealPGBuyInLeaseLossWhileWalletLocked(t *testing.T) {
	owner, pool := localPokerDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	leases := &fixtureLeases{entries: map[string]leaseEntry{}}
	s, err := legacyTestNew(Options{Pool: pool, Keyring: Keyring{Current: "test", Keys: map[string][]byte{"test": make([]byte, 32)}}, Leases: leases, MailboxCapacity: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	created, err := s.CreateTable(ctx, CreateTableCommand{UserID: 910001, Key: "wallet-wait-create", Name: "Wallet wait", BlindPreset: "5-10", MaxSeats: 2, AllowSpectators: true})
	if err != nil {
		t.Fatal(err)
	}
	table := created.TableID
	reservation, err := s.ReserveSeat(ctx, ReserveSeatCommand{UserID: 910001, Key: "wallet-wait-reserve", TableID: table, Seat: 1})
	if err != nil {
		t.Fatal(err)
	}
	held, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(held)
	if _, err = held.Exec(ctx, "SELECT balance_units FROM economy.wallet_balances WHERE newapi_user_id=910001 AND asset_type='AVAILABLE_CHIPS' FOR UPDATE"); err != nil {
		t.Fatal(err)
	}
	result := make(chan actorReply, 1)
	go func() {
		r, e := s.BuyIn(ctx, BuyInCommand{UserID: 910001, Key: "wallet-wait-buyin", TableID: table, ReservationID: reservation.ReservationID, AmountUnits: 400 * engine.UnitsPerChip})
		result <- actorReply{r, e}
	}()
	blocked := false
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		if err = owner.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND cardinality(pg_blocking_pids(pid))>0)", pool.Config().ConnConfig.User).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !blocked {
		t.Fatal("buy-in did not reach real wallet lock wait")
	}
	leases.mu.Lock()
	delete(leases.entries, fmt.Sprintf("%s:1", table))
	leases.mu.Unlock()
	if err = held.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	r := <-result
	if r.err != nil || r.receipt.Status != "FAILED_NO_EFFECT" || r.receipt.FailureCode != "RESERVATION_LEASE_LOST" {
		t.Fatal("lease vanished during wallet wait yet funded", r)
	}
	var balance, sessions, ledger int64
	if err = owner.QueryRow(ctx, "SELECT balance_units FROM economy.wallet_balances WHERE newapi_user_id=910001 AND asset_type='AVAILABLE_CHIPS'").Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if err = owner.QueryRow(ctx, "SELECT count(*) FROM poker.sessions").Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if err = owner.QueryRow(ctx, "SELECT count(*) FROM economy.wallet_ledger").Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if balance != 3000*engine.UnitsPerChip || sessions != 0 || ledger != 0 {
		t.Fatal("post-lock lease failure moved value", balance, sessions, ledger)
	}
	t.Log("real wallet lock wait, external lease loss, post-lock validation rollback and no wallet/ledger/session effects passed")
}

func TestRealPGAllSitOutResumeStartsNextHand(t *testing.T) {
	_, pool := localPokerDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s, err := legacyTestNew(Options{Pool: pool, Keyring: Keyring{Current: "test", Keys: map[string][]byte{"test": make([]byte, 32)}}, Leases: &fixtureLeases{entries: map[string]leaseEntry{}}, MailboxCapacity: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	created, err := s.CreateTable(ctx, CreateTableCommand{UserID: 910001, Key: "cohort-create-table", Name: "Cohort resume", BlindPreset: "5-10", MaxSeats: 3, AllowSpectators: true})
	if err != nil {
		t.Fatal(err)
	}
	table := created.TableID
	for seat := 1; seat <= 2; seat++ {
		user := int64(910000 + seat)
		r, e := s.ReserveSeat(ctx, ReserveSeatCommand{UserID: user, Key: fmt.Sprintf("cohort-reserve-%04d", seat), TableID: table, Seat: seat})
		if e != nil {
			t.Fatal(e)
		}
		b, e := s.BuyIn(ctx, BuyInCommand{UserID: user, Key: fmt.Sprintf("cohort-buyin-%04d", seat), TableID: table, ReservationID: r.ReservationID, AmountUnits: 400 * engine.UnitsPerChip})
		if e != nil || b.Status != "CONFIRMED" {
			t.Fatal(b, e)
		}
	}
	legacyRef(t, s, table, 910001)
	legacyRef(t, s, table, 910002)
	first, err := s.StartHand(ctx, StartHandCommand{UserID: 910001, Key: "cohort-first-hand", TableID: table})
	if err != nil {
		t.Fatal(err)
	}
	exportSyntheticViews(t, ctx, s, table)
	for user := int64(910001); user <= 910002; user++ {
		if _, err = s.RequestSitOut(ctx, legacySession(t, s, SessionCommand{UserID: user, Key: fmt.Sprintf("cohort-sitout-%d", user), TableID: table})); err != nil {
			t.Fatal(err)
		}
	}
	v, err := s.View(ctx, table, 910001)
	if err != nil {
		t.Fatal(err)
	}
	previousButton := v.Hand.ButtonSeat
	user := int64(910000 + v.Hand.ActorSeat)
	v, err = s.View(ctx, table, user)
	if err != nil {
		t.Fatal(err)
	}
	hv, _ := strconv.ParseUint(v.Hand.HandVersion, 10, 64)
	ce, _ := strconv.ParseUint(v.Viewer.ControlEpoch, 10, 64)
	if _, err = legacyAct(t, s, ActCommand{UserID: user, Key: "cohort-fold-first", TableID: table, HandID: first.HandID, HandVersion: hv, ControlEpoch: ce, Kind: engine.Fold}); err != nil {
		t.Fatal(err)
	}
	v, err = s.View(ctx, table, 910001)
	if err != nil {
		t.Fatal(err)
	}
	for _, seat := range v.Seats {
		if seat.State != "SIT_OUT" {
			t.Fatal("cohort not sitting out", seat.State)
		}
	}
	for user := int64(910001); user <= 910002; user++ {
		if _, err = s.ResumeSeat(ctx, legacySession(t, s, SessionCommand{UserID: user, Key: fmt.Sprintf("cohort-resume-%d", user), TableID: table})); err != nil {
			t.Fatal(err)
		}
	}
	v, err = s.View(ctx, table, 910001)
	if err != nil {
		t.Fatal(err)
	}
	if v.IntermissionUntil == nil {
		t.Fatal("missing intermission")
	}
	wait := time.Until(*v.IntermissionUntil) + 40*time.Millisecond
	if wait > 0 {
		time.Sleep(wait)
	}
	if err = s.Tick(ctx, table); err != nil {
		t.Fatal(err)
	}
	v, err = s.View(ctx, table, 910001)
	if err != nil || v.Hand.HandID == first.HandID || v.Hand.Street != "PREFLOP" || v.Hand.ButtonSeat == previousButton || v.Hand.PotUnits != decimal(15*engine.UnitsPerChip) {
		t.Fatal("all-ready cohort stayed stuck or introduced entry bets", v.Hand, err)
	}
	t.Log("all players sit out, explicitly resume, real 5s boundary, clockwise next hand and only normal SB+BB passed")
}

func exportSyntheticViews(t *testing.T, ctx context.Context, s *Service, table string) {
	t.Helper()
	dir := os.Getenv("POKER_TEST_VIEW_OUTPUT_DIR")
	if dir == "" {
		return
	}
	if !filepath.IsAbs(dir) {
		t.Fatal("view artifact directory must be absolute")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	public, err := s.View(ctx, table, 910003)
	if err != nil {
		t.Fatal(err)
	}
	player, err := s.View(ctx, table, int64(910000+public.Hand.ActorSeat))
	if err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]TableView{"public.json": public, "player-self.json": player} {
		body, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, name), append(body, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Log("exported actual service TableView JSON from isolated synthetic PG participants; no seed/deck/credentials included")
}
