package bffauth

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOptionalDomainPublicBoundary(t *testing.T) {
	s := &Service{}
	r := httptest.NewRequest("GET", "https://fixture.invalid/platform/v1/announcements", nil)
	r.Header.Set("Cookie", "unrelated=value")
	r.Header.Set("X-CSRF-Token", "do-not-project")
	out, err := s.OptionalDomainRequest(r)
	if err != nil || out == nil {
		t.Fatal("absent platform cookie should preserve public browsing")
	}
	for _, key := range []string{"Cookie", "Authorization", "New-Api-User", "X-Auth-Session", "X-CSRF-Token"} {
		if len(out.Header.Values(key)) != 0 {
			t.Errorf("public projection contains %s", key)
		}
	}
	for _, method := range []string{"POST", "DELETE", "OPTIONS"} {
		r.Method = method
		if out, err := s.OptionalDomainRequest(r); err == nil || out != nil {
			t.Errorf("optional authentication allowed %s", method)
		}
	}
	r.Method = "GET"
	r.Header.Set("Cookie", CookieName+"=malformed")
	if out, err := s.OptionalDomainRequest(r); err == nil || out != nil {
		t.Error("malformed session downgraded to public")
	}
}

func TestOptionalDomainPreauthStates(t *testing.T) {
	for _, state := range []string{"ANONYMOUS", "PASSWORD_PENDING", "TWO_FA", "TWO_FA_PENDING"} {
		if optionalPreauthState(state) != nil {
			t.Errorf("valid preauth state rejected: %s", state)
		}
	}
	for _, state := range []string{"COMMITTED", "CANCELLED", "UNKNOWN", "", "AUTHENTICATED"} {
		if optionalPreauthState(state) == nil {
			t.Errorf("uncertain or upgraded state downgraded: %s", state)
		}
	}
}

func TestOptionalDomainFallbackRejectsAuthenticatedCSRFFailure(t *testing.T) {
	if !optionalDomainFallback(Fault{Status: 401, Code: "SESSION_UNAUTHORIZED", ClearCookie: true}) {
		t.Fatal("verified preauth candidate was not classified for explicit state validation")
	}
	for _, err := range []error{
		Fault{Status: 403, Code: "SESSION_CSRF_FAILED"},
		Fault{Status: 401, Code: "AUTH_UNAUTHORIZED", ClearCookie: true},
		errors.New("fixture failure"),
	} {
		if optionalDomainFallback(err) {
			t.Fatalf("authenticated failure downgraded to public: %v", err)
		}
	}
}

func TestNativeUICallbackOwnershipIsBoundToSurfaceAndState(t *testing.T) {
	state := strings.Repeat("s", 43)
	record := preauthRecord{
		State:           "DISCORD_CALLBACK",
		Purpose:         DiscordPurposeLogin,
		AuthSurface:     nativeUIAuthSurface,
		NativeStateHash: hashText(state),
	}
	input := DiscordCallbackInput{Code: "discord-code", State: state}
	if !nativeUICallbackOwned(record, input) {
		t.Fatal("matching Native UI callback was not owned")
	}
	for name, changed := range map[string]preauthRecord{
		"legacy platform flow":  {State: record.State, Purpose: record.Purpose, NativeStateHash: record.NativeStateHash},
		"account proof flow":    {State: record.State, Purpose: DiscordPurposeFresh, AuthSurface: nativeUIAuthSurface, NativeStateHash: record.NativeStateHash},
		"wrong callback phase":  {State: "ANONYMOUS", Purpose: record.Purpose, AuthSurface: nativeUIAuthSurface, NativeStateHash: record.NativeStateHash},
		"different OAuth state": {State: record.State, Purpose: record.Purpose, AuthSurface: nativeUIAuthSurface, NativeStateHash: hashText(strings.Repeat("x", 43))},
	} {
		if nativeUICallbackOwned(changed, input) {
			t.Errorf("%s was incorrectly routed to the Native UI", name)
		}
	}
	clearDiscord(&record, "CANCELLED")
	if record.AuthSurface != "" || nativeUICallbackOwned(record, input) {
		t.Fatal("completed or cancelled callback retained Native UI ownership")
	}
}
