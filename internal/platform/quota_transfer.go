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
	// Resolve accepted historical keys before applying the current Native limit.
	if amount > MaxNativeQuotaDelta {
		return QuotaTransfer{}, ErrInvalidMutation
	}
	if pending, err := unresolvedAPIChips(ctx, tx, user); err != nil {
		return QuotaTransfer{}, err
	} else if pending {
		return QuotaTransfer{}, ErrTransferPending
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
func (s *Store) ProcessQuotaTransfer(ctx context.Context, native NativeQuotaOperator) (bool, error) {
	if native == nil {
		return false, ErrNativeQuotaDependency
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer rollback(tx)
	v, err := scanTransfer(tx.QueryRow(ctx, `SELECT `+transferColumns+` FROM economy.quota_transfers WHERE status='PENDING' ORDER BY created_at,transfer_id LIMIT 1`))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = lockIdentity(ctx, tx, "quota-transfer-user", v.UserID); err != nil {
		return true, err
	}
	v, err = scanTransfer(tx.QueryRow(ctx, `SELECT `+transferColumns+` FROM economy.quota_transfers WHERE transfer_id=$1 FOR UPDATE`, v.ID))
	if err != nil {
		return true, err
	}
	if v.Status != "PENDING" {
		return false, tx.Commit(ctx)
	}
	if pending, e := unresolvedAPIChips(ctx, tx, v.UserID); e != nil {
		return true, e
	} else if pending {
		return true, ErrTransferPending
	}
	_, err = settleQuotaTransfer(ctx, tx, native, v)
	if err != nil {
		return true, err
	}
	return true, tx.Commit(ctx)
}

// ProcessQuotaTransferByID lets the Native request-time hook finish the exact
// durable transfer it just created. The background worker uses the same effect
// function and remains the recovery owner after timeouts or process exits.
func (s *Store) ProcessQuotaTransferByID(ctx context.Context, native NativeQuotaOperator, id string, user int64) (QuotaTransfer, error) {
	if native == nil || !ValidOperationKey(id) || !validateQuotaUser(user) {
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
	v, err := scanTransfer(tx.QueryRow(ctx, `SELECT `+transferColumns+` FROM economy.quota_transfers WHERE transfer_id=$1 AND newapi_user_id=$2 FOR UPDATE`, id, user))
	if errors.Is(err, pgx.ErrNoRows) {
		return QuotaTransfer{}, ErrInvalidMutation
	}
	if err != nil {
		return QuotaTransfer{}, err
	}
	if v.Status != "PENDING" {
		return v, tx.Commit(ctx)
	}
	if pending, e := unresolvedAPIChips(ctx, tx, user); e != nil {
		return QuotaTransfer{}, e
	} else if pending {
		return QuotaTransfer{}, ErrTransferPending
	}
	v, err = settleQuotaTransfer(ctx, tx, native, v)
	if err != nil {
		return QuotaTransfer{}, err
	}
	return v, tx.Commit(ctx)
}

func settleQuotaTransfer(ctx context.Context, tx pgx.Tx, native NativeQuotaOperator, v QuotaTransfer) (QuotaTransfer, error) {
	var receipt NativeQuotaReceipt
	var err error
	rejection := ""
	if v.AmountUnits > MaxNativeQuotaDelta {
		// The signed Native port rejects this amount before sending an apply.
		// Never apply it again; first preserve any receipt from a legacy adapter.
		receipt, err = native.QueryQuotaOperation(ctx, v.ID, v.UserID)
		if err == nil {
			if receipt.ID != v.ID || receipt.UserID != v.UserID {
				return QuotaTransfer{}, ErrNativeQuotaDependency
			}
			switch receipt.Result {
			case "NOT_APPLIED":
				if receipt.DeltaRawQuota != 0 || receipt.Before != nil || receipt.After != nil {
					return QuotaTransfer{}, ErrNativeQuotaDependency
				}
				rejection = "AMOUNT_OUT_OF_RANGE"
			case "APPLIED":
				// Checked against the original amount below, never compensated.
			default:
				return QuotaTransfer{}, ErrNativeQuotaDependency
			}
		}
	} else {
		receipt, err = native.Credit(ctx, v.ID, v.UserID, v.AmountUnits)
	}
	if err != nil {
		return QuotaTransfer{}, err
	} // Unknown target outcome stays PENDING; never replace its operation ID.
	status, reason := "CONFIRMED", ""
	if receipt.Result != "APPLIED" {
		if rejection == "" {
			switch receipt.Result {
			case "ACCOUNT_RESTRICTED", "SOURCE_INCOMPATIBLE", "BALANCE_OVERFLOW":
			default:
				return QuotaTransfer{}, errors.New("unrecognized native outcome")
			}
			rejection = receipt.Result
		}
		status, reason = "REFUNDED", rejection
		sub, e := tx.Begin(ctx)
		if e != nil {
			return QuotaTransfer{}, e
		}
		_, e = applyInTx(ctx, sub, Mutation{UserID: v.UserID, Asset: ReserveAPICredit, DeltaUnits: v.AmountUnits, BizType: "NATIVE_QUOTA_TRANSFER", BizID: v.ID + ":refund", EntryType: "RESERVE_TO_ACTIVE_REFUND", IdempotencyKey: "quota:" + v.ID + ":refund"})
		if e != nil {
			rollback(sub)
			if !errors.Is(e, ErrBalanceOverflow) {
				return QuotaTransfer{}, e
			}
			status, reason = "NEEDS_REVIEW", "REFUND_BALANCE_OVERFLOW"
		} else if e = sub.Commit(ctx); e != nil {
			return QuotaTransfer{}, e
		}
	} else if receipt.ID != v.ID || receipt.UserID != v.UserID || receipt.DeltaRawQuota != v.AmountUnits ||
		receipt.Before == nil || receipt.After == nil || *receipt.Before < 0 || *receipt.After < *receipt.Before || *receipt.After-*receipt.Before != v.AmountUnits {
		return QuotaTransfer{}, errors.New("invalid native receipt")
	}
	v, err = scanTransfer(tx.QueryRow(ctx, `UPDATE economy.quota_transfers SET status=$2,reason=$3,native_before=$4,native_after=$5,updated_at=clock_timestamp() WHERE transfer_id=$1 RETURNING `+transferColumns, v.ID, status, reason, receipt.Before, receipt.After))
	return v, err
}
