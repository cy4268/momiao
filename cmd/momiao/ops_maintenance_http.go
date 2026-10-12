package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
)

func newOpsMaintenanceHandler(store *platform.Store, environment string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := identity.UserFromContext(r.Context())
		if !ok {
			walletError(w, 401, "AUTH_UNAUTHORIZED")
			return
		}
		if user.Role < 10 {
			walletError(w, 403, "OPS_FORBIDDEN")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		var value platform.MaintenanceSettings
		var err error
		switch r.Method {
		case http.MethodGet:
			value, err = store.ReadMaintenance(ctx, environment)
		case http.MethodPut:
			var input struct {
				Enabled *bool    `json:"enabled"`
				Message string   `json:"message"`
				Scopes  []string `json:"scopes"`
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil || input.Enabled == nil {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			value = platform.MaintenanceSettings{Enabled: *input.Enabled, Message: input.Message, Scopes: input.Scopes}
			if platform.ValidateMaintenance(&value) != nil {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			value, err = store.SaveMaintenance(ctx, user.ID, environment, value)
		default:
			w.Header().Set("Allow", "GET, PUT")
			walletError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		if err != nil {
			writeOpsError(w, err)
			return
		}
		walletSuccess(w, value)
	})
}
