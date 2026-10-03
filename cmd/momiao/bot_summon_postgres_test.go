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

func requireFreshSummonFixture(t *testing.T) {
	t.Helper()
	path := os.Getenv("MOMIAO_GAMES_TEST_CONNECTION_FILE")
	if path == "" {
		t.Skip("fresh isolated local summon G1 connection required")
	}
	raw, e := os.ReadFile(path)
	var c struct{ OwnerURL, RuntimeURL, NativeURL string }
	if e != nil || json.Unmarshal(raw, &c) != nil {
		t.Fatal("summon fixture unreadable")
	}
	var database string
	for _, url := range []string{c.OwnerURL, c.RuntimeURL, c.NativeURL} {
		config, err := pgx.ParseConfig(url)
		if err != nil || config.Host != "127.0.0.1" || config.Port != 55432 || !strings.HasPrefix(config.Database, "momiao_test_g1_original_summon_") {
			t.Fatal("refusing non-summon loopback fixture")
		}
		if database != "" && config.Database != database {
			t.Fatal("split fixture databases")
		}
		database = config.Database
	}
	connection, e := pgx.Connect(context.Background(), c.OwnerURL)
	botGamesPGCheck(t, e, "fresh summon guard")
	defer connection.Close(context.Background())
	var fresh bool
	e = connection.QueryRow(context.Background(), `SELECT NOT EXISTS(SELECT FROM pg_namespace WHERE nspname IN ('games','economy','identity')) AND to_regclass('public.users') IS NULL`).Scan(&fresh)
	botGamesPGCheck(t, e, "fresh database guard")
	if !fresh {
		t.Fatal("summon integration requires a NEW empty database; preserve prior evidence")
	}
}

func (f *botGamesPGFixture) prepareSummon(t *testing.T, subject, wager, mode string) botgames.SummonPrepared {
	t.Helper()
	f.next++
	p, e := f.service.PrepareSummon(context.Background(), subject, botgames.SummonPrepareInput{RequestID: strconv.FormatInt(f.next, 10), BaseWager: wager, Mode: mode})
	botGamesPGCheck(t, e, "summon prepare")
	return p
}

func (f *botGamesPGFixture) summonLedger(t *testing.T, user int64, r botgames.SummonResult) {
	t.Helper()
	var state string
	var rounds, draws, wagers, settlements int
	var stake, payout, supply int64
	e := f.owner.WithTx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT c.state,
 (SELECT count(*) FROM games.game_rounds WHERE newapi_user_id=$1),
 (SELECT count(*) FROM games.summon_draws WHERE round_id=$2),
 (SELECT count(*) FROM economy.asset_transactions WHERE newapi_user_id=$1 AND biz_type='GAME_WAGER'),
 (SELECT count(*) FROM economy.asset_transactions WHERE newapi_user_id=$1 AND biz_type='GAME_SETTLEMENT'),
 (SELECT coalesce(sum(l.delta_units),0)::bigint FROM economy.wallet_ledger l JOIN economy.asset_transactions a ON a.transaction_id=l.transaction_id WHERE a.newapi_user_id=$1 AND a.biz_type='GAME_WAGER'),
 (SELECT coalesce(sum(l.delta_units),0)::bigint FROM economy.wallet_ledger l JOIN economy.asset_transactions a ON a.transaction_id=l.transaction_id WHERE a.newapi_user_id=$1 AND a.biz_type='GAME_SETTLEMENT'),
 (SELECT coalesce(sum(CASE WHEN direction='ISSUE' THEN amount_units ELSE -amount_units END),0)::bigint FROM games.supply_events WHERE round_id=$2)
 FROM games.game_rounds r JOIN games.fairness_commitments c ON c.commitment_id=r.commitment_id WHERE r.round_id=$2 AND r.newapi_user_id=$1`, user, r.RoundID).Scan(&state, &rounds, &draws, &wagers, &settlements, &stake, &payout, &supply)
	})
	botGamesPGCheck(t, e, "summon durable ledger")
	total, _ := strconv.ParseInt(r.StakeUnits, 10, 64)
	credited, _ := strconv.ParseInt(r.CreditedPayoutUnits, 10, 64)
	net, _ := strconv.ParseInt(r.ActualNetUnits, 10, 64)
	if state != "REVEALED" || rounds != 1 || draws != r.DrawCount || wagers != 1 || settlements != 1 || stake != -total || payout != credited || supply != net {
		t.Fatalf("summon ledger mismatch state=%s rounds=%d draws=%d wagers=%d settlements=%d stake=%d payout=%d supply=%d", state, rounds, draws, wagers, settlements, stake, payout, supply)
	}
}

func TestBotSummonPostgres(t *testing.T) {
	requireFreshSummonFixture(t)
	f := newBotGamesPGFixture(t)
	ctx := context.Background()
	f.exec(t, `GRANT SELECT,INSERT ON games.slot_results,games.slot_line_results TO `+pgx.Identifier{f.connection.RuntimeRole}.Sanitize())
	t.Run("single_and_tenfold_10_and_11_real_http_accounting", func(t *testing.T) {
		h, e := newBotGamesHandler(f.service, botGamesTestToken)
		botGamesPGCheck(t, e, "summon HTTP handler")
		for _, mode := range []string{"SINGLE", "TENFOLD"} {
			for _, wager := range []string{"10", "11"} {
				t.Run(mode+"_"+wager, func(t *testing.T) {
					user, subject := f.player(t, true, 550000000)
					req := botGamesRequest("POST", "/internal/v1/bot-games/summon/prepare", fmt.Sprintf(`{"request_id":%q,"base_wager":%q,"mode":%q}`, subject, wager, mode))
					req.Header.Set("X-Discord-User", subject)
					w := httptest.NewRecorder()
					h.ServeHTTP(w, req)
					if w.Code != 200 {
						t.Fatalf("summon HTTP prepare status=%d body=%s", w.Code, w.Body.String())
					}
					var p botgames.SummonPrepared
					botGamesPGCheck(t, json.Unmarshal(w.Body.Bytes(), &p), "summon prepared JSON")
					chips, _ := strconv.ParseInt(wager, 10, 64)
					count := 1
					maximum := "550000000"
					if mode == "TENFOLD" {
						count = 10
						maximum = "55000000"
					}
					if p.BaseWager != wager || p.Mode != mode || p.DrawCount != count || p.StakeUnits != strconv.FormatInt(chips*500000*int64(count), 10) || p.MaximumBaseWagerUnits != maximum || p.MinimumBaseWagerUnits != "5000000" || p.Ruleset != "summon-rules-v1" {
						t.Fatal("summon prepare accounting", p)
					}
					if !reflect.DeepEqual(p.Prizes, []games.Prize{{0, "T0", 40000}, {1, "T1", 20000}, {2, "T2", 39000}, {5, "T3", 850}, {20, "T4", 100}, {100, "T5", 50}}) {
						t.Fatal("prepare not using actual active non-default pool", p.Prizes)
					}
					before := f.balance(t, user)
					var first botgames.SummonResult
					for _, op := range []string{"play", "lookup", "play"} {
						payload, _ := json.Marshal(map[string]string{"quote": p.Quote})
						req = botGamesRequest("POST", "/internal/v1/bot-games/summon/"+op, string(payload))
						req.Header.Set("X-Discord-User", subject)
						w = httptest.NewRecorder()
						h.ServeHTTP(w, req)
						if w.Code != 200 {
							t.Fatalf("summon HTTP %s status=%d body=%s", op, w.Code, w.Body.String())
						}
						var got botgames.SummonResult
						botGamesPGCheck(t, json.Unmarshal(w.Body.Bytes(), &got), "summon result JSON")
						if got.BaseWager != wager || got.Mode != mode || got.DrawCount != count || len(got.Draws) != count || got.StakeUnits != p.StakeUnits || got.Status != "SETTLED" {
							t.Fatal("summon result accounting", got)
						}
						if first.RoundID != "" && !reflect.DeepEqual(first, got) {
							t.Fatal("replay changed receipt")
						}
						first = got
					}
					net, e := strconv.ParseInt(first.ActualNetUnits, 10, 64)
					botGamesPGCheck(t, e, "summon net")
					if f.balance(t, user)-before != net {
						t.Fatal("wallet differs from actual net")
					}
					f.effects(t, user, 1)
					f.summonLedger(t, user, first)
					fair, e := f.engine.Verify(ctx, user, first.RoundID)
					botGamesPGCheck(t, e, "summon fairness")
					if !fair.Verified {
						t.Fatal("summon fairness failed")
					}
					history, e := f.engine.History(ctx, user, games.HistoryQuery{Game: "summon"})
					botGamesPGCheck(t, e, "summon history")
					if len(history.Items) != 1 || history.Items[0].ID != first.RoundID {
						t.Fatal("history missing round")
					}
				})
			}
		}
	})
	for _, mode := range []string{"SINGLE", "TENFOLD"} {
		t.Run(mode+"_20_concurrent_lost_response_restart_expired_replay", func(t *testing.T) {
			user, subject := f.player(t, true, 550000000)
			p := f.prepareSummon(t, subject, "11", mode)
			requestID := strconv.FormatInt(f.next, 10)
			before := f.balance(t, user)
			var wg sync.WaitGroup
			results := make(chan botgames.SummonResult, 20)
			errs := make(chan error, 20)
			for range 20 {
				wg.Add(1)
				go func() { defer wg.Done(); r, e := f.service.PlaySummon(ctx, subject, p.Quote); results <- r; errs <- e }()
			}
			wg.Wait()
			close(results)
			close(errs)
			for e := range errs {
				botGamesPGCheck(t, e, "concurrent summon")
			}
			var first botgames.SummonResult
			for r := range results {
				if first.RoundID != "" && !reflect.DeepEqual(first, r) {
					t.Fatal("concurrent result differs")
				}
				first = r
			}
			net, e := strconv.ParseInt(first.ActualNetUnits, 10, 64)
			botGamesPGCheck(t, e, "concurrent net")
			if f.balance(t, user)-before != net {
				t.Fatal("duplicate wallet effect")
			}
			f.summonLedger(t, user, first)
			for _, input := range []botgames.SummonPrepareInput{{RequestID: requestID, BaseWager: "10", Mode: mode}, {RequestID: requestID, BaseWager: "11", Mode: map[string]string{"SINGLE": "TENFOLD", "TENFOLD": "SINGLE"}[mode]}} {
				changed, e := f.service.PrepareSummon(ctx, subject, input)
				botGamesPGCheck(t, e, "conflicting quote")
				_, e = f.service.PlaySummon(ctx, subject, changed.Quote)
				botGamesWantFault(t, e, "IDEMPOTENCY_CONFLICT")
			}
			f.now = f.now.Add(121 * time.Second)
			defer func() { f.now = f.now.Add(-121 * time.Second) }()
			engine, e := games.NewService(f.runtime, games.Keyring{Active: "bot-fixture-v1", Keys: map[string][32]byte{"bot-fixture-v1": {9}}})
			botGamesPGCheck(t, e, "restart engine")
			service, e := botgames.NewService(engine, botGamesResolver{native: f.native, platform: f.runtime, declaration: f.declaration}, [32]byte{2}, func() time.Time { return f.now })
			botGamesPGCheck(t, e, "restart summon")
			recovered, e := service.LookupSummon(ctx, subject, p.Quote)
			botGamesPGCheck(t, e, "lost response lookup")
			replayed, e := service.PlaySummon(ctx, subject, p.Quote)
			botGamesPGCheck(t, e, "expired saved replay")
			if !reflect.DeepEqual(recovered, first) || !reflect.DeepEqual(replayed, first) {
				t.Fatal("restart changed saved receipt")
			}
			f.summonLedger(t, user, first)
			f.exec(t, `UPDATE public.users SET status=2 WHERE id=$1`, user)
			_, e = service.LookupSummon(ctx, subject, p.Quote)
			botGamesWantFault(t, e, "ACCOUNT_RESTRICTED")
		})
	}
	t.Run("all_three_signature_domains_and_durable_keys", func(t *testing.T) {
		user, subject := f.player(t, true, 550000000)
		u, e := f.service.PrepareSummon(ctx, subject, botgames.SummonPrepareInput{RequestID: subject, BaseWager: "11", Mode: "TENFOLD"})
		botGamesPGCheck(t, e, "same ID summon")
		d, e := f.service.Prepare(ctx, subject, botgames.PrepareInput{RequestID: subject, Wager: "10", Choice: "BIG"})
		botGamesPGCheck(t, e, "same ID dice")
		s, e := f.service.PrepareSlot(ctx, subject, botgames.SlotPrepareInput{RequestID: subject, TotalWager: "11"})
		botGamesPGCheck(t, e, "same ID slot")
		for _, q := range []string{d.Quote, s.Quote} {
			_, e = f.service.PlaySummon(ctx, subject, q)
			botGamesWantFault(t, e, "UNAUTHORIZED")
			_, e = f.service.LookupSummon(ctx, subject, q)
			botGamesWantFault(t, e, "UNAUTHORIZED")
		}
		for _, q := range []string{u.Quote, s.Quote} {
			_, e = f.service.Play(ctx, subject, q)
			botGamesWantFault(t, e, "UNAUTHORIZED")
			_, e = f.service.Lookup(ctx, subject, q)
			botGamesWantFault(t, e, "UNAUTHORIZED")
		}
		for _, q := range []string{u.Quote, d.Quote} {
			_, e = f.service.PlaySlot(ctx, subject, q)
			botGamesWantFault(t, e, "UNAUTHORIZED")
			_, e = f.service.LookupSlot(ctx, subject, q)
			botGamesWantFault(t, e, "UNAUTHORIZED")
		}
		f.effects(t, user, 0)
		ur, e := f.service.PlaySummon(ctx, subject, u.Quote)
		botGamesPGCheck(t, e, "summon own key")
		dr, e := f.service.Play(ctx, subject, d.Quote)
		botGamesPGCheck(t, e, "dice own key")
		sr, e := f.service.PlaySlot(ctx, subject, s.Quote)
		botGamesPGCheck(t, e, "slot own key")
		if ur.RoundID == dr.RoundID || ur.RoundID == sr.RoundID || sr.RoundID == dr.RoundID {
			t.Fatal("cross-game key collision")
		}
		f.effects(t, user, 3)
	})
	t.Run("rebound_identity", func(t *testing.T) {
		user, subject := f.player(t, true, 550000000)
		other, _ := f.player(t, true, 550000000)
		p := f.prepareSummon(t, subject, "11", "TENFOLD")
		f.exec(t, `UPDATE public.users SET discord_id='' WHERE id=$1`, user)
		f.exec(t, `UPDATE public.users SET discord_id=$2 WHERE id=$1`, other, subject)
		_, e := f.service.PlaySummon(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "BINDING_CHANGED")
		_, e = f.service.LookupSummon(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "BINDING_CHANGED")
		f.effects(t, user, 0)
		f.effects(t, other, 0)
	})
	t.Run("lookup_never_creates_expiry_and_consumed_commitment", func(t *testing.T) {
		user, subject := f.player(t, true, 550000000)
		p := f.prepareSummon(t, subject, "11", "TENFOLD")
		_, e := f.service.LookupSummon(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "NOT_FOUND")
		f.now = f.now.Add(120 * time.Second)
		_, e = f.service.PlaySummon(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "QUOTE_EXPIRED")
		_, e = f.service.LookupSummon(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "NOT_FOUND")
		f.now = f.now.Add(-120 * time.Second)
		f.effects(t, user, 0)
		_, e = f.engine.Create(ctx, user, "summon", "summon-browser:"+subject, p.CommitmentID, games.CreateInput{Type: "SUMMON", BaseWager: "11", Mode: "TENFOLD"})
		botGamesPGCheck(t, e, "browser consume commitment")
		before := f.balance(t, user)
		_, e = f.service.PlaySummon(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "COMMITMENT_INVALID")
		if f.balance(t, user) != before {
			t.Fatal("invalid commitment changed wallet")
		}
		f.effects(t, user, 1)
	})
	t.Run("transaction_rechecks_total_balance_and_maintenance", func(t *testing.T) {
		user, subject := f.player(t, true, 55000000)
		p := f.prepareSummon(t, subject, "11", "TENFOLD")
		_, e := f.owner.Apply(ctx, platform.Mutation{UserID: user, Asset: platform.AvailableChips, DeltaUnits: -500000, BizType: "BOT_TEST", BizID: "summon-drain:" + subject, EntryType: "TEST_DRAIN", IdempotencyKey: "summon-drain:" + subject})
		botGamesPGCheck(t, e, "drain wallet")
		_, e = f.service.PlaySummon(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "INSUFFICIENT_CHIPS")
		_, e = f.service.PrepareSummon(ctx, subject, botgames.SummonPrepareInput{RequestID: subject, BaseWager: "11", Mode: "TENFOLD"})
		botGamesWantFault(t, e, "INSUFFICIENT_CHIPS")
		f.effects(t, user, 0)
		other, subject := f.player(t, true, 550000000)
		p = f.prepareSummon(t, subject, "11", "TENFOLD")
		f.exec(t, `UPDATE games.runtime_gate SET maintenance=true WHERE singleton`)
		defer f.exec(t, `UPDATE games.runtime_gate SET maintenance=false WHERE singleton`)
		_, e = f.service.PlaySummon(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "MAINTENANCE")
		_, e = f.service.PrepareSummon(ctx, subject, botgames.SummonPrepareInput{RequestID: subject, BaseWager: "11", Mode: "TENFOLD"})
		botGamesWantFault(t, e, "MAINTENANCE")
		f.effects(t, other, 0)
	})
	for _, tc := range []struct {
		mode     string
		headroom int64
	}{{"SINGLE", 544500000}, {"TENFOLD", 5445000000}} {
		t.Run(tc.mode+"_pre_rng_headroom_boundary", func(t *testing.T) {
			user, subject := f.player(t, true, math.MaxInt64-tc.headroom)
			p := f.prepareSummon(t, subject, "11", tc.mode)
			if p.MaximumBaseWagerUnits != "5500000" {
				t.Fatal("wrong maximum", p.MaximumBaseWagerUnits)
			}
			_, e := f.service.PrepareSummon(ctx, subject, botgames.SummonPrepareInput{RequestID: subject, BaseWager: "12", Mode: tc.mode})
			botGamesWantFault(t, e, "INVALID_REQUEST")
			_, e = f.engine.Create(ctx, user, "summon", "summon-overflow:"+subject, p.CommitmentID, games.CreateInput{Type: "SUMMON", BaseWager: "12", Mode: tc.mode})
			if !errors.Is(e, platform.ErrBalanceOverflow) {
				t.Fatalf("pre-RNG overflow engine mismatch (%T)", e)
			}
			f.effects(t, user, 0)
			b, e := f.engine.Bootstrap(ctx, user, "summon")
			botGamesPGCheck(t, e, "commitment after overflow")
			if b.Next == nil || b.Next.ID != p.CommitmentID {
				t.Fatal("overflow consumed RNG commitment")
			}
			_, e = f.service.PlaySummon(ctx, subject, p.Quote)
			botGamesPGCheck(t, e, "exact headroom play")
			f.effects(t, user, 1)
		})
	}
	t.Run("deterministic_active_pool_and_economic_cap", func(t *testing.T) {
		installSummonT2Fixture(t, f)
		f.exec(t, platform.NativeQuotaMigration)
		f.exec(t, `UPDATE momiao_quota.settings SET enabled=true; UPDATE economy.policy_runtime SET active_version='economy-cap-v1' WHERE singleton`, pgx.QueryExecModeSimpleProtocol)
		defer f.exec(t, `UPDATE economy.policy_runtime SET active_version=NULL WHERE singleton`)
		for _, tc := range []struct{ mode, maximum, tooMuch, stake, gross string }{{"SINGLE", "500000000000", "1000001", "5500000", "11000000"}, {"TENFOLD", "50000000000", "100001", "55000000", "110000000"}} {
			t.Run(tc.mode, func(t *testing.T) {
				limitUser, limitSubject := f.player(t, true, 1000000000000)
				p := f.prepareSummon(t, limitSubject, "11", tc.mode)
				if p.MaximumBaseWagerUnits != tc.maximum {
					t.Fatal("economic cap must apply to whole round", p.MaximumBaseWagerUnits)
				}
				_, e := f.service.PrepareSummon(ctx, limitSubject, botgames.SummonPrepareInput{RequestID: limitSubject, BaseWager: tc.tooMuch, Mode: tc.mode})
				botGamesWantFault(t, e, "INVALID_REQUEST")
				_, e = f.engine.Create(ctx, limitUser, "summon", "summon-cap-limit:"+limitSubject, p.CommitmentID, games.CreateInput{Type: "SUMMON", BaseWager: tc.tooMuch, Mode: tc.mode})
				if !errors.Is(e, games.ErrInvalidInput) {
					t.Fatalf("engine economic stake cap mismatch (%T)", e)
				}
				f.effects(t, limitUser, 0)
				user, subject := f.player(t, true, 2000000000)
				_, e = f.owner.Apply(ctx, platform.Mutation{UserID: user, Asset: platform.ReserveAPICredit, DeltaUnits: platform.AssetCapUnits - 2000000000, BizType: "BOT_CAP_TEST", BizID: "summon-cap:" + subject, EntryType: "TEST_GRANT", IdempotencyKey: "summon-cap:" + subject})
				botGamesPGCheck(t, e, "cap reserve fill")
				p = f.prepareSummon(t, subject, "11", tc.mode)
				if p.Prizes[0].Tier != "T2" || p.Prizes[0].Weight != 100000 {
					t.Fatal("changed active pool ignored")
				}
				before := f.balance(t, user)
				r, e := f.service.PlaySummon(ctx, subject, p.Quote)
				botGamesPGCheck(t, e, "capped summon")
				if r.Result != "WIN" || r.GrossPayoutUnits != tc.gross || r.WithheldUnits != tc.stake || r.CreditedPayoutUnits != tc.stake || r.ActualNetUnits != "0" || f.balance(t, user) != before {
					t.Fatal("raw win and clipped net conflated", r)
				}
				for _, draw := range r.Draws {
					if draw.Tier != "T2" || draw.Multiplier != 2 || draw.PayoutUnits != "11000000" {
						t.Fatal("configured draw differs", draw)
					}
				}
				replay, e := f.service.LookupSummon(ctx, subject, p.Quote)
				botGamesPGCheck(t, e, "capped lookup")
				if !reflect.DeepEqual(r, replay) {
					t.Fatal("capped receipt changed")
				}
				f.summonLedger(t, user, r)
			})
		}
	})
}

// New immutable local configuration, constructed and validated by the real
// engine. No RNG stub, seeded draw override or production configuration change.
func installSummonT2Fixture(t *testing.T, f *botGamesPGFixture) {
	t.Helper()
	c, e := games.NewSummonConfig("00000000-0000-4000-8000-000000000701", "summon-test-t2-v1", []games.Prize{{2, "T2", 100000}, {100, "T5", 0}, {0, "T0", 0}, {1, "T1", 0}, {5, "T3", 0}, {20, "T4", 0}})
	botGamesPGCheck(t, e, "forced pool config")
	b := c.Binding()
	canonical := c.CanonicalJSON()
	summary := []byte(`{"break_even":"0","loss":"0","rtp":"2","top":"0","win":"1"}`)
	hash := sha256.Sum256(summary)
	e = f.owner.WithTx(context.Background(), func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `UPDATE games.game_config_versions SET status='SUPERSEDED',superseded_at=statement_timestamp() WHERE game_slug='summon' AND status='ACTIVE'`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO games.game_config_versions(config_version_id,game_slug,parent_version_id,config_schema_version,ruleset_version,algorithm_version,config_payload,canonical_payload,config_hash,resource_versions,version_number,status,validated_at,previewed_at,activated_at)
 VALUES($1,'summon',(SELECT active_config_version_id FROM games.game_registry WHERE game_slug='summon'),$2,$3,$4,$5::jsonb,$6,$7,'{"pool_id":"SUMMON_MAIN_V1","prize_table_version":"summon-test-t2-v1"}'::jsonb,(SELECT max(version_number)+1 FROM games.game_config_versions WHERE game_slug='summon'),'ACTIVE',statement_timestamp(),statement_timestamp(),statement_timestamp())`, b.Version, b.SchemaVersion, b.RulesetVersion, b.AlgorithmVersion, string(canonical), canonical, b.Hash[:]); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO games.game_validation_artifacts(validation_artifact_id,game_slug,artifact_type,implementation_key,ruleset_version,algorithm_version,config_version_id,config_hash,validator_version,validation_build,result_summary,artifact_sha256,status,verified_at)
 VALUES('00000000-0000-4000-8000-000000000702','summon','EXACT_MATH','direct.summon.v1',$1,$2,$3,$4,'direct-validate-v1','direct-games-v1',$5::jsonb,$6,'VERIFIED',statement_timestamp())`, b.RulesetVersion, b.AlgorithmVersion, b.Version, b.Hash[:], string(summary), hash[:]); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE games.game_registry SET active_config_version_id=$1,version=version+1,updated_at=statement_timestamp() WHERE game_slug='summon'`, b.Version)
		return err
	})
	botGamesPGCheck(t, e, "install deterministic pool")
}
