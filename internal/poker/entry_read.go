package poker

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type EntryReceiptQuery struct {
	Kind, MutationID string
	TableID          *string
}

type EntryReceiptLookup struct {
	UserID     string   `json:"user_id"`
	Kind       string   `json:"kind"`
	MutationID string   `json:"mutation_id"`
	State      string   `json:"state"`
	Receipt    *Receipt `json:"receipt,omitempty"`
}

type ReservationView struct {
	SeatNo       int       `json:"seat_no"`
	DurableState string    `json:"durable_state"`
	ExpiresAt    time.Time `json:"expires_at"`
	CheckedAt    time.Time `json:"checked_at"`
	Valid        bool      `json:"valid"`
}

type ReservationLookup struct {
	UserID        string           `json:"user_id"`
	TableID       string           `json:"table_id"`
	ReservationID string           `json:"reservation_id"`
	State         string           `json:"state"`
	Reservation   *ReservationView `json:"reservation,omitempty"`
}

// LookupEntryReceipt recovers an owner's original entry acknowledgement, not a
// current admission grant. Only create discovers its table from the durable row.
func (s *Service) LookupEntryReceipt(ctx context.Context, userID int64, q EntryReceiptQuery) (EntryReceiptLookup, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if !validIdentityKey(userID, q.MutationID) {
		return EntryReceiptLookup{}, ErrInvalid
	}
	var scope string
	switch q.Kind {
	case "create":
		if q.TableID != nil {
			return EntryReceiptLookup{}, ErrInvalid
		}
		scope = "poker.create.v1"
	case "reserve", "buyin":
		if q.TableID == nil || !canonicalSessionID(*q.TableID) {
			return EntryReceiptLookup{}, ErrInvalid
		}
		scope = "poker." + q.Kind + ".v1"
	default:
		return EntryReceiptLookup{}, ErrInvalid
	}
	result := EntryReceiptLookup{UserID: decimal(userID), Kind: q.Kind, MutationID: q.MutationID, State: "NOT_FOUND"}
	key := sha256.Sum256([]byte(q.MutationID))
	var table string
	var body []byte
	err := s.opts.Pool.QueryRow(ctx, `SELECT table_id::text,response FROM poker.request_receipts
 WHERE newapi_user_id=$1 AND scope=$2 AND key_hash=$3 AND ($4::uuid IS NULL OR table_id=$4)`, userID, scope, key[:], q.TableID).Scan(&table, &body)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil // Visibility only; this never proves the command had no effect.
	}
	if err != nil {
		return EntryReceiptLookup{}, err
	}
	var receipt Receipt
	if json.Unmarshal(body, &receipt) != nil || !canonicalSessionID(table) || !canonicalSessionID(receipt.TableID) || receipt.TableID != table || receipt.Status == "" {
		return EntryReceiptLookup{}, errors.New("poker entry receipt projection invalid")
	}
	result.State, result.Receipt = "FOUND", &receipt
	return result, nil
}

// Reservation is a short-lived observation, not an atomic PG/Redis grant. BuyIn
// still checks the original lease and durable deadline after its wallet wait.
func (s *Service) Reservation(ctx context.Context, userID int64, tableID, reservationID string) (ReservationLookup, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if userID < 1 || !canonicalSessionID(tableID) || !canonicalSessionID(reservationID) {
		return ReservationLookup{}, ErrInvalid
	}
	result := ReservationLookup{UserID: decimal(userID), TableID: tableID, ReservationID: reservationID, State: "NOT_FOUND"}
	tx, err := s.opts.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadOnly})
	if err != nil {
		return ReservationLookup{}, err
	}
	defer tx.Rollback(ctx)
	type row struct {
		ReservationView
		token string
		empty bool
	}
	read := func() (row, error) {
		var r row
		err := tx.QueryRow(ctx, `SELECT r.seat_no,r.state,r.expires_at,r.lease_token,
 (SELECT s.session_id IS NULL FROM poker.seats s WHERE s.table_id=r.table_id AND s.seat_no=r.seat_no),clock_timestamp()
 FROM poker.seat_reservations r WHERE r.newapi_user_id=$1 AND r.table_id=$2 AND r.reservation_id=$3`, userID, tableID, reservationID).Scan(&r.SeatNo, &r.DurableState, &r.ExpiresAt, &r.token, &r.empty, &r.CheckedAt)
		if err == nil && (r.SeatNo < 1 || r.SeatNo > 9 || r.ExpiresAt.IsZero() || r.CheckedAt.IsZero() || r.token == "") {
			err = errors.New("poker reservation projection invalid")
		}
		return r, err
	}
	candidate := func(r row) bool {
		return r.DurableState == "LEASE_ACTIVE" && r.ExpiresAt.After(r.CheckedAt) && r.empty
	}
	first, err := read()
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return ReservationLookup{}, err
	}
	current := first
	if candidate(first) {
		valid, e := s.opts.Leases.Valid(ctx, tableID, first.SeatNo, first.token, first.CheckedAt)
		if e != nil {
			return ReservationLookup{}, e
		}
		current, err = read() // READ COMMITTED deliberately obtains a new snapshot.
		if errors.Is(err, pgx.ErrNoRows) {
			return result, nil
		}
		if err != nil {
			return ReservationLookup{}, err
		}
		current.Valid = valid && candidate(current) && current.SeatNo == first.SeatNo && current.token == first.token && current.ExpiresAt.Equal(first.ExpiresAt)
	}
	result.State, result.Reservation = "FOUND", &current.ReservationView
	return result, nil
}
