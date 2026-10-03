package botgames

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math"
	"strings"
)

// This is a separate contract, not a new interpretation of legacy dice quotes.
type summonQuotePayload struct {
	Version      int    `json:"v"`
	Game         string `json:"game"`
	RequestID    string `json:"request_id"`
	Subject      string `json:"subject"`
	Binding      string `json:"binding"`
	BaseWager    string `json:"base_wager"`
	Mode         string `json:"mode"`
	CommitmentID string `json:"commitment_id"`
	IssuedAt     int64  `json:"iat"`
	ExpiresAt    int64  `json:"exp"`
}

func (s *Service) summonQuoteMAC(raw []byte) []byte {
	mac := hmac.New(sha256.New, s.key[:])
	mac.Write([]byte("bot-games.summon.quote.v1\x00"))
	mac.Write(raw)
	return mac.Sum(nil)
}

func (s *Service) signSummonQuote(p summonQuotePayload) (string, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return "", Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	quote := base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(s.summonQuoteMAC(raw))
	if len(quote) > maxQuoteBytes {
		return "", Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	return quote, nil
}

func (s *Service) verifySummonQuote(quote string) (summonQuotePayload, error) {
	invalid := Fault{Code: "INVALID_REQUEST"}
	if len(quote) == 0 || len(quote) > maxQuoteBytes {
		return summonQuotePayload{}, invalid
	}
	parts := strings.Split(quote, ".")
	if len(parts) != 2 {
		return summonQuotePayload{}, invalid
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != parts[0] {
		return summonQuotePayload{}, invalid
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || base64.RawURLEncoding.EncodeToString(signature) != parts[1] || !hmac.Equal(signature, s.summonQuoteMAC(raw)) {
		return summonQuotePayload{}, Fault{Code: "UNAUTHORIZED"}
	}
	p, err := parseSummonQuote(raw)
	if err != nil {
		return summonQuotePayload{}, invalid
	}
	_, _, valid := summonWagerUnits(p.BaseWager, p.Mode)
	if p.Version != 1 || !validID(p.RequestID) || !validID(p.Subject) || !valid || p.Game != "summon" ||
		!validUUID(p.CommitmentID) || !validHash(p.Binding) || p.IssuedAt <= 0 || p.IssuedAt > math.MaxInt64-120 ||
		p.ExpiresAt != p.IssuedAt+120 || p.IssuedAt > s.now().Unix() {
		return summonQuotePayload{}, invalid
	}
	return p, nil
}

// Decode exact field names once each; encoding/json's struct decoder alone
// accepts duplicate keys and case aliases. Authenticate bytes before parsing.
func parseSummonQuote(raw []byte) (summonQuotePayload, error) {
	var p summonQuotePayload
	fields := map[string]any{"v": &p.Version, "request_id": &p.RequestID, "subject": &p.Subject,
		"binding": &p.Binding, "base_wager": &p.BaseWager, "mode": &p.Mode, "game": &p.Game, "commitment_id": &p.CommitmentID,
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
