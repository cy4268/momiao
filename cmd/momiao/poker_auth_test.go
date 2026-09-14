package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker/authbridge"
	"github.com/cy4268/momiao/internal/poker/connectticket"
	pt "github.com/cy4268/momiao/internal/poker/transport"
)

func TestPokerHTTPAuthVerifiesNativeBeforeBinding(t *testing.T) {
	req := announcementReq("POST", "/api/v1/poker/connect-tickets", "")
	verified := false
	nativeStatus := 200
	bindCalls := 0
	native := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "http://unix/api/user/self" || r.Host != "localhost" || r.Header.Get("Authorization") != req.Header.Get("Authorization") || r.Header.Get("X-Auth-Session") != "fixture-session" || r.Header.Get("Cookie") != "" {
			t.Fatal("original native credential was not checked at fixed boundary")
		}
		verified = nativeStatus == 200
		return &http.Response{StatusCode: nativeStatus, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"id":9007199254740993,"status":1}}`))}, nil
	})
	bind := func(_ context.Context, r authbridge.NativeRef) (connectticket.Session, error) {
		bindCalls++
		if !verified {
			t.Fatal("unverified native claim reached binding")
		}
		hash := sha256.Sum256([]byte("fixture-session"))
		if r.UserID != "9007199254740993" || r.SessionVersion != 1 || r.SessionIDHash != hex.EncodeToString(hash[:]) {
			t.Fatal("native reference was rounded or not hashed", r)
		}
		return connectticket.Session{UserID: r.UserID, SessionIDHash: r.SessionIDHash, SessionVersion: r.SessionVersion, SecurityEpoch: 7}, nil
	}
	auth := newPokerHTTPAuth(native, bind)
	p, e := auth(req)
	if e != nil || p.UserID != 9007199254740993 || p.SecurityEpoch != 7 || p.ControlIntent != connectticket.ReadOnly {
		t.Fatal("verified principal", p, e)
	}
	for _, status := range []int{401, 403, 502} {
		nativeStatus = status
		verified = false
		if _, e = auth(req); e == nil {
			t.Fatal("native failure granted principal")
		}
	}
	if bindCalls != 1 {
		t.Fatal("failed native verification entered binder")
	}
	nativeStatus = 200
	for _, change := range []func(*http.Request){func(r *http.Request) { r.Header.Set("Authorization", "Bearer opaque-pat") }, func(r *http.Request) { r.Header.Set("X-Auth-Session", "different") }, func(r *http.Request) { r.Header.Set("New-Api-User", "1") }} {
		r := req.Clone(context.Background())
		change(r)
		if _, e = auth(r); e == nil {
			t.Fatal("invalid credential accepted")
		}
	}
	if bindCalls != 1 {
		t.Fatal("credential gate lost")
	}
}

func TestPokerHTTPAuthFailsClosedOnBindingDrift(t *testing.T) {
	req := announcementReq("GET", "/api/v1/poker/lobby", "")
	native := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"id":9007199254740993,"status":1}}`))}, nil
	})
	for _, bind := range []func(context.Context, authbridge.NativeRef) (connectticket.Session, error){
		func(context.Context, authbridge.NativeRef) (connectticket.Session, error) {
			return connectticket.Session{}, errors.New("private-password")
		},
		func(context.Context, authbridge.NativeRef) (connectticket.Session, error) {
			return connectticket.Session{UserID: "42", SessionVersion: 1}, nil
		},
	} {
		_, e := newPokerHTTPAuth(native, bind)(req)
		if e == nil || strings.Contains(e.Error(), "private-password") {
			t.Fatal("binding drift/error", e)
		}
	}
	if _, e := newPokerHTTPAuth(nil, nil)(req); e == nil {
		t.Fatal("missing provider accepted")
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
