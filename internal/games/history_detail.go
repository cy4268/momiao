package games

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	bj "github.com/cy4268/momiao/internal/games/blackjack"
	"github.com/cy4268/momiao/internal/games/slot"
	"github.com/cy4268/momiao/internal/historyaccess"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

type HistoryRoundMetadata struct {
	SnapshotID       string    `json:"snapshot_id"`
	Game             string    `json:"game_slug"`
	GameTitle        string    `json:"game_title"`
	ActorDisplayName *string   `json:"actor_display_name"`
	Origin           string    `json:"metadata_origin"`
	CapturedAt       time.Time `json:"captured_at"`
}

type HistoryRoundAction struct {
	ID              string    `json:"action_id"`
	Type            string    `json:"action_type"`
	HandID          string    `json:"hand_id,omitempty"`
	NewHandID       string    `json:"new_hand_id,omitempty"`
	Sequence        int       `json:"action_sequence"`
	At              time.Time `json:"created_at"`
	AdditionalStake int64     `json:"additional_stake_units,string"`
}

type HistoryBlackjack struct {
	Hands          []bj.PublicHand      `json:"hands"`
	DealerCards    []uint16             `json:"dealer_cards"`
	DealerRevealed bool                 `json:"dealer_revealed"`
	DealerTotal    *bj.Value            `json:"dealer_total,omitempty"`
	Actions        []HistoryRoundAction `json:"actions"`
}

// Whitelist intentionally does not embed GameRound or the live BJ Projection.
type HistoryRound struct {
	ID                      string                        `json:"id"`
	Game                    string                        `json:"game"`
	State                   string                        `json:"state"`
	RecoveryState           string                        `json:"recovery_state"`
	Input                   CreateInput                   `json:"input"`
	Metadata                HistoryRoundMetadata          `json:"metadata"`
	StakeUnits              int64                         `json:"total_stake_units,string"`
	PayoutUnits             *int64                        `json:"total_payout_units,string"`
	NetUnits                *int64                        `json:"net_change_units,string"`
	Outcome                 Outcome                       `json:"common_result,omitempty"`
	BalanceBeforeUnits      int64                         `json:"balance_before_units,string"`
	BalanceAfterUnits       int64                         `json:"balance_after_units,string"`
	CreatedAt               time.Time                     `json:"created_at"`
	SettledAt               *time.Time                    `json:"settled_at"`
	PresentationCompletedAt *time.Time                    `json:"presentation_completed_at,omitempty"`
	Fairness                Commitment                    `json:"fairness"`
	Transactions            []platform.HistoryTransaction `json:"transactions"`
	Dice                    *DiceResult                   `json:"dice,omitempty"`
	Scratch                 *ScratchResult                `json:"scratch,omitempty"`
	Summon                  *SummonResult                 `json:"summon,omitempty"`
	Slot                    *slot.Result                  `json:"slot,omitempty"`
	Blackjack               *HistoryBlackjack             `json:"blackjack,omitempty"`
}

// Proof is requested separately; terminal-only seed/audit policy stays in Verify.
type HistoryVerification struct {
	RoundID         string          `json:"round_id"`
	Commitment      Commitment      `json:"commitment"`
	ServerSeedHash  string          `json:"server_seed_hash"`
	ServerSeed      string          `json:"server_seed,omitempty"`
	RevealState     string          `json:"reveal_state"`
	Verified        bool            `json:"verified"`
	CanonicalConfig string          `json:"canonical_config,omitempty"`
	BlackjackAudit  *BlackjackAudit `json:"blackjack_audit,omitempty"`
}

func (s *Service) historyBlackjack(ctx context.Context, tx pgx.Tx, user int64, r *GameRound) error {
	state, _, _, _, err := s.recoverBlackjack(ctx, tx, user, *r)
	if err != nil {
		return err
	}
	p := bj.PublicView(state, 0)
	r.Blackjack = &p
	return nil
}

func historyRoundError(ctx context.Context, err error) error {
	if errors.Is(err, ErrNotFound) {
		return historyaccess.ErrNotFound
	}
	return platform.HistoryReadError(ctx, err)
}

func (s *Service) HistoryDetail(ctx context.Context, access historyaccess.Access, id string) (HistoryRound, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, subject, err := access.Check(ctx)
	if err != nil {
		return HistoryRound{}, err
	}
	if !platform.ValidOperationKey(id) {
		return HistoryRound{}, historyaccess.ErrInvalid
	}
	var detail HistoryRound
	err = s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if e := platform.HistoryReadOnly(ctx, tx); e != nil {
			return e
		}
		r, e := s.readRoundMode(ctx, tx, subject, id, true)
		if e != nil {
			return e
		}
		detail = HistoryRound{ID: r.ID, Game: r.Game, State: r.State, RecoveryState: r.RecoveryState, Input: r.Input,
			StakeUnits: r.StakeUnits, Outcome: r.Outcome, BalanceBeforeUnits: r.BalanceBeforeUnits, BalanceAfterUnits: r.BalanceAfterUnits,
			CreatedAt: r.CreatedAt.UTC(), SettledAt: r.SettledAt, PresentationCompletedAt: r.PresentationCompletedAt,
			Dice: r.Dice, Scratch: r.Scratch, Summon: r.Summon, Slot: r.Slot, Transactions: []platform.HistoryTransaction{}}
		if r.State == "SETTLED" {
			if r.NetUnits != r.PayoutUnits-r.StakeUnits || r.SettlementTransactionID == "" {
				return historyaccess.ErrUnavailable
			}
			detail.PayoutUnits, detail.NetUnits = &r.PayoutUnits, &r.NetUnits
		} else if r.SettlementTransactionID != "" {
			return historyaccess.ErrUnavailable
		}
		var raw []byte
		if e = tx.QueryRow(ctx, "SELECT games.history_record_snapshot($1,'DIRECT_PLAY_ROUND',$2)", subject, id).Scan(&raw); e != nil {
			return e
		}
		if json.Unmarshal(raw, &detail.Metadata) != nil || detail.Metadata.SnapshotID == "" || detail.Metadata.Game != r.Game {
			return historyaccess.ErrUnavailable
		}
		detail.Fairness, e = scanCommitment(tx.QueryRow(ctx, `SELECT `+commitmentColumns+` FROM games.fairness_commitments WHERE commitment_id=$1 AND newapi_user_id=$2`, r.CommitmentID, subject))
		if e != nil {
			return historyaccess.ErrUnavailable
		}
		if detail.Fairness.ReservedRoundID != r.ID || detail.Fairness.ConfigHash != r.ConfigHash || detail.Fairness.PolicyHash != r.PolicyHash || detail.Fairness.Nonce != r.Nonce {
			return historyaccess.ErrUnavailable
		}
		if p := r.Blackjack; p != nil {
			detail.Blackjack = &HistoryBlackjack{Hands: p.Hands, DealerCards: p.DealerCards, DealerRevealed: p.DealerRevealed, DealerTotal: p.DealerTotal, Actions: []HistoryRoundAction{}}
		}
		return historyRoundTransactions(ctx, tx, subject, r, &detail)
	})
	if err != nil {
		return HistoryRound{}, historyRoundError(ctx, err)
	}
	return detail, nil
}

func historyRoundTransactions(ctx context.Context, tx pgx.Tx, subject int64, r GameRound, detail *HistoryRound) error {
	ids := []string{r.WagerTransactionID}
	rows, err := tx.Query(ctx, `SELECT action_id::text,action_type,coalesce(hand_id::text,''),coalesce(new_hand_id::text,''),action_sequence,created_at,additional_stake_units,coalesce(stake_transaction_id::text,''),newapi_user_id FROM games.round_actions WHERE round_id=$1 ORDER BY action_sequence`, r.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var a HistoryRoundAction
		var stake string
		var owner int64
		if err = rows.Scan(&a.ID, &a.Type, &a.HandID, &a.NewHandID, &a.Sequence, &a.At, &a.AdditionalStake, &stake, &owner); err != nil {
			rows.Close()
			return err
		}
		if owner != subject || (a.AdditionalStake > 0) != (stake != "") || (stake != "" && r.Game != "blackjack") {
			rows.Close()
			return historyaccess.ErrUnavailable
		}
		if stake != "" {
			ids = append(ids, stake)
		}
		if detail.Blackjack != nil {
			a.At = a.At.UTC()
			detail.Blackjack.Actions = append(detail.Blackjack.Actions, a)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if r.SettlementTransactionID != "" {
		ids = append(ids, r.SettlementTransactionID)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			return historyaccess.ErrUnavailable
		}
		seen[id] = true
		t, e := platform.HistoryTransactionInTx(ctx, tx, subject, id)
		if e != nil {
			return historyaccess.ErrUnavailable
		}
		if len(t.Links) != 1 || t.Links[0].RecordType != "DIRECT_PLAY_ROUND" || t.Links[0].SourceID != r.ID {
			return historyaccess.ErrUnavailable
		}
		detail.Transactions = append(detail.Transactions, t)
	}
	return nil
}

func (s *Service) HistoryVerify(ctx context.Context, access historyaccess.Access, id string) (HistoryVerification, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, subject, err := access.Check(ctx)
	if err != nil {
		return HistoryVerification{}, err
	}
	if !platform.ValidOperationKey(id) {
		return HistoryVerification{}, historyaccess.ErrInvalid
	}
	v, err := s.verifyRound(ctx, subject, id, true)
	if err != nil {
		return HistoryVerification{}, historyRoundError(ctx, err)
	}
	return HistoryVerification{RoundID: v.RoundID, Commitment: v.Commitment, ServerSeedHash: v.ServerSeedHash, ServerSeed: v.ServerSeed,
		RevealState: v.RevealState, Verified: v.Verified, CanonicalConfig: v.CanonicalConfig, BlackjackAudit: v.BlackjackAudit}, nil
}
