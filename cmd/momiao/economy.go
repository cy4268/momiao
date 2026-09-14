package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/cy4268/momiao/internal/platform"
	"io"
	"mime"
	"net/http"
	"net/url"
	"time"
	"unicode/utf8"
)

type economyStore interface {
	ReadDaily(context.Context, int64) (platform.Daily, error)
	ClaimDaily(context.Context, int64, string) (platform.Transaction, error)
	Exchange(context.Context, int64, string, platform.Asset, int64) (platform.Transaction, error)
	Transactions(context.Context, int64, string) ([]platform.Transaction, error)
	FindOperation(context.Context, int64, string, string) (*platform.Transaction, error)
}

type hourlyRewardStore interface {
	ReadHourlyReward(context.Context, int64) (platform.HourlyReward, error)
	ClaimHourlyReward(context.Context, int64, string) (platform.Transaction, error)
}

type reliefRewardStore interface {
	ReadReliefReward(context.Context, int64) (platform.ReliefReward, error)
	ClaimReliefReward(context.Context, int64, string) (platform.Transaction, error)
}

func decodeStringFields(body io.Reader, fields ...string) (map[string]string, error) {
	allowed := map[string]bool{}
	for _, field := range fields {
		allowed[field] = true
	}
	invalid := platform.ErrInvalidMutation
	raw, err := io.ReadAll(io.LimitReader(body, 2049))
	if err != nil || len(raw) > 2048 || !utf8.Valid(raw) {
		return nil, invalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	t, err := d.Token()
	if err != nil || t != json.Delim('{') {
		return nil, invalid
	}
	values := map[string]string{}
	for d.More() {
		t, err = d.Token()
		key, ok := t.(string)
		if err != nil || !ok || !allowed[key] {
			return nil, invalid
		}
		if _, ok = values[key]; ok {
			return nil, invalid
		}
		t, err = d.Token()
		v, ok := t.(string)
		if err != nil || !ok {
			return nil, invalid
		}
		values[key] = v
	}
	t, err = d.Token()
	if err != nil || t != json.Delim('}') {
		return nil, invalid
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, invalid
	}
	if len(values) != len(fields) {
		return nil, invalid
	}
	return values, nil
}
func decodeEconomyRequest(body io.Reader, exchange bool) (map[string]string, error) {
	fields := []string{"idempotency_key"}
	if exchange {
		fields = append(fields, "from_asset", "amount")
	}
	values, err := decodeStringFields(body, fields...)
	if err != nil || !platform.ValidOperationKey(values["idempotency_key"]) {
		return nil, platform.ErrInvalidMutation
	}
	return values, nil
}

func newEconomyHandler(origin string, store economyStore, transport http.RoundTripper) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			walletError(w, 503, "ECONOMY_UNAVAILABLE")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		user, status := verifyWalletUser(r, transport)
		if status != 0 {
			code := "AUTH_UNAVAILABLE"
			if status == 401 {
				code = "AUTH_UNAUTHORIZED"
			}
			if status == 403 {
				code = "AUTH_FORBIDDEN"
			}
			walletError(w, status, code)
			return
		}
		path := r.URL.Path
		exchange := path == "/platform/v1/wallet/exchange"
		dailyClaim := path == "/platform/v1/rewards/daily/claim"
		hourlyClaim := path == "/platform/v1/rewards/hourly/claim"
		reliefClaim := path == "/platform/v1/rewards/relief/claim"
		daily := path == "/platform/v1/rewards/daily"
		hourly := path == "/platform/v1/rewards/hourly"
		relief := path == "/platform/v1/rewards/relief"
		list := path == "/platform/v1/transactions"
		lookup := path == "/platform/v1/transactions/by-key"
		if !exchange && !dailyClaim && !hourlyClaim && !reliefClaim && !daily && !hourly && !relief && !list && !lookup {
			walletError(w, 404, "NOT_FOUND")
			return
		}
		method := "GET"
		if exchange || dailyClaim || hourlyClaim || reliefClaim {
			method = "POST"
		}
		if r.Method != method {
			w.Header().Set("Allow", method)
			walletError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || r.URL.ForceQuery {
			walletError(w, 400, "INVALID_REQUEST")
			return
		}
		for k, v := range q {
			if len(v) != 1 || (list && k != "after_id") || (lookup && k != "key" && k != "kind") || (!list && !lookup) {
				walletError(w, 400, "INVALID_REQUEST")
				return
			}
		}
		var result any
		if method == "POST" {
			if v := r.Header.Values("Origin"); len(v) != 1 || v[0] != origin {
				walletError(w, 403, "ORIGIN_REJECTED")
				return
			}
			ct, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if e != nil || ct != "application/json" || len(r.Header.Values("Content-Type")) != 1 {
				walletError(w, 415, "INVALID_CONTENT_TYPE")
				return
			}
			body, e := decodeEconomyRequest(r.Body, exchange)
			if e != nil {
				walletError(w, 400, "INVALID_REQUEST")
				return
			}
			if exchange {
				amount, e := platform.ParseAmount(body["amount"])
				if e != nil {
					walletError(w, 400, e.Error())
					return
				}
				result, err = store.Exchange(ctx, user, body["idempotency_key"], platform.Asset(body["from_asset"]), amount)
			} else if dailyClaim {
				result, err = store.ClaimDaily(ctx, user, body["idempotency_key"])
			} else if hourlyClaim {
				rewards, ok := store.(hourlyRewardStore)
				if !ok {
					walletError(w, http.StatusServiceUnavailable, "REWARDS_UNAVAILABLE")
					return
				}
				result, err = rewards.ClaimHourlyReward(ctx, user, body["idempotency_key"])
			} else {
				rewards, ok := store.(reliefRewardStore)
				if !ok {
					walletError(w, http.StatusServiceUnavailable, "REWARDS_UNAVAILABLE")
					return
				}
				result, err = rewards.ClaimReliefReward(ctx, user, body["idempotency_key"])
			}
		} else if daily {
			result, err = store.ReadDaily(ctx, user)
		} else if hourly {
			rewards, ok := store.(hourlyRewardStore)
			if !ok {
				walletError(w, http.StatusServiceUnavailable, "REWARDS_UNAVAILABLE")
				return
			}
			result, err = rewards.ReadHourlyReward(ctx, user)
		} else if relief {
			rewards, ok := store.(reliefRewardStore)
			if !ok {
				walletError(w, http.StatusServiceUnavailable, "REWARDS_UNAVAILABLE")
				return
			}
			result, err = rewards.ReadReliefReward(ctx, user)
		} else if lookup {
			result, err = store.FindOperation(ctx, user, q.Get("kind"), q.Get("key"))
		} else {
			var items []platform.Transaction
			items, err = store.Transactions(ctx, user, q.Get("after_id"))
			more := len(items) > 20
			if more {
				items = items[:20]
			}
			var next *string
			if more {
				v := items[len(items)-1].ID
				next = &v
			}
			result = struct {
				Items   []platform.Transaction `json:"items"`
				HasMore bool                   `json:"has_more"`
				Next    *string                `json:"next_after_id"`
			}{items, more, next}
		}
		if err != nil {
			status, code := 503, "ECONOMY_UNAVAILABLE"
			switch {
			case errors.Is(err, platform.ErrMaintenanceActive):
				status, code = http.StatusServiceUnavailable, "MAINTENANCE_ACTIVE"
			case errors.Is(err, platform.ErrHourlyDailyLimit):
				status, code = http.StatusConflict, "HOURLY_DAILY_LIMIT"
			case errors.Is(err, platform.ErrReliefCooldown):
				status, code = http.StatusConflict, "RELIEF_COOLDOWN"
			case errors.Is(err, platform.ErrReliefIneligible):
				status, code = http.StatusConflict, "RELIEF_NOT_ELIGIBLE"
			case errors.Is(err, platform.ErrInsufficientBalance):
				status, code = 409, "INSUFFICIENT_BALANCE"
			case errors.Is(err, platform.ErrIdempotencyConflict):
				status, code = 409, "IDEMPOTENCY_CONFLICT"
			case errors.Is(err, platform.ErrTransferPending):
				status, code = 409, "TRANSFER_PENDING"
			case errors.Is(err, platform.ErrWalletNotFound):
				status, code = 409, "WALLET_NOT_INITIALIZED"
			case errors.Is(err, platform.ErrBalanceOverflow):
				status, code = 409, "BALANCE_OVERFLOW"
			case errors.Is(err, platform.ErrInvalidMutation), errors.Is(err, platform.ErrInvalidPage):
				status, code = 400, "INVALID_REQUEST"
			}
			walletError(w, status, code)
			return
		}
		if receipt, ok := result.(platform.Transaction); ok && exchange && receipt.Kind == "API_CHIPS_EXCHANGE" && (receipt.Status == "PENDING" || receipt.Status == "SOURCE_DEBITED" || receipt.Status == "COMPENSATING") {
			walletJSON(w, http.StatusAccepted, struct {
				Success bool `json:"success"`
				Data    any  `json:"data"`
			}{true, result})
			return
		}
		walletSuccess(w, result)
	})
}
