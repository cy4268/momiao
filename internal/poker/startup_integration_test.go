package poker

import (
	"context"
	"testing"
	"time"
)

func TestControlReadyPlayersAutomaticallyStart(t *testing.T) {
	f := newControlFixture(t)
	table := f.table(t)
	f.buy(t, table, 910001, 1)
	f.buy(t, table, 910002, 2)
	connected(t, f.service, fixtureRef(table, 910001), true)
	connected(t, f.service, fixtureRef(table, 910002), true)
	deadline := time.Now().Add(2500 * time.Millisecond)
	var hands int
	for time.Now().Before(deadline) {
		if e := f.owner.QueryRow(context.Background(), "SELECT count(*) FROM poker.hands WHERE table_id=$1", table).Scan(&hands); e != nil {
			t.Fatal(e)
		}
		if hands == 1 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("ready players never started automatically without StartHand: hands=%d", hands)
}

func TestControlReadyNeedsConfirmedOwner(t *testing.T) {
	f := newControlFixture(t)
	table := f.table(t)
	f.buy(t, table, 910001, 1)
	f.buy(t, table, 910002, 2)
	connected(t, f.service, fixtureRef(table, 910001), true)
	f.faults.failAssign.Store(true)
	pending := connected(t, f.service, fixtureRef(table, 910002), true)
	if pending.Control.Mode != "READ_ONLY" {
		t.Fatal("fixture did not produce pending control")
	}
	time.Sleep(1250 * time.Millisecond)
	var hands int
	if e := f.owner.QueryRow(context.Background(), "SELECT count(*) FROM poker.hands WHERE table_id=$1", table).Scan(&hands); e != nil {
		t.Fatal(e)
	}
	if hands != 0 {
		t.Fatal("PG connected flag started hand without confirmed owner", hands)
	}
}

func startRecoveryFixture(t *testing.T) (*controlFixture, string, string) {
	t.Helper()
	f := newControlFixture(t)
	table := f.table(t)
	f.buy(t, table, 910001, 1)
	f.buy(t, table, 910002, 2)
	connected(t, f.service, fixtureRef(table, 910001), true)
	connected(t, f.service, fixtureRef(table, 910002), true)
	// Internal test setup only: production has no public StartHand route.
	hand, e := f.service.StartHand(context.Background(), StartHandCommand{UserID: 910001, Key: "setup-recovery-hand", TableID: table})
	if e != nil {
		t.Fatal(e)
	}
	f.service.Close()
	return f, table, hand.HandID
}
func TestControlStartupHydratesBeforeMutation(t *testing.T) {
	f, table, hand := startRecoveryFixture(t)
	ctx := context.Background()
	s, e := New(f.options())
	if e != nil {
		t.Fatal(e)
	}
	f.service = s
	report, e := s.Start(ctx)
	if e != nil || report.Loaded != 1 || report.Recovering != 1 {
		t.Fatal("startup did not hydrate its durable hand", report, e)
	}
	var state string
	var grace, timeNow time.Time
	if e = f.owner.QueryRow(ctx, "SELECT state,grace_until,clock_timestamp() FROM poker.recovery_state WHERE table_id=$1", table).Scan(&state, &grace, &timeNow); e != nil {
		t.Fatal(e)
	}
	if state != "GRACE" || grace.Sub(timeNow) < 28*time.Second || grace.Sub(timeNow) > 30*time.Second {
		t.Fatal("fresh recovery grace not authoritative", state, grace.Sub(timeNow))
	}
	var count int
	if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM poker.actions WHERE hand_id=$1 AND event_type IN('TIMEOUT','AUTO_FOLD','AUTO_CHECK')", hand).Scan(&count); e != nil || count != 0 {
		t.Fatal("old deadline charged before recovery", count, e)
	}
	again, e := s.Start(ctx)
	if e != nil || again.Loaded != 1 {
		t.Fatal("same-instance startup not idempotent", again, e)
	}
	// Actual elapsed PG grace and actual internal 10-second Redis renewals; no
	// ping/timer in the transport and no owner timestamp rewrites in this test.
	refs := []ControlRef{connected(t, s, fixtureRef(table, 910001), true).Ref, connected(t, s, fixtureRef(table, 910002), true).Ref}
	start := time.Now()
	var resumed TableView
	for time.Since(start) < 34*time.Second {
		resumed, e = s.ViewForConnection(ctx, refs[0])
		if e != nil {
			t.Fatal(e)
		}
		if resumed.Hand != nil && !resumed.Hand.Recovering {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if resumed.Hand == nil || resumed.Hand.Recovering || resumed.Hand.ActionDeadlineAt == nil || resumed.ServerNow.Before(grace) || resumed.Hand.ActionDeadlineAt.Sub(resumed.ServerNow) < 28*time.Second {
		t.Fatal("grace did not grant a fresh action clock", resumed.Hand)
	}
	if wait := 31*time.Second - time.Since(start); wait > 0 {
		time.Sleep(wait)
	}
	for _, ref := range refs {
		if e = s.AuthorizeControl(ctx, ref); e != nil {
			t.Fatal("live owner expired despite internal renew", e)
		}
	}
	if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM poker.actions WHERE hand_id=$1 AND event_type IN('TIMEOUT','AUTO_FOLD','AUTO_CHECK')", hand).Scan(&count); e != nil || count != 0 {
		t.Fatal("recovery charged old timeout", count, e)
	}
}
func TestControlCorruptSnapshotIsDurablyIsolated(t *testing.T) {
	f, table, hand := startRecoveryFixture(t)
	ctx := context.Background()
	var stacks, ledgerBefore int64
	if e := f.owner.QueryRow(ctx, "SELECT sum(current_stack_units) FROM poker.sessions").Scan(&stacks); e != nil {
		t.Fatal(e)
	}
	if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM economy.wallet_ledger").Scan(&ledgerBefore); e != nil {
		t.Fatal(e)
	}
	if _, e := f.owner.Exec(ctx, "UPDATE poker.hands SET snapshot_cipher=decode('00','hex') WHERE hand_id=$1", hand); e != nil {
		t.Fatal(e)
	}
	s, e := New(f.options())
	if e != nil {
		t.Fatal(e)
	}
	f.service = s
	report, e := s.Start(ctx)
	if e != nil || report.Loaded != 1 || report.NeedsReview != 1 {
		t.Fatal("corrupt hand was not durably isolated", report, e)
	}
	var status string
	var stacksAfter, ledgerAfter, hands int64
	if e = f.owner.QueryRow(ctx, "SELECT state FROM poker.recovery_state WHERE table_id=$1", table).Scan(&status); e != nil {
		t.Fatal(e)
	}
	if e = f.owner.QueryRow(ctx, "SELECT sum(current_stack_units) FROM poker.sessions").Scan(&stacksAfter); e != nil {
		t.Fatal(e)
	}
	if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM economy.wallet_ledger").Scan(&ledgerAfter); e != nil {
		t.Fatal(e)
	}
	if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM poker.hands WHERE table_id=$1", table).Scan(&hands); e != nil {
		t.Fatal(e)
	}
	if status != "NEEDS_REVIEW" || stacksAfter != stacks || ledgerAfter != ledgerBefore || hands != 1 {
		t.Fatal("isolation changed funds or reshuffled", status, stacksAfter, ledgerAfter, hands)
	}
	var session string
	if e = f.owner.QueryRow(ctx, "SELECT session_id::text FROM poker.hand_participants WHERE hand_id=$1 AND seat_no=1", hand).Scan(&session); e != nil {
		t.Fatal(e)
	}
	if _, e = s.RequestSafeLeave(ctx, SessionCommand{UserID: 910001, Key: "corrupt-no-cashout", TableID: table, TargetSessionID: session}); e == nil {
		t.Fatal("bad snapshot allowed financial processing")
	}
}
