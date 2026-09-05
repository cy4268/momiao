package platform

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Asset is a local wallet position. Native Active quota is deliberately excluded.
type Asset string

const (
	ReserveAPICredit Asset = "RESERVE_API_CREDIT"
	AvailableChips   Asset = "AVAILABLE_CHIPS"
)

var (
	ErrInvalidDatabaseURL  = errors.New("explicit nonempty database URL required")
	ErrInvalidMutation     = errors.New("invalid wallet mutation")
	ErrWalletNotFound      = errors.New("wallet not found")
	ErrIdempotencyConflict = errors.New("idempotency or business identity conflict")
	ErrInsufficientBalance = errors.New("insufficient wallet balance")
	ErrBalanceOverflow     = errors.New("wallet balance or sequence overflow")
	ErrInvalidPage         = errors.New("invalid ledger page")
)

type Store struct{ pool *pgxpool.Pool }

// OpenLazy validates connection configuration without requiring an available database.
// Queries acquire connections on demand, so a wallet outage does not prevent portal startup.
func OpenLazy(ctx context.Context, databaseURL string) (*Store, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, ErrInvalidDatabaseURL
	}
	p, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	return &Store{pool: p}, nil
}

// Open verifies connectivity, but never changes schemas or initializes accounts.
func Open(ctx context.Context, databaseURL string) (*Store, error) {
	s, err := OpenLazy(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err = s.pool.Ping(ctx); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() { s.pool.Close() }

type Wallet struct {
	UserID       int64 `json:"user_id,string"`
	Asset        Asset `json:"asset"`
	BalanceUnits int64 `json:"balance_units,string"`
	LedgerSeq    int64 `json:"ledger_seq,string"`
	Version      int64 `json:"version,string"`
}
type LedgerEntry struct {
	ID                 string    `json:"id"`
	TransactionID      string    `json:"transaction_id"`
	UserID             int64     `json:"user_id,string"`
	Asset              Asset     `json:"asset"`
	LedgerSeq          int64     `json:"ledger_seq,string"`
	WalletVersion      int64     `json:"wallet_version,string"`
	EntryType          string    `json:"entry_type"`
	BizType            string    `json:"biz_type"`
	BizID              string    `json:"biz_id"`
	DeltaUnits         int64     `json:"delta_units,string"`
	BalanceBeforeUnits int64     `json:"balance_before_units,string"`
	BalanceAfterUnits  int64     `json:"balance_after_units,string"`
	CreatedAt          time.Time `json:"created_at"`
}

// Mutation is an internal single-wallet business operation, not an HTTP body.
// Callers must supply a verified identity and stable, globally unique business
// identity. Reuse the SAME identity and key after an uncertain commit outcome.
type Mutation struct {
	UserID         int64
	Asset          Asset
	DeltaUnits     int64
	BizType        string
	BizID          string
	EntryType      string
	IdempotencyKey string
}

func validAsset(a Asset) bool { return a == ReserveAPICredit || a == AvailableChips }
func validLabel(v string) bool {
	return len(v) > 0 && len(v) <= 128 && utf8.ValidString(v) && strings.TrimSpace(v) == v && !strings.ContainsRune(v, 0)
}
func (m Mutation) hashes() ([32]byte, [32]byte, error) {
	if m.UserID <= 0 || !validAsset(m.Asset) || m.DeltaUnits == 0 || !validLabel(m.BizType) || !validLabel(m.BizID) || !validLabel(m.EntryType) || len(m.IdempotencyKey) < 16 || len(m.IdempotencyKey) > 128 {
		return [32]byte{}, [32]byte{}, ErrInvalidMutation
	}
	for i := 0; i < len(m.IdempotencyKey); i++ {
		if m.IdempotencyKey[i] < 33 || m.IdempotencyKey[i] > 126 {
			return [32]byte{}, [32]byte{}, ErrInvalidMutation
		}
	}
	// Fixed ordered typed fields, with a version domain separator. Never hash a
	// caller's raw JSON; the transport key is deliberately not semantic.
	data, _ := json.Marshal(struct {
		Version                   string
		UserID                    int64
		Asset                     Asset
		Delta                     int64
		BizType, BizID, EntryType string
	}{"wallet.apply.v1", m.UserID, m.Asset, m.DeltaUnits, m.BizType, m.BizID, m.EntryType})
	return sha256.Sum256([]byte(m.IdempotencyKey)), sha256.Sum256(data), nil
}
func uuidV7() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	ms := uint64(time.Now().UnixMilli())
	for i := 5; i >= 0; i-- {
		b[i] = byte(ms)
		ms >>= 8
	}
	b[6] = (b[6] & 15) | 0x70
	b[8] = (b[8] & 63) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// rollback uses a fresh bounded context because the operation context may have
// expired while blocked on a row lock. pgx otherwise leaves cleanup to callers.
func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func (s *Store) EnsureAccount(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return ErrInvalidMutation
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, "INSERT INTO identity.account_refs(newapi_user_id) VALUES($1) ON CONFLICT DO NOTHING", userID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO economy.wallet_balances(newapi_user_id,asset_type) VALUES($1,'RESERVE_API_CREDIT'),($1,'AVAILABLE_CHIPS') ON CONFLICT DO NOTHING`, userID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) ReadWallet(ctx context.Context, userID int64, asset Asset) (Wallet, error) {
	var w Wallet
	err := s.pool.QueryRow(ctx, "SELECT newapi_user_id,asset_type,balance_units,ledger_seq,version FROM economy.wallet_balances WHERE newapi_user_id=$1 AND asset_type=$2", userID, asset).Scan(&w.UserID, &w.Asset, &w.BalanceUnits, &w.LedgerSeq, &w.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrWalletNotFound
	}
	return w, err
}

const ledgerColumns = "ledger_entry_id::text,transaction_id::text,newapi_user_id,asset_type,ledger_seq,wallet_version,entry_type,biz_type,biz_id,delta_units,balance_before_units,balance_after_units,created_at"

func scanEntry(row pgx.Row) (LedgerEntry, error) {
	var e LedgerEntry
	err := row.Scan(&e.ID, &e.TransactionID, &e.UserID, &e.Asset, &e.LedgerSeq, &e.WalletVersion, &e.EntryType, &e.BizType, &e.BizID, &e.DeltaUnits, &e.BalanceBeforeUnits, &e.BalanceAfterUnits, &e.CreatedAt)
	return e, err
}

// Ledger returns one user's one-asset history in ascending sequence order.
// Use the last returned LedgerSeq as afterSeq; page size is restricted to 1..100.
func (s *Store) Ledger(ctx context.Context, userID int64, asset Asset, afterSeq int64, limit int) ([]LedgerEntry, error) {
	if userID <= 0 || !validAsset(asset) || afterSeq < 0 || limit < 1 || limit > 100 {
		return nil, ErrInvalidPage
	}
	if _, err := s.ReadWallet(ctx, userID, asset); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT "+ledgerColumns+" FROM economy.wallet_ledger WHERE newapi_user_id=$1 AND asset_type=$2 AND ledger_seq>$3 ORDER BY ledger_seq LIMIT $4", userID, asset, afterSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []LedgerEntry{}
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
func lockIdentity(ctx context.Context, tx pgx.Tx, fields ...any) error {
	data, _ := json.Marshal(fields)
	h := sha256.Sum256(data)
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(binary.BigEndian.Uint64(h[:8])))
	return err
}
func insertIdempotency(ctx context.Context, tx pgx.Tx, m Mutation, key, semantic [32]byte, entryID string) error {
	id, err := uuidV7()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO platform_meta.mutation_idempotency_records(idempotency_record_id,newapi_user_id,scope,key_hash,request_hash,resource_type,resource_id) VALUES($1,$2,'wallet.apply.v1',$3,$4,'wallet_ledger',$5)`, id, m.UserID, key[:], semantic[:], entryID)
	return err
}

// Apply atomically commits identity, wallet projection and immutable ledger.
// No automatic retries: an ambiguous COMMIT must be reconciled by replaying the
// original key/business identity. Missing accounts are not silently initialized.
func (s *Store) Apply(ctx context.Context, m Mutation) (LedgerEntry, error) {
	if _, _, err := m.hashes(); err != nil {
		return LedgerEntry{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return LedgerEntry{}, err
	}
	defer rollback(tx)
	e, err := applyInTx(ctx, tx, m)
	if err != nil {
		return LedgerEntry{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return LedgerEntry{}, err
	}
	return e, nil
}

func applyInTx(ctx context.Context, tx pgx.Tx, m Mutation) (LedgerEntry, error) {
	key, semantic, err := m.hashes()
	if err != nil {
		return LedgerEntry{}, err
	}
	// Shared lock order: business identity, user-scoped idempotency, then wallet.
	// Hash collisions only serialize unrelated work; SQL uniqueness stays authoritative.
	if err = lockIdentity(ctx, tx, "business", m.BizType, m.BizID); err != nil {
		return LedgerEntry{}, err
	}
	if err = lockIdentity(ctx, tx, "idempotency", m.UserID, fmt.Sprintf("%x", key)); err != nil {
		return LedgerEntry{}, err
	}
	var stored []byte
	var entryID string
	err = tx.QueryRow(ctx, `SELECT request_hash,resource_id::text FROM platform_meta.mutation_idempotency_records WHERE newapi_user_id=$1 AND scope='wallet.apply.v1' AND key_hash=$2`, m.UserID, key[:]).Scan(&stored, &entryID)
	if err == nil {
		if string(stored) != string(semantic[:]) {
			return LedgerEntry{}, ErrIdempotencyConflict
		}
		e, err := scanEntry(tx.QueryRow(ctx, "SELECT "+ledgerColumns+" FROM economy.wallet_ledger WHERE ledger_entry_id=$1 AND newapi_user_id=$2", entryID, m.UserID))
		if err != nil {
			return LedgerEntry{}, err
		}
		return e, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return LedgerEntry{}, err
	}
	var transactionID string
	var owner int64
	err = tx.QueryRow(ctx, "SELECT transaction_id::text,newapi_user_id,request_hash FROM economy.asset_transactions WHERE biz_type=$1 AND biz_id=$2", m.BizType, m.BizID).Scan(&transactionID, &owner, &stored)
	if err == nil {
		if owner != m.UserID || string(stored) != string(semantic[:]) {
			return LedgerEntry{}, ErrIdempotencyConflict
		}
		e, err := scanEntry(tx.QueryRow(ctx, "SELECT "+ledgerColumns+" FROM economy.wallet_ledger WHERE transaction_id=$1 AND newapi_user_id=$2 AND leg_no=1", transactionID, m.UserID))
		if err != nil {
			return LedgerEntry{}, err
		}
		if err = insertIdempotency(ctx, tx, m, key, semantic, e.ID); err != nil {
			return LedgerEntry{}, err
		}
		return e, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return LedgerEntry{}, err
	}
	var before, seq, version int64
	err = tx.QueryRow(ctx, "SELECT balance_units,ledger_seq,version FROM economy.wallet_balances WHERE newapi_user_id=$1 AND asset_type=$2 FOR UPDATE", m.UserID, m.Asset).Scan(&before, &seq, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return LedgerEntry{}, ErrWalletNotFound
	}
	if err != nil {
		return LedgerEntry{}, err
	}
	if m.DeltaUnits < 0 && m.DeltaUnits < -before {
		return LedgerEntry{}, ErrInsufficientBalance
	}
	if (m.DeltaUnits > 0 && before > math.MaxInt64-m.DeltaUnits) || seq == math.MaxInt64 || version == math.MaxInt64 {
		return LedgerEntry{}, ErrBalanceOverflow
	}
	after := before + m.DeltaUnits
	seq++
	version++
	transactionID, err = uuidV7()
	if err != nil {
		return LedgerEntry{}, err
	}
	entryID, err = uuidV7()
	if err != nil {
		return LedgerEntry{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO economy.asset_transactions(transaction_id,biz_type,biz_id,newapi_user_id,operation_type,status,request_hash) VALUES($1,$2,$3,$4,$5,'CONFIRMED',$6)`, transactionID, m.BizType, m.BizID, m.UserID, m.EntryType, semantic[:])
	if err != nil {
		return LedgerEntry{}, err
	}
	_, err = tx.Exec(ctx, "UPDATE economy.wallet_balances SET balance_units=$3,ledger_seq=$4,version=$5,updated_at=now() WHERE newapi_user_id=$1 AND asset_type=$2", m.UserID, m.Asset, after, seq, version)
	if err != nil {
		return LedgerEntry{}, err
	}
	e, err := scanEntry(tx.QueryRow(ctx, `INSERT INTO economy.wallet_ledger(ledger_entry_id,transaction_id,leg_no,newapi_user_id,asset_type,ledger_seq,wallet_version,entry_type,biz_type,biz_id,delta_units,balance_before_units,balance_after_units) VALUES($1,$2,1,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING `+ledgerColumns, entryID, transactionID, m.UserID, m.Asset, seq, version, m.EntryType, m.BizType, m.BizID, m.DeltaUnits, before, after))
	if err != nil {
		return LedgerEntry{}, err
	}
	if err = insertIdempotency(ctx, tx, m, key, semantic, e.ID); err != nil {
		return LedgerEntry{}, err
	}
	return e, nil
}
