package botgames

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/games"
	bj "github.com/cy4268/momiao/internal/games/blackjack"
	"github.com/cy4268/momiao/internal/platform"
)

const bjHand = "00000000-0000-7000-8000-000000000004"
const bjRound = "00000000-0000-7000-8000-000000000003"

type blackjackFake struct {
	applied *games.GameRound
	*fakeGames
	current                         games.GameRound
	original                        *games.GameRound
	readErr, actionErr, matchingErr error
	reads, actions, matches         int
	actionInput                     games.BlackjackActionInput
	roundID                         string
}

func (g *blackjackFake) Read(_ context.Context, _ int64, id string) (games.GameRound, error) {
	g.reads++
	g.roundID = id
	return g.current, g.readErr
}
func (g *blackjackFake) BlackjackAction(_ context.Context, _ int64, id string, in games.BlackjackActionInput) (games.GameRound, error) {
	g.actions++
	g.roundID = id
	g.actionInput = in
	if g.applied != nil {
		return *g.applied, g.actionErr
	}
	if g.original == nil {
		return games.GameRound{}, g.actionErr
	}
	return *g.original, g.actionErr
}
func (g *blackjackFake) FindBlackjackActionMatching(_ context.Context, _ int64, id string, in games.BlackjackActionInput) (*games.GameRound, error) {
	g.matches++
	g.roundID = id
	g.actionInput = in
	return g.original, g.matchingErr
}
func blackjackFixtureRound(t *testing.T, natural bool) games.GameRound {
	t.Helper()
	head := []uint16{7, 5, 20, 6, 2, 3, 4}
	if natural {
		head = []uint16{0, 5, 9, 6}
	}
	var shoe [312]uint16
	used := map[uint16]bool{}
	i := 0
	for _, c := range head {
		shoe[i] = c
		i++
		used[c] = true
	}
	for c := uint16(0); c < 312; c++ {
		if !used[c] {
			shoe[i] = c
			i++
		}
	}
	at := time.Unix(1700000000, 0).UTC()
	state, e := bj.New(shoe, 5000000, bjHand, at)
	if e != nil {
		t.Fatal(e)
	}
	view := bj.PublicView(state, 50000000)
	r := games.GameRound{ID: bjRound, Game: "blackjack", State: string(state.Phase), RecoveryState: "NORMAL", Input: games.CreateInput{Type: "BLACKJACK", InitialWager: "10"}, StakeUnits: state.TotalStakeUnits, PayoutUnits: state.TotalPayoutUnits, NetUnits: state.TotalPayoutUnits - state.TotalStakeUnits, Outcome: games.Outcome(state.Class), Ruleset: bj.RulesetVersion, Algorithm: bj.AlgorithmVersion, CreatedAt: at, Blackjack: &view}
	if natural {
		r.SettledAt = &at
	}
	return r
}
func newBlackjackFixture(t *testing.T) (*Service, *blackjackFake, *fakeResolver, *time.Time) {
	s, base, res, now, _ := newFixture(t)
	g := &blackjackFake{fakeGames: base, current: blackjackFixtureRound(t, false)}
	base.created = g.current
	s.games = g
	b := &g.bootstrap
	c, e := games.BlackjackV2(testCommitment)
	if e != nil {
		t.Fatal(e)
	}
	bind := c.Binding()
	hash := hex.EncodeToString(bind.Hash[:])
	b.Game = games.CatalogEntry{Slug: "blackjack", State: "PLAY", Config: &games.ConfigSummary{Version: bind.Version, Hash: hash, Schema: bind.SchemaVersion, Ruleset: bind.RulesetVersion, Algorithm: bind.AlgorithmVersion}}
	b.Next = &games.Commitment{ID: "00000000-0000-7000-8000-000000000001", ConfigVersion: bind.Version, ConfigHash: hash, Ruleset: bind.RulesetVersion, Algorithm: bind.AlgorithmVersion, ServerSeedHash: strings.Repeat("a", 64), Resources: json.RawMessage(`{"shuffle_algorithm_version":"blackjack-fy-v1","fair_return_version":"blackjack-fair-return-v1"}`), EconomicVersion: platform.EconomicPolicyVersion}
	b.WagerPolicy.ID = "00000000-0000-7000-8000-000000000008"
	b.WagerPolicy.Hash = strings.Repeat("d", 64)
	b.Next.PolicyVersion, b.Next.PolicyHash = b.WagerPolicy.ID, b.WagerPolicy.Hash
	b.EconomicPolicy = &platform.EconomicPolicy{Version: platform.EconomicPolicyVersion, Hash: strings.Repeat("b", 64), SinglePlayerMaxUnits: platform.SinglePlayerMaxUnits, AssetCapUnits: platform.AssetCapUnits, CapMode: "CLIP_PROFIT"}
	return s, g, res, now
}
func blackjackDeal(t *testing.T, s *Service) BlackjackPrepared {
	t.Helper()
	p, e := s.PrepareBlackjack(context.Background(), testSubject, BlackjackPrepareInput{RequestID: testRequest, InitialWager: "10"})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestBlackjackPrepareAndResume(t *testing.T) {
	s, g, _, _ := newBlackjackFixture(t)
	p := blackjackDeal(t, s)
	if p.Action != "DEAL" || p.InitialWager != "10" || p.StakeUnits != "5000000" || p.MaximumWagerUnits != "50000000" || p.ExpiresAt != "1700000120" || g.createCalls != 0 {
		t.Fatalf("prepare terms %+v", p)
	}
	raw, _ := json.Marshal(p)
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	if len(fields) != 12 || fields["round"] != nil {
		t.Fatal(string(raw))
	}
	g.bootstrap.EntryAction = "RESUME"
	g.bootstrap.Active = &g.current
	g.bootstrap.Next = nil
	g.bootstrap.Game.State = "MAINTENANCE"
	p, e := s.PrepareBlackjack(context.Background(), testSubject, BlackjackPrepareInput{RequestID: testRequest, InitialWager: "999999999"})
	if e != nil || p.Action != "RESUME" || p.Round == nil || p.Round.RoundID != bjRound || g.createCalls != 0 || g.reads != 1 {
		t.Fatal("resume must read original without price", e)
	}
	raw, _ = json.Marshal(p)
	_ = json.Unmarshal(raw, &fields)
	var union map[string]any
	_ = json.Unmarshal(raw, &union)
	if len(union) != 2 || union["round"] == nil {
		t.Fatal(string(raw))
	}
}
func TestBlackjackLimitsAndConfig(t *testing.T) {
	for _, bad := range []string{"0", "9", "010", "10.0", "-10", "+10", "18446744073709551615"} {
		s, _, _, _ := newBlackjackFixture(t)
		_, e := s.PrepareBlackjack(context.Background(), testSubject, BlackjackPrepareInput{RequestID: testRequest, InitialWager: bad})
		wantBlackjackFault(t, e, "INVALID_REQUEST")
	}
	for _, balance := range []int64{0, 5000001, math.MaxInt64, math.MaxInt64 - 80000000} {
		s, g, _, _ := newBlackjackFixture(t)
		g.bootstrap.AvailableUnits = balance
		_, max, e := blackjackWagerLimits(g.bootstrap)
		if e != nil {
			t.Fatal(e)
		}
		want := min(balance, math.MaxInt64/17, (math.MaxInt64-balance)/16, platform.SinglePlayerMaxUnits)
		want -= want % games.UnitsPerChip
		if max != want {
			t.Fatal(max, want)
		}
		_ = s
	}
	for _, mutate := range []func(*games.Bootstrap){func(b *games.Bootstrap) { b.Next.ConfigHash = strings.Repeat("c", 64) }, func(b *games.Bootstrap) { b.Next.Ruleset = bj.RulesetVersion }, func(b *games.Bootstrap) { b.Game.Config.Schema = "new" }, func(b *games.Bootstrap) { b.Next.Resources = json.RawMessage(`{"shuffle_algorithm_version":"wrong"}`) }, func(b *games.Bootstrap) { b.EconomicPolicy.SinglePlayerMaxUnits++ }} {
		s, g, _, _ := newBlackjackFixture(t)
		mutate(&g.bootstrap)
		_, e := s.PrepareBlackjack(context.Background(), testSubject, BlackjackPrepareInput{RequestID: testRequest, InitialWager: "10"})
		wantBlackjackFault(t, e, "UPSTREAM_UNAVAILABLE")
	}
}
func TestBlackjackOriginalDealAndExpiredState(t *testing.T) {
	s, g, _, now := newBlackjackFixture(t)
	p := blackjackDeal(t, s)
	*now = now.Add(121 * time.Second)
	_, e := s.LookupBlackjack(context.Background(), testSubject, p.Quote)
	wantBlackjackFault(t, e, "NOT_FOUND")
	_, e = s.PlayBlackjack(context.Background(), testSubject, p.Quote)
	wantBlackjackFault(t, e, "QUOTE_EXPIRED")
	if g.createCalls != 0 {
		t.Fatal("expired create")
	}
	g.found = &g.current
	r, e := s.PlayBlackjack(context.Background(), testSubject, p.Quote)
	if e != nil || r.RoundID != bjRound || g.key != "discord-blackjack-v1:"+testRequest || g.createCalls != 0 {
		t.Fatal(r, e)
	}
	*now = now.Add(121 * time.Second)
	r, e = s.StateBlackjack(context.Background(), testSubject, r.Quote)
	if e != nil || r.RoundID != bjRound {
		t.Fatal(e)
	}
	_, e = s.StateBlackjack(context.Background(), testSubject, p.Quote)
	wantBlackjackFault(t, e, "INVALID_REQUEST")
	_, e = s.PlayBlackjack(context.Background(), testSubject, r.Quote)
	wantBlackjackFault(t, e, "INVALID_REQUEST")
	g.found.Input.InitialWager = "11"
	_, e = s.LookupBlackjack(context.Background(), testSubject, p.Quote)
	wantBlackjackFault(t, e, "IDEMPOTENCY_CONFLICT")
}
func TestBlackjackActionOriginalReceiptFreshRound(t *testing.T) {
	s, g, _, now := newBlackjackFixture(t)
	initial, e := s.projectBlackjackRound(testSubject, 424242, g.current)
	if e != nil {
		t.Fatal(e)
	}
	in := BlackjackActionRequest{Quote: initial.Quote, RequestID: testRequest, ActionType: "HIT"}
	_, e = s.LookupBlackjackAction(context.Background(), testSubject, in)
	wantBlackjackFault(t, e, "NOT_FOUND")
	if g.actions != 0 || g.reads != 0 {
		t.Fatal("lookup wrote/read absent action")
	}
	original := blackjackFixtureRound(t, false)
	original.Blackjack.Version = 2
	g.original = &original
	g.current = blackjackFixtureRound(t, false)
	g.current.Blackjack.Version = 3
	*now = now.Add(121 * time.Second)
	out, e := s.ActBlackjack(context.Background(), testSubject, in)
	if e != nil || out.AppliedRoundVersion != "2" || out.Round.RoundVersion != "3" || out.AdditionalStakeUnits != "0" || g.actions != 0 || g.matches != 2 {
		t.Fatal("durable receipt/current separation", out, e)
	}
	if out.ActionID != g.actionInput.ActionID || out.HandID != bjHand || out.RequestID != testRequest {
		t.Fatal(out)
	}
	id := g.actionInput.ActionID
	in.ActionType = "STAND"
	g.matchingErr = platform.ErrIdempotencyConflict
	_, e = s.ActBlackjack(context.Background(), testSubject, in)
	wantBlackjackFault(t, e, "IDEMPOTENCY_CONFLICT")
	if g.actionInput.ActionID != id {
		t.Fatal("id changed with action semantics")
	}
	g.matchingErr = nil
	g.original = nil
	_, e = s.ActBlackjack(context.Background(), testSubject, in)
	wantBlackjackFault(t, e, "QUOTE_EXPIRED")
	g.original = &original
	g.readErr = errors.New("read after committed action")
	_, e = s.LookupBlackjackAction(context.Background(), testSubject, in)
	wantBlackjackFault(t, e, "UPSTREAM_UNAVAILABLE")
}
func TestBlackjackProjectionPrivacyExactShapeAndEconomy(t *testing.T) {
	s, g, _, _ := newBlackjackFixture(t)
	r, e := s.projectBlackjackRound(testSubject, 424242, g.current)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(r)
	var f map[string]any
	_ = json.Unmarshal(raw, &f)
	if len(f) != 18 || f["dealer_total"] != nil || f["settlement"] != nil || f["available_units"] != nil || len(f["hands"].([]any)[0].(map[string]any)) != 10 {
		t.Fatal(string(raw))
	}
	if f["hands"].([]any)[0].(map[string]any)["result"] != "" {
		t.Fatal("empty result omitted")
	}
	r.Hands[0].Cards[0] = 50
	if g.current.Blackjack.Hands[0].Cards[0] == 50 {
		t.Fatal("shared hand cards")
	}
	g.current.EconomicVersion = platform.EconomicPolicyVersion
	g.current.StakeUnits = platform.SinglePlayerMaxUnits
	g.current.Input.InitialWager = "1000000"
	g.current.Blackjack.TotalStakeUnits = g.current.StakeUnits
	g.current.Blackjack.Hands[0].StakeUnits = g.current.StakeUnits
	g.current.Blackjack.Hands[0].NetChangeUnits = 0
	g.current.NetUnits = -g.current.StakeUnits
	r, e = s.projectBlackjackRound(testSubject, 424242, g.current)
	if e != nil || !reflect.DeepEqual(r.LegalActions, []string{"HIT", "STAND"}) {
		t.Fatal("economic cap filter", r.LegalActions, e)
	}
	g.current = blackjackFixtureRound(t, true)
	g.current.BalanceAfterUnits = 999
	r, e = s.projectBlackjackRound(testSubject, 424242, g.current)
	if e != nil || r.Settlement == nil || r.LastPlayerActionAt != "" || r.AutoResolveAt != "" || r.ActiveHandID != "" || r.LegalActions == nil {
		t.Fatal("natural terminal", r, e)
	}
	g.current.RecoveryState = "NEEDS_REVIEW"
	_, e = s.projectBlackjackRound(testSubject, 424242, g.current)
	wantBlackjackFault(t, e, "BLACKJACK_NEEDS_REVIEW")
}
func wantBlackjackFault(t *testing.T, e error, want string) {
	t.Helper()
	if e == nil || e.Error() != want {
		t.Fatalf("fault=%v want=%s", e, want)
	}
}

func TestBlackjackCumulativeCapActionIsDefinitive(t *testing.T) {
	s, g, _, _ := newBlackjackFixture(t)
	r, e := s.projectBlackjackRound(testSubject, 424242, g.current)
	if e != nil {
		t.Fatal(e)
	}
	g.actionErr = games.ErrInvalidInput
	_, e = s.ActBlackjack(context.Background(), testSubject, BlackjackActionRequest{Quote: r.Quote, RequestID: testRequest, ActionType: "DOUBLE"})
	wantBlackjackFault(t, e, "BLACKJACK_ACTION_NOT_ALLOWED")
	if g.actions != 1 || g.matches != 1 {
		t.Fatal("action attempt count")
	}
}

func TestBlackjackPrepareRequiresCommitmentPolicyCoherence(t *testing.T) {
	s, g, _, _ := newBlackjackFixture(t)
	g.bootstrap.WagerPolicy.ID = "00000000-0000-7000-8000-000000000008"
	g.bootstrap.WagerPolicy.Hash = strings.Repeat("d", 64)
	g.bootstrap.Next.PolicyVersion = g.bootstrap.WagerPolicy.ID
	g.bootstrap.Next.PolicyHash = strings.Repeat("e", 64)
	_, e := s.PrepareBlackjack(context.Background(), testSubject, BlackjackPrepareInput{RequestID: testRequest, InitialWager: "10"})
	wantBlackjackFault(t, e, "UPSTREAM_UNAVAILABLE")
}

func TestBlackjackPostCommitCurrentReadAlwaysUnknown(t *testing.T) {
	for _, write := range []bool{false, true} {
		for _, failure := range []string{"not_found", "needs_review_error", "needs_review_projection"} {
			t.Run(fmt.Sprintf("write_%t/%s", write, failure), func(t *testing.T) {
				s, g, _, _ := newBlackjackFixture(t)
				r, e := s.projectBlackjackRound(testSubject, 424242, g.current)
				if e != nil {
					t.Fatal(e)
				}
				original := blackjackFixtureRound(t, false)
				original.Blackjack.Version = 2
				g.current = original
				if write {
					g.applied = &original
				} else {
					g.original = &original
				}
				switch failure {
				case "not_found":
					g.readErr = games.ErrNotFound
				case "needs_review_error":
					g.readErr = bj.ErrNeedsReview
				case "needs_review_projection":
					g.current.RecoveryState = "NEEDS_REVIEW"
				}
				_, e = s.ActBlackjack(context.Background(), testSubject, BlackjackActionRequest{Quote: r.Quote, RequestID: testRequest, ActionType: "HIT"})
				wantBlackjackFault(t, e, "UPSTREAM_UNAVAILABLE")
				wantWrites := 0
				if write {
					wantWrites = 1
				}
				if g.matches != 1 || g.reads != 1 || g.actions != wantWrites {
					t.Fatal("lookup/read/write counts", g.matches, g.reads, g.actions)
				}
			})
		}
	}
}

func TestBlackjackV1AndV2FrozenPreviewRules(t *testing.T) {
	for _, v2 := range []bool{false, true} {
		t.Run(fmt.Sprintf("fair_%t", v2), func(t *testing.T) {
			s, g, _, _ := newBlackjackFixture(t)
			c, e := games.BlackjackV1(testCommitment)
			if v2 {
				c, e = games.BlackjackV2(testCommitment)
			}
			if e != nil {
				t.Fatal(e)
			}
			b := c.Binding()
			g.bootstrap.Game.Config = &games.ConfigSummary{Version: b.Version, Hash: hex.EncodeToString(b.Hash[:]), Schema: b.SchemaVersion, Ruleset: b.RulesetVersion, Algorithm: b.AlgorithmVersion}
			g.bootstrap.Next.ConfigVersion = b.Version
			g.bootstrap.Next.ConfigHash = g.bootstrap.Game.Config.Hash
			g.bootstrap.Next.Ruleset = b.RulesetVersion
			g.bootstrap.Next.Algorithm = b.AlgorithmVersion
			if !v2 {
				g.bootstrap.Next.Resources = json.RawMessage(`{"shuffle_algorithm_version":"blackjack-fy-v1"}`)
				g.bootstrap.EconomicPolicy = nil
				g.bootstrap.Next.EconomicVersion = ""
			}
			p := blackjackDeal(t, s)
			if p.Ruleset != b.RulesetVersion || strings.Contains(p.RulesText, "公平返还") != v2 || len([]rune(p.RulesText)) > 1024 {
				t.Fatal("frozen rules projection", p.Ruleset)
			}
		})
	}
}
