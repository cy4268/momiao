package engine_test

import (
	. "github.com/cy4268/momiao/internal/poker/engine"
	"reflect"
	"testing"
)

func TestInitialButtonChoiceAndClockwiseRotation(t *testing.T) {
	choice, err := SelectInitialButton([]int{9, 2, 5}, func(n int) (int, error) {
		if n != 3 {
			t.Fatal(n)
		}
		return 1, nil
	})
	if err != nil || choice.Seat != 5 || choice.Version != ButtonSelectionVersion || !reflect.DeepEqual(choice.EligibleSeats, []int{2, 5, 9}) {
		t.Fatal(choice, err)
	}
	if _, err := SelectInitialButton([]int{2, 5}, func(int) (int, error) { return 2, nil }); err == nil {
		t.Fatal("out-of-range sample")
	}
	if s, err := NextButton(9, []int{9, 2, 5}); err != nil || s != 2 {
		t.Fatal(s, err)
	}
	if s, err := NextButton(4, []int{9, 2, 5}); err != nil || s != 5 {
		t.Fatal(s, err)
	}
}

func TestApprovedPotShortcutsAndNoRaiseRights(t *testing.T) {
	e := hand(t, []int64{100, 100, 100}, 1, deck())
	// P=15,T=10,B=10 => half=22,two-thirds=26,pot=35.
	x := e.ShortcutTargets()
	want := []int64{20, 22, 26, 35, 100}
	if len(x) != len(want) {
		t.Fatal(x)
	}
	for i, s := range x {
		if s.To != chips(want[i]) {
			t.Fatalf("%s = %d units, want %d", s.Name, s.To, chips(want[i]))
		}
	}
	f := hand(t, []int64{100, 15, 100}, 1, deck())
	act(t, f, Call, 0)
	act(t, f, AllIn, 0)
	act(t, f, Call, 0)
	if len(f.ShortcutTargets()) != 0 {
		t.Fatal("short-all-in reopened shortcuts")
	}
	g := hand(t, []int64{15, 100, 100}, 1, deck())
	for _, s := range g.ShortcutTargets() {
		if s.To != chips(15) || s.Kind != AllIn {
			t.Fatal("short all-in must remain all-in", s)
		}
	}
}

func TestRecordedInitialChoiceNewAndInputValidation(t *testing.T) {
	choice, err := SelectInitialButton([]int{9, 2, 5}, func(int) (int, error) { return 1, nil })
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{HandID: "chosen", ButtonSeat: choice.Seat, ButtonSelection: &choice, SmallBlind: chips(5), BigBlind: chips(10), Deck: deck(), EvaluatorVersion: EvaluatorVersion}
	seats := []Seat{{SeatNo: 9, PlayerID: "9", Stack: chips(100)}, {SeatNo: 2, PlayerID: "2", Stack: chips(100)}, {SeatNo: 5, PlayerID: "5", Stack: chips(100)}}
	e, err := New(cfg, seats, epoch, Evaluate)
	if err != nil {
		t.Fatal(err)
	}
	if e.State().SmallBlindSeat != 9 || e.State().BigBlindSeat != 2 || e.State().Actor != 5 {
		t.Fatal("sparse seat order")
	}
	b, _ := e.Snapshot()
	if _, err := Restore(b, Evaluate); err != nil {
		t.Fatal(err)
	}
	choice.EligibleSeats[0] = 1
	cfg.Deck[0] = 51
	seats[0].Stack = 1
	if e.State().Config.ButtonSelection.EligibleSeats[0] != 2 || e.State().Config.Deck[0] != 0 || e.State().Players[2].InitialStack != chips(100) {
		t.Fatal("caller mutated internal state")
	}
	for _, change := range []func(*Config, []Seat){
		func(c *Config, _ []Seat) { c.ButtonSeat = 0 },
		func(c *Config, _ []Seat) { c.Deck[0] = c.Deck[1] },
		func(c *Config, _ []Seat) { c.SmallBlind = 1 },
		func(_ *Config, s []Seat) { s[1].PlayerID = s[0].PlayerID },
		func(_ *Config, s []Seat) { s[1].SeatNo = s[0].SeatNo },
		func(_ *Config, s []Seat) { s[1].Stack = -1 },
	} {
		c := Config{HandID: "invalid", ButtonSeat: 1, SmallBlind: chips(5), BigBlind: chips(10), Deck: deck(), EvaluatorVersion: EvaluatorVersion}
		s := []Seat{{SeatNo: 1, PlayerID: "1", Stack: chips(100)}, {SeatNo: 2, PlayerID: "2", Stack: chips(100)}}
		change(&c, s)
		if _, err := New(c, s, epoch, Evaluate); err == nil {
			t.Fatal("invalid config accepted")
		}
	}
}
