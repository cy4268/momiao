package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/nativeself"
	"github.com/cy4268/momiao/internal/poker/authbridge"
	"github.com/cy4268/momiao/internal/poker/connectticket"
	pt "github.com/cy4268/momiao/internal/poker/transport"
)

// These ports are deliberately not mounted here. Composition must use the real
// native Unix transport, authbridge.Authority, issuer and verifier together.
func newPokerHTTPAuth(native http.RoundTripper, bind func(context.Context, authbridge.NativeRef) (connectticket.Session, error)) func(*http.Request) (pt.Principal, error) {
	return func(r *http.Request) (pt.Principal, error) {
		if r == nil || native == nil || bind == nil {
			return pt.Principal{}, pokerAuthFault(connectticket.ErrUnavailable)
		}
		if !nativeself.SessionCredential(r) {
			return pt.Principal{}, &pt.Fault{Status: 401, Code: "POKER_AUTH_UNAUTHORIZED"}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		verifyCtx, verifyCancel := context.WithTimeout(ctx, 2*time.Second)
		self, status := readNativeSelf(r.WithContext(verifyCtx), native)
		verifyCancel()
		if status != 0 {
			if status == 401 || status == 403 {
				return pt.Principal{}, &pt.Fault{Status: status, Code: "POKER_AUTH_UNAUTHORIZED"}
			}
			return pt.Principal{}, pokerAuthFault(connectticket.ErrUnavailable)
		}
		if self.Status == nil || *self.Status != 1 {
			return pt.Principal{}, pokerAuthFault(connectticket.ErrRevoked)
		}
		// Only after the unchanged credential's signature and live native session
		// were checked do we derive its reference. Claims alone never authenticate.
		parts := strings.Split(strings.Split(r.Header.Get("Authorization"), " ")[1], ".")
		payload, e := base64.RawURLEncoding.DecodeString(parts[1])
		if e != nil {
			return pt.Principal{}, pokerAuthFault(connectticket.ErrInvalid)
		}
		var claims map[string]json.RawMessage
		if json.Unmarshal(payload, &claims) != nil {
			return pt.Principal{}, pokerAuthFault(connectticket.ErrInvalid)
		}
		var version uint64
		if json.Unmarshal(claims["sv"], &version) != nil || version == 0 || version > pt.MaxSafeInteger {
			return pt.Principal{}, pokerAuthFault(connectticket.ErrInvalid)
		}
		hash := sha256.Sum256([]byte(r.Header.Get("X-Auth-Session")))
		ref := authbridge.NativeRef{UserID: strconv.FormatInt(self.ID, 10), SessionIDHash: hex.EncodeToString(hash[:]), SessionVersion: version}
		s, e := bind(ctx, ref)
		if e != nil {
			return pt.Principal{}, pokerAuthFault(e)
		}
		if s.UserID != ref.UserID || s.SessionIDHash != ref.SessionIDHash || s.SessionVersion != ref.SessionVersion || s.SecurityEpoch > pt.MaxSafeInteger {
			return pt.Principal{}, pokerAuthFault(connectticket.ErrUnavailable)
		}
		// HTTP authentication has no socket or ticket scope. The requested mint
		// intent is separately explicit; takeover resolves the live target socket.
		return pokerPrincipal(s, connectticket.ReadOnly), nil
	}
}
func pokerPrincipal(s connectticket.Session, intent string) pt.Principal {
	id, _ := strconv.ParseInt(s.UserID, 10, 64)
	return pt.Principal{UserID: id, SessionIDHash: s.SessionIDHash, SessionVersion: s.SessionVersion, SecurityEpoch: s.SecurityEpoch, ControlIntent: intent}
}
func pokerSession(p pt.Principal) connectticket.Session {
	return connectticket.Session{UserID: strconv.FormatInt(p.UserID, 10), SessionIDHash: p.SessionIDHash, SessionVersion: p.SessionVersion, SecurityEpoch: p.SecurityEpoch}
}
func pokerLiveSession(check connectticket.SessionCheck) func(context.Context, pt.Principal) error {
	return func(ctx context.Context, p pt.Principal) error {
		if check == nil {
			return pokerAuthFault(connectticket.ErrUnavailable)
		}
		if e := check(ctx, pokerSession(p)); e != nil {
			return pokerAuthFault(e)
		}
		return nil
	}
}
func pokerTicketMint(issuer *connectticket.Issuer) func(context.Context, pt.Principal, pt.TicketRequest) (json.RawMessage, error) {
	return func(ctx context.Context, p pt.Principal, r pt.TicketRequest) (json.RawMessage, error) {
		if issuer == nil {
			return nil, pokerAuthFault(connectticket.ErrUnavailable)
		}
		token, e := issuer.Issue(ctx, connectticket.MintRequest{Session: pokerSession(p), TargetTableID: r.TargetTableID, ControlIntent: r.ControlIntent})
		if e != nil {
			return nil, pokerAuthFault(e)
		}
		return json.Marshal(struct {
			Ticket string `json:"poker_connect_ticket"`
		}{token})
	}
}
func pokerTicketAuthentication(verifier *connectticket.Verifier) func(context.Context, pt.ConnectRequest) (pt.Principal, error) {
	return func(ctx context.Context, r pt.ConnectRequest) (pt.Principal, error) {
		if verifier == nil {
			return pt.Principal{}, pokerAuthFault(connectticket.ErrUnavailable)
		}
		c, e := verifier.Accept(ctx, r.PokerConnectTicket, r.TableID)
		if e != nil {
			return pt.Principal{}, pokerAuthFault(e)
		}
		return pokerPrincipal(c.Session, c.ControlIntent), nil
	}
}
func pokerAuthFault(e error) *pt.Fault {
	switch {
	case errors.Is(e, connectticket.ErrInvalid):
		return &pt.Fault{Status: 400, Code: "POKER_TICKET_INVALID"}
	case errors.Is(e, connectticket.ErrRevoked):
		return &pt.Fault{Status: 401, Code: "POKER_SESSION_REVOKED"}
	case errors.Is(e, connectticket.ErrExpired):
		return &pt.Fault{Status: 401, Code: "POKER_TICKET_EXPIRED"}
	case errors.Is(e, connectticket.ErrReplay):
		return &pt.Fault{Status: 409, Code: "TICKET_REPLAYED"}
	case errors.Is(e, connectticket.ErrRestart):
		return &pt.Fault{Status: 409, Code: "POKER_TICKET_RESTART_FENCED"}
	default:
		return &pt.Fault{Status: 503, Code: "POKER_AUTH_UNAVAILABLE"}
	}
}
