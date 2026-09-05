package main

import (
	"errors"
	"net"
	"strconv"
	"time"
)

type config struct {
	ListenAddr      string
	ShutdownTimeout time.Duration
}

func loadConfig(lookup func(string) (string, bool)) (config, error) {
	cfg := config{ListenAddr: "127.0.0.1:8080", ShutdownTimeout: 10 * time.Second}
	if value, ok := lookup("MOMIAO_LISTEN_ADDR"); ok {
		cfg.ListenAddr = value
	}
	host, port, err := net.SplitHostPort(cfg.ListenAddr)
	if err != nil || net.ParseIP(host) == nil {
		return config{}, errors.New("MOMIAO_LISTEN_ADDR must be a numeric IP and port (IPv6 in brackets)")
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber == 0 {
		return config{}, errors.New("MOMIAO_LISTEN_ADDR port must be in 1..65535")
	}
	if value, ok := lookup("MOMIAO_SHUTDOWN_TIMEOUT"); ok {
		cfg.ShutdownTimeout, err = time.ParseDuration(value)
		if err != nil || cfg.ShutdownTimeout < time.Second || cfg.ShutdownTimeout > 30*time.Second {
			return config{}, errors.New("MOMIAO_SHUTDOWN_TIMEOUT must be a duration in 1s..30s")
		}
	}
	return cfg, nil
}
