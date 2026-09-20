package poker

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func profileResult(t *testing.T, got, want, cause error) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Error("compatible profiles were rejected")
		}
		return
	}
	if !errors.Is(got, want) || cause != nil && !errors.Is(got, cause) {
		t.Error("expected controlled result or context cause is missing")
		return
	}
	allowed := []string{want.Error()}
	if cause != nil {
		allowed = append(allowed, errors.Join(want, cause).Error())
	} else if want == ErrPasswordUnavailable {
		allowed = append(allowed, errors.Join(want, context.DeadlineExceeded).Error())
	}
	if !slices.Contains(allowed, got.Error()) {
		t.Error("result includes unexpected provider details")
	}
}

func profilePHC(t *testing.T, memory uint32) string {
	t.Helper()
	config := PasswordConfig{memory, 1, 1}
	phc, err := HashTablePassword(config, "profile fixture password")
	ok, verifyErr := VerifyTablePassword(config, phc, "profile fixture password")
	if err != nil || verifyErr != nil || !ok {
		t.Fatal("real P1 Hash and Verify fixture failed")
	}
	return phc
}

func profileInsert(t *testing.T, owner *pgxpool.Pool, phc, lifecycle string) {
	t.Helper()
	table := uuid()
	if _, err := owner.Exec(t.Context(), `INSERT INTO poker.tables
 (table_id,owner_newapi_user_id,name,max_seats,blind_preset_version,ruleset_version,lifecycle_state)
 VALUES($1,910001,'profile-fixture',2,'5-10','poker-cash-v1-20260906',$2)`, table, lifecycle); err != nil {
		t.Fatal("fixture table insertion failed")
	}
	if _, err := owner.Exec(t.Context(), "INSERT INTO poker.table_access_credentials(table_id,password_phc) VALUES($1,$2)", table, phc); err != nil {
		t.Fatal("fixture credential insertion failed")
	}
}

func profileSnapshot(t *testing.T, owner *pgxpool.Pool) string {
	t.Helper()
	var value string
	err := owner.QueryRow(t.Context(), `SELECT COALESCE(jsonb_agg(to_jsonb(c) ORDER BY table_id),'[]'::jsonb)::text
 FROM poker.table_access_credentials c`).Scan(&value)
	if err != nil {
		t.Fatal("fixture credential snapshot failed")
	}
	return value
}

func profileCheck(t *testing.T, owner, pool *pgxpool.Pool, runtime *PasswordRuntime, want error) {
	t.Helper()
	before := profileSnapshot(t, owner)
	profileResult(t, CheckPasswordProfiles(t.Context(), pool, runtime), want, nil)
	if profileSnapshot(t, owner) != before {
		t.Error("profile scan changed immutable rows, identifiers, PHC components or timestamps")
	}
}

func TestPasswordProfilesEmptyAndProfiles(t *testing.T) {
	owner, pool, _, _ := passwordProfileFixture(t, false)
	configs := []PasswordConfig{{64, 1, 1}, {128, 1, 1}, {192, 1, 1}, {256, 1, 1}}
	build := func(t *testing.T, profiles []PasswordConfig) *PasswordRuntime {
		return runtimeForTest(t, PasswordPolicy{Current: profiles[0], VerifyProfiles: profiles, MaxConcurrent: 1, MaxWorkMemoryKiB: 256})
	}
	stages := []struct {
		name string
		rows int
		want error
	}{
		{"empty_no_config", 0, nil},
		{"empty_configured", 0, nil},
		{"zero_runtime", 0, ErrPasswordConfig},
		{"nil_config_with_row", 1, ErrPasswordConfig},
		{"current_roundtrip", 1, nil},
		{"slot_independent", 1, nil},
		{"closed_profile_removed", 2, ErrPasswordConfig},
		{"closed_profile_retained", 2, nil},
		{"four_profiles", 4, nil},
		{"each_profile_required", 4, ErrPasswordConfig},
		{"fifth_profile", 5, ErrPasswordConfig},
		{"read_only_preserves_rows", 5, ErrPasswordConfig},
	}
	rows := 0
	for _, stage := range stages {
		t.Run(stage.name, func(t *testing.T) {
			for rows < stage.rows {
				lifecycle := "CLOSED"
				if rows == 0 {
					lifecycle = "WAITING"
				}
				profileInsert(t, owner, profilePHC(t, uint32(rows+1)*64), lifecycle)
				rows++
			}
			runtime := build(t, configs[:1])
			switch stage.name {
			case "empty_no_config", "nil_config_with_row":
				runtime = nil
			case "zero_runtime":
				runtime = &PasswordRuntime{}
			case "closed_profile_retained":
				runtime = build(t, configs[:2])
			case "four_profiles", "fifth_profile", "read_only_preserves_rows":
				runtime = build(t, configs)
			case "each_profile_required":
				for i := range configs {
					remaining := append(slices.Clone(configs[:i]), configs[i+1:]...)
					profileCheck(t, owner, pool, build(t, remaining), ErrPasswordConfig)
				}
				return
			case "slot_independent":
				runtime.slots <- struct{}{}
				defer func() {
					select {
					case <-runtime.slots:
					default:
					}
				}()
			}
			profileCheck(t, owner, pool, runtime, stage.want)
			if stage.name == "slot_independent" && len(runtime.slots) != 1 {
				t.Error("profile scan consumed or released the occupied KDF slot")
			}
		})
	}
}

func TestPasswordProfilesMalformed(t *testing.T) {
	seed := profilePHC(t, 64)
	parts := strings.Split(seed, "$")
	salt, key := parts[4], parts[5]
	cases := []struct{ name, from, to string }{
		{"algorithm", "argon2id", "argon2i"},
		{"version", "v=19", "v=16"},
		{"order", "m=64,t=1,p=1", "t=1,m=64,p=1"},
		{"leading_zero", "m=64", "m=064"},
		{"zero_cost", "t=1", "t=0"},
		{"signed_cost", "m=64", "m=+64"},
		{"leading_junk", seed, "!" + seed},
		{"trailing_junk", seed, seed + "!"},
		{"extra_separator", seed, seed + "$"},
		{"salt_short", salt, salt[:21]},
		{"salt_long", salt, salt + "A"},
		{"key_short", key, key[:42]},
		{"key_long", key, key + "A"},
		{"salt_padding_bits", salt, salt[:21] + "B"},
		{"key_padding_bits", key, key[:42] + "B"},
		{"explicit_padding", salt, salt + "="},
		{"salt_urlalphabet", salt, "-" + salt[1:]},
		{"key_urlalphabet", key, "_" + key[1:]},
		{"salt_cr", salt, salt[:10] + "\r" + salt[10:]},
		{"key_lf", key, key[:10] + "\n" + key[10:]},
		{"trailing_lf", seed, seed + "\n"},
		{"salt_nonascii", salt, "御" + salt[1:]},
		{"key_nonascii", key, "御" + key[1:]},
		{"garbage", seed, "garbage"},
	}
	fixture := passwordProfileFixtureFactory(t, false)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			phc := strings.Replace(seed, tc.from, tc.to, 1)
			ok, err := VerifyTablePassword(PasswordConfig{64, 1, 1}, phc, "profile fixture password")
			if ok || !errors.Is(err, ErrInvalid) {
				t.Fatal("malformed fixture did not fail the real P1 strict parser")
			}
			owner, pool, _, _ := fixture(t)
			profileInsert(t, owner, phc, "CLOSED")
			profileCheck(t, owner, pool, runtimeForTest(t, runtimePolicy()), ErrPasswordConfig)
		})
	}
}

type profileTraceKey struct{}
type profileTrace struct {
	seen              []string
	cancelKind        string
	cancel            context.CancelFunc
	readOnly, bounded bool
}

func (trace *profileTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	sql, kind := strings.ToLower(strings.Join(strings.Fields(data.SQL), " ")), ""
	switch {
	case strings.HasPrefix(sql, "begin"):
		kind = "begin"
		trace.readOnly = sql == "begin read only"
		deadline, ok := ctx.Deadline()
		trace.bounded = ok && time.Until(deadline) <= 2100*time.Millisecond
	case strings.ReplaceAll(sql, " ", "") == "setlocalstatement_timeout='2s'":
		kind = "timeout"
	case strings.HasPrefix(sql, "select not exists"):
		kind = "check"
	case sql == "commit":
		kind = "commit"
	}
	return context.WithValue(ctx, profileTraceKey{}, kind)
}

func (trace *profileTrace) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	kind, _ := ctx.Value(profileTraceKey{}).(string)
	if data.Err == nil && kind != "" {
		trace.seen = append(trace.seen, kind)
		if kind == trace.cancelKind && trace.cancel != nil {
			trace.cancel()
		}
	}
}

func profileSettings(t *testing.T, pool *pgxpool.Pool) [2]string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	var settings [2]string
	if err := pool.QueryRow(ctx, "SELECT current_setting('transaction_read_only'),current_setting('statement_timeout')").Scan(&settings[0], &settings[1]); err != nil {
		t.Fatal("single-connection pool was not reusable after profile scan")
	}
	return settings
}

func TestPasswordProfilesFailures(t *testing.T) {
	names := []string{
		"nil_pool", "closed_pool", "missing_table", "revoked_select", "platform_role", "ops_role",
		"readonly_and_timeout", "nil_context", "precancelled", "expired", "blocked_default",
		"blocked_caller", "cancel_after_query", "pool_reuse", "cancel_after_commit",
	}
	fixture := passwordProfileFixtureFactory(t, false)
	raw21Fixture := passwordProfileFixtureFactory(t, true)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			runtime := runtimeForTest(t, runtimePolicy())
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var cause error
			if slices.Contains([]string{"nil_pool", "nil_context", "precancelled", "expired"}, name) {
				pool := &pgxpool.Pool{} // Context guards must precede any use; never a query replacement.
				switch name {
				case "nil_pool":
					pool = nil
				case "nil_context":
					ctx = nil
				case "precancelled":
					cancel()
					cause = context.Canceled
				case "expired":
					ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
					defer cancel()
					cause = context.DeadlineExceeded
				}
				profileResult(t, CheckPasswordProfiles(ctx, pool, runtime), ErrPasswordUnavailable, cause)
				return
			}
			open := fixture
			if name == "missing_table" {
				open = raw21Fixture
			}
			owner, pool, platform, ops := open(t)
			if name == "missing_table" {
				var absent bool
				err := owner.QueryRow(ctx, "SELECT pg_catalog.to_regclass('poker.table_access_credentials') IS NULL").Scan(&absent)
				if err != nil || !absent || ctx.Err() != nil {
					t.Fatal("raw21 owner must successfully prove credential table absence before C2")
				}
				t.Log("raw21 owner absence precondition passed")
				profileResult(t, CheckPasswordProfiles(ctx, pool, runtime), ErrPasswordUnavailable, nil)
				return
			}
			before := profileSnapshot(t, owner)
			trace := &profileTrace{cancel: cancel}
			config := pool.Config()
			pool.Close()
			config.ConnConfig.Tracer = trace
			pool, err := pgxpool.NewWithConfig(ctx, config)
			if err != nil {
				t.Fatal("traced actual Poker pool construction failed")
			}
			t.Cleanup(pool.Close)
			baseline := profileSettings(t, pool)
			if baseline[0] != "off" {
				t.Fatal("fixture connection defaults must not supply readonly transactions")
			}
			want := ErrPasswordUnavailable
			var lock pgx.Tx
			switch name {
			case "closed_pool":
				pool.Close()
			case "revoked_select":
				var canSelect, canInsert bool
				role := pool.Config().ConnConfig.User
				err = owner.QueryRow(ctx, "SELECT has_table_privilege($1,'poker.table_access_credentials','SELECT'),has_table_privilege($1,'poker.table_access_credentials','INSERT')", role).Scan(&canSelect, &canInsert)
				if err != nil || !canSelect || !canInsert {
					t.Fatal("revocation fixture must first have required Poker SELECT and INSERT")
				}
				if _, err = owner.Exec(ctx, "REVOKE SELECT ON poker.table_access_credentials FROM "+pgx.Identifier{role}.Sanitize()); err != nil {
					t.Fatal("exact self-owned credential SELECT revocation failed")
				}
				err = owner.QueryRow(ctx, "SELECT has_table_privilege($1,'poker.table_access_credentials','SELECT')", role).Scan(&canSelect)
				if err != nil || canSelect {
					t.Fatal("required Poker SELECT remained after revocation")
				}
			case "platform_role":
				pool = platform
			case "ops_role":
				pool = ops
			case "readonly_and_timeout", "pool_reuse":
				want = nil
			case "cancel_after_query":
				profileInsert(t, owner, profilePHC(t, 128), "CLOSED")
				before = profileSnapshot(t, owner)
				trace.cancelKind, cause = "check", context.Canceled
			case "cancel_after_commit":
				trace.cancelKind, cause = "commit", context.Canceled
			case "blocked_default", "blocked_caller":
				lock, err = owner.Begin(ctx)
				if err != nil {
					t.Fatal("self-owned blocking transaction failed")
				}
				defer func() {
					cleanup, stop := context.WithTimeout(context.Background(), time.Second)
					defer stop()
					if err := lock.Rollback(cleanup); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
						t.Error("self-owned table lock cleanup failed")
					}
				}()
				if _, err = lock.Exec(ctx, "LOCK TABLE poker.table_access_credentials IN ACCESS EXCLUSIVE MODE"); err != nil {
					t.Fatal("self-owned credential table lock was not acknowledged")
				}
				if name == "blocked_caller" {
					ctx, cancel = context.WithTimeout(t.Context(), 100*time.Millisecond)
					defer cancel()
					cause = context.DeadlineExceeded
				}
			}
			start := time.Now()
			result := CheckPasswordProfiles(ctx, pool, runtime)
			elapsed := time.Since(start)
			if lock != nil {
				cleanup, stop := context.WithTimeout(t.Context(), time.Second)
				err = lock.Rollback(cleanup)
				stop()
				if err != nil {
					t.Fatal("blocking fixture release failed")
				}
				upper := 3 * time.Second
				if name == "blocked_caller" {
					upper = time.Second
				}
				if elapsed >= upper || !trace.bounded {
					t.Error("blocked scan exceeded caller or internal deadline")
				}
			}
			profileResult(t, result, want, cause)
			if trace.cancelKind != "" && !slices.Contains(trace.seen, trace.cancelKind) {
				t.Error("cancellation did not follow the required successful real PG ACK")
			}
			if name == "readonly_and_timeout" || name == "pool_reuse" {
				if !trace.readOnly || !trace.bounded || !slices.Equal(trace.seen, []string{"begin", "timeout", "check", "commit"}) {
					t.Error("scan lacks its readonly transaction, bounded child context or local timeout")
				}
				if name == "pool_reuse" {
					trace.seen = nil
					profileResult(t, CheckPasswordProfiles(t.Context(), pool, runtime), nil, nil)
					if !slices.Equal(trace.seen, []string{"begin", "timeout", "check", "commit"}) {
						t.Error("second call did not use a new complete transaction")
					}
				}
			}
			if name != "closed_pool" && profileSettings(t, pool) != baseline {
				t.Error("SET LOCAL leaked into a later single-pool checkout")
			}
			if profileSnapshot(t, owner) != before {
				t.Error("failed or successful profile scan changed immutable credential rows")
			}
		})
	}
}
