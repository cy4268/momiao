package platform

import (
	"context"
	"crypto/sha256"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

var ErrTransferPending = errors.New("unresolved quota transfer")

type QuotaTransfer struct {
	ID           string    `json:"id"`
	UserID       int64     `json:"user_id,string"`
	AmountUnits  int64     `json:"amount_units,string"`
	Amount       string    `json:"amount"`
	Status       string    `json:"status"`
	Reason       string    `json:"reason"`
	NativeBefore *int64    `json:"native_before,string"`
	NativeAfter  *int64    `json:"native_after,string"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

const transferColumns = `transfer_id::text,newapi_user_id,amount_units,status,reason,native_before,native_after,created_at,updated_at`

func scanTransfer(row pgx.Row) (QuotaTransfer, error) {
	var v QuotaTransfer
	err := row.Scan(&v.ID, &v.UserID, &v.AmountUnits, &v.Status, &v.Reason, &v.NativeBefore, &v.NativeAfter, &v.CreatedAt, &v.UpdatedAt)
	v.Amount = FormatAmount(v.AmountUnits)
	v.CreatedAt = v.CreatedAt.UTC()
	v.UpdatedAt = v.UpdatedAt.UTC()
	return v, err
}
func (s *Store) QuotaTransferByKey(ctx context.Context, user int64, key string) (*QuotaTransfer, error) {
	if user <= 0 || !ValidOperationKey(key) {
		return nil, ErrInvalidMutation
	}
	hash := sha256.Sum256([]byte(key))
	v, err := scanTransfer(s.pool.QueryRow(ctx, `SELECT `+transferColumns+` FROM economy.quota_transfers WHERE newapi_user_id=$1 AND request_key_hash=$2`, user, hash[:]))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &v, err
}
func (s *Store) QuotaTransfers(ctx context.Context, user int64) ([]QuotaTransfer, error) {
	if user <= 0 {
		return nil, ErrInvalidMutation
	}
	rows, err := s.pool.Query(ctx, `SELECT `+transferColumns+` FROM economy.quota_transfers WHERE newapi_user_id=$1 ORDER BY transfer_id DESC LIMIT 20`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []QuotaTransfer{}
	for rows.Next() {
		v, e := scanTransfer(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) CreateQuotaTransfer(ctx context.Context, user int64, key string, amount int64) (QuotaTransfer, error) {
	if user <= 0 || !ValidOperationKey(key) || amount <= 0 || amount > 9007199254740991 {
		return QuotaTransfer{}, ErrInvalidMutation
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return QuotaTransfer{}, err
	}
	defer rollback(tx)
	if err = lockIdentity(ctx, tx, "quota-transfer-user", user); err != nil {
		return QuotaTransfer{}, err
	}
	hash := sha256.Sum256([]byte(key))
	old, err := scanTransfer(tx.QueryRow(ctx, `SELECT `+transferColumns+` FROM economy.quota_transfers WHERE newapi_user_id=$1 AND request_key_hash=$2`, user, hash[:]))
	if err == nil {
		if old.AmountUnits != amount {
			return QuotaTransfer{}, ErrIdempotencyConflict
		}
		return old, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return QuotaTransfer{}, err
	}
	var pending bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM economy.quota_transfers WHERE newapi_user_id=$1 AND status IN ('PENDING','NEEDS_REVIEW'))`, user).Scan(&pending); err != nil {
		return QuotaTransfer{}, err
	}
	if pending {
		return QuotaTransfer{}, ErrTransferPending
	}
	id, err := uuidV7()
	if err != nil {
		return QuotaTransfer{}, err
	}
	_, err = applyInTx(ctx, tx, Mutation{UserID: user, Asset: ReserveAPICredit, DeltaUnits: -amount, BizType: "NATIVE_QUOTA_TRANSFER", BizID: id + ":debit", EntryType: "RESERVE_TO_ACTIVE_DEBIT", IdempotencyKey: "quota:" + id + ":debit"})
	if err != nil {
		return QuotaTransfer{}, err
	}
	v, err := scanTransfer(tx.QueryRow(ctx, `INSERT INTO economy.quota_transfers(transfer_id,newapi_user_id,request_key_hash,amount_units,status) VALUES($1,$2,$3,$4,'PENDING') RETURNING `+transferColumns, id, user, hash[:], amount))
	if err != nil {
		return QuotaTransfer{}, err
	}
	return v, tx.Commit(ctx)
}

// ProcessQuotaTransfer performs one durable job, independent of browser lifetime.
// ponytail: one worker with SKIP LOCKED; add throughput only when measured demand needs it.
func (s *Store) ProcessQuotaTransfer(ctx context.Context, native *NativeQuota) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer rollback(tx)
	v, err := scanTransfer(tx.QueryRow(ctx, `SELECT `+transferColumns+` FROM economy.quota_transfers WHERE status='PENDING' ORDER BY created_at,transfer_id FOR UPDATE SKIP LOCKED LIMIT 1`))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	receipt, err := native.Credit(ctx, v.ID, v.UserID, v.AmountUnits)
	if err != nil {
		return true, err
	} // Unknown target outcome stays PENDING; never replace its operation ID.
	status, reason := "CONFIRMED", ""
	if receipt.Result != "APPLIED" {
		switch receipt.Result {
		case "ACCOUNT_RESTRICTED", "SOURCE_INCOMPATIBLE", "BALANCE_OVERFLOW":
		default:
			return true, errors.New("unrecognized native outcome")
		}
		status, reason = "REFUNDED", receipt.Result
		sub, e := tx.Begin(ctx)
		if e != nil {
			return true, e
		}
		_, e = applyInTx(ctx, sub, Mutation{UserID: v.UserID, Asset: ReserveAPICredit, DeltaUnits: v.AmountUnits, BizType: "NATIVE_QUOTA_TRANSFER", BizID: v.ID + ":refund", EntryType: "RESERVE_TO_ACTIVE_REFUND", IdempotencyKey: "quota:" + v.ID + ":refund"})
		if e != nil {
			rollback(sub)
			if !errors.Is(e, ErrBalanceOverflow) {
				return true, e
			}
			status, reason = "NEEDS_REVIEW", "REFUND_BALANCE_OVERFLOW"
		} else if e = sub.Commit(ctx); e != nil {
			return true, e
		}
	} else if receipt.Before == nil || receipt.After == nil || *receipt.Before < 0 || *receipt.After-*receipt.Before != v.AmountUnits {
		return true, errors.New("invalid native receipt")
	}
	_, err = tx.Exec(ctx, `UPDATE economy.quota_transfers SET status=$2,reason=$3,native_before=$4,native_after=$5,updated_at=clock_timestamp() WHERE transfer_id=$1`, v.ID, status, reason, receipt.Before, receipt.After)
	if err != nil {
		return true, err
	}
	return true, tx.Commit(ctx)
}
