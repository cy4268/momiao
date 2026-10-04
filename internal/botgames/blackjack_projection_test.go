package botgames

import (
	"github.com/cy4268/momiao/internal/games"
	"testing"
)

func TestBlackjackRejectsNonV7RoundRecords(t *testing.T) {
	for _, mutate := range []func(*games.GameRound){func(r *games.GameRound) { r.ID = "00000000-0000-4000-8000-000000000003" }, func(r *games.GameRound) {
		r.Blackjack.Hands[0].ID = "00000000-0000-4000-8000-000000000004"
		r.Blackjack.ActiveHandID = r.Blackjack.Hands[0].ID
	}} {
		s, g, _, _ := newBlackjackFixture(t)
		mutate(&g.current)
		_, e := s.projectBlackjackRound(testSubject, 424242, g.current)
		wantBlackjackFault(t, e, "UPSTREAM_UNAVAILABLE")
	}
}
func TestBlackjackProjectionRejectsMalformedSnapshots(t *testing.T) {
	for name, mutate := range map[string]func(*games.GameRound){
		"wrong_game":         func(r *games.GameRound) { r.Game = "dice" },
		"other_input":        func(r *games.GameRound) { r.Input.Wager = "10" },
		"missing_projection": func(r *games.GameRound) { r.Blackjack = nil },
		"unknown_rules":      func(r *games.GameRound) { r.Ruleset = "blackjack-rules-v3" },
		"hidden_hole":        func(r *games.GameRound) { r.Blackjack.DealerCards = append(r.Blackjack.DealerCards, 9) },
		"dealer_total":       func(r *games.GameRound) { r.Blackjack.DealerTotal = &r.Blackjack.Hands[0].Value },
		"instance_card":      func(r *games.GameRound) { r.Blackjack.Hands[0].Cards[0] = 52 },
		"no_cards":           func(r *games.GameRound) { r.Blackjack.Hands[0].Cards = nil },
		"hand_index":         func(r *games.GameRound) { r.Blackjack.Hands[0].Index = 8 },
		"wrong_active":       func(r *games.GameRound) { r.Blackjack.ActiveHandID = "00000000-0000-7000-8000-000000000099" },
		"duplicate_hand":     func(r *games.GameRound) { r.Blackjack.Hands = append(r.Blackjack.Hands, r.Blackjack.Hands[0]) },
		"stake_sum":          func(r *games.GameRound) { r.StakeUnits++; r.Blackjack.TotalStakeUnits++ },
		"bad_value":          func(r *games.GameRound) { r.Blackjack.Hands[0].Value.BestTotal++ },
		"bad_timestamp":      func(r *games.GameRound) { r.Blackjack.AutoResolveAt = r.Blackjack.LastPlayerActionAt },
		"unknown_economy":    func(r *games.GameRound) { r.EconomicVersion = "economy-other" },
		"duplicate_legal":    func(r *games.GameRound) { r.Blackjack.LegalActions = append(r.Blackjack.LegalActions, "HIT") },
		"unknown_legal":      func(r *games.GameRound) { r.Blackjack.LegalActions = append(r.Blackjack.LegalActions, "SURRENDER") },
	} {
		t.Run(name, func(t *testing.T) {
			s, g, _, _ := newBlackjackFixture(t)
			mutate(&g.current)
			_, e := s.projectBlackjackRound(testSubject, 424242, g.current)
			wantBlackjackFault(t, e, "UPSTREAM_UNAVAILABLE")
		})
	}
}
