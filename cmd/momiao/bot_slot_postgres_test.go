package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/botgames"
	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

func (f *botGamesPGFixture) prepareSlot(t *testing.T, subject, wager string) botgames.SlotPrepared {
	t.Helper()
	f.next++
	p, e := f.service.PrepareSlot(context.Background(), subject, botgames.SlotPrepareInput{RequestID: strconv.FormatInt(f.next, 10), TotalWager: wager})
	botGamesPGCheck(t, e, "slot prepare")
	return p
}

func TestBotSlotPostgres(t *testing.T) {
	f := newBotGamesPGFixture(t)
	ctx := context.Background()
	// The current browser harness only applies 0015 grants in its historical-gap
	// mode. Reuse the exact existing slot grant, never broaden production roles.
	f.exec(t, `GRANT SELECT,INSERT ON games.slot_results,games.slot_line_results TO `+pgx.Identifier{f.connection.RuntimeRole}.Sanitize())
	t.Run("10_and_11_real_http", func(t *testing.T) {
		h, e := newBotGamesHandler(f.service, botGamesTestToken)
		botGamesPGCheck(t, e, "slot HTTP handler")
		for _, wager := range []string{"10", "11"} {
			t.Run(wager, func(t *testing.T) {
				u, subject := f.player(t, true, 50000000)
				req := botGamesRequest("POST", "/internal/v1/bot-games/slot/prepare", fmt.Sprintf(`{"request_id":%q,"total_wager":%q}`, subject, wager))
				req.Header.Set("X-Discord-User", subject)
				w := httptest.NewRecorder()
				h.ServeHTTP(w, req)
				if w.Code != 200 {
					t.Fatalf("slot HTTP prepare: %d %s", w.Code, w.Body.String())
				}
				var p botgames.SlotPrepared
				botGamesPGCheck(t, json.Unmarshal(w.Body.Bytes(), &p), "slot prepared JSON")
				chips, _ := strconv.ParseInt(wager, 10, 64)
				if p.TotalWager != wager || p.LineCount != 10 || p.LineStakeUnits != strconv.FormatInt(chips*50000, 10) || p.Ruleset != "slot-rules-v3" {
					t.Fatal("wrong slot prepare contract", p)
				}
				before := f.balance(t, u)
				var first botgames.SlotResult
				for _, op := range []string{"play", "lookup", "play"} {
					payload, _ := json.Marshal(map[string]string{"quote": p.Quote})
					req = botGamesRequest("POST", "/internal/v1/bot-games/slot/"+op, string(payload))
					req.Header.Set("X-Discord-User", subject)
					w = httptest.NewRecorder()
					h.ServeHTTP(w, req)
					if w.Code != 200 {
						t.Fatalf("slot HTTP %s: %d %s", op, w.Code, w.Body.String())
					}
					var got botgames.SlotResult
					botGamesPGCheck(t, json.Unmarshal(w.Body.Bytes(), &got), "slot result JSON")
					if got.TotalWager != wager || got.StakeUnits != strconv.FormatInt(chips*500000, 10) || got.LineStakeUnits != p.LineStakeUnits || got.Status != "SETTLED" || got.LineCount != 10 {
						t.Fatal("wrong slot result contract", got)
					}
					var fields map[string]json.RawMessage
					botGamesPGCheck(t, json.Unmarshal(w.Body.Bytes(), &fields), "slot wire fields")
					allowed := []string{"round_id", "created_at", "settled_at", "status", "total_wager", "line_count", "line_stake_units", "full_grid", "lines", "result", "result_detail", "stake_units", "gross_payout_units", "withheld_units", "credited_payout_units", "actual_net_units", "ruleset"}
					if len(fields) != len(allowed) {
						t.Fatal("slot result leaked fields")
					}
					for _, key := range allowed {
						if _, ok := fields[key]; !ok {
							t.Fatal("slot result omitted", key)
						}
					}
					if first.RoundID != "" && first != got {
						t.Fatal("durable replay changed projection")
					}
					first = got
				}
				net, e := strconv.ParseInt(first.ActualNetUnits, 10, 64)
				botGamesPGCheck(t, e, "slot net")
				if f.balance(t, u)-before != net {
					t.Fatal("real wallet differs from slot net")
				}
				f.effects(t, u, 1)
				verified, e := f.engine.Verify(ctx, u, first.RoundID)
				botGamesPGCheck(t, e, "slot fairness verification")
				if !verified.Verified {
					t.Fatal("slot fairness failed")
				}
			})
		}
	})
	t.Run("20_concurrent_confirmation_restart_expired_recovery", func(t *testing.T) {
		u, subject := f.player(t, true, 50000000)
		p := f.prepareSlot(t, subject, "11")
		requestID := strconv.FormatInt(f.next, 10)
		var wg sync.WaitGroup
		results := make(chan botgames.SlotResult, 20)
		errs := make(chan error, 20)
		before := f.balance(t, u)
		for range 20 {
			wg.Add(1)
			go func() { defer wg.Done(); r, e := f.service.PlaySlot(ctx, subject, p.Quote); results <- r; errs <- e }()
		}
		wg.Wait()
		close(results)
		close(errs)
		for e := range errs {
			botGamesPGCheck(t, e, "slot concurrent play")
		}
		var first botgames.SlotResult
		for r := range results {
			if first.RoundID != "" && r != first {
				t.Fatal("concurrent slot results differ")
			}
			first = r
		}
		net, e := strconv.ParseInt(first.ActualNetUnits, 10, 64)
		botGamesPGCheck(t, e, "concurrent actual net")
		if f.balance(t, u)-before != net {
			t.Fatal("concurrent wallet double effect")
		}
		f.effects(t, u, 1)
		changed, e := f.service.PrepareSlot(ctx, subject, botgames.SlotPrepareInput{RequestID: requestID, TotalWager: "10"})
		botGamesPGCheck(t, e, "changed slot quote")
		_, e = f.service.PlaySlot(ctx, subject, changed.Quote)
		botGamesWantFault(t, e, "IDEMPOTENCY_CONFLICT")
		f.now = f.now.Add(121 * time.Second)
		defer func() { f.now = f.now.Add(-121 * time.Second) }()
		engine, e := games.NewService(f.runtime, games.Keyring{Active: "bot-fixture-v1", Keys: map[string][32]byte{"bot-fixture-v1": {9}}})
		botGamesPGCheck(t, e, "restarted game engine")
		service, e := botgames.NewService(engine, botGamesResolver{native: f.native, platform: f.runtime, declaration: f.declaration}, [32]byte{2}, func() time.Time { return f.now })
		botGamesPGCheck(t, e, "restarted slot service")
		recovered, e := service.LookupSlot(ctx, subject, p.Quote)
		botGamesPGCheck(t, e, "expired restarted lookup")
		replay, e := service.PlaySlot(ctx, subject, p.Quote)
		botGamesPGCheck(t, e, "expired restarted replay")
		if recovered != first || replay != first {
			t.Fatal("restart recovery changed result")
		}
		f.effects(t, u, 1)
		f.exec(t, `UPDATE public.users SET status=2 WHERE id=$1`, u)
		_, e = service.LookupSlot(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "ACCOUNT_RESTRICTED")
	})
	t.Run("slot_and_dice_are_separate_durable_keys", func(t *testing.T) {
		u, subject := f.player(t, true, 50000000)
		slot, e := f.service.PrepareSlot(ctx, subject, botgames.SlotPrepareInput{RequestID: subject, TotalWager: "11"})
		botGamesPGCheck(t, e, "same ID slot prepare")
		dice, e := f.service.Prepare(ctx, subject, botgames.PrepareInput{RequestID: subject, Wager: "10", Choice: "BIG"})
		botGamesPGCheck(t, e, "same ID dice prepare")
		_, e = f.service.PlaySlot(ctx, subject, dice.Quote)
		botGamesWantFault(t, e, "UNAUTHORIZED")
		_, e = f.service.LookupSlot(ctx, subject, dice.Quote)
		botGamesWantFault(t, e, "UNAUTHORIZED")
		_, e = f.service.Play(ctx, subject, slot.Quote)
		botGamesWantFault(t, e, "UNAUTHORIZED")
		_, e = f.service.Lookup(ctx, subject, slot.Quote)
		botGamesWantFault(t, e, "UNAUTHORIZED")
		f.effects(t, u, 0)
		sr, e := f.service.PlaySlot(ctx, subject, slot.Quote)
		botGamesPGCheck(t, e, "slot unique key")
		dr, e := f.service.Play(ctx, subject, dice.Quote)
		botGamesPGCheck(t, e, "dice unique key")
		if sr.RoundID == dr.RoundID {
			t.Fatal("cross-game idempotency collision")
		}
		f.effects(t, u, 2)
	})
	t.Run("identity_and_admission_have_zero_effects", func(t *testing.T) {
		for _, kind := range []string{"unlinked", "disabled", "deleted", "duplicate", "missing_ref", "incomplete_profile"} {
			t.Run(kind, func(t *testing.T) {
				u, subject := f.player(t, kind != "missing_ref" && kind != "incomplete_profile", 50000000)
				want := "ACCOUNT_RESTRICTED"
				switch kind {
				case "unlinked":
					subject = "18446744073709551615"
					want = "NOT_LINKED"
				case "disabled":
					f.exec(t, `UPDATE public.users SET status=2 WHERE id=$1`, u)
				case "deleted":
					f.exec(t, `UPDATE public.users SET deleted_at=now() WHERE id=$1`, u)
				case "duplicate":
					f.exec(t, `INSERT INTO public.users(id,discord_id,status) VALUES($1,$2,2)`, u+900000000, subject)
				case "missing_ref":
					want = "ACCOUNT_NOT_READY"
				case "incomplete_profile":
					botGamesPGCheck(t, f.owner.EnsureAccount(ctx, u), "incomplete slot account")
					want = "ACCOUNT_NOT_READY"
				}
				_, e := f.service.PrepareSlot(ctx, subject, botgames.SlotPrepareInput{RequestID: subject, TotalWager: "11"})
				botGamesWantFault(t, e, want)
				f.effects(t, u, 0)
			})
		}
	})
	t.Run("binding_changed", func(t *testing.T) {
		u, subject := f.player(t, true, 50000000)
		other, _ := f.player(t, true, 50000000)
		p := f.prepareSlot(t, subject, "11")
		f.exec(t, `UPDATE public.users SET discord_id='' WHERE id=$1`, u)
		f.exec(t, `UPDATE public.users SET discord_id=$2 WHERE id=$1`, other, subject)
		_, e := f.service.PlaySlot(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "BINDING_CHANGED")
		_, e = f.service.LookupSlot(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "BINDING_CHANGED")
		f.effects(t, u, 0)
		f.effects(t, other, 0)
	})
	t.Run("expiry_and_lookup_never_make_new_round", func(t *testing.T) {
		u, subject := f.player(t, true, 50000000)
		p := f.prepareSlot(t, subject, "11")
		_, e := f.service.LookupSlot(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "NOT_FOUND")
		f.now = f.now.Add(120 * time.Second)
		defer func() { f.now = f.now.Add(-120 * time.Second) }()
		_, e = f.service.PlaySlot(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "QUOTE_EXPIRED")
		_, e = f.service.LookupSlot(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "NOT_FOUND")
		f.effects(t, u, 0)
	})
	t.Run("browser_consumed_commitment", func(t *testing.T) {
		u, subject := f.player(t, true, 50000000)
		p := f.prepareSlot(t, subject, "11")
		_, e := f.engine.Create(ctx, u, "slot", "slot-browser:"+subject, p.CommitmentID, games.CreateInput{Type: "SLOT", TotalWager: "11"})
		botGamesPGCheck(t, e, "browser slot consume")
		before := f.balance(t, u)
		_, e = f.service.PlaySlot(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "COMMITMENT_INVALID")
		if f.balance(t, u) != before {
			t.Fatal("invalid commitment changed wallet")
		}
		f.effects(t, u, 1)
	})
	t.Run("transaction_rechecks_balance_and_maintenance", func(t *testing.T) {
		u, subject := f.player(t, true, 5500000)
		p := f.prepareSlot(t, subject, "11")
		_, e := f.owner.Apply(ctx, platform.Mutation{UserID: u, Asset: platform.AvailableChips, DeltaUnits: -5500000, BizType: "BOT_TEST", BizID: "slot-drain:" + subject, EntryType: "TEST_DRAIN", IdempotencyKey: "slot-drain:" + subject})
		botGamesPGCheck(t, e, "drain slot wallet")
		_, e = f.service.PlaySlot(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "INSUFFICIENT_CHIPS")
		f.effects(t, u, 0)
		other, os := f.player(t, true, 50000000)
		p = f.prepareSlot(t, os, "11")
		f.exec(t, `UPDATE games.runtime_gate SET maintenance=true WHERE singleton`)
		defer f.exec(t, `UPDATE games.runtime_gate SET maintenance=false WHERE singleton`)
		_, e = f.service.PlaySlot(ctx, os, p.Quote)
		botGamesWantFault(t, e, "MAINTENANCE")
		_, e = f.service.PrepareSlot(ctx, os, botgames.SlotPrepareInput{RequestID: os, TotalWager: "11"})
		botGamesWantFault(t, e, "MAINTENANCE")
		f.effects(t, other, 0)
	})
	t.Run("maximum_matches_actual_pre_rng_guard", func(t *testing.T) {
		u, subject := f.player(t, true, math.MaxInt64-2855050000)
		p := f.prepareSlot(t, subject, "11")
		if p.MaximumWagerUnits != "5500000" {
			t.Fatal("v3 overflow bound", p.MaximumWagerUnits)
		}
		_, e := f.service.PrepareSlot(ctx, subject, botgames.SlotPrepareInput{RequestID: subject, TotalWager: "12"})
		botGamesWantFault(t, e, "INVALID_REQUEST")
		_, e = f.engine.Create(ctx, u, "slot", "slot-overflow:"+subject, p.CommitmentID, games.CreateInput{Type: "SLOT", TotalWager: "12"})
		if !errors.Is(e, platform.ErrBalanceOverflow) {
			t.Fatalf("engine pre-RNG bound differs (%T)", e)
		}
		f.effects(t, u, 0)
		b, e := f.engine.Bootstrap(ctx, u, "slot")
		botGamesPGCheck(t, e, "commitment after rejected overflow")
		if b.Next == nil || b.Next.ID != p.CommitmentID {
			t.Fatal("overflow consumed randomness commitment")
		}
		_, e = f.service.PlaySlot(ctx, subject, p.Quote)
		botGamesPGCheck(t, e, "exact overflow boundary play")
		f.effects(t, u, 1)
	})
	t.Run("economic_limit_and_capped_net", func(t *testing.T) {
		f.exec(t, platform.NativeQuotaMigration)
		f.exec(t, `UPDATE momiao_quota.settings SET enabled=true; UPDATE economy.policy_runtime SET active_version='economy-cap-v1' WHERE singleton`, pgx.QueryExecModeSimpleProtocol)
		defer f.exec(t, `UPDATE economy.policy_runtime SET active_version=NULL WHERE singleton`)
		limitUser, limitSubject := f.player(t, true, 1000000000000)
		p := f.prepareSlot(t, limitSubject, "11")
		if p.MaximumWagerUnits != "500000000000" {
			t.Fatal("economic maximum not advertised", p.MaximumWagerUnits)
		}
		_, e := f.service.PrepareSlot(ctx, limitSubject, botgames.SlotPrepareInput{RequestID: limitSubject, TotalWager: "1000001"})
		botGamesWantFault(t, e, "INVALID_REQUEST")
		f.effects(t, limitUser, 0)
		u, subject := f.player(t, true, 2000000000)
		clipped := false
		for i := 0; i < 64; i++ {
			before := f.balance(t, u)
			reserve, e := f.owner.ReadWallet(ctx, u, platform.ReserveAPICredit)
			botGamesPGCheck(t, e, "cap reserve read")
			fill := platform.AssetCapUnits - before - reserve.BalanceUnits
			if fill > 0 {
				_, e = f.owner.Apply(ctx, platform.Mutation{UserID: u, Asset: platform.ReserveAPICredit, DeltaUnits: fill, BizType: "BOT_CAP_TEST", BizID: fmt.Sprintf("slot:%s:%d", subject, i), EntryType: "TEST_GRANT", IdempotencyKey: fmt.Sprintf("slot-cap:%s:%d", subject, i)})
				botGamesPGCheck(t, e, "slot cap fill")
			}
			p := f.prepareSlot(t, subject, "11")
			r, e := f.service.PlaySlot(ctx, subject, p.Quote)
			botGamesPGCheck(t, e, "slot capped play")
			net, e := strconv.ParseInt(r.ActualNetUnits, 10, 64)
			botGamesPGCheck(t, e, "slot cap net")
			if f.balance(t, u)-before != net {
				t.Fatal("cap wallet differs from projected actual net")
			}
			replay, e := f.service.LookupSlot(ctx, subject, p.Quote)
			botGamesPGCheck(t, e, "slot cap replay")
			if replay != r {
				t.Fatal("slot cap lookup changed receipt")
			}
			if r.WithheldUnits != "0" {
				if r.Result != "WIN" || r.ResultDetail != "WIN" || r.ActualNetUnits != "0" || r.CreditedPayoutUnits != "5500000" {
					t.Fatal("slot raw outcome conflated with clipped profit", r)
				}
				clipped = true
				f.effects(t, u, i+1)
				t.Logf("slot cap exercised after %d rounds; raw win retained with zero actual net", i+1)
				break
			}
		}
		if !clipped {
			t.Fatal("no clipped slot win in 64 synthetic rounds")
		}
	})
}
