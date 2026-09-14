package poker

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker/engine"
	"github.com/cy4268/momiao/internal/testutil/pokerhistoryfixture"
	"github.com/jackc/pgx/v5/pgxpool"
)

func rootCurrent24CloseFixture(t *testing.T) (*pgxpool.Pool, *pgxpool.Pool) {
	t.Helper()
	f := pokerhistoryfixture.Open(t)
	for _, user := range []int64{910001, 910002, 910003} {
		f.SQL(t, `INSERT INTO identity.account_refs(newapi_user_id) VALUES($1);
          INSERT INTO identity.master_profiles(newapi_user_id,display_name,normalized_name) VALUES($1,$2,$2);
          INSERT INTO economy.wallet_balances(newapi_user_id,asset_type,balance_units) VALUES($1,'AVAILABLE_CHIPS',$3)`, user, fmt.Sprintf("Synthetic %d", user), 3000*engine.UnitsPerChip)
	}
	return f.Pool, f.Poker
}

func TestHostCloseRecoveryIntent(t *testing.T) {
	for _, mode := range []string{"close_then_two_restarts", "restart_then_close"} {
		t.Run(mode, func(t *testing.T) { // Sequential: each subtest owns and cleans a fresh root current24 database.
			owner, pool := rootCurrent24CloseFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
			defer cancel()
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			var narrow bool
			must(owner.QueryRow(ctx, `SELECT to_regclass('games.history_index') IS NOT NULL AND has_table_privilege($1,'poker.request_receipts','SELECT') AND NOT has_any_column_privilege($1,'economy.wallet_balances','INSERT,UPDATE') AND NOT has_table_privilege($1,'economy.wallet_balances','DELETE,TRUNCATE') AND NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1 AND (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls))`, pool.Config().ConnConfig.User).Scan(&narrow))
			if !narrow {
				t.Fatal("current schema or narrow runtime privileges changed")
			}
			opts := Options{Pool: pool, Keyring: Keyring{Current: "c0", Keys: map[string][]byte{"c0": make([]byte, 32)}}, Leases: &fixtureLeases{entries: map[string]leaseEntry{}}, MailboxCapacity: 8}
			s, err := legacyTestNew(opts) // Existing synthetic auth/control; actual PG, engine and funding.
			must(err)
			defer func() { s.Close() }()
			created, err := s.CreateTable(ctx, CreateTableCommand{UserID: 910001, Key: "c0-create-funded-table", Name: "Close recovery", BlindPreset: "5-10", MaxSeats: 2, AllowSpectators: false})
			must(err)
			table := created.TableID
			for seat, user := range []int64{910001, 910002} {
				r, err := s.ReserveSeat(ctx, ReserveSeatCommand{UserID: user, Key: fmt.Sprintf("c0-reserve-seat-%d", seat), TableID: table, Seat: seat + 1})
				must(err)
				_, err = s.BuyIn(ctx, BuyInCommand{UserID: user, Key: fmt.Sprintf("c0-buy-funded-seat-%d", seat), TableID: table, ReservationID: r.ReservationID, AmountUnits: 400 * engine.UnitsPerChip})
				must(err)
				legacyRef(t, s, table, user)
			}
			hand, err := s.StartHand(ctx, StartHandCommand{UserID: 910001, Key: "c0-start-real-hand", TableID: table})
			must(err)
			if hand.HandID == "" {
				t.Fatal("missing actual Hand")
			}
			digest := func(names ...string) string {
				t.Helper()
				var out []string
				for _, name := range names {
					var hash string
					must(owner.QueryRow(ctx, "SELECT md5(coalesce(jsonb_agg(to_jsonb(x) ORDER BY to_jsonb(x)::text)::text,'[]')) FROM "+name+" x").Scan(&hash))
					out = append(out, hash)
				}
				return strings.Join(out, ":") // Never print private engine, deck or wallet material.
			}
			funds := func() string {
				return digest("poker.sessions", "poker.hand_participants", "poker.funding_operations", "poker.settlements", "poker.pots", "poker.pot_awards", "economy.wallet_balances", "economy.wallet_ledger", "economy.asset_transactions")
			}
			money, fairness := funds(), digest("poker.hand_fairness", "poker.dealt_cards")
			var grace time.Time
			restart := func() {
				s.Close()
				s, err = legacyTestNew(opts)
				must(err)
				report, err := s.Start(ctx)
				must(err)
				v, viewErr := s.View(ctx, table, 910001)
				if viewErr != nil || v.LifecycleState != "RECOVERING" || v.Hand == nil || !v.Hand.Recovering {
					t.Fatal("recovery projection lost its raw lifecycle", viewErr)
				}
				if report.Loaded != 1 || report.Recovering != 1 {
					t.Fatal("real new Service did not hydrate its current Hand", report)
				}
				var next, now time.Time
				var state, previous string
				must(owner.QueryRow(ctx, "SELECT state,previous_table_state,grace_until,clock_timestamp() FROM poker.recovery_state WHERE table_id=$1 AND hand_id=$2", table, hand.HandID).Scan(&state, &previous, &next, &now))
				if state != "GRACE" || next.Sub(now) < 0 || next.Sub(now) > 30*time.Second || (!grace.IsZero() && !grace.Equal(next)) {
					t.Fatal("actual recovery window was lost or extended")
				}
				grace = next
				if mode == "close_then_two_restarts" && previous != "CLOSING" {
					t.Error("R1: repeated startup lost previous CLOSING:", previous)
				}
			}
			if mode == "restart_then_close" {
				restart()
			}
			command := HostCommand{UserID: 910001, Key: "c0-original-host-close", TableID: table, Kind: "CLOSE"}
			beforeHand := digest("poker.hands", "poker.actions")
			original, err := s.HostControl(ctx, command)
			must(err)
			if original.Status != "CLOSING" || original.Duplicate || money != funds() || beforeHand != digest("poker.hands", "poker.actions") || fairness != digest("poker.hand_fairness", "poker.dealt_cards") {
				t.Fatal("Close changed current Hand or cashed out a participant before Settlement")
			}
			if mode == "close_then_two_restarts" {
				restart()
			}
			before := digest("poker.tables", "poker.seats", "poker.request_receipts")
			replay, err := s.HostControl(ctx, command)
			must(err)
			expected := original
			expected.Duplicate = true
			conflict := command
			conflict.Kind = "PAUSE_ACCEPTING_PLAYERS"
			_, err = s.HostControl(ctx, conflict)
			if replay != expected || err != ErrConflict || before != digest("poker.tables", "poker.seats", "poker.request_receipts") {
				t.Fatal("historical receipt did not precede closing guard or conflict changed facts")
			}
			_, err = s.HostControl(ctx, HostCommand{UserID: 910001, Key: "c0-new-resume-accepting", TableID: table, Kind: "RESUME_ACCEPTING_PLAYERS"})
			if err != ErrDenied || before != digest("poker.tables", "poker.seats", "poker.request_receipts") {
				t.Error("R3: Close allowed a new Host command during recovery:", err)
			}
			if mode == "close_then_two_restarts" {
				restart()
			}
			if money != funds() || fairness != digest("poker.hand_fairness", "poker.dealt_cards") {
				t.Fatal("restart moved participant funds or changed committed cards")
			}
			for _, user := range []int64{910001, 910002} {
				legacyRef(t, s, table, user)
			}
			wait := time.Until(grace) + 30*time.Millisecond
			if wait < 0 || wait > 31*time.Second {
				t.Fatal("original recovery deadline was not retained")
			}
			time.Sleep(wait) // Actual original 30s grace; no clock/deck/SQL rewrites.
			must(s.Tick(ctx, table))
			v, err := s.View(ctx, table, 910001)
			must(err)
			if v.Hand == nil || v.Hand.HandID != hand.HandID || v.Hand.Recovering || v.Hand.ActionDeadlineAt == nil || v.Hand.ActionDeadlineAt.Sub(v.ServerNow) < 28*time.Second {
				t.Fatal("same Hand did not resume with its fresh decision clock")
			}
			var closedFlags bool
			must(owner.QueryRow(ctx, "SELECT NOT accepting_players AND NOT allow_new_hands AND hand_no=1 AND current_hand_id=$2 FROM poker.tables WHERE table_id=$1", table, hand.HandID).Scan(&closedFlags))
			if v.LifecycleState != "CLOSING" || !closedFlags {
				t.Error("R1/R2: resumed Hand lost accepted Close:", v.LifecycleState)
			}
			actorUser := int64(910000 + v.Hand.ActorSeat)
			_, err = legacyAct(t, s, ActCommand{UserID: actorUser, Key: "c0-normal-final-fold", TableID: table, HandID: hand.HandID, Kind: engine.Fold})
			must(err) // Actual engine selects the winner and normal Settlement/Cash Out amounts.
			var lifecycle string
			var settled bool
			must(owner.QueryRow(ctx, `SELECT lifecycle_state,NOT accepting_players AND NOT allow_new_hands,hand_no=1 AND NOT EXISTS(SELECT 1 FROM poker.seats WHERE session_id IS NOT NULL) AND (SELECT count(*) FROM poker.sessions WHERE state='SETTLED' AND current_stack_units=0)=2 AND (SELECT count(*) FROM poker.funding_operations WHERE kind='CASH_OUT' AND state='CONFIRMED')=2 AND (SELECT count(*) FROM economy.asset_transactions WHERE biz_id LIKE 'poker_cashout:%')=2 AND (SELECT count(*) FROM poker.settlements WHERE hand_id=$2 AND total_commitment_units=total_award_units+uncalled_return_units)=1 AND (SELECT count(*) FROM poker.hands)=1 AND NOT EXISTS(SELECT 1 FROM poker.hands WHERE state<>'SETTLED') AND NOT EXISTS(SELECT 1 FROM poker.funding_operations WHERE state='PENDING') AND (SELECT sum(balance_units) FROM economy.wallet_balances WHERE newapi_user_id IN(910001,910002))=$3 FROM poker.tables WHERE table_id=$1`, table, hand.HandID, 6000*engine.UnitsPerChip).Scan(&lifecycle, &closedFlags, &settled))
			if lifecycle != "CLOSED" || !closedFlags || !settled {
				t.Error("accepted Close did not finish unique funded closure:", lifecycle, "closing_flags", closedFlags, "funds_closed", settled)
			}
			s.Close()
			readonlyConfig := pool.Config()
			readonlyConfig.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
			readonly, err := pgxpool.NewWithConfig(ctx, readonlyConfig)
			must(err)
			defer readonly.Close()
			var readOnly string
			must(readonly.QueryRow(ctx, "SHOW transaction_read_only").Scan(&readOnly))
			var bound bool
			must(readonly.QueryRow(ctx, "SELECT current_database()=$1 AND current_user=$2", pool.Config().ConnConfig.Database, pool.Config().ConnConfig.User).Scan(&bound))
			if readOnly != "on" || !bound {
				t.Fatal("receipt restart did not use same isolated readonly runtime")
			}
			opts.Pool = readonly
			s, err = legacyTestNew(opts)
			must(err)
			before = funds() + digest("poker.tables", "poker.hands", "poker.hand_fairness", "poker.request_receipts")
			for range 2 {
				got, err := s.LookupReceipt(ctx, 910001, ReceiptQuery{table, "host", command.Key})
				if err != nil || got.State != "FOUND" || got.Receipt == nil || *got.Receipt != original {
					t.Error("original CLOSING receipt changed after settlement/restart:", err)
				}
			}
			if len(s.actors) != 0 || before != funds()+digest("poker.tables", "poker.hands", "poker.hand_fairness", "poker.request_receipts") {
				t.Fatal("readonly receipt lookup changed funded facts or started an actor")
			}
			if !t.Failed() {
				t.Log("C0 real PG: actual restart order and original 30s grace; receipt-first replay/conflict, new Host denial, same actual Hand/normal engine fold, unique Settlement/two Cash Outs, wallet closure and readonly original receipt; synthetic auth/control only")
			}
		})
	}
}
