package main

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/session"
)

const (
	opsEconomyOverviewPath  = "/api/v1/ops/economy/overview"
	opsEconomyUserPath      = "/api/v1/ops/economy/user"
	opsEconomyLedgerPath    = "/api/v1/ops/economy/ledger"
	opsEconomyTransfersPath = "/api/v1/ops/economy/transfers"
	opsRewardPoliciesPath   = "/api/v1/ops/rewards/policies"
	opsRewardClaimsPath     = "/api/v1/ops/rewards/claims"
)

type opsEconomyRuntimeHTTP struct {
	sessions *session.Service
	service  *platform.OpsEconomyService
	next     http.Handler
}

// newOpsEconomyRuntimeHandler owns only the six exact economy and reward read
// routes. Every other Ops route remains with the supplied runtime handler.
func newOpsEconomyRuntimeHandler(sessions *session.Service, service *platform.OpsEconomyService, next http.Handler) http.Handler {
	return &opsEconomyRuntimeHTTP{sessions: sessions, service: service, next: next}
}

func opsEconomyRuntimeRoute(path string) bool {
	switch path {
	case opsEconomyOverviewPath, opsEconomyUserPath, opsEconomyLedgerPath,
		opsEconomyTransfersPath, opsRewardPoliciesPath, opsRewardClaimsPath:
		return true
	default:
		return false
	}
}

func (h *opsEconomyRuntimeHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r == nil || r.URL == nil || !opsEconomyRuntimeRoute(r.URL.Path) {
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

	var (
		userID   int64
		before   time.Time
		beforeID string
		limit    = 50
		program  string
		status   string
		ok       bool
	)
	switch r.URL.Path {
	case opsEconomyOverviewPath, opsRewardPoliciesPath:
		if !requireOpsRead(w, r) {
			return
		}
	case opsEconomyUserPath:
		if !requireOpsRead(w, r, "newapi_user_id") {
			return
		}
		userID, ok = parseOpsEconomyUser(r.URL.Query().Get("newapi_user_id"), true)
		if !ok {
			walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
			return
		}
	case opsEconomyLedgerPath:
		if !requireOpsRead(w, r, "newapi_user_id", "before", "before_id", "limit") {
			return
		}
		userID, ok = parseOpsEconomyUser(r.URL.Query().Get("newapi_user_id"), true)
		if !ok {
			walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
			return
		}
		before, beforeID, limit, ok = parseOpsEconomyPage(r)
		if !ok {
			walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
			return
		}
	case opsEconomyTransfersPath:
		if !requireOpsRead(w, r, "newapi_user_id", "status", "before", "before_id", "limit") {
			return
		}
		userID, ok = parseOpsEconomyUser(r.URL.Query().Get("newapi_user_id"), false)
		if !ok {
			walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
			return
		}
		before, beforeID, limit, ok = parseOpsEconomyPage(r)
		if !ok {
			walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
			return
		}
		status = r.URL.Query().Get("status")
	case opsRewardClaimsPath:
		if !requireOpsRead(w, r, "newapi_user_id", "program", "status", "before", "before_id", "limit") {
			return
		}
		userID, ok = parseOpsEconomyUser(r.URL.Query().Get("newapi_user_id"), false)
		if !ok {
			walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
			return
		}
		before, beforeID, limit, ok = parseOpsEconomyPage(r)
		if !ok {
			walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
			return
		}
		program = r.URL.Query().Get("program")
		status = r.URL.Query().Get("status")
	}

	if h == nil || h.sessions == nil || h.service == nil {
		walletError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE")
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

	switch r.URL.Path {
	case opsEconomyOverviewPath:
		result, callErr := h.service.Overview(ctx, actor)
		if callErr != nil {
			writeOpsError(w, callErr)
			return
		}
		sessionEnvelope(w, http.StatusOK, result)
	case opsEconomyUserPath:
		result, callErr := h.service.User(ctx, actor, userID)
		if callErr != nil {
			writeOpsError(w, callErr)
			return
		}
		sessionEnvelope(w, http.StatusOK, result)
	case opsEconomyLedgerPath:
		result, callErr := h.service.Ledger(ctx, actor, platform.OpsLedgerQuery{
			UserID: userID, Before: before, BeforeID: beforeID, Limit: limit,
		})
		if callErr != nil {
			writeOpsError(w, callErr)
			return
		}
		sessionEnvelope(w, http.StatusOK, result)
	case opsEconomyTransfersPath:
		result, callErr := h.service.Transfers(ctx, actor, platform.OpsTransferQuery{
			UserID: userID, Status: status, Before: before, BeforeID: beforeID, Limit: limit,
		})
		if callErr != nil {
			writeOpsError(w, callErr)
			return
		}
		sessionEnvelope(w, http.StatusOK, result)
	case opsRewardPoliciesPath:
		result, callErr := h.service.RewardPolicies(ctx, actor)
		if callErr != nil {
			writeOpsError(w, callErr)
			return
		}
		sessionEnvelope(w, http.StatusOK, map[string]any{"items": result})
	case opsRewardClaimsPath:
		result, callErr := h.service.RewardClaims(ctx, actor, platform.OpsRewardClaimQuery{
			UserID: userID, Program: program, Status: status, Before: before, BeforeID: beforeID, Limit: limit,
		})
		if callErr != nil {
			writeOpsError(w, callErr)
			return
		}
		sessionEnvelope(w, http.StatusOK, result)
	}
}

func parseOpsEconomyUser(value string, required bool) (int64, bool) {
	if value == "" {
		return 0, !required
	}
	return opsEpoch(value)
}

func parseOpsEconomyPage(r *http.Request) (time.Time, string, int, bool) {
	values := r.URL.Query()
	limit := 50
	if value := values.Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 || strconv.Itoa(parsed) != value {
			return time.Time{}, "", 0, false
		}
		limit = parsed
	}
	beforeValue, beforeID := values.Get("before"), values.Get("before_id")
	if (beforeValue == "") != (beforeID == "") {
		return time.Time{}, "", 0, false
	}
	if beforeValue == "" {
		return time.Time{}, "", limit, true
	}
	before, err := time.Parse(time.RFC3339Nano, beforeValue)
	if err != nil || before.IsZero() || !opsUUIDPatternHTTP(beforeID) {
		return time.Time{}, "", 0, false
	}
	return before, beforeID, limit, true
}
