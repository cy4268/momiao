package redislease

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type accessFixture struct {
	s                                         *Store
	admin                                     *redis.Client
	config                                    Config
	sid, otherSID, table, otherTable, binding string
	user, otherUser                           int64
	keys                                      []string
	proof                                     map[string]any
}

func newAccessFixture(t *testing.T, ctx context.Context) accessFixture {
	t.Helper()
	addr := os.Getenv("MOMIAO_POKER_ACCESS_TEST_ADDR")
	if addr == "" {
		t.Skip("explicit owned TLS Redis fixture required")
	}
	if addr != "127.0.0.1:16380" || os.Getenv("MOMIAO_POKER_REDIS_TEST_CONFIRM") != "owned-local-fixture" {
		t.Fatal("wrong local fixture")
	}
	root := os.Getenv("MOMIAO_POKER_REDIS_TEST_FIXTURE")
	secret, err := os.ReadFile(filepath.Join(root, "admin-secret.txt"))
	if err != nil {
		t.Fatal("fixture credential unavailable")
	}
	cert, err := os.ReadFile(filepath.Join(root, "server.crt"))
	ca := x509.NewCertPool()
	if err != nil || !ca.AppendCertsFromPEM(cert) {
		t.Fatal("fixture CA unavailable")
	}
	cfg := Config{Addr: addr, Username: "g2-admin", Password: string(secret), TLSConfig: &tls.Config{RootCAs: ca}, Timeout: time.Second}
	options, err := clientOptions(cfg)
	if err != nil {
		t.Fatal("fixture configuration invalid")
	}
	seed := redis.NewClient(options)
	t.Cleanup(func() { seed.Close() })
	info, err := seed.Info(ctx, "server").Result()
	runID := os.Getenv("MOMIAO_POKER_ACCESS_TEST_RUN_ID")
	if err != nil || len(runID) != 40 || !strings.Contains(info, "run_id:"+runID+"\r\n") {
		t.Fatal("fixture identity changed")
	}
	f := accessFixture{sid: fixtureToken(t), otherSID: fixtureToken(t), table: fixtureUUID(t), otherTable: fixtureUUID(t)}
	n, _ := strconv.ParseUint(strings.ReplaceAll(f.table, "-", "")[:12], 16, 64)
	f.user, f.otherUser = int64(n)+1, int64(n)+2
	user, other := strconv.FormatInt(f.user, 10), strconv.FormatInt(f.otherUser, 10)
	f.binding = "v1:" + user + ":1:0:" + fixtureToken(t)
	f.keys = []string{accessPrefix + f.sid + ":" + f.table, accessPrefix + f.otherSID + ":" + f.table, accessPrefix + f.sid + ":" + f.otherTable,
		accessAttemptPrefix + "owner:" + user, accessAttemptPrefix + "owner-table:" + user + ":" + f.table, accessAttemptPrefix + "owner-table:" + user + ":" + f.otherTable,
		accessAttemptPrefix + "owner:" + other, accessAttemptPrefix + "owner-table:" + other + ":" + f.table, accessPrefix + f.otherSID + ":" + f.otherTable}
	users := []string{"g3-table-access-" + fixtureUUID(t), "g3-table-access-" + f.table} // data, runtime
	for _, name := range users {
		if v, e := seed.Do(ctx, "ACL", "GETUSER", name).Result(); e != nil && !errors.Is(e, redis.Nil) || v != nil {
			t.Fatal("fresh ACL required")
		}
	}
	proof := map[string]any{"kind": "REAL_TLS_REDIS_TABLE_ACCESS", "users": users, "keys": f.keys, "run_id": runID, "target": addr, "cleanup": false}
	f.proof = proof
	write := func() {
		raw, _ := json.MarshalIndent(proof, "", "  ")
		if path := os.Getenv("MOMIAO_POKER_ACCESS_TEST_EVIDENCE"); path != "" {
			if os.WriteFile(path, raw, 0600) != nil {
				t.Error("evidence write failed")
			}
		}
	}
	write()
	t.Logf("owned data ACL=%s runtime ACL=%s keys=%s", users[0], users[1], strings.Join(f.keys, ","))
	owned := false
	created := []string{}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if f.s != nil {
			f.s.Close()
		}
		clean := true
		if owned {
			if f.admin.Del(c, f.keys...).Err() != nil {
				clean = false
			}
			if left, e := f.admin.Exists(c, f.keys...).Result(); e != nil || left != 0 {
				clean = false
			}
		}
		if f.admin != nil {
			f.admin.Close()
		}
		for _, name := range created {
			if seed.Do(c, "ACL", "DELUSER", name).Err() != nil {
				clean = false
			}
			if v, e := seed.Do(c, "ACL", "GETUSER", name).Result(); e != nil && !errors.Is(e, redis.Nil) || v != nil {
				clean = false
			}
		}
		current, e := seed.Info(c, "server").Result()
		proof["keys_owned"], proof["cleanup"] = owned, clean && e == nil && strings.Contains(current, "run_id:"+runID+"\r\n")
		if proof["cleanup"] != true {
			t.Error("exact cleanup or unchanged run_id failed")
		}
		proof["test_passed"] = !t.Failed()
		write()
	})
	commands := []string{"+ping +hello +exists +get +set +del +pttl +time +pexpiretime +pexpire +persist +hset", "+ping +hello +eval +get +set +del +pttl +time +pexpiretime +incr"}
	proof["commands_data"], proof["commands_runtime"] = commands[0], commands[1]
	for i, name := range users {
		password := fixtureToken(t)
		ph := sha256.Sum256([]byte(password))
		rules := []any{"ACL", "SETUSER", name, "reset", "on", "#" + hex.EncodeToString(ph[:])}
		for _, command := range strings.Fields(commands[i]) {
			rules = append(rules, command)
		}
		keys := f.keys
		if i == 1 {
			keys = keys[:8]
		}
		for _, key := range keys {
			rules = append(rules, "~"+key)
		}
		created = append(created, name) // Even an ambiguous create outcome is cleaned by exact fresh name.
		if seed.Do(ctx, rules...).Err() != nil {
			t.Fatal("scoped ACL creation failed")
		}
		cfg.Username, cfg.Password = name, password
		if i == 0 {
			options, e := clientOptions(cfg)
			if e != nil {
				t.Fatal("data ACL configuration failed")
			}
			f.admin = redis.NewClient(options)
			if count, e := f.admin.Exists(ctx, f.keys...).Result(); e != nil || count != 0 {
				t.Fatal("fresh exact key scope required; no key ownership claimed")
			}
			owned = true
		} else {
			f.config = cfg
			f.s, err = Open(ctx, cfg)
			if err != nil {
				t.Fatal("verified TLS runtime ACL Store failed")
			}
		}
	}
	return f
}

func TestTableAccessRealRedis(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	f := newAccessFixture(t, ctx)
	now, err := f.admin.Time(ctx).Result()
	if err != nil {
		t.Fatal("Redis TIME failed")
	}
	until := now.Add(13 * time.Second)
	assert := func(ok bool, err error, want bool) {
		t.Helper()
		if err != nil || ok != want {
			t.Fatal("real Lua outcome", ok, want, err)
		}
	}
	expires := func(key string) int64 {
		t.Helper()
		v, e := f.admin.Do(ctx, "PEXPIRETIME", key).Int64()
		if e != nil {
			t.Fatal("expiry observation failed")
		}
		return v
	}
	valid := func(binding string, live time.Time, want bool) {
		t.Helper()
		ok, e := f.s.AccessValid(ctx, f.sid, f.table, binding, live)
		assert(ok, e, want)
	}
	put := func(deadline time.Time, want bool) {
		t.Helper()
		ok, e := f.s.AccessPut(ctx, f.sid, f.table, f.binding, deadline)
		assert(ok, e, want)
	}
	put(until, true)
	if value, e := f.admin.Get(ctx, f.keys[0]).Result(); e != nil || value != f.binding+"|"+strconv.FormatInt(until.UnixMilli(), 10) || expires(f.keys[0]) != until.UnixMilli() {
		t.Fatal("exact key/value/PXAT mismatch")
	}
	f.proof["native_fixture_until"], f.proof["grant_pexpiretime_ms"] = until.UTC(), expires(f.keys[0])
	valid(f.binding, until, true)
	valid(f.binding, until.Add(-time.Millisecond), false)
	valid(f.binding, until.Add(time.Minute), true)
	if expires(f.keys[0]) != until.UnixMilli() {
		t.Fatal("read renewed original expiry")
	}
	for _, bad := range []string{strings.Replace(f.binding, ":1:0:", ":2:0:", 1), strings.Replace(f.binding, ":1:0:", ":1:1:", 1), strings.Replace(f.binding, "v1:"+strconv.FormatInt(f.user, 10)+":", "v1:"+strconv.FormatInt(f.otherUser, 10)+":", 1), f.binding[:strings.LastIndex(f.binding, ":")+1] + testToken} {
		valid(bad, until, false)
	}
	for _, pair := range [][2]string{{f.otherSID, f.table}, {f.sid, f.otherTable}} {
		ok, e := f.s.AccessValid(ctx, pair[0], pair[1], f.binding, until)
		assert(ok, e, false)
	}
	if ok, e := f.s.AccessValid(ctx, f.otherSID, f.otherTable, f.binding, until); ok || !errors.Is(e, ErrUnavailable) {
		t.Fatal("ACL accepted an undeclared exact key")
	}
	for _, op := range []string{"PERSIST", "PEXPIRE"} {
		args := []any{op, f.keys[0]}
		if op == "PEXPIRE" {
			args = append(args, 1000)
		}
		if f.admin.Do(ctx, args...).Err() != nil {
			t.Fatal("own TTL mutation failed")
		}
		valid(f.binding, until, false)
		put(until, true)
	}
	for _, suffix := range []string{"0" + strconv.FormatInt(until.UnixMilli(), 10), "1e12", strconv.FormatInt(until.UnixMilli(), 10) + "|x"} {
		if f.admin.Do(ctx, "SET", f.keys[0], f.binding+"|"+suffix, "PXAT", until.UnixMilli()).Err() != nil {
			t.Fatal("own value mutation failed")
		}
		valid(f.binding, until, false)
	}
	put(until, true)
	if f.admin.Del(ctx, f.keys[0]).Err() != nil {
		t.Fatal("own-key deletion failed")
	}
	valid(f.binding, until, false)
	now, _ = f.admin.Time(ctx).Result()
	put(now.Add(250*time.Millisecond), true)
	time.Sleep(300 * time.Millisecond)
	valid(f.binding, until, false)
	put(now.Add(-time.Second), false)
	t.Log("REAL_REDIS grant: absolute expiry, binding isolation, ACL, no renewal, altered TTL/value, loss and expiration")
	attempt := func(user int64, table string, ownerMax, tableMax uint32, ownerWindow, tableWindow time.Duration, want bool) {
		t.Helper()
		ok, e := f.s.AccessAttempt(ctx, user, table, ownerMax, ownerWindow, tableMax, tableWindow)
		assert(ok, e, want)
	}
	attempt(f.user, "", 3, 0, 3*time.Second, 0, true)
	ownerExpiry := expires(f.keys[3])
	var wg sync.WaitGroup
	results := make(chan bool, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, e := f.s.AccessAttempt(ctx, f.user, f.table, 3, 3*time.Second, 2, time.Second)
			if e != nil {
				t.Error("real concurrent budget error")
			}
			results <- ok
		}()
	}
	wg.Wait()
	close(results)
	wins := 0
	for ok := range results {
		if ok {
			wins++
		}
	}
	if wins != 2 || expires(f.keys[3]) != ownerExpiry {
		t.Fatal("concurrent owner/table quota or original window changed", wins)
	}
	f.proof["budget_contenders"], f.proof["budget_wins"], f.proof["original_owner_expiry_ms"] = 8, wins, ownerExpiry
	tableExpiry := expires(f.keys[4])
	attempt(f.user, f.table, 3, 2, 3*time.Second, time.Second, false)
	attempt(f.user, f.otherTable, 3, 2, 3*time.Second, time.Second, false)
	if expires(f.keys[4]) != tableExpiry || expires(f.keys[5]) != -2 {
		t.Fatal("denial renewed a window or wrote second table")
	}
	attempt(f.otherUser, f.table, 1, 1, 3*time.Second, time.Second, true)
	time.Sleep(time.Until(time.UnixMilli(tableExpiry)) + 20*time.Millisecond)
	attempt(f.user, f.table, 3, 2, 3*time.Second, time.Second, false)
	if expires(f.keys[4]) != -2 {
		t.Fatal("owner quota bypassed by table window expiry")
	}
	time.Sleep(time.Until(time.UnixMilli(ownerExpiry)) + 20*time.Millisecond)
	attempt(f.user, f.table, 3, 2, 3*time.Second, time.Second, true)
	if count, e := f.admin.Get(ctx, f.keys[3]).Int(); e != nil || count != 1 {
		t.Fatal("natural owner window did not reset")
	}
	if f.admin.Del(ctx, f.keys[3], f.keys[4]).Err() != nil || f.admin.Set(ctx, f.keys[3], "1", 5*time.Second).Err() != nil || f.admin.HSet(ctx, f.keys[4], "x", "y").Err() != nil {
		t.Fatal("own malformed second-key setup")
	}
	ownerExpiry = expires(f.keys[3])
	if ok, e := f.s.AccessAttempt(ctx, f.user, f.table, 3, 5*time.Second, 2, 5*time.Second); ok || !errors.Is(e, ErrUnavailable) {
		t.Fatal("wrong-type second key did not fail closed")
	}
	if f.admin.Del(ctx, f.keys[4]).Err() != nil {
		t.Fatal("own wrong-type cleanup")
	}
	for _, value := range []string{"0", "01", "-1", "1.5", "1e2", "4294967296"} {
		if f.admin.Set(ctx, f.keys[4], value, time.Second).Err() != nil {
			t.Fatal("own counter setup")
		}
		attempt(f.user, f.table, 3, 2, 5*time.Second, 5*time.Second, false)
	}
	if f.admin.Set(ctx, f.keys[4], "1", 0).Err() != nil {
		t.Fatal("own persistent counter setup")
	}
	attempt(f.user, f.table, 3, 2, 5*time.Second, 5*time.Second, false)
	if count, e := f.admin.Get(ctx, f.keys[3]).Int(); e != nil || count != 1 || expires(f.keys[3]) != ownerExpiry {
		t.Fatal("preflight failure consumed owner budget")
	}
	t.Log("REAL_REDIS limiter: 8 contenders/2 wins, owner-only and cross-table quota, independent windows, malformed second-key preflight")
	bad := f.config
	bad.TLSConfig = &tls.Config{RootCAs: x509.NewCertPool()}
	if s, e := Open(ctx, bad); e == nil {
		s.Close()
		t.Fatal("wrong TLS CA accepted")
	}
	addr, pause, stop := accessSlowProxy(t, f.config.Addr)
	proxied := f.config
	proxied.Addr = addr
	s, e := Open(ctx, proxied)
	if e != nil {
		t.Fatal("own verified TLS forwarder failed")
	}
	defer s.Close()
	pause()
	c, cc := context.WithTimeout(ctx, 25*time.Millisecond)
	start := time.Now()
	ok, e := s.AccessValid(c, f.sid, f.table, f.binding, until)
	cc()
	elapsed := time.Since(start)
	f.proof["context_timeout_ms"], f.proof["observed_context_ns"] = 25, elapsed.Nanoseconds()
	if ok || !errors.Is(e, ErrUnavailable) || elapsed > 500*time.Millisecond {
		t.Fatal("real delayed TLS path missed shorter context")
	}
	stop()
	if ok, e = s.AccessPut(ctx, f.sid, f.table, f.binding, until); ok || !errors.Is(e, ErrUnavailable) {
		t.Fatal("closed TLS path granted access")
	}
	t.Log("REAL_REDIS TLS: wrong CA denied; own delayed/closed byte forwarder fails closed without server pause")
}

type accessGateReader struct {
	io.Reader
	paused *atomic.Bool
	gate   <-chan struct{}
}

func (r accessGateReader) Read(p []byte) (int, error) {
	n, e := r.Reader.Read(p)
	if r.paused.Load() {
		<-r.gate
	}
	return n, e
}

// One owned TLS byte stream; no RESP emulation and no shared-server pause.
func accessSlowProxy(t *testing.T, target string) (string, func(), func()) {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal("own proxy listen failed")
	}
	var paused atomic.Bool
	gate := make(chan struct{})
	pair := make(chan []net.Conn, 1)
	go func() {
		local, e := l.Accept()
		if e != nil {
			pair <- nil
			return
		}
		remote, e := net.DialTimeout("tcp", target, time.Second)
		if e != nil {
			local.Close()
			pair <- nil
			return
		}
		pair <- []net.Conn{local, remote}
		go io.Copy(local, remote)
		io.Copy(remote, accessGateReader{local, &paused, gate})
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			l.Close()
			for _, c := range <-pair {
				c.Close()
			}
			close(gate)
		})
	}
	t.Cleanup(stop)
	return l.Addr().String(), func() { paused.Store(true) }, stop
}
