package games

import "strconv"

type SummonMode string

const (
	Single  SummonMode = "SINGLE"
	Tenfold SummonMode = "TENFOLD"
)

type DrawResult struct {
	Index      int    `json:"index"`
	Tier       string `json:"tier"`
	Multiplier int64  `json:"multiplier,string"`
}

type SummonResult struct {
	Mode        SummonMode   `json:"mode"`
	Draws       []DrawResult `json:"draws"`
	HighestTier string       `json:"highest_tier"`
	Reward      Reward       `json:"reward"`
}

func SummonDraw(seed []byte, r FairRound, c Config, index int) (DrawResult, error) {
	if index < 1 || index > 10 {
		return DrawResult{}, ErrInvalidInput
	}
	if err := c.checkRound(r, "summon"); err != nil {
		return DrawResult{}, err
	}
	p, err := c.samplePrize(seed, r, "summon:draw:"+strconv.Itoa(index))
	if err != nil {
		return DrawResult{}, err
	}
	return DrawResult{index, p.Tier, p.Multiplier}, nil
}

func Summon(seed []byte, r FairRound, c Config, mode SummonMode) (SummonResult, error) {
	count := 0
	switch mode {
	case Single:
		count = 1
	case Tenfold:
		count = 10
	default:
		return SummonResult{}, ErrInvalidInput
	}
	result := SummonResult{Mode: mode, Draws: make([]DrawResult, 0, count), HighestTier: "T0"}
	var payout int64
	for i := 1; i <= count; i++ {
		draw, err := SummonDraw(seed, r, c, i)
		if err != nil {
			return SummonResult{}, err
		}
		result.Draws = append(result.Draws, draw)
		// Fixed T0..T5 identifiers have the same lexical and logical order.
		if draw.Tier > result.HighestTier {
			result.HighestTier = draw.Tier
		}
		payout += draw.Multiplier // At most 10 * 100 for the frozen v1 tiers.
	}
	result.Reward = reward(int64(count), payout)
	return result, nil
}
