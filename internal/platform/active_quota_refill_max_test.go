package platform

import (
	"context"
	"testing"
)

func TestActiveQuotaRefillFullMAXBoundary(t *testing.T) {
	const (
		user   int64 = 893008
		low    int64 = 10_000_000
		target int64 = 50_000_000
		maxRaw int64 = 500_000_000
	)
	ctx := context.Background()
	s := integrationStore(t)
	mustEnsure(t, s, user)
	mustApply(t, s, mutation(user, "refill-max-reserve", maxRaw-low))

	// Explicit isolated Native component: no real account or Native runtime is used.
	native := &exchangeNative{quota: low}
	assets, err := NewUnifiedAssetReader(s, native)
	if err != nil {
		t.Fatal(err)
	}
	policy := ActiveQuotaRefillPolicy{
		Enabled:         true,
		LowWatermark:    low,
		TargetWatermark: target,
		MaxActiveBuffer: maxRaw,
	}
	assertAssets := func(stage string, active, reserve, inFlight int64) {
		t.Helper()
		got, readErr := assets.ReadUnifiedAssets(ctx, user)
		if readErr != nil {
			t.Fatalf("%s unified assets: %v", stage, readErr)
		}
		if got.ActiveQuotaUnits != active || got.ReserveUnits != reserve ||
			got.AvailableChipsUnits != 0 || got.PokerStackUnits != 0 || got.PokerPotUnits != 0 ||
			got.QuotaTransferInFlightUnits != inFlight || got.TotalUnits != maxRaw {
			t.Fatalf("%s assets = %+v; want active=%d reserve=%d in_flight=%d total=%d", stage, got, active, reserve, inFlight, maxRaw)
		}
	}
	countEffects := func() int {
		native.mu.Lock()
		defer native.mu.Unlock()
		return len(native.effects)
	}
	countSourceDebits := func() int {
		t.Helper()
		var count int
		if queryErr := s.pool.QueryRow(ctx, `SELECT count(*) FROM economy.wallet_ledger
			WHERE newapi_user_id=$1 AND entry_type='RESERVE_TO_ACTIVE_DEBIT'`, user).Scan(&count); queryErr != nil {
			t.Fatal(queryErr)
		}
		return count
	}

	assertAssets("before", low, maxRaw-low, 0)
	request := ActiveQuotaRefillRequest{
		RequestID:        "refill-max-boundary-original",
		UserID:           user,
		RequiredRawQuota: maxRaw,
	}
	plan, err := s.PlanActiveQuotaRefill(ctx, native, request, policy)
	if err != nil || plan.Transfer == nil {
		t.Fatalf("plan exact MAX refill: plan=%+v err=%v", plan, err)
	}
	if plan.ObservedActiveRawQuota != low || plan.Transfer.AmountUnits != maxRaw-low || plan.Transfer.Status != "PENDING" {
		t.Fatalf("exact MAX plan = %+v", plan)
	}
	if countSourceDebits() != 1 || countEffects() != 0 {
		t.Fatalf("planning effects: source_debits=%d native_effects=%d", countSourceDebits(), countEffects())
	}
	assertAssets("planned", low, 0, maxRaw-low)

	confirmed, err := s.ProcessQuotaTransferByID(ctx, native, plan.Transfer.ID, user)
	if err != nil || confirmed.Status != "CONFIRMED" || confirmed.NativeBefore == nil || confirmed.NativeAfter == nil {
		t.Fatalf("confirm exact MAX refill: transfer=%+v err=%v", confirmed, err)
	}
	if *confirmed.NativeBefore != low || *confirmed.NativeAfter != maxRaw || confirmed.AmountUnits != maxRaw-low {
		t.Fatalf("exact MAX Native receipt = %+v", confirmed)
	}
	if countSourceDebits() != 1 || countEffects() != 1 {
		t.Fatalf("confirmation effects: source_debits=%d native_effects=%d", countSourceDebits(), countEffects())
	}
	assertAssets("confirmed", maxRaw, 0, 0)

	replay, err := s.PlanActiveQuotaRefill(ctx, native, request, policy)
	if err != nil || replay.Transfer == nil || replay.Transfer.ID != confirmed.ID || replay.Transfer.Status != "CONFIRMED" {
		t.Fatalf("original request replay: plan=%+v err=%v", replay, err)
	}
	replayedTransfer, err := s.ProcessQuotaTransferByID(ctx, native, replay.Transfer.ID, user)
	if err != nil || replayedTransfer.ID != confirmed.ID || replayedTransfer.UserID != confirmed.UserID ||
		replayedTransfer.AmountUnits != confirmed.AmountUnits || replayedTransfer.Status != confirmed.Status ||
		replayedTransfer.NativeBefore == nil || replayedTransfer.NativeAfter == nil ||
		*replayedTransfer.NativeBefore != *confirmed.NativeBefore || *replayedTransfer.NativeAfter != *confirmed.NativeAfter {
		t.Fatalf("confirmed transfer replay: transfer=%+v err=%v", replayedTransfer, err)
	}
	if countSourceDebits() != 1 || countEffects() != 1 {
		t.Fatalf("replay duplicated an effect: source_debits=%d native_effects=%d", countSourceDebits(), countEffects())
	}
	assertAssets("replayed", maxRaw, 0, 0)

	overMAX, err := s.PlanActiveQuotaRefill(ctx, native, ActiveQuotaRefillRequest{
		RequestID:        "refill-over-max-new-request",
		UserID:           user,
		RequiredRawQuota: maxRaw + 1,
	}, policy)
	if err != nil || overMAX.Transfer != nil || overMAX.Result != ActiveQuotaNotAvailable || overMAX.ObservedActiveRawQuota != maxRaw {
		t.Fatalf("new over-MAX request: plan=%+v err=%v", overMAX, err)
	}
	if countSourceDebits() != 1 || countEffects() != 1 {
		t.Fatalf("over-MAX request created an effect: source_debits=%d native_effects=%d", countSourceDebits(), countEffects())
	}
	assertAssets("over MAX rejected", maxRaw, 0, 0)
}
