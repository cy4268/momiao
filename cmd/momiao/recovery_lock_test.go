package main

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func recoveryTestConfig(t *testing.T, env map[string]string) config {
	t.Helper()
	cfg, err := loadConfig(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestRecoveryLockRejectsMisconfiguration(t *testing.T) {
	for _, env := range []map[string]string{
		{"DR_RECOVERY_LOCK": ""}, {"DR_RECOVERY_LOCK": "true"}, {"DR_RECOVERY_LOCK": "on"}, {"DR_RECOVERY_LOCK": " ON"},
		{"DR_RECOVERY_REQUIRED": "ON"}, {"DR_RECOVERY_REQUIRED": "ON", "DR_RECOVERY_LOCK": "OFF"},
		{"DR_RECOVERY_REQUIRED": "typo"}, {"DR_RECOVERY_AUTH_PROBES": "ON"},
		{"DR_RECOVERY_LOCK": "ON", "DR_RECOVERY_AUTH_PROBES": "ON"},
		{"DR_RECOVERY_LOCK": "ON", "DR_RECOVERY_AUTH_PROBES": "true"},
	} {
		if _, err := loadConfig(func(k string) (string, bool) { v, ok := env[k]; return v, ok }); err == nil {
			t.Errorf("accepted invalid recovery configuration: %v", env)
		}
	}
}

func TestRecoveryLockDeniesEveryBusinessSurface(t *testing.T) {
	cfg := recoveryTestConfig(t, map[string]string{"DR_RECOVERY_LOCK": "ON", "DR_RECOVERY_REQUIRED": "ON"})
	handler := newServer(cfg).Handler
	for _, path := range []string{
		"/", "/api/user/self", "/api/token/", "/v1/chat/completions", "/pg/chat/completions",
		"/platform/v1/wallet", "/platform/v1/admission", "/platform/v1/admission/config",
		"/platform/v1/quota-transfers", "/platform/v1/key-purposes", "/platform/v1/ops/announcements",
		"/platform/v1/models", "/api/v1/games", "/api/v1/game-rounds", "/api/v1/rankings",
		"/api/v1/maintenance/notices", "/api/v1/ops/commands", "/api/v1/history",
		"/api/v1/poker/tables", "/api/v1/poker/connect-tickets", "/ws/poker",
		"/internal/v1/poker/revoke", "/internal/v1/poker/ops", "/internal/v1/quota/refill",
		"/api/v1/auth/password/login", "/api/v1/auth/discord/registration/start", "/api/v1/account",
		"/api/v1/session/bootstrap", "/api/v1/auth/logout", "/future/unknown-writer",
	} {
		for _, method := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "CONNECT"} {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(method, path, nil))
			if w.Code != 503 || !strings.Contains(w.Body.String(), "DR_RECOVERY_LOCKED") {
				t.Errorf("%s %s: %d %s", method, path, w.Code, w.Body.String())
			}
		}
	}
}

func TestRecoveryLockStatusNeverClaimsBusinessReadiness(t *testing.T) {
	cfg := recoveryTestConfig(t, map[string]string{"DR_RECOVERY_LOCK": "ON"})
	h := newServer(cfg).Handler
	for _, path := range []string{"/healthz", "/readyz", "/internal/v1/recovery/status"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		want := 200
		if path == "/readyz" {
			want = 503
		}
		if w.Code != want {
			t.Fatalf("%s = %d", path, w.Code)
		}
		var status struct {
			Lock                   string `json:"lock"`
			BusinessLocked         bool   `json:"business_locked"`
			BusinessWorkersStarted bool   `json:"business_workers_started"`
			Ready                  bool   `json:"ready"`
			ContractVersion        int    `json:"contract_version"`
		}
		if json.Unmarshal(w.Body.Bytes(), &status) != nil || status.Lock != "ON" || !status.BusinessLocked || status.BusinessWorkersStarted || status.Ready || status.ContractVersion != 1 {
			t.Errorf("%s: %s", path, w.Body.String())
		}
	}
}

func TestRecoveryLockDefaultAndExplicitOffPreserveRoutes(t *testing.T) {
	for _, env := range []map[string]string{{}, {"DR_RECOVERY_LOCK": "OFF", "DR_RECOVERY_REQUIRED": "OFF", "DR_RECOVERY_AUTH_PROBES": "OFF"}} {
		cfg := recoveryTestConfig(t, env)
		w := httptest.NewRecorder()
		newServer(cfg).Handler.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
		if w.Code != http.StatusOK || w.Body.String() != "{\"status\":\"ok\"}\n" {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}

// The early return must dominate both role dispatch and every legacy constructor,
// including constructors that spawn workers before an HTTP listener exists.
func TestRecoveryLockDispatchDominatesAllBusinessStartup(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "run" {
			continue
		}
		guard, ok := fn.Body.List[0].(*ast.IfStmt)
		if !ok {
			t.Fatal("run must first test recovery lock")
		}
		found := false
		ast.Inspect(guard, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "runRecoveryLockedProcess" {
					found = true
				}
			}
			return true
		})
		if !found {
			t.Fatal("recovery lock does not dominate games, announcements, maintenance, admission, catalog, history, attribution, keypurpose, quota, ranking, Ops remote/health, refill and Poker startup")
		}
		return
	}
	t.Fatal("run not found")
}

func TestRecoveryLockAuthProbeAllowlistAndAmbiguousRequests(t *testing.T) {
	cfg := recoveryTestConfig(t, map[string]string{"DR_RECOVERY_LOCK": "ON"})
	cfg.RecoveryAuthProbes, cfg.Session.Enabled = true, true
	called := 0
	cfg.sessionAuth = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called++; w.WriteHeader(204) })
	h := newPortalHandler(cfg, nil)
	for _, path := range []string{"/api/v1/session/bootstrap", "/api/v1/account"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 204 {
			t.Fatalf("allowed authentication probe %s = %d", path, w.Code)
		}
	}
	if called != 2 {
		t.Fatal("authentication probe not dispatched")
	}
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/api/v1/account", ""}, {"POST", "/api/v1/auth/password/login", ""},
		{"GET", "/api/v1/account/password/set", ""}, {"GET", "/api/v1/auth/discord/callback", ""},
		{"GET", "/api/v1/account?x=1", ""}, {"GET", "/api/v1/account?", ""},
		{"GET", "/api/v1/%61ccount", ""}, {"GET", "/api/v1/session/bootstrap", "{}"},
		{"POST", "/internal/v1/recovery/status", ""}, {"GET", "/healthz?unlock=1", ""},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if w.Code != 503 || !strings.Contains(w.Body.String(), "DR_RECOVERY_LOCKED") {
			t.Errorf("ambiguity escaped lock: %+v = %d", tc, w.Code)
		}
	}
	for _, header := range []string{"upgrade", "transfer-encoding"} {
		r := httptest.NewRequest("GET", "/api/v1/account", nil)
		if header == "upgrade" {
			r.Header.Set("Upgrade", "websocket")
		} else {
			r.TransferEncoding = []string{"chunked"}
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 503 {
			t.Errorf("%s escaped lock", header)
		}
	}
	if called != 2 {
		t.Fatal("a disallowed route reached authentication")
	}
	cfg.ProcessRole = "poker"
	w := httptest.NewRecorder()
	newRecoveryLockedHandler(cfg).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/account", nil))
	if w.Code != 503 || called != 2 {
		t.Fatal("Poker must never expose Platform auth probes")
	}
}

func TestRecoveryLockHealthyDependenciesStillDoNotOpenBusiness(t *testing.T) {
	cfg := recoveryTestConfig(t, map[string]string{"DR_RECOVERY_LOCK": "ON"})
	cfg.readiness = map[string]readinessCheck{"fixture": func(context.Context) error { return nil }}
	w := httptest.NewRecorder()
	newServer(cfg).Handler.ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
	var s recoveryStatus
	if json.Unmarshal(w.Body.Bytes(), &s) != nil || w.Code != 503 || s.Ready || !s.DependencyReady || !s.BusinessLocked {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestRecoveryLockRuntimeBranchesServeAndStopForBothRoles(t *testing.T) {
	// This is a local HTTP/process-composition test, not restored-database
	// acceptance. Both roles exercise run's real early-return path, listener and
	// graceful stop without creating a database, worker, peer or Docker runtime.
	for _, role := range []string{"platform", "poker"} {
		t.Run(role, func(t *testing.T) {
			reservation, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			addr := reservation.Addr().String()
			reservation.Close()
			cfg := recoveryTestConfig(t, map[string]string{"DR_RECOVERY_LOCK": "ON"})
			cfg.ProcessRole, cfg.ListenAddr, cfg.ShutdownTimeout = role, addr, time.Second
			// These legacy startup key paths are poison pills: any accidental
			// dispatch to runPokerProcess would fail before opening the listener.
			cfg.PokerPeerKeyringFile = "missing-recovery-test-peer-keys"
			cfg.PokerServiceKeyringFile = "missing-recovery-test-service-keys"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- run(ctx, cfg, log.New(io.Discard, "", 0)) }()
			client := &http.Client{Timeout: time.Second}
			deadline := time.Now().Add(3 * time.Second)
			var response *http.Response
			for time.Now().Before(deadline) {
				response, err = client.Get("http://" + addr + "/healthz")
				if err == nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err != nil {
				t.Fatal(err)
			}
			var s recoveryStatus
			err = json.NewDecoder(response.Body).Decode(&s)
			response.Body.Close()
			if err != nil || response.StatusCode != 200 || !s.BusinessLocked || s.BusinessWorkersStarted || s.Ready {
				t.Fatalf("unexpected locked status: %+v (%v)", s, err)
			}
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("locked process failed to stop")
			}
		})
	}
}
