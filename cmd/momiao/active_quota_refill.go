package main

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/platform"
)

var errActiveQuotaRefillHandlerConfig = errors.New("active quota refill handler configuration invalid")

func newActiveQuotaRefillHandler(service *platform.ActiveQuotaRefillService, bridgeSecret string) (http.Handler, error) {
	decoded, err := hex.DecodeString(bridgeSecret)
	if service == nil || err != nil || len(decoded) != 32 || strings.ToLower(bridgeSecret) != bridgeSecret {
		return nil, errActiveQuotaRefillHandlerConfig
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Vary", "Authorization")
		if r.URL.Path != "/internal/momiao/active-quota/refill" || r.URL.RawPath != "" {
			activeQuotaRefillError(w, http.StatusNotFound, "NOT_FOUND")
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			activeQuotaRefillError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
			return
		}
		auth := r.Header.Values("Authorization")
		if len(auth) != 1 || subtle.ConstantTimeCompare([]byte(auth[0]), []byte("Bearer "+bridgeSecret)) != 1 {
			activeQuotaRefillError(w, http.StatusUnauthorized, "AUTH_UNAUTHORIZED")
			return
		}
		mediaType, parameters, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if mediaErr != nil || mediaType != "application/json" || len(parameters) != 0 || len(r.Header.Values("Content-Type")) != 1 ||
			r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength > 2048 {
			activeQuotaRefillError(w, http.StatusBadRequest, "INPUT_INVALID")
			return
		}
		body, err := decodeStringFields(r.Body, "request_id", "user_id", "required_raw_quota")
		user, userErr := strconv.ParseInt(body["user_id"], 10, 64)
		required, requiredErr := strconv.ParseInt(body["required_raw_quota"], 10, 64)
		if err != nil || userErr != nil || requiredErr != nil || user <= 0 || user > 1<<31-1 || required <= 0 || required > 1<<31-1 ||
			strconv.FormatInt(user, 10) != body["user_id"] || strconv.FormatInt(required, 10) != body["required_raw_quota"] {
			activeQuotaRefillError(w, http.StatusBadRequest, "INPUT_INVALID")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 1400*time.Millisecond)
		defer cancel()
		result, err := service.Refill(ctx, platform.ActiveQuotaRefillRequest{RequestID: body["request_id"], UserID: user, RequiredRawQuota: required})
		if err != nil {
			status, code := http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE"
			if errors.Is(err, platform.ErrInvalidMutation) {
				status, code = http.StatusBadRequest, "INPUT_INVALID"
			}
			activeQuotaRefillError(w, status, code)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}), nil
}

func activeQuotaRefillError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Connection", "close")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Code string `json:"code"`
	}{Code: code})
}
