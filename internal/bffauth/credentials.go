package bffauth

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/nativeself"
	"github.com/cy4268/momiao/internal/session"
	"github.com/redis/go-redis/v9"
)

const credentialPrefix = "chaldea:bff:credential:"

// An owned operation must finish or record uncertainty even after disconnect.
// This is bounded synchronous cleanup, not a retry or a durable recovery queue.
func finalizationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
}

type Fault struct {
	Status      int
	Code        string
	ClearCookie bool
	Unknown     bool
}

func (f Fault) Error() string { return f.Code }

var (
	errInput        = Fault{Status: 400, Code: "AUTH_INPUT_INVALID"}
	errUnauthorized = Fault{Status: 401, Code: "AUTH_UNAUTHORIZED", ClearCookie: true}
	errCSRF         = Fault{Status: 403, Code: "AUTH_CSRF_FAILED"}
	errConflict     = Fault{Status: 409, Code: "AUTH_CONFLICT"}
	errUnavailable  = Fault{Status: 503, Code: "AUTH_UNAVAILABLE"}
	errUnknown      = Fault{Status: 503, Code: "AUTH_RESULT_UNKNOWN", ClearCookie: true, Unknown: true}
	errMissing      = errors.New("bff value missing")
)

type Options struct {
	Redis             *redis.Client
	Sessions          *session.Service
	Native            http.RoundTripper
	Origin            string
	CredentialSealKey [32]byte
	Control           ControlRevoker
}

type Service struct {
	redis        *redis.Client
	sessions     *session.Service
	native       http.RoundTripper
	origin       string
	seal         cipher.AEAD
	authChainKey [32]byte
	control      ControlRevoker
	accounts     AccountRefReader
}

func New(o Options) (*Service, error) {
	u, err := url.Parse(o.Origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.String() != o.Origin || o.Redis == nil || o.Sessions == nil || o.Native == nil || o.CredentialSealKey == [32]byte{} {
		return nil, errInput
	}
	block, err := aes.NewCipher(o.CredentialSealKey[:])
	if err != nil {
		return nil, errInput
	}
	seal, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errInput
	}
	return &Service{redis: o.Redis, sessions: o.Sessions, native: o.Native, origin: o.Origin, seal: seal, authChainKey: deriveAuthChainKey(o.CredentialSealKey), control: o.Control}, nil
}

type nativeCredential struct {
	UserID, Username, DisplayName    string
	SID, AccessToken, RefreshToken   string
	AccessExpiresAt, NativeExpiresAt int64
}

type credentialRecord struct {
	Schema     int    `json:"schema_version"`
	State      string `json:"state"`
	NativeHash string `json:"native_session_id_hash"`
	Revision   uint64 `json:"revision"`
	Absolute   int64  `json:"absolute_expires_at_ms"`
	LeaseHash  string `json:"lease_hash,omitempty"`
	LeaseUntil int64  `json:"lease_expires_at_ms,omitempty"`
	Box        string `json:"credential_box,omitempty"`
	raw        string `json:"-"`
}

func (nativeCredential) String() string   { return "nativeCredential([redacted])" }
func (nativeCredential) GoString() string { return "nativeCredential([redacted])" }

func randomSecret() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", errUnavailable
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func opaque(v string) bool {
	b, err := base64.RawURLEncoding.DecodeString(v)
	return err == nil && len(b) == 32 && base64.RawURLEncoding.EncodeToString(b) == v
}

func digest(v string) bool {
	b, err := hex.DecodeString(v)
	return err == nil && len(b) == sha256.Size && hex.EncodeToString(b) == v
}

func hashText(v string) string { sum := sha256.Sum256([]byte(v)); return hex.EncodeToString(sum[:]) }

func deriveAuthChainKey(master [32]byte) [32]byte {
	mac := hmac.New(sha256.New, master[:])
	_, _ = mac.Write([]byte("chaldea:bff:auth-chain:key:v1"))
	var key [32]byte
	copy(key[:], mac.Sum(nil))
	return key
}

func (s *Service) authChainID(binding session.BFFBinding) (string, error) {
	userID, err := strconv.ParseInt(binding.UserID, 10, 64)
	if s == nil || s.origin == "" || s.authChainKey == [32]byte{} || err != nil || userID <= 0 || strconv.FormatInt(userID, 10) != binding.UserID || !digest(binding.NativeSessionIDHash) {
		return "", errInput
	}
	mac := hmac.New(sha256.New, s.authChainKey[:])
	for _, field := range []string{
		"chaldea:bff:auth-chain:id:v1",
		"origin", s.origin,
		"user_id", binding.UserID,
		"native_session_id_hash", binding.NativeSessionIDHash,
	} {
		if uint64(len(field)) > uint64(^uint32(0)) {
			return "", errInput
		}
		var size [4]byte
		binary.BigEndian.PutUint32(size[:], uint32(len(field)))
		_, _ = mac.Write(size[:])
		_, _ = mac.Write([]byte(field))
	}
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func strictJSON(raw []byte, out any, limit int) bool {
	if len(raw) == 0 || len(raw) > limit {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if !nativeself.UniqueJSON(d, 0) {
		return false
	}
	if _, err := d.Token(); err != io.EOF || json.Unmarshal(raw, out) != nil {
		return false
	}
	var left, right any
	encoded, err := json.Marshal(out)
	return err == nil && json.Unmarshal(raw, &left) == nil && json.Unmarshal(encoded, &right) == nil && bytes.Equal(mustJSON(left), mustJSON(right))
}

func mustJSON(v any) []byte { raw, _ := json.Marshal(v); return raw }

func (s *Service) sealJSON(label string, value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 12288 {
		return "", errUnavailable
	}
	nonce := make([]byte, s.seal.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", errUnavailable
	}
	return base64.RawURLEncoding.EncodeToString(s.seal.Seal(nonce, nonce, raw, []byte(label))), nil
}

func (s *Service) openJSON(label, box string, out any) error {
	raw, err := base64.RawURLEncoding.DecodeString(box)
	if err != nil || len(raw) <= s.seal.NonceSize() || base64.RawURLEncoding.EncodeToString(raw) != box {
		return errUnauthorized
	}
	plain, err := s.seal.Open(nil, raw[:s.seal.NonceSize()], raw[s.seal.NonceSize():], []byte(label))
	if err != nil || !strictJSON(plain, out, 12288) {
		return errUnauthorized
	}
	return nil
}

const readValueScript = `local v=redis.call('GET',KEYS[1]); if not v then return false end; local t=redis.call('TIME'); return {v,tostring(redis.call('PTTL',KEYS[1])),string.format('%.0f',t[1]*1000+math.floor(t[2]/1000))}`
const casValueScript = `if redis.call('GET',KEYS[1])~=ARGV[1] then return 0 end; if ARGV[2]=='' then redis.call('DEL',KEYS[1]); elseif tonumber(ARGV[3])>0 then redis.call('SET',KEYS[1],ARGV[2],'XX','PX',ARGV[3]); else redis.call('SET',KEYS[1],ARGV[2],'XX','KEEPTTL'); end; return 1`

func (s *Service) readValue(ctx context.Context, key string) (string, int64, int64, error) {
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	reply, err := s.redis.Eval(c, readValueScript, []string{key}).StringSlice()
	if err == redis.Nil {
		return "", 0, 0, errMissing
	}
	if err != nil || len(reply) != 3 {
		return "", 0, 0, errUnavailable
	}
	ttl, e1 := strconv.ParseInt(reply[1], 10, 64)
	now, e2 := strconv.ParseInt(reply[2], 10, 64)
	if e1 != nil || e2 != nil || ttl <= 0 || now <= 0 {
		return "", 0, 0, errUnavailable
	}
	return reply[0], now, ttl, nil
}

func (s *Service) casValue(ctx context.Context, key, old, next string, ttl time.Duration) error {
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	result, err := s.redis.Eval(c, casValueScript, []string{key}, old, next, ttl.Milliseconds()).Int()
	if err != nil {
		return errUnavailable
	}
	if result != 1 {
		return errConflict
	}
	return nil
}

func validCredential(c nativeCredential, nativeHash string, now int64) bool {
	return c.UserID != "" && c.Username != "" && c.SID != "" && hashText(c.SID) == nativeHash && c.AccessToken != "" && c.RefreshToken != "" && c.AccessExpiresAt > 0 && c.NativeExpiresAt*1000 > now
}

func (s *Service) storeCredential(ctx context.Context, nativeHash string, c nativeCredential, absolute time.Time) error {
	now := time.Now().UnixMilli()
	if !digest(nativeHash) || !validCredential(c, nativeHash, now) || !absolute.After(time.Now()) || absolute.UnixMilli() > c.NativeExpiresAt*1000 {
		return errInput
	}
	box, err := s.sealJSON("credential:1:"+nativeHash, c)
	if err != nil {
		return err
	}
	r := credentialRecord{Schema: 1, State: "ACTIVE", NativeHash: nativeHash, Revision: 1, Absolute: absolute.UnixMilli(), Box: box}
	raw, _ := json.Marshal(r)
	cx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	result, err := s.redis.SetNX(cx, credentialPrefix+nativeHash, raw, time.Until(absolute)).Result()
	if err != nil {
		return errUnavailable
	}
	if !result {
		return errConflict
	}
	return nil
}

func (s *Service) loadCredential(ctx context.Context, nativeHash string) (credentialRecord, nativeCredential, error) {
	raw, now, ttl, err := s.readValue(ctx, credentialPrefix+nativeHash)
	if err != nil {
		if errors.Is(err, errMissing) {
			return credentialRecord{}, nativeCredential{}, errUnavailable
		}
		return credentialRecord{}, nativeCredential{}, err
	}
	var r credentialRecord
	var c nativeCredential
	if !strictJSON([]byte(raw), &r, 16384) || r.Schema != 1 || r.NativeHash != nativeHash || !digest(nativeHash) || r.Revision == 0 || r.Absolute <= now || ttl > r.Absolute-now+1000 || r.Box == "" || (r.State != "ACTIVE" && r.State != "REFRESHING" && r.State != "MUTATING" && r.State != "REVOKING" && r.State != "UNKNOWN") || s.openJSON("credential:1:"+nativeHash, r.Box, &c) != nil || !validCredential(c, nativeHash, now) {
		return credentialRecord{}, nativeCredential{}, errUnauthorized
	}
	r.raw = raw
	return r, c, nil
}

func (s *Service) claimCredential(ctx context.Context, r credentialRecord, state string) (credentialRecord, string, error) {
	lease, err := randomSecret()
	if err != nil {
		return r, "", err
	}
	if r.State != "ACTIVE" && !(state == "REVOKING" && (r.State == "REFRESHING" || r.State == "MUTATING")) {
		return r, "", errConflict
	}
	r.State, r.LeaseHash, r.LeaseUntil, r.Revision = state, hashText(lease), time.Now().Add(10*time.Second).UnixMilli(), r.Revision+1
	raw, _ := json.Marshal(r)
	if err = s.casValue(ctx, credentialPrefix+r.NativeHash, r.raw, string(raw), 0); err != nil {
		return r, "", err
	}
	r.raw = string(raw)
	return r, lease, nil
}

func (s *Service) finishCredential(ctx context.Context, r credentialRecord, lease, state string, c *nativeCredential) error {
	if r.LeaseHash != hashText(lease) || r.LeaseUntil < time.Now().UnixMilli() {
		return errConflict
	}
	r.State, r.LeaseHash, r.LeaseUntil, r.Revision = state, "", 0, r.Revision+1
	if c != nil {
		box, err := s.sealJSON("credential:1:"+r.NativeHash, *c)
		if err != nil {
			return err
		}
		r.Box = box
	}
	if state == "REVOKED" {
		r.Box = ""
	}
	raw, _ := json.Marshal(r)
	if err := s.casValue(ctx, credentialPrefix+r.NativeHash, r.raw, string(raw), 0); err != nil {
		return err
	}
	return nil
}

func (s *Service) ensureCredential(ctx context.Context, binding session.BFFBinding) (nativeCredential, error) {
	record, current, err := s.loadCredential(ctx, binding.NativeSessionIDHash)
	if err != nil {
		return nativeCredential{}, err
	}
	if current.UserID != binding.UserID {
		return nativeCredential{}, errUnauthorized
	}
	if record.State != "ACTIVE" {
		if record.State == "UNKNOWN" {
			return nativeCredential{}, errUnknown
		}
		return nativeCredential{}, errConflict
	}
	if current.AccessExpiresAt > time.Now().Add(30*time.Second).Unix() {
		return current, nil
	}
	record, lease, err := s.claimCredential(ctx, record, "REFRESHING")
	if err != nil {
		return nativeCredential{}, err
	}
	next, refreshErr := s.refresh(ctx, current)
	finishCtx, finish := finalizationContext(ctx)
	defer finish()
	if refreshErr != nil {
		_ = s.finishCredential(finishCtx, record, lease, "UNKNOWN", &current)
		return nativeCredential{}, refreshErr
	}
	if err = s.finishCredential(finishCtx, record, lease, "ACTIVE", &next); err != nil {
		// Another operation won the credential CAS. Revoke the just-rotated
		// Native chain instead of allowing its late success to restore access.
		_ = s.nativeLogout(finishCtx, next)
		return nativeCredential{}, errUnknown
	}
	return next, nil
}

func (s *Service) orphanCredential(ctx context.Context, nativeHash string) {
	record, current, err := s.loadCredential(ctx, nativeHash)
	if err != nil || record.State != "ACTIVE" {
		return
	}
	record, lease, err := s.claimCredential(ctx, record, "REVOKING")
	if err == nil {
		_ = s.finishCredential(ctx, record, lease, "UNKNOWN", &current)
	}
}
