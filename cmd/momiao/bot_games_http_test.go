package main

import (
	"context"
	"errors"
	"github.com/cy4268/momiao/internal/botgames"
	"github.com/cy4268/momiao/internal/games"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const botGamesTestToken = "1111111111111111111111111111111111111111111111111111111111111111"
const botGamesTestSubject = "970000000000000002"
const botGamesTestPrepare = `{"request_id":"970000000000000001","wager":"10","choice":"BIG"}`

type botGamesTestResolver func(context.Context, string) (int64, error)

func (f botGamesTestResolver) Resolve(ctx context.Context, s string) (int64, error) { return f(ctx, s) }

type botGamesUnusedEngine struct{}

func (botGamesUnusedEngine) Bootstrap(context.Context, int64, string) (games.Bootstrap, error) {
	return games.Bootstrap{}, errors.New("unexpected engine")
}
func (botGamesUnusedEngine) FindByKey(context.Context, int64, string, string) (*games.GameRound, error) {
	return nil, errors.New("unexpected engine")
}
func (botGamesUnusedEngine) Create(context.Context, int64, string, string, string, games.CreateInput) (games.GameRound, error) {
	return games.GameRound{}, errors.New("unexpected engine")
}
func botGamesTestHandler(t *testing.T, r botgames.Resolver) http.Handler {
	t.Helper()
	s, e := botgames.NewService(botGamesUnusedEngine{}, r, [32]byte{1}, time.Now)
	if e != nil {
		t.Fatal(e)
	}
	h, e := newBotGamesHandler(s, botGamesTestToken)
	if e != nil {
		t.Fatal(e)
	}
	return h
}
func botGamesRequest(method, path, body string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+botGamesTestToken)
	r.Header.Set("X-Discord-User", botGamesTestSubject)
	r.Header.Set("Content-Type", "application/json")
	return r
}
func TestBotGamesHTTP(t *testing.T) {
	h := botGamesTestHandler(t, botGamesTestResolver(func(context.Context, string) (int64, error) { return 0, botgames.Fault{Code: "NOT_LINKED"} }))
	type testCase struct {
		name, method, path, body string
		headers                  map[string][]string
		wantStatus               int
		wantCode                 string
	}
	cases := []testCase{
		{name: "prepare", wantStatus: 404, wantCode: "NOT_LINKED"},
		{name: "play", path: "/internal/v1/bot-games/dice/play", body: `{"quote":"e30.YQ"}`, wantStatus: 401, wantCode: "UNAUTHORIZED"},
		{name: "lookup", path: "/internal/v1/bot-games/dice/lookup", body: `{"quote":"e30.YQ"}`, wantStatus: 401, wantCode: "UNAUTHORIZED"},
		{name: "get", method: "GET", wantStatus: 400, wantCode: "INVALID_REQUEST"},
		{name: "unknown", path: "/internal/v1/bot-games/dice/no", wantStatus: 404, wantCode: "NOT_FOUND"},
		{name: "query", path: "/internal/v1/bot-games/dice/prepare?x=1", wantStatus: 400, wantCode: "INVALID_REQUEST"},
		{name: "empty_query", path: "/internal/v1/bot-games/dice/prepare?", wantStatus: 400, wantCode: "INVALID_REQUEST"},
		{name: "encoded", path: "/internal/v1/bot-games/dice/%70repare", wantStatus: 400, wantCode: "INVALID_REQUEST"},
		{name: "slash", path: "/internal/v1/bot-games/dice//prepare", wantStatus: 404, wantCode: "NOT_FOUND"},
		{name: "auth_missing", headers: map[string][]string{"Authorization": {}}, wantStatus: 401, wantCode: "UNAUTHORIZED"},
		{name: "auth_duplicate", headers: map[string][]string{"Authorization": {"Bearer " + botGamesTestToken, "Bearer " + botGamesTestToken}}, wantStatus: 401, wantCode: "UNAUTHORIZED"},
		{name: "auth_wrong", headers: map[string][]string{"Authorization": {"Bearer wrong"}}, wantStatus: 401, wantCode: "UNAUTHORIZED"},
		{name: "subject_missing", headers: map[string][]string{"X-Discord-User": {}}, wantStatus: 400, wantCode: "INVALID_REQUEST"},
		{name: "subject_duplicate", headers: map[string][]string{"X-Discord-User": {botGamesTestSubject, botGamesTestSubject}}, wantStatus: 400, wantCode: "INVALID_REQUEST"},
		{name: "subject_zero", headers: map[string][]string{"X-Discord-User": {"0"}}, wantStatus: 400, wantCode: "INVALID_REQUEST"},
		{name: "subject_leading_zero", headers: map[string][]string{"X-Discord-User": {"01"}}, wantStatus: 400, wantCode: "INVALID_REQUEST"},
		{name: "subject_max", headers: map[string][]string{"X-Discord-User": {"18446744073709551615"}}, wantStatus: 404, wantCode: "NOT_LINKED"},
		{name: "subject_overflow", headers: map[string][]string{"X-Discord-User": {"18446744073709551616"}}, wantStatus: 400, wantCode: "INVALID_REQUEST"},
		{name: "json_type", headers: map[string][]string{"Content-Type": {"text/plain"}}, wantStatus: 400, wantCode: "INVALID_REQUEST"},
		{name: "duplicate_type", headers: map[string][]string{"Content-Type": {"application/json", "application/json"}}, wantStatus: 400, wantCode: "INVALID_REQUEST"},
		{name: "type_parameter", headers: map[string][]string{"Content-Type": {"application/json; charset=utf-8"}}, wantStatus: 400, wantCode: "INVALID_REQUEST"},
		{name: "duplicate_field", body: `{"request_id":"970000000000000001","wager":"10","choice":"BIG","choice":"BIG"}`, wantStatus: 400, wantCode: "INVALID_REQUEST"},
		{name: "unknown_field", body: `{"request_id":"970000000000000001","wager":"10","choice":"BIG","user_id":"1"}`, wantStatus: 400, wantCode: "INVALID_REQUEST"},
		{name: "trailing_json", body: botGamesTestPrepare + `{}`, wantStatus: 400, wantCode: "INVALID_REQUEST"},
		{name: "4096", body: botGamesTestPrepare + strings.Repeat(" ", 4096-len(botGamesTestPrepare)), wantStatus: 404, wantCode: "NOT_LINKED"},
		{name: "4097", body: botGamesTestPrepare + strings.Repeat(" ", 4097-len(botGamesTestPrepare)), wantStatus: 400, wantCode: "INVALID_REQUEST"},
		{name: "4097_stream", body: botGamesTestPrepare + strings.Repeat(" ", 4097-len(botGamesTestPrepare)), wantStatus: 400, wantCode: "INVALID_REQUEST"},
	}
	for _, name := range []string{"Cookie", "Proxy-Authorization", "New-Api-User", "X-Auth-Session", "X-Native-User", "X-User-ID", "X-Forwarded-User", "X-Authenticated-User", "Idempotency-Key", "X-Fairness-Commitment"} {
		cases = append(cases, testCase{name: name, headers: map[string][]string{name: {""}}, wantStatus: 400, wantCode: "INVALID_REQUEST"})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.method == "" {
				tc.method = "POST"
			}
			if tc.path == "" {
				tc.path = "/internal/v1/bot-games/dice/prepare"
			}
			if tc.body == "" {
				tc.body = botGamesTestPrepare
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
			if w.Code != tc.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.wantStatus, w.Body.String())
			}
			if w.Body.String() != `{"error":"`+tc.wantCode+`"}`+"\n" {
				t.Fatalf("unprojected error: %s", w.Body.String())
			}
			if w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("private JSON headers absent")
			}
		})
	}
	t.Run("legacy_limit", func(t *testing.T) {
		valid := `{"a":"b"}`
		if _, e := decodeStringFields(strings.NewReader(valid+strings.Repeat(" ", 2048-len(valid))), "a"); e != nil {
			t.Fatal("2048 rejected")
		}
		if _, e := decodeStringFields(strings.NewReader(valid+strings.Repeat(" ", 2049-len(valid))), "a"); e == nil {
			t.Fatal("legacy limit weakened")
		}
	})
	t.Run("fault_projection", func(t *testing.T) {
		for code, status := range map[string]int{"INVALID_REQUEST": 400, "UNAUTHORIZED": 401, "BINDING_CHANGED": 403, "ACCOUNT_RESTRICTED": 403, "NOT_LINKED": 404, "NOT_FOUND": 404, "ACCOUNT_NOT_READY": 409, "QUOTE_EXPIRED": 409, "COMMITMENT_INVALID": 409, "IDEMPOTENCY_CONFLICT": 409, "INSUFFICIENT_CHIPS": 409, "MAINTENANCE": 409, "UPSTREAM_UNAVAILABLE": 503, "RAW_SQL_SECRET": 503} {
			h := botGamesTestHandler(t, botGamesTestResolver(func(context.Context, string) (int64, error) { return 0, botgames.Fault{Code: code} }))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, botGamesRequest("POST", "/internal/v1/bot-games/dice/prepare", botGamesTestPrepare))
			if w.Code != status || strings.Contains(w.Body.String(), "RAW_SQL_SECRET") {
				t.Fatalf("fault %s status=%d", code, w.Code)
			}
		}
	})
	t.Run("bounded_inflight", func(t *testing.T) {
		entered := make(chan struct{}, 16)
		release := make(chan struct{})
		h := botGamesTestHandler(t, botGamesTestResolver(func(ctx context.Context, _ string) (int64, error) {
			d, ok := ctx.Deadline()
			if !ok || time.Until(d) > 8*time.Second {
				t.Error("handler deadline missing")
			}
			entered <- struct{}{}
			<-release
			return 0, botgames.Fault{Code: "NOT_LINKED"}
		}))
		var wg sync.WaitGroup
		defer func() { close(release); wg.Wait() }()
		for range 16 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				h.ServeHTTP(httptest.NewRecorder(), botGamesRequest("POST", "/internal/v1/bot-games/dice/prepare", botGamesTestPrepare))
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
		done := make(chan struct{})
		go func() {
			h.ServeHTTP(w, botGamesRequest("POST", "/internal/v1/bot-games/dice/prepare", botGamesTestPrepare))
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("overload waited instead of rejecting")
		}
		if w.Code != 503 {
			t.Fatalf("overload status=%d", w.Code)
		}
	})
}
