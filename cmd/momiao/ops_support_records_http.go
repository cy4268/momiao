package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/history"
	"github.com/cy4268/momiao/internal/historyaccess"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/poker"
 "github.com/cy4268/momiao/internal/roulette"
	"github.com/cy4268/momiao/internal/session"
)

const (
	opsUsersPath        = "/api/v1/ops/users"
	opsRecordsPath      = "/api/v1/ops/records"
	opsSupportCasesPath = "/api/v1/ops/support-cases"
	opsIncidentsPath    = "/api/v1/ops/incidents"
)

type opsSupportRecordsHTTP struct {
	sessions *session.Service
	store    *platform.Store
	history  *historyHTTP
	next     http.Handler
}

func newOpsSupportRecordsHandler(sessions *session.Service, store *platform.Store,
	historyHandler *historyHTTP, next http.Handler) http.Handler {
	return &opsSupportRecordsHTTP{sessions: sessions, store: store, history: historyHandler, next: next}
}

func opsSupportRecordsRoute(path string) bool {
	for _, prefix := range []string{opsUsersPath, opsRecordsPath, opsSupportCasesPath, opsIncidentsPath} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func (h *opsSupportRecordsHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r == nil || r.URL == nil || !opsSupportRecordsRoute(r.URL.Path) {
		if h != nil && h.next != nil {
			h.next.ServeHTTP(w, r)
			return
		}
		walletError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !opsRequestSafe(r) {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if r.ContentLength > 0 || h == nil || h.sessions == nil || h.store == nil {
		if r.ContentLength > 0 {
			walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		} else {
			walletError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE")
		}
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	verified, err := h.sessions.VerifyPrivateRequest(r)
	if err != nil {
		sessionError(w, err)
		return
	}
	actor, err := strconv.ParseInt(verified.View().UserID, 10, 64)
	if err != nil || actor <= 0 {
		walletError(w, http.StatusUnauthorized, "SESSION_UNAUTHORIZED")
		return
	}
	switch {
	case r.URL.Path == opsUsersPath:
		h.users(w, r, actor)
	case strings.HasPrefix(r.URL.Path, opsUsersPath+"/"):
		h.user(w, r, actor)
	case r.URL.Path == opsSupportCasesPath:
		h.supportCases(w, r, actor)
	case strings.HasPrefix(r.URL.Path, opsSupportCasesPath+"/"):
		h.supportCase(w, r, actor)
	case r.URL.Path == opsIncidentsPath:
		h.incidents(w, r, actor)
	case strings.HasPrefix(r.URL.Path, opsIncidentsPath+"/"):
		h.incident(w, r, actor)
	case r.URL.Path == opsRecordsPath:
		h.records(w, r, actor)
	case strings.HasPrefix(r.URL.Path, opsRecordsPath+"/"):
		h.record(w, r, actor)
	default:
		walletError(w, http.StatusNotFound, "NOT_FOUND")
	}
}

func opsReadLimit(value string) (int, bool) {
	if value == "" {
		return 50, true
	}
	limit, err := strconv.Atoi(value)
	return limit, err == nil && limit >= 1 && limit <= 100 && strconv.Itoa(limit) == value
}

func opsOptionalSubject(value string) (int64, bool) {
	if value == "" {
		return 0, true
	}
	return opsEpoch(value)
}

func (h *opsSupportRecordsHTTP) users(w http.ResponseWriter, r *http.Request, actor int64) {
	if !requireOpsQuery(w, r, "authz_epoch", "q", "limit") {
		return
	}
	epoch, validEpoch := opsEpoch(r.URL.Query().Get("authz_epoch"))
	limit, validLimit := opsReadLimit(r.URL.Query().Get("limit"))
	if !validEpoch || !validLimit {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	items, err := h.store.OpsUsers(r.Context(), actor, epoch, r.URL.Query().Get("q"), limit)
	if err != nil {
		writeOpsError(w, err)
		return
	}
	sessionEnvelope(w, http.StatusOK, map[string]any{"items": items})
}

func (h *opsSupportRecordsHTTP) user(w http.ResponseWriter, r *http.Request, actor int64) {
	if !requireOpsQuery(w, r, "authz_epoch") {
		return
	}
	raw := strings.TrimPrefix(r.URL.Path, opsUsersPath+"/")
	subject, validSubject := opsEpoch(raw)
	epoch, validEpoch := opsEpoch(r.URL.Query().Get("authz_epoch"))
	if !validSubject || !validEpoch || strings.Contains(raw, "/") {
		walletError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	item, err := h.store.OpsUser(r.Context(), actor, epoch, subject)
	if err != nil {
		writeOpsError(w, err)
		return
	}
	sessionEnvelope(w, http.StatusOK, item)
}

func (h *opsSupportRecordsHTTP) supportCases(w http.ResponseWriter, r *http.Request, actor int64) {
	if !requireOpsQuery(w, r, "authz_epoch", "state", "subject_newapi_user_id", "limit") {
		return
	}
	values := r.URL.Query()
	epoch, validEpoch := opsEpoch(values.Get("authz_epoch"))
	subject, validSubject := opsOptionalSubject(values.Get("subject_newapi_user_id"))
	limit, validLimit := opsReadLimit(values.Get("limit"))
	if !validEpoch || !validSubject || !validLimit {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	items, err := h.store.OpsSupportCases(r.Context(), actor, epoch, platform.OpsSupportCaseQuery{
		State: values.Get("state"), SubjectID: subject, Limit: limit,
	})
	if err != nil {
		writeOpsError(w, err)
		return
	}
	sessionEnvelope(w, http.StatusOK, map[string]any{"items": items})
}

func (h *opsSupportRecordsHTTP) supportCase(w http.ResponseWriter, r *http.Request, actor int64) {
	if !requireOpsQuery(w, r, "authz_epoch") {
		return
	}
	caseID := strings.TrimPrefix(r.URL.Path, opsSupportCasesPath+"/")
	epoch, validEpoch := opsEpoch(r.URL.Query().Get("authz_epoch"))
	if !opsUUIDPatternHTTP(caseID) || !validEpoch {
		walletError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	detail, err := h.store.OpsSupportCaseDetail(r.Context(), actor, epoch, caseID)
	if err != nil {
		writeOpsError(w, err)
		return
	}
	sessionEnvelope(w, http.StatusOK, detail)
}

func (h *opsSupportRecordsHTTP) incidents(w http.ResponseWriter, r *http.Request, actor int64) {
	if !requireOpsQuery(w, r, "authz_epoch", "state", "severity", "limit") {
		return
	}
	values := r.URL.Query()
	epoch, validEpoch := opsEpoch(values.Get("authz_epoch"))
	limit, validLimit := opsReadLimit(values.Get("limit"))
	if !validEpoch || !validLimit {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	items, err := h.store.OpsIncidents(r.Context(), actor, epoch, platform.OpsIncidentQuery{
		State: values.Get("state"), Severity: values.Get("severity"), Limit: limit,
	})
	if err != nil {
		writeOpsError(w, err)
		return
	}
	sessionEnvelope(w, http.StatusOK, map[string]any{"items": items})
}

func (h *opsSupportRecordsHTTP) incident(w http.ResponseWriter, r *http.Request, actor int64) {
	if !requireOpsQuery(w, r, "authz_epoch") {
		return
	}
	incidentID := strings.TrimPrefix(r.URL.Path, opsIncidentsPath+"/")
	epoch, validEpoch := opsEpoch(r.URL.Query().Get("authz_epoch"))
	if !opsUUIDPatternHTTP(incidentID) || !validEpoch {
		walletError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	detail, err := h.store.OpsIncidentDetail(r.Context(), actor, epoch, incidentID)
	if err != nil {
		writeOpsError(w, err)
		return
	}
	sessionEnvelope(w, http.StatusOK, detail)
}

func (h *opsSupportRecordsHTTP) records(w http.ResponseWriter, r *http.Request, actor int64) {
	if !requireOpsQuery(w, r, "authz_epoch", "subject_newapi_user_id", "record_type", "mode",
		"game_slug", "time_from", "time_to", "result", "status", "id", "limit", "cursor") {
		return
	}
	if h.history == nil || h.history.list == nil {
		walletError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE")
		return
	}
	values := r.URL.Query()
	actorEpoch, validEpoch := opsEpoch(values.Get("authz_epoch"))
	subject, validSubject := opsEpoch(values.Get("subject_newapi_user_id"))
	limit, validLimit := opsReadLimit(values.Get("limit"))
	if !validEpoch || !validSubject || !validLimit {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	query := history.Query{
		RecordType: history.Source(values.Get("record_type")), Mode: values.Get("mode"),
		GameSlug: values.Get("game_slug"), TimeFrom: values.Get("time_from"), TimeTo: values.Get("time_to"),
		Result: values.Get("result"), Status: values.Get("status"), ID: values.Get("id"),
		Limit: limit, Cursor: values.Get("cursor"),
	}
	recordID := "*"
	if query.ID != "" {
		recordID = query.ID
	}
	if err := h.store.AuditOpsRecordAccess(r.Context(), actor, actorEpoch, subject,
		"LIST", "HISTORY_LIST", recordID); err != nil {
		writeOpsError(w, err)
		return
	}
	page, err := h.history.list.List(r.Context(), subject, query)
	if err != nil {
		historyHTTPError(w, err)
		return
	}
	sessionEnvelope(w, http.StatusOK, struct {
		SubjectID int64 `json:"subject_newapi_user_id,string"`
		history.Page
	}{subject, page})
}

func (h *opsSupportRecordsHTTP) record(w http.ResponseWriter, r *http.Request, actor int64) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, opsRecordsPath+"/"), "/")
	verify := len(parts) == 3 && parts[2] == "verify"
	if len(parts) != 2 && !verify || !opsUUIDPatternHTTP(parts[1]) {
		walletError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	kind, recordID := parts[0], parts[1]
	recordType := map[string]string{
		"roulette": "ROULETTE_ROUND", "rounds": "DIRECT_PLAY_ROUND", "sessions": "POKER_SESSION",
		"hands": "POKER_HAND", "transactions": "TRANSACTION",
	}[kind]
	if recordType == "" || verify && kind != "rounds" && kind != "hands" && kind != "roulette" {
		walletError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	allowed := []string{"authz_epoch", "subject_newapi_user_id"}
	if !verify {
		switch kind {
		case "sessions":
			allowed = append(allowed, "funding_limit", "funding_cursor", "hand_limit", "hand_cursor")
		case "hands", "roulette":
			allowed = append(allowed, "limit", "cursor")
		}
	}
	if !requireOpsQuery(w, r, allowed...) {
		return
	}
	if h.history == nil {
		walletError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE")
		return
	}
	values := r.URL.Query()
	epoch, validEpoch := opsEpoch(values.Get("authz_epoch"))
	subject, validSubject := opsEpoch(values.Get("subject_newapi_user_id"))
	if !validEpoch || !validSubject {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	accessKind := "DETAIL"
	if verify {
		accessKind = "VERIFY"
	}
	access := historyaccess.Records(actor, subject, func(ctx context.Context, viewer, checkedSubject int64) error {
		if viewer != actor || checkedSubject != subject {
			return platform.ErrOpsForbidden
		}
		return h.store.AuditOpsRecordAccess(ctx, actor, epoch, subject, accessKind, recordType, recordID)
	})
	var (
		record any
		err    error
	)
	switch kind {
	case "roulette":
  if h.history.roulette==nil {err=historyaccess.ErrUnavailable} else if verify {record,err=h.history.roulette.HistoryVerify(r.Context(),access,recordID)} else {
   limit,valid:=opsReadLimit(values.Get("limit"));if !valid{walletError(w,400,"OPS_INPUT_INVALID");return}
   record,err=h.history.roulette.HistoryDetail(r.Context(),access,recordID,roulette.HistoryQuery{Limit:limit,Cursor:values.Get("cursor")})
  }
 case "rounds":
		if h.history.rounds == nil {
			err = historyaccess.ErrUnavailable
		} else if verify {
			record, err = h.history.rounds.HistoryVerify(r.Context(), access, recordID)
		} else {
			record, err = h.history.rounds.HistoryDetail(r.Context(), access, recordID)
		}
	case "sessions":
		if h.history.poker == nil {
			err = historyaccess.ErrUnavailable
			break
		}
		fundingLimit, validFundingLimit := opsReadLimit(values.Get("funding_limit"))
		handLimit, validHandLimit := opsReadLimit(values.Get("hand_limit"))
		if !validFundingLimit || !validHandLimit {
			walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
			return
		}
		record, err = h.history.poker.SessionDetail(r.Context(), access, recordID, poker.HistorySessionQuery{
			FundingLimit: fundingLimit, FundingCursor: values.Get("funding_cursor"),
			HandLimit: handLimit, HandCursor: values.Get("hand_cursor"),
		})
	case "hands":
		if h.history.poker == nil {
			err = historyaccess.ErrUnavailable
			break
		}
		if verify {
			record, err = h.history.poker.HandFairness(r.Context(), access, recordID)
		} else {
			limit, validLimit := opsReadLimit(values.Get("limit"))
			if !validLimit {
				walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
				return
			}
			record, err = h.history.poker.HandDetail(r.Context(), access, recordID, poker.HistoryHandQuery{
				Limit: limit, Cursor: values.Get("cursor"),
			})
		}
	case "transactions":
		if h.history.wallet == nil {
			err = historyaccess.ErrUnavailable
		} else {
			record, err = h.history.wallet.HistoryTransaction(r.Context(), access, recordID)
		}
	}
	if err != nil {
		historyHTTPError(w, err)
		return
	}
	sessionEnvelope(w, http.StatusOK, struct {
		SubjectID  int64  `json:"subject_newapi_user_id,string"`
		RecordType string `json:"record_type"`
		Record     any    `json:"record"`
	}{subject, recordType, record})
}
