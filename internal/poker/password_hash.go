package poker

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Explicit parameters only; no production defaults or feature enablement.
type PasswordConfig struct {
	MemoryKiB, Time uint32
	Parallelism     uint8
}

func (c PasswordConfig) valid() bool {
	// Allocation safety ceilings, not a production cost recommendation.
	return c.Time > 0 && c.Time <= 10 && c.Parallelism > 0 && c.Parallelism <= 16 &&
		c.MemoryKiB >= 8*uint32(c.Parallelism) && c.MemoryKiB <= 262144
}

func (c PasswordConfig) prefix() string {
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$", c.MemoryKiB, c.Time, c.Parallelism)
}

func validTablePassword(password string) bool {
	return len(password) >= 1 && len(password) <= 128 && utf8.ValidString(password)
}

func HashTablePassword(c PasswordConfig, password string) (string, error) {
	if !c.valid() || !validTablePassword(password) {
		return "", ErrInvalid
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", ErrInvalid
	}
	key := argon2.IDKey([]byte(password), salt, c.Time, c.MemoryKiB, c.Parallelism, 32)
	return c.prefix() + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key), nil
}

func VerifyTablePassword(c PasswordConfig, phc, password string) (bool, error) {
	if !c.valid() || !validTablePassword(password) || len(phc) > 512 {
		return false, ErrInvalid
	}
	// Compare the full configured profile before parsing or deriving: stored costs
	// never control allocations. Reject algorithm/version, ordering and numeric drift.
	body, ok := strings.CutPrefix(phc, c.prefix())
	if !ok {
		return false, ErrInvalid
	}
	saltText, keyText, ok := strings.Cut(body, "$")
	if !ok || len(saltText) != 22 || len(keyText) != 43 {
		return false, ErrInvalid
	}
	salt, saltErr := base64.RawStdEncoding.Strict().DecodeString(saltText)
	key, keyErr := base64.RawStdEncoding.Strict().DecodeString(keyText)
	if saltErr != nil || keyErr != nil || len(salt) != 16 || len(key) != 32 ||
		base64.RawStdEncoding.EncodeToString(salt) != saltText || base64.RawStdEncoding.EncodeToString(key) != keyText {
		return false, ErrInvalid
	}
	derived := argon2.IDKey([]byte(password), salt, c.Time, c.MemoryKiB, c.Parallelism, 32)
	return subtle.ConstantTimeCompare(derived, key) == 1, nil
}
