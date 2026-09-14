package bffauth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/session"
	"github.com/redis/go-redis/v9"
)

// Redis is real; the Native transport and unused F4 handle are controlled fixtures.
// This exercises post-call CAS finalization, not full authentication acceptance.
func TestCanceledNativeOperationFinalizesRedis(t *testing.T) {
	addr, password := os.Getenv("S1_CANCEL_REDIS_ADDR"), os.Getenv("S1_CANCEL_REDIS_PASSWORD")
	if addr == "" || password == "" {
		t.Skip("explicit isolated Redis fixture required")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, Username: "s1fixture", Password: password, Protocol: 2, MaxRetries: -1, ContextTimeoutEnabled: true, DisableIdentity: true, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second})
	defer rdb.Close()
	base, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	if err := rdb.Ping(base).Err(); err != nil {
		t.Fatal("fixture Redis unavailable")
	}
	for _, kind := range []string{"password", "twofa", "refresh", "logout finalization"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(base)
			defer cancel()
			calls := 0
			s, err := New(Options{Redis: rdb, Sessions: &session.Service{}, Origin: "https://fixture.invalid", CredentialSealKey: [32]byte{1}, Native: logoutTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Context().Err() != nil {
					t.Error("Native call cancelled before fixture trigger")
				}
				if kind == "logout finalization" {
					h := make(http.Header)
					h.Set("Content-Type", "application/json")
					h.Set("Set-Cookie", "new_api_refresh=; Path=/api/user/auth; Max-Age=0; HttpOnly; Secure; SameSite=Strict")
					return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(`{"success":true,"message":""}`))}, nil
				}
				cancel() // Disconnect only after the production operation has acquired its CAS state.
				return nil, context.Canceled
			})})
			if err != nil {
				t.Fatal("fixture service construction")
			}
			var key string
			defer func() {
				if key == "" {
					return
				}
				cleanup, done := context.WithTimeout(context.Background(), time.Second)
				defer done()
				if err := rdb.Del(cleanup, key).Err(); err != nil {
					t.Error("owned-key cleanup failed")
				}
				if _, err := rdb.Get(cleanup, key).Result(); !errors.Is(err, redis.Nil) {
					t.Error("owned key still present")
				}
			}()
			if kind == "refresh" || kind == "logout finalization" {
				sid, err := randomSecret()
				if err != nil {
					t.Fatal("fixture entropy")
				}
				nativeHash := hashText(sid)
				key = credentialPrefix + nativeHash
				credential := nativeCredential{UserID: "1", Username: "fixture", SID: sid, AccessToken: "fixture-expired", RefreshToken: "fixture-refresh", AccessExpiresAt: time.Now().Add(-time.Minute).Unix(), NativeExpiresAt: time.Now().Add(time.Hour).Unix()}
				if err := s.storeCredential(base, nativeHash, credential, time.Now().Add(30*time.Minute)); err != nil {
					t.Fatal("fixture credential store")
				}
				binding := session.BFFBinding{UserID: "1", NativeSessionIDHash: nativeHash}
				if kind == "logout finalization" {
					cancel()
					// Enter the owned-intent helper directly. An invalid F4 handle must
					// remain UNKNOWN; only Native compensation/Redis state is asserted.
					result := s.revokeAuthenticated(ctx, session.RequestSession{}, binding)
					if result.NativeSession != "REVOKED" || result.PlatformSession != "UNKNOWN" {
						t.Errorf("logout result=%+v", result)
					}
					raw, _, _, readErr := s.readValue(base, key)
					var record credentialRecord
					if readErr != nil || !strictJSON([]byte(raw), &record, 16384) || record.State != "REVOKED" || record.Box != "" {
						t.Error("logout credential revocation not persisted")
					}
				} else {
					_, err = s.ensureCredential(ctx, binding)
					if err == nil {
						t.Error("uncertain refresh reported success")
					}
					r, _, readErr := s.loadCredential(base, nativeHash)
					if readErr != nil || r.State != "UNKNOWN" {
						t.Errorf("persisted credential state=%s err=%v, want UNKNOWN", r.State, readErr)
					}
				}
			} else {
				sid, view, err := s.NewAnonymous(base)
				if err != nil {
					t.Fatal("fixture anonymous state")
				}
				key = preauthPrefix + hashText(sid)
				r, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.origin+"/api/v1/auth/password/login", nil)
				r.Header.Set("Cookie", CookieName+"="+sid)
				r.Header.Set("Origin", s.origin)
				r.Header.Set("Sec-Fetch-Site", "same-origin")
				r.Header.Set("X-CSRF-Token", view.CSRFToken)
				if kind == "twofa" {
					pending, op, e := s.begin(r, "")
					if e != nil {
						t.Fatal("fixture password state")
					}
					challenge, e := s.challenge(base, pending, op, "fixture-flow", time.Now().Add(time.Minute))
					if e != nil {
						t.Fatal("fixture challenge")
					}
					_, err = s.AuthenticateTwoFA(r, challenge.ID, "123456")
				} else {
					_, err = s.AuthenticatePassword(r, "fixture", "fixture", "")
				}
				if err == nil {
					t.Error("uncertain login reported success")
				}
				pending, readErr := s.loadPreauth(base, sid)
				if readErr != nil || pending.State != "UNKNOWN" {
					t.Errorf("persisted preauth state=%s err=%v, want UNKNOWN", pending.State, readErr)
				}
			}
			if calls != 1 {
				t.Errorf("Native calls=%d, want exactly one", calls)
			}
		})
	}
}
