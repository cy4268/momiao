package main

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/platform"
)

type keyPurposeStore interface {
	KeyPurposes(context.Context, int64) ([]platform.KeyPurposeItem, error)
	KeyPurposeOperation(context.Context, int64, string) (*platform.KeyPurposeOperation, error)
	SetKeyPurpose(context.Context, platform.KeyPurposeCommand) (platform.KeyPurposeItem, error)
}

func keyPurposeTokenID(path string) (int64, bool) {
	const prefix = "/platform/v1/key-purposes/"
	if !strings.HasPrefix(path, prefix) {
		return 0, false
	}
	value := strings.TrimPrefix(path, prefix)
	if value == "" || strings.Contains(value, "/") || value[0] == '0' {
		return 0, false
	}
	id, err := strconv.ParseInt(value, 10, 64)
	return id, err == nil && id > 0 && id <= 1<<31-1 && strconv.FormatInt(id, 10) == value
}

func newKeyPurposeHandler(origin string, store keyPurposeStore, native platform.NativePurposeWriter, transport http.RoundTripper) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		user, authStatus := verifyWalletUser(r, transport)
		if authStatus != 0 {
			code := "AUTH_UNAVAILABLE"
			if authStatus == http.StatusUnauthorized {
				code = "AUTH_UNAUTHORIZED"
			} else if authStatus == http.StatusForbidden {
				code = "AUTH_FORBIDDEN"
			}
			walletError(w, authStatus, code)
			return
		}
		if store == nil || r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery {
			walletError(w, http.StatusServiceUnavailable, "KEY_PURPOSE_UNAVAILABLE")
			return
		}
		collection := r.URL.Path == "/platform/v1/key-purposes"
		tokenID, item := keyPurposeTokenID(r.URL.Path)
		if !collection && !item {
			walletError(w, http.StatusNotFound, "NOT_FOUND")
			return
		}
		if collection && r.Method == http.MethodGet {
			items, err := store.KeyPurposes(ctx, user)
			if err != nil {
				walletError(w, http.StatusServiceUnavailable, "KEY_PURPOSE_UNAVAILABLE")
				return
			}
			walletSuccess(w, struct {
				Items []platform.KeyPurposeItem `json:"items"`
			}{Items: items})
			return
		}
		if !((collection && r.Method == http.MethodPost) || (item && r.Method == http.MethodPut)) {
			allow := http.MethodPut
			if collection {
				allow = "GET, POST"
			}
			w.Header().Set("Allow", allow)
			walletError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
			return
		}
		if native == nil {
			walletError(w, http.StatusServiceUnavailable, "KEY_PURPOSE_UNAVAILABLE")
			return
		}
		if values := r.Header.Values("Origin"); len(values) != 1 || values[0] != origin {
			walletError(w, http.StatusForbidden, "ORIGIN_REJECTED")
			return
		}
		mediaType, parameters, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" || len(parameters) != 0 || len(r.Header.Values("Content-Type")) != 1 {
			walletError(w, http.StatusUnsupportedMediaType, "INVALID_CONTENT_TYPE")
			return
		}
		fields := []string{"operation_id", "token_id", "purpose"}
		if item {
			fields = []string{"operation_id", "purpose", "expected_version"}
		}
		body, err := decodeStringFields(r.Body, fields...)
		if err != nil || !platform.ValidOperationKey(body["operation_id"]) {
			walletError(w, http.StatusBadRequest, "INVALID_REQUEST")
			return
		}
		if collection {
			var ok bool
			tokenID, ok = keyPurposeTokenID("/platform/v1/key-purposes/" + body["token_id"])
			if !ok {
				walletError(w, http.StatusBadRequest, "INVALID_REQUEST")
				return
			}
		}
		expected := int64(0)
		if item {
			expected, err = platform.ParseKeyPurposeVersion(body["expected_version"])
			if err != nil || expected == 0 {
				walletError(w, http.StatusBadRequest, "INVALID_REQUEST")
				return
			}
		}
		purpose := platform.KeyPurpose(body["purpose"])
		if purpose != platform.KeyPurposeGeneral && purpose != platform.KeyPurposeRoleplay {
			walletError(w, http.StatusBadRequest, "INVALID_REQUEST")
			return
		}
		command := platform.KeyPurposeCommand{OperationID: body["operation_id"], UserID: user, TokenID: tokenID, Purpose: purpose, ExpectedVersion: expected}
		prior, err := store.KeyPurposeOperation(ctx, user, command.OperationID)
		if err != nil {
			walletError(w, http.StatusServiceUnavailable, "KEY_PURPOSE_UNAVAILABLE")
			return
		}
		if prior != nil {
			if prior.Item.TokenID != tokenID || prior.Item.Purpose != purpose || prior.ExpectedVersion != expected {
				walletError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT")
				return
			}
			walletSuccess(w, prior.Item)
			return
		}
		owned, err := native.TokenOwnedByUser(ctx, user, tokenID)
		if err != nil {
			walletError(w, http.StatusServiceUnavailable, "KEY_PURPOSE_UNAVAILABLE")
			return
		}
		if !owned {
			walletError(w, http.StatusNotFound, "KEY_NOT_FOUND")
			return
		}
		result, err := store.SetKeyPurpose(ctx, command)
		if err != nil {
			status, code := http.StatusServiceUnavailable, "KEY_PURPOSE_UNAVAILABLE"
			switch {
			case errors.Is(err, platform.ErrMaintenanceActive):
				status, code = http.StatusServiceUnavailable, "MAINTENANCE_ACTIVE"
			case errors.Is(err, platform.ErrInvalidMutation):
				status, code = http.StatusBadRequest, "INVALID_REQUEST"
			case errors.Is(err, platform.ErrIdempotencyConflict):
				status, code = http.StatusConflict, "IDEMPOTENCY_CONFLICT"
			case errors.Is(err, platform.ErrKeyPurposeVersionStale):
				status, code = http.StatusConflict, "KEY_PURPOSE_VERSION_STALE"
			case errors.Is(err, platform.ErrKeyPurposePending):
				status, code = http.StatusConflict, "KEY_PURPOSE_PENDING"
			}
			walletError(w, status, code)
			return
		}
		walletSuccess(w, result)
	})
}
