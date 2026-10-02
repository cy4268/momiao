package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestBotGamesApplication(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		a, e := openBotGamesApplication(context.Background(), config{}, nil, nil)
		if e != nil || a != nil {
			t.Fatalf("default opened listener: %v", e)
		}
	})
	t.Run("invalid_enabled", func(t *testing.T) {
		cfg := config{BotGames: botGamesConfig{Socket: filepath.Join(t.TempDir(), "bot.sock")}}
		if _, e := openBotGamesApplication(context.Background(), cfg, nil, nil); e == nil {
			t.Fatal("incomplete application started")
		}
		if _, e := os.Stat(cfg.BotGames.Socket); !os.IsNotExist(e) {
			t.Fatal("startup failure left socket")
		}
	})
	t.Run("socket_lifecycle", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("POSIX 0600 acceptance requires Linux")
		}
		p := filepath.Join(t.TempDir(), "bot.sock")
		l, e := openListener(config{ListenSocket: p})
		if e != nil {
			t.Fatal(e)
		}
		a := &botGamesApplication{listener: l, server: &http.Server{Handler: botGamesTestHandler(t, botGamesTestResolver(func(context.Context, string) (int64, error) { return 0, nil }))}}
		t.Cleanup(func() { _ = a.Close() })
		info, e := os.Stat(p)
		if e != nil {
			t.Fatal(e)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("socket mode=%o", info.Mode().Perm())
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- a.Run(ctx, time.Second) }()
		conn, e := net.DialTimeout("unix", p, time.Second)
		if e != nil {
			t.Fatal(e)
		}
		_ = conn.Close()
		cancel()
		select {
		case e := <-done:
			if e != nil {
				t.Fatal(e)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("shutdown stuck")
		}
		if e = a.Close(); e != nil {
			t.Fatal(e)
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatal("owned socket not removed")
		}
		if e = a.Close(); e != nil {
			t.Fatal("close not idempotent")
		}
	})
	t.Run("existing_file", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "keep")
		if e := os.WriteFile(p, []byte("keep"), 0600); e != nil {
			t.Fatal(e)
		}
		if l, e := openListener(config{ListenSocket: p}); e == nil {
			l.Close()
			t.Fatal("existing file overwritten")
		}
		b, e := os.ReadFile(p)
		if e != nil || string(b) != "keep" {
			t.Fatal("existing path changed")
		}
	})
	t.Run("private_hex_files", func(t *testing.T) {
		for _, tc := range []struct {
			name, raw string
			mode      os.FileMode
			ok        bool
		}{{"valid", botGamesTestToken, 0600, true}, {"newline", botGamesTestToken + "\n", 0600, true}, {"uppercase", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", 0600, false}, {"short", "aa", 0600, false}, {"zero", "0000000000000000000000000000000000000000000000000000000000000000", 0600, false}, {"public", botGamesTestToken, 0644, false}} {
			t.Run(tc.name, func(t *testing.T) {
				if runtime.GOOS == "windows" && tc.name == "public" {
					t.Skip("POSIX file permissions")
				}
				p := filepath.Join(t.TempDir(), "key")
				if e := os.WriteFile(p, []byte(tc.raw), tc.mode); e != nil {
					t.Fatal(e)
				}
				_, e := readBotGamesKey(p)
				if (e == nil) != tc.ok {
					t.Fatalf("private key accepted=%t want=%t", e == nil, tc.ok)
				}
			})
		}
	})
	t.Run("listener_failure", func(t *testing.T) {
		l, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		_ = l.Close()
		a := &botGamesApplication{listener: l, server: httptest.NewUnstartedServer(http.NotFoundHandler()).Config}
		if e = a.Run(context.Background(), time.Second); e == nil || e.Error() != "bot games listener failed" {
			t.Fatalf("listener failure not stable: %v", e)
		}
	})
}
