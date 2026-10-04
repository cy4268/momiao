package botgames

import (
	"math"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/games"
	bj "github.com/cy4268/momiao/internal/games/blackjack"
	"github.com/cy4268/momiao/internal/platform"
)

type BlackjackPrepared struct {
	Action            string          `json:"action"`
	Quote             string          `json:"quote,omitempty"`
	InitialWager      string          `json:"initial_wager,omitempty"`
	StakeUnits        string          `json:"stake_units,omitempty"`
	AvailableUnits    string          `json:"available_units,omitempty"`
	MinimumWagerUnits string          `json:"minimum_wager_units,omitempty"`
	MaximumWagerUnits string          `json:"maximum_wager_units,omitempty"`
	Ruleset           string          `json:"ruleset,omitempty"`
	RulesText         string          `json:"rules_text,omitempty"`
	ServerSeedHash    string          `json:"server_seed_hash,omitempty"`
	CommitmentID      string          `json:"commitment_id,omitempty"`
	ExpiresAt         string          `json:"expires_at,omitempty"`
	Round             *BlackjackRound `json:"round,omitempty"`
}
type BlackjackHand struct {
	ID             string        `json:"hand_id"`
	Index          int           `json:"hand_index"`
	Cards          []uint16      `json:"cards"`
	StakeUnits     string        `json:"stake_units"`
	Status         bj.HandStatus `json:"hand_state"`
	Value          bj.Value      `json:"value"`
	Natural        bool          `json:"is_natural"`
	Result         string        `json:"result"`
	PayoutUnits    string        `json:"payout_units"`
	NetChangeUnits string        `json:"net_change_units"`
}
type BlackjackSettlement struct {
	SettledAt           string `json:"settled_at"`
	Result              string `json:"result"`
	GrossPayoutUnits    string `json:"gross_payout_units"`
	FairReturnUnits     string `json:"fair_return_units"`
	WithheldUnits       string `json:"withheld_units"`
	CreditedPayoutUnits string `json:"credited_payout_units"`
	ActualNetUnits      string `json:"actual_net_units"`
}
type BlackjackRound struct {
	Quote              string               `json:"quote"`
	ExpiresAt          string               `json:"expires_at"`
	RoundID            string               `json:"round_id"`
	CreatedAt          string               `json:"created_at"`
	Status             string               `json:"status"`
	InitialWager       string               `json:"initial_wager"`
	Ruleset            string               `json:"ruleset"`
	RoundVersion       string               `json:"round_version"`
	Hands              []BlackjackHand      `json:"hands"`
	DealerCards        []uint16             `json:"dealer_cards"`
	DealerRevealed     bool                 `json:"dealer_revealed"`
	DealerTotal        *bj.Value            `json:"dealer_total"`
	ActiveHandID       string               `json:"active_hand_id"`
	LegalActions       []string             `json:"legal_actions"`
	LastPlayerActionAt string               `json:"last_player_action_at"`
	AutoResolveAt      string               `json:"auto_resolve_at"`
	StakeUnits         string               `json:"stake_units"`
	Settlement         *BlackjackSettlement `json:"settlement"`
}
type BlackjackActionResult struct {
	ActionID             string         `json:"action_id"`
	RequestID            string         `json:"request_id"`
	ActionType           string         `json:"action_type"`
	HandID               string         `json:"hand_id"`
	ExpectedRoundVersion string         `json:"expected_round_version"`
	AppliedRoundVersion  string         `json:"applied_round_version"`
	AdditionalStakeUnits string         `json:"additional_stake_units"`
	Round                BlackjackRound `json:"round"`
}

func (s *Service) projectBlackjackRound(subject string, user int64, r games.GameRound) (BlackjackRound, error) {
	invalid := Fault{Code: "UPSTREAM_UNAVAILABLE"}
	var out BlackjackRound
	if r.RecoveryState != "NORMAL" {
		return out, Fault{Code: "BLACKJACK_NEEDS_REVIEW"}
	}
	initial, valid := blackjackWagerUnits(r.Input.InitialWager)
	p := r.Blackjack
	if !valid || r.Input != (games.CreateInput{Type: "BLACKJACK", InitialWager: r.Input.InitialWager}) || r.Game != "blackjack" || !validBlackjackRecordID(r.ID) || !scratchTimestamp(r.CreatedAt) || p == nil || r.Algorithm != bj.AlgorithmVersion || (r.Ruleset != bj.RulesetVersion && r.Ruleset != bj.FairRulesetVersion) || string(p.Phase) != r.State || p.Version < 1 || p.Version == math.MaxInt64 || r.StakeUnits < initial || r.StakeUnits > initial*8 || p.TotalStakeUnits != r.StakeUnits || len(p.Hands) < 1 || len(p.Hands) > 4 || (r.EconomicVersion != "" && r.EconomicVersion != platform.EconomicPolicyVersion) {
		return out, invalid
	}
	out = BlackjackRound{RoundID: r.ID, CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339Nano), Status: r.State, InitialWager: r.Input.InitialWager, Ruleset: r.Ruleset, RoundVersion: strconv.FormatInt(p.Version, 10), ActiveHandID: p.ActiveHandID, StakeUnits: strconv.FormatInt(r.StakeUnits, 10), DealerRevealed: p.DealerRevealed, DealerCards: append([]uint16{}, p.DealerCards...), Hands: make([]BlackjackHand, 0, len(p.Hands)), LegalActions: []string{}}
	seen := map[string]bool{}
	prev := -1
	var total, payout, activeStake int64
	cards := len(p.DealerCards)
	activeFound := false
	for _, h := range p.Hands {
		if !validBlackjackRecordID(h.ID) || seen[h.ID] || h.Index <= prev || h.Index > 7 || len(h.Cards) < 2 || len(h.Cards) > 312 || (h.StakeUnits != initial && h.StakeUnits != initial*2) || h.PayoutUnits < 0 || h.PayoutUnits > math.MaxInt64-payout || !validBlackjackValue(h.Value) {
			return BlackjackRound{}, invalid
		}
		seen[h.ID] = true
		prev = h.Index
		total += h.StakeUnits
		payout += h.PayoutUnits
		cards += len(h.Cards)
		switch h.Status {
		case bj.Active, bj.Stood, bj.Bust, bj.DoubledComplete, bj.SplitAcesComplete, bj.NaturalComplete:
		default:
			return BlackjackRound{}, invalid
		}
		switch h.Result {
		case "", "LOSS", "BUST", "PUSH", "NORMAL_WIN", "NATURAL":
		default:
			return BlackjackRound{}, invalid
		}
		if h.ID == p.ActiveHandID {
			activeFound = h.Status == bj.Active
			activeStake = h.StakeUnits
		}
		if r.State == "SETTLED" && (h.Status == bj.Active || h.Result == "" || h.NetChangeUnits != h.PayoutUnits-h.StakeUnits) {
			return BlackjackRound{}, invalid
		}
		for _, c := range h.Cards {
			if c > 51 {
				return BlackjackRound{}, invalid
			}
		}
		out.Hands = append(out.Hands, BlackjackHand{ID: h.ID, Index: h.Index, Cards: append([]uint16{}, h.Cards...), StakeUnits: strconv.FormatInt(h.StakeUnits, 10), Status: h.Status, Value: h.Value, Natural: h.Natural, Result: h.Result, PayoutUnits: strconv.FormatInt(h.PayoutUnits, 10), NetChangeUnits: strconv.FormatInt(h.NetChangeUnits, 10)})
	}
	if total != r.StakeUnits || cards > 312 {
		return BlackjackRound{}, invalid
	}
	for _, c := range p.DealerCards {
		if c > 51 {
			return BlackjackRound{}, invalid
		}
	}
	if !p.LastPlayerActionAt.IsZero() || !p.AutoResolveAt.IsZero() {
		if !scratchTimestamp(p.LastPlayerActionAt) || !scratchTimestamp(p.AutoResolveAt) || p.LastPlayerActionAt.Before(r.CreatedAt) || !p.AutoResolveAt.Equal(p.LastPlayerActionAt.Add(24*time.Hour)) {
			return BlackjackRound{}, invalid
		}
		out.LastPlayerActionAt = p.LastPlayerActionAt.UTC().Format(time.RFC3339Nano)
		out.AutoResolveAt = p.AutoResolveAt.UTC().Format(time.RFC3339Nano)
	}
	switch r.State {
	case "PLAYER_TURN":
		if p.DealerRevealed || len(p.DealerCards) != 1 || p.DealerTotal != nil || !activeFound || out.LastPlayerActionAt == "" || r.SettledAt != nil || r.EconomySettlement != nil {
			return BlackjackRound{}, invalid
		}
		actions := map[string]bool{}
		for _, a := range p.LegalActions {
			v := string(a)
			if !validBlackjackAction(v) || actions[v] {
				return BlackjackRound{}, invalid
			}
			actions[v] = true
			if (a == bj.Double || a == bj.Split) && r.EconomicVersion == platform.EconomicPolicyVersion && activeStake > platform.SinglePlayerMaxUnits-r.StakeUnits {
				continue
			}
			out.LegalActions = append(out.LegalActions, v)
		}
		if !actions["HIT"] || !actions["STAND"] {
			return BlackjackRound{}, invalid
		}
	case "SETTLED":
		if !p.DealerRevealed || len(p.DealerCards) < 2 || p.DealerTotal == nil || !validBlackjackValue(*p.DealerTotal) || p.ActiveHandID != "" || len(p.LegalActions) != 0 || r.SettledAt == nil || !scratchTimestamp(*r.SettledAt) || r.SettledAt.Before(r.CreatedAt) || (!p.LastPlayerActionAt.IsZero() && r.SettledAt.Before(p.LastPlayerActionAt)) || p.FairReturnUnits < 0 || p.FairReturnUnits > initial || payout > math.MaxInt64-p.FairReturnUnits || payout+p.FairReturnUnits != r.PayoutUnits || p.TotalPayoutUnits != r.PayoutUnits || p.NetChangeUnits != r.NetUnits || r.NetUnits != r.PayoutUnits-r.StakeUnits || p.Class != string(r.Outcome) || (r.Ruleset == bj.RulesetVersion && p.FairReturnUnits != 0) {
			return BlackjackRound{}, invalid
		}
		switch r.Outcome {
		case games.Win, games.Loss, games.BreakEven:
		default:
			return BlackjackRound{}, invalid
		}
		v := *p.DealerTotal
		out.DealerTotal = &v
		gross, withheld, credit, net := r.PayoutUnits, int64(0), r.PayoutUnits, r.NetUnits
		if cap := r.EconomySettlement; cap != nil {
			if r.EconomicVersion != platform.EconomicPolicyVersion || cap.PolicyVersion != r.EconomicVersion || !validHash(cap.PolicyHash) || cap.NeutralReturnUnits != 0 || cap.GrossPayoutUnits != gross || cap.WithheldUnits < 0 || cap.WithheldUnits > max(gross-r.StakeUnits, 0) || cap.CreditedPayoutUnits != gross-cap.WithheldUnits || cap.ActualNetUnits != cap.CreditedPayoutUnits-r.StakeUnits {
				return BlackjackRound{}, invalid
			}
			withheld, credit, net = cap.WithheldUnits, cap.CreditedPayoutUnits, cap.ActualNetUnits
		} else if r.EconomicVersion != "" {
			return BlackjackRound{}, invalid
		}
		out.Settlement = &BlackjackSettlement{SettledAt: r.SettledAt.UTC().Format(time.RFC3339Nano), Result: string(r.Outcome), GrossPayoutUnits: strconv.FormatInt(gross, 10), FairReturnUnits: strconv.FormatInt(p.FairReturnUnits, 10), WithheldUnits: strconv.FormatInt(withheld, 10), CreditedPayoutUnits: strconv.FormatInt(credit, 10), ActualNetUnits: strconv.FormatInt(net, 10)}
	default:
		return BlackjackRound{}, invalid
	}
	iat := s.now().Unix()
	if iat <= 0 || iat > math.MaxInt64-120 {
		return BlackjackRound{}, invalid
	}
	out.ExpiresAt = strconv.FormatInt(iat+120, 10)
	payload := blackjackQuotePayload{Version: 1, Game: "blackjack", Kind: "ROUND", Subject: subject, Binding: s.binding(subject, user), InitialWager: r.Input.InitialWager, IssuedAt: strconv.FormatInt(iat, 10), ExpiresAt: out.ExpiresAt, RoundID: r.ID, RoundVersion: out.RoundVersion, ActiveHandID: &out.ActiveHandID, StakeUnits: out.StakeUnits, ActiveHandStakeUnits: strconv.FormatInt(activeStake, 10)}
	q, e := s.signBlackjackQuote(payload)
	if e != nil {
		return BlackjackRound{}, e
	}
	out.Quote = q
	return out, nil
}
func validBlackjackValue(v bj.Value) bool {
	return v.HardTotal >= 2 && v.HardTotal <= 3120 && v.BestTotal >= v.HardTotal && v.BestTotal <= 3120 && ((v.Soft && v.BestTotal == v.HardTotal+10 && v.BestTotal <= 21) || (!v.Soft && v.BestTotal == v.HardTotal))
}
func validBlackjackAction(a string) bool {
	return a == "HIT" || a == "STAND" || a == "DOUBLE" || a == "SPLIT"
}
