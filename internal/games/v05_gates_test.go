package games

import (
	"context"
	"errors"
	"testing"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

// All changes are confined to the explicit gameTestStores database. Every
// rejected request must leave its commitment, wallet and original key unused.
func TestV05AllGameGatesAndInsufficientBalance(t *testing.T) {
	owner, runtime := gameTestStores(t)
	s := newTestService(t, runtime)
	ctx := context.Background()
	user := fundedPlayer(t, owner)
	empty := user + 1
	if err := owner.EnsureAccount(ctx, empty); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		slug  string
		input CreateInput
	}{
		{"dice", CreateInput{Type: "DICE", Wager: "10", Choice: "BIG"}},
		{"scratch", CreateInput{Type: "SCRATCH", Wager: "10"}},
		{"summon", CreateInput{Type: "SUMMON", BaseWager: "10", Mode: "SINGLE"}},
		{"slot", CreateInput{Type: "SLOT", TotalWager: "10"}},
		{"blackjack", CreateInput{Type: "BLACKJACK", InitialWager: "10"}},
	} {
		t.Run(tc.slug, func(t *testing.T) {
			before, err := s.Bootstrap(ctx, user, tc.slug)
			if err != nil || before.Next == nil {
				t.Fatal("initial commitment missing", err)
			}
			for _, mode := range []string{"MAINTENANCE", "UNAVAILABLE", "GLOBAL_MAINTENANCE"} {
				t.Run(mode, func(t *testing.T) {
					global := mode == "GLOBAL_MAINTENANCE"
					err := owner.WithTx(ctx, func(tx pgx.Tx) error {
						if global {
							_, e := tx.Exec(ctx, `UPDATE games.runtime_gate SET maintenance=TRUE WHERE singleton`)
							return e
						}
						_, e := tx.Exec(ctx, `UPDATE games.game_registry SET configured_runtime_state=$2 WHERE game_slug=$1`, tc.slug, mode)
						return e
					})
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() {
						err := owner.WithTx(ctx, func(tx pgx.Tx) error {
							if global {
								_, e := tx.Exec(ctx, `UPDATE games.runtime_gate SET maintenance=FALSE WHERE singleton`)
								return e
							}
							_, e := tx.Exec(ctx, `UPDATE games.game_registry SET configured_runtime_state='AVAILABLE' WHERE game_slug=$1`, tc.slug)
							return e
						})
						if err != nil {
							t.Error("restore fixture gate", err)
						}
					})
					key := requestKey(t)
					_, err = s.Create(ctx, user, tc.slug, key, before.Next.ID, tc.input)
					want := ErrMaintenance
					if mode == "UNAVAILABLE" {
						want = ErrUnavailable
					}
					if !errors.Is(err, want) {
						t.Fatal("gate accepted wager or returned wrong error", err)
					}
					if r, e := s.FindByKey(ctx, user, tc.slug, key); e != nil || r != nil {
						t.Fatal("rejected gate acquired a round", e)
					}
				})
			}
			after, err := s.Bootstrap(ctx, user, tc.slug)
			if err != nil || after.Next == nil || after.Next.ID != before.Next.ID || after.AvailableUnits != before.AvailableUnits || after.Latest != nil || after.Active != nil {
				t.Fatal("gate rejection changed commitment, balance or rounds", err)
			}
			zero, err := s.Bootstrap(ctx, empty, tc.slug)
			if err != nil || zero.Next == nil || zero.AvailableUnits != 0 {
				t.Fatal("empty fixture differs", err)
			}
			key := requestKey(t)
			if _, err := s.Create(ctx, empty, tc.slug, key, zero.Next.ID, tc.input); !errors.Is(err, platform.ErrInsufficientBalance) {
				t.Fatal("empty wallet accepted wager", err)
			}
			if r, err := s.FindByKey(ctx, empty, tc.slug, key); err != nil || r != nil {
				t.Fatal("insufficient wager acquired round", err)
			}
			again, err := s.Bootstrap(ctx, empty, tc.slug)
			if err != nil || again.Next == nil || again.Next.ID != zero.Next.ID || again.AvailableUnits != 0 {
				t.Fatal("insufficient wager consumed commitment", err)
			}
		})
	}
	if err := owner.WithTx(ctx, func(tx pgx.Tx) error {
		var rounds, ledger int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM games.game_rounds WHERE newapi_user_id IN($1,$2)`, user, empty).Scan(&rounds); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM economy.wallet_ledger WHERE newapi_user_id IN($1,$2) AND entry_type IN('GAME_WAGER','GAME_PAYOUT')`, user, empty).Scan(&ledger); err != nil {
			return err
		}
		if rounds != 0 || ledger != 0 {
			t.Errorf("rejected requests left %d rounds and %d game ledger entries", rounds, ledger)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
