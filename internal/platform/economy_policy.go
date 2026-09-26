package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

const EconomicPolicyVersion = "economy-cap-v1"
const SinglePlayerMaxUnits int64 = 500000000000
const AssetCapUnits int64 = 50000000000000000

var ErrAssetCap = errors.New("asset cap would be exceeded")
var ErrEconomicPolicy = errors.New("economic policy unavailable or incompatible")

type EconomicPolicy struct {
	Version              string `json:"version"`
	Hash                 string `json:"hash"`
	SinglePlayerMaxUnits int64  `json:"single_player_max_units,string"`
	AssetCapUnits        int64  `json:"asset_cap_units,string"`
	CapMode              string `json:"cap_mode"`
}

// CapSettlement is private accounting evidence, never a multiplayer response.
type CapSettlement struct {
	PolicyVersion, PolicyHash                                           string
	TotalBeforeUnits, StakeUnits, GrossPayoutUnits                      int64
	CreditedPayoutUnits, WithheldUnits, ActualNetUnits, TotalAfterUnits int64
	QuantumUnits                                                        int64
	AssetsObservedAt                                                    time.Time
}

type PayoutCapView struct {
	PolicyVersion       string `json:"policy_version"`
	PolicyHash          string `json:"policy_hash"`
	GrossPayoutUnits    int64  `json:"gross_payout_units,string"`
	CreditedPayoutUnits int64  `json:"credited_payout_units,string"`
	WithheldUnits       int64  `json:"withheld_units,string"`
	ActualNetUnits      int64  `json:"actual_net_units,string"`
}

func (c CapSettlement) PublicView() PayoutCapView {
	return PayoutCapView{c.PolicyVersion, c.PolicyHash, c.GrossPayoutUnits, c.CreditedPayoutUnits, c.WithheldUnits, c.ActualNetUnits}
}

// An empty binding means legacy, not the current policy. Callers starting NEW
// work must use ActiveEconomicPolicyInTx, and persist that version before RNG.
func ResolveEconomicPolicyInTx(ctx context.Context, tx pgx.Tx, version string) (EconomicPolicy, error) {
	if version == "" {
		return EconomicPolicy{}, nil
	}
	var canonical []byte
	var hash string
	if err := tx.QueryRow(ctx, `SELECT canonical_json,policy_hash FROM economy.policy_versions WHERE version=$1`, version).Scan(&canonical, &hash); err != nil {
		return EconomicPolicy{}, err
	}
	sum := sha256.Sum256(canonical)
	var p EconomicPolicy
	if json.Unmarshal(canonical, &p) != nil || hex.EncodeToString(sum[:]) != hash || p.Version != version || p.Version != EconomicPolicyVersion || p.AssetCapUnits != AssetCapUnits || p.SinglePlayerMaxUnits != SinglePlayerMaxUnits || p.CapMode != "CLIP_PROFIT" {
		return EconomicPolicy{}, ErrEconomicPolicy
	}
	p.Hash = hash
	return p, nil
}

func ActiveEconomicPolicyInTx(ctx context.Context, tx pgx.Tx) (EconomicPolicy, error) {
	var version string
	if err := tx.QueryRow(ctx, `SELECT coalesce(active_version,'') FROM economy.policy_runtime WHERE singleton`).Scan(&version); err != nil {
		return EconomicPolicy{}, err
	}
	return ResolveEconomicPolicyInTx(ctx, tx, version)
}

// Only positive profit is clipped; returned principal is never clipped, even
// for a pre-existing account already above the cap. Integer-only arithmetic.
func CapPayout(p EconomicPolicy, total, stake, gross, quantum int64) (CapSettlement, error) {
	c := CapSettlement{PolicyVersion: p.Version, PolicyHash: p.Hash, TotalBeforeUnits: total, StakeUnits: stake, GrossPayoutUnits: gross, QuantumUnits: quantum}
	if total < 0 || stake < 0 || stake > total || gross < 0 || quantum <= 0 || stake%quantum != 0 || gross%quantum != 0 || p.AssetCapUnits <= 0 || p.Version == "" {
		return c, ErrInvalidMutation
	}
	principal := min(stake, gross)
	base := total - stake + principal
	profit := gross - principal
	room := max(p.AssetCapUnits-base, 0)
	profit = min(profit, room)
	profit -= profit % quantum
	if base > math.MaxInt64-profit || principal > math.MaxInt64-profit {
		return c, ErrBalanceOverflow
	}
	c.CreditedPayoutUnits = principal + profit
	c.WithheldUnits = gross - c.CreditedPayoutUnits
	c.ActualNetUnits = c.CreditedPayoutUnits - stake
	c.TotalAfterUnits = base + profit
	return c, nil
}

func LockEconomyUsersInTx(ctx context.Context, tx pgx.Tx, users ...int64) error {
	users = slices.Clone(users)
	slices.Sort(users)
	for _, user := range slices.Compact(users) {
		if user <= 0 {
			return ErrInvalidMutation
		}
		if err := lockIdentity(ctx, tx, "quota-transfer-user", user); err != nil {
			return err
		}
	}
	return nil
}

// Called once during startup, before handlers and workers can access the Store.
func (s *Store) ConfigureEconomicObserver(observer NativeQuotaObserver) error {
	if s == nil || observer == nil || s.economicObserver != nil {
		return ErrInvalidMutation
	}
	s.economicObserver = observer
	return nil
}

func LoadCapSettlementInTx(ctx context.Context, tx pgx.Tx, kind, id string, user int64) (*CapSettlement, error) {
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT receipt FROM economy.cap_settlements WHERE source_kind=$1 AND source_id=$2 AND newapi_user_id=$3`, kind, id, user).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c CapSettlement
	if json.Unmarshal(raw, &c) != nil {
		return nil, ErrEconomicPolicy
	}
	return &c, nil
}

func RecordCapSettlementInTx(ctx context.Context, tx pgx.Tx, kind, id string, user int64, c CapSettlement) error {
	if !validLabel(kind) || !validLabel(id) || user <= 0 {
		return ErrInvalidMutation
	}
	p, err := ResolveEconomicPolicyInTx(ctx, tx, c.PolicyVersion)
	if err != nil {
		return err
	}
	check, err := CapPayout(p, c.TotalBeforeUnits, c.StakeUnits, c.GrossPayoutUnits, c.QuantumUnits)
	if err != nil || check.PublicView() != c.PublicView() || check.TotalAfterUnits != c.TotalAfterUnits || c.AssetsObservedAt.IsZero() {
		return ErrInvalidMutation
	}
	old, err := LoadCapSettlementInTx(ctx, tx, kind, id, user)
	if err != nil {
		return err
	}
	if old != nil {
		if *old != c {
			return ErrIdempotencyConflict
		}
		return nil
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO economy.cap_settlements(source_kind,source_id,newapi_user_id,policy_version,gross_payout_units,credited_payout_units,withheld_units,actual_net_units,receipt) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, kind, id, user, c.PolicyVersion, c.GrossPayoutUnits, c.CreditedPayoutUnits, c.WithheldUnits, c.ActualNetUnits, raw)
	return err
}

func PrepareCapSettlementInTx(ctx context.Context, tx pgx.Tx, observer NativeQuotaObserver, p EconomicPolicy, user, stake, gross, quantum int64) (CapSettlement, error) {
	assets, err := ReadUnifiedAssetsInTx(ctx, tx, observer, user)
	if err != nil {
		return CapSettlement{}, err
	}
	c, err := CapPayout(p, assets.TotalUnits, stake, gross, quantum)
	c.AssetsObservedAt = assets.ObservedAt
	return c, err
}

// Rewards opt in explicitly. ApplyInTx never silently changes a caller's delta.
// The immutable cap receipt is checked BEFORE capacity is observed again.
func (s *Store) applyRewardInTx(ctx context.Context, tx pgx.Tx, m Mutation) (LedgerEntry, error) {
	if err := LockEconomyUsersInTx(ctx, tx, m.UserID); err != nil {
		return LedgerEntry{}, err
	}
	key, semantic, err := m.hashes()
	if err != nil {
		return LedgerEntry{}, err
	}
	if err = lockIdentity(ctx, tx, "business", m.BizType, m.BizID); err != nil {
		return LedgerEntry{}, err
	}
	if err = lockIdentity(ctx, tx, "idempotency", m.UserID, hex.EncodeToString(key[:])); err != nil {
		return LedgerEntry{}, err
	}
	var priorID string
	var priorHash []byte
	err = tx.QueryRow(ctx, `SELECT transaction_id::text,request_hash FROM economy.reward_request_keys WHERE newapi_user_id=$1 AND key_hash=$2`, m.UserID, key[:]).Scan(&priorID, &priorHash)
	if err == nil {
		if string(priorHash) != string(semantic[:]) {
			return LedgerEntry{}, ErrIdempotencyConflict
		}
		return rewardEntryInTx(ctx, tx, m.UserID, priorID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return LedgerEntry{}, err
	}
	old, err := LoadCapSettlementInTx(ctx, tx, m.BizType, m.BizID, m.UserID)
	if err != nil {
		return LedgerEntry{}, err
	}
	var c CapSettlement
	if old != nil {
		if old.StakeUnits != 0 || old.GrossPayoutUnits != m.DeltaUnits {
			return LedgerEntry{}, ErrIdempotencyConflict
		}
		c = *old
	} else {
		// Legacy business receipts must also replay after policy activation.
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM economy.asset_transactions WHERE biz_type=$1 AND biz_id=$2)`, m.BizType, m.BizID).Scan(&exists); err != nil {
			return LedgerEntry{}, err
		}
		if exists {
			return applyInTx(ctx, tx, m)
		}
		p, e := ActiveEconomicPolicyInTx(ctx, tx)
		if e != nil {
			return LedgerEntry{}, e
		}
		if p.Version == "" {
			return applyInTx(ctx, tx, m)
		}
		c, err = PrepareCapSettlementInTx(ctx, tx, s.economicObserver, p, m.UserID, 0, m.DeltaUnits, 1)
		if err != nil {
			return LedgerEntry{}, err
		}
	}
	actual := m
	actual.DeltaUnits = c.CreditedPayoutUnits
	var entry LedgerEntry
	if actual.DeltaUnits > 0 {
		entry, err = applyInTx(ctx, tx, actual)
	} else {
		// Zero rewards have a real transaction but deliberately no zero ledger leg.
		var conflicting bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform_meta.mutation_idempotency_records WHERE newapi_user_id=$1 AND scope='wallet.apply.v1' AND key_hash=$2)`, m.UserID, key[:]).Scan(&conflicting); err != nil {
			return LedgerEntry{}, err
		}
		if conflicting {
			return LedgerEntry{}, ErrIdempotencyConflict
		}
		entry.TransactionID, err = uuidV7()
		if err != nil {
			return LedgerEntry{}, err
		}
		err = tx.QueryRow(ctx, `SELECT transaction_id::text FROM economy.asset_transactions WHERE biz_type=$1 AND biz_id=$2 AND newapi_user_id=$3`, m.BizType, m.BizID, m.UserID).Scan(&priorID)
		if err == nil {
			entry.TransactionID = priorID
		} else if errors.Is(err, pgx.ErrNoRows) {
			_, err = tx.Exec(ctx, `INSERT INTO economy.asset_transactions(transaction_id,biz_type,biz_id,newapi_user_id,operation_type,status,request_hash) VALUES($1,$2,$3,$4,$5,'CONFIRMED',$6)`, entry.TransactionID, m.BizType, m.BizID, m.UserID, m.EntryType, semantic[:])
		}
	}
	if err != nil {
		return LedgerEntry{}, err
	}
	if old == nil {
		if err = RecordCapSettlementInTx(ctx, tx, m.BizType, m.BizID, m.UserID, c); err != nil {
			return LedgerEntry{}, err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO economy.reward_request_keys(newapi_user_id,key_hash,request_hash,transaction_id) VALUES($1,$2,$3,$4)`, m.UserID, key[:], semantic[:], entry.TransactionID)
	return entry, err
}

func rewardEntryInTx(ctx context.Context, tx pgx.Tx, user int64, id string) (LedgerEntry, error) {
	entry, err := scanEntry(tx.QueryRow(ctx, `SELECT `+ledgerColumns+` FROM economy.wallet_ledger WHERE transaction_id=$1 AND newapi_user_id=$2 AND leg_no=1`, id, user))
	if errors.Is(err, pgx.ErrNoRows) {
		return LedgerEntry{TransactionID: id, UserID: user, Asset: ReserveAPICredit}, nil
	}
	return entry, err
}
