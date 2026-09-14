package connectticket

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// These tests execute real Ed25519. Only the external session/replay ports are
// controlled in memory; actual Redis coverage is in the separate opt-in test.
func fixture(t *testing.T) (*Issuer, *Verifier, *time.Time, ed25519.PrivateKey, MintRequest) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	request := MintRequest{Session: Session{UserID: "910001", SessionIDHash: strings.Repeat("a", 64), SessionVersion: 3, SecurityEpoch: 2}, ControlIntent: ClaimControl}
	table := "01993200-0000-7000-8000-000000000001"
	request.TargetTableID = &table
	check := func(_ context.Context, s Session) error {
		if s != request.Session {
			return ErrRevoked
		}
		return nil
	}
	issuer, err := NewIssuer(IssuerOptions{KeyID: "k1", PrivateKey: priv, CheckSession: check, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	used := map[[32]byte]bool{}
	consume := func(_ context.Context, hash [32]byte, _ time.Time) (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		if used[hash] {
			return false, nil
		}
		used[hash] = true
		return true, nil
	}
	verifier, err := NewVerifier(VerifierOptions{PublicKeys: map[string]ed25519.PublicKey{"k1": pub}, StartedAt: now, CheckSession: check, Consume: consume, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return issuer, verifier, &now, priv, request
}

func TestRealSignatureSingleUseAndExactBindings(t *testing.T) {
	i, v, now, priv, r := fixture(t)
	token, err := i.Issue(context.Background(), r)
	if err != nil {
		t.Fatal("issue", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "ct1" {
		t.Fatal("ct1 grammar")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !ed25519.Verify(priv.Public().(ed25519.PublicKey), []byte(parts[0]+"."+parts[1]), sig) {
		t.Fatal("not exact ct1 signed bytes")
	}
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var fields map[string]any
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 14 {
		t.Fatal("payload field contract")
	}
	c, err := v.Accept(context.Background(), token, *r.TargetTableID)
	if err != nil {
		t.Fatal("accept", err)
	}
	if c.Session != r.Session || c.ControlIntent != ClaimControl || c.TargetTableID == nil || *c.TargetTableID != *r.TargetTableID || !c.IssuedAt.Equal(*now) || c.ExpiresAt.Sub(c.IssuedAt) != 60*time.Second {
		t.Fatal("claim binding mismatch")
	}
	if _, err = v.Accept(context.Background(), token, *r.TargetTableID); !errors.Is(err, ErrReplay) {
		t.Fatal("replay not denied", err)
	}
}

func TestExpiryRestartTargetTamperAndStrictJSON(t *testing.T) {
	for _, name := range []string{"expiry", "restart", "future", "target", "tamper", "unknown-key", "duplicate-json", "unknown-json", "case-alias-json", "oversized", "padding"} {
		t.Run(name, func(t *testing.T) {
			i, v, now, priv, r := fixture(t)
			token, err := i.Issue(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			target := *r.TargetTableID
			switch name {
			case "expiry":
				*now = now.Add(60 * time.Second)
			case "restart":
				v.startedAt = now.Add(time.Nanosecond)
			case "future":
				*now = now.Add(-time.Nanosecond)
			case "target":
				target = "01993200-0000-7000-8000-000000000002"
			case "tamper":
				p := strings.Split(token, ".")
				b, _ := base64.RawURLEncoding.DecodeString(p[2])
				b[0] ^= 1
				p[2] = base64.RawURLEncoding.EncodeToString(b)
				token = strings.Join(p, ".")
			case "unknown-key":
				v.keys = map[string]ed25519.PublicKey{}
			case "duplicate-json", "unknown-json", "case-alias-json":
				p := strings.Split(token, ".")
				b, _ := base64.RawURLEncoding.DecodeString(p[1])
				addition := `,"purpose":"poker_connect"}`
				if name == "unknown-json" {
					addition = `,"extra":1}`
				} else if name == "case-alias-json" {
					addition = `,"Purpose":"poker_connect"}`
				}
				b = append(b[:len(b)-1], addition...)
				p[1] = base64.RawURLEncoding.EncodeToString(b)
				p[2] = base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, []byte("ct1."+p[1])))
				token = strings.Join(p, ".")
			case "oversized":
				token = strings.Repeat("a", 8193)
			case "padding":
				p := strings.Split(token, ".")
				p[1] += "="
				token = strings.Join(p, ".")
			}
			if _, err = v.Accept(context.Background(), token, target); err == nil {
				t.Fatal("invalid ticket accepted")
			}
		})
	}
}

func TestExternalAuthorityFailsClosedAndLateExpiry(t *testing.T) {
	for _, name := range []string{"revoked", "unavailable", "replay-down", "expiry-during-session", "expiry-during-consume", "canceled"} {
		t.Run(name, func(t *testing.T) {
			i, v, now, _, r := fixture(t)
			token, err := i.Issue(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			want := ErrUnavailable
			switch name {
			case "revoked":
				v.check = func(context.Context, Session) error { return ErrRevoked }
				want = ErrRevoked
			case "unavailable":
				v.check = func(context.Context, Session) error { return errors.New("private provider secret") }
			case "replay-down":
				v.consume = func(context.Context, [32]byte, time.Time) (bool, error) {
					return true, errors.New("private redis secret")
				}
			case "expiry-during-session":
				v.check = func(context.Context, Session) error { *now = now.Add(time.Minute); return nil }
				want = ErrExpired
			case "expiry-during-consume":
				v.consume = func(context.Context, [32]byte, time.Time) (bool, error) {
					*now = now.Add(time.Minute)
					return true, nil
				}
				want = ErrExpired
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if _, err = v.Accept(ctx, token, *r.TargetTableID); !errors.Is(err, want) || strings.Contains(err.Error(), "secret") {
				t.Fatal("authority failure", err)
			}
		})
	}
}

func TestConcurrentReplayHasOnlyOneAcceptedConnection(t *testing.T) {
	i, v, _, _, r := fixture(t)
	token, err := i.Issue(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	var winners atomic.Int32
	var group sync.WaitGroup
	for n := 0; n < 12; n++ {
		group.Go(func() {
			_, err := v.Accept(context.Background(), token, *r.TargetTableID)
			if err == nil {
				winners.Add(1)
			} else if !errors.Is(err, ErrReplay) {
				t.Error(err)
			}
		})
	}
	group.Wait()
	if winners.Load() != 1 {
		t.Fatal("concurrent ticket accepted more than once", winners.Load())
	}
}

func TestSignedClaimsStrictScalarAndJTIShape(t *testing.T) {
	for _, tc := range []struct{ field, value string }{
		{"security_epoch_snapshot", "null"}, {"security_epoch_snapshot", "-0"},
		{"security_epoch_snapshot", "0.0"}, {"security_epoch_snapshot", "1e0"},
		{"security_epoch_snapshot", "9007199254740992"}, {"session_version", "null"},
		{"jti", `"01993200-0000-4000-8000-000000000001"`},
		{"jti", `"01993200-0000-7000-0000-000000000001"`},
	} {
		t.Run(tc.field+"-"+tc.value, func(t *testing.T) {
			i, v, _, priv, r := fixture(t)
			// A live epoch-zero session is valid; null must not coerce into it.
			r.Session.SecurityEpoch = 0
			i.check = func(context.Context, Session) error { return nil }
			v.check = i.check
			token, err := i.Issue(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			p := strings.Split(token, ".")
			b, _ := base64.RawURLEncoding.DecodeString(p[1])
			var fields map[string]json.RawMessage
			if err = json.Unmarshal(b, &fields); err != nil {
				t.Fatal(err)
			}
			fields[tc.field] = json.RawMessage(tc.value)
			b, err = json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			p[1] = base64.RawURLEncoding.EncodeToString(b)
			p[2] = base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, []byte("ct1."+p[1])))
			if _, err = v.Accept(context.Background(), strings.Join(p, "."), *r.TargetTableID); !errors.Is(err, ErrInvalid) {
				t.Fatal("signed malformed field accepted", err)
			}
		})
	}
}

func TestKeyRotationAndCopiedConfiguration(t *testing.T) {
	i, v, now, priv, r := fixture(t)
	pub2, priv2, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	check := func(context.Context, Session) error { return nil }
	oldPublic := priv.Public().(ed25519.PublicKey)
	keys := map[string]ed25519.PublicKey{"k1": oldPublic, "k2": pub2}
	v2, err := NewVerifier(VerifierOptions{PublicKeys: keys, StartedAt: *now, CheckSession: check, Consume: v.consume, Now: v.now})
	if err != nil {
		t.Fatal(err)
	}
	i2, err := NewIssuer(IssuerOptions{KeyID: "k2", PrivateKey: priv2, CheckSession: check, Now: i.now})
	if err != nil {
		t.Fatal(err)
	}
	clear(priv2)
	clear(pub2)
	clear(oldPublic)
	delete(keys, "k1")
	for _, issuer := range []*Issuer{i, i2} {
		token, err := issuer.Issue(context.Background(), r)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = v2.Accept(context.Background(), token, *r.TargetTableID); err != nil {
			t.Fatal("copied/rotated key failed", err)
		}
	}
	if _, err = NewIssuer(IssuerOptions{KeyID: "k", PrivateKey: priv2, CheckSession: check}); !errors.Is(err, ErrConfig) {
		t.Fatal("inconsistent private key accepted")
	}
	if _, err = NewVerifier(VerifierOptions{PublicKeys: keys, StartedAt: *now, CheckSession: check, Now: v.now}); !errors.Is(err, ErrConfig) {
		t.Fatal("missing replay authority accepted")
	}
}
