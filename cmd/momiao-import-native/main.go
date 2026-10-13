// momiao-import-native is an explicit migration tool, never a server startup hook.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "native data import failed")
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	stage := flag.String("stage", "identity", "identity, keys or workspace")
	flag.Parse()
	// Files keep connection credentials out of command arguments and logs.
	sourceDSN, err := os.ReadFile(os.Getenv("MOMIAO_IMPORT_NATIVE_DSN_FILE"))
	if err != nil {
		return err
	}
	targetDSN, err := os.ReadFile(os.Getenv("MOMIAO_MIGRATION_DSN_FILE"))
	if err != nil {
		return err
	}
	source, err := pgx.Connect(ctx, strings.TrimSpace(string(sourceDSN)))
	if err != nil {
		return fmt.Errorf("connect source: %w", err)
	}
	defer source.Close(ctx)
	target, err := platform.Open(ctx, strings.TrimSpace(string(targetDSN)))
	if err != nil {
		return fmt.Errorf("connect target: %w", err)
	}
	defer target.Close()
	// Schema migration is deliberately a separate, explicit command.
	if *stage == "workspace" {
		counts, err := target.ImportNativeWorkspace(ctx, source)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"stage": "workspace", "counts": counts})
	}
	if *stage == "keys" {
		count, err := target.ImportNativeKeys(ctx, source)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"stage": "keys", "api_keys": count})
	}
	if *stage != "identity" {
		return fmt.Errorf("unknown import stage")
	}
	result, err := target.ImportNativeIdentity(ctx, source)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Stage string `json:"stage"`
		platform.IdentityImportResult
	}{"identity", result})
}
