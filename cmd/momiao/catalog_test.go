package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/platform"
)

type catalogHTTPStore struct {
	*platform.Store
	publicCalls, authorityCalls, writes int
	denied, withdrawn                   bool
	filter                              platform.CatalogFilter
}

func (s *catalogHTTPStore) CatalogAuthority(_ context.Context, user int64) (platform.AnnouncementPrincipal, error) {
	s.authorityCalls++
	if s.denied {
		return platform.AnnouncementPrincipal{}, platform.ErrCatalogForbidden
	}
	return platform.AnnouncementPrincipal{UserID: user, Role: "OPERATOR", Epoch: 2, Permissions: []string{"models.read", "models.write", "models.publish"}}, nil
}
func (s *catalogHTTPStore) PublicCatalog(_ context.Context, f platform.CatalogFilter, _ platform.CatalogPolicy) (platform.CatalogPage, error) {
	s.publicCalls++
	s.filter = f
	return platform.CatalogPage{Items: []platform.CatalogModel{}}, nil
}
func (s *catalogHTTPStore) PublicCatalogModel(_ context.Context, id string, _ platform.CatalogPolicy) (platform.CatalogModel, error) {
	s.publicCalls++
	if s.withdrawn {
		return platform.CatalogModel{}, platform.ErrCatalogNotFound
	}
	return platform.CatalogModel{ModelID: id, PublicationState: "PUBLISHED", CanUse: true}, nil
}
func (s *catalogHTTPStore) OpsCatalog(ctx context.Context, user int64, _ platform.CatalogOpsFilter, _ platform.CatalogPolicy) (platform.CatalogOpsPage, error) {
	p, e := s.CatalogAuthority(ctx, user)
	return platform.CatalogOpsPage{Principal: p, Items: []platform.CatalogModel{}}, e
}
func (s *catalogHTTPStore) PrepareCatalog(_ context.Context, _ int64, c platform.CatalogCommand, _ platform.CatalogSource, _ platform.CatalogPolicy) (platform.CatalogPreview, error) {
	s.writes++
	return platform.CatalogPreview{ID: c.OperationID}, nil
}
func (s *catalogHTTPStore) ExecuteCatalog(_ context.Context, _ int64, c platform.CatalogCommand, id string, confirmed bool, _ platform.CatalogSource, _ platform.CatalogPolicy) (platform.CatalogResult, error) {
	s.writes++
	if !confirmed || id == "" {
		return platform.CatalogResult{}, platform.ErrCatalogConfirmation
	}
	return platform.CatalogResult{OperationID: c.OperationID}, nil
}
func catalogHTTPConfig(s catalogStore) config {
	return config{catalog: s, PublicOrigin: "https://wallet.example", CatalogStaleAfter: 10 * time.Minute, CatalogDisableAfter: 30 * time.Minute, APIBaseURL: "https://api.example/v1"}
}

func TestCatalogHTTPPublicAndStrictQueries(t *testing.T) {
	s := &catalogHTTPStore{}
	native := catalogRoundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("public read reached native identity")
		return nil, nil
	})
	h := newCatalogHandler(catalogHTTPConfig(s), native)
	for _, path := range []string{"/platform/v1/models", "/platform/v1/models/detail?model_id=" + url.QueryEscape("组织/x%2F'"), "/platform/v1/models/access-config"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer ignored-public-credential")
		h.ServeHTTP(w, r)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(path, w.Code, w.Body)
		}
	}
	for _, query := range []string{"?user_id=1", "?limit=-1", "?limit=101", "?limit=0", "?offset=1000001", "?q=a&q=b", "?recommended=maybe", "?min_context=1e3", "?unknown_context=false", "?x=%GG"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/platform/v1/models"+query, nil))
		if w.Code != 400 {
			t.Fatal(query, w.Code)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/platform/v1/models?q=100%25&recommended=true&min_context=100&price_dimension=input&min_price=0.000000001&sort=price", nil))
	if w.Code != 200 || s.filter.Search != "100%" || !s.filter.RecommendedOnly || s.filter.MinPrice == nil || *s.filter.MinPrice != "0.000000001" {
		t.Fatal(w.Code, s.filter)
	}
	for _, path := range []string{"/platform/v1/models/detail", "/platform/v1/models/detail?model_id=x&group=default", "/platform/v1/models/access-config?base_url=https://evil.example"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 400 {
			t.Fatal(path, w.Code)
		}
	}
}
func TestCatalogHTTPProtectedWrites(t *testing.T) {
	s := &catalogHTTPStore{denied: true}
	h := newCatalogHandler(catalogHTTPConfig(s), profileTransport())
	for _, path := range []string{"/platform/v1/ops/models", "/platform/v1/models/personal-price?model_id=x"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 401 {
			t.Fatal(path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, announcementReq("GET", "/platform/v1/ops/models", ""))
	if w.Code != 403 {
		t.Fatal("native admin gained scope", w.Code)
	}
	s.denied = false
	for _, body := range []string{`null`, `[]`, `{"command":{},"command":{}}`, `{"command":{"action":"SAVE","action":"SYNC"}}`, `{"command":{},"other":1}`, strings.Repeat(" ", 66000)} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, announcementReq("POST", "/platform/v1/ops/models/prepare", body))
		if w.Code != 400 {
			t.Fatal(body[:min(len(body), 80)], w.Code)
		}
	}
	for _, kind := range []string{"origin", "content", "query"} {
		r := announcementReq("POST", "/platform/v1/ops/models/prepare", `{"command":{}}`)
		if kind == "origin" {
			r.Header.Set("Origin", "https://evil.example")
		}
		if kind == "content" {
			r.Header.Set("Content-Type", "text/plain")
		}
		if kind == "query" {
			r.URL.RawQuery = "x=1"
		}
		w = httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code < 400 || s.writes != 0 {
			t.Fatal(kind, w.Code, s.writes)
		}
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, announcementReq("POST", "/platform/v1/ops/models/prepare", `{"command":{"operation_id":"fixture"}}`))
	if w.Code != 200 || s.writes != 1 {
		t.Fatal(w.Code, w.Body)
	}
}
func TestCatalogHTTPPersonalPriceBoundaryAndWithdrawal(t *testing.T) {
	id := "组织/a%2F'"
	s := &catalogHTTPStore{}
	nativeCalls := 0
	withdraw := false
	native := catalogRoundTrip(func(r *http.Request) (*http.Response, error) {
		nativeCalls++
		if r.URL.Path == "/api/user/self" {
			return profileTransport().RoundTrip(r)
		}
		if r.Method != "GET" || r.URL.Scheme != "http" || r.URL.Host != "unix" || r.URL.Path != "/api/momiao/catalog/prices" || r.URL.Query().Get("model_id") != id || len(r.URL.Query()) != 1 {
			t.Fatal("personal request broadened", r.URL)
		}
		for k := range r.Header {
			if k != "Authorization" && k != "New-Api-User" && k != "X-Auth-Session" && k != "Accept" {
				t.Fatal("forwarded private header", k)
			}
		}
		if r.Header.Get("X-Auth-Session") != "fixture-session" || r.Header.Get("New-Api-User") != "9007199254740993" {
			t.Fatal("unbound subject")
		}
		if withdraw {
			s.withdrawn = true
		}
		body, _ := json.Marshal(platform.NativePersonalCatalog{Success: true, Schema: platform.NativeCatalogSchema, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), ModelID: id, Basis: "current_user_group_reference_not_token_selection", BillingAuthority: "native_settlement", Quotes: []platform.NativePersonalQuote{{Candidate: 1, Reason: "model_not_enabled_in_candidate"}}})
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})
	h := newCatalogHandler(catalogHTTPConfig(s), native)
	for _, want := range []int{200, 404} {
		w := httptest.NewRecorder()
		r := announcementReq("GET", "/platform/v1/models/personal-price?model_id="+url.QueryEscape(id), "")
		r.Header.Set("Cookie", "do-not-forward")
		r.Header.Set("X-Private", "do-not-forward")
		h.ServeHTTP(w, r)
		if w.Code != want || strings.Contains(w.Body.String(), "group_multiplier") || strings.Contains(w.Body.String(), "do-not-forward") {
			t.Fatal(w.Code, w.Body)
		}
		withdraw = true
	}
	if nativeCalls != 4 || s.publicCalls != 4 {
		t.Fatal("missing native validation or withdrawal fence", nativeCalls, s.publicCalls)
	}
}
func TestCatalogBrowserOpaqueRoutesAndExactAccessProxy(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("catalog SPA"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	cfg := catalogHTTPConfig(&catalogHTTPStore{})
	cfg.WebDir = dir
	h := newPortalHandler(cfg, catalogRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 201, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("native"))}, nil
	}))
	for _, path := range []string{"/api/access?model_id=x", "/models/" + url.PathEscape("组织/x%2F'"), "/models/~Lg", "/models/~Li4", "/models/%7ELg", "/ops/models"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "catalog SPA") {
			t.Fatal(path, w.Code, w.Body)
		}
	}
	if calls != 0 {
		t.Fatal("access page was proxied")
	}
	for _, path := range []string{"/api/access/", "/api/user/self"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 201 {
			t.Fatal(path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/access", nil))
	if w.Code != 405 || calls != 2 {
		t.Fatal("access method boundary", w.Code, calls)
	}
}

func TestCatalogWorkerRecoversAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := make(chan int, 4)
	done := make(chan struct{})
	n := 0
	go func() {
		defer close(done)
		runCatalogWorker(ctx, 5*time.Millisecond, func(context.Context) (platform.CatalogSyncResult, error) {
			n++
			calls <- n
			return platform.CatalogSyncResult{}, errors.New("synthetic failure")
		})
	}()
	for i := 1; i <= 2; i++ {
		select {
		case got := <-calls:
			if got != i {
				t.Fatal(got)
			}
		case <-time.After(time.Second):
			t.Fatal("worker failed to recover")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker ignored cancellation")
	}
}
