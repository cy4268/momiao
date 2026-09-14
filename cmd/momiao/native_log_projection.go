package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/nativeself"
)

const nativeLogProjectionPath = "/api/log/self"

type nativeLogProjectionItem struct {
	ID               int64  `json:"id"`
	CreatedAt        int64  `json:"created_at"`
	Type             int64  `json:"type"`
	TokenName        string `json:"token_name"`
	ModelName        string `json:"model_name"`
	Quota            int64  `json:"quota"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
}

type nativeLogProjectionPage struct {
	Items    []nativeLogProjectionItem `json:"items"`
	Total    int64                     `json:"total"`
	Page     int64                     `json:"page"`
	PageSize int64                     `json:"page_size"`
}

func nativeLogUnsigned(value string) (int64, bool) {
	if value == "" || strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, false
	}
	n, err := strconv.ParseInt(value, 10, 64)
	return n, err == nil && nativeSelfSafeInteger(n)
}

func nativeLogProjectionQuery(r *http.Request) (url.Values, int64, int64, bool) {
	if r.URL.ForceQuery && r.URL.RawQuery == "" {
		return nil, 0, 0, false
	}
	if r.URL.RawQuery != "" {
		for _, part := range strings.Split(r.URL.RawQuery, "&") {
			if part == "" {
				return nil, 0, 0, false
			}
		}
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, 0, 0, false
	}
	page, size, logType := int64(1), int64(10), int64(0)
	var start, end int64
	model := ""
	for key, entries := range values {
		if len(entries) != 1 {
			return nil, 0, 0, false
		}
		value := entries[0]
		switch key {
		case "p", "page_size":
			n, ok := nativeTokenListPositive(value)
			if !ok {
				return nil, 0, 0, false
			}
			if key == "p" {
				page = n
			} else {
				size = n
			}
		case "type", "start_timestamp", "end_timestamp":
			n, ok := nativeLogUnsigned(value)
			if !ok {
				return nil, 0, 0, false
			}
			if key == "type" {
				logType = n
			} else if key == "start_timestamp" {
				start = n
			} else {
				end = n
			}
		case "model_name":
			if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 200 {
				return nil, 0, 0, false
			}
			model = value
		default:
			return nil, 0, 0, false
		}
	}
	if size > 100 || page-1 > nativeTokenListMaxOffset/size || logType > 7 || (start != 0 && end != 0 && start > end) {
		return nil, 0, 0, false
	}
	query := url.Values{"p": {strconv.FormatInt(page, 10)}, "page_size": {strconv.FormatInt(size, 10)}, "type": {strconv.FormatInt(logType, 10)}}
	if model != "" {
		query.Set("model_name", model)
	}
	if start != 0 {
		query.Set("start_timestamp", strconv.FormatInt(start, 10))
	}
	if end != 0 {
		query.Set("end_timestamp", strconv.FormatInt(end, 10))
	}
	return query, page, size, true
}

func parseNativeLogProjectionItem(raw json.RawMessage, claimed int64) (nativeLogProjectionItem, int) {
	var fields map[string]json.RawMessage
	var item nativeLogProjectionItem
	var owner int64
	if json.Unmarshal(raw, &fields) != nil || fields == nil || !nativeSelfField(fields, "id", &item.ID) ||
		!nativeSelfField(fields, "user_id", &owner) || !nativeSelfField(fields, "created_at", &item.CreatedAt) ||
		!nativeSelfField(fields, "type", &item.Type) || !nativeSelfField(fields, "token_name", &item.TokenName) ||
		!nativeSelfField(fields, "model_name", &item.ModelName) || !nativeSelfField(fields, "quota", &item.Quota) ||
		!nativeSelfField(fields, "prompt_tokens", &item.PromptTokens) || !nativeSelfField(fields, "completion_tokens", &item.CompletionTokens) {
		return item, http.StatusBadGateway
	}
	valid := item.ID > 0 && nativeSelfSafeInteger(item.ID) && owner > 0 && nativeSelfSafeInteger(owner) &&
		nativeSelfSafeInteger(item.CreatedAt) && item.Type >= 0 && item.Type <= 7 && nativeTokenListSigned(item.Quota) &&
		nativeSelfSafeInteger(item.PromptTokens) && nativeSelfSafeInteger(item.CompletionTokens)
	if !valid {
		return item, http.StatusBadGateway
	}
	if owner != claimed {
		return item, http.StatusUnauthorized
	}
	return item, 0
}

func parseNativeLogProjectionPage(raw json.RawMessage, claimed, page, size int64) (nativeLogProjectionPage, int) {
	var fields map[string]json.RawMessage
	var rawItems []json.RawMessage
	var result nativeLogProjectionPage
	if json.Unmarshal(raw, &fields) != nil || fields == nil || !nativeSelfField(fields, "total", &result.Total) ||
		!nativeSelfField(fields, "page", &result.Page) || !nativeSelfField(fields, "page_size", &result.PageSize) {
		return result, http.StatusBadGateway
	}
	items, ok := fields["items"]
	if !ok || json.Unmarshal(items, &rawItems) != nil || result.Total < 0 || !nativeSelfSafeInteger(result.Total) ||
		result.Page != page || result.PageSize != size || int64(len(rawItems)) > size {
		return result, http.StatusBadGateway
	}
	result.Items = make([]nativeLogProjectionItem, len(rawItems))
	seen := make(map[int64]struct{}, len(rawItems))
	for index, rawItem := range rawItems {
		item, status := parseNativeLogProjectionItem(rawItem, claimed)
		if status != 0 {
			return nativeLogProjectionPage{}, status
		}
		if _, exists := seen[item.ID]; exists {
			return nativeLogProjectionPage{}, http.StatusBadGateway
		}
		seen[item.ID] = struct{}{}
		result.Items[index] = item
	}
	return result, 0
}

func newNativeLogProjectionHandler(transport http.RoundTripper) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != nativeLogProjectionPath {
			walletError(w, http.StatusNotFound, "NOT_FOUND")
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			walletError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
			return
		}
		query, page, size, ok := nativeLogProjectionQuery(r)
		if !ok {
			walletError(w, http.StatusBadRequest, "INVALID_REQUEST")
			return
		}
		claimed, ok := authHeader(r, "New-Api-User", 19, true)
		userID, err := decimalInt(claimed)
		// Shape only: preceding F4 and the fixed Native route establish authority.
		if !ok || err != nil || userID <= 0 || !nativeSelfSafeInteger(userID) || !nativeself.SessionCredential(r) {
			nativeSelfProjectionError(w, http.StatusUnauthorized)
			return
		}
		data, status := nativeWorkspaceData(r, transport, nativeLogProjectionPath, query)
		if status != 0 {
			nativeSelfProjectionError(w, status)
			return
		}
		result, status := parseNativeLogProjectionPage(data, userID, page, size)
		if status != 0 {
			nativeSelfProjectionError(w, status)
			return
		}
		walletSuccess(w, result)
	})
}
