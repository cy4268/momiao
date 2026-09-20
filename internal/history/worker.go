package history

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Worker struct{ pool *pgxpool.Pool }

type Progress struct {
	Busy             bool       `json:"busy"`
	NewSources       int        `json:"new_sources"`
	MissingSources   int        `json:"missing_sources"`
	RefreshedSources int        `json:"refreshed_sources"`
	WrittenRows      int        `json:"written_rows"`
	SourceTimestamp  *time.Time `json:"source_timestamp"`
	SourceID         *string    `json:"source_id"`
}

func NewWorker(pool *pgxpool.Pool) *Worker { return &Worker{pool: pool} }

func (w *Worker) Step(ctx context.Context, source Source, batch int) (Progress, error) {
	var progress Progress
	if batch == 0 {
		batch = 100
	}
	if !validSource(source) || batch < 1 || batch > 200 {
		return progress, ErrQuery
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, err := w.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return progress, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SET LOCAL statement_timeout='2s'"); err != nil {
		return progress, err
	}
	var raw []byte
	if err = tx.QueryRow(ctx, "SELECT games.history_ingest_batch($1,$2)", string(source), batch).Scan(&raw); err != nil {
		return progress, err
	}
	if err = json.Unmarshal(raw, &progress); err != nil {
		return progress, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Progress{}, err
	}
	return progress, nil
}

// Run uses a local five-second default, not a product latency guarantee.
// Every cycle serves all three sources; errors and cancellation reach the caller.
func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		for _, source := range []Source{Round, Session, Hand, RouletteRound} {
			if _, err := w.Step(ctx, source, 100); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
