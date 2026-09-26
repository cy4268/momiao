// Package poker supplies the persistent, single-writer cash-table service.
package poker

import (
	"context"
 "github.com/cy4268/momiao/internal/platform"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cy4268/momiao/internal/poker/engine"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rivo/uniseg"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

var ErrInvalid = errors.New("POKER_INVALID_COMMAND")
var ErrConflict = errors.New("POKER_REQUEST_CONFLICT")
var ErrBusy = errors.New("POKER_TABLE_BUSY")
var ErrFenced = errors.New("STALE_RUNTIME_EPOCH")
var ErrClosed = errors.New("POKER_SERVICE_CLOSED")
var ErrDenied = errors.New("POKER_COMMAND_DENIED")
var ErrNoControl = errors.New("POKER_CONTROL_NOT_OWNED")
var ErrStaleVersion = errors.New("POKER_STALE_VERSION")
var ErrNeedsReview = errors.New("POKER_HAND_NEEDS_REVIEW")
var ErrCorruptSnapshot = errors.New("POKER_CORRUPT_SNAPSHOT")
var ErrChatDisabled = errors.New("POKER_CHAT_DISABLED")
var ErrChatMuted = errors.New("POKER_CHAT_MUTED")

type Keyring struct {
	Current string
	Keys    map[string][]byte
}
type LeaseStore interface {
	Acquire(context.Context, string, int, string, time.Time) (bool, error)
	Valid(context.Context, string, int, string, time.Time) (bool, error)
	Release(context.Context, string, int, string) error
}
type Options struct {
 EconomyObserver platform.NativeQuotaObserver
	Pool                *pgxpool.Pool
	Keyring             Keyring
	Leases              LeaseStore
	Controls            ControlStore
	TableAccess         TableAccessStore
	Password            *PasswordRuntime
	PasswordLimits      PasswordLimits
	ValidateSession     func(context.Context, AuthSession) error
	SessionDeadline     func(context.Context, AuthSession) (time.Time, error)
	MailboxCapacity     int
	AfterCommit         func(tableID string, version uint64)
	AfterControlChanged func(tableID, connectionID string)
	AfterSpectatorRemoved func(tableID string, userID int64)
}
type Service struct {
	opts          Options
	mu            sync.Mutex
	actorMu       sync.Mutex
	actors        map[string]*tableActor
	closed        bool
	ownedLease    io.Closer
	connections   map[string]ControlRef
	claimEligible map[string]bool
	startMu       sync.Mutex
	started       bool
	startupReport StartupReport
}
type Receipt struct {
	TableID       string `json:"table_id"`
	SessionID     string `json:"session_id,omitempty"`
	HandID        string `json:"hand_id,omitempty"`
	ReservationID string `json:"reservation_id,omitempty"`
	FundingID     string `json:"funding_operation_id,omitempty"`
	Status        string `json:"status"`
	AmountUnits   string `json:"amount_units,omitempty"`
	Version       uint64 `json:"table_version,string"`
	Duplicate     bool   `json:"duplicate"`
	FailureCode   string `json:"failure_code,omitempty"`
	ControlEpoch  string `json:"control_epoch,omitempty"`
	ChatSequence  string `json:"chat_sequence,omitempty"`
}
type CreateTableCommand struct {
	UserID                 int64
	Key, Name, BlindPreset string
	MaxSeats               int
	AllowSpectators        bool
	AccessMode             string      `json:",omitempty"`
	ChatEnabled            bool        `json:",omitempty"`
	Password               string      `json:"-"`
	Auth                   AuthSession `json:"-"`
}
type ReserveSeatCommand struct {
	UserID       int64
	Key, TableID string
	Seat         int
	Auth         AuthSession `json:"-"`
}
type BuyInCommand struct {
	UserID                      int64
	Key, TableID, ReservationID string
	AmountUnits                 int64
	Auth                        AuthSession `json:"-"`
}
type SessionCommand struct {
	UserID                                    int64
	Key, TableID                              string
	TargetSessionID                           string
	Control                                   *ControlRef
	HandID                                    string
	ExpectedTableVersion, ExpectedHandVersion uint64
}
type StartHandCommand = SessionCommand
type TopUpCommand struct {
	UserID          int64
	Key, TableID    string
	TargetSessionID string
	AmountUnits     int64
}
type ActCommand struct {
	UserID                    int64
	Key, TableID, HandID      string
	ControlEpoch, HandVersion uint64
	Kind                      engine.ActionKind
	ToUnits                   int64
	Control                   *ControlRef
	TableVersion              uint64
}
type ConnectionCommand struct {
	UserID       int64
	Key, TableID string
	Connected    bool
	ControlEpoch uint64
}

// Accepting-player commands are distinct from legacy new-hand PAUSE/RESUME.
// UserID is the trusted server identity; HostControl checks the persisted owner.
type HostCommand struct {
	UserID             int64
	Key, TableID, Kind string
	TargetSessionID    string `json:",omitempty"`
	TargetUserID       int64  `json:",omitempty"`
}

func New(opts Options) (*Service, error) {
	if opts.Controls != nil && opts.ValidateSession == nil {
		return nil, ErrInvalid
	}
	passwordParts := opts.Password != nil || opts.TableAccess != nil || opts.SessionDeadline != nil || opts.PasswordLimits != (PasswordLimits{})
	if passwordParts && (!validPasswordRuntime(opts.Password) || opts.TableAccess == nil || opts.SessionDeadline == nil || !opts.PasswordLimits.valid()) {
		return nil, ErrInvalid
	}
	if opts.MailboxCapacity == 0 {
		opts.MailboxCapacity = 64
	}
	if opts.Pool == nil || opts.Leases == nil || opts.MailboxCapacity < 1 || opts.MailboxCapacity > 1024 || opts.Keyring.Current == "" || len(opts.Keyring.Current) > 64 || len(opts.Keyring.Keys[opts.Keyring.Current]) != 32 {
		return nil, ErrInvalid
	}
	keys := make(map[string][]byte, len(opts.Keyring.Keys))
	for id, key := range opts.Keyring.Keys {
		if len(key) != 32 {
			return nil, ErrInvalid
		}
		keys[id] = append([]byte{}, key...)
	}
	opts.Keyring.Keys = keys
	return &Service{opts: opts, actors: map[string]*tableActor{}, connections: map[string]ControlRef{}, claimEligible: map[string]bool{}}, nil
}
func (s *Service) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	actors := make([]*tableActor, 0, len(s.actors))
	for _, a := range s.actors {
		actors = append(actors, a)
	}
	s.mu.Unlock()
	for _, a := range actors {
		a.close()
	}
	if s.ownedLease != nil {
		_ = s.ownedLease.Close()
	}
}

type tableRow struct {
	Ruleset, AccessMode                      string
	ID                                       string
	Owner                                    int64
	Name                                     string
	MaxSeats                                 int
	Preset                                   string
	SmallBlind, BigBlind                     int64
	State                                    string
	Version, Epoch                           uint64
	HandNo                                   int64
	Button                                   int
	HandID                                   string
	Intermission, SettingsLocked, EmptySince *time.Time
	Accepting, AllowHands, AllowSpectators   bool
	ChatEnabled                              bool
	Spectators                               int
	ChatSequence                             uint64
	NeedsReview                              bool
	CloseRequested                           bool
	Now                                      time.Time
	onRollback, onCommit                     func(context.Context)
	beforeCommit                             func(context.Context, *Receipt) error
}

func (t tableRow) recoveryPreviousState() string {
	if t.CloseRequested {
		return "CLOSING"
	}
	return t.State
}

func loadTable(ctx context.Context, tx pgx.Tx, id string, lock bool) (tableRow, error) {
	var t tableRow
	sql := `SELECT t.table_id::text,t.owner_newapi_user_id,t.name,t.max_seats,t.blind_preset_version,b.small_blind_units,b.big_blind_units,t.lifecycle_state,t.table_version,t.runtime_epoch,t.hand_no,coalesce(t.button_seat,0),coalesce(t.current_hand_id::text,''),t.intermission_until,t.settings_locked_at,t.empty_since,t.accepting_players,t.allow_new_hands,t.allow_spectators,t.chat_enabled,t.spectator_count,t.chat_sequence,EXISTS(SELECT 1 FROM poker.recovery_state r WHERE r.table_id=t.table_id AND r.state='NEEDS_REVIEW'),t.ruleset_version,t.access_mode,
 (t.lifecycle_state='CLOSING' OR (t.lifecycle_state='RECOVERING' AND EXISTS(
   SELECT 1 FROM poker.recovery_state r WHERE r.table_id=t.table_id AND r.hand_id=t.current_hand_id
     AND r.state IN('GRACE','NEEDS_REVIEW') AND r.previous_table_state='CLOSING')))
 FROM poker.tables t JOIN poker.blind_preset_versions b ON b.version=t.blind_preset_version WHERE t.table_id=$1`
	if lock {
		sql += " FOR UPDATE OF t"
	}
	err := tx.QueryRow(ctx, sql, id).Scan(&t.ID, &t.Owner, &t.Name, &t.MaxSeats, &t.Preset, &t.SmallBlind, &t.BigBlind, &t.State, &t.Version, &t.Epoch, &t.HandNo, &t.Button, &t.HandID, &t.Intermission, &t.SettingsLocked, &t.EmptySince, &t.Accepting, &t.AllowHands, &t.AllowSpectators, &t.ChatEnabled, &t.Spectators, &t.ChatSequence, &t.NeedsReview, &t.Ruleset, &t.AccessMode, &t.CloseRequested)
	return t, err
}
func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func uuid() string {
	var b [16]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		panic("OS entropy unavailable")
	}
	ms := uint64(time.Now().UnixMilli())
	for i := 5; i >= 0; i-- {
		b[i] = byte(ms)
		ms >>= 8
	}
	b[6] = (b[6] & 15) | 0x70
	b[8] = (b[8] & 63) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func validIdentityKey(user int64, key string) bool {
	if user <= 0 || len(key) < 16 || len(key) > 128 {
		return false
	}
	for _, c := range key {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}
func units(n int64) bool     { return n >= 0 && n%engine.UnitsPerChip == 0 }
func decimal(n int64) string { return strconv.FormatInt(n, 10) }
func (s *Service) seal(aad string, plain []byte) ([]byte, error) {
	id := s.opts.Keyring.Current
	block, err := aes.NewCipher(s.opts.Keyring.Keys[id])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	out := append([]byte{1, byte(len(id))}, []byte(id)...)
	out = append(out, nonce...)
	return gcm.Seal(out, nonce, plain, []byte("poker:v1:"+aad)), nil
}
func (s *Service) open(aad string, sealed []byte) ([]byte, error) {
	return openCipher(s.opts.Keyring, aad, sealed)
}

func openCipher(keys Keyring, aad string, sealed []byte) ([]byte, error) {
	if len(sealed) < 2 || sealed[0] != 1 || int(sealed[1])+2 > len(sealed) {
		return nil, ErrInvalid
	}
	n := int(sealed[1])
	key := keys.Keys[string(sealed[2:2+n])]
	if len(key) != 32 {
		return nil, ErrDenied
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	body := sealed[2+n:]
	if len(body) < gcm.NonceSize()+gcm.Overhead() {
		return nil, ErrInvalid
	}
	return gcm.Open(nil, body[:gcm.NonceSize()], body[gcm.NonceSize():], []byte("poker:v1:"+aad))
}
func hashes(key string, command any) ([32]byte, [32]byte) {
	b, _ := json.Marshal(command)
	return sha256.Sum256([]byte(key)), sha256.Sum256(b)
}

// Receipt identity is immutable user intent, not the connection that currently
// authorizes its delivery. SessionID comes from the locked Poker seat; native
// auth, runtime/control epochs and optimistic versions remain command guards.
// This is an unpublished protocol correction; old local fixture receipts are
// not reused or rewritten, and no production data migration is implied.
type sessionIntent struct {
	UserID             int64
	TableID, SessionID string
}
type actionIntent struct {
	sessionIntent
	HandID  string
	Kind    engine.ActionKind
	ToUnits int64
}
type nextSeedIntent struct {
	sessionIntent
	Contribution string
}
type topUpIntent struct {
	sessionIntent
	AmountUnits int64
}

func canonicalSessionID(id string) bool {
	return len(id) == 36 && id[8] == '-' && id[13] == '-' && id[18] == '-' && id[23] == '-' &&
		isHex(id[:8]+id[9:13]+id[14:18]+id[19:23]+id[24:], 32)
}

func findReceipt(ctx context.Context, tx pgx.Tx, user int64, scope, key string, command any) (Receipt, bool, error) {
	if !validIdentityKey(user, key) {
		return Receipt{}, false, ErrInvalid
	}
	kh, rh := hashes(key, command)
	var stored, body []byte
	err := tx.QueryRow(ctx, "SELECT request_hash,response FROM poker.request_receipts WHERE newapi_user_id=$1 AND scope=$2 AND key_hash=$3", user, scope, kh[:]).Scan(&stored, &body)
	if errors.Is(err, pgx.ErrNoRows) {
		return Receipt{}, false, nil
	}
	if err != nil {
		return Receipt{}, false, err
	}
	if !equal(stored, rh[:]) {
		return Receipt{}, false, ErrConflict
	}
	var r Receipt
	if json.Unmarshal(body, &r) != nil {
		return r, false, ErrInvalid
	}
	r.Duplicate = true
	return r, true, nil
}
func saveReceipt(ctx context.Context, tx pgx.Tx, user int64, scope, key string, command any, r Receipt) error {
	kh, rh := hashes(key, command)
	body, _ := json.Marshal(r)
	_, err := tx.Exec(ctx, "INSERT INTO poker.request_receipts(newapi_user_id,scope,key_hash,request_hash,table_id,response) VALUES($1,$2,$3,$4,$5,$6)", user, scope, kh[:], rh[:], r.TableID, body)
	return err
}
func equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

func (s *Service) CreateTable(ctx context.Context, c CreateTableCommand) (Receipt, error) {
	if c.AccessMode == "PASSWORD" {
		return s.createPasswordTable(ctx, c)
	}
	if c.AccessMode != "" && c.AccessMode != "PUBLIC" || c.Password != "" {
		return Receipt{}, ErrInvalid
	}
	c.AccessMode = "" // PUBLIC/false preserves the exact legacy create receipt hash.
	if !validIdentityKey(c.UserID, c.Key) || !utf8.ValidString(c.Name) || c.Name == "" || strings.TrimSpace(c.Name) != c.Name || strings.IndexFunc(c.Name, unicode.IsControl) >= 0 || uniseg.GraphemeClusterCount(c.Name) > 40 || c.MaxSeats < 2 || c.MaxSeats > 9 {
		return Receipt{}, ErrInvalid
	}
	tx, err := s.opts.Pool.Begin(ctx)
	if err != nil {
		return Receipt{}, err
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", c.UserID); err != nil {
		return Receipt{}, err
	}
	if r, ok, err := findReceipt(ctx, tx, c.UserID, "poker.create.v1", c.Key, c); err != nil || ok {
		return r, err
	}
	if err = requireAdmission(ctx, tx, nil, c.UserID, c.BlindPreset); err != nil {
		return Receipt{}, err
	}
	id := uuid()
	_, err = tx.Exec(ctx, `INSERT INTO poker.tables(table_id,owner_newapi_user_id,name,max_seats,blind_preset_version,ruleset_version,allow_spectators,access_mode,chat_enabled) VALUES($1,$2,$3,$4,$5,$6,$7,'PUBLIC',$8)`, id, c.UserID, c.Name, c.MaxSeats, c.BlindPreset, approvedRulesetVersion, c.AllowSpectators, c.ChatEnabled)
	if err != nil {
		return Receipt{}, err
	}
	for seat := 1; seat <= c.MaxSeats; seat++ {
		var seed [32]byte
		if _, err = io.ReadFull(rand.Reader, seed[:]); err != nil {
			return Receipt{}, err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO poker.seats(table_id,seat_no,next_client_seed_contribution) VALUES($1,$2,$3)", id, seat, "pcs1-"+hex.EncodeToString(seed[:])); err != nil {
			return Receipt{}, err
		}
	}
	r := Receipt{TableID: id, Status: "WAITING", Version: 1}
	if err = saveReceipt(ctx, tx, c.UserID, "poker.create.v1", c.Key, c, r); err != nil {
		return Receipt{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Receipt{}, err
	}
	s.publish(id, 1)
	return r, nil
}
