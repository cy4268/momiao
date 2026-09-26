package games

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"

	bj "github.com/cy4268/momiao/internal/games/blackjack"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

func (s *Service) RunJobs(ctx context.Context) (int, error) {
	type job struct{ round, kind string }
	pending := []job{}
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT round_id::text,job_type FROM games.round_jobs WHERE status='PENDING' AND run_at<=clock_timestamp() ORDER BY run_at,job_id LIMIT 16`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var j job
			if err = rows.Scan(&j.round, &j.kind); err != nil {
				return err
			}
			pending = append(pending, j)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}
	done := 0
	for _, j := range pending {
		// A zero argument means obtain fresh DB time after the round/job locks. The
		// internal nonzero seam is only for an elapsed-time integration test.
		err = s.processBlackjackJob(ctx, j.round, j.kind, time.Time{})
		if err != nil {
			// Rollback leaves original cards/actions intact. Retry is a durable job
			// update, not a new deal; log only a stable error category, never authority.
			_ = s.store.WithTx(ctx, func(tx pgx.Tx) error {
				_, e := tx.Exec(ctx, `UPDATE games.round_jobs SET attempts=attempts+1,run_at=clock_timestamp()+INTERVAL '30 seconds',last_error='RETRY_REQUIRED',updated_at=clock_timestamp() WHERE round_id=$1 AND job_type=$2 AND status='PENDING'`, j.round, j.kind)
				return e
			})
			if ctx.Err() != nil {
				return done, ctx.Err()
			}
			continue
		}
		done++
	}
	return done, nil
}

func (s *Service) processBlackjackJob(ctx context.Context, id, kind string, now time.Time) error {
	return s.store.WithTx(ctx, func(tx pgx.Tx) error {
		var user int64
		err := tx.QueryRow(ctx, `SELECT newapi_user_id FROM games.game_rounds WHERE round_id=$1 AND game_slug='blackjack'`, id).Scan(&user)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err = lockUserGame(ctx, tx, user, "blackjack"); err != nil {
			return err
		}
		// Every path locks user/game, round, job, then wallet, including concurrent
		// workers. No lease survives a process crash; PostgreSQL owns the work claim.
		if _, err = tx.Exec(ctx, `SELECT round_id FROM games.game_rounds WHERE round_id=$1 FOR UPDATE`, id); err != nil {
			return err
		}
		var status string
		var runAt time.Time
		err = tx.QueryRow(ctx, `SELECT status,run_at FROM games.round_jobs WHERE round_id=$1 AND job_type=$2 FOR UPDATE`, id, kind).Scan(&status, &runAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if status != "PENDING" {
			return nil
		}
		if now.IsZero() {
			now, err = databaseNow(ctx, tx)
			if err != nil {
				return err
			}
		}
		if now.Before(runAt) {
			return nil
		}
		r, err := s.readRound(ctx, tx, user, id)
		if err != nil {
			return err
		}
		if r.RecoveryState != "NORMAL" {
			return nil
		}
		state, shoe, seed, fair, err := s.recoverBlackjack(ctx, tx, user, r)
		if err != nil {
			return err
		}
		if kind == "GAME_ROUND_RECOVERY" || r.State == "SETTLED" {
			_, err = tx.Exec(ctx, `UPDATE games.round_jobs SET status='DONE',last_error=NULL,attempts=attempts+1,updated_at=clock_timestamp() WHERE round_id=$1 AND job_type=$2`, id, kind)
			return err
		}
		if kind != "BLACKJACK_AUTO_RESOLVE" {
			return ErrInvalidInput
		}
		if now.Before(state.AutoResolveAt) {
			_, err = tx.Exec(ctx, `UPDATE games.round_jobs SET run_at=$2,updated_at=clock_timestamp() WHERE round_id=$1 AND job_type='BLACKJACK_AUTO_RESOLVE'`, id, state.AutoResolveAt)
			return err
		}
		available, seq, version, err := lockedChips(ctx, tx, user)
		if err != nil {
			return err
		}
		if available > math.MaxInt64-state.InitialWagerUnits*17 || seq == math.MaxInt64 || version == math.MaxInt64 {
			return platform.ErrBalanceOverflow
		}
		transition, err := bj.AutoResolve(state, shoe, now, func(string) (string, error) { return newUUID() })
		if err != nil {
			return err
		}
		if transition.State.Phase == bj.Settled {
			bonus, fairErr := blackjackFairReturn(seed, fair, r.Ruleset, transition.State.InitialWagerUnits)
			if fairErr != nil {
				return fairErr
			}
			if fairErr = addBlackjackFairReturn(&transition.State, bonus); fairErr != nil {
				return fairErr
			}
		}
		if err = persistBlackjackState(ctx, tx, id, transition.State); err != nil {
			return err
		}
		if err = s.finishBlackjackTransition(ctx, tx, user, &r, transition.State, seed, available, now); err != nil {
			return err
		}
		response, err := json.Marshal(r)
		if err != nil {
			return err
		}
		for _, record := range transition.State.Actions[len(state.Actions):] {
			a := record.Action
			input := BlackjackActionInput{ActionID: a.ID, ActionType: string(a.Type), HandID: a.HandID, ExpectedVersion: strconv.FormatInt(a.ExpectedVersion, 10)}
			hash := blackjackActionHash(user, id, input)
			_, err = tx.Exec(ctx, `INSERT INTO games.round_actions(action_id,round_id,newapi_user_id,action_sequence,action_type,additional_stake_units,system_action,created_at,expected_round_version,hand_id,request_hash,original_response) VALUES($1,$2,$3,$4,'SYSTEM_AUTO_STAND',0,TRUE,$5,$6,$7,$8,$9)`, a.ID, id, user, record.Sequence, record.At, a.ExpectedVersion, a.HandID, hash[:], response)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// RunWorker is bounded on shutdown and quiet during ordinary unchanged state.
// Accepted rounds keep progressing even when the new-deal gate is in maintenance.
func (s *Service) RunWorker(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		call, cancel := context.WithTimeout(ctx, 8*time.Second)
		_, _ = s.RunJobs(call)
		cancel()
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
