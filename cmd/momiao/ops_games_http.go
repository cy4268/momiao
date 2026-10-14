package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
)

const opsGamesPath = "/api/v1/ops/games"

func opsGamesRoute(path string) bool {
	return path == opsGamesPath || strings.HasPrefix(path, opsGamesPath+"/")
}
func newOpsGamesHandler(service *games.Service) http.Handler {
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
		path := strings.TrimPrefix(r.URL.Path, opsGamesPath)
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		var result any
		var err error
		switch {
		case path == "" && r.Method == "GET":
			result, err = service.ReadOps(ctx)
		case len(parts) == 1 && r.Method == "GET":
			filter := games.OpsConfigFilter{Page: 1, Status: r.URL.Query().Get("status")}
			if v := r.URL.Query().Get("page"); v != "" {
				filter.Page, err = strconv.Atoi(v)
			}
			if err != nil || filter.Page < 1 || filter.Page > 100000 {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			switch filter.Status {
			case "", "DRAFT", "VALIDATED", "PREVIEWED", "ACTIVE", "SUPERSEDED":
			default:
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			for key, target := range map[string]**time.Time{"from": &filter.From, "to": &filter.To} {
				if v := r.URL.Query().Get(key); v != "" {
					date, e := time.Parse(time.RFC3339, v)
					if e != nil {
						walletError(w, 400, "OPS_INPUT_INVALID")
						return
					}
					*target = &date
				}
			}
			if filter.From != nil && filter.To != nil && !filter.To.After(*filter.From) {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			result, err = service.ReadOpsGame(ctx, parts[0], filter)
		case len(parts) == 1 && r.Method == "PUT":
			var input games.OpsGameInput
			if json.NewDecoder(r.Body).Decode(&input) != nil {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			result, err = service.SaveOpsGame(ctx, user.ID, parts[0], input)
		case (len(parts) == 3 || len(parts) == 4) && parts[1] == "configs":
			if !opsUUIDPatternHTTP(parts[2]) {
				walletError(w, 404, "NOT_FOUND")
				return
			}
			action := "save"
			var input games.OpsEditableConfig
			if len(parts) == 3 && r.Method == "PUT" {
				if json.NewDecoder(r.Body).Decode(&input) != nil {
					walletError(w, 400, "OPS_INPUT_INVALID")
					return
				}
			} else if len(parts) == 4 && r.Method == "POST" && (parts[3] == "clone" || parts[3] == "validate" || parts[3] == "activate") {
				action = parts[3]
			} else {
				walletError(w, 404, "NOT_FOUND")
				return
			}
			result, err = service.ChangeOpsConfig(ctx, user.ID, parts[0], parts[2], action, input)
		case len(parts) == 4 && parts[1] == "rounds" && parts[3] == "void" && r.Method == "POST":
			if !opsUUIDPatternHTTP(parts[2]) {
				walletError(w, 404, "NOT_FOUND")
				return
			}
			err = service.VoidOpsRound(ctx, user.ID, parts[0], parts[2])
			result = map[string]string{"round_id": parts[2], "state": "CANCELLED"}
		default:
			walletError(w, 404, "NOT_FOUND")
			return
		}
		if errors.Is(err, platform.ErrCatalogForbidden) {
			walletError(w, 403, "OPS_FORBIDDEN")
			return
		}
		if errors.Is(err, games.ErrNotFound) {
			walletError(w, 404, "NOT_FOUND")
			return
		}
		if err != nil {
			writeOpsError(w, err)
			return
		}
		walletSuccess(w, result)
	})
}
