package transport

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestReceiptQueryHTTPBoundary(t *testing.T) {
	var reads, forbidden atomic.Int32
	var revoked atomic.Bool
	b := newSynthetic()
	h, s := newServer(t, b, func(o *Options) {
		original := o.AuthHTTP
		o.AuthHTTP = func(r *http.Request) (Principal, error) {
			p, e := original(r)
			p.ControlIntent = "READ_ONLY"
			return p, e
		}
		o.ValidateSession = func(context.Context, Principal) error {
			if revoked.Load() {
				return &Fault{401, "SESSION_REVOKED"}
			}
			return nil
		}
		o.Connected = func(context.Context, Principal, ConnectionRef) error { forbidden.Add(1); return nil }
		o.Disconnected = func(context.Context, Principal, ConnectionRef) { forbidden.Add(1) }
		o.AuthorizeControl = func(context.Context, Principal, ConnectionRef, uint64) (ControlRef, error) {
			forbidden.Add(1)
			return ControlRef{}, errInvalid
		}
		o.Snapshot = func(context.Context, Principal, ConnectionRef) (Snapshot, error) {
			forbidden.Add(1)
			return Snapshot{}, errInvalid
		}
		o.Ports.TakeOver = func(context.Context, Principal, string, TakeOverRequest) (json.RawMessage, error) {
			forbidden.Add(1)
			return nil, errInvalid
		}
		o.Ports.TopUp = func(context.Context, Principal, string, TopUpRequest) (json.RawMessage, error) {
			forbidden.Add(1)
			return nil, errInvalid
		}
		o.Ports.SafeLeave = func(context.Context, Principal, string, SessionRequest) (json.RawMessage, error) {
			forbidden.Add(1)
			return nil, errInvalid
		}
		o.Ports.HostCommand = func(context.Context, Principal, string, HostRequest) (json.RawMessage, error) {
			forbidden.Add(1)
			return nil, errInvalid
		}
		o.Ports.Act = func(context.Context, Principal, HandAction) (json.RawMessage, error) {
			forbidden.Add(1)
			return nil, errInvalid
		}
		o.Ports.SitOut = func(context.Context, Principal, SessionAction) (json.RawMessage, error) {
			forbidden.Add(1)
			return nil, errInvalid
		}
		o.Ports.Resume = func(context.Context, Principal, SessionAction) (json.RawMessage, error) {
			forbidden.Add(1)
			return nil, errInvalid
		}
		o.Ports.SetClientSeed = func(context.Context, Principal, ClientSeed) (json.RawMessage, error) {
			forbidden.Add(1)
			return nil, errInvalid
		}
		o.Ports.ReadReceipt = func(_ context.Context, p Principal, table string, q ReceiptQueryRequest) (json.RawMessage, error) {
			reads.Add(1)
			if p.UserID != 1 || p.ControlIntent != "READ_ONLY" || table != testTable {
				return nil, errInvalid
			}
			out := map[string]any{"table_id": table, "kind": q.Kind, "mutation_id": q.MutationID, "state": "NOT_FOUND"}
			if q.MutationID == "original-identity-001" {
				out["state"] = "FOUND"
				out["receipt"] = map[string]any{"table_id": table, "status": "CONFIRMED", "table_version": "12", "duplicate": false}
			}
			return json.Marshal(out)
		}
	})
	request := func(method, path, body, auth, origin string) (int, map[string]json.RawMessage) {
		t.Helper()
		r, e := http.NewRequest(method, s.URL+path, strings.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		r.Header.Set("Content-Type", "application/json")
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		response, e := s.Client().Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		raw, e := io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		var out map[string]json.RawMessage
		if json.Unmarshal(raw, &out) != nil {
			t.Fatal("receipt-query response malformed")
		}
		if response.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("receipt locator was cacheable")
		}
		return response.StatusCode, out
	}
	path := "/api/v1/poker/tables/" + testTable + "/receipt-query"
	for _, kind := range []string{"action", "sitout", "resume", "nextseed", "topup", "leave", "takeover", "host"} {
		t.Run(kind, func(t *testing.T) {
			status, envelope := request("POST", path, `{"kind":"`+kind+`","mutation_id":"original-identity-001"}`, "Bearer synthetic-http", testOrigin)
			if status != 200 {
				t.Errorf("new readonly receipt route status=%d want=200", status)
				return
			}
			var body struct {
				State, Kind string
				Mutation    string `json:"mutation_id"`
				Receipt     map[string]json.RawMessage
			}
			if json.Unmarshal(envelope["data"], &body) != nil || body.State != "FOUND" || body.Kind != kind || body.Mutation != "original-identity-001" || string(body.Receipt["duplicate"]) != "false" {
				t.Fatal("original readonly receipt altered")
			}
		})
	}
	status, envelope := request("POST", path, `{"kind":"action","mutation_id":"uncommitted-identity-001"}`, "Bearer synthetic-http", testOrigin)
	var absent map[string]json.RawMessage
	if status != 200 || json.Unmarshal(envelope["data"], &absent) != nil || string(absent["state"]) != `"NOT_FOUND"` || absent["receipt"] != nil || absent["ack"] != nil || absent["no_effect"] != nil {
		t.Error("NOT_FOUND was not an unacknowledged absence")
	}
	valid := `{"kind":"action","mutation_id":"original-identity-001"}`
	for _, tc := range []struct {
		name, method, path, body, auth, origin string
		status                                 int
	}{
		{"missing-auth", "POST", path, valid, "", testOrigin, 401},
		{"foreign-origin", "POST", path, valid, "Bearer synthetic-http", "https://foreign.invalid", 403},
		{"missing-origin", "POST", path, valid, "Bearer synthetic-http", "", 403},
		{"query", "POST", path + "?mutation_id=forbidden", valid, "Bearer synthetic-http", testOrigin, 400},
		{"get", "GET", path, valid, "Bearer synthetic-http", testOrigin, 405},
		{"bad-table", "POST", "/api/v1/poker/tables/not-a-uuid/receipt-query", valid, "Bearer synthetic-http", testOrigin, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := reads.Load()
			status, _ := request(tc.method, tc.path, tc.body, tc.auth, tc.origin)
			if status != tc.status || reads.Load() != before {
				t.Errorf("boundary status=%d want=%d; query called=%t", status, tc.status, reads.Load() != before)
			}
		})
	}
	for _, body := range []string{
		`{}`, `null`, `{"kind":"action"}`, `{"mutation_id":"original-identity-001"}`,
		`{"kind":"action","mutation_id":""}`, `{"kind":"action","mutation_id":"short"}`,
		`{"kind":"action","mutation_id":null}`, `{"kind":null,"mutation_id":"original-identity-001"}`,
		`{"Kind":"action","mutation_id":"original-identity-001"}`,
		`{"kind":"action","kind":"leave","mutation_id":"original-identity-001"}`,
		`{"kind":"action","mutation_id":"original-identity-001","mutation_id":"different-identity"}`,
		`{"kind":"action","mutation_id":"original-identity-001","user_id":1}`,
		`{"kind":"action","mutation_id":"original-identity-001","payload":{}}`,
		`{"kind":"action","mutation_id":"` + strings.Repeat("x", 129) + `"}`,
		`{"kind":"action","mutation_id":"original-identity-\u0001"}`,
		`{"kind":"poker.action.v1","mutation_id":"original-identity-001"}`,
		`{"kind":"create","mutation_id":"original-identity-001"}`,
		`{"kind":"ACTION","mutation_id":"original-identity-001"}`,
		valid + `{}`,
	} {
		before := reads.Load()
		status, _ := request("POST", path, body, "Bearer synthetic-http", testOrigin)
		if status != 400 || reads.Load() != before {
			t.Errorf("malformed/unknown query status=%d, read_called=%t", status, reads.Load() != before)
		}
	}
	revoked.Store(true)
	before := reads.Load()
	status, _ = request("POST", path, valid, "Bearer synthetic-http", testOrigin)
	if status != 401 || reads.Load() != before {
		t.Error("revoked session reached read authority")
	}
	revoked.Store(false)
	// Requests are sequential and complete before this test-only port removal.
	h.opts.Ports.ReadReceipt = nil
	status, envelope = request("POST", path, valid, "Bearer synthetic-http", testOrigin)
	if status != 503 || string(envelope["code"]) != `"POKER_CAPABILITY_UNAVAILABLE"` {
		t.Error("missing read capability fabricated an outcome")
	}
	if forbidden.Load() != 0 {
		t.Fatal("receipt lookup touched control/snapshot/mutation ports")
	}
	if !t.Failed() {
		t.Logf("RECEIPT_HTTP readonly kinds=8; malformed/auth/origin/query/nil-port gates; read_calls=%d forbidden_effect_calls=%d; NOT_FOUND carries no receipt or no-effect proof", reads.Load(), forbidden.Load())
	}
}
