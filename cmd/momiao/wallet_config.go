package main

import (
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func walletConfig(cfg *config, lookup func(string) (string, bool)) error {
	dsn, ds := lookup("MOMIAO_WALLET_DSN_FILE")
	origin, orig := lookup("MOMIAO_PUBLIC_ORIGIN")
	if !ds && !orig {
		return nil
	}
	if !ds || !orig || dsn == "" || origin == "" || cfg.WebDir == "" || cfg.NewAPISocket == "" {
		return errors.New("wallet configuration requires DSN file, public origin and complete portal")
	}
	if !filepath.IsAbs(dsn) {
		return errors.New("wallet DSN file must be an absolute path")
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.String() != origin || strings.ContainsAny(origin, "\r\n\t ") {
		return errors.New("wallet public origin must be an exact HTTPS origin")
	}
	cfg.WalletDSNFile = filepath.Clean(dsn)
	cfg.PublicOrigin = origin
	return nil
}
func readWalletDSN(path string) (string, error) {
	fail := errors.New("wallet DSN file is invalid or unreadable")
	if !filepath.IsAbs(path) {
		return "", fail
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fail
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 8192 {
		return "", fail
	}
	b, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil || len(b) > 8192 || strings.TrimSpace(string(b)) == "" {
		return "", fail
	}
	return strings.TrimSpace(string(b)), nil
}
