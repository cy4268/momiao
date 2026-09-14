package platform

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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
func (p apiChipsExchange) hash() [32]byte {
	// Versioned ordered fields, raw integer units and lower-case persisted UUIDs.
	raw, _ := json.Marshal([]any{"api-chips-plan.v1", p.ID, p.User, p.KeyHash, "API_TO_CHIPS", p.Requested, p.Reserve, p.Active, p.Requested, p.DebitID})
	return sha256.Sum256(raw)
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
func acceptAPIChipsExchange(ctx context.Context, tx pgx.Tx, native NativeQuotaOperator, user int64, key string, amount int64, wallets map[Asset]Wallet) (Transaction, error) {
	reserve := wallets[ReserveAPICredit].BalanceUnits
	active := amount - reserve
	if !validateQuotaUser(user) || !validateQuotaDelta(active) {
		return Transaction{}, ErrInvalidMutation
	}
	if _, legacy := native.(*NativeQuota); legacy {
		return Transaction{}, ErrNativeQuotaDependency
	}
	pending, err := unresolvedAPIChips(ctx, tx, user)
	if err != nil {
		return Transaction{}, err
	}
	if pending {
		return Transaction{}, ErrTransferPending
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM economy.quota_transfers WHERE newapi_user_id=$1 AND status IN('PENDING','NEEDS_REVIEW'))`, user).Scan(&pending); err != nil {
		return Transaction{}, err
	}
	if pending {
		return Transaction{}, ErrTransferPending
	}
	target := wallets[AvailableChips]
	if target.BalanceUnits > math.MaxInt64-amount || target.Version == math.MaxInt64 || target.LedgerSeq == math.MaxInt64 {
		return Transaction{}, ErrBalanceOverflow
	}
	snapshot, err := native.ReadNativeQuota(ctx, user)
	if err != nil {
		return Transaction{}, err
	}
	if snapshot.UserID != user || !snapshot.Enabled || snapshot.Result != "APPLIED" || snapshot.RawQuota < 0 || snapshot.RawQuota > math.MaxInt32 || snapshot.ObservedAt.IsZero() {
		return Transaction{}, ErrNativeQuotaDependency
	}
	if snapshot.RawQuota < active {
		return Transaction{}, ErrInsufficientBalance
	}
	id, err := uuidV7()
	if err != nil {
		return Transaction{}, err
	}
	debitID, err := uuidV7()
	if err != nil {
		return Transaction{}, err
	}
	keyHash := sha256.Sum256([]byte(key))
	p := apiChipsExchange{ID: id, User: user, KeyHash: keyHash[:], Requested: amount, Reserve: reserve, Active: active, DebitID: debitID}
	planHash := p.hash()
	if reserve > 0 {
		entry, err := applyInTx(ctx, tx, p.mutation("reserve-debit", ReserveAPICredit, -reserve))
		if err != nil {
			return Transaction{}, err
		}
		p.ReserveLedger = &entry.ID
	}
	p, err = scanAPIChips(tx.QueryRow(ctx, `INSERT INTO economy.api_chips_exchanges(exchange_id,newapi_user_id,request_key_hash,requested_units,
 reserve_debit_units,active_debit_units,chips_credit_units,debit_operation_id,plan_hash,reserve_ledger_id)
 VALUES($1,$2,$3,$4,$5,$6,$4,$7,$8,$9) RETURNING `+apiChipsColumns, id, user, keyHash[:], amount, reserve, active, debitID, planHash[:], p.ReserveLedger))
	return p.receipt(), err
}
func (p apiChipsExchange) mutation(leg string, asset Asset, delta int64) Mutation {
	return Mutation{UserID: p.User, Asset: asset, DeltaUnits: delta, BizType: "API_CHIPS_EXCHANGE_LEG", BizID: p.ID + ":" + leg, EntryType: "API_CHIPS_" + map[string]string{"reserve-debit": "RESERVE_DEBIT", "chips-credit": "CHIPS_CREDIT", "reserve-refund": "RESERVE_REFUND"}[leg], IdempotencyKey: "api-chips:" + p.ID + ":" + leg}
}

// nativeExchangeReceipt checks the complete authority and signed arithmetic even
// for injected/legacy adapters. NOT_APPLIED query receipts have no stored delta.
func nativeExchangeReceipt(r NativeQuotaReceipt, id string, user, delta int64, query bool) error {
	if r.ID != id || r.UserID != user {
		return ErrNativeQuotaDependency
	}
	if query && r.Result == "NOT_APPLIED" {
		if r.DeltaRawQuota != 0 || r.Before != nil || r.After != nil {
			return ErrNativeQuotaDependency
		}
		return nil
	}
	if r.DeltaRawQuota != delta {
		return ErrIdempotencyConflict
	}
	switch r.Result {
	case "APPLIED":
		if r.Before == nil || r.After == nil || *r.Before < 0 || *r.After < 0 || *r.Before > math.MaxInt32 || *r.After > math.MaxInt32 || *r.After-*r.Before != delta {
			return ErrNativeQuotaDependency
		}
	case "INSUFFICIENT", "ACCOUNT_RESTRICTED", "SOURCE_INCOMPATIBLE":
		// A rejected effect may report the incompatible quota that caused the
		// rejection. Paired unchanged values prove no effect even outside the
		// APPLIED range; never reinterpret them as a spendable quota snapshot.
		if (r.Before == nil) != (r.After == nil) || (r.Before != nil && *r.After != *r.Before) {
			return ErrNativeQuotaDependency
		}
	default:
		return ErrNativeQuotaDependency
	}
	return nil
}
func queryExchangeEffect(ctx context.Context, native NativeQuotaOperator, id string, user, delta int64) (NativeQuotaReceipt, error) {
	r, err := native.QueryQuotaOperation(ctx, id, user)
	if err != nil {
		return r, err
	}
	return r, nativeExchangeReceipt(r, id, user, delta, true)
}
func applyExchangeEffect(ctx context.Context, native NativeQuotaOperator, id string, user, delta int64) (NativeQuotaReceipt, error) {
	// Native Apply owns journal idempotency AND Redis fence recovery. A journal
	// query can say APPLIED while its fence remains, or UNKNOWN before the journal
	// commit. Always replay the original persisted ID to recover either boundary.
	r, err := native.ApplyRawQuotaDelta(ctx, id, user, delta)
	if err != nil {
		return r, err
	}
	return r, nativeExchangeReceipt(r, id, user, delta, false)
}

// Each call commits one recoverable phase. Native calls are fenced by the same
// user lock as refill, transfer and unified assets. Browser lifetime is irrelevant.
func (s *Store) ProcessAPIChipsExchange(ctx context.Context, native NativeQuotaOperator) (bool, error) {
	if native == nil {
		return false, ErrNativeQuotaDependency
	}
	// Select identity without row locking; take user before transfer everywhere.
	var id string
	var user int64
	err := s.pool.QueryRow(ctx, `SELECT exchange_id::text,newapi_user_id FROM economy.api_chips_exchanges
 WHERE status IN('PENDING','SOURCE_DEBITED','COMPENSATING') ORDER BY updated_at,exchange_id LIMIT 1`).Scan(&id, &user)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return true, err
	}
	defer rollback(tx)
	if err = lockIdentity(ctx, tx, "quota-transfer-user", user); err != nil {
		return true, err
	}
	p, err := scanAPIChips(tx.QueryRow(ctx, `SELECT `+apiChipsColumns+` FROM economy.api_chips_exchanges WHERE exchange_id=$1 AND newapi_user_id=$2 FOR UPDATE`, id, user))
	if err != nil {
		return true, err
	}
	if p.Status != "PENDING" && p.Status != "SOURCE_DEBITED" && p.Status != "COMPENSATING" {
		return false, tx.Commit(ctx)
	}
	hash := p.hash()
	if string(hash[:]) != string(p.PlanHash) {
		return true, reviewAPIChips(ctx, tx, p, "PLAN_INTEGRITY")
	}
	switch p.Status {
	case "PENDING":
		r, e := applyExchangeEffect(ctx, native, p.DebitID, user, -p.Active)
		if e != nil {
			return true, retryAPIChips(ctx, tx, p, e)
		} // Unknown retains the original ID and durable job.
		if r.Result == "APPLIED" {
			_, err = tx.Exec(ctx, `UPDATE economy.api_chips_exchanges SET status='SOURCE_DEBITED',native_before=$2,native_after=$3,updated_at=clock_timestamp() WHERE exchange_id=$1`, id, r.Before, r.After)
		} else {
			err = restoreExchangeReserve(ctx, tx, &p)
			if errors.Is(err, ErrBalanceOverflow) || errors.Is(err, ErrWalletNotFound) {
				return true, reviewAPIChips(ctx, tx, p, "RESERVE_REFUND_UNAVAILABLE")
			}
			if err != nil {
				return true, err
			}
			status := "COMPENSATED"
			if p.Reserve == 0 {
				status = "FAILED_NO_EFFECT"
			}
			_, err = tx.Exec(ctx, `UPDATE economy.api_chips_exchanges SET status=$2,reason=$3,reserve_refund_ledger_id=$4,updated_at=clock_timestamp() WHERE exchange_id=$1`, id, status, r.Result, p.ReserveRefund)
		}
	case "SOURCE_DEBITED":
		r, e := queryExchangeEffect(ctx, native, p.DebitID, user, -p.Active)
		if e != nil || r.Result != "APPLIED" {
			if e == nil {
				e = ErrNativeQuotaDependency
			}
			return true, retryAPIChips(ctx, tx, p, e)
		}
		if p.Before == nil || p.After == nil || *r.Before != *p.Before || *r.After != *p.After {
			return true, reviewAPIChips(ctx, tx, p, "DEBIT_EVIDENCE_MISMATCH")
		}
		// Savepoint lets a permanent target arithmetic failure enter compensation;
		// transient SQL/commit failures leave SOURCE_DEBITED for exact-id recovery.
		sub, e := tx.Begin(ctx)
		if e != nil {
			return true, e
		}
		entry, e := applyInTx(ctx, sub, p.mutation("chips-credit", AvailableChips, p.Requested))
		if e != nil {
			rollback(sub)
			if !errors.Is(e, ErrBalanceOverflow) && !errors.Is(e, ErrWalletNotFound) {
				return true, e
			}
			refund, e := uuidV7()
			if e != nil {
				return true, e
			}
			_, err = tx.Exec(ctx, `UPDATE economy.api_chips_exchanges SET status='COMPENSATING',reason='TARGET_UNAVAILABLE',refund_operation_id=$2,updated_at=clock_timestamp() WHERE exchange_id=$1`, id, refund)
		} else {
			if e = sub.Commit(ctx); e != nil {
				return true, e
			}
			_, err = tx.Exec(ctx, `UPDATE economy.api_chips_exchanges SET status='CONFIRMED',chips_ledger_id=$2,confirmed_at=clock_timestamp(),updated_at=clock_timestamp() WHERE exchange_id=$1`, id, entry.ID)
		}
	case "COMPENSATING":
		if p.RefundID == nil {
			return true, reviewAPIChips(ctx, tx, p, "REFUND_ID_MISSING")
		}
		debit, e := queryExchangeEffect(ctx, native, p.DebitID, user, -p.Active)
		if e != nil || debit.Result != "APPLIED" {
			if e == nil {
				e = ErrNativeQuotaDependency
			}
			return true, retryAPIChips(ctx, tx, p, e)
		}
		if p.Before == nil || p.After == nil || *debit.Before != *p.Before || *debit.After != *p.After {
			return true, reviewAPIChips(ctx, tx, p, "DEBIT_EVIDENCE_MISMATCH")
		}
		refund, e := applyExchangeEffect(ctx, native, *p.RefundID, user, p.Active)
		if e != nil {
			return true, retryAPIChips(ctx, tx, p, e)
		}
		if refund.Result != "APPLIED" {
			return true, reviewAPIChips(ctx, tx, p, "NATIVE_REFUND_REJECTED")
		}
		if err = restoreExchangeReserve(ctx, tx, &p); err != nil {
			if errors.Is(err, ErrBalanceOverflow) || errors.Is(err, ErrWalletNotFound) {
				if _, e = tx.Exec(ctx, `UPDATE economy.api_chips_exchanges SET refund_before=$2,refund_after=$3 WHERE exchange_id=$1`, id, refund.Before, refund.After); e != nil {
					return true, e
				}
				return true, reviewAPIChips(ctx, tx, p, "RESERVE_REFUND_UNAVAILABLE")
			}
			return true, err
		}
		_, err = tx.Exec(ctx, `UPDATE economy.api_chips_exchanges SET status='COMPENSATED',reserve_refund_ledger_id=$2,refund_before=$3,refund_after=$4,updated_at=clock_timestamp() WHERE exchange_id=$1`, id, p.ReserveRefund, refund.Before, refund.After)
	}
	if err != nil {
		return true, err
	}
	return true, tx.Commit(ctx)
}
func restoreExchangeReserve(ctx context.Context, tx pgx.Tx, p *apiChipsExchange) error {
	if p.Reserve == 0 {
		return nil
	}
	entry, err := applyInTx(ctx, tx, p.mutation("reserve-refund", ReserveAPICredit, p.Reserve))
	if err != nil {
		return err
	}
	p.ReserveRefund = &entry.ID
	return nil
}
func reviewAPIChips(ctx context.Context, tx pgx.Tx, p apiChipsExchange, reason string) error {
	_, err := tx.Exec(ctx, `UPDATE economy.api_chips_exchanges SET status='NEEDS_REVIEW',reason=$2,updated_at=clock_timestamp() WHERE exchange_id=$1`, p.ID, reason)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Rotate uncertain jobs behind other users; never let one unavailable Native
// effect monopolize the bounded worker. The same immutable plan remains active.
func retryAPIChips(ctx context.Context, tx pgx.Tx, p apiChipsExchange, cause error) error {
	if _, err := tx.Exec(ctx, `UPDATE economy.api_chips_exchanges SET updated_at=clock_timestamp() WHERE exchange_id=$1`, p.ID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return cause
}

func apiChipsInFlight(ctx context.Context, tx pgx.Tx, native NativeQuotaOperator, user int64) (int64, error) {
	rows, err := tx.Query(ctx, `SELECT `+apiChipsColumns+` FROM economy.api_chips_exchanges WHERE newapi_user_id=$1 AND status NOT IN('CONFIRMED','COMPENSATED','FAILED_NO_EFFECT')`, user)
	if err != nil {
		return 0, err
	}
	plans := []apiChipsExchange{}
	for rows.Next() {
		p, e := scanAPIChips(rows)
		if e != nil {
			rows.Close()
			return 0, e
		}
		plans = append(plans, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	var total int64
	for _, p := range plans {
		hash := p.hash()
		if string(hash[:]) != string(p.PlanHash) {
			return 0, ErrNativeQuotaDependency
		}
		pending := p.Reserve
		if p.ReserveRefund != nil {
			pending = 0
		}
		debit, e := queryExchangeEffect(ctx, native, p.DebitID, user, -p.Active)
		if e != nil {
			return 0, e
		}
		applied := debit.Result == "APPLIED"
		if p.Before != nil && !applied {
			return 0, ErrNativeQuotaDependency
		}
		if p.Before != nil && (p.After == nil || *debit.Before != *p.Before || *debit.After != *p.After) {
			return 0, ErrNativeQuotaDependency
		}
		if applied {
			if e = checkedAssetAdd(&pending, p.Active); e != nil {
				return 0, e
			}
		}
		if p.RefundID != nil {
			if !applied {
				return 0, ErrNativeQuotaDependency
			}
			refund, e := queryExchangeEffect(ctx, native, *p.RefundID, user, p.Active)
			if e != nil {
				return 0, e
			}
			if refund.Result == "APPLIED" {
				pending -= p.Active
			}
			if p.RefundBefore != nil && (refund.Result != "APPLIED" || p.RefundAfter == nil || *refund.Before != *p.RefundBefore || *refund.After != *p.RefundAfter) {
				return 0, ErrNativeQuotaDependency
			}
		}
		if e = checkedAssetAdd(&total, pending); e != nil {
			return 0, fmt.Errorf("exchange assets: %w", e)
		}
	}
	return total, nil
}
