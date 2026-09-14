// Package engine is a deterministic, single-hand, no-ante V1 cash-game core.
// It owns no wallet, network, clock, shuffle, database, or production ruleset.
package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"
)

const SnapshotVersion = 1
const DecisionWindow = 30 * time.Second

// UnitsPerChip is the frozen wallet atomic-unit scale. Every poker amount,
// including state/events/awards and action targets, must be a whole Chip.
const UnitsPerChip int64 = 500_000

const maxWholeUnits = math.MaxInt64 / UnitsPerChip * UnitsPerChip

func wholeUnits(amount int64) bool { return amount >= 0 && amount%UnitsPerChip == 0 }

type ActionKind string

const (
	Fold  ActionKind = "FOLD"
	Check ActionKind = "CHECK"
	Call  ActionKind = "CALL"
	Bet   ActionKind = "BET"
	Raise ActionKind = "RAISE"
	AllIn ActionKind = "ALL_IN"
)

type Street string

const (
	Preflop Street = "PREFLOP"
	Flop    Street = "FLOP"
	Turn    Street = "TURN"
	River   Street = "RIVER"
	Settled Street = "SETTLED"
)

var (
	ErrInvalid    = errors.New("invalid poker input")
	ErrIllegal    = errors.New("illegal poker action")
	ErrStale      = errors.New("stale hand version")
	ErrConflict   = errors.New("action id conflict")
	ErrDeadline   = errors.New("action deadline reached")
	ErrRecovering = errors.New("hand recovering")
)

type Config struct {
	HandID           string        `json:"hand_id"`
	ButtonSeat       int           `json:"button_seat"`
	SmallBlind       int64         `json:"small_blind_units"`
	BigBlind         int64         `json:"big_blind_units"`
	Deck             []Card        `json:"frozen_deck"`
	EvaluatorVersion string        `json:"evaluator_version"`
	ButtonSelection  *ButtonChoice `json:"button_selection,omitempty"`
}

type Seat struct {
	SeatNo              int    `json:"seat_no"`
	PlayerID            string `json:"player_id"`
	Stack               int64  `json:"stack_units"`
	ConsecutiveTimeouts int    `json:"consecutive_timeouts"`
	TimeoutSitOut       bool   `json:"timeout_sit_out"`
}

type Participant struct {
	Seat
	InitialStack               int64      `json:"initial_stack_units"`
	Hole                       [2]Card    `json:"hole_cards"`
	StreetCommitted            int64      `json:"street_committed_units"`
	TotalCommitted             int64      `json:"total_committed_units"`
	Folded                     bool       `json:"folded"`
	LastActedFullRaiseSequence int64      `json:"last_acted_full_raise_sequence"`
	Pending                    bool       `json:"pending_action"`
	DisconnectedSince          *time.Time `json:"disconnected_since,omitempty"`
	DisconnectSitOut           bool       `json:"disconnect_sit_out"`
}

type Action struct {
	ID              string     `json:"action_id"`
	Seat            int        `json:"seat_no"`
	ExpectedVersion uint64     `json:"expected_hand_version"`
	Kind            ActionKind `json:"kind"`
	To              int64      `json:"requested_to_units"`
}

type Legal struct {
	Actions        []ActionKind `json:"legal_actions"`
	Seat           int          `json:"seat_no"`
	ToCall         int64        `json:"to_call_units"`
	MinimumBet     int64        `json:"minimum_bet_units"`
	MinimumRaiseTo int64        `json:"minimum_raise_to_units"`
	MaximumRaiseTo int64        `json:"maximum_raise_to_units"`
	CurrentPot     int64        `json:"current_pot_units"`
	CurrentStack   int64        `json:"current_stack_units"`
	RaiseRights    bool         `json:"raise_rights"`
}

type Event struct {
	Sequence   uint64    `json:"sequence"`
	Version    uint64    `json:"hand_version"`
	Type       string    `json:"type"`
	Street     Street    `json:"street"`
	Seat       int       `json:"seat_no,omitempty"`
	Delta      int64     `json:"delta_units,omitempty"`
	To         int64     `json:"to_units,omitempty"`
	ActionID   string    `json:"action_id,omitempty"`
	DeckIndex  *int      `json:"deck_index,omitempty"`
	Card       *Card     `json:"card,omitempty"`
	Visibility string    `json:"visibility,omitempty"`
	At         time.Time `json:"at"`
}

type Award struct {
	Seat   int   `json:"seat_no"`
	Base   int64 `json:"base_share_units"`
	Odd    int64 `json:"odd_chip_units"`
	Amount int64 `json:"award_units"`
}

type Pot struct {
	Index    int     `json:"index"`
	Amount   int64   `json:"amount_units"`
	Floor    int64   `json:"contribution_floor"`
	Ceiling  int64   `json:"contribution_ceiling"`
	Eligible []int   `json:"eligible_seats"`
	Awards   []Award `json:"awards"`
}

type Return struct {
	Seat   int   `json:"seat_no"`
	Amount int64 `json:"amount_units"`
}

type Receipt struct {
	Action     Action `json:"action"`
	FirstEvent int    `json:"first_event"`
	LastEvent  int    `json:"last_event"`
}

type State struct {
	SchemaVersion          int              `json:"schema_version"`
	Config                 Config           `json:"config"`
	Players                []Participant    `json:"players"`
	InitialTotal           int64            `json:"initial_total_units"`
	Street                 Street           `json:"street"`
	Board                  []Card           `json:"board"`
	DeckCursor             int              `json:"deck_cursor"`
	SmallBlindSeat         int              `json:"small_blind_seat"`
	BigBlindSeat           int              `json:"big_blind_seat"`
	Actor                  int              `json:"actor_seat"`
	CurrentBet             int64            `json:"current_bet_to_units"`
	LastFullRaiseIncrement int64            `json:"last_full_raise_increment_units"`
	FullRaiseSequence      int64            `json:"full_raise_sequence"`
	Version                uint64           `json:"hand_version"`
	ActionSequence         uint64           `json:"action_sequence"`
	StartedAt              time.Time        `json:"action_started_at"`
	Deadline               time.Time        `json:"action_deadline_at"`
	RecoveryUntil          *time.Time       `json:"reconnect_grace_until,omitempty"`
	Pots                   []Pot            `json:"pots"`
	Returns                []Return         `json:"uncalled_returns"`
	Ranks                  map[int]HandRank `json:"ranks,omitempty"`
	Events                 []Event          `json:"events"`
	Receipts               []Receipt        `json:"receipts"`
}

type Result struct {
	Version   uint64
	Events    []Event
	Duplicate bool
	NoOp      bool
}

// Engine is single-writer. The future table actor must serialize callers and
// durably commit Snapshot plus events before broadcasting any projection.
type Engine struct {
	state    State
	evaluate Evaluator
}

func New(config Config, seats []Seat, now time.Time, evaluate Evaluator) (*Engine, error) {
	if evaluate == nil {
		return nil, fmt.Errorf("%w: evaluator required", ErrInvalid)
	}
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	if len(seats) < 2 || len(seats) > 9 || now.IsZero() {
		return nil, ErrInvalid
	}
	e := &Engine{evaluate: evaluate, state: State{SchemaVersion: SnapshotVersion, Config: config, Street: Preflop, LastFullRaiseIncrement: config.BigBlind}}
	s := &e.state
	ordered := append([]Seat{}, seats...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].SeatNo < ordered[j].SeatNo })
	ids := map[string]bool{}
	prev := 0
	for _, seat := range ordered {
		if seat.SeatNo < 1 || seat.SeatNo > 9 || seat.SeatNo <= prev || seat.PlayerID == "" || ids[seat.PlayerID] || seat.Stack <= 0 || !wholeUnits(seat.Stack) || seat.ConsecutiveTimeouts < 0 || seat.ConsecutiveTimeouts > math.MaxInt32 || seat.TimeoutSitOut {
			return nil, ErrInvalid
		}
		if seat.Stack > math.MaxInt64-s.InitialTotal {
			return nil, ErrInvalid
		}
		ids[seat.PlayerID] = true
		prev = seat.SeatNo
		s.InitialTotal += seat.Stack
		s.Players = append(s.Players, Participant{Seat: seat, InitialStack: seat.Stack, LastActedFullRaiseSequence: -1, Pending: true})
	}
	if s.player(config.ButtonSeat) == nil {
		return nil, fmt.Errorf("%w: explicit participating button required", ErrInvalid)
	}
	if config.ButtonSelection != nil {
		if err := validateButtonChoice(*config.ButtonSelection, s.seats()); err != nil || config.ButtonSelection.Seat != config.ButtonSeat {
			return nil, ErrInvalid
		}
	}
	// Own the caller's deck and optional selection before any runtime mutation.
	e.state = clone(s)
	s = &e.state
	s.emit(Event{Type: "HAND_COMMITTED"}, now)
	s.SmallBlindSeat = s.nextSeat(config.ButtonSeat, func(p Participant) bool { return true })
	if len(s.Players) == 2 {
		s.SmallBlindSeat = config.ButtonSeat
	}
	s.BigBlindSeat = s.nextSeat(s.SmallBlindSeat, func(p Participant) bool { return true })
	for _, blind := range []struct {
		seat   int
		amount int64
		kind   string
	}{{s.SmallBlindSeat, config.SmallBlind, "POST_SB"}, {s.BigBlindSeat, config.BigBlind, "POST_BB"}} {
		p := s.player(blind.seat)
		paid := min(p.Stack, blind.amount)
		s.commit(p, paid)
		s.emit(Event{Type: blind.kind, Seat: p.SeatNo, Delta: paid, To: p.StreetCommitted}, now)
	}
	s.CurrentBet = config.BigBlind
	for pass := 0; pass < 2; pass++ {
		for i := range s.Players {
			p := &s.Players[i]
			p.Hole[pass] = s.deal(p.SeatNo, "PRIVATE", now)
		}
	}
	if err := e.progress(s.BigBlindSeat, now); err != nil {
		return nil, err
	}
	if err := s.validate(); err != nil {
		return nil, err
	}
	return e, nil
}

// State and Snapshot contain private hole cards and the entire frozen deck.
// They are trusted server persistence only, never a viewer projection.
func (e *Engine) State() State              { return clone(&e.state) }
func (e *Engine) LegalActions() Legal       { return e.state.legal() }
func (e *Engine) Snapshot() ([]byte, error) { return json.Marshal(e.state) }

// Restore validates the persisted invariants. Authentication and durable integrity
// of the snapshot remain the storage adapter's responsibility.
func Restore(data []byte, evaluate Evaluator) (*Engine, error) {
	if evaluate == nil {
		return nil, ErrInvalid
	}
	var s State
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, ErrInvalid
	}
	if err := s.validate(); err != nil {
		return nil, err
	}
	return &Engine{state: s, evaluate: evaluate}, nil
}

func (e *Engine) Apply(a Action, now time.Time) (Result, error) {
	for _, r := range e.state.Receipts {
		if r.Action.ID == a.ID {
			if r.Action != a {
				return Result{}, ErrConflict
			}
			return Result{Version: e.state.Version, Duplicate: true, Events: clone(&e.state).Events[r.FirstEvent:r.LastEvent]}, nil
		}
	}
	if a.ID == "" || len(a.ID) > 128 || strings.HasPrefix(a.ID, "timeout:") {
		return Result{}, ErrInvalid
	}
	return e.apply(a, now, false)
}

func (e *Engine) apply(a Action, now time.Time, timeout bool) (Result, error) {
	s := &e.state
	if s.RecoveryUntil != nil {
		return Result{}, ErrRecovering
	}
	if a.ExpectedVersion != s.Version {
		return Result{}, ErrStale
	}
	if s.Street == Settled || s.Actor != a.Seat || a.Seat == 0 || now.IsZero() || now.Before(s.StartedAt) {
		return Result{}, ErrIllegal
	}
	if !timeout && !now.Before(s.Deadline) {
		return Result{}, ErrDeadline
	}
	l := s.legal()
	allowed := false
	for _, kind := range l.Actions {
		if kind == a.Kind {
			allowed = true
		}
	}
	if !allowed || !wholeUnits(a.To) || ((a.Kind != Raise && a.Kind != Bet) && a.To != 0) {
		return Result{}, ErrIllegal
	}
	p := s.player(a.Seat)
	target := p.StreetCommitted
	switch a.Kind {
	case Call:
		target += min(l.ToCall, p.Stack)
	case AllIn:
		target += p.Stack
	case Bet, Raise:
		minimum := l.MinimumRaiseTo
		if a.Kind == Bet {
			minimum = l.MinimumBet
		}
		if a.To < minimum || a.To > l.MaximumRaiseTo {
			return Result{}, ErrIllegal
		}
		target = a.To
	}
	// Transactional value-copy: errors from evaluation or validation never partly
	// consume cards/chips. ponytail: bounded 2–9 seats; durable actors can optimize
	// copying only after profiling, without changing the atomic API contract.
	next := &Engine{state: clone(s), evaluate: e.evaluate}
	n := &next.state
	p = n.player(a.Seat)
	start := len(n.Events)
	if a.Kind == Fold {
		p.Folded = true
	}
	delta := target - p.StreetCommitted
	n.commit(p, delta)
	if target > n.CurrentBet {
		increment := target - n.CurrentBet
		if increment >= n.LastFullRaiseIncrement {
			n.FullRaiseSequence++
			n.LastFullRaiseIncrement = increment
		}
		n.CurrentBet = target
		for i := range n.Players {
			other := &n.Players[i]
			if other.SeatNo != p.SeatNo && !other.Folded && other.Stack > 0 && other.StreetCommitted < target {
				other.Pending = true
			}
		}
	}
	p.Pending = false
	p.LastActedFullRaiseSequence = n.FullRaiseSequence
	if timeout {
		p.ConsecutiveTimeouts++
		if p.ConsecutiveTimeouts >= 2 {
			p.TimeoutSitOut = true
		}
	} else {
		p.ConsecutiveTimeouts = 0
	}
	kind := string(a.Kind)
	if timeout {
		kind = "AUTO_" + kind
	}
	n.emit(Event{Type: kind, Seat: a.Seat, Delta: delta, To: target, ActionID: a.ID}, now)
	if err := next.progress(a.Seat, now); err != nil {
		return Result{}, err
	}
	n.Receipts = append(n.Receipts, Receipt{Action: a, FirstEvent: start, LastEvent: len(n.Events)})
	if err := n.validate(); err != nil {
		return Result{}, err
	}
	e.state = next.state
	return e.result(start), nil
}

func (e *Engine) Timeout(sequence uint64, now time.Time) (Result, error) {
	s := &e.state
	if s.Street == Settled || s.RecoveryUntil != nil || sequence != s.ActionSequence || now.Before(s.Deadline) {
		return Result{Version: s.Version, NoOp: true}, nil
	}
	kind := Fold
	if s.legal().ToCall == 0 {
		kind = Check
	}
	a := Action{ID: fmt.Sprintf("timeout:%s:%d", s.Config.HandID, sequence), Seat: s.Actor, ExpectedVersion: s.Version, Kind: kind}
	return e.apply(a, now, true)
}

func (e *Engine) SetConnected(seat int, connected bool, now time.Time) (Result, error) {
	s := &e.state
	p := s.player(seat)
	if p == nil || now.IsZero() {
		return Result{}, ErrInvalid
	}
	if connected == (p.DisconnectedSince == nil) {
		return Result{Version: s.Version, NoOp: true}, nil
	}
	start := len(s.Events)
	kind := "DISCONNECTED"
	if connected {
		p.DisconnectedSince = nil
		if s.Street != Settled {
			p.DisconnectSitOut = false
		}
		kind = "RECONNECTED"
	} else {
		at := now.UTC()
		p.DisconnectedSince = &at
		if s.Street == Settled {
			p.DisconnectSitOut = true
		}
	}
	s.emit(Event{Type: kind, Seat: seat}, now)
	return e.result(start), nil
}

// BeginRecovery is called when service reconstruction has finished. It starts
// the reconnect grace; old deadlines are not charged to the player.
func (e *Engine) BeginRecovery(now time.Time) (Result, error) {
	s := &e.state
	if now.IsZero() {
		return Result{}, ErrInvalid
	}
	if s.Street == Settled || s.RecoveryUntil != nil {
		return Result{Version: s.Version, NoOp: true}, nil
	}
	start := len(s.Events)
	until := now.UTC().Add(DecisionWindow)
	s.RecoveryUntil = &until
	s.emit(Event{Type: "RECOVERING"}, now)
	return e.result(start), nil
}

func (e *Engine) Resume(now time.Time) (Result, error) {
	s := &e.state
	if s.RecoveryUntil == nil || now.Before(*s.RecoveryUntil) {
		return Result{Version: s.Version, NoOp: true}, nil
	}
	start := len(s.Events)
	s.RecoveryUntil = nil
	s.emit(Event{Type: "RESUMED"}, now)
	s.setActor(s.Actor, now)
	return e.result(start), nil
}

func (e *Engine) result(start int) Result {
	s := clone(&e.state)
	return Result{Version: s.Version, Events: s.Events[start:]}
}

func clone(s *State) State {
	data, _ := json.Marshal(s)
	var c State
	_ = json.Unmarshal(data, &c)
	return c
}

func validateConfig(c Config) error {
	if c.HandID == "" || len(c.HandID) > 128 || c.EvaluatorVersion == "" || c.ButtonSeat < 1 || c.ButtonSeat > 9 || len(c.Deck) != 52 {
		return ErrInvalid
	}
	valid := false
	for _, bb := range []int64{10, 20, 50, 100, 200, 1000} {
		if c.BigBlind == bb*UnitsPerChip && c.SmallBlind == (bb/2)*UnitsPerChip {
			valid = true
		}
	}
	if !valid {
		return fmt.Errorf("%w: V1 blind preset required", ErrInvalid)
	}
	seen := [52]bool{}
	for _, card := range c.Deck {
		if card >= 52 || seen[card] {
			return fmt.Errorf("%w: deck must be a permutation of 0..51", ErrInvalid)
		}
		seen[card] = true
	}
	return nil
}

func (s *State) player(seat int) *Participant {
	for i := range s.Players {
		if s.Players[i].SeatNo == seat {
			return &s.Players[i]
		}
	}
	return nil
}
func (s *State) seats() []int {
	out := make([]int, len(s.Players))
	for i, p := range s.Players {
		out[i] = p.SeatNo
	}
	return out
}
func (s *State) nextSeat(after int, eligible func(Participant) bool) int {
	for _, p := range s.Players {
		if p.SeatNo > after && eligible(p) {
			return p.SeatNo
		}
	}
	for _, p := range s.Players {
		if eligible(p) {
			return p.SeatNo
		}
	}
	return 0
}
func (s *State) commit(p *Participant, delta int64) {
	p.Stack -= delta
	p.StreetCommitted += delta
	p.TotalCommitted += delta
	if p.Stack == 0 {
		p.Pending = false
	}
}
func (s *State) emit(event Event, now time.Time) {
	s.Version++
	event.Sequence = uint64(len(s.Events) + 1)
	event.Version = s.Version
	event.Street = s.Street
	event.At = now.UTC()
	s.Events = append(s.Events, event)
}
func (s *State) setActor(seat int, now time.Time) {
	s.Actor = seat
	s.ActionSequence++
	s.StartedAt = now.UTC()
	s.Deadline = now.UTC().Add(DecisionWindow)
	s.emit(Event{Type: "ACTOR_CHANGED", Seat: seat}, now)
}
func (s *State) deal(seat int, visibility string, now time.Time) Card {
	index := s.DeckCursor
	card := s.Config.Deck[index]
	s.DeckCursor++
	s.emit(Event{Type: "CARD_DEALT", Seat: seat, DeckIndex: &index, Card: &card, Visibility: visibility}, now)
	return card
}

func (s *State) legal() Legal {
	l := Legal{Seat: s.Actor, MinimumBet: s.Config.BigBlind}
	for _, p := range s.Players {
		l.CurrentPot += p.TotalCommitted
	}
	p := s.player(s.Actor)
	if p == nil || s.Street == Settled || s.RecoveryUntil != nil {
		return l
	}
	l.ToCall = max(int64(0), s.CurrentBet-p.StreetCommitted)
	l.CurrentStack = p.Stack
	l.MaximumRaiseTo = p.Stack + p.StreetCommitted
	l.MinimumRaiseTo = addCap(s.CurrentBet, s.LastFullRaiseIncrement)
	opponentCanAct := false
	for _, other := range s.Players {
		if other.SeatNo != p.SeatNo && !other.Folded && other.Stack > 0 {
			opponentCanAct = true
		}
	}
	l.RaiseRights = p.LastActedFullRaiseSequence < s.FullRaiseSequence && opponentCanAct && l.MaximumRaiseTo > s.CurrentBet
	l.Actions = []ActionKind{Fold}
	if l.ToCall == 0 {
		l.Actions = append(l.Actions, Check)
	} else {
		l.Actions = append(l.Actions, Call)
	}
	if l.RaiseRights {
		if s.CurrentBet == 0 && l.MaximumRaiseTo >= l.MinimumBet {
			l.Actions = append(l.Actions, Bet)
		} else if s.CurrentBet > 0 && l.MaximumRaiseTo >= l.MinimumRaiseTo {
			l.Actions = append(l.Actions, Raise)
		}
	}
	if p.Stack > 0 && (l.MaximumRaiseTo <= s.CurrentBet || l.RaiseRights) {
		l.Actions = append(l.Actions, AllIn)
	}
	return l
}

func addCap(a, b int64) int64 {
	if a > maxWholeUnits-b {
		return maxWholeUnits
	}
	return a + b
}

func (e *Engine) progress(after int, now time.Time) error {
	s := &e.state
	for {
		live, active := 0, 0
		var lone *Participant
		for i := range s.Players {
			p := &s.Players[i]
			if !p.Folded {
				live++
				if p.Stack > 0 {
					active++
					lone = p
				}
			}
			if p.Stack == 0 || p.Folded {
				p.Pending = false
			}
		}
		if live == 1 {
			return e.settle(now)
		}
		if active <= 1 {
			needsCall := false
			if lone != nil {
				for _, p := range s.Players {
					if p.SeatNo != lone.SeatNo && p.StreetCommitted > lone.StreetCommitted {
						needsCall = true
					}
				}
			}
			if !needsCall {
				s.emit(Event{Type: "ALL_IN_RUNOUT"}, now)
				for s.Street != River {
					s.nextStreet(now)
				}
				return e.settle(now)
			}
		}
		next := s.nextSeat(after, func(p Participant) bool { return p.Pending && !p.Folded && p.Stack > 0 })
		if next != 0 {
			s.setActor(next, now)
			return nil
		}
		if s.Street == River {
			return e.settle(now)
		}
		s.nextStreet(now)
		after = s.Config.ButtonSeat
	}
}

func (s *State) nextStreet(now time.Time) {
	count := 1
	switch s.Street {
	case Preflop:
		s.Street = Flop
		count = 3
	case Flop:
		s.Street = Turn
	case Turn:
		s.Street = River
	}
	s.CurrentBet = 0
	s.LastFullRaiseIncrement = s.Config.BigBlind
	s.FullRaiseSequence = 0
	for i := range s.Players {
		p := &s.Players[i]
		p.StreetCommitted = 0
		p.LastActedFullRaiseSequence = -1
		p.Pending = !p.Folded && p.Stack > 0
	}
	s.emit(Event{Type: "DEAL_" + string(s.Street)}, now)
	for i := 0; i < count; i++ {
		s.Board = append(s.Board, s.deal(0, "PUBLIC", now))
	}
}

func (e *Engine) settle(now time.Time) error {
	s := &e.state
	// Return a unique top level before constructing any pot or award.
	var highest, second int64
	var top *Participant
	for i := range s.Players {
		p := &s.Players[i]
		if p.TotalCommitted > highest {
			second = highest
			highest = p.TotalCommitted
			top = p
		} else if p.TotalCommitted > second {
			second = p.TotalCommitted
		}
	}
	if highest > second {
		amount := highest - second
		top.TotalCommitted -= amount
		top.StreetCommitted -= min(top.StreetCommitted, amount)
		top.Stack += amount
		s.Returns = append(s.Returns, Return{top.SeatNo, amount})
		s.emit(Event{Type: "RETURN_UNCALLED", Seat: top.SeatNo, Delta: amount}, now)
	}
	levels := []int64{}
	for _, p := range s.Players {
		if p.TotalCommitted > 0 {
			levels = append(levels, p.TotalCommitted)
		}
	}
	sort.Slice(levels, func(i, j int) bool { return levels[i] < levels[j] })
	live := 0
	for _, p := range s.Players {
		if !p.Folded {
			live++
		}
	}
	if live > 1 {
		s.emit(Event{Type: "SHOWDOWN"}, now)
		s.Ranks = map[int]HandRank{}
		for _, p := range s.Players {
			if p.Folded {
				continue
			}
			rank, err := e.evaluate(p.Hole, append([]Card{}, s.Board...), s.Config.EvaluatorVersion)
			if err != nil {
				return err
			}
			if err := validateRank(rank, s.Config.EvaluatorVersion); err != nil {
				return err
			}
			s.Ranks[p.SeatNo] = rank
		}
	}
	var previous int64
	for _, level := range levels {
		if level == previous {
			continue
		}
		pot := Pot{Index: len(s.Pots), Floor: previous, Ceiling: level}
		for _, p := range s.Players {
			if p.TotalCommitted >= level {
				pot.Amount += level - previous
				if !p.Folded {
					pot.Eligible = append(pot.Eligible, p.SeatNo)
				}
			}
		}
		if len(pot.Eligible) == 0 {
			return fmt.Errorf("%w: pot has no eligible winner", ErrInvalid)
		}
		var winners []int
		for _, seat := range pot.Eligible {
			if len(winners) == 0 {
				winners = append(winners, seat)
				continue
			}
			comparison := compareRanks(s.Ranks[seat].Vector, s.Ranks[winners[0]].Vector)
			if comparison > 0 {
				winners = []int{seat}
			} else if comparison == 0 {
				winners = append(winners, seat)
			}
		}
		// Clockwise from button left, only this pot's tied winners participate.
		sort.Slice(winners, func(i, j int) bool {
			return (winners[i]-s.Config.ButtonSeat+9)%9 < (winners[j]-s.Config.ButtonSeat+9)%9
		})
		// Button itself is last, not distance zero.
		if len(winners) > 1 && winners[0] == s.Config.ButtonSeat {
			winners = append(winners[1:], winners[0])
		}
		potChips := pot.Amount / UnitsPerChip
		base := (potChips / int64(len(winners))) * UnitsPerChip
		odd := potChips % int64(len(winners))
		for i, seat := range winners {
			extra := int64(0)
			if int64(i) < odd {
				extra = UnitsPerChip
			}
			award := Award{Seat: seat, Base: base, Odd: extra, Amount: base + extra}
			pot.Awards = append(pot.Awards, award)
			s.player(seat).Stack += award.Amount
			s.emit(Event{Type: "POT_AWARD", Seat: seat, Delta: award.Amount}, now)
		}
		s.Pots = append(s.Pots, pot)
		previous = level
	}
	for i := range s.Players {
		p := &s.Players[i]
		p.TotalCommitted = 0
		p.StreetCommitted = 0
		p.Pending = false
		if p.DisconnectedSince != nil {
			p.DisconnectSitOut = true
		}
	}
	s.Actor = 0
	s.CurrentBet = 0
	s.StartedAt = time.Time{}
	s.Deadline = time.Time{}
	s.Street = Settled
	s.emit(Event{Type: "SYSTEM_SETTLEMENT"}, now)
	return nil
}

func compareRanks(a, b []int) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] > b[i] {
			return 1
		}
		if a[i] < b[i] {
			return -1
		}
	}
	if len(a) > len(b) {
		return 1
	}
	if len(a) < len(b) {
		return -1
	}
	return 0
}
func validateRank(r HandRank, version string) error {
	if r.Version != version || r.Category == "" || len(r.Vector) == 0 || len(r.Vector) > 6 {
		return ErrInvalid
	}
	seen := [7]bool{}
	for _, i := range r.BestFive {
		if i < 0 || i >= 7 || seen[i] {
			return ErrInvalid
		}
		seen[i] = true
	}
	return nil
}

func (s *State) validate() error {
	if s.SchemaVersion != SnapshotVersion || len(s.Players) < 2 || len(s.Players) > 9 || s.InitialTotal <= 0 || !wholeUnits(s.InitialTotal) || s.LastFullRaiseIncrement <= 0 || !wholeUnits(s.LastFullRaiseIncrement) || s.FullRaiseSequence < 0 || s.Version != uint64(len(s.Events)) {
		return ErrInvalid
	}
	if err := validateConfig(s.Config); err != nil {
		return err
	}
	if s.player(s.Config.ButtonSeat) == nil || s.player(s.SmallBlindSeat) == nil || s.player(s.BigBlindSeat) == nil || s.SmallBlindSeat == s.BigBlindSeat {
		return ErrInvalid
	}
	var total, initial int64
	prev := 0
	ids := map[string]bool{}
	used := map[Card]bool{}
	for _, p := range s.Players {
		if p.SeatNo <= prev || p.SeatNo < 1 || p.SeatNo > 9 || p.PlayerID == "" || ids[p.PlayerID] || !wholeUnits(p.Stack) || !wholeUnits(p.TotalCommitted) || !wholeUnits(p.StreetCommitted) || p.StreetCommitted > p.TotalCommitted || p.InitialStack <= 0 || !wholeUnits(p.InitialStack) || p.ConsecutiveTimeouts < 0 || p.ConsecutiveTimeouts > math.MaxInt32 || p.LastActedFullRaiseSequence < -1 || p.LastActedFullRaiseSequence > s.FullRaiseSequence {
			return ErrInvalid
		}
		if p.Stack > math.MaxInt64-total {
			return ErrInvalid
		}
		total += p.Stack
		if p.TotalCommitted > math.MaxInt64-total {
			return ErrInvalid
		}
		total += p.TotalCommitted
		if p.InitialStack > math.MaxInt64-initial {
			return ErrInvalid
		}
		initial += p.InitialStack
		prev = p.SeatNo
		ids[p.PlayerID] = true
		for _, c := range p.Hole {
			if c >= 52 || used[c] {
				return ErrInvalid
			}
			used[c] = true
		}
	}
	if total != s.InitialTotal || initial != s.InitialTotal {
		return fmt.Errorf("%w: chip conservation", ErrInvalid)
	}
	for _, c := range s.Board {
		if c >= 52 || used[c] {
			return ErrInvalid
		}
		used[c] = true
	}
	if s.DeckCursor != len(used) || s.DeckCursor > 23 || s.DeckCursor < 4 {
		return ErrInvalid
	}
	for i, p := range s.Players {
		if p.Hole[0] != s.Config.Deck[i] || p.Hole[1] != s.Config.Deck[len(s.Players)+i] {
			return ErrInvalid
		}
	}
	for i, c := range s.Board {
		if c != s.Config.Deck[2*len(s.Players)+i] {
			return ErrInvalid
		}
	}
	boardCount := 0
	switch s.Street {
	case Preflop:
		boardCount = 0
	case Flop:
		boardCount = 3
	case Turn:
		boardCount = 4
	case River:
		boardCount = 5
	case Settled:
		boardCount = len(s.Board)
		if boardCount != 0 && boardCount != 3 && boardCount != 4 && boardCount != 5 {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	if len(s.Board) != boardCount || !wholeUnits(s.CurrentBet) {
		return ErrInvalid
	}
	if s.Street != Settled {
		expectedBet := int64(0)
		for _, p := range s.Players {
			expectedBet = max(expectedBet, p.StreetCommitted)
		}
		if s.Street == Preflop {
			expectedBet = max(expectedBet, s.Config.BigBlind)
		}
		if s.CurrentBet != expectedBet {
			return fmt.Errorf("%w: current bet disagrees with commitments", ErrInvalid)
		}
	}
	if s.Street != Settled {
		p := s.player(s.Actor)
		if p == nil || p.Folded || p.Stack == 0 || !p.Pending || s.ActionSequence == 0 || s.StartedAt.IsZero() || !s.Deadline.Equal(s.StartedAt.Add(DecisionWindow)) {
			return ErrInvalid
		}
	} else {
		if s.Actor != 0 || !s.Deadline.IsZero() {
			return ErrInvalid
		}
		for _, p := range s.Players {
			if p.TotalCommitted != 0 || p.StreetCommitted != 0 || p.Pending {
				return ErrInvalid
			}
		}
	}
	dealt := 0
	for i, event := range s.Events {
		if event.Sequence != uint64(i+1) || event.Version != uint64(i+1) || event.At.IsZero() || !wholeUnits(event.Delta) || !wholeUnits(event.To) {
			return ErrInvalid
		}
		if event.Type == "CARD_DEALT" {
			if event.DeckIndex == nil || event.Card == nil || *event.DeckIndex != dealt || dealt >= s.DeckCursor || *event.Card != s.Config.Deck[dealt] {
				return ErrInvalid
			}
			if dealt < 2*len(s.Players) {
				if event.Visibility != "PRIVATE" || event.Seat != s.Players[dealt%len(s.Players)].SeatNo {
					return ErrInvalid
				}
			} else if event.Visibility != "PUBLIC" || event.Seat != 0 {
				return ErrInvalid
			}
			dealt++
		}
	}
	if dealt != s.DeckCursor {
		return ErrInvalid
	}
	seenActions := map[string]bool{}
	for _, r := range s.Receipts {
		if r.Action.ID == "" || !wholeUnits(r.Action.To) || seenActions[r.Action.ID] || r.FirstEvent < 0 || r.LastEvent <= r.FirstEvent || r.LastEvent > len(s.Events) || s.Events[r.FirstEvent].ActionID != r.Action.ID {
			return ErrInvalid
		}
		seenActions[r.Action.ID] = true
	}
	if s.Config.ButtonSelection != nil {
		if err := validateButtonChoice(*s.Config.ButtonSelection, s.seats()); err != nil {
			return err
		}
	}
	if s.Street == Settled {
		var awards, returns, commits int64
		for _, event := range s.Events {
			switch event.Type {
			case "POST_SB", "POST_BB", "CALL", "BET", "RAISE", "ALL_IN":
				commits += event.Delta
			}
		}
		for _, pot := range s.Pots {
			var sum int64
			for _, a := range pot.Awards {
				if !wholeUnits(a.Amount) || !wholeUnits(a.Base) || (a.Odd != 0 && a.Odd != UnitsPerChip) || a.Amount != a.Base+a.Odd || s.player(a.Seat) == nil || s.player(a.Seat).Folded {
					return ErrInvalid
				}
				sum += a.Amount
			}
			if sum != pot.Amount || pot.Amount <= 0 || !wholeUnits(pot.Amount) || !wholeUnits(pot.Floor) || !wholeUnits(pot.Ceiling) {
				return ErrInvalid
			}
			awards += sum
		}
		for _, r := range s.Returns {
			if r.Amount <= 0 || !wholeUnits(r.Amount) || s.player(r.Seat) == nil {
				return ErrInvalid
			}
			returns += r.Amount
		}
		if commits != awards+returns {
			return fmt.Errorf("%w: settlement conservation", ErrInvalid)
		}
	}
	return s.validateAccounting()
}

// Reconcile each stack against the persisted event ledger, not merely the table
// sum; a corrupt snapshot that shifts chips between seats must be rejected.
func (s *State) validateAccounting() error {
	paid, returned, awarded := map[int]int64{}, map[int]int64{}, map[int]int64{}
	for _, event := range s.Events {
		if event.Delta < 0 {
			return ErrInvalid
		}
		var ledger map[int]int64
		switch event.Type {
		case "POST_SB", "POST_BB", "CALL", "BET", "RAISE", "ALL_IN":
			ledger = paid
		case "RETURN_UNCALLED":
			ledger = returned
		case "POT_AWARD":
			ledger = awarded
		}
		if ledger != nil {
			if s.player(event.Seat) == nil || event.Delta > s.InitialTotal-ledger[event.Seat] {
				return ErrInvalid
			}
			ledger[event.Seat] += event.Delta
		}
	}
	for _, p := range s.Players {
		if paid[p.SeatNo] > p.InitialStack || returned[p.SeatNo] > paid[p.SeatNo] {
			return ErrInvalid
		}
		want := p.InitialStack - paid[p.SeatNo] + returned[p.SeatNo]
		if awarded[p.SeatNo] > s.InitialTotal-want || p.Stack != want+awarded[p.SeatNo] {
			return fmt.Errorf("%w: seat stack ledger mismatch", ErrInvalid)
		}
		if s.Street != Settled && (p.TotalCommitted != paid[p.SeatNo]-returned[p.SeatNo] || returned[p.SeatNo] != 0 || awarded[p.SeatNo] != 0) {
			return ErrInvalid
		}
	}
	if s.Street != Settled {
		if len(s.Pots) != 0 || len(s.Returns) != 0 || len(s.Ranks) != 0 {
			return ErrInvalid
		}
		return nil
	}
	if len(s.Events) == 0 || s.Events[len(s.Events)-1].Type != "SYSTEM_SETTLEMENT" {
		return ErrInvalid
	}
	for _, r := range s.Ranks {
		if err := validateRank(r, s.Config.EvaluatorVersion); err != nil {
			return err
		}
	}
	potAwards, recordedReturns := map[int]int64{}, map[int]int64{}
	previous := int64(0)
	for index, pot := range s.Pots {
		if pot.Index != index || pot.Floor != previous || pot.Ceiling <= pot.Floor {
			return ErrInvalid
		}
		var eligible []int
		amount := int64(0)
		for _, p := range s.Players {
			if paid[p.SeatNo]-returned[p.SeatNo] >= pot.Ceiling {
				if pot.Ceiling-pot.Floor > s.InitialTotal-amount {
					return ErrInvalid
				}
				amount += pot.Ceiling - pot.Floor
				if !p.Folded {
					eligible = append(eligible, p.SeatNo)
				}
			}
		}
		if len(eligible) != len(pot.Eligible) || len(eligible) == 0 || amount != pot.Amount {
			return ErrInvalid
		}
		for i, seat := range eligible {
			if pot.Eligible[i] != seat {
				return ErrInvalid
			}
		}
		seen := map[int]bool{}
		for _, a := range pot.Awards {
			found := false
			for _, seat := range eligible {
				if seat == a.Seat {
					found = true
				}
				if compareRanks(s.Ranks[seat].Vector, s.Ranks[a.Seat].Vector) > 0 {
					return ErrInvalid
				}
			}
			if !found || seen[a.Seat] || a.Amount > s.InitialTotal-potAwards[a.Seat] {
				return ErrInvalid
			}
			seen[a.Seat] = true
			potAwards[a.Seat] += a.Amount
		}
		previous = pot.Ceiling
	}
	for _, r := range s.Returns {
		if r.Amount > s.InitialTotal-recordedReturns[r.Seat] {
			return ErrInvalid
		}
		recordedReturns[r.Seat] += r.Amount
	}
	for _, p := range s.Players {
		if potAwards[p.SeatNo] != awarded[p.SeatNo] || recordedReturns[p.SeatNo] != returned[p.SeatNo] {
			return ErrInvalid
		}
	}
	return nil
}
