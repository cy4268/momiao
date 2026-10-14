package relay

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/cy4268/momiao/internal/platform"
)

func ModelsHandler(store *platform.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			writeError(w, 405, "method not allowed")
			return
		}
		key := apiKey(r)
		if key == "" {
			writeError(w, 401, "invalid API key")
			return
		}
		if store == nil {
			writeError(w, 503, "relay unavailable")
			return
		}
		identity, err := store.AuthenticateRelay(r.Context(), key)
		if errors.Is(err, platform.ErrRelayKey) {
			writeError(w, 401, "invalid API key")
			return
		}
		if err != nil {
			writeError(w, 503, "relay unavailable")
			return
		}
		models, err := store.RelayModelsForKey(r.Context(), identity)
		if err != nil {
			writeError(w, 503, "relay unavailable")
			return
		}
		type model struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		}
		items := make([]model, 0, len(models))
		for _, id := range models {
			items = append(items, model{ID: id, Object: "model", OwnedBy: "momiao"})
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(struct {
			Object string  `json:"object"`
			Data   []model `json:"data"`
		}{"list", items})
	})
}

func apiKey(r *http.Request) string {
	key := r.Header.Get("X-API-Key")
	if authorization := r.Header.Get("Authorization"); authorization != "" {
		scheme, value, _ := strings.Cut(authorization, " ")
		if !strings.EqualFold(scheme, "Bearer") {
			return ""
		}
		key = strings.TrimSpace(value)
	}
	return strings.TrimPrefix(key, "sk-")
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": message, "type": "relay_error", "code": http.StatusText(status)}})
}
