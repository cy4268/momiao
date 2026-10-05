package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/botroulette"
	"github.com/cy4268/momiao/internal/historyaccess"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/roulette"
	"github.com/jackc/pgx/v5"
)

// One fixture owns this test's database. Both entry points use the same engine,
// account rows and UUID; no duplicate wallet or roulette implementation exists.
func TestBotRouletteCrossPlatformConservation(t *testing.T) {
	f := newBotRoulettePGFixture(t)
	ctx := context.Background()
	observer, err := platform.OpenNativeQuota(ctx, f.base.connection.OwnerURL)
	botGamesPGCheck(t, err, "cross native quota observer")
	defer observer.Close()
	f.engine, err = roulette.NewServiceWithEconomy(f.base.runtime,
		roulette.Keyring{Active: "bot-fixture-v1", Keys: map[string][32]byte{"bot-fixture-v1": {9}}}, observer)
	botGamesPGCheck(t, err, "cross engine")
	f.base.exec(t, `UPDATE games.game_registry SET publication_state='PUBLISHED' WHERE game_slug='pressure-roulette'`)
	f.base.exec(t, platform.NativeQuotaMigration)
	f.base.exec(t, `UPDATE momiao_quota.settings SET enabled=true`)
	f.base.exec(t, `UPDATE economy.policy_runtime SET active_version='economy-cap-v1' WHERE singleton`)
	service, err := botroulette.NewService(f.engine,
		botGamesResolver{native: f.base.native, platform: f.base.runtime, declaration: f.base.declaration}, [32]byte{2}, time.Now)
	botGamesPGCheck(t, err, "cross bot service")
	serial := uint64(0)
	prepare := func(subject string, in botroulette.PrepareInput) botroulette.PreparedReply {
		serial++
		in.RequestID = strconv.FormatUint(uint64(time.Now().UnixMilli()-1420070400000)<<22|serial, 10)
		p, e := service.Prepare(ctx, subject, in)
		botGamesPGCheck(t, e, "cross prepare")
		return p
	}
	for _, count := range []int{2, 3, 4, 6} {
		t.Run(fmt.Sprintf("players_%d", count), func(t *testing.T) {
			game := "pressure-roulette"
			if count == 2 {
				game = "devil-roulette"
			}
			users, subjects := make([]int64, count), make([]string, count)
			for i := range users {
				users[i], subjects[i] = f.base.player(t, true, 50000000)
				_, e := f.base.owner.Apply(ctx, platform.Mutation{UserID: users[i], Asset: platform.ReserveAPICredit,
					DeltaUnits: platform.AssetCapUnits - 50000000, BizType: "CROSS_CAP", BizID: subjects[i],
					EntryType: "TEST_GRANT", IdempotencyKey: "cross-cap:" + subjects[i]})
				botGamesPGCheck(t, e, "cross cap fixture grant")
			}
			var id string
			if count%3 == 0 {
				p := prepare(subjects[0], botroulette.PrepareInput{Purpose: "CREATE", Game: game,
					Input: botroulette.TypedInput{Create: &botroulette.CreateInput{Stake: "10", Players: count}}})
				r, e := service.Commit(ctx, subjects[0], p.Quote)
				botGamesPGCheck(t, e, "bot creates original room")
				id = r.Receipt.RoundID
			} else {
				r, e := f.engine.Create(ctx, users[0], roulette.CreateRequest{Key: f.key(), Game: game, Stake: "10", Players: count})
				botGamesPGCheck(t, e, "browser creates original room")
				id = r.RoundID
			}
			botCommand := func(seat int, kind string) (botroulette.PreparedReply, botroulette.ReceiptReply) {
				v := f.view(t, users[seat], id)
				in := botroulette.CommandInput{ExpectedVersion: v.Version, Action: botroulette.Action{Kind: kind}}
				if kind == "READY" {
					in.Ready = &botroulette.ReadyConfirmation{ClientSeed: "cross-cap-seed", ConfigHash: v.Binding.ConfigHash,
						PolicyHash: v.Binding.PolicyHash, ServerSeedHash: v.ServerSeedHash, StakeUnits: v.StakeUnits}
				}
				p := prepare(subjects[seat], botroulette.PrepareInput{Purpose: "COMMAND", Game: game, RoomID: &id,
					Input: botroulette.TypedInput{Command: &in}})
				r, e := service.Commit(ctx, subjects[seat], p.Quote)
				botGamesPGCheck(t, e, "cross bot command")
				return p, r
			}
			for i := 1; i < count; i++ {
				if count%3 == 0 {
					f.command(t, users[i], id, "JOIN")
				} else {
					botCommand(i, "JOIN")
				}
			}
			for _, user := range users {
				if f.base.balance(t, user) != 50000000 {
					t.Fatal("CREATE/JOIN charged funds")
				}
			}
			p, original := botCommand(0, "READY")
			f.command(t, users[0], id, "UNREADY")
			if f.base.balance(t, users[0]) != 50000000 {
				t.Fatal("UNREADY did not refund principal")
			}
			for i := range users {
				if i%2 == 0 {
					f.command(t, users[i], id, "READY")
				} else {
					botCommand(i, "READY")
				}
			}
			before := f.domain(t, id)
			lookup, e := service.Lookup(ctx, subjects[0], p.Quote)
			botGamesPGCheck(t, e, "original READY lookup after newer browser actions")
			if !reflect.DeepEqual(original, lookup) || !reflect.DeepEqual(before, f.domain(t, id)) {
				t.Fatal("lookup changed original receipt or funds")
			}
			for i := 0; i < count-1; i++ {
				v := f.view(t, users[0], id)
				if v.TurnSeat == nil {
					t.Fatal("missing original room turn")
				}
				seat := *v.TurnSeat
				if i%2 == 0 {
					botCommand(seat, "SURRENDER")
				} else {
					f.command(t, users[seat], id, "SURRENDER")
				}
				after := f.view(t, users[0], id)
				if after.State == "PLAYING" && after.PoolUnits != int64(count)*5000000 {
					t.Fatal("surrender removed escrow from pool")
				}
			}
			if f.view(t, users[0], id).State != "FINISHED" {
				t.Fatal("mixed original room did not finish")
			}
			var credited, withheld, gross int64
			for _, user := range users {
				v := f.view(t, user, id)
				c := v.EconomySettlement
				if c == nil || c.GrossPayoutUnits != c.CreditedPayoutUnits+c.WithheldUnits {
					t.Fatal("gross/actual credit/withheld mismatch")
				}
				credited += c.CreditedPayoutUnits
				withheld += c.WithheldUnits
				gross += c.GrossPayoutUnits
				if f.base.balance(t, user)-50000000 != c.ActualNetUnits {
					t.Fatal("actual wallet delta differs from cap result")
				}
				history, e := f.engine.HistoryDetail(ctx, historyaccess.Own(user), id, roulette.HistoryQuery{Limit: 50})
				botGamesPGCheck(t, e, "same-room history")
				if history.PayoutUnits != strconv.FormatInt(c.CreditedPayoutUnits, 10) || history.NetUnits != strconv.FormatInt(c.ActualNetUnits, 10) {
					t.Fatal("history differs from actual money")
				}
				proof, e := f.engine.HistoryVerify(ctx, historyaccess.Own(user), id)
				botGamesPGCheck(t, e, "same-room history verify")
				if !proof.Valid || !proof.SettlementValid {
					t.Fatal("history conservation proof invalid")
				}
			}
			if gross != int64(count)*5000000 || credited+withheld != gross || withheld <= 0 {
				t.Fatal("cross cap conservation")
			}
			t.Logf("CROSS_MONEY game=%s players=%d room=%s gross=%d credited=%d withheld=%d held=0 history=VALID", game, count, id, gross, credited, withheld)
		})
	}
}

type botRoulettePGFixture struct {
	base           *botGamesPGFixture
	engine, worker *roulette.Service
}

func newBotRoulettePGFixture(t *testing.T) *botRoulettePGFixture {
	t.Helper()
	f := &botRoulettePGFixture{base: newBotGamesPGFixture(t)}
	keys := roulette.Keyring{Active: "bot-fixture-v1", Keys: map[string][32]byte{"bot-fixture-v1": {9}}}
	var err error
	f.engine, err = roulette.NewService(f.base.runtime, keys)
	botGamesPGCheck(t, err, "roulette engine")
	f.worker, err = roulette.NewService(f.base.runtime, keys)
	botGamesPGCheck(t, err, "roulette worker")
	return f
}

func (f *botRoulettePGFixture) key() string {
	f.base.next++
	return fmt.Sprintf("bot-roulette-fixture-%d", f.base.next)
}

func (f *botRoulettePGFixture) view(t *testing.T, user int64, id string) roulette.RoomView {
	t.Helper()
	v, err := f.engine.View(context.Background(), user, id)
	botGamesPGCheck(t, err, "browser roulette view")
	return v
}

func (f *botRoulettePGFixture) command(t *testing.T, user int64, id, kind string) roulette.Receipt {
	t.Helper()
	v := f.view(t, user, id)
	cmd := roulette.Command{Key: f.key(), ExpectedVersion: v.Version, Action: roulette.Action{Kind: kind}}
	if kind == "READY" {
		cmd.Ready = &roulette.ReadyConfirmation{ClientSeed: "roulette-fixture-seed", ConfigHash: v.Binding.ConfigHash,
			PolicyHash: v.Binding.PolicyHash, ServerSeedHash: v.ServerSeedHash, StakeUnits: strconv.FormatInt(v.StakeUnits, 10)}
	}
	r, err := f.engine.Command(context.Background(), user, id, cmd)
	botGamesPGCheck(t, err, "roulette "+kind)
	return r
}

func (f *botRoulettePGFixture) room(t *testing.T, game string, start bool) (string, []int64) {
	t.Helper()
	count := 2
	if game == "pressure-roulette" {
		count = 3
	}
	users := make([]int64, count)
	for i := range users {
		users[i], _ = f.base.player(t, true, 50000000)
	}
	r, err := f.engine.Create(context.Background(), users[0], roulette.CreateRequest{Key: f.key(), Game: game, Stake: "10", Players: count})
	botGamesPGCheck(t, err, "roulette create")
	for _, u := range users[1:] {
		f.command(t, u, r.RoundID, "JOIN")
	}
	// Even WAITING has real escrow, so accidental expiry/refunds are observable.
	f.command(t, users[0], r.RoundID, "READY")
	if start {
		for _, u := range users[1:] {
			f.command(t, u, r.RoundID, "READY")
		}
	}
	return r.RoundID, users
}

// Capture actual domain rows, not the response under test. Equality catches
// timeout advancement, receipt/action insertion, wallet changes and settlement.
func (f *botRoulettePGFixture) domain(t *testing.T, id string) json.RawMessage {
	t.Helper()
	var raw []byte
	err := f.base.owner.WithTx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `WITH users AS (SELECT newapi_user_id FROM roulette.participants WHERE round_id=$1)
		SELECT jsonb_build_object(
		 'round',(SELECT to_jsonb(r) FROM roulette.rounds r WHERE round_id=$1),
		 'participants',(SELECT jsonb_agg(to_jsonb(p) ORDER BY newapi_user_id) FROM roulette.participants p WHERE round_id=$1),
		 'actions',(SELECT jsonb_agg(to_jsonb(a) ORDER BY sequence) FROM roulette.actions a WHERE round_id=$1),
		 'action_count',(SELECT count(*) FROM roulette.actions WHERE round_id=$1),
		 'commands',(SELECT jsonb_agg(to_jsonb(c) ORDER BY actor_user_id,key_hash) FROM roulette.commands c WHERE round_id=$1),
		 'receipt_count',(SELECT count(*) FROM roulette.commands WHERE round_id=$1),
		 'funding',(SELECT jsonb_agg(to_jsonb(f) ORDER BY funding_id) FROM roulette.funding f WHERE round_id=$1),
		 'wallets',(SELECT jsonb_agg(to_jsonb(w) ORDER BY newapi_user_id,asset_type) FROM economy.wallet_balances w WHERE newapi_user_id IN(SELECT * FROM users)),
		 'ledger',(SELECT jsonb_agg(to_jsonb(l) ORDER BY ledger_entry_id) FROM economy.wallet_ledger l WHERE newapi_user_id IN(SELECT * FROM users)),
		 'transactions',(SELECT jsonb_agg(to_jsonb(a) ORDER BY transaction_id) FROM economy.asset_transactions a WHERE newapi_user_id IN(SELECT * FROM users)),
		 'caps',(SELECT jsonb_agg(to_jsonb(c) ORDER BY newapi_user_id) FROM economy.cap_settlements c WHERE source_kind='ROULETTE_ROUND' AND source_id=$1::text))`, id).Scan(&raw)
	})
	botGamesPGCheck(t, err, "domain snapshot")
	return raw
}

func (f *botRoulettePGFixture) assertPublicRead(t *testing.T, id string, user int64, state string) roulette.PublicRoomView {
	t.Helper()
	current := f.view(t, user, id)
	if current.State != state {
		t.Fatalf("fixture state=%s want=%s", current.State, state)
	}
	before := f.domain(t, id)
	role := pgx.Identifier{f.base.connection.RuntimeRole}.Sanitize()
	// Public reads must not depend on personal wallet/cap permissions at all.
	f.base.exec(t, `REVOKE SELECT ON economy.wallet_balances,economy.cap_settlements FROM `+role)
	f.base.exec(t, `REVOKE SELECT(asset_type,balance_units,ledger_seq,newapi_user_id,updated_at,version) ON economy.wallet_balances FROM `+role)
	defer f.base.exec(t, `GRANT SELECT ON economy.cap_settlements TO `+role)
	defer f.base.exec(t, `GRANT SELECT(asset_type,balance_units,ledger_seq,newapi_user_id,updated_at,version) ON economy.wallet_balances TO `+role)
	var got roulette.PublicRoomView
	for range 3 {
		var err error
		got, err = f.engine.PublicView(context.Background(), id)
		botGamesPGCheck(t, err, "actor-free public read")
		if got.ID != id || got.Version != current.Version {
			t.Fatal("wrong room")
		}
		if got.State != state || got.Sequence != current.Sequence || got.Game != current.Game || got.Title != current.Title ||
			got.TargetPlayers != current.TargetPlayers || got.StakeUnits != 5000000 || got.PoolUnits != current.PoolUnits ||
			got.Binding != current.Binding || got.ServerSeedHash != current.ServerSeedHash ||
			!reflect.DeepEqual(got.TurnSeat, current.TurnSeat) || !reflect.DeepEqual(got.Deadline, current.Deadline) ||
			!reflect.DeepEqual(got.GameDeadline, current.GameDeadline) || !reflect.DeepEqual(got.Players, current.Players) ||
			!reflect.DeepEqual(got.Devil, current.Devil) || !reflect.DeepEqual(got.Pressure, current.Pressure) {
			t.Fatal("public snapshot differs from browser public state")
		}
		log := current.Log
		if len(log) > 20 {
			log = log[len(log)-20:]
		}
		if !reflect.DeepEqual(got.Log, log) || got.ServerNow.Before(current.ServerNow) || got.ServerNow.After(time.Now().Add(time.Second)) {
			t.Fatal("wrong public log or server clock")
		}
	}
	after := f.domain(t, id)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("read changed domain state")
	}
	t.Logf("READ_ONLY %s %s: before=%x after=%x; round/version/actions/receipts/wallets/escrow unchanged", got.Game, state, sha256.Sum256(before), sha256.Sum256(after))
	return got
}

func TestBotRoulettePublicReadNoMutation(t *testing.T) {
	f := newBotRoulettePGFixture(t)
	ctx := context.Background()
	// Publish only the isolated fixture's existing pressure configuration.
	f.base.exec(t, `UPDATE games.game_registry SET publication_state='PUBLISHED' WHERE game_slug='pressure-roulette'`)
	for _, game := range []string{"devil-roulette", "pressure-roulette"} {
		for _, state := range []string{"WAITING", "PLAYING", "SETTLING", "FINISHED", "CANCELLED", "NEEDS_REVIEW"} {
			t.Run(game+"/"+state, func(t *testing.T) {
				id, users := f.room(t, game, state != "WAITING" && state != "CANCELLED")
				switch state {
				case "WAITING", "PLAYING":
					f.base.exec(t, `UPDATE roulette.rounds SET deadline=clock_timestamp()-interval '1 second',version=version+1 WHERE round_id=$1`, id)
					defer f.base.exec(t, `UPDATE roulette.rounds SET deadline=clock_timestamp()+interval '1 hour',version=version+1 WHERE round_id=$1`, id)
				case "CANCELLED":
					f.command(t, users[0], id, "CANCEL")
				case "SETTLING", "FINISHED":
					role := pgx.Identifier{f.base.connection.RuntimeRole}.Sanitize()
					if state == "SETTLING" {
						// Fail only the wallet/funding handoff, after the real final action committed.
						f.base.exec(t, `REVOKE INSERT ON roulette.funding FROM `+role)
						defer f.base.exec(t, `GRANT INSERT ON roulette.funding TO `+role)
					}
					for range len(users) - 1 {
						v := f.view(t, users[0], id)
						if v.TurnSeat == nil {
							t.Fatal("missing fixture turn")
						}
						f.command(t, users[*v.TurnSeat], id, "SURRENDER")
					}
				case "NEEDS_REVIEW":
					f.base.exec(t, `UPDATE roulette.rounds SET snapshot_key_version='missing-fixture-key',version=version+1 WHERE round_id=$1`, id)
					before := f.domain(t, id)
					if _, err := f.engine.PublicView(ctx, id); !errors.Is(err, roulette.ErrUnavailable) {
						t.Fatalf("damaged PLAYING snapshot: %v", err)
					}
					if !reflect.DeepEqual(before, f.domain(t, id)) {
						t.Fatal("failed public read changed domain state")
					}
					f.base.exec(t, `UPDATE roulette.rounds SET state='NEEDS_REVIEW',deadline=NULL,version=version+1 WHERE round_id=$1`, id)
				}
				got := f.assertPublicRead(t, id, users[0], state)
				if state == "PLAYING" && game == "pressure-roulette" && got.Pressure.Chambers != [6]string{"UNKNOWN", "UNKNOWN", "UNKNOWN", "UNKNOWN", "UNKNOWN", "UNKNOWN"} {
					t.Fatal("public projection disclosed pressure chambers")
				}
				if state == "NEEDS_REVIEW" && (got.Devil != nil || got.Pressure != nil || got.TurnSeat != nil) {
					t.Fatal("damaged review snapshot was projected as playable")
				}
				if state == "SETTLING" {
					f.base.exec(t, `GRANT INSERT ON roulette.funding TO `+pgx.Identifier{f.base.connection.RuntimeRole}.Sanitize())
					_, err := f.worker.StepDue(ctx, 1)
					botGamesPGCheck(t, err, "shared-key settlement worker")
					if f.view(t, users[0], id).State != "FINISHED" {
						t.Fatal("worker did not finish the original room")
					}
				}
			})
		}
	}
	t.Run("latest_twenty_public_events", func(t *testing.T) {
		id, users := f.room(t, "devil-roulette", true)
		// Known immutable event rows make the tail boundary deterministic without
		// depending on random game duration; the encrypted room is a real PLAYING snapshot.
		for seq := int64(1); seq <= 25; seq++ {
			event, err := json.Marshal(roulette.PublicEvent{Sequence: seq, At: time.Date(2026, 10, 5, 0, 0, int(seq), 0, time.UTC), Kind: "FIXTURE_PUBLIC", Text: fmt.Sprintf("event %d", seq)})
			botGamesPGCheck(t, err, "public event fixture")
			f.base.exec(t, `INSERT INTO roulette.actions(round_id,sequence,input,occurred_at,domain,state_hash,public_event) VALUES($1,$2::bigint,'{}',clock_timestamp(),'roulette/action/v1/'||($2::bigint)::text,decode(repeat('00',32),'hex'),$3)`, id, seq, event)
			f.base.exec(t, `UPDATE roulette.rounds SET action_sequence=$2,version=version+1 WHERE round_id=$1`, id, seq)
		}
		got := f.assertPublicRead(t, id, users[0], "PLAYING")
		if len(got.Log) != 20 || got.Log[0].Sequence != 6 || got.Log[19].Sequence != 25 || got.Log[0].Text != "event 6" || got.Log[19].Text != "event 25" {
			t.Fatal("public event tail is not the newest 20 in ascending order")
		}
	})
	t.Run("absent_room", func(t *testing.T) {
		if _, err := f.engine.PublicView(ctx, "019a6000-0000-7000-8000-000000000099"); !errors.Is(err, roulette.ErrNotFound) {
			t.Fatalf("absent room: %v", err)
		}
	})
}

func (f *botRoulettePGFixture) readyInput(t *testing.T, user int64, id string) roulette.Command {
	t.Helper()
	v := f.view(t, user, id)
	return roulette.Command{Key: f.key(), ExpectedVersion: v.Version, Action: roulette.Action{Kind: "READY"},
		Ready: &roulette.ReadyConfirmation{ClientSeed: "guarded-fixture-seed", ConfigHash: v.Binding.ConfigHash,
			PolicyHash: v.Binding.PolicyHash, ServerSeedHash: v.ServerSeedHash, StakeUnits: strconv.FormatInt(v.StakeUnits, 10)}}
}

func assertBotRouletteLookup(t *testing.T, got roulette.BotLookup, status string, receipt *roulette.Receipt) {
	t.Helper()
	if got.Status != status || !reflect.DeepEqual(got.Receipt, receipt) {
		t.Fatalf("lookup=%+v receipt=%+v; want %s %+v", got, got.Receipt, status, receipt)
	}
}

// Replacing the persisted receipt with a current view, omitting semantic
// matching, or repeating the READY debit must each break this test.
func TestBotRouletteGuardedReceiptPG(t *testing.T) {
	f := newBotRoulettePGFixture(t)
	ctx := context.Background()
	f.base.exec(t, `UPDATE games.game_registry SET publication_state='PUBLISHED' WHERE game_slug='pressure-roulette'`)
	future, past := time.Now().Add(2*time.Minute), time.Now().Add(-time.Minute)
	for _, game := range []string{"devil-roulette", "pressure-roulette"} {
		t.Run(game, func(t *testing.T) {
			host, _ := f.base.player(t, true, 50000000)
			guest, _ := f.base.player(t, true, 50000000)
			players := 2
			if game == "pressure-roulette" {
				players = 3
			}
			create := roulette.CreateRequest{Key: f.key(), Game: game, Stake: "10", Players: players}
			created, err := f.engine.CreateBefore(ctx, host, create, future)
			botGamesPGCheck(t, err, "guarded CREATE")
			if created.Version != 1 || created.Sequence != 0 || created.State != "WAITING" {
				t.Fatalf("create receipt=%+v", created)
			}
			join := roulette.Command{Key: f.key(), ExpectedVersion: 1, Action: roulette.Action{Kind: "JOIN"}}
			joined, err := f.engine.CommandBefore(ctx, guest, created.RoundID, join, future)
			botGamesPGCheck(t, err, "guarded JOIN")
			ready := f.readyInput(t, host, created.RoundID)
			readied, err := f.engine.CommandBefore(ctx, host, created.RoundID, ready, future)
			botGamesPGCheck(t, err, "guarded READY")
			if joined.Version != 2 || readied.Version != 3 {
				t.Fatalf("unexpected original versions: JOIN=%d READY=%d", joined.Version, readied.Version)
			}
			// A different actor advances the shared room after all three receipts.
			f.command(t, guest, created.RoundID, "READY")
			current := f.view(t, host, created.RoundID)
			if current.Version <= readied.Version {
				t.Fatal("fixture did not advance the shared room")
			}
			before := f.domain(t, created.RoundID)
			for _, tc := range []struct {
				name     string
				user     int64
				intent   roulette.BotIntent
				original roulette.Receipt
			}{
				{"CREATE", host, roulette.BotIntent{Purpose: "CREATE", Game: game, Create: &create}, created},
				{"JOIN", guest, roulette.BotIntent{Purpose: "COMMAND", Game: game, RoundID: created.RoundID, Command: &join}, joined},
				{"READY", host, roulette.BotIntent{Purpose: "COMMAND", Game: game, RoundID: created.RoundID, Command: &ready}, readied},
			} {
				t.Run(tc.name, func(t *testing.T) {
					var replay roulette.Receipt
					var err error
					if tc.intent.Create != nil {
						replay, err = f.engine.CreateBefore(ctx, tc.user, *tc.intent.Create, past)
					} else {
						replay, err = f.engine.CommandBefore(ctx, tc.user, tc.intent.RoundID, *tc.intent.Command, past)
					}
					botGamesPGCheck(t, err, "expired original replay")
					if tc.original != replay {
						t.Fatal("receipt replaced by current state")
					}
					lookup, err := f.engine.FindReceiptMatching(ctx, tc.user, tc.intent, past)
					botGamesPGCheck(t, err, "matching original lookup")
					assertBotRouletteLookup(t, lookup, "APPLIED", &tc.original)
					key := create.Key
					if tc.intent.Command != nil {
						key = tc.intent.Command.Key
					}
					browser, err := f.engine.FindReceipt(ctx, tc.user, key)
					botGamesPGCheck(t, err, "browser original receipt")
					if browser != tc.original {
						t.Fatal("browser receipt semantics changed")
					}
					t.Logf("ORIGINAL %s: version=%d current=%d expired replay and lookup identical", tc.name, replay.Version, current.Version)
				})
			}
			changedCreate := create
			changedCreate.Stake = "11"
			_, err = f.engine.CreateBefore(ctx, host, changedCreate, past)
			if !errors.Is(err, roulette.ErrIdempotencyConflict) {
				t.Fatalf("changed CREATE semantic: %v", err)
			}
			_, err = f.engine.FindReceiptMatching(ctx, host, roulette.BotIntent{Purpose: "CREATE", Game: game, Create: &changedCreate}, past)
			if !errors.Is(err, roulette.ErrIdempotencyConflict) {
				t.Fatalf("changed CREATE lookup semantic: %v", err)
			}
			for _, kind := range []string{"version", "action", "seed", "purpose", "join"} {
				changed := ready
				actor := host
				r := *ready.Ready
				changed.Ready = &r
				switch kind {
				case "version":
					changed.ExpectedVersion++
				case "action":
					changed.Action.Kind, changed.Ready = "UNREADY", nil
				case "seed":
					changed.Ready.ClientSeed = "different-explicit-decision"
				case "purpose":
					changed.Key = create.Key
				case "join":
					changed, actor = join, guest
					changed.ExpectedVersion++
				}
				_, err = f.engine.CommandBefore(ctx, actor, created.RoundID, changed, past)
				if !errors.Is(err, roulette.ErrIdempotencyConflict) {
					t.Fatalf("changed COMMAND %s: %v", kind, err)
				}
				_, err = f.engine.FindReceiptMatching(ctx, actor, roulette.BotIntent{Purpose: "COMMAND", Game: game, RoundID: created.RoundID, Command: &changed}, past)
				if !errors.Is(err, roulette.ErrIdempotencyConflict) {
					t.Fatalf("changed COMMAND lookup %s: %v", kind, err)
				}
			}
			otherGame := "pressure-roulette"
			if game == otherGame {
				otherGame = "devil-roulette"
			}
			_, err = f.engine.FindReceiptMatching(ctx, host, roulette.BotIntent{Purpose: "COMMAND", Game: otherGame, RoundID: created.RoundID, Command: &ready}, past)
			if !errors.Is(err, roulette.ErrInvalidInput) {
				t.Fatalf("same key/room/action under another game: %v", err)
			}
			if !reflect.DeepEqual(before, f.domain(t, created.RoundID)) {
				t.Fatal("replay, lookup or conflict changed domain state")
			}
			var debitCount, roundCount int
			err = f.base.owner.WithTx(ctx, func(tx pgx.Tx) error {
				return tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM roulette.funding WHERE round_id=$1 AND newapi_user_id=$2 AND kind='ESCROW'),(SELECT count(*) FROM roulette.rounds WHERE host_user_id=$2)`, created.RoundID, host).Scan(&debitCount, &roundCount)
			})
			botGamesPGCheck(t, err, "single effects")
			if debitCount != 1 || roundCount != 1 || f.base.balance(t, host) != 45000000 {
				t.Fatalf("READY debits=%d CREATE rounds=%d", debitCount, roundCount)
			}
			t.Log("EFFECTS: one CREATE, one READY escrow debit, balance=45000000")
		})
	}
	t.Run("due_is_command_only", func(t *testing.T) {
		id, users := f.room(t, "devil-roulette", false)
		in := roulette.Command{Key: f.key(), ExpectedVersion: f.view(t, users[0], id).Version, Action: roulette.Action{Kind: "CANCEL"}}
		intent := roulette.BotIntent{Purpose: "COMMAND", Game: "devil-roulette", RoundID: id, Command: &in}
		f.base.exec(t, `UPDATE roulette.rounds SET deadline=clock_timestamp()-interval '1 second',version=version+1 WHERE round_id=$1`, id)
		in.ExpectedVersion++
		before := f.domain(t, id)
		lookup, err := f.engine.FindReceiptMatching(ctx, users[0], intent, future)
		botGamesPGCheck(t, err, "lookup before due command")
		assertBotRouletteLookup(t, lookup, "UNKNOWN", nil)
		_, err = f.engine.CommandBefore(ctx, users[0], id, in, past)
		if !errors.Is(err, roulette.ErrQuoteExpired) || !reflect.DeepEqual(before, f.domain(t, id)) {
			t.Fatalf("expired command or lookup advanced due: %v", err)
		}
		_, err = f.engine.CommandBefore(ctx, users[0], id, in, future)
		if !errors.Is(err, roulette.ErrVersionConflict) || f.view(t, users[0], id).State != "SETTLING" {
			t.Fatalf("existing due handoff changed: %v", err)
		}
		before = f.domain(t, id)
		lookup, err = f.engine.FindReceiptMatching(ctx, users[0], intent, past)
		botGamesPGCheck(t, err, "absent receipt after due")
		assertBotRouletteLookup(t, lookup, "ABSENT_FINAL", nil)
		if !reflect.DeepEqual(before, f.domain(t, id)) {
			t.Fatal("lookup finished pending settlement")
		}
	})
	t.Run("settling_receipt_survives_handoff", func(t *testing.T) {
		id, users := f.room(t, "devil-roulette", false)
		in := roulette.Command{Key: f.key(), ExpectedVersion: f.view(t, users[0], id).Version, Action: roulette.Action{Kind: "CANCEL"}}
		original, err := f.engine.CommandBefore(ctx, users[0], id, in, future)
		botGamesPGCheck(t, err, "guarded cancellation")
		if original.State != "SETTLING" || f.view(t, users[0], id).State != "CANCELLED" {
			t.Fatal("existing post-commit settlement handoff changed")
		}
		lookup, err := f.engine.FindReceiptMatching(ctx, users[0], roulette.BotIntent{Purpose: "COMMAND", Game: "devil-roulette", RoundID: id, Command: &in}, past)
		botGamesPGCheck(t, err, "original settling receipt")
		assertBotRouletteLookup(t, lookup, "APPLIED", &original)
	})
}

type botRouletteCall struct {
	receipt roulette.Receipt
	lookup  roulette.BotLookup
	err     error
}

// The gate is a real independent transaction, released explicitly by the test.
// Table locking pauses a call after its actor/key lock but before receipt read;
// advisory locking pauses it before that lock is acquired. Neither uses sleeps.
func (f *botRoulettePGFixture) gate(t *testing.T, ctx context.Context, sql string, args ...any) pgx.Tx {
	t.Helper()
	conn, err := pgx.Connect(ctx, f.base.connection.OwnerURL)
	botGamesPGCheck(t, err, "gate connection")
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	tx, err := conn.Begin(ctx)
	botGamesPGCheck(t, err, "gate transaction")
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	_, err = tx.Exec(ctx, sql, args...)
	botGamesPGCheck(t, err, "acquire gate")
	return tx
}

func (f *botRoulettePGFixture) awaitDB(t *testing.T, ctx context.Context, sql string, args ...any) {
	t.Helper()
	for {
		var ready bool
		err := f.base.owner.WithTx(ctx, func(tx pgx.Tx) error { return tx.QueryRow(ctx, sql, args...).Scan(&ready) })
		botGamesPGCheck(t, err, "await observed database condition")
		if ready {
			return
		}
	}
}

func (f *botRoulettePGFixture) awaitCommandLock(t *testing.T, ctx context.Context, key string, granted bool) {
	t.Helper()
	f.awaitDB(t, ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks a WHERE a.locktype='advisory'
	 AND a.database=(SELECT oid FROM pg_database WHERE datname=current_database())
	 AND a.classid=((hashtextextended($1,0)>>32)&4294967295)::oid
	 AND a.objid=(hashtextextended($1,0)&4294967295)::oid AND a.objsubid=1 AND a.granted=$2
	 AND (NOT $2 OR EXISTS(SELECT 1 FROM pg_locks r WHERE r.pid=a.pid AND r.relation='roulette.commands'::regclass AND NOT r.granted)))`, key, granted)
}

func awaitBotRouletteCall(t *testing.T, ctx context.Context, result <-chan botRouletteCall) botRouletteCall {
	t.Helper()
	select {
	case got := <-result:
		return got
	case <-ctx.Done():
		t.Fatal("ordered roulette call did not finish")
		return botRouletteCall{}
	}
}

func botRoulettePGHTTPConfig(t *testing.T, f *botRoulettePGFixture, enabled string) config {
	t.Helper()
	dir, err := os.MkdirTemp("", "br-")
	botGamesPGCheck(t, err, "roulette socket directory")
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	cfg := config{ProcessRole: "platform", WalletDSNFile: filepath.Join(dir, "wallet"), GameFairnessKeyringFile: filepath.Join(dir, "fairness"), accessDeclaration: f.base.declaration, roulette: f.engine}
	values := map[string]string{
		"MOMIAO_BOT_GAMES_SOCKET":          filepath.Join(dir, "bot.sock"),
		"MOMIAO_BOT_GAMES_TOKEN_FILE":      filepath.Join(dir, "token"),
		"MOMIAO_BOT_GAMES_QUOTE_KEY_FILE":  filepath.Join(dir, "quote"),
		"MOMIAO_BOT_GAMES_NATIVE_DSN_FILE": filepath.Join(dir, "native"),
		"MOMIAO_BOT_ROULETTE_ENABLED":      enabled,
	}
	botGamesPGCheck(t, loadBotGamesConfig(&cfg, func(k string) (string, bool) { v, ok := values[k]; return v, ok }), "roulette config")
	quoteKey, fairnessKey := [32]byte{2}, [32]byte{9}
	for p, value := range map[string]string{
		cfg.BotGames.TokenFile:      botGamesTestToken,
		cfg.BotGames.QuoteKeyFile:   hex.EncodeToString(quoteKey[:]),
		cfg.BotGames.NativeDSNFile:  f.base.connection.NativeURL,
		cfg.GameFairnessKeyringFile: `{"active":"bot-fixture-v1","keys":{"bot-fixture-v1":"` + hex.EncodeToString(fairnessKey[:]) + `"}}`,
	} {
		botGamesPGCheck(t, os.WriteFile(p, []byte(value), 0600), "roulette private fixture file")
	}
	return cfg
}

func botRoulettePGHTTPClient(t *testing.T, f *botRoulettePGFixture, cfg config) *http.Client {
	t.Helper()
	app, err := openBotGamesApplication(context.Background(), cfg, f.base.runtime, f.base.engine)
	botGamesPGCheck(t, err, "roulette private application")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, time.Second) }()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", cfg.BotGames.Socket)
	}}
	t.Cleanup(func() {
		transport.CloseIdleConnections()
		cancel()
		botGamesPGCheck(t, <-done, "roulette application shutdown")
		_ = app.Close()
	})
	return &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func botRoulettePGPost(t *testing.T, client *http.Client, path, subject string, input any, want int) []byte {
	t.Helper()
	raw, err := json.Marshal(input)
	botGamesPGCheck(t, err, "roulette request JSON")
	r := botGamesRequest("POST", "http://private"+path, string(raw))
	r.RequestURI = ""
	r.Header.Del("X-Discord-User")
	if subject != "" {
		r.Header.Set("X-Discord-User", subject)
	}
	response, err := client.Do(r)
	botGamesPGCheck(t, err, "roulette socket request")
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	botGamesPGCheck(t, err, "roulette socket body")
	if response.StatusCode != want || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Set-Cookie") != "" {
		t.Fatalf("private route %s status=%d want=%d body=%s", path, response.StatusCode, want, body)
	}
	return body
}

func TestBotRouletteHTTPPostgres(t *testing.T) {
	f := newBotRoulettePGFixture(t)
	f.base.exec(t, `UPDATE games.game_registry SET publication_state='PUBLISHED' WHERE game_slug='pressure-roulette'`)
	t.Run("requires_domain_service", func(t *testing.T) {
		cfg := botRoulettePGHTTPConfig(t, f, "1")
		cfg.roulette = nil
		if app, err := openBotGamesApplication(context.Background(), cfg, f.base.runtime, f.base.engine); err == nil {
			_ = app.Close()
			t.Fatal("enabled roulette listener accepted absent domain service")
		}
		if _, err := os.Stat(cfg.BotGames.Socket); !os.IsNotExist(err) {
			t.Fatal("failed startup left private socket")
		}
	})
	client := botRoulettePGHTTPClient(t, f, botRoulettePGHTTPConfig(t, f, "1"))
	const prefix = "/internal/v1/bot-roulette/"
	body := botRoulettePGPost(t, client, prefix+"public", "", map[string]any{}, 400)
	if string(body) != `{"schema_version":"1","error":"INVALID_REQUEST"}`+"\n" {
		t.Fatalf("probe incompatible: %s", body)
	}
	t.Run("socket_path_is_not_cleaned_or_redirected", func(t *testing.T) {
		for path, status := range map[string]int{"public?": 400, "public?x=1": 400, "%70ublic": 400, "/public": 404, "../bot-roulette/public": 404, "public/": 404, "probe": 404} {
			botRoulettePGPost(t, client, prefix+path, "", map[string]any{}, status)
		}
	})
	for _, game := range []string{"devil-roulette", "pressure-roulette"} {
		t.Run(game, func(t *testing.T) {
			count := 2
			if game == "pressure-roulette" {
				count = 3
			}
			users, subjects := make([]int64, count), make([]string, count)
			for i := range users {
				users[i], subjects[i] = f.base.player(t, true, 50000000)
			}
			freshID := func() string {
				f.base.next++
				return strconv.FormatUint(uint64(time.Now().UnixMilli()-1420070400000)<<22|uint64(f.base.next&0x3fffff), 10)
			}
			prepare := func(subject string, in botroulette.PrepareInput) botroulette.PreparedReply {
				in.RequestID = freshID()
				var p botroulette.PreparedReply
				botGamesPGCheck(t, json.Unmarshal(botRoulettePGPost(t, client, prefix+"prepare", subject, in, 200), &p), "HTTP prepare")
				if p.SchemaVersion != "1" || p.RequestID != in.RequestID || p.Quote == "" {
					t.Fatal("prepare identity lost")
				}
				return p
			}
			commit := func(subject string, p botroulette.PreparedReply) botroulette.ReceiptReply {
				var result botroulette.ReceiptReply
				botGamesPGCheck(t, json.Unmarshal(botRoulettePGPost(t, client, prefix+"commit", subject, botroulette.QuoteInput{Quote: p.Quote}, 200), &result), "HTTP commit")
				if result.SchemaVersion != "1" || result.Status != "APPLIED" || result.Receipt == nil {
					t.Fatal("commit receipt absent")
				}
				return result
			}
			p := prepare(subjects[0], botroulette.PrepareInput{Purpose: "CREATE", Game: game, Input: botroulette.TypedInput{Create: &botroulette.CreateInput{Stake: "10", Players: count}}})
			created := commit(subjects[0], p)
			id := created.Receipt.RoundID
			for i := 1; i < count; i++ {
				v := f.view(t, users[i], id)
				commit(subjects[i], prepare(subjects[i], botroulette.PrepareInput{Purpose: "COMMAND", Game: game, RoomID: &id, Input: botroulette.TypedInput{Command: &botroulette.CommandInput{ExpectedVersion: v.Version, Action: botroulette.Action{Kind: "JOIN"}}}}))
			}
			for _, user := range users {
				if f.base.balance(t, user) != 50000000 {
					t.Fatal("CREATE or JOIN charged chips")
				}
			}
			for i, user := range users {
				v := f.view(t, user, id)
				in := botroulette.CommandInput{ExpectedVersion: v.Version, Action: botroulette.Action{Kind: "READY"}, Ready: &botroulette.ReadyConfirmation{ClientSeed: "http-fixture-seed", ConfigHash: v.Binding.ConfigHash, PolicyHash: v.Binding.PolicyHash, ServerSeedHash: v.ServerSeedHash, StakeUnits: v.StakeUnits}}
				commit(subjects[i], prepare(subjects[i], botroulette.PrepareInput{Purpose: "COMMAND", Game: game, RoomID: &id, Input: botroulette.TypedInput{Command: &in}}))
				if f.base.balance(t, user) != 45000000 {
					t.Fatal("confirmed READY did not escrow exactly once")
				}
			}
			var state botroulette.StateReply
			botGamesPGCheck(t, json.Unmarshal(botRoulettePGPost(t, client, prefix+"state", subjects[0], botroulette.StateInput{RoomID: id}, 200), &state), "HTTP private state")
			if state.Room.ID != id || state.Room.Game != game || state.Room.State != "PLAYING" || state.Self == nil || state.OwnRoundID == nil || *state.OwnRoundID != id {
				t.Fatal("private state differs from original browser room")
			}
			before := f.domain(t, id)
			if again := commit(subjects[0], p); !reflect.DeepEqual(again, created) {
				t.Fatal("commit returned current state instead of original CREATE receipt")
			}
			var lookup botroulette.ReceiptReply
			botGamesPGCheck(t, json.Unmarshal(botRoulettePGPost(t, client, prefix+"lookup", subjects[0], botroulette.QuoteInput{Quote: p.Quote}, 200), &lookup), "HTTP lookup")
			if !reflect.DeepEqual(lookup, created) || !reflect.DeepEqual(before, f.domain(t, id)) {
				t.Fatal("receipt replay mutated original room")
			}
			f.base.exec(t, `UPDATE public.users SET discord_id='' WHERE id=$1`, users[0])
			before = f.domain(t, id)
			var public botroulette.PublicReply
			botGamesPGCheck(t, json.Unmarshal(botRoulettePGPost(t, client, prefix+"public", "", botroulette.RoomInput{Game: game, RoomID: id}, 200), &public), "service-only public view")
			if !reflect.DeepEqual(public.Room, state.Room) { // server_now is a fresh read, not a room mutation.
				public.Room.ServerNow = state.Room.ServerNow
				if !reflect.DeepEqual(public.Room, state.Room) {
					t.Fatal("service public view changed after host unlinked")
				}
			}
			for route, in := range map[string]any{"lobby": botroulette.LobbyInput{Game: game}, "state": botroulette.StateInput{RoomID: id}, "prepare": botroulette.PrepareInput{RequestID: freshID(), Purpose: "CREATE", Game: game, Input: botroulette.TypedInput{Create: &botroulette.CreateInput{Stake: "10", Players: count}}}, "commit": botroulette.QuoteInput{Quote: p.Quote}, "lookup": botroulette.QuoteInput{Quote: p.Quote}} {
				if got := botRoulettePGPost(t, client, prefix+route, subjects[0], in, 404); string(got) != `{"schema_version":"1","error":"NOT_LINKED"}`+"\n" {
					t.Fatalf("unlinked personal route %s accepted", route)
				}
			}
			if !reflect.DeepEqual(before, f.domain(t, id)) {
				t.Fatal("unlinked display/personal rejection mutated room or wallet")
			}
			t.Logf("SOCKET_FLOW %s: CREATE/JOIN free; READY escrow; PLAYING private state; original receipt replay; public after unlink; five personal routes rejected", game)
		})
	}
	t.Run("disabled_adapter_preserves_existing_routes_and_worker", func(t *testing.T) {
		cfg := botRoulettePGHTTPConfig(t, f, "0")
		disabled := botRoulettePGHTTPClient(t, f, cfg)
		for _, route := range []string{"lobby", "public", "state", "prepare", "commit", "lookup"} {
			botRoulettePGPost(t, disabled, prefix+route, "", map[string]any{}, 404)
		}
		for game, in := range map[string]any{
			"dice":      map[string]string{"request_id": "1", "wager": "10", "choice": "BIG"},
			"slot":      map[string]string{"request_id": "1", "total_wager": "10"},
			"summon":    map[string]string{"request_id": "1", "base_wager": "10", "mode": "SINGLE"},
			"scratch":   map[string]string{"request_id": "1", "wager": "10"},
			"blackjack": map[string]string{"request_id": "1", "initial_wager": "10"},
		} {
			for _, c := range []*http.Client{client, disabled} {
				if got := botRoulettePGPost(t, c, "/internal/v1/bot-games/"+game+"/prepare", "18446744073709551615", in, 404); string(got) != `{"error":"NOT_LINKED"}`+"\n" {
					t.Fatal("legacy response contract changed")
				}
			}
		}
		id, users := f.room(t, "devil-roulette", false)
		f.base.exec(t, `UPDATE roulette.rounds SET deadline=clock_timestamp()-interval '1 second',version=version+1 WHERE round_id=$1`, id)
		_, err := f.worker.StepDue(context.Background(), 50)
		botGamesPGCheck(t, err, "disabled adapter browser worker")
		if cfg.roulette != f.engine || f.view(t, users[0], id).State != "CANCELLED" || f.base.balance(t, users[0]) != 50000000 {
			t.Fatal("adapter switch disabled browser expiry/refund")
		}
	})
}

func TestBotRouletteExpiryRacePG(t *testing.T) {
	f := newBotRoulettePGFixture(t)
	for _, purpose := range []string{"CREATE", "READY"} {
		for _, ordering := range []string{"commit_first", "lookup_first_expired", "lookup_first_pending", "commit_wait_expires", "lookup_wait_expires", "lookup_timeout"} {
			t.Run(purpose+"/"+ordering, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				t.Cleanup(cancel)
				user, _ := f.base.player(t, true, 50000000)
				create := roulette.CreateRequest{Key: f.key(), Game: "devil-roulette", Stake: "10", Players: 2}
				intent := roulette.BotIntent{Purpose: "CREATE", Game: create.Game, Create: &create}
				key := create.Key
				var before json.RawMessage
				if purpose == "READY" {
					id, users := f.room(t, "devil-roulette", false)
					user = users[1]
					in := f.readyInput(t, user, id)
					intent = roulette.BotIntent{Purpose: "COMMAND", Game: create.Game, RoundID: id, Command: &in}
					key, before = in.Key, f.domain(t, id)
				}
				hash := sha256.Sum256([]byte(key))
				lockKey := "roulette/command/" + strconv.FormatInt(user, 10) + "/" + hex.EncodeToString(hash[:])
				expires := time.Now().Add(2 * time.Minute)
				if ordering == "lookup_first_expired" {
					expires = time.Now().Add(-time.Minute)
				}
				commit := func() botRouletteCall {
					var receipt roulette.Receipt
					var err error
					if intent.Create != nil {
						receipt, err = f.engine.CreateBefore(ctx, user, *intent.Create, expires)
					} else {
						receipt, err = f.engine.CommandBefore(ctx, user, intent.RoundID, *intent.Command, expires)
					}
					return botRouletteCall{receipt: receipt, err: err}
				}
				lookup := func(callCtx context.Context) botRouletteCall {
					got, err := f.engine.FindReceiptMatching(callCtx, user, intent, expires)
					return botRouletteCall{lookup: got, err: err}
				}
				committed, lookedUp := make(chan botRouletteCall, 1), make(chan botRouletteCall, 1)
				var c, l botRouletteCall
				switch ordering {
				case "commit_first", "lookup_first_expired", "lookup_first_pending":
					gate := f.gate(t, ctx, `LOCK TABLE roulette.commands IN ACCESS EXCLUSIVE MODE`)
					if ordering == "commit_first" {
						go func() { committed <- commit() }()
						f.awaitCommandLock(t, ctx, lockKey, true)
						go func() { lookedUp <- lookup(ctx) }()
					} else {
						go func() { lookedUp <- lookup(ctx) }()
						f.awaitCommandLock(t, ctx, lockKey, true)
						go func() { committed <- commit() }()
					}
					f.awaitCommandLock(t, ctx, lockKey, false)
					botGamesPGCheck(t, gate.Commit(ctx), "release receipt-read gate")
					c, l = awaitBotRouletteCall(t, ctx, committed), awaitBotRouletteCall(t, ctx, lookedUp)
				case "commit_wait_expires", "lookup_wait_expires":
					gate := f.gate(t, ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey)
					botGamesPGCheck(t, gate.QueryRow(ctx, `SELECT clock_timestamp()+interval '200 milliseconds'`).Scan(&expires), "DB expiry")
					if ordering == "commit_wait_expires" {
						go func() { committed <- commit() }()
					} else {
						go func() { lookedUp <- lookup(ctx) }()
					}
					f.awaitCommandLock(t, ctx, lockKey, false)
					// Observe expiry on the database clock, not a scheduled sleep.
					f.awaitDB(t, ctx, `SELECT clock_timestamp() >= $1`, expires)
					botGamesPGCheck(t, gate.Commit(ctx), "release expired actor/key gate")
					if ordering == "commit_wait_expires" {
						c, l = awaitBotRouletteCall(t, ctx, committed), lookup(ctx)
					} else {
						l, c = awaitBotRouletteCall(t, ctx, lookedUp), commit()
					}
				case "lookup_timeout":
					gate := f.gate(t, ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey)
					timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 500*time.Millisecond)
					defer timeoutCancel()
					go func() { lookedUp <- lookup(timeoutCtx) }()
					f.awaitCommandLock(t, ctx, lockKey, false)
					l = awaitBotRouletteCall(t, ctx, lookedUp)
					if !errors.Is(l.err, context.DeadlineExceeded) || l.lookup.Status != "" || l.lookup.Receipt != nil {
						t.Fatalf("timeout fabricated absence: lookup=%+v err=%v", l.lookup, l.err)
					}
					botGamesPGCheck(t, gate.Commit(ctx), "release timeout gate")
					c = commit()
				}
				if ordering != "lookup_timeout" {
					botGamesPGCheck(t, l.err, "ordered lookup")
				}
				expired := ordering == "lookup_first_expired" || ordering == "commit_wait_expires" || ordering == "lookup_wait_expires"
				if expired {
					assertBotRouletteLookup(t, l.lookup, "ABSENT_FINAL", nil)
					if !errors.Is(c.err, roulette.ErrQuoteExpired) {
						t.Fatalf("late request overtook final absence: %v", c.err)
					}
					if purpose == "READY" {
						if !reflect.DeepEqual(before, f.domain(t, intent.RoundID)) {
							t.Fatal("expired READY changed room or wallet")
						}
					} else {
						var count int
						err := f.base.owner.WithTx(ctx, func(tx pgx.Tx) error {
							return tx.QueryRow(ctx, `SELECT count(*) FROM roulette.rounds WHERE host_user_id=$1`, user).Scan(&count)
						})
						botGamesPGCheck(t, err, "expired CREATE effects")
						if count != 0 {
							t.Fatal("expired CREATE made a room")
						}
					}
				} else {
					botGamesPGCheck(t, c.err, "ordered commit")
					if ordering == "commit_first" {
						assertBotRouletteLookup(t, l.lookup, "APPLIED", &c.receipt)
					} else if ordering == "lookup_first_pending" {
						assertBotRouletteLookup(t, l.lookup, "UNKNOWN", nil)
					}
					final := lookup(ctx)
					botGamesPGCheck(t, final.err, "eventual original lookup")
					assertBotRouletteLookup(t, final.lookup, "APPLIED", &c.receipt)
				}
				t.Logf("ORDERED %s/%s: lookup=%q commit_error=%v; lock order observed in pg_locks", purpose, ordering, l.lookup.Status, c.err)
			})
		}
	}
}
