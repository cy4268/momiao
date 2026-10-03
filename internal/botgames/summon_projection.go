package botgames

import (
	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
	"math"
	"strconv"
	"time"
)

type SummonPrepared struct {
	Quote                 string        `json:"quote"`
	BaseWager             string        `json:"base_wager"`
	Mode                  string        `json:"mode"`
	DrawCount             int           `json:"draw_count"`
	StakeUnits            string        `json:"stake_units"`
	AvailableUnits        string        `json:"available_units"`
	MinimumBaseWagerUnits string        `json:"minimum_base_wager_units"`
	MaximumBaseWagerUnits string        `json:"maximum_base_wager_units"`
	Prizes                []games.Prize `json:"prizes"`
	Ruleset               string        `json:"ruleset"`
	RulesText             string        `json:"rules_text"`
	ServerSeedHash        string        `json:"server_seed_hash"`
	CommitmentID          string        `json:"commitment_id"`
	ExpiresAt             string        `json:"expires_at"`
}
type SummonDraw struct {
	Index       int    `json:"index"`
	Tier        string `json:"tier"`
	Multiplier  int64  `json:"multiplier"`
	PayoutUnits string `json:"payout_units"`
}
type SummonResult struct {
	RoundID             string       `json:"round_id"`
	CreatedAt           string       `json:"created_at"`
	SettledAt           string       `json:"settled_at"`
	Status              string       `json:"status"`
	BaseWager           string       `json:"base_wager"`
	Mode                string       `json:"mode"`
	DrawCount           int          `json:"draw_count"`
	Draws               []SummonDraw `json:"draws"`
	HighestTier         string       `json:"highest_tier"`
	Result              string       `json:"result"`
	StakeUnits          string       `json:"stake_units"`
	GrossPayoutUnits    string       `json:"gross_payout_units"`
	WithheldUnits       string       `json:"withheld_units"`
	CreditedPayoutUnits string       `json:"credited_payout_units"`
	ActualNetUnits      string       `json:"actual_net_units"`
	Ruleset             string       `json:"ruleset"`
}

func projectSummonRound(r games.GameRound) (SummonResult, error) {
	invalid := Fault{Code: "UPSTREAM_UNAVAILABLE"}
	base, count, valid := summonWagerUnits(r.Input.BaseWager, r.Input.Mode)
	stake := base * int64(count)
	if !valid || r.Input != (games.CreateInput{Type: "SUMMON", BaseWager: r.Input.BaseWager, Mode: r.Input.Mode}) || r.Game != "summon" || r.State != "SETTLED" || !validUUID(r.ID) || r.Ruleset != "summon-rules-v1" || r.Summon == nil ||
		r.CreatedAt.IsZero() || r.SettledAt == nil || r.SettledAt.IsZero() || r.SettledAt.Before(r.CreatedAt) || r.CreatedAt.UTC().Year() < 1 || r.CreatedAt.UTC().Year() > 9999 || r.SettledAt.UTC().Year() < 1 || r.SettledAt.UTC().Year() > 9999 ||
		r.StakeUnits != stake || r.PayoutUnits < 0 || r.NetUnits != r.PayoutUnits-stake || r.BalanceBeforeUnits < stake {
		return SummonResult{}, invalid
	}
	stored := r.Summon
	if stored.Mode != games.SummonMode(r.Input.Mode) || len(stored.Draws) != count {
		return SummonResult{}, invalid
	}
	draws := make([]SummonDraw, count)
	highest := "T0"
	var sum int64
	for i, d := range stored.Draws {
		multiplier, known := summonTierMultiplier(d.Tier)
		if d.Index != i+1 || !known || d.Multiplier != multiplier {
			return SummonResult{}, invalid
		}
		sum += multiplier
		if d.Tier > highest {
			highest = d.Tier
		}
		draws[i] = SummonDraw{Index: d.Index, Tier: d.Tier, Multiplier: multiplier, PayoutUnits: strconv.FormatInt(base*multiplier, 10)}
	}
	outcome := games.BreakEven
	if sum < int64(count) {
		outcome = games.Loss
	} else if sum > int64(count) {
		outcome = games.Win
	}
	if stored.HighestTier != highest || stored.Reward != (games.Reward{CostMultiplier: int64(count), PayoutMultiplier: sum, Outcome: outcome}) || r.Outcome != outcome || r.PayoutUnits != base*sum {
		return SummonResult{}, invalid
	}
	gross, withheld, credited, net := r.PayoutUnits, int64(0), r.PayoutUnits, r.NetUnits
	if c := r.EconomySettlement; c != nil {
		gross, withheld, credited, net = c.GrossPayoutUnits, c.WithheldUnits, c.CreditedPayoutUnits, c.ActualNetUnits
		if r.EconomicVersion != platform.EconomicPolicyVersion || c.PolicyVersion != r.EconomicVersion || !validHash(c.PolicyHash) || c.NeutralReturnUnits != 0 || gross != r.PayoutUnits || withheld < 0 || withheld > max(gross-stake, 0) || credited != gross-withheld || net != credited-stake {
			return SummonResult{}, invalid
		}
	} else if r.EconomicVersion != "" {
		return SummonResult{}, invalid
	}
	if r.BalanceBeforeUnits-stake > math.MaxInt64-credited || r.BalanceAfterUnits != r.BalanceBeforeUnits-stake+credited {
		return SummonResult{}, invalid
	}
	return SummonResult{RoundID: r.ID, CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339Nano), SettledAt: r.SettledAt.UTC().Format(time.RFC3339Nano), Status: r.State, BaseWager: r.Input.BaseWager, Mode: r.Input.Mode, DrawCount: count, Draws: draws, HighestTier: highest, Result: string(outcome), StakeUnits: strconv.FormatInt(stake, 10), GrossPayoutUnits: strconv.FormatInt(gross, 10), WithheldUnits: strconv.FormatInt(withheld, 10), CreditedPayoutUnits: strconv.FormatInt(credited, 10), ActualNetUnits: strconv.FormatInt(net, 10), Ruleset: r.Ruleset}, nil
}

func summonTierMultiplier(tier string) (int64, bool) {
	switch tier {
	case "T0":
		return 0, true
	case "T1":
		return 1, true
	case "T2":
		return 2, true
	case "T3":
		return 5, true
	case "T4":
		return 20, true
	case "T5":
		return 100, true
	default:
		return 0, false
	}
}
