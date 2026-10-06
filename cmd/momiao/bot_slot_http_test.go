package main

import (
	"context"
	"github.com/cy4268/momiao/internal/botgames"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// The registered private slot route must reach the same admission boundary as
// dice, rather than falling through to NOT_FOUND or creating a public route.
func TestBotSlotHTTPPrepareAdmission(t *testing.T) {
	h := botGamesTestHandler(t, botGamesTestResolver(func(context.Context, string) (int64, error) {
		return 0, botgames.Fault{Code: "NOT_LINKED"}
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, botGamesRequest("POST", "/internal/v1/bot-games/slot/prepare", `{"request_id":"970000000000000001","total_wager":"10"}`))
	if w.Code != 404 || w.Body.String() != "{\"error\":\"NOT_LINKED\"}\n" {
		t.Fatalf("slot prepare must reach admission: status=%d body=%s; want 404 NOT_LINKED", w.Code, w.Body.String())
	}
}

func TestBotSlotHTTPPlayAndLookupAuthenticateQuotes(t *testing.T) {
	h := botGamesTestHandler(t, botGamesTestResolver(func(context.Context, string) (int64, error) { t.Fatal("bad signature reached resolver"); return 0, nil }))
	for _, op := range []string{"play", "lookup"} {
		t.Run(op, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, botGamesRequest("POST", "/internal/v1/bot-games/slot/"+op, `{"quote":"e30.YQ"}`))
			if w.Code != 401 || w.Body.String() != "{\"error\":\"UNAUTHORIZED\"}\n" {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}

const slotTestPrepare = `{"request_id":"970000000000000001","total_wager":"11"}`

func TestBotSlotHTTPStrictPrivateBoundary(t *testing.T) {
	h := botGamesTestHandler(t, botGamesTestResolver(func(context.Context, string) (int64, error) { return 0, botgames.Fault{Code: "NOT_LINKED"} }))
	for _, tc := range []struct {
		name, path, body, method string
		headers                  map[string][]string
		status                   int
		code                     string
	}{
		{name: "11", status: 404, code: "NOT_LINKED"},
		{name: "query", path: "/internal/v1/bot-games/slot/prepare?x=1", status: 400, code: "INVALID_REQUEST"},
		{name: "empty_query", path: "/internal/v1/bot-games/slot/prepare?", status: 400, code: "INVALID_REQUEST"},
		{name: "encoded", path: "/internal/v1/bot-games/slot/%70repare", status: 400, code: "INVALID_REQUEST"},
		{name: "arbitrary_game", path: "/internal/v1/bot-games/roulette/prepare", status: 404, code: "NOT_FOUND"},
		{name: "get", method: "GET", status: 400, code: "INVALID_REQUEST"},
		{name: "auth_missing", headers: map[string][]string{"Authorization": {}}, status: 401, code: "UNAUTHORIZED"},
		{name: "auth_duplicate", headers: map[string][]string{"Authorization": {"Bearer " + botGamesTestToken, "Bearer " + botGamesTestToken}}, status: 401, code: "UNAUTHORIZED"},
		{name: "auth_wrong", headers: map[string][]string{"Authorization": {"Bearer wrong"}}, status: 401, code: "UNAUTHORIZED"},
		{name: "subject_duplicate", headers: map[string][]string{"X-Discord-User": {botGamesTestSubject, botGamesTestSubject}}, status: 400, code: "INVALID_REQUEST"},
		{name: "subject_missing", headers: map[string][]string{"X-Discord-User": {}}, status: 400, code: "INVALID_REQUEST"},
		{name: "subject_invalid", headers: map[string][]string{"X-Discord-User": {"01"}}, status: 400, code: "INVALID_REQUEST"},
		{name: "content_type", headers: map[string][]string{"Content-Type": {"text/plain"}}, status: 400, code: "INVALID_REQUEST"},
		{name: "content_type_parameter", headers: map[string][]string{"Content-Type": {"application/json; charset=utf-8"}}, status: 400, code: "INVALID_REQUEST"},
		{name: "dice_input", body: botGamesTestPrepare, status: 400, code: "INVALID_REQUEST"},
		{name: "missing", body: `{"request_id":"970000000000000001"}`, status: 400, code: "INVALID_REQUEST"},
		{name: "duplicate", body: `{"request_id":"970000000000000001","total_wager":"11","total_wager":"11"}`, status: 400, code: "INVALID_REQUEST"},
		{name: "case_alias", body: `{"request_id":"970000000000000001","TOTAL_WAGER":"11"}`, status: 400, code: "INVALID_REQUEST"},
		{name: "unknown", body: `{"request_id":"970000000000000001","total_wager":"11","game":"slot"}`, status: 400, code: "INVALID_REQUEST"},
		{name: "numeric", body: `{"request_id":"970000000000000001","total_wager":11}`, status: 400, code: "INVALID_REQUEST"},
		{name: "bool", body: `{"request_id":"970000000000000001","total_wager":true}`, status: 400, code: "INVALID_REQUEST"},
		{name: "null", body: `{"request_id":"970000000000000001","total_wager":null}`, status: 400, code: "INVALID_REQUEST"},
		{name: "subminimum", body: `{"request_id":"970000000000000001","total_wager":"9"}`, status: 400, code: "INVALID_REQUEST"},
		{name: "trailing", body: slotTestPrepare + `{}`, status: 400, code: "INVALID_REQUEST"},
		{name: "4096", body: slotTestPrepare + strings.Repeat(" ", 4096-len(slotTestPrepare)), status: 404, code: "NOT_LINKED"},
		{name: "4097", body: slotTestPrepare + strings.Repeat(" ", 4097-len(slotTestPrepare)), status: 400, code: "INVALID_REQUEST"},
		{name: "4097_stream", body: slotTestPrepare + strings.Repeat(" ", 4097-len(slotTestPrepare)), status: 400, code: "INVALID_REQUEST"},
		{name: "play_unknown_field", path: "/internal/v1/bot-games/slot/play", body: `{"quote":"e30.YQ","total_wager":"11"}`, status: 400, code: "INVALID_REQUEST"},
		{name: "lookup_duplicate", path: "/internal/v1/bot-games/slot/lookup", body: `{"quote":"e30.YQ","quote":"e30.YQ"}`, status: 400, code: "INVALID_REQUEST"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.path == "" {
				tc.path = "/internal/v1/bot-games/slot/prepare"
			}
			if tc.method == "" {
				tc.method = "POST"
			}
			if tc.body == "" {
				tc.body = slotTestPrepare
			}
			r := botGamesRequest(tc.method, tc.path, tc.body)
			if tc.name == "4097_stream" {
				r.ContentLength = -1
			}
			for k, v := range tc.headers {
				r.Header.Del(k)
				for _, x := range v {
					r.Header.Add(k, x)
				}
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status || w.Body.String() != "{\"error\":\""+tc.code+"\"}\n" {
				t.Fatal(w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
				t.Fatal("private response headers missing")
			}
		})
	}
	for _, header := range []string{"Cookie", "Proxy-Authorization", "New-Api-User", "X-Auth-Session", "X-Native-User", "X-User-ID", "X-Forwarded-User", "X-Authenticated-User", "Idempotency-Key", "X-Fairness-Commitment"} {
		t.Run(header, func(t *testing.T) {
			r := botGamesRequest("POST", "/internal/v1/bot-games/slot/prepare", slotTestPrepare)
			r.Header.Set(header, "")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 400 {
				t.Fatal(header, w.Code)
			}
		})
	}
}

func TestBotSlotSharesExistingInflightBound(t *testing.T) {
	entered := make(chan struct{}, 16)
	release := make(chan struct{})
	var wg sync.WaitGroup
	h := botGamesTestHandler(t, botGamesTestResolver(func(ctx context.Context, _ string) (int64, error) {
		d, ok := ctx.Deadline()
		if !ok || time.Until(d) > 8*time.Second {
			t.Error("request deadline missing")
		}
		entered <- struct{}{}
		<-release
		return 0, botgames.Fault{Code: "NOT_LINKED"}
	}))
	defer func() { close(release); wg.Wait() }()
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			path, body := "/internal/v1/bot-games/slot/prepare", slotTestPrepare
			if i%2 == 0 {
				path, body = "/internal/v1/bot-games/dice/prepare", botGamesTestPrepare
			}
			h.ServeHTTP(httptest.NewRecorder(), botGamesRequest("POST", path, body))
		}()
	}
	for range 16 {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("16 requests did not enter")
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, botGamesRequest("POST", "/internal/v1/bot-games/slot/prepare", slotTestPrepare))
	if w.Code != 503 {
		t.Fatal("slot escaped shared in-flight bound", w.Code)
	}
}
