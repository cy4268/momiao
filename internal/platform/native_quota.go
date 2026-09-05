package platform

import (
	"context"
	_ "embed"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NativeQuotaMigration is run only by an explicit operator/test, never at startup.
//
//go:embed native_quota.sql
var NativeQuotaMigration string

type NativeQuota struct{ pool *pgxpool.Pool }
type NativeQuotaSnapshot struct {
	UserID   int64  `json:"user_id,string"`
	RawQuota int64  `json:"raw_quota,string"`
	Amount   string `json:"amount"`
	Enabled  bool   `json:"enabled"`
}
type NativeQuotaReceipt struct {
	ID            string
	UserID        int64
	Amount        int64
	Before, After *int64
	Result        string
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
	if user <= 0 {
		return v, ErrInvalidMutation
	}
	err := n.pool.QueryRow(ctx, `SELECT user_id,quota,enabled FROM momiao_quota.read_quota($1)`, user).Scan(&v.UserID, &v.RawQuota, &v.Enabled)
	v.Amount = FormatAmount(v.RawQuota)
	return v, err
}
func scanNativeQuota(row pgx.Row) (NativeQuotaReceipt, error) {
	var v NativeQuotaReceipt
	err := row.Scan(&v.ID, &v.UserID, &v.Amount, &v.Before, &v.After, &v.Result)
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
