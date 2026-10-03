package botgames

import (
	"context"
	"encoding/json"
	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/games/slot"
	"github.com/cy4268/momiao/internal/platform"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"
)

func fixtureSlotRound(t *testing.T, ruleset, wager string, stops [5]int) games.GameRound {
	t.Helper()
	chips, e := strconv.ParseInt(wager, 10, 64)
	if e != nil {
		t.Fatal(e)
	}
	resolve := slot.Resolve
	if ruleset == "slot-rules-v2" {
		resolve = slot.ResolveFair
	}
	if ruleset == "slot-rules-v3" {
		resolve = slot.ResolveFrequent
	}
	result, e := resolve(chips*500000, stops)
	if e != nil {
		t.Fatal(e)
	}
	r := fixtureRound()
	r.Game = "slot"
	r.Input = games.CreateInput{Type: "SLOT", TotalWager: wager}
	r.StakeUnits = chips * 500000
	r.Slot = &result
	r.Dice = nil
	r.Ruleset = ruleset
	r.PayoutUnits = result.TotalPayoutUnits
	r.NetUnits = result.NetChangeUnits
	r.Outcome = games.Outcome(result.Class)
	return r
}

func TestSlotProjectionCapDoesNotRewriteGrossOutcome(t *testing.T) {
	s, g, _, _, _ := newSlotFixture(t)
	g.created.EconomicVersion = "economy-cap-v1"
	g.created.EconomySettlement = &platform.PayoutCapView{PolicyVersion: "economy-cap-v1", GrossPayoutUnits: 2779700000, WithheldUnits: 2774200000, CreditedPayoutUnits: 5500000, ActualNetUnits: 0}
	got, e := s.PlaySlot(context.Background(), testSubject, prepareSlotQuote(t, s))
	if e != nil {
		t.Fatal(e)
	}
	if got.Result != "WIN" || got.ResultDetail != "WIN" || got.GrossPayoutUnits != "2779700000" || got.WithheldUnits != "2774200000" || got.CreditedPayoutUnits != "5500000" || got.ActualNetUnits != "0" {
		t.Fatal(got)
	}
	if got.FullGrid[0] != [3]slot.Symbol{slot.M1, slot.W, slot.L1} || got.FullGrid[2] != [3]slot.Symbol{slot.M2, slot.W, slot.L2} || got.LineCount != 10 {
		t.Fatal("reel-major grid changed", got)
	}
	if got.CreatedAt != "2023-11-14T22:13:21.123Z" || got.SettledAt != "2023-11-14T22:13:22.123Z" {
		t.Fatal(got)
	}
	if got.Lines[0].Multiplier != 5000 || got.Lines[0].LinePayoutUnits != "2750000000" || got.Lines[0].InterpretedSymbol != "W" || got.Lines[0].MatchLength != 5 {
		t.Fatal(got.Lines[0])
	}
	if got.Lines[1].Multiplier != 0 || got.Lines[1].LinePayoutUnits != "0" || got.Lines[1].InterpretedSymbol != "" || got.Lines[1].MatchLength != 0 {
		t.Fatal(got.Lines[1])
	}
	raw, _ := json.Marshal(got)
	var fields map[string]any
	if e = json.Unmarshal(raw, &fields); e != nil {
		t.Fatal(e)
	}
	keys := []string{}
	for k, v := range fields {
		keys = append(keys, k)
		if k != "line_count" && k != "lines" && k != "full_grid" {
			if _, ok := v.(string); !ok {
				t.Fatal(k, "not string")
			}
		}
	}
	sort.Strings(keys)
	want := []string{"actual_net_units", "created_at", "credited_payout_units", "full_grid", "gross_payout_units", "line_count", "line_stake_units", "lines", "result", "result_detail", "round_id", "ruleset", "settled_at", "stake_units", "status", "total_wager", "withheld_units"}
	if !reflect.DeepEqual(keys, want) || fields["line_count"] != float64(10) {
		t.Fatalf("wrong slot result keys/types: %s", raw)
	}
	for i, line := range fields["lines"].([]any) {
		f := line.(map[string]any)
		keys = nil
		for k := range f {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		want := []string{"interpreted_symbol", "line_number", "line_payout_units", "line_stake_units", "match_length", "multiplier"}
		if !reflect.DeepEqual(keys, want) || f["line_number"] != float64(i+1) || f["line_stake_units"] != "550000" {
			t.Fatal(f)
		}
	}
}

func TestSlotProjectionRejectsCorruptStoredResult(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*games.GameRound)
	}{
		{"wrong_game", func(r *games.GameRound) { r.Game = "dice" }},
		{"unsettled", func(r *games.GameRound) { r.State = "PENDING" }},
		{"missing_slot", func(r *games.GameRound) { r.Slot = nil }},
		{"unknown_rules", func(r *games.GameRound) { r.Ruleset = "slot-rules-v99" }},
		{"grid_symbol", func(r *games.GameRound) { r.Slot.Grid[0][0] = "HACK" }},
		{"grid_orientation", func(r *games.GameRound) { r.Slot.Grid[0][0], r.Slot.Grid[0][1] = r.Slot.Grid[0][1], r.Slot.Grid[0][0] }},
		{"invalid_stop", func(r *games.GameRound) { r.Slot.Stops[0] = 32 }},
		{"line_order", func(r *games.GameRound) { r.Slot.Lines[0], r.Slot.Lines[1] = r.Slot.Lines[1], r.Slot.Lines[0] }},
		{"line_stake", func(r *games.GameRound) { r.Slot.Lines[0].LineStakeUnits++ }},
		{"line_payout", func(r *games.GameRound) { r.Slot.Lines[0].PayoutUnits++ }},
		{"multiplier", func(r *games.GameRound) { r.Slot.Lines[0].Multiplier = 5001 }},
		{"line_symbol", func(r *games.GameRound) { r.Slot.Lines[0].Symbol = slot.H1 }},
		{"match_length", func(r *games.GameRound) { r.Slot.Lines[0].MatchLength = 4 }},
		{"no_win_metadata", func(r *games.GameRound) { r.Slot.Lines[1].Symbol = slot.L1 }},
		{"negative_payout", func(r *games.GameRound) { r.PayoutUnits = -1 }},
		{"gross_disagrees", func(r *games.GameRound) { r.PayoutUnits++; r.NetUnits++ }},
		{"stake_disagrees", func(r *games.GameRound) { r.StakeUnits++ }},
		{"detail", func(r *games.GameRound) { r.Slot.Detail = "NO_WIN" }},
		{"outcome", func(r *games.GameRound) { r.Outcome = games.Loss }},
		{"missing_id", func(r *games.GameRound) { r.ID = "" }},
		{"missing_created", func(r *games.GameRound) { r.CreatedAt = time.Time{} }},
		{"missing_settled", func(r *games.GameRound) { r.SettledAt = nil }},
		{"settled_before_created", func(r *games.GameRound) { x := r.CreatedAt.Add(-time.Second); r.SettledAt = &x }},
		{"utc_time_overflow", func(r *games.GameRound) {
			r.CreatedAt = time.Date(9999, 12, 31, 23, 59, 0, 0, time.FixedZone("west", -3600))
			x := r.CreatedAt.Add(time.Second)
			r.SettledAt = &x
		}},
		{"missing_cap", func(r *games.GameRound) { r.EconomicVersion = "economy-cap-v1" }},
		{"cap_wrong_gross", func(r *games.GameRound) {
			r.EconomicVersion = "economy-cap-v1"
			r.EconomySettlement = &platform.PayoutCapView{PolicyVersion: "economy-cap-v1", GrossPayoutUnits: 5500000, CreditedPayoutUnits: 5500000, ActualNetUnits: 0}
		}},
		{"cap_negative_withheld", func(r *games.GameRound) {
			r.EconomicVersion = "economy-cap-v1"
			r.EconomySettlement = &platform.PayoutCapView{PolicyVersion: "economy-cap-v1", GrossPayoutUnits: r.PayoutUnits, WithheldUnits: -1, CreditedPayoutUnits: r.PayoutUnits + 1, ActualNetUnits: r.NetUnits + 1}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, g, _, _, _ := newSlotFixture(t)
			q := prepareSlotQuote(t, s)
			tc.change(&g.created)
			g.found = &g.created
			got, e := s.LookupSlot(context.Background(), testSubject, q)
			requireFault(t, e, "UPSTREAM_UNAVAILABLE")
			if got.RoundID != "" || g.createCalls != 0 {
				t.Fatal("corruption projected success or created")
			}
		})
	}
}

func TestSlotProjectionUsesEachHistoricalRuleset(t *testing.T) {
	for _, rules := range []string{"slot-rules-v1", "slot-rules-v2", "slot-rules-v3"} {
		t.Run(rules, func(t *testing.T) {
			s, g, _, _, _ := newSlotFixture(t)
			q := prepareSlotQuote(t, s)
			r := fixtureSlotRound(t, rules, "11", [5]int{24, 6, 31, 2, 14})
			g.found = &r
			got, e := s.LookupSlot(context.Background(), testSubject, q)
			if e != nil || got.Ruleset != rules || got.Lines[0].Multiplier != 5000 || got.Result != "WIN" {
				t.Fatal(got, e)
			}
		})
	}
}

// Hand-checked v1/v2 vectors: only middle line wins for L1x3 (4) and
// L3x3 (10), while all ten first-three-symbol prefixes miss in the loss.
func TestSlotProjectionOutcomeClasses(t *testing.T) {
	for _, tc := range []struct {
		name, rules, result, detail, gross, net string
		stops                                   [5]int
	}{
		{"no_win", "slot-rules-v1", "LOSS", "NO_WIN", "0", "-5500000", [5]int{14, 10, 5, 0, 0}},
		{"partial_return", "slot-rules-v1", "LOSS", "PARTIAL_RETURN", "2200000", "-3300000", [5]int{5, 2, 2, 0, 0}},
		{"break_even", "slot-rules-v1", "BREAK_EVEN", "BREAK_EVEN", "5500000", "0", [5]int{0, 4, 6, 0, 0}},
		{"v2_partial_return", "slot-rules-v2", "LOSS", "PARTIAL_RETURN", "2200000", "-3300000", [5]int{5, 2, 2, 0, 0}},
		{"v2_break_even", "slot-rules-v2", "BREAK_EVEN", "BREAK_EVEN", "5500000", "0", [5]int{0, 4, 6, 0, 0}},
		{"win", "slot-rules-v1", "WIN", "WIN", "2779700000", "2774200000", [5]int{24, 6, 31, 2, 14}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, g, _, _, _ := newSlotFixture(t)
			q := prepareSlotQuote(t, s)
			r := fixtureSlotRound(t, tc.rules, "11", tc.stops)
			g.found = &r
			got, e := s.LookupSlot(context.Background(), testSubject, q)
			if e != nil || got.Result != tc.result || got.ResultDetail != tc.detail || got.GrossPayoutUnits != tc.gross || got.ActualNetUnits != tc.net {
				t.Fatal(got, e)
			}
		})
	}
}
