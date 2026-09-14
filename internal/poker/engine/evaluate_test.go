package engine_test

import (
	. "github.com/cy4268/momiao/internal/poker/engine"
	"reflect"
	"testing"
)

// Card identity is IS-07 §340: suit*13 + rank, A=0 ... K=12.
func cards(ranks ...string) []Card {
	out := make([]Card, len(ranks))
	for i, s := range ranks {
		r, su := -1, -1
		for j, c := range "A23456789TJQK" {
			if byte(c) == s[0] {
				r = j
			}
		}
		for j, c := range "CDHS" {
			if byte(c) == s[1] {
				su = j
			}
		}
		if r < 0 || su < 0 {
			panic(s)
		}
		out[i] = Card(su*13 + r)
	}
	return out
}

func TestEvaluatorCategoriesAndEdges(t *testing.T) {
	cases := []struct {
		name string
		all  []Card
		want []int
	}{
		{"straight_flush", cards("AS", "KS", "QS", "JS", "TS", "2D", "3C"), []int{8, 14}},
		{"quads", cards("AC", "AD", "AH", "AS", "KD", "2D", "3C"), []int{7, 14, 13}},
		{"two_trips_full_house", cards("AC", "AD", "AH", "KS", "KD", "KH", "3C"), []int{6, 14, 13}},
		{"flush", cards("AS", "JS", "9S", "6S", "4S", "KD", "QC"), []int{5, 14, 11, 9, 6, 4}},
		{"wheel", cards("AC", "2D", "3H", "4S", "5D", "KD", "QC"), []int{4, 5}},
		{"trips", cards("AC", "AD", "AH", "KS", "QD", "2D", "3C"), []int{3, 14, 13, 12}},
		{"three_pairs", cards("AC", "AD", "KH", "KS", "QD", "QS", "3C"), []int{2, 14, 13, 12}},
		{"pair", cards("AC", "AD", "KH", "QS", "JD", "2D", "3C"), []int{1, 14, 13, 12, 11}},
		{"high", cards("AC", "JD", "9H", "6S", "4D", "2D", "3C"), []int{0, 14, 11, 9, 6, 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Evaluate([2]Card{tc.all[0], tc.all[1]}, tc.all[2:], EvaluatorVersion)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(r.Vector, tc.want) {
				t.Fatalf("rank %v, want %v", r.Vector, tc.want)
			}
			if r.Version != EvaluatorVersion {
				t.Fatal(r.Version)
			}
		})
	}
}

func TestEvaluatorBoardTieAndValidation(t *testing.T) {
	b := cards("AS", "KS", "QS", "JS", "TS")
	a, err := Evaluate([2]Card{1, 2}, b, EvaluatorVersion)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Evaluate([2]Card{14, 15}, b, EvaluatorVersion)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Vector, c.Vector) {
		t.Fatal("board plays must tie; suits do not break ties")
	}
	if _, err := Evaluate([2]Card{1, 1}, b, EvaluatorVersion); err == nil {
		t.Fatal("duplicate accepted")
	}
	if _, err := Evaluate([2]Card{1, 2}, b, "production"); err == nil {
		t.Fatal("unapproved version accepted")
	}
}
