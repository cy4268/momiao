package games

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/cy4268/momiao/internal/games/fairness"
	"github.com/cy4268/momiao/internal/games/slot"
)

func TestExtraFrozenConfigBinding(t *testing.T) {
	c, err := SlotV1("01993200-0000-7000-8000-000000000004")
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		ReelStrips [5][32]slot.Symbol `json:"reel_strips"`
		Paylines   [10][5]int         `json:"paylines"`
		Paytable   [8][3]int64        `json:"paytable"`
	}
	if err = json.Unmarshal(c.CanonicalJSON(), &decoded); err != nil {
		t.Fatal(err)
	}
	f := slot.FrozenConfig()
	if decoded.ReelStrips != f.ReelStrips || decoded.Paylines != f.Paylines || decoded.Paytable != f.Paytable {
		t.Fatal("config differs from accepted engine resources")
	}
	if c.Binding().AlgorithmVersion != slot.AlgorithmVersion || !bytes.Contains(expectedResources(c), []byte(slot.ReelStripVersion)) {
		t.Fatal("missing original resource binding")
	}
	b, err := BlackjackV1("01993200-0000-7000-8000-000000000005")
	if err != nil {
		t.Fatal(err)
	}
	var rules map[string]any
	if json.Unmarshal(b.CanonicalJSON(), &rules) != nil || rules["deck_count"] != float64(6) || rules["maximum_hands"] != float64(4) || rules["dealer_rule"] != "S17" || rules["shuffle_algorithm_version"] != "blackjack-fy-v1" {
		t.Fatal("blackjack frozen rule binding missing")
	}
	for _, config := range []Config{c, b} {
		if _, err = config.Statistics(); err == nil {
			t.Fatal("external validation must not be presented as zero or unmeasured exact statistics")
		}
	}
}

func TestExtraWagerExactUnits(t *testing.T) {
	for _, tc := range []struct {
		slug    string
		input   CreateInput
		maximum int64
	}{
		{"slot", CreateInput{Type: "SLOT", TotalWager: "11"}, 550000 * 5164},
		{"blackjack", CreateInput{Type: "BLACKJACK", InitialWager: "11"}, 5500000 * 17},
	} {
		n, err := normalizeCreate(tc.slug, tc.input)
		if err != nil || n.Base != 5500000 || n.TotalStake != 5500000 || n.MaxPayout != tc.maximum {
			t.Fatalf("%s arithmetic: %#v %v", tc.slug, n, err)
		}
	}
	if _, err := normalizeCreate("slot", CreateInput{Type: "SLOT", TotalWager: "10", Wager: "10"}); err == nil {
		t.Fatal("cross-game input accepted")
	}
	if _, err := normalizeCreate("blackjack", CreateInput{Type: "BLACKJACK", InitialWager: "9223372036854775807"}); err == nil {
		t.Fatal("overflow accepted")
	}
}

func TestFrequentWinConfigurationVersions(t *testing.T) {
	config, err := loadConfigVersion("slot", "019a6000-0000-7000-8000-000000000004", []byte(`{"paytable_version":"slot-paytable-v3"}`))
	if err != nil {
		t.Fatal(err)
	}
	if config.Binding().RulesetVersion != "slot-rules-v3" {
		t.Fatalf("new slot paytable loaded as %s; want slot-rules-v3", config.Binding().RulesetVersion)
	}
	// Independent canonical/hash vectors also freeze the approved reel positions.
	sql, err := os.ReadFile("../platform/migrations/0036_frequent_win_games.sql")
	if err != nil {
		t.Fatal(err)
	}
	scratch, _ := NewScratchConfig("019a6000-0000-7000-8000-000000000002", "scratch-prize-frequent-v3", []Prize{{0, "LOSS", 40000}, {1, "BREAK_EVEN", 20000}, {2, "T2", 37330}, {3, "T3", 2000}, {5, "T5", 500}, {10, "T10", 100}, {25, "T25", 50}, {100, "TOP", 20}})
	summon, _ := NewSummonConfig("019a6000-0000-7000-8000-000000000003", "summon-prize-frequent-v3", []Prize{{0, "T0", 40000}, {1, "T1", 20000}, {2, "T2", 39000}, {5, "T3", 850}, {20, "T4", 100}, {100, "T5", 50}})
	for _, item := range []struct {
		config    Config
		hash, rtp string
	}{
		{config, "bd35c1cd233ecb97512b7aa50151eaf5b1ac44ae9b158e9b5bf7d017aa44da21", ""},
		{scratch, "e7c606cb36674021ceee5d32b3634f5f3483f6800878f869f71cf0424eadd98b", "10741/10000"},
		{summon, "e18875ce77979b04b8826fc9936f289bbe9bbb657a1677602424d0dd701f6422", "437/400"},
	} {
		if hex.EncodeToString(item.config.binding.Hash[:]) != item.hash || !bytes.Contains(sql, item.config.CanonicalJSON()) || !bytes.Contains(sql, []byte(item.hash)) {
			t.Fatalf("compiled %s configuration differs from frozen migration", item.config.binding.Game)
		}
		if item.rtp != "" {
			stats, e := item.config.Statistics()
			if e != nil || stats.RTP.RatString() != item.rtp || stats.Win.RatString() != "2/5" {
				t.Fatal("lottery probabilities", stats, e)
			}
		}
	}
	var resources map[string]string
	if json.Unmarshal(expectedResources(config), &resources) != nil || resources["reel_strip_version"] != "slot-strips-v2" || resources["paytable_version"] != "slot-paytable-v3" {
		t.Fatal("v3 resources not bound")
	}
	var metadata struct {
		Config string `json:"config_version"`
		Hash   string `json:"config_hash"`
		Result string `json:"result_json"`
	}
	if json.Unmarshal(summary(config).Validation, &metadata) != nil || metadata.Config != config.binding.Version || metadata.Hash != hex.EncodeToString(config.binding.Hash[:]) {
		t.Fatal("proof bound to another configuration")
	}
	var proof struct {
		RTP struct {
			Fraction string `json:"fraction"`
		} `json:"rtp"`
		Rates map[string]struct {
			Fraction string `json:"fraction"`
		} `json:"rates"`
	}
	if json.Unmarshal([]byte(metadata.Result), &proof) != nil || proof.RTP.Fraction != "19181593/16777216" || proof.Rates["WIN"].Fraction != "4565/8192" {
		t.Fatal("wrong exhaustive proof")
	}
	cloned, _ := SlotV3("cloned-v3")
	if json.Unmarshal(summary(cloned).Validation, &metadata) != nil || metadata.Config != "cloned-v3" || metadata.Hash != hex.EncodeToString(cloned.binding.Hash[:]) {
		t.Fatal("cloned proof retains old identity")
	}
	legacy, _ := SlotV2("019a5b00-0000-7000-8000-000000000004")
	if hex.EncodeToString(legacy.binding.Hash[:]) != "ac6a661fa9cc10fb45ff89aeca29d32ca52c3fac497fa4c575173531e7e17511" || string(summary(legacy).Validation) != extraValidationSummaries["slot"] {
		t.Fatal("v2 configuration or proof changed")
	}
	top, e := slot.ResolveFrequent(5_000_000, [5]int{23, 6, 31, 2, 15})
	if e != nil || top.TotalPayoutUnits != 2_600_500_000 {
		t.Fatal("new maximum payout", top, e)
	}
	seed := make([]byte, 32)
	commitment, _ := fairness.SeedCommitment(seed)
	fair := FairRound{ID: [16]byte{0, 0, 0, 0, 0, 0, 0x70, 0, 0x80}, Game: "slot", ClientSeed: "frequent-win-test", StreamVersion: fairness.StreamVersion, AlgorithmVersion: slot.AlgorithmVersion, ConfigVersion: config.binding.Version, ConfigHash: config.binding.Hash, ServerSeedHash: commitment}
	for nonce := int64(0); nonce < 10; nonce++ {
		fair.Nonce = nonce
		round := GameRound{}
		if _, e = deriveSlot(&round, seed, fair, config, 5_000_000); e != nil {
			t.Fatal(e)
		}
		want, e := slot.ResolveFrequent(5_000_000, round.Slot.Stops)
		if e != nil || !reflect.DeepEqual(*round.Slot, want) {
			t.Fatal("runtime did not use v3 evaluator", e)
		}
	}
	input := CreateInput{Type: "SLOT", TotalWager: "10"}
	n, e := normalizeCreateForRuleset("slot", input, slot.FrequentRulesetVersion)
	if e != nil || n.MaxPayout != top.TotalPayoutUnits {
		t.Fatal("wallet overflow reserve uses old maximum", n, e)
	}
	// A historically valid large wager remains replayable, but v3 rejects it
	// before asking the sampler for any result.
	chips := int64(math.MaxInt64) / slot.MaxRoundLineMultiplier / (slot.UnitsPerChip / 10)
	input.TotalWager = strconv.FormatInt(chips, 10)
	if _, e = normalizeCreateForRuleset("slot", input, slot.FairRulesetVersion); e != nil {
		t.Fatal("old wager bound changed", e)
	}
	if _, e = normalizeCreateForRuleset("slot", input, slot.FrequentRulesetVersion); e == nil {
		t.Fatal("v3 overflow accepted")
	}
	sampled := false
	if _, e = slot.SpinFrequent(chips*slot.UnitsPerChip, func(string, uint32) (uint32, error) { sampled = true; return 0, nil }); e != slot.ErrOverflow || sampled {
		t.Fatal("overflow reached randomness", e)
	}
}
