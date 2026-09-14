package games

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"
)

func wordsReader(words ...uint32) *bytes.Reader {
	var b []byte
	for _, word := range words {
		b = binary.BigEndian.AppendUint32(b, word)
	}
	return bytes.NewReader(b)
}

func countsOf(cells [9]string) map[string]int {
	counts := map[string]int{}
	for _, s := range cells {
		counts[s]++
	}
	return counts
}

func TestScratchEveryCanonicalMultiset(t *testing.T) {
	for _, tc := range []struct{ tier, symbol string }{
		{"BREAK_EVEN", "P1"}, {"T2", "P2"}, {"T3", "P3"}, {"T5", "P5"},
		{"T10", "P10"}, {"T25", "P25"}, {"TOP", "P100"},
	} {
		cells, match, err := scratchMultiset(tc.tier, nil)
		if err != nil || match != tc.symbol {
			t.Fatal(cells, match, err)
		}
		counts := countsOf(cells)
		if len(counts) != 7 || counts[match] != 3 {
			t.Fatal(counts)
		}
		for symbol, count := range counts {
			if symbol != match && count != 1 {
				t.Fatal(counts)
			}
		}
	}
	// Exhaust all 7*6 ordered choices, proving that LOSS cannot have a triple.
	pairs := map[[2]string]int{}
	for first := uint32(0); first < 7; first++ {
		for second := uint32(0); second < 6; second++ {
			cells, match, err := scratchMultiset("LOSS", wordsReader(first, second))
			if err != nil || match != "" {
				t.Fatal(cells, match, err)
			}
			counts := countsOf(cells)
			if len(counts) != 7 || cells[7] == cells[8] {
				t.Fatal(cells)
			}
			duplicated := 0
			for _, count := range counts {
				if count < 1 || count > 2 {
					t.Fatal("loss ticket has triple", counts)
				}
				if count == 2 {
					duplicated++
				}
			}
			if duplicated != 2 {
				t.Fatal(counts)
			}
			pairs[[2]string{cells[7], cells[8]}]++
		}
	}
	if len(pairs) != 42 {
		t.Fatal("filler mapping is not a bijection", pairs)
	}
	if _, _, err := scratchMultiset("UNKNOWN", nil); err == nil {
		t.Fatal("invalid tier accepted")
	}
	if _, _, err := scratchMultiset("LOSS", wordsReader()); err == nil {
		t.Fatal("missing filler source accepted")
	}
	if _, _, err := scratchMultiset("LOSS", wordsReader(1)); err == nil {
		t.Fatal("incomplete filler source accepted")
	}
}

func TestFisherYatesEveryThreeSymbolPermutation(t *testing.T) {
	seen := map[string]bool{}
	for a := uint32(0); a < 3; a++ {
		for b := uint32(0); b < 2; b++ {
			cells := []string{"a", "b", "c"}
			if err := shuffle(cells, wordsReader(a, b)); err != nil {
				t.Fatal(err)
			}
			seen[cells[0]+cells[1]+cells[2]] = true
		}
	}
	if !reflect.DeepEqual(seen, map[string]bool{"abc": true, "acb": true, "bac": true, "bca": true, "cab": true, "cba": true}) {
		t.Fatal(seen)
	}
	if err := shuffle([]string{"a", "b"}, wordsReader()); err == nil {
		t.Fatal("shuffle ignored source error")
	}
}

func TestScratchGoldenFullTicketAndReplay(t *testing.T) {
	c, _ := ScratchV1("fixture-v1")
	r := roundFor(c)
	want := ScratchResult{Tier: "BREAK_EVEN", Reward: Reward{CostMultiplier: 1, PayoutMultiplier: 1, Outcome: BreakEven}, Cells: [9]ScratchCell{
		{"P25", false}, {"P100", false}, {"P1", true}, {"P3", false}, {"P5", false}, {"P1", true}, {"P1", true}, {"P2", false}, {"P10", false},
	}}
	for i := 0; i < 3; i++ {
		got, err := ScratchCard(testSeed(), r, c)
		if err != nil || got != want {
			t.Fatal(got, err)
		}
	}
	r.Nonce = 0
	got, err := ScratchCard(testSeed(), r, c)
	if err != nil {
		t.Fatal(err)
	}
	wantSymbols := [9]string{"P5", "P5", "P1", "P2", "P3", "P10", "P1", "P100", "P25"}
	for i, cell := range got.Cells {
		if cell.Symbol != wantSymbols[i] {
			t.Fatal(got)
		}
	}
}

func TestScratchAllPrizeBranchesWithDeterministicStreams(t *testing.T) {
	base, _ := ScratchV1("fixture-v1")
	for selected, prize := range base.Prizes() {
		p := base.Prizes()
		for i := range p {
			p[i].Weight = 0
		}
		p[selected].Weight = 100000
		c, err := NewScratchConfig("fixture-v2", "forced-test-v1", p)
		if err != nil {
			t.Fatal(err)
		}
		for nonce := int64(0); nonce < 32; nonce++ {
			r := roundFor(c)
			r.Nonce = nonce
			got, err := ScratchCard(testSeed(), r, c)
			if err != nil || got.Tier != prize.Tier || got.Reward.PayoutMultiplier != prize.Multiplier || got.Reward.CostMultiplier != 1 {
				t.Fatal(got, err)
			}
			var cells [9]string
			matching := 0
			for i, cell := range got.Cells {
				cells[i] = cell.Symbol
				if cell.Matching {
					matching++
				}
			}
			counts := countsOf(cells)
			if selected == 0 {
				if got.Reward.Outcome != Loss || matching != 0 {
					t.Fatal(got)
				}
				for _, count := range counts {
					if count > 2 {
						t.Fatal(got)
					}
				}
			} else {
				if matching != 3 || len(counts) != 7 {
					t.Fatal(got)
				}
				for _, cell := range got.Cells {
					if cell.Matching != (counts[cell.Symbol] == 3) {
						t.Fatal(got)
					}
				}
				if selected == 1 && got.Reward.Outcome != BreakEven {
					t.Fatal("1x counted as win", got)
				}
				if selected > 1 && got.Reward.Outcome != Win {
					t.Fatal(got)
				}
			}
		}
	}
}

func TestScratchRejectsUnboundInputs(t *testing.T) {
	c, _ := ScratchV1("fixture-v1")
	r := roundFor(c)
	for _, mutate := range []func(*FairRound){
		func(r *FairRound) { r.ConfigHash[0]++ }, func(r *FairRound) { r.ConfigVersion = "other" },
		func(r *FairRound) { r.Game = "dice" }, func(r *FairRound) { r.AlgorithmVersion = "other-scratch-map-v1" },
		func(r *FairRound) { r.ServerSeedHash[0]++ },
	} {
		bad := r
		mutate(&bad)
		if _, err := ScratchCard(testSeed(), bad, c); err == nil {
			t.Fatal("unbound input accepted")
		}
	}
	if _, err := ScratchCard(nil, r, c); err == nil {
		t.Fatal("missing seed accepted")
	}
	if _, err := ScratchCard(testSeed(), r, Config{}); err == nil {
		t.Fatal("zero config accepted")
	}
	wrong, _ := DiceV1("fixture-v1")
	if _, err := ScratchCard(testSeed(), roundFor(wrong), wrong); err == nil {
		t.Fatal("wrong game config accepted")
	}
}
