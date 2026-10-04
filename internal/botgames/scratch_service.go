package botgames

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
)

type ScratchPrepareInput struct {
	RequestID string `json:"request_id"`
	Wager     string `json:"wager"`
}

// Scratch alone has a second, presentation-only transaction. Keep this narrow
// capability separate so the other games need not implement reveal operations.
type scratchGameService interface {
	Read(context.Context, int64, string) (games.GameRound, error)
	RevealComplete(context.Context, int64, string, string) (games.GameRound, error)
}

func (s *Service) PrepareScratch(ctx context.Context, subject string, input ScratchPrepareInput) (ScratchPrepared, error) {
	stake, valid := scratchWagerUnits(input.Wager)
	if !validID(subject) || !validID(input.RequestID) || !valid {
		return ScratchPrepared{}, Fault{Code: "INVALID_REQUEST"}
	}
	user, err := s.resolve(ctx, subject)
	if err != nil {
		return ScratchPrepared{}, err
	}
	b, err := s.games.Bootstrap(ctx, user, "scratch")
	if err != nil {
		return ScratchPrepared{}, mapFault(err)
	}
	if b.Game.Slug != "scratch" {
		return ScratchPrepared{}, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	iat := s.now().Unix()
	if iat <= 0 || iat > math.MaxInt64-120 {
		return ScratchPrepared{}, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	p := scratchQuotePayload{Version: 1, Game: "scratch", RequestID: input.RequestID, Subject: subject, Binding: s.binding(subject, user), Wager: input.Wager, IssuedAt: iat, ExpiresAt: iat + 120}
	var prepared ScratchPrepared
	if b.EntryAction == "RESUME" {
		// Bootstrap may expose RESUME even during maintenance. This branch never
		// needs a new commitment or purchase terms and cannot debit a wallet.
		if b.ScratchBlocker == nil || b.ScratchBlocker.PresentationCompletedAt != nil {
			return ScratchPrepared{}, Fault{Code: "UPSTREAM_UNAVAILABLE"}
		}
		r, err := projectScratchRound(*b.ScratchBlocker, "RESUME", false)
		if err != nil {
			return ScratchPrepared{}, err
		}
		p.Action, p.Wager, p.RoundID = "RESUME", r.Wager, r.RoundID
		prepared = ScratchPrepared{Action: p.Action, Wager: r.Wager, StakeUnits: r.StakeUnits, AdditionalStakeUnits: "0", RoundID: r.RoundID, CreatedAt: r.CreatedAt, Ruleset: r.Ruleset, RulesText: "已有未揭晓券，本次只揭晓原券，不再次扣款；本次输入的筹码不会用于购买新券。确认后一次揭晓原券九格，三个相同图案即为对应奖项；预览在 120 秒固定期限后到期。"}
	} else {
		if b.EntryAction == "MAINTENANCE" || b.Game.State == "MAINTENANCE" {
			return ScratchPrepared{}, Fault{Code: "MAINTENANCE"}
		}
		if b.EntryAction != "PLAY" || b.Game.State != "PLAY" || b.ScratchBlocker != nil {
			return ScratchPrepared{}, Fault{Code: "UPSTREAM_UNAVAILABLE"}
		}
		if b.Next == nil || !validUUID(b.Next.ID) || !validHash(b.Next.ServerSeedHash) {
			return ScratchPrepared{}, Fault{Code: "COMMITMENT_INVALID"}
		}
		prizes, err := scratchPrizes(b)
		if err != nil {
			return ScratchPrepared{}, err
		}
		minimum, maximum, err := scratchWagerLimits(b)
		if err != nil {
			return ScratchPrepared{}, err
		}
		if stake < minimum {
			return ScratchPrepared{}, Fault{Code: "INVALID_REQUEST"}
		}
		if stake > b.AvailableUnits {
			return ScratchPrepared{}, Fault{Code: "INSUFFICIENT_CHIPS"}
		}
		if stake > maximum {
			return ScratchPrepared{}, Fault{Code: "INVALID_REQUEST"}
		}
		p.Action, p.CommitmentID = "PURCHASE", b.Next.ID
		prepared = ScratchPrepared{Action: p.Action, Wager: p.Wager, StakeUnits: strconv.FormatInt(stake, 10), AdditionalStakeUnits: strconv.FormatInt(stake, 10), AvailableUnits: strconv.FormatInt(b.AvailableUnits, 10), MinimumWagerUnits: strconv.FormatInt(minimum, 10), MaximumWagerUnits: strconv.FormatInt(maximum, 10), Prizes: prizes, Ruleset: b.Next.Ruleset, RulesText: "确认后按本次筹码购买一张券并一次揭晓九格；三个相同图案即为对应奖项。当前奖池概率、私密可用余额与下注范围以上方预览为准。总派彩包含返还本金，经济上限可能扣留利润，实际净变化以结算为准；预览在 120 秒固定期限后到期。", ServerSeedHash: b.Next.ServerSeedHash, CommitmentID: b.Next.ID}
	}
	prepared.Quote, err = s.signScratchQuote(p)
	if err != nil {
		return ScratchPrepared{}, err
	}
	prepared.ExpiresAt = strconv.FormatInt(p.ExpiresAt, 10)
	return prepared, nil
}

// Rebuild the immutable configuration binding; never sample RNG in a preview.
func scratchPrizes(b games.Bootstrap) ([]games.Prize, error) {
	invalid := Fault{Code: "UPSTREAM_UNAVAILABLE"}
	c, n := b.Game.Config, b.Next
	if c == nil || n == nil || c.Schema != "scratch-config-v1" || c.Ruleset != "scratch-rules-v1" || c.Algorithm != games.ScratchAlgorithm || n.Ruleset != c.Ruleset || n.Algorithm != c.Algorithm || !validUUID(c.Version) || n.ConfigVersion != c.Version || !validHash(c.Hash) || n.ConfigHash != c.Hash {
		return nil, invalid
	}
	var version string
	if exactScratchObject(n.Resources, map[string]any{"prize_table_version": &version}) != nil || version == "" {
		return nil, invalid
	}
	config, err := games.NewScratchConfig(c.Version, version, c.Prizes)
	if err != nil {
		return nil, invalid
	}
	binding := config.Binding()
	if hex.EncodeToString(binding.Hash[:]) != c.Hash {
		return nil, invalid
	}
	return config.Prizes(), nil
}
func scratchWagerUnits(value string) (int64, bool) {
	if !canonicalPositive(value) {
		return 0, false
	}
	chips, err := strconv.ParseInt(value, 10, 64)
	if err != nil || chips < 10 || chips > math.MaxInt64/(100*games.UnitsPerChip) {
		return 0, false
	}
	return chips * games.UnitsPerChip, true
}
func scratchWagerLimits(b games.Bootstrap) (int64, int64, error) {
	p := b.WagerPolicy
	invalid := Fault{Code: "UPSTREAM_UNAVAILABLE"}
	if p.Minimum < 10*games.UnitsPerChip || p.Minimum%games.UnitsPerChip != 0 || p.Step != games.UnitsPerChip || p.MaximumMode != "NONE" || b.AvailableUnits < 0 || b.Next == nil {
		return 0, 0, invalid
	}
	// Match both gross and post-payout balance overflow checks BEFORE RNG.
	maximum := min(b.AvailableUnits, math.MaxInt64/100, (math.MaxInt64-b.AvailableUnits)/99)
	if e := b.EconomicPolicy; e != nil {
		if e.Version != platform.EconomicPolicyVersion || !validHash(e.Hash) || e.SinglePlayerMaxUnits <= 0 || e.AssetCapUnits <= 0 || e.CapMode != "CLIP_PROFIT" || b.Next.EconomicVersion != e.Version {
			return 0, 0, invalid
		}
		maximum = min(maximum, e.SinglePlayerMaxUnits)
	} else if b.Next.EconomicVersion != "" {
		return 0, 0, invalid
	}
	maximum -= maximum % games.UnitsPerChip
	return p.Minimum, maximum, nil
}
func (s *Service) PlayScratch(ctx context.Context, subject, quote string) (ScratchResult, error) {
	return s.scratchResult(ctx, subject, quote, true)
}

// LookupScratch can complete presentation on the original settled ticket.
// Unlike lookup for older games it is not entirely read-only, but never buys.
func (s *Service) LookupScratch(ctx context.Context, subject, quote string) (ScratchResult, error) {
	return s.scratchResult(ctx, subject, quote, false)
}
func (s *Service) scratchResult(ctx context.Context, subject, quote string, create bool) (ScratchResult, error) {
	p, err := s.verifyScratchQuote(quote)
	if err != nil {
		return ScratchResult{}, err
	}
	if !validID(subject) {
		return ScratchResult{}, Fault{Code: "INVALID_REQUEST"}
	}
	if p.Subject != subject {
		return ScratchResult{}, Fault{Code: "UNAUTHORIZED"}
	}
	user, err := s.resolve(ctx, subject)
	if err != nil {
		return ScratchResult{}, err
	}
	if !hmac.Equal([]byte(p.Binding), []byte(s.binding(subject, user))) {
		return ScratchResult{}, Fault{Code: "BINDING_CHANGED"}
	}
	g, ok := s.games.(scratchGameService)
	if !ok {
		return ScratchResult{}, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	input := games.CreateInput{Type: "SCRATCH", Wager: p.Wager}
	var round games.GameRound
	if p.Action == "RESUME" {
		round, err = g.Read(ctx, user, p.RoundID)
		if err != nil {
			return ScratchResult{}, mapFault(err)
		}
		if round.ID != p.RoundID {
			return ScratchResult{}, Fault{Code: "UPSTREAM_UNAVAILABLE"}
		}
	} else {
		key := "discord-scratch-v1:" + p.RequestID
		found, err := s.games.FindByKey(ctx, user, "scratch", key)
		if err != nil {
			return ScratchResult{}, mapFault(err)
		}
		if found != nil {
			round = *found
		} else {
			if !create {
				return ScratchResult{}, Fault{Code: "NOT_FOUND"}
			}
			if s.now().Unix() >= p.ExpiresAt {
				return ScratchResult{}, Fault{Code: "QUOTE_EXPIRED"}
			}
			round, err = s.games.Create(ctx, user, "scratch", key, p.CommitmentID, input)
			if err != nil {
				return ScratchResult{}, mapFault(err)
			}
		}
	}
	if round.Input != input {
		return ScratchResult{}, Fault{Code: "IDEMPOTENCY_CONFLICT"}
	}
	if _, err = projectScratchRound(round, p.Action, false); err != nil {
		return ScratchResult{}, err
	}
	if round.PresentationCompletedAt == nil {
		// An existing PURCHASE key proves a confirmation reached settlement.
		// An expired RESUME preview alone proves no owner confirmation at all.
		if p.Action == "RESUME" && s.now().Unix() >= p.ExpiresAt {
			return ScratchResult{}, Fault{Code: "QUOTE_EXPIRED"}
		}
		action, err := s.scratchRevealActionID(round.ID)
		if err != nil {
			return ScratchResult{}, err
		}
		completed, err := g.RevealComplete(ctx, user, round.ID, action)
		if err != nil {
			return ScratchResult{}, mapFault(err)
		}
		if completed.ID != round.ID || completed.Input != input {
			return ScratchResult{}, Fault{Code: "UPSTREAM_UNAVAILABLE"}
		}
		round = completed
	}
	return projectScratchRound(round, p.Action, true)
}

func (s *Service) scratchRevealActionID(round string) (string, error) {
	if !validUUID(round) {
		return "", Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	raw, err := hex.DecodeString(strings.ReplaceAll(round, "-", ""))
	if err != nil || len(raw) != 16 {
		return "", Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	mac := hmac.New(sha256.New, s.key[:])
	mac.Write([]byte("bot-games.scratch.reveal.v1\x00"))
	mac.Write(raw)
	b := mac.Sum(nil)[:16]
	copy(b[:6], raw[:6])
	b[6] = (b[6] & 15) | 0x70
	b[8] = (b[8] & 63) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
