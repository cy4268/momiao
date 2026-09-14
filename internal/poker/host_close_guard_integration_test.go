package poker

import (
	"context"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker/engine"
)

// Catches either CLOSED writer ignoring residual facts, or auto reporting a zero-row close.
func TestHostCloseFinalGuard(t *testing.T) {
	facts := []struct {
		name, session, pending string
		chips                  int64
		hand, control          bool
	}{
		{"orphan_active", "ACTIVE", "", 400, false, false},
		{"orphan_needs_review", "NEEDS_REVIEW", "", 0, false, false},
		{"settled_nonzero", "SETTLED", "", 1, false, false},
		{"pending_buy_in", "", "BUY_IN", 0, false, false},
		{"pending_cash_out", "", "CASH_OUT", 0, false, false},
		{"orphan_committed_hand", "", "", 0, true, false},
		{"empty_control", "", "", 0, false, true},
		{"settled_zero_control", "SETTLED", "", 0, false, true},
	}
	for _, entry := range []string{"manual", "auto"} {
		for _, fact := range facts {
			observed := false
			t.Run(entry+"/"+fact.name, func(t *testing.T) { // Sequential inherited PID-scoped fixtures.
				owner, pool := localPokerDB(t) // This helper seeds wallets before the measured boundary.
				ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
				defer cancel()
				must := func(err error) {
					t.Helper()
					if err != nil {
						t.Fatal("infrastructure:", err)
					}
				}
				var ready bool
				must(pool.QueryRow(ctx, `SELECT current_user=$1 AND session_user=current_user
				 AND NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=current_user
				 AND (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls))
				 AND NOT EXISTS(SELECT 1 FROM unnest(ARRAY['economy.wallet_balances','economy.wallet_ledger','economy.asset_transactions']) name
				 WHERE has_any_column_privilege(current_user,name,'INSERT,UPDATE') OR has_table_privilege(current_user,name,'DELETE,TRUNCATE'))`,
					pool.Config().ConnConfig.User).Scan(&ready))
				if !ready {
					t.Fatal("infrastructure: runtime identity or grants")
				}
				var publications atomic.Int64
				s, err := legacyTestNew(Options{Pool: pool, Leases: &fixtureLeases{entries: map[string]leaseEntry{}},
					Keyring:     Keyring{Current: "c1g", Keys: map[string][]byte{"c1g": make([]byte, 32)}},
					AfterCommit: func(string, uint64) { publications.Add(1) }})
				must(err)
				defer s.Close()
				created, err := s.CreateTable(ctx, CreateTableCommand{UserID: 910001, Key: "c1g-create-guard-table", Name: "Close guard", BlindPreset: "5-10", MaxSeats: 2})
				must(err)
				table, state := created.TableID, "CLOSING"
				if entry == "auto" {
					state = "WAITING"
				}
				tx, err := owner.Begin(ctx)
				must(err)
				defer rollback(tx)
				_, err = tx.Exec(ctx, `UPDATE poker.tables SET lifecycle_state=$2,accepting_players=$3,allow_new_hands=$3,
				 empty_since=CASE WHEN $3 THEN clock_timestamp()-interval '31 minutes' ELSE empty_since END WHERE table_id=$1`, table, state, entry == "auto")
				must(err)
				if fact.session != "" {
					_, err = tx.Exec(ctx, `INSERT INTO poker.sessions(session_id,newapi_user_id,table_id,seat_no,display_name_snapshot,state,initial_buyin_units,current_stack_units)
					 VALUES($2,910001,$1,1,'Guard fixture',$3,$4,$4)`, table, uuid(), fact.session, fact.chips*engine.UnitsPerChip)
					must(err)
				}
				if fact.pending != "" {
					_, err = tx.Exec(ctx, `INSERT INTO poker.funding_operations(funding_operation_id,table_id,seat_no,session_id,newapi_user_id,kind,
					 amount_units,request_hash,planned_transaction_id,planned_ledger_id)
					 VALUES($2,$1,1,$3,910001,$4,0,decode(repeat('00',32),'hex'),$5,$6)`, table, uuid(), uuid(), fact.pending, uuid(), uuid())
					must(err)
				}
				if fact.hand {
					_, err = tx.Exec(ctx, `INSERT INTO poker.hands(hand_id,table_id,hand_no,state,button_seat,runtime_epoch,setup_cipher)
					 VALUES($2,$1,1,'COMMITTED',1,0,''::bytea)`, table, uuid())
					must(err)
				}
				must(tx.Commit(ctx)) // All owner setup and 0021 trigger effects precede snapshots.
				must(owner.QueryRow(ctx, `SELECT lifecycle_state=$2 AND accepting_players=$3 AND allow_new_hands=$3
				 AND closed_at IS NULL AND current_hand_id IS NULL AND runtime_epoch=0 AND spectator_count=0
				 AND ($3=false OR empty_since<=clock_timestamp()-interval '30 minutes')
				 AND NOT EXISTS(SELECT 1 FROM poker.recovery_state) AND NOT EXISTS(SELECT 1 FROM poker.seats WHERE session_id IS NOT NULL)
				 AND (SELECT count(*) FROM poker.sessions)=CASE WHEN $4='' THEN 0 ELSE 1 END
				 AND NOT EXISTS(SELECT 1 FROM poker.sessions WHERE state<>$4 OR initial_buyin_units<>$5 OR current_stack_units<>$5)
				 AND (SELECT count(*) FROM poker.funding_operations)=CASE WHEN $6='' THEN 0 ELSE 1 END
				 AND NOT EXISTS(SELECT 1 FROM poker.funding_operations WHERE kind<>$6 OR state<>'PENDING' OR amount_units<>0)
				 AND (SELECT count(*) FROM poker.hands)=CASE WHEN $7 THEN 1 ELSE 0 END
				 AND NOT EXISTS(SELECT 1 FROM poker.hands WHERE state<>'COMMITTED' OR runtime_epoch<>0)
				 FROM poker.tables WHERE table_id=$1`, table, state, entry == "auto", fact.session, fact.chips*engine.UnitsPerChip, fact.pending, fact.hand).Scan(&ready))
				if !ready || len(s.actors) != 0 {
					t.Fatal("infrastructure: exact single-fact entry preconditions")
				}
				digest := func() string {
					t.Helper()
					var hashes []string
					for _, name := range []string{
						"poker.seats", "poker.sessions", "poker.hands", "poker.hand_fairness", "poker.hand_participants",
						"poker.actions", "poker.dealt_cards", "poker.recovery_state", "poker.audit_events", "poker.seat_reservations",
						"poker.pots", "poker.pot_eligible_players", "poker.pot_awards", "poker.settlements", "poker.funding_operations",
						"poker.request_receipts", "economy.wallet_balances", "economy.wallet_ledger", "economy.asset_transactions",
					} {
						var hash string
						must(owner.QueryRow(ctx, "SELECT md5(coalesce(jsonb_agg(to_jsonb(x) ORDER BY to_jsonb(x)::text)::text,'[]')) FROM "+name+" x").Scan(&hash))
						hashes = append(hashes, hash)
					}
					return strings.Join(hashes, ":")
				}
				type snapshot struct {
					state, full, stable      string
					version                  uint64
					closed, accepting, hands bool
					empty                    *time.Time
				}
				read := func() snapshot {
					t.Helper()
					var v snapshot
					must(owner.QueryRow(ctx, `SELECT lifecycle_state,table_version,closed_at IS NOT NULL,accepting_players,allow_new_hands,empty_since,
					 md5(to_jsonb(x)::text),md5((to_jsonb(x)-ARRAY['lifecycle_state','table_version','closed_at','accepting_players','allow_new_hands','empty_since'])::text)
					 FROM poker.tables x WHERE table_id=$1`, table).Scan(&v.state, &v.version, &v.closed, &v.accepting, &v.hands, &v.empty, &v.full, &v.stable))
					return v
				}
				before, otherFacts, pubs := read(), digest(), publications.Load()
				a := tableActor{s: s, id: table, epoch: 0}
				reply, err := a.execute(ctx, s.tickMutation)
				must(err) // No direct helper call, actor goroutine, owner mutation or fake transaction.
				after, unchanged := read(), digest() == otherFacts
				delta := publications.Load() - pubs
				t.Logf("fact=%s entry=%s residual_unchanged=%t DB=%s closed_at=%t reply=%s version_delta=%d publications=%d",
					fact.name, entry, unchanged, after.state, after.closed, reply.Status, after.version-before.version, delta)
				if !unchanged || before.stable != after.stable || len(s.actors) != 0 {
					t.Error("non-table facts, stable table columns or actor registry changed")
				}
				if entry == "auto" && !fact.control {
					if reply != (Receipt{Status: "NOOP"}) || before.full != after.full || delta != 0 {
						t.Error("blocked auto close must be a zero-effect NOOP, not CLOSED")
					}
				} else {
					want := Receipt{TableID: table, Status: "BOUNDARY", Version: before.version + 1}
					if entry == "auto" {
						want.Status = "CLOSED"
					}
					if reply != want || after.version != before.version+1 || delta != 1 {
						t.Error("accepted boundary/control receipt, version or publication mismatch")
					}
					wantState := state
					if fact.control {
						wantState = "CLOSED"
					}
					if after.state != wantState || after.closed != fact.control || after.accepting || after.hands {
						t.Error("terminal state/closed_at/flags violated residual guard")
					}
					wantEmpty := before.empty
					if entry == "manual" && (fact.pending != "" || fact.hand) {
						wantEmpty = nil
					}
					if !reflect.DeepEqual(after.empty, wantEmpty) {
						t.Error("existing empty_since semantics changed")
					}
				}
				observed = true
			})
			if !observed {
				t.Fatal("infrastructure: stop remaining cases after incomplete observation")
			}
		}
	}
}
