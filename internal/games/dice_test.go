package games

import (
	"reflect"
	"testing"
)

func roundFor(c Config) FairRound {
	r := vectorRound()
	b := c.Binding()
	r.Game, r.AlgorithmVersion = b.Game, b.AlgorithmVersion
	r.ConfigVersion, r.ConfigHash = b.Version, b.Hash
	return r
}

func TestDiceEveryOrderedOutcome(t *testing.T) {
	counts := map[DiceSide]int{}
	wins := map[DiceSide]int{}
	for a := uint8(1); a <= 6; a++ {
		for b := uint8(1); b <= 6; b++ {
			for c := uint8(1); c <= 6; c++ {
				dice := [3]uint8{a, b, c}
				for _, choice := range []DiceSide{Big, Small} {
					got, err := EvaluateDice(dice, choice)
					if err != nil {
						t.Fatal(err)
					}
					if got.Dice != dice || got.Total != a+b+c || got.Triple != (a == b && b == c) || got.Choice != choice {
						t.Fatal("classification inputs lost", got)
					}
					if choice == Big {
						counts[got.Side]++
					}
					if got.Reward.Outcome == Win {
						wins[choice]++
						if got.Triple || got.Side != choice || got.Reward.PayoutMultiplier != 2 {
							t.Fatal(got)
						}
					} else if got.Reward.Outcome != Loss || got.Reward.PayoutMultiplier != 0 {
						t.Fatal(got)
					}
					if got.Reward.CostMultiplier != 1 {
						t.Fatal(got)
					}
				}
			}
		}
	}
	if !reflect.DeepEqual(counts, map[DiceSide]int{Big: 105, Small: 105, Triple: 6}) || !reflect.DeepEqual(wins, map[DiceSide]int{Big: 105, Small: 105}) {
		t.Fatal(counts, wins)
	}
}

func TestDiceRangeAndTripleBranches(t *testing.T) {
	for _, tc := range []struct {
		dice [3]uint8
		side DiceSide
	}{
		{[3]uint8{1, 1, 2}, Small}, {[3]uint8{1, 3, 6}, Small},
		{[3]uint8{1, 4, 6}, Big}, {[3]uint8{5, 6, 6}, Big},
		{[3]uint8{1, 1, 1}, Triple}, {[3]uint8{2, 2, 2}, Triple},
		{[3]uint8{3, 3, 3}, Triple}, {[3]uint8{4, 4, 4}, Triple},
		{[3]uint8{5, 5, 5}, Triple}, {[3]uint8{6, 6, 6}, Triple},
	} {
		for _, choice := range []DiceSide{Big, Small} {
			got, err := EvaluateDice(tc.dice, choice)
			if err != nil || got.Side != tc.side {
				t.Fatal(got, err)
			}
			if tc.side == Triple && got.Reward.Outcome != Loss {
				t.Fatal("triple paid", got)
			}
		}
	}
	for _, choice := range []DiceSide{"", Triple, "big", "BIG,SMALL"} {
		if _, err := EvaluateDice([3]uint8{1, 2, 3}, choice); err == nil {
			t.Fatal("invalid choice accepted")
		}
	}
	for _, dice := range [][3]uint8{{0, 1, 1}, {1, 0, 1}, {1, 1, 0}, {7, 1, 1}, {1, 7, 1}, {1, 1, 7}} {
		if _, err := EvaluateDice(dice, Big); err == nil {
			t.Fatal("invalid die accepted")
		}
	}
}

func TestDicePublishedReplayAndConfigurationBinding(t *testing.T) {
	c, _ := DiceV1("fixture-v1")
	r := roundFor(c)
	want := DiceResult{Dice: [3]uint8{4, 4, 1}, Total: 9, Side: Small, Choice: Small, Reward: Reward{CostMultiplier: 1, PayoutMultiplier: 2, Outcome: Win}}
	for i := 0; i < 3; i++ {
		got, err := RollDice(testSeed(), r, c, Small)
		if err != nil || got != want {
			t.Fatal(got, err)
		}
	}
	got, err := RollDice(testSeed(), r, c, Big)
	if err != nil || got.Dice != want.Dice || got.Reward.Outcome != Loss {
		t.Fatal("choice changed random result", got, err)
	}
	if _, err := RollDice(testSeed(), r, c, Triple); err == nil {
		t.Fatal("invalid choice accepted")
	}
	for _, mutate := range []func(*FairRound){
		func(r *FairRound) { r.ConfigHash[0]++ },
		func(r *FairRound) { r.ConfigVersion = "other-version" },
		func(r *FairRound) { r.Game = "scratch" },
		func(r *FairRound) { r.AlgorithmVersion = "other-algorithm" },
		func(r *FairRound) { r.ServerSeedHash[0]++ },
	} {
		bad := r
		mutate(&bad)
		if _, err := RollDice(testSeed(), bad, c, Small); err == nil {
			t.Fatal("unbound input accepted")
		}
	}
	if _, err := RollDice(testSeed(), r, Config{}, Small); err == nil {
		t.Fatal("zero config accepted")
	}
	wrong, _ := ScratchV1("fixture-v1")
	if _, err := RollDice(testSeed(), roundFor(wrong), wrong, Small); err == nil {
		t.Fatal("wrong game config accepted")
	}
}
