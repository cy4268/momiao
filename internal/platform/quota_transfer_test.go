package platform

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestQuotaTransferIntegration(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	user := int64(870001)
	mustEnsure(t, s, user)
	key, _ := uuidV7()
	if _, err := s.ClaimDaily(ctx, user, key); err != nil {
		t.Fatal(err)
	}
	key, _ = uuidV7()
	tr, err := s.CreateQuotaTransfer(ctx, user, key, 50000000)
	if err != nil || tr.Status != "PENDING" || tr.ID == "" {
		t.Fatalf("valid transfer was not durably accepted: %+v %v", tr, err)
	}
	before, _ := s.ReadWallet(ctx, user, ReserveAPICredit)
	if before.BalanceUnits != 200000000 {
		t.Fatal(before)
	}
	replay, err := s.CreateQuotaTransfer(ctx, user, key, 50000000)
	if err != nil || replay.ID != tr.ID {
		t.Fatal(replay, err)
	}
	if _, err = s.CreateQuotaTransfer(ctx, user, key, 1); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatal(err)
	}
	another, _ := uuidV7()
	if _, err = s.CreateQuotaTransfer(ctx, user, another, 1); !errors.Is(err, ErrTransferPending) {
		t.Fatal(err)
	}
	if v, err := s.QuotaTransferByKey(ctx, user+1, key); err != nil || v != nil {
		t.Fatal("owner", v, err)
	}
	// Synthetic native schema in the disposable test DB. Production uses a separate native database.
	_, err = s.pool.Exec(ctx, `CREATE TABLE public.users(id BIGINT PRIMARY KEY,quota BIGINT,status BIGINT,deleted_at TIMESTAMPTZ);`+NativeQuotaMigration)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO public.users VALUES($1,0,1,NULL)`, user)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.pool.Exec(ctx, `UPDATE momiao_quota.settings SET enabled=true`)
	if err != nil {
		t.Fatal(err)
	}
	// Production's native quota adapter owns a pool distinct from the wallet
	// store. Keep that boundary in this single-database fixture too: sharing the
	// four-connection test pool lets concurrent outer wallet transactions consume
	// every connection before the winning worker can query the native receipt.
	nativePool, err := pgxpool.NewWithConfig(ctx, s.pool.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nativePool.Close)
	native := &NativeQuota{pool: nativePool}
	// Target committed, but local completion is lost. Restarted worker queries the original receipt.
	receipt, err := native.Credit(ctx, tr.ID, user, tr.AmountUnits)
	if err != nil || receipt.Result != "APPLIED" {
		t.Fatal(receipt, err)
	}
	_, err = s.pool.Exec(ctx, `CREATE FUNCTION economy.fail_transfer_finish() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected finish failure'; END $$; CREATE TRIGGER fail_transfer_finish BEFORE UPDATE ON economy.quota_transfers FOR EACH STATEMENT EXECUTE FUNCTION economy.fail_transfer_finish()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ProcessQuotaTransfer(ctx, native); err == nil {
		t.Fatal("injected failure was ignored")
	}
	_, err = s.pool.Exec(ctx, `DROP TRIGGER fail_transfer_finish ON economy.quota_transfers`)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := s.ProcessQuotaTransfer(ctx, native); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	found, err := s.QuotaTransferByKey(ctx, user, key)
	if err != nil || found.Status != "CONFIRMED" {
		t.Fatal(found, err)
	}
	raw, err := native.ReadNativeQuota(ctx, user)
	if err != nil || raw.RawQuota != 50000000 {
		t.Fatal(raw, err)
	}
	after, _ := s.ReadWallet(ctx, user, ReserveAPICredit)
	if after != before {
		t.Fatal("duplicate source debit", before, after)
	}
	if _, err = native.Credit(ctx, tr.ID, user, 1); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatal(err)
	}
	// A definitive target rejection refunds exactly once, never guesses that an outage means rejection.
	_, err = s.pool.Exec(ctx, `UPDATE public.users SET status=2 WHERE id=$1`, user)
	if err != nil {
		t.Fatal(err)
	}
	key2, _ := uuidV7()
	refund, err := s.CreateQuotaTransfer(ctx, user, key2, 10000000)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ProcessQuotaTransfer(ctx, native)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.QuotaTransferByKey(ctx, user, key2)
	if got.Status != "REFUNDED" || got.Reason != "ACCOUNT_RESTRICTED" {
		t.Fatal(got)
	}
	if _, err = native.Credit(ctx, refund.ID, user, refund.AmountUnits); err != nil {
		t.Fatal(err)
	}
	after, _ = s.ReadWallet(ctx, user, ReserveAPICredit)
	if after.BalanceUnits != before.BalanceUnits {
		t.Fatal(after)
	}
	_, err = s.pool.Exec(ctx, `UPDATE public.users SET status=1 WHERE id=$1`, user)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.pool.Exec(ctx, `UPDATE momiao_quota.settings SET enabled=false`)
	if err != nil {
		t.Fatal(err)
	}
	key3, _ := uuidV7()
	_, err = s.CreateQuotaTransfer(ctx, user, key3, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ProcessQuotaTransfer(ctx, native); err == nil {
		t.Fatal("disabled bridge credited")
	}
	got, _ = s.QuotaTransferByKey(ctx, user, key3)
	if got.Status != "PENDING" {
		t.Fatal(got)
	}
	_, err = s.pool.Exec(ctx, `UPDATE momiao_quota.settings SET enabled=true`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ProcessQuotaTransfer(ctx, native); err != nil {
		t.Fatal(err)
	}
	raw, _ = native.ReadNativeQuota(ctx, user)
	if raw.RawQuota != 50000001 {
		t.Fatal(raw)
	}
	// Quota row updates serialize with direct native consumption, preserving exact additions/deductions.
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			id, _ := uuidV7()
			if _, e := native.Credit(ctx, id, user, 1); e != nil {
				t.Error(e)
			}
		}()
		go func() {
			defer wg.Done()
			if _, e := s.pool.Exec(ctx, `UPDATE public.users SET quota=quota-1 WHERE id=$1 AND quota>=1`, user); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	raw, _ = native.ReadNativeQuota(ctx, user)
	if raw.RawQuota != 50000001 {
		t.Fatal(raw)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE momiao_quota.operations SET amount_units=amount_units+1`); err == nil {
		t.Fatal("journal mutable")
	}
	if _, err = s.pool.Exec(ctx, `DELETE FROM economy.quota_transfers`); err == nil {
		t.Fatal("transfers deletable")
	}
	// A technical target overflow is a durable rejection, not a partial credit.
	before, _ = s.ReadWallet(ctx, user, ReserveAPICredit)
	if _, err = s.pool.Exec(ctx, `UPDATE public.users SET quota=9007199254740991 WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
	key4, _ := uuidV7()
	if _, err = s.CreateQuotaTransfer(ctx, user, key4, 1); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ProcessQuotaTransfer(ctx, native); err != nil {
		t.Fatal(err)
	}
	got, err = s.QuotaTransferByKey(ctx, user, key4)
	if err != nil || got.Status != "REFUNDED" || got.Reason != "BALANCE_OVERFLOW" {
		t.Fatal(got, err)
	}
	after, _ = s.ReadWallet(ctx, user, ReserveAPICredit)
	if after.BalanceUnits != before.BalanceUnits {
		t.Fatal("overflow refund", after)
	}
	key5, _ := uuidV7()
	if _, err = s.CreateQuotaTransfer(ctx, user, key5, before.BalanceUnits+1); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatal(err)
	}
}

func TestQuotaTransferOutOfRangeRecovery(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	const oversized int64 = 2_750_500_000
	for _, applied := range []bool{false, true} {
		user := int64(870002)
		if applied {
			user++
		}
		mustEnsure(t, s, user)
		id, _ := uuidV7()
		mustApply(t, s, mutation(user, "legacy-large-reserve-"+id, oversized+50_000_000))
		key, _ := uuidV7()
		hash := sha256.Sum256([]byte(key))
		// Reproduce a durable request accepted by the old JS-safe amount guard.
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = applyInTx(ctx, tx, Mutation{UserID: user, Asset: ReserveAPICredit, DeltaUnits: -oversized, BizType: "NATIVE_QUOTA_TRANSFER", BizID: id + ":debit", EntryType: "RESERVE_TO_ACTIVE_DEBIT", IdempotencyKey: "quota:" + id + ":debit"})
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO economy.quota_transfers(transfer_id,newapi_user_id,request_key_hash,amount_units,status) VALUES($1,$2,$3,$4,'PENDING')`, id, user, hash[:], oversized)
		}
		if err != nil {
			rollback(tx)
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		native := &exchangeNative{}
		if applied {
			before, after := int64(0), oversized
			native.effects = map[string]NativeQuotaReceipt{id: {ID: id, UserID: user, DeltaRawQuota: oversized, Before: &before, After: &after, Result: "APPLIED"}}
		} else {
			// An ambiguous or foreign receipt must neither refund nor release the request.
			for _, forged := range []bool{false, true} {
				native.unknown, native.forged = !forged, forged
				if _, err = s.ProcessQuotaTransfer(ctx, native); err == nil {
					t.Fatal("unreliable legacy outcome was settled")
				}
				pending, _ := s.QuotaTransferByKey(ctx, user, key)
				wallet, _ := s.ReadWallet(ctx, user, ReserveAPICredit)
				if pending.Status != "PENDING" || wallet.BalanceUnits != 50_000_000 || len(native.effects) != 0 {
					t.Fatal("unknown outcome changed source or destination", pending, wallet)
				}
			}
			native.unknown, native.forged = false, false
		}
		if worked, err := s.ProcessQuotaTransfer(ctx, native); err != nil || !worked {
			t.Fatal("legacy queue head did not settle", worked, err)
		}
		got, err := s.QuotaTransferByKey(ctx, user, key)
		wantStatus, wantRefund := "REFUNDED", 1
		if applied {
			wantStatus, wantRefund = "CONFIRMED", 0
		}
		if err != nil || got.Status != wantStatus {
			t.Fatalf("legacy result=%+v err=%v want=%s", got, err, wantStatus)
		}
		if !applied && (got.Reason != "AMOUNT_OUT_OF_RANGE" || len(native.effects) != 0) {
			t.Fatal("oversized request was applied instead of refunded", got, native.effects)
		}
		replay, err := s.CreateQuotaTransfer(ctx, user, key, oversized)
		if err != nil || replay.ID != id || replay.Status != wantStatus {
			t.Fatal("old key replay was blocked or replaced", replay, err)
		}
		if worked, err := s.ProcessQuotaTransfer(ctx, native); err != nil || worked {
			t.Fatal("settled request was retried", worked, err)
		}
		var refunds int
		if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM economy.wallet_ledger WHERE newapi_user_id=$1 AND biz_id=$2`, user, id+":refund").Scan(&refunds); err != nil || refunds != wantRefund {
			t.Fatal("legacy refund must occur exactly once, only if not applied", refunds, err)
		}
		if applied {
			continue
		}
		newKey, _ := uuidV7()
		if _, err = s.CreateQuotaTransfer(ctx, user, newKey, 1<<31); !errors.Is(err, ErrInvalidMutation) {
			t.Fatal("new out-of-range transfer accepted", err)
		}
		wallet, _ := s.ReadWallet(ctx, user, ReserveAPICredit)
		if wallet.BalanceUnits != oversized+50_000_000 {
			t.Fatal("rejection or duplicate recovery changed Reserve", wallet)
		}
		manual, err := s.CreateQuotaTransfer(ctx, user, newKey, 50_000_000)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.ProcessQuotaTransfer(ctx, native); err != nil {
			t.Fatal("valid transfer behind old queue head did not progress", err)
		}
		manual, err = s.ProcessQuotaTransferByID(ctx, native, manual.ID, user)
		if err != nil || manual.Status != "CONFIRMED" || len(native.effects) != 1 {
			t.Fatal("manual settlement or replay", manual, err)
		}
		// Simulated consumption drops Active below LOW; the existing refill path must resume.
		native.quota = 0
		service, err := NewActiveQuotaRefillService(s, native, ActiveQuotaRefillPolicy{Enabled: true, LowWatermark: 10_000_000, TargetWatermark: 50_000_000, MaxActiveBuffer: 500_000_000})
		if err != nil {
			t.Fatal(err)
		}
		result, err := service.Refill(ctx, ActiveQuotaRefillRequest{UserID: user, RequestID: "after-legacy-refund-refill", RequiredRawQuota: 1})
		if err != nil || result.Result != ActiveQuotaRefilled || native.quota != 50_000_000 || len(native.effects) != 2 {
			t.Fatal("automatic refill stayed blocked after recovery", result, err)
		}
	}
}
