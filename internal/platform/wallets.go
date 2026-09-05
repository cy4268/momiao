package platform

import (
	"context"
	"errors"
)

var ErrIncompleteWallets = errors.New("incomplete wallet pair")

// ReadWallets reads both local assets in one statement snapshot, never creating rows.
// Zero rows means uninitialized; a partial pair is an integrity failure, not zero balance.
func (s *Store) ReadWallets(ctx context.Context, userID int64) ([]Wallet, error) {
	if userID <= 0 {
		return nil, ErrInvalidMutation
	}
	rows, err := s.pool.Query(ctx, `SELECT newapi_user_id,asset_type,balance_units,ledger_seq,version FROM economy.wallet_balances WHERE newapi_user_id=$1 ORDER BY CASE asset_type WHEN 'RESERVE_API_CREDIT' THEN 0 ELSE 1 END`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	wallets := []Wallet{}
	for rows.Next() {
		var w Wallet
		if err := rows.Scan(&w.UserID, &w.Asset, &w.BalanceUnits, &w.LedgerSeq, &w.Version); err != nil {
			return nil, err
		}
		wallets = append(wallets, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(wallets) != 0 && (len(wallets) != 2 || wallets[0].Asset != ReserveAPICredit || wallets[1].Asset != AvailableChips) {
		return nil, ErrIncompleteWallets
	}
	return wallets, nil
}
