// Package connectticket implements IS §§163–164 signed one-use Poker tickets.
// It does not infer session authority from unverified native credentials.
package connectticket

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/nativeself"
)

const TTL = 60 * time.Second
const ClaimControl = "CLAIM_CONTROL"
const ReadOnly = "READ_ONLY"
const maxSafe = uint64(9007199254740991)

var ErrInvalid = errors.New("POKER_TICKET_INVALID")
var ErrConfig = errors.New("POKER_TICKET_CONFIG_INVALID")
var ErrExpired = errors.New("POKER_TICKET_EXPIRED")
var ErrRestart = errors.New("POKER_TICKET_RESTART_FENCED")
var ErrReplay = errors.New("TICKET_REPLAYED")
var ErrRevoked = errors.New("POKER_SESSION_REVOKED")
var ErrUnavailable = errors.New("POKER_AUTH_UNAVAILABLE")

type Session struct {
	UserID         string `json:"newapi_user_id"`
	SessionIDHash  string `json:"session_id_hash"`
	SessionVersion uint64 `json:"session_version"`
	SecurityEpoch  uint64 `json:"security_epoch_snapshot"`
}
type MintRequest struct {
	Session       Session
	TargetTableID *string
	ControlIntent string
}
type Claims struct {
	Version  int    `json:"v"`
	KeyID    string `json:"kid"`
	Issuer   string `json:"iss"`
	Audience string `json:"aud"`
	JTI      string `json:"jti"`
	Session
	Purpose       string    `json:"purpose"`
	TargetTableID *string   `json:"target_table_id"`
	ControlIntent string    `json:"control_intent"`
	IssuedAt      time.Time `json:"issued_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}
type SessionCheck func(context.Context, Session) error
type ConsumeFunc func(context.Context, [32]byte, time.Time) (bool, error)
type IssuerOptions struct {
	KeyID        string
	PrivateKey   ed25519.PrivateKey
	CheckSession SessionCheck
	Now          func() time.Time
}
type VerifierOptions struct {
	PublicKeys   map[string]ed25519.PublicKey
	StartedAt    time.Time
	CheckSession SessionCheck
	Consume      ConsumeFunc
	Now          func() time.Time
}
type Issuer struct {
	kid     string
	private ed25519.PrivateKey
	check   SessionCheck
	now     func() time.Time
}
type Verifier struct {
	keys      map[string]ed25519.PublicKey
	startedAt time.Time
	check     SessionCheck
	consume   ConsumeFunc
	now       func() time.Time
}

func NewIssuer(o IssuerOptions) (*Issuer, error) {
	if !keyID(o.KeyID) || len(o.PrivateKey) != ed25519.PrivateKeySize || o.CheckSession == nil {
		return nil, ErrConfig
	}
	if !bytes.Equal(ed25519.NewKeyFromSeed(o.PrivateKey.Seed()), o.PrivateKey) {
		return nil, ErrConfig
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Issuer{o.KeyID, append(ed25519.PrivateKey{}, o.PrivateKey...), o.CheckSession, o.Now}, nil
}
func NewVerifier(o VerifierOptions) (*Verifier, error) {
	if len(o.PublicKeys) == 0 || o.CheckSession == nil || o.Consume == nil || o.StartedAt.IsZero() {
		return nil, ErrConfig
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.StartedAt.After(o.Now()) {
		return nil, ErrConfig
	}
	keys := make(map[string]ed25519.PublicKey, len(o.PublicKeys))
	for id, key := range o.PublicKeys {
		if !keyID(id) || len(key) != ed25519.PublicKeySize {
			return nil, ErrConfig
		}
		keys[id] = append(ed25519.PublicKey{}, key...)
	}
	return &Verifier{keys, o.StartedAt, o.CheckSession, o.Consume, o.Now}, nil
}

// Issue requires the live platform session check before minting; no caller-supplied
// user ID, hash or epoch becomes authoritative just by passing shape validation.
func (i *Issuer) Issue(ctx context.Context, r MintRequest) (string, error) {
	if i == nil || ctx == nil || !validSession(r.Session) || !validTarget(r.TargetTableID) || !validIntent(r.ControlIntent) {
		return "", ErrInvalid
	}
	if err := checkLive(ctx, i.check, r.Session); err != nil {
		return "", err
	}
	now := i.now().UTC()
	if !validInstant(now) {
		return "", ErrInvalid
	}
	var b [16]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return "", ErrUnavailable
	}
	ms := uint64(now.UnixMilli())
	for n := 5; n >= 0; n-- {
		b[n] = byte(ms)
		ms >>= 8
	}
	b[6] = (b[6] & 15) | 0x70
	b[8] = (b[8] & 63) | 0x80
	h := hex.EncodeToString(b[:])
	id := h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
	c := Claims{Version: 1, KeyID: i.kid, Issuer: "chaldea-platform", Audience: "chaldea-poker", JTI: id, Session: r.Session, Purpose: "poker_connect", TargetTableID: r.TargetTableID, ControlIntent: r.ControlIntent, IssuedAt: now, ExpiresAt: now.Add(TTL)}
	raw, err := json.Marshal(c)
	if err != nil {
		return "", ErrInvalid
	}
	message := "ct1." + base64.RawURLEncoding.EncodeToString(raw)
	return message + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(i.private, []byte(message))), nil
}

// Accept checks signature and exact target before any external authority call.
// The consumer must atomically retain sha256(jti) as used; a missing consumer is
// never replaced by a local map. Time is rechecked after each external call.
func (v *Verifier) Accept(ctx context.Context, token, table string) (Claims, error) {
	if v == nil || ctx == nil || len(token) > 8192 || len(token) < 100 || (table != "" && !uuid(table)) {
		return Claims{}, ErrInvalid
	}
	if ctx.Err() != nil {
		return Claims{}, ErrUnavailable
	}
	p := strings.Split(token, ".")
	if len(p) != 3 || p[0] != "ct1" {
		return Claims{}, ErrInvalid
	}
	raw, err := decode(p[1])
	if err != nil || !utf8.Valid(raw) {
		return Claims{}, ErrInvalid
	}
	sig, err := decode(p[2])
	if err != nil || len(sig) != ed25519.SignatureSize {
		return Claims{}, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if !nativeself.UniqueJSON(d, 0) {
		return Claims{}, ErrInvalid
	}
	if _, err = d.Token(); err != io.EOF {
		return Claims{}, ErrInvalid
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 14 {
		return Claims{}, ErrInvalid
	}
	for _, key := range []string{"v", "kid", "iss", "aud", "jti", "newapi_user_id", "session_id_hash", "session_version", "security_epoch_snapshot", "purpose", "target_table_id", "control_intent", "issued_at", "expires_at"} {
		value, ok := fields[key]
		if !ok || (key != "target_table_id" && bytes.Equal(bytes.TrimSpace(value), []byte("null"))) {
			return Claims{}, ErrInvalid
		}
	}
	var c Claims
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil {
		return Claims{}, ErrInvalid
	}
	key := v.keys[c.KeyID]
	if len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, []byte("ct1."+p[1]), sig) {
		return Claims{}, ErrInvalid
	}
	if c.Version != 1 || c.Issuer != "chaldea-platform" || c.Audience != "chaldea-poker" || c.Purpose != "poker_connect" || !uuid(c.JTI) || c.JTI[14] != '7' || !strings.ContainsRune("89ab", rune(c.JTI[19])) || !validSession(c.Session) || !validTarget(c.TargetTableID) || !validIntent(c.ControlIntent) {
		return Claims{}, ErrInvalid
	}
	if c.TargetTableID != nil && *c.TargetTableID != table {
		return Claims{}, ErrInvalid
	}
	if err = v.checkTime(c); err != nil {
		return Claims{}, err
	}
	if err = checkLive(ctx, v.check, c.Session); err != nil {
		return Claims{}, err
	}
	if err = v.checkTime(c); err != nil {
		return Claims{}, err
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ok, err := v.consume(bounded, sha256.Sum256([]byte(c.JTI)), c.ExpiresAt)
	if err != nil || bounded.Err() != nil {
		return Claims{}, ErrUnavailable
	}
	if !ok {
		return Claims{}, ErrReplay
	}
	if err = v.checkTime(c); err != nil {
		return Claims{}, err
	}
	return c, nil
}
func (v *Verifier) checkTime(c Claims) error {
	now := v.now().UTC()
	if !validInstant(c.IssuedAt) || !validInstant(c.ExpiresAt) || !validInstant(now) || c.ExpiresAt.Sub(c.IssuedAt) != TTL || c.IssuedAt.After(now) {
		return ErrInvalid
	}
	if !c.ExpiresAt.After(now) {
		return ErrExpired
	}
	if c.IssuedAt.Before(v.startedAt) {
		return ErrRestart
	}
	return nil
}
func checkLive(ctx context.Context, check SessionCheck, s Session) error {
	if ctx.Err() != nil {
		return ErrUnavailable
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err := check(bounded, s)
	if bounded.Err() != nil {
		return ErrUnavailable
	}
	if errors.Is(err, ErrRevoked) {
		return ErrRevoked
	}
	if err != nil {
		return ErrUnavailable
	}
	return nil
}
func decode(s string) ([]byte, error) {
	b, e := base64.RawURLEncoding.DecodeString(s)
	if e != nil || base64.RawURLEncoding.EncodeToString(b) != s {
		return nil, ErrInvalid
	}
	return b, nil
}
func keyID(s string) bool {
	if len(s) < 1 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
func validIntent(s string) bool  { return s == ClaimControl || s == ReadOnly }
func validTarget(s *string) bool { return s == nil || uuid(*s) }
func validInstant(t time.Time) bool {
	_, offset := t.Zone()
	return !t.IsZero() && t.Year() >= 2000 && t.Year() < 9999 && offset == 0
}
func validSession(s Session) bool {
	n, e := strconv.ParseInt(s.UserID, 10, 64)
	return e == nil && n > 0 && strconv.FormatInt(n, 10) == s.UserID && len(s.SessionIDHash) == 64 && hexLower(s.SessionIDHash) && s.SessionVersion > 0 && s.SessionVersion <= maxSafe && s.SecurityEpoch <= maxSafe
}
func uuid(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
		} else if !hexLower(string(r)) {
			return false
		}
	}
	return true
}
func hexLower(s string) bool {
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func (*Issuer) String() string     { return "connectticket.Issuer<redacted>" }
func (*Issuer) GoString() string   { return "connectticket.Issuer<redacted>" }
func (*Verifier) String() string   { return "connectticket.Verifier<redacted>" }
func (*Verifier) GoString() string { return "connectticket.Verifier<redacted>" }
