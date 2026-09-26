package platform

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/historyaccess"
)

// The Native ledger is deterministic and isolated from every real Native account.
type exchangeNative struct {
	mu        sync.Mutex
	quota     int64
	effects   map[string]NativeQuotaReceipt
	ambiguous bool
	unknown   bool
	forged    bool
}

func (n *exchangeNative) ReadNativeQuota(_ context.Context, user int64) (NativeQuotaSnapshot, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return NativeQuotaSnapshot{UserID: user, RawQuota: n.quota, Enabled: true, Result: "APPLIED", ObservedAt: time.Now().UTC()}, nil
}
func (n *exchangeNative) QueryQuotaOperation(_ context.Context, id string, user int64) (NativeQuotaReceipt, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.unknown {
		return NativeQuotaReceipt{ID: id, UserID: user, Result: "UNKNOWN"}, nil
	}
	if n.forged {
		return NativeQuotaReceipt{ID: id, UserID: user + 1, Result: "NOT_APPLIED"}, nil
	}
	if r, ok := n.effects[id]; ok {
		return r, nil
	}
	return NativeQuotaReceipt{ID: id, UserID: user, Result: "NOT_APPLIED"}, nil
}
func (n *exchangeNative) ApplyRawQuotaDelta(_ context.Context, id string, user, delta int64) (NativeQuotaReceipt, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.unknown {
		return NativeQuotaReceipt{ID: id, UserID: user, DeltaRawQuota: delta, Result: "UNKNOWN"}, nil
	}
	if n.forged {
		return NativeQuotaReceipt{ID: id, UserID: user + 1, DeltaRawQuota: delta, Result: "APPLIED"}, nil
	}
	if r, ok := n.effects[id]; ok {
		if r.UserID != user || r.DeltaRawQuota != delta {
			return NativeQuotaReceipt{}, ErrIdempotencyConflict
		}
		return r, nil
	}
	before := n.quota
	after := before + delta
	r := NativeQuotaReceipt{ID: id, UserID: user, DeltaRawQuota: delta, Before: &before, After: &after, Result: "APPLIED"}
	if after < 0 {
		r.Result = "INSUFFICIENT"
		unchanged := before
		r.After = &unchanged
	} else {
		n.quota = after
	}
	if n.effects == nil {
		n.effects = map[string]NativeQuotaReceipt{}
	}
	n.effects[id] = r
	if n.ambiguous {
		n.ambiguous = false
		return NativeQuotaReceipt{}, ErrNativeQuotaDependency
	}
	return r, nil
}
func (n *exchangeNative) Credit(ctx context.Context, id string, user, amount int64) (NativeQuotaReceipt, error) {
	return n.ApplyRawQuotaDelta(ctx, id, user, amount)
}

func TestAPIChipsMixedAcceptance(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	user := int64(893001)
	mustEnsure(t, s, user)
	mustApply(t, s, mutation(user, "mixed-reserve", 70))
	n := &exchangeNative{quota: 80}
	assets, _ := NewUnifiedAssetReader(s, n)
	service, _ := NewEconomyService(s, assets)
	key, _ := uuidV7()
	receipt, err := service.Exchange(ctx, user, key, ReserveAPICredit, 100)
	if err != nil || receipt.Status != "PENDING" {
		t.Fatalf("mixed exchange must durably accept Reserve=70 Active=80 request=100: receipt=%+v err=%v", receipt, err)
	}
	if *receipt.ReserveDebitUnits != 70 || *receipt.ActiveDebitUnits != 30 || *receipt.ChipsCreditUnits != 100 {
		t.Fatal(receipt)
	}
	assertExchangeAssets(t, assets, user, 150, 70)
	detail, err := s.HistoryTransaction(ctx, historyaccess.Own(user), receipt.ID)
	if err != nil || detail.Status != "PENDING" || len(detail.Effects) != 1 || !detail.ConfirmedAt.IsZero() {
		t.Fatal("pending whole history", detail, err)
	}
	processExchange(t, s, n)
	assertExchangeAssets(t, assets, user, 150, 100)
	processExchange(t, s, n)
	assertExchangeAssets(t, assets, user, 150, 0)
	assertExchangeWallets(t, s, n, user, 0, 50, 100)
	detail, err = s.HistoryTransaction(ctx, historyaccess.Own(user), receipt.ID)
	if err != nil || detail.Status != "CONFIRMED" || len(detail.Effects) != 2 || len(detail.NativeEffects) != 1 || detail.NativeEffects[0].DeltaUnits != "-30" {
		t.Fatal("confirmed whole history", detail, err)
	}
	if _, err = s.HistoryTransaction(ctx, historyaccess.Own(user+1), receipt.ID); !errors.Is(err, historyaccess.ErrNotFound) {
		t.Fatal("history owner leak", err)
	}
	if len(n.effects) != 1 {
		t.Fatal("native debit duplicated", len(n.effects))
	}
	got, err := s.FindOperation(ctx, user, "EXCHANGE", key)
	if err != nil || got == nil || got.Status != "CONFIRMED" || got.ID != receipt.ID {
		t.Fatal(got, err)
	}
	history, err := s.Transactions(ctx, user, "")
	if err != nil || len(history) != 1 || history[0].ID != receipt.ID || history[0].Kind != "API_CHIPS_EXCHANGE" {
		t.Fatal(history, err)
	}
	if other, err := s.FindOperation(ctx, user+1, "EXCHANGE", key); err != nil || other != nil {
		t.Fatal("foreign owner receipt", other, err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE economy.api_chips_exchanges SET active_debit_units=active_debit_units+1 WHERE exchange_id=$1`, receipt.ID); err == nil {
		t.Fatal("plan mutable")
	}
}

func exchangeFixture(t *testing.T, user, reserve, active int64) (*Store, *EconomyService, *exchangeNative) {
	t.Helper()
	s := integrationStore(t)
	mustEnsure(t, s, user)
	if reserve > 0 {
		mustApply(t, s, mutation(user, fmt.Sprintf("fixture-reserve:%d", user), reserve))
	}
	n := &exchangeNative{quota: active}
	assets, _ := NewUnifiedAssetReader(s, n)
	service, _ := NewEconomyService(s, assets)
	return s, service, n
}
func processExchange(t *testing.T, s *Store, n *exchangeNative) {
	t.Helper()
	if worked, err := s.ProcessAPIChipsExchange(context.Background(), n); err != nil || !worked {
		t.Fatal(worked, err)
	}
}
func assertExchangeWallets(t *testing.T, s *Store, n *exchangeNative, user, reserve, active, chips int64) {
	t.Helper()
	r, err := s.ReadWallet(context.Background(), user, ReserveAPICredit)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.ReadWallet(context.Background(), user, AvailableChips)
	if err != nil {
		t.Fatal(err)
	}
	if r.BalanceUnits != reserve || n.quota != active || c.BalanceUnits != chips {
		t.Fatalf("balances = Reserve %d Active %d Chips %d; want %d %d %d", r.BalanceUnits, n.quota, c.BalanceUnits, reserve, active, chips)
	}
}
func assertExchangeAssets(t *testing.T, a *UnifiedAssetReader, user, total, inflight int64) {
	t.Helper()
	got, err := a.ReadUnifiedAssets(context.Background(), user)
	if err != nil || got.TotalUnits != total || got.QuotaTransferInFlightUnits != inflight {
		t.Fatalf("assets %+v err=%v want total=%d inflight=%d", got, err, total, inflight)
	}
}

func TestAPIChipsAllActiveAndLocalPaths(t *testing.T) {
	ctx := context.Background()
	s, service, n := exchangeFixture(t, 893002, 0, 150)
	key, _ := uuidV7()
	r, err := service.Exchange(ctx, 893002, key, ReserveAPICredit, 100)
	if err != nil || r.Status != "PENDING" {
		t.Fatal(r, err)
	}
	assertExchangeAssets(t, service.assets, 893002, 150, 0)
	processExchange(t, s, n)
	assertExchangeAssets(t, service.assets, 893002, 150, 100)
	processExchange(t, s, n)
	assertExchangeWallets(t, s, n, 893002, 0, 50, 100)
	var count int
	err = s.pool.QueryRow(ctx, `SELECT count(*) FROM economy.wallet_ledger WHERE newapi_user_id=$1 AND asset_type='RESERVE_API_CREDIT'`, 893002).Scan(&count)
	if err != nil || count != 0 {
		t.Fatal("zero reserve leg", count, err)
	}
	key2, _ := uuidV7()
	r, err = service.Exchange(ctx, 893002, key2, AvailableChips, 60)
	if err != nil || r.Kind != "LOCAL_EXCHANGE" || r.Status != "CONFIRMED" {
		t.Fatal(r, err)
	}
	key3, _ := uuidV7()
	r, err = service.Exchange(ctx, 893002, key3, ReserveAPICredit, 40)
	if err != nil || r.Kind != "LOCAL_EXCHANGE" {
		t.Fatal(r, err)
	}
	assertExchangeWallets(t, s, n, 893002, 20, 50, 80)
	if len(n.effects) != 1 {
		t.Fatal("local path called Native", n.effects)
	}
}
func TestAPIChipsInsufficientAndIdentity(t *testing.T) {
	ctx := context.Background()
	s, service, n := exchangeFixture(t, 893003, 70, 80)
	key, _ := uuidV7()
	r, err := service.Exchange(ctx, 893003, key, ReserveAPICredit, 100)
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.Exchange(ctx, 893003, key, ReserveAPICredit, 100)
	if err != nil || again.ID != r.ID {
		t.Fatal(again, err)
	}
	for _, change := range []struct {
		from   Asset
		amount int64
	}{{ReserveAPICredit, 101}, {AvailableChips, 100}} {
		if _, err = service.Exchange(ctx, 893003, key, change.from, change.amount); !errors.Is(err, ErrIdempotencyConflict) {
			t.Fatal(err)
		}
	}
	n.quota = 20 // independently consumed after acceptance; the plan must not change.
	processExchange(t, s, n)
	assertExchangeWallets(t, s, n, 893003, 70, 20, 0)
	assertExchangeAssets(t, service.assets, 893003, 90, 0)
	got, err := s.FindOperation(ctx, 893003, "EXCHANGE", key)
	if err != nil || got.Status != "COMPENSATED" || got.Reason != "INSUFFICIENT" || *got.ReserveDebitUnits != 70 || *got.ActiveDebitUnits != 30 {
		t.Fatal(got, err)
	}
	again, err = service.Exchange(ctx, 893003, key, ReserveAPICredit, 100)
	if err != nil || again.Status != "COMPENSATED" {
		t.Fatal(again, err)
	}
	key2, _ := uuidV7()
	if _, err = service.Exchange(ctx, 893003, key2, ReserveAPICredit, 100); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatal(err)
	}
	assertExchangeWallets(t, s, n, 893003, 70, 20, 0)
}
func TestAPIChipsAmbiguousRecoveryAndRefillFence(t *testing.T) {
	ctx := context.Background()
	s, service, n := exchangeFixture(t, 893004, 70, 80)
	key, _ := uuidV7()
	r, err := service.Exchange(ctx, 893004, key, ReserveAPICredit, 100)
	if err != nil {
		t.Fatal(err)
	}
	key2, _ := uuidV7()
	if _, err = s.CreateQuotaTransfer(ctx, 893004, key2, 1); !errors.Is(err, ErrTransferPending) {
		t.Fatal("old transfer not fenced", err)
	}
	if _, err = service.Exchange(ctx, 893004, key2, ReserveAPICredit, 1); !errors.Is(err, ErrTransferPending) {
		t.Fatal("second mixed exchange not fenced", err)
	}
	refill, err := s.PlanActiveQuotaRefill(ctx, n, ActiveQuotaRefillRequest{RequestID: "exchange-refill", UserID: 893004, RequiredRawQuota: 90}, ActiveQuotaRefillPolicy{Enabled: true, LowWatermark: 80, TargetWatermark: 100, MaxActiveBuffer: 100})
	if err != nil || refill.Transfer != nil || refill.Result != ActiveQuotaNotAvailable {
		t.Fatal(refill, err)
	}
	n.ambiguous = true
	if _, err = s.ProcessAPIChipsExchange(ctx, n); err == nil {
		t.Fatal("lost Native response was treated as definite")
	}
	assertExchangeWallets(t, s, n, 893004, 0, 50, 0)
	assertExchangeAssets(t, service.assets, 893004, 150, 100)
	n.unknown = true
	if assets, err := service.assets.ReadUnifiedAssets(ctx, 893004); err == nil || assets.TotalUnits != 0 {
		t.Fatal("unknown Native effect certified a total", assets, err)
	}
	n.unknown = false
	n.forged = true
	if _, err := service.assets.ReadUnifiedAssets(ctx, 893004); err == nil {
		t.Fatal("foreign operation receipt certified assets")
	}
	n.forged = false
	got, err := s.FindOperation(ctx, 893004, "EXCHANGE", key)
	if err != nil || got.ID != r.ID || got.Status != "PENDING" {
		t.Fatal(got, err)
	}
	processExchange(t, s, n)
	processExchange(t, s, n)
	assertExchangeWallets(t, s, n, 893004, 0, 50, 100)
	if len(n.effects) != 1 {
		t.Fatal(n.effects)
	}
}
func TestAPIChipsTargetCommitRecovery(t *testing.T) {
	ctx := context.Background()
	s, service, n := exchangeFixture(t, 893005, 70, 80)
	key, _ := uuidV7()
	r, err := service.Exchange(ctx, 893005, key, ReserveAPICredit, 100)
	if err != nil {
		t.Fatal(err)
	}
	processExchange(t, s, n)
	_, err = s.pool.Exec(ctx, `CREATE FUNCTION economy.test_api_chips_abort() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.newapi_user_id=893005 AND NEW.status='CONFIRMED' THEN RAISE EXCEPTION 'injected target finish failure'; END IF; RETURN NEW; END$$;CREATE TRIGGER test_api_chips_abort BEFORE UPDATE ON economy.api_chips_exchanges FOR EACH ROW EXECUTE FUNCTION economy.test_api_chips_abort()`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(ctx, `DROP TRIGGER IF EXISTS test_api_chips_abort ON economy.api_chips_exchanges;DROP FUNCTION IF EXISTS economy.test_api_chips_abort()`)
	})
	if _, err = s.ProcessAPIChipsExchange(ctx, n); err == nil {
		t.Fatal("injected Platform failure ignored")
	}
	assertExchangeWallets(t, s, n, 893005, 0, 50, 0)
	assertExchangeAssets(t, service.assets, 893005, 150, 100)
	if _, err = s.pool.Exec(ctx, `DROP TRIGGER test_api_chips_abort ON economy.api_chips_exchanges`); err != nil {
		t.Fatal(err)
	}
	processExchange(t, s, n)
	assertExchangeWallets(t, s, n, 893005, 0, 50, 100)
	if len(n.effects) != 1 {
		t.Fatal("second native debit")
	}
	got, _ := s.FindOperation(ctx, 893005, "EXCHANGE", key)
	if got.ID != r.ID || got.Status != "CONFIRMED" {
		t.Fatal(got)
	}
}
func TestAPIChipsTargetCompensation(t *testing.T) {
	ctx := context.Background()
	s, service, n := exchangeFixture(t, 893006, 70, 80)
	key, _ := uuidV7()
	_, err := service.Exchange(ctx, 893006, key, ReserveAPICredit, 100)
	if err != nil {
		t.Fatal(err)
	}
	processExchange(t, s, n)
	m := mutation(893006, "target-full", math.MaxInt64)
	m.Asset = AvailableChips
	mustApply(t, s, m)
	processExchange(t, s, n)
	p, err := scanAPIChips(s.pool.QueryRow(ctx, `SELECT `+apiChipsColumns+` FROM economy.api_chips_exchanges WHERE newapi_user_id=893006`))
	if err != nil || p.Status != "COMPENSATING" || p.RefundID == nil {
		t.Fatal(p, err)
	}
	if len(n.effects) != 1 {
		t.Fatal("refund happened before refund ID durable")
	}
	refundID := *p.RefundID
	n.ambiguous = true
	if _, err = s.ProcessAPIChipsExchange(ctx, n); err == nil {
		t.Fatal("refund ambiguity ignored")
	}
	// Independent target funds leave, making total representable again. Refund
	// already exists but Platform has not observed it; only Reserve remains in flight.
	m = mutation(893006, "target-unfill", -math.MaxInt64)
	m.Asset = AvailableChips
	mustApply(t, s, m)
	assertExchangeAssets(t, service.assets, 893006, 150, 70)
	processExchange(t, s, n)
	assertExchangeWallets(t, s, n, 893006, 70, 80, 0)
	assertExchangeAssets(t, service.assets, 893006, 150, 0)
	if len(n.effects) != 2 || n.effects[refundID].DeltaRawQuota != 30 {
		t.Fatal("incorrect compensation", n.effects)
	}
}
func TestAPIChipsUnknownFailsClosed(t *testing.T) {
	ctx := context.Background()
	s, service, n := exchangeFixture(t, 893007, 70, 80)
	key, _ := uuidV7()
	_, err := service.Exchange(ctx, 893007, key, ReserveAPICredit, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, forged := range []bool{false, true} {
		n.unknown = !forged
		n.forged = forged
		if _, err = s.ProcessAPIChipsExchange(ctx, n); err == nil {
			t.Fatal("unprovable Native receipt accepted")
		}
		if _, err = service.assets.ReadUnifiedAssets(ctx, 893007); err == nil {
			t.Fatal("unprovable total returned")
		}
		assertExchangeWallets(t, s, n, 893007, 0, 80, 0)
	}
	n.unknown = false
	n.forged = false
	processExchange(t, s, n)
	processExchange(t, s, n)
	assertExchangeWallets(t, s, n, 893007, 0, 50, 100)
}
