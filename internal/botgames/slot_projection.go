package botgames

import (
	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/games/slot"
	"github.com/cy4268/momiao/internal/platform"
	"strconv"
	"time"
)

type SlotPrepared struct {
	Quote             string `json:"quote"`
	TotalWager        string `json:"total_wager"`
	LineCount         int    `json:"line_count"`
	LineStakeUnits    string `json:"line_stake_units"`
	AvailableUnits    string `json:"available_units"`
	MinimumWagerUnits string `json:"minimum_wager_units"`
	MaximumWagerUnits string `json:"maximum_wager_units"`
	Ruleset           string `json:"ruleset"`
	RulesText         string `json:"rules_text"`
	ServerSeedHash    string `json:"server_seed_hash"`
	CommitmentID      string `json:"commitment_id"`
	ExpiresAt         string `json:"expires_at"`
}

type SlotLine struct {
	LineNumber        int    `json:"line_number"`
	InterpretedSymbol string `json:"interpreted_symbol"`
	MatchLength       int    `json:"match_length"`
	Multiplier        int64  `json:"multiplier"`
	LineStakeUnits    string `json:"line_stake_units"`
	LinePayoutUnits   string `json:"line_payout_units"`
}

type SlotResult struct {
	RoundID             string            `json:"round_id"`
	CreatedAt           string            `json:"created_at"`
	SettledAt           string            `json:"settled_at"`
	Status              string            `json:"status"`
	TotalWager          string            `json:"total_wager"`
	LineCount           int               `json:"line_count"`
	LineStakeUnits      string            `json:"line_stake_units"`
	FullGrid            [5][3]slot.Symbol `json:"full_grid"`
	Lines               [10]SlotLine      `json:"lines"`
	Result              string            `json:"result"`
	ResultDetail        string            `json:"result_detail"`
	StakeUnits          string            `json:"stake_units"`
	GrossPayoutUnits    string            `json:"gross_payout_units"`
	WithheldUnits       string            `json:"withheld_units"`
	CreditedPayoutUnits string            `json:"credited_payout_units"`
	ActualNetUnits      string            `json:"actual_net_units"`
	Ruleset             string            `json:"ruleset"`
}

func projectSlotRound(r games.GameRound) (SlotResult, error) {
	invalid := Fault{Code: "UPSTREAM_UNAVAILABLE"}
	stake, valid := slotWagerUnits(r.Input.TotalWager)
	_, known := slotRules(r.Ruleset)
	if !valid || !known || r.Input != (games.CreateInput{Type: "SLOT", TotalWager: r.Input.TotalWager}) || r.Game != "slot" || r.State != "SETTLED" || !validUUID(r.ID) || r.Slot == nil ||
		r.CreatedAt.IsZero() || r.SettledAt == nil || r.SettledAt.IsZero() || r.SettledAt.Before(r.CreatedAt) ||
		r.CreatedAt.UTC().Year() < 1 || r.CreatedAt.UTC().Year() > 9999 || r.SettledAt.UTC().Year() < 1 || r.SettledAt.UTC().Year() > 9999 ||
		r.StakeUnits != stake || r.PayoutUnits < 0 || r.NetUnits != r.PayoutUnits-stake {
		return SlotResult{}, invalid
	}
	// Replay only the stored stops with the existing pure engine. No RNG, wallet
	// effect or copied paytable is involved; every line and symbol must agree.
	resolve := slot.Resolve
	if r.Ruleset == slot.FairRulesetVersion {
		resolve = slot.ResolveFair
	}
	if r.Ruleset == slot.FrequentRulesetVersion {
		resolve = slot.ResolveFrequent
	}
	expected, err := resolve(stake, r.Slot.Stops)
	if err != nil || *r.Slot != expected || r.PayoutUnits != expected.TotalPayoutUnits || string(r.Outcome) != expected.Class {
		return SlotResult{}, invalid
	}
	gross, withheld, credited, net := r.PayoutUnits, int64(0), r.PayoutUnits, r.NetUnits
	if c := r.EconomySettlement; c != nil {
		gross, withheld, credited, net = c.GrossPayoutUnits, c.WithheldUnits, c.CreditedPayoutUnits, c.ActualNetUnits
		if r.EconomicVersion != platform.EconomicPolicyVersion || c.PolicyVersion != r.EconomicVersion || c.NeutralReturnUnits != 0 || gross != r.PayoutUnits || withheld < 0 || withheld > gross || credited != gross-withheld || net != credited-stake {
			return SlotResult{}, invalid
		}
	} else if r.EconomicVersion != "" {
		return SlotResult{}, invalid
	}
	result := SlotResult{RoundID: r.ID, CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339Nano), SettledAt: r.SettledAt.UTC().Format(time.RFC3339Nano), Status: r.State,
		TotalWager: r.Input.TotalWager, LineCount: 10, LineStakeUnits: strconv.FormatInt(stake/10, 10), FullGrid: r.Slot.Grid,
		Result: expected.Class, ResultDetail: expected.Detail, StakeUnits: strconv.FormatInt(stake, 10), GrossPayoutUnits: strconv.FormatInt(gross, 10),
		WithheldUnits: strconv.FormatInt(withheld, 10), CreditedPayoutUnits: strconv.FormatInt(credited, 10), ActualNetUnits: strconv.FormatInt(net, 10), Ruleset: r.Ruleset}
	for i, l := range r.Slot.Lines {
		result.Lines[i] = SlotLine{LineNumber: l.LineNumber, InterpretedSymbol: string(l.Symbol), MatchLength: l.MatchLength, Multiplier: l.Multiplier, LineStakeUnits: strconv.FormatInt(l.LineStakeUnits, 10), LinePayoutUnits: strconv.FormatInt(l.PayoutUnits, 10)}
	}
	return result, nil
}
