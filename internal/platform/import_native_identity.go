package platform

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type IdentityImportResult struct {
	Users       int64 `json:"users"`
	TwoFA       int64 `json:"two_fas"`
	BackupCodes int64 `json:"two_fa_backup_codes"`
}

// ImportNativeIdentity copies official credential formats without rehashing.
// The source is read-only; all three target tables commit together. Import into
// empty identity tables: any existing row aborts the whole transaction.
// Wallets, user IDs, native rows and existing sessions are not changed here.
func (s *Store) ImportNativeIdentity(ctx context.Context, source *pgx.Conn) (IdentityImportResult, error) {
	result := IdentityImportResult{}
	from, err := source.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer from.Rollback(ctx)
	to, err := s.pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer rollback(to)
	if _, err = to.Exec(ctx, `LOCK TABLE identity.users, identity.two_fas, identity.two_fa_backup_codes IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return result, err
	}
	var occupied bool
	err = to.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM identity.users) OR EXISTS(SELECT 1 FROM identity.two_fas) OR EXISTS(SELECT 1 FROM identity.two_fa_backup_codes)`).Scan(&occupied)
	if err != nil {
		return result, err
	}
	if occupied {
		return result, fmt.Errorf("identity import requires empty target tables")
	}
	var sourceUserHighWater int64
	err = from.QueryRow(ctx, `SELECT coalesce(pg_sequence_last_value(pg_get_serial_sequence('public.users','id')::regclass),0)`).Scan(&sourceUserHighWater)
	if err != nil {
		return result, err
	}
	for _, table := range []struct {
		name    string
		columns []string
		query   string
		count   *int64
	}{
		{"users", []string{"id", "username", "password_hash", "display_name", "email", "role", "status", "discord_id", "group_name", "created_at", "last_login_at", "deleted_at"},
			`SELECT id,username,coalesce(password,''),coalesce(display_name,''),coalesce(email,''),role,status,nullif(discord_id,''),"group",
				to_timestamp(created_at),to_timestamp(nullif(last_login_at,0)),deleted_at FROM public.users ORDER BY id`, &result.Users},
		{"two_fas", []string{"id", "user_id", "secret", "is_enabled", "failed_attempts", "locked_until", "last_used_at", "created_at", "updated_at", "deleted_at"},
			`SELECT id,user_id,secret,is_enabled,failed_attempts,locked_until,last_used_at,created_at,updated_at,deleted_at FROM public.two_fas ORDER BY id`, &result.TwoFA},
		{"two_fa_backup_codes", []string{"id", "user_id", "code_hash", "is_used", "used_at", "created_at", "deleted_at"},
			`SELECT id,user_id,code_hash,is_used,used_at,created_at,deleted_at FROM public.two_fa_backup_codes ORDER BY id`, &result.BackupCodes},
	} {
		rows, err := from.Query(ctx, table.query)
		if err != nil {
			return IdentityImportResult{}, err
		}
		values, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) ([]any, error) { return row.Values() })
		if err != nil {
			return IdentityImportResult{}, err
		}
		*table.count, err = to.CopyFrom(ctx, pgx.Identifier{"identity", table.name}, table.columns, pgx.CopyFromRows(values))
		if err != nil {
			return IdentityImportResult{}, fmt.Errorf("import %s: %w", table.name, err)
		}
	}
	// Deleted native users can still own historical wallet/account references.
	// Never allocate those IDs to a new user after the import.
	if _, err = to.Exec(ctx, `SELECT setval(pg_get_serial_sequence('identity.users','id'),
		greatest($1::bigint,coalesce((SELECT max(id) FROM identity.users),0),
		coalesce((SELECT max(newapi_user_id) FROM identity.account_refs),0))+1,false)`, sourceUserHighWater); err != nil {
		return IdentityImportResult{}, err
	}
	for _, table := range []string{"two_fas", "two_fa_backup_codes"} {
		if _, err = to.Exec(ctx, fmt.Sprintf(
			`SELECT setval(pg_get_serial_sequence('identity.%s','id'),coalesce(max(id),0)+1,false) FROM identity.%s`, table, table)); err != nil {
			return IdentityImportResult{}, err
		}
	}
	if err = from.Commit(ctx); err != nil {
		return IdentityImportResult{}, err
	}
	if err = to.Commit(ctx); err != nil {
		return IdentityImportResult{}, err
	}
	return result, nil
}
