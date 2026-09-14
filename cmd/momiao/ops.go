package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/nativeself"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/session"
)

const opsAPIPath = "/api/v1/ops"

var opsSecretPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,512}$`)

func opsAPIRoute(path string) bool {
	return path == opsAPIPath || strings.HasPrefix(path, opsAPIPath+"/")
}

type opsFreshFactorResult struct {
	Proof      string `json:"proof,omitempty"`
	Require2FA bool   `json:"require_2fa,omitempty"`
	FlowToken  string `json:"flow_token,omitempty"`
}

// Implementations resolve Native credentials from the verified opaque session.
// Passwords, codes and server-held Native credentials must never enter Ops audit.
type opsFreshFactor interface {
	Password(context.Context, session.RequestSession, string, string) (opsFreshFactorResult, error)
	TwoFactor(context.Context, session.RequestSession, string, string) (opsFreshFactorResult, error)
	Cancel(context.Context, session.RequestSession, string) error
	DiscordStart(context.Context, session.RequestSession, string, string, string) (string, error)
	DiscordCallback(context.Context, session.RequestSession, string, string, opsDiscordCallbackInput) (opsFreshFactorResult, error)
	DiscordTwoFactor(context.Context, session.RequestSession, string, string, string, string) (opsFreshFactorResult, error)
}

type opsDiscordCallbackInput struct {
	Code             string
	Error            string
	ErrorDescription string
	State            string
}

type opsHTTP struct {
	sessions              *session.Service
	service               *platform.OpsService
	fresh                 opsFreshFactor
	nativeAdminCapability string
	runtime               http.Handler
}

// runtime owns additional exact /api/v1/ops/* domain reads and commands. Core
// operation/access/audit/fresh routes remain owned here and cannot fall through.
func newOpsHandler(sessions *session.Service, service *platform.OpsService, fresh opsFreshFactor,
	nativeAdminCapability string, runtime http.Handler) http.Handler {
	if nativeAdminCapability != "AVAILABLE" && nativeAdminCapability != "NOT_AVAILABLE" {
		nativeAdminCapability = "UNKNOWN"
	}
	return &opsHTTP{sessions: sessions, service: service, fresh: fresh,
		nativeAdminCapability: nativeAdminCapability, runtime: runtime}
}

func coreOpsRoute(path string) bool {
	if path == opsAPIPath+"/bootstrap" || path == opsAPIPath+"/operations" ||
		path == opsAPIPath+"/admin-principals" || path == opsAPIPath+"/audit" ||
		path == opsAPIPath+"/fresh/password" || path == opsAPIPath+"/fresh/2fa" ||
		path == opsAPIPath+"/fresh/discord/start" || path == opsAPIPath+"/fresh/discord/callback" ||
		path == opsAPIPath+"/fresh/discord/2fa" ||
		path == opsAPIPath+"/fresh/exchange" || path == opsAPIPath+"/fresh/cancel" {
		return true
	}
	if strings.HasPrefix(path, opsAPIPath+"/operations/") || strings.HasPrefix(path, opsAPIPath+"/audit/") {
		return true
	}
	return false
}

func (h *opsHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !opsAPIRoute(r.URL.Path) {
		walletError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if !coreOpsRoute(r.URL.Path) && h != nil && h.runtime != nil {
		h.runtime.ServeHTTP(w, r)
		return
	}
	if h == nil || h.sessions == nil || h.service == nil {
		walletError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE")
		return
	}
	if !opsRequestSafe(r) {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	verified, err := h.sessions.VerifyPrivateRequest(r)
	if err != nil {
		sessionError(w, err)
		return
	}
	userID, err := strconv.ParseInt(verified.View().UserID, 10, 64)
	if err != nil || userID <= 0 {
		walletError(w, http.StatusUnauthorized, "SESSION_UNAUTHORIZED")
		return
	}
	switch r.URL.Path {
	case opsAPIPath + "/bootstrap":
		h.bootstrap(w, r, userID)
	case opsAPIPath + "/operations":
		if r.Method == http.MethodGet {
			h.operationList(w, r, userID)
		} else {
			h.prepare(w, r, verified, userID)
		}
	case opsAPIPath + "/admin-principals":
		h.adminPrincipals(w, r, userID)
	case opsAPIPath + "/audit":
		h.auditList(w, r, userID)
	case opsAPIPath + "/fresh/password":
		h.freshPassword(w, r, verified)
	case opsAPIPath + "/fresh/2fa":
		h.freshTwoFactor(w, r, verified)
	case opsAPIPath + "/fresh/discord/start":
		h.freshDiscordStart(w, r, verified, userID)
	case opsAPIPath + "/fresh/discord/callback":
		h.freshDiscordCallback(w, r, verified, userID)
	case opsAPIPath + "/fresh/discord/2fa":
		h.freshDiscordTwoFactor(w, r, verified, userID)
	case opsAPIPath + "/fresh/exchange":
		h.freshExchange(w, r, verified, userID)
	case opsAPIPath + "/fresh/cancel":
		h.freshCancel(w, r, verified)
	default:
		switch {
		case strings.HasPrefix(r.URL.Path, opsAPIPath+"/operations/"):
			h.operation(w, r, verified, userID)
		case strings.HasPrefix(r.URL.Path, opsAPIPath+"/audit/"):
			h.auditOne(w, r, userID)
		default:
			walletError(w, http.StatusNotFound, "NOT_FOUND")
		}
	}
}

func opsRequestSafe(r *http.Request) bool {
	if r == nil || r.URL == nil || r.URL.RawPath != "" || r.ContentLength > 65536 {
		return false
	}
	if _, err := url.ParseQuery(r.URL.RawQuery); err != nil {
		return false
	}
	for _, name := range []string{"Authorization", "New-Api-User", "X-Auth-Session", "X-Game-CSRF-Token", "Proxy-Authorization"} {
		if len(r.Header.Values(name)) != 0 {
			return false
		}
	}
	return true
}

func decodeOpsBody(r *http.Request, out any) bool {
	if r == nil || len(r.Header.Values("Content-Type")) != 1 {
		return false
	}
	media, parameters, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(parameters) != 0 {
		return false
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 65537))
	if err != nil || len(raw) == 0 || len(raw) > 65536 || !utf8.Valid(raw) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if !nativeself.UniqueJSON(decoder, 0) {
		return false
	}
	if _, err = decoder.Token(); !errors.Is(err, io.EOF) {
		return false
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(out); err != nil {
		return false
	}
	return errors.Is(decoder.Decode(new(any)), io.EOF)
}

func exactOpsQuery(values url.Values, allowed ...string) bool {
	set := map[string]bool{}
	for _, name := range allowed {
		set[name] = true
	}
	for name, value := range values {
		if !set[name] || len(value) != 1 || value[0] == "" {
			return false
		}
	}
	return true
}

func requireOpsQuery(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || r.URL.ForceQuery || !exactOpsQuery(values, allowed...) {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return false
	}
	return true
}

func requireNoOpsQuery(w http.ResponseWriter, r *http.Request) bool {
	return requireOpsQuery(w, r)
}

func requireOpsRead(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	if !requireMethod(w, r, http.MethodGet) {
		return false
	}
	if r.ContentLength > 0 {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return false
	}
	return requireOpsQuery(w, r, allowed...)
}

func opsEpoch(value string) (int64, bool) {
	epoch, err := strconv.ParseInt(value, 10, 64)
	return epoch, err == nil && epoch > 0 && strconv.FormatInt(epoch, 10) == value
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	walletError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
	return false
}

func (h *opsHTTP) bootstrap(w http.ResponseWriter, r *http.Request, userID int64) {
	if !requireOpsRead(w, r) {
		return
	}
	bootstrap, err := h.service.Bootstrap(r.Context(), userID)
	if err != nil {
		writeOpsError(w, err)
		return
	}
	sessionEnvelope(w, http.StatusOK, struct {
		platform.OpsBootstrap
		NativeAdminCapability string `json:"native_admin_capability"`
	}{bootstrap, h.nativeAdminCapability})
}

type opsPrepareResponse struct {
	platform.OpsPrepared
	FreshChallenge *session.Challenge `json:"fresh_challenge,omitempty"`
}

func (h *opsHTTP) prepare(w http.ResponseWriter, r *http.Request, verified session.RequestSession, userID int64) {
	if !requireMethod(w, r, http.MethodPost) || !requireNoOpsQuery(w, r) {
		return
	}
	var request platform.OpsPrepareRequest
	if !decodeOpsBody(r, &request) {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	prepared, err := h.service.Prepare(r.Context(), userID, request)
	if err != nil {
		writeOpsError(w, err)
		return
	}
	response := opsPrepareResponse{OpsPrepared: prepared}
	if prepared.Descriptor.RequiresFreshAuth {
		if prepared.Operation.FreshChallengeBound() {
			writeOpsError(w, platform.ErrOpsConflict)
			return
		}
		binding, bindingErr := prepared.Operation.FreshBinding()
		if bindingErr != nil {
			writeOpsError(w, bindingErr)
			return
		}
		challenge, startErr := h.sessions.StartOps(r.Context(), verified, session.Operation{
			ID: binding.OperationID, Action: binding.Action, TargetKind: binding.TargetKind,
			TargetID: binding.TargetID, ExpectedVersion: binding.ExpectedVersion,
			CommandDigest: binding.CommandDigest, ImpactDigest: binding.ImpactDigest,
		})
		if startErr != nil {
			sessionError(w, startErr)
			return
		}
		if bindErr := h.service.BindFreshChallenge(r.Context(), userID, request.AuthzEpoch, request.OperationID, challenge.ID); bindErr != nil {
			_ = h.sessions.CancelOps(r.Context(), verified, request.OperationID, challenge.ID)
			if h.fresh != nil {
				_ = h.fresh.Cancel(r.Context(), verified, request.OperationID)
			}
			writeOpsError(w, bindErr)
			return
		}
		response.FreshChallenge = &challenge
	}
	sessionEnvelope(w, http.StatusOK, response)
}

func (h *opsHTTP) operationList(w http.ResponseWriter, r *http.Request, userID int64) {
	if !requireOpsRead(w, r, "authz_epoch", "state", "operation_type", "actor_newapi_user_id", "before", "before_id", "limit") {
		return
	}
	epoch, ok := opsEpoch(r.URL.Query().Get("authz_epoch"))
	if !ok {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	query := platform.OpsOperationListQuery{State: r.URL.Query().Get("state"),
		OperationType: r.URL.Query().Get("operation_type"), Limit: 50,
		BeforeID: r.URL.Query().Get("before_id")}
	if value := r.URL.Query().Get("actor_newapi_user_id"); value != "" {
		query.ActorUserID, ok = opsEpoch(value)
		if !ok {
			walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
			return
		}
	}
	if value := r.URL.Query().Get("limit"); value != "" {
		query.Limit, _ = strconv.Atoi(value)
	}
	if value := r.URL.Query().Get("before"); value != "" {
		query.Before, _ = time.Parse(time.RFC3339Nano, value)
		if query.Before.IsZero() {
			walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
			return
		}
	}
	page, err := h.service.Operations(r.Context(), userID, epoch, query)
	if err != nil {
		writeOpsError(w, err)
		return
	}
	sessionEnvelope(w, http.StatusOK, page)
}

type opsExecuteBody struct {
	AuthzEpoch        int64  `json:"authz_epoch,string"`
	ImpactHash        string `json:"impact_hash"`
	Confirmed         bool   `json:"confirmed"`
	TypedConfirmation string `json:"typed_confirmation,omitempty"`
	Fresh             *struct {
		ChallengeID string `json:"challenge_id"`
	} `json:"fresh,omitempty"`
}

func (h *opsHTTP) operation(w http.ResponseWriter, r *http.Request, verified session.RequestSession, userID int64) {
	tail := strings.TrimPrefix(r.URL.Path, opsAPIPath+"/operations/")
	parts := strings.Split(tail, "/")
	if len(parts) < 1 || !opsUUIDPatternHTTP(parts[0]) {
		walletError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	operationID := parts[0]
	action := "read"
	switch {
	case len(parts) == 1:
	case len(parts) == 2 && parts[1] == "execute":
		action = "execute"
	case len(parts) == 2 && parts[1] == "cancel":
		action = "cancel"
	case len(parts) == 3 && parts[1] == "fresh" && parts[2] == "renew":
		action = "renew"
	default:
		walletError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if action == "read" {
		if !requireOpsRead(w, r, "authz_epoch") {
			return
		}
		epoch, ok := opsEpoch(r.URL.Query().Get("authz_epoch"))
		if !ok {
			walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
			return
		}
		view, err := h.service.Operation(r.Context(), userID, epoch, operationID)
		if err != nil {
			writeOpsError(w, err)
			return
		}
		sessionEnvelope(w, http.StatusOK, view)
		return
	}
	if action == "cancel" {
		h.cancelOperation(w, r, verified, userID, operationID)
		return
	}
	if action == "renew" {
		h.renewFresh(w, r, verified, userID, operationID)
		return
	}
	if !requireMethod(w, r, http.MethodPost) || !requireNoOpsQuery(w, r) {
		return
	}
	var body opsExecuteBody
	if !decodeOpsBody(r, &body) || !validOpsHTTPText(body.TypedConfirmation, 256, false) {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	request := platform.OpsExecuteRequest{AuthzEpoch: body.AuthzEpoch, ImpactHash: body.ImpactHash,
		Confirmed: body.Confirmed, TypedConfirmation: body.TypedConfirmation}
	if body.Fresh != nil {
		_, err := h.service.FreshOperation(r.Context(), userID, body.AuthzEpoch, operationID)
		if err != nil {
			writeOpsError(w, err)
			return
		}
		if !opsSecretPattern.MatchString(body.Fresh.ChallengeID) {
			walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
			return
		}
		view := verified.View()
		if view.Confirmation == nil || view.Confirmation.OperationID != operationID || view.FreshAuthAt == nil {
			walletError(w, http.StatusUnauthorized, "OPS_PROOF_REJECTED")
			return
		}
		request.Fresh = &platform.OpsFreshEvidence{OperationID: operationID, ChallengeID: body.Fresh.ChallengeID,
			ContextHash: view.Confirmation.ContextHash, Method: view.FreshAuthMethod, VerifiedAt: *view.FreshAuthAt}
	}
	result, err := h.service.Execute(r.Context(), userID, operationID, request)
	if err != nil {
		writeOpsError(w, err)
		return
	}
	sessionEnvelope(w, http.StatusOK, map[string]any{"operation": result})
}

func (h *opsHTTP) cancelOperation(w http.ResponseWriter, r *http.Request, verified session.RequestSession, userID int64, operationID string) {
	if !requireMethod(w, r, http.MethodPost) || !requireNoOpsQuery(w, r) {
		return
	}
	var body struct {
		AuthzEpoch int64 `json:"authz_epoch,string"`
	}
	if !decodeOpsBody(r, &body) || body.AuthzEpoch <= 0 {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	result, err := h.service.CancelOperation(r.Context(), userID, body.AuthzEpoch, operationID)
	if err != nil {
		writeOpsError(w, err)
		return
	}
	_ = h.sessions.CancelOps(r.Context(), verified, operationID, "")
	if h.fresh != nil {
		_ = h.fresh.Cancel(r.Context(), verified, operationID)
	}
	sessionEnvelope(w, http.StatusOK, map[string]any{"operation": result})
}

func (h *opsHTTP) renewFresh(w http.ResponseWriter, r *http.Request, verified session.RequestSession, userID int64, operationID string) {
	if !requireMethod(w, r, http.MethodPost) || !requireNoOpsQuery(w, r) {
		return
	}
	var body struct {
		AuthzEpoch int64 `json:"authz_epoch,string"`
	}
	if !decodeOpsBody(r, &body) || body.AuthzEpoch <= 0 {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	renewal, err := h.service.BeginFreshRenewal(r.Context(), userID, body.AuthzEpoch, operationID)
	if err != nil {
		writeOpsError(w, err)
		return
	}
	if err = h.sessions.CancelOps(r.Context(), verified, operationID, ""); err != nil {
		_ = h.service.AbortFreshRenewal(r.Context(), userID, operationID, renewal.Lease)
		sessionError(w, err)
		return
	}
	if h.fresh != nil {
		_ = h.fresh.Cancel(r.Context(), verified, operationID)
	}
	binding, err := renewal.Operation.FreshBinding()
	if err != nil {
		_ = h.service.AbortFreshRenewal(r.Context(), userID, operationID, renewal.Lease)
		writeOpsError(w, err)
		return
	}
	challenge, err := h.sessions.StartOps(r.Context(), verified, session.Operation{
		ID: binding.OperationID, Action: binding.Action, TargetKind: binding.TargetKind,
		TargetID: binding.TargetID, ExpectedVersion: binding.ExpectedVersion,
		CommandDigest: binding.CommandDigest, ImpactDigest: binding.ImpactDigest,
	})
	if err != nil {
		_ = h.service.AbortFreshRenewal(r.Context(), userID, operationID, renewal.Lease)
		sessionError(w, err)
		return
	}
	completed, err := h.service.CompleteFreshRenewal(r.Context(), userID, body.AuthzEpoch, operationID, renewal.Lease, challenge.ID)
	if err != nil {
		_ = h.sessions.CancelOps(r.Context(), verified, operationID, challenge.ID)
		if h.fresh != nil {
			_ = h.fresh.Cancel(r.Context(), verified, operationID)
		}
		_ = h.service.AbortFreshRenewal(r.Context(), userID, operationID, renewal.Lease)
		writeOpsError(w, err)
		return
	}
	response := opsPrepareResponse{OpsPrepared: completed, FreshChallenge: &challenge}
	sessionEnvelope(w, http.StatusOK, response)
}

func opsUUIDPatternHTTP(value string) bool {
	if len(value) != 36 {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return err == nil && strings.ToLower(value) == value && value[8] == '-' && value[13] == '-' && value[18] == '-' && value[23] == '-'
}

func validOpsHTTPText(value string, maximum int, required bool) bool {
	return (!required && value == "" || value != "") && len(value) <= maximum && utf8.ValidString(value) &&
		strings.TrimSpace(value) == value && !strings.ContainsRune(value, 0)
}

func (h *opsHTTP) adminPrincipals(w http.ResponseWriter, r *http.Request, userID int64) {
	if !requireOpsRead(w, r, "authz_epoch") {
		return
	}
	epoch, ok := opsEpoch(r.URL.Query().Get("authz_epoch"))
	if !ok {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	principals, err := h.service.AdminPrincipals(r.Context(), userID, epoch)
	if err != nil {
		writeOpsError(w, err)
		return
	}
	sessionEnvelope(w, http.StatusOK, map[string]any{"items": principals})
}

func (h *opsHTTP) auditOne(w http.ResponseWriter, r *http.Request, userID int64) {
	id := strings.TrimPrefix(r.URL.Path, opsAPIPath+"/audit/")
	if !opsUUIDPatternHTTP(id) {
		walletError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if !requireOpsRead(w, r, "authz_epoch") {
		return
	}
	epoch, ok := opsEpoch(r.URL.Query().Get("authz_epoch"))
	if !ok {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	event, err := h.service.AuditEvent(r.Context(), userID, epoch, id)
	if err != nil {
		writeOpsError(w, err)
		return
	}
	sessionEnvelope(w, http.StatusOK, event)
}

func (h *opsHTTP) auditList(w http.ResponseWriter, r *http.Request, userID int64) {
	if !requireOpsRead(w, r, "authz_epoch", "operation_id", "actor_newapi_user_id", "action", "target_type", "target_id", "before", "limit") {
		return
	}
	epoch, ok := opsEpoch(r.URL.Query().Get("authz_epoch"))
	if !ok {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	query := platform.OpsAuditQuery{OperationID: r.URL.Query().Get("operation_id"), Action: r.URL.Query().Get("action"),
		TargetType: r.URL.Query().Get("target_type"), TargetID: r.URL.Query().Get("target_id"), Limit: 50}
	if value := r.URL.Query().Get("actor_newapi_user_id"); value != "" {
		query.ActorUserID, ok = opsEpoch(value)
		if !ok {
			walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
			return
		}
	}
	if value := r.URL.Query().Get("limit"); value != "" {
		query.Limit, _ = strconv.Atoi(value)
	}
	if value := r.URL.Query().Get("before"); value != "" {
		query.Before, _ = time.Parse(time.RFC3339Nano, value)
		if query.Before.IsZero() {
			walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
			return
		}
	}
	events, err := h.service.AuditEvents(r.Context(), userID, epoch, query)
	if err != nil {
		writeOpsError(w, err)
		return
	}
	sessionEnvelope(w, http.StatusOK, map[string]any{"items": events})
}

func validFreshFactorResult(result opsFreshFactorResult) bool {
	if result.Proof != "" {
		return opsSecretPattern.MatchString(result.Proof) && !result.Require2FA && result.FlowToken == ""
	}
	return result.Require2FA && opsSecretPattern.MatchString(result.FlowToken)
}

func (h *opsHTTP) freshPassword(w http.ResponseWriter, r *http.Request, verified session.RequestSession) {
	if !requireMethod(w, r, http.MethodPost) || !requireNoOpsQuery(w, r) {
		return
	}
	if h.fresh == nil {
		walletError(w, http.StatusServiceUnavailable, "OPS_FRESH_UNAVAILABLE")
		return
	}
	var body struct {
		ChallengeToken string `json:"challenge_token"`
		Password       string `json:"password"`
	}
	if !decodeOpsBody(r, &body) || !opsSecretPattern.MatchString(body.ChallengeToken) || body.Password == "" || len(body.Password) > 1024 || !utf8.ValidString(body.Password) {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	result, err := h.fresh.Password(r.Context(), verified, body.ChallengeToken, body.Password)
	if err != nil {
		sessionError(w, err)
		return
	}
	if !validFreshFactorResult(result) {
		walletError(w, http.StatusServiceUnavailable, "OPS_FRESH_UNAVAILABLE")
		return
	}
	sessionEnvelope(w, http.StatusOK, result)
}

func (h *opsHTTP) freshTwoFactor(w http.ResponseWriter, r *http.Request, verified session.RequestSession) {
	if !requireMethod(w, r, http.MethodPost) || !requireNoOpsQuery(w, r) {
		return
	}
	if h.fresh == nil {
		walletError(w, http.StatusServiceUnavailable, "OPS_FRESH_UNAVAILABLE")
		return
	}
	var body struct {
		FlowToken string `json:"flow_token"`
		Code      string `json:"code"`
	}
	if !decodeOpsBody(r, &body) || !opsSecretPattern.MatchString(body.FlowToken) || !validOpsHTTPText(body.Code, 64, true) {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	result, err := h.fresh.TwoFactor(r.Context(), verified, body.FlowToken, body.Code)
	if err != nil {
		sessionError(w, err)
		return
	}
	if !validFreshFactorResult(result) || result.Require2FA {
		walletError(w, http.StatusServiceUnavailable, "OPS_FRESH_UNAVAILABLE")
		return
	}
	sessionEnvelope(w, http.StatusOK, result)
}

type opsFreshDiscordBase struct {
	OperationID string `json:"operation_id"`
	AuthzEpoch  int64  `json:"authz_epoch,string"`
	ChallengeID string `json:"challenge_id"`
}

func validOpsFreshDiscordBase(body opsFreshDiscordBase) bool {
	return opsUUIDPatternHTTP(body.OperationID) && body.AuthzEpoch > 0 &&
		opsSecretPattern.MatchString(body.ChallengeID)
}

func (h *opsHTTP) freshDiscordStart(w http.ResponseWriter, r *http.Request, verified session.RequestSession, userID int64) {
	if !requireMethod(w, r, http.MethodPost) || !requireNoOpsQuery(w, r) {
		return
	}
	if h.fresh == nil {
		walletError(w, http.StatusServiceUnavailable, "OPS_FRESH_UNAVAILABLE")
		return
	}
	var body struct {
		opsFreshDiscordBase
		ChallengeToken string `json:"challenge_token"`
	}
	if !decodeOpsBody(r, &body) || !validOpsFreshDiscordBase(body.opsFreshDiscordBase) ||
		!opsSecretPattern.MatchString(body.ChallengeToken) {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	if _, err := h.service.FreshOperation(r.Context(), userID, body.AuthzEpoch, body.OperationID); err != nil {
		writeOpsError(w, err)
		return
	}
	authorizationURL, err := h.fresh.DiscordStart(r.Context(), verified, body.OperationID, body.ChallengeID, body.ChallengeToken)
	if err != nil {
		sessionError(w, err)
		return
	}
	parsed, parseErr := url.Parse(authorizationURL)
	if parseErr != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		walletError(w, http.StatusServiceUnavailable, "OPS_FRESH_UNAVAILABLE")
		return
	}
	sessionEnvelope(w, http.StatusOK, map[string]string{"authorization_url": authorizationURL})
}

func (h *opsHTTP) freshDiscordCallback(w http.ResponseWriter, r *http.Request, verified session.RequestSession, userID int64) {
	if !requireMethod(w, r, http.MethodPost) || !requireNoOpsQuery(w, r) {
		return
	}
	if h.fresh == nil {
		walletError(w, http.StatusServiceUnavailable, "OPS_FRESH_UNAVAILABLE")
		return
	}
	var body struct {
		opsFreshDiscordBase
		Code             string `json:"code,omitempty"`
		Error            string `json:"error,omitempty"`
		ErrorDescription string `json:"error_description,omitempty"`
		State            string `json:"state"`
	}
	if !decodeOpsBody(r, &body) {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	callbackShape := (body.Code != "" && body.Error == "" && body.ErrorDescription == "") ||
		(body.Code == "" && body.Error == "access_denied")
	if !validOpsFreshDiscordBase(body.opsFreshDiscordBase) || !opsSecretPattern.MatchString(body.State) ||
		!callbackShape || !validOpsHTTPText(body.Code, 4096, false) ||
		!validOpsHTTPText(body.ErrorDescription, 4096, false) {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	if _, err := h.service.FreshOperation(r.Context(), userID, body.AuthzEpoch, body.OperationID); err != nil {
		writeOpsError(w, err)
		return
	}
	result, err := h.fresh.DiscordCallback(r.Context(), verified, body.OperationID, body.ChallengeID,
		opsDiscordCallbackInput{Code: body.Code, Error: body.Error, ErrorDescription: body.ErrorDescription, State: body.State})
	if err != nil {
		sessionError(w, err)
		return
	}
	if !validFreshFactorResult(result) {
		walletError(w, http.StatusServiceUnavailable, "OPS_FRESH_UNAVAILABLE")
		return
	}
	sessionEnvelope(w, http.StatusOK, result)
}

func (h *opsHTTP) freshDiscordTwoFactor(w http.ResponseWriter, r *http.Request, verified session.RequestSession, userID int64) {
	if !requireMethod(w, r, http.MethodPost) || !requireNoOpsQuery(w, r) {
		return
	}
	if h.fresh == nil {
		walletError(w, http.StatusServiceUnavailable, "OPS_FRESH_UNAVAILABLE")
		return
	}
	var body struct {
		opsFreshDiscordBase
		FlowToken string `json:"flow_token"`
		Code      string `json:"code"`
	}
	if !decodeOpsBody(r, &body) || !validOpsFreshDiscordBase(body.opsFreshDiscordBase) ||
		!opsSecretPattern.MatchString(body.FlowToken) || !validOpsHTTPText(body.Code, 64, true) {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	if _, err := h.service.FreshOperation(r.Context(), userID, body.AuthzEpoch, body.OperationID); err != nil {
		writeOpsError(w, err)
		return
	}
	result, err := h.fresh.DiscordTwoFactor(r.Context(), verified, body.OperationID, body.ChallengeID, body.FlowToken, body.Code)
	if err != nil {
		sessionError(w, err)
		return
	}
	if !validFreshFactorResult(result) || result.Require2FA {
		walletError(w, http.StatusServiceUnavailable, "OPS_FRESH_UNAVAILABLE")
		return
	}
	sessionEnvelope(w, http.StatusOK, result)
}

func (h *opsHTTP) freshExchange(w http.ResponseWriter, r *http.Request, verified session.RequestSession, userID int64) {
	if !requireMethod(w, r, http.MethodPost) || !requireNoOpsQuery(w, r) {
		return
	}
	var body struct {
		OperationID string `json:"operation_id"`
		AuthzEpoch  int64  `json:"authz_epoch,string"`
		ChallengeID string `json:"challenge_id"`
		Proof       string `json:"proof"`
	}
	if !decodeOpsBody(r, &body) || !opsUUIDPatternHTTP(body.OperationID) ||
		!opsSecretPattern.MatchString(body.ChallengeID) || !opsSecretPattern.MatchString(body.Proof) {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	if _, err := h.service.FreshOperation(r.Context(), userID, body.AuthzEpoch, body.OperationID); err != nil {
		writeOpsError(w, err)
		return
	}
	if err := h.sessions.CheckOpsChallenge(r.Context(), verified, body.OperationID, body.ChallengeID); err != nil {
		sessionError(w, err)
		return
	}
	grant, err := h.sessions.CompleteOps(r.Context(), verified, body.ChallengeID, body.Proof)
	if err != nil {
		sessionError(w, err)
		return
	}
	view := grant.View()
	if view.Confirmation == nil || view.Confirmation.OperationID != body.OperationID || view.FreshAuthAt == nil {
		walletError(w, http.StatusUnauthorized, "OPS_PROOF_REJECTED")
		return
	}
	if err = h.sessions.WriteGrant(w, grant); err != nil {
		sessionError(w, err)
		return
	}
	sessionEnvelope(w, http.StatusOK, map[string]any{"session": view})
}

func (h *opsHTTP) freshCancel(w http.ResponseWriter, r *http.Request, verified session.RequestSession) {
	if !requireMethod(w, r, http.MethodPost) || !requireNoOpsQuery(w, r) {
		return
	}
	var body struct {
		OperationID string `json:"operation_id"`
		ChallengeID string `json:"challenge_id,omitempty"`
	}
	if !decodeOpsBody(r, &body) || !opsUUIDPatternHTTP(body.OperationID) ||
		body.ChallengeID != "" && !opsSecretPattern.MatchString(body.ChallengeID) {
		walletError(w, http.StatusBadRequest, "OPS_INPUT_INVALID")
		return
	}
	if err := h.sessions.CancelOps(r.Context(), verified, body.OperationID, body.ChallengeID); err != nil {
		sessionError(w, err)
		return
	}
	sessionEnvelope(w, http.StatusOK, map[string]any{"cancelled": true})
}

func writeOpsError(w http.ResponseWriter, err error) {
	var sessionFault session.Fault
	if errors.As(err, &sessionFault) {
		sessionError(w, err)
		return
	}
	status, code := http.StatusServiceUnavailable, "OPS_UNAVAILABLE"
	switch {
	case errors.Is(err, platform.ErrOpsInvalid):
		status, code = http.StatusBadRequest, err.Error()
	case errors.Is(err, platform.ErrOpsForbidden), errors.Is(err, platform.ErrOpsAuthorizationStale):
		status, code = http.StatusForbidden, err.Error()
	case errors.Is(err, platform.ErrOpsNotFound):
		status, code = http.StatusNotFound, err.Error()
	case errors.Is(err, platform.ErrMaintenanceOverlap):
		status, code = http.StatusConflict, "MAINTENANCE_SCOPE_OVERLAP"
	case errors.Is(err, platform.ErrOpsFreshRequired):
		status, code = http.StatusConflict, err.Error()
	case errors.Is(err, platform.ErrOpsConflict), errors.Is(err, platform.ErrOpsPreviewStale),
		errors.Is(err, platform.ErrOpsConfirmation), errors.Is(err, platform.ErrOpsEnvironment),
		errors.Is(err, platform.ErrOpsLastSuperAdmin):
		status, code = http.StatusConflict, err.Error()
	case errors.Is(err, platform.ErrOpsAuditRejected):
		status, code = http.StatusUnprocessableEntity, err.Error()
	case errors.Is(err, platform.ErrOpsUnavailable):
		status, code = http.StatusServiceUnavailable, err.Error()
	}
	walletError(w, status, code)
}
