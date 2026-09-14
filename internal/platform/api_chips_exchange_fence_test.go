package platform

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
)

// Models Native's two durable authorities: a per-user Redis fence and a SQL
// effect journal. Query never releases a fence. Apply replay must release its
// original fence even when the journal was already committed.
type fencedExchangeNative struct {
	*exchangeNative
	fenceMu            sync.Mutex
	fence              string
	failRelease        bool
	crashBeforeJournal bool
}

func (n *fencedExchangeNative) ReadNativeQuota(ctx context.Context, user int64) (NativeQuotaSnapshot, error) {
	snapshot, err := n.exchangeNative.ReadNativeQuota(ctx, user)
	if err == nil && (snapshot.RawQuota < 0 || snapshot.RawQuota > math.MaxInt32) {
		snapshot.Result = "SOURCE_INCOMPATIBLE"
		snapshot.RawQuota = 0
	}
	return snapshot, err
}

func (n *fencedExchangeNative) QueryQuotaOperation(ctx context.Context, id string, user int64) (NativeQuotaReceipt, error) {
	n.fenceMu.Lock()
	defer n.fenceMu.Unlock()
	r, err := n.exchangeNative.QueryQuotaOperation(ctx, id, user)
	if err == nil && r.Result == "NOT_APPLIED" && n.fence == id {
		r.Result = "UNKNOWN"
	}
	return r, err
}
func (n *fencedExchangeNative) ApplyRawQuotaDelta(ctx context.Context, id string, user, delta int64) (NativeQuotaReceipt, error) {
	n.fenceMu.Lock()
	defer n.fenceMu.Unlock()
	n.mu.Lock()
	existing, exists := n.effects[id]
	n.mu.Unlock()
	if exists {
		if existing.UserID != user || existing.DeltaRawQuota != delta {
			return NativeQuotaReceipt{}, ErrIdempotencyConflict
		}
		if n.fence == id {
			if n.failRelease {
				n.failRelease = false
				return NativeQuotaReceipt{}, ErrNativeQuotaDependency
			}
			n.fence = ""
		}
		return existing, nil
	}
	if n.fence != "" && n.fence != id {
		return NativeQuotaReceipt{}, ErrNativeQuotaDependency
	}
	n.fence = id
	if n.crashBeforeJournal {
		n.crashBeforeJournal = false
		return NativeQuotaReceipt{}, ErrNativeQuotaDependency
	}
	n.mu.Lock()
	before := n.quota
	if !exists && (before < 0 || before > math.MaxInt32 || before+delta > math.MaxInt32) {
		after := before
		existing = NativeQuotaReceipt{ID: id, UserID: user, DeltaRawQuota: delta, Before: &before, After: &after, Result: "SOURCE_INCOMPATIBLE"}
		if n.effects == nil {
			n.effects = map[string]NativeQuotaReceipt{}
		}
		n.effects[id] = existing
		exists = true
	}
	n.mu.Unlock()
	var err error
	if !exists {
		existing, err = n.exchangeNative.ApplyRawQuotaDelta(ctx, id, user, delta)
	} else if existing.UserID != user || existing.DeltaRawQuota != delta {
		err = ErrIdempotencyConflict
	}
	if err != nil {
		return NativeQuotaReceipt{}, err
	}
	if n.failRelease {
		n.failRelease = false
		return NativeQuotaReceipt{}, ErrNativeQuotaDependency
	}
	n.fence = ""
	return existing, nil
}
func (n *fencedExchangeNative) Credit(ctx context.Context, id string, user, amount int64) (NativeQuotaReceipt, error) {
	return n.ApplyRawQuotaDelta(ctx, id, user, amount)
}
func (n *fencedExchangeNative) consume(units int64) error {
	n.fenceMu.Lock()
	defer n.fenceMu.Unlock()
	if n.fence != "" {
		return errors.New("ordinary Native usage remains blocked by quota fence")
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.quota < units {
		return ErrInsufficientBalance
	}
	n.quota -= units
	return nil
}
func fencedExchangeFixture(t *testing.T, user int64) (*Store, *EconomyService, *fencedExchangeNative, string) {
	t.Helper()
	ctx := context.Background()
	s, _, base := exchangeFixture(t, user, 70, 80)
	n := &fencedExchangeNative{exchangeNative: base}
	assets, _ := NewUnifiedAssetReader(s, n)
	service, _ := NewEconomyService(s, assets)
	key, _ := uuidV7()
	receipt, err := service.Exchange(ctx, user, key, ReserveAPICredit, 100)
	if err != nil {
		t.Fatal(err)
	}
	// Restore this disposable fixture on a failed assertion so another case in
	// the same isolated DB is not blocked by the intentionally crashed job.
	t.Cleanup(func() {
		p, e := scanAPIChips(s.pool.QueryRow(ctx, `SELECT `+apiChipsColumns+` FROM economy.api_chips_exchanges WHERE exchange_id=$1`, receipt.ID))
		if e != nil {
			return
		}
		n.failRelease = false
		n.crashBeforeJournal = false
		_, _ = n.ApplyRawQuotaDelta(ctx, p.DebitID, user, -p.Active)
		if p.RefundID != nil {
			_, _ = n.ApplyRawQuotaDelta(ctx, *p.RefundID, user, p.Active)
		}
		for i := 0; i < 3; i++ {
			_, _ = s.ProcessAPIChipsExchange(ctx, n)
		}
		// An intentionally incompatible source may be unrecoverable on the RED
		// implementation. Preserve its fixture history under review, allowing the
		// next independent user case to run; the controller drops the whole DB.
		_, _ = s.pool.Exec(ctx, `UPDATE economy.api_chips_exchanges SET status='NEEDS_REVIEW',reason='TEST_FIXTURE_CLEANUP'
   WHERE exchange_id=$1 AND status IN('PENDING','SOURCE_DEBITED','COMPENSATING')`, receipt.ID)
	})
	return s, service, n, receipt.ID
}
func fencedPlan(t *testing.T, s *Store, id string) apiChipsExchange {
	t.Helper()
	p, err := scanAPIChips(s.pool.QueryRow(context.Background(), `SELECT `+apiChipsColumns+` FROM economy.api_chips_exchanges WHERE exchange_id=$1`, id))
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func fencedProcess(t *testing.T, s *Store, n *fencedExchangeNative) {
	t.Helper()
	worked, err := s.ProcessAPIChipsExchange(context.Background(), n)
	if err != nil || !worked {
		t.Fatalf("same-ID Native recovery: worked=%v error=%v", worked, err)
	}
}

func TestAPIChipsNativeJournalFenceReplay(t *testing.T) {
	ctx := context.Background()
	s, _, n, id := fencedExchangeFixture(t, 893011)
	n.failRelease = true
	if _, err := s.ProcessAPIChipsExchange(ctx, n); err == nil {
		t.Fatal("Native post-commit fence-release fault ignored")
	}
	p := fencedPlan(t, s, id)
	r, err := n.QueryQuotaOperation(ctx, p.DebitID, p.User)
	if err != nil || r.Result != "APPLIED" || n.fence != p.DebitID || p.Status != "PENDING" {
		t.Fatal("expected committed journal with retained fence", r, p.Status, err)
	}
	fencedProcess(t, s, n)
	fencedProcess(t, s, n)
	assertExchangeWallets(t, s, n.exchangeNative, p.User, 0, 50, 100)
	if len(n.effects) != 1 || n.effects[p.DebitID].DeltaRawQuota != -30 {
		t.Fatal("debit effect was replaced or duplicated")
	}
	if err = n.consume(1); err != nil {
		t.Fatal("confirmed Chips with Native account still fenced:", err)
	}
}
func TestAPIChipsNativePreJournalFenceReplay(t *testing.T) {
	ctx := context.Background()
	s, service, n, id := fencedExchangeFixture(t, 893012)
	n.crashBeforeJournal = true
	if _, err := s.ProcessAPIChipsExchange(ctx, n); err == nil {
		t.Fatal("pre-journal crash ignored")
	}
	p := fencedPlan(t, s, id)
	r, err := n.QueryQuotaOperation(ctx, p.DebitID, p.User)
	if err != nil || r.Result != "UNKNOWN" || len(n.effects) != 0 || n.fence != p.DebitID {
		t.Fatal("expected orphaned owned fence", r, err)
	}
	if _, err = service.assets.ReadUnifiedAssets(ctx, p.User); err == nil {
		t.Fatal("UNKNOWN was accepted as a reliable total")
	}
	if n.fence != p.DebitID || len(n.effects) != 0 {
		t.Fatal("read-only total mutated Native fence/journal")
	}
	fencedProcess(t, s, n)
	fencedProcess(t, s, n)
	assertExchangeWallets(t, s, n.exchangeNative, p.User, 0, 50, 100)
	if len(n.effects) != 1 || n.effects[p.DebitID].DeltaRawQuota != -30 {
		t.Fatal("pre-journal recovery used a replacement debit")
	}
	if err = n.consume(1); err != nil {
		t.Fatal(err)
	}
}
func TestAPIChipsNativeRefundFenceReplay(t *testing.T) {
	ctx := context.Background()
	s, _, n, id := fencedExchangeFixture(t, 893013)
	fencedProcess(t, s, n)
	m := mutation(893013, "fence-target-full", math.MaxInt64)
	m.Asset = AvailableChips
	mustApply(t, s, m)
	fencedProcess(t, s, n)
	p := fencedPlan(t, s, id)
	if p.RefundID == nil || p.Status != "COMPENSATING" {
		t.Fatal("refund identity not durable", p)
	}
	n.failRelease = true
	if _, err := s.ProcessAPIChipsExchange(ctx, n); err == nil {
		t.Fatal("refund release fault ignored")
	}
	r, err := n.QueryQuotaOperation(ctx, *p.RefundID, p.User)
	if err != nil || r.Result != "APPLIED" || n.fence != *p.RefundID {
		t.Fatal("missing refund fence fixture", r, err)
	}
	fencedProcess(t, s, n)
	assertExchangeWallets(t, s, n.exchangeNative, p.User, 70, 80, math.MaxInt64)
	if len(n.effects) != 2 || n.effects[*p.RefundID].DeltaRawQuota != 30 || fencedPlan(t, s, id).Status != "COMPENSATED" {
		t.Fatal("incorrect refund recovery")
	}
	if err = n.consume(1); err != nil {
		t.Fatal("compensated receipt retained Native refund fence:", err)
	}
}
func TestAPIChipsNativeOutOfRangeRejectionRefund(t *testing.T) {
	for index, quota := range []int64{-1, math.MaxInt32 + 1} {
		t.Run(fmt.Sprint(quota), func(t *testing.T) {
			ctx := context.Background()
			user := int64(893014 + index)
			s, service, n, id := fencedExchangeFixture(t, user)
			n.quota = quota // Native became source-incompatible after the accepted snapshot.
			fencedProcess(t, s, n)
			p := fencedPlan(t, s, id)
			if p.Status != "COMPENSATED" || p.Reason != "SOURCE_INCOMPATIBLE" {
				t.Fatal("definitive zero-effect rejection did not restore Reserve", p)
			}
			assertExchangeWallets(t, s, n.exchangeNative, user, 70, quota, 0)
			if len(n.effects) != 1 || n.effects[p.DebitID].Result != "SOURCE_INCOMPATIBLE" || p.RefundID != nil {
				t.Fatal("rejection created a Native refund effect")
			}
			if _, err := service.assets.ReadUnifiedAssets(ctx, user); err == nil {
				t.Fatal("invalid Native quota produced a reliable total")
			}
		})
	}
}
