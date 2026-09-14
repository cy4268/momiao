package poker

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker/engine"
)

func TestHostAcceptingDomain(t *testing.T) {
	owner, pool := localPokerDB(t) // All 21 migrations and existing narrow Poker grants; no skip or grant edits.
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	must := func(t *testing.T, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	files, err := filepath.Glob(filepath.Join("..", "platform", "migrations", "*.sql"))
	must(t, err)
	var privileges bool
	must(t, owner.QueryRow(ctx, `SELECT to_regclass('games.history_index') IS NOT NULL AND has_column_privilege($1,'poker.tables','accepting_players','UPDATE') AND NOT has_any_column_privilege($1,'economy.wallet_balances','INSERT,UPDATE') AND NOT has_table_privilege($1,'economy.wallet_balances','DELETE,TRUNCATE') AND NOT EXISTS(SELECT 1 FROM pg_class c WHERE c.oid IN('games.history_display_snapshots'::regclass,'games.history_index'::regclass,'games.history_ingestion_cursors'::regclass,'games.history_source_rows'::regclass) AND (has_any_column_privilege($1,c.oid,'SELECT,INSERT,UPDATE,REFERENCES') OR has_table_privilege($1,c.oid,'DELETE,TRUNCATE,TRIGGER'))) AND NOT EXISTS(SELECT 1 FROM pg_proc p WHERE p.oid IN('games.history_capture_display()'::regprocedure,'games.history_ingest_batch(text,integer)'::regprocedure,'games.history_list(bigint,jsonb)'::regprocedure) AND has_function_privilege($1,p.oid,'EXECUTE'))`, pool.Config().ConnConfig.User).Scan(&privileges))
	if len(files) != 21 || !privileges {
		t.Fatal("current21 objects or narrow runtime privileges changed")
	}
	opts := Options{Pool: pool, Keyring: Keyring{Current: "h1", Keys: map[string][]byte{"h1": make([]byte, 32)}}, Leases: &fixtureLeases{entries: map[string]leaseEntry{}}, MailboxCapacity: 8}
	s, err := legacyTestNew(opts)
	must(t, err) // Existing synthetic control/auth boundary; engine and PostgreSQL are real.
	defer func() { s.Close() }()
	created, err := s.CreateTable(ctx, CreateTableCommand{UserID: 910001, Key: "host-accepting-create", Name: "Host accepting domain", BlindPreset: "5-10", MaxSeats: 3, AllowSpectators: true})
	must(t, err)
	table := created.TableID
	err = s.Tick(ctx, table)
	must(t, err) // Finish normal actor hydration before comparing command-only effects.
	facts := func(t *testing.T) string {
		var out []string
		for _, name := range []string{"poker.tables", "poker.seats", "poker.sessions", "poker.hands", "poker.hand_fairness", "poker.hand_participants", "poker.actions", "poker.dealt_cards", "poker.recovery_state", "poker.audit_events", "poker.seat_reservations", "poker.pots", "poker.pot_eligible_players", "poker.pot_awards", "poker.settlements", "poker.funding_operations", "economy.wallet_balances", "economy.wallet_ledger"} {
			projection := "to_jsonb(t)"
			if name == "poker.tables" {
				projection += "-'accepting_players'-'table_version'"
			}
			var digest string
			must(t, owner.QueryRow(ctx, "SELECT md5(coalesce(jsonb_agg(("+projection+") ORDER BY ("+projection+")::text)::text,'[]')) FROM "+name+" t").Scan(&digest))
			out = append(out, digest)
		}
		return strings.Join(out, ":") // Never log deck, seed, encrypted snapshots, private cards, or credentials.
	}
	state := func(t *testing.T) (bool, uint64) {
		var accepting bool
		var version uint64
		must(t, owner.QueryRow(ctx, "SELECT accepting_players,table_version FROM poker.tables WHERE table_id=$1", table).Scan(&accepting, &version))
		return accepting, version
	}
	var original HostCommand
	var originalReceipt Receipt
	for _, mode := range []string{"WAITING", "PAUSED", "IN_HAND"} {
		if mode == "PAUSED" {
			// Explicit boundary setup, not a claim that an Ops pause route ran.
			_, err = owner.Exec(ctx, "UPDATE poker.tables SET lifecycle_state='PAUSED',allow_new_hands=false WHERE table_id=$1", table)
			must(t, err)
		}
		if mode == "IN_HAND" {
			_, err = owner.Exec(ctx, "UPDATE poker.tables SET lifecycle_state='WAITING',allow_new_hands=true WHERE table_id=$1", table)
			must(t, err)
			for seat, user := range []int64{910001, 910002} {
				r, e := s.ReserveSeat(ctx, ReserveSeatCommand{UserID: user, Key: fmt.Sprintf("host-accepting-seat-%d", seat), TableID: table, Seat: seat + 1})
				must(t, e)
				_, e = s.BuyIn(ctx, BuyInCommand{UserID: user, Key: fmt.Sprintf("host-accepting-buy-%d", seat), TableID: table, ReservationID: r.ReservationID, AmountUnits: 400 * engine.UnitsPerChip})
				must(t, e)
				legacyRef(t, s, table, user)
			}
			if !t.Failed() {
				_, err = s.HostControl(ctx, HostCommand{UserID: 910001, Key: "host-accepting-before-hand", TableID: table, Kind: "PAUSE_ACCEPTING_PLAYERS"})
				must(t, err)
			}
			hand, e := s.StartHand(ctx, StartHandCommand{UserID: 910001, Key: "host-accepting-hand", TableID: table})
			must(t, e)
			if hand.HandID == "" {
				t.Fatal("real engine/PG hand was not started")
			}
		}
		for _, command := range []struct {
			kind      string
			accepting bool
		}{{"PAUSE_ACCEPTING_PLAYERS", false}, {"RESUME_ACCEPTING_PLAYERS", true}} {
			t.Run(mode+"/"+command.kind, func(t *testing.T) {
				before := facts(t)
				_, version := state(t)
				c := HostCommand{UserID: 910001, Key: "host-accepting-" + mode + command.kind, TableID: table, Kind: command.kind}
				r, e := s.HostControl(ctx, c)
				if e != nil {
					t.Fatalf("original V1 command rejected: %v", e)
				}
				accepting, afterVersion := state(t)
				if accepting != command.accepting || afterVersion != version+1 || r.Version != afterVersion || r.Status != mode || facts(t) != before {
					t.Fatal("accepting-only mutation changed lifecycle, hand, configuration, or funds")
				}
				if !command.accepting {
					original, originalReceipt = c, r
				}
			})
		}
	}
	if t.Failed() {
		return
	} // The original implementation produces six behavior REDs, not dependency/setup errors.
	before := facts(t)
	accepting, version := state(t)
	duplicate, err := s.HostControl(ctx, original)
	must(t, err)
	if !duplicate.Duplicate || duplicate.Version != originalReceipt.Version || duplicate.Status != originalReceipt.Status {
		t.Fatal("same-key replay lost original receipt")
	}
	changed := original
	changed.Kind = "RESUME_ACCEPTING_PLAYERS"
	if _, err = s.HostControl(ctx, changed); err != ErrConflict {
		t.Fatal("same key with different intent must conflict", err)
	}
	for _, command := range []HostCommand{{UserID: 910003, Key: "host-accepting-not-owner", TableID: table, Kind: original.Kind}, {UserID: 910001, Key: "host-accepting-unknown", TableID: table, Kind: "UNKNOWN"}, {UserID: 910001, Key: "short", TableID: table, Kind: original.Kind}} {
		if _, err = s.HostControl(ctx, command); err == nil {
			t.Fatal("untrusted or invalid host intent accepted")
		}
	}
	afterAccepting, afterVersion := state(t)
	if accepting != afterAccepting || version != afterVersion || facts(t) != before {
		t.Fatal("replay/conflict/denial changed durable state")
	}
	paused := HostCommand{UserID: 910001, Key: "host-accepting-admission-pause", TableID: table, Kind: "PAUSE_ACCEPTING_PLAYERS"}
	_, err = s.HostControl(ctx, paused)
	must(t, err)
	reserve := ReserveSeatCommand{UserID: 910003, Key: "host-accepting-blocked-seat", TableID: table, Seat: 3}
	if _, err = s.ReserveSeat(ctx, reserve); err != ErrDenied {
		t.Fatal("pause still admits a new player", err)
	}
	paused.Kind, paused.Key = "RESUME_ACCEPTING_PLAYERS", "host-accepting-admission-resume"
	_, err = s.HostControl(ctx, paused)
	must(t, err)
	reserve.Key = "host-accepting-allowed-seat"
	reservation, err := s.ReserveSeat(ctx, reserve)
	must(t, err)
	if reservation.ReservationID == "" {
		t.Fatal("resume did not restore new-player admission")
	}
	s.Close()
	s, err = legacyTestNew(opts)
	must(t, err)
	duplicate, err = s.HostControl(ctx, original)
	must(t, err) // Restart hydration may change its own epoch/recovery fields.
	if !duplicate.Duplicate || duplicate.Version != originalReceipt.Version || duplicate.Status != originalReceipt.Status {
		t.Fatal("restart lost durable receipt")
	}
	for _, closed := range []string{"CLOSING", "CLOSED"} {
		// Boundary fixture only: the complete safe-close chain is the independent H2 gate.
		_, err = owner.Exec(ctx, "UPDATE poker.tables SET lifecycle_state=$2 WHERE table_id=$1", table, closed)
		must(t, err)
		before = facts(t)
		accepting, version = state(t)
		for _, kind := range []string{"PAUSE_ACCEPTING_PLAYERS", "RESUME_ACCEPTING_PLAYERS"} {
			if _, err = s.HostControl(ctx, HostCommand{UserID: 910001, Key: "host-accepting-" + closed + kind, TableID: table, Kind: kind}); err == nil {
				t.Fatal("closing/closed table accepted a new host intent")
			}
		}
		afterAccepting, afterVersion = state(t)
		if accepting != afterAccepting || version != afterVersion || facts(t) != before {
			t.Fatal("closing/closed denial changed state")
		}
	}
	var receipts int
	must(t, owner.QueryRow(ctx, "SELECT count(*) FROM poker.request_receipts WHERE table_id=$1 AND scope='poker.host.v1'", table).Scan(&receipts))
	if receipts != 9 {
		t.Fatal("duplicate, conflicting or denied command wrote extra host receipts", receipts)
	}
	t.Log("H1 current21 narrow-PG grants, accepting-only in three lifecycles, new-player admission, owner/invalid/closed denial, idempotency/conflict/restart passed; control/auth are existing synthetic fixtures")
}
