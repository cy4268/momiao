package botgames

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
)

const testSubject = "970000000000000002"
const testRequest = "970000000000000001"
const testCommitment = "00000000-0000-4000-8000-000000000001"

var testKey = [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9}

type fakeResolver struct {
	user    int64
	err     error
	calls   int
	subject string
	order   *[]string
}

func (f *fakeResolver) Resolve(_ context.Context, subject string) (int64, error) {
	f.calls++
	f.subject = subject
	*f.order = append(*f.order, "resolve")
	return f.user, f.err
}

type fakeGames struct {
	bootstrap                              games.Bootstrap
	bootstrapErr, findErr, createErr       error
	found                                  *games.GameRound
	created                                games.GameRound
	bootstrapCalls, findCalls, createCalls int
	user                                   int64
	slug, key, commitment                  string
	input                                  games.CreateInput
	order                                  *[]string
}

func (f *fakeGames) Bootstrap(_ context.Context, user int64, slug string) (games.Bootstrap, error) {
	f.bootstrapCalls++
	f.user, f.slug = user, slug
	*f.order = append(*f.order, "bootstrap")
	return f.bootstrap, f.bootstrapErr
}

func (f *fakeGames) FindByKey(_ context.Context, user int64, slug, key string) (*games.GameRound, error) {
	f.findCalls++
	f.user, f.slug, f.key = user, slug, key
	*f.order = append(*f.order, "find")
	return f.found, f.findErr
}

func (f *fakeGames) Create(_ context.Context, user int64, slug, key, commitment string, input games.CreateInput) (games.GameRound, error) {
	f.createCalls++
	f.user, f.slug, f.key, f.commitment, f.input = user, slug, key, commitment, input
	*f.order = append(*f.order, "create")
	return f.created, f.createErr
}

func fixtureRound() games.GameRound {
	created := time.Unix(1700000001, 123000000).In(time.FixedZone("fixture", 8*3600))
	settled := created.Add(time.Second)
	return games.GameRound{
		ID: "00000000-0000-4000-8000-000000000002", Game: "dice", State: "SETTLED",
		Input:      games.CreateInput{Type: "DICE", Wager: "10", Choice: "BIG"},
		StakeUnits: 5000000, PayoutUnits: 10000000, NetUnits: 5000000, Outcome: games.Win,
		BalanceBeforeUnits: 50000000, BalanceAfterUnits: 55000000,
		WagerTransactionID: "private-wager", SettlementTransactionID: "private-settlement",
		Ruleset: "dice-rules-v2", CommitmentID: testCommitment,
		CreatedAt: created, SettledAt: &settled,
		Dice: &games.DiceResult{Dice: [3]uint8{4, 5, 6}, Total: 15, Side: games.Big, Choice: games.Big,
			Reward: games.Reward{CostMultiplier: 1, PayoutMultiplier: 2, Outcome: games.Win}},
	}
}

func newFixture(t *testing.T) (*Service, *fakeGames, *fakeResolver, *time.Time, *[]string) {
	t.Helper()
	order := []string{}
	now := time.Unix(1700000000, 0)
	r := &fakeResolver{user: 424242, order: &order}
	g := &fakeGames{order: &order, created: fixtureRound(), bootstrap: games.Bootstrap{
		Game:        games.CatalogEntry{Slug: "dice", State: "PLAY", Config: &games.ConfigSummary{Ruleset: "dice-rules-v2"}},
		EntryAction: "PLAY", AvailableUnits: 50000000,
		WagerPolicy:    games.Policy{Minimum: 5000000, MaximumMode: "NONE", Step: 500000},
		EconomicPolicy: &platform.EconomicPolicy{Version: "economy-cap-v1", SinglePlayerMaxUnits: 500000000000},
		Next:           &games.Commitment{ID: testCommitment, Ruleset: "dice-rules-v2", ServerSeedHash: strings.Repeat("a", 64)},
	}}
	s, err := NewService(g, r, testKey, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return s, g, r, &now, &order
}

func prepareQuote(t *testing.T, s *Service) string {
	t.Helper()
	p, err := s.Prepare(context.Background(), testSubject, PrepareInput{RequestID: testRequest, Wager: "10", Choice: "BIG"})
	if err != nil {
		t.Fatal(err)
	}
	return p.Quote
}

func requireFault(t *testing.T, err error, code string) {
	t.Helper()
	var f Fault
	if !errors.As(err, &f) || f.Code != code || err.Error() != code {
		t.Fatalf("fault = %v, want %s", err, code)
	}
}

func TestPrepare(t *testing.T) {
	// A missing identity check, unexpected debit, or quote/limit leak must fail here.
	s, g, r, _, order := newFixture(t)
	p, err := s.Prepare(context.Background(), testSubject, PrepareInput{testRequest, "10", "BIG"})
	if err != nil {
		t.Fatal(err)
	}
	if g.createCalls != 0 || g.findCalls != 0 || g.bootstrapCalls != 1 || r.calls != 1 {
		t.Fatalf("calls: %+v / %+v", g, r)
	}
	if !reflect.DeepEqual(*order, []string{"resolve", "bootstrap"}) || g.user != 424242 || g.slug != "dice" || r.subject != testSubject {
		t.Fatal(*order, g, r)
	}
	if p.Wager != "10" || p.Choice != "BIG" || p.AvailableUnits != "50000000" || p.MinimumWagerUnits != "5000000" || p.MaximumWagerUnits != "50000000" || p.ExpiresAt != "1700000120" {
		t.Fatal(p)
	}
	if p.CommitmentID != testCommitment || p.ServerSeedHash != strings.Repeat("a", 64) || p.Ruleset != "dice-rules-v2" || !strings.Contains(p.RulesText, "豹子退回本金") {
		t.Fatal(p)
	}
	parts := strings.Split(p.Quote, ".")
	if len(parts) != 2 || len(p.Quote) > 2048 {
		t.Fatal("invalid quote envelope")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["iat"] != float64(1700000000) || payload["exp"] != float64(1700000120) || payload["request_id"] != testRequest || payload["subject"] != testSubject || payload["commitment_id"] != testCommitment {
		t.Fatal(payload)
	}
	// Independent HMAC-SHA256 vector (not generated by the production signer).
	if payload["binding"] != "50e7c966895932690f01a617c0ee2980ce850e322814489be5cb8f95c8bf2f98" {
		t.Fatal("binding fingerprint is not the domain-separated HMAC", payload["binding"])
	}
	if strings.Contains(string(raw), "424242") || strings.Contains(string(raw), "native_id") {
		t.Fatalf("native identity leaked: %s", raw)
	}
}

func TestPrepareRejectsNoncanonicalInput(t *testing.T) {
	for _, tc := range []struct{ name, subject, request, wager, choice string }{
		{"empty-subject", "", testRequest, "10", "BIG"}, {"zero-subject", "0", testRequest, "10", "BIG"},
		{"subject-overflow", "18446744073709551616", testRequest, "10", "BIG"},
		{"leading-subject", "01", testRequest, "10", "BIG"}, {"zero-request", testSubject, "0", "10", "BIG"},
		{"leading-request", testSubject, "01", "10", "BIG"}, {"request-overflow", testSubject, "18446744073709551616", "10", "BIG"},
		{"blank", testSubject, testRequest, "", "BIG"}, {"zero", testSubject, testRequest, "0", "BIG"},
		{"leading-zero", testSubject, testRequest, "010", "BIG"}, {"sign", testSubject, testRequest, "+10", "BIG"},
		{"space", testSubject, testRequest, "10 ", "BIG"}, {"fraction", testSubject, testRequest, "10.0", "BIG"},
		{"exponent", testSubject, testRequest, "1e1", "BIG"}, {"unicode", testSubject, testRequest, "１０", "BIG"},
		{"int64-overflow", testSubject, testRequest, "9223372036854775808", "BIG"},
		{"payout-overflow", testSubject, testRequest, "9223372036855", "BIG"},
		{"choice", testSubject, testRequest, "10", "big"}, {"triple", testSubject, testRequest, "10", "TRIPLE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, g, r, _, _ := newFixture(t)
			_, err := s.Prepare(context.Background(), tc.subject, PrepareInput{tc.request, tc.wager, tc.choice})
			requireFault(t, err, "INVALID_REQUEST")
			if g.createCalls != 0 || g.bootstrapCalls != 0 || r.calls != 0 {
				t.Fatal("invalid request reached dependency")
			}
		})
	}
}

func TestPrepareAdmissionAndLimits(t *testing.T) {
	// Each row catches one wrong admission or amount-bound branch; no game is created.
	for _, tc := range []struct {
		name                 string
		change               func(*fakeGames, *fakeResolver)
		wager, want, maximum string
	}{
		{"resolver-first", func(g *fakeGames, r *fakeResolver) { r.err = Fault{Code: "NOT_LINKED"} }, "10", "NOT_LINKED", ""},
		{"invalid-resolved-id", func(g *fakeGames, r *fakeResolver) { r.user = 0 }, "10", "UPSTREAM_UNAVAILABLE", ""},
		{"maintenance", func(g *fakeGames, r *fakeResolver) { g.bootstrap.EntryAction = "MAINTENANCE" }, "10", "MAINTENANCE", ""},
		{"non-play", func(g *fakeGames, r *fakeResolver) { g.bootstrap.EntryAction = "READ_ONLY" }, "10", "UPSTREAM_UNAVAILABLE", ""},
		{"missing-next", func(g *fakeGames, r *fakeResolver) { g.bootstrap.Next = nil }, "10", "COMMITMENT_INVALID", ""},
		{"empty-commitment", func(g *fakeGames, r *fakeResolver) { g.bootstrap.Next.ID = "" }, "10", "COMMITMENT_INVALID", ""},
		{"unknown-rules", func(g *fakeGames, r *fakeResolver) { g.bootstrap.Next.Ruleset = "dice-rules-v99" }, "10", "UPSTREAM_UNAVAILABLE", ""},
		{"policy-minimum", func(g *fakeGames, r *fakeResolver) { g.bootstrap.WagerPolicy.Minimum = 10000000 }, "10", "INVALID_REQUEST", ""},
		{"low-balance", func(g *fakeGames, r *fakeResolver) { g.bootstrap.AvailableUnits = 4999999 }, "10", "INSUFFICIENT_CHIPS", ""},
		{"economic-limit", func(g *fakeGames, r *fakeResolver) { g.bootstrap.EconomicPolicy.SinglePlayerMaxUnits = 5000000 }, "11", "INVALID_REQUEST", ""},
		{"overflow-room", func(g *fakeGames, r *fakeResolver) {
			g.bootstrap.AvailableUnits = math.MaxInt64 - 5000001
			g.bootstrap.EconomicPolicy = nil
		}, "11", "INVALID_REQUEST", ""},
		{"unsupported-policy", func(g *fakeGames, r *fakeResolver) { g.bootstrap.WagerPolicy.MaximumMode = "FUTURE" }, "10", "UPSTREAM_UNAVAILABLE", ""},
		{"bad-step", func(g *fakeGames, r *fakeResolver) { g.bootstrap.WagerPolicy.Step = 0 }, "10", "UPSTREAM_UNAVAILABLE", ""},
		{"economic-max", func(g *fakeGames, r *fakeResolver) { g.bootstrap.EconomicPolicy.SinglePlayerMaxUnits = 20000000 }, "10", "", "20000000"},
		{"integer-balance", func(g *fakeGames, r *fakeResolver) { g.bootstrap.AvailableUnits = 50249999 }, "10", "", "50000000"},
		{"overflow-max", func(g *fakeGames, r *fakeResolver) {
			g.bootstrap.AvailableUnits = math.MaxInt64 - 5000001
			g.bootstrap.EconomicPolicy = nil
		}, "10", "", "5000000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, g, r, _, _ := newFixture(t)
			tc.change(g, r)
			p, err := s.Prepare(context.Background(), testSubject, PrepareInput{testRequest, tc.wager, "BIG"})
			if tc.want != "" {
				requireFault(t, err, tc.want)
			} else if err != nil || p.MaximumWagerUnits != tc.maximum {
				t.Fatal(p, err)
			}
			if g.createCalls != 0 || g.findCalls != 0 || (r.err != nil && g.bootstrapCalls != 0) {
				t.Fatal("unexpected downstream call")
			}
		})
	}
}

func TestPrepareRulesAndIDBoundary(t *testing.T) {
	s, g, _, _, _ := newFixture(t)
	g.bootstrap.Next.Ruleset, g.bootstrap.Game.Config.Ruleset = "dice-rules-v1", "dice-rules-v1"
	p, err := s.Prepare(context.Background(), "18446744073709551615", PrepareInput{"18446744073709551615", "10", "SMALL"})
	if err != nil || p.Ruleset != "dice-rules-v1" || !strings.Contains(p.RulesText, "豹子输") || strings.Contains(p.RulesText, "豹子退回本金") {
		t.Fatal(p, err)
	}
}

func TestPlay(t *testing.T) {
	s, g, r, _, order := newFixture(t)
	q := prepareQuote(t, s)
	*order = nil
	got, err := s.Play(context.Background(), testSubject, q)
	if err != nil || got.RoundID != "00000000-0000-4000-8000-000000000002" {
		t.Fatal(got, err)
	}
	if !reflect.DeepEqual(*order, []string{"resolve", "find", "create"}) || r.calls != 2 || g.bootstrapCalls != 1 || g.createCalls != 1 || g.findCalls != 1 {
		t.Fatal(*order, g, r)
	}
	if g.slug != "dice" || g.user != 424242 || g.key != "discord-dice-v1:970000000000000001" || g.commitment != testCommitment || g.input != (games.CreateInput{Type: "DICE", Wager: "10", Choice: "BIG"}) {
		t.Fatal(g)
	}
}

func TestPlayRevalidatesSubjectBindingAndSignature(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		change     func(*string, *string, *fakeResolver)
	}{
		{"subject", "UNAUTHORIZED", func(q, subject *string, r *fakeResolver) { *subject = "970000000000000003" }},
		{"binding", "BINDING_CHANGED", func(q, subject *string, r *fakeResolver) { r.user++ }},
		{"signature", "UNAUTHORIZED", func(q, subject *string, r *fakeResolver) {
			p := strings.Split(*q, ".")
			p[1] = strings.Repeat("A", 43)
			*q = strings.Join(p, ".")
		}},
		{"new-restriction", "ACCOUNT_RESTRICTED", func(q, subject *string, r *fakeResolver) { r.err = Fault{Code: "ACCOUNT_RESTRICTED"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, g, r, _, _ := newFixture(t)
			q := prepareQuote(t, s)
			subject := testSubject
			tc.change(&q, &subject, r)
			_, err := s.Play(context.Background(), subject, q)
			requireFault(t, err, tc.want)
			if g.createCalls != 0 || g.findCalls != 0 {
				t.Fatal("identity failure reached games")
			}
		})
	}
}

func TestRetry(t *testing.T) {
	for _, tc := range []struct {
		name   string
		found  bool
		mutate func(*games.GameRound)
		want   string
	}{
		{"expired-existing", true, nil, ""}, {"expired-absent", false, nil, "QUOTE_EXPIRED"},
		{"changed-wager", true, func(r *games.GameRound) { r.Input.Wager = "11" }, "IDEMPOTENCY_CONFLICT"},
		{"changed-choice", true, func(r *games.GameRound) { r.Input.Choice = "SMALL" }, "IDEMPOTENCY_CONFLICT"},
		{"changed-input-type", true, func(r *games.GameRound) { r.Input.Type = "SLOT" }, "IDEMPOTENCY_CONFLICT"},
		{"extra-input", true, func(r *games.GameRound) { r.Input.Mode = "unexpected" }, "IDEMPOTENCY_CONFLICT"},
		{"new-quote-commitment", true, func(r *games.GameRound) { r.CommitmentID = "00000000-0000-4000-8000-000000000099" }, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, g, _, now, _ := newFixture(t)
			q := prepareQuote(t, s)
			*now = time.Unix(1700000121, 0)
			if tc.found {
				r := fixtureRound()
				if tc.mutate != nil {
					tc.mutate(&r)
				}
				g.found = &r
			}
			got, err := s.Play(context.Background(), testSubject, q)
			if tc.want != "" {
				requireFault(t, err, tc.want)
			} else if err != nil || got.RoundID != "00000000-0000-4000-8000-000000000002" {
				t.Fatal(got, err)
			}
			if g.createCalls != 0 || g.findCalls != 1 {
				t.Fatal("retry created or skipped lookup")
			}
		})
	}
}

func TestPlayExpiresAtBoundary(t *testing.T) {
	s, g, _, now, _ := newFixture(t)
	q := prepareQuote(t, s)
	*now = time.Unix(1700000120, 0)
	_, err := s.Play(context.Background(), testSubject, q)
	requireFault(t, err, "QUOTE_EXPIRED")
	if g.createCalls != 0 || g.findCalls != 1 {
		t.Fatal("expired quote created")
	}
}

func TestLookup(t *testing.T) {
	for _, tc := range []struct {
		name  string
		found bool
		want  string
	}{
		{"existing", true, ""}, {"missing", false, "NOT_FOUND"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, g, r, now, order := newFixture(t)
			q := prepareQuote(t, s)
			*now = time.Unix(1700000121, 0)
			*order = nil
			if tc.found {
				round := fixtureRound()
				g.found = &round
			}
			got, err := s.Lookup(context.Background(), testSubject, q)
			if tc.want != "" {
				requireFault(t, err, tc.want)
			} else if err != nil || got.RoundID != "00000000-0000-4000-8000-000000000002" {
				t.Fatal(got, err)
			}
			if g.createCalls != 0 || g.findCalls != 1 || r.calls != 2 || !reflect.DeepEqual(*order, []string{"resolve", "find"}) {
				t.Fatal(*order, g, r)
			}
		})
	}
}

func TestErrorsAreStable(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"invalid", games.ErrInvalidInput, "INVALID_REQUEST"}, {"commitment", games.ErrCommitmentInvalid, "COMMITMENT_INVALID"},
		{"conflict", platform.ErrIdempotencyConflict, "IDEMPOTENCY_CONFLICT"}, {"balance", platform.ErrInsufficientBalance, "INSUFFICIENT_CHIPS"},
		{"maintenance", games.ErrMaintenance, "MAINTENANCE"}, {"platform-maintenance", platform.ErrMaintenanceActive, "MAINTENANCE"},
		{"overflow", platform.ErrBalanceOverflow, "INVALID_REQUEST"}, {"unknown", errors.New("private SQL and token"), "UPSTREAM_UNAVAILABLE"},
		{"bad-fault", Fault{Code: "private SQL and token"}, "UPSTREAM_UNAVAILABLE"},
		{"wrapped-fault", fmt.Errorf("private: %w", Fault{Code: "ACCOUNT_NOT_READY"}), "ACCOUNT_NOT_READY"},
		{"pointer-fault", &Fault{Code: "ACCOUNT_RESTRICTED"}, "ACCOUNT_RESTRICTED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, g, _, _, _ := newFixture(t)
			q := prepareQuote(t, s)
			g.createErr = tc.err
			got, err := s.Play(context.Background(), testSubject, q)
			requireFault(t, err, tc.want)
			if got.RoundID != "" || g.createCalls != 1 {
				t.Fatal(got, g.createCalls)
			}
		})
	}
}

func TestLookupFailureNeverCreates(t *testing.T) {
	s, g, _, _, _ := newFixture(t)
	q := prepareQuote(t, s)
	g.findErr = errors.New("connection outcome unknown")
	_, err := s.Play(context.Background(), testSubject, q)
	requireFault(t, err, "UPSTREAM_UNAVAILABLE")
	if g.createCalls != 0 {
		t.Fatal("created after uncertain lookup")
	}
}

func TestNewServiceRequiresDependenciesAndKey(t *testing.T) {
	_, g, r, now, _ := newFixture(t)
	clock := func() time.Time { return *now }
	for _, tc := range []struct {
		name     string
		games    GameService
		resolver Resolver
		key      [32]byte
		clock    func() time.Time
	}{
		{"games", nil, r, testKey, clock}, {"resolver", g, nil, testKey, clock},
		{"key", g, r, [32]byte{}, clock}, {"clock", g, r, testKey, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewService(tc.games, tc.resolver, tc.key, tc.clock)
			requireFault(t, err, "UPSTREAM_UNAVAILABLE")
			if got != nil {
				t.Fatal("invalid service constructed")
			}
		})
	}
}

func TestPrepareInputWireNames(t *testing.T) {
	var input PrepareInput
	if err := json.Unmarshal([]byte(`{"request_id":"970000000000000001","wager":"10","choice":"BIG"}`), &input); err != nil {
		t.Fatal(err)
	}
	if input != (PrepareInput{testRequest, "10", "BIG"}) {
		t.Fatalf("wire input lost a field: %+v", input)
	}
	raw, err := json.Marshal(input)
	if err != nil || string(raw) != `{"request_id":"970000000000000001","wager":"10","choice":"BIG"}` {
		t.Fatalf("wire input = %s, %v", raw, err)
	}
}
