//go:build linux && session_integration

package session

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	urlpkg "net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/nativeself"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/maintnotifications"
)

type f4NativeTransport struct{ *http.Transport }

func (f f4NativeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c := r.Clone(r.Context())
	c.URL.Scheme, c.URL.Host, c.Host = "http", "unix", "localhost"
	return f.Transport.RoundTrip(c)
}
func f4JSON(t *testing.T, c *http.Client, method, path string, body any, headers map[string]string) map[string]any {
	t.Helper()
	raw, err := json.Marshal(body)
	f4Check(t, err == nil, "fixture request encoding")
	r, err := http.NewRequest(method, "https://f4.test"+path, bytes.NewReader(raw))
	f4Check(t, err == nil, "fixture request")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://f4.test")
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	resp, err := c.Do(r)
	if err != nil {
		t.Fatalf("fixture transport failed at %s", path)
	}
	defer resp.Body.Close()
	if path == "/_f4/finish" && resp.StatusCode == 204 {
		return map[string]any{}
	}
	var out map[string]any
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 131072)).Decode(&out) != nil || out["success"] == false {
		t.Fatalf("fixture response failed at %s: HTTP%d", path, resp.StatusCode)
	}
	if data, ok := out["data"].(map[string]any); ok {
		return data
	}
	return out
}
func f4Redis(t *testing.T, admin bool) *redis.Client {
	t.Helper()
	ca, err := os.ReadFile(os.Getenv("F4_REDIS_CA"))
	f4Check(t, err == nil, "fixture CA")
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		t.Fatal("fixture certificate")
	}
	user, password := "f4session", os.Getenv("F4_REDIS_PASSWORD")
	if admin {
		user, password = "f4admin", os.Getenv("F4_REDIS_ADMIN_PASSWORD")
	}
	c := redis.NewClient(&redis.Options{Addr: os.Getenv("F4_REDIS_ADDR"), Username: user, Password: password, Protocol: 2, MaxRetries: -1, DialerRetries: 1, ContextTimeoutEnabled: true, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second, PoolTimeout: time.Second, DisableIdentity: true, MaintNotificationsConfig: &maintnotifications.Config{Mode: "disabled"}, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: "redis", RootCAs: roots}})
	t.Cleanup(func() { c.Close() })
	f4Check(t, c.Ping(context.Background()).Err() == nil, "fixture TLSRedis ping")
	return c
}
func TestF4Baseline(t *testing.T) {
	ctx := context.Background()
	native := nativeself.NewTransport(os.Getenv("F4_NATIVE_SOCKET"))
	defer native.CloseIdleConnections()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Transport: f4NativeTransport{native}, Jar: jar, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer f4JSON(t, client, "POST", "/_f4/finish", map[string]string{}, nil)
	store, err := platform.Open(ctx, os.Getenv("F4_PLATFORM_OWNER_DSN"))
	f4Check(t, err == nil, "fixture migration owner")
	f4Check(t, store.Migrate(ctx) == nil, "fixture migration")
	store.Close()
	owner, err := pgxpool.New(ctx, os.Getenv("F4_PLATFORM_OWNER_DSN"))
	f4Check(t, err == nil, "fixture owner inspection")
	defer owner.Close()
	var count int
	if owner.QueryRow(ctx, "SELECT count(*) FROM platform_meta.schema_migrations").Scan(&count) != nil || count != 21 {
		t.Fatal("fixture21 migrations")
	}
	manifest, err := os.ReadFile("/inputs/pinned-migrations.json")
	f4Check(t, err == nil, "pinned migration manifest")
	var migrations []struct{ Path, SHA256 string }
	f4Check(t, json.Unmarshal(manifest, &migrations) == nil && len(migrations) == 21, "pinned21 migration hashes")
	for i, migration := range migrations {
		var checksum string
		f4Check(t, owner.QueryRow(ctx, "SELECT checksum FROM platform_meta.schema_migrations WHERE version=$1", i+1).Scan(&checksum) == nil && checksum == migration.SHA256, "actual migration checksum drift")
	}
	_, err = owner.Exec(ctx, `INSERT INTO identity.account_refs(newapi_user_id) SELECT generate_series(1,5);
	GRANT USAGE ON SCHEMA identity,ops TO f4_session;
	GRANT SELECT(newapi_user_id,security_epoch,security_epoch_changed_at) ON identity.account_refs TO f4_session;
	GRANT SELECT ON identity.native_session_bindings TO f4_session;
	GRANT INSERT(session_id_hash,newapi_user_id,session_version,native_auth_version,security_epoch_snapshot,native_created_at) ON identity.native_session_bindings TO f4_session;
	GRANT UPDATE(session_version,native_auth_version) ON identity.native_session_bindings TO f4_session;
	GRANT SELECT(newapi_user_id,status,authz_epoch) ON ops.admin_principals TO f4_session;`)
	f4Check(t, err == nil, "fixture grants")
	pool, err := pgxpool.New(ctx, os.Getenv("F4_PLATFORM_DSN"))
	f4Check(t, err == nil, "fixture runtime pool")
	defer pool.Close()
	cache := f4Redis(t, false)
	if cache.Set(ctx, "chaldea:poker-ticket-used:forbidden", "x", time.Second).Err() == nil {
		t.Fatal("fixture runtime escaped Redis prefix")
	}
	if _, err := pool.Exec(ctx, "UPDATE identity.account_refs SET security_epoch=security_epoch+1 WHERE newapi_user_id=1"); err == nil {
		t.Fatal("fixture runtime escaped PG grant")
	}
	login := f4JSON(t, client, "POST", "/api/user/login", map[string]string{"username": "f4actor1", "password": os.Getenv("F4_PASSWORD")}, nil)
	token, ok := login["access_token"].(string)
	if !ok {
		t.Fatal("actual Native password login did not return token")
	}
	sid := login["session"].(map[string]any)["sid"].(string)
	f4Check(t, nativeself.Verify(ctx, native, 1, "f4actor1", token, sid) == nil, "actual Native self")
	var seal [32]byte
	sealBytes, _ := hex.DecodeString(os.Getenv("F4_SEAL_KEY"))
	copy(seal[:], sealBytes)
	s, err := New(Options{Redis: cache, Pool: pool, NativeSocket: os.Getenv("F4_NATIVE_SOCKET"), NativeReaderKey: os.Getenv("F4_READER_KEY"), OpsSocket: os.Getenv("F4_OPS_SOCKET"), OpsKey: os.Getenv("F4_OPS_KEY"), SessionSealKey: seal, Origin: "https://f4.test", Environment: "DEVELOPMENT", Lifetime: Lifetime{120 * time.Second, 600 * time.Second, 15 * time.Second}})
	f4Check(t, err == nil, "explicit Session constructor")
	defer s.Close()
	grant, err := s.AdoptNative(ctx, NativeCredential{"1", "f4actor1", sid, token})
	f4Check(t, err == nil && grant.View().UserID == "1" && grant.View().FreshAuthAt == nil, "real Native chain must yield non-Fresh opaque Session")
	if _, err = owner.Exec(ctx, `INSERT INTO ops.admin_principals(admin_principal_id,newapi_user_id,base_role)
	SELECT ('00000000-0000-0000-0000-00000000000'||id)::uuid,id,'OPERATOR' FROM generate_series(1,4) id`); err != nil {
		t.Fatal("fixture principals")
	}
	operation := Operation{ID: "b3c7be70-49cd-4a73-8428-f52fa934340d", Action: "maintenance.activate", TargetKind: "maintenance", TargetID: "516f6aca-f3d9-42b7-8b07-68b418fc1e6b", ExpectedVersion: "1", CommandDigest: sha256.Sum256([]byte(`{"state":"ACTIVE"}`)), ImpactDigest: sha256.Sum256([]byte(`{"state":"SCHEDULED"}`))}
	factsBefore := f4Facts(t, owner)
	harness := &f4HTTP{service: s, grant: grant, operation: operation}
	web := httptest.NewTLSServer(harness)
	defer web.Close()
	s.options.Origin = web.URL
	browser := web.Client()
	browser.Jar, _ = cookiejar.New(nil)
	before, cookies := f4Web(t, browser, web.URL+"/_f4/current", "GET", nil, "", 200)
	if len(cookies) != 1 || cookies[0].Name != cookieName || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].Domain != "" || cookies[0].Path != "/" || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("real TLS Cookie attributes")
	}
	oldCSRF := before["csrf_token"].(string)
	f4Web(t, browser, web.URL+"/_f4/fresh/start", "GET", nil, "", 400)
	start, _ := f4Web(t, browser, web.URL+"/_f4/fresh/start", "POST", nil, oldCSRF, 200)
	proof := f4JSON(t, client, "POST", "/api/momiao/ops/fresh/password", map[string]string{"challenge_token": start["challenge_token"].(string), "password": os.Getenv("F4_PASSWORD")}, map[string]string{"Authorization": "Bearer " + token, "New-Api-User": "1", "X-Auth-Session": sid})
	consumeBefore := f4JSON(t, client, "POST", "/_f4/control", map[string]string{"action": "status"}, nil)["consume_calls"]
	for _, change := range []func(*http.Request){
		func(r *http.Request) { r.Header.Del("Origin") },
		func(r *http.Request) { r.Header.Set("Origin", "https://foreign.test") },
		func(r *http.Request) { r.Header.Add("Origin", web.URL) },
		func(r *http.Request) { r.Header.Del("Sec-Fetch-Site") },
		func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
		func(r *http.Request) { r.Header.Add("Sec-Fetch-Site", "same-origin") },
		func(r *http.Request) { r.Header.Del("X-CSRF-Token") },
		func(r *http.Request) { r.Header.Set("X-CSRF-Token", "wrong") },
		func(r *http.Request) { r.Header.Add("X-CSRF-Token", oldCSRF) },
	} {
		f4Web(t, browser, web.URL+"/_f4/fresh/complete", "POST", map[string]any{"challenge_id": start["challenge_id"], "proof": proof["proof"]}, oldCSRF, 403, change)
	}
	f4Web(t, browser, web.URL+"/_f4/fresh/complete", "POST", map[string]any{"challenge_id": start["challenge_id"], "proof": proof["proof"], "expected_context": map[string]string{"action": "client.chosen"}}, oldCSRF, 400)
	if f4JSON(t, client, "POST", "/_f4/control", map[string]string{"action": "status"}, nil)["consume_calls"] != consumeBefore {
		t.Fatal("rejected HTTP metadata invoked Native consume")
	}
	after, rotated := f4Web(t, browser, web.URL+"/_f4/fresh/complete", "POST", map[string]any{"challenge_id": start["challenge_id"], "proof": proof["proof"]}, oldCSRF, 200)
	if len(rotated) != 1 || rotated[0].Value == cookies[0].Value || after["csrf_token"] == oldCSRF || after["fresh_auth_at"] == nil || after["fresh_auth_method"] != "password" || after["auth_chain_started_at"] != before["auth_chain_started_at"] || after["absolute_expires_at"] != before["absolute_expires_at"] {
		t.Fatal("actual Fresh must rotate both secrets without extending original Native chain")
	}
	f4Web(t, browser, web.URL+"/_f4/fresh/start", "POST", nil, oldCSRF, 403)
	admin := f4Redis(t, true)
	oldExists, oldErr := admin.Exists(ctx, sessionPrefix+secretHash(cookies[0].Value)).Result()
	newExists, newErr := admin.Exists(ctx, sessionPrefix+secretHash(rotated[0].Value)).Result()
	f4Check(t, oldErr == nil && newErr == nil && oldExists == 0 && newExists == 1, "Redis old/new authority")
	nativeOwner, err := pgxpool.New(ctx, os.Getenv("F4_NATIVE_OWNER_DSN"))
	f4Check(t, err == nil, "owned Native PAT seed connection")
	defer nativeOwner.Close()
	pat := secret()[:32]
	updated, err := nativeOwner.Exec(ctx, "UPDATE users SET access_token=$1 WHERE id=1", pat)
	f4Check(t, err == nil && updated.RowsAffected() == 1, "owned Native PAT seed")
	patSelf := f4JSON(t, client, "GET", "/api/user/self", nil, map[string]string{"Authorization": "Bearer " + pat, "New-Api-User": "1"})
	f4Check(t, patSelf["id"] == float64(1), "actual valid Native dashboard PAT positive control")
	for name, bad := range map[string]NativeCredential{
		"bogus signature":       {"1", "f4actor1", sid, strings.Join(strings.Split(token, ".")[:2], ".") + "." + strings.Repeat("a", 43)},
		"PAT-shaped credential": {"1", "f4actor1", sid, "test-access-token-placeholder-0001"},
		"actual valid PAT":      {"1", "f4actor1", sid, pat},
		"wrong actor":           {"2", "f4actor2", sid, token},
		"wrong SID":             {"1", "f4actor1", "00000000-0000-0000-0000-000000000001", token},
	} {
		candidate, err := s.AdoptNative(ctx, bad)
		if err == nil || candidate.owner != nil || candidate.sid != "" {
			t.Fatalf("adoption accepted %s", name)
		}
	}
	t.Log("FRESH actual TLS CookieJar/password/F3 consume/SID+CSRF rotation/absolute anchor PASS; business prepare remains synthetic")
	for actor := 2; actor <= 4; actor++ {
		bundle := f4Authenticate(t, client, actor, "", "")
		credential := NativeCredential{strconv.Itoa(actor), fmt.Sprintf("f4actor%d", actor), bundle["session"].(map[string]any)["sid"].(string), bundle["access_token"].(string)}
		harness.grant, err = s.AdoptNative(ctx, credential)
		f4Check(t, err == nil, "actual actor adoption")
		initial, _ := f4Web(t, browser, web.URL+"/_f4/current", "GET", nil, "", 200)
		start, _ := f4Web(t, browser, web.URL+"/_f4/fresh/start", "POST", nil, initial["csrf_token"].(string), 200)
		proof := f4Authenticate(t, client, actor, start["challenge_token"].(string), credential.AccessToken)
		fresh, _ := f4Web(t, browser, web.URL+"/_f4/fresh/complete", "POST", map[string]any{"challenge_id": start["challenge_id"], "proof": proof["proof"]}, initial["csrf_token"].(string), 200)
		f4Check(t, fresh["newapi_user_id"] == credential.UserID && fresh["fresh_auth_at"] != nil && fresh["csrf_token"] != initial["csrf_token"], "actual actor Fresh rotation")
		t.Logf("FACTOR actor%d actual Native factor/once-consume/TLS rotation PASS; controlled Discord provider synthetic", actor)
	}
	f4RuntimeBounds(t, s, admin, native, NativeCredential{"1", "f4actor1", sid, token}, secretHash(rotated[0].Value))
	f4Revocations(t, harness, web.URL, browser, client, owner, admin)
	f4Exchanges(t, harness, web.URL, browser, client, admin)
	f4Absolute(t, harness, web.URL, browser, client, admin)
	f4RemainingBoundaries(t, harness, web.URL, browser, client, admin)
	f4WireNegatives(t, harness, web.URL, browser, client, admin)
	beforeJSON, _ := json.Marshal(factsBefore)
	afterJSON, _ := json.Marshal(f4Facts(t, owner))
	f4Check(t, bytes.Equal(beforeJSON, afterJSON), "business or identity ownership facts changed")
	t.Logf("BOUNDARIES actual21 migration checksums and%d table projections unchanged; only Native auth flows, Session records and declared security/principal fields changed", len(factsBefore))
}

func f4Facts(t *testing.T, owner *pgxpool.Pool) map[string]string {
	t.Helper()
	rows, err := owner.Query(context.Background(), `SELECT format('%I.%I',schemaname,tablename) FROM pg_catalog.pg_tables
	WHERE schemaname NOT IN ('pg_catalog','information_schema') AND schemaname NOT LIKE 'pg_%'
	AND (schemaname,tablename) <> ('identity','native_session_bindings') ORDER BY 1`)
	f4Check(t, err == nil, "fixture fact table inventory")
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	f4Check(t, err == nil, "fixture fact inventory read")
	facts := map[string]string{}
	for _, table := range tables {
		expression := "to_jsonb(t)"
		if table == "identity.account_refs" {
			expression += "-ARRAY['security_epoch','security_epoch_changed_at']"
		}
		if table == "ops.admin_principals" {
			expression += "-ARRAY['status','authz_epoch','version','updated_at']"
		}
		var hash string
		err = owner.QueryRow(context.Background(), "SELECT md5(coalesce(jsonb_agg("+expression+" ORDER BY ("+expression+")::text)::text,'[]')) FROM "+table+" t").Scan(&hash)
		f4Check(t, err == nil, "fixture business fact hash")
		facts[table] = hash
	}
	return facts
}

func f4RemainingBoundaries(t *testing.T, h *f4HTTP, url string, browser, native *http.Client, admin *redis.Client) {
	t.Run("failed_proof_preserves_existing_fresh", func(t *testing.T) {
		state := f4Login(t, h, url, browser, native, 1)
		f4Proof(t, &state, url, browser, native)
		state.view, _ = f4Web(t, browser, url+"/_f4/fresh/complete", "POST", state.body(), state.view["csrf_token"].(string), 200)
		fresh := state.view["fresh_auth_at"]
		f4Proof(t, &state, url, browser, native)
		state.proof = secret()
		f4Web(t, browser, url+"/_f4/fresh/complete", "POST", state.body(), state.view["csrf_token"].(string), 401)
		current, _ := f4Web(t, browser, url+"/_f4/current", "GET", nil, "", 200)
		f4Check(t, fresh != nil && current["fresh_auth_at"] == fresh && current["csrf_token"] == state.view["csrf_token"], "failed proof cleared previous Fresh authority")
	})
	t.Run("absolute_expires_after_native_consume", func(t *testing.T) {
		original := h.service
		options := original.options
		options.Lifetime = Lifetime{2 * time.Second, 2 * time.Second, 20 * time.Millisecond}
		var err error
		h.service, err = New(options)
		f4Check(t, err == nil, "expiring exchange fixture")
		defer func() { h.service.Close(); h.service = original }()
		state := f4Login(t, h, url, browser, native, 1)
		f4Proof(t, &state, url, browser, native)
		f4Control(t, native, "arm", "", "", 0)
		defer f4Control(t, native, "release", "", "", 0)
		pending := make(chan f4Response, 1)
		go func() {
			pending <- f4Send(browser, url+"/_f4/fresh/complete", "POST", state.body(), state.view["csrf_token"].(string))
		}()
		f4WaitBarrier(t, native)
		expires, err := time.Parse(time.RFC3339Nano, state.view["absolute_expires_at"].(string))
		f4Check(t, err == nil, "expiring exchange timestamp")
		time.Sleep(time.Until(expires) + 25*time.Millisecond)
		f4Control(t, native, "release", "", "", 0)
		reply := <-pending
		f4Check(t, reply.err == nil && reply.status == 401, "expired consumed proof produced authority")
		for _, cookie := range reply.cookies {
			f4Check(t, cookie.Value == "", "expired exchange issued SID")
		}
	})
	t.Run("cookie_native_role_logout", func(t *testing.T) {
		state := f4Login(t, h, url, browser, native, 5)
		f4Web(t, browser, url+"/_f4/fresh/start", "POST", nil, state.view["csrf_token"].(string), 403)
		naked := *browser
		naked.Jar = nil
		for _, cookie := range []string{"", cookieName + "=invalid", cookieName + "=" + state.cookie.Value + "; " + cookieName + "=" + state.cookie.Value, "new_api_refresh=not-a-chaldea-session"} {
			f4Web(t, &naked, url+"/_f4/current", "GET", nil, "", 401, func(r *http.Request) {
				r.Header.Set("Cookie", cookie)
				r.Header.Set("Authorization", "Bearer "+state.native.AccessToken)
			})
		}
		f4JSON(t, native, "POST", "/api/user/auth/logout", map[string]string{}, map[string]string{"Authorization": "Bearer " + state.native.AccessToken, "X-Auth-Session": state.native.SID})
		f4Web(t, browser, url+"/_f4/current", "GET", nil, "", 401)
	})
	t.Run("cross_session_proof_and_nonce", func(t *testing.T) {
		first := f4Login(t, h, url, browser, native, 1)
		f4Proof(t, &first, url, browser, native)
		second := f4Login(t, h, url, browser, native, 1)
		f4Proof(t, &second, url, browser, native)
		f4Check(t, first.id != second.id && first.cookie.Value != second.cookie.Value && first.view["csrf_token"] != second.view["csrf_token"], "independent Session ceremony secrets")
		before := f4Control(t, native, "status", "", "", 0)
		f4Web(t, browser, url+"/_f4/fresh/complete", "POST", first.body(), second.view["csrf_token"].(string), 409)
		second.proof = first.proof
		reply := f4Send(browser, url+"/_f4/fresh/complete", "POST", second.body(), second.view["csrf_token"].(string))
		f4Check(t, reply.err == nil && reply.status != 200 && len(reply.cookies) == 0, "foreign chain proof granted Session")
		after := f4Control(t, native, "status", "", "", 0)
		f4Check(t, f4Consumed(after) == f4Consumed(before), "foreign proof consumed as current ceremony")
		old := second.id
		f4Proof(t, &second, url, browser, native)
		f4Check(t, second.id != old, "explicit new Start reused nonce")
		f4Web(t, browser, url+"/_f4/fresh/complete", "POST", map[string]string{"challenge_id": old, "proof": first.proof}, second.view["csrf_token"].(string), 409)
		stored, err := admin.Get(context.Background(), sessionPrefix+secretHash(second.cookie.Value)).Result()
		f4Check(t, err == nil && !strings.Contains(stored, second.cookie.Value) && !strings.Contains(stored, second.native.SID) && !strings.Contains(stored, second.native.AccessToken) && !strings.Contains(stored, second.proof), "record contains raw bearer/SID/proof")
	})
	t.Run("controlled_Discord_provider_outage", func(t *testing.T) {
		state := f4Login(t, h, url, browser, native, 3)
		start, _ := f4Web(t, browser, url+"/_f4/fresh/start", "POST", nil, state.view["csrf_token"].(string), 200)
		oauth := f4JSON(t, native, "POST", "/api/momiao/auth/discord/ops-fresh/start", map[string]any{"challenge_token": start["challenge_token"]}, map[string]string{"Authorization": "Bearer " + state.native.AccessToken, "New-Api-User": "3"})
		location, err := urlpkg.Parse(oauth["authorization_url"].(string))
		f4Check(t, err == nil, "provider fixture OAuth URL")
		f4Control(t, native, "provider", "", "", 1)
		defer f4Control(t, native, "provider", "", "", 0)
		request, _ := http.NewRequest("GET", "https://f4.test/api/momiao/auth/discord/callback?code=3&state="+urlpkg.QueryEscape(location.Query().Get("state")), nil)
		response, err := native.Do(request)
		f4Check(t, err == nil, "actual provider failure response")
		defer response.Body.Close()
		var body map[string]any
		err = json.NewDecoder(response.Body).Decode(&body)
		f4Check(t, err == nil && !(response.StatusCode == 200 && body["success"] == true), "provider outage succeeded")
		current, _ := f4Web(t, browser, url+"/_f4/current", "GET", nil, "", 200)
		f4Check(t, current["fresh_auth_at"] == nil && current["csrf_token"] == state.view["csrf_token"], "provider outage advanced platform Fresh")
	})
}

func f4IAT(token string) int64 {
	raw, _ := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[1])
	var claims struct {
		IAT int64 `json:"iat"`
	}
	json.Unmarshal(raw, &claims)
	return claims.IAT
}
func f4Absolute(t *testing.T, h *f4HTTP, url string, browser, native *http.Client, admin *redis.Client) {
	t.Run("absolute_refresh_touch_restart", func(t *testing.T) {
		original := h.service
		options := original.options
		options.Lifetime = Lifetime{3 * time.Second, 6 * time.Second, 20 * time.Millisecond}
		var err error
		h.service, err = New(options)
		f4Check(t, err == nil, "short explicit lifetime")
		defer func() { h.service.Close(); h.service = original }()
		anchor := time.Now().Unix() - 1
		state := f4Login(t, h, url, browser, native, 1, func(c NativeCredential) { f4Control(t, native, "mutate", "created_at", c.SID, anchor) })
		initial, err := h.service.load(context.Background(), secretHash(state.cookie.Value))
		f4Check(t, err == nil && milliseconds(initial.Idle) < milliseconds(initial.Absolute), "idle deadline must dominate initial TTL fixture")
		initialTTL, err := admin.PTTL(context.Background(), sessionPrefix+initial.Hash).Result()
		f4Check(t, err == nil && initialTTL > 0 && initialTTL <= time.Until(instant(initial.Idle))+20*time.Millisecond, "actual Redis TTL exceeded idle")
		chain, err := time.Parse(time.RFC3339Nano, state.view["auth_chain_started_at"].(string))
		f4Check(t, err == nil && chain.Unix() == anchor && chain.Unix() < f4IAT(state.native.AccessToken), "Native CreatedAt must anchor before JWT iat")
		absolute := state.view["absolute_expires_at"]
		for round := 0; round < 2; round++ {
			f4Proof(t, &state, url, browser, native)
			view, cookies := f4Web(t, browser, url+"/_f4/fresh/complete", "POST", state.body(), state.view["csrf_token"].(string), 200)
			f4Check(t, len(cookies) == 1 && view["absolute_expires_at"] == absolute && view["auth_chain_started_at"] == state.view["auth_chain_started_at"], "repeated real Fresh extended chain")
			state.view, state.cookie = view, cookies[0]
			time.Sleep(25 * time.Millisecond)
			touched, _ := f4Web(t, browser, url+"/_f4/current", "GET", nil, "", 200)
			f4Check(t, touched["absolute_expires_at"] == absolute, "touch extended absolute deadline")
		}
		if wait := time.Until(time.Unix(f4IAT(state.native.AccessToken)+1, 0)) + 20*time.Millisecond; wait > 0 {
			time.Sleep(wait)
		}
		refreshed := f4JSON(t, native, "POST", "/api/user/auth/refresh", map[string]string{}, map[string]string{"X-Auth-Session": state.native.SID})
		newToken := refreshed["access_token"].(string)
		f4Check(t, f4IAT(newToken) > f4IAT(state.native.AccessToken) && refreshed["session"].(map[string]any)["sid"] == state.native.SID, "actual refresh must advance iat on original Native SID")
		state.native.AccessToken = newToken
		h.grant, err = h.service.AdoptNative(context.Background(), state.native)
		f4Check(t, err == nil, "refreshed original chain adoption")
		adopted, cookies := f4Web(t, browser, url+"/_f4/current", "GET", nil, "", 200)
		f4Check(t, adopted["absolute_expires_at"] == absolute && adopted["fresh_auth_at"] == nil, "re-adoption reset chain or inherited Fresh")
		kept := cookies[0]
		h.service.Close()
		h.service, err = New(options)
		f4Check(t, err == nil, "reconstructed service")
		restored, _ := f4Web(t, browser, url+"/_f4/current", "GET", nil, "", 200)
		f4Check(t, restored["csrf_token"] == adopted["csrf_token"] && restored["absolute_expires_at"] == absolute, "reconstructed service changed persisted authority")
		h.grant, err = h.service.AdoptNative(context.Background(), state.native)
		f4Check(t, err == nil, "record deletion fixture adoption")
		_, deleted := f4Web(t, browser, url+"/_f4/current", "GET", nil, "", 200)
		f4Check(t, admin.Del(context.Background(), sessionPrefix+secretHash(deleted[0].Value)).Err() == nil, "delete exact fixture session")
		f4Web(t, browser, url+"/_f4/current", "GET", nil, "", 401, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+newToken) })
		expires, err := time.Parse(time.RFC3339Nano, absolute.(string))
		f4Check(t, err == nil, "absolute timestamp")
		pttl, err := admin.PTTL(context.Background(), sessionPrefix+secretHash(kept.Value)).Result()
		f4Check(t, err == nil && pttl > 0 && pttl <= time.Until(expires)+20*time.Millisecond, "actual Redis TTL exceeded absolute")
		naked := *browser
		naked.Jar = nil
		for _, cookie := range []*http.Cookie{state.cookie, kept} {
			f4Web(t, &naked, url+"/_f4/current", "GET", nil, "", 200, func(r *http.Request) { r.AddCookie(cookie) })
		}
		time.Sleep(time.Until(expires) + 30*time.Millisecond)
		for _, cookie := range []*http.Cookie{state.cookie, kept} {
			f4Web(t, &naked, url+"/_f4/current", "GET", nil, "", 401, func(r *http.Request) { r.AddCookie(cookie) })
		}
	})
	t.Run("stale_touch_after_revoke", func(t *testing.T) {
		original := h.service
		options := original.options
		options.Lifetime.Touch = 20 * time.Millisecond
		var err error
		h.service, err = New(options)
		f4Check(t, err == nil, "stale touch fixture")
		defer func() { h.service.Close(); h.service = original }()
		state := f4Login(t, h, url, browser, native, 1)
		stale, err := h.service.load(context.Background(), secretHash(state.cookie.Value))
		f4Check(t, err == nil, "actual Redis snapshot before revoke")
		time.Sleep(25 * time.Millisecond)
		f4Web(t, browser, url+"/_f4/logout", "POST", nil, state.view["csrf_token"].(string), 200)
		_, err = h.service.touch(context.Background(), stale)
		f4Check(t, err == conflictFault, "stale touch restored revoked record")
		remaining, err := admin.Exists(context.Background(), sessionPrefix+stale.Hash).Result()
		f4Check(t, err == nil && remaining == 0, "revoked Redis authority reappeared")
	})
}

func f4Consumed(snapshot map[string]any, ids ...float64) int {
	count := 0
	for _, raw := range snapshot["flows"].([]any) {
		flow := raw.(map[string]any)
		if flow["purpose"] == "momiao_ops_proof" && flow["consumed_at"] != nil && (len(ids) == 0 || flow["id"] == ids[0]) {
			count++
		}
	}
	return count
}

func f4WireNegatives(t *testing.T, h *f4HTTP, url string, browser, native *http.Client, admin *redis.Client) {
	type mutation struct {
		kind, field, value string
		expected           pending
	}
	var selected atomic.Pointer[mutation]
	var hits atomic.Int32
	var observed atomic.Bool
	original := h.service
	listener, err := net.Listen("unix", filepath.Join(t.TempDir(), "wire.sock"))
	f4Check(t, err == nil, "owned F3 wire proxy")
	proxy := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		m := selected.Load()
		change := func(raw []byte) []byte {
			var fields map[string]string
			if json.Unmarshal(raw, &fields) == nil && fields[m.field] != "" && fields[m.field] != m.value {
				fields[m.field] = m.value
				hits.Add(1)
				raw, _ = json.Marshal(fields)
			}
			return raw
		}
		request.URL.Scheme, request.URL.Host, request.Host = "http", "unix", "localhost"
		if m != nil && m.kind == "context" {
			var wire map[string]string
			json.NewDecoder(request.Body).Decode(&wire)
			request.Body.Close()
			wire["context"] = string(change([]byte(wire["context"])))
			raw, _ := json.Marshal(wire)
			request.Body, request.ContentLength = io.NopCloser(bytes.NewReader(raw)), int64(len(raw))
		}
		response, err := original.ops.RoundTrip(request)
		if err != nil {
			w.WriteHeader(503)
			return
		}
		defer response.Body.Close()
		raw, err := io.ReadAll(response.Body)
		if m != nil {
			if m.kind == "receipt" {
				var actual receipt
				observed.Store(err == nil && response.StatusCode == 200 && json.Unmarshal(raw, &actual) == nil && actual.valid(m.expected, time.Now().UnixMilli()))
				raw = change(raw)
			} else {
				observed.Store(response.StatusCode == 400 || response.StatusCode == 401)
			}
		}
		w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
		w.WriteHeader(response.StatusCode)
		w.Write(raw)
	})}
	go proxy.Serve(listener)
	defer proxy.Close()
	options := original.options
	options.OpsSocket = listener.Addr().String()
	h.service, err = New(options)
	f4Check(t, err == nil, "actual F3 proxy Service")
	defer func() { h.service.Close(); h.service = original }()
	state := f4Login(t, h, url, browser, native, 1)
	f4Proof(t, &state, url, browser, native)
	view, cookies := f4Web(t, browser, url+"/_f4/fresh/complete", "POST", state.body(), state.view["csrf_token"].(string), 200)
	f4Check(t, view["fresh_auth_at"] != nil && len(cookies) == 1, "same Unix proxy actual positive control")
	state.view, state.cookie = view, cookies[0]
	for kind, cases := range map[string]map[string]string{
		"context": {
			"version":                "2",
			"operation_id":           "00000000-0000-0000-0000-000000000099",
			"actor_user_id":          "2",
			"native_sid_hash":        strings.Repeat("f", 64),
			"native_session_version": "999",
			"native_auth_version":    "999",
			"platform_session_hash":  strings.Repeat("f", 64),
			"security_epoch":         "999",
			"authz_epoch":            "999",
			"action":                 "maintenance.cancel",
			"target_kind":            "other",
			"target_id":              "00000000-0000-0000-0000-000000000099",
			"expected_version":       "2",
			"environment":            "STAGING",
			"command_digest":         strings.Repeat("f", 64),
			"impact_digest":          strings.Repeat("f", 64),
			"nonce_hash":             strings.Repeat("f", 64),
		},
		"receipt": {
			"proof_id":               "01",
			"purpose":                "LOGIN",
			"actor_user_id":          "2",
			"native_sid_hash":        strings.Repeat("f", 64),
			"native_session_version": "999",
			"native_auth_version":    "999",
			"context_hash":           strings.Repeat("f", 64),
			"authenticated_at":       strconv.FormatInt(time.Now().Unix()+60, 10),
			"expires_at":             "1",
			"method":                 "webauthn",
		},
	} {
		for field, value := range cases {
			t.Run("wire/"+kind+"/"+field, func(t *testing.T) {
				f4Proof(t, &state, url, browser, native)
				saved, err := h.service.load(context.Background(), secretHash(state.cookie.Value))
				f4Check(t, err == nil && saved.Pending != nil && saved.Fresh != nil, "real nonempty Fresh and pending wire baseline")
				before := f4Control(t, native, "status", "", "", 0)
				var proofID float64
				for _, item := range before["flows"].([]any) {
					flow := item.(map[string]any)
					if flow["purpose"] == "momiao_ops_proof" && flow["consumed_at"] == nil {
						proofID = flow["id"].(float64)
					}
				}
				keys, err := admin.Keys(context.Background(), sessionPrefix+"*").Result()
				f4Check(t, err == nil && proofID > 0, "exact new proof and authority inventory")
				hits.Store(0)
				observed.Store(false)
				selected.Store(&mutation{kind, field, value, *saved.Pending})
				reply := f4Send(browser, url+"/_f4/fresh/complete", "POST", state.body(), state.view["csrf_token"].(string))
				selected.Store(nil)
				f4Check(t, reply.err == nil && (reply.status == 401 || reply.status == 503) && len(reply.cookies) == 0 && hits.Load() == 1 && observed.Load(), "actual wire mutation not rejected semantically exactly once")
				after := f4Control(t, native, "status", "", "", 0)
				delta := 0
				if kind == "receipt" {
					delta = 1
				}
				f4Check(t, f4Consumed(after, proofID) == delta && after["consume_calls"].(float64)-before["consume_calls"].(float64) == 1, "exact actual proof consumption changed")
				current, _ := f4Web(t, browser, url+"/_f4/current", "GET", nil, "", 200)
				f4Check(t, current["fresh_auth_at"] == state.view["fresh_auth_at"] && current["csrf_token"] == state.view["csrf_token"], "invalid wire advanced or cleared old Fresh/CSRF")
				remaining, err := admin.Keys(context.Background(), sessionPrefix+"*").Result()
				slices.Sort(keys)
				slices.Sort(remaining)
				f4Check(t, err == nil && slices.Equal(remaining, keys), "invalid wire created replacement authority")
			})
		}
	}
}

type f4LostConn struct {
	net.Conn
	final    bool
	observed *atomic.Bool
}

func (c *f4LostConn) Write(p []byte) (int, error) {
	parts := bytes.Split(p, []byte("\r\n"))
	n, err := c.Conn.Write(p)
	if err == nil && n == len(p) && len(parts) > 6 && string(parts[2]) == "eval" && string(parts[6]) == "2" {
		c.final = true
	}
	return n, err
}
func (c *f4LostConn) Read(p []byte) (int, error) {
	if !c.final {
		return c.Conn.Read(p)
	}
	reply, err := bufio.NewReader(c.Conn).ReadString('\n')
	if err == nil && reply == ":1\r\n" {
		c.observed.Store(true)
	}
	c.Conn.Close()
	return 0, io.ErrUnexpectedEOF
}
func f4Exchanges(t *testing.T, h *f4HTTP, url string, browser, native *http.Client, admin *redis.Client) {
	for _, mode := range []string{"concurrent", "revoke", "timeout", "acl", "lost", "redis_unavailable"} {
		t.Run("exchange/"+mode, func(t *testing.T) {
			state := f4Login(t, h, url, browser, native, 1)
			f4Proof(t, &state, url, browser, native)
			var observed atomic.Bool
			if mode == "lost" {
				original := h.service
				ro := *original.options.Redis.Options()
				ro.MaxRetries = -1
				dial := ro.Dialer
				ro.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
					conn, err := dial(ctx, network, addr)
					if err != nil {
						return nil, err
					}
					return &f4LostConn{Conn: conn, observed: &observed}, nil
				}
				cache := redis.NewClient(&ro)
				defer cache.Close()
				o := original.options
				o.Redis = cache
				var err error
				h.service, err = New(o)
				f4Check(t, err == nil, "lost reply client")
				defer func() { h.service.Close(); h.service = original }()
			}
			before := f4Control(t, native, "status", "", "", 0)
			keys, err := admin.Keys(context.Background(), sessionPrefix+"*").Result()
			f4Check(t, err == nil, "fixture session inventory")
			oldKey := sessionPrefix + secretHash(state.cookie.Value)
			if mode == "redis_unavailable" {
				f4Check(t, admin.Do(context.Background(), "ACL", "SETUSER", "f4session", "-eval").Err() == nil, "deny actual Redis EVAL")
			} else {
				f4Control(t, native, "arm", "", "", 0)
			}
			defer f4Control(t, native, "release", "", "", 0)
			defer admin.Do(context.Background(), "ACL", "SETUSER", "f4session", "+eval", "+set")
			pending := make(chan f4Response, 1)
			go func() {
				pending <- f4Send(browser, url+"/_f4/fresh/complete", "POST", state.body(), state.view["csrf_token"].(string))
			}()
			if mode != "redis_unavailable" {
				f4WaitBarrier(t, native)
			}
			switch mode {
			case "concurrent":
				f4Web(t, browser, url+"/_f4/fresh/complete", "POST", state.body(), state.view["csrf_token"].(string), 409)
			case "revoke":
				f4Web(t, browser, url+"/_f4/logout", "POST", nil, state.view["csrf_token"].(string), 200)
			case "acl":
				f4Check(t, admin.Do(context.Background(), "ACL", "SETUSER", "f4session", "-set").Err() == nil, "deny actual final SET")
			}
			if mode != "timeout" {
				f4Control(t, native, "release", "", "", 0)
			}
			reply := <-pending
			want := 503
			if mode == "concurrent" {
				want = 200
			}
			if mode == "revoke" {
				want = 409
			}
			if reply.err != nil || reply.status != want {
				t.Fatalf("exchange %s expected%d got%d", mode, want, reply.status)
			}
			if mode != "concurrent" {
				for _, cookie := range reply.cookies {
					if cookie.Value != "" {
						t.Fatal("failed exchange sent new SID")
					}
				}
			}
			f4Check(t, admin.Do(context.Background(), "ACL", "SETUSER", "f4session", "+eval", "+set").Err() == nil, "restore fixture ACL")
			f4Control(t, native, "release", "", "", 0)
			after := f4Control(t, native, "status", "", "", 0)
			delta := 1
			if mode == "redis_unavailable" {
				delta = 0
			}
			if after["consume_calls"].(float64)-before["consume_calls"].(float64) != float64(delta) || f4Consumed(after)-f4Consumed(before) != delta {
				t.Fatal("Native endpoint/actual consumed_at once facts")
			}
			remaining, err := admin.Keys(context.Background(), sessionPrefix+"*").Result()
			f4Check(t, err == nil, "fixture final session inventory")
			newKeys := []string{}
			known := map[string]bool{}
			for _, key := range keys {
				known[key] = true
			}
			for _, key := range remaining {
				if !known[key] {
					newKeys = append(newKeys, key)
				}
			}
			oldExists, err := admin.Exists(context.Background(), oldKey).Result()
			f4Check(t, err == nil, "fixture old authority query")
			if mode == "timeout" || mode == "redis_unavailable" {
				if oldExists != 1 || len(newKeys) != 0 {
					t.Fatal("pre-finalize failure changed authority")
				}
				record, err := h.service.load(context.Background(), secretHash(state.cookie.Value))
				f4Check(t, err == nil && record.Fresh == nil, "failed exchange advanced Fresh")
			} else {
				newCount := 0
				if mode == "lost" || mode == "concurrent" {
					newCount = 1
				}
				if oldExists != 0 || len(newKeys) != newCount {
					t.Fatal("finalization old/new authority mismatch")
				}
			}
			if mode == "lost" && !observed.Load() {
				t.Fatal("real Redis success reply was not observed before connection loss")
			}
			if mode == "lost" {
				raw, err := admin.Get(context.Background(), newKeys[0]).Bytes()
				var orphan record
				f4Check(t, err == nil && json.Unmarshal(raw, &orphan) == nil && orphan.Fresh != nil && orphan.Confirmation != nil && orphan.Confirmation.SourceSessionHash == secretHash(state.cookie.Value), "unknown-result orphan lacks actual Fresh receipt binding")
			}
			if mode == "concurrent" {
				f4Web(t, browser, url+"/_f4/fresh/complete", "POST", state.body(), reply.body["csrf_token"].(string), 409)
				if len(reply.cookies) != 1 || reply.cookies[0].Value == state.cookie.Value {
					t.Fatal("concurrent winner Cookie")
				}
			}
		})
	}
}

type f4Session struct {
	native    NativeCredential
	view      map[string]any
	cookie    *http.Cookie
	id, proof string
}

func f4Login(t *testing.T, h *f4HTTP, url string, browser, native *http.Client, actor int, before ...func(NativeCredential)) f4Session {
	t.Helper()
	login := f4Authenticate(t, native, actor, "", "")
	c := NativeCredential{strconv.Itoa(actor), fmt.Sprintf("f4actor%d", actor), login["session"].(map[string]any)["sid"].(string), login["access_token"].(string)}
	for _, prepare := range before {
		prepare(c)
	}
	var err error
	h.grant, err = h.service.AdoptNative(context.Background(), c)
	f4Check(t, err == nil, "fixture fresh Native adoption")
	view, cookies := f4Web(t, browser, url+"/_f4/current", "GET", nil, "", 200)
	f4Check(t, len(cookies) == 1 && view["fresh_auth_at"] == nil, "fresh Native adoption Cookie")
	saved, err := h.service.options.Redis.Get(context.Background(), sessionPrefix+secretHash(cookies[0].Value)).Result()
	f4Check(t, err == nil && !strings.Contains(saved, c.SID) && !strings.Contains(saved, c.AccessToken) && !strings.Contains(saved, cookies[0].Value), "Session stored raw Native or platform credentials")
	for _, private := range []string{os.Getenv("F4_PASSWORD"), os.Getenv("F4_BACKUP_CODE"), os.Getenv("F4_TOTP_SECRET")} {
		f4Check(t, private != "" && !strings.Contains(saved, private), "Session record exposed factor secret")
	}
	return f4Session{native: c, view: view, cookie: cookies[0]}
}
func f4Proof(t *testing.T, s *f4Session, url string, browser, native *http.Client) {
	t.Helper()
	start, _ := f4Web(t, browser, url+"/_f4/fresh/start", "POST", nil, s.view["csrf_token"].(string), 200)
	actor, _ := strconv.Atoi(s.native.UserID)
	proof := f4Authenticate(t, native, actor, start["challenge_token"].(string), s.native.AccessToken)
	s.id, s.proof = start["challenge_id"].(string), proof["proof"].(string)
}
func (s f4Session) body() any { return map[string]string{"challenge_id": s.id, "proof": s.proof} }
func f4Control(t *testing.T, client *http.Client, action, field, sid string, value int64) map[string]any {
	t.Helper()
	return f4JSON(t, client, "POST", "/_f4/control", map[string]any{"action": action, "actor": 1, "sid": sid, "field": field, "value": value}, nil)
}
func f4WaitBarrier(t *testing.T, client *http.Client) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for f4Control(t, client, "status", "", "", 0)["waiting"] != true {
		if time.Now().After(deadline) {
			t.Fatal("actual Native after-consume barrier not reached")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func f4Revocations(t *testing.T, h *f4HTTP, url string, browser, native *http.Client, owner *pgxpool.Pool, admin *redis.Client) {
	for _, stage := range []string{"before_start", "after_factor", "after_consume"} {
		for _, field := range []string{"revoked", "status", "deleted", "uv", "sv", "security_epoch", "principal", "authz_epoch"} {
			t.Run(stage+"/"+field, func(t *testing.T) {
				state := f4Login(t, h, url, browser, native, 1)
				csrf := state.view["csrf_token"].(string)
				if stage != "before_start" {
					f4Proof(t, &state, url, browser, native)
				}
				var pending chan f4Response
				if stage == "after_consume" {
					f4Control(t, native, "arm", "", "", 0)
					defer f4Control(t, native, "release", "", "", 0)
					pending = make(chan f4Response, 1)
					go func() { pending <- f4Send(browser, url+"/_f4/fresh/complete", "POST", state.body(), csrf) }()
					f4WaitBarrier(t, native)
				}
				want := 401
				switch field {
				case "security_epoch":
					if _, err := owner.Exec(context.Background(), "UPDATE identity.account_refs SET security_epoch=security_epoch+1 WHERE newapi_user_id=1"); err != nil {
						t.Fatal("fixture security epoch")
					}
				case "principal", "authz_epoch":
					status := "ACTIVE"
					if field == "principal" {
						status = "DISABLED"
					}
					if _, err := owner.Exec(context.Background(), "UPDATE ops.admin_principals SET status=$1 WHERE newapi_user_id=1", status); err != nil {
						t.Fatal("fixture principal boundary")
					}
					want = 403
					if field == "authz_epoch" && stage == "before_start" {
						want = 200
					}
				default:
					value := int64(2)
					if field == "deleted" {
						value = 1
					}
					f4Control(t, native, "mutate", field, state.native.SID, value)
				}
				var reply f4Response
				if pending != nil {
					f4Control(t, native, "release", "", "", 0)
					reply = <-pending
				} else {
					path, body := "/_f4/fresh/complete", state.body()
					if stage == "before_start" {
						path, body = "/_f4/fresh/start", nil
					}
					reply = f4Send(browser, url+path, "POST", body, csrf)
				}
				if reply.err != nil || reply.status != want {
					t.Fatalf("revocation expected%d got%d", want, reply.status)
				}
				for _, cookie := range reply.cookies {
					if cookie.Value != "" {
						t.Fatal("revocation issued replacement SID")
					}
				}
				exists, err := admin.Exists(context.Background(), sessionPrefix+secretHash(state.cookie.Value)).Result()
				if err != nil || (want == 401 && exists != 0) || (want != 401 && exists != 1) {
					t.Fatal("revocation old authority mismatch")
				}
				if field == "security_epoch" {
					if _, err := h.service.AdoptNative(context.Background(), state.native); err == nil {
						t.Fatal("epoch changed old chain re-adopted")
					}
					time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second)) + 10*time.Millisecond)
				}
				if field == "principal" {
					if _, err := owner.Exec(context.Background(), "UPDATE ops.admin_principals SET status='ACTIVE' WHERE newapi_user_id=1"); err != nil {
						t.Fatal("fixture principal restore")
					}
				}
				if field == "status" || field == "uv" || field == "deleted" || field == "sv" {
					value := int64(1)
					if field == "deleted" {
						value = 0
					}
					f4Control(t, native, "mutate", field, state.native.SID, value)
				}
			})
		}
	}
}

func f4RuntimeBounds(t *testing.T, s *Service, admin *redis.Client, native *http.Transport, credential NativeCredential, hash string) {
	t.Run("Redis_complete_RPC_deadline", func(t *testing.T) {
		ro := *s.options.Redis.Options()
		ro.MaxRetries, ro.PoolSize, ro.MaxActiveConns = -1, 1, 1
		ro.ReadTimeout, ro.WriteTimeout, ro.PoolTimeout = 2*time.Second, 2*time.Second, 2*time.Second
		cache := redis.NewClient(&ro)
		defer cache.Close()
		held := cache.Conn()
		f4Check(t, held.Ping(context.Background()).Err() == nil, "held actual Redis connection")
		defer held.Close()
		options := s.options
		options.Redis = cache
		bounded, err := New(options)
		f4Check(t, err == nil, "bounded fixture service")
		defer bounded.Close()
		f4Check(t, admin.Do(context.Background(), "CLIENT", "PAUSE", 2500, "ALL").Err() == nil, "actual fixture Redis pause")
		go func() { time.Sleep(700 * time.Millisecond); held.Close() }()
		started := time.Now()
		_, err = bounded.load(context.Background(), hash)
		if err != unavailableFault || time.Since(started) > 2300*time.Millisecond {
			t.Fatal("Redis full RPC exceeded2s or incorrectly returned authority")
		}
	})
	for _, mode := range []string{"truncated", "timeout"} {
		t.Run("Native_self_body_"+mode, func(t *testing.T) {
			listener, err := net.Listen("unix", filepath.Join(t.TempDir(), "self.sock"))
			f4Check(t, err == nil, "owned Native fault proxy socket")
			var observed atomic.Bool
			proxy := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				request := r.Clone(r.Context())
				request.URL.Scheme, request.URL.Host, request.Host = "http", "unix", "localhost"
				response, err := native.RoundTrip(request)
				if err != nil {
					w.WriteHeader(503)
					return
				}
				defer response.Body.Close()
				if response.StatusCode != 200 || response.ContentLength < 2 {
					w.WriteHeader(503)
					return
				}
				conn, buffer, err := w.(http.Hijacker).Hijack()
				if err != nil {
					return
				}
				defer conn.Close()
				fmt.Fprintf(buffer, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n", response.ContentLength)
				if _, err = io.CopyN(buffer, response.Body, 1); err != nil || buffer.Flush() != nil {
					return
				}
				observed.Store(true)
				if mode == "timeout" {
					time.Sleep(2100 * time.Millisecond)
				}
			})}
			go proxy.Serve(listener)
			defer proxy.Close()
			options := s.options
			options.NativeSocket = listener.Addr().String()
			broken, err := New(options)
			f4Check(t, err == nil, "Native proxy fixture constructor")
			defer broken.Close()
			grant, err := broken.AdoptNative(context.Background(), credential)
			if err != unavailableFault || grant.owner != nil || !observed.Load() {
				t.Fatal("real HTTP200 body I/O failure must be503 without Grant")
			}
		})
	}
}

func f4Authenticate(t *testing.T, client *http.Client, actor int, challenge, token string) map[string]any {
	t.Helper()
	headers := map[string]string{"Authorization": "Bearer " + token, "New-Api-User": strconv.Itoa(actor)}
	var result map[string]any
	second := "/api/user/login/2fa"
	if actor == 3 || actor == 4 {
		path := "/api/momiao/auth/discord/login/start"
		body := map[string]string{}
		if challenge != "" {
			path = "/api/momiao/auth/discord/ops-fresh/start"
			body["challenge_token"] = challenge
		}
		start := f4JSON(t, client, "POST", path, body, headers)
		location, err := urlpkg.Parse(start["authorization_url"].(string))
		f4Check(t, err == nil, "actual Native OAuth start")
		result = f4JSON(t, client, "GET", "/api/momiao/auth/discord/callback?code="+strconv.Itoa(actor)+"&state="+urlpkg.QueryEscape(location.Query().Get("state")), nil, nil)
		second = "/api/momiao/auth/2fa"
	} else {
		path := "/api/user/login"
		body := map[string]string{"username": fmt.Sprintf("f4actor%d", actor), "password": os.Getenv("F4_PASSWORD")}
		if challenge != "" {
			path = "/api/momiao/ops/fresh/password"
			body = map[string]string{"challenge_token": challenge, "password": os.Getenv("F4_PASSWORD")}
		}
		result = f4JSON(t, client, "POST", path, body, headers)
	}
	if actor == 2 || actor == 4 {
		if result["require_2fa"] != true {
			t.Fatal("actual Native required second factor missing")
		}
		code := os.Getenv("F4_BACKUP_CODE")
		if challenge != "" {
			second = "/api/momiao/ops/fresh/2fa"
			key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(os.Getenv("F4_TOTP_SECRET"))
			var counter [8]byte
			binary.BigEndian.PutUint64(counter[:], uint64(time.Now().Unix()/30))
			mac := hmac.New(sha1.New, key)
			mac.Write(counter[:])
			sum := mac.Sum(nil)
			offset := sum[len(sum)-1] & 15
			code = fmt.Sprintf("%06d", (binary.BigEndian.Uint32(sum[offset:offset+4])&0x7fffffff)%1000000)
		}
		result = f4JSON(t, client, "POST", second, map[string]any{"flow_token": result["flow_token"], "code": code}, headers)
	}
	return result
}

type f4HTTP struct {
	service   *Service
	grant     Grant
	operation Operation
}

func (h *f4HTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var value any
	var err error
	if h.grant.owner != nil && r.URL.Path == "/_f4/current" {
		err, value = h.service.WriteGrant(w, h.grant), h.grant.View()
		h.grant = Grant{}
	} else {
		var current RequestSession
		current, err = h.service.VerifyRequest(r)
		if err == nil {
			switch r.URL.Path {
			case "/_f4/current":
				value = current.View()
			case "/_f4/fresh/start":
				value, err = h.service.StartOps(r.Context(), current, h.operation)
			case "/_f4/fresh/complete":
				var input struct {
					ID    string `json:"challenge_id"`
					Proof string `json:"proof"`
				}
				decoder := json.NewDecoder(io.LimitReader(r.Body, 1024))
				decoder.DisallowUnknownFields()
				if decoder.Decode(&input) != nil {
					err = inputFault
					break
				}
				var next Grant
				next, err = h.service.CompleteOps(r.Context(), current, input.ID, input.Proof)
				if err == nil {
					err, value = h.service.WriteGrant(w, next), next.View()
				}
			case "/_f4/logout":
				err = h.service.Revoke(r.Context(), current)
				if err == nil {
					h.service.ClearCookie(w)
				}
			default:
				err = inputFault
			}
		}
	}
	if err != nil {
		fault, ok := err.(Fault)
		if !ok {
			fault = unavailableFault
		}
		if fault.ClearCookie {
			h.service.ClearCookie(w)
		}
		w.WriteHeader(fault.Status)
		value = map[string]string{"code": fault.Code}
	}
	json.NewEncoder(w).Encode(value)
}
func f4Web(t *testing.T, client *http.Client, url, method string, body any, csrf string, want int, changes ...func(*http.Request)) (map[string]any, []*http.Cookie) {
	t.Helper()
	reply := f4Send(client, url, method, body, csrf, changes...)
	if reply.err != nil || reply.status != want {
		t.Fatalf("TLS fixture wanted %d got %d (transport failed=%t)", want, reply.status, reply.err != nil)
	}
	return reply.body, reply.cookies
}

type f4Response struct {
	status  int
	body    map[string]any
	cookies []*http.Cookie
	err     error
}

func f4Send(client *http.Client, url, method string, body any, csrf string, changes ...func(*http.Request)) f4Response {
	raw, _ := json.Marshal(body)
	r, err := http.NewRequest(method, url, bytes.NewReader(raw))
	if err != nil {
		return f4Response{err: err}
	}
	r.Header.Set("Origin", r.URL.Scheme+"://"+r.URL.Host)
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("X-CSRF-Token", csrf)
	r.Header.Set("Content-Type", "application/json")
	for _, change := range changes {
		change(r)
	}
	response, err := client.Do(r)
	if err != nil {
		return f4Response{err: err}
	}
	defer response.Body.Close()
	var out map[string]any
	err = json.NewDecoder(io.LimitReader(response.Body, 16384)).Decode(&out)
	return f4Response{response.StatusCode, out, response.Cookies(), err}
}

func f4Check(t *testing.T, ok bool, message string) {
	t.Helper()
	if !ok {
		t.Fatal(message)
	}
}
