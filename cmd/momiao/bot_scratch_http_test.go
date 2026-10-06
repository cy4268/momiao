package main

import (
	"context"
	"errors"
	"github.com/cy4268/momiao/internal/botgames"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBotScratchHTTPPrepareAdmission(t *testing.T) {
	h := botGamesTestHandler(t, botGamesTestResolver(func(context.Context, string) (int64, error) {
		return 0, botgames.Fault{Code: "NOT_LINKED"}
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, botGamesRequest("POST", "/internal/v1/bot-games/scratch/prepare", `{"request_id":"970000000000000001","wager":"11"}`))
	if w.Code != 404 || w.Body.String() != "{\"error\":\"NOT_LINKED\"}\n" {
		t.Fatalf("scratch prepare must reach admission: status=%d body=%s; want 404 NOT_LINKED", w.Code, w.Body.String())
	}
}

const scratchTestPrepare = `{"request_id":"970000000000000001","wager":"11"}`

func TestBotScratchHTTPStrictBoundary(t *testing.T) {
	h := botGamesTestHandler(t, botGamesTestResolver(func(context.Context, string) (int64, error) { return 0, botgames.Fault{Code: "NOT_LINKED"} }))
	for _, tc := range []struct {
		name, path, body, method string
		headers                  map[string][]string
		status                   int
		code                     string
	}{
		{name: "valid11", status: 404, code: "NOT_LINKED"},
		{name: "valid10", body: `{"request_id":"970000000000000001","wager":"10"}`, status: 404, code: "NOT_LINKED"},
		{name: "play_signature", path: "/internal/v1/bot-games/scratch/play", body: `{"quote":"e30.YQ"}`, status: 401, code: "UNAUTHORIZED"},
		{name: "lookup_signature", path: "/internal/v1/bot-games/scratch/lookup", body: `{"quote":"e30.YQ"}`, status: 401, code: "UNAUTHORIZED"},
		{name: "query", path: "/internal/v1/bot-games/scratch/prepare?x=1", status: 400, code: "INVALID_REQUEST"},
		{name: "empty_query", path: "/internal/v1/bot-games/scratch/prepare?", status: 400, code: "INVALID_REQUEST"},
		{name: "encoding", path: "/internal/v1/bot-games/scratch/%70repare", status: 400, code: "INVALID_REQUEST"},
		{name: "get", method: "GET", status: 400, code: "INVALID_REQUEST"},
		{name: "unknown", path: "/internal/v1/bot-games/scratch/auto", status: 404, code: "NOT_FOUND"},
		{name: "auth_missing", headers: map[string][]string{"Authorization": {}}, status: 401, code: "UNAUTHORIZED"},
		{name: "auth_duplicate", headers: map[string][]string{"Authorization": {"Bearer " + botGamesTestToken, "Bearer " + botGamesTestToken}}, status: 401, code: "UNAUTHORIZED"},
		{name: "auth_wrong", headers: map[string][]string{"Authorization": {"Bearer wrong"}}, status: 401, code: "UNAUTHORIZED"},
		{name: "subject_missing", headers: map[string][]string{"X-Discord-User": {}}, status: 400, code: "INVALID_REQUEST"},
		{name: "subject_duplicate", headers: map[string][]string{"X-Discord-User": {"1", "1"}}, status: 400, code: "INVALID_REQUEST"},
		{name: "subject_bad", headers: map[string][]string{"X-Discord-User": {"01"}}, status: 400, code: "INVALID_REQUEST"},
		{name: "content_parameter", headers: map[string][]string{"Content-Type": {"application/json; charset=utf-8"}}, status: 400, code: "INVALID_REQUEST"},
		{name: "content_duplicate", headers: map[string][]string{"Content-Type": {"application/json", "application/json"}}, status: 400, code: "INVALID_REQUEST"},
		{name: "subminimum", body: `{"request_id":"970000000000000001","wager":"9"}`, status: 400, code: "INVALID_REQUEST"},
		{name: "missing", body: `{"request_id":"970000000000000001"}`, status: 400, code: "INVALID_REQUEST"},
		{name: "numeric", body: `{"request_id":"970000000000000001","wager":11}`, status: 400, code: "INVALID_REQUEST"},
		{name: "boolean", body: `{"request_id":"970000000000000001","wager":true}`, status: 400, code: "INVALID_REQUEST"},
		{name: "null", body: `{"request_id":"970000000000000001","wager":null}`, status: 400, code: "INVALID_REQUEST"},
		{name: "duplicate", body: `{"request_id":"970000000000000001","wager":"11","wager":"11"}`, status: 400, code: "INVALID_REQUEST"},
		{name: "alias", body: `{"request_id":"970000000000000001","Wager":"11"}`, status: 400, code: "INVALID_REQUEST"},
		{name: "unknown_field", body: `{"request_id":"970000000000000001","wager":"11","round_id":"x"}`, status: 400, code: "INVALID_REQUEST"},
		{name: "trailing", body: scratchTestPrepare + `{}`, status: 400, code: "INVALID_REQUEST"},
		{name: "4096", body: scratchTestPrepare + strings.Repeat(" ", 4096-len(scratchTestPrepare)), status: 404, code: "NOT_LINKED"},
		{name: "4097", body: scratchTestPrepare + strings.Repeat(" ", 4097-len(scratchTestPrepare)), status: 400, code: "INVALID_REQUEST"},
		{name: "4097_stream", body: scratchTestPrepare + strings.Repeat(" ", 4097-len(scratchTestPrepare)), status: 400, code: "INVALID_REQUEST"},
		{name: "play_extra", path: "/internal/v1/bot-games/scratch/play", body: `{"quote":"e30.YQ","wager":"11"}`, status: 400, code: "INVALID_REQUEST"},
		{name: "lookup_duplicate", path: "/internal/v1/bot-games/scratch/lookup", body: `{"quote":"e30.YQ","quote":"e30.YQ"}`, status: 400, code: "INVALID_REQUEST"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.path == "" {
				tc.path = "/internal/v1/bot-games/scratch/prepare"
			}
			if tc.method == "" {
				tc.method = "POST"
			}
			if tc.body == "" {
				tc.body = scratchTestPrepare
			}
			r := botGamesRequest(tc.method, tc.path, tc.body)
			if tc.name == "4097_stream" {
				r.ContentLength = -1
			}
			for k, v := range tc.headers {
				r.Header.Del(k)
				for _, value := range v {
					r.Header.Add(k, value)
				}
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status || w.Body.String() != `{"error":"`+tc.code+`"}`+"\n" {
				t.Fatal(w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
				t.Fatal("private headers absent")
			}
		})
	}
	for _, header := range []string{"Cookie", "Proxy-Authorization", "New-Api-User", "X-Auth-Session", "X-Native-User", "X-User-ID", "X-Forwarded-User", "X-Authenticated-User", "Idempotency-Key", "X-Fairness-Commitment"} {
		t.Run(header, func(t *testing.T) {
			r := botGamesRequest("POST", "/internal/v1/bot-games/scratch/prepare", scratchTestPrepare)
			r.Header.Set(header, "")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 400 {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	for _, code := range []string{"SCRATCH_PREVIOUS_REVEAL_INCOMPLETE", "QUOTE_EXPIRED", "IDEMPOTENCY_CONFLICT"} {
		t.Run(code, func(t *testing.T) {
			h := botGamesTestHandler(t, botGamesTestResolver(func(context.Context, string) (int64, error) { return 0, botgames.Fault{Code: code} }))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, botGamesRequest("POST", "/internal/v1/bot-games/scratch/prepare", scratchTestPrepare))
			if w.Code != 409 || w.Body.String() != `{"error":"`+code+`"}`+"\n" {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}

func TestBotScratchAbsentFromPublicPortal(t *testing.T) {
	for _, web := range []string{"", t.TempDir()} {
		for _, enabled := range []bool{false, true} {
			cfg := config{WebDir: web}
			cfg.Session.Enabled = enabled
			h := newPortalHandler(cfg, roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Error("scratch reached Native proxy")
				return nil, errors.New("unexpected Native")
			}))
			for _, path := range []string{"prepare", "play", "lookup", "%70repare", "prepare?x=1"} {
				for _, method := range []string{"GET", "POST"} {
					w := httptest.NewRecorder()
					h.ServeHTTP(w, botGamesRequest(method, "/internal/v1/bot-games/scratch/"+path, scratchTestPrepare))
					if w.Code != 404 {
						t.Fatal(method, path, w.Code)
					}
				}
			}
		}
	}
}
