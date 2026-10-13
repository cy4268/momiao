package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"os"

	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/redis/go-redis/v9"
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

// The existing authority regression now uses local dev accounts and Redis cookies.
func TestAnnouncementHTTPAuthority(t *testing.T) {
	path := os.Getenv("MOMIAO_SESSION_DSN_FILE")
	if path == "" {
		t.Skip("requires existing isolated dev PG and Redis")
	}
	dsn, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read dev DSN")
	}
	store, err := platform.Open(t.Context(), strings.TrimSpace(string(dsn)))
	if err != nil {
		t.Fatal("open dev database")
	}
	defer store.Close()
	options, err := readSessionRedisConfig(os.Getenv("MOMIAO_SESSION_REDIS_CONFIG_FILE"))
	if err != nil {
		t.Fatal("read dev Redis config")
	}
	redisClient := redis.NewClient(options)
	defer redisClient.Close()
	auth := &identity.HTTP{Store: &identity.Store{Pool: store.Pool()}, Sessions: identity.Sessions{Redis: redisClient}, Origin: "https://dev.momiao.win"}
	var admin, regular int64
	if err = store.Pool().QueryRow(t.Context(), `SELECT min(id) FILTER(WHERE role>=10),min(id) FILTER(WHERE role<10) FROM identity.users WHERE status=1 AND deleted_at IS NULL`).Scan(&admin, &regular); err != nil {
		t.Fatal(err)
	}
	sessions := map[int64]identity.Session{}
	for _, user := range []int64{admin, regular} {
		value, e := auth.Sessions.Create(t.Context(), user)
		if e != nil {
			t.Fatal(e)
		}
		sessions[user] = value
		defer func() {
			if e := auth.Sessions.Revoke(context.Background(), value); e != nil {
				t.Error(e)
			}
		}()
	}
	for _, webDir := range []string{"", "/unused-announcements-web"} {
		cfg := config{WebDir: webDir, PublicOrigin: auth.Origin, announcements: store, localIdentity: auth}
		cfg.Session.Enabled = true
		h := newPortalHandler(cfg)
		request := func(user int64, method, path, body string, csrf bool) *httptest.ResponseRecorder {
			r := httptest.NewRequest(method, path, strings.NewReader(body))
			r.Header.Set("Origin", auth.Origin)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("New-Api-User", "1")
			r.Header.Set("X-Auth-Session", "forged")
			if value, ok := sessions[user]; ok {
				r.AddCookie(&http.Cookie{Name: identity.SessionCookieName, Value: value.Token})
				if csrf {
					r.Header.Set("X-CSRF-Token", value.CSRFToken)
				}
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			return w
		}
		const ops = "/platform/v1/ops/announcements"
		w := request(admin, "GET", ops, "", false)
		if w.Code != 200 {
			t.Fatal("local announcement Ops", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), `"authz_epoch"`) || strings.Contains(w.Body.String(), `"principal"`) {
			t.Fatal("legacy authority or cached Ops response", w.Body.String())
		}
		var page struct {
			Data struct {
				Items []platform.OpsAnnouncement `json:"items"`
			} `json:"data"`
		}
		if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Data.Items) == 0 {
			t.Fatal("existing dev announcements", err)
		}
		if w = request(admin, "GET", ops+"/"+page.Data.Items[0].ID, "", false); w.Code != 200 {
			t.Fatal("local detail", w.Code, w.Body.String())
		}
		if w = request(regular, "GET", ops, "", false); w.Code != 403 {
			t.Fatal("regular Ops access", w.Code)
		}
		if w = request(0, "GET", ops, "", false); w.Code != 401 {
			t.Fatal("anonymous Ops access", w.Code)
		}
		if w = request(admin, "POST", ops, `{"action":"SAVE"}`, false); w.Code != 403 {
			t.Fatal("missing CSRF", w.Code)
		}
		if w = request(admin, "POST", ops, `{"action":"SAVE"}`, true); w.Code != 400 {
			t.Fatal("invalid content", w.Code, w.Body.String())
		}
		if w = request(admin, "POST", ops+"/prepare", `{}`, true); w.Code != 404 {
			t.Fatal("legacy prepare", w.Code)
		}
		if w = request(admin, "POST", ops+"/execute", `{}`, true); w.Code != 404 {
			t.Fatal("legacy execute", w.Code)
		}
		if w = request(admin, "POST", ops+"/render-preview", `{"content":{"body_markdown":"## Preview\n\n**Dev content**"}}`, true); w.Code != 200 || !strings.Contains(w.Body.String(), "Dev content") {
			t.Fatal("server render", w.Code, w.Body.String())
		}
		if w = request(admin, "POST", ops+"/render-preview", `{"content":{"body_markdown":"<script>alert(1)</script>"}}`, true); w.Code != 400 {
			t.Fatal("unsafe markdown", w.Code)
		}
		if w = request(admin, "POST", ops, `{"action":"SAVE"} trailing`, true); w.Code != 400 {
			t.Fatal("malformed write", w.Code)
		}
		if w = request(0, "GET", "/platform/v1/announcements", "", false); w.Code != 200 {
			t.Fatal("public list", w.Code, w.Body.String())
		}
		if w = request(regular, "GET", "/platform/v1/announcements/current-post-login-popup", "", false); w.Code != 200 {
			t.Fatal("local post-login popup", w.Code, w.Body.String())
		}
		if w = request(0, "GET", "/platform/v1/announcements/current-post-login-popup", "", false); w.Code != 401 {
			t.Fatal("anonymous popup", w.Code)
		}
	}
}
