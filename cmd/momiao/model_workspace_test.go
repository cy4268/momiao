package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelWorkspaceRoutes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>workspace</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	h := newPortalHandler(config{WebDir: root}, nil)
	for _, path := range []string{"/models", "/admin/channels"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "workspace") {
			t.Errorf("workspace route %s: %d", path, w.Code)
		}
	}
	for _, path := range []string{"/admin", "/admin/settings", "/pg/unknown", "/playground", "/playground?model=example"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Errorf("unimplemented route %s: %d", path, w.Code)
		}
	}
}

func TestRetiredPlaygroundNeverReachesNative(t *testing.T) {
	calls := 0
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("unexpected upstream call"))}, nil
	})
	h := newPortalHandler(config{WebDir: t.TempDir()}, transport)
	for _, tc := range []struct {
		method string
		status int
	}{
		{http.MethodGet, http.StatusNotFound},
		{http.MethodPost, http.StatusMethodNotAllowed},
	} {
		r := httptest.NewRequest(tc.method, "/pg/chat/completions", strings.NewReader(`{"model":"fixture-model","stream":true}`))
		r.Header.Set("Authorization", "Bearer fixture-session")
		r.Header.Set("X-Auth-Session", "fixture-sid")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("retired playground %s: got %d, want %d", tc.method, w.Code, tc.status)
		}
	}
	if calls != 0 {
		t.Fatalf("retired playground reached native upstream %d times", calls)
	}
}
