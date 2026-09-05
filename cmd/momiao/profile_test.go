package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cy4268/momiao/internal/platform"
)

type profileTestStore struct {
	user                  int64
	reads, inits, updates int
	patch                 platform.ProfilePatch
	expected              int64
	name, avatar          string
	err                   error
}

func (s *profileTestStore) result(u int64) (platform.Profile, error) {
	s.user = u
	return platform.Profile{UserID: u, ShortAccountID: platform.ShortAccountID(u), Status: "INCOMPLETE", AvatarID: "system-default", SuggestedName: "Master-" + platform.ShortAccountID(u), Avatars: []platform.ProfileAvatar{{ID: "system-default", Label: "系统默认头像", Source: "SYSTEM"}}}, s.err
}
func (s *profileTestStore) ReadProfile(_ context.Context, u int64) (platform.Profile, error) {
	s.reads++
	return s.result(u)
}
func (s *profileTestStore) InitializeProfile(_ context.Context, u, v int64, n, a string) (platform.Profile, error) {
	s.inits++
	s.expected = v
	s.name = n
	s.avatar = a
	return s.result(u)
}
func (s *profileTestStore) UpdateProfile(_ context.Context, u int64, p platform.ProfilePatch) (platform.Profile, error) {
	s.updates++
	s.patch = p
	return s.result(u)
}
func profileTransport() http.RoundTripper {
	return walletTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"id":9007199254740993,"status":1}}`))}, nil
	})
}

const validProfileInit = `{"expected_version":"0","display_name":"Alice","avatar_id":"system-default"}`

func TestProfileHTTPReadAndWriteContract(t *testing.T) {
	s := &profileTestStore{}
	h := newProfileHandler("https://wallet.example", s, profileTransport())
	w := httptest.NewRecorder()
	r := walletReq("GET", "/platform/v1/master-profile", "")
	r.Header.Set("X-User-Id", "2")
	h.ServeHTTP(w, r)
	if w.Code != 200 || s.reads != 1 || s.inits != 0 || s.updates != 0 || s.user != 9007199254740993 {
		t.Fatal(w.Code, w.Body, s)
	}
	for _, part := range []string{`"success":true`, `"user_id":"9007199254740993"`, `"profile_version":"0"`, `"nickname_changed_at":null`, `"next_rename_at":null`, `"status":"INCOMPLETE"`, `"avatar_id":"system-default"`, `"source":"SYSTEM"`, `"short_account_id":"CA-76F010C76AD3"`} {
		if !strings.Contains(w.Body.String(), part) {
			t.Fatal(part, w.Body)
		}
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal(w.Header())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, walletReq("POST", "/platform/v1/master-profile/initialize", validProfileInit))
	if w.Code != 200 || s.inits != 1 || s.expected != 0 || s.name != "Alice" || s.avatar != "system-default" {
		t.Fatal(w.Code, w.Body, s)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, walletReq("PATCH", "/platform/v1/master-profile", `{"expected_version":"9007199254740993","display_name":"Alice"}`))
	if w.Code != 200 || s.updates != 1 || s.patch.ExpectedVersion != 9007199254740993 || s.patch.DisplayName == nil || *s.patch.DisplayName != "Alice" || s.patch.AvatarID != nil {
		t.Fatal(w.Code, w.Body, s)
	}
}

func TestProfileHTTPStrictRequests(t *testing.T) {
	for _, tc := range []struct {
		method, path, body string
		want               int
	}{
		{"GET", "/platform/v1/master-profile?user_id=2", "", 400}, {"GET", "/platform/v1/master-profile?", "", 400},
		{"GET", "/platform/v1/master-profile?%zz", "", 400}, {"GET", "/platform/v1/master-profile/initialize", "", 405},
		{"DELETE", "/platform/v1/master-profile", "", 405}, {"POST", "/platform/v1/master-profile", "", 405},
		{"PATCH", "/platform/v1/master-profile/initialize", validProfileInit, 405}, {"GET", "/platform/v1/master-profile/other", "", 404},
		{"POST", "/platform/v1/master-profile/initialize", `{"expected_version":"0","display_name":"Alice"}`, 400},
		{"POST", "/platform/v1/master-profile/initialize", `{"expected_version":"1","display_name":"Alice","avatar_id":"system-default"}`, 400},
		{"POST", "/platform/v1/master-profile/initialize", `{"expected_version":0,"display_name":"Alice","avatar_id":"system-default"}`, 400},
		{"POST", "/platform/v1/master-profile/initialize", `{"expected_version":"0","display_name":"Alice","avatar_id":"system-default","user_id":"2"}`, 400},
		{"POST", "/platform/v1/master-profile/initialize", `{"expected_version":"0","display_name":"Alice","display_name":"Bob","avatar_id":"system-default"}`, 400},
		{"POST", "/platform/v1/master-profile/initialize", `{"expected_version":"0","display_name":null,"avatar_id":"system-default"}`, 400},
		{"POST", "/platform/v1/master-profile/initialize", `{"expected_version":"0","display_name":"` + string([]byte{255}) + `","avatar_id":"system-default"}`, 400},
		{"POST", "/platform/v1/master-profile/initialize", "null", 400}, {"POST", "/platform/v1/master-profile/initialize", validProfileInit + "{}", 400},
		{"POST", "/platform/v1/master-profile/initialize", "[]", 400}, {"POST", "/platform/v1/master-profile/initialize", strings.Repeat(" ", 8193), 400},
		{"PATCH", "/platform/v1/master-profile", `{"expected_version":"1"}`, 400}, {"PATCH", "/platform/v1/master-profile", `{"expected_version":"0","avatar_id":"system-default"}`, 400},
		{"PATCH", "/platform/v1/master-profile", `{"expected_version":"01","display_name":"Alice"}`, 400}, {"PATCH", "/platform/v1/master-profile", `{"expected_version":"9223372036854775808","display_name":"Alice"}`, 400},
		{"PATCH", "/platform/v1/master-profile", `{"expected_version":"1","avatar_id":null}`, 400}, {"PATCH", "/platform/v1/master-profile", `{"expected_version":"1","display_name":false}`, 400},
		{"PATCH", "/platform/v1/master-profile", `{"expected_version":"1","display_name":{}}`, 400}, {"PATCH", "/platform/v1/master-profile", `{"expected_version":"1","avatar_id":"system-default","expected_version":"1"}`, 400},
	} {
		t.Run(tc.method+tc.path+tc.body, func(t *testing.T) {
			s := &profileTestStore{}
			w := httptest.NewRecorder()
			newProfileHandler("https://wallet.example", s, profileTransport()).ServeHTTP(w, walletReq(tc.method, tc.path, tc.body))
			if w.Code != tc.want || s.inits+s.updates+s.reads != 0 {
				t.Fatal(w.Code, w.Body, s)
			}
		})
	}
}

func TestProfileHTTPOriginContentTypeAndAuth(t *testing.T) {
	for _, tc := range []struct {
		key, value string
		want       int
	}{
		{"Origin", "", 403}, {"Origin", "null", 403}, {"Origin", "https://wallet.example/", 403}, {"Origin", "https://evil.invalid", 403},
		{"Content-Type", "text/plain", 415}, {"Content-Type", "", 415}, {"Authorization", "", 401}, {"New-Api-User", "2", 401},
	} {
		s := &profileTestStore{}
		r := walletReq("POST", "/platform/v1/master-profile/initialize", validProfileInit)
		r.Header.Set(tc.key, tc.value)
		w := httptest.NewRecorder()
		newProfileHandler("https://wallet.example", s, profileTransport()).ServeHTTP(w, r)
		if w.Code != tc.want || s.inits != 0 {
			t.Fatal(tc, w.Code, w.Body)
		}
	}
	for _, key := range []string{"Origin", "Content-Type"} {
		s := &profileTestStore{}
		r := walletReq("POST", "/platform/v1/master-profile/initialize", validProfileInit)
		r.Header.Add(key, r.Header.Get(key))
		w := httptest.NewRecorder()
		newProfileHandler("https://wallet.example", s, profileTransport()).ServeHTTP(w, r)
		if w.Code == 200 || s.inits != 0 {
			t.Fatal("duplicate header", key, w.Code)
		}
	}
	for _, status := range []int{401, 403, 500} {
		s := &profileTestStore{}
		tr := walletTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "http://unix/api/user/self" || r.Header.Get("Cookie") != "" {
				t.Error("bad native verification", r)
			}
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("private"))}, nil
		})
		r := walletReq("GET", "/platform/v1/master-profile", "")
		r.Header.Set("Cookie", "secret")
		w := httptest.NewRecorder()
		newProfileHandler("https://wallet.example", s, tr).ServeHTTP(w, r)
		want := status
		if status == 500 {
			want = 502
		}
		if w.Code != want || s.reads != 0 || strings.Contains(w.Body.String(), "private") {
			t.Fatal(w.Code, w.Body)
		}
	}
}

func TestProfileHTTPErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
		code string
	}{
		{platform.ErrNicknameTaken, 409, "NICKNAME_TAKEN"}, {platform.ErrStaleProfileVersion, 409, "STALE_RESOURCE_VERSION"}, {platform.ErrRenameCooldown, 409, "RENAME_COOLDOWN"},
		{platform.ErrNicknameReserved, 403, "NICKNAME_RESERVED"}, {platform.ErrInvalidNickname, 400, "INVALID_NICKNAME"}, {platform.ErrInvalidAvatar, 400, "INVALID_AVATAR"}, {platform.ErrInvalidProfile, 400, "INVALID_REQUEST"}, {errors.New("password secret host private"), 503, "PROFILE_UNAVAILABLE"},
	} {
		w := httptest.NewRecorder()
		newProfileHandler("https://wallet.example", &profileTestStore{err: tc.err}, profileTransport()).ServeHTTP(w, walletReq("PATCH", "/platform/v1/master-profile", `{"expected_version":"1","display_name":"Alice"}`))
		if w.Code != tc.want || !strings.Contains(w.Body.String(), tc.code) || strings.Contains(w.Body.String(), "private") {
			t.Fatal(tc, w.Code, w.Body)
		}
	}
}

func TestProfileRoutesOptionalAndIndependent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("portal"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, cfg := range []config{{}, {WebDir: dir}} {
		w := httptest.NewRecorder()
		newPortalHandler(cfg, profileTransport()).ServeHTTP(w, walletReq("GET", "/platform/v1/master-profile", ""))
		if w.Code != 503 || !strings.Contains(w.Body.String(), "PROFILE_UNAVAILABLE") {
			t.Fatal(w.Code, w.Body)
		}
	}
	w := httptest.NewRecorder()
	newPortalHandler(config{WebDir: dir}, profileTransport()).ServeHTTP(w, walletReq("GET", "/master-profile", ""))
	if w.Code != 200 || w.Body.String() != "portal" {
		t.Fatal("browser route", w.Code, w.Body)
	}
}
