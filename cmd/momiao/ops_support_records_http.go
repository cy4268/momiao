package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/session"
)

const (
	opsUsersPath        = "/api/v1/ops/users"
	opsSupportCasesPath = "/api/v1/ops/support-cases"
	opsIncidentsPath    = "/api/v1/ops/incidents"
)

type opsSupportRecordsHTTP struct {
	sessions *session.Service
	store    *platform.Store
	next     http.Handler
}

func newOpsSupportRecordsHandler(sessions *session.Service, store *platform.Store,
	next http.Handler) http.Handler {
	return &opsSupportRecordsHTTP{sessions: sessions, store: store, next: next}
}

func opsSupportRecordsRoute(path string) bool {
	for _, prefix := range []string{opsSupportCasesPath, opsIncidentsPath} {
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
	case r.URL.Path == opsSupportCasesPath:
		h.supportCases(w, r, actor)
	case strings.HasPrefix(r.URL.Path, opsSupportCasesPath+"/"):
		h.supportCase(w, r, actor)
	case r.URL.Path == opsIncidentsPath:
		h.incidents(w, r, actor)
	case strings.HasPrefix(r.URL.Path, opsIncidentsPath+"/"):
		h.incident(w, r, actor)
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
