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

func TestM1DirectRoutesAndNativePrefix(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>M1 candidate</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	var forwarded string
	handler := newPortalHandler(config{WebDir: root}, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		forwarded = r.URL.Path
		return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"native":true}`))}, nil
	}))
	for _, path := range []string{"/", "/me", "/rewards"} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(method+path, func(t *testing.T) {
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, httptest.NewRequest(method, path, nil))
				if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "text/html; charset=utf-8" || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("direct route rejected: status=%d headers=%v", w.Code, w.Header())
				}
				if method == http.MethodGet && !strings.Contains(w.Body.String(), "M1 candidate") {
					t.Fatal("route did not serve the SPA index")
				}
			})
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/keys", nil))
	if forwarded != "/api/keys" || w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "M1 candidate") {
		t.Fatal("native /api prefix was captured by SPA routing")
	}
}

func TestM1PublicImageFormats(t *testing.T) {
	root := t.TempDir()
	assetDir := filepath.Join(root, "assets", "home")
	if err := os.MkdirAll(assetDir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{"avif", "webp"} {
		name := "bg-royal-observatory-v001." + ext
		if err := os.WriteFile(filepath.Join(assetDir, name), []byte("synthetic image bytes"), 0600); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		serveWebFile(root, w, httptest.NewRequest(http.MethodGet, "/assets/home/"+name, nil))
		if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/"+ext || w.Body.String() != "synthetic image bytes" {
			t.Fatalf("%s asset not served correctly: status=%d headers=%v", ext, w.Code, w.Header())
		}
	}
}
