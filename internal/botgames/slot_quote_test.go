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

// The fixture signer is independent of the implementation, so malformed but
// authenticated payloads exercise parsing rather than just signature rejection.
func slotFixturePayload(t *testing.T) string {
	t.Helper()
	mac := hmac.New(sha256.New, testKey[:])
	mac.Write([]byte("bot-games.binding.v1\x00" + testSubject + "\x00424242"))
	fields := map[string]any{
		"v": 1, "request_id": testRequest, "subject": testSubject,
		"binding": hex.EncodeToString(mac.Sum(nil)), "total_wager": "11", "game": "slot",
		"commitment_id": testCommitment, "iat": int64(1700000000), "exp": int64(1700000120),
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func slotFixtureSign(raw string) string {
	mac := hmac.New(sha256.New, testKey[:])
	mac.Write([]byte("bot-games.slot.quote.v1\x00"))
	mac.Write([]byte(raw))
	return base64.RawURLEncoding.EncodeToString([]byte(raw)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestSlotQuoteRejectsSignedNoncanonicalFields(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(string) string
	}{
		{"unknown", func(raw string) string { return strings.Replace(raw, "{", `{"native_id":424242,`, 1) }},
		{"duplicate", func(raw string) string { return strings.Replace(raw, "{", `{"total_wager":"11",`, 1) }},
		{"case-alias", func(raw string) string { return strings.Replace(raw, `"total_wager"`, `"TOTAL_WAGER"`, 1) }},
		{"missing", func(raw string) string { return strings.Replace(raw, `"game":"slot",`, "", 1) }},
		{"null", func(raw string) string { return strings.Replace(raw, `"game":"slot"`, `"game":null`, 1) }},
		{"version", func(raw string) string { return strings.Replace(raw, `"v":1`, `"v":2`, 1) }},
		{"noninteger-version", func(raw string) string { return strings.Replace(raw, `"v":1`, `"v":1.0`, 1) }},
		{"noncanonical-wager", func(raw string) string { return strings.Replace(raw, `"total_wager":"11"`, `"total_wager":"011"`, 1) }},
		{"numeric-wager", func(raw string) string { return strings.Replace(raw, `"total_wager":"11"`, `"total_wager":11`, 1) }},
		{"wager-overflow", func(raw string) string {
			return strings.Replace(raw, `"total_wager":"11"`, `"total_wager":"18446744073710"`, 1)
		}},
		{"request-overflow", func(raw string) string { return strings.Replace(raw, testRequest, "18446744073709551616", 1) }},
		{"game", func(raw string) string { return strings.Replace(raw, `"slot"`, `"dice"`, 1) }},
		{"empty-commitment", func(raw string) string { return strings.Replace(raw, testCommitment, "", 1) }},
		{"invalid-commitment", func(raw string) string { return strings.Replace(raw, testCommitment, "not-a-uuid", 1) }},
		{"binding-format", func(raw string) string { return strings.Replace(raw, `"binding":"`, `"binding":"x`, 1) }},
		{"lifetime", func(raw string) string { return strings.Replace(raw, "1700000120", "1700000121", 1) }},
		{"zero-issued", func(raw string) string { return strings.Replace(raw, "1700000000", "0", 1) }},
		{"time-overflow", func(raw string) string { return strings.Replace(raw, "1700000120", "9223372036854775808", 1) }},
		{"time-string", func(raw string) string { return strings.Replace(raw, "1700000000", `"1700000000"`, 1) }},
		{"future", func(raw string) string {
			return strings.ReplaceAll(strings.ReplaceAll(raw, "1700000000", "1700000001"), "1700000120", "1700000121")
		}},
		{"trailing-json", func(raw string) string { return raw + "{}" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, g, r, _, _ := newSlotFixture(t)
			_, err := s.PlaySlot(context.Background(), testSubject, slotFixtureSign(tc.change(slotFixturePayload(t))))
			requireFault(t, err, "INVALID_REQUEST")
			if r.calls != 0 || g.findCalls != 0 || g.createCalls != 0 {
				t.Fatal("malformed quote reached dependencies")
			}
		})
	}
}

func TestSlotQuoteChecksSignatureBeforePayload(t *testing.T) {
	s, g, r, _, _ := newSlotFixture(t)
	q := slotFixtureSign(`{"unknown":"private"}`)
	parts := strings.Split(q, ".")
	parts[1] = strings.Repeat("A", 43)
	_, err := s.PlaySlot(context.Background(), testSubject, strings.Join(parts, "."))
	requireFault(t, err, "UNAUTHORIZED")
	if r.calls != 0 || g.findCalls != 0 || g.createCalls != 0 {
		t.Fatal("bad signature reached dependencies")
	}
}

func TestSlotQuoteEnvelopeBound(t *testing.T) {
	for _, q := range []string{"", "a.b.c", strings.Repeat("A", 2049), "a.b", "====.===="} {
		s, g, r, _, _ := newSlotFixture(t)
		_, err := s.PlaySlot(context.Background(), testSubject, q)
		if err == nil || (err.Error() != "INVALID_REQUEST" && err.Error() != "UNAUTHORIZED") {
			t.Fatal(err)
		}
		if r.calls != 0 || g.findCalls != 0 || g.createCalls != 0 {
			t.Fatal("bad envelope reached dependencies")
		}
	}
}

func TestSlotQuoteDomainSeparation(t *testing.T) {
	s, g, r, _, _ := newSlotFixture(t)
	raw := slotFixturePayload(t)
	mac := hmac.New(sha256.New, testKey[:])
	mac.Write([]byte("bot-games.binding.v1\x00"))
	mac.Write([]byte(raw))
	q := base64.RawURLEncoding.EncodeToString([]byte(raw)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	_, err := s.PlaySlot(context.Background(), testSubject, q)
	requireFault(t, err, "UNAUTHORIZED")
	if r.calls != 0 || g.createCalls != 0 {
		t.Fatal("cross-domain signature accepted")
	}
}

func TestSlotQuotePersistsAcrossServiceRestart(t *testing.T) {
	s, g, r, now, _ := newSlotFixture(t)
	q := prepareSlotQuote(t, s)
	restarted, err := NewService(g, r, testKey, func() time.Time { return *now })
	if err != nil {
		t.Fatal(err)
	}
	got, err := restarted.PlaySlot(context.Background(), testSubject, q)
	if err != nil || got.RoundID != "00000000-0000-4000-8000-000000000002" || g.createCalls != 1 {
		t.Fatal(got, err)
	}
}

func TestSlotQuotesCannotCrossEitherGameBoundary(t *testing.T) {
	s, g, r, _, _ := newSlotFixture(t)
	slotQuote := prepareSlotQuote(t, s)
	for _, lookup := range []bool{false, true} {
		var e error
		if lookup {
			_, e = s.Lookup(context.Background(), testSubject, slotQuote)
		} else {
			_, e = s.Play(context.Background(), testSubject, slotQuote)
		}
		requireFault(t, e, "UNAUTHORIZED")
		if lookup {
			_, e = s.LookupSlot(context.Background(), testSubject, fixtureSign(fixturePayload(t)))
		} else {
			_, e = s.PlaySlot(context.Background(), testSubject, fixtureSign(fixturePayload(t)))
		}
		requireFault(t, e, "UNAUTHORIZED")
	}
	if r.calls != 1 || g.findCalls != 0 || g.createCalls != 0 {
		t.Fatal("cross-game quote reached dependency")
	}
}
