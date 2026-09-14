package engine

import (
	"fmt"
	"sort"
)

const EvaluatorVersion = "poker-holdem-high-v1"

// Evaluate chooses the best five of seven. Approved 2026-09-06: A2345 is five-high,
// suits never break ties, and standard high-poker kickers compare lexicographically.
func Evaluate(hole [2]Card, board []Card, version string) (HandRank, error) {
	if version != EvaluatorVersion || len(board) != 5 {
		return HandRank{}, fmt.Errorf("evaluator requires explicit version and five board cards")
	}
	cards := append([]Card{hole[0], hole[1]}, board...)
	seen := [52]bool{}
	for _, c := range cards {
		if c >= 52 || seen[c] {
			return HandRank{}, fmt.Errorf("invalid or duplicate card")
		}
		seen[c] = true
	}
	var best HandRank
	for a := 0; a < 3; a++ {
		for b := a + 1; b < 4; b++ {
			for c := b + 1; c < 5; c++ {
				for d := c + 1; d < 6; d++ {
					for e := d + 1; e < 7; e++ {
						idx := [5]int{a, b, c, d, e}
						var five [5]Card
						for i, n := range idx {
							five[i] = cards[n]
						}
						v := rank(five)
						if best.Vector == nil || compare(v, best.Vector) > 0 {
							best = HandRank{Vector: v, BestFive: idx, Version: version}
						}
					}
				}
			}
		}
	}
	best.Category = []string{"HIGH_CARD", "PAIR", "TWO_PAIR", "THREE_OF_A_KIND", "STRAIGHT", "FLUSH", "FULL_HOUSE", "FOUR_OF_A_KIND", "STRAIGHT_FLUSH"}[best.Vector[0]]
	return best, nil
}

func compare(a, b []int) int {
	for i := range a {
		if a[i] > b[i] {
			return 1
		}
		if a[i] < b[i] {
			return -1
		}
	}
	return 0
}

func rank(cards [5]Card) []int {
	counts := [15]int{}
	flush := true
	for _, c := range cards {
		r := int(c%13) + 1
		if r == 1 {
			r = 14
		}
		counts[r]++
		if c/13 != cards[0]/13 {
			flush = false
		}
	}
	var values []int
	for r := 14; r >= 2; r-- {
		for i := 0; i < counts[r]; i++ {
			values = append(values, r)
		}
	}
	straight := 0
	for high := 14; high >= 5; high-- {
		ok := true
		for d := 0; d < 5; d++ {
			r := high - d
			if r == 1 {
				r = 14
			}
			if counts[r] == 0 {
				ok = false
			}
		}
		if ok {
			straight = high
			break
		}
	}
	if flush && straight > 0 {
		return []int{8, straight}
	}
	type group struct{ rank, count int }
	var groups []group
	for r := 14; r >= 2; r-- {
		if counts[r] > 0 {
			groups = append(groups, group{r, counts[r]})
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].count != groups[j].count {
			return groups[i].count > groups[j].count
		}
		return groups[i].rank > groups[j].rank
	})
	if groups[0].count == 4 {
		return []int{7, groups[0].rank, groups[1].rank}
	}
	if groups[0].count == 3 && groups[1].count == 2 {
		return []int{6, groups[0].rank, groups[1].rank}
	}
	if flush {
		return append([]int{5}, values...)
	}
	if straight > 0 {
		return []int{4, straight}
	}
	v := []int{0}
	if groups[0].count == 3 {
		v[0] = 3
	} else if groups[0].count == 2 && groups[1].count == 2 {
		v[0] = 2
	} else if groups[0].count == 2 {
		v[0] = 1
	}
	for _, g := range groups {
		v = append(v, g.rank)
	}
	return v
}

type Card uint8

type HandRank struct {
	Category string `json:"category"`
	Vector   []int  `json:"primary_rank_vector"`
	BestFive [5]int `json:"best_five_card_indices"`
	Version  string `json:"hand_evaluator_version"`
}

// Evaluator is supplied by the caller and must honor its explicitly frozen version.
// Rank vectors compare lexicographically, higher first; suits must not be inferred here.
type Evaluator func(hole [2]Card, board []Card, version string) (HandRank, error)
