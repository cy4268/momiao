package botroulette

import (
	"context"
	"crypto/hmac"
	"errors"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/botgames"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/roulette"
)

type Service struct {
	engine   Engine
	resolver botgames.Resolver
	key      [32]byte
	now      func() time.Time
}

func NewService(engine Engine, resolver botgames.Resolver, key [32]byte, now func() time.Time) (*Service, error) {
	if engine == nil || resolver == nil || key == [32]byte{} || now == nil {
		return nil, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	return &Service{engine: engine, resolver: resolver, key: key, now: now}, nil
}
func (s *Service) resolve(ctx context.Context, subject string) (int64, error) {
	if !validID(subject) {
		return 0, Fault{Code: "INVALID_REQUEST"}
	}
	user, err := s.resolver.Resolve(ctx, subject)
	if err != nil {
		return 0, mapFault(err)
	}
	if user <= 0 {
		return 0, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	return user, nil
}
func (s *Service) Lobby(ctx context.Context, subject string, in LobbyInput) (LobbyReply, error) {
	if !roulette.IsGame(in.Game) || (in.Cursor != nil && (len(*in.Cursor) == 0 || len(*in.Cursor) > 256)) {
		return LobbyReply{}, Fault{Code: "INVALID_REQUEST"}
	}
	user, err := s.resolve(ctx, subject)
	if err != nil {
		return LobbyReply{}, err
	}
	q := roulette.LobbyQuery{Game: in.Game}
	if in.Cursor != nil {
		q.Cursor = *in.Cursor
	}
	v, err := s.engine.List(ctx, user, q)
	if err != nil {
		return LobbyReply{}, mapFault(err)
	}
	return projectLobby(v), nil
}
func (s *Service) Public(ctx context.Context, in RoomInput) (PublicReply, error) {
	if !roulette.IsGame(in.Game) || !roulette.ValidRoundID(in.RoomID) {
		return PublicReply{}, Fault{Code: "INVALID_REQUEST"}
	}
	v, err := s.engine.PublicView(ctx, in.RoomID)
	if err != nil {
		return PublicReply{}, mapFault(err)
	}
	if v.ID != in.RoomID || v.Game != in.Game {
		return PublicReply{}, Fault{Code: "INVALID_REQUEST"}
	}
	return PublicReply{SchemaVersion: "1", Room: projectPublic(v)}, nil
}
func (s *Service) State(ctx context.Context, subject string, in StateInput) (StateReply, error) {
	if !roulette.ValidRoundID(in.RoomID) {
		return StateReply{}, Fault{Code: "INVALID_REQUEST"}
	}
	user, err := s.resolve(ctx, subject)
	if err != nil {
		return StateReply{}, err
	}
	v, err := s.engine.View(ctx, user, in.RoomID)
	if err != nil {
		return StateReply{}, mapFault(err)
	}
	if v.ID != in.RoomID || !roulette.IsGame(v.Game) {
		return StateReply{}, Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	// List's own seat is global across both games, not inferred from this room's
	// Self (which can remain visible after a participant's seat is released).
	lobby, err := s.engine.List(ctx, user, roulette.LobbyQuery{Game: v.Game})
	if err != nil {
		return StateReply{}, mapFault(err)
	}
	return projectState(v, lobby.OwnRoundID), nil
}
func (s *Service) Prepare(ctx context.Context, subject string, in PrepareInput) (PreparedReply, error) {
	if !validIntent(in) {
		return PreparedReply{}, Fault{Code: "INVALID_REQUEST"}
	}
	user, err := s.resolve(ctx, subject)
	if err != nil {
		return PreparedReply{}, err
	}
	iat, exp := quoteTimes(in.RequestID)
	if s.now().UnixMilli() < iat {
		return PreparedReply{}, Fault{Code: "INVALID_REQUEST"}
	}
	if s.now().UnixMilli() >= exp {
		return PreparedReply{}, Fault{Code: "QUOTE_EXPIRED"}
	}
	if in.Purpose == "CREATE" {
		err = s.checkCreate(ctx, user, in)
	} else {
		err = s.checkCommand(ctx, user, in)
	}
	if err != nil {
		return PreparedReply{}, err
	}
	// A slow read does not authorize issuing an already expired preview.
	if s.now().UnixMilli() >= exp {
		return PreparedReply{}, Fault{Code: "QUOTE_EXPIRED"}
	}
	p := quotePayload{Version: 1, Subject: subject, Binding: s.binding(subject, user), RequestID: in.RequestID, Purpose: in.Purpose, Game: in.Game, RoomID: in.RoomID, Input: in.Input, IssuedAt: strconv.FormatInt(iat, 10), ExpiresAt: strconv.FormatInt(exp, 10)}
	quote, err := s.signQuote(p)
	if err != nil {
		return PreparedReply{}, err
	}
	return PreparedReply{SchemaVersion: "1", RequestID: p.RequestID, Purpose: p.Purpose, Game: p.Game, RoomID: p.RoomID, Input: p.Input, Quote: quote, BindingFingerprint: p.Binding, IssuedAt: p.IssuedAt, ExpiresAt: p.ExpiresAt}, nil
}
func (s *Service) checkCreate(ctx context.Context, user int64, in PrepareInput) error {
	lobby, err := s.engine.List(ctx, user, roulette.LobbyQuery{Game: in.Game})
	if err != nil {
		return mapFault(err)
	}
	if lobby.Game != in.Game || lobby.MinimumUnits <= 0 || lobby.StepUnits <= 0 {
		return Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	if lobby.OwnRoundID != nil {
		return Fault{Code: "ROULETTE_ALREADY_SEATED"}
	}
	if lobby.State == "MAINTENANCE" {
		return Fault{Code: "MAINTENANCE"}
	}
	if lobby.State != "PLAY" {
		return Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	amount, _ := platform.ParseAmount(in.Input.Create.Stake)
	if amount < lobby.MinimumUnits || amount%lobby.StepUnits != 0 {
		return Fault{Code: "INVALID_REQUEST"}
	}
	// Creating a room is free. Available balance is checked by the engine only
	// for a later confirmed READY, not turned into a debit or create limit here.
	return nil
}
func (s *Service) checkCommand(ctx context.Context, user int64, in PrepareInput) error {
	v, err := s.engine.View(ctx, user, *in.RoomID)
	if err != nil {
		return mapFault(err)
	}
	if v.Game != in.Game || v.ID != *in.RoomID {
		return Fault{Code: "INVALID_REQUEST"}
	}
	if v.State == "NEEDS_REVIEW" {
		return Fault{Code: "ROULETTE_NEEDS_REVIEW"}
	}
	c := in.Input.Command
	if v.Version != c.ExpectedVersion {
		return Fault{Code: "ROULETTE_VERSION_CONFLICT"}
	}
	allowed := false
	for _, a := range v.Actions {
		if equalAction(a, domainAction(c.Action)) {
			allowed = true
			break
		}
	}
	if !allowed {
		return Fault{Code: "ROULETTE_ACTION_INVALID"}
	}
	if r := c.Ready; r != nil && (r.ConfigHash != v.Binding.ConfigHash || r.PolicyHash != v.Binding.PolicyHash || r.ServerSeedHash != v.ServerSeedHash || r.StakeUnits != v.StakeUnits) {
		return Fault{Code: "ROULETTE_VERSION_CONFLICT"}
	}
	return nil
}
func equalAction(a, b roulette.Action) bool {
	return a.Kind == b.Kind && a.Target == b.Target && a.Item == b.Item && a.StolenItem == b.StolenItem && ((a.Agree == nil && b.Agree == nil) || (a.Agree != nil && b.Agree != nil && *a.Agree == *b.Agree))
}
func (s *Service) authenticate(ctx context.Context, subject, quote string) (quotePayload, int64, error) {
	p, err := s.verifyQuote(quote)
	if err != nil {
		return p, 0, err
	}
	if !validID(subject) {
		return p, 0, Fault{Code: "INVALID_REQUEST"}
	}
	if p.Subject != subject {
		return p, 0, Fault{Code: "UNAUTHORIZED"}
	}
	user, err := s.resolve(ctx, subject)
	if err != nil {
		return p, 0, err
	}
	if !hmac.Equal([]byte(p.Binding), []byte(s.binding(subject, user))) {
		return p, 0, Fault{Code: "BINDING_CHANGED"}
	}
	return p, user, nil
}
func intent(p quotePayload) roulette.BotIntent {
	key := "discord-roulette-v1:" + p.Purpose + ":" + p.RequestID
	out := roulette.BotIntent{Purpose: p.Purpose, Game: p.Game}
	if c := p.Input.Create; c != nil {
		out.Create = &roulette.CreateRequest{Key: key, Game: p.Game, Stake: c.Stake, Players: c.Players}
	} else {
		c := p.Input.Command
		out.RoundID = *p.RoomID
		out.Command = &roulette.Command{Key: key, ExpectedVersion: c.ExpectedVersion, Action: domainAction(c.Action)}
		if r := c.Ready; r != nil {
			out.Command.Ready = &roulette.ReadyConfirmation{ClientSeed: r.ClientSeed, ConfigHash: r.ConfigHash, PolicyHash: r.PolicyHash, ServerSeedHash: r.ServerSeedHash, StakeUnits: strconv.FormatInt(r.StakeUnits, 10)}
		}
	}
	return out
}
func (s *Service) Commit(ctx context.Context, subject, quote string) (ReceiptReply, error) {
	p, user, err := s.authenticate(ctx, subject, quote)
	if err != nil {
		return ReceiptReply{}, err
	}
	in := intent(p)
	_, exp := quoteTimes(p.RequestID)
	deadline := time.UnixMilli(exp)
	var r roulette.Receipt
	// Both guarded methods read the original receipt before their locked DB-time
	// expiry check. A locally expired quote must still recover that receipt.
	if in.Create != nil {
		r, err = s.engine.CreateBefore(ctx, user, *in.Create, deadline)
	} else {
		r, err = s.engine.CommandBefore(ctx, user, in.RoundID, *in.Command, deadline)
	}
	if err != nil {
		return ReceiptReply{}, mapFault(err)
	}
	return receiptReply(p.RequestID, "APPLIED", &r), nil
}
func (s *Service) Lookup(ctx context.Context, subject, quote string) (ReceiptReply, error) {
	p, user, err := s.authenticate(ctx, subject, quote)
	if err != nil {
		return ReceiptReply{}, err
	}
	_, exp := quoteTimes(p.RequestID)
	result, err := s.engine.FindReceiptMatching(ctx, user, intent(p), time.UnixMilli(exp))
	if err != nil {
		f := mapFault(err)
		if f.Code != "UPSTREAM_UNAVAILABLE" {
			return ReceiptReply{}, f
		}
		return receiptReply(p.RequestID, "UNKNOWN", nil), nil
	}
	switch result.Status {
	case "APPLIED":
		if result.Receipt != nil {
			return receiptReply(p.RequestID, result.Status, result.Receipt), nil
		}
	case "UNKNOWN", "ABSENT_FINAL":
		if result.Receipt == nil {
			return receiptReply(p.RequestID, result.Status, nil), nil
		}
	}
	return receiptReply(p.RequestID, "UNKNOWN", nil), nil
}
func mapFault(err error) Fault {
	var f Fault
	if errors.As(err, &f) {
		return Fault{Code: f.Error()}
	}
	var fp *Fault
	if errors.As(err, &fp) && fp != nil {
		return Fault{Code: fp.Error()}
	}
	var b botgames.Fault
	if errors.As(err, &b) {
		return Fault{Code: (Fault{Code: b.Code}).Error()}
	}
	var bp *botgames.Fault
	if errors.As(err, &bp) && bp != nil {
		return Fault{Code: (Fault{Code: bp.Code}).Error()}
	}
	switch {
	case errors.Is(err, roulette.ErrInvalidInput), errors.Is(err, platform.ErrBalanceOverflow):
		return Fault{Code: "INVALID_REQUEST"}
	case errors.Is(err, roulette.ErrVersionConflict):
		return Fault{Code: "ROULETTE_VERSION_CONFLICT"}
	case errors.Is(err, roulette.ErrActionInvalid):
		return Fault{Code: "ROULETTE_ACTION_INVALID"}
	case errors.Is(err, roulette.ErrAlreadySeated):
		return Fault{Code: "ROULETTE_ALREADY_SEATED"}
	case errors.Is(err, roulette.ErrIdempotencyConflict), errors.Is(err, platform.ErrIdempotencyConflict):
		return Fault{Code: "IDEMPOTENCY_CONFLICT"}
	case errors.Is(err, roulette.ErrNotFound):
		return Fault{Code: "NOT_FOUND"}
	case errors.Is(err, roulette.ErrQuoteExpired):
		return Fault{Code: "QUOTE_EXPIRED"}
	case errors.Is(err, platform.ErrInsufficientBalance):
		return Fault{Code: "INSUFFICIENT_CHIPS"}
	case errors.Is(err, platform.ErrWalletNotFound), errors.Is(err, platform.ErrIncompleteWallets):
		return Fault{Code: "ACCOUNT_NOT_READY"}
	case errors.Is(err, platform.ErrMaintenanceActive):
		return Fault{Code: "MAINTENANCE"}
	default:
		return Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
}
