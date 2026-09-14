package platform

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestV08OpsUsersShortAccountSearch(t *testing.T) {
	s, _, actor := v04OpsFixture(t)
	ctx := context.Background()
	for _, user := range []int64{2, 3} {
		if err := s.EnsureAccount(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.InitializeProfile(ctx, 2, 0, "V08 Search Fixture", "system-default"); err != nil {
		t.Fatal(err)
	}
	want, err := s.OpsUser(ctx, actor.UserID, actor.Epoch, 2)
	if err != nil || want.ShortAccountID != "CA-E213042DECE9" || want.ProfileStatus != "COMPLETE" {
		t.Fatalf("safe user detail=%+v error=%v", want, err)
	}
	for _, query := range []string{want.ShortAccountID, strings.ToLower(want.ShortAccountID), "2", "search fixture"} {
		users, err := s.OpsUsers(ctx, actor.UserID, actor.Epoch, query, 10)
		if err != nil || !reflect.DeepEqual(users, []OpsUser{want}) {
			t.Fatalf("query %q users=%+v error=%v", query, users, err)
		}
	}
	for _, query := range []string{"CA-000000000000", "missing synthetic account"} {
		users, err := s.OpsUsers(ctx, actor.UserID, actor.Epoch, query, 10)
		if err != nil || len(users) != 0 {
			t.Fatalf("nonmatching query %q users=%+v error=%v", query, users, err)
		}
	}
	if _, err := s.OpsUsers(ctx, 2, 1, want.ShortAccountID, 10); !errors.Is(err, ErrOpsForbidden) {
		t.Fatalf("ordinary user list access=%v", err)
	}
}
