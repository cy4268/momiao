package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/nativeself"
)

const (
	nativeTokenWritePath            = "/api/token/"
	nativeTokenWriteMaxRequestBody  = 16 << 10
	nativeTokenWriteMaxResponseBody = 1 << 20
)

type nativeTokenWriteOperation uint8

const (
	nativeTokenWriteNone nativeTokenWriteOperation = iota
	nativeTokenWriteCreate
	nativeTokenWriteEdit
	nativeTokenWriteStatus
	nativeTokenWriteReveal
	nativeTokenWriteDelete
)

type nativeTokenWriteRoute struct {
	operation nativeTokenWriteOperation
	id        int64
	target    string
}

type nativeTokenCreate struct {
	Name               string `json:"name"`
	RemainQuota        int64  `json:"remain_quota"`
	UnlimitedQuota     bool   `json:"unlimited_quota"`
	ExpiredTime        int64  `json:"expired_time"`
	ModelLimitsEnabled bool   `json:"model_limits_enabled"`
	ModelLimits        string `json:"model_limits"`
	AllowIPs           string `json:"allow_ips"`
	Group              string `json:"group"`
	CrossGroupRetry    bool   `json:"cross_group_retry"`
}

type nativeTokenStatus struct {
	ID     int64 `json:"id"`
	Status int64 `json:"status"`
}

type nativeTokenEdit struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	RemainQuota    int64  `json:"remain_quota"`
	UnlimitedQuota bool   `json:"unlimited_quota"`
	ExpiredTime    int64  `json:"expired_time"`
}

type nativeTokenReveal struct {
	Key string `json:"key"`
}

func nativeTokenWriteID(value string) (int64, bool) {
	if value == "" || value[0] == '0' || strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, false
	}
	id, err := strconv.ParseInt(value, 10, 64)
	return id, err == nil && id > 0 && nativeSelfSafeInteger(id)
}

func classifyNativeTokenWrite(r *http.Request) (nativeTokenWriteRoute, int, string) {
	if r.URL.RawPath != "" {
		return nativeTokenWriteRoute{}, http.StatusNotFound, ""
	}
	if r.URL.Path == nativeTokenWritePath {
		switch r.Method {
		case http.MethodPost:
			if r.URL.RawQuery != "" || r.URL.ForceQuery {
				return nativeTokenWriteRoute{}, http.StatusBadRequest, ""
			}
			return nativeTokenWriteRoute{operation: nativeTokenWriteCreate, target: "http://unix/api/token/"}, 0, ""
		case http.MethodPut:
			if r.URL.ForceQuery {
				return nativeTokenWriteRoute{}, http.StatusBadRequest, ""
			}
			if r.URL.RawQuery == "" {
				return nativeTokenWriteRoute{operation: nativeTokenWriteEdit, target: "http://unix/api/token/?basic_only=true"}, 0, ""
			}
			if r.URL.RawQuery == "status_only=true" {
				return nativeTokenWriteRoute{operation: nativeTokenWriteStatus, target: "http://unix/api/token/?status_only=true"}, 0, ""
			}
			return nativeTokenWriteRoute{}, http.StatusBadRequest, ""
		default:
			return nativeTokenWriteRoute{}, http.StatusMethodNotAllowed, "POST, PUT"
		}
	}
	if !strings.HasPrefix(r.URL.Path, nativeTokenWritePath) {
		return nativeTokenWriteRoute{}, http.StatusNotFound, ""
	}
	rest := strings.TrimPrefix(r.URL.Path, nativeTokenWritePath)
	operation, allow := nativeTokenWriteDelete, http.MethodDelete
	idText := rest
	if strings.HasSuffix(rest, "/key") {
		operation, allow = nativeTokenWriteReveal, http.MethodPost
		idText = strings.TrimSuffix(rest, "/key")
	}
	if idText == "" || strings.Contains(idText, "/") {
		return nativeTokenWriteRoute{}, http.StatusNotFound, ""
	}
	id, ok := nativeTokenWriteID(idText)
	if !ok {
		return nativeTokenWriteRoute{}, http.StatusBadRequest, ""
	}
	if r.Method != allow {
		return nativeTokenWriteRoute{}, http.StatusMethodNotAllowed, allow
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		return nativeTokenWriteRoute{}, http.StatusBadRequest, ""
	}
	return nativeTokenWriteRoute{operation: operation, id: id, target: "http://unix" + r.URL.Path}, 0, ""
}

func nativeTokenWriteJSONBody(r *http.Request) (map[string]json.RawMessage, bool) {
	if r.Body == nil {
		return nil, false
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, nativeTokenWriteMaxRequestBody+1))
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err != nil || len(raw) == 0 || len(raw) > nativeTokenWriteMaxRequestBody || !utf8.Valid(raw) || !nativeself.UniqueJSON(decoder, 0) {
		return nil, false
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, false
	}
	return fields, true
}

func nativeTokenWriteCreateBody(r *http.Request) ([]byte, bool) {
	fields, ok := nativeTokenWriteJSONBody(r)
	var input nativeTokenCreate
	if !ok || len(fields) != 9 || !nativeSelfField(fields, "name", &input.Name) ||
		!nativeSelfField(fields, "remain_quota", &input.RemainQuota) || !nativeSelfField(fields, "unlimited_quota", &input.UnlimitedQuota) ||
		!nativeSelfField(fields, "expired_time", &input.ExpiredTime) || !nativeSelfField(fields, "model_limits_enabled", &input.ModelLimitsEnabled) ||
		!nativeSelfField(fields, "model_limits", &input.ModelLimits) || !nativeSelfField(fields, "allow_ips", &input.AllowIPs) ||
		!nativeSelfField(fields, "group", &input.Group) || !nativeSelfField(fields, "cross_group_retry", &input.CrossGroupRetry) {
		return nil, false
	}
	input.Name = strings.TrimSpace(input.Name)
	quotaOK := (!input.UnlimitedQuota && input.RemainQuota >= 1 && input.RemainQuota <= 1_000_000_000_000) || (input.UnlimitedQuota && input.RemainQuota == 0)
	expiryOK := input.ExpiredTime == -1 || nativeSelfSafeInteger(input.ExpiredTime)
	if input.Name == "" || len(input.Name) > 50 || !quotaOK || !expiryOK || input.ModelLimitsEnabled || input.ModelLimits != "" || input.AllowIPs != "" || input.Group != "" || input.CrossGroupRetry {
		return nil, false
	}
	body, err := json.Marshal(input)
	return body, err == nil
}

func nativeTokenWriteStatusBody(r *http.Request) ([]byte, nativeTokenStatus, bool) {
	fields, ok := nativeTokenWriteJSONBody(r)
	var input nativeTokenStatus
	if !ok || len(fields) != 2 || !nativeSelfField(fields, "id", &input.ID) || !nativeSelfField(fields, "status", &input.Status) ||
		input.ID <= 0 || !nativeSelfSafeInteger(input.ID) || (input.Status != 1 && input.Status != 2) {
		return nil, input, false
	}
	body, err := json.Marshal(input)
	return body, input, err == nil
}

func nativeTokenWriteEditBody(r *http.Request) ([]byte, nativeTokenEdit, bool) {
	fields, ok := nativeTokenWriteJSONBody(r)
	var input nativeTokenEdit
	if !ok || len(fields) != 5 || !nativeSelfField(fields, "id", &input.ID) || !nativeSelfField(fields, "name", &input.Name) ||
		!nativeSelfField(fields, "remain_quota", &input.RemainQuota) || !nativeSelfField(fields, "unlimited_quota", &input.UnlimitedQuota) ||
		!nativeSelfField(fields, "expired_time", &input.ExpiredTime) {
		return nil, input, false
	}
	input.Name = strings.TrimSpace(input.Name)
	quotaOK := (!input.UnlimitedQuota && input.RemainQuota >= 1 && input.RemainQuota <= 2_147_483_647) || (input.UnlimitedQuota && input.RemainQuota == 0)
	expiryOK := input.ExpiredTime == -1 || nativeSelfSafeInteger(input.ExpiredTime)
	if input.ID <= 0 || !nativeSelfSafeInteger(input.ID) || input.Name == "" || len(input.Name) > 50 || !quotaOK || !expiryOK {
		return nil, input, false
	}
	body, err := json.Marshal(input)
	return body, input, err == nil
}

func nativeTokenWriteEmptyBody(r *http.Request) bool {
	if r.Body == nil {
		return true
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1))
	return err == nil && len(body) == 0
}

func nativeTokenWriteRoundTrip(r *http.Request, transport http.RoundTripper, route nativeTokenWriteRoute, body []byte) (map[string]json.RawMessage, int) {
	if transport == nil {
		return nil, http.StatusServiceUnavailable
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	upstream, _ := http.NewRequestWithContext(ctx, r.Method, route.target, reader)
	upstream.Host = "localhost"
	upstream.Header.Set("Accept", "application/json")
	if body != nil {
		upstream.Header.Set("Content-Type", "application/json")
	}
	upstream.Header.Set("Authorization", r.Header.Get("Authorization"))
	upstream.Header.Set("New-Api-User", r.Header.Get("New-Api-User"))
	upstream.Header.Set("X-Auth-Session", r.Header.Get("X-Auth-Session"))
	response, err := transport.RoundTrip(upstream)
	if err != nil || response == nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, http.StatusBadGateway
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	if ctx.Err() != nil {
		return nil, http.StatusBadGateway
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, response.StatusCode
	}
	if response.StatusCode != http.StatusOK || response.Body == nil {
		return nil, http.StatusBadGateway
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, nativeTokenWriteMaxResponseBody+1))
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err != nil || len(raw) > nativeTokenWriteMaxResponseBody || !utf8.Valid(raw) || ctx.Err() != nil || !nativeself.UniqueJSON(decoder, 0) {
		return nil, http.StatusBadGateway
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, http.StatusBadGateway
	}
	var envelope map[string]json.RawMessage
	var success bool
	if json.Unmarshal(raw, &envelope) != nil || envelope == nil || !nativeSelfField(envelope, "success", &success) || !success {
		return nil, http.StatusBadGateway
	}
	return envelope, 0
}

func nativeTokenWriteSuccess(w http.ResponseWriter) {
	walletJSON(w, http.StatusOK, struct {
		Success bool `json:"success"`
	}{true})
}

func newNativeTokenWriteProjectionHandler(transport http.RoundTripper) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route, status, allow := classifyNativeTokenWrite(r)
		if status != 0 {
			if allow != "" {
				w.Header().Set("Allow", allow)
			}
			code := "INVALID_REQUEST"
			if status == http.StatusNotFound {
				code = "NOT_FOUND"
			} else if status == http.StatusMethodNotAllowed {
				code = "METHOD_NOT_ALLOWED"
			}
			walletError(w, status, code)
			return
		}
		claimed, ok := authHeader(r, "New-Api-User", 19, true)
		userID, err := decimalInt(claimed)
		// Shape only: preceding F4 and the fixed Native route establish authority.
		if !ok || err != nil || userID <= 0 || !nativeSelfSafeInteger(userID) || !nativeself.SessionCredential(r) {
			nativeSelfProjectionError(w, http.StatusUnauthorized)
			return
		}
		var body []byte
		var requested nativeTokenStatus
		var edit nativeTokenEdit
		switch route.operation {
		case nativeTokenWriteCreate:
			body, ok = nativeTokenWriteCreateBody(r)
		case nativeTokenWriteEdit:
			body, edit, ok = nativeTokenWriteEditBody(r)
		case nativeTokenWriteStatus:
			body, requested, ok = nativeTokenWriteStatusBody(r)
		case nativeTokenWriteReveal, nativeTokenWriteDelete:
			ok = nativeTokenWriteEmptyBody(r)
		}
		if !ok {
			walletError(w, http.StatusBadRequest, "INVALID_REQUEST")
			return
		}
		envelope, status := nativeTokenWriteRoundTrip(r, transport, route, body)
		if status != 0 {
			nativeSelfProjectionError(w, status)
			return
		}
		switch route.operation {
		case nativeTokenWriteCreate:
			var fields map[string]json.RawMessage
			var idText, ownerText string
			if !nativeSelfField(envelope, "data", &fields) || len(fields) != 2 || !nativeSelfField(fields, "id", &idText) || !nativeSelfField(fields, "user_id", &ownerText) {
				nativeSelfProjectionError(w, http.StatusBadGateway)
				return
			}
			_, idOK := nativeTokenWriteID(idText)
			owner, ownerOK := nativeTokenWriteID(ownerText)
			if !idOK || !ownerOK {
				nativeSelfProjectionError(w, http.StatusBadGateway)
				return
			}
			if owner != userID {
				nativeSelfProjectionError(w, http.StatusUnauthorized)
				return
			}
			walletSuccess(w, struct{ ID string `json:"id"` }{idText})
			return
		case nativeTokenWriteEdit:
			data, exists := envelope["data"]
			item, resultStatus := parseNativeTokenListItem(data, userID)
			if !exists || resultStatus != 0 || item.ID != edit.ID || item.Name != edit.Name || item.RemainQuota != edit.RemainQuota ||
				item.UnlimitedQuota != edit.UnlimitedQuota || item.ExpiredTime != edit.ExpiredTime {
				if resultStatus == http.StatusUnauthorized {
					nativeSelfProjectionError(w, resultStatus)
				} else {
					nativeSelfProjectionError(w, http.StatusBadGateway)
				}
				return
			}
			walletSuccess(w, item)
			return
		case nativeTokenWriteStatus:
			data, exists := envelope["data"]
			item, resultStatus := parseNativeTokenListItem(data, userID)
			if !exists || resultStatus != 0 || item.ID != requested.ID || item.Status != requested.Status {
				if resultStatus == http.StatusUnauthorized {
					nativeSelfProjectionError(w, resultStatus)
				} else {
					nativeSelfProjectionError(w, http.StatusBadGateway)
				}
				return
			}
		case nativeTokenWriteReveal:
			var fields map[string]json.RawMessage
			var projected nativeTokenReveal
			if !nativeSelfField(envelope, "data", &fields) || fields == nil || !nativeSelfField(fields, "key", &projected.Key) || projected.Key == "" || utf8.RuneCountInString(projected.Key) > 128 {
				nativeSelfProjectionError(w, http.StatusBadGateway)
				return
			}
			walletSuccess(w, projected)
			return
		}
		nativeTokenWriteSuccess(w)
	})
}
