package slot

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestFrozenStripsAndPaytable(t *testing.T) {
	c := FrozenConfig()
	// Verbatim IS-06 section 273 order; frequency-only tests miss adjacency changes.
	wantStrips := [5]string{
		"L3,L2,L3,M1,L2,L1,L3,L2,M2,H2,L1,M2,L3,M1,H1,L2,L1,H1,L1,M2,L2,L3,L1,M1,W,L1,L2,L1,L2,H2,M1,L1",
		"L2,M2,L1,L2,L3,L2,W,L1,H1,L1,M1,L3,M1,H2,M1,L2,L1,L3,L1,L3,H1,L1,H2,L3,L1,L2,M2,L2,L1,L2,M1,M2",
		"L2,L3,L1,M2,L1,H1,L3,L1,M1,M2,L2,M1,L3,L1,L2,L3,H2,L2,L1,M1,L2,L1,H1,L2,L1,H2,M1,L3,L1,L2,M2,W",
		"M1,L3,W,L2,L1,H2,L2,M2,L1,M1,L1,L3,H1,L2,L3,M2,H1,L1,L2,L3,M1,L1,L3,L1,L2,H2,L1,M1,M2,L2,L1,L2",
		"L1,H2,L2,L1,L3,L2,L3,L1,H1,L2,L1,L2,M1,L1,W,H2,M1,L2,H1,L1,L2,L1,M1,M2,L3,M1,L2,L1,M2,L3,M2,L3",
	}
	for r, strip := range c.ReelStrips {
		var names []string
		for _, s := range strip {
			names = append(names, string(s))
		}
		if strings.Join(names, ",") != wantStrips[r] {
			t.Fatalf("reel %d order drift", r)
		}
	}
	want := [8]int{8, 7, 5, 4, 3, 2, 2, 1}
	for r, strip := range c.ReelStrips {
		var got [8]int
		for _, s := range strip {
			for i, symbol := range symbols {
				if s == symbol {
					got[i]++
				}
			}
		}
		if got != want {
			t.Fatalf("reel %d frequencies %v", r, got)
		}
	}
	if c.ReelStrips[0][24] != W || c.ReelStrips[1][6] != W || c.ReelStrips[2][31] != W || c.ReelStrips[3][2] != W || c.ReelStrips[4][14] != W {
		t.Fatal("frozen wild offsets changed")
	}
	c.ReelStrips[0][0] = W
	if FrozenConfig().ReelStrips[0][0] != L3 {
		t.Fatal("caller mutated frozen rules")
	}
	for _, tc := range []struct {
		line       [5]Symbol
		symbol     Symbol
		length     int
		multiplier int64
	}{
		{[5]Symbol{W, W, W, L1, L1}, W, 3, 125},
		{[5]Symbol{W, W, W, H2, H2}, H2, 5, 2500},
		{[5]Symbol{W, W, W, W, W}, W, 5, 5000},
		{[5]Symbol{L1, L1, L1, L1, L1}, L1, 5, 50},
		{[5]Symbol{L2, L3, L3, L3, L3}, "", 0, 0},
	} {
		got := evaluateLine(tc.line)
		if got.Symbol != tc.symbol || got.MatchLength != tc.length || got.Multiplier != tc.multiplier {
			t.Fatalf("%v: %+v", tc.line, got)
		}
	}
}

func TestMoneyProjectionUsesDecimalStrings(t *testing.T) {
	r, e := Resolve(11*UnitsPerChip, [5]int{})
	if e != nil {
		t.Fatal(e)
	}
	data, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(data), `"total_wager_units":"5500000"`) || !strings.Contains(string(data), `"line_stake_units":"550000"`) {
		t.Fatal("integer wire precision", string(data))
	}
}

func TestStopsReplayAndIntegerPayout(t *testing.T) {
	stops := [5]int{24, 6, 31, 2, 14}
	got, err := Resolve(11*UnitsPerChip, stops)
	if err != nil {
		t.Fatal(err)
	}
	if got.Grid[0] != [3]Symbol{M1, W, L1} || got.Grid[2] != [3]Symbol{M2, W, L2} {
		t.Fatalf("grid %v", got.Grid)
	}
	if got.Lines[0].Multiplier != 5000 || got.Lines[0].PayoutUnits != 2_750_000_000 {
		t.Fatalf("line %+v", got.Lines[0])
	}
	var sum int64
	for _, line := range got.Lines {
		sum += line.PayoutUnits
	}
	if sum != got.TotalPayoutUnits || got.NetChangeUnits != sum-got.TotalWagerUnits {
		t.Fatal("settlement arithmetic")
	}
	i := 0
	spin, err := Spin(11*UnitsPerChip, func(domain string, n uint32) (uint32, error) {
		if domain != domains[i] || n != 32 {
			t.Fatalf("stream %q %d", domain, n)
		}
		v := uint32(stops[i])
		i++
		return v, nil
	})
	if err != nil || i != 5 || !reflect.DeepEqual(got, spin) {
		t.Fatalf("replay %v", err)
	}
}

func TestInputAndOverflow(t *testing.T) {
	for _, wager := range []int64{-1, 0, 9 * UnitsPerChip, 10*UnitsPerChip + 1, math.MaxInt64 / UnitsPerChip * UnitsPerChip} {
		if _, err := Resolve(wager, [5]int{}); err == nil {
			t.Fatalf("accepted wager %d", wager)
		}
	}
	for _, stop := range []int{-1, 32} {
		if _, err := Resolve(10*UnitsPerChip, [5]int{stop}); err == nil {
			t.Fatal("accepted stop")
		}
	}
	e := errors.New("stream failure")
	if _, err := Spin(10*UnitsPerChip, func(string, uint32) (uint32, error) { return 0, e }); !errors.Is(err, e) {
		t.Fatal(err)
	}
	if _, err := Spin(10*UnitsPerChip, func(string, uint32) (uint32, error) { return 32, nil }); err == nil {
		t.Fatal("accepted biased/out-of-range sample")
	}
}
