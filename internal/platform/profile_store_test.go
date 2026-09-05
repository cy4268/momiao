package platform

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func textPtr(s string) *string { return &s }
func profileCount(t *testing.T, s *Store, table string, user int64) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table+" WHERE newapi_user_id=$1", user).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func initializeProfile(t *testing.T, s *Store, user int64, name string) Profile {
	t.Helper()
	p, err := s.InitializeProfile(context.Background(), user, 0, name, "system-default")
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestProfileInvalidStoreInputs(t *testing.T) {
	s := &Store{}
	ctx := context.Background()
	if _, err := s.ReadProfile(ctx, 0); !errors.Is(err, ErrInvalidProfile) {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		user, version int64
		name, avatar  string
		want          error
	}{
		{0, 0, "Alice", "system-default", ErrInvalidProfile}, {1, 1, "Alice", "system-default", ErrInvalidProfile},
		{1, 0, "Admin", "system-default", ErrNicknameReserved}, {1, 0, "😀", "system-default", ErrInvalidNickname}, {1, 0, "Alice", "https://evil.invalid", ErrInvalidAvatar},
	} {
		if _, err := s.InitializeProfile(ctx, tc.user, tc.version, tc.name, tc.avatar); !errors.Is(err, tc.want) {
			t.Errorf("%+v: %v", tc, err)
		}
	}
	for _, tc := range []struct {
		patch ProfilePatch
		want  error
	}{
		{ProfilePatch{ExpectedVersion: 0, DisplayName: textPtr("Alice")}, ErrInvalidProfile},
		{ProfilePatch{ExpectedVersion: 1}, ErrInvalidProfile},
		{ProfilePatch{ExpectedVersion: 1, DisplayName: textPtr("")}, ErrInvalidNickname},
		{ProfilePatch{ExpectedVersion: 1, AvatarID: textPtr("")}, ErrInvalidAvatar},
	} {
		if _, err := s.UpdateProfile(ctx, 1, tc.patch); !errors.Is(err, tc.want) {
			t.Errorf("%+v: %v", tc, err)
		}
	}
}

func TestProfileStoreIntegration(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	base := time.Now().UnixMicro()
	name := func(suffix string) string { return fmt.Sprintf("M%d%s", base, suffix) }
	t.Run("read_is_pure_and_initialize_is_atomic_idempotent", func(t *testing.T) {
		u := base
		p, err := s.ReadProfile(ctx, u)
		if err != nil || p.Status != "INCOMPLETE" || p.UserID != u || p.ProfileVersion != 0 || p.DisplayName != "" || p.NicknameChangedAt != nil || p.NextRenameAt != nil || p.AvatarID != "system-default" || p.SuggestedName != "Master-"+ShortAccountID(u) {
			t.Fatalf("incomplete: %+v %v", p, err)
		}
		for _, table := range []string{"identity.account_refs", "identity.master_profiles", "identity.master_profile_name_history", "economy.wallet_balances"} {
			if n := profileCount(t, s, table, u); n != 0 {
				t.Fatal("read wrote", table, n)
			}
		}
		p = initializeProfile(t, s, u, name("a"))
		if p.ProfileVersion != 1 || p.Status != "COMPLETE" || p.NicknameChangedAt != nil || p.NextRenameAt != nil || len(p.Avatars) != 1 || p.Avatars[0].Source != "SYSTEM" {
			t.Fatal(p)
		}
		for _, table := range []string{"identity.account_refs", "identity.master_profiles", "identity.master_profile_name_history"} {
			if n := profileCount(t, s, table, u); n != 1 {
				t.Fatal(table, n)
			}
		}
		if n := profileCount(t, s, "economy.wallet_balances", u); n != 0 {
			t.Fatal("profile initialized wallets", n)
		}
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Go(func() {
				got, e := s.InitializeProfile(ctx, u, 0, name("a"), "system-default")
				if e != nil || !reflect.DeepEqual(got, p) {
					t.Errorf("repeat %+v %v", got, e)
				}
			})
		}
		wg.Wait()
		if n := profileCount(t, s, "identity.master_profile_name_history", u); n != 1 {
			t.Fatal("replay appended", n)
		}
		if _, err := s.InitializeProfile(ctx, u, 0, name("b"), "system-default"); !errors.Is(err, ErrStaleProfileVersion) {
			t.Fatal(err)
		}
	})
	t.Run("rename_cooldown_noop_stale_ownership_and_wallet_isolation", func(t *testing.T) {
		u := base + 1
		other := base + 2
		mustEnsure(t, s, u)
		before, err := s.ReadWallets(ctx, u)
		if err != nil {
			t.Fatal(err)
		}
		p := initializeProfile(t, s, u, name("c"))
		otherP := initializeProfile(t, s, other, name("d"))
		got, err := s.UpdateProfile(ctx, u, ProfilePatch{1, nil, textPtr("system-default")})
		if err != nil || !reflect.DeepEqual(got, p) {
			t.Fatal("avatar-only no-op", got, err)
		}
		renamed, err := s.UpdateProfile(ctx, u, ProfilePatch{1, textPtr(name("C")), nil})
		if err != nil || renamed.DisplayName != name("C") || renamed.ProfileVersion != 2 || renamed.NicknameChangedAt == nil || renamed.NextRenameAt == nil || renamed.NextRenameAt.Sub(*renamed.NicknameChangedAt) != 7*24*time.Hour || renamed.NicknameChangedAt.Location() != time.UTC {
			t.Fatal("case-only first rename", renamed, err)
		}
		if _, err = s.UpdateProfile(ctx, u, ProfilePatch{1, textPtr(name("C")), nil}); !errors.Is(err, ErrStaleProfileVersion) {
			t.Fatal("stale no-op", err)
		}
		got, err = s.UpdateProfile(ctx, u, ProfilePatch{2, textPtr(name("C")), textPtr("system-default")})
		if err != nil || !reflect.DeepEqual(got, renamed) {
			t.Fatal("cooldown no-op", got, err)
		}
		if _, err = s.UpdateProfile(ctx, u, ProfilePatch{2, textPtr(name("e")), nil}); !errors.Is(err, ErrRenameCooldown) {
			t.Fatal("cooldown", err)
		}
		if _, err = s.InitializeProfile(ctx, u, 0, name("C"), "system-default"); !errors.Is(err, ErrStaleProfileVersion) {
			t.Fatal("renamed init replay", err)
		}
		if n := profileCount(t, s, "identity.master_profile_name_history", u); n != 2 {
			t.Fatal(n)
		}
		if got, e := s.ReadProfile(ctx, other); e != nil || !reflect.DeepEqual(got, otherP) {
			t.Fatal("other user changed", got, e)
		}
		after, e := s.ReadWallets(ctx, u)
		if e != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("wallet changed", before, after, e)
		}
		if _, err = s.pool.Exec(ctx, "UPDATE identity.master_profiles SET nickname_changed_at=clock_timestamp()-interval '7 days' WHERE newapi_user_id=$1", u); err != nil {
			t.Fatal(err)
		}
		got, err = s.UpdateProfile(ctx, u, ProfilePatch{2, textPtr(name("e")), nil})
		if err != nil || got.ProfileVersion != 3 {
			t.Fatal("cooldown expired", got, err)
		}
	})
	t.Run("database_unique_concurrent_casefold_and_rollback", func(t *testing.T) {
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			i := i
			wg.Go(func() {
				n := name("unique")
				if i == 1 {
					n = "m" + n[1:]
				}
				_, e := s.InitializeProfile(ctx, base+10+int64(i), 0, n, "system-default")
				results <- e
			})
		}
		wg.Wait()
		close(results)
		successes, conflicts := 0, 0
		for e := range results {
			if e == nil {
				successes++
			} else if errors.Is(e, ErrNicknameTaken) {
				conflicts++
			} else {
				t.Fatal(e)
			}
		}
		if successes != 1 || conflicts != 1 {
			t.Fatal(successes, conflicts)
		}
		anchors := profileCount(t, s, "identity.account_refs", base+10) + profileCount(t, s, "identity.account_refs", base+11)
		if anchors != 1 {
			t.Fatal("failed init leaked FK anchor", anchors)
		}
		u := base + 12
		p := initializeProfile(t, s, u, name("f"))
		if _, err := s.UpdateProfile(ctx, u, ProfilePatch{1, textPtr(name("unique")), nil}); !errors.Is(err, ErrNicknameTaken) {
			t.Fatal(err)
		}
		got, err := s.ReadProfile(ctx, u)
		if err != nil || !reflect.DeepEqual(got, p) || profileCount(t, s, "identity.master_profile_name_history", u) != 1 {
			t.Fatal("conflict mutated", got, err)
		}
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer rollback(tx)
		if _, err = tx.Exec(ctx, "INSERT INTO identity.account_refs(newapi_user_id) VALUES($1)", base+13); err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, "INSERT INTO identity.master_profiles(newapi_user_id,display_name,normalized_name) VALUES($1,'Bypass',$2)", base+13, strings.ToLower(name("unique")))
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != "master_profiles_normalized_name_key" {
			t.Fatal("DB normalized-name uniqueness absent", err)
		}
	})
	t.Run("statement_failure_and_cancel_preserve_all_rows", func(t *testing.T) {
		existing := initializeProfile(t, s, base+21, name("j"))
		_, err := s.pool.Exec(ctx, `CREATE FUNCTION identity.test_profile_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected'; END $$; CREATE TRIGGER test_profile_fail BEFORE INSERT ON identity.master_profile_name_history FOR EACH STATEMENT EXECUTE FUNCTION identity.test_profile_fail()`)
		if err != nil {
			t.Fatal(err)
		}
		defer s.pool.Exec(ctx, "DROP TRIGGER IF EXISTS test_profile_fail ON identity.master_profile_name_history; DROP FUNCTION IF EXISTS identity.test_profile_fail()")
		u := base + 20
		if _, err = s.InitializeProfile(ctx, u, 0, name("g"), "system-default"); err == nil {
			t.Fatal("expected injected failure")
		}
		for _, table := range []string{"identity.account_refs", "identity.master_profiles", "identity.master_profile_name_history"} {
			if profileCount(t, s, table, u) != 0 {
				t.Fatal("partial init", table)
			}
		}
		if _, err = s.UpdateProfile(ctx, base+21, ProfilePatch{1, textPtr(name("k")), nil}); err == nil {
			t.Fatal("expected rename history failure")
		}
		if got, e := s.ReadProfile(ctx, base+21); e != nil || !reflect.DeepEqual(got, existing) || profileCount(t, s, "identity.master_profile_name_history", base+21) != 1 {
			t.Fatal("partial rename", got, e)
		}
		if _, err = s.pool.Exec(ctx, "DROP TRIGGER test_profile_fail ON identity.master_profile_name_history; DROP FUNCTION identity.test_profile_fail()"); err != nil {
			t.Fatal(err)
		}
		p := initializeProfile(t, s, u, name("g"))
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer rollback(tx)
		if _, err = tx.Exec(ctx, "SELECT 1 FROM identity.master_profiles WHERE newapi_user_id=$1 FOR UPDATE", u); err != nil {
			t.Fatal(err)
		}
		c, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		if _, err = s.UpdateProfile(c, u, ProfilePatch{1, textPtr(name("h")), nil}); err == nil {
			t.Fatal("cancel accepted")
		}
		if err = tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := s.ReadProfile(ctx, u)
		if err != nil || !reflect.DeepEqual(got, p) {
			t.Fatal("cancel changed profile", got, err)
		}
	})
	t.Run("append_only_history_with_negative_control", func(t *testing.T) {
		u := base + 30
		initializeProfile(t, s, u, name("i"))
		for _, op := range []string{"UPDATE identity.master_profile_name_history SET display_name=display_name", "DELETE FROM identity.master_profile_name_history", "TRUNCATE identity.master_profile_name_history"} {
			for _, protected := range []bool{true, false} {
				tx, err := s.pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if !protected {
					if _, err = tx.Exec(ctx, "DROP TRIGGER master_profile_name_history_immutable ON identity.master_profile_name_history"); err != nil {
						rollback(tx)
						t.Fatal(err)
					}
				}
				_, err = tx.Exec(ctx, op)
				rollback(tx)
				if protected {
					var pgErr *pgconn.PgError
					if !errors.As(err, &pgErr) || pgErr.Code != "55000" {
						t.Fatalf("immutable %s: %v", op, err)
					}
				} else if err != nil {
					t.Fatal("negative control", op, err)
				}
			}
		}
	})
}
