package session

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/maintnotifications"
)

func testOptions(t *testing.T) Options {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), "postgres://fixture:unused@localhost/fixture?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	cache := redis.NewClient(&redis.Options{
		Addr: "localhost:6379", Username: "fixture", Password: "unused", Protocol: 2,
		MaxRetries: -1, DialerRetries: 1, ContextTimeoutEnabled: true,
		DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second, PoolTimeout: time.Second,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}, DisableIdentity: true,
		MaintNotificationsConfig: &maintnotifications.Config{Mode: "disabled"},
	})
	t.Cleanup(func() { cache.Close() })
	return Options{
		Redis: cache, Pool: pool, NativeSocket: filepath.Join(t.TempDir(), "native.sock"),
		OpsSocket: filepath.Join(t.TempDir(), "ops.sock"), NativeReaderKey: strings.Repeat("1", 64), OpsKey: strings.Repeat("2", 64),
		SessionSealKey: [32]byte{3}, Origin: "https://f4.test", Environment: "DEVELOPMENT",
		Lifetime: Lifetime{Idle: 120 * time.Second, Absolute: 600 * time.Second, Touch: 15 * time.Second},
	}
}

func TestCookieMetadataRejectsBeforeAuthorityRead(t *testing.T) {
	s, err := New(testOptions(t))
	if err != nil {
		t.Fatal("fixture constructor")
	}
	defer s.Close()
	for name, change := range map[string]func(*http.Request){
		"missing Origin":            func(r *http.Request) { r.Header.Del("Origin") },
		"foreign Origin":            func(r *http.Request) { r.Header.Set("Origin", "https://foreign.test") },
		"duplicate Origin":          func(r *http.Request) { r.Header.Add("Origin", "https://f4.test") },
		"missing Fetch Metadata":    func(r *http.Request) { r.Header.Del("Sec-Fetch-Site") },
		"cross-site Fetch Metadata": func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
		"duplicate Fetch Metadata":  func(r *http.Request) { r.Header.Add("Sec-Fetch-Site", "same-origin") },
		"missing CSRF":              func(r *http.Request) { r.Header.Del("X-CSRF-Token") },
		"duplicate CSRF":            func(r *http.Request) { r.Header.Add("X-CSRF-Token", "duplicate") },
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "https://f4.test/operation", nil)
			r.Header.Set("Origin", "https://f4.test")
			r.Header.Set("Sec-Fetch-Site", "same-origin")
			r.Header.Set("X-CSRF-Token", "candidate")
			r.AddCookie(sessionCookie(secret(), 0))
			change(r)
			_, err := s.VerifyRequest(r)
			requireFault(t, err, 403, "SESSION_CSRF_FAILED")
		})
	}
	for _, handle := range []RequestSession{{owner: s, hash: strings.Repeat("a", 64)}, {owner: &Service{}, hash: strings.Repeat("a", 64), unsafe: true}} {
		_, err := s.StartOps(context.Background(), handle, Operation{})
		requireFault(t, err, 400, "SESSION_INPUT_INVALID")
	}
}

func TestReceiptRejectsChangedContextAndUntrustedClaims(t *testing.T) {
	now := time.Now().UnixMilli()
	c := opsContext{"1", "00000000-0000-0000-0000-000000000001", "1", strings.Repeat("a", 64), "1", "1", strings.Repeat("b", 64), "0", "1", "maintenance.activate", "maintenance", "00000000-0000-0000-0000-000000000002", "1", "DEVELOPMENT", strings.Repeat("c", 64), strings.Repeat("d", 64), strings.Repeat("e", 64)}
	p := pending{Context: c, Hash: c.hash(), Expires: ms(now + 600000)}
	reply := receipt{"1", "OPS_FRESH", "1", c.NativeSIDHash, "1", "1", p.Hash, strconv.FormatInt(now/1000, 10), strconv.FormatInt(now/1000+600, 10), "password"}
	if !c.valid() || !reply.valid(p, now) {
		t.Fatal("validator fixture shape")
	}
	raw, _ := json.Marshal(c)
	var fields map[string]string
	json.Unmarshal(raw, &fields)
	for field, previous := range fields {
		fields[field] = previous + "changed"
		encoded, _ := json.Marshal(fields)
		var changed opsContext
		json.Unmarshal(encoded, &changed)
		candidate := pending{Context: changed, Hash: changed.hash(), Expires: p.Expires}
		if reply.valid(candidate, now) {
			t.Errorf("receipt accepted changed context field %s", field)
		}
		fields[field] = previous
	}
	for name, change := range map[string]func(*receipt){
		"wrong actor":           func(r *receipt) { r.ActorUserID = "2" },
		"wrong native":          func(r *receipt) { r.NativeSIDHash = strings.Repeat("f", 64) },
		"wrong sv":              func(r *receipt) { r.NativeSessionVersion = "2" },
		"wrong uv":              func(r *receipt) { r.NativeAuthVersion = "2" },
		"wrong purpose":         func(r *receipt) { r.Purpose = "LOGIN" },
		"unsupported method":    func(r *receipt) { r.Method = "webauthn" },
		"noncanonical proof":    func(r *receipt) { r.ProofID = "01" },
		"future authentication": func(r *receipt) { r.AuthenticatedAt = strconv.FormatInt(now/1000+10, 10) },
		"expired":               func(r *receipt) { r.ExpiresAt = strconv.FormatInt(now/1000-1, 10) },
		"old freshness":         func(r *receipt) { r.AuthenticatedAt = strconv.FormatInt(now/1000-601, 10) },
		"extended expiry":       func(r *receipt) { r.ExpiresAt = strconv.FormatInt(now/1000+601, 10) },
	} {
		bad := reply
		change(&bad)
		if bad.valid(p, now) {
			t.Errorf("receipt accepted %s", name)
		}
	}
}
func requireFault(t *testing.T, err error, status int, code string) {
	t.Helper()
	var fault Fault
	if !errors.As(err, &fault) || fault.Status != status || fault.Code != code || fault.Error() != code {
		t.Fatalf("expected sanitized %d %s", status, code)
	}
}
func TestConfigurationRejectsAmbiguousAuthority(t *testing.T) {
	options := testOptions(t)
	s, err := New(options)
	if err != nil {
		t.Fatal("valid explicit isolated configuration rejected")
	}
	s.Close()
	for name, change := range map[string]func(*Options){
		"zero lifetime":         func(o *Options) { o.Lifetime = Lifetime{} },
		"touch covers idle":     func(o *Options) { o.Lifetime.Touch = o.Lifetime.Idle },
		"idle exceeds absolute": func(o *Options) { o.Lifetime.Idle = o.Lifetime.Absolute + time.Second },
		"http origin":           func(o *Options) { o.Origin = "http://f4.test" },
		"origin path":           func(o *Options) { o.Origin += "/" },
		"origin credentials":    func(o *Options) { o.Origin = "https://user@f4.test" },
		"same keys":             func(o *Options) { o.OpsKey = o.NativeReaderKey },
		"zero seal":             func(o *Options) { o.SessionSealKey = [32]byte{} },
		"same sockets":          func(o *Options) { o.OpsSocket = o.NativeSocket },
		"missing redis":         func(o *Options) { o.Redis = nil },
		"missing PG":            func(o *Options) { o.Pool = nil },
	} {
		t.Run(name, func(t *testing.T) {
			bad := options
			change(&bad)
			_, err := New(bad)
			requireFault(t, err, 500, "SESSION_CONFIG_INVALID")
		})
	}
}
func TestOpaqueHandlesRejectZeroAndRedact(t *testing.T) {
	s, err := New(testOptions(t))
	if err != nil {
		t.Fatal("fixture options")
	}
	defer s.Close()
	_, err = s.StartOps(context.Background(), RequestSession{}, Operation{})
	requireFault(t, err, 400, "SESSION_INPUT_INVALID")
	_, err = s.CompleteOps(context.Background(), RequestSession{}, "candidate", "secret-proof")
	requireFault(t, err, 400, "SESSION_INPUT_INVALID")
	requireFault(t, s.Revoke(context.Background(), RequestSession{}), 400, "SESSION_INPUT_INVALID")
	w := httptest.NewRecorder()
	requireFault(t, s.WriteGrant(w, Grant{}), 400, "SESSION_INPUT_INVALID")
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("zero grant emitted Cookie")
	}
	for _, value := range []any{NativeCredential{SID: "secret-native-sid", AccessToken: "secret-token"}, s, *s, Grant{}, RequestSession{}} {
		if printed := fmt.Sprintf("%v %#v", value, value); strings.Contains(printed, "secret-") || !strings.Contains(printed, "redacted") {
			t.Fatal("opaque object debug output leaked")
		}
	}
}

func TestStrictJSONRejectsAmbiguousWireFields(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte(`{"actor":"1","actor":"2"}`), []byte(`{"ACTOR":"1"}`),
		[]byte(`{"actor":"1","extra":"x"}`), []byte(`{}`), []byte(`null`),
		[]byte(`{"actor":1}`), []byte(`{"actor":"1"} {}`), []byte("{\"actor\":\"\xff\"}"),
	} {
		var response struct {
			Actor string `json:"actor"`
		}
		if strictJSON(raw, &response) {
			t.Fatal("ambiguous JSON was accepted")
		}
	}
}

func TestNativeSIDCiphertextCannotMoveBetweenSessions(t *testing.T) {
	s, err := New(testOptions(t))
	if err != nil {
		t.Fatal("fixture options")
	}
	defer s.Close()
	sid := "01234567-89ab-cdef-0123-456789abcdef"
	r := record{Hash: strings.Repeat("a", 64), NativeHash: hashHexBytes([]byte(sid))}
	r.NativeBox = s.sealSID(sid, r)
	plain, err := s.openSID(r)
	if err != nil || plain != sid || strings.Contains(r.NativeBox, sid) {
		t.Fatal("Native SID sealing failed")
	}
	for _, change := range []func(*record){
		func(r *record) { r.Hash = strings.Repeat("b", 64) },
		func(r *record) { r.NativeHash = strings.Repeat("c", 64) },
		func(r *record) { r.NativeBox = "invalid" },
	} {
		bad := r
		change(&bad)
		if _, err := s.openSID(bad); err == nil {
			t.Fatal("ciphertext binding substitution accepted")
		}
	}
	view := View{FreshAuthAt: new(time.Time), Confirmation: &Confirmation{OperationID: "original"}}
	grant := Grant{owner: s, view: view}
	copy := grant.View()
	*copy.FreshAuthAt = time.Now()
	copy.Confirmation.OperationID = "changed"
	if !grant.View().FreshAuthAt.IsZero() || grant.View().Confirmation.OperationID != "original" {
		t.Fatal("View mutated sealed handle")
	}
}
