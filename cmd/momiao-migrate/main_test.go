package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationInputFailsClosed(t *testing.T) {
	for _, v := range []string{"", "relative", filepath.Join(t.TempDir(), "missing-secret")} {
		var out bytes.Buffer
		if code := runMigration(context.Background(), func(string) string { return v }, &out); code != 1 || out.String() != "migration failed\n" {
			t.Fatal(code, out.String())
		}
	}
	p := filepath.Join(t.TempDir(), "dsn")
	for _, v := range []string{"", strings.Repeat("x", 8193), "postgres://private:secret@127.0.0.1:1/database?connect_timeout=1"} {
		os.WriteFile(p, []byte(v), 0600)
		var out bytes.Buffer
		if code := runMigration(context.Background(), func(k string) string {
			if k != "MOMIAO_MIGRATION_DSN_FILE" {
				t.Fatal(k)
			}
			return p
		}, &out); code != 1 || out.String() != "migration failed\n" {
			t.Fatal(code, out.String())
		}
	}
}
