package botgames

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestSlotPrepareWholeChipAndFractionalLineStake(t *testing.T) {
	for _, tc := range []struct{ wager, line string }{{"10", "500000"}, {"11", "550000"}} {
		t.Run(tc.wager, func(t *testing.T) {
			s, g, _, _, _ := newFixture(t)
			g.bootstrap.Game.Slug = "slot"
			g.bootstrap.Game.Config.Algorithm = "slot-map-v1"
			g.bootstrap.Game.Config.Schema = "slot-config-v1"
			g.bootstrap.EconomicPolicy = &platform.EconomicPolicy{Version: "economy-cap-v1", Hash: strings.Repeat("a", 64), SinglePlayerMaxUnits: 500000000000, AssetCapUnits: 50000000000000000, CapMode: "CLIP_PROFIT"}
			g.bootstrap.Next.Ruleset = "slot-rules-v3"
			g.bootstrap.Game.Config.Ruleset = "slot-rules-v3"
			g.bootstrap.Game.Config.Schema = "slot-config-v3"
			p, e := s.PrepareSlot(context.Background(), testSubject, SlotPrepareInput{RequestID: testRequest, TotalWager: tc.wager})
			if e != nil {
				t.Fatal(e)
			}
			if p.TotalWager != tc.wager || p.LineCount != 10 || p.LineStakeUnits != tc.line || p.MaximumWagerUnits != "50000000" || p.MinimumWagerUnits != "5000000" || p.ExpiresAt != "1700000120" || p.Ruleset != "slot-rules-v3" || p.RulesText == "" || len([]rune(p.RulesText)) > 1024 {
				t.Fatal(p)
			}
			if g.slug != "slot" || g.bootstrapCalls != 1 || g.findCalls != 0 || g.createCalls != 0 {
				t.Fatal("prepare must only bootstrap slot")
			}
			raw, _ := json.Marshal(p)
			var fields map[string]any
			if e = json.Unmarshal(raw, &fields); e != nil {
				t.Fatal(e)
			}
			keys := []string{}
			for k, v := range fields {
				keys = append(keys, k)
				if k != "line_count" {
					if _, ok := v.(string); !ok {
						t.Fatal(k, "not string")
					}
				}
			}
			sort.Strings(keys)
			want := []string{"available_units", "commitment_id", "expires_at", "line_count", "line_stake_units", "maximum_wager_units", "minimum_wager_units", "quote", "rules_text", "ruleset", "server_seed_hash", "total_wager"}
			if !reflect.DeepEqual(keys, want) || fields["line_count"] != float64(10) {
				t.Fatalf("wrong response shape: %s", raw)
			}
		})
	}
}

func newSlotFixture(t *testing.T) (*Service, *fakeGames, *fakeResolver, *time.Time, *[]string) {
	t.Helper()
	s, g, r, now, order := newFixture(t)
	g.bootstrap.Game.Slug = "slot"
	g.bootstrap.Game.Config.Algorithm = "slot-map-v1"
	g.bootstrap.Game.Config.Schema = "slot-config-v1"
	g.bootstrap.EconomicPolicy = &platform.EconomicPolicy{Version: "economy-cap-v1", Hash: strings.Repeat("a", 64), SinglePlayerMaxUnits: 500000000000, AssetCapUnits: 50000000000000000, CapMode: "CLIP_PROFIT"}
	g.bootstrap.Next.Ruleset = "slot-rules-v1"
	g.bootstrap.Game.Config.Ruleset = "slot-rules-v1"
	g.created = fixtureSlotRound(t, "slot-rules-v1", "11", [5]int{24, 6, 31, 2, 14})
	return s, g, r, now, order
}

func prepareSlotQuote(t *testing.T, s *Service) string {
	t.Helper()
	p, e := s.PrepareSlot(context.Background(), testSubject, SlotPrepareInput{testRequest, "11"})
	if e != nil {
		t.Fatal(e)
	}
	return p.Quote
}

func TestSlotPlayDelegatesAuthoritativeSingleRound(t *testing.T) {
	s, g, r, _, order := newSlotFixture(t)
	q := prepareSlotQuote(t, s)
	*order = nil
	got, e := s.PlaySlot(context.Background(), testSubject, q)
	if e != nil {
		t.Fatal(e)
	}
	if got.TotalWager != "11" || got.StakeUnits != "5500000" || got.LineStakeUnits != "550000" || got.GrossPayoutUnits != "2779700000" || got.ActualNetUnits != "2774200000" || got.Result != "WIN" || got.ResultDetail != "WIN" {
		t.Fatal(got)
	}
	if !reflect.DeepEqual(*order, []string{"resolve", "find", "create"}) || r.calls != 2 || g.createCalls != 1 || g.findCalls != 1 {
		t.Fatal(*order)
	}
	if g.slug != "slot" || g.key != "discord-slot-v1:970000000000000001" || g.commitment != testCommitment || g.input != (games.CreateInput{Type: "SLOT", TotalWager: "11"}) {
		t.Fatalf("wrong authoritative create: %+v", g)
	}
}

func TestSlotPrepareAdmissionAndOverflowLimits(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		change               func(*fakeGames, *fakeResolver)
		wager, want, maximum string
	}{
		{"minimum", func(g *fakeGames, r *fakeResolver) { g.bootstrap.WagerPolicy.Minimum = 6000000 }, "11", "INVALID_REQUEST", ""},
		{"maintenance", func(g *fakeGames, r *fakeResolver) { g.bootstrap.Game.State = "MAINTENANCE" }, "11", "MAINTENANCE", ""},
		{"not_play", func(g *fakeGames, r *fakeResolver) { g.bootstrap.EntryAction = "READ_ONLY" }, "11", "UPSTREAM_UNAVAILABLE", ""},
		{"not_slot", func(g *fakeGames, r *fakeResolver) { g.bootstrap.Game.Slug = "dice" }, "11", "UPSTREAM_UNAVAILABLE", ""},
		{"balance", func(g *fakeGames, r *fakeResolver) { g.bootstrap.AvailableUnits = 5499999 }, "11", "INSUFFICIENT_CHIPS", ""},
		{"economy", func(g *fakeGames, r *fakeResolver) { g.bootstrap.EconomicPolicy.SinglePlayerMaxUnits = 5000000 }, "11", "INVALID_REQUEST", ""},
		{"rounded_balance", func(g *fakeGames, r *fakeResolver) { g.bootstrap.AvailableUnits = 55499999 }, "11", "", "55000000"},
		{"rounded_cap", func(g *fakeGames, r *fakeResolver) { g.bootstrap.EconomicPolicy.SinglePlayerMaxUnits = 11999999 }, "11", "", "11500000"},
		{"v1_guard_boundary", func(g *fakeGames, r *fakeResolver) {
			g.bootstrap.AvailableUnits = math.MaxInt64 - 2834700000
			g.bootstrap.EconomicPolicy = nil
		}, "11", "", "5500000"},
		{"v1_guard_above", func(g *fakeGames, r *fakeResolver) {
			g.bootstrap.AvailableUnits = math.MaxInt64 - 2834700000
			g.bootstrap.EconomicPolicy = nil
		}, "12", "INVALID_REQUEST", ""},
		{"v2_guard_boundary", func(g *fakeGames, r *fakeResolver) {
			g.bootstrap.Next.Ruleset = "slot-rules-v2"
			g.bootstrap.Game.Config.Ruleset = "slot-rules-v2"
			g.bootstrap.Game.Config.Schema = "slot-config-v2"
			g.bootstrap.AvailableUnits = math.MaxInt64 - 2834700000
			g.bootstrap.EconomicPolicy = nil
		}, "11", "", "5500000"},
		{"v3_stricter_guard", func(g *fakeGames, r *fakeResolver) {
			g.bootstrap.Next.Ruleset = "slot-rules-v3"
			g.bootstrap.Game.Config.Ruleset = "slot-rules-v3"
			g.bootstrap.Game.Config.Schema = "slot-config-v3"
			g.bootstrap.AvailableUnits = math.MaxInt64 - 2834700000
			g.bootstrap.EconomicPolicy = nil
		}, "11", "INVALID_REQUEST", ""},
		{"v3_guard_boundary", func(g *fakeGames, r *fakeResolver) {
			g.bootstrap.Next.Ruleset = "slot-rules-v3"
			g.bootstrap.Game.Config.Ruleset = "slot-rules-v3"
			g.bootstrap.Game.Config.Schema = "slot-config-v3"
			g.bootstrap.AvailableUnits = math.MaxInt64 - 2855050000
			g.bootstrap.EconomicPolicy = nil
		}, "11", "", "5500000"},
		{"no_overflow_room", func(g *fakeGames, r *fakeResolver) {
			g.bootstrap.AvailableUnits = math.MaxInt64
			g.bootstrap.EconomicPolicy = nil
		}, "10", "INVALID_REQUEST", ""},
		{"payout_overflow", func(g *fakeGames, r *fakeResolver) {
			g.bootstrap.AvailableUnits = math.MaxInt64 / 2
			g.bootstrap.EconomicPolicy = nil
		}, "35720263506", "INVALID_REQUEST", ""},
		{"resolver", func(g *fakeGames, r *fakeResolver) { r.err = Fault{Code: "NOT_LINKED"} }, "11", "NOT_LINKED", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, g, r, _, _ := newSlotFixture(t)
			tc.change(g, r)
			p, e := s.PrepareSlot(context.Background(), testSubject, SlotPrepareInput{testRequest, tc.wager})
			if tc.want != "" {
				requireFault(t, e, tc.want)
			} else if e != nil || p.MaximumWagerUnits != tc.maximum {
				t.Fatal(p, e)
			}
			if g.createCalls != 0 || g.findCalls != 0 {
				t.Fatal("prepare created round")
			}
		})
	}
}

func TestSlotPrepareRejectsMalformedInputAndPolicy(t *testing.T) {
	for _, wager := range []string{"", "0", "9", "-10", "+10", "010", "11.0", "1e1", "１０", " 10", "10 ", "18446744073710", "9223372036854775808"} {
		t.Run("wager_"+wager, func(t *testing.T) {
			s, g, r, _, _ := newSlotFixture(t)
			_, e := s.PrepareSlot(context.Background(), testSubject, SlotPrepareInput{testRequest, wager})
			requireFault(t, e, "INVALID_REQUEST")
			if r.calls != 0 || g.bootstrapCalls != 0 {
				t.Fatal("malformed wager reached authority")
			}
		})
	}
	for _, tc := range []struct {
		name, want string
		change     func(*games.Bootstrap)
	}{
		{"missing_config", "UPSTREAM_UNAVAILABLE", func(b *games.Bootstrap) { b.Game.Config = nil }},
		{"unknown_rules", "UPSTREAM_UNAVAILABLE", func(b *games.Bootstrap) { b.Next.Ruleset = "slot-rules-v99"; b.Game.Config.Ruleset = b.Next.Ruleset }},
		{"mismatched_rules", "UPSTREAM_UNAVAILABLE", func(b *games.Bootstrap) { b.Game.Config.Ruleset = "slot-rules-v2" }},
		{"mismatched_schema", "UPSTREAM_UNAVAILABLE", func(b *games.Bootstrap) { b.Game.Config.Schema = "slot-config-v3" }},
		{"unknown_algorithm", "UPSTREAM_UNAVAILABLE", func(b *games.Bootstrap) { b.Game.Config.Algorithm = "slot-map-v99" }},
		{"missing_commitment", "COMMITMENT_INVALID", func(b *games.Bootstrap) { b.Next = nil }},
		{"invalid_commitment", "COMMITMENT_INVALID", func(b *games.Bootstrap) { b.Next.ID = "private" }},
		{"bad_seed_hash", "COMMITMENT_INVALID", func(b *games.Bootstrap) { b.Next.ServerSeedHash = strings.Repeat("A", 64) }},
		{"minimum_low", "UPSTREAM_UNAVAILABLE", func(b *games.Bootstrap) { b.WagerPolicy.Minimum = 4500000 }},
		{"minimum_fraction", "UPSTREAM_UNAVAILABLE", func(b *games.Bootstrap) { b.WagerPolicy.Minimum = 5500001 }},
		{"bad_step", "UPSTREAM_UNAVAILABLE", func(b *games.Bootstrap) { b.WagerPolicy.Step = 1 }},
		{"bad_mode", "UPSTREAM_UNAVAILABLE", func(b *games.Bootstrap) { b.WagerPolicy.MaximumMode = "FUTURE" }},
		{"negative_balance", "UPSTREAM_UNAVAILABLE", func(b *games.Bootstrap) { b.AvailableUnits = -1 }},
		{"zero_economic_max", "UPSTREAM_UNAVAILABLE", func(b *games.Bootstrap) { b.EconomicPolicy.SinglePlayerMaxUnits = 0 }},
		{"unknown_economy", "UPSTREAM_UNAVAILABLE", func(b *games.Bootstrap) { b.EconomicPolicy.Version = "unknown" }},
		{"bad_economic_hash", "UPSTREAM_UNAVAILABLE", func(b *games.Bootstrap) { b.EconomicPolicy.Hash = "" }},
		{"bad_economic_cap", "UPSTREAM_UNAVAILABLE", func(b *games.Bootstrap) { b.EconomicPolicy.AssetCapUnits = 0 }},
		{"bad_economic_mode", "UPSTREAM_UNAVAILABLE", func(b *games.Bootstrap) { b.EconomicPolicy.CapMode = "ALLOW_OVERFLOW" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, g, _, _, _ := newSlotFixture(t)
			tc.change(&g.bootstrap)
			_, e := s.PrepareSlot(context.Background(), testSubject, SlotPrepareInput{testRequest, "11"})
			requireFault(t, e, tc.want)
		})
	}
}

func TestSlotIdentityAndLookupOnlyRecovery(t *testing.T) {
	for _, lookup := range []bool{false, true} {
		for _, tc := range []struct {
			name, want string
			change     func(*string, *string, *fakeResolver)
		}{
			{"subject", "UNAUTHORIZED", func(q, subject *string, r *fakeResolver) { *subject = "970000000000000003" }},
			{"bad_subject", "INVALID_REQUEST", func(q, subject *string, r *fakeResolver) { *subject = "01" }},
			{"binding", "BINDING_CHANGED", func(q, subject *string, r *fakeResolver) { r.user++ }},
			{"restricted", "ACCOUNT_RESTRICTED", func(q, subject *string, r *fakeResolver) { r.err = Fault{Code: "ACCOUNT_RESTRICTED"} }},
			{"signature", "UNAUTHORIZED", func(q, subject *string, r *fakeResolver) {
				*q = strings.Split(*q, ".")[0] + "." + strings.Repeat("A", 43)
			}},
		} {
			t.Run(fmt.Sprintf("%s_lookup_%t", tc.name, lookup), func(t *testing.T) {
				s, g, r, _, _ := newSlotFixture(t)
				q := prepareSlotQuote(t, s)
				subject := testSubject
				tc.change(&q, &subject, r)
				var e error
				if lookup {
					_, e = s.LookupSlot(context.Background(), subject, q)
				} else {
					_, e = s.PlaySlot(context.Background(), subject, q)
				}
				requireFault(t, e, tc.want)
				if g.findCalls != 0 || g.createCalls != 0 {
					t.Fatal("identity failure reached engine")
				}
			})
		}
	}
	for _, tc := range []struct {
		name, want    string
		lookup, found bool
		advance       time.Duration
		change        func(*games.GameRound)
	}{
		{"missing_lookup", "NOT_FOUND", true, false, 0, nil},
		{"expired_missing_lookup", "NOT_FOUND", true, false, 121 * time.Second, nil},
		{"expiry_boundary", "QUOTE_EXPIRED", false, false, 120 * time.Second, nil},
		{"expired_existing", "", false, true, 121 * time.Second, nil},
		{"expired_existing_lookup", "", true, true, 121 * time.Second, nil},
		{"conflicting_wager", "IDEMPOTENCY_CONFLICT", false, true, 0, func(r *games.GameRound) { r.Input.TotalWager = "10" }},
		{"conflicting_input", "IDEMPOTENCY_CONFLICT", true, true, 0, func(r *games.GameRound) { r.Input.Choice = "BIG" }},
		{"new_commitment_existing", "", false, true, 0, func(r *games.GameRound) { r.CommitmentID = "00000000-0000-4000-8000-000000000099" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, g, r, now, order := newSlotFixture(t)
			q := prepareSlotQuote(t, s)
			*now = now.Add(tc.advance)
			if tc.found {
				g.found = &g.created
				if tc.change != nil {
					tc.change(g.found)
				}
			}
			restarted, e := NewService(g, r, testKey, func() time.Time { return *now })
			if e != nil {
				t.Fatal(e)
			}
			*order = nil
			var got SlotResult
			if tc.lookup {
				got, e = restarted.LookupSlot(context.Background(), testSubject, q)
			} else {
				got, e = restarted.PlaySlot(context.Background(), testSubject, q)
			}
			if tc.want != "" {
				requireFault(t, e, tc.want)
			} else if e != nil || got.RoundID != g.created.ID {
				t.Fatal(got, e)
			}
			if g.createCalls != 0 || !reflect.DeepEqual(*order, []string{"resolve", "find"}) {
				t.Fatal("recovery tried a new round", *order)
			}
		})
	}
	t.Run("unknown_lookup_never_creates", func(t *testing.T) {
		s, g, _, _, _ := newSlotFixture(t)
		q := prepareSlotQuote(t, s)
		g.findErr = errors.New("outcome unknown")
		_, e := s.PlaySlot(context.Background(), testSubject, q)
		requireFault(t, e, "UPSTREAM_UNAVAILABLE")
		if g.createCalls != 0 {
			t.Fatal("created after uncertain lookup")
		}
	})
}
