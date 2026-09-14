package poker

import (
	"encoding/json"
	"github.com/cy4268/momiao/internal/poker/engine"
	"testing"
)

func TestCardsMarshalAsNumericArraysNotBase64(t *testing.T) {
	v := TableView{Hand: &HandView{}, Seats: []SeatView{{IsSelf: true}, {}}}
	v.Hand.BoardCards = append(v.Hand.BoardCards, 0, 13)
	v.Seats[0].HoleCards = append(v.Seats[0].HoleCards, 0, 13)
	v.Seats[1].PublicHoleCards = append(v.Seats[1].PublicHoleCards, 0, 13)
	v.Seats[1].HoleCardsReleased = true
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	assertCards := func(value any) {
		t.Helper()
		a, ok := value.([]any)
		if !ok || len(a) != 2 || a[0] != float64(0) || a[1] != float64(13) {
			t.Fatalf("card wire is not numeric array: %#v", value)
		}
	}
	assertCards(decoded["hand"].(map[string]any)["board_cards"])
	seats := decoded["seats"].([]any)
	assertCards(seats[0].(map[string]any)["hole_cards"])
	assertCards(seats[1].(map[string]any)["public_hole_cards"])
	if _, exists := seats[1].(map[string]any)["hole_cards"]; exists {
		t.Fatal("opponent private field emitted")
	}
	v.Hand.BoardCards = v.Hand.BoardCards[:0]
	raw, _ = json.Marshal(v)
	_ = json.Unmarshal(raw, &decoded)
	empty, ok := decoded["hand"].(map[string]any)["board_cards"].([]any)
	if !ok || len(empty) != 0 {
		t.Fatalf("empty board not []: %s", raw)
	}
	proof := FairnessView{}
	proof.Deck = append(proof.Deck, 0, 13)
	raw, _ = json.Marshal(proof)
	_ = json.Unmarshal(raw, &decoded)
	assertCards(decoded["deck"])
}

func TestPublicHoleReleasePolicy(t *testing.T) {
	players := []engine.Participant{{Seat: engine.Seat{SeatNo: 1}, Hole: [2]engine.Card{1, 2}}, {Seat: engine.Seat{SeatNo: 2}, Hole: [2]engine.Card{3, 4}}, {Seat: engine.Seat{SeatNo: 3}, Hole: [2]engine.Card{5, 6}, Folded: true}}
	state := engine.State{Players: players, Pots: []engine.Pot{{Awards: []engine.Award{{Seat: 1, Amount: 10 * engine.UnitsPerChip}}}}}
	if got := releasedHoles(state); len(got) != 0 {
		t.Fatal("fold-win release", got)
	}
	state.Events = []engine.Event{{Type: "SHOWDOWN"}}
	if got := releasedHoles(state); len(got) != 1 || len(got[1]) != 2 || got[2] != nil || got[3] != nil {
		t.Fatal("ordinary showdown only necessary winner", got)
	}
	state.Events = append(state.Events, engine.Event{Type: "ALL_IN_RUNOUT"})
	if got := releasedHoles(state); len(got) != 2 || len(got[2]) != 2 || got[3] != nil {
		t.Fatal("runout release excludes folded", got)
	}
}

func TestWaitForBigBlindUsesFinalParticipantOrder(t *testing.T) {
	seats := []seatRow{{Seat: 1, State: "ACTIVE", Connected: true, Stack: 500}, {Seat: 2, State: "ACTIVE", Connected: true, Stack: 500}, {Seat: 3, State: "WAITING_BIG_BLIND", Connected: true, Stack: 500}}
	table := tableRow{HandNo: 2, Button: 1}
	p, b, err := selectParticipants(&table, seats)
	if err != nil || b != 2 || len(p) != 2 {
		t.Fatal("new seat would be SB, must wait", p, b, err)
	}
	table.Button = 2
	p, b, err = selectParticipants(&table, seats)
	if err != nil || b != 1 || len(p) != 3 || p[2].Seat != 3 {
		t.Fatal("seat 3 must join at natural BB", p, b, err)
	}
}

func TestCipherBindsKeyHandAndPurpose(t *testing.T) {
	s := &Service{opts: Options{Keyring: Keyring{Current: "k1", Keys: map[string][]byte{"k1": make([]byte, 32)}}}}
	plain := []byte("private seed and future deck fixture")
	sealed, err := s.seal("hand1:snapshot", plain)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := s.open("hand1:snapshot", sealed)
	if err != nil || !equal(opened, plain) {
		t.Fatal("roundtrip", err)
	}
	if _, err = s.open("hand2:snapshot", sealed); err == nil {
		t.Fatal("cross-hand substitution")
	}
	if _, err = s.open("hand1:setup", sealed); err == nil {
		t.Fatal("cross-purpose substitution")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err = s.open("hand1:snapshot", sealed); err == nil {
		t.Fatal("tampering accepted")
	}
}
