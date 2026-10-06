package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/botgames"
	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

func requireFreshScratchFixture(t *testing.T) {
	t.Helper()
	path := os.Getenv("MOMIAO_GAMES_TEST_CONNECTION_FILE")
	if path == "" {
		t.Skip("fresh isolated local scratch G1 connection required")
	}
	raw, e := os.ReadFile(path)
	var c struct{ OwnerURL, RuntimeURL, NativeURL string }
	if e != nil || json.Unmarshal(raw, &c) != nil {
		t.Fatal("scratch fixture unreadable")
	}
	var database string
	for _, url := range []string{c.OwnerURL, c.RuntimeURL, c.NativeURL} {
		config, e := pgx.ParseConfig(url)
		if e != nil || config.Host != "127.0.0.1" || config.Port != 55432 || !strings.HasPrefix(config.Database, "momiao_test_g1_original_scratch_") {
			t.Fatal("refusing non-scratch loopback fixture")
		}
		if database != "" && database != config.Database {
			t.Fatal("split fixture databases")
		}
		database = config.Database
	}
	conn, e := pgx.Connect(context.Background(), c.OwnerURL)
	botGamesPGCheck(t, e, "fresh scratch guard")
	defer conn.Close(context.Background())
	var fresh bool
	e = conn.QueryRow(context.Background(), `SELECT NOT EXISTS(SELECT FROM pg_namespace WHERE nspname IN ('games','economy','identity')) AND to_regclass('public.users') IS NULL`).Scan(&fresh)
	botGamesPGCheck(t, e, "empty scratch database")
	if !fresh {
		t.Fatal("scratch integration requires a NEW empty database; preserve prior evidence")
	}
}
func (f *botGamesPGFixture) prepareScratch(t *testing.T, subject, wager string) botgames.ScratchPrepared {
	t.Helper()
	f.next++
	p, e := f.service.PrepareScratch(context.Background(), subject, botgames.ScratchPrepareInput{RequestID: strconv.FormatInt(f.next, 10), Wager: wager})
	botGamesPGCheck(t, e, "scratch prepare")
	return p
}
func (f *botGamesPGFixture) scratchActions(t *testing.T, round string, want int) {
	t.Helper()
	var count int
	e := f.owner.WithTx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM games.round_actions WHERE round_id=$1 AND action_type='SCRATCH_REVEAL_COMPLETE'`, round).Scan(&count)
	})
	botGamesPGCheck(t, e, "scratch action count")
	if count != want {
		t.Fatalf("reveal actions=%d want=%d", count, want)
	}
}

func (f *botGamesPGFixture) scratchLedger(t *testing.T, user int64, r botgames.ScratchResult) {
	t.Helper()
	var rounds, cells, wagers, settlements, actions int
	var stake, credit, supply int64
	var state string
	e := f.owner.WithTx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT c.state,
 (SELECT count(*) FROM games.game_rounds WHERE newapi_user_id=$1),
 (SELECT count(*) FROM games.scratch_cells WHERE round_id=$2),
 (SELECT count(*) FROM economy.asset_transactions WHERE newapi_user_id=$1 AND biz_type='GAME_WAGER'),
 (SELECT count(*) FROM economy.asset_transactions WHERE newapi_user_id=$1 AND biz_type='GAME_SETTLEMENT'),
 (SELECT count(*) FROM games.round_actions WHERE round_id=$2 AND action_type='SCRATCH_REVEAL_COMPLETE'),
 (SELECT coalesce(sum(l.delta_units),0)::bigint FROM economy.wallet_ledger l JOIN economy.asset_transactions a ON a.transaction_id=l.transaction_id WHERE a.newapi_user_id=$1 AND a.biz_type='GAME_WAGER'),
 (SELECT coalesce(sum(l.delta_units),0)::bigint FROM economy.wallet_ledger l JOIN economy.asset_transactions a ON a.transaction_id=l.transaction_id WHERE a.newapi_user_id=$1 AND a.biz_type='GAME_SETTLEMENT'),
 (SELECT coalesce(sum(CASE WHEN direction='ISSUE' THEN amount_units ELSE -amount_units END),0)::bigint FROM games.supply_events WHERE round_id=$2)
 FROM games.game_rounds r JOIN games.fairness_commitments c ON c.commitment_id=r.commitment_id WHERE r.round_id=$2 AND r.newapi_user_id=$1`, user, r.RoundID).Scan(&state, &rounds, &cells, &wagers, &settlements, &actions, &stake, &credit, &supply)
	})
	botGamesPGCheck(t, e, "scratch durable ledger")
	wantStake, _ := strconv.ParseInt(r.StakeUnits, 10, 64)
	wantCredit, _ := strconv.ParseInt(r.CreditedPayoutUnits, 10, 64)
	wantNet, _ := strconv.ParseInt(r.ActualNetUnits, 10, 64)
	if state != "REVEALED" || rounds != 1 || cells != 9 || wagers != 1 || settlements != 1 || actions != 1 || stake != -wantStake || credit != wantCredit || supply != wantNet {
		t.Fatalf("scratch ledger mismatch state=%s rounds=%d cells=%d wagers=%d settlements=%d actions=%d stake=%d credit=%d supply=%d", state, rounds, cells, wagers, settlements, actions, stake, credit, supply)
	}
}
func (f *botGamesPGFixture) websiteScratch(t *testing.T, user int64, subject, wager string) games.GameRound {
	t.Helper()
	b, e := f.engine.Bootstrap(context.Background(), user, "scratch")
	botGamesPGCheck(t, e, "website scratch bootstrap")
	f.next++
	r, e := f.engine.Create(context.Background(), user, "scratch", "scratch-website:"+strconv.FormatInt(f.next, 10)+":"+subject, b.Next.ID, games.CreateInput{Type: "SCRATCH", Wager: wager})
	botGamesPGCheck(t, e, "website scratch purchase")
	return r
}
func (f *botGamesPGFixture) restartScratch(t *testing.T) *botgames.Service {
	t.Helper()
	engine, e := games.NewService(f.runtime, games.Keyring{Active: "bot-fixture-v1", Keys: map[string][32]byte{"bot-fixture-v1": {9}}})
	botGamesPGCheck(t, e, "restart scratch engine")
	s, e := botgames.NewService(engine, botGamesResolver{native: f.native, platform: f.runtime, declaration: f.declaration}, [32]byte{2}, func() time.Time { return f.now })
	botGamesPGCheck(t, e, "restart scratch bridge")
	return s
}

// Hold only the bridge's completion call, after its original-ticket read.
// The website then commits first under the real per-user scratch lock. Releasing
// the bridge deterministically reproduces that cross-channel race ordering.
type scratchRevealBarrier struct {
	*games.Service
	entered chan struct{}
	release chan struct{}
}

func (g *scratchRevealBarrier) RevealComplete(ctx context.Context, user int64, id, action string) (games.GameRound, error) {
	close(g.entered)
	<-g.release
	return g.Service.RevealComplete(ctx, user, id, action)
}

func TestBotScratchPostgres(t *testing.T) {
	requireFreshScratchFixture(t)
	f := newBotGamesPGFixture(t)
	ctx := context.Background()
	f.exec(t, `GRANT SELECT,INSERT ON games.slot_results,games.slot_line_results TO `+pgx.Identifier{f.connection.RuntimeRole}.Sanitize())
	t.Run("website_and_bridge_reveal_race", func(t *testing.T) {
		user, subject := f.player(t, true, 550000000)
		boot, e := f.engine.Bootstrap(ctx, user, "scratch")
		botGamesPGCheck(t, e, "website bootstrap")
		old, e := f.engine.Create(ctx, user, "scratch", "scratch-website:"+subject, boot.Next.ID, games.CreateInput{Type: "SCRATCH", Wager: "11"})
		botGamesPGCheck(t, e, "website scratch purchase")
		p := f.prepareScratch(t, subject, "99")
		barrier := &scratchRevealBarrier{Service: f.engine, entered: make(chan struct{}), release: make(chan struct{})}
		s, e := botgames.NewService(barrier, botGamesResolver{native: f.native, platform: f.runtime, declaration: f.declaration}, [32]byte{2}, func() time.Time { return f.now })
		botGamesPGCheck(t, e, "barrier bridge")
		before := f.balance(t, user)
		done := make(chan error, 1)
		go func() { _, e := s.PlayScratch(ctx, subject, p.Quote); done <- e }()
		select {
		case <-barrier.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("bridge did not reach presentation")
		}
		_, e = f.engine.RevealComplete(ctx, user, old.ID, "00000000-0000-7000-8000-000000000901")
		close(barrier.release)
		botGamesPGCheck(t, e, "website reveal")
		botGamesPGCheck(t, <-done, "bridge raced reveal")
		if f.balance(t, user) != before {
			t.Fatal("presentation race changed wallet")
		}
		f.effects(t, user, 1)
		f.scratchActions(t, old.ID, 1)
	})
	t.Run("cross_round_action_conflict_even_after_completion", func(t *testing.T) {
		user, subject := f.player(t, true, 550000000)
		first := f.websiteScratch(t, user, subject, "11")
		const a = "00000000-0000-7000-8000-000000000902"
		const b = "00000000-0000-7000-8000-000000000903"
		_, e := f.engine.RevealComplete(ctx, user, first.ID, a)
		botGamesPGCheck(t, e, "first website reveal")
		second := f.websiteScratch(t, user, subject, "11")
		_, e = f.engine.RevealComplete(ctx, user, second.ID, b)
		botGamesPGCheck(t, e, "second website reveal")
		before := f.balance(t, user)
		_, e = f.engine.RevealComplete(ctx, user, second.ID, a)
		if !errors.Is(e, platform.ErrIdempotencyConflict) {
			t.Fatal("cross-round action collision accepted")
		}
		_, e = f.engine.RevealComplete(ctx, user, second.ID, "00000000-0000-7000-8000-000000000904")
		botGamesPGCheck(t, e, "completed different action no-op")
		f.scratchActions(t, first.ID, 1)
		f.scratchActions(t, second.ID, 1)
		f.effects(t, user, 2)
		if f.balance(t, user) != before {
			t.Fatal("completion changed wallet")
		}
	})
	for _, wager := range []string{"10", "11"} {
		t.Run("real_http_purchase_"+wager, func(t *testing.T) {
			user, subject := f.player(t, true, 550000000)
			h, e := newBotGamesHandler(f.service, botGamesTestToken)
			botGamesPGCheck(t, e, "scratch HTTP handler")
			request := botGamesRequest("POST", "/internal/v1/bot-games/scratch/prepare", fmt.Sprintf(`{"request_id":%q,"wager":%q}`, subject, wager))
			request.Header.Set("X-Discord-User", subject)
			response := httptest.NewRecorder()
			h.ServeHTTP(response, request)
			if response.Code != 200 {
				t.Fatal(response.Code, response.Body.String())
			}
			var p botgames.ScratchPrepared
			botGamesPGCheck(t, json.Unmarshal(response.Body.Bytes(), &p), "scratch prepare response")
			if p.Action != "PURCHASE" || p.Wager != wager || p.AdditionalStakeUnits != p.StakeUnits || len(p.Prizes) != 8 {
				t.Fatal("purchase preview mismatch")
			}
			f.effects(t, user, 0)
			var first botgames.ScratchResult
			before := f.balance(t, user)
			for _, op := range []string{"play", "lookup", "play"} {
				body, _ := json.Marshal(map[string]string{"quote": p.Quote})
				request = botGamesRequest("POST", "/internal/v1/bot-games/scratch/"+op, string(body))
				request.Header.Set("X-Discord-User", subject)
				response = httptest.NewRecorder()
				h.ServeHTTP(response, request)
				if response.Code != 200 {
					t.Fatal(response.Code, response.Body.String())
				}
				var r botgames.ScratchResult
				botGamesPGCheck(t, json.Unmarshal(response.Body.Bytes(), &r), "scratch result response")
				if r.Action != "PURCHASE" || r.Wager != wager || r.Status != "SETTLED" || r.PresentationCompletedAt == "" {
					t.Fatal("incomplete HTTP result")
				}
				if first.RoundID != "" && !reflect.DeepEqual(first, r) {
					t.Fatal("replayed HTTP result changed")
				}
				first = r
				var fields map[string]json.RawMessage
				json.Unmarshal(response.Body.Bytes(), &fields)
				for _, name := range strings.Fields("action round_id created_at settled_at presentation_completed_at status wager cells prize_tier multiplier result stake_units gross_payout_units withheld_units credited_payout_units actual_net_units ruleset") {
					if _, ok := fields[name]; !ok {
						t.Fatal("missing result field", name)
					}
					delete(fields, name)
				}
				if len(fields) != 0 {
					t.Fatal("private result leaked fields")
				}
			}
			net, _ := strconv.ParseInt(first.ActualNetUnits, 10, 64)
			if f.balance(t, user)-before != net {
				t.Fatal("HTTP accounting differs from wallet")
			}
			f.scratchLedger(t, user, first)
		})
	}
	t.Run("20_concurrent_confirmations_one_settlement_one_reveal_restart", func(t *testing.T) {
		user, subject := f.player(t, true, 550000000)
		p := f.prepareScratch(t, subject, "11")
		requestID := strconv.FormatInt(f.next, 10)
		before := f.balance(t, user)
		var wg sync.WaitGroup
		results := make(chan botgames.ScratchResult, 20)
		errs := make(chan error, 20)
		for range 20 {
			wg.Add(1)
			go func() { defer wg.Done(); r, e := f.service.PlayScratch(ctx, subject, p.Quote); results <- r; errs <- e }()
		}
		wg.Wait()
		close(results)
		close(errs)
		for e := range errs {
			botGamesPGCheck(t, e, "concurrent scratch")
		}
		var first botgames.ScratchResult
		for r := range results {
			if first.RoundID != "" && !reflect.DeepEqual(first, r) {
				t.Fatal("concurrent result changed")
			}
			first = r
		}
		net, _ := strconv.ParseInt(first.ActualNetUnits, 10, 64)
		if f.balance(t, user)-before != net {
			t.Fatal("duplicate settlement")
		}
		f.scratchLedger(t, user, first)
		conflict, e := f.service.PrepareScratch(ctx, subject, botgames.ScratchPrepareInput{RequestID: requestID, Wager: "10"})
		botGamesPGCheck(t, e, "conflicting purchase quote")
		_, e = f.service.PlayScratch(ctx, subject, conflict.Quote)
		botGamesWantFault(t, e, "IDEMPOTENCY_CONFLICT")
		f.now = f.now.Add(120 * time.Second)
		defer func() { f.now = f.now.Add(-120 * time.Second) }()
		restart := f.restartScratch(t)
		for _, fn := range []func(context.Context, string, string) (botgames.ScratchResult, error){restart.PlayScratch, restart.LookupScratch} {
			r, e := fn(ctx, subject, p.Quote)
			botGamesPGCheck(t, e, "expired saved recovery")
			if !reflect.DeepEqual(r, first) {
				t.Fatal("restart receipt changed")
			}
		}
		f.scratchLedger(t, user, first)
		f.exec(t, `UPDATE public.users SET status=2 WHERE id=$1`, user)
		_, e = restart.LookupScratch(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "ACCOUNT_RESTRICTED")
	})
	t.Run("website_original_resume_maintenance_different_wager_zero_money_delta", func(t *testing.T) {
		user, subject := f.player(t, true, 550000000)
		old := f.websiteScratch(t, user, subject, "11")
		before := f.balance(t, user)
		f.exec(t, `UPDATE games.runtime_gate SET maintenance=true WHERE singleton`)
		p := f.prepareScratch(t, subject, "99")
		f.exec(t, `UPDATE games.runtime_gate SET maintenance=false WHERE singleton`)
		if p.Action != "RESUME" || p.Wager != "11" || p.RoundID != old.ID || p.AdditionalStakeUnits != "0" || p.AvailableUnits != "" || p.CommitmentID != "" || p.Prizes != nil {
			t.Fatal("old ticket misrepresented as purchase", p)
		}
		f.scratchActions(t, old.ID, 0)
		r, e := f.service.PlayScratch(ctx, subject, p.Quote)
		botGamesPGCheck(t, e, "website resume")
		if r.Action != "RESUME" || r.RoundID != old.ID || r.Wager != "11" || f.balance(t, user) != before {
			t.Fatal("resume charged/switched")
		}
		f.scratchLedger(t, user, r)
		next := f.prepareScratch(t, subject, "10")
		if next.Action != "PURCHASE" {
			t.Fatal("completed original blocks next purchase")
		}
		newRound, e := f.service.PlayScratch(ctx, subject, next.Quote)
		botGamesPGCheck(t, e, "next purchase")
		if newRound.RoundID == old.ID {
			t.Fatal("next purchase reused ticket")
		}
		f.effects(t, user, 2)
		newer := f.websiteScratch(t, user, subject, "11")
		before = f.balance(t, user)
		recovered, e := f.service.LookupScratch(ctx, subject, p.Quote)
		botGamesPGCheck(t, e, "original-only resume recovery")
		if !reflect.DeepEqual(recovered, r) || f.balance(t, user) != before {
			t.Fatal("old quote followed newest ticket")
		}
		f.scratchActions(t, newer.ID, 0)
		f.effects(t, user, 3)
	})
	t.Run("expired_resume_unconfirmed_and_completed_recovery", func(t *testing.T) {
		user, subject := f.player(t, true, 550000000)
		old := f.websiteScratch(t, user, subject, "11")
		p := f.prepareScratch(t, subject, "99")
		before := f.balance(t, user)
		f.now = f.now.Add(120 * time.Second)
		defer func() { f.now = f.now.Add(-120 * time.Second) }()
		for _, fn := range []func(context.Context, string, string) (botgames.ScratchResult, error){f.service.PlayScratch, f.service.LookupScratch} {
			_, e := fn(ctx, subject, p.Quote)
			botGamesWantFault(t, e, "QUOTE_EXPIRED")
		}
		f.scratchActions(t, old.ID, 0)
		f.effects(t, user, 1)
		_, e := f.engine.RevealComplete(ctx, user, old.ID, "00000000-0000-7000-8000-000000000905")
		botGamesPGCheck(t, e, "website complete original")
		for _, fn := range []func(context.Context, string, string) (botgames.ScratchResult, error){f.service.PlayScratch, f.service.LookupScratch} {
			r, e := fn(ctx, subject, p.Quote)
			botGamesPGCheck(t, e, "expired completed resume")
			if r.RoundID != old.ID || r.Action != "RESUME" {
				t.Fatal("wrong old round")
			}
		}
		if f.balance(t, user) != before {
			t.Fatal("expired recovery charged")
		}
		f.scratchActions(t, old.ID, 1)
	})
	t.Run("presentation_transaction_failure_expired_purchase_restart_lookup", func(t *testing.T) {
		user, subject := f.player(t, true, 550000000)
		p := f.prepareScratch(t, subject, "11")
		key := "discord-scratch-v1:" + strconv.FormatInt(f.next, 10)
		f.exec(t, fmt.Sprintf(`CREATE FUNCTION games.scratch_fixture_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.newapi_user_id=%d THEN RAISE EXCEPTION 'scratch fixture presentation failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER scratch_fixture_failure BEFORE INSERT ON games.round_actions FOR EACH ROW EXECUTE FUNCTION games.scratch_fixture_failure()`, user), pgx.QueryExecModeSimpleProtocol)
		_, e := f.service.PlayScratch(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "UPSTREAM_UNAVAILABLE")
		original, e := f.engine.FindByKey(ctx, user, "scratch", key)
		botGamesPGCheck(t, e, "durable partial purchase")
		if original == nil || original.PresentationCompletedAt != nil {
			t.Fatal("settlement/reveal boundary fabricated")
		}
		f.effects(t, user, 1)
		f.scratchActions(t, original.ID, 0)
		before := f.balance(t, user)
		f.exec(t, `DROP TRIGGER scratch_fixture_failure ON games.round_actions; DROP FUNCTION games.scratch_fixture_failure()`, pgx.QueryExecModeSimpleProtocol)
		f.now = f.now.Add(121 * time.Second)
		defer func() { f.now = f.now.Add(-121 * time.Second) }()
		restart := f.restartScratch(t)
		r, e := restart.LookupScratch(ctx, subject, p.Quote)
		botGamesPGCheck(t, e, "restart original lookup completes presentation")
		if r.RoundID != original.ID || r.PresentationCompletedAt == "" || f.balance(t, user) != before {
			t.Fatal("partial recovery repurchased")
		}
		f.scratchLedger(t, user, r)
		next := f.prepareScratch(t, subject, "10")
		if next.Action != "PURCHASE" {
			t.Fatal("partial recovery retained blocker")
		}
	})
	t.Run("expired_absent_purchase_lookup_never_creates", func(t *testing.T) {
		user, subject := f.player(t, true, 550000000)
		p := f.prepareScratch(t, subject, "11")
		_, e := f.service.LookupScratch(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "NOT_FOUND")
		f.now = f.now.Add(120 * time.Second)
		defer func() { f.now = f.now.Add(-120 * time.Second) }()
		_, e = f.service.PlayScratch(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "QUOTE_EXPIRED")
		_, e = f.service.LookupScratch(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "NOT_FOUND")
		f.effects(t, user, 0)
	})
	t.Run("stale_purchase_does_not_switch_to_website_ticket", func(t *testing.T) {
		user, subject := f.player(t, true, 550000000)
		p := f.prepareScratch(t, subject, "11")
		old, e := f.engine.Create(ctx, user, "scratch", "scratch-raced-website:"+subject, p.CommitmentID, games.CreateInput{Type: "SCRATCH", Wager: "10"})
		botGamesPGCheck(t, e, "website raced purchase")
		before := f.balance(t, user)
		_, e = f.service.LookupScratch(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "NOT_FOUND")
		h, e := newBotGamesHandler(f.service, botGamesTestToken)
		botGamesPGCheck(t, e, "stale handler")
		body, _ := json.Marshal(map[string]string{"quote": p.Quote})
		request := botGamesRequest("POST", "/internal/v1/bot-games/scratch/play", string(body))
		request.Header.Set("X-Discord-User", subject)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, request)
		if w.Code != 409 || w.Body.String() != "{\"error\":\"SCRATCH_PREVIOUS_REVEAL_INCOMPLETE\"}\n" {
			t.Fatal(w.Code, w.Body.String())
		}
		if f.balance(t, user) != before {
			t.Fatal("stale purchase charged")
		}
		f.effects(t, user, 1)
		f.scratchActions(t, old.ID, 0)
	})
	t.Run("cross_binding_and_all_game_domains", func(t *testing.T) {
		user, subject := f.player(t, true, 550000000)
		p := f.prepareScratch(t, subject, "11")
		dice, e := f.service.Prepare(ctx, subject, botgames.PrepareInput{RequestID: subject, Wager: "10", Choice: "BIG"})
		botGamesPGCheck(t, e, "dice quote")
		slot, e := f.service.PrepareSlot(ctx, subject, botgames.SlotPrepareInput{RequestID: subject, TotalWager: "11"})
		botGamesPGCheck(t, e, "slot quote")
		summon, e := f.service.PrepareSummon(ctx, subject, botgames.SummonPrepareInput{RequestID: subject, BaseWager: "11", Mode: "SINGLE"})
		botGamesPGCheck(t, e, "summon quote")
		for _, q := range []string{dice.Quote, slot.Quote, summon.Quote} {
			_, e = f.service.PlayScratch(ctx, subject, q)
			botGamesWantFault(t, e, "UNAUTHORIZED")
			_, e = f.service.LookupScratch(ctx, subject, q)
			botGamesWantFault(t, e, "UNAUTHORIZED")
		}
		_, e = f.service.Play(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "UNAUTHORIZED")
		_, e = f.service.Lookup(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "UNAUTHORIZED")
		_, e = f.service.PlaySlot(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "UNAUTHORIZED")
		_, e = f.service.LookupSlot(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "UNAUTHORIZED")
		_, e = f.service.PlaySummon(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "UNAUTHORIZED")
		_, e = f.service.LookupSummon(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "UNAUTHORIZED")
		other, _ := f.player(t, true, 550000000)
		f.exec(t, `UPDATE public.users SET discord_id='' WHERE id=$1`, user)
		f.exec(t, `UPDATE public.users SET discord_id=$2 WHERE id=$1`, other, subject)
		_, e = f.service.PlayScratch(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "BINDING_CHANGED")
		_, e = f.service.LookupScratch(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "BINDING_CHANGED")
		f.effects(t, user, 0)
		f.effects(t, other, 0)
	})
	t.Run("malformed_stored_grid_rejected_before_reveal", func(t *testing.T) {
		user, subject := f.player(t, true, 550000000)
		old := f.websiteScratch(t, user, subject, "11")
		p := f.prepareScratch(t, subject, "99")
		before := f.balance(t, user)
		// Owner-only corruption injection in this disposable fixture. The normal
		// immutable-history guard rejected a plain UPDATE (SQLSTATE 55000).
		// Re-enable it in the same transaction before exercising the runtime.
		e := f.owner.WithTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `ALTER TABLE games.scratch_cells DISABLE TRIGGER immutable_history`); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE games.scratch_cells SET symbol='P1',is_matching_symbol=false WHERE round_id=$1`, old.ID); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `ALTER TABLE games.scratch_cells ENABLE TRIGGER immutable_history`)
			return err
		})
		botGamesPGCheck(t, e, "fixture corrupt stored grid")
		_, e = f.service.PrepareScratch(ctx, subject, botgames.ScratchPrepareInput{RequestID: subject, Wager: "99"})
		botGamesWantFault(t, e, "UPSTREAM_UNAVAILABLE")
		_, e = f.service.PlayScratch(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "UPSTREAM_UNAVAILABLE")
		_, e = f.service.LookupScratch(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "UPSTREAM_UNAVAILABLE")
		f.scratchActions(t, old.ID, 0)
		f.effects(t, user, 1)
		if f.balance(t, user) != before {
			t.Fatal("corrupt read changed wallet")
		}
	})
	t.Run("pre_rng_maximum_and_headroom", func(t *testing.T) {
		user, subject := f.player(t, true, math.MaxInt64-544500000)
		p := f.prepareScratch(t, subject, "11")
		if p.MaximumWagerUnits != "5500000" {
			t.Fatal("wrong pre-RNG maximum", p.MaximumWagerUnits)
		}
		_, e := f.service.PrepareScratch(ctx, subject, botgames.ScratchPrepareInput{RequestID: subject, Wager: "12"})
		botGamesWantFault(t, e, "INVALID_REQUEST")
		_, e = f.engine.Create(ctx, user, "scratch", "scratch-overflow:"+subject, p.CommitmentID, games.CreateInput{Type: "SCRATCH", Wager: "12"})
		if !errors.Is(e, platform.ErrBalanceOverflow) {
			t.Fatal("engine overflow not pre-RNG")
		}
		f.effects(t, user, 0)
		b, e := f.engine.Bootstrap(ctx, user, "scratch")
		botGamesPGCheck(t, e, "post-overflow commitment")
		if b.Next == nil || b.Next.ID != p.CommitmentID {
			t.Fatal("overflow consumed commitment")
		}
		_, e = f.service.PlayScratch(ctx, subject, p.Quote)
		botGamesPGCheck(t, e, "exact maximum play")
		f.effects(t, user, 1)
	})
	t.Run("transaction_rechecks_balance_and_maintenance", func(t *testing.T) {
		user, subject := f.player(t, true, 5500000)
		p := f.prepareScratch(t, subject, "11")
		_, e := f.owner.Apply(ctx, platform.Mutation{UserID: user, Asset: platform.AvailableChips, DeltaUnits: -500000, BizType: "BOT_TEST", BizID: "scratch-drain:" + subject, EntryType: "TEST_DRAIN", IdempotencyKey: "scratch-drain:" + subject})
		botGamesPGCheck(t, e, "drain wallet")
		_, e = f.service.PlayScratch(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "INSUFFICIENT_CHIPS")
		f.effects(t, user, 0)
		user, subject = f.player(t, true, 550000000)
		p = f.prepareScratch(t, subject, "11")
		f.exec(t, `UPDATE games.runtime_gate SET maintenance=true WHERE singleton`)
		defer f.exec(t, `UPDATE games.runtime_gate SET maintenance=false WHERE singleton`)
		_, e = f.service.PlayScratch(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "MAINTENANCE")
		_, e = f.service.PrepareScratch(ctx, subject, botgames.ScratchPrepareInput{RequestID: subject, Wager: "11"})
		botGamesWantFault(t, e, "MAINTENANCE")
		f.effects(t, user, 0)
	})
	t.Run("current_odds_and_capped_raw_win", func(t *testing.T) {
		installScratchT2Fixture(t, f)
		f.exec(t, platform.NativeQuotaMigration)
		f.exec(t, `UPDATE momiao_quota.settings SET enabled=true; UPDATE economy.policy_runtime SET active_version='economy-cap-v1' WHERE singleton`, pgx.QueryExecModeSimpleProtocol)
		defer f.exec(t, `UPDATE economy.policy_runtime SET active_version=NULL WHERE singleton`)
		limitUser, limitSubject := f.player(t, true, 1000000000000)
		p := f.prepareScratch(t, limitSubject, "11")
		if p.MaximumWagerUnits != "500000000000" {
			t.Fatal("economic single-player maximum missing")
		}
		_, e := f.service.PrepareScratch(ctx, limitSubject, botgames.ScratchPrepareInput{RequestID: limitSubject, Wager: "1000001"})
		botGamesWantFault(t, e, "INVALID_REQUEST")
		_, e = f.engine.Create(ctx, limitUser, "scratch", "scratch-cap-limit:"+limitSubject, p.CommitmentID, games.CreateInput{Type: "SCRATCH", Wager: "1000001"})
		if !errors.Is(e, games.ErrInvalidInput) {
			t.Fatal("engine economic stake cap differs")
		}
		f.effects(t, limitUser, 0)
		user, subject := f.player(t, true, 2000000000)
		_, e = f.owner.Apply(ctx, platform.Mutation{UserID: user, Asset: platform.ReserveAPICredit, DeltaUnits: platform.AssetCapUnits - 2000000000, BizType: "BOT_CAP_TEST", BizID: "scratch-cap:" + subject, EntryType: "TEST_GRANT", IdempotencyKey: "scratch-cap:" + subject})
		botGamesPGCheck(t, e, "fill reserve cap")
		p = f.prepareScratch(t, subject, "11")
		if p.Prizes[0].Tier != "T2" || p.Prizes[0].Weight != 100000 {
			t.Fatal("current nondefault odds ignored")
		}
		before := f.balance(t, user)
		r, e := f.service.PlayScratch(ctx, subject, p.Quote)
		botGamesPGCheck(t, e, "capped scratch")
		if r.Result != "WIN" || r.PrizeTier != "T2" || r.Multiplier != 2 || r.GrossPayoutUnits != "11000000" || r.WithheldUnits != "5500000" || r.CreditedPayoutUnits != "5500000" || r.ActualNetUnits != "0" || f.balance(t, user) != before {
			t.Fatal("raw win/capped actual accounting mismatch", r)
		}
		matching := 0
		for _, cell := range r.Cells {
			if cell.Matching {
				matching++
				if cell.Symbol != "P2" {
					t.Fatal("wrong winning symbol")
				}
			}
		}
		if matching != 3 {
			t.Fatal("canonical match count")
		}
		replayed, e := f.service.LookupScratch(ctx, subject, p.Quote)
		botGamesPGCheck(t, e, "capped lookup")
		if !reflect.DeepEqual(replayed, r) {
			t.Fatal("capped receipt changed")
		}
		f.scratchLedger(t, user, r)
	})
}

func installScratchT2Fixture(t *testing.T, f *botGamesPGFixture) {
	t.Helper()
	c, e := games.NewScratchConfig("00000000-0000-4000-8000-000000000801", "scratch-test-t2-v1", []games.Prize{{Multiplier: 2, Tier: "T2", Weight: 100000}, {Multiplier: 100, Tier: "TOP", Weight: 0}, {Multiplier: 0, Tier: "LOSS", Weight: 0}, {Multiplier: 1, Tier: "BREAK_EVEN", Weight: 0}, {Multiplier: 3, Tier: "T3", Weight: 0}, {Multiplier: 5, Tier: "T5", Weight: 0}, {Multiplier: 10, Tier: "T10", Weight: 0}, {Multiplier: 25, Tier: "T25", Weight: 0}})
	botGamesPGCheck(t, e, "scratch deterministic pool config")
	b := c.Binding()
	canonical := c.CanonicalJSON()
	summary := []byte(`{"break_even":"0","loss":"0","rtp":"2","top":"0","win":"1"}`)
	hash := sha256.Sum256(summary)
	e = f.owner.WithTx(context.Background(), func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `UPDATE games.game_config_versions SET status='SUPERSEDED',superseded_at=statement_timestamp() WHERE game_slug='scratch' AND status='ACTIVE'`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO games.game_config_versions(config_version_id,game_slug,parent_version_id,config_schema_version,ruleset_version,algorithm_version,config_payload,canonical_payload,config_hash,resource_versions,version_number,status,validated_at,previewed_at,activated_at) VALUES($1,'scratch',(SELECT active_config_version_id FROM games.game_registry WHERE game_slug='scratch'),$2,$3,$4,$5::jsonb,$6,$7,'{"prize_table_version":"scratch-test-t2-v1"}'::jsonb,(SELECT max(version_number)+1 FROM games.game_config_versions WHERE game_slug='scratch'),'ACTIVE',statement_timestamp(),statement_timestamp(),statement_timestamp())`, b.Version, b.SchemaVersion, b.RulesetVersion, b.AlgorithmVersion, string(canonical), canonical, b.Hash[:]); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO games.game_validation_artifacts(validation_artifact_id,game_slug,artifact_type,implementation_key,ruleset_version,algorithm_version,config_version_id,config_hash,validator_version,validation_build,result_summary,artifact_sha256,status,verified_at) VALUES('00000000-0000-4000-8000-000000000802','scratch','EXACT_MATH','direct.scratch.v1',$1,$2,$3,$4,'direct-validate-v1','direct-games-v1',$5::jsonb,$6,'VERIFIED',statement_timestamp())`, b.RulesetVersion, b.AlgorithmVersion, b.Version, b.Hash[:], string(summary), hash[:]); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE games.game_registry SET active_config_version_id=$1,version=version+1,updated_at=statement_timestamp() WHERE game_slug='scratch'`, b.Version)
		return err
	})
	botGamesPGCheck(t, e, "install scratch nondefault pool")
}
