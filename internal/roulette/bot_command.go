package roulette

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrQuoteExpired = errors.New("ROULETTE_QUOTE_EXPIRED")

// BotIntent identifies exactly one original decision; its request owns the key.
type BotIntent struct {
	Purpose string
	Game    string
	RoundID string
	Create  *CreateRequest
	Command *Command
}

type BotLookup struct {
	Status  string
	Receipt *Receipt
}

func (s *Service) CreateBefore(ctx context.Context, user int64, in CreateRequest, expiresAt time.Time) (Receipt, error) {
	return s.create(ctx, user, in, &expiresAt)
}

func (s *Service) CommandBefore(ctx context.Context, user int64, id string, in Command, expiresAt time.Time) (Receipt, error) {
	return s.command(ctx, user, id, in, &expiresAt)
}

// The caller holds the actor/key lock and has found no original receipt. A nil
// deadline is reserved for the unchanged browser path, not for Bot requests.
func checkBotExpiry(ctx context.Context, tx pgx.Tx, expiresAt *time.Time) error {
	if expiresAt == nil {
		return nil
	}
	now, err := dbNow(ctx, tx)
	if err != nil {
		return err
	}
	if !now.Before(*expiresAt) {
		return ErrQuoteExpired
	}
	return nil
}

func botIntentSemantic(user int64, intent BotIntent) (string, [32]byte, error) {
	if !IsGame(intent.Game) {
		return "", [32]byte{}, ErrInvalidInput
	}
	switch intent.Purpose {
	case "CREATE":
		if intent.Create == nil || intent.Command != nil || intent.RoundID != "" || intent.Game != intent.Create.Game {
			return "", [32]byte{}, ErrInvalidInput
		}
		_, hash, err := createSemantic(user, *intent.Create)
		return intent.Create.Key, hash, err
	case "COMMAND":
		if intent.Command == nil || intent.Create != nil {
			return "", [32]byte{}, ErrInvalidInput
		}
		if err := validateCommand(user, intent.RoundID, *intent.Command); err != nil {
			return "", [32]byte{}, err
		}
		return intent.Command.Key, commandHash(intent.RoundID, *intent.Command), nil
	default:
		return "", [32]byte{}, ErrInvalidInput
	}
}

// FindReceiptMatching orders recovery with the original submission. It never
// loads/advances the game snapshot or runs settlement. Lock/transaction errors
// are returned to the adapter, which must treat them as UNKNOWN, not absence.
func (s *Service) FindReceiptMatching(ctx context.Context, user int64, intent BotIntent, expiresAt time.Time) (BotLookup, error) {
	key, semantic, err := botIntentSemantic(user, intent)
	if err != nil {
		return BotLookup{}, err
	}
	var result BotLookup
	err = s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := lockCommand(ctx, tx, user, key); err != nil {
			return err
		}
		receipt, exists, err := readReceipt(ctx, tx, user, key, &semantic)
		if err != nil {
			return err
		}
		if intent.Command != nil {
			// Game is immutable room metadata, not part of the browser's command
			// hash. Validate the Bot intent without reading the current snapshot.
			var game string
			err = tx.QueryRow(ctx, `SELECT game_slug FROM roulette.rounds WHERE round_id=$1`, intent.RoundID).Scan(&game)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			if game != intent.Game {
				return ErrInvalidInput
			}
		}
		if exists {
			result = BotLookup{Status: "APPLIED", Receipt: &receipt}
			return nil
		}
		now, err := dbNow(ctx, tx)
		if err != nil {
			return err
		}
		result.Status = "UNKNOWN"
		if !now.Before(expiresAt) {
			result.Status = "ABSENT_FINAL"
		}
		return nil
	})
	if err != nil {
		return BotLookup{}, err
	}
	return result, nil
}
