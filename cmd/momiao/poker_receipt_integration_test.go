package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cy4268/momiao/internal/poker"
	pt "github.com/cy4268/momiao/internal/poker/transport"
)

func TestPokerReceiptQueryLostAckOwnerBound(t *testing.T) {
	f := newPokerAppFixture(t, 910001, 910002)
	lease, _, err := readPokerRedisConfig(f.redisFile)
	if err != nil {
		t.Fatal(err)
	}
	s, err := poker.NewWithRedis(f.ctx, poker.Options{Pool: f.domain, Keyring: poker.Keyring{Current: "query", Keys: map[string][]byte{"query": make([]byte, 32)}}, ValidateSession: func(context.Context, poker.AuthSession) error { return nil }}, lease)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := &pokerAdapter{service: s}
	ports := a.ports()
	var topupCalls, controlCalls atomic.Int32
	topup := ports.TopUp
	ports.TopUp = func(ctx context.Context, p pt.Principal, id string, r pt.TopUpRequest) (json.RawMessage, error) {
		topupCalls.Add(1)
		return topup(ctx, p, id, r)
	}
	h, err := pt.New(pt.Options{Origin: "http://127.0.0.1:32000", AuthHTTP: func(r *http.Request) (pt.Principal, error) {
		id, e := strconv.ParseInt(r.Header.Get("X-Synthetic-User"), 10, 64)
		if e != nil || id != 910001 && id != 910002 {
			return pt.Principal{}, &pt.Fault{Status: 401, Code: "AUTH_REQUIRED"}
		}
		return pt.Principal{UserID: id, SessionIDHash: strings.Repeat("a", 64), SessionVersion: 1, ControlIntent: "READ_ONLY"}, nil
	}, AuthenticateTicket: func(context.Context, pt.ConnectRequest) (pt.Principal, error) {
		controlCalls.Add(1)
		return pt.Principal{}, &pt.Fault{Status: 401, Code: "UNUSED"}
	}, ValidateSession: func(context.Context, pt.Principal) error { return nil }, Snapshot: func(context.Context, pt.Principal, pt.ConnectionRef) (pt.Snapshot, error) {
		controlCalls.Add(1)
		return pt.Snapshot{}, &pt.Fault{Status: 503, Code: "UNUSED"}
	}, Connected: func(context.Context, pt.Principal, pt.ConnectionRef) error { controlCalls.Add(1); return nil }, AuthorizeControl: func(context.Context, pt.Principal, pt.ConnectionRef, uint64) (pt.ControlRef, error) {
		controlCalls.Add(1)
		return pt.ControlRef{}, &pt.Fault{Status: 403, Code: "UNUSED"}
	}, Ports: ports})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	portal := newPortalHandler(config{WebDir: f.dir, poker: h}, nil)
	server := httptest.NewServer(portal)
	defer server.Close()
	user1, user2 := http.Header{"X-Synthetic-User": {"910001"}}, http.Header{"X-Synthetic-User": {"910002"}}
	create := func(key string, headers http.Header) poker.Receipt {
		return appReceipt(t, f, portal, "/api/v1/poker/tables", map[string]any{"request_id": key, "name": "Receipt query fixture", "blind_preset": "5-10", "max_seats": 2, "allow_spectators": true}, headers)
	}
	table := create("receipt-owner-table-001", user1).TableID
	other := create("receipt-other-table-001", user2).TableID
	r := appReceipt(t, f, portal, "/api/v1/poker/tables/"+table+"/seat-reservations", map[string]any{"request_id": "receipt-seat-reserve-001", "seat_no": 1}, user1)
	bought := appReceipt(t, f, portal, "/api/v1/poker/tables/"+table+"/buy-ins", map[string]any{"request_id": "receipt-buy-in-001", "reservation_id": r.ReservationID, "amount_units": "200000000"}, user1)
	const mutationID = "receipt-top-up-lost-ack-001"
	type lookup struct {
		Table   string         `json:"table_id"`
		Kind    string         `json:"kind"`
		ID      string         `json:"mutation_id"`
		State   string         `json:"state"`
		Receipt *poker.Receipt `json:"receipt"`
	}
	query := func(table, kind, id string, headers http.Header) lookup {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"kind": kind, "mutation_id": id})
		request, e := http.NewRequest(http.MethodPost, server.URL+"/api/v1/poker/tables/"+table+"/receipt-query", strings.NewReader(string(body)))
		if e != nil {
			t.Fatal(e)
		}
		request.Header = headers.Clone()
		request.Header.Set("Origin", "http://127.0.0.1:32000")
		request.Header.Set("Content-Type", "application/json")
		response, e := server.Client().Do(request)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		data, e := io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		if response.StatusCode != 200 {
			t.Errorf("actual receipt-query HTTP status=%d want=200", response.StatusCode)
			return lookup{}
		}
		var envelope struct {
			Success bool   `json:"success"`
			Data    lookup `json:"data"`
		}
		if json.Unmarshal(data, &envelope) != nil || !envelope.Success || response.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("lookup envelope malformed/cacheable")
		}
		if envelope.Data.Table != table || envelope.Data.Kind != kind || envelope.Data.ID != id {
			t.Fatal("lookup locator changed")
		}
		var public struct {
			Data map[string]json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(data, &public)
		if envelope.Data.State == "NOT_FOUND" && public.Data["receipt"] != nil {
			t.Fatal("NOT_FOUND carried a fabricated receipt")
		}
		for _, field := range []string{"ack", "no_effect", "request_hash", "key_hash", "seed", "control", "connection_id", "auth"} {
			if public.Data[field] != nil {
				t.Fatal("lookup exposed a noncontract field", field)
			}
		}
		return envelope.Data
	}
	before := query(table, "topup", mutationID, user1)
	if before.State != "NOT_FOUND" || before.Receipt != nil {
		t.Error("query-before-commit was not unresolved NOT_FOUND")
	}
	// Real HTTP submission. The client deliberately discards the response body:
	// no receipt or response-derived mutation outcome is used for reconciliation.
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/poker/sessions/"+bought.SessionID+"/top-ups", strings.NewReader(`{"request_id":"`+mutationID+`","amount_units":"5000000"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header = user1.Clone()
	request.Header.Set("Origin", "http://127.0.0.1:32000")
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("real top-up submission failed before lost-ACK test")
	}
	var stored []byte
	if err = f.owner.QueryRow(f.ctx, "SELECT response FROM poker.request_receipts WHERE newapi_user_id=910001 AND scope='poker.topup.v1' AND table_id=$1", table).Scan(&stored); err != nil {
		t.Fatal("actual top-up did not commit a receipt")
	}
	var expected poker.Receipt
	if json.Unmarshal(stored, &expected) != nil || expected.Status != "CONFIRMED" || expected.SessionID != bought.SessionID || expected.Duplicate {
		t.Fatal("stored original receipt invalid")
	}
	counts := func() string {
		t.Helper()
		var evidence string
		err := f.owner.QueryRow(f.ctx, `SELECT json_build_array(
 (SELECT count(*) FROM poker.funding_operations WHERE kind='TOP_UP'),
 (SELECT count(*) FROM poker.request_receipts WHERE scope='poker.topup.v1'),
 (SELECT count(*) FROM economy.wallet_ledger WHERE transaction_id=(SELECT confirmed_transaction_id FROM poker.funding_operations WHERE kind='TOP_UP')),
 (SELECT count(*) FROM poker.request_receipts),(SELECT count(*) FROM economy.wallet_ledger),
 (SELECT balance_units FROM economy.wallet_balances WHERE newapi_user_id=910001 AND asset_type='AVAILABLE_CHIPS'),
 (SELECT current_stack_units FROM poker.sessions WHERE session_id=$1),
 (SELECT runtime_epoch FROM poker.tables WHERE table_id=$2),
 (SELECT control_epoch FROM poker.sessions WHERE session_id=$1))::text`, bought.SessionID, table).Scan(&evidence)
		if err != nil {
			t.Fatal(err)
		}
		return evidence
	}
	baseline := counts()
	var metrics []int64
	if json.Unmarshal([]byte(baseline), &metrics) != nil || len(metrics) != 9 || metrics[0] != 1 || metrics[1] != 1 || metrics[2] != 1 {
		t.Fatal("lost ACK fixture did not contain exactly one confirmed topup/receipt/ledger")
	}
	for range 2 {
		found := query(table, "topup", mutationID, user1)
		if found.State != "FOUND" || found.Receipt == nil {
			t.Error("committed original owner receipt not found")
			continue
		}
		if *found.Receipt != expected {
			t.Fatal("lookup changed the original receipt, duplicate flag or funding identity")
		}
	}
	for _, q := range []struct {
		table, kind, id string
		headers         http.Header
	}{{table, "topup", mutationID, user2}, {other, "topup", mutationID, user1}, {table, "leave", mutationID, user1}, {table, "topup", "never-committed-identity-001", user1}} {
		result := query(q.table, q.kind, q.id, q.headers)
		if result.State != "NOT_FOUND" || result.Receipt != nil {
			t.Error("nonowner/other table/scope/unknown ID did not share NOT_FOUND semantics")
		}
	}
	if final := counts(); final != baseline || topupCalls.Load() != 1 || controlCalls.Load() != 0 {
		t.Fatal("readonly reconciliation changed money, receipts, runtime/control or resubmitted a mutation")
	}
	if !t.Failed() {
		t.Log("RECEIPT_LOST_ACK AUTHENTICATION=SYNTHETIC; actual portal HTTP + fresh PG/TLS Redis; committed topup=1 receipt=1 ledger=1; precommit NOT_FOUND unresolved; owner FOUND original; cross-owner/table/scope/unknown NOT_FOUND; reads changed no money/receipts/runtime/control and mutation submissions=1")
	}
}
