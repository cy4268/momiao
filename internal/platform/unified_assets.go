package platform

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type UnifiedAssets struct {
	UserID                    int64     `json:"user_id,string"`
	ReserveUnits              int64     `json:"reserve_units,string"`
	AvailableChipsUnits       int64     `json:"available_chips_units,string"`
	PokerStackUnits           int64     `json:"poker_stack_units,string"`
	PokerPotUnits             int64     `json:"poker_pot_units,string"`
	SinglePlayerInFlightUnits int64     `json:"single_player_in_flight_units,string"`
	RouletteEscrowUnits       int64     `json:"roulette_escrow_units,string"`
	TotalUnits                int64     `json:"total_units,string"`
	TotalAmount               string    `json:"total_amount"`
	ObservedAt                time.Time `json:"observed_at"`
}

type UnifiedAssetReader struct {
	store *Store
}

func NewUnifiedAssetReader(store *Store) *UnifiedAssetReader {
	return &UnifiedAssetReader{store: store}
}

func checkedAssetAdd(total *int64, value int64) error {
	if value < 0 || *total > math.MaxInt64-value {
		return ErrBalanceOverflow
	}
	*total += value
	return nil
}

// ReadUnifiedAssets reads the six local balances in one repeatable-read snapshot.
// Existing wallet locks also serialize game funding and reward eligibility.
func (reader *UnifiedAssetReader) ReadUnifiedAssets(ctx context.Context, user int64) (UnifiedAssets, error) {
	tx, err := reader.store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return UnifiedAssets{UserID: user}, err
	}
	defer rollback(tx)
	result, err := ReadUnifiedAssetsInTx(ctx, tx, user)
	if err != nil {
		return result, err
	}
	if err = tx.Commit(ctx); err != nil {
		return UnifiedAssets{}, err
	}
	return result, nil
}

// Write callers use READ COMMITTED when also checking maintenance; wallet locks
// serialize funding, and each game projection uses a single SQL snapshot.
func ReadUnifiedAssetsInTx(ctx context.Context, tx pgx.Tx, user int64) (UnifiedAssets, error) {
	result := UnifiedAssets{UserID: user}
	var err error
	if err = lockIdentity(ctx, tx, "quota-transfer-user", user); err != nil {
		return result, err
	}
	rows, err := tx.Query(ctx, `SELECT asset_type,balance_units FROM economy.cap_wallets_read($1)`, user)
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
		return result, ErrEconomicPolicy
	}
	var localHealthy bool
	if err = tx.QueryRow(ctx, `SELECT single_player_units,roulette_units,healthy FROM economy.cap_local_assets_read($1)`, user).Scan(&result.SinglePlayerInFlightUnits, &result.RouletteEscrowUnits, &localHealthy); err != nil {
		return result, err
	}
	if !localHealthy {
		return result, ErrEconomicPolicy
	}
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&result.ObservedAt); err != nil {
		return result, err
	}
	result.ObservedAt = result.ObservedAt.UTC()
	for _, value := range []int64{result.ReserveUnits, result.AvailableChipsUnits, result.PokerStackUnits, result.PokerPotUnits, result.SinglePlayerInFlightUnits, result.RouletteEscrowUnits} {
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
