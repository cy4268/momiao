package redislease

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// SDK boundary stub only; real Lua/TLS/ACL evidence is in access_integration_test.go.
func TestTableAccessBoundary(t *testing.T) {
	stub := &stubRedis{value: int64(1)}
	s := &Store{client: stub, timeout: time.Second}
	ctx := context.Background()
	binding := "v1:42:1:0:" + testToken // Credential tag fixture, not a password digest.
	until := time.Date(2026, 9, 7, 0, 0, 0, 123456789, time.UTC)
	for _, call := range []func(context.Context, string, string, string, time.Time) (bool, error){s.AccessPut, s.AccessValid} {
		bad := []struct {
			sid, table, binding string
			until               time.Time
		}{
			{"", testTable, binding, until}, {strings.ToUpper(testToken), testTable, binding, until},
			{testToken, strings.ToUpper(testTable), binding, until}, {testToken, testTable + ":x", binding, until},
			{testToken, testTable, binding, time.Time{}},
			{testToken, testTable, binding, time.Date(2000, 1, 1, 0, 0, 0, 0, time.FixedZone("east", 3600))},
		}
		for _, value := range []string{"", binding + ":x", "v1:042:1:0:" + testToken, "v1:0:1:0:" + testToken,
			"v1:9223372036854775808:1:0:" + testToken, "v1:42:0:0:" + testToken,
			"v1:42:9007199254740992:0:" + testToken, "v1:42:1:9007199254740992:" + testToken,
			"v1:42:1:00:" + testToken, "v1:42:1:0:" + strings.ToUpper(testToken)} {
			bad = append(bad, struct {
				sid, table, binding string
				until               time.Time
			}{testToken, testTable, value, until})
		}
		for _, in := range bad {
			before := stub.calls
			if ok, err := call(ctx, in.sid, in.table, in.binding, in.until); ok || !errors.Is(err, ErrInvalidInput) || stub.calls != before {
				t.Fatal("invalid input crossed Redis boundary", err)
			}
		}
		if ok, err := call(nil, testToken, testTable, binding, until); ok || !errors.Is(err, ErrInvalidInput) {
			t.Fatal("nil context accepted", err)
		}
		if ok, err := call(ctx, testToken, testTable, binding, until); err != nil || !ok || len(stub.keys) != 1 || stub.keys[0] != "chaldea:poker:table-access:"+testToken+":"+testTable || !stub.deadline || stub.args[0] != binding || stub.args[1] != until.UnixMilli() {
			t.Fatal("grant SDK arguments/deadline mismatch", err)
		}
	}
}

func TestAccessAttemptBoundary(t *testing.T) {
	stub := &stubRedis{value: int64(1)}
	s := &Store{client: stub, timeout: time.Second}
	ctx := context.Background()
	for _, in := range []struct {
		user                     int64
		table                    string
		ownerMax, tableMax       uint32
		ownerWindow, tableWindow time.Duration
	}{
		{0, testTable, 2, 1, time.Second, time.Second}, {-1, testTable, 2, 1, time.Second, time.Second},
		{42, "bad", 2, 1, time.Second, time.Second}, {42, testTable, 0, 1, time.Second, time.Second},
		{42, testTable, 2, 0, time.Second, time.Second}, {42, testTable, 2, 1, 0, time.Second},
		{42, testTable, 2, 1, time.Second, 0}, {42, testTable, 2, 1, -time.Second, time.Second},
		{42, testTable, 2, 1, time.Microsecond, time.Second}, {42, testTable, 2, 1, time.Second, time.Second + 1},
		{42, testTable, 2, 1, 24*time.Hour + time.Millisecond, time.Second},
	} {
		before := stub.calls
		if ok, err := s.AccessAttempt(ctx, in.user, in.table, in.ownerMax, in.ownerWindow, in.tableMax, in.tableWindow); ok || !errors.Is(err, ErrInvalidInput) || stub.calls != before {
			t.Fatal("bad budget input crossed Redis boundary", err)
		}
	}
	if ok, err := s.AccessAttempt(ctx, 42, "", 2, time.Second, 0, 0); err != nil || !ok || len(stub.keys) != 1 || stub.keys[0] != "chaldea:ratelimit:poker:table-password:owner:42" {
		t.Fatal("Create must use owner-only budget", err)
	}
	if ok, err := s.AccessAttempt(ctx, 42, testTable, 2, time.Second, 1, time.Second); err != nil || !ok || len(stub.keys) != 2 || stub.keys[1] != "chaldea:ratelimit:poker:table-password:owner-table:42:"+testTable || !stub.deadline {
		t.Fatal("dual budget SDK keys/deadline", err)
	}
	c, cancel := context.WithCancel(ctx)
	cancel()
	before := stub.calls
	if ok, err := s.AccessAttempt(c, 42, testTable, 2, time.Second, 1, time.Second); ok || !errors.Is(err, context.Canceled) || stub.calls != before {
		t.Fatal("cancel crossed Redis boundary", err)
	}
	for _, value := range []any{int64(2), "1", nil} {
		stub.value = value
		if ok, err := s.AccessAttempt(ctx, 42, testTable, 2, time.Second, 1, time.Second); ok || !errors.Is(err, ErrUnavailable) {
			t.Fatal("invalid Redis reply accepted", err)
		}
	}
	stub.err = errors.New("private " + testToken)
	if ok, err := s.AccessAttempt(ctx, 42, testTable, 2, time.Second, 1, time.Second); ok || !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), testToken) {
		t.Fatal("provider error leaked or opened budget")
	}
}
