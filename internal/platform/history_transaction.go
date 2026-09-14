package platform

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/cy4268/momiao/internal/historyaccess"
	"github.com/jackc/pgx/v5"
)

type HistoryEffect struct {
	LedgerID           string `json:"ledger_id"`
	LegNo              int    `json:"leg_no"`
	Asset              Asset  `json:"asset"`
	DeltaUnits         string `json:"delta_units"`
	BalanceBeforeUnits string `json:"balance_before_units"`
	BalanceAfterUnits  string `json:"balance_after_units"`
}

type HistoryLink struct {
	RecordType string `json:"record_type"`
	SourceID   string `json:"source_id"`
}

type HistoryTransaction struct {
	ExchangeID        string                `json:"exchange_id,omitempty"`
	ReserveDebitUnits *int64                `json:"reserve_debit_units,string,omitempty"`
	ActiveDebitUnits  *int64                `json:"active_debit_units,string,omitempty"`
	ChipsCreditUnits  *int64                `json:"chips_credit_units,string,omitempty"`
	NativeEffects     []HistoryNativeEffect `json:"native_effects,omitempty"`
	ID                string                `json:"id"`
	Kind              string                `json:"kind"`
	Status            string                `json:"status"`
	CreatedAt         time.Time             `json:"created_at"`
	ConfirmedAt       time.Time             `json:"confirmed_at,omitzero"`
	Effects           []HistoryEffect       `json:"effects"`
	Links             []HistoryLink         `json:"links"`
}
type HistoryNativeEffect struct {
	DeltaUnits  string `json:"delta_units"`
	BeforeUnits string `json:"before_units"`
	AfterUnits  string `json:"after_units"`
}

// HistoryReadOnly must be the first statement in the caller-owned transaction.
func HistoryReadOnly(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY; SET LOCAL statement_timeout='2s'", pgx.QueryExecModeSimpleProtocol)
	return err
}

// HistoryReadError never exposes database/cryptographic error details to the caller.
func HistoryReadError(ctx context.Context, err error) error {
	if err == nil || errors.Is(err, historyaccess.ErrInvalid) || errors.Is(err, historyaccess.ErrNotFound) {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return historyaccess.ErrUnavailable
}

func (s *Store) HistoryTransaction(ctx context.Context, access historyaccess.Access, id string) (HistoryTransaction, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, subject, err := access.Check(ctx)
	if err != nil {
		return HistoryTransaction{}, err
	}
	if !ValidOperationKey(id) {
		return HistoryTransaction{}, historyaccess.ErrInvalid
	}
	var result HistoryTransaction
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		if e := HistoryReadOnly(ctx, tx); e != nil {
			return e
		}
		var e error
		result, e = HistoryTransactionInTx(ctx, tx, subject, id)
		return e
	})
	if err != nil {
		return HistoryTransaction{}, HistoryReadError(ctx, err)
	}
	return result, nil
}

// HistoryTransactionInTx joins a domain detail's already authorized read-only snapshot.
// subject comes from Access.Check; this is not a request-body or untrusted SQL API.
func HistoryTransactionInTx(ctx context.Context, tx pgx.Tx, subject int64, id string) (HistoryTransaction, error) {
	var result HistoryTransaction
	if subject <= 0 || !ValidOperationKey(id) {
		return result, historyaccess.ErrInvalid
	}
	var raw []byte
	if err := tx.QueryRow(ctx, "SELECT coalesce(economy.history_api_chips_exchange_read($1,$2),economy.history_transaction_read($1,$2))", subject, id).Scan(&raw); err != nil {
		return result, HistoryReadError(ctx, err)
	}
	if len(raw) == 0 {
		return result, historyaccess.ErrNotFound
	}
	if err := json.Unmarshal(raw, &result); err != nil || result.ID != id || result.Effects == nil || result.Links == nil {
		return HistoryTransaction{}, historyaccess.ErrUnavailable
	}
	return result, nil
}
