package poker

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

type StartupReport struct{ Loaded, Recovering, NeedsReview int }

func (s *Service) Start(ctx context.Context) (StartupReport, error) {
	s.startMu.Lock()
	defer s.startMu.Unlock()
	if s.started {
		return s.startupReport, nil
	}
	var report StartupReport
	cursor := "00000000-0000-0000-0000-000000000000"
	for {
		rows, err := s.opts.Pool.Query(ctx, "SELECT table_id::text FROM poker.tables WHERE lifecycle_state<>'CLOSED' AND table_id>$1::uuid ORDER BY table_id LIMIT 100", cursor)
		if err != nil {
			return report, err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return report, err
			}
			ids = append(ids, id)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return report, err
		}
		if len(ids) == 0 {
			break
		}
		for _, id := range ids {
			a, err := s.actor(ctx, id, false)
			if err != nil {
				return report, err
			}
			select {
			case <-a.ready:
			case <-ctx.Done():
				return report, ctx.Err()
			}
			if a.initErr != nil {
				return report, a.initErr
			}
			var status string
			if err = s.opts.Pool.QueryRow(ctx, "SELECT coalesce((SELECT state FROM poker.recovery_state WHERE table_id=$1),'NORMAL')", id).Scan(&status); err != nil {
				return report, err
			}
			report.Loaded++
			if status == "GRACE" || status == "RECOVERING" {
				report.Recovering++
			}
			if status == "NEEDS_REVIEW" {
				report.NeedsReview++
			}
		}
		cursor = ids[len(ids)-1]
	}
	s.started = true
	s.startupReport = report
	return report, nil
}

func (s *Service) recoverMutation(ctx context.Context, tx pgx.Tx, t *tableRow) (Receipt, error) {
	if t.HandID == "" {
		tag, err := tx.Exec(ctx, "UPDATE poker.seats SET connected=false,disconnected_since=coalesce(disconnected_since,$2) WHERE table_id=$1 AND session_id IS NOT NULL AND connected", t.ID, t.Now)
		if err != nil {
			return Receipt{}, err
		}
		if tag.RowsAffected() == 0 {
			return Receipt{Status: "NOOP"}, nil
		}
		return Receipt{TableID: t.ID, Status: "RECOVERED"}, nil
	}
	var state string
	if err := tx.QueryRow(ctx, "SELECT state FROM poker.hands WHERE hand_id=$1", t.HandID).Scan(&state); err != nil {
		return Receipt{}, err
	}
	if state == "COMMITTED" {
		return s.dealCommitted(ctx, tx, t)
	}
	if state == "SETTLED" {
		tag, err := tx.Exec(ctx, "UPDATE poker.seats SET connected=false,disconnected_since=coalesce(disconnected_since,$2) WHERE table_id=$1 AND session_id IS NOT NULL AND connected", t.ID, t.Now)
		if err != nil {
			return Receipt{}, err
		}
		if tag.RowsAffected() == 0 {
			return Receipt{Status: "NOOP"}, nil
		}
		return Receipt{TableID: t.ID, Status: "RECOVERED"}, nil
	}
	e, err := s.restoreEngine(ctx, tx, t.HandID)
	if err != nil {
		return Receipt{}, err
	}
	for _, p := range e.State().Players {
		if _, err = e.SetConnected(p.SeatNo, false, t.Now); err != nil {
			return Receipt{}, err
		}
	}
	if _, err = e.BeginRecovery(t.Now); err != nil {
		return Receipt{}, err
	}
	if err = s.persistEngine(ctx, tx, t, e); err != nil {
		return Receipt{}, err
	}
	if _, err = tx.Exec(ctx, "UPDATE poker.tables SET lifecycle_state='RECOVERING' WHERE table_id=$1", t.ID); err != nil {
		return Receipt{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO poker.recovery_state(table_id,hand_id,runtime_epoch,state,started_at,grace_until,previous_table_state,reason_code,detected_at) VALUES($1,$2,$3,'GRACE',$4,$5,$6,'SERVICE_RESTART',$4) ON CONFLICT(table_id) DO UPDATE SET hand_id=excluded.hand_id,runtime_epoch=excluded.runtime_epoch,state='GRACE',started_at=excluded.started_at,grace_until=excluded.grace_until,previous_table_state=excluded.previous_table_state,reason_code=excluded.reason_code,detected_at=excluded.detected_at,resumed_at=NULL`, t.ID, t.HandID, t.Epoch, t.Now, e.State().RecoveryUntil, t.recoveryPreviousState())
	return Receipt{TableID: t.ID, HandID: t.HandID, Status: "RECOVERING"}, err
}
func (s *Service) isolateHand(ctx context.Context, tx pgx.Tx, t *tableRow) (Receipt, error) {
	if t.HandID == "" {
		return Receipt{}, ErrCorruptSnapshot
	}
	if _, err := tx.Exec(ctx, "UPDATE poker.tables SET lifecycle_state='RECOVERING' WHERE table_id=$1", t.ID); err != nil {
		return Receipt{}, err
	}
	_, err := tx.Exec(ctx, `INSERT INTO poker.recovery_state(table_id,hand_id,runtime_epoch,state,previous_table_state,reason_code,detected_at) VALUES($1,$2,$3,'NEEDS_REVIEW',$4,'CORRUPT_SNAPSHOT',$5) ON CONFLICT(table_id) DO UPDATE SET hand_id=excluded.hand_id,runtime_epoch=excluded.runtime_epoch,state='NEEDS_REVIEW',previous_table_state=excluded.previous_table_state,reason_code=excluded.reason_code,detected_at=excluded.detected_at,started_at=NULL,grace_until=NULL,resumed_at=NULL`, t.ID, t.HandID, t.Epoch, t.recoveryPreviousState(), t.Now)
	if err != nil {
		return Receipt{}, err
	}
	_, hash := hashes("poker.isolate", struct {
		Table, Hand string
		Epoch       uint64
	}{t.ID, t.HandID, t.Epoch})
	_, err = tx.Exec(ctx, `INSERT INTO poker.audit_events(event_id,table_id,actor_kind,event_type,request_hash,details) VALUES($1,$2,'SYSTEM','POKER_NEEDS_REVIEW',$3,'{"reason_code":"CORRUPT_SNAPSHOT"}')`, uuid(), t.ID, hash[:])
	return Receipt{TableID: t.ID, HandID: t.HandID, Status: "NEEDS_REVIEW"}, err
}

// Recover is an explicit INTERNAL replacement hook. Normal startup uses Start;
// normal table commands await hydration instead of invoking this ad hoc.
func (s *Service) Recover(ctx context.Context, table string) error {
	a, err := s.actor(ctx, table, true)
	if err != nil {
		return err
	}
	select {
	case <-a.ready:
	case <-ctx.Done():
		return ctx.Err()
	}
	if errors.Is(a.initErr, ErrNeedsReview) {
		return nil
	}
	return a.initErr
}
