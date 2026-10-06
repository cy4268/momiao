package botgames

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func rawScratchQuote(raw []byte) string {
	mac := hmac.New(sha256.New, testKey[:])
	mac.Write([]byte("bot-games.scratch.quote.v1\x00"))
	mac.Write(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func TestScratchQuoteExactAuthenticatedEnvelope(t *testing.T) {
	for _, resume := range []bool{false, true} {
		t.Run(map[bool]string{false: "purchase", true: "resume"}[resume], func(t *testing.T) {
			s, g, _, _ := newScratchFixture(t)
			p := scratchPrepare(t, s, "11")
			if resume {
				p = resumeScratch(t, s, g)
			}
			raw, e := base64.RawURLEncoding.DecodeString(strings.Split(p.Quote, ".")[0])
			if e != nil {
				t.Fatal(e)
			}
			var fields map[string]any
			if json.Unmarshal(raw, &fields) != nil {
				t.Fatal("quote invalid JSON")
			}
			exactFields(t, fields, "v game action request_id subject binding wager commitment_id round_id iat exp")
			if fields["action"] != p.Action || fields["game"] != "scratch" || fields["wager"] != "11" || fields["request_id"] != testRequest || fields["subject"] != testSubject || fields["iat"] != float64(1700000000) || fields["exp"] != float64(1700000120) {
				t.Fatal("quote authority mismatch")
			}
			if rawScratchQuote(raw) != p.Quote {
				t.Fatal("wrong signature domain")
			}
			for name, mutate := range map[string]func(map[string]any){"version": func(v map[string]any) { v["v"] = 2 }, "game": func(v map[string]any) { v["game"] = "dice" }, "action": func(v map[string]any) { v["action"] = "UNKNOWN" }, "subject": func(v map[string]any) { v["subject"] = "01" }, "request": func(v map[string]any) { v["request_id"] = "01" }, "binding": func(v map[string]any) { v["binding"] = "bad" }, "wager": func(v map[string]any) { v["wager"] = "011" }, "extra": func(v map[string]any) { v["extra"] = "x" }, "missing": func(v map[string]any) { delete(v, "round_id") }, "future": func(v map[string]any) { v["iat"] = 1700000001; v["exp"] = 1700000121 }, "duration": func(v map[string]any) { v["exp"] = 1700000121 }, "null": func(v map[string]any) { v["round_id"] = nil }, "wrong_uuid": func(v map[string]any) {
				if resume {
					v["round_id"] = "bad"
				} else {
					v["commitment_id"] = "bad"
				}
			}, "both_ids": func(v map[string]any) { v["round_id"] = g.created.ID; v["commitment_id"] = testCommitment }} {
				t.Run(name, func(t *testing.T) {
					var v map[string]any
					json.Unmarshal(raw, &v)
					mutate(v)
					changed, _ := json.Marshal(v)
					_, e := s.LookupScratch(context.Background(), testSubject, rawScratchQuote(changed))
					requireFault(t, e, "INVALID_REQUEST")
				})
			}
			for _, bad := range [][]byte{append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"wager":"11"}`)...), []byte(strings.Replace(string(raw), `"game"`, `"GAME"`, 1))} {
				_, e = s.PlayScratch(context.Background(), testSubject, rawScratchQuote(bad))
				requireFault(t, e, "INVALID_REQUEST")
			}
		})
	}
}
func TestScratchQuoteEveryCrossGameDirection(t *testing.T) {
	s, g, _, _ := newScratchFixture(t)
	scratch := scratchPrepare(t, s, "11").Quote
	for _, fn := range []func(string) error{func(q string) error { _, e := s.Play(context.Background(), testSubject, q); return e }, func(q string) error { _, e := s.Lookup(context.Background(), testSubject, q); return e }, func(q string) error { _, e := s.PlaySlot(context.Background(), testSubject, q); return e }, func(q string) error { _, e := s.LookupSlot(context.Background(), testSubject, q); return e }, func(q string) error { _, e := s.PlaySummon(context.Background(), testSubject, q); return e }, func(q string) error { _, e := s.LookupSummon(context.Background(), testSubject, q); return e }} {
		requireFault(t, fn(scratch), "UNAUTHORIZED")
	}
	dice, _, _, _, _ := newFixture(t)
	dq := prepareQuote(t, dice)
	for _, sign := range []func([]byte) []byte{s.quoteMAC, s.slotQuoteMAC, s.summonQuoteMAC} {
		raw := []byte(`{}`)
		q := base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(sign(raw))
		for _, fn := range []func(context.Context, string, string) (ScratchResult, error){s.PlayScratch, s.LookupScratch} {
			_, e := fn(context.Background(), testSubject, q)
			requireFault(t, e, "UNAUTHORIZED")
		}
	}
	_, e := s.PlayScratch(context.Background(), testSubject, dq)
	requireFault(t, e, "UNAUTHORIZED")
	if g.createCalls != 0 || g.revealCalls != 0 {
		t.Fatal("cross-domain mutation")
	}
}
func TestScratchRevealActionDeterministicDomainSeparatedUUIDv7(t *testing.T) {
	s, g, r, now := newScratchFixture(t)
	p := scratchPrepare(t, s, "11")
	_, e := s.PlayScratch(context.Background(), testSubject, p.Quote)
	if e != nil {
		t.Fatal(e)
	}
	first := g.actionID
	b, e := hex.DecodeString(strings.ReplaceAll(first, "-", ""))
	if e != nil || len(b) != 16 || b[6]>>4 != 7 || b[8]>>6 != 2 || first == g.created.ID {
		t.Fatal("not a distinct UUIDv7", first)
	}
	roundBytes, _ := hex.DecodeString(strings.ReplaceAll(g.created.ID, "-", ""))
	if string(b[:6]) != string(roundBytes[:6]) {
		t.Fatal("timestamp bytes changed")
	}
	restart, e := NewService(g, r, testKey, func() time.Time { return *now })
	if e != nil {
		t.Fatal(e)
	}
	g.found = &g.created
	_, e = restart.LookupScratch(context.Background(), testSubject, p.Quote)
	if e != nil || g.actionID != first {
		t.Fatal("restart completion ID changed")
	}
	key := testKey
	key[0]++
	other, e := NewService(g, r, key, func() time.Time { return *now })
	if e != nil {
		t.Fatal(e)
	}
	p = scratchPrepare(t, other, "11")
	_, e = other.LookupScratch(context.Background(), testSubject, p.Quote)
	if e != nil || g.actionID == first {
		t.Fatal("reveal ID is not key-bound")
	}
	g.created.ID = "00000000-0000-7000-8000-000000000003"
	_, e = restart.LookupScratch(context.Background(), testSubject, scratchPrepare(t, restart, "11").Quote)
	if e != nil || g.actionID == first {
		t.Fatal("different rounds reused reveal action")
	}
}
