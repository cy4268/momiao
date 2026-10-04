package botgames

import (
	"math"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
)

// ScratchPrepared is an action-tagged union. Purchase-only fields and
// resume-only fields are deliberately omitted from the opposite action.
type ScratchPrepared struct {
	Action               string        `json:"action"`
	Quote                string        `json:"quote"`
	Wager                string        `json:"wager"`
	StakeUnits           string        `json:"stake_units"`
	AdditionalStakeUnits string        `json:"additional_stake_units"`
	AvailableUnits       string        `json:"available_units,omitempty"`
	MinimumWagerUnits    string        `json:"minimum_wager_units,omitempty"`
	MaximumWagerUnits    string        `json:"maximum_wager_units,omitempty"`
	Prizes               []games.Prize `json:"prizes,omitempty"`
	RoundID              string        `json:"round_id,omitempty"`
	CreatedAt            string        `json:"created_at,omitempty"`
	Ruleset              string        `json:"ruleset"`
	RulesText            string        `json:"rules_text"`
	ServerSeedHash       string        `json:"server_seed_hash,omitempty"`
	CommitmentID         string        `json:"commitment_id,omitempty"`
	ExpiresAt            string        `json:"expires_at"`
}
type ScratchCell struct {
	Index    int    `json:"index"`
	Symbol   string `json:"symbol"`
	Matching bool   `json:"matching"`
}
type ScratchResult struct {
	Action                  string         `json:"action"`
	RoundID                 string         `json:"round_id"`
	CreatedAt               string         `json:"created_at"`
	SettledAt               string         `json:"settled_at"`
	PresentationCompletedAt string         `json:"presentation_completed_at"`
	Status                  string         `json:"status"`
	Wager                   string         `json:"wager"`
	Cells                   [9]ScratchCell `json:"cells"`
	PrizeTier               string         `json:"prize_tier"`
	Multiplier              int64          `json:"multiplier"`
	Result                  string         `json:"result"`
	StakeUnits              string         `json:"stake_units"`
	GrossPayoutUnits        string         `json:"gross_payout_units"`
	WithheldUnits           string         `json:"withheld_units"`
	CreditedPayoutUnits     string         `json:"credited_payout_units"`
	ActualNetUnits          string         `json:"actual_net_units"`
	Ruleset                 string         `json:"ruleset"`
}

func projectScratchRound(r games.GameRound, action string, requireCompleted bool) (ScratchResult, error) {
	invalid := Fault{Code: "UPSTREAM_UNAVAILABLE"}
	stake, valid := scratchWagerUnits(r.Input.Wager)
	if !valid || (action != "PURCHASE" && action != "RESUME") || r.Input != (games.CreateInput{Type: "SCRATCH", Wager: r.Input.Wager}) || r.Game != "scratch" || r.State != "SETTLED" || !validUUID(r.ID) || r.Ruleset != "scratch-rules-v1" || r.Algorithm != games.ScratchAlgorithm || r.Scratch == nil || !scratchTimestamp(r.CreatedAt) || r.SettledAt == nil || !scratchTimestamp(*r.SettledAt) || r.SettledAt.Before(r.CreatedAt) || r.StakeUnits != stake || r.PayoutUnits < 0 || r.NetUnits != r.PayoutUnits-stake || r.BalanceBeforeUnits < stake {
		return ScratchResult{}, invalid
	}
	completed := ""
	if r.PresentationCompletedAt != nil {
		if !scratchTimestamp(*r.PresentationCompletedAt) || r.PresentationCompletedAt.Before(*r.SettledAt) {
			return ScratchResult{}, invalid
		}
		completed = r.PresentationCompletedAt.UTC().Format(time.RFC3339Nano)
	} else if requireCompleted {
		return ScratchResult{}, invalid
	}
	multiplier, symbol, known := scratchTier(r.Scratch.Tier)
	if !known {
		return ScratchResult{}, invalid
	}
	counts := map[string]int{"P1": 0, "P2": 0, "P3": 0, "P5": 0, "P10": 0, "P25": 0, "P100": 0}
	var cells [9]ScratchCell
	for i, c := range r.Scratch.Cells {
		if _, ok := counts[c.Symbol]; !ok || c.Matching != (c.Symbol == symbol) {
			return ScratchResult{}, invalid
		}
		counts[c.Symbol]++
		cells[i] = ScratchCell{Index: i + 1, Symbol: c.Symbol, Matching: c.Matching}
	}
	doubled := 0
	for cellSymbol, count := range counts {
		if multiplier == 0 {
			if count == 2 {
				doubled++
			} else if count != 1 {
				return ScratchResult{}, invalid
			}
		} else if cellSymbol == symbol {
			if count != 3 {
				return ScratchResult{}, invalid
			}
		} else if count != 1 {
			return ScratchResult{}, invalid
		}
	}
	if multiplier == 0 && doubled != 2 {
		return ScratchResult{}, invalid
	}
	outcome := games.Win
	if multiplier == 0 {
		outcome = games.Loss
	} else if multiplier == 1 {
		outcome = games.BreakEven
	}
	if r.Scratch.Reward != (games.Reward{CostMultiplier: 1, PayoutMultiplier: multiplier, Outcome: outcome}) || r.Outcome != outcome || r.PayoutUnits != stake*multiplier {
		return ScratchResult{}, invalid
	}
	gross, withheld, credited, net := r.PayoutUnits, int64(0), r.PayoutUnits, r.NetUnits
	if c := r.EconomySettlement; c != nil {
		gross, withheld, credited, net = c.GrossPayoutUnits, c.WithheldUnits, c.CreditedPayoutUnits, c.ActualNetUnits
		if r.EconomicVersion != platform.EconomicPolicyVersion || c.PolicyVersion != r.EconomicVersion || !validHash(c.PolicyHash) || c.NeutralReturnUnits != 0 || gross != r.PayoutUnits || withheld < 0 || withheld > max(gross-stake, 0) || credited != gross-withheld || net != credited-stake {
			return ScratchResult{}, invalid
		}
	} else if r.EconomicVersion != "" {
		return ScratchResult{}, invalid
	}
	if r.BalanceBeforeUnits-stake > math.MaxInt64-credited || r.BalanceAfterUnits != r.BalanceBeforeUnits-stake+credited {
		return ScratchResult{}, invalid
	}
	return ScratchResult{Action: action, RoundID: r.ID, CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339Nano), SettledAt: r.SettledAt.UTC().Format(time.RFC3339Nano), PresentationCompletedAt: completed, Status: r.State, Wager: r.Input.Wager, Cells: cells, PrizeTier: r.Scratch.Tier, Multiplier: multiplier, Result: string(outcome), StakeUnits: strconv.FormatInt(stake, 10), GrossPayoutUnits: strconv.FormatInt(gross, 10), WithheldUnits: strconv.FormatInt(withheld, 10), CreditedPayoutUnits: strconv.FormatInt(credited, 10), ActualNetUnits: strconv.FormatInt(net, 10), Ruleset: r.Ruleset}, nil
}
func scratchTimestamp(t time.Time) bool {
	return !t.IsZero() && t.UTC().Year() >= 1 && t.UTC().Year() <= 9999
}
func scratchTier(tier string) (int64, string, bool) {
	switch tier {
	case "LOSS":
		return 0, "", true
	case "BREAK_EVEN":
		return 1, "P1", true
	case "T2":
		return 2, "P2", true
	case "T3":
		return 3, "P3", true
	case "T5":
		return 5, "P5", true
	case "T10":
		return 10, "P10", true
	case "T25":
		return 25, "P25", true
	case "TOP":
		return 100, "P100", true
	default:
		return 0, "", false
	}
}
