package main

import (
	"context"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/session"
	"net/http"
	"strconv"
	"time"
)

func newOpsGamblerHandler(sessions *session.Service, campaign *platform.GamblerCampaignService, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/ops/economy/gambler-campaigns/"+platform.GamblerCampaignID {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !opsRequestSafe(r) {
			walletError(w, 400, "OPS_INPUT_INVALID")
			return
		}
		if !requireOpsRead(w, r, "after_user") {
			return
		}
		after, ok := parseOpsEconomyUser(r.URL.Query().Get("after_user"), false)
		if !ok {
			walletError(w, 400, "OPS_INPUT_INVALID")
			return
		}
		if sessions == nil || campaign == nil {
			walletError(w, 503, "OPS_UNAVAILABLE")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		verified, err := sessions.VerifyPrivateRequest(r)
		if err != nil {
			sessionError(w, err)
			return
		}
		actor, err := strconv.ParseInt(verified.View().UserID, 10, 64)
		if err != nil || actor <= 0 {
			walletError(w, 401, "SESSION_UNAUTHORIZED")
			return
		}
		view, err := campaign.ReadPage(ctx, actor, platform.GamblerCampaignID, after)
		if err != nil {
			writeOpsError(w, err)
			return
		}
		sessionEnvelope(w, 200, view)
	})
}
