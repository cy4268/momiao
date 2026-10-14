package main

import (
	"net/http"
	"strings"

	"github.com/cy4268/momiao/internal/identity"
)

// Local domain handlers receive the verified user in context, not native headers.
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
		prepared, err := cfg.localIdentity.Request(r, optional)
		if err != nil {
			identity.WriteError(w, err)
			return
		}
		if game {
			// The game's existing commitment token is separate from login CSRF.
			prepared.Header.Set("X-CSRF-Token", r.Header.Get("X-Game-CSRF-Token"))
		}
		next.ServeHTTP(w, prepared)
	})
}
