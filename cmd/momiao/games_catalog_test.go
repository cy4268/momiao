package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/poker"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type catalogForbiddenAuth struct{ calls int }

func (t *catalogForbiddenAuth) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls++
	return nil, errors.New("public catalog attempted private authentication")
}

func TestGameCatalogQueryBoundaryAndOutage(t *testing.T) {
	// A closed lazy pool gives a deterministic real storage failure without network I/O.
	store, err := platform.OpenLazy(context.Background(), "postgres://catalog-readonly@127.0.0.1:1/catalog_readonly?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	service, err := games.NewService(store, games.Keyring{Active: "catalog-test", Keys: map[string][32]byte{"catalog-test": {1}}})
	if err != nil {
		t.Fatal(err)
	}
	transport := &catalogForbiddenAuth{}
	handler := newGameHandler("http://127.0.0.1", service, transport, nil)
	for _, tc := range []struct {
		raw    string
		status int
	}{
		{"", 503}, {"q=Dice", 503}, {"q=%E9%AA%B0&availability=PLAY&sort=NAME", 503},
		{"availability=ALL&sort=RECOMMENDED", 503}, {"q=%F0%9F%8E%B2", 503},
		{"q=%", 400}, {"q=%G1", 400}, {"q=%FF", 400}, {"q=%C0%AF", 400},
		{"q=%ED%A0%80", 400}, {"q=%F4%90%80%80", 400}, {"q=%00", 400},
		{"q=x&q=y", 400}, {"%71=x&q=y", 400}, {"q=x;sort=NAME", 400},
		{"sort=NAME&sort=NAME", 400}, {"sort=HOT", 400}, {"availability=RESUME", 400},
		{"availability=", 400}, {"sort=", 400}, {"user_id=2", 400},
		{"q=" + strings.Repeat("x", 129), 400}, {"q=" + string([]byte{255}), 400},
		{"?q=Dice", 400},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/games", nil)
			r.URL.RawQuery = tc.raw
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("query %q: status %d, want %d; body %s", tc.raw, w.Code, tc.status, w.Body.String())
			}
			if tc.status == 503 && !strings.Contains(w.Body.String(), "GAME_TEMPORARILY_UNAVAILABLE") {
				t.Fatal("storage failure became an empty or successful catalog", w.Body.String())
			}
		})
	}
	for _, path := range []string{"/api/v1/games/dice/bootstrap?q=Dice", "/api/v1/game-rounds?availability=PLAY"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != 400 {
			t.Fatalf("catalog query leaked to %s: %d", path, w.Code)
		}
	}
	for _, method := range []string{"POST", "PUT", "DELETE"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(method, "/api/v1/games?q=Dice", nil))
		if w.Code != 405 || w.Header().Get("Allow") != "GET" {
			t.Fatal("catalog accepted mutation", method, w.Code)
		}
	}
	if transport.calls != 0 {
		t.Fatal("public or rejected query reached identity transport")
	}
}

func TestPokerCatalogHTTPRealRead(t *testing.T) {
	owner, runtime := gameBrowserStores(t)
	f := newPokerAppFixture(t, 910001)
	ctx := context.Background()
	var fresh bool
	if err := owner.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT publication_state='COMING_SOON' AND configured_runtime_state='UNAVAILABLE' AND implementation_key='poker.texas-holdem.v1' AND NOT EXISTS(SELECT 1 FROM games.game_rounds) FROM games.game_registry WHERE game_slug='texas-holdem'").Scan(&fresh)
	}); err != nil || !fresh {
		t.Fatal("HTTP catalog acceptance requires the owned fresh seed", err)
	}
	update := func(publication, configured string) {
		t.Helper()
		if err := owner.WithTx(ctx, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, "UPDATE games.game_registry SET publication_state=$1,configured_runtime_state=$2 WHERE game_slug='texas-holdem'", publication, configured)
			return e
		}); err != nil {
			t.Fatal(err)
		}
	}
	update("PUBLISHED", "AVAILABLE")
	t.Cleanup(func() { update("COMING_SOON", "UNAVAILABLE") })
	service, err := games.NewService(runtime, games.Keyring{Active: "catalog", Keys: map[string][32]byte{"catalog": {1}}})
	if err != nil {
		t.Fatal(err)
	}
	c := f.domain.Config()
	c.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	pool, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	denied := &lobbyDeniedLease{}
	reader, err := poker.New(poker.Options{Pool: pool, Leases: denied, Keyring: poker.Keyring{Current: "catalog", Keys: map[string][]byte{"catalog": make([]byte, 32)}}})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var maintenance string
	err = f.owner.QueryRow(ctx, `WITH op AS (INSERT INTO ops.admin_operations(operation_id,actor_kind,newapi_user_id,action,request_hash,details,result) VALUES(gen_random_uuid(),'OFFLINE',910001,'SYNTHETIC_CATALOG',repeat('a',64),'{}','{}') RETURNING operation_id), w AS (INSERT INTO ops.maintenance_windows(maintenance_id,state,reason,impact_snapshot,impact_hash,environment,created_by,operation_id) SELECT gen_random_uuid(),'DRAFT','catalog fixture','{}',decode(repeat('a',64),'hex'),'STAGING',910001,operation_id FROM op RETURNING maintenance_id) INSERT INTO ops.maintenance_window_scopes SELECT maintenance_id,'POKER_NEW_TABLES_NEW_HANDS' FROM w RETURNING maintenance_id::text`).Scan(&maintenance)
	if err != nil {
		t.Fatal(err)
	}
	facts := func() int {
		t.Helper()
		var n int
		if err := f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM poker.tables)+(SELECT count(*) FROM poker.sessions)+(SELECT count(*) FROM poker.request_receipts)+(SELECT count(*) FROM economy.wallet_ledger)`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := facts()
	for _, tc := range []struct {
		name, want string
		calls      int
	}{
		{"ready", "PLAY", 1}, {"disabled", "TEMPORARILY_UNAVAILABLE", 0}, {"no handler", "TEMPORARILY_UNAVAILABLE", 0}, {"no reader", "TEMPORARILY_UNAVAILABLE", 0},
		{"maintenance", "MAINTENANCE", 1}, {"incomplete", "TEMPORARILY_UNAVAILABLE", 1}, {"closed", "TEMPORARILY_UNAVAILABLE", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.owner.Exec(ctx, "UPDATE ops.maintenance_windows SET state=$1 WHERE maintenance_id=$2", map[bool]string{true: "ACTIVE", false: "DRAFT"}[tc.name == "maintenance"], maintenance); err != nil {
				t.Fatal(err)
			}
			if _, err := f.owner.Exec(ctx, "UPDATE poker.ruleset_versions SET active=$1", tc.name != "incomplete"); err != nil {
				t.Fatal(err)
			}
			if tc.name == "closed" {
				reader.Close()
			}
			calls := 0
			auth := &catalogForbiddenAuth{}
			cfg := config{WebDir: t.TempDir(), games: service, Poker: pokerConfig{Enabled: true}, poker: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("catalog called private Poker handler") }), pokerCatalogRuntime: func(ctx context.Context) (string, error) { calls++; return reader.CatalogRuntime(ctx) }}
			if tc.name == "disabled" {
				cfg.Poker.Enabled = false
			}
			if tc.name == "no handler" {
				cfg.poker = nil
			}
			if tc.name == "no reader" {
				cfg.pokerCatalogRuntime = nil
			}
			server := httptest.NewServer(newPortalHandler(cfg, auth))
			defer server.Close()
			request := func(method, query string, status int) []byte {
				t.Helper()
				req, err := http.NewRequest(method, server.URL+"/api/v1/games"+query, nil)
				if err != nil {
					t.Fatal(err)
				}
				response, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				body, err := io.ReadAll(response.Body)
				if err != nil || response.StatusCode != status {
					t.Fatalf("public response status=%d want=%d error=%v", response.StatusCode, status, err)
				}
				return body
			}
			body := request("GET", "?q=texas-holdem", 200)
			var envelope struct {
				Data struct {
					Items []map[string]any `json:"items"`
				} `json:"data"`
			}
			if err := json.Unmarshal(body, &envelope); err != nil {
				t.Fatal(err)
			}
			if len(envelope.Data.Items) != 1 {
				t.Fatal("public Poker row missing", string(body))
			}
			row := envelope.Data.Items[0]
			if row["effective_runtime"] != tc.want || row["slug"] != "texas-holdem" || row["implementation_key"] != "poker.texas-holdem.v1" || row["title"] != "德州扑克" || len(row) != 4 || calls != tc.calls {
				t.Fatalf("public projection=%v calls=%d want=%s/%d", row, calls, tc.want, tc.calls)
			}
			calls = 0
			request("GET", "?user_id=910001", 400)
			request("POST", "", 405)
			if calls != 0 || auth.calls != 0 || denied.calls.Load() != 0 || facts() != before {
				t.Fatal("public/rejected catalog crossed private boundary")
			}
		})
	}
}

// Opt-in public-only acceptance surface: no login, account grants, worker, or game mutations.
func TestCatalogDiscoveryBrowserFixture(t *testing.T) {
	address := os.Getenv("MOMIAO_CATALOG_BROWSER_ADDRESS")
	if address == "" {
		t.Skip("explicit isolated catalog browser fixture only")
	}
	if address != "127.0.0.1:58107" {
		t.Fatal("catalog fixture requires its dedicated loopback port")
	}
	_, runtime := gameBrowserStores(t)
	service, err := games.NewService(runtime, games.Keyring{Active: "catalog-browser", Keys: map[string][32]byte{"catalog-browser": {1}}})
	if err != nil {
		t.Fatal(err)
	}
	web, err := filepath.Abs("../../web/dist")
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	portal := newPortalHandler(config{WebDir: web, PublicOrigin: "http://" + address, games: service}, &catalogForbiddenAuth{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/user/auth/refresh" {
			walletError(w, 401, "AUTH_UNAUTHORIZED")
			return
		}
		if (r.Method != "GET" && r.Method != "HEAD") || strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/v1/games" {
			walletError(w, 404, "NOT_FOUND")
			return
		}
		portal.ServeHTTP(w, r)
	})}
	t.Logf("CATALOG_READ_ONLY_PREVIEW=http://%s/games", address)
	if err = server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		t.Fatal(err)
	}
}
