package poker

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fundingCallbackTx struct {
	pgx.Tx
	child                 *fundingCallbackTx
	rolledBack, committed bool
	writes                int
}

func (t *fundingCallbackTx) Begin(context.Context) (pgx.Tx, error) { return t.child, nil }
func (t *fundingCallbackTx) Rollback(context.Context) error        { t.rolledBack = true; return nil }
func (t *fundingCallbackTx) Commit(context.Context) error          { t.committed = true; return nil }
func (t *fundingCallbackTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	t.writes++
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
func (t *fundingCallbackTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return fundingCallbackRow{}
}
func (t *fundingCallbackTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	// This callback-only case has no other table accounts to pre-lock.
	return fundingCallbackRow{}, nil
}

type fundingCallbackRow struct{ pgx.Rows }

func (fundingCallbackRow) Next() bool { return false }
func (fundingCallbackRow) Err() error { return nil }
func (fundingCallbackRow) Close()     {}

func (fundingCallbackRow) Scan(values ...any) error {
	*values[0].(*[]byte) = []byte(`{"status":"CONFIRMED"}`)
	return nil
}

func TestFundingCallbackFailureBoundary(t *testing.T) {
	infrastructure := errors.New("controlled infrastructure failure")
	for _, tc := range []struct {
		name    string
		failure error
		durable bool
	}{
		{"grant absent", ErrTableAccessRequired, true},
		{"lease absent", errLeaseLost, true},
		{"infrastructure unavailable", infrastructure, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sp := &fundingCallbackTx{}
			tx := &fundingCallbackTx{child: sp}
			r, err := applyFunding(context.Background(), tx, &tableRow{ID: "fixture", Version: 1}, "funding", "BUY_IN", func(context.Context, pgx.Tx) error { return tc.failure })
			if !sp.rolledBack || sp.committed {
				t.Fatal("failed effects were not rolled back")
			}
			if tc.durable {
				if err != nil || r.Status != "FAILED_NO_EFFECT" || tx.writes != 1 {
					t.Fatal("definite failure receipt missing")
				}
			} else if !errors.Is(err, infrastructure) || tx.writes != 0 || r.Status != "" {
				t.Fatal("infrastructure error became an immutable failure receipt")
			}
		})
	}
}
