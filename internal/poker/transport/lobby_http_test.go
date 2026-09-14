package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestLobbyHTTPWholeSnapshotAndStrictQuery(t *testing.T) {
	const whole = `{"server_now":"2026-09-06T00:00:00Z","service":{"state":"MAINTENANCE"},"ruleset":null,"viewer":{"user_id":"1"},"active_session":null,"create_options":{},"blind_presets":[],"tables":[],"page":{"limit":50,"next_cursor":null}}`
	var reads, forbidden atomic.Int32
	var revoked atomic.Bool
	var seen LobbyQuery
	b := newSynthetic()
	h, server := newServer(t, b, func(o *Options) {
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
		read := func(_ context.Context, p Principal) (json.RawMessage, error) {
			if p.UserID != 1 || p.ControlIntent != "READ_ONLY" {
				t.Error("Lobby did not receive verified readonly principal")
			}
			reads.Add(1)
			return json.RawMessage(whole), nil
		}
		o.Ports.ReadLobby = read
		o.Ports.ReadTables = func(ctx context.Context, p Principal, q LobbyQuery) (json.RawMessage, error) {
			seen = q
			return read(ctx, p)
		}
		o.Snapshot = func(context.Context, Principal, ConnectionRef) (Snapshot, error) {
			forbidden.Add(1)
			return Snapshot{}, errInvalid
		}
		o.Connected = func(context.Context, Principal, ConnectionRef) error { forbidden.Add(1); return nil }
		o.AuthorizeControl = func(context.Context, Principal, ConnectionRef, uint64) (ControlRef, error) {
			forbidden.Add(1)
			return ControlRef{}, errInvalid
		}
		o.Ports.CreateTable = func(context.Context, Principal, CreateTableRequest) (json.RawMessage, error) {
			forbidden.Add(1)
			return nil, errInvalid
		}
	})
	request := func(t *testing.T, method, path, body, auth string, want int) {
		t.Helper()
		r, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", auth)
		r.Header.Set("Origin", testOrigin)
		r.Header.Set("Content-Type", "application/json")
		response, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != want {
			t.Errorf("%s %s status=%d want=%d", method, path, response.StatusCode, want)
		}
		if response.Header.Get("Cache-Control") != "no-store" {
			t.Error("Lobby response cacheable")
		}
		if want == 200 && response.StatusCode == 200 {
			var envelope struct {
				Success bool
				Data    json.RawMessage
			}
			if json.Unmarshal(raw, &envelope) != nil || !envelope.Success || string(envelope.Data) != whole {
				t.Error("whole Lobby projection altered or reduced to tables")
			}
		}
	}
	const auth = "Bearer synthetic-http"
	const base = "/api/v1/poker"
	request(t, "GET", base, "", auth, 200)
	request(t, "GET", base+"/tables", "", auth, 200)
	request(t, "GET", base+"/tables?q=%25_&access_mode=PUBLIC&open_seats_only=true&max_seats=3&blind_preset=5-10&lifecycle_state=WAITING&spectators_only=false&sort=NEAR_FULL&limit=1&cursor=opaque-public-cursor", "", auth, 200)
	if seen != (LobbyQuery{Query: "%_", AccessMode: "PUBLIC", OpenSeatsOnly: true, MaxSeats: 3, BlindPreset: "5-10", LifecycleState: "WAITING", Sort: "NEAR_FULL", Limit: 1, Cursor: "opaque-public-cursor"}) {
		t.Fatal("typed query fields were not passed intact")
	}
	request(t, "GET", base+"/tables?", "", auth, 200)
	before := reads.Load()
	for i, path := range []string{
		base + "?", base + "?limit=1", base + "/tables/" + testTable + "?q=x", base + "/sessions/active?", base + "/%74ables?q=x",
		base + "/tables?user_id=2", base + "/tables?token=x", base + "/tables?seed=x", base + "/tables?mutation_id=x", base + "/tables?intent=x",
		base + "/tables?q=a&q=b", base + "/tables?%71=a&q=a", base + "/tables?q=", base + "/tables?q=x;limit=1", base + "/tables?%zz=1",
		base + "/tables?open_seats_only=1", base + "/tables?spectators_only=TRUE", base + "/tables?limit=0", base + "/tables?limit=101", base + "/tables?limit=01", base + "/tables?limit=+1", base + "/tables?limit=1.0", base + "/tables?max_seats=1", base + "/tables?max_seats=10", base + "/tables?limit=999999999999999999999999", base + "/tables?q=" + strings.Repeat("a", 4097),
	} {
		t.Run(fmt.Sprint("invalid-query-", i), func(t *testing.T) { request(t, "GET", path, "", auth, 400) })
	}
	request(t, "GET", base+"/tables", "", "", 401)
	revoked.Store(true)
	request(t, "GET", base+"/tables?limit=1", "", auth, 401)
	revoked.Store(false)
	const create = `{"request_id":"lobby-create-identity-001","name":"Public","blind_preset":"5-10","max_seats":3,"allow_spectators":true}`
	request(t, "POST", base+"/tables?", create, auth, 400)
	request(t, "POST", base+"/tables?limit=1", create, auth, 400)
	request(t, "POST", base+"/tables", strings.TrimSuffix(create, "}")+`,"password":"x"}`, auth, 400)
	request(t, "POST", base+"/tables", strings.TrimSuffix(create, "}")+`,"chat_enabled":true}`, auth, 400)
	request(t, "PUT", base+"/tables", "", auth, 405)
	if reads.Load() != before || forbidden.Load() != 0 {
		t.Error("invalid/read-only Lobby request reached a forbidden port")
	}
	h.opts.Ports.ReadLobby = nil
	h.opts.Ports.ReadTables = nil
	request(t, "GET", base, "", auth, 503)
	request(t, "GET", base+"/tables?limit=1", "", auth, 503)
	if !t.Failed() {
		t.Logf("synthetic transport only: whole snapshots=%d forbidden=%d; exact GET/tables query and original create shape verified", reads.Load(), forbidden.Load())
	}
}
