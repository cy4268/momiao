package redislease

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"strings"
	"testing"
	"time"
)

const testTable = "617f0530-861e-4b99-88ee-48be3133b894"
const testToken = "c4e119208980f3f9c09acdc372d4f952bde74916f4ef31f878b91c9a647a1285"

// Stub only checks the adapter boundary. Lua semantics are tested on real Redis.
type stubRedis struct {
	calls    int
	value    any
	err      error
	script   string
	keys     []string
	args     []any
	deadline bool
}

func (s *stubRedis) Eval(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd {
	s.calls++
	s.script = script
	s.keys = keys
	s.args = args
	_, s.deadline = ctx.Deadline()
	return redis.NewCmdResult(s.value, s.err)
}
func (s *stubRedis) Close() error { return nil }

func TestBoundaryAndFailClosed(t *testing.T) {
	stub := &stubRedis{value: int64(1)}
	s := &Store{client: stub, timeout: time.Second}
	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	ok, e := s.Acquire(context.Background(), testTable, 1, testToken, now.Add(30*time.Second))
	if e != nil || !ok {
		t.Fatal("acquire", ok, e)
	}
	if !stub.deadline || len(stub.keys) != 1 || stub.keys[0] != "chaldea:poker:seat-reservation:"+testTable+":1" || stub.args[0] != testToken || stub.args[1] != now.Add(30*time.Second).UnixMilli() {
		t.Fatal("script boundary")
	}
	for _, in := range []struct {
		table string
		seat  int
		token string
	}{{testTable + ":2", 1, testToken}, {strings.ToUpper(testTable), 1, testToken}, {testTable, 0, testToken}, {testTable, 10, testToken}, {testTable, 1, ""}, {testTable, 1, strings.ToUpper(testToken)}} {
		before := stub.calls
		ok, e = s.Acquire(context.Background(), in.table, in.seat, in.token, now)
		if ok || !errors.Is(e, ErrInvalidInput) || stub.calls != before {
			t.Fatal("input crossed Redis boundary")
		}
	}
	if ok, e = s.Valid(context.Background(), testTable, 1, testToken, time.Time{}); ok || !errors.Is(e, ErrInvalidInput) {
		t.Fatal("missing PG time")
	}
	stub.value = int64(0)
	if ok, e = s.Valid(context.Background(), testTable, 1, testToken, now); ok || e != nil {
		t.Fatal("missing owner not false")
	}
	if e = s.Release(context.Background(), testTable, 1, testToken); !errors.Is(e, ErrNotOwned) {
		t.Fatal("missing release owner")
	}
	stub.value = int64(2)
	if ok, e = s.Valid(context.Background(), testTable, 1, testToken, now); ok || !errors.Is(e, ErrUnavailable) {
		t.Fatal("unexpected server reply")
	}
	stub.err = errors.New("secret " + testToken)
	if ok, e = s.Acquire(context.Background(), testTable, 1, testToken, now); ok || !errors.Is(e, ErrUnavailable) || strings.Contains(e.Error(), testToken) {
		t.Fatal("backend failure leaked/opened")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	before := stub.calls
	if ok, e = s.Valid(cancelled, testTable, 1, testToken, now); ok || !errors.Is(e, context.Canceled) || stub.calls != before {
		t.Fatal("cancel ignored")
	}
}

func TestClientConfiguration(t *testing.T) {
	o, e := clientOptions(Config{Addr: "127.0.0.1:16379"})
	if e != nil {
		t.Fatal(e)
	}
	if o.MaxRetries != -1 || !o.ContextTimeoutEnabled || o.Protocol != 2 || o.PoolSize != 8 || o.MaxActiveConns != 8 || o.DialTimeout <= 0 || o.ReadTimeout <= 0 || o.WriteTimeout <= 0 || o.PoolTimeout <= 0 || o.ClientSideCacheConfig != nil || o.ClientSideCache != nil {
		t.Fatal("unbounded/unsafe client options")
	}
	for _, c := range []Config{{Addr: ""}, {Addr: "redis://user:secret@host:6379"}, {Addr: "host:6379"}, {Addr: "host:6379", TLSConfig: &tls.Config{}}, {Addr: "127.0.0.1:16379", Timeout: -1}, {Addr: "127.0.0.1:16379", TLSConfig: &tls.Config{InsecureSkipVerify: true}}} {
		if _, e = clientOptions(c); !errors.Is(e, ErrInvalidConfig) {
			t.Fatal("unsafe config accepted")
		}
	}
	tlsConfig := &tls.Config{}
	c := Config{Addr: "redis.example:6379", Username: "acl-user", Password: "private-password", TLSConfig: tlsConfig}
	o, e = clientOptions(c)
	if e != nil || o.TLSConfig == tlsConfig || o.TLSConfig.MinVersion < tls.VersionTLS12 || o.Username != c.Username || o.Password != c.Password {
		t.Fatal("TLS/ACL config", e)
	}
	b, _ := json.Marshal(c)
	for _, v := range []string{string(b), fmt.Sprintf("%v", c), fmt.Sprintf("%#v", c)} {
		if strings.Contains(v, c.Password) || strings.Contains(v, c.Username) {
			t.Fatal("config leaks secrets")
		}
	}
}
