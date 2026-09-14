package poker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker/engine"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLobbyReadOnlyOwnerAndPagination(t *testing.T) {
	f := newControlFixture(t)
	ctx := context.Background()
	table := f.table(t)
	session := f.buy(t, table, 910001, 1)
	otherCommand := CreateTableCommand{UserID: 910002, Key: "lobby-other-table-01", Name: "Literal %_ table", BlindPreset: "500-1000", MaxSeats: 3}
	other, err := f.service.CreateTable(ctx, otherCommand)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.owner.Exec(ctx, `UPDATE poker.tables SET chat_enabled=true WHERE table_id=$1`, other.TableID); err != nil {
		t.Fatal(err)
	}
	reader := receiptReader(t, f) // Actual default_transaction_read_only=on pool.
	before := receiptFacts(t, f)
	v, err := reader.Lobby(ctx, 910001, LobbyFilter{})
	if err != nil {
		t.Fatal("real owner Lobby read must not require Actor/Controller", err)
	}
	if v.Viewer.UserID != "910001" || v.Viewer.AvailableChipsUnits != "1300000000" || v.Viewer.PokerInPlayUnits != "200000000" || v.Viewer.CanCreate || v.Viewer.CanJoin || v.ActiveSession == nil || v.ActiveSession.SessionID != session || !v.ActiveSession.CanReconnect || v.Service.State != "READY" || !v.Service.ProductionReady || len(v.BlindPresets) != 6 {
		t.Fatalf("owner assets/session/ruleset contract: %+v", v)
	}
	for _, row := range v.Tables {
		if row.CanJoin || row.CanSpectate {
			t.Fatal("active session admitted a different viewer mode/table")
		}
	}
	otherView, err := reader.Lobby(ctx, 910003, LobbyFilter{Sort: "HIGH_BLIND", Limit: 1})
	if err != nil || otherView.Viewer.UserID != "910003" || otherView.Viewer.AvailableChipsUnits != "1500000000" || otherView.Viewer.PokerInPlayUnits != "0" || otherView.ActiveSession != nil || !otherView.Viewer.CanCreate || len(otherView.Tables) != 1 || otherView.Tables[0].TableID != other.TableID || !otherView.Tables[0].ChatEnabled || otherView.Tables[0].CanSpectate || !otherView.Tables[0].CanJoin || otherView.Page.NextCursor == nil {
		t.Fatalf("other owner and actual metadata page: %+v %v", otherView, err)
	}
	second, err := reader.Lobby(ctx, 910003, LobbyFilter{Sort: "HIGH_BLIND", Limit: 1, Cursor: *otherView.Page.NextCursor})
	if err != nil || len(second.Tables) != 1 || second.Tables[0].TableID != table || second.Page.NextCursor != nil {
		t.Fatalf("bound keyset page: %+v %v", second.Page, err)
	}
	literal, err := reader.Lobby(ctx, 910003, LobbyFilter{Query: "%_"})
	if err != nil || len(literal.Tables) != 1 || literal.Tables[0].TableID != other.TableID {
		t.Fatalf("literal wildcard search: %+v %v", literal.Tables, err)
	}
	near, err := reader.Lobby(ctx, 910003, LobbyFilter{Sort: "NEAR_FULL"})
	if err != nil || near.Tables[0].TableID != table || len(near.Tables[0].OpenSeatNumbers) != 1 || near.Tables[0].OpenSeatNumbers[0] != 2 {
		t.Fatalf("near-full means occupancy ratio, not WAITING sort: %+v %v", near.Tables, err)
	}
	for _, filter := range []LobbyFilter{{Limit: 101}, {MaxSeats: 10}, {Query: strings.Repeat("a", 41)}, {Sort: "NEAR_START"}, {AccessMode: "UNLISTED"}, {Cursor: strings.Repeat("a", 513)}, {Cursor: base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"v":1}`))}, {Sort: "LOW_BLIND", Limit: 1, Cursor: *otherView.Page.NextCursor}} {
		if _, err = reader.Lobby(ctx, 910003, filter); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid filter/cursor accepted: %+v %v", filter, err)
		}
	}
	if _, err = reader.Lobby(ctx, 0, LobbyFilter{}); !errors.Is(err, ErrInvalid) {
		t.Fatal("unverified identity accepted", err)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"control_epoch", "connection_id", "can_control", "runtime_id", "event_sequence", "hole_cards", "seed", "deck", "initial_buy_in_units", "ledger"} {
		if strings.Contains(string(raw), `"`+field+`"`) {
			t.Fatalf("Lobby exposed private/control field %s", field)
		}
	}
	if receiptFacts(t, f) != before || len(reader.actors) != 0 || len(reader.connections) != 0 {
		t.Fatal("read changed business facts/Redis/notices or allocated runtime authority")
	}
	legacy := struct {
		UserID                 int64
		Key, Name, BlindPreset string
		MaxSeats               int
		AllowSpectators        bool
	}{otherCommand.UserID, otherCommand.Key, otherCommand.Name, otherCommand.BlindPreset, otherCommand.MaxSeats, otherCommand.AllowSpectators}
	kh, rh := hashes(otherCommand.Key, legacy)
	var stored []byte
	if err = f.owner.QueryRow(ctx, `SELECT request_hash FROM poker.request_receipts WHERE newapi_user_id=$1 AND scope='poker.create.v1' AND key_hash=$2`, otherCommand.UserID, kh[:]).Scan(&stored); err != nil || !equal(stored, rh[:]) {
		t.Fatal("legacy create receipt hash changed", err)
	}
	otherCommand.AccessMode = "PUBLIC"
	if duplicate, e := f.service.CreateTable(ctx, otherCommand); e != nil || !duplicate.Duplicate || duplicate.TableID != other.TableID {
		t.Fatal("explicit PUBLIC lost legacy duplicate", duplicate, e)
	}
	for _, unsupported := range []CreateTableCommand{{UserID: 910003, Key: "lobby-unsupported-password", Name: "New", BlindPreset: "5-10", MaxSeats: 2, AccessMode: "PASSWORD"}, {UserID: 910003, Key: "lobby-unsupported-chat", Name: "New", BlindPreset: "5-10", MaxSeats: 2, ChatEnabled: true}, {UserID: 910003, Key: "lobby-unknown-real-preset", Name: "New", BlindPreset: "not-a-preset", MaxSeats: 2}} {
		if _, e := f.service.CreateTable(ctx, unsupported); !errors.Is(e, ErrInvalid) {
			t.Fatal("unsupported create fields silently dropped", e)
		}
	}
	if receiptFacts(t, f) != before {
		t.Fatal("duplicate or invalid create changed business facts")
	}
	if _, err = f.owner.Exec(ctx, `UPDATE poker.ruleset_versions SET evaluator_version='unsupported'`); err != nil {
		t.Fatal(err)
	}
	bad, err := reader.Lobby(ctx, 910003, LobbyFilter{})
	if err != nil || bad.Service.State != "CONFIG_INCOMPLETE" || bad.Service.ProductionReady || bad.Ruleset != nil || bad.Viewer.CanCreate || bad.Viewer.CanJoin {
		t.Fatalf("active flag is not a complete supported ruleset: %+v %v", bad.Service, err)
	}
	if _, err = reader.Lobby(ctx, 919999, LobbyFilter{}); err == nil {
		t.Fatal("missing wallet invented zero available chips")
	}
	t.Logf("owner read matches real %d-unit buyin; actual metadata, bounded filters and zero side effects verified", 400*engine.UnitsPerChip)
}

// Pause a real pgx query, not a production hook or replacement SQL function.
type lobbyQueryBarrier struct {
	sql     string
	before  bool
	used    atomic.Bool
	reached chan int
	resume  chan struct{}
}

func (b *lobbyQueryBarrier) pause(ctx context.Context, conn *pgx.Conn) {
	b.reached <- int(conn.PgConn().PID())
	select {
	case <-b.resume:
	case <-ctx.Done():
	}
}
func (b *lobbyQueryBarrier) TraceQueryStart(ctx context.Context, conn *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if !strings.Contains(d.SQL, b.sql) || !b.used.CompareAndSwap(false, true) {
		return ctx
	}
	if b.before {
		b.pause(ctx, conn)
	}
	return context.WithValue(ctx, b, true)
}
func (b *lobbyQueryBarrier) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, _ pgx.TraceQueryEndData) {
	if ctx.Value(b) == true && !b.before {
		b.pause(ctx, conn)
	}
}
func tracedLobbyService(t *testing.T, f *controlFixture, sql string, before, readonly bool) (*Service, *lobbyQueryBarrier) {
	t.Helper()
	b := &lobbyQueryBarrier{sql: sql, before: before, reached: make(chan int, 1), resume: make(chan struct{})}
	c := f.pool.Config()
	c.ConnConfig.Tracer = b
	if readonly {
		c.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	}
	p, err := pgxpool.NewWithConfig(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	opts := f.options()
	opts.Pool = p
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, b
}
func awaitLobbyBarrier(t *testing.T, b *lobbyQueryBarrier) int {
	t.Helper()
	select {
	case pid := <-b.reached:
		return pid
	case <-time.After(2 * time.Second):
		t.Fatal("real query barrier not reached")
	}
	return 0
}

func TestLobbyWalletSessionCoherentSnapshot(t *testing.T) {
	f := newControlFixture(t)
	table := f.table(t)
	ctx := context.Background()
	var session string
	for _, cashout := range []bool{false, true} {
		reader, barrier := tracedLobbyService(t, f, "economy.poker_lobby_balance", false, true)
		type result struct {
			snapshot LobbySnapshot
			err      error
		}
		done := make(chan result, 1)
		go func() { v, err := reader.Lobby(ctx, 910001, LobbyFilter{}); done <- result{v, err} }()
		awaitLobbyBarrier(t, barrier) // Wallet row read; session SELECT not yet issued.
		if !cashout {
			session = f.buy(t, table, 910001, 1)
		} else {
			if _, err := f.service.RequestSafeLeave(ctx, SessionCommand{UserID: 910001, Key: "lobby-coherent-safe-leave", TableID: table, TargetSessionID: session}); err != nil {
				t.Fatal(err)
			}
		}
		close(barrier.resume)
		r := <-done
		if r.err != nil {
			t.Fatal(r.err)
		}
		fresh, err := reader.Lobby(ctx, 910001, LobbyFilter{})
		if err != nil {
			t.Fatal(err)
		}
		before, after := r.snapshot, fresh
		if cashout {
			before, after = fresh, r.snapshot
		}
		if before.Viewer.AvailableChipsUnits != "1500000000" || before.Viewer.PokerInPlayUnits != "0" || before.ActiveSession != nil || after.Viewer.AvailableChipsUnits != "1300000000" || after.Viewer.PokerInPlayUnits != "200000000" || after.ActiveSession == nil || after.ActiveSession.SessionID != session {
			t.Fatalf("wallet/session torn across real committed buyin/cashout=%v: old=%+v new=%+v", cashout, r.snapshot.Viewer, fresh.Viewer)
		}
	}
	t.Log("real buyin and cashout committed between wallet and active-session SELECTs; each Lobby remained one RR read-only snapshot")
}
