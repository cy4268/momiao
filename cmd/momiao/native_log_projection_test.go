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

const nativeLogPage = `{"success":true,"message":"","data":{"page":2,"page_size":2,"total":1,"items":[{"id":3,"user_id":42,"created_at":1700000000,"type":2,"token_name":"primary","model_name":"gpt%model","quota":-7,"prompt_tokens":8,"completion_tokens":9,"content":"prompt-secret","other":"response-secret","ip":"private","channel":99,"username":"private","request_id":"private"},{"id":4,"user_id":42,"created_at":1700000001,"type":6,"token_name":"","model_name":"","quota":10,"prompt_tokens":0,"completion_tokens":0}]},"private":"discard"}`

func logProjectionRequest(target string) *http.Request {
	r := projectionRequest()
	r.URL = httptest.NewRequest(http.MethodGet, target, nil).URL
	return r
}

func logProjectionResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Set-Cookie": {"native=private"}, "X-Upstream": {"private"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestNativeLogProjectionIsFixedCompleteAndSafe(t *testing.T) {
	r := logProjectionRequest("/api/log/self?type=2&page_size=2&p=2&model_name=gpt%25&end_timestamp=20&start_timestamp=10")
	r.Header.Set("Accept", "text/plain")
	r.Header.Set("Cookie", "browser=private")
	r.Header.Set("Origin", "https://portal.invalid")
	r.Header.Set("Forwarded", "for=private")
	token := r.Header.Get("Authorization")
	calls := 0
	transport := roundTripFunc(func(got *http.Request) (*http.Response, error) {
		calls++
		if got.Method != http.MethodGet || got.URL.String() != "http://unix/api/log/self?end_timestamp=20&model_name=gpt%25&p=2&page_size=2&start_timestamp=10&type=2" || got.Host != "localhost" || got.Body != nil {
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
		return logProjectionResponse(200, nativeLogPage), nil
	})
	w := httptest.NewRecorder()
	// Synthetic JWT shape plus a transport stub is not Native/F4 authentication acceptance.
	newNativeLogProjectionHandler(transport).ServeHTTP(w, r)
	if w.Code != 200 || calls != 1 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Set-Cookie") != "" || w.Header().Get("X-Upstream") != "" {
		t.Fatalf("status=%d calls=%d headers=%v body=%s", w.Code, calls, w.Header(), w.Body.String())
	}
	workspaceJSON(t, w.Body.Bytes(), `{"success":true,"data":{"items":[{"id":3,"created_at":1700000000,"type":2,"token_name":"primary","model_name":"gpt%model","quota":-7,"prompt_tokens":8,"completion_tokens":9},{"id":4,"created_at":1700000001,"type":6,"token_name":"","model_name":"","quota":10,"prompt_tokens":0,"completion_tokens":0}],"total":1,"page":2,"page_size":2}}`)
	for _, secret := range []string{"user_id", "content", "prompt-secret", "response-secret", "other", "ip", "channel", "username", "request_id", "private", "discard"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatalf("unsafe field %q in %s", secret, w.Body.String())
		}
	}
}

func TestNativeLogProjectionDefaultsAndNormalizesNoFilterValues(t *testing.T) {
	for _, target := range []string{"/api/log/self", "/api/log/self?p=1&page_size=10&type=0&model_name=&start_timestamp=0&end_timestamp=0"} {
		t.Run(target, func(t *testing.T) {
			calls := 0
			h := newNativeLogProjectionHandler(roundTripFunc(func(got *http.Request) (*http.Response, error) {
				calls++
				if got.URL.String() != "http://unix/api/log/self?p=1&page_size=10&type=0" {
					t.Fatalf("Native target=%s", got.URL)
				}
				return logProjectionResponse(200, `{"success":true,"data":{"items":null,"total":0,"page":1,"page_size":10}}`), nil
			}))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, logProjectionRequest(target))
			if w.Code != 200 || calls != 1 {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
			}
			workspaceJSON(t, w.Body.Bytes(), `{"success":true,"data":{"items":[],"total":0,"page":1,"page_size":10}}`)
		})
	}
}

func TestNativeLogProjectionRejectsUnsafeRequestsBeforeNative(t *testing.T) {
	cases := []struct {
		name, target, method string
		edit                 func(*http.Request)
		want                 int
	}{
		{"wrong method", "/api/log/self", http.MethodPost, nil, 405},
		{"wrong path", "/api/log/self/", http.MethodGet, nil, 404},
		{"forced empty query", "/api/log/self?", http.MethodGet, nil, 400},
		{"unknown", "/api/log/self?token_name=x", http.MethodGet, nil, 400},
		{"duplicate", "/api/log/self?p=1&p=2", http.MethodGet, nil, 400},
		{"empty numeric", "/api/log/self?type=", http.MethodGet, nil, 400},
		{"malformed", "/api/log/self?p=%zz", http.MethodGet, nil, 400},
		{"zero page", "/api/log/self?p=0", http.MethodGet, nil, 400},
		{"large size", "/api/log/self?page_size=101", http.MethodGet, nil, 400},
		{"large offset", "/api/log/self?p=2147483648&page_size=2", http.MethodGet, nil, 400},
		{"negative type", "/api/log/self?type=-1", http.MethodGet, nil, 400},
		{"large type", "/api/log/self?type=8", http.MethodGet, nil, 400},
		{"invalid model UTF8", "/api/log/self?model_name=%FF", http.MethodGet, nil, 400},
		{"long model", "/api/log/self?model_name=" + strings.Repeat("m", 201), http.MethodGet, nil, 400},
		{"empty timestamp", "/api/log/self?start_timestamp=", http.MethodGet, nil, 400},
		{"negative timestamp", "/api/log/self?start_timestamp=-1", http.MethodGet, nil, 400},
		{"unsafe timestamp", "/api/log/self?end_timestamp=9007199254740992", http.MethodGet, nil, 400},
		{"reverse range", "/api/log/self?start_timestamp=20&end_timestamp=10", http.MethodGet, nil, 400},
		{"claim mismatch", "/api/log/self", http.MethodGet, func(r *http.Request) { r.Header.Set("New-Api-User", "43") }, 401},
		{"ordinary key", "/api/log/self", http.MethodGet, func(r *http.Request) { r.Header.Set("Authorization", "Bearer ordinary-key") }, 401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			h := newNativeLogProjectionHandler(roundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, nil }))
			r := logProjectionRequest(tc.target)
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

func TestNativeLogProjectionRejectsUnsafeNativeResultsWithoutRetry(t *testing.T) {
	valid := `{"success":true,"data":{"page":1,"page_size":10,"total":1,"items":[{"id":1,"user_id":42,"created_at":0,"type":0,"token_name":"","model_name":"","quota":-1,"prompt_tokens":0,"completion_tokens":0}]}}`
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
		{"native failure", `{"success":false,"message":"private secret"}`, 200, 502, nil},
		{"redirect", `{}`, 302, 502, nil},
		{"duplicate JSON", strings.Replace(valid, `"total":1`, `"total":1,"total":1`, 1), 200, 502, nil},
		{"invalid UTF8", string([]byte{0xff}), 200, 502, nil},
		{"oversized", strings.Repeat(" ", (1<<20)+1), 200, 502, nil},
		{"page mismatch", strings.Replace(valid, `"page":1`, `"page":2`, 1), 200, 502, nil},
		{"size mismatch", strings.Replace(valid, `"page_size":10`, `"page_size":9`, 1), 200, 502, nil},
		{"negative total", strings.Replace(valid, `"total":1`, `"total":-1`, 1), 200, 502, nil},
		{"too many items", strings.Replace(strings.Replace(valid, `"page_size":10`, `"page_size":1`, 1), `"items":[{`, `"items":[{"id":2,"user_id":42,"created_at":0,"type":0,"token_name":"","model_name":"","quota":0,"prompt_tokens":0,"completion_tokens":0},{`, 1), 200, 502, nil},
		{"missing required", strings.Replace(valid, `,"model_name":""`, ``, 1), 200, 502, nil},
		{"missing owner", strings.Replace(valid, `,"user_id":42`, ``, 1), 200, 502, nil},
		{"wrong type", strings.Replace(valid, `"prompt_tokens":0`, `"prompt_tokens":"0"`, 1), 200, 502, nil},
		{"owner mismatch", strings.Replace(valid, `"user_id":42`, `"user_id":43`, 1), 200, 401, nil},
		{"duplicate id", strings.Replace(valid, `"items":[{`, `"items":[{"id":1,"user_id":42,"created_at":0,"type":0,"token_name":"","model_name":"","quota":0,"prompt_tokens":0,"completion_tokens":0},{`, 1), 200, 502, nil},
		{"bad id", strings.Replace(valid, `"id":1`, `"id":0`, 1), 200, 502, nil},
		{"bad created", strings.Replace(valid, `"created_at":0`, `"created_at":-1`, 1), 200, 502, nil},
		{"bad log type", strings.Replace(valid, `"type":0`, `"type":8`, 1), 200, 502, nil},
		{"unsafe signed quota", strings.Replace(valid, `"quota":-1`, `"quota":-9007199254740992`, 1), 200, 502, nil},
		{"negative completion", strings.Replace(valid, `"completion_tokens":0`, `"completion_tokens":-1`, 1), 200, 502, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			h := newNativeLogProjectionHandler(roundTripFunc(func(*http.Request) (*http.Response, error) {
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
				return logProjectionResponse(tc.status, tc.body), nil
			}))
			w := httptest.NewRecorder()
			target := "/api/log/self"
			if tc.name == "too many items" {
				target += "?page_size=1"
			}
			h.ServeHTTP(w, logProjectionRequest(target))
			if w.Code != tc.want || calls != 1 || w.Header().Get("Set-Cookie") != "" || strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), "secret") {
				t.Fatalf("status=%d calls=%d headers=%v body=%s", w.Code, calls, w.Header(), w.Body.String())
			}
		})
	}
	w := httptest.NewRecorder()
	newNativeLogProjectionHandler(nil).ServeHTTP(w, logProjectionRequest("/api/log/self"))
	if w.Code != 503 {
		t.Fatalf("nil transport status=%d body=%s", w.Code, w.Body.String())
	}
}
