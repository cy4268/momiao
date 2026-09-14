package poker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker/engine"
	"github.com/cy4268/momiao/internal/poker/redislease"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func TestControlSessionMutationRequiresSocketAuthority(t *testing.T) {
	_, pool := localPokerDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s, err := New(Options{Pool: pool, Keyring: Keyring{Current: "test", Keys: map[string][]byte{"test": make([]byte, 32)}}, Leases: &fixtureLeases{entries: map[string]leaseEntry{}}, MailboxCapacity: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	table, err := s.CreateTable(ctx, CreateTableCommand{UserID: 910001, Key: "control-red-table", Name: "Control regression", BlindPreset: "5-10", MaxSeats: 2, AllowSpectators: true})
	if err != nil {
		t.Fatal(err)
	}
	reservation, err := s.ReserveSeat(ctx, ReserveSeatCommand{UserID: 910001, Key: "control-red-reserve", TableID: table.TableID, Seat: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.BuyIn(ctx, BuyInCommand{UserID: 910001, Key: "control-red-buyin", TableID: table.TableID, ReservationID: reservation.ReservationID, AmountUnits: 400 * engine.UnitsPerChip}); err != nil {
		t.Fatal(err)
	}
	if receipt, err := s.RequestSitOut(ctx, SessionCommand{UserID: 910001, Key: "control-red-no-socket", TableID: table.TableID}); err == nil {
		t.Fatalf("UserID-only SessionCommand changed state without socket/control authority: %+v", receipt)
	}
}

type controlFaultStore struct {
	ControlStore
	failAssign atomic.Bool
}

func (s *controlFaultStore) ControlAssign(ctx context.Context, id, old, next string) (bool, error) {
	if s.failAssign.Load() {
		return false, errors.New("synthetic assignment failure after PG commit")
	}
	return s.ControlStore.ControlAssign(ctx, id, old, next)
}

type controlFixture struct {
	owner, pool *pgxpool.Pool
	service     *Service
	store       *redislease.Store
	faults      *controlFaultStore
	admin       *redis.Client
	username    string
	keys        []string
	revoked     atomic.Bool
	notices     atomic.Int64
}

func newControlFixture(t *testing.T) *controlFixture {
	t.Helper()
	addr := os.Getenv("POKER_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set POKER_TEST_REDIS_ADDR for real PG+Redis control regression")
	}
	if addr != "127.0.0.1:16379" || os.Getenv("POKER_TEST_REDIS_TLS_ADDR") != "127.0.0.1:16380" || os.Getenv("POKER_TEST_REDIS_CONFIRM") != "owned-local-fixture" {
		t.Fatal("explicit owned Redis fixture required")
	}
	dir := os.Getenv("POKER_TEST_REDIS_FIXTURE_DIR")
	if !filepath.IsAbs(dir) {
		t.Fatal("absolute fixture path required")
	}
	secret, err := os.ReadFile(filepath.Join(dir, "admin-secret.txt"))
	if err != nil {
		t.Fatal("fixture credential unavailable")
	}
	cert, err := os.ReadFile(filepath.Join(dir, "server.crt"))
	if err != nil {
		t.Fatal("fixture trust unavailable")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(cert) {
		t.Fatal("fixture trust invalid")
	}
	owner, pool := localPokerDB(t)
	f := &controlFixture{owner: owner, pool: pool, username: "g3-control-" + uuid()}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	f.admin = redis.NewClient(&redis.Options{Addr: addr, Username: "g2-admin", Password: string(secret), Protocol: 2, MaxRetries: -1, DisableIdentity: true, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second})
	if f.admin.Ping(ctx).Err() != nil {
		t.Fatal("real Redis fixture unreachable")
	}
	var bytes [32]byte
	if _, err = rand.Read(bytes[:]); err != nil {
		t.Fatal(err)
	}
	password := hex.EncodeToString(bytes[:])
	hash := sha256.Sum256([]byte(password))
	if err = f.admin.Do(ctx, "ACL", "SETUSER", f.username, "reset", "on", "#"+hex.EncodeToString(hash[:]), "+ping", "+hello", "+eval", "+time", "+get", "+pttl", "+set", "+del").Err(); err != nil {
		t.Fatal("isolated ACL creation failed")
	}
	t.Cleanup(func() {
		if f.service != nil {
			f.service.Close()
		}
		c, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		for _, key := range f.keys {
			if strings.HasPrefix(key, redislease.ControlKeyPrefix) {
				id := strings.TrimPrefix(key, redislease.ControlKeyPrefix)
				value, e := f.store.ControlLoad(c, id)
				if e != nil {
					t.Error("own control cleanup read failed")
					continue
				}
				if value != "" {
					if _, e = f.store.ControlRelease(c, id, value); e != nil {
						t.Error("own control cleanup release failed")
					}
				}
				if value, e = f.store.ControlLoad(c, id); e != nil || value != "" {
					t.Error("control key absence unverified")
				}
			} else {
				if e := f.admin.Del(c, key).Err(); e != nil {
					t.Error("own seat cleanup failed")
				}
				if n, e := f.admin.Exists(c, key).Result(); e != nil || n != 0 {
					t.Error("own seat absence unverified")
				}
			}
		}
		if f.store != nil {
			_ = f.store.Close()
		}
		if e := f.admin.Do(c, "ACL", "DELUSER", f.username).Err(); e != nil {
			t.Error("control ACL cleanup failed")
		}
		if v, e := f.admin.Do(c, "ACL", "GETUSER", f.username).Result(); e != redis.Nil && (e != nil || v != nil) {
			t.Error("control ACL absence unverified")
		}
		_ = f.admin.Close()
	})
	f.store, err = redislease.Open(ctx, redislease.Config{Addr: "127.0.0.1:16380", Username: f.username, Password: password, TLSConfig: &tls.Config{RootCAs: roots, ServerName: "localhost", MinVersion: tls.VersionTLS12}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	f.faults = &controlFaultStore{ControlStore: f.store}
	f.service, err = New(f.options())
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *controlFixture) options() Options {
	return Options{Pool: f.pool, Keyring: Keyring{Current: "control-test", Keys: map[string][]byte{"control-test": make([]byte, 32)}}, Leases: f.store, Controls: f.faults, MailboxCapacity: 8,
		ValidateSession: func(_ context.Context, a AuthSession) error {
			if f.revoked.Load() || a.UserID < 910001 || a.UserID > 910003 || a.SessionVersion != 1 || a.SecurityEpoch != 1 {
				return ErrDenied
			}
			return nil
		},
		AfterControlChanged: func(string, string) { f.notices.Add(1) }}
}
func (f *controlFixture) table(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	r, e := f.service.CreateTable(ctx, CreateTableCommand{UserID: 910001, Key: "control-table-" + uuid(), Name: "Actual PG Redis control", BlindPreset: "5-10", MaxSeats: 2, AllowSpectators: true})
	if e != nil {
		t.Fatal(e)
	}
	for _, seat := range []int{1, 2} {
		key := redislease.KeyPrefix + r.TableID + fmt.Sprintf(":%d", seat)
		f.keys = append(f.keys, key)
		if e = f.admin.Do(ctx, "ACL", "SETUSER", f.username, "~"+key).Err(); e != nil {
			t.Fatal("exact seat ACL failed")
		}
	}
	return r.TableID
}
func (f *controlFixture) buy(t *testing.T, table string, user int64, seat int) string {
	t.Helper()
	ctx := context.Background()
	r, e := f.service.ReserveSeat(ctx, ReserveSeatCommand{UserID: user, Key: "control-reserve-" + uuid(), TableID: table, Seat: seat})
	if e != nil {
		t.Fatal(e)
	}
	b, e := f.service.BuyIn(ctx, BuyInCommand{UserID: user, Key: "control-buy-" + uuid(), TableID: table, ReservationID: r.ReservationID, AmountUnits: 400 * engine.UnitsPerChip})
	if e != nil || b.Status != "CONFIRMED" {
		t.Fatal(b, e)
	}
	key := redislease.ControlKeyPrefix + b.SessionID
	f.keys = append(f.keys, key)
	if e = f.admin.Do(ctx, "ACL", "SETUSER", f.username, "~"+key).Err(); e != nil {
		t.Fatal("exact control ACL failed")
	}
	return b.SessionID
}
func fixtureRef(table string, user int64) ControlRef {
	hash := sha256.Sum256([]byte(fmt.Sprintf("synthetic-auth-%d", user)))
	return ControlRef{UserID: user, SessionIDHash: hex.EncodeToString(hash[:]), SessionVersion: 1, SecurityEpoch: 1, ConnectionID: strings.ReplaceAll(uuid(), "-", ""), TableID: table}
}
func connected(t *testing.T, s *Service, ref ControlRef, claim bool) ControlResult {
	t.Helper()
	r, e := s.Connected(context.Background(), ref, claim)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func controlledSession(t *testing.T, f *controlFixture, ref ControlRef) SessionCommand {
	t.Helper()
	var tableVersion, handVersion uint64
	var hand string
	e := f.owner.QueryRow(context.Background(), `SELECT t.table_version,coalesce(t.current_hand_id::text,''),coalesce(h.hand_version,0) FROM poker.tables t LEFT JOIN poker.hands h ON h.hand_id=t.current_hand_id WHERE t.table_id=$1`, ref.TableID).Scan(&tableVersion, &hand, &handVersion)
	if e != nil {
		t.Fatal(e)
	}
	return SessionCommand{UserID: ref.UserID, TableID: ref.TableID, Key: "control-session-" + uuid(), Control: &ref, ExpectedTableVersion: tableVersion, ExpectedHandVersion: handVersion, HandID: hand}
}
func TestControlRealPGRedisTwoConnectionsAndTakeOver(t *testing.T) {
	f := newControlFixture(t)
	table := f.table(t)
	session := f.buy(t, table, 910001, 1)
	ctx := context.Background()
	a := connected(t, f.service, fixtureRef(table, 910001), true)
	if a.Control.Mode != "CONTROLLER" {
		t.Fatal("first legal claim not controller", a.Control)
	}
	b := connected(t, f.service, fixtureRef(table, 910001), true)
	if b.Control.Mode != "READ_ONLY" {
		t.Fatal("second claim not readonly", b.Control)
	}
	stolen := b.Ref
	stolen.ControlEpoch = a.Ref.ControlEpoch
	if e := f.service.AuthorizeControl(ctx, stolen); e == nil {
		t.Fatal("epoch possession alone became authority")
	}
	readTicket := connected(t, f.service, fixtureRef(table, 910001), false)
	if _, e := f.service.TakeOver(ctx, TakeOverCommand{RequestID: "read-ticket-takeover", SessionID: session, TargetConnectionID: readTicket.Ref.ConnectionID, Auth: readTicket.Ref.auth()}); e == nil {
		t.Fatal("READ_ONLY ticket silently upgraded")
	}
	takeover := TakeOverCommand{RequestID: "actual-takeover-01", SessionID: session, TargetConnectionID: b.Ref.ConnectionID, Auth: b.Ref.auth()}
	took, e := f.service.TakeOver(ctx, takeover)
	if e != nil || took.Control.Mode != "CONTROLLER" || took.Ref.ControlEpoch != a.Ref.ControlEpoch+1 {
		t.Fatal("takeover did not fence", took.Control, e)
	}
	if e = f.service.AuthorizeControl(ctx, a.Ref); e == nil {
		t.Fatal("old owner still valid")
	}
	if e = f.service.Disconnected(ctx, a.Ref); e != nil {
		t.Fatal(e)
	}
	var online bool
	if e = f.owner.QueryRow(ctx, "SELECT connected FROM poker.seats WHERE table_id=$1 AND seat_no=1", table).Scan(&online); e != nil || !online {
		t.Fatal("old disconnect disconnected replacement", e)
	}
	duplicate, e := f.service.TakeOver(ctx, takeover)
	if e != nil || duplicate.Receipt == nil || !duplicate.Receipt.Duplicate || duplicate.Receipt.ControlEpoch != took.Receipt.ControlEpoch {
		t.Fatal("takeover retry lost immutable receipt", duplicate.Control, e)
	}
	var audits, epoch int64
	if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM poker.audit_events").Scan(&audits); e != nil {
		t.Fatal(e)
	}
	if e = f.owner.QueryRow(ctx, "SELECT control_epoch FROM poker.sessions WHERE session_id=$1", session).Scan(&epoch); e != nil {
		t.Fatal(e)
	}
	if audits != 2 || uint64(epoch) != took.Ref.ControlEpoch {
		t.Fatal("duplicate changed control facts", audits, epoch)
	}
	// This is the TOCTOU boundary: preflight succeeds, another connection takes
	// over, and the already-authorized typed command then reaches the PG actor.
	command := controlledSession(t, f, took.Ref)
	if e = f.service.AuthorizeControl(ctx, took.Ref); e != nil {
		t.Fatal(e)
	}
	c := connected(t, f.service, fixtureRef(table, 910001), true)
	if _, e = f.service.TakeOver(ctx, TakeOverCommand{RequestID: "actual-takeover-02", SessionID: session, TargetConnectionID: c.Ref.ConnectionID, Auth: c.Ref.auth()}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.service.RequestSitOut(ctx, command); e == nil {
		t.Fatal("sit-out passed stale preflight")
	}
	command.Key = "stale-resume-control"
	if _, e = f.service.ResumeSeat(ctx, command); e == nil {
		t.Fatal("resume passed stale preflight")
	}
	command.Key = "stale-seed-control"
	if _, e = f.service.SetNextSeed(ctx, NextSeedCommand{SessionCommand: command, Contribution: "synthetic next seed"}); e == nil {
		t.Fatal("seed passed stale preflight")
	}
	var state string
	if e = f.owner.QueryRow(ctx, "SELECT state FROM poker.seats WHERE table_id=$1 AND seat_no=1", table).Scan(&state); e != nil || state != "ACTIVE" {
		t.Fatal("stale commands changed seat", state, e)
	}
}

func TestControlRealPGRedisCommitThenAssignmentFailure(t *testing.T) {
	f := newControlFixture(t)
	table := f.table(t)
	session := f.buy(t, table, 910001, 1)
	ctx := context.Background()
	a := connected(t, f.service, fixtureRef(table, 910001), true)
	b := connected(t, f.service, fixtureRef(table, 910001), true)
	f.faults.failAssign.Store(true)
	command := TakeOverCommand{RequestID: "commit-before-redis-01", SessionID: session, TargetConnectionID: b.Ref.ConnectionID, Auth: b.Ref.auth()}
	pending, e := f.service.TakeOver(ctx, command)
	if e != nil || pending.Control.Mode != "READ_ONLY" || pending.Receipt == nil || pending.Receipt.Status != "CONTROL_EPOCH_ADVANCED" {
		t.Fatal("uncertain assignment pretended no PG effect", pending.Control, e)
	}
	if e = f.service.AuthorizeControl(ctx, a.Ref); e == nil {
		t.Fatal("old PG epoch still accepted")
	}
	if e = f.service.AuthorizeControl(ctx, pending.Ref); e == nil {
		t.Fatal("new owner granted without Redis")
	}
	f.faults.failAssign.Store(false)
	retried, e := f.service.TakeOver(ctx, command)
	if e != nil || retried.Control.Mode != "CONTROLLER" || retried.Receipt == nil || !retried.Receipt.Duplicate {
		t.Fatal("committed intent not reconciled", retried.Control, e)
	}
	if retried.Ref.ControlEpoch != pending.Ref.ControlEpoch {
		t.Fatal("retry advanced epoch again")
	}
	// The REQUIRED online auth port revokes otherwise-live TCP identities. It
	// removes control, never the durable Poker Session or its funds.
	f.revoked.Store(true)
	if _, e = f.service.RenewControl(ctx, retried.Ref); e == nil {
		t.Fatal("revoked auth was renewed")
	}
	if e = f.service.AuthorizeControl(ctx, retried.Ref); e == nil {
		t.Fatal("revoked auth kept action authority")
	}
	var stack, ledger int64
	var status string
	if e = f.owner.QueryRow(ctx, "SELECT current_stack_units,state FROM poker.sessions WHERE session_id=$1", session).Scan(&stack, &status); e != nil {
		t.Fatal(e)
	}
	if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM economy.wallet_ledger").Scan(&ledger); e != nil {
		t.Fatal(e)
	}
	if stack != 400*engine.UnitsPerChip || status != "ACTIVE" || ledger != 1 {
		t.Fatal("auth revoke moved funds", stack, status, ledger)
	}
	if f.notices.Load() < 3 {
		t.Fatal("ephemeral control change did not wake authoritative projection")
	}
	// Transport only retains authenticated identity plus its server-generated
	// connection ID; cached durable epochs may already be cleared by revocation.
	basic := retried.Ref
	basic.SessionID = ""
	basic.RuntimeEpoch = 0
	basic.ControlEpoch = 0
	if e = f.service.Disconnected(ctx, basic); e != nil {
		t.Fatal("basic identity disconnect", e)
	}
	var online bool
	if e = f.owner.QueryRow(ctx, "SELECT connected FROM poker.seats WHERE table_id=$1 AND seat_no=1", table).Scan(&online); e != nil || online {
		t.Fatal("revoked owner disconnect left seat connected", e)
	}
}
