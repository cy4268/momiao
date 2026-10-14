package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/rankings"
)

const opsRankingsPath = "/api/v1/ops/rankings"

func opsRankingsRoute(path string) bool {
	return path == opsRankingsPath || strings.HasPrefix(path, opsRankingsPath+"/")
}

func newOpsRankingsHandler(service *rankings.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		user, ok := identity.UserFromContext(r.Context())
		if !ok {
			walletError(w, 401, "AUTH_UNAUTHORIZED")
			return
		}
		if user.Role < 10 {
			walletError(w, 403, "OPS_FORBIDDEN")
			return
		}
		if service == nil {
			walletError(w, 503, "OPS_UNAVAILABLE")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		var result any
		var err error
		path := strings.TrimPrefix(r.URL.Path, opsRankingsPath)
		switch {
		case path == "" && r.Method == "GET":
			q := r.URL.Query()
			filter := rankings.OpsFilter{Domain: q.Get("domain"), Page: 1}
			if value := q.Get("page"); value != "" {
				filter.Page, err = strconv.Atoi(value)
			}
			if err != nil || filter.Page < 1 || filter.Page > 100000 || (filter.Domain != "" && filter.Domain != "GAMES" && filter.Domain != "RP" && filter.Domain != "ASSETS") {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			for key, target := range map[string]**time.Time{"from": &filter.From, "to": &filter.To} {
				if value := q.Get(key); value != "" {
					v, e := time.Parse(time.RFC3339, value)
					if e != nil {
						walletError(w, 400, "OPS_INPUT_INVALID")
						return
					}
					*target = &v
				}
			}
			if filter.From != nil && filter.To != nil && !filter.To.After(*filter.From) {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			result, err = service.ReadOpsOverview(ctx, filter)
		case path == "/rebuilds" && r.Method == "POST":
			var input rankings.OpsRebuildInput
			if json.NewDecoder(r.Body).Decode(&input) != nil {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			result, err = service.CreateRebuild(ctx, user.ID, input)
		case strings.HasPrefix(path, "/snapshots/"):
			parts := strings.Split(strings.TrimPrefix(path, "/snapshots/"), "/")
			if !opsUUIDPatternHTTP(parts[0]) {
				walletError(w, 404, "NOT_FOUND")
				return
			}
			if len(parts) == 1 && r.Method == "GET" {
				result, err = service.ReadOpsSnapshot(ctx, parts[0])
			} else if len(parts) == 2 && parts[1] == "publish" && r.Method == "POST" {
				err = service.PublishRepair(ctx, user.ID, parts[0])
				result = map[string]string{"snapshot_id": parts[0], "state": "PUBLISHED"}
			} else {
				walletError(w, 404, "NOT_FOUND")
				return
			}
		default:
			walletError(w, 404, "NOT_FOUND")
			return
		}
		if errors.Is(err, platform.ErrCatalogForbidden) {
			walletError(w, 403, "OPS_FORBIDDEN")
			return
		}
		if err != nil {
			writeOpsError(w, err)
			return
		}
		walletSuccess(w, result)
	})
}
