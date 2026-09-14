package bffauth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/nativeself"
	"github.com/cy4268/momiao/internal/session"
)

const (
	accountFlowPrefix = "chaldea:bff:account-flow:"
	accountFlowTTL    = 10 * time.Minute
	maxBrowserUserID  = int64(9007199254740991)
)

type AccountResult struct {
	ID               int64  `json:"id"`
	Username         string `json:"username"`
	HasPassword      bool   `json:"has_password"`
	DiscordLinked    bool   `json:"discord_linked"`
	TwoFactorEnabled bool   `json:"two_fa_enabled"`
}

type PasswordResult struct {
	AuthenticationState string         `json:"authentication_state"`
	CSRFToken           string         `json:"csrf_token"`
	HasPassword         bool           `json:"has_password"`
	Grant               *session.Grant `json:"-"`
}

type accountFlowRecord struct {
	Schema           int    `json:"schema_version"`
	State            string `json:"state"`
	SessionHash      string `json:"platform_session_hash"`
	UserID           string `json:"newapi_user_id"`
	NativeHash       string `json:"native_session_id_hash"`
	Purpose          string `json:"discord_purpose"`
	Created          int64  `json:"created_at_ms"`
	Expires          int64  `json:"expires_at_ms"`
	OperationHash    string `json:"operation_hash,omitempty"`
	NativeStateHash  string `json:"native_state_hash,omitempty"`
	NativeBrowserBox string `json:"native_browser_box,omitempty"`
	NativeFlowBox    string `json:"native_flow_box,omitempty"`
	ChallengeHash    string `json:"challenge_hash,omitempty"`
	ChallengeExpires int64  `json:"challenge_expires_at_ms,omitempty"`
	Attempts         int    `json:"attempts,omitempty"`
	ProofPurpose     string `json:"proof_purpose,omitempty"`
	ProofExpires     int64  `json:"proof_expires_at_ms,omitempty"`
	ProofBox         string `json:"proof_box,omitempty"`
	raw              string `json:"-"`
}

type accountBrowserValue struct {
	Cookie           string `json:"cookie"`
	AuthorizationURL string `json:"authorization_url"`
}

func accountBrowserLabel(r accountFlowRecord) string {
	return "account-browser:1:" + r.SessionHash + ":" + r.NativeHash + ":" + r.Purpose + ":" + r.NativeStateHash
}

func accountFlowLabel(r accountFlowRecord) string {
	return "account-discord-flow:1:" + r.SessionHash + ":" + r.NativeHash + ":" + r.Purpose + ":" + r.NativeStateHash + ":" + r.ChallengeHash
}

func accountProofLabel(r accountFlowRecord) string {
	return "account-proof:1:" + r.SessionHash + ":" + r.NativeHash + ":" + r.Purpose + ":" + r.NativeStateHash + ":" + r.ProofPurpose
}

func canonicalAccountUserID(value string) bool {
	id, err := strconv.ParseInt(value, 10, 64)
	return err == nil && id > 0 && id <= maxBrowserUserID && strconv.FormatInt(id, 10) == value
}

func validAccountFlow(r accountFlowRecord, hash string, now int64) bool {
	if r.Schema != 1 || r.SessionHash != hash || !digest(hash) || !canonicalAccountUserID(r.UserID) || !digest(r.NativeHash) || !validAccountDiscordPurpose(r.Purpose) || r.Created <= 0 || r.Created > now || r.Expires <= now || r.Expires-r.Created != accountFlowTTL.Milliseconds() || r.Attempts < 0 || r.Attempts > maximumAttempts {
		return false
	}
	emptyChallenge := r.NativeFlowBox == "" && r.ChallengeHash == "" && r.ChallengeExpires == 0
	emptyProof := r.ProofPurpose == "" && r.ProofExpires == 0 && r.ProofBox == ""
	switch r.State {
	case "START_PENDING":
		return digest(r.OperationHash) && r.NativeStateHash == "" && r.NativeBrowserBox == "" && emptyChallenge && emptyProof && r.Attempts == 0
	case "DISCORD_CALLBACK", "CALLBACK_PENDING":
		return digest(r.NativeStateHash) && r.NativeBrowserBox != "" && emptyChallenge && emptyProof && r.Attempts == 0 && (r.State == "DISCORD_CALLBACK" && r.OperationHash == "" || r.State == "CALLBACK_PENDING" && digest(r.OperationHash))
	case "DISCORD_TWO_FA", "TWO_FA_PENDING":
		return digest(r.NativeStateHash) && r.NativeBrowserBox != "" && r.NativeFlowBox != "" && digest(r.ChallengeHash) && r.ChallengeExpires > now && r.ChallengeExpires <= r.Expires && emptyProof && (r.State == "DISCORD_TWO_FA" && r.OperationHash == "" || r.State == "TWO_FA_PENDING" && digest(r.OperationHash))
	case "PROOF_READY", "PASSWORD_PENDING":
		purposeOK := r.Purpose == DiscordPurposeFresh && r.ProofPurpose == "PASSWORD_SET" || r.Purpose == DiscordPurposePasswordReset && r.ProofPurpose == "PASSWORD_RESET"
		return purposeOK && digest(r.NativeStateHash) && r.NativeBrowserBox != "" && emptyChallenge && r.ProofBox != "" && r.ProofExpires > now && r.ProofExpires <= r.Expires && r.Attempts == 0 && (r.State == "PROOF_READY" && r.OperationHash == "" || r.State == "PASSWORD_PENDING" && digest(r.OperationHash))
	default:
		return false
	}
}

func (s *Service) loadAccountFlow(ctx context.Context, rawSID string) (accountFlowRecord, error) {
	hash := hashText(rawSID)
	raw, now, ttl, err := s.readValue(ctx, accountFlowPrefix+hash)
	if err != nil {
		return accountFlowRecord{}, err
	}
	var record accountFlowRecord
	if !strictJSON([]byte(raw), &record, 16384) || !validAccountFlow(record, hash, now) || ttl > record.Expires-now+1000 {
		return accountFlowRecord{}, errUnauthorized
	}
	record.raw = raw
	return record, nil
}

func (s *Service) createAccountFlow(ctx context.Context, rawSID string, binding session.BFFBinding, purpose, operation string) (accountFlowRecord, error) {
	if s == nil || ctx == nil || !opaque(rawSID) || !canonicalAccountUserID(binding.UserID) || !digest(binding.NativeSessionIDHash) || !validAccountDiscordPurpose(purpose) || !opaque(operation) {
		return accountFlowRecord{}, errInput
	}
	now := time.Now().UnixMilli()
	record := accountFlowRecord{Schema: 1, State: "START_PENDING", SessionHash: hashText(rawSID), UserID: binding.UserID, NativeHash: binding.NativeSessionIDHash, Purpose: purpose, Created: now, Expires: now + accountFlowTTL.Milliseconds(), OperationHash: hashText(operation)}
	raw, _ := json.Marshal(record)
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	created, err := s.redis.SetNX(c, accountFlowPrefix+record.SessionHash, raw, accountFlowTTL).Result()
	cancel()
	if err != nil {
		return accountFlowRecord{}, errUnavailable
	}
	if !created {
		return accountFlowRecord{}, errConflict
	}
	record.raw = string(raw)
	return record, nil
}

func (s *Service) claimAccountFlow(ctx context.Context, record accountFlowRecord, want, next string) (accountFlowRecord, string, error) {
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
	if err = s.casValue(ctx, accountFlowPrefix+record.SessionHash, old, string(raw), 0); err != nil {
		return record, "", err
	}
	record.raw = string(raw)
	return record, operation, nil
}

func (s *Service) deleteAccountFlow(ctx context.Context, record accountFlowRecord) error {
	return s.casValue(ctx, accountFlowPrefix+record.SessionHash, record.raw, "", 0)
}

func (s *Service) restoreAccountFlow(ctx context.Context, record accountFlowRecord, operation, state string, attempt bool) (bool, error) {
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
		return true, s.casValue(ctx, accountFlowPrefix+record.SessionHash, old, "", 0)
	}
	raw, _ := json.Marshal(record)
	return false, s.casValue(ctx, accountFlowPrefix+record.SessionHash, old, string(raw), 0)
}

func (s *Service) openAccountBrowser(record accountFlowRecord) (accountBrowserValue, error) {
	var value accountBrowserValue
	if s.openJSON(accountBrowserLabel(record), record.NativeBrowserBox, &value) != nil || !opaque(value.Cookie) {
		return accountBrowserValue{}, errUnauthorized
	}
	state, ok := validateDiscordAuthorization(value.AuthorizationURL, s.origin, record.Purpose)
	if !ok || subtle.ConstantTimeCompare([]byte(hashText(state)), []byte(record.NativeStateHash)) != 1 {
		return accountBrowserValue{}, errUnauthorized
	}
	return value, nil
}

func (s *Service) openAccountNativeFlow(record accountFlowRecord) (string, error) {
	var value struct {
		Token string `json:"token"`
	}
	if s.openJSON(accountFlowLabel(record), record.NativeFlowBox, &value) != nil || !opaque(value.Token) {
		return "", errUnauthorized
	}
	return value.Token, nil
}

func (s *Service) openAccountProof(record accountFlowRecord) (string, error) {
	var value struct {
		Proof string `json:"proof"`
	}
	if s.openJSON(accountProofLabel(record), record.ProofBox, &value) != nil || !opaque(value.Proof) {
		return "", errUnauthorized
	}
	return value.Proof, nil
}

func (s *Service) storeAccountStart(ctx context.Context, record accountFlowRecord, operation, authorization, state, browser string) (DiscordResult, error) {
	if record.State != "START_PENDING" || record.OperationHash != hashText(operation) {
		return DiscordResult{}, errConflict
	}
	record.NativeStateHash = hashText(state)
	box, err := s.sealJSON(accountBrowserLabel(record), accountBrowserValue{Cookie: browser, AuthorizationURL: authorization})
	if err != nil {
		return DiscordResult{}, err
	}
	old := record.raw
	record.State, record.OperationHash, record.NativeBrowserBox = "DISCORD_CALLBACK", "", box
	raw, _ := json.Marshal(record)
	if err = s.casValue(ctx, accountFlowPrefix+record.SessionHash, old, string(raw), 0); err != nil {
		return DiscordResult{}, err
	}
	return DiscordResult{AuthenticationState: "DISCORD_CALLBACK", AuthorizationURL: authorization}, nil
}

func (s *Service) storeAccountChallenge(ctx context.Context, record accountFlowRecord, operation, nativeFlow, csrf string) (DiscordResult, error) {
	if record.State != "CALLBACK_PENDING" || record.OperationHash != hashText(operation) || !opaque(nativeFlow) {
		return DiscordResult{}, errConflict
	}
	challenge, err := randomSecret()
	if err != nil {
		return DiscordResult{}, err
	}
	deadline := time.Now().Add(challengeTTL)
	if limit := time.UnixMilli(record.Expires); deadline.After(limit) {
		deadline = limit
	}
	if !deadline.After(time.Now()) {
		return DiscordResult{}, errUnknown
	}
	record.ChallengeHash = hashText(challenge)
	box, err := s.sealJSON(accountFlowLabel(record), struct {
		Token string `json:"token"`
	}{nativeFlow})
	if err != nil {
		return DiscordResult{}, err
	}
	old := record.raw
	record.State, record.OperationHash, record.NativeFlowBox, record.ChallengeExpires, record.Attempts = "DISCORD_TWO_FA", "", box, deadline.UnixMilli(), 0
	raw, _ := json.Marshal(record)
	if err = s.casValue(ctx, accountFlowPrefix+record.SessionHash, old, string(raw), 0); err != nil {
		return DiscordResult{}, err
	}
	return DiscordResult{AuthenticationState: "DISCORD_TWO_FA", CSRFToken: csrf, Challenge: &LoginChallenge{ID: challenge, ExpiresAt: deadline.UTC()}}, nil
}

func proofPurpose(purpose string) string {
	if purpose == DiscordPurposeFresh {
		return "PASSWORD_SET"
	}
	return "PASSWORD_RESET"
}

func (s *Service) storeAccountProof(ctx context.Context, record accountFlowRecord, operation, proof string, expires time.Time, csrf string) (DiscordResult, error) {
	if (record.State != "CALLBACK_PENDING" && record.State != "TWO_FA_PENDING") || record.OperationHash != hashText(operation) || !opaque(proof) {
		return DiscordResult{}, errConflict
	}
	deadline := expires
	if limit := time.UnixMilli(record.Expires); deadline.After(limit) {
		deadline = limit
	}
	if !deadline.After(time.Now()) {
		return DiscordResult{}, errUnknown
	}
	deadline = deadline.UTC()
	record.ProofPurpose, record.ProofExpires = proofPurpose(record.Purpose), deadline.UnixMilli()
	box, err := s.sealJSON(accountProofLabel(record), struct {
		Proof string `json:"proof"`
	}{proof})
	if err != nil {
		return DiscordResult{}, err
	}
	old := record.raw
	record.State, record.OperationHash, record.ProofBox = "PROOF_READY", "", box
	record.NativeFlowBox, record.ChallengeHash, record.ChallengeExpires, record.Attempts = "", "", 0, 0
	raw, _ := json.Marshal(record)
	if err = s.casValue(ctx, accountFlowPrefix+record.SessionHash, old, string(raw), 0); err != nil {
		return DiscordResult{}, err
	}
	return DiscordResult{AuthenticationState: "ACCOUNT_PROOF_READY", CSRFToken: csrf, ProofPurpose: record.ProofPurpose, ExpiresAt: &deadline}, nil
}

func (s *Service) authenticatedCredential(r *http.Request, verified *session.RequestSession, private bool) (session.RequestSession, session.BFFBinding, nativeCredential, string, error) {
	var current session.RequestSession
	var err error
	if verified != nil {
		current = *verified
	} else if private {
		current, err = s.sessions.VerifyPrivateRequest(r)
	} else {
		current, err = s.sessions.VerifyRequest(r)
	}
	if err != nil {
		return session.RequestSession{}, session.BFFBinding{}, nativeCredential{}, "", mapSessionError(err)
	}
	binding, err := s.sessions.BFFBinding(r.Context(), current)
	if err != nil {
		return session.RequestSession{}, session.BFFBinding{}, nativeCredential{}, "", mapSessionError(err)
	}
	credential, err := s.ensureCredential(r.Context(), binding)
	if err != nil {
		return session.RequestSession{}, session.BFFBinding{}, nativeCredential{}, "", err
	}
	rawSID, err := cookieID(r)
	if err != nil {
		return session.RequestSession{}, session.BFFBinding{}, nativeCredential{}, "", err
	}
	return current, binding, credential, rawSID, nil
}

func (s *Service) invalidateAccountAuthority(ctx context.Context, verified session.RequestSession, binding session.BFFBinding, credential nativeCredential, browser string) error {
	s.orphanCredential(ctx, binding.NativeSessionIDHash)
	if browser != "" {
		_ = s.nativeBrowserLogout(ctx, browser)
	} else {
		_ = s.nativeLogout(ctx, credential)
	}
	_ = s.sessions.Revoke(ctx, verified)
	return errUnknown
}

func (s *Service) startAccountDiscord(r *http.Request, purpose string) (DiscordResult, error) {
	verified, binding, credential, rawSID, err := s.authenticatedCredential(r, nil, false)
	if err != nil {
		return DiscordResult{}, err
	}
	if current, loadErr := s.loadAccountFlow(r.Context(), rawSID); loadErr == nil {
		if current.UserID != binding.UserID || current.NativeHash != binding.NativeSessionIDHash || current.Purpose != purpose || current.State != "DISCORD_CALLBACK" {
			return DiscordResult{}, errConflict
		}
		browser, openErr := s.openAccountBrowser(current)
		if openErr != nil {
			return DiscordResult{}, openErr
		}
		return DiscordResult{AuthenticationState: "DISCORD_CALLBACK", CSRFToken: verified.View().CSRFToken, AuthorizationURL: browser.AuthorizationURL}, nil
	} else if !errors.Is(loadErr, errMissing) {
		return DiscordResult{}, loadErr
	}
	operation, err := randomSecret()
	if err != nil {
		return DiscordResult{}, err
	}
	record, err := s.createAccountFlow(r.Context(), rawSID, binding, purpose, operation)
	if err != nil {
		return DiscordResult{}, err
	}
	authorization, state, browser, nativeErr := s.nativeDiscordStart(r.Context(), purpose, &credential)
	ctx, finish := finalizationContext(r.Context())
	defer finish()
	if nativeErr != nil {
		_ = s.deleteAccountFlow(ctx, record)
		if browser != "" {
			return DiscordResult{}, s.invalidateAccountAuthority(ctx, verified, binding, credential, browser)
		}
		var fault Fault
		if errors.As(nativeErr, &fault) && fault.Code == "AUTH_REQUIRED" {
			_ = s.invalidateAccountAuthority(ctx, verified, binding, credential, "")
		}
		return DiscordResult{}, nativeErr
	}
	result, err := s.storeAccountStart(ctx, record, operation, authorization, state, browser)
	if err != nil {
		return DiscordResult{}, s.invalidateAccountAuthority(ctx, verified, binding, credential, browser)
	}
	result.CSRFToken = verified.View().CSRFToken
	return result, nil
}

func (s *Service) completeAccountDiscord(r *http.Request, verified session.RequestSession, input DiscordCallbackInput) (DiscordResult, error) {
	current, binding, credential, rawSID, err := s.authenticatedCredential(r, &verified, false)
	if err != nil {
		return DiscordResult{}, err
	}
	record, err := s.loadAccountFlow(r.Context(), rawSID)
	if errors.Is(err, errMissing) {
		return DiscordResult{}, errConflict
	}
	if err != nil {
		return DiscordResult{}, err
	}
	if record.UserID != binding.UserID || record.NativeHash != binding.NativeSessionIDHash || record.State != "DISCORD_CALLBACK" || subtle.ConstantTimeCompare([]byte(record.NativeStateHash), []byte(hashText(input.State))) != 1 {
		return DiscordResult{}, errConflict
	}
	pending, operation, err := s.claimAccountFlow(r.Context(), record, "DISCORD_CALLBACK", "CALLBACK_PENDING")
	if err != nil {
		return DiscordResult{}, err
	}
	browser, err := s.openAccountBrowser(pending)
	if err != nil {
		_ = s.deleteAccountFlow(r.Context(), pending)
		return DiscordResult{}, err
	}
	result, nativeErr := s.nativeDiscordResult(r.Context(), http.MethodGet, "/api/momiao/auth/discord/callback", input.query(), nil, &credential, browser.Cookie, pending.Purpose, "callback")
	ctx, finish := finalizationContext(r.Context())
	defer finish()
	if nativeErr != nil {
		_ = s.deleteAccountFlow(ctx, pending)
		if nativeUnknown(nativeErr) {
			return DiscordResult{}, s.invalidateAccountAuthority(ctx, current, binding, credential, browser.Cookie)
		}
		var fault Fault
		if errors.As(nativeErr, &fault) && fault.Code == "AUTH_REQUIRED" {
			_ = s.invalidateAccountAuthority(ctx, current, binding, credential, browser.Cookie)
		}
		return DiscordResult{}, nativeErr
	}
	if result.Challenge != nil {
		return s.storeAccountChallenge(ctx, pending, operation, result.Challenge.FlowToken, current.View().CSRFToken)
	}
	if result.Proof == "" {
		_ = s.deleteAccountFlow(ctx, pending)
		return DiscordResult{}, s.invalidateAccountAuthority(ctx, current, binding, credential, browser.Cookie)
	}
	return s.storeAccountProof(ctx, pending, operation, result.Proof, result.ProofExpires, current.View().CSRFToken)
}

func (s *Service) verifyAccountDiscordTwoFA(r *http.Request, verified session.RequestSession, challengeID, code string) (DiscordResult, error) {
	current, binding, credential, rawSID, err := s.authenticatedCredential(r, &verified, false)
	if err != nil {
		return DiscordResult{}, err
	}
	record, err := s.loadAccountFlow(r.Context(), rawSID)
	if errors.Is(err, errMissing) {
		return DiscordResult{}, errConflict
	}
	if err != nil {
		return DiscordResult{}, err
	}
	if record.UserID != binding.UserID || record.NativeHash != binding.NativeSessionIDHash || record.State != "DISCORD_TWO_FA" || record.ChallengeExpires <= time.Now().UnixMilli() || subtle.ConstantTimeCompare([]byte(record.ChallengeHash), []byte(hashText(challengeID))) != 1 {
		return DiscordResult{}, errConflict
	}
	pending, operation, err := s.claimAccountFlow(r.Context(), record, "DISCORD_TWO_FA", "TWO_FA_PENDING")
	if err != nil {
		return DiscordResult{}, err
	}
	flow, flowErr := s.openAccountNativeFlow(pending)
	browser, browserErr := s.openAccountBrowser(pending)
	if flowErr != nil || browserErr != nil {
		_ = s.deleteAccountFlow(r.Context(), pending)
		return DiscordResult{}, s.invalidateAccountAuthority(r.Context(), current, binding, credential, "")
	}
	result, nativeErr := s.nativeDiscordResult(r.Context(), http.MethodPost, "/api/momiao/auth/2fa", nil, map[string]string{"flow_token": flow, "code": code}, &credential, browser.Cookie, pending.Purpose, "2fa")
	ctx, finish := finalizationContext(r.Context())
	defer finish()
	if nativeErr != nil {
		var fault Fault
		if errors.As(nativeErr, &fault) && fault.Code == "INVALID_TWO_FA" {
			exhausted, restoreErr := s.restoreAccountFlow(ctx, pending, operation, "DISCORD_TWO_FA", true)
			if restoreErr != nil {
				return DiscordResult{}, restoreErr
			}
			if exhausted {
				_ = s.invalidateAccountAuthority(ctx, current, binding, credential, browser.Cookie)
				fault.ClearCookie = true
			}
			return DiscordResult{}, fault
		}
		if errors.As(nativeErr, &fault) && fault.Code == "RATE_LIMITED" {
			_, err = s.restoreAccountFlow(ctx, pending, operation, "DISCORD_TWO_FA", false)
			if err != nil {
				return DiscordResult{}, err
			}
			return DiscordResult{}, nativeErr
		}
		_ = s.deleteAccountFlow(ctx, pending)
		if nativeUnknown(nativeErr) {
			return DiscordResult{}, s.invalidateAccountAuthority(ctx, current, binding, credential, browser.Cookie)
		}
		if errors.As(nativeErr, &fault) && fault.Code == "AUTH_REQUIRED" {
			_ = s.invalidateAccountAuthority(ctx, current, binding, credential, browser.Cookie)
		}
		return DiscordResult{}, nativeErr
	}
	if result.Proof == "" || result.Credential != nil || result.Challenge != nil {
		_ = s.deleteAccountFlow(ctx, pending)
		return DiscordResult{}, s.invalidateAccountAuthority(ctx, current, binding, credential, browser.Cookie)
	}
	return s.storeAccountProof(ctx, pending, operation, result.Proof, result.ProofExpires, current.View().CSRFToken)
}

func (s *Service) Account(r *http.Request) (AccountResult, error) {
	if s == nil || r == nil || r.Method != http.MethodGet {
		return AccountResult{}, errInput
	}
	verified, binding, credential, _, err := s.authenticatedCredential(r, nil, true)
	if err != nil {
		return AccountResult{}, err
	}
	response, raw, err := s.momiaoCall(r.Context(), http.MethodGet, "/api/momiao/account", nil, nil, &credential, "")
	if err != nil {
		return AccountResult{}, errUnavailable
	}
	if len(response.Header.Values("Set-Cookie")) != 0 {
		return AccountResult{}, errNativeProtocol
	}
	envelope, ok := parseMomiaoEnvelope(raw)
	if !ok {
		return AccountResult{}, errNativeProtocol
	}
	if response.StatusCode != http.StatusOK || !envelope.Success {
		nativeErr := momiaoFailure(response, envelope, "account")
		var fault Fault
		if errors.As(nativeErr, &fault) && fault.Code == "AUTH_REQUIRED" {
			_ = s.invalidateAccountAuthority(r.Context(), verified, binding, credential, "")
		}
		return AccountResult{}, nativeErr
	}
	object, ok := exactObject(envelope.Data, "id", "username", "has_password", "discord_connected", "two_fa_enabled")
	if !ok || len(object) != 5 {
		return AccountResult{}, errNativeProtocol
	}
	var native struct {
		ID               int64  `json:"id"`
		Username         string `json:"username"`
		HasPassword      bool   `json:"has_password"`
		DiscordConnected bool   `json:"discord_connected"`
		TwoFactorEnabled bool   `json:"two_fa_enabled"`
	}
	if json.Unmarshal(envelope.Data, &native) != nil || native.ID <= 0 || native.ID > maxBrowserUserID || strconv.FormatInt(native.ID, 10) != binding.UserID || native.Username == "" || len(native.Username) > 128 || strings.TrimSpace(native.Username) != native.Username || !utf8.ValidString(native.Username) {
		return AccountResult{}, errNativeProtocol
	}
	return AccountResult{ID: native.ID, Username: native.Username, HasPassword: native.HasPassword, DiscordLinked: native.DiscordConnected, TwoFactorEnabled: native.TwoFactorEnabled}, nil
}

func parsePasswordCredential(data json.RawMessage, response *http.Response, current nativeCredential) (nativeCredential, bool) {
	object, ok := exactObject(data, "access_token", "token_type", "access_expires_at", "session", "has_password")
	if !ok || len(object) != 5 || len(response.Header.Values("Set-Cookie")) != 0 {
		return nativeCredential{}, false
	}
	var top struct {
		AccessToken string          `json:"access_token"`
		TokenType   string          `json:"token_type"`
		AccessUntil int64           `json:"access_expires_at"`
		Session     json.RawMessage `json:"session"`
		HasPassword bool            `json:"has_password"`
	}
	if json.Unmarshal(data, &top) != nil || top.TokenType != "Bearer" || top.AccessUntil <= time.Now().Unix() || !top.HasPassword {
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
	if json.Unmarshal(top.Session, &nativeSession) != nil || nativeSession.SID != current.SID || nativeSession.CreatedAt <= 0 || nativeSession.ExpiresAt != current.NativeExpiresAt || nativeSession.ExpiresAt <= nativeSession.CreatedAt {
		return nativeCredential{}, false
	}
	next := current
	next.AccessToken, next.AccessExpiresAt = top.AccessToken, top.AccessUntil
	probe, _ := http.NewRequest(http.MethodGet, "http://unix/api/user/self", nil)
	probe.Header.Set("Authorization", "Bearer "+next.AccessToken)
	probe.Header.Set("New-Api-User", next.UserID)
	probe.Header.Set("X-Auth-Session", next.SID)
	return next, nativeself.SessionCredential(probe)
}

func (s *Service) nativePassword(ctx context.Context, mode, password, oldPassword, proof, browser string, current nativeCredential) (nativeCredential, error) {
	body := map[string]string{"password": password}
	if mode == "change" {
		body["old_password"] = oldPassword
	} else {
		body["proof"] = proof
	}
	response, raw, err := s.momiaoCall(ctx, http.MethodPost, "/api/momiao/account/password/"+mode, nil, body, &current, browser)
	if err != nil {
		return nativeCredential{}, err
	}
	envelope, ok := parseMomiaoEnvelope(raw)
	if !ok {
		return nativeCredential{}, errUnknown
	}
	if response.StatusCode != http.StatusOK || !envelope.Success {
		return nativeCredential{}, momiaoFailure(response, envelope, "password")
	}
	next, ok := parsePasswordCredential(envelope.Data, response, current)
	if !ok {
		return nativeCredential{}, errUnknown
	}
	return next, nil
}

func (s *Service) AccountPassword(r *http.Request, mode, password, oldPassword string) (PasswordResult, error) {
	if s == nil || r == nil || r.Method != http.MethodPost || (mode != "set" && mode != "change" && mode != "reset") || password == "" || len(password) > 1024 || !utf8.ValidString(password) || (mode == "change" && (oldPassword == "" || len(oldPassword) > 1024 || !utf8.ValidString(oldPassword))) || mode != "change" && oldPassword != "" {
		return PasswordResult{}, errInput
	}
	verified, binding, _, rawSID, err := s.authenticatedCredential(r, nil, false)
	if err != nil {
		return PasswordResult{}, err
	}
	var flow accountFlowRecord
	var flowOperation, proof, browser string
	if mode != "change" {
		flow, err = s.loadAccountFlow(r.Context(), rawSID)
		if errors.Is(err, errMissing) {
			return PasswordResult{}, errConflict
		}
		if err != nil {
			return PasswordResult{}, err
		}
		wantPurpose := DiscordPurposeFresh
		if mode == "reset" {
			wantPurpose = DiscordPurposePasswordReset
		}
		if flow.UserID != binding.UserID || flow.NativeHash != binding.NativeSessionIDHash || flow.Purpose != wantPurpose || flow.State != "PROOF_READY" {
			return PasswordResult{}, errConflict
		}
		flow, flowOperation, err = s.claimAccountFlow(r.Context(), flow, "PROOF_READY", "PASSWORD_PENDING")
		if err != nil {
			return PasswordResult{}, err
		}
		proof, err = s.openAccountProof(flow)
		if err == nil {
			var value accountBrowserValue
			value, err = s.openAccountBrowser(flow)
			browser = value.Cookie
		}
		if err != nil {
			_ = s.deleteAccountFlow(r.Context(), flow)
			return PasswordResult{}, errUnknown
		}
	}
	record, current, err := s.loadCredential(r.Context(), binding.NativeSessionIDHash)
	if err != nil {
		if mode != "change" {
			_, _ = s.restoreAccountFlow(r.Context(), flow, flowOperation, "PROOF_READY", false)
		}
		return PasswordResult{}, err
	}
	if record.State != "ACTIVE" || current.UserID != binding.UserID {
		if mode != "change" {
			_, _ = s.restoreAccountFlow(r.Context(), flow, flowOperation, "PROOF_READY", false)
		}
		return PasswordResult{}, errConflict
	}
	record, lease, err := s.claimCredential(r.Context(), record, "MUTATING")
	if err != nil {
		if mode != "change" {
			_, _ = s.restoreAccountFlow(r.Context(), flow, flowOperation, "PROOF_READY", false)
		}
		return PasswordResult{}, err
	}
	next, nativeErr := s.nativePassword(r.Context(), mode, password, oldPassword, proof, browser, current)
	ctx, finish := finalizationContext(r.Context())
	defer finish()
	if nativeErr != nil {
		var fault Fault
		if errors.As(nativeErr, &fault) && fault.Code == "AUTH_REQUIRED" {
			_ = s.finishCredential(ctx, record, lease, "UNKNOWN", &current)
			if mode != "change" {
				_ = s.deleteAccountFlow(ctx, flow)
			}
			_ = s.invalidateAccountAuthority(ctx, verified, binding, current, browser)
			return PasswordResult{}, fault
		}
		if mode != "change" && errors.As(nativeErr, &fault) && fault.Code == "AUTH_CONFLICT" {
			if err = s.finishCredential(ctx, record, lease, "ACTIVE", &current); err == nil {
				err = s.deleteAccountFlow(ctx, flow)
			}
			if err == nil {
				return PasswordResult{}, fault
			}
		}
		if errors.As(nativeErr, &fault) && !fault.Unknown {
			if err = s.finishCredential(ctx, record, lease, "ACTIVE", &current); err == nil {
				if mode != "change" {
					_, err = s.restoreAccountFlow(ctx, flow, flowOperation, "PROOF_READY", false)
				}
				if err == nil {
					return PasswordResult{}, nativeErr
				}
			}
		}
		_ = s.finishCredential(ctx, record, lease, "UNKNOWN", &current)
		if mode != "change" {
			_ = s.deleteAccountFlow(ctx, flow)
		}
		return PasswordResult{}, s.invalidateAccountAuthority(ctx, verified, binding, current, browser)
	}
	if err = s.finishCredential(ctx, record, lease, "ACTIVE", &next); err != nil {
		if mode != "change" {
			_ = s.deleteAccountFlow(ctx, flow)
		}
		return PasswordResult{}, s.invalidateAccountAuthority(ctx, verified, binding, next, browser)
	}
	if mode != "change" {
		if err = s.deleteAccountFlow(ctx, flow); err != nil {
			return PasswordResult{}, s.invalidateAccountAuthority(ctx, verified, binding, next, browser)
		}
	}
	grant, err := s.sessions.AdoptNative(ctx, session.NativeCredential{UserID: next.UserID, Username: next.Username, SID: next.SID, AccessToken: next.AccessToken})
	if err != nil {
		s.orphanCredential(ctx, binding.NativeSessionIDHash)
		_ = s.nativeLogout(ctx, next)
		_ = s.sessions.Revoke(ctx, verified)
		return PasswordResult{}, Fault{Status: http.StatusServiceUnavailable, Code: "AUTH_RESULT_UNKNOWN", ClearCookie: true, Unknown: true}
	}
	_ = s.sessions.Revoke(ctx, verified)
	copy := grant
	return PasswordResult{AuthenticationState: "AUTHENTICATED", CSRFToken: grant.View().CSRFToken, HasPassword: true, Grant: &copy}, nil
}

// cancelAccountDiscord deletes the server-held OAuth/proof record on logout.
// Native session logout invalidates the linked Native browser flow.
func (s *Service) cancelAccountDiscord(ctx context.Context, r *http.Request) error {
	rawSID, err := cookieID(r)
	if err != nil {
		return err
	}
	record, err := s.loadAccountFlow(ctx, rawSID)
	if errors.Is(err, errMissing) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.deleteAccountFlow(ctx, record)
}
