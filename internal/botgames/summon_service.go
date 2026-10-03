package botgames

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"strconv"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
)

type SummonPrepareInput struct {
	RequestID string `json:"request_id"`
	BaseWager string `json:"base_wager"`
	Mode      string `json:"mode"`
}

func (s *Service) PrepareSummon(ctx context.Context, subject string, input SummonPrepareInput) (SummonPrepared, error) {
	base, count, valid := summonWagerUnits(input.BaseWager, input.Mode)
	if !validID(subject) || !validID(input.RequestID) || !valid {
		return SummonPrepared{}, Fault{Code: "INVALID_REQUEST"}
	}
	user, err := s.resolve(ctx, subject)
	if err != nil {
		return SummonPrepared{}, err
	}
	b, err := s.games.Bootstrap(ctx, user, "summon")
	if err != nil {
		return SummonPrepared{}, mapFault(err)
	}
	if b.EntryAction == "MAINTENANCE" || b.Game.State == "MAINTENANCE" {
		return SummonPrepared{}, Fault{Code: "MAINTENANCE"}
	}
	if b.EntryAction != "PLAY" || b.Game.State != "PLAY" || b.Game.Slug != "summon" {
		return SummonPrepared{}, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	if b.Next == nil || !validUUID(b.Next.ID) || !validHash(b.Next.ServerSeedHash) {
		return SummonPrepared{}, Fault{Code: "COMMITMENT_INVALID"}
	}
	prizes, err := summonPrizes(b)
	if err != nil {
		return SummonPrepared{}, err
	}
	minimum, maximum, err := summonWagerLimits(b, count)
	if err != nil {
		return SummonPrepared{}, err
	}
	if base < minimum {
		return SummonPrepared{}, Fault{Code: "INVALID_REQUEST"}
	}
	if base*int64(count) > b.AvailableUnits {
		return SummonPrepared{}, Fault{Code: "INSUFFICIENT_CHIPS"}
	}
	if base > maximum {
		return SummonPrepared{}, Fault{Code: "INVALID_REQUEST"}
	}
	iat := s.now().Unix()
	if iat <= 0 || iat > math.MaxInt64-120 {
		return SummonPrepared{}, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	p := summonQuotePayload{Version: 1, Game: "summon", RequestID: input.RequestID, Subject: subject, Binding: s.binding(subject, user), BaseWager: input.BaseWager, Mode: input.Mode, CommitmentID: b.Next.ID, IssuedAt: iat, ExpiresAt: iat + 120}
	quote, err := s.signSummonQuote(p)
	if err != nil {
		return SummonPrepared{}, err
	}
	return SummonPrepared{Quote: quote, BaseWager: input.BaseWager, Mode: input.Mode, DrawCount: count, StakeUnits: strconv.FormatInt(base*int64(count), 10), AvailableUnits: strconv.FormatInt(b.AvailableUnits, 10), MinimumBaseWagerUnits: strconv.FormatInt(minimum, 10), MaximumBaseWagerUnits: strconv.FormatInt(maximum, 10), Prizes: prizes, Ruleset: b.Next.Ruleset, RulesText: "每抽独立使用当前奖池，没有保底、累计进度或角色入库。筹码为每抽下注；单抽抽取 1 次，十连一次扣除每抽下注 × 10。总派彩为各抽派彩之和，包含返还金额；胜负按总派彩与整轮总下注比较，经济上限可能扣留利润，实际净变化以结算为准。", ServerSeedHash: b.Next.ServerSeedHash, CommitmentID: b.Next.ID, ExpiresAt: strconv.FormatInt(p.ExpiresAt, 10)}, nil
}

// The commitment resources carry the prize-table identity omitted by the public
// config summary. Rebuild only its immutable config, never a draw or RNG stream.
func summonPrizes(b games.Bootstrap) ([]games.Prize, error) {
	invalid := Fault{Code: "UPSTREAM_UNAVAILABLE"}
	c, n := b.Game.Config, b.Next
	if c == nil || n == nil || c.Schema != "summon-config-v1" || c.Ruleset != "summon-rules-v1" || c.Algorithm != games.SummonAlgorithm || n.Ruleset != c.Ruleset || n.Algorithm != c.Algorithm || !validUUID(c.Version) || n.ConfigVersion != c.Version || !validHash(c.Hash) || n.ConfigHash != c.Hash {
		return nil, invalid
	}
	resources := map[string]string{}
	d := json.NewDecoder(bytes.NewReader(n.Resources))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, invalid
	}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return nil, invalid
		}
		name, ok := token.(string)
		if !ok || (name != "pool_id" && name != "prize_table_version") {
			return nil, invalid
		}
		if _, ok = resources[name]; ok {
			return nil, invalid
		}
		var value string
		if d.Decode(&value) != nil || value == "" {
			return nil, invalid
		}
		resources[name] = value
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') || len(resources) != 2 || d.Decode(new(json.RawMessage)) != io.EOF || resources["pool_id"] != games.SummonPool {
		return nil, invalid
	}
	config, err := games.NewSummonConfig(c.Version, resources["prize_table_version"], c.Prizes)
	if err != nil {
		return nil, invalid
	}
	binding := config.Binding()
	if hex.EncodeToString(binding.Hash[:]) != c.Hash {
		return nil, invalid
	}
	return config.Prizes(), nil
}

func summonWagerLimits(b games.Bootstrap, count int) (int64, int64, error) {
	p := b.WagerPolicy
	if (count != 1 && count != 10) || p.Minimum < 10*games.UnitsPerChip || p.Minimum%games.UnitsPerChip != 0 || p.Step != games.UnitsPerChip || p.MaximumMode != "NONE" || b.AvailableUnits < 0 {
		return 0, 0, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	n := int64(count)
	// The website tests both the maximum gross and balance - stake + maximum
	// gross before RNG. The economic single-player cap applies to the WHOLE round.
	maximum := min(b.AvailableUnits/n, math.MaxInt64/(100*n), (math.MaxInt64-b.AvailableUnits)/(99*n))
	if e := b.EconomicPolicy; e != nil {
		if e.Version != platform.EconomicPolicyVersion || !validHash(e.Hash) || e.SinglePlayerMaxUnits <= 0 || e.AssetCapUnits <= 0 || e.CapMode != "CLIP_PROFIT" || b.Next.EconomicVersion != e.Version {
			return 0, 0, Fault{Code: "UPSTREAM_UNAVAILABLE"}
		}
		maximum = min(maximum, e.SinglePlayerMaxUnits/n)
	} else if b.Next.EconomicVersion != "" {
		return 0, 0, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	maximum -= maximum % games.UnitsPerChip
	return p.Minimum, maximum, nil
}

func summonWagerUnits(value, mode string) (int64, int, bool) {
	count := 0
	switch mode {
	case "SINGLE":
		count = 1
	case "TENFOLD":
		count = 10
	default:
		return 0, 0, false
	}
	if !canonicalPositive(value) {
		return 0, 0, false
	}
	chips, err := strconv.ParseInt(value, 10, 64)
	if err != nil || chips < 10 || chips > math.MaxInt64/(100*int64(count)*games.UnitsPerChip) {
		return 0, 0, false
	}
	return chips * games.UnitsPerChip, count, true
}

func (s *Service) PlaySummon(ctx context.Context, subject, quote string) (SummonResult, error) {
	return s.summonResult(ctx, subject, quote, true)
}
func (s *Service) LookupSummon(ctx context.Context, subject, quote string) (SummonResult, error) {
	return s.summonResult(ctx, subject, quote, false)
}
func (s *Service) summonResult(ctx context.Context, subject, quote string, create bool) (SummonResult, error) {
	p, err := s.verifySummonQuote(quote)
	if err != nil {
		return SummonResult{}, err
	}
	if !validID(subject) {
		return SummonResult{}, Fault{Code: "INVALID_REQUEST"}
	}
	if p.Subject != subject {
		return SummonResult{}, Fault{Code: "UNAUTHORIZED"}
	}
	user, err := s.resolve(ctx, subject)
	if err != nil {
		return SummonResult{}, err
	}
	if !hmac.Equal([]byte(p.Binding), []byte(s.binding(subject, user))) {
		return SummonResult{}, Fault{Code: "BINDING_CHANGED"}
	}
	key := "discord-summon-v1:" + p.RequestID
	input := games.CreateInput{Type: "SUMMON", BaseWager: p.BaseWager, Mode: p.Mode}
	round, err := s.games.FindByKey(ctx, user, "summon", key)
	if err != nil {
		return SummonResult{}, mapFault(err)
	}
	if round != nil {
		if round.Input != input {
			return SummonResult{}, Fault{Code: "IDEMPOTENCY_CONFLICT"}
		}
		return projectSummonRound(*round)
	}
	if !create {
		return SummonResult{}, Fault{Code: "NOT_FOUND"}
	}
	if s.now().Unix() >= p.ExpiresAt {
		return SummonResult{}, Fault{Code: "QUOTE_EXPIRED"}
	}
	// Existing game transactions own RNG, accounting and concurrent idempotency.
	created, err := s.games.Create(ctx, user, "summon", key, p.CommitmentID, input)
	if err != nil {
		return SummonResult{}, mapFault(err)
	}
	if created.Input != input {
		return SummonResult{}, Fault{Code: "IDEMPOTENCY_CONFLICT"}
	}
	return projectSummonRound(created)
}
