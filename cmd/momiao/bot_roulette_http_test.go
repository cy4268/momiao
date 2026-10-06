package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/botgames"
	"github.com/cy4268/momiao/internal/botroulette"
	"github.com/cy4268/momiao/internal/games/fairness"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/roulette"
)

const botRouletteTestRoom = "019923a0-0000-7000-8000-000000000001"
const botRouletteTestSubject = "123456789012345678"
const botRouletteTestUser int64 = 42
const botRoulettePrefix = "/internal/v1/bot-roulette/"

var botRouletteTestTime = time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
var botRouletteTestKey = [32]byte{1, 2, 3, 4, 5}

// Only the database-owning engine is replaced. HTTP parsing, identity,
// projection, quote signing/verification and deadline forwarding remain real.
type botRouletteHTTPEngine struct {
	lobby    roulette.Lobby
	view     roulette.RoomView
	public   roulette.PublicRoomView
	receipt  roulette.Receipt
	lookup   roulette.BotLookup
	err      error
	calls    []string
	user     int64
	intent   roulette.BotIntent
	deadline time.Time
}

func (e *botRouletteHTTPEngine) List(_ context.Context, user int64, q roulette.LobbyQuery) (roulette.Lobby, error) {
	e.calls = append(e.calls, "list")
	e.user = user
	if q.Game != e.lobby.Game {
		return roulette.Lobby{}, roulette.ErrInvalidInput
	}
	return e.lobby, e.err
}
func (e *botRouletteHTTPEngine) View(_ context.Context, user int64, id string) (roulette.RoomView, error) {
	e.calls = append(e.calls, "view")
	e.user = user
	if id != e.view.ID {
		return roulette.RoomView{}, roulette.ErrNotFound
	}
	return e.view, e.err
}
func (e *botRouletteHTTPEngine) PublicView(_ context.Context, id string) (roulette.PublicRoomView, error) {
	e.calls = append(e.calls, "public")
	if id != e.public.ID {
		return roulette.PublicRoomView{}, roulette.ErrNotFound
	}
	return e.public, e.err
}
func (e *botRouletteHTTPEngine) CreateBefore(_ context.Context, user int64, in roulette.CreateRequest, deadline time.Time) (roulette.Receipt, error) {
	e.calls = append(e.calls, "create")
	e.user = user
	e.deadline = deadline
	e.intent = roulette.BotIntent{Purpose: "CREATE", Game: in.Game, Create: &in}
	return e.receipt, e.err
}
func (e *botRouletteHTTPEngine) CommandBefore(_ context.Context, user int64, id string, in roulette.Command, deadline time.Time) (roulette.Receipt, error) {
	e.calls = append(e.calls, "command")
	e.user = user
	e.deadline = deadline
	e.intent = roulette.BotIntent{Purpose: "COMMAND", Game: e.view.Game, RoundID: id, Command: &in}
	return e.receipt, e.err
}
func (e *botRouletteHTTPEngine) FindReceiptMatching(_ context.Context, user int64, in roulette.BotIntent, deadline time.Time) (roulette.BotLookup, error) {
	e.calls = append(e.calls, "lookup")
	e.user = user
	e.intent = in
	e.deadline = deadline
	return e.lookup, e.err
}

func botRouletteHTTPEngineFixture(game string) *botRouletteHTTPEngine {
	seat, live, blank, riposte := 0, 3, 2, 1
	deadline, gameDeadline := botRouletteTestTime.Add(45*time.Second), botRouletteTestTime.Add(30*time.Minute)
	kind, count, title := "devil", 2, "恶魔轮盘"
	if game == "pressure-roulette" {
		kind, count, title = "pressure", 3, "压力轮盘"
	}
	b := roulette.Binding{ConfigID: botRouletteTestRoom, ConfigHash: strings.Repeat("a", 64), PolicyID: "019923a0-0000-7000-8000-000000000002", PolicyHash: strings.Repeat("b", 64), Ruleset: "momiao-" + kind + "-rules-v1", Algorithm: "momiao-" + kind + "-rng-v1", Stream: fairness.StreamVersion}
	players := []roulette.PlayerView{}
	for i := 0; i < count; i++ {
		players = append(players, roulette.PlayerView{Seat: i, Name: fmt.Sprintf("Fixture-%d", i), Ready: true, Alive: true, HP: 3, ItemCount: 1})
	}
	self := &roulette.SelfView{Seat: 0, AvailableUnits: 45000000, Items: []string{"magnifier"}, Intel: []roulette.IntelEntry{{Index: 2, Live: true}}}
	actions := []roulette.Action{{Kind: "DEVIL_SHOOT", Target: "OPPONENT"}, {Kind: "DEVIL_ITEM", Item: "adrenaline", StolenItem: "magnifier"}, {Kind: "SURRENDER"}}
	devil := &roulette.DevilView{Remaining: 5, Live: &live, Blank: &blank, Cuffed: [2]bool{false, true}}
	var pressure *roulette.PressureView
	if kind == "pressure" {
		no := false
		self.Items = nil
		self.Intel = nil
		actions = []roulette.Action{{Kind: "PRESSURE_VOTE", Agree: &no}, {Kind: "SURRENDER"}}
		devil = nil
		pressure = &roulette.PressureView{ActualLoad: 2, ForcedShots: 1, Order: []int{0, 2, 1}, Pointer: 0, TimeoutTier: 1, Phase: "VOTE", Chambers: [6]string{"UNKNOWN", "UNKNOWN", "UNKNOWN", "UNKNOWN", "UNKNOWN", "UNKNOWN"}, PoolRemaining: 4, Duds: 1, Charge: 2, Loaded: 2, Forced: 1, RiposteTarget: &riposte, Votes: map[int]bool{0: true, 1: false}}
	}
	v := roulette.RoomView{ID: botRouletteTestRoom, Game: game, Title: title, Version: 7, Sequence: 5, State: "PLAYING", TargetPlayers: count, StakeUnits: 5000000, PoolUnits: int64(count) * 5000000, Binding: b, ServerSeedHash: strings.Repeat("c", 64), TurnSeat: &seat, ServerNow: botRouletteTestTime, Deadline: &deadline, GameDeadline: &gameDeadline, Players: players, Self: self, Actions: actions, Log: []roulette.PublicEvent{{Sequence: 5, At: botRouletteTestTime, Seat: &seat, Kind: "TURN", Text: "Synthetic turn"}}, Devil: devil, Pressure: pressure, EconomicPolicy: &platform.EconomicPolicy{Version: "fixture-economy-v1", Hash: strings.Repeat("d", 64), SinglePlayerMaxUnits: 500000000, AssetCapUnits: 5000000000, CapMode: "CLIP_PROFIT"}}
	lobbyRoom := v
	lobbyRoom.Version = 1
	lobbyRoom.State = "WAITING"
	lobbyRoom.PoolUnits = 0
	lobbyRoom.Players = []roulette.PlayerView{{Seat: 0, Name: "Fixture-0", Alive: true}}
	return &botRouletteHTTPEngine{view: v, public: roulette.PublicRoomView{ID: v.ID, Game: v.Game, Title: v.Title, Version: v.Version, Sequence: v.Sequence, State: v.State, TargetPlayers: v.TargetPlayers, StakeUnits: v.StakeUnits, PoolUnits: v.PoolUnits, Binding: v.Binding, ServerSeedHash: v.ServerSeedHash, TurnSeat: v.TurnSeat, ServerNow: v.ServerNow, Deadline: v.Deadline, GameDeadline: v.GameDeadline, Players: v.Players, Log: v.Log, Devil: v.Devil, Pressure: v.Pressure}, lobby: roulette.Lobby{Game: game, State: "PLAY", Binding: b, MinimumUnits: 5000000, StepUnits: 500000, AvailableUnits: 45000000, Rooms: []roulette.RoomView{lobbyRoom}}, receipt: roulette.Receipt{RoundID: v.ID, Version: 6, Sequence: 4, State: "PLAYING"}, lookup: roulette.BotLookup{Status: "UNKNOWN"}}
}

func botRouletteHTTPService(t *testing.T, e *botRouletteHTTPEngine, resolver botgames.Resolver) *botroulette.Service {
	t.Helper()
	if resolver == nil {
		resolver = botGamesTestResolver(func(context.Context, string) (int64, error) { return botRouletteTestUser, nil })
	}
	s, err := botroulette.NewService(e, resolver, botRouletteTestKey, func() time.Time { return botRouletteTestTime })
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func botRouletteHTTPHandler(t *testing.T, e *botRouletteHTTPEngine, resolver botgames.Resolver) http.Handler {
	t.Helper()
	h, err := newBotRouletteHandler(botRouletteHTTPService(t, e, resolver), botGamesTestToken)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func botRouletteRequest(path, subject, body string) *http.Request {
	r := botGamesRequest("POST", botRoulettePrefix+path, body)
	r.Header.Del("X-Discord-User")
	if subject != "" {
		r.Header.Set("X-Discord-User", subject)
	}
	return r
}
func botRouletteError(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if w.Code != status || w.Body.String() != `{"schema_version":"1","error":"`+code+`"}`+"\n" {
		t.Fatalf("status=%d want=%d body=%s", w.Code, status, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Set-Cookie") != "" {
		t.Fatal("private response headers changed")
	}
}

func TestBotRouletteHTTPContract(t *testing.T) {
	type testCase struct {
		name, body   string
		headers      map[string][]string
		method, path string
		status       int
		code         string
	}
	create := `{"request_id":"1556455882752000000","purpose":"CREATE","game":"devil-roulette","input":{"stake":"10","players":2}}`
	ready := `{"request_id":"1556455882752000000","purpose":"COMMAND","game":"devil-roulette","room_id":"` + botRouletteTestRoom + `","input":{"expected_version":"7","action":{"kind":"READY"},"ready":{"client_seed":"fixture-seed","config_hash":"` + strings.Repeat("a", 64) + `","policy_hash":"` + strings.Repeat("b", 64) + `","server_seed_hash":"` + strings.Repeat("c", 64) + `","stake_units":"5000000"}}}`
	for _, route := range []string{"lobby", "public", "state", "prepare", "commit", "lookup"} {
		t.Run(route+"/identity_and_envelope", func(t *testing.T) {
			body := map[string]string{"lobby": `{"game":"devil-roulette","cursor":null}`, "public": `{"game":"devil-roulette","room_id":"` + botRouletteTestRoom + `"}`, "state": `{"room_id":"` + botRouletteTestRoom + `"}`, "prepare": create, "commit": `{"quote":"e30.YQ"}`, "lookup": `{"quote":"e30.YQ"}`}[route]
			subject := botRouletteTestSubject
			if route == "public" {
				subject = ""
			}
			cases := []testCase{
				{name: "valid", status: 404, code: "NOT_LINKED"},
				{name: "auth_missing", headers: map[string][]string{"Authorization": {}}, status: 401, code: "UNAUTHORIZED"},
				{name: "auth_duplicate", headers: map[string][]string{"Authorization": {"Bearer " + botGamesTestToken, "Bearer " + botGamesTestToken}}, status: 401, code: "UNAUTHORIZED"},
				{name: "auth_wrong", headers: map[string][]string{"Authorization": {"Bearer wrong"}}, status: 401, code: "UNAUTHORIZED"},
				{name: "wrong_method", method: "GET", status: 400, code: "INVALID_REQUEST"},
				{name: "extra_field", body: strings.TrimSuffix(body, "}") + `,"user_id":"42"}`, status: 400, code: "INVALID_REQUEST"},
				{name: "duplicate", body: strings.TrimSuffix(body, "}") + "," + strings.TrimSuffix(strings.SplitN(strings.TrimPrefix(body, "{"), ",", 2)[0], "}") + "}", status: 400, code: "INVALID_REQUEST"},
				{name: "missing_fields", body: `{}`, status: 400, code: "INVALID_REQUEST"},
				{name: "null", body: `null`, status: 400, code: "INVALID_REQUEST"},
				{name: "trailing", body: body + `{}`, status: 400, code: "INVALID_REQUEST"},
				{name: "wrong_type", headers: map[string][]string{"Content-Type": {"text/plain"}}, status: 400, code: "INVALID_REQUEST"},
				{name: "duplicate_type", headers: map[string][]string{"Content-Type": {"application/json", "application/json"}}, status: 400, code: "INVALID_REQUEST"},
				{name: "type_parameter", headers: map[string][]string{"Content-Type": {"application/json; charset=utf-8"}}, status: 400, code: "INVALID_REQUEST"},
				{name: "query", path: route + "?x=1", status: 400, code: "INVALID_REQUEST"},
				{name: "empty_query", path: route + "?", status: 400, code: "INVALID_REQUEST"},
				{name: "encoded", path: fmt.Sprintf("%%%02x%s", route[0], route[1:]), status: 400, code: "INVALID_REQUEST"},
				{name: "double_slash", path: "/" + route, status: 404, code: "NOT_FOUND"},
				{name: "dot_path", path: "../bot-roulette/" + route, status: 404, code: "NOT_FOUND"},
				{name: "trailing_slash", path: route + "/", status: 404, code: "NOT_FOUND"},
				{name: "oversize", body: body + strings.Repeat(" ", 4097-len(body)), status: 400, code: "INVALID_REQUEST"},
				{name: "oversize_stream", body: body + strings.Repeat(" ", 4097-len(body)), status: 400, code: "INVALID_REQUEST"},
			}
			if route == "public" {
				cases[0].status = 200
				cases[0].code = ""
			} else if route == "commit" || route == "lookup" {
				cases[0].status = 401
				cases[0].code = "UNAUTHORIZED"
			}
			for _, header := range []string{"Cookie", "Proxy-Authorization", "New-Api-User", "X-Auth-Session", "X-Native-User", "X-User-ID", "X-Forwarded-User", "X-Authenticated-User", "Idempotency-Key", "X-Fairness-Commitment"} {
				cases = append(cases, testCase{name: header, headers: map[string][]string{header: {""}}, status: 400, code: "INVALID_REQUEST"})
			}
			for _, bad := range []string{"", "0", "01", "+1", "18446744073709551616", "1, 2"} {
				if route == "public" && bad == "" {
					continue
				}
				values := []string{bad}
				if bad == "" {
					values = nil
				}
				cases = append(cases, testCase{name: "subject_" + bad, headers: map[string][]string{"X-Discord-User": values}, status: 400, code: "INVALID_REQUEST"})
			}
			cases = append(cases, testCase{name: "subject_duplicate", headers: map[string][]string{"X-Discord-User": {botRouletteTestSubject, botRouletteTestSubject}}, status: 400, code: "INVALID_REQUEST"})
			if route == "public" {
				cases = append(cases, testCase{name: "personal_on_public", headers: map[string][]string{"X-Discord-User": {botRouletteTestSubject}}, status: 400, code: "INVALID_REQUEST"})
				cases = append(cases, testCase{name: "empty_personal_header_on_public", headers: map[string][]string{"X-Discord-User": {""}}, status: 400, code: "INVALID_REQUEST"})
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					e := botRouletteHTTPEngineFixture("devil-roulette")
					resolves := 0
					h := botRouletteHTTPHandler(t, e, botGamesTestResolver(func(_ context.Context, s string) (int64, error) {
						resolves++
						if s != botRouletteTestSubject {
							t.Error("wrong identity forwarded")
						}
						return 0, botgames.Fault{Code: "NOT_LINKED"}
					}))
					if tc.body == "" {
						tc.body = body
					}
					if tc.path == "" {
						tc.path = route
					}
					r := botRouletteRequest(tc.path, subject, tc.body)
					if tc.method != "" {
						r.Method = tc.method
					}
					if tc.name == "oversize_stream" {
						r.ContentLength = -1
					}
					for key, values := range tc.headers {
						r.Header.Del(key)
						for _, v := range values {
							r.Header.Add(key, v)
						}
					}
					w := httptest.NewRecorder()
					h.ServeHTTP(w, r)
					if tc.status == 200 {
						if w.Code != 200 || resolves != 0 || !reflect.DeepEqual(e.calls, []string{"public"}) {
							t.Fatalf("public used personal identity/domain: %d %v", w.Code, e.calls)
						}
					} else {
						botRouletteError(t, w, tc.status, tc.code)
						if len(e.calls) != 0 {
							t.Fatal("rejected request reached engine")
						}
					}
					if tc.name != "valid" && resolves != 0 {
						t.Fatal("invalid envelope reached resolver")
					}
				})
			}
		})
	}
	t.Run("quote_subject_binding", func(t *testing.T) {
		e := botRouletteHTTPEngineFixture("devil-roulette")
		resolves := 0
		s := botRouletteHTTPService(t, e, botGamesTestResolver(func(context.Context, string) (int64, error) { resolves++; return 42, nil }))
		var in botroulette.PrepareInput
		if err := json.Unmarshal([]byte(create), &in); err != nil {
			t.Fatal(err)
		}
		p, err := s.Prepare(context.Background(), botRouletteTestSubject, in)
		if err != nil {
			t.Fatal(err)
		}
		e.calls, resolves = nil, 0
		h, err := newBotRouletteHandler(s, botGamesTestToken)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(botroulette.QuoteInput{Quote: p.Quote})
		for _, route := range []string{"commit", "lookup"} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, botRouletteRequest(route, "123456789012345679", string(body)))
			botRouletteError(t, w, 401, "UNAUTHORIZED")
		}
		if resolves != 0 || len(e.calls) != 0 {
			t.Fatal("another subject's quote reached identity or domain")
		}
	})
	t.Run("recursive_typed_decoding", func(t *testing.T) {
		cases := []string{
			strings.Replace(create, `"stake":"10"`, `"stake":"10","stake":"10"`, 1),
			strings.Replace(create, `"players":2`, `"players":2,"wallet":"42"`, 1),
			strings.Replace(create, `"players":2`, `"players":true`, 1),
			strings.Replace(ready, `"kind":"READY"`, `"kind":"READY","kind":"READY"`, 1),
			strings.Replace(ready, `"kind":"READY"`, `"kind":"READY","subject":"42"`, 1),
			strings.Replace(ready, `"client_seed":"fixture-seed"`, `"client_seed":"fixture-seed","client_seed":"fixture-seed"`, 1),
			strings.Replace(ready, `"client_seed":"fixture-seed"`, `"client_seed":"fixture-seed","hidden":"x"`, 1),
			strings.Replace(ready, `"client_seed":"fixture-seed"`, `"client_seed":"\ud800"`, 1),
			strings.Replace(ready, `"expected_version":"7"`, `"expected_version":"07"`, 1),
			strings.Replace(ready, `"expected_version":"7"`, `"expected_version":7`, 1),
		}
		for i, body := range cases {
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				e := botRouletteHTTPEngineFixture("devil-roulette")
				h := botRouletteHTTPHandler(t, e, botGamesTestResolver(func(context.Context, string) (int64, error) {
					t.Error("malformed nested input resolved identity")
					return 42, nil
				}))
				w := httptest.NewRecorder()
				h.ServeHTTP(w, botRouletteRequest("prepare", botRouletteTestSubject, body))
				botRouletteError(t, w, 400, "INVALID_REQUEST")
				if len(e.calls) != 0 {
					t.Fatal("malformed nested input reached engine")
				}
			})
		}
	})
	t.Run("exact_request_and_response_bounds", func(t *testing.T) {
		e := botRouletteHTTPEngineFixture("devil-roulette")
		h := botRouletteHTTPHandler(t, e, nil)
		body := `{"game":"devil-roulette","room_id":"` + botRouletteTestRoom + `"}`
		w := httptest.NewRecorder()
		h.ServeHTTP(w, botRouletteRequest("public", "", body+strings.Repeat(" ", 4096-len(body))))
		if w.Code != 200 {
			t.Fatal("4096-byte request rejected")
		}
		e.public.Title = ""
		w = httptest.NewRecorder()
		h.ServeHTTP(w, botRouletteRequest("public", "", body))
		padding := 65536 - w.Body.Len()
		e.public.Title = strings.Repeat("x", padding)
		w = httptest.NewRecorder()
		h.ServeHTTP(w, botRouletteRequest("public", "", body))
		if w.Code != 200 || w.Body.Len() != 65536 {
			t.Fatal("65536-byte response rejected")
		}
		e.public.Title = strings.Repeat("x", padding+1)
		w = httptest.NewRecorder()
		h.ServeHTTP(w, botRouletteRequest("public", "", body))
		botRouletteError(t, w, 503, "UPSTREAM_UNAVAILABLE")
		if w.Body.Len() > 65536 {
			t.Fatal("partial oversized response escaped")
		}
	})
	t.Run("twenty_public_events", func(t *testing.T) {
		e := botRouletteHTTPEngineFixture("devil-roulette")
		e.public.Log = nil
		for i := int64(1); i <= 25; i++ {
			e.public.Log = append(e.public.Log, roulette.PublicEvent{Sequence: i, At: botRouletteTestTime, Kind: "TURN", Text: "Synthetic event"})
		}
		w := httptest.NewRecorder()
		botRouletteHTTPHandler(t, e, nil).ServeHTTP(w, botRouletteRequest("public", "", `{"game":"devil-roulette","room_id":"`+botRouletteTestRoom+`"}`))
		var result botroulette.PublicReply
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Room.Log) != 20 || result.Room.Log[0].Sequence != 6 || result.Room.Log[19].Sequence != 25 {
			t.Fatal("public event tail violated")
		}
	})
	t.Run("fault_statuses", func(t *testing.T) {
		for code, status := range map[string]int{"INVALID_REQUEST": 400, "UNAUTHORIZED": 401, "BINDING_CHANGED": 403, "ACCOUNT_RESTRICTED": 403, "NOT_LINKED": 404, "NOT_FOUND": 404, "ACCOUNT_NOT_READY": 409, "QUOTE_EXPIRED": 409, "COMMITMENT_INVALID": 409, "IDEMPOTENCY_CONFLICT": 409, "INSUFFICIENT_CHIPS": 409, "MAINTENANCE": 409, "ROULETTE_VERSION_CONFLICT": 409, "ROULETTE_ACTION_INVALID": 409, "ROULETTE_ALREADY_SEATED": 409, "ROULETTE_NEEDS_REVIEW": 409, "UPSTREAM_UNAVAILABLE": 503, "RAW_SQL_SECRET": 503} {
			t.Run(code, func(t *testing.T) {
				e := botRouletteHTTPEngineFixture("devil-roulette")
				e.err = botroulette.Fault{Code: code}
				w := httptest.NewRecorder()
				botRouletteHTTPHandler(t, e, nil).ServeHTTP(w, botRouletteRequest("public", "", `{"game":"devil-roulette","room_id":"`+botRouletteTestRoom+`"}`))
				if code == "RAW_SQL_SECRET" {
					code = "UPSTREAM_UNAVAILABLE"
				}
				botRouletteError(t, w, status, code)
			})
		}
	})
	t.Run("bounded_inflight_and_deadline", func(t *testing.T) {
		entered, release := make(chan struct{}, 16), make(chan struct{})
		h := botRouletteHTTPHandler(t, botRouletteHTTPEngineFixture("devil-roulette"), botGamesTestResolver(func(ctx context.Context, _ string) (int64, error) {
			d, ok := ctx.Deadline()
			if !ok || time.Until(d) > 8*time.Second || time.Until(d) < 7*time.Second {
				t.Error("8-second handler deadline missing")
			}
			entered <- struct{}{}
			<-release
			return 0, errors.New("synthetic unavailable")
		}))
		var wg sync.WaitGroup
		defer func() { close(release); wg.Wait() }()
		for range 16 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				h.ServeHTTP(httptest.NewRecorder(), botRouletteRequest("lobby", botRouletteTestSubject, `{"game":"devil-roulette","cursor":null}`))
			}()
		}
		for range 16 {
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("16 calls did not enter")
			}
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, botRouletteRequest("state", botRouletteTestSubject, `{"room_id":"`+botRouletteTestRoom+`"}`))
		botRouletteError(t, w, 503, "UPSTREAM_UNAVAILABLE")
	})
}

func TestBotRouletteProbe(t *testing.T) {
	e := botRouletteHTTPEngineFixture("devil-roulette")
	h := botRouletteHTTPHandler(t, e, botGamesTestResolver(func(context.Context, string) (int64, error) {
		t.Fatal("probe resolved personal subject")
		return 0, nil
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, botRouletteRequest("public", "", `{}`))
	botRouletteError(t, w, 400, "INVALID_REQUEST")
	if len(e.calls) != 0 {
		t.Fatal("probe reached domain")
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, botRouletteRequest("probe", "", `{}`))
	botRouletteError(t, w, 404, "NOT_FOUND")
}

func TestBotRoulettePrivateMountOnly(t *testing.T) {
	for _, web := range []string{"", t.TempDir()} {
		for _, route := range []string{"lobby", "public", "state", "prepare", "commit", "lookup"} {
			cfg := config{WebDir: web, BotGames: botGamesConfig{RouletteEnabled: true}, roulette: &roulette.Service{}}
			w := httptest.NewRecorder()
			newPortalHandler(cfg, nil).ServeHTTP(w, botRouletteRequest(route, botRouletteTestSubject, `{}`))
			if w.Code != 404 || w.Header().Get("Location") != "" {
				t.Fatalf("private route exposed on public router: %s %d", route, w.Code)
			}
		}
	}
}

func TestBotRouletteConstructor(t *testing.T) {
	s := botRouletteHTTPService(t, botRouletteHTTPEngineFixture("devil-roulette"), nil)
	for _, token := range []string{"", "aa", strings.Repeat("0", 64), strings.Repeat("A", 64), botGamesTestToken + "\n"} {
		if _, err := newBotRouletteHandler(s, token); err == nil {
			t.Fatal("invalid service token accepted")
		}
	}
	if _, err := newBotRouletteHandler(nil, botGamesTestToken); err == nil {
		t.Fatal("nil service accepted")
	}
}

func TestBotRouletteContractFixture(t *testing.T) {
	raw, err := os.ReadFile("../../internal/botroulette/testdata/contract-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SchemaVersion string `json:"schema_version"`
		Synthetic     struct {
			Now          string `json:"now"`
			Subject      string `json:"subject"`
			ResolvedUser string `json:"resolved_user_id"`
			QuoteKey     string `json:"quote_key_hex"`
		} `json:"synthetic"`
		Cases []struct {
			Name, Game, Route, Subject string
			Request, Response          json.RawMessage
			Status                     int
		}
	}
	if json.Unmarshal(raw, &fixture) != nil || fixture.SchemaVersion != "1" || len(fixture.Cases) != 26 {
		t.Fatal("incomplete frozen fixture")
	}
	if fixture.Synthetic.Now != botRouletteTestTime.Format(time.RFC3339) ||
		fixture.Synthetic.Subject != botRouletteTestSubject ||
		fixture.Synthetic.ResolvedUser != strconv.FormatInt(botRouletteTestUser, 10) ||
		fixture.Synthetic.QuoteKey != hex.EncodeToString(botRouletteTestKey[:]) {
		t.Fatal("synthetic metadata differs from HTTP producer inputs")
	}
	// Wire examples must follow the existing domain, not mutually agreeing fakes.
	var frozen any
	if err := json.Unmarshal(raw, &frozen); err != nil {
		t.Fatal(err)
	}
	streams := 0
	var checkStream func(any)
	checkStream = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				if key == "fairness_stream_version" {
					streams++
					if child != fairness.StreamVersion {
						t.Fatalf("frozen stream=%v; authoritative=%s", child, fairness.StreamVersion)
					}
				}
				checkStream(child)
			}
		case []any:
			for _, child := range v {
				checkStream(child)
			}
		}
	}
	checkStream(frozen)
	if streams != 10 {
		t.Fatalf("frozen stream coverage=%d", streams)
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			e := botRouletteHTTPEngineFixture(tc.Game)
			kind := strings.SplitN(tc.Name, "/", 2)[1]
			switch kind {
			case "state":
				id := botRouletteTestRoom
				e.lobby.OwnRoundID = &id
			case "state_spectator":
				e.view.Self = nil
				e.view.Actions = nil
				e.view.EconomicPolicy = nil
			case "state_settled":
				e.view.State = "FINISHED"
				e.view.TurnSeat = nil
				e.view.Deadline = nil
				e.view.GameDeadline = nil
				e.view.Actions = nil
				e.view.EconomySettlement = &platform.PayoutCapView{PolicyVersion: "fixture-economy-v1", PolicyHash: strings.Repeat("d", 64), GrossPayoutUnits: 10000000, CreditedPayoutUnits: 8000000, WithheldUnits: 2000000, ActualNetUnits: 3000000}
				if tc.Game == "pressure-roulette" {
					e.view.EconomySettlement.NeutralReturnUnits = 5000000
				}
			case "prepare_ready":
				e.view.State = "WAITING"
				e.view.Actions = []roulette.Action{{Kind: "READY"}}
			case "commit_create":
				e.receipt = roulette.Receipt{RoundID: botRouletteTestRoom, Version: 1, Sequence: 0, State: "WAITING"}
			case "lookup_applied":
				e.lookup = roulette.BotLookup{Status: "APPLIED", Receipt: &e.receipt}
			case "lookup_absent_final":
				e.lookup = roulette.BotLookup{Status: "ABSENT_FINAL"}
			}
			resolves := 0
			h := botRouletteHTTPHandler(t, e, botGamesTestResolver(func(_ context.Context, s string) (int64, error) {
				resolves++
				if s != botRouletteTestSubject {
					t.Error("fixture subject changed")
				}
				return botRouletteTestUser, nil
			}))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, botRouletteRequest(tc.Route, tc.Subject, string(tc.Request)))
			var got, want any
			if json.Unmarshal(w.Body.Bytes(), &got) != nil || json.Unmarshal(tc.Response, &want) != nil || w.Code != tc.Status || !reflect.DeepEqual(got, want) {
				t.Fatalf("frozen %s contract differs: status=%d body=%s", tc.Name, w.Code, w.Body.String())
			}
			if tc.Route == "public" {
				if resolves != 0 || !reflect.DeepEqual(e.calls, []string{"public"}) {
					t.Fatal("public leaked personal resolution")
				}
			} else if resolves != 1 || e.user != botRouletteTestUser {
				t.Fatal("personal route failed exact re-resolution")
			}
			expectedCalls := map[string][]string{"lobby": {"list"}, "public": {"public"}, "state": {"view", "list"}, "prepare": {"view"}, "commit": {"command"}, "lookup": {"lookup"}}[tc.Route]
			if kind == "prepare_create" {
				expectedCalls = []string{"list"}
			}
			if kind == "commit_create" {
				expectedCalls = []string{"create"}
			}
			if !reflect.DeepEqual(e.calls, expectedCalls) {
				t.Fatalf("wrong domain dispatch: %v", e.calls)
			}
			if tc.Route == "commit" || tc.Route == "lookup" {
				if !e.deadline.Equal(botRouletteTestTime.Add(120 * time.Second)) {
					t.Fatal("quote deadline changed")
				}
			}
		})
	}
}
