// Package redislease implements the production Redis authority for IS-06 §314
// seat reservations. PostgreSQL owns audit/idempotency and wallet transactions;
// an audit row alone never grants a lease. No poker parent-package dependency.
package redislease

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/maintnotifications"
)

const KeyPrefix = "chaldea:poker:seat-reservation:"
const LeaseTTL = 30 * time.Second

var (
	ErrInvalidConfig = errors.New("POKER_REDIS_LEASE_INVALID_CONFIG")
	ErrInvalidInput  = errors.New("POKER_REDIS_LEASE_INVALID_INPUT")
	ErrUnavailable   = errors.New("POKER_REDIS_LEASE_UNAVAILABLE")
	ErrNotOwned      = errors.New("POKER_REDIS_LEASE_NOT_OWNED")
)

// Config takes credentials separately from Addr so no credential-bearing URL is
// needed. Non-loopback connections require verified TLS and explicit ACL auth.
// Timeout defaults to 2s and is limited to 5s, always below the fixed lease TTL.
type Config struct {
	Addr      string        `json:"-"`
	Username  string        `json:"-"`
	Password  string        `json:"-"`
	TLSConfig *tls.Config   `json:"-"`
	Timeout   time.Duration `json:"-"`
}

func (Config) String() string   { return "redislease.Config<redacted>" }
func (Config) GoString() string { return "redislease.Config<redacted>" }

// evalClient is the small SDK boundary used by scoped unit tests. Open always
// supplies the official *redis.Client; no custom pool, RESP or local authority.
type evalClient interface {
	Eval(context.Context, string, []string, ...any) *redis.Cmd
	Close() error
}
type Store struct {
	client  evalClient
	timeout time.Duration
}

func (*Store) String() string   { return "redislease.Store<redacted>" }
func (*Store) GoString() string { return "redislease.Store<redacted>" }

func clientOptions(c Config) (*redis.Options, error) {
	host, port, e := net.SplitHostPort(c.Addr)
	p, pe := strconv.Atoi(port)
	if e != nil || pe != nil || p < 1 || p > 65535 || host == "" || strings.TrimSpace(host) != host || strings.ContainsAny(host, "/@?#\r\n\t") {
		return nil, ErrInvalidConfig
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	if timeout <= 0 || timeout > 5*time.Second {
		return nil, ErrInvalidConfig
	}
	loopback := host == "localhost"
	if ip := net.ParseIP(host); ip != nil {
		loopback = ip.IsLoopback()
	}
	if !loopback && (c.TLSConfig == nil || c.Username == "" || c.Password == "") {
		return nil, ErrInvalidConfig
	}
	if (c.Username == "") != (c.Password == "") {
		return nil, ErrInvalidConfig
	}
	var secure *tls.Config
	if c.TLSConfig != nil {
		secure = c.TLSConfig.Clone()
		if secure.MinVersion == 0 {
			secure.MinVersion = tls.VersionTLS12
		}
		if secure.InsecureSkipVerify || secure.MinVersion < tls.VersionTLS12 || (secure.MaxVersion != 0 && secure.MaxVersion < secure.MinVersion) {
			return nil, ErrInvalidConfig
		}
	}
	return &redis.Options{Addr: c.Addr, Username: c.Username, Password: c.Password, TLSConfig: secure, DB: 0,
		Protocol: 2, MaxRetries: -1, DialerRetries: 1, ContextTimeoutEnabled: true, DialTimeout: timeout, ReadTimeout: timeout, WriteTimeout: timeout, PoolTimeout: timeout,
		PoolSize: 8, MaxActiveConns: 8, MaxIdleConns: 8, MinIdleConns: 0, ConnMaxIdleTime: time.Minute,
		DisableIdentity: true, MaintNotificationsConfig: &maintnotifications.Config{Mode: maintnotifications.ModeDisabled}}, nil
}

// Open creates its own bounded official client and proves connectivity with PING.
// It never changes Redis configuration, ACLs, keys, logging or global client hooks.
func Open(ctx context.Context, c Config) (*Store, error) {
	if ctx == nil {
		return nil, ErrInvalidInput
	}
	o, e := clientOptions(c)
	if e != nil {
		return nil, e
	}
	client := redis.NewClient(o)
	s := &Store{client: client, timeout: o.ReadTimeout}
	bounded, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	if e = client.Ping(bounded).Err(); e != nil {
		client.Close()
		return nil, unavailable(bounded)
	}
	if e = bounded.Err(); e != nil {
		client.Close()
		return nil, unavailable(bounded)
	}
	return s, nil
}
func (s *Store) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	if e := s.client.Close(); e != nil {
		return ErrUnavailable
	}
	return nil
}

// The three scripts are single-key atomic operations. Redis TIME, not the app
// clock, caps a new TTL at min(30s, PG expiresAt - Redis now). Same-token retries
// do not SET/EXPIRE an existing key; altered/absent TTL never passes validation.
const acquireScript = `
local now = redis.call('TIME')
local remaining = tonumber(ARGV[2]) - (tonumber(now[1])*1000 + math.floor(tonumber(now[2])/1000))
if remaining <= 0 then return 0 end
local current = redis.call('GET', KEYS[1])
if current then
  if current ~= ARGV[1] then return 0 end
  local ttl = redis.call('PTTL', KEYS[1])
  if ttl > 0 and ttl <= 30000 then return 1 end
  return 0
end
local ttl = math.min(30000, remaining)
if redis.call('SET', KEYS[1], ARGV[1], 'NX', 'PX', ttl) then return 1 end
return 0
`
const validScript = `
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
local ttl = redis.call('PTTL', KEYS[1])
if ttl > 0 and ttl <= 30000 then return 1 end
return 0
`
const releaseScript = `
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`

// Acquire returns false for contention, past expiry or invalid existing TTL.
// Unknown network outcomes are errors: do not debit or presume acquisition.
func (s *Store) Acquire(ctx context.Context, tableID string, seatNo int, token string, expiresAt time.Time) (bool, error) {
	key, e := leaseKey(tableID, seatNo, token)
	if e != nil {
		return false, e
	}
	if !validPGTime(expiresAt) {
		return false, ErrInvalidInput
	}
	return s.eval(ctx, acquireScript, key, token, expiresAt.UnixMilli())
}

// Valid is a point-in-time Redis authority check, never a cached grant or renewed
// TTL. pgNow preserves LeaseStore compatibility; the caller separately checks its
// durable expiry with that PG time. Neither time is used to invent a missing key.
func (s *Store) Valid(ctx context.Context, tableID string, seatNo int, token string, pgNow time.Time) (bool, error) {
	key, e := leaseKey(tableID, seatNo, token)
	if e != nil {
		return false, e
	}
	if !validPGTime(pgNow) {
		return false, ErrInvalidInput
	}
	return s.eval(ctx, validScript, key, token)
}

// Release only deletes the matching owner. Missing/mismatched ownership returns
// ErrNotOwned; post-commit cleanup can treat it as a benign already-lost lease.
func (s *Store) Release(ctx context.Context, tableID string, seatNo int, token string) error {
	key, e := leaseKey(tableID, seatNo, token)
	if e != nil {
		return e
	}
	owned, e := s.eval(ctx, releaseScript, key, token)
	if e != nil {
		return e
	}
	if !owned {
		return ErrNotOwned
	}
	return nil
}
func (s *Store) eval(ctx context.Context, script, key string, args ...any) (bool, error) {
	if ctx == nil {
		return false, ErrInvalidInput
	}
	if s == nil || s.client == nil || s.timeout <= 0 {
		return false, ErrUnavailable
	}
	if ctx.Err() != nil {
		return false, unavailable(ctx)
	}
	bounded, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	value, e := s.client.Eval(bounded, script, []string{key}, args...).Result()
	if e != nil || bounded.Err() != nil {
		return false, unavailable(bounded)
	}
	result, ok := value.(int64)
	if !ok || (result != 0 && result != 1) {
		return false, ErrUnavailable
	}
	return result == 1, nil
}
func unavailable(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return errors.Join(ErrUnavailable, e)
	}
	return ErrUnavailable
}
func validPGTime(t time.Time) bool { return !t.IsZero() && t.Year() >= 2000 && t.Year() <= 9999 }
func leaseKey(tableID string, seatNo int, token string) (string, error) {
	if len(tableID) != 36 || seatNo < 1 || seatNo > 9 || len(token) != 64 {
		return "", ErrInvalidInput
	}
	for i, c := range tableID {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return "", ErrInvalidInput
			}
		} else if !lowerHex(c) {
			return "", ErrInvalidInput
		}
	}
	for _, c := range token {
		if !lowerHex(c) {
			return "", ErrInvalidInput
		}
	}
	return KeyPrefix + tableID + ":" + strconv.Itoa(seatNo), nil
}
func lowerHex(c rune) bool { return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' }
