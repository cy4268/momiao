package poker

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const passwordProfilesQuery = `SELECT NOT EXISTS (
 SELECT 1 FROM poker.table_access_credentials
 WHERE split_part(password_phc,'$',4) COLLATE "C"
       <> ALL(COALESCE($1::text[], ARRAY[]::text[]))
 OR password_phc COLLATE "C" !~ '^[$]argon2id[$]v=19[$]m=[1-9][0-9]*,t=[1-9][0-9]*,p=[1-9][0-9]*[$][A-Za-z0-9+/]{21}[AQgw][$][A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]$'
)`

func CheckPasswordProfiles(ctx context.Context, pool *pgxpool.Pool, runtime *PasswordRuntime) error {
	if err := passwordContextError(ctx); err != nil {
		return err
	}
	if pool == nil {
		return ErrPasswordUnavailable
	}
	profiles := make([]string, 0, 4)
	if runtime != nil {
		if runtime.slots == nil || len(runtime.profiles) == 0 || len(runtime.profiles) > 4 {
			return ErrPasswordConfig
		}
		for _, profile := range runtime.profiles {
			if !profile.valid() {
				return ErrPasswordConfig
			}
			profiles = append(profiles, strings.Split(profile.prefix(), "$")[3])
		}
	}
	opCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, err := pool.BeginTx(opCtx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return passwordProfileFailure(opCtx)
	}
	defer tx.Rollback(opCtx)
	if _, err = tx.Exec(opCtx, "SET LOCAL statement_timeout='2s'"); err != nil {
		return passwordProfileFailure(opCtx)
	}
	var compatible bool
	err = tx.QueryRow(opCtx, passwordProfilesQuery, profiles).Scan(&compatible)
	if ctxErr := passwordContextError(opCtx); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		return ErrPasswordUnavailable
	}
	if !compatible {
		return ErrPasswordConfig
	}
	if err = tx.Commit(opCtx); err != nil {
		return passwordProfileFailure(opCtx)
	}
	return passwordContextError(opCtx)
}

func passwordProfileFailure(ctx context.Context) error {
	if err := passwordContextError(ctx); err != nil {
		return err
	}
	return ErrPasswordUnavailable
}
