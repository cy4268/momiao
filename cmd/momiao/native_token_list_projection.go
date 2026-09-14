package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/cy4268/momiao/internal/nativeself"
)

const (
	nativeTokenListPath      = "/api/token/"
	nativeTokenListMaxOffset = int64(1<<31 - 1)
)

type nativeTokenListItem struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Key            string `json:"key"`
	Status         int64  `json:"status"`
	CreatedTime    int64  `json:"created_time"`
	ExpiredTime    int64  `json:"expired_time"`
	RemainQuota    int64  `json:"remain_quota"`
	UsedQuota      int64  `json:"used_quota"`
	UnlimitedQuota bool   `json:"unlimited_quota"`
}

type nativeTokenListPage struct {
	Items    []nativeTokenListItem `json:"items"`
	Total    int64                 `json:"total"`
	Page     int64                 `json:"page"`
	PageSize int64                 `json:"page_size"`
}

func nativeTokenListPositive(value string) (int64, bool) {
	if value == "" || strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, false
	}
	n, err := strconv.ParseInt(value, 10, 64)
	return n, err == nil && n > 0
}

func nativeTokenListQuery(r *http.Request) (url.Values, int64, int64, bool) {
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
	page, size := int64(1), int64(10)
	for key, entries := range values {
		if len(entries) != 1 || (key != "p" && key != "page_size") {
			return nil, 0, 0, false
		}
		n, ok := nativeTokenListPositive(entries[0])
		if !ok {
			return nil, 0, 0, false
		}
		if key == "p" {
			page = n
		} else {
			size = n
		}
	}
	if size > 100 || page-1 > nativeTokenListMaxOffset/size {
		return nil, 0, 0, false
	}
	return url.Values{"p": {strconv.FormatInt(page, 10)}, "page_size": {strconv.FormatInt(size, 10)}}, page, size, true
}

func nativeTokenListMaskedKey(key string) bool {
	switch len(key) {
	case 0:
		return true
	case 1, 2, 3, 4:
		return strings.Trim(key, "*") == ""
	case 8:
		return key[2:6] == "****"
	case 18:
		return key[4:14] == "**********"
	default:
		return false
	}
}

func nativeTokenListSigned(value int64) bool {
	return value >= -nativeSelfMaxSafeInteger && value <= nativeSelfMaxSafeInteger
}

func parseNativeTokenListItem(raw json.RawMessage, claimed int64) (nativeTokenListItem, int) {
	var fields map[string]json.RawMessage
	var item nativeTokenListItem
	var owner int64
	if json.Unmarshal(raw, &fields) != nil || fields == nil ||
		!nativeSelfField(fields, "id", &item.ID) || !nativeSelfField(fields, "user_id", &owner) ||
		!nativeSelfField(fields, "name", &item.Name) || !nativeSelfField(fields, "key", &item.Key) ||
		!nativeSelfField(fields, "status", &item.Status) || !nativeSelfField(fields, "created_time", &item.CreatedTime) ||
		!nativeSelfField(fields, "expired_time", &item.ExpiredTime) || !nativeSelfField(fields, "remain_quota", &item.RemainQuota) ||
		!nativeSelfField(fields, "used_quota", &item.UsedQuota) || !nativeSelfField(fields, "unlimited_quota", &item.UnlimitedQuota) {
		return item, http.StatusBadGateway
	}
	valid := item.ID > 0 && nativeSelfSafeInteger(item.ID) && owner > 0 && nativeSelfSafeInteger(owner) &&
		item.Status >= 1 && item.Status <= 4 && nativeSelfSafeInteger(item.CreatedTime) &&
		(item.ExpiredTime == -1 || nativeSelfSafeInteger(item.ExpiredTime)) && nativeTokenListSigned(item.RemainQuota) &&
		nativeSelfSafeInteger(item.UsedQuota) && nativeTokenListMaskedKey(item.Key)
	if !valid {
		return item, http.StatusBadGateway
	}
	if owner != claimed {
		return item, http.StatusUnauthorized
	}
	return item, 0
}

func parseNativeTokenListPage(raw json.RawMessage, claimed, page, size int64) (nativeTokenListPage, int) {
	var fields map[string]json.RawMessage
	var rawItems []json.RawMessage
	var result nativeTokenListPage
	if json.Unmarshal(raw, &fields) != nil || fields == nil || !nativeSelfField(fields, "items", &rawItems) || rawItems == nil ||
		!nativeSelfField(fields, "total", &result.Total) || !nativeSelfField(fields, "page", &result.Page) ||
		!nativeSelfField(fields, "page_size", &result.PageSize) || result.Total < 0 || !nativeSelfSafeInteger(result.Total) ||
		result.Page != page || result.PageSize != size || int64(len(rawItems)) > size {
		return result, http.StatusBadGateway
	}
	result.Items = make([]nativeTokenListItem, len(rawItems))
	seen := make(map[int64]struct{}, len(rawItems))
	for index, rawItem := range rawItems {
		item, status := parseNativeTokenListItem(rawItem, claimed)
		if status != 0 {
			return nativeTokenListPage{}, status
		}
		if _, exists := seen[item.ID]; exists {
			return nativeTokenListPage{}, http.StatusBadGateway
		}
		seen[item.ID] = struct{}{}
		result.Items[index] = item
	}
	return result, 0
}

func newNativeTokenListProjectionHandler(transport http.RoundTripper) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != nativeTokenListPath {
			walletError(w, http.StatusNotFound, "NOT_FOUND")
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			walletError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
			return
		}
		query, page, size, ok := nativeTokenListQuery(r)
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
		data, status := nativeWorkspaceData(r, transport, nativeTokenListPath, query)
		if status != 0 {
			nativeSelfProjectionError(w, status)
			return
		}
		result, status := parseNativeTokenListPage(data, userID, page, size)
		if status != 0 {
			nativeSelfProjectionError(w, status)
			return
		}
		walletSuccess(w, result)
	})
}
