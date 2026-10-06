package botgames

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func signedSummonFixture(raw, domain string) string {
	mac := hmac.New(sha256.New, testKey[:])
	mac.Write([]byte(domain))
	mac.Write([]byte(raw))
	return base64.RawURLEncoding.EncodeToString([]byte(raw)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestSummonQuoteIndependentMACAndBindings(t *testing.T) {
	s, _, r, _, _ := newSummonFixture(t)
	q := prepareSummonQuote(t, s, "TENFOLD")
	parts := strings.Split(q, ".")
	raw, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		t.Fatal(e)
	}
	if q != signedSummonFixture(string(raw), "bot-games.summon.quote.v1\x00") {
		t.Fatal("wrong signature domain")
	}
	var p map[string]any
	_ = json.Unmarshal(raw, &p)
	if len(p) != 10 || p["game"] != "summon" || p["v"] != float64(1) || p["base_wager"] != "11" || p["mode"] != "TENFOLD" || p["request_id"] != testRequest || p["subject"] != testSubject || p["binding"] != s.binding(testSubject, r.user) || p["commitment_id"] != testCommitment || p["iat"] != float64(1700000000) || p["exp"] != float64(1700000120) {
		t.Fatal(p)
	}
}

func TestSummonQuoteCrossUseAllThreeDomains(t *testing.T) {
	ctx := context.Background()
	ds, _, _, _, _ := newFixture(t)
	dq := prepareQuote(t, ds)
	ss, _, _, _, _ := newSlotFixture(t)
	sq := prepareSlotQuote(t, ss)
	us, g, r, _, _ := newSummonFixture(t)
	uq := prepareSummonQuote(t, us, "SINGLE")
	before := r.calls
	for _, q := range []string{dq, sq} {
		_, e := us.PlaySummon(ctx, testSubject, q)
		requireFault(t, e, "UNAUTHORIZED")
		_, e = us.LookupSummon(ctx, testSubject, q)
		requireFault(t, e, "UNAUTHORIZED")
	}
	for _, q := range []string{sq, uq} {
		_, e := ds.Play(ctx, testSubject, q)
		requireFault(t, e, "UNAUTHORIZED")
		_, e = ds.Lookup(ctx, testSubject, q)
		requireFault(t, e, "UNAUTHORIZED")
	}
	for _, q := range []string{dq, uq} {
		_, e := ss.PlaySlot(ctx, testSubject, q)
		requireFault(t, e, "UNAUTHORIZED")
		_, e = ss.LookupSlot(ctx, testSubject, q)
		requireFault(t, e, "UNAUTHORIZED")
	}
	if r.calls != before || g.findCalls != 0 || g.createCalls != 0 {
		t.Fatal("cross use reached authority")
	}
}

func TestSummonQuoteStrictSignedPayloadAndTamper(t *testing.T) {
	s, g, r, now, _ := newSummonFixture(t)
	q := prepareSummonQuote(t, s, "SINGLE")
	parts := strings.Split(q, ".")
	bytes, _ := base64.RawURLEncoding.DecodeString(parts[0])
	raw := string(bytes)
	mutations := map[string]string{
		"duplicate":      strings.Replace(raw, `"mode":"SINGLE"`, `"mode":"SINGLE","mode":"TENFOLD"`, 1),
		"unknown":        strings.TrimSuffix(raw, "}") + `,"seed":"caller"}`,
		"case_alias":     strings.Replace(raw, `"mode"`, `"MODE"`, 1),
		"missing":        strings.Replace(raw, `"mode":"SINGLE",`, "", 1),
		"null":           strings.Replace(raw, `"mode":"SINGLE"`, `"mode":null`, 1),
		"numeric_wager":  strings.Replace(raw, `"base_wager":"11"`, `"base_wager":11`, 1),
		"wager_format":   strings.Replace(raw, `"base_wager":"11"`, `"base_wager":"011"`, 1),
		"mode":           strings.Replace(raw, `"mode":"SINGLE"`, `"mode":"single"`, 1),
		"game":           strings.Replace(raw, `"game":"summon"`, `"game":"slot"`, 1),
		"version":        strings.Replace(raw, `"v":1`, `"v":2`, 1),
		"expiry":         strings.Replace(raw, `"exp":1700000120`, `"exp":1700000121`, 1),
		"future":         strings.ReplaceAll(strings.ReplaceAll(raw, "1700000000", "1700000001"), "1700000120", "1700000121"),
		"issue_zero":     strings.ReplaceAll(strings.ReplaceAll(raw, "1700000000", "0"), "1700000120", "120"),
		"issue_overflow": strings.Replace(raw, "1700000000", "9223372036854775800", 1),
		"subject":        strings.Replace(raw, testSubject, "01", 1),
		"request":        strings.Replace(raw, testRequest, "0", 1),
		"commitment":     strings.Replace(raw, testCommitment, "invalid", 1),
		"binding":        strings.Replace(raw, s.binding(testSubject, r.user), "invalid", 1),
		"trailing":       raw + `{}`, "array": "[]",
	}
	before := r.calls
	for name, value := range mutations {
		t.Run(name, func(t *testing.T) {
			if value == raw {
				t.Fatal("mutation not applied")
			}
			bad := signedSummonFixture(value, "bot-games.summon.quote.v1\x00")
			_, e := s.PlaySummon(context.Background(), testSubject, bad)
			requireFault(t, e, "INVALID_REQUEST")
		})
	}
	for _, bad := range []string{"", "x.y.z", strings.Repeat("x", 2049), parts[0] + "=." + parts[1]} {
		_, e := s.PlaySummon(context.Background(), testSubject, bad)
		requireFault(t, e, "INVALID_REQUEST")
	}
	for _, bad := range []string{".", parts[0] + ".YQ", parts[0] + "." + parts[1] + "=", signedSummonFixture(raw, "bot-games.slot.quote.v1\x00")} {
		_, e := s.PlaySummon(context.Background(), testSubject, bad)
		requireFault(t, e, "UNAUTHORIZED")
	}
	if r.calls != before || g.createCalls != 0 || g.findCalls != 0 {
		t.Fatal("invalid quote reached authority")
	}
	*now = now.Add(120 * time.Second)
	_, e := s.LookupSummon(context.Background(), testSubject, q)
	requireFault(t, e, "NOT_FOUND")
}
