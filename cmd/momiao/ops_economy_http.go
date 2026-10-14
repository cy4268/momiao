package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
)

const (
	opsEconomyOverviewPath  = "/api/v1/ops/economy/overview"
	opsEconomyUserPath      = "/api/v1/ops/economy/user"
	opsEconomyLedgerPath    = "/api/v1/ops/economy/ledger"
	opsEconomyTransfersPath = "/api/v1/ops/economy/transfers"
	opsRewardPoliciesPath   = "/api/v1/ops/rewards/policies"
	opsRewardClaimsPath     = "/api/v1/ops/rewards/claims"
)

func opsEconomyRuntimeRoute(path string) bool {
	switch path {
	case opsEconomyOverviewPath, opsEconomyUserPath, opsEconomyLedgerPath, opsEconomyTransfersPath, opsRewardPoliciesPath, opsRewardClaimsPath:
		return true
	}
	return strings.HasPrefix(path, opsRewardClaimsPath+"/")
}

func newOpsEconomyRuntimeHandler(store *platform.Store) http.Handler {
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
		service, err := platform.NewOpsEconomyService(store)
		if err != nil {
			writeOpsError(w, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		var result any
		if strings.HasPrefix(r.URL.Path, opsRewardClaimsPath+"/") {
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, opsRewardClaimsPath+"/"), "/")
			if len(parts) != 2 || parts[1] != "retry" || r.Method != "POST" {
				walletError(w, 404, "NOT_FOUND")
				return
			}
			result, err = service.RetryReward(ctx, user.ID, parts[0])
		} else {
			if r.Method != "GET" {
				w.Header().Set("Allow", "GET")
				walletError(w, 405, "METHOD_NOT_ALLOWED")
				return
			}
			q := r.URL.Query()
			id, valid := parseOpsEconomyUser(q.Get("newapi_user_id"), r.URL.Path == opsEconomyUserPath)
			before, beforeID, limit, paged := parseOpsEconomyPage(r)
			if !valid || !paged {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			switch r.URL.Path {
			case opsEconomyOverviewPath:
				result, err = service.Overview(ctx, user.ID)
			case opsEconomyUserPath:
				result, err = service.User(ctx, user.ID, id)
			case opsEconomyLedgerPath:
				result, err = service.Ledger(ctx, user.ID, platform.OpsLedgerQuery{UserID: id, Before: before, BeforeID: beforeID, Limit: limit, Asset: platform.Asset(q.Get("asset")), BizType: q.Get("biz_type")})
			case opsEconomyTransfersPath:
				result, err = service.Transfers(ctx, user.ID, platform.OpsTransferQuery{UserID: id, Status: q.Get("status"), Before: before, BeforeID: beforeID, Limit: limit})
			case opsRewardPoliciesPath:
				items, e := service.RewardPolicies(ctx, user.ID)
				err = e
				result = map[string]any{"items": items}
			case opsRewardClaimsPath:
				result, err = service.RewardClaims(ctx, user.ID, platform.OpsRewardClaimQuery{UserID: id, Program: q.Get("program"), Status: q.Get("status"), Before: before, BeforeID: beforeID, Limit: limit})
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
