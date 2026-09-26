package games

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	bj "github.com/cy4268/momiao/internal/games/blackjack"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

// Fixture-only precommit construction: choose a reproducible starting hand
// before the test accepts a wager, then persist the encrypted seed/commitment.
// There is no production seed/result override or shoe supplied by an HTTP user.
func blackjackFixture(t *testing.T, owner, runtime *platform.Store) (*Service, int64, Commitment) {
	return blackjackFixtureMatching(t, owner, runtime, func(shoe [312]uint16) bool {
		return shoe[0]%13 == 7 && shoe[2]%13 == 7 && shoe[4]%13 < 8 && shoe[5]%13 < 8 && shoe[1]%13 >= 1 && shoe[1]%13 <= 8
	})
}
func blackjackFixtureMatching(t *testing.T, owner, runtime *platform.Store, match func([312]uint16) bool) (*Service, int64, Commitment) {
	t.Helper()
	ctx := context.Background()
	s := newTestService(t, runtime)
	user := time.Now().UnixMicro()
	var capActive bool
	if e := owner.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT active_version IS NOT NULL FROM economy.policy_runtime WHERE singleton`).Scan(&capActive)
	}); e != nil {
		t.Fatal(e)
	}
	if capActive {
		user = 1000000000 + time.Now().UnixNano()%900000000
		if e := owner.WithTx(ctx, func(tx pgx.Tx) error { _, e := tx.Exec(ctx, `INSERT INTO public.users(id) VALUES($1)`, user); return e }); e != nil {
			t.Fatal(e)
		}
	}
	if err := owner.EnsureAccount(ctx, user); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Apply(ctx, platform.Mutation{UserID: user, Asset: platform.AvailableChips, DeltaUnits: 5000000000000, BizType: "TEST_G1_BLACKJACK", BizID: requestKey(t), EntryType: "TEST_GRANT", IdempotencyKey: requestKey(t)}); err != nil {
		t.Fatal(err)
	}
	b, err := s.Bootstrap(ctx, user, "blackjack")
	if err != nil || b.Next == nil {
		t.Fatal(b, err)
	}
	c := *b.Next
	c.ID = requestKey(t)
	c.ReservedRoundID = requestKey(t)
	c.Nonce++
	var seed [32]byte
	found := false
	for i := 0; i < 20000; i++ {
		seed = sha256.Sum256([]byte(fmt.Sprintf("g1-private-test-precommit-%d", i)))
		h := sha256.Sum256(seed[:])
		c.ServerSeedHash = hex.EncodeToString(h[:])
		fair, e := fairnessInput("blackjack", c)
		if e != nil {
			t.Fatal(e)
		}
		shoe, e := blackjackShoe(seed[:], fair)
		if e != nil {
			t.Fatal(e)
		}
		// Pair of eights plus both split replacement cards <= 8: guarantees two
		// live split hands and makes double/stand persistence independently testable.
		if match(shoe) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("fixture pair not found")
	}
	nonce, encrypted, err := s.sealSeed(user, "blackjack", c, seed[:])
	if err != nil {
		t.Fatal(err)
	}
	err = owner.WithTx(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `UPDATE games.fairness_commitments SET state='INVALIDATED' WHERE commitment_id=$1`, b.Next.ID); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE games.fairness_nonce_cursors SET next_nonce=$2 WHERE newapi_user_id=$1 AND game_slug='blackjack'`, user, c.Nonce+1); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO games.fairness_commitments(commitment_id,reserved_round_id,newapi_user_id,game_slug,nonce,client_seed,client_seed_version,state,server_seed_hash,key_version,gcm_nonce,ciphertext,ruleset_version,algorithm_version,fairness_stream_version,game_config_version_id,game_config_hash,wager_policy_version_id,wager_policy_hash,resource_versions,economic_policy_version) VALUES($1,$2,$3,'blackjack',$4,$5,$6,'AVAILABLE',decode($7,'hex'),'fixture-v1',$8,$9,$10,$11,$12,$13,decode($14,'hex'),$15,decode($16,'hex'),$17,NULLIF($18,''))`, c.ID, c.ReservedRoundID, user, c.Nonce, c.ClientSeed, c.ClientVersion, c.ServerSeedHash, nonce, encrypted, c.Ruleset, c.Algorithm, c.Stream, c.ConfigVersion, c.ConfigHash, c.PolicyVersion, c.PolicyHash, []byte(c.Resources), c.EconomicVersion)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, user, c
}

func TestBlackjackNaturalAtomicSettlement(t *testing.T) {
	owner, runtime := gameTestStores(t)
	s, user, c := blackjackFixtureMatching(t, owner, runtime, func(shoe [312]uint16) bool {
		return shoe[0]%13 == 0 && shoe[2]%13 >= 9 && shoe[1]%13 >= 1 && shoe[1]%13 <= 8
	})
	r, err := s.Create(context.Background(), user, "blackjack", requestKey(t), c.ID, CreateInput{Type: "BLACKJACK", InitialWager: "11"})
	if err != nil {
		t.Fatal(err)
	}
	if r.State != "SETTLED" || r.PayoutUnits != 13750000 || r.NetUnits != 8250000 || !r.Blackjack.Hands[0].Natural || r.SettledAt == nil {
		t.Fatal("natural must settle directly at 3:2 profit", r)
	}
	v, err := newTestService(t, runtime).Verify(context.Background(), user, r.ID)
	if err != nil || !v.Verified {
		t.Fatal("natural recovery", err)
	}
	t.Logf("Immediate natural %s: 11 stake, 27.5 total payout, no durable pending state and deterministic audit passed", r.ID)
}

func TestBlackjackPersistentActionsAndOriginalReplay(t *testing.T) {
	owner, runtime := gameTestStores(t)
	s, user, c := blackjackFixture(t, owner, runtime)
	ctx := context.Background()
	key := requestKey(t)
	input := CreateInput{Type: "BLACKJACK", InitialWager: "10"}
	r, err := s.Create(ctx, user, "blackjack", key, c.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if r.State != "PLAYER_TURN" || r.Blackjack == nil || r.Blackjack.Version != 1 || r.SettlementTransactionID != "" || r.SettledAt != nil {
		t.Fatal("not an active durable round", r)
	}
	if len(r.Blackjack.DealerCards) != 1 || r.Blackjack.DealerRevealed {
		t.Fatal("hole exposed")
	}
	v, err := s.Verify(ctx, user, r.ID)
	if err != nil || v.ServerSeed != "" || v.RevealState != "NOT_YET_AVAILABLE" {
		t.Fatal("early seed reveal", err)
	}
	b, err := newTestService(t, runtime).Bootstrap(ctx, user, "blackjack")
	if err != nil || b.Active == nil || b.Active.ID != r.ID || b.Next != nil || b.EntryAction != "RESUME" {
		t.Fatal("restart resume", b, err)
	}
	if _, err = s.Create(ctx, user, "blackjack", requestKey(t), c.ID, input); !errors.Is(err, ErrActiveRound) {
		t.Fatal("second active accepted", err)
	}
	split := BlackjackActionInput{ActionID: requestKey(t), ActionType: "SPLIT", HandID: r.Blackjack.ActiveHandID, ExpectedVersion: "1"}
	first, err := s.BlackjackAction(ctx, user, r.ID, split)
	if err != nil {
		t.Fatal(err)
	}
	if first.StakeUnits != 10000000 || len(first.Blackjack.Hands) != 2 || first.Blackjack.Version != 2 {
		t.Fatal(first)
	}
	stale := BlackjackActionInput{ActionID: requestKey(t), ActionType: "STAND", HandID: first.Blackjack.ActiveHandID, ExpectedVersion: "1"}
	if _, err = s.BlackjackAction(ctx, user, r.ID, stale); !errors.Is(err, bj.ErrStaleVersion) {
		t.Fatal("stale action", err)
	}
	action := BlackjackActionInput{ActionID: requestKey(t), ActionType: "DOUBLE", HandID: first.Blackjack.ActiveHandID, ExpectedVersion: "2"}
	next, err := s.BlackjackAction(ctx, user, r.ID, action)
	if err != nil {
		t.Fatal(err)
	}
	if next.StakeUnits != 15000000 || next.Blackjack.Version != 3 || next.Blackjack.ActiveHandID == first.Blackjack.ActiveHandID {
		t.Fatal("double did not advance", next)
	}
	// Accepted actions continue during maintenance, while another deal is blocked.
	err = owner.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE games.game_registry SET configured_runtime_state='MAINTENANCE' WHERE game_slug='blackjack'`)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = owner.WithTx(ctx, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `UPDATE games.game_registry SET configured_runtime_state='AVAILABLE' WHERE game_slug='blackjack'`)
			return e
		})
	})
	final, err := s.BlackjackAction(ctx, user, r.ID, BlackjackActionInput{ActionID: requestKey(t), ActionType: "STAND", HandID: next.Blackjack.ActiveHandID, ExpectedVersion: "3"})
	if err != nil {
		t.Fatal(err)
	}
	if final.State != "SETTLED" || final.Blackjack.Version != 4 || !final.Blackjack.DealerRevealed || final.SettledAt == nil {
		t.Fatal(final)
	}
	original, err := newTestService(t, runtime).BlackjackAction(ctx, user, r.ID, split)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(first)
	z, _ := json.Marshal(original)
	if string(a) != string(z) {
		t.Fatal("duplicate action returned later state instead of its original response")
	}
	split.ActionType = "HIT"
	if _, err = s.BlackjackAction(ctx, user, r.ID, split); !errors.Is(err, platform.ErrIdempotencyConflict) {
		t.Fatal("action ID conflict", err)
	}
	v, err = s.Verify(ctx, user, r.ID)
	if err != nil || !v.Verified || v.BlackjackAudit == nil {
		t.Fatal("terminal audit", v.Verified, err)
	}
	if _, err = s.Read(ctx, user+1, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-user round", err)
	}
	err = owner.WithTx(ctx, func(tx pgx.Tx) error {
		var actions, debits, payouts int
		var sum int64
		if e := tx.QueryRow(ctx, `SELECT count(*),coalesce(sum(additional_stake_units),0) FROM games.round_actions WHERE round_id=$1`, r.ID).Scan(&actions, &sum); e != nil {
			return e
		}
		if e := tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE delta_units<0),count(*) FILTER (WHERE delta_units>0) FROM economy.wallet_ledger WHERE newapi_user_id=$1 AND entry_type IN ('GAME_WAGER','GAME_ADDITIONAL_WAGER','GAME_PAYOUT')`, user).Scan(&debits, &payouts); e != nil {
			return e
		}
		if actions != 3 || sum != 10000000 || debits != 3 || payouts > 1 {
			t.Fatal(actions, sum, debits, payouts)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Blackjack %s: encrypted precommit, hidden hole, resume, split/double/stand, original action replay, maintenance continuation and terminal audit passed", r.ID)
	capOwner, capRuntime := gameTestStores(t, true)
	if e := capOwner.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE games.game_registry SET configured_runtime_state='AVAILABLE' WHERE game_slug='blackjack'`)
		return e
	}); e != nil {
		t.Fatal(e)
	}
	cappedSvc, capUser, commit := blackjackFixture(t, capOwner, capRuntime)
	capped, e := cappedSvc.Create(ctx, capUser, "blackjack", requestKey(t), commit.ID, CreateInput{Type: "BLACKJACK", InitialWager: "600000"})
	if e != nil {
		t.Fatal(e)
	}
	before, e := capOwner.ReadWallet(ctx, capUser, platform.AvailableChips)
	if e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"SPLIT", "DOUBLE"} {
		if _, e = cappedSvc.BlackjackAction(ctx, capUser, capped.ID, BlackjackActionInput{ActionID: requestKey(t), ActionType: kind, HandID: capped.Blackjack.ActiveHandID, ExpectedVersion: "1"}); !errors.Is(e, ErrInvalidInput) {
			t.Fatal("cumulative cap", kind, e)
		}
	}
	after, e := capOwner.ReadWallet(ctx, capUser, platform.AvailableChips)
	if e != nil || before != after {
		t.Fatal("rejected action debited", e)
	}
	settled, e := cappedSvc.BlackjackAction(ctx, capUser, capped.ID, BlackjackActionInput{ActionID: requestKey(t), ActionType: "STAND", HandID: capped.Blackjack.ActiveHandID, ExpectedVersion: "1"})
	if e != nil || settled.EconomySettlement == nil {
		t.Fatal("cap final", e)
	}

}

func TestBlackjackDurableTimeoutAndMismatch(t *testing.T) {
	owner, runtime := gameTestStores(t)
	s, user, c := blackjackFixture(t, owner, runtime)
	ctx := context.Background()
	r, err := s.Create(ctx, user, "blackjack", requestKey(t), c.ID, CreateInput{Type: "BLACKJACK", InitialWager: "10"})
	if err != nil {
		t.Fatal(err)
	}
	// The worker entry point gets database time; this internal boundary receives
	// the same durable deadline plus one second to model 24 hours without sleeping.
	if err = s.processBlackjackJob(ctx, r.ID, "BLACKJACK_AUTO_RESOLVE", r.Blackjack.AutoResolveAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	after, err := newTestService(t, runtime).Read(ctx, user, r.ID)
	if err != nil || after.State != "SETTLED" {
		t.Fatal(after, err)
	}
	v, err := s.Verify(ctx, user, r.ID)
	if err != nil || !v.Verified {
		t.Fatal("timeout replay", err)
	}
	if err = s.processBlackjackJob(ctx, r.ID, "BLACKJACK_AUTO_RESOLVE", r.Blackjack.AutoResolveAt.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	s, user, c = blackjackFixture(t, owner, runtime)
	r, err = s.Create(ctx, user, "blackjack", requestKey(t), c.ID, CreateInput{Type: "BLACKJACK", InitialWager: "10"})
	if err != nil {
		t.Fatal(err)
	}
	// Owner fault injection on a private test round, never an HTTP mutation.
	err = owner.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `ALTER TABLE games.blackjack_round_state DISABLE TRIGGER blackjack_state_guard; UPDATE games.blackjack_round_state SET shoe_hash=decode(repeat('00',32),'hex') WHERE round_id='`+r.ID+`'; SET CONSTRAINTS ALL IMMEDIATE; ALTER TABLE games.blackjack_round_state ENABLE TRIGGER blackjack_state_guard`, pgx.QueryExecModeSimpleProtocol)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	broken, err := s.Read(ctx, user, r.ID)
	if err != nil || broken.RecoveryState != "NEEDS_REVIEW" || broken.Blackjack != nil {
		t.Fatal("mismatch was not frozen", broken, err)
	}
	_, err = s.BlackjackAction(ctx, user, r.ID, BlackjackActionInput{ActionID: requestKey(t), ActionType: "STAND", HandID: r.Blackjack.ActiveHandID, ExpectedVersion: "1"})
	if !errors.Is(err, bj.ErrNeedsReview) {
		t.Fatal("mismatched shoe dealt", err)
	}
	t.Log("Durable timeout job replayed/settled once with simulated elapsed DB clock; inconsistent protected shoe hash froze the original round without dealing")
}

func TestBlackjackConcurrentCreateAndDouble(t *testing.T) {
	owner, runtime := gameTestStores(t, true)
	s, user, c := blackjackFixture(t, owner, runtime)
	ctx := context.Background()
	key := requestKey(t)
	concurrent := func(call func() (GameRound, error)) GameRound {
		t.Helper()
		var wg sync.WaitGroup
		results := make([]GameRound, 8)
		errs := make([]error, 8)
		for i := range results {
			wg.Add(1)
			go func(i int) { defer wg.Done(); results[i], errs[i] = call() }(i)
		}
		wg.Wait()
		first, _ := json.Marshal(results[0])
		for i, err := range errs {
			if err != nil {
				t.Fatal(err)
			}
			response, _ := json.Marshal(results[i])
			if string(first) != string(response) {
				var expected, actual map[string]json.RawMessage
				_ = json.Unmarshal(first, &expected)
				_ = json.Unmarshal(response, &actual)
				for k, v := range expected {
					if string(v) != string(actual[k]) {
						t.Logf("public field %s differs: %s vs %s", k, v, actual[k])
					}
				}
				t.Fatal("concurrent retry returned a different response")
			}
		}
		return results[0]
	}
	r := concurrent(func() (GameRound, error) {
		return s.Create(ctx, user, "blackjack", key, c.ID, CreateInput{Type: "BLACKJACK", InitialWager: "500000"})
	})
	action := BlackjackActionInput{ActionID: requestKey(t), ActionType: "DOUBLE", HandID: r.Blackjack.ActiveHandID, ExpectedVersion: "1"}
	final := concurrent(func() (GameRound, error) { return s.BlackjackAction(ctx, user, r.ID, action) })
	if final.State != "SETTLED" || final.StakeUnits != platform.SinglePlayerMaxUnits || final.Blackjack.Version != 2 {
		t.Fatal("concurrent double changed more than once")
	}
	stored, err := newTestService(t, runtime).FindBlackjackAction(ctx, user, r.ID, action.ActionID)
	if err != nil || stored == nil || stored.ID != final.ID || stored.Blackjack.Version != 2 {
		t.Fatal("reconcile original action", err)
	}
	err = owner.WithTx(ctx, func(tx pgx.Tx) error {
		var rounds, actions, initial, additional, settlements int
		if e := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM games.game_rounds WHERE newapi_user_id=$1), (SELECT count(*) FROM games.round_actions WHERE round_id=$2), (SELECT count(*) FROM economy.wallet_ledger WHERE newapi_user_id=$1 AND entry_type='GAME_WAGER'), (SELECT count(*) FROM economy.wallet_ledger WHERE newapi_user_id=$1 AND entry_type='GAME_ADDITIONAL_WAGER'), (SELECT count(*) FROM economy.asset_transactions WHERE transaction_id=$3 AND status='CONFIRMED')`, user, r.ID, final.SettlementTransactionID).Scan(&rounds, &actions, &initial, &additional, &settlements); e != nil {
			return e
		}
		if rounds != 1 || actions != 1 || initial != 1 || additional != 1 || settlements != 1 {
			t.Fatal("duplicated financial work", rounds, actions, initial, additional, settlements)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("8 simultaneous creates and 8 simultaneous DOUBLE actions: one round, one additional debit, one original response and one confirmed settlement")
}
