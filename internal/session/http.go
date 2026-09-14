package session

import (
	"context"
	"net/http"
	"strings"
	"time"
)

const cookieName = "__Host-chaldea_session"

// VerifyPrivateRequest also binds reads to the identity observed by this tab.
// A shared cookie may have changed after another tab signed in or rotated it.
func (s *Service) VerifyPrivateRequest(request *http.Request) (RequestSession, error) {
	verified, err := s.VerifyRequest(request)
	if err != nil {
		return RequestSession{}, err
	}
	if len(request.Header.Values("X-CSRF-Token")) != 1 || !sameSecret(verified.View().CSRFToken, request.Header.Get("X-CSRF-Token")) {
		return RequestSession{}, csrfFault
	}
	return verified, nil
}

func (s *Service) VerifyRequest(request *http.Request) (RequestSession, error) {
	if s == nil || s.authority == nil || request == nil {
		return RequestSession{}, inputFault
	}
	unsafe := request.Method == "POST" || request.Method == "PUT" || request.Method == "PATCH" || request.Method == "DELETE"
	if !unsafe && request.Method != "GET" && request.Method != "HEAD" {
		return RequestSession{}, inputFault
	}
	if unsafe && (!oneHeader(request, "Origin", s.options.Origin) || !oneHeader(request, "Sec-Fetch-Site", "same-origin") ||
		len(request.Header.Values("X-CSRF-Token")) != 1) {
		return RequestSession{}, csrfFault
	}
	sid, count := "", 0
	for _, header := range request.Header.Values("Cookie") {
		for _, part := range strings.Split(header, ";") {
			name, value, _ := strings.Cut(strings.TrimSpace(part), "=")
			if name == cookieName {
				count++
				sid = value
			}
		}
	}
	if count != 1 || !opaque(sid) {
		return RequestSession{}, authFault
	}
	ctx, cancel := context.WithTimeout(request.Context(), 8*time.Second)
	defer cancel()
	r, err := s.load(ctx, secretHash(sid))
	if err != nil {
		return RequestSession{}, err
	}
	if unsafe && !sameSecret(r.CSRF, request.Header.Get("X-CSRF-Token")) {
		return RequestSession{}, csrfFault
	}
	if err = s.check(ctx, r); err != nil {
		return RequestSession{}, err
	}
	r, err = s.touch(ctx, r)
	if err != nil {
		return RequestSession{}, err
	}
	return RequestSession{s, r.view(), r.Hash, unsafe}, nil
}
func oneHeader(r *http.Request, key, want string) bool {
	values := r.Header.Values(key)
	return len(values) == 1 && values[0] == want
}
func sessionCookie(value string, age int) *http.Cookie {
	return &http.Cookie{Name: cookieName, Value: value, Path: "/", Secure: true,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: age}
}
func (s *Service) WriteGrant(w http.ResponseWriter, g Grant) error {
	if s == nil || w == nil || g.owner != s || !opaque(g.sid) ||
		!g.view.IdleExpiresAt.After(time.Now()) || !g.view.AbsoluteExpiresAt.After(time.Now()) {
		return inputFault
	}
	w.Header().Set("Cache-Control", "no-store")
	http.SetCookie(w, sessionCookie(g.sid, 0))
	return nil
}
func (*Service) ClearCookie(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	http.SetCookie(w, sessionCookie("", -1))
}
func (r record) view() View {
	v := View{UserID: r.UserID, AuthChainStartedAt: instant(r.ChainStart), AbsoluteExpiresAt: instant(r.Absolute),
		IdleExpiresAt: instant(r.Idle), FreshAuthMethod: r.FreshMethod, CSRFToken: r.CSRF, Confirmation: r.Confirmation}
	if r.Fresh != nil {
		at := instant(*r.Fresh)
		v.FreshAuthAt = &at
	}
	return copyView(v)
}
