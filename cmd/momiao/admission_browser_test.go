package main

// Opt-in browser acceptance harness. This file is excluded from every production
// binary. The portal and PostgreSQL are real; native identity/provider responses
// are stateful synthetic fixtures with no Discord request or real credential.
import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
	"html/template"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

type browserUser struct {
	ID             int64
	Name, Password string
	TwoFA          bool
}
type browserSession struct {
	User       int64
	SID, Token string
}
type browserFlow struct {
	User                int64
	Purpose, SID, Nonce string
}
type admissionBrowserFixture struct {
	mu                            sync.Mutex
	origin                        string
	seq                           int64
	users                         map[int64]*browserUser
	sessions                      map[string]browserSession
	flows, challenges, proofs     map[string]browserFlow
	receipts                      []platform.RegistrationReceipt
	sourceOffline, recoveryPaused bool
}

func (f *admissionBrowserFixture) next() string {
	f.seq++
	return fmt.Sprintf("42684268-0000-4000-8000-%012x", f.seq)
}
func fixtureCookie(r *http.Request, key string) string {
	c, e := r.Cookie(key)
	if e != nil {
		return ""
	}
	return c.Value
}
func (f *admissionBrowserFixture) identity(r *http.Request) (browserSession, bool) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	for _, s := range f.sessions {
		if s.Token == token && r.Header.Get("New-Api-User") == fmt.Sprint(s.User) && r.Header.Get("X-Auth-Session") == s.SID {
			return s, true
		}
	}
	return browserSession{}, false
}
func (f *admissionBrowserFixture) bundle(w http.ResponseWriter, user int64) {
	id := f.next()
	s := browserSession{user, id, f.nativeToken(user, id)}
	f.sessions[id] = s
	http.SetCookie(w, &http.Cookie{Name: "m2b_refresh", Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	walletSuccess(w, f.bundleData(s))
}

// Matches the reviewed dashboard credential grammar. The synthetic native
// service still authenticates by comparing this exact token with its live map.
func (f *admissionBrowserFixture) nativeToken(user int64, sid string) string {
	now := time.Now()
	claims, _ := json.Marshal(map[string]any{"iss": "new-api", "aud": []string{"new-api-dashboard"}, "token_use": "access", "sub": fmt.Sprint(user), "sid": sid, "uv": 1, "sv": 1, "exp": now.Add(time.Hour).Unix(), "iat": now.Unix(), "nbf": now.Add(-time.Second).Unix(), "jti": f.next()})
	raw := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(claims)
	mac := hmac.New(sha256.New, []byte("public-synthetic-browser-fixture-key"))
	mac.Write([]byte(raw))
	return raw + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (f *admissionBrowserFixture) bundleData(s browserSession) map[string]any {
	u := f.users[s.User]
	return map[string]any{"access_token": s.Token, "access_expires_at": time.Now().Add(time.Hour).Unix(), "session": map[string]any{"sid": s.SID}, "user": map[string]any{"id": u.ID, "username": u.Name, "display_name": u.Name, "role": 1, "status": 1, "quota": 0, "used_quota": 0, "request_count": 0}}
}
func (f *admissionBrowserFixture) finish(w http.ResponseWriter, flow browserFlow) {
	if flow.Purpose == "fresh" || flow.Purpose == "password-reset" {
		if _, ok := f.sessions[flow.SID]; !ok {
			walletError(w, 401, "MOMIAO_AUTH_RESTART_REQUIRED")
			return
		}
		proof := f.next()
		f.proofs[proof] = flow
		walletSuccess(w, map[string]any{"proof": proof, "expires_at": time.Now().Add(10 * time.Minute).Unix()})
		return
	}
	f.bundle(w, flow.User)
}
func (f *admissionBrowserFixture) native(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path == "/internal/momiao/registrations" {
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("k", 32) {
			walletError(w, 403, "FORBIDDEN")
			return
		}
		if f.sourceOffline {
			walletError(w, 503, "MOMIAO_UNAVAILABLE")
			return
		}
		after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		rows := []platform.RegistrationReceipt{}
		next := after
		for _, receipt := range f.receipts {
			if receipt.Ordinal > after && len(rows) < limit {
				rows = append(rows, receipt)
				next = receipt.Ordinal
			}
		}
		walletSuccess(w, platform.RegistrationPage{Receipts: rows, NextCursor: next})
		return
	}
	if r.Method == "POST" && r.Header.Get("Origin") != f.origin {
		walletError(w, 403, "ORIGIN_REJECTED")
		return
	}
	var body map[string]string
	if r.Method == "POST" {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body)
	}
	session, authenticated := f.identity(r)
	switch {
	case r.URL.Path == "/api/user/auth/refresh":
		s, ok := f.sessions[fixtureCookie(r, "m2b_refresh")]
		if !ok {
			walletError(w, 401, "AUTH_UNAUTHORIZED")
			return
		}
		walletSuccess(w, f.bundleData(s))
	case r.URL.Path == "/api/user/auth/logout":
		delete(f.sessions, fixtureCookie(r, "m2b_refresh"))
		for key, flow := range f.flows {
			if flow.Nonce == fixtureCookie(r, "m2b_oauth") {
				delete(f.flows, key)
			}
		}
		http.SetCookie(w, &http.Cookie{Name: "m2b_refresh", Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
		walletSuccess(w, map[string]any{})
	case r.URL.Path == "/api/user/self":
		if !authenticated {
			walletError(w, 401, "AUTH_UNAUTHORIZED")
			return
		}
		walletSuccess(w, f.bundleData(session)["user"])
	case r.URL.Path == "/api/user/login":
		var found *browserUser
		for _, u := range f.users {
			if u.Name == body["username"] && u.Password != "" && u.Password == body["password"] {
				found = u
			}
		}
		if found == nil {
			walletError(w, 400, "MOMIAO_INVALID_REQUEST")
			return
		}
		flow := browserFlow{User: found.ID, Purpose: "login"}
		if found.TwoFA {
			id := f.next()
			f.challenges[id] = flow
			walletSuccess(w, map[string]any{"require_2fa": true, "flow_token": id})
			return
		}
		f.finish(w, flow)
	case strings.HasPrefix(r.URL.Path, "/api/momiao/auth/discord/") && strings.HasSuffix(r.URL.Path, "/start"):
		purpose := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/momiao/auth/discord/"), "/start")
		selected := int64(12)
		if fixtureCookie(r, "m2b_subject") == "13" {
			selected = 13
		}
		flow := browserFlow{User: selected, Purpose: purpose}
		if purpose == "fresh" || purpose == "password-reset" {
			if !authenticated {
				walletError(w, 401, "AUTH_UNAUTHORIZED")
				return
			}
			flow.User = session.User
			flow.SID = session.SID
		}
		nonce := f.next()
		flow.Nonce = nonce
		state := f.next()
		f.flows[state] = flow
		http.SetCookie(w, &http.Cookie{Name: "m2b_oauth", Value: nonce, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		q := url.Values{"client_id": {"123456789012345678"}, "redirect_uri": {f.origin + "/oauth/discord"}, "response_type": {"code"}, "scope": {"identify"}, "state": {state}}
		if purpose == "registration" {
			q.Set("scope", "identify guilds.members.read")
		}
		walletSuccess(w, map[string]any{"authorization_url": "https://discord.com/oauth2/authorize?" + q.Encode()})
	case r.URL.Path == "/api/momiao/auth/discord/callback":
		state := r.URL.Query().Get("state")
		flow, ok := f.flows[state]
		if !ok || flow.Nonce != fixtureCookie(r, "m2b_oauth") || r.URL.Query().Get("code") != "synthetic-authorization-code" {
			walletError(w, 401, "MOMIAO_AUTH_RESTART_REQUIRED")
			return
		}
		delete(f.flows, state)
		if f.users[flow.User] == nil {
			if flow.Purpose != "registration" {
				walletError(w, 403, "MOMIAO_DISCORD_UNBOUND")
				return
			}
			f.users[flow.User] = &browserUser{ID: flow.User, Name: "discord_native_12"}
			f.receipts = append(f.receipts, platform.RegistrationReceipt{Ordinal: int64(len(f.receipts) + 1), OperationID: f.next(), NativeUserID: flow.User, DiscordSubject: "923456789012345678", Source: "NEW_DISCORD_REGISTRATION", PolicyVersion: "browser-policy-v1", CreatedAt: time.Now().UTC().Truncate(time.Microsecond)})
		}
		if f.users[flow.User].TwoFA {
			id := f.next()
			f.challenges[id] = flow
			walletSuccess(w, map[string]any{"require_2fa": true, "flow_token": id})
			return
		}
		f.finish(w, flow)
	case r.URL.Path == "/api/momiao/auth/2fa" || r.URL.Path == "/api/user/login/2fa":
		flow, ok := f.challenges[body["flow_token"]]
		if !ok || body["code"] != "123456" {
			walletError(w, 400, "MOMIAO_INVALID_REQUEST")
			return
		}
		delete(f.challenges, body["flow_token"])
		f.finish(w, flow)
	case r.URL.Path == "/api/momiao/account":
		if !authenticated {
			walletError(w, 401, "AUTH_UNAUTHORIZED")
			return
		}
		u := f.users[session.User]
		walletSuccess(w, map[string]any{"id": u.ID, "username": u.Name, "has_password": u.Password != "", "discord_connected": true, "two_fa_enabled": u.TwoFA})
	case strings.HasPrefix(r.URL.Path, "/api/momiao/account/password/"):
		if !authenticated {
			walletError(w, 401, "AUTH_UNAUTHORIZED")
			return
		}
		u := f.users[session.User]
		mode := strings.TrimPrefix(r.URL.Path, "/api/momiao/account/password/")
		proof, ok := f.proofs[body["proof"]]
		valid := false
		switch mode {
		case "change":
			valid = u.Password != "" && body["old_password"] == u.Password
		case "set":
			valid = u.Password == "" && ok && proof.User == u.ID && proof.SID == session.SID && proof.Purpose == "fresh"
		case "reset":
			valid = ok && proof.User == u.ID && proof.SID == session.SID && proof.Purpose == "password-reset"
		}
		pw := body["password"]
		if !valid || utf8.RuneCountInString(pw) < 8 || utf8.RuneCountInString(pw) > 20 || len(pw) > 72 {
			walletError(w, 400, "MOMIAO_INVALID_REQUEST")
			return
		}
		delete(f.proofs, body["proof"])
		u.Password = pw
		for id, s := range f.sessions {
			if s.User == u.ID && id != session.SID {
				delete(f.sessions, id)
			}
		}
		session.Token = f.nativeToken(session.User, session.SID)
		f.sessions[session.SID] = session
		walletSuccess(w, map[string]any{"access_token": session.Token, "access_expires_at": time.Now().Add(time.Hour).Unix(), "session": map[string]any{"sid": session.SID}, "has_password": true})
	default:
		walletSuccess(w, map[string]any{"items": []any{}, "total": 0, "page": 1, "page_size": 10})
	}
}

type browserRecoveryStore struct {
	*platform.Store
	fixture *admissionBrowserFixture
}

func (s browserRecoveryStore) RecoverRegistrationGrant(ctx context.Context) (bool, error) {
	s.fixture.mu.Lock()
	paused := s.fixture.recoveryPaused
	s.fixture.mu.Unlock()
	if paused {
		return false, nil
	}
	return s.Store.RecoverRegistrationGrant(ctx)
}

func TestAdmissionBrowserHarness(t *testing.T) {
	if os.Getenv("MOMIAO_BROWSER_HARNESS") != "1" {
		t.Skip("opt-in real browser harness")
	}
	dsn := os.Getenv("MOMIAO_TEST_DATABASE_URL")
	dbConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(dbConfig.ConnConfig.Database, "momiao_test_m2b_browser_") {
		t.Fatal("dedicated M2b browser database required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, err := platform.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = store.InitializeProfile(ctx, 13, 0, "既有观测员", "system-default"); err != nil {
		t.Fatal(err)
	}
	fixture := &admissionBrowserFixture{users: map[int64]*browserUser{13: {ID: 13, Name: "legacy_native_13", Password: "LegacyPass123", TwoFA: true}}, sessions: map[string]browserSession{}, flows: map[string]browserFlow{}, challenges: map[string]browserFlow{}, proofs: map[string]browserFlow{}, receipts: []platform.RegistrationReceipt{}, recoveryPaused: true}
	combined := os.Getenv("MOMIAO_PORTAL_COMBINED_BROWSER") == "1"
	if combined {
		// Test-only seeding in the dedicated synthetic DB, never an HTTP
		// bootstrap or proof of production deployment-source verification.
		if _, err = store.Bootstrap(ctx, platform.BootstrapInput{Environment: "STAGING", UserID: 14, Username: "synthetic-portal-seed", ReleaseBuild: "synthetic-portal-integration", ExpectedEmpty: true}); err != nil {
			t.Fatal(err)
		}
		principal, err := store.AnnouncementAuthority(ctx, 14)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range []struct{ visibility, title string }{{"PUBLIC", "组合验收：公开公告"}, {"AUTHENTICATED", "组合验收：登录后公告"}} {
			draft, err := store.ExecuteAnnouncement(ctx, 14, platform.AnnouncementCommand{OperationID: fixture.next(), Epoch: principal.Epoch, Action: "SAVE", Content: &platform.AnnouncementContent{Title: item.title, Type: "SYSTEM", Visibility: item.visibility, Markdown: "这是独立组合数据库中的合成验收公告。"}}, "", false)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC().Add(-time.Minute)
			command := platform.AnnouncementCommand{OperationID: fixture.next(), Epoch: principal.Epoch, ID: draft.ID, ExpectedVersion: draft.Version, Action: "PUBLISH", PublishAt: &now, VisibleFrom: &now, Reason: "Synthetic integration seed"}
			preview, err := store.PrepareAnnouncement(ctx, 14, command)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.ExecuteAnnouncement(ctx, 14, command, preview.ID, true); err != nil {
				t.Fatal(err)
			}
		}
	}
	native := httptest.NewServer(http.HandlerFunc(fixture.native))
	defer native.Close()
	nativeURL, _ := url.Parse(native.URL)
	transport := admissionTransport(func(r *http.Request) (*http.Response, error) {
		copy := r.Clone(r.Context())
		u := *r.URL
		u.Scheme = nativeURL.Scheme
		u.Host = nativeURL.Host
		copy.URL = &u
		return http.DefaultTransport.RoundTrip(copy)
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fixture.origin = "http://" + listener.Addr().String()
	webDir, err := filepath.Abs(filepath.Join("..", "..", "web", "dist"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config{WebDir: webDir, PublicOrigin: fixture.origin, AdmissionEnabled: true, admission: store, wallet: store, profile: store, economy: store}
	if combined {
		cfg.announcements = store
	}
	portal := newPortalHandler(cfg, transport)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/__fixture/") {
			portal.ServeHTTP(w, r)
			return
		}
		fixture.mu.Lock()
		defer fixture.mu.Unlock()
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/__fixture/stop":
			cancel()
		case "/__fixture/release":
			fixture.recoveryPaused = false
		case "/__fixture/offline":
			fixture.sourceOffline = true
		case "/__fixture/online":
			fixture.sourceOffline = false
		case "/__fixture/new":
			http.SetCookie(w, &http.Cookie{Name: "m2b_subject", Value: "12", Path: "/", SameSite: http.SameSiteLaxMode})
		case "/__fixture/legacy":
			http.SetCookie(w, &http.Cookie{Name: "m2b_subject", Value: "13", Path: "/", SameSite: http.SameSiteLaxMode})
		case "/__fixture/twofa":
			if u := fixture.users[12]; u != nil {
				u.TwoFA = true
			}
		case "/__fixture/registration", "/__fixture/login", "/__fixture/fresh", "/__fixture/password-reset":
			// Local test-only provider entry. Production Discord-start responses
			// remain fixed to discord.com and are verified separately by tests.
			purpose := strings.TrimPrefix(r.URL.Path, "/__fixture/")
			flow := browserFlow{User: 12, Purpose: purpose}
			if fixtureCookie(r, "m2b_subject") == "13" {
				flow.User = 13
			}
			valid := true
			if purpose == "fresh" || purpose == "password-reset" {
				s, ok := fixture.sessions[fixtureCookie(r, "m2b_refresh")]
				valid = ok
				flow.User, flow.SID = s.User, s.SID
			}
			if valid {
				flow.Nonce = fixture.next()
				state := fixture.next()
				fixture.flows[state] = flow
				http.SetCookie(w, &http.Cookie{Name: "m2b_oauth", Value: flow.Nonce, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
				// Render the completion link on the following local page so the
				// new browser cookie participates in the real callback request.
				http.Redirect(w, r, "/__fixture/provider", http.StatusSeeOther)
				return
			}
		}
		fmt.Fprint(w, `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>M2b 本地验收夹具</title><body><h1>M2b 本地验收夹具</h1><p>仅使用虚构账户与本地测试数据库。此页面不进入生产构建。</p><nav><a href="/register">新用户注册页面</a> · <a href="/login">登录页面</a> · <a href="/account">账户安全页面</a> · <a href="/wallet">钱包页面</a></nav>`)
		fmt.Fprintf(w, "<p>注册来源可用：%t；赠额恢复暂停：%t；原生注册回执数：%d</p>", !fixture.sourceOffline, fixture.recoveryPaused, len(fixture.receipts))
		fmt.Fprint(w, `<p><a href="/__fixture/new">选择新用户 Discord</a> · <a href="/__fixture/legacy">选择既有用户 Discord</a> · <a href="/__fixture/twofa">为测试新账户开启二次验证</a></p><p><a href="/__fixture/offline">模拟来源不可用</a> · <a href="/__fixture/online">恢复来源服务</a> · <a href="/__fixture/release">继续赠额恢复任务</a></p>`)
		fmt.Fprint(w, `<p>纯本地模拟授权入口：<a href="/__fixture/registration">注册验证</a> · <a href="/__fixture/login">登录验证</a> · <a href="/__fixture/fresh">首个密码身份验证</a> · <a href="/__fixture/password-reset">密码重置身份验证</a></p>`)
		if combined {
			fmt.Fprint(w, `<p><a href="/announcements">组合公告页面</a></p>`)
		}
		for state, flow := range fixture.flows {
			if flow.Nonce == fixtureCookie(r, "m2b_oauth") {
				href := "/oauth/discord?" + url.Values{"code": {"synthetic-authorization-code"}, "state": {state}}.Encode()
				fmt.Fprintf(w, `<p>模拟提供方已验证。用途：%s</p><a href="%s">完成本次模拟 Discord 授权</a>`, template.HTMLEscapeString(flow.Purpose), template.HTMLEscapeString(href))
			}
		}
		fmt.Fprint(w, `</body></html>`)
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	go server.Serve(listener)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		runAdmissionWorker(ctx, browserRecoveryStore{store, fixture}, transport, strings.Repeat("k", 32))
	}()
	defer func() { cancel(); <-workerDone }()
	t.Logf("BROWSER_FIXTURE_PORTAL=%s", fixture.origin)
	runtimePath := os.Getenv("MOMIAO_BROWSER_RUNTIME_FILE")
	if runtimePath != "" {
		body, _ := json.Marshal(map[string]any{"portal_url": fixture.origin, "database": dbConfig.ConnConfig.Database, "synthetic": true})
		if err = os.WriteFile(runtimePath, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	duration := 20 * time.Minute
	if v := os.Getenv("MOMIAO_BROWSER_DURATION"); v != "" {
		if d, e := time.ParseDuration(v); e == nil && d > 0 && d <= 45*time.Minute {
			duration = d
		}
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}
