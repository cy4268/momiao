package main

import (
	"strings"
	"testing"
)

func TestGameTypedCreateBoundary(t *testing.T) {
	for _, tc := range []struct{ slug, body string }{
		{"dice", `{"type":"DICE","wager":"10","choice":"BIG"}`},
		{"scratch", `{"type":"SCRATCH","wager":"10"}`},
		{"summon", `{"type":"SUMMON","base_wager":"10","mode":"TENFOLD"}`},
		{"slot", `{"type":"SLOT","total_wager":"11"}`},
		{"blackjack", `{"type":"BLACKJACK","initial_wager":"10"}`},
	} {
		if _, err := decodeGameCreate(tc.slug, strings.NewReader(tc.body)); err != nil {
			t.Fatal(err)
		}
	}
	for _, body := range []string{
		`{"type":"DICE","wager":"10","choice":"BIG","payout":"200"}`,
		`{"type":"DICE","wager":"10","choice":"BIG","nonce":"0"}`,
		`{"type":"DICE","wager":"10","choice":"BIG","choice":"SMALL"}`,
		`{"type":"DICE","wager":10,"choice":"BIG"}`,
		`{"type":"DICE","wager":"10","choice":"BIG"} {}`,
		`{"type":"DICE","wager":"10"}`,
	} {
		if _, err := decodeGameCreate("dice", strings.NewReader(body)); err == nil {
			t.Fatal("unsafe game body accepted", body)
		}
	}
}
func TestGameBrowserRoutes(t *testing.T) {
	for _, route := range []string{"/games/dice", "/games/scratch", "/games/summon", "/games/slot", "/games/blackjack", "/history", "/history/01993200-0000-7000-8000-000000000001"} {
		if !gameBrowserRoute(route) || gateRouteDomain(route) != "EXPERIENCE" {
			t.Fatal("game route unavailable", route)
		}
	}
	for _, route := range []string{"/history/../account", "/history/not-a-round", "/games/dice/extra", "/api/v1/games/dice"} {
		if gameBrowserRoute(route) {
			t.Fatal("unsafe browser route", route)
		}
	}
}
func TestBlackjackActionBoundary(t *testing.T) {
	body := `{"action_id":"01993200-0000-7000-8000-000000000001","action_type":"STAND","hand_id":"01993200-0000-7000-8000-000000000002","expected_round_version":"2"}`
	a, err := decodeBlackjackAction(strings.NewReader(body))
	if err != nil || a.ExpectedVersion != "2" || a.ActionType != "STAND" {
		t.Fatal(a, err)
	}
	for _, invalid := range []string{strings.Replace(body, `"2"`, `2`, 1), strings.Replace(body, `"2"}`, `"2","new_hand_id":"client-picked"}`, 1), strings.Replace(body, `"STAND"`, `"STAND","action_type":"HIT"`, 1)} {
		if _, err := decodeBlackjackAction(strings.NewReader(invalid)); err == nil {
			t.Fatal("unsafe action accepted")
		}
	}
}
