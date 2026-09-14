package engine_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	. "github.com/cy4268/momiao/internal/poker/engine"
)

var epoch = time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)

// Test scenarios are authored in Chip counts, then cross the public API in
// Atomic Units. No production serialization or storage adapter converts them.
func chips(n int64) int64 { return n * UnitsPerChip }

func deck(prefix ...Card) []Card {
	out := append([]Card{}, prefix...)
	seen := [52]bool{}
	for _, c := range prefix {
		if c >= 52 || seen[c] {
			panic("bad fixture deck")
		}
		seen[c] = true
	}
	for c := Card(0); c < 52; c++ {
		if !seen[c] {
			out = append(out, c)
		}
	}
	return out
}

func hand(t *testing.T, stacks []int64, button int, frozen []Card) *Engine {
	t.Helper()
	seats := make([]Seat, len(stacks))
	for i, s := range stacks {
		seats[i] = Seat{SeatNo: i + 1, PlayerID: fmt.Sprint(i + 1), Stack: chips(s)}
	}
	e, err := New(Config{HandID: "test-hand", ButtonSeat: button, SmallBlind: chips(5), BigBlind: chips(10), Deck: frozen, EvaluatorVersion: EvaluatorVersion}, seats, epoch, Evaluate)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func act(t *testing.T, e *Engine, kind ActionKind, toChips int64) {
	t.Helper()
	actUnits(t, e, kind, chips(toChips))
}

func actUnits(t *testing.T, e *Engine, kind ActionKind, to int64) {
	t.Helper()
	s := e.State()
	_, err := e.Apply(Action{ID: fmt.Sprintf("a-%d", s.Version), Seat: s.Actor, ExpectedVersion: s.Version, Kind: kind, To: to}, s.StartedAt.Add(time.Second))
	if err != nil {
		t.Fatalf("%s seat %d: %v, legal=%+v", kind, s.Actor, err, e.LegalActions())
	}
	conserved(t, e)
}

func conserved(t *testing.T, e *Engine) {
	t.Helper()
	s := e.State()
	var total int64
	for _, p := range s.Players {
		if p.Stack < 0 || p.TotalCommitted < 0 {
			t.Fatal("negative chips")
		}
		if p.Stack%UnitsPerChip != 0 || p.InitialStack%UnitsPerChip != 0 || p.TotalCommitted%UnitsPerChip != 0 || p.StreetCommitted%UnitsPerChip != 0 {
			t.Fatal("fractional-Chip participant amount")
		}
		total += p.Stack + p.TotalCommitted
	}
	if total != s.InitialTotal {
		t.Fatalf("chips %d != %d", total, s.InitialTotal)
	}
	seen := map[Card]bool{}
	for _, p := range s.Players {
		for _, c := range p.Hole {
			if seen[c] {
				t.Fatal("duplicate hole card")
			}
			seen[c] = true
		}
	}
	for _, c := range s.Board {
		if seen[c] {
			t.Fatal("duplicate board card")
		}
		seen[c] = true
	}
	if len(seen) != s.DeckCursor {
		t.Fatalf("consumed cards %d != %d", len(seen), s.DeckCursor)
	}
	for i, event := range s.Events {
		if event.Sequence != uint64(i+1) {
			t.Fatal("event gap")
		}
		if event.Delta%UnitsPerChip != 0 || event.To%UnitsPerChip != 0 {
			t.Fatal("fractional-Chip event amount")
		}
	}
}

func TestDealGoldenPrefixAndTurnOrder(t *testing.T) {
	e := hand(t, []int64{100, 100, 100}, 1, deck(46, 25, 19, 38, 44, 28, 6, 16, 22, 8, 11, 1, 47, 23, 35))
	s := e.State()
	if s.Players[0].Hole != [2]Card{46, 38} || s.Players[1].Hole != [2]Card{25, 44} || s.Players[2].Hole != [2]Card{19, 28} {
		t.Fatal("not ascending seat two-pass deal")
	}
	if s.SmallBlindSeat != 2 || s.BigBlindSeat != 3 || s.Actor != 1 {
		t.Fatal("three-player order")
	}
	act(t, e, Call, 0)
	act(t, e, Call, 0)
	act(t, e, Check, 0)
	if e.State().Street != Flop || e.State().Actor != 2 || !reflect.DeepEqual(e.State().Board, []Card{6, 16, 22}) {
		t.Fatal("flop order/no-burn vector")
	}
	u := hand(t, []int64{100, 100}, 1, deck())
	if u.State().Actor != 1 || u.State().SmallBlindSeat != 1 {
		t.Fatal("heads-up button/SB first")
	}
	act(t, u, Call, 0)
	act(t, u, Check, 0)
	if u.State().Actor != 2 {
		t.Fatal("heads-up BB first postflop")
	}
}

func TestIllegalActionsLeaveSnapshotUnchangedAndDuplicateIDs(t *testing.T) {
	e := hand(t, []int64{100, 100, 100}, 1, deck())
	s := e.State()
	invalid := []Action{
		{ID: "x", Seat: 2, ExpectedVersion: s.Version, Kind: Call},
		{ID: "x", Seat: 1, ExpectedVersion: s.Version - 1, Kind: Call},
		{ID: "x", Seat: 1, ExpectedVersion: s.Version, Kind: Check},
		{ID: "x", Seat: 1, ExpectedVersion: s.Version, Kind: Raise, To: chips(19)},
		{ID: "x", Seat: 1, ExpectedVersion: s.Version, Kind: Raise, To: chips(101)},
		{ID: "x", Seat: 1, ExpectedVersion: s.Version, Kind: Call, To: chips(10)},
	}
	for _, a := range invalid {
		before, _ := e.Snapshot()
		if _, err := e.Apply(a, epoch.Add(time.Second)); err == nil {
			t.Fatalf("accepted %+v", a)
		}
		after, _ := e.Snapshot()
		if !bytes.Equal(before, after) {
			t.Fatal("invalid action mutated state")
		}
	}
	a := Action{ID: "valid", Seat: 1, ExpectedVersion: s.Version, Kind: Call}
	first, err := e.Apply(a, epoch.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	before, _ := e.Snapshot()
	second, err := e.Apply(a, epoch.Add(time.Second))
	if err != nil || !second.Duplicate || !reflect.DeepEqual(first.Events, second.Events) {
		t.Fatal("retry not idempotent")
	}
	after, _ := e.Snapshot()
	if !bytes.Equal(before, after) {
		t.Fatal("retry mutated")
	}
	a.Kind = Fold
	if _, err := e.Apply(a, epoch.Add(time.Second)); err == nil {
		t.Fatal("conflicting duplicate accepted")
	}
}

func TestShortAllInDoesNotReopenAndFullRaiseDoes(t *testing.T) {
	e := hand(t, []int64{100, 15, 100}, 1, deck())
	act(t, e, Call, 0)
	act(t, e, AllIn, 0)
	if !e.LegalActions().RaiseRights || e.LegalActions().MinimumRaiseTo != chips(25) {
		t.Fatal("unacted BB raise rights")
	}
	act(t, e, Call, 0)
	if e.State().Actor != 1 || e.LegalActions().RaiseRights || e.LegalActions().ToCall != chips(5) {
		t.Fatal("short all-in reopened caller")
	}
	before, _ := e.Snapshot()
	s := e.State()
	if _, err := e.Apply(Action{ID: "bad", Seat: 1, ExpectedVersion: s.Version, Kind: AllIn}, s.StartedAt); err == nil {
		t.Fatal("raise disguised as all-in")
	}
	after, _ := e.Snapshot()
	if !bytes.Equal(before, after) {
		t.Fatal("changed on bad all-in")
	}
	act(t, e, Call, 0)
	f := hand(t, []int64{100, 15, 100}, 1, deck())
	act(t, f, Call, 0)
	act(t, f, AllIn, 0)
	act(t, f, Raise, 25)
	if !f.LegalActions().RaiseRights || f.LegalActions().MinimumRaiseTo != chips(35) {
		t.Fatal("full raise did not reopen")
	}
}

func TestAllInSidePotsAndUncalledReturn(t *testing.T) {
	// Seat 2 AA wins main, seat 3 KK wins side, seat 1 QQ gets unmatched excess back.
	d := deck(cards("QC", "AC", "KC", "QD", "AD", "KD", "2H", "4S", "7C", "9D", "JH")...)
	e := hand(t, []int64{200, 50, 100}, 1, d)
	act(t, e, AllIn, 0)
	act(t, e, AllIn, 0)
	act(t, e, AllIn, 0)
	s := e.State()
	if s.Street != Settled || len(s.Board) != 5 || len(s.Pots) != 2 {
		t.Fatalf("not complete side-pot hand: %+v", s)
	}
	if s.Pots[0].Amount != chips(150) || s.Pots[1].Amount != chips(100) || s.Players[0].Stack != chips(100) || s.Players[1].Stack != chips(150) || s.Players[2].Stack != chips(100) {
		t.Fatal("side pot distribution")
	}
	if !reflect.DeepEqual(s.Returns, []Return{{Seat: 1, Amount: chips(100)}}) {
		t.Fatal(s.Returns)
	}
}

func TestFoldContributionAndOddChipLeftOfButton(t *testing.T) {
	// Board royal flush ties seats 1 and 2, folded seat 3 contributes 15.
	d := deck(cards("2C", "3C", "4C", "5C", "6C", "7C", "AS", "KS", "QS", "JS", "TS")...)
	e := hand(t, []int64{100, 100, 100}, 1, d)
	act(t, e, Raise, 20)
	act(t, e, Call, 0)
	act(t, e, Call, 0)
	act(t, e, Bet, 15)
	act(t, e, Call, 0)
	act(t, e, Raise, 30)
	act(t, e, Call, 0)
	act(t, e, Fold, 0)
	for e.State().Street != Settled {
		act(t, e, Check, 0)
	}
	s := e.State()
	if s.Players[0].Stack != chips(117) || s.Players[1].Stack != chips(118) || s.Players[2].Stack != chips(65) {
		t.Fatalf("odd chip: %+v", s.Players)
	}
	for _, p := range s.Pots {
		for _, a := range p.Awards {
			if a.Seat == 3 {
				t.Fatal("folded award")
			}
		}
	}
}

func TestEarlyFoldDoesNotDealBoard(t *testing.T) {
	e := hand(t, []int64{100, 100}, 1, deck())
	act(t, e, Fold, 0)
	s := e.State()
	if s.Street != Settled || len(s.Board) != 0 || s.Players[0].Stack != chips(95) || s.Players[1].Stack != chips(105) {
		t.Fatal("early settlement")
	}
	if len(s.Returns) != 1 || s.Returns[0].Amount != chips(5) {
		t.Fatal("uncalled blind")
	}
}

func TestTimeoutDisconnectAndServiceRecovery(t *testing.T) {
	e := hand(t, []int64{100, 100}, 1, deck())
	act(t, e, Call, 0)
	s := e.State()
	if r, err := e.Timeout(s.ActionSequence, s.Deadline.Add(-time.Millisecond)); err != nil || !r.NoOp {
		t.Fatal("early timer")
	}
	if _, err := e.SetConnected(2, false, s.StartedAt); err != nil {
		t.Fatal(err)
	}
	if !e.State().Deadline.Equal(s.Deadline) {
		t.Fatal("disconnect reset deadline")
	}
	if _, err := e.Timeout(s.ActionSequence, s.Deadline); err != nil {
		t.Fatal(err)
	}
	s = e.State()
	if s.Street != Flop || s.Actor != 2 {
		t.Fatal("auto-check should reach flop")
	}
	if _, err := e.Timeout(s.ActionSequence, s.Deadline); err != nil {
		t.Fatal(err)
	}
	if !e.State().Players[1].TimeoutSitOut || e.State().Players[1].ConsecutiveTimeouts != 2 {
		t.Fatal("two-timeout sitout")
	}
	if _, err := e.SetConnected(2, true, e.State().StartedAt); err != nil {
		t.Fatal(err)
	}
	if !e.State().Players[1].TimeoutSitOut {
		t.Fatal("reconnect cleared timeout sitout")
	}
	s = e.State()
	if _, err := e.BeginRecovery(s.StartedAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if r, err := e.Timeout(s.ActionSequence, s.Deadline.Add(time.Hour)); err != nil || !r.NoOp {
		t.Fatal("outage charged timeout")
	}
	un := *e.State().RecoveryUntil
	if r, err := e.Resume(un.Add(-time.Millisecond)); err != nil || !r.NoOp {
		t.Fatal("short grace")
	}
	if _, err := e.Resume(un); err != nil {
		t.Fatal(err)
	}
	if !e.State().Deadline.Equal(un.Add(DecisionWindow)) {
		t.Fatal("fresh full window")
	}
	f := hand(t, []int64{100, 100}, 1, deck())
	if _, err := f.Timeout(f.State().ActionSequence, f.State().Deadline); err != nil {
		t.Fatal(err)
	}
	if f.State().Street != Settled || !f.State().Players[0].Folded {
		t.Fatal("auto fold facing call")
	}
}

func TestSnapshotRestoreSameContinuationAndIsolation(t *testing.T) {
	e := hand(t, []int64{50, 100, 200}, 1, deck())
	act(t, e, AllIn, 0)
	b, err := e.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	r, err := Restore(b, Evaluate)
	if err != nil {
		t.Fatal(err)
	}
	act(t, e, AllIn, 0)
	act(t, r, AllIn, 0)
	act(t, e, Call, 0)
	act(t, r, Call, 0)
	a, _ := e.Snapshot()
	c, _ := r.Snapshot()
	if !bytes.Equal(a, c) {
		t.Fatal("restore diverged")
	}
	s := e.State()
	s.Players[0].Stack = 99999
	s.Config.Deck[0] = 99
	a2, _ := e.Snapshot()
	if !bytes.Equal(a, a2) {
		t.Fatal("State exposes mutable internals")
	}
	if _, err := Restore([]byte(`{"schema_version":1}`), Evaluate); err == nil {
		t.Fatal("invalid snapshot accepted")
	}
}

func TestRestoreRejectsCorruptBetAndPerPlayerAccounting(t *testing.T) {
	e := hand(t, []int64{100, 100, 100}, 1, deck())
	for _, change := range []func(*State){
		func(s *State) { s.CurrentBet = 0 },
		func(s *State) { s.Players[0].Stack++; s.Players[1].Stack-- },
		func(s *State) { s.Events[1].Delta++ },
		func(s *State) { s.Events[3].Visibility = "PUBLIC" },
	} {
		s := e.State()
		change(&s)
		data, _ := json.Marshal(s)
		if _, err := Restore(data, Evaluate); err == nil {
			t.Fatal("corrupt snapshot accepted")
		}
	}
}

func TestShortBlindsAndCumulativeShortRaises(t *testing.T) {
	// A lone funded SB has already covered the all-in BB; do not request an
	// unmatched nominal 10 call from it. No side can raise without an opponent.
	e := hand(t, []int64{100, 2}, 1, deck())
	if e.State().Street != Settled {
		t.Fatal("spurious lone decision")
	}
	conserved(t, e)
	u := hand(t, []int64{100, 100, 3}, 1, deck())
	if u.LegalActions().ToCall != chips(10) {
		t.Fatal("short BB lost nominal bring-in")
	}
	act(t, u, Call, 0)
	act(t, u, Call, 0)
	if u.State().Street != Flop {
		t.Fatal("short blind turn did not complete")
	}
	v := hand(t, []int64{15, 20, 100, 100}, 1, deck())
	// Seat 4 opens by calling 10. Seat 1 all-ins to 15, seat 2 to 20.
	act(t, v, Call, 0)
	act(t, v, AllIn, 0)
	act(t, v, AllIn, 0)
	act(t, v, Call, 0)
	if v.State().Actor != 4 || v.LegalActions().RaiseRights || v.State().FullRaiseSequence != 0 {
		t.Fatal("cumulative shorts reopened action")
	}
}

func TestHumanTimeoutRaceAndEvaluationFailureAreAtomic(t *testing.T) {
	e := hand(t, []int64{100, 100}, 1, deck())
	s := e.State()
	a := Action{ID: "race", Seat: s.Actor, ExpectedVersion: s.Version, Kind: Call}
	before, _ := e.Snapshot()
	if _, err := e.Apply(a, s.Deadline); !errors.Is(err, ErrDeadline) {
		t.Fatal(err)
	}
	after, _ := e.Snapshot()
	if !bytes.Equal(before, after) {
		t.Fatal("late human mutated")
	}
	if _, err := e.Apply(a, s.Deadline.Add(-time.Nanosecond)); err != nil {
		t.Fatal(err)
	}
	if r, err := e.Timeout(s.ActionSequence, s.Deadline); err != nil || !r.NoOp {
		t.Fatal("old timer won race")
	}
	broken := func([2]Card, []Card, string) (HandRank, error) {
		return HandRank{}, errors.New("synthetic evaluator failure")
	}
	f, err := New(Config{HandID: "failure", ButtonSeat: 1, SmallBlind: chips(5), BigBlind: chips(10), Deck: deck(), EvaluatorVersion: EvaluatorVersion}, []Seat{{SeatNo: 1, PlayerID: "1", Stack: chips(20)}, {SeatNo: 2, PlayerID: "2", Stack: chips(20)}}, epoch, broken)
	if err != nil {
		t.Fatal(err)
	}
	act(t, f, AllIn, 0)
	fs := f.State()
	before, _ = f.Snapshot()
	if _, err := f.Apply(Action{ID: "evaluate-fails", Seat: fs.Actor, ExpectedVersion: fs.Version, Kind: Call}, fs.StartedAt.Add(time.Second)); err == nil {
		t.Fatal("evaluator error lost")
	}
	after, _ = f.Snapshot()
	if !bytes.Equal(before, after) {
		t.Fatal("evaluation failure consumed chips/cards")
	}
}

func TestRecoveryRoundTripAndManualTimeoutReset(t *testing.T) {
	e := hand(t, []int64{100, 100}, 1, deck())
	act(t, e, Call, 0)
	s := e.State()
	if _, err := e.Timeout(s.ActionSequence, s.Deadline); err != nil {
		t.Fatal(err)
	}
	if e.State().Players[1].ConsecutiveTimeouts != 1 {
		t.Fatal("timeout count")
	}
	act(t, e, Check, 0)
	if e.State().Players[1].ConsecutiveTimeouts != 0 {
		t.Fatal("manual streak reset")
	}
	if _, err := e.BeginRecovery(e.State().StartedAt); err != nil {
		t.Fatal(err)
	}
	b, _ := e.Snapshot()
	r, err := Restore(b, Evaluate)
	if err != nil {
		t.Fatal(err)
	}
	until := *r.State().RecoveryUntil
	for _, x := range []*Engine{e, r} {
		if _, err := x.Resume(until); err != nil {
			t.Fatal(err)
		}
		act(t, x, Check, 0)
	}
	a, _ := e.Snapshot()
	c, _ := r.Snapshot()
	if !bytes.Equal(a, c) {
		t.Fatal("recovery timer/state diverged")
	}
}

func TestFewSyntheticTablesCompleteWithConservation(t *testing.T) {
	for _, n := range []int{2, 3, 5, 9} {
		t.Run(fmt.Sprintf("%d-seats", n), func(t *testing.T) {
			stacks := make([]int64, n)
			for i := range stacks {
				stacks[i] = int64(60 + 10*i)
			}
			e := hand(t, stacks, n, deck())
			for turn := 0; e.State().Street != Settled; turn++ {
				if turn >= 120 {
					t.Fatal("hand failed to terminate")
				}
				l := e.LegalActions()
				kind := Check
				to := int64(0)
				if l.ToCall > 0 {
					kind = Call
				}
				if turn%7 == 1 && l.RaiseRights {
					for _, a := range l.Actions {
						if a == Raise {
							kind = Raise
							to = l.MinimumRaiseTo
						}
						if a == Bet {
							kind = Bet
							to = l.MinimumBet
						}
					}
				}
				actUnits(t, e, kind, to)
			}
			b, _ := e.Snapshot()
			if _, err := Restore(b, Evaluate); err != nil {
				t.Fatal(err)
			}
		})
	}
}
