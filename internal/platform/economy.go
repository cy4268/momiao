package platform

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5"
)

const DailyAmount int64 = 250000000

var operationKey = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func ValidOperationKey(key string) bool { return operationKey.MatchString(key) }

var shanghai, _ = time.LoadLocation("Asia/Shanghai")

func shanghaiDay(at time.Time) (string, time.Time) {
	local := at.In(shanghai)
	y, m, d := local.Date()
	return local.Format("2006-01-02"), time.Date(y, m, d+1, 0, 0, 0, 0, shanghai).UTC()
}

type Transaction struct {
	ReserveDebitUnits *int64    `json:"reserve_debit_units,string,omitempty"`
	ActiveDebitUnits  *int64    `json:"active_debit_units,string,omitempty"`
	ChipsCreditUnits  *int64    `json:"chips_credit_units,string,omitempty"`
	Reason            string    `json:"reason,omitempty"`
	ID                string    `json:"id"`
	UserID            int64     `json:"user_id,string"`
	BizID             string    `json:"biz_id"`
	Kind              string    `json:"kind"`
	Status            string    `json:"status"`
	FromAsset         Asset     `json:"from_asset"`
	ToAsset           Asset     `json:"to_asset"`
	AmountUnits       int64     `json:"amount_units,string"`
	Amount            string    `json:"amount"`
	CreatedAt         time.Time `json:"created_at"`
	ConfirmedAt       time.Time `json:"confirmed_at,omitzero"`
}

const transactionSelect = `SELECT t.transaction_id::text,t.newapi_user_id,t.biz_id,t.operation_type,t.status,
 CASE WHEN l2.ledger_entry_id IS NULL THEN '' ELSE l.asset_type END,
 coalesce(l2.asset_type,l.asset_type),abs(l.delta_units),t.created_at,t.confirmed_at,NULL::bigint,NULL::bigint,NULL::bigint,''::text
 FROM economy.asset_transactions t
 JOIN economy.wallet_ledger l ON l.transaction_id=t.transaction_id AND l.leg_no=1
 LEFT JOIN economy.wallet_ledger l2 ON l2.transaction_id=t.transaction_id AND l2.leg_no=2 `

func scanTransaction(row pgx.Row) (Transaction, error) {
	var t Transaction
	var confirmed *time.Time
	err := row.Scan(&t.ID, &t.UserID, &t.BizID, &t.Kind, &t.Status, &t.FromAsset, &t.ToAsset, &t.AmountUnits, &t.CreatedAt, &confirmed, &t.ReserveDebitUnits, &t.ActiveDebitUnits, &t.ChipsCreditUnits, &t.Reason)
	t.Amount = FormatAmount(t.AmountUnits)
	t.CreatedAt = t.CreatedAt.UTC()
	if confirmed != nil {
		t.ConfirmedAt = confirmed.UTC()
	}
	return t, err
}
func transactionInTx(ctx context.Context, tx pgx.Tx, user int64, id string) (Transaction, error) {
	return scanTransaction(tx.QueryRow(ctx, transactionSelect+" WHERE t.newapi_user_id=$1 AND t.transaction_id=$2", user, id))
}
func (s *Store) Transactions(ctx context.Context, user int64, after string) ([]Transaction, error) {
	if user <= 0 || (after != "" && !ValidOperationKey(after)) {
		return nil, ErrInvalidPage
	}
	// Combine whole receipts in one snapshot; local source legs are not exchanges.
	query := `SELECT * FROM (` + transactionSelect + ` WHERE t.newapi_user_id=$1 AND t.operation_type IN ('DAILY_REWARD','HOURLY_REWARD','RELIEF_REWARD','LOCAL_EXCHANGE','INITIAL_GRANT_REGISTRATION')
 UNION ALL SELECT exchange_id::text,newapi_user_id,'api-chips:'||exchange_id::text,'API_CHIPS_EXCHANGE',status,
 'RESERVE_API_CREDIT','AVAILABLE_CHIPS',requested_units,created_at,confirmed_at,
 reserve_debit_units,active_debit_units,chips_credit_units,reason FROM economy.api_chips_exchanges WHERE newapi_user_id=$1
 ) AS receipt(id,user_id,biz_id,kind,status,from_asset,to_asset,amount_units,created_at,confirmed_at,reserve_debit_units,active_debit_units,chips_credit_units,reason)`
	args := []any{user}
	if after != "" {
		query += " WHERE id<$2"
		args = append(args, after)
	}
	rows, err := s.pool.Query(ctx, query+" ORDER BY id DESC LIMIT 21", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Transaction{}
	for rows.Next() {
		v, e := scanTransaction(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func operationScope(kind string) string {
	if kind == "DAILY" || kind == "HOURLY" || kind == "RELIEF" {
		return "wallet.apply.v1"
	}
	return "wallet.exchange.v1"
}

func operationKindMatches(kind, operationType string) bool {
	switch kind {
	case "DAILY":
		return operationType == "DAILY_REWARD"
	case "HOURLY":
		return operationType == "HOURLY_REWARD"
	case "RELIEF":
		return operationType == "RELIEF_REWARD"
	case "EXCHANGE":
		return operationType == "LOCAL_EXCHANGE"
	default:
		return false
	}
}

func validOperationKind(kind string) bool {
	return kind == "DAILY" || kind == "HOURLY" || kind == "RELIEF" || kind == "EXCHANGE"
}
func findOperation(ctx context.Context, tx pgx.Tx, user int64, kind, key string) (*Transaction, error) {
	if kind == "EXCHANGE" {
		prior, err := findAPIChipsExchange(ctx, tx, user, key)
		if err != nil || prior != nil {
			return prior, err
		}
	}
	hash := sha256.Sum256([]byte(key))
	var id string
	err := tx.QueryRow(ctx, `SELECT l.transaction_id::text FROM platform_meta.mutation_idempotency_records i JOIN economy.wallet_ledger l ON l.ledger_entry_id=i.resource_id WHERE i.newapi_user_id=$1 AND i.scope=$2 AND i.key_hash=$3`, user, operationScope(kind), hash[:]).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t, err := transactionInTx(ctx, tx, user, id)
	if err != nil {
		return nil, err
	}
	if !operationKindMatches(kind, t.Kind) {
		return nil, ErrIdempotencyConflict
	}
	return &t, nil
}
func (s *Store) FindOperation(ctx context.Context, user int64, kind, key string) (*Transaction, error) {
	if user <= 0 || !ValidOperationKey(key) || !validOperationKind(kind) {
		return nil, ErrInvalidMutation
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer rollback(tx)
	t, err := findOperation(ctx, tx, user, kind, key)
	if err != nil {
		return nil, err
	}
	return t, tx.Commit(ctx)
}

type Daily struct {
	UserID            int64     `json:"user_id,string"`
	BusinessDate      string    `json:"business_date"`
	Timezone          string    `json:"timezone"`
	NextResetAt       time.Time `json:"next_reset_at"`
	Amount            string    `json:"amount"`
	AmountUnits       int64     `json:"amount_units,string"`
	Asset             Asset     `json:"asset"`
	PolicyVersion     string    `json:"policy_version"`
	Claimed           bool      `json:"claimed"`
	TransactionID     *string   `json:"transaction_id"`
	MaintenanceActive bool      `json:"maintenance_active"`
}

func (s *Store) ReadDaily(ctx context.Context, user int64) (Daily, error) {
	if user <= 0 {
		return Daily{}, ErrInvalidMutation
	}
	// Use one statement snapshot and database clock, never a browser date.
	var now time.Time
	var id *string
	var maintenanceActive bool
	err := s.pool.QueryRow(ctx, `WITH d AS MATERIALIZED (SELECT clock_timestamp() AS at)
	 SELECT d.at,c.transaction_id::text,
	 ops.is_maintenance_scope_active('CHALDEA_USER_WRITES') OR ops.is_maintenance_scope_active('REWARDS')
	 FROM d LEFT JOIN rewards.daily_checkins c ON c.newapi_user_id=$1 AND c.checkin_date=(d.at AT TIME ZONE 'Asia/Shanghai')::date`, user).Scan(&now, &id, &maintenanceActive)
	day, next := shanghaiDay(now)
	return Daily{UserID: user, BusinessDate: day, Timezone: "Asia/Shanghai", NextResetAt: next,
		Amount: "500", AmountUnits: DailyAmount, Asset: ReserveAPICredit, PolicyVersion: "1",
		Claimed: id != nil, TransactionID: id, MaintenanceActive: maintenanceActive}, err
}
func (s *Store) ClaimDaily(ctx context.Context, user int64, key string) (Transaction, error) {
	if user <= 0 || !ValidOperationKey(key) {
		return Transaction{}, ErrInvalidMutation
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Transaction{}, err
	}
	defer rollback(tx)
	if err = lockIdentity(ctx, tx, "daily-request", user, key); err != nil {
		return Transaction{}, err
	}
	// A retry after midnight resolves its original receipt, not a new day's grant.
	prior, err := findOperation(ctx, tx, user, "DAILY", key)
	if err != nil {
		return Transaction{}, err
	}
	if prior != nil {
		return *prior, tx.Commit(ctx)
	}
	if err = RequireNoMaintenance(ctx, tx, "CHALDEA_USER_WRITES", "REWARDS"); err != nil {
		return Transaction{}, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return Transaction{}, err
	}
	day, _ := shanghaiDay(now)
	m := Mutation{UserID: user, Asset: ReserveAPICredit, DeltaUnits: DailyAmount, BizType: "DAILY_REWARD_V1", BizID: fmt.Sprintf("daily:%d:%s", user, day), EntryType: "DAILY_REWARD", IdempotencyKey: key}
	entry, err := applyInTx(ctx, tx, m)
	if err != nil {
		return Transaction{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO rewards.daily_checkins(newapi_user_id,checkin_date,policy_version,amount_units,asset_type,transaction_id) VALUES($1,$2,1,250000000,'RESERVE_API_CREDIT',$3) ON CONFLICT (newapi_user_id,checkin_date) DO NOTHING`, user, day, entry.TransactionID)
	if err != nil {
		return Transaction{}, err
	}
	result, err := transactionInTx(ctx, tx, user, entry.TransactionID)
	if err != nil {
		return Transaction{}, err
	}
	return result, tx.Commit(ctx)
}

func (s *Store) Exchange(ctx context.Context, user int64, key string, from Asset, amount int64) (Transaction, error) {
	return s.exchange(ctx, nil, user, key, from, amount)
}
func (service *EconomyService) Exchange(ctx context.Context, user int64, key string, from Asset, amount int64) (Transaction, error) {
	return service.Store.exchange(ctx, service.assets.native, user, key, from, amount)
}
func (s *Store) exchange(ctx context.Context, native NativeQuotaOperator, user int64, key string, from Asset, amount int64) (Transaction, error) {
	if user <= 0 || !ValidOperationKey(key) || !validAsset(from) || amount <= 0 {
		return Transaction{}, ErrInvalidMutation
	}
	to := ReserveAPICredit
	if from == ReserveAPICredit {
		to = AvailableChips
	}
	raw, _ := json.Marshal([]any{"wallet.exchange.v1", user, from, amount})
	semantic := sha256.Sum256(raw)
	hash := sha256.Sum256([]byte(key))
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Transaction{}, err
	}
	defer rollback(tx)
	if err = lockIdentity(ctx, tx, "exchange", user, key); err != nil {
		return Transaction{}, err
	}
	prior, err := findOperation(ctx, tx, user, "EXCHANGE", key)
	if err != nil {
		return Transaction{}, err
	}
	if prior != nil {
		if prior.FromAsset != from || prior.AmountUnits != amount {
			return Transaction{}, ErrIdempotencyConflict
		}
		return *prior, tx.Commit(ctx)
	}
	if err = RequireNoMaintenance(ctx, tx, "CHALDEA_USER_WRITES", "WALLET_EXCHANGE"); err != nil {
		return Transaction{}, err
	}
	if err = lockIdentity(ctx, tx, "quota-transfer-user", user); err != nil {
		return Transaction{}, err
	}
	// Both directions take the same row lock order; the two legs share one commit.
	rows, err := tx.Query(ctx, "SELECT newapi_user_id,asset_type,balance_units,ledger_seq,version FROM economy.wallet_balances WHERE newapi_user_id=$1 ORDER BY asset_type FOR UPDATE", user)
	if err != nil {
		return Transaction{}, err
	}
	wallets := map[Asset]Wallet{}
	for rows.Next() {
		var w Wallet
		if err = rows.Scan(&w.UserID, &w.Asset, &w.BalanceUnits, &w.LedgerSeq, &w.Version); err != nil {
			rows.Close()
			return Transaction{}, err
		}
		wallets[w.Asset] = w
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Transaction{}, err
	}
	if len(wallets) != 2 {
		return Transaction{}, ErrWalletNotFound
	}
	if wallets[from].BalanceUnits < amount {
		if from == ReserveAPICredit && native != nil {
			result, err := acceptAPIChipsExchange(ctx, tx, native, user, key, amount, wallets)
			if err != nil {
				return Transaction{}, err
			}
			return result, tx.Commit(ctx)
		}
		return Transaction{}, ErrInsufficientBalance
	}
	if wallets[to].BalanceUnits > math.MaxInt64-amount {
		return Transaction{}, ErrBalanceOverflow
	}
	for _, w := range wallets {
		if w.Version == math.MaxInt64 || w.LedgerSeq == math.MaxInt64 {
			return Transaction{}, ErrBalanceOverflow
		}
	}
	id, err := uuidV7()
	if err != nil {
		return Transaction{}, err
	}
	biz := fmt.Sprintf("exchange:%d:%s", user, key)
	_, err = tx.Exec(ctx, `INSERT INTO economy.asset_transactions(transaction_id,biz_type,biz_id,newapi_user_id,operation_type,status,request_hash) VALUES($1,'LOCAL_EXCHANGE',$2,$3,'LOCAL_EXCHANGE','CONFIRMED',$4)`, id, biz, user, semantic[:])
	if err != nil {
		return Transaction{}, err
	}
	first := ""
	for i, asset := range []Asset{from, to} {
		w := wallets[asset]
		delta := amount
		if i == 0 {
			delta = -amount
		}
		after := w.BalanceUnits + delta
		_, err = tx.Exec(ctx, "UPDATE economy.wallet_balances SET balance_units=$3,ledger_seq=$4,version=$5,updated_at=now() WHERE newapi_user_id=$1 AND asset_type=$2", user, asset, after, w.LedgerSeq+1, w.Version+1)
		if err != nil {
			return Transaction{}, err
		}
		entryID, e := uuidV7()
		if e != nil {
			return Transaction{}, e
		}
		if i == 0 {
			first = entryID
		}
		_, err = tx.Exec(ctx, `INSERT INTO economy.wallet_ledger(ledger_entry_id,transaction_id,leg_no,newapi_user_id,asset_type,ledger_seq,wallet_version,entry_type,biz_type,biz_id,delta_units,balance_before_units,balance_after_units) VALUES($1,$2,$3,$4,$5,$6,$7,'LOCAL_EXCHANGE','LOCAL_EXCHANGE',$8,$9,$10,$11)`, entryID, id, i+1, user, asset, w.LedgerSeq+1, w.Version+1, biz, delta, w.BalanceUnits, after)
		if err != nil {
			return Transaction{}, err
		}
	}
	recordID, err := uuidV7()
	if err != nil {
		return Transaction{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO platform_meta.mutation_idempotency_records(idempotency_record_id,newapi_user_id,scope,key_hash,request_hash,resource_type,resource_id) VALUES($1,$2,'wallet.exchange.v1',$3,$4,'wallet_ledger',$5)`, recordID, user, hash[:], semantic[:], first)
	if err != nil {
		return Transaction{}, err
	}
	result, err := transactionInTx(ctx, tx, user, id)
	if err != nil {
		return Transaction{}, err
	}
	return result, tx.Commit(ctx)
}
