package botroulette

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
	"unicode"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/roulette"
)

const maxQuoteBytes = 4096

type quotePayload struct {
	Version   int        `json:"v"`
	Subject   string     `json:"subject"`
	Binding   string     `json:"binding"`
	RequestID string     `json:"request_id"`
	Purpose   string     `json:"purpose"`
	Game      string     `json:"game"`
	RoomID    *string    `json:"room_id"`
	Input     TypedInput `json:"input"`
	IssuedAt  string     `json:"issued_at"`
	ExpiresAt string     `json:"expires_at"`
}

func (s *Service) binding(subject string, user int64) string {
	mac := hmac.New(sha256.New, s.key[:])
	mac.Write([]byte("bot-roulette.binding.v1\x00" + subject + "\x00" + strconv.FormatInt(user, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}
func (s *Service) quoteMAC(raw []byte) []byte {
	mac := hmac.New(sha256.New, s.key[:])
	mac.Write([]byte("bot-roulette.quote.v1\x00"))
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
	var p quotePayload
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
	if err != nil || base64.RawURLEncoding.EncodeToString(sig) != parts[1] || !hmac.Equal(sig, s.quoteMAC(raw)) {
		return p, Fault{Code: "UNAUTHORIZED"}
	}
	if json.Unmarshal(raw, &p) != nil {
		return p, invalid
	}
	// Only canonical typed payloads are signed. Requiring the same bytes also
	// rejects unknown/duplicate/case-alias keys, null scalars and alternate JSON
	// representations at the outer level, before any identity or engine lookup.
	canonical, err := json.Marshal(p)
	if err != nil || !bytes.Equal(raw, canonical) || p.Version != 1 || !validID(p.Subject) || !validHash(p.Binding) || !validIntent(PrepareInput{RequestID: p.RequestID, Purpose: p.Purpose, Game: p.Game, RoomID: p.RoomID, Input: p.Input}) {
		return p, invalid
	}
	iat, exp := quoteTimes(p.RequestID)
	if p.IssuedAt != strconv.FormatInt(iat, 10) || p.ExpiresAt != strconv.FormatInt(exp, 10) || iat > s.now().UnixMilli() {
		return p, invalid
	}
	// Expiry is intentionally not checked here: recovery authenticates old
	// decisions; the guarded engine arbitrates new commits using locked DB time.
	return p, nil
}
func quoteTimes(requestID string) (int64, int64) {
	id, _ := strconv.ParseUint(requestID, 10, 64)
	iat := int64(id>>22) + 1420070400000
	return iat, iat + 120000
}
func validID(s string) bool {
	n, err := strconv.ParseUint(s, 10, 64)
	return err == nil && n > 0 && strconv.FormatUint(n, 10) == s
}
func canonicalInt(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil && strconv.FormatInt(n, 10) == s
}
func validHash(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32 && hex.EncodeToString(b) == s
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
func validIntent(in PrepareInput) bool {
	if !validID(in.RequestID) || !roulette.IsGame(in.Game) {
		return false
	}
	switch in.Purpose {
	case "CREATE":
		c := in.Input.Create
		if in.RoomID != nil || c == nil || in.Input.Command != nil {
			return false
		}
		chips, ok := canonicalInt(c.Stake)
		if !ok || chips <= 0 {
			return false
		}
		if (in.Game == "devil-roulette" && c.Players != 2) || (in.Game == "pressure-roulette" && (c.Players < 3 || c.Players > 6)) {
			return false
		}
		amount, err := platform.ParseAmount(c.Stake)
		return err == nil && amount > 0 && amount <= math.MaxInt64/int64(c.Players)
	case "COMMAND":
		c := in.Input.Command
		if in.RoomID == nil || !roulette.ValidRoundID(*in.RoomID) || c == nil || in.Input.Create != nil || c.ExpectedVersion < 1 || !roulette.ValidAction(domainAction(c.Action), false) || (c.Action.Kind == "READY") != (c.Ready != nil) {
			return false
		}
		if r := c.Ready; r != nil {
			return validSeed(r.ClientSeed) && validHash(r.ConfigHash) && validHash(r.PolicyHash) && validHash(r.ServerSeedHash) && r.StakeUnits > 0
		}
		return true
	default:
		return false
	}
}

// exactObject is used at every request nesting level. A struct decoder alone
// accepts duplicate keys, case aliases and null scalars, all invalid on wire.
func exactObject(raw []byte, required, optional string) (map[string]json.RawMessage, error) {
	invalid := Fault{Code: "INVALID_REQUEST"}
	fields := map[string]json.RawMessage{}
	allowed := map[string]bool{}
	for _, names := range []string{required, optional} {
		if names != "" {
			for _, name := range strings.Split(names, ",") {
				allowed[name] = true
			}
		}
	}
	if !utf8.Valid(raw) {
		return nil, invalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, invalid
	}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return nil, invalid
		}
		name, ok := token.(string)
		if !ok || !allowed[name] {
			return nil, invalid
		}
		if _, exists := fields[name]; exists {
			return nil, invalid
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return nil, invalid
		}
		fields[name] = value
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') || d.Decode(new(json.RawMessage)) != io.EOF {
		return nil, invalid
	}
	if required != "" {
		for _, name := range strings.Split(required, ",") {
			if _, ok := fields[name]; !ok {
				return nil, invalid
			}
		}
	}
	return fields, nil
}
func decodeFields(fields map[string]json.RawMessage, targets map[string]any) error {
	for name, target := range targets {
		raw, ok := fields[name]
		if !ok || bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, target) != nil {
			return Fault{Code: "INVALID_REQUEST"}
		}
	}
	return nil
}
func (in *LobbyInput) UnmarshalJSON(raw []byte) error {
	f, err := exactObject(raw, "game,cursor", "")
	if err != nil {
		return err
	}
	var out LobbyInput
	if err = decodeFields(f, map[string]any{"game": &out.Game}); err != nil {
		return err
	}
	if json.Unmarshal(f["cursor"], &out.Cursor) != nil {
		return Fault{Code: "INVALID_REQUEST"}
	}
	*in = out
	return nil
}
func (in *RoomInput) UnmarshalJSON(raw []byte) error {
	f, err := exactObject(raw, "game,room_id", "")
	if err != nil {
		return err
	}
	var out RoomInput
	if err = decodeFields(f, map[string]any{"game": &out.Game, "room_id": &out.RoomID}); err != nil {
		return err
	}
	*in = out
	return nil
}
func (in *StateInput) UnmarshalJSON(raw []byte) error {
	f, err := exactObject(raw, "room_id", "")
	if err != nil {
		return err
	}
	var out StateInput
	if err = decodeFields(f, map[string]any{"room_id": &out.RoomID}); err != nil {
		return err
	}
	*in = out
	return nil
}
func (in *QuoteInput) UnmarshalJSON(raw []byte) error {
	f, err := exactObject(raw, "quote", "")
	if err != nil {
		return err
	}
	var out QuoteInput
	if err = decodeFields(f, map[string]any{"quote": &out.Quote}); err != nil {
		return err
	}
	*in = out
	return nil
}
func (in *PrepareInput) UnmarshalJSON(raw []byte) error {
	f, err := exactObject(raw, "request_id,purpose,game,input", "room_id")
	if err != nil {
		return err
	}
	var out PrepareInput
	if err = decodeFields(f, map[string]any{"request_id": &out.RequestID, "purpose": &out.Purpose, "game": &out.Game, "input": &out.Input}); err != nil {
		return err
	}
	if _, ok := f["room_id"]; ok {
		if out.Purpose != "COMMAND" {
			return Fault{Code: "INVALID_REQUEST"}
		}
		var id string
		if err = decodeFields(f, map[string]any{"room_id": &id}); err != nil {
			return err
		}
		out.RoomID = &id
	}
	if !validIntent(out) {
		return Fault{Code: "INVALID_REQUEST"}
	}
	*in = out
	return nil
}
func (in *TypedInput) UnmarshalJSON(raw []byte) error {
	f, err := exactObject(raw, "", "stake,players,expected_version,action,ready")
	if err != nil {
		return err
	}
	var out TypedInput
	if _, ok := f["stake"]; ok {
		out.Create = &CreateInput{}
		err = json.Unmarshal(raw, out.Create)
	} else {
		out.Command = &CommandInput{}
		err = json.Unmarshal(raw, out.Command)
	}
	if err != nil {
		return err
	}
	*in = out
	return nil
}
func (in *CreateInput) UnmarshalJSON(raw []byte) error {
	f, err := exactObject(raw, "stake,players", "")
	if err != nil {
		return err
	}
	var out CreateInput
	if err = decodeFields(f, map[string]any{"stake": &out.Stake, "players": &out.Players}); err != nil {
		return err
	}
	*in = out
	return nil
}
func (in *CommandInput) UnmarshalJSON(raw []byte) error {
	f, err := exactObject(raw, "expected_version,action,ready", "")
	if err != nil {
		return err
	}
	var out CommandInput
	var version string
	if err = decodeFields(f, map[string]any{"expected_version": &version, "action": &out.Action}); err != nil {
		return err
	}
	var ok bool
	out.ExpectedVersion, ok = canonicalInt(version)
	if !ok || out.ExpectedVersion < 1 {
		return Fault{Code: "INVALID_REQUEST"}
	}
	if json.Unmarshal(f["ready"], &out.Ready) != nil || (out.Action.Kind == "READY") != (out.Ready != nil) {
		return Fault{Code: "INVALID_REQUEST"}
	}
	*in = out
	return nil
}
func (in *ReadyConfirmation) UnmarshalJSON(raw []byte) error {
	f, err := exactObject(raw, "client_seed,config_hash,policy_hash,server_seed_hash,stake_units", "")
	if err != nil {
		return err
	}
	var out ReadyConfirmation
	var stake string
	if err = decodeFields(f, map[string]any{"client_seed": &out.ClientSeed, "config_hash": &out.ConfigHash, "policy_hash": &out.PolicyHash, "server_seed_hash": &out.ServerSeedHash, "stake_units": &stake}); err != nil {
		return err
	}
	var ok bool
	out.StakeUnits, ok = canonicalInt(stake)
	if !ok || out.StakeUnits <= 0 || !validSeed(out.ClientSeed) || !validHash(out.ConfigHash) || !validHash(out.PolicyHash) || !validHash(out.ServerSeedHash) {
		return Fault{Code: "INVALID_REQUEST"}
	}
	*in = out
	return nil
}
func (in *Action) UnmarshalJSON(raw []byte) error {
	f, err := exactObject(raw, "kind", "target,item,stolen_item,agree")
	if err != nil {
		return err
	}
	var out Action
	targets := map[string]any{"kind": &out.Kind, "target": &out.Target, "item": &out.Item, "stolen_item": &out.StolenItem, "agree": &out.Agree}
	for name := range targets {
		if _, ok := f[name]; !ok {
			delete(targets, name)
		}
	}
	if err = decodeFields(f, targets); err != nil {
		return err
	}
	if !roulette.ValidAction(domainAction(out), false) {
		return Fault{Code: "INVALID_REQUEST"}
	}
	count := 1
	switch out.Kind {
	case "DEVIL_SHOOT", "PRESSURE_VOTE":
		count = 2
	case "DEVIL_ITEM":
		count = 2
		if out.Item == "adrenaline" {
			count = 3
		}
	}
	if len(f) != count {
		return Fault{Code: "INVALID_REQUEST"}
	}
	*in = out
	return nil
}
