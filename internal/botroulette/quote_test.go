package botroulette

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestQuoteStableDeadline(t *testing.T) {
	s, f, _, now := fixture(t)
	q1 := prepare(t, s, createInput())
	*now = now.Add(60 * time.Second)
	q2 := prepare(t, s, createInput())
	if q1.ExpiresAt != q2.ExpiresAt || q1.Quote != q2.Quote {
		t.Fatal("same request renewed")
	}
	iat, _ := strconv.ParseInt(q1.IssuedAt, 10, 64)
	exp, _ := strconv.ParseInt(q1.ExpiresAt, 10, 64)
	if iat != 1791194400000 || exp != 1791194520000 || exp-iat != 120000 {
		t.Fatal("not 120 seconds from snowflake")
	}
	*now = testTime.Add(120 * time.Second)
	_, err := s.Prepare(context.Background(), subject, createInput())
	requireFault(t, err, "QUOTE_EXPIRED")
	*now = testTime.Add(-time.Millisecond)
	_, err = s.Prepare(context.Background(), subject, createInput())
	requireFault(t, err, "INVALID_REQUEST")
	if f.mutations != 0 {
		t.Fatal("prepare mutated")
	}
}

func TestPrepareRejectsDeadlineCrossedDuringRead(t *testing.T) {
	for _, in := range []PrepareInput{createInput(), readyInput()} {
		t.Run(in.Purpose, func(t *testing.T) {
			s, f, _, now := fixture(t)
			f.afterRead = func() { *now = testTime.Add(120 * time.Second) }
			_, err := s.Prepare(context.Background(), subject, in)
			requireFault(t, err, "QUOTE_EXPIRED")
			if f.mutations != 0 {
				t.Fatal("expired prepare mutated")
			}
		})
	}
}

func authenticatedQuote(raw string, domain string) string {
	mac := hmac.New(sha256.New, testKey[:])
	mac.Write([]byte(domain))
	mac.Write([]byte(raw))
	return base64.RawURLEncoding.EncodeToString([]byte(raw)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func quoteRaw(t *testing.T, quote string) string {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(strings.Split(quote, ".")[0])
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestQuoteRejectsCrossGameSubjectBinding(t *testing.T) {
	s, f, r, _ := fixture(t)
	p := prepare(t, s, createInput())
	raw := quoteRaw(t, p.Quote)
	for _, tc := range []struct{ name, quote, actor, code string }{
		{"subject", p.Quote, "223456789012345678", "UNAUTHORIZED"},
		{"other-domain", authenticatedQuote(raw, "bot-games.quote.v1\x00"), subject, "UNAUTHORIZED"},
		{"other-game", authenticatedQuote(strings.Replace(raw, "devil-roulette", "blackjack", 1), "bot-roulette.quote.v1\x00"), subject, "INVALID_REQUEST"},
		{"tampered", authenticatedQuote(raw, "bad-domain"), subject, "UNAUTHORIZED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.Commit(context.Background(), tc.actor, tc.quote)
			requireFault(t, err, tc.code)
		})
	}
	r.user = 43
	_, err := s.Commit(context.Background(), subject, p.Quote)
	requireFault(t, err, "BINDING_CHANGED")
	_, err = s.Lookup(context.Background(), subject, p.Quote)
	requireFault(t, err, "BINDING_CHANGED")
	if f.mutations != 0 || f.lookups != 0 {
		t.Fatal("invalid quote reached engine")
	}
	if strings.Contains(raw, "native") || strings.Contains(raw, `"user"`) {
		t.Fatal("native identity leaked")
	}
	mac := hmac.New(sha256.New, testKey[:])
	mac.Write([]byte("bot-roulette.binding.v1\x00" + subject + "\x0042"))
	if p.BindingFingerprint != hex.EncodeToString(mac.Sum(nil)) {
		t.Fatal("binding domain changed")
	}
}

func TestQuoteRejectsStrictNestedJSONAndTimeTampering(t *testing.T) {
	s, f, _, _ := fixture(t)
	p := prepare(t, s, readyInput())
	raw := quoteRaw(t, p.Quote)
	for _, tc := range []struct {
		name   string
		change func(string) string
	}{
		{"extra", func(s string) string { return strings.Replace(s, `"v":1`, `"v":1,"extra":1`, 1) }},
		{"duplicate", func(s string) string { return strings.Replace(s, `"v":1`, `"v":1,"v":1`, 1) }},
		{"case", func(s string) string { return strings.Replace(s, `"subject"`, `"Subject"`, 1) }},
		{"nested-extra", func(s string) string {
			return strings.Replace(s, `"kind":"READY"`, `"kind":"READY","unknown":false`, 1)
		}},
		{"ready-extra", func(s string) string { return strings.Replace(s, `"client_seed":`, `"unknown":true,"client_seed":`, 1) }},
		{"seed-duplicate", func(s string) string {
			return strings.Replace(s, `"client_seed":`, `"client_seed":"other","client_seed":`, 1)
		}},
		{"null-seed", func(s string) string {
			return strings.Replace(s, `"client_seed":"御主的种子"`, `"client_seed":null`, 1)
		}},
		{"integer-version", func(s string) string { return strings.Replace(s, `"expected_version":"7"`, `"expected_version":7`, 1) }},
		{"bool-version", func(s string) string {
			return strings.Replace(s, `"expected_version":"7"`, `"expected_version":true`, 1)
		}},
		{"leading-zero", func(s string) string {
			return strings.Replace(s, `"expected_version":"7"`, `"expected_version":"07"`, 1)
		}},
		{"overflow", func(s string) string {
			return strings.Replace(s, `"expected_version":"7"`, `"expected_version":"9223372036854775808"`, 1)
		}},
		{"iat", func(s string) string {
			return strings.Replace(s, `"issued_at":"1791194400000"`, `"issued_at":"1791194400001"`, 1)
		}},
		{"exp", func(s string) string {
			return strings.Replace(s, `"expires_at":"1791194520000"`, `"expires_at":"1791194520001"`, 1)
		}},
		{"uuid-v4", func(s string) string { return strings.Replace(s, roomID, "019923a0-0000-4000-8000-000000000001", 1) }},
		{"trailing", func(s string) string { return s + `{}` }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			modified := tc.change(raw)
			if modified == raw {
				t.Fatal("test did not alter quote")
			}
			_, err := s.Lookup(context.Background(), subject, authenticatedQuote(modified, "bot-roulette.quote.v1\x00"))
			requireFault(t, err, "INVALID_REQUEST")
		})
	}
	if f.lookups != 0 || f.mutations != 0 {
		t.Fatal("malformed quote reached engine")
	}
}

func TestTypedInputRejectsInvalidNumbersIDsAndSeeds(t *testing.T) {
	for _, stake := range []string{"", "0", "01", "+1", "1.0", "-1", "18446744073709551616", "18446744073710", "9223372036855"} {
		s, _, _, _ := fixture(t)
		in := createInput()
		in.Input.Create.Stake = stake
		_, err := s.Prepare(context.Background(), subject, in)
		requireFault(t, err, "INVALID_REQUEST")
	}
	for _, id := range []string{"0", "01", "+1", "-1", "18446744073709551616"} {
		s, _, _, _ := fixture(t)
		in := createInput()
		in.RequestID = id
		_, err := s.Prepare(context.Background(), subject, in)
		requireFault(t, err, "INVALID_REQUEST")
	}
	for _, seed := range []string{"", strings.Repeat("a", 129), strings.Repeat("界", 43), "a\n", "a\x7f", "a\u0085", string([]byte{0xff})} {
		s, _, _, _ := fixture(t)
		in := readyInput()
		in.Input.Command.Ready.ClientSeed = seed
		_, err := s.Prepare(context.Background(), subject, in)
		requireFault(t, err, "INVALID_REQUEST")
	}
	for _, seed := range []string{"a", strings.Repeat("a", 128), strings.Repeat("界", 42) + "ab"} {
		s, _, _, _ := fixture(t)
		in := readyInput()
		in.Input.Command.Ready.ClientSeed = seed
		prepare(t, s, in)
	}
	for _, players := range []int{0, 1, 3, 6, 7} {
		s, _, _, _ := fixture(t)
		in := createInput()
		in.Input.Create.Players = players
		_, err := s.Prepare(context.Background(), subject, in)
		requireFault(t, err, "INVALID_REQUEST")
	}
	for _, players := range []int{3, 4, 5, 6} {
		s, f, _, _ := fixture(t)
		f.lobby.Game = "pressure-roulette"
		in := createInput()
		in.Game = "pressure-roulette"
		in.Input.Create.Players = players
		prepare(t, s, in)
	}
}

func TestWireInputStrictUnionAndNullPositions(t *testing.T) {
	valid := `{"request_id":"1480544260915200000","purpose":"CREATE","game":"devil-roulette","input":{"stake":"10","players":2}}`
	for _, tc := range []struct{ name, raw string }{
		{"bool-players", strings.Replace(valid, `"players":2`, `"players":true`, 1)},
		{"numeric-stake", strings.Replace(valid, `"stake":"10"`, `"stake":10`, 1)},
		{"extra-input", strings.Replace(valid, `"stake":"10"`, `"stake":"10","room_id":null`, 1)},
		{"null-input", strings.Replace(valid, `{"stake":"10","players":2}`, `null`, 1)},
		{"wrong-purpose", strings.Replace(valid, `"CREATE"`, `"COMMAND"`, 1)},
		{"create-room", strings.Replace(valid, `"input":`, `"room_id":"`+roomID+`","input":`, 1)},
		{"case", strings.Replace(valid, `"game"`, `"Game"`, 1)},
		{"duplicate", strings.Replace(valid, `"players":2`, `"players":2,"players":2`, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var in PrepareInput
			if err := json.Unmarshal([]byte(tc.raw), &in); err == nil {
				t.Fatal("invalid JSON admitted")
			}
		})
	}
	var in PrepareInput
	if err := json.Unmarshal([]byte(valid), &in); err != nil || in.Input.Create == nil || in.Input.Create.Stake != "10" {
		t.Fatalf("valid union: %+v %v", in, err)
	}
	command := `{"request_id":"1480544260915200000","purpose":"COMMAND","game":"devil-roulette","room_id":"` + roomID + `","input":{"expected_version":"7","action":{"kind":"LEAVE"},"ready":null}}`
	if err := json.Unmarshal([]byte(command), &in); err != nil || in.Input.Command == nil || in.Input.Command.Ready != nil {
		t.Fatalf("command: %+v %v", in, err)
	}
	for _, raw := range []string{strings.Replace(command, `,"ready":null`, "", 1), strings.Replace(command, `"kind":"LEAVE"`, `"kind":"LEAVE","agree":null`, 1), strings.Replace(command, `"7"`, `"+7"`, 1)} {
		if err := json.Unmarshal([]byte(raw), &in); err == nil {
			t.Fatal("noncanonical command accepted")
		}
	}
	s, _, _, _ := fixture(t)
	p := prepare(t, s, createInput())
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"room_id":null`) || !strings.Contains(string(raw), `"issued_at":"1791194400000"`) {
		t.Fatal("wrong prepared wire types")
	}
}

func TestReadAndQuoteRequestEnvelopesAreClosed(t *testing.T) {
	for _, tc := range []struct {
		name, good string
		target     func() any
	}{
		{"lobby", `{"game":"devil-roulette","cursor":null}`, func() any { return &LobbyInput{} }},
		{"public", `{"game":"devil-roulette","room_id":"` + roomID + `"}`, func() any { return &RoomInput{} }},
		{"state", `{"room_id":"` + roomID + `"}`, func() any { return &StateInput{} }},
		{"commit-lookup", `{"quote":"signed.quote"}`, func() any { return &QuoteInput{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tc.good), tc.target()); err != nil {
				t.Fatal(err)
			}
			for _, raw := range []string{`null`, `{}`, strings.Replace(tc.good, `{`, `{"extra":1,`, 1), strings.Replace(tc.good, `{`, `{"native_user_id":42,`, 1), tc.good + `{}`} {
				if err := json.Unmarshal([]byte(raw), tc.target()); err == nil {
					t.Fatalf("accepted %s", raw)
				}
			}
		})
	}
	for _, raw := range []string{`{"quote":null}`, `{"quote":true}`, `{"Quote":"x"}`, `{"quote":"x","quote":"y"}`} {
		var in QuoteInput
		if json.Unmarshal([]byte(raw), &in) == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestInvalidRoomActionAndUnionNeverReachEngine(t *testing.T) {
	for _, id := range []string{"", strings.ToUpper(roomID), "019923a0-0000-4000-8000-000000000001", "019923a0-0000-7000-c000-000000000001", strings.ReplaceAll(roomID, "-", "")} {
		s, f, _, _ := fixture(t)
		_, err := s.State(context.Background(), subject, StateInput{RoomID: id})
		requireFault(t, err, "INVALID_REQUEST")
		_, err = s.Public(context.Background(), RoomInput{Game: "devil-roulette", RoomID: id})
		requireFault(t, err, "INVALID_REQUEST")
		if len(f.users) != 0 || f.publics != 0 {
			t.Fatal("invalid room reached engine")
		}
	}
	for _, edit := range []func(*PrepareInput){
		func(in *PrepareInput) { in.Input.Command.Action = Action{Kind: "TIMEOUT"} },
		func(in *PrepareInput) { in.Input.Command.Action = Action{Kind: "GAME_LIMIT"} },
		func(in *PrepareInput) { in.Input.Command.ExpectedVersion = 0 },
		func(in *PrepareInput) { in.Input.Command.Ready = nil },
		func(in *PrepareInput) { in.Input.Create = &CreateInput{Stake: "10", Players: 2} },
		func(in *PrepareInput) { in.Purpose = "CREATE" },
		func(in *PrepareInput) { in.RoomID = nil },
		func(in *PrepareInput) { in.Input.Command.Action = Action{Kind: "JOIN", Target: "SELF"} },
	} {
		s, f, _, _ := fixture(t)
		in := readyInput()
		edit(&in)
		_, err := s.Prepare(context.Background(), subject, in)
		requireFault(t, err, "INVALID_REQUEST")
		if len(f.users) != 0 {
			t.Fatal("invalid intent reached engine")
		}
	}
}

func TestMalformedQuoteNeverReachesResolver(t *testing.T) {
	s, f, r, _ := fixture(t)
	p := prepare(t, s, createInput())
	before := len(r.subjects)
	for _, quote := range []string{"", "a", strings.Repeat("x", 4097), "=.a", p.Quote + ".extra", p.Quote + "="} {
		_, err := s.Lookup(context.Background(), subject, quote)
		if err == nil {
			t.Fatal("malformed quote accepted")
		}
	}
	if len(r.subjects) != before || f.lookups != 0 || f.mutations != 0 {
		t.Fatal("malformed quote reached identity/engine")
	}
}

func readyWireSeed(t *testing.T, seedJSON string) []byte {
	t.Helper()
	raw, err := json.Marshal(readyInput())
	if err != nil {
		t.Fatal(err)
	}
	wire := strings.Replace(string(raw), `"client_seed":"御主的种子"`, `"client_seed":`+seedJSON, 1)
	if wire == string(raw) && seedJSON != `"御主的种子"` {
		t.Fatal("test did not replace the wire seed")
	}
	return []byte(wire)
}

func TestPrepareWireRejectsUnpairedSurrogates(t *testing.T) {
	for _, tc := range []struct{ name, seedJSON string }{
		{"lone-high", `"\ud800"`},
		{"lone-low", `"\udfff"`},
		{"high-end", `"\udbff"`},
		{"low-start", `"\udc00"`},
		{"high-then-text", `"\ud800a"`},
		{"high-then-bmp", `"\ud800\u0041"`},
		{"high-then-high", `"\ud800\udbff"`},
		{"reversed-pair", `"\udfff\ud800"`},
		{"pair-then-low", `"\ud83d\ude80\udfff"`},
		{"high-then-literal-escape", `"\ud800\\udc00"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, f, r, _ := fixture(t)
			var in PrepareInput
			err := json.Unmarshal(readyWireSeed(t, tc.seedJSON), &in)
			var preview PreparedReply
			if err == nil {
				preview, err = s.Prepare(context.Background(), subject, in)
			}
			if err == nil {
				t.Fatalf("unpaired surrogate admitted: decoded_seed=%q quote_issued=%t", in.Input.Command.Ready.ClientSeed, preview.Quote != "")
			}
			requireFault(t, err, "INVALID_REQUEST")
			if len(r.subjects) != 0 || f.views != 0 || f.mutations != 0 || preview.Quote != "" {
				t.Fatal("invalid wire seed reached identity, engine or signing")
			}
		})
	}
}

func TestPrepareWirePreservesValidUnicodeAndEscapes(t *testing.T) {
	for _, tc := range []struct{ name, seedJSON, want string }{
		{"chinese", `"御主的种子"`, "御主的种子"},
		{"escaped-chinese", `"\u5fa1\u4e3b"`, "御主"},
		{"emoji-pair", `"\ud83d\ude80"`, "🚀"},
		{"uppercase-pair", `"\uD83D\uDE80"`, "🚀"},
		{"literal-emoji", `"🚀"`, "🚀"},
		{"literal-replacement", `"�"`, "�"},
		{"escaped-replacement", `"\ufffd"`, "�"},
		{"literal-escape", `"\\ud800"`, `\ud800`},
		{"literal-escape-pair", `"\\ud800\\udfff"`, `\ud800\udfff`},
		{"two-backslashes", `"\\\\ud800"`, `\\ud800`},
		{"backslash-then-pair", `"\\\ud83d\ude80"`, `\🚀`},
		{"lowest-pair", `"\ud800\udc00"`, "\U00010000"},
		{"highest-pair", `"\udbff\udfff"`, "\U0010ffff"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, f, _, _ := fixture(t)
			var in PrepareInput
			if err := json.Unmarshal(readyWireSeed(t, tc.seedJSON), &in); err != nil {
				t.Fatal(err)
			}
			preview := prepare(t, s, in)
			if preview.Input.Command.Ready.ClientSeed != tc.want || preview.Quote == "" {
				t.Fatalf("preview changed seed: got %q, want %q", preview.Input.Command.Ready.ClientSeed, tc.want)
			}
			if _, err := s.Lookup(context.Background(), subject, preview.Quote); err != nil {
				t.Fatal(err)
			}
			if f.intent.Command.Ready.ClientSeed != tc.want || f.mutations != 0 {
				t.Fatal("signed wire seed changed across read-only recovery")
			}
		})
	}
}
