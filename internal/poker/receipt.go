package poker

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

type ReceiptQuery struct {
	TableID    string
	Kind       string
	MutationID string
}

type ReceiptLookup struct {
	TableID    string   `json:"table_id"`
	Kind       string   `json:"kind"`
	MutationID string   `json:"mutation_id"`
	State      string   `json:"state"`
	Receipt    *Receipt `json:"receipt,omitempty"`
}

// LookupReceipt reads one authenticated owner's historical acknowledgement.
// It neither retries a command nor needs a current seat, Actor or Controller.
// NOT_FOUND is only this SELECT's visibility result, never proof of no effect.
func (s *Service) LookupReceipt(ctx context.Context, userID int64, q ReceiptQuery) (ReceiptLookup, error) {
	if !validIdentityKey(userID, q.MutationID) || !canonicalSessionID(q.TableID) {
		return ReceiptLookup{}, ErrInvalid
	}
	var scope string
	switch q.Kind {
	case "action":
		scope = "poker.action.v1"
	case "sitout":
		scope = "poker.sitout.v1"
	case "resume":
		scope = "poker.resume.v1"
	case "nextseed":
		scope = "poker.nextseed.v1"
	case "topup":
		scope = "poker.topup.v1"
	case "leave":
		scope = "poker.leave.v1"
	case "takeover":
		scope = "poker.control.v1"
	case "host":
		scope = "poker.host.v1"
	case "chat":
		scope = "poker.chat.v1"
	default:
		return ReceiptLookup{}, ErrInvalid
	}
	kh := sha256.Sum256([]byte(q.MutationID))
	var body []byte
	err := s.opts.Pool.QueryRow(ctx, `SELECT response FROM poker.request_receipts
 WHERE newapi_user_id=$1 AND scope=$2 AND key_hash=$3 AND table_id=$4`, userID, scope, kh[:], q.TableID).Scan(&body)
	result := ReceiptLookup{TableID: q.TableID, Kind: q.Kind, MutationID: q.MutationID, State: "NOT_FOUND"}
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return ReceiptLookup{}, err
	}
	var receipt Receipt
	if json.Unmarshal(body, &receipt) != nil || receipt.TableID != q.TableID || receipt.Status == "" {
		return ReceiptLookup{}, errors.New("poker receipt projection invalid")
	}
	// Preserve the stored duplicate flag and control_epoch as receipt metadata.
	result.State, result.Receipt = "FOUND", &receipt
	return result, nil
}
