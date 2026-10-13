package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// OpsEconomyService reads the local wallet and reward authorities.
type OpsEconomyService struct{ store *Store }

func NewOpsEconomyService(store *Store) (*OpsEconomyService, error) {
	if store == nil || store.pool == nil {
		return nil, ErrOpsUnavailable
	}
	return &OpsEconomyService{store: store}, nil
}

type OpsEconomyOverview struct {
	ObservedAt          time.Time `json:"observed_at"`
	WalletAccounts      string    `json:"wallet_accounts"`
	ReserveUnits        string    `json:"reserve_units"`
	AvailableChipsUnits string    `json:"available_chips_units"`
	Transactions        string    `json:"transactions"`
}

func (s *OpsEconomyService) Overview(ctx context.Context, actor int64) (OpsEconomyOverview, error) {
	var result OpsEconomyOverview
	if s == nil || s.store == nil {
		return result, ErrOpsUnavailable
	}
	if err := catalogAdmin(ctx, s.store.pool, actor, false); err != nil {
		return result, err
	}
	err := s.store.pool.QueryRow(ctx, `SELECT clock_timestamp(),
	 (SELECT count(DISTINCT newapi_user_id)::text FROM economy.wallet_balances),
	 (SELECT coalesce(sum(balance_units),0)::text FROM economy.wallet_balances WHERE asset_type='RESERVE_API_CREDIT'),
	 (SELECT coalesce(sum(balance_units),0)::text FROM economy.wallet_balances WHERE asset_type='AVAILABLE_CHIPS'),
     (SELECT count(*)::text FROM economy.asset_transactions)`).Scan(
		&result.ObservedAt, &result.WalletAccounts, &result.ReserveUnits, &result.AvailableChipsUnits, &result.Transactions)
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
	Assets     *UnifiedAssets      `json:"assets"`
	Migration  OpsEconomyMigration `json:"migration"`
}

func (s *OpsEconomyService) User(ctx context.Context, actor, user int64) (OpsEconomyUser, error) {
	var result OpsEconomyUser
	if s == nil || s.store == nil {
		return result, ErrOpsUnavailable
	}
	if err := catalogAdmin(ctx, s.store.pool, actor, false); err != nil {
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
	assets, err := NewUnifiedAssetReader(s.store).ReadUnifiedAssets(ctx, user)
	if err != nil && !errors.Is(err, ErrWalletNotFound) {
		return result, err
	}
	if err == nil {
		result.Assets = &assets
	}
	result.ObservedAt = time.Now().UTC()
	result.Migration.AccountCreatedAt = result.Migration.AccountCreatedAt.UTC()
	result.Migration.FirstSeenAt = result.Migration.FirstSeenAt.UTC()
	return result, nil
}

type OpsLedgerQuery struct {
	Asset    Asset
	BizType  string
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
	if err := catalogAdmin(ctx, s.store.pool, actor, false); err != nil {
		return page, err
	}
	if query.UserID < 0 || query.UserID > math.MaxInt32 || (query.Asset != "" && !validAsset(query.Asset)) || len(query.BizType) > 128 || query.Limit < 1 || query.Limit > 100 ||
		query.Before.IsZero() != (query.BeforeID == "") || query.BeforeID != "" && !opsUUIDPattern.MatchString(query.BeforeID) {
		return page, ErrOpsInvalid
	}
	rows, err := s.store.pool.Query(ctx, `SELECT `+ledgerColumns+` FROM economy.wallet_ledger
	 WHERE ($1=0 OR newapi_user_id=$1) AND ($2::timestamptz IS NULL OR (created_at,ledger_entry_id)<($2,NULLIF($3,'')::uuid))
     AND ($5='' OR asset_type=$5) AND ($6='' OR biz_type=$6)
     ORDER BY created_at DESC,ledger_entry_id DESC LIMIT $4`, query.UserID, nullableTime(query.Before), query.BeforeID, query.Limit, query.Asset, query.BizType)
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

type OpsTransferPage struct {
	Items        []QuotaTransfer `json:"items"`
	NextBefore   *time.Time      `json:"next_before,omitempty"`
	NextBeforeID string          `json:"next_before_id,omitempty"`
}

func (s *OpsEconomyService) Transfers(ctx context.Context, actor int64, query OpsTransferQuery) (OpsTransferPage, error) {
	page := OpsTransferPage{Items: []QuotaTransfer{}}
	if s == nil || s.store == nil {
		return page, ErrOpsUnavailable
	}
	if err := catalogAdmin(ctx, s.store.pool, actor, false); err != nil {
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
	if err := catalogAdmin(ctx, s.store.pool, actor, false); err != nil {
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
	if err := catalogAdmin(ctx, s.store.pool, actor, false); err != nil {
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
	  coalesce(i.amount_units,500000000)::bigint AS amount_units,'RESERVE_API_CREDIT'::text AS asset_type,g.transaction_id::text,
	  g.biz_id AS business_id,g.created_at,g.confirmed_at AS claimed_at,j.attempts,
	  coalesce(j.last_error_code,'') AS last_error_code,j.next_attempt_at,
	  g.status IN ('PENDING','RECOVERING') AND j.status IN ('PENDING','RECOVERING') AS retryable,NULL::bigint AS total_assets_units,
	  NULL::timestamptz AS assets_observed_at
	 FROM rewards.registration_grants g JOIN identity.native_registration_inbox n ON n.ordinal=g.source_ordinal
	 JOIN platform_meta.registration_grant_jobs j USING(claim_id)
     LEFT JOIN rewards.registration_issuances i USING(claim_id)
	 UNION ALL
	 SELECT d.transaction_id::text,'DAILY',d.newapi_user_id,d.policy_version::text,'SUCCEEDED',d.amount_units,
	  d.asset_type,d.transaction_id::text,t.biz_id,d.created_at,d.created_at,0,'',NULL,false,NULL,NULL
	 FROM rewards.daily_checkins d JOIN economy.asset_transactions t USING(transaction_id)
	 UNION ALL
	 SELECT h.transaction_id::text,'HOURLY',h.newapi_user_id,h.policy_version::text,'SUCCEEDED',h.amount_units,
	  h.asset_type,h.transaction_id::text,t.biz_id,h.created_at,h.created_at,0,'',NULL,false,NULL,NULL
	 FROM rewards.hourly_claims h JOIN economy.asset_transactions t USING(transaction_id)
	 UNION ALL
	 SELECT r.relief_claim_id::text,'RELIEF',r.newapi_user_id,r.policy_version::text,'SUCCEEDED',r.amount_units,
	  r.asset_type,r.transaction_id::text,t.biz_id,r.claimed_at,r.claimed_at,0,'',r.cooldown_until,false,
	  r.total_assets_units,r.assets_observed_at
	 FROM rewards.relief_claims r JOIN economy.asset_transactions t USING(transaction_id)
	 ) SELECT id,program,newapi_user_id,policy_version,status,amount_units,asset_type,transaction_id,
	 business_id,created_at,claimed_at,attempts,last_error_code,next_attempt_at,retryable,total_assets_units,
	 assets_observed_at FROM claims
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
		if err = rows.Scan(&item.ID, &item.Program, &item.UserID, &item.PolicyVersion, &item.Status,
			&item.AmountUnits, &item.Asset, &item.TransactionID, &item.BusinessID, &item.CreatedAt,
			&item.ClaimedAt, &item.Attempts, &item.LastErrorCode, &item.NextActionAt, &item.Retryable,
			&item.TotalAssetsUnits, &item.AssetsObservedAt); err != nil {
			return page, err
		}
		item.Amount = FormatAmount(item.AmountUnits)
		item.CreatedAt = item.CreatedAt.UTC()
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

type opsRegistrationRetryState struct {
	ClaimID     string    `json:"claim_id"`
	UserID      int64     `json:"newapi_user_id,string"`
	ClaimStatus string    `json:"claim_status"`
	JobStatus   string    `json:"job_status"`
	Attempts    int64     `json:"attempts,string"`
	LastError   string    `json:"last_error_code,omitempty"`
	NextAttempt time.Time `json:"next_attempt_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
	return result, nil
}

// Retry schedules the original registration job; the worker owns issuance and
// its existing business/idempotency keys. This endpoint never credits money.
func (s *OpsEconomyService) RetryReward(ctx context.Context, actor int64, id string) (opsRegistrationRetryState, error) {
	tx, err := s.store.pool.Begin(ctx)
	if err != nil {
		return opsRegistrationRetryState{}, err
	}
	defer rollback(tx)
	result, err := retryRegistrationReward(ctx, tx, actor, id)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func retryRegistrationReward(ctx context.Context, tx pgx.Tx, actor int64, id string) (opsRegistrationRetryState, error) {
	if !opsUUIDPattern.MatchString(id) {
		return opsRegistrationRetryState{}, ErrOpsInvalid
	}
	if err := catalogAdmin(ctx, tx, actor, true); err != nil {
		return opsRegistrationRetryState{}, err
	}
	before, err := loadRegistrationRetry(ctx, tx, id, true)
	if err != nil {
		return before, err
	}
	if !containsString([]string{"PENDING", "RECOVERING"}, before.ClaimStatus) || !containsString([]string{"PENDING", "RECOVERING"}, before.JobStatus) {
		return before, ErrOpsConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE platform_meta.registration_grant_jobs SET next_attempt_at=clock_timestamp(),updated_at=clock_timestamp() WHERE claim_id=$1`, id); err != nil {
		return before, err
	}
	after, err := loadRegistrationRetry(ctx, tx, id, false)
	if err != nil {
		return after, err
	}
	if err = relayOpsAudit(ctx, tx, actor, "REWARD_CLAIM_RETRY", "REWARD_CLAIM", id, before, after); err != nil {
		return after, err
	}
	return after, nil
}
