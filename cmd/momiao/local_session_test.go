package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/identity"
	"github.com/redis/go-redis/v9"
)

// Reuses the configured dev Redis; it creates no database users or services.
func TestLocalSessionLifecycle(t *testing.T) {
	path := os.Getenv("MOMIAO_SESSION_REDIS_CONFIG_FILE")
	if path == "" {
		t.Skip("requires the existing dev session Redis configuration")
	}
	options, err := readSessionRedisConfig(path)
	if err != nil {
		t.Fatal("read session Redis config")
	}
	client := redis.NewClient(options)
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
}
