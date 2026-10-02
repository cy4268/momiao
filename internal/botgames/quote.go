package botgames

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/cy4268/momiao/internal/games"
)

const maxQuoteBytes = 2048

type quotePayload struct {
	Version      int    `json:"v"`
	RequestID    string `json:"request_id"`
	Subject      string `json:"subject"`
	Binding      string `json:"binding"`
	Wager        string `json:"wager"`
	Choice       string `json:"choice"`
	CommitmentID string `json:"commitment_id"`
	IssuedAt     int64  `json:"iat"`
	ExpiresAt    int64  `json:"exp"`
}

func (s *Service) binding(subject string, user int64) string {
	mac := hmac.New(sha256.New, s.key[:])
	mac.Write([]byte("bot-games.binding.v1\x00" + subject + "\x00" + strconv.FormatInt(user, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) quoteMAC(raw []byte) []byte {
	mac := hmac.New(sha256.New, s.key[:])
	mac.Write([]byte("bot-games.quote.v1\x00"))
	mac.Write(raw)
	return mac.Sum(nil)
}

func (s *Service) signQuote(p quotePayload) (string, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return "", Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	quote := base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(s.quoteMAC(raw))
	if len(quote) > maxQuoteBytes {
		return "", Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	return quote, nil
}

func (s *Service) verifyQuote(quote string) (quotePayload, error) {
	invalid := Fault{Code: "INVALID_REQUEST"}
	if len(quote) == 0 || len(quote) > maxQuoteBytes {
		return quotePayload{}, invalid
	}
	parts := strings.Split(quote, ".")
	if len(parts) != 2 {
		return quotePayload{}, invalid
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != parts[0] {
		return quotePayload{}, invalid
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || base64.RawURLEncoding.EncodeToString(signature) != parts[1] || !hmac.Equal(signature, s.quoteMAC(raw)) {
		return quotePayload{}, Fault{Code: "UNAUTHORIZED"}
	}
	p, err := parseQuote(raw)
	if err != nil {
		return quotePayload{}, invalid
	}
	_, valid := wagerUnits(p.Wager)
	if p.Version != 1 || !validID(p.RequestID) || !validID(p.Subject) || !valid || !validChoice(p.Choice) ||
		!validUUID(p.CommitmentID) || !validHash(p.Binding) || p.IssuedAt <= 0 || p.IssuedAt > math.MaxInt64-120 ||
		p.ExpiresAt != p.IssuedAt+120 || p.IssuedAt > s.now().Unix() {
		return quotePayload{}, invalid
	}
	return p, nil
}

// Decode exact field names once each; encoding/json's struct decoder alone
// accepts duplicate keys and case aliases. Authenticate bytes before parsing.
func parseQuote(raw []byte) (quotePayload, error) {
	var p quotePayload
	fields := map[string]any{"v": &p.Version, "request_id": &p.RequestID, "subject": &p.Subject,
		"binding": &p.Binding, "wager": &p.Wager, "choice": &p.Choice, "commitment_id": &p.CommitmentID,
		"iat": &p.IssuedAt, "exp": &p.ExpiresAt}
	invalid := Fault{Code: "INVALID_REQUEST"}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return p, invalid
	}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return p, invalid
		}
		name, ok := token.(string)
		if !ok {
			return p, invalid
		}
		target, ok := fields[name]
		if !ok {
			return p, invalid
		}
		var value json.RawMessage
		if d.Decode(&value) != nil || bytes.Equal(value, []byte("null")) || json.Unmarshal(value, target) != nil {
			return p, invalid
		}
		delete(fields, name)
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') || len(fields) != 0 {
		return p, invalid
	}
	if d.Decode(new(json.RawMessage)) != io.EOF {
		return p, invalid
	}
	return p, nil
}

func validID(value string) bool {
	if !canonicalPositive(value) {
		return false
	}
	_, err := strconv.ParseUint(value, 10, 64)
	return err == nil
}

func canonicalPositive(value string) bool {
	if len(value) == 0 || len(value) > 20 || value[0] < '1' || value[0] > '9' {
		return false
	}
	for i := 1; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func wagerUnits(value string) (int64, bool) {
	if !canonicalPositive(value) {
		return 0, false
	}
	chips, err := strconv.ParseInt(value, 10, 64)
	if err != nil || chips > math.MaxInt64/(2*games.UnitsPerChip) {
		return 0, false
	}
	return chips * games.UnitsPerChip, true
}

func validChoice(choice string) bool { return choice == "BIG" || choice == "SMALL" }

func validHash(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == sha256.Size && hex.EncodeToString(raw) == value
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	raw, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return err == nil && len(raw) == 16 && strings.ToLower(value) == value
}
