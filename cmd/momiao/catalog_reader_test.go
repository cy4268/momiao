package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type catalogRoundTrip func(*http.Request) (*http.Response, error)

func (f catalogRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCatalogReaderFixedRequestAndSanitizedFailures(t *testing.T) {
	key := strings.Repeat("a", 64)
	for _, kind := range []string{"network", "redirect", "status", "content_type", "oversize", "partial", "timeout"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			transport := catalogRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "GET" || r.URL.String() != "http://unix/internal/momiao/catalog" || r.Header.Get("Authorization") != "Bearer "+key || len(r.Header) != 2 {
					t.Fatalf("reader request broadened: %s %s", r.Method, r.URL)
				}
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > 5*time.Second {
					t.Fatal("missing bounded deadline")
				}
				if kind == "network" {
					return nil, errors.New("DO_NOT_EXPOSE_" + key)
				}
				if kind == "timeout" {
					<-r.Context().Done()
					return nil, r.Context().Err()
				}
				response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"success":false,"private":"DO_NOT_EXPOSE"}`))}
				if kind == "redirect" {
					response.StatusCode = 302
					response.Header.Set("Location", "https://untrusted.invalid/")
				}
				if kind == "status" {
					response.StatusCode = 503
				}
				if kind == "content_type" {
					response.Header.Set("Content-Type", "text/html")
				}
				if kind == "oversize" {
					response.Body = io.NopCloser(strings.NewReader(strings.Repeat("x", (2<<20)+1)))
				}
				return response, nil
			})
			reader := nativeCatalogReader{transport: transport, key: key}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			_, err := reader.Read(ctx)
			if err == nil || strings.Contains(err.Error(), key) || strings.Contains(err.Error(), "DO_NOT_EXPOSE") {
				t.Fatalf("error not safe: %v", err)
			}
			if calls != 1 {
				t.Fatalf("redirect/retry unexpectedly followed: %d", calls)
			}
		})
	}
}
func TestCatalogKeyFileAndTimingConfig(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "reader.key")
	if err := os.WriteFile(keyPath, []byte(strings.Repeat("c", 64)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := readCatalogKey(keyPath); err != nil || got != strings.Repeat("c", 64) {
		t.Fatal("valid private key rejected")
	}
	for _, value := range []string{"", strings.Repeat("g", 64), strings.Repeat("b", 63), strings.Repeat("b", 65), "Bearer " + strings.Repeat("b", 64), strings.Repeat("b", 64) + "\nextra"} {
		_ = os.WriteFile(keyPath, []byte(value), 0600)
		if _, err := readCatalogKey(keyPath); err == nil {
			t.Fatal("invalid reader key accepted")
		}
	}
	cfg := config{WalletDSNFile: filepath.Join(dir, "platform.dsn"), NewAPISocket: filepath.Join(dir, "native.sock"), WebDir: dir}
	lookup := func(k string) (string, bool) {
		if k == "MOMIAO_CATALOG_READER_KEY_FILE" {
			return keyPath, true
		}
		return "", false
	}
	if err := catalogConfig(&cfg, lookup); err != nil {
		t.Fatal(err)
	}
	if cfg.CatalogSyncInterval != 5*time.Minute || cfg.CatalogStaleAfter != 10*time.Minute || cfg.CatalogDisableAfter != 30*time.Minute {
		t.Fatal("wrong engineering defaults")
	}
	for name, values := range map[string]map[string]string{
		"relative_key":         {"MOMIAO_CATALOG_READER_KEY_FILE": "relative.key"},
		"bad_duration":         {"MOMIAO_CATALOG_READER_KEY_FILE": keyPath, "MOMIAO_CATALOG_SYNC_INTERVAL": "oops"},
		"stale_before_sync":    {"MOMIAO_CATALOG_READER_KEY_FILE": keyPath, "MOMIAO_CATALOG_STALE_AFTER": "1m"},
		"expired_before_stale": {"MOMIAO_CATALOG_READER_KEY_FILE": keyPath, "MOMIAO_CATALOG_DISABLE_AFTER": "5m"},
		"key_missing":          {"MOMIAO_CATALOG_SYNC_INTERVAL": "5m"},
		"api_userinfo":         {"MOMIAO_API_BASE_URL": "https://key@example.invalid/v1"},
		"api_query":            {"MOMIAO_API_BASE_URL": "https://api.example.invalid/v1?token=secret"},
		"api_path":             {"MOMIAO_API_BASE_URL": "https://api.example.invalid/admin"},
		"api_http":             {"MOMIAO_API_BASE_URL": "http://api.example.invalid/v1"},
	} {
		t.Run(name, func(t *testing.T) {
			c := cfg
			if catalogConfig(&c, func(k string) (string, bool) { v, ok := values[k]; return v, ok }) == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
	c := config{}
	if catalogConfig(&c, lookup) == nil {
		t.Fatal("reader without platform/transport accepted")
	}
	c = cfg
	if err := catalogConfig(&c, func(k string) (string, bool) {
		if k == "MOMIAO_API_BASE_URL" {
			return "https://api.example.invalid/v1", true
		}
		return "", false
	}); err != nil || c.APIBaseURL != "https://api.example.invalid/v1" {
		t.Fatal("explicit deployment base URL rejected")
	}
}
