package platform

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrBootstrapInvalid        = errors.New("BOOTSTRAP_INVALID_INPUT")
	ErrBootstrapClosed         = errors.New("BOOTSTRAP_ALREADY_CLOSED")
	ErrBootstrapFailed         = errors.New("BOOTSTRAP_TRANSACTION_FAILED")
	ErrBootstrapOutcomeUnknown = errors.New("BOOTSTRAP_COMMIT_OUTCOME_UNKNOWN")
	bootstrapLabel             = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
)

type BootstrapInput struct {
	Environment   string
	UserID        int64
	Username      string
	ReleaseBuild  string
	ExpectedEmpty bool
}
type BootstrapReceipt struct {
	PrincipalID string    `json:"admin_principal_id"`
	OperationID string    `json:"operation_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// Bootstrap is a deployment-only writer, not a runtime account-management API.
// Call only after the pinned source has verified this exact live target. The
// database credential receives EXECUTE on the narrow function, not table DML.
// There is intentionally no retry, force, reset, replacement, or DDL path.
func (s *Store) Bootstrap(ctx context.Context, input BootstrapInput) (BootstrapReceipt, error) {
	var result BootstrapReceipt
	if (input.Environment != "DEVELOPMENT" && input.Environment != "STAGING" && input.Environment != "PRODUCTION") || input.UserID <= 0 || !validLabel(input.Username) || !bootstrapLabel.MatchString(input.ReleaseBuild) || !input.ExpectedEmpty {
		return result, ErrBootstrapInvalid
	}
	principal, err := uuidV7()
	if err != nil {
		return result, ErrBootstrapFailed
	}
	history, err := uuidV7()
	if err != nil {
		return result, ErrBootstrapFailed
	}
	operation, err := uuidV7()
	if err != nil {
		return result, ErrBootstrapFailed
	}
	// READ COMMITTED is required: the post-guard count must see the winner of a
	// concurrent bootstrap after the row/advisory lock wait.
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return result, ErrBootstrapFailed
	}
	defer rollback(tx)
	err = tx.QueryRow(ctx, `SELECT admin_principal_id::text,operation_id::text,created_at FROM ops.bootstrap_super_admin($1,$2,$3,$4,$5,$6,$7,$8)`, input.Environment, input.UserID, input.Username, input.ReleaseBuild, input.ExpectedEmpty, principal, history, operation).Scan(&result.PrincipalID, &result.OperationID, &result.CreatedAt)
	if err != nil {
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && pgerr.Code == "P0001" && pgerr.Message == "BOOTSTRAP_ALREADY_CLOSED" {
			return BootstrapReceipt{}, ErrBootstrapClosed
		}
		return BootstrapReceipt{}, ErrBootstrapFailed
	}
	if err = tx.Commit(ctx); err != nil {
		return BootstrapReceipt{}, ErrBootstrapOutcomeUnknown
	}
	return result, nil
}

// BootstrapDatabaseMatches is read-only and compares against the independently
// mounted deployment manifest before any account/principal transaction begins.
func (s *Store) BootstrapDatabaseMatches(ctx context.Context, expected string) bool {
	var name string
	return expected != "" && s.pool.QueryRow(ctx, "SELECT current_database()").Scan(&name) == nil && name == expected
}
