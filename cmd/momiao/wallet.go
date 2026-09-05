package main

import (
	"context"
	"encoding/json"
	"github.com/cy4268/momiao/internal/platform"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// walletStore deliberately exposes no asset mutation or migration methods.
type walletStore interface {
	EnsureAccount(context.Context, int64) error
	ReadWallets(context.Context, int64) ([]platform.Wallet, error)
	Ledger(context.Context, int64, platform.Asset, int64, int) ([]platform.LedgerEntry, error)
}

func walletJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func walletError(w http.ResponseWriter, status int, code string) {
	walletJSON(w, status, struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}{false, code, http.StatusText(status)})
}
func walletSuccess(w http.ResponseWriter, data any) {
	walletJSON(w, 200, struct {
		Success bool `json:"success"`
		Data    any  `json:"data"`
	}{true, data})
}
func decimalInt(v string) (int64, error) {
	if v == "" || strings.IndexFunc(v, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseInt(v, 10, 64)
}
func authHeader(r *http.Request, key string, max int, required bool) (string, bool) {
	v := r.Header.Values(key)
	if len(v) == 0 {
		return "", !required
	}
	if len(v) != 1 || len(v[0]) > max || v[0] == "" {
		return "", false
	}
	for _, c := range v[0] {
		if c < 33 || c > 126 {
			if key == "Authorization" && c == ' ' {
				continue
			}
			return "", false
		}
	}
	return v[0], true
}
func verifyWalletUser(r *http.Request, transport http.RoundTripper) (int64, int) {
	auth, ok := authHeader(r, "Authorization", 8192, true)
	parts := strings.Split(auth, " ")
	if !ok || len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return 0, 401
	}
	claimed, ok := authHeader(r, "New-Api-User", 19, true)
	id, err := decimalInt(claimed)
	if !ok || err != nil || id <= 0 {
		return 0, 401
	}
	session, ok := authHeader(r, "X-Auth-Session", 512, false)
	if !ok {
		return 0, 401
	}
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "http://unix/api/user/self", nil)
	req.Host = "localhost"
	req.Header.Set("Authorization", auth)
	req.Header.Set("New-Api-User", claimed)
	if session != "" {
		req.Header.Set("X-Auth-Session", session)
	}
	// RoundTrip never follows redirects and has no cookie jar. Only fixed native Unix transport is wired in production.
	resp, err := transport.RoundTrip(req)
	if err != nil {
		return 0, 502
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return 0, resp.StatusCode
	}
	if resp.StatusCode != 200 {
		return 0, 502
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if err != nil || len(body) > 65536 {
		return 0, 502
	}
	var result struct {
		Success bool `json:"success"`
		Data    struct {
			ID     int64 `json:"id"`
			Status *int  `json:"status"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &result) != nil || !result.Success || result.Data.ID <= 0 || result.Data.Status == nil {
		return 0, 502
	}
	if result.Data.ID != id || *result.Data.Status != 1 {
		return 0, 401
	}
	return result.Data.ID, 0
}

type walletView struct {
	Asset        platform.Asset `json:"asset"`
	BalanceUnits int64          `json:"balance_units,string"`
	Amount       string         `json:"amount"`
	LedgerSeq    int64          `json:"ledger_seq,string"`
	Version      int64          `json:"version,string"`
}

func newWalletHandler(origin string, store walletStore, transport http.RoundTripper) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			walletError(w, 503, "WALLET_UNAVAILABLE")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		id, status := verifyWalletUser(r, transport)
		if status != 0 {
			code := "AUTH_UNAVAILABLE"
			if status == 401 {
				code = "AUTH_UNAUTHORIZED"
			} else if status == 403 {
				code = "AUTH_FORBIDDEN"
			}
			walletError(w, status, code)
			return
		}
		ledger := r.URL.Path == "/platform/v1/wallet/ledger"
		initialize := r.URL.Path == "/platform/v1/wallet/initialize"
		if !ledger && !initialize && r.URL.Path != "/platform/v1/wallet" {
			walletError(w, 404, "NOT_FOUND")
			return
		}
		method := http.MethodGet
		if initialize {
			method = http.MethodPost
		}
		if r.Method != method {
			w.Header().Set("Allow", method)
			walletError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			walletError(w, 400, "INVALID_REQUEST")
			return
		}
		for k, v := range q {
			if len(v) != 1 || !ledger || (k != "asset" && k != "after_seq" && k != "limit") {
				walletError(w, 400, "INVALID_REQUEST")
				return
			}
		}
		if initialize {
			if origins := r.Header.Values("Origin"); len(origins) != 1 || origins[0] != origin {
				walletError(w, 403, "ORIGIN_REJECTED")
				return
			}
			ct, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || ct != "application/json" || len(r.Header.Values("Content-Type")) != 1 {
				walletError(w, 415, "INVALID_CONTENT_TYPE")
				return
			}
			body, err := io.ReadAll(io.LimitReader(r.Body, 1025))
			var object map[string]json.RawMessage
			if err != nil || len(body) > 1024 || json.Unmarshal(body, &object) != nil || object == nil || len(object) != 0 {
				walletError(w, 400, "INVALID_REQUEST")
				return
			}
			if err := store.EnsureAccount(ctx, id); err != nil {
				walletError(w, 503, "WALLET_UNAVAILABLE")
				return
			}
		}
		if ledger {
			asset := platform.Asset(q.Get("asset"))
			if asset != platform.ReserveAPICredit && asset != platform.AvailableChips {
				walletError(w, 400, "INVALID_REQUEST")
				return
			}
			after := int64(0)
			limit := int64(20)
			if v, ok := q["after_seq"]; ok {
				after, err = decimalInt(v[0])
				if err != nil {
					walletError(w, 400, "INVALID_REQUEST")
					return
				}
			}
			if v, ok := q["limit"]; ok {
				limit, err = decimalInt(v[0])
				if err != nil || limit < 1 || limit > 50 {
					walletError(w, 400, "INVALID_REQUEST")
					return
				}
			}
			wallets, err := store.ReadWallets(ctx, id)
			if err != nil {
				walletError(w, 503, "WALLET_UNAVAILABLE")
				return
			}
			entries := []platform.LedgerEntry{}
			if len(wallets) > 0 {
				entries, err = store.Ledger(ctx, id, asset, after, int(limit)+1)
				if err != nil {
					walletError(w, 503, "WALLET_UNAVAILABLE")
					return
				}
			}
			more := len(entries) > int(limit)
			if more {
				entries = entries[:limit]
			}
			var next *string
			type entry struct {
				platform.LedgerEntry
				DeltaAmount        string `json:"delta_amount"`
				BalanceAfterAmount string `json:"balance_after_amount"`
			}
			items := make([]entry, 0, len(entries))
			for _, e := range entries {
				items = append(items, entry{e, platform.FormatAmount(e.DeltaUnits), platform.FormatAmount(e.BalanceAfterUnits)})
			}
			if more {
				v := strconv.FormatInt(entries[len(entries)-1].LedgerSeq, 10)
				next = &v
			}
			walletSuccess(w, struct {
				Items   []entry `json:"items"`
				HasMore bool    `json:"has_more"`
				Next    *string `json:"next_after_seq"`
			}{items, more, next})
			return
		}
		wallets, err := store.ReadWallets(ctx, id)
		if err != nil {
			walletError(w, 503, "WALLET_UNAVAILABLE")
			return
		}
		views := make([]walletView, 0, len(wallets))
		for _, v := range wallets {
			views = append(views, walletView{v.Asset, v.BalanceUnits, platform.FormatAmount(v.BalanceUnits), v.LedgerSeq, v.Version})
		}
		walletSuccess(w, struct {
			Initialized bool         `json:"initialized"`
			UserID      int64        `json:"user_id,string"`
			Wallets     []walletView `json:"wallets"`
			Scope       string       `json:"scope"`
			TotalAssets any          `json:"total_assets"`
		}{len(wallets) == 2, id, views, "LOCAL_WALLETS_ONLY", nil})
	})
}
