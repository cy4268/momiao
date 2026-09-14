package bffauth

import (
	"errors"
	"net/http"
)

// OptionalDomainRequest preserves anonymous reads only for a selected local
// public endpoint. It never treats a failed authenticated credential as public,
// issues a cookie, or completes an in-flight login upgrade.
func (s *Service) OptionalDomainRequest(r *http.Request) (*http.Request, error) {
	if s == nil || r == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		return nil, errInput
	}
	if err := domainInput(r, false); err != nil {
		return nil, err
	}
	sid, err := cookieID(r)
	if errors.Is(err, errMissing) {
		return cleanDomainRequest(r), nil
	}
	if err != nil {
		return nil, err
	}
	out, err := s.DomainRequest(r, false)
	if err == nil {
		return out, nil
	}
	if !optionalDomainFallback(err) {
		return nil, err
	}
	// A valid preauth record is positive evidence of an anonymous/login-pending
	// flow. Missing/expired records and authority outages never cause fallback.
	preauth, err := s.loadPreauth(r.Context(), sid)
	if err != nil {
		return nil, err
	}
	if err = optionalPreauthState(preauth.State); err != nil {
		return nil, err
	}
	return cleanDomainRequest(r), nil
}

func optionalDomainFallback(err error) bool {
	var fault Fault
	return errors.As(err, &fault) && fault.Code == "SESSION_UNAUTHORIZED"
}

func optionalPreauthState(state string) error {
	switch state {
	case "ANONYMOUS", "PASSWORD_PENDING", "TWO_FA", "TWO_FA_PENDING",
		"DISCORD_START_PENDING", "DISCORD_CALLBACK", "DISCORD_CALLBACK_PENDING",
		"DISCORD_TWO_FA", "DISCORD_TWO_FA_PENDING":
		return nil
	case "COMMITTED":
		return errConflict // Bootstrap owns recovery of the committed upgrade.
	default:
		return errUnauthorized
	}
}
