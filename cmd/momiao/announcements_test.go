package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cy4268/momiao/internal/platform"
)

func announcementSessionToken(patch map[string]any) string {
	claims := map[string]any{"iss": "new-api", "aud": []string{"new-api-dashboard"}, "token_use": "access", "sub": "9007199254740993", "sid": "fixture-session", "uv": 1, "sv": 1, "exp": 9999999999, "iat": 1788634800, "nbf": 1788634795, "jti": "fixture-id"}
	for key, value := range patch {
		claims[key] = value
	}
	b, _ := json.Marshal(claims)
	return base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString([]byte("synthetic-signature"))
}
func announcementReq(method, path, body string) *http.Request {
	r := walletReq(method, path, body)
	r.Header.Set("Authorization", "Bearer "+announcementSessionToken(nil))
	r.Header.Set("X-Auth-Session", "fixture-session")
	return r
}

type announcementTestStore struct {
	*platform.Store
	user                        int64
	authorityCalls, publicCalls int
	denied                      bool
}

func (s *announcementTestStore) AnnouncementAuthority(_ context.Context, user int64) (platform.AnnouncementPrincipal, error) {
	s.user = user
	s.authorityCalls++
	if s.denied {
		return platform.AnnouncementPrincipal{}, platform.ErrAnnouncementForbidden
	}
	return platform.AnnouncementPrincipal{UserID: user, Role: "AUDITOR", Epoch: 2, Permissions: []string{"announcements.read"}}, nil
}
func (s *announcementTestStore) OpsAnnouncements(ctx context.Context, user int64) (platform.AnnouncementPrincipal, []platform.OpsAnnouncement, error) {
	p, e := s.AnnouncementAuthority(ctx, user)
	return p, []platform.OpsAnnouncement{}, e
}
func (s *announcementTestStore) PublicAnnouncements(_ context.Context, user int64, f platform.AnnouncementFilter) (platform.AnnouncementPage, error) {
	s.user = user
	s.publicCalls++
	return platform.AnnouncementPage{Items: []platform.Announcement{}}, nil
}
func TestAnnouncementHTTPAuthority(t *testing.T) {
	nativeCalls := 0
	transport := walletTransport(func(r *http.Request) (*http.Response, error) {
		nativeCalls++
		if r.URL.String() != "http://unix/api/user/self" {
			t.Fatal(r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"id":9007199254740993,"status":1,"role":100}}`))}, nil
	})
	s := &announcementTestStore{denied: true}
	h := newAnnouncementHandler("https://wallet.example", s, transport)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/platform/v1/ops/announcements", nil))
	if w.Code != 401 || nativeCalls != 0 || s.authorityCalls != 0 {
		t.Fatal("anonymous Ops gate", w.Code)
	}
	w = httptest.NewRecorder()
	r := announcementReq("GET", "/platform/v1/ops/announcements", "")
	r.Header.Set("X-Role", "SUPER_ADMIN")
	h.ServeHTTP(w, r)
	if w.Code != 403 || nativeCalls != 1 || s.user != 9007199254740993 {
		t.Fatal("native root mapped to Ops", w.Code, w.Body)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/platform/v1/announcements", nil))
	if w.Code != 200 || s.user != 0 || s.publicCalls != 1 {
		t.Fatal("public list", w.Code, w.Body)
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/platform/v1/announcements", nil)
	r.Header.Set("New-Api-User", "4")
	h.ServeHTTP(w, r)
	if w.Code != 401 || s.publicCalls != 1 {
		t.Fatal("invalid auth downgraded to guest", w.Code)
	}
}
func TestAnnouncementHTTPStrictWrites(t *testing.T) {
	s := &announcementTestStore{}
	h := newAnnouncementHandler("https://wallet.example", s, profileTransport())
	for _, body := range []string{`null`, `[]`, `{"content":{"title":"x","title":"y"}}`, `{"content":{},"unknown":1}`, `{"content":{}} trailing`, strings.Repeat(" ", 66000)} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, announcementReq("POST", "/platform/v1/ops/announcements/render-preview", body))
		if w.Code != 400 {
			t.Fatal("invalid body", w.Code)
		}
	}
	w := httptest.NewRecorder()
	r := announcementReq("POST", "/platform/v1/ops/announcements/render-preview", `{}`)
	r.Header.Set("Origin", "https://evil.example")
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("origin accepted", w.Code)
	}
	for _, query := range []string{"?type=SYSTEM&type=IMPORTANT", "?limit=51", "?offset=-1", "?archive=maybe", "?date_from=2026-02-30", "?private=1"} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/platform/v1/announcements"+query, nil))
		if w.Code != 400 {
			t.Fatal(query, w.Code)
		}
	}
}

func TestAnnouncementHTTPRequiresNativeSessionCredential(t *testing.T) {
	valid := announcementSessionToken(nil)
	headerToken := func(header string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(header)) + "." + strings.Join(strings.Split(valid, ".")[1:], ".")
	}
	for _, tc := range []struct {
		name, token  string
		nativeStatus int
		want, calls  int
	}{
		{"opaque PAT", "opaque-personal-access-token", 200, 401, 0},
		{"dotted PAT", "opaque.key.with-dots", 200, 401, 0},
		{"uppercase-only algorithm", headerToken(`{"ALG":"HS256"}`), 200, 401, 0},
		{"missing algorithm", headerToken(`{"typ":"JWT"}`), 200, 401, 0},
		{"non-string algorithm", headerToken(`{"alg":123}`), 200, 401, 0},
		{"wrong issuer", announcementSessionToken(map[string]any{"iss": "other"}), 200, 401, 0},
		{"wrong audience", announcementSessionToken(map[string]any{"aud": []string{"other"}}), 200, 401, 0},
		{"wrong purpose", announcementSessionToken(map[string]any{"token_use": "security_proof"}), 200, 401, 0},
		{"subject mismatch", announcementSessionToken(map[string]any{"sub": "1"}), 200, 401, 0},
		{"session mismatch", announcementSessionToken(map[string]any{"sid": "other"}), 200, 401, 0},
		{"malformed registered claim", announcementSessionToken(map[string]any{"exp": map[string]any{"x": 1}}), 200, 401, 0},
		{"forged signature", valid + "forged", 401, 401, 1},
		{"revoked session", valid, 401, 401, 1},
		{"native-validated session", valid, 200, 200, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			s := &announcementTestStore{}
			h := newAnnouncementHandler("https://wallet.example", s, walletTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Header.Get("Authorization") != "Bearer "+tc.token {
					t.Fatal("credential was rewritten")
				}
				return &http.Response{StatusCode: tc.nativeStatus, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"id":9007199254740993,"status":1}}`))}, nil
			}))
			r := announcementReq("GET", "/platform/v1/ops/announcements", "")
			r.Header.Set("Authorization", "Bearer "+tc.token)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want || calls != tc.calls {
				t.Fatalf("status=%d native calls=%d", w.Code, calls)
			}
			if tc.want != 200 && s.authorityCalls != 0 {
				t.Fatal("principal loaded before session verified")
			}
		})
	}
}
