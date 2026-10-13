package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
)

func newOpsGamblerHandler(store *platform.Store, environment string) http.Handler {
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
		if r.URL.Path != "/api/v1/ops/economy/gambler-campaigns/"+platform.GamblerCampaignID {
			walletError(w, 404, "NOT_FOUND")
			return
		}
		if store == nil || environment == "" {
			walletError(w, 503, "OPS_UNAVAILABLE")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		campaign := platform.NewGamblerCampaignService(store, environment)
		var result platform.GamblerCampaignView
		var err error
		switch r.Method {
		case "GET":
			values := r.URL.Query()
			query := platform.GamblerQuery{Page: 1, State: values.Get("state")}
			if page := values.Get("page"); page != "" {
				query.Page, err = strconv.Atoi(page)
			}
			if err != nil || query.Page < 1 || query.Page > 100000 || !slices.Contains([]string{"", "PENDING", "CHECKED", "COMPLETED"}, query.State) {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			var valid bool
			query.UserID, valid = opsOptionalSubject(values.Get("user"))
			if !valid {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			if v := values.Get("eligible"); v != "" {
				if v != "true" && v != "false" {
					walletError(w, 400, "OPS_INPUT_INVALID")
					return
				}
				eligible := v == "true"
				query.Eligible = &eligible
			}
			for key, target := range map[string]*time.Time{"from": &query.From, "to": &query.To} {
				if v := values.Get(key); v != "" {
					*target, err = time.Parse(time.RFC3339, v)
					if err != nil {
						walletError(w, 400, "OPS_INPUT_INVALID")
						return
					}
				}
			}
			if !query.From.IsZero() && !query.To.IsZero() && !query.To.After(query.From) {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			result, err = campaign.ReadPage(ctx, user.ID, query)
		case "POST":
			var input platform.GamblerCommand
			if json.NewDecoder(r.Body).Decode(&input) != nil {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			result, err = campaign.Save(ctx, user.ID, input)
		default:
			walletError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		if errors.Is(err, platform.ErrCatalogForbidden) {
			walletError(w, 403, "OPS_FORBIDDEN")
			return
		}
		if errors.Is(err, platform.ErrGamblerBlocked) {
			walletError(w, 409, "GAMBLER_CAMPAIGN_BLOCKED")
			return
		}
		if err != nil {
			writeOpsError(w, err)
			return
		}
		walletSuccess(w, result)
	})
}
