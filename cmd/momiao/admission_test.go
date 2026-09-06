package main

import (
	"context"
	"github.com/cy4268/momiao/internal/platform"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type admissionTestStore struct {
	profileTestStore
	statusReads int
}

func (s *admissionTestStore) EnsureProvisionalProfile(ctx context.Context, u int64) (platform.Profile, error) {
	s.inits++
	return s.result(u)
}
func (s *admissionTestStore) ReadAdmission(ctx context.Context, u int64) (platform.AdmissionStatus, error) {
	s.statusReads++
	return platform.AdmissionStatus{UserID: u, Source: "UNVERIFIED", GrantStatus: "PENDING_SOURCE"}, nil
}
func TestAdmissionHTTPAuthorityAndExplicitEnsure(t *testing.T) {
	s := &admissionTestStore{}
	h := newAdmissionHandler("https://wallet.example", s, profileTransport())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, walletReq("GET", "/platform/v1/admission", ""))
	if w.Code != 200 || s.statusReads != 1 || s.inits != 0 {
		t.Fatal("GET wrote state")
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, walletReq("POST", "/platform/v1/admission/ensure", "{}"))
	if w.Code != 200 || s.inits != 1 || s.user != 9007199254740993 {
		t.Fatal("native identity not authoritative")
	}
	for _, body := range []string{`{"registered":true}`, `{"source":"NEW_DISCORD_REGISTRATION"}`, `{"user_id":"1"}`, `{"grant":true}`, `null`, `{} {}`, strings.Repeat(" ", 2049) + `{}`} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, walletReq("POST", "/platform/v1/admission/ensure", body))
		if w.Code != 400 {
			t.Fatal("client source accepted", w.Code)
		}
	}
	for _, change := range []func(*http.Request){func(r *http.Request) { r.Header.Del("Authorization") }, func(r *http.Request) { r.Header.Set("New-Api-User", "1") }, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }, func(r *http.Request) { r.Header.Add("Origin", "https://wallet.example") }} {
		r := walletReq("POST", "/platform/v1/admission/ensure", "{}")
		change(r)
		w = httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code < 400 {
			t.Fatal("forged authority accepted")
		}
	}
	if s.inits != 1 {
		t.Fatal("invalid request wrote profile")
	}
}
func TestAdmissionPrivatePathNeverProxied(t *testing.T) {
	cfg := config{WebDir: t.TempDir()}
	called := false
	h := newPortalHandler(cfg, admissionTransport(func(*http.Request) (*http.Response, error) { called = true; return readerResponse(200, `{}`), nil }))
	for _, path := range []string{"/internal/momiao/registrations", "/internal", "/internal/else", "/api/../internal/momiao/registrations", "/api/%2e%2e/internal/momiao/registrations", "/%69nternal/momiao/registrations"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer "+strings.Repeat("k", 32))
		h.ServeHTTP(w, r)
		if w.Code != 404 || called {
			t.Fatal("private source reached proxy")
		}
	}
}
