package games

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/cy4268/momiao/internal/games/slot"
	"github.com/cy4268/momiao/internal/platform"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Service) readRound(ctx context.Context, tx pgx.Tx, user int64, id string) (GameRound, error) {
	return s.readRoundMode(ctx, tx, user, id, false)
}

func (s *Service) readRoundMode(ctx context.Context, tx pgx.Tx, user int64, id string, historical bool) (GameRound, error) {
	var r GameRound
	var raw []byte
	var slug string
	err := tx.QueryRow(ctx, `SELECT game_slug FROM games.game_rounds WHERE round_id=$1 AND newapi_user_id=$2`, id, user).Scan(&slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	// Read the mutable typed tables under the same user/game lock as actions.
	if slug == "blackjack" && !historical {
		if err = lockUserGame(ctx, tx, user, slug); err != nil {
			return r, err
		}
	}
	err = tx.QueryRow(ctx, `SELECT round_id::text,game_slug,state,recovery_state,typed_input,total_stake_units,total_payout_units,net_change_units,coalesce(common_result,''),balance_before_units,balance_after_units,wager_transaction_id::text,coalesce(settlement_transaction_id::text,''),game_config_version_id::text,encode(game_config_hash,'hex'),wager_policy_version_id::text,encode(wager_policy_hash,'hex'),algorithm_version,ruleset_version,fairness_stream_version,nonce,commitment_id::text,created_at,settled_at FROM games.game_rounds WHERE round_id=$1 AND newapi_user_id=$2`, id, user).Scan(&r.ID, &r.Game, &r.State, &r.RecoveryState, &raw, &r.StakeUnits, &r.PayoutUnits, &r.NetUnits, &r.Outcome, &r.BalanceBeforeUnits, &r.BalanceAfterUnits, &r.WagerTransactionID, &r.SettlementTransactionID, &r.ConfigVersion, &r.ConfigHash, &r.PolicyVersion, &r.PolicyHash, &r.Algorithm, &r.Ruleset, &r.Stream, &r.Nonce, &r.CommitmentID, &r.CreatedAt, &r.SettledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	if json.Unmarshal(raw, &r.Input) != nil {
		return r, ErrUnavailable
	}
	switch r.Game {
	case "blackjack":
		// Fresh transitions use DB time in UTC; keep retry/read JSON identical
		// when a connection returns the same timestamptz in another time zone.
		r.CreatedAt = r.CreatedAt.UTC()
		if r.SettledAt != nil {
			utc := r.SettledAt.UTC()
			r.SettledAt = &utc
		}
		if historical {
			err = s.historyBlackjack(ctx, tx, user, &r)
		} else {
			err = s.publicBlackjack(ctx, tx, user, &r)
		}
		if err != nil {
			return r, err
		}
	case "slot":
		r.Slot, err = readSlot(ctx, tx, r)
		if err != nil {
			return r, err
		}
	case "dice":
		var dice [3]uint8
		var choice DiceSide
		if err = tx.QueryRow(ctx, `SELECT die_1,die_2,die_3,choice FROM games.dice_results WHERE round_id=$1`, id).Scan(&dice[0], &dice[1], &dice[2], &choice); err != nil {
			return r, err
		}
		d, err := EvaluateDice(dice, choice)
		if err != nil {
			return r, err
		}
		r.Dice = &d
	case "scratch":
		var d ScratchResult
		var payout int64
		if err = tx.QueryRow(ctx, `SELECT prize_tier,payout_multiplier,presentation_completed_at FROM games.scratch_results WHERE round_id=$1`, id).Scan(&d.Tier, &payout, &r.PresentationCompletedAt); err != nil {
			return r, err
		}
		d.Reward = reward(1, payout)
		rows, err := tx.Query(ctx, `SELECT cell_index,symbol,is_matching_symbol FROM games.scratch_cells WHERE round_id=$1 ORDER BY cell_index`, id)
		if err != nil {
			return r, err
		}
		defer rows.Close()
		count := 0
		for rows.Next() {
			var i int
			var cell ScratchCell
			if err = rows.Scan(&i, &cell.Symbol, &cell.Matching); err != nil {
				return r, err
			}
			if i != count || i >= 9 {
				return r, ErrUnavailable
			}
			d.Cells[i] = cell
			count++
		}
		if rows.Err() != nil {
			return r, rows.Err()
		}
		if count != 9 {
			return r, ErrUnavailable
		}
		r.Scratch = &d
	case "summon":
		var d SummonResult
		if err = tx.QueryRow(ctx, `SELECT mode,highest_tier FROM games.summon_results WHERE round_id=$1`, id).Scan(&d.Mode, &d.HighestTier); err != nil {
			return r, err
		}
		rows, err := tx.Query(ctx, `SELECT draw_index,prize_tier,payout_multiplier FROM games.summon_draws WHERE round_id=$1 ORDER BY draw_index`, id)
		if err != nil {
			return r, err
		}
		defer rows.Close()
		var payout int64
		for rows.Next() {
			var draw DrawResult
			if err = rows.Scan(&draw.Index, &draw.Tier, &draw.Multiplier); err != nil {
				return r, err
			}
			if draw.Index != len(d.Draws)+1 {
				return r, ErrUnavailable
			}
			d.Draws = append(d.Draws, draw)
			payout += draw.Multiplier
		}
		if rows.Err() != nil {
			return r, rows.Err()
		}
		count := 1
		if d.Mode == Tenfold {
			count = 10
		}
		if len(d.Draws) != count {
			return r, ErrUnavailable
		}
		d.Reward = reward(int64(count), payout)
		r.Summon = &d
	default:
		return r, ErrUnavailable
	}
	return r, nil
}
func (s *Service) latestRound(ctx context.Context, tx pgx.Tx, user int64, slug string) (*GameRound, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT round_id::text FROM games.game_rounds WHERE newapi_user_id=$1 AND game_slug=$2 ORDER BY nonce DESC LIMIT 1`, user, slug).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r, err := s.readRound(ctx, tx, user, id)
	return &r, err
}
func (s *Service) Read(ctx context.Context, user int64, id string) (GameRound, error) {
	var r GameRound
	if _, err := uuidBytes(id); err != nil {
		return r, ErrInvalidInput
	}
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error { var err error; r, err = s.readRound(ctx, tx, user, id); return err })
	return r, err
}

// FindByKey also takes the create lock. A definitive absent answer cannot race
// a create currently in flight; an uncertain HTTP client must reconcile here.
func (s *Service) FindByKey(ctx context.Context, user int64, slug, key string) (*GameRound, error) {
	var r *GameRound
	if !validRequestKey(key) {
		return nil, ErrInvalidInput
	}
	hash := sha256.Sum256([]byte(key))
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := lockUserGame(ctx, tx, user, slug); err != nil {
			return err
		}
		var id string
		err := tx.QueryRow(ctx, `SELECT round_id::text FROM games.game_rounds WHERE newapi_user_id=$1 AND game_slug=$2 AND idempotency_key_hash=$3`, user, slug, hash[:]).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		result, err := s.readRound(ctx, tx, user, id)
		r = &result
		return err
	})
	return r, err
}
func (s *Service) History(ctx context.Context, user int64, q HistoryQuery) (HistoryPage, error) {
	page := HistoryPage{Items: []GameRound{}}
	if user <= 0 || (q.Game != "" && q.Game != "dice" && q.Game != "scratch" && q.Game != "summon" && q.Game != "slot" && q.Game != "blackjack") {
		return page, ErrInvalidInput
	}
	if q.Limit == 0 {
		q.Limit = 20
	}
	if q.Limit < 1 || q.Limit > 50 {
		return page, ErrInvalidInput
	}
	if q.Before != "" {
		if _, err := uuidBytes(q.Before); err != nil {
			return page, ErrInvalidInput
		}
	}
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		// UUIDs are reserved at bootstrap, potentially long before the actual
		// round. Resolve the cursor from this user's durable record and paginate
		// by the same actual creation time / UUID tuple as the display order.
		var before *time.Time
		if q.Before != "" {
			var timestamp time.Time
			err := tx.QueryRow(ctx, `SELECT created_at FROM games.game_rounds WHERE newapi_user_id=$1 AND round_id=$2 AND ($3='' OR game_slug=$3)`, user, q.Before, q.Game).Scan(&timestamp)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInvalidInput
			}
			if err != nil {
				return err
			}
			before = &timestamp
		}
		rows, err := tx.Query(ctx, `SELECT round_id::text FROM games.game_rounds WHERE newapi_user_id=$1 AND ($2='' OR game_slug=$2) AND ($3::timestamptz IS NULL OR (created_at,round_id)<($3::timestamptz,NULLIF($4,'')::uuid)) ORDER BY created_at DESC,round_id DESC LIMIT $5`, user, q.Game, before, q.Before, q.Limit+1)
		if err != nil {
			return err
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(ids) > q.Limit {
			ids = ids[:q.Limit]
			page.NextCursor = ids[len(ids)-1]
		}
		for _, id := range ids {
			r, err := s.readRound(ctx, tx, user, id)
			if err != nil {
				return err
			}
			page.Items = append(page.Items, r)
		}
		return nil
	})
	return page, err
}
func (s *Service) Verify(ctx context.Context, user int64, id string) (Verification, error) {
	return s.verifyRound(ctx, user, id, false)
}

func (s *Service) verifyRound(ctx context.Context, user int64, id string, historical bool) (Verification, error) {
	var v Verification
	if _, err := uuidBytes(id); err != nil {
		return v, ErrInvalidInput
	}
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if historical {
			if err := platform.HistoryReadOnly(ctx, tx); err != nil {
				return err
			}
		}
		round, err := s.readRoundMode(ctx, tx, user, id, historical)
		if err != nil {
			return err
		}
		c, err := scanCommitment(tx.QueryRow(ctx, `SELECT `+commitmentColumns+` FROM games.fairness_commitments WHERE commitment_id=$1 AND newapi_user_id=$2`, round.CommitmentID, user))
		if err != nil {
			return err
		}
		v = Verification{RoundID: id, Commitment: c, ServerSeedHash: c.ServerSeedHash, RevealState: "NOT_YET_AVAILABLE", Result: round}
		if round.State != "SETTLED" {
			return nil
		}
		if round.Game == "blackjack" {
			state, shoe, seed, _, err := s.recoverBlackjack(ctx, tx, user, round)
			if err != nil {
				return err
			}
			cfg, err := configByID(ctx, tx, "blackjack", round.ConfigVersion)
			if err != nil {
				return err
			}
			v.Verified = true
			v.ServerSeed = hex.EncodeToString(seed)
			v.RevealState = "REVEALED"
			v.CanonicalConfig = string(cfg.CanonicalJSON())
			v.BlackjackAudit = blackjackAudit(state, shoe)
			return nil
		}
		var seed []byte
		if err = tx.QueryRow(ctx, `SELECT revealed_server_seed FROM games.fairness_commitments WHERE commitment_id=$1 AND state='REVEALED'`, c.ID).Scan(&seed); err != nil {
			return err
		}
		cfig, err := configByID(ctx, tx, round.Game, round.ConfigVersion)
		if err != nil {
			return err
		}
		fair, err := fairnessInput(round.Game, c)
		if err != nil {
			return err
		}
		reconstructed := round
		reconstructed.Dice = nil
		reconstructed.Scratch = nil
		reconstructed.Summon = nil
		reconstructed.Slot = nil
		payout, err := derive(&reconstructed, seed, fair, cfig)
		if err != nil {
			return err
		}
		n, err := normalizeCreateForRuleset(round.Game, round.Input, round.Ruleset)
		if err != nil {
			return err
		}
		original, _ := json.Marshal(struct {
			D *DiceResult
			S *ScratchResult
			U *SummonResult
			L *slot.Result
		}{round.Dice, round.Scratch, round.Summon, round.Slot})
		replay, _ := json.Marshal(struct {
			D *DiceResult
			S *ScratchResult
			U *SummonResult
			L *slot.Result
		}{reconstructed.Dice, reconstructed.Scratch, reconstructed.Summon, reconstructed.Slot})
		v.Verified = bytes.Equal(original, replay) && round.PayoutUnits == payout && round.StakeUnits == n.TotalStake && round.ConfigHash == c.ConfigHash && round.PolicyHash == c.PolicyHash && round.Nonce == c.Nonce
		v.ServerSeed = hex.EncodeToString(seed)
		v.RevealState = "REVEALED"
		v.CanonicalConfig = string(cfig.CanonicalJSON())
		return nil
	})
	return v, err
}
