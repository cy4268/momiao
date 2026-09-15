// Package blackjack implements the frozen IS-06 sections 277-287 pure rules.
// All mutation is copy-on-success. The caller owns transactions, action-result
// idempotency, UUIDs, protected persistence, the DB clock, and ledger application.
package blackjack

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"time"
)

type Phase string

const (
	PlayerTurn Phase = "PLAYER_TURN"
	Settled    Phase = "SETTLED"
)

type HandStatus string

const (
	Active            HandStatus = "ACTIVE"
	Stood             HandStatus = "STOOD"
	Bust              HandStatus = "BUST"
	DoubledComplete   HandStatus = "DOUBLED_COMPLETE"
	SplitAcesComplete HandStatus = "SPLIT_ACES_COMPLETE"
	NaturalComplete   HandStatus = "NATURAL_COMPLETE"
)

type ActionType string

const (
	Hit             ActionType = "HIT"
	Stand           ActionType = "STAND"
	Double          ActionType = "DOUBLE"
	Split           ActionType = "SPLIT"
	SystemAutoStand ActionType = "SYSTEM_AUTO_STAND"
)

var (
	ErrNeedsReview      = errors.New("BLACKJACK_NEEDS_REVIEW")
	ErrWager            = errors.New("BLACKJACK_INVALID_WAGER")
	ErrOverflow         = errors.New("BLACKJACK_INTEGER_OVERFLOW")
	ErrStaleVersion     = errors.New("BLACKJACK_STALE_ROUND_VERSION")
	ErrHandNotActive    = errors.New("BLACKJACK_HAND_NOT_ACTIVE")
	ErrActionNotAllowed = errors.New("BLACKJACK_ACTION_NOT_ALLOWED")
	ErrDuplicateAction  = errors.New("BLACKJACK_DUPLICATE_ACTION")
	ErrMaxHands         = errors.New("BLACKJACK_MAX_HANDS_REACHED")
	ErrDoubleBalance    = errors.New("BLACKJACK_INSUFFICIENT_CHIPS_FOR_DOUBLE")
	ErrSplitBalance     = errors.New("BLACKJACK_INSUFFICIENT_CHIPS_FOR_SPLIT")
)

type Hand struct {
	ID string
	// Stable sparse ordering key, not the mutable ordinal in Hands. Starting at
	// 0, midpoint insertion within [0,8) accommodates all three possible splits.
	Index          int
	ParentID       string
	Cards          []Card
	StakeUnits     int64
	FromSplit      bool
	SplitAces      bool
	Natural        bool
	Status         HandStatus
	Value          Value
	Result         string
	PayoutUnits    int64
	NetChangeUnits int64
}

type Action struct {
	ID              string
	ExpectedVersion int64
	HandID          string
	Type            ActionType
	// Required fresh ID for the right hand on Split; left retains its ID/index.
	NewHandID string
}
type ActionRecord struct {
	Action               Action
	Sequence             int
	At                   time.Time
	AdditionalStakeUnits int64
}

// State is protected server authority, never a browser/log/audit projection.
// A shoe is passed separately and is deliberately not stored in this struct.
type State struct {
	InitialHandID      string
	CreatedAt          time.Time
	InitialWagerUnits  int64
	ShoeHash           string
	ShoeIndex          int
	Dealt              []Card
	Dealer             []Card
	DealerRevealed     bool
	Hands              []Hand // left-to-right; the order is retained across recovery
	ActiveHandID       string
	Phase              Phase
	Version            int64
	Actions            []ActionRecord
	LastPlayerActionAt time.Time
	AutoResolveAt      time.Time
	TotalStakeUnits    int64
	TotalPayoutUnits   int64
	FairReturnUnits    int64
	NetChangeUnits     int64
	Class              string
}
type Transition struct {
	State                State
	AdditionalStakeUnits int64
}

// Explicitly require PublicView for transport; typed protected storage can use
// the fields directly. Accidental JSON or ordinary formatted logs are redacted.
func (State) MarshalJSON() ([]byte, error) {
	return nil, errors.New("BLACKJACK_PRIVATE_AUTHORITY_USE_PUBLIC_VIEW")
}
func (State) String() string   { return "blackjack.State<private authority>" }
func (State) GoString() string { return "blackjack.State<private authority>" }

func canonicalTime(t time.Time) time.Time { return t.UTC().Round(0) }

func New(shoe [312]uint16, wagerUnits int64, initialHandID string, now time.Time) (State, error) {
	if wagerUnits < 10*UnitsPerChip || wagerUnits%UnitsPerChip != 0 {
		return State{}, ErrWager
	}
	// Four split hands, each doubled and paid at 2x, is the worst legal future
	// payout (16x initial). Reject before accepting a wager, not mid-round.
	if wagerUnits > math.MaxInt64/16 {
		return State{}, ErrOverflow
	}
	if initialHandID == "" || now.IsZero() {
		return State{}, ErrActionNotAllowed
	}
	hash, e := ShoeHash(shoe)
	if e != nil {
		return State{}, e
	}
	now = canonicalTime(now)
	s := State{InitialHandID: initialHandID, CreatedAt: now, InitialWagerUnits: wagerUnits, ShoeHash: hash, Phase: PlayerTurn, Version: 1, TotalStakeUnits: wagerUnits,
		Hands: []Hand{{ID: initialHandID, Index: 0, StakeUnits: wagerUnits, Status: Active}}}
	// American hole-card dealing: P1, dealer up, P2, dealer hole.
	for i := 0; i < 4; i++ {
		card, _ := s.draw(shoe)
		if i%2 == 0 {
			s.Hands[0].Cards = append(s.Hands[0].Cards, card)
		} else {
			s.Dealer = append(s.Dealer, card)
		}
	}
	h := &s.Hands[0]
	h.revalue()
	h.Natural = h.Value.BestTotal == 21
	if h.Natural {
		h.Status = NaturalComplete
	}
	dealer, _ := Evaluate(s.Dealer)
	// A dealer natural necessarily has an A/ten upcard; peek always precedes
	// action. A player natural also settles without unnecessary dealer draws.
	if dealer.BestTotal == 21 || h.Natural {
		if !h.Natural {
			h.Status = Stood
		}
		s.settle()
		return s, nil
	}
	s.ActiveHandID = h.ID
	s.LastPlayerActionAt = now
	s.AutoResolveAt = now.Add(24 * time.Hour)
	return s, nil
}

func (s State) clone() State {
	out := s
	out.CreatedAt = canonicalTime(s.CreatedAt)
	out.LastPlayerActionAt = canonicalTime(s.LastPlayerActionAt)
	out.AutoResolveAt = canonicalTime(s.AutoResolveAt)
	out.Dealt = append([]Card(nil), s.Dealt...)
	out.Dealer = append([]Card(nil), s.Dealer...)
	out.Hands = append([]Hand(nil), s.Hands...)
	for i := range out.Hands {
		out.Hands[i].Cards = append([]Card(nil), s.Hands[i].Cards...)
	}
	out.Actions = append([]ActionRecord(nil), s.Actions...)
	for i := range out.Actions {
		out.Actions[i].At = canonicalTime(out.Actions[i].At)
	}
	return out
}
func (h *Hand) revalue() { h.Value, _ = Evaluate(h.Cards) }
func (s *State) draw(shoe [312]uint16) (Card, error) {
	if s.ShoeIndex < 0 || s.ShoeIndex >= len(shoe) {
		return Card{}, ErrNeedsReview
	}
	c := Card{ShoeIndex: s.ShoeIndex, InstanceID: shoe[s.ShoeIndex]}
	s.ShoeIndex++
	s.Dealt = append(s.Dealt, c)
	return c, nil
}
func (s State) activeIndex() int {
	for i, h := range s.Hands {
		if h.ID == s.ActiveHandID && h.Status == Active {
			return i
		}
	}
	return -1
}

// LegalActions uses the current available wallet units after all previous
// debits. Call after recovery validation / on an engine-returned state only.
func LegalActions(s State, availableUnits int64) []ActionType {
	i := s.activeIndex()
	if s.Phase != PlayerTurn || i < 0 {
		return nil
	}
	h := s.Hands[i]
	result := []ActionType{Hit, Stand}
	if len(h.Cards) == 2 && !h.SplitAces && availableUnits >= h.StakeUnits {
		result = append(result, Double)
		if len(s.Hands) < MaxHands && point(h.Cards[0].InstanceID) == point(h.Cards[1].InstanceID) {
			result = append(result, Split)
		}
	}
	return result
}

func Apply(s State, shoe [312]uint16, a Action, availableUnits int64, now time.Time) (Transition, error) {
	if e := VerifyRecovery(s, shoe); e != nil {
		return Transition{}, e
	}
	next := s.clone()
	additional, e := next.apply(shoe, a, availableUnits, canonicalTime(now), false)
	if e != nil {
		return Transition{}, e
	}
	return Transition{State: next, AdditionalStakeUnits: additional}, nil
}

// apply mutates only a private copy / freshly replayed state. It never performs
// an external effect; any failure discards this copy at the public boundary.
func (s *State) apply(shoe [312]uint16, a Action, availableUnits int64, now time.Time, system bool) (int64, error) {
	for _, record := range s.Actions {
		if record.Action.ID == a.ID {
			return 0, ErrDuplicateAction
		}
	}
	if a.ID == "" || now.IsZero() || now.Before(s.LastPlayerActionAt) {
		return 0, ErrActionNotAllowed
	}
	if a.ExpectedVersion != s.Version {
		return 0, ErrStaleVersion
	}
	i := s.activeIndex()
	if s.Phase != PlayerTurn || i < 0 || a.HandID != s.ActiveHandID {
		return 0, ErrHandNotActive
	}
	if system {
		if a.Type != SystemAutoStand || now.Before(s.AutoResolveAt) {
			return 0, ErrActionNotAllowed
		}
	} else if a.Type != Hit && a.Type != Stand && a.Type != Double && a.Type != Split {
		return 0, ErrActionNotAllowed
	}
	h := &s.Hands[i]
	var additional int64
	switch a.Type {
	case Hit:
		c, e := s.draw(shoe)
		if e != nil {
			return 0, e
		}
		h.Cards = append(h.Cards, c)
		h.revalue()
		if h.Value.BestTotal > 21 {
			h.Status = Bust
		} else if h.Value.BestTotal == 21 {
			h.Status = Stood
		}
	case Stand, SystemAutoStand:
		h.Status = Stood
	case Double:
		if len(h.Cards) != 2 || h.SplitAces {
			return 0, ErrActionNotAllowed
		}
		if availableUnits < h.StakeUnits {
			return 0, ErrDoubleBalance
		}
		additional = h.StakeUnits
		h.StakeUnits *= 2
		c, e := s.draw(shoe)
		if e != nil {
			return 0, e
		}
		h.Cards = append(h.Cards, c)
		h.revalue()
		h.Status = DoubledComplete
	case Split:
		if len(h.Cards) != 2 || h.SplitAces || point(h.Cards[0].InstanceID) != point(h.Cards[1].InstanceID) {
			return 0, ErrActionNotAllowed
		}
		if len(s.Hands) >= MaxHands {
			return 0, ErrMaxHands
		}
		if availableUnits < h.StakeUnits {
			return 0, ErrSplitBalance
		}
		if a.NewHandID == "" {
			return 0, ErrActionNotAllowed
		}
		for _, other := range s.Hands {
			if other.ID == a.NewHandID {
				return 0, ErrActionNotAllowed
			}
		}
		additional = h.StakeUnits
		rightBoundary := 8
		if i+1 < len(s.Hands) {
			rightBoundary = s.Hands[i+1].Index
		}
		rightIndex := (h.Index + rightBoundary) / 2
		if rightIndex <= h.Index {
			return 0, ErrNeedsReview
		}
		aces := point(h.Cards[0].InstanceID) == 1
		right := Hand{ID: a.NewHandID, Index: rightIndex, ParentID: h.ID, StakeUnits: h.StakeUnits, FromSplit: true, SplitAces: aces, Status: Active, Cards: []Card{h.Cards[1]}}
		h.Cards = h.Cards[:1:1]
		h.FromSplit = true
		h.SplitAces = aces
		h.Natural = false
		leftCard, e := s.draw(shoe)
		if e != nil {
			return 0, e
		}
		rightCard, e := s.draw(shoe)
		if e != nil {
			return 0, e
		}
		h.Cards = append(h.Cards, leftCard)
		right.Cards = append(right.Cards, rightCard)
		h.revalue()
		right.revalue()
		if aces {
			h.Status = SplitAcesComplete
			right.Status = SplitAcesComplete
		}
		s.Hands = append(s.Hands, Hand{})
		copy(s.Hands[i+2:], s.Hands[i+1:])
		s.Hands[i+1] = right
	}
	s.TotalStakeUnits += additional
	s.Version++
	s.Actions = append(s.Actions, ActionRecord{Action: a, Sequence: len(s.Actions) + 1, At: now, AdditionalStakeUnits: additional})
	if !system {
		s.LastPlayerActionAt = now
		s.AutoResolveAt = now.Add(24 * time.Hour)
	}
	if e := s.advance(shoe); e != nil {
		return 0, e
	}
	return additional, nil
}

func (s *State) advance(shoe [312]uint16) error {
	for _, h := range s.Hands {
		if h.Status == Active {
			s.ActiveHandID = h.ID
			return nil
		}
	}
	s.ActiveHandID = ""
	s.DealerRevealed = true
	allBust := true
	for _, h := range s.Hands {
		if h.Value.BestTotal <= 21 {
			allBust = false
			break
		}
	}
	if !allBust {
		for {
			v, _ := Evaluate(s.Dealer)
			if v.BestTotal >= 17 {
				break
			}
			c, e := s.draw(shoe)
			if e != nil {
				return e
			}
			s.Dealer = append(s.Dealer, c)
		}
	}
	s.settle()
	return nil
}

func (s *State) settle() {
	dealer, _ := Evaluate(s.Dealer)
	dealerNatural := len(s.Dealer) == 2 && dealer.BestTotal == 21
	s.DealerRevealed = true
	s.ActiveHandID = ""
	s.Phase = Settled
	s.TotalPayoutUnits = 0
	for i := range s.Hands {
		h := &s.Hands[i]
		h.PayoutUnits = 0
		switch {
		case h.Value.BestTotal > 21:
			h.Result = "BUST"
		case dealerNatural && h.Natural:
			h.Result = "PUSH"
			h.PayoutUnits = h.StakeUnits
		case dealerNatural:
			h.Result = "LOSS"
		case h.Natural:
			h.Result = "NATURAL"
			h.PayoutUnits = (h.StakeUnits / 2) * 5
		case dealer.BestTotal > 21 || h.Value.BestTotal > dealer.BestTotal:
			h.Result = "NORMAL_WIN"
			h.PayoutUnits = h.StakeUnits * 2
		case h.Value.BestTotal == dealer.BestTotal:
			h.Result = "PUSH"
			h.PayoutUnits = h.StakeUnits
		default:
			h.Result = "LOSS"
		}
		h.NetChangeUnits = h.PayoutUnits - h.StakeUnits
		s.TotalPayoutUnits += h.PayoutUnits
	}
	s.NetChangeUnits = s.TotalPayoutUnits - s.TotalStakeUnits
	switch {
	case s.NetChangeUnits < 0:
		s.Class = "LOSS"
	case s.NetChangeUnits == 0:
		s.Class = "BREAK_EVEN"
	default:
		s.Class = "WIN"
	}
}

// AutoResolve is a no-op before expiry or after settlement. At expiry each
// remaining hand receives one recorded SYSTEM_AUTO_STAND, left-to-right. It
// never advances the manual inactivity anchor, refunds, debits, or reshuffles.
// systemActionID supplies transaction-owned UUIDs; no callback is needed or
// invoked for a no-op. Callback errors discard every intermediate change.
func AutoResolve(s State, shoe [312]uint16, now time.Time, systemActionID func(handID string) (string, error)) (Transition, error) {
	if e := VerifyRecovery(s, shoe); e != nil {
		return Transition{}, e
	}
	next := s.clone()
	now = canonicalTime(now)
	if next.Phase != PlayerTurn || now.Before(next.AutoResolveAt) {
		return Transition{State: next}, nil
	}
	if systemActionID == nil {
		return Transition{}, ErrActionNotAllowed
	}
	for next.Phase == PlayerTurn {
		id, e := systemActionID(next.ActiveHandID)
		if e != nil {
			return Transition{}, e
		}
		a := Action{ID: id, ExpectedVersion: next.Version, HandID: next.ActiveHandID, Type: SystemAutoStand}
		if _, e := next.apply(shoe, a, 0, now, true); e != nil {
			return Transition{}, e
		}
	}
	return Transition{State: next}, nil
}

// VerifyRecovery requires the independently rebuilt, same-seed shoe. It checks
// its hash, every consumed index/instance/recipient, all hand identities/order,
// actions, stakes, inactivity anchors and results by complete deterministic
// replay. On error the transaction layer must mark NEEDS_REVIEW and not deal.
func VerifyRecovery(s State, shoe [312]uint16) error {
	hash, e := ShoeHash(shoe)
	if e != nil || hash != s.ShoeHash || len(s.Actions) > 320 {
		return ErrNeedsReview
	}
	replay, e := New(shoe, s.InitialWagerUnits, s.InitialHandID, s.CreatedAt)
	if e != nil {
		return ErrNeedsReview
	}
	for _, record := range s.Actions {
		if _, e = replay.apply(shoe, record.Action, math.MaxInt64, record.At, record.Action.Type == SystemAutoStand); e != nil {
			return ErrNeedsReview
		}
	}
	// PostgreSQL drivers can restore equivalent timestamp locations and empty
	// slices differently. Normalize representation, never values or ordering.
	if !reflect.DeepEqual(s.clone(), replay.clone()) {
		return ErrNeedsReview
	}
	return nil
}

// PublicHand contains canonical card codes (0..51), never deck-copy IDs.
type PublicHand struct {
	ID             string     `json:"hand_id"`
	Index          int        `json:"hand_index"`
	Cards          []uint16   `json:"cards"`
	StakeUnits     int64      `json:"stake_units,string"`
	Status         HandStatus `json:"hand_state"`
	Value          Value      `json:"value"`
	Natural        bool       `json:"is_natural"`
	Result         string     `json:"result,omitempty"`
	PayoutUnits    int64      `json:"payout_units,string"`
	NetChangeUnits int64      `json:"net_change_units,string"`
}
type Projection struct {
	Phase              Phase        `json:"phase"`
	Version            int64        `json:"round_version,string"`
	ActiveHandID       string       `json:"active_hand_id"`
	Hands              []PublicHand `json:"hands"`
	DealerCards        []uint16     `json:"dealer_cards"`
	DealerRevealed     bool         `json:"dealer_revealed"`
	DealerTotal        *Value       `json:"dealer_total,omitempty"`
	LegalActions       []ActionType `json:"legal_actions"`
	LastPlayerActionAt time.Time    `json:"last_player_action_at"`
	AutoResolveAt      time.Time    `json:"auto_resolve_at"`
	TotalStakeUnits    int64        `json:"total_stake_units,string"`
	TotalPayoutUnits   int64        `json:"total_payout_units,string"`
	FairReturnUnits    int64        `json:"fair_return_units,string"`
	NetChangeUnits     int64        `json:"net_change_units,string"`
	Class              string       `json:"result_class,omitempty"`
}

// PublicView is an independent safe snapshot. Amounts and round_version use
// decimal JSON strings. The service adds its common round/fairness projection.
func PublicView(s State, availableUnits int64) Projection {
	p := Projection{Phase: s.Phase, Version: s.Version, ActiveHandID: s.ActiveHandID, DealerRevealed: s.DealerRevealed, LegalActions: LegalActions(s, availableUnits), LastPlayerActionAt: s.LastPlayerActionAt, AutoResolveAt: s.AutoResolveAt, TotalStakeUnits: s.TotalStakeUnits, TotalPayoutUnits: s.TotalPayoutUnits, FairReturnUnits: s.FairReturnUnits, NetChangeUnits: s.NetChangeUnits, Class: s.Class}
	for _, h := range s.Hands {
		ph := PublicHand{ID: h.ID, Index: h.Index, StakeUnits: h.StakeUnits, Status: h.Status, Value: h.Value, Natural: h.Natural, Result: h.Result, PayoutUnits: h.PayoutUnits, NetChangeUnits: h.NetChangeUnits}
		for _, c := range h.Cards {
			ph.Cards = append(ph.Cards, c.InstanceID%52)
		}
		p.Hands = append(p.Hands, ph)
	}
	for i, c := range s.Dealer {
		if i > 0 && !s.DealerRevealed {
			break
		}
		p.DealerCards = append(p.DealerCards, c.InstanceID%52)
	}
	if s.DealerRevealed {
		v, _ := Evaluate(s.Dealer)
		p.DealerTotal = &v
	}
	return p
}

// GoString redacts the transition's nested authority even under %#v formatting.
func (Transition) GoString() string { return "blackjack.Transition<private authority>" }
func (t Transition) String() string {
	return fmt.Sprintf("blackjack.Transition<additional_stake=%d>", t.AdditionalStakeUnits)
}
