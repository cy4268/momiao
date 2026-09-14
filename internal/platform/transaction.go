package platform

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// WithTx lets an in-process business service use the same database transaction
// as its wallet entries. The callback must not commit, retry, or retain tx.
func (s *Store) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ApplyInTx retains all wallet locking, immutable ledger and idempotency rules.
// The caller owns the transaction and supplies a stable business identity.
func ApplyInTx(ctx context.Context, tx pgx.Tx, mutation Mutation) (LedgerEntry, error) {
	return applyInTx(ctx, tx, mutation)
}
