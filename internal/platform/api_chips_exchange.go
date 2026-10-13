package platform

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type apiChipsExchange struct {
	ID                                                  string
	User                                                int64
	KeyHash                                             []byte
	Requested, Reserve, Active                          int64
	DebitID                                             string
	PlanHash                                            []byte
	Status, Reason                                      string
	ReserveLedger, ChipsLedger, ReserveRefund, RefundID *string
	Before, After, RefundBefore, RefundAfter            *int64
	Created, Updated                                    time.Time
	Confirmed                                           *time.Time
}

const apiChipsColumns = `exchange_id::text,newapi_user_id,request_key_hash,requested_units,reserve_debit_units,active_debit_units,
 debit_operation_id::text,plan_hash,status,reason,reserve_ledger_id::text,chips_ledger_id::text,reserve_refund_ledger_id::text,
 refund_operation_id::text,native_before,native_after,refund_before,refund_after,created_at,updated_at,confirmed_at`

func scanAPIChips(row pgx.Row) (apiChipsExchange, error) {
	var p apiChipsExchange
	err := row.Scan(&p.ID, &p.User, &p.KeyHash, &p.Requested, &p.Reserve, &p.Active, &p.DebitID, &p.PlanHash, &p.Status, &p.Reason,
		&p.ReserveLedger, &p.ChipsLedger, &p.ReserveRefund, &p.RefundID, &p.Before, &p.After, &p.RefundBefore, &p.RefundAfter, &p.Created, &p.Updated, &p.Confirmed)
	return p, err
}
func (p apiChipsExchange) receipt() Transaction {
	t := Transaction{ID: p.ID, UserID: p.User, BizID: "api-chips:" + p.ID, Kind: "API_CHIPS_EXCHANGE", Status: p.Status,
		FromAsset: ReserveAPICredit, ToAsset: AvailableChips, AmountUnits: p.Requested, Amount: FormatAmount(p.Requested), CreatedAt: p.Created.UTC(),
		ReserveDebitUnits: &p.Reserve, ActiveDebitUnits: &p.Active, ChipsCreditUnits: &p.Requested, Reason: p.Reason}
	if p.Confirmed != nil {
		t.ConfirmedAt = p.Confirmed.UTC()
	}
	return t
}
func findAPIChipsExchange(ctx context.Context, tx pgx.Tx, user int64, key string) (*Transaction, error) {
	hash := sha256.Sum256([]byte(key))
	p, err := scanAPIChips(tx.QueryRow(ctx, `SELECT `+apiChipsColumns+` FROM economy.api_chips_exchanges WHERE newapi_user_id=$1 AND request_key_hash=$2`, user, hash[:]))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t := p.receipt()
	return &t, nil
}
func unresolvedAPIChips(ctx context.Context, tx pgx.Tx, user int64) (bool, error) {
	var pending bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM economy.api_chips_exchanges WHERE newapi_user_id=$1 AND status NOT IN('CONFIRMED','COMPENSATED','FAILED_NO_EFFECT'))`, user).Scan(&pending)
	return pending, err
}
