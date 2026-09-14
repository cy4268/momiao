package bffauth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

var domainHeaders = [...]string{"Accept", "Content-Type", "Origin", "Sec-Fetch-Site", "Idempotency-Key", "X-Fairness-Commitment"}

// DomainRequest is only for an already-selected local domain handler, never a
// generic Native proxy. Its result contains server credentials: do not log,
// serialize, return to the browser, or use it for a browser-selected upstream.
// Route selection, response DTOs and additional Ops/Fresh gates remain callers'
// responsibilities. No business call or retry occurs here.
func (s *Service) DomainRequest(r *http.Request, game bool) (*http.Request, error) {
	if err := domainInput(r, game); err != nil {
		return nil, err
	}
	if s == nil || s.sessions == nil || s.redis == nil {
		return nil, errUnavailable
	}
	verified, err := s.sessions.VerifyRequest(r)
	if err != nil {
		return nil, mapSessionError(err)
	}
	if err = domainSessionCSRF(r, verified.View().CSRFToken); err != nil {
		return nil, err
	}
	binding, err := s.sessions.BFFBinding(r.Context(), verified)
	if err != nil {
		return nil, mapSessionError(err)
	}
	credential, err := s.ensureCredential(r.Context(), binding)
	if err != nil {
		return nil, err
	}
	// Refresh can yield to logout, epoch changes or another operation. Recheck
	// the live platform/Native binding before projecting the resolved credential.
	current, err := s.sessions.BFFBinding(r.Context(), verified)
	if err != nil {
		return nil, mapSessionError(err)
	}
	if current != binding {
		return nil, errUnauthorized
	}
	if r.Context().Err() != nil {
		return nil, errUnavailable
	}
	return projectDomainRequest(r, credential, game), nil
}

var errDomainSessionCSRF = Fault{Status: http.StatusForbidden, Code: "SESSION_CSRF_FAILED"}

func domainSessionCSRF(r *http.Request, want string) error {
	if r == nil {
		return errDomainSessionCSRF
	}
	values := r.Header.Values("X-CSRF-Token")
	if len(values) != 1 || subtle.ConstantTimeCompare([]byte(values[0]), []byte(want)) != 1 {
		return errDomainSessionCSRF
	}
	return nil
}

func domainInput(r *http.Request, game bool) error {
	if r == nil || r.URL == nil {
		return errInput
	}
	for key, values := range r.Header {
		if http.CanonicalHeaderKey(key) != key {
			return errInput
		}
		switch strings.ToLower(key) {
		case "authorization", "new-api-user", "x-auth-session", "upgrade", "proxy-authorization":
			return errInput
		case "connection":
			if len(values) != 1 || (!strings.EqualFold(strings.TrimSpace(values[0]), "keep-alive") && !strings.EqualFold(strings.TrimSpace(values[0]), "close")) {
				return errInput
			}
		case "origin", "sec-fetch-site", "x-csrf-token", "x-game-csrf-token", "accept", "content-type", "idempotency-key", "x-fairness-commitment":
			if len(values) != 1 || values[0] == "" || strings.ContainsAny(values[0], "\r\n\x00") {
				return errInput
			}
		}
	}
	for _, raw := range r.Header.Values("Cookie") {
		cookies, err := http.ParseCookie(raw)
		if err != nil {
			return errInput
		}
		for _, cookie := range cookies {
			if cookie.Name == "new_api_refresh" {
				return errInput
			}
		}
	}
	write := r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" || r.Method == "DELETE"
	if game && write {
		if !digest(r.Header.Get("X-Game-CSRF-Token")) {
			return errInput
		}
	} else if len(r.Header.Values("X-Game-CSRF-Token")) != 0 {
		return errInput
	}
	return nil
}

func cleanDomainRequest(r *http.Request) *http.Request {
	out := r.Clone(r.Context())
	out.Header = make(http.Header)
	out.Trailer = nil
	out.Host = ""
	for _, key := range domainHeaders {
		if values := r.Header.Values(key); len(values) == 1 {
			out.Header.Set(key, values[0])
		}
	}
	return out
}

func projectDomainRequest(r *http.Request, c nativeCredential, game bool) *http.Request {
	out := cleanDomainRequest(r)
	out.Header.Set("Authorization", "Bearer "+c.AccessToken)
	out.Header.Set("New-Api-User", c.UserID)
	out.Header.Set("X-Auth-Session", c.SID)
	if game && r.Method != "GET" && r.Method != "HEAD" {
		out.Header.Set("X-CSRF-Token", r.Header.Get("X-Game-CSRF-Token"))
	}
	return out
}
