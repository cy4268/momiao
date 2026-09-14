package poker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker/engine"
)

func playableFixture(t *testing.T) (*controlFixture, string, map[int]ControlRef) {
	t.Helper()
	f := newControlFixture(t)
	table := f.table(t)
	f.buy(t, table, 910001, 1)
	f.buy(t, table, 910002, 2)
	refs := map[int]ControlRef{1: connected(t, f.service, fixtureRef(table, 910001), true).Ref, 2: connected(t, f.service, fixtureRef(table, 910002), true).Ref}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		v, e := f.service.View(context.Background(), table, 910001)
		if e != nil {
			t.Fatal(e)
		}
		if v.Hand != nil && v.Hand.Street == "PREFLOP" {
			return f, table, refs
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("automatic first hand not ready")
	return nil, "", nil
}

func foldFromSocket(t *testing.T, f *controlFixture, table string, refs map[int]ControlRef) Receipt {
	t.Helper()
	ctx := context.Background()
	v, err := f.service.View(ctx, table, 910001)
	if err != nil || v.Hand == nil {
		t.Fatal("hand unavailable", err)
	}
	ref := refs[v.Hand.ActorSeat]
	v, err = f.service.ViewForConnection(ctx, ref)
	if err != nil || !v.Viewer.CanAct || v.Viewer.Legal == nil || v.Viewer.Control == nil || v.Viewer.Control.Mode != "CONTROLLER" {
		t.Fatal("controlling actor has no legal projection", v.Viewer, err)
	}
	tv, _ := strconv.ParseUint(v.TableVersion, 10, 64)
	hv, _ := strconv.ParseUint(v.Hand.HandVersion, 10, 64)
	r, err := f.service.Act(ctx, ActCommand{UserID: ref.UserID, Key: "socket-fold-" + uuid(), TableID: table, HandID: v.Hand.HandID, ControlEpoch: ref.ControlEpoch, HandVersion: hv, Kind: engine.Fold, Control: &ref, TableVersion: tv})
	if err != nil {
		t.Fatal("controlled fold failed", err)
	}
	return r
}

func TestControlSocketProjectionActionAndCashClosure(t *testing.T) {
	f, table, refs := playableFixture(t)
	ctx := context.Background()
	controller, err := f.service.ViewForConnection(ctx, refs[1])
	if err != nil {
		t.Fatal(err)
	}
	observerIdentity := fixtureRef(table, 910001)
	observer := connected(t, f.service, observerIdentity, false)
	readonly, err := f.service.ViewForConnection(ctx, observerIdentity)
	if err != nil || readonly.Viewer.Control == nil || readonly.Viewer.Control.Mode != "READ_ONLY" || readonly.Viewer.CanAct || readonly.Viewer.Legal != nil {
		t.Fatal("readonly projection", err)
	}
	if f.service.AuthorizeControl(ctx, observer.Ref) == nil {
		t.Fatal("observer gained cached owner")
	}
	for _, v := range []TableView{controller, readonly} {
		for _, seat := range v.Seats {
			if seat.IsSelf && len(seat.HoleCards) != 2 {
				t.Fatal("self holes absent")
			}
			if !seat.IsSelf && (len(seat.HoleCards) != 0 || len(seat.PublicHoleCards) != 0) {
				t.Fatal("private opponent holes leaked")
			}
		}
		b, _ := json.Marshal(v)
		for _, secret := range []string{"session_id_hash", "security_epoch", "runtime_epoch", "server_seed\"", "snapshot_cipher", "\"deck\""} {
			if strings.Contains(string(b), secret) {
				t.Fatal("private authority in viewer DTO", secret)
			}
		}
	}
	active, err := f.service.ActiveSession(ctx, 910001)
	if err != nil || active == nil || active.PokerInPlayUnits != decimal(400*engine.UnitsPerChip) || active.CommittedUnits == "0" {
		t.Fatal("active funds summary", active, err)
	}
	if _, err = f.service.Session(ctx, 910002, active.SessionID); err == nil {
		t.Fatal("other user read session")
	}
	if out := os.Getenv("POKER_TEST_CONTROL_EVIDENCE_DIR"); out != "" {
		if err = os.MkdirAll(out, 0700); err != nil {
			t.Fatal(err)
		}
		acting, e := f.service.ViewForConnection(ctx, refs[controller.Hand.ActorSeat])
		if e != nil || !acting.Viewer.CanAct || acting.Viewer.Legal == nil {
			t.Fatal("acting export", e)
		}
		for name, value := range map[string]any{"socket-controller.json": controller, "socket-acting.json": acting, "socket-readonly.json": readonly, "active-session.json": active} {
			b, e := json.MarshalIndent(value, "", "  ")
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(out, name), append(b, '\n'), 0600); e != nil {
				t.Fatal(e)
			}
		}
	}
	firstHand := controller.Hand.HandID
	foldFromSocket(t, f, table, refs)
	var settled, nextReady time.Time
	if err = f.owner.QueryRow(ctx, `SELECT h.settled_at,t.intermission_until FROM poker.tables t JOIN poker.hands h ON h.hand_id=t.current_hand_id WHERE t.table_id=$1`, table).Scan(&settled, &nextReady); err != nil {
		t.Fatal(err)
	}
	if nextReady.Sub(settled) < 5*time.Second {
		t.Fatal("intermission shorter than five seconds")
	}
	deadline := time.Now().Add(7 * time.Second)
	var second string
	for time.Now().Before(deadline) {
		v, e := f.service.View(ctx, table, 910001)
		if e != nil {
			t.Fatal(e)
		}
		if v.Hand != nil && v.Hand.HandID != firstHand && v.Hand.Street == "PREFLOP" {
			second = v.Hand.HandID
			if v.ServerNow.Before(nextReady) {
				t.Fatal("early automatic second hand")
			}
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if second == "" {
		t.Fatal("second hand did not start automatically")
	}
	leave, err := f.service.RequestSafeLeave(ctx, SessionCommand{UserID: 910001, Key: "control-safe-leave-01", TableID: table, TargetSessionID: refs[1].SessionID})
	if err != nil || leave.Status != "LEAVE_AFTER_HAND" {
		t.Fatal("safe leave did not defer", leave, err)
	}
	foldFromSocket(t, f, table, refs)
	if _, err = f.service.RequestSafeLeave(ctx, SessionCommand{UserID: 910002, Key: "control-safe-leave-02", TableID: table, TargetSessionID: refs[2].SessionID}); err != nil {
		t.Fatal(err)
	}
	var wallets, stacks, ledger, settlements int64
	if err = f.owner.QueryRow(ctx, `SELECT sum(balance_units) FROM economy.wallet_balances WHERE newapi_user_id IN(910001,910002) AND asset_type='AVAILABLE_CHIPS'`).Scan(&wallets); err != nil {
		t.Fatal(err)
	}
	if err = f.owner.QueryRow(ctx, `SELECT coalesce(sum(current_stack_units),0) FROM poker.sessions WHERE state<>'SETTLED'`).Scan(&stacks); err != nil {
		t.Fatal(err)
	}
	if err = f.owner.QueryRow(ctx, `SELECT count(*) FROM economy.wallet_ledger`).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if err = f.owner.QueryRow(ctx, `SELECT count(*) FROM poker.settlements`).Scan(&settlements); err != nil {
		t.Fatal(err)
	}
	if wallets != 6000*engine.UnitsPerChip || stacks != 0 || ledger != 4 || settlements != 2 {
		t.Fatal("financial closure", wallets, stacks, ledger, settlements)
	}
	if remaining, e := f.service.ActiveSession(ctx, 910001); e != nil || remaining != nil {
		t.Fatal("cashout still active", remaining, e)
	}
	ended, err := f.service.Session(ctx, 910001, active.SessionID)
	if err != nil || ended.State != "SETTLED" || ended.FinalCashOutUnits == nil || ended.RealizedPLUnits == nil || ended.PokerInPlayUnits != "0" {
		t.Fatal("settled session summary", ended, err)
	}
}
func TestControlHTTPViewDoesNotClaimSocketAuthority(t *testing.T) {
	f, table, refs := playableFixture(t)
	v, e := f.service.View(context.Background(), table, 910001)
	if e != nil {
		t.Fatal(e)
	}
	actor := refs[v.Hand.ActorSeat]
	v, e = f.service.View(context.Background(), table, actor.UserID)
	if e != nil {
		t.Fatal(e)
	}
	if v.Viewer.CanAct || v.Viewer.Legal != nil || v.Viewer.Control != nil || v.Viewer.CanStart {
		t.Fatal("HTTP/account projection impersonates a controlling socket")
	}
}
