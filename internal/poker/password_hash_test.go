package poker

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"
)

// TEST ONLY: fast functional encoding checks, not production cost/strength evidence.
var passwordTestConfig = PasswordConfig{MemoryKiB: 64, Time: 1, Parallelism: 1}

func TestTablePasswordConcurrentRoundTrip(t *testing.T) {
	const workers = 8
	const password = " test-only-password\x00é "
	type result struct {
		phc string
		ok  bool
		err error
	}
	results := make(chan result, workers)
	start := time.Now()
	for i := 0; i < workers; i++ {
		go func() {
			phc, err := HashTablePassword(passwordTestConfig, password)
			var ok bool
			if err == nil {
				ok, err = VerifyTablePassword(passwordTestConfig, phc, password)
			}
			results <- result{phc, ok, err}
		}()
	}
	seen := map[string]bool{}
	for i := 0; i < workers; i++ {
		r := <-results
		if r.err != nil || !r.ok || seen[r.phc] {
			t.Fatal("parallel hash/verify must succeed with fresh independent salts")
		}
		seen[r.phc] = true
		parts := strings.Split(r.phc, "$")
		if len(parts) != 6 || strings.Join(parts[:4], "$") != "$argon2id$v=19$m=64,t=1,p=1" {
			t.Fatal("canonical Argon2id PHC header required")
		}
		salt, se := base64.RawStdEncoding.Strict().DecodeString(parts[4])
		key, ke := base64.RawStdEncoding.Strict().DecodeString(parts[5])
		if se != nil || ke != nil || len(salt) != 16 || len(key) != 32 {
			t.Fatal("fixed 16-byte salt and 32-byte key required")
		}
		// Independent upstream API checks our algorithm/argument wiring, not itself.
		if !bytes.Equal(key, argon2.IDKey([]byte(password), salt, 1, 64, 1, 32)) {
			t.Fatal("encoded key differs from explicit Argon2id derivation")
		}
		if ok, err := VerifyTablePassword(passwordTestConfig, r.phc, strings.TrimSpace(password)); err != nil || ok {
			t.Fatal("password bytes must not be normalized")
		}
	}
	t.Logf("TEST_ONLY m=64KiB/t=1/p=1 parallel=%d distinct-salt roundtrips verified; elapsed=%s; not a production benchmark", workers, time.Since(start))
}

func TestTablePasswordStrictPHC(t *testing.T) {
	const password = "test-only-phc-input"
	phc, err := HashTablePassword(passwordTestConfig, password)
	if err != nil {
		t.Fatal("valid PHC seed must be generated")
	}
	p := strings.Split(phc, "$")
	prefix := "$argon2id$v=19$m=64,t=1,p=1$"
	encoded := func(salt, key string) string { return prefix + salt + "$" + key }
	bad := []string{
		"", strings.Repeat("x", 513), phc + "$", " " + phc,
		strings.Replace(phc, "argon2id", "argon2i", 1), strings.Replace(phc, "v=19", "v=16", 1),
		strings.Replace(phc, "v=19", "v=019", 1), strings.Replace(phc, "m=64", "m=064", 1),
		strings.Replace(phc, "m=64", "m=+64", 1), strings.Replace(phc, "m=64", "m=-1", 1),
		strings.Replace(phc, "m=64", "m=4294967296", 1), strings.Replace(phc, "t=1", "t=0", 1),
		strings.Replace(phc, "p=1", "p=256", 1), strings.Replace(phc, "t=1,p=1", "p=1,t=1", 1),
		strings.Replace(phc, "p=1", "p=1,p=1", 1), strings.Replace(phc, "m=64", "m=128", 1),
		encoded(p[4]+"=", p[5]), encoded(p[4], p[5]+"="), encoded(p[4][:21], p[5]),
		encoded(p[4], p[5][:42]), encoded(p[4]+"\n", p[5]), encoded(p[4], p[5]+"\r\n"),
		encoded("-"+p[4][1:], p[5]), encoded(p[4][:21]+"B", p[5]),
	}
	for i, input := range bad {
		if ok, err := VerifyTablePassword(passwordTestConfig, input, password); ok || !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid PHC case %d must fail before derivation", i)
		}
	}
	if ok, err := VerifyTablePassword(PasswordConfig{128, 1, 1}, phc, password); ok || !errors.Is(err, ErrInvalid) {
		t.Fatal("PHC must match the explicit configured cost profile")
	}
	if ok, err := VerifyTablePassword(passwordTestConfig, phc, password+"x"); err != nil || ok {
		t.Fatal("wrong valid password is a mismatch, not a parser error")
	}
}

func TestTablePasswordInputAndConfigBounds(t *testing.T) {
	const password = "test-only-bounds-input"
	phc, err := HashTablePassword(passwordTestConfig, password)
	if err != nil {
		t.Fatal("valid PHC seed must be generated")
	}
	for _, input := range []string{"", strings.Repeat("é", 64) + "x", "\xff"} {
		if out, err := HashTablePassword(passwordTestConfig, input); out != "" || !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid password must not produce a PHC or leak its bytes")
		}
		if ok, err := VerifyTablePassword(passwordTestConfig, phc, input); ok || !errors.Is(err, ErrInvalid) {
			t.Fatal("verify must enforce the same password byte bounds")
		}
	}
	for _, input := range []string{"x", strings.Repeat("é", 64)} {
		out, e := HashTablePassword(passwordTestConfig, input)
		ok, v := VerifyTablePassword(passwordTestConfig, out, input)
		if e != nil || v != nil || !ok {
			t.Fatal("inclusive 1 and 128 UTF-8 byte boundaries must work")
		}
	}
	for _, cfg := range []PasswordConfig{{}, {0, 1, 1}, {64, 0, 1}, {64, 1, 0}, {7, 1, 1}, {8, 1, 2}, {262145, 1, 1}, {64, 11, 1}, {128, 1, 17}, {^uint32(0), 1, 1}, {64, ^uint32(0), 1}, {64, 1, 255}} {
		if out, err := HashTablePassword(cfg, password); out != "" || !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid config must be rejected before allocating KDF memory")
		}
		if ok, err := VerifyTablePassword(cfg, phc, password); ok || !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid config must fail before parsing/derivation")
		}
	}
}
