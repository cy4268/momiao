package games

import (
	"io"

	"github.com/cy4268/momiao/internal/games/fairness"
)

type ScratchCell struct {
	Symbol   string `json:"symbol"`
	Matching bool   `json:"matching"`
}

type ScratchResult struct {
	Tier   string         `json:"tier"`
	Cells  [9]ScratchCell `json:"cells"`
	Reward Reward         `json:"reward"`
}

func ScratchCard(seed []byte, r FairRound, c Config) (ScratchResult, error) {
	if err := c.checkRound(r, "scratch"); err != nil {
		return ScratchResult{}, err
	}
	p, err := c.samplePrize(seed, r, "scratch:prize")
	if err != nil {
		return ScratchResult{}, err
	}
	filler, err := fairness.NewStream(seed, r, "scratch:filler")
	if err != nil {
		return ScratchResult{}, err
	}
	cells, matching, err := scratchMultiset(p.Tier, filler)
	if err != nil {
		return ScratchResult{}, err
	}
	layout, err := fairness.NewStream(seed, r, "scratch:layout")
	if err != nil {
		return ScratchResult{}, err
	}
	if err = shuffle(cells[:], layout); err != nil {
		return ScratchResult{}, err
	}
	result := ScratchResult{Tier: p.Tier, Reward: reward(1, p.Multiplier)}
	for i, symbol := range cells {
		result.Cells[i] = ScratchCell{symbol, symbol == matching}
	}
	return result, nil
}

// Canonical order is P1/P2/P3/P5/P10/P25/P100. A winning prefix contains
// three copies of its symbol followed by each other symbol in canonical order.
// LOSS appends two distinct uniformly chosen symbols to the seven-symbol base.
func scratchMultiset(tier string, filler io.Reader) ([9]string, string, error) {
	symbols := [7]string{"P1", "P2", "P3", "P5", "P10", "P25", "P100"}
	var cells [9]string
	if tier == "LOSS" {
		copy(cells[:], symbols[:])
		first, err := fairness.UniformInt(filler, 7)
		if err != nil {
			return cells, "", err
		}
		second, err := fairness.UniformInt(filler, 6)
		if err != nil {
			return cells, "", err
		}
		if second >= first {
			second++
		}
		cells[7], cells[8] = symbols[first], symbols[second]
		return cells, "", nil
	}
	for i, p := range scratchPrizesV1()[1:] {
		if p.Tier != tier {
			continue
		}
		winning := symbols[i]
		cells[0], cells[1], cells[2] = winning, winning, winning
		n := 3
		for _, s := range symbols {
			if s != winning {
				cells[n] = s
				n++
			}
		}
		return cells, winning, nil
	}
	return cells, "", ErrInvalidInput
}

func shuffle(cells []string, source io.Reader) error {
	for i := len(cells) - 1; i > 0; i-- {
		j, err := fairness.UniformInt(source, uint64(i+1))
		if err != nil {
			return err
		}
		cells[i], cells[j] = cells[j], cells[i]
	}
	return nil
}
