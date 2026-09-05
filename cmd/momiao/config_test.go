package main

import (
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
