package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// OpsEconomyService projects the existing wallet, transfer and reward
// authorities for Operations. It never substitutes local-only values for a
// unified asset total when the Native reader is unavailable.
type OpsEconomyService struct {
	store  *Store
	assets *UnifiedAssetReader
}

func NewOpsEconomyService(store *Store, assets *UnifiedAssetReader) (*OpsEconomyService, error) {
	if store == nil || store.pool == nil || assets != nil && assets.store != store {
		return nil, ErrOpsInvalid
	}
	return &OpsEconomyService{store: store, assets: assets}, nil
}

type OpsEconomyOverview struct {
	ObservedAt               time.Time `json:"observed_at"`
	WalletAccounts           string    `json:"wallet_accounts"`
	ReserveUnits             string    `json:"reserve_units"`
	AvailableChipsUnits      string    `json:"available_chips_units"`
	Transactions             string    `json:"transactions"`
	PendingTransfers         string    `json:"pending_transfers"`
	PendingTransferUnits     string    `json:"pending_transfer_units"`
	NeedsReviewTransfers     string    `json:"needs_review_transfers"`
	NeedsReviewTransferUnits string    `json:"needs_review_transfer_units"`
}

func (s *OpsEconomyService) Overview(ctx context.Context, actor int64) (OpsEconomyOverview, error) {
	var result OpsEconomyOverview
	if s == nil || s.store == nil {
		return result, ErrOpsUnavailable
	}
	if _, err := requireOpsPermission(ctx, s.store.pool, actor, 0, "economy.read", false); err != nil {
		return result, err
	}
	err := s.store.pool.QueryRow(ctx, `SELECT clock_timestamp(),
	 (SELECT count(DISTINCT newapi_user_id)::text FROM economy.wallet_balances),
	 (SELECT coalesce(sum(balance_units),0)::text FROM economy.wallet_balances WHERE asset_type='RESERVE_API_CREDIT'),
	 (SELECT coalesce(sum(balance_units),0)::text FROM economy.wallet_balances WHERE asset_type='AVAILABLE_CHIPS'),
	 (SELECT count(*)::text FROM economy.asset_transactions),
	 (SELECT count(*)::text FROM economy.quota_transfers WHERE status='PENDING'),
	 (SELECT coalesce(sum(amount_units),0)::text FROM economy.quota_transfers WHERE status='PENDING'),
	 (SELECT count(*)::text FROM economy.quota_transfers WHERE status='NEEDS_REVIEW'),
	 (SELECT coalesce(sum(amount_units),0)::text FROM economy.quota_transfers WHERE status='NEEDS_REVIEW')`).Scan(
		&result.ObservedAt, &result.WalletAccounts, &result.ReserveUnits, &result.AvailableChipsUnits,
		&result.Transactions, &result.PendingTransfers, &result.PendingTransferUnits,
		&result.NeedsReviewTransfers, &result.NeedsReviewTransferUnits)
	result.ObservedAt = result.ObservedAt.UTC()
	return result, err
}

type OpsEconomyMigration struct {
	MigrationBatchID    *string   `json:"migration_batch_id"`
	AccountCreatedAt    time.Time `json:"account_created_at"`
	FirstSeenAt         time.Time `json:"first_seen_at"`
	RegistrationClaimID *string   `json:"registration_claim_id"`
	RegistrationStatus  string    `json:"registration_status"`
	RegistrationTxID    *string   `json:"registration_transaction_id"`
}

type OpsEconomyUser struct {
	ObservedAt time.Time           `json:"observed_at"`
	Assets     UnifiedAssets       `json:"assets"`
	Wallets    []Wallet            `json:"wallets"`
	Migration  OpsEconomyMigration `json:"migration"`
}

func (s *OpsEconomyService) User(ctx context.Context, actor, user int64) (OpsEconomyUser, error) {
	var result OpsEconomyUser
	if s == nil || s.store == nil {
		return result, ErrOpsUnavailable
	}
	if _, err := requireOpsPermission(ctx, s.store.pool, actor, 0, "economy.read", false); err != nil {
		return result, err
	}
	if !validateQuotaUser(user) {
		return result, ErrOpsInvalid
	}
	err := s.store.pool.QueryRow(ctx, `SELECT a.migration_batch_id::text,a.created_at,a.first_seen_at,
	 g.claim_id::text,coalesce(g.status,''),g.transaction_id::text
	 FROM identity.account_refs a LEFT JOIN rewards.registration_grants g USING(newapi_user_id)
	 WHERE a.newapi_user_id=$1`, user).Scan(&result.Migration.MigrationBatchID,
		&result.Migration.AccountCreatedAt, &result.Migration.FirstSeenAt,
		&result.Migration.RegistrationClaimID, &result.Migration.RegistrationStatus,
		&result.Migration.RegistrationTxID)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrOpsNotFound
	}
	if err != nil {
		return result, err
	}
	if s.assets == nil {
		return result, ErrOpsUnavailable
	}
	result.Assets, err = s.assets.ReadUnifiedAssets(ctx, user)
	if err != nil {
		return result, err
	}
	result.Wallets, err = s.store.ReadWallets(ctx, user)
	if err != nil {
		return result, err
	}
	result.ObservedAt = result.Assets.ObservedAt
	result.Migration.AccountCreatedAt = result.Migration.AccountCreatedAt.UTC()
	result.Migration.FirstSeenAt = result.Migration.FirstSeenAt.UTC()
	return result, nil
}

type OpsLedgerQuery struct {
	UserID   int64
	Before   time.Time
	BeforeID string
	Limit    int
}

type OpsLedgerPage struct {
	Items        []LedgerEntry `json:"items"`
	NextBefore   *time.Time    `json:"next_before,omitempty"`
	NextBeforeID string        `json:"next_before_id,omitempty"`
}

func (s *OpsEconomyService) Ledger(ctx context.Context, actor int64, query OpsLedgerQuery) (OpsLedgerPage, error) {
	page := OpsLedgerPage{Items: []LedgerEntry{}}
	if s == nil || s.store == nil {
		return page, ErrOpsUnavailable
	}
	if _, err := requireOpsPermission(ctx, s.store.pool, actor, 0, "economy.read", false); err != nil {
		return page, err
	}
	if !validateQuotaUser(query.UserID) || query.Limit < 1 || query.Limit > 100 ||
		query.Before.IsZero() != (query.BeforeID == "") || query.BeforeID != "" && !opsUUIDPattern.MatchString(query.BeforeID) {
		return page, ErrOpsInvalid
	}
	rows, err := s.store.pool.Query(ctx, `SELECT `+ledgerColumns+` FROM economy.wallet_ledger
	 WHERE newapi_user_id=$1 AND ($2::timestamptz IS NULL OR (created_at,ledger_entry_id)<($2,NULLIF($3,'')::uuid))
	 ORDER BY created_at DESC,ledger_entry_id DESC LIMIT $4`, query.UserID, nullableTime(query.Before), query.BeforeID, query.Limit)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		item, scanErr := scanEntry(rows)
		if scanErr != nil {
			return page, scanErr
		}
		item.CreatedAt = item.CreatedAt.UTC()
		page.Items = append(page.Items, item)
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) == query.Limit {
		last := page.Items[len(page.Items)-1]
		at := last.CreatedAt
		page.NextBefore, page.NextBeforeID = &at, last.ID
	}
	return page, nil
}

type OpsTransferQuery struct {
	UserID   int64
	Status   string
	Before   time.Time
	BeforeID string
	Limit    int
}

type OpsQuotaTransfer struct {
	QuotaTransfer
	Version string `json:"version"`
}

type OpsTransferPage struct {
	Items        []OpsQuotaTransfer `json:"items"`
	NextBefore   *time.Time         `json:"next_before,omitempty"`
	NextBeforeID string             `json:"next_before_id,omitempty"`
}

func opsTimeVersion(value time.Time) string {
	return strconv.FormatInt(value.UTC().UnixNano(), 10)
}

func (s *OpsEconomyService) Transfers(ctx context.Context, actor int64, query OpsTransferQuery) (OpsTransferPage, error) {
	page := OpsTransferPage{Items: []OpsQuotaTransfer{}}
	if s == nil || s.store == nil {
		return page, ErrOpsUnavailable
	}
	if _, err := requireOpsPermission(ctx, s.store.pool, actor, 0, "economy.read", false); err != nil {
		return page, err
	}
	if query.UserID < 0 || query.UserID > math.MaxInt32 || query.Limit < 1 || query.Limit > 100 ||
		query.Status != "" && !containsString([]string{"PENDING", "CONFIRMED", "REFUNDED", "NEEDS_REVIEW"}, query.Status) ||
		query.Before.IsZero() != (query.BeforeID == "") || query.BeforeID != "" && !opsUUIDPattern.MatchString(query.BeforeID) {
		return page, ErrOpsInvalid
	}
	rows, err := s.store.pool.Query(ctx, `SELECT `+transferColumns+` FROM economy.quota_transfers
	 WHERE ($1=0 OR newapi_user_id=$1) AND ($2='' OR status=$2)
	  AND ($3::timestamptz IS NULL OR (created_at,transfer_id)<($3,NULLIF($4,'')::uuid))
	 ORDER BY created_at DESC,transfer_id DESC LIMIT $5`, query.UserID, query.Status,
		nullableTime(query.Before), query.BeforeID, query.Limit)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		item, scanErr := scanTransfer(rows)
		if scanErr != nil {
			return page, scanErr
		}
		page.Items = append(page.Items, OpsQuotaTransfer{QuotaTransfer: item, Version: opsTimeVersion(item.UpdatedAt)})
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) == query.Limit {
		last := page.Items[len(page.Items)-1]
		at := last.CreatedAt
		page.NextBefore, page.NextBeforeID = &at, last.ID
	}
	return page, nil
}

type OpsRewardPolicy struct {
	Program         string `json:"program"`
	PolicyVersion   string `json:"policy_version"`
	Amount          string `json:"amount"`
	AmountUnits     int64  `json:"amount_units,string"`
	Asset           Asset  `json:"asset"`
	Editable        bool   `json:"editable"`
	Schedule        string `json:"schedule"`
	CooldownSeconds *int64 `json:"cooldown_seconds,string,omitempty"`
	Threshold       string `json:"threshold,omitempty"`
	ThresholdUnits  *int64 `json:"threshold_units,string,omitempty"`
}

func (s *OpsEconomyService) RewardPolicies(ctx context.Context, actor int64) ([]OpsRewardPolicy, error) {
	if s == nil || s.store == nil {
		return nil, ErrOpsUnavailable
	}
	if _, err := requireOpsPermission(ctx, s.store.pool, actor, 0, "rewards.read", false); err != nil {
		return nil, err
	}
	cooldown := int64(ReliefCooldown / time.Second)
	threshold := ReliefAssetThreshold
	return []OpsRewardPolicy{
		{Program: "REGISTRATION", PolicyVersion: "1", Amount: FormatAmount(RegistrationGrantAmount), AmountUnits: RegistrationGrantAmount, Asset: ReserveAPICredit, Schedule: "ONE_TIME_ON_ACCEPTED_REGISTRATION"},
		{Program: "DAILY", PolicyVersion: "1", Amount: FormatAmount(DailyAmount), AmountUnits: DailyAmount, Asset: ReserveAPICredit, Schedule: "ASIA_SHANGHAI_NATURAL_DAY"},
		{Program: "HOURLY", PolicyVersion: "1", Amount: FormatAmount(HourlyRewardAmount), AmountUnits: HourlyRewardAmount, Asset: ReserveAPICredit, Schedule: "ASIA_SHANGHAI_NATURAL_HOUR_MAX_24_PER_DAY"},
		{Program: "RELIEF", PolicyVersion: "1", Amount: FormatAmount(ReliefRewardAmount), AmountUnits: ReliefRewardAmount, Asset: ReserveAPICredit, Schedule: "ROLLING_AFTER_SUCCESS", CooldownSeconds: &cooldown, Threshold: FormatAmount(threshold), ThresholdUnits: &threshold},
	}, nil
}

type OpsRewardClaimQuery struct {
	UserID   int64
	Program  string
	Status   string
	Before   time.Time
	BeforeID string
	Limit    int
}

type OpsRewardClaim struct {
	ID               string     `json:"id"`
	Program          string     `json:"program"`
	UserID           int64      `json:"newapi_user_id,string"`
	PolicyVersion    string     `json:"policy_version"`
	Status           string     `json:"status"`
	Amount           string     `json:"amount"`
	AmountUnits      int64      `json:"amount_units,string"`
	Asset            Asset      `json:"asset"`
	TransactionID    *string    `json:"transaction_id"`
	BusinessID       string     `json:"business_id"`
	CreatedAt        time.Time  `json:"created_at"`
	ClaimedAt        *time.Time `json:"claimed_at,omitempty"`
	Attempts         int64      `json:"attempts,string"`
	LastErrorCode    string     `json:"last_error_code,omitempty"`
	NextActionAt     *time.Time `json:"next_action_at,omitempty"`
	Retryable        bool       `json:"retryable"`
	TotalAssetsUnits *int64     `json:"total_assets_units,string,omitempty"`
	AssetsObservedAt *time.Time `json:"assets_observed_at,omitempty"`
	Version          string     `json:"version"`
}

type OpsRewardClaimPage struct {
	Items        []OpsRewardClaim `json:"items"`
	NextBefore   *time.Time       `json:"next_before,omitempty"`
	NextBeforeID string           `json:"next_before_id,omitempty"`
}

func (s *OpsEconomyService) RewardClaims(ctx context.Context, actor int64, query OpsRewardClaimQuery) (OpsRewardClaimPage, error) {
	page := OpsRewardClaimPage{Items: []OpsRewardClaim{}}
	if s == nil || s.store == nil {
		return page, ErrOpsUnavailable
	}
	if _, err := requireOpsPermission(ctx, s.store.pool, actor, 0, "rewards.read", false); err != nil {
		return page, err
	}
	if query.UserID < 0 || query.UserID > math.MaxInt32 || query.Limit < 1 || query.Limit > 100 ||
		query.Program != "" && !containsString([]string{"REGISTRATION", "DAILY", "HOURLY", "RELIEF"}, query.Program) ||
		query.Status != "" && !containsString([]string{"PENDING", "RECOVERING", "SUCCEEDED"}, query.Status) ||
		query.Before.IsZero() != (query.BeforeID == "") || query.BeforeID != "" && !opsUUIDPattern.MatchString(query.BeforeID) {
		return page, ErrOpsInvalid
	}
	rows, err := s.store.pool.Query(ctx, `WITH claims AS (
	 SELECT g.claim_id::text AS id,'REGISTRATION'::text AS program,g.newapi_user_id,n.policy_version,
	  CASE WHEN g.status='CONFIRMED' THEN 'SUCCEEDED' ELSE g.status END AS status,
	  500000000::bigint AS amount_units,'RESERVE_API_CREDIT'::text AS asset_type,g.transaction_id::text,
	  g.biz_id AS business_id,g.created_at,g.confirmed_at AS claimed_at,j.attempts,
	  coalesce(j.last_error_code,'') AS last_error_code,j.next_attempt_at,
	  g.status<>'CONFIRMED' AND j.status<>'DONE' AS retryable,NULL::bigint AS total_assets_units,
	  NULL::timestamptz AS assets_observed_at,j.updated_at AS version_at
	 FROM rewards.registration_grants g JOIN identity.native_registration_inbox n ON n.ordinal=g.source_ordinal
	 JOIN platform_meta.registration_grant_jobs j USING(claim_id)
	 UNION ALL
	 SELECT d.transaction_id::text,'DAILY',d.newapi_user_id,d.policy_version::text,'SUCCEEDED',d.amount_units,
	  d.asset_type,d.transaction_id::text,t.biz_id,d.created_at,d.created_at,0,'',NULL,false,NULL,NULL,d.created_at
	 FROM rewards.daily_checkins d JOIN economy.asset_transactions t USING(transaction_id)
	 UNION ALL
	 SELECT h.transaction_id::text,'HOURLY',h.newapi_user_id,h.policy_version::text,'SUCCEEDED',h.amount_units,
	  h.asset_type,h.transaction_id::text,t.biz_id,h.created_at,h.created_at,0,'',NULL,false,NULL,NULL,h.created_at
	 FROM rewards.hourly_claims h JOIN economy.asset_transactions t USING(transaction_id)
	 UNION ALL
	 SELECT r.relief_claim_id::text,'RELIEF',r.newapi_user_id,r.policy_version::text,'SUCCEEDED',r.amount_units,
	  r.asset_type,r.transaction_id::text,t.biz_id,r.claimed_at,r.claimed_at,0,'',r.cooldown_until,false,
	  r.total_assets_units,r.assets_observed_at,r.claimed_at
	 FROM rewards.relief_claims r JOIN economy.asset_transactions t USING(transaction_id)
	 ) SELECT id,program,newapi_user_id,policy_version,status,amount_units,asset_type,transaction_id,
	 business_id,created_at,claimed_at,attempts,last_error_code,next_attempt_at,retryable,total_assets_units,
	 assets_observed_at,version_at FROM claims
	 WHERE ($1=0 OR newapi_user_id=$1) AND ($2='' OR program=$2) AND ($3='' OR status=$3)
	  AND ($4::timestamptz IS NULL OR (created_at,id)<($4,$5))
	 ORDER BY created_at DESC,id DESC LIMIT $6`, query.UserID, query.Program, query.Status,
		nullableTime(query.Before), query.BeforeID, query.Limit)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var item OpsRewardClaim
		var versionAt time.Time
		if err = rows.Scan(&item.ID, &item.Program, &item.UserID, &item.PolicyVersion, &item.Status,
			&item.AmountUnits, &item.Asset, &item.TransactionID, &item.BusinessID, &item.CreatedAt,
			&item.ClaimedAt, &item.Attempts, &item.LastErrorCode, &item.NextActionAt, &item.Retryable,
			&item.TotalAssetsUnits, &item.AssetsObservedAt, &versionAt); err != nil {
			return page, err
		}
		item.Amount = FormatAmount(item.AmountUnits)
		item.CreatedAt = item.CreatedAt.UTC()
		item.Version = opsTimeVersion(versionAt)
		if item.ClaimedAt != nil {
			value := item.ClaimedAt.UTC()
			item.ClaimedAt = &value
		}
		if item.NextActionAt != nil {
			value := item.NextActionAt.UTC()
			item.NextActionAt = &value
		}
		if item.AssetsObservedAt != nil {
			value := item.AssetsObservedAt.UTC()
			item.AssetsObservedAt = &value
		}
		page.Items = append(page.Items, item)
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) == query.Limit {
		last := page.Items[len(page.Items)-1]
		at := last.CreatedAt
		page.NextBefore, page.NextBeforeID = &at, last.ID
	}
	return page, nil
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func decodeOpsInput(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return ErrOpsInvalid
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return ErrOpsInvalid
	}
	return nil
}

func canonicalTargetUser(target OpsTarget) (int64, error) {
	user, err := strconv.ParseInt(target.ID, 10, 64)
	if err != nil || user <= 0 || strconv.FormatInt(user, 10) != target.ID {
		return 0, ErrOpsInvalid
	}
	return user, nil
}

func rawOpsValue(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}

func affectedOne() *int64 {
	value := int64(1)
	return &value
}

type OpsEconomyAdjustmentInput struct {
	Asset      Asset  `json:"asset"`
	DeltaUnits string `json:"delta_units"`
	Reference  string `json:"reference"`
}

func normalizeAdjustmentInput(input OpsEconomyAdjustmentInput) (OpsEconomyAdjustmentInput, int64, error) {
	delta, err := strconv.ParseInt(input.DeltaUnits, 10, 64)
	if err != nil || delta == 0 || strconv.FormatInt(delta, 10) != input.DeltaUnits ||
		!validAsset(input.Asset) || !validOpsText(input.Reference, 512, true) {
		return input, 0, ErrOpsInvalid
	}
	return input, delta, nil
}

type opsWalletSnapshot struct {
	UserID       int64  `json:"newapi_user_id,string"`
	Asset        Asset  `json:"asset"`
	Balance      string `json:"balance"`
	BalanceUnits int64  `json:"balance_units,string"`
	Version      int64  `json:"version,string"`
}

func walletSnapshot(user int64, asset Asset, balance, version int64) opsWalletSnapshot {
	return opsWalletSnapshot{UserID: user, Asset: asset, Balance: FormatAmount(balance), BalanceUnits: balance, Version: version}
}

type economyAdjustmentOpsHandler struct{ store *Store }

func (economyAdjustmentOpsHandler) Canonicalize(raw json.RawMessage) (json.RawMessage, error) {
	var input OpsEconomyAdjustmentInput
	if err := decodeOpsInput(raw, &input); err != nil {
		return nil, err
	}
	input, _, err := normalizeAdjustmentInput(input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(input)
}

func (handler economyAdjustmentOpsHandler) Prepare(ctx context.Context, tx pgx.Tx, _ OpsPrincipal, request OpsPrepareRequest, lock bool) (OpsPreparedMaterial, error) {
	if handler.store == nil || tx == nil {
		return OpsPreparedMaterial{}, ErrOpsUnavailable
	}
	user, err := canonicalTargetUser(request.Target)
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	var input OpsEconomyAdjustmentInput
	if err = decodeOpsInput(request.Input, &input); err != nil {
		return OpsPreparedMaterial{}, err
	}
	input, delta, err := normalizeAdjustmentInput(input)
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	if lock {
		if err = LockEconomyUsersInTx(ctx, tx, user); err != nil {
			return OpsPreparedMaterial{}, err
		}
		if err = lockIdentity(ctx, tx, "business", "ADMIN_ADJUSTMENT_V1", request.OperationID); err != nil {
			return OpsPreparedMaterial{}, err
		}
		key := sha256.Sum256([]byte(request.OperationID))
		if err = lockIdentity(ctx, tx, "idempotency", user, fmt.Sprintf("%x", key)); err != nil {
			return OpsPreparedMaterial{}, err
		}
	}
	query := `SELECT balance_units,version FROM economy.wallet_balances WHERE newapi_user_id=$1 AND asset_type=$2`
	if lock {
		query += " FOR UPDATE"
	}
	var balance, version int64
	err = tx.QueryRow(ctx, query, user, input.Asset).Scan(&balance, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return OpsPreparedMaterial{}, ErrOpsNotFound
	}
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	if request.Target.ExpectedVersion != strconv.FormatInt(version, 10) {
		return OpsPreparedMaterial{}, ErrOpsPreviewStale
	}
	if delta < 0 && delta < -balance || delta > 0 && balance > math.MaxInt64-delta || version == math.MaxInt64 {
		return OpsPreparedMaterial{}, ErrOpsConflict
	}
	if delta > 0 {
		policy, e := ActiveEconomicPolicyInTx(ctx, tx)
		if e != nil {
			return OpsPreparedMaterial{}, e
		}
		if policy.Version != "" {
			assets, e := ReadUnifiedAssetsInTx(ctx, tx, handler.store.economicObserver, user)
			if e != nil {
				return OpsPreparedMaterial{}, e
			}
			if assets.TotalUnits >= policy.AssetCapUnits || delta > policy.AssetCapUnits-assets.TotalUnits {
				return OpsPreparedMaterial{}, ErrAssetCap
			}
		}
	}
	after := balance + delta
	beforeSnapshot := walletSnapshot(user, input.Asset, balance, version)
	afterSnapshot := walletSnapshot(user, input.Asset, after, version+1)
	deltaSnapshot := struct {
		DeltaUnits string `json:"delta_units"`
		Reference  string `json:"reference"`
	}{input.DeltaUnits, input.Reference}
	impact := OpsImpact{
		CurrentState: rawOpsValue(beforeSnapshot), ProposedChange: rawOpsValue(afterSnapshot),
		Before: rawOpsValue(beforeSnapshot), Delta: rawOpsValue(deltaSnapshot), After: rawOpsValue(afterSnapshot),
		AffectedUsers: affectedOne(), AffectedItems: affectedOne(), BlockingFacts: []string{},
		ContinuingAcceptedWork: []string{}, RelatedIDs: []string{request.Target.ID}, UnavailableMeasurements: []string{},
	}
	return OpsPreparedMaterial{Impact: impact, TargetVersion: strconv.FormatInt(version, 10),
		TargetLocator: request.Target.ID + ":" + string(input.Asset)}, nil
}

func (handler economyAdjustmentOpsHandler) Execute(ctx context.Context, tx pgx.Tx, _ OpsPrincipal, operation OpsOperation, _ OpsPreparedMaterial) (OpsExecutionResult, error) {
	user, err := canonicalTargetUser(operation.Target)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	var input OpsEconomyAdjustmentInput
	if err = decodeOpsInput(operation.inputPayload, &input); err != nil {
		return OpsExecutionResult{}, err
	}
	input, delta, err := normalizeAdjustmentInput(input)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	entryType := "ADMIN_ADJUSTMENT_ISSUE"
	if delta < 0 {
		entryType = "ADMIN_ADJUSTMENT_BURN"
	}
	entry, err := applyInTx(ctx, tx, Mutation{UserID: user, Asset: input.Asset, DeltaUnits: delta,
		BizType: "ADMIN_ADJUSTMENT_V1", BizID: operation.OperationID, EntryType: entryType,
		IdempotencyKey: operation.OperationID})
	if err != nil {
		return OpsExecutionResult{}, err
	}
	before := walletSnapshot(user, input.Asset, entry.BalanceBeforeUnits, entry.WalletVersion-1)
	after := walletSnapshot(user, input.Asset, entry.BalanceAfterUnits, entry.WalletVersion)
	result := rawOpsValue(struct {
		TransactionID string `json:"transaction_id"`
		LedgerEntryID string `json:"ledger_entry_id"`
		Reference     string `json:"reference"`
	}{entry.TransactionID, entry.ID, input.Reference})
	return OpsExecutionResult{BeforeSnapshot: rawOpsValue(before), AfterSnapshot: rawOpsValue(after),
		Result: result, RelatedBusinessID: operation.OperationID}, nil
}

type OpsEconomyReconcileInput struct {
	Action string `json:"action"`
}

func normalizeReconcileInput(input OpsEconomyReconcileInput) (OpsEconomyReconcileInput, error) {
	if input.Action != "RESUME" && input.Action != "MARK_FOR_REVIEW" {
		return input, ErrOpsInvalid
	}
	return input, nil
}

type economyReconcileOpsHandler struct{ store *Store }

func (economyReconcileOpsHandler) Canonicalize(raw json.RawMessage) (json.RawMessage, error) {
	var input OpsEconomyReconcileInput
	if err := decodeOpsInput(raw, &input); err != nil {
		return nil, err
	}
	input, err := normalizeReconcileInput(input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(input)
}

func loadOpsTransfer(ctx context.Context, tx pgx.Tx, id string, lock bool) (QuotaTransfer, error) {
	query := `SELECT ` + transferColumns + ` FROM economy.quota_transfers WHERE transfer_id=$1`
	if lock {
		query += " FOR UPDATE"
	}
	result, err := scanTransfer(tx.QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrOpsNotFound
	}
	return result, err
}

func (handler economyReconcileOpsHandler) Prepare(ctx context.Context, tx pgx.Tx, _ OpsPrincipal, request OpsPrepareRequest, lock bool) (OpsPreparedMaterial, error) {
	if handler.store == nil || tx == nil || !opsUUIDPattern.MatchString(request.Target.ID) {
		return OpsPreparedMaterial{}, ErrOpsInvalid
	}
	var input OpsEconomyReconcileInput
	if err := decodeOpsInput(request.Input, &input); err != nil {
		return OpsPreparedMaterial{}, err
	}
	input, err := normalizeReconcileInput(input)
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	current, err := loadOpsTransfer(ctx, tx, request.Target.ID, false)
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	if lock {
		// Match refill, exchange, assets and both quota-worker entry points.
		if err = lockIdentity(ctx, tx, "quota-transfer-user", current.UserID); err != nil {
			return OpsPreparedMaterial{}, err
		}
		current, err = loadOpsTransfer(ctx, tx, request.Target.ID, true)
		if err != nil {
			return OpsPreparedMaterial{}, err
		}
	}
	version := opsTimeVersion(current.UpdatedAt)
	if request.Target.ExpectedVersion != version {
		return OpsPreparedMaterial{}, ErrOpsPreviewStale
	}
	nextStatus, nextReason := "NEEDS_REVIEW", "ADMIN_MARKED_FOR_REVIEW"
	continuing := []string{}
	if input.Action == "RESUME" {
		if current.Status != "NEEDS_REVIEW" {
			return OpsPreparedMaterial{}, ErrOpsConflict
		}
		nextStatus, nextReason = "PENDING", "ADMIN_RESUME"
		continuing = []string{"quota_transfer_worker"}
	} else if current.Status != "PENDING" {
		return OpsPreparedMaterial{}, ErrOpsConflict
	}
	proposed := struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Reason string `json:"reason"`
	}{current.ID, nextStatus, nextReason}
	impact := OpsImpact{CurrentState: rawOpsValue(OpsQuotaTransfer{QuotaTransfer: current, Version: version}),
		ProposedChange: rawOpsValue(proposed), AffectedUsers: affectedOne(), AffectedItems: affectedOne(),
		BlockingFacts: []string{}, ContinuingAcceptedWork: continuing, RelatedIDs: []string{current.ID},
		UnavailableMeasurements: []string{}}
	return OpsPreparedMaterial{Impact: impact, TargetVersion: version, TargetLocator: current.ID}, nil
}

func (handler economyReconcileOpsHandler) Execute(ctx context.Context, tx pgx.Tx, _ OpsPrincipal, operation OpsOperation, _ OpsPreparedMaterial) (OpsExecutionResult, error) {
	var input OpsEconomyReconcileInput
	if err := decodeOpsInput(operation.inputPayload, &input); err != nil {
		return OpsExecutionResult{}, err
	}
	input, err := normalizeReconcileInput(input)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	before, err := loadOpsTransfer(ctx, tx, operation.Target.ID, false)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	status, reason := "NEEDS_REVIEW", "ADMIN_MARKED_FOR_REVIEW"
	if input.Action == "RESUME" {
		status, reason = "PENDING", "ADMIN_RESUME"
	}
	after, err := scanTransfer(tx.QueryRow(ctx, `UPDATE economy.quota_transfers SET status=$2,reason=$3,
	 native_before=CASE WHEN $2='PENDING' THEN NULL ELSE native_before END,
	 native_after=CASE WHEN $2='PENDING' THEN NULL ELSE native_after END,updated_at=clock_timestamp()
	 WHERE transfer_id=$1 RETURNING `+transferColumns, operation.Target.ID, status, reason))
	if err != nil {
		return OpsExecutionResult{}, err
	}
	result := rawOpsValue(struct {
		Transfer OpsQuotaTransfer `json:"transfer"`
	}{OpsQuotaTransfer{QuotaTransfer: after, Version: opsTimeVersion(after.UpdatedAt)}})
	return OpsExecutionResult{BeforeSnapshot: rawOpsValue(OpsQuotaTransfer{QuotaTransfer: before, Version: opsTimeVersion(before.UpdatedAt)}),
		AfterSnapshot: rawOpsValue(OpsQuotaTransfer{QuotaTransfer: after, Version: opsTimeVersion(after.UpdatedAt)}),
		Result:        result, RelatedBusinessID: after.ID}, nil
}

type OpsRewardRetryInput struct {
	Action string `json:"action"`
}

type opsRegistrationRetryState struct {
	ClaimID     string    `json:"claim_id"`
	UserID      int64     `json:"newapi_user_id,string"`
	ClaimStatus string    `json:"claim_status"`
	JobStatus   string    `json:"job_status"`
	Attempts    int64     `json:"attempts,string"`
	LastError   string    `json:"last_error_code,omitempty"`
	NextAttempt time.Time `json:"next_attempt_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Version     string    `json:"version"`
}

func loadRegistrationRetry(ctx context.Context, tx pgx.Tx, id string, lock bool) (opsRegistrationRetryState, error) {
	var result opsRegistrationRetryState
	query := `SELECT g.claim_id::text,g.newapi_user_id,g.status,j.status,j.attempts,
	 coalesce(j.last_error_code,''),j.next_attempt_at,j.updated_at
	 FROM rewards.registration_grants g JOIN platform_meta.registration_grant_jobs j USING(claim_id)
	 WHERE g.claim_id=$1`
	if lock {
		query += " FOR UPDATE OF g,j"
	}
	err := tx.QueryRow(ctx, query, id).Scan(&result.ClaimID, &result.UserID, &result.ClaimStatus,
		&result.JobStatus, &result.Attempts, &result.LastError, &result.NextAttempt, &result.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrOpsNotFound
	}
	if err != nil {
		return result, err
	}
	result.NextAttempt = result.NextAttempt.UTC()
	result.UpdatedAt = result.UpdatedAt.UTC()
	result.Version = opsTimeVersion(result.UpdatedAt)
	return result, nil
}

type rewardRetryOpsHandler struct{ store *Store }

func (rewardRetryOpsHandler) Canonicalize(raw json.RawMessage) (json.RawMessage, error) {
	var input OpsRewardRetryInput
	if err := decodeOpsInput(raw, &input); err != nil || input.Action != "RETRY" {
		return nil, ErrOpsInvalid
	}
	return json.Marshal(input)
}

func (handler rewardRetryOpsHandler) Prepare(ctx context.Context, tx pgx.Tx, _ OpsPrincipal, request OpsPrepareRequest, lock bool) (OpsPreparedMaterial, error) {
	if handler.store == nil || tx == nil || !opsUUIDPattern.MatchString(request.Target.ID) {
		return OpsPreparedMaterial{}, ErrOpsInvalid
	}
	var input OpsRewardRetryInput
	if err := decodeOpsInput(request.Input, &input); err != nil || input.Action != "RETRY" {
		return OpsPreparedMaterial{}, ErrOpsInvalid
	}
	current, err := loadRegistrationRetry(ctx, tx, request.Target.ID, lock)
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	if current.ClaimStatus == "CONFIRMED" || current.JobStatus == "DONE" ||
		!containsString([]string{"PENDING", "RECOVERING"}, current.ClaimStatus) ||
		!containsString([]string{"PENDING", "RECOVERING"}, current.JobStatus) {
		return OpsPreparedMaterial{}, ErrOpsConflict
	}
	if request.Target.ExpectedVersion != current.Version {
		return OpsPreparedMaterial{}, ErrOpsPreviewStale
	}
	proposed := struct {
		ClaimID string `json:"claim_id"`
		Action  string `json:"action"`
		Queue   string `json:"queue"`
	}{current.ClaimID, "RETRY", "IMMEDIATE"}
	impact := OpsImpact{CurrentState: rawOpsValue(current), ProposedChange: rawOpsValue(proposed),
		AffectedUsers: affectedOne(), AffectedItems: affectedOne(), BlockingFacts: []string{},
		ContinuingAcceptedWork: []string{"registration_grant_worker"}, RelatedIDs: []string{current.ClaimID},
		UnavailableMeasurements: []string{}}
	return OpsPreparedMaterial{Impact: impact, TargetVersion: current.Version, TargetLocator: current.ClaimID}, nil
}

func (handler rewardRetryOpsHandler) Execute(ctx context.Context, tx pgx.Tx, _ OpsPrincipal, operation OpsOperation, _ OpsPreparedMaterial) (OpsExecutionResult, error) {
	before, err := loadRegistrationRetry(ctx, tx, operation.Target.ID, false)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE platform_meta.registration_grant_jobs
	 SET next_attempt_at=clock_timestamp(),updated_at=clock_timestamp() WHERE claim_id=$1`, operation.Target.ID); err != nil {
		return OpsExecutionResult{}, err
	}
	after, err := loadRegistrationRetry(ctx, tx, operation.Target.ID, false)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	result := rawOpsValue(struct {
		Claim opsRegistrationRetryState `json:"claim"`
	}{after})
	return OpsExecutionResult{BeforeSnapshot: rawOpsValue(before), AfterSnapshot: rawOpsValue(after),
		Result: result, RelatedBusinessID: after.ClaimID}, nil
}

func (s *OpsEconomyService) OpsBindings() []OpsOperationBinding {
	if s == nil || s.store == nil {
		return []OpsOperationBinding{}
	}
	return []OpsOperationBinding{
		{Descriptor: OpsOperationDescriptor{OperationType: "ECONOMY_ADJUSTMENT", Risk: OpsRiskCritical,
			RequiredPermission: "economy.adjust", AllowedRoles: []string{"SUPER_ADMIN"}, TargetType: "WALLET",
			InputSchemaVersion: "economy-adjustment.v1", ImpactSchemaVersion: "economy-adjustment-impact.v1",
			RequiresReason: true, RequiresFreshAuth: true, ConfirmationMode: OpsConfirmTyped, ExecutionMode: OpsSameDatabase},
			Handler: economyAdjustmentOpsHandler{store: s.store}},
		{Descriptor: OpsOperationDescriptor{OperationType: "ECONOMY_TRANSFER_RECONCILE", Risk: OpsRiskCritical,
			RequiredPermission: "economy.reconciliation.write", AllowedRoles: []string{"SUPER_ADMIN"}, TargetType: "QUOTA_TRANSFER",
			InputSchemaVersion: "economy-transfer-reconcile.v1", ImpactSchemaVersion: "economy-transfer-reconcile-impact.v1",
			RequiresReason: true, RequiresFreshAuth: true, ConfirmationMode: OpsConfirmTyped, ExecutionMode: OpsSameDatabase},
			Handler: economyReconcileOpsHandler{store: s.store}},
		{Descriptor: OpsOperationDescriptor{OperationType: "REWARD_CLAIM_RETRY", Risk: OpsRiskImpactful,
			RequiredPermission: "rewards.claim.retry", AllowedRoles: []string{"SUPER_ADMIN", "OPERATOR"}, TargetType: "REWARD_CLAIM",
			InputSchemaVersion: "reward-claim-retry.v1", ImpactSchemaVersion: "reward-claim-retry-impact.v1",
			RequiresReason: true, ConfirmationMode: OpsConfirmExplicit, ExecutionMode: OpsSameDatabase},
			Handler: rewardRetryOpsHandler{store: s.store}},
	}
}
