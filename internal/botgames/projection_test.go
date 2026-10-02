package botgames

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
)

func TestProjection(t *testing.T) {
	// Using nominal GameRound fields for credited/net values must fail this test.
	r := fixtureRound()
	r.EconomicVersion = "economy-cap-v1"
	r.EconomySettlement = &platform.PayoutCapView{PolicyVersion: "economy-cap-v1", GrossPayoutUnits: 10000000, WithheldUnits: 5000000, CreditedPayoutUnits: 5000000, ActualNetUnits: 0}
	got, err := projectRound(r)
	if err != nil {
		t.Fatal(err)
	}
	if got.StakeUnits != "5000000" || got.GrossPayoutUnits != "10000000" || got.WithheldUnits != "5000000" || got.CreditedPayoutUnits != "5000000" || got.ActualNetUnits != "0" || got.Result != "WIN" {
		t.Fatal(got)
	}
	if got.Status != "SETTLED" || got.Dice != [3]uint8{4, 5, 6} || got.Total != 15 || got.Wager != "10" || got.Choice != "BIG" || got.Ruleset != "dice-rules-v2" {
		t.Fatal(got)
	}
	if got.CreatedAt != "2023-11-14T22:13:21.123Z" || got.SettledAt != "2023-11-14T22:13:22.123Z" {
		t.Fatal(got)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	want := []string{"actual_net_units", "choice", "created_at", "credited_payout_units", "dice", "gross_payout_units", "result", "round_id", "ruleset", "settled_at", "stake_units", "status", "total", "wager", "withheld_units"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("response field leak/missing field: %s", raw)
	}
	if !reflect.DeepEqual(fields["dice"], []any{float64(4), float64(5), float64(6)}) || fields["stake_units"] != "5000000" || fields["actual_net_units"] != "0" {
		t.Fatalf("wrong wire types: %s", raw)
	}
}

func TestProjectionLegacy(t *testing.T) {
	r := fixtureRound()
	r.PayoutUnits, r.NetUnits, r.Outcome = 0, -5000000, games.Loss
	got, err := projectRound(r)
	if err != nil || got.GrossPayoutUnits != "0" || got.CreditedPayoutUnits != "0" || got.WithheldUnits != "0" || got.ActualNetUnits != "-5000000" || got.Result != "LOSS" {
		t.Fatal(got, err)
	}
}

func TestProjectionRejectsIncompleteResults(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*games.GameRound)
	}{
		{"not-dice", func(r *games.GameRound) { r.Game = "slot" }},
		{"not-settled", func(r *games.GameRound) { r.State = "PENDING" }},
		{"missing-id", func(r *games.GameRound) { r.ID = "" }},
		{"missing-dice", func(r *games.GameRound) { r.Dice = nil }},
		{"bad-face", func(r *games.GameRound) { r.Dice.Dice[0] = 0 }},
		{"bad-total", func(r *games.GameRound) { r.Dice.Total = 16 }},
		{"missing-created", func(r *games.GameRound) { r.CreatedAt = time.Time{} }},
		{"missing-settled", func(r *games.GameRound) { r.SettledAt = nil }},
		{"bad-outcome", func(r *games.GameRound) { r.Outcome = "" }},
		{"missing-cap", func(r *games.GameRound) { r.EconomicVersion = "economy-cap-v1" }},
		{"negative-payout", func(r *games.GameRound) { r.PayoutUnits = -1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := fixtureRound()
			tc.change(&r)
			got, err := projectRound(r)
			requireFault(t, err, "UPSTREAM_UNAVAILABLE")
			if got.RoundID != "" {
				t.Fatal("partial success", got)
			}
		})
	}
}

func TestPreparedWireProjection(t *testing.T) {
	s, _, _, _, _ := newFixture(t)
	p, err := s.Prepare(context.Background(), testSubject, PrepareInput{testRequest, "10", "BIG"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(fields))
	for key, value := range fields {
		keys = append(keys, key)
		if _, ok := value.(string); !ok {
			t.Fatalf("%s is not a string", key)
		}
	}
	sort.Strings(keys)
	want := []string{"available_units", "choice", "commitment_id", "expires_at", "maximum_wager_units", "minimum_wager_units", "quote", "rules_text", "ruleset", "server_seed_hash", "wager"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("response field leak/missing field: %s", raw)
	}
}
