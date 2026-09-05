// momiao-migrate is an explicit administration command, never a portal startup mode.
package main

import (
	"context"
	"fmt"
	"github.com/cy4268/momiao/internal/platform"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "migration failed")
		os.Exit(1)
	}
	os.Exit(runMigration(context.Background(), os.Getenv, os.Stdout))
}
func runMigration(ctx context.Context, getenv func(string) string, out io.Writer) int {
	if migrate(ctx, getenv("MOMIAO_MIGRATION_DSN_FILE")) {
		fmt.Fprintln(out, "migration succeeded")
		return 0
	}
	fmt.Fprintln(out, "migration failed")
	return 1
}
func migrate(ctx context.Context, path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 8192 {
		return false
	}
	b, err := io.ReadAll(io.LimitReader(f, 8193))
	dsn := strings.TrimSpace(string(b))
	if err != nil || len(b) > 8192 || dsn == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	store, err := platform.Open(ctx, dsn)
	if err != nil {
		return false
	}
	defer store.Close()
	return store.Migrate(ctx) == nil
}
