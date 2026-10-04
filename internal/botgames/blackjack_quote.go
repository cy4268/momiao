package botgames

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// DEAL and ROUND have disjoint exact fields. The signed snapshot alone supplies
// round/hand/version authority; callers never submit arbitrary engine authority.
type blackjackQuotePayload struct {
	Version              int     `json:"v"`
	Game                 string  `json:"game"`
	Kind                 string  `json:"kind"`
	Subject              string  `json:"subject"`
	Binding              string  `json:"binding"`
	InitialWager         string  `json:"initial_wager"`
	IssuedAt             string  `json:"issued_at"`
	ExpiresAt            string  `json:"expires_at"`
	RequestID            string  `json:"request_id,omitempty"`
	CommitmentID         string  `json:"commitment_id,omitempty"`
	RoundID              string  `json:"round_id,omitempty"`
	RoundVersion         string  `json:"round_version,omitempty"`
	ActiveHandID         *string `json:"active_hand_id,omitempty"`
	StakeUnits           string  `json:"stake_units,omitempty"`
	ActiveHandStakeUnits string  `json:"active_hand_stake_units,omitempty"`
}

func (s *Service) blackjackQuoteMAC(raw []byte) []byte {
	mac := hmac.New(sha256.New, s.key[:])
	mac.Write([]byte("bot-games.blackjack.quote.v1\x00"))
	mac.Write(raw)
	return mac.Sum(nil)
}
func (s *Service) signBlackjackQuote(p blackjackQuotePayload) (string, error) {
	raw, e := json.Marshal(p)
	if e != nil {
		return "", Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	q := base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(s.blackjackQuoteMAC(raw))
	if len(q) > maxQuoteBytes {
		return "", Fault{Code: "UPSTREAM_UNAVAILABLE"}
	}
	return q, nil
}
func (s *Service) verifyBlackjackQuote(q, kind string) (blackjackQuotePayload, error) {
	var p blackjackQuotePayload
	invalid := Fault{Code: "INVALID_REQUEST"}
	if len(q) == 0 || len(q) > maxQuoteBytes {
		return p, invalid
	}
	parts := strings.Split(q, ".")
	if len(parts) != 2 {
		return p, invalid
	}
	raw, e := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if e != nil || base64.RawURLEncoding.EncodeToString(raw) != parts[0] {
		return p, invalid
	}
	sig, e := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if e != nil || base64.RawURLEncoding.EncodeToString(sig) != parts[1] || !hmac.Equal(sig, s.blackjackQuoteMAC(raw)) {
		return p, Fault{Code: "UNAUTHORIZED"}
	}
	fields := map[string]any{"v": &p.Version, "game": &p.Game, "kind": &p.Kind, "subject": &p.Subject, "binding": &p.Binding, "initial_wager": &p.InitialWager, "issued_at": &p.IssuedAt, "expires_at": &p.ExpiresAt}
	if kind == "DEAL" {
		fields["request_id"] = &p.RequestID
		fields["commitment_id"] = &p.CommitmentID
	} else if kind == "ROUND" {
		p.ActiveHandID = new(string)
		fields["round_id"] = &p.RoundID
		fields["round_version"] = &p.RoundVersion
		fields["active_hand_id"] = p.ActiveHandID
		fields["stake_units"] = &p.StakeUnits
		fields["active_hand_stake_units"] = &p.ActiveHandStakeUnits
	} else {
		return p, invalid
	}
	if exactScratchObject(raw, fields) != nil {
		return p, invalid
	}
	canonical, _ := json.Marshal(p)
	if !bytes.Equal(raw, canonical) {
		return p, invalid
	}
	initial, ok := blackjackWagerUnits(p.InitialWager)
	iat, iv := blackjackDecimal(p.IssuedAt)
	exp, ev := blackjackDecimal(p.ExpiresAt)
	if p.Version != 1 || p.Game != "blackjack" || p.Kind != kind || !validID(p.Subject) || !validHash(p.Binding) || !ok || !iv || !ev || iat <= 0 || iat > math.MaxInt64-120 || exp != iat+120 || iat > s.now().Unix() {
		return p, invalid
	}
	if kind == "DEAL" {
		if !validID(p.RequestID) || !validBlackjackRecordID(p.CommitmentID) {
			return p, invalid
		}
	} else {
		version, vv := blackjackDecimal(p.RoundVersion)
		stake, sv := blackjackDecimal(p.StakeUnits)
		active, av := blackjackDecimal(p.ActiveHandStakeUnits)
		if !validBlackjackRecordID(p.RoundID) || !vv || version < 1 || version == math.MaxInt64 || !sv || stake < initial || stake > initial*8 || !av {
			return p, invalid
		}
		if *p.ActiveHandID == "" {
			if active != 0 {
				return p, invalid
			}
		} else if !validBlackjackRecordID(*p.ActiveHandID) || (active != initial && active != initial*2) {
			return p, invalid
		}
	}
	return p, nil
}
func blackjackDecimal(v string) (int64, bool) {
	n, e := strconv.ParseInt(v, 10, 64)
	return n, e == nil && n >= 0 && strconv.FormatInt(n, 10) == v
}
func (s *Service) blackjackActionID(subject, request string) (string, error) {
	if !validID(subject) || !validID(request) {
		return "", Fault{Code: "INVALID_REQUEST"}
	}
	n, _ := strconv.ParseUint(request, 10, 64)
	millis := (n >> 22) + 1420070400000
	mac := hmac.New(sha256.New, s.key[:])
	mac.Write([]byte("bot-games.blackjack.action-id.v1\x00" + subject + "\x00" + request))
	b := mac.Sum(nil)[:16]
	for i := 5; i >= 0; i-- {
		b[i] = byte(millis)
		millis >>= 8
	}
	b[6] = b[6]&15 | 0x70
	b[8] = b[8]&63 | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// Mutable game records use the same UUIDv7/variant requirement as games.uuidBytes.
func validBlackjackRecordID(v string) bool {
	return validUUID(v) && v[14] == '7' && strings.ContainsRune("89ab", rune(v[19]))
}
