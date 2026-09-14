package main

import (
	"context"
	"path/filepath"
	"testing"
)

func TestHistoryRuntimeConfiguration(t *testing.T) {
	env := map[string]string{}
	lookup := func(key string) (string, bool) { value, ok := env[key]; return value, ok }
	cfg, err := loadHistoryConfig(lookup)
	if err != nil || cfg.Enabled {
		t.Fatal("history must remain opt-in")
	}
	env["MOMIAO_HISTORY_ENABLED"] = "true"
	if _, err = loadHistoryConfig(lookup); err == nil {
		t.Fatal("incomplete role split accepted")
	}
	dir := t.TempDir()
	env["MOMIAO_HISTORY_READER_DSN_FILE"] = filepath.Join(dir, "reader")
	env["MOMIAO_HISTORY_WORKER_DSN_FILE"] = filepath.Join(dir, "worker")
	env["MOMIAO_HISTORY_POKER_DSN_FILE"] = filepath.Join(dir, "poker-reader")
	env["MOMIAO_HISTORY_POKER_KEYRING_FILE"] = filepath.Join(dir, "poker-state-keyring")
	if cfg, err = loadHistoryConfig(lookup); err != nil || !cfg.Enabled {
		t.Fatal("distinct absolute file configuration rejected")
	}
	env["MOMIAO_HISTORY_WORKER_DSN_FILE"] = env["MOMIAO_HISTORY_READER_DSN_FILE"]
	if _, err = loadHistoryConfig(lookup); err == nil {
		t.Fatal("same credential path accepted")
	}
}

func TestHistoryRuntimeMissingDependencies(t *testing.T) {
	app, err := openHistoryApplication(context.Background(), historyConfig{}, nil, nil, nil, nil)
	if err != nil || app != nil {
		t.Fatal("disabled feature opened dependencies")
	}
	if _, err = openHistoryApplication(context.Background(), historyConfig{Enabled: true}, nil, nil, nil, nil); err == nil {
		t.Fatal("incomplete identity/domain services accepted")
	}
	if validateHistoryPools(context.Background(), nil, nil, nil) == nil {
		t.Fatal("missing dedicated pools accepted")
	}
	var empty *historyApplication
	empty.Close()
}
