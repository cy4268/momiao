package games

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

func gameTestStores(t *testing.T, capped ...bool) (*platform.Store, *platform.Store) {
	t.Helper()
	file := os.Getenv("MOMIAO_GAMES_TEST_CONNECTION_FILE")
	if file == "" {
		t.Skip("explicit isolated local game test connection required")
	}
	raw, err := os.ReadFile(file)
	var connection struct{ OwnerURL, RuntimeURL, RuntimeRole string }
	if err != nil || json.Unmarshal(raw, &connection) != nil {
		t.Fatal("cannot read isolated game connection")
	}
	for _, url := range []string{connection.OwnerURL, connection.RuntimeURL} {
		c, e := pgx.ParseConfig(url)
		if e != nil || c.Host != "127.0.0.1" || c.Port != 55432 || !strings.HasPrefix(c.Database, "momiao_test_g1_original_") {
			t.Fatal("refusing non-G1 loopback database")
		}
	}
	ctx := context.Background()
	owner, err := platform.Open(ctx, connection.OwnerURL)
	if err != nil {
		t.Fatal("isolated owner connection failed")
	}
	t.Cleanup(owner.Close)
	if os.Getenv("MOMIAO_GAMES_TEST_ISOLATED_GAP") == "1" {
		err = isolatedPendingMigrations(ctx, owner)
	} else {
		err = owner.Migrate(ctx)
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(capped) > 0 && capped[0] {
		err = owner.WithTx(ctx, func(tx pgx.Tx) error {
			var installed bool
			if e := tx.QueryRow(ctx, "SELECT to_regnamespace('momiao_quota') IS NOT NULL").Scan(&installed); e != nil {
				return e
			}
			if !installed {
				if _, e := tx.Exec(ctx, `CREATE TABLE public.users(id bigint PRIMARY KEY,quota bigint NOT NULL DEFAULT 0,status int NOT NULL DEFAULT 1,deleted_at timestamptz);`+platform.NativeQuotaMigration, pgx.QueryExecModeSimpleProtocol); e != nil {
					return e
				}
			}
			_, e := tx.Exec(ctx, `UPDATE momiao_quota.settings SET enabled=true; UPDATE economy.policy_runtime SET active_version='economy-cap-v1' WHERE singleton`, pgx.QueryExecModeSimpleProtocol)
			return e
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = owner.WithTx(context.Background(), func(tx pgx.Tx) error {
				_, e := tx.Exec(context.Background(), `UPDATE economy.policy_runtime SET active_version=NULL WHERE singleton`)
				return e
			})
		})
	}
	grantPaths := []string{"../platform/testdata/runtime-baseline-0001-0004.sql", "../../deploy/sql/runtime-grants-0010-games.psql"}
	if os.Getenv("MOMIAO_GAMES_TEST_ISOLATED_GAP") == "1" {
		grantPaths = append(grantPaths, "../../deploy/sql/runtime-grants-0015-slot.psql")
		if _, e := os.Stat("../../deploy/sql/runtime-grants-0016-blackjack.psql"); e == nil {
			grantPaths = append(grantPaths, "../../deploy/sql/runtime-grants-0016-blackjack.psql")
		}
	} else {
		// The assembled candidate migrates both games through the normal registry.
		grantPaths = append(grantPaths, "../../deploy/sql/runtime-grants-0015-slot.psql", "../../deploy/sql/runtime-grants-0016-blackjack.psql")
	}
	for _, path := range grantPaths {
		sql, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		err = owner.WithTx(ctx, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, strings.ReplaceAll(string(sql), `:"runtime_role"`, pgx.Identifier{connection.RuntimeRole}.Sanitize()), pgx.QueryExecModeSimpleProtocol)
			return e
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if os.Getenv("MOMIAO_GAMES_TEST_ISOLATED_GAP") != "1" {
		// New-round writes use shared maintenance guards and lock the active config row.
		role := pgx.Identifier{connection.RuntimeRole}.Sanitize()
		err = owner.WithTx(ctx, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `GRANT USAGE ON SCHEMA ops TO `+role+`; GRANT EXECUTE ON FUNCTION ops.lock_write_scopes(text[]),ops.is_maintenance_scope_active(text) TO `+role+`; GRANT UPDATE(status) ON games.game_config_versions TO `+role, pgx.QueryExecModeSimpleProtocol)
			return e
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	rawCap, err := os.ReadFile("../../deploy/sql/runtime-grants-0041-economy-cap.psql")
	if err != nil {
		t.Fatal(err)
	}
	capLines := []string{}
	for _, line := range strings.Split(string(rawCap), "\n") {
		if strings.HasPrefix(line, "GRANT ") {
			capLines = append(capLines, line)
		}
	}
	if err = owner.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, strings.ReplaceAll(strings.Join(capLines, "\n"), `:"runtime_role"`, pgx.Identifier{connection.RuntimeRole}.Sanitize()), pgx.QueryExecModeSimpleProtocol)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	runtime, err := platform.Open(ctx, connection.RuntimeURL)
	if err != nil {
		t.Fatal("isolated runtime connection failed")
	}
	t.Cleanup(runtime.Close)
	return owner, runtime
}

func newTestService(t *testing.T, store *platform.Store) *Service {
	t.Helper()
	key := [32]byte{}
	for i := range key {
		key[i] = byte(i + 17)
	}
	raw, e := os.ReadFile(os.Getenv("MOMIAO_GAMES_TEST_CONNECTION_FILE"))
	var conn struct{ OwnerURL string }
	if e != nil || json.Unmarshal(raw, &conn) != nil {
		t.Fatal("local connection")
	}
	observer, e := platform.OpenNativeQuota(context.Background(), conn.OwnerURL)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(observer.Close)
	s, err := NewServiceWithEconomy(store, Keyring{Active: "fixture-v1", Keys: map[string][32]byte{"fixture-v1": key}}, observer)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func requestKey(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	b[6] = (b[6] & 15) | 0x70
	b[8] = (b[8] & 63) | 0x80
	s := hex.EncodeToString(b[:])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}

func TestWagerValidationBeforeAnyResult(t *testing.T) {
	for _, tc := range []struct {
		slug            string
		input           CreateInput
		cost, maxPayout int64
	}{
		{"dice", CreateInput{Type: "DICE", Wager: "10", Choice: "SMALL"}, 5000000, 10000000},
		{"scratch", CreateInput{Type: "SCRATCH", Wager: "11"}, 5500000, 550000000},
		{"summon", CreateInput{Type: "SUMMON", BaseWager: "10", Mode: "SINGLE"}, 5000000, 500000000},
		{"summon", CreateInput{Type: "SUMMON", BaseWager: "10", Mode: "TENFOLD"}, 50000000, 5000000000},
	} {
		n, err := normalizeCreate(tc.slug, tc.input)
		if err != nil || n.TotalStake != tc.cost || n.MaxPayout != tc.maxPayout {
			t.Fatal(n, err)
		}
	}
	for _, input := range []CreateInput{
		{Type: "DICE", Wager: "9", Choice: "SMALL"}, {Type: "DICE", Wager: "10.5", Choice: "SMALL"},
		{Type: "DICE", Wager: "-10", Choice: "SMALL"}, {Type: "DICE", Wager: "1e2", Choice: "SMALL"},
		{Type: "DICE", Wager: "10", Choice: "TRIPLE"}, {Type: "DICE", Wager: "10", Choice: ""},
		{Type: "SUMMON", BaseWager: "10", Mode: "TENFOLD"},
		{Type: "DICE", Wager: "18446744073709", Choice: "BIG"},
	} {
		if _, err := normalizeCreate("dice", input); err == nil {
			t.Fatal("invalid wager/type/choice accepted", input)
		}
	}
	for _, mode := range []string{"", "TENFOLD_GUARANTEE", "DOUBLE"} {
		if _, err := normalizeCreate("summon", CreateInput{Type: "SUMMON", BaseWager: "10", Mode: mode}); err == nil {
			t.Fatal("invalid summon mode accepted")
		}
	}
	owner, runtime := gameTestStores(t, true)
	ctx := context.Background()
	user := int64(823001)
	if e := owner.EnsureAccount(ctx, user); e != nil {
		t.Fatal(e)
	}
	if e := owner.WithTx(ctx, func(tx pgx.Tx) error { _, e := tx.Exec(ctx, `INSERT INTO public.users(id) VALUES($1)`, user); return e }); e != nil {
		t.Fatal(e)
	}
	if _, e := owner.Apply(ctx, platform.Mutation{UserID: user, Asset: platform.AvailableChips, DeltaUnits: platform.SinglePlayerMaxUnits * 4, BizType: "TEST_CAP", BizID: requestKey(t), EntryType: "TEST_GRANT", IdempotencyKey: requestKey(t)}); e != nil {
		t.Fatal(e)
	}
	svc := newTestService(t, runtime)
	b, e := svc.Bootstrap(ctx, user, "dice")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Create(ctx, user, "dice", requestKey(t), b.Next.ID, CreateInput{Type: "DICE", Wager: "1000001", Choice: "BIG"}); !errors.Is(e, ErrInvalidInput) {
		t.Fatal("million cap not enforced", e)
	}
	next, e := svc.Bootstrap(ctx, user, "dice")
	if e != nil || next.Next.ID != b.Next.ID {
		t.Fatal("rejected wager consumed randomness", e)
	}
	if _, e = svc.Create(ctx, user, "dice", requestKey(t), b.Next.ID, CreateInput{Type: "DICE", Wager: "1000000", Choice: "BIG"}); e != nil {
		t.Fatal("exact million rejected", e)
	}

}

func TestDiceServiceIntegration(t *testing.T) {
	owner, runtime := gameTestStores(t)
	s := newTestService(t, runtime)
	ctx := context.Background()
	user := time.Now().UnixMicro()
	other := user + 1
	for _, u := range []int64{user, other} {
		if err := owner.EnsureAccount(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	const starting = int64(5000000000)
	if _, err := owner.Apply(ctx, platform.Mutation{UserID: user, Asset: platform.AvailableChips, DeltaUnits: starting, BizType: "TEST_G1", BizID: requestKey(t), EntryType: "TEST_GRANT", IdempotencyKey: requestKey(t)}); err != nil {
		t.Fatal(err)
	}
	b, err := s.Bootstrap(ctx, user, "dice")
	if err != nil {
		t.Fatal(err)
	}
	if b.Next == nil || b.Next.ServerSeedHash == "" || b.Latest != nil || b.AvailableUnits != starting {
		t.Fatal("incomplete bootstrap", b)
	}
	second, err := s.Bootstrap(ctx, user, "dice")
	if err != nil || second.Next.ID != b.Next.ID {
		t.Fatal("bootstrap reminted compatible commitment", err)
	}
	key := requestKey(t)
	input := CreateInput{Type: "DICE", Wager: "10", Choice: "SMALL"}
	var wg sync.WaitGroup
	results := make(chan GameRound, 8)
	failures := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := s.Create(ctx, user, "dice", key, b.Next.ID, input)
			if e != nil {
				failures <- e
			} else {
				results <- r
			}
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	var round GameRound
	count := 0
	for r := range results {
		if count > 0 && r.ID != round.ID {
			t.Fatal("duplicate round")
		}
		round = r
		count++
	}
	if count != 8 || round.State != "SETTLED" || round.Dice == nil || round.StakeUnits != 5000000 || round.BalanceBeforeUnits != starting || round.BalanceAfterUnits != starting+round.NetUnits || round.NetUnits != round.PayoutUnits-round.StakeUnits {
		t.Fatal(round)
	}
	wallet, err := owner.ReadWallet(ctx, user, platform.AvailableChips)
	if err != nil || wallet.BalanceUnits != round.BalanceAfterUnits {
		t.Fatal(wallet, err)
	}
	entries, err := owner.Ledger(ctx, user, platform.AvailableChips, 1, 10)
	wantEntries := 1
	if round.PayoutUnits > 0 {
		wantEntries = 2
	}
	if err != nil || len(entries) != wantEntries {
		t.Fatal("duplicate/missing wager or payout", entries, err)
	}
	if entries[0].DeltaUnits != -round.StakeUnits || entries[0].EntryType != "GAME_WAGER" {
		t.Fatal(entries)
	}
	if round.PayoutUnits > 0 && entries[1].DeltaUnits != round.PayoutUnits {
		t.Fatal(entries)
	}
	changed := input
	changed.Choice = "BIG"
	if _, err = s.Create(ctx, user, "dice", key, b.Next.ID, changed); !errors.Is(err, platform.ErrIdempotencyConflict) {
		t.Fatal("same-key conflict", err)
	}
	if _, err = s.Create(ctx, user, "dice", requestKey(t), b.Next.ID, input); !errors.Is(err, ErrCommitmentInvalid) {
		t.Fatal("reused commitment accepted", err)
	}
	if _, err = s.Read(ctx, other, round.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-user round leak", err)
	}
	if _, err = s.Verify(ctx, other, round.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-user fairness leak", err)
	}
	fair, err := s.Verify(ctx, user, round.ID)
	if err != nil || !fair.Verified || fair.ServerSeed == "" || fair.ServerSeedHash != b.Next.ServerSeedHash {
		t.Fatal(fair, err)
	}
	// New service instance: only durable PostgreSQL facts may restore this round.
	restarted := newTestService(t, runtime)
	recovered, err := restarted.Read(ctx, user, round.ID)
	if err != nil || recovered.ID != round.ID || recovered.NetUnits != round.NetUnits {
		t.Fatal(recovered, err)
	}
	lookup, err := restarted.FindByKey(ctx, user, "dice", key)
	if err != nil || lookup == nil || lookup.ID != round.ID {
		t.Fatal(lookup, err)
	}
	boot, err := restarted.Bootstrap(ctx, user, "dice")
	if err != nil || boot.Latest == nil || boot.Latest.ID != round.ID || boot.Next.ID == b.Next.ID {
		t.Fatal(boot, err)
	}
	page, err := s.History(ctx, user, HistoryQuery{Game: "dice"})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != round.ID {
		t.Fatal(page, err)
	}
	// An uninitialized/empty wallet must not acquire a paid round or ledger effect.
	empty, err := s.Bootstrap(ctx, other, "dice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Create(ctx, other, "dice", requestKey(t), empty.Next.ID, input); !errors.Is(err, platform.ErrInsufficientBalance) {
		t.Fatal("insufficient balance", err)
	}
	page, err = s.History(ctx, other, HistoryQuery{})
	if err != nil || len(page.Items) != 0 {
		t.Fatal(page, err)
	}
	// Runtime role cannot mutate immutable config or settled result authority.
	err = runtime.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE games.game_config_versions SET config_hash=decode(repeat('00',32),'hex')`)
		return e
	})
	if err == nil {
		t.Fatal("runtime changed config")
	}
	err = runtime.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE games.dice_results SET die_1=1 WHERE round_id=$1`, round.ID)
		return e
	})
	if err == nil {
		t.Fatal("runtime changed settled result")
	}
	// Technical overflow is rejected before revealing or consuming the commitment.
	if _, err = owner.Apply(ctx, platform.Mutation{UserID: other, Asset: platform.AvailableChips, DeltaUnits: math.MaxInt64, BizType: "TEST_G1", BizID: requestKey(t), EntryType: "TEST_GRANT", IdempotencyKey: requestKey(t)}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Create(ctx, other, "dice", requestKey(t), empty.Next.ID, input); !errors.Is(err, platform.ErrBalanceOverflow) {
		t.Fatal("unsafe possible payout accepted", err)
	}
	after, err := s.Bootstrap(ctx, other, "dice")
	if err != nil || after.Next.ID != empty.Next.ID {
		t.Fatal("failed wager consumed commitment", err)
	}
	capOwner, capRuntime := gameTestStores(t, true)
	capUser := int64(823002)
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
		boot, e := capSvc.Bootstrap(ctx, capUser, "dice")
		if e != nil {
			t.Fatal(e)
		}
		k := requestKey(t)
		input := CreateInput{Type: "DICE", Wager: "10", Choice: "BIG"}
		capped, e := capSvc.Create(ctx, capUser, "dice", k, boot.Next.ID, input)
		if e != nil {
			t.Fatal(e)
		}
		c := capped.EconomySettlement
		if c == nil || c.GrossPayoutUnits != capped.PayoutUnits || c.CreditedPayoutUnits != min(capped.StakeUnits, capped.PayoutUnits) || capped.BalanceAfterUnits != chips.BalanceUnits+c.ActualNetUnits {
			t.Fatalf("actual vs raw payout: %+v", capped)
		}
		retry, e := capSvc.Create(ctx, capUser, "dice", k, boot.Next.ID, input)
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

func fundedPlayer(t *testing.T, owner *platform.Store) int64 {
	t.Helper()
	user := time.Now().UnixMicro()
	if err := owner.EnsureAccount(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Apply(context.Background(), platform.Mutation{UserID: user, Asset: platform.AvailableChips, DeltaUnits: 5000000000, BizType: "TEST_G1", BizID: requestKey(t), EntryType: "TEST_GRANT", IdempotencyKey: requestKey(t)}); err != nil {
		t.Fatal(err)
	}
	return user
}

func TestHistoryUsesRoundTimeNotReservedUUID(t *testing.T) {
	owner, runtime := gameTestStores(t)
	s := newTestService(t, runtime)
	ctx := context.Background()
	user := fundedPlayer(t, owner)
	a, err := s.Bootstrap(ctx, user, "dice")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond) // Different UUID reservation milliseconds, intentionally opposite to actual round creation.
	b, err := s.Bootstrap(ctx, user, "scratch")
	if err != nil {
		t.Fatal(err)
	}
	if a.Next.ReservedRoundID >= b.Next.ReservedRoundID {
		t.Fatal("fixture reservation order is not distinct")
	}
	older, err := s.Create(ctx, user, "scratch", requestKey(t), b.Next.ID, CreateInput{Type: "SCRATCH", Wager: "10"})
	if err != nil {
		t.Fatal(err)
	}
	newer, err := s.Create(ctx, user, "dice", requestKey(t), a.Next.ID, CreateInput{Type: "DICE", Wager: "10", Choice: "BIG"})
	if err != nil {
		t.Fatal(err)
	}
	page, err := s.History(ctx, user, HistoryQuery{Limit: 1})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != newer.ID || page.NextCursor != newer.ID {
		t.Fatal("reserved UUID displaced the actual latest round", page, err)
	}
	next, err := s.History(ctx, user, HistoryQuery{Limit: 1, Before: page.NextCursor})
	if err != nil || len(next.Items) != 1 || next.Items[0].ID != older.ID || next.NextCursor != "" {
		t.Fatal("unstable history keyset", next, err)
	}
	if _, err = s.History(ctx, user+1, HistoryQuery{Before: newer.ID}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("foreign cursor accepted", err)
	}
}

func TestScratchAndSummonDurableSettlement(t *testing.T) {
	owner, runtime := gameTestStores(t)
	s := newTestService(t, runtime)
	ctx := context.Background()
	user := fundedPlayer(t, owner)
	boot, err := s.Bootstrap(ctx, user, "scratch")
	if err != nil {
		t.Fatal(err)
	}
	key := requestKey(t)
	r, err := s.Create(ctx, user, "scratch", key, boot.Next.ID, CreateInput{Type: "SCRATCH", Wager: "11"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Scratch == nil || r.State != "SETTLED" || r.StakeUnits != 5500000 || r.PresentationCompletedAt != nil {
		t.Fatal(r)
	}
	before, err := runtime.ReadWallet(ctx, user, platform.AvailableChips)
	if err != nil {
		t.Fatal(err)
	}
	restarted := newTestService(t, runtime)
	b, err := restarted.Bootstrap(ctx, user, "scratch")
	if err != nil || b.EntryAction != "RESUME" || b.ScratchBlocker == nil || b.ScratchBlocker.ID != r.ID || b.Next != nil {
		t.Fatal("scratch gate was not durable", b, err)
	}
	if _, err = s.Create(ctx, user, "scratch", requestKey(t), boot.Next.ID, CreateInput{Type: "SCRATCH", Wager: "10"}); !errors.Is(err, ErrScratchIncomplete) {
		t.Fatal("incomplete scratch purchased again", err)
	}
	fair, err := s.Verify(ctx, user, r.ID)
	if err != nil || !fair.Verified {
		t.Fatal(fair, err)
	}
	action := requestKey(t)
	revealed, err := s.RevealComplete(ctx, user, r.ID, action)
	if err != nil || revealed.PresentationCompletedAt == nil {
		t.Fatal(revealed, err)
	}
	repeat, err := s.RevealComplete(ctx, user, r.ID, action)
	if err != nil || !repeat.PresentationCompletedAt.Equal(*revealed.PresentationCompletedAt) {
		t.Fatal("reveal replay changed presentation", repeat, err)
	}
	after, err := runtime.ReadWallet(ctx, user, platform.AvailableChips)
	if err != nil || before != after {
		t.Fatal("presentation changed wallet", before, after, err)
	}
	for _, mode := range []SummonMode{Single, Tenfold} {
		boot, err := s.Bootstrap(ctx, user, "summon")
		if err != nil {
			t.Fatal(err)
		}
		input := CreateInput{Type: "SUMMON", BaseWager: "10", Mode: string(mode)}
		key := requestKey(t)
		r, err := s.Create(ctx, user, "summon", key, boot.Next.ID, input)
		if err != nil {
			t.Fatal(err)
		}
		count := 1
		if mode == Tenfold {
			count = 10
		}
		if r.Summon == nil || len(r.Summon.Draws) != count || r.StakeUnits != 5000000*int64(count) {
			t.Fatal(r)
		}
		var multiplier int64
		for _, d := range r.Summon.Draws {
			multiplier += d.Multiplier
		}
		if r.PayoutUnits != multiplier*5000000 || r.NetUnits != r.PayoutUnits-r.StakeUnits {
			t.Fatal("summon aggregate mismatch", r)
		}
		again, err := restarted.Create(ctx, user, "summon", key, boot.Next.ID, input)
		if err != nil || again.ID != r.ID || again.PayoutUnits != r.PayoutUnits {
			t.Fatal(again, err)
		}
		fair, err := restarted.Verify(ctx, user, r.ID)
		if err != nil || !fair.Verified {
			t.Fatal(fair, err)
		}
	}
}
