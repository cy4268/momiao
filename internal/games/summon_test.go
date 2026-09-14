package games

import (
	"reflect"
	"testing"
)

func TestSummonGoldenSingleTenfoldAndIndexedReplay(t *testing.T) {
	c, _ := SummonV1("fixture-v1")
	r := roundFor(c)
	want := []DrawResult{{1, "T0", 0}, {2, "T1", 1}, {3, "T0", 0}, {4, "T1", 1}, {5, "T2", 2}, {6, "T1", 1}, {7, "T0", 0}, {8, "T1", 1}, {9, "T0", 0}, {10, "T0", 0}}
	got, err := Summon(testSeed(), r, c, Tenfold)
	if err != nil || !reflect.DeepEqual(got.Draws, want) || got.Mode != Tenfold || got.HighestTier != "T2" || got.Reward != (Reward{CostMultiplier: 10, PayoutMultiplier: 6, Outcome: Loss}) {
		t.Fatal(got, err)
	}
	single, err := Summon(testSeed(), r, c, Single)
	if err != nil || len(single.Draws) != 1 || single.Draws[0] != want[0] || single.Reward != (Reward{CostMultiplier: 1, PayoutMultiplier: 0, Outcome: Loss}) {
		t.Fatal(single, err)
	}
	for _, i := range []int{10, 1, 5, 3, 6, 9, 4, 7, 8, 2, 5} {
		draw, err := SummonDraw(testSeed(), r, c, i)
		if err != nil || draw != want[i-1] {
			t.Fatal("index depends on call history", draw, err)
		}
	}
	got.Draws[0].Tier = "changed-caller-copy"
	replay, err := Summon(testSeed(), r, c, Tenfold)
	if err != nil || !reflect.DeepEqual(replay.Draws, want) {
		t.Fatal("replay aliased result", replay, err)
	}
}

func TestSummonAllTiersAndWholeRoundClassification(t *testing.T) {
	base, _ := SummonV1("fixture-v1")
	for selected, prize := range base.Prizes() {
		p := base.Prizes()
		for i := range p {
			p[i].Weight = 0
		}
		p[selected].Weight = 100000
		// Reverse configured order to ensure highest tier is based on logical ID.
		for a, b := 0, len(p)-1; a < b; a, b = a+1, b-1 {
			p[a], p[b] = p[b], p[a]
		}
		c, err := NewSummonConfig("fixture-v2", "forced-test-v1", p)
		if err != nil {
			t.Fatal(err)
		}
		for _, mode := range []SummonMode{Single, Tenfold} {
			got, err := Summon(testSeed(), roundFor(c), c, mode)
			if err != nil {
				t.Fatal(err)
			}
			count := int64(1)
			if mode == Tenfold {
				count = 10
			}
			if int64(len(got.Draws)) != count || got.HighestTier != prize.Tier || got.Reward.CostMultiplier != count || got.Reward.PayoutMultiplier != count*prize.Multiplier {
				t.Fatal(got)
			}
			for i, draw := range got.Draws {
				if draw.Index != i+1 || draw.Tier != prize.Tier || draw.Multiplier != prize.Multiplier {
					t.Fatal(got)
				}
			}
			want := Win
			if selected == 0 {
				want = Loss
			}
			if selected == 1 {
				want = BreakEven
			}
			if got.Reward.Outcome != want {
				t.Fatal(got)
			}
		}
	}
}

func TestSummonRejectsInvalidModeIndexAndBinding(t *testing.T) {
	c, _ := SummonV1("fixture-v1")
	r := roundFor(c)
	for _, mode := range []SummonMode{"", "single", "DOUBLE", "TENFOLD_WITH_GUARANTEE"} {
		if _, err := Summon(testSeed(), r, c, mode); err == nil {
			t.Fatal("invalid mode accepted")
		}
	}
	for _, index := range []int{-1, 0, 11, 100} {
		if _, err := SummonDraw(testSeed(), r, c, index); err == nil {
			t.Fatal("invalid index accepted")
		}
	}
	for _, mutate := range []func(*FairRound){
		func(r *FairRound) { r.ConfigHash[0]++ }, func(r *FairRound) { r.ConfigVersion = "other" },
		func(r *FairRound) { r.Game = "dice" }, func(r *FairRound) { r.AlgorithmVersion = "other" },
		func(r *FairRound) { r.ServerSeedHash[0]++ },
	} {
		bad := r
		mutate(&bad)
		if _, err := Summon(testSeed(), bad, c, Tenfold); err == nil {
			t.Fatal("unbound input accepted")
		}
		if _, err := SummonDraw(testSeed(), bad, c, 1); err == nil {
			t.Fatal("draw accepted unbound input")
		}
	}
	if _, err := Summon(nil, r, c, Single); err == nil {
		t.Fatal("missing seed accepted")
	}
	if _, err := Summon(testSeed(), r, Config{}, Single); err == nil {
		t.Fatal("zero config accepted")
	}
	wrong, _ := DiceV1("fixture-v1")
	if _, err := Summon(testSeed(), roundFor(wrong), wrong, Single); err == nil {
		t.Fatal("wrong game config accepted")
	}
}
