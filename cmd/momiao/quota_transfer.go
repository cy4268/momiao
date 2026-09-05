package main

import (
	"context"
	"errors"
	"github.com/cy4268/momiao/internal/platform"
	"mime"
	"net/http"
	"net/url"
	"time"
)

type quotaTransferStore interface {
	CreateQuotaTransfer(context.Context, int64, string, int64) (platform.QuotaTransfer, error)
	QuotaTransferByKey(context.Context, int64, string) (*platform.QuotaTransfer, error)
	QuotaTransfers(context.Context, int64) ([]platform.QuotaTransfer, error)
}
type nativeQuotaReader interface {
	ReadNativeQuota(context.Context, int64) (platform.NativeQuotaSnapshot, error)
}

func newQuotaHandler(origin string, store quotaTransferStore, native nativeQuotaReader, transport http.RoundTripper) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		user, authStatus := verifyWalletUser(r, transport)
		if authStatus != 0 {
			code := "AUTH_UNAVAILABLE"
			if authStatus == 401 {
				code = "AUTH_UNAUTHORIZED"
			}
			if authStatus == 403 {
				code = "AUTH_FORBIDDEN"
			}
			walletError(w, authStatus, code)
			return
		}
		if store == nil || native == nil {
			walletError(w, 503, "QUOTA_TRANSFER_UNAVAILABLE")
			return
		}
		path := r.URL.Path
		read := path == "/platform/v1/native-quota"
		list := path == "/platform/v1/quota-transfers"
		lookup := path == "/platform/v1/quota-transfers/by-key"
		if !read && !list && !lookup {
			walletError(w, 404, "NOT_FOUND")
			return
		}
		if r.Method != "GET" && !(list && r.Method == "POST") {
			allow := "GET"
			if list {
				allow = "GET, POST"
			}
			w.Header().Set("Allow", allow)
			walletError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || r.URL.ForceQuery {
			walletError(w, 400, "INVALID_REQUEST")
			return
		}
		if (!lookup && len(q) != 0) || (lookup && (len(q) != 1 || len(q["key"]) != 1 || !platform.ValidOperationKey(q.Get("key")))) {
			walletError(w, 400, "INVALID_REQUEST")
			return
		}
		var result any
		status := 200
		if r.Method == "POST" {
			if v := r.Header.Values("Origin"); len(v) != 1 || v[0] != origin {
				walletError(w, 403, "ORIGIN_REJECTED")
				return
			}
			ct, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if e != nil || ct != "application/json" || len(r.Header.Values("Content-Type")) != 1 {
				walletError(w, 415, "INVALID_CONTENT_TYPE")
				return
			}
			body, e := decodeStringFields(r.Body, "idempotency_key", "amount")
			if e != nil || !platform.ValidOperationKey(body["idempotency_key"]) {
				walletError(w, 400, "INVALID_REQUEST")
				return
			}
			units, e := platform.ParseAmount(body["amount"])
			if e != nil || units > 9007199254740991 {
				walletError(w, 400, "INVALID_AMOUNT")
				return
			}
			// Existing receipts remain readable/replayable even when the target is temporarily disabled.
			old, e := store.QuotaTransferByKey(ctx, user, body["idempotency_key"])
			if e != nil {
				walletError(w, 503, "QUOTA_TRANSFER_UNAVAILABLE")
				return
			}
			if old != nil {
				if old.AmountUnits != units {
					walletError(w, 409, "IDEMPOTENCY_CONFLICT")
					return
				}
				result = old
			} else {
				snapshot, e := native.ReadNativeQuota(ctx, user)
				if e != nil || !snapshot.Enabled {
					walletError(w, 503, "QUOTA_TRANSFER_UNAVAILABLE")
					return
				}
				result, err = store.CreateQuotaTransfer(ctx, user, body["idempotency_key"], units)
				status = 202
			}
		} else if read {
			result, err = native.ReadNativeQuota(ctx, user)
		} else if lookup {
			result, err = store.QuotaTransferByKey(ctx, user, q.Get("key"))
		} else {
			result, err = store.QuotaTransfers(ctx, user)
		}
		if err != nil {
			status, code := 503, "QUOTA_TRANSFER_UNAVAILABLE"
			switch {
			case errors.Is(err, platform.ErrInsufficientBalance):
				status, code = 409, "INSUFFICIENT_BALANCE"
			case errors.Is(err, platform.ErrWalletNotFound):
				status, code = 409, "WALLET_NOT_INITIALIZED"
			case errors.Is(err, platform.ErrIdempotencyConflict):
				status, code = 409, "IDEMPOTENCY_CONFLICT"
			case errors.Is(err, platform.ErrTransferPending):
				status, code = 409, "TRANSFER_PENDING"
			case errors.Is(err, platform.ErrInvalidMutation):
				status, code = 400, "INVALID_REQUEST"
			}
			walletError(w, status, code)
			return
		}
		walletJSON(w, status, struct {
			Success bool `json:"success"`
			Data    any  `json:"data"`
		}{true, result})
	})
}

// The bounded background worker owns recovery, not the request or browser.
func runQuotaWorker(ctx context.Context, store *platform.Store, native *platform.NativeQuota) {
	for {
		if ctx.Err() != nil {
			return
		}
		call, cancel := context.WithTimeout(ctx, 5*time.Second)
		worked, err := store.ProcessQuotaTransfer(call, native)
		cancel()
		if worked && err == nil {
			continue
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
