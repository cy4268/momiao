package games

import (
	"context"
	"errors"
	"testing"

	"github.com/cy4268/momiao/internal/platform"
)

type matchingBlackjackFinder interface {
	FindBlackjackActionMatching(context.Context, int64, string, BlackjackActionInput) (*GameRound, error)
}

func TestBlackjackMatchingLookupOriginalConflictAbsent(t *testing.T) {
	owner, runtime := gameTestStores(t)
	s, user, c := blackjackFixture(t, owner, runtime)
	ctx := context.Background()
	r, e := s.Create(ctx, user, "blackjack", requestKey(t), c.ID, CreateInput{Type: "BLACKJACK", InitialWager: "10"})
	if e != nil {
		t.Fatal(e)
	}
	a := BlackjackActionInput{ActionID: requestKey(t), ActionType: "SPLIT", HandID: r.Blackjack.ActiveHandID, ExpectedVersion: "1"}
	original, e := s.BlackjackAction(ctx, user, r.ID, a)
	if e != nil {
		t.Fatal(e)
	}
	// Exercise the prior reconciliation behavior until the additive capability exists.
	// Before the change this returns an original for changed semantics, not a conflict.
	find := func(round string, in BlackjackActionInput) (*GameRound, error) {
		if m, ok := any(s).(matchingBlackjackFinder); ok {
			return m.FindBlackjackActionMatching(ctx, user, round, in)
		}
		return s.FindBlackjackAction(ctx, user, round, in.ActionID)
	}
	changed := a
	changed.ActionType = "HIT"
	if _, e = find(r.ID, changed); !errors.Is(e, platform.ErrIdempotencyConflict) {
		t.Fatalf("read-only lookup must reject reused ID with changed action hash; got %v", e)
	}
	for _, change := range []func(*BlackjackActionInput){func(v *BlackjackActionInput) { v.HandID = requestKey(t) }, func(v *BlackjackActionInput) { v.ExpectedVersion = "2" }} {
		changed = a
		change(&changed)
		if _, e = find(r.ID, changed); !errors.Is(e, platform.ErrIdempotencyConflict) {
			t.Fatalf("matching hash conflict: %v", e)
		}
	}
	if _, e = find(requestKey(t), a); !errors.Is(e, platform.ErrIdempotencyConflict) {
		t.Fatalf("matching original round conflict: %v", e)
	}
	found, e := find(r.ID, a)
	if e != nil || found == nil || found.Blackjack.Version != original.Blackjack.Version || found.StakeUnits != original.StakeUnits {
		t.Fatal("original response lost", e)
	}
	absent := a
	absent.ActionID = requestKey(t)
	found, e = find(r.ID, absent)
	if e != nil || found != nil {
		t.Fatal("absent lookup must not act", e)
	}
	current, e := s.Read(ctx, user, r.ID)
	if e != nil || current.Blackjack.Version != 2 || current.StakeUnits != 10000000 {
		t.Fatal("lookup mutated current round", e)
	}
	legacy, e := s.FindBlackjackAction(ctx, user, requestKey(t), a.ActionID)
	if e != nil || legacy != nil {
		t.Fatal("public lookup compatibility changed", e)
	}
}
