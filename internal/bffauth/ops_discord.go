package bffauth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/session"
)

const (
	opsDiscordPrefix = "chaldea:bff:ops-discord:"
	opsDiscordTTL    = 10 * time.Minute
)

var opsDiscordUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type OpsDiscordStartResult struct {
	AuthorizationURL string `json:"authorization_url"`
}

type opsDiscordRecord struct {
	Schema           int    `json:"schema_version"`
	State            string `json:"state"`
	UserID           string `json:"newapi_user_id"`
	NativeHash       string `json:"native_session_id_hash"`
	OperationID      string `json:"operation_id"`
	ChallengeHash    string `json:"challenge_hash"`
	Created          int64  `json:"created_at_ms"`
	Expires          int64  `json:"expires_at_ms"`
	LeaseHash        string `json:"lease_hash,omitempty"`
	NativeStateHash  string `json:"native_state_hash,omitempty"`
	NativeBrowserBox string `json:"native_browser_box,omitempty"`
	FlowHash         string `json:"flow_hash,omitempty"`
	FlowExpires      int64  `json:"flow_expires_at_ms,omitempty"`
	NativeFlowBox    string `json:"native_flow_box,omitempty"`
	ProofExpires     int64  `json:"proof_expires_at_ms,omitempty"`
	ProofBox         string `json:"proof_box,omitempty"`
	Attempts         int    `json:"attempts,omitempty"`
	raw              string `json:"-"`
}

type opsDiscordBrowserValue struct {
	Cookie           string `json:"cookie"`
	AuthorizationURL string `json:"authorization_url"`
}

type opsDiscordFlowValue struct {
	NativeToken string `json:"native_token"`
	ClientToken string `json:"client_token"`
}

func opsDiscordKey(nativeHash, operationID string) string {
	return opsDiscordPrefix + nativeHash + ":" + operationID
}

func opsDiscordBrowserLabel(record opsDiscordRecord) string {
	return "ops-discord-browser:1:" + record.NativeHash + ":" + record.OperationID + ":" +
		record.ChallengeHash + ":" + record.NativeStateHash
}

func opsDiscordFlowLabel(record opsDiscordRecord) string {
	return "ops-discord-flow:1:" + record.NativeHash + ":" + record.OperationID + ":" +
		record.ChallengeHash + ":" + record.NativeStateHash + ":" + record.FlowHash
}

func opsDiscordProofLabel(record opsDiscordRecord) string {
	return "ops-discord-proof:1:" + record.NativeHash + ":" + record.OperationID + ":" +
		record.ChallengeHash + ":" + record.NativeStateHash + ":" + record.FlowHash
}

func validOpsDiscordRecord(record opsDiscordRecord, binding session.BFFBinding, operationID string, now int64) bool {
	if record.Schema != 1 || record.UserID != binding.UserID || record.NativeHash != binding.NativeSessionIDHash ||
		record.OperationID != operationID || !canonicalAccountUserID(record.UserID) || !digest(record.NativeHash) ||
		!opsDiscordUUID.MatchString(record.OperationID) || !digest(record.ChallengeHash) || record.Created <= 0 ||
		record.Created > now || record.Expires <= now || record.Expires-record.Created != opsDiscordTTL.Milliseconds() ||
		record.Attempts < 0 || record.Attempts > maximumAttempts {
		return false
	}
	emptyBrowser := record.NativeStateHash == "" && record.NativeBrowserBox == ""
	emptyFlow := record.FlowHash == "" && record.FlowExpires == 0 && record.NativeFlowBox == ""
	emptyProof := record.ProofExpires == 0 && record.ProofBox == ""
	switch record.State {
	case "START_PENDING":
		return digest(record.LeaseHash) && emptyBrowser && emptyFlow && emptyProof && record.Attempts == 0
	case "DISCORD_CALLBACK", "CALLBACK_PENDING":
		return digest(record.NativeStateHash) && record.NativeBrowserBox != "" && emptyFlow && emptyProof &&
			record.Attempts == 0 && (record.State == "DISCORD_CALLBACK" && record.LeaseHash == "" ||
			record.State == "CALLBACK_PENDING" && digest(record.LeaseHash))
	case "DISCORD_TWO_FA", "TWO_FA_PENDING":
		return digest(record.NativeStateHash) && record.NativeBrowserBox != "" && digest(record.FlowHash) &&
			record.FlowExpires > now && record.FlowExpires <= record.Expires && record.NativeFlowBox != "" && emptyProof &&
			(record.State == "DISCORD_TWO_FA" && record.LeaseHash == "" ||
				record.State == "TWO_FA_PENDING" && digest(record.LeaseHash))
	case "PROOF_READY":
		flowShape := emptyFlow || digest(record.FlowHash) && record.FlowExpires == record.ProofExpires && record.NativeFlowBox == ""
		return record.LeaseHash == "" && digest(record.NativeStateHash) && record.NativeBrowserBox != "" &&
			flowShape && record.ProofExpires > now && record.ProofExpires <= record.Expires && record.ProofBox != "" &&
			record.Attempts == 0
	default:
		return false
	}
}

func (s *Service) opsDiscordAuthority(ctx context.Context, verified session.RequestSession, operationID, challengeID string) (session.BFFBinding, nativeCredential, error) {
	if s == nil || s.sessions == nil || s.redis == nil || ctx == nil || !opsDiscordUUID.MatchString(operationID) || !opaque(challengeID) {
		return session.BFFBinding{}, nativeCredential{}, errInput
	}
	binding, err := s.sessions.BFFBinding(ctx, verified)
	if err != nil {
		return session.BFFBinding{}, nativeCredential{}, mapSessionError(err)
	}
	if err = s.sessions.CheckOpsChallenge(ctx, verified, operationID, challengeID); err != nil {
		return session.BFFBinding{}, nativeCredential{}, mapSessionError(err)
	}
	credential, err := s.ensureCredential(ctx, binding)
	if err != nil {
		return session.BFFBinding{}, nativeCredential{}, err
	}
	current, err := s.sessions.BFFBinding(ctx, verified)
	if err != nil {
		return session.BFFBinding{}, nativeCredential{}, mapSessionError(err)
	}
	if current != binding {
		return session.BFFBinding{}, nativeCredential{}, errUnauthorized
	}
	if err = s.sessions.CheckOpsChallenge(ctx, verified, operationID, challengeID); err != nil {
		return session.BFFBinding{}, nativeCredential{}, mapSessionError(err)
	}
	return binding, credential, nil
}

func (s *Service) loadOpsDiscord(ctx context.Context, binding session.BFFBinding, operationID string) (opsDiscordRecord, error) {
	key := opsDiscordKey(binding.NativeSessionIDHash, operationID)
	raw, now, ttl, err := s.readValue(ctx, key)
	if err != nil {
		return opsDiscordRecord{}, err
	}
	var record opsDiscordRecord
	if !strictJSON([]byte(raw), &record, 16384) || !validOpsDiscordRecord(record, binding, operationID, now) ||
		ttl > record.Expires-now+1000 {
		return opsDiscordRecord{}, errUnauthorized
	}
	record.raw = raw
	return record, nil
}

func (s *Service) createOpsDiscord(ctx context.Context, binding session.BFFBinding, operationID, challengeID, lease string) (opsDiscordRecord, error) {
	now := time.Now().UnixMilli()
	record := opsDiscordRecord{Schema: 1, State: "START_PENDING", UserID: binding.UserID,
		NativeHash: binding.NativeSessionIDHash, OperationID: operationID, ChallengeHash: hashText(challengeID),
		Created: now, Expires: now + opsDiscordTTL.Milliseconds(), LeaseHash: hashText(lease)}
	raw, _ := json.Marshal(record)
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	created, err := s.redis.SetNX(c, opsDiscordKey(record.NativeHash, operationID), raw, opsDiscordTTL).Result()
	cancel()
	if err != nil {
		return opsDiscordRecord{}, errUnavailable
	}
	if !created {
		return opsDiscordRecord{}, errConflict
	}
	record.raw = string(raw)
	return record, nil
}

func (s *Service) deleteOpsDiscord(ctx context.Context, record opsDiscordRecord) error {
	return s.casValue(ctx, opsDiscordKey(record.NativeHash, record.OperationID), record.raw, "", 0)
}

// CancelOpsDiscord removes the server-side browser/2FA state for one Ops
// operation. It never exposes or returns the sealed Native browser credential.
func (s *Service) CancelOpsDiscord(ctx context.Context, verified session.RequestSession, operationID string) error {
	if s == nil || s.sessions == nil || s.redis == nil || ctx == nil || !opsDiscordUUID.MatchString(operationID) {
		return errInput
	}
	binding, err := s.sessions.BFFBinding(ctx, verified)
	if err != nil {
		return mapSessionError(err)
	}
	record, err := s.loadOpsDiscord(ctx, binding, operationID)
	if errors.Is(err, errMissing) {
		return nil
	}
	if err != nil {
		return err
	}
	current, err := s.sessions.BFFBinding(ctx, verified)
	if err != nil {
		return mapSessionError(err)
	}
	if current != binding {
		return errUnauthorized
	}
	var browser string
	if record.NativeBrowserBox != "" {
		if value, openErr := s.openOpsDiscordBrowser(record); openErr == nil {
			browser = value.Cookie
		}
	}
	if err = s.deleteOpsDiscord(ctx, record); err != nil {
		return err
	}
	if browser != "" {
		_ = s.nativeBrowserLogout(ctx, browser)
	}
	return nil
}

func (s *Service) claimOpsDiscord(ctx context.Context, record opsDiscordRecord, want, next string) (opsDiscordRecord, string, error) {
	if record.State != want || record.LeaseHash != "" {
		return record, "", errConflict
	}
	lease, err := randomSecret()
	if err != nil {
		return record, "", err
	}
	old := record.raw
	record.State, record.LeaseHash = next, hashText(lease)
	raw, _ := json.Marshal(record)
	if err = s.casValue(ctx, opsDiscordKey(record.NativeHash, record.OperationID), old, string(raw), 0); err != nil {
		return record, "", err
	}
	record.raw = string(raw)
	return record, lease, nil
}

func (s *Service) restoreOpsDiscord(ctx context.Context, record opsDiscordRecord, lease string, attempt bool) (bool, error) {
	if record.LeaseHash != hashText(lease) || record.State != "TWO_FA_PENDING" {
		return false, errConflict
	}
	old := record.raw
	record.State, record.LeaseHash = "DISCORD_TWO_FA", ""
	if attempt {
		record.Attempts++
	}
	if record.Attempts >= maximumAttempts {
		return true, s.casValue(ctx, opsDiscordKey(record.NativeHash, record.OperationID), old, "", 0)
	}
	raw, _ := json.Marshal(record)
	return false, s.casValue(ctx, opsDiscordKey(record.NativeHash, record.OperationID), old, string(raw), 0)
}

func (s *Service) storeOpsDiscordStart(ctx context.Context, record opsDiscordRecord, lease, authorization, state, browser string) (OpsDiscordStartResult, error) {
	if record.State != "START_PENDING" || record.LeaseHash != hashText(lease) {
		return OpsDiscordStartResult{}, errConflict
	}
	record.NativeStateHash = hashText(state)
	box, err := s.sealJSON(opsDiscordBrowserLabel(record), opsDiscordBrowserValue{Cookie: browser, AuthorizationURL: authorization})
	if err != nil {
		return OpsDiscordStartResult{}, err
	}
	old := record.raw
	record.State, record.LeaseHash, record.NativeBrowserBox = "DISCORD_CALLBACK", "", box
	raw, _ := json.Marshal(record)
	if err = s.casValue(ctx, opsDiscordKey(record.NativeHash, record.OperationID), old, string(raw), 0); err != nil {
		return OpsDiscordStartResult{}, err
	}
	return OpsDiscordStartResult{AuthorizationURL: authorization}, nil
}

func (s *Service) openOpsDiscordBrowser(record opsDiscordRecord) (opsDiscordBrowserValue, error) {
	var value opsDiscordBrowserValue
	if s.openJSON(opsDiscordBrowserLabel(record), record.NativeBrowserBox, &value) != nil || !opaque(value.Cookie) {
		return opsDiscordBrowserValue{}, errUnauthorized
	}
	state, ok := validateDiscordAuthorization(value.AuthorizationURL, s.origin, DiscordPurposeOpsFresh)
	if !ok || subtle.ConstantTimeCompare([]byte(hashText(state)), []byte(record.NativeStateHash)) != 1 {
		return opsDiscordBrowserValue{}, errUnauthorized
	}
	return value, nil
}

func (s *Service) storeOpsDiscordFlow(ctx context.Context, record opsDiscordRecord, lease string, nativeToken string, nativeExpires time.Time) (OpsFactorResult, error) {
	if record.State != "CALLBACK_PENDING" || record.LeaseHash != hashText(lease) || !opaque(nativeToken) {
		return OpsFactorResult{}, errConflict
	}
	clientToken, err := randomSecret()
	if err != nil {
		return OpsFactorResult{}, err
	}
	now := time.Now()
	deadline := nativeExpires
	if limit := now.Add(challengeTTL); deadline.After(limit) {
		deadline = limit
	}
	if limit := time.UnixMilli(record.Expires); deadline.After(limit) {
		deadline = limit
	}
	if !deadline.After(now) {
		return OpsFactorResult{}, errUnknown
	}
	record.FlowHash, record.FlowExpires = hashText(clientToken), deadline.UnixMilli()
	box, err := s.sealJSON(opsDiscordFlowLabel(record), opsDiscordFlowValue{NativeToken: nativeToken, ClientToken: clientToken})
	if err != nil {
		return OpsFactorResult{}, err
	}
	old := record.raw
	record.State, record.LeaseHash, record.NativeFlowBox, record.Attempts = "DISCORD_TWO_FA", "", box, 0
	raw, _ := json.Marshal(record)
	if err = s.casValue(ctx, opsDiscordKey(record.NativeHash, record.OperationID), old, string(raw), 0); err != nil {
		return OpsFactorResult{}, err
	}
	return OpsFactorResult{Require2FA: true, FlowToken: clientToken}, nil
}

func (s *Service) openOpsDiscordFlow(record opsDiscordRecord) (opsDiscordFlowValue, error) {
	var value opsDiscordFlowValue
	if s.openJSON(opsDiscordFlowLabel(record), record.NativeFlowBox, &value) != nil ||
		!opaque(value.NativeToken) || !opaque(value.ClientToken) || hashText(value.ClientToken) != record.FlowHash {
		return opsDiscordFlowValue{}, errUnauthorized
	}
	return value, nil
}

func (s *Service) storeOpsDiscordProof(ctx context.Context, record opsDiscordRecord, lease, proof string, expires time.Time) (OpsFactorResult, error) {
	if (record.State != "CALLBACK_PENDING" && record.State != "TWO_FA_PENDING") ||
		record.LeaseHash != hashText(lease) || !opaque(proof) {
		return OpsFactorResult{}, errConflict
	}
	now := time.Now()
	deadline := expires
	if limit := time.UnixMilli(record.Expires); deadline.After(limit) {
		deadline = limit
	}
	if !deadline.After(now) {
		return OpsFactorResult{}, errUnknown
	}
	record.ProofExpires = deadline.UnixMilli()
	if record.FlowHash != "" {
		record.FlowExpires = record.ProofExpires
	}
	box, err := s.sealJSON(opsDiscordProofLabel(record), struct {
		Proof string `json:"proof"`
	}{proof})
	if err != nil {
		return OpsFactorResult{}, err
	}
	old := record.raw
	record.State, record.LeaseHash, record.ProofBox = "PROOF_READY", "", box
	record.NativeFlowBox, record.Attempts = "", 0
	raw, _ := json.Marshal(record)
	if err = s.casValue(ctx, opsDiscordKey(record.NativeHash, record.OperationID), old, string(raw), 0); err != nil {
		return OpsFactorResult{}, err
	}
	return OpsFactorResult{Proof: proof}, nil
}

func (s *Service) openOpsDiscordProof(record opsDiscordRecord) (OpsFactorResult, error) {
	var value struct {
		Proof string `json:"proof"`
	}
	if s.openJSON(opsDiscordProofLabel(record), record.ProofBox, &value) != nil || !opaque(value.Proof) {
		return OpsFactorResult{}, errUnauthorized
	}
	return OpsFactorResult{Proof: value.Proof}, nil
}

func opsDiscordFactorError(err error) error {
	if nativeUnknown(err) {
		return Fault{Status: http.StatusServiceUnavailable, Code: "OPS_FACTOR_RESULT_UNKNOWN", Unknown: true}
	}
	var fault Fault
	if !errors.As(err, &fault) {
		return errUnavailable
	}
	switch fault.Code {
	case "INVALID_TWO_FA":
		return Fault{Status: http.StatusUnprocessableEntity, Code: "OPS_FACTOR_REJECTED"}
	case "RATE_LIMITED":
		return Fault{Status: http.StatusTooManyRequests, Code: "AUTH_RATE_LIMITED"}
	case "AUTH_CANCELLED":
		return Fault{Status: http.StatusBadRequest, Code: "AUTH_CANCELLED"}
	case "DISCORD_ADMISSION_DENIED":
		return Fault{Status: http.StatusForbidden, Code: "OPS_FACTOR_REJECTED"}
	case "AUTH_REQUIRED":
		return fault
	default:
		return Fault{Status: http.StatusServiceUnavailable, Code: "OPS_FRESH_UNAVAILABLE"}
	}
}

func (s *Service) OpsDiscordStart(ctx context.Context, verified session.RequestSession, operationID, challengeID, challengeToken string) (OpsDiscordStartResult, error) {
	if !opaque(challengeToken) {
		return OpsDiscordStartResult{}, errInput
	}
	binding, credential, err := s.opsDiscordAuthority(ctx, verified, operationID, challengeID)
	if err != nil {
		return OpsDiscordStartResult{}, err
	}
	if current, loadErr := s.loadOpsDiscord(ctx, binding, operationID); loadErr == nil {
		if current.ChallengeHash != hashText(challengeID) || current.State != "DISCORD_CALLBACK" {
			return OpsDiscordStartResult{}, errConflict
		}
		browser, openErr := s.openOpsDiscordBrowser(current)
		if openErr != nil {
			return OpsDiscordStartResult{}, openErr
		}
		return OpsDiscordStartResult{AuthorizationURL: browser.AuthorizationURL}, nil
	} else if !errors.Is(loadErr, errMissing) {
		return OpsDiscordStartResult{}, loadErr
	}
	lease, err := randomSecret()
	if err != nil {
		return OpsDiscordStartResult{}, err
	}
	record, err := s.createOpsDiscord(ctx, binding, operationID, challengeID, lease)
	if err != nil {
		return OpsDiscordStartResult{}, err
	}
	authorization, state, browser, nativeErr := s.nativeOpsDiscordStart(ctx, challengeToken, &credential)
	finishCtx, finish := finalizationContext(ctx)
	defer finish()
	if nativeErr != nil {
		_ = s.deleteOpsDiscord(finishCtx, record)
		if browser != "" {
			_ = s.nativeBrowserLogout(finishCtx, browser)
		}
		return OpsDiscordStartResult{}, opsDiscordFactorError(nativeErr)
	}
	current, _, authorityErr := s.opsDiscordAuthority(finishCtx, verified, operationID, challengeID)
	if authorityErr != nil || current != binding {
		_ = s.deleteOpsDiscord(finishCtx, record)
		_ = s.nativeBrowserLogout(finishCtx, browser)
		if authorityErr != nil {
			return OpsDiscordStartResult{}, authorityErr
		}
		return OpsDiscordStartResult{}, errUnauthorized
	}
	result, err := s.storeOpsDiscordStart(finishCtx, record, lease, authorization, state, browser)
	if err != nil {
		_ = s.nativeBrowserLogout(finishCtx, browser)
	}
	return result, err
}

func (s *Service) OpsDiscordCallback(ctx context.Context, verified session.RequestSession, operationID, challengeID string, input DiscordCallbackInput) (OpsFactorResult, error) {
	if !input.valid() {
		return OpsFactorResult{}, errInput
	}
	binding, credential, err := s.opsDiscordAuthority(ctx, verified, operationID, challengeID)
	if err != nil {
		return OpsFactorResult{}, err
	}
	record, err := s.loadOpsDiscord(ctx, binding, operationID)
	if err != nil || record.ChallengeHash != hashText(challengeID) ||
		subtle.ConstantTimeCompare([]byte(record.NativeStateHash), []byte(hashText(input.State))) != 1 {
		if err != nil {
			return OpsFactorResult{}, err
		}
		return OpsFactorResult{}, errConflict
	}
	if record.State == "PROOF_READY" {
		return s.openOpsDiscordProof(record)
	}
	if record.State == "DISCORD_TWO_FA" {
		flow, openErr := s.openOpsDiscordFlow(record)
		if openErr != nil {
			return OpsFactorResult{}, openErr
		}
		return OpsFactorResult{Require2FA: true, FlowToken: flow.ClientToken}, nil
	}
	pending, lease, err := s.claimOpsDiscord(ctx, record, "DISCORD_CALLBACK", "CALLBACK_PENDING")
	if err != nil {
		return OpsFactorResult{}, err
	}
	browser, err := s.openOpsDiscordBrowser(pending)
	if err != nil {
		_ = s.deleteOpsDiscord(ctx, pending)
		return OpsFactorResult{}, err
	}
	result, nativeErr := s.nativeDiscordResult(ctx, http.MethodGet, "/api/momiao/auth/discord/callback",
		input.query(), nil, &credential, browser.Cookie, DiscordPurposeOpsFresh, "callback")
	finishCtx, finish := finalizationContext(ctx)
	defer finish()
	if nativeErr != nil {
		_ = s.deleteOpsDiscord(finishCtx, pending)
		_ = s.nativeBrowserLogout(finishCtx, browser.Cookie)
		return OpsFactorResult{}, opsDiscordFactorError(nativeErr)
	}
	current, _, authorityErr := s.opsDiscordAuthority(finishCtx, verified, operationID, challengeID)
	if authorityErr != nil || current != binding {
		_ = s.deleteOpsDiscord(finishCtx, pending)
		_ = s.nativeBrowserLogout(finishCtx, browser.Cookie)
		if authorityErr != nil {
			return OpsFactorResult{}, authorityErr
		}
		return OpsFactorResult{}, errUnauthorized
	}
	if result.Challenge != nil {
		return s.storeOpsDiscordFlow(finishCtx, pending, lease, result.Challenge.FlowToken, result.Challenge.ExpiresAt)
	}
	if result.Proof == "" {
		_ = s.deleteOpsDiscord(finishCtx, pending)
		_ = s.nativeBrowserLogout(finishCtx, browser.Cookie)
		return OpsFactorResult{}, Fault{Status: http.StatusServiceUnavailable, Code: "OPS_FACTOR_RESULT_UNKNOWN", Unknown: true}
	}
	return s.storeOpsDiscordProof(finishCtx, pending, lease, result.Proof, result.ProofExpires)
}

func (s *Service) OpsDiscordTwoFactor(ctx context.Context, verified session.RequestSession, operationID, challengeID, flowToken, code string) (OpsFactorResult, error) {
	if !opaque(flowToken) || code == "" || len(code) > 64 || strings.TrimSpace(code) != code || !utf8.ValidString(code) {
		return OpsFactorResult{}, errInput
	}
	binding, credential, err := s.opsDiscordAuthority(ctx, verified, operationID, challengeID)
	if err != nil {
		return OpsFactorResult{}, err
	}
	record, err := s.loadOpsDiscord(ctx, binding, operationID)
	if err != nil || record.ChallengeHash != hashText(challengeID) || record.FlowHash != hashText(flowToken) {
		if err != nil {
			return OpsFactorResult{}, err
		}
		return OpsFactorResult{}, errConflict
	}
	if record.State == "PROOF_READY" {
		return s.openOpsDiscordProof(record)
	}
	if record.State != "DISCORD_TWO_FA" || record.FlowExpires <= time.Now().UnixMilli() {
		return OpsFactorResult{}, errConflict
	}
	pending, lease, err := s.claimOpsDiscord(ctx, record, "DISCORD_TWO_FA", "TWO_FA_PENDING")
	if err != nil {
		return OpsFactorResult{}, err
	}
	flow, flowErr := s.openOpsDiscordFlow(pending)
	browser, browserErr := s.openOpsDiscordBrowser(pending)
	if flowErr != nil || browserErr != nil {
		_ = s.deleteOpsDiscord(ctx, pending)
		return OpsFactorResult{}, errUnauthorized
	}
	result, nativeErr := s.nativeDiscordResult(ctx, http.MethodPost, "/api/momiao/ops/fresh/2fa", nil,
		map[string]string{"flow_token": flow.NativeToken, "code": code}, &credential, browser.Cookie,
		DiscordPurposeOpsFresh, "2fa")
	finishCtx, finish := finalizationContext(ctx)
	defer finish()
	if nativeErr != nil {
		var fault Fault
		if errors.As(nativeErr, &fault) && fault.Code == "INVALID_TWO_FA" {
			exhausted, restoreErr := s.restoreOpsDiscord(finishCtx, pending, lease, true)
			if restoreErr != nil {
				return OpsFactorResult{}, restoreErr
			}
			if exhausted {
				_ = s.nativeBrowserLogout(finishCtx, browser.Cookie)
			}
			return OpsFactorResult{}, opsDiscordFactorError(nativeErr)
		}
		if errors.As(nativeErr, &fault) && fault.Code == "RATE_LIMITED" {
			_, restoreErr := s.restoreOpsDiscord(finishCtx, pending, lease, false)
			if restoreErr != nil {
				return OpsFactorResult{}, restoreErr
			}
			return OpsFactorResult{}, opsDiscordFactorError(nativeErr)
		}
		_ = s.deleteOpsDiscord(finishCtx, pending)
		_ = s.nativeBrowserLogout(finishCtx, browser.Cookie)
		return OpsFactorResult{}, opsDiscordFactorError(nativeErr)
	}
	current, _, authorityErr := s.opsDiscordAuthority(finishCtx, verified, operationID, challengeID)
	if authorityErr != nil || current != binding {
		_ = s.deleteOpsDiscord(finishCtx, pending)
		_ = s.nativeBrowserLogout(finishCtx, browser.Cookie)
		if authorityErr != nil {
			return OpsFactorResult{}, authorityErr
		}
		return OpsFactorResult{}, errUnauthorized
	}
	if result.Proof == "" || result.Challenge != nil || result.Credential != nil {
		_ = s.deleteOpsDiscord(finishCtx, pending)
		_ = s.nativeBrowserLogout(finishCtx, browser.Cookie)
		return OpsFactorResult{}, Fault{Status: http.StatusServiceUnavailable, Code: "OPS_FACTOR_RESULT_UNKNOWN", Unknown: true}
	}
	return s.storeOpsDiscordProof(finishCtx, pending, lease, result.Proof, result.ProofExpires)
}
