package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker"
	"github.com/cy4268/momiao/internal/poker/redislease"
	pt "github.com/cy4268/momiao/internal/poker/transport"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type lobbyDeniedLease struct{ calls atomic.Int32 }

func (p *lobbyDeniedLease) Acquire(context.Context, string, int, string, time.Time) (bool, error) {
	p.calls.Add(1)
	return false, poker.ErrDenied
}
func (p *lobbyDeniedLease) Valid(context.Context, string, int, string, time.Time) (bool, error) {
	p.calls.Add(1)
	return false, poker.ErrDenied
}
func (p *lobbyDeniedLease) Release(context.Context, string, int, string) error {
	p.calls.Add(1)
	return poker.ErrDenied
}

func TestPokerLobbyHTTPReadOnlyMaintenanceBoundary(t *testing.T) {
	f := newPokerAppFixture(t, 910001, 910002, 910003)
	lease, _, err := readPokerRedisConfig(f.redisFile)
	if err != nil {
		t.Fatal(err)
	}
	keys := poker.Keyring{Current: "lobby", Keys: map[string][]byte{"lobby": make([]byte, 32)}}
	s, err := poker.NewWithRedis(f.ctx, poker.Options{Pool: f.domain, Keyring: keys, ValidateSession: func(context.Context, poker.AuthSession) error { return nil }}, lease)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := f.domain.Config()
	c.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	pool, err := pgxpool.NewWithConfig(f.ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var readOnly string
	if err = pool.QueryRow(f.ctx, "SHOW default_transaction_read_only").Scan(&readOnly); err != nil || readOnly != "on" {
		t.Fatal("actual readonly pool missing")
	}
	_, err = pool.Exec(f.ctx, "UPDATE poker.tables SET name=name WHERE false")
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "25006" {
		t.Fatal("real readonly write denial missing", err)
	}
	denied := &lobbyDeniedLease{}
	reader, err := poker.New(poker.Options{Pool: pool, Keyring: keys, Leases: denied})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	ports := (&pokerAdapter{service: s}).ports()
	reads := (&pokerAdapter{service: reader}).ports()
	ports.ReadLobby, ports.ReadTables, ports.ReadReceipt = reads.ReadLobby, reads.ReadTables, reads.ReadReceipt
	var forbidden atomic.Int32
	h, err := pt.New(pt.Options{Origin: "http://127.0.0.1:32000", AuthHTTP: func(r *http.Request) (pt.Principal, error) {
		id, e := strconv.ParseInt(r.Header.Get("X-Synthetic-User"), 10, 64)
		if e != nil || id != 910001 && id != 910002 && id != 910003 && id != 919999 {
			return pt.Principal{}, &pt.Fault{Status: 401, Code: "AUTH_REQUIRED"}
		}
		return pt.Principal{UserID: id, SessionIDHash: strings.Repeat("a", 64), SessionVersion: 1, ControlIntent: "READ_ONLY"}, nil
	}, ValidateSession: func(context.Context, pt.Principal) error { return nil }, AuthenticateTicket: func(context.Context, pt.ConnectRequest) (pt.Principal, error) {
		forbidden.Add(1)
		return pt.Principal{}, poker.ErrDenied
	}, Snapshot: func(context.Context, pt.Principal, pt.ConnectionRef) (pt.Snapshot, error) {
		forbidden.Add(1)
		return pt.Snapshot{}, poker.ErrDenied
	}, Connected: func(context.Context, pt.Principal, pt.ConnectionRef) error { forbidden.Add(1); return poker.ErrDenied }, AuthorizeControl: func(context.Context, pt.Principal, pt.ConnectionRef, uint64) (pt.ControlRef, error) {
		forbidden.Add(1)
		return pt.ControlRef{}, poker.ErrDenied
	}, Ports: ports})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	server := httptest.NewServer(newPortalHandler(config{WebDir: f.dir, poker: h}, nil))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	wire := httputil.NewSingleHostReverseProxy(target) // appHTTP now traverses an actual TCP listener.
	user := func(id string) http.Header { return http.Header{"X-Synthetic-User": {id}} }
	create := func(key, name, preset string, id string) poker.Receipt {
		return appReceipt(t, f, wire, "/api/v1/poker/tables", map[string]any{"request_id": key, "name": name, "blind_preset": preset, "max_seats": 3, "allow_spectators": true}, user(id))
	}
	table := create("lobby-http-create-001", "HTTP Public", "5-10", "910001").TableID
	other := create("lobby-http-create-002", "Literal %_", "500-1000", "910002").TableID
	reserve := func(key string, seat int, id string) poker.Receipt {
		return appReceipt(t, f, wire, "/api/v1/poker/tables/"+table+"/seat-reservations", map[string]any{"request_id": key, "seat_no": seat}, user(id))
	}
	r := reserve("lobby-http-reserve-001", 1, "910001")
	session := appReceipt(t, f, wire, "/api/v1/poker/tables/"+table+"/buy-ins", map[string]any{"request_id": "lobby-http-buy-in-001", "reservation_id": r.ReservationID, "amount_units": "200000000"}, user("910001")).SessionID
	appReceipt(t, f, wire, "/api/v1/poker/sessions/"+session+"/top-ups", map[string]any{"request_id": "lobby-http-top-up-001", "amount_units": "5000000"}, user("910001"))
	pending := reserve("lobby-http-reserve-002", 2, "910002")
	store, err := redislease.Open(f.ctx, lease)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	facts := func() [32]byte {
		var values []string
		for _, name := range []string{"poker.tables", "poker.seats", "poker.sessions", "poker.actions", "poker.hands", "poker.request_receipts", "poker.funding_operations", "poker.audit_events", "economy.wallet_balances", "economy.wallet_ledger"} {
			var digest string
			if e := f.owner.QueryRow(f.ctx, "SELECT md5(coalesce(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text)::text,'[]')) FROM "+name+" t").Scan(&digest); e != nil {
				t.Fatal(e)
			}
			values = append(values, digest)
		}
		var names []string
		for key := range f.keys {
			names = append(names, key)
		}
		sort.Strings(names)
		for _, key := range names {
			var value string
			var e error
			if strings.HasPrefix(key, redislease.ControlKeyPrefix) {
				value, e = store.ControlLoad(f.ctx, strings.TrimPrefix(key, redislease.ControlKeyPrefix))
			} else {
				value, e = f.redis.Get(f.ctx, key).Result()
			}
			if e != nil && e != redis.Nil {
				t.Fatal(e)
			}
			values = append(values, value)
		}
		body, _ := json.Marshal(values)
		return sha256.Sum256(body)
	}
	lobby := func(path, id string) poker.LobbySnapshot {
		t.Helper()
		status, data, code := appHTTP(t, wire, "GET", path, nil, user(id))
		if status != 200 {
			t.Fatalf("whole readonly Lobby HTTP status=%d code=%s", status, code)
		}
		var v poker.LobbySnapshot
		var fields map[string]json.RawMessage
		if json.Unmarshal(data, &v) != nil || json.Unmarshal(data, &fields) != nil || len(fields) != 9 {
			t.Fatal("whole Lobby shape missing")
		}
		for _, key := range []string{"server_now", "service", "ruleset", "viewer", "active_session", "create_options", "blind_presets", "tables", "page"} {
			if fields[key] == nil {
				t.Fatal("Lobby omitted", key)
			}
		}
		for _, key := range []string{"control_epoch", "connection_id", "can_control", "runtime_id", "event_sequence", "hole_cards", "seed", "deck", "ledger"} {
			if strings.Contains(string(data), `"`+key+`"`) {
				t.Fatal("Lobby disclosed", key)
			}
		}
		if v.Viewer.UserID != id {
			t.Fatal("wrong owner Lobby")
		}
		return v
	}
	before := facts()
	a := lobby("/api/v1/poker", "910001")
	b := lobby("/api/v1/poker/tables", "910002")
	if a.Service.State != "READY" || !a.Service.ProductionReady || a.Ruleset == nil || a.Viewer.AvailableChipsUnits != "1295000000" || a.Viewer.PokerInPlayUnits != "205000000" || a.ActiveSession == nil || a.ActiveSession.SessionID != session || !a.ActiveSession.CanReconnect || a.Viewer.CanCreate || a.Viewer.CanJoin {
		t.Fatal("real owner asset/session/readiness mismatch")
	}
	if b.ActiveSession != nil || b.Viewer.AvailableChipsUnits != "1500000000" || b.Viewer.PokerInPlayUnits != "0" || len(b.Tables) != 2 || b.Page.Limit != 50 || len(b.CreateOptions.AccessModes) != 1 || b.CreateOptions.AccessModes[0] != "PUBLIC" || b.CreateOptions.ChatConfigurable {
		t.Fatal("other owner or default PUBLIC creation capability mismatch")
	}
	page := lobby("/api/v1/poker/tables?sort=LOW_BLIND&limit=1", "910003")
	if len(page.Tables) != 1 || page.Tables[0].TableID != table || page.Page.NextCursor == nil {
		t.Fatal("server first page mismatch")
	}
	next := lobby("/api/v1/poker/tables?sort=LOW_BLIND&limit=1&cursor="+url.QueryEscape(*page.Page.NextCursor), "910003")
	if len(next.Tables) != 1 || next.Tables[0].TableID != other || next.Page.NextCursor != nil {
		t.Fatal("server bound next page mismatch")
	}
	literal := lobby("/api/v1/poker/tables?q=%25_", "910003")
	if len(literal.Tables) != 1 || literal.Tables[0].TableID != other {
		t.Fatal("literal filter not handed to real domain")
	}
	if facts() != before || forbidden.Load() != 0 || denied.calls.Load() != 0 {
		t.Fatal("Lobby read changed business facts/lease or entered runtime authority")
	}
	tx, err := f.owner.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	for _, sql := range []string{
		`SELECT scope FROM ops.maintenance_scope_guards WHERE scope IN('CHALDEA_USER_WRITES','POKER_NEW_TABLES_NEW_HANDS') ORDER BY scope FOR UPDATE`,
		`INSERT INTO ops.admin_operations(operation_id,actor_kind,newapi_user_id,action,request_hash,details,result) VALUES('10000000-0000-4000-8000-000000000001','OFFLINE',910001,'SYNTHETIC_LOBBY_HTTP',repeat('a',64),'{}','{}')`,
		`INSERT INTO ops.maintenance_windows(maintenance_id,state,reason,impact_snapshot,impact_hash,environment,activated_at,created_by,activated_by,operation_id) VALUES('10000000-0000-4000-8000-000000000002','ACTIVE','synthetic local HTTP boundary','{}',decode(repeat('a',64),'hex'),'STAGING',clock_timestamp(),910001,910001,'10000000-0000-4000-8000-000000000001')`,
		`INSERT INTO ops.maintenance_window_scopes(maintenance_id,scope) VALUES('10000000-0000-4000-8000-000000000002','POKER_NEW_TABLES_NEW_HANDS')`,
	} {
		if _, err = tx.Exec(f.ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	before = facts()
	maint := lobby("/api/v1/poker", "910001")
	if maint.Service.State != "MAINTENANCE" || len(maint.Service.MaintenanceScopes) != 1 || maint.Service.MaintenanceScopes[0] != "POKER_NEW_TABLES_NEW_HANDS" || maint.Viewer.CanCreate || maint.Viewer.CanJoin || maint.ActiveSession == nil || maint.ActiveSession.SessionID != session || !maint.ActiveSession.CanReconnect {
		t.Fatal("real maintenance erased accepted recovery or granted new work")
	}
	for _, tc := range []struct {
		path, id string
		body     map[string]any
	}{
		{"/api/v1/poker/tables", "910003", map[string]any{"request_id": "lobby-maint-create-001", "name": "Denied", "blind_preset": "5-10", "max_seats": 3, "allow_spectators": true}},
		{"/api/v1/poker/tables/" + table + "/seat-reservations", "910003", map[string]any{"request_id": "lobby-maint-reserve-001", "seat_no": 3}},
		{"/api/v1/poker/tables/" + table + "/buy-ins", "910002", map[string]any{"request_id": "lobby-maint-buy-in-001", "reservation_id": pending.ReservationID, "amount_units": "200000000"}},
	} {
		status, _, code := appHTTP(t, wire, "POST", tc.path, tc.body, user(tc.id))
		if status != 503 || code != "MAINTENANCE_ACTIVE" {
			t.Errorf("new work status=%d code=%s", status, code)
		}
	}
	status, data, code := appHTTP(t, wire, "POST", "/api/v1/poker/tables/"+table+"/receipt-query", map[string]string{"kind": "topup", "mutation_id": "lobby-http-top-up-001"}, user("910001"))
	var receipt poker.ReceiptLookup
	if status != 200 || json.Unmarshal(data, &receipt) != nil || receipt.State != "FOUND" || receipt.Receipt == nil || receipt.Receipt.AmountUnits != "5000000" || receipt.Receipt.Duplicate {
		t.Fatal("old committed receipt altered during maintenance", status, code)
	}
	if facts() != before {
		t.Fatal("maintenance read or denied write changed table/funding/ledger/hand/lease facts")
	}
	appReceipt(t, f, wire, "/api/v1/poker/sessions/"+session+"/safe-leave", map[string]any{"request_id": "lobby-http-safe-leave-001"}, user("910001"))
	after := lobby("/api/v1/poker/tables", "910001")
	var cashouts int
	if err = f.owner.QueryRow(f.ctx, "SELECT count(*) FROM poker.funding_operations WHERE kind='CASH_OUT'").Scan(&cashouts); err != nil {
		t.Fatal(err)
	}
	if after.ActiveSession != nil || after.Viewer.PokerInPlayUnits != "0" || after.Viewer.AvailableChipsUnits != "1500000000" || after.Service.State != "MAINTENANCE" || cashouts != 1 {
		t.Fatal("single accepted cashout not reflected in next whole Lobby")
	}
	before = facts()
	status, data, code = appHTTP(t, wire, "GET", "/api/v1/poker", nil, user("919999"))
	if status != 503 || len(data) != 0 || code != "POKER_SERVICE_UNAVAILABLE" || facts() != before {
		t.Fatal("missing wallet fabricated a success or wrote identity")
	}
	pool.Close()
	status, data, code = appHTTP(t, wire, "GET", "/api/v1/poker/tables", nil, user("910001"))
	if status != 503 || len(data) != 0 || code != "POKER_SERVICE_UNAVAILABLE" {
		t.Fatal("real reader dependency failure did not fail closed")
	}
	if forbidden.Load() != 0 || denied.calls.Load() != 0 {
		t.Fatal("readonly Lobby allocated runtime/lease authority")
	}
	if !t.Failed() {
		t.Log("AUTHENTICATION=SYNTHETIC_CALLBACK; actual TCP portal + PG readonly SQLSTATE25006 + TLS Redis; two owner whole DTO, server cursor/literal filter, maintenance denies three new work paths, old receipt and exactly one cashout preserved; readonly PG/Redis facts unchanged")
	}
}
