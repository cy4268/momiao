package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const testTable = "11111111-1111-4111-8111-111111111111"
const testOther = "22222222-2222-4222-8222-222222222222"
const testHand = "33333333-3333-4333-8333-333333333333"
const testPokerSession = "44444444-4444-4444-8444-444444444444"
const testOrigin = "https://poker.example.test"

// syntheticBackend is deliberately not PostgreSQL or ct1. These socket tests
// verify the transport boundary, not real authentication or durable settlement.
type syntheticBackend struct {
	mu                                             sync.Mutex
	version, epoch                                 uint64
	controlEpoch                                   uint64
	authCalls, snapshotCalls, actionCalls, effects int
	actions                                        map[string]bool
	lastAction                                     HandAction
	revoked                                        bool
	controllers                                    map[int64]string
}

func newSynthetic() *syntheticBackend {
	return &syntheticBackend{version: 1, epoch: 1, controlEpoch: 1, actions: map[string]bool{}, controllers: map[int64]string{}}
}
func (b *syntheticBackend) opts() Options {
	return Options{
		Origin: testOrigin,
		AuthHTTP: func(r *http.Request) (Principal, error) {
			if r.Header.Get("Authorization") != "Bearer synthetic-http" {
				return Principal{}, &Fault{401, "AUTH_REQUIRED"}
			}
			return Principal{UserID: 1, SessionIDHash: "synthetic-session-1", SessionVersion: 1}, nil
		},
		AuthenticateTicket: func(ctx context.Context, r ConnectRequest) (Principal, error) {
			b.mu.Lock()
			defer b.mu.Unlock()
			b.authCalls++
			if r.TableID != testTable {
				return Principal{}, &Fault{403, "TICKET_TARGET_MISMATCH"}
			}
			id, err := strconv.ParseInt(strings.TrimPrefix(r.PokerConnectTicket, "synthetic-ticket-"), 10, 64)
			if err != nil || id < 1 {
				return Principal{}, &Fault{401, "TICKET_INVALID"}
			}
			intent := "CLAIM_CONTROL"
			if id > 2 {
				intent = "READ_ONLY"
			}
			return Principal{UserID: id, SessionIDHash: fmt.Sprintf("synthetic-session-%d", id), SessionVersion: 1, ControlIntent: intent}, nil
		},
		ValidateSession: func(context.Context, Principal) error {
			b.mu.Lock()
			defer b.mu.Unlock()
			if b.revoked {
				return &Fault{401, "SESSION_REVOKED"}
			}
			return nil
		},
		Snapshot: b.snapshot,
		Connected: func(ctx context.Context, p Principal, r ConnectionRef) error {
			b.mu.Lock()
			defer b.mu.Unlock()
			if b.controllers[p.UserID] == "" && p.ControlIntent == "CLAIM_CONTROL" {
				b.controllers[p.UserID] = r.ID
			}
			return nil
		},
		Disconnected: func(ctx context.Context, p Principal, r ConnectionRef) {
			b.mu.Lock()
			defer b.mu.Unlock()
			if b.controllers[p.UserID] == r.ID {
				delete(b.controllers, p.UserID)
			}
		},
		AuthorizeControl: func(ctx context.Context, p Principal, r ConnectionRef, epoch uint64) (ControlRef, error) {
			b.mu.Lock()
			defer b.mu.Unlock()
			if b.controllers[p.UserID] != r.ID || epoch != b.controlEpoch {
				return ControlRef{}, &Fault{409, "STALE_CONTROL_EPOCH"}
			}
			return ControlRef{UserID: p.UserID, SessionIDHash: p.SessionIDHash, SessionVersion: p.SessionVersion, SecurityEpoch: p.SecurityEpoch, ConnectionID: r.ID, TableID: r.TableID, SessionID: testPokerSession, RuntimeEpoch: b.epoch, ControlEpoch: b.controlEpoch}, nil
		},
		Ports: Ports{Act: func(ctx context.Context, p Principal, a HandAction) (json.RawMessage, error) {
			b.mu.Lock()
			defer b.mu.Unlock()
			b.actionCalls++
			b.lastAction = a
			duplicate := b.actions[a.ActionID]
			if !duplicate {
				b.effects++
				b.actions[a.ActionID] = true
			}
			return json.Marshal(map[string]any{"table_id": a.TableID, "table_version": strconv.FormatUint(b.version, 10), "duplicate": duplicate, "status": "APPLIED"})
		}},
	}
}
func (b *syntheticBackend) snapshot(ctx context.Context, p Principal, ref ConnectionRef) (Snapshot, error) {
	table := ref.TableID
	b.mu.Lock()
	defer b.mu.Unlock()
	b.snapshotCalls++
	seats := []map[string]any{}
	for id := int64(1); id <= 2; id++ {
		s := map[string]any{"seat_no": id, "is_self": id == p.UserID, "stack_units": "50000000"}
		if id == p.UserID {
			s["hole_cards"] = []int{int(id * 2), int(id*2 + 1)}
		}
		seats = append(seats, s)
	}
	viewer := map[string]any{"control_epoch": strconv.FormatUint(b.controlEpoch, 10), "can_act": false}
	if p.UserID <= 2 {
		viewer["session_id"] = testPokerSession
	}
	var control *ControlView
	if ref.ID != "" {
		control = &ControlView{ConnectionID: ref.ID, Mode: "READ_ONLY", ControlEpoch: strconv.FormatUint(b.controlEpoch, 10)}
		if p.UserID <= 2 {
			control.SessionID = testPokerSession
		}
		if b.controllers[p.UserID] == ref.ID {
			control.Mode = "CONTROLLER"
		}
		viewer["control"] = control
	}
	body, _ := json.Marshal(map[string]any{"table_id": table, "table_version": strconv.FormatUint(b.version, 10), "seats": seats, "viewer": viewer})
	return Snapshot{TableID: table, TableVersion: b.version, RuntimeEpoch: b.epoch, ServerTime: time.Now().UTC(), Payload: body, Control: control}, nil
}
func newServer(t *testing.T, b *syntheticBackend, edit func(*Options)) (*Handler, *httptest.Server) {
	t.Helper()
	o := b.opts()
	if edit != nil {
		edit(&o)
	}
	h, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	s := httptest.NewServer(h)
	t.Cleanup(func() { h.Close(); s.Close() })
	return h, s
}

type readResult struct {
	event serverEnvelope
	err   error
}
type peer struct {
	c      *websocket.Conn
	events chan readResult
}

func dialPeer(t *testing.T, s *httptest.Server, table string, user int64) *peer {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(s.URL, "http")+"/ws/poker", &websocket.DialOptions{Subprotocols: []string{Subprotocol}, HTTPHeader: http.Header{"Origin": []string{testOrigin}}})
	if err != nil {
		t.Fatal(err)
	}
	p := &peer{c: c, events: make(chan readResult, 128)}
	t.Cleanup(func() { _ = c.CloseNow() })
	go func() {
		for {
			_, raw, err := c.Read(context.Background())
			if err != nil {
				p.events <- readResult{err: err}
				return
			}
			var e serverEnvelope
			err = json.Unmarshal(raw, &e)
			p.events <- readResult{event: e, err: err}
		}
	}()
	send(t, p, clientMessage("auth.connect", table, map[string]any{"poker_connect_ticket": fmt.Sprintf("synthetic-ticket-%d", user)}))
	return p
}
func clientMessage(kind, table string, payload any) map[string]any {
	return map[string]any{"type": kind, "request_id": "request-00000001", "table_id": table, "hand_id": nil, "expected_table_version": 1, "expected_hand_version": 0, "control_epoch": 0, "action_id": nil, "payload": payload}
}
func send(t *testing.T, p *peer, value any) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err = p.c.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
}
func next(t *testing.T, p *peer, kind string) serverEnvelope {
	t.Helper()
	select {
	case r := <-p.events:
		if r.err != nil {
			t.Fatal(r.err)
		}
		if r.event.Type != kind {
			t.Fatalf("want %s, got %s (%s)", kind, r.event.Type, r.event.Payload)
		}
		return r.event
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout waiting for %s", kind)
	}
	return serverEnvelope{}
}
func authenticated(t *testing.T, p *peer) serverEnvelope {
	t.Helper()
	next(t, p, "auth.accepted")
	return next(t, p, "table.snapshot")
}
func noEvent(t *testing.T, p *peer) {
	t.Helper()
	select {
	case r := <-p.events:
		t.Fatalf("unexpected event/error %s %v", r.event.Type, r.err)
	case <-time.After(80 * time.Millisecond):
	}
}
func expectClosed(t *testing.T, p *peer) {
	t.Helper()
	select {
	case r := <-p.events:
		if r.err == nil {
			t.Fatalf("expected close, got %s", r.event.Type)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("socket remained open")
	}
}

func TestRealSocketUpgradeGates(t *testing.T) {
	b := newSynthetic()
	_, s := newServer(t, b, nil)
	cases := []struct{ name, origin, protocol, path string }{{"missing-origin", "", Subprotocol, "/ws/poker"}, {"foreign-origin", "https://evil.test", Subprotocol, "/ws/poker"}, {"scheme-mismatch", "http://poker.example.test", Subprotocol, "/ws/poker"}, {"missing-protocol", testOrigin, "", "/ws/poker"}, {"wrong-protocol", testOrigin, "chaldea-poker.v2", "/ws/poker"}, {"token-subprotocol", testOrigin, Subprotocol + ", secret", "/ws/poker"}, {"ticket-url", testOrigin, Subprotocol, "/ws/poker?ticket=secret"}}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			header := http.Header{}
			if tt.origin != "" {
				header.Set("Origin", tt.origin)
			}
			opts := &websocket.DialOptions{HTTPHeader: header}
			if tt.protocol != "" {
				opts.Subprotocols = []string{tt.protocol}
			}
			c, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(s.URL, "http")+tt.path, opts)
			if c != nil {
				c.CloseNow()
			}
			if err == nil || resp == nil || resp.StatusCode == 101 {
				t.Fatalf("upgrade accepted: %v", err)
			}
		})
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.authCalls != 0 {
		t.Fatal("rejected Upgrade reached ticket authority")
	}
}
func TestRealSocketTenSecondAuthDeadline(t *testing.T) {
	_, s := newServer(t, newSynthetic(), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 13*time.Second)
	defer cancel()
	started := time.Now()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(s.URL, "http")+"/ws/poker", &websocket.DialOptions{Subprotocols: []string{Subprotocol}, HTTPHeader: http.Header{"Origin": []string{testOrigin}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	_, _, err = c.Read(ctx)
	elapsed := time.Since(started)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation || !strings.Contains(err.Error(), "AUTH_TIMEOUT") || elapsed < 9900*time.Millisecond || elapsed > 12*time.Second {
		t.Fatalf("elapsed=%s err=%v", elapsed, err)
	}
	t.Logf("real fixed authentication deadline observed: %s", elapsed)
}
func TestRealSocketFirstBusinessFrameMustAuthenticate(t *testing.T) {
	b := newSynthetic()
	_, s := newServer(t, b, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(s.URL, "http")+"/ws/poker", &websocket.DialOptions{Subprotocols: []string{Subprotocol}, HTTPHeader: http.Header{"Origin": []string{testOrigin}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	raw, _ := json.Marshal(clientMessage("ping", testTable, map[string]any{}))
	if err = c.Write(ctx, websocket.MessageText, raw); err != nil {
		t.Fatal(err)
	}
	_, _, err = c.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatal(err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.snapshotCalls != 0 || b.authCalls != 0 {
		t.Fatal("unauthenticated frame reached authority")
	}
}
func TestRealSocketsViewerIsolationCommitOnlyAndReconnect(t *testing.T) {
	b := newSynthetic()
	h, s := newServer(t, b, nil)
	p1 := dialPeer(t, s, testTable, 1)
	p2 := dialPeer(t, s, testTable, 2)
	spectator := dialPeer(t, s, testTable, 3)
	for i, p := range []*peer{p1, p2, spectator} {
		env := authenticated(t, p)
		var body struct {
			Seats []struct {
				IsSelf bool  `json:"is_self"`
				Hole   []int `json:"hole_cards"`
			} `json:"seats"`
		}
		if json.Unmarshal(env.Payload, &body) != nil {
			t.Fatal("payload")
		}
		for _, seat := range body.Seats {
			if !seat.IsSelf && len(seat.Hole) != 0 {
				t.Fatal("other player's private cards exposed")
			}
		}
		if i == 2 && strings.Contains(string(env.Payload), "hole_cards") {
			t.Fatal("spectator saw private cards")
		}
	}
	b.mu.Lock()
	b.version = 2
	before := b.snapshotCalls
	b.mu.Unlock()
	noEvent(t, p1)
	b.mu.Lock()
	if b.snapshotCalls != before {
		t.Fatal("view polled without commit or sync")
	}
	b.version = 3
	b.mu.Unlock()
	h.NotifyCommitted(testTable, 2)
	for _, p := range []*peer{p1, p2, spectator} {
		env := next(t, p, "table.snapshot")
		if env.TableVersion != 3 || !strings.Contains(string(env.Payload), `"table_version":"3"`) {
			t.Fatalf("old callback mislabeled newer projection: %+v", env)
		}
	}
	h.NotifyCommitted(testTable, 3)
	noEvent(t, p1)
	_ = p1.c.CloseNow()
	p1 = dialPeer(t, s, testTable, 1)
	env := authenticated(t, p1)
	if env.TableVersion != 3 {
		t.Fatal("reconnect replayed stale cached private snapshot")
	}
	send(t, p2, clientMessage("sync.request", testTable, map[string]any{}))
	if next(t, p2, "table.snapshot").TableVersion != 3 {
		t.Fatal("resync")
	}
}
func TestRealSocketTypedActionsReceiptsAndNoPrematureSnapshot(t *testing.T) {
	b := newSynthetic()
	b.controlEpoch = 4
	_, s := newServer(t, b, nil)
	p := dialPeer(t, s, testTable, 1)
	authenticated(t, p)
	action := clientMessage("hand.action", testTable, map[string]any{"action_type": "ALL_IN", "requested_to_units": nil})
	action["hand_id"] = testHand
	action["action_id"] = "action-000000001"
	action["expected_hand_version"] = 7
	action["control_epoch"] = 4
	send(t, p, action)
	ack := next(t, p, "service.notice")
	if !strings.Contains(string(ack.Payload), `"action_id":"action-000000001"`) {
		t.Fatal("missing stable action correlation")
	}
	noEvent(t, p)
	send(t, p, action)
	ack = next(t, p, "service.notice")
	if !strings.Contains(string(ack.Payload), `"duplicate":true`) {
		t.Fatal("duplicate receipt changed")
	}
	noEvent(t, p)
	b.mu.Lock()
	if b.effects != 1 || b.actionCalls != 2 || b.lastAction.ControlEpoch != 4 || b.lastAction.ExpectedHandVersion != 7 || b.lastAction.RequestedToUnits != nil || b.snapshotCalls != 1 {
		t.Fatal("typed action guards or commit-only invariant changed")
	}
	b.mu.Unlock()
	action["payload"] = map[string]any{"action_type": "ALL_IN", "requested_to_units": "500000"}
	send(t, p, action)
	next(t, p, "error")
	action["type"] = "host.close"
	send(t, p, action)
	errEnv := next(t, p, "error")
	if !strings.Contains(string(errEnv.Payload), "PROTOCOL_UNSUPPORTED_MESSAGE") {
		t.Fatal("unknown message was dispatched")
	}
}
func TestRealSocketBindingRevocationAndRuntimeBaseline(t *testing.T) {
	t.Run("binding", func(t *testing.T) {
		_, s := newServer(t, newSynthetic(), nil)
		p := dialPeer(t, s, testTable, 1)
		authenticated(t, p)
		send(t, p, clientMessage("sync.request", testOther, map[string]any{}))
		expectClosed(t, p)
	})
	t.Run("revoked", func(t *testing.T) {
		b := newSynthetic()
		_, s := newServer(t, b, nil)
		p := dialPeer(t, s, testTable, 1)
		authenticated(t, p)
		b.mu.Lock()
		b.revoked = true
		b.mu.Unlock()
		send(t, p, clientMessage("ping", testTable, map[string]any{}))
		expectClosed(t, p)
	})
	t.Run("new-runtime", func(t *testing.T) {
		b := newSynthetic()
		h, s := newServer(t, b, nil)
		p := dialPeer(t, s, testTable, 1)
		first := authenticated(t, p)
		b.mu.Lock()
		b.epoch = 2
		b.version = 4
		b.mu.Unlock()
		h.NotifyCommitted(testTable, 4)
		second := next(t, p, "table.snapshot")
		if second.EventSeq != 1 || second.EventID == first.EventID {
			t.Fatal("new actor did not start snapshot baseline")
		}
		send(t, p, clientMessage("sync.request", testTable, map[string]any{}))
		next(t, p, "table.snapshot")
	})
}

func TestRealHTTPRoutesStrictBodiesAndTrustedIdentity(t *testing.T) {
	b := newSynthetic()
	var captured Principal
	var create CreateTableRequest
	_, s := newServer(t, b, func(o *Options) {
		o.Ports.CreateTable = func(ctx context.Context, p Principal, v CreateTableRequest) (json.RawMessage, error) {
			captured = p
			create = v
			return json.RawMessage(`{"table_id":"` + testTable + `","table_version":"1"}`), nil
		}
		o.Ports.ReadLobby = func(context.Context, Principal) (json.RawMessage, error) {
			return json.RawMessage(`{"ready":true}`), nil
		}
	})
	good := `{"request_id":"request-00000001","name":"fixture","blind_preset":"5-10","max_seats":2,"allow_spectators":true}`
	cases := []struct {
		name, method, path, body, origin, auth string
		status                                 int
	}{{"create", "POST", "/api/v1/poker/tables", good, testOrigin, "Bearer synthetic-http", 200}, {"auth", "GET", "/api/v1/poker", "", "", "", 401}, {"origin", "POST", "/api/v1/poker/tables", good, "https://evil.test", "Bearer synthetic-http", 403}, {"unknown-id", "POST", "/api/v1/poker/tables", strings.TrimSuffix(good, "}") + `,"user_id":2}`, testOrigin, "Bearer synthetic-http", 400}, {"case-field", "POST", "/api/v1/poker/tables", strings.Replace(good, "name", "NAME", 1), testOrigin, "Bearer synthetic-http", 400}, {"duplicate", "POST", "/api/v1/poker/tables", strings.TrimSuffix(good, "}") + `,"name":"twice"}`, testOrigin, "Bearer synthetic-http", 400}, {"unavailable", "GET", "/api/v1/poker/sessions/active", "", "", "Bearer synthetic-http", 503}, {"wrong-route", "GET", "/api/v1/poker/unknown", "", "", "Bearer synthetic-http", 404}, {"secret-url", "GET", "/api/v1/poker?ticket=secret", "", "", "Bearer synthetic-http", 400}}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest(tt.method, s.URL+tt.path, strings.NewReader(tt.body))
			r.Header.Set("Content-Type", "application/json")
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			if tt.auth != "" {
				r.Header.Set("Authorization", tt.auth)
			}
			resp, err := s.Client().Do(r)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			raw, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != tt.status {
				t.Fatalf("%d %s", resp.StatusCode, raw)
			}
			if resp.Header.Get("Cache-Control") != "no-store" {
				t.Fatal("private response cacheable")
			}
		})
	}
	if captured.UserID != 1 || create.Name != "fixture" {
		t.Fatal("verified identity not passed to typed command")
	}
}

// gateConn wraps the REAL accepted TCP socket. Once gated, a server write
// blocks exactly as a saturated slow-reader socket does; Close unblocks it.
// No fake websocket, fake clock or sleep-to-fill OS-buffer assumption is used.
type gateConn struct {
	net.Conn
	gate    *atomic.Bool
	entered chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func (c *gateConn) Write(b []byte) (int, error) {
	if c.gate.Load() {
		select {
		case c.entered <- struct{}{}:
		default:
		}
		<-c.closed
		return 0, net.ErrClosed
	}
	return c.Conn.Write(b)
}
func (c *gateConn) Close() error { c.once.Do(func() { close(c.closed) }); return c.Conn.Close() }

type gateListener struct {
	net.Listener
	gate    atomic.Bool
	entered chan struct{}
}

func (l *gateListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &gateConn{Conn: c, gate: &l.gate, entered: l.entered, closed: make(chan struct{})}, nil
}

func TestRealSocketSlowWriterBoundedCommitQueueAndResync(t *testing.T) {
	b := newSynthetic()
	o := b.opts()
	o.SendQueueCapacity = 1
	o.WriteTimeout = 100 * time.Millisecond
	h, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	s := httptest.NewUnstartedServer(h)
	listener := &gateListener{Listener: s.Listener, entered: make(chan struct{}, 1)}
	s.Listener = listener
	s.Start()
	t.Cleanup(func() { h.Close(); s.Close() })
	p := dialPeer(t, s, testTable, 1)
	authenticated(t, p)
	listener.gate.Store(true)
	b.mu.Lock()
	b.version = 2
	b.mu.Unlock()
	h.NotifyCommitted(testTable, 2)
	select {
	case <-listener.entered:
	case <-time.After(time.Second):
		t.Fatal("did not enter real socket write")
	}
	started := time.Now()
	for i := uint64(3); i < 10003; i++ {
		h.NotifyCommitted(testTable, i)
	}
	elapsed := time.Since(started)
	if elapsed > 250*time.Millisecond {
		t.Fatalf("slow viewer blocked commit callback: %s", elapsed)
	}
	expectClosed(t, p)
	listener.gate.Store(false)
	b.mu.Lock()
	b.version = 10003
	b.mu.Unlock()
	p2 := dialPeer(t, s, testTable, 1)
	env := authenticated(t, p2)
	if env.TableVersion != 10003 {
		t.Fatal("reconnect did not return current authoritative snapshot")
	}
	t.Logf("10000 nonblocking commit notifications while TCP write gated: %s", elapsed)
}
func TestProjectionMetadataAndFaultRedaction(t *testing.T) {
	b := newSynthetic()
	h, _ := newServer(t, b, nil)
	original := h.opts.Snapshot
	h.opts.Snapshot = func(ctx context.Context, p Principal, ref ConnectionRef) (Snapshot, error) {
		s, err := original(ctx, p, ref)
		s.TableVersion = 7
		return s, err
	}
	if _, err := h.readSnapshot(context.Background(), Principal{UserID: 1}, ConnectionRef{TableID: testTable}); err == nil {
		t.Fatal("metadata/body mismatch accepted")
	}
	if fault(errors.New("secret SQL password ticket=abc")).Code != "POKER_INTERNAL_ERROR" {
		t.Fatal("secret error exposed")
	}
	h.opts.Snapshot = func(ctx context.Context, p Principal, ref ConnectionRef) (Snapshot, error) {
		s, err := original(ctx, p, ref)
		s.TableVersion = MaxSafeInteger + 1
		return s, err
	}
	if _, err := h.readSnapshot(context.Background(), Principal{UserID: 1}, ConnectionRef{TableID: testTable}); !errors.Is(err, errVersion) {
		t.Fatalf("unsafe envelope version accepted: %v", err)
	}
}

func TestRealSocketControllerIsSpecificSocketNotTicketIntent(t *testing.T) {
	b := newSynthetic()
	_, s := newServer(t, b, nil)
	first := dialPeer(t, s, testTable, 1)
	authenticated(t, first)
	second := dialPeer(t, s, testTable, 1)
	authenticated(t, second)
	msg := clientMessage("hand.action", testTable, map[string]any{"action_type": "CHECK"})
	msg["hand_id"] = testHand
	msg["action_id"] = "action-000000001"
	msg["control_epoch"] = 1
	send(t, second, msg)
	reply := next(t, second, "error")
	if !strings.Contains(string(reply.Payload), "STALE_CONTROL_EPOCH") {
		t.Fatal("same-user second socket reused controller")
	}
	send(t, first, msg)
	next(t, first, "service.notice")
	readonly := dialPeer(t, s, testTable, 3)
	authenticated(t, readonly)
	send(t, readonly, msg)
	reply = next(t, readonly, "error")
	if !strings.Contains(string(reply.Payload), "POKER_CONNECTION_READ_ONLY") {
		t.Fatal("READ_ONLY mutated")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.effects != 1 {
		t.Fatal("noncontroller entered domain command")
	}
}

func TestRealSocketMissingControlOwnerFailsClosed(t *testing.T) {
	b := newSynthetic()
	_, s := newServer(t, b, func(o *Options) { o.AuthorizeControl = nil })
	p := dialPeer(t, s, testTable, 1)
	authenticated(t, p)
	msg := clientMessage("hand.action", testTable, map[string]any{"action_type": "CHECK"})
	msg["hand_id"] = testHand
	msg["action_id"] = "action-000000001"
	send(t, p, msg)
	if !strings.Contains(string(next(t, p, "error").Payload), "POKER_CAPABILITY_UNAVAILABLE") {
		t.Fatal("unwired controller accepted")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.effects != 0 {
		t.Fatal("missing authority mutated")
	}
}

func TestRealSocketSafeVersionOverflowIsExplicit(t *testing.T) {
	_, s := newServer(t, newSynthetic(), nil)
	p := dialPeer(t, s, testTable, 1)
	authenticated(t, p)
	msg := clientMessage("ping", testTable, map[string]any{})
	msg["expected_table_version"] = uint64(MaxSafeInteger + 1)
	send(t, p, msg)
	select {
	case r := <-p.events:
		if r.err == nil || !strings.Contains(r.err.Error(), "PROTOCOL_VERSION_OUT_OF_RANGE") {
			t.Fatalf("overflow not explicit: %v", r.err)
		}
	case <-time.After(time.Second):
		t.Fatal("overflow accepted")
	}
}

func TestRealSocketWriteDeadlineWithoutQueueOverflow(t *testing.T) {
	b := newSynthetic()
	o := b.opts()
	o.WriteTimeout = 100 * time.Millisecond
	h, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	s := httptest.NewUnstartedServer(h)
	listener := &gateListener{Listener: s.Listener, entered: make(chan struct{}, 1)}
	s.Listener = listener
	s.Start()
	t.Cleanup(func() { h.Close(); s.Close() })
	p := dialPeer(t, s, testTable, 1)
	authenticated(t, p)
	listener.gate.Store(true)
	b.mu.Lock()
	b.version = 2
	b.mu.Unlock()
	started := time.Now()
	h.NotifyCommitted(testTable, 2)
	expectClosed(t, p)
	elapsed := time.Since(started)
	if elapsed < 80*time.Millisecond || elapsed > time.Second {
		t.Fatalf("write deadline not enforced: %s", elapsed)
	}
	t.Logf("real gated TCP write deadline: %s", elapsed)
}

func TestCloseCancelsPendingAuthentication(t *testing.T) {
	h, s := newServer(t, newSynthetic(), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(s.URL, "http")+"/ws/poker", &websocket.DialOptions{Subprotocols: []string{Subprotocol}, HTTPHeader: http.Header{"Origin": []string{testOrigin}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	started := time.Now()
	h.Close()
	_, _, err = c.Read(ctx)
	if err == nil || time.Since(started) > time.Second {
		t.Fatal("shutdown left AUTH_PENDING socket alive")
	}
}

func TestRealHTTPFrozenPasswordAndTableNameBounds(t *testing.T) {
	// Each family emoji is one extended grapheme, 25 UTF-8 bytes. The domain
	// owns the 40-grapheme validator; transport must not substitute 128 bytes.
	validName := strings.Repeat("👨‍👩‍👧‍👦", 40)
	if len(validName) <= 128 {
		t.Fatal("fixture does not cross the former byte cap")
	}
	var accessCalls, createCalls atomic.Int32
	var receivedName string
	_, s := newServer(t, newSynthetic(), func(o *Options) {
		o.Ports.VerifyTableAccess = func(context.Context, Principal, string, AccessRequest) (json.RawMessage, error) {
			accessCalls.Add(1)
			return json.RawMessage(`{}`), nil
		}
		o.Ports.CreateTable = func(_ context.Context, _ Principal, request CreateTableRequest) (json.RawMessage, error) {
			receivedName = request.Name
			createCalls.Add(1)
			// Representative domain rejection: the HTTP boundary must preserve
			// a real validator's decision rather than acknowledge it itself.
			if request.Name == strings.Repeat("a", 41) {
				return nil, errInvalid
			}
			return json.RawMessage(`{}`), nil
		}
	})
	post := func(path string, body any) int {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r, err := http.NewRequest(http.MethodPost, s.URL+path, strings.NewReader(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer synthetic-http")
		r.Header.Set("Origin", testOrigin)
		r.Header.Set("Content-Type", "application/json")
		response, err := s.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, response.Body)
		return response.StatusCode
	}
	t.Run("password-129-bytes", func(t *testing.T) {
		if status := post("/api/v1/poker/tables/"+testTable+"/access", AccessRequest{Password: strings.Repeat("a", 129)}); status != http.StatusBadRequest || accessCalls.Load() != 0 {
			t.Errorf("129-byte password status=%d port calls=%d", status, accessCalls.Load())
		}
	})
	request := CreateTableRequest{RequestID: "request-00000001", Name: validName, BlindPreset: "5-10", MaxSeats: 2, AllowSpectators: true}
	t.Run("name-40-graphemes-over-128-bytes", func(t *testing.T) {
		if status := post("/api/v1/poker/tables", request); status != http.StatusOK || createCalls.Load() != 1 || receivedName != validName {
			t.Errorf("valid 40-grapheme / %d-byte name status=%d port calls=%d", len(validName), status, createCalls.Load())
		}
	})
	request.Name = strings.Repeat("a", 41)
	beforeCalls := createCalls.Load()
	if status := post("/api/v1/poker/tables", request); status != http.StatusBadRequest || createCalls.Load() != beforeCalls+1 {
		t.Fatalf("domain name rejection not preserved: status=%d calls=%d", status, createCalls.Load())
	}
}
