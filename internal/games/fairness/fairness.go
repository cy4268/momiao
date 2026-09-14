// Package fairness provides the frozen deterministic Direct Play random stream. It performs no
// I/O, balance changes, seed generation, persistence, or lifecycle authorization.
package fairness

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"unicode"
	"unicode/utf8"
)

const StreamVersion = "chaldea-pf-hmac-sha256-v1"

var (
	ErrInvalidInput    = errors.New("gameplay: invalid input")
	ErrSeedMismatch    = errors.New("gameplay: seed commitment mismatch")
	ErrConfigMismatch  = errors.New("gameplay: configuration binding mismatch")
	ErrInvalidConfig   = errors.New("gameplay: invalid configuration")
	ErrStreamExhausted = errors.New("gameplay: random stream exhausted")
)

// Round contains the public, locked inputs. ConfigHash is verified against the
// actual snapshot by each game mapper, not inserted into the frozen HMAC message.
// ID is the reserved UUIDv7 in its 16-byte network representation.
type Round struct {
	ID               [16]byte
	Game             string
	ClientSeed       string
	Nonce            int64
	StreamVersion    string
	AlgorithmVersion string
	ConfigVersion    string
	ConfigHash       [32]byte
	ServerSeedHash   [32]byte
}

// SeedCommitment hashes exactly 32 raw bytes. The caller must generate those
// bytes with a CSPRNG and publish the commitment before accepting a new round.
func SeedCommitment(serverSeed []byte) ([32]byte, error) {
	if len(serverSeed) != sha256.Size {
		return [32]byte{}, ErrInvalidInput
	}
	return sha256.Sum256(serverSeed), nil
}

// Stream is the IS §263 HMAC byte stream. It retains a private in-memory seed
// copy and is not safe for concurrent Read calls. Never log or serialize it.
type Stream struct {
	seed       [32]byte
	message    []byte
	block      [32]byte
	offset     int
	blockIndex uint64
	exhausted  bool
}

// NewStream validates the public inputs and checks the supplied seed against
// its commitment. It cannot prove that the seed has legally been revealed, was
// generated randomly, or has never been reused; those are service obligations.
func NewStream(serverSeed []byte, round Round, domain string) (*Stream, error) {
	commitment, err := SeedCommitment(serverSeed)
	if err != nil {
		return nil, err
	}
	if !validLabel(round.Game, 128) || !validLabel(round.AlgorithmVersion, 128) ||
		!validLabel(round.ConfigVersion, 128) || !validLabel(domain, math.MaxUint16) ||
		round.StreamVersion != StreamVersion || round.Nonce < 0 ||
		round.ID[6]>>4 != 7 || round.ID[8]>>6 != 2 ||
		round.ConfigHash == ([32]byte{}) || !validClientSeed(round.ClientSeed) {
		return nil, ErrInvalidInput
	}
	if !hmac.Equal(commitment[:], round.ServerSeedHash[:]) {
		return nil, ErrSeedMismatch
	}
	s := &Stream{message: canonicalStreamMessage(round, domain, 0), offset: sha256.Size}
	copy(s.seed[:], serverSeed)
	return s, nil
}

func validLabel(s string, maxBytes int) bool {
	if len(s) == 0 || len(s) > maxBytes {
		return false
	}
	for i := range len(s) {
		if s[i] < '!' || s[i] > '~' {
			return false
		}
	}
	return true
}

func validClientSeed(s string) bool {
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

// appendLP16 is used only after length/encoding validation.
func appendLP16(dst []byte, s string) []byte {
	dst = binary.BigEndian.AppendUint16(dst, uint16(len(s)))
	return append(dst, s...)
}

func canonicalStreamMessage(r Round, domain string, block uint64) []byte {
	b := []byte("CHALDEA-PF-HMAC-SHA256-V1\x00")
	b = appendLP16(b, r.Game)
	b = append(b, r.ID[:]...)
	b = binary.BigEndian.AppendUint64(b, uint64(r.Nonce))
	b = appendLP16(b, r.ClientSeed)
	b = appendLP16(b, r.AlgorithmVersion)
	b = appendLP16(b, domain)
	return binary.BigEndian.AppendUint64(b, block)
}

func (s *Stream) Read(p []byte) (int, error) {
	n := 0
	for len(p) > 0 {
		if s.offset == len(s.block) {
			if s.exhausted {
				return n, ErrStreamExhausted
			}
			binary.BigEndian.PutUint64(s.message[len(s.message)-8:], s.blockIndex)
			mac := hmac.New(sha256.New, s.seed[:])
			_, _ = mac.Write(s.message)
			mac.Sum(s.block[:0])
			s.offset = 0
			if s.blockIndex == math.MaxUint64 {
				s.exhausted = true
			} else {
				s.blockIndex++
			}
		}
		copied := copy(p, s.block[s.offset:])
		s.offset += copied
		n += copied
		p = p[copied:]
	}
	return n, nil
}

// UniformInt implements IS §265: sample U32BE, reject the incomplete tail,
// then reduce modulo n. The result is in [0,n), with 1 <= n <= 2^32.
// Game mappers always pass a domain-separated Stream as the source.
func UniformInt(source io.Reader, n uint64) (uint64, error) {
	if source == nil || n == 0 || n > 1<<32 {
		return 0, ErrInvalidInput
	}
	limit := uint64(1<<32) - uint64(1<<32)%n
	var word [4]byte
	for {
		if _, err := io.ReadFull(source, word[:]); err != nil {
			return 0, err
		}
		x := uint64(binary.BigEndian.Uint32(word[:]))
		if x < limit {
			return x % n, nil
		}
	}
}
