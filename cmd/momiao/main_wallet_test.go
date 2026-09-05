package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunWalletOpenFailureRedacted(t *testing.T) {
	cfg := config{WalletDSNFile: filepath.Join(t.TempDir(), "missing-secret"), ListenAddr: "127.0.0.1:0"}
	if err := run(context.Background(), cfg, log.New(io.Discard, "", 0)); err == nil || err.Error() != "wallet startup failed" {
		t.Fatal(err)
	}
}

type startupLog chan string

func (c startupLog) Write(p []byte) (int, error) { c <- string(p); return len(p), nil }
func TestWalletDatabaseOutageDoesNotBlockPortalStartup(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("portal alive"), 0600)
	dsn := filepath.Join(dir, "dsn")
	os.WriteFile(dsn, []byte("host=127.0.0.1 port=1 user=wallet password=private dbname=wallet sslmode=disable"), 0600)
	cfg := config{WalletDSNFile: dsn, PublicOrigin: "https://wallet.example", WebDir: dir, NewAPISocket: filepath.Join(dir, "unavailable.sock"), ListenAddr: "127.0.0.1:0", ShutdownTimeout: time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logs := make(startupLog, 4)
	done := make(chan error, 1)
	go func() { done <- run(ctx, cfg, log.New(logs, "", 0)) }()
	var addr string
	select {
	case line := <-logs:
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != "listening" {
			t.Fatal(line)
		}
		addr = fields[2]
	case err := <-done:
		t.Fatalf("wallet outage prevented startup: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("startup connected to database")
	}
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	for _, path := range []string{"/healthz", "/wallet", "/dashboard"} {
		resp, err := client.Get("http://" + addr + path)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatal(path, resp.StatusCode)
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
func TestRunMalformedWalletDSNRedacted(t *testing.T) {
	p := filepath.Join(t.TempDir(), "dsn")
	os.WriteFile(p, []byte("postgres://%private"), 0600)
	if err := run(context.Background(), config{WalletDSNFile: p}, log.New(io.Discard, "", 0)); err == nil || err.Error() != "wallet startup failed" {
		t.Fatal(err)
	}
}
