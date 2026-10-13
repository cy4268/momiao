package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDomainRouteBoundary(t *testing.T) {
	for _, path := range []string{"/api/v1/ops/native-runtime", "/platform/v1/wallet", "/api/v1/games/dice/bootstrap", "/api/v1/poker/tables", "/platform/v1/models/personal-price", "/platform/v1/announcements/current-post-login-popup", "/platform/v1/ops/announcements"} {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		called := false
		domainHandler(config{Session: sessionConfig{Enabled: true}}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).ServeHTTP(w, r)
		if called || w.Code != 503 {
			t.Errorf("missing platform auth service reached domain: %s status=%d", path, w.Code)
		}
	}
	for _, path := range []string{"/api/v1/games", "/platform/v1/models", "/platform/v1/models/example", "/platform/v1/admission/config", "/ws/poker"} {
		called := false
		domainHandler(config{Session: sessionConfig{Enabled: true}}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
		if !called {
			t.Errorf("public or ticket route intercepted: %s", path)
		}
	}
	called := false
	domainHandler(config{}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/platform/v1/wallet", nil))
	if !called {
		t.Error("disabled opt-in changed legacy route")
	}
}

func TestDomainPortalWiring(t *testing.T) {

	cfg := config{WebDir: t.TempDir(), Session: sessionConfig{Enabled: true}, poker: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("unverified platform request reached Poker") })}
	portal := newPortalHandler(cfg)
	for _, p := range []string{"/api/user/self", "/api/user/self/groups", "/api/user/models?group=default", "/api/token/?p=1&page_size=10", "/api/log/self?p=1&page_size=10", "/platform/v1/wallet", "/platform/v1/master-profile", "/platform/v1/access-gate", "/platform/v1/admission/ensure", "/platform/v1/rewards/daily", "/api/v1/games/dice/bootstrap", "/api/v1/poker/tables", "/platform/v1/models/personal-price", "/platform/v1/announcements", "/platform/v1/ops/announcements"} {
		w := httptest.NewRecorder()
		portal.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		if w.Code != 503 {
			t.Errorf("%s missing bridge status=%d", p, w.Code)
		}
	}
}

func TestRemovedBrowserAuthenticationRoutes(t *testing.T) {
	routes := []struct{ method, path string }{
		{http.MethodPost, "/api/user/login"},
		{http.MethodPost, "/api/user/login/2fa"},
		{http.MethodPost, "/api/user/auth/refresh"},
		{http.MethodPost, "/api/user/auth/logout"},
		{http.MethodPost, "/api/momiao/auth/discord/login/start"},
		{http.MethodGet, "/api/momiao/auth/discord/callback?code=x&state=y"},
		{http.MethodPost, "/api/momiao/auth/2fa"},
	}
	for _, tc := range routes {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			calls := 0

			response := httptest.NewRecorder()
			newPortalHandler(config{WebDir: t.TempDir(), Session: sessionConfig{Enabled: true}}).ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
			if calls != 0 || response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"code":"NOT_FOUND"`) {
				t.Fatalf("removed auth route reached Native: calls=%d status=%d body=%s", calls, response.Code, response.Body.String())
			}
		})
	}
}
