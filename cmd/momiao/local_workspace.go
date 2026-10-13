package main

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
)

func localWorkspaceRoute(path string) bool {
	return path == "/api/log/self" || path == "/api/user/self/groups" || path == "/api/user/models"
}
func newLocalWorkspaceHandler(store *platform.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			walletError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		user, ok := identity.UserFromContext(r.Context())
		if !ok {
			walletError(w, 401, "AUTH_UNAUTHORIZED")
			return
		}
		if store == nil {
			walletError(w, 503, "RELAY_UNAVAILABLE")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		var data any
		var err error
		switch r.URL.Path {
		case "/api/user/self/groups":
			data, err = store.ReadRelayGroup(ctx, user.Group)
		case "/api/user/models":
			if group := r.URL.Query().Get("group"); group != "" && group != user.Group {
				walletError(w, 403, "GROUP_FORBIDDEN")
				return
			}
			data, err = store.RelayModels(ctx, user.Group)
		case "/api/log/self":
			query := r.URL.Query()
			f := platform.RelayLogFilter{Page: 1, PageSize: 10, Model: query.Get("model_name")}
			for name, dest := range map[string]*int64{"p": &f.Page, "page_size": &f.PageSize, "type": &f.Type, "start_timestamp": &f.Start, "end_timestamp": &f.End} {
				if raw := query.Get(name); raw != "" {
					*dest, err = strconv.ParseInt(raw, 10, 64)
					if err != nil {
						walletError(w, 400, "INVALID_REQUEST")
						return
					}
				}
			}
			if f.Page < 1 || f.PageSize < 1 || f.PageSize > 100 || f.Page-1 > math.MaxInt64/f.PageSize || f.Type < 0 || f.Type > 7 || f.Start < 0 || f.End < 0 || f.Start > 253402300799 || f.End > 253402300799 || (f.End != 0 && f.Start > f.End) || utf8.RuneCountInString(f.Model) > 200 {
				walletError(w, 400, "INVALID_REQUEST")
				return
			}
			// Official explicit-% search; '_' remains a literal model-name character.
			if strings.Contains(f.Model, "%") && (strings.Contains(f.Model, "%%") || strings.Count(f.Model, "%") > 2 || len(strings.ReplaceAll(f.Model, "%", "")) < 2) {
				walletError(w, 400, "INVALID_REQUEST")
				return
			}
			data, err = store.ListRelayLogs(ctx, user.ID, f)
		}
		if err != nil {
			walletError(w, 503, "RELAY_UNAVAILABLE")
			return
		}
		walletSuccess(w, data)
	})
}
