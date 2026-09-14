package platform

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type UnifiedAssets struct {
	UserID                     int64     `json:"user_id,string"`
	ActiveQuotaUnits           int64     `json:"active_quota_units,string"`
	ReserveUnits               int64     `json:"reserve_units,string"`
	AvailableChipsUnits        int64     `json:"available_chips_units,string"`
	PokerStackUnits            int64     `json:"poker_stack_units,string"`
	PokerPotUnits              int64     `json:"poker_pot_units,string"`
	QuotaTransferInFlightUnits int64     `json:"quota_transfer_in_flight_units,string"`
	TotalUnits                 int64     `json:"total_units,string"`
	TotalAmount                string    `json:"total_amount"`
	ObservedAt                 time.Time `json:"observed_at"`
	NativeObservedAt           time.Time `json:"native_observed_at"`
}

type UnifiedAssetReader struct {
	store  *Store
	native NativeQuotaOperator
}

func NewUnifiedAssetReader(store *Store, native NativeQuotaOperator) (*UnifiedAssetReader, error) {
	if store == nil || native == nil {
		return nil, ErrInvalidMutation
	}
	return &UnifiedAssetReader{store: store, native: native}, nil
}

func checkedAssetAdd(total *int64, value int64) error {
	if value < 0 || *total > math.MaxInt64-value {
		return ErrBalanceOverflow
	}
	*total += value
	return nil
}

// ReadUnifiedAssets serializes with every Platform Reserve-to-Active transfer.
// A definitely absent Native effect is counted once as in-flight; an applied
// effect is already in Active and is not counted again. Poker and wallet values
// come from one repeatable-read snapshot. The runtime only executes a narrow
// aggregate function and has no direct Poker table read privilege.
func (reader *UnifiedAssetReader) ReadUnifiedAssets(ctx context.Context, user int64) (UnifiedAssets, error) {
	if reader == nil || reader.store == nil || reader.native == nil || !validateQuotaUser(user) {
		return UnifiedAssets{UserID: user}, ErrInvalidMutation
	}
	tx, err := reader.store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return UnifiedAssets{UserID: user}, err
	}
	defer rollback(tx)
	result, err := reader.readUnifiedAssetsInTx(ctx, tx, user)
	if err != nil {
		return result, err
	}
	if err = tx.Commit(ctx); err != nil {
		return UnifiedAssets{}, err
	}
	return result, nil
}

// readUnifiedAssetsInTx lets eligibility checks and their local wallet effect
// share one transaction. Callers must use READ COMMITTED when they also call
// RequireNoMaintenance; the wallet row locks are the shared Poker funding
// fence, while the Poker function returns stack and pot in one SQL snapshot.
func (reader *UnifiedAssetReader) readUnifiedAssetsInTx(ctx context.Context, tx pgx.Tx, user int64) (UnifiedAssets, error) {
	result := UnifiedAssets{UserID: user}
	var err error
	if reader == nil || reader.store == nil || reader.native == nil || tx == nil || !validateQuotaUser(user) {
		return result, ErrInvalidMutation
	}
	if err = lockIdentity(ctx, tx, "quota-transfer-user", user); err != nil {
		return result, err
	}
	rows, err := tx.Query(ctx, `SELECT asset_type,balance_units FROM economy.wallet_balances
 WHERE newapi_user_id=$1 AND asset_type IN('RESERVE_API_CREDIT','AVAILABLE_CHIPS') ORDER BY asset_type FOR UPDATE`, user)
	if err != nil {
		return result, err
	}
	count := 0
	for rows.Next() {
		var asset Asset
		var amount int64
		if err = rows.Scan(&asset, &amount); err != nil {
			rows.Close()
			return result, err
		}
		if asset == ReserveAPICredit {
			result.ReserveUnits = amount
		} else if asset == AvailableChips {
			result.AvailableChipsUnits = amount
		} else {
			rows.Close()
			return result, ErrInvalidMutation
		}
		count++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if count != 2 {
		return result, ErrWalletNotFound
	}
	var pokerHealthy bool
	if err = tx.QueryRow(ctx, "SELECT stack_units,pot_units,healthy FROM economy.unified_poker_assets_read($1)", user).
		Scan(&result.PokerStackUnits, &result.PokerPotUnits, &pokerHealthy); err != nil {
		return result, err
	}
	if !pokerHealthy {
		return result, ErrNativeQuotaDependency
	}
	type transfer struct {
		id     string
		amount int64
	}
	transfers := []transfer{}
	transferRows, err := tx.Query(ctx, `SELECT transfer_id::text,amount_units FROM economy.quota_transfers
 WHERE newapi_user_id=$1 AND status IN('PENDING','NEEDS_REVIEW') ORDER BY transfer_id`, user)
	if err != nil {
		return result, err
	}
	for transferRows.Next() {
		var item transfer
		if err = transferRows.Scan(&item.id, &item.amount); err != nil {
			transferRows.Close()
			return result, err
		}
		transfers = append(transfers, item)
	}
	err = transferRows.Err()
	transferRows.Close()
	if err != nil {
		return result, err
	}
	for _, item := range transfers {
		receipt, err := reader.native.QueryQuotaOperation(ctx, item.id, user)
		if err != nil {
			return result, err
		}
		switch receipt.Result {
		case "APPLIED":
			// Active contains the effect; adding a processing bucket would double count it.
		case "NOT_APPLIED", "INSUFFICIENT", "ACCOUNT_RESTRICTED":
			if err = checkedAssetAdd(&result.QuotaTransferInFlightUnits, item.amount); err != nil {
				return result, err
			}
		case "UNKNOWN", "SOURCE_INCOMPATIBLE":
			// The Native effect may already exist but cannot be certified. Returning
			// a total would risk counting it both in Active and in-flight.
			return result, ErrNativeQuotaDependency
		default:
			return result, ErrNativeQuotaDependency
		}
	}
	exchangeUnits, err := apiChipsInFlight(ctx, tx, reader.native, user)
	if err != nil {
		return result, err
	}
	if err = checkedAssetAdd(&result.QuotaTransferInFlightUnits, exchangeUnits); err != nil {
		return result, err
	}
	snapshot, err := reader.native.ReadNativeQuota(ctx, user)
	if err != nil || !snapshot.Enabled || snapshot.Result != "APPLIED" || snapshot.RawQuota < 0 || snapshot.ObservedAt.IsZero() {
		if err != nil {
			return result, err
		}
		return result, ErrNativeQuotaDependency
	}
	result.ActiveQuotaUnits = snapshot.RawQuota
	result.NativeObservedAt = snapshot.ObservedAt.UTC()
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&result.ObservedAt); err != nil {
		return result, err
	}
	result.ObservedAt = result.ObservedAt.UTC()
	if result.NativeObservedAt.Before(result.ObservedAt) {
		result.ObservedAt = result.NativeObservedAt
	}
	for _, value := range []int64{result.ActiveQuotaUnits, result.ReserveUnits, result.AvailableChipsUnits, result.PokerStackUnits, result.PokerPotUnits, result.QuotaTransferInFlightUnits} {
		if err = checkedAssetAdd(&result.TotalUnits, value); err != nil {
			return result, err
		}
	}
	result.TotalAmount = FormatAmount(result.TotalUnits)
	return result, nil
}

func (reader *UnifiedAssetReader) ReadRankingAssets(ctx context.Context, user int64) (string, time.Time, error) {
	assets, err := reader.ReadUnifiedAssets(ctx, user)
	if err != nil {
		return "", time.Time{}, err
	}
	return strconv.FormatInt(assets.TotalUnits, 10), assets.ObservedAt, nil
}
