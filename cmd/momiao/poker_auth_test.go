package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker/connectticket"
	pt "github.com/cy4268/momiao/internal/poker/transport"
)

func TestPokerHTTPAuthRequiresLocalContext(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/poker/connect-tickets", nil)
	req.Header.Set("Authorization", "Bearer forged")
	req.Header.Set("New-Api-User", "9007199254740993")
	req.Header.Set("X-Auth-Session", "session")
	if _, err := pokerHTTPAuth(req); err == nil {
		t.Fatal("native headers established a local Poker identity")
	}
	principal, err := pokerHTTPAuth(announcementReq("POST", "/api/v1/poker/connect-tickets", ""))
	if err != nil || principal.UserID != 9007199254740993 || principal.SessionIDHash != strings.Repeat("a", 64) {
		t.Fatal("local user/session context not preserved", principal, err)
	}
}

func TestPokerTicketPortsUseSignedOneUseEnvelopeWithoutHTTPGrant(t *testing.T) {
	pub, priv, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	s := connectticket.Session{UserID: "42", SessionIDHash: strings.Repeat("a", 64), SessionVersion: 3, SecurityEpoch: 2}
	active := true
	check := func(_ context.Context, got connectticket.Session) error {
		if !active || got != s {
			return connectticket.ErrRevoked
		}
		return nil
	}
	issuer, e := connectticket.NewIssuer(connectticket.IssuerOptions{KeyID: "local-test", PrivateKey: priv, CheckSession: check})
	if e != nil {
		t.Fatal(e)
	}
	// Crypto is real; this test labels the external atomic-consumer port as a
	// synthetic one-use fixture. Real Redis evidence belongs to connectticket.
	used := false
	verifier, e := connectticket.NewVerifier(connectticket.VerifierOptions{PublicKeys: map[string]ed25519.PublicKey{"local-test": pub}, StartedAt: time.Now().Add(-time.Minute), CheckSession: check, Consume: func(context.Context, [32]byte, time.Time) (bool, error) {
		if used {
			return false, nil
		}
		used = true
		return true, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	p := pt.Principal{UserID: 42, SessionIDHash: s.SessionIDHash, SessionVersion: 3, SecurityEpoch: 2, ControlIntent: connectticket.ReadOnly}
	table := "019a0000-0000-7000-8000-000000000001"
	raw, e := pokerTicketMint(issuer)(context.Background(), p, pt.TicketRequest{TargetTableID: &table, ControlIntent: connectticket.ClaimControl})
	if e != nil {
		t.Fatal(e)
	}
	var data map[string]string
	if json.Unmarshal(raw, &data) != nil || len(data) != 1 || !strings.HasPrefix(data["poker_connect_ticket"], "ct1.") {
		t.Fatal("mint data contract")
	}
	got, e := pokerTicketAuthentication(verifier)(context.Background(), pt.ConnectRequest{PokerConnectTicket: data["poker_connect_ticket"], TableID: table})
	if e != nil || got.UserID != 42 || got.SessionVersion != 3 || got.SecurityEpoch != 2 || got.ControlIntent != connectticket.ClaimControl {
		t.Fatal("signed claims not bound exactly", got, e)
	}
	if _, e = pokerTicketAuthentication(verifier)(context.Background(), pt.ConnectRequest{PokerConnectTicket: data["poker_connect_ticket"], TableID: table}); e == nil {
		t.Fatal("ticket replay passed")
	}
	if e = pokerLiveSession(check)(context.Background(), got); e != nil {
		t.Fatal(e)
	}
	active = false
	if e = pokerLiveSession(check)(context.Background(), got); e == nil {
		t.Fatal("live revoke ignored")
	}
	if _, e = pokerTicketMint(issuer)(context.Background(), p, pt.TicketRequest{TargetTableID: &table, ControlIntent: connectticket.ClaimControl}); e == nil {
		t.Fatal("mint after revoke accepted")
	}
}
