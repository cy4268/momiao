package games

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

func TestSlotDurableAtomicSettlement(t *testing.T) {
	owner, runtime := gameTestStores(t)
	ctx := context.Background()
	user := time.Now().UnixMicro()
	if err := owner.EnsureAccount(ctx, user); err != nil {
		t.Fatal(err)
	}
	const starting = int64(5000000000)
	if _, err := owner.Apply(ctx, platform.Mutation{UserID: user, Asset: platform.AvailableChips, DeltaUnits: starting, BizType: "TEST_G1_SLOT", BizID: requestKey(t), EntryType: "TEST_GRANT", IdempotencyKey: requestKey(t)}); err != nil {
		t.Fatal(err)
	}
	s := newTestService(t, runtime)
	b, err := s.Bootstrap(ctx, user, "slot")
	if err != nil || b.Next == nil {
		t.Fatal(b, err)
	}
	if b.Game.State != "PLAY" || b.Game.Config.Validation == nil {
		t.Fatal("missing verified bound artifact", b.Game)
	}
	key := requestKey(t)
	input := CreateInput{Type: "SLOT", TotalWager: "11"}
	var wg sync.WaitGroup
	rounds := make(chan GameRound, 8)
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() { r, e := s.Create(ctx, user, "slot", key, b.Next.ID, input); rounds <- r; errs <- e })
	}
	wg.Wait()
	close(rounds)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var result GameRound
	for r := range rounds {
		if result.ID != "" && result.ID != r.ID {
			t.Fatal("duplicated round")
		}
		result = r
	}
	if result.Slot == nil || result.Slot.LineStakeUnits != 550000 || result.Slot.TotalPayoutUnits != result.PayoutUnits {
		t.Fatal(result)
	}
	if result.StakeUnits != 5500000 || result.BalanceAfterUnits != starting+result.NetUnits {
		t.Fatal("bad units", result)
	}
	restored, err := newTestService(t, runtime).Read(ctx, user, result.ID)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(result)
	z, _ := json.Marshal(restored)
	if string(a) != string(z) {
		t.Fatalf("restored typed result differs\n%s\n%s", a, z)
	}
	verified, err := s.Verify(ctx, user, result.ID)
	if err != nil || !verified.Verified {
		t.Fatal(verified, err)
	}
	if _, err = s.Read(ctx, user+1, result.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("ownership", err)
	}
	err = owner.WithTx(ctx, func(tx pgx.Tx) error {
		var lines, debits int
		var payout int64
		if e := tx.QueryRow(ctx, `SELECT count(*),sum(line_payout_units) FROM games.slot_line_results WHERE round_id=$1`, result.ID).Scan(&lines, &payout); e != nil {
			return e
		}
		if e := tx.QueryRow(ctx, `SELECT count(*) FROM economy.wallet_ledger WHERE transaction_id=$1`, result.WagerTransactionID).Scan(&debits); e != nil {
			return e
		}
		if lines != 10 || payout != result.PayoutUnits || debits != 1 {
			t.Fatal(lines, payout, debits)
		}
		var canUpdate bool
		if e := tx.QueryRow(ctx, `SELECT has_table_privilege($1,'games.slot_results','UPDATE')`, runtimeRole(t)).Scan(&canUpdate); e != nil {
			return e
		}
		if canUpdate {
			t.Fatal("runtime can rewrite slot outcome")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Slot round %s: 8 duplicate creates, stake=%d payout=%d, ten immutable lines, restart and deterministic verify passed", result.ID, result.StakeUnits, result.PayoutUnits)
	capOwner, capRuntime := gameTestStores(t, true)
	capUser := int64(823003)
	if e := capOwner.EnsureAccount(ctx, capUser); e != nil {
		t.Fatal(e)
	}
	if e := capOwner.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `INSERT INTO public.users(id) VALUES($1)`, capUser)
		return e
	}); e != nil {
		t.Fatal(e)
	}
	if _, e := capOwner.Apply(ctx, platform.Mutation{UserID: capUser, Asset: platform.AvailableChips, DeltaUnits: 2000000000, BizType: "TEST_CAP", BizID: requestKey(t), EntryType: "TEST_GRANT", IdempotencyKey: requestKey(t)}); e != nil {
		t.Fatal(e)
	}
	capSvc := newTestService(t, capRuntime)
	clipped := false
	for range 64 {
		chips, e := capOwner.ReadWallet(ctx, capUser, platform.AvailableChips)
		if e != nil {
			t.Fatal(e)
		}
		reserve, e := capOwner.ReadWallet(ctx, capUser, platform.ReserveAPICredit)
		if e != nil {
			t.Fatal(e)
		}
		fill := platform.AssetCapUnits - chips.BalanceUnits - reserve.BalanceUnits
		if fill > 0 {
			if _, e = capOwner.Apply(ctx, platform.Mutation{UserID: capUser, Asset: platform.ReserveAPICredit, DeltaUnits: fill, BizType: "TEST_CAP", BizID: requestKey(t), EntryType: "TEST_GRANT", IdempotencyKey: requestKey(t)}); e != nil {
				t.Fatal(e)
			}
		}
		boot, e := capSvc.Bootstrap(ctx, capUser, "slot")
		if e != nil {
			t.Fatal(e)
		}
		k := requestKey(t)
		input := CreateInput{Type: "SLOT", TotalWager: "11"}
		capped, e := capSvc.Create(ctx, capUser, "slot", k, boot.Next.ID, input)
		if e != nil {
			t.Fatal(e)
		}
		c := capped.EconomySettlement
		if c == nil || c.GrossPayoutUnits != capped.PayoutUnits || c.CreditedPayoutUnits != min(capped.StakeUnits, capped.PayoutUnits) || capped.BalanceAfterUnits != chips.BalanceUnits+c.ActualNetUnits {
			t.Fatalf("actual vs raw payout: %+v", capped)
		}
		retry, e := capSvc.Create(ctx, capUser, "slot", k, boot.Next.ID, input)
		if e != nil || retry.EconomySettlement == nil || *retry.EconomySettlement != *c {
			t.Fatal("changed cap replay", e)
		}
		proof, e := capSvc.Verify(ctx, capUser, capped.ID)
		if e != nil || !proof.Verified {
			t.Fatal("raw proof changed", e)
		}
		if c.WithheldUnits > 0 {
			clipped = true
			break
		}
	}
	if !clipped {
		t.Fatal("no positive win exercised in 64 rounds")
	}

}
