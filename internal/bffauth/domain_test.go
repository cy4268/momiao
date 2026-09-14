package bffauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDomainProjectionBoundary(t *testing.T) {
	r := httptest.NewRequest("POST", "https://fixture.invalid/api/v1/games/dice/rounds?key=x", strings.NewReader(`{}`))
	r.Header.Set("Cookie", CookieName+"=fixture")
	r.Header.Set("X-CSRF-Token", "platform-csrf")
	r.Header.Set("X-Game-CSRF-Token", strings.Repeat("a", 64))
	r.Header.Set("Origin", "https://fixture.invalid")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", "receipt")
	r.Header.Set("X-Fairness-Commitment", "commitment")
	r.Header.Set("X-Untrusted", "discard")
	r.Host = "untrusted.invalid"
	r.Trailer = http.Header{"X-Untrusted": {"discard"}, "Authorization": {"browser"}}
	ctx, cancel := context.WithCancel(r.Context())
	r = r.WithContext(ctx)
	c := nativeCredential{UserID: "1", SID: "server-sid", AccessToken: "server-access", RefreshToken: "never-project"}
	out := projectDomainRequest(r, c, true)
	if out.Host != "" {
		t.Error("browser Host projected outside header allowlist")
	}
	if out.Trailer != nil {
		t.Error("browser trailers projected outside header allowlist")
	}
	if out.Header.Get("Authorization") != "Bearer server-access" || out.Header.Get("New-Api-User") != "1" || out.Header.Get("X-Auth-Session") != "server-sid" {
		t.Error("server credential projection missing")
	}
	if out.Header.Get("X-CSRF-Token") != strings.Repeat("a", 64) || out.Header.Get("Idempotency-Key") != "receipt" || out.Header.Get("X-Fairness-Commitment") != "commitment" {
		t.Error("game CSRF or business receipt contract lost")
	}
	for _, key := range []string{"Cookie", "X-Game-CSRF-Token", "X-Untrusted"} {
		if len(out.Header.Values(key)) != 0 {
			t.Errorf("unexpected projected header %s", key)
		}
	}
	if out.Body != r.Body || out.URL.String() != r.URL.String() || out.Method != r.Method || r.Header.Get("Authorization") != "" || r.Header.Get("X-CSRF-Token") != "platform-csrf" {
		t.Error("body/route changed or original request mutated")
	}
	if projectDomainRequest(r, c, false).Header.Get("X-CSRF-Token") != "" {
		t.Error("platform CSRF leaked into domain request")
	}
	cancel()
	if out.Context().Err() != context.Canceled {
		t.Error("client cancellation detached")
	}
}

func TestDomainInputBoundary(t *testing.T) {
	var service *Service
	if out, err := service.DomainRequest(nil, false); out != nil || err == nil {
		t.Fatal("missing request/service accepted")
	}
	if out, err := service.DomainRequest(httptest.NewRequest("GET", "https://fixture.invalid/platform/v1/wallet", nil), false); out != nil || err == nil {
		t.Fatal("missing service returned domain credentials")
	}
	for _, tc := range []struct {
		name, key, value string
		game             bool
	}{
		{"native access", "Authorization", "Bearer browser", false},
		{"native user", "New-Api-User", "1", false},
		{"native SID", "X-Auth-Session", "browser", false},
		{"native cookie", "Cookie", "new_api_refresh=browser", false},
		{"upgrade", "Upgrade", "websocket", false},
		{"connection nomination", "Connection", "X-CSRF-Token", false},
		{"unexpected game token", "X-Game-CSRF-Token", strings.Repeat("a", 64), false},
		{"short game token", "X-Game-CSRF-Token", "short", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "https://fixture.invalid/platform/v1/wallet", nil)
			r.Header.Set(tc.key, tc.value)
			if domainInput(r, tc.game) == nil {
				t.Error("untrusted input accepted")
			}
		})
	}
	r := httptest.NewRequest(http.MethodPost, "https://fixture.invalid/api/v1/games/dice/rounds", nil)
	r.Header.Set("X-Game-CSRF-Token", strings.Repeat("a", 64))
	r.Header.Set("Connection", "keep-alive")
	if domainInput(r, true) != nil {
		t.Error("valid game token input rejected")
	}
	r.Header.Add("X-Game-CSRF-Token", strings.Repeat("b", 64))
	if domainInput(r, true) == nil {
		t.Error("duplicate game token accepted")
	}
}

func TestDomainSessionCSRFFence(t *testing.T) {
	want := strings.Repeat("c", 43)
	for _, tc := range []struct {
		name   string
		values []string
		ok     bool
	}{
		{name: "exact", values: []string{want}, ok: true},
		{name: "missing"},
		{name: "mismatch", values: []string{strings.Repeat("d", 43)}},
		{name: "duplicate", values: []string{want, want}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "https://fixture.invalid/platform/v1/wallet", nil)
			for _, value := range tc.values {
				r.Header.Add("X-CSRF-Token", value)
			}
			err := domainSessionCSRF(r, want)
			if tc.ok {
				if err != nil {
					t.Fatalf("exact session CSRF rejected: %v", err)
				}
				return
			}
			fault, ok := err.(Fault)
			if !ok || fault.Code != "SESSION_CSRF_FAILED" || fault.Status != http.StatusForbidden || fault.ClearCookie {
				t.Fatalf("session CSRF fault=%+v, want non-clearing SESSION_CSRF_FAILED", err)
			}
		})
	}
}
