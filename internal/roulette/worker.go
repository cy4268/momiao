package roulette

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Service) due(ctx context.Context, tx pgx.Tx, r *round, players []participant, now time.Time) (bool, error) {
	if r.State == "SETTLING" {
		if e := s.settle(ctx, tx, r, players, now); e != nil {
			return false, e
		}
		return true, s.save(ctx, tx, r)
	}
	if r.Deadline == nil || now.Before(*r.Deadline) || (r.State != "WAITING" && r.State != "PLAYING") {
		return false, nil
	}
	var e error
	if r.State == "WAITING" {
		e = s.cancel(ctx, tx, r, players, now, "WAIT_EXPIRED")
	} else {
		a := Action{Kind: "TIMEOUT"}
		if r.GameDeadline != nil && !now.Before(*r.GameDeadline) {
			a.Kind = "GAME_LIMIT"
		}
		seat := r.ruleTurn()
		if seat < 0 {
			return false, ErrUnavailable
		}
		e = s.play(ctx, tx, r, players, &seat, nil, a, now)
	}
	if e != nil {
		return false, e
	}
	return true, s.save(ctx, tx, r)
}
func (s *Service) StepDue(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 50 {
		return 0, ErrInvalidInput
	}
	count := 0
	for range limit {
		found := false
		e := s.store.WithTx(ctx, func(tx pgx.Tx) error {
			var id string
			e := tx.QueryRow(ctx, `SELECT round_id::text FROM roulette.rounds WHERE state IN('WAITING','PLAYING','SETTLING') AND (deadline<=clock_timestamp() OR state='SETTLING') ORDER BY deadline NULLS FIRST,round_id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&id)
			if errors.Is(e, pgx.ErrNoRows) {
				return nil
			}
			if e != nil {
				return e
			}
			found = true
			r, e := s.load(ctx, tx, id, false)
			if e != nil {
				if r != nil && errors.Is(e, ErrUnavailable) {
					return markReview(ctx, tx, r)
				}
				return e
			}
			ps, e := participants(ctx, tx, id)
			if e != nil {
				return e
			}
			now, e := dbNow(ctx, tx)
			if e != nil {
				return e
			}
			_, e = s.due(ctx, tx, r, ps, now)
			return e
		})
		if e != nil {
			return count, e
		}
		if !found {
			break
		}
		count++
	}
	return count, nil
}
func (s *Service) RunWorker(ctx context.Context) {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			_, _ = s.StepDue(ctx, 50)
		}
	}
}
