package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/poker"
	"github.com/cy4268/momiao/internal/session"
)

const opsPokerPath = "/api/v1/ops/poker"

type opsPokerHTTP struct {
	sessions *session.Service
	store    *platform.Store
	service  PokerOpsPort
	next     http.Handler
}

func newOpsPokerHandler(sessions *session.Service, store *platform.Store, service PokerOpsPort, next http.Handler) http.Handler {
	return &opsPokerHTTP{sessions: sessions, store: store, service: service, next: next}
}

func (h *opsPokerHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r == nil || r.URL == nil || r.URL.Path != opsPokerPath && !strings.HasPrefix(r.URL.Path, opsPokerPath+"/") {
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
	if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}

	var tableQuery poker.OpsTableQuery
	var sessionQuery poker.OpsSessionQuery
	path := strings.TrimPrefix(r.URL.Path, opsPokerPath)
	switch {
	case path == "":
		if !requireOpsQuery(w, r, "authz_epoch") {
			return
		}
	case path == "/tables":
		if !requireOpsQuery(w, r, "authz_epoch", "cursor", "limit") {
			return
		}
		var ok bool
		tableQuery.Limit, ok = opsPokerLimit(r.URL.Query().Get("limit"))
		tableQuery.Cursor = r.URL.Query().Get("cursor")
		if !ok || tableQuery.Cursor != "" && !opsUUIDPatternHTTP(tableQuery.Cursor) {
			walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
			return
		}
	case path == "/sessions":
		if !requireOpsQuery(w, r, "authz_epoch", "table_id", "cursor", "limit") {
			return
		}
		var ok bool
		sessionQuery.Limit, ok = opsPokerLimit(r.URL.Query().Get("limit"))
		sessionQuery.TableID = r.URL.Query().Get("table_id")
		sessionQuery.Cursor = r.URL.Query().Get("cursor")
		if !ok || sessionQuery.TableID != "" && !opsUUIDPatternHTTP(sessionQuery.TableID) ||
			sessionQuery.Cursor != "" && !opsUUIDPatternHTTP(sessionQuery.Cursor) {
			walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
			return
		}
	case strings.HasPrefix(path, "/tables/"), strings.HasPrefix(path, "/sessions/"), strings.HasPrefix(path, "/hands/"):
		if !requireOpsQuery(w, r, "authz_epoch") {
			return
		}
		id := path[strings.LastIndexByte(path, '/')+1:]
		if !opsUUIDPatternHTTP(id) || strings.Count(path, "/") != 2 {
			walletError(w, http.StatusNotFound, "NOT_FOUND")
			return
		}
	default:
		walletError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}

	epoch, ok := opsEpoch(r.URL.Query().Get("authz_epoch"))
	if !ok {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	if h == nil || h.sessions == nil || h.store == nil || h.service == nil {
		walletError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
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
	if _, err = h.store.RequireOpsPermission(ctx, actor, epoch, "poker.read"); err != nil {
		writeOpsError(w, err)
		return
	}

	switch {
	case path == "":
		result, callErr := h.service.ReadOpsOverview(ctx)
		writeOpsPokerRead(w, result, callErr)
	case path == "/tables":
		result, callErr := h.service.ListOpsTables(ctx, tableQuery)
		writeOpsPokerRead(w, result, callErr)
	case path == "/sessions":
		result, callErr := h.service.ListOpsSessions(ctx, sessionQuery)
		writeOpsPokerRead(w, result, callErr)
	case strings.HasPrefix(path, "/tables/"):
		result, callErr := h.service.ReadOpsTable(ctx, strings.TrimPrefix(path, "/tables/"))
		writeOpsPokerRead(w, result, callErr)
	case strings.HasPrefix(path, "/sessions/"):
		result, callErr := h.service.ReadOpsSession(ctx, strings.TrimPrefix(path, "/sessions/"))
		writeOpsPokerRead(w, result, callErr)
	case strings.HasPrefix(path, "/hands/"):
		result, callErr := h.service.ReadOpsHand(ctx, strings.TrimPrefix(path, "/hands/"))
		writeOpsPokerRead(w, result, callErr)
	}
}

func opsPokerLimit(value string) (int, bool) {
	if value == "" {
		return 50, true
	}
	limit, err := strconv.Atoi(value)
	return limit, err == nil && limit >= 1 && limit <= 100 && strconv.Itoa(limit) == value
}

func writeOpsPokerRead(w http.ResponseWriter, result any, err error) {
	if err == nil {
		sessionEnvelope(w, http.StatusOK, result)
		return
	}
	switch {
	case errors.Is(err, poker.ErrInvalid):
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
	case errors.Is(err, poker.ErrDenied), errors.Is(err, errPokerOpsNotFound):
		walletError(w, http.StatusNotFound, "OPS_NOT_FOUND")
	default:
		walletError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE")
	}
}
