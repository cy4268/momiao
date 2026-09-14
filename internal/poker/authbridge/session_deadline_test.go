package authbridge

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker/connectticket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type deadlineTrace struct{ afterQuery func() }

func (*deadlineTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return ctx
}
func (d *deadlineTrace) TraceQueryEnd(_ context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if d.afterQuery != nil {
		d.afterQuery()
	}
}

// Real local PG, raw 0001-0009 + 0018 only; synthetic native callback, not native HTTP.
func TestCheckUntilRealPGAbsoluteDeadline(t *testing.T) {
	owner, originalPool := bindingDB(t)
	ctx := context.Background()
	var db, role string
	if err := owner.QueryRow(ctx, "SELECT current_database(),current_user").Scan(&db, &role); err != nil {
		t.Fatal(err)
	}
	t.Logf("owned DB=%s owner=%s runtime=%s", db, role, originalPool.Config().ConnConfig.User)
	trace := &deadlineTrace{}
	cfg := originalPool.Config()
	cfg.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = owner.Exec(ctx, "INSERT INTO identity.account_refs(newapi_user_id) VALUES(42)"); err != nil {
		t.Fatal(err)
	}
	ref := NativeRef{"42", strings.Repeat("a", 64), 1}
	native := NativeSession{ref, 1, time.Now().Add(-time.Hour).Truncate(time.Second), time.Now().Add(13 * time.Minute).In(time.FixedZone("fixture", 3600))}
	saved := native
	var nativeErr error
	a, err := New(pool, func(context.Context, NativeRef) (NativeSession, error) { return native, nativeErr })
	if err != nil {
		t.Fatal(err)
	}
	s, err := a.Bind(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	assert := func(t *testing.T, c context.Context, want time.Time, wantErr error) {
		t.Helper()
		until, e := a.CheckUntil(c, s)
		if e != wantErr || !until.Equal(want) || e != nil && !until.IsZero() || e == nil && until.Location() != time.UTC {
			t.Fatalf("deadline=%s error=%v; want=%s error=%v", until, e, want, wantErr)
		}
		if e = a.Check(c, s); e != wantErr {
			t.Fatalf("Check compatibility: %v; want %v", e, wantErr)
		}
	}
	for _, shift := range []time.Duration{0, -5 * time.Minute, 5 * time.Minute} {
		native.ExpiresAt = saved.ExpiresAt.Add(shift)
		assert(t, ctx, native.ExpiresAt, nil)
	}
	for _, tc := range []struct {
		name   string
		change func()
		want   error
	}{
		{"native-revoked", func() { nativeErr = connectticket.ErrRevoked }, connectticket.ErrRevoked},
		{"provider-error-redacted", func() { nativeErr = errors.New("private provider detail") }, connectticket.ErrUnavailable},
		{"native-expired", func() { native.ExpiresAt = time.Now().Add(-time.Second) }, connectticket.ErrRevoked},
		{"native-version", func() { native.Ref.SessionVersion++ }, connectticket.ErrRevoked},
		{"auth-version-pg-mismatch", func() { native.UserAuthVersion++ }, connectticket.ErrRevoked},
		{"created-at-pg-mismatch", func() { native.CreatedAt = native.CreatedAt.Add(-time.Second) }, connectticket.ErrRevoked},
		{"tuple-invalid", func() { s.SessionIDHash = "invalid" }, connectticket.ErrInvalid},
		{"epoch-invalid", func() { s.SecurityEpoch = maxSafe + 1 }, connectticket.ErrInvalid},
		{"binding-epoch-mismatch", func() { s.SecurityEpoch++ }, connectticket.ErrRevoked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			native, nativeErr = saved, nil
			s.SessionIDHash, s.SecurityEpoch = ref.SessionIDHash, 0
			tc.change()
			assert(t, ctx, time.Time{}, tc.want)
		})
	}
	native, nativeErr, s.SecurityEpoch = saved, nil, 0
	assert(t, nil, time.Time{}, connectticket.ErrInvalid)
	savedAuthority := a
	a = nil
	assert(t, ctx, time.Time{}, connectticket.ErrInvalid)
	a = savedAuthority
	c, cancel := context.WithCancel(ctx)
	cancel()
	assert(t, c, time.Time{}, connectticket.ErrUnavailable)
	c, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer cancel()
	assert(t, c, time.Time{}, connectticket.ErrUnavailable)
	c, cancel = context.WithCancel(ctx)
	defer cancel()
	trace.afterQuery = cancel // Query succeeds, but cancellation before return must deny.
	assert(t, c, time.Time{}, connectticket.ErrUnavailable)
	native.ExpiresAt = time.Now().Add(100 * time.Millisecond)
	queried := false
	trace.afterQuery = func() { queried = true; time.Sleep(time.Until(native.ExpiresAt) + time.Millisecond) }
	assert(t, ctx, time.Time{}, connectticket.ErrRevoked)
	if !queried {
		t.Fatal("expiry must cross the completed real PG query, not expire before it")
	}
	trace.afterQuery, native = nil, saved
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, "LOCK TABLE identity.native_session_bindings IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	assert(t, ctx, time.Time{}, connectticket.ErrUnavailable)
	if elapsed := time.Since(start); elapsed < 3*time.Second || elapsed > 8*time.Second {
		t.Fatalf("two real PG child timeouts outside bound: %s", elapsed)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Exec(ctx, "UPDATE identity.account_refs SET security_epoch=1 WHERE newapi_user_id=42"); err != nil {
		t.Fatal(err)
	}
	assert(t, ctx, time.Time{}, connectticket.ErrRevoked)
	pool.Close()
	assert(t, ctx, time.Time{}, connectticket.ErrUnavailable)
}
