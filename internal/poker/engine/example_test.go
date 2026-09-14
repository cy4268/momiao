package engine_test

import (
	"fmt"
	"github.com/cy4268/momiao/internal/poker/engine"
	"time"
)

func ExampleNew() {
	// In integration, the existing fairness boundary supplies the frozen deck.
	deck := make([]engine.Card, 52)
	for i := range deck {
		deck[i] = engine.Card(i)
	}
	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	e, err := engine.New(engine.Config{HandID: "demo", ButtonSeat: 1, SmallBlind: 5 * engine.UnitsPerChip, BigBlind: 10 * engine.UnitsPerChip, Deck: deck, EvaluatorVersion: engine.EvaluatorVersion}, []engine.Seat{{SeatNo: 1, PlayerID: "alice", Stack: 100 * engine.UnitsPerChip}, {SeatNo: 2, PlayerID: "bob", Stack: 100 * engine.UnitsPerChip}}, now, engine.Evaluate)
	if err != nil {
		panic(err)
	}
	s := e.State()
	_, err = e.Apply(engine.Action{ID: "alice-fold", Seat: 1, ExpectedVersion: s.Version, Kind: engine.Fold}, now.Add(time.Second))
	if err != nil {
		panic(err)
	}
	s = e.State()
	fmt.Println(s.Street, s.Players[0].Stack, s.Players[1].Stack)
	// Output: SETTLED 47500000 52500000
}
