package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type config struct {
	WalletDSNFile   string
	PublicOrigin    string
	wallet          walletStore
	ListenAddr      string
	ListenSocket    string
	WebDir          string
	NewAPISocket    string
	ShutdownTimeout time.Duration
}

func loadConfig(lookup func(string) (string, bool)) (config, error) {
	cfg := config{ListenAddr: "127.0.0.1:8080", ShutdownTimeout: 10 * time.Second}
	listenAddr, listenAddrSet := lookup("MOMIAO_LISTEN_ADDR")
	if listenAddrSet {
		if listenAddr == "" {
			return config{}, errors.New("MOMIAO_LISTEN_ADDR must not be empty")
		}
		cfg.ListenAddr = listenAddr
	}
	if value, ok := lookup("MOMIAO_LISTEN_SOCKET"); ok {
		if value == "" {
			return config{}, errors.New("MOMIAO_LISTEN_SOCKET must not be empty")
		}
		if !filepath.IsAbs(value) {
			return config{}, errors.New("MOMIAO_LISTEN_SOCKET must be an absolute path")
		}
		if listenAddrSet {
			return config{}, errors.New("MOMIAO_LISTEN_ADDR and MOMIAO_LISTEN_SOCKET are mutually exclusive")
		}
		cfg.ListenAddr = ""
		cfg.ListenSocket = filepath.Clean(value)
	}
	if cfg.ListenAddr != "" {
		host, port, err := net.SplitHostPort(cfg.ListenAddr)
		if err != nil || net.ParseIP(host) == nil {
			return config{}, errors.New("MOMIAO_LISTEN_ADDR must be a numeric IP and port (IPv6 in brackets)")
		}
		portNumber, err := strconv.ParseUint(port, 10, 16)
		if err != nil || portNumber == 0 {
			return config{}, errors.New("MOMIAO_LISTEN_ADDR port must be in 1..65535")
		}
	}
	if value, ok := lookup("MOMIAO_WEB_DIR"); ok {
		if value == "" {
			return config{}, errors.New("MOMIAO_WEB_DIR must not be empty")
		}
		if !filepath.IsAbs(value) {
			return config{}, errors.New("MOMIAO_WEB_DIR must be an absolute path")
		}
		resolved, err := filepath.EvalSymlinks(value)
		if err != nil {
			return config{}, errors.New("MOMIAO_WEB_DIR must be an existing directory")
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() {
			return config{}, errors.New("MOMIAO_WEB_DIR must be an existing directory")
		}
		index, err := os.Stat(filepath.Join(resolved, "index.html"))
		if err != nil || !index.Mode().IsRegular() {
			return config{}, errors.New("MOMIAO_WEB_DIR must contain a regular index.html")
		}
		cfg.WebDir = resolved
	}
	if value, ok := lookup("MOMIAO_NEWAPI_SOCKET"); ok {
		if value == "" {
			return config{}, errors.New("MOMIAO_NEWAPI_SOCKET must not be empty")
		}
		if !filepath.IsAbs(value) {
			return config{}, errors.New("MOMIAO_NEWAPI_SOCKET must be an absolute path")
		}
		cfg.NewAPISocket = filepath.Clean(value)
	}
	if cfg.WebDir != "" && cfg.NewAPISocket == "" {
		return config{}, errors.New("MOMIAO_NEWAPI_SOCKET is required when MOMIAO_WEB_DIR is set")
	}
	if cfg.WebDir == "" && cfg.NewAPISocket != "" {
		return config{}, errors.New("MOMIAO_NEWAPI_SOCKET requires MOMIAO_WEB_DIR")
	}
	if cfg.ListenSocket != "" && cfg.ListenSocket == cfg.NewAPISocket {
		return config{}, errors.New("MOMIAO_LISTEN_SOCKET and MOMIAO_NEWAPI_SOCKET must differ")
	}
	if value, ok := lookup("MOMIAO_SHUTDOWN_TIMEOUT"); ok {
		if value == "" {
			return config{}, errors.New("MOMIAO_SHUTDOWN_TIMEOUT must not be empty")
		}
		var err error
		cfg.ShutdownTimeout, err = time.ParseDuration(value)
		if err != nil || cfg.ShutdownTimeout < time.Second || cfg.ShutdownTimeout > 30*time.Second {
			return config{}, errors.New("MOMIAO_SHUTDOWN_TIMEOUT must be a duration in 1s..30s")
		}
	}
	if err := walletConfig(&cfg, lookup); err != nil {
		return config{}, err
	}
	return cfg, nil
}
