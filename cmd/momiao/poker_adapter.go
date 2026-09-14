package main

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cy4268/momiao/internal/poker"
	"github.com/cy4268/momiao/internal/poker/engine"
	pt "github.com/cy4268/momiao/internal/poker/transport"
)

type pokerAppSocket struct {
	mu        sync.Mutex
	closed    atomic.Bool
	principal pt.Principal
	ref       pt.ConnectionRef
	order     uint64
}

// This registry retains original transport identities, not controller grants.
// G3 remains the only authority for the PG epoch and current Redis owner.
type pokerAdapter struct {
	service     *poker.Service
	mu          sync.Mutex
	next        uint64
	sockets     map[string]*pokerAppSocket
	wakeControl func(pt.ConnectionRef)
}

func (a *pokerAdapter) connected(ctx context.Context, p pt.Principal, ref pt.ConnectionRef) error {
	entry := &pokerAppSocket{principal: p, ref: ref}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	a.mu.Lock()
	if a.sockets == nil {
		a.sockets = map[string]*pokerAppSocket{}
	}
	if a.sockets[ref.ID] != nil || len(a.sockets) >= 1024 || a.next == ^uint64(0) {
		a.mu.Unlock()
		return pokerDomainError(poker.ErrDenied)
	}
	a.next++
	entry.order = a.next
	a.sockets[ref.ID] = entry
	a.mu.Unlock()
	_, err := a.service.Connected(ctx, pokerIdentity(p, ref), p.ControlIntent == "CLAIM_CONTROL")
	if err != nil {
		entry.closed.Store(true)
		a.mu.Lock()
		if a.sockets[ref.ID] == entry {
			delete(a.sockets, ref.ID)
		}
		a.mu.Unlock()
		// Connected may have registered a ref before a later store failure.
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = a.service.Disconnected(cleanup, pokerIdentity(p, ref))
		cancel()
	}
	return pokerDomainError(err)
}

func (a *pokerAdapter) disconnected(ctx context.Context, p pt.Principal, ref pt.ConnectionRef) {
	a.mu.Lock()
	entry := a.sockets[ref.ID]
	if entry != nil && entry.ref == ref && samePokerAuth(entry.principal, p) {
		entry.closed.Store(true)
		delete(a.sockets, ref.ID)
	} else {
		entry = nil
	}
	a.mu.Unlock()
	if entry == nil {
		return
	}
	// A close is marked before waiting for an in-progress rebind; the domain
	// disconnect therefore always follows any such registration, never before it.
	entry.mu.Lock()
	defer entry.mu.Unlock()
	_ = a.service.Disconnected(ctx, pokerIdentity(p, ref))
}

func (a *pokerAdapter) rebindAfterBuyIn(ctx context.Context, p pt.Principal, table string) {
	a.mu.Lock()
	entries := make([]*pokerAppSocket, 0)
	for _, entry := range a.sockets {
		if entry.ref.TableID == table && entry.principal.ControlIntent == "CLAIM_CONTROL" && samePokerAuth(entry.principal, p) && !entry.closed.Load() {
			entries = append(entries, entry)
		}
	}
	a.mu.Unlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].order < entries[j].order })
	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		entry.mu.Lock()
		if !entry.closed.Load() {
			_, _ = a.service.Connected(ctx, pokerIdentity(entry.principal, entry.ref), true)
			// Even assignment failure may have updated the read-only session ref.
			// This only requests a fresh projection; it carries no grant or receipt.
			if a.wakeControl != nil {
				a.wakeControl(entry.ref)
			}
		}
		entry.mu.Unlock()
	}
}

func samePokerAuth(a, b pt.Principal) bool {
	return a.UserID == b.UserID && a.SessionIDHash == b.SessionIDHash && a.SessionVersion == b.SessionVersion && a.SecurityEpoch == b.SecurityEpoch
}

func pokerIdentity(p pt.Principal, c pt.ConnectionRef) poker.ControlRef {
	return poker.ControlRef{UserID: p.UserID, SessionIDHash: p.SessionIDHash, SessionVersion: p.SessionVersion, SecurityEpoch: p.SecurityEpoch, ConnectionID: c.ID, TableID: c.TableID}
}
func pokerAuth(p pt.Principal) poker.AuthSession {
	return poker.AuthSession{UserID: p.UserID, SessionIDHash: p.SessionIDHash, SessionVersion: p.SessionVersion, SecurityEpoch: p.SecurityEpoch}
}
func pokerDomainControl(c pt.ControlRef) poker.ControlRef {
	return poker.ControlRef{UserID: c.UserID, SessionIDHash: c.SessionIDHash, SessionVersion: c.SessionVersion, SecurityEpoch: c.SecurityEpoch, ConnectionID: c.ConnectionID, TableID: c.TableID, SessionID: c.SessionID, RuntimeEpoch: c.RuntimeEpoch, ControlEpoch: c.ControlEpoch}
}
func pokerTransportControl(c poker.ControlRef) pt.ControlRef {
	return pt.ControlRef{UserID: c.UserID, SessionIDHash: c.SessionIDHash, SessionVersion: c.SessionVersion, SecurityEpoch: c.SecurityEpoch, ConnectionID: c.ConnectionID, TableID: c.TableID, SessionID: c.SessionID, RuntimeEpoch: c.RuntimeEpoch, ControlEpoch: c.ControlEpoch}
}
func pokerSessionCommand(p pt.Principal, c pt.SessionAction) poker.SessionCommand {
	ref := pokerDomainControl(c.Control)
	out := poker.SessionCommand{UserID: p.UserID, Key: c.ActionID, TableID: c.TableID, Control: &ref, ExpectedTableVersion: c.ExpectedTableVersion, ExpectedHandVersion: c.ExpectedHandVersion}
	if c.HandID != nil {
		out.HandID = *c.HandID
	}
	return out
}

func pokerTransportSnapshot(v poker.TableView) (pt.Snapshot, error) {
	parse := func(raw string) (uint64, error) {
		n, e := strconv.ParseUint(raw, 10, 64)
		if e != nil || strconv.FormatUint(n, 10) != raw || n > pt.MaxSafeInteger {
			return 0, &pt.Fault{Status: 409, Code: "PROTOCOL_VERSION_OUT_OF_RANGE"}
		}
		return n, nil
	}
	version, e := parse(v.TableVersion)
	if e != nil {
		return pt.Snapshot{}, e
	}
	out := pt.Snapshot{TableID: v.TableID, TableVersion: version, RuntimeEpoch: v.RuntimeEpoch, ServerTime: v.ServerNow}
	if v.Hand != nil {
		hv, e := parse(v.Hand.HandVersion)
		if e != nil {
			return pt.Snapshot{}, e
		}
		id := v.Hand.HandID
		out.HandID = &id
		out.HandVersion = &hv
	}
	if c := v.Viewer.Control; c != nil {
		out.Control = &pt.ControlView{ConnectionID: c.ConnectionID, SessionID: c.SessionID, Mode: c.Mode, ControlEpoch: c.ControlEpoch}
	}
	out.Payload, e = boundedPokerSnapshot(v)
	if e != nil {
		return pt.Snapshot{}, &pt.Fault{Status: 500, Code: "POKER_INVALID_SNAPSHOT"}
	}
	return out, nil
}

const pokerSnapshotPayloadBudget = 60 << 10

// The wire contract is bounded at 64 KiB. Chat and owner-only target history
// are projections, so retain the newest/useful suffixes and state explicitly
// when they were shortened. Seats, hand state and control authority are never
// discarded to make a frame fit.
func boundedPokerSnapshot(v poker.TableView) (json.RawMessage, error) {
	if v.Chat != nil {
		chat := *v.Chat
		chat.Messages = append([]poker.ChatMessageView{}, chat.Messages...)
		v.Chat = &chat
	}
	if v.Host != nil {
		host := *v.Host
		host.Players = append([]poker.HostPlayerView{}, host.Players...)
		host.Spectators = append([]poker.HostSpectatorView{}, host.Spectators...)
		host.ChatTargets = append([]poker.HostChatTargetView{}, host.ChatTargets...)
		v.Host = &host
	}
	v.Timeline.Events = append([]poker.TimelineEventView{}, v.Timeline.Events...)
	for {
		payload, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		if len(payload) <= pokerSnapshotPayloadBudget {
			return payload, nil
		}
		switch {
		case v.Chat != nil && len(v.Chat.Messages) > 0:
			v.Chat.Truncated = true
			v.Chat.Messages = v.Chat.Messages[1:]
		case v.Host != nil && len(v.Host.ChatTargets) > 0:
			v.Host.ChatTargetsTruncated = true
			v.Host.ChatTargets = v.Host.ChatTargets[:len(v.Host.ChatTargets)-1]
		case v.Host != nil && len(v.Host.Spectators) > 0:
			v.Host.SpectatorsTruncated = true
			v.Host.Spectators = v.Host.Spectators[:len(v.Host.Spectators)-1]
		case len(v.Timeline.Events) > 0:
			v.Timeline.Truncated = true
			v.Timeline.Events = v.Timeline.Events[1:]
		default:
			return nil, errors.New("poker snapshot core exceeds transport budget")
		}
	}
}

func (a *pokerAdapter) snapshot(ctx context.Context, p pt.Principal, ref pt.ConnectionRef) (pt.Snapshot, error) {
	var view poker.TableView
	var err error
	if ref.ID == "" {
		view, err = a.service.ViewForSession(ctx, ref.TableID, pokerAuth(p))
	} else {
		view, err = a.service.ViewForConnection(ctx, pokerIdentity(p, ref))
	}
	if err != nil {
		return pt.Snapshot{}, pokerDomainError(err)
	}
	return pokerTransportSnapshot(view)
}

func (a *pokerAdapter) authorize(ctx context.Context, p pt.Principal, ref pt.ConnectionRef, epoch uint64) (pt.ControlRef, error) {
	control, err := a.service.ResolveControl(ctx, pokerIdentity(p, ref), epoch)
	return pokerTransportControl(control), pokerDomainError(err)
}

func (a *pokerAdapter) ports() pt.Ports {
	s := a.service
	return pt.Ports{
		ReadLobby: func(ctx context.Context, p pt.Principal) (json.RawMessage, error) {
			return pokerJSON(s.LobbyForSession(ctx, pokerAuth(p), poker.LobbyFilter{}))
		},
		ReadTables: func(ctx context.Context, p pt.Principal, q pt.LobbyQuery) (json.RawMessage, error) {
			return pokerJSON(s.LobbyForSession(ctx, pokerAuth(p), poker.LobbyFilter{Query: q.Query, AccessMode: q.AccessMode, BlindPreset: q.BlindPreset, LifecycleState: q.LifecycleState, Sort: q.Sort, Cursor: q.Cursor, OpenSeatsOnly: q.OpenSeatsOnly, SpectatorsOnly: q.SpectatorsOnly, MaxSeats: q.MaxSeats, Limit: q.Limit}))
		},
		ReadActiveSession: func(ctx context.Context, p pt.Principal) (json.RawMessage, error) {
			return pokerJSON(s.ActiveSession(ctx, p.UserID))
		},
		ReadSession: func(ctx context.Context, p pt.Principal, id string) (json.RawMessage, error) {
			return pokerJSON(s.Session(ctx, p.UserID, id))
		},
		ReadFairness: func(ctx context.Context, p pt.Principal, id string) (json.RawMessage, error) {
			return pokerJSON(s.Fairness(ctx, id, p.UserID))
		},
		ReadReceipt: func(ctx context.Context, p pt.Principal, table string, q pt.ReceiptQueryRequest) (json.RawMessage, error) {
			return pokerJSON(s.LookupReceipt(ctx, p.UserID, poker.ReceiptQuery{TableID: table, Kind: q.Kind, MutationID: q.MutationID}))
		},
		ReadEntryReceipt: func(ctx context.Context, p pt.Principal, _ string, q pt.EntryReceiptRequest) (json.RawMessage, error) {
			var table *string
			if len(q.TableID) != 0 {
				if err := json.Unmarshal(q.TableID, &table); err != nil {
					return nil, pokerDomainError(poker.ErrInvalid)
				}
			}
			return pokerJSON(s.LookupEntryReceipt(ctx, p.UserID, poker.EntryReceiptQuery{Kind: q.Kind, MutationID: q.MutationID, TableID: table}))
		},
		ReadReservation: func(ctx context.Context, p pt.Principal, table string, q pt.ReservationQueryRequest) (json.RawMessage, error) {
			return pokerJSON(s.Reservation(ctx, p.UserID, table, q.ReservationID))
		},
		CreateTable: func(ctx context.Context, p pt.Principal, c pt.CreateTableRequest) (json.RawMessage, error) {
			password := ""
			if len(c.Password) != 0 && json.Unmarshal(c.Password, &password) != nil {
				return nil, pokerDomainError(poker.ErrInvalid)
			}
			return pokerJSON(s.CreateTable(ctx, poker.CreateTableCommand{UserID: p.UserID, Key: c.RequestID, Name: c.Name, BlindPreset: c.BlindPreset, MaxSeats: c.MaxSeats, AllowSpectators: c.AllowSpectators, AccessMode: c.AccessMode, ChatEnabled: c.ChatEnabled, Password: password, Auth: pokerAuth(p)}))
		},
		VerifyTableAccess: func(ctx context.Context, p pt.Principal, table string, c pt.AccessRequest) (json.RawMessage, error) {
			return pokerJSON(s.VerifyTableAccess(ctx, table, pokerAuth(p), c.Password))
		},
		ReserveSeat: func(ctx context.Context, p pt.Principal, table string, c pt.ReserveSeatRequest) (json.RawMessage, error) {
			return pokerJSON(s.ReserveSeat(ctx, poker.ReserveSeatCommand{UserID: p.UserID, Key: c.RequestID, TableID: table, Seat: c.SeatNo, Auth: pokerAuth(p)}))
		},
		BuyIn: func(ctx context.Context, p pt.Principal, table string, c pt.BuyInRequest) (json.RawMessage, error) {
			amount, e := pokerAmount(c.AmountUnits)
			if e != nil {
				return nil, e
			}
			r, e := s.BuyIn(ctx, poker.BuyInCommand{UserID: p.UserID, Key: c.RequestID, TableID: table, ReservationID: c.ReservationID, AmountUnits: amount, Auth: pokerAuth(p)})
			if e == nil && r.SessionID != "" && r.FailureCode == "" {
				a.rebindAfterBuyIn(ctx, p, table)
			}
			return pokerJSON(r, e)
		},
		TopUp: func(ctx context.Context, p pt.Principal, id string, c pt.TopUpRequest) (json.RawMessage, error) {
			v, e := s.Session(ctx, p.UserID, id)
			if e != nil {
				return nil, pokerDomainError(e)
			}
			amount, e := pokerAmount(c.AmountUnits)
			if e != nil {
				return nil, e
			}
			return pokerJSON(s.RequestTopUp(ctx, poker.TopUpCommand{UserID: p.UserID, Key: c.RequestID, TableID: v.TableID, TargetSessionID: v.SessionID, AmountUnits: amount}))
		},
		SafeLeave: func(ctx context.Context, p pt.Principal, id string, c pt.SessionRequest) (json.RawMessage, error) {
			v, e := s.Session(ctx, p.UserID, id)
			if e != nil {
				return nil, pokerDomainError(e)
			}
			return pokerJSON(s.RequestSafeLeave(ctx, poker.SessionCommand{UserID: p.UserID, Key: c.RequestID, TableID: v.TableID, TargetSessionID: v.SessionID}))
		},
		TakeOver: func(ctx context.Context, p pt.Principal, id string, c pt.TakeOverRequest) (json.RawMessage, error) {
			v, e := s.Session(ctx, p.UserID, id)
			if e != nil {
				return nil, pokerDomainError(e)
			}
			if c.Target.ID == "" || c.Target.ID != c.ConnectionID || c.Target.TableID != v.TableID {
				return nil, pokerDomainError(poker.ErrDenied)
			}
			r, e := s.TakeOver(ctx, poker.TakeOverCommand{RequestID: c.RequestID, SessionID: id, TargetConnectionID: c.Target.ID, Auth: poker.AuthSession{UserID: p.UserID, SessionIDHash: p.SessionIDHash, SessionVersion: p.SessionVersion, SecurityEpoch: p.SecurityEpoch}})
			if e != nil {
				return nil, pokerDomainError(e)
			}
			if r.Receipt == nil {
				return nil, &pt.Fault{Status: 500, Code: "POKER_INVALID_RECEIPT"}
			}
			return pokerJSON(r.Receipt, nil)
		},
		HostCommand: func(ctx context.Context, p pt.Principal, table string, c pt.HostRequest) (json.RawMessage, error) {
			if err := s.AuthorizeTable(ctx, table, pokerAuth(p)); err != nil {
				return nil, pokerDomainError(err)
			}
			var targetUser int64
			if c.TargetUserID != "" {
				var err error
				targetUser, err = strconv.ParseInt(c.TargetUserID, 10, 64)
				if err != nil || targetUser <= 0 || strconv.FormatInt(targetUser, 10) != c.TargetUserID {
					return nil, pokerDomainError(poker.ErrInvalid)
				}
			}
			return pokerJSON(s.HostControl(ctx, poker.HostCommand{UserID: p.UserID, Key: c.RequestID, TableID: table, Kind: c.Command, TargetSessionID: c.TargetSessionID, TargetUserID: targetUser}))
		},
		Act: func(ctx context.Context, p pt.Principal, c pt.HandAction) (json.RawMessage, error) {
			var amount int64
			if c.RequestedToUnits != nil {
				var e error
				amount, e = pokerAmount(*c.RequestedToUnits)
				if e != nil {
					return nil, e
				}
			}
			ref := pokerDomainControl(c.Control)
			return pokerJSON(s.Act(ctx, poker.ActCommand{UserID: p.UserID, Key: c.ActionID, TableID: c.TableID, HandID: c.HandID, ControlEpoch: c.ControlEpoch, HandVersion: c.ExpectedHandVersion, TableVersion: c.ExpectedTableVersion, Control: &ref, Kind: engine.ActionKind(c.ActionType), ToUnits: amount}))
		},
		SitOut: func(ctx context.Context, p pt.Principal, c pt.SessionAction) (json.RawMessage, error) {
			return pokerJSON(s.RequestSitOut(ctx, pokerSessionCommand(p, c)))
		},
		Resume: func(ctx context.Context, p pt.Principal, c pt.SessionAction) (json.RawMessage, error) {
			return pokerJSON(s.ResumeSeat(ctx, pokerSessionCommand(p, c)))
		},
		SetClientSeed: func(ctx context.Context, p pt.Principal, c pt.ClientSeed) (json.RawMessage, error) {
			session := pokerSessionCommand(p, pt.SessionAction{RequestID: c.RequestID, ActionID: c.ActionID, TableID: c.TableID, ExpectedTableVersion: c.ExpectedTableVersion, ExpectedHandVersion: c.ExpectedHandVersion, ControlEpoch: c.ControlEpoch, HandID: c.HandID, Control: c.Control})
			return pokerJSON(s.SetNextSeed(ctx, poker.NextSeedCommand{SessionCommand: session, Contribution: c.Seed}))
		},
		Chat: func(ctx context.Context, p pt.Principal, c pt.ChatMessage) (json.RawMessage, error) {
			return pokerJSON(s.SendChat(ctx, poker.ChatCommand{UserID: p.UserID, Key: c.RequestID, TableID: c.TableID, Body: c.Message, Connection: pokerIdentity(p, c.Connection)}))
		},
	}
}

func pokerAmount(raw string) (int64, error) {
	n, e := strconv.ParseInt(raw, 10, 64)
	if e != nil || n <= 0 || n%500000 != 0 || strconv.FormatInt(n, 10) != raw {
		return 0, pokerDomainError(poker.ErrInvalid)
	}
	return n, nil
}
func pokerJSON(value any, err error) (json.RawMessage, error) {
	if err != nil {
		return nil, pokerDomainError(err)
	}
	b, e := json.Marshal(value)
	if e != nil {
		return nil, &pt.Fault{Status: 500, Code: "POKER_INVALID_RECEIPT"}
	}
	return b, nil
}
func pokerDomainError(err error) error {
	if err == nil {
		return nil
	}
	status, code := 503, "POKER_SERVICE_UNAVAILABLE"
	switch {
	case errors.Is(err, poker.ErrInvalid), errors.Is(err, engine.ErrInvalid):
		status, code = 400, "POKER_INVALID_COMMAND"
	case errors.Is(err, poker.ErrDenied):
		status, code = 403, "POKER_COMMAND_DENIED"
	case errors.Is(err, poker.ErrTableAccessRequired):
		status, code = 403, "TABLE_ACCESS_REQUIRED"
	case errors.Is(err, poker.ErrTablePasswordInvalid):
		status, code = 403, "TABLE_PASSWORD_INVALID"
	case errors.Is(err, poker.ErrRateLimited):
		status, code = 429, "RATE_LIMITED"
	case errors.Is(err, poker.ErrChatDisabled):
		status, code = 409, "POKER_CHAT_DISABLED"
	case errors.Is(err, poker.ErrChatMuted):
		status, code = 403, "POKER_CHAT_MUTED"
	case errors.Is(err, poker.ErrNoControl):
		status, code = 403, "POKER_CONTROL_NOT_OWNED"
	case errors.Is(err, poker.ErrBusy):
		code = "POKER_TABLE_BUSY"
	case errors.Is(err, poker.ErrClosed):
		code = "POKER_SERVICE_CLOSED"
	case errors.Is(err, poker.ErrMaintenance):
		code = "MAINTENANCE_ACTIVE"
	case errors.Is(err, poker.ErrRulesetIncomplete):
		code = "POKER_RULESET_INCOMPLETE"
	case errors.Is(err, poker.ErrFenced):
		status, code = 409, "STALE_RUNTIME_EPOCH"
	case errors.Is(err, poker.ErrStaleVersion), errors.Is(err, engine.ErrStale):
		status, code = 409, "POKER_STALE_VERSION"
	case errors.Is(err, poker.ErrConflict), errors.Is(err, engine.ErrConflict):
		status, code = 409, "POKER_REQUEST_CONFLICT"
	case errors.Is(err, poker.ErrNeedsReview), errors.Is(err, poker.ErrCorruptSnapshot):
		status, code = 409, "POKER_HAND_NEEDS_REVIEW"
	case errors.Is(err, engine.ErrIllegal):
		status, code = 409, "POKER_ACTION_ILLEGAL"
	case errors.Is(err, engine.ErrDeadline):
		status, code = 409, "POKER_ACTION_DEADLINE"
	case errors.Is(err, engine.ErrRecovering):
		status, code = 409, "POKER_HAND_RECOVERING"
	}
	return &pt.Fault{Status: status, Code: code}
}
