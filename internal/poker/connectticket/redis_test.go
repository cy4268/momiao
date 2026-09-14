package connectticket

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func boundedRedisOptions() *redis.Options {
	return &redis.Options{Addr: "127.0.0.1:16380", Protocol: 2, MaxRetries: -1, DialerRetries: 1, ContextTimeoutEnabled: true, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second, PoolTimeout: time.Second, DisableIdentity: true}
}

func TestRedisConsumerRequiresBoundedFailClosedClient(t *testing.T) {
	if _, err := RedisConsumer(nil); !errors.Is(err, ErrConfig) {
		t.Fatal("nil client accepted")
	}
	for _, name := range []string{"default-retries", "context-disabled", "negative-timeout", "remote-plaintext", "insecure-tls"} {
		t.Run(name, func(t *testing.T) {
			o := boundedRedisOptions()
			switch name {
			case "default-retries":
				o.MaxRetries = 0
			case "context-disabled":
				o.ContextTimeoutEnabled = false
			case "negative-timeout":
				o.ReadTimeout = -1
			case "remote-plaintext":
				o.Addr = "192.0.2.1:6379"
			case "insecure-tls":
				o.TLSConfig = &tls.Config{InsecureSkipVerify: true}
			}
			c := redis.NewClient(o)
			defer c.Close()
			if _, err := RedisConsumer(c); !errors.Is(err, ErrConfig) {
				t.Fatal("unsafe client accepted", err)
			}
		})
	}
}

// Real opt-in test uses only the owner-confirmed local fixture, verified TLS and
// two exact random JTI keys. No key scans, FLUSHDB or production data access.
func TestRealRedisTicketSingleWinnerAndOutage(t *testing.T) {
	fixtureDir := os.Getenv("POKER_TICKET_REDIS_FIXTURE")
	if fixtureDir == "" {
		t.Skip("owned local Redis fixture not configured")
	}
	if !filepath.IsAbs(fixtureDir) || os.Getenv("POKER_TICKET_REDIS_CONFIRM") != "owned-local-fixture" {
		t.Fatal("owned absolute fixture required")
	}
	secret, err := os.ReadFile(filepath.Join(fixtureDir, "admin-secret.txt"))
	if err != nil {
		t.Fatal("fixture secret missing")
	}
	cert, err := os.ReadFile(filepath.Join(fixtureDir, "server.crt"))
	if err != nil {
		t.Fatal("fixture certificate missing")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(cert) {
		t.Fatal("fixture certificate invalid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ao := boundedRedisOptions()
	ao.Addr = "127.0.0.1:16379"
	ao.Username = "g2-admin"
	ao.Password = string(secret)
	admin := redis.NewClient(ao)
	defer admin.Close()
	if admin.Ping(ctx).Err() != nil {
		t.Fatal("owned Redis not reachable")
	}
	i, v, _, _, r := fixture(t)
	// Real wall clock for this network test, not the deterministic crypto clock.
	i.now = time.Now
	v.now = time.Now
	v.startedAt = time.Now()
	token, err := i.Issue(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	c, err := v.Accept(ctx, token, *r.TargetTableID)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(c.JTI))
	otherHash := sha256.Sum256([]byte(c.JTI + "-other"))
	keys := []string{RedisKeyPrefix + hex.EncodeToString(hash[:]), RedisKeyPrefix + hex.EncodeToString(otherHash[:])}
	username := "root-ticket-" + c.JTI
	password := c.JTI + "-local-only"
	ph := sha256.Sum256([]byte(password))
	if admin.Do(ctx, "ACL", "SETUSER", username, "reset", "on", "#"+hex.EncodeToString(ph[:]), "~"+keys[0], "~"+keys[1], "+hello", "+set").Err() != nil {
		t.Fatal("exact-key ACL creation failed")
	}
	// The reused fixture admin is deliberately restricted to seat-lease keys.
	// Create our own exact-key observer rather than widen that existing account.
	observerUser := username + "-observer"
	if admin.Do(ctx, "ACL", "SETUSER", observerUser, "reset", "on", "#"+hex.EncodeToString(ph[:]), "~"+keys[0], "~"+keys[1], "+hello", "+pttl", "+pexpiretime", "+del", "+exists").Err() != nil {
		_ = admin.Do(ctx, "ACL", "DELUSER", username).Err()
		t.Fatal("exact-key observer creation failed")
	}
	observerOptions := boundedRedisOptions()
	observerOptions.Username = observerUser
	observerOptions.Password = password
	observerOptions.TLSConfig = &tls.Config{RootCAs: roots, ServerName: "localhost", MinVersion: tls.VersionTLS12}
	observer := redis.NewClient(observerOptions)
	defer observer.Close()
	t.Cleanup(func() {
		cc, done := context.WithTimeout(context.Background(), 2*time.Second)
		defer done()
		cleaner := redis.NewClient(ao)
		defer cleaner.Close()
		ownKeys := redis.NewClient(observerOptions)
		defer ownKeys.Close()
		if ownKeys.Del(cc, keys...).Err() != nil {
			t.Error("owned key cleanup failed")
		}
		if n, e := ownKeys.Exists(cc, keys...).Result(); e != nil || n != 0 {
			t.Error("owned keys still present")
		}
		if cleaner.Do(cc, "ACL", "DELUSER", username, observerUser).Err() != nil {
			t.Error("owned ACL cleanup failed")
		}
		if x, e := cleaner.Do(cc, "ACL", "GETUSER", username).Result(); e != redis.Nil && (e != nil || x != nil) {
			t.Error("owned ACL still present")
		}
		if x, e := cleaner.Do(cc, "ACL", "GETUSER", observerUser).Result(); e != redis.Nil && (e != nil || x != nil) {
			t.Error("owned observer ACL still present")
		}
	})
	o := boundedRedisOptions()
	o.Username = username
	o.Password = password
	o.TLSConfig = &tls.Config{RootCAs: roots, ServerName: "localhost", MinVersion: tls.VersionTLS12}
	client := redis.NewClient(o)
	defer client.Close()
	v.consume, err = RedisConsumer(client)
	if err != nil {
		t.Fatal(err)
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for n := 0; n < 12; n++ {
		wg.Go(func() {
			_, e := v.Accept(ctx, token, *r.TargetTableID)
			if e == nil {
				winners.Add(1)
			} else if !errors.Is(e, ErrReplay) {
				t.Error("real Redis accept failed", e)
			}
		})
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatal("real Redis single-use failed", winners.Load())
	}
	ttl, err := observer.PTTL(ctx, keys[0]).Result()
	if err != nil || ttl < 58*time.Second || ttl > TTL {
		t.Fatal("actual Redis TTL outside fixed retention", ttl)
	}
	expires, err := observer.Do(ctx, "PEXPIRETIME", keys[0]).Int64()
	if err != nil {
		t.Fatal("expiry observation failed")
	}
	if _, err = v.Accept(ctx, token, *r.TargetTableID); !errors.Is(err, ErrReplay) {
		t.Fatal("replay accepted", err)
	}
	after, err := observer.Do(ctx, "PEXPIRETIME", keys[0]).Int64()
	if err != nil || after != expires {
		t.Fatal("replay extended key retention")
	}
	if client.Close() != nil {
		t.Fatal("client close failed")
	}
	if ok, e := v.consume(ctx, otherHash, time.Now().Add(TTL)); ok || !errors.Is(e, ErrUnavailable) || strings.Contains(e.Error(), password) {
		t.Fatal("outage not fail closed", e)
	}
	if n, e := observer.Exists(ctx, keys[1]).Result(); e != nil || n != 0 {
		t.Fatal("outage fabricated used state")
	}
	t.Log("REAL_REDIS: verified TLS/exact-key ACL, 12 signed accepts one winner, replay unchanged PEXPIRETIME, TTL", ttl, "closed real client fail-closed; cleanup exact keys and ACL")
}
