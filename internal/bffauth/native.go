package bffauth

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/nativeself"
)

type nativeChallenge struct {
	FlowToken string
	ExpiresAt time.Time
}

type nativeLoginResult struct {
	Credential *nativeCredential
	Challenge  *nativeChallenge
}

type PasswordProvider struct {
	State             string `json:"state"`
	TurnstileRequired bool   `json:"turnstile_required"`
	TurnstileSiteKey  string `json:"turnstile_site_key,omitempty"`
}

type nativeEnvelope struct {
	Success bool            `json:"success"`
	Code    string          `json:"code,omitempty"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func exactObject(raw []byte, allowed ...string) (map[string]json.RawMessage, bool) {
	var object map[string]json.RawMessage
	if !strictJSON(raw, &object, 65536) || object == nil {
		return nil, false
	}
	set := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		set[key] = true
	}
	for key := range object {
		if !set[key] {
			return nil, false
		}
	}
	return object, true
}

func (s *Service) nativeCall(ctx context.Context, method, path string, body any, credential *nativeCredential) (*http.Response, []byte, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, nil, errInput
		}
		reader = bytes.NewReader(raw)
	}
	c, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(c, method, "http://unix"+path, reader)
	if err != nil {
		return nil, nil, errInput
	}
	req.Host = "localhost"
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if credential != nil {
		req.Header.Set("Origin", s.origin)
		req.Header.Set("Authorization", "Bearer "+credential.AccessToken)
		req.Header.Set("New-Api-User", credential.UserID)
		req.Header.Set("X-Auth-Session", credential.SID)
		req.AddCookie(&http.Cookie{Name: "new_api_refresh", Value: credential.RefreshToken})
	}
	response, err := s.native.RoundTrip(req)
	if err != nil {
		return nil, nil, errUnknown
	}
	defer response.Body.Close()
	media, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, 65537))
	if readErr != nil || len(raw) > 65536 || !utf8.Valid(raw) || mediaErr != nil || media != "application/json" || c.Err() != nil {
		return response, nil, errUnknown
	}
	return response, raw, nil
}

func parseEnvelope(raw []byte) (nativeEnvelope, bool) {
	object, ok := exactObject(raw, "success", "code", "message", "data")
	if !ok || object["success"] == nil || object["message"] == nil {
		return nativeEnvelope{}, false
	}
	var envelope nativeEnvelope
	if json.Unmarshal(raw, &envelope) != nil {
		return nativeEnvelope{}, false
	}
	// Native refresh-only logout succeeds without data. Each data-bearing
	// endpoint validates its own payload after this shared envelope check.
	return envelope, true
}

func nativeRejected(kind string, status int) error {
	if status == http.StatusTooManyRequests {
		return Fault{Status: 429, Code: "AUTH_RATE_LIMITED"}
	}
	if status >= 500 {
		return errUnknown
	}
	code := "AUTH_INVALID_CREDENTIALS"
	if kind == "2fa" {
		code = "AUTH_2FA_INVALID"
	}
	return Fault{Status: 401, Code: code}
}

func refreshToken(response *http.Response, sid string, clearing bool) (string, bool) {
	if response == nil {
		return "", false
	}
	cookies := response.Cookies()
	if len(cookies) != len(response.Header.Values("Set-Cookie")) || len(cookies) < 1 || len(cookies) > 2 || !clearing && len(cookies) != 1 {
		return "", false
	}
	var refresh *http.Cookie
	for _, cookie := range cookies {
		switch cookie.Name {
		case "new_api_refresh":
			if refresh != nil {
				return "", false
			}
			refresh = cookie
		case "__Host-momiao_oauth":
			// The Native admission guard adds this tombstone on logout only.
			if !clearing || cookie.Value != "" || cookie.MaxAge >= 0 || cookie.Path != "/" || cookie.Domain != "" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
				return "", false
			}
		default:
			return "", false
		}
	}
	if refresh == nil || refresh.Path != "/api/user/auth" || !refresh.HttpOnly || !refresh.Secure || refresh.SameSite != http.SameSiteStrictMode {
		return "", false
	}
	if clearing {
		return "", refresh.Value == "" && refresh.MaxAge < 0
	}
	value := refresh.Value
	return value, strings.HasPrefix(value, sid+".") && len(value) <= 1024
}

func parseBundle(data json.RawMessage, response *http.Response) (nativeCredential, bool) {
	object, ok := exactObject(data, "access_token", "token_type", "access_expires_at", "session", "user")
	if !ok || len(object) != 5 {
		return nativeCredential{}, false
	}
	var top struct {
		AccessToken string          `json:"access_token"`
		TokenType   string          `json:"token_type"`
		AccessUntil int64           `json:"access_expires_at"`
		Session     json.RawMessage `json:"session"`
		User        json.RawMessage `json:"user"`
	}
	if json.Unmarshal(data, &top) != nil || top.TokenType != "Bearer" || top.AccessUntil <= time.Now().Unix() {
		return nativeCredential{}, false
	}
	sessionObject, ok := exactObject(top.Session, "sid", "current", "login_method", "ip", "user_agent", "created_at", "last_active_at", "expires_at")
	if !ok || len(sessionObject) != 8 {
		return nativeCredential{}, false
	}
	var nativeSession struct {
		SID       string `json:"sid"`
		CreatedAt int64  `json:"created_at"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if json.Unmarshal(top.Session, &nativeSession) != nil || nativeSession.CreatedAt <= 0 || nativeSession.ExpiresAt <= time.Now().Unix() || nativeSession.ExpiresAt <= nativeSession.CreatedAt {
		return nativeCredential{}, false
	}
	_, ok = exactObject(top.User, "id", "username", "display_name", "role", "status", "email", "github_id", "discord_id", "oidc_id", "wechat_id", "telegram_id", "group", "quota", "used_quota", "request_count", "aff_code", "aff_count", "aff_quota", "aff_history_quota", "inviter_id", "linux_do_id", "setting", "stripe_customer", "sidebar_modules", "permissions")
	if !ok {
		return nativeCredential{}, false
	}
	var user struct {
		ID          int64  `json:"id"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Status      int    `json:"status"`
	}
	if json.Unmarshal(top.User, &user) != nil || user.ID <= 0 || user.Username == "" || user.Status != 1 {
		return nativeCredential{}, false
	}
	refresh, ok := refreshToken(response, nativeSession.SID, false)
	if !ok {
		return nativeCredential{}, false
	}
	credential := nativeCredential{UserID: strconv.FormatInt(user.ID, 10), Username: user.Username, DisplayName: user.DisplayName, SID: nativeSession.SID, AccessToken: top.AccessToken, RefreshToken: refresh, AccessExpiresAt: top.AccessUntil, NativeExpiresAt: nativeSession.ExpiresAt}
	probe, _ := http.NewRequest(http.MethodGet, "http://unix/api/user/self", nil)
	probe.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	probe.Header.Set("New-Api-User", credential.UserID)
	probe.Header.Set("X-Auth-Session", credential.SID)
	return credential, nativeself.SessionCredential(probe)
}

func (s *Service) nativeLogin(ctx context.Context, path string, body map[string]string, kind string) (nativeLoginResult, error) {
	response, raw, err := s.nativeCall(ctx, http.MethodPost, path, body, nil)
	if err != nil {
		return nativeLoginResult{}, err
	}
	envelope, ok := parseEnvelope(raw)
	if !ok {
		return nativeLoginResult{}, errUnknown
	}
	if response.StatusCode != http.StatusOK || !envelope.Success {
		return nativeLoginResult{}, nativeRejected(kind, response.StatusCode)
	}
	var challenge struct {
		Require bool   `json:"require_2fa"`
		Flow    string `json:"flow_token"`
		Expires int64  `json:"expires_at"`
	}
	if object, valid := exactObject(envelope.Data, "require_2fa", "flow_token", "expires_at"); valid && len(object) == 3 && json.Unmarshal(envelope.Data, &challenge) == nil && challenge.Require && challenge.Flow != "" && challenge.Expires > time.Now().Unix() {
		if len(response.Header.Values("Set-Cookie")) != 0 {
			return nativeLoginResult{}, errUnknown
		}
		return nativeLoginResult{Challenge: &nativeChallenge{FlowToken: challenge.Flow, ExpiresAt: time.Unix(challenge.Expires, 0).UTC()}}, nil
	}
	credential, ok := parseBundle(envelope.Data, response)
	if !ok {
		return nativeLoginResult{}, errUnknown
	}
	return nativeLoginResult{Credential: &credential}, nil
}

func (s *Service) password(ctx context.Context, identifier, password, turnstile string) (nativeLoginResult, error) {
	path := "/api/user/login"
	if turnstile != "" {
		path += "?turnstile=" + url.QueryEscape(turnstile)
	}
	return s.nativeLogin(ctx, path, map[string]string{"username": identifier, "password": password}, "password")
}

func (s *Service) twoFA(ctx context.Context, flow, code string) (nativeLoginResult, error) {
	return s.nativeLogin(ctx, "/api/user/login/2fa", map[string]string{"flow_token": flow, "code": code}, "2fa")
}

func (s *Service) refresh(ctx context.Context, current nativeCredential) (nativeCredential, error) {
	response, raw, err := s.nativeCall(ctx, http.MethodPost, "/api/user/auth/refresh", struct{}{}, &current)
	if err != nil {
		return nativeCredential{}, err
	}
	envelope, ok := parseEnvelope(raw)
	if !ok || response.StatusCode != http.StatusOK || !envelope.Success {
		return nativeCredential{}, errUnknown
	}
	next, ok := parseBundle(envelope.Data, response)
	if !ok || next.UserID != current.UserID || next.SID != current.SID || next.NativeExpiresAt != current.NativeExpiresAt {
		return nativeCredential{}, errUnknown
	}
	return next, nil
}

func (s *Service) nativeLogout(ctx context.Context, current nativeCredential) error {
	response, raw, err := s.nativeCall(ctx, http.MethodPost, "/api/user/auth/logout", struct{}{}, &current)
	if err != nil {
		return err
	}
	envelope, ok := parseEnvelope(raw)
	if !ok || response.StatusCode != http.StatusOK || !envelope.Success {
		return errUnknown
	}
	if _, ok = refreshToken(response, current.SID, true); !ok {
		return errUnknown
	}
	return nil
}

func (s *Service) passwordProvider(ctx context.Context) PasswordProvider {
	response, raw, err := s.nativeCall(ctx, http.MethodGet, "/api/status", nil, nil)
	if err != nil || response.StatusCode != http.StatusOK || len(response.Header.Values("Set-Cookie")) != 0 {
		return PasswordProvider{State: "UNAVAILABLE"}
	}
	envelope, ok := parseEnvelope(raw)
	if !ok || !envelope.Success {
		return PasswordProvider{State: "UNAVAILABLE"}
	}
	_, ok = exactObject(envelope.Data, "version", "start_time", "email_verification", "github_oauth", "github_client_id", "discord_oauth", "discord_client_id", "linuxdo_oauth", "linuxdo_client_id", "linuxdo_minimum_trust_level", "telegram_oauth", "telegram_bot_name", "theme", "system_name", "logo", "footer_html", "wechat_qrcode", "wechat_login", "server_address", "turnstile_check", "turnstile_site_key", "docs_link", "quota_per_unit", "display_in_currency", "quota_display_type", "custom_currency_symbol", "custom_currency_exchange_rate", "enable_batch_update", "enable_drawing", "enable_task", "enable_data_export", "data_export_default_time", "default_collapse_sidebar", "mj_notify_enabled", "chats", "demo_site_enabled", "self_use_mode_enabled", "register_enabled", "password_login_enabled", "password_register_enabled", "default_use_auto_group", "usd_exchange_rate", "price", "stripe_unit_price", "api_info_enabled", "uptime_kuma_enabled", "announcements_enabled", "faq_enabled", "HeaderNavModules", "SidebarModulesAdmin", "oidc_enabled", "oidc_client_id", "oidc_authorization_endpoint", "oidc_display_name", "passkey_login", "passkey_display_name", "passkey_rp_id", "passkey_origins", "passkey_allow_insecure", "passkey_user_verification", "passkey_attachment", "setup", "user_agreement_enabled", "privacy_policy_enabled", "checkin_enabled", "api_info", "announcements", "faq", "custom_oauth_providers")
	if !ok {
		return PasswordProvider{State: "UNAVAILABLE"}
	}
	var values struct {
		Enabled   bool   `json:"password_login_enabled"`
		Turnstile bool   `json:"turnstile_check"`
		SiteKey   string `json:"turnstile_site_key"`
		Setup     bool   `json:"setup"`
	}
	if json.Unmarshal(envelope.Data, &values) != nil || !values.Setup || values.Turnstile && values.SiteKey == "" {
		return PasswordProvider{State: "UNAVAILABLE"}
	}
	state := "DISABLED"
	if values.Enabled {
		state = "AVAILABLE"
	}
	return PasswordProvider{State: state, TurnstileRequired: values.Turnstile, TurnstileSiteKey: values.SiteKey}
}

func nativeHash(c nativeCredential) string { return hashText(c.SID) }

func nativeCredentialForAdoption(c nativeCredential) (string, string, string, string) {
	return c.UserID, c.Username, c.SID, c.AccessToken
}
