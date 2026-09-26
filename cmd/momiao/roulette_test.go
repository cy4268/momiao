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
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func TestRouletteEscrowReplayLifecycle(t *testing.T) {
	if os.Getenv("MOMIAO_GAMES_TEST_CONNECTION_FILE") == "" {
		t.Skip("isolated local G1 connection required")
	}
	// Confine the UTC-host regression to its own test process. Other tests can
	// still have HTTP cleanup goroutines reading time.Local after they return.
	if os.Getenv("MOMIAO_ROULETTE_UTC_TEST_CHILD") != "1" {
		deadline := time.Now().Add(time.Minute)
		if limit, ok := t.Deadline(); ok && limit.Before(deadline) {
			deadline = limit.Add(-time.Second)
		}
		childCtx, cancel := context.WithDeadline(t.Context(), deadline)
		defer cancel()
		cmd := exec.CommandContext(childCtx, os.Args[0], "-test.run=^TestRouletteEscrowReplayLifecycle$", "-test.count=1", "-test.v")
		cmd.Env = append(os.Environ(), "MOMIAO_ROULETTE_UTC_TEST_CHILD=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("isolated UTC-host regression: %v\n%s", err, output)
		}
		t.Logf("%s", output)
		return
	}
	// Set once before database workers start; process exit releases this state.
	time.Local = time.FixedZone("roulette-utc-host", 0)
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
	if e := owner.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE economy.policy_runtime SET active_version='economy-cap-v1' WHERE singleton`)
		return e
	}); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		_ = owner.WithTx(context.Background(), func(tx pgx.Tx) error {
			_, e := tx.Exec(context.Background(), `UPDATE economy.policy_runtime SET active_version=NULL WHERE singleton`)
			return e
		})
	})
	capped, e := roulette.NewServiceWithEconomy(runtime, keys, &quotaFixture{})
	if e != nil {
		t.Fatal(e)
	}
	for idx, game := range []string{"devil-roulette", "pressure-roulette"} {
		count := idx + 2
		users := []int64{}
		for seat := 0; seat < count; seat++ {
			u := int64(824000 + idx*10 + seat)
			users = append(users, u)
			if e = owner.EnsureAccount(ctx, u); e != nil {
				t.Fatal(e)
			}
			for asset, value := range map[platform.Asset]int64{platform.AvailableChips: 2000000000000, platform.ReserveAPICredit: platform.AssetCapUnits - 2000000000000} {
				k := fmt.Sprintf("cap-%s-%d-%s", game, u, asset)
				if _, e = owner.Apply(ctx, platform.Mutation{UserID: u, Asset: asset, DeltaUnits: value, BizType: "TEST_CAP", BizID: k, EntryType: "TEST_GRANT", IdempotencyKey: k}); e != nil {
					t.Fatal(e)
				}
			}
		}
		receipt, e := capped.Create(ctx, users[0], roulette.CreateRequest{Key: key(1000 + idx), Game: game, Stake: "1000001", Players: count})
		if e != nil {
			t.Fatal("multiplayer capped by solo maximum", e)
		}
		for seat, u := range users {
			v, e := capped.View(ctx, u, receipt.RoundID)
			if e != nil {
				t.Fatal(e)
			}
			if seat > 0 {
				if _, e = capped.Command(ctx, u, receipt.RoundID, roulette.Command{Key: key(1100 + idx*10 + seat), ExpectedVersion: v.Version, Action: roulette.Action{Kind: "JOIN"}}); e != nil {
					t.Fatal(e)
				}
			}
			v, e = capped.View(ctx, u, receipt.RoundID)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = capped.Command(ctx, u, receipt.RoundID, roulette.Command{Key: key(1200 + idx*10 + seat), ExpectedVersion: v.Version, Action: roulette.Action{Kind: "READY"}, Ready: &roulette.ReadyConfirmation{ClientSeed: "cap-player", ConfigHash: v.Binding.ConfigHash, PolicyHash: v.Binding.PolicyHash, ServerSeedHash: v.ServerSeedHash, StakeUnits: strconv.FormatInt(v.StakeUnits, 10)}}); e != nil {
				t.Fatal(e)
			}
		}
		for seat, u := range users[:len(users)-1] {
			v, e := capped.View(ctx, u, receipt.RoundID)
			if e != nil {
				t.Fatal(e)
			}
			if game == "pressure-roulette" {
				if v.TurnSeat == nil {
					t.Fatal("missing turn")
				}
				u = users[*v.TurnSeat]
			}
			cmd := roulette.Command{Key: key(1300 + idx*10 + seat), ExpectedVersion: v.Version, Action: roulette.Action{Kind: "SURRENDER"}}
			first, e := capped.Command(ctx, u, receipt.RoundID, cmd)
			if e != nil {
				t.Fatal(e)
			}
			replay, e := capped.Command(ctx, u, receipt.RoundID, cmd)
			if e != nil || first != replay {
				t.Fatal("cap command replay", e)
			}
		}
		if _, e = capped.StepDue(ctx, 1); e != nil {
			t.Fatal("cap settlement worker", e)
		}
		winner := users[len(users)-1]
		finalView, e := capped.View(ctx, users[0], receipt.RoundID)
		if e != nil {
			t.Fatal(e)
		}
		for _, p := range finalView.Players {
			if p.Alive {
				winner = users[p.Seat]
			}
		}
		v, e := capped.View(ctx, winner, receipt.RoundID)
		if e != nil || v.State != "FINISHED" || v.EconomySettlement == nil {
			t.Fatal("cap terminal", v.State, e)
		}
		c := v.EconomySettlement
		if c.CreditedPayoutUnits != 500000500000 || c.WithheldUnits != 500000500000*int64(count-1) || c.ActualNetUnits != 0 {
			t.Fatalf("multiplayer cap %+v", c)
		}
		proof, e := capped.HistoryVerify(ctx, historyaccess.Own(winner), receipt.RoundID)
		if e != nil || !proof.Valid {
			t.Fatalf("capped proof %+v %v", proof, e)
		}
		report, e := admin.ReadOpsGame(ctx, game)
		if e != nil || report.Roulette.ConservationDelta != "0" {
			t.Fatal("withheld not accounted", e)
		}
		cancellation, e := capped.Create(ctx, winner, roulette.CreateRequest{Key: key(1400 + idx), Game: game, Stake: "1000001", Players: count})
		if e != nil {
			t.Fatal(e)
		}
		v, e = capped.View(ctx, winner, cancellation.RoundID)
		if e != nil {
			t.Fatal(e)
		}
		before, e := owner.ReadWallet(ctx, winner, platform.AvailableChips)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = capped.Command(ctx, winner, cancellation.RoundID, roulette.Command{Key: key(1500 + idx), ExpectedVersion: v.Version, Action: roulette.Action{Kind: "READY"}, Ready: &roulette.ReadyConfirmation{ClientSeed: "cap-refund", ConfigHash: v.Binding.ConfigHash, PolicyHash: v.Binding.PolicyHash, ServerSeedHash: v.ServerSeedHash, StakeUnits: strconv.FormatInt(v.StakeUnits, 10)}}); e != nil {
			t.Fatal(e)
		}
		v, e = capped.View(ctx, winner, cancellation.RoundID)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = capped.Command(ctx, winner, cancellation.RoundID, roulette.Command{Key: key(1600 + idx), ExpectedVersion: v.Version, Action: roulette.Action{Kind: "CANCEL"}}); e != nil {
			t.Fatal(e)
		}
		after, e := owner.ReadWallet(ctx, winner, platform.AvailableChips)
		if e != nil || after.BalanceUnits != before.BalanceUnits {
			t.Fatal("principal refund clipped", e)
		}
	}

}
