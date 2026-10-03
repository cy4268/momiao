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
func fixturePayload(t *testing.T) string {
	t.Helper()
	mac := hmac.New(sha256.New, testKey[:])
	mac.Write([]byte("bot-games.binding.v1\x00" + testSubject + "\x00424242"))
	fields := map[string]any{
		"v": 1, "request_id": testRequest, "subject": testSubject,
		"binding": hex.EncodeToString(mac.Sum(nil)), "wager": "10", "choice": "BIG",
		"commitment_id": testCommitment, "iat": int64(1700000000), "exp": int64(1700000120),
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func fixtureSign(raw string) string {
	mac := hmac.New(sha256.New, testKey[:])
	mac.Write([]byte("bot-games.quote.v1\x00"))
	mac.Write([]byte(raw))
	return base64.RawURLEncoding.EncodeToString([]byte(raw)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestQuoteRejectsSignedNoncanonicalFields(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(string) string
	}{
		{"unknown", func(raw string) string { return strings.Replace(raw, "{", `{"native_id":424242,`, 1) }},
		{"duplicate", func(raw string) string { return strings.Replace(raw, "{", `{"wager":"10",`, 1) }},
		{"case-alias", func(raw string) string { return strings.Replace(raw, `"wager"`, `"WAGER"`, 1) }},
		{"missing", func(raw string) string { return strings.Replace(raw, `"choice":"BIG",`, "", 1) }},
		{"null", func(raw string) string { return strings.Replace(raw, `"choice":"BIG"`, `"choice":null`, 1) }},
		{"version", func(raw string) string { return strings.Replace(raw, `"v":1`, `"v":2`, 1) }},
		{"noninteger-version", func(raw string) string { return strings.Replace(raw, `"v":1`, `"v":1.0`, 1) }},
		{"noncanonical-wager", func(raw string) string { return strings.Replace(raw, `"wager":"10"`, `"wager":"010"`, 1) }},
		{"numeric-wager", func(raw string) string { return strings.Replace(raw, `"wager":"10"`, `"wager":10`, 1) }},
		{"wager-overflow", func(raw string) string { return strings.Replace(raw, `"wager":"10"`, `"wager":"9223372036855"`, 1) }},
		{"request-overflow", func(raw string) string { return strings.Replace(raw, testRequest, "18446744073709551616", 1) }},
		{"choice", func(raw string) string { return strings.Replace(raw, `"BIG"`, `"big"`, 1) }},
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
			s, g, r, _, _ := newFixture(t)
			_, err := s.Play(context.Background(), testSubject, fixtureSign(tc.change(fixturePayload(t))))
			requireFault(t, err, "INVALID_REQUEST")
			if r.calls != 0 || g.findCalls != 0 || g.createCalls != 0 {
				t.Fatal("malformed quote reached dependencies")
			}
		})
	}
}

func TestQuoteChecksSignatureBeforePayload(t *testing.T) {
	s, g, r, _, _ := newFixture(t)
	q := fixtureSign(`{"unknown":"private"}`)
	parts := strings.Split(q, ".")
	parts[1] = strings.Repeat("A", 43)
	_, err := s.Play(context.Background(), testSubject, strings.Join(parts, "."))
	requireFault(t, err, "UNAUTHORIZED")
	if r.calls != 0 || g.findCalls != 0 || g.createCalls != 0 {
		t.Fatal("bad signature reached dependencies")
	}
}

func TestQuoteEnvelopeBound(t *testing.T) {
	for _, q := range []string{"", "a.b.c", strings.Repeat("A", 2049), "a.b", "====.===="} {
		s, g, r, _, _ := newFixture(t)
		_, err := s.Play(context.Background(), testSubject, q)
		if err == nil || (err.Error() != "INVALID_REQUEST" && err.Error() != "UNAUTHORIZED") {
			t.Fatal(err)
		}
		if r.calls != 0 || g.findCalls != 0 || g.createCalls != 0 {
			t.Fatal("bad envelope reached dependencies")
		}
	}
}

func TestQuoteDomainSeparation(t *testing.T) {
	s, g, r, _, _ := newFixture(t)
	raw := fixturePayload(t)
	mac := hmac.New(sha256.New, testKey[:])
	mac.Write([]byte("bot-games.binding.v1\x00"))
	mac.Write([]byte(raw))
	q := base64.RawURLEncoding.EncodeToString([]byte(raw)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	_, err := s.Play(context.Background(), testSubject, q)
	requireFault(t, err, "UNAUTHORIZED")
	if r.calls != 0 || g.createCalls != 0 {
		t.Fatal("cross-domain signature accepted")
	}
}

func TestQuotePersistsAcrossServiceRestart(t *testing.T) {
	s, g, r, now, _ := newFixture(t)
	q := prepareQuote(t, s)
	restarted, err := NewService(g, r, testKey, func() time.Time { return *now })
	if err != nil {
		t.Fatal(err)
	}
	got, err := restarted.Play(context.Background(), testSubject, q)
	if err != nil || got.RoundID != "00000000-0000-4000-8000-000000000002" || g.createCalls != 1 {
		t.Fatal(got, err)
	}
}
