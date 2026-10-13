package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/maintnotifications"
)

var errSessionConfig = errors.New("local sessions require Redis configuration, platform database and public origin")

type sessionConfig struct {
	Enabled         bool
	RedisConfigFile string
}

func loadSessionConfig(cfg *config, lookup func(string) (string, bool)) error {
	enabled, present := lookup("MOMIAO_SESSION_ENABLED")
	if present && enabled != "true" && enabled != "false" {
		return errSessionConfig
	}
	path, configured := lookup("MOMIAO_SESSION_REDIS_CONFIG_FILE")
	s := sessionConfig{Enabled: enabled == "true"}
	if s.Enabled {
		if !filepath.IsAbs(path) || cfg.PublicOrigin == "" || cfg.WalletDSNFile == "" {
			return errSessionConfig
		}
		s.RedisConfigFile = filepath.Clean(path)
	} else if configured {
		return errSessionConfig
	}
	cfg.Session = s
	return nil
}

func readSessionRedisConfig(path string) (*redis.Options, error) {
	raw, err := readPokerPrivateFile(path, 8192)
	if err != nil {
		return nil, errSessionConfig
	}
	var value struct {
		Addr       string `json:"addr"`
		Username   string `json:"username"`
		Password   string `json:"password"`
		CAFile     string `json:"ca_file"`
		ServerName string `json:"server_name"`
	}
	if decodePokerObject(raw, &value, "addr", "username", "password", "ca_file", "server_name") != nil {
		return nil, errSessionConfig
	}
	host, port, splitErr := net.SplitHostPort(value.Addr)
	number, portErr := strconv.ParseUint(port, 10, 16)
	if splitErr != nil || portErr != nil || number == 0 || host == "" || strings.ContainsAny(host, "/@?#\r\n\t ") || value.Username == "" || value.Password == "" || value.ServerName == "" || strings.ContainsAny(value.ServerName, "/@?#:\r\n\t ") || !filepath.IsAbs(value.CAFile) {
		return nil, errSessionConfig
	}
	file, err := os.Open(value.CAFile)
	if err != nil {
		return nil, errSessionConfig
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, errSessionConfig
	}
	pem, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(pem) > 1<<20 {
		return nil, errSessionConfig
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return nil, errSessionConfig
	}
	return &redis.Options{Addr: value.Addr, Username: value.Username, Password: value.Password, DB: 0, Protocol: 2, MaxRetries: -1, DialerRetries: 1, ContextTimeoutEnabled: true, DialTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second, PoolTimeout: 2 * time.Second, PoolSize: 4, MaxActiveConns: 4, MaxIdleConns: 4, DisableIdentity: true, MaintNotificationsConfig: &maintnotifications.Config{Mode: maintnotifications.ModeDisabled}, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: value.ServerName}}, nil
}
