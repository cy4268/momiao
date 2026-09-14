package connectticket

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"net"
	"time"

	"github.com/redis/go-redis/v9"
)

const RedisKeyPrefix = "chaldea:poker-ticket-used:"

// RedisConsumer uses a caller-owned official client. Configure MaxRetries=-1
// before redis.NewClient (its initialized Options reports zero retries), enable
// context timeouts and bound all I/O to <=2s. The caller retains Close ownership.
// It performs one SET NX; replay never extends expiry, and errors never fall
// back to process memory. Production ACL needs only HELLO/SET on this prefix.
func RedisConsumer(client *redis.Client) (ConsumeFunc, error) {
	if client == nil {
		return nil, ErrConfig
	}
	o := client.Options()
	if o == nil || o.MaxRetries != 0 || o.DialerRetries != 1 || !o.ContextTimeoutEnabled || o.DB != 0 {
		return nil, ErrConfig
	}
	for _, d := range []time.Duration{o.DialTimeout, o.ReadTimeout, o.WriteTimeout, o.PoolTimeout} {
		if d <= 0 || d > 2*time.Second {
			return nil, ErrConfig
		}
	}
	host, _, err := net.SplitHostPort(o.Addr)
	if err != nil {
		return nil, ErrConfig
	}
	ip := net.ParseIP(host)
	local := host == "localhost" || (ip != nil && ip.IsLoopback())
	if !local && (o.TLSConfig == nil || o.Username == "" || o.Password == "") {
		return nil, ErrConfig
	}
	if c := o.TLSConfig; c != nil && (c.InsecureSkipVerify || c.MinVersion < tls.VersionTLS12 || (c.MaxVersion != 0 && c.MaxVersion < c.MinVersion)) {
		return nil, ErrConfig
	}
	return func(ctx context.Context, hash [32]byte, expires time.Time) (bool, error) {
		if ctx == nil || hash == [32]byte{} {
			return false, ErrInvalid
		}
		now := time.Now()
		if !expires.After(now) {
			return false, ErrExpired
		}
		if expires.After(now.Add(TTL)) {
			return false, ErrInvalid
		}
		bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		// Fixed relative retention covers the full remaining ticket lifetime even
		// when the application and Redis clocks have different absolute offsets.
		ok, err := client.SetNX(bounded, RedisKeyPrefix+hex.EncodeToString(hash[:]), "1", TTL).Result()
		if err != nil || bounded.Err() != nil {
			return false, ErrUnavailable
		}
		return ok, nil
	}, nil
}
