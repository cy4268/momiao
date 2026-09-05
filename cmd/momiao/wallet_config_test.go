package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWalletConfig(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("ok"), 0600)
	base := map[string]string{"MOMIAO_WEB_DIR": dir, "MOMIAO_NEWAPI_SOCKET": filepath.Join(dir, "native.sock")}
	for _, tc := range []struct {
		dsn, origin string
		valid       bool
	}{{"", "", true}, {"dsn", "", false}, {"", "https://wallet.example", false}, {filepath.Join(dir, "dsn"), "https://wallet.example", true}, {filepath.Join(dir, "dsn"), "http://wallet.example", false}, {filepath.Join(dir, "dsn"), "https://wallet.example/", false}, {filepath.Join(dir, "dsn"), "https://u:p@wallet.example", false}, {filepath.Join(dir, "dsn"), "https://wallet.example?x", false}, {"relative", "https://wallet.example", false}} {
		env := map[string]string{}
		for k, v := range base {
			env[k] = v
		}
		if tc.dsn != "" {
			env["MOMIAO_WALLET_DSN_FILE"] = tc.dsn
		}
		if tc.origin != "" {
			env["MOMIAO_PUBLIC_ORIGIN"] = tc.origin
		}
		c, err := loadConfig(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
		if (err == nil) != tc.valid {
			t.Fatalf("%+v %v", tc, err)
		}
		if tc.valid && c.WalletDSNFile != tc.dsn {
			t.Fatal(c.WalletDSNFile)
		}
	}
}
func TestReadWalletDSN(t *testing.T) {
	p := filepath.Join(t.TempDir(), "dsn")
	for _, v := range []string{"", " ", strings.Repeat("x", 8193), "host=localhost password=private\n"} {
		os.WriteFile(p, []byte(v), 0600)
		dsn, err := readWalletDSN(p)
		if strings.Contains(v, "host=") {
			if err != nil || dsn != strings.TrimSpace(v) {
				t.Fatal(err)
			}
		} else if err == nil {
			t.Fatal("accepted bad file")
		}
	}
	for _, p := range []string{"", "relative", t.TempDir()} {
		if _, err := readWalletDSN(p); err == nil {
			t.Fatal("accepted path")
		}
	}
}

func TestWalletConfigRequiresCompletePortal(t *testing.T) {
	for _, env := range []map[string]string{{"MOMIAO_WALLET_DSN_FILE": ""}, {"MOMIAO_PUBLIC_ORIGIN": ""}, {"MOMIAO_WALLET_DSN_FILE": filepath.Join(t.TempDir(), "dsn"), "MOMIAO_PUBLIC_ORIGIN": "https://wallet.example"}} {
		if _, err := loadConfig(func(k string) (string, bool) { v, ok := env[k]; return v, ok }); err == nil {
			t.Fatal("accepted incomplete configuration")
		}
	}
}
