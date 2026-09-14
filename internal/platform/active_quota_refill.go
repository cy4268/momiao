package platform

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
)

const (
	ActiveQuotaAvailable          = "AVAILABLE"
	ActiveQuotaRefilled           = "REFILLED"
	ActiveQuotaNotAvailable       = "NOT_AVAILABLE"
	ActiveQuotaSourceIncompatible = "SOURCE_INCOMPATIBLE"
)

var ErrActiveQuotaRefillConfig = errors.New("active quota refill configuration invalid")

type ActiveQuotaRefillPolicy struct {
	Enabled         bool
	LowWatermark    int64
	TargetWatermark int64
	MaxActiveBuffer int64
}

func (policy ActiveQuotaRefillPolicy) valid() bool {
	if !policy.Enabled {
		return policy.LowWatermark == 0 && policy.TargetWatermark == 0 && policy.MaxActiveBuffer == 0
	}
	return policy.LowWatermark >= 0 && policy.LowWatermark < policy.TargetWatermark &&
		policy.TargetWatermark <= policy.MaxActiveBuffer && policy.MaxActiveBuffer <= 1<<31-1
}

type ActiveQuotaRefillRequest struct {
	RequestID        string
	UserID           int64
	RequiredRawQuota int64
}

type ActiveQuotaRefillResult struct {
	RequestID         string `json:"request_id"`
	UserID            int64  `json:"user_id,string"`
	RequiredRawQuota  int64  `json:"required_raw_quota,string"`
	AvailableRawQuota int64  `json:"available_raw_quota,string"`
	OperationID       string `json:"operation_id,omitempty"`
	Result            string `json:"result"`
}

type activeQuotaRefillStore interface {
	PlanActiveQuotaRefill(context.Context, NativeQuotaOperator, ActiveQuotaRefillRequest, ActiveQuotaRefillPolicy) (ActiveQuotaRefillPlan, error)
	ProcessQuotaTransferByID(context.Context, NativeQuotaOperator, string, int64) (QuotaTransfer, error)
}

type ActiveQuotaRefillPlan struct {
	ObservedActiveRawQuota int64
	Transfer               *QuotaTransfer
	Result                 string
}

type ActiveQuotaRefillService struct {
	store  activeQuotaRefillStore
	native NativeQuotaOperator
	policy ActiveQuotaRefillPolicy
}

func NewActiveQuotaRefillService(store activeQuotaRefillStore, native NativeQuotaOperator, policy ActiveQuotaRefillPolicy) (*ActiveQuotaRefillService, error) {
	if !policy.valid() || policy.Enabled && (store == nil || native == nil) {
		return nil, ErrActiveQuotaRefillConfig
	}
	return &ActiveQuotaRefillService{store: store, native: native, policy: policy}, nil
}

func validRefillRequestID(value string) bool {
	if len(value) < 8 || len(value) > 128 {
		return false
	}
	for i := range value {
		c := value[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// ActiveQuotaRefillOperationKey is stable for one Native logical request. A
// retry can only find the first transfer; changing required quota conflicts
// with that transfer instead of moving Reserve twice.
func ActiveQuotaRefillOperationKey(user int64, requestID string) (string, error) {
	if !validateQuotaUser(user) || !validRefillRequestID(requestID) {
		return "", ErrInvalidMutation
	}
	hash := sha256.Sum256([]byte("momiao.active-quota-refill.v1\x00" + strconv.FormatInt(user, 10) + "\x00" + requestID))
	hash[6] = hash[6]&0x0f | 0x50
	hash[8] = hash[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", hash[:4], hash[4:6], hash[6:8], hash[8:10], hash[10:16]), nil
}

func refillResult(request ActiveQuotaRefillRequest, available int64, operationID, result string) ActiveQuotaRefillResult {
	return ActiveQuotaRefillResult{RequestID: request.RequestID, UserID: request.UserID, RequiredRawQuota: request.RequiredRawQuota, AvailableRawQuota: available, OperationID: operationID, Result: result}
}

func (s *Store) PlanActiveQuotaRefill(ctx context.Context, native NativeQuotaOperator, request ActiveQuotaRefillRequest, policy ActiveQuotaRefillPolicy) (ActiveQuotaRefillPlan, error) {
	if s == nil || native == nil || !policy.valid() || !policy.Enabled || !validRefillRequestID(request.RequestID) ||
		!validateQuotaUser(request.UserID) || request.RequiredRawQuota <= 0 || request.RequiredRawQuota > 1<<31-1 {
		return ActiveQuotaRefillPlan{}, ErrInvalidMutation
	}
	key, _ := ActiveQuotaRefillOperationKey(request.UserID, request.RequestID)
	keyHash := sha256.Sum256([]byte(key))
	requestHash := sha256.Sum256([]byte(request.RequestID))
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ActiveQuotaRefillPlan{}, err
	}
	defer rollback(tx)
	if err = lockIdentity(ctx, tx, "quota-transfer-user", request.UserID); err != nil {
		return ActiveQuotaRefillPlan{}, err
	}
	if pending, err := unresolvedAPIChips(ctx, tx, request.UserID); err != nil {
		return ActiveQuotaRefillPlan{}, err
	} else if pending {
		return ActiveQuotaRefillPlan{Result: ActiveQuotaNotAvailable}, tx.Commit(ctx)
	}
	prior, priorErr := scanTransfer(tx.QueryRow(ctx, `SELECT `+transferColumns+` FROM economy.quota_transfers WHERE newapi_user_id=$1 AND request_key_hash=$2`, request.UserID, keyHash[:]))
	if priorErr == nil {
		var storedHash []byte
		var storedRequired int64
		if err = tx.QueryRow(ctx, `SELECT request_id_hash,required_raw_quota FROM economy.active_quota_refill_plans WHERE transfer_id=$1 AND newapi_user_id=$2`, prior.ID, request.UserID).Scan(&storedHash, &storedRequired); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ActiveQuotaRefillPlan{}, ErrIdempotencyConflict
			}
			return ActiveQuotaRefillPlan{}, err
		}
		if string(storedHash) != string(requestHash[:]) || storedRequired != request.RequiredRawQuota {
			return ActiveQuotaRefillPlan{}, ErrIdempotencyConflict
		}
		snapshot, err := native.ReadNativeQuota(ctx, request.UserID)
		if err != nil {
			return ActiveQuotaRefillPlan{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return ActiveQuotaRefillPlan{}, err
		}
		return ActiveQuotaRefillPlan{ObservedActiveRawQuota: max(snapshot.RawQuota, 0), Transfer: &prior}, nil
	}
	if !errors.Is(priorErr, pgx.ErrNoRows) {
		return ActiveQuotaRefillPlan{}, priorErr
	}
	snapshot, err := native.ReadNativeQuota(ctx, request.UserID)
	if err != nil {
		return ActiveQuotaRefillPlan{}, err
	}
	if !snapshot.Enabled || snapshot.Result != "APPLIED" || snapshot.RawQuota < 0 {
		if err = tx.Commit(ctx); err != nil {
			return ActiveQuotaRefillPlan{}, err
		}
		return ActiveQuotaRefillPlan{ObservedActiveRawQuota: max(snapshot.RawQuota, 0), Result: ActiveQuotaSourceIncompatible}, nil
	}
	if snapshot.RawQuota >= request.RequiredRawQuota && snapshot.RawQuota > policy.LowWatermark {
		if err = tx.Commit(ctx); err != nil {
			return ActiveQuotaRefillPlan{}, err
		}
		return ActiveQuotaRefillPlan{ObservedActiveRawQuota: snapshot.RawQuota, Result: ActiveQuotaAvailable}, nil
	}
	if request.RequiredRawQuota > policy.MaxActiveBuffer {
		if err = tx.Commit(ctx); err != nil {
			return ActiveQuotaRefillPlan{}, err
		}
		return ActiveQuotaRefillPlan{ObservedActiveRawQuota: snapshot.RawQuota, Result: ActiveQuotaNotAvailable}, nil
	}
	desired := min(max(policy.TargetWatermark, request.RequiredRawQuota), policy.MaxActiveBuffer)
	needed := desired - snapshot.RawQuota
	if needed <= 0 {
		if err = tx.Commit(ctx); err != nil {
			return ActiveQuotaRefillPlan{}, err
		}
		return ActiveQuotaRefillPlan{ObservedActiveRawQuota: snapshot.RawQuota, Result: ActiveQuotaAvailable}, nil
	}
	var reserve int64
	if err = tx.QueryRow(ctx, `SELECT balance_units FROM economy.wallet_balances WHERE newapi_user_id=$1 AND asset_type='RESERVE_API_CREDIT' FOR UPDATE`, request.UserID).Scan(&reserve); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if err = tx.Commit(ctx); err != nil {
				return ActiveQuotaRefillPlan{}, err
			}
			return ActiveQuotaRefillPlan{ObservedActiveRawQuota: snapshot.RawQuota, Result: ActiveQuotaNotAvailable}, nil
		}
		return ActiveQuotaRefillPlan{}, err
	}
	var pending bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM economy.quota_transfers WHERE newapi_user_id=$1 AND status IN('PENDING','NEEDS_REVIEW'))`, request.UserID).Scan(&pending); err != nil {
		return ActiveQuotaRefillPlan{}, err
	}
	if pending || reserve <= 0 {
		if err = tx.Commit(ctx); err != nil {
			return ActiveQuotaRefillPlan{}, err
		}
		return ActiveQuotaRefillPlan{ObservedActiveRawQuota: snapshot.RawQuota, Result: ActiveQuotaNotAvailable}, nil
	}
	amount := min(needed, reserve)
	id, err := uuidV7()
	if err != nil {
		return ActiveQuotaRefillPlan{}, err
	}
	_, err = applyInTx(ctx, tx, Mutation{UserID: request.UserID, Asset: ReserveAPICredit, DeltaUnits: -amount, BizType: "NATIVE_QUOTA_TRANSFER", BizID: id + ":debit", EntryType: "RESERVE_TO_ACTIVE_DEBIT", IdempotencyKey: "quota:" + id + ":debit"})
	if err != nil {
		return ActiveQuotaRefillPlan{}, err
	}
	transfer, err := scanTransfer(tx.QueryRow(ctx, `INSERT INTO economy.quota_transfers(transfer_id,newapi_user_id,request_key_hash,amount_units,status)
	 VALUES($1,$2,$3,$4,'PENDING') RETURNING `+transferColumns, id, request.UserID, keyHash[:], amount))
	if err != nil {
		return ActiveQuotaRefillPlan{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO economy.active_quota_refill_plans(transfer_id,newapi_user_id,request_id_hash,required_raw_quota,
	 observed_active_raw_quota,low_watermark,target_watermark,max_active_buffer,desired_active_raw_quota,refill_raw_quota)
	 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, transfer.ID, request.UserID, requestHash[:], request.RequiredRawQuota,
		snapshot.RawQuota, policy.LowWatermark, policy.TargetWatermark, policy.MaxActiveBuffer, desired, amount)
	if err != nil {
		return ActiveQuotaRefillPlan{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ActiveQuotaRefillPlan{}, err
	}
	return ActiveQuotaRefillPlan{ObservedActiveRawQuota: snapshot.RawQuota, Transfer: &transfer}, nil
}

func (service *ActiveQuotaRefillService) Refill(ctx context.Context, request ActiveQuotaRefillRequest) (ActiveQuotaRefillResult, error) {
	if service == nil || !service.policy.Enabled || service.store == nil || service.native == nil {
		return ActiveQuotaRefillResult{}, ErrActiveQuotaRefillConfig
	}
	if !validRefillRequestID(request.RequestID) || !validateQuotaUser(request.UserID) || request.RequiredRawQuota <= 0 || request.RequiredRawQuota > 1<<31-1 {
		return ActiveQuotaRefillResult{}, ErrInvalidMutation
	}
	plan, err := service.store.PlanActiveQuotaRefill(ctx, service.native, request, service.policy)
	if err != nil {
		return ActiveQuotaRefillResult{}, err
	}
	if plan.Transfer == nil {
		return refillResult(request, plan.ObservedActiveRawQuota, "", plan.Result), nil
	}
	return service.finishPrior(ctx, request, plan.ObservedActiveRawQuota, *plan.Transfer)
}

func (service *ActiveQuotaRefillService) finishPrior(ctx context.Context, request ActiveQuotaRefillRequest, observed int64, transfer QuotaTransfer) (ActiveQuotaRefillResult, error) {
	if transfer.Status == "PENDING" {
		var err error
		transfer, err = service.store.ProcessQuotaTransferByID(ctx, service.native, transfer.ID, request.UserID)
		if err != nil {
			return ActiveQuotaRefillResult{}, err
		}
	}
	available := observed
	if transfer.NativeAfter != nil {
		available = *transfer.NativeAfter
	}
	switch transfer.Status {
	case "CONFIRMED":
		result := ActiveQuotaNotAvailable
		if available >= request.RequiredRawQuota {
			result = ActiveQuotaRefilled
		}
		return refillResult(request, available, transfer.ID, result), nil
	case "REFUNDED":
		result := ActiveQuotaNotAvailable
		if transfer.Reason == "SOURCE_INCOMPATIBLE" || transfer.Reason == "ACCOUNT_RESTRICTED" {
			result = ActiveQuotaSourceIncompatible
		}
		return refillResult(request, available, transfer.ID, result), nil
	case "NEEDS_REVIEW":
		return ActiveQuotaRefillResult{}, ErrNativeQuotaDependency
	default:
		return ActiveQuotaRefillResult{}, ErrNativeQuotaDependency
	}
}
