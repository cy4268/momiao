package bffauth

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/session"
)

const (
	DiscordPurposeLogin         = "login"
	DiscordPurposeRegistration  = "registration"
	DiscordPurposeFresh         = "fresh"
	DiscordPurposePasswordReset = "password-reset"
	DiscordPurposeOpsFresh      = "ops-fresh"
	nativeUIAuthSurface         = "NATIVE_UI"
	accountRefWait              = 5 * time.Second
)

var (
	errAccountImportPending     = Fault{Status: http.StatusConflict, Code: "ACCOUNT_IMPORT_PENDING", ClearCookie: true}
	errAccountImportUnavailable = Fault{Status: http.StatusServiceUnavailable, Code: "AUTH_UNAVAILABLE", ClearCookie: true}
	errAuthCancelled            = Fault{Status: http.StatusBadRequest, Code: "AUTH_CANCELLED", ClearCookie: true}
	errNativeProtocol           = Fault{Status: http.StatusBadGateway, Code: "NATIVE_PROTOCOL_ERROR"}
)

type AccountRefReader interface {
	AccountRefExists(context.Context, int64) (bool, error)
}

// SetAccountRefReader is a startup-only seam. The reader is installed after
// the platform store opens and before the HTTP server starts accepting work.
func (s *Service) SetAccountRefReader(reader AccountRefReader) error {
	if s == nil || reader == nil || s.accounts != nil {
		return errInput
	}
	s.accounts = reader
	return nil
}

type DiscordCallbackInput struct {
	Code             string `json:"code,omitempty"`
	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
	State            string `json:"state"`
}

func (in DiscordCallbackInput) valid() bool {
	if !opaque(in.State) || !utf8.ValidString(in.Code) || !utf8.ValidString(in.ErrorDescription) || len(in.Code) > 4096 || len(in.ErrorDescription) > 4096 {
		return false
	}
	if in.Code != "" {
		return in.Error == "" && in.ErrorDescription == ""
	}
	return in.Error == "access_denied"
}

func (in DiscordCallbackInput) query() url.Values {
	query := url.Values{"state": {in.State}}
	if in.Code != "" {
		query.Set("code", in.Code)
		return query
	}
	query.Set("error", in.Error)
	if in.ErrorDescription != "" {
		query.Set("error_description", in.ErrorDescription)
	}
	return query
}

type DiscordResult struct {
	AuthenticationState string          `json:"authentication_state"`
	CSRFToken           string          `json:"csrf_token"`
	AuthorizationURL    string          `json:"authorization_url,omitempty"`
	Challenge           *LoginChallenge `json:"challenge,omitempty"`
	ProofPurpose        string          `json:"proof_purpose,omitempty"`
	ExpiresAt           *time.Time      `json:"expires_at,omitempty"`
	Grant               *session.Grant  `json:"-"`
}

type momiaoEnvelope struct {
	Success bool            `json:"success"`
	Code    string          `json:"code,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type momiaoAuthResult struct {
	Credential   *nativeCredential
	Challenge    *nativeChallenge
	Proof        string
	ProofExpires time.Time
}

func validDiscordPurpose(purpose string) bool {
	return validAnonymousDiscordPurpose(purpose) || validAccountDiscordPurpose(purpose) || purpose == DiscordPurposeOpsFresh
}

func validAnonymousDiscordPurpose(purpose string) bool {
	return purpose == DiscordPurposeLogin || purpose == DiscordPurposeRegistration
}

func validAccountDiscordPurpose(purpose string) bool {
	return purpose == DiscordPurposeFresh || purpose == DiscordPurposePasswordReset
}

func parseMomiaoEnvelope(raw []byte) (momiaoEnvelope, bool) {
	object, ok := exactObject(raw, "success", "code", "data")
	if !ok || object["success"] == nil {
		return momiaoEnvelope{}, false
	}
	var envelope momiaoEnvelope
	if json.Unmarshal(raw, &envelope) != nil {
		return momiaoEnvelope{}, false
	}
	if envelope.Success {
		return envelope, len(object) == 2 && object["data"] != nil && envelope.Code == ""
	}
	return envelope, len(object) == 2 && object["code"] != nil && validMomiaoCode(envelope.Code)
}

func validMomiaoCode(code string) bool {
	if code == "" || len(code) > 64 {
		return false
	}
	for _, c := range code {
		if c != '_' && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func validateDiscordAuthorization(raw, origin, purpose string) (string, bool) {
	if raw == "" || len(raw) > 8192 || !validDiscordPurpose(purpose) {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "discord.com" || u.User != nil || u.Path != "/oauth2/authorize" || u.RawPath != "" || u.Fragment != "" || u.String() != raw {
		return "", false
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(query) != 6 || u.RawQuery != query.Encode() {
		return "", false
	}
	for _, key := range []string{"client_id", "redirect_uri", "response_type", "scope", "state", "prompt"} {
		if len(query[key]) != 1 {
			return "", false
		}
	}
	clientID := query.Get("client_id")
	if len(clientID) < 1 || len(clientID) > 20 {
		return "", false
	}
	for _, c := range clientID {
		if c < '0' || c > '9' {
			return "", false
		}
	}
	scope := "identify"
	if purpose == DiscordPurposeRegistration {
		scope += " guilds.members.read"
	}
	state := query.Get("state")
	return state, query.Get("redirect_uri") == origin+"/oauth/discord" && query.Get("response_type") == "code" && query.Get("scope") == scope && opaque(state) && query.Get("prompt") == "consent"
}

func nativeBrowserCookie(response *http.Response) (string, bool) {
	if response == nil || len(response.Header.Values("Set-Cookie")) != 1 {
		return "", false
	}
	cookies := response.Cookies()
	if len(cookies) != 1 {
		return "", false
	}
	cookie := cookies[0]
	if cookie.Name != "__Host-momiao_oauth" || !opaque(cookie.Value) || cookie.Path != "/" || cookie.Domain != "" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge <= 0 || cookie.Expires.IsZero() || !cookie.Expires.After(time.Now()) || cookie.Partitioned || cookie.Quoted || len(cookie.Unparsed) != 0 {
		return "", false
	}
	return cookie.Value, true
}

func (s *Service) momiaoCall(ctx context.Context, method, path string, query url.Values, body any, credential *nativeCredential, browser string) (*http.Response, []byte, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, nil, errInput
		}
		reader = bytes.NewReader(raw)
	}
	target := "http://unix" + path
	if len(query) != 0 {
		target += "?" + query.Encode()
	}
	c, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(c, method, target, reader)
	if err != nil {
		return nil, nil, errInput
	}
	request.Host = "localhost"
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Origin", s.origin)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if credential != nil {
		request.Header.Set("Authorization", "Bearer "+credential.AccessToken)
		request.Header.Set("New-Api-User", credential.UserID)
		request.Header.Set("X-Auth-Session", credential.SID)
		request.AddCookie(&http.Cookie{Name: "new_api_refresh", Value: credential.RefreshToken})
	}
	if browser != "" {
		request.AddCookie(&http.Cookie{Name: "__Host-momiao_oauth", Value: browser})
	}
	response, err := s.native.RoundTrip(request)
	if err != nil {
		return nil, nil, errUnknown
	}
	defer response.Body.Close()
	media, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, 65537))
	if readErr != nil || len(raw) > 65536 || !utf8.Valid(raw) || c.Err() != nil {
		return response, nil, errUnknown
	}
	// Native's limiter rejects these operations before execution with an empty 429.
	discordStart := method == http.MethodPost && (path == "/api/momiao/auth/discord/login/start" || path == "/api/momiao/auth/discord/registration/start")
	if response.StatusCode == http.StatusTooManyRequests && (discordStart || path == "/api/momiao/ops/fresh/password" || path == "/api/momiao/ops/fresh/2fa") {
		return response, nil, nil
	}
	if mediaErr != nil || media != "application/json" {
		return response, nil, errUnknown
	}
	return response, raw, nil
}

func momiaoFailure(response *http.Response, envelope momiaoEnvelope, stage string) error {
	if response == nil || response.StatusCode >= 500 {
		if response != nil && stage == "account" {
			return errUnavailable
		}
		return errUnknown
	}
	switch envelope.Code {
	case "DISCORD_RATE_LIMITED", "AUTH_SESSION_ISSUANCE_LIMIT":
		return Fault{Status: http.StatusTooManyRequests, Code: "RATE_LIMITED"}
	case "DISCORD_DENIED":
		if stage == "callback" {
			return errAuthCancelled
		}
		return Fault{Status: http.StatusForbidden, Code: "DISCORD_ADMISSION_DENIED"}
	case "DISCORD_NOT_MEMBER", "DISCORD_ROLE_REQUIRED", "MOMIAO_DISCORD_UNBOUND", "MOMIAO_NOT_ELIGIBLE":
		return Fault{Status: http.StatusForbidden, Code: "DISCORD_ADMISSION_DENIED"}
	case "DISCORD_CODE_INVALID", "MOMIAO_INVALID_REQUEST":
		if stage == "password" {
			return Fault{Status: http.StatusUnprocessableEntity, Code: "PASSWORD_REJECTED"}
		}
		return Fault{Status: http.StatusBadRequest, Code: "INVALID_REQUEST"}
	case "MOMIAO_2FA_INVALID":
		return Fault{Status: http.StatusUnprocessableEntity, Code: "INVALID_TWO_FA"}
	case "MOMIAO_AUTH_RESTART_REQUIRED":
		return Fault{Status: http.StatusUnauthorized, Code: "AUTH_REQUIRED", ClearCookie: true}
	case "MOMIAO_FLOW_CONSUMED", "AUTH_SESSION_LIMIT":
		return errConflict
	default:
		return errUnknown
	}
}

func (s *Service) nativeDiscordStart(ctx context.Context, purpose string, credential *nativeCredential) (authorization, state, browser string, err error) {
	return s.nativeDiscordStartWithBody(ctx, purpose, struct{}{}, credential)
}

func (s *Service) nativeOpsDiscordStart(ctx context.Context, challengeToken string, credential *nativeCredential) (authorization, state, browser string, err error) {
	if !opaque(challengeToken) {
		return "", "", "", errInput
	}
	return s.nativeDiscordStartWithBody(ctx, DiscordPurposeOpsFresh,
		map[string]string{"challenge_token": challengeToken}, credential)
}

func (s *Service) nativeDiscordStartWithBody(ctx context.Context, purpose string, body any, credential *nativeCredential) (authorization, state, browser string, err error) {
	response, raw, callErr := s.momiaoCall(ctx, http.MethodPost, "/api/momiao/auth/discord/"+purpose+"/start", nil, body, credential, "")
	if value, ok := nativeBrowserCookie(response); ok {
		browser = value
	}
	if callErr != nil {
		return "", "", browser, callErr
	}
	if response.StatusCode == http.StatusTooManyRequests {
		return "", "", browser, Fault{Status: http.StatusTooManyRequests, Code: "RATE_LIMITED"}
	}
	envelope, ok := parseMomiaoEnvelope(raw)
	if !ok {
		return "", "", browser, errNativeProtocol
	}
	if response.StatusCode != http.StatusOK || !envelope.Success {
		return "", "", browser, momiaoFailure(response, envelope, "start")
	}
	object, ok := exactObject(envelope.Data, "authorization_url")
	if !ok || len(object) != 1 {
		return "", "", browser, errNativeProtocol
	}
	var data struct {
		AuthorizationURL string `json:"authorization_url"`
	}
	if json.Unmarshal(envelope.Data, &data) != nil {
		return "", "", browser, errNativeProtocol
	}
	state, ok = validateDiscordAuthorization(data.AuthorizationURL, s.origin, purpose)
	if !ok || browser == "" {
		return "", "", browser, errNativeProtocol
	}
	return data.AuthorizationURL, state, browser, nil
}

func (s *Service) nativeDiscordResult(ctx context.Context, method, path string, query url.Values, body any, credential *nativeCredential, browser, purpose, stage string) (momiaoAuthResult, error) {
	response, raw, err := s.momiaoCall(ctx, method, path, query, body, credential, browser)
	if err != nil {
		return momiaoAuthResult{}, err
	}
	envelope, ok := parseMomiaoEnvelope(raw)
	if !ok {
		return momiaoAuthResult{}, errUnknown
	}
	if response.StatusCode != http.StatusOK || !envelope.Success {
		return momiaoAuthResult{}, momiaoFailure(response, envelope, stage)
	}
	var challenge struct {
		Require bool   `json:"require_2fa"`
		Flow    string `json:"flow_token"`
	}
	if object, valid := exactObject(envelope.Data, "require_2fa", "flow_token"); valid && len(object) == 2 && json.Unmarshal(envelope.Data, &challenge) == nil && challenge.Require && opaque(challenge.Flow) {
		if len(response.Header.Values("Set-Cookie")) != 0 {
			return momiaoAuthResult{}, errUnknown
		}
		return momiaoAuthResult{Challenge: &nativeChallenge{FlowToken: challenge.Flow, ExpiresAt: time.Now().Add(challengeTTL)}}, nil
	}
	if validAnonymousDiscordPurpose(purpose) {
		credential, ok := parseBundle(envelope.Data, response)
		if !ok {
			return momiaoAuthResult{}, errUnknown
		}
		return momiaoAuthResult{Credential: &credential}, nil
	}
	object, ok := exactObject(envelope.Data, "proof", "expires_at")
	if !ok || len(object) != 2 || len(response.Header.Values("Set-Cookie")) != 0 {
		return momiaoAuthResult{}, errUnknown
	}
	var proof struct {
		Value   string `json:"proof"`
		Expires int64  `json:"expires_at"`
	}
	if json.Unmarshal(envelope.Data, &proof) != nil || !opaque(proof.Value) || proof.Expires <= time.Now().Unix() {
		return momiaoAuthResult{}, errUnknown
	}
	return momiaoAuthResult{Proof: proof.Value, ProofExpires: time.Unix(proof.Expires, 0).UTC()}, nil
}

func (s *Service) nativeBrowserLogout(ctx context.Context, browser string) error {
	if !opaque(browser) {
		return errInput
	}
	response, raw, err := s.momiaoCall(ctx, http.MethodPost, "/api/user/auth/logout", nil, struct{}{}, nil, browser)
	if err != nil {
		return err
	}
	envelope, ok := parseEnvelope(raw)
	if !ok || response.StatusCode != http.StatusOK || !envelope.Success {
		return errUnknown
	}
	if _, ok = refreshToken(response, "", true); !ok {
		return errUnknown
	}
	return nil
}

type discordBrowserValue struct {
	Cookie           string `json:"cookie"`
	AuthorizationURL string `json:"authorization_url"`
}

func discordBrowserLabel(r preauthRecord) string {
	label := "discord-browser:1:" + r.Hash + ":" + r.Purpose + ":" + r.NativeStateHash
	if r.AuthSurface != "" {
		label += ":" + r.AuthSurface
	}
	return label
}

func discordFlowLabel(r preauthRecord) string {
	label := "discord-flow:1:" + r.Hash + ":" + r.Purpose + ":" + r.NativeStateHash + ":" + r.ChallengeHash
	if r.AuthSurface != "" {
		label += ":" + r.AuthSurface
	}
	return label
}

func (s *Service) openDiscordBrowser(r preauthRecord) (discordBrowserValue, error) {
	var value discordBrowserValue
	if s.openJSON(discordBrowserLabel(r), r.NativeBrowserBox, &value) != nil || !opaque(value.Cookie) {
		return discordBrowserValue{}, errUnauthorized
	}
	state, ok := validateDiscordAuthorization(value.AuthorizationURL, s.origin, r.Purpose)
	if !ok || subtle.ConstantTimeCompare([]byte(hashText(state)), []byte(r.NativeStateHash)) != 1 {
		return discordBrowserValue{}, errUnauthorized
	}
	return value, nil
}

func (s *Service) openDiscordFlow(r preauthRecord) (string, error) {
	var value struct {
		Token string `json:"token"`
	}
	if s.openJSON(discordFlowLabel(r), r.NativeFlowBox, &value) != nil || !opaque(value.Token) {
		return "", errUnauthorized
	}
	return value.Token, nil
}

func (s *Service) claimDiscord(ctx context.Context, record preauthRecord, want, next string) (preauthRecord, string, error) {
	if record.State != want {
		return record, "", errConflict
	}
	operation, err := randomSecret()
	if err != nil {
		return record, "", err
	}
	old := record.raw
	record.State, record.OperationHash = next, hashText(operation)
	raw, _ := json.Marshal(record)
	if err = s.casValue(ctx, preauthPrefix+record.Hash, old, string(raw), 0); err != nil {
		return record, "", err
	}
	record.raw = string(raw)
	return record, operation, nil
}

func clearDiscord(record *preauthRecord, state string) {
	record.State, record.Purpose, record.AuthSurface, record.OperationHash, record.NativeStateHash = state, "", "", "", ""
	record.NativeBrowserBox, record.NativeFlowBox, record.ChallengeHash = "", "", ""
	record.ChallengeExpires, record.Attempts = 0, 0
}

func (s *Service) resetDiscord(ctx context.Context, record preauthRecord, operation, state string) error {
	if record.OperationHash != hashText(operation) {
		return errConflict
	}
	old := record.raw
	clearDiscord(&record, state)
	raw, _ := json.Marshal(record)
	return s.casValue(ctx, preauthPrefix+record.Hash, old, string(raw), 0)
}

func (s *Service) restoreDiscord(ctx context.Context, record preauthRecord, operation, state string, attempt bool) (bool, error) {
	if record.OperationHash != hashText(operation) {
		return false, errConflict
	}
	old := record.raw
	record.State, record.OperationHash = state, ""
	if attempt {
		record.Attempts++
	}
	exhausted := record.Attempts >= maximumAttempts
	if exhausted {
		clearDiscord(&record, "CANCELLED")
	}
	raw, _ := json.Marshal(record)
	return exhausted, s.casValue(ctx, preauthPrefix+record.Hash, old, string(raw), 0)
}

func (s *Service) storeDiscordStart(ctx context.Context, record preauthRecord, operation, state, browser, authorization string) (DiscordResult, error) {
	if record.State != "DISCORD_START_PENDING" || record.OperationHash != hashText(operation) || !validAnonymousDiscordPurpose(record.Purpose) {
		return DiscordResult{}, errConflict
	}
	record.NativeStateHash = hashText(state)
	box, err := s.sealJSON(discordBrowserLabel(record), discordBrowserValue{Cookie: browser, AuthorizationURL: authorization})
	if err != nil {
		return DiscordResult{}, err
	}
	old := record.raw
	record.State, record.OperationHash, record.NativeBrowserBox = "DISCORD_CALLBACK", "", box
	raw, _ := json.Marshal(record)
	if err = s.casValue(ctx, preauthPrefix+record.Hash, old, string(raw), 0); err != nil {
		return DiscordResult{}, err
	}
	return DiscordResult{AuthenticationState: "DISCORD_CALLBACK", CSRFToken: record.CSRF, AuthorizationURL: authorization}, nil
}

func (s *Service) storeDiscordChallenge(ctx context.Context, record preauthRecord, operation, nativeFlow string) (DiscordResult, error) {
	if record.State != "DISCORD_CALLBACK_PENDING" || record.OperationHash != hashText(operation) || !opaque(nativeFlow) {
		return DiscordResult{}, errConflict
	}
	challenge, err := randomSecret()
	if err != nil {
		return DiscordResult{}, err
	}
	now := time.Now()
	deadline := now.Add(challengeTTL)
	if expires := time.UnixMilli(record.Expires); deadline.After(expires) {
		deadline = expires
	}
	if !deadline.After(now) {
		return DiscordResult{}, errUnknown
	}
	record.ChallengeHash = hashText(challenge)
	box, err := s.sealJSON(discordFlowLabel(record), struct {
		Token string `json:"token"`
	}{nativeFlow})
	if err != nil {
		return DiscordResult{}, err
	}
	old := record.raw
	record.State, record.OperationHash, record.ChallengeExpires, record.NativeFlowBox, record.Attempts = "DISCORD_TWO_FA", "", deadline.UnixMilli(), box, 0
	raw, _ := json.Marshal(record)
	if err = s.casValue(ctx, preauthPrefix+record.Hash, old, string(raw), 0); err != nil {
		return DiscordResult{}, err
	}
	return DiscordResult{AuthenticationState: "DISCORD_TWO_FA", CSRFToken: record.CSRF, Challenge: &LoginChallenge{ID: challenge, ExpiresAt: deadline.UTC()}}, nil
}

func (s *Service) waitAccountRef(ctx context.Context, userID string) (bool, error) {
	if s.accounts == nil {
		return false, errUnavailable
	}
	id, err := strconv.ParseInt(userID, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != userID {
		return false, errUnknown
	}
	waitCtx, cancel := context.WithTimeout(ctx, accountRefWait)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		exists, readErr := s.accounts.AccountRefExists(waitCtx, id)
		if readErr != nil {
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return false, nil
			}
			return false, readErr
		}
		if exists {
			return true, nil
		}
		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return false, nil
			}
			return false, waitCtx.Err()
		case <-ticker.C:
		}
	}
}

func discordFromLogin(result LoginResult) DiscordResult {
	return DiscordResult{AuthenticationState: result.AuthenticationState, CSRFToken: result.CSRFToken, Challenge: result.Challenge, Grant: result.Grant}
}

func (s *Service) finishDiscordLogin(ctx context.Context, record preauthRecord, operation string, credential nativeCredential, browser string) (DiscordResult, error) {
	if record.Purpose == DiscordPurposeRegistration {
		exists, err := s.waitAccountRef(ctx, credential.UserID)
		if err != nil || !exists {
			_ = s.resetDiscord(ctx, record, operation, "CANCELLED")
			_ = s.nativeBrowserLogout(ctx, browser)
			if err != nil {
				return DiscordResult{}, errAccountImportUnavailable
			}
			return DiscordResult{}, errAccountImportPending
		}
	}
	result, err := s.finishLogin(ctx, record, operation, credential)
	if err != nil {
		_ = s.nativeBrowserLogout(ctx, browser)
		return DiscordResult{}, err
	}
	return discordFromLogin(result), nil
}

func (s *Service) StartDiscord(r *http.Request, purpose string) (DiscordResult, error) {
	if s == nil || r == nil || r.Method != http.MethodPost || !validDiscordPurpose(purpose) {
		return DiscordResult{}, errInput
	}
	if validAccountDiscordPurpose(purpose) {
		return s.startAccountDiscord(r, purpose)
	}
	return s.startAnonymousDiscord(r, purpose, "")
}

// StartNativeUIDiscord starts the same Native admission ceremony as StartDiscord
// while binding browser callback rendering to this one anonymous OAuth state.
func (s *Service) StartNativeUIDiscord(r *http.Request, purpose string) (DiscordResult, error) {
	if s == nil || r == nil || r.Method != http.MethodPost || !validAnonymousDiscordPurpose(purpose) {
		return DiscordResult{}, errInput
	}
	return s.startAnonymousDiscord(r, purpose, nativeUIAuthSurface)
}

func (s *Service) startAnonymousDiscord(r *http.Request, purpose, surface string) (DiscordResult, error) {
	record, _, err := s.verifyPreauth(r)
	if err != nil {
		return DiscordResult{}, err
	}
	if record.State == "DISCORD_CALLBACK" && record.Purpose == purpose && record.AuthSurface == surface {
		browser, openErr := s.openDiscordBrowser(record)
		if openErr != nil {
			return DiscordResult{}, openErr
		}
		return DiscordResult{AuthenticationState: record.State, CSRFToken: record.CSRF, AuthorizationURL: browser.AuthorizationURL}, nil
	}
	if record.State != "ANONYMOUS" {
		return DiscordResult{}, errConflict
	}
	record.Purpose, record.AuthSurface = purpose, surface
	pending, operation, err := s.claimDiscord(r.Context(), record, "ANONYMOUS", "DISCORD_START_PENDING")
	if err != nil {
		return DiscordResult{}, err
	}
	authorization, state, browser, nativeErr := s.nativeDiscordStart(r.Context(), purpose, nil)
	ctx, finish := finalizationContext(r.Context())
	defer finish()
	if nativeErr != nil {
		_ = s.resetDiscord(ctx, pending, operation, "ANONYMOUS")
		if browser != "" {
			_ = s.nativeBrowserLogout(ctx, browser)
		}
		return DiscordResult{}, nativeErr
	}
	result, err := s.storeDiscordStart(ctx, pending, operation, state, browser, authorization)
	if err != nil {
		_ = s.nativeBrowserLogout(ctx, browser)
	}
	return result, err
}

func nativeUICallbackOwned(record preauthRecord, input DiscordCallbackInput) bool {
	return input.valid() && record.State == "DISCORD_CALLBACK" &&
		validAnonymousDiscordPurpose(record.Purpose) && record.AuthSurface == nativeUIAuthSurface &&
		subtle.ConstantTimeCompare([]byte(record.NativeStateHash), []byte(hashText(input.State))) == 1
}

// OwnsNativeUICallback identifies only the server-recorded Native UI ceremony
// for this exact OAuth state. Account and operations callbacks keep using the
// Platform callback surface.
func (s *Service) OwnsNativeUICallback(r *http.Request) bool {
	if s == nil || r == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) || r.URL.Path != "/oauth/discord" {
		return false
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || r.URL.RawQuery == "" || r.URL.ForceQuery {
		return false
	}
	for key, values := range query {
		if len(values) != 1 || key != "state" && key != "code" && key != "error" && key != "error_description" {
			return false
		}
	}
	input := DiscordCallbackInput{State: query.Get("state"), Code: query.Get("code"), Error: query.Get("error"), ErrorDescription: query.Get("error_description")}
	_, hasCode := query["code"]
	_, hasError := query["error"]
	if hasCode == hasError || hasCode && len(query) != 2 || hasError && (len(query) < 2 || len(query) > 3) {
		return false
	}
	sid, err := cookieID(r)
	if err != nil {
		return false
	}
	record, err := s.loadPreauth(r.Context(), sid)
	return err == nil && nativeUICallbackOwned(record, input)
}

func (s *Service) CompleteDiscord(r *http.Request, input DiscordCallbackInput) (DiscordResult, error) {
	if s == nil || r == nil || r.Method != http.MethodPost || !input.valid() {
		return DiscordResult{}, errInput
	}
	if verified, err := s.sessions.VerifyRequest(r); err == nil {
		return s.completeAccountDiscord(r, verified, input)
	} else if !isSessionUnauthorized(err) {
		return DiscordResult{}, mapSessionError(err)
	}
	record, _, err := s.verifyPreauth(r)
	if err != nil {
		return DiscordResult{}, err
	}
	if record.State != "DISCORD_CALLBACK" || !validAnonymousDiscordPurpose(record.Purpose) || subtle.ConstantTimeCompare([]byte(record.NativeStateHash), []byte(hashText(input.State))) != 1 {
		return DiscordResult{}, errConflict
	}
	pending, operation, err := s.claimDiscord(r.Context(), record, "DISCORD_CALLBACK", "DISCORD_CALLBACK_PENDING")
	if err != nil {
		return DiscordResult{}, err
	}
	browser, err := s.openDiscordBrowser(pending)
	if err != nil {
		_ = s.resetDiscord(r.Context(), pending, operation, "CANCELLED")
		return DiscordResult{}, err
	}
	result, nativeErr := s.nativeDiscordResult(r.Context(), http.MethodGet, "/api/momiao/auth/discord/callback", input.query(), nil, nil, browser.Cookie, pending.Purpose, "callback")
	ctx, finish := finalizationContext(r.Context())
	defer finish()
	if nativeErr != nil {
		state := "ANONYMOUS"
		if nativeUnknown(nativeErr) || errorsIsAuthCancelled(nativeErr) {
			state = "UNKNOWN"
			if errorsIsAuthCancelled(nativeErr) {
				state = "CANCELLED"
			}
		}
		if err = s.resetDiscord(ctx, pending, operation, state); err != nil {
			return DiscordResult{}, err
		}
		_ = s.nativeBrowserLogout(ctx, browser.Cookie)
		return DiscordResult{}, nativeErr
	}
	if result.Challenge != nil {
		return s.storeDiscordChallenge(ctx, pending, operation, result.Challenge.FlowToken)
	}
	if result.Credential == nil {
		_ = s.resetDiscord(ctx, pending, operation, "UNKNOWN")
		_ = s.nativeBrowserLogout(ctx, browser.Cookie)
		return DiscordResult{}, errUnknown
	}
	return s.finishDiscordLogin(ctx, pending, operation, *result.Credential, browser.Cookie)
}

func errorsIsAuthCancelled(err error) bool {
	var fault Fault
	return errors.As(err, &fault) && fault.Code == errAuthCancelled.Code
}

func (s *Service) VerifyDiscordTwoFA(r *http.Request, challengeID, code string) (DiscordResult, error) {
	if s == nil || r == nil || r.Method != http.MethodPost || !opaque(challengeID) || code == "" || len(code) > 128 || strings.TrimSpace(code) != code || !utf8.ValidString(code) {
		return DiscordResult{}, errInput
	}
	if verified, err := s.sessions.VerifyRequest(r); err == nil {
		return s.verifyAccountDiscordTwoFA(r, verified, challengeID, code)
	} else if !isSessionUnauthorized(err) {
		return DiscordResult{}, mapSessionError(err)
	}
	record, _, err := s.verifyPreauth(r)
	if err != nil {
		return DiscordResult{}, err
	}
	if record.State != "DISCORD_TWO_FA" || record.ChallengeExpires <= time.Now().UnixMilli() || subtle.ConstantTimeCompare([]byte(record.ChallengeHash), []byte(hashText(challengeID))) != 1 {
		return DiscordResult{}, errConflict
	}
	pending, operation, err := s.claimDiscord(r.Context(), record, "DISCORD_TWO_FA", "DISCORD_TWO_FA_PENDING")
	if err != nil {
		return DiscordResult{}, err
	}
	flow, flowErr := s.openDiscordFlow(pending)
	browser, browserErr := s.openDiscordBrowser(pending)
	if flowErr != nil || browserErr != nil {
		_ = s.resetDiscord(r.Context(), pending, operation, "UNKNOWN")
		return DiscordResult{}, errUnknown
	}
	result, nativeErr := s.nativeDiscordResult(r.Context(), http.MethodPost, "/api/momiao/auth/2fa", nil, map[string]string{"flow_token": flow, "code": code}, nil, browser.Cookie, pending.Purpose, "2fa")
	ctx, finish := finalizationContext(r.Context())
	defer finish()
	if nativeErr != nil {
		var fault Fault
		if errors.As(nativeErr, &fault) && fault.Code == "INVALID_TWO_FA" {
			exhausted, restoreErr := s.restoreDiscord(ctx, pending, operation, "DISCORD_TWO_FA", true)
			if restoreErr != nil {
				return DiscordResult{}, restoreErr
			}
			if exhausted {
				_ = s.nativeBrowserLogout(ctx, browser.Cookie)
				fault.ClearCookie = true
			}
			return DiscordResult{}, fault
		}
		if errors.As(nativeErr, &fault) && fault.Code == "RATE_LIMITED" {
			_, err = s.restoreDiscord(ctx, pending, operation, "DISCORD_TWO_FA", false)
			if err != nil {
				return DiscordResult{}, err
			}
			return DiscordResult{}, nativeErr
		}
		state := "ANONYMOUS"
		if nativeUnknown(nativeErr) {
			state = "UNKNOWN"
		}
		if err = s.resetDiscord(ctx, pending, operation, state); err != nil {
			return DiscordResult{}, err
		}
		_ = s.nativeBrowserLogout(ctx, browser.Cookie)
		return DiscordResult{}, nativeErr
	}
	if result.Credential == nil || result.Challenge != nil {
		_ = s.resetDiscord(ctx, pending, operation, "UNKNOWN")
		_ = s.nativeBrowserLogout(ctx, browser.Cookie)
		return DiscordResult{}, errUnknown
	}
	return s.finishDiscordLogin(ctx, pending, operation, *result.Credential, browser.Cookie)
}

// cleanupPreauthDiscord is called by logout before cancelPreauth clears the
// sealed Native browser handle. It performs one bounded cleanup and never
// retries an uncertain Native result.
func (s *Service) cleanupPreauthDiscord(ctx context.Context, record preauthRecord) string {
	if record.NativeBrowserBox == "" {
		return "NOT_ISSUED"
	}
	browser, err := s.openDiscordBrowser(record)
	if err != nil || s.nativeBrowserLogout(ctx, browser.Cookie) != nil {
		return "UNKNOWN"
	}
	return "REVOKED"
}
