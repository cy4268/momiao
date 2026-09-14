package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func projectionRequest() *http.Request {
	r := httptest.NewRequest(http.MethodGet, "https://portal.invalid/api/user/self", nil)
	token := announcementSessionToken(map[string]any{"sub": "42", "sid": "projection-session"})
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("New-Api-User", "42")
	r.Header.Set("X-Auth-Session", "projection-session")
	r.Header.Set("Accept", "application/json")
	return r
}

const projectedSelf = `{"success":true,"message":"","data":{"id":42,"username":"native-user","display_name":"Native User","role":10,"status":1,"group":"default","quota":80,"used_quota":20,"request_count":3,"email":"private@example.invalid","password":"private","access_token":"private","permissions":{"admin":true},"unknown":"discard"}}`

func TestNativeSelfProjectionIsFixedAndSafe(t *testing.T) {
	r := projectionRequest()
	r.Header.Set("Cookie", "browser=private")
	r.Header.Set("Origin", "https://portal.invalid")
	r.Header.Set("Forwarded", "for=private")
	r.Header.Set("X-Forwarded-For", "private")
	token := r.Header.Get("Authorization")
	calls := 0
	transport := roundTripFunc(func(got *http.Request) (*http.Response, error) {
		calls++
		if got.Method != http.MethodGet || got.URL.String() != "http://unix/api/user/self" || got.Host != "localhost" || got.Body != nil {
			t.Fatalf("mutable Native target: %s %s host=%q", got.Method, got.URL, got.Host)
		}
		want := http.Header{"Accept": {"application/json"}, "Authorization": {token}, "New-Api-User": {"42"}, "X-Auth-Session": {"projection-session"}}
		if !reflect.DeepEqual(got.Header, want) {
			t.Fatalf("Native headers=%v", got.Header)
		}
		deadline, ok := got.Context().Deadline()
		if remaining := time.Until(deadline); !ok || remaining < 7*time.Second || remaining > 8*time.Second {
			t.Fatalf("Native deadline=%v remaining=%v", ok, remaining)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Set-Cookie": {"native=private"}, "X-Upstream": {"private"}}, Body: io.NopCloser(strings.NewReader(projectedSelf))}, nil
	})
	w := httptest.NewRecorder()
	// Synthetic JWT shape plus a transport stub is not Native/F4 authentication acceptance.
	newNativeSelfProjectionHandler(transport).ServeHTTP(w, r)
	var envelope struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	if json.Unmarshal(w.Body.Bytes(), &envelope) != nil || w.Code != 200 || !envelope.Success || calls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
	}
	want := map[string]any{"id": float64(42), "username": "native-user", "display_name": "Native User", "role": float64(10), "group": "default", "quota": float64(80), "used_quota": float64(20), "request_count": float64(3)}
	if !reflect.DeepEqual(envelope.Data, want) {
		t.Fatalf("unsafe or incomplete projection: %#v", envelope.Data)
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Set-Cookie") != "" || w.Header().Get("X-Upstream") != "" {
		t.Fatalf("response headers=%v", w.Header())
	}
}

func TestNativeSelfProjectionRejectsUnsafeNativeResults(t *testing.T) {
	valid := projectedSelf
	cases := []struct {
		name, body     string
		upstream, want int
	}{
		{"oversized", strings.Repeat(" ", 65537), 200, 502},
		{"invalid UTF-8", string([]byte{0xff}), 200, 502},
		{"duplicate", strings.Replace(valid, `"id":42`, `"id":42,"id":42`, 1), 200, 502},
		{"identity mismatch", strings.Replace(valid, `"id":42`, `"id":43`, 1), 200, 401},
		{"disabled", strings.Replace(valid, `"status":1`, `"status":2`, 1), 200, 401},
		{"missing required", strings.Replace(valid, `,"request_count":3`, ``, 1), 200, 502},
		{"wrong field type", strings.Replace(valid, `"quota":80`, `"quota":"80"`, 1), 200, 502},
		{"negative integer", strings.Replace(valid, `"quota":80`, `"quota":-1`, 1), 200, 502},
		{"unsafe integer", strings.Replace(valid, `"quota":80`, `"quota":9007199254740992`, 1), 200, 502},
		{"overlong text", strings.Replace(valid, `"username":"native-user"`, `"username":"`+strings.Repeat("a", 21)+`"`, 1), 200, 502},
		{"redirect", `{}`, 302, 502},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: tc.upstream, Header: http.Header{"Set-Cookie": {"native=private"}}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			w := httptest.NewRecorder()
			newNativeSelfProjectionHandler(transport).ServeHTTP(w, projectionRequest())
			if w.Code != tc.want || calls != 1 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Set-Cookie") != "" || strings.Contains(w.Body.String(), "private") {
				t.Fatalf("status=%d calls=%d headers=%v body=%s", w.Code, calls, w.Header(), w.Body.String())
			}
		})
	}
}

func TestNativeSelfProjectionRejectsUnsafeClaimBeforeNative(t *testing.T) {
	calls := 0
	h := newNativeSelfProjectionHandler(roundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, nil }))
	r := projectionRequest()
	r.Header.Set("New-Api-User", "9007199254740992")
	r.Header.Set("Authorization", "Bearer "+announcementSessionToken(map[string]any{"sub": "9007199254740992", "sid": "projection-session"}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 || calls != 0 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unsafe claim status=%d calls=%d", w.Code, calls)
	}
	r = projectionRequest()
	r.Header.Set("Authorization", "Bearer ordinary-api-key")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 || calls != 0 {
		t.Fatalf("API key credential status=%d calls=%d", w.Code, calls)
	}
	for _, tc := range []struct {
		method, path string
		want         int
	}{{http.MethodPost, "/api/user/self", 405}, {http.MethodGet, "/api/user/self?copy=1", 400}, {http.MethodGet, "/api/user/self/", 404}} {
		w = httptest.NewRecorder()
		r = projectionRequest()
		r.Method, r.URL = tc.method, httptest.NewRequest(tc.method, tc.path, nil).URL
		h.ServeHTTP(w, r)
		if w.Code != tc.want || calls != 0 {
			t.Fatalf("%s %s status=%d calls=%d", tc.method, tc.path, w.Code, calls)
		}
	}
}
