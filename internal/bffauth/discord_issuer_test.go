package bffauth

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestNativeUICallbackIssuer(t *testing.T) {
	addr, password := os.Getenv("S1_CANCEL_REDIS_ADDR"), os.Getenv("S1_CANCEL_REDIS_PASSWORD")
	if addr == "" || password == "" {
		t.Skip("explicit isolated Redis fixture required")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, Username: "s1fixture", Password: password, Protocol: 2, MaxRetries: -1, DisableIdentity: true})
	defer rdb.Close()
	ctx := context.Background()
	s := &Service{redis: rdb}
	sid, _, err := s.NewAnonymous(ctx)
	if err != nil {
		t.Fatal(err)
	}
	key := preauthPrefix + hashText(sid)
	defer rdb.Del(ctx, key)
	record, err := s.loadPreauth(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	state := strings.Repeat("s", 43)
	record.State, record.Purpose, record.AuthSurface = "DISCORD_CALLBACK", DiscordPurposeLogin, nativeUIAuthSurface
	record.NativeStateHash, record.NativeBrowserBox = hashText(state), "unused-routing-fixture"
	store := func(record preauthRecord) {
		t.Helper()
		raw, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		if err := rdb.Set(ctx, key, raw, time.Until(time.UnixMilli(record.Expires))).Err(); err != nil {
			t.Fatal(err)
		}
	}
	store(record)
	success := "code=synthetic-code&state=" + state
	denied := "error=access_denied&state=" + state
	issuer := "&iss=https%3A%2F%2Fdiscord.com"
	for _, tc := range []struct {
		name, query string
		want        bool
	}{
		{"legacy success", success, true},
		{"issuer success", success + issuer, true},
		{"legacy denial", denied, true},
		{"issuer denial", denied + issuer, true},
		{"issuer denial description", denied + "&error_description=Cancelled" + issuer, true},
		{"wrong issuer", success + "&iss=https://evil.example", false},
		{"wrong issuer denial", denied + "&iss=https://evil.example", false},
		{"empty issuer", success + "&iss=", false},
		{"issuer suffix", success + "&iss=https://discord.com.evil.example", false},
		{"issuer slash", success + "&iss=https://discord.com/", false},
		{"issuer case", success + "&iss=https://DISCORD.com", false},
		{"duplicate issuer", success + issuer + issuer, false},
		{"unknown parameter", success + issuer + "&extra=x", false},
		{"duplicate code", success + issuer + "&code=other", false},
		{"duplicate state", success + issuer + "&state=" + state, false},
		{"mixed response", success + issuer + "&error=access_denied", false},
		{"missing state", "code=synthetic-code" + issuer, false},
		{"wrong state", "code=synthetic-code&state=" + strings.Repeat("x", 43) + issuer, false},
		{"missing code", "state=" + state + issuer, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "https://fixture.invalid/oauth/discord?"+tc.query, nil)
			r.Header.Set("Cookie", CookieName+"="+sid)
			if got := s.OwnsNativeUICallback(r); got != tc.want {
				t.Fatalf("OwnsNativeUICallback=%v, want %v", got, tc.want)
			}
		})
	}
	for _, tc := range []struct{ name, surface, phase, cookie string }{
		{"missing cookie", nativeUIAuthSurface, "DISCORD_CALLBACK", ""},
		{"different session", nativeUIAuthSurface, "DISCORD_CALLBACK", strings.Repeat("x", 43)},
		{"platform ceremony", "", "DISCORD_CALLBACK", sid},
		{"consumed ceremony", nativeUIAuthSurface, "DISCORD_CALLBACK_PENDING", sid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := record
			changed.AuthSurface, changed.State = tc.surface, tc.phase
			if changed.State == "DISCORD_CALLBACK_PENDING" {
				changed.OperationHash = hashText("owned-operation")
			}
			store(changed)
			r := httptest.NewRequest("GET", "https://fixture.invalid/oauth/discord?"+success+issuer, nil)
			if tc.cookie != "" {
				r.Header.Set("Cookie", CookieName+"="+tc.cookie)
			}
			if s.OwnsNativeUICallback(r) {
				t.Fatal("issuer bypassed ceremony ownership")
			}
		})
	}
}
