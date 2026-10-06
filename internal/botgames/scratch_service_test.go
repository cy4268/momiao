package botgames

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
)

type scratchGames struct {
	*fakeGames
	read                   games.GameRound
	readErr, revealErr     error
	readCalls, revealCalls int
	readID, actionID       string
	wrongReveal            bool
	incompleteReveal       bool
}

func (g *scratchGames) Read(_ context.Context, user int64, id string) (games.GameRound, error) {
	g.user, g.readID = user, id
	g.readCalls++
	return g.read, g.readErr
}
func (g *scratchGames) RevealComplete(_ context.Context, user int64, id, action string) (games.GameRound, error) {
	g.user, g.readID, g.actionID = user, id, action
	g.revealCalls++
	if g.revealErr != nil {
		return games.GameRound{}, g.revealErr
	}
	r := g.created
	if g.read.ID != "" {
		r = g.read
	}
	if g.found != nil {
		r = *g.found
	}
	done := r.SettledAt.Add(time.Second)
	if !g.incompleteReveal {
		r.PresentationCompletedAt = &done
	}
	if g.wrongReveal {
		r.ID = "00000000-0000-7000-8000-000000000003"
	}
	return r, nil
}
func scratchRound(tier string) games.GameRound {
	created := time.Unix(1700000001, 123).UTC()
	settled := created.Add(time.Second)
	m := map[string]int64{"LOSS": 0, "BREAK_EVEN": 1, "T2": 2, "T3": 3, "T5": 5, "T10": 10, "T25": 25, "TOP": 100}[tier]
	symbol := map[string]string{"BREAK_EVEN": "P1", "T2": "P2", "T3": "P3", "T5": "P5", "T10": "P10", "T25": "P25", "TOP": "P100"}[tier]
	symbols := []string{"P1", "P2", "P3", "P5", "P10", "P25", "P100"}
	if tier == "LOSS" {
		symbols = append(symbols, "P1", "P2")
	} else {
		symbols = append(symbols, symbol, symbol)
	}
	outcome := games.Win
	if m == 0 {
		outcome = games.Loss
	}
	if m == 1 {
		outcome = games.BreakEven
	}
	d := games.ScratchResult{Tier: tier, Reward: games.Reward{CostMultiplier: 1, PayoutMultiplier: m, Outcome: outcome}}
	for i, v := range symbols {
		d.Cells[i] = games.ScratchCell{Symbol: v, Matching: v == symbol}
	}
	return games.GameRound{ID: "00000000-0000-7000-8000-000000000002", Game: "scratch", State: "SETTLED", Input: games.CreateInput{Type: "SCRATCH", Wager: "11"}, StakeUnits: 5500000, PayoutUnits: 5500000 * m, NetUnits: 5500000 * (m - 1), Outcome: outcome, BalanceBeforeUnits: 550000000, BalanceAfterUnits: 550000000 + 5500000*(m-1), Ruleset: "scratch-rules-v1", Algorithm: games.ScratchAlgorithm, CreatedAt: created, SettledAt: &settled, Scratch: &d}
}
func newScratchFixture(t *testing.T) (*Service, *scratchGames, *fakeResolver, *time.Time) {
	t.Helper()
	s, g, r, now, _ := newFixture(t)
	c, e := games.ScratchV1(testCommitment)
	if e != nil {
		t.Fatal(e)
	}
	b := c.Binding()
	g.bootstrap.Game = games.CatalogEntry{Slug: "scratch", State: "PLAY", Config: &games.ConfigSummary{Version: b.Version, Hash: hex.EncodeToString(b.Hash[:]), Schema: b.SchemaVersion, Ruleset: b.RulesetVersion, Algorithm: b.AlgorithmVersion, Prizes: c.Prizes()}}
	g.bootstrap.Next = &games.Commitment{ID: testCommitment, ServerSeedHash: strings.Repeat("a", 64), ConfigVersion: b.Version, ConfigHash: hex.EncodeToString(b.Hash[:]), Ruleset: b.RulesetVersion, Algorithm: b.AlgorithmVersion, Resources: json.RawMessage(`{"prize_table_version":"scratch-prize-v1"}`)}
	g.bootstrap.EconomicPolicy = nil
	g.created = scratchRound("T2")
	sg := &scratchGames{fakeGames: g}
	s.games = sg
	return s, sg, r, now
}
func scratchPrepare(t *testing.T, s *Service, wager string) ScratchPrepared {
	t.Helper()
	p, e := s.PrepareScratch(context.Background(), testSubject, ScratchPrepareInput{testRequest, wager})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func resumeScratch(t *testing.T, s *Service, g *scratchGames) ScratchPrepared {
	t.Helper()
	g.read = scratchRound("T2")
	g.bootstrap.ScratchBlocker = &g.read
	g.bootstrap.EntryAction = "RESUME"
	g.bootstrap.Next = nil
	return scratchPrepare(t, s, "99")
}
func exactFields(t *testing.T, v any, names string) {
	t.Helper()
	raw, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	var fields map[string]json.RawMessage
	if e = json.Unmarshal(raw, &fields); e != nil {
		t.Fatal(e)
	}
	for _, name := range strings.Fields(names) {
		if _, ok := fields[name]; !ok {
			t.Fatalf("missing %s: %s", name, raw)
		}
		delete(fields, name)
	}
	if len(fields) != 0 {
		t.Fatalf("unexpected keys %v", fields)
	}
}
func TestScratchPurchasePrepareAndOriginalLookup(t *testing.T) {
	s, g, r, _ := newScratchFixture(t)
	p := scratchPrepare(t, s, "11")
	if p.Action != "PURCHASE" || p.Wager != "11" || p.StakeUnits != "5500000" || p.AdditionalStakeUnits != "5500000" || p.AvailableUnits != "50000000" || p.MinimumWagerUnits != "5000000" || p.MaximumWagerUnits != "50000000" || p.ExpiresAt != "1700000120" || len(p.Prizes) != 8 {
		t.Fatal(p)
	}
	if g.createCalls != 0 || g.revealCalls != 0 || g.slug != "scratch" || r.calls != 1 {
		t.Fatal("prepare mutated or skipped identity")
	}
	exactFields(t, p, "action quote wager stake_units additional_stake_units available_units minimum_wager_units maximum_wager_units prizes ruleset rules_text server_seed_hash commitment_id expires_at")
	_, e := s.LookupScratch(context.Background(), testSubject, p.Quote)
	requireFault(t, e, "NOT_FOUND")
	if g.createCalls != 0 || g.revealCalls != 0 {
		t.Fatal("lookup bought or revealed another ticket")
	}
	result, e := s.PlayScratch(context.Background(), testSubject, p.Quote)
	if e != nil {
		t.Fatal(e)
	}
	if result.Action != "PURCHASE" || result.Wager != "11" || result.PrizeTier != "T2" || result.Multiplier != 2 || result.ActualNetUnits != "5500000" || result.PresentationCompletedAt == "" {
		t.Fatal(result)
	}
	if g.input != (games.CreateInput{Type: "SCRATCH", Wager: "11"}) || g.key != "discord-scratch-v1:"+testRequest || g.commitment != testCommitment || g.createCalls != 1 || g.revealCalls != 1 || g.user != 424242 {
		t.Fatal("wrong create/complete authority")
	}
	exactFields(t, result, "action round_id created_at settled_at presentation_completed_at status wager cells prize_tier multiplier result stake_units gross_payout_units withheld_units credited_payout_units actual_net_units ruleset")
	for i, cell := range result.Cells {
		if cell.Index != i+1 {
			t.Fatal("order changed")
		}
		exactFields(t, cell, "index symbol matching")
	}
}
func TestScratchResumeNoChargeAndNoCurrentTerms(t *testing.T) {
	s, g, _, _ := newScratchFixture(t)
	g.bootstrap.Game.State = "MAINTENANCE"
	p := resumeScratch(t, s, g)
	if p.Action != "RESUME" || p.Wager != "11" || p.StakeUnits != "5500000" || p.AdditionalStakeUnits != "0" || p.RoundID != g.read.ID || !strings.Contains(p.RulesText, "已有未揭晓券，本次只揭晓原券，不再次扣款；本次输入的筹码不会用于购买新券") {
		t.Fatal(p)
	}
	exactFields(t, p, "action quote wager stake_units additional_stake_units round_id created_at ruleset rules_text expires_at")
	result, e := s.LookupScratch(context.Background(), testSubject, p.Quote)
	if e != nil {
		t.Fatal(e)
	}
	if result.Action != "RESUME" || result.RoundID != p.RoundID || g.createCalls != 0 || g.findCalls != 0 || g.readID != p.RoundID || g.revealCalls != 1 {
		t.Fatal("resume switched or purchased ticket")
	}
	_, e = s.PrepareScratch(context.Background(), testSubject, ScratchPrepareInput{testRequest, "9"})
	requireFault(t, e, "INVALID_REQUEST")
}
func TestScratchExpiryAndPartialCompletionRecovery(t *testing.T) {
	for _, action := range []string{"PURCHASE", "RESUME"} {
		for _, completed := range []bool{false, true} {
			for _, lookup := range []bool{false, true} {
				t.Run(action+"/completed="+map[bool]string{true: "true", false: "false"}[completed]+"/lookup="+map[bool]string{true: "true", false: "false"}[lookup], func(t *testing.T) {
					s, g, _, now := newScratchFixture(t)
					var p ScratchPrepared
					if action == "RESUME" {
						p = resumeScratch(t, s, g)
					} else {
						p = scratchPrepare(t, s, "11")
						g.found = &g.created
					}
					if completed {
						done := g.created.SettledAt.Add(time.Second)
						g.created.PresentationCompletedAt = &done
						g.read.PresentationCompletedAt = &done
					}
					*now = now.Add(120 * time.Second)
					var e error
					var result ScratchResult
					if lookup {
						result, e = s.LookupScratch(context.Background(), testSubject, p.Quote)
					} else {
						result, e = s.PlayScratch(context.Background(), testSubject, p.Quote)
					}
					if action == "RESUME" && !completed {
						requireFault(t, e, "QUOTE_EXPIRED")
						if g.revealCalls != 0 {
							t.Fatal("expired unconfirmed reveal")
						}
					} else {
						if e != nil || result.PresentationCompletedAt == "" {
							t.Fatal(result, e)
						}
						if completed && g.revealCalls != 0 {
							t.Fatal("already completed inserted action")
						}
					}
					if g.createCalls != 0 {
						t.Fatal("recovery charged")
					}
				})
			}
		}
	}
	s, g, r, now := newScratchFixture(t)
	p := scratchPrepare(t, s, "11")
	g.revealErr = errors.New("presentation failure")
	_, e := s.PlayScratch(context.Background(), testSubject, p.Quote)
	requireFault(t, e, "UPSTREAM_UNAVAILABLE")
	if g.createCalls != 1 {
		t.Fatal("settlement absent")
	}
	g.found = &g.created
	g.revealErr = nil
	*now = now.Add(121 * time.Second)
	restarted, e := NewService(g, r, testKey, func() time.Time { return *now })
	if e != nil {
		t.Fatal(e)
	}
	result, e := restarted.LookupScratch(context.Background(), testSubject, p.Quote)
	if e != nil || result.RoundID != g.created.ID || g.createCalls != 1 || g.revealCalls != 2 {
		t.Fatal(result, e)
	}
}
func TestScratchNeverFallsBackAndRechecksBinding(t *testing.T) {
	for _, resume := range []bool{false, true} {
		t.Run(map[bool]string{false: "purchase", true: "resume"}[resume], func(t *testing.T) {
			s, g, r, _ := newScratchFixture(t)
			p := scratchPrepare(t, s, "11")
			if resume {
				p = resumeScratch(t, s, g)
			}
			r.user++
			_, e := s.PlayScratch(context.Background(), testSubject, p.Quote)
			requireFault(t, e, "BINDING_CHANGED")
			_, e = s.LookupScratch(context.Background(), testSubject, p.Quote)
			requireFault(t, e, "BINDING_CHANGED")
			r.user--
			r.err = Fault{Code: "ACCOUNT_RESTRICTED"}
			_, e = s.LookupScratch(context.Background(), testSubject, p.Quote)
			requireFault(t, e, "ACCOUNT_RESTRICTED")
			r.err = nil
			_, e = s.PlayScratch(context.Background(), "1", p.Quote)
			requireFault(t, e, "UNAUTHORIZED")
			if resume {
				g.read.Input.Wager = "10"
			} else {
				g.created.Input.Wager = "10"
				g.found = &g.created
			}
			_, e = s.LookupScratch(context.Background(), testSubject, p.Quote)
			requireFault(t, e, "IDEMPOTENCY_CONFLICT")
			if g.createCalls != 0 || g.revealCalls != 0 {
				t.Fatal("failed authority caused mutation")
			}
		})
	}
	s, g, _, _ := newScratchFixture(t)
	p := scratchPrepare(t, s, "11")
	g.createErr = games.ErrScratchIncomplete
	_, e := s.PlayScratch(context.Background(), testSubject, p.Quote)
	requireFault(t, e, "SCRATCH_PREVIOUS_REVEAL_INCOMPLETE")
	if g.readCalls != 0 || g.revealCalls != 0 {
		t.Fatal("stale purchase silently revealed website ticket")
	}
	g.createErr = nil
	g.wrongReveal = true
	_, e = s.PlayScratch(context.Background(), testSubject, p.Quote)
	requireFault(t, e, "UPSTREAM_UNAVAILABLE")
	g.wrongReveal = false
	g.incompleteReveal = true
	_, e = s.PlayScratch(context.Background(), testSubject, p.Quote)
	requireFault(t, e, "UPSTREAM_UNAVAILABLE")
}
func TestScratchInvalidPrepareAndOverflow(t *testing.T) {
	for _, w := range []string{"", "9", "01", "+10", " 10", "10.0", "0", "18446744073709551615", "184467440738"} {
		t.Run(w, func(t *testing.T) {
			s, g, _, _ := newScratchFixture(t)
			_, e := s.PrepareScratch(context.Background(), testSubject, ScratchPrepareInput{testRequest, w})
			requireFault(t, e, "INVALID_REQUEST")
			if g.bootstrapCalls != 0 {
				t.Fatal("invalid wager reached game")
			}
		})
	}
	for name, change := range map[string]func(*games.Bootstrap){
		"maintenance":     func(b *games.Bootstrap) { b.Game.State = "MAINTENANCE" },
		"not_play":        func(b *games.Bootstrap) { b.EntryAction = "DISABLED" },
		"wrong_game":      func(b *games.Bootstrap) { b.Game.Slug = "dice" },
		"missing_next":    func(b *games.Bootstrap) { b.Next = nil },
		"bad_hash":        func(b *games.Bootstrap) { b.Next.ServerSeedHash = "abc" },
		"bad_config_hash": func(b *games.Bootstrap) { b.Game.Config.Hash = strings.Repeat("b", 64) },
		"wrong_algorithm": func(b *games.Bootstrap) { b.Next.Algorithm = "scratch-map-v2" },
		"wrong_schema":    func(b *games.Bootstrap) { b.Game.Config.Schema = "scratch-config-v2" },
		"bad_prize":       func(b *games.Bootstrap) { b.Game.Config.Prizes[0].Multiplier = 3 },
		"bad_weights":     func(b *games.Bootstrap) { b.Game.Config.Prizes[0].Weight++ },
		"duplicate_resource": func(b *games.Bootstrap) {
			b.Next.Resources = json.RawMessage(`{"prize_table_version":"scratch-prize-v1","prize_table_version":"scratch-prize-v1"}`)
		},
		"unknown_resource": func(b *games.Bootstrap) {
			b.Next.Resources = json.RawMessage(`{"prize_table_version":"scratch-prize-v1","pool_id":"x"}`)
		},
		"wrong_table_identity": func(b *games.Bootstrap) {
			b.Next.Resources = json.RawMessage(`{"prize_table_version":"different-prize-v1"}`)
		},
		"nil_resource": func(b *games.Bootstrap) { b.Next.Resources = json.RawMessage(`{"prize_table_version":null}`) },
		"bad_policy":   func(b *games.Bootstrap) { b.WagerPolicy.Step = 1 },
		"low_balance":  func(b *games.Bootstrap) { b.AvailableUnits = 5000000 },
		"overflow":     func(b *games.Bootstrap) { b.AvailableUnits = math.MaxInt64 - 544499999 },
		"bad_economy":  func(b *games.Bootstrap) { b.EconomicPolicy = &platform.EconomicPolicy{Version: "bad"} },
	} {
		t.Run(name, func(t *testing.T) {
			s, g, _, _ := newScratchFixture(t)
			change(&g.bootstrap)
			_, e := s.PrepareScratch(context.Background(), testSubject, ScratchPrepareInput{testRequest, "11"})
			if e == nil {
				t.Fatal("invalid bootstrap accepted")
			}
			if g.createCalls != 0 || g.revealCalls != 0 {
				t.Fatal("prepare mutation")
			}
		})
	}
	s, g, _, _ := newScratchFixture(t)
	g.bootstrap.AvailableUnits = math.MaxInt64 - 544500000
	p := scratchPrepare(t, s, "11")
	if p.MaximumWagerUnits != "5500000" {
		t.Fatal(p.MaximumWagerUnits)
	}
	prizes := g.bootstrap.Game.Config.Prizes
	prizes[0].Weight--
	prizes[1].Weight++
	c, e := games.NewScratchConfig(testCommitment, "scratch-adjusted-v1", prizes)
	if e != nil {
		t.Fatal(e)
	}
	b := c.Binding()
	g.bootstrap.Game.Config.Prizes = c.Prizes()
	g.bootstrap.Game.Config.Hash = hex.EncodeToString(b.Hash[:])
	g.bootstrap.Next.ConfigHash = g.bootstrap.Game.Config.Hash
	g.bootstrap.Next.Resources = json.RawMessage(`{"prize_table_version":"scratch-adjusted-v1"}`)
	p = scratchPrepare(t, s, "11")
	if !reflect.DeepEqual(p.Prizes, prizes) {
		t.Fatal("current odds ignored")
	}
}
