package games

import (
	"bytes"
	"encoding/json"
	"testing"

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
