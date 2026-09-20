package roulette

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/games/fairness"
)

type Contribution struct {
	Seat    uint16 `json:"seat"`
	Version uint64 `json:"version,string"`
	Seed    string `json:"seed"`
}

func validSeed(s string) bool {
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
func effectiveClientSeed(round [16]byte, game string, hash [32]byte, values []Contribution) (string, error) {
	if (game != "devil-roulette" && game != "pressure-roulette") || len(values) < 2 || len(values) > 6 {
		return "", ErrInvalidInput
	}
	values = slices.Clone(values)
	slices.SortFunc(values, func(a, b Contribution) int { return int(a.Seat) - int(b.Seat) })
	b := append([]byte("MOMIAO-ROULETTE-CLIENT-V1\x00"), round[:]...)
	b = binary.BigEndian.AppendUint16(b, uint16(len(game)))
	b = append(b, game...)
	b = append(b, hash[:]...)
	b = binary.BigEndian.AppendUint16(b, uint16(len(values)))
	for i, v := range values {
		if v.Seat > 5 || v.Version == 0 || !validSeed(v.Seed) || (i > 0 && values[i-1].Seat == v.Seat) {
			return "", ErrInvalidInput
		}
		b = binary.BigEndian.AppendUint16(b, v.Seat)
		b = binary.BigEndian.AppendUint64(b, v.Version)
		b = binary.BigEndian.AppendUint16(b, uint16(len(v.Seed)))
		b = append(b, v.Seed...)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
func uuidBytes(id string) ([16]byte, error) {
	var b [16]byte
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' || strings.ToLower(id) != id {
		return b, ErrInvalidInput
	}
	raw, e := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	if e != nil || len(raw) != 16 {
		return b, ErrInvalidInput
	}
	copy(b[:], raw)
	if b[6]>>4 != 7 || b[8]>>6 != 2 {
		return b, ErrInvalidInput
	}
	return b, nil
}
func ValidRoundID(id string) bool { _, e := uuidBytes(id); return e == nil }
func newID() (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	ms := uint64(time.Now().UnixMilli())
	for i := 5; i >= 0; i-- {
		b[i] = byte(ms)
		ms >>= 8
	}
	b[6] = b[6]&15 | 0x70
	b[8] = b[8]&63 | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}

type sealed struct {
	Key               string
	Nonce, Ciphertext []byte
}

func aad(label string, id [16]byte, hash [32]byte, version int64) []byte {
	b := append([]byte(label+"\x00"), id[:]...)
	b = append(b, hash[:]...)
	if version > 0 {
		b = binary.BigEndian.AppendUint64(b, uint64(version))
	}
	return b
}
func (k Keyring) gcm(version string) (cipher.AEAD, error) {
	key, ok := k.Keys[version]
	if !ok {
		return nil, ErrUnavailable
	}
	block, e := aes.NewCipher(key[:])
	if e != nil {
		return nil, e
	}
	return cipher.NewGCM(block)
}
func (k Keyring) seal(data, aad []byte) (sealed, error) {
	g, e := k.gcm(k.Active)
	if e != nil {
		return sealed{}, e
	}
	n := make([]byte, g.NonceSize())
	if _, e = rand.Read(n); e != nil {
		return sealed{}, e
	}
	return sealed{k.Active, n, g.Seal(nil, n, data, aad)}, nil
}
func (k Keyring) open(v sealed, aad []byte) ([]byte, error) {
	g, e := k.gcm(v.Key)
	if e != nil || len(v.Nonce) != 12 {
		return nil, ErrUnavailable
	}
	b, e := g.Open(nil, v.Nonce, v.Ciphertext, aad)
	if e != nil {
		return nil, ErrUnavailable
	}
	return b, nil
}
func ruleHash(state any) [32]byte {
	b, e := json.Marshal(state)
	if e != nil {
		panic("nonserializable roulette rule state")
	}
	return sha256.Sum256(b)
}
func randomInt(source io.Reader, n int) (int, error) {
	v, e := fairness.UniformInt(source, uint64(n))
	return int(v), e
}
func shuffle[T any](r io.Reader, values []T) error {
	for i := len(values) - 1; i > 0; i-- {
		j, e := randomInt(r, i+1)
		if e != nil {
			return e
		}
		values[i], values[j] = values[j], values[i]
	}
	return nil
}
