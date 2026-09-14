package games

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"

	bj "github.com/cy4268/momiao/internal/games/blackjack"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

func databaseNow(ctx context.Context, tx pgx.Tx) (time.Time, error) {
	var now time.Time
	err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now)
	return now.UTC(), err
}
func lockedChips(ctx context.Context, tx pgx.Tx, user int64) (int64, int64, int64, error) {
	var balance, sequence, version int64
	err := tx.QueryRow(ctx, `SELECT balance_units,ledger_seq,version FROM economy.wallet_balances WHERE newapi_user_id=$1 AND asset_type='AVAILABLE_CHIPS' FOR UPDATE`, user).Scan(&balance, &sequence, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		err = platform.ErrWalletNotFound
	}
	return balance, sequence, version, err
}
func (s *Service) createBlackjack(ctx context.Context, user int64, key, commitmentID string, input CreateInput) (GameRound, error) {
	var r GameRound
	n, err := normalizeCreate("blackjack", input)
	if err != nil {
		return r, err
	}
	if user <= 0 || !validRequestKey(key) {
		return r, ErrInvalidInput
	}
	if _, err = uuidBytes(commitmentID); err != nil {
		return r, ErrInvalidInput
	}
	keyhash := sha256.Sum256([]byte(key))
	requesthash := semanticHash(user, "blackjack", n.Input)
	err = s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if e := lockUserGame(ctx, tx, user, "blackjack"); e != nil {
			return e
		}
		var existing string
		var hash []byte
		e := tx.QueryRow(ctx, `SELECT round_id::text,request_hash FROM games.game_rounds WHERE newapi_user_id=$1 AND game_slug='blackjack' AND idempotency_key_hash=$2`, user, keyhash[:]).Scan(&existing, &hash)
		if e == nil {
			if !bytes.Equal(hash, requesthash[:]) {
				return platform.ErrIdempotencyConflict
			}
			r, e = s.readRound(ctx, tx, user, existing)
			return e
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if err := platform.RequireNoMaintenance(ctx, tx, "CHALDEA_USER_WRITES", "DIRECT_PLAY_NEW_ROUNDS"); err != nil {
			if errors.Is(err, platform.ErrMaintenanceActive) { return ErrMaintenance }; return err
		}
		var active bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM games.game_rounds WHERE newapi_user_id=$1 AND game_slug='blackjack' AND state='PLAYER_TURN')`, user).Scan(&active); e != nil {
			return e
		}
		if active {
			return ErrActiveRound
		}
		runtime, e := resolveRuntime(ctx, tx, "blackjack", true)
		if e != nil {
			return e
		}
		if runtime.Entry.State == "MAINTENANCE" {
			return ErrMaintenance
		}
		if runtime.Entry.State != "PLAY" {
			return ErrUnavailable
		}
		c, e := scanCommitment(tx.QueryRow(ctx, `SELECT `+commitmentColumns+` FROM games.fairness_commitments WHERE commitment_id=$1 AND newapi_user_id=$2 AND game_slug='blackjack' AND state='AVAILABLE' FOR UPDATE`, commitmentID, user))
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrCommitmentInvalid
		}
		if e != nil {
			return e
		}
		pref, e := preference(ctx, tx, user, "blackjack")
		if e != nil {
			return e
		}
		if !compatible(c, runtime, pref) {
			return ErrCommitmentInvalid
		}
		before, seq, version, e := lockedChips(ctx, tx, user)
		if e != nil {
			return e
		}
		if before < n.TotalStake {
			return platform.ErrInsufficientBalance
		}
		// Reserve representable future headroom before any hidden card is examined.
		if before-n.TotalStake > math.MaxInt64-n.MaxPayout || seq > math.MaxInt64-9 || version > math.MaxInt64-9 {
			return platform.ErrBalanceOverflow
		}
		var keyVersion string
		var nonce, encrypted []byte
		if e = tx.QueryRow(ctx, `SELECT key_version,gcm_nonce,ciphertext FROM games.fairness_commitments WHERE commitment_id=$1`, c.ID).Scan(&keyVersion, &nonce, &encrypted); e != nil {
			return e
		}
		seed, e := s.openSeed(user, "blackjack", c, keyVersion, nonce, encrypted)
		if e != nil {
			return e
		}
		fair, e := fairnessInput("blackjack", c)
		if e != nil {
			return e
		}
		if e = runtime.Config.checkRound(fair, "blackjack"); e != nil {
			return e
		}
		shoe, e := blackjackShoe(seed, fair)
		if e != nil {
			return e
		}
		handID, e := newUUID()
		if e != nil {
			return e
		}
		now, e := databaseNow(ctx, tx)
		if e != nil {
			return e
		}
		state, e := bj.New(shoe, n.TotalStake, handID, now)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE games.fairness_commitments SET state='CONSUMED' WHERE commitment_id=$1`, c.ID); e != nil {
			return e
		}
		wager, e := platform.ApplyInTx(ctx, tx, platform.Mutation{UserID: user, Asset: platform.AvailableChips, DeltaUnits: -n.TotalStake, BizType: "GAME_WAGER", BizID: c.ReservedRoundID, EntryType: "GAME_WAGER", IdempotencyKey: "game:wager:" + c.ReservedRoundID})
		if e != nil {
			return e
		}
		r = GameRound{ID: c.ReservedRoundID, Game: "blackjack", State: "PLAYER_TURN", RecoveryState: "NORMAL", Input: n.Input, StakeUnits: n.TotalStake, NetUnits: -n.TotalStake, BalanceBeforeUnits: before, BalanceAfterUnits: before - n.TotalStake, WagerTransactionID: wager.TransactionID, ConfigVersion: c.ConfigVersion, ConfigHash: c.ConfigHash, PolicyVersion: c.PolicyVersion, PolicyHash: c.PolicyHash, Algorithm: c.Algorithm, Ruleset: c.Ruleset, Stream: c.Stream, Nonce: c.Nonce, CommitmentID: c.ID, CreatedAt: now}
		typed, _ := json.Marshal(n.Input)
		_, e = tx.Exec(ctx, `INSERT INTO games.game_rounds(round_id,newapi_user_id,game_slug,commitment_id,idempotency_key_hash,request_hash,typed_input,implementation_key,game_config_version_id,game_config_hash,wager_policy_version_id,wager_policy_hash,fairness_stream_version,ruleset_version,algorithm_version,nonce,state,total_stake_units,total_payout_units,net_change_units,common_result,balance_before_units,balance_after_units,wager_transaction_id,settlement_transaction_id,created_at,settled_at) VALUES($1,$2,'blackjack',$3,$4,$5,$6,'direct.blackjack.v1',$7,$8,$9,$10,$11,$12,$13,$14,'PLAYER_TURN',$15,0,$16,NULL,$17,$18,$19,NULL,$20,NULL)`, r.ID, user, c.ID, keyhash[:], requesthash[:], typed, c.ConfigVersion, decodeHash(c.ConfigHash), c.PolicyVersion, decodeHash(c.PolicyHash), c.Stream, c.Ruleset, c.Algorithm, c.Nonce, n.TotalStake, -n.TotalStake, before, r.BalanceAfterUnits, wager.TransactionID, now)
		if e != nil {
			return e
		}
		if e = persistBlackjackState(ctx, tx, r.ID, state); e != nil {
			return e
		}
		if e = finishBlackjackTransition(ctx, tx, user, &r, state, seed, r.BalanceAfterUnits, now); e != nil {
			return e
		}
		if state.Phase == bj.PlayerTurn {
			for kind, due := range map[string]time.Time{"BLACKJACK_AUTO_RESOLVE": state.AutoResolveAt, "GAME_ROUND_RECOVERY": now} {
				id, e := newUUID()
				if e != nil {
					return e
				}
				if _, e = tx.Exec(ctx, `INSERT INTO games.round_jobs(job_id,round_id,job_type,run_at) VALUES($1,$2,$3,$4)`, id, r.ID, kind, due); e != nil {
					return e
				}
			}
		}
		return nil
	})
	if err != nil {
		return GameRound{}, err
	}
	return r, nil
}

// Apply an already accepted transition. All added stakes are applied by the
// caller before this function. The terminal payout/reveal/net supply is atomic.
func finishBlackjackTransition(ctx context.Context, tx pgx.Tx, user int64, r *GameRound, state bj.State, seed []byte, available int64, now time.Time) error {
	r.StakeUnits = state.TotalStakeUnits
	r.PayoutUnits = state.TotalPayoutUnits
	r.NetUnits = r.PayoutUnits - r.StakeUnits
	r.State = string(state.Phase)
	r.BalanceAfterUnits = available
	if state.Phase == bj.Settled {
		r.Outcome = Outcome(state.Class)
		r.SettledAt = &now
		if state.TotalPayoutUnits > 0 {
			payout, err := platform.ApplyInTx(ctx, tx, platform.Mutation{UserID: user, Asset: platform.AvailableChips, DeltaUnits: state.TotalPayoutUnits, BizType: "GAME_SETTLEMENT", BizID: r.ID, EntryType: "GAME_PAYOUT", IdempotencyKey: "game:settlement:" + r.ID})
			if err != nil {
				return err
			}
			r.SettlementTransactionID = payout.TransactionID
			r.BalanceAfterUnits = available + state.TotalPayoutUnits
		} else {
			id, err := newUUID()
			if err != nil {
				return err
			}
			r.SettlementTransactionID = id
			hash := semanticHash(user, "blackjack", r.Input)
			if _, err = tx.Exec(ctx, `INSERT INTO economy.asset_transactions(transaction_id,biz_type,biz_id,newapi_user_id,operation_type,status,request_hash) VALUES($1,'GAME_SETTLEMENT',$2,$3,'GAME_PAYOUT','CONFIRMED',$4)`, id, r.ID, user, hash[:]); err != nil {
				return err
			}
		}
		if r.NetUnits != 0 {
			event, direction, amount := "GAME_ISSUANCE", "ISSUE", r.NetUnits
			if amount < 0 {
				event, direction, amount = "GAME_BURN", "BURN", -amount
			}
			if _, err := tx.Exec(ctx, `INSERT INTO games.supply_events(round_id,event_type,direction,amount_units) VALUES($1,$2,$3,$4)`, r.ID, event, direction, amount); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE games.fairness_commitments SET state='REVEALED',revealed_server_seed=$2 WHERE commitment_id=$1`, r.CommitmentID, seed); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `UPDATE games.game_rounds SET state=$2,total_stake_units=$3,total_payout_units=$4,net_change_units=$5,common_result=NULLIF($6,''),balance_after_units=$7,settlement_transaction_id=NULLIF($8,'')::uuid,settled_at=$9 WHERE round_id=$1`, r.ID, r.State, r.StakeUnits, r.PayoutUnits, r.NetUnits, r.Outcome, r.BalanceAfterUnits, r.SettlementTransactionID, r.SettledAt)
	if err != nil {
		return err
	}
	projection := bj.PublicView(state, r.BalanceAfterUnits)
	r.Blackjack = &projection
	if state.Phase == bj.Settled {
		_, err = tx.Exec(ctx, `UPDATE games.round_jobs SET status='DONE',last_error=NULL,updated_at=clock_timestamp() WHERE round_id=$1 AND status='PENDING'`, r.ID)
	} else {
		_, err = tx.Exec(ctx, `UPDATE games.round_jobs SET run_at=$2,updated_at=clock_timestamp() WHERE round_id=$1 AND job_type='BLACKJACK_AUTO_RESOLVE' AND status='PENDING'`, r.ID, state.AutoResolveAt)
	}
	return err
}

func blackjackActionHash(user int64, round string, input BlackjackActionInput) [32]byte {
	raw, _ := json.Marshal(input)
	return sha256.Sum256(append([]byte("blackjack.action.v1\x00"+strconv.FormatInt(user, 10)+"\x00"+round+"\x00"), raw...))
}
func normalizeBlackjackAction(input BlackjackActionInput) (bj.Action, error) {
	a := bj.Action{ID: input.ActionID, HandID: input.HandID, Type: bj.ActionType(input.ActionType)}
	for _, id := range []string{a.ID, a.HandID} {
		if _, err := uuidBytes(id); err != nil {
			return a, ErrInvalidInput
		}
	}
	v, err := strconv.ParseInt(input.ExpectedVersion, 10, 64)
	if err != nil || v < 1 || strconv.FormatInt(v, 10) != input.ExpectedVersion {
		return a, ErrInvalidInput
	}
	a.ExpectedVersion = v
	if a.Type != bj.Hit && a.Type != bj.Stand && a.Type != bj.Double && a.Type != bj.Split {
		return a, ErrInvalidInput
	}
	return a, nil
}
func (s *Service) BlackjackAction(ctx context.Context, user int64, id string, input BlackjackActionInput) (GameRound, error) {
	var r GameRound
	if user <= 0 {
		return r, ErrInvalidInput
	}
	if _, err := uuidBytes(id); err != nil {
		return r, ErrInvalidInput
	}
	action, err := normalizeBlackjackAction(input)
	if err != nil {
		return r, err
	}
	hash := blackjackActionHash(user, id, input)
	var refused error
	err = s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if e := lockUserGame(ctx, tx, user, "blackjack"); e != nil {
			return e
		}
		// Return the action's original safe response before checking current version.
		var originalRound string
		var originalHash, original []byte
		e := tx.QueryRow(ctx, `SELECT round_id::text,request_hash,original_response FROM games.round_actions WHERE action_id=$1 AND newapi_user_id=$2`, action.ID, user).Scan(&originalRound, &originalHash, &original)
		if e == nil {
			if originalRound != id || !bytes.Equal(hash[:], originalHash) {
				return platform.ErrIdempotencyConflict
			}
			if json.Unmarshal(original, &r) != nil {
				return ErrUnavailable
			}
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		r, e = s.readRound(ctx, tx, user, id)
		if e != nil {
			return e
		}
		if r.Game != "blackjack" {
			return ErrInvalidInput
		}
		if r.RecoveryState != "NORMAL" {
			refused = bj.ErrNeedsReview
			return nil
		}
		state, shoe, seed, e := s.recoverBlackjack(ctx, tx, user, r)
		if e != nil {
			return e
		}
		available, seq, version, e := lockedChips(ctx, tx, user)
		if e != nil {
			return e
		}
		if available > math.MaxInt64-state.InitialWagerUnits*16 || seq > math.MaxInt64-2 || version > math.MaxInt64-2 {
			return platform.ErrBalanceOverflow
		}
		if action.Type == bj.Split {
			action.NewHandID, e = newUUID()
			if e != nil {
				return e
			}
		}
		now, e := databaseNow(ctx, tx)
		if e != nil {
			return e
		}
		transition, e := bj.Apply(state, shoe, action, available, now)
		if e != nil {
			return e
		}
		stakeTx := ""
		if transition.AdditionalStakeUnits > 0 {
			stake, e := platform.ApplyInTx(ctx, tx, platform.Mutation{UserID: user, Asset: platform.AvailableChips, DeltaUnits: -transition.AdditionalStakeUnits, BizType: "GAME_ADDITIONAL_WAGER", BizID: id + ":" + action.ID, EntryType: "GAME_ADDITIONAL_WAGER", IdempotencyKey: "game:action:" + action.ID})
			if e != nil {
				return e
			}
			stakeTx = stake.TransactionID
			available -= transition.AdditionalStakeUnits
		}
		if e = persistBlackjackState(ctx, tx, id, transition.State); e != nil {
			return e
		}
		if e = finishBlackjackTransition(ctx, tx, user, &r, transition.State, seed, available, now); e != nil {
			return e
		}
		original, e = json.Marshal(r)
		if e != nil {
			return e
		}
		record := transition.State.Actions[len(transition.State.Actions)-1]
		_, e = tx.Exec(ctx, `INSERT INTO games.round_actions(action_id,round_id,newapi_user_id,action_sequence,action_type,additional_stake_units,system_action,created_at,expected_round_version,hand_id,new_hand_id,request_hash,original_response,stake_transaction_id) VALUES($1,$2,$3,$4,$5,$6,FALSE,$7,$8,$9,NULLIF($10,'')::uuid,$11,$12,NULLIF($13,'')::uuid)`, action.ID, id, user, record.Sequence, action.Type, record.AdditionalStakeUnits, record.At, action.ExpectedVersion, action.HandID, action.NewHandID, hash[:], original, stakeTx)
		return e
	})
	if err != nil {
		return GameRound{}, err
	}
	if refused != nil {
		return GameRound{}, refused
	}
	return r, nil
}

// Read-only reconciliation for an uncertain action response. It takes the same
// per-user game lock, so a definitive absent answer cannot overtake an action.
func (s *Service) FindBlackjackAction(ctx context.Context, user int64, round, id string) (*GameRound, error) {
	for _, v := range []string{round, id} {
		if _, err := uuidBytes(v); err != nil {
			return nil, ErrInvalidInput
		}
	}
	var result *GameRound
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := lockUserGame(ctx, tx, user, "blackjack"); err != nil {
			return err
		}
		var raw []byte
		err := tx.QueryRow(ctx, `SELECT original_response FROM games.round_actions WHERE round_id=$1 AND action_id=$2 AND newapi_user_id=$3`, round, id, user).Scan(&raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		var r GameRound
		if json.Unmarshal(raw, &r) != nil {
			return ErrUnavailable
		}
		result = &r
		return nil
	})
	return result, err
}
