package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker"
	"github.com/cy4268/momiao/internal/poker/redislease"
	pt "github.com/cy4268/momiao/internal/poker/transport"
)

func TestPokerEntryReadsOwnerTCP(t *testing.T) {
	f := newPokerAppFixture(t, 910001, 910002)
	lease, _, err := readPokerRedisConfig(f.redisFile)
	if err != nil {
		t.Fatal(err)
	}
	s, err := poker.NewWithRedis(f.ctx, poker.Options{Pool: f.domain, Keyring: poker.Keyring{Current: "entry", Keys: map[string][]byte{"entry": make([]byte, 32)}}, ValidateSession: func(context.Context, poker.AuthSession) error { return nil }}, lease)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := &pokerAdapter{service: s}
	h, err := pt.New(pt.Options{Origin: "http://127.0.0.1:32000", AuthHTTP: func(r *http.Request) (pt.Principal, error) {
		id := int64(910001)
		if r.Header.Get("X-Synthetic-Other") == "yes" {
			id = 910002
		}
		return pt.Principal{UserID: id, SessionIDHash: strings.Repeat("a", 64), SessionVersion: 1, ControlIntent: "READ_ONLY"}, nil
	}, ValidateSession: func(context.Context, pt.Principal) error { return nil }, AuthenticateTicket: func(context.Context, pt.ConnectRequest) (pt.Principal, error) { return pt.Principal{}, poker.ErrDenied }, Snapshot: a.snapshot, Connected: a.connected, AuthorizeControl: a.authorize, Ports: a.ports()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	server := httptest.NewServer(newPortalHandler(config{WebDir: f.dir, poker: h}, nil))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	wire := httputil.NewSingleHostReverseProxy(target)
	wire.ModifyResponse = func(r *http.Response) error {
		if r.Header.Get("Cache-Control") != "no-store" {
			t.Error("actual TCP response was cacheable")
		}
		return nil
	}
	create := appReceipt(t, f, wire, "/api/v1/poker/tables", map[string]any{"request_id": "entry-http-create-01", "name": "Entry recovery", "blind_preset": "5-10", "max_seats": 2, "allow_spectators": true}, nil)
	base := "/api/v1/poker/tables/" + create.TableID
	reserved := appReceipt(t, f, wire, base+"/seat-reservations", map[string]any{"request_id": "entry-http-reserve-01", "seat_no": 1}, nil)
	query := func(path string, body any, other bool, out any, want int) {
		t.Helper()
		headers := http.Header{}
		if other {
			headers.Set("X-Synthetic-Other", "yes")
		}
		status, data, code := appHTTP(t, wire, "POST", path, body, headers)
		if status != want || want == 200 && json.Unmarshal(data, out) != nil || want != 200 && len(data) != 0 {
			t.Errorf("actual TCP read %s status=%d want=%d code=%s", path, status, want, code)
		}
		for _, key := range []string{"lease_token", "key_hash", "request_hash", "connection_id", "control_epoch", "no_effect"} {
			if strings.Contains(string(data), `"`+key+`"`) {
				t.Error("read leaked", key)
			}
		}
	}
	facts := func() string {
		t.Helper()
		var all string
		for _, name := range []string{"poker.tables", "poker.seats", "poker.seat_reservations", "poker.sessions", "poker.actions", "poker.hands", "poker.request_receipts", "poker.funding_operations", "poker.audit_events", "economy.wallet_balances", "economy.wallet_ledger"} {
			var digest string
			if e := f.owner.QueryRow(f.ctx, "SELECT md5(coalesce(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text)::text,'[]')) FROM "+name+" t").Scan(&digest); e != nil {
				t.Fatal(e)
			}
			all += digest
		}
		return all
	}
	var deadline time.Time
	if err = f.owner.QueryRow(f.ctx, "SELECT expires_at FROM poker.seat_reservations WHERE reservation_id=$1", reserved.ReservationID).Scan(&deadline); err != nil {
		t.Fatal(err)
	}
	key := redislease.KeyPrefix + create.TableID + ":1"
	token, err := f.redis.Get(f.ctx, key).Result()
	if err != nil {
		t.Fatal(err)
	}
	ttl, err := f.redis.PTTL(f.ctx, key).Result()
	if err != nil || ttl <= 0 {
		t.Fatal("real live reservation TTL missing", err)
	}
	before := facts()
	var current poker.ReservationLookup
	query(base+"/reservation-query", map[string]string{"reservation_id": reserved.ReservationID}, false, &current, 200)
	var absent poker.EntryReceiptLookup
	query("/api/v1/poker/entry-receipt-query", map[string]string{"kind": "buyin", "mutation_id": "entry-http-buyin-01", "table_id": create.TableID}, false, &absent, 200)
	if t.Failed() {
		return // RED records both actual 404 routes, without trusting empty projections.
	}
	afterTTL, err := f.redis.PTTL(f.ctx, key).Result()
	afterToken, tokenErr := f.redis.Get(f.ctx, key).Result()
	if err != nil || tokenErr != nil || token != afterToken || afterTTL <= 0 || afterTTL > ttl || facts() != before || current.UserID != "910001" || current.TableID != create.TableID || current.ReservationID != reserved.ReservationID || current.State != "FOUND" || current.Reservation == nil || !current.Reservation.Valid || current.Reservation.SeatNo != 1 || !current.Reservation.ExpiresAt.Equal(deadline) || current.Reservation.CheckedAt.IsZero() || !current.Reservation.CheckedAt.Before(deadline) || absent.State != "NOT_FOUND" || absent.Receipt != nil {
		t.Fatal("reservation current fact, unchanged TTL/token, or unresolved precommit visibility mismatch")
	}
	bought := appReceipt(t, f, wire, base+"/buy-ins", map[string]any{"request_id": "entry-http-buyin-01", "reservation_id": reserved.ReservationID, "amount_units": "200000000"}, nil)
	before = facts()
	for _, tc := range []struct {
		kind, key string
		want      poker.Receipt
	}{{"create", "entry-http-create-01", create}, {"reserve", "entry-http-reserve-01", reserved}, {"buyin", "entry-http-buyin-01", bought}} {
		body := map[string]string{"kind": tc.kind, "mutation_id": tc.key}
		if tc.kind != "create" {
			body["table_id"] = create.TableID
		}
		for _, other := range []bool{false, true} {
			var got poker.EntryReceiptLookup
			query("/api/v1/poker/entry-receipt-query", body, other, &got, 200)
			if !other && (got.UserID != "910001" || got.Kind != tc.kind || got.MutationID != tc.key || got.State != "FOUND" || got.Receipt == nil || *got.Receipt != tc.want) || other && (got.UserID != "910002" || got.State != "NOT_FOUND" || got.Receipt != nil) {
				t.Error("original entry receipt or owner boundary mismatch", tc.kind, other)
			}
		}
		if tc.kind != "create" {
			body["table_id"] = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
			var wrong poker.EntryReceiptLookup
			query("/api/v1/poker/entry-receipt-query", body, false, &wrong, 200)
			if wrong.State != "NOT_FOUND" || wrong.Receipt != nil {
				t.Error("wrong original table disclosed receipt", tc.kind)
			}
		}
	}
	for _, other := range []bool{false, true} {
		current = poker.ReservationLookup{}
		query(base+"/reservation-query", map[string]string{"reservation_id": reserved.ReservationID}, other, &current, 200)
		if !other && (current.Reservation == nil || current.Reservation.Valid || current.Reservation.DurableState != "CONSUMED" || !current.Reservation.ExpiresAt.Equal(deadline)) || other && (current.State != "NOT_FOUND" || current.Reservation != nil) {
			t.Error("consumed reservation or other owner mismatch")
		}
	}
	if facts() != before || len(a.sockets) != 0 {
		t.Fatal("reads changed durable facts or rebound a socket")
	}
	f.domain.Close()
	query("/api/v1/poker/entry-receipt-query", map[string]string{"kind": "create", "mutation_id": "entry-http-create-01"}, false, nil, 503)
	query(base+"/reservation-query", map[string]string{"reservation_id": reserved.ReservationID}, false, nil, 503)
	t.Log("AUTHENTICATION=SYNTHETIC; actual TCP + fresh 20 PG + TLS Redis; 3 original receipts/owner isolation; PG deadline + lease unchanged, consumed invalid, no PG write/rebind, dependency fails closed")
}
