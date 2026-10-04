package botgames

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
)

func TestScratchCanonicalTiersAndRawCappedResult(t *testing.T) {
	for _, tc := range []struct {
		tier        string
		multiplier  int64
		result, net string
	}{{"LOSS", 0, "LOSS", "-5500000"}, {"BREAK_EVEN", 1, "BREAK_EVEN", "0"}, {"T2", 2, "WIN", "5500000"}, {"T3", 3, "WIN", "11000000"}, {"T5", 5, "WIN", "22000000"}, {"T10", 10, "WIN", "49500000"}, {"T25", 25, "WIN", "132000000"}, {"TOP", 100, "WIN", "544500000"}} {
		t.Run(tc.tier, func(t *testing.T) {
			s, g, _, _ := newScratchFixture(t)
			p := scratchPrepare(t, s, "11")
			r := scratchRound(tc.tier)
			g.found = &r
			result, e := s.LookupScratch(context.Background(), testSubject, p.Quote)
			if e != nil || result.PrizeTier != tc.tier || result.Multiplier != tc.multiplier || result.Result != tc.result || result.ActualNetUnits != tc.net {
				t.Fatal(result, e)
			}
			for i, c := range result.Cells {
				if c.Symbol != r.Scratch.Cells[i].Symbol || c.Matching != r.Scratch.Cells[i].Matching {
					t.Fatal("stored cell order replaced")
				}
			}
		})
	}
	s, g, _, _ := newScratchFixture(t)
	p := scratchPrepare(t, s, "11")
	r := scratchRound("T2")
	r.EconomicVersion = platform.EconomicPolicyVersion
	r.EconomySettlement = &platform.PayoutCapView{PolicyVersion: platform.EconomicPolicyVersion, PolicyHash: strings.Repeat("a", 64), GrossPayoutUnits: 11000000, WithheldUnits: 5500000, CreditedPayoutUnits: 5500000, ActualNetUnits: 0}
	r.BalanceAfterUnits = r.BalanceBeforeUnits
	g.found = &r
	result, e := s.LookupScratch(context.Background(), testSubject, p.Quote)
	if e != nil || result.Result != "WIN" || result.ActualNetUnits != "0" || result.WithheldUnits != "5500000" {
		t.Fatal(result, e)
	}
}

func TestScratchRejectsMalformedOriginalBeforePresentation(t *testing.T) {
	mutations := map[string]func(*games.GameRound){
		"wrong_game":         func(r *games.GameRound) { r.Game = "dice" },
		"unsettled":          func(r *games.GameRound) { r.State = "ACTIVE" },
		"nil_typed":          func(r *games.GameRound) { r.Scratch = nil },
		"noncanonical_wager": func(r *games.GameRound) { r.Input.Wager = "011" },
		"extra_input":        func(r *games.GameRound) { r.Input.Mode = "SINGLE" },
		"invalid_round":      func(r *games.GameRound) { r.ID = "invalid" },
		"ruleset":            func(r *games.GameRound) { r.Ruleset = "scratch-rules-v2" },
		"algorithm":          func(r *games.GameRound) { r.Algorithm = "scratch-map-v2" },
		"created_zero":       func(r *games.GameRound) { r.CreatedAt = time.Time{} },
		"settled_nil":        func(r *games.GameRound) { r.SettledAt = nil },
		"settled_earlier":    func(r *games.GameRound) { v := r.CreatedAt.Add(-time.Second); r.SettledAt = &v },
		"completion_earlier": func(r *games.GameRound) { v := r.CreatedAt; r.PresentationCompletedAt = &v },
		"completion_out_of_range": func(r *games.GameRound) {
			v := time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
			r.PresentationCompletedAt = &v
		},
		"stake":                       func(r *games.GameRound) { r.StakeUnits++ },
		"gross":                       func(r *games.GameRound) { r.PayoutUnits++ },
		"net":                         func(r *games.GameRound) { r.NetUnits++ },
		"balance_before":              func(r *games.GameRound) { r.BalanceBeforeUnits = 1 },
		"balance_after":               func(r *games.GameRound) { r.BalanceAfterUnits++ },
		"tier":                        func(r *games.GameRound) { r.Scratch.Tier = "T4" },
		"multiplier":                  func(r *games.GameRound) { r.Scratch.Reward.PayoutMultiplier = 3 },
		"cost":                        func(r *games.GameRound) { r.Scratch.Reward.CostMultiplier = 2 },
		"reward_result":               func(r *games.GameRound) { r.Scratch.Reward.Outcome = games.Loss },
		"round_result":                func(r *games.GameRound) { r.Outcome = games.Loss },
		"symbol":                      func(r *games.GameRound) { r.Scratch.Cells[0].Symbol = "P4" },
		"matching_false":              func(r *games.GameRound) { r.Scratch.Cells[1].Matching = false },
		"matching_true":               func(r *games.GameRound) { r.Scratch.Cells[0].Matching = true },
		"noncanonical_winning_filler": func(r *games.GameRound) { r.Scratch.Cells[0] = r.Scratch.Cells[2] },
		"noncanonical_loss":           func(r *games.GameRound) { *r = scratchRound("LOSS"); r.Scratch.Cells[8].Symbol = "P1" },
		"loss_matching":               func(r *games.GameRound) { *r = scratchRound("LOSS"); r.Scratch.Cells[0].Matching = true },
		"missing_economy":             func(r *games.GameRound) { r.EconomicVersion = platform.EconomicPolicyVersion },
		"negative_withheld": func(r *games.GameRound) {
			r.EconomicVersion = platform.EconomicPolicyVersion
			r.EconomySettlement = &platform.PayoutCapView{PolicyVersion: platform.EconomicPolicyVersion, PolicyHash: strings.Repeat("a", 64), GrossPayoutUnits: r.PayoutUnits, WithheldUnits: -1, CreditedPayoutUnits: r.PayoutUnits + 1, ActualNetUnits: r.NetUnits + 1}
			r.BalanceAfterUnits++
		},
		"clipped_principal": func(r *games.GameRound) {
			r.EconomicVersion = platform.EconomicPolicyVersion
			r.EconomySettlement = &platform.PayoutCapView{PolicyVersion: platform.EconomicPolicyVersion, PolicyHash: strings.Repeat("a", 64), GrossPayoutUnits: r.PayoutUnits, WithheldUnits: r.PayoutUnits, CreditedPayoutUnits: 0, ActualNetUnits: -r.StakeUnits}
			r.BalanceAfterUnits = r.BalanceBeforeUnits - r.StakeUnits
		},
	}
	for name, change := range mutations {
		t.Run(name, func(t *testing.T) {
			s, g, _, _ := newScratchFixture(t)
			p := scratchPrepare(t, s, "11")
			r := scratchRound("T2")
			change(&r)
			g.found = &r
			_, e := s.LookupScratch(context.Background(), testSubject, p.Quote)
			if e == nil {
				t.Fatal("malformed stored ticket projected")
			}
			if g.revealCalls != 0 || g.createCalls != 0 {
				t.Fatal("malformed original mutated")
			}
			g.bootstrap.EntryAction = "RESUME"
			g.bootstrap.ScratchBlocker = &r
			g.bootstrap.Next = nil
			_, e = s.PrepareScratch(context.Background(), testSubject, ScratchPrepareInput{testRequest, "99"})
			if e == nil {
				t.Fatal("malformed resume preview accepted")
			}
		})
	}
}
