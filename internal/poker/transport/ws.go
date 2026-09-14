package transport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

type serverEnvelope struct {
	Type         string          `json:"type"`
	EventID      string          `json:"event_id"`
	EventSeq     SafeNumber      `json:"event_seq"`
	TableID      string          `json:"table_id"`
	TableVersion SafeNumber      `json:"table_version"`
	HandID       *string         `json:"hand_id"`
	HandVersion  *SafeNumber     `json:"hand_version"`
	ServerTime   time.Time       `json:"server_time"`
	Payload      json.RawMessage `json:"payload"`
}
type subscriber struct {
	commits chan uint64
	control chan struct{}
	cancel  context.CancelFunc
}

// NotifyCommitted accepts version-only notifications AFTER durable commit.
// It never fetches a view, writes a socket or waits for queue space on the actor.
// Overflow cancels that slow connection; reconnect/sync reads a fresh projection.
func (h *Handler) NotifyCommitted(tableID string, version uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	for sub := range h.subs[tableID] {
		select {
		case sub.commits <- version:
		default:
			sub.cancel()
		}
	}
}
func (h *Handler) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	h.cancel()
	for _, subs := range h.subs {
		for sub := range subs {
			sub.cancel()
		}
	}
}

// ForgetClosedTable releases a sequence ONLY after the owner has durably closed
// the table and all its sockets are gone. Active actor sequences are not evicted.
func (h *Handler) ForgetClosedTable(tableID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.subs[tableID]) != 0 {
		return false
	}
	delete(h.sequences, tableID)
	return true
}
func (h *Handler) nextEnvelope(kind string, s Snapshot, payload json.RawMessage) (serverEnvelope, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	state, exists := h.sequences[s.TableID]
	if !exists && len(h.sequences) >= h.opts.MaxConnections {
		return serverEnvelope{}, &Fault{503, "POKER_TABLE_CAPACITY"}
	}
	if exists && s.RuntimeEpoch < state.epoch {
		return serverEnvelope{}, &Fault{409, "STALE_RUNTIME_EPOCH"}
	}
	if !exists || s.RuntimeEpoch != state.epoch {
		state = sequence{epoch: s.RuntimeEpoch}
	}
	if state.seq >= MaxSafeInteger {
		return serverEnvelope{}, errVersion
	}
	state.seq++
	if s.TableVersion > state.version {
		state.version = s.TableVersion
	}
	h.sequences[s.TableID] = state
	var hv *SafeNumber
	if s.HandVersion != nil {
		v := SafeNumber(*s.HandVersion)
		hv = &v
	}
	return serverEnvelope{Type: kind, EventID: fmt.Sprintf("%s:%d:%d", s.TableID, s.RuntimeEpoch, state.seq), EventSeq: SafeNumber(state.seq), TableID: s.TableID, TableVersion: SafeNumber(s.TableVersion), HandID: s.HandID, HandVersion: hv, ServerTime: s.ServerTime, Payload: payload}, nil
}
func (h *Handler) writeEvent(ctx context.Context, c *websocket.Conn, kind string, s Snapshot, payload json.RawMessage) error {
	env, err := h.nextEnvelope(kind, s, payload)
	if err != nil {
		return err
	}
	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
	if int64(len(b)) > h.opts.MaxMessageBytes {
		return &Fault{413, "MESSAGE_TOO_LARGE"}
	}
	ctx, cancel := context.WithTimeout(ctx, h.opts.WriteTimeout)
	defer cancel()
	return c.Write(ctx, websocket.MessageText, b)
}
func (h *Handler) serveWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.RawQuery != "" || !h.validOrigin(r) {
		writeError(w, &Fault{403, "WS_UPGRADE_DENIED"})
		return
	}
	protocols := r.Header.Values("Sec-WebSocket-Protocol")
	// A single exact protocol also makes smuggling a credential in another
	// subprotocol impossible. No Origin wildcard or credential-bearing URL.
	if len(protocols) != 1 || strings.TrimSpace(protocols[0]) != Subprotocol {
		writeError(w, &Fault{400, "WS_SUBPROTOCOL_REQUIRED"})
		return
	}
	h.mu.Lock()
	if h.closed || h.connections >= h.opts.MaxConnections {
		h.mu.Unlock()
		writeError(w, &Fault{503, "POKER_CONNECTION_CAPACITY"})
		return
	}
	h.connections++
	h.mu.Unlock()
	defer func() { h.mu.Lock(); h.connections--; h.mu.Unlock() }()
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{Subprotocol}, InsecureSkipVerify: true, CompressionMode: websocket.CompressionDisabled})
	// InsecureSkipVerify disables the library's host-pattern policy only AFTER
	// exact scheme+host+port Origin validation above; never disables TLS checks.
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(h.opts.MaxMessageBytes)
	ctx, cancel := context.WithCancel(h.ctx)
	defer cancel()
	authCtx, authCancel := context.WithTimeout(ctx, AuthDeadline)
	defer authCancel()
	authDone := make(chan struct{})
	defer close(authDone)
	go func() {
		select {
		case <-authCtx.Done():
			if authCtx.Err() == context.DeadlineExceeded {
				_ = c.Close(websocket.StatusPolicyViolation, "AUTH_TIMEOUT")
			}
		case <-authDone:
		}
	}()
	typ, b, err := c.Read(ctx)
	if err != nil {
		authCancel()
		return
	}
	e, err := decodeClient(b)
	if typ != websocket.MessageText || err != nil || e.Type != "auth.connect" {
		authCancel()
		_ = c.Close(websocket.StatusPolicyViolation, "AUTH_REQUIRED")
		return
	}
	var auth struct {
		Ticket string `json:"poker_connect_ticket"`
	}
	if strictJSON(e.Payload, &auth, "poker_connect_ticket") != nil || len(auth.Ticket) < 1 || len(auth.Ticket) > 8192 {
		authCancel()
		_ = c.Close(websocket.StatusPolicyViolation, "AUTH_INVALID")
		return
	}
	p, err := h.opts.AuthenticateTicket(authCtx, ConnectRequest{auth.Ticket, e.TableID})
	if err == nil && p.UserID > 0 {
		err = h.opts.ValidateSession(authCtx, p)
	}
	if err != nil || p.UserID <= 0 || authCtx.Err() != nil {
		if authCtx.Err() == context.DeadlineExceeded {
			err = &Fault{401, "AUTH_TIMEOUT"}
		}
		authCancel()
		if err == nil {
			err = &Fault{401, "AUTH_FAILED"}
		}
		closeFault(c, err)
		return
	}
	authCancel()
	var connectionBytes [16]byte
	if _, err := rand.Read(connectionBytes[:]); err != nil {
		closeFault(c, err)
		return
	}
	ref := ConnectionRef{ID: hex.EncodeToString(connectionBytes[:]), TableID: e.TableID}
	if h.opts.Connected != nil {
		opCtx, opCancel := context.WithTimeout(ctx, h.opts.OperationTimeout)
		err := h.opts.Connected(opCtx, p, ref)
		opCancel()
		if err != nil {
			closeFault(c, err)
			return
		}
	}
	if h.opts.Disconnected != nil {
		defer func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), h.opts.OperationTimeout)
			defer cleanupCancel()
			h.opts.Disconnected(cleanupCtx, p, ref)
		}()
	}
	sub := &subscriber{commits: make(chan uint64, h.opts.SendQueueCapacity), control: make(chan struct{}, 1), cancel: cancel}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	if h.subs[e.TableID] == nil {
		h.subs[e.TableID] = map[*subscriber]struct{}{}
	}
	h.subs[e.TableID][sub] = struct{}{}
	h.live[ref.ID] = &liveConnection{principal: p, ref: ref, ctx: ctx, sub: sub}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.subs[e.TableID], sub)
		delete(h.live, ref.ID)
		if len(h.subs[e.TableID]) == 0 {
			delete(h.subs, e.TableID)
		}
		h.mu.Unlock()
	}()
	// Register before reading: a commit concurrent with the first view cannot
	// fall into a subscribe-after-snapshot gap.
	last, err := h.sessionSnapshot(ctx, p, ref)
	if err != nil {
		closeFault(c, err)
		return
	}
	accepted, _ := json.Marshal(struct {
		RequestID    string `json:"request_id"`
		ConnectionID string `json:"connection_id"`
	}{e.RequestID, ref.ID})
	if h.writeEvent(ctx, c, "auth.accepted", last, accepted) != nil || h.writeEvent(ctx, c, "table.snapshot", last, last.Payload) != nil {
		return
	}
	incoming := make(chan clientEnvelope, h.opts.SendQueueCapacity)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		defer cancel()
		for {
			typ, b, err := c.Read(ctx)
			if err != nil {
				return
			}
			msg, err := decodeClient(b)
			if typ != websocket.MessageText || err != nil {
				if errors.Is(err, errVersion) {
					closeFault(c, errVersion)
				} else {
					_ = c.Close(websocket.StatusPolicyViolation, "PROTOCOL_INVALID_MESSAGE")
				}
				return
			}
			select {
			case incoming <- msg:
			case <-ctx.Done():
				return
			default:
				return
			}
		}
	}()
	defer func() { cancel(); _ = c.CloseNow(); <-readDone }()
	for {
		select {
		case <-ctx.Done():
			return
		case version := <-sub.commits:
			if version <= last.TableVersion {
				continue
			}
			next, err := h.sessionSnapshot(ctx, p, ref)
			if err != nil {
				closeFault(c, err)
				return
			}
			if next.RuntimeEpoch == last.RuntimeEpoch && next.TableVersion <= last.TableVersion {
				continue
			}
			if h.writeSnapshotTransition(ctx, c, last, next) != nil {
				return
			}
			last = next
		case <-sub.control:
			next, err := h.sessionSnapshot(ctx, p, ref)
			if err != nil {
				closeFault(c, err)
				return
			}
			if next.TableVersion < last.TableVersion || next.RuntimeEpoch < last.RuntimeEpoch {
				closeFault(c, &Fault{409, "STALE_RUNTIME_EPOCH"})
				return
			}
			if !sameControl(last.Control, next.Control) || next.TableVersion != last.TableVersion || next.RuntimeEpoch != last.RuntimeEpoch {
				if h.writeSnapshotTransition(ctx, c, last, next) != nil {
					return
				}
				last = next
			}
		case msg := <-incoming:
			if msg.TableID != e.TableID {
				_ = c.Close(websocket.StatusPolicyViolation, "TABLE_BINDING_MISMATCH")
				return
			}
			if msg.Type == "auth.connect" {
				_ = c.Close(websocket.StatusPolicyViolation, "AUTH_ALREADY_COMPLETE")
				return
			}
			opCtx, opCancel := context.WithTimeout(ctx, h.opts.OperationTimeout)
			err := h.opts.ValidateSession(opCtx, p)
			if err != nil {
				opCancel()
				_ = c.Close(websocket.StatusPolicyViolation, "SESSION_INVALID")
				return
			}
			if msg.Type == "sync.request" {
				if err = emptyPayload(msg.Payload); err == nil {
					var next Snapshot
					next, err = h.readSnapshot(opCtx, p, ref)
					if err == nil && (next.TableVersion < last.TableVersion || next.RuntimeEpoch < last.RuntimeEpoch) {
						err = &Fault{409, "STALE_RUNTIME_EPOCH"}
					}
					if err == nil {
						err = h.writeSnapshotTransition(ctx, c, last, next)
						if err == nil {
							last = next
						}
					}
				}
				opCancel()
				if err != nil {
					if h.writeProtocolError(ctx, c, last, msg.RequestID, err) != nil {
						return
					}
				}
				continue
			}
			if msg.Type == "ping" {
				err = emptyPayload(msg.Payload)
				opCancel()
				if err == nil {
					payload, _ := json.Marshal(struct {
						RequestID string `json:"request_id"`
					}{msg.RequestID})
					if h.writeEvent(ctx, c, "pong", last, payload) != nil {
						return
					}
				} else if h.writeProtocolError(ctx, c, last, msg.RequestID, err) != nil {
					return
				}
				continue
			}
			var control ControlRef
			if msg.Type == "hand.action" || msg.Type == "session.sit_out_next_hand" || msg.Type == "session.resume_play" || msg.Type == "client_seed.set_next" {
				if p.ControlIntent != "CLAIM_CONTROL" {
					err = &Fault{403, "POKER_CONNECTION_READ_ONLY"}
				} else if h.opts.AuthorizeControl == nil {
					err = errUnavailable
				} else {
					control, err = h.opts.AuthorizeControl(opCtx, p, ref, uint64(msg.ControlEpoch))
					if err == nil {
						err = validateControlRef(control, p, ref, last, uint64(msg.ControlEpoch))
					}
				}
			}
			if err != nil {
				opCancel()
				if h.writeProtocolError(ctx, c, last, msg.RequestID, err) != nil {
					return
				}
				continue
			}
			receipt, err := h.dispatchConnection(opCtx, p, msg, control, ref)
			opCancel()
			if err != nil {
				if h.writeProtocolError(ctx, c, last, msg.RequestID, err) != nil {
					return
				}
				continue
			}
			// A receipt acknowledges a command, never substitutes for a committed view.
			// Only NotifyCommitted (or explicit sync) can trigger a live state snapshot.
			payload, _ := json.Marshal(struct {
				RequestID string          `json:"request_id"`
				ActionID  *string         `json:"action_id"`
				Receipt   json.RawMessage `json:"receipt"`
			}{msg.RequestID, msg.ActionID, receipt})
			if h.writeEvent(ctx, c, "service.notice", last, payload) != nil {
				return
			}
		}
	}
}

func closeFault(c *websocket.Conn, err error) {
	f := fault(err)
	code := websocket.StatusPolicyViolation
	if f.Status >= 500 {
		code = websocket.StatusInternalError
	}
	_ = c.Close(code, f.Code)
}
func (h *Handler) sessionSnapshot(ctx context.Context, p Principal, ref ConnectionRef) (Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, h.opts.OperationTimeout)
	defer cancel()
	if err := h.opts.ValidateSession(ctx, p); err != nil {
		return Snapshot{}, err
	}
	return h.readSnapshot(ctx, p, ref)
}

func (h *Handler) writeSnapshotTransition(ctx context.Context, c *websocket.Conn, previous, next Snapshot) error {
	if !sameControl(previous.Control, next.Control) {
		payload, err := json.Marshal(next.Control)
		if err != nil {
			return err
		}
		if err = h.writeEvent(ctx, c, "control.changed", next, payload); err != nil {
			return err
		}
	}
	return h.writeEvent(ctx, c, "table.snapshot", next, next.Payload)
}
func (h *Handler) writeProtocolError(ctx context.Context, c *websocket.Conn, s Snapshot, requestID string, err error) error {
	payload, _ := json.Marshal(struct {
		RequestID string `json:"request_id"`
		Code      string `json:"code"`
	}{requestID, fault(err).Code})
	return h.writeEvent(ctx, c, "error", s, payload)
}
func emptyPayload(b []byte) error { var v struct{}; return strictJSON(b, &v) }

func (h *Handler) dispatch(ctx context.Context, p Principal, e clientEnvelope, control ControlRef) (json.RawMessage, error) {
	return h.dispatchConnection(ctx, p, e, control, ConnectionRef{})
}

func (h *Handler) dispatchConnection(ctx context.Context, p Principal, e clientEnvelope, control ControlRef, connection ConnectionRef) (json.RawMessage, error) {
	ports := h.opts.Ports
	if e.ActionID == nil && (e.Type == "hand.action" || e.Type == "session.sit_out_next_hand" || e.Type == "session.resume_play" || e.Type == "client_seed.set_next") {
		return nil, errInvalid
	}
	var out json.RawMessage
	var err error
	switch e.Type {
	case "hand.action":
		var b struct {
			ActionType       string  `json:"action_type"`
			RequestedToUnits *string `json:"requested_to_units"`
		}
		if strictJSON(e.Payload, &b, "action_type", "requested_to_units") != nil || e.HandID == nil {
			return nil, errInvalid
		}
		switch b.ActionType {
		case "BET", "RAISE":
			if b.RequestedToUnits == nil || !validAmount(*b.RequestedToUnits, true) {
				return nil, errInvalid
			}
		case "FOLD", "CHECK", "CALL", "ALL_IN":
			if b.RequestedToUnits != nil {
				return nil, errInvalid
			}
		default:
			return nil, errInvalid
		}
		if ports.Act == nil {
			return nil, errUnavailable
		}
		out, err = ports.Act(ctx, p, HandAction{RequestID: e.RequestID, ActionID: *e.ActionID, TableID: e.TableID, HandID: *e.HandID, ExpectedTableVersion: uint64(e.ExpectedTableVersion), ExpectedHandVersion: uint64(e.ExpectedHandVersion), ControlEpoch: uint64(e.ControlEpoch), ActionType: b.ActionType, RequestedToUnits: b.RequestedToUnits, Control: control})
	case "session.sit_out_next_hand", "session.resume_play":
		if emptyPayload(e.Payload) != nil {
			return nil, errInvalid
		}
		fn := ports.SitOut
		if e.Type == "session.resume_play" {
			fn = ports.Resume
		}
		if fn == nil {
			return nil, errUnavailable
		}
		out, err = fn(ctx, p, SessionAction{RequestID: e.RequestID, ActionID: *e.ActionID, TableID: e.TableID, ExpectedTableVersion: uint64(e.ExpectedTableVersion), ExpectedHandVersion: uint64(e.ExpectedHandVersion), ControlEpoch: uint64(e.ControlEpoch), HandID: e.HandID, Control: control})
	case "client_seed.set_next":
		var b struct {
			Seed string `json:"client_seed"`
		}
		if strictJSON(e.Payload, &b, "client_seed") != nil || len(b.Seed) == 0 || len(b.Seed) > 256 {
			return nil, errInvalid
		}
		if ports.SetClientSeed == nil {
			return nil, errUnavailable
		}
		out, err = ports.SetClientSeed(ctx, p, ClientSeed{RequestID: e.RequestID, ActionID: *e.ActionID, TableID: e.TableID, Seed: b.Seed, ExpectedTableVersion: uint64(e.ExpectedTableVersion), ExpectedHandVersion: uint64(e.ExpectedHandVersion), ControlEpoch: uint64(e.ControlEpoch), HandID: e.HandID, Control: control})
	case "chat.send":
		var b struct {
			Message string `json:"message"`
		}
		if strictJSON(e.Payload, &b, "message") != nil || strings.TrimSpace(b.Message) == "" || len(b.Message) > 2048 {
			return nil, errInvalid
		}
		if ports.Chat == nil {
			return nil, errUnavailable
		}
		out, err = ports.Chat(ctx, p, ChatMessage{RequestID: e.RequestID, TableID: e.TableID, Message: b.Message, Connection: connection})
	default:
		return nil, errProtocol
	}
	if err == nil && !json.Valid(out) {
		return nil, &Fault{500, "POKER_INVALID_RECEIPT"}
	}
	return out, err
}
