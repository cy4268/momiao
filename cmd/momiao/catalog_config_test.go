package main

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type catalogRoundTrip func(*http.Request) (*http.Response, error)

func (f catalogRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCatalogAssetAndAPIConfig(t *testing.T) {
	for _, origin := range []string{"https://assets.example.test", "https://assets.example.test:8443"} {
		cfg, err := loadConfig(func(k string) (string, bool) {
			if k == "MOMIAO_ASSET_CDN_ORIGIN" {
				return origin, true
			}
			return "", false
		})
		if err != nil || cfg.AssetCDNOrigin != origin {
			t.Fatal("explicit CDN origin rejected", err)
		}
	}
	for _, origin := range []string{"", "http://assets.example.test", "https://*.example.test", "https://assets.example.test/", "https://assets.example.test/path", "https://user@assets.example.test", "https://assets.example.test?x", "https://assets.example.test#x", "https://assets.example.test;script-src *", "https://assets.example.test:0", "https://assets.example.test:65536"} {
		if _, err := loadConfig(func(k string) (string, bool) {
			if k == "MOMIAO_ASSET_CDN_ORIGIN" {
				return origin, true
			}
			return "", false
		}); err == nil {
			t.Fatal("invalid CDN origin accepted", origin)
		}
	}
	dir := t.TempDir()
	assetPath := filepath.Join(dir, "assets.json")
	values := map[string]string{"MOMIAO_CATALOG_ASSET_R2_ACCOUNT_ID": strings.Repeat("a", 32), "MOMIAO_CATALOG_ASSET_R2_BUCKET": "test-assets", "MOMIAO_CATALOG_ASSET_R2_CREDENTIALS_FILE": assetPath}
	assetLookup := func(k string) (string, bool) { v, ok := values[k]; return v, ok }
	if _, err := loadCatalogAssetConfig(func(string) (string, bool) { return "", false }); err != nil {
		t.Fatal("disabled upload rejected", err)
	}
	for _, data := range []string{`{"access_key_id":"test-access","secret_access_key":"test-secret","unknown":1}`, strings.Repeat("x", 4097), `{"access_key_id":"test-access"}`, `{"access_key_id":"test-access","access_key_id":"replaced","secret_access_key":"test-secret"}`} {
		_ = os.WriteFile(assetPath, []byte(data), 0600)
		if _, err := loadCatalogAssetConfig(assetLookup); err == nil {
			t.Fatal("unsafe asset credentials accepted")
		}
	}
	_ = os.WriteFile(assetPath, []byte(`{"access_key_id":"test-access","secret_access_key":"test-secret"}`), 0600)
	if _, err := loadCatalogAssetConfig(assetLookup); err != nil {
		t.Fatal("valid asset config rejected", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(assetPath, 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := loadCatalogAssetConfig(assetLookup); err == nil {
			t.Fatal("publicly readable R2 credentials accepted")
		}
		if err := os.Chmod(assetPath, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for key, bad := range map[string]string{"MOMIAO_CATALOG_ASSET_R2_ACCOUNT_ID": "../account", "MOMIAO_CATALOG_ASSET_R2_BUCKET": "", "MOMIAO_CATALOG_ASSET_R2_CREDENTIALS_FILE": dir} {
		old := values[key]
		values[key] = bad
		if _, err := loadCatalogAssetConfig(assetLookup); err == nil {
			t.Fatal("invalid asset config accepted", key)
		}
		values[key] = old
	}
	cfg := config{}
	for name, values := range map[string]map[string]string{
		"api_userinfo": {"MOMIAO_API_BASE_URL": "https://key@example.invalid/v1"},
		"api_query":    {"MOMIAO_API_BASE_URL": "https://api.example.invalid/v1?token=secret"},
		"api_path":     {"MOMIAO_API_BASE_URL": "https://api.example.invalid/admin"},
		"api_http":     {"MOMIAO_API_BASE_URL": "http://api.example.invalid/v1"},
	} {
		t.Run(name, func(t *testing.T) {
			c := cfg
			if catalogConfig(&c, func(k string) (string, bool) { v, ok := values[k]; return v, ok }) == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
	c := cfg
	if err := catalogConfig(&c, func(k string) (string, bool) {
		if k == "MOMIAO_API_BASE_URL" {
			return "https://api.example.invalid/v1", true
		}
		return "", false
	}); err != nil || c.APIBaseURL != "https://api.example.invalid/v1" {
		t.Fatal("explicit deployment base URL rejected")
	}
}
