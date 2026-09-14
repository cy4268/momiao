package poker

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/cy4268/momiao/internal/poker/engine"
)

// Focused regression for IS 476 / AC-07-37. The service commits a real action;
// its returned ACK is deliberately not used by the reconnecting caller. No
// claim is made that this service-only fixture simulates actual WS delivery.
func TestControlCommittedActionRetryAfterTakeOver(t *testing.T) {
	f, table, refs := playableFixture(t)
	ctx := context.Background()
	v, err := f.service.View(ctx, table, 910001)
	if err != nil {
		t.Fatal(err)
	}
	old := refs[v.Hand.ActorSeat]
	v, err = f.service.ViewForConnection(ctx, old)
	if err != nil {
		t.Fatal(err)
	}
	tv, _ := strconv.ParseUint(v.TableVersion, 10, 64)
	hv, _ := strconv.ParseUint(v.Hand.HandVersion, 10, 64)
	original := ActCommand{UserID: old.UserID, Key: "committed-reconnect-action-01", TableID: table, HandID: v.Hand.HandID, ControlEpoch: old.ControlEpoch, HandVersion: hv, Kind: engine.Call, Control: &old, TableVersion: tv}
	committed, err := f.service.Act(ctx, original)
	if err != nil || committed.Duplicate {
		t.Fatal("original real action did not commit", err)
	}
	kh, _ := hashes(original.Key, original)
	var originalHash, originalBody []byte
	if err = f.owner.QueryRow(ctx, `SELECT request_hash,response FROM poker.request_receipts WHERE newapi_user_id=$1 AND scope='poker.action.v1' AND key_hash=$2`, old.UserID, kh[:]).Scan(&originalHash, &originalBody); err != nil {
		t.Fatal(err)
	}
	// The prior TCP may still be registered when a reconnect arrives. Its new
	// CLAIM_CONTROL socket must explicitly take over; no test owner shortcut.
	incoming := connected(t, f.service, fixtureRef(table, old.UserID), true)
	if incoming.Control.Mode != "READ_ONLY" {
		t.Fatal("reconnect unexpectedly controlled old socket")
	}
	took, err := f.service.TakeOver(ctx, TakeOverCommand{RequestID: "committed-reconnect-takeover", SessionID: old.SessionID, TargetConnectionID: incoming.Ref.ConnectionID, Auth: incoming.Ref.auth()})
	if err != nil || took.Control.Mode != "CONTROLLER" {
		t.Fatal("explicit takeover failed", err)
	}
	if err = f.service.AuthorizeControl(ctx, took.Ref); err != nil {
		t.Fatal("new exact owner is not authorized", err)
	}
	retry := original
	retry.Control = &took.Ref
	retry.ControlEpoch = took.Ref.ControlEpoch
	// Keep original action ID, hand, kind, target and even original version
	// guards. Only reconnect/control authority changes in this reproduction.
	duplicated, retryErr := f.service.Act(ctx, retry)
	var receipts int
	var stored, body []byte
	if err = f.owner.QueryRow(ctx, `SELECT count(*) FROM poker.request_receipts WHERE newapi_user_id=$1 AND scope='poker.action.v1' AND key_hash=$2`, old.UserID, kh[:]).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if err = f.owner.QueryRow(ctx, `SELECT request_hash,response FROM poker.request_receipts WHERE newapi_user_id=$1 AND scope='poker.action.v1' AND key_hash=$2`, old.UserID, kh[:]).Scan(&stored, &body); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 || !equal(stored, originalHash) || !equal(body, originalBody) {
		t.Fatal("retry changed the already committed receipt")
	}
	t.Log("actual new controller authorized; original action fields retained; exactly one unchanged durable action receipt")
	if retryErr != nil || !duplicated.Duplicate || duplicated.Version != committed.Version || duplicated.Status != committed.Status {
		t.Fatalf("reconnect of committed action must return original duplicate receipt; got error=%v duplicate=%v", retryErr, duplicated.Duplicate)
	}
	if _, err = f.service.Act(ctx, original); !errors.Is(err, ErrNoControl) {
		t.Fatal("old controller used known receipt as authorization", err)
	}
	current, err := f.service.ViewForConnection(ctx, took.Ref)
	if err != nil {
		t.Fatal(err)
	}
	retry.TableVersion, _ = strconv.ParseUint(current.TableVersion, 10, 64)
	retry.HandVersion, _ = strconv.ParseUint(current.Hand.HandVersion, 10, 64)
	if again, e := f.service.Act(ctx, retry); e != nil || !again.Duplicate || again.Version != committed.Version {
		t.Fatal("fresh guards changed committed intent", again, e)
	}
	for _, kind := range []string{"hand", "kind", "amount"} {
		changed := retry
		switch kind {
		case "hand":
			changed.HandID = uuid()
		case "kind":
			changed.Kind = engine.Raise
		case "amount":
			changed.ToUnits = engine.UnitsPerChip
		}
		if _, e := f.service.Act(ctx, changed); !errors.Is(e, ErrConflict) {
			t.Fatal("same ID changed "+kind+" without conflict", e)
		}
	}
	var calls int
	var applied int64
	if err = f.owner.QueryRow(ctx, `SELECT count(*),coalesce(sum(applied_delta_units),0) FROM poker.actions WHERE hand_id=$1 AND event_type='CALL'`, original.HandID).Scan(&calls, &applied); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || applied != 5*engine.UnitsPerChip {
		t.Fatal("committed action applied more than once", calls, applied)
	}
	refs[v.Hand.ActorSeat] = took.Ref
	foldFromSocket(t, f, table, refs)
	if _, err = f.service.RequestSafeLeave(ctx, SessionCommand{UserID: old.UserID, TableID: table, Key: "intent-old-session-leave", TargetSessionID: old.SessionID}); err != nil {
		t.Fatal(err)
	}
	newSession := f.buy(t, table, old.UserID, v.Hand.ActorSeat)
	if newSession == old.SessionID {
		t.Fatal("session boundary not exercised")
	}
	newOwner := connected(t, f.service, fixtureRef(table, old.UserID), true).Ref
	retry.Control = &newOwner
	retry.ControlEpoch = newOwner.ControlEpoch
	if _, err = f.service.Act(ctx, retry); !errors.Is(err, ErrConflict) {
		t.Fatal("same ID crossed actual Poker Session", err)
	}
}

func TestControlSessionAndSeedRetryAfterTakeOver(t *testing.T) {
	f := newControlFixture(t)
	table := f.table(t)
	session := f.buy(t, table, 910001, 1)
	ctx := context.Background()
	old := connected(t, f.service, fixtureRef(table, 910001), true).Ref
	sit := controlledSession(t, f, old)
	sit.Key = "intent-sitout-reconnect"
	sat, e := f.service.RequestSitOut(ctx, sit)
	if e != nil {
		t.Fatal(e)
	}
	seed := NextSeedCommand{SessionCommand: controlledSession(t, f, old), Contribution: "unchanged synthetic contribution"}
	seed.Key = "intent-nextseed-reconnect"
	seeded, e := f.service.SetNextSeed(ctx, seed)
	if e != nil {
		t.Fatal(e)
	}
	resume := controlledSession(t, f, old)
	resume.Key = "intent-resume-reconnect"
	resumed, e := f.service.ResumeSeat(ctx, resume)
	if e != nil {
		t.Fatal(e)
	}
	var contributionVersion int64
	if e = f.owner.QueryRow(ctx, "SELECT contribution_version FROM poker.seats WHERE table_id=$1 AND seat_no=1", table).Scan(&contributionVersion); e != nil {
		t.Fatal(e)
	}
	next := connected(t, f.service, fixtureRef(table, 910001), true)
	took, e := f.service.TakeOver(ctx, TakeOverCommand{RequestID: "intent-session-takeover", SessionID: session, TargetConnectionID: next.Ref.ConnectionID, Auth: next.Ref.auth()})
	if e != nil || took.Control.Mode != "CONTROLLER" {
		t.Fatal(e)
	}
	if _, e = f.service.RequestSitOut(ctx, sit); !errors.Is(e, ErrNoControl) {
		t.Fatal("old sitout controller accepted", e)
	}
	if _, e = f.service.SetNextSeed(ctx, seed); !errors.Is(e, ErrNoControl) {
		t.Fatal("old seed controller accepted", e)
	}
	sit.Control = &took.Ref
	resume.Control = &took.Ref
	seed.Control = &took.Ref
	for _, entry := range []struct {
		command  SessionCommand
		original Receipt
		run      func(context.Context, SessionCommand) (Receipt, error)
	}{{sit, sat, f.service.RequestSitOut}, {resume, resumed, f.service.ResumeSeat}} {
		r, err := entry.run(ctx, entry.command)
		if err != nil || !r.Duplicate || r.Version != entry.original.Version || r.Status != entry.original.Status {
			t.Fatal("session retry lost original receipt", r, err)
		}
	}
	if r, err := f.service.SetNextSeed(ctx, seed); err != nil || !r.Duplicate || r.Version != seeded.Version {
		t.Fatal("seed retry lost original receipt", r, err)
	}
	changedSeed := seed
	changedSeed.Contribution = "changed synthetic contribution"
	if _, e = f.service.SetNextSeed(ctx, changedSeed); !errors.Is(e, ErrConflict) {
		t.Fatal("same ID changed seed", e)
	}
	var state string
	var version int64
	if e = f.owner.QueryRow(ctx, "SELECT state,contribution_version FROM poker.seats WHERE table_id=$1 AND seat_no=1", table).Scan(&state, &version); e != nil {
		t.Fatal(e)
	}
	if state != "WAITING_BIG_BLIND" || version != contributionVersion {
		t.Fatal("duplicate replayed historical state or seed", state, version)
	}
	if _, e = f.service.RequestSafeLeave(ctx, SessionCommand{UserID: old.UserID, TableID: table, Key: "intent-session-safe-leave", TargetSessionID: session}); e != nil {
		t.Fatal(e)
	}
	newSession := f.buy(t, table, old.UserID, 1)
	if newSession == session {
		t.Fatal("session unchanged")
	}
	owner := connected(t, f.service, fixtureRef(table, old.UserID), true).Ref
	sit.Control = &owner
	seed.Control = &owner
	if _, e = f.service.RequestSitOut(ctx, sit); !errors.Is(e, ErrConflict) {
		t.Fatal("same sitout ID crossed Session", e)
	}
	if _, e = f.service.SetNextSeed(ctx, seed); !errors.Is(e, ErrConflict) {
		t.Fatal("same seed ID crossed Session", e)
	}
}
