package main

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
)

func opsRuntimeRoute(path string) bool {
	return path == "/api/v1/ops/jobs" || path == "/api/v1/ops/attention" || path == "/api/v1/ops/service-health"
}
func newOpsRuntimeHandler(store *platform.Store, environment string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		user, ok := identity.UserFromContext(r.Context())
		if !ok {
			walletError(w, 401, "AUTH_UNAUTHORIZED")
			return
		}
		if user.Role < 10 {
			walletError(w, 403, "OPS_FORBIDDEN")
			return
		}
		if store == nil {
			walletError(w, 503, "OPS_UNAVAILABLE")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		var result any
		var err error
		if r.URL.Path == "/api/v1/ops/service-health" {
			result, err = store.ReadOpsHealthSnapshot(ctx, environment)
		} else {
			query := r.URL.Query()
			filter := platform.OpsJobsFilter{Page: 1, Kind: query.Get("kind"), State: query.Get("status"), AttentionOnly: r.URL.Path == "/api/v1/ops/attention"}
			if value := query.Get("page"); value != "" {
				filter.Page, err = strconv.Atoi(value)
			}
			if err != nil || filter.Page < 1 || filter.Page > 100000 {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			for key, target := range map[string]*time.Time{"from": &filter.From, "to": &filter.To} {
				if value := query.Get(key); value != "" {
					*target, err = time.Parse(time.RFC3339, value)
					if err != nil {
						walletError(w, 400, "OPS_INPUT_INVALID")
						return
					}
				}
			}
			if !filter.From.IsZero() && !filter.To.IsZero() && !filter.To.After(filter.From) {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			result, err = store.ReadOpsJobs(ctx, filter)
		}
		if err != nil {
			writeOpsError(w, err)
			return
		}
		walletSuccess(w, result)
	})
}
