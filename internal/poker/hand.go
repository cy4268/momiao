package poker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/poker/engine"
	"github.com/cy4268/momiao/internal/poker/fairness"
	"github.com/jackc/pgx/v5"
)

type handSetup struct {
	Config        engine.Config
	Seats         []engine.Seat
	Sessions      map[int]string
	Contributions []fairness.Contribution
}

func selectParticipants(t *tableRow, seats []seatRow) ([]seatRow, int, error) {
	var active, waiting []seatRow
	for _, seat := range seats {
		if seat.Stack == 0 || seat.Leave || seat.SitOutNext || seat.TimeoutSitOut || !seat.Connected {
			continue
		}
		if seat.State == "ACTIVE" {
			active = append(active, seat)
		} else if seat.State == "WAITING_BIG_BLIND" {
			waiting = append(waiting, seat)
		}
	}
	if t.HandNo == 0 {
		// Before any hand there is no running-table blind rotation to join.
		active = append(active, waiting...)
		sort.Slice(active, func(i, j int) bool { return active[i].Seat < active[j].Seat })
		if len(active) < 2 {
			return nil, 0, ErrDenied
		}
		return active, 0, nil
	}
	if len(active) == 0 {
		if len(waiting) < 2 {
			return nil, 0, ErrDenied
		}
		// Reviewed restart boundary: with no incumbent ACTIVE seats, a ready
		// cohort resumes normal blinds. prepareHand also requires no live hand.
		active = append(active, waiting...)
		waiting = nil
	}
	button := active[0].Seat
	for _, seat := range active {
		if seat.Seat > t.Button {
			button = seat.Seat
			break
		}
	}
	// Only the next BB candidate can join established rotation; no extra entry BB.
	sb := button
	if len(active) > 1 {
		for _, seat := range active {
			if seat.Seat > button {
				sb = seat.Seat
				break
			}
		}
		if sb == button {
			sb = active[0].Seat
		}
	}
	candidates := append(append([]seatRow{}, active...), waiting...)
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Seat < candidates[j].Seat })
	bb := candidates[0].Seat
	for _, seat := range candidates {
		if seat.Seat > sb {
			bb = seat.Seat
			break
		}
	}
	for _, seat := range waiting {
		if seat.Seat == bb {
			active = append(active, seat)
			break
		}
	}
	sort.Slice(active, func(i, j int) bool { return active[i].Seat < active[j].Seat })
	if len(active) < 2 {
		return nil, 0, ErrDenied
	}
	return active, button, nil
}

func (s *Service) prepareHand(user int64, key string) tableMutation {
	return func(ctx context.Context, tx pgx.Tx, t *tableRow) (Receipt, error) {
		c := StartHandCommand{UserID: user, Key: key, TableID: t.ID}
		if user != 0 {
			if r, ok, err := findReceipt(ctx, tx, user, "poker.start.v1", key, c); err != nil || ok {
				return r, err
			}
			if user != t.Owner {
				return Receipt{}, ErrDenied
			}
		}
		if !t.AllowHands || t.State == "CLOSING" || t.State == "CLOSED" || t.State == "PAUSED" || t.State == "RECOVERING" || t.State == "IN_HAND" || (t.Intermission != nil && t.Intermission.After(t.Now)) {
			return Receipt{}, ErrDenied
		}
		var unresolved bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM poker.hands WHERE table_id=$1 AND state<>'SETTLED')", t.ID).Scan(&unresolved); err != nil {
			return Receipt{}, err
		}
		if unresolved {
			return Receipt{}, ErrDenied
		}
		if err := requireAdmission(ctx, tx, t, 0, ""); err != nil {
			return Receipt{}, err
		}
		if err := s.boundary(ctx, tx, t); err != nil {
			return Receipt{}, err
		}
		seats, err := loadSeats(ctx, tx, t.ID)
		if err != nil {
			return Receipt{}, err
		}
		seats, err = s.readySeats(ctx, tx, t, seats)
		if err != nil {
			return Receipt{}, err
		}
		participants, button, err := selectParticipants(t, seats)
		if err != nil {
			return Receipt{}, err
		}
		id := uuid()
		seed := make([]byte, 32)
		if _, err = io.ReadFull(rand.Reader, seed); err != nil {
			return Receipt{}, err
		}
		setup := handSetup{Sessions: map[int]string{}}
		for _, p := range participants {
			setup.Seats = append(setup.Seats, engine.Seat{SeatNo: p.Seat, PlayerID: decimal(p.User), Stack: p.Stack, ConsecutiveTimeouts: p.Timeouts})
			setup.Sessions[p.Seat] = p.Session
			setup.Contributions = append(setup.Contributions, fairness.Contribution{Seat: p.Seat, Version: p.ContributionVersion, Value: p.Contribution})
		}
		proof, err := fairness.Derive(t.ID, id, setup.Contributions, seed)
		if err != nil {
			return Receipt{}, err
		}
		var choice *engine.ButtonChoice
		if t.HandNo == 0 {
			eligible := make([]int, len(participants))
			for i, p := range participants {
				eligible[i] = p.Seat
			}
			picked, err := engine.SelectInitialButton(eligible, func(n int) (int, error) { v, err := fairness.Uniform(rand.Reader, uint64(n)); return int(v), err })
			if err != nil {
				return Receipt{}, err
			}
			button = picked.Seat
			choice = &picked
		}
		deck := make([]engine.Card, 52)
		for i, c := range proof.Deck {
			deck[i] = engine.Card(c)
		}
		setup.Config = engine.Config{HandID: id, ButtonSeat: button, SmallBlind: t.SmallBlind, BigBlind: t.BigBlind, Deck: deck, EvaluatorVersion: engine.EvaluatorVersion, ButtonSelection: choice}
		plain, _ := json.Marshal(setup)
		sealed, err := s.seal(id+":setup", plain)
		if err != nil {
			return Receipt{}, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO poker.hands(hand_id,table_id,hand_no,state,button_seat,runtime_epoch,setup_cipher) VALUES($1,$2,$3,'COMMITTED',$4,$5,$6)`, id, t.ID, t.HandNo+1, button, t.Epoch, sealed)
		if err != nil {
			return Receipt{}, err
		}
		seedCipher, err := s.seal(id+":server_seed", seed)
		if err != nil {
			return Receipt{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO poker.hand_fairness(hand_id,server_seed_hash,encrypted_server_seed,server_seed_key_version,effective_client_seed_hash,algorithm_version,deck_version,deal_sequence_version,deck_hash) VALUES($1,$2,$3,$4,$5,$6,$6,'poker-deal-v1',$7)`, id, proof.ServerSeedHash[:], seedCipher, s.opts.Keyring.Current, proof.EffectiveSeed[:], fairness.AlgorithmVersion, proof.DeckHash[:]); err != nil {
			return Receipt{}, err
		}
		for _, p := range participants {
			contribution, err := s.seal(id+":contribution:"+strconv.Itoa(p.Seat), []byte(p.Contribution))
			if err != nil {
				return Receipt{}, err
			}
			_, err = tx.Exec(ctx, `INSERT INTO poker.hand_participants(hand_id,seat_no,session_id,newapi_user_id,hand_start_stack_units,contribution_version,contribution_cipher) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, p.Seat, p.Session, p.User, p.Stack, p.ContributionVersion, contribution)
			if err != nil {
				return Receipt{}, err
			}
			if p.State == "WAITING_BIG_BLIND" {
				if _, err = tx.Exec(ctx, "UPDATE poker.seats SET state='ACTIVE' WHERE table_id=$1 AND seat_no=$2", t.ID, p.Seat); err != nil {
					return Receipt{}, err
				}
			}
		}
		if _, err = tx.Exec(ctx, "UPDATE poker.tables SET current_hand_id=$2,hand_no=hand_no+1,button_seat=$3,lifecycle_state='IN_HAND',intermission_until=NULL WHERE table_id=$1", t.ID, id, button); err != nil {
			return Receipt{}, err
		}
		r := Receipt{TableID: t.ID, HandID: id, Status: "COMMITTED", Version: t.Version + 1}
		t.beforeCommit = func(ctx context.Context, _ *Receipt) error {
			bounded, cancel := context.WithTimeout(ctx, controlIOTimeout)
			defer cancel()
			for _, p := range participants {
				ok, err := s.readySeat(bounded, tx, t, p)
				if err != nil {
					return err
				}
				if !ok {
					return ErrNoControl
				}
			}
			return nil
		}
		if user != 0 {
			err = saveReceipt(ctx, tx, user, "poker.start.v1", key, c, r)
		}
		return r, err
	}
}

func (s *Service) StartHand(ctx context.Context, c StartHandCommand) (Receipt, error) {
	if !validIdentityKey(c.UserID, c.Key) {
		return Receipt{}, ErrInvalid
	}
	r, err := s.mutate(ctx, c.TableID, s.prepareHand(c.UserID, c.Key))
	if err != nil {
		return r, err
	}
	return s.mutate(ctx, c.TableID, s.dealHand(r.HandID))
}
func (s *Service) dealCommitted(ctx context.Context, tx pgx.Tx, t *tableRow) (Receipt, error) {
	return s.dealHand("")(ctx, tx, t)
}
func (s *Service) dealHand(expected string) tableMutation {
	return func(ctx context.Context, tx pgx.Tx, t *tableRow) (Receipt, error) {
		if expected != "" && expected != t.HandID {
			return Receipt{TableID: t.ID, HandID: expected, Status: "ALREADY_STARTED", Duplicate: true}, nil
		}
		if t.HandID == "" {
			return Receipt{}, ErrDenied
		}
		var state string
		var sealed []byte
		if err := tx.QueryRow(ctx, "SELECT state,setup_cipher FROM poker.hands WHERE hand_id=$1 FOR UPDATE", t.HandID).Scan(&state, &sealed); err != nil {
			return Receipt{}, err
		}
		if state != "COMMITTED" {
			return Receipt{TableID: t.ID, HandID: t.HandID, Status: state, Duplicate: true}, nil
		}
		plain, err := s.open(t.HandID+":setup", sealed)
		if err != nil {
			return Receipt{}, errors.Join(ErrCorruptSnapshot, err)
		}
		var setup handSetup
		if err = json.Unmarshal(plain, &setup); err != nil {
			return Receipt{}, errors.Join(ErrCorruptSnapshot, err)
		}
		e, err := engine.New(setup.Config, setup.Seats, t.Now, engine.Evaluate)
		if err != nil {
			return Receipt{}, errors.Join(ErrCorruptSnapshot, err)
		}
		if err = s.persistEngine(ctx, tx, t, e); err != nil {
			return Receipt{}, err
		}
		return Receipt{TableID: t.ID, HandID: t.HandID, Status: string(e.State().Street)}, nil
	}
}

func (s *Service) restoreEngine(ctx context.Context, tx pgx.Tx, handID string) (*engine.Engine, error) {
	return restoreHistoryEngine(ctx, tx, s.opts.Keyring, handID)
}

func restoreHistoryEngine(ctx context.Context, tx pgx.Tx, keys Keyring, handID string) (*engine.Engine, error) {
	var sealed, hash []byte
	var status string
	var version, sequence uint64
	if err := tx.QueryRow(ctx, "SELECT snapshot_cipher,snapshot_hash,state,hand_version,event_sequence FROM poker.hands WHERE hand_id=$1", handID).Scan(&sealed, &hash, &status, &version, &sequence); err != nil {
		return nil, err
	}
	plain, err := openCipher(keys, handID+":snapshot", sealed)
	if err != nil {
		return nil, errors.Join(ErrCorruptSnapshot, err)
	}
	actual := sha256.Sum256(plain)
	if !equal(actual[:], hash) {
		return nil, ErrCorruptSnapshot
	}
	e, err := engine.Restore(plain, engine.Evaluate)
	if err != nil {
		return nil, errors.Join(ErrCorruptSnapshot, err)
	}
	state := e.State()
	if state.Config.HandID != handID || string(state.Street) != status || state.Version != version || uint64(len(state.Events)) != sequence {
		return nil, ErrCorruptSnapshot
	}
	return e, nil
}

func (s *Service) persistEngine(ctx context.Context, tx pgx.Tx, t *tableRow, e *engine.Engine) error {
	state := e.State()
	plain, err := e.Snapshot()
	if err != nil {
		return err
	}
	sealed, err := s.seal(t.HandID+":snapshot", plain)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(plain)
	var oldSequence int
	var oldState string
	if err = tx.QueryRow(ctx, "SELECT event_sequence,state FROM poker.hands WHERE hand_id=$1 FOR UPDATE", t.HandID).Scan(&oldSequence, &oldState); err != nil {
		return err
	}
	if oldSequence > len(state.Events) {
		return ErrInvalid
	}
	for _, event := range state.Events[oldSequence:] {
		body, _ := json.Marshal(event)
		cipher, err := s.seal(fmt.Sprintf("%s:event:%d", t.HandID, event.Sequence), body)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO poker.actions(hand_id,event_sequence,hand_version,event_type,actor_seat,applied_delta_units,to_units,payload_cipher,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, t.HandID, event.Sequence, event.Version, event.Type, event.Seat, event.Delta, event.To, cipher, event.At)
		if err != nil {
			return err
		}
		if event.DeckIndex != nil && event.Card != nil {
			card, err := s.seal(fmt.Sprintf("%s:card:%d", t.HandID, *event.DeckIndex), []byte{byte(*event.Card)})
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, "INSERT INTO poker.dealt_cards(hand_id,deck_index,recipient_seat,visibility,card_cipher) VALUES($1,$2,$3,$4,$5)", t.HandID, *event.DeckIndex, event.Seat, event.Visibility, card); err != nil {
				return err
			}
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE poker.hands SET state=$2,snapshot_cipher=$3,snapshot_hash=$4,hand_version=$5,event_sequence=$6,runtime_epoch=$7 WHERE hand_id=$1", t.HandID, string(state.Street), sealed, hash[:], state.Version, len(state.Events), t.Epoch); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE poker.hand_fairness SET next_deck_index=$2 WHERE hand_id=$1", t.HandID, state.DeckCursor); err != nil {
		return err
	}
	for _, p := range state.Players {
		_, err = tx.Exec(ctx, "UPDATE poker.sessions sess SET current_stack_units=$3 FROM poker.hand_participants hp WHERE hp.hand_id=$1 AND hp.seat_no=$2 AND sess.session_id=hp.session_id", t.HandID, p.SeatNo, p.Stack)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "UPDATE poker.hand_participants SET street_committed_units=$3,total_committed_units=$4,folded=$5 WHERE hand_id=$1 AND seat_no=$2", t.HandID, p.SeatNo, p.StreetCommitted, p.TotalCommitted, p.Folded)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "UPDATE poker.seats SET timeout_count=$3,timeout_sit_out=$4,connected=$5,disconnected_since=$6 WHERE table_id=$1 AND seat_no=$2", t.ID, p.SeatNo, p.ConsecutiveTimeouts, p.TimeoutSitOut, p.DisconnectedSince == nil, p.DisconnectedSince)
		if err != nil {
			return err
		}
	}
	if state.Street != engine.Settled {
		return nil
	}
	if oldState == "SETTLED" {
		return nil
	}
	var paid, awarded, returned int64
	for _, event := range state.Events {
		switch event.Type {
		case "POST_SB", "POST_BB", "CALL", "BET", "RAISE", "ALL_IN":
			paid += event.Delta
		}
	}
	for _, p := range state.Pots {
		if _, err = tx.Exec(ctx, "INSERT INTO poker.pots(hand_id,pot_index,amount_units,contribution_floor,contribution_ceiling) VALUES($1,$2,$3,$4,$5)", t.HandID, p.Index, p.Amount, p.Floor, p.Ceiling); err != nil {
			return err
		}
		for _, seat := range p.Eligible {
			if _, err = tx.Exec(ctx, "INSERT INTO poker.pot_eligible_players(hand_id,pot_index,seat_no) VALUES($1,$2,$3)", t.HandID, p.Index, seat); err != nil {
				return err
			}
		}
		for _, award := range p.Awards {
			awarded += award.Amount
			if _, err = tx.Exec(ctx, "INSERT INTO poker.pot_awards(hand_id,pot_index,seat_no,base_share_units,odd_chip_units,award_units) VALUES($1,$2,$3,$4,$5,$6)", t.HandID, p.Index, award.Seat, award.Base, award.Odd, award.Amount); err != nil {
				return err
			}
		}
	}
	for _, r := range state.Returns {
		returned += r.Amount
	}
	if _, err = tx.Exec(ctx, "INSERT INTO poker.settlements(hand_id,biz_id,total_commitment_units,total_award_units,uncalled_return_units) VALUES($1,$2,$3,$4,$5)", t.HandID, "poker_hand_settlement:"+t.HandID, paid, awarded, returned); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE poker.hands SET settled_at=$2 WHERE hand_id=$1", t.HandID, t.Now); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE poker.hand_fairness SET full_fairness_reveal_at=$2::timestamptz+interval '24 hours' WHERE hand_id=$1", t.HandID, t.Now); err != nil {
		return err
	}
	lifecycle := "INTERMISSION"
	if !t.AllowHands {
		lifecycle = "PAUSED"
	}
	if t.CloseRequested || t.State == "CLOSING" {
		lifecycle = "CLOSING"
	}
	if _, err = tx.Exec(ctx, "UPDATE poker.tables SET lifecycle_state=$2,intermission_until=$3::timestamptz+interval '5 seconds' WHERE table_id=$1", t.ID, lifecycle, t.Now); err != nil {
		return err
	}
	t.State = lifecycle
	return s.boundary(ctx, tx, t)
}

func (s *Service) Act(ctx context.Context, c ActCommand) (Receipt, error) {
	return s.mutate(ctx, c.TableID, func(ctx context.Context, tx pgx.Tx, t *tableRow) (Receipt, error) {
		seat, err := userSeat(ctx, tx, t.ID, c.UserID)
		if err != nil {
			return Receipt{}, err
		}
		if err = s.validateControl(ctx, tx, t, seat, c.Control); err != nil {
			return Receipt{}, err
		}
		intent := actionIntent{sessionIntent{c.UserID, c.TableID, seat.Session}, c.HandID, c.Kind, c.ToUnits}
		if r, ok, err := findReceipt(ctx, tx, c.UserID, "poker.action.v1", c.Key, intent); err != nil || ok {
			return r, err
		}
		if c.HandID != t.HandID {
			return Receipt{}, ErrDenied
		}
		if err = s.guardMutation(ctx, tx, t, seat, c.Control, c.TableVersion, c.HandVersion, c.HandID); err != nil {
			return Receipt{}, err
		}
		if seat.Control != c.ControlEpoch || !seat.Connected {
			return Receipt{}, ErrDenied
		}
		e, err := s.restoreEngine(ctx, tx, t.HandID)
		if err != nil {
			return Receipt{}, err
		}
		applied, err := e.Apply(engine.Action{ID: c.Key, Seat: seat.Seat, ExpectedVersion: c.HandVersion, Kind: c.Kind, To: c.ToUnits}, t.Now)
		if err != nil {
			return Receipt{}, err
		}
		if err = s.persistEngine(ctx, tx, t, e); err != nil {
			return Receipt{}, err
		}
		r := Receipt{TableID: t.ID, HandID: t.HandID, Status: string(e.State().Street), Version: t.Version + 1, Duplicate: applied.Duplicate}
		return r, saveReceipt(ctx, tx, c.UserID, "poker.action.v1", c.Key, intent, r)
	})
}

func (s *Service) Tick(ctx context.Context, table string) error {
	r, err := s.mutate(ctx, table, s.tickMutation)
	if err == nil && r.Status == "AUTO_START" {
		_, err = s.mutate(ctx, table, s.prepareHand(0, ""))
		if err == nil {
			_, err = s.mutate(ctx, table, s.dealCommitted)
		}
	}
	return err
}
func (s *Service) tickMutation(ctx context.Context, tx pgx.Tx, t *tableRow) (Receipt, error) {
	if t.State == "CLOSED" {
		return Receipt{Status: "NOOP"}, nil
	}
	if t.HandID != "" {
		var state string
		if err := tx.QueryRow(ctx, "SELECT state FROM poker.hands WHERE hand_id=$1", t.HandID).Scan(&state); err != nil {
			return Receipt{}, err
		}
		if state == "COMMITTED" {
			return s.dealCommitted(ctx, tx, t)
		}
		if state != "SETTLED" {
			e, err := s.restoreEngine(ctx, tx, t.HandID)
			if err != nil {
				return Receipt{}, err
			}
			var result engine.Result
			if e.State().RecoveryUntil != nil {
				result, err = e.Resume(t.Now)
				if err == nil && !result.NoOp {
					t.State = "IN_HAND"
					if t.CloseRequested {
						t.State = "CLOSING"
					}
					_, err = tx.Exec(ctx, "UPDATE poker.tables SET lifecycle_state=$2 WHERE table_id=$1", t.ID, t.State)
					if err == nil {
						_, err = tx.Exec(ctx, "UPDATE poker.recovery_state SET state='RESUMED',resumed_at=$2 WHERE table_id=$1", t.ID, t.Now)
					}
				}
			} else {
				result, err = e.Timeout(e.State().ActionSequence, t.Now)
			}
			if err != nil {
				return Receipt{}, err
			}
			if result.NoOp {
				return Receipt{Status: "NOOP"}, nil
			}
			if err = s.persistEngine(ctx, tx, t, e); err != nil {
				return Receipt{}, err
			}
			return Receipt{TableID: t.ID, HandID: t.HandID, Status: string(e.State().Street)}, nil
		}
	}
	seats, err := loadSeats(ctx, tx, t.ID)
	if err != nil {
		return Receipt{}, err
	}
	due := t.State == "CLOSING"
	for _, seat := range seats {
		if seat.Leave || seat.Pending > 0 || (seat.RebuyUntil != nil && !seat.RebuyUntil.After(t.Now)) || (seat.Disconnected != nil && !seat.Disconnected.Add(15*time.Minute).After(t.Now)) || (seat.SitOutSince != nil && !seat.SitOutSince.Add(15*time.Minute).After(t.Now)) {
			due = true
		}
	}
	if due {
		if err = s.boundary(ctx, tx, t); err != nil {
			return Receipt{}, err
		}
		return Receipt{TableID: t.ID, Status: "BOUNDARY"}, nil
	}
	if t.EmptySince != nil && !t.EmptySince.Add(30*time.Minute).After(t.Now) && len(seats) == 0 && t.Spectators == 0 {
		closed, err := closeTableIfSettled(ctx, tx, t)
		if err != nil {
			return Receipt{}, err
		}
		if !closed {
			return Receipt{Status: "NOOP"}, nil
		}
		return Receipt{TableID: t.ID, Status: "CLOSED"}, nil
	}
	if t.AllowHands && (t.State == "WAITING" || t.State == "INTERMISSION" && t.Intermission != nil && !t.Intermission.After(t.Now)) {
		seats, err = s.readySeats(ctx, tx, t, seats)
		if err != nil {
			return Receipt{}, err
		}
		if _, _, err := selectParticipants(t, seats); err == nil {
			return Receipt{TableID: t.ID, Status: "AUTO_START"}, nil
		}
	}
	return Receipt{Status: "NOOP"}, nil
}
