package identity

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// HTTP adapts the official credential flow to momiao's existing route names.
// It neither calls native nor creates or forwards native credentials.
type HTTP struct {
	Store               *Store
	Sessions            Sessions
	Origin              string
	Discord             Discord
	RegistrationEnabled bool
}

type userContextKey struct{}
type requestIdentity struct {
	User      User
	SessionID string
}

func UserFromContext(ctx context.Context) (User, bool) {
	value, ok := ctx.Value(userContextKey{}).(requestIdentity)
	return value.User, ok
}

// SessionIDFromContext is the cookie SHA-256 digest, shared by games and Poker.
// It is not a bearer credential and contains no Redis key prefix.
func SessionIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(userContextKey{}).(requestIdentity)
	return value.SessionID
}

type Fault struct {
	Status int
	Code   string
}

func (f Fault) Error() string { return f.Code }

func WriteError(w http.ResponseWriter, err error) {
	fault := Fault{503, "AUTH_UNAVAILABLE"}
	if errors.Is(err, ErrSessionExpired) || errors.Is(err, ErrCredentials) || errors.Is(err, ErrTwoFALocked) {
		fault = Fault{401, "AUTH_UNAUTHORIZED"}
	}
	var explicit Fault
	if errors.As(err, &explicit) {
		fault = explicit
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(fault.Status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]string{"code": fault.Code}})
}

func writeSuccess(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
}

func (h *HTTP) read(r *http.Request) (Session, User, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return Session{}, User{}, ErrSessionExpired
	}
	value, err := h.Sessions.Read(r.Context(), cookie.Value)
	if err != nil {
		return Session{}, User{}, err
	}
	if value.UserID == 0 {
		return value, User{}, nil
	}
	u, err := h.Store.User(r.Context(), value.UserID)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && u.Status != 1 {
		return Session{}, User{}, ErrSessionExpired
	}
	return value, u, err
}

func (h *HTTP) checkWrite(r *http.Request, value Session) error {
	if r.Header.Get("Origin") != h.Origin || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(value.CSRFToken)) != 1 {
		return Fault{403, "AUTH_FORBIDDEN"}
	}
	return nil
}

// Request is the local business boundary: live user status, write CSRF, then
// a typed context value. Client-supplied native headers never establish identity.
func (h *HTTP) Request(r *http.Request, optional bool) (*http.Request, error) {
	optional = optional && (r.Method == http.MethodGet || r.Method == http.MethodHead)
	value, user, err := h.read(r)
	if optional && errors.Is(err, ErrSessionExpired) {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	if user.ID == 0 && !optional {
		return nil, ErrSessionExpired
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if err := h.checkWrite(r, value); err != nil {
			return nil, err
		}
	}
	ctx := r.Context()
	if user.ID != 0 {
		ctx = context.WithValue(ctx, userContextKey{}, requestIdentity{User: user, SessionID: sessionID(value.Token)})
	}
	r = r.Clone(ctx)
	r.Header.Del("Authorization")
	r.Header.Del("New-Api-User")
	r.Header.Del("X-Auth-Session")
	return r, nil
}

func bootstrap(value Session) map[string]any {
	state := "ANONYMOUS"
	if value.UserID != 0 {
		state = "AUTHENTICATED"
	}
	return map[string]any{"authentication_state": state, "csrf_token": value.CSRFToken}
}

func (h *HTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h.serve(w, r); err != nil {
		WriteError(w, err)
	}
}

func (h *HTTP) serve(w http.ResponseWriter, r *http.Request) error {
	if r.URL.Path == "/api/v1/session/bootstrap" {
		if r.Method != http.MethodGet {
			return Fault{405, "METHOD_NOT_ALLOWED"}
		}
		value, _, err := h.read(r)
		if errors.Is(err, ErrSessionExpired) {
			value, err = h.Sessions.CreateAnonymous(r.Context())
			if err == nil {
				WriteSessionCookie(w, value)
			}
		}
		if err != nil {
			return err
		}
		writeSuccess(w, bootstrap(value))
		return nil
	}
	if r.URL.Path == "/api/v1/account" {
		if r.Method != http.MethodGet {
			return Fault{405, "METHOD_NOT_ALLOWED"}
		}
		_, user, err := h.read(r)
		if err != nil {
			return err
		}
		if user.ID == 0 {
			return ErrSessionExpired
		}
		status, err := h.Store.AccountStatus(r.Context(), user)
		if err != nil {
			return err
		}
		writeSuccess(w, status)
		return nil
	}
	if r.Method != http.MethodPost {
		return Fault{405, "METHOD_NOT_ALLOWED"}
	}
	value, _, err := h.read(r)
	if err != nil {
		return err
	}
	if err := h.checkWrite(r, value); err != nil {
		return err
	}
	if r.URL.Path == "/api/v1/auth/password/login" || r.URL.Path == "/api/v1/auth/password/2fa" {
		if err := h.limitPasswordLogin(w, r); err != nil {
			return err
		}
	}
	switch r.URL.Path {
	case "/api/v1/account/password/change", "/api/v1/account/password/set", "/api/v1/account/password/reset", "/api/v1/account/2fa/setup", "/api/v1/account/2fa/enable", "/api/v1/account/2fa/disable":
		return h.accountWrite(w, r, value)
	case "/api/v1/auth/discord/login/start", "/api/v1/auth/discord/registration/start", "/api/v1/auth/discord/password-set/start", "/api/v1/auth/discord/password-reset/start":
		return h.discordStart(w, r, value)
	case "/api/v1/auth/discord/callback":
		return h.discordCallback(w, r, value)
	case "/api/v1/auth/password/login":
		var body struct {
			Identifier string `json:"identifier"`
			Password   string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return Fault{400, "AUTH_INPUT_INVALID"}
		}
		if value.UserID != 0 {
			return Fault{409, "AUTH_CONFLICT"}
		}
		authenticated, challenge, err := h.Store.PasswordLogin(r.Context(), h.Sessions, value, body.Identifier, body.Password)
		if err != nil {
			return err
		}
		writeLogin(w, value, authenticated, challenge)
	case "/api/v1/auth/password/2fa":
		var body struct {
			ChallengeID string `json:"challenge_id"`
			Code        string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return Fault{400, "AUTH_INPUT_INVALID"}
		}
		if value.UserID != 0 {
			return Fault{409, "AUTH_CONFLICT"}
		}
		authenticated, err := h.Store.SecondFactorLogin(r.Context(), h.Sessions, value, body.ChallengeID, body.Code)
		if err != nil {
			return err
		}
		WriteSessionCookie(w, authenticated)
		writeSuccess(w, bootstrap(authenticated))
	case "/api/v1/session/logout":
		if err := h.Sessions.Revoke(r.Context(), value); err != nil {
			return err
		}
		anonymous, err := h.Sessions.CreateAnonymous(r.Context())
		if err != nil {
			return err
		}
		WriteSessionCookie(w, anonymous)
		writeSuccess(w, bootstrap(anonymous))
	default:
		return Fault{404, "AUTH_ROUTE_NOT_FOUND"}
	}
	return nil
}

func writeLogin(w http.ResponseWriter, browser, authenticated Session, challenge *Challenge) {
	if challenge != nil {
		writeSuccess(w, map[string]any{"authentication_state": "TWO_FA", "csrf_token": browser.CSRFToken, "challenge": challenge})
		return
	}
	WriteSessionCookie(w, authenticated)
	writeSuccess(w, bootstrap(authenticated))
}
