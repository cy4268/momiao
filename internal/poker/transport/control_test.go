package transport

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestControlWireAcknowledgementAndSnapshotAreConnectionBound(t *testing.T) {
	b := newSynthetic()
	_, s := newServer(t, b, nil)
	p := dialPeer(t, s, testTable, 1)
	ack := next(t, p, "auth.accepted")
	var accepted struct {
		RequestID    string `json:"request_id"`
		ConnectionID string `json:"connection_id"`
	}
	if strictJSON(ack.Payload, &accepted, "request_id", "connection_id") != nil || !validConnectionID(accepted.ConnectionID) {
		t.Fatal("missing server connection id")
	}
	snapshot := next(t, p, "table.snapshot")
	var body struct {
		Viewer struct {
			Control *ControlView `json:"control"`
		} `json:"viewer"`
	}
	if json.Unmarshal(snapshot.Payload, &body) != nil || body.Viewer.Control == nil || body.Viewer.Control.ConnectionID != accepted.ConnectionID || body.Viewer.Control.Mode != "CONTROLLER" {
		t.Fatal("snapshot did not confirm exact live owner")
	}
	second := dialPeer(t, s, testTable, 1)
	otherAck := next(t, second, "auth.accepted")
	if string(otherAck.Payload) == string(ack.Payload) {
		t.Fatal("reconnect reused server id")
	}
	if json.Unmarshal(next(t, second, "table.snapshot").Payload, &body) != nil || body.Viewer.Control == nil || body.Viewer.Control.Mode != "READ_ONLY" {
		t.Fatal("second socket inherited control from ticket")
	}
}

func controlConnectionID(t *testing.T, p *peer) string {
	t.Helper()
	var body struct {
		ConnectionID string `json:"connection_id"`
	}
	if json.Unmarshal(next(t, p, "auth.accepted").Payload, &body) != nil || !validConnectionID(body.ConnectionID) {
		t.Fatal("invalid auth ack")
	}
	next(t, p, "table.snapshot")
	return body.ConnectionID
}
func postControl(t *testing.T, url string, body any, scope string) int {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", testOrigin)
	r.Header.Set("Authorization", "Bearer synthetic-http")
	r.Header.Set("X-Synthetic-Scope", scope)
	response, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return response.StatusCode
}
func mutateSyntheticViewer(s *Snapshot, change func(map[string]any)) {
	var body map[string]any
	_ = json.Unmarshal(s.Payload, &body)
	viewer := body["viewer"].(map[string]any)
	change(viewer)
	s.Payload, _ = json.Marshal(body)
}

func TestControlTakeoverTargetsSameLiveAuthChainAndNotAnotherTable(t *testing.T) {
	b := newSynthetic()
	var calls atomic.Int32
	h, s := newServer(t, b, func(o *Options) {
		originalAuth := o.AuthHTTP
		o.AuthHTTP = func(r *http.Request) (Principal, error) {
			p, err := originalAuth(r)
			switch r.Header.Get("X-Synthetic-Scope") {
			case "user":
				p.UserID++
			case "session-hash":
				p.SessionIDHash += "-other"
			case "session-version":
				p.SessionVersion++
			case "security-epoch":
				p.SecurityEpoch++
			}
			return p, err
		}
		originalTicket := o.AuthenticateTicket
		o.AuthenticateTicket = func(ctx context.Context, r ConnectRequest) (Principal, error) {
			r.TableID = testTable
			return originalTicket(ctx, r)
		}
		originalSnapshot := o.Snapshot
		o.Snapshot = func(ctx context.Context, p Principal, r ConnectionRef) (Snapshot, error) {
			s, err := originalSnapshot(ctx, p, r)
			if r.TableID == testOther && s.Control != nil {
				s.Control.SessionID = testOther
				mutateSyntheticViewer(&s, func(v map[string]any) { v["session_id"] = testOther; v["control"] = s.Control })
			}
			return s, err
		}
		o.Ports.TakeOver = func(ctx context.Context, p Principal, session string, request TakeOverRequest) (json.RawMessage, error) {
			calls.Add(1)
			if request.Target.ID != request.ConnectionID || request.Target.TableID != testTable || session != testPokerSession {
				return nil, errInvalid
			}
			return json.RawMessage(`{"status":"ACCEPTED"}`), nil
		}
	})
	p := dialPeer(t, s, testTable, 1)
	id := controlConnectionID(t, p)
	endpoint := s.URL + "/api/v1/poker/sessions/" + testPokerSession + "/take-over"
	body := map[string]any{"request_id": "request-00000001", "connection_id": id}
	for _, scope := range []string{"user", "session-hash", "session-version", "security-epoch"} {
		if status := postControl(t, endpoint, body, scope); status != 403 {
			t.Fatalf("cross %s target accepted: %d", scope, status)
		}
	}
	other := dialPeer(t, s, testOther, 1)
	otherID := controlConnectionID(t, other)
	body["connection_id"] = otherID
	if status := postControl(t, endpoint, body, ""); status != 403 {
		t.Fatalf("other-table target accepted: %d", status)
	}
	body["connection_id"] = strings.Repeat("f", 32)
	if status := postControl(t, endpoint, body, ""); status != 403 {
		t.Fatal("unknown target accepted")
	}
	body["connection_id"] = id
	body["user_id"] = 1
	if status := postControl(t, endpoint, body, ""); status != 400 {
		t.Fatal("client-selected identity accepted")
	}
	delete(body, "user_id")
	if calls.Load() != 0 {
		t.Fatal("denied target entered domain")
	}
	if status := postControl(t, endpoint, body, ""); status != 200 || calls.Load() != 1 {
		t.Fatal("verified target was not forwarded")
	}
	_ = p.c.CloseNow()
	deadline := time.Now().Add(time.Second)
	for {
		h.mu.Lock()
		_, live := h.live[id]
		h.mu.Unlock()
		if !live {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("closed target still registered")
		}
		time.Sleep(time.Millisecond)
	}
	if status := postControl(t, endpoint, body, ""); status != 403 || calls.Load() != 1 {
		t.Fatal("closed target reentered domain")
	}
	reconnected := dialPeer(t, s, testTable, 1)
	newID := controlConnectionID(t, reconnected)
	if newID == id {
		t.Fatal("reconnect inherited old connection id")
	}
	h.NotifyControlChanged(ConnectionRef{ID: id, TableID: testTable})
	noEvent(t, reconnected)
}

func TestControlOwnerChangesWithoutCommitRefreshOldAndNewSockets(t *testing.T) {
	b := newSynthetic()
	var h *Handler
	var oldID, newID string
	h, s := newServer(t, b, func(o *Options) {
		o.Ports.TakeOver = func(ctx context.Context, p Principal, session string, request TakeOverRequest) (json.RawMessage, error) {
			b.mu.Lock()
			b.controllers[p.UserID] = request.Target.ID
			b.controlEpoch++
			b.mu.Unlock()
			h.NotifyControlChanged(ConnectionRef{ID: oldID, TableID: testTable})
			h.NotifyControlChanged(request.Target)
			return json.RawMessage(`{"status":"APPLIED"}`), nil
		}
	})
	old := dialPeer(t, s, testTable, 1)
	oldID = controlConnectionID(t, old)
	current := dialPeer(t, s, testTable, 1)
	newID = controlConnectionID(t, current)
	if status := postControl(t, s.URL+"/api/v1/poker/sessions/"+testPokerSession+"/take-over", map[string]any{"request_id": "request-00000001", "connection_id": newID}, ""); status != 200 {
		t.Fatalf("takeover %d", status)
	}
	for _, tc := range []struct {
		peer     *peer
		id, mode string
	}{{old, oldID, "READ_ONLY"}, {current, newID, "CONTROLLER"}} {
		changed := next(t, tc.peer, "control.changed")
		var view ControlView
		if strictJSON(changed.Payload, &view, "connection_id", "session_id", "mode", "control_epoch") != nil || view.ConnectionID != tc.id || view.Mode != tc.mode || view.ControlEpoch != "2" {
			t.Fatalf("wrong control projection: %s", changed.Payload)
		}
		full := next(t, tc.peer, "table.snapshot")
		if full.TableVersion != 1 {
			t.Fatal("control wakeup fabricated a commit version")
		}
	}
	msg := clientMessage("hand.action", testTable, map[string]any{"action_type": "CHECK"})
	msg["hand_id"] = testHand
	msg["action_id"] = "action-000000001"
	msg["control_epoch"] = 1
	send(t, old, msg)
	next(t, old, "error")
	msg["control_epoch"] = 2
	send(t, current, msg)
	next(t, current, "service.notice")
	b.mu.Lock()
	if b.actionCalls != 1 || b.lastAction.Control.ConnectionID != newID || b.lastAction.Control.SessionID != testPokerSession || b.lastAction.Control.ControlEpoch != 2 {
		t.Fatal("domain did not receive exact verified control tuple")
	}
	b.mu.Unlock()
	h.NotifyControlChanged(ConnectionRef{ID: newID, TableID: testTable})
	noEvent(t, current)
	// An external owner loss with no PG version change still revokes the UI.
	b.mu.Lock()
	delete(b.controllers, 1)
	b.mu.Unlock()
	h.NotifyControlChanged(ConnectionRef{ID: newID, TableID: testTable})
	next(t, current, "control.changed")
	next(t, current, "table.snapshot")
	send(t, current, msg)
	next(t, current, "error")
}

func TestControlProjectionMismatchClosesInsteadOfGranting(t *testing.T) {
	for _, kind := range []string{"missing", "connection", "session", "epoch", "mode"} {
		t.Run(kind, func(t *testing.T) {
			_, s := newServer(t, newSynthetic(), func(o *Options) {
				original := o.Snapshot
				o.Snapshot = func(ctx context.Context, p Principal, r ConnectionRef) (Snapshot, error) {
					s, err := original(ctx, p, r)
					switch kind {
					case "missing":
						s.Control = nil
					case "connection":
						s.Control.ConnectionID = strings.Repeat("f", 32)
					case "session":
						s.Control.SessionID = testOther
					case "epoch":
						s.Control.ControlEpoch = "2"
					case "mode":
						s.Control.Mode = "ROOT"
					}
					if kind != "missing" {
						mutateSyntheticViewer(&s, func(v map[string]any) { v["control"] = s.Control })
					}
					return s, err
				}
			})
			p := dialPeer(t, s, testTable, 1)
			expectClosed(t, p)
		})
	}
}

func TestControlReadonlyTicketCannotUpgradeThroughTakeover(t *testing.T) {
	b := newSynthetic()
	var calls atomic.Int32
	_, s := newServer(t, b, func(o *Options) {
		original := o.AuthenticateTicket
		o.AuthenticateTicket = func(ctx context.Context, r ConnectRequest) (Principal, error) {
			p, err := original(ctx, r)
			p.ControlIntent = "READ_ONLY"
			return p, err
		}
		o.Ports.TakeOver = func(context.Context, Principal, string, TakeOverRequest) (json.RawMessage, error) {
			calls.Add(1)
			return json.RawMessage(`{}`), nil
		}
	})
	p := dialPeer(t, s, testTable, 1)
	id := controlConnectionID(t, p)
	if status := postControl(t, s.URL+"/api/v1/poker/sessions/"+testPokerSession+"/take-over", map[string]any{"request_id": "request-00000001", "connection_id": id}, ""); status != 403 || calls.Load() != 0 {
		t.Fatalf("READ_ONLY upgraded: status=%d calls=%d", status, calls.Load())
	}
}

func TestControlSnapshotHTTPIsReadonlyAndHasNoFakeConnection(t *testing.T) {
	b := newSynthetic()
	_, s := newServer(t, b, nil)
	r, _ := http.NewRequest(http.MethodGet, s.URL+"/api/v1/poker/tables/"+testTable, nil)
	r.Header.Set("Authorization", "Bearer synthetic-http")
	response, err := s.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body struct {
		Data struct {
			Viewer struct {
				CanAct  bool            `json:"can_act"`
				Control json.RawMessage `json:"control"`
			} `json:"viewer"`
		} `json:"data"`
	}
	if response.StatusCode != 200 || json.NewDecoder(response.Body).Decode(&body) != nil || body.Data.Viewer.CanAct || len(body.Data.Viewer.Control) != 0 {
		t.Fatal("HTTP fabricated a controlling socket")
	}
}

func TestControlReturnedRefMustMatchCurrentVerifiedTuple(t *testing.T) {
	for _, kind := range []string{"user", "session-hash", "session-version", "security-epoch", "connection", "table", "poker-session", "runtime", "epoch"} {
		t.Run(kind, func(t *testing.T) {
			b := newSynthetic()
			_, s := newServer(t, b, func(o *Options) {
				original := o.AuthorizeControl
				o.AuthorizeControl = func(ctx context.Context, p Principal, c ConnectionRef, epoch uint64) (ControlRef, error) {
					ref, err := original(ctx, p, c, epoch)
					switch kind {
					case "user":
						ref.UserID++
					case "session-hash":
						ref.SessionIDHash += "-other"
					case "session-version":
						ref.SessionVersion++
					case "security-epoch":
						ref.SecurityEpoch++
					case "connection":
						ref.ConnectionID = strings.Repeat("f", 32)
					case "table":
						ref.TableID = testOther
					case "poker-session":
						ref.SessionID = testOther
					case "runtime":
						ref.RuntimeEpoch++
					case "epoch":
						ref.ControlEpoch++
					}
					return ref, err
				}
			})
			p := dialPeer(t, s, testTable, 1)
			authenticated(t, p)
			msg := clientMessage("hand.action", testTable, map[string]any{"action_type": "CHECK"})
			msg["hand_id"] = testHand
			msg["action_id"] = "action-000000001"
			msg["control_epoch"] = 1
			send(t, p, msg)
			if !strings.Contains(string(next(t, p, "error").Payload), "CONTROL_BINDING_MISMATCH") {
				t.Fatal("mismatched server control ref reached domain")
			}
			b.mu.Lock()
			defer b.mu.Unlock()
			if b.actionCalls != 0 {
				t.Fatal("untrusted tuple dispatched")
			}
		})
	}
}

func TestControlFourMutationPortsReceiveVerifiedRefAndOriginalGuards(t *testing.T) {
	type received struct {
		ref                       ControlRef
		tableVersion, handVersion uint64
		hand                      *string
	}
	seen := make(chan received, 4)
	b := newSynthetic()
	_, s := newServer(t, b, func(o *Options) {
		o.Ports.Act = func(ctx context.Context, p Principal, a HandAction) (json.RawMessage, error) {
			seen <- received{a.Control, a.ExpectedTableVersion, a.ExpectedHandVersion, &a.HandID}
			return json.RawMessage(`{}`), nil
		}
		session := func(ctx context.Context, p Principal, a SessionAction) (json.RawMessage, error) {
			seen <- received{a.Control, a.ExpectedTableVersion, a.ExpectedHandVersion, a.HandID}
			return json.RawMessage(`{}`), nil
		}
		o.Ports.SitOut = session
		o.Ports.Resume = session
		o.Ports.SetClientSeed = func(ctx context.Context, p Principal, a ClientSeed) (json.RawMessage, error) {
			seen <- received{a.Control, a.ExpectedTableVersion, a.ExpectedHandVersion, a.HandID}
			return json.RawMessage(`{}`), nil
		}
	})
	p := dialPeer(t, s, testTable, 1)
	id := controlConnectionID(t, p)
	for _, kind := range []string{"hand.action", "session.sit_out_next_hand", "session.resume_play", "client_seed.set_next"} {
		payload := map[string]any{}
		if kind == "hand.action" {
			payload["action_type"] = "CHECK"
		}
		if kind == "client_seed.set_next" {
			payload["client_seed"] = "next-seed"
		}
		msg := clientMessage(kind, testTable, payload)
		msg["hand_id"] = testHand
		msg["action_id"] = "action-000000001"
		msg["expected_table_version"] = 11
		msg["expected_hand_version"] = 7
		msg["control_epoch"] = 1
		send(t, p, msg)
		next(t, p, "service.notice")
		got := <-seen
		if got.ref.ConnectionID != id || got.ref.UserID != 1 || got.ref.SessionIDHash != "synthetic-session-1" || got.ref.SessionVersion != 1 || got.ref.SecurityEpoch != 0 || got.ref.SessionID != testPokerSession || got.ref.TableID != testTable || got.ref.RuntimeEpoch != 1 || got.ref.ControlEpoch != 1 || got.tableVersion != 11 || got.handVersion != 7 || got.hand == nil || *got.hand != testHand {
			t.Fatalf("%s lost authoritative ref or optimistic guard: %+v", kind, got)
		}
	}
}
