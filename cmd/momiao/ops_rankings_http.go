package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/rankings"
	"github.com/cy4268/momiao/internal/session"
)

const opsRankingsPath = "/api/v1/ops/rankings"

type opsRankingsHTTP struct {
	sessions *session.Service
	store    *platform.Store
	service  *rankings.Service
	next     http.Handler
}

func newOpsRankingsHandler(sessions *session.Service, store *platform.Store,
	service *rankings.Service, next http.Handler) http.Handler {
	return &opsRankingsHTTP{sessions: sessions, store: store, service: service, next: next}
}

func opsRankingsRoute(path string) bool {
	return path == opsRankingsPath || strings.HasPrefix(path, opsRankingsPath+"/")
}

func (h *opsRankingsHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r == nil || r.URL == nil || !opsRankingsRoute(r.URL.Path) {
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
	if !requireOpsRead(w, r, "authz_epoch") {
		return
	}
	if h == nil || h.sessions == nil || h.store == nil || h.service == nil {
		walletError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE")
		return
	}
	epoch, valid := opsEpoch(r.URL.Query().Get("authz_epoch"))
	if !valid {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	var snapshotID string
	if r.URL.Path != opsRankingsPath {
		prefix := opsRankingsPath + "/snapshots/"
		if !strings.HasPrefix(r.URL.Path, prefix) {
			walletError(w, http.StatusNotFound, "NOT_FOUND")
			return
		}
		snapshotID = strings.TrimPrefix(r.URL.Path, prefix)
		if !opsUUIDPatternHTTP(snapshotID) || strings.Contains(snapshotID, "/") {
			walletError(w, http.StatusNotFound, "NOT_FOUND")
			return
		}
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
	if _, err = h.store.RequireOpsPermission(ctx, actor, epoch, "rankings.read"); err != nil {
		writeOpsError(w, err)
		return
	}
	if snapshotID == "" {
		result, readErr := h.service.ReadOpsOverview(ctx)
		if readErr != nil {
			writeOpsError(w, readErr)
			return
		}
		sessionEnvelope(w, http.StatusOK, result)
		return
	}
	result, err := h.service.ReadOpsSnapshot(ctx, snapshotID)
	if err != nil {
		writeOpsError(w, err)
		return
	}
	sessionEnvelope(w, http.StatusOK, result)
}
