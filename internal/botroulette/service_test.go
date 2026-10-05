package botroulette

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/botgames"
	"github.com/cy4268/momiao/internal/games/fairness"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/roulette"
)

const roomID = "019923a0-0000-7000-8000-000000000001"
const otherRoomID = "019923a0-0000-7000-8000-000000000002"
const subject = "123456789012345678"

var testTime = time.UnixMilli(1791194400000).UTC()
var testKey = [32]byte{1, 2, 3, 4, 5}

func TestFixtureBindingUsesAuthoritativeFairnessStream(t *testing.T) {
	_, engine, _, _ := fixture(t)
	for _, binding := range []roulette.Binding{engine.view.Binding, engine.public.Binding, engine.lobby.Binding} {
		if binding.Stream != fairness.StreamVersion || projectBinding(binding).Stream != fairness.StreamVersion {
			t.Fatalf("adapter fixture stream=%q; authoritative stream=%q", binding.Stream, fairness.StreamVersion)
		}
	}
}

type fakeResolver struct {
	user     int64
	err      error
	subjects []string
}

func (f *fakeResolver) Resolve(_ context.Context, subject string) (int64, error) {
	f.subjects = append(f.subjects, subject)
	return f.user, f.err
}

// The fake replaces only the database-owning Engine. Calls retain full inputs
// so tests catch writes from read paths and wrong actor/key/deadline forwarding.
type fakeEngine struct {
	lobby                              roulette.Lobby
	view                               roulette.RoomView
	public                             roulette.PublicRoomView
	receipt                            roulette.Receipt
	lookup                             roulette.BotLookup
	err                                error
	lists                              []roulette.LobbyQuery
	users                              []int64
	views, publics, mutations, lookups int
	intent                             roulette.BotIntent
	deadline                           time.Time
	afterRead                          func()
}

var _ Engine = (*fakeEngine)(nil)
var _ Engine = (*roulette.Service)(nil)

func (f *fakeEngine) List(_ context.Context, user int64, in roulette.LobbyQuery) (roulette.Lobby, error) {
	f.users = append(f.users, user)
	f.lists = append(f.lists, in)
	if f.afterRead != nil {
		f.afterRead()
	}
	return f.lobby, f.err
}
func (f *fakeEngine) View(_ context.Context, user int64, id string) (roulette.RoomView, error) {
	f.users = append(f.users, user)
	f.views++
	if f.afterRead != nil {
		f.afterRead()
	}
	if id != roomID {
		return roulette.RoomView{}, roulette.ErrNotFound
	}
	return f.view, f.err
}
func (f *fakeEngine) PublicView(_ context.Context, id string) (roulette.PublicRoomView, error) {
	f.publics++
	if id != roomID {
		return roulette.PublicRoomView{}, roulette.ErrNotFound
	}
	return f.public, f.err
}
func (f *fakeEngine) CreateBefore(_ context.Context, user int64, in roulette.CreateRequest, deadline time.Time) (roulette.Receipt, error) {
	f.users = append(f.users, user)
	f.mutations++
	f.deadline = deadline
	f.intent = roulette.BotIntent{Purpose: "CREATE", Game: in.Game, Create: &in}
	return f.receipt, f.err
}
func (f *fakeEngine) CommandBefore(_ context.Context, user int64, id string, in roulette.Command, deadline time.Time) (roulette.Receipt, error) {
	f.users = append(f.users, user)
	f.mutations++
	f.deadline = deadline
	f.intent = roulette.BotIntent{Purpose: "COMMAND", RoundID: id, Command: &in}
	return f.receipt, f.err
}
func (f *fakeEngine) FindReceiptMatching(_ context.Context, user int64, intent roulette.BotIntent, deadline time.Time) (roulette.BotLookup, error) {
	f.users = append(f.users, user)
	f.lookups++
	f.intent = intent
	f.deadline = deadline
	return f.lookup, f.err
}

func fixture(t *testing.T) (*Service, *fakeEngine, *fakeResolver, *time.Time) {
	t.Helper()
	now := testTime
	b := roulette.Binding{ConfigID: roomID, ConfigHash: strings.Repeat("a", 64), PolicyID: otherRoomID, PolicyHash: strings.Repeat("b", 64), Ruleset: "momiao-devil-rules-v1", Algorithm: "momiao-devil-rng-v1", Stream: fairness.StreamVersion}
	v := roulette.RoomView{ID: roomID, Game: "devil-roulette", Title: "恶魔轮盘", Version: 7, Sequence: 3, State: "WAITING", TargetPlayers: 2, StakeUnits: 5000000, PoolUnits: 5000000, Binding: b, ServerSeedHash: strings.Repeat("c", 64), ServerNow: now, Players: []roulette.PlayerView{{Seat: 0, Name: "旅人", Alive: true}}, Self: &roulette.SelfView{Seat: 0, AvailableUnits: 123000000, Items: []string{"adrenaline"}, Intel: []roulette.IntelEntry{{Index: 1, Live: true}}}, Actions: []roulette.Action{{Kind: "READY"}}, Log: []roulette.PublicEvent{}}
	for i := range 25 {
		v.Log = append(v.Log, roulette.PublicEvent{Sequence: int64(i + 1), At: now, Kind: "PUBLIC", Text: "公开事件"})
	}
	f := &fakeEngine{view: v, public: roulette.PublicRoomView{ID: v.ID, Game: v.Game, Title: v.Title, Version: v.Version, Sequence: v.Sequence, State: v.State, TargetPlayers: v.TargetPlayers, StakeUnits: v.StakeUnits, PoolUnits: v.PoolUnits, Binding: b, ServerSeedHash: v.ServerSeedHash, ServerNow: now, Players: v.Players, Log: v.Log}, receipt: roulette.Receipt{RoundID: roomID, Version: 8, Sequence: 4, State: "PLAYING"}, lookup: roulette.BotLookup{Status: "UNKNOWN"}, lobby: roulette.Lobby{Game: "devil-roulette", State: "PLAY", Binding: b, MinimumUnits: 5000000, StepUnits: 500000, AvailableUnits: 0, Rooms: []roulette.RoomView{v}}}
	r := &fakeResolver{user: 42}
	s, err := NewService(f, r, testKey, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return s, f, r, &now
}
func requestID(at time.Time) string {
	return strconv.FormatUint(uint64(at.UnixMilli()-1420070400000)<<22, 10)
}
func createInput() PrepareInput {
	return PrepareInput{RequestID: requestID(testTime), Purpose: "CREATE", Game: "devil-roulette", Input: TypedInput{Create: &CreateInput{Stake: "10", Players: 2}}}
}
func commandInput(kind string) PrepareInput {
	id := roomID
	return PrepareInput{RequestID: requestID(testTime), Purpose: "COMMAND", Game: "devil-roulette", RoomID: &id, Input: TypedInput{Command: &CommandInput{ExpectedVersion: 7, Action: Action{Kind: kind}}}}
}
func readyInput() PrepareInput {
	in := commandInput("READY")
	in.Input.Command.Ready = &ReadyConfirmation{ClientSeed: "御主的种子", ConfigHash: strings.Repeat("a", 64), PolicyHash: strings.Repeat("b", 64), ServerSeedHash: strings.Repeat("c", 64), StakeUnits: 5000000}
	return in
}
func requireFault(t *testing.T, err error, code string) {
	t.Helper()
	var f Fault
	if !errors.As(err, &f) || f.Code != code || err.Error() != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}
func prepare(t *testing.T, s *Service, in PrepareInput) PreparedReply {
	t.Helper()
	out, err := s.Prepare(context.Background(), subject, in)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPublicDoesNotResolveSubjectAndUsesWhitelist(t *testing.T) {
	s, f, r, _ := fixture(t)
	r.err = errors.New("no personal identity allowed")
	out, err := s.Public(context.Background(), RoomInput{Game: "devil-roulette", RoomID: roomID})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.subjects) != 0 || f.views != 0 || f.publics != 1 || f.mutations != 0 {
		t.Fatal("public used personal or mutation path")
	}
	if len(out.Room.Log) != 20 || out.Room.Log[0].Sequence != 6 {
		t.Fatal("public log not bounded to last 20")
	}
	assertFields(t, out, "schema_version,room")
	assertFields(t, out.Room, "id,game,title,version,sequence,state,target_players,stake_units,pool_units,binding,server_seed_hash,turn_seat,server_now,deadline,game_deadline,players,log,devil,pressure")
	assertFields(t, out.Room.Players[0], "seat,name,ready,alive,forfeited,hp,item_count")
	assertFields(t, out.Room.Binding, "config_version_id,config_hash,wager_policy_version_id,wager_policy_hash,ruleset_version,algorithm_version,fairness_stream_version")
	_, err = s.Public(context.Background(), RoomInput{Game: "pressure-roulette", RoomID: roomID})
	requireFault(t, err, "INVALID_REQUEST")
}

func TestPrivateRoutesResolveEveryCallAndOriginalSeatIsCrossGame(t *testing.T) {
	s, f, r, _ := fixture(t)
	ctx := context.Background()
	f.lobby.OwnRoundID = new(otherRoomID)
	lobby, err := s.Lobby(ctx, subject, LobbyInput{Game: "devil-roulette"})
	if err != nil || lobby.OwnRoundID == nil || *lobby.OwnRoundID != otherRoomID {
		t.Fatalf("lobby: %+v %v", lobby, err)
	}
	assertFields(t, lobby.Rooms[0], "id,game,title,version,state,target_players,stake_units,pool_units,players")
	f.view.Game = "pressure-roulette"
	f.lobby.Game = "pressure-roulette"
	f.view.EconomySettlement = &platform.PayoutCapView{PolicyVersion: "v1", PolicyHash: strings.Repeat("d", 64), GrossPayoutUnits: 17000000, CreditedPayoutUnits: 9100000, WithheldUnits: 7900000, ActualNetUnits: 4100000}
	state, err := s.State(ctx, subject, StateInput{RoomID: roomID})
	if err != nil {
		t.Fatal(err)
	}
	if state.Room.Game != "pressure-roulette" || *state.OwnRoundID != otherRoomID || f.lists[len(f.lists)-1].Game != "pressure-roulette" {
		t.Fatal("state guessed slug or own room")
	}
	if state.EconomySettlement.CreditedPayoutUnits != 9100000 || state.Self.Items[0] != "adrenaline" {
		t.Fatal("private projection changed website evidence")
	}
	assertFields(t, state, "schema_version,room,self,actions,economic_policy,economy_settlement,own_round_id")
	f.lobby.Game = "devil-roulette"
	f.lobby.OwnRoundID = nil
	p := prepare(t, s, createInput())
	if _, err = s.Commit(ctx, subject, p.Quote); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Lookup(ctx, subject, p.Quote); err != nil {
		t.Fatal(err)
	}
	if len(r.subjects) != 5 {
		t.Fatalf("binding calls=%d", len(r.subjects))
	}
	for _, got := range r.subjects {
		if got != subject {
			t.Fatal("wrong subject")
		}
	}
	for _, got := range f.users {
		if got != 42 {
			t.Fatal("caller-supplied actor")
		}
	}
}

func TestPrepareDoesNotCreateOrDebit(t *testing.T) {
	s, f, _, _ := fixture(t)
	// A free create is allowed even with no currently available funds.
	p := prepare(t, s, createInput())
	if p.RoomID != nil || p.Input.Create.Stake != "10" || p.SchemaVersion != "1" {
		t.Fatalf("bad create preview: %+v", p)
	}
	ready := prepare(t, s, readyInput())
	if !reflect.DeepEqual(ready.Input.Command.Ready, readyInput().Input.Command.Ready) {
		t.Fatal("READY confirmation silently changed")
	}
	_, err := s.Lookup(context.Background(), subject, ready.Quote)
	if err != nil {
		t.Fatal(err)
	}
	if f.mutations != 0 {
		t.Fatal("prepare/lookup mutated")
	}
}

func TestPrepareChecksDisplayedVersionCompleteActionAndReadyMaterials(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*fakeEngine, *PrepareInput)
		code string
	}{
		{"version", func(_ *fakeEngine, in *PrepareInput) { in.Input.Command.ExpectedVersion = 6 }, "ROULETTE_VERSION_CONFLICT"},
		{"game", func(_ *fakeEngine, in *PrepareInput) { in.Game = "pressure-roulette" }, "INVALID_REQUEST"},
		{"no-action", func(f *fakeEngine, _ *PrepareInput) { f.view.Actions = nil }, "ROULETTE_ACTION_INVALID"},
		{"review", func(f *fakeEngine, _ *PrepareInput) { f.view.State = "NEEDS_REVIEW" }, "ROULETTE_NEEDS_REVIEW"},
		{"config", func(_ *fakeEngine, in *PrepareInput) { in.Input.Command.Ready.ConfigHash = strings.Repeat("d", 64) }, "ROULETTE_VERSION_CONFLICT"},
		{"policy", func(_ *fakeEngine, in *PrepareInput) { in.Input.Command.Ready.PolicyHash = strings.Repeat("d", 64) }, "ROULETTE_VERSION_CONFLICT"},
		{"commitment", func(_ *fakeEngine, in *PrepareInput) { in.Input.Command.Ready.ServerSeedHash = strings.Repeat("d", 64) }, "ROULETTE_VERSION_CONFLICT"},
		{"stake", func(_ *fakeEngine, in *PrepareInput) { in.Input.Command.Ready.StakeUnits++ }, "ROULETTE_VERSION_CONFLICT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, f, _, _ := fixture(t)
			in := readyInput()
			tc.edit(f, &in)
			_, err := s.Prepare(context.Background(), subject, in)
			requireFault(t, err, tc.code)
			if f.mutations != 0 {
				t.Fatal("invalid preview mutated")
			}
		})
	}
	for _, action := range []roulette.Action{{Kind: "DEVIL_SHOOT", Target: "SELF"}, {Kind: "DEVIL_ITEM", Item: "adrenaline", StolenItem: "beer"}, {Kind: "PRESSURE_VOTE", Agree: new(false)}} {
		s, f, _, _ := fixture(t)
		f.view.Actions = []roulette.Action{action}
		in := commandInput(action.Kind)
		in.Input.Command.Action = Action{Kind: action.Kind, Target: action.Target, Item: action.Item, StolenItem: action.StolenItem, Agree: action.Agree}
		prepare(t, s, in)
		switch action.Kind {
		case "DEVIL_SHOOT":
			in.Input.Command.Action.Target = "OPPONENT"
		case "DEVIL_ITEM":
			in.Input.Command.Action.StolenItem = "saw"
		case "PRESSURE_VOTE":
			in.Input.Command.Action.Agree = new(true)
		}
		_, err := s.Prepare(context.Background(), subject, in)
		requireFault(t, err, "ROULETTE_ACTION_INVALID")
	}
}

func TestCommitUsesGuardedMethodsAndOnlyOriginalReceipt(t *testing.T) {
	for _, kind := range []string{"CREATE", "COMMAND"} {
		t.Run(kind, func(t *testing.T) {
			s, f, _, now := fixture(t)
			in := createInput()
			if kind == "COMMAND" {
				in = readyInput()
			}
			p := prepare(t, s, in)
			beforeViews := f.views
			beforeLists := len(f.lists)
			f.view.Version = 99
			f.view.State = "SETTLED"
			*now = testTime.Add(121 * time.Second)
			out, err := s.Commit(context.Background(), subject, p.Quote)
			if err != nil {
				t.Fatal(err)
			}
			if out.Status != "APPLIED" || out.Receipt.Version != 8 || out.Receipt.State != "PLAYING" || out.RequestID != in.RequestID {
				t.Fatalf("not original receipt: %+v", out)
			}
			if f.views != beforeViews || len(f.lists) != beforeLists || f.lookups != 0 || f.mutations != 1 {
				t.Fatal("commit read current state or bypassed guarded method")
			}
			if !f.deadline.Equal(testTime.Add(120 * time.Second)) {
				t.Fatal("wrong deadline")
			}
			key := "discord-roulette-v1:" + kind + ":" + in.RequestID
			if kind == "CREATE" {
				if f.intent.Create.Key != key || f.intent.Create.Stake != "10" {
					t.Fatal("create semantic changed")
				}
			} else {
				c := f.intent.Command
				if c.Key != key || c.ExpectedVersion != 7 || c.Ready.ClientSeed != "御主的种子" || c.Ready.StakeUnits != "5000000" || f.intent.RoundID != roomID {
					t.Fatal("command semantic changed")
				}
			}
			assertFields(t, out, "schema_version,status,request_id,receipt")
			assertFields(t, out.Receipt, "round_id,version,sequence,state")
		})
	}
}

func TestLookupAfterExpiryKeepsOriginalReceipt(t *testing.T) {
	s, f, _, now := fixture(t)
	p := prepare(t, s, readyInput())
	f.lookup = roulette.BotLookup{Status: "APPLIED", Receipt: &f.receipt}
	*now = testTime.Add(24 * time.Hour)
	f.view.Version = 99
	before := f.views
	out, err := s.Lookup(context.Background(), subject, p.Quote)
	if err != nil {
		t.Fatal(err)
	}
	if out.Receipt.Version != 8 || out.Status != "APPLIED" || f.mutations != 0 || f.views != before {
		t.Fatal("lookup substituted current view or mutated")
	}
	if f.intent.Game != "devil-roulette" || f.intent.Purpose != "COMMAND" || f.intent.Command.ExpectedVersion != 7 || f.intent.RoundID != roomID || !f.deadline.Equal(testTime.Add(120*time.Second)) {
		t.Fatal("lookup lost original typed intent")
	}
}

func TestLookupIndeterminateVersusExplicitFaults(t *testing.T) {
	for _, tc := range []struct {
		name         string
		err          error
		status, code string
	}{
		{"inflight", nil, "UNKNOWN", ""}, {"timeout", context.DeadlineExceeded, "UNKNOWN", ""}, {"cancel", context.Canceled, "UNKNOWN", ""}, {"offline", roulette.ErrUnavailable, "UNKNOWN", ""}, {"io", errors.New("secret connection detail"), "UNKNOWN", ""},
		{"invalid", roulette.ErrInvalidInput, "", "INVALID_REQUEST"}, {"conflict", roulette.ErrIdempotencyConflict, "", "IDEMPOTENCY_CONFLICT"}, {"not-found", roulette.ErrNotFound, "", "NOT_FOUND"}, {"identity", botgames.Fault{Code: "ACCOUNT_RESTRICTED"}, "", "ACCOUNT_RESTRICTED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, f, _, _ := fixture(t)
			p := prepare(t, s, createInput())
			f.err = tc.err
			out, err := s.Lookup(context.Background(), subject, p.Quote)
			if tc.code != "" {
				requireFault(t, err, tc.code)
			} else if err != nil || out.Status != tc.status || out.Receipt != nil {
				t.Fatalf("%+v %v", out, err)
			}
			if f.mutations != 0 {
				t.Fatal("lookup mutated")
			}
		})
	}
	for _, status := range []string{"UNKNOWN", "ABSENT_FINAL"} {
		s, f, _, now := fixture(t)
		p := prepare(t, s, createInput())
		*now = testTime.Add(121 * time.Second)
		f.lookup.Status = status
		out, err := s.Lookup(context.Background(), subject, p.Quote)
		if err != nil || out.Status != status || out.Receipt != nil {
			t.Fatalf("status: %+v %v", out, err)
		}
	}
}

func TestIdentityAndDomainErrorsAreClosedFaults(t *testing.T) {
	for _, code := range []string{"INVALID_REQUEST", "UNAUTHORIZED", "BINDING_CHANGED", "ACCOUNT_RESTRICTED", "NOT_LINKED", "ACCOUNT_NOT_READY", "MAINTENANCE", "UPSTREAM_UNAVAILABLE"} {
		for _, pointer := range []bool{false, true} {
			s, _, r, _ := fixture(t)
			f := botgames.Fault{Code: code}
			r.err = fmt.Errorf("private diagnostic: %w", f)
			if pointer {
				var pointerError error = &f
				r.err = fmt.Errorf("private diagnostic: %w", pointerError)
			}
			_, err := s.Lobby(context.Background(), subject, LobbyInput{Game: "devil-roulette"})
			requireFault(t, err, code)
		}
	}
	for _, tc := range []struct {
		err  error
		code string
	}{{roulette.ErrVersionConflict, "ROULETTE_VERSION_CONFLICT"}, {roulette.ErrActionInvalid, "ROULETTE_ACTION_INVALID"}, {roulette.ErrAlreadySeated, "ROULETTE_ALREADY_SEATED"}, {roulette.ErrQuoteExpired, "QUOTE_EXPIRED"}, {roulette.ErrIdempotencyConflict, "IDEMPOTENCY_CONFLICT"}, {platform.ErrInsufficientBalance, "INSUFFICIENT_CHIPS"}, {platform.ErrWalletNotFound, "ACCOUNT_NOT_READY"}, {platform.ErrMaintenanceActive, "MAINTENANCE"}, {errors.New("native id=42 secret"), "UPSTREAM_UNAVAILABLE"}} {
		s, f, _, _ := fixture(t)
		p := prepare(t, s, createInput())
		f.err = fmt.Errorf("private: %w", tc.err)
		_, err := s.Commit(context.Background(), subject, p.Quote)
		requireFault(t, err, tc.code)
	}
	if (Fault{Code: "native id=42 secret"}).Error() != "UPSTREAM_UNAVAILABLE" {
		t.Fatal("fault leaked diagnostics")
	}
}

func TestCreateValidationUsesWebsitePolicyWithoutDebiting(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*fakeEngine, *PrepareInput)
		code string
	}{
		{"seated", func(f *fakeEngine, _ *PrepareInput) { f.lobby.OwnRoundID = new(otherRoomID) }, "ROULETTE_ALREADY_SEATED"},
		{"maintenance", func(f *fakeEngine, _ *PrepareInput) { f.lobby.State = "MAINTENANCE" }, "MAINTENANCE"},
		{"minimum", func(f *fakeEngine, _ *PrepareInput) { f.lobby.MinimumUnits = 10000000 }, "INVALID_REQUEST"},
		{"step", func(f *fakeEngine, in *PrepareInput) { f.lobby.StepUnits = 1000000; in.Input.Create.Stake = "11" }, "INVALID_REQUEST"},
		{"bad-policy", func(f *fakeEngine, _ *PrepareInput) { f.lobby.StepUnits = 0 }, "UPSTREAM_UNAVAILABLE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, f, _, _ := fixture(t)
			in := createInput()
			tc.edit(f, &in)
			_, err := s.Prepare(context.Background(), subject, in)
			requireFault(t, err, tc.code)
			if f.mutations != 0 {
				t.Fatal("create preview mutated")
			}
		})
	}
}

func assertFields(t *testing.T, value any, names string) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err = json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	want := strings.Split(names, ",")
	if len(object) != len(want) {
		t.Fatalf("wrong fields: %s", raw)
	}
	for _, name := range want {
		if _, ok := object[name]; !ok {
			t.Fatalf("missing %s: %s", name, raw)
		}
	}
}

func TestConstructorAndInvalidIdentityFailClosed(t *testing.T) {
	_, f, r, _ := fixture(t)
	for _, tc := range []struct {
		name string
		e    Engine
		r    botgames.Resolver
		key  [32]byte
		now  func() time.Time
	}{
		{"engine", nil, r, testKey, time.Now}, {"resolver", f, nil, testKey, time.Now}, {"key", f, r, [32]byte{}, time.Now}, {"clock", f, r, testKey, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewService(tc.e, tc.r, tc.key, tc.now)
			requireFault(t, err, "UPSTREAM_UNAVAILABLE")
		})
	}
	for _, actor := range []string{"", "0", "01", "+1", "-1", "18446744073709551616"} {
		s, f, r, _ := fixture(t)
		_, err := s.Lobby(context.Background(), actor, LobbyInput{Game: "devil-roulette"})
		requireFault(t, err, "INVALID_REQUEST")
		if len(r.subjects) != 0 || len(f.users) != 0 {
			t.Fatal("malformed identity reached resolver")
		}
	}
	for _, user := range []int64{0, -1} {
		s, f, r, _ := fixture(t)
		r.user = user
		_, err := s.State(context.Background(), subject, StateInput{RoomID: roomID})
		requireFault(t, err, "UPSTREAM_UNAVAILABLE")
		if len(f.users) != 0 {
			t.Fatal("invalid resolved actor reached engine")
		}
	}
}

func TestCommandRoundTripRetainsEveryActionField(t *testing.T) {
	for _, a := range []Action{
		{Kind: "JOIN"}, {Kind: "LEAVE"}, {Kind: "UNREADY"}, {Kind: "CANCEL"}, {Kind: "SURRENDER"},
		{Kind: "DEVIL_SHOOT", Target: "OPPONENT"}, {Kind: "DEVIL_ITEM", Item: "beer"}, {Kind: "DEVIL_ITEM", Item: "adrenaline", StolenItem: "saw"},
		{Kind: "PRESSURE_FIRE"}, {Kind: "PRESSURE_PASS"}, {Kind: "PRESSURE_AGAIN"}, {Kind: "PRESSURE_CHARGE"}, {Kind: "PRESSURE_UNLOAD"}, {Kind: "PRESSURE_RIPOSTE"}, {Kind: "PRESSURE_VOTE", Agree: new(false)}, {Kind: "PRESSURE_VOTE", Agree: new(true)},
	} {
		name := a.Kind
		if a.Item != "" {
			name += "/" + a.Item
		}
		if a.Agree != nil {
			name += "/" + strconv.FormatBool(*a.Agree)
		}
		t.Run(name, func(t *testing.T) {
			s, f, _, _ := fixture(t)
			in := commandInput(a.Kind)
			in.Input.Command.Action = a
			if strings.HasPrefix(a.Kind, "PRESSURE_") {
				in.Game = "pressure-roulette"
				f.view.Game = in.Game
			}
			f.view.Actions = []roulette.Action{{Kind: a.Kind, Target: a.Target, Item: a.Item, StolenItem: a.StolenItem, Agree: a.Agree}}
			p := prepare(t, s, in)
			if _, err := s.Commit(context.Background(), subject, p.Quote); err != nil {
				t.Fatal(err)
			}
			got := f.intent.Command.Action
			if !reflect.DeepEqual(got, roulette.Action{Kind: a.Kind, Target: a.Target, Item: a.Item, StolenItem: a.StolenItem, Agree: a.Agree}) {
				t.Fatal("full action changed across quote")
			}
			if _, err := s.Lookup(context.Background(), subject, p.Quote); err != nil {
				t.Fatal(err)
			}
			if f.intent.Game != in.Game || f.intent.Create != nil || f.intent.Command.Ready != nil {
				t.Fatal("lookup changed command branch")
			}
		})
	}
}

func TestPublicAndPrivateProjectionRetainsWebsiteValues(t *testing.T) {
	s, f, _, _ := fixture(t)
	f.public.Devil = &roulette.DevilView{Remaining: 4, Live: nil, Blank: nil, Saw: true, Cuffed: [2]bool{true, false}}
	f.public.Pressure = &roulette.PressureView{ActualLoad: 2, ForcedShots: 1, Order: []int{0, 2, 1}, Pointer: 1, UnloadSkipsShot: true, TimeoutTier: 2, Phase: "VOTE", Chambers: [6]string{"UNKNOWN", "UNKNOWN", "UNKNOWN", "UNKNOWN", "UNKNOWN", "UNKNOWN"}, PoolRemaining: 7, Duds: 2, Charge: 1, Loaded: 3, Forced: 2, Aggressor: new(1), RiposteTarget: new(2), Votes: map[int]bool{0: false, 1: true}}
	out, err := s.Public(context.Background(), RoomInput{Game: "devil-roulette", RoomID: roomID})
	if err != nil {
		t.Fatal(err)
	}
	assertFields(t, out.Room.Devil, "remaining,live,blank,saw,cuffed")
	assertFields(t, out.Room.Pressure, "actual_load,forced_shots,order,pointer,unload_skips_shot,timeout_tier,phase,chambers,pool_remaining,duds,charge,loaded,forced,aggressor,riposte_target,votes")
	if out.Room.Devil.Live != nil || out.Room.Devil.Blank != nil || out.Room.Pressure.Chambers[0] != "UNKNOWN" || out.Room.Pressure.Votes[0] || !out.Room.Pressure.Votes[1] {
		t.Fatal("public hidden/observable state changed")
	}
	f.view.EconomicPolicy = &platform.EconomicPolicy{Version: "policy", Hash: strings.Repeat("d", 64), SinglePlayerMaxUnits: 123, AssetCapUnits: 456, CapMode: "CLIP_PROFIT"}
	f.view.EconomySettlement = &platform.PayoutCapView{PolicyVersion: "policy", PolicyHash: strings.Repeat("d", 64), GrossPayoutUnits: 9000000, CreditedPayoutUnits: 4000000, WithheldUnits: 5000000, ActualNetUnits: -1000000, NeutralReturnUnits: 500000}
	state, err := s.State(context.Background(), subject, StateInput{RoomID: roomID})
	if err != nil {
		t.Fatal(err)
	}
	assertFields(t, state.EconomicPolicy, "version,hash,single_player_max_units,asset_cap_units,cap_mode")
	assertFields(t, state.EconomySettlement, "policy_version,policy_hash,gross_payout_units,credited_payout_units,withheld_units,actual_net_units,neutral_return_units")
	assertFields(t, state.Self, "seat,available_units,items,intel")
	if state.EconomySettlement.ActualNetUnits != -1000000 || state.EconomySettlement.CreditedPayoutUnits != 4000000 || state.EconomySettlement.NeutralReturnUnits != 500000 {
		t.Fatal("personal cap values were recomputed")
	}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"version":"7"`) || !strings.Contains(string(raw), `"actual_net_units":"-1000000"`) {
		t.Fatal("money/version not decimal strings")
	}
	f.view.Self = nil
	f.view.EconomicPolicy = nil
	f.view.EconomySettlement = nil
	f.view.Actions = nil
	f.view.Log = nil
	state, err = s.State(context.Background(), subject, StateInput{RoomID: roomID})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(state)
	for _, wire := range []string{`"self":null`, `"actions":[]`, `"economic_policy":null`, `"economy_settlement":null`, `"own_round_id":null`, `"log":[]`} {
		if !strings.Contains(string(raw), wire) {
			t.Fatalf("missing %s", wire)
		}
	}
}
