package session

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/nativeself"
	"github.com/redis/go-redis/v9"
)

const sessionPrefix = "chaldea:session:"

type record struct {
	Schema        int           `json:"schema_version"`
	State         string        `json:"state"`
	Hash          string        `json:"session_id_hash"`
	UserID        string        `json:"newapi_user_id"`
	Created       string        `json:"created_at"`
	ChainStart    string        `json:"auth_chain_started_at"`
	LastSeen      string        `json:"last_seen_at"`
	Idle          string        `json:"idle_expires_at"`
	Absolute      string        `json:"absolute_expires_at"`
	LoginMethod   string        `json:"login_method"`
	Authenticated string        `json:"authenticated_at"`
	Fresh         *string       `json:"fresh_auth_at"`
	FreshMethod   string        `json:"fresh_auth_method"`
	SecurityEpoch string        `json:"security_epoch_snapshot"`
	AccountStatus string        `json:"account_status_snapshot"`
	Checked       string        `json:"account_status_checked_at"`
	CSRF          string        `json:"csrf_secret"`
	Version       string        `json:"session_version"`
	Revoked       *string       `json:"revoked_at"`
	NativeHash    string        `json:"native_sid_hash"`
	NativeSV      string        `json:"native_session_version"`
	NativeUV      string        `json:"native_auth_version"`
	NativeCreated string        `json:"native_created_at"`
	NativeExpires string        `json:"native_expires_at"`
	NativeBox     string        `json:"native_sid_box"`
	Pending       *pending      `json:"pending_ops,omitempty"`
	Confirmation  *Confirmation `json:"confirmation,omitempty"`
	raw           string
}

func validRedis(client *redis.Client) bool {
	if client == nil {
		return false
	}
	o := client.Options()
	if o.Protocol != 2 || o.DB != 0 || o.MaxRetries != 0 || o.DialerRetries != 1 ||
		!o.ContextTimeoutEnabled || !o.DisableIdentity || o.ClientName != "" ||
		o.ClientSideCache != nil || o.ClientSideCacheConfig != nil || o.MaintNotificationsConfig.Mode != "disabled" ||
		o.Username == "" || o.Password == "" || o.TLSConfig == nil || o.TLSConfig.InsecureSkipVerify ||
		o.TLSConfig.MinVersion < tls.VersionTLS12 {
		return false
	}
	for _, d := range []time.Duration{o.DialTimeout, o.ReadTimeout, o.WriteTimeout, o.PoolTimeout} {
		if d <= 0 || d > 2*time.Second {
			return false
		}
	}
	return true
}
func strictJSON(raw []byte, out any) bool {
	if len(raw) > 8192 || !utf8.Valid(raw) || !nativeself.UniqueJSON(json.NewDecoder(bytes.NewReader(raw)), 0) ||
		json.Unmarshal(raw, out) != nil {
		return false
	}
	var input, canonical any
	if json.Unmarshal(raw, &input) != nil {
		return false
	}
	encoded, _ := json.Marshal(out)
	if json.Unmarshal(encoded, &canonical) != nil {
		return false
	}
	left, _ := json.Marshal(input)
	right, _ := json.Marshal(canonical)
	return bytes.Equal(left, right)
}
func secret() string {
	var raw [32]byte
	rand.Read(raw[:])
	return base64.RawURLEncoding.EncodeToString(raw[:])
}
func opaque(raw string) bool {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	return err == nil && len(b) == 32 && base64.RawURLEncoding.EncodeToString(b) == raw
}
func secretHash(raw string) string {
	b, _ := base64.RawURLEncoding.DecodeString(raw)
	return hashHexBytes(b)
}
func equalHex(b []byte, h string) bool { return hex.EncodeToString(b) == h }
func (s *Service) sealSID(sid string, r record) string {
	nonce := make([]byte, s.seal.NonceSize())
	rand.Read(nonce)
	box := s.seal.Seal(nonce, nonce, []byte(sid), []byte("1:"+r.Hash+":"+r.NativeHash))
	return base64.RawURLEncoding.EncodeToString(box)
}
func (s *Service) openSID(r record) (string, error) {
	box, err := base64.RawURLEncoding.DecodeString(r.NativeBox)
	if err != nil || len(box) < s.seal.NonceSize() || base64.RawURLEncoding.EncodeToString(box) != r.NativeBox {
		return "", authFault
	}
	plain, err := s.seal.Open(nil, box[:s.seal.NonceSize()], box[s.seal.NonceSize():], []byte("1:"+r.Hash+":"+r.NativeHash))
	if err != nil || !canonicalUUID(string(plain)) || hashHexBytes(plain) != r.NativeHash {
		return "", authFault
	}
	return string(plain), nil
}

const readSession = `
local raw=redis.call('GET',KEYS[1])
if not raw then return false end
local t=redis.call('TIME')
return {raw,tostring(redis.call('PTTL',KEYS[1])),string.format('%.0f',t[1]*1000+math.floor(t[2]/1000))}
`

func (s *Service) load(ctx context.Context, hash string) (record, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	r := record{}
	reply, err := s.options.Redis.Eval(ctx, readSession, []string{sessionPrefix + hash}).StringSlice()
	if err == redis.Nil {
		return r, authFault
	}
	if err != nil || len(reply) != 3 {
		return r, unavailableFault
	}
	if !strictJSON([]byte(reply[0]), &r) || !r.valid(hash, time.Now().UnixMilli()) ||
		milliseconds(reply[2]) <= 0 || milliseconds(reply[1]) <= 0 || milliseconds(reply[1]) > milliseconds(r.Idle)-milliseconds(reply[2])+1 {
		_ = s.drop(ctx, sessionPrefix+hash)
		return record{}, authFault
	}
	if _, err := s.openSID(r); err != nil {
		_ = s.drop(ctx, sessionPrefix+hash)
		return record{}, authFault
	}
	r.raw = reply[0]
	return r, nil
}

// Every fallible preflight runs before DEL; Redis script errors do not roll back DEL.
const writeSession = `
if #ARGV~=5 or (#KEYS~=1 and #KEYS~=2) then return 0 end
local old=redis.call('GET',KEYS[1])
if old~=(ARGV[1]~='' and ARGV[1] or false) then return 0 end
local t=redis.call('TIME')
local now=t[1]*1000+math.floor(t[2]/1000)
local expiry,guard,fresh=tonumber(ARGV[3]),tonumber(ARGV[4]),tonumber(ARGV[5])
if not expiry or not guard or not fresh or expiry<=now or guard<=now or fresh>now then return 0 end
if old and redis.call('PTTL',KEYS[1])<=0 then return 0 end
if #KEYS==2 then
  if not old or KEYS[1]==KEYS[2] or redis.call('GET',KEYS[2]) then return 0 end
  redis.call('DEL',KEYS[1])
  redis.call('SET',KEYS[2],ARGV[2],'NX','PX',expiry-now)
else
  redis.call('SET',KEYS[1],ARGV[2],'PX',expiry-now)
end
return 1
`

func (s *Service) put(ctx context.Context, old *record, next record, guard, fresh int64) (record, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if !next.valid(next.Hash, time.Now().UnixMilli()) {
		return record{}, authFault
	}
	raw, err := json.Marshal(next)
	if err != nil || len(raw) > 8192 {
		return record{}, unavailableFault
	}
	keys, expected := []string{sessionPrefix + next.Hash}, ""
	if old != nil {
		keys[0], expected = sessionPrefix+old.Hash, old.raw
		if old.Hash != next.Hash {
			keys = append(keys, sessionPrefix+next.Hash)
		}
	}
	result, err := s.options.Redis.Eval(ctx, writeSession, keys, expected, string(raw), milliseconds(next.Idle), guard, fresh).Int()
	if err != nil {
		return record{}, unavailableFault
	}
	if result != 1 {
		return record{}, conflictFault
	}
	next.raw = string(raw)
	return next, nil
}

func (s *Service) touch(ctx context.Context, r record) (record, error) {
	now := time.Now().UnixMilli()
	if now-milliseconds(r.LastSeen) < s.options.Lifetime.Touch.Milliseconds() || r.Pending != nil && r.Pending.Phase == "EXCHANGING" {
		return r, nil
	}
	old := r
	r.LastSeen, r.Checked = ms(now), ms(now)
	r.Idle = ms(min(now+s.options.Lifetime.Idle.Milliseconds(), milliseconds(r.Absolute)))
	return s.put(ctx, &old, r, milliseconds(old.Idle), 0)
}

func (s *Service) Revoke(ctx context.Context, h RequestSession) error {
	if ctx == nil {
		return inputFault
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	r, err := s.current(ctx, h)
	if err != nil {
		return err
	}
	if err = s.drop(ctx, sessionPrefix+r.Hash); err != nil {
		return unavailableFault
	}
	return nil
}

func (s *Service) drop(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.options.Redis.Del(ctx, key).Err()
}
