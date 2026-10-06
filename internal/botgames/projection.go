package botgames

import (
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/games"
)

type Prepared struct {
	Quote             string `json:"quote"`
	Wager             string `json:"wager"`
	Choice            string `json:"choice"`
	AvailableUnits    string `json:"available_units"`
	MinimumWagerUnits string `json:"minimum_wager_units"`
	MaximumWagerUnits string `json:"maximum_wager_units"`
	Ruleset           string `json:"ruleset"`
	RulesText         string `json:"rules_text"`
	ServerSeedHash    string `json:"server_seed_hash"`
	CommitmentID      string `json:"commitment_id"`
	ExpiresAt         string `json:"expires_at"`
}
type Result struct {
	RoundID             string   `json:"round_id"`
	CreatedAt           string   `json:"created_at"`
	SettledAt           string   `json:"settled_at"`
	Status              string   `json:"status"`
	Wager               string   `json:"wager"`
	Choice              string   `json:"choice"`
	Dice                [3]uint8 `json:"dice"`
	Total               uint8    `json:"total"`
	Result              string   `json:"result"`
	StakeUnits          string   `json:"stake_units"`
	GrossPayoutUnits    string   `json:"gross_payout_units"`
	WithheldUnits       string   `json:"withheld_units"`
	CreditedPayoutUnits string   `json:"credited_payout_units"`
	ActualNetUnits      string   `json:"actual_net_units"`
	Ruleset             string   `json:"ruleset"`
}

func projectRound(r games.GameRound) (Result, error) {
	invalid := Fault{Code: "UPSTREAM_UNAVAILABLE"}
	stake, valid := wagerUnits(r.Input.Wager)
	_, knownRules := diceRules(r.Ruleset)
	if r.Game != "dice" || r.State != "SETTLED" || !validUUID(r.ID) || r.Dice == nil ||
		r.CreatedAt.IsZero() || r.SettledAt == nil || r.SettledAt.IsZero() || r.SettledAt.Before(r.CreatedAt) ||
		r.Input.Type != "DICE" || !validChoice(r.Input.Choice) || !valid || r.StakeUnits != stake || !knownRules ||
		r.PayoutUnits < 0 || r.NetUnits != r.PayoutUnits-r.StakeUnits ||
		(r.Outcome != games.Win && r.Outcome != games.Loss && r.Outcome != games.BreakEven) {
		return Result{}, invalid
	}
	var total uint8
	for _, face := range r.Dice.Dice {
		if face < 1 || face > 6 {
			return Result{}, invalid
		}
		total += face
	}
	if total != r.Dice.Total {
		return Result{}, invalid
	}
	gross, withheld, credited, net := r.PayoutUnits, int64(0), r.PayoutUnits, r.NetUnits
	if c := r.EconomySettlement; c != nil {
		gross, withheld, credited, net = c.GrossPayoutUnits, c.WithheldUnits, c.CreditedPayoutUnits, c.ActualNetUnits
		if gross < 0 || withheld < 0 || withheld > gross || credited != gross-withheld || net != credited-r.StakeUnits {
			return Result{}, invalid
		}
	} else if r.EconomicVersion != "" {
		return Result{}, invalid
	}
	return Result{RoundID: r.ID, CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339Nano), SettledAt: r.SettledAt.UTC().Format(time.RFC3339Nano),
		Status: r.State, Wager: r.Input.Wager, Choice: r.Input.Choice, Dice: r.Dice.Dice, Total: total, Result: string(r.Outcome),
		StakeUnits: strconv.FormatInt(r.StakeUnits, 10), GrossPayoutUnits: strconv.FormatInt(gross, 10), WithheldUnits: strconv.FormatInt(withheld, 10),
		CreditedPayoutUnits: strconv.FormatInt(credited, 10), ActualNetUnits: strconv.FormatInt(net, 10), Ruleset: r.Ruleset}, nil
}

func diceRules(ruleset string) (string, bool) {
	const common = "三颗骰子，非豹子时总点数 4–10 为小、11–17 为大；猜中返还下注额的 2 倍（含本金），猜错损失本金。"
	switch ruleset {
	case "dice-rules-v1":
		return common + "豹子输。", true
	case "dice-rules-v2":
		return common + "豹子退回本金。", true
	default:
		return "", false
	}
}
