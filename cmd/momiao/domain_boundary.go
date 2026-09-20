package main

import (
	"net/http"
	"strings"
)

// Only wrap explicitly selected local domain handlers. Never wrap the Native
// proxy, session/History handlers (already F4), or ticket-authenticated sockets.
func domainHandler(cfg config, next http.Handler) http.Handler {
	if next == nil || !cfg.Session.Enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		catalog := p == "/platform/v1/models" || strings.HasPrefix(p, "/platform/v1/models/")
		if p == "/ws/poker" || p == "/platform/v1/admission/config" || p == "/api/v1/games" || catalog && p != "/platform/v1/models/personal-price" {
			next.ServeHTTP(w, r)
			return
		}
		game := p == "/api/v1/game-rounds" || strings.HasPrefix(p, "/api/v1/game-rounds/") || strings.HasPrefix(p, "/api/v1/games/") || p == "/api/v1/roulette" || strings.HasPrefix(p, "/api/v1/roulette/")
		announcement := p == "/platform/v1/announcements" || strings.HasPrefix(p, "/platform/v1/announcements/")
		optional := announcement && r.Method == http.MethodGet && !strings.HasSuffix(p, "/reads") && p != "/platform/v1/announcements/current-post-login-popup"
		var prepared *http.Request
		var err error
		if optional && cfg.sessionBridge != nil {
			prepared, err = cfg.sessionBridge.OptionalDomainRequest(r)
		} else {
			prepared, err = cfg.sessionBridge.DomainRequest(r, game)
		}
		if err != nil {
			sessionError(w, err)
			return
		}
		next.ServeHTTP(w, prepared)
	})
}
