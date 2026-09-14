package poker

import (
	"context"
	"errors"
	"testing"

	"github.com/cy4268/momiao/internal/poker/engine"
)

// No controller connection is made: HTTP account-authenticated funding targets
// one explicit durable Poker Session, not whichever seat the user occupies now.
func TestFundingTargetSessionRolloverAndOriginalReceipts(t *testing.T) {
	f := newControlFixture(t)
	ctx := context.Background()
	table := f.table(t)
	old := f.buy(t, table, 910001, 1)
	top := TopUpCommand{UserID: 910001, TableID: table, TargetSessionID: old, Key: "target-session-top-original", AmountUnits: 10 * engine.UnitsPerChip}
	topped, err := f.service.RequestTopUp(ctx, top)
	if err != nil || topped.Status != "CONFIRMED" {
		t.Fatal(topped, err)
	}
	leave := SessionCommand{UserID: 910001, TableID: table, TargetSessionID: old, Key: "target-session-leave-original"}
	left, err := f.service.RequestSafeLeave(ctx, leave)
	if err != nil || left.Status != "CONFIRMED" {
		t.Fatal(left, err)
	}
	assertOriginal := func(got, want Receipt, e error) {
		t.Helper()
		if e != nil || !got.Duplicate || got.SessionID != old || got.Version != want.Version || got.Status != want.Status || got.FundingID != want.FundingID {
			t.Fatal("original target receipt lost", got, e)
		}
	}
	r, e := f.service.RequestTopUp(ctx, top)
	assertOriginal(r, topped, e)
	r, e = f.service.RequestSafeLeave(ctx, leave)
	assertOriginal(r, left, e)
	next := f.buy(t, table, 910001, 1)
	if next == old {
		t.Fatal("actual session did not roll over")
	}
	readEffects := func() [5]int64 {
		t.Helper()
		var v [5]int64
		if e := f.owner.QueryRow(ctx, `SELECT (SELECT balance_units FROM economy.wallet_balances WHERE newapi_user_id=910001 AND asset_type='AVAILABLE_CHIPS'),(SELECT current_stack_units FROM poker.sessions WHERE session_id=$1),(SELECT count(*) FROM economy.wallet_ledger),(SELECT count(*) FROM poker.funding_operations),(SELECT count(*) FROM poker.request_receipts)`, next).Scan(&v[0], &v[1], &v[2], &v[3], &v[4]); e != nil {
			t.Fatal(e)
		}
		return v
	}
	before := readEffects()
	staleTop := top
	staleTop.Key = "target-session-stale-new-top"
	r, e = f.service.RequestTopUp(ctx, staleTop)
	if !errors.Is(e, ErrDenied) {
		t.Fatalf("old target with new key affected current Session: returned_session=%s expected_old_target=%s err=%v effects_before=%v effects_after=%v", r.SessionID, old, e, before, readEffects())
	}
	staleLeave := leave
	staleLeave.Key = "target-session-stale-new-leave"
	if _, e = f.service.RequestSafeLeave(ctx, staleLeave); !errors.Is(e, ErrDenied) {
		t.Fatal("old target leave accepted", e)
	}
	newTop := top
	newTop.TargetSessionID = next
	if _, e = f.service.RequestTopUp(ctx, newTop); !errors.Is(e, ErrConflict) {
		t.Fatal("top-up ID crossed Session target", e)
	}
	newLeave := leave
	newLeave.TargetSessionID = next
	if _, e = f.service.RequestSafeLeave(ctx, newLeave); !errors.Is(e, ErrConflict) {
		t.Fatal("leave ID crossed Session target", e)
	}
	missingTop := top
	missingTop.TargetSessionID = ""
	missingTop.Key = "target-session-missing-top"
	if _, e = f.service.RequestTopUp(ctx, missingTop); !errors.Is(e, ErrInvalid) {
		t.Fatal("empty target had implicit fallback", e)
	}
	missingLeave := leave
	missingLeave.TargetSessionID = ""
	missingLeave.Key = "target-session-missing-leave"
	if _, e = f.service.RequestSafeLeave(ctx, missingLeave); !errors.Is(e, ErrInvalid) {
		t.Fatal("empty leave target had implicit fallback", e)
	}
	noncanonical := top
	noncanonical.TargetSessionID = "AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA"
	noncanonical.Key = "target-session-canonical-top"
	if _, e = f.service.RequestTopUp(ctx, noncanonical); !errors.Is(e, ErrInvalid) {
		t.Fatal("noncanonical target accepted", e)
	}
	r, e = f.service.RequestTopUp(ctx, top)
	assertOriginal(r, topped, e)
	r, e = f.service.RequestSafeLeave(ctx, leave)
	assertOriginal(r, left, e)
	if after := readEffects(); after != before {
		t.Fatal("rejected/duplicate requests changed money or receipts", before, after)
	}
	t.Log("explicit target guards and unchanged original receipts survive actual cashout/rebuy; zero new wallet/stack/ledger/funding/receipt effects")
}
