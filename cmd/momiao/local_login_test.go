package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/history"
	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/poker/connectticket"
	pt "github.com/cy4268/momiao/internal/poker/transport"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp/totp"
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
	auth := &identity.HTTP{Store: &identity.Store{Pool: pool}, Sessions: identity.Sessions{Redis: client}, Origin: "https://dev.momiao.win", Discord: discord, RegistrationEnabled: true}
	wallets, err := platform.Open(t.Context(), strings.TrimSpace(string(dsn)))
	if err != nil {
		t.Fatal("open wallet reader")
	}
	t.Cleanup(wallets.Close)
	historyPool, err := openPokerPool(t.Context(), os.Getenv("MOMIAO_HISTORY_READER_DSN_FILE"))
	if err != nil {
		t.Fatal("open existing dev History reader")
	}
	t.Cleanup(historyPool.Close)
	cfg := config{OpsEnvironment: "DEVELOPMENT", PublicOrigin: auth.Origin, Session: sessionConfig{Enabled: true}, sessionAuth: auth, localIdentity: auth, wallet: wallets, history: &historyHTTP{list: history.NewReader(historyPool)},
		PokerRemoteSocket: os.Getenv("MOMIAO_POKER_REMOTE_SOCKET"), PokerServiceKeyringFile: os.Getenv("MOMIAO_POKER_SERVICE_KEYRING_FILE"), PokerPeerKeyringFile: os.Getenv("MOMIAO_POKER_PEER_KEYRING_FILE"),
		Poker: pokerConfig{Enabled: true, TicketKeyringFile: os.Getenv("MOMIAO_POKER_TICKET_KEYRING_FILE")}}
	remote, err := openPokerRemote(t.Context(), cfg)
	if err != nil {
		t.Fatal("local Poker composition", err)
	}
	t.Cleanup(remote.Close)
	cfg.poker = remote.handler
	cfg.opsMaintenance = newOpsMaintenanceHandler(wallets, "DEVELOPMENT")
	cfg.opsAudit = newOpsAuditHandler(wallets)
	handler := newPortalHandler(cfg, nil)
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
	anonymousCSRF := response.Data.CSRF
	registration := call("POST", "/api/v1/auth/discord/registration/start", nil, anonymous, anonymousCSRF)
	if registration.Code != 200 {
		t.Fatal("Discord registration start", registration.Code)
	}
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
	audits := call("GET", "/api/v1/ops/audit?page=1&page_size=20", nil, cookie, "")
	if audits.Code != 200 || !strings.Contains(audits.Body.String(), `"items":`) {
		t.Fatal("local audit list", audits.Code)
	}
	if denied := call("GET", "/api/v1/ops/audit", nil, nil, ""); denied.Code != 401 {
		t.Fatal("anonymous audit read", denied.Code)
	}
	if invalid := call("GET", "/api/v1/ops/audit?page=-1", nil, cookie, ""); invalid.Code != 400 {
		t.Fatal("invalid audit page", invalid.Code)
	}
	maintenance := call("GET", "/api/v1/ops/maintenance", nil, cookie, "")
	if maintenance.Code != 200 || !strings.Contains(maintenance.Body.String(), `"enabled":false`) {
		t.Fatal("local maintenance read", maintenance.Code)
	}
	if denied := call("GET", "/api/v1/ops/maintenance", nil, nil, ""); denied.Code != 401 {
		t.Fatal("anonymous maintenance read", denied.Code)
	}
	if denied := call("PUT", "/api/v1/ops/maintenance", map[string]any{"enabled": false}, cookie, ""); denied.Code != 403 {
		t.Fatal("maintenance write accepted missing CSRF", denied.Code)
	}
	if invalid := call("PUT", "/api/v1/ops/maintenance", map[string]any{"enabled": true, "scopes": []string{"UNKNOWN"}}, cookie, csrf); invalid.Code != 400 {
		t.Fatal("maintenance accepted invalid scope", invalid.Code)
	}
	var ordinaryID int64
	if err := pool.QueryRow(t.Context(), `SELECT id FROM identity.users WHERE role=1 AND status=1 AND deleted_at IS NULL ORDER BY id LIMIT 1`).Scan(&ordinaryID); err != nil {
		t.Fatal(err)
	}
	ordinary, err := auth.Sessions.Create(t.Context(), ordinaryID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = auth.Sessions.Revoke(context.Background(), ordinary) })
	ordinaryCookie := &http.Cookie{Name: cookie.Name, Value: ordinary.Token}
	if denied := call("GET", "/api/v1/ops/audit", nil, ordinaryCookie, ""); denied.Code != 403 {
		t.Fatal("ordinary user audit read", denied.Code)
	}
	if denied := call("GET", "/api/v1/ops/maintenance", nil, ordinaryCookie, ""); denied.Code != 403 {
		t.Fatal("ordinary user maintenance read", denied.Code)
	}
	principal, err := pokerHTTPAuth(prepared)
	if err != nil || principal.UserID != user.ID || len(principal.SessionIDHash) != 64 {
		t.Fatal("local Poker authentication", err)
	}
	hash := sha256.Sum256([]byte(cookie.Value))
	if principal.SessionIDHash != hex.EncodeToString(hash[:]) {
		t.Fatal("Poker cookie digest differs from local session")
	}
	pokerAuthority := pokerSessionAuthority{sessions: auth.Sessions, pool: pool}
	deadline, err := pokerAuthority.CheckUntil(t.Context(), pokerSession(principal))
	if err != nil || !deadline.After(time.Now()) {
		t.Fatal("local Poker session deadline", err)
	}
	signing, err := readPokerTicketKeys(os.Getenv("MOMIAO_POKER_TICKET_KEYRING_FILE"))
	if err != nil {
		t.Fatal("existing dev Poker signing keys")
	}
	_, ticketOptions, err := readPokerRedisConfig(os.Getenv("MOMIAO_POKER_REDIS_CONFIG_FILE"))
	if err != nil {
		t.Fatal("existing dev Poker Redis configuration")
	}
	ticketRedis := redis.NewClient(ticketOptions)
	t.Cleanup(func() { _ = ticketRedis.Close() })
	consume, err := connectticket.RedisConsumer(ticketRedis)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := connectticket.NewVerifier(connectticket.VerifierOptions{PublicKeys: signing.public, StartedAt: time.Now().Add(-time.Minute), CheckSession: pokerAuthority.Check, Consume: consume})
	if err != nil {
		t.Fatal(err)
	}
	minted := call("POST", "/api/v1/poker/connect-tickets", map[string]any{"target_table_id": nil, "control_intent": connectticket.ReadOnly}, cookie, csrf)
	var ticket struct {
		Data struct {
			Value string `json:"poker_connect_ticket"`
		} `json:"data"`
	}
	if minted.Code != 200 || json.Unmarshal(minted.Body.Bytes(), &ticket) != nil {
		t.Fatal("local Poker ticket route", minted.Code)
	}
	connected, err := pokerTicketAuthentication(verifier)(t.Context(), pt.ConnectRequest{PokerConnectTicket: ticket.Data.Value})
	if err != nil || connected != principal {
		t.Fatal("local Poker ticket authentication", err)
	}
	if _, err := pokerTicketAuthentication(verifier)(t.Context(), pt.ConnectRequest{PokerConnectTicket: ticket.Data.Value}); err == nil {
		t.Fatal("Poker ticket replay accepted")
	}
	historyResponse := call("GET", "/api/v1/history?limit=1", nil, cookie, "")
	if historyResponse.Code != 200 {
		t.Error("local History request", historyResponse.Code)
	}
	if denied := call("GET", "/api/v1/history?limit=1", nil, anonymous, ""); denied.Code != 401 {
		t.Error("anonymous History request", denied.Code)
	}
	var botUser int64
	var botSubject string
	if err := pool.QueryRow(t.Context(), `SELECT u.id,u.discord_id FROM identity.users u
 JOIN identity.master_profiles p ON p.newapi_user_id=u.id
 WHERE u.discord_id IS NOT NULL AND u.status=1 AND u.deleted_at IS NULL AND p.normalized_name IS NOT NULL
 ORDER BY u.id LIMIT 1`).Scan(&botUser, &botSubject); err != nil {
		t.Fatal("existing dev Bot-ready account")
	}
	resolver := botGamesResolver{platform: wallets, declaration: &accessDeclaration{MigrationApplicability: "NO_MIGRATION_APPLICABLE", Resources: map[string]string{"EXPERIENCE": "AVAILABLE"}}}
	if got, err := resolver.Resolve(t.Context(), botSubject); err != nil || got != botUser {
		t.Error("local Bot identity", got, err)
	}
	if t.Failed() {
		t.FailNow()
	}
	prepared.URL, _ = url.Parse("https://dev.momiao.win/platform/v1/access-gate?route=/account")
	gate := httptest.NewRecorder()
	newAccessGateHandler(auth.Origin, wallets, &accessDeclaration{MigrationApplicability: "NO_MIGRATION_APPLICABLE", Resources: map[string]string{"ACCOUNT": "AVAILABLE"}}, nil, false).ServeHTTP(gate, prepared)
	if gate.Code != 200 || !(strings.Contains(gate.Body.String(), `"stage":"READY"`) || strings.Contains(gate.Body.String(), `"stage":"MASTER_REQUIRED"`)) {
		t.Fatal("local access gate", gate.Code)
	}
	cursor, err := wallets.RegistrationCursor(t.Context())
	if err != nil {
		t.Fatal("local registration cursor")
	}
	page, err := wallets.ReadRegistrationPage(t.Context(), max(cursor-1, 0), 1)
	if err != nil || platform.ValidateRegistrationPage(page, max(cursor-1, 0), 1) != nil || page.NextCursor != cursor {
		t.Fatal("local registration receipt read")
	}
	registrationWorkerCycle(t.Context(), wallets)
	current, err := wallets.RegistrationCursor(t.Context())
	if err != nil || current != cursor {
		t.Fatal("local worker changed the existing receipt cursor")
	}
	admission, err := wallets.ReadAdmission(t.Context(), user.ID)
	if err != nil || !admission.SourceAvailable {
		t.Fatal("local registration source unavailable")
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
	// Exercise account mutations on the existing dev owner, restoring the exact
	// original hash and removing only this run's factor enrollment on exit.
	var factors int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM identity.two_fas WHERE user_id=$1`, user.ID).Scan(&factors); err != nil || factors != 0 {
		t.Fatal("account flow requires the existing dev owner without a factor")
	}
	t.Cleanup(func() {
		ctx := context.Background()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Error("restore account", err)
			return
		}
		defer tx.Rollback(ctx)
		for _, query := range []string{`DELETE FROM identity.two_fa_backup_codes WHERE user_id=$1`, `DELETE FROM identity.two_fas WHERE user_id=$1`} {
			if _, err := tx.Exec(ctx, query, user.ID); err != nil {
				t.Error("restore account", err)
				return
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE identity.users SET password_hash=$2 WHERE id=$1`, user.ID, user.PasswordHash); err != nil {
			t.Error("restore account", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			t.Error("restore account", err)
			return
		}
		var restored bool
		if err := pool.QueryRow(ctx, `SELECT password_hash=$2 AND NOT EXISTS(SELECT 1 FROM identity.two_fas WHERE user_id=$1) FROM identity.users WHERE id=$1`, user.ID, user.PasswordHash).Scan(&restored); err != nil || !restored {
			t.Error("original account credentials not restored")
		}
	})
	newPassword := rand.Text()[:18]
	oldCookie := cookie
	changed := call("POST", "/api/v1/account/password/change", map[string]string{"old_password": body["password"], "password": newPassword}, cookie, csrf)
	if changed.Code != 200 || json.Unmarshal(changed.Body.Bytes(), &response) != nil {
		t.Fatal("password change", changed.Code)
	}
	cookie, csrf = changed.Result().Cookies()[0], response.Data.CSRF
	if denied := call("GET", "/api/v1/account", nil, oldCookie, ""); denied.Code != 401 {
		t.Fatal("password change left old session active", denied.Code)
	}
	if err := pokerAuthority.Check(t.Context(), pokerSession(principal)); !errors.Is(err, connectticket.ErrRevoked) {
		t.Fatal("password change left Poker session active", err)
	}
	setup := call("POST", "/api/v1/account/2fa/setup", nil, cookie, csrf)
	var enrollment struct {
		Data struct {
			Secret      string   `json:"secret"`
			URI         string   `json:"qr_code_data"`
			BackupCodes []string `json:"backup_codes"`
		} `json:"data"`
	}
	if setup.Code != 200 || json.Unmarshal(setup.Body.Bytes(), &enrollment) != nil || len(enrollment.Data.BackupCodes) != 4 || !strings.HasPrefix(enrollment.Data.URI, "otpauth://totp/") {
		t.Fatal("2FA setup", setup.Code)
	}
	code, err := totp.GenerateCode(enrollment.Data.Secret, time.Now())
	if err != nil {
		t.Fatal("generate enrollment code")
	}
	enabled := call("POST", "/api/v1/account/2fa/enable", map[string]string{"code": code}, cookie, csrf)
	if enabled.Code != 200 || json.Unmarshal(enabled.Body.Bytes(), &response) != nil {
		t.Fatal("2FA enable", enabled.Code)
	}
	cookie, csrf = enabled.Result().Cookies()[0], response.Data.CSRF
	account = call("GET", "/api/v1/account", nil, cookie, "")
	if account.Code != 200 || !strings.Contains(account.Body.String(), `"two_fa_enabled":true`) {
		t.Fatal("2FA account flag")
	}
	login = call("POST", "/api/v1/auth/password/login", map[string]string{"identifier": username, "password": newPassword}, anonymous, anonymousCSRF)
	var challenge struct {
		Data struct {
			State     string `json:"authentication_state"`
			Challenge struct {
				ID string `json:"challenge_id"`
			} `json:"challenge"`
		} `json:"data"`
	}
	if login.Code != 200 || json.Unmarshal(login.Body.Bytes(), &challenge) != nil || challenge.Data.State != "TWO_FA" {
		t.Fatal("password login requires factor", login.Code)
	}
	code, err = totp.GenerateCode(enrollment.Data.Secret, time.Now())
	if err != nil {
		t.Fatal("generate login code")
	}
	verified := call("POST", "/api/v1/auth/password/2fa", map[string]string{"challenge_id": challenge.Data.Challenge.ID, "code": code}, anonymous, anonymousCSRF)
	if verified.Code != 200 || json.Unmarshal(verified.Body.Bytes(), &response) != nil || response.Data.State != "AUTHENTICATED" {
		t.Fatal("TOTP login", verified.Code)
	}
	cookie, csrf = verified.Result().Cookies()[0], response.Data.CSRF
	disabled := call("POST", "/api/v1/account/2fa/disable", map[string]string{"code": enrollment.Data.BackupCodes[0]}, cookie, csrf)
	if disabled.Code != 200 || json.Unmarshal(disabled.Body.Bytes(), &response) != nil {
		t.Fatal("backup code disables 2FA", disabled.Code)
	}
	cookie, csrf = disabled.Result().Cookies()[0], response.Data.CSRF
	// Reuse the existing HTTP transport adapter for Discord's external exchange;
	// identity, account rows and single-use proofs still use the real dev PG/Redis.
	transport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = transport })
	profileID := user.DiscordID + "-other"
	memberStatus, memberRequests := 404, 0
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "discord.com" {
			t.Fatal("unexpected OAuth destination")
		}
		w := httptest.NewRecorder()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v10/oauth2/token":
			if r.Method != "POST" || r.ParseForm() != nil || r.Form.Get("code") != "local-proof-code" || r.Form.Get("redirect_uri") != discord.RedirectURI {
				t.Fatal("Discord exchange request")
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "local-proof-token"})
		case "/api/v10/users/@me":
			if r.Header.Get("Authorization") != "Bearer local-proof-token" {
				t.Fatal("Discord profile authorization")
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": profileID, "username": "proof-check"})
		case "/api/v10/users/@me/guilds/" + discord.GuildID + "/member":
			if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer local-proof-token" {
				t.Fatal("Discord member authorization")
			}
			memberRequests++
			w.WriteHeader(memberStatus)
			_ = json.NewEncoder(w).Encode(map[string]any{"roles": []string{}})
		default:
			t.Fatal("unexpected Discord endpoint")
		}
		return w.Result(), nil
	})
	authorizePassword := func(mode string) *httptest.ResponseRecorder {
		t.Helper()
		start := call("POST", "/api/v1/auth/discord/password-"+mode+"/start", nil, cookie, csrf)
		if start.Code != 200 || json.Unmarshal(start.Body.Bytes(), &oauth) != nil {
			t.Fatal("Discord password "+mode+" start", start.Code)
		}
		destination, err := url.Parse(oauth.Data.URL)
		if err != nil {
			t.Fatal("password authorization URL")
		}
		callback := map[string]string{"state": destination.Query().Get("state"), "code": "local-proof-code"}
		result := call("POST", "/api/v1/auth/discord/callback", callback, cookie, csrf)
		if replay := call("POST", "/api/v1/auth/discord/callback", callback, cookie, csrf); replay.Code != 403 {
			t.Fatal("password OAuth state replay", replay.Code)
		}
		return result
	}
	if wrong := authorizePassword("reset"); wrong.Code != 403 || !strings.Contains(wrong.Body.String(), "MOMIAO_AUTH_RESTART_REQUIRED") {
		t.Fatal("password proof accepted a different Discord account", wrong.Code)
	}
	if user.DiscordID == "" {
		t.Fatal("proof check requires the existing linked dev owner")
	}
	profileID = user.DiscordID
	authorizeRegistration := func() *httptest.ResponseRecorder {
		t.Helper()
		start := call("POST", "/api/v1/auth/discord/registration/start", nil, anonymous, anonymousCSRF)
		if start.Code != 200 || json.Unmarshal(start.Body.Bytes(), &oauth) != nil {
			t.Fatal("registration OAuth start", start.Code)
		}
		destination, err := url.Parse(oauth.Data.URL)
		if err != nil || destination.Query().Get("scope") != "identify guilds.members.read" {
			t.Fatal("registration membership scope")
		}
		callback := map[string]string{"state": destination.Query().Get("state"), "code": "local-proof-code"}
		result := call("POST", "/api/v1/auth/discord/callback", callback, anonymous, anonymousCSRF)
		if replay := call("POST", "/api/v1/auth/discord/callback", callback, anonymous, anonymousCSRF); replay.Code != 403 {
			t.Fatal("registration OAuth state replay", replay.Code)
		}
		return result
	}
	profileID = "900000000000000001"
	if denied := authorizeRegistration(); denied.Code != 403 || !strings.Contains(denied.Body.String(), "DISCORD_NOT_MEMBER") {
		t.Fatal("registration accepted non-member", denied.Code)
	}
	memberStatus = 200
	if denied := authorizeRegistration(); denied.Code != 403 || !strings.Contains(denied.Body.String(), "DISCORD_ROLE_REQUIRED") {
		t.Fatal("registration accepted missing role", denied.Code)
	}
	profileID = user.DiscordID
	registered := authorizeRegistration()
	if registered.Code != 200 || json.Unmarshal(registered.Body.Bytes(), &response) != nil || response.Data.State != "AUTHENTICATED" || memberRequests != 2 {
		t.Fatal("existing registration did not use local login", registered.Code)
	}
	call("POST", "/api/v1/session/logout", nil, registered.Result().Cookies()[0], response.Data.CSRF)
	var receiptCursor int64
	if err := pool.QueryRow(t.Context(), `SELECT ordinal FROM momiao_admission.receipt_counter WHERE id=1`).Scan(&receiptCursor); err != nil || receiptCursor != cursor {
		t.Fatal("denied/existing registration issued another receipt")
	}
	auth.RegistrationEnabled = false
	if denied := call("POST", "/api/v1/auth/discord/registration/start", nil, anonymous, anonymousCSRF); denied.Code != 403 || !strings.Contains(denied.Body.String(), "REGISTRATION_DISABLED") {
		t.Fatal("closed registration accepted start", denied.Code)
	}
	auth.RegistrationEnabled = true
	if denied := call("POST", "/api/v1/auth/discord/password-reset/start", nil, anonymous, anonymousCSRF); denied.Code != 401 {
		t.Fatal("anonymous password proof", denied.Code)
	}
	var proof struct {
		Data struct {
			Token     string `json:"proof"`
			Purpose   string `json:"purpose"`
			ExpiresAt int64  `json:"expires_at"`
		} `json:"data"`
	}
	approved := authorizePassword("reset")
	if approved.Code != 200 || json.Unmarshal(approved.Body.Bytes(), &proof) != nil || len(proof.Data.Token) != 43 || proof.Data.Purpose != "PASSWORD_RESET" || proof.Data.ExpiresAt <= time.Now().Unix() || proof.Data.ExpiresAt > time.Now().Add(5*time.Minute).Unix() || len(approved.Result().Cookies()) != 0 {
		t.Fatal("Discord password reset proof", approved.Code)
	}
	secondLogin := call("POST", "/api/v1/auth/password/login", map[string]string{"identifier": username, "password": newPassword}, anonymous, anonymousCSRF)
	if secondLogin.Code != 200 || json.Unmarshal(secondLogin.Body.Bytes(), &response) != nil {
		t.Fatal("second account browser", secondLogin.Code)
	}
	secondCookie := secondLogin.Result().Cookies()[0]
	resetBody := map[string]string{"password": rand.Text()[:18], "proof": proof.Data.Token}
	if crossed := call("POST", "/api/v1/account/password/reset", resetBody, secondCookie, response.Data.CSRF); crossed.Code != 403 {
		t.Fatal("cross-browser password proof", crossed.Code)
	}
	oldCookie = cookie
	reset := call("POST", "/api/v1/account/password/reset", resetBody, cookie, csrf)
	if reset.Code != 200 || json.Unmarshal(reset.Body.Bytes(), &response) != nil {
		t.Fatal("Discord password reset", reset.Code)
	}
	cookie, csrf = reset.Result().Cookies()[0], response.Data.CSRF
	if replay := call("POST", "/api/v1/account/password/reset", resetBody, cookie, csrf); replay.Code != 403 {
		t.Fatal("password proof replay", replay.Code)
	}
	for _, previous := range []*http.Cookie{oldCookie, secondCookie} {
		if denied := call("GET", "/api/v1/account", nil, previous, ""); denied.Code != 401 {
			t.Fatal("password reset left old session active", denied.Code)
		}
	}
	updated, err := auth.Store.User(t.Context(), user.ID)
	if err != nil || !identity.ValidatePasswordAndHash(resetBody["password"], updated.PasswordHash) {
		t.Fatal("reset password was not persisted")
	}
	// The same imported account exercises first-set, with its original hash
	// already protected by the cleanup above; no extra account is created.
	if _, err := pool.Exec(t.Context(), `UPDATE identity.users SET password_hash='' WHERE id=$1`, user.ID); err != nil {
		t.Fatal("prepare first password set")
	}
	approved = authorizePassword("set")
	if approved.Code != 200 || json.Unmarshal(approved.Body.Bytes(), &proof) != nil || proof.Data.Purpose != "PASSWORD_SET" {
		t.Fatal("Discord first-password proof", approved.Code)
	}
	setBody := map[string]string{"password": newPassword, "proof": proof.Data.Token}
	set := call("POST", "/api/v1/account/password/set", setBody, cookie, csrf)
	if set.Code != 200 || json.Unmarshal(set.Body.Bytes(), &response) != nil {
		t.Fatal("first password set", set.Code)
	}
	cookie, csrf = set.Result().Cookies()[0], response.Data.CSRF
	updated, err = auth.Store.User(t.Context(), user.ID)
	if err != nil || !identity.ValidatePasswordAndHash(newPassword, updated.PasswordHash) {
		t.Fatal("first password was not persisted")
	}
	logout := call("POST", "/api/v1/session/logout", nil, cookie, csrf)
	if logout.Code != 200 {
		t.Fatal("logout", logout.Code)
	}
	if denied := call("GET", "/api/v1/account", nil, cookie, ""); denied.Code != 401 {
		t.Fatal("logout left authenticated request active")
	}
}
