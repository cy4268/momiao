package botgames

import (
	"encoding/json"
	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fixtureSummonRound(mode string) games.GameRound {
	created := time.Unix(1700000001, 123000000).In(time.FixedZone("fixture", 8*3600))
	settled := created.Add(time.Second)
	draws := []games.DrawResult{{Index: 1, Tier: "T2", Multiplier: 2}}
	count, sum := int64(1), int64(2)
	highest := "T2"
	if mode == "TENFOLD" {
		count, sum, highest = 10, 131, "T5"
		draws = []games.DrawResult{{Index: 1, Tier: "T0", Multiplier: 0}, {Index: 2, Tier: "T1", Multiplier: 1}, {Index: 3, Tier: "T2", Multiplier: 2}, {Index: 4, Tier: "T3", Multiplier: 5}, {Index: 5, Tier: "T4", Multiplier: 20}, {Index: 6, Tier: "T5", Multiplier: 100}, {Index: 7, Tier: "T1", Multiplier: 1}, {Index: 8, Tier: "T0", Multiplier: 0}, {Index: 9, Tier: "T2", Multiplier: 2}, {Index: 10, Tier: "T0", Multiplier: 0}}
	}
	return games.GameRound{ID: "00000000-0000-4000-8000-000000000002", Game: "summon", State: "SETTLED", Ruleset: "summon-rules-v1", Input: games.CreateInput{Type: "SUMMON", BaseWager: "11", Mode: mode}, StakeUnits: 5500000 * count, PayoutUnits: 5500000 * sum, NetUnits: 5500000 * (sum - count), Outcome: games.Win, BalanceBeforeUnits: 550000000, BalanceAfterUnits: 550000000 + 5500000*(sum-count), CreatedAt: created, SettledAt: &settled, Summon: &games.SummonResult{Mode: games.SummonMode(mode), Draws: draws, HighestTier: highest, Reward: games.Reward{CostMultiplier: count, PayoutMultiplier: sum, Outcome: games.Win}}}
}

// Hand-calculated 11-Chip draws prove index/order, each payout, total classification and cap accounting.
func TestSummonProjectionDrawsAndWholeRoundAccounting(t *testing.T) {
	for _, mode := range []string{"SINGLE", "TENFOLD"} {
		t.Run(mode, func(t *testing.T) {
			r := fixtureSummonRound(mode)
			got, e := projectSummonRound(r)
			if e != nil {
				t.Fatal(e)
			}
			if got.CreatedAt != "2023-11-14T22:13:21.123Z" || got.SettledAt != "2023-11-14T22:13:22.123Z" || got.Status != "SETTLED" || got.BaseWager != "11" || got.Mode != mode || got.Result != "WIN" || got.WithheldUnits != "0" || got.CreditedPayoutUnits != got.GrossPayoutUnits {
				t.Fatal(got)
			}
			if mode == "SINGLE" {
				if got.DrawCount != 1 || got.StakeUnits != "5500000" || got.GrossPayoutUnits != "11000000" || got.ActualNetUnits != "5500000" || got.HighestTier != "T2" {
					t.Fatal(got)
				}
			} else {
				if got.DrawCount != 10 || got.StakeUnits != "55000000" || got.GrossPayoutUnits != "720500000" || got.ActualNetUnits != "665500000" || got.HighestTier != "T5" {
					t.Fatal(got)
				}
				payouts := []string{"0", "5500000", "11000000", "27500000", "110000000", "550000000", "5500000", "0", "11000000", "0"}
				for i, d := range got.Draws {
					if d.Index != i+1 || d.PayoutUnits != payouts[i] {
						t.Fatal(i, d)
					}
				}
			}
			raw, _ := json.Marshal(got)
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(raw, &fields)
			allowed := []string{"round_id", "created_at", "settled_at", "status", "base_wager", "mode", "draw_count", "draws", "highest_tier", "result", "stake_units", "gross_payout_units", "withheld_units", "credited_payout_units", "actual_net_units", "ruleset"}
			if len(fields) != len(allowed) {
				t.Fatal(string(raw))
			}
			for _, key := range allowed {
				if _, ok := fields[key]; !ok {
					t.Fatal("missing", key)
				}
			}
			var draws []map[string]any
			_ = json.Unmarshal(fields["draws"], &draws)
			for _, d := range draws {
				if len(d) != 4 {
					t.Fatal(d)
				}
				for _, key := range []string{"index", "tier", "multiplier", "payout_units"} {
					if _, ok := d[key]; !ok {
						t.Fatal(key)
					}
				}
				if _, ok := d["multiplier"].(float64); !ok {
					t.Fatal("multiplier must be numeric")
				}
			}
			r.EconomicVersion = platform.EconomicPolicyVersion
			r.EconomySettlement = &platform.PayoutCapView{PolicyVersion: r.EconomicVersion, PolicyHash: strings.Repeat("a", 64), GrossPayoutUnits: r.PayoutUnits, WithheldUnits: r.NetUnits, CreditedPayoutUnits: r.StakeUnits, ActualNetUnits: 0}
			r.BalanceAfterUnits = r.BalanceBeforeUnits
			got, e = projectSummonRound(r)
			if e != nil || got.ActualNetUnits != "0" || got.Result != "WIN" || got.CreditedPayoutUnits != got.StakeUnits {
				t.Fatal(got, e)
			}
		})
	}
	for _, tc := range []struct {
		tier        string
		multiplier  int64
		outcome     games.Outcome
		payout, net string
	}{{"T0", 0, games.Loss, "0", "-5500000"}, {"T1", 1, games.BreakEven, "5500000", "0"}} {
		r := fixtureSummonRound("SINGLE")
		r.Summon.Draws[0] = games.DrawResult{Index: 1, Tier: tc.tier, Multiplier: tc.multiplier}
		r.Summon.HighestTier = tc.tier
		r.Summon.Reward = games.Reward{CostMultiplier: 1, PayoutMultiplier: tc.multiplier, Outcome: tc.outcome}
		r.PayoutUnits = 5500000 * tc.multiplier
		r.NetUnits = r.PayoutUnits - r.StakeUnits
		r.Outcome = tc.outcome
		r.BalanceAfterUnits = r.BalanceBeforeUnits + r.NetUnits
		got, e := projectSummonRound(r)
		if e != nil || got.Result != string(tc.outcome) || got.GrossPayoutUnits != tc.payout || got.ActualNetUnits != tc.net {
			t.Fatal(got, e)
		}
	}
}

func TestSummonProjectionRejectsCorruptStoredData(t *testing.T) {
	for name, change := range map[string]func(*games.GameRound){
		"missing_summon": func(r *games.GameRound) { r.Summon = nil }, "game": func(r *games.GameRound) { r.Game = "dice" }, "state": func(r *games.GameRound) { r.State = "PENDING" }, "id": func(r *games.GameRound) { r.ID = "x" }, "ruleset": func(r *games.GameRound) { r.Ruleset = "summon-rules-v2" },
		"type": func(r *games.GameRound) { r.Input.Type = "SLOT" }, "extra_input": func(r *games.GameRound) { r.Input.TotalWager = "11" }, "base": func(r *games.GameRound) { r.Input.BaseWager = "011" }, "base_zero": func(r *games.GameRound) { r.Input.BaseWager = "0" }, "base_overflow": func(r *games.GameRound) { r.Input.BaseWager = "9223372036854775807" }, "mode": func(r *games.GameRound) { r.Input.Mode = "SINGLE" }, "stored_mode": func(r *games.GameRound) { r.Summon.Mode = games.Single },
		"draw_missing": func(r *games.GameRound) { r.Summon.Draws = r.Summon.Draws[:9] }, "draw_extra": func(r *games.GameRound) {
			r.Summon.Draws = append(r.Summon.Draws, games.DrawResult{Index: 11, Tier: "T0", Multiplier: 0})
		}, "index": func(r *games.GameRound) { r.Summon.Draws[3].Index = 3 }, "tier": func(r *games.GameRound) { r.Summon.Draws[3].Tier = "T6" }, "multiplier": func(r *games.GameRound) { r.Summon.Draws[3].Multiplier = 6 }, "negative_multiplier": func(r *games.GameRound) { r.Summon.Draws[3].Multiplier = -1 }, "highest": func(r *games.GameRound) { r.Summon.HighestTier = "T4" },
		"reward_cost": func(r *games.GameRound) { r.Summon.Reward.CostMultiplier = 1 }, "reward_payout": func(r *games.GameRound) { r.Summon.Reward.PayoutMultiplier = 132 }, "reward_outcome": func(r *games.GameRound) { r.Summon.Reward.Outcome = games.Loss }, "common_outcome": func(r *games.GameRound) { r.Outcome = games.Loss }, "stake": func(r *games.GameRound) { r.StakeUnits++ }, "gross": func(r *games.GameRound) { r.PayoutUnits++ }, "net": func(r *games.GameRound) { r.NetUnits++ }, "negative_gross": func(r *games.GameRound) { r.PayoutUnits = -1 },
		"created_zero": func(r *games.GameRound) { r.CreatedAt = time.Time{} }, "settled_nil": func(r *games.GameRound) { r.SettledAt = nil }, "settled_early": func(r *games.GameRound) { v := r.CreatedAt.Add(-time.Second); r.SettledAt = &v }, "created_year": func(r *games.GameRound) { r.CreatedAt = time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC) }, "settled_year": func(r *games.GameRound) { v := time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC); r.SettledAt = &v },
		"before_insufficient": func(r *games.GameRound) { r.BalanceBeforeUnits = r.StakeUnits - 1 }, "after_wrong": func(r *games.GameRound) { r.BalanceAfterUnits++ }, "before_overflow": func(r *games.GameRound) { r.BalanceBeforeUnits = math.MaxInt64 }, "missing_economy": func(r *games.GameRound) { r.EconomicVersion = platform.EconomicPolicyVersion },
	} {
		t.Run(name, func(t *testing.T) {
			r := fixtureSummonRound("TENFOLD")
			change(&r)
			got, e := projectSummonRound(r)
			requireFault(t, e, "UPSTREAM_UNAVAILABLE")
			if !reflect.DeepEqual(got, SummonResult{}) {
				t.Fatal("partial projection leaked")
			}
		})
	}
}

func TestSummonProjectionRejectsIncompatibleEconomics(t *testing.T) {
	for name, change := range map[string]func(*games.GameRound){
		"version": func(r *games.GameRound) { r.EconomicVersion = "unknown" }, "policy_version": func(r *games.GameRound) { r.EconomySettlement.PolicyVersion = "unknown" }, "policy_hash": func(r *games.GameRound) { r.EconomySettlement.PolicyHash = "" }, "neutral": func(r *games.GameRound) { r.EconomySettlement.NeutralReturnUnits = 1 }, "gross": func(r *games.GameRound) { r.EconomySettlement.GrossPayoutUnits++ }, "negative_withheld": func(r *games.GameRound) { r.EconomySettlement.WithheldUnits = -1 }, "over_withheld": func(r *games.GameRound) { r.EconomySettlement.WithheldUnits = r.PayoutUnits + 1 }, "principal_withheld": func(r *games.GameRound) {
			r.EconomySettlement.WithheldUnits = r.PayoutUnits
			r.EconomySettlement.CreditedPayoutUnits = 0
			r.EconomySettlement.ActualNetUnits = -r.StakeUnits
			r.BalanceAfterUnits = r.BalanceBeforeUnits - r.StakeUnits
		}, "credited": func(r *games.GameRound) { r.EconomySettlement.CreditedPayoutUnits++ }, "net": func(r *games.GameRound) { r.EconomySettlement.ActualNetUnits++ },
	} {
		t.Run(name, func(t *testing.T) {
			r := fixtureSummonRound("TENFOLD")
			r.EconomicVersion = platform.EconomicPolicyVersion
			r.EconomySettlement = &platform.PayoutCapView{PolicyVersion: r.EconomicVersion, PolicyHash: strings.Repeat("a", 64), GrossPayoutUnits: r.PayoutUnits, CreditedPayoutUnits: r.StakeUnits, WithheldUnits: r.NetUnits}
			r.BalanceAfterUnits = r.BalanceBeforeUnits
			change(&r)
			_, e := projectSummonRound(r)
			requireFault(t, e, "UPSTREAM_UNAVAILABLE")
		})
	}
}
