package platform

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ImportNativeKeys copies live keys once; identity and all balances stay intact.
// Deleted key IDs still raise the sequence high-water mark, preventing ID reuse.
func (s *Store) ImportNativeKeys(ctx context.Context, source *pgx.Conn) (int64, error) {
	from, err := source.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return 0, err
	}
	defer from.Rollback(ctx)
	to, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer rollback(to)
	if _, err = to.Exec(ctx, `LOCK TABLE relay.api_keys IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return 0, err
	}
	var occupied bool
	if err = to.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM relay.api_keys)`).Scan(&occupied); err != nil {
		return 0, err
	}
	if occupied {
		return 0, fmt.Errorf("key import requires empty target table")
	}
	var high int64
	if err = from.QueryRow(ctx, `SELECT greatest(coalesce(max(id),0),coalesce(pg_sequence_last_value(pg_get_serial_sequence('tokens','id')::regclass),0)) FROM tokens`).Scan(&high); err != nil {
		return 0, err
	}
	rows, err := from.Query(ctx, `SELECT t.id,t.user_id,t.key,coalesce(t.name,''),t.status,coalesce(p.purpose,'UNCLASSIFIED'),
 CASE WHEN t.unlimited_quota THEN NULL ELSE greatest(t.remain_quota,0)::bigint+t.used_quota END,t.used_quota,
 CASE WHEN t.expired_time=-1 THEN NULL ELSE to_timestamp(t.expired_time) END,
 CASE WHEN t.model_limits_enabled THEN string_to_array(t.model_limits,',') ELSE NULL END,
 to_timestamp(t.created_time),to_timestamp(nullif(t.accessed_time,0))
 FROM public.tokens t LEFT JOIN public.momiao_key_purpose_mirrors p ON p.token_id=t.id AND p.user_id=t.user_id WHERE t.deleted_at IS NULL ORDER BY t.id`)
	if err != nil {
		return 0, err
	}
	values, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) ([]any, error) { return row.Values() })
	if err != nil {
		return 0, err
	}
	count, err := to.CopyFrom(ctx, pgx.Identifier{"relay", "api_keys"}, []string{"id", "user_id", "key", "name", "status", "purpose", "quota_limit", "used_quota", "expires_at", "models", "created_at", "accessed_at"}, pgx.CopyFromRows(values))
	if err != nil {
		return 0, err
	}
	if _, err = to.Exec(ctx, `SELECT setval(pg_get_serial_sequence('relay.api_keys','id'),$1+1,false)`, high); err != nil {
		return 0, err
	}
	if err = from.Commit(ctx); err != nil {
		return 0, err
	}
	return count, to.Commit(ctx)
}
