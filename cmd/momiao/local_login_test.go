package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Uses the imported dev account and existing PostgreSQL/Redis, not a fixture.
func TestLocalPasswordHTTP(t *testing.T) {
	passwordFile := os.Getenv("MOMIAO_IDENTITY_TEST_PASSWORD_FILE")
	if passwordFile == "" {
		t.Skip("requires the existing dev account password file")
	}
	password, err := os.ReadFile(passwordFile)
	if err != nil {
		t.Fatal("read test password")
	}
	dsn, err := os.ReadFile(os.Getenv("MOMIAO_SESSION_DSN_FILE"))
	if err != nil {
		t.Fatal("read dev DSN")
	}
	pool, err := pgxpool.New(t.Context(), strings.TrimSpace(string(dsn)))
	if err != nil {
		t.Fatal("open dev database")
	}
	t.Cleanup(pool.Close)
	options, err := readSessionRedisConfig(os.Getenv("MOMIAO_SESSION_REDIS_CONFIG_FILE"))
	if err != nil {
		t.Fatal("read dev Redis config")
	}
	client := redis.NewClient(options)
	t.Cleanup(func() { _ = client.Close() })
	// Match the already provided owner password without printing credentials.
	rows, err := pool.Query(t.Context(), `SELECT username,password_hash FROM identity.users WHERE role>=10 AND status=1 AND deleted_at IS NULL`)
	if err != nil {
		t.Fatal("read imported dev owner")
	}
	var username string
	for rows.Next() {
		var name, hash string
		if err := rows.Scan(&name, &hash); err != nil {
			t.Fatal(err)
		}
		if identity.ValidatePasswordAndHash(strings.TrimSpace(string(password)), hash) {
			username = name
		}
	}
	rows.Close()
	if rows.Err() != nil || username == "" {
		t.Fatal("existing dev owner credentials not found")
	}
	discord, err := readDiscordConfig(os.Getenv("MOMIAO_DISCORD_CONFIG_FILE"), "https://dev.momiao.win")
	if err != nil {
		t.Fatal("read dev Discord configuration")
	}
	auth := &identity.HTTP{Store: &identity.Store{Pool: pool}, Sessions: identity.Sessions{Redis: client}, Origin: "https://dev.momiao.win", Discord: discord}
	wallets, err := platform.Open(t.Context(), strings.TrimSpace(string(dsn)))
	if err != nil {
		t.Fatal("open wallet reader")
	}
	t.Cleanup(wallets.Close)
	handler := newPortalHandler(config{Session: sessionConfig{Enabled: true}, sessionAuth: auth, localIdentity: auth, wallet: wallets}, nil)
	call := func(method, path string, body any, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "https://dev.momiao.win"+path, bytes.NewReader(raw))
		r.Header.Set("Origin", "https://dev.momiao.win")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	var response struct {
		Data struct {
			State string `json:"authentication_state"`
			CSRF  string `json:"csrf_token"`
		} `json:"data"`
	}
	bootstrap := call("GET", "/api/v1/session/bootstrap", nil, nil, "")
	if bootstrap.Code != 200 || json.Unmarshal(bootstrap.Body.Bytes(), &response) != nil || response.Data.State != "ANONYMOUS" {
		t.Fatal("anonymous bootstrap", bootstrap.Code)
	}
	anonymous := bootstrap.Result().Cookies()[0]
	start := call("POST", "/api/v1/auth/discord/login/start", map[string]string{}, anonymous, response.Data.CSRF)
	var oauth struct {
		Data struct {
			URL string `json:"authorization_url"`
		} `json:"data"`
	}
	if start.Code != 200 || json.Unmarshal(start.Body.Bytes(), &oauth) != nil {
		t.Fatal("Discord start", start.Code)
	}
	destination, err := url.Parse(oauth.Data.URL)
	if err != nil || destination.Host != "discord.com" || destination.Query().Get("redirect_uri") != "https://dev.momiao.win/oauth/discord" || destination.Query().Get("scope") != "identify" {
		t.Fatal("Discord authorization destination")
	}
	state := destination.Query().Get("state")
	if len(state) != 43 {
		t.Fatal("Discord state missing")
	}
	// The second browser must not consume the original browser's state.
	other := call("GET", "/api/v1/session/bootstrap", nil, nil, "")
	var otherResponse struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		} `json:"data"`
	}
	if json.Unmarshal(other.Body.Bytes(), &otherResponse) != nil {
		t.Fatal("second browser bootstrap")
	}
	cancel := map[string]string{"state": state, "error": "access_denied"}
	crossed := call("POST", "/api/v1/auth/discord/callback", cancel, other.Result().Cookies()[0], otherResponse.Data.CSRF)
	if crossed.Code != 403 || !strings.Contains(crossed.Body.String(), "AUTH_STATE_INVALID") {
		t.Fatal("cross-browser state", crossed.Code)
	}
	cancelled := call("POST", "/api/v1/auth/discord/callback", cancel, anonymous, response.Data.CSRF)
	if cancelled.Code != 403 || !strings.Contains(cancelled.Body.String(), "DISCORD_DENIED") {
		t.Fatal("OAuth cancellation", cancelled.Code)
	}
	replay := call("POST", "/api/v1/auth/discord/callback", cancel, anonymous, response.Data.CSRF)
	if replay.Code != 403 || !strings.Contains(replay.Body.String(), "AUTH_STATE_INVALID") {
		t.Fatal("OAuth state replay", replay.Code)
	}
	body := map[string]string{"identifier": username, "password": strings.TrimSpace(string(password))}
	if denied := call("POST", "/api/v1/auth/password/login", body, anonymous, ""); denied.Code != 403 {
		t.Fatal("login accepted missing CSRF", denied.Code)
	}
	if denied := call("POST", "/api/v1/auth/password/login", map[string]string{"identifier": username, "password": body["password"] + "wrong"}, anonymous, response.Data.CSRF); denied.Code != 401 {
		t.Fatal("login accepted incorrect password", denied.Code)
	}
	login := call("POST", "/api/v1/auth/password/login", body, anonymous, response.Data.CSRF)
	if login.Code != 200 || json.Unmarshal(login.Body.Bytes(), &response) != nil || response.Data.State != "AUTHENTICATED" {
		t.Fatal("local password login", login.Code)
	}
	cookie := login.Result().Cookies()[0]
	csrf := response.Data.CSRF
	t.Cleanup(func() { call("POST", "/api/v1/session/logout", nil, cookie, csrf) })
	if cookie.Value == anonymous.Value || !cookie.Secure || !cookie.HttpOnly {
		t.Fatal("login did not rotate secure session")
	}
	r := httptest.NewRequest("GET", "https://dev.momiao.win/protected", nil)
	r.AddCookie(cookie)
	r.Header.Set("Authorization", "Bearer ignored")
	r.Header.Set("New-Api-User", "999999")
	prepared, err := auth.Request(r, false)
	if err != nil {
		t.Fatal("local request identity", err)
	}
	user, ok := identity.UserFromContext(prepared.Context())
	if !ok || user.Username != username {
		t.Fatal("request identity mismatch")
	}
	if id, status := verifyWalletUser(prepared, nil); status != 0 || id != user.ID {
		t.Fatal("wallet still requires native credentials", status)
	}
	if binding := identity.SessionIDFromContext(prepared.Context()); binding == "" || binding == cookie.Value {
		t.Fatal("game session binding missing or exposes cookie")
	}
	if prepared.Header.Get("Authorization") != "" || prepared.Header.Get("New-Api-User") != "" {
		t.Fatal("native credential projection remains")
	}
	self := call("GET", "/api/user/self", nil, cookie, "")
	var local struct {
		Data struct {
			ID            int64  `json:"id"`
			Quota         int64  `json:"quota"`
			ReserveAmount string `json:"reserve_amount"`
		} `json:"data"`
	}
	if self.Code != 200 || json.Unmarshal(self.Body.Bytes(), &local) != nil || local.Data.ID != user.ID {
		t.Fatal("local self", self.Code)
	}
	balances, err := wallets.ReadWallets(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	var reserve int64
	for _, balance := range balances {
		if balance.Asset == platform.ReserveAPICredit {
			reserve = balance.BalanceUnits
		}
	}
	if local.Data.Quota != reserve || local.Data.ReserveAmount != platform.FormatAmount(reserve) {
		t.Fatal("self does not expose Reserve")
	}
	if strings.Contains(self.Body.String(), "password") {
		t.Fatal("self exposes credentials")
	}
	if denied := call("GET", "/api/user/self", nil, anonymous, ""); denied.Code != 401 {
		t.Fatal("anonymous self", denied.Code)
	}
	account := call("GET", "/api/v1/account", nil, cookie, "")
	var status struct {
		Data struct {
			ID            int64 `json:"id"`
			HasPassword   bool  `json:"has_password"`
			DiscordLinked bool  `json:"discord_linked"`
		} `json:"data"`
	}
	if account.Code != 200 || json.Unmarshal(account.Body.Bytes(), &status) != nil || status.Data.ID != user.ID || !status.Data.HasPassword || status.Data.DiscordLinked != (user.DiscordID != "") {
		t.Fatal("local account status", account.Code)
	}
	if denied := call("GET", "/api/v1/account", nil, anonymous, ""); denied.Code != 401 {
		t.Fatal("anonymous account status", denied.Code)
	}
	logout := call("POST", "/api/v1/session/logout", nil, cookie, csrf)
	if logout.Code != 200 {
		t.Fatal("logout", logout.Code)
	}
	if _, err := auth.Request(r, false); err == nil {
		t.Fatal("logout left authenticated request active")
	}
}
