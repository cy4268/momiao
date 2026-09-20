package roulette

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"reflect"
	"slices"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/historyaccess"
	"github.com/jackc/pgx/v5"
)

var ErrHistoryCursorStale = errors.New("HISTORY_CURSOR_STALE")

type Proof struct {
	RoundID         string         `json:"round_id"`
	Status          string         `json:"status"`
	Valid           bool           `json:"valid"`
	CommitmentValid bool           `json:"commitment_valid"`
	ConfigValid     bool           `json:"config_valid"`
	ActionsValid    bool           `json:"actions_valid"`
	SettlementValid bool           `json:"settlement_valid"`
	ServerSeed      string         `json:"server_seed,omitempty"`
	Contributions   []Contribution `json:"contributions,omitempty"`
	Binding         Binding        `json:"binding"`
	ActionCount     int64          `json:"action_count,string"`
	HistoryPath     string         `json:"history_path"`
}
type RecordedAction struct {
	Sequence  int64       `json:"sequence,string"`
	Seat      int         `json:"seat"`
	User      *int64      `json:"-"`
	Input     Action      `json:"input"`
	At        time.Time   `json:"at"`
	Domain    string      `json:"domain"`
	StateHash string      `json:"state_hash"`
	Event     PublicEvent `json:"event"`
}
type ReplayFrame struct {
	Sequence int64    `json:"sequence,string"`
	View     RoomView `json:"view"`
}
type HistoryDetail struct {
	ID                    string           `json:"id"`
	Game                  string           `json:"game"`
	State                 string           `json:"state"`
	Binding               Binding          `json:"binding"`
	StakeUnits            string           `json:"stake_units"`
	PayoutUnits           string           `json:"payout_units"`
	NetUnits              string           `json:"net_units"`
	Reason                string           `json:"reason"`
	Created               time.Time        `json:"created_at"`
	Ended                 *time.Time       `json:"ended_at"`
	Snapshot              json.RawMessage  `json:"snapshot"`
	View                  RoomView         `json:"view"`
	Actions               []PublicEvent    `json:"actions"`
	Commands              []RecordedAction `json:"commands"`
	Frames                []ReplayFrame    `json:"frames"`
	NextCursor            *string          `json:"next_cursor"`
	TransactionIDs        []string         `json:"transaction_ids"`
	TransactionsTruncated bool             `json:"transactions_truncated"`
}
type historyCursor struct {
	ID                   string
	User, Version, After int64
}

func historyOwner(ctx context.Context, tx pgx.Tx, user int64, id string) error {
	var owned bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM roulette.funding WHERE round_id=$1 AND newapi_user_id=$2 AND kind='ESCROW')`, id, user).Scan(&owned); e != nil {
		return e
	}
	if !owned {
		return historyaccess.ErrNotFound
	}
	return nil
}
func actionBatch(ctx context.Context, tx pgx.Tx, id string, after int64, limit int) ([]RecordedAction, error) {
	rows, e := tx.Query(ctx, `SELECT sequence,actor_seat,actor_user_id,input,occurred_at,domain,encode(state_hash,'hex'),public_event FROM roulette.actions WHERE round_id=$1 AND sequence>$2 ORDER BY sequence LIMIT $3`, id, after, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []RecordedAction{}
	for rows.Next() {
		var a RecordedAction
		var input, event []byte
		if e = rows.Scan(&a.Sequence, &a.Seat, &a.User, &input, &a.At, &a.Domain, &a.StateHash, &event); e != nil {
			return nil, e
		}
		if json.Unmarshal(input, &a.Input) != nil || json.Unmarshal(event, &a.Event) != nil {
			return nil, historyaccess.ErrUnavailable
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func terminal(r *round) bool { return r.State == "FINISHED" || r.State == "CANCELLED" }

// replay reads at most 100 recorded commands at a time, and retains only the requested frames.
// It shares the compiled rule implementation, never the saved hidden state as an RNG shortcut.
func replay(ctx context.Context, tx pgx.Tx, r *round, ps []participant, user, after int64, limit int) ([]ReplayFrame, bool, error) {
	frames := []ReplayFrame{}
	if r.Started == nil {
		return frames, r.Sequence == 0 && r.State == "CANCELLED", nil
	}
	// Derive the public limit from the frozen rules, not a mutable stored deadline.
	if r.GameDeadline == nil || !r.GameDeadline.Equal(r.Started.Add(1800*time.Second)) {
		return frames, false, nil
	}
	rr := *r
	rr.Snapshot = snapshot{Contributions: r.Snapshot.Contributions}
	rr.State = "PLAYING"
	rr.Sequence = 0
	if len(rr.Snapshot.Contributions) != r.Target {
		return frames, false, nil
	}
	for _, c := range rr.Snapshot.Contributions {
		i := slices.IndexFunc(ps, func(p participant) bool {
			return p.Ready && p.Seat != nil && *p.Seat == int(c.Seat) && uint64(p.ContributionVersion) == c.Version
		})
		if i < 0 {
			return frames, false, nil
		}
	}
	rng, e := rr.stream("roulette/init/v1")
	if e != nil {
		return frames, false, nil
	}
	if e = rr.initRule(rng); e != nil {
		return frames, false, e
	}
	lastAction := Action{}
	rr.setDeadline(*r.Started)
	lastAt := *r.Started
	for rr.Sequence < r.Sequence {
		batch, e := actionBatch(ctx, tx, r.ID, rr.Sequence, 100)
		if e != nil {
			return frames, false, e
		}
		if len(batch) == 0 {
			return frames, false, nil
		}
		for _, a := range batch {
			if a.Sequence != rr.Sequence+1 || a.Sequence > r.Sequence || a.Domain != "roulette/action/v1/"+strconv.FormatInt(a.Sequence, 10) || !ValidAction(a.Input, true) || a.At.Before(lastAt) {
				return frames, false, nil
			}
			if a.User == nil {
				if a.Seat != rr.ruleTurn() || a.Input.Kind != "TIMEOUT" && a.Input.Kind != "GAME_LIMIT" || rr.Deadline == nil || a.At.Before(*rr.Deadline) {
					return frames, false, nil
				}
				capDue := rr.GameDeadline != nil && !a.At.Before(*rr.GameDeadline)
				if (a.Input.Kind == "GAME_LIMIT") != capDue {
					return frames, false, nil
				}
			} else {
				if a.Input.Kind == "TIMEOUT" || a.Input.Kind == "GAME_LIMIT" || rr.Deadline == nil || !a.At.Before(*rr.Deadline) {
					return frames, false, nil
				}
				if !slices.ContainsFunc(ps, func(p participant) bool { return p.User == *a.User && p.Ready && p.Seat != nil && *p.Seat == a.Seat }) {
					return frames, false, nil
				}
			}
			rng, e = rr.stream(a.Domain)
			if e != nil {
				return frames, false, e
			}
			oldVote := rr.inVote()
			if e = rr.applyRule(a.Seat, a.Input, rng); e != nil {
				return frames, false, nil
			}
			ended, _, _, eventText := rr.ruleResult(a.Input)
			hash := ruleHash(rr.ruleState())
			// SQL and JSON may attach different Locations to the same instant.
			expectedEvent := PublicEvent{a.Sequence, a.At.UTC(), &a.Seat, a.Input.Kind, eventText}
			recordedEvent := a.Event
			recordedEvent.At = recordedEvent.At.UTC()
			if a.StateHash != hex.EncodeToString(hash[:]) || !reflect.DeepEqual(recordedEvent, expectedEvent) {
				return frames, false, nil
			}
			rr.Sequence = a.Sequence
			if !(oldVote && rr.inVote()) {
				rr.setDeadline(a.At)
			}
			lastAt = a.At
			lastAction = a.Input
			if ended {
				rr.State = "FINISHED"
				rr.Deadline = nil
			}
			if a.Sequence > after && len(frames) < limit {
				frames = append(frames, ReplayFrame{a.Sequence, project(&rr, ps, user, a.At)})
			}
		}
	}
	if ruleHash(rr.ruleState()) != ruleHash(r.ruleState()) {
		return frames, false, nil
	}
	if r.Outcome == nil {
		return frames, false, nil
	}
	if r.Outcome.Reason != "SYSTEM_VOID" {
		ended, eligible, reason, _ := rr.ruleResult(lastAction)
		if !ended || reason != r.Outcome.Reason {
			return frames, false, nil
		}
		rr.Escrow = r.Stake * int64(r.Target)
		want := makeOutcome(&rr, ps, eligible, reason, false)
		if !reflect.DeepEqual(want, r.Outcome) {
			return frames, false, nil
		}
	}
	return frames, true, nil
}

// Funding rows are immutable, but neither a label nor aggregate conservation alone
// proves them: check every actual formal transaction, ledger, cycle and final award.
func verifyFunding(ctx context.Context, tx pgx.Tx, r *round, ps []participant) (bool, error) {
	rows, e := tx.Query(ctx, `SELECT f.newapi_user_id,f.ready_cycle,f.kind,f.amount_units,economy.history_transaction_read(f.newapi_user_id,f.transaction_id) FROM roulette.funding f WHERE f.round_id=$1 ORDER BY f.newapi_user_id,f.ready_cycle,CASE f.kind WHEN 'ESCROW' THEN 0 ELSE 1 END`, r.ID)
	if e != nil {
		return false, e
	}
	defer rows.Close()
	total := new(big.Int)
	paid := map[int64]int64{}
	var prevUser, prevCycle, held int64
	valid := true
	for rows.Next() {
		var user, cycle, amount int64
		var kind string
		var raw []byte
		if e = rows.Scan(&user, &cycle, &kind, &amount, &raw); e != nil {
			return false, e
		}
		if user != prevUser || cycle != prevCycle {
			if held != 0 && !slices.ContainsFunc(ps, func(p participant) bool { return p.User == prevUser && p.Ready && p.Cycle == prevCycle }) {
				valid = false
			}
			held = 0
			prevUser, prevCycle = user, cycle
		}
		var t struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(raw, &t) != nil || t.Status != "CONFIRMED" {
			valid = false
		}
		n := big.NewInt(amount)
		if kind == "ESCROW" {
			if held != 0 || amount != r.Stake {
				valid = false
			}
			held += amount
			total.Add(total, n)
		} else {
			total.Sub(total, n)
			if kind == "PAYOUT" {
				if paid[user] != 0 {
					valid = false
				}
				paid[user] = amount
			} else {
				if held != amount {
					valid = false
				}
				held -= amount
			}
		}
	}
	if e = rows.Err(); e != nil {
		return false, e
	}
	if held != 0 && !slices.ContainsFunc(ps, func(p participant) bool { return p.User == prevUser && p.Ready && p.Cycle == prevCycle }) {
		valid = false
	}
	if total.Cmp(big.NewInt(r.Escrow)) != 0 {
		return false, nil
	}
	if terminal(r) {
		if r.Escrow != 0 || r.Outcome == nil {
			return false, nil
		}
		if r.Outcome.Refunds {
			if len(paid) != 0 {
				return false, nil
			}
		} else {
			if len(paid) != len(r.Outcome.Awards) {
				return false, nil
			}
			for _, a := range r.Outcome.Awards {
				if paid[a.User] != a.Amount {
					return false, nil
				}
			}
		}
	}
	return valid, nil
}

func (s *Service) HistoryVerify(ctx context.Context, access historyaccess.Access, id string) (Proof, error) {
	out := Proof{RoundID: id, Status: "UNREVEALED", HistoryPath: "/api/v1/history/roulette/" + id}
	_, user, e := access.Check(ctx)
	if e != nil {
		return out, e
	}
	if !ValidRoundID(id) {
		return out, historyaccess.ErrInvalid
	}
	e = s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if e := readOnly(ctx, tx); e != nil {
			return e
		}
		if e := historyOwner(ctx, tx, user, id); e != nil {
			return e
		}
		r, e := s.load(ctx, tx, id, false)
		if e != nil {
			return historyaccess.ErrUnavailable
		}
		out.Binding = binding(r.Config, r.Policy)
		out.ActionCount = r.Sequence
		if !terminal(r) {
			return nil
		}
		out.Status = "REVEALED"
		out.ServerSeed = hex.EncodeToString(r.Seed)
		out.Contributions = r.Snapshot.Contributions
		out.CommitmentValid = sha256.Sum256(r.Seed) == r.Commitment
		out.ConfigValid = true // load checked the archived config, policy, trusted artifact and stream.
		ps, e := participants(ctx, tx, id)
		if e != nil {
			return e
		}
		_, out.ActionsValid, e = replay(ctx, tx, r, ps, user, 0, 0)
		if e != nil {
			return e
		}
		out.SettlementValid, e = verifyFunding(ctx, tx, r, ps)
		if e != nil {
			return e
		}
		out.Valid = out.CommitmentValid && out.ConfigValid && out.ActionsValid && out.SettlementValid
		return nil
	})
	return out, e
}

func (s *Service) HistoryDetail(ctx context.Context, access historyaccess.Access, id string, q HistoryQuery) (HistoryDetail, error) {
	out := HistoryDetail{Actions: []PublicEvent{}, Commands: []RecordedAction{}, Frames: []ReplayFrame{}, TransactionIDs: []string{}}
	_, user, e := access.Check(ctx)
	if e != nil {
		return out, e
	}
	if q.Limit == 0 {
		q.Limit = 50
	}
	if !ValidRoundID(id) || q.Limit < 1 || q.Limit > 100 || len(q.Cursor) > 1024 {
		return out, historyaccess.ErrInvalid
	}
	var c historyCursor
	if q.Cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(q.Cursor)
		if e != nil || json.Unmarshal(raw, &c) != nil || c.ID != id || c.User != user || c.After < 0 {
			return out, historyaccess.ErrInvalid
		}
	}
	e = s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if e := readOnly(ctx, tx); e != nil {
			return e
		}
		if e := historyOwner(ctx, tx, user, id); e != nil {
			return e
		}
		r, e := s.load(ctx, tx, id, false)
		if e != nil {
			return historyaccess.ErrUnavailable
		}
		if q.Cursor != "" && c.Version != r.Version {
			return ErrHistoryCursorStale
		}
		ps, e := participants(ctx, tx, id)
		if e != nil {
			return e
		}
		now, e := dbNow(ctx, tx)
		if e != nil {
			return e
		}
		out.ID = id
		out.Game = r.Game
		out.State = r.State
		out.Binding = binding(r.Config, r.Policy)
		out.Created = r.Created
		out.Ended = r.Ended
		out.View = project(r, ps, user, now)
		if r.Outcome != nil {
			out.Reason = r.Outcome.Reason
		}
		e = tx.QueryRow(ctx, `SELECT games.history_record_snapshot($2,'ROULETTE_ROUND',$1),
   (coalesce(sum(amount_units::numeric) FILTER(WHERE kind='ESCROW'),0)-coalesce(sum(amount_units::numeric) FILTER(WHERE kind IN('REFUND','VOID_REFUND')),0))::text,
   coalesce(sum(amount_units::numeric) FILTER(WHERE kind='PAYOUT'),0)::text,
   coalesce(sum(CASE WHEN kind='ESCROW' THEN -amount_units::numeric ELSE amount_units::numeric END),0)::text
   FROM roulette.funding WHERE round_id=$1 AND newapi_user_id=$2`, id, user).Scan(&out.Snapshot, &out.StakeUnits, &out.PayoutUnits, &out.NetUnits)
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT transaction_id::text FROM roulette.funding WHERE round_id=$1 AND newapi_user_id=$2 ORDER BY created_at,funding_id LIMIT 101`, id, user)
		if e != nil {
			return e
		}
		for rows.Next() {
			var v string
			if e = rows.Scan(&v); e != nil {
				rows.Close()
				return e
			}
			out.TransactionIDs = append(out.TransactionIDs, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(out.TransactionIDs) > 100 {
			out.TransactionsTruncated = true
			out.TransactionIDs = out.TransactionIDs[:100]
		}
		batch, e := actionBatch(ctx, tx, id, c.After, q.Limit+1)
		if e != nil {
			return e
		}
		if len(batch) > q.Limit {
			batch = batch[:q.Limit]
			raw, _ := json.Marshal(historyCursor{id, user, r.Version, batch[len(batch)-1].Sequence})
			next := base64.RawURLEncoding.EncodeToString(raw)
			out.NextCursor = &next
		}
		for _, a := range batch {
			out.Actions = append(out.Actions, a.Event)
		}
		if terminal(r) {
			out.Commands = batch
			out.Frames, _, e = replay(ctx, tx, r, ps, user, c.After, q.Limit)
		}
		return e
	})
	return out, e
}
