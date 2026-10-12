package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	SessionCookieName = "__Host-chaldea_session"
	sessionPrefix     = "chaldea:session:local:"
	sessionIdle       = 7 * 24 * time.Hour
	sessionLifetime   = 30 * 24 * time.Hour
	anonymousLifetime = 15 * time.Minute
)

var ErrSessionExpired = errors.New("登录已失效，请重新登录")

// Session contains local identity only. Token stays in the HttpOnly cookie;
// no native SID, access token, encrypted credential box or auth version remains.
type Session struct {
	UserID    int64     `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	CSRFToken string    `json:"csrf_token"`
	Token     string    `json:"-"`
}

type Sessions struct{ Redis *redis.Client }

func sessionToken() string {
	var raw [32]byte
	rand.Read(raw[:])
	return base64.RawURLEncoding.EncodeToString(raw[:])
}

func sessionKey(token string) string {
	hash := sha256.Sum256([]byte(token))
	return sessionPrefix + hex.EncodeToString(hash[:])
}

func userSessionsKey(userID int64) string {
	return sessionPrefix + "user:" + strconv.FormatInt(userID, 10)
}

// The index expires after the newest session's absolute lifetime. On login,
// remove old members as well so active users' indexes stay bounded.
const createSession = `
local expired=redis.call('ZRANGEBYSCORE',KEYS[2],'-inf',ARGV[4])
for _,key in ipairs(expired) do redis.call('ZREM',KEYS[2],key) end
redis.call('SET',KEYS[1],ARGV[1],'PX',ARGV[2])
redis.call('ZADD',KEYS[2],ARGV[3],KEYS[1])
redis.call('PEXPIRE',KEYS[2],ARGV[5])
return 1
`

// Create is called after credential verification. Password/status mutations
// and login issuance must hold the same identity.users row lock while calling
// RevokeUser/Create, respectively; this prevents a concurrent login escaping
// revocation without adding a separate session version or identity authority.
func (s Sessions) Create(ctx context.Context, userID int64) (Session, error) {
	now := time.Now().UTC()
	value := Session{UserID: userID, CreatedAt: now, CSRFToken: sessionToken(), Token: sessionToken()}
	raw, err := json.Marshal(value)
	if err != nil {
		return Session{}, err
	}
	err = s.Redis.Eval(ctx, createSession, []string{sessionKey(value.Token), userSessionsKey(userID)},
		string(raw), sessionIdle.Milliseconds(), now.Add(sessionLifetime).UnixMilli(), now.UnixMilli(), sessionLifetime.Milliseconds()).Err()
	if err != nil {
		return Session{}, err
	}
	return value, nil
}

// Anonymous sessions only bind login CSRF to a browser. They have no user index.
func (s Sessions) CreateAnonymous(ctx context.Context) (Session, error) {
	value := Session{CreatedAt: time.Now().UTC(), CSRFToken: sessionToken(), Token: sessionToken()}
	raw, err := json.Marshal(value)
	if err != nil {
		return Session{}, err
	}
	err = s.Redis.Set(ctx, sessionKey(value.Token), raw, anonymousLifetime).Err()
	return value, err
}

// Read renews only an existing key. SET XX cannot resurrect a session that a
// concurrent logout has deleted, and the renewal never exceeds 30 days.
// HTTP callers still verify CSRF and the live local user's enabled status.
func (s Sessions) Read(ctx context.Context, token string) (Session, error) {
	if len(token) != 43 {
		return Session{}, ErrSessionExpired
	}
	key := sessionKey(token)
	raw, err := s.Redis.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return Session{}, ErrSessionExpired
	}
	if err != nil {
		return Session{}, err
	}
	var value Session
	if err := json.Unmarshal(raw, &value); err != nil {
		return Session{}, err
	}
	value.Token = token
	lifetime := sessionLifetime
	if value.UserID == 0 {
		lifetime = anonymousLifetime
	}
	remaining := time.Until(value.CreatedAt.Add(lifetime))
	if remaining <= 0 {
		_ = s.Revoke(ctx, value)
		return Session{}, ErrSessionExpired
	}
	err = s.Redis.SetArgs(ctx, key, string(raw), redis.SetArgs{Mode: "XX", TTL: min(remaining, sessionIdle)}).Err()
	if errors.Is(err, redis.Nil) {
		return Session{}, ErrSessionExpired
	}
	if err != nil {
		return Session{}, err
	}
	return value, nil
}

const revokeSession = `
redis.call('DEL',KEYS[1])
redis.call('ZREM',KEYS[2],KEYS[1])
return 1
`

func (s Sessions) Revoke(ctx context.Context, value Session) error {
	return s.Redis.Eval(ctx, revokeSession, []string{sessionKey(value.Token), userSessionsKey(value.UserID)}).Err()
}

const revokeUserSessions = `
local keys=redis.call('ZRANGEBYSCORE',KEYS[1],'-inf','+inf')
for _,key in ipairs(keys) do redis.call('DEL',key) end
redis.call('DEL',KEYS[1])
return 1
`

func (s Sessions) RevokeUser(ctx context.Context, userID int64) error {
	return s.Redis.Eval(ctx, revokeUserSessions, []string{userSessionsKey(userID)}).Err()
}

func WriteSessionCookie(w http.ResponseWriter, value Session) {
	w.Header().Set("Cache-Control", "no-store")
	lifetime := sessionLifetime
	if value.UserID == 0 {
		lifetime = anonymousLifetime
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookieName, Value: value.Token, Path: "/", Secure: true,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: value.CreatedAt.Add(lifetime),
	})
}
