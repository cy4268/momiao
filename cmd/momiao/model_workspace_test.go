package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestModelWorkspaceRoutes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>workspace</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	h := newPortalHandler(config{WebDir: root}, nil)
	for _, path := range []string{"/models", "/playground", "/admin/channels"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "workspace") {
			t.Errorf("workspace route %s: %d", path, w.Code)
		}
	}
	for _, path := range []string{"/admin", "/admin/settings", "/pg/unknown"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Errorf("unimplemented route %s: %d", path, w.Code)
		}
	}
}

func TestPlaygroundUsesNativeSessionAndRelayDeadline(t *testing.T) {
	const body = `{"model":"fixture-model","messages":[{"role":"user","content":"hello"}],"stream":true}`
	const stream = "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n"
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/pg/chat/completions" || r.Header.Get("Authorization") != "Bearer fixture-session" || r.Header.Get("X-Auth-Session") != "fixture-sid" {
			t.Errorf("native playground request changed")
		}
		got, _ := io.ReadAll(r.Body)
		if string(got) != body {
			t.Errorf("playground payload changed")
		}
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) < 4*time.Minute || time.Until(deadline) > 5*time.Minute {
			t.Errorf("playground must use the bounded model-call deadline")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}, nil
	})
	r := httptest.NewRequest("POST", "/pg/chat/completions", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer fixture-session")
	r.Header.Set("X-Auth-Session", "fixture-sid")
	w := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	newPortalHandler(config{WebDir: t.TempDir()}, transport).ServeHTTP(w, r)
	if w.Code != 200 || w.Body.String() != stream || !w.Flushed || time.Until(w.writeDeadline) < 4*time.Minute {
		t.Fatalf("playground stream not transparently forwarded: status=%d flushed=%v", w.Code, w.Flushed)
	}
}
