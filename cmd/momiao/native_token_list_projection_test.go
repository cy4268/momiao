package main

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

const nativeTokenPage = `{"success":true,"message":"","data":{"page":2,"page_size":2,"total":3,"items":[{"id":9,"user_id":42,"name":"primary","key":"abcd**********wxyz","status":1,"created_time":1700000000,"accessed_time":1700000001,"expired_time":-1,"remain_quota":-7,"used_quota":8,"unlimited_quota":false,"model_limits":"secret","private_raw_key":"raw-secret"},{"id":8,"user_id":42,"name":"backup","key":"ab****yz","status":4,"created_time":1700000002,"expired_time":1800000000,"remain_quota":99,"used_quota":0,"unlimited_quota":true}]},"private":"discard"}`

func tokenListRequest(target string) *http.Request {
	r := projectionRequest()
	r.URL = httptest.NewRequest(http.MethodGet, target, nil).URL
	return r
}

func tokenListResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Set-Cookie": {"native=private"}, "X-Upstream": {"private"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestNativeTokenListProjectionIsFixedCompleteAndSafe(t *testing.T) {
	r := tokenListRequest("/api/token/?page_size=2&p=2")
	r.Header.Set("Accept", "text/plain")
	r.Header.Set("Cookie", "browser=private")
	r.Header.Set("Origin", "https://portal.invalid")
	token := r.Header.Get("Authorization")
	calls := 0
	transport := roundTripFunc(func(got *http.Request) (*http.Response, error) {
		calls++
		if got.Method != http.MethodGet || got.URL.String() != "http://unix/api/token/?p=2&page_size=2" || got.Host != "localhost" || got.Body != nil {
			t.Fatalf("mutable Native target: %s %s host=%q body=%v", got.Method, got.URL, got.Host, got.Body)
		}
		want := http.Header{"Accept": {"application/json"}, "Authorization": {token}, "New-Api-User": {"42"}, "X-Auth-Session": {"projection-session"}}
		if !reflect.DeepEqual(got.Header, want) {
			t.Fatalf("Native headers=%v", got.Header)
		}
		deadline, ok := got.Context().Deadline()
		if remaining := time.Until(deadline); !ok || remaining < 7*time.Second || remaining > 8*time.Second {
			t.Fatalf("Native deadline=%v remaining=%v", ok, remaining)
		}
		return tokenListResponse(200, nativeTokenPage), nil
	})
	w := httptest.NewRecorder()
	// Synthetic JWT shape plus a transport stub is not Native/F4 authentication acceptance.
	newNativeTokenListProjectionHandler(transport).ServeHTTP(w, r)
	if w.Code != 200 || calls != 1 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Set-Cookie") != "" || w.Header().Get("X-Upstream") != "" {
		t.Fatalf("status=%d calls=%d headers=%v body=%s", w.Code, calls, w.Header(), w.Body.String())
	}
	workspaceJSON(t, w.Body.Bytes(), `{"success":true,"data":{"items":[{"id":9,"name":"primary","key":"abcd**********wxyz","status":1,"created_time":1700000000,"expired_time":-1,"remain_quota":-7,"used_quota":8,"unlimited_quota":false},{"id":8,"name":"backup","key":"ab****yz","status":4,"created_time":1700000002,"expired_time":1800000000,"remain_quota":99,"used_quota":0,"unlimited_quota":true}],"total":3,"page":2,"page_size":2}}`)
	for _, secret := range []string{"user_id", "accessed_time", "model_limits", "private_raw_key", "raw-secret", "private"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatalf("unsafe field %q in %s", secret, w.Body.String())
		}
	}
}

func TestNativeTokenListProjectionPreservesEmptyPage(t *testing.T) {
	h := newNativeTokenListProjectionHandler(roundTripFunc(func(got *http.Request) (*http.Response, error) {
		if got.URL.String() != "http://unix/api/token/?p=1&page_size=10" {
			t.Fatalf("Native target=%s", got.URL)
		}
		return tokenListResponse(200, `{"success":true,"data":{"items":[],"total":0,"page":1,"page_size":10}}`), nil
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, tokenListRequest("/api/token/"))
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	workspaceJSON(t, w.Body.Bytes(), `{"success":true,"data":{"items":[],"total":0,"page":1,"page_size":10}}`)
}

func TestNativeTokenListProjectionRejectsUnsafeRequestsBeforeNative(t *testing.T) {
	calls := 0
	h := newNativeTokenListProjectionHandler(roundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, nil }))
	cases := []struct {
		name, target, method string
		edit                 func(*http.Request)
		want                 int
	}{
		{"wrong method", "/api/token/", http.MethodPost, nil, 405},
		{"wrong path", "/api/token", http.MethodGet, nil, 404},
		{"forced empty query", "/api/token/?", http.MethodGet, nil, 400},
		{"unknown query", "/api/token/?ps=1", http.MethodGet, nil, 400},
		{"duplicate", "/api/token/?p=1&p=2", http.MethodGet, nil, 400},
		{"empty", "/api/token/?p=", http.MethodGet, nil, 400},
		{"malformed", "/api/token/?p=%zz", http.MethodGet, nil, 400},
		{"non decimal", "/api/token/?p=1.0", http.MethodGet, nil, 400},
		{"zero", "/api/token/?p=0", http.MethodGet, nil, 400},
		{"negative", "/api/token/?p=-1", http.MethodGet, nil, 400},
		{"large size", "/api/token/?page_size=101", http.MethodGet, nil, 400},
		{"large offset", "/api/token/?p=2147483648&page_size=2", http.MethodGet, nil, 400},
		{"claim mismatch", "/api/token/", http.MethodGet, func(r *http.Request) { r.Header.Set("New-Api-User", "43") }, 401},
		{"ordinary key", "/api/token/", http.MethodGet, func(r *http.Request) { r.Header.Set("Authorization", "Bearer ordinary-key") }, 401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := tokenListRequest(tc.target)
			r.Method = tc.method
			if tc.edit != nil {
				tc.edit(r)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want || calls != 0 || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
			}
		})
	}
}

func TestNativeTokenListProjectionRejectsUnsafeNativeResultsWithoutRetry(t *testing.T) {
	valid := `{"success":true,"data":{"page":1,"page_size":10,"total":1,"items":[{"id":1,"user_id":42,"name":"one","key":"****","status":2,"created_time":0,"expired_time":-1,"remain_quota":-1,"used_quota":0,"unlimited_quota":false}]}}`
	cases := []struct {
		name, body   string
		status, want int
		err          error
	}{
		{"transport error", "", 0, 502, errors.New("private transport")},
		{"nil response", "", -1, 502, nil},
		{"nil body", "", -2, 502, nil},
		{"unauthorized", `{"private":"secret"}`, 401, 401, nil},
		{"forbidden", `{"private":"secret"}`, 403, 403, nil},
		{"native failure", `{"message":"private"}`, 500, 502, nil},
		{"redirect", `{}`, 302, 502, nil},
		{"oversized", strings.Repeat(" ", (1<<20)+1), 200, 502, nil},
		{"invalid UTF-8", string([]byte{0xff}), 200, 502, nil},
		{"duplicate JSON", strings.Replace(valid, `"total":1`, `"total":1,"total":1`, 1), 200, 502, nil},
		{"trailing JSON", valid + `{}`, 200, 502, nil},
		{"false envelope", strings.Replace(valid, `"success":true`, `"success":false`, 1), 200, 502, nil},
		{"page mismatch", strings.Replace(valid, `"page":1`, `"page":2`, 1), 200, 502, nil},
		{"size mismatch", strings.Replace(valid, `"page_size":10`, `"page_size":9`, 1), 200, 502, nil},
		{"unsafe total", strings.Replace(valid, `"total":1`, `"total":9007199254740992`, 1), 200, 502, nil},
		{"null items", strings.Replace(valid, `"items":[{`, `"items":null,"discard":[{`, 1), 200, 502, nil},
		{"items exceed size", strings.Replace(valid, `"page_size":10,"total":1,"items":[{`, `"page_size":1,"total":2,"items":[{"id":2,"user_id":42,"name":"two","key":"","status":1,"created_time":0,"expired_time":-1,"remain_quota":0,"used_quota":0,"unlimited_quota":false},{`, 1), 200, 502, nil},
		{"missing required", strings.Replace(valid, `,"name":"one"`, ``, 1), 200, 502, nil},
		{"wrong field type", strings.Replace(valid, `"unlimited_quota":false`, `"unlimited_quota":"false"`, 1), 200, 502, nil},
		{"owner mismatch", strings.Replace(valid, `"user_id":42`, `"user_id":43`, 1), 200, 401, nil},
		{"raw key", strings.Replace(valid, `"key":"****"`, `"key":"raw-secret"`, 1), 200, 502, nil},
		{"bad status", strings.Replace(valid, `"status":2`, `"status":5`, 1), 200, 502, nil},
		{"bad expiry", strings.Replace(valid, `"expired_time":-1`, `"expired_time":-2`, 1), 200, 502, nil},
		{"unsafe remain", strings.Replace(valid, `"remain_quota":-1`, `"remain_quota":-9007199254740992`, 1), 200, 502, nil},
		{"negative used", strings.Replace(valid, `"used_quota":0`, `"used_quota":-1`, 1), 200, 502, nil},
		{"duplicate ID", strings.Replace(valid, `"items":[{`, `"items":[{"id":1,"user_id":42,"name":"duplicate","key":"","status":1,"created_time":0,"expired_time":-1,"remain_quota":0,"used_quota":0,"unlimited_quota":false},{`, 1), 200, 502, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			h := newNativeTokenListProjectionHandler(roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if tc.err != nil {
					return nil, tc.err
				}
				if tc.status == -1 {
					return nil, nil
				}
				if tc.status == -2 {
					return &http.Response{StatusCode: 200}, nil
				}
				return tokenListResponse(tc.status, tc.body), nil
			}))
			w := httptest.NewRecorder()
			target := "/api/token/"
			if tc.name == "items exceed size" {
				target += "?page_size=1"
			}
			h.ServeHTTP(w, tokenListRequest(target))
			if w.Code != tc.want || calls != 1 || w.Header().Get("Set-Cookie") != "" || strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), "secret") {
				t.Fatalf("status=%d calls=%d headers=%v body=%s", w.Code, calls, w.Header(), w.Body.String())
			}
		})
	}
	w := httptest.NewRecorder()
	newNativeTokenListProjectionHandler(nil).ServeHTTP(w, tokenListRequest("/api/token/"))
	if w.Code != 503 {
		t.Fatalf("nil transport status=%d body=%s", w.Code, w.Body.String())
	}
}
