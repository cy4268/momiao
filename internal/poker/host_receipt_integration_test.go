package poker

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHostReceiptReadAfterClosedRestart(t *testing.T) {
	owner, pool := localPokerDB(t) // All 21 migrations and existing grants; no skip or helper changes.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	must := func(t *testing.T, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	files, err := filepath.Glob(filepath.Join("..", "platform", "migrations", "*.sql"))
	must(t, err)
	var narrow bool
	must(t, owner.QueryRow(ctx, `SELECT to_regclass('games.history_index') IS NOT NULL AND has_table_privilege($1,'poker.request_receipts','SELECT') AND NOT has_any_column_privilege($1,'economy.wallet_balances','INSERT,UPDATE') AND NOT has_table_privilege($1,'economy.wallet_balances','DELETE,TRUNCATE') AND NOT EXISTS(SELECT 1 FROM pg_class c WHERE c.oid IN('games.history_display_snapshots'::regclass,'games.history_index'::regclass,'games.history_ingestion_cursors'::regclass,'games.history_source_rows'::regclass) AND (has_any_column_privilege($1,c.oid,'SELECT,INSERT,UPDATE,REFERENCES') OR has_table_privilege($1,c.oid,'DELETE,TRUNCATE,TRIGGER'))) AND NOT EXISTS(SELECT 1 FROM pg_proc p WHERE p.oid IN('games.history_capture_display()'::regprocedure,'games.history_ingest_batch(text,integer)'::regprocedure,'games.history_list(bigint,jsonb)'::regprocedure) AND has_function_privilege($1,p.oid,'EXECUTE'))`, pool.Config().ConnConfig.User).Scan(&narrow))
	if len(files) != 21 || !narrow {
		t.Fatal("current21 or narrow runtime grants changed")
	}
	opts := Options{Pool: pool, Keyring: Keyring{Current: "h2r", Keys: map[string][]byte{"h2r": make([]byte, 32)}}, Leases: &fixtureLeases{entries: map[string]leaseEntry{}}, MailboxCapacity: 8}
	s, err := legacyTestNew(opts)
	must(t, err) // Auth/control are existing synthetic fixtures; PostgreSQL and the domain are real.
	defer s.Close()
	created, err := s.CreateTable(ctx, CreateTableCommand{UserID: 910001, Key: "h2r-create-empty-table", Name: "Host receipt", BlindPreset: "5-10", MaxSeats: 2, AllowSpectators: false})
	must(t, err)
	table := created.TableID
	type saved struct {
		key string
		got Receipt
	}
	var originals []saved
	for _, kind := range []string{"PAUSE_ACCEPTING_PLAYERS", "CLOSE"} {
		key := "h2r-original-" + kind
		r, err := s.HostControl(ctx, HostCommand{UserID: 910001, Key: key, TableID: table, Kind: kind})
		must(t, err)
		originals = append(originals, saved{key, r})
	}
	s.Close()
	var state string
	var empty bool
	must(t, owner.QueryRow(ctx, `SELECT lifecycle_state,NOT accepting_players AND NOT allow_new_hands AND NOT allow_spectators AND NOT EXISTS(SELECT 1 FROM poker.sessions) AND NOT EXISTS(SELECT 1 FROM poker.hands) AND NOT EXISTS(SELECT 1 FROM poker.funding_operations) FROM poker.tables WHERE table_id=$1`, table).Scan(&state, &empty))
	if state != "CLOSED" || !empty || originals[1].got.Status != "CLOSING" {
		t.Fatal("real empty-table close or original acknowledgement changed")
	}
	readonlyConfig := pool.Config()
	readonlyConfig.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	readonly, err := pgxpool.NewWithConfig(ctx, readonlyConfig)
	must(t, err)
	defer readonly.Close()
	var readOnly string
	must(t, readonly.QueryRow(ctx, "SHOW transaction_read_only").Scan(&readOnly))
	var bound bool
	must(t, readonly.QueryRow(ctx, "SELECT current_database()=$1 AND current_user=$2", pool.Config().ConnConfig.Database, pool.Config().ConnConfig.User).Scan(&bound))
	if readOnly != "on" || !bound {
		t.Fatal("restarted service does not have a readonly PG pool")
	}
	opts.Pool = readonly
	restarted, err := legacyTestNew(opts)
	must(t, err)
	defer restarted.Close()
	facts := func(t *testing.T) string {
		t.Helper()
		var out []string
		for _, name := range []string{"poker.tables", "poker.seats", "poker.sessions", "poker.hands", "poker.hand_fairness", "poker.hand_participants", "poker.actions", "poker.dealt_cards", "poker.recovery_state", "poker.audit_events", "poker.seat_reservations", "poker.pots", "poker.pot_eligible_players", "poker.pot_awards", "poker.settlements", "poker.funding_operations", "poker.request_receipts", "economy.wallet_balances", "economy.wallet_ledger"} {
			var digest string
			must(t, owner.QueryRow(ctx, "SELECT md5(coalesce(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text)::text,'[]')) FROM "+name+" t").Scan(&digest))
			out = append(out, digest)
		}
		return strings.Join(out, ":") // Compare private facts without emitting their contents.
	}
	before := facts(t)
	for _, original := range originals {
		t.Run(original.key, func(t *testing.T) {
			q := ReceiptQuery{TableID: table, Kind: "host", MutationID: original.key}
			for range 2 {
				got, err := restarted.LookupReceipt(ctx, 910001, q)
				if err != nil || got.State != "FOUND" || got.Kind != q.Kind || got.MutationID != q.MutationID || got.TableID != table || got.Receipt == nil || *got.Receipt != original.got {
					t.Errorf("original host receipt after CLOSED/restart: %v", err)
				}
			}
		})
	}
	for _, tc := range []struct {
		name string
		user int64
		q    ReceiptQuery
	}{
		{"wrong-user", 910002, ReceiptQuery{table, "host", originals[0].key}},
		{"wrong-table", 910001, ReceiptQuery{uuid(), "host", originals[0].key}},
		{"new-key", 910001, ReceiptQuery{table, "host", "h2r-uncommitted-request"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := restarted.LookupReceipt(ctx, tc.user, tc.q)
			if err != nil || got.State != "NOT_FOUND" || got.Receipt != nil || got.Kind != tc.q.Kind || got.TableID != tc.q.TableID || got.MutationID != tc.q.MutationID {
				t.Errorf("host receipt isolation: %v", err)
			}
		})
	}
	for _, q := range []ReceiptQuery{{table, "HOST", originals[0].key}, {table, "host", "short"}, {"not-a-table", "host", originals[0].key}} {
		if _, err := restarted.LookupReceipt(ctx, 910001, q); err != ErrInvalid {
			t.Error("invalid receipt locator was accepted", err)
		}
	}
	restarted.mu.Lock()
	noActors := len(restarted.actors) == 0 && len(restarted.connections) == 0 && len(restarted.claimEligible) == 0
	restarted.mu.Unlock()
	if !noActors || before != facts(t) {
		t.Fatal("receipt read created runtime authority or changed PG facts")
	}
	if !t.Failed() {
		t.Log("H2-R current21 narrow-PG; true H1 pause/legacy empty close -> CLOSED -> new service/readonly PG; two original receipts preserved, owner/table/key isolation, zero actors and 19 unchanged PG facts; auth/control synthetic; no funded-close or closed-write-replay claim")
	}
}
