package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
)

func newRPUsageHandler(cfg config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.URL.Path != "/api/v1/usage/rp" || r.URL.RawPath != "" {
			walletError(w, 404, "NOT_FOUND")
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			walletError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		user, ok := identity.UserFromContext(r.Context())
		if !ok {
			walletError(w, 401, "AUTH_UNAUTHORIZED")
			return
		}
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(r.URL.RawQuery) > 2048 || r.URL.ForceQuery {
			walletError(w, 400, "USAGE_QUERY_INVALID")
			return
		}
		for key, values := range query {
			if len(values) != 1 {
				walletError(w, 400, "USAGE_QUERY_INVALID")
				return
			}
			switch key {
			case "model", "status", "from", "to", "page":
			default:
				walletError(w, 400, "USAGE_QUERY_INVALID")
				return
			}
		}
		filter := platform.RPUsageFilter{ActivationTime: cfg.RankingActivationTime, ModelID: query.Get("model"), Status: query.Get("status"), Page: 1, PageSize: 50}
		if value := query.Get("page"); value != "" {
			filter.Page, err = strconv.Atoi(value)
			if err != nil {
				walletError(w, 400, "USAGE_QUERY_INVALID")
				return
			}
		}
		for _, bound := range []struct {
			key    string
			target *time.Time
		}{{"from", &filter.From}, {"to", &filter.To}} {
			if value := query.Get(bound.key); value != "" {
				parsed, e := time.Parse(time.RFC3339, value)
				if e != nil {
					walletError(w, 400, "USAGE_QUERY_INVALID")
					return
				}
				*bound.target = parsed.UTC()
			}
		}
		if cfg.rpUsage == nil || cfg.RankingActivationTime.IsZero() || cfg.RankingActivationTime.After(time.Now()) {
			walletError(w, 503, "RP_USAGE_UNAVAILABLE")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		result, err := cfg.rpUsage.ReadRPUsage(ctx, user.ID, filter)
		if err != nil {
			if errors.Is(err, platform.ErrInvalidPage) {
				walletError(w, 400, "USAGE_QUERY_INVALID")
			} else {
				walletError(w, 503, "RP_USAGE_UNAVAILABLE")
			}
			return
		}
		walletSuccess(w, struct {
			platform.RPUsagePage
			ObservedAt time.Time `json:"observed_at"`
		}{result, time.Now().UTC()})
	})
}
