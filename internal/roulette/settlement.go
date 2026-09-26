package roulette

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

func lockWallets(ctx context.Context, tx pgx.Tx, players []participant) error {
	users := make([]int64, 0, len(players))
	for _, p := range players {
		users = append(users, p.User)
	}
	if err := platform.LockEconomyUsersInTx(ctx, tx, users...); err != nil {
		return err
	}
	players = slices.Clone(players)
	slices.SortFunc(players, func(a, b participant) int {
		if a.User < b.User {
			return -1
		}
		if a.User > b.User {
			return 1
		}
		return 0
	})
	for _, p := range players {
		var balance int64
		if e := tx.QueryRow(ctx, `SELECT balance_units FROM economy.wallet_balances WHERE newapi_user_id=$1 AND asset_type='AVAILABLE_CHIPS' FOR UPDATE`, p.User).Scan(&balance); e != nil {
			return e
		}
	}
	return nil
}
func fund(ctx context.Context, tx pgx.Tx, r *round, p participant, kind string, amount int64) error {
	if amount <= 0 {
		return ErrInvalidInput
	}
	delta := amount
	if kind == "ESCROW" {
		delta = -amount
	}
	biz := fmt.Sprintf("roulette/%s/%d/%d/%s", r.ID, p.User, p.Cycle, kind)
	entry, e := platform.ApplyInTx(ctx, tx, platform.Mutation{UserID: p.User, Asset: platform.AvailableChips, DeltaUnits: delta, BizType: "ROULETTE_FUNDING", BizID: biz, EntryType: "ROULETTE_" + kind, IdempotencyKey: biz})
	if e != nil {
		return e
	}
	id, e := newID()
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO roulette.funding(funding_id,round_id,newapi_user_id,ready_cycle,kind,amount_units,biz_id,transaction_id,ledger_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, r.ID, p.User, p.Cycle, kind, amount, biz, entry.TransactionID, entry.ID)
	return e
}
func makeOutcome(r *round, players []participant, eligible []int, reason string, refund bool) *outcome {
	eligible = slices.Clone(eligible)
	slices.Sort(eligible)
	eligible = slices.Compact(eligible)
	if len(eligible) == 0 {
		return nil
	}
	result := &outcome{Reason: reason, Eligible: eligible, Awards: []award{}, Refunds: refund}
	base, rest := r.Escrow/int64(len(eligible)), r.Escrow%int64(len(eligible))
	for i, seat := range eligible {
		index := slices.IndexFunc(players, func(p participant) bool { return p.Seat != nil && *p.Seat == seat && p.Ready })
		if index < 0 {
			return nil
		}
		amount := base
		if int64(i) < rest {
			amount++
		}
		if amount > 0 {
			result.Awards = append(result.Awards, award{players[index].User, seat, amount})
		}
	}
	return result
}
func (s *Service) cancel(ctx context.Context, tx pgx.Tx, r *round, players []participant, now time.Time, reason string) error {
	r.Outcome = &outcome{Reason: reason, Eligible: []int{}, Awards: []award{}, Refunds: true}
	for _, p := range players {
		if p.Ready && p.Seat != nil {
			r.Outcome.Eligible = append(r.Outcome.Eligible, *p.Seat)
			r.Outcome.Awards = append(r.Outcome.Awards, award{p.User, *p.Seat, r.Stake})
		}
	}
	r.State = "SETTLING"
	r.Deadline = nil
	return nil
}

func (s *Service) finishPending(ctx context.Context, id string) error {
	return s.store.WithTx(ctx, func(tx pgx.Tx) error {
		r, e := s.load(ctx, tx, id, true)
		if e != nil {
			if r != nil && errors.Is(e, ErrUnavailable) {
				return markReview(ctx, tx, r)
			}
			return e
		}
		if r.State != "SETTLING" {
			return nil
		}
		ps, e := participants(ctx, tx, id)
		if e != nil {
			return e
		}
		now, e := dbNow(ctx, tx)
		if e != nil {
			return e
		}
		if e = s.settle(ctx, tx, r, ps, now); e != nil {
			return e
		}
		return s.save(ctx, tx, r)
	})
}

func (s *Service) settle(ctx context.Context, tx pgx.Tx, r *round, players []participant, now time.Time) error {
	if r.State != "SETTLING" || r.Outcome == nil {
		return ErrActionInvalid
	}
	if e := lockWallets(ctx, tx, players); e != nil {
		return e
	}
	// Validate all awards before the first wallet mutation. A later overflow holds
	// the original outcome and escrow, never picks a different winner.
	total := int64(0)
	for _, a := range r.Outcome.Awards {
		if a.Amount <= 0 || total > math.MaxInt64-a.Amount {
			return ErrUnavailable
		}
		total += a.Amount
		var balance int64
		if e := tx.QueryRow(ctx, `SELECT balance_units FROM economy.wallet_balances WHERE newapi_user_id=$1 AND asset_type='AVAILABLE_CHIPS'`, a.User).Scan(&balance); e != nil {
			return e
		}
		if balance > math.MaxInt64-a.Amount {
			r.State = "NEEDS_REVIEW"
			r.Deadline = nil
			return nil
		}
	}
	if total != r.Escrow {
		r.State = "NEEDS_REVIEW"
		r.Deadline = nil
		return nil
	}
	// The nested transaction is a savepoint: a funding integrity failure must not
	// leave half a payout while recording NEEDS_REVIEW.
	payout, e := tx.Begin(ctx)
	if e != nil {
		return e
	}
	defer payout.Rollback(ctx)
	for _, a := range r.Outcome.Awards {
		i := slices.IndexFunc(players, func(p participant) bool { return p.User == a.User })
		if i < 0 {
			return ErrUnavailable
		}
		kind := "PAYOUT"
		if r.Outcome.Refunds {
			kind = "REFUND"
			if r.Outcome.Reason == "SYSTEM_VOID" {
				kind = "VOID_REFUND"
			}
		}
		if e = fund(ctx, payout, r, players[i], kind, a.Amount); e != nil {
			if errors.Is(e, platform.ErrBalanceOverflow) || errors.Is(e, platform.ErrIdempotencyConflict) {
				if e = payout.Rollback(ctx); e != nil {
					return e
				}
				r.State = "NEEDS_REVIEW"
				r.Deadline = nil
				return nil
			}
			return e
		}
	}
	if e = payout.Commit(ctx); e != nil {
		return e
	}
	r.Escrow = 0
	r.State = "FINISHED"
	if r.Outcome.Refunds {
		r.State = "CANCELLED"
	}
	r.Ended = &now
	r.Deadline = nil
	_, e = tx.Exec(ctx, `UPDATE roulette.participants SET active=false WHERE round_id=$1`, r.ID)
	return e
}
