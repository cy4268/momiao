package platform

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NativeQuotaMigration is run only by an explicit operator/test, never at startup.
//
//go:embed native_quota.sql
var NativeQuotaMigration string

type NativeQuota struct{ pool *pgxpool.Pool }

type NativeQuotaObserver interface {
	ReadNativeQuota(context.Context, int64) (NativeQuotaSnapshot, error)
	QueryQuotaOperation(context.Context, string, int64) (NativeQuotaReceipt, error)
}
type NativeQuotaOperator interface {
	NativeQuotaObserver
	ApplyRawQuotaDelta(context.Context, string, int64, int64) (NativeQuotaReceipt, error)
	Credit(context.Context, string, int64, int64) (NativeQuotaReceipt, error)
}

type NativeQuotaSnapshot struct {
	UserID     int64     `json:"user_id,string"`
	RawQuota   int64     `json:"raw_quota,string"`
	Amount     string    `json:"amount"`
	Enabled    bool      `json:"account_enabled"`
	Result     string    `json:"result"`
	ObservedAt time.Time `json:"observed_at"`
}
type NativeQuotaReceipt struct {
	ID            string    `json:"operation_id"`
	UserID        int64     `json:"user_id,string"`
	Amount        int64     `json:"-"` // positive-credit compatibility
	DeltaRawQuota int64     `json:"delta_raw_quota,string"`
	Before        *int64    `json:"before_raw_quota,string"`
	After         *int64    `json:"after_raw_quota,string"`
	Result        string    `json:"result"`
	CreatedAt     time.Time `json:"created_at"`
}

func OpenNativeQuota(ctx context.Context, dsn string) (*NativeQuota, error) {
	if dsn == "" {
		return nil, ErrInvalidDatabaseURL
	}
	p, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return &NativeQuota{p}, nil
}
func (n *NativeQuota) Close() { n.pool.Close() }
func (n *NativeQuota) ReadNativeQuota(ctx context.Context, user int64) (NativeQuotaSnapshot, error) {
	var v NativeQuotaSnapshot
	if user <= 0 || user > 1<<31-1 {
		return v, ErrInvalidMutation
	}
	err := n.pool.QueryRow(ctx, `SELECT q.user_id,q.quota,q.enabled,clock_timestamp() FROM momiao_quota.read_quota($1) q`, user).
		Scan(&v.UserID, &v.RawQuota, &v.Enabled, &v.ObservedAt)
	if err == nil {
		if v.UserID != user || v.RawQuota < 0 || v.RawQuota > 1<<31-1 || v.ObservedAt.IsZero() {
			return NativeQuotaSnapshot{}, ErrNativeQuotaDependency
		}
		v.Result = "APPLIED"
		v.ObservedAt = v.ObservedAt.UTC()
	}
	v.Amount = FormatAmount(v.RawQuota)
	return v, err
}
func scanNativeQuota(row pgx.Row) (NativeQuotaReceipt, error) {
	var v NativeQuotaReceipt
	err := row.Scan(&v.ID, &v.UserID, &v.Amount, &v.Before, &v.After, &v.Result)
	v.DeltaRawQuota = v.Amount
	return v, err
}

const quotaReceiptColumns = `operation_id::text,newapi_user_id,amount_units,before_quota,after_quota,result`

func (n *NativeQuota) Credit(ctx context.Context, id string, user, amount int64) (NativeQuotaReceipt, error) {
	if !ValidOperationKey(id) || user <= 0 || amount <= 0 || amount > 9007199254740991 {
		return NativeQuotaReceipt{}, ErrInvalidMutation
	}
	// Always query the original receipt first. Concurrent create is also deduplicated in target SQL.
	v, err := scanNativeQuota(n.pool.QueryRow(ctx, `SELECT `+quotaReceiptColumns+` FROM momiao_quota.query_operation($1,$2)`, id, user))
	if errors.Is(err, pgx.ErrNoRows) {
		v, err = scanNativeQuota(n.pool.QueryRow(ctx, `SELECT `+quotaReceiptColumns+` FROM momiao_quota.credit($1,$2,$3)`, id, user, amount))
	}
	if err == nil && (v.ID != id || v.UserID != user || v.Amount != amount) {
		return NativeQuotaReceipt{}, ErrIdempotencyConflict
	}
	return v, err
}

// The direct-DSN adapter is an explicit legacy compatibility branch. Its SQL
// contract can credit only; negative deltas are reported as incompatible.
func (n *NativeQuota) ApplyRawQuotaDelta(ctx context.Context, id string, user, delta int64) (NativeQuotaReceipt, error) {
	if delta <= 0 {
		if !ValidOperationKey(id) || user <= 0 || delta == 0 {
			return NativeQuotaReceipt{}, ErrInvalidMutation
		}
		return NativeQuotaReceipt{ID: id, UserID: user, DeltaRawQuota: delta, Result: "SOURCE_INCOMPATIBLE"}, nil
	}
	return n.Credit(ctx, id, user, delta)
}

func (n *NativeQuota) QueryQuotaOperation(ctx context.Context, id string, user int64) (NativeQuotaReceipt, error) {
	if !ValidOperationKey(id) || user <= 0 {
		return NativeQuotaReceipt{}, ErrInvalidMutation
	}
	v, err := scanNativeQuota(n.pool.QueryRow(ctx, `SELECT `+quotaReceiptColumns+` FROM momiao_quota.query_operation($1,$2)`, id, user))
	if errors.Is(err, pgx.ErrNoRows) {
		return NativeQuotaReceipt{ID: id, UserID: user, Result: "NOT_APPLIED"}, nil
	}
	return v, err
}
