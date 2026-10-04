package botgames

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBlackjackQuoteExactCanonicalAndDomain(t *testing.T) {
	s, g, _, _ := newBlackjackFixture(t)
	deal := blackjackDeal(t, s)
	round, e := s.projectBlackjackRound(testSubject, 424242, g.current)
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct{ kind, quote string }{{"DEAL", deal.Quote}, {"ROUND", round.Quote}} {
		t.Run(tc.kind, func(t *testing.T) {
			p, e := s.verifyBlackjackQuote(tc.quote, tc.kind)
			if e != nil || p.Game != "blackjack" || p.Kind != tc.kind || p.Subject != testSubject || p.ExpiresAt != "1700000120" {
				t.Fatal("roundtrip", e)
			}
			raw, e := base64.RawURLEncoding.DecodeString(strings.Split(tc.quote, ".")[0])
			if e != nil {
				t.Fatal(e)
			}
			for name, change := range map[string]func(string) string{
				"unknown":            func(v string) string { return strings.TrimSuffix(v, "}") + `,"authority":"x"}` },
				"duplicate":          func(v string) string { return strings.TrimSuffix(v, "}") + `,"v":1}` },
				"case_alias":         func(v string) string { return strings.Replace(v, `"subject":`, `"Subject":`, 1) },
				"null":               func(v string) string { return strings.Replace(v, `"initial_wager":"10"`, `"initial_wager":null`, 1) },
				"numeric_wager":      func(v string) string { return strings.Replace(v, `"initial_wager":"10"`, `"initial_wager":10`, 1) },
				"noncanonical_wager": func(v string) string { return strings.Replace(v, `"initial_wager":"10"`, `"initial_wager":"010"`, 1) },
				"wrong_game":         func(v string) string { return strings.Replace(v, `"game":"blackjack"`, `"game":"dice"`, 1) },
				"wrong_kind":         func(v string) string { return strings.Replace(v, `"kind":"`+tc.kind+`"`, `"kind":"OTHER"`, 1) },
				"long_expiry": func(v string) string {
					return strings.Replace(v, `"expires_at":"1700000120"`, `"expires_at":"1700000121"`, 1)
				},
				"future_issued": func(v string) string {
					return strings.Replace(strings.Replace(v, `"issued_at":"1700000000"`, `"issued_at":"1700000001"`, 1), `"expires_at":"1700000120"`, `"expires_at":"1700000121"`, 1)
				},
				"noncanonical_whitespace": func(v string) string { return " " + v },
				"trailing":                func(v string) string { return v + "{}" },
			} {
				t.Run(name, func(t *testing.T) {
					altered := []byte(change(string(raw)))
					q := base64.RawURLEncoding.EncodeToString(altered) + "." + base64.RawURLEncoding.EncodeToString(s.blackjackQuoteMAC(altered))
					_, e := s.verifyBlackjackQuote(q, tc.kind)
					wantBlackjackFault(t, e, "INVALID_REQUEST")
				})
			}
			for _, invalid := range []string{"", strings.Repeat("x", 2049), "a.b.c"} {
				_, e := s.verifyBlackjackQuote(invalid, tc.kind)
				wantBlackjackFault(t, e, "INVALID_REQUEST")
			}
			parts := strings.Split(tc.quote, ".")
			sig, _ := base64.RawURLEncoding.DecodeString(parts[1])
			sig[0] ^= 1
			_, e = s.verifyBlackjackQuote(parts[0]+"."+base64.RawURLEncoding.EncodeToString(sig), tc.kind)
			wantBlackjackFault(t, e, "UNAUTHORIZED")
		})
	}
	old := prepareQuote(t, func() *Service { s, _, _, _, _ := newFixture(t); return s }())
	_, e = s.verifyBlackjackQuote(old, "DEAL")
	wantBlackjackFault(t, e, "UNAUTHORIZED")
	_, e = s.verifyQuote(deal.Quote)
	wantBlackjackFault(t, e, "UNAUTHORIZED")
	_, e = s.verifySlotQuote(deal.Quote)
	wantBlackjackFault(t, e, "UNAUTHORIZED")
	_, e = s.verifySummonQuote(deal.Quote)
	wantBlackjackFault(t, e, "UNAUTHORIZED")
	_, e = s.verifyScratchQuote(deal.Quote)
	wantBlackjackFault(t, e, "UNAUTHORIZED")
	_, e = s.verifyBlackjackQuote(deal.Quote, "ROUND")
	wantBlackjackFault(t, e, "INVALID_REQUEST")
	_, e = s.verifyBlackjackQuote(round.Quote, "DEAL")
	wantBlackjackFault(t, e, "INVALID_REQUEST")
}
func TestBlackjackActionIDUUIDv7StableIndependentDomain(t *testing.T) {
	s, _, _, _ := newBlackjackFixture(t)
	for _, request := range []string{"1", testRequest, "18446744073709551615"} {
		id, e := s.blackjackActionID(testSubject, request)
		if e != nil {
			t.Fatal(e)
		}
		again, e := s.blackjackActionID(testSubject, request)
		if e != nil || again != id {
			t.Fatal("unstable action id")
		}
		raw, e := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
		if e != nil || len(raw) != 16 || raw[6]>>4 != 7 || raw[8]>>6 != 2 {
			t.Fatal("invalid action UUID")
		}
		millis := uint64(0)
		for _, v := range raw[:6] {
			millis = millis<<8 | uint64(v)
		}
		n, _ := strconv.ParseUint(request, 10, 64)
		if millis != (n>>22)+1420070400000 {
			t.Fatal("wrong snowflake timestamp")
		}
		other, e := s.blackjackActionID("970000000000000003", request)
		if e != nil || other == id {
			t.Fatal("subject not bound")
		}
	}
	for _, request := range []string{"0", "01", "-1", "18446744073709551616"} {
		_, e := s.blackjackActionID(testSubject, request)
		wantBlackjackFault(t, e, "INVALID_REQUEST")
	}
}
func TestBlackjackQuoteBindingAndExpiredReads(t *testing.T) {
	s, g, res, now := newBlackjackFixture(t)
	r, e := s.projectBlackjackRound(testSubject, 424242, g.current)
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.verifyBlackjackQuote(r.Quote, "ROUND")
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.StateBlackjack(context.Background(), "970000000000000003", r.Quote)
	wantBlackjackFault(t, e, "UNAUTHORIZED")
	res.user++
	_, e = s.StateBlackjack(context.Background(), testSubject, r.Quote)
	wantBlackjackFault(t, e, "BINDING_CHANGED")
	res.user--
	*now = now.Add(3600 * time.Second)
	_, e = s.StateBlackjack(context.Background(), testSubject, r.Quote)
	if e != nil {
		t.Fatal("expired read", e)
	}
	for _, change := range []func(*blackjackQuotePayload){func(p *blackjackQuotePayload) { p.RoundVersion = "01" }, func(p *blackjackQuotePayload) { p.RoundVersion = "0" }, func(p *blackjackQuotePayload) { p.RoundVersion = "9223372036854775807" }, func(p *blackjackQuotePayload) { p.StakeUnits = "4999999" }, func(p *blackjackQuotePayload) { p.ActiveHandStakeUnits = "5000001" }, func(p *blackjackQuotePayload) { p.RoundID = "not-uuid" }, func(p *blackjackQuotePayload) { x := ""; p.ActiveHandID = &x }, func(p *blackjackQuotePayload) { p.InitialWager = "108510259257115010" }} {
		changed := p
		change(&changed)
		q, e := s.signBlackjackQuote(changed)
		if e != nil {
			t.Fatal(e)
		}
		_, e = s.verifyBlackjackQuote(q, "ROUND")
		wantBlackjackFault(t, e, "INVALID_REQUEST")
	}
	// Terminal tokens retain an explicit empty hand field and canonical zero stake.
	g.current = blackjackFixtureRound(t, true)
	r, e = s.projectBlackjackRound(testSubject, 424242, g.current)
	if e != nil {
		t.Fatal(e)
	}
	p, e = s.verifyBlackjackQuote(r.Quote, "ROUND")
	if e != nil || p.ActiveHandID == nil || *p.ActiveHandID != "" || p.ActiveHandStakeUnits != "0" {
		t.Fatal("terminal token", e)
	}
	raw, _ := json.Marshal(p)
	if !strings.Contains(string(raw), `"active_hand_id":""`) {
		t.Fatal("terminal empty field omitted")
	}
}
