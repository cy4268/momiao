package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cy4268/momiao/internal/platform"
)

const pokerNavigationTable = "/poker/table/550e8400-e29b-41d4-a716-446655440000"

func TestPokerNavigationDocumentAndGateRoutes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<!doctype html><title>poker navigation fixture</title>"), 0600); err != nil {
		t.Fatal(err)
	}
	handler := newPortalHandler(config{WebDir: root}, nil)
	for _, tc := range []struct {
		path  string
		valid bool
	}{
		{"/poker", true}, {pokerNavigationTable, true},
		{"/poker/", false}, {"/Poker", false}, {"/poker/table", false},
		{pokerNavigationTable + "/", false}, {pokerNavigationTable + "/extra", false},
		{strings.ToUpper(pokerNavigationTable), false}, {"/poker/table/not-a-uuid", false},
		{"/poker/table/550e8400-e29b-41d4-a716-44665544000g", false},
		{"/poker/table/550e8400-e29b-41d4-a716-4466554400000", false},
		{"/poker//table/550e8400-e29b-41d4-a716-446655440000", false},
		{"/p%6fker", false}, {"/poker%2ftable/550e8400-e29b-41d4-a716-446655440000", false},
		{"/poker/table/%350e8400-e29b-41d4-a716-446655440000", false},
		{"/poker?", false}, {"/poker?request_id=forbidden", false},
		{pokerNavigationTable + "?seed=forbidden", false}, {"/poker#fragment", false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if got := gateRouteDomain(tc.path); (got == "EXPERIENCE") != tc.valid || !tc.valid && got != "" {
				t.Errorf("navigation gate domain=%q valid=%t", got, tc.valid)
			}
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				r := httptest.NewRequest(method, tc.path, nil)
				// Browser fragments never cross HTTP. Explicitly test the in-process
				// URL field too, rather than letting EscapedPath conceal it.
				if strings.Contains(tc.path, "#") {
					r.URL.Fragment = "fragment"
					r.URL.Path = "/poker"
				}
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				want := http.StatusNotFound
				if tc.valid {
					want = http.StatusOK
				}
				if w.Code != want {
					t.Errorf("document %s=%d want=%d", method, w.Code, want)
				}
				if tc.valid && (w.Header().Get("Cache-Control") != "no-store" || method == "GET" && !strings.Contains(w.Body.String(), "poker navigation fixture")) {
					t.Error("document did not serve the uncached existing SPA")
				}
			}
		})
	}
	for _, path := range []string{"https://foreign.invalid/poker", "//foreign.invalid/poker", pokerNavigationTable + "#fragment"} {
		if gateRouteDomain(path) != "" {
			t.Error("external/fragment intent accepted")
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/poker", nil))
	if w.Code != 405 || w.Header().Get("Allow") != "GET, HEAD" {
		t.Error("Poker document became a write endpoint")
	}
}

type pokerNavigationStore struct{ gateFixture }

func (s *pokerNavigationStore) ReadMigrationNotice(ctx context.Context, user int64, none bool) (platform.MigrationNotice, error) {
	if s.stage == "unverified" {
		s.calls = append(s.calls, "migration")
		return platform.MigrationNotice{State: "UNVERIFIED"}, nil
	}
	return s.gateFixture.ReadMigrationNotice(ctx, user, none)
}

func TestPokerNavigationApplicationGateOrder(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name, route, stage, resource string
		enabled, handler             bool
		account, code                int
		want, calls                  string
	}{
		{"both absent", "/poker", "", "AVAILABLE", false, false, 1, 200, "RESOURCE_UNAVAILABLE", "master,migration"},
		{"disabled with handler", "/poker", "", "AVAILABLE", false, true, 1, 200, "RESOURCE_UNAVAILABLE", "master,migration"},
		{"enabled without handler", "/poker", "", "AVAILABLE", true, false, 1, 200, "RESOURCE_UNAVAILABLE", "master,migration"},
		{"wired lobby", "/poker", "", "AVAILABLE", true, true, 1, 200, "READY", "master,migration"},
		{"wired table", pokerNavigationTable, "", "AVAILABLE", true, true, 1, 200, "READY", "master,migration"},
		{"account first", "/poker", "", "AVAILABLE", false, false, 2, 200, "ACCOUNT_RESTRICTED", ""},
		{"master first", "/poker", "master", "AVAILABLE", false, false, 1, 200, "MASTER_REQUIRED", "master"},
		{"migration first", "/poker", "migration", "AVAILABLE", false, false, 1, 200, "MIGRATION_REQUIRED", "master,migration"},
		{"migration unverified", "/poker", "unverified", "AVAILABLE", false, false, 1, 200, "MIGRATION_UNVERIFIED", "master,migration"},
		{"maintenance unchanged", pokerNavigationTable, "", "MAINTENANCE", true, true, 1, 200, "MAINTENANCE", "master,migration"},
		{"unwired maintenance", pokerNavigationTable, "", "MAINTENANCE", false, false, 1, 200, "RESOURCE_UNAVAILABLE", "master,migration"},
		{"resource absent", "/poker", "", "", true, true, 1, 200, "RESOURCE_UNVERIFIED", "master,migration"},
		{"native failure", "/poker", "native-error", "AVAILABLE", true, true, 1, 502, "", ""},
		{"missing auth", "/poker", "no-auth", "AVAILABLE", true, true, 1, 401, "", ""},
		{"other route unchanged", "/wallet", "", "AVAILABLE", false, false, 1, 200, "READY", "master,migration"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &pokerNavigationStore{gateFixture: gateFixture{stage: tc.stage}}
			transport := walletTransport(func(*http.Request) (*http.Response, error) {
				if tc.stage == "native-error" {
					return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("{}"))}, nil
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"success":true,"data":{"id":9007199254740993,"username":"synthetic-user","status":%d,"role":1}}`, tc.account)))}, nil
			})
			cfg := config{WebDir: root, PublicOrigin: "https://example.test", Poker: pokerConfig{Enabled: tc.enabled}, accessGate: store, accessDeclaration: &accessDeclaration{Resources: map[string]string{"EXPERIENCE": tc.resource, "ASSETS": "AVAILABLE"}}}
			if tc.handler {
				cfg.poker = http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("navigation invoked Poker runtime") })
			}
			w := httptest.NewRecorder()
			request := announcementReq("GET", "/platform/v1/access-gate?route="+url.QueryEscape(tc.route), "")
			if tc.stage == "no-auth" {
				request.Header.Del("Authorization")
			}
			newPortalHandler(cfg, transport).ServeHTTP(w, request)
			var body struct {
				Data accessGateView `json:"data"`
			}
			if json.Unmarshal(w.Body.Bytes(), &body) != nil || w.Code != tc.code || body.Data.Stage != tc.want {
				t.Errorf("actual gate=%d %s want=%d %s", w.Code, w.Body.String(), tc.code, tc.want)
			}
			if strings.Join(store.calls, ",") != tc.calls || store.ackUser != 0 {
				t.Errorf("gate order/effects changed: %v", store.calls)
			}
			if tc.code == 200 && (body.Data.UserID != 9007199254740993 || body.Data.Route != tc.route) {
				t.Error("gate locator/owner changed")
			}
		})
	}
	if !t.Failed() {
		t.Log("POKER_NAV_GATE SYNTHETIC Native; wired requires enabled AND handler; account/master/migration order unchanged; maintenance retained; no runtime or acknowledgement effects")
	}
}
