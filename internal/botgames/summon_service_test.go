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

func newSummonFixture(t *testing.T) (*Service, *fakeGames, *fakeResolver, *time.Time, *[]string) {
	t.Helper()
	s, g, r, now, order := newFixture(t)
	c, e := games.NewSummonConfig("00000000-0000-4000-8000-000000000003", "custom-summon-v1", []games.Prize{{Multiplier: 100, Tier: "T5", Weight: 50}, {Multiplier: 0, Tier: "T0", Weight: 40000}, {Multiplier: 2, Tier: "T2", Weight: 39000}, {Multiplier: 1, Tier: "T1", Weight: 20000}, {Multiplier: 5, Tier: "T3", Weight: 850}, {Multiplier: 20, Tier: "T4", Weight: 100}})
	if e != nil {
		t.Fatal(e)
	}
	b := c.Binding()
	hash := hex.EncodeToString(b.Hash[:])
	g.bootstrap.Game = games.CatalogEntry{Slug: "summon", State: "PLAY", Config: &games.ConfigSummary{Version: b.Version, Hash: hash, Algorithm: b.AlgorithmVersion, Schema: b.SchemaVersion, Ruleset: b.RulesetVersion, Prizes: c.Prizes()}}
	g.bootstrap.AvailableUnits = 550000000
	g.bootstrap.EconomicPolicy = nil
	g.bootstrap.Next = &games.Commitment{ID: testCommitment, ServerSeedHash: strings.Repeat("a", 64), ConfigVersion: b.Version, ConfigHash: hash, Ruleset: b.RulesetVersion, Algorithm: b.AlgorithmVersion, Resources: json.RawMessage(`{"pool_id":"SUMMON_MAIN_V1","prize_table_version":"custom-summon-v1"}`)}
	g.created = fixtureSummonRound("SINGLE")
	return s, g, r, now, order
}
func prepareSummonQuote(t *testing.T, s *Service, mode string) string {
	t.Helper()
	p, e := s.PrepareSummon(context.Background(), testSubject, SummonPrepareInput{RequestID: testRequest, BaseWager: "11", Mode: mode})
	if e != nil {
		t.Fatal(e)
	}
	return p.Quote
}

// Catches per-draw/total confusion, hardcoded old probabilities and extra private fields.
func TestSummonPrepareActualPoolAndModeAccounting(t *testing.T) {
	for _, tc := range []struct {
		mode, stake, maximum string
		count                int
	}{{"SINGLE", "5500000", "550000000", 1}, {"TENFOLD", "55000000", "55000000", 10}} {
		t.Run(tc.mode, func(t *testing.T) {
			s, g, _, _, _ := newSummonFixture(t)
			p, e := s.PrepareSummon(context.Background(), testSubject, SummonPrepareInput{testRequest, "11", tc.mode})
			if e != nil {
				t.Fatal(e)
			}
			if p.BaseWager != "11" || p.Mode != tc.mode || p.DrawCount != tc.count || p.StakeUnits != tc.stake || p.MinimumBaseWagerUnits != "5000000" || p.MaximumBaseWagerUnits != tc.maximum || p.AvailableUnits != "550000000" || p.Ruleset != "summon-rules-v1" || p.ExpiresAt != "1700000120" || p.RulesText == "" || len([]rune(p.RulesText)) > 1024 {
				t.Fatal(p)
			}
			if !reflect.DeepEqual(p.Prizes, g.bootstrap.Game.Config.Prizes) || p.Prizes[0].Weight != 50 || p.Prizes[2].Weight != 39000 {
				t.Fatal("actual configured pool was replaced", p.Prizes)
			}
			if g.slug != "summon" || g.createCalls != 0 || g.findCalls != 0 {
				t.Fatal("prepare crossed settlement boundary")
			}
			raw, _ := json.Marshal(p)
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(raw, &fields)
			names := []string{"quote", "base_wager", "mode", "draw_count", "stake_units", "available_units", "minimum_base_wager_units", "maximum_base_wager_units", "prizes", "ruleset", "rules_text", "server_seed_hash", "commitment_id", "expires_at"}
			if len(fields) != len(names) {
				t.Fatal(string(raw))
			}
			for _, n := range names {
				if _, ok := fields[n]; !ok {
					t.Fatal("missing", n)
				}
			}
		})
	}
}

func TestSummonPrepareInputRejectedBeforeAuthority(t *testing.T) {
	for _, tc := range []struct{ wager, mode string }{{"", "SINGLE"}, {"9", "SINGLE"}, {"0", "SINGLE"}, {"-1", "SINGLE"}, {"+11", "SINGLE"}, {"011", "SINGLE"}, {"11.0", "SINGLE"}, {" 11", "SINGLE"}, {"１１", "SINGLE"}, {"1e2", "SINGLE"}, {"9223372036854775808", "SINGLE"}, {"184467440738", "SINGLE"}, {"18446744074", "TENFOLD"}, {"11", ""}, {"11", "single"}, {"11", "TENFOLD_WITH_GUARANTEE"}} {
		t.Run(tc.wager+"_"+tc.mode, func(t *testing.T) {
			s, g, r, _, _ := newSummonFixture(t)
			_, e := s.PrepareSummon(context.Background(), testSubject, SummonPrepareInput{testRequest, tc.wager, tc.mode})
			requireFault(t, e, "INVALID_REQUEST")
			if r.calls != 0 || g.bootstrapCalls != 0 {
				t.Fatal("invalid input reached authority")
			}
		})
	}
	for _, id := range []string{"", "0", "01", "18446744073709551616"} {
		s, g, r, _, _ := newSummonFixture(t)
		_, e := s.PrepareSummon(context.Background(), id, SummonPrepareInput{testRequest, "11", "SINGLE"})
		requireFault(t, e, "INVALID_REQUEST")
		_, e = s.PrepareSummon(context.Background(), testSubject, SummonPrepareInput{id, "11", "SINGLE"})
		requireFault(t, e, "INVALID_REQUEST")
		if r.calls != 0 || g.bootstrapCalls != 0 {
			t.Fatal("invalid ID reached authority")
		}
	}
}

func TestSummonPrepareLimitsAndUpstreamValidation(t *testing.T) {
	for _, tc := range []struct {
		name, mode, wager, want, maximum string
		change                           func(*games.Bootstrap)
	}{
		{"single_headroom", "SINGLE", "11", "", "5500000", func(b *games.Bootstrap) { b.AvailableUnits = math.MaxInt64 - 544500000 }},
		{"single_headroom_above", "SINGLE", "12", "INVALID_REQUEST", "", func(b *games.Bootstrap) { b.AvailableUnits = math.MaxInt64 - 544500000 }},
		{"tenfold_headroom", "TENFOLD", "11", "", "5500000", func(b *games.Bootstrap) { b.AvailableUnits = math.MaxInt64 - 5445000000 }},
		{"tenfold_headroom_above", "TENFOLD", "12", "INVALID_REQUEST", "", func(b *games.Bootstrap) { b.AvailableUnits = math.MaxInt64 - 5445000000 }},
		{"gross_guard_single", "SINGLE", "184467440737", "INVALID_REQUEST", "", func(b *games.Bootstrap) { b.AvailableUnits = math.MaxInt64 / 2 }},
		{"gross_guard_tenfold", "TENFOLD", "18446744073", "INVALID_REQUEST", "", func(b *games.Bootstrap) { b.AvailableUnits = math.MaxInt64 / 2 }},
		{"no_headroom", "SINGLE", "10", "INVALID_REQUEST", "", func(b *games.Bootstrap) { b.AvailableUnits = math.MaxInt64 }},
		{"rounded_balance", "TENFOLD", "11", "", "5500000", func(b *games.Bootstrap) { b.AvailableUnits = 59999999 }},
		{"balance", "TENFOLD", "11", "INSUFFICIENT_CHIPS", "", func(b *games.Bootstrap) { b.AvailableUnits = 54999999 }},
		{"higher_minimum", "SINGLE", "11", "INVALID_REQUEST", "", func(b *games.Bootstrap) { b.WagerPolicy.Minimum = 6000000 }},
		{"cap_per_round", "TENFOLD", "11", "", "5500000", func(b *games.Bootstrap) {
			b.EconomicPolicy = &platform.EconomicPolicy{Version: platform.EconomicPolicyVersion, Hash: strings.Repeat("a", 64), SinglePlayerMaxUnits: 59999999, AssetCapUnits: platform.AssetCapUnits, CapMode: "CLIP_PROFIT"}
			b.Next.EconomicVersion = platform.EconomicPolicyVersion
		}},
		{"cap_reject", "TENFOLD", "12", "INVALID_REQUEST", "", func(b *games.Bootstrap) {
			b.EconomicPolicy = &platform.EconomicPolicy{Version: platform.EconomicPolicyVersion, Hash: strings.Repeat("a", 64), SinglePlayerMaxUnits: 59999999, AssetCapUnits: platform.AssetCapUnits, CapMode: "CLIP_PROFIT"}
			b.Next.EconomicVersion = platform.EconomicPolicyVersion
		}},
		{"maintenance", "SINGLE", "11", "MAINTENANCE", "", func(b *games.Bootstrap) { b.Game.State = "MAINTENANCE" }},
		{"entry_maintenance", "SINGLE", "11", "MAINTENANCE", "", func(b *games.Bootstrap) { b.EntryAction = "MAINTENANCE" }},
		{"read_only", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.EntryAction = "READ_ONLY" }},
		{"wrong_game", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.Game.Slug = "dice" }},
		{"no_commitment", "SINGLE", "11", "COMMITMENT_INVALID", "", func(b *games.Bootstrap) { b.Next = nil }},
		{"bad_seed_hash", "SINGLE", "11", "COMMITMENT_INVALID", "", func(b *games.Bootstrap) { b.Next.ServerSeedHash = strings.Repeat("A", 64) }},
		{"bad_commitment", "SINGLE", "11", "COMMITMENT_INVALID", "", func(b *games.Bootstrap) { b.Next.ID = "bad" }},
		{"no_config", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.Game.Config = nil }},
		{"schema", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.Game.Config.Schema = "summon-config-v2" }},
		{"algorithm", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.Game.Config.Algorithm = "dice-map-v1" }},
		{"next_algorithm", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.Next.Algorithm = "dice-map-v1" }},
		{"ruleset", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.Game.Config.Ruleset = "summon-rules-v2" }},
		{"next_ruleset", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.Next.Ruleset = "summon-rules-v2" }},
		{"config_identity", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.Next.ConfigVersion = testCommitment }},
		{"config_hash", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.Next.ConfigHash = strings.Repeat("0", 64) }},
		{"config_hash_format", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.Next.ConfigHash = "x"; b.Game.Config.Hash = "x" }},
		{"config_hash_content", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) {
			b.Next.ConfigHash = strings.Repeat("0", 64)
			b.Game.Config.Hash = b.Next.ConfigHash
		}},
		{"prize_duplicate", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.Game.Config.Prizes[1] = b.Game.Config.Prizes[0] }},
		{"prize_missing", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.Game.Config.Prizes = b.Game.Config.Prizes[:5] }},
		{"prize_multiplier", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.Game.Config.Prizes[0].Multiplier = 101 }},
		{"prize_sum", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.Game.Config.Prizes[0].Weight++ }},
		{"prize_negative", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.Game.Config.Prizes[0].Weight = -1 }},
		{"resources", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) {
			b.Next.Resources = json.RawMessage(`{"pool_id":"WRONG","prize_table_version":"custom-summon-v1"}`)
		}},
		{"minimum", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.WagerPolicy.Minimum = 4999999 }},
		{"step", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.WagerPolicy.Step = 1 }},
		{"maximum_mode", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.WagerPolicy.MaximumMode = "BALANCE" }},
		{"negative_balance", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.AvailableUnits = -1 }},
		{"unknown_economy", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.EconomicPolicy = &platform.EconomicPolicy{Version: "unknown"} }},
		{"missing_economy", "SINGLE", "11", "UPSTREAM_UNAVAILABLE", "", func(b *games.Bootstrap) { b.Next.EconomicVersion = platform.EconomicPolicyVersion }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, g, _, _, _ := newSummonFixture(t)
			tc.change(&g.bootstrap)
			p, e := s.PrepareSummon(context.Background(), testSubject, SummonPrepareInput{testRequest, tc.wager, tc.mode})
			if tc.want != "" {
				requireFault(t, e, tc.want)
			} else if e != nil || p.MaximumBaseWagerUnits != tc.maximum {
				t.Fatal(p, e)
			}
			if g.createCalls != 0 || g.findCalls != 0 {
				t.Fatal("prepare mutated round")
			}
		})
	}
}

func TestSummonPlayDelegatesOneTypedRound(t *testing.T) {
	for _, mode := range []string{"SINGLE", "TENFOLD"} {
		t.Run(mode, func(t *testing.T) {
			s, g, _, _, order := newSummonFixture(t)
			g.created = fixtureSummonRound(mode)
			q := prepareSummonQuote(t, s, mode)
			*order = nil
			got, e := s.PlaySummon(context.Background(), testSubject, q)
			if e != nil {
				t.Fatal(e)
			}
			if got.Mode != mode || got.BaseWager != "11" || got.Result != "WIN" || len(got.Draws) == 0 {
				t.Fatal(got)
			}
			if !reflect.DeepEqual(*order, []string{"resolve", "find", "create"}) || g.key != "discord-summon-v1:970000000000000001" || g.slug != "summon" || g.commitment != testCommitment || g.input != (games.CreateInput{Type: "SUMMON", BaseWager: "11", Mode: mode}) {
				t.Fatal("wrong authority arguments", *order, g.input, g.key)
			}
		})
	}
}

func TestSummonLookupExpiryReplayBindingAndConflicts(t *testing.T) {
	ctx := context.Background()
	for _, op := range []string{"play", "lookup"} {
		t.Run(op, func(t *testing.T) {
			s, g, r, now, _ := newSummonFixture(t)
			q := prepareSummonQuote(t, s, "SINGLE")
			call := s.PlaySummon
			if op == "lookup" {
				call = s.LookupSummon
			}
			_, e := call(ctx, "970000000000000003", q)
			requireFault(t, e, "UNAUTHORIZED")
			r.user++
			_, e = call(ctx, testSubject, q)
			requireFault(t, e, "BINDING_CHANGED")
			r.user--
			*now = now.Add(120 * time.Second)
			_, e = call(ctx, testSubject, q)
			want := "QUOTE_EXPIRED"
			if op == "lookup" {
				want = "NOT_FOUND"
			}
			requireFault(t, e, want)
			if g.createCalls != 0 {
				t.Fatal("expiry/lookup created")
			}
			saved := fixtureSummonRound("SINGLE")
			g.found = &saved
			got, e := call(ctx, testSubject, q)
			if e != nil || got.RoundID != saved.ID {
				t.Fatal(got, e)
			}
			for _, change := range []func(*games.GameRound){func(r *games.GameRound) { r.Input.BaseWager = "10" }, func(r *games.GameRound) { r.Input.Mode = "TENFOLD" }, func(r *games.GameRound) { r.Input.Type = "DICE" }} {
				bad := saved
				change(&bad)
				g.found = &bad
				_, e = call(ctx, testSubject, q)
				requireFault(t, e, "IDEMPOTENCY_CONFLICT")
			}
			if g.createCalls != 0 {
				t.Fatal("saved replay created")
			}
		})
	}
}

func TestSummonAuthorityErrorsRemainBounded(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{{games.ErrCommitmentInvalid, "COMMITMENT_INVALID"}, {platform.ErrBalanceOverflow, "INVALID_REQUEST"}, {platform.ErrInsufficientBalance, "INSUFFICIENT_CHIPS"}, {platform.ErrIdempotencyConflict, "IDEMPOTENCY_CONFLICT"}, {games.ErrMaintenance, "MAINTENANCE"}, {errors.New("secret"), "UPSTREAM_UNAVAILABLE"}} {
		t.Run(tc.want, func(t *testing.T) {
			s, g, _, _, _ := newSummonFixture(t)
			q := prepareSummonQuote(t, s, "SINGLE")
			g.createErr = tc.err
			_, e := s.PlaySummon(context.Background(), testSubject, q)
			requireFault(t, e, tc.want)
		})
	}
	s, g, r, _, _ := newSummonFixture(t)
	r.err = Fault{Code: "NOT_LINKED"}
	_, e := s.PrepareSummon(context.Background(), testSubject, SummonPrepareInput{testRequest, "11", "SINGLE"})
	requireFault(t, e, "NOT_LINKED")
	r.err = nil
	g.bootstrapErr = errors.New("secret")
	_, e = s.PrepareSummon(context.Background(), testSubject, SummonPrepareInput{testRequest, "11", "SINGLE"})
	requireFault(t, e, "UPSTREAM_UNAVAILABLE")
	g.bootstrapErr = nil
	q := prepareSummonQuote(t, s, "SINGLE")
	g.findErr = errors.New("secret")
	_, e = s.PlaySummon(context.Background(), testSubject, q)
	requireFault(t, e, "UPSTREAM_UNAVAILABLE")
	if g.createCalls != 0 {
		t.Fatal("failed lookup created")
	}
	g.findErr = nil
	g.created.Input.Mode = "TENFOLD"
	_, e = s.PlaySummon(context.Background(), testSubject, q)
	requireFault(t, e, "IDEMPOTENCY_CONFLICT")
}
