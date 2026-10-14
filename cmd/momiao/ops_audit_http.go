package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
)

func newOpsAuditHandler(store *platform.Store) http.Handler {
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
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		q := r.URL.Query()
		filter := platform.OpsAuditFilter{AuditID: q.Get("audit_id"), Action: q.Get("action"), TargetType: q.Get("target_type"), TargetID: q.Get("target_id"), Page: 1, PageSize: 20}
		detail := r.URL.Path != "/api/v1/ops/audit"
		if detail {
			filter.AuditID = strings.TrimPrefix(r.URL.Path, "/api/v1/ops/audit/")
		}
		var err error
		if raw := q.Get("actor_user_id"); raw != "" {
			filter.ActorUserID, err = strconv.ParseInt(raw, 10, 64)
			if err != nil || filter.ActorUserID <= 0 {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
		}
		for name, dest := range map[string]*int{"page": &filter.Page, "page_size": &filter.PageSize} {
			if raw := q.Get(name); raw != "" {
				*dest, err = strconv.Atoi(raw)
				if err != nil {
					walletError(w, 400, "OPS_INPUT_INVALID")
					return
				}
			}
		}
		for name, dest := range map[string]*time.Time{"from": &filter.From, "to": &filter.To} {
			if raw := q.Get(name); raw != "" {
				*dest, err = time.Parse(time.RFC3339, raw)
				if err != nil {
					walletError(w, 400, "OPS_INPUT_INVALID")
					return
				}
			}
		}
		if filter.Page < 1 || filter.PageSize < 1 || filter.PageSize > 100 || (detail || filter.AuditID != "") && !platform.ValidOperationKey(filter.AuditID) || !filter.From.IsZero() && !filter.To.IsZero() && !filter.To.After(filter.From) {
			walletError(w, 400, "OPS_INPUT_INVALID")
			return
		}
		if detail {
			filter.Page = 1
			filter.PageSize = 1
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		result, err := store.ReadOpsAudit(ctx, filter)
		if err != nil {
			writeOpsError(w, err)
			return
		}
		if detail {
			if len(result.Items) == 0 {
				walletError(w, 404, "OPS_NOT_FOUND")
				return
			}
			walletSuccess(w, result.Items[0])
			return
		}
		walletSuccess(w, result)
	})
}
