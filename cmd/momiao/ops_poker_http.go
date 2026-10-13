package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/poker"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const opsPokerPath = "/api/v1/ops/poker"

func opsPokerRoute(path string) bool {
	return path == opsPokerPath || strings.HasPrefix(path, opsPokerPath+"/")
}
func newOpsPokerHandler(service PokerOpsPort) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
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
		path := strings.TrimPrefix(r.URL.Path, opsPokerPath)
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		var result any
		var err error
		switch {
		case path == "" && r.Method == "GET":
			result, err = service.ReadOpsOverview(ctx)
		case (path == "/tables" || path == "/sessions") && r.Method == "GET":
			q := r.URL.Query()
			limit := 50
			if value := q.Get("limit"); value != "" {
				limit, err = strconv.Atoi(value)
			}
			cursor := q.Get("cursor")
			if err != nil || limit < 1 || limit > 100 || cursor != "" && !opsUUIDPatternHTTP(cursor) {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			var from, to *time.Time
			for key, target := range map[string]**time.Time{"from": &from, "to": &to} {
				if value := q.Get(key); value != "" {
					date, e := time.Parse(time.RFC3339, value)
					if e != nil {
						walletError(w, 400, "OPS_INPUT_INVALID")
						return
					}
					*target = &date
				}
			}
			if from != nil && to != nil && !to.After(*from) {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			if path == "/tables" {
				result, err = service.ListOpsTables(ctx, poker.OpsTableQuery{Cursor: cursor, Limit: limit, Search: q.Get("q"), State: q.Get("state"), From: from, To: to})
			} else {
				var userID int64
				if value := q.Get("user_id"); value != "" {
					userID, err = strconv.ParseInt(value, 10, 64)
					if err != nil || userID < 1 {
						walletError(w, 400, "OPS_INPUT_INVALID")
						return
					}
				}
				result, err = service.ListOpsSessions(ctx, poker.OpsSessionQuery{Cursor: cursor, Limit: limit, UserID: userID, State: q.Get("state"), TableID: q.Get("table_id"), From: from, To: to})
			}
		case len(parts) == 2 && opsUUIDPatternHTTP(parts[1]):
			switch {
			case parts[0] == "tables" && r.Method == "GET":
				result, err = service.ReadOpsTable(ctx, parts[1])
			case parts[0] == "sessions" && r.Method == "GET":
				result, err = service.ReadOpsSession(ctx, parts[1])
			case parts[0] == "hands" && r.Method == "GET":
				result, err = service.ReadOpsHand(ctx, parts[1])
			case parts[0] == "tables" && r.Method == "POST":
				var command poker.OpsCommand
				if json.NewDecoder(r.Body).Decode(&command) != nil {
					walletError(w, 400, "OPS_INPUT_INVALID")
					return
				}
				command.ActorUserID, command.TableID = user.ID, parts[1]
				result, err = service.ExecuteOpsCommand(ctx, command)
			default:
				walletError(w, 404, "NOT_FOUND")
				return
			}
		default:
			walletError(w, 404, "NOT_FOUND")
			return
		}
		switch {
		case err == nil:
			walletSuccess(w, result)
		case errors.Is(err, platform.ErrCatalogForbidden):
			walletError(w, 403, "OPS_FORBIDDEN")
		case errors.Is(err, poker.ErrInvalid):
			walletError(w, 400, "OPS_INPUT_INVALID")
		case errors.Is(err, errPokerOpsNotFound), errors.Is(err, pgx.ErrNoRows), errors.Is(err, poker.ErrDenied):
			walletError(w, 404, "NOT_FOUND")
		default:
			pokerOpsRPCResult(w, nil, err, false)
		}
	})
}
