// Package session binds opaque browser sessions to the live Native authority.
package session

import (
	"crypto/cipher"
	"crypto/subtle"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/poker/authbridge"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Lifetime struct{ Idle, Absolute, Touch time.Duration }
type Options struct {
	Redis           *redis.Client
	Pool            *pgxpool.Pool
	NativeSocket    string
	NativeReaderKey string
	OpsSocket       string
	OpsKey          string
	SessionSealKey  [32]byte
	Origin          string
	Environment     string
	Lifetime        Lifetime
}
type NativeCredential struct{ UserID, Username, SID, AccessToken string }
type Operation struct {
	ID, Action, TargetKind, TargetID, ExpectedVersion string
	CommandDigest, ImpactDigest                       [32]byte
}
type Challenge struct {
	ID        string    `json:"challenge_id"`
	Token     string    `json:"challenge_token"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Confirmation struct {
	OperationID       string `json:"operation_id"`
	ContextHash       string `json:"context_hash"`
	SourceSessionHash string `json:"source_session_hash"`
}
type View struct {
	UserID             string        `json:"newapi_user_id"`
	AuthChainStartedAt time.Time     `json:"auth_chain_started_at"`
	AbsoluteExpiresAt  time.Time     `json:"absolute_expires_at"`
	IdleExpiresAt      time.Time     `json:"idle_expires_at"`
	FreshAuthAt        *time.Time    `json:"fresh_auth_at"`
	FreshAuthMethod    string        `json:"fresh_auth_method"`
	CSRFToken          string        `json:"csrf_token"`
	Confirmation       *Confirmation `json:"confirmation,omitempty"`
}
type Fault struct {
	Status      int
	Code        string
	ClearCookie bool
}

func (f Fault) Error() string { return f.Code }

var (
	inputFault       = Fault{400, "SESSION_INPUT_INVALID", false}
	configFault      = Fault{500, "SESSION_CONFIG_INVALID", false}
	authFault        = Fault{401, "SESSION_UNAUTHORIZED", true}
	unavailableFault = Fault{503, "SESSION_UNAVAILABLE", false}
	conflictFault    = Fault{409, "SESSION_CONFLICT", false}
	csrfFault        = Fault{403, "SESSION_CSRF_FAILED", false}
	principalFault   = Fault{403, "OPS_PRINCIPAL_REJECTED", false}
	proofFault       = Fault{401, "OPS_PROOF_REJECTED", false}
	consumedFault    = Fault{409, "OPS_FLOW_CONSUMED", false}
)

type Service struct {
	options     Options
	native, ops *http.Transport
	reader      *authbridge.NativeReader
	authority   *authbridge.Authority
	seal        cipher.AEAD
}
type Grant struct {
	owner *Service
	view  View
	sid   string
}
type RequestSession struct {
	owner  *Service
	view   View
	hash   string
	unsafe bool
}

func (NativeCredential) String() string   { return "NativeCredential([redacted])" }
func (NativeCredential) GoString() string { return "NativeCredential([redacted])" }
func (Service) String() string            { return "Service([redacted])" }
func (Service) GoString() string          { return "Service([redacted])" }
func (Grant) String() string              { return "Grant([redacted])" }
func (Grant) GoString() string            { return "Grant([redacted])" }
func (RequestSession) String() string     { return "RequestSession([redacted])" }
func (RequestSession) GoString() string   { return "RequestSession([redacted])" }
func (g Grant) View() View                { return copyView(g.view) }
func (r RequestSession) View() View       { return copyView(r.view) }
func copyView(v View) View {
	if v.FreshAuthAt != nil {
		at := *v.FreshAuthAt
		v.FreshAuthAt = &at
	}
	if v.Confirmation != nil {
		c := *v.Confirmation
		v.Confirmation = &c
	}
	return v
}
func (o Options) valid() bool {
	origin, err := url.Parse(o.Origin)
	if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil ||
		origin.Path != "" || origin.RawQuery != "" || origin.ForceQuery || origin.Fragment != "" ||
		origin.String() != o.Origin || !wireName.MatchString(o.Environment) {
		return false
	}
	if !filepath.IsAbs(o.NativeSocket) || !filepath.IsAbs(o.OpsSocket) || o.NativeSocket == o.OpsSocket ||
		filepath.Clean(o.NativeSocket) != o.NativeSocket || filepath.Clean(o.OpsSocket) != o.OpsSocket ||
		!digest(o.NativeReaderKey) || !digest(o.OpsKey) || o.NativeReaderKey == o.OpsKey ||
		o.SessionSealKey == [32]byte{} {
		return false
	}
	if equalHex(o.SessionSealKey[:], o.NativeReaderKey) || equalHex(o.SessionSealKey[:], o.OpsKey) {
		return false
	}
	l := o.Lifetime
	return o.Pool != nil && validRedis(o.Redis) && l.Touch >= time.Millisecond && l.Touch < l.Idle && l.Idle <= l.Absolute
}
func (r record) valid(hash string, now int64) bool {
	if r.Schema != 1 || r.State != "AUTHENTICATED" || r.Hash != hash || !digest(hash) ||
		r.AccountStatus != "ACTIVE" || r.Revoked != nil || r.LoginMethod != "NATIVE_SESSION" ||
		!decimal(r.UserID, false) || !decimal(r.SecurityEpoch, true) || !decimal(r.Version, false) ||
		!decimal(r.NativeSV, false) || !decimal(r.NativeUV, false) || !digest(r.NativeHash) || !opaque(r.CSRF) {
		return false
	}
	for _, stamp := range []string{r.Created, r.ChainStart, r.LastSeen, r.Idle, r.Absolute, r.Authenticated, r.Checked, r.NativeCreated, r.NativeExpires} {
		if milliseconds(stamp) <= 0 {
			return false
		}
	}
	if r.ChainStart != r.NativeCreated || r.Authenticated != r.ChainStart ||
		milliseconds(r.ChainStart) > milliseconds(r.Created) || milliseconds(r.Created) > milliseconds(r.LastSeen) ||
		milliseconds(r.LastSeen) > now || milliseconds(r.Checked) > now ||
		milliseconds(r.Idle) <= now || milliseconds(r.Absolute) < milliseconds(r.Idle) ||
		milliseconds(r.NativeExpires) < milliseconds(r.Absolute) {
		return false
	}
	if r.Fresh == nil {
		return r.FreshMethod == "" && r.Confirmation == nil && r.validPending()
	}
	if milliseconds(*r.Fresh) < milliseconds(r.ChainStart) || milliseconds(*r.Fresh) > milliseconds(r.Created) ||
		(r.FreshMethod != "password" && r.FreshMethod != "discord") || r.Confirmation == nil {
		return false
	}
	return canonicalUUID(r.Confirmation.OperationID) && digest(r.Confirmation.ContextHash) &&
		digest(r.Confirmation.SourceSessionHash) && r.validPending()
}
func sameSecret(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

func milliseconds(v string) int64 {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 || n > 253402300799999 || strconv.FormatInt(n, 10) != v {
		return 0
	}
	return n
}
func ms(v int64) string          { return strconv.FormatInt(v, 10) }
func instant(v string) time.Time { return time.UnixMilli(milliseconds(v)).UTC() }
