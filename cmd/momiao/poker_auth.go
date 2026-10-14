package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/poker/connectticket"
	pt "github.com/cy4268/momiao/internal/poker/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The portal middleware has already checked the local cookie, user and CSRF.
func pokerHTTPAuth(r *http.Request) (pt.Principal, error) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		return pt.Principal{}, &pt.Fault{Status: 401, Code: "POKER_AUTH_UNAUTHORIZED"}
	}
	return pt.Principal{UserID: user.ID, SessionIDHash: identity.SessionIDFromContext(r.Context()), SessionVersion: 1, ControlIntent: connectticket.ReadOnly}, nil
}

// Existing ticket/control fields remain 1/0; revocation depends only on the
// shared Redis session and the live local user, not versions or native bindings.
type pokerSessionAuthority struct {
	sessions identity.Sessions
	pool     *pgxpool.Pool
}

func (a pokerSessionAuthority) Check(ctx context.Context, s connectticket.Session) error {
	_, err := a.CheckUntil(ctx, s)
	return err
}

func (a pokerSessionAuthority) CheckUntil(ctx context.Context, s connectticket.Session) (time.Time, error) {
	userID, err := strconv.ParseInt(s.UserID, 10, 64)
	if err != nil || userID <= 0 {
		return time.Time{}, connectticket.ErrInvalid
	}
	deadline, err := a.sessions.ExpiresAt(ctx, s.SessionIDHash, userID)
	if errors.Is(err, identity.ErrSessionExpired) {
		return time.Time{}, connectticket.ErrRevoked
	}
	if err != nil {
		return time.Time{}, connectticket.ErrUnavailable
	}
	var enabled bool
	err = a.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM identity.users WHERE id=$1 AND status=1 AND deleted_at IS NULL)`, userID).Scan(&enabled)
	if err != nil {
		return time.Time{}, connectticket.ErrUnavailable
	}
	if !enabled || !deadline.After(time.Now()) {
		return time.Time{}, connectticket.ErrRevoked
	}
	return deadline, nil
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
