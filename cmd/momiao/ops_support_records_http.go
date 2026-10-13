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
)

const (
	opsUsersPath        = "/api/v1/ops/users"
	opsSupportCasesPath = "/api/v1/ops/support-cases"
	opsIncidentsPath    = "/api/v1/ops/incidents"
)

func opsSupportRecordsRoute(path string) bool {
	for _, prefix := range []string{opsSupportCasesPath, opsIncidentsPath} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
func opsOptionalSubject(value string) (int64, bool) {
	if value == "" {
		return 0, true
	}
	return opsEpoch(value)
}
func newOpsSupportRecordsHandler(store *platform.Store) http.Handler {
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
		if store == nil {
			walletError(w, 503, "OPS_UNAVAILABLE")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		support := r.URL.Path == opsSupportCasesPath || strings.HasPrefix(r.URL.Path, opsSupportCasesPath+"/")
		root := opsIncidentsPath
		if support {
			root = opsSupportCasesPath
		}
		path := strings.TrimPrefix(r.URL.Path, root)
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		var result any
		var err error
		switch {
		case path == "" && r.Method == "GET":
			values := r.URL.Query()
			page := 1
			if v := values.Get("page"); v != "" {
				page, err = strconv.Atoi(v)
			}
			if err != nil || page < 1 || page > 100000 {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			subject, valid := opsOptionalSubject(values.Get("subject_newapi_user_id"))
			if !valid {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			var from, to time.Time
			for key, target := range map[string]*time.Time{"from": &from, "to": &to} {
				if v := values.Get(key); v != "" {
					*target, err = time.Parse(time.RFC3339, v)
					if err != nil {
						walletError(w, 400, "OPS_INPUT_INVALID")
						return
					}
				}
			}
			if !from.IsZero() && !to.IsZero() && !to.After(from) {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			if support {
				result, err = store.OpsSupportCases(ctx, platform.OpsSupportCaseQuery{Page: page, SubjectID: subject, State: values.Get("state"), Category: values.Get("category"), Search: values.Get("q"), From: from, To: to})
			} else {
				result, err = store.OpsIncidents(ctx, platform.OpsIncidentQuery{Page: page, State: values.Get("state"), Severity: values.Get("severity"), Search: values.Get("q"), From: from, To: to})
			}
		case path != "" && len(parts) == 1 && r.Method == "GET":
			if !opsUUIDPatternHTTP(parts[0]) {
				walletError(w, 404, "NOT_FOUND")
				return
			}
			if support {
				result, err = store.OpsSupportCaseDetail(ctx, parts[0])
			} else {
				result, err = store.OpsIncidentDetail(ctx, parts[0])
			}
		default:
			id, action := "", ""
			if path == "" && r.Method == "POST" {
				action = "create"
			} else if len(parts) == 1 && r.Method == "PUT" {
				id, action = parts[0], "state"
			} else if len(parts) == 2 && r.Method == "POST" && (support && parts[1] == "facts" || !support && parts[1] == "events") {
				id, action = parts[0], parts[1]
			}
			if action == "" || action != "create" && !opsUUIDPatternHTTP(id) {
				walletError(w, 404, "NOT_FOUND")
				return
			}
			if support {
				var input platform.OpsSupportInput
				if json.NewDecoder(r.Body).Decode(&input) != nil {
					walletError(w, 400, "OPS_INPUT_INVALID")
					return
				}
				result, err = store.SaveOpsSupportCase(ctx, user.ID, id, action, input)
			} else {
				var input platform.OpsIncidentInput
				if json.NewDecoder(r.Body).Decode(&input) != nil {
					walletError(w, 400, "OPS_INPUT_INVALID")
					return
				}
				result, err = store.SaveOpsIncident(ctx, user.ID, id, action, input)
			}
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
