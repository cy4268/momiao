package botgames

import (
	"context"
	"crypto/hmac"
	"encoding/hex"
	"errors"
	"math"
	"strconv"

	"github.com/cy4268/momiao/internal/games"
	bj "github.com/cy4268/momiao/internal/games/blackjack"
	"github.com/cy4268/momiao/internal/platform"
)

type BlackjackPrepareInput struct {
	RequestID    string `json:"request_id"`
	InitialWager string `json:"initial_wager"`
}
type BlackjackActionRequest struct {
	Quote      string `json:"quote"`
	RequestID  string `json:"request_id"`
	ActionType string `json:"action_type"`
}
type blackjackGameService interface {
	Read(context.Context, int64, string) (games.GameRound, error)
	BlackjackAction(context.Context, int64, string, games.BlackjackActionInput) (games.GameRound, error)
	FindBlackjackActionMatching(context.Context, int64, string, games.BlackjackActionInput) (*games.GameRound, error)
}

func (s *Service) PrepareBlackjack(ctx context.Context, subject string, input BlackjackPrepareInput) (BlackjackPrepared, error) {
	var out BlackjackPrepared
	if !validID(subject) || !validID(input.RequestID) {
		return out, Fault{Code: "INVALID_REQUEST"}
	}
	user, e := s.resolve(ctx, subject)
	if e != nil {
		return out, e
	}
	b, e := s.games.Bootstrap(ctx, user, "blackjack")
	if e != nil {
		return out, mapFault(e)
	}
	if b.Game.Slug != "blackjack" {
		return out, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	if b.EntryAction == "RESUME" {
		if b.Active == nil || !validBlackjackRecordID(b.Active.ID) {
			return out, Fault{Code: "UPSTREAM_UNAVAILABLE"}
		}
		r, e := s.readBlackjackRound(ctx, subject, user, b.Active.ID, "")
		if e != nil {
			return out, e
		}
		return BlackjackPrepared{Action: "RESUME", Round: &r}, nil
	}
	if b.EntryAction == "MAINTENANCE" || b.Game.State == "MAINTENANCE" {
		return out, Fault{Code: "MAINTENANCE"}
	}
	if b.EntryAction != "PLAY" || b.Game.State != "PLAY" || b.Active != nil {
		return out, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	stake, ok := blackjackWagerUnits(input.InitialWager)
	if !ok {
		return out, Fault{Code: "INVALID_REQUEST"}
	}
	if b.Next == nil || !validBlackjackRecordID(b.Next.ID) || !validHash(b.Next.ServerSeedHash) {
		return out, Fault{Code: "COMMITMENT_INVALID"}
	}
	rules, e := blackjackRules(b)
	if e != nil {
		return out, e
	}
	minimum, maximum, e := blackjackWagerLimits(b)
	if e != nil {
		return out, e
	}
	if stake < minimum {
		return out, Fault{Code: "INVALID_REQUEST"}
	}
	if stake > b.AvailableUnits {
		return out, Fault{Code: "INSUFFICIENT_CHIPS"}
	}
	if stake > maximum {
		return out, Fault{Code: "INVALID_REQUEST"}
	}
	iat := s.now().Unix()
	if iat <= 0 || iat > math.MaxInt64-120 {
		return out, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	p := blackjackQuotePayload{Version: 1, Game: "blackjack", Kind: "DEAL", Subject: subject, Binding: s.binding(subject, user), InitialWager: input.InitialWager, IssuedAt: strconv.FormatInt(iat, 10), ExpiresAt: strconv.FormatInt(iat+120, 10), RequestID: input.RequestID, CommitmentID: b.Next.ID}
	q, e := s.signBlackjackQuote(p)
	if e != nil {
		return out, e
	}
	return BlackjackPrepared{Action: "DEAL", Quote: q, InitialWager: p.InitialWager, StakeUnits: strconv.FormatInt(stake, 10), AvailableUnits: strconv.FormatInt(b.AvailableUnits, 10), MinimumWagerUnits: strconv.FormatInt(minimum, 10), MaximumWagerUnits: strconv.FormatInt(maximum, 10), Ruleset: b.Next.Ruleset, RulesText: rules, ServerSeedHash: b.Next.ServerSeedHash, CommitmentID: b.Next.ID, ExpiresAt: p.ExpiresAt}, nil
}
func blackjackWagerUnits(value string) (int64, bool) {
	if !canonicalPositive(value) {
		return 0, false
	}
	chips, e := strconv.ParseInt(value, 10, 64)
	if e != nil || chips < 10 || chips > math.MaxInt64/(17*games.UnitsPerChip) {
		return 0, false
	}
	return chips * games.UnitsPerChip, true
}
func blackjackWagerLimits(b games.Bootstrap) (int64, int64, error) {
	p := b.WagerPolicy
	invalid := Fault{Code: "UPSTREAM_UNAVAILABLE"}
	if p.Minimum < 10*games.UnitsPerChip || p.Minimum%games.UnitsPerChip != 0 || p.Step != games.UnitsPerChip || p.MaximumMode != "NONE" || b.AvailableUnits < 0 {
		return 0, 0, invalid
	}
	maximum := min(b.AvailableUnits, math.MaxInt64/17, (math.MaxInt64-b.AvailableUnits)/16)
	version := ""
	if e := b.EconomicPolicy; e != nil {
		if e.Version != platform.EconomicPolicyVersion || e.SinglePlayerMaxUnits != platform.SinglePlayerMaxUnits || e.AssetCapUnits != platform.AssetCapUnits || e.CapMode != "CLIP_PROFIT" || !validHash(e.Hash) {
			return 0, 0, invalid
		}
		version = e.Version
		maximum = min(maximum, e.SinglePlayerMaxUnits)
	}
	if b.Next == nil || b.Next.EconomicVersion != version || !validUUID(p.ID) || !validHash(p.Hash) || b.Next.PolicyVersion != p.ID || b.Next.PolicyHash != p.Hash {
		return 0, 0, invalid
	}
	return p.Minimum, maximum - maximum%games.UnitsPerChip, nil
}
func blackjackRules(b games.Bootstrap) (string, error) {
	invalid := Fault{Code: "UPSTREAM_UNAVAILABLE"}
	c, n := b.Game.Config, b.Next
	if c == nil || n == nil || !validUUID(c.Version) || !validHash(c.Hash) || n.ConfigVersion != c.Version || n.ConfigHash != c.Hash || n.Ruleset != c.Ruleset || n.Algorithm != c.Algorithm || c.Algorithm != bj.AlgorithmVersion {
		return "", invalid
	}
	var config games.Config
	var e error
	var shuffle, fair string
	fields := map[string]any{"shuffle_algorithm_version": &shuffle}
	rules := "六副牌，每局新牌靴；庄家软 17 停牌。天然 21 点按 3:2 利润结算，可加倍、分牌至最多四手，分 A 各补一张。加倍/分牌另扣当前手同额筹码；网站与 Bot 共用原局。最后成功行动后 24 小时系统自动停牌；预览 120 秒到期不改变牌局。总派彩包含本金，经济上限可能扣留利润，实际净变化以结算为准。"
	switch c.Ruleset {
	case bj.RulesetVersion:
		if c.Schema != bj.ConfigSchemaVersion {
			return "", invalid
		}
		config, e = games.BlackjackV1(c.Version)
	case bj.FairRulesetVersion:
		if c.Schema != bj.FairConfigSchemaVersion {
			return "", invalid
		}
		config, e = games.BlackjackV2(c.Version)
		fields["fair_return_version"] = &fair
		rules += "本规则另按初始下注提供公平返还，使用网站既有独立随机舍入。"
	default:
		return "", invalid
	}
	if e != nil {
		return "", invalid
	}
	binding := config.Binding()
	if hex.EncodeToString(binding.Hash[:]) != c.Hash || exactScratchObject(n.Resources, fields) != nil || shuffle != bj.ShuffleAlgorithmVersion || (c.Ruleset == bj.FairRulesetVersion && fair != bj.FairReturnVersion) {
		return "", invalid
	}
	return rules, nil
}
func (s *Service) bindBlackjack(ctx context.Context, subject, quote, kind string) (blackjackQuotePayload, int64, error) {
	p, e := s.verifyBlackjackQuote(quote, kind)
	if e != nil {
		return p, 0, e
	}
	if !validID(subject) {
		return p, 0, Fault{Code: "INVALID_REQUEST"}
	}
	if p.Subject != subject {
		return p, 0, Fault{Code: "UNAUTHORIZED"}
	}
	user, e := s.resolve(ctx, subject)
	if e != nil {
		return p, 0, e
	}
	if !hmac.Equal([]byte(p.Binding), []byte(s.binding(subject, user))) {
		return p, 0, Fault{Code: "BINDING_CHANGED"}
	}
	return p, user, nil
}
func (s *Service) PlayBlackjack(ctx context.Context, subject, quote string) (BlackjackRound, error) {
	return s.blackjackDealResult(ctx, subject, quote, true)
}
func (s *Service) LookupBlackjack(ctx context.Context, subject, quote string) (BlackjackRound, error) {
	return s.blackjackDealResult(ctx, subject, quote, false)
}
func (s *Service) blackjackDealResult(ctx context.Context, subject, quote string, create bool) (BlackjackRound, error) {
	p, user, e := s.bindBlackjack(ctx, subject, quote, "DEAL")
	if e != nil {
		return BlackjackRound{}, e
	}
	key := "discord-blackjack-v1:" + p.RequestID
	input := games.CreateInput{Type: "BLACKJACK", InitialWager: p.InitialWager}
	r, e := s.games.FindByKey(ctx, user, "blackjack", key)
	if e != nil {
		return BlackjackRound{}, mapFault(e)
	}
	if r == nil {
		if !create {
			return BlackjackRound{}, Fault{Code: "NOT_FOUND"}
		}
		exp, _ := blackjackDecimal(p.ExpiresAt)
		if s.now().Unix() >= exp {
			return BlackjackRound{}, Fault{Code: "QUOTE_EXPIRED"}
		}
		created, e := s.games.Create(ctx, user, "blackjack", key, p.CommitmentID, input)
		if e != nil {
			return BlackjackRound{}, mapFault(e)
		}
		r = &created
	}
	if r.Input != input {
		return BlackjackRound{}, Fault{Code: "IDEMPOTENCY_CONFLICT"}
	}
	current, e := s.readBlackjackRound(ctx, subject, user, r.ID, p.InitialWager)
	if e != nil {
		// The original deal is already durable; a failed fresh view cannot turn
		// its accepted wager into a definitive refusal or an absent original.
		return BlackjackRound{}, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	return current, nil
}
func (s *Service) StateBlackjack(ctx context.Context, subject, quote string) (BlackjackRound, error) {
	p, user, e := s.bindBlackjack(ctx, subject, quote, "ROUND")
	if e != nil {
		return BlackjackRound{}, e
	}
	return s.readBlackjackRound(ctx, subject, user, p.RoundID, p.InitialWager)
}
func (s *Service) readBlackjackRound(ctx context.Context, subject string, user int64, id, wager string) (BlackjackRound, error) {
	g, ok := s.games.(blackjackGameService)
	if !ok {
		return BlackjackRound{}, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	r, e := g.Read(ctx, user, id)
	if e != nil {
		return BlackjackRound{}, mapFault(e)
	}
	if r.ID != id || (wager != "" && r.Input.InitialWager != wager) {
		return BlackjackRound{}, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	return s.projectBlackjackRound(subject, user, r)
}
func (s *Service) ActBlackjack(ctx context.Context, subject string, input BlackjackActionRequest) (BlackjackActionResult, error) {
	return s.blackjackActionResult(ctx, subject, input, true)
}
func (s *Service) LookupBlackjackAction(ctx context.Context, subject string, input BlackjackActionRequest) (BlackjackActionResult, error) {
	return s.blackjackActionResult(ctx, subject, input, false)
}
func (s *Service) blackjackActionResult(ctx context.Context, subject string, input BlackjackActionRequest, act bool) (BlackjackActionResult, error) {
	var out BlackjackActionResult
	if !validID(input.RequestID) || !validBlackjackAction(input.ActionType) {
		return out, Fault{Code: "INVALID_REQUEST"}
	}
	p, user, e := s.bindBlackjack(ctx, subject, input.Quote, "ROUND")
	if e != nil {
		return out, e
	}
	if *p.ActiveHandID == "" {
		return out, Fault{Code: "BLACKJACK_ACTION_NOT_ALLOWED"}
	}
	g, ok := s.games.(blackjackGameService)
	if !ok {
		return out, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	id, e := s.blackjackActionID(subject, input.RequestID)
	if e != nil {
		return out, e
	}
	in := games.BlackjackActionInput{ActionID: id, ActionType: input.ActionType, HandID: *p.ActiveHandID, ExpectedVersion: p.RoundVersion}
	original, e := g.FindBlackjackActionMatching(ctx, user, p.RoundID, in)
	if e != nil {
		return out, mapFault(e)
	}
	if original == nil {
		if !act {
			return out, Fault{Code: "NOT_FOUND"}
		}
		exp, _ := blackjackDecimal(p.ExpiresAt)
		if s.now().Unix() >= exp {
			return out, Fault{Code: "QUOTE_EXPIRED"}
		}
		r, e := g.BlackjackAction(ctx, user, p.RoundID, in)
		// Signed, normalized inputs can still exceed the cumulative stake cap.
		if errors.Is(e, games.ErrInvalidInput) {
			return out, Fault{Code: "BLACKJACK_ACTION_NOT_ALLOWED"}
		}
		if e != nil {
			return out, mapFault(e)
		}
		original = &r
	}
	expected, _ := blackjackDecimal(p.RoundVersion)
	prior, _ := blackjackDecimal(p.StakeUnits)
	additional := int64(0)
	if input.ActionType == "DOUBLE" || input.ActionType == "SPLIT" {
		additional, _ = blackjackDecimal(p.ActiveHandStakeUnits)
	}
	if original.ID != p.RoundID || original.Game != "blackjack" || original.Input != (games.CreateInput{Type: "BLACKJACK", InitialWager: p.InitialWager}) || original.Blackjack == nil || original.Blackjack.Version != expected+1 || original.StakeUnits-prior != additional {
		return out, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	// Receipt comes only from the durable original, current controls only from Read.
	current, e := s.readBlackjackRound(ctx, subject, user, p.RoundID, p.InitialWager)
	if e != nil {
		// The original action is already durable. Even a business/read-recovery
		// failure here must retain UNKNOWN lookup recovery rather than claim refusal.
		return out, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	version, _ := blackjackDecimal(current.RoundVersion)
	if version < expected+1 {
		return out, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	return BlackjackActionResult{ActionID: id, RequestID: input.RequestID, ActionType: input.ActionType, HandID: in.HandID, ExpectedRoundVersion: in.ExpectedVersion, AppliedRoundVersion: strconv.FormatInt(expected+1, 10), AdditionalStakeUnits: strconv.FormatInt(additional, 10), Round: current}, nil
}
