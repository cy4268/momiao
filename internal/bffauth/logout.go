package bffauth

import (
	"context"
	"errors"
	"net/http"

	"github.com/cy4268/momiao/internal/session"
)

type ControlRevoker interface {
	RevokeSessionControl(context.Context, session.BFFBinding) error
}

type LogoutResult struct {
	PlatformSession string `json:"platform_session"`
	NativeSession   string `json:"native_session"`
	PokerControl    string `json:"poker_control"`
}

func (r LogoutResult) Confirmed() bool {
	if r.PlatformSession == "NOT_ISSUED" && (r.NativeSession == "NOT_ISSUED" || r.NativeSession == "REVOKED") && r.PokerControl == "UNAVAILABLE_S2" {
		return true
	}
	return r.PlatformSession == "REVOKED" && r.NativeSession == "REVOKED" && (r.PokerControl == "REVOKED" || r.PokerControl == "UNAVAILABLE_S2")
}

func (s *Service) claimLogoutCredential(ctx context.Context, nativeHash, userID string) (credentialRecord, nativeCredential, string, error) {
	record, credential, err := s.loadCredential(ctx, nativeHash)
	if err != nil || userID != "" && credential.UserID != userID {
		return credentialRecord{}, nativeCredential{}, "", errUnavailable
	}
	if record.State == "UNKNOWN" {
		return record, credential, "", errUnknown
	}
	record, lease, err := s.claimCredential(ctx, record, "REVOKING")
	return record, credential, lease, err
}

func (s *Service) completeNativeLogout(ctx context.Context, record credentialRecord, credential nativeCredential, lease string) string {
	if lease == "" {
		return "UNKNOWN"
	}
	if err := s.nativeLogout(ctx, credential); err != nil {
		_ = s.finishCredential(ctx, record, lease, "UNKNOWN", &credential)
		return "UNKNOWN"
	}
	if err := s.finishCredential(ctx, record, lease, "REVOKED", nil); err != nil {
		return "UNKNOWN"
	}
	return "REVOKED"
}

func (s *Service) revokeAuthenticated(ctx context.Context, requestSession session.RequestSession, binding session.BFFBinding) LogoutResult {
	ctx, finish := finalizationContext(ctx)
	defer finish()
	result := LogoutResult{PlatformSession: "UNKNOWN", NativeSession: "UNKNOWN", PokerControl: "UNAVAILABLE_S2"}
	record, credential, lease, _ := s.claimLogoutCredential(ctx, binding.NativeSessionIDHash, binding.UserID)
	if err := s.sessions.Revoke(ctx, requestSession); err == nil {
		result.PlatformSession = "REVOKED"
	}
	// Fence new tickets/handshakes at their shared Native authority before
	// enumerating this runtime's sockets and control leases.
	result.NativeSession = s.completeNativeLogout(ctx, record, credential, lease)
	if s.control != nil {
		if err := s.control.RevokeSessionControl(ctx, binding); err == nil && result.NativeSession == "REVOKED" {
			result.PokerControl = "REVOKED"
		} else {
			result.PokerControl = "UNKNOWN"
		}
	}
	return result
}

func (s *Service) Logout(r *http.Request) (LogoutResult, error) {
	if s == nil || r == nil || r.Method != http.MethodPost {
		return LogoutResult{}, errInput
	}
	requestSession, sessionErr := s.sessions.VerifyRequest(r)
	if sessionErr == nil {
		binding, err := s.sessions.BFFBinding(r.Context(), requestSession)
		if err != nil {
			return LogoutResult{}, mapSessionError(err)
		}
		_ = s.cancelAccountDiscord(r.Context(), r)
		return s.revokeAuthenticated(r.Context(), requestSession, binding), nil
	}
	if !isSessionUnauthorized(sessionErr) {
		return LogoutResult{}, mapSessionError(sessionErr)
	}
	preauth, _, err := s.verifyPreauth(r)
	if err != nil {
		return LogoutResult{}, err
	}
	ctx, finish := finalizationContext(r.Context())
	defer finish()
	was := preauth.State
	discordCleanup := s.cleanupPreauthDiscord(ctx, preauth)
	if err = s.cancelPreauth(ctx, preauth); err != nil {
		return LogoutResult{}, err
	}
	result := LogoutResult{PlatformSession: "NOT_ISSUED", NativeSession: "NOT_ISSUED", PokerControl: "UNAVAILABLE_S2"}
	if was == "DISCORD_START_PENDING" || was == "DISCORD_CALLBACK_PENDING" || was == "DISCORD_TWO_FA_PENDING" {
		result.NativeSession = "PENDING_COMPENSATION"
		return result, nil
	}
	if was == "DISCORD_CALLBACK" || was == "DISCORD_TWO_FA" {
		result.NativeSession = discordCleanup
		return result, nil
	}
	if was == "PASSWORD_PENDING" || was == "TWO_FA_PENDING" {
		result.NativeSession = "PENDING_COMPENSATION"
		return result, nil
	}
	if was == "UNKNOWN" {
		result.NativeSession = "UNKNOWN"
		return result, nil
	}
	if was != "COMMITTED" {
		return result, nil
	}
	result.PlatformSession, result.NativeSession = "UNKNOWN", "UNKNOWN"
	var upgrade struct {
		SID string `json:"sid"`
	}
	if s.openJSON("upgrade:1:"+preauth.Hash, preauth.PlatformSessionBox, &upgrade) != nil || !opaque(upgrade.SID) {
		result.PlatformSession, result.NativeSession = "UNKNOWN", "UNKNOWN"
		return result, nil
	}
	recovered, verifyErr := s.verifiedPlatform(ctx, upgrade.SID, preauth.PlatformCSRF, http.MethodPost)
	if verifyErr == nil {
		binding, err := s.sessions.BFFBinding(ctx, recovered)
		if err == nil {
			return s.revokeAuthenticated(ctx, recovered, binding), nil
		}
	}
	if errors.Is(verifyErr, context.Canceled) {
		return result, verifyErr
	}
	if digest(preauth.NativeHash) {
		record, credential, lease, _ := s.claimLogoutCredential(ctx, preauth.NativeHash, "")
		result.NativeSession = s.completeNativeLogout(ctx, record, credential, lease)
	}
	return result, nil
}
