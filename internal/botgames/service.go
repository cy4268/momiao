package botgames

import (
	"context"
	"crypto/hmac"
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
)

type Resolver interface {
	Resolve(context.Context, string) (int64, error)
}
type GameService interface {
	Bootstrap(context.Context, int64, string) (games.Bootstrap, error)
	FindByKey(context.Context, int64, string, string) (*games.GameRound, error)
	Create(context.Context, int64, string, string, string, games.CreateInput) (games.GameRound, error)
}
type PrepareInput struct {
	RequestID string `json:"request_id"`
	Wager     string `json:"wager"`
	Choice    string `json:"choice"`
}
type Fault struct{ Code string }

func (f Fault) Error() string {
	switch f.Code {
	case "INVALID_REQUEST", "UNAUTHORIZED", "BINDING_CHANGED", "ACCOUNT_RESTRICTED",
		"NOT_LINKED", "NOT_FOUND", "ACCOUNT_NOT_READY", "QUOTE_EXPIRED", "COMMITMENT_INVALID",
		"IDEMPOTENCY_CONFLICT", "INSUFFICIENT_CHIPS", "MAINTENANCE", "UPSTREAM_UNAVAILABLE", "SCRATCH_PREVIOUS_REVEAL_INCOMPLETE":
		return f.Code
	default:
		return "UPSTREAM_UNAVAILABLE"
	}
}

type Service struct {
	games    GameService
	resolver Resolver
	key      [32]byte
	now      func() time.Time
}

func NewService(g GameService, r Resolver, key [32]byte, now func() time.Time) (*Service, error) {
	if g == nil || r == nil || key == [32]byte{} || now == nil {
		return nil, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	return &Service{games: g, resolver: r, key: key, now: now}, nil
}

func (s *Service) Prepare(ctx context.Context, subject string, input PrepareInput) (Prepared, error) {
	stake, valid := wagerUnits(input.Wager)
	if !validID(subject) || !validID(input.RequestID) || !valid || !validChoice(input.Choice) {
		return Prepared{}, Fault{Code: "INVALID_REQUEST"}
	}
	user, err := s.resolve(ctx, subject)
	if err != nil {
		return Prepared{}, err
	}
	b, err := s.games.Bootstrap(ctx, user, "dice")
	if err != nil {
		return Prepared{}, mapFault(err)
	}
	if b.EntryAction == "MAINTENANCE" || b.Game.State == "MAINTENANCE" {
		return Prepared{}, Fault{Code: "MAINTENANCE"}
	}
	if b.EntryAction != "PLAY" || b.Game.State != "PLAY" || b.Game.Slug != "dice" {
		return Prepared{}, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	if b.Next == nil || !validUUID(b.Next.ID) || !validHash(b.Next.ServerSeedHash) {
		return Prepared{}, Fault{Code: "COMMITMENT_INVALID"}
	}
	rules, ok := diceRules(b.Next.Ruleset)
	if !ok || b.Game.Config == nil || b.Game.Config.Ruleset != b.Next.Ruleset {
		return Prepared{}, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	minimum, maximum, err := wagerLimits(b)
	if err != nil {
		return Prepared{}, err
	}
	if stake < minimum {
		return Prepared{}, Fault{Code: "INVALID_REQUEST"}
	}
	if stake > b.AvailableUnits {
		return Prepared{}, Fault{Code: "INSUFFICIENT_CHIPS"}
	}
	if stake > maximum {
		return Prepared{}, Fault{Code: "INVALID_REQUEST"}
	}
	iat := s.now().Unix()
	if iat <= 0 || iat > math.MaxInt64-120 {
		return Prepared{}, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	p := quotePayload{Version: 1, RequestID: input.RequestID, Subject: subject,
		Binding: s.binding(subject, user), Wager: input.Wager, Choice: input.Choice,
		CommitmentID: b.Next.ID, IssuedAt: iat, ExpiresAt: iat + 120}
	quote, err := s.signQuote(p)
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{Quote: quote, Wager: input.Wager, Choice: input.Choice,
		AvailableUnits: strconv.FormatInt(b.AvailableUnits, 10), MinimumWagerUnits: strconv.FormatInt(minimum, 10),
		MaximumWagerUnits: strconv.FormatInt(maximum, 10), Ruleset: b.Next.Ruleset, RulesText: rules,
		ServerSeedHash: b.Next.ServerSeedHash, CommitmentID: b.Next.ID, ExpiresAt: strconv.FormatInt(p.ExpiresAt, 10)}, nil
}

func wagerLimits(b games.Bootstrap) (int64, int64, error) {
	p := b.WagerPolicy
	if p.Minimum < 10*games.UnitsPerChip || p.Minimum%games.UnitsPerChip != 0 ||
		p.Step != games.UnitsPerChip || p.MaximumMode != "NONE" || b.AvailableUnits < 0 {
		return 0, 0, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	// Dice may return twice the stake. The game's pre-RNG overflow guard also
	// requires balance + stake <= MaxInt64, even when profit will be capped.
	maximum := min(b.AvailableUnits, math.MaxInt64/2, math.MaxInt64-b.AvailableUnits)
	if e := b.EconomicPolicy; e != nil {
		if e.Version == "" || e.SinglePlayerMaxUnits <= 0 {
			return 0, 0, Fault{Code: "UPSTREAM_UNAVAILABLE"}
		}
		maximum = min(maximum, e.SinglePlayerMaxUnits)
	}
	maximum -= maximum % games.UnitsPerChip
	return p.Minimum, maximum, nil
}

func (s *Service) Play(ctx context.Context, subject, quote string) (Result, error) {
	return s.result(ctx, subject, quote, true)
}

func (s *Service) Lookup(ctx context.Context, subject, quote string) (Result, error) {
	return s.result(ctx, subject, quote, false)
}

func (s *Service) result(ctx context.Context, subject, quote string, create bool) (Result, error) {
	p, err := s.verifyQuote(quote)
	if err != nil {
		return Result{}, err
	}
	if !validID(subject) {
		return Result{}, Fault{Code: "INVALID_REQUEST"}
	}
	if p.Subject != subject {
		return Result{}, Fault{Code: "UNAUTHORIZED"}
	}
	user, err := s.resolve(ctx, subject)
	if err != nil {
		return Result{}, err
	}
	if !hmac.Equal([]byte(p.Binding), []byte(s.binding(subject, user))) {
		return Result{}, Fault{Code: "BINDING_CHANGED"}
	}
	key := "discord-dice-v1:" + p.RequestID
	input := games.CreateInput{Type: "DICE", Wager: p.Wager, Choice: p.Choice}
	round, err := s.games.FindByKey(ctx, user, "dice", key)
	if err != nil {
		return Result{}, mapFault(err)
	}
	if round != nil {
		if round.Input != input {
			return Result{}, Fault{Code: "IDEMPOTENCY_CONFLICT"}
		}
		return projectRound(*round)
	}
	if !create {
		return Result{}, Fault{Code: "NOT_FOUND"}
	}
	if s.now().Unix() >= p.ExpiresAt {
		return Result{}, Fault{Code: "QUOTE_EXPIRED"}
	}
	// The existing game transaction arbitrates concurrent creates and owns all
	// randomness, wallet effects, commitment validity and durable idempotency.
	created, err := s.games.Create(ctx, user, "dice", key, p.CommitmentID, input)
	if err != nil {
		return Result{}, mapFault(err)
	}
	if created.Input != input {
		return Result{}, Fault{Code: "IDEMPOTENCY_CONFLICT"}
	}
	return projectRound(created)
}

func (s *Service) resolve(ctx context.Context, subject string) (int64, error) {
	user, err := s.resolver.Resolve(ctx, subject)
	if err != nil {
		return 0, mapFault(err)
	}
	if user <= 0 {
		return 0, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	return user, nil
}

func mapFault(err error) Fault {
	var f Fault
	if errors.As(err, &f) {
		return Fault{Code: f.Error()}
	}
	var pointer *Fault
	if errors.As(err, &pointer) && pointer != nil {
		return Fault{Code: pointer.Error()}
	}
	switch {
	case errors.Is(err, games.ErrInvalidInput), errors.Is(err, platform.ErrBalanceOverflow):
		return Fault{Code: "INVALID_REQUEST"}
	case errors.Is(err, games.ErrCommitmentInvalid):
		return Fault{Code: "COMMITMENT_INVALID"}
	case errors.Is(err, games.ErrScratchIncomplete):
		return Fault{Code: "SCRATCH_PREVIOUS_REVEAL_INCOMPLETE"}
	case errors.Is(err, platform.ErrIdempotencyConflict):
		return Fault{Code: "IDEMPOTENCY_CONFLICT"}
	case errors.Is(err, platform.ErrInsufficientBalance):
		return Fault{Code: "INSUFFICIENT_CHIPS"}
	case errors.Is(err, games.ErrMaintenance), errors.Is(err, platform.ErrMaintenanceActive):
		return Fault{Code: "MAINTENANCE"}
	case errors.Is(err, games.ErrNotFound):
		return Fault{Code: "NOT_FOUND"}
	case errors.Is(err, platform.ErrWalletNotFound), errors.Is(err, platform.ErrIncompleteWallets):
		return Fault{Code: "ACCOUNT_NOT_READY"}
	default:
		return Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
}
