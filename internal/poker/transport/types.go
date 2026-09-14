// Package transport is the Poker HTTP/WS boundary. Authentication and durable
// state are mandatory external authorities; this package owns neither SQL nor JWTs.
package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const Subprotocol = "chaldea-poker.v1"
const AuthDeadline = 10 * time.Second
const MaxSafeInteger uint64 = 9007199254740991

// Principal is server-only. All fields originate from verified authentication,
// never a browser-selected user ID or an unverified token claim.
type Principal struct {
	UserID         int64
	SessionIDHash  string
	SessionVersion uint64
	SecurityEpoch  uint64
	ControlIntent  string
}
type ConnectRequest struct{ PokerConnectTicket, TableID string }
type TicketRequest struct {
	TargetTableID *string `json:"target_table_id"`
	ControlIntent string  `json:"control_intent"`
}
type LobbyQuery struct {
	Query, AccessMode, BlindPreset, LifecycleState, Sort, Cursor string
	OpenSeatsOnly, SpectatorsOnly                                bool
	MaxSeats, Limit                                              int
}
type ReceiptQueryRequest struct {
	Kind       string `json:"kind"`
	MutationID string `json:"mutation_id"`
}
type EntryReceiptRequest struct {
	Kind       string          `json:"kind"`
	MutationID string          `json:"mutation_id"`
	TableID    json.RawMessage `json:"table_id"` // Preserve absent versus null in the strict union.
}
type ReservationQueryRequest struct {
	ReservationID string `json:"reservation_id"`
}
type CreateTableRequest struct {
	RequestID       string          `json:"request_id"`
	Name            string          `json:"name"`
	BlindPreset     string          `json:"blind_preset"`
	MaxSeats        int             `json:"max_seats"`
	AllowSpectators bool            `json:"allow_spectators"`
	AccessMode      string          `json:"access_mode,omitempty"`
	Password        json.RawMessage `json:"password,omitempty"`
	ChatEnabled     bool            `json:"chat_enabled"`
}
type AccessRequest struct {
	Password string `json:"password"`
}
type ReserveSeatRequest struct {
	RequestID string `json:"request_id"`
	SeatNo    int    `json:"seat_no"`
}
type BuyInRequest struct {
	RequestID     string `json:"request_id"`
	ReservationID string `json:"reservation_id"`
	AmountUnits   string `json:"amount_units"`
}
type TopUpRequest struct {
	RequestID   string `json:"request_id"`
	AmountUnits string `json:"amount_units"`
}
type SessionRequest struct {
	RequestID string `json:"request_id"`
}
type TakeOverRequest struct {
	RequestID    string `json:"request_id"`
	ConnectionID string `json:"connection_id"`
	// Resolved from the live server registry, never decoded from client JSON.
	Target ConnectionRef `json:"-"`
}
type HostRequest struct {
	RequestID       string `json:"request_id"`
	Command         string `json:"command"`
	TargetSessionID string `json:"target_session_id,omitempty"`
	TargetUserID    string `json:"target_user_id,omitempty"`
}
type HandAction struct {
	RequestID, ActionID, TableID, HandID                    string
	ExpectedTableVersion, ExpectedHandVersion, ControlEpoch uint64
	ActionType                                              string
	RequestedToUnits                                        *string
	Control                                                 ControlRef
}
type SessionAction struct {
	RequestID, ActionID, TableID                            string
	ExpectedTableVersion, ExpectedHandVersion, ControlEpoch uint64
	HandID                                                  *string
	Control                                                 ControlRef
}
type ClientSeed struct {
	RequestID, ActionID, TableID, Seed                      string
	ExpectedTableVersion, ExpectedHandVersion, ControlEpoch uint64
	HandID                                                  *string
	Control                                                 ControlRef
}
type ChatMessage struct {
	RequestID, TableID, Message string
	Connection                  ConnectionRef
}

// ConnectionRef is assigned by the server, never supplied by a browser. A
// CLAIM_CONTROL ticket requests a normal lease, not a takeover or ownership.
type ConnectionRef struct{ ID, TableID string }

// Only a verified preflight creates this tuple; the domain must revalidate it
// in the PG transaction that applies the command. It is never client JSON.
type ControlRef struct {
	UserID                           int64
	SessionIDHash                    string
	SessionVersion, SecurityEpoch    uint64
	ConnectionID, TableID, SessionID string
	RuntimeEpoch, ControlEpoch       uint64
}
type ControlView struct {
	ConnectionID string `json:"connection_id"`
	SessionID    string `json:"session_id,omitempty"`
	Mode         string `json:"mode"`
	ControlEpoch string `json:"control_epoch"`
}

// Snapshot must be a committed, viewer-specific projection read atomically.
// Versions describe Payload, not the older commit notification that triggered it.
// Payload is already-projected JSON, never an engine snapshot or fairness proof.
type Snapshot struct {
	TableID      string
	TableVersion uint64
	HandID       *string
	HandVersion  *uint64
	RuntimeEpoch uint64
	ServerTime   time.Time
	Payload      json.RawMessage
	// Required for WS and absent for HTTP; agrees with Payload.viewer.control.
	Control *ControlView
}

// Ports are narrow typed commands. Nil optional capabilities fail closed with
// POKER_CAPABILITY_UNAVAILABLE; no command is silently acknowledged or emulated.
type Ports struct {
	ReadLobby         func(context.Context, Principal) (json.RawMessage, error)
	ReadTables        func(context.Context, Principal, LobbyQuery) (json.RawMessage, error)
	ReadSession       func(context.Context, Principal, string) (json.RawMessage, error)
	ReadActiveSession func(context.Context, Principal) (json.RawMessage, error)
	ReadFairness      func(context.Context, Principal, string) (json.RawMessage, error)
	ReadReceipt       func(context.Context, Principal, string, ReceiptQueryRequest) (json.RawMessage, error)
	ReadEntryReceipt  func(context.Context, Principal, string, EntryReceiptRequest) (json.RawMessage, error)
	ReadReservation   func(context.Context, Principal, string, ReservationQueryRequest) (json.RawMessage, error)
	MintTicket        func(context.Context, Principal, TicketRequest) (json.RawMessage, error)
	CreateTable       func(context.Context, Principal, CreateTableRequest) (json.RawMessage, error)
	VerifyTableAccess func(context.Context, Principal, string, AccessRequest) (json.RawMessage, error)
	ReserveSeat       func(context.Context, Principal, string, ReserveSeatRequest) (json.RawMessage, error)
	BuyIn             func(context.Context, Principal, string, BuyInRequest) (json.RawMessage, error)
	TopUp             func(context.Context, Principal, string, TopUpRequest) (json.RawMessage, error)
	SafeLeave         func(context.Context, Principal, string, SessionRequest) (json.RawMessage, error)
	TakeOver          func(context.Context, Principal, string, TakeOverRequest) (json.RawMessage, error)
	HostCommand       func(context.Context, Principal, string, HostRequest) (json.RawMessage, error)
	Act               func(context.Context, Principal, HandAction) (json.RawMessage, error)
	SitOut            func(context.Context, Principal, SessionAction) (json.RawMessage, error)
	Resume            func(context.Context, Principal, SessionAction) (json.RawMessage, error)
	SetClientSeed     func(context.Context, Principal, ClientSeed) (json.RawMessage, error)
	Chat              func(context.Context, Principal, ChatMessage) (json.RawMessage, error)
}

type Options struct {
	Origin   string
	AuthHTTP func(*http.Request) (Principal, error)
	// AuthenticateTicket verifies ct1 Ed25519, issuer/audience/purpose, 60-second
	// expiry, process-start fence, live session/epochs, target and atomic replay.
	AuthenticateTicket func(context.Context, ConnectRequest) (Principal, error)
	ValidateSession    func(context.Context, Principal) error
	// The external control-lease owner binds a specific socket to the current
	// durable control epoch. Missing authorization fails closed for WS mutations.
	Connected         func(context.Context, Principal, ConnectionRef) error
	Disconnected      func(context.Context, Principal, ConnectionRef)
	AuthorizeControl  func(context.Context, Principal, ConnectionRef, uint64) (ControlRef, error)
	Snapshot          func(context.Context, Principal, ConnectionRef) (Snapshot, error)
	Ports             Ports
	SendQueueCapacity int
	MaxMessageBytes   int64
	WriteTimeout      time.Duration
	OperationTimeout  time.Duration
	MaxConnections    int
}

// Fault is the only adapter error exposed to clients. Unexpected errors are
// mapped to a fixed generic code; SQL, tickets and exception text stay private.
type Fault struct {
	Status int
	Code   string
}

func (e *Fault) Error() string { return e.Code }

var errInvalid = &Fault{400, "POKER_INVALID_COMMAND"}
var errUnavailable = &Fault{503, "POKER_CAPABILITY_UNAVAILABLE"}
var errProtocol = &Fault{400, "PROTOCOL_UNSUPPORTED_MESSAGE"}
var errVersion = &Fault{409, "PROTOCOL_VERSION_OUT_OF_RANGE"}

type Handler struct {
	opts        Options
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	subs        map[string]map[*subscriber]struct{}
	sequences   map[string]sequence
	connections int
	closed      bool
	live        map[string]*liveConnection
}
type sequence struct{ epoch, version, seq uint64 }

func New(opts Options) (*Handler, error) {
	u, err := url.Parse(opts.Origin)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || opts.AuthHTTP == nil || opts.AuthenticateTicket == nil || opts.ValidateSession == nil || opts.Snapshot == nil {
		return nil, errors.New("poker transport requires exact origin and verified authority ports")
	}
	if opts.SendQueueCapacity == 0 {
		opts.SendQueueCapacity = 32
	}
	if opts.MaxMessageBytes == 0 {
		opts.MaxMessageBytes = 64 << 10
	}
	if opts.WriteTimeout == 0 {
		opts.WriteTimeout = 5 * time.Second
	}
	if opts.OperationTimeout == 0 {
		opts.OperationTimeout = 10 * time.Second
	}
	if opts.MaxConnections == 0 {
		opts.MaxConnections = 1024
	}
	if opts.SendQueueCapacity < 1 || opts.SendQueueCapacity > 1024 || opts.MaxMessageBytes < 1024 || opts.MaxMessageBytes > 4<<20 || opts.WriteTimeout <= 0 || opts.OperationTimeout <= 0 || opts.MaxConnections < 1 || opts.MaxConnections > 100000 {
		return nil, errors.New("invalid poker transport bounds")
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Handler{opts: opts, ctx: ctx, cancel: cancel, subs: map[string]map[*subscriber]struct{}{}, sequences: map[string]sequence{}, live: map[string]*liveConnection{}}, nil
}

func fault(err error) *Fault {
	var f *Fault
	if errors.As(err, &f) && f.Status >= 400 && f.Status <= 599 && len(f.Code) > 0 && len(f.Code) <= 80 {
		for _, c := range f.Code {
			if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
				return &Fault{500, "POKER_INTERNAL_ERROR"}
			}
		}
		return f
	}
	return &Fault{500, "POKER_INTERNAL_ERROR"}
}
