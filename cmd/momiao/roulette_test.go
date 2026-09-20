package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/historyaccess"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/roulette"
	"github.com/jackc/pgx/v5"
	"strconv"
	"testing"
	"time"
)

func TestRouletteEscrowReplayLifecycle(t *testing.T) {
	// A UTC-local host returns SQL times with a different Location from JSON Z timestamps.
	previousLocal := time.Local
	time.Local = time.FixedZone("roulette-utc-host", 0)
	t.Cleanup(func() { time.Local = previousLocal })
	owner, runtime := gameBrowserStores(t)
	ctx := context.Background()
	// 账户／金额使用这个既有 fixture 的本地数据库；从随机 UUID 派生两个正 int64 用户 ID，避免重复运行串数据。
	var seed [16]byte
	if _, err := rand.Read(seed[:]); err != nil {
		t.Fatal(err)
	}
	a := int64(binary.BigEndian.Uint64(seed[:8])&((1<<60)-1)) + 1000
	b := a + 1
	for _, user := range []int64{a, b} {
		if err := owner.EnsureAccount(ctx, user); err != nil {
			t.Fatal(err)
		}
		wallet, err := owner.ReadWallet(ctx, user, platform.AvailableChips)
		if err != nil || wallet.BalanceUnits != 0 {
			t.Fatal("test account is not fresh")
		}
		key := fmt.Sprintf("roulette-fixture-%x-%d", seed, user)
		if _, err := owner.Apply(ctx, platform.Mutation{UserID: user, Asset: platform.AvailableChips,
			DeltaUnits: 500000000, BizType: "ROULETTE_FIXTURE", BizID: key,
			EntryType: "ROULETTE_FIXTURE", IdempotencyKey: key}); err != nil {
			t.Fatal(err)
		}
	}
	// Share this isolated fixture's key with its worker during the SETTLING handoff.
	var fixtureKey [32]byte
	for i := range fixtureKey {
		fixtureKey[i] = byte(i + 41)
	}
	keys := roulette.Keyring{Active: "g1-browser-v1", Keys: map[string][32]byte{"g1-browser-v1": fixtureKey}}
	service, err := roulette.NewService(runtime, keys)
	if err != nil {
		t.Fatal(err)
	}
	key := func(n int) string { return fmt.Sprintf("roulette-check-%x-%d", seed, n) }
	receipt, err := service.Create(ctx, a, roulette.CreateRequest{Key: key(0), Game: "devil-roulette", Stake: "10", Players: 2})
	if err != nil {
		t.Fatal(err)
	}
	id := receipt.RoundID
	view := func(user int64) roulette.RoomView {
		v, e := service.View(ctx, user, id)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	v := view(a)
	if _, err = service.Command(ctx, b, id, roulette.Command{Key: key(1), ExpectedVersion: v.Version,
		Action: roulette.Action{Kind: "JOIN"}}); err != nil {
		t.Fatal(err)
	}
	for i, user := range []int64{a, b} {
		v = view(user)
		ready := &roulette.ReadyConfirmation{ClientSeed: fmt.Sprintf("player-%d", i),
			ConfigHash: v.Binding.ConfigHash, PolicyHash: v.Binding.PolicyHash,
			ServerSeedHash: v.ServerSeedHash, StakeUnits: strconv.FormatInt(v.StakeUnits, 10)}
		if _, err = service.Command(ctx, user, id, roulette.Command{Key: key(i + 2), ExpectedVersion: v.Version,
			Action: roulette.Action{Kind: "READY"}, Ready: ready}); err != nil {
			t.Fatal(err)
		}
	}
	var last roulette.Command
	var lastUser int64
	var accepted roulette.Receipt
	for n := 0; n < 64; n++ {
		v = view(a)
		if v.State == "FINISHED" {
			break
		}
		if v.State != "PLAYING" || v.TurnSeat == nil {
			t.Fatalf("unexpected state %s", v.State)
		}
		lastUser = a
		if v.Self == nil || v.Self.Seat != *v.TurnSeat {
			lastUser = b
		}
		last = roulette.Command{Key: key(n + 10), ExpectedVersion: v.Version,
			Action: roulette.Action{Kind: "DEVIL_SHOOT", Target: "OPPONENT"}}
		accepted, err = service.Command(ctx, lastUser, id, last)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			service, err = roulette.NewService(runtime, keys)
			if err != nil {
				t.Fatal(err)
			}
			retried, e := service.Command(ctx, lastUser, id, last)
			if e != nil || retried != accepted {
				t.Fatalf("replayed command changed: %v", e)
			}
		}
	}
	if v = view(a); v.State != "FINISHED" {
		t.Fatalf("game did not finish: %s", v.State)
	}
	retried, err := service.Command(ctx, lastUser, id, last)
	if err != nil || retried != accepted {
		t.Fatalf("settlement retry changed: %v", err)
	}
	wa, err := owner.ReadWallet(ctx, a, platform.AvailableChips)
	if err != nil {
		t.Fatal(err)
	}
	wb, err := owner.ReadWallet(ctx, b, platform.AvailableChips)
	if err != nil {
		t.Fatal(err)
	}
	if wa.BalanceUnits+wb.BalanceUnits != 1000000000 || v.PoolUnits != 0 {
		t.Fatal("wallet plus escrow conservation failed")
	}
	if !((wa.BalanceUnits == 505000000 && wb.BalanceUnits == 495000000) ||
		(wa.BalanceUnits == 495000000 && wb.BalanceUnits == 505000000)) {
		t.Fatal("winner and loser balances do not match equal stakes")
	}
	err = owner.WithTx(ctx, func(tx pgx.Tx) error {
		var locks, payouts, actions, lastSequence int64
		if e := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE kind='ESCROW'),
            count(*) FILTER(WHERE kind='PAYOUT') FROM roulette.funding WHERE round_id=$1`, id).
			Scan(&locks, &payouts); e != nil {
			return e
		}
		if e := tx.QueryRow(ctx, `SELECT count(*),coalesce(max(sequence),0)
            FROM roulette.actions WHERE round_id=$1`, id).Scan(&actions, &lastSequence); e != nil {
			return e
		}
		if locks != 2 || payouts != 1 || actions == 0 || actions != lastSequence {
			return fmt.Errorf("duplicate funding or broken sequence: %d %d %d %d", locks, payouts, actions, lastSequence)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	proof, err := service.HistoryVerify(ctx, historyaccess.Own(a), id)
	if err != nil || !proof.Valid || !proof.CommitmentValid || !proof.ConfigValid || !proof.ActionsValid || !proof.SettlementValid {
		t.Fatalf("same-round replay failed: %+v: %v", proof, err)
	}

	admin, err := games.NewService(runtime, games.Keyring{Active: keys.Active, Keys: keys.Keys})
	if err != nil {
		t.Fatal(err)
	}
	overview, err := admin.ReadOpsGame(ctx, "devil-roulette")
	if err != nil || overview.Roulette == nil || overview.Roulette.ConservationDelta != "0" {
		t.Fatalf("same-domain Ops conservation: %v", err)
	}
	if _, err = service.HistoryVerify(ctx, historyaccess.Own(b+1), id); err != historyaccess.ErrNotFound {
		t.Fatalf("non-funded history access: %v", err)
	}
}
