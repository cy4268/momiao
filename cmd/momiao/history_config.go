package main

import (
	"errors"
	"path/filepath"
)

var errHistoryConfig = errors.New("history configuration is incomplete or invalid")

type historyConfig struct {
	Enabled                        bool
	ReaderDSNFile, WorkerDSNFile   string
	PokerDSNFile, PokerKeyringFile string
}

func loadHistoryConfig(lookup func(string) (string, bool)) (historyConfig, error) {
	var cfg historyConfig
	flag, flagSet := lookup("MOMIAO_HISTORY_ENABLED")
	reader, readerSet := lookup("MOMIAO_HISTORY_READER_DSN_FILE")
	worker, workerSet := lookup("MOMIAO_HISTORY_WORKER_DSN_FILE")
	pokerDSN, pokerSet := lookup("MOMIAO_HISTORY_POKER_DSN_FILE")
	keys, keysSet := lookup("MOMIAO_HISTORY_POKER_KEYRING_FILE")
	if (!flagSet || flag == "false") && !readerSet && !workerSet && !pokerSet && !keysSet {
		return cfg, nil
	}
	if flag != "true" || !readerSet || !workerSet || !pokerSet || !keysSet || !filepath.IsAbs(reader) || !filepath.IsAbs(worker) || !filepath.IsAbs(pokerDSN) || !filepath.IsAbs(keys) {
		return cfg, errHistoryConfig
	}
	cfg.ReaderDSNFile, cfg.WorkerDSNFile = filepath.Clean(reader), filepath.Clean(worker)
	cfg.PokerDSNFile, cfg.PokerKeyringFile = filepath.Clean(pokerDSN), filepath.Clean(keys)
	if cfg.ReaderDSNFile == cfg.WorkerDSNFile || cfg.ReaderDSNFile == cfg.PokerDSNFile || cfg.WorkerDSNFile == cfg.PokerDSNFile {
		return historyConfig{}, errHistoryConfig
	}
	cfg.Enabled = true
	return cfg, nil
}
