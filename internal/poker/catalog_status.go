package poker

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// CatalogRuntime exposes discovery readiness, never an identity or admission.
func (s *Service) CatalogRuntime(ctx context.Context) (string, error) {
	const unavailable = "TEMPORARILY_UNAVAILABLE"
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return unavailable, ErrClosed
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, err := s.opts.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return unavailable, err
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, "SET LOCAL statement_timeout='2s'"); err != nil {
		return unavailable, err
	}
	facts, err := readAdmissionFacts(ctx, tx, approvedRulesetVersion)
	if err != nil {
		return unavailable, err
	}
	presets, err := readLobbyPresets(ctx, tx)
	if err != nil {
		return unavailable, err
	}
	state := unavailable
	if facts.ready && len(presets) > 0 {
		state = "PLAY"
		if len(facts.scopes) > 0 {
			state = "MAINTENANCE"
		}
	}
	return state, tx.Commit(ctx)
}
