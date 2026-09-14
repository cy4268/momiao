package poker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker/engine"
	"github.com/cy4268/momiao/internal/poker/redislease"
	"github.com/redis/go-redis/v9"
)

func TestRealPGRedisWalletWaitAndCashClosure(t *testing.T) {
	addr := os.Getenv("POKER_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set POKER_TEST_REDIS_ADDR for the owned PG+Redis fixture")
	}
	if addr != "127.0.0.1:16379" || os.Getenv("POKER_TEST_REDIS_TLS_ADDR") != "127.0.0.1:16380" || os.Getenv("POKER_TEST_REDIS_CONFIRM") != "owned-local-fixture" {
		t.Fatal("explicit owned Redis fixture required")
	}
	fixture := os.Getenv("POKER_TEST_REDIS_FIXTURE_DIR")
	if !filepath.IsAbs(fixture) {
		t.Fatal("absolute Redis fixture directory required")
	}
	secret, err := os.ReadFile(filepath.Join(fixture, "admin-secret.txt"))
	if err != nil {
		t.Fatal("fixture secret unavailable")
	}
	cert, err := os.ReadFile(filepath.Join(fixture, "server.crt"))
	if err != nil {
		t.Fatal("fixture certificate unavailable")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(cert) {
		t.Fatal("fixture certificate malformed")
	}
	owner, pool := localPokerDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin := redis.NewClient(&redis.Options{Addr: addr, Username: "g2-admin", Password: string(secret), Protocol: 2, MaxRetries: -1, DisableIdentity: true, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second})
	defer admin.Close()
	if admin.Ping(ctx).Err() != nil {
		t.Fatal("owned Redis fixture not reachable")
	}
	username := "g3-pgredis-" + uuid()
	var random [32]byte
	if _, err = rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	password := hex.EncodeToString(random[:])
	hash := sha256.Sum256([]byte(password))
	if err = admin.Do(ctx, "ACL", "SETUSER", username, "reset", "on", "#"+hex.EncodeToString(hash[:]), "+ping", "+hello", "+eval", "+time", "+get", "+pttl", "+set", "+del").Err(); err != nil {
		t.Fatal("isolated runtime Redis ACL creation failed")
	}
	var keys []string
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		cleanupAdmin := redis.NewClient(&redis.Options{Addr: addr, Username: "g2-admin", Password: string(secret), Protocol: 2, DisableIdentity: true, MaxRetries: -1})
		defer cleanupAdmin.Close()
		if len(keys) > 0 {
			if err := cleanupAdmin.Del(cleanup, keys...).Err(); err != nil {
				t.Error("own Redis key cleanup failed")
			}
			if n, err := cleanupAdmin.Exists(cleanup, keys...).Result(); err != nil || n != 0 {
				t.Error("own Redis key absence not verified")
			}
		}
		if err := cleanupAdmin.Do(cleanup, "ACL", "DELUSER", username).Err(); err != nil {
			t.Error("own Redis ACL cleanup failed")
		}
		if found, err := cleanupAdmin.Do(cleanup, "ACL", "GETUSER", username).Result(); err != redis.Nil && (err != nil || found != nil) {
			t.Error("own Redis ACL absence not verified")
		}
	})
	s, err := NewWithRedis(ctx, Options{Pool: pool, Keyring: Keyring{Current: "test", Keys: map[string][]byte{"test": make([]byte, 32)}}, MailboxCapacity: 8, ValidateSession: fixtureAuth}, redislease.Config{Addr: os.Getenv("POKER_TEST_REDIS_TLS_ADDR"), Username: username, Password: password, TLSConfig: &tls.Config{RootCAs: roots, ServerName: "localhost", MinVersion: tls.VersionTLS12}, Timeout: time.Second})
	if err != nil {
		t.Fatal("real Redis service composition failed", err)
	}
	defer s.Close()
	var controlSessions []string
	sessionsByUser := map[int64]string{}
	controlCleaned := false
	cleanControl := func() {
		if controlCleaned {
			return
		}
		controlCleaned = true
		c, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		for _, id := range controlSessions {
			value, e := s.opts.Controls.ControlLoad(c, id)
			if e != nil {
				t.Error("own control cleanup read", e)
				continue
			}
			if value != "" {
				if _, e = s.opts.Controls.ControlRelease(c, id, value); e != nil {
					t.Error("own control cleanup release", e)
				}
			}
			if value, e = s.opts.Controls.ControlLoad(c, id); e != nil || value != "" {
				t.Error("own control absence unverified")
			}
		}
	}
	defer cleanControl()
	created, err := s.CreateTable(ctx, CreateTableCommand{UserID: 910001, Key: "real-redis-table-001", Name: "Real PG Redis friends", BlindPreset: "5-10", MaxSeats: 2, AllowSpectators: true})
	if err != nil {
		t.Fatal(err)
	}
	table := created.TableID
	keys = []string{redislease.KeyPrefix + table + ":1", redislease.KeyPrefix + table + ":2"}
	if err = admin.Do(ctx, "ACL", "SETUSER", username, "resetkeys", "~"+redislease.KeyPrefix+table+":*").Err(); err != nil {
		t.Fatal("restrict Redis runtime to exact synthetic table")
	}
	reserved, err := s.ReserveSeat(ctx, ReserveSeatCommand{UserID: 910001, Key: "real-redis-reserve-a", TableID: table, Seat: 1})
	if err != nil {
		t.Fatal(err)
	}
	ttl, err := admin.PTTL(ctx, keys[0]).Result()
	if err != nil || ttl <= 0 || ttl > 30*time.Second {
		t.Fatal("actual Redis lease TTL invalid")
	}
	held, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(held)
	if _, err = held.Exec(ctx, "SELECT balance_units FROM economy.wallet_balances WHERE newapi_user_id=910001 AND asset_type='AVAILABLE_CHIPS' FOR UPDATE"); err != nil {
		t.Fatal(err)
	}
	buy := BuyInCommand{UserID: 910001, Key: "real-redis-wait-buy", TableID: table, ReservationID: reserved.ReservationID, AmountUnits: 400 * engine.UnitsPerChip}
	result := make(chan actorReply, 1)
	go func() { r, e := s.BuyIn(ctx, buy); result <- actorReply{r, e} }()
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
		t.Fatal("runtime never reached the real wallet lock wait")
	}
	if removed, err := admin.Del(ctx, keys[0]).Result(); err != nil || removed != 1 {
		t.Fatal("targeted actual Redis lease loss not injected")
	}
	if err = held.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	failed := <-result
	if failed.err != nil || failed.receipt.Status != "FAILED_NO_EFFECT" || failed.receipt.FailureCode != "RESERVATION_LEASE_LOST" {
		t.Fatal("post-wallet-lock real Redis loss funded", failed)
	}
	var wallet, sessions, ledger int64
	if err = owner.QueryRow(ctx, "SELECT sum(balance_units) FROM economy.wallet_balances").Scan(&wallet); err != nil {
		t.Fatal(err)
	}
	if err = owner.QueryRow(ctx, "SELECT count(*) FROM poker.sessions").Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if err = owner.QueryRow(ctx, "SELECT count(*) FROM economy.wallet_ledger").Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if wallet != 9000*engine.UnitsPerChip || sessions != 0 || ledger != 0 {
		t.Fatal("real Redis loss moved wallet/session/ledger value", wallet, sessions, ledger)
	}
	duplicate, err := s.BuyIn(ctx, buy)
	if err != nil || !duplicate.Duplicate || duplicate.FundingID != failed.receipt.FundingID {
		t.Fatal("failed buy-in lost durable idempotency", duplicate, err)
	}
	// The prior PG audit is still within 30 seconds; a different live owner can
	// reserve and fund without treating that audit as a durable seat lock.
	for _, entry := range []struct {
		user int64
		seat int
	}{{910002, 1}, {910001, 2}} {
		r, e := s.ReserveSeat(ctx, ReserveSeatCommand{UserID: entry.user, Key: fmt.Sprintf("real-redis-new-%d", entry.user), TableID: table, Seat: entry.seat})
		if e != nil {
			t.Fatal(e)
		}
		b, e := s.BuyIn(ctx, BuyInCommand{UserID: entry.user, Key: fmt.Sprintf("real-redis-buy-%d", entry.user), TableID: table, ReservationID: r.ReservationID, AmountUnits: 400 * engine.UnitsPerChip})
		if e != nil || b.Status != "CONFIRMED" {
			t.Fatal(b, e)
		}
		controlSessions = append(controlSessions, b.SessionID)
		sessionsByUser[entry.user] = b.SessionID
		if e = admin.Do(ctx, "ACL", "SETUSER", username, "~"+redislease.ControlKeyPrefix+b.SessionID).Err(); e != nil {
			t.Fatal("exact control ACL failed")
		}
	}
	if n, err := admin.Exists(ctx, keys...).Result(); err != nil || n != 0 {
		t.Fatal("successful commit left live external owners")
	}
	legacyRef(t, s, table, 910001)
	legacyRef(t, s, table, 910002)
	started, err := s.StartHand(ctx, SessionCommand{UserID: 910001, Key: "real-redis-hand-001", TableID: table})
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.View(ctx, table, 910001)
	if err != nil {
		t.Fatal(err)
	}
	actor := int64(910001)
	if v.Hand.ActorSeat == 1 {
		actor = 910002
	}
	v, err = s.View(ctx, table, actor)
	if err != nil {
		t.Fatal(err)
	}
	hv, _ := strconv.ParseUint(v.Hand.HandVersion, 10, 64)
	ce, _ := strconv.ParseUint(v.Viewer.ControlEpoch, 10, 64)
	if _, err = legacyAct(t, s, ActCommand{UserID: actor, Key: "real-redis-fold-001", TableID: table, HandID: started.HandID, HandVersion: hv, ControlEpoch: ce, Kind: engine.Fold}); err != nil {
		t.Fatal(err)
	}
	for _, user := range []int64{910001, 910002} {
		if _, err = s.RequestSafeLeave(ctx, SessionCommand{UserID: user, Key: fmt.Sprintf("real-redis-leave-%d", user), TableID: table, TargetSessionID: sessionsByUser[user]}); err != nil {
			t.Fatal(err)
		}
	}
	var stacks, settlements int64
	if err = owner.QueryRow(ctx, "SELECT sum(balance_units) FROM economy.wallet_balances WHERE newapi_user_id IN(910001,910002)").Scan(&wallet); err != nil {
		t.Fatal(err)
	}
	if err = owner.QueryRow(ctx, "SELECT coalesce(sum(current_stack_units),0) FROM poker.sessions WHERE table_id=$1", table).Scan(&stacks); err != nil {
		t.Fatal(err)
	}
	if err = owner.QueryRow(ctx, "SELECT count(*) FROM economy.wallet_ledger").Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if err = owner.QueryRow(ctx, "SELECT count(*) FROM poker.settlements").Scan(&settlements); err != nil {
		t.Fatal(err)
	}
	if wallet != 6000*engine.UnitsPerChip || stacks != 0 || ledger != 4 || settlements != 1 {
		t.Fatal("real PG Redis cash closure", wallet, stacks, ledger, settlements)
	}
	version := ""
	info, err := admin.Info(ctx, "server").Result()
	if err != nil {
		t.Fatal("Redis version observation failed")
	}
	for _, line := range strings.Split(info, "\n") {
		if strings.HasPrefix(line, "redis_version:") {
			version = strings.TrimSpace(strings.TrimPrefix(line, "redis_version:"))
		}
	}
	if path := os.Getenv("POKER_TEST_REDIS_EVIDENCE"); path != "" {
		if !filepath.IsAbs(path) {
			t.Fatal("absolute evidence path required")
		}
		body, _ := json.MarshalIndent(map[string]any{"real_postgresql": true, "real_redis_version": version, "verified_tls": true, "runtime_acl_exact_table_only": true, "observed_wallet_lock_wait": true, "deleted_real_lease_during_wait": true, "failed_effects_zero": true, "failed_receipt_stable": true, "replacement_lease_before_audit_expiry": true, "cash_closure_wallet_units": wallet, "cash_closure_stack_units": stacks, "ledger_entries": ledger, "settlements": settlements}, "", "  ")
		if err = os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	cleanControl()
	s.Close()
	if _, err = s.opts.Leases.Valid(ctx, table, 1, strings.Repeat("a", 64), time.Now()); !errors.Is(err, redislease.ErrUnavailable) {
		t.Fatal("service did not close its owned Redis client")
	}
	if err = pool.Ping(ctx); err != nil {
		t.Fatal("service closed caller-owned PG pool")
	}
	t.Log("real PostgreSQL + Redis", version, "verified TLS/exact-table ACL: wallet-lock lease loss rolls back; early audit replacement, stable failure, two buy-ins, hand settlement, safe cashout and conservation passed")
}
