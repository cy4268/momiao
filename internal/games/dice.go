package games

import "github.com/cy4268/momiao/internal/games/fairness"

type Outcome string

const (
	Loss      Outcome = "LOSS"
	BreakEven Outcome = "BREAK_EVEN"
	Win       Outcome = "WIN"
)

// Reward describes exact multiples of one base input. A tenfold summon costs
// ten base inputs and its payout is the sum of all ten draw multipliers. There
// is no balance or currency here; the service must handle exact unit arithmetic.
type Reward struct {
	CostMultiplier   int64   `json:"cost_multiplier,string"`
	PayoutMultiplier int64   `json:"payout_multiplier,string"`
	Outcome          Outcome `json:"outcome"`
}

func reward(cost, payout int64) Reward {
	r := Reward{CostMultiplier: cost, PayoutMultiplier: payout, Outcome: BreakEven}
	if payout < cost {
		r.Outcome = Loss
	} else if payout > cost {
		r.Outcome = Win
	}
	return r
}

type DiceSide string

const (
	Big    DiceSide = "BIG"
	Small  DiceSide = "SMALL"
	Triple DiceSide = "TRIPLE"
)

type DiceResult struct {
	Dice   [3]uint8 `json:"dice"`
	Total  uint8    `json:"total"`
	Triple bool     `json:"triple"`
	Side   DiceSide `json:"side"`
	Choice DiceSide `json:"choice"`
	Reward Reward   `json:"reward"`
}

// RollDice resolves one deterministic round using the frozen dice-map-v1.
func RollDice(seed []byte, r FairRound, c Config, choice DiceSide) (DiceResult, error) {
	if err := c.checkRound(r, "dice"); err != nil {
		return DiceResult{}, err
	}
	if choice != Big && choice != Small {
		return DiceResult{}, ErrInvalidInput
	}
	var dice [3]uint8
	for i, domain := range []string{"dice:d1", "dice:d2", "dice:d3"} {
		s, err := fairness.NewStream(seed, r, domain)
		if err != nil {
			return DiceResult{}, err
		}
		n, err := fairness.UniformInt(s, 6)
		if err != nil {
			return DiceResult{}, err
		}
		dice[i] = uint8(n + 1)
	}
	result, err := EvaluateDice(dice, choice)
	if err == nil && result.Triple && c.binding.RulesetVersion == "dice-rules-v2" {
		result.Reward = reward(1, 1)
	}
	return result, err
}

// EvaluateDice classifies already specified faces for auditing and enumeration.
// It is not an authoritative round creation API; services must use RollDice.
func EvaluateDice(dice [3]uint8, choice DiceSide) (DiceResult, error) {
	if choice != Big && choice != Small {
		return DiceResult{}, ErrInvalidInput
	}
	for _, d := range dice {
		if d < 1 || d > 6 {
			return DiceResult{}, ErrInvalidInput
		}
	}
	side := classifyDice(dice)
	payout := int64(0)
	if side == choice {
		payout = 2
	}
	return DiceResult{Dice: dice, Total: dice[0] + dice[1] + dice[2], Triple: side == Triple, Side: side, Choice: choice, Reward: reward(1, payout)}, nil
}

func classifyDice(dice [3]uint8) DiceSide {
	if dice[0] == dice[1] && dice[1] == dice[2] {
		return Triple
	}
	if dice[0]+dice[1]+dice[2] <= 10 {
		return Small
	}
	return Big
}
