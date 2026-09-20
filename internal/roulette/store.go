package roulette

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/games/fairness"
	"github.com/jackc/pgx/v5"
)

type snapshot struct {
	Pressure      *PressureState `json:"pressure,omitempty"`
	Contributions []Contribution `json:"contributions"`
	Devil         *DevilState    `json:"devil,omitempty"`
}
type award struct {
	User   int64 `json:"user_id,string"`
	Seat   int   `json:"seat"`
	Amount int64 `json:"amount_units,string"`
}
type outcome struct {
	Reason   string  `json:"reason"`
	Eligible []int   `json:"eligible_seats"`
	Awards   []award `json:"awards"`
	Refunds  bool    `json:"refunds"`
}
type round struct {
	ID, Game, Title, State                            string
	Host                                              int64
	Target                                            int
	Stake, Escrow, Version, Sequence, SnapshotVersion int64
	Config                                            Config
	Policy                                            policy
	Stream                                            string
	Commitment                                        [32]byte
	SeedBox, StateBox                                 sealed
	Seed                                              []byte
	Snapshot                                          snapshot
	Outcome                                           *outcome
	Created                                           time.Time
	Started, Ended, Deadline, GameDeadline            *time.Time
}
type participant struct {
	User                       int64
	Seat                       *int
	Cycle, ContributionVersion int64
	Ready, Active              bool
	Name                       string
}

func (s *Service) load(ctx context.Context, tx pgx.Tx, id string, lock bool) (*round, error) {
	r := &round{ID: id}
	var hash, policyHash, seedHash, outcomeJSON []byte
	var configID, policyID, rules, algorithm string
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	e := tx.QueryRow(ctx, `SELECT game_slug,title_snapshot,host_user_id,target_players,stake_units,config_version_id::text,config_hash,policy_version_id::text,policy_hash,ruleset_version,algorithm_version,stream_version,server_seed_hash,seed_key_version,seed_nonce,seed_ciphertext,snapshot_key_version,snapshot_nonce,snapshot_ciphertext,snapshot_version,version,action_sequence,state,escrow_units,coalesce(void_outcome,outcome),created_at,started_at,ended_at,deadline,game_deadline FROM roulette.rounds WHERE round_id=$1`+suffix, id).Scan(&r.Game, &r.Title, &r.Host, &r.Target, &r.Stake, &configID, &hash, &policyID, &policyHash, &rules, &algorithm, &r.Stream, &seedHash, &r.SeedBox.Key, &r.SeedBox.Nonce, &r.SeedBox.Ciphertext, &r.StateBox.Key, &r.StateBox.Nonce, &r.StateBox.Ciphertext, &r.SnapshotVersion, &r.Version, &r.Sequence, &r.State, &r.Escrow, &outcomeJSON, &r.Created, &r.Started, &r.Ended, &r.Deadline, &r.GameDeadline)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	copy(r.Commitment[:], seedHash)
	r.Config = Config{ID: configID, Game: r.Game, Ruleset: rules, Algorithm: algorithm}
	copy(r.Config.Hash[:], hash)
	r.Policy = policy{ID: policyID}
	copy(r.Policy.Hash[:], policyHash)
	if len(outcomeJSON) > 0 {
		if json.Unmarshal(outcomeJSON, &r.Outcome) != nil {
			return r, ErrUnavailable
		}
	}
	c, e := ReadConfig(ctx, tx, r.Game, configID)
	if e != nil {
		return r, e
	}
	p, e := readPolicy(ctx, tx, policyID)
	if e != nil {
		return r, e
	}
	if !bytes.Equal(c.Hash[:], hash) || !bytes.Equal(p.Hash[:], policyHash) || rules != c.Ruleset || algorithm != c.Algorithm || r.Stream != fairness.StreamVersion {
		return r, ErrUnavailable
	}
	r.Config = c
	r.Policy = p
	idBytes, e := uuidBytes(id)
	if e != nil {
		return r, e
	}
	r.Seed, e = s.keys.open(r.SeedBox, aad("MOMIAO-ROULETTE-SEED-AAD-V1", idBytes, c.Hash, 0))
	if e != nil {
		return r, e
	}
	h := sha256.Sum256(r.Seed)
	if len(r.Seed) != 32 || h != r.Commitment {
		return r, ErrUnavailable
	}
	data, e := s.keys.open(r.StateBox, aad("MOMIAO-ROULETTE-STATE-AAD-V1", idBytes, c.Hash, r.SnapshotVersion))
	if e != nil {
		return r, e
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&r.Snapshot) != nil || decoder.Decode(&struct{}{}) != io.EOF || !r.validHiddenState() {
		return r, ErrUnavailable
	}
	return r, nil
}

// GCM authenticates bytes, not their meaning. Validate before any array index,
// slice or rule projection; never normalize a damaged forensic snapshot.
func (r *round) validHiddenState() bool {
	seatOK := func(seat int) bool { return seat >= 0 && seat < r.Target }
	seatsOK := func(seats []int) bool {
		seen := map[int]bool{}
		for _, seat := range seats {
			if !seatOK(seat) || seen[seat] {
				return false
			}
			seen[seat] = true
		}
		return true
	}
	if r.Game == "devil-roulette" && r.Target != 2 || r.Game == "pressure-roulette" && (r.Target < 3 || r.Target > 6) || !IsGame(r.Game) {
		return false
	}
	contributions := []int{}
	for _, c := range r.Snapshot.Contributions {
		if c.Version == 0 || !validSeed(c.Seed) {
			return false
		}
		contributions = append(contributions, int(c.Seat))
	}
	if !seatsOK(contributions) {
		return false
	}
	if r.Started == nil {
		return r.Sequence == 0 && r.Snapshot.Devil == nil && r.Snapshot.Pressure == nil
	}
	if len(contributions) != r.Target {
		return false
	}
	if r.Game == "devil-roulette" {
		d := r.Snapshot.Devil
		if d == nil || r.Snapshot.Pressure != nil || !seatOK(d.Turn) || !seatOK(d.First) || len(d.Shells) < 5 || len(d.Shells) > 8 || d.Pointer < 0 || d.Pointer > len(d.Shells) || !d.Ended && d.Pointer == len(d.Shells) || !seatsOK(d.Eligible) || d.Ended != (len(d.Eligible) > 0) {
			return false
		}
		for seat := range 2 {
			if d.HP[seat] < 0 || d.HP[seat] > 4 || !d.Ended && (d.HP[seat] == 0 || d.Forfeited[seat]) || len(d.Items[seat]) > 4 || d.Known[seat] == nil {
				return false
			}
			for _, item := range d.Items[seat] {
				if !validItem(item) {
					return false
				}
			}
			for index, live := range d.Known[seat] {
				if index < d.Pointer || index >= len(d.Shells) || d.Shells[index] != live {
					return false
				}
			}
		}
		return true
	}
	p := r.Snapshot.Pressure
	if p == nil || r.Snapshot.Devil != nil || !seatOK(p.Turn) || p.Pointer < 0 || p.Pointer >= 6 || !seatsOK(p.Alive) || !seatsOK(p.Eligible) || !p.Ended && (len(p.Alive) < 2 || !slices.Contains(p.Alive, p.Turn)) || p.Ended != (p.Phase == "ENDED") {
		return false
	}
	if !slices.Contains([]string{"FIRE", "CHOICE", "VOTE", "ENDED"}, p.Phase) || !slices.Contains([]string{"", "FIRE", "CHOICE", "forcedPass"}, p.ResumeMode) || p.Votes == nil || p.Charge < 0 || p.Charge == math.MaxInt || p.Wave < 1 || p.Wave == math.MaxInt || p.PoolDuds < 1 || p.PoolDuds > 3 || len(p.Pool) > 9 || p.Bullets < 0 || p.Bullets > 6 || p.GunDuds < 0 || p.GunDuds > p.Bullets || p.Debt < 0 || p.Debt > 2 {
		return false
	}
	for _, seat := range []*int{p.DebtOwner, p.Aggressor, p.Holder, p.RipInitiator, p.RipTarget} {
		if seat != nil && !seatOK(*seat) {
			return false
		}
	}
	if (p.DebtOwner == nil) != (p.Debt == 0) || (p.RipTarget == nil) != (p.RipInitiator == nil) || (p.Aggressor == nil) != (p.Holder == nil) {
		return false
	}
	for _, tier := range p.TimeoutTiers {
		if tier < 0 || tier > 2 {
			return false
		}
	}
	for _, bullet := range p.Pool {
		if bullet != pressureLive && bullet != pressureDud {
			return false
		}
	}
	bullets, duds := 0, 0
	for index, bullet := range p.Chambers {
		if bullet < pressureEmpty || bullet > pressureDud || !slices.Contains([]string{"UNKNOWN", "EMPTY", "LIVE_SPENT", "DUD_SPENT"}, p.Revealed[index]) || p.Revealed[index] != "UNKNOWN" && bullet != pressureEmpty {
			return false
		}
		if bullet != pressureEmpty {
			bullets++
		}
		if bullet == pressureDud {
			duds++
		}
	}
	if bullets != p.Bullets || duds != p.GunDuds || p.Phase == "VOTE" && (p.Bullets != 0 || len(p.Pool) != 0 || p.ResumeMode == "") {
		return false
	}
	for seat := range p.Votes {
		if !slices.Contains(p.Alive, seat) {
			return false
		}
	}
	for _, seat := range p.Eligible {
		if !p.Ended || !slices.Contains(p.Alive, seat) {
			return false
		}
	}
	return true
}
func participants(ctx context.Context, tx pgx.Tx, id string) ([]participant, error) {
	rows, e := tx.Query(ctx, `SELECT newapi_user_id,seat_no,ready_cycle,contribution_version,ready,active,display_name_snapshot FROM roulette.participants WHERE round_id=$1 ORDER BY newapi_user_id`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []participant{}
	for rows.Next() {
		var p participant
		if e = rows.Scan(&p.User, &p.Seat, &p.Cycle, &p.ContributionVersion, &p.Ready, &p.Active, &p.Name); e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Service) save(ctx context.Context, tx pgx.Tx, r *round) error {
	if !r.validHiddenState() {
		return ErrUnavailable
	}
	r.Version++
	r.SnapshotVersion++
	data, e := json.Marshal(r.Snapshot)
	if e != nil {
		return e
	}
	id, e := uuidBytes(r.ID)
	if e != nil {
		return e
	}
	box, e := s.keys.seal(data, aad("MOMIAO-ROULETTE-STATE-AAD-V1", id, r.Config.Hash, r.SnapshotVersion))
	if e != nil {
		return e
	}
	var revealed []byte
	if r.State == "FINISHED" || r.State == "CANCELLED" {
		revealed = r.Seed
	}
	var result []byte
	if r.Outcome != nil {
		result, e = json.Marshal(r.Outcome)
		if e != nil {
			return e
		}
	}
	_, e = tx.Exec(ctx, `UPDATE roulette.rounds SET snapshot_key_version=$2,snapshot_nonce=$3,snapshot_ciphertext=$4,snapshot_version=$5,version=$6,action_sequence=$7,state=$8,escrow_units=$9,outcome=$10,started_at=$11,ended_at=$12,deadline=$13,game_deadline=$14,revealed_seed=$15 WHERE round_id=$1`, r.ID, box.Key, box.Nonce, box.Ciphertext, r.SnapshotVersion, r.Version, r.Sequence, r.State, r.Escrow, result, r.Started, r.Ended, r.Deadline, r.GameDeadline, revealed)
	return e
}
func markReview(ctx context.Context, tx pgx.Tx, r *round) error {
	if r == nil || r.State == "FINISHED" || r.State == "CANCELLED" || r.State == "NEEDS_REVIEW" {
		return nil
	}
	_, e := tx.Exec(ctx, `UPDATE roulette.rounds SET state='NEEDS_REVIEW',version=version+1,deadline=NULL WHERE round_id=$1`, r.ID)
	r.State = "NEEDS_REVIEW"
	r.Version++
	r.Deadline = nil
	return e
}
func (r *round) receipt() Receipt { return Receipt{r.ID, r.Version, r.Sequence, r.State} }
func (r *round) stream(domain string) (*fairness.Stream, error) {
	id, e := uuidBytes(r.ID)
	if e != nil {
		return nil, e
	}
	client, e := effectiveClientSeed(id, r.Game, r.Config.Hash, r.Snapshot.Contributions)
	if e != nil {
		return nil, e
	}
	return fairness.NewStream(r.Seed, fairness.Round{ID: id, Game: r.Game, ClientSeed: client, StreamVersion: r.Stream, AlgorithmVersion: r.Config.Algorithm, ConfigVersion: r.Config.ID, ConfigHash: r.Config.Hash, ServerSeedHash: r.Commitment}, domain)
}
func validKey(key string) bool {
	if len(key) < 16 || len(key) > 128 {
		return false
	}
	for _, r := range key {
		if r < 33 || r > 126 {
			return false
		}
	}
	return true
}
func lockCommand(ctx context.Context, tx pgx.Tx, user int64, key string) error {
	h := sha256.Sum256([]byte(key))
	_, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "roulette/command/"+strconv.FormatInt(user, 10)+"/"+hex.EncodeToString(h[:]))
	return e
}
func commandHash(id string, in any) [32]byte {
	data, _ := json.Marshal(struct {
		ID    string
		Input any
	}{id, in})
	return sha256.Sum256(data)
}
func readReceipt(ctx context.Context, tx pgx.Tx, user int64, key string, semantic *[32]byte) (Receipt, bool, error) {
	keyHash := sha256.Sum256([]byte(key))
	var saved, raw []byte
	e := tx.QueryRow(ctx, `SELECT semantic_hash,receipt FROM roulette.commands WHERE actor_user_id=$1 AND key_hash=$2`, user, keyHash[:]).Scan(&saved, &raw)
	if errors.Is(e, pgx.ErrNoRows) {
		return Receipt{}, false, nil
	}
	if e != nil {
		return Receipt{}, false, e
	}
	if semantic != nil && !bytes.Equal(saved, semantic[:]) {
		return Receipt{}, true, ErrIdempotencyConflict
	}
	var out Receipt
	e = json.Unmarshal(raw, &out)
	return out, true, e
}
func writeReceipt(ctx context.Context, tx pgx.Tx, user int64, key string, semantic [32]byte, r Receipt) error {
	h := sha256.Sum256([]byte(key))
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO roulette.commands(actor_user_id,key_hash,semantic_hash,round_id,receipt) VALUES($1,$2,$3,$4,$5)`, user, h[:], semantic[:], r.RoundID, b)
	return e
}
func dbNow(ctx context.Context, tx pgx.Tx) (time.Time, error) {
	var t time.Time
	e := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&t)
	return t, e
}
