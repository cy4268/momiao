package main

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/platform"
)

func catalogConfig(cfg *config, lookup func(string) (string, bool)) error {
	cfg.CatalogSyncInterval = 5 * time.Minute
	cfg.CatalogStaleAfter = 10 * time.Minute
	cfg.CatalogDisableAfter = 30 * time.Minute
	key, keySet := lookup("MOMIAO_CATALOG_READER_KEY_FILE")
	if keySet {
		if !filepath.IsAbs(key) || cfg.WalletDSNFile == "" || cfg.WebDir == "" || cfg.NewAPISocket == "" {
			return errors.New("catalog reader requires an absolute key file and complete platform configuration")
		}
		cfg.CatalogReaderKeyFile = filepath.Clean(key)
	}
	for name, destination := range map[string]*time.Duration{
		"MOMIAO_CATALOG_SYNC_INTERVAL": &cfg.CatalogSyncInterval,
		"MOMIAO_CATALOG_STALE_AFTER":   &cfg.CatalogStaleAfter,
		"MOMIAO_CATALOG_DISABLE_AFTER": &cfg.CatalogDisableAfter,
	} {
		if value, ok := lookup(name); ok {
			duration, err := time.ParseDuration(value)
			if !keySet || err != nil || duration < time.Minute || duration > 24*time.Hour {
				return errors.New("catalog durations require a reader key and values in 1m..24h")
			}
			*destination = duration
		}
	}
	if cfg.CatalogStaleAfter < cfg.CatalogSyncInterval || cfg.CatalogDisableAfter <= cfg.CatalogStaleAfter {
		return errors.New("catalog freshness thresholds must follow sync interval < expiry")
	}
	if value, ok := lookup("MOMIAO_API_BASE_URL"); ok {
		u, err := url.Parse(value)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Path != "/v1" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.String() != value || strings.ContainsAny(value, "\r\n\t ") {
			return errors.New("API base URL must be an explicit HTTPS deployment URL ending in /v1")
		}
		cfg.APIBaseURL = value
	}
	return nil
}
func readCatalogKey(path string) (string, error) {
	fail := errors.New("catalog reader key file is invalid or unreadable")
	if !filepath.IsAbs(path) {
		return "", fail
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 128 || runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return "", fail
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fail
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 129))
	if err != nil || len(raw) > 128 {
		return "", fail
	}
	key := strings.TrimSpace(string(raw))
	decoded, err := hex.DecodeString(key)
	if err != nil || len(key) != 64 || len(decoded) != 32 {
		return "", fail
	}
	return key, nil
}

type nativeCatalogReader struct {
	transport http.RoundTripper
	key       string
}

func (reader nativeCatalogReader) Read(parent context.Context) (platform.NativeCatalog, error) {
	fail := errors.New("CATALOG_READ_FAILED")
	if reader.transport == nil || len(reader.key) != 64 {
		return platform.NativeCatalog{}, fail
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/internal/momiao/catalog", nil)
	if err != nil {
		return platform.NativeCatalog{}, fail
	}
	request.Header.Set("Authorization", "Bearer "+reader.key)
	request.Header.Set("Accept", "application/json")
	client := http.Client{Transport: reader.transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return platform.NativeCatalog{}, fail
	}
	defer response.Body.Close()
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.StatusCode != http.StatusOK || err != nil || media != "application/json" {
		return platform.NativeCatalog{}, fail
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, platform.NativeCatalogMaxBytes+1))
	if err != nil || ctx.Err() != nil {
		return platform.NativeCatalog{}, fail
	}
	return platform.ParseNativeCatalog(raw, time.Now())
}
