package games

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	bj "github.com/cy4268/momiao/internal/games/blackjack"
	"github.com/cy4268/momiao/internal/games/fairness"
	"github.com/jackc/pgx/v5"
)

type BlackjackActionInput struct {
	ActionID        string `json:"action_id"`
	ActionType      string `json:"action_type"`
	HandID          string `json:"hand_id"`
	ExpectedVersion string `json:"expected_round_version"`
}
type BlackjackAuditAction struct {
	ID              string        `json:"action_id"`
	Type            bj.ActionType `json:"action_type"`
	HandID          string        `json:"hand_id"`
	NewHandID       string        `json:"new_hand_id,omitempty"`
	ExpectedVersion int64         `json:"expected_round_version,string"`
	Sequence        int           `json:"action_sequence"`
	At              time.Time     `json:"created_at"`
	AdditionalStake int64         `json:"additional_stake_units,string"`
}
type BlackjackAudit struct {
	Shoe          [312]uint16            `json:"shoe_instance_ids"`
	ShoeHash      string                 `json:"shoe_hash"`
	InitialHandID string                 `json:"initial_hand_id"`
	InitialWager  int64                  `json:"initial_wager_units,string"`
	CreatedAt     time.Time              `json:"created_at"`
	Actions       []BlackjackAuditAction `json:"actions"`
}

func blackjackShoe(seed []byte, fair FairRound) ([312]uint16, error) {
	stream, err := fairness.NewStream(seed, fair, bj.ShuffleDomain)
	if err != nil {
		return [312]uint16{}, err
	}
	return bj.Shuffle(func(n uint32) (uint32, error) { v, e := fairness.UniformInt(stream, uint64(n)); return uint32(v), e })
}
func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC()
}

func persistBlackjackState(ctx context.Context, tx pgx.Tx, id string, s bj.State) error {
	_, err := tx.Exec(ctx, `INSERT INTO games.blackjack_round_state(round_id,initial_hand_id,initial_wager_units,shoe_hash,shoe_index,round_version,active_hand_id,dealer_revealed,last_player_action_at,auto_resolve_at) VALUES($1,$2,$3,decode($4,'hex'),$5,$6,NULLIF($7,'')::uuid,$8,$9,$10) ON CONFLICT(round_id) DO UPDATE SET shoe_index=excluded.shoe_index,round_version=excluded.round_version,active_hand_id=excluded.active_hand_id,dealer_revealed=excluded.dealer_revealed,last_player_action_at=excluded.last_player_action_at,auto_resolve_at=excluded.auto_resolve_at`, id, s.InitialHandID, s.InitialWagerUnits, s.ShoeHash, s.ShoeIndex, s.Version, s.ActiveHandID, s.DealerRevealed, nullableTime(s.LastPlayerActionAt), nullableTime(s.AutoResolveAt))
	if err != nil {
		return err
	}
	for _, h := range s.Hands {
		_, err = tx.Exec(ctx, `INSERT INTO games.blackjack_hands(hand_id,round_id,hand_index,parent_hand_id,stake_units,from_split,split_aces,is_natural,hand_state,hard_total,best_total,is_soft,result,payout_units,net_change_units) VALUES($1,$2,$3,NULLIF($4,'')::uuid,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) ON CONFLICT(hand_id) DO UPDATE SET stake_units=excluded.stake_units,from_split=excluded.from_split,split_aces=excluded.split_aces,is_natural=excluded.is_natural,hand_state=excluded.hand_state,hard_total=excluded.hard_total,best_total=excluded.best_total,is_soft=excluded.is_soft,result=excluded.result,payout_units=excluded.payout_units,net_change_units=excluded.net_change_units`, h.ID, id, h.Index, h.ParentID, h.StakeUnits, h.FromSplit, h.SplitAces, h.Natural, h.Status, h.Value.HardTotal, h.Value.BestTotal, h.Value.Soft, h.Result, h.PayoutUnits, h.NetChangeUnits)
		if err != nil {
			return err
		}
	}
	recipients := map[int]string{}
	for _, h := range s.Hands {
		for _, card := range h.Cards {
			recipients[card.ShoeIndex] = h.ID
		}
	}
	for _, card := range s.Dealt {
		hand := recipients[card.ShoeIndex]
		recipient := "PLAYER"
		if hand == "" {
			recipient = "DEALER"
		}
		_, err = tx.Exec(ctx, `INSERT INTO games.blackjack_cards(round_id,shoe_index,card_instance_id,recipient,hand_id) VALUES($1,$2,$3,$4,NULLIF($5,'')::uuid) ON CONFLICT(round_id,shoe_index) DO UPDATE SET hand_id=excluded.hand_id WHERE games.blackjack_cards.hand_id IS DISTINCT FROM excluded.hand_id`, id, card.ShoeIndex, card.InstanceID, recipient, hand)
		if err != nil {
			return err
		}
	}
	return nil
}

func loadBlackjackState(ctx context.Context, tx pgx.Tx, r GameRound) (bj.State, error) {
	s := bj.State{CreatedAt: r.CreatedAt, Phase: bj.Phase(r.State), TotalStakeUnits: r.StakeUnits, TotalPayoutUnits: r.PayoutUnits}
	if r.State == "SETTLED" {
		s.NetChangeUnits = r.NetUnits
		s.Class = string(r.Outcome)
	}
	var last, due *time.Time
	err := tx.QueryRow(ctx, `SELECT initial_hand_id::text,initial_wager_units,encode(shoe_hash,'hex'),shoe_index,round_version,coalesce(active_hand_id::text,''),dealer_revealed,last_player_action_at,auto_resolve_at FROM games.blackjack_round_state WHERE round_id=$1`, r.ID).Scan(&s.InitialHandID, &s.InitialWagerUnits, &s.ShoeHash, &s.ShoeIndex, &s.Version, &s.ActiveHandID, &s.DealerRevealed, &last, &due)
	if err != nil {
		return s, err
	}
	if last != nil {
		s.LastPlayerActionAt = last.UTC()
	}
	if due != nil {
		s.AutoResolveAt = due.UTC()
	}
	rows, err := tx.Query(ctx, `SELECT hand_id::text,hand_index,coalesce(parent_hand_id::text,''),stake_units,from_split,split_aces,is_natural,hand_state,hard_total,best_total,is_soft,result,payout_units,net_change_units FROM games.blackjack_hands WHERE round_id=$1 ORDER BY hand_index`, r.ID)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var h bj.Hand
		if err = rows.Scan(&h.ID, &h.Index, &h.ParentID, &h.StakeUnits, &h.FromSplit, &h.SplitAces, &h.Natural, &h.Status, &h.Value.HardTotal, &h.Value.BestTotal, &h.Value.Soft, &h.Result, &h.PayoutUnits, &h.NetChangeUnits); err != nil {
			rows.Close()
			return s, err
		}
		s.Hands = append(s.Hands, h)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return s, err
	}
	rows, err = tx.Query(ctx, `SELECT shoe_index,card_instance_id,recipient,coalesce(hand_id::text,'') FROM games.blackjack_cards WHERE round_id=$1 ORDER BY shoe_index`, r.ID)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var c bj.Card
		var recipient, hand string
		if err = rows.Scan(&c.ShoeIndex, &c.InstanceID, &recipient, &hand); err != nil {
			rows.Close()
			return s, err
		}
		s.Dealt = append(s.Dealt, c)
		if recipient == "DEALER" {
			s.Dealer = append(s.Dealer, c)
		} else {
			found := false
			for i := range s.Hands {
				if s.Hands[i].ID == hand {
					s.Hands[i].Cards = append(s.Hands[i].Cards, c)
					found = true
					break
				}
			}
			if !found {
				rows.Close()
				return s, bj.ErrNeedsReview
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return s, err
	}
	rows, err = tx.Query(ctx, `SELECT action_id::text,expected_round_version,hand_id::text,action_type,coalesce(new_hand_id::text,''),action_sequence,created_at,additional_stake_units FROM games.round_actions WHERE round_id=$1 ORDER BY action_sequence`, r.ID)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var a bj.ActionRecord
		if err = rows.Scan(&a.Action.ID, &a.Action.ExpectedVersion, &a.Action.HandID, &a.Action.Type, &a.Action.NewHandID, &a.Sequence, &a.At, &a.AdditionalStakeUnits); err != nil {
			rows.Close()
			return s, err
		}
		s.Actions = append(s.Actions, a)
	}
	err = rows.Err()
	rows.Close()
	return s, err
}

func (s *Service) recoverBlackjack(ctx context.Context, tx pgx.Tx, user int64, r GameRound) (bj.State, [312]uint16, []byte, error) {
	var shoe [312]uint16
	if r.RecoveryState != "NORMAL" {
		return bj.State{}, shoe, nil, bj.ErrNeedsReview
	}
	state, err := loadBlackjackState(ctx, tx, r)
	if err != nil {
		return state, shoe, nil, err
	}
	c, err := scanCommitment(tx.QueryRow(ctx, `SELECT `+commitmentColumns+` FROM games.fairness_commitments WHERE commitment_id=$1 AND newapi_user_id=$2`, r.CommitmentID, user))
	if err != nil {
		return state, shoe, nil, err
	}
	cfg, err := configByID(ctx, tx, "blackjack", r.ConfigVersion)
	if err != nil {
		return state, shoe, nil, err
	}
	var key string
	var nonce, encrypted []byte
	if err = tx.QueryRow(ctx, `SELECT key_version,gcm_nonce,ciphertext FROM games.fairness_commitments WHERE commitment_id=$1`, c.ID).Scan(&key, &nonce, &encrypted); err != nil {
		return state, shoe, nil, err
	}
	seed, err := s.openSeed(user, "blackjack", c, key, nonce, encrypted)
	if err != nil {
		return state, shoe, nil, bj.ErrNeedsReview
	}
	fair, err := fairnessInput("blackjack", c)
	if err != nil {
		return state, shoe, nil, bj.ErrNeedsReview
	}
	if err = cfg.checkRound(fair, "blackjack"); err != nil {
		return state, shoe, nil, bj.ErrNeedsReview
	}
	if r.ConfigHash != c.ConfigHash || r.PolicyHash != c.PolicyHash || r.Nonce != c.Nonce {
		return state, shoe, nil, bj.ErrNeedsReview
	}
	shoe, err = blackjackShoe(seed, fair)
	if err != nil {
		return state, shoe, nil, bj.ErrNeedsReview
	}
	if err = bj.VerifyRecovery(state, shoe); err != nil {
		return state, shoe, nil, err
	}
	return state, shoe, seed, nil
}
func markBlackjackReview(ctx context.Context, tx pgx.Tx, r GameRound) error {
	if r.State == "PLAYER_TURN" && r.RecoveryState == "NORMAL" {
		if _, err := tx.Exec(ctx, `UPDATE games.game_rounds SET recovery_state='NEEDS_REVIEW' WHERE round_id=$1`, r.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE games.round_jobs SET status='NEEDS_REVIEW',last_error='RECOVERY_MISMATCH',updated_at=clock_timestamp() WHERE round_id=$1 AND status='PENDING'`, r.ID); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) publicBlackjack(ctx context.Context, tx pgx.Tx, user int64, r *GameRound) error {
	state, _, _, err := s.recoverBlackjack(ctx, tx, user, *r)
	if errors.Is(err, bj.ErrNeedsReview) {
		if e := markBlackjackReview(ctx, tx, *r); e != nil {
			return e
		}
		r.RecoveryState = "NEEDS_REVIEW"
		return nil
	}
	if err != nil {
		return err
	}
	var available int64
	if err = tx.QueryRow(ctx, `SELECT balance_units FROM economy.wallet_balances WHERE newapi_user_id=$1 AND asset_type='AVAILABLE_CHIPS'`, user).Scan(&available); err != nil {
		return err
	}
	p := bj.PublicView(state, available)
	r.Blackjack = &p
	return nil
}
func blackjackAudit(state bj.State, shoe [312]uint16) *BlackjackAudit {
	a := &BlackjackAudit{Shoe: shoe, ShoeHash: state.ShoeHash, InitialHandID: state.InitialHandID, InitialWager: state.InitialWagerUnits, CreatedAt: state.CreatedAt, Actions: []BlackjackAuditAction{}}
	for _, record := range state.Actions {
		v := record.Action
		a.Actions = append(a.Actions, BlackjackAuditAction{v.ID, v.Type, v.HandID, v.NewHandID, v.ExpectedVersion, record.Sequence, record.At, record.AdditionalStakeUnits})
	}
	return a
}

// Keep hex conversion centralized in the protected storage path.
func decodeHash(value string) []byte { v, _ := hex.DecodeString(value); return v }
