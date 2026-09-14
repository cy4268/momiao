package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/nativeself"
	"github.com/cy4268/momiao/internal/poker"
	"github.com/cy4268/momiao/internal/poker/redislease"
	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/maintnotifications"
)

var errPokerConfig = errors.New("poker requires complete, distinct, private authority configuration")

type pokerConfig struct {
	Enabled                                                                      bool
	DSNFile, ReaderKeyFile, StateKeyringFile, TicketKeyringFile, RedisConfigFile string
	Password                                                                     *pokerPasswordConfig
}

func loadPokerConfig(cfg *config, lookup func(string) (string, bool)) error {
	enabled, present := lookup("MOMIAO_POKER_ENABLED")
	if present && enabled != "true" && enabled != "false" {
		return errPokerConfig
	}
	p := pokerConfig{Enabled: enabled == "true"}
	if cfg.PokerRemoteSocket != "" {
		if !p.Enabled || cfg.ProcessRole != "platform" || cfg.WalletDSNFile == "" || cfg.NewAPISocket == "" { return errPokerConfig }
		for _, name := range []string{"MOMIAO_POKER_DSN_FILE", "MOMIAO_POKER_STATE_KEYRING_FILE", "MOMIAO_POKER_REDIS_CONFIG_FILE"} {
			if _, present := lookup(name); present { return errPokerConfig }
		}
		password, err := readPokerPasswordConfig(lookup)
		if err != nil || password != nil { return errPokerConfig }
		reader, _ := lookup("MOMIAO_POKER_SESSION_READER_KEY_FILE")
		ticket, _ := lookup("MOMIAO_POKER_TICKET_KEYRING_FILE")
		if !filepath.IsAbs(reader) || !filepath.IsAbs(ticket) { return errPokerConfig }
		seen:=map[string]bool{}
		for _,path:=range []string{cfg.WalletDSNFile,cfg.PokerServiceKeyringFile,cfg.PokerPeerKeyringFile,reader,ticket}{if path==""||seen[pokerPathKey(path)]{return errPokerConfig};seen[pokerPathKey(path)]=true}
		p.ReaderKeyFile=filepath.Clean(reader);p.TicketKeyringFile=filepath.Clean(ticket)
		cfg.Poker = p
		return nil
	}
	password, err := readPokerPasswordConfig(lookup)
	if err != nil || !p.Enabled && password != nil {
		return errPokerConfig
	}
	p.Password = password
	paths := []struct {
		name   string
		target *string
	}{
		{"MOMIAO_POKER_DSN_FILE", &p.DSNFile},
		{"MOMIAO_POKER_SESSION_READER_KEY_FILE", &p.ReaderKeyFile},
		{"MOMIAO_POKER_STATE_KEYRING_FILE", &p.StateKeyringFile},
		{"MOMIAO_POKER_TICKET_KEYRING_FILE", &p.TicketKeyringFile},
		{"MOMIAO_POKER_REDIS_CONFIG_FILE", &p.RedisConfigFile},
	}
	seen := map[string]bool{}
	for _, value := range []string{cfg.WalletDSNFile, cfg.GameFairnessKeyringFile, cfg.RegistrationReaderKeyFile, cfg.CatalogReaderKeyFile, cfg.PokerServiceKeyringFile, cfg.PokerPeerKeyringFile} {
		if value != "" {
			seen[pokerPathKey(value)] = true
		}
	}
	for _, field := range paths {
		value, set := lookup(field.name)
		if !p.Enabled {
			if set {
				return errPokerConfig
			}
			continue
		}
		if !set || !filepath.IsAbs(value) || seen[pokerPathKey(value)] {
			return errPokerConfig
		}
		seen[pokerPathKey(value)] = true
		*field.target = filepath.Clean(value)
	}
	if p.Enabled && (cfg.WebDir == "" && cfg.ProcessRole != "poker" || cfg.NewAPISocket == "" || cfg.PublicOrigin == "" || cfg.WalletDSNFile == "") {
		return errPokerConfig
	}
	cfg.Poker = p
	return nil
}

func pokerPathKey(value string) string {
	value = filepath.Clean(value)
	if runtime.GOOS == "windows" {
		value = strings.ToLower(value)
	}
	return value
}

// POSIX production checks private mode bits. Windows fixture operators must
// enforce the containing directory's ACL; mode bits do not establish a DACL.
func readPokerPrivateFile(path string, limit int64) ([]byte, error) {
	if !filepath.IsAbs(path) {
		return nil, errPokerConfig
	}
	before, e := os.Lstat(path)
	if e != nil || !before.Mode().IsRegular() || before.Size() > limit || runtime.GOOS != "windows" && before.Mode().Perm()&0077 != 0 {
		return nil, errPokerConfig
	}
	resolved, e := filepath.EvalSymlinks(path)
	if e != nil || pokerPathKey(resolved) != pokerPathKey(path) {
		return nil, errPokerConfig
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, errPokerConfig
	}
	defer f.Close()
	after, e := f.Stat()
	if e != nil || !os.SameFile(before, after) || !after.Mode().IsRegular() || after.Size() > limit || runtime.GOOS != "windows" && after.Mode().Perm()&0077 != 0 {
		return nil, errPokerConfig
	}
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil || int64(len(b)) > limit || len(b) == 0 {
		return nil, errPokerConfig
	}
	return b, nil
}

func decodePokerObject(raw []byte, out any, fields ...string) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	if !nativeself.UniqueJSON(d, 0) {
		return errPokerConfig
	}
	if _, e := d.Token(); e != io.EOF {
		return errPokerConfig
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || len(object) != len(fields) {
		return errPokerConfig
	}
	for _, field := range fields {
		if v, ok := object[field]; !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return errPokerConfig
		}
	}
	if json.Unmarshal(raw, out) != nil {
		return errPokerConfig
	}
	return nil
}

func pokerKeyID(id string) bool {
	if len(id) < 1 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func canonicalKey(raw string, length int) ([]byte, error) {
	b, e := hex.DecodeString(raw)
	if e != nil || len(b) != length || hex.EncodeToString(b) != raw {
		return nil, errPokerConfig
	}
	return b, nil
}

func readPokerStateKeys(path string) (poker.Keyring, error) {
	raw, e := readPokerPrivateFile(path, 8192)
	if e != nil {
		return poker.Keyring{}, e
	}
	var v struct {
		Active string            `json:"active"`
		Keys   map[string]string `json:"keys"`
	}
	if decodePokerObject(raw, &v, "active", "keys") != nil || !pokerKeyID(v.Active) || len(v.Keys) < 1 || len(v.Keys) > 16 {
		return poker.Keyring{}, errPokerConfig
	}
	out := poker.Keyring{Current: v.Active, Keys: map[string][]byte{}}
	for id, value := range v.Keys {
		b, e := canonicalKey(value, 32)
		if e != nil || !pokerKeyID(id) {
			return poker.Keyring{}, errPokerConfig
		}
		out.Keys[id] = b
	}
	if len(out.Keys[out.Current]) != 32 {
		return poker.Keyring{}, errPokerConfig
	}
	return out, nil
}

type pokerTicketKeys struct {
	active  string
	private ed25519.PrivateKey
	public  map[string]ed25519.PublicKey
}

func (pokerTicketKeys) String() string   { return "pokerTicketKeys<redacted>" }
func (pokerTicketKeys) GoString() string { return "pokerTicketKeys<redacted>" }

func readPokerTicketKeys(path string) (pokerTicketKeys, error) {
	raw, e := readPokerPrivateFile(path, 8192)
	if e != nil {
		return pokerTicketKeys{}, e
	}
	var v struct {
		Active  string            `json:"active"`
		Private string            `json:"private_key"`
		Public  map[string]string `json:"public_keys"`
	}
	if decodePokerObject(raw, &v, "active", "private_key", "public_keys") != nil || !pokerKeyID(v.Active) || len(v.Public) < 1 || len(v.Public) > 16 {
		return pokerTicketKeys{}, errPokerConfig
	}
	private, e := canonicalKey(v.Private, ed25519.PrivateKeySize)
	if e != nil {
		return pokerTicketKeys{}, e
	}
	if !bytes.Equal(ed25519.NewKeyFromSeed(private[:ed25519.SeedSize]), private) {
		return pokerTicketKeys{}, errPokerConfig
	}
	out := pokerTicketKeys{active: v.Active, private: ed25519.PrivateKey(private), public: map[string]ed25519.PublicKey{}}
	for id, value := range v.Public {
		b, e := canonicalKey(value, ed25519.PublicKeySize)
		if e != nil || !pokerKeyID(id) {
			return pokerTicketKeys{}, errPokerConfig
		}
		out.public[id] = ed25519.PublicKey(b)
	}
	if !bytes.Equal(out.public[out.active], out.private.Public().(ed25519.PublicKey)) {
		return pokerTicketKeys{}, errPokerConfig
	}
	return out, nil
}

func readPokerReaderKey(path string) (string, error) {
	raw, e := readPokerPrivateFile(path, 66)
	if e != nil {
		return "", e
	}
	key := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	if _, e = canonicalKey(key, 32); e != nil {
		return "", e
	}
	return key, nil
}

// One private configuration file supplies distinct lease and ticket ACL users.
// Both use verified TLS, bounded official clients and no automatic retries.
func readPokerRedisConfig(path string) (redislease.Config, *redis.Options, error) {
	raw, e := readPokerPrivateFile(path, 8192)
	if e != nil {
		return redislease.Config{}, nil, e
	}
	var v struct {
		Addr           string `json:"addr"`
		Username       string `json:"username"`
		Password       string `json:"password"`
		TicketUsername string `json:"ticket_username"`
		TicketPassword string `json:"ticket_password"`
		CAFile         string `json:"ca_file"`
		ServerName     string `json:"server_name"`
	}
	if decodePokerObject(raw, &v, "addr", "username", "password", "ticket_username", "ticket_password", "ca_file", "server_name") != nil {
		return redislease.Config{}, nil, errPokerConfig
	}
	host, port, e := net.SplitHostPort(v.Addr)
	n, pe := strconv.ParseUint(port, 10, 16)
	if e != nil || pe != nil || n == 0 || host == "" || strings.ContainsAny(host, "/@?#\r\n\t ") || v.Username == "" || v.Password == "" || v.TicketUsername == "" || v.TicketPassword == "" || v.Username == v.TicketUsername || v.ServerName == "" || strings.ContainsAny(v.ServerName, "/@?#:\r\n\t ") || !filepath.IsAbs(v.CAFile) {
		return redislease.Config{}, nil, errPokerConfig
	}
	f, e := os.Open(v.CAFile)
	if e != nil {
		return redislease.Config{}, nil, errPokerConfig
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return redislease.Config{}, nil, errPokerConfig
	}
	pem, e := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if e != nil || len(pem) > 1<<20 {
		return redislease.Config{}, nil, errPokerConfig
	}
	ca := x509.NewCertPool()
	if !ca.AppendCertsFromPEM(pem) {
		return redislease.Config{}, nil, errPokerConfig
	}
	secure := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: ca, ServerName: v.ServerName}
	lease := redislease.Config{Addr: v.Addr, Username: v.Username, Password: v.Password, TLSConfig: secure, Timeout: 2 * time.Second}
	tickets := &redis.Options{Addr: v.Addr, Username: v.TicketUsername, Password: v.TicketPassword, TLSConfig: secure.Clone(), DB: 0, Protocol: 2, MaxRetries: -1, DialerRetries: 1, ContextTimeoutEnabled: true, DialTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second, PoolTimeout: 2 * time.Second, PoolSize: 4, MaxActiveConns: 4, MaxIdleConns: 4, DisableIdentity: true, MaintNotificationsConfig: &maintnotifications.Config{Mode: maintnotifications.ModeDisabled}}
	return lease, tickets, nil
}
