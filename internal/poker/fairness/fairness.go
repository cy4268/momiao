// Package fairness implements Poker's own IS-07 byte encoding, not Direct Play.
package fairness

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const StreamVersion = "chaldea-poker-hmac-sha256-v1"

// The original golden input omitted this label. poker-deck-v1 is independently
// reconstructed by matching block 0, the 15-card prefix and the full U8 deck hash.
const AlgorithmVersion = "poker-deck-v1"

type Contribution struct {
	Seat    int
	Version uint64
	Value   string
}
type Proof struct {
	EffectiveSeed, ServerSeedHash, DeckHash [32]byte
	Deck                                    []byte
}

func Derive(tableID, handID string, contributions []Contribution, seed []byte) (Proof, error) {
	var p Proof
	if len(seed) != 32 {
		return p, errors.New("Poker seed must be 32 bytes")
	}
	effective, err := Effective(tableID, handID, contributions)
	if err != nil {
		return p, err
	}
	s, err := NewStream(tableID, handID, effective, seed)
	if err != nil {
		return p, err
	}
	p.EffectiveSeed = effective
	p.ServerSeedHash = sha256.Sum256(seed)
	p.Deck = make([]byte, 52)
	for i := range p.Deck {
		p.Deck[i] = byte(i)
	}
	for i := 51; i > 0; i-- {
		j, err := Uniform(s, uint64(i+1))
		if err != nil {
			return Proof{}, err
		}
		p.Deck[i], p.Deck[j] = p.Deck[j], p.Deck[i]
	}
	p.DeckHash = sha256.Sum256(p.Deck)
	return p, nil
}

func ValidContribution(s string) bool {
	if len(s) < 1 || len(s) > 128 || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func Effective(tableID, handID string, contributions []Contribution) ([32]byte, error) {
	var zero [32]byte
	table, err := uuidBytes(tableID)
	if err != nil {
		return zero, err
	}
	hand, err := uuidBytes(handID)
	if err != nil {
		return zero, err
	}
	ordered := append([]Contribution{}, contributions...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Seat < ordered[j].Seat })
	if len(ordered) < 2 || len(ordered) > 9 {
		return zero, errors.New("Poker participant count")
	}
	b := bytes.NewBufferString("CHALDEA-POKER-EFFECTIVE-CLIENT-SEED-V1\x00")
	b.Write(table)
	b.Write(hand)
	_ = binary.Write(b, binary.BigEndian, uint16(len(ordered)))
	for i, c := range ordered {
		if c.Seat < 1 || c.Seat > 9 || (i > 0 && ordered[i-1].Seat == c.Seat) || c.Version == 0 || !ValidContribution(c.Value) {
			return zero, errors.New("Poker contribution")
		}
		_ = binary.Write(b, binary.BigEndian, uint16(c.Seat))
		_ = binary.Write(b, binary.BigEndian, c.Version)
		lp(b, c.Value)
	}
	return sha256.Sum256(b.Bytes()), nil
}

type Stream struct {
	seed, prefix, block []byte
	index               uint64
	offset              int
}

func NewStream(tableID, handID string, effective [32]byte, seed []byte) (*Stream, error) {
	if len(seed) != 32 {
		return nil, errors.New("Poker seed length")
	}
	table, err := uuidBytes(tableID)
	if err != nil {
		return nil, err
	}
	hand, err := uuidBytes(handID)
	if err != nil {
		return nil, err
	}
	b := bytes.NewBufferString("CHALDEA-POKER-DECK-HMAC-SHA256-V1\x00")
	b.Write(table)
	b.Write(hand)
	b.Write(effective[:])
	lp(b, AlgorithmVersion)
	return &Stream{seed: append([]byte{}, seed...), prefix: b.Bytes()}, nil
}
func (s *Stream) Read(out []byte) (int, error) {
	for i := range out {
		if s.offset == len(s.block) {
			h := hmac.New(sha256.New, s.seed)
			_, _ = h.Write(s.prefix)
			var index [8]byte
			binary.BigEndian.PutUint64(index[:], s.index)
			_, _ = h.Write(index[:])
			s.block = h.Sum(nil)
			s.index++
			s.offset = 0
		}
		out[i] = s.block[s.offset]
		s.offset++
	}
	return len(out), nil
}

// Uniform reads U32BE and rejects the modulo-biased suffix. It is intentionally
// small and Poker-local until a stable domain-neutral shared primitive exists.
func Uniform(source io.Reader, n uint64) (uint64, error) {
	if n < 1 || n > 1<<32 {
		return 0, errors.New("Poker uniform range")
	}
	limit := uint64(1<<32) - (uint64(1<<32) % n)
	var b [4]byte
	for {
		if _, err := io.ReadFull(source, b[:]); err != nil {
			return 0, err
		}
		x := uint64(binary.BigEndian.Uint32(b[:]))
		if x < limit {
			return x % n, nil
		}
	}
}
func lp(b *bytes.Buffer, s string) {
	_ = binary.Write(b, binary.BigEndian, uint16(len(s)))
	b.WriteString(s)
}
func uuidBytes(id string) ([]byte, error) {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return nil, errors.New("Poker UUID")
	}
	return hex.DecodeString(strings.ReplaceAll(id, "-", ""))
}
