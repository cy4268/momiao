package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/roulette"
	"github.com/jackc/pgx/v5"
)

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
