package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDomainRouteBoundary(t *testing.T) {
	for _, path := range []string{"/platform/v1/wallet", "/api/v1/games/dice/bootstrap", "/api/v1/poker/tables", "/platform/v1/models/personal-price", "/platform/v1/announcements/current-post-login-popup", "/platform/v1/ops/announcements"} {
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
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("unverified platform request reached Native")
		return nil, errors.New("unexpected Native call")
	})
	cfg := config{WebDir: t.TempDir(), Session: sessionConfig{Enabled: true}, poker: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("unverified platform request reached Poker") })}
	portal := newPortalHandler(cfg, transport)
	for _, p := range []string{"/api/user/self", "/api/user/self/groups", "/api/user/models?group=default", "/api/token/?p=1&page_size=10", "/api/log/self?p=1&page_size=10", "/platform/v1/wallet", "/platform/v1/master-profile", "/platform/v1/access-gate", "/platform/v1/admission/ensure", "/platform/v1/native-quota", "/platform/v1/rewards/daily", "/api/v1/games/dice/bootstrap", "/api/v1/poker/tables", "/platform/v1/models/personal-price", "/platform/v1/announcements", "/platform/v1/ops/announcements"} {
		w := httptest.NewRecorder()
		portal.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		if w.Code != 503 {
			t.Errorf("%s missing bridge status=%d", p, w.Code)
		}
	}
}

func TestNativeSelfRouteOptInIsExact(t *testing.T) {
	for _, tc := range []struct {
		enabled bool
		path    string
	}{{false, "/api/user/self"}, {false, "/api/user/self/groups"}, {false, "/api/user/models"}, {false, "/api/log/self"}} {
		calls := 0
		transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Path != tc.path {
				t.Errorf("legacy proxy path=%s", r.URL.Path)
			}
			return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: http.NoBody}, nil
		})
		portal := newPortalHandler(config{WebDir: t.TempDir(), Session: sessionConfig{Enabled: tc.enabled}}, transport)
		w := httptest.NewRecorder()
		portal.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if calls != 1 || w.Code != http.StatusNoContent {
			t.Errorf("enabled=%v path=%s calls=%d status=%d", tc.enabled, tc.path, calls, w.Code)
		}
	}
}

func TestNativeTokenListRouteOptInIsExact(t *testing.T) {
	for _, tc := range []struct {
		enabled      bool
		method, path string
	}{
		{false, http.MethodGet, "/api/token/"},
		{false, http.MethodPost, "/api/token/"},
		{false, http.MethodPut, "/api/token/?status_only=true"},
		{false, http.MethodPost, "/api/token/1/key"},
		{false, http.MethodDelete, "/api/token/1"},
	} {
		calls := 0
		transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Method != tc.method {
				t.Errorf("legacy method=%s", r.Method)
			}
			return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: http.NoBody}, nil
		})
		portal := newPortalHandler(config{WebDir: t.TempDir(), Session: sessionConfig{Enabled: tc.enabled}}, transport)
		w := httptest.NewRecorder()
		portal.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if calls != 1 || w.Code != http.StatusNoContent {
			t.Errorf("enabled=%v %s %s calls=%d status=%d", tc.enabled, tc.method, tc.path, calls, w.Code)
		}
	}
}

func TestNativeTokenWritePortalRequiresBridge(t *testing.T) {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("unverified token mutation reached Native")
		return nil, errors.New("unexpected Native call")
	})
	portal := newPortalHandler(config{WebDir: t.TempDir(), Session: sessionConfig{Enabled: true}}, transport)
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/token/"},
		{http.MethodPut, "/api/token/?status_only=true"},
		{http.MethodPost, "/api/token/9/key"},
		{http.MethodDelete, "/api/token/9"},
		{http.MethodPost, "/api/token/09/key"},
		{http.MethodPut, "/api/token/?status_only=false"},
		{http.MethodPost, "/api/token/9/key?x=1"},
	} {
		w := httptest.NewRecorder()
		portal.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s missing bridge status=%d", tc.method, tc.path, w.Code)
		}
	}
}

func TestOpaqueModeBlocksLegacyBrowserAuthenticationRoutes(t *testing.T) {
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
			transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: http.NoBody}, nil
			})
			response := httptest.NewRecorder()
			newPortalHandler(config{WebDir: t.TempDir(), Session: sessionConfig{Enabled: true}}, transport).ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
			if calls != 0 || response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"code":"AUTH_ROUTE_NOT_FOUND"`) {
				t.Fatalf("legacy auth route escaped opaque boundary: calls=%d status=%d body=%s", calls, response.Code, response.Body.String())
			}
		})
	}
}

func TestNativeModeKeepsLegacyPasswordLoginProxy(t *testing.T) {
	calls := 0
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/api/user/login" {
			t.Fatalf("legacy proxy path=%s", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: http.NoBody}, nil
	})
	response := httptest.NewRecorder()
	newPortalHandler(config{WebDir: t.TempDir()}, transport).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/user/login", nil))
	if calls != 1 || response.Code != http.StatusNoContent {
		t.Fatalf("native mode changed: calls=%d status=%d", calls, response.Code)
	}
}
