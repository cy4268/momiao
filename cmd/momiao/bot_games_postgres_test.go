package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/botgames"
	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type botGamesPGFixture struct {
	owner, runtime *platform.Store
	native         *pgxpool.Pool
	engine         *games.Service
	service        *botgames.Service
	declaration    *accessDeclaration
	now            time.Time
	next           int64
	connection     struct{ OwnerURL, NativeURL, NativeRole, RuntimeRole string }
}

func botGamesPGCheck(t *testing.T, err error, stage string) {
	t.Helper()
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) {
			t.Fatalf("%s failed (SQLSTATE %s)", stage, pg.Code)
		}
		t.Fatalf("%s failed (%T)", stage, err)
	}
}
func (f *botGamesPGFixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	e := f.owner.WithTx(context.Background(), func(tx pgx.Tx) error { _, e := tx.Exec(context.Background(), sql, args...); return e })
	botGamesPGCheck(t, e, "fixture SQL")
}
func newBotGamesPGFixture(t *testing.T) *botGamesPGFixture {
	t.Helper()
	owner, runtime := gameBrowserStores(t)
	f := &botGamesPGFixture{owner: owner, runtime: runtime, now: time.Now().UTC(), next: 937000000, declaration: &accessDeclaration{MigrationApplicability: "NO_MIGRATION_APPLICABLE", Resources: map[string]string{"EXPERIENCE": "AVAILABLE"}}}
	raw, e := os.ReadFile(os.Getenv("MOMIAO_GAMES_TEST_CONNECTION_FILE"))
	if e != nil || json.Unmarshal(raw, &f.connection) != nil {
		t.Fatal("isolated fixture connection unreadable")
	}
	c, e := pgx.ParseConfig(f.connection.NativeURL)
	if e != nil || c.Host != "127.0.0.1" || c.Port != 55432 || !strings.HasPrefix(c.Database, "momiao_test_g1_original_") {
		t.Fatal("refusing non-G1 Native fixture")
	}
	f.exec(t, `CREATE TABLE public.users(id bigint PRIMARY KEY,discord_id text NOT NULL DEFAULT '',status int NOT NULL DEFAULT 1,deleted_at timestamptz,quota bigint NOT NULL DEFAULT 0)`)
	f.exec(t, `GRANT SELECT(id,discord_id,status,deleted_at) ON public.users TO `+pgx.Identifier{f.connection.NativeRole}.Sanitize())
	// The reused browser fixture predates the current games row lock / maintenance guard.
	f.exec(t, `GRANT EXECUTE ON FUNCTION ops.lock_write_scopes(text[]),ops.is_maintenance_scope_active(text) TO `+pgx.Identifier{f.connection.RuntimeRole}.Sanitize())
	f.exec(t, `GRANT UPDATE(status) ON games.game_config_versions TO `+pgx.Identifier{f.connection.RuntimeRole}.Sanitize())
	poolConfig, e := pgxpool.ParseConfig(f.connection.NativeURL)
	botGamesPGCheck(t, e, "Native pool config")
	poolConfig.MaxConns = 4
	f.native, e = pgxpool.NewWithConfig(context.Background(), poolConfig)
	botGamesPGCheck(t, e, "Native pool")
	t.Cleanup(f.native.Close)
	observer, e := platform.OpenNativeQuota(context.Background(), f.connection.OwnerURL)
	botGamesPGCheck(t, e, "synthetic quota observer")
	t.Cleanup(observer.Close)
	f.engine, e = games.NewServiceWithEconomy(runtime, games.Keyring{Active: "bot-fixture-v1", Keys: map[string][32]byte{"bot-fixture-v1": {9}}}, observer)
	botGamesPGCheck(t, e, "engine")
	f.service, e = botgames.NewService(f.engine, botGamesResolver{native: f.native, platform: runtime, declaration: f.declaration}, [32]byte{2}, func() time.Time { return f.now })
	botGamesPGCheck(t, e, "bot service")
	return f
}
func (f *botGamesPGFixture) player(t *testing.T, ready bool, balance int64) (int64, string) {
	t.Helper()
	f.next++
	u := f.next
	s := strconv.FormatInt(u, 10)
	f.exec(t, `INSERT INTO public.users(id,discord_id) VALUES($1,$2)`, u, s)
	if ready {
		botGamesPGCheck(t, f.owner.EnsureAccount(context.Background(), u), "account fixture")
		_, e := f.owner.InitializeProfile(context.Background(), u, 0, "Bot"+s, "system-default")
		botGamesPGCheck(t, e, "profile fixture")
		if balance > 0 {
			_, e = f.owner.Apply(context.Background(), platform.Mutation{UserID: u, Asset: platform.AvailableChips, DeltaUnits: balance, BizType: "BOT_TEST", BizID: s, EntryType: "TEST_GRANT", IdempotencyKey: "bot-fixture:" + s})
			botGamesPGCheck(t, e, "wallet fixture")
		}
	}
	return u, s
}
func (f *botGamesPGFixture) prepare(t *testing.T, s string) botgames.Prepared {
	t.Helper()
	f.next++
	p, e := f.service.Prepare(context.Background(), s, botgames.PrepareInput{RequestID: strconv.FormatInt(f.next, 10), Wager: "10", Choice: "BIG"})
	botGamesPGCheck(t, e, "prepare")
	return p
}
func (f *botGamesPGFixture) effects(t *testing.T, u int64, want int) {
	t.Helper()
	var rounds, wagers, settlements int
	e := f.owner.WithTx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM games.game_rounds WHERE newapi_user_id=$1),(SELECT count(*) FROM economy.asset_transactions WHERE newapi_user_id=$1 AND biz_type='GAME_WAGER'),(SELECT count(*) FROM economy.asset_transactions WHERE newapi_user_id=$1 AND biz_type='GAME_SETTLEMENT')`, u).Scan(&rounds, &wagers, &settlements)
	})
	botGamesPGCheck(t, e, "effect counts")
	if rounds != want || wagers != want || settlements != want {
		t.Fatalf("effects rounds=%d wagers=%d settlements=%d want=%d", rounds, wagers, settlements, want)
	}
}
func botGamesWantFault(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || err.Error() != want {
		t.Fatalf("fault=%v want=%s", err, want)
	}
}
func (f *botGamesPGFixture) balance(t *testing.T, u int64) int64 {
	t.Helper()
	w, e := f.owner.ReadWallet(context.Background(), u, platform.AvailableChips)
	botGamesPGCheck(t, e, "wallet read")
	return w.BalanceUnits
}
func TestBotGamesPostgres(t *testing.T) {
	f := newBotGamesPGFixture(t)
	ctx := context.Background()
	t.Run("native_read_only", func(t *testing.T) {
		_, e := f.native.Exec(ctx, `UPDATE public.users SET status=2`)
		if e == nil {
			t.Fatal("Native reader updated users")
		}
		var pg *pgconn.PgError
		if !errors.As(e, &pg) || (pg.Code != "25006" && pg.Code != "42501") {
			t.Fatal("UPDATE did not fail on read-only/ACL")
		}
		tx, e := f.native.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadWrite})
		botGamesPGCheck(t, e, "explicit read-write probe")
		defer tx.Rollback(ctx)
		_, e = tx.Exec(ctx, `UPDATE public.users SET status=2`)
		if !errors.As(e, &pg) || pg.Code != "42501" {
			t.Fatal("Native column-only ACL did not reject UPDATE")
		}
	})
	t.Run("admission_zero_effects", func(t *testing.T) {
		for _, kind := range []string{"unlinked", "disabled", "deleted", "duplicate_disabled", "missing_ref", "incomplete_profile", "migration_required", "migration_unverified", "experience_maintenance", "experience_unavailable"} {
			t.Run(kind, func(t *testing.T) {
				u, s := f.player(t, kind != "missing_ref" && kind != "incomplete_profile", 50000000)
				want := "ACCOUNT_NOT_READY"
				switch kind {
				case "unlinked":
					s = "18446744073709551615"
					want = "NOT_LINKED"
				case "disabled":
					f.exec(t, `UPDATE public.users SET status=2 WHERE id=$1`, u)
					want = "ACCOUNT_RESTRICTED"
				case "deleted":
					f.exec(t, `UPDATE public.users SET deleted_at=now() WHERE id=$1`, u)
					want = "ACCOUNT_RESTRICTED"
				case "duplicate_disabled":
					f.exec(t, `INSERT INTO public.users(id,discord_id,status) VALUES($1,$2,2)`, u+900000000, s)
					want = "ACCOUNT_RESTRICTED"
				case "incomplete_profile":
					botGamesPGCheck(t, f.owner.EnsureAccount(ctx, u), "incomplete account fixture")
				case "migration_required":
					f.exec(t, `INSERT INTO identity.migration_notice_versions(version,title,body,completed_at,evidence_ref) VALUES($1,'Synthetic','Fixture only',now(),'bot-fixture')`, u)
					f.exec(t, `INSERT INTO identity.migration_notice_requirements(newapi_user_id,version) VALUES($1,$1)`, u)
				case "migration_unverified":
					f.declaration.MigrationApplicability = "UNVERIFIED"
					defer func() { f.declaration.MigrationApplicability = "NO_MIGRATION_APPLICABLE" }()
				case "experience_maintenance":
					f.declaration.Resources["EXPERIENCE"] = "MAINTENANCE"
					defer func() { f.declaration.Resources["EXPERIENCE"] = "AVAILABLE" }()
					want = "MAINTENANCE"
				case "experience_unavailable":
					delete(f.declaration.Resources, "EXPERIENCE")
					defer func() { f.declaration.Resources["EXPERIENCE"] = "AVAILABLE" }()
					want = "MAINTENANCE"
				}
				_, e := f.service.Prepare(ctx, s, botgames.PrepareInput{RequestID: "970000000000000001", Wager: "10", Choice: "BIG"})
				botGamesWantFault(t, e, want)
				f.effects(t, u, 0)
				if kind == "missing_ref" {
					exists, e := f.owner.AccountRefExists(ctx, u)
					botGamesPGCheck(t, e, "missing ref recheck")
					if exists {
						t.Fatal("resolver initialized missing account")
					}
				}
				if kind == "incomplete_profile" {
					var rows int
					e := f.owner.WithTx(ctx, func(tx pgx.Tx) error {
						return tx.QueryRow(ctx, `SELECT count(*) FROM identity.master_profiles WHERE newapi_user_id=$1`, u).Scan(&rows)
					})
					botGamesPGCheck(t, e, "missing profile recheck")
					if rows != 0 {
						t.Fatal("resolver initialized a profile")
					}
				}
				if kind == "migration_required" {
					notice, e := f.owner.ReadMigrationNotice(ctx, u, true)
					botGamesPGCheck(t, e, "notice recheck")
					if notice.State != "REQUIRED" {
						t.Fatal("resolver acknowledged migration")
					}
				}
			})
		}
	})
	t.Run("binding_changed", func(t *testing.T) {
		u, s := f.player(t, true, 50000000)
		other, _ := f.player(t, true, 50000000)
		p := f.prepare(t, s)
		f.exec(t, `UPDATE public.users SET discord_id='' WHERE id=$1`, u)
		f.exec(t, `UPDATE public.users SET discord_id=$2 WHERE id=$1`, other, s)
		_, e := f.service.Play(ctx, s, p.Quote)
		botGamesWantFault(t, e, "BINDING_CHANGED")
		_, e = f.service.Lookup(ctx, s, p.Quote)
		botGamesWantFault(t, e, "BINDING_CHANGED")
		f.effects(t, u, 0)
		f.effects(t, other, 0)
	})
	t.Run("real_bootstrap_replay_20", func(t *testing.T) {
		u, s := f.player(t, true, 50000000)
		b, e := f.engine.Bootstrap(ctx, u, "dice")
		botGamesPGCheck(t, e, "real bootstrap")
		if b.WagerPolicy.MaximumMode != "NONE" || b.WagerPolicy.Step != 500000 || b.WagerPolicy.Minimum != 5000000 {
			t.Fatal("real wager policy incompatible")
		}
		p := f.prepare(t, s)
		var wg sync.WaitGroup
		results := make(chan botgames.Result, 20)
		errs := make(chan error, 20)
		before := f.balance(t, u)
		for range 20 {
			wg.Add(1)
			go func() { defer wg.Done(); r, e := f.service.Play(ctx, s, p.Quote); results <- r; errs <- e }()
		}
		wg.Wait()
		close(results)
		close(errs)
		for e := range errs {
			botGamesPGCheck(t, e, "concurrent replay")
		}
		var first botgames.Result
		for r := range results {
			if first.RoundID != "" && r != first {
				t.Fatal("replay projection changed")
			}
			first = r
		}
		if first.Wager != "10" || first.StakeUnits != "5000000" || first.Status != "SETTLED" {
			t.Fatal("10 Chip result invalid")
		}
		net, e := strconv.ParseInt(first.ActualNetUnits, 10, 64)
		botGamesPGCheck(t, e, "actual net parse")
		if f.balance(t, u)-before != net {
			t.Fatal("wallet delta differs from actual_net")
		}
		f.effects(t, u, 1)
		for _, in := range []botgames.PrepareInput{{RequestID: strconv.FormatInt(f.next, 10), Wager: "11", Choice: "BIG"}, {RequestID: strconv.FormatInt(f.next, 10), Wager: "10", Choice: "SMALL"}} {
			q, e := f.service.Prepare(ctx, s, in)
			botGamesPGCheck(t, e, "changed prepare")
			_, e = f.service.Play(ctx, s, q.Quote)
			botGamesWantFault(t, e, "IDEMPOTENCY_CONFLICT")
		}
		f.now = f.now.Add(121 * time.Second)
		defer func() { f.now = f.now.Add(-121 * time.Second) }()
		recovered, e := f.service.Lookup(ctx, s, p.Quote)
		botGamesPGCheck(t, e, "lost response expired lookup")
		replayed, e := f.service.Play(ctx, s, p.Quote)
		botGamesPGCheck(t, e, "expired replay")
		if recovered != first || replayed != first {
			t.Fatal("expired recovery changed result")
		}
		f.effects(t, u, 1)
		f.exec(t, `UPDATE public.users SET status=2 WHERE id=$1`, u)
		_, e = f.service.Lookup(ctx, s, p.Quote)
		botGamesWantFault(t, e, "ACCOUNT_RESTRICTED")
		_, e = f.service.Play(ctx, s, p.Quote)
		botGamesWantFault(t, e, "ACCOUNT_RESTRICTED")
	})
	t.Run("expired_no_new_round", func(t *testing.T) {
		u, s := f.player(t, true, 50000000)
		p := f.prepare(t, s)
		f.now = f.now.Add(120 * time.Second)
		defer func() { f.now = f.now.Add(-120 * time.Second) }()
		_, e := f.service.Play(ctx, s, p.Quote)
		botGamesWantFault(t, e, "QUOTE_EXPIRED")
		f.effects(t, u, 0)
	})
	t.Run("browser_consumed_commitment", func(t *testing.T) {
		u, s := f.player(t, true, 50000000)
		p := f.prepare(t, s)
		_, e := f.engine.Create(ctx, u, "dice", "browser:"+s, p.CommitmentID, games.CreateInput{Type: "DICE", Wager: "10", Choice: "BIG"})
		botGamesPGCheck(t, e, "browser consume")
		before := f.balance(t, u)
		_, e = f.service.Play(ctx, s, p.Quote)
		botGamesWantFault(t, e, "COMMITMENT_INVALID")
		if f.balance(t, u) != before {
			t.Fatal("failed play changed wallet")
		}
		f.effects(t, u, 1)
	})
	t.Run("insufficient_chips", func(t *testing.T) {
		u, s := f.player(t, true, 5000000)
		p := f.prepare(t, s)
		_, e := f.owner.Apply(ctx, platform.Mutation{UserID: u, Asset: platform.AvailableChips, DeltaUnits: -5000000, BizType: "BOT_TEST", BizID: "drain:" + s, EntryType: "TEST_DRAIN", IdempotencyKey: "bot-drain:" + s})
		botGamesPGCheck(t, e, "synthetic drain")
		_, e = f.service.Play(ctx, s, p.Quote)
		botGamesWantFault(t, e, "INSUFFICIENT_CHIPS")
		_, e = f.service.Prepare(ctx, s, botgames.PrepareInput{RequestID: s, Wager: "10", Choice: "BIG"})
		botGamesWantFault(t, e, "INSUFFICIENT_CHIPS")
		if f.balance(t, u) != 0 {
			t.Fatal("failed play changed empty balance")
		}
		f.effects(t, u, 0)
	})
	t.Run("maintenance_after_prepare", func(t *testing.T) {
		u, s := f.player(t, true, 50000000)
		p := f.prepare(t, s)
		before := f.balance(t, u)
		f.exec(t, `UPDATE games.runtime_gate SET maintenance=true WHERE singleton`)
		defer f.exec(t, `UPDATE games.runtime_gate SET maintenance=false WHERE singleton`)
		_, e := f.service.Play(ctx, s, p.Quote)
		botGamesWantFault(t, e, "MAINTENANCE")
		if f.balance(t, u) != before {
			t.Fatal("maintenance changed balance")
		}
		f.effects(t, u, 0)
	})
	t.Run("acknowledged_admission", func(t *testing.T) {
		u, s := f.player(t, true, 50000000)
		f.exec(t, `INSERT INTO identity.migration_notice_versions(version,title,body,completed_at,evidence_ref) VALUES($1,'Synthetic','Fixture only',now(),'bot-fixture')`, u)
		f.exec(t, `INSERT INTO identity.migration_notice_requirements(newapi_user_id,version) VALUES($1,$1)`, u)
		_, e := f.owner.AcknowledgeMigrationNotice(ctx, u, u)
		botGamesPGCheck(t, e, "synthetic acknowledgment")
		_ = f.prepare(t, s)
		f.effects(t, u, 0)
	})
	t.Run("private_application", func(t *testing.T) {
		u, s := f.player(t, true, 50000000)
		dir, e := os.MkdirTemp("", "bg-")
		botGamesPGCheck(t, e, "socket directory")
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		cfg := config{ProcessRole: "platform", WalletDSNFile: filepath.Join(dir, "wallet"), GameFairnessKeyringFile: filepath.Join(dir, "fairness"), accessDeclaration: f.declaration, BotGames: botGamesConfig{Socket: filepath.Join(dir, "b.sock"), TokenFile: filepath.Join(dir, "token"), QuoteKeyFile: filepath.Join(dir, "quote"), NativeDSNFile: filepath.Join(dir, "native")}}
		fairnessKey := [32]byte{9}
		quoteKey := [32]byte{2}
		for p, v := range map[string]string{cfg.BotGames.TokenFile: botGamesTestToken, cfg.BotGames.QuoteKeyFile: hex.EncodeToString(quoteKey[:]), cfg.BotGames.NativeDSNFile: f.connection.NativeURL, cfg.GameFairnessKeyringFile: `{"active":"bot-fixture-v1","keys":{"bot-fixture-v1":"` + hex.EncodeToString(fairnessKey[:]) + `"}}`} {
			botGamesPGCheck(t, os.WriteFile(p, []byte(v), 0600), "private fixture file")
		}
		t.Run("reject_empty_socket", func(t *testing.T) {
			bad := cfg
			bad.BotGames.Socket = ""
			a, e := openBotGamesApplication(ctx, bad, f.runtime, f.engine)
			if e == nil {
				_ = a.Close()
				t.Fatal("incomplete config opened a TCP listener")
			}
		})
		t.Run("reject_equal_keys", func(t *testing.T) {
			botGamesPGCheck(t, os.WriteFile(cfg.BotGames.QuoteKeyFile, []byte(botGamesTestToken), 0600), "equal key fixture")
			defer func() {
				botGamesPGCheck(t, os.WriteFile(cfg.BotGames.QuoteKeyFile, []byte(hex.EncodeToString(quoteKey[:])), 0600), "restore quote key")
			}()
			if a, e := openBotGamesApplication(ctx, cfg, f.runtime, f.engine); e == nil {
				_ = a.Close()
				t.Fatal("shared token/quote material accepted")
			}
		})
		a, e := openBotGamesApplication(ctx, cfg, f.runtime, f.engine)
		botGamesPGCheck(t, e, "application open")
		defer a.Close()
		if a.native.Config().MaxConns != 4 {
			t.Fatal("unbounded Native pool")
		}
		info, e := os.Stat(cfg.BotGames.Socket)
		botGamesPGCheck(t, e, "socket stat")
		if info.Mode().Perm() != 0600 {
			t.Fatalf("socket mode=%o", info.Mode().Perm())
		}
		if a.server.ReadHeaderTimeout != time.Second || a.server.ReadTimeout != 2*time.Second || a.server.WriteTimeout != 10*time.Second || a.server.IdleTimeout != 10*time.Second || a.server.MaxHeaderBytes != 4096 {
			t.Fatal("application timeouts differ")
		}
		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- a.Run(runCtx, time.Second) }()
		defer func() { cancel(); botGamesPGCheck(t, <-done, "application shutdown") }()
		transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", cfg.BotGames.Socket)
		}}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
		request := botGamesRequest("POST", "http://private/internal/v1/bot-games/dice/prepare", fmt.Sprintf(`{"request_id":%q,"wager":"10","choice":"BIG"}`, s))
		request.RequestURI = ""
		request.Header.Set("X-Discord-User", s)
		response, e := client.Do(request)
		botGamesPGCheck(t, e, "socket request")
		defer response.Body.Close()
		body, e := io.ReadAll(response.Body)
		botGamesPGCheck(t, e, "socket body")
		if response.StatusCode != 200 {
			t.Fatalf("private prepare status=%d body=%s", response.StatusCode, body)
		}
		var p botgames.Prepared
		botGamesPGCheck(t, json.Unmarshal(body, &p), "prepared JSON")
		if p.Quote == "" || p.Wager != "10" {
			t.Fatal("private response missing quote")
		}
		before := f.balance(t, u)
		var original botgames.Result
		for _, operation := range []string{"play", "lookup"} {
			payload, _ := json.Marshal(map[string]string{"quote": p.Quote})
			request := botGamesRequest("POST", "http://private/internal/v1/bot-games/dice/"+operation, string(payload))
			request.RequestURI = ""
			request.Header.Set("X-Discord-User", s)
			response, err := client.Do(request)
			botGamesPGCheck(t, err, "private "+operation)
			body, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			botGamesPGCheck(t, err, "result body")
			if response.StatusCode != 200 {
				t.Fatalf("private %s status=%d body=%s", operation, response.StatusCode, body)
			}
			var result botgames.Result
			var fields map[string]json.RawMessage
			botGamesPGCheck(t, json.Unmarshal(body, &result), "result JSON")
			botGamesPGCheck(t, json.Unmarshal(body, &fields), "result fields")
			allowed := []string{"round_id", "created_at", "settled_at", "status", "wager", "choice", "dice", "total", "result", "stake_units", "gross_payout_units", "withheld_units", "credited_payout_units", "actual_net_units", "ruleset"}
			if len(fields) != len(allowed) {
				t.Fatal("private result has unprojected fields")
			}
			for _, field := range allowed {
				if _, ok := fields[field]; !ok {
					t.Fatalf("private result missing %s", field)
				}
			}
			if operation == "play" {
				original = result
			} else if result != original {
				t.Fatal("HTTP lookup changed result")
			}
		}
		net, e := strconv.ParseInt(original.ActualNetUnits, 10, 64)
		botGamesPGCheck(t, e, "HTTP actual net")
		if f.balance(t, u)-before != net {
			t.Fatal("HTTP result differs from durable wallet change")
		}
		f.effects(t, u, 1)
	})
	t.Run("asset_cap_actual_net", func(t *testing.T) {
		f.exec(t, platform.NativeQuotaMigration)
		f.exec(t, `UPDATE momiao_quota.settings SET enabled=true; UPDATE economy.policy_runtime SET active_version='economy-cap-v1' WHERE singleton`, pgx.QueryExecModeSimpleProtocol)
		defer f.exec(t, `UPDATE economy.policy_runtime SET active_version=NULL WHERE singleton`)
		u, s := f.player(t, true, 2000000000)
		clipped := false
		for i := 0; i < 64; i++ {
			balance := f.balance(t, u)
			reserve, e := f.owner.ReadWallet(ctx, u, platform.ReserveAPICredit)
			botGamesPGCheck(t, e, "reserve read")
			fill := platform.AssetCapUnits - balance - reserve.BalanceUnits
			if fill > 0 {
				_, e = f.owner.Apply(ctx, platform.Mutation{UserID: u, Asset: platform.ReserveAPICredit, DeltaUnits: fill, BizType: "BOT_CAP_TEST", BizID: fmt.Sprintf("%s:%d", s, i), EntryType: "TEST_GRANT", IdempotencyKey: fmt.Sprintf("bot-cap:%s:%d", s, i)})
				botGamesPGCheck(t, e, "cap fill")
			}
			p := f.prepare(t, s)
			r, e := f.service.Play(ctx, s, p.Quote)
			botGamesPGCheck(t, e, "capped play")
			net, e := strconv.ParseInt(r.ActualNetUnits, 10, 64)
			botGamesPGCheck(t, e, "cap net parse")
			if f.balance(t, u)-balance != net {
				t.Fatal("cap wallet delta != actual_net")
			}
			replay, e := f.service.Lookup(ctx, s, p.Quote)
			botGamesPGCheck(t, e, "cap replay")
			if replay != r {
				t.Fatal("cap result changed")
			}
			if r.WithheldUnits != "0" {
				if r.Result != "WIN" || r.ActualNetUnits != "0" || r.CreditedPayoutUnits != "5000000" || r.GrossPayoutUnits != "10000000" || r.WithheldUnits != "5000000" {
					t.Fatal("raw win and capped net conflated")
				}
				clipped = true
				f.effects(t, u, i+1)
				break
			}
		}
		if !clipped {
			t.Fatal("no capped win in 64 rounds")
		}
	})
}
