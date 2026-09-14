package bffauth

import (
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/session"
)

func TestAuthChainIDIsStableUniqueAndDomainSeparated(t *testing.T) {
	master := [32]byte{1, 2, 3}
	key := deriveAuthChainKey(master)
	if key == master || key == [32]byte{} {
		t.Fatal("auth-chain key was not domain-derived")
	}
	s := &Service{origin: "https://fixture.invalid", authChainKey: key}
	binding := session.BFFBinding{UserID: "12", NativeSessionIDHash: strings.Repeat("a", 64)}
	id, err := s.authChainID(binding)
	if err != nil || !opaque(id) {
		t.Fatalf("opaque auth-chain ID invalid: id=%q err=%v", id, err)
	}
	if id != "Z8tU-PPd-NCutDBck2JIZUcWMDIzJ-vBDbrjq0AXPn4" {
		t.Fatalf("auth-chain field encoding changed: %q", id)
	}
	repeat, err := s.authChainID(binding)
	if err != nil || repeat != id {
		t.Fatal("same verified binding did not produce a stable auth-chain ID")
	}
	changed := []struct {
		name    string
		service *Service
		binding session.BFFBinding
	}{
		{"native chain", s, session.BFFBinding{UserID: "12", NativeSessionIDHash: strings.Repeat("b", 64)}},
		{"user", s, session.BFFBinding{UserID: "13", NativeSessionIDHash: binding.NativeSessionIDHash}},
		{"origin", &Service{origin: "https://other.invalid", authChainKey: key}, binding},
		{"master key", &Service{origin: s.origin, authChainKey: deriveAuthChainKey([32]byte{4, 5, 6})}, binding},
	}
	for _, tc := range changed {
		t.Run(tc.name, func(t *testing.T) {
			next, e := tc.service.authChainID(tc.binding)
			if e != nil || next == id {
				t.Fatalf("boundary input was not isolated: id=%q err=%v", next, e)
			}
		})
	}
	for _, invalid := range []session.BFFBinding{{}, {UserID: "0", NativeSessionIDHash: binding.NativeSessionIDHash}, {UserID: "12", NativeSessionIDHash: "not-a-digest"}} {
		if id, err = s.authChainID(invalid); err == nil || id != "" {
			t.Fatalf("invalid binding produced auth-chain ID %q", id)
		}
	}
}

func TestAuthenticatedBootstrapProjectsStableNonAuthorityChainID(t *testing.T) {
	s := &Service{origin: "https://fixture.invalid", authChainKey: deriveAuthChainKey([32]byte{7})}
	binding := session.BFFBinding{UserID: "12", NativeSessionIDHash: strings.Repeat("c", 64)}
	view := session.View{
		UserID:             "12",
		AuthChainStartedAt: time.Unix(100, 0).UTC(),
		AbsoluteExpiresAt:  time.Unix(300, 0).UTC(),
		IdleExpiresAt:      time.Unix(200, 0).UTC(),
		CSRFToken:          strings.Repeat("d", 43),
	}
	first, err := s.authenticatedBootstrap(view, binding, PasswordProvider{State: "AVAILABLE"}, "cookie")
	if err != nil || first.Session == nil || !opaque(first.Session.AuthChainID) {
		t.Fatalf("authenticated bootstrap chain projection failed: %+v err=%v", first.Session, err)
	}
	rotated := view
	rotated.CSRFToken = strings.Repeat("e", 43)
	second, err := s.authenticatedBootstrap(rotated, binding, PasswordProvider{State: "AVAILABLE"}, "")
	if err != nil || second.Session == nil || second.Session.AuthChainID != first.Session.AuthChainID {
		t.Fatal("same Native chain changed auth-chain ID during CSRF rotation")
	}
	other, err := s.authenticatedBootstrap(view, session.BFFBinding{UserID: "12", NativeSessionIDHash: strings.Repeat("f", 64)}, PasswordProvider{State: "AVAILABLE"}, "")
	if err != nil || other.Session == nil || other.Session.AuthChainID == first.Session.AuthChainID {
		t.Fatal("new Native chain reused projected auth-chain ID")
	}
	if _, err = s.authenticatedBootstrap(view, session.BFFBinding{UserID: "13", NativeSessionIDHash: binding.NativeSessionIDHash}, PasswordProvider{State: "AVAILABLE"}, ""); err == nil {
		t.Fatal("mismatched session view and BFF binding were projected")
	}
}
