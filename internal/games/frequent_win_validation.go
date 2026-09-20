// Frozen mathematical proof; generated from the approved production-engine enumeration.
package games

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/cy4268/momiao/internal/games/slot"
)

const frequentSlotValidation = `{"algorithm_version":"slot-map-v1","artifact_type":"SLOT_EXHAUSTIVE","canonical_payload_sha256":"623239b5db0c2c48c807d85a51bcb8d31c4f3ff3e4a05cb66bf0a4b4ec77a767","config_hash":"bd35c1cd233ecb97512b7aa50151eaf5b1ac44ae9b158e9b5bf7d017aa44da21","config_version":"019a6000-0000-7000-8000-000000000004","implementation_key":"direct.slot.v1","result_json":"{\"cases\":33554432,\"complete\":true,\"counts\":{\"break_even\":0,\"loss\":14856192,\"no_win\":14856192,\"nonzero_payout\":18698240,\"partial_return\":0,\"win\":18698240},\"enumeration_order\":\"5 base-32 digits; reel 5 fastest; [0,0,0,0,0] first\",\"histogram_sha256\":\"c968a5a4e154d86e61c0e49425de61222f6ecb4207544f57229d3e944cf48620\",\"house_edge\":{\"decimal\":\"-0.143312036991119385\",\"fraction\":\"-2404377/16777216\"},\"initial_wager_units\":\"5000000\",\"method\":\"EXACT_ENUMERATION_PRODUCTION_LINE_EVALUATOR\",\"production_line_sequences_checked\":32768,\"rates\":{\"BREAK_EVEN\":{\"decimal\":\"0.000000000000000000\",\"fraction\":\"0/1\"},\"LOSS\":{\"decimal\":\"0.442749023437500000\",\"fraction\":\"3627/8192\"},\"NONZERO_PAYOUT\":{\"decimal\":\"0.557250976562500000\",\"fraction\":\"4565/8192\"},\"NO_WIN\":{\"decimal\":\"0.442749023437500000\",\"fraction\":\"3627/8192\"},\"PARTIAL_RETURN\":{\"decimal\":\"0.000000000000000000\",\"fraction\":\"0/1\"},\"WIN\":{\"decimal\":\"0.557250976562500000\",\"fraction\":\"4565/8192\"}},\"rtp\":{\"decimal\":\"1.143312036991119385\",\"fraction\":\"19181593/16777216\"},\"top\":{\"count\":1,\"first_lexicographic_stops\":[23,6,31,2,15],\"payout_units\":\"2600500000\",\"total_wager_multiplier\":{\"decimal\":\"520.100000000000022737\",\"fraction\":\"5201/10\"}},\"total_combinations\":33554432,\"total_net_units\":\"24043770000000\",\"total_payout_units\":\"191815930000000\",\"total_wager_units\":\"167772160000000\",\"wild_five\":{\"count\":10,\"line_multiplier\":5000,\"probability\":{\"decimal\":\"0.000000298023223877\",\"fraction\":\"5/16777216\"}}}","ruleset_version":"slot-rules-v3","source_artifact_sha256":"4a0cbc1af790ecde1c2f5a419bb67120ad047cb46190b5799dc5a4f57366fb47","source_result_sha256":"4a0cbc1af790ecde1c2f5a419bb67120ad047cb46190b5799dc5a4f57366fb47","validation_build":"frequent-win-20260920","validator_version":"game-math-validator-v3"}`

// Match the rules, then bind the proof to this immutable configuration identity.
// Cloning identical resources must not retain another version's id or hash.
func extraValidationSummary(c Config) string {
	raw := extraValidationSummaries[c.binding.Game]
	if c.binding.RulesetVersion == slot.FrequentRulesetVersion {
		raw = frequentSlotValidation
	}
	if raw == "" {
		return ""
	}
	var metadata map[string]any
	if json.Unmarshal([]byte(raw), &metadata) != nil || metadata["ruleset_version"] != c.binding.RulesetVersion {
		return ""
	}
	metadata["config_version"] = c.binding.Version
	metadata["config_hash"] = hex.EncodeToString(c.binding.Hash[:])
	digest := sha256.Sum256(c.CanonicalJSON())
	metadata["canonical_payload_sha256"] = hex.EncodeToString(digest[:])
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return ""
	}
	return string(encoded)
}
