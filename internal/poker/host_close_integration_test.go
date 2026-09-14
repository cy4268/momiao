package poker

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker/engine"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHostCloseFundedNoHand(t *testing.T) {
	for _, kind := range []string{"CLOSE_TABLE", "CLOSE"} {
		t.Run(kind, func(t *testing.T) { // Sequential PID-scoped real PG; no owner row or clock injection.
			owner, pool := localPokerDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			opts := Options{Pool: pool, Keyring: Keyring{Current: "c1a", Keys: map[string][]byte{"c1a": make([]byte, 32)}}, Leases: &fixtureLeases{entries: map[string]leaseEntry{}}, MailboxCapacity: 8}
			s, err := legacyTestNew(opts) // Existing synthetic auth/leases; actual actor, PG and funding.
			must(err)
			defer func() { s.Close() }()
			created, err := s.CreateTable(ctx, CreateTableCommand{UserID: 910001, Key: "c1a-create-funded-table", Name: "Funded close", BlindPreset: "5-10", MaxSeats: 3, AllowSpectators: false})
			must(err)
			table := created.TableID
			var buys []BuyInCommand
			var bought []Receipt
			for seat, user := range []int64{910001, 910002} {
				r, err := s.ReserveSeat(ctx, ReserveSeatCommand{UserID: user, Key: fmt.Sprintf("c1a-reserve-seat-%d", seat), TableID: table, Seat: seat + 1})
				must(err)
				buy := BuyInCommand{UserID: user, Key: fmt.Sprintf("c1a-buy-funded-seat-%d", seat), TableID: table, ReservationID: r.ReservationID, AmountUnits: 400 * engine.UnitsPerChip}
				r, err = s.BuyIn(ctx, buy)
				must(err)
				if r.Status != "CONFIRMED" || r.SessionID == "" {
					t.Fatal("real funded Seat missing")
				}
				buys, bought = append(buys, buy), append(bought, r)
			} // No legacyRef or StartHand: funded Seats have no live control connection.
			third, err := s.ReserveSeat(ctx, ReserveSeatCommand{UserID: 910003, Key: "c1a-third-real-reservation", TableID: table, Seat: 3})
			must(err)
			must(s.Tick(ctx, table)) // Finish hydration before command-only comparisons.
			var ready bool
			must(owner.QueryRow(ctx, `SELECT
			 (SELECT count(*) FROM poker.sessions WHERE state='ACTIVE' AND current_stack_units=$1)=2
			 AND (SELECT count(*) FROM poker.funding_operations WHERE kind='BUY_IN' AND state='CONFIRMED')=2
			 AND NOT EXISTS(SELECT 1 FROM poker.funding_operations WHERE kind='CASH_OUT')
			 AND NOT EXISTS(SELECT 1 FROM poker.hands) AND NOT EXISTS(SELECT 1 FROM poker.settlements)
			 AND EXISTS(SELECT 1 FROM poker.seat_reservations WHERE reservation_id=$2 AND newapi_user_id=910003 AND state='LEASE_ACTIVE' AND expires_at>clock_timestamp())`, 400*engine.UnitsPerChip, third.ReservationID).Scan(&ready))
			if !ready {
				t.Fatal("funded no-Hand and valid third reservation preconditions missing")
			}
			facts := func() string {
				t.Helper()
				var out []string
				for _, name := range []string{"poker.tables", "poker.seats", "poker.sessions", "poker.hands", "poker.hand_fairness", "poker.hand_participants", "poker.actions", "poker.dealt_cards", "poker.recovery_state", "poker.audit_events", "poker.seat_reservations", "poker.pots", "poker.pot_eligible_players", "poker.pot_awards", "poker.settlements", "poker.funding_operations", "poker.request_receipts", "economy.wallet_balances", "economy.wallet_ledger", "economy.asset_transactions"} {
					var digest string
					must(owner.QueryRow(ctx, "SELECT md5(coalesce(jsonb_agg(to_jsonb(x) ORDER BY to_jsonb(x)::text)::text,'[]')) FROM "+name+" x").Scan(&digest))
					out = append(out, digest)
				}
				return strings.Join(out, ":") // Compare facts without printing private snapshots or wallet material.
			}
			before := facts()
			_, err = s.HostControl(ctx, HostCommand{UserID: 910003, Key: "c1a-non-owner-close", TableID: table, Kind: kind})
			if err != ErrDenied || facts() != before {
				t.Fatal("non-owner Close changed facts", err)
			}
			command := HostCommand{UserID: 910001, Key: "c1a-original-host-close", TableID: table, Kind: kind}
			var beforeVersion uint64
			must(owner.QueryRow(ctx, "SELECT table_version FROM poker.tables WHERE table_id=$1", table).Scan(&beforeVersion))
			original, err := s.HostControl(ctx, command)
			if err != nil {
				t.Fatalf("original %s rejected: %v", kind, err)
			}
			if original.Status != "CLOSING" || original.Duplicate || original.TableID != table || original.Version != beforeVersion+1 {
				t.Fatal("historical accepted receipt changed")
			}
			must(owner.QueryRow(ctx, `SELECT lifecycle_state='CLOSED' AND closed_at IS NOT NULL AND NOT accepting_players AND NOT allow_new_hands AND table_version=$3
			 AND NOT EXISTS(SELECT 1 FROM poker.seats WHERE session_id IS NOT NULL)
			 AND (SELECT count(*) FROM poker.sessions WHERE state='SETTLED' AND current_stack_units=0)=2
			 AND (SELECT count(*) FROM poker.funding_operations WHERE kind='CASH_OUT' AND state='CONFIRMED')=2
			 AND (SELECT count(*) FROM economy.asset_transactions WHERE biz_id LIKE 'poker_cashout:%')=2
			 AND (SELECT count(*) FROM economy.wallet_ledger WHERE biz_id LIKE 'poker_cashout:%')=2
			 AND NOT EXISTS(SELECT 1 FROM poker.hands) AND NOT EXISTS(SELECT 1 FROM poker.settlements)
			 AND NOT EXISTS(SELECT 1 FROM poker.funding_operations WHERE state='PENDING')
			 AND (SELECT sum(balance_units) FROM economy.wallet_balances WHERE newapi_user_id IN(910001,910002))=$2
			 FROM poker.tables WHERE table_id=$1`, table, 6000*engine.UnitsPerChip, original.Version).Scan(&ready))
			if !ready {
				t.Fatal("funded close was not unique, terminal and balanced")
			}
			for i, buy := range buys {
				must(owner.QueryRow(ctx, `SELECT (SELECT balance_units FROM economy.wallet_balances WHERE newapi_user_id=$1 AND asset_type='AVAILABLE_CHIPS')=$3
				 AND (SELECT count(*) FROM poker.funding_operations f JOIN economy.asset_transactions a ON a.transaction_id=f.confirmed_transaction_id
				 JOIN economy.wallet_ledger l ON l.ledger_entry_id=f.confirmed_ledger_id AND l.transaction_id=a.transaction_id
				 WHERE f.session_id=$2 AND f.newapi_user_id=$1 AND f.kind='CASH_OUT' AND f.state='CONFIRMED' AND f.amount_units=$4
				 AND a.biz_id='poker_cashout:'||$2::text AND a.newapi_user_id=$1 AND l.newapi_user_id=$1 AND l.delta_units=$4)=1`, buy.UserID, bought[i].SessionID, 3000*engine.UnitsPerChip, 400*engine.UnitsPerChip).Scan(&ready))
				if !ready {
					t.Fatal("individual Session cashout/ledger or wallet mismatch")
				}
			}
			before = facts()
			replay, err := s.HostControl(ctx, command)
			must(err)
			expected := original
			expected.Duplicate = true
			changed := command
			changed.Kind = "CLOSE"
			if kind == "CLOSE" {
				changed.Kind = "CLOSE_TABLE"
			}
			_, err = s.HostControl(ctx, changed)
			if replay != expected || err != ErrConflict {
				t.Fatal("original Kind was normalized or original receipt lost", err)
			}
			for i, buy := range buys {
				replay, err = s.BuyIn(ctx, buy)
				expected = bought[i]
				expected.Duplicate = true
				if err != nil || replay != expected {
					t.Fatal("old successful BuyIn receipt lost", err)
				}
			}
			_, reserveErr := s.ReserveSeat(ctx, ReserveSeatCommand{UserID: 910003, Key: "c1a-closed-new-reserve", TableID: table, Seat: 3})
			_, buyErr := s.BuyIn(ctx, BuyInCommand{UserID: 910003, Key: "c1a-closed-valid-reservation-buy", TableID: table, ReservationID: third.ReservationID, AmountUnits: 400 * engine.UnitsPerChip})
			_, startErr := s.StartHand(ctx, StartHandCommand{UserID: 910001, Key: "c1a-closed-new-hand", TableID: table})
			if reserveErr != ErrDenied || buyErr != ErrDenied || startErr != ErrDenied {
				t.Fatal("closed table admitted new funded work", reserveErr, buyErr, startErr)
			}
			for _, kind := range []string{"PAUSE_ACCEPTING_PLAYERS", "RESUME_ACCEPTING_PLAYERS", "RESUME"} {
				_, err = s.HostControl(ctx, HostCommand{UserID: 910001, Key: "c1a-closed-new-" + kind, TableID: table, Kind: kind})
				if err != ErrDenied {
					t.Fatal("closed table accepted a new Host intent", err)
				}
			}
			if facts() != before {
				t.Fatal("replay/conflict/denial changed durable facts or version")
			}
			s.Close()
			readonlyConfig := pool.Config()
			readonlyConfig.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
			readonly, err := pgxpool.NewWithConfig(ctx, readonlyConfig)
			must(err)
			defer readonly.Close()
			var readOnly string
			must(readonly.QueryRow(ctx, "SHOW transaction_read_only").Scan(&readOnly))
			must(readonly.QueryRow(ctx, "SELECT current_database()=$1 AND current_user=$2", pool.Config().ConnConfig.Database, pool.Config().ConnConfig.User).Scan(&ready))
			if readOnly != "on" || !ready {
				t.Fatal("restart pool is not the same readonly runtime")
			}
			opts.Pool = readonly
			s, err = legacyTestNew(opts)
			must(err)
			for range 2 {
				got, err := s.LookupReceipt(ctx, 910001, ReceiptQuery{table, "host", command.Key})
				if err != nil || got.State != "FOUND" || got.Receipt == nil || *got.Receipt != original {
					t.Fatal("readonly restart lost original CLOSING receipt", err)
				}
			}
			if len(s.actors) != 0 || facts() != before {
				t.Fatal("readonly restart changed facts or created an actor")
			}
			t.Log("real PG funded/no-Hand close: two unique 400-chip Cash Outs/ledgers, each wallet 3000/total6000, original Kind replay/conflict, valid prior reservation BuyIn denied, immutable original receipt on readonly restart; synthetic auth/lease only")
		})
	}
}
