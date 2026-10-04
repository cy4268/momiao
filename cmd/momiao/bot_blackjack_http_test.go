package main

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cy4268/momiao/internal/botgames"
)

func TestBotBlackjackPrivatePrepareRoute(t *testing.T) {
	h := botGamesTestHandler(t, botGamesTestResolver(func(context.Context, string) (int64, error) { return 0, botgames.Fault{Code: "NOT_LINKED"} }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, botGamesRequest("POST", "/internal/v1/bot-games/blackjack/prepare", `{"request_id":"970000000000000001","initial_wager":"10"}`))
	if w.Code != 404 || w.Body.String() != `{"error":"NOT_LINKED"}`+"\n" {
		t.Fatalf("blackjack prepare must resolve private linked account; status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestBotBlackjackHTTPExactBodiesAndErrors(t *testing.T) {
	h := botGamesTestHandler(t, botGamesTestResolver(func(context.Context, string) (int64, error) { return 0, botgames.Fault{Code: "NOT_LINKED"} }))
	for _, operation := range []string{"prepare", "play", "lookup", "state", "action", "action-lookup"} {
		t.Run(operation, func(t *testing.T) {
			body := `{"quote":"e30.YQ"}`
			want := 401
			code := "UNAUTHORIZED"
			if operation == "prepare" {
				body = `{"request_id":"970000000000000001","initial_wager":"10"}`
				want = 404
				code = "NOT_LINKED"
			}
			if operation == "action" || operation == "action-lookup" {
				body = `{"quote":"e30.YQ","request_id":"970000000000000001","action_type":"HIT"}`
			}
			path := "/internal/v1/bot-games/blackjack/" + operation
			w := httptest.NewRecorder()
			h.ServeHTTP(w, botGamesRequest("POST", path, body))
			if w.Code != want || w.Body.String() != `{"error":"`+code+`"}`+"\n" {
				t.Fatalf("route status=%d body=%s", w.Code, w.Body.String())
			}
			for _, bad := range []string{`{}`, `[]`, `null`, body + `{}`, strings.TrimSuffix(body, "}") + `,"unknown":"x"}`, strings.TrimSuffix(body, "}") + `,"quote":"second"}`, strings.ReplaceAll(body, `"request_id"`, `"Request_ID"`), strings.ReplaceAll(body, `"quote"`, `"Quote"`)} {
				if bad == body {
					continue
				}
				w = httptest.NewRecorder()
				h.ServeHTTP(w, botGamesRequest("POST", path, bad))
				if w.Code != 400 {
					t.Fatalf("malformed body accepted status=%d body=%s", w.Code, bad)
				}
			}
			for _, method := range []string{"GET", "PUT", "DELETE"} {
				w = httptest.NewRecorder()
				h.ServeHTTP(w, botGamesRequest(method, path, body))
				if w.Code != 400 {
					t.Fatal("method accepted", method)
				}
			}
			for _, header := range []string{"Cookie", "Proxy-Authorization", "X-User-ID", "Idempotency-Key", "X-Fairness-Commitment"} {
				r := botGamesRequest("POST", path, body)
				r.Header.Set(header, "injected")
				w = httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != 400 {
					t.Fatal("forbidden header", header)
				}
			}
			w = httptest.NewRecorder()
			h.ServeHTTP(w, botGamesRequest("POST", path, body+strings.Repeat(" ", 4097-len(body))))
			if w.Code != 400 {
				t.Fatal("body limit")
			}
		})
	}
	for _, code := range []string{"BLACKJACK_ACTIVE_ROUND", "BLACKJACK_STALE_STATE", "BLACKJACK_ACTION_NOT_ALLOWED", "BLACKJACK_NEEDS_REVIEW"} {
		h := botGamesTestHandler(t, botGamesTestResolver(func(context.Context, string) (int64, error) { return 0, botgames.Fault{Code: code} }))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, botGamesRequest("POST", "/internal/v1/bot-games/blackjack/prepare", `{"request_id":"970000000000000001","initial_wager":"10"}`))
		if w.Code != 409 || w.Body.String() != `{"error":"`+code+`"}`+"\n" {
			t.Fatal("business status", code, w.Code)
		}
	}
}
