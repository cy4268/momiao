package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/poker/connectticket"
)

// Reuses the configured dev Redis; it creates no database users or services.
func TestLocalSessionLifecycle(t *testing.T) {
	path := os.Getenv("MOMIAO_SESSION_REDIS_CONFIG_FILE")
	if path == "" {
		t.Skip("requires the existing dev session Redis configuration")
	}
	cfg := config{WalletDSNFile: os.Getenv("MOMIAO_SESSION_DSN_FILE"), PublicOrigin: "https://dev.momiao.win"}
	values := map[string]string{"MOMIAO_SESSION_ENABLED": "true", "MOMIAO_SESSION_REDIS_CONFIG_FILE": path}
	if err := loadSessionConfig(&cfg, func(key string) (string, bool) { value, ok := values[key]; return value, ok }); err != nil {
		t.Fatalf("local-only session configuration: %v", err)
	}
	client, err := openSessionRedis(t.Context(), cfg.Session.RedisConfigFile)
	if err != nil {
		t.Fatal("local Redis startup", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	sessions := identity.Sessions{Redis: client}
	userID := time.Now().UnixNano() // Redis-only test ID, not an application account.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = sessions.RevokeUser(ctx, userID)
	})
	first, err := sessions.Create(t.Context(), userID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := sessions.Create(t.Context(), userID)
	if err != nil {
		t.Fatal(err)
	}
	indexTTL, err := client.PTTL(t.Context(), "chaldea:session:local:user:"+strconv.FormatInt(userID, 10)).Result()
	if err != nil || indexTTL <= 29*24*time.Hour || indexTTL > 30*24*time.Hour {
		t.Fatal("user session index must expire after its last session", indexTTL, err)
	}
	w := httptest.NewRecorder()
	identity.WriteSessionCookie(w, first)
	cookie := w.Result().Cookies()[0]
	if !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode || cookie.Value != first.Token || !cookie.Expires.After(time.Now()) {
		t.Fatal("session cookie attributes")
	}
	loaded, err := sessions.Read(t.Context(), cookie.Value)
	if err != nil || loaded.UserID != userID || loaded.CSRFToken != first.CSRFToken || !loaded.CreatedAt.Equal(first.CreatedAt) {
		t.Fatal("session round trip")
	}
	if err := sessions.Revoke(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Read(t.Context(), first.Token); !errors.Is(err, identity.ErrSessionExpired) {
		t.Fatal("logout did not revoke this browser")
	}
	if _, err := sessions.Read(t.Context(), second.Token); err != nil {
		t.Fatal("logout revoked another browser")
	}
	if err := sessions.RevokeUser(t.Context(), userID); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Read(t.Context(), second.Token); !errors.Is(err, identity.ErrSessionExpired) {
		t.Fatal("user-wide revocation left a session active")
	}
	if readerPath := os.Getenv("MOMIAO_POKER_SESSION_REDIS_CONFIG_FILE"); readerPath != "" {
		t.Run("poker-reader", func(t *testing.T) {
			reader, err := openSessionRedis(t.Context(), readerPath)
			if err != nil {
				t.Fatal("Poker session reader connection", err)
			}
			defer reader.Close()
			pool, err := openPokerPool(t.Context(), os.Getenv("MOMIAO_POKER_AUTH_TEST_DSN_FILE"))
			if err != nil {
				t.Fatal("Poker authority database connection", err)
			}
			defer pool.Close()
			var user int64
			if err := pool.QueryRow(t.Context(), `SELECT id FROM identity.users WHERE status=1 AND deleted_at IS NULL ORDER BY id LIMIT 1`).Scan(&user); err != nil {
				t.Fatal("Poker local user read", err)
			}
			var writable bool
			if err := pool.QueryRow(t.Context(), `SELECT has_table_privilege(current_user,'identity.users','INSERT,UPDATE,DELETE')`).Scan(&writable); err != nil || writable {
				t.Fatal("Poker user reader has write access", err)
			}
			value, err := sessions.Create(t.Context(), user)
			if err != nil {
				t.Fatal(err)
			}
			defer sessions.Revoke(context.Background(), value)
			digest := sha256.Sum256([]byte(value.Token))
			id := hex.EncodeToString(digest[:])
			key := "chaldea:session:local:" + id
			before, err := reader.PTTL(t.Context(), key).Result()
			if err != nil {
				t.Fatal(err)
			}
			authority := pokerSessionAuthority{sessions: identity.Sessions{Redis: reader}, pool: pool}
			ref := connectticket.Session{UserID: strconv.FormatInt(user, 10), SessionIDHash: id, SessionVersion: 1}
			deadline, err := authority.CheckUntil(t.Context(), ref)
			if err != nil || !deadline.After(time.Now()) {
				t.Fatal("Poker did not read the platform's live session", err)
			}
			after, err := reader.PTTL(t.Context(), key).Result()
			if err != nil || after > before {
				t.Fatal("Poker extended the session lifetime", err)
			}
			if err := reader.Set(t.Context(), key, "test-only", time.Minute).Err(); err == nil || !strings.Contains(err.Error(), "NOPERM") {
				t.Fatal("Poker session reader can write", err)
			}
			if err := reader.Get(t.Context(), "chaldea:session:flow:reader-check").Err(); err == nil || !strings.Contains(err.Error(), "NOPERM") {
				t.Fatal("Poker session reader can access login flows", err)
			}
			if err := sessions.Revoke(t.Context(), value); err != nil {
				t.Fatal(err)
			}
			if _, err := authority.CheckUntil(t.Context(), ref); !errors.Is(err, connectticket.ErrRevoked) {
				t.Fatal("Poker accepted a revoked local session", err)
			}
			t.Log("POKER_LOCAL_SESSION_READ_ONLY_AND_REVOKED")
		})
	}
}
