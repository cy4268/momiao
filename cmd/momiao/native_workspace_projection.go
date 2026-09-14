package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/nativeself"
)

const (
	nativeWorkspaceGroupsPath = "/api/user/self/groups"
	nativeWorkspaceModelsPath = "/api/user/models"
	nativeWorkspaceMaxBody    = 1 << 20
)

type nativeWorkspaceGroup struct {
	Ratio json.RawMessage `json:"ratio"`
	Desc  string          `json:"desc"`
}

func nativeWorkspaceRatio(name string, raw json.RawMessage) bool {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 {
		return false
	}
	if value[0] == '"' {
		var label string
		return name == "auto" && json.Unmarshal(value, &label) == nil && label == "自动"
	}
	if name == "auto" {
		return false
	}
	var number json.Number
	if json.Unmarshal(value, &number) != nil {
		return false
	}
	numeric, err := strconv.ParseFloat(number.String(), 64)
	return err == nil && numeric >= 0 && !math.IsInf(numeric, 0) && !math.IsNaN(numeric)
}

func nativeWorkspaceGroups(raw json.RawMessage) (map[string]nativeWorkspaceGroup, bool) {
	var groups map[string]json.RawMessage
	if json.Unmarshal(raw, &groups) != nil || groups == nil {
		return nil, false
	}
	projected := make(map[string]nativeWorkspaceGroup, len(groups))
	for name, rawGroup := range groups {
		var fields map[string]json.RawMessage
		if json.Unmarshal(rawGroup, &fields) != nil || fields == nil {
			return nil, false
		}
		ratio, ok := fields["ratio"]
		var desc string
		if !ok || !nativeWorkspaceRatio(name, ratio) || !nativeSelfField(fields, "desc", &desc) {
			return nil, false
		}
		projected[name] = nativeWorkspaceGroup{Ratio: append(json.RawMessage(nil), bytes.TrimSpace(ratio)...), Desc: desc}
	}
	return projected, true
}

func nativeWorkspaceModels(raw json.RawMessage) ([]string, bool) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return []string{}, true
	}
	var models []string
	if json.Unmarshal(raw, &models) != nil || models == nil {
		return nil, false
	}
	for _, model := range models {
		if model == "" {
			return nil, false
		}
	}
	return models, true
}

func nativeWorkspaceData(r *http.Request, transport http.RoundTripper, path string, query url.Values) (json.RawMessage, int) {
	if transport == nil {
		return nil, http.StatusServiceUnavailable
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	target := "http://unix" + path
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}
	upstream, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	upstream.Host = "localhost"
	upstream.Header.Set("Accept", "application/json")
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
	raw, err := io.ReadAll(io.LimitReader(response.Body, nativeWorkspaceMaxBody+1))
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err != nil || len(raw) > nativeWorkspaceMaxBody || !utf8.Valid(raw) || ctx.Err() != nil || !nativeself.UniqueJSON(decoder, 0) {
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
	data, ok := envelope["data"]
	if !ok {
		return nil, http.StatusBadGateway
	}
	return data, 0
}

func newNativeWorkspaceProjectionHandler(transport http.RoundTripper) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path != nativeWorkspaceGroupsPath && path != nativeWorkspaceModelsPath {
			walletError(w, http.StatusNotFound, "NOT_FOUND")
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			walletError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
			return
		}
		query := make(url.Values)
		if path == nativeWorkspaceGroupsPath {
			if r.URL.RawQuery != "" || r.URL.ForceQuery {
				walletError(w, http.StatusBadRequest, "INVALID_REQUEST")
				return
			}
		} else {
			parsed, err := url.ParseQuery(r.URL.RawQuery)
			if err != nil {
				walletError(w, http.StatusBadRequest, "INVALID_REQUEST")
				return
			}
			for key, values := range parsed {
				if !utf8.ValidString(key) || key != "group" || len(values) != 1 || !utf8.ValidString(values[0]) || utf8.RuneCountInString(values[0]) > 64 {
					walletError(w, http.StatusBadRequest, "INVALID_REQUEST")
					return
				}
				if values[0] != "" {
					query.Set("group", values[0])
				}
			}
		}
		claimed, ok := authHeader(r, "New-Api-User", 19, true)
		userID, err := decimalInt(claimed)
		// Shape only: preceding F4 and the fixed Native route establish authority.
		if !ok || err != nil || userID <= 0 || !nativeSelfSafeInteger(userID) || !nativeself.SessionCredential(r) {
			nativeSelfProjectionError(w, http.StatusUnauthorized)
			return
		}
		data, status := nativeWorkspaceData(r, transport, path, query)
		if status != 0 {
			nativeSelfProjectionError(w, status)
			return
		}
		if path == nativeWorkspaceGroupsPath {
			groups, valid := nativeWorkspaceGroups(data)
			if !valid {
				nativeSelfProjectionError(w, http.StatusBadGateway)
				return
			}
			walletSuccess(w, groups)
			return
		}
		models, valid := nativeWorkspaceModels(data)
		if !valid {
			nativeSelfProjectionError(w, http.StatusBadGateway)
			return
		}
		walletSuccess(w, models)
	})
}
