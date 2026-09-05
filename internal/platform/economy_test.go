package platform

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestShanghaiDay(t *testing.T) {
	for _, tc := range []struct{ at, day string }{{"2026-09-05T15:59:59Z", "2026-09-05"}, {"2026-09-05T16:00:00Z", "2026-09-06"}} {
		at, _ := time.Parse(time.RFC3339, tc.at)
		day, next := shanghaiDay(at)
		if day != tc.day || !next.After(at) {
			t.Fatal(day, next)
		}
	}
}

func TestEconomyIntegration(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	u := int64(830001)
	mustEnsure(t, s, u)
	before, _ := s.ReadWallet(ctx, u, ReserveAPICredit)
	d, err := s.ReadDaily(ctx, u)
	if err != nil || d.Claimed {
		t.Fatal(d, err)
	}
	// Different browser keys still produce one business effect for a Shanghai day.
	var wg sync.WaitGroup
	ids := make(chan string, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key, _ := uuidV7()
			x, e := s.ClaimDaily(ctx, u, key)
			if e != nil {
				t.Error(e)
			}
			ids <- x.ID
		}()
	}
	wg.Wait()
	close(ids)
	id := ""
	for got := range ids {
		if id == "" {
			id = got
		}
		if got != id {
			t.Fatal("duplicate daily claim")
		}
	}
	after, _ := s.ReadWallet(ctx, u, ReserveAPICredit)
	if after.BalanceUnits-before.BalanceUnits != 250000000 || after.LedgerSeq != before.LedgerSeq+1 {
		t.Fatal(after)
	}
	key, _ := uuidV7()
	x, err := s.Exchange(ctx, u, key, ReserveAPICredit, 125000000)
	if err != nil || x.FromAsset != ReserveAPICredit || x.ToAsset != AvailableChips {
		t.Fatal(x, err)
	}
	replay, err := s.Exchange(ctx, u, key, ReserveAPICredit, 125000000)
	if err != nil || replay.ID != x.ID {
		t.Fatal(replay, err)
	}
	if _, err = s.Exchange(ctx, u, key, ReserveAPICredit, 1); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatal(err)
	}
	found, err := s.FindOperation(ctx, u, "EXCHANGE", key)
	if err != nil || found == nil || found.ID != x.ID {
		t.Fatal(found, err)
	}
	other, err := s.FindOperation(ctx, u+1, "EXCHANGE", key)
	if err != nil || other != nil {
		t.Fatal("ownership", other, err)
	}
	key2, _ := uuidV7()
	if _, err = s.Exchange(ctx, u, key2, AvailableChips, 125000001); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatal(err)
	}
	// A failure after the debit was written must roll the entire two-leg transaction back.
	_, err = s.pool.Exec(ctx, `CREATE FUNCTION economy.test_exchange_abort() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.newapi_user_id=830001 AND NEW.leg_no=2 THEN RAISE EXCEPTION 'test'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_exchange_abort BEFORE INSERT ON economy.wallet_ledger FOR EACH ROW EXECUTE FUNCTION economy.test_exchange_abort()`)
	if err != nil {
		t.Fatal(err)
	}
	key3, _ := uuidV7()
	_, err = s.Exchange(ctx, u, key3, AvailableChips, 1)
	if err == nil {
		t.Fatal("missing injected failure")
	}
	_, err = s.pool.Exec(ctx, `DROP TRIGGER test_exchange_abort ON economy.wallet_ledger; DROP FUNCTION economy.test_exchange_abort()`)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := s.ReadWallet(ctx, u, ReserveAPICredit)
	b, _ := s.ReadWallet(ctx, u, AvailableChips)
	if a.BalanceUnits != 125000000 || b.BalanceUnits != 125000000 {
		t.Fatal(a, b)
	}
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			k, _ := uuidV7()
			from := ReserveAPICredit
			if i%2 == 0 {
				from = AvailableChips
			}
			if _, e := s.Exchange(ctx, u, k, from, 1); e != nil {
				t.Error(e)
			}
		}(i)
	}
	wg.Wait()
	a, _ = s.ReadWallet(ctx, u, ReserveAPICredit)
	b, _ = s.ReadWallet(ctx, u, AvailableChips)
	if a.BalanceUnits+b.BalanceUnits != 250000000 {
		t.Fatal("assets not conserved")
	}
	list, err := s.Transactions(ctx, u, "")
	if err != nil || len(list) != 14 {
		t.Fatal(len(list), err)
	}
	if rows, err := s.Transactions(ctx, u+1, ""); err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	var count int
	if err = s.pool.QueryRow(ctx, "SELECT count(*) FROM rewards.daily_checkins WHERE newapi_user_id=$1", u).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if _, err = s.pool.Exec(ctx, "UPDATE rewards.daily_checkins SET amount_units=amount_units WHERE false"); err == nil {
		t.Fatal("daily history is mutable")
	}
	// Replaying a prior-day request keeps its original receipt across midnight.
	v := u + 10
	mustEnsure(t, s, v)
	oldKey, _ := uuidV7()
	m := Mutation{UserID: v, Asset: ReserveAPICredit, DeltaUnits: DailyAmount, BizType: "DAILY_REWARD_V1", BizID: "daily:830011:2020-01-01", EntryType: "DAILY_REWARD", IdempotencyKey: oldKey}
	old := mustApply(t, s, m)
	if _, err = s.pool.Exec(ctx, `INSERT INTO rewards.daily_checkins VALUES($1,'2020-01-01',1,250000000,'RESERVE_API_CREDIT',$2,now())`, v, old.TransactionID); err != nil {
		t.Fatal(err)
	}
	continued, err := s.ClaimDaily(ctx, v, oldKey)
	if err != nil || continued.ID != old.TransactionID {
		t.Fatal("midnight retry changed claim", continued, err)
	}
	current, err := s.ReadDaily(ctx, v)
	if err != nil || current.Claimed {
		t.Fatal(current, err)
	}
}
