package botgames

import (
	"context"
	"crypto/hmac"
	"math"
	"strconv"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/games/slot"
	"github.com/cy4268/momiao/internal/platform"
)

type SlotPrepareInput struct {
	RequestID  string `json:"request_id"`
	TotalWager string `json:"total_wager"`
}

func (s *Service) PrepareSlot(ctx context.Context, subject string, input SlotPrepareInput) (SlotPrepared, error) {
	stake, valid := slotWagerUnits(input.TotalWager)
	if !validID(subject) || !validID(input.RequestID) || !valid {
		return SlotPrepared{}, Fault{Code: "INVALID_REQUEST"}
	}
	user, err := s.resolve(ctx, subject)
	if err != nil {
		return SlotPrepared{}, err
	}
	b, err := s.games.Bootstrap(ctx, user, "slot")
	if err != nil {
		return SlotPrepared{}, mapFault(err)
	}
	if b.EntryAction == "MAINTENANCE" || b.Game.State == "MAINTENANCE" {
		return SlotPrepared{}, Fault{Code: "MAINTENANCE"}
	}
	if b.EntryAction != "PLAY" || b.Game.State != "PLAY" || b.Game.Slug != "slot" {
		return SlotPrepared{}, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	if b.Next == nil || !validUUID(b.Next.ID) || !validHash(b.Next.ServerSeedHash) {
		return SlotPrepared{}, Fault{Code: "COMMITMENT_INVALID"}
	}
	rules, known := slotRules(b.Next.Ruleset)
	if !known || b.Game.Config == nil || b.Game.Config.Ruleset != b.Next.Ruleset ||
		b.Game.Config.Algorithm != slot.AlgorithmVersion || b.Game.Config.Schema != "slot-config-v"+b.Next.Ruleset[len(b.Next.Ruleset)-1:] {
		return SlotPrepared{}, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	minimum, maximum, err := slotWagerLimits(b)
	if err != nil {
		return SlotPrepared{}, err
	}
	if stake < minimum {
		return SlotPrepared{}, Fault{Code: "INVALID_REQUEST"}
	}
	if stake > b.AvailableUnits {
		return SlotPrepared{}, Fault{Code: "INSUFFICIENT_CHIPS"}
	}
	if stake > maximum {
		return SlotPrepared{}, Fault{Code: "INVALID_REQUEST"}
	}
	iat := s.now().Unix()
	if iat <= 0 || iat > math.MaxInt64-120 {
		return SlotPrepared{}, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	p := slotQuotePayload{Version: 1, Game: "slot", RequestID: input.RequestID, Subject: subject,
		Binding: s.binding(subject, user), TotalWager: input.TotalWager, CommitmentID: b.Next.ID, IssuedAt: iat, ExpiresAt: iat + 120}
	quote, err := s.signSlotQuote(p)
	if err != nil {
		return SlotPrepared{}, err
	}
	return SlotPrepared{Quote: quote, TotalWager: input.TotalWager, LineCount: 10, LineStakeUnits: strconv.FormatInt(stake/10, 10),
		AvailableUnits: strconv.FormatInt(b.AvailableUnits, 10), MinimumWagerUnits: strconv.FormatInt(minimum, 10), MaximumWagerUnits: strconv.FormatInt(maximum, 10),
		Ruleset: b.Next.Ruleset, RulesText: rules, ServerSeedHash: b.Next.ServerSeedHash, CommitmentID: b.Next.ID, ExpiresAt: strconv.FormatInt(p.ExpiresAt, 10)}, nil
}

func slotWagerLimits(b games.Bootstrap) (int64, int64, error) {
	p := b.WagerPolicy
	if p.Minimum < 10*games.UnitsPerChip || p.Minimum%games.UnitsPerChip != 0 || p.Step != games.UnitsPerChip || p.MaximumMode != "NONE" || b.AvailableUnits < 0 {
		return 0, 0, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	multiplier := slot.MaxRoundLineMultiplier
	if b.Next.Ruleset == slot.FrequentRulesetVersion {
		multiplier = slot.FrequentMaxRoundLineMultiplier
	}
	// The engine guards balance - 10*lineStake + multiplier*lineStake before
	// RNG, even with an economic cap. Both products must fit before any draw.
	lineMaximum := min(math.MaxInt64/multiplier, (math.MaxInt64-b.AvailableUnits)/(multiplier-10))
	maximum := min(b.AvailableUnits, lineMaximum*10)
	if e := b.EconomicPolicy; e != nil {
		if e.Version != platform.EconomicPolicyVersion || !validHash(e.Hash) || e.SinglePlayerMaxUnits <= 0 || e.AssetCapUnits <= 0 || e.CapMode != "CLIP_PROFIT" {
			return 0, 0, Fault{Code: "UPSTREAM_UNAVAILABLE"}
		}
		maximum = min(maximum, e.SinglePlayerMaxUnits)
	}
	maximum -= maximum % games.UnitsPerChip
	return p.Minimum, maximum, nil
}

func slotWagerUnits(value string) (int64, bool) {
	if !canonicalPositive(value) {
		return 0, false
	}
	chips, err := strconv.ParseInt(value, 10, 64)
	if err != nil || chips < 10 || chips > math.MaxInt64/games.UnitsPerChip {
		return 0, false
	}
	return chips * games.UnitsPerChip, true
}

func slotRules(ruleset string) (string, bool) {
	const common = "5 列 × 3 行，固定启用 10 条线；每线下注为总下注的十分之一。从左向右连续 3–5 个相同符号中奖；W 可替代其他符号，每线只按最高的单次组合派彩。总派彩含返还金额，经济上限可能扣留利润；实际净变化以结算为准。"
	switch ruleset {
	case slot.RulesetVersion:
		return common + "使用原始 v1 转轴和派彩表。", true
	case slot.FairRulesetVersion:
		return common + "使用 v2 派彩表。", true
	case slot.FrequentRulesetVersion:
		return common + "使用 v3 转轴和派彩表。", true
	default:
		return "", false
	}
}

func (s *Service) PlaySlot(ctx context.Context, subject, quote string) (SlotResult, error) {
	return s.slotResult(ctx, subject, quote, true)
}

func (s *Service) LookupSlot(ctx context.Context, subject, quote string) (SlotResult, error) {
	return s.slotResult(ctx, subject, quote, false)
}

func (s *Service) slotResult(ctx context.Context, subject, quote string, create bool) (SlotResult, error) {
	p, err := s.verifySlotQuote(quote)
	if err != nil {
		return SlotResult{}, err
	}
	if !validID(subject) {
		return SlotResult{}, Fault{Code: "INVALID_REQUEST"}
	}
	if p.Subject != subject {
		return SlotResult{}, Fault{Code: "UNAUTHORIZED"}
	}
	user, err := s.resolve(ctx, subject)
	if err != nil {
		return SlotResult{}, err
	}
	if !hmac.Equal([]byte(p.Binding), []byte(s.binding(subject, user))) {
		return SlotResult{}, Fault{Code: "BINDING_CHANGED"}
	}
	key := "discord-slot-v1:" + p.RequestID
	input := games.CreateInput{Type: "SLOT", TotalWager: p.TotalWager}
	round, err := s.games.FindByKey(ctx, user, "slot", key)
	if err != nil {
		return SlotResult{}, mapFault(err)
	}
	if round != nil {
		if round.Input != input {
			return SlotResult{}, Fault{Code: "IDEMPOTENCY_CONFLICT"}
		}
		return projectSlotRound(*round)
	}
	if !create {
		return SlotResult{}, Fault{Code: "NOT_FOUND"}
	}
	if s.now().Unix() >= p.ExpiresAt {
		return SlotResult{}, Fault{Code: "QUOTE_EXPIRED"}
	}
	// The existing game transaction arbitrates concurrent creates and owns all
	// randomness, wallet effects, commitment validity and durable idempotency.
	created, err := s.games.Create(ctx, user, "slot", key, p.CommitmentID, input)
	if err != nil {
		return SlotResult{}, mapFault(err)
	}
	if created.Input != input {
		return SlotResult{}, Fault{Code: "IDEMPOTENCY_CONFLICT"}
	}
	return projectSlotRound(created)
}
