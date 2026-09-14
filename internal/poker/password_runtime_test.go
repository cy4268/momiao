package poker

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func runtimePolicy() PasswordPolicy {
	return PasswordPolicy{Current: PasswordConfig{64, 1, 1}, MaxConcurrent: 1, MaxWorkMemoryKiB: 128}
}

func runtimeForTest(t *testing.T, p PasswordPolicy) *PasswordRuntime {
	t.Helper()
	r, err := NewPasswordRuntime(p)
	if err != nil || r == nil || ValidatePasswordPolicy(p) != nil {
		t.Fatal("valid explicit TEST-only policy rejected")
	}
	return r
}

func TestPasswordRuntimePolicy(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*PasswordPolicy)
	}{
		{"memory_zero", func(p *PasswordPolicy) { p.Current.MemoryKiB = 0 }},
		{"memory_high", func(p *PasswordPolicy) { p.Current.MemoryKiB = 262145 }},
		{"time_zero", func(p *PasswordPolicy) { p.Current.Time = 0 }},
		{"time_high", func(p *PasswordPolicy) { p.Current.Time = 11 }},
		{"lanes_zero", func(p *PasswordPolicy) { p.Current.Parallelism = 0 }},
		{"lanes_high", func(p *PasswordPolicy) { p.Current.Parallelism = 17 }},
		{"lanes_memory", func(p *PasswordPolicy) { p.Current.Parallelism = 16 }},
		{"concurrent_zero", func(p *PasswordPolicy) { p.MaxConcurrent = 0 }},
		{"concurrent_high", func(p *PasswordPolicy) { p.MaxConcurrent = 17 }},
		{"budget_zero", func(p *PasswordPolicy) { p.MaxWorkMemoryKiB = 0 }},
		{"budget_high", func(p *PasswordPolicy) { p.MaxWorkMemoryKiB = 262145 }},
		{"historical_memory", func(p *PasswordPolicy) { p.VerifyProfiles = []PasswordConfig{{256, 1, 1}} }},
		{"historical_invalid", func(p *PasswordPolicy) { p.VerifyProfiles = []PasswordConfig{{128, 0, 1}} }},
		{"five_inputs", func(p *PasswordPolicy) {
			p.VerifyProfiles = []PasswordConfig{p.Current, p.Current, p.Current, p.Current, p.Current}
		}},
		{"five_distinct", func(p *PasswordPolicy) {
			p.VerifyProfiles = []PasswordConfig{{8, 1, 1}, {16, 1, 1}, {32, 1, 1}, {128, 1, 1}}
		}},
		{"product_over_budget", func(p *PasswordPolicy) { p.MaxConcurrent = 3 }},
		{"large_product", func(p *PasswordPolicy) { p.Current.MemoryKiB, p.MaxConcurrent = ^uint32(0), ^uint8(0) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := runtimePolicy()
			tc.edit(&p)
			r, err := NewPasswordRuntime(p)
			if r != nil || !errors.Is(err, ErrPasswordConfig) || !errors.Is(ValidatePasswordPolicy(p), ErrPasswordConfig) {
				t.Fatal("invalid policy must fail before runtime construction")
			}
		})
	}
	p := runtimePolicy()
	p.VerifyProfiles = []PasswordConfig{{8, 1, 1}, {16, 1, 1}, {32, 1, 1}}
	runtimeForTest(t, p)
	p.VerifyProfiles = []PasswordConfig{p.Current, {128, 1, 1}, p.Current, {128, 1, 1}}
	runtimeForTest(t, p)
	p.Current, p.VerifyProfiles, p.MaxConcurrent, p.MaxWorkMemoryKiB = PasswordConfig{32768, 1, 1}, nil, 8, 262144
	runtimeForTest(t, p)
}

func TestPasswordRuntimeRealKDF(t *testing.T) {
	ctx, password := context.Background(), " \x00御主 "
	p := runtimePolicy()
	history := []PasswordConfig{{128, 1, 1}}
	p.VerifyProfiles = history
	r := runtimeForTest(t, p)
	history[0], p.Current = PasswordConfig{256, 1, 1}, PasswordConfig{256, 1, 1}
	phc, err := r.Hash(ctx, password)
	match, verifyErr := VerifyTablePassword(PasswordConfig{64, 1, 1}, phc, password)
	if err != nil || verifyErr != nil || !match || !strings.HasPrefix(phc, "$argon2id$v=19$m=64,t=1,p=1$") {
		t.Fatal("runtime Hash must use the copied Current profile and real PHC")
	}
	for _, cfg := range []PasswordConfig{{64, 1, 1}, {128, 1, 1}} {
		stored, e := HashTablePassword(cfg, password)
		if e != nil {
			t.Fatal("P1 true-KDF fixture failed")
		}
		ok, e := r.Verify(ctx, stored, password)
		wrong, wrongErr := r.Verify(ctx, stored, "wrong")
		if e != nil || !ok || wrongErr != nil || wrong {
			t.Fatal("copied allowed profiles must verify original PHC; wrong password alone is false,nil")
		}
	}
	unknown, err := HashTablePassword(PasswordConfig{256, 1, 1}, password)
	if err != nil {
		t.Fatal("unknown-profile fixture failed")
	}
	for _, bad := range []string{unknown, strings.Repeat("x", 513), phc + "\n", phc[:len(phc)-1] + "!"} {
		if ok, e := r.Verify(ctx, bad, password); ok || !errors.Is(e, ErrPasswordConfig) {
			t.Fatal("unknown or malformed PHC must be a credential/config error, not a password mismatch")
		}
	}
	for _, bad := range []string{"", strings.Repeat("a", 129), strings.Repeat("a", 127) + "é", string([]byte{255})} {
		out, e := r.Hash(ctx, bad)
		ok, ve := r.Verify(ctx, phc, bad)
		if out != "" || !errors.Is(e, ErrInvalid) || ok || !errors.Is(ve, ErrInvalid) {
			t.Fatal("raw UTF-8 password byte bounds must apply to both operations")
		}
	}
	for _, good := range []string{"x", strings.Repeat("é", 64)} {
		out, e := r.Hash(ctx, good)
		ok, ve := r.Verify(ctx, out, good)
		if e != nil || ve != nil || !ok {
			t.Fatal("one-byte and 128-byte passwords must roundtrip")
		}
	}
	t.Log("REAL_KDF TEST-only current=64KiB/1/1, retained=128KiB/1/1; not production capacity")
}

func TestPasswordRuntimeSlotScheduling(t *testing.T) {
	r := runtimeForTest(t, runtimePolicy())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	go func() {
		done <- r.withSlot(ctx, func() error { close(entered); <-release; return nil })
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first synchronous work was not dispatched")
	}
	busy := func() {
		t.Helper()
		result := make(chan error, 1)
		go func() {
			result <- r.withSlot(context.Background(), func() error { return errors.New("unexpected second dispatch") })
		}()
		select {
		case err := <-result:
			if !errors.Is(err, ErrPasswordBusy) {
				t.Fatal("full slot must reject immediately without dispatch")
			}
		case <-time.After(time.Second):
			t.Fatal("full slot queued work")
		}
	}
	busy()
	cancel()
	busy()
	unblock()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrPasswordUnavailable) {
			t.Fatal("cancelled work must retain its slot until actual completion and discard success")
		}
	case <-time.After(time.Second):
		t.Fatal("completed synchronous work did not return")
	}
	if _, err := r.Hash(context.Background(), "after-release"); err != nil {
		t.Fatal("completed work must release the slot")
	}
	t.Log("SYNTHETIC_SLOT barrier proves scheduling only, not KDF timing or production capacity")
}

type runtimeStepContext struct {
	context.Context
	cancel    context.CancelFunc
	at, calls int
	err       error
}

func (c *runtimeStepContext) Err() error {
	if c.err != nil {
		return c.err
	}
	c.calls++
	if c.calls == c.at {
		c.cancel()
	}
	return c.Context.Err()
}

func TestPasswordRuntimeCancellation(t *testing.T) {
	r := runtimeForTest(t, runtimePolicy())
	for _, empty := range []*PasswordRuntime{nil, {}} {
		out, err := empty.Hash(context.Background(), "x")
		ok, ve := empty.Verify(context.Background(), "phc", "x")
		if out != "" || ok || !errors.Is(err, ErrPasswordConfig) || !errors.Is(ve, ErrPasswordConfig) {
			t.Fatal("nil and zero runtimes must fail with controlled configuration errors")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		ctx  context.Context
		want error
	}{
		{nil, ErrPasswordUnavailable}, {ctx, context.Canceled},
		{&runtimeStepContext{Context: context.Background(), err: errors.Join(context.DeadlineExceeded, errors.New("provider-marker"))}, context.DeadlineExceeded},
	} {
		called := false
		err := r.withSlot(tc.ctx, func() error { called = true; return nil })
		if called || !errors.Is(err, ErrPasswordUnavailable) || !errors.Is(err, tc.want) || strings.Contains(err.Error(), "provider-marker") {
			t.Fatal("nil or failed context must not dispatch or expose provider text")
		}
	}
	ctx, cancel = context.WithCancel(context.Background())
	t.Cleanup(cancel)
	step, called := &runtimeStepContext{Context: ctx, cancel: cancel, at: 2}, false
	if err := r.withSlot(step, func() error { called = true; return nil }); called || !errors.Is(err, context.Canceled) {
		t.Fatal("context cancelled after slot acquisition must not dispatch")
	}
	stored, err := HashTablePassword(PasswordConfig{64, 1, 1}, "password")
	if err != nil {
		t.Fatal("P1 KDF fixture failed")
	}
	for _, verify := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		step := &runtimeStepContext{Context: ctx, cancel: cancel, at: 3}
		if verify {
			ok, e := r.Verify(step, stored, "password")
			if ok || !errors.Is(e, context.Canceled) {
				t.Fatal("Verify published result after cancellation")
			}
		} else {
			out, e := r.Hash(step, "password")
			if out != "" || !errors.Is(e, context.Canceled) {
				t.Fatal("Hash published result after cancellation")
			}
		}
		cancel()
	}
	t.Log("SYNTHETIC_CONTEXT cancels at dispatch/completion checks; Hash/Verify still execute REAL_KDF")
}

func TestPasswordRuntimeConcurrentKDF(t *testing.T) {
	p := runtimePolicy()
	p.MaxConcurrent, p.MaxWorkMemoryKiB = 4, 256
	r := runtimeForTest(t, p)
	results, start := make(chan bool, 4), time.Now()
	for range 4 {
		go func() {
			out, err := r.Hash(context.Background(), "concurrent")
			ok, verifyErr := r.Verify(context.Background(), out, "concurrent")
			results <- err == nil && verifyErr == nil && ok
		}()
	}
	for range 4 {
		if !<-results {
			t.Fatal("independent real-KDF roundtrip failed")
		}
	}
	t.Logf("REAL_KDF TEST-only 64KiB/1/1, configured C=4, 4 concurrent roundtrips, elapsed=%s; not a production benchmark", time.Since(start))
}
