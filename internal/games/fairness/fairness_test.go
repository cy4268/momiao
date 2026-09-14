package fairness

import (
	"bytes"
	"encoding/hex"
	"io"
	"math"
	"strings"
	"testing"
)

func testSeed() []byte {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}
	return seed
}

func vectorRound() Round {
	return Round{
		ID:   [16]byte{0x01, 0x8f, 0x47, 0xa2, 0x6e, 0x9d, 0x7c, 0x31, 0x8a, 0x4b, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc},
		Game: "dice", ClientSeed: "client-seed-demo", Nonce: 42,
		StreamVersion: StreamVersion, AlgorithmVersion: "dice-map-v1",
		ConfigVersion: "test-config-v1", ConfigHash: [32]byte{1},
		ServerSeedHash: [32]byte{0x63, 0x0d, 0xcd, 0x29, 0x66, 0xc4, 0x33, 0x66, 0x91, 0x12, 0x54, 0x48, 0xbb, 0xb2, 0x5b, 0x4f, 0xf4, 0x12, 0xa4, 0x9c, 0x73, 0x2d, 0xb2, 0xc8, 0xab, 0xc1, 0xb8, 0x58, 0x1b, 0xd7, 0x10, 0xdd},
	}
}

func TestPublishedFairnessVector(t *testing.T) {
	round := vectorRound()
	commitment, err := SeedCommitment(testSeed())
	if err != nil || commitment != round.ServerSeedHash {
		t.Fatalf("commitment mismatch: %x %v", commitment, err)
	}
	message := canonicalStreamMessage(round, "dice:d1", 0)
	const wantMessage = "4348414c4445412d50462d484d41432d5348413235362d563100000464696365018f47a26e9d7c318a4b123456789abc000000000000002a0010636c69656e742d736565642d64656d6f000b646963652d6d61702d76310007646963653a64310000000000000000"
	if hex.EncodeToString(message) != wantMessage {
		t.Fatalf("canonical message mismatch: %x", message)
	}
	stream, err := NewStream(testSeed(), round, "dice:d1")
	if err != nil {
		t.Fatal(err)
	}
	block := make([]byte, 32)
	if _, err := io.ReadFull(stream, block); err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(block); got != "cd64aee5909adf6dad4968da9629db9fb8674b713f1076b484704ba70cff75ba" {
		t.Fatalf("HMAC block mismatch: %s", got)
	}
	for i, domain := range []string{"dice:d1", "dice:d2", "dice:d3"} {
		s, err := NewStream(testSeed(), round, domain)
		if err != nil {
			t.Fatal(err)
		}
		n, err := UniformInt(s, 6)
		if err != nil || n+1 != []uint64{4, 4, 1}[i] {
			t.Fatalf("%s: %d %v", domain, n+1, err)
		}
	}
}

func TestUniformIntRejectionAndLimits(t *testing.T) {
	// For N=6, 0xfffffffc..0xffffffff must be rejected, not reduced modulo 6.
	source := bytes.NewReader([]byte{255, 255, 255, 252, 255, 255, 255, 255, 0, 0, 0, 5})
	got, err := UniformInt(source, 6)
	if err != nil || got != 5 || source.Len() != 0 {
		t.Fatalf("rejection not honored: %d, remaining=%d, %v", got, source.Len(), err)
	}
	for _, tc := range []struct{ n, want uint64 }{{1, 0}, {2, 1}, {1 << 32, math.MaxUint32}} {
		got, err := UniformInt(bytes.NewReader([]byte{255, 255, 255, 255}), tc.n)
		if err != nil || got != tc.want {
			t.Fatalf("N=%d: %d %v", tc.n, got, err)
		}
	}
	for _, n := range []uint64{0, 1<<32 + 1, math.MaxUint64} {
		if _, err := UniformInt(bytes.NewReader(nil), n); err == nil {
			t.Fatalf("accepted invalid N=%d", n)
		}
	}
	if _, err := UniformInt(bytes.NewReader([]byte{1, 2, 3}), 6); err == nil {
		t.Fatal("accepted incomplete random word")
	}
}

func TestUniformIntExhaustiveAcceptedTail(t *testing.T) {
	// The final 600 accepted U32 words contain exactly 100 of each residue.
	var counts [6]int
	for x := uint64(4294966692); x < 4294967292; x++ {
		b := []byte{byte(x >> 24), byte(x >> 16), byte(x >> 8), byte(x)}
		n, err := UniformInt(bytes.NewReader(b), 6)
		if err != nil || n >= 6 {
			t.Fatal(n, err)
		}
		counts[n]++
	}
	if counts != [6]int{100, 100, 100, 100, 100, 100} {
		t.Fatal(counts)
	}
}

func TestFairnessRejectsInvalidInputs(t *testing.T) {
	for _, n := range []int{0, 31, 33} {
		if _, err := SeedCommitment(make([]byte, n)); err == nil {
			t.Fatalf("accepted %d-byte seed", n)
		}
		if _, err := NewStream(make([]byte, n), vectorRound(), "dice:d1"); err == nil {
			t.Fatalf("stream accepted %d-byte seed", n)
		}
	}
	bad := map[string]func(*Round){
		"commitment mismatch":  func(r *Round) { r.ServerSeedHash[0]++ },
		"empty client seed":    func(r *Round) { r.ClientSeed = "" },
		"long UTF8 seed":       func(r *Round) { r.ClientSeed = strings.Repeat("界", 43) },
		"invalid UTF8":         func(r *Round) { r.ClientSeed = "\xff" },
		"control":              func(r *Round) { r.ClientSeed = "a\u0085b" },
		"NUL":                  func(r *Round) { r.ClientSeed = "a\x00b" },
		"CR":                   func(r *Round) { r.ClientSeed = "a\rb" },
		"LF":                   func(r *Round) { r.ClientSeed = "a\nb" },
		"negative nonce":       func(r *Round) { r.Nonce = -1 },
		"empty UUID":           func(r *Round) { r.ID = [16]byte{} },
		"UUID version":         func(r *Round) { r.ID[6] = 0x4c },
		"UUID variant":         func(r *Round) { r.ID[8] = 0x4a },
		"empty game":           func(r *Round) { r.Game = "" },
		"empty algorithm":      func(r *Round) { r.AlgorithmVersion = "" },
		"unsupported stream":   func(r *Round) { r.StreamVersion = "unknown" },
		"empty config version": func(r *Round) { r.ConfigVersion = "" },
		"empty config hash":    func(r *Round) { r.ConfigHash = [32]byte{} },
	}
	for name, mutate := range bad {
		t.Run(name, func(t *testing.T) {
			r := vectorRound()
			mutate(&r)
			if _, err := NewStream(testSeed(), r, "dice:d1"); err == nil {
				t.Fatal("accepted invalid round")
			}
		})
	}
	for _, domain := range []string{"", "a\x00b", strings.Repeat("x", 65536)} {
		if _, err := NewStream(testSeed(), vectorRound(), domain); err == nil {
			t.Fatal("accepted invalid domain")
		}
	}
	r := vectorRound()
	r.ClientSeed = strings.Repeat("x", 128)
	r.Nonce = math.MaxInt64
	if _, err := NewStream(testSeed(), r, "dice:d1"); err != nil {
		t.Fatalf("rejected allowed boundaries: %v", err)
	}
}

func readStream(t *testing.T, r Round, domain string) []byte {
	t.Helper()
	s, err := NewStream(testSeed(), r, domain)
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 96)
	if _, err := io.ReadFull(s, b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestStreamReplayAndInputSeparation(t *testing.T) {
	r := vectorRound()
	want := readStream(t, r, "dice:d1")
	s, _ := NewStream(testSeed(), r, "dice:d1")
	var got []byte
	for _, size := range []int{1, 31, 0, 17, 47} {
		b := make([]byte, size)
		if _, err := io.ReadFull(s, b); err != nil {
			t.Fatal(err)
		}
		got = append(got, b...)
	}
	if !bytes.Equal(got, want) || bytes.Equal(want[:32], want[32:64]) {
		t.Fatal("stream does not replay across block/read boundaries")
	}
	for _, mutate := range []func(*Round){
		func(r *Round) { r.Game = "scratch" },
		func(r *Round) { r.Nonce++ },
		func(r *Round) { r.ID[15]++ },
		func(r *Round) { r.ClientSeed += ":" },
		func(r *Round) { r.AlgorithmVersion += "2" },
	} {
		changed := r
		mutate(&changed)
		if bytes.Equal(readStream(t, changed, "dice:d1"), want) {
			t.Fatal("input not bound to stream")
		}
	}
	if bytes.Equal(readStream(t, r, "dice:d2"), want) {
		t.Fatal("domain not separated")
	}
	// Configuration is deliberately bound by the mapper, outside the IS §263 stream.
	changed := r
	changed.ConfigVersion = "other-config"
	changed.ConfigHash[0]++
	if !bytes.Equal(readStream(t, changed, "dice:d1"), want) {
		t.Fatal("configuration altered frozen stream encoding")
	}
	a, b := r, r
	a.ClientSeed, a.AlgorithmVersion = "a:b", "c"
	b.ClientSeed, b.AlgorithmVersion = "a", "b:c"
	if bytes.Equal(readStream(t, a, "dice:d1"), readStream(t, b, "dice:d1")) {
		t.Fatal("ambiguous delimiter encoding")
	}
	a.ClientSeed, b.ClientSeed = "é", "e\u0301"
	if bytes.Equal(readStream(t, a, "dice:d1"), readStream(t, b, "dice:d1")) {
		t.Fatal("client seed was Unicode-normalized")
	}
}

func TestStreamCounterDoesNotWrap(t *testing.T) {
	s, _ := NewStream(testSeed(), vectorRound(), "dice:d1")
	s.blockIndex = math.MaxUint64
	if _, err := io.ReadFull(s, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(make([]byte, 1)); err == nil {
		t.Fatal("block counter wrapped and reused stream")
	}
}
