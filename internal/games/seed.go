package games

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/platform"
)

type Keyring struct {
	Active string              `json:"active"`
	Keys   map[string][32]byte `json:"-"`
}
type Service struct {
	store    *platform.Store
	keyring  Keyring
	observer platform.NativeQuotaObserver
}

func NewService(store *platform.Store, keys Keyring) (*Service, error) {
	return NewServiceWithEconomy(store, keys, nil)
}
func NewServiceWithEconomy(store *platform.Store, keys Keyring, observer platform.NativeQuotaObserver) (*Service, error) {
	if store == nil || !validVersion(keys.Active) || len(keys.Keys) == 0 || len(keys.Keys) > 16 {
		return nil, ErrUnavailable
	}
	if _, ok := keys.Keys[keys.Active]; !ok {
		return nil, ErrUnavailable
	}
	s := &Service{store: store, observer: observer, keyring: Keyring{Active: keys.Active, Keys: make(map[string][32]byte, len(keys.Keys))}}
	for version, key := range keys.Keys {
		if !validVersion(version) {
			return nil, ErrUnavailable
		}
		s.keyring.Keys[version] = key
	}
	return s, nil
}

// ReadKeyring accepts a private bounded JSON file {active, keys:{version:hex}}.
// No fallback ephemeral key: restarts must decrypt every outstanding commitment.
func ReadKeyring(path string) (Keyring, error) {
	file, err := os.Open(path)
	if err != nil {
		return Keyring{}, ErrUnavailable
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() > 8192 {
		return Keyring{}, ErrUnavailable
	}
	var input struct {
		Active string            `json:"active"`
		Keys   map[string]string `json:"keys"`
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil {
		return Keyring{}, ErrUnavailable
	}
	ring := Keyring{Active: input.Active, Keys: map[string][32]byte{}}
	for version, value := range input.Keys {
		raw, e := hex.DecodeString(value)
		if e != nil || len(raw) != 32 {
			return Keyring{}, ErrUnavailable
		}
		ring.Keys[version] = [32]byte(raw)
	}
	if !validVersion(ring.Active) || len(ring.Keys) == 0 || len(ring.Keys) > 16 {
		return Keyring{}, ErrUnavailable
	}
	if _, ok := ring.Keys[ring.Active]; !ok {
		return Keyring{}, ErrUnavailable
	}
	return ring, nil
}
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	millis := uint64(time.Now().UnixMilli())
	for i := 5; i >= 0; i-- {
		b[i] = byte(millis)
		millis >>= 8
	}
	b[6] = (b[6] & 15) | 0x70
	b[8] = (b[8] & 63) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
func uuidBytes(id string) ([16]byte, error) {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return [16]byte{}, ErrInvalidInput
	}
	b, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	if err != nil || len(b) != 16 || b[6]>>4 != 7 || b[8]>>6 != 2 {
		return [16]byte{}, ErrInvalidInput
	}
	return [16]byte(b), nil
}
func seedAAD(user int64, slug string, c Commitment) ([]byte, error) {
	id, err := uuidBytes(c.ID)
	if err != nil {
		return nil, err
	}
	round, err := uuidBytes(c.ReservedRoundID)
	if err != nil {
		return nil, err
	}
	b := []byte("CHALDEA-GAME-SEED-AAD-V1\x00")
	b = append(b, id[:]...)
	b = append(b, round[:]...)
	b = appendLP16(b, strconv.FormatInt(user, 10))
	b = appendLP16(b, slug)
	b = binary.BigEndian.AppendUint64(b, uint64(c.Nonce))
	b = appendLP16(b, c.Algorithm)
	if c.EconomicVersion != "" {
		b = appendLP16(b, c.EconomicVersion)
	}
	return b, nil
}
func (s *Service) gcm(version string) (cipher.AEAD, error) {
	key, ok := s.keyring.Keys[version]
	if !ok {
		return nil, ErrUnavailable
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func (s *Service) sealSeed(user int64, slug string, c Commitment, seed []byte) ([]byte, []byte, error) {
	aead, err := s.gcm(s.keyring.Active)
	if err != nil {
		return nil, nil, err
	}
	aad, err := seedAAD(user, slug, c)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, 12)
	if _, err = rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return nonce, aead.Seal(nil, nonce, seed, aad), nil
}
func (s *Service) openSeed(user int64, slug string, c Commitment, key string, nonce, encrypted []byte) ([]byte, error) {
	aead, err := s.gcm(key)
	if err != nil {
		return nil, err
	}
	aad, err := seedAAD(user, slug, c)
	if err != nil {
		return nil, err
	}
	if len(nonce) != 12 || len(encrypted) != 48 {
		return nil, ErrUnavailable
	}
	seed, err := aead.Open(nil, nonce, encrypted, aad)
	if err != nil || len(seed) != 32 {
		return nil, ErrUnavailable
	}
	hash := sha256.Sum256(seed)
	if hex.EncodeToString(hash[:]) != c.ServerSeedHash {
		return nil, ErrUnavailable
	}
	return seed, nil
}
func (s *Service) CSRFToken(user int64, session string) string {
	key := s.keyring.Keys[s.keyring.Active]
	mac := hmac.New(sha256.New, key[:])
	mac.Write([]byte("game.csrf.v1\x00" + strconv.FormatInt(user, 10) + "\x00" + session))
	return hex.EncodeToString(mac.Sum(nil))
}
func (s *Service) ValidCSRF(user int64, session, token string) bool {
	return session != "" && len(token) == 64 && hmac.Equal([]byte(s.CSRFToken(user, session)), []byte(token))
}
