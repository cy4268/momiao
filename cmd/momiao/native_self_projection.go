package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/nativeself"
)

const nativeSelfMaxSafeInteger int64 = 9007199254740991

type nativeSelfProjection struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	// Native UI metadata only; platform authorization never consumes this DTO.
	Role         int64  `json:"role"`
	Group        string `json:"group"`
	Quota        int64  `json:"quota"`
	UsedQuota    int64  `json:"used_quota"`
	RequestCount int64  `json:"request_count"`
}

func nativeSelfField(fields map[string]json.RawMessage, name string, out any) bool {
	raw, ok := fields[name]
	return ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) && json.Unmarshal(raw, out) == nil
}

func nativeSelfSafeInteger(value int64) bool {
	return value >= 0 && value <= nativeSelfMaxSafeInteger
}

func nativeSelfText(value string, limit int, required bool) bool {
	return (!required || value != "") && utf8.RuneCountInString(value) <= limit
}

func nativeSelfProjectionError(w http.ResponseWriter, status int) {
	code := "AUTH_UNAVAILABLE"
	if status == http.StatusUnauthorized {
		code = "AUTH_UNAUTHORIZED"
	} else if status == http.StatusForbidden {
		code = "AUTH_FORBIDDEN"
	}
	walletError(w, status, code)
}

func newNativeSelfProjectionHandler(transport http.RoundTripper) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/self" {
			walletError(w, http.StatusNotFound, "NOT_FOUND")
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			walletError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
			return
		}
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			walletError(w, http.StatusBadRequest, "INVALID_REQUEST")
			return
		}
		claimed, ok := authHeader(r, "New-Api-User", 19, true)
		userID, err := decimalInt(claimed)
		// SessionCredential is a shape gate; F4 and this Native call establish authority.
		if !ok || err != nil || userID <= 0 || userID > nativeSelfMaxSafeInteger || !nativeself.SessionCredential(r) {
			nativeSelfProjectionError(w, http.StatusUnauthorized)
			return
		}
		if transport == nil {
			nativeSelfProjectionError(w, http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		upstream, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/api/user/self", nil)
		upstream.Host = "localhost"
		upstream.Header.Set("Accept", "application/json")
		upstream.Header.Set("Authorization", r.Header.Get("Authorization"))
		upstream.Header.Set("New-Api-User", claimed)
		upstream.Header.Set("X-Auth-Session", r.Header.Get("X-Auth-Session"))
		response, err := transport.RoundTrip(upstream)
		if err != nil || response == nil || response.Body == nil {
			if response != nil && response.Body != nil {
				_ = response.Body.Close()
			}
			nativeSelfProjectionError(w, http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		if ctx.Err() != nil {
			nativeSelfProjectionError(w, http.StatusBadGateway)
			return
		}
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			nativeSelfProjectionError(w, response.StatusCode)
			return
		}
		if response.StatusCode != http.StatusOK {
			nativeSelfProjectionError(w, http.StatusBadGateway)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, 65537))
		decoder := json.NewDecoder(bytes.NewReader(raw))
		if err != nil || len(raw) > 65536 || !utf8.Valid(raw) || ctx.Err() != nil || !nativeself.UniqueJSON(decoder, 0) {
			nativeSelfProjectionError(w, http.StatusBadGateway)
			return
		}
		if _, err = decoder.Token(); err != io.EOF {
			nativeSelfProjectionError(w, http.StatusBadGateway)
			return
		}
		var envelope map[string]json.RawMessage
		var fields map[string]json.RawMessage
		var success bool
		if json.Unmarshal(raw, &envelope) != nil || envelope == nil || !nativeSelfField(envelope, "success", &success) || !success || !nativeSelfField(envelope, "data", &fields) || fields == nil {
			nativeSelfProjectionError(w, http.StatusBadGateway)
			return
		}
		var data nativeSelfProjection
		var status int64
		valid := nativeSelfField(fields, "id", &data.ID) && nativeSelfField(fields, "username", &data.Username) && nativeSelfField(fields, "display_name", &data.DisplayName) && nativeSelfField(fields, "role", &data.Role) && nativeSelfField(fields, "status", &status) && nativeSelfField(fields, "group", &data.Group) && nativeSelfField(fields, "quota", &data.Quota) && nativeSelfField(fields, "used_quota", &data.UsedQuota) && nativeSelfField(fields, "request_count", &data.RequestCount)
		valid = valid && data.ID > 0 && nativeSelfSafeInteger(data.ID) && nativeSelfSafeInteger(data.Role) && nativeSelfSafeInteger(status) && nativeSelfSafeInteger(data.Quota) && nativeSelfSafeInteger(data.UsedQuota) && nativeSelfSafeInteger(data.RequestCount) && nativeSelfText(data.Username, 20, true) && nativeSelfText(data.DisplayName, 20, false) && nativeSelfText(data.Group, 64, false)
		if !valid {
			nativeSelfProjectionError(w, http.StatusBadGateway)
			return
		}
		if data.ID != userID || status != 1 {
			nativeSelfProjectionError(w, http.StatusUnauthorized)
			return
		}
		walletSuccess(w, data)
	})
}
