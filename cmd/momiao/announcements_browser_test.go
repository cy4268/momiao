package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

// Manual local browser acceptance fixture. Test-only compilation, fixed loopback
// listener, dedicated synthetic DB, and no connection to an actual native service.
func TestAnnouncementBrowserFixture(t *testing.T) {
	if os.Getenv("MOMIAO_ANNOUNCEMENTS_BROWSER_FIXTURE") != "1" {
		t.Skip("explicit local browser fixture only")
	}
	cfg, err := pgx.ParseConfig(os.Getenv("MOMIAO_ANNOUNCEMENTS_TEST_DATABASE_URL"))
	if err != nil || cfg.Host != "127.0.0.1" || cfg.Database != "m3_announcements_20260906" {
		t.Fatal("isolated fixture configuration required")
	}
	cfg.Database = "m3_announcements_browser_20260906"
	fixtureURL, err := url.Parse(cfg.ConnString())
	if err != nil {
		t.Fatal("invalid local fixture URL")
	}
	fixtureURL.Path = "/" + cfg.Database
	ctx := context.Background()
	store, err := platform.Open(ctx, fixtureURL.String())
	if err != nil {
		t.Fatal("local fixture database unavailable")
	}
	defer store.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal("fixture seed connection unavailable")
	}
	// Explicit synthetic identity; no HTTP bootstrap or production bootstrap is provided.
	_, err = conn.Exec(ctx, `INSERT INTO ops.admin_principals(admin_principal_id,newapi_user_id,base_role) VALUES('01990000-1111-7777-aaaa-000000000301',910000001,'SUPER_ADMIN') ON CONFLICT(newapi_user_id) DO NOTHING`)
	conn.Close(ctx)
	if err != nil {
		t.Fatal(err)
	}
	makeToken := func(id int64) string {
		claims := map[string]any{"iss": "new-api", "aud": []string{"new-api-dashboard"}, "token_use": "access", "sub": strconv.FormatInt(id, 10), "sid": "m3-synthetic-session-" + strconv.FormatInt(id, 10), "uv": 1, "sv": 1, "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nbf": time.Now().Add(-5 * time.Second).Unix(), "jti": "m3-browser-fixture"}
		b, _ := json.Marshal(claims)
		raw := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(b)
		mac := hmac.New(sha256.New, []byte("public-synthetic-test-key-not-a-native-secret"))
		mac.Write([]byte(raw))
		return raw + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	}
	tokens := map[int64]string{910000001: makeToken(910000001), 910000002: makeToken(910000002)}
	var mu sync.Mutex
	active := map[int64]bool{}
	user := func(id int64) map[string]any {
		role := 1
		if id == 910000002 {
			role = 100
		}
		return map[string]any{"id": id, "username": "m3-synthetic-" + strconv.FormatInt(id, 10), "display_name": "本地合成验收身份", "role": role, "status": 1}
	}
	bundle := func(id int64) map[string]any {
		return map[string]any{"access_token": tokens[id], "access_expires_at": time.Now().Add(time.Hour).Unix(), "session": map[string]any{"sid": "m3-synthetic-session-" + strconv.FormatInt(id, 10)}, "user": user(id)}
	}
	transport := walletTransport(func(r *http.Request) (*http.Response, error) {
		id, _ := strconv.ParseInt(r.Header.Get("New-Api-User"), 10, 64)
		mu.Lock()
		valid := active[id] && r.Header.Get("Authorization") == "Bearer "+tokens[id]
		mu.Unlock()
		status := 401
		body := []byte(`{"success":false}`)
		if valid {
			status = 503
		} // Unimplemented native features are outside this fixture.
		if valid && r.URL.Path == "/api/user/self" {
			status = 200
			body, _ = json.Marshal(map[string]any{"success": true, "data": user(id)})
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})
	web, err := filepath.Abs("../../web/dist")
	if err != nil {
		t.Fatal(err)
	}
	portal := newPortalHandler(config{WebDir: web, PublicOrigin: "http://127.0.0.1:14211", announcements: store}, transport)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			var b struct{ Username, Password string }
			if json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&b) != nil || b.Password != "m3-synthetic-only" {
				walletError(w, 401, "AUTH_UNAUTHORIZED")
				return
			}
			var id int64
			switch b.Username {
			case "m3-review-admin":
				id = 910000001
			case "m3-review-reader":
				id = 910000002
			default:
				walletError(w, 401, "AUTH_UNAUTHORIZED")
				return
			}
			mu.Lock()
			active[id] = true
			mu.Unlock()
			http.SetCookie(w, &http.Cookie{Name: "m3_synthetic_fixture", Value: strconv.FormatInt(id, 10), Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			walletSuccess(w, bundle(id))
		case "/api/user/auth/refresh":
			cookie, err := r.Cookie("m3_synthetic_fixture")
			if err != nil {
				walletError(w, 401, "AUTH_UNAUTHORIZED")
				return
			}
			id, _ := strconv.ParseInt(cookie.Value, 10, 64)
			mu.Lock()
			valid := active[id]
			mu.Unlock()
			if !valid {
				walletError(w, 401, "AUTH_UNAUTHORIZED")
				return
			}
			walletSuccess(w, bundle(id))
		case "/api/user/auth/logout":
			if c, e := r.Cookie("m3_synthetic_fixture"); e == nil {
				id, _ := strconv.ParseInt(c.Value, 10, 64)
				mu.Lock()
				active[id] = false
				mu.Unlock()
			}
			http.SetCookie(w, &http.Cookie{Name: "m3_synthetic_fixture", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
			walletSuccess(w, map[string]any{})
		default:
			portal.ServeHTTP(w, r)
		}
	})
	listener, err := net.Listen("tcp", "127.0.0.1:14211")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	t.Log("SYNTHETIC_FIXTURE_READY http://127.0.0.1:14211 (no real native identity or financial service)")
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go runAnnouncementWorker(workerCtx, store)
	if err = server.Serve(listener); err != nil && err != http.ErrServerClosed {
		t.Fatal(err)
	}
}
