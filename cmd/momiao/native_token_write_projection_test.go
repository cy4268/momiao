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

const tokenWriteCreate = `{"name":"  portal key  ","remain_quota":100,"unlimited_quota":false,"expired_time":-1,"model_limits_enabled":false,"model_limits":"","allow_ips":"","group":"","cross_group_retry":false}`
const tokenWriteStatusItem = `{"id":9,"user_id":42,"name":"portal key","key":"ab****yz","status":2,"created_time":1700000000,"expired_time":-1,"remain_quota":100,"used_quota":0,"unlimited_quota":false}`
const tokenWriteCurrentItem = `{"id":9,"user_id":42,"name":"before","key":"ab****yz","status":1,"created_time":1700000000,"accessed_time":1700000010,"expired_time":-1,"remain_quota":100,"used_quota":7,"unlimited_quota":false,"model_limits_enabled":true,"model_limits":"gpt-4o,gpt-5","allow_ips":"127.0.0.1\n10.0.0.1","group":"auto","cross_group_retry":true,"auto_groups":["alpha","beta"],"DeletedAt":null}`

func tokenWriteRequest(method, target, body string) *http.Request {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	r := httptest.NewRequest(method, "https://portal.invalid"+target, reader)
	r.Header = projectionRequest().Header.Clone()
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	return r
}

func tokenWriteResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Set-Cookie": {"native=private"}, "X-Upstream": {"private"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestNativeTokenBasicEditUsesNarrowOwnedUpdate(t *testing.T) {
	const input = `{"id":9,"name":"  edited key  ","remain_quota":200,"unlimited_quota":false,"expired_time":1800000000}`
	const normalized = `{"id":9,"name":"edited key","remain_quota":200,"unlimited_quota":false,"expired_time":1800000000}`
	const updated = `{"id":9,"user_id":42,"name":"edited key","key":"ab****yz","status":1,"created_time":1700000000,"accessed_time":1700000010,"expired_time":1800000000,"remain_quota":200,"used_quota":7,"unlimited_quota":false,"model_limits_enabled":true,"model_limits":"gpt-4o,gpt-5","allow_ips":"127.0.0.1\n10.0.0.1","group":"auto","cross_group_retry":true,"auto_groups":["alpha","beta"],"DeletedAt":null}`
	calls := 0
	transport := roundTripFunc(func(got *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(got.Body)
		if got.Method != http.MethodPut || got.URL.String() != "http://unix/api/token/?basic_only=true" || string(body) != normalized {
			t.Fatalf("narrow update=%s %s body=%s", got.Method, got.URL, body)
		}
		return tokenWriteResponse(200, `{"success":true,"data":`+updated+`}`), nil
	})
	w := httptest.NewRecorder()
	newNativeTokenWriteProjectionHandler(transport).ServeHTTP(w, tokenWriteRequest(http.MethodPut, "/api/token/", input))
	if w.Code != 200 || calls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
	}
	workspaceJSON(t, w.Body.Bytes(), `{"success":true,"data":{"id":9,"name":"edited key","key":"ab****yz","status":1,"created_time":1700000000,"expired_time":1800000000,"remain_quota":200,"used_quota":7,"unlimited_quota":false}}`)
}

func TestNativeTokenBasicEditRejectsOwnerMismatchReceipt(t *testing.T) {
	calls := 0
	transport := roundTripFunc(func(got *http.Request) (*http.Response, error) {
		calls++
		return tokenWriteResponse(200, `{"success":true,"data":`+strings.Replace(tokenWriteCurrentItem, `"user_id":42`, `"user_id":43`, 1)+`}`), nil
	})
	w := httptest.NewRecorder()
	newNativeTokenWriteProjectionHandler(transport).ServeHTTP(w, tokenWriteRequest(http.MethodPut, "/api/token/", `{"id":9,"name":"edited","remain_quota":200,"unlimited_quota":false,"expired_time":-1}`))
	if w.Code != 401 || calls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
	}
}

func TestNativeTokenWriteProjectionSuccessesAreFixedAndProjected(t *testing.T) {
	cases := []struct {
		name, method, target, input, nativeTarget, nativeBody, output string
	}{
		{"create", http.MethodPost, "/api/token/", tokenWriteCreate, "http://unix/api/token/", `{"success":true,"data":{"id":"9","user_id":"42"},"message":"","private":"discard"}`, `{"success":true,"data":{"id":"9"}}`},
		{"create unlimited", http.MethodPost, "/api/token/", `{"name":"unlimited","remain_quota":0,"unlimited_quota":true,"expired_time":0,"model_limits_enabled":false,"model_limits":"","allow_ips":"","group":"","cross_group_retry":false}`, "http://unix/api/token/", `{"success":true,"data":{"id":"9","user_id":"42"}}`, `{"success":true,"data":{"id":"9"}}`},
		{"status", http.MethodPut, "/api/token/?status_only=true", `{"id":9,"status":2}`, "http://unix/api/token/?status_only=true", `{"success":true,"data":` + tokenWriteStatusItem + `,"private":"discard"}`, `{"success":true}`},
		{"reveal", http.MethodPost, "/api/token/9/key", "", "http://unix/api/token/9/key", `{"success":true,"data":{"key":"raw-product-key","authorization":"private","cookie":"private"},"private":"discard"}`, `{"success":true,"data":{"key":"raw-product-key"}}`},
		{"delete", http.MethodDelete, "/api/token/9", "", "http://unix/api/token/9", `{"success":true,"message":"","private":"discard"}`, `{"success":true}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := tokenWriteRequest(tc.method, tc.target, tc.input)
			r.Header.Set("Accept", "text/plain")
			r.Header.Set("Cookie", "browser=private")
			r.Header.Set("Origin", "https://portal.invalid")
			r.Header.Set("Forwarded", "for=private")
			token := r.Header.Get("Authorization")
			calls := 0
			transport := roundTripFunc(func(got *http.Request) (*http.Response, error) {
				calls++
				if got.Method != tc.method || got.URL.String() != tc.nativeTarget || got.Host != "localhost" {
					t.Fatalf("mutable Native target: %s %s host=%q", got.Method, got.URL, got.Host)
				}
				var body []byte
				if got.Body != nil {
					body, _ = io.ReadAll(got.Body)
				}
				wantBody := tc.input
				if tc.name == "create" {
					wantBody = strings.Replace(tokenWriteCreate, "  portal key  ", "portal key", 1)
				}
				if string(body) != wantBody || (wantBody == "" && got.Body != nil) {
					t.Fatalf("Native body=%q want=%q", body, wantBody)
				}
				wantHeader := http.Header{"Accept": {"application/json"}, "Authorization": {token}, "New-Api-User": {"42"}, "X-Auth-Session": {"projection-session"}}
				if wantBody != "" {
					wantHeader.Set("Content-Type", "application/json")
				}
				if !reflect.DeepEqual(got.Header, wantHeader) {
					t.Fatalf("Native headers=%v", got.Header)
				}
				deadline, ok := got.Context().Deadline()
				if remaining := time.Until(deadline); !ok || remaining < 7*time.Second || remaining > 8*time.Second {
					t.Fatalf("Native deadline=%v remaining=%v", ok, remaining)
				}
				return tokenWriteResponse(200, tc.nativeBody), nil
			})
			w := httptest.NewRecorder()
			// Synthetic JWT shape plus a transport stub is not Native/F4 authentication acceptance.
			newNativeTokenWriteProjectionHandler(transport).ServeHTTP(w, r)
			if w.Code != 200 || calls != 1 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Set-Cookie") != "" || w.Header().Get("X-Upstream") != "" {
				t.Fatalf("status=%d calls=%d headers=%v body=%s", w.Code, calls, w.Header(), w.Body.String())
			}
			workspaceJSON(t, w.Body.Bytes(), tc.output)
			for _, secret := range []string{"authorization", "cookie", "private", "discard"} {
				if strings.Contains(w.Body.String(), secret) {
					t.Fatalf("unsafe field %q in %s", secret, w.Body.String())
				}
			}
		})
	}
}

func TestNativeTokenWriteProjectionRejectsInvalidInputBeforeNative(t *testing.T) {
	validStatus := `{"id":9,"status":2}`
	validEdit := `{"id":9,"name":"edited","remain_quota":200,"unlimited_quota":false,"expired_time":-1}`
	cases := []struct {
		name, method, target, body string
		edit                       func(*http.Request)
		want                       int
	}{
		{"wrong method", http.MethodGet, "/api/token/", "", nil, 405},
		{"wrong path", http.MethodPost, "/api/token", tokenWriteCreate, nil, 404},
		{"create query", http.MethodPost, "/api/token/?status_only=true", tokenWriteCreate, nil, 400},
		{"forced empty query", http.MethodPost, "/api/token/?", tokenWriteCreate, nil, 400},
		{"empty create", http.MethodPost, "/api/token/", "", nil, 400},
		{"oversized create", http.MethodPost, "/api/token/", strings.Repeat(" ", (16<<10)+1), nil, 400},
		{"invalid UTF8", http.MethodPost, "/api/token/", string([]byte{0xff}), nil, 400},
		{"duplicate create", http.MethodPost, "/api/token/", strings.Replace(tokenWriteCreate, `"name":`, `"name":"duplicate","name":`, 1), nil, 400},
		{"trailing create", http.MethodPost, "/api/token/", tokenWriteCreate + `{}`, nil, 400},
		{"extra create", http.MethodPost, "/api/token/", strings.Replace(tokenWriteCreate, `}`, `,"user_id":42}`, 1), nil, 400},
		{"missing create", http.MethodPost, "/api/token/", strings.Replace(tokenWriteCreate, `,"group":""`, ``, 1), nil, 400},
		{"blank name", http.MethodPost, "/api/token/", strings.Replace(tokenWriteCreate, "  portal key  ", "   ", 1), nil, 400},
		{"long byte name", http.MethodPost, "/api/token/", strings.Replace(tokenWriteCreate, "  portal key  ", strings.Repeat("a", 51), 1), nil, 400},
		{"fractional quota", http.MethodPost, "/api/token/", strings.Replace(tokenWriteCreate, `"remain_quota":100`, `"remain_quota":1.5`, 1), nil, 400},
		{"zero finite quota", http.MethodPost, "/api/token/", strings.Replace(tokenWriteCreate, `"remain_quota":100`, `"remain_quota":0`, 1), nil, 400},
		{"quota over maximum", http.MethodPost, "/api/token/", strings.Replace(tokenWriteCreate, `"remain_quota":100`, `"remain_quota":1000000000001`, 1), nil, 400},
		{"unlimited nonzero", http.MethodPost, "/api/token/", strings.Replace(tokenWriteCreate, `"unlimited_quota":false`, `"unlimited_quota":true`, 1), nil, 400},
		{"bad expiry", http.MethodPost, "/api/token/", strings.Replace(tokenWriteCreate, `"expired_time":-1`, `"expired_time":-2`, 1), nil, 400},
		{"unsafe expiry", http.MethodPost, "/api/token/", strings.Replace(tokenWriteCreate, `"expired_time":-1`, `"expired_time":9007199254740992`, 1), nil, 400},
		{"expanded model access", http.MethodPost, "/api/token/", strings.Replace(tokenWriteCreate, `"model_limits_enabled":false`, `"model_limits_enabled":true`, 1), nil, 400},
		{"edit unknown field", http.MethodPut, "/api/token/", `{"id":9,"name":"edited","remain_quota":200,"unlimited_quota":false,"expired_time":-1,"group":"default"}`, nil, 400},
		{"edit forced empty query", http.MethodPut, "/api/token/?", validEdit, nil, 400},
		{"edit cannot select Native basic query", http.MethodPut, "/api/token/?basic_only=true", validEdit, nil, 400},
		{"edit cannot alias status query", http.MethodPut, "/api/token/?status_only=false", validEdit, nil, 400},
		{"edit duplicate field", http.MethodPut, "/api/token/", strings.Replace(validEdit, `"name":`, `"name":"duplicate","name":`, 1), nil, 400},
		{"edit missing field", http.MethodPut, "/api/token/", strings.Replace(validEdit, `,"expired_time":-1`, ``, 1), nil, 400},
		{"edit zero id", http.MethodPut, "/api/token/", strings.Replace(validEdit, `"id":9`, `"id":0`, 1), nil, 400},
		{"edit unsafe id", http.MethodPut, "/api/token/", strings.Replace(validEdit, `"id":9`, `"id":9007199254740992`, 1), nil, 400},
		{"edit blank name", http.MethodPut, "/api/token/", strings.Replace(validEdit, `"edited"`, `"   "`, 1), nil, 400},
		{"edit long byte name", http.MethodPut, "/api/token/", strings.Replace(validEdit, `"edited"`, `"`+strings.Repeat("a", 51)+`"`, 1), nil, 400},
		{"edit fractional quota", http.MethodPut, "/api/token/", strings.Replace(validEdit, `"remain_quota":200`, `"remain_quota":1.5`, 1), nil, 400},
		{"edit zero finite quota", http.MethodPut, "/api/token/", strings.Replace(validEdit, `"remain_quota":200`, `"remain_quota":0`, 1), nil, 400},
		{"edit quota over maximum", http.MethodPut, "/api/token/", strings.Replace(validEdit, `"remain_quota":200`, `"remain_quota":2147483648`, 1), nil, 400},
		{"edit unlimited nonzero", http.MethodPut, "/api/token/", strings.Replace(validEdit, `"unlimited_quota":false`, `"unlimited_quota":true`, 1), nil, 400},
		{"edit bad expiry", http.MethodPut, "/api/token/", strings.Replace(validEdit, `"expired_time":-1`, `"expired_time":-2`, 1), nil, 400},
		{"edit unsafe expiry", http.MethodPut, "/api/token/", strings.Replace(validEdit, `"expired_time":-1`, `"expired_time":9007199254740992`, 1), nil, 400},
		{"status duplicate query", http.MethodPut, "/api/token/?status_only=true&status_only=true", validStatus, nil, 400},
		{"status encoded query alias", http.MethodPut, "/api/token/?status_only=%74rue", validStatus, nil, 400},
		{"status extra field", http.MethodPut, "/api/token/?status_only=true", `{"id":9,"status":2,"name":"x"}`, nil, 400},
		{"status bad id", http.MethodPut, "/api/token/?status_only=true", `{"id":0,"status":2}`, nil, 400},
		{"status bad enum", http.MethodPut, "/api/token/?status_only=true", `{"id":9,"status":3}`, nil, 400},
		{"reveal leading zero", http.MethodPost, "/api/token/09/key", "", nil, 400},
		{"reveal encoded alias", http.MethodPost, "/api/token/%39/key", "", nil, 404},
		{"reveal query", http.MethodPost, "/api/token/9/key?x=1", "", nil, 400},
		{"reveal body", http.MethodPost, "/api/token/9/key", " ", nil, 400},
		{"delete zero", http.MethodDelete, "/api/token/0", "", nil, 400},
		{"delete suffix", http.MethodDelete, "/api/token/9/", "", nil, 404},
		{"delete body", http.MethodDelete, "/api/token/9", `{}`, nil, 400},
		{"ordinary key", http.MethodPost, "/api/token/", tokenWriteCreate, func(r *http.Request) { r.Header.Set("Authorization", "Bearer ordinary-key") }, 401},
		{"unsafe claim", http.MethodPost, "/api/token/", tokenWriteCreate, func(r *http.Request) { r.Header.Set("New-Api-User", "9007199254740992") }, 401},
		{"session mismatch", http.MethodPost, "/api/token/", tokenWriteCreate, func(r *http.Request) { r.Header.Set("X-Auth-Session", "other") }, 401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			h := newNativeTokenWriteProjectionHandler(roundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, nil }))
			r := tokenWriteRequest(tc.method, tc.target, tc.body)
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

func TestNativeTokenWriteProjectionRejectsNativeFailuresWithoutRetry(t *testing.T) {
	validStatus := `{"success":true,"data":` + tokenWriteStatusItem + `}`
	validEditInput := `{"id":9,"name":"edited","remain_quota":200,"unlimited_quota":false,"expired_time":-1}`
	validEditItem := strings.NewReplacer(`"name":"portal key"`, `"name":"edited"`, `"remain_quota":100`, `"remain_quota":200`, `"status":2`, `"status":1`).Replace(tokenWriteStatusItem)
	validEdit := `{"success":true,"data":` + validEditItem + `}`
	cases := []struct {
		name, method, target, input, body string
		upstream, want                    int
		err                               error
	}{
		{"transport error", http.MethodPost, "/api/token/", tokenWriteCreate, "", 0, 502, errors.New("native-secret")},
		{"delete ambiguous transport", http.MethodDelete, "/api/token/9", "", "", 0, 502, errors.New("native-secret")},
		{"nil response", http.MethodPost, "/api/token/", tokenWriteCreate, "", -1, 502, nil},
		{"nil body", http.MethodPost, "/api/token/", tokenWriteCreate, "", -2, 502, nil},
		{"unauthorized", http.MethodPost, "/api/token/", tokenWriteCreate, `{"message":"native-secret"}`, 401, 401, nil},
		{"forbidden", http.MethodPost, "/api/token/", tokenWriteCreate, `{"message":"native-secret"}`, 403, 403, nil},
		{"native business failure", http.MethodPost, "/api/token/", tokenWriteCreate, `{"success":false,"message":"native-secret"}`, 200, 502, nil},
		{"redirect", http.MethodPost, "/api/token/", tokenWriteCreate, `{}`, 302, 502, nil},
		{"oversized", http.MethodPost, "/api/token/", tokenWriteCreate, strings.Repeat(" ", (1<<20)+1), 200, 502, nil},
		{"invalid UTF8", http.MethodPost, "/api/token/", tokenWriteCreate, string([]byte{0xff}), 200, 502, nil},
		{"duplicate envelope", http.MethodPost, "/api/token/", tokenWriteCreate, `{"success":true,"success":true}`, 200, 502, nil},
		{"trailing envelope", http.MethodPost, "/api/token/", tokenWriteCreate, `{"success":true}{}`, 200, 502, nil},
		{"wrong success type", http.MethodPost, "/api/token/", tokenWriteCreate, `{"success":"true"}`, 200, 502, nil},
		{"status missing data", http.MethodPut, "/api/token/?status_only=true", `{"id":9,"status":2}`, `{"success":true}`, 200, 502, nil},
		{"status owner mismatch", http.MethodPut, "/api/token/?status_only=true", `{"id":9,"status":2}`, strings.Replace(validStatus, `"user_id":42`, `"user_id":43`, 1), 200, 401, nil},
		{"status id mismatch", http.MethodPut, "/api/token/?status_only=true", `{"id":9,"status":2}`, strings.Replace(validStatus, `"id":9`, `"id":10`, 1), 200, 502, nil},
		{"status mismatch", http.MethodPut, "/api/token/?status_only=true", `{"id":9,"status":2}`, strings.Replace(validStatus, `"status":2`, `"status":1`, 1), 200, 502, nil},
		{"status raw key", http.MethodPut, "/api/token/?status_only=true", `{"id":9,"status":2}`, strings.Replace(validStatus, `"key":"ab****yz"`, `"key":"native-secret"`, 1), 200, 502, nil},
		{"edit missing data", http.MethodPut, "/api/token/", validEditInput, `{"success":true}`, 200, 502, nil},
		{"edit id mismatch", http.MethodPut, "/api/token/", validEditInput, strings.Replace(validEdit, `"id":9`, `"id":10`, 1), 200, 502, nil},
		{"edit mutable mismatch", http.MethodPut, "/api/token/", validEditInput, strings.Replace(validEdit, `"remain_quota":200`, `"remain_quota":201`, 1), 200, 502, nil},
		{"edit raw key", http.MethodPut, "/api/token/", validEditInput, strings.Replace(validEdit, `"key":"ab****yz"`, `"key":"native-secret"`, 1), 200, 502, nil},
		{"reveal missing data", http.MethodPost, "/api/token/9/key", "", `{"success":true}`, 200, 502, nil},
		{"reveal empty key", http.MethodPost, "/api/token/9/key", "", `{"success":true,"data":{"key":""}}`, 200, 502, nil},
		{"reveal long key", http.MethodPost, "/api/token/9/key", "", `{"success":true,"data":{"key":"` + strings.Repeat("k", 129) + `"}}`, 200, 502, nil},
		{"reveal wrong type", http.MethodPost, "/api/token/9/key", "", `{"success":true,"data":{"key":42}}`, 200, 502, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			h := newNativeTokenWriteProjectionHandler(roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if tc.err != nil {
					return nil, tc.err
				}
				if tc.upstream == -1 {
					return nil, nil
				}
				if tc.upstream == -2 {
					return &http.Response{StatusCode: 200}, nil
				}
				return tokenWriteResponse(tc.upstream, tc.body), nil
			}))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, tokenWriteRequest(tc.method, tc.target, tc.input))
			if w.Code != tc.want || calls != 1 || w.Header().Get("Set-Cookie") != "" || strings.Contains(w.Body.String(), "native-secret") {
				t.Fatalf("status=%d calls=%d headers=%v body=%s", w.Code, calls, w.Header(), w.Body.String())
			}
		})
	}
	w := httptest.NewRecorder()
	newNativeTokenWriteProjectionHandler(nil).ServeHTTP(w, tokenWriteRequest(http.MethodDelete, "/api/token/9", ""))
	if w.Code != 503 {
		t.Fatalf("nil transport status=%d body=%s", w.Code, w.Body.String())
	}
}
