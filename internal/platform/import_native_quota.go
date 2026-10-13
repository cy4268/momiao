package platform

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type QuotaImportResult struct {
	Users          int64 `json:"users"`
	ZeroQuotaUsers int64 `json:"zero_quota_users"`
	QuotaUnits     int64 `json:"quota_units"`
}

// ImportNativeQuota runs only after the old writers are stopped and drained.
// Native stays read-only; account initialization and all credits commit together.
func (s *Store) ImportNativeQuota(ctx context.Context, source *pgx.Conn) (QuotaImportResult, error) {
	from, err := source.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return QuotaImportResult{}, err
	}
	defer from.Rollback(ctx)
	to, err := s.pool.Begin(ctx)
	if err != nil {
		return QuotaImportResult{}, err
	}
	defer rollback(to)
	result, err := mergeNativeQuota(ctx, from, to)
	if err != nil {
		return QuotaImportResult{}, err
	}
	if err = from.Commit(ctx); err != nil {
		return QuotaImportResult{}, err
	}
	return result, to.Commit(ctx)
}

func mergeNativeQuota(ctx context.Context, from, to pgx.Tx) (QuotaImportResult, error) {
	result := QuotaImportResult{}
	if _, err := to.Exec(ctx, `LOCK TABLE economy.quota_transfers, economy.api_chips_exchanges IN SHARE MODE`); err != nil {
		return result, err
	}
	var pending bool
	err := to.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM economy.quota_transfers WHERE status IN('PENDING','NEEDS_REVIEW'))
 OR EXISTS(SELECT 1 FROM economy.api_chips_exchanges WHERE status IN('PENDING','SOURCE_DEBITED','COMPENSATING','NEEDS_REVIEW'))`).Scan(&pending)
	if err != nil {
		return result, err
	}
	if pending {
		return result, fmt.Errorf("native quota merge requires resolved transfers and API exchanges")
	}
	if err = from.QueryRow(ctx, `SELECT count(*) FILTER(WHERE coalesce(quota,0)=0),coalesce(sum(quota) FILTER(WHERE quota>0),0) FROM public.users`).Scan(&result.ZeroQuotaUsers, &result.QuotaUnits); err != nil {
		return result, err
	}
	rows, err := from.Query(ctx, `SELECT id,quota FROM public.users WHERE quota<>0 ORDER BY id`)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var user, quota int64
		if err = rows.Scan(&user, &quota); err != nil {
			return result, err
		}
		if quota < 0 {
			return result, fmt.Errorf("negative native quota for user %d", user)
		}
		// Same zero-wallet initialization as EnsureAccount, inside this import's
		// transaction. Native-only accounts also keep their remaining credit.
		if _, err = to.Exec(ctx, `INSERT INTO identity.account_refs(newapi_user_id) VALUES($1) ON CONFLICT DO NOTHING`, user); err != nil {
			return result, err
		}
		if _, err = to.Exec(ctx, `INSERT INTO economy.wallet_balances(newapi_user_id,asset_type) VALUES($1,'RESERVE_API_CREDIT'),($1,'AVAILABLE_CHIPS') ON CONFLICT DO NOTHING`, user); err != nil {
			return result, err
		}
		id := fmt.Sprintf("native-merge:%d", user)
		_, err = applyInTx(ctx, to, Mutation{
			UserID: user, Asset: ReserveAPICredit, DeltaUnits: quota,
			BizType: "NATIVE_QUOTA_MERGE", BizID: id, EntryType: "NATIVE_QUOTA_MERGE",
			// Pad only the transport key to satisfy the existing wallet minimum.
			IdempotencyKey: fmt.Sprintf("native-merge:%020d", user),
		})
		if err != nil {
			return result, err
		}
		result.Users++
	}
	return result, rows.Err()
}
