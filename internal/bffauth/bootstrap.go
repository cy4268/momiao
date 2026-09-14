package bffauth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/cy4268/momiao/internal/session"
)

type Identity struct {
	NewAPIUserID string `json:"newapi_user_id"`
}

type AuthenticatedSession struct {
	AuthChainStartedAt time.Time `json:"auth_chain_started_at"`
	// AuthChainID is a non-authoritative browser boundary marker, never a credential.
	AuthChainID       string     `json:"auth_chain_id"`
	AbsoluteExpiresAt time.Time  `json:"absolute_expires_at"`
	IdleExpiresAt     time.Time  `json:"idle_expires_at"`
	FreshAuthAt       *time.Time `json:"fresh_auth_at"`
	FreshAuthMethod   string     `json:"fresh_auth_method"`
}

type Bootstrap struct {
	AuthenticationState string                `json:"authentication_state"`
	CSRFToken           string                `json:"csrf_token"`
	AnonymousExpiresAt  *time.Time            `json:"anonymous_expires_at,omitempty"`
	Identity            *Identity             `json:"identity,omitempty"`
	Session             *AuthenticatedSession `json:"session,omitempty"`
	Password            PasswordProvider      `json:"password"`
	Unavailable         []string              `json:"unavailable"`
	Cookie              string                `json:"-"`
}

type LoginResult struct {
	AuthenticationState string          `json:"authentication_state"`
	CSRFToken           string          `json:"csrf_token"`
	Challenge           *LoginChallenge `json:"challenge,omitempty"`
	Grant               *session.Grant  `json:"-"`
}

func mapSessionError(err error) error {
	var fault session.Fault
	if !errors.As(err, &fault) {
		return errUnavailable
	}
	return Fault{Status: fault.Status, Code: fault.Code, ClearCookie: fault.ClearCookie}
}

func isSessionUnauthorized(err error) bool {
	var fault session.Fault
	return errors.As(err, &fault) && fault.Code == "SESSION_UNAUTHORIZED"
}

func (s *Service) bootstrapUnavailable() []string {
	unavailable := make([]string, 0, 2)
	if s == nil || s.accounts == nil {
		unavailable = append(unavailable, "DISCORD_S2")
	}
	if s == nil || s.control == nil {
		unavailable = append(unavailable, "POKER_LOGOUT_S2")
	}
	return unavailable
}

func (s *Service) authenticatedBootstrap(view session.View, binding session.BFFBinding, provider PasswordProvider, cookie string) (Bootstrap, error) {
	if view.UserID != binding.UserID {
		return Bootstrap{}, errUnauthorized
	}
	chainID, err := s.authChainID(binding)
	if err != nil {
		return Bootstrap{}, err
	}
	return Bootstrap{
		AuthenticationState: "AUTHENTICATED",
		CSRFToken:           view.CSRFToken,
		Identity:            &Identity{NewAPIUserID: view.UserID},
		Session: &AuthenticatedSession{
			AuthChainStartedAt: view.AuthChainStartedAt,
			AuthChainID:        chainID,
			AbsoluteExpiresAt:  view.AbsoluteExpiresAt,
			IdleExpiresAt:      view.IdleExpiresAt,
			FreshAuthAt:        view.FreshAuthAt,
			FreshAuthMethod:    view.FreshAuthMethod,
		},
		Password:    provider,
		Unavailable: s.bootstrapUnavailable(),
		Cookie:      cookie,
	}, nil
}

func (s *Service) verifiedPlatform(ctx context.Context, rawSID, csrf, method string) (session.RequestSession, error) {
	request, err := http.NewRequestWithContext(ctx, method, s.origin+"/api/v1/session/bootstrap", nil)
	if err != nil {
		return session.RequestSession{}, errInput
	}
	request.Header.Set("Cookie", CookieName+"="+rawSID)
	if method != http.MethodGet && method != http.MethodHead {
		request.Header.Set("Origin", s.origin)
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		request.Header.Set("X-CSRF-Token", csrf)
	}
	return s.sessions.VerifyRequest(request)
}

func (s *Service) Bootstrap(r *http.Request) (Bootstrap, error) {
	if s == nil || r == nil || r.Method != http.MethodGet {
		return Bootstrap{}, errInput
	}
	provider := s.passwordProvider(r.Context())
	sid, cookieErr := cookieID(r)
	if errors.Is(cookieErr, errMissing) {
		cookie, view, err := s.NewAnonymous(r.Context())
		if err != nil {
			return Bootstrap{}, err
		}
		expires := view.ExpiresAt
		return Bootstrap{AuthenticationState: view.State, CSRFToken: view.CSRFToken, AnonymousExpiresAt: &expires, Password: provider, Unavailable: s.bootstrapUnavailable(), Cookie: cookie}, nil
	}
	if cookieErr != nil {
		return Bootstrap{}, cookieErr
	}
	verified, sessionErr := s.sessions.VerifyRequest(r)
	if sessionErr == nil {
		binding, err := s.sessions.BFFBinding(r.Context(), verified)
		if err != nil {
			return Bootstrap{}, mapSessionError(err)
		}
		if _, err = s.ensureCredential(r.Context(), binding); err != nil {
			return Bootstrap{}, err
		}
		return s.authenticatedBootstrap(verified.View(), binding, provider, "")
	}
	if !isSessionUnauthorized(sessionErr) {
		return Bootstrap{}, mapSessionError(sessionErr)
	}
	preauth, err := s.loadPreauth(r.Context(), sid)
	if err != nil {
		return Bootstrap{}, err
	}
	if preauth.State == "COMMITTED" {
		var upgrade struct {
			SID string `json:"sid"`
		}
		if s.openJSON("upgrade:1:"+preauth.Hash, preauth.PlatformSessionBox, &upgrade) != nil || !opaque(upgrade.SID) {
			return Bootstrap{}, errUnauthorized
		}
		requestSession, err := s.verifiedPlatform(r.Context(), upgrade.SID, preauth.PlatformCSRF, http.MethodGet)
		if err != nil {
			return Bootstrap{}, mapSessionError(err)
		}
		binding, err := s.sessions.BFFBinding(r.Context(), requestSession)
		if err != nil {
			return Bootstrap{}, mapSessionError(err)
		}
		if _, err = s.ensureCredential(r.Context(), binding); err != nil {
			return Bootstrap{}, err
		}
		return s.authenticatedBootstrap(requestSession.View(), binding, provider, upgrade.SID)
	}
	view := preauth.view()
	expires := view.ExpiresAt
	return Bootstrap{AuthenticationState: view.State, CSRFToken: view.CSRFToken, AnonymousExpiresAt: &expires, Password: provider, Unavailable: s.bootstrapUnavailable()}, nil
}

func nativeUnknown(err error) bool {
	var fault Fault
	return errors.As(err, &fault) && fault.Unknown
}

func (s *Service) AuthenticatePassword(r *http.Request, identifier, password, turnstile string) (LoginResult, error) {
	pending, operation, err := s.begin(r, "")
	if err != nil {
		return LoginResult{}, err
	}
	ctx, finish := finalizationContext(r.Context())
	defer finish()
	result, err := s.password(r.Context(), identifier, password, turnstile)
	if err != nil {
		if resolveErr := s.resolve(ctx, pending, operation, nativeUnknown(err)); resolveErr != nil {
			return LoginResult{}, resolveErr
		}
		return LoginResult{}, err
	}
	if result.Challenge != nil {
		challenge, err := s.challenge(ctx, pending, operation, result.Challenge.FlowToken, result.Challenge.ExpiresAt)
		if err != nil {
			return LoginResult{}, err
		}
		return LoginResult{AuthenticationState: "TWO_FA", CSRFToken: pending.CSRF, Challenge: &challenge}, nil
	}
	if result.Credential == nil {
		_ = s.resolve(ctx, pending, operation, true)
		return LoginResult{}, errUnknown
	}
	return s.finishLogin(ctx, pending, operation, *result.Credential)
}

func (s *Service) AuthenticateTwoFA(r *http.Request, challengeID, code string) (LoginResult, error) {
	pending, operation, err := s.begin(r, challengeID)
	if err != nil {
		return LoginResult{}, err
	}
	ctx, finish := finalizationContext(r.Context())
	defer finish()
	flow, err := s.nativeFlow(pending)
	if err != nil {
		_ = s.resolve(ctx, pending, operation, true)
		return LoginResult{}, err
	}
	result, err := s.twoFA(r.Context(), flow, code)
	if err != nil {
		if resolveErr := s.resolve(ctx, pending, operation, nativeUnknown(err)); resolveErr != nil {
			return LoginResult{}, resolveErr
		}
		return LoginResult{}, err
	}
	if result.Credential == nil || result.Challenge != nil {
		_ = s.resolve(ctx, pending, operation, true)
		return LoginResult{}, errUnknown
	}
	return s.finishLogin(ctx, pending, operation, *result.Credential)
}

func (s *Service) finishLogin(ctx context.Context, pending preauthRecord, operation string, credential nativeCredential) (LoginResult, error) {
	userID, username, nativeSID, access := nativeCredentialForAdoption(credential)
	grant, err := s.sessions.AdoptNative(ctx, session.NativeCredential{UserID: userID, Username: username, SID: nativeSID, AccessToken: access})
	if err != nil {
		_ = s.resolve(ctx, pending, operation, true)
		_ = s.nativeLogout(ctx, credential)
		return LoginResult{}, mapSessionError(err)
	}
	nativeSessionHash := nativeHash(credential)
	if err = s.storeCredential(ctx, nativeSessionHash, credential, grant.View().AbsoluteExpiresAt); err != nil {
		s.discardGrant(ctx, grant)
		_ = s.resolve(ctx, pending, operation, true)
		_ = s.nativeLogout(ctx, credential)
		return LoginResult{}, err
	}
	if _, err = s.commit(ctx, pending, operation, nativeSessionHash, grant); err != nil {
		s.discardGrant(ctx, grant)
		s.orphanCredential(ctx, nativeSessionHash)
		_ = s.nativeLogout(ctx, credential)
		return LoginResult{}, err
	}
	copy := grant
	return LoginResult{AuthenticationState: "AUTHENTICATED", CSRFToken: grant.View().CSRFToken, Grant: &copy}, nil
}

func (s *Service) discardGrant(ctx context.Context, grant session.Grant) {
	w := &headerWriter{header: make(http.Header)}
	if s.sessions.WriteGrant(w, grant) != nil {
		return
	}
	cookies := (&http.Response{Header: w.header}).Cookies()
	if len(cookies) != 1 {
		return
	}
	requestSession, err := s.verifiedPlatform(ctx, cookies[0].Value, grant.View().CSRFToken, http.MethodPost)
	if err == nil {
		_ = s.sessions.Revoke(ctx, requestSession)
	}
}
