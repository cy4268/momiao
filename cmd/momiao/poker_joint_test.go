package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/cy4268/momiao/internal/poker"
	"github.com/cy4268/momiao/internal/poker/connectticket"
	pt "github.com/cy4268/momiao/internal/poker/transport"
)

// This test has no auth callbacks, control mocks, memory replay consumer or
// replacement NativeReader. Its sole opt-in input is the real M2 native server.
type appNativeChain struct {
	AccessToken    string `json:"access_token"`
	RawSID         string `json:"raw_sid"`
	SessionIDHash  string `json:"session_id_hash"`
	SessionVersion string `json:"session_version"`
}
type appNativeUser struct {
	UserID   string           `json:"user_id"`
	Username string           `json:"username"`
	Password string           `json:"password"`
	Chains   []appNativeChain `json:"chains"`
}

func (appNativeChain) String() string { return "appNativeChain<redacted>" }
func (appNativeUser) String() string  { return "appNativeUser<redacted>" }
func (u appNativeUser) headers(index int) http.Header {
	return http.Header{"Authorization": {"Bearer " + u.Chains[index].AccessToken}, "X-Auth-Session": {u.Chains[index].RawSID}, "New-Api-User": {u.UserID}}
}

type appWireEvent struct {
	Type         string          `json:"type"`
	TableID      string          `json:"table_id"`
	TableVersion uint64          `json:"table_version"`
	Payload      json.RawMessage `json:"payload"`
}
type appPeerResult struct {
	event appWireEvent
	err   error
}
type appPeer struct {
	c         *websocket.Conn
	id, table string
	last      poker.TableView
	events    chan appPeerResult
	done      chan struct{}
}

func appMessage(kind, table string, payload any) map[string]any {
	return map[string]any{"type": kind, "request_id": "app-request-00000001", "table_id": table, "hand_id": nil, "expected_table_version": 0, "expected_hand_version": 0, "control_epoch": 0, "action_id": nil, "payload": payload}
}
func (p *appPeer) send(t *testing.T, value any) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal("message encoding failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = p.c.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatal("socket write failed")
	}
}
func appDial(t *testing.T, server *httptest.Server, table, ticket string) *appPeer {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws/poker", &websocket.DialOptions{Subprotocols: []string{pt.Subprotocol}, HTTPHeader: http.Header{"Origin": {"http://127.0.0.1:32000"}}})
	if err != nil {
		t.Fatal("real application WS upgrade failed")
	}
	p := &appPeer{c: c, table: table, events: make(chan appPeerResult, 256), done: make(chan struct{})}
	readCtx, stop := context.WithCancel(context.Background())
	t.Cleanup(func() { stop(); _ = c.CloseNow(); <-p.done })
	go func() {
		defer close(p.done)
		for {
			_, b, e := c.Read(readCtx)
			r := appPeerResult{err: e}
			if e == nil {
				r.err = json.Unmarshal(b, &r.event)
			}
			select {
			case p.events <- r:
			case <-readCtx.Done():
				return
			}
			if r.err != nil {
				return
			}
		}
	}()
	p.send(t, appMessage("auth.connect", table, map[string]string{"poker_connect_ticket": ticket}))
	return p
}
func (p *appPeer) until(t *testing.T, predicate func(appWireEvent) bool) appWireEvent {
	t.Helper()
	timer := time.NewTimer(8 * time.Second)
	defer timer.Stop()
	for {
		select {
		case r := <-p.events:
			if r.err != nil {
				t.Fatalf("socket ended before expected application event: status=%d", websocket.CloseStatus(r.err))
			}
			if r.event.Type == "table.snapshot" {
				if json.Unmarshal(r.event.Payload, &p.last) != nil {
					t.Fatal("table view malformed")
				}
			}
			if predicate(r.event) {
				return r.event
			}
		case <-timer.C:
			t.Fatal("application event deadline exceeded")
		}
	}
}
func (p *appPeer) accept(t *testing.T) {
	t.Helper()
	e := p.until(t, func(e appWireEvent) bool { return e.Type == "auth.accepted" })
	var b struct {
		ID string `json:"connection_id"`
	}
	if json.Unmarshal(e.Payload, &b) != nil || len(b.ID) != 32 {
		t.Fatal("real server connection id absent")
	}
	p.id = b.ID
	p.until(t, func(e appWireEvent) bool { return e.Type == "table.snapshot" })
}
func (p *appPeer) snapshot(t *testing.T, version uint64, mode string) poker.TableView {
	t.Helper()
	p.send(t, appMessage("sync.request", p.table, map[string]any{}))
	// A matching pong follows this explicit sync in the socket's serial input
	// queue, so an earlier queued broadcast cannot masquerade as its response.
	marker := "joint-sync-marker-" + appRandom(t, 8)
	ping := appMessage("ping", p.table, map[string]any{})
	ping["request_id"] = marker
	p.send(t, ping)
	p.until(t, func(e appWireEvent) bool {
		var b struct {
			Request string `json:"request_id"`
		}
		return e.Type == "pong" && json.Unmarshal(e.Payload, &b) == nil && b.Request == marker
	})
	v, err := strconv.ParseUint(p.last.TableVersion, 10, 64)
	if err != nil || v < version || mode != "" && (p.last.Viewer.Control == nil || p.last.Viewer.Control.Mode != mode) {
		t.Fatal("fresh sync did not confirm expected authoritative version/control")
	}
	return p.last
}
func (p *appPeer) closed(t *testing.T, reason string) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case r := <-p.events:
			if r.err != nil {
				if websocket.CloseStatus(r.err) != websocket.StatusPolicyViolation || !strings.Contains(r.err.Error(), reason) {
					t.Fatal("unexpected policy-close status/reason")
				}
				return
			}
		case <-timer.C:
			t.Fatal("revoked/replayed socket remained live")
		}
	}
}
func appAction(p *appPeer, kind, key string, payload any) map[string]any {
	m := appMessage(kind, p.table, payload)
	m["action_id"] = key
	m["request_id"] = "request-" + key
	v, _ := strconv.ParseUint(p.last.TableVersion, 10, 64)
	m["expected_table_version"] = v
	if c := p.last.Viewer.Control; c != nil {
		e, _ := strconv.ParseUint(c.ControlEpoch, 10, 64)
		m["control_epoch"] = e
	}
	if h := p.last.Hand; h != nil {
		m["hand_id"] = h.HandID
		v, _ := strconv.ParseUint(h.HandVersion, 10, 64)
		m["expected_hand_version"] = v
	}
	return m
}
func (p *appPeer) result(t *testing.T, request string) (poker.Receipt, string) {
	t.Helper()
	e := p.until(t, func(e appWireEvent) bool {
		if e.Type != "service.notice" && e.Type != "error" {
			return false
		}
		var b struct {
			Request string `json:"request_id"`
		}
		return json.Unmarshal(e.Payload, &b) == nil && b.Request == request
	})
	var b struct {
		Receipt poker.Receipt `json:"receipt"`
		Code    string        `json:"code"`
	}
	if json.Unmarshal(e.Payload, &b) != nil {
		t.Fatal("command outcome malformed")
	}
	return b.Receipt, b.Code
}

func appMint(t *testing.T, f *pokerAppFixture, h http.Handler, headers http.Header, table, intent string) string {
	t.Helper()
	status, data, code := appHTTP(t, h, http.MethodPost, "/api/v1/poker/connect-tickets", map[string]any{"target_table_id": table, "control_intent": intent}, headers)
	var body map[string]string
	if status != 200 || json.Unmarshal(data, &body) != nil || len(body) != 1 || body["poker_connect_ticket"] == "" {
		t.Fatalf("real ticket mint failed: status=%d code=%s", status, code)
	}
	ticket := body["poker_connect_ticket"]
	parts := strings.Split(ticket, ".")
	if len(parts) != 3 || parts[0] != "ct1" {
		t.Fatal("mint did not return ct1")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	var claims connectticket.Claims
	if err != nil || json.Unmarshal(payload, &claims) != nil || claims.JTI == "" {
		t.Fatal("locally issued ticket malformed")
	}
	digest := sha256.Sum256([]byte(claims.JTI))
	f.keys[connectticket.RedisKeyPrefix+hex.EncodeToString(digest[:])] = true
	return ticket
}

func appNativeRequest(t *testing.T, tr http.RoundTripper, method, path string, body any, headers http.Header) (int, []byte) {
	t.Helper()
	var b []byte
	if body != nil {
		var err error
		b, err = json.Marshal(body)
		if err != nil {
			t.Fatal("native request encoding failed")
		}
	}
	r, err := http.NewRequest(method, "http://native"+path, bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	r.Header = headers.Clone()
	if r.Header == nil {
		r.Header = http.Header{}
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://native")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := http.Client{Transport: tr, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(r.WithContext(ctx))
	if err != nil {
		t.Fatal("actual native endpoint unavailable")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(data) > 65536 {
		t.Fatal("native response bounds failed")
	}
	return response.StatusCode, data
}
func appNativeLogin(t *testing.T, tr http.RoundTripper, u appNativeUser) appNativeUser {
	t.Helper()
	status, data := appNativeRequest(t, tr, http.MethodPost, "/api/user/login", map[string]string{"username": u.Username, "password": u.Password}, nil)
	var b struct {
		Success bool `json:"success"`
		Data    struct {
			Token   string `json:"access_token"`
			Session struct {
				SID string `json:"sid"`
			} `json:"session"`
		} `json:"data"`
	}
	if status != 200 || json.Unmarshal(data, &b) != nil || !b.Success || b.Data.Token == "" || b.Data.Session.SID == "" {
		t.Fatal("new real native login failed")
	}
	u.Chains = []appNativeChain{{AccessToken: b.Data.Token, RawSID: b.Data.Session.SID}}
	return u
}

func TestPokerApplicationNativeJoint(t *testing.T) {
	readyPath := os.Getenv("POKER_APP_TEST_NATIVE_READY")
	if readyPath == "" {
		t.Skip("set POKER_APP_TEST_NATIVE_READY for real native joint acceptance")
	}
	var ready struct {
		Socket      string    `json:"socket"`
		Database    string    `json:"database"`
		ReaderKey   string    `json:"reader_key_file"`
		Credentials string    `json:"credentials_file"`
		Deadline    time.Time `json:"deadline"`
	}
	b, err := os.ReadFile(readyPath)
	if err != nil || json.Unmarshal(b, &ready) != nil || !filepath.IsAbs(ready.Socket) || !strings.HasPrefix(ready.Database, "native_poker_joint_") || time.Until(ready.Deadline) < 2*time.Minute {
		t.Fatal("live owned native fixture readiness required")
	}
	var credentials struct {
		appNativeUser
		Second appNativeUser `json:"second_player"`
	}
	b, err = os.ReadFile(ready.Credentials)
	if err != nil || json.Unmarshal(b, &credentials) != nil || len(credentials.Chains) != 2 || len(credentials.Second.Chains) != 1 {
		t.Fatal("private native fixture credentials malformed")
	}
	u1, u2 := credentials.appNativeUser, credentials.Second
	id1, err1 := strconv.ParseInt(u1.UserID, 10, 64)
	id2, err2 := strconv.ParseInt(u2.UserID, 10, 64)
	if err1 != nil || err2 != nil || id1 <= 0 || id2 <= 0 || id1 == id2 {
		t.Fatal("two real ordinary fixture users required")
	}
	f := newPokerAppFixture(t, id1, id2)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signing, _ := json.Marshal(map[string]any{"active": "joint", "private_key": hex.EncodeToString(private), "public_keys": map[string]string{"joint": hex.EncodeToString(public)}})
	state, _ := json.Marshal(map[string]any{"active": "joint", "keys": map[string]string{"joint": appRandom(t, 32)}})
	cfg := config{PublicOrigin: "http://127.0.0.1:32000", WebDir: f.dir, NewAPISocket: ready.Socket, WalletDSNFile: f.authDSN, Poker: pokerConfig{Enabled: true, DSNFile: f.pokerDSN, ReaderKeyFile: ready.ReaderKey, StateKeyringFile: f.write("state.json", state), TicketKeyringFile: f.write("ticket.json", signing), RedisConfigFile: f.redisFile}}
	app, err := openPokerApplication(f.ctx, cfg)
	if err != nil {
		t.Fatal("actual application authority composition failed", err)
	}
	defer func() { app.Close() }()
	cfg.poker = app.handler
	h := newPortalHandler(cfg, app.native)
	server := httptest.NewServer(h)
	defer func() { server.Close() }()
	t.Log("JOINT_COMPOSITION real NativeReader/self + Authority + persistent keyrings + PG runtime + TLS Redis ct1 consume + G3 Start + portal route mounted")
	if status, _, code := appHTTP(t, h, http.MethodGet, "/api/v1/poker/sessions/active", nil, nil); status != 401 || code != "POKER_AUTH_UNAUTHORIZED" {
		t.Fatal("missing native credential passed mounted route")
	}
	spoof := u1.headers(0)
	parts := strings.Split(u1.Chains[0].AccessToken, ".")
	if len(parts) != 3 {
		t.Fatal("native token shape")
	}
	parts[2] = strings.Repeat("a", len(parts[2]))
	spoof.Set("Authorization", "Bearer "+strings.Join(parts, "."))
	if status, _, _ := appHTTP(t, h, http.MethodGet, "/api/v1/poker/sessions/active", nil, spoof); status != 401 && status != 403 {
		t.Fatal("unverified native claims accepted")
	}
	created := appReceipt(t, f, h, "/api/v1/poker/tables", map[string]any{"request_id": "joint-create-table-001", "name": "Real native joint table", "blind_preset": "5-10", "max_seats": 2, "allow_spectators": true}, u1.headers(0))
	table := created.TableID
	newChain := appNativeLogin(t, app.native, u1)
	connect := func(user appNativeUser, intent string) *appPeer {
		p := appDial(t, server, table, appMint(t, f, h, user.headers(0), table, intent))
		p.accept(t)
		return p
	}
	ticket := appMint(t, f, h, u1.headers(0), table, "CLAIM_CONTROL")
	p1 := appDial(t, server, table, ticket)
	p1.accept(t)
	replay := appDial(t, server, table, ticket)
	replay.closed(t, "TICKET_REPLAYED")
	p1second := connect(u1, "CLAIM_CONTROL")
	readonly := connect(u1, "READ_ONLY")
	otherChain := connect(newChain, "CLAIM_CONTROL")
	p2 := connect(u2, "CLAIM_CONTROL")
	for _, p := range []*appPeer{p1, p1second, readonly, otherChain, p2} {
		if p.last.Viewer.Control == nil || p.last.Viewer.Control.Mode != "READ_ONLY" {
			t.Fatal("unseated socket received controller grant")
		}
	}
	buy := func(user appNativeUser, seat int, key string) poker.Receipt {
		r := appReceipt(t, f, h, "/api/v1/poker/tables/"+table+"/seat-reservations", map[string]any{"request_id": "joint-reserve-" + key, "seat_no": seat}, user.headers(0))
		return appReceipt(t, f, h, "/api/v1/poker/tables/"+table+"/buy-ins", map[string]any{"request_id": "joint-buy-in-" + key, "reservation_id": r.ReservationID, "amount_units": "200000000"}, user.headers(0))
	}
	bought1 := buy(u1, 1, "primary-001")
	p1.snapshot(t, bought1.Version, "CONTROLLER")
	for _, p := range []*appPeer{p1second, readonly, otherChain} {
		p.snapshot(t, bought1.Version, "READ_ONLY")
	}
	if p1.last.Viewer.Control.ConnectionID != p1.id || p1.last.Viewer.Control.SessionID != bought1.SessionID {
		t.Fatal("BuyIn did not rebind the original authenticated claim socket")
	}
	t.Log("JOINT_BUYIN_REBIND original earliest same-chain CLAIM socket controls; second original, READ_ONLY and different native chain remain observers; Redis ticket replay rejected")
	for i, kind := range []string{"hand.action", "session.sit_out_next_hand", "session.resume_play", "client_seed.set_next"} {
		payload := map[string]any{}
		if kind == "hand.action" {
			payload["action_type"] = "FOLD"
		}
		if kind == "client_seed.set_next" {
			payload["client_seed"] = "joint-ignored-seed"
		}
		m := appAction(readonly, kind, "readonly-reject-000"+strconv.Itoa(i), payload)
		readonly.send(t, m)
		_, code := readonly.result(t, m["request_id"].(string))
		if code != "POKER_CONNECTION_READ_ONLY" {
			t.Fatal("read-only four-command gate failed", kind, code)
		}
	}
	// Funding is an owner/session HTTP command, independent of socket control.
	top := appReceipt(t, f, h, "/api/v1/poker/sessions/"+bought1.SessionID+"/top-ups", map[string]any{"request_id": "joint-http-top-up-001", "amount_units": "5000000"}, newChain.headers(0))
	if top.Status != "CONFIRMED" {
		t.Fatal("owner HTTP top-up unnecessarily required a controller")
	}
	status, _, _ := appHTTP(t, h, http.MethodPost, "/api/v1/poker/sessions/"+bought1.SessionID+"/take-over", map[string]any{"request_id": "joint-cross-chain-takeover", "connection_id": otherChain.id}, u1.headers(0))
	if status != 403 {
		t.Fatal("HTTP takeover targeted a different native chain")
	}
	take := appReceipt(t, f, h, "/api/v1/poker/sessions/"+bought1.SessionID+"/take-over", map[string]any{"request_id": "joint-take-over-001", "connection_id": p1second.id}, u1.headers(0))
	if take.ControlEpoch == "" {
		t.Fatal("takeover durable receipt metadata missing")
	}
	p1second.snapshot(t, take.Version, "CONTROLLER")
	p1.snapshot(t, take.Version, "READ_ONLY")
	t.Log("JOINT_TAKEOVER HTTP returns durable Receipt only; exact target control.changed/snapshot grants and old controller demotion observed")
	for _, v := range []struct {
		kind, key string
		payload   map[string]any
	}{{"session.sit_out_next_hand", "joint-sitout-action-001", map[string]any{}}, {"session.resume_play", "joint-resume-action-001", map[string]any{}}, {"client_seed.set_next", "joint-next-seed-action-001", map[string]any{"client_seed": "joint-next-private-contribution"}}} {
		p1second.snapshot(t, 0, "CONTROLLER")
		m := appAction(p1second, v.kind, v.key, v.payload)
		p1second.send(t, m)
		r, code := p1second.result(t, m["request_id"].(string))
		if code != "" || r.SessionID != bought1.SessionID {
			t.Fatal("typed controller command failed", v.kind, code)
		}
		p1second.snapshot(t, r.Version, "CONTROLLER")
		if v.kind == "client_seed.set_next" {
			retry := appAction(p1second, v.kind, v.key, v.payload)
			retry["request_id"] = "joint-retry-seed-request"
			p1second.send(t, retry)
			dup, code := p1second.result(t, retry["request_id"].(string))
			if code != "" || !dup.Duplicate || dup.Version != r.Version {
				t.Fatal("action_id idempotency lost after different request_id/current version")
			}
		}
	}
	bought2 := buy(u2, 2, "secondary-001")
	p2.snapshot(t, bought2.Version, "CONTROLLER")
	// Wait for the real actor's one-second lifecycle; no public/internal manual
	// StartHand, SetConnected or TakeControl is used to manufacture this hand.
	if p2.last.Hand == nil || p2.last.Hand.Street != "PREFLOP" {
		p2.until(t, func(e appWireEvent) bool {
			return e.Type == "table.snapshot" && p2.last.Hand != nil && p2.last.Hand.Street == "PREFLOP"
		})
	}
	p1second.snapshot(t, 0, "CONTROLLER")
	for _, p := range []*appPeer{p1second, p2} {
		own := 0
		for _, seat := range p.last.Seats {
			if seat.IsSelf {
				own = len(seat.HoleCards)
			} else if len(seat.HoleCards) != 0 {
				t.Fatal("private hand leaked across real native users")
			}
		}
		if own != 2 {
			t.Fatal("owner private hand unavailable")
		}
	}
	actor := p1second
	if p2.last.Hand.ActorSeat == 2 {
		actor = p2
	}
	actor.snapshot(t, 0, "CONTROLLER")
	hand := actor.last.Hand.HandID
	m := appAction(actor, "hand.action", "joint-fold-action-001", map[string]any{"action_type": "FOLD"})
	actor.send(t, m)
	fold, code := actor.result(t, m["request_id"].(string))
	if code != "" || fold.HandID != hand {
		t.Fatal("actual controlled hand action failed", code)
	}
	appReceipt(t, f, h, "/api/v1/poker/sessions/"+bought1.SessionID+"/safe-leave", map[string]any{"request_id": "joint-safe-leave-primary"}, u1.headers(0))
	appReceipt(t, f, h, "/api/v1/poker/sessions/"+bought2.SessionID+"/safe-leave", map[string]any{"request_id": "joint-safe-leave-secondary"}, u2.headers(0))
	var wallet, stack, settled int64
	if err = f.owner.QueryRow(f.ctx, "SELECT (SELECT sum(balance_units) FROM economy.wallet_balances WHERE asset_type='AVAILABLE_CHIPS'),(SELECT sum(current_stack_units) FROM poker.sessions),(SELECT count(*) FROM poker.sessions WHERE state='SETTLED')").Scan(&wallet, &stack, &settled); err != nil || wallet != 3000000000 || stack != 0 || settled != 2 {
		t.Fatal("real two-player ledger closure failed")
	}
	var funding, buys, topups, cashouts, ledger, net, linked int64
	err = f.owner.QueryRow(f.ctx, `SELECT count(*),count(*) FILTER(WHERE kind='BUY_IN'),count(*) FILTER(WHERE kind='TOP_UP'),count(*) FILTER(WHERE kind='CASH_OUT'),
 (SELECT count(*) FROM economy.wallet_ledger WHERE biz_type='POKER_FUNDING'),
 (SELECT coalesce(sum(delta_units),0) FROM economy.wallet_ledger WHERE biz_type='POKER_FUNDING'),
 (SELECT count(DISTINCT l.ledger_entry_id) FROM poker.funding_operations f JOIN economy.wallet_ledger l ON l.ledger_entry_id=f.confirmed_ledger_id AND l.transaction_id=f.confirmed_transaction_id WHERE f.state='CONFIRMED')
 FROM poker.funding_operations WHERE state='CONFIRMED'`).Scan(&funding, &buys, &topups, &cashouts, &ledger, &net, &linked)
	if err != nil || funding != 5 || buys != 2 || topups != 1 || cashouts != 2 || ledger != 5 || net != 0 || linked != 5 {
		t.Fatal("five funding operations do not have exact one-to-one zero-net wallet ledger closure")
	}
	t.Log("JOINT_HAND real actor auto-start + per-user private hole cards + typed WS Act + HTTP safe-leave; two sessions settled, wallet total=3000000000 stack=0; funding=5 (2 buy-in,1 top-up,2 cash-out), ledger=5, linked=5, net=0")
	for _, u := range []appNativeUser{u1, u2} {
		if status, _, code := appHTTP(t, h, http.MethodGet, "/api/v1/poker/hands/"+hand+"/fairness", nil, u.headers(0)); status != 200 {
			t.Fatalf("settled fairness route failed: %d %s", status, code)
		}
	}
	beforeRestart := appMint(t, f, h, u1.headers(0), table, "READ_ONLY")
	app.Close()
	server.Close()
	app, err = openPokerApplication(f.ctx, cfg)
	if err != nil {
		t.Fatal("same persistent authorities restart failed")
	}
	cfg.poker = app.handler
	h = newPortalHandler(cfg, app.native)
	server = httptest.NewServer(h)
	appDial(t, server, table, beforeRestart).closed(t, "POKER_TICKET_RESTART_FENCED")
	pAfter := appDial(t, server, table, appMint(t, f, h, u1.headers(0), table, "READ_ONLY"))
	pAfter.accept(t)
	t.Log("JOINT_RESTART same persistent keyring reopened; pre-start ticket fenced; fresh real-native mint accepted")
	// Real PG epoch trigger invalidates both the bound old chain and a second
	// native chain created before the epoch but never bound into this platform.
	if _, err = f.owner.Exec(f.ctx, "UPDATE identity.account_refs SET security_epoch=security_epoch+1 WHERE newapi_user_id=$1", id1); err != nil {
		t.Fatal("owned fixture security epoch transition failed")
	}
	for _, index := range []int{0, 1} {
		if status, _, _ := appHTTP(t, h, http.MethodGet, "/api/v1/poker/sessions/active", nil, u1.headers(index)); status != 401 {
			t.Fatal("old native chain rebound across platform epoch", index, status)
		}
	}
	pAfter.send(t, appMessage("ping", table, map[string]any{}))
	pAfter.closed(t, "SESSION_INVALID")
	time.Sleep(1100 * time.Millisecond)
	fresh := appNativeLogin(t, app.native, u1)
	freshTicket := appMint(t, f, h, fresh.headers(0), table, "READ_ONLY")
	pFresh := appDial(t, server, table, freshTicket)
	pFresh.accept(t)
	logoutTicket := appMint(t, f, h, fresh.headers(0), table, "READ_ONLY")
	status, data := appNativeRequest(t, app.native, http.MethodPost, "/api/user/auth/logout", map[string]any{}, fresh.headers(0))
	var logout struct {
		Success bool `json:"success"`
	}
	if status != 200 || json.Unmarshal(data, &logout) != nil || !logout.Success {
		t.Fatal("real native logout failed")
	}
	appDial(t, server, table, logoutTicket).closed(t, "POKER_SESSION_REVOKED")
	pFresh.send(t, appMessage("ping", table, map[string]any{}))
	pFresh.closed(t, "SESSION_INVALID")
	if status, _, _ := appHTTP(t, h, http.MethodGet, "/api/v1/poker/sessions/active", nil, fresh.headers(0)); status != 401 && status != 403 {
		t.Fatal("native-logged-out HTTP credential remained valid")
	}
	if status, _, _ := appHTTP(t, h, http.MethodGet, "/api/v1/poker/sessions/active", nil, u2.headers(0)); status != 200 {
		t.Fatal("primary user revocation affected distinct second user")
	}
	t.Log("JOINT_REVOCATION real PG epoch denies old bound/unbound chains; actual post-epoch login works; real native logout denies HTTP, unused ct1 and active WS; second user unaffected")
}
