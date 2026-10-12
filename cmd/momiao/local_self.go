package main

import (
	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
	"net/http"
)

func newLocalSelfHandler(store walletStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			walletError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		user, ok := identity.UserFromContext(r.Context())
		if !ok {
			walletError(w, 401, "AUTH_UNAUTHORIZED")
			return
		}
		if store == nil {
			walletError(w, 503, "WALLET_UNAVAILABLE")
			return
		}
		wallets, err := store.ReadWallets(r.Context(), user.ID)
		if err != nil {
			walletError(w, 503, "WALLET_UNAVAILABLE")
			return
		}
		var reserve int64
		for _, wallet := range wallets {
			if wallet.Asset == platform.ReserveAPICredit {
				reserve = wallet.BalanceUnits
			}
		}
		walletSuccess(w, map[string]any{
			"id": user.ID, "username": user.Username, "display_name": user.DisplayName, "role": user.Role, "group": user.Group,
			"quota": reserve, "reserve_amount": platform.FormatAmount(reserve),
			// Local relay logs supply these in the relay migration, not native counters.
			"used_quota": nil, "request_count": nil,
		})
	})
}
