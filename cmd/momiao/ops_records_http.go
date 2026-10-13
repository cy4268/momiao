package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/history"
	"github.com/cy4268/momiao/internal/historyaccess"
	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
)

const opsRecordsPath = "/api/v1/ops/records"

func opsRecordsRoute(path string) bool {
	return path == opsRecordsPath || strings.HasPrefix(path, opsRecordsPath+"/")
}

func newLocalOpsRecordsHandler(store *platform.Store, records *historyHTTP) http.Handler {
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
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if store == nil || records == nil {
			walletError(w, 503, "OPS_UNAVAILABLE")
			return
		}
		if !opsRequestSafe(r) || r.ContentLength > 0 {
			walletError(w, 400, "OPS_INPUT_INVALID")
			return
		}
		// The only Ops-specific query field is the optional subject. All other
		// filters/detail cursors keep the personal history contract.
		u := *r.URL
		values := u.Query()
		subject, valid := opsOptionalSubject(values.Get("user"))
		if !valid || len(values["user"]) > 1 {
			walletError(w, 400, "OPS_INPUT_INVALID")
			return
		}
		values.Del("user")
		u.Path, u.RawQuery = historyPath+strings.TrimPrefix(u.Path, opsRecordsPath), values.Encode()
		kind, id, proof, query, err := parseHistoryRequest(&u)
		if err != nil || kind != "list" && subject == 0 {
			walletError(w, 400, "OPS_INPUT_INVALID")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		accessKind, recordType := "DETAIL", map[string]string{"list": "HISTORY_LIST", "rounds": "DIRECT_PLAY_ROUND", "sessions": "POKER_SESSION", "hands": "POKER_HAND", "roulette": "ROULETTE_ROUND", "transactions": "TRANSACTION"}[kind]
		if proof {
			accessKind = "VERIFY"
		}
		if kind == "list" {
			accessKind, id = "LIST", query.Get("id")
			if id == "" {
				id = "*"
			}
		}
		audit := func(ctx context.Context, viewer, target int64) error {
			return store.AuditOpsRecordAccess(ctx, viewer, target, accessKind, recordType, id, query)
		}
		var data any
		if kind == "list" {
			if err = audit(ctx, user.ID, subject); err != nil {
				writeOpsError(w, err)
				return
			}
			data, err = history.NewReader(store.Pool()).ListOps(ctx, subject, history.Query{
				RecordType: history.Source(query.Get("record_type")), Mode: query.Get("mode"), GameSlug: query.Get("game_slug"),
				TimeFrom: query.Get("time_from"), TimeTo: query.Get("time_to"), Result: query.Get("result"), Status: query.Get("status"),
				ID: query.Get("id"), Limit: historyLimit(query, "limit"), Cursor: query.Get("cursor"),
			})
		} else {
			data, err = records.readRecord(ctx, kind, id, proof, query, historyaccess.Records(user.ID, subject, audit))
		}
		if err != nil {
			historyHTTPError(w, err)
			return
		}
		if kind == "list" {
			sessionEnvelope(w, 200, data)
			return
		}
		sessionEnvelope(w, 200, struct {
			Subject    int64  `json:"subject_user_id,string"`
			RecordType string `json:"record_type"`
			Record     any    `json:"record"`
		}{subject, recordType, data})
	})
}
