package games

import (
	"github.com/cy4268/momiao/internal/games/blackjack"
	"github.com/cy4268/momiao/internal/games/slot"
)

// These payloads name every frozen resource used by the already reviewed pure
// engines. Large-sample/exhaustive mathematics is bound separately, not rerun by
// config construction or inferred from a hardcoded RTP display value.
func SlotV1(version string) (Config, error) {
	f := slot.FrozenConfig()
	c := Config{binding: ConfigBinding{Game: "slot", Version: version, AlgorithmVersion: slot.AlgorithmVersion, RulesetVersion: slot.RulesetVersion, SchemaVersion: slot.ConfigSchemaVersion}}
	return sealConfig(c, map[string]any{
		"reel_strip_version": f.ReelStripVersion, "payline_version": f.PaylineVersion, "paytable_version": f.PaytableVersion,
		"symbols": f.Symbols, "reel_strips": f.ReelStrips, "paylines": f.Paylines, "paytable": f.Paytable,
		"enabled_line_count": 10, "payout_rule": "LEFT_TO_RIGHT_LONGEST_BEST_SINGLE_INTERPRETATION",
		"wild_rule": "SUBSTITUTE_OR_OWN_MAXIMUM_ONCE", "grid_order": "REEL_MAJOR_TOP_MIDDLE_BOTTOM",
	})
}

func BlackjackV1(version string) (Config, error) {
	c := Config{binding: ConfigBinding{Game: "blackjack", Version: version, AlgorithmVersion: blackjack.AlgorithmVersion, RulesetVersion: blackjack.RulesetVersion, SchemaVersion: blackjack.ConfigSchemaVersion}}
	return sealConfig(c, map[string]any{
		"deck_count": 6, "card_count": 312, "card_encoding": "DECK_TIMES_52_PLUS_SUIT_TIMES_13_PLUS_RANK",
		"shuffle_algorithm_version": blackjack.ShuffleAlgorithmVersion, "shoe_model": "FRESH_PER_ROUND",
		"initial_deal_order": []string{"PLAYER", "DEALER_UP", "PLAYER", "DEALER_HOLE"},
		"peek_rule":          "AMERICAN_ACE_OR_TEN", "dealer_rule": "S17",
		"natural_profit_numerator": 3, "natural_profit_denominator": 2, "split_21_is_natural": false,
		"double_rule": "ANY_INITIAL_TWO_CARDS", "double_after_split": true,
		"split_rule": "EQUAL_POINT_VALUE", "resplit": true, "maximum_hands": 4,
		"split_aces_draw_count": 1, "resplit_aces": false,
		"insurance": false, "even_money": false, "surrender": false, "side_bets": false,
		"inactivity_hours": 24, "inactivity_action": "SYSTEM_AUTO_STAND",
	})
}
