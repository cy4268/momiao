package bffauth

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type logoutTransport func(*http.Request) (*http.Response, error)

func (f logoutTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNativeLogoutResponseContract(t *testing.T) {
	for _, tc := range []struct {
		name, body    string
		status        int
		clear, wantOK bool
	}{
		{"refresh only success", `{"success":true,"message":""}`, 200, true, true},
		{"access success", `{"success":true,"message":"","data":{"revoked_sid":"fixture","cookie_cleared":true}}`, 200, true, true},
		{"rejection", `{"success":false,"message":""}`, 200, true, false},
		{"server error", `{"success":true,"message":""}`, 500, true, false},
		{"no cleared cookie", `{"success":true,"message":""}`, 200, false, false},
		{"missing message", `{"success":true}`, 200, true, false},
		{"duplicate success", `{"success":false,"success":true,"message":""}`, 200, true, false},
		{"unknown field", `{"success":true,"message":"","extra":true}`, 200, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{origin: "https://fixture.invalid", native: logoutTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/user/auth/logout" {
					t.Fatal("unexpected request")
				}
				h := make(http.Header)
				h.Set("Content-Type", "application/json")
				if tc.clear {
					h.Add("Set-Cookie", "new_api_refresh=; Path=/api/user/auth; Max-Age=0; HttpOnly; Secure; SameSite=Strict")
				}
				return &http.Response{StatusCode: tc.status, Header: h, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			if err := s.nativeLogout(context.Background(), nativeCredential{SID: "fixture"}); (err == nil) != tc.wantOK {
				t.Fatalf("logout error = %v, want success = %v", err, tc.wantOK)
			}
		})
	}
}

func TestNativeLogoutAdmissionCookies(t *testing.T) {
	refresh := "new_api_refresh=; Path=/api/user/auth; Max-Age=0; HttpOnly; Secure; SameSite=Strict"
	oauth := "__Host-momiao_oauth=; Path=/; Expires=Thu, 01 Jan 1970 00:00:01 GMT; Max-Age=0; HttpOnly; Secure; SameSite=Lax"
	for _, tc := range []struct {
		name    string
		cookies []string
		wantOK  bool
	}{
		{"admission first", []string{oauth, refresh}, true},
		{"refresh first", []string{refresh, oauth}, true},
		{"duplicate refresh", []string{refresh, refresh}, false},
		{"duplicate admission", []string{oauth, oauth, refresh}, false},
		{"unknown cookie", []string{refresh, "other=; Max-Age=0"}, false},
		{"malformed cookie", []string{refresh, "=invalid"}, false},
		{"live admission cookie", []string{refresh, strings.Replace(oauth, "oauth=;", "oauth=live;", 1)}, false},
		{"admission wrong path", []string{refresh, strings.Replace(oauth, "Path=/;", "Path=/wrong;", 1)}, false},
		{"admission domain", []string{refresh, oauth + "; Domain=fixture.invalid"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{origin: "https://fixture.invalid", native: logoutTransport(func(*http.Request) (*http.Response, error) {
				h := http.Header{"Content-Type": []string{"application/json"}, "Set-Cookie": tc.cookies}
				return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(`{"success":true,"message":""}`))}, nil
			})}
			if err := s.nativeLogout(context.Background(), nativeCredential{SID: "fixture"}); (err == nil) != tc.wantOK {
				t.Fatalf("logout error = %v, want success = %v", err, tc.wantOK)
			}
		})
	}
	response := &http.Response{Header: http.Header{"Set-Cookie": []string{
		"new_api_refresh=fixture.token; Path=/api/user/auth; HttpOnly; Secure; SameSite=Strict", oauth,
	}}}
	if _, ok := refreshToken(response, "fixture", false); ok {
		t.Fatal("login/refresh must not accept the logout-only companion cookie")
	}
}
