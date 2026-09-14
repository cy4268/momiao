package platform

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	HourlyRewardAmount   int64 = 100 * UnitsPerCredit
	ReliefRewardAmount   int64 = 300 * UnitsPerCredit
	ReliefAssetThreshold int64 = 10 * UnitsPerCredit
	HourlyDailyLimit           = 24
	ReliefCooldown             = 4 * time.Hour
)

var (
	ErrHourlyDailyLimit = errors.New("hourly reward daily limit reached")
	ErrReliefIneligible = errors.New("relief reward not eligible")
	ErrReliefCooldown   = errors.New("relief reward cooldown active")
)

type HourlyReward struct {
	UserID          int64      `json:"user_id,string"`
	RewardHour      time.Time  `json:"reward_hour"`
	BusinessDate    string     `json:"business_date"`
	Timezone        string     `json:"timezone"`
	NextResetAt     time.Time  `json:"next_reset_at"`
	Amount          string     `json:"amount"`
	AmountUnits     int64      `json:"amount_units,string"`
	Asset           Asset      `json:"asset"`
	PolicyVersion   string     `json:"policy_version"`
	Claimed         bool       `json:"claimed"`
	TransactionID   *string    `json:"transaction_id"`
	ClaimsToday     int        `json:"claims_today"`
	DailyLimit      int        `json:"daily_limit"`
	Accumulation    bool       `json:"accumulation"`
}

type ReliefReward struct {
	UserID                  int64      `json:"user_id,string"`
	Amount                  string     `json:"amount"`
	AmountUnits             int64      `json:"amount_units,string"`
	Asset                   Asset      `json:"asset"`
	PolicyVersion           string     `json:"policy_version"`
	Threshold               string     `json:"threshold"`
	ThresholdUnits          int64      `json:"threshold_units,string"`
	CurrentTotalAssets      string     `json:"current_total_assets"`
	CurrentTotalAssetsUnits int64      `json:"current_total_assets_units,string"`
	AssetsObservedAt        time.Time  `json:"assets_observed_at"`
	Eligible                bool       `json:"eligible"`
	CooldownSeconds         int64      `json:"cooldown_seconds,string"`
	NextEligibleAt          *time.Time `json:"next_eligible_at"`
	LastTransactionID       *string    `json:"last_transaction_id"`
	Accumulation            bool       `json:"accumulation"`
}

// EconomyService adds Relief eligibility to Store without making ordinary
// wallet and Hourly operations depend on Native quota availability.
type EconomyService struct {
	*Store
	assets *UnifiedAssetReader
}

func NewEconomyService(store *Store, assets *UnifiedAssetReader) (*EconomyService, error) {
	if store == nil || assets == nil || assets.store != store {
		return nil, ErrInvalidMutation
	}
	return &EconomyService{Store: store, assets: assets}, nil
}

func shanghaiHour(at time.Time) (time.Time, time.Time) {
	local := at.In(shanghai)
	year, month, day := local.Date()
	start := time.Date(year, month, day, local.Hour(), 0, 0, 0, shanghai)
	return start.UTC(), start.Add(time.Hour).UTC()
}

func hourlyReward(now time.Time, user int64, id *string, claims int) HourlyReward {
	hour, next := shanghaiHour(now)
	day, _ := shanghaiDay(now)
	return HourlyReward{
		UserID: user, RewardHour: hour, BusinessDate: day, Timezone: "Asia/Shanghai", NextResetAt: next,
		Amount: "100", AmountUnits: HourlyRewardAmount, Asset: ReserveAPICredit, PolicyVersion: "1",
		Claimed: id != nil, TransactionID: id, ClaimsToday: claims, DailyLimit: HourlyDailyLimit, Accumulation: false,
	}
}

func (s *Store) ReadHourlyReward(ctx context.Context, user int64) (HourlyReward, error) {
	if !validateQuotaUser(user) {
		return HourlyReward{}, ErrInvalidMutation
	}
	var now time.Time
	if err := s.pool.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return HourlyReward{}, err
	}
	hour, _ := shanghaiHour(now)
	day, _ := shanghaiDay(now)
	var id *string
	var claims int
	err := s.pool.QueryRow(ctx, `SELECT
 (SELECT transaction_id::text FROM rewards.hourly_claims WHERE newapi_user_id=$1 AND reward_hour=$2),
 (SELECT count(*)::integer FROM rewards.hourly_claims WHERE newapi_user_id=$1 AND business_date=$3)`, user, hour, day).Scan(&id, &claims)
	if err != nil {
		return HourlyReward{}, err
	}
	return hourlyReward(now, user, id, claims), nil
}

func hourlyMutation(user int64, hour time.Time, key string) Mutation {
	return Mutation{
		UserID: user, Asset: ReserveAPICredit, DeltaUnits: HourlyRewardAmount,
		BizType: "HOURLY_REWARD_V1", BizID: fmt.Sprintf("hourly:%d:%s", user, hour.Format(time.RFC3339)),
		EntryType: "HOURLY_REWARD", IdempotencyKey: key,
	}
}

func hourlyClaimInTx(ctx context.Context, tx pgx.Tx, user int64, hour time.Time, key string) (*Transaction, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT transaction_id::text FROM rewards.hourly_claims
 WHERE newapi_user_id=$1 AND reward_hour=$2`, user, hour).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	entry, err := applyInTx(ctx, tx, hourlyMutation(user, hour, key))
	if err != nil {
		return nil, err
	}
	if entry.TransactionID != id {
		return nil, ErrIdempotencyConflict
	}
	result, err := transactionInTx(ctx, tx, user, id)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Store) ClaimHourlyReward(ctx context.Context, user int64, key string) (Transaction, error) {
	if !validateQuotaUser(user) || !ValidOperationKey(key) {
		return Transaction{}, ErrInvalidMutation
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Transaction{}, err
	}
	defer rollback(tx)
	if err = lockIdentity(ctx, tx, "hourly-request", user, key); err != nil {
		return Transaction{}, err
	}
	prior, err := findOperation(ctx, tx, user, "HOURLY", key)
	if err != nil {
		return Transaction{}, err
	}
	if prior != nil {
		return *prior, tx.Commit(ctx)
	}
	if err = lockIdentity(ctx, tx, "hourly-reward-user", user); err != nil {
		return Transaction{}, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return Transaction{}, err
	}
	hour, _ := shanghaiHour(now)
	if existing, lookupErr := hourlyClaimInTx(ctx, tx, user, hour, key); lookupErr != nil {
		return Transaction{}, lookupErr
	} else if existing != nil {
		return *existing, tx.Commit(ctx)
	}
	if err = RequireNoMaintenance(ctx, tx, "CHALDEA_USER_WRITES", "REWARDS"); err != nil {
		return Transaction{}, err
	}
	// The maintenance guard may have waited across an hour boundary. Re-read
	// database time after it and bind the grant only to the then-current hour.
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return Transaction{}, err
	}
	hour, _ = shanghaiHour(now)
	day, _ := shanghaiDay(now)
	if existing, lookupErr := hourlyClaimInTx(ctx, tx, user, hour, key); lookupErr != nil {
		return Transaction{}, lookupErr
	} else if existing != nil {
		return *existing, tx.Commit(ctx)
	}
	var claims int
	if err = tx.QueryRow(ctx, `SELECT count(*)::integer FROM rewards.hourly_claims
 WHERE newapi_user_id=$1 AND business_date=$2`, user, day).Scan(&claims); err != nil {
		return Transaction{}, err
	}
	if claims >= HourlyDailyLimit {
		return Transaction{}, ErrHourlyDailyLimit
	}
	entry, err := applyInTx(ctx, tx, hourlyMutation(user, hour, key))
	if err != nil {
		return Transaction{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO rewards.hourly_claims(newapi_user_id,reward_hour,business_date,policy_version,amount_units,asset_type,transaction_id)
	 VALUES($1,$2,$3,1,$4,'RESERVE_API_CREDIT',$5)`, user, hour, day, HourlyRewardAmount, entry.TransactionID); err != nil {
		return Transaction{}, err
	}
	result, err := transactionInTx(ctx, tx, user, entry.TransactionID)
	if err != nil {
		return Transaction{}, err
	}
	return result, tx.Commit(ctx)
}

func (service *EconomyService) ReadReliefReward(ctx context.Context, user int64) (ReliefReward, error) {
	if service == nil || service.Store == nil || service.assets == nil || !validateQuotaUser(user) {
		return ReliefReward{}, ErrInvalidMutation
	}
	assets, err := service.assets.ReadUnifiedAssets(ctx, user)
	if err != nil {
		return ReliefReward{}, err
	}
	var now time.Time
	var id *string
	var next *time.Time
	err = service.pool.QueryRow(ctx, `SELECT n.at,c.transaction_id::text,c.cooldown_until
	 FROM (SELECT clock_timestamp() AS at) n
	 LEFT JOIN LATERAL (SELECT transaction_id, cooldown_until FROM rewards.relief_claims
	  WHERE newapi_user_id=$1 ORDER BY claimed_at DESC,relief_claim_id DESC LIMIT 1) c ON true`, user).Scan(&now, &id, &next)
	if err != nil {
		return ReliefReward{}, err
	}
	if next != nil {
		value := next.UTC()
		next = &value
	}
	return ReliefReward{
		UserID: user, Amount: "300", AmountUnits: ReliefRewardAmount, Asset: ReserveAPICredit, PolicyVersion: "1",
		Threshold: "10", ThresholdUnits: ReliefAssetThreshold, CurrentTotalAssets: assets.TotalAmount,
		CurrentTotalAssetsUnits: assets.TotalUnits, AssetsObservedAt: assets.ObservedAt,
		Eligible: assets.TotalUnits < ReliefAssetThreshold && (next == nil || !now.Before(*next)),
		CooldownSeconds: int64(ReliefCooldown / time.Second), NextEligibleAt: next, LastTransactionID: id, Accumulation: false,
	}, nil
}

func reliefCooldownInTx(ctx context.Context, tx pgx.Tx, user int64, now time.Time) error {
	var until time.Time
	err := tx.QueryRow(ctx, `SELECT cooldown_until FROM rewards.relief_claims
	 WHERE newapi_user_id=$1 ORDER BY claimed_at DESC,relief_claim_id DESC LIMIT 1`, user).Scan(&until)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if now.Before(until) {
		return ErrReliefCooldown
	}
	return nil
}

func (service *EconomyService) ClaimReliefReward(ctx context.Context, user int64, key string) (Transaction, error) {
	if service == nil || service.Store == nil || service.assets == nil || !validateQuotaUser(user) || !ValidOperationKey(key) {
		return Transaction{}, ErrInvalidMutation
	}
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return Transaction{}, err
	}
	defer rollback(tx)
	if err = lockIdentity(ctx, tx, "relief-request", user, key); err != nil {
		return Transaction{}, err
	}
	prior, err := findOperation(ctx, tx, user, "RELIEF", key)
	if err != nil {
		return Transaction{}, err
	}
	if prior != nil {
		return *prior, tx.Commit(ctx)
	}
	if err = lockIdentity(ctx, tx, "relief-reward-user", user); err != nil {
		return Transaction{}, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return Transaction{}, err
	}
	if err = reliefCooldownInTx(ctx, tx, user, now); err != nil {
		return Transaction{}, err
	}
	if err = RequireNoMaintenance(ctx, tx, "CHALDEA_USER_WRITES", "REWARDS"); err != nil {
		return Transaction{}, err
	}
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return Transaction{}, err
	}
	if err = reliefCooldownInTx(ctx, tx, user, now); err != nil {
		return Transaction{}, err
	}
	claimID, err := uuidV7()
	if err != nil {
		return Transaction{}, err
	}
	biz := "relief:" + claimID
	// readUnifiedAssetsInTx locks both wallets. Take applyInTx's global lock
	// prefix first so a reused idempotency key cannot invert the lock order.
	if err = lockIdentity(ctx, tx, "business", "RELIEF_REWARD_V1", biz); err != nil {
		return Transaction{}, err
	}
	keyHash := sha256.Sum256([]byte(key))
	if err = lockIdentity(ctx, tx, "idempotency", user, fmt.Sprintf("%x", keyHash)); err != nil {
		return Transaction{}, err
	}
	assets, err := service.assets.readUnifiedAssetsInTx(ctx, tx, user)
	if err != nil {
		return Transaction{}, err
	}
	if assets.TotalUnits >= ReliefAssetThreshold {
		return Transaction{}, ErrReliefIneligible
	}
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return Transaction{}, err
	}
	entry, err := applyInTx(ctx, tx, Mutation{UserID: user, Asset: ReserveAPICredit, DeltaUnits: ReliefRewardAmount,
		BizType: "RELIEF_REWARD_V1", BizID: biz, EntryType: "RELIEF_REWARD", IdempotencyKey: key})
	if err != nil {
		return Transaction{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO rewards.relief_claims(relief_claim_id,newapi_user_id,policy_version,total_assets_units,
	 active_quota_units,reserve_units,available_chips_units,poker_stack_units,poker_pot_units,quota_transfer_in_flight_units,
	 assets_observed_at,native_observed_at,amount_units,asset_type,transaction_id,claimed_at,cooldown_until)
	 VALUES($1,$2,1,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'RESERVE_API_CREDIT',$13,$14::timestamptz,$14::timestamptz+interval '4 hours')`,
		claimID, user, assets.TotalUnits, assets.ActiveQuotaUnits, assets.ReserveUnits, assets.AvailableChipsUnits,
		assets.PokerStackUnits, assets.PokerPotUnits, assets.QuotaTransferInFlightUnits, assets.ObservedAt,
		assets.NativeObservedAt, ReliefRewardAmount, entry.TransactionID, now)
	if err != nil {
		return Transaction{}, err
	}
	result, err := transactionInTx(ctx, tx, user, entry.TransactionID)
	if err != nil {
		return Transaction{}, err
	}
	return result, tx.Commit(ctx)
}
