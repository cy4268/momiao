package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestHistoryHTTPBoundary(t *testing.T) {
	var h *historyHTTP
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/other", 404},
		{"POST", historyPath, 405},
		{"GET", historyPath + "?user_id=123", 503},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Header.Set("Authorization", "Bearer fixture-not-an-identity")
		h.ServeHTTP(w, r)
		if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s %s: status %d", tc.method, tc.path, w.Code)
		}
		if tc.status == 405 && w.Header().Get("Allow") != http.MethodGet {
			t.Fatal("missing read-only method declaration")
		}
	}
}

func TestHistoryHTTPQuery(t *testing.T) {
	u, _ := url.Parse(historyPath + "?record_type=POKER_SESSION&mode=POKER&game_slug=poker&time_from=2026-09-01T00%3A00%3A00Z&time_to=2026-09-02T00%3A00%3A00Z&result=WIN&status=SETTLED&id=11111111-1111-4111-8111-111111111111&limit=25&cursor=opaque")
	kind, _, proof, q, err := parseHistoryRequest(u)
	if err != nil || kind != "list" || proof || historyLimit(q, "limit") != 25 || q.Get("cursor") != "opaque" {
		t.Fatalf("valid filters lost: %v", err)
	}
	for _, raw := range []string{"user_id=1", "mode+game_slug=POKER", "limit=1&limit=2", "limit=101", "limit=01", "cursor=%zz", "status=SETTLED;user_id=1"} {
		u := &url.URL{Path: historyPath, RawQuery: raw}
		if _, _, _, _, err := parseHistoryRequest(u); err == nil {
			t.Fatalf("accepted ambiguous or untrusted query %q", raw)
		}
	}
}

func TestHistoryHTTPDetailRoutes(t *testing.T) {
	id := "11111111-1111-4111-8111-111111111111"
	for _, part := range []string{"rounds/" + id, "rounds/" + id + "/verify", "hands/" + id + "/verify", "sessions/" + id + "?funding_limit=20&hand_cursor=opaque", "hands/" + id + "?limit=20"} {
		u, _ := url.Parse(historyPath + "/" + part)
		if _, got, _, _, err := parseHistoryRequest(u); err != nil || got != id {
			t.Fatalf("valid detail route %q: %v", part, err)
		}
	}
	for _, part := range []string{"rounds/not-an-id", "rounds/" + id + "?subject=2", "hands/" + id + "/verify?cursor=opaque", "sessions/" + id + "/", "records/" + id} {
		u, _ := url.Parse(historyPath + "/" + part)
		if _, _, _, _, err := parseHistoryRequest(u); err == nil {
			t.Fatalf("accepted unknown detail route %q", part)
		}
	}
}
