package roulette

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/games/fairness"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Service struct {
	store    *platform.Store
	keys     Keyring
	observer platform.NativeQuotaObserver
}

func NewService(store *platform.Store, keys Keyring) (*Service, error) {
	return NewServiceWithEconomy(store, keys, nil)
}
func NewServiceWithEconomy(store *platform.Store, keys Keyring, observer platform.NativeQuotaObserver) (*Service, error) {
	if store == nil || keys.Active == "" {
		return nil, ErrUnavailable
	}
	if _, ok := keys.Keys[keys.Active]; !ok {
		return nil, ErrUnavailable
	}
	k := Keyring{Active: keys.Active, Keys: map[string][32]byte{}}
	for name, key := range keys.Keys {
		k.Keys[name] = key
	}
	return &Service{store: store, keys: k, observer: observer}, nil
}
func uniqueError(e error) error {
	var pg *pgconn.PgError
	if errors.As(e, &pg) && pg.Code == "23505" && (pg.ConstraintName == "roulette_one_active_seat" || pg.ConstraintName == "roulette_room_seat") {
		return ErrAlreadySeated
	}
	return e
}
func (s *Service) Create(ctx context.Context, user int64, in CreateRequest) (Receipt, error) {
	amount, e := platform.ParseAmount(in.Stake)
	if e != nil || user <= 0 || !validKey(in.Key) || !IsGame(in.Game) || (in.Game == "devil-roulette" && in.Players != 2) || (in.Game == "pressure-roulette" && (in.Players < 3 || in.Players > 6)) || amount <= 0 || amount > math.MaxInt64/int64(in.Players) {
		return Receipt{}, ErrInvalidInput
	}
	semantic := commandHash("create", struct {
		Game    string
		Stake   int64
		Players int
	}{in.Game, amount, in.Players})
	var receipt Receipt
	e = s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if e := lockCommand(ctx, tx, user, in.Key); e != nil {
			return e
		}
		v, exists, e := readReceipt(ctx, tx, user, in.Key, &semantic)
		if e != nil {
			return e
		}
		if exists {
			receipt = v
			return nil
		}
		entry, c, p, e := resolve(ctx, tx, in.Game, true)
		if e != nil {
			return e
		}
		if entry.State != "PLAY" {
			return ErrUnavailable
		}
		if amount < p.Minimum || amount%p.Step != 0 {
			return ErrInvalidInput
		}
		economic, e := platform.ActiveEconomicPolicyInTx(ctx, tx)
		if e != nil {
			return e
		}
		if economic.Version != "" {
			if _, e = platform.ReadUnifiedAssetsInTx(ctx, tx, s.observer, user); e != nil {
				return e
			}
		}
		id, e := newID()
		if e != nil {
			return e
		}
		idBytes, _ := uuidBytes(id)
		seed := make([]byte, 32)
		if _, e = rand.Read(seed); e != nil {
			return e
		}
		hash := sha256.Sum256(seed)
		sb, e := s.keys.seal(seed, aad("MOMIAO-ROULETTE-SEED-AAD-V1", idBytes, c.Hash, 0))
		if e != nil {
			return e
		}
		snapshotData, _ := json.Marshal(snapshot{Contributions: []Contribution{}})
		state, e := s.keys.seal(snapshotData, aad("MOMIAO-ROULETTE-STATE-AAD-V1", idBytes, c.Hash, 1))
		if e != nil {
			return e
		}
		now, e := dbNow(ctx, tx)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO roulette.rounds(round_id,game_slug,title_snapshot,host_user_id,target_players,stake_units,config_version_id,config_hash,policy_version_id,policy_hash,ruleset_version,algorithm_version,stream_version,server_seed_hash,seed_key_version,seed_nonce,seed_ciphertext,snapshot_key_version,snapshot_nonce,snapshot_ciphertext,snapshot_version,state,created_at,deadline,economic_policy_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,1,'WAITING',$21,$22,NULLIF($23,''))`, id, in.Game, entry.Title, user, in.Players, amount, c.ID, c.Hash[:], p.ID, p.Hash[:], c.Ruleset, c.Algorithm, fairness.StreamVersion, hash[:], sb.Key, sb.Nonce, sb.Ciphertext, state.Key, state.Nonce, state.Ciphertext, now, now.Add(300*time.Second), economic.Version)
		if e != nil {
			return e
		}
		if e = join(ctx, tx, id, user, 0); e != nil {
			return e
		}
		receipt = Receipt{id, 1, 0, "WAITING"}
		return writeReceipt(ctx, tx, user, in.Key, semantic, receipt)
	})
	return receipt, uniqueError(e)
}
func join(ctx context.Context, tx pgx.Tx, id string, user int64, seat int) error {
	// Names are immutable round display snapshots, not account IDs.
	_, e := tx.Exec(ctx, `INSERT INTO roulette.participants(round_id,newapi_user_id,seat_no,display_name_snapshot) VALUES($1,$2,$3,coalesce((SELECT display_name FROM identity.master_profiles WHERE newapi_user_id=$2),'旅人')) ON CONFLICT(round_id,newapi_user_id) DO UPDATE SET seat_no=EXCLUDED.seat_no,active=true WHERE NOT roulette.participants.active`, id, user, seat)
	return e
}
func (s *Service) FindReceipt(ctx context.Context, user int64, key string) (Receipt, error) {
	if user <= 0 || !validKey(key) {
		return Receipt{}, ErrInvalidInput
	}
	var result Receipt
	e := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		v, ok, e := readReceipt(ctx, tx, user, key, nil)
		result = v
		if e == nil && !ok {
			return ErrNotFound
		}
		return e
	})
	return result, e
}
func (s *Service) Command(ctx context.Context, user int64, id string, in Command) (Receipt, error) {
	if user <= 0 || !ValidRoundID(id) || !validKey(in.Key) || in.ExpectedVersion < 1 || !ValidAction(in.Action, false) || (in.Action.Kind == "READY") != (in.Ready != nil) {
		return Receipt{}, ErrInvalidInput
	}
	if in.Ready != nil && !validSeed(in.Ready.ClientSeed) {
		return Receipt{}, ErrInvalidInput
	}
	semantic := commandHash(id, in)
	var result Receipt
	var replyErr error
	e := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if e := lockCommand(ctx, tx, user, in.Key); e != nil {
			return e
		}
		v, exists, e := readReceipt(ctx, tx, user, in.Key, &semantic)
		if e != nil {
			return e
		}
		if exists {
			result = v
			return nil
		}
		// Gate before room locks; a saved receipt remains readable during maintenance.
		if in.Action.Kind == "READY" {
			var game string
			if e = tx.QueryRow(ctx, `SELECT game_slug FROM roulette.rounds WHERE round_id=$1`, id).Scan(&game); e != nil {
				if errors.Is(e, pgx.ErrNoRows) {
					return ErrNotFound
				}
				return e
			}
			if e = readyGate(ctx, tx, game); e != nil {
				return e
			}
		}
		r, e := s.load(ctx, tx, id, true)
		if e != nil {
			if r != nil && errors.Is(e, ErrUnavailable) {
				if e = markReview(ctx, tx, r); e != nil {
					return e
				}
				replyErr = ErrUnavailable
				return nil
			}
			return e
		}
		players, e := participants(ctx, tx, id)
		if e != nil {
			return e
		}
		index := slices.IndexFunc(players, func(p participant) bool { return p.User == user && p.Seat != nil && p.Active })
		if index < 0 && in.Action.Kind != "JOIN" {
			return ErrNotFound
		}
		now, e := dbNow(ctx, tx)
		if e != nil {
			return e
		}
		advanced, e := s.due(ctx, tx, r, players, now)
		if e != nil {
			return e
		}
		if advanced {
			replyErr = ErrVersionConflict
			return nil
		}
		if in.ExpectedVersion != r.Version {
			return ErrVersionConflict
		}
		if r.State == "WAITING" {
			e = s.waiting(ctx, tx, r, players, index, user, in, now)
		} else if r.State == "PLAYING" && index >= 0 {
			e = s.play(ctx, tx, r, players, players[index].Seat, &user, in.Action, now)
		} else {
			e = ErrActionInvalid
		}
		if e != nil {
			return e
		}
		if e = s.save(ctx, tx, r); e != nil {
			return e
		}
		result = r.receipt()
		return writeReceipt(ctx, tx, user, in.Key, semantic, result)
	})
	if e != nil {
		return Receipt{}, uniqueError(e)
	}
	if replyErr != nil {
		return Receipt{}, replyErr
	}
	if result.State == "SETTLING" {
		// The accepted action, outcome and original receipt are already committed.
		// A failed wallet attempt leaves SETTLING for the worker, not PLAYING.
		_ = s.finishPending(ctx, id)
	}
	return result, nil
}
func (s *Service) waiting(ctx context.Context, tx pgx.Tx, r *round, players []participant, index int, user int64, in Command, now time.Time) error {
	switch in.Action.Kind {
	case "JOIN":
		if index >= 0 {
			return ErrActionInvalid
		}
		occupied := map[int]bool{}
		for _, p := range players {
			if p.Seat != nil {
				occupied[*p.Seat] = true
			}
		}
		for seat := 0; seat < r.Target; seat++ {
			if !occupied[seat] {
				return join(ctx, tx, r.ID, user, seat)
			}
		}
		return ErrActionInvalid
	case "CANCEL":
		if r.Host != user {
			return ErrActionInvalid
		}
		return s.cancel(ctx, tx, r, players, now, "HOST_CANCELLED")
	case "LEAVE", "UNREADY":
		p := &players[index]
		if in.Action.Kind == "UNREADY" && !p.Ready {
			return ErrActionInvalid
		}
		// A departing host closes the waiting room; no stranded host privileges.
		if in.Action.Kind == "LEAVE" && r.Host == user {
			return s.cancel(ctx, tx, r, players, now, "HOST_LEFT")
		}
		if p.Ready {
			if e := lockWallets(ctx, tx, []participant{*p}); e != nil {
				return e
			}
			if e := fund(ctx, tx, r, *p, "REFUND", r.Stake); e != nil {
				return e
			}
			r.Escrow -= r.Stake
		}
		r.Snapshot.Contributions = slices.DeleteFunc(r.Snapshot.Contributions, func(c Contribution) bool { return int(c.Seat) == *p.Seat })
		_, e := tx.Exec(ctx, `UPDATE roulette.participants SET ready=false,active=CASE WHEN $3 THEN false ELSE active END,seat_no=CASE WHEN $3 THEN NULL ELSE seat_no END WHERE round_id=$1 AND newapi_user_id=$2`, r.ID, user, in.Action.Kind == "LEAVE")
		return e
	case "READY":
		p := &players[index]
		if p.Ready {
			return ErrActionInvalid
		}
		confirm := in.Ready
		if confirm.ConfigHash != hex.EncodeToString(r.Config.Hash[:]) || confirm.PolicyHash != hex.EncodeToString(r.Policy.Hash[:]) || confirm.ServerSeedHash != hex.EncodeToString(r.Commitment[:]) || confirm.StakeUnits != strconv.FormatInt(r.Stake, 10) {
			return ErrVersionConflict
		}
		if p.Cycle == math.MaxInt64 || p.ContributionVersion == math.MaxInt64 {
			return ErrUnavailable
		}
		if e := lockWallets(ctx, tx, players); e != nil {
			return e
		}
		var balance int64
		if e := tx.QueryRow(ctx, `SELECT balance_units FROM economy.wallet_balances WHERE newapi_user_id=$1 AND asset_type='AVAILABLE_CHIPS'`, user).Scan(&balance); e != nil {
			return e
		}
		if balance < r.Stake {
			return platform.ErrInsufficientBalance
		}
		if balance-r.Stake > math.MaxInt64-r.Stake*int64(r.Target) {
			return platform.ErrBalanceOverflow
		}
		if r.Economy.Version != "" {
			if _, e := platform.ReadUnifiedAssetsInTx(ctx, tx, s.observer, user); e != nil {
				return e
			}
		}
		p.Cycle++
		p.ContributionVersion++
		p.Ready = true
		_, e := tx.Exec(ctx, `UPDATE roulette.participants SET ready_cycle=$3,contribution_version=$4,ready=true WHERE round_id=$1 AND newapi_user_id=$2`, r.ID, user, p.Cycle, p.ContributionVersion)
		if e != nil {
			return e
		}
		if e = fund(ctx, tx, r, *p, "ESCROW", r.Stake); e != nil {
			return e
		}
		r.Escrow += r.Stake
		r.Snapshot.Contributions = append(r.Snapshot.Contributions, Contribution{uint16(*p.Seat), uint64(p.ContributionVersion), confirm.ClientSeed})
		count := 0
		for _, other := range players {
			if other.Ready && other.Active {
				count++
			}
		}
		if count == r.Target {
			stream, e := r.stream("roulette/init/v1")
			if e != nil {
				return e
			}
			if e = r.initRule(stream); e != nil {
				return e
			}
			r.State = "PLAYING"
			r.Started = &now
			limit := now.Add(1800 * time.Second)
			r.GameDeadline = &limit
			r.setDeadline(now)
		}
		return nil
	default:
		return ErrActionInvalid
	}
}
func (r *round) setDeadline(now time.Time) {
	seconds := 62
	if p := r.Snapshot.Pressure; p != nil {
		seconds = []int{45, 25, 10}[p.TimeoutTiers[p.Turn]]
		if p.Phase == "VOTE" {
			seconds = 35
		}
	}
	next := now.Add(time.Duration(seconds) * time.Second)
	if r.GameDeadline != nil && r.GameDeadline.Before(next) {
		next = *r.GameDeadline
	}
	r.Deadline = &next
}
func (s *Service) play(ctx context.Context, tx pgx.Tx, r *round, players []participant, seat *int, user *int64, a Action, now time.Time) error {
	if seat == nil || r.Sequence == math.MaxInt64 {
		return ErrActionInvalid
	}
	sequence := r.Sequence + 1
	domain := "roulette/action/v1/" + strconv.FormatInt(sequence, 10)
	rng, e := r.stream(domain)
	if e != nil {
		return e
	}
	oldVote := r.inVote()
	if e = r.applyRule(*seat, a, rng); e != nil {
		return e
	}
	if !r.validHiddenState() {
		return ErrUnavailable
	}
	ended, eligible, reason, eventText := r.ruleResult(a)
	r.Sequence = sequence
	event := PublicEvent{sequence, now, seat, a.Kind, eventText}
	input, _ := json.Marshal(a)
	public, _ := json.Marshal(event)
	hash := ruleHash(r.ruleState())
	_, e = tx.Exec(ctx, `INSERT INTO roulette.actions(round_id,sequence,actor_user_id,actor_seat,input,occurred_at,domain,state_hash,public_event) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, r.ID, sequence, user, seat, input, now, domain, hash[:], public)
	if e != nil {
		return e
	}
	if ended {
		r.Outcome = makeOutcome(r, players, eligible, reason, false)
		if r.Outcome == nil {
			r.State = "NEEDS_REVIEW"
			r.Deadline = nil
			return nil
		}
		r.State = "SETTLING"
		r.Deadline = nil
		return nil
	}
	if !(oldVote && r.inVote()) {
		r.setDeadline(now)
	}
	return nil
}

// Keep error details in the caller's local logs, never in HTTP rule responses.
func (s *Service) String() string { return fmt.Sprintf("roulette.Service(%s)", s.keys.Active) }

func (r *round) initRule(rng io.Reader) error {
	switch r.Game {
	case "devil-roulette":
		state, e := newDevil(rng)
		if e == nil {
			r.Snapshot.Devil = &state
		}
		return e
	case "pressure-roulette":
		seats := make([]int, r.Target)
		for i := range seats {
			seats[i] = i
		}
		state, e := newPressure(rng, seats)
		if e == nil {
			r.Snapshot.Pressure = &state
		}
		return e
	}
	return ErrUnavailable
}
func (r *round) applyRule(seat int, a Action, rng io.Reader) error {
	switch r.Game {
	case "devil-roulette":
		if r.Snapshot.Devil != nil {
			state, e := applyDevil(*r.Snapshot.Devil, seat, a, rng)
			if e == nil {
				r.Snapshot.Devil = &state
			}
			return e
		}
	case "pressure-roulette":
		if r.Snapshot.Pressure != nil {
			state, e := applyPressure(*r.Snapshot.Pressure, seat, a, rng)
			if e == nil {
				r.Snapshot.Pressure = &state
			}
			return e
		}
	}
	return ErrUnavailable
}
func (r *round) ruleState() any {
	if r.Game == "devil-roulette" {
		return r.Snapshot.Devil
	}
	return r.Snapshot.Pressure
}
func (r *round) ruleTurn() int {
	if r.Game == "devil-roulette" && r.Snapshot.Devil != nil {
		return r.Snapshot.Devil.Turn
	}
	if r.Snapshot.Pressure != nil {
		return r.Snapshot.Pressure.Turn
	}
	return -1
}
func (r *round) inVote() bool {
	return r.Snapshot.Pressure != nil && r.Snapshot.Pressure.Phase == "VOTE"
}
func (r *round) ruleResult(a Action) (bool, []int, string, string) {
	if p := r.Snapshot.Pressure; p != nil {
		return p.Ended, p.Eligible, p.Reason, p.Event
	}
	d := r.Snapshot.Devil
	reason := "LAST_SURVIVOR"
	if a.Kind == "GAME_LIMIT" {
		reason = "GAME_LIMIT"
	}
	if a.Kind == "SURRENDER" {
		reason = "FORFEIT"
	}
	return d.Ended, d.Eligible, reason, d.Event
}
