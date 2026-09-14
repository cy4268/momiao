package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
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

var errSessionConfig = errors.New("session requires complete, distinct, private authority configuration")

type sessionConfig struct {
	Enabled                                                           bool
	DSNFile, RedisConfigFile, ReaderKeyFile, OpsKeyFile               string
	SessionSealKeyFile, CredentialSealKeyFile, OpsSocket, Environment string
}

func loadSessionConfig(cfg *config, lookup func(string) (string, bool)) error {
	enabled, present := lookup("MOMIAO_SESSION_ENABLED")
	if present && enabled != "true" && enabled != "false" {
		return errSessionConfig
	}
	s := sessionConfig{Enabled: enabled == "true"}
	fields := []struct {
		name   string
		target *string
		path   bool
	}{
		{"MOMIAO_SESSION_DSN_FILE", &s.DSNFile, true},
		{"MOMIAO_SESSION_REDIS_CONFIG_FILE", &s.RedisConfigFile, true},
		{"MOMIAO_SESSION_READER_KEY_FILE", &s.ReaderKeyFile, true},
		{"MOMIAO_SESSION_OPS_KEY_FILE", &s.OpsKeyFile, true},
		{"MOMIAO_SESSION_SEAL_KEY_FILE", &s.SessionSealKeyFile, true},
		{"MOMIAO_SESSION_CREDENTIAL_SEAL_KEY_FILE", &s.CredentialSealKeyFile, true},
		{"MOMIAO_SESSION_OPS_SOCKET", &s.OpsSocket, true},
		{"MOMIAO_SESSION_ENVIRONMENT", &s.Environment, false},
	}
	seen := map[string]bool{}
	for _, value := range []string{cfg.WalletDSNFile, cfg.RegistrationReaderKeyFile, cfg.CatalogReaderKeyFile, cfg.Poker.DSNFile, cfg.Poker.ReaderKeyFile, cfg.Poker.StateKeyringFile, cfg.Poker.TicketKeyringFile, cfg.Poker.RedisConfigFile} {
		if value != "" {
			seen[pokerPathKey(value)] = true
		}
	}
	for _, field := range fields {
		value, set := lookup(field.name)
		if !s.Enabled {
			if set {
				return errSessionConfig
			}
			continue
		}
		if !set || value == "" {
			return errSessionConfig
		}
		if field.path {
			if !filepath.IsAbs(value) || seen[pokerPathKey(value)] {
				return errSessionConfig
			}
			seen[pokerPathKey(value)] = true
			value = filepath.Clean(value)
		}
		*field.target = value
	}
	if s.Enabled && (cfg.WebDir == "" || cfg.NewAPISocket == "" || cfg.PublicOrigin == "" || cfg.WalletDSNFile == "" || s.OpsSocket == cfg.NewAPISocket || !sessionName(s.Environment)) {
		return errSessionConfig
	}
	cfg.Session = s
	return nil
}

func sessionName(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for i, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || i > 0 && strings.ContainsRune("_.:-", r)) {
			return false
		}
	}
	return true
}

func readSessionKey(path string) ([32]byte, error) {
	var out [32]byte
	raw, err := readPokerPrivateFile(path, 66)
	if err != nil {
		return out, errSessionConfig
	}
	text := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	key, err := hex.DecodeString(text)
	if err != nil || len(key) != len(out) || hex.EncodeToString(key) != text {
		return out, errSessionConfig
	}
	copy(out[:], key)
	return out, nil
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
