package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigDefaults(t *testing.T) {
	cfg, err := loadConfig(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "127.0.0.1:8080" || cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestNativeQuotaConfigRequiresExplicitWallet(t *testing.T) {
	web := t.TempDir()
	if err := os.WriteFile(filepath.Join(web, "index.html"), []byte("portal"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path          string
		wallet, valid bool
	}{
		{"", true, false}, {"relative.dsn", true, false},
		{filepath.Join(t.TempDir(), "native.dsn"), false, false},
		{filepath.Join(t.TempDir(), "native.dsn"), true, true},
	} {
		env := map[string]string{"MOMIAO_WEB_DIR": web, "MOMIAO_NEWAPI_SOCKET": filepath.Join(t.TempDir(), "native.sock"), "MOMIAO_NATIVE_QUOTA_DSN_FILE": tc.path}
		if tc.wallet {
			env["MOMIAO_WALLET_DSN_FILE"] = filepath.Join(t.TempDir(), "wallet.dsn")
			env["MOMIAO_PUBLIC_ORIGIN"] = "https://wallet.example"
		}
		cfg, err := loadConfig(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
		if (err == nil) != tc.valid {
			t.Fatalf("valid=%v error=%v", tc.valid, err)
		}
		if err == nil && cfg.NativeQuotaDSNFile != tc.path {
			t.Fatal("native DSN not retained")
		}
	}
}

func TestConfigOverridesAndValidation(t *testing.T) {
	for _, tc := range []struct {
		name, key, value string
		valid            bool
	}{
		{"ipv4", "MOMIAO_LISTEN_ADDR", "127.0.0.1:8090", true},
		{"ipv6", "MOMIAO_LISTEN_ADDR", "[::1]:8090", true},
		{"explicit wildcard", "MOMIAO_LISTEN_ADDR", "0.0.0.0:8080", true},
		{"highest port", "MOMIAO_LISTEN_ADDR", "127.0.0.1:65535", true},
		{"empty address", "MOMIAO_LISTEN_ADDR", "", false},
		{"missing host", "MOMIAO_LISTEN_ADDR", ":8080", false},
		{"hostname", "MOMIAO_LISTEN_ADDR", "localhost:8080", false},
		{"missing port", "MOMIAO_LISTEN_ADDR", "127.0.0.1", false},
		{"zero port", "MOMIAO_LISTEN_ADDR", "127.0.0.1:0", false},
		{"negative port", "MOMIAO_LISTEN_ADDR", "127.0.0.1:-1", false},
		{"overflow port", "MOMIAO_LISTEN_ADDR", "127.0.0.1:65536", false},
		{"named port", "MOMIAO_LISTEN_ADDR", "127.0.0.1:http", false},
		{"invalid ip", "MOMIAO_LISTEN_ADDR", "999.0.0.1:8080", false},
		{"whitespace", "MOMIAO_LISTEN_ADDR", " 127.0.0.1:8080", false},
		{"sensitive input", "MOMIAO_LISTEN_ADDR", "private-test-value", false},
		{"minimum shutdown", "MOMIAO_SHUTDOWN_TIMEOUT", "1s", true},
		{"maximum shutdown", "MOMIAO_SHUTDOWN_TIMEOUT", "30s", true},
		{"empty shutdown", "MOMIAO_SHUTDOWN_TIMEOUT", "", false},
		{"zero shutdown", "MOMIAO_SHUTDOWN_TIMEOUT", "0s", false},
		{"negative shutdown", "MOMIAO_SHUTDOWN_TIMEOUT", "-1s", false},
		{"short shutdown", "MOMIAO_SHUTDOWN_TIMEOUT", "999ms", false},
		{"long shutdown", "MOMIAO_SHUTDOWN_TIMEOUT", "31s", false},
		{"unbounded shutdown", "MOMIAO_SHUTDOWN_TIMEOUT", "999999999999h", false},
		{"missing duration unit", "MOMIAO_SHUTDOWN_TIMEOUT", "10", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadConfig(func(key string) (string, bool) {
				return tc.value, key == tc.key
			})
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
			if err != nil {
				if !strings.Contains(err.Error(), tc.key) {
					t.Fatalf("error must identify configuration key: %v", err)
				}
				if strings.Contains(err.Error(), "private-test-value") {
					t.Fatal("configuration error exposed raw input")
				}
				return
			}
			if tc.key == "MOMIAO_LISTEN_ADDR" && cfg.ListenAddr != tc.value {
				t.Fatalf("address override was not applied: %q", cfg.ListenAddr)
			}
			if tc.key == "MOMIAO_SHUTDOWN_TIMEOUT" && cfg.ShutdownTimeout.String() != tc.value {
				t.Fatalf("shutdown override was not applied: %v", cfg.ShutdownTimeout)
			}
		})
	}
}

func TestPortalConfigAcceptanceAndRejection(t *testing.T) {
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("portal"), 0o600); err != nil {
		t.Fatal(err)
	}
	absSocket := filepath.Join(t.TempDir(), "portal.sock")
	upstreamSocket := filepath.Join(t.TempDir(), "newapi.sock")

	for _, tc := range []struct {
		name string
		env  map[string]string
		ok   bool
	}{
		{"web with fixed upstream", map[string]string{"MOMIAO_WEB_DIR": webDir, "MOMIAO_NEWAPI_SOCKET": upstreamSocket}, true},
		{"unix listener", map[string]string{"MOMIAO_LISTEN_SOCKET": absSocket}, true},
		{"complete production shape", map[string]string{"MOMIAO_WEB_DIR": webDir, "MOMIAO_LISTEN_SOCKET": absSocket, "MOMIAO_NEWAPI_SOCKET": upstreamSocket}, true},
		{"empty web", map[string]string{"MOMIAO_WEB_DIR": ""}, false},
		{"empty listener socket", map[string]string{"MOMIAO_LISTEN_SOCKET": ""}, false},
		{"empty upstream socket", map[string]string{"MOMIAO_NEWAPI_SOCKET": ""}, false},
		{"relative web", map[string]string{"MOMIAO_WEB_DIR": "web"}, false},
		{"missing web", map[string]string{"MOMIAO_WEB_DIR": filepath.Join(t.TempDir(), "missing"), "MOMIAO_NEWAPI_SOCKET": upstreamSocket}, false},
		{"web without index", map[string]string{"MOMIAO_WEB_DIR": t.TempDir(), "MOMIAO_NEWAPI_SOCKET": upstreamSocket}, false},
		{"relative listener socket", map[string]string{"MOMIAO_LISTEN_SOCKET": "portal.sock"}, false},
		{"relative upstream socket", map[string]string{"MOMIAO_NEWAPI_SOCKET": "newapi.sock"}, false},
		{"web without upstream", map[string]string{"MOMIAO_WEB_DIR": webDir}, false},
		{"upstream without web", map[string]string{"MOMIAO_NEWAPI_SOCKET": upstreamSocket}, false},
		{"tcp and unix listener", map[string]string{"MOMIAO_LISTEN_ADDR": "127.0.0.1:8090", "MOMIAO_LISTEN_SOCKET": absSocket}, false},
		{"same listener and upstream", map[string]string{"MOMIAO_WEB_DIR": webDir, "MOMIAO_LISTEN_SOCKET": absSocket, "MOMIAO_NEWAPI_SOCKET": absSocket}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadConfig(func(key string) (string, bool) {
				value, ok := tc.env[key]
				return value, ok
			})
			if (err == nil) != tc.ok {
				t.Fatalf("ok=%v, error=%v", tc.ok, err)
			}
			if err != nil {
				for _, value := range tc.env {
					if value != "" && strings.Contains(err.Error(), value) {
						t.Fatalf("configuration error exposed raw value: %v", err)
					}
				}
				return
			}
			if tc.env["MOMIAO_LISTEN_SOCKET"] != "" && cfg.ListenAddr != "" {
				t.Fatalf("unix listener retained TCP address: %+v", cfg)
			}
		})
	}
}
