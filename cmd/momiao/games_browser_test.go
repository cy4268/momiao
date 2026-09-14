package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

func gameBrowserStores(t *testing.T) (*platform.Store, *platform.Store) {
	t.Helper()
	file := os.Getenv("MOMIAO_GAMES_TEST_CONNECTION_FILE")
	if file == "" {
		t.Skip("isolated local G1 connection required")
	}
	raw, err := os.ReadFile(file)
	var c struct{ OwnerURL, RuntimeURL, RuntimeRole string }
	if err != nil || json.Unmarshal(raw, &c) != nil {
		t.Fatal("cannot read isolated game connection")
	}
	for _, url := range []string{c.OwnerURL, c.RuntimeURL} {
		cfg, e := pgx.ParseConfig(url)
		if e != nil || cfg.Host != "127.0.0.1" || cfg.Port != 55432 || !strings.HasPrefix(cfg.Database, "momiao_test_g1_original_") {
			t.Fatal("refusing non-G1 loopback database")
		}
	}
	ctx := context.Background()
	owner, err := platform.Open(ctx, c.OwnerURL)
	if err != nil {
		t.Fatal("isolated owner connection failed")
	}
	t.Cleanup(owner.Close)
	if os.Getenv("MOMIAO_GAMES_TEST_ISOLATED_GAP") == "1" {
		// Read-only acceptance of the explicit local pending receipt produced by
		// the game service integration fixture. Never fake missing global versions.
		err = owner.WithTx(ctx, func(tx pgx.Tx) error {
			for _, version := range []int{15, 16} {
				paths, e := filepath.Glob(fmt.Sprintf("../../internal/platform/migrations/%04d_*.sql", version))
				if e != nil {
					return e
				}
				if len(paths) == 0 {
					continue
				}
				if len(paths) != 1 {
					return fmt.Errorf("duplicate pending migration")
				}
				raw, e := os.ReadFile(paths[0])
				if e != nil {
					return e
				}
				hash := sha256.Sum256(raw)
				var saved string
				if e = tx.QueryRow(ctx, `SELECT checksum FROM platform_meta.g1_pending_migrations WHERE version=$1 AND scope='LOCAL_BRANCH_ONLY'`, version).Scan(&saved); e != nil {
					return e
				}
				if saved != hex.EncodeToString(hash[:]) {
					return fmt.Errorf("pending migration receipt differs")
				}
			}
			return nil
		})
	} else {
		err = owner.Migrate(ctx)
	}
	if err != nil {
		t.Fatal(err)
	}
	grantPaths := []string{"../../internal/platform/testdata/runtime-baseline-0001-0004.sql", "../../deploy/sql/runtime-grants-0005-0009.psql", "../../deploy/sql/runtime-grants-0010-games.psql"}
	if os.Getenv("MOMIAO_GAMES_TEST_ISOLATED_GAP") == "1" {
		grantPaths = append(grantPaths, "../../deploy/sql/runtime-grants-0015-slot.psql")
		if _, e := os.Stat("../../deploy/sql/runtime-grants-0016-blackjack.psql"); e == nil {
			grantPaths = append(grantPaths, "../../deploy/sql/runtime-grants-0016-blackjack.psql")
		}
	}
	for _, path := range grantPaths {
		raw, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		lines := []string{}
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "GRANT ") {
				lines = append(lines, line)
			}
		}
		grants := strings.ReplaceAll(strings.Join(lines, "\n"), `:"runtime_role"`, pgx.Identifier{c.RuntimeRole}.Sanitize())
		err = owner.WithTx(ctx, func(tx pgx.Tx) error { _, e := tx.Exec(ctx, grants, pgx.QueryExecModeSimpleProtocol); return e })
		if err != nil {
			t.Fatal(err)
		}
	}
	runtime, err := platform.Open(ctx, c.RuntimeURL)
	if err != nil {
		t.Fatal("isolated runtime connection failed")
	}
	t.Cleanup(runtime.Close)
	return owner, runtime
}

// Opt-in real portal and least-privilege PostgreSQL acceptance surface. The
// native login/identity is synthetic; game HTTP, config, wallet, ledger, result,
// history and verification all run the production code and actual database.
func TestGamesBrowserFixture(t *testing.T) {
	if os.Getenv("MOMIAO_GAMES_BROWSER_FIXTURE") != "1" {
		t.Skip("explicit local game browser fixture only")
	}
	owner, runtime := gameBrowserStores(t)
	ctx := context.Background()
	for _, user := range []int64{935000101, 935000102} {
		if err := owner.EnsureAccount(ctx, user); err != nil {
			t.Fatal(err)
		}
		profile, err := owner.ReadProfile(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		if profile.Status != "COMPLETE" {
			if _, err = owner.InitializeProfile(ctx, user, 0, "G1Player"+strconv.FormatInt(user%1000, 10), "system-default"); err != nil {
				t.Fatal(err)
			}
		}
		for _, asset := range []platform.Asset{platform.AvailableChips, platform.ReserveAPICredit} {
			_, err = owner.Apply(ctx, platform.Mutation{UserID: user, Asset: asset, DeltaUnits: 5000000000, BizType: "TEST_G1_BROWSER", BizID: fmt.Sprintf("bootstrap:%d:%s", user, asset), EntryType: "TEST_GRANT", IdempotencyKey: fmt.Sprintf("g1:browser:initial:%d:%s", user, asset)})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	key := [32]byte{}
	for i := range key {
		key[i] = byte(i + 41)
	}
	service, err := games.NewService(runtime, games.Keyring{Active: "g1-browser-v1", Keys: map[string][32]byte{"g1-browser-v1": key, "fixture-v1": func() [32]byte {
		k := [32]byte{}
		for i := range k {
			k[i] = byte(i + 17)
		}
		return k
	}()}})
	if err != nil {
		t.Fatal(err)
	}
	workerCtx, stopWorker := context.WithCancel(ctx)
	defer stopWorker()
	go service.RunWorker(workerCtx)
	nativeUser := func(id int64) map[string]any {
		return map[string]any{"id": id, "username": "g1-player-" + strconv.FormatInt(id, 10), "display_name": "G1 合成试玩", "role": 1, "status": 1, "quota": 0, "used_quota": 0, "request_count": 0}
	}
	sessionID := func(id int64) string { return "g1-synthetic-" + strconv.FormatInt(id, 10) }
	makeToken := func(id int64) string {
		payload, _ := json.Marshal(map[string]any{"iss": "new-api", "aud": []string{"new-api-dashboard"}, "token_use": "access", "sub": strconv.FormatInt(id, 10), "sid": sessionID(id), "uv": 1, "sv": 1, "exp": time.Now().Add(12 * time.Hour).Unix(), "iat": time.Now().Unix(), "nbf": time.Now().Add(-5 * time.Second).Unix(), "jti": "g1-browser-fixture"})
		raw := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(payload)
		mac := hmac.New(sha256.New, []byte("synthetic-public-browser-fixture-only"))
		mac.Write([]byte(raw))
		return raw + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	}
	tokens := map[int64]string{935000101: makeToken(935000101), 935000102: makeToken(935000102)}
	bundle := func(id int64) map[string]any {
		return map[string]any{"access_token": tokens[id], "access_expires_at": time.Now().Add(12 * time.Hour).Unix(), "user": nativeUser(id), "session": map[string]any{"sid": sessionID(id)}}
	}
	var mu sync.Mutex
	active := map[int64]bool{}
	native := walletTransport(func(r *http.Request) (*http.Response, error) {
		id, _ := strconv.ParseInt(r.Header.Get("New-Api-User"), 10, 64)
		mu.Lock()
		valid := active[id] && tokens[id] != "" && r.Header.Get("Authorization") == "Bearer "+tokens[id] && r.Header.Get("X-Auth-Session") == sessionID(id)
		mu.Unlock()
		status := 200
		var body []byte
		if !valid {
			status = 401
			body = []byte(`{"success":false,"code":"AUTH_UNAUTHORIZED"}`)
		} else if r.URL.Path == "/api/user/self" {
			body, _ = json.Marshal(map[string]any{"success": true, "data": nativeUser(id)})
		} else {
			status = 404
			body = []byte(`{"success":false}`)
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})
	web, err := filepath.Abs("../../web/dist")
	if err != nil {
		t.Fatal(err)
	}
	address := os.Getenv("MOMIAO_GAMES_BROWSER_ADDR")
	if address == "" {
		address = "127.0.0.1:0"
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		t.Fatal("browser fixture must bind loopback")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + listener.Addr().String()
	cfg := config{WebDir: web, PublicOrigin: origin, games: service, wallet: runtime, economy: runtime, profile: runtime, announcements: runtime, accessGate: runtime, accessDeclaration: &accessDeclaration{Version: 1, Environment: "DEVELOPMENT", Origin: origin, EvidenceRef: "g1-real-games-synthetic-native-only", MigrationApplicability: "NO_MIGRATION_APPLICABLE", Resources: map[string]string{"ACCOUNT": "AVAILABLE", "ASSETS": "AVAILABLE", "EXPERIENCE": "AVAILABLE", "COMMUNITY": "AVAILABLE"}}}
	portal := newPortalHandler(cfg, native)
	var server *http.Server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			var input struct{ Username, Password string }
			if r.Method != "POST" || json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&input) != nil || input.Password != "g1-local-only" {
				walletError(w, 401, "AUTH_UNAUTHORIZED")
				return
			}
			id := int64(0)
			if input.Username == "g1-player" {
				id = 935000101
			}
			if input.Username == "g1-other" {
				id = 935000102
			}
			if id == 0 {
				walletError(w, 401, "AUTH_UNAUTHORIZED")
				return
			}
			mu.Lock()
			active[id] = true
			mu.Unlock()
			http.SetCookie(w, &http.Cookie{Name: "g1_synthetic_fixture", Value: strconv.FormatInt(id, 10), Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			walletSuccess(w, bundle(id))
		case "/api/user/auth/refresh":
			cookie, e := r.Cookie("g1_synthetic_fixture")
			if e != nil {
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
			if c, e := r.Cookie("g1_synthetic_fixture"); e == nil {
				id, _ := strconv.ParseInt(c.Value, 10, 64)
				mu.Lock()
				active[id] = false
				mu.Unlock()
			}
			http.SetCookie(w, &http.Cookie{Name: "g1_synthetic_fixture", Path: "/", HttpOnly: true, MaxAge: -1, SameSite: http.SameSiteStrictMode})
			walletSuccess(w, map[string]any{})
		case "/__games-fixture/stop":
			if r.Method != "POST" || r.Header.Get("X-Games-Fixture") != "synthetic-local-only" {
				walletError(w, 403, "FIXTURE_ONLY")
				return
			}
			walletSuccess(w, map[string]any{"stopping": true})
			go server.Shutdown(context.Background())
		default:
			portal.ServeHTTP(w, r)
		}
	})
	server = &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	t.Logf("GAMES_SYNTHETIC_FIXTURE_READY %s; real Go + least-privilege PostgreSQL games; synthetic native only", origin)
	if err = server.Serve(listener); err != nil && err != http.ErrServerClosed {
		t.Fatal(err)
	}
}
