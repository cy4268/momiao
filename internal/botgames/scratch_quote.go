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

type scratchQuotePayload struct {
	Version      int    `json:"v"`
	Game         string `json:"game"`
	Action       string `json:"action"`
	RequestID    string `json:"request_id"`
	Subject      string `json:"subject"`
	Binding      string `json:"binding"`
	Wager        string `json:"wager"`
	CommitmentID string `json:"commitment_id"`
	RoundID      string `json:"round_id"`
	IssuedAt     int64  `json:"iat"`
	ExpiresAt    int64  `json:"exp"`
}

func (s *Service) scratchQuoteMAC(raw []byte) []byte {
	mac := hmac.New(sha256.New, s.key[:])
	mac.Write([]byte("bot-games.scratch.quote.v1\x00"))
	mac.Write(raw)
	return mac.Sum(nil)
}
func (s *Service) signScratchQuote(p scratchQuotePayload) (string, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return "", Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	q := base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(s.scratchQuoteMAC(raw))
	if len(q) > maxQuoteBytes {
		return "", Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	return q, nil
}
func (s *Service) verifyScratchQuote(quote string) (scratchQuotePayload, error) {
	var p scratchQuotePayload
	invalid := Fault{Code: "INVALID_REQUEST"}
	if len(quote) == 0 || len(quote) > maxQuoteBytes {
		return p, invalid
	}
	parts := strings.Split(quote, ".")
	if len(parts) != 2 {
		return p, invalid
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != parts[0] {
		return p, invalid
	}
	sig, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || base64.RawURLEncoding.EncodeToString(sig) != parts[1] || !hmac.Equal(sig, s.scratchQuoteMAC(raw)) {
		return p, Fault{Code: "UNAUTHORIZED"}
	}
	if exactScratchObject(raw, map[string]any{"v": &p.Version, "game": &p.Game, "action": &p.Action, "request_id": &p.RequestID, "subject": &p.Subject, "binding": &p.Binding, "wager": &p.Wager, "commitment_id": &p.CommitmentID, "round_id": &p.RoundID, "iat": &p.IssuedAt, "exp": &p.ExpiresAt}) != nil {
		return p, invalid
	}
	_, valid := scratchWagerUnits(p.Wager)
	if p.Version != 1 || p.Game != "scratch" || !validID(p.RequestID) || !validID(p.Subject) || !validHash(p.Binding) || !valid || p.IssuedAt <= 0 || p.IssuedAt > math.MaxInt64-120 || p.ExpiresAt != p.IssuedAt+120 || p.IssuedAt > s.now().Unix() {
		return p, invalid
	}
	switch p.Action {
	case "PURCHASE":
		if !validUUID(p.CommitmentID) || p.RoundID != "" {
			return p, invalid
		}
	case "RESUME":
		if !validUUID(p.RoundID) || p.CommitmentID != "" {
			return p, invalid
		}
	default:
		return p, invalid
	}
	return p, nil
}

// The signed payload and immutable resource reference both reject unknown,
// duplicate, null and case-aliased fields rather than relying on struct decode.
func exactScratchObject(raw []byte, fields map[string]any) error {
	invalid := Fault{Code: "INVALID_REQUEST"}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return invalid
	}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return invalid
		}
		name, ok := token.(string)
		if !ok {
			return invalid
		}
		target, ok := fields[name]
		if !ok {
			return invalid
		}
		var value json.RawMessage
		if d.Decode(&value) != nil || bytes.Equal(value, []byte("null")) || json.Unmarshal(value, target) != nil {
			return invalid
		}
		delete(fields, name)
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') || len(fields) != 0 || d.Decode(new(json.RawMessage)) != io.EOF {
		return invalid
	}
	return nil
}
