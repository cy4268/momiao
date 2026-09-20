package games

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

// Create accepts only a previously published commitment. All financial effects,
// typed outputs, supply accounting and reveal commit together, once. There is no
// durable nonterminal state for these fast games and no in-memory recovery job.
func (s *Service) Create(ctx context.Context, user int64, slug, key, commitmentID string, input CreateInput) (GameRound, error) {
	if slug == "blackjack" {
		return s.createBlackjack(ctx, user, key, commitmentID, input)
	}
	var round GameRound
	n, err := normalizeCreate(slug, input)
	if err != nil {
		return round, err
	}
	if user <= 0 || !validRequestKey(key) {
		return round, ErrInvalidInput
	}
	if _, err = uuidBytes(commitmentID); err != nil {
		return round, ErrInvalidInput
	}
	keyHash := sha256.Sum256([]byte(key))
	requestHash := semanticHash(user, slug, n.Input)
	err = s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := lockUserGame(ctx, tx, user, slug); err != nil {
			return err
		}
		var existing string
		var hash []byte
		err := tx.QueryRow(ctx, `SELECT round_id::text,request_hash FROM games.game_rounds WHERE newapi_user_id=$1 AND game_slug=$2 AND idempotency_key_hash=$3`, user, slug, keyHash[:]).Scan(&existing, &hash)
		if err == nil {
			if !bytes.Equal(hash, requestHash[:]) {
				return platform.ErrIdempotencyConflict
			}
			round, err = s.readRound(ctx, tx, user, existing)
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := platform.RequireNoMaintenance(ctx, tx, "CHALDEA_USER_WRITES", "DIRECT_PLAY_NEW_ROUNDS"); err != nil {
			if errors.Is(err, platform.ErrMaintenanceActive) { return ErrMaintenance }; return err
		}
		runtime, err := resolveRuntime(ctx, tx, slug, true)
		if err != nil {
			return err
		}
		if runtime.Entry.State == "MAINTENANCE" {
			return ErrMaintenance
		}
		if runtime.Entry.State != "PLAY" {
			return ErrUnavailable
		}
		// Bind the maximum payout before resolving randomness, while preserving
		// historical wager bounds when old rounds are verified or replayed.
		n, err = normalizeCreateForRuleset(slug, input, runtime.Config.binding.RulesetVersion)
		if err != nil {
			return err
		}
		if slug == "scratch" {
			var blocked bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM games.game_rounds r JOIN games.scratch_results t ON t.round_id=r.round_id WHERE r.newapi_user_id=$1 AND t.presentation_completed_at IS NULL)`, user).Scan(&blocked); err != nil {
				return err
			}
			if blocked {
				return ErrScratchIncomplete
			}
		}
		c, err := scanCommitment(tx.QueryRow(ctx, `SELECT `+commitmentColumns+` FROM games.fairness_commitments WHERE commitment_id=$1 AND newapi_user_id=$2 AND game_slug=$3 AND state='AVAILABLE' FOR UPDATE`, commitmentID, user, slug))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCommitmentInvalid
		}
		if err != nil {
			return err
		}
		p, err := preference(ctx, tx, user, slug)
		if err != nil {
			return err
		}
		if !compatible(c, runtime, p) {
			return ErrCommitmentInvalid
		}
		var before, seq, version int64
		err = tx.QueryRow(ctx, `SELECT balance_units,ledger_seq,version FROM economy.wallet_balances WHERE newapi_user_id=$1 AND asset_type='AVAILABLE_CHIPS' FOR UPDATE`, user).Scan(&before, &seq, &version)
		if errors.Is(err, pgx.ErrNoRows) {
			return platform.ErrWalletNotFound
		}
		if err != nil {
			return err
		}
		if before < n.TotalStake {
			return platform.ErrInsufficientBalance
		}
		// Reject every wager whose possible result could overflow, before decrypting
		// or resolving it. Outcome-dependent rejection would create a free probe.
		if before-n.TotalStake > math.MaxInt64-n.MaxPayout || seq > math.MaxInt64-2 || version > math.MaxInt64-2 {
			return platform.ErrBalanceOverflow
		}
		var keyVersion string
		var gcmNonce, encrypted []byte
		err = tx.QueryRow(ctx, `SELECT key_version,gcm_nonce,ciphertext FROM games.fairness_commitments WHERE commitment_id=$1`, c.ID).Scan(&keyVersion, &gcmNonce, &encrypted)
		if err != nil {
			return err
		}
		seed, err := s.openSeed(user, slug, c, keyVersion, gcmNonce, encrypted)
		if err != nil {
			return err
		}
		fair, err := fairnessInput(slug, c)
		if err != nil {
			return err
		}
		round = GameRound{ID: c.ReservedRoundID, Game: slug, State: "SETTLED", RecoveryState: "NORMAL", Input: n.Input, StakeUnits: n.TotalStake, BalanceBeforeUnits: before, ConfigVersion: c.ConfigVersion, ConfigHash: c.ConfigHash, PolicyVersion: c.PolicyVersion, PolicyHash: c.PolicyHash, Algorithm: c.Algorithm, Ruleset: c.Ruleset, Stream: c.Stream, Nonce: c.Nonce, CommitmentID: c.ID}
		if _, err = tx.Exec(ctx, `UPDATE games.fairness_commitments SET state='CONSUMED' WHERE commitment_id=$1`, c.ID); err != nil {
			return err
		}
		wager, err := platform.ApplyInTx(ctx, tx, platform.Mutation{UserID: user, Asset: platform.AvailableChips, DeltaUnits: -n.TotalStake, BizType: "GAME_WAGER", BizID: round.ID, EntryType: "GAME_WAGER", IdempotencyKey: "game:wager:" + round.ID})
		if err != nil {
			return err
		}
		round.WagerTransactionID = wager.TransactionID
		payout, err := derive(&round, seed, fair, runtime.Config)
		if err != nil {
			return err
		}
		round.PayoutUnits = payout
		round.NetUnits = round.PayoutUnits - round.StakeUnits
		round.BalanceAfterUnits = before + round.NetUnits
		round.Outcome = BreakEven
		if round.NetUnits > 0 {
			round.Outcome = Win
		} else if round.NetUnits < 0 {
			round.Outcome = Loss
		}
		if round.PayoutUnits > 0 {
			settlement, err := platform.ApplyInTx(ctx, tx, platform.Mutation{UserID: user, Asset: platform.AvailableChips, DeltaUnits: round.PayoutUnits, BizType: "GAME_SETTLEMENT", BizID: round.ID, EntryType: "GAME_PAYOUT", IdempotencyKey: "game:settlement:" + round.ID})
			if err != nil {
				return err
			}
			round.SettlementTransactionID = settlement.TransactionID
		} else {
			// A zero-payout settlement has an immutable business transaction, no zero
			// ledger leg (the existing wallet contract forbids zero delta entries).
			round.SettlementTransactionID, err = newUUID()
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO economy.asset_transactions(transaction_id,biz_type,biz_id,newapi_user_id,operation_type,status,request_hash) VALUES($1,'GAME_SETTLEMENT',$2,$3,'GAME_PAYOUT','CONFIRMED',$4)`, round.SettlementTransactionID, round.ID, user, requestHash[:])
			if err != nil {
				return err
			}
		}
		configHash, _ := hex.DecodeString(round.ConfigHash)
		policyHash, _ := hex.DecodeString(round.PolicyHash)
		typed, _ := json.Marshal(n.Input)
		err = tx.QueryRow(ctx, `INSERT INTO games.game_rounds(round_id,newapi_user_id,game_slug,commitment_id,idempotency_key_hash,request_hash,typed_input,implementation_key,game_config_version_id,game_config_hash,wager_policy_version_id,wager_policy_hash,fairness_stream_version,ruleset_version,algorithm_version,nonce,state,total_stake_units,total_payout_units,net_change_units,common_result,balance_before_units,balance_after_units,wager_transaction_id,settlement_transaction_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,'SETTLED',$17,$18,$19,$20,$21,$22,$23,$24) RETURNING created_at,settled_at`, round.ID, user, slug, c.ID, keyHash[:], requestHash[:], typed, runtime.Entry.Implementation, c.ConfigVersion, configHash, c.PolicyVersion, policyHash, c.Stream, c.Ruleset, c.Algorithm, c.Nonce, round.StakeUnits, round.PayoutUnits, round.NetUnits, round.Outcome, before, round.BalanceAfterUnits, round.WagerTransactionID, round.SettlementTransactionID).Scan(&round.CreatedAt, &round.SettledAt)
		if err != nil {
			return err
		}
		if err = persistTyped(ctx, tx, round); err != nil {
			return err
		}
		if round.NetUnits != 0 {
			event, direction, amount := "GAME_ISSUANCE", "ISSUE", round.NetUnits
			if amount < 0 {
				event, direction, amount = "GAME_BURN", "BURN", -amount
			}
			if _, err = tx.Exec(ctx, `INSERT INTO games.supply_events(round_id,event_type,direction,amount_units) VALUES($1,$2,$3,$4)`, round.ID, event, direction, amount); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `UPDATE games.fairness_commitments SET state='REVEALED',revealed_server_seed=$2 WHERE commitment_id=$1`, c.ID, seed)
		return err
	})
	if err != nil {
		return GameRound{}, err
	}
	return round, nil
}
func derive(round *GameRound, seed []byte, fair FairRound, c Config) (int64, error) {
	n, err := normalizeCreateForRuleset(round.Game, round.Input, c.binding.RulesetVersion)
	if err != nil {
		return 0, err
	}
	switch round.Game {
	case "slot":
		return deriveSlot(round, seed, fair, c, n.TotalStake)
	case "dice":
		result, err := RollDice(seed, fair, c, DiceSide(round.Input.Choice))
		if err != nil {
			return 0, err
		}
		round.Dice = &result
		return n.Base * result.Reward.PayoutMultiplier, nil
	case "scratch":
		result, err := ScratchCard(seed, fair, c)
		if err != nil {
			return 0, err
		}
		round.Scratch = &result
		return n.Base * result.Reward.PayoutMultiplier, nil
	case "summon":
		result, err := Summon(seed, fair, c, SummonMode(round.Input.Mode))
		if err != nil {
			return 0, err
		}
		round.Summon = &result
		return n.Base * result.Reward.PayoutMultiplier, nil
	default:
		return 0, ErrUnavailable
	}
}
func persistTyped(ctx context.Context, tx pgx.Tx, r GameRound) error {
	switch r.Game {
	case "slot":
		return persistSlot(ctx, tx, r)
	case "dice":
		d := r.Dice
		_, err := tx.Exec(ctx, `INSERT INTO games.dice_results(round_id,die_1,die_2,die_3,choice) VALUES($1,$2,$3,$4,$5)`, r.ID, d.Dice[0], d.Dice[1], d.Dice[2], d.Choice)
		return err
	case "scratch":
		d := r.Scratch
		if _, err := tx.Exec(ctx, `INSERT INTO games.scratch_results(round_id,prize_tier,payout_multiplier) VALUES($1,$2,$3)`, r.ID, d.Tier, d.Reward.PayoutMultiplier); err != nil {
			return err
		}
		for i, cell := range d.Cells {
			if _, err := tx.Exec(ctx, `INSERT INTO games.scratch_cells(round_id,cell_index,symbol,is_matching_symbol) VALUES($1,$2,$3,$4)`, r.ID, i, cell.Symbol, cell.Matching); err != nil {
				return err
			}
		}
		return nil
	case "summon":
		d := r.Summon
		if _, err := tx.Exec(ctx, `INSERT INTO games.summon_results(round_id,mode,highest_tier) VALUES($1,$2,$3)`, r.ID, d.Mode, d.HighestTier); err != nil {
			return err
		}
		for _, draw := range d.Draws {
			if _, err := tx.Exec(ctx, `INSERT INTO games.summon_draws(round_id,draw_index,prize_tier,payout_multiplier) VALUES($1,$2,$3,$4)`, r.ID, draw.Index, draw.Tier, draw.Multiplier); err != nil {
				return err
			}
		}
		return nil
	}
	return ErrUnavailable
}
func (s *Service) RevealComplete(ctx context.Context, user int64, id, actionID string) (GameRound, error) {
	var r GameRound
	if _, err := uuidBytes(id); err != nil {
		return r, ErrInvalidInput
	}
	if _, err := uuidBytes(actionID); err != nil {
		return r, ErrInvalidInput
	}
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := lockUserGame(ctx, tx, user, "scratch"); err != nil {
			return err
		}
		var err error
		r, err = s.readRound(ctx, tx, user, id)
		if err != nil {
			return err
		}
		if r.Game != "scratch" {
			return ErrInvalidInput
		}
		var previousRound string
		err = tx.QueryRow(ctx, `SELECT round_id::text FROM games.round_actions WHERE action_id=$1`, actionID).Scan(&previousRound)
		if err == nil {
			if previousRound != id {
				return platform.ErrIdempotencyConflict
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO games.round_actions(action_id,round_id,newapi_user_id,action_sequence,action_type) SELECT $1,$2,$3,coalesce(max(action_sequence),0)+1,'SCRATCH_REVEAL_COMPLETE' FROM games.round_actions WHERE round_id=$2`, actionID, id, user); err != nil {
			return err
		}
		if r.PresentationCompletedAt == nil {
			if _, err = tx.Exec(ctx, `UPDATE games.scratch_results SET presentation_completed_at=now() WHERE round_id=$1 AND presentation_completed_at IS NULL`, id); err != nil {
				return err
			}
		}
		r, err = s.readRound(ctx, tx, user, id)
		return err
	})
	return r, err
}
