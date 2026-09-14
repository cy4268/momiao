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

func workspaceRequest(path string) *http.Request {
	r := projectionRequest()
	r.URL = httptest.NewRequest(http.MethodGet, path, nil).URL
	return r
}

func workspaceResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Set-Cookie": {"native=private"}, "X-Upstream": {"private"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func workspaceJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	var actual, expected any
	if json.Unmarshal(got, &actual) != nil || json.Unmarshal([]byte(want), &expected) != nil || !reflect.DeepEqual(actual, expected) {
		t.Fatalf("response=%s want=%s", got, want)
	}
}

func TestNativeWorkspaceGroupsProjectionIsFixedAndSafe(t *testing.T) {
	r := workspaceRequest("/api/user/self/groups")
	r.Header.Set("Accept", "text/plain")
	r.Header.Set("Cookie", "browser=private")
	r.Header.Set("Origin", "https://portal.invalid")
	r.Header.Set("Forwarded", "for=private")
	token := r.Header.Get("Authorization")
	calls := 0
	transport := roundTripFunc(func(got *http.Request) (*http.Response, error) {
		calls++
		if got.Method != http.MethodGet || got.URL.String() != "http://unix/api/user/self/groups" || got.Host != "localhost" || got.Body != nil {
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
		return workspaceResponse(200, `{"success":true,"data":{"default":{"ratio":1.25,"desc":"Default","secret":true},"auto":{"ratio":"自动","desc":"Auto"}},"private":"discard"}`), nil
	})
	w := httptest.NewRecorder()
	// Synthetic JWT shape plus a transport stub is not Native/F4 authentication acceptance.
	newNativeWorkspaceProjectionHandler(transport).ServeHTTP(w, r)
	if w.Code != 200 || calls != 1 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Set-Cookie") != "" || w.Header().Get("X-Upstream") != "" {
		t.Fatalf("status=%d calls=%d headers=%v body=%s", w.Code, calls, w.Header(), w.Body.String())
	}
	workspaceJSON(t, w.Body.Bytes(), `{"success":true,"data":{"default":{"ratio":1.25,"desc":"Default"},"auto":{"ratio":"自动","desc":"Auto"}}}`)
}

func TestNativeWorkspaceModelsQueryAndProjection(t *testing.T) {
	calls := 0
	transport := roundTripFunc(func(got *http.Request) (*http.Response, error) {
		calls++
		if got.URL.String() != "http://unix/api/user/models?group=vip%26other%3D1" || got.Host != "localhost" {
			t.Fatalf("Native target=%s host=%q", got.URL, got.Host)
		}
		return workspaceResponse(200, `{"success":true,"data":["model-a","model-b"],"private":"discard"}`), nil
	})
	w := httptest.NewRecorder()
	newNativeWorkspaceProjectionHandler(transport).ServeHTTP(w, workspaceRequest("/api/user/models?group=vip%26other%3D1"))
	if w.Code != 200 || calls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
	}
	workspaceJSON(t, w.Body.Bytes(), `{"success":true,"data":["model-a","model-b"]}`)
}

func TestNativeWorkspacePreservesEmptyCollections(t *testing.T) {
	for _, tc := range []struct{ path, query, data, want string }{
		{"/api/user/models", "", "null", "[]"},
		{"/api/user/models", "?group=", "[]", "[]"},
		{"/api/user/self/groups", "", "{}", "{}"},
	} {
		t.Run(tc.path+tc.data, func(t *testing.T) {
			calls := 0
			h := newNativeWorkspaceProjectionHandler(roundTripFunc(func(got *http.Request) (*http.Response, error) {
				calls++
				if got.URL.String() != "http://unix"+tc.path {
					t.Fatalf("Native target=%s", got.URL)
				}
				return workspaceResponse(200, `{"success":true,"data":`+tc.data+`}`), nil
			}))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, workspaceRequest(tc.path+tc.query))
			if w.Code != 200 || calls != 1 {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
			}
			workspaceJSON(t, w.Body.Bytes(), `{"success":true,"data":`+tc.want+`}`)
		})
	}
}

func TestNativeWorkspaceRejectsUnsafeRequestsBeforeNative(t *testing.T) {
	calls := 0
	h := newNativeWorkspaceProjectionHandler(roundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, nil }))
	cases := []struct{ path, query string }{
		{"/api/user/self/groups", "group=x"},
		{"/api/user/models", "group=a&group=b"},
		{"/api/user/models", "other=x"},
		{"/api/user/models", "group=%zz"},
		{"/api/user/models", "group=%FF"},
		{"/api/user/models", "group=" + strings.Repeat("界", 65)},
	}
	for _, tc := range cases {
		w, r := httptest.NewRecorder(), workspaceRequest(tc.path)
		r.URL.RawQuery = tc.query
		h.ServeHTTP(w, r)
		if w.Code != 400 || calls != 0 {
			t.Fatalf("%s?%s status=%d calls=%d", tc.path, tc.query, w.Code, calls)
		}
	}
	w, r := httptest.NewRecorder(), workspaceRequest("/api/user/models")
	r.Header.Set("Authorization", "Bearer ordinary-api-key")
	h.ServeHTTP(w, r)
	if w.Code != 401 || calls != 0 {
		t.Fatalf("API key status=%d calls=%d", w.Code, calls)
	}
	for _, tc := range []struct {
		method, path string
		want         int
	}{{http.MethodPost, "/api/user/models", 405}, {http.MethodGet, "/api/user/groups", 404}} {
		w, r = httptest.NewRecorder(), workspaceRequest(tc.path)
		r.Method = tc.method
		h.ServeHTTP(w, r)
		if w.Code != tc.want || calls != 0 {
			t.Fatalf("%s %s status=%d calls=%d", tc.method, tc.path, w.Code, calls)
		}
	}
}

func TestNativeWorkspaceRejectsUnsafeNativeResultsWithoutRetry(t *testing.T) {
	cases := []struct {
		name, path, body string
		upstream, want   int
	}{
		{"duplicate JSON", "/api/user/self/groups", `{"success":true,"success":true,"data":{}}`, 200, 502},
		{"over limit", "/api/user/self/groups", strings.Repeat(" ", (1<<20)+1), 200, 502},
		{"invalid UTF-8", "/api/user/self/groups", string([]byte{0xff}), 200, 502},
		{"bad ratio", "/api/user/self/groups", `{"success":true,"data":{"default":{"ratio":"1","desc":"Default"}}}`, 200, 502},
		{"non-auto label", "/api/user/self/groups", `{"success":true,"data":{"default":{"ratio":"自动","desc":"Default"}}}`, 200, 502},
		{"numeric auto", "/api/user/self/groups", `{"success":true,"data":{"auto":{"ratio":1,"desc":"Auto"}}}`, 200, 502},
		{"negative ratio", "/api/user/self/groups", `{"success":true,"data":{"default":{"ratio":-1,"desc":"Default"}}}`, 200, 502},
		{"nonfinite ratio", "/api/user/self/groups", `{"success":true,"data":{"default":{"ratio":1e400,"desc":"Default"}}}`, 200, 502},
		{"bad model", "/api/user/models", `{"success":true,"data":[1]}`, 200, 502},
		{"missing data", "/api/user/models", `{"success":true}`, 200, 502},
		{"unauthorized", "/api/user/models", `{"private":"discard"}`, 401, 401},
		{"forbidden", "/api/user/models", `{"private":"discard"}`, 403, 403},
		{"redirect", "/api/user/models", `{"private":"discard"}`, 302, 502},
		{"native failure", "/api/user/models", `{"private":"discard"}`, 500, 502},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			h := newNativeWorkspaceProjectionHandler(roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return workspaceResponse(tc.upstream, tc.body), nil
			}))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, workspaceRequest(tc.path))
			if w.Code != tc.want || calls != 1 || w.Header().Get("Set-Cookie") != "" || strings.Contains(w.Body.String(), "private") {
				t.Fatalf("status=%d calls=%d headers=%v body=%s", w.Code, calls, w.Header(), w.Body.String())
			}
		})
	}
}
