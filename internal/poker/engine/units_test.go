package engine_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	. "github.com/cy4268/momiao/internal/poker/engine"
)

// The literal is the frozen IS §316 atomic-unit contract, not engine-local scale.
const atomicPerChip int64 = 500_000

func atomicHand(t *testing.T, stacks []int64, frozen []Card) *Engine {
	t.Helper()
	seats := make([]Seat, len(stacks))
	for i, stack := range stacks {
		seats[i] = Seat{SeatNo: i + 1, PlayerID: string(rune('a' + i)), Stack: stack}
	}
	e, err := New(Config{HandID: "atomic-units", ButtonSeat: 1, SmallBlind: 2_500_000, BigBlind: 5_000_000, Deck: frozen, EvaluatorVersion: EvaluatorVersion}, seats, epoch, Evaluate)
	if err != nil {
		t.Fatalf("real 5/10 table atomic-unit config rejected: %v", err)
	}
	return e
}

func atomicAction(t *testing.T, e *Engine, kind ActionKind, to int64) {
	t.Helper()
	s := e.State()
	if _, err := e.Apply(Action{ID: fmt.Sprintf("units-%d", s.Version), Seat: s.Actor, ExpectedVersion: s.Version, Kind: kind, To: to}, s.StartedAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
}

func TestAtomicUnitsRealPresetAndFractionalBoundaries(t *testing.T) {
	e := atomicHand(t, []int64{100 * atomicPerChip, 100 * atomicPerChip, 100 * atomicPerChip}, deck())
	if e.State().Config.SmallBlind != 2_500_000 || e.LegalActions().ToCall != 5_000_000 {
		t.Fatal("public amounts are not atomic units")
	}
	for _, stack := range []int64{7, 100*atomicPerChip + 1} {
		cfg := e.State().Config
		if _, err := New(cfg, []Seat{{SeatNo: 1, PlayerID: "a", Stack: stack}, {SeatNo: 2, PlayerID: "b", Stack: 100 * atomicPerChip}}, epoch, Evaluate); err == nil {
			t.Fatal("fractional-chip stack accepted")
		}
	}
	s := e.State()
	before, _ := e.Snapshot()
	if _, err := e.Apply(Action{ID: "fractional-raise", Seat: s.Actor, ExpectedVersion: s.Version, Kind: Raise, To: 20*atomicPerChip + 1}, s.StartedAt.Add(time.Second)); err == nil {
		t.Fatal("single atomic remainder raise accepted")
	}
	after, _ := e.Snapshot()
	if !bytes.Equal(before, after) {
		t.Fatal("rejected fractional action mutated state")
	}
	// Keep every ledger equality valid; only the frozen whole-Chip granularity
	// should reject this otherwise self-consistent recovered snapshot.
	s.Players[0].Stack++
	s.Players[0].InitialStack++
	s.InitialTotal++
	data, _ := json.Marshal(s)
	if _, err := Restore(data, Evaluate); err == nil {
		t.Fatal("fractional recovered stack accepted")
	}
}

func TestAtomicUnitsFifteenChipTieSplitsSevenEight(t *testing.T) {
	d := deck(cards("2C", "3C", "4C", "5C", "6C", "7C", "AS", "KS", "QS", "JS", "TS")...)
	e := atomicHand(t, []int64{5 * atomicPerChip, 100 * atomicPerChip, 100 * atomicPerChip}, d)
	atomicAction(t, e, AllIn, 0)
	atomicAction(t, e, Fold, 0)
	s := e.State()
	if s.Street != Settled || len(s.Pots) != 1 || s.Pots[0].Amount != 15*atomicPerChip {
		t.Fatal("expected one 15-Chip pot")
	}
	a := s.Pots[0].Awards
	if len(a) != 2 || a[0].Seat != 3 || a[0].Amount != 8*atomicPerChip || a[0].Base != 7*atomicPerChip || a[0].Odd != atomicPerChip || a[1].Amount != 7*atomicPerChip || a[1].Odd != 0 {
		t.Fatalf("want 7/8 whole-Chip split, got %+v", a)
	}
	if _, err := Restore(mustSnapshot(t, e), Evaluate); err != nil {
		t.Fatal(err)
	}
}

func TestAtomicUnitsHalfOf101ChipPotFloorsWholeChip(t *testing.T) {
	e := atomicHand(t, []int64{100 * atomicPerChip, 100 * atomicPerChip, 33 * atomicPerChip}, deck())
	atomicAction(t, e, Raise, 34*atomicPerChip)
	atomicAction(t, e, Call, 0)
	atomicAction(t, e, AllIn, 0)
	if e.State().Street != Flop || e.LegalActions().CurrentPot != 101*atomicPerChip || e.LegalActions().ToCall != 0 {
		t.Fatal("expected 101-Chip pot after call")
	}
	for _, target := range e.ShortcutTargets() {
		if target.To%atomicPerChip != 0 {
			t.Fatalf("fractional-Chip target: %+v", target)
		}
		if target.Name == "1/2 Pot" && target.To != 50*atomicPerChip {
			t.Fatalf("half should floor to 50 Chips: %+v", target)
		}
	}
}

func mustSnapshot(t *testing.T, e *Engine) []byte {
	t.Helper()
	data, err := e.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return data
}
