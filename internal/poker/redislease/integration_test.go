package redislease

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
	"github.com/redis/go-redis/v9"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestRealRedisLease is explicitly opt-in and accepts only a localhost fixture.
// It never uses FLUSHDB, scans keys, or accesses a production connection string.
func TestRealRedisLease(t *testing.T) {
	addr := os.Getenv("MOMIAO_POKER_REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("real isolated Redis fixture not configured")
	}
	host, _, e := net.SplitHostPort(addr)
	if e != nil || host != "127.0.0.1" || os.Getenv("MOMIAO_POKER_REDIS_TEST_CONFIRM") != "owned-local-fixture" {
		t.Fatal("dedicated localhost Redis fixture required")
	}
	fixture := os.Getenv("MOMIAO_POKER_REDIS_TEST_FIXTURE")
	secret, e := os.ReadFile(filepath.Join(fixture, "admin-secret.txt"))
	if e != nil {
		t.Fatal("fixture secret missing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	options, e := clientOptions(Config{Addr: addr, Username: "g2-admin", Password: string(secret), Timeout: time.Second})
	if e != nil {
		t.Fatal(e)
	}
	admin := redis.NewClient(options)
	t.Cleanup(func() { admin.Close() })
	if admin.Ping(ctx).Err() != nil {
		t.Fatal("real Redis fixture unavailable")
	}
	info, e := admin.Info(ctx, "server").Result()
	if e != nil {
		t.Fatal("real Redis server info unavailable")
	}
	server := map[string]string{}
	for _, line := range strings.Split(info, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) == 2 && (parts[0] == "redis_version" || parts[0] == "redis_mode" || parts[0] == "arch_bits") {
			server[parts[0]] = parts[1]
		}
	}
	table := fixtureUUID(t)
	forbidden := KeyPrefix + fixtureUUID(t) + ":1"
	keys := []string{forbidden}
	for seat := 1; seat <= 9; seat++ {
		keys = append(keys, fmt.Sprintf("%s%s:%d", KeyPrefix, table, seat))
	}
	username := "g2-seatlease-" + table
	password := fixtureToken(t)
	ph := sha256.Sum256([]byte(password))
	rules := []any{"ACL", "SETUSER", username, "reset", "on", "#" + hex.EncodeToString(ph[:]), "~" + KeyPrefix + table + ":*", "+ping", "+hello", "+eval", "+get", "+pttl", "+set", "+time", "+del"}
	if admin.Do(ctx, rules...).Err() != nil {
		t.Fatal("fixture scoped ACL creation failed")
	}
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 3*time.Second)
		defer c()
		if admin.Del(cleanup, keys...).Err() != nil {
			t.Error("own-key cleanup failed")
		}
		if admin.Do(cleanup, "ACL", "DELUSER", username).Err() != nil {
			t.Error("own ACL cleanup failed")
		}
	})
	s, e := Open(ctx, Config{Addr: addr, Username: username, Password: password, Timeout: time.Second})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	checks := []string{}
	mark := func(label string) { checks = append(checks, label); t.Log("REAL_REDIS PASS", label) }
	now := func() time.Time {
		v, e := admin.Time(ctx).Result()
		if e != nil {
			t.Fatal("Redis TIME failed")
		}
		return v
	}
	key := func(seat int) string { return fmt.Sprintf("%s%s:%d", KeyPrefix, table, seat) }
	expiry := now().Add(30 * time.Second)
	type attempt struct {
		token string
		ok    bool
		err   error
	}
	results := make(chan attempt, 8)
	for i := 0; i < 8; i++ {
		token := fixtureToken(t)
		go func() { ok, e := s.Acquire(ctx, table, 1, token, expiry); results <- attempt{token, ok, e} }()
	}
	winner := ""
	wins := 0
	for i := 0; i < 8; i++ {
		a := <-results
		if a.err != nil {
			t.Fatal("NX contention error")
		}
		if a.ok {
			winner = a.token
			wins++
		}
	}
	if wins != 1 {
		t.Fatal("NX contention did not yield exactly one owner")
	}
	mark("8-way SET NX contention: exactly one owner")
	expiresBefore, e := admin.Do(ctx, "PEXPIRETIME", key(1)).Int64()
	if e != nil {
		t.Fatal("expiry observation failed")
	}
	ttlBefore, e := admin.PTTL(ctx, key(1)).Result()
	if e != nil || ttlBefore <= 0 || ttlBefore > LeaseTTL {
		t.Fatal("initial TTL outside (0,30s]")
	}
	time.Sleep(25 * time.Millisecond)
	if ok, e := s.Acquire(ctx, table, 1, winner, expiry.Add(time.Minute)); e != nil || !ok {
		t.Fatal("same owner retry failed")
	}
	expiresAfter, e := admin.Do(ctx, "PEXPIRETIME", key(1)).Int64()
	if e != nil || expiresBefore != expiresAfter {
		t.Fatal("same owner retry extended absolute expiry")
	}
	mark("same token retry preserves exact PEXPIRETIME")
	wrong := fixtureToken(t)
	if ok, e := s.Valid(ctx, table, 1, wrong, now()); e != nil || ok {
		t.Fatal("wrong owner valid")
	}
	if e = s.Release(ctx, table, 1, wrong); !errors.Is(e, ErrNotOwned) {
		t.Fatal("wrong owner release accepted")
	}
	if ok, e := s.Valid(ctx, table, 1, winner, now()); e != nil || !ok {
		t.Fatal("wrong release removed owner")
	}
	mark("wrong token validation/release cannot affect owner")
	if e = s.Release(ctx, table, 1, winner); e != nil {
		t.Fatal("owner release failed")
	}
	if ok, e := s.Valid(ctx, table, 1, winner, now()); e != nil || ok {
		t.Fatal("deleted lease still valid")
	}
	if e = s.Release(ctx, table, 1, winner); !errors.Is(e, ErrNotOwned) {
		t.Fatal("missing release not reported")
	}
	mark("correct compare-delete and absent-key rejection")
	expiring := fixtureToken(t)
	pgBeforeExpiry := now()
	if ok, e := s.Acquire(ctx, table, 2, expiring, pgBeforeExpiry.Add(150*time.Millisecond)); e != nil || !ok {
		t.Fatal("short expiry acquisition failed")
	}
	time.Sleep(200 * time.Millisecond)
	if ok, e := s.Valid(ctx, table, 2, expiring, pgBeforeExpiry); e != nil || ok {
		t.Fatal("Redis-expired lease granted despite stale PG time")
	}
	mark("Redis expiry overrides still-early PG input")
	old, newOwner := fixtureToken(t), fixtureToken(t)
	pgNow := now()
	if ok, e := s.Acquire(ctx, table, 3, old, pgNow.Add(30*time.Second)); e != nil || !ok {
		t.Fatal("early delete setup")
	}
	if admin.Del(ctx, key(3)).Err() != nil {
		t.Fatal("own-key early deletion failed")
	}
	if ok, e := s.Valid(ctx, table, 3, old, pgNow); e != nil || ok {
		t.Fatal("early deleted key granted")
	}
	if ok, e := s.Acquire(ctx, table, 3, newOwner, now().Add(30*time.Second)); e != nil || !ok {
		t.Fatal("new reservation after loss failed")
	}
	if ok, e := s.Valid(ctx, table, 3, old, pgNow); e != nil || ok {
		t.Fatal("old token owns replacement")
	}
	if e = s.Release(ctx, table, 3, old); !errors.Is(e, ErrNotOwned) {
		t.Fatal("old owner released replacement")
	}
	mark("early deletion loses authority; new owner unaffected by old release")
	malformed := fixtureToken(t)
	if admin.Set(ctx, key(4), malformed, 0).Err() != nil {
		t.Fatal("persistent-key setup")
	}
	if ok, e := s.Valid(ctx, table, 4, malformed, now()); e != nil || ok {
		t.Fatal("persistent key accepted")
	}
	if ok, e := s.Acquire(ctx, table, 4, malformed, now().Add(30*time.Second)); e != nil || ok {
		t.Fatal("persistent key reacquired")
	}
	if e = s.Release(ctx, table, 4, malformed); e != nil {
		t.Fatal("persistent owner cleanup")
	}
	if admin.Set(ctx, key(4), malformed, time.Minute).Err() != nil {
		t.Fatal("long-TTL setup")
	}
	if ok, e := s.Valid(ctx, table, 4, malformed, now()); e != nil || ok {
		t.Fatal("oversized TTL accepted")
	}
	mark("missing TTL and oversized TTL fail closed")
	if ok, e := s.Acquire(ctx, table, 5, fixtureToken(t), now().Add(-time.Millisecond)); e != nil || ok {
		t.Fatal("expired PG deadline accepted")
	}
	if admin.LPush(ctx, key(5), "wrong-type-test-only").Err() != nil {
		t.Fatal("wrong-type setup")
	}
	if ok, e := s.Valid(ctx, table, 5, fixtureToken(t), now()); ok || !errors.Is(e, ErrUnavailable) {
		t.Fatal("Redis type error did not fail closed")
	}
	mark("expired acquire and Redis command errors fail closed")
	if s.client.(*redis.Client).Set(ctx, forbidden, fixtureToken(t), LeaseTTL).Err() == nil {
		t.Fatal("ACL allowed other owned test namespace")
	}
	if n, e := admin.Exists(ctx, forbidden).Result(); e != nil || n != 0 {
		t.Fatal("ACL denial had key effect")
	}
	mark("least-privilege ACL denies other namespace")
	proxyAddr, stopProxy := faultProxy(t, addr)
	defer stopProxy()
	throughProxy, e := Open(ctx, Config{Addr: proxyAddr, Username: username, Password: password, Timeout: 250 * time.Millisecond})
	if e != nil {
		t.Fatal("proxy real-Redis connection failed")
	}
	defer throughProxy.Close()
	stopProxy()
	if ok, e := throughProxy.Acquire(ctx, table, 6, fixtureToken(t), now().Add(30*time.Second)); ok || !errors.Is(e, ErrUnavailable) {
		t.Fatal("network-down acquire not closed")
	}
	if ok, e := throughProxy.Valid(ctx, table, 3, newOwner, now()); ok || !errors.Is(e, ErrUnavailable) {
		t.Fatal("network-down valid not closed")
	}
	if e := throughProxy.Release(ctx, table, 3, newOwner); !errors.Is(e, ErrUnavailable) {
		t.Fatal("network-down release not closed")
	}
	mark("real Redis connection then closed TCP path: all operations fail closed")
	// Pause only this owned fixture briefly to exercise a real socket read deadline.
	if admin.Do(ctx, "CLIENT", "PAUSE", 100, "ALL").Err() != nil {
		t.Fatal("owned fixture pause failed")
	}
	deadlineCtx, deadlineCancel := context.WithTimeout(ctx, 25*time.Millisecond)
	deadlineStarted := time.Now()
	deadlineOK, deadlineError := s.Valid(deadlineCtx, table, 3, newOwner, pgNow)
	deadlineElapsed := time.Since(deadlineStarted)
	deadlineCancel()
	if deadlineOK || !errors.Is(deadlineError, ErrUnavailable) || deadlineElapsed > 500*time.Millisecond {
		t.Fatal("real command did not respect short context deadline")
	}
	if admin.Ping(ctx).Err() != nil {
		t.Fatal("owned fixture did not resume")
	}
	mark("real Redis brief pause: context deadline fails closed within 500ms")
	tlsAddr := os.Getenv("MOMIAO_POKER_REDIS_TEST_TLS_ADDR")
	tlsHost, _, e := net.SplitHostPort(tlsAddr)
	if e != nil || tlsHost != "127.0.0.1" {
		t.Fatal("dedicated localhost TLS fixture required")
	}
	cert, e := os.ReadFile(filepath.Join(fixture, "server.crt"))
	if e != nil {
		t.Fatal("fixture CA missing")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(cert) {
		t.Fatal("fixture CA invalid")
	}
	secure, e := Open(ctx, Config{Addr: tlsAddr, Username: username, Password: password, TLSConfig: &tls.Config{RootCAs: roots}, Timeout: time.Second})
	if e != nil {
		t.Fatal("verified TLS/ACL connect failed")
	}
	defer secure.Close()
	tlsToken := fixtureToken(t)
	if ok, e := secure.Acquire(ctx, table, 7, tlsToken, now().Add(30*time.Second)); e != nil || !ok {
		t.Fatal("TLS lease acquire failed")
	}
	if e = secure.Release(ctx, table, 7, tlsToken); e != nil {
		t.Fatal("TLS release failed")
	}
	if bad, e := Open(ctx, Config{Addr: tlsAddr, Username: username, Password: password, TLSConfig: &tls.Config{RootCAs: x509.NewCertPool()}, Timeout: time.Second}); e == nil {
		bad.Close()
		t.Fatal("untrusted TLS certificate accepted")
	}
	mark("real verified TLS + ACL succeeds; untrusted CA rejected")
	if bad, e := Open(ctx, Config{Addr: addr, Username: username, Password: fixtureToken(t), Timeout: time.Second}); e == nil {
		bad.Close()
		t.Fatal("invalid ACL credential accepted")
	}
	mark("invalid ACL credential fails closed")
	if e = admin.Del(ctx, keys...).Err(); e != nil {
		t.Fatal("own-key final cleanup failed")
	}
	if n, e := admin.Exists(ctx, keys...).Result(); e != nil || n != 0 {
		t.Fatal("own namespace cleanup not empty")
	}
	if admin.Do(ctx, "ACL", "DELUSER", username).Err() != nil {
		t.Fatal("own ACL final cleanup failed")
	}
	if userResult, e := admin.Do(ctx, "ACL", "GETUSER", username).Result(); (e != nil && !errors.Is(e, redis.Nil)) || userResult != nil {
		t.Fatal("own ACL cleanup not absent")
	}
	mark("exact own UUID namespace keys removed; no FLUSHDB")
	if out := os.Getenv("MOMIAO_POKER_REDIS_TEST_EVIDENCE"); out != "" {
		b, e := json.MarshalIndent(map[string]any{"kind": "REAL_REDIS_INTEGRATION", "server": server, "tests": checks, "observations": map[string]any{"nx_contenders": 8, "nx_winners": wins, "initial_pttl_ms": ttlBefore.Milliseconds(), "pexpiretime_before_ms": expiresBefore, "pexpiretime_after_same_token_retry_ms": expiresAfter, "context_timeout_ms": 25, "observed_deadline_elapsed_ns": deadlineElapsed.Nanoseconds()}, "test_table_uuid": table, "namespace_cleanup": "verified empty", "test_acl_cleanup": "verified absent", "pg_input_note": "No PostgreSQL in adapter test: inputs model PG expiresAt/now using Redis TIME; real PG service integration belongs to G3", "network_fault_note": "Raw TCP forwarder to actual Redis was closed; no RESP simulator", "status": "PASS"}, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(out, append(b, '\n'), 0600); e != nil {
			t.Fatal("evidence write failed")
		}
	}
}

func fixtureToken(t *testing.T) string {
	t.Helper()
	var b [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		t.Fatal(e)
	}
	return hex.EncodeToString(b[:])
}
func fixtureUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		t.Fatal(e)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// A bounded test-only byte forwarder, not a RESP implementation or Redis stub.
func faultProxy(t *testing.T, target string) (string, func()) {
	t.Helper()
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan struct{})
	var mu sync.Mutex
	var connections []net.Conn
	var copies sync.WaitGroup
	var once sync.Once
	go func() {
		defer close(done)
		for {
			down, e := listener.Accept()
			if e != nil {
				return
			}
			up, e := net.DialTimeout("tcp", target, time.Second)
			if e != nil {
				down.Close()
				continue
			}
			mu.Lock()
			connections = append(connections, down, up)
			mu.Unlock()
			copies.Add(2)
			go func() { defer copies.Done(); defer down.Close(); defer up.Close(); io.Copy(up, down) }()
			go func() { defer copies.Done(); defer down.Close(); defer up.Close(); io.Copy(down, up) }()
		}
	}()
	return listener.Addr().String(), func() {
		once.Do(func() {
			listener.Close()
			<-done
			mu.Lock()
			for _, c := range connections {
				c.Close()
			}
			mu.Unlock()
			copies.Wait()
		})
	}
}
