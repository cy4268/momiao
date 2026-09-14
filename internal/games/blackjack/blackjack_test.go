package blackjack

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

var epoch = time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)

func systemID(handID string) (string, error) { return "system-" + handID, nil }

// Preserves every distinct instance while arranging only a known deal prefix.
func shoeWith(prefix ...uint16) (shoe [312]uint16) {
	seen := map[uint16]bool{}
	i := 0
	for _, v := range prefix {
		if v >= 312 || seen[v] {
			panic("bad test prefix")
		}
		seen[v] = true
		shoe[i] = v
		i++
	}
	for v := uint16(0); v < 312; v++ {
		if !seen[v] {
			shoe[i] = v
			i++
		}
	}
	return
}
func newRound(t *testing.T, prefix ...uint16) (State, [312]uint16) {
	t.Helper()
	shoe := shoeWith(prefix...)
	s, e := New(shoe, 11*UnitsPerChip, "h0", epoch)
	if e != nil {
		t.Fatal(e)
	}
	return s, shoe
}
func action(t *testing.T, s State, shoe [312]uint16, typ ActionType, id string) Transition {
	t.Helper()
	tr, e := Apply(s, shoe, Action{ID: id, ExpectedVersion: s.Version, HandID: s.ActiveHandID, Type: typ, NewHandID: "hand-" + id}, math.MaxInt64, epoch.Add(time.Minute*time.Duration(s.Version)))
	if e != nil {
		t.Fatalf("%s: %v", typ, e)
	}
	if e = VerifyRecovery(tr.State, shoe); e != nil {
		t.Fatalf("recovery: %v", e)
	}
	return tr
}

func TestCanonicalShuffleAndHash(t *testing.T) {
	nWant := uint32(312)
	shoe, e := Shuffle(func(n uint32) (uint32, error) {
		if n != nWant {
			t.Fatalf("bound %d != %d", n, nWant)
		}
		nWant--
		return n - 1, nil
	})
	if e != nil || nWant != 1 {
		t.Fatal(e)
	}
	var serialized [624]byte
	for i, v := range shoe {
		if int(v) != i {
			t.Fatal("wrong input order")
		}
		binary.BigEndian.PutUint16(serialized[i*2:], v)
	}
	sum := sha256.Sum256(serialized[:])
	hash, e := ShoeHash(shoe)
	if e != nil || hash != hex.EncodeToString(sum[:]) {
		t.Fatal(hash, e)
	}
	shifted, e := Shuffle(func(uint32) (uint32, error) { return 0, nil })
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 311; i++ {
		if shifted[i] != uint16(i+1) {
			t.Fatal("wrong FY direction")
		}
	}
	if shifted[311] != 0 {
		t.Fatal("wrong final swap")
	}
	if _, e = Shuffle(func(n uint32) (uint32, error) { return n, nil }); e == nil {
		t.Fatal("invalid sample accepted")
	}
	shoe[311] = shoe[0]
	if _, e = ShoeHash(shoe); e == nil {
		t.Fatal("duplicate card accepted")
	}
}

func TestInitialNaturalPeekAndHalfChip(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cards  []uint16
		payout int64
		result string
	}{
		{"natural", []uint16{0, 8, 9, 6}, 13_750_000, "NATURAL"},
		{"both natural", []uint16{0, 13, 9, 22}, 5_500_000, "PUSH"},
		{"dealer natural", []uint16{8, 13, 9, 22}, 0, "LOSS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, shoe := newRound(t, tc.cards...)
			if s.Phase != Settled || s.ShoeIndex != 4 || !s.DealerRevealed || s.TotalPayoutUnits != tc.payout || s.Hands[0].Result != tc.result {
				t.Fatalf("settlement %+v", s)
			}
			if e := VerifyRecovery(s, shoe); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestPrivacyAndS17(t *testing.T) {
	s, shoe := newRound(t, 9, 0, 7, 5) // Player 18, dealer A+6 (soft 17).
	if s.Phase != PlayerTurn || s.DealerRevealed || s.ShoeIndex != 4 || s.AutoResolveAt != epoch.Add(24*time.Hour) {
		t.Fatal("initial state")
	}
	pub := PublicView(s, 0)
	if len(pub.DealerCards) != 1 || pub.DealerTotal != nil {
		t.Fatal("hole total leaked")
	}
	raw, e := json.Marshal(s)
	if e == nil || len(raw) != 0 {
		t.Fatal("private authority marshaled")
	}
	text, _ := json.Marshal(pub)
	for _, secret := range []string{"shoe_hash", "shoe_index", "card_instance_id", "history"} {
		if strings.Contains(string(text), secret) {
			t.Fatalf("public field leak %s", secret)
		}
	}
	tr := action(t, s, shoe, Stand, "stand")
	if tr.State.ShoeIndex != 4 || tr.State.TotalPayoutUnits != 22*UnitsPerChip || tr.State.Class != "WIN" {
		t.Fatal("dealer hit S17 or wrong payout")
	}
	if len(PublicView(tr.State, 0).DealerCards) != 2 {
		t.Fatal("terminal dealer not revealed")
	}
}

func TestHitBustAndDouble(t *testing.T) {
	s, shoe := newRound(t, 9, 4, 8, 6, 22)
	tr := action(t, s, shoe, Hit, "hit-bust")
	if tr.State.ShoeIndex != 5 || tr.State.Hands[0].Status != Bust || tr.State.TotalPayoutUnits != 0 || tr.State.Class != "LOSS" {
		t.Fatal("all bust must skip dealer draw")
	}
	s, shoe = newRound(t, 4, 9, 5, 7, 22)
	tr = action(t, s, shoe, Double, "double")
	if tr.AdditionalStakeUnits != 11*UnitsPerChip || tr.State.ShoeIndex != 5 || tr.State.Hands[0].Status != DoubledComplete || tr.State.TotalStakeUnits != 22*UnitsPerChip || tr.State.TotalPayoutUnits != 44*UnitsPerChip {
		t.Fatalf("double %+v", tr)
	}
	s, shoe = newRound(t, 4, 9, 5, 7, 2, 3)
	tr = action(t, s, shoe, Hit, "hit")
	if tr.State.LastPlayerActionAt != epoch.Add(time.Minute) || tr.State.AutoResolveAt != epoch.Add(24*time.Hour+time.Minute) {
		t.Fatal("manual anchor")
	}
	for _, a := range LegalActions(tr.State, math.MaxInt64) {
		if a == Double || a == Split {
			t.Fatal("three-card double/split")
		}
	}
}

func TestSplitTensResplitAndStableOrder(t *testing.T) {
	s, shoe := newRound(t, 9, 6, 10, 22, 11, 12, 35, 1, 48, 2)
	tr := action(t, s, shoe, Split, "s1")
	if tr.AdditionalStakeUnits != s.InitialWagerUnits || tr.State.Hands[0].ID != "h0" || len(tr.State.Hands) != 2 || tr.State.Hands[0].Cards[1].InstanceID != 11 || tr.State.Hands[1].Cards[1].InstanceID != 12 {
		t.Fatal("split order")
	}
	right := tr.State.Hands[1]
	tr = action(t, tr.State, shoe, Split, "s2")
	if len(tr.State.Hands) != 3 || tr.State.Hands[2].ID != right.ID || tr.State.Hands[2].Index != right.Index {
		t.Fatal("unstable existing right hand")
	}
	tr = action(t, tr.State, shoe, Split, "s3")
	if len(tr.State.Hands) != 4 {
		t.Fatal("resplit max")
	}
	for _, a := range LegalActions(tr.State, math.MaxInt64) {
		if a == Split {
			t.Fatal("fifth hand allowed")
		}
	}
	if tr.State.TotalStakeUnits != 4*s.InitialWagerUnits {
		t.Fatal("split stake conservation")
	}
	for i := 1; i < len(tr.State.Hands); i++ {
		if tr.State.Hands[i].Index <= tr.State.Hands[i-1].Index {
			t.Fatal("index ordering")
		}
	}
}

func TestSplitAcesAndDAS(t *testing.T) {
	s, shoe := newRound(t, 0, 9, 13, 7, 22, 26)
	tr := action(t, s, shoe, Split, "aces")
	if tr.State.Phase != Settled || tr.State.ShoeIndex != 6 || tr.State.TotalPayoutUnits != 22*UnitsPerChip || len(LegalActions(tr.State, math.MaxInt64)) != 0 {
		t.Fatal("split ace completion")
	}
	for _, h := range tr.State.Hands {
		if h.Status != SplitAcesComplete || h.Natural || len(h.Cards) != 2 {
			t.Fatal("split ace hand")
		}
	}
	s, shoe = newRound(t, 7, 9, 20, 6, 1, 2, 22)
	tr = action(t, s, shoe, Split, "pair")
	tr = action(t, tr.State, shoe, Double, "das")
	if tr.AdditionalStakeUnits != 11*UnitsPerChip || tr.State.Hands[0].Status != DoubledComplete || tr.State.ActiveHandID != tr.State.Hands[1].ID {
		t.Fatal("DAS")
	}
}

func TestIllegalActionsDoNotMutate(t *testing.T) {
	s, shoe := newRound(t, 7, 9, 20, 6, 1, 2)
	before := s.clone()
	for _, tc := range []struct {
		a         Action
		available int64
	}{
		{Action{ID: "a", ExpectedVersion: s.Version + 1, HandID: "h0", Type: Hit}, math.MaxInt64},
		{Action{ID: "a", ExpectedVersion: s.Version, HandID: "other", Type: Hit}, math.MaxInt64},
		{Action{ID: "a", ExpectedVersion: s.Version, HandID: "h0", Type: "SURRENDER"}, math.MaxInt64},
		{Action{ID: "a", ExpectedVersion: s.Version, HandID: "h0", Type: Double}, 0},
		{Action{ID: "a", ExpectedVersion: s.Version, HandID: "h0", Type: Split, NewHandID: "h0"}, math.MaxInt64},
	} {
		if _, e := Apply(s, shoe, tc.a, tc.available, epoch); e == nil {
			t.Fatal("illegal accepted")
		}
		if !reflect.DeepEqual(s, before) {
			t.Fatal("input mutated")
		}
	}
	tr := action(t, s, shoe, Hit, "once")
	_, e := Apply(tr.State, shoe, Action{ID: "once", ExpectedVersion: s.Version, HandID: "h0", Type: Hit}, 0, epoch)
	if !errors.Is(e, ErrDuplicateAction) {
		t.Fatal("duplicate id", e)
	}
}

func TestTimeoutAndRecoveryTampering(t *testing.T) {
	s, shoe := newRound(t, 7, 9, 20, 6, 1, 2)
	tr := action(t, s, shoe, Split, "pair")
	s = tr.State
	before := s.clone()
	for _, factory := range []func(string) (string, error){
		nil,
		func(string) (string, error) { return "duplicate", nil },
		func() func(string) (string, error) {
			calls := 0
			return func(string) (string, error) {
				calls++
				if calls == 2 {
					return "", errors.New("factory failure")
				}
				return "first-system-id", nil
			}
		}(),
	} {
		if _, err := AutoResolve(s, shoe, s.AutoResolveAt, factory); err == nil {
			t.Fatal("invalid action ID factory accepted")
		}
		if !reflect.DeepEqual(s, before) {
			t.Fatal("factory failure mutated original")
		}
	}
	early, e := AutoResolve(s, shoe, s.AutoResolveAt.Add(-time.Nanosecond), nil)
	if e != nil || !reflect.DeepEqual(early.State, s) {
		t.Fatal("early timeout changed state")
	}
	auto, e := AutoResolve(s, shoe, s.AutoResolveAt, systemID)
	if e != nil {
		t.Fatal(e)
	}
	if auto.State.Phase != Settled || auto.AdditionalStakeUnits != 0 || auto.State.LastPlayerActionAt != s.LastPlayerActionAt || len(auto.State.Actions) != len(s.Actions)+2 {
		t.Fatal("auto sequence/anchor")
	}
	again, e := AutoResolve(auto.State, shoe, s.AutoResolveAt.Add(time.Hour), nil)
	if e != nil || !reflect.DeepEqual(auto.State, again.State) {
		t.Fatal("timeout retry not noop")
	}
	if e = VerifyRecovery(auto.State, shoe); e != nil {
		t.Fatal(e)
	}
	for _, mutate := range []func(*State){func(s *State) { s.ShoeIndex++ }, func(s *State) { s.ShoeHash = "bad" }, func(s *State) { s.Hands[0].Cards[0].InstanceID = 300 }, func(s *State) { s.TotalPayoutUnits++ }, func(s *State) { s.AutoResolveAt = s.AutoResolveAt.Add(time.Minute) }} {
		bad := auto.State.clone()
		mutate(&bad)
		if e := VerifyRecovery(bad, shoe); !errors.Is(e, ErrNeedsReview) {
			t.Fatal("tamper not blocked", e)
		}
	}
}

func TestValuesAndOverflow(t *testing.T) {
	cards := []Card{{InstanceID: 0}, {InstanceID: 13}, {InstanceID: 8}}
	value, e := Evaluate(cards)
	if e != nil || value.HardTotal != 11 || value.BestTotal != 21 || !value.Soft {
		t.Fatal(value, e)
	}
	cards = append(cards, Card{InstanceID: 1})
	value, e = Evaluate(cards)
	if e != nil || value.BestTotal != 13 || value.Soft {
		t.Fatal(value, e)
	}
	if _, e = Evaluate([]Card{{InstanceID: 312}}); e == nil {
		t.Fatal("invalid instance")
	}
	for _, w := range []int64{0, 9 * UnitsPerChip, 10*UnitsPerChip + 1, math.MaxInt64 / UnitsPerChip * UnitsPerChip} {
		if _, e := New(shoeWith(), w, "h0", epoch); e == nil {
			t.Fatal("wager accepted", w)
		}
	}
}

func TestRecoveredEquivalentRepresentations(t *testing.T) {
	s, shoe := newRound(t, 7, 9, 20, 6, 1, 2)
	s.Actions = []ActionRecord{}
	zone := time.FixedZone("database-offset", 8*60*60)
	s.CreatedAt = s.CreatedAt.In(zone)
	s.LastPlayerActionAt = s.LastPlayerActionAt.In(zone)
	s.AutoResolveAt = s.AutoResolveAt.In(zone)
	if e := VerifyRecovery(s, shoe); e != nil {
		t.Fatal("equivalent persisted representation", e)
	}
	p := PublicView(s, 0)
	encoded, e := json.Marshal(p)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(encoded), `"stake_units":"5500000"`) || !strings.Contains(string(encoded), `"round_version":"1"`) {
		t.Fatal("integer wire precision", string(encoded))
	}
}

func TestBoundedReplayAndConservation(t *testing.T) {
	for rotation := 0; rotation < 32; rotation++ {
		shoe := shoeWith()
		for i := range shoe {
			shoe[i] = uint16((i + rotation*7) % 312)
		}
		s, e := New(shoe, 11*UnitsPerChip, "initial", epoch)
		if e != nil {
			t.Fatal(e)
		}
		totalStake := s.InitialWagerUnits
		for step := 0; s.Phase == PlayerTurn; step++ {
			if step > 312 {
				t.Fatal("round did not terminate")
			}
			legal := LegalActions(s, math.MaxInt64)
			typ := legal[(rotation+step)%len(legal)]
			a := Action{ID: fmt.Sprintf("action-%d", step), ExpectedVersion: s.Version, HandID: s.ActiveHandID, Type: typ, NewHandID: fmt.Sprintf("child-%d", step)}
			before := s.clone()
			tr, e := Apply(s, shoe, a, math.MaxInt64, epoch.Add(time.Duration(step+1)*time.Minute))
			if e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(s, before) {
				t.Fatal("successful action mutated source")
			}
			s = tr.State
			totalStake += tr.AdditionalStakeUnits
			if e = VerifyRecovery(s, shoe); e != nil {
				t.Fatal(e)
			}
		}
		seen := map[int]bool{}
		cards := append([]Card(nil), s.Dealer...)
		var stakes, payout, net int64
		for _, h := range s.Hands {
			cards = append(cards, h.Cards...)
			stakes += h.StakeUnits
			payout += h.PayoutUnits
			net += h.NetChangeUnits
		}
		for _, c := range cards {
			if seen[c.ShoeIndex] || shoe[c.ShoeIndex] != c.InstanceID {
				t.Fatal("duplicate/lost card ownership")
			}
			seen[c.ShoeIndex] = true
		}
		if len(cards) != s.ShoeIndex || len(s.Dealt) != s.ShoeIndex || stakes != totalStake || s.TotalStakeUnits != stakes || s.TotalPayoutUnits != payout || s.NetChangeUnits != net || net != payout-stakes {
			t.Fatal("card/stake/payout conservation")
		}
		p := PublicView(s, 0)
		p.Hands[0].Cards[0] = 999
		if s.Hands[0].Cards[0].InstanceID == 999 {
			t.Fatal("public view aliases authority")
		}
	}
}
