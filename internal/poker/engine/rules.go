package engine

import "sort"

const ButtonSelectionVersion = "poker-initial-button-v1"

type ButtonChoice struct {
	Version       string `json:"version"`
	EligibleSeats []int  `json:"eligible_seats"`
	DrawIndex     int    `json:"draw_index"`
	Seat          int    `json:"seat_no"`
}

// SelectInitialButton records a choice from a caller-provided unbiased [0,n)
// sampler. Randomness generation, commitment and proof belong to fairness.
func SelectInitialButton(seats []int, uniform func(int) (int, error)) (ButtonChoice, error) {
	ordered, err := orderedSeats(seats)
	if err != nil || uniform == nil {
		return ButtonChoice{}, ErrInvalid
	}
	i, err := uniform(len(ordered))
	if err != nil {
		return ButtonChoice{}, err
	}
	if i < 0 || i >= len(ordered) {
		return ButtonChoice{}, ErrInvalid
	}
	return ButtonChoice{Version: ButtonSelectionVersion, EligibleSeats: ordered, DrawIndex: i, Seat: ordered[i]}, nil
}

// NextButton moves clockwise across a frozen set of eligible seats. Admission
// and WAIT_FOR_BB eligibility remain the table service's explicit responsibility.
func NextButton(previous int, seats []int) (int, error) {
	ordered, err := orderedSeats(seats)
	if err != nil || previous < 1 || previous > 9 {
		return 0, ErrInvalid
	}
	for _, seat := range ordered {
		if seat > previous {
			return seat, nil
		}
	}
	return ordered[0], nil
}

func orderedSeats(seats []int) ([]int, error) {
	if len(seats) < 2 || len(seats) > 9 {
		return nil, ErrInvalid
	}
	out := append([]int{}, seats...)
	sort.Ints(out)
	for i, s := range out {
		if s < 1 || s > 9 || (i > 0 && out[i-1] == s) {
			return nil, ErrInvalid
		}
	}
	return out, nil
}

func validateButtonChoice(choice ButtonChoice, seats []int) error {
	if choice.Version != ButtonSelectionVersion || len(choice.EligibleSeats) != len(seats) || choice.DrawIndex < 0 || choice.DrawIndex >= len(seats) {
		return ErrInvalid
	}
	for i, s := range seats {
		if choice.EligibleSeats[i] != s {
			return ErrInvalid
		}
	}
	if choice.Seat != seats[choice.DrawIndex] {
		return ErrInvalid
	}
	return nil
}

type Shortcut struct {
	Name string     `json:"name"`
	Kind ActionKind `json:"action_type"`
	To   int64      `json:"target_to_units"`
}

// ShortcutTargets implements the 2026-09-06 approved integer target-to formula:
// B + UnitsPerChip*floor(r*(P+max(0,B-C))/UnitsPerChip). All public amounts
// are Atomic Units. It never substitutes a call for an illegal raise.
// ALL_IN shortcuts carry target-to for display; Action.To must still be zero.
func (e *Engine) ShortcutTargets() []Shortcut {
	l := e.LegalActions()
	if !l.RaiseRights {
		return nil
	}
	minimum := l.MinimumRaiseTo
	kind := Raise
	if e.state.CurrentBet == 0 {
		minimum = l.MinimumBet
		kind = Bet
	}
	makeTarget := func(name string, target int64) Shortcut {
		target = min(max(target, minimum), l.MaximumRaiseTo)
		k := kind
		if target < minimum || target == l.MaximumRaiseTo {
			k = AllIn
		}
		return Shortcut{Name: name, Kind: k, To: target}
	}
	out := []Shortcut{makeTarget("Min", minimum)}
	potAfterCallChips := addCap(l.CurrentPot, l.ToCall) / UnitsPerChip
	for _, fraction := range []struct {
		name string
		n, d int64
	}{{"1/2 Pot", 1, 2}, {"2/3 Pot", 2, 3}, {"Pot", 1, 1}} {
		// Divide before multiply; numerator<=denominator avoids integer overflow.
		portionChips := potAfterCallChips/fraction.d*fraction.n + (potAfterCallChips%fraction.d)*fraction.n/fraction.d
		out = append(out, makeTarget(fraction.name, addCap(e.state.CurrentBet, portionChips*UnitsPerChip)))
	}
	return append(out, Shortcut{Name: "All-in", Kind: AllIn, To: l.MaximumRaiseTo})
}
